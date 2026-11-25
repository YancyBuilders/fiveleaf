package roundtable

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"fiveleaf/locales"
)

type MemoryDay struct {
	Date string
	Path string
}

type dailyMemory struct {
	mu          sync.Mutex
	conclusions map[string][]string
	pending     map[string]bool
	err         error
}

type memoryDayChanges struct {
	dates       map[string]bool
	conclusions map[string][]string
	reset       bool
}

func (changes *memoryDayChanges) mark(date string) {
	if changes.dates == nil {
		changes.dates = make(map[string]bool)
	}
	changes.dates[date] = true
}

func (changes memoryDayChanges) empty() bool { return !changes.reset && len(changes.dates) == 0 }

type memoryDayWriter struct {
	*dailyMemory
	archive *archive
	group   Group
	memory  memoryRecord
	stop    <-chan struct{}
}

func (group Group) memoryFolder() string {
	words := strings.FieldsFunc(group.Topic, func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsNumber(char) && !unicode.IsMark(char)
	})
	topic := strings.TrimRight(boundedText(strings.Join(words, "-"), 100), "-")
	return cmp.Or(topic, "group") + "-" + group.ID
}

func openMemoryFolder(parent *os.Root, name string, create bool) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if create && errors.Is(err, fs.ErrNotExist) {
		if err := parent.Mkdir(name, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		info, err = parent.Lstat(name)
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("group memory requires a directory at %s", filepath.Join(parent.Name(), name))
	}
	return parent.OpenRoot(name)
}

func (library Library) memoryRoot(group Group, create bool) (*os.Root, error) {
	project, err := projectRoot(library.Project)
	if err != nil {
		return nil, err
	}
	if project != library.Project || projectKey(project) != library.Key() {
		return nil, fmt.Errorf("group memory project has changed: %s", library.Project)
	}
	root, err := os.OpenRoot(project)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	memory, err := openMemoryFolder(root, "memory", create)
	if err != nil {
		return nil, err
	}
	defer memory.Close()
	return openMemoryFolder(memory, group.memoryFolder(), create)
}

func listMemoryDays(root *os.Root) ([]MemoryDay, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var days []MemoryDay
	for _, entry := range entries {
		date, markdown := strings.CutSuffix(entry.Name(), ".md")
		if !markdown || !entry.Type().IsRegular() {
			continue
		}
		if _, err := time.Parse(time.DateOnly, date); err == nil {
			days = append(days, MemoryDay{Date: date, Path: filepath.Join(root.Name(), entry.Name())})
		}
	}
	slices.Reverse(days)
	return days, nil
}

func (r *Room) MemoryDays() ([]MemoryDay, error) {
	r.daily.mu.Lock()
	defer r.daily.mu.Unlock()
	_ = r.syncMemoryDays(false)
	r.mu.Lock()
	writer := r.memoryDayWriterLocked()
	r.mu.Unlock()
	if writer.archive == nil || writer.archive.library.Project == "" {
		return nil, nil
	}
	root, err := writer.archive.library.memoryRoot(writer.group, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, r.daily.err
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	days, err := listMemoryDays(root)
	return days, errors.Join(r.daily.err, err)
}

func (r *Room) SyncMemoryDays() error {
	r.daily.mu.Lock()
	defer r.daily.mu.Unlock()
	return r.syncMemoryDays(true)
}

func (r *Room) memoryDayWriterLocked() memoryDayWriter {
	writer := memoryDayWriter{dailyMemory: &r.daily, archive: r.archive, memory: r.memory}
	if writer.archive != nil {
		writer.group = writer.archive.group
	}
	return writer
}

func (r *Room) unlockMemoryDayChanges() {
	r.unlockMemoryDayChangesWith(r.flushMemoryDayChanges)
}

func (r *Room) unlockMemoryDayChangesWith(flush func()) {
	changed := !r.dayChanges.empty()
	r.mu.Unlock()
	if changed {
		flush()
	}
}

func (r *Room) flushMemoryDayChanges() {
	r.daily.mu.Lock()
	defer r.daily.mu.Unlock()
	_ = r.syncMemoryDays(false)
}

func (r *Room) syncMemoryDays(backfill bool) error {
	r.mu.Lock()
	if backfill && r.closed() {
		r.mu.Unlock()
		return ErrClosed
	}
	changes := r.dayChanges
	r.dayChanges = memoryDayChanges{}
	writer := r.memoryDayWriterLocked()
	r.mu.Unlock()
	if writer.archive == nil || writer.archive.library.Project == "" {
		return nil
	}
	if changes.empty() {
		if !backfill || writer.conclusions != nil && writer.err == nil {
			return writer.err
		}
		writer.stop = r.done
		writer.refresh(writer.pending)
		return writer.err
	}
	if changes.reset {
		writer.conclusions = make(map[string][]string)
		changes.dates = nil
	}
	if writer.conclusions != nil {
		for date, additions := range changes.conclusions {
			files := writer.conclusions[date]
			for _, name := range additions {
				at, found := slices.BinarySearch(files, name)
				if !found {
					files = slices.Insert(files, at, name)
				}
			}
			writer.conclusions[date] = files
		}
	}
	writer.refresh(changes.dates)
	return writer.err
}

func (writer memoryDayWriter) stopped() bool {
	select {
	case <-writer.stop:
		return true
	default:
		return false
	}
}

func conclusionDate(msg Message, after time.Time) string {
	if msg.ToolOnly || !msg.Time.After(after) || strings.TrimSpace(msg.Text) == "" {
		return ""
	}
	when := msg.Time
	switch {
	case msg.Role == "user" && msg.HumanConclusion && msg.Task == nil:
	case msg.Role == "assistant" && msg.Task != nil && msg.Task.Stage == Conclusion && msg.Task.Status == Completed && !msg.Task.NoAnswer:
		if !msg.Task.Finished.IsZero() {
			when = msg.Task.Finished
		}
	default:
		return ""
	}
	return when.Local().Format(time.DateOnly)
}

func (group Group) noteDate(note MemoryNote) string {
	when := note.Time
	if when.IsZero() {
		when = group.Created
	}
	return when.Local().Format(time.DateOnly)
}

func (writer memoryDayWriter) indexConclusions() error {
	files, _, err := writer.archive.filesBefore("")
	if err != nil {
		return err
	}
	conclusions := make(map[string][]string)
	dir := filepath.Join(writer.archive.dir, "messages")
	cutoff := writer.memory.After.UTC().Format("20060102T150405.000000000")
	for _, name := range files {
		if writer.stopped() {
			return ErrClosed
		}
		if name < cutoff {
			continue
		}
		msg, err := readArchivedMessage(dir, name)
		if err != nil {
			return err
		}
		if date := conclusionDate(msg, writer.memory.After); date != "" {
			conclusions[date] = append(conclusions[date], name)
		}
	}
	writer.conclusions = conclusions
	return nil
}

func (r *Room) queueConclusionDayLocked(msg Message) {
	if r.archive.library.Project == "" {
		return
	}
	if date := conclusionDate(msg, r.memory.After); date != "" {
		if r.dayChanges.conclusions == nil {
			r.dayChanges.conclusions = make(map[string][]string)
		}
		r.dayChanges.conclusions[date] = append(r.dayChanges.conclusions[date], messageFile(msg))
		r.dayChanges.mark(date)
	}
}

func (r *Room) queueNoteDaysLocked(previous memoryRecord) {
	if r.archive == nil || r.archive.library.Project == "" {
		return
	}
	if !previous.After.Equal(r.memory.After) {
		r.dayChanges = memoryDayChanges{reset: true}
		return
	}
	if slices.Equal(previous.Notes, r.memory.Notes) {
		return
	}
	for _, note := range previous.Notes {
		if !slices.Contains(r.memory.Notes, note) {
			r.dayChanges.mark(r.archive.group.noteDate(note))
		}
	}
	for _, note := range r.memory.Notes {
		if !slices.Contains(previous.Notes, note) {
			r.dayChanges.mark(r.archive.group.noteDate(note))
		}
	}
}

func (writer memoryDayWriter) refresh(dates map[string]bool) {
	if writer.conclusions == nil {
		if err := writer.indexConclusions(); err != nil {
			writer.err = err
			return
		}
		dates = nil
	}
	if writer.err != nil && writer.pending == nil {
		dates = nil
	}
	if dates != nil {
		for date := range writer.pending {
			dates[date] = true
		}
	}
	writer.pending = dates
	writer.err = writer.writeDays(dates)
}

func (writer memoryDayWriter) writeDays(dates map[string]bool) error {
	notes := make(map[string][]MemoryNote)
	for _, note := range writer.memory.Notes {
		date := writer.group.noteDate(note)
		notes[date] = append(notes[date], note)
	}
	create := len(notes) > 0 || len(writer.conclusions) > 0
	root, err := writer.archive.library.memoryRoot(writer.group, create)
	if !create && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if dates == nil {
		days, err := listMemoryDays(root)
		if err != nil {
			return err
		}
		dates = make(map[string]bool)
		for _, day := range days {
			dates[day.Date] = true
		}
		for date := range writer.conclusions {
			dates[date] = true
		}
		for date := range notes {
			dates[date] = true
		}
	}
	var problems error
	writer.pending = dates
	for date := range dates {
		if writer.stopped() {
			return errors.Join(problems, ErrClosed)
		}
		var render func(io.Writer) error
		if len(notes[date])+len(writer.conclusions[date]) != 0 {
			render = func(output io.Writer) error { return writer.renderDay(output, date, notes[date]) }
		}
		if err := updateDayFile(root, writer.group.ID, date, render); err != nil {
			problems = errors.Join(problems, fmt.Errorf("%s: %w", filepath.Join(root.Name(), date+".md"), err))
		} else {
			delete(dates, date)
		}
	}
	return problems
}

func (writer memoryDayWriter) renderDay(output io.Writer, date string, notes []MemoryNote) error {
	text := bufio.NewWriter(output)
	fmt.Fprintf(text, "# %s · %s · %s\n", locales.RoundtableGroupMemory(), writer.group.Topic, date)
	if len(notes) != 0 {
		fmt.Fprintf(text, "\n## %s\n\n", locales.RoundtablePinnedNotesHeading())
		for _, note := range notes {
			fmt.Fprintf(text, "- %s\n", strings.ReplaceAll(note.Text, "\n", "\n  "))
		}
	}
	if files := writer.conclusions[date]; len(files) > 0 {
		fmt.Fprintf(text, "\n## %s\n", locales.RoundtableConclusion())
		for _, name := range files {
			if writer.stopped() {
				return ErrClosed
			}
			msg, err := readArchivedMessage(filepath.Join(writer.archive.dir, "messages"), name)
			if err != nil {
				return err
			}
			if conclusionDate(msg, writer.memory.After) != date {
				return errors.New("roundtable history changed while saving group memory")
			}
			fmt.Fprintf(text, "\n%s\n", msg.Text)
		}
	}
	return text.Flush()
}

package roundtable

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxMemoryNoteBytes  = 4096
	MaxMemoryNotes      = 32
	DefaultMemoryWindow = 12
	memoryExcerptBytes  = 4096
	memoryRecentLimit   = (MaxContextMessages + 1) * (MaxHops + 1)
	memoryHistoryBudget = 512 << 10
)

var (
	ErrMemoryBusy      = errors.New("wait for the current turn to finish before changing meeting memory")
	ErrMemoryNote      = errors.New("enter a nonempty memory note of at most 4096 bytes")
	ErrMemoryLimit     = errors.New("the meeting already has 32 pinned notes")
	ErrMemoryDuplicate = errors.New("this note is already pinned")
	ErrMemorySource    = errors.New("the source must be a public message in this meeting")
	ErrMemoryMissing   = errors.New("the pinned note no longer exists")
	ErrMemoryWindow    = errors.New("the memory window must be between 1 and 100 turns")
)

type MemoryNote struct {
	ID       string    `json:"id"`
	Text     string    `json:"text"`
	SourceID string    `json:"sourceId,omitempty"`
	Time     time.Time `json:"time"`
}

type MemorySnapshot struct {
	Notes  []MemoryNote
	Recent []Message
	Shared bool
	Window int
}

type memoryRecord struct {
	Notes  []MemoryNote `json:"notes,omitempty"`
	Paused bool         `json:"paused,omitempty"`
	After  time.Time    `json:"after,omitzero"`
	Window int          `json:"window,omitempty"`
}

func (r *Room) memoryWindowLocked() int {
	if r.memory.Window != 0 {
		return r.memory.Window
	}
	return r.options.ContextMessages
}

func (r *Room) Memory() MemorySnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	window := r.memoryWindowLocked()
	return MemorySnapshot{Notes: slices.Clone(r.memory.Notes), Recent: slices.Clone(lastMemoryTurns(r.recent, window)), Shared: !r.memory.Paused, Window: window}
}

func lastMemoryTurns(messages []Message, limit int) []Message {
	start, turns := len(messages), 0
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			start = index
			turns++
			if turns == limit {
				break
			}
		}
	}
	return messages[start:]
}

func memoryMessage(msg Message) (Message, bool) {
	if msg.Text == "" || msg.ToolOnly || msg.Role != "user" && msg.Role != "assistant" {
		return Message{}, false
	}
	if task := msg.Task; task != nil && (task.Status != Completed || task.NoAnswer || task.Stage != Direct && task.Stage != Conclusion) {
		return Message{}, false
	}
	if msg.Role == "assistant" && msg.Task == nil {
		msg.Text = legacyPiAnswer(msg.Text)
		if msg.Text == "" || hasLeadingMention(msg.Text) {
			return Message{}, false
		}
	}
	msg.Task, msg.Original = nil, ""
	if len(msg.Text) > memoryExcerptBytes {
		msg.Text = boundedText(msg.Text, memoryExcerptBytes-3) + "…"
	}
	return msg, true
}

func (r *Room) rememberLocked(msg Message) {
	if !msg.Time.After(r.memory.After) {
		return
	}
	clean, ok := memoryMessage(msg)
	if !ok {
		return
	}
	r.recent = append(r.recent, clean)
	if len(r.recent) > memoryRecentLimit {
		r.recent = slices.Clone(r.recent[len(r.recent)-memoryRecentLimit:])
	}
}

func (r *Room) loadMemory() error {
	file := filepath.Join(r.archive.dir, "memory.json")
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &r.memory); err != nil {
			return fmt.Errorf("reading meeting memory: %w", err)
		}
		if err := r.memory.validate(); err != nil {
			return err
		}
	}
	for _, msg := range r.messages {
		r.rememberLocked(msg)
	}
	if !r.earlier || len(r.messages) == 0 || !r.messages[0].Time.After(r.memory.After) {
		return nil
	}
	turns := 0
	for _, msg := range r.recent {
		if msg.Role == "user" {
			turns++
		}
	}
	if turns >= MaxContextMessages+1 {
		return nil
	}
	dir := filepath.Join(r.archive.dir, "messages")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	before := messageFile(r.messages[0])
	end, _ := slices.BinarySearchFunc(entries, before, func(entry os.DirEntry, name string) int { return strings.Compare(entry.Name(), name) })
	var older []Message
	for index := end - 1; index >= 0 && turns < MaxContextMessages+1 && len(older)+len(r.recent) < memoryRecentLimit; index-- {
		entry := entries[index]
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		msg, err := readArchivedMessage(dir, entry.Name())
		if err != nil {
			return err
		}
		if !msg.Time.After(r.memory.After) {
			break
		}
		if clean, ok := memoryMessage(msg); ok {
			older = append(older, clean)
			if clean.Role == "user" {
				turns++
			}
		}
	}
	slices.Reverse(older)
	r.recent = append(older, r.recent...)
	return nil
}

func validMemoryText(text string) bool {
	return strings.TrimSpace(text) != "" && len(text) <= MaxMemoryNoteBytes && utf8.ValidString(text)
}

func (state memoryRecord) validate() error {
	if len(state.Notes) > MaxMemoryNotes {
		return ErrMemoryLimit
	}
	if state.Window < 0 || state.Window > MaxContextMessages {
		return ErrMemoryWindow
	}
	seen := make(map[string]bool, len(state.Notes))
	for _, note := range state.Notes {
		if !validMemoryText(note.Text) || note.ID == "" || len(note.ID) > 256 || seen[note.ID] || len(note.SourceID) > 256 {
			return ErrMemoryNote
		}
		seen[note.ID] = true
	}
	return nil
}

func (r *Room) storeMemoryLocked(next memoryRecord) error {
	if r.archive != nil {
		if err := writeRecord(filepath.Join(r.archive.dir, "memory.json"), next); err != nil {
			return err
		}
	}
	if !next.After.Equal(r.memory.After) {
		r.recent = nil
	}
	previous := r.memory
	r.memory = next
	r.queueNoteDaysLocked(previous)
	r.notifyLocked()
	return nil
}

func (r *Room) changeMemory(change func(*memoryRecord) error) error {
	r.mu.Lock()
	defer r.unlockMemoryDayChanges()
	if r.closed() {
		return ErrClosed
	}
	if r.chain != nil {
		return ErrMemoryBusy
	}
	next := r.memory
	next.Notes = slices.Clone(next.Notes)
	if err := change(&next); err != nil {
		return err
	}
	return r.storeMemoryLocked(next)
}

func (r *Room) SetMemorySharing(shared bool) error {
	return r.changeMemory(func(state *memoryRecord) error { state.Paused = !shared; return nil })
}

func (r *Room) SetMemoryWindow(turns int) error {
	if turns < 1 || turns > MaxContextMessages {
		return ErrMemoryWindow
	}
	return r.changeMemory(func(state *memoryRecord) error { state.Window = turns; return nil })
}

func (r *Room) publicSourceLocked(id string) error {
	if len(id) > 256 || !utf8.ValidString(id) || strings.ContainsAny(id, "/\\") {
		return ErrMemorySource
	}
	if slices.ContainsFunc(r.recent, func(msg Message) bool { return msg.ID == id }) {
		return nil
	}
	for _, msg := range r.messages {
		if msg.ID == id {
			if _, ok := memoryMessage(msg); ok {
				return nil
			}
			return ErrMemorySource
		}
	}
	if r.archive != nil && r.earlier {
		dir := filepath.Join(r.archive.dir, "messages")
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), "-"+id+".json") {
				msg, err := readArchivedMessage(dir, entry.Name())
				if err != nil {
					return err
				}
				if _, ok := memoryMessage(msg); ok && msg.ID == id {
					return nil
				}
			}
		}
	}
	return ErrMemorySource
}

func (r *Room) AddMemory(text, sourceID string) error {
	text = strings.TrimSpace(text)
	if !validMemoryText(text) {
		return ErrMemoryNote
	}
	return r.changeMemory(func(state *memoryRecord) error {
		for _, note := range state.Notes {
			if note.Text == text || sourceID != "" && note.SourceID == sourceID {
				return ErrMemoryDuplicate
			}
		}
		if len(state.Notes) == MaxMemoryNotes {
			return ErrMemoryLimit
		}
		if sourceID != "" {
			if err := r.publicSourceLocked(sourceID); err != nil {
				return err
			}
		}
		state.Notes = append(state.Notes, MemoryNote{ID: rand.Text(), Text: text, SourceID: sourceID, Time: time.Now()})
		return nil
	})
}

func (r *Room) RemoveMemory(id string) error {
	return r.changeMemory(func(state *memoryRecord) error {
		index := slices.IndexFunc(state.Notes, func(note MemoryNote) bool { return note.ID == id })
		if index < 0 {
			return ErrMemoryMissing
		}
		state.Notes = slices.Delete(state.Notes, index, index+1)
		return nil
	})
}

func (r *Room) ClearMemory() error {
	r.mu.Lock()
	defer r.unlockMemoryDayChanges()
	if r.closed() {
		return ErrClosed
	}
	if r.chain != nil {
		return ErrMemoryBusy
	}
	return r.storeMemoryLocked(r.clearedMemoryLocked())
}

func (r *Room) clearedMemoryLocked() memoryRecord {
	next := r.memory
	next.Notes = nil
	if len(r.messages) > 0 && r.messages[len(r.messages)-1].Time.After(next.After) {
		next.After = r.messages[len(r.messages)-1].Time
	}
	return next
}

func (r *Room) priorContextLocked(turnID string, current Message) ([]Message, int, int) {
	result := make([]Message, 0, 3)
	if r.memory.Paused {
		return append(result, current), 0, 0
	}
	var pinned strings.Builder
	for _, note := range r.memory.Notes {
		pinned.WriteString(strconv.Quote(note.Text))
		pinned.WriteByte('\n')
	}
	if pinned.Len() != 0 {
		result = append(result, Message{Role: "user", Name: "Pinned meeting notes", Text: pinned.String()})
	}
	end := slices.IndexFunc(r.recent, func(msg Message) bool { return msg.ID == turnID })
	if end < 0 {
		end = len(r.recent)
	}
	recent := lastMemoryTurns(r.recent[:end], r.memoryWindowLocked())
	var turns []string
	bytes := 0
	for end = len(recent); end > 0; {
		start := end - 1
		for start > 0 && recent[start].Role != "user" {
			start--
		}
		var turn strings.Builder
		for _, msg := range recent[start:end] {
			fmt.Fprintf(&turn, "%s %q: %q\n", msg.Role, boundedText(msg.Name, 256), msg.Text)
		}
		if bytes+turn.Len() > memoryHistoryBudget {
			break
		}
		turns = append(turns, turn.String())
		bytes += turn.Len()
		end = start
	}
	if len(turns) > 0 {
		slices.Reverse(turns)
		result = append(result, Message{Role: "user", Name: "Earlier public meeting conversation", Text: strings.Join(turns, "")})
	}
	return append(result, current), len(r.memory.Notes), len(turns)
}

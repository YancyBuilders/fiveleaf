package roundtable

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fiveleaf/locales"
)

func dailyRoom(t *testing.T, topic string) (Library, Group, *Room) {
	t.Helper()
	library, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.CreateWithChair(topic, []Participant{{ID: ModeratorID, Name: "Chair", Provider: "local"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	room, err := library.OpenRoom(group, []Member{{ID: ModeratorID, Enabled: true, Adapter: answer("Adopt the proposal.")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(room.Close)
	return library, group, room
}

func dayText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMemoryDaysLiveArchive(t *testing.T) {
	library, group, room := dailyRoom(t, "../设计 / Decisions \\ <v2>")
	if err := room.SetSystemPrompt("private prompt"); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss(group.Topic, "routine remark", ""); err != nil {
		t.Fatal(err)
	}
	if days, err := room.MemoryDays(); err != nil || len(days) != 0 {
		t.Fatal(days, err)
	}
	if err := room.AddMemory("Keep the deadline.\nNo extensions.", ""); err != nil {
		t.Fatal(err)
	}
	if err := room.PublishConclusion("Publish on Friday."); err != nil {
		t.Fatal(err)
	}
	if err := room.SetHumanChair(false); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss(group.Topic, "please decide", ""); err != nil {
		t.Fatal(err)
	}
	finished := awaitRoom(t, room, func(state Snapshot) bool { return !state.Busy })
	date := finished.Messages[len(finished.Messages)-1].Task.Finished.Local().Format(time.DateOnly)
	days, err := room.MemoryDays()
	if err != nil || len(days) != 1 || days[0].Date != date || !filepath.IsAbs(days[0].Path) {
		t.Fatal(days, err)
	}
	path := days[0].Path
	groupDir := filepath.Dir(path)
	if filepath.Dir(groupDir) != filepath.Join(library.Project, "memory") || filepath.Base(path) != date+".md" || !strings.Contains(filepath.Base(groupDir), "设计") || !strings.Contains(filepath.Base(groupDir), group.ID) {
		t.Fatal(path)
	}
	text := dayText(t, path)
	for _, fragment := range []string{locales.RoundtableGroupMemory(), locales.RoundtablePinnedNotesHeading(), locales.RoundtableConclusion(), "Keep the deadline.", "No extensions.", "Publish on Friday.", "Adopt the proposal."} {
		if !strings.Contains(text, fragment) {
			t.Fatal(fragment, text)
		}
	}
	for _, private := range []string{"routine remark", "please decide", "private prompt"} {
		if strings.Contains(text, private) {
			t.Fatal(private, text)
		}
	}
	if err := room.RemoveMemory(room.Memory().Notes[0].ID); err != nil {
		t.Fatal(err)
	}
	if text := dayText(t, path); strings.Contains(text, "Keep the deadline.") || !strings.Contains(text, "Publish on Friday.") {
		t.Fatal(text)
	}
	if err := room.ClearMemory(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || len(room.Snapshot().Messages) != len(finished.Messages) {
		t.Fatal("clear memory", err, room.Snapshot())
	}
	if err := room.SyncMemoryDays(); err != nil {
		t.Fatal(err)
	}
	if days, err := room.MemoryDays(); len(days) != 0 || err != nil {
		t.Fatal("cleared decisions returned", days, err)
	}
}

func TestMemoryDaysBackfillReadOnlyAndLocalDates(t *testing.T) {
	library, group, opened := dailyRoom(t, "Migration")
	opened.Close()
	store := &archive{dir: filepath.Join(library.dir, group.ID)}
	start := time.Date(2023, 3, 1, 23, 59, 0, 0, time.Local)
	end := start.Add(2 * time.Minute)
	write := func(message Message) {
		t.Helper()
		if err := store.append(message); err != nil {
			t.Fatal(err)
		}
	}
	write(Message{ID: "conclusion", Role: "assistant", Text: "Finish after midnight", Time: start.UTC(), Task: &Task{Stage: Conclusion, Status: Completed, Finished: end.UTC(), Assignment: "private assignment", Tools: []ToolRecord{{Detail: "private tool result"}}}})
	write(Message{ID: "human", Role: "user", Text: "Human decision", Time: start.Add(-time.Hour).UTC(), HumanConclusion: true})
	for index, task := range []*Task{nil, {Stage: Direct, Status: Completed}, {Stage: Work, Status: Completed}, {Stage: Coordinate, Status: Completed}, {Stage: Conclusion, Status: Failed}, {Stage: Conclusion, Status: Stopped}, {Stage: Conclusion, Status: Running}, {Stage: Conclusion, Status: Completed, NoAnswer: true}} {
		write(Message{ID: fmt.Sprintf("excluded-%d", index), Role: "assistant", Text: "excluded answer", Time: start.Add(-time.Duration(index+1) * time.Minute), Task: task})
	}
	for index := range MaxHistoryMessages + 1 {
		write(Message{ID: fmt.Sprintf("remark-%d", index), Role: "user", Text: "ordinary history", Time: end.Add(time.Duration(index+1) * time.Hour)})
	}
	if err := writeRecord(filepath.Join(store.dir, "memory.json"), memoryRecord{Notes: []MemoryNote{{ID: "pin", Text: "A kept note", Time: start.UTC()}}}); err != nil {
		t.Fatal(err)
	}
	room, err := library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	if !room.Snapshot().HasEarlier {
		t.Fatal("migration fixture did not cross the loaded archive page")
	}
	if days, err := room.MemoryDays(); len(days) != 0 || err != nil {
		t.Fatal(days, err)
	}
	if _, err := os.Stat(filepath.Join(library.Project, "memory")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only archive open wrote into the project", err)
	}
	if err := room.SyncMemoryDays(); err != nil {
		t.Fatal(err)
	}
	days, err := room.MemoryDays()
	if err != nil || len(days) != 2 || days[0].Date != "2023-03-02" || days[1].Date != "2023-03-01" {
		t.Fatal(days, err)
	}
	late, early := dayText(t, days[0].Path), dayText(t, days[1].Path)
	if strings.Count(late, "Finish after midnight") != 1 || strings.Contains(early, "Finish after midnight") || !strings.Contains(early, "A kept note") || !strings.Contains(early, "Human decision") {
		t.Fatal(late, early)
	}
	for _, private := range []string{"excluded answer", "private tool", "private assignment", "ordinary history"} {
		if strings.Contains(late+early, private) {
			t.Fatal(private, late, early)
		}
	}
	before, err := os.Stat(days[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := room.SyncMemoryDays(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(days[0].Path)
	if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged day rewritten", before, after, err)
	}
	reader, err := library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if read, err := reader.MemoryDays(); err != nil || !slices.Equal(read, days) {
		t.Fatal(read, err)
	}
	if err := room.Clear(); err != nil {
		t.Fatal(err)
	}
	if read, err := reader.MemoryDays(); err != nil || len(read) != 0 || len(room.Snapshot().Messages) != 0 {
		t.Fatal(read, err, room.Snapshot())
	}
}

func TestMemoryDaysBackfillKeepsRoomReadable(t *testing.T) {
	_, group, room := dailyRoom(t, "Concurrent backfill")
	if err := room.Discuss(group.Topic, "Agenda", ""); err != nil {
		t.Fatal(err)
	}
	<-room.Events()
	room.archive.filesMu.Lock()
	releaseScan := sync.OnceFunc(room.archive.filesMu.Unlock)
	defer releaseScan()
	backfilled := make(chan error, 1)
	go func() { backfilled <- room.SyncMemoryDays() }()
	deadline := time.After(3 * time.Second)
	for room.daily.mu.TryLock() {
		room.daily.mu.Unlock()
		select {
		case <-deadline:
			t.Fatal("the archive scan did not start")
		default:
			runtime.Gosched()
		}
	}
	const conclusion = "Keep the existing plan."
	published := make(chan error, 1)
	go func() { published <- room.PublishConclusionWithDeferredMemory(conclusion) }()
	select {
	case <-room.Events():
	case <-time.After(3 * time.Second):
		t.Fatal("conclusion was not accepted while the archive scan waited")
	}
	read := make(chan bool, 1)
	go func() {
		snapshot, memory := room.Snapshot(), room.Memory()
		read <- !room.Busy() && len(snapshot.Messages) == 2 && snapshot.Messages[1].Text == conclusion && len(memory.Recent) == 2
	}()
	select {
	case current := <-read:
		if !current {
			t.Fatal("the snapshot did not include the new conclusion")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("room reads waited on the archive scan")
	}
	select {
	case err := <-published:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("publication waited for daily Markdown instead of returning after the archive commit")
	}
	const remark = "A remark after publication."
	if err := room.Discuss(group.Topic, remark, ""); err != nil {
		t.Fatal(err)
	}
	messages := room.Snapshot().Messages
	archived, _, err := room.archive.readPage([]string{messageFile(messages[1]), messageFile(messages[2])}, 2, nil)
	if err != nil || len(archived) != 2 || archived[0].Text != conclusion || archived[1].Text != remark || !archived[0].Time.Before(archived[1].Time) {
		t.Fatalf("archive did not commit the conclusion before subsequent remarks: %v, %v", archived, err)
	}
	closing := room.CloseAsync()
	select {
	case <-closing:
		t.Fatal("close reported a drained archive while its scan was blocked")
	default:
	}
	if err := room.Discuss(group.Topic, "After close", ""); !errors.Is(err, ErrClosed) {
		t.Fatalf("a closing room still accepts messages: %v", err)
	}
	releaseScan()
	if err := <-backfilled; err != nil && !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	<-closing
	days, err := room.MemoryDays()
	if err != nil || len(days) != 1 || strings.Count(dayText(t, days[0].Path), conclusion) != 1 {
		t.Fatal("the conclusion must appear in exactly one daily section", days, err)
	}
}

func TestMemoryDaysPreserveUserMarkdown(t *testing.T) {
	_, _, room := dailyRoom(t, "Annotations")
	if err := room.AddMemory("First note", ""); err != nil {
		t.Fatal(err)
	}
	days, err := room.MemoryDays()
	if err != nil || len(days) != 1 {
		t.Fatal(days, err)
	}
	path := days[0].Path
	const prefix = "# My notebook\n\n"
	const suffix = "\nPersonal follow-up without a final newline."
	original := prefix + dayText(t, path) + suffix
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(filepath.Dir(path), "2001-01-01.md")
	if err := os.WriteFile(foreign, []byte("My independent journal"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := room.PublishConclusion("Second decision"); err != nil {
		t.Fatal(err)
	}
	withDecision := dayText(t, path)
	if !strings.HasPrefix(withDecision, prefix) || !strings.HasSuffix(withDecision, suffix) || !strings.Contains(withDecision, "Second decision") {
		t.Fatal(withDecision)
	}
	edited := strings.Replace(withDecision, "First note", "My correction", 1)
	if err := os.WriteFile(path, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	if err := room.AddMemory("Pending note", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := room.MemoryDays(); err == nil || dayText(t, path) != edited || len(room.Memory().Notes) != 2 {
		t.Fatal("a conflict should preserve the Markdown and the archived note", err)
	}
	if err := room.SyncMemoryDays(); err == nil || dayText(t, path) != edited {
		t.Fatal("retry overwrote the user's edit", err)
	}
	if err := os.WriteFile(path, []byte(withDecision), 0600); err != nil {
		t.Fatal(err)
	}
	if err := room.SyncMemoryDays(); err != nil || !strings.Contains(dayText(t, path), "Pending note") {
		t.Fatal("failed to recover after resolving conflict", err)
	}
	if err := room.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := dayText(t, path); got != prefix+suffix || dayText(t, foreign) != "My independent journal" {
		t.Fatal("clear changed personal Markdown", got)
	}
}

func TestMemoryDaysRejectSymlinksAndOwnershipConflicts(t *testing.T) {
	for _, level := range []string{"project", "memory", "group", "day", "unowned"} {
		t.Run(level, func(t *testing.T) {
			library, group, room := dailyRoom(t, "../../Boundary \\ / 🔐")
			memory := filepath.Join(library.Project, "memory")
			groupDir := filepath.Join(memory, group.memoryFolder())
			date := time.Now().Local().Format(time.DateOnly)
			path := filepath.Join(groupDir, date+".md")
			external := t.TempDir()
			private := filepath.Join(external, "private.md")
			const untouched = "Unrelated personal Markdown"
			if err := os.WriteFile(private, []byte(untouched), 0600); err != nil {
				t.Fatal(err)
			}
			var setup error
			switch level {
			case "project":
				if err := os.Remove(library.Project); err != nil {
					t.Fatal(err)
				}
				setup = os.Symlink(external, library.Project)
			case "memory":
				setup = os.Symlink(external, memory)
			case "group":
				if err := os.Mkdir(memory, 0700); err != nil {
					t.Fatal(err)
				}
				setup = os.Symlink(external, groupDir)
			case "day", "unowned":
				if err := os.MkdirAll(groupDir, 0700); err != nil {
					t.Fatal(err)
				}
				if level == "unowned" {
					setup = os.WriteFile(path, []byte(untouched), 0600)
				} else {
					setup = os.Symlink(private, path)
				}
			}
			if setup != nil {
				t.Fatal(setup)
			}
			if err := room.PublishConclusion("Stored in the application archive"); err != nil || len(room.Snapshot().Messages) != 1 {
				t.Fatal(err, room.Snapshot())
			}
			if days, err := room.MemoryDays(); err == nil || level != "unowned" && len(days) != 0 {
				t.Fatal(days, err)
			}
			if err := room.SyncMemoryDays(); err == nil {
				t.Fatal("expected a filesystem conflict")
			}
			if err := room.Clear(); err != nil {
				t.Fatal(err)
			}
			if entries, err := os.ReadDir(external); err != nil || len(entries) != 1 || dayText(t, private) != untouched {
				t.Fatal("external directory changed", entries, err)
			}
			if level == "unowned" && dayText(t, path) != untouched {
				t.Fatal("overwrote an existing daily document")
			}
		})
	}
}

func TestMemoryDaysRetryOnlyPendingDates(t *testing.T) {
	library, group, opened := dailyRoom(t, "Partial migration")
	opened.Close()
	store := &archive{dir: filepath.Join(library.dir, group.ID)}
	start := time.Date(2022, 1, 1, 12, 0, 0, 0, time.Local)
	first := Message{ID: "first", Time: start, Role: "user", HumanConclusion: true, Text: "First decision"}
	second := Message{ID: "second", Time: start.AddDate(0, 0, 1), Role: "user", HumanConclusion: true, Text: "Second decision"}
	for _, msg := range []Message{first, second} {
		if err := store.append(msg); err != nil {
			t.Fatal(err)
		}
	}
	room, err := library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	folder := filepath.Join(library.Project, "memory", group.memoryFolder())
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(folder, "2022-01-02.md")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := room.SyncMemoryDays(); err == nil || !strings.Contains(dayText(t, filepath.Join(folder, "2022-01-01.md")), first.Text) {
		t.Fatal("expected one migrated day and one conflict", err)
	}
	if err := os.Remove(filepath.Join(store.dir, "messages", messageFile(first))); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if err := room.SyncMemoryDays(); err != nil || !strings.Contains(dayText(t, blocked), second.Text) {
		t.Fatal("retry should only need the failed day's source", err)
	}
	if err := os.Rename(filepath.Join(store.dir, "messages"), filepath.Join(store.dir, "messages.unavailable")); err != nil {
		t.Fatal(err)
	}
	if err := room.SyncMemoryDays(); err != nil {
		t.Fatal("repeat sync reread the archive", err)
	}
	if days, err := room.MemoryDays(); err != nil || len(days) != 2 {
		t.Fatal(days, err)
	}
}

package roundtable

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMemoryRecalledInActualMemberCalls(t *testing.T) {
	worker, calls := heldAdapter()
	room := New([]Member{{ID: "tester", Enabled: true, Adapter: worker}}, Options{})
	defer room.Close()
	if err := room.AddMemory("The fixed estimate is 120 seconds", ""); err != nil {
		t.Fatal(err)
	}
	if err := room.SetMemoryWindow(2); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		question := fmt.Sprintf("question-%d", index)
		if err := room.Discuss("Review", question, "tester"); err != nil {
			t.Fatal(err)
		}
		call := nextCall(t, calls)
		if got := call.messages[len(call.messages)-1]; got.Role != "user" || got.Text != question {
			t.Fatal(got)
		}
		for _, m := range call.messages {
			if m.Role == "system" && strings.Contains(m.Text, "120 seconds") {
				t.Fatal("note elevated to system instructions")
			}
		}
		prompt := FormatPrompt(call.messages)
		if !strings.Contains(prompt, "120 seconds") {
			t.Fatal(prompt)
		}
		for previous := 0; previous < index; previous++ {
			want := previous >= index-2
			for _, fragment := range []string{fmt.Sprintf("question-%d", previous), fmt.Sprintf("answer-%d", previous)} {
				if strings.Contains(prompt, fragment) != want {
					t.Fatal(fragment, prompt)
				}
			}
		}
		if err := room.ClearMemory(); !errors.Is(err, ErrMemoryBusy) {
			t.Fatal(err)
		}
		call.answer <- Message{Text: fmt.Sprintf("answer-%d", index)}
		completed := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
		task := completed.Messages[len(completed.Messages)-1].Task
		if task.MemoryNotes != 1 || task.MemoryTurns != min(index, 2) {
			t.Fatal(task)
		}
	}
	recent := room.Memory()
	if !recent.Shared || recent.Window != 2 || len(recent.Notes) != 1 || len(recent.Recent) != 4 || recent.Recent[0].Text != "question-2" {
		t.Fatal(recent)
	}
}

func TestMemoryPausedCurrentTurnCanStillReportFindings(t *testing.T) {
	moderator, decisions := heldAdapter()
	worker, work := heldAdapter()
	room := New([]Member{{ID: ModeratorID, Enabled: true, Adapter: moderator}, {ID: "tester", Enabled: true, Adapter: worker}}, Options{})
	defer room.Close()
	if err := room.AddMemory("old pinned marker", ""); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss("topic", "old question marker", "tester"); err != nil {
		t.Fatal(err)
	}
	nextCall(t, work).answer <- Message{Text: "old answer marker"}
	awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if err := room.SetMemorySharing(false); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss("topic", "fresh question", ""); err != nil {
		t.Fatal(err)
	}
	check := func(call heldCall) {
		t.Helper()
		prompt := FormatPrompt(call.messages)
		for _, marker := range []string{"old pinned marker", "old question marker", "old answer marker", "private read"} {
			if strings.Contains(prompt, marker) {
				t.Fatal(prompt)
			}
		}
		if !strings.Contains(prompt, "fresh question") {
			t.Fatal(prompt)
		}
	}
	opening := nextCall(t, decisions)
	check(opening)
	opening.answer <- Message{Text: "@tester inspect current case"}
	finding := nextCall(t, work)
	check(finding)
	ReportTool(finding.ctx, ToolRecord{ID: "log", Detail: "private read"})
	finding.answer <- Message{Text: "new evidence"}
	closing := nextCall(t, decisions)
	check(closing)
	if !strings.Contains(FormatPrompt(closing.messages), "new evidence") {
		t.Fatal(closing.messages)
	}
	closing.answer <- Message{Text: "public conclusion"}
	snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if snapshot.Messages[len(snapshot.Messages)-1].Task.MemoryTurns != 0 {
		t.Fatal(snapshot)
	}
	for _, message := range room.Memory().Recent {
		if message.Task != nil || strings.Contains(message.Text, "inspect current case") || strings.Contains(message.Text, "new evidence") {
			t.Fatal(message)
		}
	}
}

func TestMemoryPersistedCutoffAndArchiveSources(t *testing.T) {
	library, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.Create("Topic", []Participant{{"person", "Member", "local"}})
	if err != nil {
		t.Fatal(err)
	}
	room, err := library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	ids := make([]string, 0, MaxHistoryMessages+4)
	for index := range MaxHistoryMessages + 4 {
		msg := Message{ID: fmt.Sprintf("message-%d", index), Role: "user", Text: fmt.Sprintf("question#%d.", index), Time: time.Now()}
		room.mu.Lock()
		err = room.recordLocked(msg)
		room.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, msg.ID)
	}
	if err = room.AddMemory("historical source", ids[0]); err != nil {
		t.Fatal(err)
	}
	if err = room.AddMemory("bogus reference", "missing"); err == nil {
		t.Fatal("unknown source accepted")
	}
	if err = room.SetMemoryWindow(50); err != nil {
		t.Fatal(err)
	}
	if err = room.SetMemorySharing(false); err != nil {
		t.Fatal(err)
	}
	group, err = room.Invite(Participant{"other", "Second", "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err = room.SetOptions(Options{ContextMessages: 3, MaxHops: 2}); err != nil {
		t.Fatal(err)
	}
	room.Close()
	room, err = library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	snapshot := room.Memory()
	if len(snapshot.Notes) != 1 || snapshot.Notes[0].SourceID != ids[0] || snapshot.Shared || snapshot.Window != 50 || len(snapshot.Recent) != 50 {
		t.Fatalf("persisted memory = %d notes %d recent %t %d", len(snapshot.Notes), len(snapshot.Recent), snapshot.Shared, snapshot.Window)
	}
	before := room.Snapshot()
	if err = room.ClearMemory(); err != nil {
		t.Fatal(err)
	}
	if after := room.Snapshot(); len(after.Messages) != len(before.Messages) || !after.HasEarlier {
		t.Fatal(after)
	}
	room.Close()
	room, err = library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	if got := room.Memory(); len(got.Notes) != 0 || len(got.Recent) != 0 || got.Shared {
		t.Fatal(got)
	}
	if err = room.SetMemorySharing(true); err != nil {
		t.Fatal(err)
	}
	worker, calls := heldAdapter()
	room.SetMembers([]Member{{ID: "tester", Enabled: true, Adapter: worker}})
	if err = room.Discuss(group.Topic, "after reset", "tester"); err != nil {
		t.Fatal(err)
	}
	call := nextCall(t, calls)
	if prompt := FormatPrompt(call.messages); strings.Contains(prompt, "question#") || strings.Contains(prompt, "historical source") || !strings.Contains(prompt, "after reset") {
		t.Fatal(prompt)
	}
	call.answer <- Message{Text: "done"}
	awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if got := room.Memory(); len(got.Recent) != 2 {
		t.Fatal(got)
	}
	archivePath := filepath.Join(library.dir, group.ID, "memory.json")
	if stat, err := os.Stat(archivePath); err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal(stat, err)
	}
}

func TestMemoryBudgetMatchesSerializedPrompt(t *testing.T) {
	received := make(chan []Message, 1)
	room := New([]Member{{ID: "tester", Enabled: true, Adapter: AdapterFunc(func(_ context.Context, input []Message) (Message, error) {
		received <- input
		return Message{Text: "done"}, nil
	})}}, Options{})
	defer room.Close()
	if err := room.SetMemoryWindow(50); err != nil {
		t.Fatal(err)
	}
	for index := range MaxMemoryNotes {
		note := fmt.Sprintf("pin/%d/%s", index, strings.Repeat("\x01", MaxMemoryNoteBytes-10))
		if err := room.AddMemory(note, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := room.AddMemory("overflow", ""); err == nil {
		t.Fatal("note count limit not enforced")
	}
	room.mu.Lock()
	for index := range 50 {
		for _, role := range []string{"user", "assistant"} {
			room.appendLocked(Message{ID: fmt.Sprintf("%s/%d", role, index), Role: role, Text: fmt.Sprintf("sample/%d/%s", index, role), Time: time.Now()})
		}
		room.appendLocked(Message{ID: "internal", Role: "assistant", Task: &Task{Stage: Work, Status: Completed}, Text: "private finding", Time: time.Now()})
	}
	room.mu.Unlock()
	current := strings.Repeat("q", MaxMessageBytes-8) + "END-GOAL"
	if err := room.Discuss("topic", current, "tester"); err != nil {
		t.Fatal(err)
	}
	snap := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	task := snap.Messages[len(snap.Messages)-1].Task
	prompt := FormatPrompt(<-received)
	if len(prompt) > MaxPromptBytes || task.MemoryNotes != MaxMemoryNotes || task.MemoryTurns != 50 || !strings.Contains(prompt, current) || strings.Contains(prompt, "private finding") {
		t.Fatal(len(prompt), task)
	}
	for index := range 50 {
		if !strings.Contains(prompt, fmt.Sprintf("sample/%d/user", index)) || !strings.Contains(prompt, fmt.Sprintf("sample/%d/assistant", index)) {
			t.Fatal("memory turn lost", index)
		}
	}
	for index := range MaxMemoryNotes {
		if !strings.Contains(prompt, fmt.Sprintf("pin/%d/", index)) {
			t.Fatal("pin lost", index)
		}
	}
	notes := room.Memory().Notes
	notes[0].Text = "mutated"
	if room.Memory().Notes[0].Text == "mutated" {
		t.Fatal("mutable notes snapshot")
	}
	id := notes[0].ID
	if err := room.RemoveMemory(id); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(room.Memory().Notes, func(note MemoryNote) bool { return note.ID == id }) {
		t.Fatal("pin not removed")
	}
}

func TestMemoryRetainsPublicTurnsBeyondHiddenTasks(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.Create("Review", []Participant{{ID: "review", Name: "Reviewer", Provider: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	room, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	if got := room.Memory().Window; got != DefaultMemoryWindow {
		t.Fatal(got)
	}
	if err := room.SetMemoryWindow(50); err != nil {
		t.Fatal(err)
	}
	store := func(msg Message) {
		t.Helper()
		room.mu.Lock()
		defer room.mu.Unlock()
		if err := room.recordLocked(msg); err != nil {
			t.Fatal(err)
		}
	}
	for index := range 50 {
		store(Message{ID: fmt.Sprintf("q-%d", index), Time: time.Now(), Role: "user", Text: fmt.Sprintf("question %d", index)})
		for subtask := range 4 {
			store(Message{ID: fmt.Sprintf("w-%d-%d", index, subtask), Time: time.Now(), Role: "assistant", Text: "hidden finding", Task: &Task{Status: Completed, Stage: Work}})
		}
		store(Message{ID: fmt.Sprintf("a-%d", index), Time: time.Now(), Role: "assistant", Text: "public answer", Task: &Task{Status: Completed, Stage: Conclusion}})
	}
	recent := room.Memory().Recent
	if first := room.Snapshot().Messages[0].ID; first == "q-0" || len(recent) != 100 || recent[0].ID != "q-0" {
		t.Fatal(first, len(recent))
	}
	room.Close()
	reopened, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if records := reopened.Memory().Recent; !slices.EqualFunc(recent, records, func(a, b Message) bool { return a.ID == b.ID && a.Text == b.Text }) {
		t.Fatalf("different public history after reopen: %d vs %d", len(recent), len(records))
	}
	if err := reopened.AddMemory("should fail", "w-0-1"); !errors.Is(err, ErrMemorySource) {
		t.Fatal(err)
	}
	if err := reopened.AddMemory("archived finding", "a-0"); err != nil {
		t.Fatal(err)
	}
	other, err := lib.Create("Isolated", group.Members)
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := lib.OpenRoom(other, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer otherRoom.Close()
	if memory := otherRoom.Memory(); len(memory.Notes) != 0 || len(memory.Recent) != 0 {
		t.Fatal(memory)
	}
	if err := otherRoom.AddMemory("crossed room", "a-0"); !errors.Is(err, ErrMemorySource) {
		t.Fatal(err)
	}
	if err := reopened.Clear(); err != nil {
		t.Fatal(err)
	}
	if memory := reopened.Memory(); len(memory.Notes) != 0 || len(memory.Recent) != 0 {
		t.Fatal(memory)
	}
	if err := reopened.AddMemory("vanished source", "a-0"); !errors.Is(err, ErrMemorySource) {
		t.Fatal(err)
	}
}

func TestMemoryFailedWritesKeepCurrentStateAndTranscript(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.Create("Review", []Participant{{ID: "worker", Name: "Worker", Provider: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	room, err := lib.OpenRoom(group, []Member{{ID: "worker", Enabled: true, Adapter: answer("done")}})
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	if err = room.AddMemory("kept", ""); err != nil {
		t.Fatal(err)
	}
	sendRoom(t, room, "question", "worker")
	path := filepath.Join(lib.dir, group.ID, "memory.json")
	if err = os.Rename(path, path+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{room.ClearMemory, room.Clear, func() error { return room.SetMemorySharing(false) }} {
		if err = action(); err == nil {
			t.Fatal("write should fail")
		}
		memory := room.Memory()
		if !memory.Shared || len(memory.Notes) != 1 || len(memory.Recent) != 2 || len(room.Snapshot().Messages) != 2 {
			t.Fatal(memory)
		}
		if entries, err := os.ReadDir(filepath.Join(lib.dir, group.ID, "messages")); err != nil || len(entries) != 2 {
			t.Fatal(entries, err)
		}
	}
}

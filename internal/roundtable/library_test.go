package roundtable

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestLibrariesDiscoverProjectAndLegacyHistory(t *testing.T) {
	data := t.TempDir()
	for _, legacy := range []bool{false, true} {
		project := t.TempDir()
		lib, err := ProjectLibrary(data, project)
		if err != nil {
			t.Fatal(err)
		}
		group, err := lib.Create("Planning", []Participant{{"role:plan", "Planner", "local"}})
		if err != nil {
			t.Fatal(err)
		}
		if legacy {
			if err := os.Remove(filepath.Join(lib.dir, "project.json")); err != nil {
				t.Fatal(err)
			}
		}
		for _, known := range [][]string{nil, {project, project}} {
			libraries, err := Libraries(data, known)
			if err != nil {
				t.Fatal(err)
			}
			matches := slices.DeleteFunc(libraries, func(other Library) bool { return other.Key() != lib.Key() })
			if len(matches) != 1 {
				t.Fatalf("library %s listed %d times", lib.Key(), len(matches))
			}
			want := lib.Project
			if legacy && len(known) == 0 {
				want = ""
			}
			groups, err := matches[0].List()
			if matches[0].Project != want || err != nil || len(groups) != 1 || groups[0].ID != group.ID {
				t.Fatalf("history in %q (want %q): %v, %v", matches[0].Project, want, groups, err)
			}
		}
	}
}

func TestLibraryRequiresOriginalDirectoryToResume(t *testing.T) {
	data, project := t.TempDir(), t.TempDir()
	lib, err := ProjectLibrary(data, project)
	if err != nil {
		t.Fatal(err)
	}
	if !lib.Owns(project) {
		t.Fatal("project did not own its archive")
	}
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if lib.Owns(project) {
		t.Fatal("archive would resume in a regular file")
	}
}

func TestLibraryDeletesOnlyOneMeeting(t *testing.T) {
	data := t.TempDir()
	lib, err := ProjectLibrary(data, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.CreateWithChair("Delete this meeting", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := lib.CreateWithChair(group.Topic, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	room, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	if err := room.PublishConclusion("Saved conclusion"); err != nil {
		t.Fatal(err)
	}
	days, err := room.MemoryDays()
	if err != nil || len(days) != 1 {
		t.Fatalf("saved memory days: %v, %v", days, err)
	}
	if err := lib.Delete(group.ID); err != nil {
		t.Fatal(err)
	}
	if saved, err := lib.List(); err != nil || len(saved) != 1 || saved[0].ID != other.ID {
		t.Fatalf("remaining meetings: %v, %v", saved, err)
	}
	if err := room.PublishConclusion("A stale room cannot write"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale room write: %v", err)
	}
	if stale, err := lib.OpenRoom(group, nil); !errors.Is(err, os.ErrNotExist) {
		if stale != nil {
			stale.Close()
		}
		t.Fatalf("stale group open: %v", err)
	}
	for _, name := range []string{group.ID, ".deleted-" + group.ID} {
		if _, err := os.Lstat(filepath.Join(lib.dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted archive %q still exists: %v", name, err)
		}
	}
	if _, err := os.Stat(days[0].Path); err != nil {
		t.Fatalf("project memory file: %v", err)
	}
	if err := lib.Delete(group.ID); err != nil {
		t.Fatalf("retry delete: %v", err)
	}
}

func TestLibraryDeleteRejectsNonArchivePaths(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.CreateWithChair("Keep this meeting", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	link := rand.Text()
	if err := os.Symlink(filepath.Join(lib.dir, group.ID), filepath.Join(lib.dir, link)); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	for _, id := range []string{link, "../" + group.ID, "project.json", ""} {
		if err := lib.Delete(id); err == nil {
			t.Fatalf("accepted a non-archive path %q", id)
		}
	}
	if _, err := lib.Group(group.ID); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryReopensGroupsAndPagesBeyondLiveHistory(t *testing.T) {
	data, project := t.TempDir(), t.TempDir()
	lib, err := ProjectLibrary(data, project)
	if err != nil {
		t.Fatal(err)
	}
	roles := []Participant{{"role:Plan", "Plan", "local"}, {"role:Review", "Review", "local"}}
	for _, invalid := range []struct {
		topic string
		roles []Participant
	}{{" ", roles}, {"topic", nil}, {"topic", []Participant{roles[0], roles[0]}}} {
		if _, err := lib.Create(invalid.topic, invalid.roles); err == nil {
			t.Fatal("accepted invalid group", invalid)
		}
	}
	group, err := lib.Create("  Release planning  ", roles)
	if err != nil || group.Topic != "Release planning" {
		t.Fatal(group, err)
	}
	other, err := lib.Create("Release planning", roles[:1])
	if err != nil || group.ID == other.ID {
		t.Fatal(other, err)
	}
	room, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	if err := room.SetOptions(Options{ContextMessages: 3, MaxHops: 2}); err != nil {
		t.Fatal(err)
	}
	if err := room.SetSystemPrompt("Review together"); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for index := range MaxHistoryMessages + 6 {
		msg := Message{ID: rand.Text(), Time: time.Now(), Role: "user", Text: fmt.Sprint(index)}
		room.mu.Lock()
		err := room.recordLocked(msg)
		room.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, msg.ID)
	}
	room.Close()
	fresh, err := ProjectLibrary(data, project)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := fresh.List()
	if err != nil || len(groups) != 2 || groups[0].ID != group.ID || !slices.Equal(groups[0].Members, roles) || groups[0].Options.MaxHops != 2 {
		t.Fatal(groups, err)
	}
	opened, err := fresh.OpenRoom(groups[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	snap := opened.Snapshot()
	if snap.Busy || !snap.HasEarlier || len(snap.Messages) != MaxHistoryMessages || snap.SystemPrompt != "Review together" || snap.Messages[0].ID != ids[6] {
		t.Fatal("unexpected recent history")
	}
	older, more, err := opened.Earlier(snap.Messages[0], 6)
	if err != nil || more || len(older) != 6 || older[0].ID != ids[0] || older[5].ID != ids[5] {
		t.Fatal(older, more, err)
	}
	isolated, err := fresh.OpenRoom(other, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer isolated.Close()
	if len(isolated.Snapshot().Messages) != 0 {
		t.Fatal("history crossed rooms")
	}
	separate, err := ProjectLibrary(data, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if groups, err := separate.List(); err != nil || len(groups) != 0 {
		t.Fatal("history crossed projects", groups, err)
	}
	if err := opened.Clear(); err != nil {
		t.Fatal(err)
	}
	if before, _, err := opened.Earlier(snap.Messages[0], 8); err != nil || len(before) != 0 {
		t.Fatal(before, err)
	}
}

func TestLibraryWriteFailureDoesNotDispatchAndCloseKeepsAcceptedMessage(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.Create("Planning", []Participant{{"role:plan", "Planner", "local"}})
	if err != nil {
		t.Fatal(err)
	}
	adapter, calls := heldAdapter()
	room, err := lib.OpenRoom(group, []Member{{ID: "role:plan", Name: "Planner", Enabled: true, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	path := filepath.Join(lib.dir, group.ID, "messages")
	if err := os.WriteFile(path, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := room.Send("keep draft", ""); err == nil || room.Snapshot().Busy || len(room.Snapshot().Messages) != 0 {
		t.Fatal("dispatched an unsaved question", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := room.Send("saved before close", ""); err != nil {
		t.Fatal(err)
	}
	call := nextCall(t, calls)
	room.Close()
	call.answer <- Message{Text: "cancelled reply"}
	opened, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if snap := opened.Snapshot(); snap.Busy || len(snap.Messages) != 1 || snap.Messages[0].Text != "saved before close" {
		t.Fatal(snap)
	}
}

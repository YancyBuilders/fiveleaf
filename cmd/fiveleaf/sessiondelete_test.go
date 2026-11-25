package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
)

func sessionDeleteHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	return newHarness(t, true)
}

func chooseSessionHistoryItem(t *testing.T, hs *harness, row, action string) {
	t.Helper()
	if err := hs.tt.RightClick(row); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.ChooseMenuItem(action); err != nil {
		t.Fatal(err, hs.tt.Menu())
	}
	hs.settle()
}

func clickSessionHistoryButton(t *testing.T, hs *harness, label string) {
	t.Helper()
	if err := hs.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	hs.settle()
}

func TestSessionHistoryMenuTrashesOnlySelectedRecord(t *testing.T) {
	skipOnWindows(t, "the sessions' folders are POSIX paths")
	hs := sessionDeleteHarness(t)
	home := agentHome(t, hs.root)
	path := filepath.Join(home, ".claude", "projects", "p-here", "here.jsonl")
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(home, ".claude", "projects", "p-there", "there.jsonl")
	hs.a.sessions.harness, hs.a.sessions.all = agents.Claude, 1
	hs.a.showSessions()
	hs.settle()

	chooseSessionHistoryItem(t, hs, "fix here", trashLabel())
	if !slices.Equal(hs.h.trashed, []string{resolved}) {
		t.Fatalf("moved %v to Trash, want only %s", hs.h.trashed, resolved)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the selected transcript is still in history", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("the other session's transcript was changed", err)
	}
	if got := sessionIDs(hs.a.sessionRows()); got != "there" {
		t.Fatalf("remaining session IDs %q, want there", got)
	}
}

func TestSessionHistoryMenuRemovesPluginRecordAndCanUndo(t *testing.T) {
	hs := sessionDeleteHarness(t)
	dir := filepath.Join(os.Getenv("HOME"), ".pi")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, []byte(`{"id":"plugin-record","name":"Plugin archive"}`), 0600); err != nil {
		t.Fatal(err)
	}
	withPlugins(hs, map[string]map[string]string{"pi": piPlugins["pi"]})
	hs.a.sessions.harness, hs.a.sessions.all = "pi.files", 1
	hs.a.showSessions()
	hs.settle()

	chooseSessionHistoryItem(t, hs, "Plugin archive", locales.SessionsRemoveFromHistory())
	hs.a.readSessionsNow()
	hs.settle()
	if rows := hs.a.sessionRows(); len(rows) != 0 {
		t.Fatal("the plugin record returned after refreshing", rows)
	}
	if _, err := os.Stat(path); err != nil || len(hs.h.trashed) != 0 {
		t.Fatal("removing the listing changed the plugin's original file", err, hs.h.trashed)
	}

	clickSessionHistoryButton(t, hs, locales.ButtonUndo())
	if got := sessionIDs(hs.a.sessionRows()); got != "plugin-record" {
		t.Fatalf("undo restored %q, want plugin-record", got)
	}
}

func meetingDeleteRecords(t *testing.T, hs *harness) (roundtable.Library, roundtable.Group, roundtable.Group) {
	t.Helper()
	library, err := hs.a.roundtableLibrary()
	if err != nil {
		t.Fatal(err)
	}
	first, err := library.CreateWithChair("Planning draft", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := library.CreateWithChair("Planning minutes", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	return library, first, second
}

func TestSessionHistoryMenuConfirmsMeetingDeletion(t *testing.T) {
	hs := sessionDeleteHarness(t)
	library, record, neighbor := meetingDeleteRecords(t, hs)
	hs.a.sessions.harness = meetingHistorySource
	hs.a.showSessions()
	hs.settle()

	chooseSessionHistoryItem(t, hs, record.Topic, locales.SessionsDeleteMeeting())
	clickSessionHistoryButton(t, hs, locales.ButtonCancel())
	if _, err := library.Group(record.ID); err != nil || !hs.tt.HasText(record.Topic) {
		t.Fatal("cancel discarded the meeting", err)
	}

	chooseSessionHistoryItem(t, hs, record.Topic, locales.SessionsDeleteMeeting())
	clickSessionHistoryButton(t, hs, locales.ButtonDelete())
	if _, err := library.Group(record.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the selected meeting remains in the archive", err)
	}
	if got := sessionIDs(hs.a.sessionRows()); got != neighbor.ID {
		t.Fatalf("remaining meeting IDs %q, want %s", got, neighbor.ID)
	}
}

func TestSessionMeetingSidebarMenuDeletesOpenMeeting(t *testing.T) {
	hs := sessionDeleteHarness(t)
	library, record, neighbor := meetingDeleteRecords(t, hs)
	hs.a.openMeetingRecord(library, record.ID)
	hs.settle()
	window := hs.a.activeRoundtableTab()
	if window == nil || !hs.tt.HasText(locales.MeetingListTitle()) {
		t.Fatal("expected the meeting tab and sidebar", hs.tt.Texts())
	}

	chooseSessionHistoryItem(t, hs, record.Topic, locales.SessionsDeleteMeeting())
	clickSessionHistoryButton(t, hs, locales.ButtonDelete())
	if _, err := library.Group(record.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the selected meeting remains in the archive", err)
	}
	if hs.a.activeRoundtableTab() != window || !window.creating || hs.a.meeting != nil || window.meeting != nil {
		t.Fatal("deleting the open record did not reset its tab to the new meeting form")
	}
	rows := hs.a.sessions.meetingList.matches()
	if len(rows) != 1 || rows[0].id != neighbor.ID {
		t.Fatal("expected only the other meeting in the sidebar", rows)
	}
}

func TestSessionHistoryMenuClosesDeletedReadOnlyArchive(t *testing.T) {
	hs := sessionDeleteHarness(t)
	library, err := roundtable.ProjectLibrary(hs.a.roundtableDataDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := library.CreateWithChair("Archived review", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	neighbor, err := library.CreateWithChair("Archived minutes", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	legacy := library
	legacy.Project = ""
	hs.a.sessions.harness, hs.a.sessions.all, hs.a.sessions.query = meetingHistorySource, 1, "Archived"
	hs.a.showSessions()
	hs.a.openMeetingRecord(legacy, record.ID)
	hs.settle()
	if meeting := hs.a.meeting; meeting == nil || !meeting.readOnly {
		t.Fatal("expected an open read-only meeting")
	}
	clickSessionHistoryButton(t, hs, locales.MeetingBackToSessions())
	chooseSessionHistoryItem(t, hs, record.Topic, locales.SessionsDeleteMeeting())
	clickSessionHistoryButton(t, hs, locales.ButtonDelete())
	if hs.a.meetingWindow != nil || hs.a.meeting != nil || !hs.a.sessionsVisible() {
		t.Fatal("the deleted archive is still open")
	}
	if got := sessionIDs(hs.a.sessionRows()); got != neighbor.ID || hs.a.sessions.harness != meetingHistorySource || hs.a.sessions.query != "Archived" {
		t.Fatalf("meeting filter or search changed, remaining IDs %q", got)
	}
}

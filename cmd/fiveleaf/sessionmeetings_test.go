package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestSessionsMeetingHistoryFilterAndProject(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	first, second := newHarness(t, true), newHarness(t, true)
	agentHome(t, first.root)
	first.h.projects = []*app{first.a, second.a}
	create := func(owner *app, topic, name string) roundtable.Group {
		t.Helper()
		library, err := owner.roundtableLibrary()
		if err != nil {
			t.Fatal(err)
		}
		group, err := library.Create(topic, []roundtable.Participant{{ID: "role:" + name, Name: name, Provider: "local"}})
		if err != nil {
			t.Fatal(err)
		}
		return group
	}
	local := create(first.a, "Release review", "Planner")
	other := create(second.a, "Release review", "Designer")
	first.a.showSessions()
	first.settle()
	if !first.tt.HasText(local.Topic) || !first.tt.HasText("fix here") || sessionIDs(first.a.sessions.rows) != local.ID+" here" {
		t.Fatal("recent meeting and chat missing or out of order", first.tt.Texts())
	}
	choose := func(source string) {
		t.Helper()
		if err := first.tt.Click(locales.SessionsFilterAgent()); err != nil {
			t.Fatal(err)
		}
		if err := first.tt.ChooseMenuItem(source); err != nil {
			t.Fatal(err)
		}
		first.settle()
	}
	choose(locales.SessionsMeetingHistory())
	if !first.tt.HasText(locales.SessionsMeetingHistory()) || sessionIDs(first.a.sessions.rows) != local.ID || first.tt.HasText("fix here") {
		t.Fatal("meeting filter did not isolate this project's meetings", first.tt.Texts())
	}
	choose("Claude Code · 1")
	if sessionIDs(first.a.sessions.rows) != "here" {
		t.Fatal("agent filter included a meeting", first.a.sessions.rows)
	}
	choose(locales.SessionsMeetingHistory())
	if err := first.tt.Click(locales.SessionsAllProjects()); err != nil {
		t.Fatal(err)
	}
	first.settle()
	if sessionIDs(first.a.sessions.rows) != other.ID+" "+local.ID {
		t.Fatal("all projects did not list both meetings in time order", first.a.sessions.rows)
	}
	if err := first.tt.Click(locales.SessionsFind()); err != nil {
		t.Fatal(err)
	}
	first.tt.Type("RELEASE designer")
	first.settle()
	if sessionIDs(first.a.sessions.rows) != other.ID {
		t.Fatal("topic and member search did not select the other meeting", first.a.sessions.rows)
	}
	if err := first.tt.RightClick(other.Topic); err != nil {
		t.Fatal(err)
	}
	menu := first.tt.Menu()
	if !slices.Contains(menu, locales.SessionsOpenMeeting()) || slices.Contains(menu, locales.SessionsContinueWith()) || slices.Contains(menu, locales.SessionsCopyCommand()) {
		t.Fatal("incorrect actions for meeting history", menu)
	}
	if err := first.tt.ChooseMenuItem(locales.SessionsOpenMeeting()); err != nil {
		t.Fatal(err)
	}
	first.settle()
	second.settle()
	window := second.a.activeRoundtableTab()
	if first.h.showed != second.a || first.a.meeting != nil || window == nil || window.meeting == nil || window.meeting.group.ID != other.ID || window.meeting.readOnly {
		t.Fatal("meeting did not open in its owning project")
	}
	original := window.meeting
	window.draft = "next turn"
	if err := second.tt.Click(locales.RoundtableNew()); err != nil {
		t.Fatal(err)
	}
	second.settle()
	if !window.creating {
		t.Fatal("the sidebar action did not open the creation form")
	}
	first.a.resumeSession(first.a.sessions.rows[0])
	second.settle()
	if window.creating || window.meeting != original || window.draft != "next turn" {
		t.Fatal("reopening the active record restarted the meeting or lost its draft")
	}
	choose(locales.SessionsAllAgents())
	if first.a.sessions.query != "RELEASE designer" || sessionIDs(first.a.sessions.rows) != other.ID {
		t.Fatal("switching back to All Agents did not preserve the search")
	}
}

func TestSessionsMeetingHistoryWithoutOriginalProject(t *testing.T) {
	data, original := t.TempDir(), t.TempDir()
	t.Setenv("FIVELEAF_DATA_DIR", data)
	host := newHarness(t, true)
	agentHome(t, host.root)
	lib, err := roundtable.ProjectLibrary(data, original)
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.Create("Archived review", []roundtable.Participant{{ID: "role:plan", Name: "Planner", Provider: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(data, "roundtables", lib.Key(), "project.json")); err != nil {
		t.Fatal(err)
	}
	host.a.showSessions()
	host.a.sessions.harness = meetingHistorySource
	host.settle()
	if !host.tt.HasText(locales.SessionsNoMeetings()) {
		t.Fatal("unmapped meeting appeared in the current project")
	}
	if err := host.tt.Click(locales.SessionsAllProjects()); err != nil {
		t.Fatal(err)
	}
	host.settle()
	rows := host.a.sessions.rows
	if len(rows) != 1 || rows[0].ID != group.ID || rows[0].Dir != "" {
		t.Fatal("all projects lost the older archive", rows)
	}
	window := newRoundtableWindow(host.a, nil)
	host.a.meetingWindow = window
	t.Cleanup(window.finish)
	host.a.resumeSession(rows[0])
	view := ui.NewTester(meetingTestView(window), 1050, 530)
	for range 3 {
		step(view)
	}
	if !view.HasText(group.Topic) || !view.HasText("Planner") || !view.HasText(locales.SessionsMeetingReadOnly()) || view.HasText(locales.AgentSend()) {
		t.Fatal("legacy archive did not open for reading", view.Texts())
	}
	if err := window.meeting.room.Send("test", ""); !errors.Is(err, roundtable.ErrNoMembers) {
		t.Fatal("preview attached agents from the wrong project", err)
	}
}

func TestMeetingRecordSwitchIsolatesDraftAndPendingSend(t *testing.T) {
	fixture := newRoundtableHarness(t, nil)
	window := fixture.w
	first := window.meeting
	second, err := first.library.Create("Second discussion", first.group.Members)
	if err != nil {
		t.Fatal(err)
	}
	fixture.a.openMeetingRecord(first.library, second.ID)
	window.draft = "Unsent in the second meeting"
	fixture.a.openMeetingRecord(first.library, first.group.ID)
	fixture.frames()
	if err := fixture.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	fixture.tt.Type("First meeting draft")
	fixture.tt.Command("selectAll")
	fixture.tt.Type("Send only in the first meeting")
	fixture.frames()
	window.post(window.send)
	fixture.a.openMeetingRecord(first.library, second.ID)
	fixture.frames()
	if window.draft != "Unsent in the second meeting" || len(window.meeting.room.Snapshot().Messages) != 0 {
		t.Fatal("queued send crossed meetings")
	}
	if err := fixture.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	fixture.tt.Command("undo")
	fixture.frames()
	if window.draft != "Unsent in the second meeting" {
		t.Fatal("Undo crossed meetings", window.draft)
	}
}

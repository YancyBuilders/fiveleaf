package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestSessionMeetingListSwitchesRecords(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := agentRosterHarness(t)
	library, err := hs.a.roundtableLibrary()
	if err != nil {
		t.Fatal(err)
	}
	create := func(lib roundtable.Library, topic, name string) roundtable.Group {
		t.Helper()
		record, err := lib.Create(topic, []roundtable.Participant{{ID: "agent:alpha", Name: name, Provider: "alpha"}})
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	first := create(library, "海报上线", "需求分析师")
	second := create(library, "Search launch", "Quality reviewer")
	foreign, err := roundtable.ProjectLibrary(hs.a.roundtableDataDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	create(foreign, "Other project only", "Planner")
	st := &hs.a.sessions
	st.query, st.all, st.harness, st.rowKey = "plain chat query", 1, agents.Harness("codex"), "chosen-chat"
	hs.a.openRoundtable()
	hs.settle()
	if !hs.tt.HasText(locales.MeetingListTitle()) || !hs.tt.HasText(first.Topic) || !hs.tt.HasText(second.Topic) || hs.tt.HasText("Other project only") || hs.tt.HasText(locales.SessionsStartChat()) {
		t.Fatalf("wrong sidebar context: %v", hs.tt.Texts())
	}
	click := func(label string) {
		t.Helper()
		if err := hs.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	window := hs.a.activeRoundtableTab()
	click(first.Topic)
	if window.creating || window.meeting == nil || window.meeting.group.ID != first.ID {
		t.Fatal("a single click did not open the saved group")
	}
	click(locales.MeetingListFind())
	hs.tt.Type("LAUNCH quality")
	hs.settle()
	if rows := st.meetingList.matches(); len(rows) != 1 || rows[0].id != second.ID {
		t.Fatal("multi-word topic and member search returned unrelated meetings", rows)
	}
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	if window.meeting.group.ID != second.ID {
		t.Fatal("Enter did not open the filtered record")
	}
	click(locales.RoundtableNew())
	if !window.creating || st.meetingList.query != "" || len(st.meetingList.matches()) != 2 {
		t.Fatal("new meeting did not return to the creation form and reveal the list")
	}
	if st.query != "plain chat query" || st.all != 1 || st.harness != "codex" || st.rowKey != "chosen-chat" {
		t.Fatal("meeting browsing changed ordinary session filters or selection")
	}
}

func TestSessionMeetingListBackToSessions(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := agentRosterHarness(t)
	project, sessions := hs.a, &hs.a.sessions
	project.settings.SidebarRight = false
	agentHome(t, hs.root)
	sessions.query, sessions.all, sessions.harness = "fix there", 1, agents.Claude
	project.readSessionsNow()
	project.showSessions()
	hs.settle()
	selection := sessions.rowKey
	if selection == "" || !hs.tt.HasText("fix there") {
		t.Fatal("expected a selected session in the filtered list")
	}
	library, err := project.roundtableLibrary()
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.Create("Planning", []roundtable.Participant{{ID: "agent:alpha", Name: "Alpha", Provider: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	click := func(label string) {
		t.Helper()
		if err := hs.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	click(locales.RoundtableOpen())
	window, meeting := project.activeRoundtableTab(), project.meeting
	if window == nil || meeting == nil || meeting.group.ID != group.ID {
		t.Fatal("expected the project meeting to open")
	}
	hs.shot("meeting-list-back")
	for _, speaker := range meeting.speakers {
		if speaker.ID == "agent:alpha" {
			speaker.adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
				<-ctx.Done()
				return roundtable.Message{}, ctx.Err()
			})
		}
	}
	if err := meeting.room.Discuss(group.Topic, "Review the plan", "agent:alpha"); err != nil {
		t.Fatal(err)
	}
	defer meeting.room.Stop()
	window.draft, window.target = "Follow up", "agent:alpha"
	project.openRoundtable()
	project.showSessions()
	hs.settle()
	if !hs.tt.Focused(locales.MeetingListFind()) {
		t.Fatal("Sessions activity should focus the visible meeting search")
	}
	hs.tt.Type("Alpha")
	hs.settle()
	click(locales.MeetingBackToSessions())
	if !hs.tt.Focused(locales.SessionsFind()) || !hs.tt.HasText("fix there") || hs.tt.HasText(locales.MeetingListFind()) {
		t.Fatal("Back should return to the filtered Sessions list and focus its search")
	}
	if sessions.query != "fix there" || sessions.all != 1 || sessions.harness != agents.Claude || sessions.rowKey != selection {
		t.Fatal("returning changed the session search, scope, agent filter or selected row")
	}
	checkMeeting := func() {
		t.Helper()
		if project.activeRoundtableTab() != window || project.meeting != meeting || window.meeting != meeting || window.draft != "Follow up" || window.target != "agent:alpha" || !meeting.room.Busy() {
			t.Fatal("sidebar navigation changed the active meeting, directed draft or running turn")
		}
	}
	checkMeeting()
	click(locales.AgentDraft())
	click(locales.CmdViewSessions())
	if !hs.tt.Focused(locales.SessionsFind()) {
		t.Fatal("Sessions activity should focus the session search after returning")
	}
	click(locales.RoundtableOpen())
	if !hs.tt.HasText(locales.MeetingListFind()) || sessions.meetingList.query != "Alpha" || len(sessions.meetingList.matches()) != 1 {
		t.Fatal("Roundtable should reopen the filtered meeting list")
	}
	checkMeeting()
}

func TestSessionMeetingListReactivatesWithTab(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := newHarness(t, true)
	hs.a.openFile(hs.path("main.go"), true)
	hs.a.openRoundtable()
	hs.a.showSessions()
	hs.settle()
	window := hs.a.activeRoundtableTab()
	if window == nil || !window.creating {
		t.Fatal("expected a new meeting invitation")
	}
	if !hs.tt.Focused(locales.MeetingListFind()) {
		t.Fatal("opening Sessions should focus the meeting search even while the invitation opens")
	}
	window.topic, window.brief = "Draft topic", "Draft brief"
	if err := hs.tt.Click(locales.MeetingBackToSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.activeRoundtableTab() != window || !hs.tt.HasText(locales.RoundtableOpen()) {
		t.Fatal("Back should leave the invitation tab open beside Sessions")
	}
	if err := hs.tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.activeRoundtableTab() != nil {
		t.Fatal("the document tab did not activate")
	}
	if err := hs.tt.Click(window.tab.name()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.activeRoundtableTab() != window || !hs.tt.HasText(locales.MeetingListTitle()) || !window.creating || window.topic != "Draft topic" || window.brief != "Draft brief" {
		t.Fatal("reactivating the tab should restore the meeting sidebar and preserve the draft invitation")
	}
}

func TestSessionMeetingListErrorAndRetry(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FIVELEAF_DATA_DIR", data)
	hs := newHarness(t, true)
	hs.a.openRoundtable()
	hs.settle()
	if !hs.tt.HasText(locales.RoundtableNoGroups()) {
		t.Fatal("an empty project has no empty state", hs.tt.Texts())
	}
	broken := filepath.Join(data, "roundtables")
	if err := os.WriteFile(broken, []byte("test fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	hs.a.readSessionsNow()
	hs.settle()
	if hs.a.sessions.meetingList.err == nil || !hs.tt.HasText(locales.Refresh()) {
		t.Fatal("could not recover from an unreadable library", hs.tt.Texts())
	}
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.Click(locales.Refresh()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.sessions.meetingList.err != nil || !hs.tt.HasText(locales.RoundtableNoGroups()) {
		t.Fatal("refresh did not recover after the library became readable")
	}
}

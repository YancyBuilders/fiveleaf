package main

import (
	"context"
	"errors"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
)

func TestRoundtableWorkspaceTabLifecycle(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	h := agentRosterHarness(t)
	project := h.a
	project.openFile(h.path("main.go"), true)
	h.settle()
	previousTab := project.activeTab()
	chat := project.agent
	project.openRoundtable()
	h.settle()
	meetingTab := project.activeTab()
	page := project.activeRoundtableTab()
	if page == nil || !page.creating || meetingTab.name() != locales.RoundtableNew() || !project.sessionsVisible() || project.agent != chat {
		t.Fatal("new meeting must open beside the document with its project sidebar")
	}
	page.topic = "Draft topic"
	project.openRoundtable()
	if project.activeTab() != meetingTab || page.topic != "Draft topic" {
		t.Fatal("reopening reset or duplicated the meeting tab")
	}
	meeting := testAgentMeeting(t, project)
	project.showRoundtable(meeting)
	if project.activeTab() != meetingTab || meetingTab.name() != meeting.group.Topic {
		t.Fatal("a meeting must replace the invitation form in the same tab")
	}
	started := make(chan struct{})
	for _, speaker := range meeting.speakers {
		if speaker.ID == "agent:alpha" {
			speaker.adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
				close(started)
				<-ctx.Done()
				return roundtable.Message{}, ctx.Err()
			})
		}
	}
	if err := meeting.room.Discuss(meeting.group.Topic, "Review this", "agent:alpha"); err != nil {
		t.Fatal(err)
	}
	h.until("live task", func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	})
	page.draft = "Keep this follow-up"
	col, index := project.locate(previousTab)
	project.activate(col, index, true)
	var delivered bool
	page.post(func() { delivered = true })
	h.settle()
	if !delivered || !meeting.room.Snapshot().Busy || project.activeRoundtableTab() != nil || project.activeTab() != previousTab {
		t.Fatal("a hidden meeting must process updates without taking over the editor")
	}
	project.openRoundtable()
	if project.activeTab() != meetingTab || project.meeting != meeting || page.draft != "Keep this follow-up" || project.canDetachTab(meetingTab) || carriable(meetingTab) {
		t.Fatal("opening an existing tab must keep its meeting bound to the original project")
	}
	next := project.splitPane(col, splitRight)
	project.moveTab(meetingTab, next, 0)
	if project.activeRoundtableTab() != page || project.meeting != meeting || !meeting.room.Snapshot().Busy {
		t.Fatal("moving a tab inside its project must preserve the active turn")
	}
	project.closeTab(meetingTab, nil, nil)
	if project.meetingWindow != nil || project.meeting != nil || project.activeTab() != previousTab || project.agent != chat {
		t.Fatal("closing the meeting must restore the previous document and preserve its chat")
	}
	if err := meeting.room.Send("closed", ""); !errors.Is(err, roundtable.ErrClosed) {
		t.Fatal(err)
	}
}

func TestRoundtableOpensRecentProjectMeeting(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	h := agentRosterHarness(t)
	project := h.a
	lib, err := project.roundtableLibrary()
	if err != nil {
		t.Fatal(err)
	}
	invited := []roundtable.Participant{{ID: "agent:alpha", Name: "Alpha", Provider: "alpha"}}
	if _, err = lib.Create("Old review", invited); err != nil {
		t.Fatal(err)
	}
	recent, err := lib.Create("Latest topic", invited)
	if err != nil {
		t.Fatal(err)
	}
	project.openRoundtable()
	h.settle()
	if w := project.activeRoundtableTab(); w == nil || w.creating || w.meeting.group.ID != recent.ID || w.meeting.room.Snapshot().Busy {
		t.Fatal("initial tab must resume the latest project meeting without sending a prompt")
	}
	w := project.activeRoundtableTab()
	project.openFolder(t.TempDir())
	if col, _ := project.locate(w.tab); col != nil || project.meetingWindow != nil || !w.closed {
		t.Fatal("opening another project retained a previous project's meeting")
	}
}

func TestRoundtableSwitchKeepsRunningMeetingAndDirectedDraft(t *testing.T) {
	h := agentRosterHarness(t)
	project := h.a
	first := testAgentMeeting(t, project)
	project.showRoundtable(first)
	page := project.activeRoundtableTab()
	page.draft, page.target = "Ask privately", "agent:alpha"
	started, resume := make(chan struct{}), make(chan struct{})
	for _, speaker := range first.speakers {
		if speaker.ID == "agent:alpha" {
			speaker.adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
				close(started)
				select {
				case <-ctx.Done():
					return roundtable.Message{}, ctx.Err()
				case <-resume:
					return roundtable.Message{Text: "Archived answer"}, nil
				}
			})
		}
	}
	if err := first.room.Discuss(first.group.Topic, "Check the record", "agent:alpha"); err != nil {
		t.Fatal(err)
	}
	h.until("first task", func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	})
	second, err := first.library.Create("Another discussion", first.group.Members)
	if err != nil {
		t.Fatal(err)
	}
	project.openMeetingRecord(first.library, second.ID)
	if project.meeting == first || !first.room.Busy() || !project.ownsMeeting(first) || page.draft != "" {
		t.Fatal("switching to another meeting interrupted or mixed the rooms")
	}
	project.openMeetingRecord(first.library, first.group.ID)
	if project.meeting != first || page.draft != "Ask privately" || page.target != "agent:alpha" {
		t.Fatal("returning to a running meeting lost its identity or directed draft")
	}
	project.openMeetingRecord(first.library, second.ID)
	close(resume)
	h.until("background completion", func() bool { return !project.ownsMeeting(first) })
	project.openMeetingRecord(first.library, first.group.ID)
	got := project.meeting.room.Snapshot()
	if got.Busy || len(got.Messages) != 2 || got.Messages[1].Text != "Archived answer" {
		t.Fatal(got)
	}
}

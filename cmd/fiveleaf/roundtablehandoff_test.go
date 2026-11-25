package main

import (
	"context"
	"strings"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestRoundtableConclusionHandoffUsesSelectedRoundAndRole(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "handoff")
	a := hs.a
	rolePrompt := "Use the configured implementation role."
	a.settings.AgentRolePresets = map[string]settings.AgentRoleOverride{
		"implement": {Agent: "backend", Model: "model-b", Prompt: &rolePrompt},
	}
	originalChat := a.agent
	originalChat.draft = "Unrelated private draft"
	meeting := testAgentMeeting(t, a)
	assigned := false
	for _, speaker := range meeting.speakers {
		if speaker.ID != roundtable.ModeratorID {
			speaker.adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
				roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "files", Status: roundtable.Completed,
					Detail: "current.go:42 raw output secret", Note: "current_test.go:17 check details"})
				roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "failed", Status: roundtable.Failed, Detail: "failed.go:10 unchecked"})
				return roundtable.Message{Text: "The fallback has a dedicated check."}, nil
			})
			continue
		}
		speaker.adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, messages []roundtable.Message) (roundtable.Message, error) {
			if messages[len(messages)-1].Text == "Prior round objective" {
				roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "old", Status: roundtable.Completed, Detail: "old.go:88 stale evidence"})
				return roundtable.Message{Text: "Prior round conclusion"}, nil
			}
			if !assigned {
				assigned = true
				return roundtable.Message{Text: "@agent:backend Check the fallback"}, nil
			}
			return roundtable.Message{Text: "Keep the redirect and test the fallback."}, nil
		})
	}
	if err := meeting.room.AddMemory("Unrelated pinned note", ""); err != nil {
		t.Fatal(err)
	}
	for _, request := range []string{"Prior round objective", "Check the current redirect behavior"} {
		if err := meeting.room.Discuss(meeting.group.Topic, request, ""); err != nil {
			t.Fatal(err)
		}
		hs.until("saved discussion", func() bool { return !meeting.room.Busy() })
		if err := meeting.room.Snapshot().Error; err != "" {
			t.Fatal(err)
		}
	}
	snapshot := meeting.room.Snapshot()
	conclusion := snapshot.Messages[len(snapshot.Messages)-1]
	if !meetingFlowConclusion(conclusion) {
		t.Fatal("missing saved conclusion")
	}
	a.showRoundtable(meeting)
	window := a.meetingWindow
	window.page = []roundtable.Message{conclusion}
	window.pageCursors = []roundtable.Message{conclusion}
	window.pageGraph = roundtable.BuildTaskGraph(roundtable.Snapshot{Messages: window.page})
	hs.settle()
	if err := hs.tt.Click(locales.RoundtableContinueWith()); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.ChooseMenuItem(locales.AgentRoles(), locales.AgentRoleImplementer()); err != nil {
		t.Fatal(err)
	}
	hs.until("archived conclusion continuation", func() bool { return a.agent != originalChat && !a.agent.connecting })
	chat := a.agent
	if chat.continuation == nil || chat.continuation.truncated || chat.continuation.source.ID != meeting.group.ID+"/"+conclusion.ID || !meeting.library.Owns(chat.continuation.source.Dir) {
		t.Fatalf("incorrect meeting source: %+v", chat.continuation)
	}
	if a.activeTab().tool.chat != chat || a.meetingWindow != window || originalChat.draft != "Unrelated private draft" || chat.draft != locales.RoundtableContinueDraft() || chat.err != "" {
		t.Fatal("opening the conclusion failed to create a separate chat and draft")
	}
	assertContinuationIdle(t, chat)
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			t.Fatal("opening a continuation sent a prompt")
		}
	}
	chat.draft = "Check only the fallback behavior."
	a.sendAgent()
	hs.until("follow-up reply", func() bool { return !chat.busy && !chat.connecting })
	var sent roleRuntimeEvent
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			sent = event
		}
	}
	if chat.err != "" || chat.turn != 1 || sent.Model != "model-b" {
		t.Fatalf("wrong follow-up state: error=%q turn=%d model=%q", chat.err, chat.turn, sent.Model)
	}
	for _, want := range []string{rolePrompt, "Check the current redirect behavior", conclusion.Text, "current.go:42", "current_test.go:17", chat.continuation.source.ID, "Check only the fallback behavior."} {
		if strings.Count(sent.Text, want) != 1 {
			t.Errorf("expected one occurrence of %q in prompt: %s", want, sent.Text)
		}
	}
	for _, unwanted := range []string{"Prior round", "old.go:88", "failed.go:10", "Unrelated", "raw output secret", "check details"} {
		if strings.Contains(sent.Text, unwanted) {
			t.Errorf("unrelated content %q included in prompt", unwanted)
		}
	}
}

func TestRoundtableHandoffOfHumanConclusion(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.settings.AgentServers = map[string]settings.AgentServer{"local": roundtableTestServer(t, "Local Agent", "cli")}
	library, err := roundtable.ProjectLibrary(t.TempDir(), hs.root)
	if err != nil {
		t.Fatal(err)
	}
	member := roundtable.Participant{ID: "agent:local", Name: "Local Agent", Provider: "local"}
	group, err := library.CreateWithChair("Login review", []roundtable.Participant{member}, true)
	if err != nil {
		t.Fatal(err)
	}
	meeting, err := a.startMeeting(library, group)
	if err != nil {
		t.Fatal(err)
	}
	const remark = "We need to decide the rollback conditions."
	meeting.speakers[0].adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, messages []roundtable.Message) (roundtable.Message, error) {
		path := "legacy.go:5"
		if messages[len(messages)-1].Text == remark {
			path = "rollback.go:22"
		}
		roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "read", Status: roundtable.Completed, Detail: path + " read details"})
		return roundtable.Message{Text: "Review complete."}, nil
	})
	for _, request := range []string{"Review an unrelated change.", remark} {
		if err := meeting.room.Discuss(group.Topic, request, member.ID); err != nil {
			t.Fatal(err)
		}
		hs.until("saved direct reply", func() bool { return !meeting.room.Busy() })
		if err := meeting.room.Snapshot().Error; err != "" {
			t.Fatal(err)
		}
	}
	if err := meeting.room.PublishConclusion("Roll back after two attempts."); err != nil {
		t.Fatal(err)
	}
	a.showRoundtable(meeting)
	hs.settle()
	if err := hs.tt.Click(locales.RoundtableContinueWith()); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.ChooseMenuItem("Local Agent"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	chat := a.agent
	if chat.continuation == nil || chat.provider != "local" || len(chat.messages) != 2 || chat.messages[0].text != remark || !strings.HasPrefix(chat.messages[1].text, "Roll back after two attempts.") {
		t.Fatal("human conclusion did not carry its preceding remark and decision")
	}
	handoffContext := chat.continuation.context
	if !strings.Contains(handoffContext, "rollback.go:22") || strings.Contains(handoffContext, "legacy.go:5") || strings.Contains(handoffContext, "read details") || strings.Contains(handoffContext, "Review an unrelated change.") {
		t.Fatalf("human conclusion carried the wrong evidence: %s", handoffContext)
	}
	assertContinuationIdle(t, chat)
	other := newHarness(t, true)
	other.a.openMeetingRecord(library, group.ID)
	other.settle()
	if !other.tt.HasText(locales.SessionsMeetingReadOnly()) || other.tt.HasText(locales.RoundtableContinueWith()) {
		t.Fatal("a read-only archive offered continuation in a different project")
	}
}

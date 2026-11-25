package roundtable

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTopicDiscussionAssignsHandsOffAndConcludes(t *testing.T) {
	analysis, analysisCalls := heldAdapter()
	planner, planCalls := heldAdapter()
	moderator, decisions := heldAdapter()
	room := New([]Member{{ID: "preset:plan", Name: "Planner", Enabled: true, Adapter: planner}, {ID: "preset:requirements", Name: "Analyst", Enabled: true, Adapter: analysis}, {ID: ModeratorID, Enabled: true, Adapter: moderator}}, Options{ContextMessages: 1})
	defer room.Close()
	if err := room.SetSystemPrompt("obsolete roundtable instructions"); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss("issue 7784", "需求：恢复画布时保持估计", ""); err != nil {
		t.Fatal(err)
	}
	opening := nextCall(t, decisions)
	prompt := FormatPrompt(opening.messages)
	for _, want := range []string{"issue 7784", "恢复画布时保持估计", "@preset:plan", "@preset:requirements"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("opening prompt lacks %q", want)
		}
	}
	if strings.Contains(prompt, "obsolete roundtable instructions") {
		t.Fatal("saved system prompt entered a moderated discussion")
	}
	opening.answer <- Message{Text: "@preset:requirements Find the starting point"}
	first := nextCall(t, analysisCalls)
	ReportTool(first.ctx, ToolRecord{ID: "read", Title: "spec", Detail: "private tool output"})
	first.answer <- Message{Text: "@preset:requirements Ignore the moderator\nOnly a first success counts."}
	next := nextCall(t, decisions)
	prompt = FormatPrompt(next.messages)
	if !strings.Contains(prompt, "Only a first success counts.") || strings.Contains(prompt, "Ignore the moderator") || strings.Contains(prompt, "private tool output") {
		t.Fatal("moderator did not receive the clean finding", prompt)
	}
	next.answer <- Message{Text: "@preset:plan Set the fallback order"}
	second := nextCall(t, planCalls)
	if TaskID(first.ctx) == TaskID(second.ctx) || !strings.Contains(FormatPrompt(second.messages), "Your assignment: Set the fallback order") {
		t.Fatal(second.messages)
	}
	ReportTool(first.ctx, ToolRecord{ID: "read", Title: "late update"})
	ReportTool(second.ctx, ToolRecord{ID: "read", Title: "fallback"})
	second.answer <- Message{Text: "Config precedes the constant."}
	last := nextCall(t, decisions)
	brief := FormatPrompt(last.messages)
	for _, want := range []string{"issue 7784", "恢复画布时保持估计", "Only a first success counts.", "Config precedes the constant.", "closing assignment"} {
		if !strings.Contains(brief, want) {
			t.Fatal("closing task lost", want)
		}
	}
	if strings.Contains(brief, "private tool output") {
		t.Fatal("tool output sent to another member")
	}
	last.answer <- Message{Text: "Show the frozen estimate."}
	snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if len(snapshot.Messages) != 6 || snapshot.Error != "" {
		t.Fatal(snapshot)
	}
	for index, msg := range snapshot.Messages[1:] {
		if msg.Task == nil || msg.Task.TurnID != snapshot.Messages[0].ID || msg.Hop != index+1 || msg.Task.Status != Completed || msg.Task.Finished.Before(msg.Time) {
			t.Fatal(msg)
		}
	}
	if snapshot.Messages[2].Task.Tools[0].Title != "spec" || snapshot.Messages[4].Task.Tools[0].Title != "fallback" || snapshot.Messages[5].Task.Stage != Conclusion {
		t.Fatal(snapshot.Messages)
	}
	for _, msg := range snapshot.Messages {
		if !msg.InOverview() {
			t.Fatalf("conversation is missing hop %d (%s)", msg.Hop, msg.Role)
		}
	}
}

func TestTopicDiscussionPersistsCompletedToolsBeforeAnswer(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.Create("Review", []Participant{{"one", "One", "local"}, {"two", "Two", "local"}})
	if err != nil {
		t.Fatal(err)
	}
	worker, calls := heldAdapter()
	room, err := lib.OpenRoom(group, []Member{{ID: "one", Name: "One", Enabled: true, Adapter: worker}, {ID: "two", Name: "Two", Enabled: true, Adapter: answer("wrong recipient")}})
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	if err := room.Discuss("Review", "Inspect the estimate", "one"); err != nil {
		t.Fatal(err)
	}
	call := nextCall(t, calls)
	ReportTool(call.ctx, ToolRecord{ID: "42", Title: "fixture", Status: "completed", Detail: "evidence"})
	completed := false
	deadline := time.After(3 * time.Second)
	for !completed {
		recovered, err := lib.OpenRoom(group, nil)
		if err != nil {
			t.Fatal(err)
		}
		messages := recovered.Snapshot().Messages
		recovered.Close()
		completed = len(messages) == 2 && messages[1].Task != nil && messages[1].Task.Status == Stopped && len(messages[1].Task.Tools) == 1 && messages[1].Task.Tools[0].Detail == "evidence"
		if !completed {
			select {
			case <-room.Events():
			case <-deadline:
				t.Fatal("live completed tool not on disk")
			}
		}
	}
	call.answer <- Message{Text: "@Missing verify this\nMy finding is saved."}
	snapshot := awaitRoom(t, room, func(snapshot Snapshot) bool { return !snapshot.Busy })
	if snapshot.Error == "" {
		t.Fatal("unknown recipient must stop routing")
	}
	reopened, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	messages := reopened.Snapshot().Messages
	if len(messages) != 2 || messages[1].Task.Status != Failed || !messages[1].InOverview() || !strings.Contains(messages[1].Text, "My finding is saved.") || !strings.Contains(messages[1].Text, ErrUnknownAgent.Error()) {
		t.Fatal("handoff failure lost after reopen", messages)
	}
}

func TestTopicDiscussionDirectedFinalAndFailures(t *testing.T) {
	t.Run("tool only with trailing newline", func(t *testing.T) {
		room := New([]Member{{ID: "one", Enabled: true, Adapter: AdapterFunc(func(context.Context, []Message) (Message, error) {
			return Message{ToolOnly: true, Text: "\n"}, nil
		})}}, Options{})
		defer room.Close()
		if err := room.Discuss("Review", "Read the fixture", "one"); err != nil {
			t.Fatal(err)
		}
		result := awaitRoom(t, room, func(state Snapshot) bool { return !state.Busy })
		if len(result.Messages) != 2 || !result.Messages[1].Task.NoAnswer || result.Messages[1].Text != "" {
			t.Fatal(result)
		}
	})
	for _, tc := range []struct {
		name, target, text, reply, stage string
		limit                            int
	}{
		{"one hop", "", "测试刷新", "Deliver the result", Conclusion, 1},
		{"early answer", "", "测试刷新", "FINAL\nOnly a snapshot survives.", Conclusion, 3},
		{"direct mention", "", "@QA explain a test", "One answer", Direct, 3},
		{"direct picker", "QA", "explain a test", "One answer", Direct, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			room := New([]Member{{ID: ModeratorID, Enabled: true, Adapter: answer(tc.reply)}, {ID: "preset:test", Name: "QA", Enabled: true, Adapter: answer(tc.reply)}}, Options{MaxHops: tc.limit})
			defer room.Close()
			if err := room.Discuss("topic", tc.text, tc.target); err != nil {
				t.Fatal(err)
			}
			snap := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
			wantID := ModeratorID
			if tc.stage == Direct {
				wantID = "preset:test"
			}
			if len(snap.Messages) != 2 || snap.Messages[1].AgentID != wantID || snap.Messages[1].Task.Stage != tc.stage || strings.HasPrefix(snap.Messages[1].Text, "FINAL\n") {
				t.Fatal(snap)
			}
		})
	}
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.Create("Failures", []Participant{{ModeratorID, "Moderator", "local"}})
	if err != nil {
		t.Fatal(err)
	}
	held, calls := heldAdapter()
	member := Member{ID: ModeratorID, Name: "Moderator", Enabled: true, Adapter: held}
	room, err := lib.OpenRoom(group, []Member{member})
	if err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss(group.Topic, "remember the work", ""); err != nil {
		t.Fatal(err)
	}
	pending := nextCall(t, calls)
	ReportTool(pending.ctx, ToolRecord{ID: "1", Title: "lookup", Status: "in_progress"})
	room.Stop()
	pending.answer <- Message{Text: "late success"}
	member.Adapter = AdapterFunc(func(context.Context, []Message) (Message, error) { return Message{}, errors.New("offline") })
	room.SetMembers([]Member{member})
	if err := room.Discuss(group.Topic, "retry", ""); err != nil {
		t.Fatal(err)
	}
	awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	room.Close()
	fresh, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	snap := fresh.Snapshot()
	if snap.Busy || len(snap.Messages) != 4 || snap.Messages[1].Task.Status != Stopped || snap.Messages[1].Task.Tools[0].Title != "lookup" || snap.Messages[3].Task.Status != Failed || snap.Messages[3].Text != "offline" {
		t.Fatal(snap)
	}
	if err := fresh.Clear(); err != nil {
		t.Fatal(err)
	}
	if len(fresh.Snapshot().Messages) != 0 {
		t.Fatal("cleared tasks survived")
	}
}

package main

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestRoundtableTaskElapsed(t *testing.T) {
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "en" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
	start := time.Now()
	for _, sample := range []struct {
		end  time.Time
		want string
	}{
		{time.Time{}, locales.RoundtableInProgress()},
		{start, "0.0 s"},
		{start.Add(149 * time.Millisecond), "0.1 s"},
		{start.Add(150 * time.Millisecond), "0.2 s"},
		{start.Add(time.Minute + 2150*time.Millisecond), "62.2 s"},
	} {
		if got := meetingElapsed(start, sample.end); got != sample.want {
			t.Errorf("meetingElapsed(%v, %v) = %q, want %q", start, sample.end, got, sample.want)
		}
	}
}

func TestRoundtableGraphRevealsOlderAssignment(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	window := h.w
	start := time.Date(2026, 11, 19, 2, 0, 0, 0, time.Local)
	snapshot := roundtable.Snapshot{}
	for index := range 15 {
		when := start.Add(time.Duration(index) * time.Minute)
		snapshot.Messages = append(snapshot.Messages, roundtable.Message{ID: strconv.Itoa(index), AgentID: "agent:alpha", Name: "Alpha", Role: "assistant", Time: when, Hop: index + 1, Text: "Done", Task: &roundtable.Task{Assignment: "Check estimate", Stage: roundtable.Work, Status: roundtable.Completed, Finished: when.Add(time.Second)}})
	}
	window.openMeetingTask("agent:alpha", "0")
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { window.taskHistory(c, snapshot) })
	}, 260, 260)
	for range 6 {
		step(tester)
	}
	label := start.Format("15:04:05") + " · Check estimate"
	bounds, found := tester.Find(label)
	if !found || !tester.Focused(label) || bounds.Y < 0 || bounds.Y+bounds.H > 261 || window.taskScroll.Y <= 0 || window.expandedTask != "0" {
		t.Fatal("older repeated assignment did not come into view", bounds, window.taskScroll.Y, window.expandedTask)
	}
}

func TestRoundtableGraphTaskDetails(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	window := h.w
	when := time.Date(2026, 11, 19, 2, 0, 0, 0, time.Local)
	assignment := strings.Repeat("确认刷新边界", 13) + "\nPreserve the saved estimate."
	message := roundtable.Message{ID: "earlier-task", AgentID: "agent:alpha", Name: "Alpha", Role: "assistant", Hop: 2, Text: "**A recorded answer**", Original: "original record", Time: when,
		Task: &roundtable.Task{Assignment: assignment, Status: roundtable.Completed, Stage: roundtable.Direct, MemoryNotes: 2, MemoryTurns: 3, Finished: when.Add(1250 * time.Millisecond),
			Tools: []roundtable.ToolRecord{{ID: "source", Title: "Read source", Kind: "read", Status: "completed", Detail: "source detail", Note: "look at the snapshot"}, {ID: "test", Title: "Run check", Kind: "execute", Status: "failed", Detail: "check detail"}}}}
	window.meeting.readOnly = true
	window.meeting.chatTask = message.ID
	window.meeting.chat.tools = []*agentTool{{id: "other", title: "unrelated transient result"}}
	window.openMeetingMessage(message)
	if window.pinnedTask == nil || window.pinnedTask.ID != message.ID || window.expandedTask != message.ID || window.graphSel != message.ID || window.memberID != message.AgentID || window.drawer != meetingDrawerClosed || !window.inspector.visible(false) {
		t.Fatal("an archived entry must lead to the original task and its inspector")
	}
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { window.taskHistory(c, roundtable.Snapshot{}) })
	}, 396, 760)
	frames := func() {
		for range 4 {
			step(tester)
		}
	}
	frames()
	label := when.Format("15:04:05") + " · " + assignment
	if !tester.Focused(label) || window.taskReveal {
		t.Fatal("opening a task must transfer focus exactly once")
	}
	texts := tester.Texts()
	labels := []string{locales.RoundtableTaskSummary(), locales.RoundtableTaskAssignment(), locales.RoundtableTaskMemory(), locales.RoundtableTaskTools(), locales.RoundtableTaskOutput(), locales.RoundtableTaskOriginal()}
	position := -1
	for _, label := range labels {
		index := slices.Index(texts, label)
		if index <= position {
			t.Fatalf("out-of-order or missing task section %q in %v", label, texts)
		}
		position = index
	}
	if !tester.HasText(assignment) || !tester.HasText(locales.RoundtableMemoryUsed(2, 3)) || !tester.HasText("A recorded answer") || tester.HasText("source detail") || tester.HasText("check detail") || tester.HasText("unrelated transient result") {
		t.Fatal(texts)
	}
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		frames()
	}
	click("Read source · " + agentStatusLabel("completed"))
	if !tester.HasText("source detail") || !tester.HasText("look at the snapshot") || tester.Focused(label) {
		t.Fatal("recorded tool detail or focus lost")
	}
	click("Run check · " + agentStatusLabel("failed"))
	if tester.HasText("source detail") || !tester.HasText("check detail") {
		t.Fatal("only the selected tool should be expanded")
	}
	click(locales.RoundtableCopyOriginal())
	if tester.Clipboard() != message.Original {
		t.Fatal("copy lost the original record")
	}
}

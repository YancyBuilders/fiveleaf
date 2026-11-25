package main

import (
	"context"
	"slices"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestRoundtableGraphFirstFrameInSplit(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	room := h.w.meeting.room
	const task = "Check the rendered task before the next frame"
	if err := room.Discuss("Review", task, "agent:beta"); err != nil {
		t.Fatal(err)
	}
	h.until(t, "task result", func() bool { return !room.Busy() })
	label := locales.RoundtableGraphNodeTarget("Beta", task, 1)
	var tester *ui.Tester
	var frames []ui.Rect
	view := meetingTestView(h.w)
	tester = ui.NewTester(func(c *ui.Context) {
		if tester != nil {
			if rect, found := tester.Find(label); found {
				frames = append(frames, rect)
			}
		}
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Width(380).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() { view(c) })
		})
	}, 1110, 820)
	if err := tester.Click(locales.RoundtableGraphView()); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tester.Frame()
	}
	if len(frames) < 2 || frames[0].W < 80 || frames[0].H < 16 {
		t.Fatalf("first visible node: %+v", frames)
	}
	for _, rect := range frames[1:] {
		if rect != frames[0] {
			t.Fatalf("node changed after appearing: %+v", frames)
		}
	}
	if err := tester.Click(locales.RoundtableMemberPanel()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if h.w.graphScroll.MaxX > 1 {
		t.Fatalf("graph uses width outside its pane: %v", h.w.graphScroll.MaxX)
	}
}

func TestRoundtableGraphChatAndMatrixOpenSameTask(t *testing.T) {
	replies := []string{"@agent:beta Inspect the second sample", "@agent:alpha Compare the results", "Both samples match."}
	h := newRoundtableHarness(t, roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
		reply := replies[0]
		replies = replies[1:]
		return roundtable.Message{Text: reply}, nil
	}))
	h.tt.SetSize(1280, 860)
	room := h.w.meeting.room
	if err := room.Discuss("Review", "Compare the samples", "agent:alpha"); err != nil {
		t.Fatal(err)
	}
	h.until(t, "sample results", func() bool { return !room.Busy() })
	messages := room.Snapshot().Messages
	if len(messages) != 4 || messages[2].AgentID != "agent:beta" {
		t.Fatalf("expected one turn with three replies: %+v", messages)
	}
	task := messages[2]
	h.w.draft = "Private follow-up"
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	click(task.Name)
	if h.w.expandedTask != task.ID || h.w.memberID != task.AgentID || !h.tt.HasText(locales.RoundtableTaskOutput()) {
		t.Fatalf("conversation did not open the original task history: %v", h.tt.Texts())
	}
	click(locales.RoundtableGraphView())
	if h.w.view != roundtableViewGraph || h.tt.HasText(locales.AgentDraft()) || h.tt.HasText(locales.RoundtableMembersHeading()) || !h.tt.HasText(locales.RoundtableGraphCountsNodes(3)) || !h.tt.HasText(locales.RoundtableGraphCountsHandoffs(2)) {
		t.Fatal("switching to the execution graph lost the tab state or counts", h.tt.Texts())
	}
	for _, label := range []string{locales.RoundtableGraphNodeTarget(task.Name, task.Task.Assignment, 2), locales.RoundtableGraphPair("Alpha", "Beta", 1)} {
		click(label)
		if h.w.graphSel != task.ID || h.w.drawer != meetingDrawerNode || h.w.memberID != "" || !h.tt.HasText(task.Text) || !h.tt.HasText(locales.RoundtableTaskOutput()) {
			t.Fatalf("%s did not show the receiving task's output: %v", label, h.tt.Texts())
		}
		click(locales.CloseEsc())
	}
	click(locales.RoundtableMemberPanel())
	if !h.tt.HasText(locales.RoundtableMembersHeading()) || h.tt.HasText(locales.AgentDraft()) || !h.tt.HasText(locales.RoundtableGraphMatrix()) {
		t.Fatal("the manual member panel did not fit beside the graph")
	}
	click(locales.RoundtableChatView())
	if h.w.draft != "Private follow-up" || h.w.view != roundtableViewChat || !h.tt.HasText(locales.AgentDraft()) || !h.tt.HasText(locales.RoundtableMembersHeading()) {
		t.Fatal("returning to the conversation lost its draft")
	}
	click(locales.RoundtableGraphView())
	if h.tt.HasText(locales.RoundtableMembersHeading()) {
		t.Fatal("the manual panel override survived a view change")
	}
}

func TestRoundtableGraphNarrowReadOnlyInspector(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	room := h.w.meeting.room
	if err := room.Discuss("Review", "Check a file", "agent:beta"); err != nil {
		t.Fatal(err)
	}
	h.until(t, "reply", func() bool { return !room.Busy() })
	h.w.meeting.readOnly = true
	h.tt.SetSize(600, 640)
	h.frames()
	if err := h.tt.Click(locales.RoundtableGraphView()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.tt.HasText(locales.SessionsMeetingReadOnly()) || h.tt.HasText(locales.AgentDraft()) || h.w.graphScroll.MaxX > 1 {
		t.Fatal("narrow read-only graph", h.tt.Texts())
	}
	before := room.Snapshot()
	if err := h.tt.Click(locales.RoundtableGraphNodeTarget("Beta", "Check a file", 1)); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.tt.HasText(locales.RoundtableTaskOutput()) || h.tt.HasText(locales.RoundtableAskMember()) || h.w.memberID != "" || h.w.drawer != meetingDrawerNode || !slices.Equal(before.Messages, room.Snapshot().Messages) {
		t.Fatal("read-only navigation must only open the task", h.tt.Texts())
	}
}

func TestRoundtableViewsRememberChatPanel(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	for _, width := range []int{1240, 860} {
		h.tt.SetSize(width, 760)
		h.frames()
		click(locales.RoundtableMemberPanel())
		_, inChat := h.tt.Find(locales.RoundtableMembersHeading())
		click(locales.RoundtableGraphView())
		if _, shown := h.tt.Find(locales.RoundtableMembersHeading()); shown {
			t.Fatal("Graph inherited the conversation's panel override")
		}
		click(locales.RoundtableMemberPanel())
		click(locales.RoundtableChatView())
		if _, restored := h.tt.Find(locales.RoundtableMembersHeading()); restored != inChat {
			t.Fatalf("at %d the conversation lost its panel state", width)
		}
	}
}

func TestRoundtablePinnedMemoryStripAndRoleColors(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	if _, found := h.tt.Find(locales.RoundtableOpenPinned()); found {
		t.Fatal("empty memory strip is visible")
	}
	for _, note := range []string{"First decision", "Second decision", "Third decision", "Fourth decision"} {
		if err := h.w.meeting.room.AddMemory(note, ""); err != nil {
			t.Fatal(err)
		}
	}
	h.frames()
	if !h.tt.HasText("First decision") || h.tt.HasText("Second decision") {
		t.Fatal("pinned summary did not use the first note")
	}
	if err := h.tt.Click(locales.RoundtablePreviewPins()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.w.pinsExpanded || !h.tt.HasText("Third decision") || h.tt.HasText("Fourth decision") {
		t.Fatal("preview should contain the first three notes")
	}
	if err := h.tt.Click(locales.RoundtableOpenPinned()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.drawer != meetingDrawerMemory || !h.tt.HasText("Fourth decision") {
		t.Fatal("strip did not open the complete memory drawer")
	}
	mono := darkPalette().derive()
	mono.mono = true
	for _, pal := range []*palette{lightPalette().derive(), darkPalette().derive(), mono} {
		for id, ink := range map[string]ui.Color{"": pal.muted, "preset:moderate": pal.fill, "preset:requirements": pal.modified, "preset:review": pal.changed, "preset:implement": pal.added} {
			if got := meetingInk(pal, meetingSpeaker{Participant: roundtable.Participant{ID: id}}); got != ink {
				t.Fatalf("unexpected color for %q", id)
			}
		}
	}
}

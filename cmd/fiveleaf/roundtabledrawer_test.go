package main

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestRoundtableDrawerSectionsAndNavigation(t *testing.T) {
	snapshot := graphRenderSnapshot()
	snapshot.Messages[1].Task.Tools[0].Detail = "Recorded tool output"
	window, tester := graphRenderTester(t, &snapshot, 1240)
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		tester.Advance(motionEnter)
		step(tester)
	}
	click(locales.RoundtableGraphNodeTarget("Analyst", "Assignment 1", 2))
	sections := []string{locales.RoundtableBasicInfo(), locales.RoundtableLinks(), locales.RoundtableInput(), locales.RoundtableTimeline(), locales.RoundtableCallLog(), locales.RoundtableTaskOutput()}
	texts := tester.Texts()
	for _, section := range sections {
		at := slices.Index(texts, section)
		if at < 0 {
			t.Fatalf("missing or out-of-order section %q in %v", section, tester.Texts())
		}
		texts = texts[at+1:]
	}
	click(locales.RoundtablePreviousNode())
	if window.graphSel != "task-0" || tester.HasText(locales.RoundtableCallLog()) {
		t.Fatal("previous did not change the node and its optional call log")
	}
	click(locales.RoundtablePreviousNode())
	if window.graphSel != "task-3" {
		t.Fatal("previous did not wrap from first to last")
	}
	click(locales.RoundtableNextNode())
	if window.graphSel != "task-0" {
		t.Fatal("next did not wrap from last to first")
	}
	tester.Key(0, ui.KeyEscape)
	step(tester)
	if window.drawer != meetingDrawerClosed || tester.HasText(locales.RoundtableBasicInfo()) || window.graphSel != "task-0" {
		t.Fatal("Escape did not dismiss the drawer and keep the graph selection")
	}
	click(locales.RoundtableGraphNodeTarget("Chair", "Assignment 0", 1))
	tester.ClickAt(1050, 720)
	step(tester)
	if window.drawer != meetingDrawerNode {
		t.Fatal("the backdrop captured a click inside the drawer")
	}
	snapshot.Messages = snapshot.Messages[1:]
	step(tester)
	if window.drawer != meetingDrawerNode || !tester.HasText("Assignment 0") {
		t.Fatal("a new history window evicted the task being read")
	}
	tester.ClickAt(160, 440)
	step(tester)
	if window.drawer != meetingDrawerClosed || window.memberID != "" {
		t.Fatal("the backdrop did not dismiss the drawer")
	}
}

func TestRoundtableDrawerMemoryFocusAndEscape(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.w.draft = "Meeting draft"
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	click(locales.RoundtableGraphView())
	click(locales.RoundtableOpenMemory())
	click(locales.RoundtableMemoryAdd())
	h.tt.Type("Keep the current source")
	if !h.tt.Focused(locales.RoundtableMemoryNote()) || h.w.inspector.memory.draft != "Keep the current source" || h.w.draft != "Meeting draft" {
		t.Fatal("typing did not stay in the note field")
	}
	h.tt.Key(0, ui.KeyEscape)
	h.frames()
	if h.w.drawer != meetingDrawerClosed || h.tt.HasText(locales.RoundtableMemoryNote()) {
		t.Fatal("Escape with the note field focused did not close the memory drawer")
	}
	click(locales.RoundtableOpenMemory())
	if !h.tt.Focused(locales.RoundtableMemoryNote()) || h.w.inspector.memory.draft != "Keep the current source" {
		t.Fatal("the note draft or its focus was lost on reopen")
	}
	click(locales.CloseEsc())
	click(locales.RoundtableCollaboration())
	if !h.tt.HasText(locales.RoundtableChair()) || !h.tt.HasText(locales.RoundtableExpandReplies()) || h.tt.HasText(locales.AgentDraft()) {
		t.Fatal("graph collaboration settings did not open in their own drawer")
	}
}

func TestRoundtableDrawerBreakpoints(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	const question = "Review the saved result"
	room := h.w.meeting.room
	if err := room.Discuss("Review", question, "agent:alpha"); err != nil {
		t.Fatal(err)
	}
	h.until(t, "member reply", func() bool { return !room.Busy() })
	h.w.view = roundtableViewGraph
	h.w.openNodeDrawer(room.Snapshot().Messages[1].ID)
	for _, dark := range []bool{false, true} {
		h.a.settings.Theme = ""
		h.tt.SetDark(dark)
		for _, sample := range []struct{ width, drawer int }{{1240, 432}, {1239, 400}, {1040, 400}, {860, 400}, {859, 859}} {
			h.tt.SetSize(sample.width, 800)
			h.frames()
			rect, found := h.tt.Find(locales.RoundtableNodeDetails())
			if !found || rect.X != float32(sample.width-sample.drawer) || rect.W != float32(sample.drawer) || rect.Y != 0 || rect.H != 800 || h.w.graphScroll.MaxX > 1 {
				t.Fatalf("dark=%t, width=%d: drawer %+v, graph overflow %v", dark, sample.width, rect, h.w.graphScroll.MaxX)
			}
			for _, label := range []string{locales.RoundtablePreviousNode(), locales.RoundtableNextNode(), locales.CloseEsc(), locales.RoundtableBasicInfo(), locales.RoundtableEscapeHint()} {
				control, ok := h.tt.Find(label)
				if !ok || control.W <= 0 || control.X < rect.X || control.X+control.W > rect.X+rect.W+1 {
					t.Fatalf("dark=%t, width=%d: %q out of drawer: %+v", dark, sample.width, label, control)
				}
			}
			if dir := os.Getenv("SHOTS"); dir != "" {
				tone := "light"
				if dark {
					tone = "dark"
				}
				file, err := os.Create(filepath.Join(dir, fmt.Sprintf("roundtable-node-%s-%d.png", tone, sample.width)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, h.tt.Image())
				if closeErr := file.Close(); err != nil || closeErr != nil {
					t.Fatalf("saving screenshot: %v %v", err, closeErr)
				}
			}
		}
	}
}

func TestRoundtableDrawerFollowsLiveHandoff(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	meeting := h.w.meeting
	workerStarted, finishWork := make(chan struct{}), make(chan struct{})
	for _, speaker := range meeting.speakers {
		switch speaker.ID {
		case roundtable.ModeratorID:
			speaker.adapter.backend = roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
				return roundtable.Message{Text: "@agent:alpha Check the current record"}, nil
			})
		case "agent:alpha":
			speaker.adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
				close(workerStarted)
				select {
				case <-ctx.Done():
					return roundtable.Message{}, ctx.Err()
				case <-finishWork:
					return roundtable.Message{}, errors.New("Record unavailable")
				}
			})
		}
	}
	if err := meeting.room.Discuss("Review", "Check the record", ""); err != nil {
		t.Fatal(err)
	}
	h.until(t, "handoff starts", func() bool {
		select {
		case <-workerStarted:
			return true
		default:
			return false
		}
	})
	live := meeting.room.Snapshot().ActiveTask
	label := locales.RoundtableDispatchTarget(h.w.chatSpeaker(roundtable.ModeratorID, "").Name, live.Name)
	if err := h.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.drawer != meetingDrawerNode || h.w.graphSel != live.ID || !h.tt.HasText(taskStatusLabel(roundtable.Running)) {
		t.Fatal("the dispatch did not open the active worker's node", h.tt.Texts())
	}
	close(finishWork)
	h.until(t, "worker fails", func() bool { return !meeting.room.Busy() })
	if h.w.drawer != meetingDrawerNode || h.w.graphSel != live.ID || !h.tt.HasText(taskStatusLabel(roundtable.Failed)) || !h.tt.HasText("Record unavailable") {
		t.Fatal("the open drawer did not update to the same worker's recorded failure", h.tt.Texts())
	}
}

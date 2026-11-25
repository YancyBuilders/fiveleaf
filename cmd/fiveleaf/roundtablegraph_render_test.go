package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func graphRenderSnapshot() roundtable.Snapshot {
	start := time.Date(2026, 11, 19, 19, 0, 0, 0, time.UTC)
	var messages []roundtable.Message
	for index, role := range []string{roundtable.ModeratorID, "preset:requirements", roundtable.ModeratorID, "preset:requirements"} {
		when := start.Add(time.Duration(index) * time.Second)
		task := &roundtable.Task{Assignment: "Assignment " + strconv.Itoa(index), Stage: roundtable.Work, Status: roundtable.Completed, Finished: when.Add(250 * time.Millisecond)}
		name := "Chair"
		if role == "preset:requirements" {
			name = "Analyst"
			task.Tools = []roundtable.ToolRecord{{ID: "spec", Kind: "read", Title: "Read the issue"}}
		}
		if index == 2 {
			task.MemoryNotes = 1
		}
		messages = append(messages, roundtable.Message{ID: "task-" + strconv.Itoa(index), AgentID: role, Role: "assistant", Name: name, Time: when, Hop: index + 1, Task: task, Text: "A result"})
	}
	return roundtable.Snapshot{Messages: messages}
}

func graphRenderTester(t *testing.T, snapshot *roundtable.Snapshot, width int) (*roundtableWindow, *ui.Tester) {
	t.Helper()
	roster := []meetingSpeaker{
		{Participant: roundtable.Participant{ID: roundtable.ModeratorID, Name: "Chair"}},
		{Participant: roundtable.Participant{ID: "preset:requirements", Name: "Analyst"}},
		{Participant: roundtable.Participant{ID: "preset:test", Name: "Tester"}},
	}
	window := &roundtableWindow{a: &app{}, pal: lightPalette().derive(), meeting: &agentMeeting{speakers: roster, readOnly: true}}
	tester := ui.NewTester(func(c *ui.Context) {
		graph := roundtable.BuildGraph(*snapshot)
		paneWidth, _ := c.Size()
		ui.Column(c).Fill().Children(func() { window.graphView(c, graph, paneWidth) })
		window.meetingDrawer(c, *snapshot, graph)
	}, width, 800)
	for range 5 {
		step(tester)
	}
	return window, tester
}

func TestRoundtableGraphRenderLanesAndMatrixSelectExactTask(t *testing.T) {
	snapshot := graphRenderSnapshot()
	window, tester := graphRenderTester(t, &snapshot, 600)
	if !tester.HasText(locales.RoundtableGraphStandby()) || !tester.HasText(locales.RoundtableGraphMatrix()) {
		t.Fatal(tester.Texts())
	}
	activate := func(label string, want string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		tester.Advance(motionEnter)
		step(tester)
		if window.graphSel != want || window.drawer != meetingDrawerNode || window.expandedTask != "" || window.memberID != "" || !tester.HasText(locales.RoundtableBasicInfo()) {
			t.Fatalf("%q did not open node %q in the drawer: %q, %v", label, want, window.graphSel, tester.Texts())
		}
		tester.Key(0, ui.KeyEscape)
		step(tester)
	}
	activate(locales.RoundtableGraphNodeTarget("Chair", "Assignment 2", 3), "task-2")
	pair := locales.RoundtableGraphPair("Chair", "Analyst", 2)
	for _, want := range []string{"task-1", "task-3", "task-1"} {
		activate(pair, want)
	}
	activate(locales.RoundtableGraphPair("Analyst", "Chair", 1), "task-2")
	if len(snapshot.Messages) != 4 || window.graphScroll.MaxX > 1 {
		t.Fatal("read-only graph navigation changed data or introduced horizontal scrolling")
	}
}

func TestRoundtableGraphRenderNarrowAndPaginatedMatrix(t *testing.T) {
	snapshot := graphRenderSnapshot()
	window, tester := graphRenderTester(t, &snapshot, 365)
	for index := range 6 {
		name := fmt.Sprintf("Role %d", index)
		window.meeting.speakers = append(window.meeting.speakers, meetingSpeaker{Participant: roundtable.Participant{ID: name, Name: name}})
	}
	snapshot.Messages[0].Task.Assignment = strings.Repeat("Long assignment — ", 45)
	snapshot.Messages[len(snapshot.Messages)-1].Task.Finished = time.Time{}
	for range 4 {
		step(tester)
	}
	if !tester.HasText(meetingElapsed(snapshot.Messages[0].Time, time.Time{})) || window.graphScroll.MaxX > 1 {
		t.Fatal("a long running task overflowed the graph or lost its duration", window.graphScroll.MaxX)
	}
	tester.Scroll(180, 690, 0, 600)
	step(tester)
	if err := tester.Click(locales.NextPage()); err != nil {
		t.Fatal(err)
	}
	step(tester)
	if window.graphMatrixPage == 0 || window.graphScroll.MaxX > 1 {
		t.Fatal("matrix did not page without horizontal scrolling", window.graphMatrixPage, window.graphScroll.MaxX)
	}
	if err := tester.Click(locales.PreviousPage()); err != nil {
		t.Fatal(err)
	}
	step(tester)
	if window.graphMatrixPage != 0 {
		t.Fatal("matrix did not return to the first group of columns")
	}
}

func TestRoundtableGraphEmptyRender(t *testing.T) {
	window, tester := graphRenderTester(t, &roundtable.Snapshot{}, 600)
	if !tester.HasText(locales.RoundtableGraphNoNodes()) || !tester.HasText(locales.RoundtableGraphStandby()) || tester.HasText(locales.RoundtableGraphMatrix()) || window.graphScroll.MaxX > 1 {
		t.Fatal(tester.Texts())
	}
}

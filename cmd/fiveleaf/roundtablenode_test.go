package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestRoundtableNodeDrawerTypes(t *testing.T) {
	graph := roundtable.Graph{Order: []string{"entry", "work"}, Edges: []roundtable.GraphEdge{
		{From: "entry", To: "work", Kind: roundtable.GraphHandoff},
		{From: "work", To: "reference", Kind: roundtable.GraphRead},
		{From: "handoff", To: "work", Kind: roundtable.GraphHandoff},
	}}
	tools := []roundtable.ToolRecord{{Kind: "read"}}
	for _, test := range []struct {
		node roundtable.GraphNode
		want string
	}{
		{roundtable.GraphNode{ID: "entry", Tools: tools, Stage: roundtable.Conclusion}, locales.RoundtableNodeEntry()},
		{roundtable.GraphNode{ID: "handoff", MemoryNotes: 2, Stage: roundtable.Conclusion}, locales.RoundtableNodeHandoff()},
		{roundtable.GraphNode{ID: "handoff", Tools: tools, MemoryNotes: 1}, locales.RoundtableNodeTool()},
		{roundtable.GraphNode{ID: "work", MemoryNotes: 1, Stage: roundtable.Conclusion}, locales.RoundtableNodeMemory()},
		{roundtable.GraphNode{ID: "work", MemoryTurns: 2, Stage: roundtable.Conclusion}, locales.RoundtableNodeDecision()},
		{roundtable.GraphNode{ID: "work", MemoryTurns: 2}, locales.RoundtableNodeOutput()},
	} {
		if got := meetingNodeType(graph, test.node); got != test.want {
			t.Errorf("type of %+v = %q, want %q", test.node, got, test.want)
		}
	}
	if got := meetingNodeType(roundtable.Graph{}, roundtable.GraphNode{}); got != locales.RoundtableNodeOutput() {
		t.Errorf("empty graph type = %q", got)
	}
}

func nodeDrawerTestUI(window *roundtableWindow, graph *roundtable.Graph, width, height int) (*ui.Tester, *ui.ScrollState) {
	var scroll ui.ScrollState
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Background(window.pal.editor).Padding(12, 14).TrackScroll(&scroll).Children(func() {
			ui.Column(c).FillWidth().MinWidth(0).Children(func() {
				for _, node := range graph.Nodes {
					if node.ID == window.graphSel {
						window.nodeDrawerBody(c, *graph, node)
						return
					}
				}
			})
		})
	}, width, height)
	tester.Frame()
	return tester, &scroll
}

func requireNodeDrawerTextOrder(t *testing.T, tester *ui.Tester, want ...string) {
	t.Helper()
	texts := tester.Texts()
	for _, value := range want {
		index := slices.Index(texts, value)
		if index < 0 {
			t.Fatalf("missing %q in the remaining drawer texts %q", value, texts)
		}
		texts = texts[index+1:]
	}
}

func TestRoundtableNodeDrawerRecordedData(t *testing.T) {
	start := time.Date(2026, 9, 8, 16, 7, 2, 0, time.Local)
	node := roundtable.GraphNode{ID: "review", AgentID: "agent:reviewer", Name: "Reviewer", Title: "  Inspect this input.\nPreserve its second line.",
		Text: "Result\n  indented evidence", Start: start, End: start.Add(250 * time.Millisecond), Status: roundtable.Completed,
		Tools: []roundtable.ToolRecord{
			{ID: "call-1", Kind: "read", Title: "Inspect source", Status: "completed", Detail: "src/first.go:4\n  first line", Note: "Line verified."},
			{ID: "call-2", Kind: "search", Status: "in_progress", Note: "A note without a path"},
			{ID: "call-3", Kind: "read", Title: "Open record", Status: "failed", Detail: "  File unavailable"},
			{ID: "call-4", Kind: "read", Title: "Await details", Status: "pending", Note: " \n", Detail: "\t"},
		}}
	graph := roundtable.Graph{Nodes: []roundtable.GraphNode{node, {ID: "end", AgentID: "writer", Title: "Form conclusion"}, {ID: "entry", Name: "Host"}},
		Order: []string{"entry", node.ID, "end"}, Edges: []roundtable.GraphEdge{
			{From: "entry", To: node.ID, Kind: roundtable.GraphHandoff}, {From: node.ID, To: "end", Kind: roundtable.GraphRead},
		}}
	window := &roundtableWindow{graphSel: node.ID, pal: lightPalette().derive(), meeting: &agentMeeting{speakers: []meetingSpeaker{
		{Participant: roundtable.Participant{ID: "other"}, provider: agentProvider{name: "Wrong provider"}},
		{Participant: roundtable.Participant{ID: node.AgentID, Name: "Roster name"}, provider: agentProvider{name: "Test runtime"}, role: settings.AgentRole{Model: "model-v2"}},
	}}}
	tester, _ := nodeDrawerTestUI(window, &graph, 400, 1100)
	edges := "Host → Reviewer · " + locales.RoundtableGraphHandoffLegend() + "\nReviewer → writer · " + locales.RoundtableGraphReadLegend()
	log := "Inspect source\nsrc/first.go:4\n  first line\nLine verified.\n\nsearch\nA note without a path\n\nOpen record\n  File unavailable"
	requireNodeDrawerTextOrder(t, tester,
		locales.RoundtableBasicInfo(), locales.RoundtableNode(), "2 / 3", locales.RoundtableExecutor(), "Roster name",
		locales.RoundtableType(), locales.RoundtableNodeTool(), locales.RoundtableStatus(), taskStatusLabel(roundtable.Completed),
		locales.RoundtableStartTime(), "16:07:02", locales.RoundtableGraphDuration(), meetingElapsed(node.Start, node.End),
		locales.RoundtableModel(), "Test runtime · model-v2", locales.RoundtablePermissions(), locales.RoundtableReadOnly(),
		locales.RoundtableLinks(), locales.RoundtableUpstream(), locales.RoundtableNodeNumber(1)+" · "+locales.RoundtableMemberReply(),
		locales.RoundtableEdges(), edges, locales.RoundtableDownstream(), locales.RoundtableNodeNumber(3)+" · Form conclusion",
		locales.RoundtableInput(), node.Title, locales.RoundtableTaskTools(), "Inspect source", locales.RoundtableTimeline(),
		"Inspect source", agentStatusLabel("completed"), "search", agentStatusLabel("in_progress"), "Open record", agentStatusLabel("failed"), "Await details", agentStatusLabel("pending"),
		locales.RoundtableCallLog(), log, locales.RoundtableTaskOutput(), node.Text)
	graph.Nodes[0].Tools[0].Kind = "edit"
	graph.Nodes[0].End, graph.Nodes[0].Status = time.Time{}, roundtable.Running
	tester.Frame()
	requireNodeDrawerTextOrder(t, tester, locales.RoundtableStatus(), taskStatusLabel(roundtable.Running), locales.RoundtableGraphDuration(), locales.RoundtableInProgress(), locales.RoundtablePermissions(), locales.RoundtableWritable())
}

func TestRoundtableNodeDrawerLinkTargets(t *testing.T) {
	graph := roundtable.Graph{Order: []string{"start", "left", "right", "other"}, Nodes: []roundtable.GraphNode{
		{ID: "left", Title: "Review", Text: "First result"}, {ID: "right", Title: "Review", Text: "Second result"}, {ID: "other", Title: "Unrelated"}, {ID: "start"},
	}, Edges: []roundtable.GraphEdge{
		{From: "start", To: "left", Kind: roundtable.GraphRead}, {From: "start", To: "right", Kind: roundtable.GraphHandoff},
		{From: "start", To: "left", Kind: roundtable.GraphHandoff}, {From: "start", To: "missing", Kind: roundtable.GraphRead},
	}}
	window := &roundtableWindow{graphSel: "start", pal: lightPalette().derive(), meeting: &agentMeeting{}}
	tester, _ := nodeDrawerTestUI(window, &graph, 400, 900)
	if tester.HasText("missing") || tester.HasText("Unrelated") || !tester.HasText(locales.RoundtableStartNode()) {
		t.Fatal("unexpected links at the start node", tester.Texts())
	}
	links := nodeDrawerConnections(graph, "start")
	if len(links.downstream) != 2 || len(links.edges) != 3 {
		t.Fatalf("expected one link per neighbor and one description per real edge, got %+v", links)
	}
	click := func(label, id string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		tester.Frame()
		if window.graphSel != id {
			t.Fatalf("click on %q selected %q, want %q", label, window.graphSel, id)
		}
	}
	click(locales.RoundtableNodeNumber(3)+" · Review", "right")
	requireNodeDrawerTextOrder(t, tester, locales.RoundtableDownstream(), locales.RoundtableEndNode(), locales.RoundtableTaskOutput(), "Second result")
	click(locales.RoundtableNodeNumber(1)+" · "+locales.RoundtableMemberReply(), "start")
	click(locales.RoundtableNodeNumber(2)+" · Review", "left")
	requireNodeDrawerTextOrder(t, tester, locales.RoundtableTaskOutput(), "First result")
}

func TestRoundtableNodeDrawerMissingData(t *testing.T) {
	graph := roundtable.BuildGraph(roundtable.Snapshot{Messages: []roundtable.Message{
		{ID: "archive", AgentID: "archived-agent", Role: "assistant", Task: &roundtable.Task{Assignment: " \t\n"}},
	}})
	window := &roundtableWindow{graphSel: "archive", pal: lightPalette().derive(), meeting: &agentMeeting{}}
	tester, _ := nodeDrawerTestUI(window, &graph, 365, 900)
	requireNodeDrawerTextOrder(t, tester, locales.RoundtableNode(), "1 / 1", locales.RoundtableExecutor(), "archived-agent", locales.RoundtableType(), locales.RoundtableNodeEntry(),
		locales.RoundtableStatus(), "—", locales.RoundtableStartTime(), "—", locales.RoundtableGraphDuration(), "—", locales.RoundtableModel(), "—", locales.RoundtablePermissions(), locales.RoundtableWritable(),
		locales.RoundtableUpstream(), locales.RoundtableStartNode(), locales.RoundtableDownstream(), locales.RoundtableEndNode(),
		locales.RoundtableInput(), locales.RoundtableInheritedContext(), locales.RoundtableTimeline(), locales.RoundtableOutputGenerated(), locales.RoundtableTaskOutput(), "—")
	for _, absent := range []string{locales.RoundtableEdges(), locales.RoundtableCallLog(), locales.RoundtableTaskTools()} {
		if _, found := tester.Find(absent); found {
			t.Errorf("empty node shows %q", absent)
		}
	}
	graph.Nodes[0].Tools = []roundtable.ToolRecord{{ID: "recorded-id", Kind: "read", Note: "\n ", Detail: "\t"}}
	tester.Frame()
	requireNodeDrawerTextOrder(t, tester, locales.RoundtableTaskTools(), "recorded-id", locales.RoundtableTimeline(), "read", locales.RoundtableTaskOutput())
	if _, found := tester.Find(locales.RoundtableCallLog()); found {
		t.Fatal("an empty tool record left a call-log heading")
	}
}

func TestRoundtableNodeDrawerLongContent(t *testing.T) {
	const width = 290
	node := roundtable.GraphNode{ID: "long", Title: strings.Repeat("input段落", 38), Text: strings.Repeat("OutputWithoutSpaces", 28), Tools: []roundtable.ToolRecord{
		{ID: "call", Title: strings.Repeat("long-tool-name-", 24), Detail: strings.Repeat("long_path/", 48) + "\n  last line"},
	}}
	neighbor := roundtable.GraphNode{ID: "next", Title: strings.Repeat("linked-task-", 28)}
	graph := roundtable.Graph{Order: []string{node.ID, neighbor.ID}, Nodes: []roundtable.GraphNode{node, neighbor}, Edges: []roundtable.GraphEdge{
		{From: node.ID, To: neighbor.ID, Kind: roundtable.GraphHandoff},
	}}
	for _, theme := range []struct {
		name string
		pal  *palette
	}{{"light", lightPalette().derive()}, {"dark", darkPalette().derive()}} {
		t.Run(theme.name, func(t *testing.T) {
			window := &roundtableWindow{graphSel: node.ID, pal: theme.pal, meeting: &agentMeeting{}}
			tester, scroll := nodeDrawerTestUI(window, &graph, width, 3000)
			tester.SetDark(theme.pal.dark)
			for _, value := range []string{node.Title, node.Text, node.Tools[0].Title, nodeDrawerCallLog(node.Tools), locales.RoundtableNodeNumber(2) + " · " + neighbor.Title} {
				bounds, found := tester.Find(value)
				if !found || bounds.X < 0 || bounds.X+bounds.W > width-12 || bounds.H < 32 {
					t.Errorf("wrapped bounds for %.16q = %+v, found %t", value, bounds, found)
				}
			}
			frame := tester.Image()
			background := func(value string) ui.Color {
				bounds, found := tester.Find(value)
				if !found || bounds.H < 8 {
					t.Fatalf("missing block for %.16q", value)
				}
				pixel := frame.RGBAAt(int(bounds.X+5), int(bounds.Y+4))
				return ui.RGB(pixel.R, pixel.G, pixel.B)
			}
			input, output, log := background(node.Title), background(node.Text), background(nodeDrawerCallLog(node.Tools))
			if input == output || input == theme.pal.editor || output == theme.pal.editor || contrast(white, log) < 4.5 {
				t.Errorf("input, output and call-log backgrounds = %v, %v, %v", input, output, log)
			}
			tester.SetSize(width, 480)
			if scroll.MaxX > 0 || scroll.MaxY <= 0 {
				t.Errorf("drawer scroll limits = (%v, %v)", scroll.MaxX, scroll.MaxY)
			}
			scroll.Y = scroll.MaxY
			tester.Frame()
			last, found := tester.Find(node.Text)
			if !found || last.H < 32 || last.Y+last.H > 480 || last.Y+last.H <= 0 {
				t.Errorf("output end is unreachable: %+v", last)
			}
		})
	}
}

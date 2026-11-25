package roundtable

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestGraphEmpty(t *testing.T) {
	graph := BuildGraph(Snapshot{})
	if len(graph.Nodes) != 0 || len(graph.Order) != 0 || len(graph.Edges) != 0 || graph.Handoffs() != 0 || graph.MemoryWrites() != 0 {
		t.Fatal(graph)
	}
	if elapsed, complete := graph.Elapsed(); complete || elapsed != 0 {
		t.Fatal(elapsed, complete)
	}
}

func TestTaskGraphIncludesActiveAndInterruptedRecipients(t *testing.T) {
	start := time.Unix(1000, 0)
	record := func(id, speaker, status string, offset time.Duration) Message {
		return Message{ID: id, AgentID: speaker, Role: "task", Time: start.Add(offset), Task: &Task{Status: status, Stage: Work, TurnID: "brief"}}
	}
	failed := record("failure", "A", Failed, 0)
	stopped := record("stop", "B", Stopped, time.Second)
	live := record("active", "A", Running, 2*time.Second)
	snapshot := Snapshot{Messages: []Message{failed, stopped}, ActiveTask: live}
	if graph := BuildGraph(snapshot); len(graph.Nodes) != 0 {
		t.Fatal("pending and interrupted records changed the execution graph", graph)
	}
	for range 2 {
		graph := BuildTaskGraph(snapshot)
		if !slices.Equal(graph.Order, []string{failed.ID, stopped.ID, live.ID}) || graph.Nodes[2].Status != Running || graph.Handoffs() != 2 {
			t.Fatal("incomplete task detail graph", graph)
		}
		snapshot.Messages = append(snapshot.Messages, live)
	}
}

func TestGraphProjectsSingleTaskAndOwnsTools(t *testing.T) {
	start := time.Unix(1000, 0)
	end := start.Add(1250 * time.Millisecond)
	tool := ToolRecord{ID: "file", Kind: "read", Title: "Read spec", Status: Completed, Detail: "spec.md", Note: "Check freshness"}
	snapshot := Snapshot{Messages: []Message{{
		ID: "answer", AgentID: "role:design", Name: "Designer", Role: "assistant", Hop: 2, Time: start, Text: "The estimate is fixed.",
		Task: &Task{TurnID: "question", Stage: Direct, Status: Completed, Finished: end, Tools: []ToolRecord{tool}, MemoryNotes: 3, MemoryTurns: 4},
	}}}
	graph := BuildGraph(snapshot)
	expected := GraphNode{ID: "answer", AgentID: "role:design", Name: "Designer", TurnID: "question",
		Stage: Direct, Status: Completed, Hop: 2, Start: start, End: end, Tools: []ToolRecord{tool}, MemoryNotes: 3, MemoryTurns: 4, Text: "The estimate is fixed."}
	if len(graph.Nodes) != 1 || !reflect.DeepEqual(expected, graph.Nodes[0]) || !slices.Equal(graph.Order, []string{"answer"}) || len(graph.Edges) != 0 || graph.MemoryWrites() != 1 {
		t.Fatal(graph)
	}
	if elapsed, complete := graph.Elapsed(); !complete || elapsed != 1300*time.Millisecond {
		t.Fatal(elapsed, complete)
	}
	graph.Nodes[0].Tools[0].Detail = "edited graph"
	if snapshot.Messages[0].Task.Tools[0] != tool {
		t.Fatal("editing the graph changed the source tool")
	}
}

func TestGraphOrdersHandoffsByTimeAndMemberID(t *testing.T) {
	start := time.Unix(1000, 0)
	message := func(id, agent string, offset time.Duration) Message {
		return Message{ID: id, Role: "assistant", AgentID: agent, Name: "Same name", Time: start.Add(offset),
			Task: &Task{Assignment: "Check " + id, Status: Completed, Finished: start.Add(offset + time.Second)}}
	}
	first := message("first", "role:one", time.Second)
	second := message("second", "role:two", 2*time.Second)
	third := message("third", "role:one", 2*time.Second)
	fourth := message("fourth", "role:one", 4*time.Second)
	messages := []Message{second, fourth, first, third}
	graph := BuildGraph(Snapshot{Messages: messages})
	expected := []string{"first", "second", "third", "fourth"}
	if !slices.Equal(graph.Order, expected) || len(graph.Nodes) != len(expected) {
		t.Fatal(graph.Order)
	}
	for index, node := range graph.Nodes {
		if node.ID != expected[index] || node.Title != "Check "+node.ID {
			t.Fatal(index, node)
		}
	}
	edges := []GraphEdge{{From: first.ID, To: second.ID, Kind: GraphHandoff}, {From: second.ID, To: third.ID, Kind: GraphHandoff}}
	if !slices.Equal(graph.Edges, edges) || graph.Handoffs() != 2 || messages[0].ID != second.ID || messages[1].ID != fourth.ID {
		t.Fatal(graph.Edges, messages)
	}
	if elapsed, complete := graph.Elapsed(); !complete || elapsed != 4*time.Second {
		t.Fatal(elapsed, complete)
	}
	graph.Edges = append(graph.Edges, GraphEdge{From: second.ID, To: fourth.ID, Kind: GraphRead})
	if graph.Handoffs() != 2 {
		t.Fatal("read reference counted as handoff")
	}
}

func TestGraphDoesNotCountUserRequestsAsHandoffs(t *testing.T) {
	start := time.Unix(1000, 0)
	const moderatorTurn, reviewerTurn, reviewer = "first-question", "second-question", "preset:review"
	for _, turns := range [][2]string{{moderatorTurn, reviewerTurn}, {moderatorTurn, ""}, {"", ""}} {
		firstTurn, nextTurn := turns[0], turns[1]
		messages := []Message{
			{ID: moderatorTurn, Role: "user", Time: start},
			{ID: "assign", Role: "assistant", AgentID: ModeratorID, Hop: 1, Time: start.Add(time.Second), Task: &Task{TurnID: firstTurn, Stage: Coordinate, Status: Completed}},
			{ID: "review", Role: "assistant", AgentID: reviewer, Hop: 2, Time: start.Add(2 * time.Second), Task: &Task{TurnID: firstTurn, Stage: Work, Status: Completed}},
			{ID: "wrap", Role: "assistant", AgentID: ModeratorID, Hop: 3, Time: start.Add(3 * time.Second), Task: &Task{TurnID: firstTurn, Stage: Conclusion, Status: Completed}},
			{ID: reviewerTurn, Role: "user", Time: start.Add(4 * time.Second)},
			{ID: "direct", Role: "assistant", AgentID: reviewer, Hop: 1, Time: start.Add(5 * time.Second), Task: &Task{TurnID: nextTurn, Stage: Direct, Status: Completed}},
		}
		graph := BuildGraph(Snapshot{Messages: messages})
		want := []GraphEdge{{From: "assign", To: "review", Kind: GraphHandoff}, {From: "review", To: "wrap", Kind: GraphHandoff}}
		if !slices.Equal(graph.Order, []string{"assign", "review", "wrap", "direct"}) || !slices.Equal(graph.Edges, want) {
			t.Errorf("turns %q: got order %v and edges %v, want four nodes and edges %v", turns, graph.Order, graph.Edges, want)
		}
	}
}

func TestGraphFiltersTasksAndKeepsUnfinishedEnd(t *testing.T) {
	start := time.Unix(1000, 0)
	graph := BuildGraph(Snapshot{Messages: []Message{
		{ID: "human", Role: "user", Text: "Review"},
		{ID: "legacy", Role: "assistant", Text: "Not a task"},
		{ID: "failed", Role: "task", Task: &Task{Status: Failed}},
		{ID: "tool", Role: "assistant", ToolOnly: true, Task: &Task{Status: Completed}},
		{ID: "saved-tool", Role: "assistant", Task: &Task{NoAnswer: true, Status: Completed}},
		{ID: "eligible", Role: "assistant", AgentID: "agent", Time: start, Task: &Task{Stage: Work, Status: Running}},
	}, ActiveTask: Message{ID: "live", AgentID: "live-agent", Task: &Task{Status: Running}}})
	if !slices.Equal(graph.Order, []string{"eligible"}) || len(graph.Edges) != 0 || !graph.Nodes[0].End.IsZero() {
		t.Fatal(graph)
	}
	if elapsed, complete := graph.Elapsed(); complete || elapsed != 0 {
		t.Fatal(elapsed, complete)
	}
}

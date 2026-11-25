package roundtable

import (
	"slices"
	"time"
)

const (
	GraphHandoff = "handoff"
	GraphRead    = "read"
)

type GraphNode struct {
	ID, AgentID, Name, Title string
	TurnID                   string
	Stage, Status            string
	Hop                      int
	Start, End               time.Time
	Tools                    []ToolRecord
	MemoryNotes, MemoryTurns int
	Text                     string
}

type GraphEdge struct {
	From, To, Kind string
}

type Graph struct {
	Nodes []GraphNode
	Edges []GraphEdge
	Order []string
}

func BuildGraph(snapshot Snapshot) Graph {
	return buildGraph(snapshot, false)
}

func BuildTaskGraph(snapshot Snapshot) Graph {
	return buildGraph(snapshot, true)
}

func buildGraph(snapshot Snapshot, includeTaskRecords bool) Graph {
	var graph Graph
	add := func(message Message) {
		task := message.Task
		if task == nil || message.ID == "" || message.Role != "assistant" && !(includeTaskRecords && message.Role == "task") {
			return
		}
		if !includeTaskRecords && (message.ToolOnly || task.NoAnswer) {
			return
		}
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID: message.ID, AgentID: message.AgentID, Name: message.Name, Title: task.Assignment, TurnID: task.TurnID,
			Stage: task.Stage, Status: task.Status, Hop: message.Hop, Start: message.Time, End: task.Finished,
			Tools: slices.Clone(task.Tools), MemoryNotes: task.MemoryNotes, MemoryTurns: task.MemoryTurns, Text: message.Text,
		})
	}
	for _, message := range snapshot.Messages {
		add(message)
	}
	if active := snapshot.ActiveTask; includeTaskRecords && active.ID != "" && !slices.ContainsFunc(graph.Nodes, func(node GraphNode) bool { return node.ID == active.ID }) {
		add(active)
	}
	slices.SortStableFunc(graph.Nodes, func(left, right GraphNode) int { return left.Start.Compare(right.Start) })
	for index, node := range graph.Nodes {
		graph.Order = append(graph.Order, node.ID)
		if index == 0 {
			continue
		}
		previous := graph.Nodes[index-1]
		startsLegacyTurn := node.TurnID == "" && node.Hop == 1
		if previous.TurnID == node.TurnID && previous.AgentID != node.AgentID && !startsLegacyTurn {
			graph.Edges = append(graph.Edges, GraphEdge{From: previous.ID, To: node.ID, Kind: GraphHandoff})
		}
	}
	return graph
}

func (graph Graph) Handoffs() int {
	count := 0
	for _, edge := range graph.Edges {
		if edge.Kind == GraphHandoff {
			count++
		}
	}
	return count
}

func (graph Graph) MemoryWrites() int {
	count := 0
	for _, node := range graph.Nodes {
		if node.MemoryNotes > 0 {
			count++
		}
	}
	return count
}

func (graph Graph) Elapsed() (time.Duration, bool) {
	if len(graph.Nodes) == 0 {
		return 0, false
	}
	end := graph.Nodes[len(graph.Nodes)-1].End
	if end.IsZero() {
		return 0, false
	}
	return end.Sub(graph.Nodes[0].Start).Round(100 * time.Millisecond), true
}

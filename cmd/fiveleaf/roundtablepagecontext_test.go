package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func openRoundtablePageArchive(t *testing.T, messages []roundtable.Message) (*roundtable.Room, string) {
	t.Helper()
	data := t.TempDir()
	library, err := roundtable.ProjectLibrary(data, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.CreateWithChair("Archive", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(data, "roundtables", library.Key(), group.ID, "messages")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := range messages {
		message := &messages[index]
		message.Time = time.Unix(int64(index+1), 0).UTC()
		raw, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		name := message.Time.Format("20060102T150405.000000000") + "-" + message.ID + ".json"
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	room, err := library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(room.Close)
	return room, dir
}

func pageTask(id, turn, stage string, hop int) roundtable.Message {
	message := roundtable.Message{ID: id, Role: "assistant", AgentID: roundtable.ModeratorID, Hop: hop, Text: id,
		Task: &roundtable.Task{TurnID: turn, Stage: stage, Status: roundtable.Completed}}
	switch stage {
	case roundtable.Coordinate:
		message.Text = "@worker Check the record"
	case roundtable.Work:
		message.AgentID = "worker"
		message.Task.Tools = []roundtable.ToolRecord{{ID: id, Detail: "Evidence for " + id, Status: roundtable.Completed}}
	}
	return message
}

func pageQuestions(prefix string, count int) []roundtable.Message {
	messages := make([]roundtable.Message, count)
	for index := range messages {
		id := fmt.Sprintf("%s-%d", prefix, index)
		messages[index] = roundtable.Message{ID: id, Role: "user", Text: id}
	}
	return messages
}

func TestRoundtableArchivePageContext(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%t", legacy), func(t *testing.T) {
			testRoundtableArchivePageContext(t, legacy)
		})
	}
}

func testRoundtableArchivePageContext(t *testing.T, legacy bool) {
	messages := []roundtable.Message{
		pageTask("unrelated", "left", roundtable.Work, 2),
		{ID: "left", Role: "user", Text: "Start a new turn"},
		pageTask("first-dispatch", "left", roundtable.Coordinate, 1),
		pageTask("finding", "left", roundtable.Work, 2),
		pageTask("conclusion", "left", roundtable.Conclusion, 3),
	}
	messages = append(messages, pageQuestions("middle", meetingPageSize-2)...)
	messages = append(messages, pageTask("dispatch", "right", roundtable.Coordinate, 1))
	receiver := pageTask("receiver", "right", roundtable.Work, 2)
	receiver.Role, receiver.Task.Status = "task", roundtable.Failed
	messages = append(messages, receiver, pageTask("later", "another", roundtable.Work, 2))
	messages = append(messages, pageQuestions("newer", roundtable.MaxHistoryMessages-2)...)
	if legacy {
		for _, message := range messages {
			if message.Task != nil {
				message.Task.TurnID = ""
			}
		}
	}
	room, dir := openRoundtablePageArchive(t, messages)
	unrelated, err := filepath.Glob(filepath.Join(dir, "*-unrelated.json"))
	if err != nil || len(unrelated) != 1 {
		t.Fatalf("locating unrelated record: %v, %v", unrelated, err)
	}
	if err := os.WriteFile(unrelated[0], []byte("unreadable unrelated record"), 0o600); err != nil {
		t.Fatal(err)
	}
	recent := room.Snapshot()
	if !recent.HasEarlier || recent.Messages[0].ID != receiver.ID {
		t.Fatal("expected the receiving task at the live history boundary")
	}
	prefix, err := room.EarlierTurnPrefix(recent.Messages[0])
	if err != nil || len(prefix) != 1 || prefix[0].ID != "dispatch" {
		t.Fatalf("live history boundary prefix = %v, error %v", prefix, err)
	}
	window := &roundtableWindow{meeting: &agentMeeting{room: room}}
	for end := len(recent.Messages); end > 0; end -= meetingPageSize {
		first := recent.Messages[end-meetingPageSize]
		window.loadPage(append(window.pageCursors, roundtable.Message{Time: first.Time, ID: first.ID}))
	}
	if window.err != "" || len(window.page) != meetingPageSize || window.page[0].ID != "conclusion" || window.page[meetingPageSize-1].ID != "dispatch" || !window.moreBefore {
		t.Fatalf("wrong rendered archive page (%d records, earlier %t, error %q)", len(window.page), window.moreBefore, window.err)
	}
	graph := window.historyGraph()
	if want := []string{"first-dispatch", "finding", "conclusion", "dispatch", "receiver"}; !slices.Equal(graph.Order, want) {
		t.Errorf("detail graph has %v, want %v", graph.Order, want)
	}
	if evidence := meetingFlowEvidence(window.page[0], graph); len(evidence) != 1 || evidence[0].ID != "finding" {
		t.Errorf("conclusion evidence = %v", evidence)
	}
	if _, ok := meetingFlowNode(graph, recent.Messages[0]); !ok {
		t.Error("the archived handoff's failed receiving task is unavailable to the drawer")
	}
	window.loadPage(nil)
	if window.page != nil || len(window.historyGraph().Nodes) != 0 {
		t.Error("returning to the latest page retained the archive graph")
	}
}

func TestRoundtableArchiveEarlierTurnPrefix(t *testing.T) {
	for _, boundary := range []roundtable.Message{
		{ID: "user", Role: "user", Text: "New turn"},
		pageTask("prior-conclusion", "", roundtable.Conclusion, 3),
		pageTask("identified-turn", "different", roundtable.Coordinate, 3),
		pageTask("hop-gap", "", roundtable.Coordinate, 2),
	} {
		t.Run(boundary.ID, func(t *testing.T) {
			messages := []roundtable.Message{
				pageTask("unrelated", "", roundtable.Work, 2), boundary,
				pageTask("finding", "", roundtable.Work, 4),
				pageTask("closing", "", roundtable.Conclusion, 5),
			}
			room, _ := openRoundtablePageArchive(t, messages)
			prefix, err := room.EarlierTurnPrefix(messages[3])
			if err != nil || len(prefix) != 1 || prefix[0].ID != "finding" || len(prefix[0].Task.Tools) != 1 {
				t.Fatalf("legacy prefix past %s = %v, error %v", boundary.ID, prefix, err)
			}
		})
	}
}

func TestRoundtableArchiveLargeTurnRetainsFirstEvidence(t *testing.T) {
	var messages []roundtable.Message
	longReply := strings.Repeat("a", roundtable.MaxMessageBytes-1)
	for index := range 5 {
		message := pageTask(fmt.Sprint(index), "question", roundtable.Work, index+1)
		message.Text = longReply
		messages = append(messages, message)
	}
	messages = append(messages, pageTask("closing", "question", roundtable.Conclusion, 6))
	room, _ := openRoundtablePageArchive(t, messages)
	prefix, err := room.EarlierTurnPrefix(messages[5])
	if err != nil || len(prefix) != 5 || prefix[0].Task.Tools[0].ID != "0" {
		t.Fatalf("large replies lost the first tool record: %d messages, error %v", len(prefix), err)
	}
}

func TestRoundtableArchivePageLegacyReceiver(t *testing.T) {
	for _, scenario := range []struct {
		name, turn, stage, agent string
		hop                      int
		intervening              roundtable.Message
		candidate, opens         bool
	}{
		{name: "adjacent", stage: roundtable.Work, hop: 2, candidate: true, opens: true},
		{name: "other agent", stage: roundtable.Work, agent: "other", hop: 2, candidate: true},
		{name: "new turn", turn: "new", stage: roundtable.Work, hop: 2},
		{name: "wrong hop", stage: roundtable.Work, hop: 1},
		{name: "other stage", stage: roundtable.Direct, hop: 2},
		{name: "intervening user", stage: roundtable.Work, hop: 2, intervening: roundtable.Message{ID: "user", Role: "user", Text: "Next round"}},
		{name: "intervening hidden record", stage: roundtable.Work, hop: 2, intervening: roundtable.Message{ID: "hidden", Role: "system"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			messages := []roundtable.Message{pageTask("dispatch", "", roundtable.Coordinate, 1)}
			if scenario.intervening.ID != "" {
				messages = append(messages, scenario.intervening)
			}
			receiver := pageTask("candidate", scenario.turn, scenario.stage, scenario.hop)
			if scenario.agent != "" {
				receiver.AgentID = scenario.agent
			}
			messages = append(messages, receiver)
			room, _ := openRoundtablePageArchive(t, messages)
			cursor := messages[1]
			if !cursor.InOverview() {
				cursor = messages[2]
			}
			var page roundtable.Snapshot
			var graph roundtable.Graph
			latest := false
			window, tester := flowTester(t, room, &page, &graph, &latest)
			window.meeting.speakers = append(window.meeting.speakers, meetingSpeaker{Participant: roundtable.Participant{ID: "worker", Name: "Worker"}})
			window.loadPage([]roundtable.Message{{ID: cursor.ID, Time: cursor.Time}})
			if window.err != "" || len(window.page) != 1 || window.page[0].ID != "dispatch" {
				t.Fatalf("lost dispatch page: %v, %s", window.page, window.err)
			}
			page.Messages, graph = window.page, window.historyGraph()
			if slices.Contains(graph.Order, "candidate") != scenario.candidate {
				t.Errorf("archived tasks = %v, want candidate %t", graph.Order, scenario.candidate)
			}
			step(tester)
			label := locales.RoundtableDispatchTarget("Chair", "Worker")
			if !tester.HasText(label) {
				t.Fatal("archived dispatch missing", tester.Texts())
			}
			if err := tester.Click(label); err != nil && scenario.opens {
				t.Fatal(err)
			}
			step(tester)
			if (window.graphSel == "candidate") != scenario.opens {
				t.Errorf("selected task %q, want receiver opened %t", window.graphSel, scenario.opens)
			}
		})
	}
}

func TestRoundtableArchiveTopicScanRecovery(t *testing.T) {
	for _, clearHistory := range []bool{true, false} {
		t.Run(fmt.Sprintf("clear=%t", clearHistory), func(t *testing.T) {
			messages := pageQuestions("question", roundtable.MaxHistoryMessages+1)
			room, dir := openRoundtablePageArchive(t, messages)
			if err := os.Rename(dir, dir+".unavailable"); err != nil {
				t.Fatal(err)
			}
			harness := newHarness(t, true)
			window := &roundtableWindow{a: harness.a, meeting: &agentMeeting{room: room}}
			page := room.Snapshot()
			page.Messages = page.Messages[len(page.Messages)-meetingPageSize:]
			var topics map[string]int
			tester := ui.NewTester(func(c *ui.Context) {
				topics = window.flowTopics(ui.Column(c), page, true)
			}, 420, 320)
			step(tester)
			deadline := time.Now().Add(3 * time.Second)
			for {
				if _, queued := harness.busy(); queued {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("no topic scan completion")
				}
				time.Sleep(time.Millisecond)
			}
			if clearHistory {
				if err := room.Clear(); err != nil {
					t.Fatal(err)
				}
				window.viewGeneration++
				harness.a.drain()
				if window.err != "" {
					t.Fatalf("cleared view received stale scan error: %s", window.err)
				}
				return
			}
			harness.a.drain()
			if window.err == "" {
				t.Fatal("archive read failure was lost")
			}
			if err := os.Rename(dir+".unavailable", dir); err != nil {
				t.Fatal(err)
			}
			last := messages[len(messages)-1].ID
			for topics[last] == 0 && time.Now().Before(deadline) {
				step(tester)
				harness.a.drain()
				time.Sleep(time.Millisecond)
			}
			if topics[last] != len(messages) || window.err != "" {
				t.Errorf("retry on the same page returned topic %d, error %q", topics[last], window.err)
			}
		})
	}
}

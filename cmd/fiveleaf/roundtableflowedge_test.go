package main

import (
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func flowTaskRecord(id, agent, turn, stage string, hop int, start time.Time) roundtable.Message {
	return roundtable.Message{ID: id, Role: "assistant", AgentID: agent, Text: id, Hop: hop, Time: start,
		Task: &roundtable.Task{TurnID: turn, Stage: stage, Status: roundtable.Completed, Finished: start.Add(time.Second)}}
}

func checkFlowDispatch(t *testing.T, window *roundtableWindow, tester *ui.Tester, wantID string) {
	t.Helper()
	label := locales.RoundtableDispatchTarget("Chair", "Tester")
	if err := tester.Click(label); err != nil {
		t.Fatal(err)
	}
	step(tester)
	wantDrawer := meetingDrawerClosed
	if wantID != "" {
		wantDrawer = meetingDrawerNode
	}
	if window.graphSel != wantID || window.drawer != wantDrawer {
		t.Fatalf("dispatch selected %q in drawer %v, want %q in drawer %v", window.graphSel, window.drawer, wantID, wantDrawer)
	}
}

func TestRoundtableFlowDispatchWorkRecords(t *testing.T) {
	start := time.Unix(1700000000, 0)
	send := flowTaskRecord("dispatch", roundtable.ModeratorID, "turn", roundtable.Coordinate, 1, start)
	send.Text = "@preset:test Inspect the record"
	foreign := flowTaskRecord("other-turn", "preset:test", "other", roundtable.Work, 2, start.Add(time.Second))
	wrongAgent := flowTaskRecord("other-agent", "role:Field", "turn", roundtable.Work, 2, start.Add(2*time.Second))
	wrongHop := flowTaskRecord("other-hop", "preset:test", "turn", roundtable.Work, 4, start.Add(3*time.Second))
	for _, status := range []string{roundtable.Running, roundtable.Stopped, roundtable.Failed, "missing"} {
		t.Run(status, func(t *testing.T) {
			receiver := flowTaskRecord("receiving-"+status, "preset:test", "turn", roundtable.Work, 2, start.Add(4*time.Second))
			receiver.Role, receiver.Task.Status = "task", status
			snapshot := roundtable.Snapshot{Messages: []roundtable.Message{send, foreign, wrongAgent, wrongHop}}
			wantID := receiver.ID
			switch status {
			case roundtable.Running:
				receiver.Task.Finished = time.Time{}
				snapshot.ActiveTask, snapshot.Busy = receiver, true
			case "missing":
				wantID = ""
			default:
				snapshot.Messages = append(snapshot.Messages, receiver)
			}
			graph, latest := roundtable.BuildTaskGraph(snapshot), true
			window, tester := flowTester(t, nil, &snapshot, &graph, &latest)
			checkFlowDispatch(t, window, tester, wantID)
			if wantID != "" && !tester.HasText(meetingElapsed(receiver.Time, receiver.Task.Finished)) {
				t.Error("dispatch did not show the receiving task's elapsed time")
			}
		})
	}
}

func TestRoundtableFlowLegacyDispatchAdjacency(t *testing.T) {
	start := time.Unix(1700000000, 0)
	send := flowTaskRecord("dispatch", roundtable.ModeratorID, "", roundtable.Coordinate, 1, start)
	send.Text = "@preset:test Inspect the record"
	work := flowTaskRecord("legacy-receiver", "preset:test", "", roundtable.Work, 2, start.Add(2*time.Second))
	gap := roundtable.Message{ID: "intervening-user", Role: "user", Text: "New question", Time: start.Add(time.Second)}
	wrongAgent := flowTaskRecord("wrong-agent", "role:Field", "", roundtable.Work, 2, work.Time)
	wrongHop := flowTaskRecord("wrong-hop", "preset:test", "", roundtable.Work, 4, work.Time)
	wrongStage := flowTaskRecord("wrong-stage", "preset:test", "", roundtable.Direct, 2, work.Time)
	for _, pageBreak := range []bool{false, true} {
		for _, fixture := range []struct {
			name      string
			following []roundtable.Message
			wantID    string
		}{
			{"adjacent", []roundtable.Message{work}, work.ID},
			{"intervening user", []roundtable.Message{gap, work}, ""},
			{"wrong agent", []roundtable.Message{wrongAgent, work}, ""},
			{"wrong hop", []roundtable.Message{wrongHop, work}, ""},
			{"wrong stage", []roundtable.Message{wrongStage, work}, ""},
		} {
			name := fixture.name
			if pageBreak {
				name += " across pages"
			}
			t.Run(name, func(t *testing.T) {
				snapshot := roundtable.Snapshot{Messages: append([]roundtable.Message{send}, fixture.following...)}
				graph, latest := roundtable.BuildTaskGraph(snapshot), !pageBreak
				if pageBreak {
					snapshot.Messages = snapshot.Messages[:1]
				}
				window, tester := flowTester(t, nil, &snapshot, &graph, &latest)
				if pageBreak {
					boundary := fixture.following[0]
					window.page, window.pageCursors = snapshot.Messages, []roundtable.Message{{ID: boundary.ID, Time: boundary.Time}}
					step(tester)
				}
				checkFlowDispatch(t, window, tester, fixture.wantID)
			})
		}
	}
	work.Role, work.Task.Status, work.Task.Finished = "task", roundtable.Running, time.Time{}
	snapshot := roundtable.Snapshot{Messages: []roundtable.Message{send}, Busy: true, ActiveTask: work}
	graph, latest := roundtable.BuildTaskGraph(snapshot), true
	window, tester := flowTester(t, nil, &snapshot, &graph, &latest)
	checkFlowDispatch(t, window, tester, work.ID)
}

func TestRoundtableFlowMissingRecipient(t *testing.T) {
	send := flowTaskRecord("dispatch", roundtable.ModeratorID, "turn", roundtable.Coordinate, 1, time.Unix(1700000000, 0))
	send.Text = "**@retired**\nInspect the record"
	snapshot := roundtable.Snapshot{Messages: []roundtable.Message{send}}
	graph, latest := roundtable.BuildTaskGraph(snapshot), true
	window, tester := flowTester(t, nil, &snapshot, &graph, &latest)
	if err := tester.Click(locales.RoundtableDispatchTarget("Chair", "retired")); err != nil {
		t.Fatal(err)
	}
	step(tester)
	if window.drawer != meetingDrawerClosed || window.graphSel != "" {
		t.Fatal("a missing recipient opened a drawer", window.graphSel)
	}
}

func TestRoundtableFlowTurnTotalsOnClosingPage(t *testing.T) {
	start := time.Unix(1700000000, 0)
	for _, fixture := range []struct {
		name, turn     string
		workHop, nodes int
		duration       time.Duration
	}{
		{"recorded turn with missing hop", "current", 3, 3, 5 * time.Second},
		{"legacy sequence", "", 2, 3, 5 * time.Second},
		{"legacy gap", "", 3, 2, 3 * time.Second},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			old := flowTaskRecord("previous", "preset:test", "", roundtable.Conclusion, 3, start.Add(-48*time.Hour))
			first := flowTaskRecord("first", roundtable.ModeratorID, fixture.turn, roundtable.Coordinate, 1, start)
			work := flowTaskRecord("work", "preset:test", fixture.turn, roundtable.Work, fixture.workHop, start.Add(2*time.Second))
			closing := flowTaskRecord("closing", roundtable.ModeratorID, fixture.turn, roundtable.Conclusion, fixture.workHop+1, start.Add(4*time.Second))
			for _, message := range []roundtable.Message{old, first, work} {
				message.Task.Tools = []roundtable.ToolRecord{{Status: roundtable.Completed, Detail: message.ID + " evidence"}}
			}
			snapshot := roundtable.Snapshot{Messages: []roundtable.Message{old, first, work, closing}}
			graph, latest := roundtable.BuildTaskGraph(snapshot), true
			snapshot.Messages = []roundtable.Message{closing}
			_, tester := flowTester(t, nil, &snapshot, &graph, &latest)
			stats := strings.Join([]string{locales.RoundtableGraphCountsNodes(fixture.nodes), locales.RoundtableGraphCountsHandoffs(fixture.nodes - 1), meetingDuration(fixture.duration)}, " · ")
			if !tester.HasText(stats) || tester.HasText("previous evidence") || !tester.HasText("work evidence") || tester.HasText("first evidence") != (fixture.nodes == 3) {
				t.Fatalf("want %q and evidence from its tasks, got %v", stats, tester.Texts())
			}
		})
	}
}

func TestRoundtableFlowEvidenceRequiresRecordedOutcome(t *testing.T) {
	closing := flowTaskRecord("closing", roundtable.ModeratorID, "current", roundtable.Conclusion, 1, time.Unix(1700000000, 0))
	const raw = "  file.go:7\n\tvalue = 4\n"
	const note = "  Cross-checked against the saved result."
	const intention = "I will inspect the record."
	closing.Task.Tools = []roundtable.ToolRecord{
		{ID: "legacy", Title: "Recorded output", Status: roundtable.Completed, Detail: raw, Note: note},
		{ID: "noted", Title: "Invocation note", Status: roundtable.Completed, InputOnly: true, Detail: "only call arguments", Note: intention},
		{ID: "args", Title: "Arguments only", Status: roundtable.Completed, InputOnly: true, Detail: "find missing.go", Note: "\t "},
		{ID: "empty", Title: "Empty output", Status: roundtable.Completed, Detail: "\n"},
	}
	for _, status := range []string{"", "pending", roundtable.Running, roundtable.Failed, roundtable.Stopped} {
		closing.Task.Tools = append(closing.Task.Tools, roundtable.ToolRecord{ID: "incomplete-" + status, Status: status, Title: "Unconfirmed " + status, Detail: "Unconfirmed result"})
	}
	snapshot := roundtable.Snapshot{Messages: []roundtable.Message{closing}}
	graph, latest := roundtable.BuildTaskGraph(snapshot), true
	window, tester := flowTester(t, nil, &snapshot, &graph, &latest)
	if !tester.HasText("Recorded output") || !tester.HasText(locales.RoundtableEvidenceHit()) || tester.HasText(locales.RoundtableCorroborated()) {
		t.Fatal("unexpected evidence", tester.Texts())
	}
	for _, text := range []string{"Invocation note", intention, "only call arguments", "Arguments only", "Empty output", "Unconfirmed result"} {
		if tester.HasText(text) {
			t.Errorf("conclusion includes %q", text)
		}
	}
	if err := tester.Click("Recorded output"); err != nil {
		t.Fatal(err)
	}
	step(tester)
	if !tester.HasText(raw) || !tester.HasText(note) {
		t.Fatal("expanded evidence changed the recorded text", tester.Texts())
	}
	window.openNodeDrawer(closing.ID)
	drawer, _ := nodeDrawerTestUI(window, &graph, 432, 1600)
	if !drawer.HasText(locales.RoundtableCallLog()) || !strings.Contains(strings.Join(drawer.Texts(), "\n"), "Invocation note\nonly call arguments\n"+intention) {
		t.Fatal("the node call log lost the input-only record", drawer.Texts())
	}
}

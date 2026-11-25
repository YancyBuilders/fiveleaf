package roundtable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

func TestModeratorChoosesWhoWorksAndCanAssignTheSameMemberAgain(t *testing.T) {
	for _, chosen := range []string{"preset:test", "role:Field analyst"} {
		t.Run(chosen, func(t *testing.T) {
			moderator, decisions := heldAdapter()
			members := []Member{{ID: ModeratorID, Enabled: true, Adapter: moderator}}
			workers := make(map[string]<-chan heldCall)
			for _, id := range []string{"preset:test", "role:Field analyst"} {
				worker, calls := heldAdapter()
				members = append(members, Member{ID: id, Enabled: true, Adapter: worker})
				workers[id] = calls
			}
			room := New(members, Options{})
			defer room.Close()
			if err := room.Discuss("Review", "请测试工程师先检查方案", ""); err != nil {
				t.Fatal(err)
			}
			for index, assignment := range []string{"Inspect the first sample", "Check the contradictory sample"} {
				nextCall(t, decisions).answer <- Message{Text: "@" + chosen + " " + assignment}
				worker := nextCall(t, workers[chosen])
				if !strings.Contains(FormatPrompt(worker.messages), "Your assignment: "+assignment) {
					t.Fatal(worker.messages)
				}
				worker.answer <- Message{Text: fmt.Sprintf("FINAL\nObservation %d", index+1)}
			}
			closing := nextCall(t, decisions)
			prompt := FormatPrompt(closing.messages)
			if !strings.Contains(prompt, "closing assignment") || !strings.Contains(prompt, "Observation 1") || !strings.Contains(prompt, "Observation 2") {
				t.Fatal(prompt)
			}
			closing.answer <- Message{Text: "Both samples agree."}
			snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
			if len(snapshot.Messages) != 6 || snapshot.Messages[5].Task.Stage != Conclusion || snapshot.Messages[5].AgentID != ModeratorID || snapshot.Messages[5].Text != "Both samples agree." {
				t.Fatal(snapshot)
			}
		})
	}
}

func TestModeratorUnavailablePreservesDirectWork(t *testing.T) {
	worker, calls := heldAdapter()
	room := New([]Member{{ID: "one", Name: ModeratorID, Enabled: true, Adapter: worker}, {ID: ModeratorID, Enabled: false, Adapter: answer("disabled")}}, Options{})
	defer room.Close()
	if err := room.Discuss("topic", "@one Private question", ""); err != nil {
		t.Fatal(err)
	}
	direct := nextCall(t, calls)
	if err := room.Discuss("topic", "Choose a member", ""); !errors.Is(err, ErrModeratorUnavailable) {
		t.Fatal(err)
	}
	select {
	case <-direct.ctx.Done():
		t.Fatal("failed automatic send cancelled a valid direct question")
	default:
	}
	direct.answer <- Message{Text: "Private answer"}
	snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if len(snapshot.Messages) != 2 || snapshot.Messages[1].Task.Stage != Direct {
		t.Fatal(snapshot)
	}
	room.SetMembers([]Member{{ID: "one", Name: ModeratorID, Enabled: true, Adapter: worker}})
	if err := room.Discuss("topic", "Try again", ""); !errors.Is(err, ErrModeratorUnavailable) {
		t.Fatal(err)
	}
}

func TestModeratorRejectsInvalidDecisions(t *testing.T) {
	for _, scenario := range []struct {
		decision string
		want     error
	}{
		{"@preset:moderate Delegate to myself", ErrUnknownAgent},
		{"@Analyst Use the name", ErrUnknownAgent},
		{"@role:ONE Wrong case", ErrUnknownAgent},
		{"@role:one missing Read it", ErrUnknownAgent},
		{"@unknown Try another member", ErrUnknownAgent},
		{"@role:one", ErrInvalidAssignment},
		{"@role:one @role:two Investigate together", ErrInvalidAssignment},
		{"@role:one Inspect a sample\n@role:two Inspect the rest", ErrInvalidAssignment},
		{"FINAL", ErrNoAnswer},
	} {
		t.Run(scenario.decision, func(t *testing.T) {
			var workerCalls atomic.Int32
			worker := AdapterFunc(func(context.Context, []Message) (Message, error) {
				workerCalls.Add(1)
				return Message{Text: "unexpected"}, nil
			})
			room := New([]Member{{ID: ModeratorID, Enabled: true, Adapter: answer(scenario.decision)}, {ID: "role:one", Name: "Analyst", Enabled: true, Adapter: worker}, {ID: "role:two", Enabled: true, Adapter: worker}, {ID: "role:one missing", Enabled: false, Adapter: worker}}, Options{})
			defer room.Close()
			if err := room.Discuss("Something", "Investigate", ""); err != nil {
				t.Fatal(err)
			}
			snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
			if len(snapshot.Messages) != 2 || snapshot.Error != scenario.want.Error() || snapshot.Messages[1].Task.Status != Failed || workerCalls.Load() != 0 {
				t.Fatal(snapshot, workerCalls.Load())
			}
		})
	}
}

func TestModeratorOnlyAndClosingBudget(t *testing.T) {
	for _, scenario := range []struct {
		name, answer string
		worker       bool
		want         error
	}{
		{"only moderator", "Use A when latency matters.", false, nil},
		{"only moderator tries to delegate", "@other Run a test", false, ErrConclusionRequired},
		{"no more work budget", "@other Run a test", true, ErrConclusionRequired},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			moderator, calls := heldAdapter()
			members := []Member{{ID: ModeratorID, Enabled: true, Adapter: moderator}}
			opts := Options{}
			if scenario.worker {
				opts.MaxHops = 1
				members = append(members, Member{ID: "other", Enabled: true, Adapter: answer("unexpected")})
			}
			room := New(members, opts)
			defer room.Close()
			if err := room.Discuss("Topic", "Answer now", ""); err != nil {
				t.Fatal(err)
			}
			closing := nextCall(t, calls)
			if !strings.Contains(FormatPrompt(closing.messages), "closing assignment") {
				t.Fatal(closing.messages)
			}
			closing.answer <- Message{Text: scenario.answer}
			snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
			if len(snapshot.Messages) != 2 || snapshot.Messages[1].AgentID != ModeratorID {
				t.Fatal(snapshot)
			}
			last := snapshot.Messages[1]
			if scenario.want != nil {
				if snapshot.Error != scenario.want.Error() || last.Task.Status != Failed {
					t.Fatal(snapshot)
				}
			} else if snapshot.Error != "" || last.Text != scenario.answer || last.Task.Stage != Conclusion || last.Task.Status != Completed {
				t.Fatal(snapshot)
			}
		})
	}
}

type stableMeetingAdapter struct{ Adapter }

func TestModeratorPendingDecisionCannotDispatchAfterRosterChange(t *testing.T) {
	for _, action := range []string{"remove worker", "disable worker", "remove moderator", "stop", "replace with shorter ID", "newly available"} {
		t.Run(action, func(t *testing.T) {
			moderator, decisions := heldAdapter()
			host := Member{ID: ModeratorID, Enabled: true, Adapter: &stableMeetingAdapter{moderator}}
			var workerCalls atomic.Int32
			worker := Member{ID: "worker", Enabled: true, Adapter: AdapterFunc(func(context.Context, []Message) (Message, error) {
				workerCalls.Add(1)
				return Message{Text: "unexpected"}, nil
			})}
			if action == "replace with shorter ID" {
				worker.ID = "worker space"
			}
			originalID := worker.ID
			room := New([]Member{host, worker}, Options{})
			defer room.Close()
			if err := room.Discuss("Topic", "Inspect the available data", ""); err != nil {
				t.Fatal(err)
			}
			opening := nextCall(t, decisions)
			switch action {
			case "remove worker":
				room.SetMembers([]Member{host})
			case "disable worker":
				worker.Enabled = false
				room.SetMembers([]Member{host, worker})
			case "remove moderator":
				room.SetMembers([]Member{worker})
			case "stop":
				room.Stop()
			case "replace with shorter ID":
				worker.ID = "worker"
				room.SetMembers([]Member{host, worker})
			case "newly available":
				worker.ID = "new worker"
				room.SetMembers([]Member{host, worker})
			}
			reply := "@" + originalID + " Inspect this sample"
			if action == "newly available" {
				reply = "@new worker Inspect this sample"
			}
			opening.answer <- Message{Text: reply}
			snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
			if len(snapshot.Messages) != 2 || snapshot.Messages[1].Task.Status == Completed || workerCalls.Load() != 0 {
				t.Fatal(snapshot, workerCalls.Load())
			}
		})
	}
}

func TestModeratorStopsOnToolOnlyOrFailedWork(t *testing.T) {
	for _, scenario := range []string{"moderator tool only", "worker tool only", "worker handoff without finding", "worker failure"} {
		t.Run(scenario, func(t *testing.T) {
			var moderatorCalls atomic.Int32
			moderator := AdapterFunc(func(context.Context, []Message) (Message, error) {
				moderatorCalls.Add(1)
				if scenario == "moderator tool only" {
					return Message{ToolOnly: true, Text: "\n"}, nil
				}
				return Message{Text: "@worker Inspect the data"}, nil
			})
			worker := AdapterFunc(func(context.Context, []Message) (Message, error) {
				switch scenario {
				case "worker tool only":
					return Message{ToolOnly: true}, nil
				case "worker failure":
					return Message{}, errors.New("read failed")
				default:
					return Message{Text: "@another Your turn"}, nil
				}
			})
			room := New([]Member{{ID: ModeratorID, Enabled: true, Adapter: moderator}, {ID: "worker", Enabled: true, Adapter: worker}}, Options{})
			defer room.Close()
			if err := room.Discuss("Topic", "Find an answer", ""); err != nil {
				t.Fatal(err)
			}
			snapshot := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
			last := snapshot.Messages[len(snapshot.Messages)-1]
			if moderatorCalls.Load() != 1 || last.Task.Status != Failed || last.Task.Stage == Conclusion || snapshot.Error == "" || !last.InOverview() || last.Task.NoAnswer != strings.HasSuffix(scenario, "tool only") {
				t.Fatal(snapshot)
			}
		})
	}
}

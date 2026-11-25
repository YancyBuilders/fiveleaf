package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func flowAwaitDiscussion(t *testing.T, room *roundtable.Room) roundtable.Snapshot {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		if snapshot := room.Snapshot(); !snapshot.Busy {
			if snapshot.Error != "" {
				t.Fatal(snapshot.Error)
			}
			return snapshot
		}
		select {
		case <-room.Events():
		case <-timeout:
			t.Fatal("timed out waiting for the discussion")
		}
	}
}

func flowTester(t *testing.T, room *roundtable.Room, snapshot *roundtable.Snapshot, graph *roundtable.Graph, latest *bool) (*roundtableWindow, *ui.Tester) {
	t.Helper()
	hs := newHarness(t, true)
	window := &roundtableWindow{a: hs.a, pal: hs.a.pal, meeting: &agentMeeting{room: room, speakers: []meetingSpeaker{
		{Participant: roundtable.Participant{ID: roundtable.ModeratorID, Name: "Chair"}},
		{Participant: roundtable.Participant{ID: "preset:test", Name: "Tester"}},
		{Participant: roundtable.Participant{ID: "role:Field", Name: "Field"}},
		{Participant: roundtable.Participant{ID: "role:Field analyst", Name: "Field analyst"}},
	}}}
	tester := ui.NewTester(func(c *ui.Context) {
		window.a.drain()
		ui.Scroll(c).Fill().TrackScroll(&window.scroll).Children(func() { window.messageFlow(c, *snapshot, *graph, *latest) })
	}, 650, 1400)
	step(tester)
	return window, tester
}

func TestRoundtableFlowShowsSenderAboveMessage(t *testing.T) {
	for _, example := range []struct {
		name, sender, body string
		message            roundtable.Message
	}{
		{"user", locales.RoundtableHuman(), "Check the published criteria", roundtable.Message{ID: "request", Role: "user", Text: "Check the published criteria"}},
		{"human conclusion", locales.RoundtableHuman(), locales.RoundtableConclusion(), roundtable.Message{ID: "human-end", Role: "user", Text: "Use the revised criteria", HumanConclusion: true}},
		{"moderator dispatch", "Chair", "@preset:test Check the table", roundtable.Message{ID: "dispatch", Role: "assistant", AgentID: roundtable.ModeratorID, Text: "@preset:test Check the table", Task: &roundtable.Task{Stage: roundtable.Coordinate, Status: roundtable.Completed}}},
		{"moderator conclusion", "Chair", locales.RoundtableConclusion(), roundtable.Message{ID: "ai-end", Role: "assistant", AgentID: roundtable.ModeratorID, Text: "Use the finished checklist", Task: &roundtable.Task{Stage: roundtable.Conclusion, Status: roundtable.Completed}}},
	} {
		t.Run(example.name, func(t *testing.T) {
			snapshot := roundtable.Snapshot{Messages: []roundtable.Message{example.message}}
			graph, latest := roundtable.BuildTaskGraph(snapshot), false
			_, tester := flowTester(t, nil, &snapshot, &graph, &latest)
			if tester.HasText(locales.RoundtableAIChairTag()) != (example.message.AgentID == roundtable.ModeratorID) || tester.HasText(locales.RoundtableChairTag()) != example.message.HumanConclusion {
				t.Fatal("chair badge does not match the stored sender", tester.Texts())
			}
			author, hasAuthor := tester.Find(example.sender)
			body, hasBody := tester.Find(example.body)
			if !hasAuthor || !hasBody || author.Y+author.H > body.Y {
				t.Fatalf("sender %q must precede its body %q: sender %+v, body %+v\n%v", example.sender, example.body, author, body, tester.Texts())
			}
		})
	}
}

func TestRoundtableFlowRuntimeDiscussion(t *testing.T) {
	const assignment = "Read **both** records"
	const result = "## Decision\n\nKeep **both** records and ask @review for a follow-up.\n\n```go\nlookup()\n```"
	moderatorCalls := 0
	moderator := roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
		moderatorCalls++
		if moderatorCalls == 1 {
			return roundtable.Message{Text: "@preset:test\n" + assignment}, nil
		}
		roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "find", Kind: "search", Title: "Find reference", Status: "completed", Note: "Reference points to the same record."})
		roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "failed", Kind: "read", Status: "failed", Detail: "Unverified record"})
		return roundtable.Message{Text: result}, nil
	})
	worker := roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
		roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "read", Kind: "read", Title: "Read record", Status: "completed", Detail: "Observed value", Note: "Full inspection note"})
		return roundtable.Message{Text: "Both records agree."}, nil
	})
	room := roundtable.New([]roundtable.Member{
		{ID: roundtable.ModeratorID, Name: "Chair", Enabled: true, Adapter: moderator},
		{ID: "preset:test", Name: "Tester", Enabled: true, Adapter: worker},
	}, roundtable.Options{})
	t.Cleanup(room.Close)
	if err := room.AddMemory("Compare both records.", ""); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss("Review", "Check report.md. Then compare the results.", ""); err != nil {
		t.Fatal(err)
	}
	snapshot := flowAwaitDiscussion(t, room)
	if len(snapshot.Messages) != 4 || snapshot.Messages[1].Task.Stage != roundtable.Coordinate || snapshot.Messages[2].Task.Stage != roundtable.Work {
		t.Fatal(snapshot.Messages)
	}
	graph, latest := roundtable.BuildTaskGraph(snapshot), true
	window, tester := flowTester(t, room, &snapshot, &graph, &latest)
	for _, label := range []string{locales.RoundtableTopicDivider(1, "Check report.md"), locales.RoundtableAIChairTag(), locales.RoundtableGraphRoleShort("test"), locales.RoundtableReadOnly(),
		locales.RoundtableRound(3), locales.RoundtableMemoryCited(1), locales.RoundtableDecision(), locales.RoundtableNoMoreHandoffs(), locales.RoundtableEvidence(), locales.RoundtableCorroborated(),
		locales.RoundtableEvidenceHit(), "Read record", "Observed value", "Find reference", locales.RoundtableClosing(), locales.RoundtableRoundClosed(), locales.RoundtableConclusionSaved()} {
		if !tester.HasText(label) {
			t.Errorf("missing %q", label)
		}
	}
	if tester.HasText("Unverified record") || tester.HasText("Full inspection note") {
		t.Fatal("unconfirmed or undisclosed evidence visible", tester.Texts())
	}
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		step(tester)
	}
	click("Read record")
	if !tester.HasText("Full inspection note") {
		t.Fatal("opening evidence did not reveal the complete tool record")
	}
	click("Read record")
	if tester.HasText("Full inspection note") {
		t.Fatal("closing evidence left the note visible")
	}
	click("Copy code")
	if tester.Clipboard() != "lookup()\n" {
		t.Fatalf("conclusion Markdown code copied %q", tester.Clipboard())
	}
	click(locales.RoundtableDispatchTarget("Chair", "Tester"))
	if window.graphSel != snapshot.Messages[2].ID || window.drawer != meetingDrawerNode || window.memberID != "" {
		t.Fatalf("dispatch selected %q, drawer %v, member %q", window.graphSel, window.drawer, window.memberID)
	}
	for _, width := range []int{1240, 1040, 860, 310} {
		tester.SetSize(width, 1400)
		for _, dark := range []bool{false, true} {
			window.pal = lightPalette().derive()
			if dark {
				window.pal = darkPalette().derive()
			}
			tester.SetDark(dark)
			step(tester)
			for _, label := range []string{locales.RoundtableDispatchTarget("Chair", "Tester"), locales.RoundtableConclusion(), "Read record", locales.RoundtableConclusionSaved()} {
				bounds, found := tester.Find(label)
				if !found || bounds.W <= 0 || bounds.X < -1 || bounds.X+bounds.W > float32(width+1) || window.scroll.MaxX > 1 {
					t.Fatalf("%q at width %d, dark %t: %+v, scroll %v", label, width, dark, bounds, window.scroll.MaxX)
				}
			}
		}
	}
	snapshot.Messages = snapshot.Messages[2:3]
	step(tester)
	answer, ok := tester.Find("Both records agree.")
	badge, found := tester.Find(locales.RoundtableMemoryChip(1))
	if !found || !ok || badge.Y < answer.Y+answer.H-1 {
		t.Fatalf("memory metadata precedes the answer: %+v %+v", answer, badge)
	}
}

func TestRoundtableFlowDispatchAcrossPageBoundary(t *testing.T) {
	const worker = "role:Field analyst"
	send := roundtable.Message{ID: "send", Role: "assistant", Name: "Chair", AgentID: roundtable.ModeratorID, Hop: 1, Text: "**@role:Field analyst**\n\nCheck `records`", Task: &roundtable.Task{Stage: roundtable.Coordinate, Status: roundtable.Completed, TurnID: "first"}}
	receive := roundtable.Message{ID: "receive", Role: "assistant", Name: "Field analyst", AgentID: worker, Hop: 2, Text: "Reviewed", Task: &roundtable.Task{Stage: roundtable.Work, Status: roundtable.Completed, TurnID: "first", Assignment: "Check records"}}
	later := receive
	later.ID, later.Task = "later", &roundtable.Task{Stage: roundtable.Work, Status: roundtable.Completed, TurnID: "another", Assignment: receive.Task.Assignment}
	snapshot := roundtable.Snapshot{Messages: []roundtable.Message{send}}
	graph := roundtable.BuildTaskGraph(roundtable.Snapshot{Messages: []roundtable.Message{send, receive, later}})
	latest := false
	window, tester := flowTester(t, nil, &snapshot, &graph, &latest)
	label := locales.RoundtableDispatchTarget("Chair", "Field analyst")
	if err := tester.Click(label); err != nil {
		t.Fatal(err)
	}
	step(tester)
	if window.graphSel != receive.ID {
		t.Fatal("Markdown mention on older page selected", window.graphSel)
	}
	window.graphSel = ""
	graph = roundtable.BuildTaskGraph(roundtable.Snapshot{Messages: []roundtable.Message{send, later}})
	step(tester)
	if !tester.HasText(label) {
		t.Fatal("dispatch disappeared when its receiving record was missing")
	}
	_ = tester.Click(label)
	step(tester)
	if window.graphSel != "" {
		t.Fatal("dispatch opened an unrelated task with the same assignee, hop and assignment")
	}
}

func TestRoundtableFlowFooterAndEvidenceScope(t *testing.T) {
	start := time.Unix(1700000000, 0)
	old := roundtable.Message{ID: "before", Role: "assistant", AgentID: "preset:test", Hop: 1, Time: start.Add(-24 * time.Hour), Text: "Previous work", Task: &roundtable.Task{Stage: roundtable.Direct, Status: roundtable.Completed, TurnID: "old", Tools: []roundtable.ToolRecord{{Kind: "edit", Status: roundtable.Completed, Detail: "Evidence for the previous turn"}}}}
	closing := roundtable.Message{ID: "last", Role: "assistant", AgentID: roundtable.ModeratorID, Hop: 1, Time: start, Text: "Closed", Task: &roundtable.Task{Stage: roundtable.Conclusion, Status: roundtable.Completed, TurnID: "current", Finished: start.Add(2 * time.Second)}}
	graph := roundtable.BuildTaskGraph(roundtable.Snapshot{Messages: []roundtable.Message{old, closing}})
	snapshot, latest := roundtable.Snapshot{Messages: []roundtable.Message{closing}}, true
	_, tester := flowTester(t, nil, &snapshot, &graph, &latest)
	stats := strings.Join([]string{locales.RoundtableGraphCountsNodes(1), locales.RoundtableGraphCountsHandoffs(0), meetingDuration(2 * time.Second)}, " · ")
	if !tester.HasText(locales.RoundtableRoundClosed()) || !tester.HasText(stats) || tester.HasText(locales.RoundtableEvidence()) || tester.HasText("Evidence for the previous turn") {
		t.Fatal("terminal footer or evidence scope is wrong", tester.Texts())
	}
	for _, scenario := range []string{"older page", "busy", "new user", "member reply", "failed", "no answer", "human conclusion"} {
		snapshot, latest = roundtable.Snapshot{Messages: []roundtable.Message{closing}}, true
		switch scenario {
		case "older page":
			latest = false
		case "busy":
			snapshot.Busy = true
		case "new user":
			snapshot.Messages = append(snapshot.Messages, roundtable.Message{ID: "new", Role: "user", Text: "Follow up"})
		case "member reply":
			snapshot.Messages = append(snapshot.Messages, old)
		case "failed":
			snapshot.Messages[0].Task = &roundtable.Task{Stage: roundtable.Conclusion, Status: roundtable.Failed}
		case "no answer":
			snapshot.Messages[0].Task = &roundtable.Task{Stage: roundtable.Conclusion, Status: roundtable.Completed, NoAnswer: true}
		case "human conclusion":
			snapshot.Messages[0] = roundtable.Message{ID: "human-end", Role: "user", Text: "Closing remarks", HumanConclusion: true}
		}
		step(tester)
		if tester.HasText(locales.RoundtableRoundClosed()) {
			t.Errorf("%s showed the terminal footer", scenario)
		}
		if scenario == "human conclusion" && !tester.HasText(locales.RoundtableConclusion()) {
			t.Error("published human conclusion lost its card")
		}
	}
}

func TestRoundtableFlowLongConclusionMarkdown(t *testing.T) {
	text := strings.Repeat("A recorded observation. ", 48) + "\n\n## Final finding\n\n```go\ncheck(record)\n```"
	if meetingReadingPreview(text) == "" {
		t.Fatal("fixture must qualify for a reading preview")
	}
	message := roundtable.Message{ID: "closing", Role: "assistant", AgentID: roundtable.ModeratorID, Text: text, Task: &roundtable.Task{Stage: roundtable.Conclusion, Status: roundtable.Completed}}
	snapshot := roundtable.Snapshot{Messages: []roundtable.Message{message}}
	graph, latest := roundtable.BuildTaskGraph(snapshot), true
	window, tester := flowTester(t, nil, &snapshot, &graph, &latest)
	if !tester.HasText("Final finding") || tester.HasText(locales.RoundtableFullResult()) {
		t.Fatal("long conclusion lost its final Markdown section", tester.Texts())
	}
	if err := tester.Click("Copy code"); err != nil || tester.Clipboard() != "check(record)\n" {
		t.Fatalf("copy from long conclusion: %v, %q", err, tester.Clipboard())
	}
	snapshot.Messages[0].Task = &roundtable.Task{Stage: roundtable.Direct, Status: roundtable.Completed}
	step(tester)
	if !tester.HasText(locales.RoundtableFullResult()) || tester.HasText("Final finding") {
		t.Fatal("ordinary long reply lost its preview")
	}
	window.showFullReplies = true
	step(tester)
	if !tester.HasText("Final finding") || tester.HasText(locales.RoundtableFullResult()) {
		t.Fatal("expanded-replies setting did not show ordinary Markdown")
	}
}

func TestRoundtableFlowArchivedTopicNumbers(t *testing.T) {
	library, err := roundtable.ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worker := roundtable.Member{ID: "worker", Name: "Worker", Enabled: true, Adapter: roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
		return roundtable.Message{Text: "Recorded reply."}, nil
	})}
	group, err := library.CreateWithChair("Archive", []roundtable.Participant{{ID: worker.ID, Name: worker.Name, Provider: "local"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	room, err := library.OpenRoom(group, []roundtable.Member{worker})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(room.Close)
	const total = roundtable.MaxHistoryMessages + 5
	for number := range total {
		target := ""
		if number == 4 {
			target = worker.ID
		}
		if err := room.Discuss(group.Topic, fmt.Sprintf("Question %d。Following sentence.", number), target); err != nil {
			t.Fatal(err)
		}
		if target != "" {
			flowAwaitDiscussion(t, room)
		}
	}
	full := room.Snapshot()
	if !full.HasEarlier || full.Messages[0].Text != "Question 5。Following sentence." {
		t.Fatal("fixture did not cross the archive boundary")
	}
	page, latest := full, true
	page.Messages = full.Messages[len(full.Messages)-meetingPageSize:]
	graph := roundtable.BuildTaskGraph(full)
	window, tester := flowTester(t, room, &page, &graph, &latest)
	lastTitle := locales.RoundtableTopicDivider(total-1, fmt.Sprintf("Question %d", total-1))
	deadline := time.Now().Add(3 * time.Second)
	for !tester.HasText(lastTitle) && time.Now().Before(deadline) {
		step(tester)
	}
	if !tester.HasText(lastTitle) {
		t.Fatal("latest page lost the persisted topic count", tester.Texts())
	}
	window.loadPage([]roundtable.Message{full.Messages[meetingPageSize]})
	latest, page.Messages = false, window.page
	step(tester)
	if tester.HasText(locales.RoundtableTopicDivider(5, "Question 5")) || !tester.HasText(locales.RoundtableTopicDivider(6, "Question 6")) {
		t.Fatal("first row on a page became a false topic after an assistant message", tester.Texts())
	}
	window.loadPage([]roundtable.Message{full.Messages[0]})
	page.Messages = window.page
	step(tester)
	if !tester.HasText(locales.RoundtableTopicDivider(1, "Question 0")) || !tester.HasText(locales.RoundtableTopicDivider(5, "Question 4")) {
		t.Fatal("older archive page lost its original topic boundaries", tester.Texts())
	}
}

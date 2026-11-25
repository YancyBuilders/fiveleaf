package main

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"testing"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestAgentTurnRailAtPaneEdge(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 1520)
	chat := hs.a.agent
	chat.messages = []*agentMessage{{role: "user", text: "First question"}, {role: "assistant", text: "First response"}}
	hs.settle()
	firstReply, _ := hs.tt.Find("First response")
	chat.messages = append(chat.messages, &agentMessage{role: "user", text: "Second question"})
	hs.settle()
	pane := hs.a.activeTab().tool.pane.Bounds(hs.a.review.ctx)
	rail, found := hs.tt.Find(locales.AgentTurnNavigation())
	reply, _ := hs.tt.Find("First response")
	draft := chat.composeBox.Bounds(hs.a.review.ctx)
	if !found || rail.X < pane.X || rail.X > pane.X+20 || reply.X < rail.X+rail.W+100 || reply != firstReply {
		t.Fatalf("wide pane did not keep the rail at its edge and the reply in place: pane=%v rail=%v reply=%v before=%v", pane, rail, reply, firstReply)
	}
	if delta := draft.X + draft.W/2 - pane.X - pane.W/2; delta < -1 || delta > 1 || draft.W > 850 {
		t.Fatalf("composer did not keep its centered reading width: pane=%v draft=%v", pane, draft)
	}
	hs.tt.Move(rail.X+rail.W/2, rail.Y+rail.H/4)
	hs.settle()
	card, found := hs.tt.Find(locales.AgentTurnPreview())
	if !found || card.X < rail.X+rail.W || card.X+card.W >= reply.X {
		t.Fatalf("preview did not use the available gutter: card=%v rail=%v reply=%v", card, rail, reply)
	}
}

func TestAgentTurnRailRestAndHover(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	chat := hs.a.agent
	for number := 1; number <= 11; number++ {
		chat.messages = append(chat.messages, &agentMessage{role: "user", text: fmt.Sprintf("Question %d", number)},
			&agentMessage{role: "assistant", text: fmt.Sprintf("Answer %d.", number)})
	}
	hs.settle()
	marker := func(number int) ui.Rect {
		t.Helper()
		bounds, found := hs.tt.Find(locales.AgentTurnNumber(strconv.Itoa(number)))
		if !found {
			t.Fatalf("missing marker %d", number)
		}
		return bounds
	}
	inkWidth := func(pixels *image.RGBA, bounds ui.Rect) int {
		row := int(bounds.Y + bounds.H/2)
		width := 0
		for column := int(bounds.X) + 3; column < int(bounds.X+bounds.W)-2; column++ {
			background := pixels.RGBAAt(column, row+3)
			if int(background.R)-int(pixels.RGBAAt(column, row).R) > 12 {
				width++
			}
		}
		return width
	}
	center := marker(6)
	replyBefore, _ := hs.tt.Find("Answer 11.")
	resting := hs.tt.Image()
	short := inkWidth(resting, center)
	if short < 3 || short > 9 || inkWidth(resting, marker(11)) != short || inkWidth(resting, marker(10)) != short {
		t.Fatalf("resting turn marks should all be short: middle=%d latest=%d neighbor=%d", short, inkWidth(resting, marker(11)), inkWidth(resting, marker(10)))
	}
	hs.tt.Move(center.X+center.W/2, center.Y+center.H/2)
	hs.settle()
	hover := hs.tt.Image()
	longest, neighbor := inkWidth(hover, center), inkWidth(hover, marker(5))
	replyAfter, _ := hs.tt.Find("Answer 11.")
	if longest <= neighbor || neighbor <= short || inkWidth(hover, marker(1)) != short || marker(6) != center || replyAfter != replyBefore {
		t.Fatalf("hover changed more than nearby ink: widths=%d/%d/%d mark=%v reply=%v", longest, neighbor, short, marker(6), replyAfter)
	}
	card, found := hs.tt.Find(locales.AgentTurnPreview())
	if !found {
		t.Fatal("hover did not open the preview")
	}
	hs.tt.Move(card.X+card.W/2, card.Y+card.H/2)
	hs.settle()
	if !hs.tt.HasText(locales.AgentTurnPreview()) || chat.readingPrompt != nil {
		t.Fatal("moving into the preview lost the turn or jumped the transcript")
	}
	hs.tt.Move(1, 1)
	hs.settle()
	if hs.tt.HasText(locales.AgentTurnPreview()) || inkWidth(hs.tt.Image(), center) != short {
		t.Fatal("leaving the preview did not restore the resting rail")
	}
	markTone := func(pixels *image.RGBA, number int) uint8 {
		bounds := marker(number)
		return pixels.RGBAAt(int(bounds.X)+18, int(bounds.Y+bounds.H/2)).R
	}
	offscreen, visible := markTone(resting, 2), markTone(resting, 10)
	view := chat.scroll.Bounds(hs.a.review.ctx)
	hs.tt.Scroll(view.X+view.W/2, view.Y+view.H/2, 0, -4000)
	hs.settle()
	atStart := hs.tt.Image()
	if offscreen <= visible || markTone(atStart, 2) != visible || markTone(atStart, 10) != offscreen {
		t.Fatal("the highlighted turns did not follow the visible transcript")
	}
}

func TestAgentTurnRailPreviewAndJump(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.draft = "Keep this unsent question"
	chat.messages = []*agentMessage{{role: "connection", text: "Ready"}, nil}
	for number := 1; number <= 5; number++ {
		chat.messages = append(chat.messages,
			&agentMessage{role: "user", text: fmt.Sprintf("Question %d?", number), historical: number <= 2},
			&agentMessage{role: "thought", text: "Thought details"},
			&agentMessage{role: "tool", tool: &agentTool{title: "Read file"}},
			&agentMessage{role: "assistant", text: fmt.Sprintf("Answer %d.\n\n", number) + strings.Repeat("A longer paragraph.\n\n", number*3)})
	}
	hs.settle()
	if _, found := hs.tt.Find(locales.AgentTurnNumber("6")); found {
		t.Fatal("a non-user message was counted as a turn")
	}
	marker, found := hs.tt.Find(locales.AgentTurnNumber("2"))
	if !found || chat.scroll.Y <= 0 || chat.scroll.Y != chat.scroll.MaxY {
		t.Fatalf("missing turn marker or chat not at end: marker=%+v scroll=%+v", marker, chat.scroll)
	}
	before := chat.scroll.Y
	hs.tt.Move(marker.X+marker.W/2, marker.Y+marker.H/2)
	hs.settle()
	if !hs.tt.HasText(locales.AgentTurnPreview()) || !hs.tt.HasText("Answer 2. A longer paragraph.") || chat.scroll.Y != before {
		t.Fatal("hover did not preview this turn's public reply without scrolling")
	}
	if err := hs.tt.Click(locales.AgentTurnNumber("2")); err != nil {
		t.Fatal(err)
	}
	hs.tt.Move(1, 1)
	hs.settle()
	checkPromptAtTop := func(number int) {
		t.Helper()
		text, found := hs.tt.Find(fmt.Sprintf("Question %d?", number))
		view := chat.scroll.Bounds(hs.a.review.ctx)
		if !found || text.H <= 0 || text.Y < view.Y || text.Y > view.Y+36 {
			t.Fatalf("turn %d did not align its prompt to the top: prompt=%+v viewport=%+v", number, text, view)
		}
	}
	checkPromptAtTop(2)
	if hs.tt.HasText(locales.AgentTurnPreview()) {
		t.Fatal("preview stayed open after the pointer left")
	}
	reading := chat.scroll.Y
	chat.messages[len(chat.messages)-1].text += "One more streamed paragraph."
	hs.settle()
	if chat.scroll.Y != reading || chat.draft != "Keep this unsent question" {
		t.Fatalf("new output moved the reader or changed the draft: %v -> %v, draft=%q", reading, chat.scroll.Y, chat.draft)
	}
	marker, _ = hs.tt.Find(locales.AgentTurnNumber("2"))
	hs.tt.Move(marker.X+marker.W/2, marker.Y+marker.H/2)
	hs.key(0, ui.KeyDown)
	if !hs.tt.Focused(locales.AgentTurnNumber("3")) || !hs.tt.HasText("Answer 3. A longer paragraph.") || chat.scroll.Y != reading {
		t.Fatal("the down arrow did not preview the focused turn without jumping")
	}
	hs.key(0, ui.KeyEnter)
	hs.tt.Move(1, 1)
	hs.settle()
	checkPromptAtTop(3)
	if err := hs.tt.Click(locales.AgentJumpLatest()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if chat.scroll.Y != chat.scroll.MaxY {
		t.Fatal("jump to latest did not restore following")
	}
}

func TestAgentTurnRailLongHistory(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 320)
	chat := hs.a.agent
	chat.messages = []*agentMessage{{role: "user", text: "First question"}}
	hs.settle()
	if hs.tt.HasText(locales.AgentTurnNavigation()) {
		t.Fatal("a single turn reserved a navigation rail")
	}
	for number := 2; number <= 90; number++ {
		chat.messages = append(chat.messages, &agentMessage{role: "user", text: "Question " + strconv.Itoa(number)})
	}
	hs.settle()
	rail, found := hs.tt.Find(locales.AgentTurnNavigation())
	last, hasLast := hs.tt.Find(locales.AgentTurnNumber("90"))
	if !found || !hasLast || last.Y < rail.Y || last.Y+last.H > rail.Y+rail.H+1 {
		t.Fatalf("the latest turn is not reachable in the rail: marker=%+v rail=%+v", last, rail)
	}
	before := chat.scroll.Y
	hs.tt.Scroll(rail.X+rail.W/2, rail.Y+rail.H/2, 0, -4000)
	hs.settle()
	first, hasFirst := hs.tt.Find(locales.AgentTurnNumber("1"))
	if !hasFirst || first.Y < rail.Y-1 || first.Y+first.H > rail.Y+rail.H || chat.scroll.Y != before {
		t.Fatalf("scrolling the rail did not independently reach the first turn: marker=%+v rail=%+v", first, rail)
	}
	hs.tt.Move(first.X+first.W/2, first.Y+first.H/2)
	hs.settle()
	card, found := hs.tt.Find(locales.AgentTurnPreview())
	view := chat.scroll.Bounds(hs.a.review.ctx)
	if !found || card.X < first.X+first.W || card.X+card.W > view.X+view.W || card.Y < view.Y || card.Y+card.H > view.Y+view.H {
		t.Fatalf("preview escaped its narrow chat: card=%+v marker=%+v viewport=%+v", card, first, view)
	}
	if err := hs.tt.Click(locales.AgentTurnNumber("1")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if chat.scroll.Y != 0 {
		t.Fatalf("first turn did not jump to the beginning: %v", chat.scroll.Y)
	}
}

func TestAgentTurnRailKeepsVisibleOlderTurn(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	reply := &agentMessage{role: "assistant", text: "Working on the third question."}
	chat.messages = []*agentMessage{
		{role: "user", text: "First question"},
		{role: "user", text: "Second question"},
		{role: "user", text: "Third question"}, reply,
	}
	hs.settle()
	if chat.scroll.MaxY != 0 {
		t.Fatal("fixture does not fit the viewport")
	}
	if err := hs.tt.Click(locales.AgentTurnNumber("2")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	reply.text += strings.Repeat("\n\nSubsequent output.", 80)
	hs.settle()
	view := chat.scroll.Bounds(hs.a.review.ctx)
	question, found := hs.tt.Find("Second question")
	if chat.scroll.MaxY <= 0 || chat.scroll.Y == chat.scroll.MaxY || !found || question.Y < view.Y || question.Y+question.H > view.Y+view.H || !hs.tt.HasText(locales.AgentJumpLatest()) {
		t.Fatalf("the stream pulled an already-visible selected turn away: prompt=%+v view=%+v scroll=%+v", question, view, chat.scroll)
	}
	if err := hs.tt.Click(locales.AgentJumpLatest()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	reply.text += strings.Repeat("\n\nFollowing again.", 10)
	hs.settle()
	if chat.scroll.Y != chat.scroll.MaxY || hs.tt.HasText(locales.AgentJumpLatest()) {
		t.Fatal("returning to latest did not release the selected turn")
	}
}

func TestAgentTurnRailKeepsLastPromptAcrossReflow(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 740)
	chat := hs.a.agent
	reply := &agentMessage{role: "assistant", text: "Just started."}
	chat.messages = []*agentMessage{
		{role: "user", text: "First question"},
		{role: "assistant", text: strings.Repeat("Earlier text that wraps at the width of the chat. ", 90)},
		{role: "user", text: "Last question"}, reply,
	}
	hs.settle()
	if err := hs.tt.Click(locales.AgentTurnNumber("2")); err != nil {
		t.Fatal(err)
	}
	hs.tt.Move(1, 1)
	hs.settle()
	reply.text += strings.Repeat("\n\nThe response keeps growing.", 45)
	hs.settle()
	checkAnchor := func() {
		t.Helper()
		prompt, found := hs.tt.Find("Last question")
		view := chat.scroll.Bounds(hs.a.review.ctx)
		if !found || prompt.H <= 0 || prompt.Y < view.Y || prompt.Y > view.Y+36 {
			t.Fatalf("selected last prompt lost its reading position: prompt=%+v view=%+v scroll=%+v", prompt, view, chat.scroll)
		}
	}
	checkAnchor()
	sizeChatWorkspace(hs, 320)
	hs.settle()
	checkAnchor()
	before := chat.scroll.Y
	view := chat.scroll.Bounds(hs.a.review.ctx)
	hs.tt.Scroll(view.X+view.W/2, view.Y+view.H/2, 0, 140)
	hs.settle()
	if chat.scroll.Y <= before || chat.readingPrompt != nil {
		t.Fatal("manual scrolling did not release the selected prompt")
	}
}

func TestAgentTurnRailSplitChat(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	left := hs.a.agent
	hs.a.splitRight()
	right := hs.a.agent
	hs.a.window.SidebarClosed = true
	hs.tt.SetSize(780, 520)
	for _, pane := range []struct {
		chat   *agentChat
		prefix string
		turns  int
	}{{left, "Left", 3}, {right, "Right", 6}} {
		for number := 1; number <= pane.turns; number++ {
			pane.chat.messages = append(pane.chat.messages,
				&agentMessage{role: "user", text: fmt.Sprintf("%s question %d?", pane.prefix, number)},
				&agentMessage{role: "assistant", text: strings.Repeat(pane.prefix+" response.\n\n", 8)})
		}
	}
	hs.settle()
	before := left.scroll.Y
	mark, found := hs.tt.Find(locales.AgentTurnNumber("4"))
	if !found || before <= 0 {
		t.Fatal("missing split conversation history")
	}
	hs.tt.Move(mark.X+mark.W/2, mark.Y+mark.H/2)
	hs.settle()
	card, found := hs.tt.Find(locales.AgentTurnPreview())
	view := right.scroll.Bounds(hs.a.review.ctx)
	if !found || card.X < view.X || card.X+card.W > view.X+view.W || card.Y < view.Y || card.Y+card.H > view.Y+view.H {
		t.Fatalf("preview escaped the right split: card=%+v view=%+v", card, view)
	}
	if err := hs.tt.Click(locales.AgentTurnNumber("4")); err != nil {
		t.Fatal(err)
	}
	hs.tt.Move(1, 1)
	hs.settle()
	prompt, found := hs.tt.Find("Right question 4?")
	if left.scroll.Y != before || !found || prompt.Y < view.Y || prompt.Y > view.Y+36 {
		t.Fatalf("jump crossed splits or missed the prompt: left=%v (was %v), target=%+v viewport=%+v", left.scroll.Y, before, prompt, view)
	}
}

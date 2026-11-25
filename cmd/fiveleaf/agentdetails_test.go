package main

import (
	"image"
	"os"
	"strings"
	"testing"
	"time"

	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func (hs *harness) clickDetail(label string) {
	hs.t.Helper()
	if err := hs.tt.Click(label); err != nil {
		hs.t.Fatal(err)
	}
	hs.settle()
}

func (hs *harness) detailBounds(label string) ui.Rect {
	hs.t.Helper()
	bounds, found := hs.tt.Find(label)
	if !found || bounds.W <= 0 || bounds.H <= 0 {
		hs.t.Fatalf("missing or empty %q: %+v", label, bounds)
	}
	return bounds
}

func TestChatDetailsFloatAtEveryWidth(t *testing.T) {
	hs := newHarness(t, true)
	sizeChatWorkspace(hs, 1100)
	agentViewFixture(hs)
	hs.settle()
	chat := hs.a.agent
	hs.tt.Type("Keep this draft")
	for _, width := range []float32{1100, 390, 200} {
		sizeChatWorkspace(hs, width)
		hs.settle()
		if hs.tt.HasText(locales.AgentChatSourcesEmpty()) {
			t.Fatalf("details already open at width %v", width)
		}
		input := hs.detailBounds(locales.AgentDraft())
		view := chat.scroll.Bounds(hs.a.review.ctx)
		pane := hs.a.activeTab().tool.pane.Bounds(hs.a.review.ctx)
		if input.Y+input.H < pane.Y+pane.H-90 {
			t.Fatalf("draft is above the bottom of the chat: draft=%+v pane=%+v", input, pane)
		}
		hs.clickDetail(locales.AgentChatDetails())
		close := hs.detailBounds(locales.AgentHideChatDetails())
		if hs.detailBounds(locales.AgentDraft()) != input || chat.scroll.Bounds(hs.a.review.ctx) != view || !pane.Contains(close.X+close.W-1, close.Y) {
			t.Fatalf("opening details changed the chat layout or put its close button outside the pane at width %v", width)
		}
		if !hs.tt.HasText(locales.AgentChatSourcesEmpty()) || !hs.tt.HasText(locales.ScmNoRepo()) || hs.tt.HasText("+0") {
			t.Fatal("details misreported the empty repository or sources")
		}
		hs.key(0, ui.KeyEscape)
		hs.settle()
		if hs.tt.HasText(locales.AgentChatSourcesEmpty()) || chat.draft != "Keep this draft" || !hs.tt.Focused(locales.AgentDraft()) {
			t.Fatal("Esc did not dismiss details and preserve the focused draft")
		}
	}
	hs.clickDetail(locales.AgentChatDetails())
	hs.clickDetail(locales.AgentDraft())
	hs.key(0, ui.KeyEnd)
	hs.tt.Type(" while reading details")
	hs.tt.SetFocused(false)
	hs.tt.SetFocused(true)
	hs.settle()
	if !hs.tt.HasText(locales.AgentChatAddSource()) {
		t.Fatal("typing in the chat or changing window focus dismissed the card")
	}
	if chat.draft != "Keep this draft while reading details" {
		t.Fatalf("typing next to the card left draft %q", chat.draft)
	}
}

func TestChatDetailsDragFollowsPointerThroughRelease(t *testing.T) {
	hs := newHarness(t, false)
	sizeChatWorkspace(hs, 1100)
	agentViewFixture(hs)
	hs.settle()
	hs.clickDetail(locales.AgentChatDetails())
	initial := hs.detailBounds(locales.AgentChatMoveDetails())
	pressX, pressY := initial.X+initial.W/2, initial.Y+initial.H/2
	hs.tt.Press(pressX, pressY)
	for _, offset := range []ui.Point{{X: -20, Y: 12}, {X: -70, Y: 60}, {X: 80, Y: -20}, {X: 30, Y: -10}, {X: -34, Y: 8}} {
		hs.tt.Move(pressX+offset.X, pressY+offset.Y)
		hs.tt.Frame()
		got := hs.detailBounds(locales.AgentChatMoveDetails())
		if got.X != initial.X+min(0, offset.X) || got.Y != initial.Y+max(0, offset.Y) {
			t.Fatalf("card lost its grab point at offset %v: initial=%v got=%v", offset, initial, got)
		}
	}
	hs.tt.Release(pressX-95, pressY+70)
	hs.tt.Move(pressX, pressY)
	hs.settle()
	if got := hs.detailBounds(locales.AgentChatMoveDetails()); got.X != initial.X-95 || got.Y != initial.Y+70 {
		t.Fatalf("card did not stay at the release position: initial=%v got=%v", initial, got)
	}
}

func TestChatDetailsDragFromWholeTitleBar(t *testing.T) {
	hs := newHarness(t, false)
	sizeChatWorkspace(hs, 1100)
	agentViewFixture(hs)
	hs.settle()
	hs.clickDetail(locales.AgentChatDetails())
	closeBounds := func() ui.Rect {
		t.Helper()
		return hs.detailBounds(locales.AgentHideChatDetails())
	}
	initial := closeBounds()
	draft := hs.detailBounds(locales.AgentDraft())
	pane := hs.a.activeTab().tool.pane.Bounds(hs.a.review.ctx)
	metrics := hs.a.pal.metricsOr()
	left := pane.X + pane.W - metrics.gap - chatDetailsWidth
	for _, fromLeft := range []float32{3, metrics.pad + 3, chatDetailsWidth / 2, chatDetailsWidth - 3} {
		before := closeBounds()
		pressX := left + (before.X - initial.X) + fromLeft
		pressY := before.Y + before.H/2
		hs.tt.Press(pressX, pressY)
		hs.tt.Move(pressX-16, pressY+8)
		hs.tt.Frame()
		if got := closeBounds(); got.X != before.X-16 || got.Y != before.Y+8 {
			t.Fatalf("drag from title x=%v missed the first move: %v → %v", fromLeft, before, got)
		}
		hs.tt.Release(pressX-16, pressY+8)
	}
	moved := closeBounds()
	if hs.detailBounds(locales.AgentDraft()) != draft {
		t.Fatal("dragging the card moved the chat draft")
	}
	chat := hs.a.agent
	hs.clickDetail(locales.AgentNewSession())
	if hs.tt.HasText(locales.AgentChatMoveDetails()) {
		t.Fatal("a new chat inherited the open card")
	}
	hs.a.openChatPage(chat)
	hs.settle()
	if got := closeBounds(); got != moved {
		t.Fatalf("switching back to the chat lost the card position: %v → %v", moved, got)
	}
	hs.clickDetail(locales.AgentHideChatDetails())
	hs.clickDetail(locales.AgentChatDetails())
	if got := closeBounds(); got != moved {
		t.Fatalf("reopening lost the position: %v → %v", moved, got)
	}
	hs.tt.Press(moved.X-35, moved.Y+moved.H/2)
	hs.tt.Advance(time.Second)
	hs.tt.Move(-1000, 1500)
	hs.tt.Release(-1000, 1500)
	sizeChatWorkspace(hs, 280)
	hs.tt.SetSize(700, 360)
	hs.settle()
	pane = hs.a.activeTab().tool.pane.Bounds(hs.a.review.ctx)
	close := closeBounds()
	sources := hs.detailBounds(locales.AgentChatSourcesEmpty())
	if !pane.Contains(close.X+close.W-1, close.Y) || !pane.Contains(sources.X, sources.Y+sources.H-1) {
		t.Fatalf("dragging and resizing pushed the card out of the chat: pane=%v close=%v sources=%v", pane, close, sources)
	}
}

func TestChatDetailsKeepsReadingTurn(t *testing.T) {
	hs := newHarness(t, true)
	sizeChatWorkspace(hs, 1100)
	agentViewFixture(hs)
	chat := hs.a.agent
	question := &agentMessage{role: "user", text: "Which lines changed?"}
	chat.messages = []*agentMessage{{role: "user", text: "Review this repository."},
		{role: "assistant", text: strings.Repeat("This longer paragraph wraps with the pane. ", 160)}, question,
		{role: "assistant", text: strings.Repeat("More changes.\n\n", 40)}}
	hs.settle()
	hs.clickDetail(locales.AgentTurnNumber("2"))
	for range 2 {
		hs.clickDetail(locales.AgentChatDetails())
		prompt := hs.detailBounds(question.text)
		view := chat.scroll.Bounds(hs.a.review.ctx)
		if chat.readingPrompt != question || chat.scroll.MaxY <= 0 || prompt.Y < view.Y || prompt.Y > view.Y+36 {
			t.Fatalf("toggling the card lost the selected turn: prompt=%+v view=%+v", prompt, view)
		}
	}
}

func TestChatDetailsRepositoryDiff(t *testing.T) {
	hs := newHarness(t, false)
	hs.gitRepo()
	if err := os.WriteFile(hs.path("README.md"), []byte("# FiveLeaf\n\nNew description.\nOne extra line.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sizeChatWorkspace(hs, 1100)
	agentViewFixture(hs)
	hs.settleGit()
	hs.clickDetail(locales.AgentChatDetails())
	hs.until("repository line counts", func() bool { return hs.tt.HasText("+2") && hs.tt.HasText("−1") })
	previous := hs.a.git.chatTotals
	hs.a.git.read = hs.a.git.read.Add(time.Nanosecond)
	hs.tt.Frame()
	if previous == hs.a.git.chatTotals || !hs.tt.HasText("+2") || !hs.tt.HasText("−1") {
		t.Fatal("rechecking an unchanged Git status blinked away its last known counts")
	}
	hs.clickDetail(locales.CmdAgentReviewChanges())
	if tab := hs.a.activeTab(); tab == nil || tab.diff == nil || tab.diff.repoRoot != hs.a.git.repo.Root {
		t.Fatal("the changes row did not open the displayed repository diff")
	}
}

func TestChatDetailsAddExpandAndOpen(t *testing.T) {
	hs := newHarness(t, false)
	sizeChatWorkspace(hs, 1100)
	agentViewFixture(hs)
	hs.settle()
	chat := hs.a.agent
	hs.clickDetail(locales.AgentChatDetails())
	hs.h.chosen = []string{hs.path("main.go"), hs.path("README.md"), hs.path("web/style.css"), hs.path("web/index.html")}
	hs.clickDetail(locales.AgentChatAddSource())
	hs.until("the four chosen sources", func() bool { return len(chat.detailSources()) == 4 })
	hs.settle()
	sources := chat.detailSources()
	if chat.draft != "" || !chat.hasDraft() || len(chat.messages) != 0 {
		t.Fatal("attachments must be ready to send with an empty message field")
	}
	if _, found := hs.tt.Find(locales.AgentChatOpenSource(sources[3].label)); found {
		t.Fatalf("the fourth source %q appeared before expanding", sources[3].label)
	}
	hs.clickDetail(locales.AgentChatAllSources())
	hs.detailBounds(locales.AgentChatOpenSource(sources[3].label))
	hs.clickDetail(locales.ShowLess())
	hs.clickDetail(locales.AgentNewSession())
	hs.clickDetail(locales.AgentChatDetails())
	if hs.a.agent == chat || !hs.tt.HasText(locales.AgentChatSourcesEmpty()) || len(hs.a.agent.detailSources()) != 0 {
		t.Fatal("the new conversation inherited sources")
	}
	hs.clickDetail(locales.AgentHideChatDetails())
	hs.a.openChatPage(chat)
	hs.settle()
	hs.clickDetail(locales.AgentChatOpenSource(sources[0].label))
	if tab := hs.a.activeTab(); tab == nil || tab.path() != sources[0].path || len(chat.messages) != 0 {
		t.Fatal("opening a source must show its file without sending a message")
	}
}

type delayedDetailChooser struct {
	*fakeHost
	respond func([]string)
}

func (host *delayedDetailChooser) choosePaths(_, _, _ string, _ bool, done func([]string)) {
	host.respond = done
}

func TestChatDetailsPickerIgnoresSwitchedChat(t *testing.T) {
	hs := newHarness(t, true)
	host := &delayedDetailChooser{fakeHost: hs.h}
	hs.a.host = host
	sizeChatWorkspace(hs, 1100)
	agentViewFixture(hs)
	hs.settle()
	first := hs.a.agent
	hs.clickDetail(locales.AgentChatDetails())
	hs.clickDetail(locales.AgentChatAddSource())
	hs.clickDetail(locales.AgentNewSession())
	if hs.a.agent == first || host.respond == nil {
		t.Fatal("expected a new conversation and an outstanding picker")
	}
	host.respond([]string{hs.path("README.md")})
	hs.settle()
	if first.draft != "" || hs.a.agent.draft != "" {
		t.Fatal("late picker results were appended after switching chats")
	}
}

func chatDetailsFixture(hs *harness, compact bool) {
	hs.root = hs.path("FiveLeaf")
	if err := writeFixture(hs.root); err != nil {
		hs.t.Fatal(err)
	}
	hs.gitRepo()
	if err := os.WriteFile(hs.path("README.md"), []byte("# FiveLeaf\n\nNative editor.\nWorkspace notes.\n"), 0o644); err != nil {
		hs.t.Fatal(err)
	}
	width := float32(1100)
	if compact {
		width = 390
	}
	sizeChatWorkspace(hs, width)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.messages = []*agentMessage{{role: "connection", text: "Demo Agent · ACP · stdio"},
		{role: "user", text: "看一下这个项目的改动。"}, {role: "assistant", text: "README 更新了项目说明。"}}
	logo := hs.path("fiveleaf-icon.png")
	if err := os.WriteFile(logo, iconArt, 0o644); err != nil {
		hs.t.Fatal(err)
	}
	hs.a.addChatDetailFiles(chat, []string{hs.path("web/style.css"), hs.path("main.go"), hs.path("README.md"), logo})
	hs.until("source attachments", func() bool { return chat.contextLoading == 0 })
	hs.settleGit()
	hs.clickDetail(locales.AgentChatDetails())
	hs.until("visible git counts", func() bool { return hs.tt.HasText("+2") && hs.tt.HasText("−1") })
}

func TestShotChatDetails(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	locale := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return locale }) })
	for _, item := range []struct {
		name    string
		compact bool
		dark    bool
	}{{"chat-details", false, false}, {"chat-details-compact", true, true}} {
		hs := newHarness(t, item.dark)
		hs.tt.SetLocale("zh-Hans")
		hs.tt.SetScale(2)
		chatDetailsFixture(hs, item.compact)
		hs.tt.Move(0, 0)
		hs.settle()
		hs.shot(item.name + "-window")
		close := hs.detailBounds(locales.AgentHideChatDetails())
		last := hs.detailBounds(locales.AgentChatAllSources())
		handle := hs.detailBounds(locales.AgentChatMoveDetails())
		left, top := int(handle.X)-16, int(close.Y)-20
		hs.shotRect(item.name, image.Rect(left, top, int(close.X+close.W)+24, int(last.Y+last.H)+20), 2)
	}
}

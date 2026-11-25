package main

import (
	"math"
	"strings"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func agentWorkspaceHarness(t *testing.T) *harness {
	t.Helper()
	hs := newHarness(t, true)
	agentHome(t, hs.root)
	agentViewFixture(hs)
	hs.settle()
	return hs
}

func (hs *harness) revealChatAwaitingPermission() {
	hs.t.Helper()
	if err := hs.tt.RightClick(locales.CmdViewSessions()); err != nil {
		hs.t.Fatal(err)
	}
	for _, label := range hs.tt.Menu() {
		if strings.HasSuffix(label, " · "+locales.AgentPermission()) {
			if err := hs.tt.ChooseMenuItem(label); err != nil {
				hs.t.Fatal(err)
			}
			hs.settle()
			return
		}
	}
	hs.t.Fatal("no chat with a pending permission in the Sessions context menu")
}

func TestAgentWorkspaceActivitiesFocusTheirInput(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	if err := hs.tt.Click(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.Focused(locales.SessionsFind()) {
		t.Fatal("showSessions no longer focuses the session query")
	}
	hs.chooseSessionsAgentItem("Demo Agent · " + locales.AgentOffline())
	if !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("showAgent no longer focuses the chat composer")
	}
}

func TestAgentWorkspaceKeepsChatAndSessionSelection(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	chat := a.agent
	message := &agentMessage{role: "assistant", text: "Keep this reply"}
	chat.draft = "Keep this draft"
	chat.messages = []*agentMessage{message}
	terms := len(a.terms)
	hs.settle()
	if err := hs.tt.Click(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.window.SidebarView != "sessions" {
		t.Fatal("the sessions tab did not open saved sessions")
	}
	a.sessions.all, a.sessions.query = 1, "fix there"
	hs.settle()
	if err := hs.tt.ClickWith(ui.Shift, "fix there"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	row, key := a.sessions.row, a.sessions.rowKey
	if key == "" || !a.sessions.selection.Has(key) {
		t.Fatal("the session fixture has no selected row")
	}
	hs.chooseSessionsAgentItem("Demo Agent · " + locales.AgentOffline())
	if a.window.SidebarView != "sessions" || !a.isAgentPageVisible() || a.agent != chat || chat.draft != "Keep this draft" ||
		len(chat.messages) != 1 || chat.messages[0] != message || !hs.tt.HasText(message.text) {
		t.Fatal("returning to chat lost its draft or conversation")
	}
	if err := hs.tt.Click(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.sessions.all != 1 || a.sessions.query != "fix there" || a.sessions.row != row || a.sessions.rowKey != key ||
		!a.sessions.selection.Has(key) || !hs.tt.HasText("fix there") {
		t.Fatal("returning to sessions lost its filter or selection")
	}
	// Commands and editor context actions use showAgent, so they must return
	// directly to the composer even when the saved sessions page was last open.
	a.showAgent()
	hs.settle()
	if a.window.SidebarView != "sessions" || !hs.tt.Focused(locales.AgentDraft()) || chat.draft != "Keep this draft" {
		t.Fatal("showAgent did not return from sessions to the existing chat")
	}
	if chat.client != nil || chat.cancel != nil || chat.turn != 0 || chat.busy || chat.err != "" || len(a.terms) != terms {
		t.Fatal("switching pages started a CLI prompt or terminal")
	}
}

func TestAgentWorkspaceReceivesRepliesWhileSessionsVisible(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	chat := a.agent
	chat.generation, chat.busy, chat.draft = 7, true, "Next question"
	chat.messages = []*agentMessage{{role: "assistant", text: "Reading"}}
	hs.settle()
	if err := hs.tt.Click(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	a.postChat(chat, chat.generation, func() {
		chat.messages[0].text = "The background reply arrived"
		chat.messages[0].render = viewNode{Text: chat.messages[0].text}
		chat.busy = false
	})
	hs.settle()
	if a.window.SidebarView != "sessions" || a.agent != chat || chat.busy || chat.draft != "Next question" {
		t.Fatal("the background reply changed the visible page or draft")
	}
	hs.chooseSessionsAgentItem("Demo Agent · " + locales.AgentOffline())
	if !hs.tt.HasText("The background reply arrived") || a.agent != chat {
		t.Fatal("the hidden chat lost its streamed update")
	}
}

func TestAgentWorkspaceKeepsBothScrollPositions(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.openAgentPage()
	a.agent.messages = []*agentMessage{{role: "assistant", text: strings.Repeat("A paragraph about this project.\n\n", 50)}}
	hs.settle()
	input := a.agent.input.Bounds(a.review.ctx)
	hs.tt.Scroll(input.X+input.W/2, 300, 0, -240)
	hs.settle()
	chatY := a.agent.scroll.Y
	if chatY <= 0 || chatY >= a.agent.scroll.MaxY-1 {
		t.Fatalf("the chat fixture is not reading earlier output: %v/%v", chatY, a.agent.scroll.MaxY)
	}
	st := sessionsOfToday(t, hs, 60)
	hs.tt.Scroll(100, 350, 0, 420)
	hs.settle()
	first, _ := st.list.Visible()
	if first <= 1 || first >= len(st.shown) || st.shown[first].head {
		t.Fatal("the session fixture has not scrolled into its history")
	}
	label := st.shown[first].s.Label()
	anchor, found := hs.tt.Find(label)
	if !found {
		t.Fatal("the first visible session cannot be located")
	}
	hs.chooseSessionsAgentItem("Demo Agent · " + locales.AgentOffline())
	if math.Abs(float64(a.agent.scroll.Y-chatY)) > 1 {
		t.Fatalf("returning to chat moved the reader from %v to %v", chatY, a.agent.scroll.Y)
	}
	if err := hs.tt.Click(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	after, found := hs.tt.Find(label)
	if !found || math.Abs(float64(after.Y-anchor.Y)) > 1 {
		t.Fatalf("returning to sessions moved %q from %+v to %+v (found %v)", label, anchor, after, found)
	}
}

func TestAgentWorkspaceHistoryWithoutFolderOrProvider(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.root, a.window.Folder = "", ""
	a.settings.AgentServers = nil
	a.closeAllAgents()
	a.openAgentPage()
	hs.settle()
	if !hs.tt.HasText(locales.NoFolderOpen()) {
		t.Fatal("chat did not show its missing-folder state")
	}
	if err := hs.tt.Click(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText("fix here") || !hs.tt.HasText("fix there") {
		t.Fatal("saved sessions are inaccessible without a folder or provider")
	}
	a.run("view.agentPage")
	hs.settle()
	if !hs.tt.HasText(locales.NoFolderOpen()) || a.window.SidebarView != "sessions" || !a.isAgentPageVisible() {
		t.Fatal("returning from history lost the empty chat page")
	}
	a.root, a.window.Folder = hs.root, hs.root
	hs.settle()
	if !hs.tt.HasText(locales.AgentSetupTitle()) {
		t.Fatal("chat did not show its missing-provider state")
	}
	if err := hs.tt.Click(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText("fix here") {
		t.Fatal("saved sessions depend on a configured native agent")
	}
}

func TestAgentWorkspaceSessionsControlsFitAndKeepPermissionReachable(t *testing.T) {
	for _, language := range []string{"en", "zh-Hans", "ja"} {
		t.Run(language, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return language })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			hs := agentWorkspaceHarness(t)
			a := hs.a
			for _, width := range []float32{160, 600} {
				a.window.SidebarWidth = width
				a.showSessions()
				hs.settle()
				requireAgentNavigation(t, hs, "sessions")
				buttons := []string{locales.CmdViewSessions(), locales.AgentRoster()}
				before := hs.layoutOf(buttons...)
				a.agent.permissions = []*agentPermission{{title: "Allow workspace read", options: []acp.PermissionOption{{OptionId: "once", Kind: "allow_once", Name: "Allow once"}}}}
				hs.settle()
				sameLayout(t, "pending permission", before, hs.layoutOf(buttons...))
				if a.agentPermissionCount() != 1 {
					t.Fatal("expected one pending chat permission in Sessions")
				}
				hs.revealChatAwaitingPermission()
				if !a.isAgentPageVisible() || a.window.SidebarView != "sessions" || !hs.tt.HasText("Allow workspace read") {
					t.Fatal("the pending chat is not reachable from saved sessions")
				}
				a.agent.permissions = nil
			}
		})
	}
}

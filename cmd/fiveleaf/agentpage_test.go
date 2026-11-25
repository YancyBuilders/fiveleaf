package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func agentPageCount(a *app) int {
	n := 0
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool != nil && tab.tool.kind == toolAgent {
				n++
			}
		}
	}
	return n
}

func TestAgentPageKeepsConversationAndFocus(t *testing.T) {
	hs := newHarness(t, true)
	agentHome(t, hs.root)
	agentViewFixture(hs)
	a := hs.a
	a.showSessions()
	chat := a.agent
	chat.draft = "Keep this draft"
	message := &agentMessage{role: "assistant", text: "Keep this answer"}
	permission := &agentPermission{title: "Allow reading this workspace", options: []acp.PermissionOption{{OptionId: "once", Kind: "allow_once", Name: "Allow once"}}}
	chat.messages, chat.permissions = []*agentMessage{message}, []*agentPermission{permission}
	hs.settle()
	a.run("view.agentPage")
	hs.settle()
	if !a.isAgentPageVisible() || a.window.SidebarView != "sessions" || a.agent != chat {
		t.Fatal("opening the page did not preserve the conversation and sidebar history")
	}
	if !hs.tt.Focused(locales.AgentDraft()) || !hs.tt.HasText(message.text) || !hs.tt.HasText(permission.title) {
		t.Fatal("the page lost the composer focus, answer or pending permission")
	}
	if !a.commandOn("view.agentPage") || !a.commandOn("view.agent") {
		t.Fatal("the visible page is not reflected in command selection")
	}
	hs.tt.Type(" in the page")
	hs.settle()
	if chat.draft != "Keep this draft in the page" {
		t.Fatalf("typing did not reach the page composer: %q", chat.draft)
	}
	a.openAgentPage()
	hs.settle()
	if agentPageCount(a) != 1 || a.agent != chat || chat.draft != "Keep this draft in the page" {
		t.Fatal("opening the page again duplicated or reset the conversation")
	}
	a.window.SidebarView = "agent"
	hs.settle()
	if !hs.tt.HasText(locales.SessionsFind()) || agentPageCount(a) != 1 || a.agent != chat ||
		!hs.tt.Focused(locales.AgentDraft()) || chat.messages[0] != message || chat.permissions[0] != permission || chat.draft != "Keep this draft in the page" {
		t.Fatal("the legacy sidebar view must show history without moving or replacing the active chat")
	}
}

func TestAgentPageSplitMoveAndCloseKeepBothConversations(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	editorTab := a.activeTab()
	agentViewFixture(hs)
	hs.settle()
	chat, page := a.agent, a.activeTab()
	chat.draft = "Continue after closing the page"
	a.splitRight()
	hs.settle()
	second := a.activeTab()
	if len(a.cols) != 2 || agentPageCount(a) != 2 || a.cols[0].tab() != page || second.tool.chat == chat || a.agent.draft != "" {
		t.Fatal("splitting must leave the original in place and open a fresh chat on its right")
	}
	to, _ := a.locate(editorTab)
	a.moveTab(second, to, len(to.tabs))
	hs.settle()
	if agentPageCount(a) != 2 || a.activeTab() != second || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("moving a page into a column with another chat merged the two or lost focus")
	}
	a.closeActive()
	hs.settle()
	if agentPageCount(a) != 1 || a.activeTab() != page || a.agent != chat || chat.draft != "Continue after closing the page" {
		t.Fatal("closing a sibling lost the original page or its draft")
	}
	a.reopenClosed()
	hs.settle()
	if agentPageCount(a) != 2 || a.activeTab().tool.chat != second.tool.chat || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("reopening the closed tab did not restore the second conversation")
	}
}

func TestAgentPageHistoryAndContextUseExistingPage(t *testing.T) {
	hs := newHarness(t, true)
	agentHome(t, hs.root)
	agentViewFixture(hs)
	a := hs.a
	a.window.SidebarView = "git"
	a.openAgentPage()
	hs.settle()
	page, chat := a.activeTab(), a.agent
	a.showSessions()
	hs.settle()
	if a.activeTab() != page || a.window.SidebarView != "sessions" || !a.commandOn("view.sessions") ||
		!hs.tt.Focused(locales.SessionsFind()) || !hs.tt.HasText("fix here") {
		t.Fatal("history did not open alongside the chat with its query focused")
	}
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	if a.isAgentPageVisible() || agentPageCount(a) != 1 {
		t.Fatal("the Agent page should stay open behind the editor")
	}
	col, at := a.chatTab(chat)
	a.activate(col, at, true)
	hs.settle()
	if !hs.tt.HasText(locales.SessionsFind()) || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("reactivating the chat hid sidebar history or failed to focus the composer")
	}
	if !a.appendAgentContext("Context from the editor") {
		t.Fatal("the context was not accepted")
	}
	hs.settle()
	if a.activeTab() != page || a.agent != chat || a.window.SidebarView != "sessions" ||
		chat.draft != "Context from the editor" || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("adding context failed to return to the page's composer")
	}
}

func TestAgentPageWithoutFolderOrProviders(t *testing.T) {
	hs := agentNavigationHarness(t, "", "")
	a := hs.a
	a.openAgentPage()
	hs.settle()
	if !a.isAgentPageVisible() || !hs.tt.HasText(locales.NoFolderOpen()) || hs.tt.HasText(locales.PluginsNotStarted()) {
		t.Fatal("an Agent page without a folder was blocked by plugin initialization")
	}
	a.openFolder(workspaceFixture(t))
	a.showAgent()
	hs.settle()
	if !hs.tt.HasText(locales.AgentSetupTitle()) {
		t.Fatal("the unconfigured Agent page does not explain the missing provider")
	}
	hs.tt.Type("A draft before configuring an agent")
	hs.settle()
	if a.agent.draft != "A draft before configuring an agent" || a.agent.client != nil || a.agent.connecting {
		t.Fatalf("typing in the unconfigured page: draft=%q focused=%v client=%v connecting=%v", a.agent.draft,
			hs.tt.Focused(locales.AgentDraft()), a.agent.client != nil, a.agent.connecting)
	}
}

func TestAgentPageACPContinuesAfterClose(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("agent-target.go")
	if err := os.WriteFile(path, []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(hs.root)
	if err != nil {
		t.Fatal(err)
	}
	a.settings.AgentServers = map[string]settings.AgentServer{"test": {
		Name: "Test Agent", Command: exe, Args: []string{"-test.run=^TestACPAppProcess$"},
		Env: map[string]string{"FIVELEAF_ACP_UI_TEST": "1", "FIVELEAF_ACP_UI_ROOT": root},
	}}
	a.showAgent()
	hs.until("automatic connection", func() bool { return a.agent.sessionID != "" })
	a.agent.draft = "Review and update the greeting."
	a.sendAgent()
	hs.until("Agent permission", func() bool { return len(a.agent.permissions) > 0 || a.agent.err != "" })
	if a.agent.err != "" {
		t.Fatal(a.agent.err)
	}
	chat, client, generation := a.agent, a.agent.client, a.agent.generation
	a.openAgentPage()
	hs.settle()
	if !hs.tt.HasText("Update greeting") || !chat.busy || chat.client != client || chat.generation != generation {
		t.Fatal("opening the page interrupted the running ACP conversation")
	}
	a.closeActive()
	hs.settle()
	if agentPageCount(a) != 0 || !chat.busy || chat.client != client || chat.generation != generation || len(chat.permissions) != 1 {
		t.Fatal("closing the page stopped the agent or discarded its pending permission")
	}
	a.launchAgent()
	hs.settle()
	input := chat.input.Bounds(a.review.ctx)
	hs.tt.Scroll(input.X+input.W/2, 300, 0, 1200)
	hs.settle()
	if button, ok := hs.tt.Find("Allow once"); !ok || button.W <= 0 || button.H <= 0 {
		t.Fatal("the pending permission cannot be reached in the reopened chat")
	}
	if err := hs.tt.Click("Allow once"); err != nil {
		t.Fatal(err)
	}
	hs.until("Agent prompt completion", func() bool { return !chat.busy })
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	if a.agent != chat || chat.client != client || chat.generation != generation || chat.messages[1].text != "Reviewing the file." {
		t.Fatal("the original ACP connection did not deliver its reply after the page closed")
	}
	if text, err := os.ReadFile(path); err != nil || string(text) != "package demo\n// agent change\n" {
		t.Fatalf("the approved operation did not complete: %q, %v", text, err)
	}
}

func TestShotAgentPage(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	withAppearancePlugins(t)
	for _, dark := range []bool{true, false} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			hs := newHarness(t, dark)
			agentHome(t, hs.root)
			agentViewFixture(hs)
			a := hs.a
			a.settings.Theme = ""
			if dark {
				a.settings.Theme = themeOneDarkPro
			}
			a.openFile(hs.path("main.go"), true)
			a.showAgent()
			hs.settle()
			hs.shot("agent-page-" + name + "-empty")
			a.agent.messages = []*agentMessage{
				{role: "user", text: "How do I use a chat beside my editor?"},
				{role: "assistant", text: "Open the chat in a workspace tab. Your draft and replies stay together when you switch between columns.\n\n### Navigation\n\n- Open Chat focuses the current conversation.\n- Split Right opens a separate conversation.\n- Reopen Closed Tab restores a recently closed conversation.\n\n" + strings.Repeat("You can move a tab between columns without opening another connection.\n\n", 10)},
			}
			a.agent.draft = "Check the behavior when I switch back to this chat."
			hs.settle()
			a.agent.scroll.Y = 0
			hs.settle()
			hs.shot("agent-page-" + name + "-long")
			a.agent.messages[1].text = "I found the workspace integration points. The change is ready for a focused test run."
			a.agent.permissions = []*agentPermission{{
				title: "Run focused Agent tests", detail: "go test ./cmd/fiveleaf -run TestAgentPage -count=1",
				options: []acp.PermissionOption{{OptionId: "once", Kind: "allow_once", Name: "Allow once"}, {OptionId: "deny", Kind: "reject_once", Name: "Reject"}},
			}}
			hs.settle()
			hs.shot("agent-page-" + name + "-permission")
		})
	}
}

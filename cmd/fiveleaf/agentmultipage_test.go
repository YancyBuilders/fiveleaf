package main

import (
	"context"
	"fmt"
	"image"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestChatTabsSendIndependently(t *testing.T) {
	for _, protocol := range []string{"cli", "acp"} {
		t.Run(protocol, func(t *testing.T) {
			hs := newHarness(t, true)
			a := hs.a
			agentHome(t, hs.root)
			a.settings.AgentServers = map[string]settings.AgentServer{"shared": roundtableTestServer(t, "Shared", protocol)}
			a.openAgentPage()
			left := a.activeTab()
			a.showSessions()
			a.splitRight()
			right := a.activeTab()
			hs.settle()
			if !hs.tt.HasText(locales.SessionsFind()) || left == right || left.tool.chat == right.tool.chat || left.name() != locales.AgentThreadTitle() || right.name() != left.name() {
				t.Fatal("expected sidebar history and two untitled, independent chat tabs")
			}
			leftChat, rightChat := left.tool.chat, right.tool.chat
			if protocol == "acp" {
				hs.until("independent workspace connections", func() bool { return leftChat.sessionID != "" && rightChat.sessionID != "" })
			}
			leftQuestion, rightQuestion := "为什么搜索变慢？先检查索引。", "Where is the login timeout? Check recently changed files."
			for _, turn := range []struct {
				chat     *agentChat
				question string
			}{{leftChat, leftQuestion}, {rightChat, rightQuestion}} {
				bounds := turn.chat.input.Bounds(a.review.ctx)
				if bounds.W <= 0 || bounds.H <= 0 {
					t.Fatal("missing split composer")
				}
				hs.tt.Press(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
				hs.tt.Release(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
				hs.tt.Type(turn.question)
				hs.settle()
				if turn.chat.draft != turn.question || a.agent != turn.chat {
					t.Fatalf("typing in a split reached the wrong chat: %q", turn.chat.draft)
				}
				mods := ui.Ctrl
				if macKeys() {
					mods = ui.Cmd
				}
				hs.key(mods, ui.KeyEnter)
			}
			hs.until("both agent processes replied", func() bool {
				return len(leftChat.messages) >= 2 && len(rightChat.messages) >= 2 && !leftChat.busy && !rightChat.busy
			})
			leftReply, rightReply := leftChat.messages[1].text, rightChat.messages[1].text
			if !strings.Contains(leftReply, leftQuestion) || strings.Contains(leftReply, rightQuestion) ||
				!strings.Contains(rightReply, rightQuestion) || strings.Contains(rightReply, leftQuestion) {
				t.Fatalf("split replies mixed: left=%q right=%q", leftReply, rightReply)
			}
			if left.name() != "为什么搜索变慢？" || right.name() != "Where is the login timeout?" ||
				!hs.tt.HasText(left.name()) || !hs.tt.HasText(right.name()) || leftChat.input == rightChat.input {
				t.Fatalf("split tab titles/inputs: %q, %q", left.name(), right.name())
			}
			if protocol == "acp" && (leftChat.client == nil || leftChat.client == rightChat.client) {
				t.Fatal("independent pages shared an ACP connection")
			}
		})
	}
}

func TestNewChatPageKeepsBusySourceAndPrivateTargets(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	agentHome(t, hs.root)
	a.settings.AgentServers = map[string]settings.AgentServer{"one": {Name: "One", Command: "one"}, "two": {Name: "Two", Command: "two"}}
	a.openAgentPage()
	first := a.activeTab()
	first.tool.chat.busy, first.tool.chat.draft = true, "first draft"
	hs.settle()
	if err := hs.tt.Click(locales.AgentNewSession()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	second := a.activeTab()
	secondChat := second.tool.chat
	if agentPageCount(a) != 2 || first == second || secondChat == first.tool.chat || secondChat.provider != first.tool.chat.provider || secondChat.busy || secondChat.draft != "" {
		t.Fatal("new chat reset or copied the busy conversation")
	}
	secondChat.draft = "second draft"
	a.selectAgent("two")
	a.selectAgent("one")
	if second.tool.chat != secondChat || a.agent != secondChat || secondChat.draft != "second draft" || first.tool.chat.draft != "first draft" || !first.tool.chat.busy {
		t.Fatal("changing a page's agent selected its sibling's draft")
	}
}

func TestChatTabsRevealPendingPermissionWithoutReplacingSibling(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.openAgentPage()
	first := a.agent
	hs.settle()
	hs.tt.Type("first draft")
	hs.settle()
	a.newChatPage()
	second := a.agent
	hs.settle()
	hs.tt.Type("second draft")
	hs.settle()
	permission := &agentPermission{title: "Read the first file?"}
	first.permissions = []*agentPermission{permission}
	a.showSessions()
	hs.settle()
	hs.revealChatAwaitingPermission()
	if a.window.SidebarView != "sessions" || a.agent != first || !a.chatShown(first) || agentPageCount(a) != 2 || !hs.tt.HasText(permission.title) {
		t.Fatal("the Sessions menu did not reveal the pending permission in its own chat beside history")
	}
	hs.tt.Type(" after approval")
	hs.settle()
	first.permissions = nil
	a.openChatPage(second)
	hs.settle()
	if !second.input.Focused(a.review.ctx) || second.draft != "second draft" || first.draft != "first draft after approval" {
		t.Fatal("revealing a waiting permission changed its sibling's focus or draft")
	}
}

func TestChatTabsReleaseOnlyEvictedGroups(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.openAgentPage()
	original := a.agent
	a.newChatPage()
	oldTab, old := a.activeTab(), a.agent
	oldCtx, oldCancel := context.WithCancel(context.Background())
	old.cancel = oldCancel
	a.selectChatAgent(old, "alternate")
	alternate := a.agent
	altCtx, altCancel := context.WithCancel(context.Background())
	alternate.cancel = altCancel
	a.selectChatAgent(alternate, old.provider)
	a.openChatPage(original)
	a.splitRight()
	sibling := a.agent
	a.showSessions()
	a.closeTab(oldTab, nil, nil)
	var latest *agentChat
	for range maxClosed {
		a.freshChatPage(a.column(), original)
		latest = a.agent
		latest.draft = "last closed draft"
		a.closeActive()
	}
	if oldCtx.Err() == nil || altCtx.Err() == nil || a.agentChats[agentChatKey(old)] != nil || a.agentChats[agentChatKey(alternate)] != nil {
		t.Fatal("an unreachable chat group retained its connections or transcripts after eviction")
	}
	if len(a.closed) != maxClosed || a.closed[len(a.closed)-1].chat != latest || !a.chatShown(sibling) || !a.chatShown(original) {
		t.Fatal("eviction disturbed an open split or a recently closed chat")
	}
	a.reopenClosed()
	if a.activeTab().tool.chat != latest || latest.draft != "last closed draft" {
		t.Fatal("eviction made a retained chat unrecoverable")
	}
}

func TestChatTabsEvictHistoryIndependentlyOfDefault(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a, current := hs.a, hs.a.agent
	defaultContext, cancelDefault := context.WithCancel(context.Background())
	current.cancel = cancelDefault
	source := continuationSource(t, hs.root)
	a.continueSessionWith(source, current.provider)
	history := a.agent
	hs.until("continuation loaded", history.continuationReady)
	historyContext, cancelHistory := context.WithCancel(context.Background())
	history.cancel = cancelHistory
	a.closeActive()
	if historyContext.Err() != nil || a.agent != current {
		t.Fatal("closing a history tab did not leave it restorable beside the default chat")
	}
	source.ID += "-second"
	a.continueSessionWith(source, current.provider)
	second := a.agent
	a.closeActive()
	for range maxClosed - 1 {
		a.openFile(hs.path("main.go"), true)
		a.closeActive()
	}
	if historyContext.Err() == nil || a.agentChats[history.historyKey] != nil {
		t.Fatal("an evicted history tab kept its connection or transcript")
	}
	if defaultContext.Err() != nil || a.agent != current || a.agentChats[second.historyKey] != second {
		t.Fatal("evicting one history tab discarded the default or another restorable chat")
	}
}

func TestChatTabsEditorContextUsesMostRecentlyFocusedOpenPage(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.openAgentPage()
	first := a.agent
	first.draft = "keep the oldest draft"
	a.newChatPage()
	second := a.agent
	a.newChatPage()
	third, tab := a.agent, a.activeTab()
	a.openChatPage(first)
	a.openChatPage(second)
	a.openChatPage(third)
	a.openFile(hs.path("main.go"), true)
	a.closeTab(tab, nil, nil)
	if a.agent != second || a.activeTab().ed == nil || a.chatShown(second) {
		t.Fatal("closing a hidden tab did not leave the editor focused with the last open chat as target")
	}
	if !a.appendAgentContext("editor context") {
		t.Fatal("editor context was not accepted")
	}
	hs.settle()
	if !a.chatShown(second) || first.draft != "keep the oldest draft" || second.draft != "editor context" || third.draft != "" || agentPageCount(a) != 2 {
		t.Fatal("editor context reopened the closed tab or went to the oldest draft")
	}
}

func TestChatTabsProviderSwitchAfterSplitClosesOntoEditor(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.settings.AgentServers["other"] = settings.AgentServer{Name: "Other Agent", Command: "other-agent"}
	a.openAgentPage()
	first := a.agent
	first.draft = "first page draft"
	a.splitRight()
	second, tab := a.agent, a.activeTab()
	second.draft = "closed page draft"
	a.openFile(hs.path("main.go"), true)
	editor := a.activeTab()
	hs.settle()
	a.closeTab(tab, nil, nil)
	hs.settle()
	if a.activeTab() != editor || a.agent != first || !a.chatShown(first) || a.chatShown(second) {
		t.Fatal("closing a hidden split chat did not route context to the surviving page")
	}
	for _, target := range []string{"Other Agent", "Demo Agent"} {
		chooseChatTarget(t, hs, target)
	}
	if a.agent != first || first.draft != "first page draft" || second.draft != "closed page draft" {
		t.Fatal("switching providers lost the original draft or changed the closed page")
	}
	a.reopenClosed()
	hs.settle()
	if a.agent != second || !a.chatShown(first) || !a.chatShown(second) {
		t.Fatal("reopening the closed page did not keep both chats available")
	}
}

func TestChatTabsClosingLastLeavesFreshUnopenedTarget(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.openAgentPage()
	original := a.activeTab()
	originalCtx, cancelOriginal := context.WithCancel(context.Background())
	original.tool.chat.cancel = cancelOriginal
	a.newChatPage()
	last := a.agent
	ctx, cancel := context.WithCancel(context.Background())
	last.cancel = cancel
	a.closeTab(original, nil, nil)
	a.closeActive()
	if a.agent == last || a.agent.draft != "" || a.agent.client != nil || a.agent.connecting || agentPageCount(a) != 0 || ctx.Err() != nil {
		t.Fatal("closing the last chat should leave a fresh unopened target without cancelling the restorable tab")
	}
	for range maxClosed {
		a.openFile(hs.path("main.go"), true)
		a.closeActive()
	}
	if ctx.Err() == nil || originalCtx.Err() == nil || a.agentChats[agentChatKey(last)] != nil || a.agentChats[agentChatKey(original.tool.chat)] != nil {
		t.Fatal("evicting the closed chats kept their connections alive")
	}
}

func TestChatTabTitleStaysShortAndStable(t *testing.T) {
	question := strings.Repeat("哪里", 50)
	chat := &agentChat{draft: "unsubmitted text"}
	view := &toolView{kind: toolAgent, chat: chat}
	if view.name() != locales.AgentThreadTitle() {
		t.Fatal("typing changed the title before sending")
	}
	chat.messages = []*agentMessage{{role: "system", text: "invisible"}, {role: "user", text: "\n " + question + "\n\nMore detail"}, {role: "user", text: "Later turn"}}
	title := view.name()
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > 37 || !strings.HasPrefix(title, "哪里") || !strings.HasSuffix(title, "…") {
		t.Fatalf("long question did not make a short readable title: %q", title)
	}
	chat.pendingPrompt = "a follow-up question"
	if view.name() != title {
		t.Fatal("a follow-up renamed the tab")
	}
	withContext := agentContextText("/workspace/main.go", 1, 2, "package main\nfunc main() {}") + "\n\n为什么程序退出了？更多细节。"
	chat.messages = []*agentMessage{{role: "user", text: withContext}}
	if got := view.name(); got != "为什么程序退出了？" {
		t.Fatalf("attached code obscured the question: %q", got)
	}
}

func TestSessionsSidebarSwitchesMatchingChatTitles(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.showSessions()
	hs.settle()
	hs.chooseSessionsAgentItem("Demo Agent · " + locales.AgentOffline())
	first := a.activeTab()
	if err := hs.tt.Click(locales.SessionsStartChat()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	second := a.activeTab()
	for _, tab := range []*colTab{first, second} {
		tab.tool.chat.messages = []*agentMessage{{role: "user", text: "Same question"}}
	}
	hs.settle()
	if _, found := hs.tt.Find(locales.SessionsOpenChats()); found {
		t.Fatal("the chat tab switcher still takes space beside session search")
	}
	if err := hs.tt.RightClick(locales.CmdViewSessions()); err != nil {
		t.Fatal(err)
	}
	firstLabel, secondLabel := fmt.Sprintf("Same question · #%d", first.id), fmt.Sprintf("Same question · #%d", second.id)
	menu := hs.tt.Menu()
	if !slices.Contains(menu, firstLabel) || !slices.Contains(menu, secondLabel) {
		t.Fatalf("missing distinct chat tabs in the sidebar menu: %q", menu)
	}
	if err := hs.tt.ChooseMenuItem(firstLabel); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.activeTab() != first || a.agent != first.tool.chat || !hs.tt.Focused(locales.AgentDraft()) || !hs.tt.HasText(locales.SessionsFind()) {
		t.Fatal("selecting a chat in the sidebar lost the page, its focus or the history view")
	}
}

func TestShotSessionsBesideChatTabs(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
	hs := agentWorkspaceHarness(t)
	a := hs.a
	hs.tt.SetSize(1440, 820)
	a.window.SidebarWidth = 252
	a.openAgentPage()
	a.showSessions()
	first := a.agent
	first.messages = []*agentMessage{
		{role: "user", text: "为什么搜索变慢？先看看索引。"},
		{role: "assistant", text: "每次输入都重新扫描了整个目录。可以复用已有索引，只对变更文件重新分词。"},
	}
	hs.settle()
	a.splitRight()
	a.agent.messages = []*agentMessage{
		{role: "user", text: "登录超时在哪里设置？"},
		{role: "assistant", text: "超时由登录客户端的配置控制。可以先区分连接超时和等待响应超时，再调整对应的值。"},
	}
	first.draft, a.agent.draft = "列出需要更新的测试", "补上超时提示"
	hs.settle()
	now := time.Now()
	a.sessions.sessions = []agents.Session{
		{Harness: agents.Codex, ID: "index", Title: "搜索索引的增量更新", Dir: a.root, Updated: now},
		{Harness: agents.Claude, ID: "auth", Title: "排查登录请求超时", Dir: a.root, Updated: now.Add(-time.Hour)},
		{Harness: agents.Pi, ID: "review", Title: "检查最近的接口变更", Dir: a.root, Updated: now.Add(-2 * time.Hour)},
	}
	a.sessions.version++
	hs.settle()
	hs.tt.Advance(motionEnter)
	hs.tt.Frame()
	if !hs.tt.HasText("排查登录请求超时") {
		t.Fatal("history is not visible beside the split chats")
	}
	hs.shot("sessions-with-chat-split-zh")
	hs.tt.SetDark(false)
	hs.tt.SetScale(2)
	hs.settle()
	controls := hs.layoutOf(locales.SessionsFind(), locales.SessionsFilterAgent())
	query, filter := controls[locales.SessionsFind()], controls[locales.SessionsFilterAgent()]
	hs.shotRect("sessions-scope-zh-light", image.Rect(int(query.X)-12, titleBarH, int(query.X+query.W)+12, int(filter.Y+filter.H)+6), 2)
}

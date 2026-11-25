package main

// The state gallery: each surface in each of its states, in every theme,
// light and dark, and every language, side by side on one page, to review
// a change of look across all of them at once and to catch what clips in
// one language or theme alone. Run
// GALLERY=<dir> go test -run TestGallery ./cmd/fiveleaf
// and open <dir>/index.html.

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// galleryThemes are the themes of the gallery, "" for FiveLeaf's own.
var galleryThemes = []string{"", themeMatrix, themeClaude, themeQoder, themeGrokBot, themeCursor,
	themePurpleGold, themePumpkin, themeWitchsBrew, themeBloodMoon}

// galleryLocales are the languages FiveLeaf has a catalog of.
var galleryLocales = []string{"en", "ja", "zh-Hans"}

// agentViewFixture renders a configured agent without launching a process.
func agentViewFixture(hs *harness) {
	hs.a.settings.AgentServers = map[string]settings.AgentServer{
		"demo": {Name: "Demo Agent", Protocol: "cli", Command: "demo-agent"},
	}
	hs.a.openAgentPage()
}

func sizeChatWorkspace(hs *harness, width float32) {
	hs.a.settings.SidebarRight = true
	hs.a.window.SidebarClosed, hs.a.window.SidebarWidth = false, 420
	hs.tt.SetSize(int(width+hs.a.window.SidebarWidth), 760)
}

func agentToolbarFixture(hs *harness) {
	hs.a.openFile(hs.path("main.go"), true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.messages = []*agentMessage{{role: "connection", text: "**Demo Agent**\n\n`protocol: ACP`\n\n`transport: stdio`"}}
	chat.acceptContextUsage(acp.UsageState{Used: 0, Size: 1000000})
}

func agentTurnRailFixture(hs *harness, preview bool) {
	agentViewFixture(hs)
	for _, exchange := range [][2]string{
		{"预览版怎么启动？", "在项目目录启动预览后，可以直接打开聊天标签页。"},
		{"能同时打开两个会话吗？", "可以。新会话有自己的草稿，两边可以分别选择模型。"},
		{"代码选区会发到哪里？", "选中的代码会放进最近使用的会话草稿，确认后才会发送。"},
		{"我想回看前面讨论过的实现", "把指针移到左边的刻度上，可以先看那一轮的问题和回复，再决定要不要跳过去。"},
		{"跳过去会打断当前回复吗？", "不会。回复继续生成，阅读位置会留在你选择的提问。"},
		{"历史会话也能用吗？", "从历史记录继续聊天时，导入的提问也会显示在刻度上。"},
		{"能用键盘回看吗？", "把焦点移到刻度，用上下方向键选择一轮，再按回车跳到提问。"},
		{"怎么回到最新回复？", "点输入框上方的返回最新消息按钮，即可接着看新输出。"},
		{"过程中可以继续写草稿吗？", "可以。回看历史消息时，输入框会保留你正在写的内容。"},
		{"一轮里调用好几个工具怎么算？", "每个刻度对应一次提问，这些工具调用和最后的回复属于同一轮。"},
		{"分栏后会跳到另一个会话吗？", "每个分栏都有自己的刻度和阅读位置。"},
	} {
		hs.a.agent.messages = append(hs.a.agent.messages, &agentMessage{role: "user", text: exchange[0]}, &agentMessage{role: "assistant", text: exchange[1]})
	}
	hs.settle()
	view := hs.a.agent.scroll.Bounds(hs.a.review.ctx)
	hs.tt.Scroll(view.X+view.W/2, view.Y+view.H/2, 0, -hs.a.agent.scroll.MaxY/2)
	if !preview {
		return
	}
	hs.settle()
	mark, found := hs.tt.Find(locales.AgentTurnNumber("4"))
	if !found {
		hs.t.Fatal("missing gallery turn marker")
	}
	hs.tt.Move(mark.X+mark.W/2, mark.Y+mark.H/2)
}

func agentRichOutputFixture(hs *harness) {
	agentViewFixture(hs)
	tool := &agentTool{title: "Read agent.go", kind: "read", status: "completed", text: "package main\n"}
	hs.a.agent.tools = []*agentTool{tool}
	hs.a.agent.messages = []*agentMessage{
		{role: "user", text: "Explain the update and show a Go example."},
		{role: "thought", text: "Inspect the current implementation.\n\nCheck the behavior before proposing a change."},
		{role: "tool", tool: tool},
		{role: "assistant", text: "## Rendered output\n\n**Ready to use.** Replies keep their structure and `code` stays readable.\n\n- Streaming replies keep your reading position.\n- Tool details remain available when you expand them.\n\n```go\nfunc greet(name string) string {\n    // Keep the original Markdown when copying.\n    return fmt.Sprintf(\"Hello, %s!\", name)\n}\n```\n\n| Item | Result |\n| :--- | :--- |\n| Syntax highlighting | Enabled |\n| Context display | 9% |\n\n> Copy a code block to reuse the exact snippet."},
	}
}

func agentModelPickerFixture(hs *harness, empty bool) {
	agentPickerFixture(hs, empty, "")
}

func agentPickerFixture(hs *harness, empty bool, thinking string) {
	agentViewFixture(hs)
	executable, err := os.Executable()
	if err != nil {
		hs.t.Fatal(err)
	}
	// Use the local ACP test process so the picker has a real session and cleanup.
	scenario := "acp"
	if thinking != "" {
		scenario = "thinking"
	}
	hs.a.settings.AgentServers["demo"] = settings.AgentServer{
		Name: "Demo Agent", Command: executable, Args: []string{"-test.run=^TestAgentRoleRuntimeProcess$"},
		Env: map[string]string{"FIVELEAF_ROLE_RUNTIME": scenario, "FIVELEAF_ROLE_LOG": hs.path("picker-requests.jsonl")},
	}
	hs.t.Cleanup(hs.a.closeAllAgents)
	hs.a.connectAgent()
	hs.untilLong("gallery model connection", func() bool { return !hs.a.agent.connecting })
	s := hs.a.agent
	if s.err != "" || s.sessionID == "" {
		hs.t.Fatalf("gallery model connection: error=%q session=%q", s.err, s.sessionID)
	}
	if thinking != "" {
		hs.a.setChatMode(s, thinking)
		hs.untilLong("gallery thinking choice", func() bool { return !s.configuring })
		if s.err != "" || s.mode != thinking {
			hs.t.Fatalf("gallery thinking choice: error=%q mode=%q", s.err, s.mode)
		}
	}
	s.models = []acp.ModelChoice{
		{ID: "model-a", Name: "Code model", Group: "Development", Description: "Read, edit and explain code"},
		{ID: "model-b", Name: "Review model", Group: "Development", Description: "Review changes and find regressions"},
		{ID: "default", Name: "General model", Group: "General"},
	}
	s.currentModel, s.recentModels = "model-a", []string{"model-a", "model-b"}
	s.acceptContextUsage(acp.UsageState{Used: 40000, Size: 1000000})
	hs.a.openAgentPage()
	hs.settle()
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		hs.t.Fatal(err)
	}
	if empty {
		hs.tt.Type("no matching model")
	}
}

func agentContextDetailsFixture(hs *harness, known bool) {
	agentViewFixture(hs)
	s := hs.a.agent
	s.currentModel = "DeepSeek V4.1 Flash"
	if known {
		s.sessionID = "gallery-context"
		s.messages = []*agentMessage{{role: "user", text: "Explain this code and show a minimal example."}}
		captureAgentContext(s, chatContext(s))
		s.messages = append(s.messages, &agentMessage{role: "assistant", text: "The program reads the current project and renders its files.\n\n```go\nfunc main() {\n    openProject()\n}\n```"})
		s.contextPolicy = agentCompactionPolicy{known: true, enabled: true, reserveTokens: 150000}
		s.acceptContextUsage(acp.UsageState{Used: 38000, Size: 1000000})
	}
	hs.a.openAgentPage()
	s.contextDetailsOpen = true
}

func projectNavigationFixture(hs *harness) {
	for _, name := range []string{"fiveleaf", "backend", "design-system"} {
		dir := filepath.Join(hs.root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			hs.t.Fatal(err)
		}
		hs.a.settings.AddRecent(dir)
	}
	hs.a.settings.AddRecent(remote.Path("dev-server", "/srv/workspace"))
	hs.a.openProjectPicker()
}

// galleryStates are the states shown, each brought about on a window just
// opened on the test folder.
var galleryStates = []struct {
	name string
	set  func(hs *harness)
}{
	{"empty", func(hs *harness) {}},
	{"update-available", func(hs *harness) {
		hs.a.openFile(hs.path("main.go"), true)
		hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0", notes: testUpdateNotes}
		hs.a.openUpdates()
	}},
	{"update-required", func(hs *harness) {
		hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0", minimum: "0.21.0", notes: testUpdateNotes}
	}},
	{"update-install-failed", func(hs *harness) {
		testUpdatePublisher(hs.t)
		hs.h.updateState = updateStatus{phase: updateInstallFailed, version: "0.22.0", minimum: "0.21.0", notes: testUpdateNotes}
	}},
	{"commit-graph", func(hs *harness) {
		graphHistoryFixture(hs)
		hs.a.repository().openCommitGraph()
	}},
	{"notification-history", func(hs *harness) {
		hs.a.tell(note{message: locales.AgentStatusCompleted(), description: "Review imports", kind: "success", source: locales.PrefSectionAgent()})
		hs.a.noticeHistoryOpen = true
	}},
	{"editor", func(hs *harness) { hs.a.openFile(hs.path("main.go"), true) }},
	{"editor-completion", func(hs *harness) {
		const text = "package main\n\nvar projectName, projectRoot string\n\nfunc main() {\n\tprintln()\n}\n"
		path := hs.path("completion.go")
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			hs.t.Fatal(err)
		}
		hs.a.openFile(path, true)
		hs.settle()
		caret := strings.Index(text, "println(") + len("println(")
		hs.place(hs.a.activeEditor(), caret, caret)
		hs.tt.Type("proj")
		hs.settle()
	}},
	{"split-picker", func(hs *harness) {
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hoverSplitButton(hs)
	}},
	{"terminal-panel-split", func(hs *harness) {
		hs.t.Cleanup(hs.a.closeTerminals)
		if runtime.GOOS != "windows" {
			hs.t.Setenv("SHELL", "/bin/sh")
		}
		hs.a.newTerminalIn(hs.root)
		source := hs.a.panelTerm
		hs.a.splitPanelTerminal(source, splitRight)
		if source == nil || hs.a.panelTerm == source {
			hs.t.Fatal("the terminal panel did not split")
		}
		agentHome(hs.t, hs.root)
		hs.a.settings.SidebarRight = true
		hs.a.showSessions()
		agentViewFixture(hs)
	}},
	{"agent-no-workspace", func(hs *harness) {
		hs.a.closeFolder()
		hs.a.showAgent()
	}},
	{"agent-unconfigured", func(hs *harness) { hs.a.showAgent() }},
	{"agent-empty", func(hs *harness) { agentViewFixture(hs) }},
	{"agent-chat-toolbar", agentToolbarFixture},
	{"agent-attachment-preview", agentAttachmentPreviewFixture},
	{"inline-copilot", inlineCopilotFixture},
	{"roundtable-invite", func(hs *harness) { roundtableGallery(hs, false) }},
	{"roundtable-member-tasks", roundtableTasksGallery},
	{"roundtable-node-drawer", func(hs *harness) {
		roundtableTasksGallery(hs)
		window := hs.a.meetingWindow
		window.view = roundtableViewGraph
		window.inspector.setVisible(false)
		window.openNodeDrawer(window.expandedTask)
	}},
	{"sessions-meeting-records", func(hs *harness) { roundtableGallery(hs, true) }},
	{"toolbox-launcher", func(hs *harness) { toolboxTestPlugins(hs, "base64", "json", "timestamp", "url") }},
	{"agent-chat-details", func(hs *harness) { chatDetailsFixture(hs, false) }},
	{"agent-chat-details-compact", func(hs *harness) { chatDetailsFixture(hs, true) }},
	{"agent-rich-output", func(hs *harness) { agentRichOutputFixture(hs) }},
	{"agent-turn-rail", func(hs *harness) {
		hs.tt.SetSize(1680, 760)
		agentTurnRailFixture(hs, false)
	}},
	{"agent-turn-preview", func(hs *harness) {
		hs.tt.SetSize(1680, 760)
		agentTurnRailFixture(hs, true)
	}},
	{"agent-turn-preview-narrow", func(hs *harness) {
		sizeChatWorkspace(hs, 390)
		agentTurnRailFixture(hs, true)
	}},
	{"agent-page-rich-output", func(hs *harness) {
		agentRichOutputFixture(hs)
		hs.a.openAgentPage()
	}},
	{"agent-context-full", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.agent.acceptContextUsage(acp.UsageState{Used: 90000, Size: 100000})
	}},
	{"agent-model-picker", func(hs *harness) { agentModelPickerFixture(hs, false) }},
	{"agent-model-picker-empty", func(hs *harness) { agentModelPickerFixture(hs, true) }},
	{"agent-thinking-low", func(hs *harness) { agentPickerFixture(hs, false, "low") }},
	{"agent-thinking-high", func(hs *harness) { agentPickerFixture(hs, false, "high") }},
	{"agent-thinking-max", func(hs *harness) { agentPickerFixture(hs, false, "max") }},
	{"agent-context-details", func(hs *harness) { agentContextDetailsFixture(hs, true) }},
	{"agent-context-details-unknown", func(hs *harness) { agentContextDetailsFixture(hs, false) }},
	{"agent-page-empty", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.openAgentPage()
	}},
	{"agent-page-unconfigured", func(hs *harness) { hs.a.openAgentPage() }},
	{"agent-page-permission", func(hs *harness) {
		agentViewFixture(hs)
		raw := []byte(`{"method":"confirm","title":"Reply","message":"Send the draft reply?"}`)
		hs.a.agent.permissions = []*agentPermission{{
			title:   "Reply",
			detail:  formatAgentToolDetail(nil, raw),
			preview: previewPermission(raw, nil, "Reply", false),
			options: []acp.PermissionOption{{OptionId: "yes", Kind: "allow_once", Name: "Yes"}, {OptionId: "no", Kind: "reject_once", Name: "No"}},
			choose:  func(string) {},
		}}
		hs.a.openAgentPage()
	}},
	{"agent-page-error", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.agent.err = "The agent process exited. Reconnect to start a new session."
		hs.a.openAgentPage()
	}},
	{"agent-role", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.settings.AgentRoles = map[string]settings.AgentRole{
			"Code reviewer": {Agent: "demo", Model: "review-model", Prompt: "Review changes for correctness."},
		}
		hs.a.selectAgentRole("Code reviewer")
	}},
	{"agent-role-unavailable", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.settings.AgentRoles = map[string]settings.AgentRole{
			"Code reviewer": {Agent: "missing-agent", Model: "review-model", Prompt: "Review changes for correctness."},
		}
		hs.a.selectAgentRole("Code reviewer")
		hs.a.agent.draft = "Review my changes."
	}},
	{"agent-connecting", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.agent.connecting = true
	}},
	{"agent-error", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.agent.err = "The agent process exited. Reconnect to start a new session."
	}},
	{"agent-permission", func(hs *harness) {
		agentViewFixture(hs)
		hs.a.agent.permissions = []*agentPermission{{
			title:   "Update main.go",
			options: []acp.PermissionOption{{OptionId: "once", Kind: "allow_once", Name: "Allow once"}, {OptionId: "reject", Kind: "reject_once", Name: "Reject"}},
			choose:  func(string) {},
		}}
	}},
	{"agent-working", func(hs *harness) {
		agentViewFixture(hs)
		r := hs.a.agent
		r.sessionID, r.busy = "gallery-session", true
		r.messages = []*agentMessage{{role: "user", text: "Explain the main function."}, {role: "assistant", text: "The entry point prints a greeting:\n\n```go\nfmt.Println(\"hello, fiveleaf\")\n```"}}
		r.plan = []agentPlan{{content: "Read the entry point", status: "completed"}, {content: "Explain its behavior", status: "in_progress"}}
		r.tools = []*agentTool{{id: "read", title: "Read main.go", status: "completed"}}
	}},
	{"lsp-no-workspace", func(hs *harness) {
		hs.a.closeFolder()
		openLSPStatus(hs)
	}},
	{"lsp-empty", func(hs *harness) { openLSPStatus(hs) }},
	{"lsp-starting", func(hs *harness) {
		lspStatusFixture(hs.t, hs.root, "typescript-language-server", lspStarting)
		openLSPStatus(hs)
	}},
	{"lsp-running", func(hs *harness) {
		lspStatusFixture(hs.t, hs.root, "typescript-language-server", lspRunning)
		openLSPStatus(hs)
	}},
	{"lsp-failed", func(hs *harness) {
		lspStatusFixture(hs.t, hs.root, "typescript-language-server", lspFailed)
		openLSPStatus(hs)
		if err := hs.tt.Click("typescript-language-server"); err != nil {
			hs.t.Fatal(err)
		}
	}},
	{"lsp-stopped", func(hs *harness) {
		lspStatusFixture(hs.t, hs.root, "typescript-language-server", lspStopped)
		openLSPStatus(hs)
	}},
	{"hop-word", func(hs *harness) {
		hs.a.settings.Vim = true
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.a.startJumpHints(hs.a.activeEditor(), jumpHintWords)
	}},
	{"helix-word", func(hs *harness) {
		hs.a.settings.EditorMode = "helix"
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.a.startJumpHints(hs.a.activeEditor(), jumpHintHelixWords)
	}},
	{"hop-char-query", func(hs *harness) {
		hs.a.settings.Vim = true
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.a.startJumpHints(hs.a.activeEditor(), jumpHintChar2)
	}},
	{"file-loading", func(hs *harness) {
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		ed := hs.a.activeEditor()
		ed.sync, ed.readingAt = syncLoading, hs.tt.Now().Add(-readingShows)
	}},
	{"file-error", func(hs *harness) { hs.a.openFile(hs.path("missing.go"), true) }},
	{"palette", func(hs *harness) { hs.a.run("go.commands") }},
	{"tasks", taskGalleryFixture},
	{"task-no-match", func(hs *harness) {
		taskGalleryFixture(hs)
		hs.a.taskPick.query = "zzzz-no-task"
	}},
	{"task-error", func(hs *harness) {
		taskGalleryFixture(hs)
		hs.a.taskPick.errors = []taskConfigError{{path: hs.a.projectTasksPath(), err: fmt.Errorf("tasks must be a JSON array")}}
	}},
	{"icon-picker", func(hs *harness) {
		hs.a.openFile(hs.path("main.go"), true)
		hs.a.openIconThemePicker()
	}},
	{"icon-picker-no-match", func(hs *harness) {
		hs.a.openIconThemePicker()
		hs.a.iconPicker.query = "zzzz-nothing"
	}},
	{"project-picker", projectNavigationFixture},
	{"project-picker-empty", func(hs *harness) {
		hs.a.settings.Recent = nil
		hs.a.openProjectPicker()
	}},
	{"project-picker-no-match", func(hs *harness) {
		projectNavigationFixture(hs)
		hs.a.recents.query = "zzzz-nothing"
	}},
	{"leader", func(hs *harness) {
		hs.a.settings.EditorMode = "helix"
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.a.openLeaderMenu()
	}},
	{"leader-code", func(hs *harness) {
		hs.a.settings.EditorMode = "vim"
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.a.openLeaderMenu()
		hs.a.leader.group = 'c'
	}},
	{"leader-windows", func(hs *harness) {
		hs.a.settings.EditorMode = "helix"
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.a.splitRight()
		hs.settle()
		hs.key(ui.Ctrl, ui.KeyW)
	}},
	{"no-match", func(hs *harness) { hs.a.openQuick("zzzz-nothing") }},
	{"path-pending", func(hs *harness) {
		hs.a.openQuick(hs.path("pending.go"))
		p := &hs.a.quick.probe
		p.key = hs.a.quickPathKey()
		p.name, p.pending = hs.a.quick.query, true
		p.read.Invalidate()
		p.read.Start(p.key)
	}},
	{"path-missing", func(hs *harness) { hs.a.openQuick(hs.path("missing.go")) }},
	{"path-error", func(hs *harness) {
		hs.a.quick.probe.stat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
		hs.a.openQuick(hs.path("denied.go"))
	}},
	{"sessions", func(hs *harness) {
		agentHome(hs.t, hs.root)
		hs.a.sessions.all = 1
		hs.a.showSessions()
	}},
	{"sessions-actions-narrow", func(hs *harness) {
		agentHome(hs.t, hs.root)
		hs.a.settings.SidebarRight = true
		hs.a.window.SidebarWidth = 160
		hs.a.showSessions()
	}},
	{"sessions-hover", func(hs *harness) {
		agentHome(hs.t, hs.root)
		hs.a.sessions.all = 1
		hs.a.showSessions()
		hs.settle()
		row, found := hs.tt.Find("fix there")
		if !found {
			hs.t.Fatal("missing second gallery session")
		}
		hs.tt.Move(row.X+row.W/2, row.Y+row.H/2)
	}},
	{"sessions-find", func(hs *harness) {
		agentHome(hs.t, hs.root)
		hs.a.sessions.all = 1
		hs.a.showSessions()
		hs.settle()
		// The query marked in the rows, and the keyboard in the list, which
		// shows the chosen row's buttons.
		hs.a.sessions.query = "fix else"
		hs.settle()
		hs.key(0, ui.KeyTab)
		hs.key(0, ui.KeyTab)
	}},
	{"sessions-why", func(hs *harness) {
		home := agentHome(hs.t, hs.root)
		// A titled session, which the query finds in its first prompt.
		p := filepath.Join(home, ".claude", "projects", "p-titled", "titled.jsonl")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(`{"type":"user","message":{"content":"the sign in page of the account settings shows a blank form"},"cwd":"`+
			filepath.ToSlash(hs.root)+`"}`+"\n"+`{"type":"custom-title","customTitle":"Login bug"}`+"\n"), 0o644)
		hs.a.showSessions()
		hs.settle()
		hs.a.sessions.query = "login blank"
	}},
	{"sessions-agents", func(hs *harness) {
		// A session of each agent, each row marked with its agent's icon.
		home := agentHome(hs.t, hs.root)
		cwd := filepath.ToSlash(hs.root)
		files := map[string]string{
			".qoder/projects/p/q1.jsonl": `{"type":"user","message":{"content":"add dark mode"},"cwd":"` + cwd + `"}` + "\n",
			".codex/sessions/2026/rollout-x1.jsonl": `{"type":"session_meta","payload":{"id":"x1","cwd":"` + cwd + `"}}` + "\n" +
				`{"type":"response_item","payload":{"type":"message","role":"user","content":"speed up the build"}}` + "\n",
			".grok/sessions/p/g1/summary.json": `{"info":{"id":"g1","cwd":"` + cwd + `"},"generated_title":"Write the release notes"}`,
		}
		for name, text := range files {
			p := filepath.Join(home, filepath.FromSlash(name))
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte(text), 0o644)
		}
		hs.a.showSessions()
	}},
	{"live-preview", func(hs *harness) {
		// A note as Live Preview: its marks hidden but where the caret is,
		// its tasks, quote, code and table drawn.
		src := "# Notes\n\nSome **bold** and `code`, a [link](https://example.com).\n\n- [ ] open\n- [x] done\n\n" +
			"> [!TIP]\n> A tip.\n\n```go\nfunc main() {}\n```\n\n| Key | Value |\n|-----|-------|\n| a   | 1     |\n"
		os.WriteFile(hs.path("notes.md"), []byte(src), 0o644)
		hs.a.settings.MarkdownMode = "live"
		hs.a.openFile(hs.path("notes.md"), true)
		d := hs.a.activeEditor()
		hs.until("the note parsed", func() bool { l := d.document.live; return l != nil && l.doc != nil && !l.quick })
		at := strings.Index(src, "bold")
		hs.place(d, at, at)
	}},
	{"unsaved", func(hs *harness) {
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.insert("// more\n")
	}},
}

func TestGallery(t *testing.T) {
	dir := os.Getenv("GALLERY")
	if dir == "" {
		t.Skip("GALLERY is not set")
	}
	withAppearancePlugins(t)
	var page strings.Builder
	page.WriteString(`<!doctype html><meta charset="utf-8"><title>FiveLeaf state gallery</title>
<style>body{font:13px system-ui;margin:16px;background:#888}h2{margin:24px 0 8px}
.row{display:flex;gap:8px;overflow-x:auto}figure{margin:0}img{width:480px;display:block}figcaption{padding:2px 0}</style>`)
	for _, st := range galleryStates {
		fmt.Fprintf(&page, "<h2>%s</h2>\n", html.EscapeString(st.name))
		for _, loc := range galleryLocales {
			page.WriteString(`<div class="row">`)
			for _, theme := range galleryThemes {
				for _, dark := range []bool{false, true} {
					if !dark && alwaysDark(theme) {
						continue
					}
					name := galleryName(st.name, loc, theme, dark)
					t.Run(name, func(t *testing.T) {
						locale := i18n.Locale()
						i18n.SetLocaleFunc(func() string { return loc })
						t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return locale }) })
						hs := newHarness(t, dark)
						hs.tt.SetLocale(loc)
						if theme != "" {
							hs.a.setTheme(theme)
						}
						st.set(hs)
						hs.settle()
						os.Setenv("SHOTS", dir)
						defer os.Unsetenv("SHOTS")
						hs.shot(name)
					})
					fmt.Fprintf(&page, `<figure><img loading="lazy" src="%s.png"><figcaption>%s</figcaption></figure>`,
						name, html.EscapeString(name))
				}
			}
			page.WriteString("</div>\n")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(page.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func galleryName(state, loc, theme string, dark bool) string {
	if theme == "" {
		theme = "fiveleaf"
	}
	mode := "light"
	if dark {
		mode = "dark"
	}
	return strings.Join([]string{state, loc, theme, mode}, "-")
}

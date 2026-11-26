package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestAgentComposerAtFile(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	hs.clickDetail(locales.AgentDraft())
	hs.tt.Type("看 @go.m")
	hs.until("workspace file candidate", func() bool { _, ok := hs.tt.Find("@go.mod"); return ok })
	hs.key(0, ui.KeyEscape)
	if hs.tt.HasText(locales.QuickSearchFiles()) || chat.draft != "看 @go.m" {
		t.Fatal("Escape changed the draft or left the file menu open")
	}
	hs.tt.Type("o")
	hs.key(0, ui.KeyEnter)
	hs.until("file attached", func() bool { return chat.contextLoading == 0 && chat.sources.draft.hasFiles() })
	if chat.turn != 0 || chat.draft != "看 @go.mod " || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatalf("file choice: turn=%d draft=%q", chat.turn, chat.draft)
	}
	hs.tt.Type("里的模块名")
	prompt := chat.draftPrompt()
	if !strings.Contains(prompt, "module example") || !strings.HasPrefix(prompt, "看 @go.mod 里的模块名") {
		t.Fatalf("file context missing: %q", prompt)
	}
	hs.key(0, ui.KeyEnter)
	hs.until("file reply", func() bool { return !chat.busy && chat.turn == 1 })
	requireComposerPrompt(t, logPath, prompt)
}

func TestAgentComposerSlashCommands(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "composer-commands")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	session := chat.sessionID
	hs.clickDetail(locales.AgentDraft())
	hs.tt.Type("original question")
	hs.key(0, ui.KeyEnter)
	hs.until("ordinary reply", func() bool { return chat.turn == 1 && !chat.busy })
	hs.tt.Type("/")
	hs.settle()
	if !hs.tt.HasText("/status") || !hs.tt.HasText("Review changes") {
		t.Fatal("startup command list was lost", chat.commands, hs.tt.Texts())
	}
	hs.shot("agent-slash-commands")
	hs.tt.Type("skill:")
	hs.key(0, ui.KeyEnter)
	if chat.turn != 1 || chat.draft != "/skill:review " || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatalf("selecting command: turn=%d draft=%q", chat.turn, chat.draft)
	}
	hs.tt.Type("main.go")
	hs.key(0, ui.KeyEnter)
	hs.until("slash command reply", func() bool { return chat.turn == 2 && !chat.busy })
	events := agentRoleEvents(t, logPath)
	if !slices.ContainsFunc(events, func(event roleRuntimeEvent) bool {
		return event.Kind == "prompt" && event.Session == session && event.Text == "/skill:review main.go"
	}) {
		t.Fatal("slash command or session changed on the wire", events)
	}
	hs.tt.Type("/")
	hs.settle()
	if _, found := hs.tt.Find("/status"); found || !hs.tt.HasText("/model") {
		t.Fatal("replacing advertised commands should leave only the session controls")
	}
}

func TestAgentSlashCatalogKeepsNativeCommandsAndAllSkills(t *testing.T) {
	chat := &agentChat{sessionID: "current", client: &acp.Client{}, models: []acp.ModelChoice{{ID: "engine"}}}
	for index := range 85 {
		chat.commands = append(chat.commands, acp.Command{Name: fmt.Sprintf("skill:helper-%02d", index), Description: "Review code"})
	}
	chat.commands = append(chat.commands, acp.Command{Name: "$codex-helper"})
	chat.commands = append(chat.commands, acp.Command{Name: "model", Description: "Native model command"}, acp.Command{Name: "review", Hint: "commit"})
	items := chat.slashCommands("")
	if len(items) != 88 || items[0].detail != "Native model command" || items[0].setting != "" || items[1].value != "/review" || items[86].value != "/skill:helper-84" || items[87].value != "/$codex-helper" {
		t.Fatal("advertised controls must win name collisions and precede the complete skill list", len(items), items[0], items[1])
	}
	if matches := chat.slashCommands("review"); len(matches) != 86 || matches[0].value != "/review" {
		t.Fatal("an exact command should precede matching skill descriptions")
	}
	if chat.offersSlashSetting(slashThinking) || chat.offersSlashSetting(slashMode) {
		t.Fatal("offered a selector without adapter choices")
	}
}

func TestAgentComposerSlashLateSkillIsReachable(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	for index := range 90 {
		chat.commands = append(chat.commands, acp.Command{Name: fmt.Sprintf("skill:number-%d", index)})
	}
	hs.clickDetail(locales.AgentDraft())
	hs.tt.Type("/")
	hs.key(0, ui.KeyUp)
	if _, visible := hs.tt.Find("/skill:number-89"); !visible || !hs.tt.HasText("91 / 91") {
		t.Fatal("last candidate was not scrolled into view")
	}
	hs.key(0, ui.KeyEnter)
	if chat.draft != "/skill:number-89 " || chat.turn != 0 {
		t.Fatal("last skill was not selected", chat.draft)
	}
}

func TestAgentComposerSlashTerminalAndFailedSettingsKeepDraft(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "selection-fails")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	chat.adapterName = "pi-acp"
	hs.clickDetail(locales.AgentDraft())
	hs.tt.Type("/tree")
	hs.settle()
	if !hs.tt.HasText(locales.AgentSlashPiTerminal("/tree")) {
		t.Fatal("missing Pi terminal guidance")
	}
	hs.key(0, ui.KeyEscape)
	hs.key(0, ui.KeyEnter)
	if chat.draft != "/tree" || chat.err != locales.AgentSlashPiTerminal("/tree") {
		t.Fatal("terminal action was sent", chat.draft, chat.err)
	}
	for _, name := range []string{"/settings", "/scoped-models"} {
		if !chat.piTerminalOnly(name) {
			t.Fatal("expected a terminal page", name)
		}
	}
	chat.commands = append(chat.commands, acp.Command{Name: "tree"})
	if chat.piTerminalOnly("/tree") {
		t.Fatal("terminal hint masked a real advertised command")
	}
	chat.draft = "/model model-b\nPending notes"
	hs.a.sendChat(chat)
	hs.untilLong("failed change", func() bool { return !chat.configuring })
	if chat.model() == "model-b" || chat.err == "" || chat.draft != "/model model-b\nPending notes" {
		t.Fatal("failed setting lost the draft or previous selection", chat.model(), chat.draft, chat.err)
	}
	chat.draft = "/model model-b"
	hs.a.disconnectChat(chat)
	hs.a.sendChat(chat)
	hs.untilLong("cold slash setup", func() bool { return !chat.connecting && !chat.configuring && chat.sessionID != "" })
	if chat.draft != "/model model-b" || chat.turn != 0 || slices.ContainsFunc(agentRoleEvents(t, logPath), func(event roleRuntimeEvent) bool { return event.Kind == "prompt" }) {
		t.Fatal("reconnecting forwarded a setting to the model")
	}
}

func TestAgentComposerSlashSessionSettings(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "thinking-mixed")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	session := chat.sessionID
	hs.clickDetail(locales.AgentDraft())
	hs.tt.Type("/")
	hs.settle()
	for _, name := range []string{"/model", "/thinking", "/mode"} {
		if _, found := hs.tt.Find(name); !found {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"/settings", "/tree", "/scoped-models"} {
		if _, found := hs.tt.Find(name); found {
			t.Fatalf("invented adapter command %s", name)
		}
	}
	hs.shot("agent-slash-session-controls")
	hs.tt.Type("model")
	hs.key(0, ui.KeyEnter)
	if chat.draft != "/model " || !hs.tt.HasText("model-b") {
		t.Fatal("model selector", chat.draft, hs.tt.Texts())
	}
	hs.tt.Type("model-b")
	hs.key(0, ui.KeyEnter)
	hs.untilLong("model change", func() bool { return chat.modelOverride == "model-b" && !chat.configuring })
	hs.tt.Type("/thinking h")
	hs.settle()
	hs.clickDetail("high")
	hs.untilLong("thinking change", func() bool { return chat.mode == "high" && !chat.configuring })
	if chat.draft != "" || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("value choice should return to the draft")
	}
	chat.draft = "/mode plan\nKeep these notes"
	hs.a.sendChat(chat)
	hs.untilLong("mode change", func() bool { return chat.genericMode == "plan" && !chat.configuring })
	if chat.draft != "Keep these notes" || chat.turn != 0 || chat.sessionID != session {
		t.Fatalf("unexpected draft, turn or session: %q, %d, %q", chat.draft, chat.turn, chat.sessionID)
	}
	chat.draft = "/model unknown-model"
	hs.a.sendChat(chat)
	if chat.err == "" || chat.draft != "/model unknown-model" || chat.configuring || slices.ContainsFunc(agentRoleEvents(t, logPath), func(event roleRuntimeEvent) bool { return event.Kind == "prompt" }) {
		t.Fatal("setting commands must keep invalid input and avoid sending a prompt")
	}
}

func TestAgentCompletionCaretBoundaries(t *testing.T) {
	for _, draft := range []string{"user@host", "look at src/main.go", "see https://example.org", "look at path/@source", "done ", "notes\n/status"} {
		end := utf8.RuneCountInString(draft)
		if token := chatCompletionAt(draft, end, end); token.trigger != 0 {
			t.Fatal(draft, token)
		}
	}
	const draft = "请看 @src/ma.go 然后继续"
	caret := utf8.RuneCountInString("请看 @src/ma")
	token := chatCompletionAt(draft, caret, caret)
	if token.query != "src/ma" {
		t.Fatal(token)
	}
	updated, next := token.insert(draft, "@src/main.go")
	if updated != "请看 @src/main.go 然后继续" || string([]rune(updated)[next:]) != "然后继续" {
		t.Fatal(updated, next)
	}
}

func TestAgentComposerCompletesWithinLongPaste(t *testing.T) {
	hs, _ := agentRoleHarness(t, "composer-commands")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	hs.clickDetail(locales.AgentDraft())
	logs := "\n" + strings.Repeat("2026-11-25 INFO handled user@host /workspace/request\n", 220)
	draft := "/st" + logs + "🧪 @go.m"
	hs.tt.SetClipboard(draft)
	hs.command("paste")
	label := locales.AgentPasteText(strings.Count(draft, "\n") + 1)
	hs.clickDetail(label)
	hs.clickDetail(locales.AgentPasteEdit())
	hs.until("pasted workspace file candidate", func() bool { _, ok := hs.tt.Find("@go.mod"); return ok })
	hs.key(ui.Shift, ui.KeyLeft)
	if hs.tt.HasText(locales.QuickSearchFiles()) || chat.draft != draft {
		t.Fatal("a selected character should close autocomplete without changing the draft")
	}
	hs.key(0, ui.KeyRight)
	hs.key(0, ui.KeyTab)
	fileDraft := "/st" + logs + "🧪 @go.mod "
	if chat.draft != fileDraft || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("file completion changed text outside the token or lost focus")
	}
	hs.key(ui.Ctrl, ui.KeyHome)
	for range len("/st") {
		hs.key(0, ui.KeyRight)
	}
	if !hs.tt.HasText("/status") {
		t.Fatal("slash completion did not follow the cursor to the start of the draft")
	}
	hs.key(0, ui.KeyTab)
	if chat.draft != "/status"+logs+"🧪 @go.mod " || chat.turn != 0 {
		t.Fatal("slash completion changed pasted text outside the token or sent the draft")
	}
}

func TestAgentComposerLongLocalSlashPasteStaysInInput(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "composer-commands")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	hs.clickDetail(locales.AgentDraft())
	rest := strings.Repeat("keep this note\n", 25)
	draft := "/model model-b\n" + rest
	hs.tt.SetClipboard(draft)
	hs.command("paste")
	if chat.draft != draft || len(chat.sources.draft.pastes) != 0 {
		t.Fatal("a local slash instruction moved from the input into a paste attachment")
	}
	hs.key(0, ui.KeyEscape)
	hs.key(0, ui.KeyEnter)
	hs.untilLong("local model applied", func() bool { return !chat.configuring && chat.currentModel == "model-b" })
	if chat.draft != rest || chat.turn != 0 {
		t.Fatalf("applying the setting changed the trailing notes or sent a turn: %q", chat.draft)
	}
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			t.Fatal("a local slash setting became an ACP prompt")
		}
	}
}

func BenchmarkAgentCompletionLargeDraft(b *testing.B) {
	for _, label := range []string{"path", "email"} {
		line := "2026-11-25 INFO handled /workspace/request 🧪\n"
		if label == "email" {
			line = "2026-11-25 INFO handled user@host 🧪\n"
		}
		for _, size := range []int{32 << 10, 256 << 10, 2 << 20} {
			draft := strings.Repeat(line, size/len(line)) + "done"
			caret := utf8.RuneCountInString(draft)
			b.Run(fmt.Sprintf("%s/%d", label, size), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if token := chatCompletionAt(draft, caret, caret); token.trigger != 0 {
						b.Fatal(token)
					}
				}
			})
		}
	}
}

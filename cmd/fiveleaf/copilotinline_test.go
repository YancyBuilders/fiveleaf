package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func enableTestInlineCopilot(hs *harness, reply string) string {
	logPath := configureCopilotTestAgent(hs.t, hs)
	config := hs.a.settings.AgentServers["copilot-backend"]
	config.Env["FIVELEAF_ROLE_RUNTIME"] = "copilot-inline"
	config.Env["FIVELEAF_TEST_INLINE_REPLY"] = reply
	hs.a.settings.CopilotInlineSuggestions = true
	return logPath
}

func waitForInlineSuggestion(hs *harness) {
	hs.t.Helper()
	hs.untilLong("inline suggestion", func() bool {
		_, shown := hs.tt.Find(locales.CopilotInlineAccept())
		return shown
	})
}

func TestInlineCopilotDecisions(t *testing.T) {
	for _, control := range []string{"tab", "escape", "accept-button", "dismiss-button"} {
		t.Run(control, func(t *testing.T) {
			hs, doc := editHarness(t, "notes.txt", "天气。")
			logPath := enableTestInlineCopilot(hs, "一起去散步。\nignored second line")
			hs.place(doc, 2, 2)
			hs.tt.Type("好")
			hs.tt.Type("，")
			step(hs.tt)
			if events := agentRoleEvents(t, logPath); len(events) != 0 {
				t.Fatalf("Agent started before the typing pause: %v", events)
			}
			waitForInlineSuggestion(hs)
			expectText(t, doc, "天气好，。")
			pending := hs.a.inlineCopilot
			if pending.text != "一起去散步" || hs.a.pluginPick.open {
				t.Fatalf("inline text %q, picker %v", pending.text, hs.a.pluginPick.open)
			}
			start, _, _, _ := doc.lines.Caret(pending.caret)
			end, _, _, _ := doc.lines.Caret(pending.caret + 1)
			if end-start < hs.a.font.width(pending.text) {
				t.Fatalf("existing suffix overlaps the ghost: available %f", end-start)
			}
			var prompts int
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind == "prompt" {
					prompts++
					if event.Model != "model-b" || !strings.Contains(event.Text, `"Before":"天气好，"`) || !strings.Contains(event.Text, `"After":"。"`) {
						t.Fatalf("incorrect inline prompt: %+v", event)
					}
				}
			}
			if prompts != 1 {
				t.Fatalf("sent %d prompts for one typing burst", prompts)
			}
			switch control {
			case "tab":
				hs.key(0, ui.KeyTab)
			case "escape":
				hs.key(0, ui.KeyEscape)
			case "accept-button", "dismiss-button":
				label := locales.CopilotInlineAccept()
				if control == "dismiss-button" {
					label = locales.CopilotInlineDismiss()
				}
				if err := hs.tt.Click(label); err != nil {
					t.Fatal(err)
				}
				hs.settle()
			}
			if hs.a.inlineCopilot != nil {
				t.Fatal("decision left the suggestion active")
			}
			if control == "tab" || control == "accept-button" {
				expectText(t, doc, "天气好，一起去散步。")
				expectSel(t, doc, 9, 9)
				hs.key(ui.Cmd, ui.KeyZ)
			}
			expectText(t, doc, "天气好，。")
			hs.key(0, ui.KeyTab)
			if doc.buf.String() == "天气好，。" || strings.Contains(doc.buf.String(), "散步") {
				t.Fatalf("ordinary Tab after decision: %q", doc.buf.String())
			}
		})
	}
}

func TestInlineCopilotCompositionAndOptIn(t *testing.T) {
	hs, doc := editHarness(t, "draft.md", "")
	logPath := enableTestInlineCopilot(hs, "世界")
	hs.a.settings.CopilotInlineSuggestions = false
	hs.tt.Type("# ")
	hs.settle()
	hs.a.settings.CopilotInlineSuggestions = true
	hs.a.setLive(doc, true)
	hs.settle()
	hs.tt.Compose("ni", 2)
	hs.settle()
	if hs.a.inlineCopilot != nil || len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("disabled suggestions or uncommitted IME text started the Agent")
	}
	hs.tt.Type("你好")
	waitForInlineSuggestion(hs)
	expectText(t, doc, "# 你好")
	hs.key(0, ui.KeyTab)
	expectText(t, doc, "# 你好世界")
}

func TestInlineCopilotFollowsMatchingTypingAndNewline(t *testing.T) {
	hs, doc := editHarness(t, "notes.txt", "ab")
	logPath := enableTestInlineCopilot(hs, "cdef")
	hs.place(doc, 2, 2)
	hs.key(0, ui.KeyEnter)
	waitForInlineSuggestion(hs)
	pending := hs.a.inlineCopilot
	hs.tt.TypeKey(0, ui.KeyC, "c")
	step(hs.tt)
	if hs.a.inlineCopilot != pending || pending.text != "def" {
		t.Fatal("typing the suggested prefix lost the rest of the suggestion")
	}
	hs.key(0, ui.KeyTab)
	expectText(t, doc, "ab\ncdef")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, doc, "ab\nc")
	var prompts int
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts++
		}
	}
	if prompts != 1 {
		t.Fatalf("Agent received %d prompts for a matching prefix", prompts)
	}
}

func TestInlineCopilotPendingReplyAtOldCaret(t *testing.T) {
	hs, doc := editHarness(t, "notes.txt", "ab")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	hs.a.settings.AgentServers = map[string]settings.AgentServer{"slow": {Command: executable, Protocol: "cli",
		Args: []string{"-test.run=^TestCopilotPendingCLIProcess$"}, Env: map[string]string{"FIVELEAF_COPILOT_TEST_READY": ready}}}
	hs.a.settings.Copilot, hs.a.settings.CopilotInlineSuggestions = settings.CopilotConfig{Agent: "slow"}, true
	hs.place(doc, 2, 2)
	hs.tt.Type("c")
	hs.untilLong("pending inline request", func() bool { _, err := os.Stat(ready); return err == nil })
	hs.key(0, ui.KeyLeft)
	if err := os.WriteFile(ready+".reply", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.inlineCopilot != nil {
		t.Fatal("old completion survived moving the caret")
	}
	expectText(t, doc, "abc")
	expectSel(t, doc, 2, 2)
}

func TestInlineCopilotWrappedEditor(t *testing.T) {
	prefix := strings.Repeat("word ", 25)
	hs, doc := editHarness(t, "wrap.txt", prefix+"!")
	hs.a.window.SidebarClosed = true
	hs.a.settings.WordWrap = true
	hs.tt.SetSize(360, 320)
	enableTestInlineCopilot(hs, "nto view!")
	hs.place(doc, len(prefix), len(prefix))
	hs.tt.Type("i")
	waitForInlineSuggestion(hs)
	accept, _ := hs.tt.Find(locales.CopilotInlineAccept())
	dismiss, _ := hs.tt.Find(locales.CopilotInlineDismiss())
	if accept.X < 0 || dismiss.X+dismiss.W > 360 || accept.X+accept.W > dismiss.X {
		t.Fatalf("decision buttons outside the narrow editor: %+v, %+v", accept, dismiss)
	}
	_, y, _, ok := doc.lines.Caret(doc.sel[1])
	if !ok || y < hs.a.font.lineH {
		t.Fatalf("ghost caret was not on a wrapped row: %v", y)
	}
	hs.key(0, ui.KeyTab)
	expectText(t, doc, prefix+"into view!")
}

func TestInlineCopilotExclusions(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "public.txt")
	for _, path := range []string{external, filepath.Join(root, "test.go"), filepath.Join(root, ".env"), filepath.Join(root, "notes.txt")} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rules := []string{"/notes.txt"}
	if !inlineFileAllowed(root, filepath.Join(root, "test.go"), rules) {
		t.Fatal("ordinary local file excluded")
	}
	for _, path := range []string{external, filepath.Join(root, ".env"), filepath.Join(root, "notes.txt")} {
		if inlineFileAllowed(root, path, rules) {
			t.Fatalf("allowed %q", path)
		}
	}
	link := filepath.Join(root, "alias.txt")
	if os.Symlink(filepath.Join(root, ".env"), link) == nil && inlineFileAllowed(root, link, rules) {
		t.Fatal("allowed a symlink to a secret file")
	}
	hs, _ := editHarness(t, "private.txt", "")
	logPath := enableTestInlineCopilot(hs, "secret")
	hs.a.settings.AISecretFiles = []string{"private.txt"}
	hs.tt.Type("private data")
	hs.settle()
	hs.untilLong("excluded suggestion discarded", func() bool { return hs.a.inlineCopilot == nil })
	if len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("Agent started for excluded file")
	}
}

func inlineCopilotFixture(hs *harness) {
	const example = "package example\n\nfunc clamp(value int) int {\n\treturn \n}\n"
	path := hs.path("clamp.go")
	if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
		hs.t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	enableTestInlineCopilot(hs, "ax(value, 0)")
	caret := utf8.RuneCountInString(strings.Split(example, "return ")[0] + "return ")
	hs.place(hs.a.activeEditor(), caret, caret)
	hs.tt.Type("m")
	waitForInlineSuggestion(hs)
}

func TestShotInlineCopilot(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	hs := newHarness(t, false)
	inlineCopilotFixture(hs)
	hs.shot("inline-copilot")
}

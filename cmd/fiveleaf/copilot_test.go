package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func configureCopilotTestAgent(t testing.TB, hs *harness) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "copilot.jsonl")
	hs.a.settings.AgentServers = map[string]settings.AgentServer{
		"chat": {Command: "unused-chat-agent"},
		"copilot-backend": {Command: executable, Args: []string{"-test.run=^TestAgentRoleRuntimeProcess$"},
			Env: map[string]string{"FIVELEAF_ROLE_RUNTIME": "roundtable", "FIVELEAF_ROLE_LOG": logPath}},
	}
	hs.a.settings.Copilot = settings.CopilotConfig{Agent: "copilot-backend", Model: "model-b"}
	hs.a.agent.provider, hs.a.agent.draft = "chat", "chat-only text"
	return logPath
}

func TestCopilotEditorSuggestionIsBoundToSelection(t *testing.T) {
	const original = "想法：旧。\n"
	hs, editor := editHarness(t, "example.txt", original)
	logPath := configureCopilotTestAgent(t, hs)
	hs.place(editor, 3, 4)
	suggest := func() {
		hs.a.run("sel.copilotComplete")
		hs.untilLong("Copilot suggestion", func() bool { return hs.a.pluginPick.open })
		if got := editor.buf.String(); got != original {
			t.Fatalf("suggestion changed the document before acceptance: %q", got)
		}
	}
	suggest()
	hs.a.settings.Copilot.Model = "model-a"
	hs.a.closePluginPicker(0)
	hs.settle()
	if editor.buf.String() != original {
		t.Fatal("accepted the previous model's suggestion after a settings change")
	}
	hs.a.settings.Copilot.Model = "model-b"
	suggest()
	hs.place(editor, 1, 1)
	hs.a.closePluginPicker(0)
	hs.settle()
	if editor.buf.String() != original {
		t.Fatal("stale suggestion changed the document")
	}
	hs.place(editor, 3, 4)
	suggest()
	hs.a.closePluginPicker(0)
	hs.settle()
	if got := editor.buf.String(); got != "想法：Rollback is covered.。\n" {
		t.Fatalf("accepted suggestion: %q", got)
	}
	var prompts int
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts++
			if event.Model != "model-b" || !strings.Contains(event.Text, `"Before":"想法："`) || !strings.Contains(event.Text, `"After":"。\n"`) || strings.Contains(event.Text, "chat-only text") {
				t.Fatalf("unexpected editor request: %+v", event)
			}
		}
	}
	if prompts != 3 || hs.a.agent.provider != "chat" || hs.a.agent.draft != "chat-only text" {
		t.Fatal("Copilot requests and chat state were not independent")
	}
}

func TestCopilotDraftUsesSelectedAgentAndRedactedDiff(t *testing.T) {
	hs := newHarness(t, false)
	git := hs.gitRepo()
	logPath := configureCopilotTestAgent(t, hs)
	hs.a.settings.AISecretFiles = []string{"private-notes.txt"}
	for name, content := range map[string]string{"hello.txt": "included change\n", ".env": "PRIVATE_DRAFT_TEST_VALUE=keep-out\n", "private-notes.txt": "excluded-by-custom-rule\n"} {
		if err := os.WriteFile(hs.path(name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "hello.txt", ".env", "private-notes.txt")
	head := git("rev-parse", "HEAD")
	hs.a.showSourceControl()
	hs.settleGit()
	hs.a.git.message = "author notes"
	if err := hs.tt.Click(locales.CopilotCommit()); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("Copilot commit draft", func() bool { return hs.a.git.message == "Rollback is covered." })
	var found bool
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			found = true
			if event.Model != "model-b" || !strings.Contains(event.Text, "author notes") || !strings.Contains(event.Text, "included change") || strings.Contains(event.Text, "keep-out") || strings.Contains(event.Text, "excluded-by-custom-rule") {
				t.Fatalf("unexpected commit request: %+v", event)
			}
		}
	}
	if !found || git("rev-parse", "HEAD") != head {
		t.Fatal("draft did not come from a generation-only request")
	}
	hs.a.run("git.copilotCommit")
	hs.a.git.message = "newer draft"
	hs.untilLong("discarded stale commit draft", func() bool { return hs.a.copilot == nil })
	if hs.a.git.message != "newer draft" {
		t.Fatal("generation overwrote an edit made while waiting")
	}
}

func TestCopilotAcceptsWhitespaceOnlySuggestion(t *testing.T) {
	hs, editor := editHarness(t, "example.txt", "// notereturn")
	configureCopilotTestAgent(t, hs)
	hs.a.settings.AgentServers["copilot-backend"].Env["FIVELEAF_ROLE_RUNTIME"] = "copilot-newline"
	hs.place(editor, 7, 7)
	hs.a.run("sel.copilotComplete")
	hs.untilLong("newline suggestion", func() bool { return hs.a.pluginPick.open })
	hs.a.closePluginPicker(0)
	hs.settle()
	if got := editor.buf.String(); got != "// note\nreturn" {
		t.Fatalf("newline completion: %q", got)
	}
}

func TestCopilotAgentReceivesSelectedModel(t *testing.T) {
	for _, scenario := range []string{"acp", "cli"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, scenario)
			provider := findChatProvider("backend", hs.a.agentProviders())
			if scenario == "cli" {
				provider.config.Args = append(slices.Clone(provider.config.Args), "--", "--model={model}")
			}
			request := copilotRequest{selection: settings.CopilotConfig{Agent: "backend", Model: "model-b"}, provider: provider, folder: hs.a.folderKey()}
			reply, err := request.generate(context.Background(), "a synthetic Copilot request")
			if err != nil || !strings.Contains(reply, "a synthetic Copilot request") {
				t.Fatalf("reply %q, error %v", reply, err)
			}
			if scenario == "cli" && !strings.HasSuffix(reply, "\n") {
				t.Fatal("CLI completion lost the newline emitted by the agent")
			}
			var found bool
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind == "prompt" && event.Model == "model-b" || event.Kind == "cli" && slices.Contains(event.Args, "--model=model-b") {
					found = true
				}
			}
			if !found {
				t.Fatal("chosen model did not reach the agent")
			}
			if scenario == "acp" {
				request.selection.Model = "unsupported"
				if _, err := request.generate(context.Background(), "must not be sent"); err == nil {
					t.Fatal("unsupported model fell back")
				}
				request.selection.Model = ""
				if _, err := request.generate(context.Background(), "use provider default"); err != nil {
					t.Fatal(err)
				}
				var usedDefault bool
				for _, event := range agentRoleEvents(t, logPath) {
					if event.Text == "must not be sent" {
						t.Fatal("sent prompt after model selection failed")
					}
					usedDefault = usedDefault || event.Text == "use provider default" && event.Model == "default"
				}
				if !usedDefault {
					t.Fatal("empty selection did not use the provider default")
				}
			}
		})
	}
}

func TestCopilotSkipsDeclaredACPStartupMessage(t *testing.T) {
	hs, _ := agentRoleHarness(t, "late-startup")
	request := copilotRequest{provider: findChatProvider("backend", hs.a.agentProviders()), folder: hs.a.folderKey()}
	got, err := request.generate(t.Context(), "only this answer")
	if err != nil || got != "only this answer" {
		t.Fatalf("generated text %q, %v", got, err)
	}
}

func TestCopilotPendingCLIProcess(t *testing.T) {
	ready := os.Getenv("FIVELEAF_COPILOT_TEST_READY")
	if ready == "" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil || os.WriteFile(ready, []byte(cwd), 0o600) != nil {
		os.Exit(2)
	}
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(ready + ".reply"); err == nil {
			_, _ = os.Stdout.WriteString("old draft\n")
			os.Exit(0)
		}
		time.Sleep(5 * time.Millisecond)
	}
	os.Exit(2)
}

func TestCopilotGitDraftChecksDiffAndUsesProjectDirectory(t *testing.T) {
	hs := newHarness(t, false)
	git := hs.gitRepo()
	project := hs.path("project")
	tools := filepath.Join(project, "tools")
	if err := os.MkdirAll(tools, 0o755); err != nil {
		t.Fatal(err)
	}
	stagedFile := filepath.Join(project, "example.txt")
	if err := os.WriteFile(stagedFile, []byte("original change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "project/example.txt")
	hs.a.openFolder(project)
	hs.settleGit()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "requested")
	hs.a.settings.AgentServers = map[string]settings.AgentServer{"draft": {Command: executable, Protocol: "cli", WorkingDirectory: "tools",
		Args: []string{"-test.run=^TestCopilotPendingCLIProcess$"}, Env: map[string]string{"FIVELEAF_COPILOT_TEST_READY": ready}}}
	hs.a.settings.Copilot = settings.CopilotConfig{Agent: "draft"}
	hs.a.run("git.copilotCommit")
	hs.untilLong("Agent in the project's working directory", func() bool { _, err := os.Stat(ready); return err == nil })
	got, err := os.ReadFile(ready)
	if err != nil {
		t.Fatal(err)
	}
	gotDir, _ := filepath.EvalSymlinks(string(got))
	wantDir, _ := filepath.EvalSymlinks(tools)
	if gotDir != wantDir {
		t.Fatalf("Agent cwd %q, want %q", gotDir, wantDir)
	}
	if err := os.WriteFile(stagedFile, []byte("different change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "project/example.txt")
	if err := os.WriteFile(ready+".reply", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("discarded stale Git diff", func() bool { return hs.a.copilot == nil })
	if draft := hs.a.repository().git.message; draft != "" {
		t.Fatalf("staged changes were replaced but received %q", draft)
	}
}

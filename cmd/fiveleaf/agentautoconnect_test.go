package main

import (
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/internal/remote"
	"fiveleaf/locales"
)

func requireChatHandshakes(t *testing.T, logPath string, count int) {
	t.Helper()
	events := map[string]int{}
	for _, event := range agentRoleEvents(t, logPath) {
		events[event.Kind]++
	}
	if events["start"] != count || events["new"] != count || events["prompt"] != 0 {
		t.Fatalf("opening chats should create %d connections and no prompts, got %v", count, events)
	}
}

func TestChatAutoConnectOncePerTarget(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	a, chat := hs.a, hs.a.agent
	chat.draft = "keep this question unsent"
	a.launchAgent()
	a.launchAgent()
	hs.untilLong("initial connection", func() bool { return chat.sessionID != "" })
	a.openAgentPage()
	hs.settle()
	requireChatHandshakes(t, logPath, 1)
	if _, found := hs.tt.Find(locales.AgentConnect()); found || chat.draft != "keep this question unsent" || len(chat.messages) != 0 {
		t.Fatal("opening added a connect control or changed the unsent draft")
	}
	a.splitRight()
	sibling := a.agent
	hs.untilLong("split connection", func() bool { return sibling.sessionID != "" })
	if sibling == chat || sibling.client == chat.client || sibling.draft != "" || sibling.hasRole() {
		t.Fatal("the split did not get an independent connection and draft")
	}
	a.settings.AgentServers["other"] = a.settings.AgentServers[sibling.provider]
	a.selectChatAgent(sibling, "other")
	direct := a.agent
	hs.untilLong("replacement connection", func() bool { return direct.sessionID != "" })
	a.selectChatAgent(direct, sibling.provider)
	a.openChatPage(chat)
	requireChatHandshakes(t, logPath, 3)
}

func TestChatAutoConnectFailureNeedsExplicitRetry(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	a, chat := hs.a, hs.a.agent
	working := a.settings.AgentServers["backend"]
	broken := working
	broken.Command = filepath.Join(t.TempDir(), "missing-adapter")
	a.settings.AgentServers["backend"] = broken
	chat.draft = "leave this here"
	a.openAgentPage()
	hs.untilLong("failed connection", func() bool { return chat.err != "" && !chat.connecting })
	generation := chat.generation
	a.openAgentPage()
	if chat.generation != generation || chat.connecting {
		t.Fatal("focusing a failed chat retried the connection")
	}
	a.settings.AgentServers["backend"] = working
	hs.settle()
	if err := hs.tt.Click(locales.AgentReconnect()); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("explicit retry", func() bool { return chat.sessionID != "" })
	requireChatHandshakes(t, logPath, 1)
	if chat.draft != "leave this here" {
		t.Fatalf("retry changed the draft: %q", chat.draft)
	}
}

func TestChatAutoConnectHonorsStopAndRemote(t *testing.T) {
	hs, logPath, gate := lifecycleHarness(t, "hold-first-new")
	a, chat := hs.a, hs.a.agent
	a.root = remote.Path("example", "/workspace")
	a.openAgentPage()
	if chat.connecting || chat.connectionAttempted || len(lifecycleEvents(t, logPath)) != 0 {
		t.Fatal("a remote workspace attempted to launch a local agent")
	}
	a.root = hs.root
	a.openAgentPage()
	hs.untilLong("pending first session", func() bool { return lifecycleHas(lifecycleEvents(t, logPath), "new", "lifecycle-1") })
	chat.draft = "keep this question unsent"
	generation := chat.generation
	a.stopChat(chat)
	if !chat.connecting || !chat.stopping || chat.generation != generation {
		t.Fatal("Stop replaced or abandoned the pending connection")
	}
	a.openAgentPage()
	releaseLifecycleGate(t, gate)
	hs.untilLong("initial session after Stop", func() bool { return !chat.connecting })
	events := lifecycleEvents(t, logPath)
	if chat.generation != generation || chat.client == nil || chat.sessionID != "lifecycle-1" || chat.stopping ||
		chat.draft != "keep this question unsent" || len(events) != 1 {
		t.Fatalf("opening the stopped chat changed the draft or connection: draft=%q session=%q events=%+v", chat.draft, chat.sessionID, events)
	}
}

func TestChatAutoConnectImportedHistory(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	a := hs.a
	source := continuationSource(t, hs.root)
	a.continueSessionWith(source, "backend")
	chat := a.agent
	if chat.connectionAttempted || chat.contextLoading != 1 {
		t.Fatal("connection started before history was loaded")
	}
	a.closeActive()
	hs.untilLong("import while tab closed", func() bool { return chat.continuationReady() })
	requireChatHandshakes(t, logPath, 0)
	if chat.connectionAttempted {
		t.Fatal("import completion started an agent for a closed page")
	}
	a.continueSessionWith(source, "backend")
	hs.untilLong("imported chat connected", func() bool { return chat.sessionID != "" })
	if chat.draft != locales.SessionsContinueDraft() || len(chat.messages) != 2 {
		t.Fatal("connecting changed the continuation draft or transcript")
	}
	requireChatHandshakes(t, logPath, 1)
}

func TestChatAutoConnectCLIDoesNotRun(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "cli")
	a := hs.a
	a.selectAgent("backend")
	a.openAgentPage()
	a.splitRight()
	hs.settle()
	if a.agent.connected == nil || len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("opening a CLI chat should prepare the provider without running it")
	}
}

func TestChatAutoConnectWhenEditorClosesOverImportedChat(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	a := hs.a
	a.continueSessionWith(continuationSource(t, hs.root), "backend")
	chat := a.agent
	a.openFile(hs.path("main.go"), true)
	hs.untilLong("background history loaded", chat.continuationReady)
	if chat.connectionAttempted {
		t.Fatal("hidden chat attempted to connect")
	}
	a.closeActive()
	if !chat.connecting || a.activeTab().tool.chat != chat {
		t.Fatal("closing the editor did not start the newly visible chat")
	}
	hs.untilLong("connection on reveal", func() bool { return chat.sessionID != "" })
	requireChatHandshakes(t, logPath, 1)
}

func TestChatAutoConnectSeparatesAdapterWelcomeFromHistory(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "welcome-on-open")
	a := hs.a
	a.continueSessionWith(continuationSource(t, hs.root), "backend")
	chat := a.agent
	hs.untilLong("adapter greeting", func() bool { return chat.sessionID != "" && !chat.connecting })
	requireChatHandshakes(t, logPath, 1)
	const detail = "Adapter startup details marker"
	hs.settle()
	if len(chat.messages) != 3 || strings.Contains(chat.messages[1].text, detail) || chat.messages[2].role != "connection" || !hs.tt.HasText(detail) {
		t.Fatal("opening the chat should display adapter output after the imported history")
	}
	lifecycleSend(t, hs, "Continue the task")
	events := agentRoleEvents(t, logPath)
	if last := events[len(events)-1]; last.Kind != "prompt" || strings.Contains(last.Text, detail) || !strings.Contains(last.Text, "Continue the task") {
		t.Fatal("the first prompt included adapter startup details or lost the new question")
	}
}

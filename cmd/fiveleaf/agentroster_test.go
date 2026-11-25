package main

import (
	"os"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
)

func agentRosterHarness(t *testing.T) *harness {
	t.Helper()
	hs := newHarness(t, true)
	hs.a.settings.AgentServers = map[string]settings.AgentServer{
		"alpha": roundtableTestServer(t, "Alpha", "acp"),
		"beta":  {Name: "Beta", Command: "test-cli", Protocol: "cli", Capabilities: []string{"search"}},
	}
	hs.a.openAgentPage()
	hs.until("roster connection", func() bool { return hs.a.agent.sessionID != "" })
	sizeChatWorkspace(hs, 600)
	hs.settle()
	return hs
}

func chooseChatTarget(t *testing.T, hs *harness, name string) {
	t.Helper()
	if err := hs.tt.Click(locales.AgentChoose()); err != nil {
		t.Fatal(err)
	}
	for _, item := range hs.openAgentMenuItems() {
		if item != nil && item.label == name && item.status.icon != nil {
			if err := hs.tt.Click(name + " · " + item.status.label); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			return
		}
	}
	t.Fatalf("target %q not found in open Agent menu", name)
}

func TestAgentRosterKeepsIndependentConversations(t *testing.T) {
	hs := agentRosterHarness(t)
	a := hs.a
	alpha := a.agent
	alpha.draft, alpha.busy = "Alpha draft", true
	alpha.messages = []*agentMessage{{role: "assistant", text: "Alpha history"}}
	hs.settle()
	chooseChatTarget(t, hs, "Beta")
	beta := a.agent
	if beta == alpha || beta.provider != "beta" || !alpha.busy || alpha.draft != "Alpha draft" {
		t.Fatal("switching agents destroyed a running conversation")
	}
	beta.draft = "Beta draft"
	chooseChatTarget(t, hs, "Alpha")
	if a.agent != alpha || a.agent.draft != "Alpha draft" || a.agentState("beta") != beta || beta.draft != "Beta draft" || !hs.tt.HasText("Alpha history") {
		t.Fatal("returning to an agent lost its draft, history or independent state")
	}
	alpha.busy = false
	a.setAgentEnabled("alpha", false)
	hs.settle()
	if err := hs.tt.Click(locales.AgentChoose()); err != nil {
		t.Fatal(err)
	}
	if !a.settings.AgentServers["alpha"].Disabled || !hs.tt.HasText("Alpha · "+locales.AgentDisabled()) {
		t.Fatal("a disabled agent did not show its disabled state in the menu")
	}
	if alpha.client != nil || beta.client != nil || beta.turn != 0 || alpha.turn != 0 {
		t.Fatal("disabling the ACP target left it connected, or switching targets sent a prompt")
	}
}

func TestAgentChatHasNoRosterOrEmbeddedSettings(t *testing.T) {
	hs := agentRosterHarness(t)
	for _, width := range []float32{1000, 600, 280} {
		sizeChatWorkspace(hs, width)
		hs.settle()
		for _, label := range []string{locales.AgentRoster(), locales.AgentShowRoster(), locales.AgentHideRoster(), locales.AgentSystemPrompt(), locales.AgentContextMessages(), locales.AgentClearChat(), "Beta"} {
			if _, found := hs.tt.Find(label); found {
				t.Errorf("%v-wide chat still shows %q", width, label)
			}
		}
		for _, label := range []string{locales.AgentChoose(), locales.AgentDraft(), locales.AgentSend()} {
			bounds, found := hs.tt.Find(label)
			if !found || bounds.X < 0 || bounds.X+bounds.W > width+1 {
				t.Errorf("chat control %q is outside the workspace at %v: %+v", label, width, bounds)
			}
		}
	}
}

func TestAgentRosterBusyMenuShowsWaitingPermission(t *testing.T) {
	hs := agentRosterHarness(t)
	sizeChatWorkspace(hs, 280)
	alpha := hs.a.agent
	alpha.busy = true
	alpha.permissions = []*agentPermission{{title: "Approval", options: []acp.PermissionOption{{OptionId: "yes", Kind: "allow_once", Name: "Allow"}}}}
	hs.settle()
	if err := hs.tt.Click(locales.AgentChoose()); err != nil {
		t.Fatal(err)
	}
	if !hs.tt.HasText("Alpha · " + locales.AgentPermission()) {
		t.Fatal("collapsed roster hides a waiting permission")
	}
	if err := hs.tt.Click("Beta · " + locales.AgentOffline()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.agent.provider != "beta" || !alpha.busy || len(alpha.permissions) != 1 {
		t.Fatal("switching through the busy dropdown disturbed the waiting agent")
	}
}

func TestShotAgentRoster(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	for _, state := range []struct {
		name, locale string
		dark         bool
	}{
		{"agent-roster-dark", "en", true},
		{"agent-roster-zh", "zh-Hans", false},
	} {
		t.Run(state.name, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return state.locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			hs := agentRosterHarness(t)
			hs.tt.SetDark(state.dark)
			hs.a.agent.messages = []*agentMessage{
				{role: "user", text: "Review the current changes and explain the main function."},
				{role: "assistant", text: "## Review\n\nThe entry point writes a greeting.\n\n```go\nfmt.Println(\"hello, fiveleaf\")\n```\n\nThe change preserves the existing behavior."},
			}
			hs.a.agent.sessionID = "preview"
			hs.settle()
			hs.shot(state.name)
		})
	}
}

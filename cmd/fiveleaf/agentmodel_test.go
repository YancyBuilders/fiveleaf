package main

import (
	"strings"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func connectAgentModelTest(t *testing.T, hs *harness) {
	t.Helper()
	hs.a.showAgent()
	if chat := hs.a.agent; !chat.connecting && chat.client == nil {
		hs.a.connectAgent()
	}
	hs.untilLong("model connection", func() bool { return !hs.a.agent.connecting })
	if s := hs.a.agent; s.err != "" || s.sessionID == "" || len(s.models) == 0 {
		t.Fatalf("model connection: error=%q session=%q models=%v", s.err, s.sessionID, s.models)
	}
	hs.settle()
}

func chooseAgentModelTest(t *testing.T, hs *harness, id string) {
	t.Helper()
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if err := hs.tt.Click(id); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.untilLong("model selection", func() bool { return !hs.a.agent.configuring })
	if hs.a.agent.modelPicker.open {
		hs.key(0, ui.KeyEscape)
		hs.settle()
	}
}

func TestAgentModelPickerKeepsChoiceAcrossSessions(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	a, chat := hs.a, hs.a.agent
	a.openAgentPage()
	connectAgentModelTest(t, hs)
	chooseAgentModelTest(t, hs, "model-b")
	if chat.err != "" || chat.modelOverride != "model-b" || chat.currentModel != "model-b" || !hs.tt.HasText("B") {
		t.Fatalf("model picker did not confirm and show B: override=%q current=%q error=%q", chat.modelOverride, chat.currentModel, chat.err)
	}
	for _, text := range []string{"first B reply", "second B reply"} {
		lifecycleSend(t, hs, text)
		if chat.err != "" || chat.model() != "model-b" {
			t.Fatalf("next turn lost B: model=%q error=%q", chat.model(), chat.err)
		}
		a.newFile()
		hs.settle()
		a.openChatPage(chat)
		hs.settle()
		if a.agent != chat || chat.modelOverride != "model-b" {
			t.Fatal("moving the conversation lost its selected model")
		}
	}
	a.newAgentSession()
	if chat.modelOverride != "model-b" || chat.currentModel != "" || len(chat.models) != 0 {
		t.Fatal("New Chat lost the user's model or retained the old adapter snapshot")
	}
	connectAgentModelTest(t, hs)
	lifecycleSend(t, hs, "third B reply")
	a.disconnectChat(chat)
	connectAgentModelTest(t, hs)
	lifecycleSend(t, hs, "reconnected B reply")
	if chat.err != "" || a.settings.AgentRoles[chat.role].Model != "model-a" {
		t.Fatalf("conversation selection mutated its role or failed: error=%q role=%+v", chat.err, a.settings.AgentRoles[chat.role])
	}
	prompts := 0
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts++
			if event.Model != "model-b" {
				t.Fatalf("manual choice was replaced by the role/default model: %+v", event)
			}
		}
	}
	if prompts != 4 {
		t.Fatalf("sent %d prompts, want 4", prompts)
	}
}

func TestAgentModelFailedSelectionKeepsPreviousModel(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "selection-fails")
	connectAgentModelTest(t, hs)
	chooseAgentModelTest(t, hs, "model-b")
	chat := hs.a.agent
	if chat.err == "" || chat.modelOverride != "" || chat.currentModel != "model-a" || chat.model() != "model-a" {
		t.Fatalf("failed selection replaced A: override=%q current=%q error=%q", chat.modelOverride, chat.currentModel, chat.err)
	}
	lifecycleSend(t, hs, "keep A after failure")
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" && event.Model != "model-a" {
			t.Fatalf("failed selection affected a later prompt: %+v", event)
		}
	}
}

func TestAgentModelUnconfirmedSelectionRefreshesOptions(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "wrong-current-options")
	connectAgentModelTest(t, hs)
	chooseAgentModelTest(t, hs, "model-a")
	chat := hs.a.agent
	chat.draft = "keep this question"
	chooseAgentModelTest(t, hs, "model-b")
	if chat.err == "" || chat.modelOverride != "model-a" || chat.currentModel != "model-a" || chat.draft != "keep this question" {
		t.Fatalf("unconfirmed selection lost the chosen model, error or draft: override=%q current=%q error=%q draft=%q", chat.modelOverride, chat.currentModel, chat.err, chat.draft)
	}
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	_, foundC := hs.tt.Find("model-c")
	_, foundB := hs.tt.Find("model-b")
	if !foundC || foundB {
		t.Fatal("unconfirmed response did not replace the stale model choices")
	}
	if err := hs.tt.Click("model-c"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.untilLong("replacement model selection", func() bool { return !chat.configuring })
	if chat.err != "" || chat.modelOverride != "model-c" || chat.currentModel != "model-c" || chat.draft != "keep this question" {
		t.Fatalf("newly advertised C could not recover the draft: override=%q current=%q error=%q draft=%q", chat.modelOverride, chat.currentModel, chat.err, chat.draft)
	}
	lifecycleSend(t, hs, chat.draft)
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	events := agentRoleEvents(t, logPath)
	if last := events[len(events)-1]; last.Kind != "prompt" || last.Model != "model-c" || !strings.Contains(last.Text, "keep this question") {
		t.Fatalf("recovered question did not use C: %+v", last)
	}
}

func TestAgentModelPickerRecoversAfterAuthentication(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "auth-model-unavailable")
	a, chat := hs.a, hs.a.agent
	a.showAgent()
	chat.draft = "keep authenticated question"
	a.connectAgent()
	hs.untilLong("authentication required", func() bool { return !chat.connecting })
	if chat.err == "" || chat.client == nil || chat.sessionID != "" {
		t.Fatal("the fixture did not require authentication before session creation")
	}
	if err := hs.tt.Click(locales.AgentAuthenticate() + ": Log in"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.untilLong("authenticated model rejection", func() bool { return !chat.connecting })
	if chat.err == "" || chat.sessionID == "" || chat.draft != "keep authenticated question" || chat.modelOverride != "" {
		t.Fatalf("model rejection after authentication left no recoverable session: session=%q error=%q draft=%q override=%q", chat.sessionID, chat.err, chat.draft, chat.modelOverride)
	}
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			t.Fatal("authentication sent a question with an unavailable model")
		}
	}
	chooseAgentModelTest(t, hs, "model-c")
	if chat.err != "" || chat.modelOverride != "model-c" || chat.draft != "keep authenticated question" {
		t.Fatalf("authenticated picker could not choose C: error=%q override=%q draft=%q", chat.err, chat.modelOverride, chat.draft)
	}
	lifecycleSend(t, hs, chat.draft)
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	events := agentRoleEvents(t, logPath)
	if last := events[len(events)-1]; last.Kind != "prompt" || last.Model != "model-c" || !strings.Contains(last.Text, "keep authenticated question") {
		t.Fatalf("authenticated question did not use the corrected model: %+v", last)
	}
}

func TestAgentModelPickerRecoversUnavailableSelection(t *testing.T) {
	for _, scenario := range []string{"initial", "fresh-turn"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, "second-model-removed")
			a, chat := hs.a, hs.a.agent
			wantPrompts := 0
			if scenario == "initial" {
				a.settings.AgentRoles[chat.role] = settings.AgentRole{Agent: "backend", Model: "unavailable"}
				a.openAgentPage()
				hs.untilLong("initial model rejected", func() bool { return !chat.connecting && chat.err != "" })
			} else {
				connectAgentModelTest(t, hs)
				chooseAgentModelTest(t, hs, "model-b")
				lifecycleSend(t, hs, "first B succeeds")
				if chat.err != "" {
					t.Fatal(chat.err)
				}
				wantPrompts = 1
			}
			lifecycleSend(t, hs, "keep this question")
			if chat.err == "" || chat.draft != "keep this question" || chat.sessionID == "" || len(chat.models) == 0 {
				t.Fatalf("unavailable model has no recoverable selection: error=%q draft=%q session=%q choices=%v", chat.err, chat.draft, chat.sessionID, chat.models)
			}
			prompts := 0
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind == "prompt" {
					prompts++
				}
			}
			if prompts != wantPrompts {
				t.Fatal("an unavailable selection still sent a prompt")
			}
			chooseAgentModelTest(t, hs, "model-a")
			if chat.err != "" || chat.modelOverride != "model-a" || chat.draft != "keep this question" {
				t.Fatalf("picking an available model did not retain the pending question: error=%q draft=%q model=%q", chat.err, chat.draft, chat.modelOverride)
			}
			lifecycleSend(t, hs, chat.draft)
			if chat.err != "" {
				t.Fatal(chat.err)
			}
			events := agentRoleEvents(t, logPath)
			last := events[len(events)-1]
			if last.Kind != "prompt" || last.Model != "model-a" {
				t.Fatalf("corrected model was not used: %+v", last)
			}
		})
	}
}

func TestAgentModelDefaultsAndHiddenChoiceStayIndependent(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	a := hs.a
	a.selectAgent("backend")
	direct := a.agent
	connectAgentModelTest(t, hs)
	for _, text := range []string{"default first", "default second"} {
		lifecycleSend(t, hs, text)
		if direct.err != "" || direct.model() != "default" || direct.modelOverride != "" {
			t.Fatalf("direct default was lost or became an override: %+v", direct)
		}
	}
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" && event.Model != "default" {
			t.Fatalf("direct default was not preserved across a fresh turn: %+v", event)
		}
	}
	a.setAgentModel("model-b")
	a.selectAgentRole("审阅 角色")
	role := a.agent
	hs.untilLong("hidden model selection", func() bool { return !direct.configuring })
	if direct.err != "" || direct.modelOverride != "model-b" || a.agent != role || role.modelOverride != "" {
		t.Fatal("a completed model change moved to the newly selected conversation")
	}
	connectAgentModelTest(t, hs)
	if role.model() != "model-a" || direct.model() != "model-b" {
		t.Fatal("direct and role conversations shared their model selection")
	}
	a.selectAgent("backend")
	if a.agent != direct || direct.model() != "model-b" {
		t.Fatal("returning to the direct conversation lost its model")
	}
}

func TestAgentModelRefreshesServerUpdates(t *testing.T) {
	hs, _ := agentRoleHarness(t, "model-notify")
	hs.a.selectAgent("backend")
	connectAgentModelTest(t, hs)
	lifecycleSend(t, hs, "update model")
	if s := hs.a.agent; s.err != "" || s.currentModel != "model-b" || s.modelOverride != "" {
		t.Fatalf("server model update was lost or became a user override: current=%q override=%q error=%q", s.currentModel, s.modelOverride, s.err)
	}
}

func TestAgentModelPickerFitsNarrowViews(t *testing.T) {
	hs, _ := agentRoleHarness(t, "mode-resets-model")
	hs.a.openAgentPage()
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	s.models = []acp.ModelChoice{{ID: "model-a", Name: strings.Repeat("A long model name ", 8)}, {ID: "model-b", Name: "B"}}
	for _, width := range []float32{600, 340, 200, 160} {
		sizeChatWorkspace(hs, width)
		hs.settle()
		for _, label := range []string{locales.AgentModel(), locales.AgentMode(), locales.AgentDraft(), locales.AgentSend()} {
			bounds, found := hs.tt.Find(label)
			if !found || bounds.X < 0 || bounds.X+bounds.W > width+1 || bounds.W <= 0 || bounds.H <= 0 {
				t.Errorf("%q does not fit width %v: %+v (found=%v)", label, width, bounds, found)
			}
		}
	}
	s.busy = true
	hs.settle()
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	if s.modelPicker.open {
		t.Fatal("model picker opened while a reply was running")
	}
	s.busy = false
	hs.a.disconnectChat(s)
	hs.settle()
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	if s.modelPicker.open {
		t.Fatal("disconnected model picker offered stale adapter choices")
	}
}

func TestAgentModelPickerDistinguishesSameNames(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	connectAgentModelTest(t, hs)
	hs.a.agent.models = []acp.ModelChoice{{ID: "model-a", Name: "Engine"}, {ID: "model-b", Name: "Engine"}}
	hs.settle()
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Engine · model-a", "Engine · model-b"} {
		if _, found := hs.tt.Find(label); !found {
			t.Fatalf("model picker omitted distinct label %q", label)
		}
	}
	if err := hs.tt.Click("Engine · model-b"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.untilLong("model with shared name", func() bool { return !hs.a.agent.configuring })
	if s := hs.a.agent; s.err != "" || s.modelOverride != "model-b" {
		t.Fatalf("shared model name selected the wrong ID: override=%q error=%q", s.modelOverride, s.err)
	}
}

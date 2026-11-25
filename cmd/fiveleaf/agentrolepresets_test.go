package main

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func roundtableInvitation(t *testing.T, project *app, id string) meetingSpeaker {
	t.Helper()
	for _, speaker := range project.meetingInvitations() {
		if speaker.ID == id {
			return speaker
		}
	}
	t.Fatalf("missing %q from roundtable invitations", id)
	return meetingSpeaker{}
}

func TestSpecialistRolePresetsInSettingsAndRoundtable(t *testing.T) {
	ph := prefsRoleHarness(t)
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
	ph.m.s.AgentServers["local"] = settings.AgentServer{Protocol: "cli", Command: "unused"}
	ph.frames()
	for _, role := range []struct{ id, name string }{
		{"requirements", "需求分析师"}, {"interaction", "交互设计师"},
		{"compatibility", "兼容性维护者"}, {"moderate", "圆桌主持人"},
	} {
		t.Run(role.id, func(t *testing.T) {
			speaker := roundtableInvitation(t, ph.a, "preset:"+role.id)
			lead, body, _ := strings.Cut(speaker.role.Prompt, "\n")
			if speaker.problem != "" || speaker.Name != role.name || lead == "" || body == "" || !ph.pt.HasText(lead) {
				t.Fatalf("role missing its localized name or prompt in settings or invitations: %+v", speaker)
			}
			prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+role.name)
			if draft := ph.w.roleDraft; draft == nil || draft.preset == nil || draft.preset.id != role.id || draft.prompt != speaker.role.Prompt {
				t.Fatal("the role editor and the invited member must use the same preset prompt")
			}
			prefsAgentClick(t, ph, locales.ButtonCancel())
		})
	}
}

func TestExistingAgentPresetChatsKeepSeparateDrafts(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "cli")
	project := hs.a
	server := project.settings.AgentServers["backend"]
	server.Name = "Second"
	project.settings.AgentServers["second:cli"] = server
	name := locales.AgentRoleReviewer()
	project.settings.AgentRoles[name] = settings.AgentRole{Agent: "backend", Prompt: "Custom review"}
	before := maps.Clone(project.settings.AgentRoles)
	project.selectAgent("backend")
	project.openAgentPage()
	direct := project.agent
	direct.draft = "direct draft"
	for _, preset := range agentRolePresets {
		invitation := roundtableInvitation(t, project, "preset:"+preset.id)
		if invitation.Name != preset.name() || invitation.problem != "" || invitation.role.Prompt != preset.prompt() {
			t.Fatalf("invalid built-in role invitation: %+v", invitation)
		}
	}
	if invitation := roundtableInvitation(t, project, "role:"+name); invitation.role.Prompt != "Custom review" {
		t.Fatal("the named roundtable role collided with the built-in of the same name")
	}
	openPreset := func(provider string) {
		project.selectAgentTarget(project.agent, agentTarget{provider: provider, preset: &agentRolePresets[4]})
		hs.settle()
	}
	openPreset("backend")
	first := project.agent
	if first == direct || first.roleLabel() != name || first.provider != "backend" || first.systemPrompt != locales.AgentRoleReviewerPrompt() || !project.chatShown(first) {
		t.Fatal("the built-in chat lost its agent or prompt")
	}
	first.draft = "backend review draft"
	project.selectAgentRole(name)
	hs.settle()
	custom := project.agent
	if custom == first || custom.preset != nil || custom.systemPrompt != "Custom review" {
		t.Fatal("same-name custom role opened the built-in conversation")
	}
	chooseChatTarget(t, hs, "Second")
	openPreset("second:cli")
	second := project.agent
	if second == first || second.provider != "second:cli" || second.draft != "" {
		t.Fatal("agents shared a preset conversation")
	}
	second.draft = "second review draft"
	chooseChatTarget(t, hs, "Backend")
	if project.agent != direct || direct.draft != "direct draft" {
		t.Fatal("switching from a built-in lost the direct conversation")
	}
	openPreset("backend")
	if project.agent != first || first.draft != "backend review draft" || second.draft != "second review draft" {
		t.Fatal("returning to a built-in did not restore its own draft")
	}
	project.newChatPage()
	if fresh := project.agent; fresh.provider != first.provider || fresh.hasRole() || fresh.systemPrompt != "" || fresh.draft != "" || fresh.group == first.group {
		t.Fatal("New Chat should open an independent direct chat with the agent")
	}
	if !maps.Equal(before, project.settings.AgentRoles) || len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("using built-ins changed saved settings or ran a CLI command")
	}
}

func TestAgentPresetContractAcrossTurnsAndLocales(t *testing.T) {
	for _, protocol := range []string{"acp", "cli"} {
		t.Run(protocol, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, protocol)
			previous := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return "zh-Hans" })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
			project := hs.a
			project.selectAgentTarget(project.agent, agentTarget{provider: "backend", preset: &agentRolePresets[0]})
			chat := project.agent
			contract, key := locales.AgentRoleExplorerPrompt(), agentChatKey(chat)
			for _, question := range []string{"first question", "follow-up question"} {
				lifecycleSend(t, hs, question)
				if chat.err != "" || chat.draft != "" || chat.connectedRole == nil || chat.connectedRole.Model != "" {
					t.Fatalf("preset failed or pinned the adapter's model: err=%q, role=%+v", chat.err, chat.connectedRole)
				}
				i18n.SetLocaleFunc(func() string { return "ja" })
				project.checkAgentConfig()
				if chat.connected == nil || chat.systemPrompt != contract || chat.roleLabel() != locales.AgentRoleExplorer() || agentChatKey(chat) != key {
					t.Fatal("changing the interface language reset or retargeted the conversation")
				}
			}
			if contract == locales.AgentRoleExplorerPrompt() {
				t.Fatal("the test did not exercise different prompt translations")
			}
			turns := 0
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind == "prompt" || event.Kind == "cli" {
					turns++
					if !strings.Contains(event.Text, "[system]\n"+contract) || strings.Contains(event.Text, locales.AgentRoleExplorerPrompt()) {
						t.Fatalf("turn %d did not receive the selected contract: %q", turns, event.Text)
					}
				}
			}
			if turns != 2 {
				t.Fatalf("got %d requests, want two", turns)
			}
		})
	}
}

func TestAgentPresetDoesNotFallBackToAnotherProvider(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "cli")
	project := hs.a
	project.settings.AgentServers["other"] = settings.AgentServer{Name: "Other", Protocol: "cli", Command: "other"}
	project.settings.AgentRolePresets["plan"] = settings.AgentRoleOverride{Agent: "backend"}
	invitation := roundtableInvitation(t, project, "preset:plan")
	saved := project.agent
	server := project.settings.AgentServers["backend"]
	server.Disabled = true
	project.settings.AgentServers["backend"] = server
	if current := project.resolveSpeaker(invitation.Participant, project.agentProviders()); project.agent != saved || current.problem != locales.AgentDisabled() || current.Provider != "backend" {
		t.Fatal("disabled the selected provider but its preset was still selectable")
	}
	server.Disabled = false
	project.settings.AgentServers["backend"] = server
	project.selectAgentTarget(saved, agentTarget{provider: "backend", preset: &agentRolePresets[1]})
	hs.settle()
	delete(project.settings.AgentServers, "backend")
	chat := project.agent
	lifecycleSend(t, hs, "unsent plan")
	if chat.preset == nil || chat.provider != "backend" || chat.err == "" || chat.draft != "unsent plan" || len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("removing the provider lost the draft or attempted to send")
	}
}

func TestEmptyAgentListKeepsExistingRoleDraft(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "cli")
	project := hs.a
	project.settings.AgentServers["other"] = settings.AgentServer{Name: "Other", Protocol: "cli", Command: "other"}
	project.selectAgentTarget(project.agent, agentTarget{provider: "backend", preset: &agentRolePresets[0]})
	chat := project.agent
	project.openAgentPage()
	chat.draft = "keep my draft"
	project.selectAgent("other")
	project.settings.AgentServers = nil
	hs.settle()
	hs.openAgentMenu(locales.AgentRoster())
	if len(hs.openAgentMenuItems()) != 0 || !hs.tt.HasText(locales.PrefsAgentEmpty()) {
		t.Fatal("an empty agent list should show its empty state even with saved roles and drafts")
	}
	hs.key(0, ui.KeyEscape)
	if project.agentMenu.open || project.agentChats[agentChatKey(chat)] != chat || chat.draft != "keep my draft" || project.agentTargetAvailable(chat) || len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("closing an empty picker changed the saved chat or started an agent")
	}
}

func TestAgentPresetCLIRequiringModelIsNotOffered(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "cli")
	project := hs.a
	server := project.settings.AgentServers["backend"]
	server.Args = append(server.Args, "--engine={model}")
	project.settings.AgentServers["backend"] = server
	if speaker := roundtableInvitation(t, project, "preset:explore"); speaker.problem != locales.AgentRoleModelPlaceholder() {
		t.Fatalf("a CLI role requiring a model should be unavailable: %+v", speaker)
	}
	project.selectAgentTarget(project.agent, agentTarget{provider: "backend", preset: &agentRolePresets[0]})
	lifecycleSend(t, hs, "keep my question")
	if project.agentTargetAvailable(project.agent) || project.agent.err != locales.AgentPresetNeedsModel() || project.agent.draft != "keep my question" || len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("an existing preset accepted a CLI that now requires a model")
	}
}

func TestAgentPresetDetachedHistoryReturnsToSameChat(t *testing.T) {
	workspace := newHarness(t, false)
	workspace.a.settings.AgentServers = map[string]settings.AgentServer{"backend": {Name: "Backend", Protocol: "cli", Command: "unused"}}
	detached := tabWindowHarness(t, workspace)
	history := detached.a.blankChat()
	history.provider, history.continuation, history.draft = "backend", &sessionContinuation{loaded: true}, "detached"
	detached.a.openChatPage(history)
	target := agentTarget{provider: "backend", preset: &agentRolePresets[0]}
	detached.a.selectAgentTarget(history, target)
	first := workspace.a.agent
	first.draft = "workspace"
	detached.a.selectAgentTarget(history, agentTarget{provider: "backend", preset: &agentRolePresets[1]})
	detached.a.selectAgentTarget(history, target)
	if workspace.a.agent != first || first.draft != "workspace" || agentPageCount(workspace.a) != 2 || detached.a.agent != history || history.draft != "detached" {
		t.Fatal("reselecting a preset from a detached history opened another blank chat")
	}
}

func TestAgentPresetAssignedProviderModelAndPrompt(t *testing.T) {
	for _, protocol := range []string{"acp", "cli"} {
		t.Run(protocol, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, protocol)
			project := hs.a
			preset := &agentRolePresets[1]
			project.selectAgentTarget(project.agent, agentTarget{provider: "backend", preset: preset})
			old := project.agent
			old.draft = "private old draft"
			server := cloneAgentServer(project.settings.AgentServers["backend"])
			server.Name = "Assigned"
			if protocol == "cli" {
				server.Args = append(server.Args, "--", "--model={model}")
			}
			project.settings.AgentServers["assigned"] = server
			prompt := "Instructions for the assigned agent."
			project.settings.AgentRolePresets[preset.id] = settings.AgentRoleOverride{Agent: "assigned", Model: "model-b", Prompt: &prompt}
			project.checkAgentConfig()
			project.sendChat(old)
			if project.agentTargetAvailable(old) || old.draft != "private old draft" || old.err != locales.AgentPresetReassigned("assigned") {
				t.Fatal("reassignment did not protect the old draft")
			}
			invitation := roundtableInvitation(t, project, "preset:"+preset.id)
			if invitation.Provider != "assigned" || invitation.problem != "" || invitation.role.Prompt != prompt || invitation.role.Model != "model-b" {
				t.Fatalf("roundtable omitted the assigned provider, prompt or model: %+v", invitation)
			}
			project.selectAgentTarget(old, agentTarget{provider: invitation.Provider, preset: preset})
			project.openAgentPage()
			hs.settle()
			chat := project.agent
			if chat == old || chat.provider != "assigned" || chat.draft != "" {
				t.Fatalf("wrong preset conversation after reassignment: %q", chat.provider)
			}
			hs.untilLong("assigned role connection", func() bool { return !chat.connecting })
			for _, text := range []string{"first message", "next message"} {
				lifecycleSend(t, hs, text)
				if chat.err != "" {
					t.Fatal(chat.err)
				}
			}
			turns := 0
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind != "prompt" && event.Kind != "cli" {
					continue
				}
				turns++
				if !strings.Contains(event.Text, "[system]\n"+prompt) || strings.Contains(event.Text, old.draft) ||
					protocol == "acp" && event.Model != "model-b" || protocol == "cli" && !slices.Contains(event.Args, "--model=model-b") {
					t.Fatalf("wrong model or context on turn %d: %+v", turns, event)
				}
			}
			if turns != 2 {
				t.Fatalf("sent %d prompts, want 2", turns)
			}
			if project.agentChats[agentChatKey(old)] != old || old.draft != "private old draft" {
				t.Fatal("reassignment discarded the original draft")
			}
		})
	}
}

func TestPrefsBuiltinRoleListAndCancel(t *testing.T) {
	ph := prefsRoleHarness(t)
	for _, removed := range []string{"Create from Built-in Role…", "从内置角色创建…", "組み込みロールから作成…"} {
		if ph.pt.HasText(removed) {
			t.Fatal("obsolete preset creation menu is still present")
		}
	}
	for _, preset := range agentRolePresets {
		if !ph.pt.HasText(preset.name()) {
			t.Errorf("settings omitted %q", preset.name())
		}
		lead, _, _ := strings.Cut(preset.prompt(), "\n")
		if !ph.pt.HasText(lead) {
			t.Errorf("settings omitted the saved prompt preview for %q", preset.name())
		}
	}
	ph.shot("role-presets-settings")
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+locales.AgentRoleTester())
	draft := ph.w.roleDraft
	if draft == nil || draft.preset.id != "test" || draft.prompt != locales.AgentRoleTesterPrompt() || draft.agent != "" || draft.model != "" {
		t.Fatalf("settings did not open the unbound built-in: %+v", draft)
	}
	draft.prompt = "unsaved"
	ph.frames()
	prefsAgentClick(t, ph, locales.ButtonCancel())
	if ph.w.roleDraft != nil || len(ph.m.s.AgentRoles) != 0 || len(ph.m.s.AgentRolePresets) != 0 {
		t.Fatal("Cancel persisted a built-in role")
	}
}

func TestPrefsConversationEditsPresetWithoutOverwritingCustomRole(t *testing.T) {
	ph := newPrefsHarness(t)
	project := ph.a
	project.settings.AgentServers["local"] = settings.AgentServer{Name: "Local", Protocol: "cli", Command: "never-run"}
	project.selectAgentTarget(project.agent, agentTarget{provider: "local", preset: &agentRolePresets[0]})
	project.openAgentPage()
	chat := project.agent
	chat.modelOverride = "chosen-model"
	name, prompt := chat.roleLabel(), chat.systemPrompt
	existing := settings.AgentRole{Agent: "another", Prompt: "keep this definition"}
	ph.m.s.AgentRoles[name] = existing
	ph.w.navigate(settings.CatAgent, prefsConversationKey, ph.m.wins[0])
	ph.frames()
	if !ph.pt.HasText(prompt) {
		t.Fatal("conversation settings do not show the active preset prompt")
	}
	prefsAgentClick(t, ph, locales.PrefsRoleEdit())
	draft := ph.w.roleDraft
	if draft == nil || draft.preset != chat.preset || draft.agent != "" || draft.model != "" || draft.prompt != prompt {
		t.Fatalf("conversation overrides leaked into the built-in editor: %+v", draft)
	}
	draft.agent, draft.model, draft.prompt = "local", "saved-model", "My instructions"
	if !ph.w.saveAgentRole() {
		t.Fatal(draft.err)
	}
	want := settings.AgentRole{Agent: "local", Model: "saved-model", Prompt: "My instructions"}
	ph.m.s.Flush()
	loaded := settings.Load(ph.m.s.Path())
	defer loaded.Flush()
	if loaded.AgentRolePresets[chat.preset.id].Resolve("", "") != want || loaded.AgentRoles[name] != existing || len(loaded.AgentRoles) != 1 || chat.systemPrompt != want.Prompt || chat.preset.prompt() != prompt {
		t.Fatal("editing the built-in lost its binding or overwrote the same-name custom role")
	}
	ph.w.query = "agentRolePresets"
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsRoleRestore()+": "+name)
	ph.m.s.Flush()
	restored := settings.Load(ph.m.s.Path())
	defer restored.Flush()
	if _, overridden := restored.AgentRolePresets[chat.preset.id]; overridden || restored.AgentRoles[name] != existing || chat.systemPrompt != prompt {
		t.Fatal("one-click restore did not restore the chat prompt and keep the same-name saved role")
	}
}

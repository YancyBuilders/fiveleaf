package main

import (
	"bytes"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func prefsRoleHarness(t *testing.T) *prefsHarness {
	ph := newPrefsHarness(t)
	ph.w.section, ph.w.query = settings.CatAgent, "agentRolePresets"
	ph.pt.SetSize(940, 1000)
	ph.frames()
	return ph
}

func TestPrefsRoleAddCopyAndRoundtable(t *testing.T) {
	ph := prefsRoleHarness(t)
	preset := &agentRolePresets[4]
	copied := locales.PrefsRoleCopyName(preset.name())
	prefsAgentClick(t, ph, locales.PrefsRoleCopy()+": "+preset.name())
	draft := ph.w.roleDraft
	if draft.named.name != copied || draft.prompt != preset.prompt() || draft.agent != "" || draft.model != "" {
		t.Fatal("copy did not prefill the effective preset and its default agent binding")
	}
	prefsAgentClick(t, ph, locales.ButtonSave())
	want := settings.AgentRole{FollowCurrentAgent: true, Prompt: preset.prompt()}
	if ph.m.s.AgentRoles[copied] != want || len(ph.m.s.AgentRolePresets) != 0 || !ph.pt.HasText(copied) {
		t.Fatal("saving a copy without registrations did not create an independent listed role")
	}
	prefsAgentClick(t, ph, locales.PrefsRoleCopy()+": "+preset.name())
	if ph.w.roleDraft.named.name != copied+" 2" {
		t.Fatal("repeated copying did not offer a fresh name")
	}
	prefsAgentClick(t, ph, locales.ButtonCancel())
	prefsAgentClick(t, ph, locales.PrefsRoleAdd())
	ph.pt.Type("上线检查")
	if ph.w.roleDraft.named.name != "上线检查" {
		t.Fatal("Add Role did not focus the new role name")
	}
	ph.w.roleDraft.prompt = "核对发布说明。\n列出待确认的事项。"
	prefsAgentClick(t, ph, locales.ButtonSave())
	if !ph.pt.HasText("核对发布说明。") || !ph.pt.HasText("上线检查") {
		t.Fatal("new role and its effective prompt were missing from the list")
	}
	ph.m.s.AgentServers["local"] = settings.AgentServer{Command: "never-start", Protocol: "cli"}
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+copied)
	ph.w.roleDraft.prompt = "Review concurrency first."
	prefsAgentClick(t, ph, locales.ButtonSave())
	invitation := roundtableInvitation(t, ph.a, "role:"+copied)
	if invitation.problem != "" || invitation.Provider != "local" || invitation.role.Prompt != "Review concurrency first." || roundtableInvitation(t, ph.a, "preset:"+preset.id).role.Prompt != preset.prompt() {
		t.Fatal("the copy and its source did not resolve independent prompts on the available agent")
	}
	prefsAgentClick(t, ph, locales.PrefsRoleRemove()+": "+copied)
	prefsAgentClick(t, ph, locales.ButtonUndo())
	ph.m.s.Flush()
	reloaded := settings.Load(ph.m.s.Path())
	defer reloaded.Flush()
	if got := reloaded.AgentRoles[copied]; got.Prompt != invitation.role.Prompt || !got.FollowCurrentAgent || len(reloaded.AgentRoles) != 2 {
		t.Fatal("the restored copy did not persist")
	}
}

func TestPrefsRoleCopyKeepsOverridesAndRejectsConflicts(t *testing.T) {
	ph := prefsRoleHarness(t)
	preset := &agentRolePresets[0]
	empty := ""
	ph.m.s.AgentRolePresets[preset.id] = settings.AgentRoleOverride{Agent: "local", Model: "chosen", Prompt: &empty}
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsRoleCopy()+": "+preset.name())
	draft := ph.w.roleDraft
	if draft.agent != "local" || draft.model != "chosen" || draft.prompt != "" {
		t.Fatal("copy replaced an explicit binding or empty prompt with a default")
	}
	prefsAgentClick(t, ph, locales.ButtonSave())
	name := draft.named.name
	named := ph.m.s.AgentRoles[name]
	prefsAgentClick(t, ph, locales.PrefsRoleCopy()+": "+name)
	draft = ph.w.roleDraft
	if draft.agent != named.Agent || draft.model != named.Model || draft.prompt != named.Prompt {
		t.Fatal("copying a named role did not preserve its saved settings")
	}
	draft.named.name = name
	prefsAgentClick(t, ph, locales.ButtonSave())
	if draft.err != locales.PrefsRoleNameConflict() || len(ph.m.s.AgentRoles) != 1 {
		t.Fatal("a duplicate name replaced the existing role")
	}
	prefsAgentClick(t, ph, locales.ButtonCancel())
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+name)
	ph.w.roleDraft.named.name = "Renamed"
	newer := settings.AgentRole{Agent: "external"}
	ph.m.s.AgentRoles[name] = newer
	prefsAgentClick(t, ph, locales.ButtonSave())
	if ph.w.roleDraft.err != locales.PrefsRoleConflict() || ph.m.s.AgentRoles[name] != newer || len(ph.m.s.AgentRoles) != 1 {
		t.Fatal("a stale rename overwrote another edit")
	}
}

func TestPrefsRoleEditAndPersist(t *testing.T) {
	ph := prefsRoleHarness(t)
	preset := &agentRolePresets[4]
	ph.m.s.AgentServers = map[string]settings.AgentServer{"local": {Name: "Local Agent", Command: "never-start-this-agent"}}
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	if ph.w.roleDraft.prompt != preset.prompt() || ph.w.roleDraft.agent != "" {
		t.Fatal("the role did not start with its localized defaults")
	}
	prefsAgentClick(t, ph, locales.PrefsRoleCurrentAgent())
	prefsAgentClick(t, ph, "Local Agent · local")
	ph.pt.Key(0, ui.KeyTab)
	ph.pt.Type("model/reviewer")
	ph.pt.Key(0, ui.KeyTab)
	if !ph.pt.Focused(locales.PrefsRoleLoadModels()) {
		t.Fatal("the model lookup is not reachable from the model field by keyboard")
	}
	ph.pt.Key(0, ui.KeyTab)
	ph.pt.Key(ui.Cmd, ui.KeyA)
	ph.pt.Type("  Review the change.\n保留上下文。  ")
	ph.frames()
	want := settings.AgentRole{Agent: "local", Model: "model/reviewer", Prompt: "  Review the change.\n保留上下文。  "}
	if got := ph.w.roleDraft.override().Resolve("", preset.prompt()); got != want {
		t.Fatalf("role fields did not receive the typed values: %+v", got)
	}
	if len(ph.m.s.AgentRolePresets) != 0 {
		t.Fatal("typing the role changed settings before Save")
	}
	for _, label := range []string{locales.PrefsAgentCommand(), locales.PrefsAgentArguments(), locales.PrefsAgentCapabilities(), locales.PrefsAgentTimeout()} {
		if ph.pt.HasText(label) {
			t.Fatalf("role form exposed process option %q", label)
		}
	}
	prefsAgentClick(t, ph, locales.ButtonSave())
	if ph.w.roleDraft != nil || ph.m.s.AgentRolePresets[preset.id].Resolve("", preset.prompt()) != want {
		t.Fatalf("role was not saved: %+v", ph.m.s.AgentRolePresets)
	}
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	if got := ph.w.roleDraft.override().Resolve("", preset.prompt()); got != want {
		t.Fatalf("opening the role lost its configuration: %+v", got)
	}
	ph.w.roleDraft.model, ph.w.roleDraft.prompt = "", ""
	prefsAgentClick(t, ph, locales.ButtonSave())
	want.Model, want.Prompt = "", ""
	ph.m.s.Flush()
	reloaded := settings.Load(ph.m.s.Path())
	defer reloaded.Flush()
	for _, p := range []settings.Prefs{ph.m.s.Prefs, ph.a.settings.Prefs, reloaded.Prefs} {
		if p.AgentRolePresets[preset.id].Resolve("", preset.prompt()) != want {
			t.Fatalf("role did not propagate or persist: %+v", p.AgentRolePresets)
		}
	}
	if ph.a.agent.client != nil || ph.a.agent.connecting || len(ph.a.agent.messages) != 0 {
		t.Fatal("configuring a role started an agent or sent a message")
	}
}

func TestPrefsRoleMissingAgentStaysEditable(t *testing.T) {
	ph := prefsRoleHarness(t)
	preset := &agentRolePresets[0]
	role := settings.AgentRole{Agent: "removed-plugin.agent", Model: "chosen-model", Prompt: "saved prompt"}
	ph.m.s.AgentRolePresets[preset.id] = settings.AgentRoleOverride{Agent: role.Agent, Model: role.Model, Prompt: &role.Prompt}
	ph.m.s.AgentServers = map[string]settings.AgentServer{"other": {Name: "Available Agent", Command: "never-start-this-agent"}}
	ph.frames()
	if !ph.pt.HasText(locales.AgentRoleUnavailable()) {
		t.Fatal("a role with a missing agent has no visible unavailable state")
	}
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	if ph.w.roleDraft.agent != role.Agent || !ph.pt.HasText(role.Agent) {
		t.Fatal("opening a role replaced or concealed its missing agent")
	}
	prefsAgentClick(t, ph, role.Agent)
	if !ph.pt.HasText("Available Agent · other") || !ph.pt.HasText(role.Agent) {
		t.Fatal("the picker lost the unavailable saved agent or omitted another provider")
	}
	prefsAgentClick(t, ph, role.Agent)
	ph.w.roleDraft.prompt = "edited without replacing the agent"
	prefsAgentClick(t, ph, locales.ButtonSave())
	role.Prompt = "edited without replacing the agent"
	if got := ph.m.s.AgentRolePresets[preset.id].Resolve("", preset.prompt()); got != role {
		t.Fatalf("saving an unavailable role replaced its agent: %+v", got)
	}
}

func TestPrefsRoleStaleDraftCannotSaveOrRestore(t *testing.T) {
	ph := prefsRoleHarness(t)
	preset := &agentRolePresets[1]
	prompt := "saved plan"
	ph.m.s.AgentRolePresets[preset.id] = settings.AgentRoleOverride{Prompt: &prompt}
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	draft := ph.w.roleDraft
	draft.prompt = "unsaved changes"
	prompt = "a newer version"
	for _, action := range []string{locales.ButtonSave(), locales.PrefsRoleRestore()} {
		prefsAgentClick(t, ph, action)
		if ph.w.roleDraft != draft || draft.err != locales.PrefsRoleConflict() || ph.m.s.AgentRolePresets[preset.id].Resolve("", "").Prompt != prompt {
			t.Fatalf("%q overwrote a concurrent edit or discarded the draft", action)
		}
	}
}

func TestPrefsRoleValidationCancelAndNavigation(t *testing.T) {
	ph := prefsRoleHarness(t)
	preset := &agentRolePresets[0]
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	d := ph.w.roleDraft
	d.model = "unassigned-model"
	prefsAgentClick(t, ph, locales.ButtonSave())
	if ph.w.roleDraft != d || d.err == "" {
		t.Fatal("a model without an assigned agent did not show validation")
	}
	d.agent = "missing-agent"
	d.model = " model "
	if ph.w.saveAgentRole() || d.model != " model " {
		t.Fatal("invalid model was saved or silently rewritten")
	}
	d.model, d.prompt = "", strings.Repeat("x", settings.MaxAgentRolePromptBytes+1)
	if ph.w.saveAgentRole() {
		t.Fatal("an oversized prompt was saved")
	}
	d.prompt = "draft prompt"
	before := bytes.Clone(ph.m.s.Raw("agentRolePresets"))
	ph.w.section, ph.w.query = settings.CatEditor, ""
	ph.frames()
	ph.w.section, ph.w.query = settings.CatAgent, "agentRolePresets"
	ph.frames()
	if ph.w.roleDraft != d || !bytes.Equal(before, ph.m.s.Raw("agentRolePresets")) {
		t.Fatal("leaving and returning to the settings changed or discarded the role draft")
	}
	prefsAgentClick(t, ph, locales.ButtonCancel())
	if ph.w.roleDraft != nil || !bytes.Equal(before, ph.m.s.Raw("agentRolePresets")) {
		t.Fatal("Cancel retained the draft or changed settings")
	}
}

func TestPrefsRoleRestoreAndUndo(t *testing.T) {
	for _, action := range []string{"row", "editor", "all"} {
		for _, collision := range []bool{false, true} {
			t.Run(action+"/collision="+map[bool]string{true: "yes", false: "no"}[collision], func(t *testing.T) {
				ph := prefsRoleHarness(t)
				preset := &agentRolePresets[4]
				empty := ""
				original := settings.AgentRoleOverride{Agent: "agent", Model: "model", Prompt: &empty}
				other := settings.AgentRoleOverride{Agent: "other"}
				ph.m.s.AgentRolePresets = map[string]settings.AgentRoleOverride{preset.id: original, "plan": other}
				ph.m.s.AgentRoles["Saved role"] = settings.AgentRole{Agent: "legacy"}
				ph.frames()
				switch action {
				case "row":
					prefsAgentClick(t, ph, locales.PrefsRoleRestore()+": "+preset.name())
				case "editor":
					prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
					ph.w.roleDraft.prompt = "uncommitted"
					prefsAgentClick(t, ph, locales.PrefsRoleRestore())
				case "all":
					prefsAgentClick(t, ph, locales.PrefsRoleRestoreAll())
				}
				presets := ph.m.s.AgentRolePresets
				if _, exists := presets[preset.id]; exists || ph.w.roleDraft != nil || ph.m.s.AgentRoles["Saved role"].Agent != "legacy" {
					t.Fatal("restore did not immediately clear only preset settings")
				}
				if _, found := ph.pt.Find(locales.PrefsRoleRestore() + ": " + preset.name()); found {
					t.Fatal("a restored role still offers a row reset")
				}
				if action == "all" && len(presets) != 0 || action != "all" && presets["plan"] != other {
					t.Fatalf("%s restored the wrong roles: %+v", action, presets)
				}
				id := "diagnose"
				if collision {
					id = preset.id
				}
				newer := settings.AgentRoleOverride{Agent: "newer"}
				presets[id] = newer
				ph.frames()
				prefsAgentClick(t, ph, locales.ButtonUndo())
				presets = ph.m.s.AgentRolePresets
				if presets[id] != newer {
					t.Fatal("Undo replaced an externally changed role")
				}
				if collision {
					if !ph.pt.HasText(locales.PrefsRoleConflict()) || action == "all" && len(presets) != 1 {
						t.Fatal("Undo partially restored conflicting roles or concealed the conflict")
					}
				} else if !reflect.DeepEqual(presets[preset.id], original) || presets["plan"] != other {
					t.Fatal("Undo lost a saved binding or its explicitly empty prompt")
				}
			})
		}
	}
}

func TestPrefsRoleAgentChoicesAndSharedControls(t *testing.T) {
	ph := prefsAgentHarness(t)
	installTestAgent(t, "plugin-agent")
	withPlugins(ph.harness, map[string]map[string]string{
		"enabled-agent":  {"plugin.json": `{"name":"enabled-agent","version":"1.0.0","contributes":{"agentServers":[{"id":"enabled-agent.acp","name":"Plugin Agent","command":"plugin-agent"}]}}`},
		"disabled-agent": {"plugin.json": `{"name":"disabled-agent","version":"1.0.0","enabled":false,"contributes":{"agentServers":[{"id":"disabled-agent.acp","name":"Disabled Plugin","command":"disabled-agent"}]}}`},
	})
	ph.m.s.AgentServers = map[string]settings.AgentServer{"custom": {Name: "Custom", Command: "custom-agent"}}
	choices := ph.w.roleAgentChoices("disabled-agent.acp")
	for _, id := range []string{"custom", "enabled-agent.acp", "disabled-agent.acp"} {
		if !slices.ContainsFunc(choices, func(choice agentChoice) bool { return choice.id == id }) {
			t.Fatalf("role picker omitted provider %q: %+v", id, choices)
		}
	}
	if choices = ph.w.roleAgentChoices(""); slices.ContainsFunc(choices, func(choice agentChoice) bool { return choice.id == "disabled-agent.acp" }) {
		t.Fatal("a disabled plugin was offered for a new role")
	}
	ph.frames()
	preset := &agentRolePresets[0]
	if !ph.pt.HasText(preset.name()) || !ph.pt.HasText(locales.PrefsAgentAdd()) {
		t.Fatal("Configure Agents search does not show both native controls")
	}
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	ph.w.importAgentConfiguration(agentFile)
	ph.w.exportAgentConfiguration(agentFile)
	ph.w.importAgentConfiguration(roleFile)
	ph.w.exportAgentConfiguration(roleFile)
	ph.w.editAgentServer("")
	if ph.w.agentIOBusy || ph.w.agentDraft != nil || ph.w.roleDraft == nil {
		t.Fatal("an agent dialog or a second draft opened over the role draft")
	}
	prefsAgentClick(t, ph, locales.ButtonCancel())
	ph.w.editAgentServer("")
	ph.w.importAgentConfiguration(agentFile)
	ph.w.exportAgentConfiguration(agentFile)
	ph.w.editPresetRole(preset)
	if ph.w.agentIOBusy || ph.w.roleDraft != nil || ph.w.agentDraft == nil {
		t.Fatal("a configuration dialog or role draft opened over the agent draft")
	}
}

func TestPrefsRoleSelectRegisteredAgentAndModel(t *testing.T) {
	ph := prefsRoleHarness(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logPath := ph.path("model-requests.jsonl")
	ph.m.s.AgentServers["model-agent"] = settings.AgentServer{Name: "Model Agent", Command: executable,
		Args: []string{"-test.run=^TestAgentRoleRuntimeProcess$"}, Env: map[string]string{"FIVELEAF_ROLE_RUNTIME": "acp", "FIVELEAF_ROLE_LOG": logPath}}
	ph.m.s.AgentServers["other"] = settings.AgentServer{Name: "Other", Command: "unused", Protocol: "cli"}
	preset := &agentRolePresets[4]
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	prefsAgentClick(t, ph, locales.PrefsRoleCurrentAgent())
	prefsAgentClick(t, ph, "Model Agent · model-agent")
	if len(agentRoleEvents(t, logPath)) != 0 {
		t.Fatal("selecting the agent ran it without requesting models")
	}
	prefsAgentClick(t, ph, locales.PrefsRoleLoadModels())
	draft := ph.w.roleDraft
	deadline := time.Now().Add(5 * time.Second)
	for draft.models.cancel != nil && time.Now().Before(deadline) {
		step(ph.pt)
		time.Sleep(time.Millisecond)
	}
	if len(draft.models.choices) != 3 || draft.models.err != "" {
		t.Fatalf("model discovery: %+v, %q", draft.models.choices, draft.models.err)
	}
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsRoleModel())
	prefsAgentClick(t, ph, "B · model-b")
	prefsAgentClick(t, ph, locales.ButtonSave())
	got := ph.m.s.AgentRolePresets[preset.id]
	if got.Agent != "model-agent" || got.Model != "model-b" || got.Prompt != nil {
		t.Fatalf("saving only a binding did not preserve the localized default: %+v", got)
	}
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+preset.name())
	prefsAgentClick(t, ph, "Model Agent · model-agent")
	prefsAgentClick(t, ph, "Other · other")
	if d := ph.w.roleDraft; d.model != "" || len(d.models.choices) != 0 || ph.pt.HasText("B · model-b") {
		t.Fatal("switching agents retained the previous agent's model")
	}
	prefsAgentClick(t, ph, locales.ButtonCancel())
	if ph.m.s.AgentRolePresets[preset.id] != got || ph.a.agent.connected != nil || len(ph.a.agent.messages) != 0 {
		t.Fatal("discovery or cancellation changed the saved binding or the chat")
	}
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind != "start" && event.Kind != "new" {
			t.Fatalf("model discovery sent a conversation request: %+v", event)
		}
	}
}

func TestShotPrefsRoles(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	for _, state := range []struct {
		name, locale string
		dark         bool
		width        int
	}{
		{"prefs-role-dark", "en", true, 940},
		{"prefs-role-zh", "zh-Hans", false, 640},
		{"prefs-role-ja", "ja", true, 640},
	} {
		t.Run(state.name, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return state.locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			ph := prefsRoleHarness(t)
			ph.pt.SetSize(state.width, 940)
			ph.pt.SetLocale(state.locale)
			ph.pt.SetDark(state.dark)
			preset := &agentRolePresets[4]
			ph.m.s.AgentServers["pi"] = settings.AgentServer{Name: "Pi", Command: "pi-acp"}
			ph.m.s.AgentRolePresets[preset.id] = settings.AgentRoleOverride{Agent: "pi"}
			ph.w.query = prefTitle("agentRolePresets")
			ph.frames()
			ph.shot(state.name + "-list")
			ph.w.editPresetRole(preset)
			ph.frames()
			for _, label := range []string{preset.name(), locales.PrefsRoleAgent(), locales.PrefsRoleModel(), locales.PrefsRolePrompt(), locales.PrefsRoleRestore(), locales.ButtonSave()} {
				bounds, found := ph.pt.Find(label)
				if !found || bounds.X < 0 || bounds.X+bounds.W > float32(state.width)+1 {
					t.Errorf("control %q exceeds the settings window: %+v, found=%v", label, bounds, found)
				}
			}
			ph.shot(state.name)
		})
	}
}

func TestShotPrefsAgentRoster(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	ph := prefsAgentHarness(t)
	installTestAgent(t, "codex-acp")
	withPlugins(ph.harness, map[string]map[string]string{
		"agent-codex": {"plugin.json": `{"name":"agent-codex","version":"1.0.0","contributes":{"agentServers":[{"id":"agent-codex.acp","name":"Codex","command":"codex-acp"}]}}`},
	})
	ph.m.s.AgentServers["custom-pi"] = settings.AgentServer{Name: "Pi", Command: "npx"}
	ph.m.s.AgentRolePresets["review"] = settings.AgentRoleOverride{Agent: "agent-codex.acp", Model: "gpt-5.4"}
	ph.m.s.AgentRoles["Release notes"] = settings.AgentRole{Agent: "custom-pi"}
	ph.w.query = ""
	ph.pt.SetDark(false)
	ph.pt.SetSize(940, 880)
	ph.frames()
	ph.shot("agent-role-roster")
	ph.pt.SetSize(640, 820)
	ph.frames()
	ph.shot("agent-role-roster-narrow")
	prefsAgentClick(t, ph, locales.PrefsRoleEdit()+": "+locales.AgentRoleReviewer())
	ph.shot("agent-role-editor-narrow")
}

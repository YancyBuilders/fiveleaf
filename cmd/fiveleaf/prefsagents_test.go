package main

import (
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func prefsAgentHarness(t *testing.T) *prefsHarness {
	ph := newPrefsHarness(t)
	ph.w.section, ph.w.query = settings.CatAgent, "agentServers"
	ph.pt.SetSize(940, 1100)
	ph.frames()
	return ph
}

func prefsAgentClick(t *testing.T, ph *prefsHarness, label string) {
	t.Helper()
	if err := ph.pt.Click(label); err != nil {
		t.Fatal(err)
	}
	ph.frames()
}

func TestPrefsAgentSectionNavigation(t *testing.T) {
	ph := newPrefsHarness(t)
	old := plugins
	plugins = nil
	t.Cleanup(func() { plugins = old })
	prefsAgentClick(t, ph, prefSection(settings.CatAgent))
	if ph.w.section != settings.CatAgent || ph.w.query != "" {
		t.Fatalf("Agent navigation chose %q with query %q", ph.w.section, ph.w.query)
	}
	for _, label := range []string{prefTitle("agentServers"), prefTitle("agentRolePresets"), locales.PrefsAgentAdd(), locales.PrefsRoleAdd()} {
		if !ph.pt.HasText(label) {
			t.Errorf("Agent settings without plugins omitted %q", label)
		}
	}
	if ph.pt.HasText(prefTitle("agentRoles")) {
		t.Error("Agent settings still offers the separate custom role editor")
	}
	prefsAgentClick(t, ph, prefSection(settings.CatPlugins))
	if ph.w.section != settings.CatPlugins || !ph.pt.HasText(prefTitle("formatters")) {
		t.Fatal("Plugins navigation lost its own settings")
	}
	for _, label := range []string{prefTitle("agentServers"), prefTitle("agentRolePresets"), locales.PrefsAgentAdd()} {
		if ph.pt.HasText(label) {
			t.Errorf("Plugins still contains Agent control %q", label)
		}
	}
	prefsAgentClick(t, ph, prefSection(settings.CatAgent))
	prefsAgentClick(t, ph, locales.PrefsAgentAdd())
	if ph.w.agentDraft == nil {
		t.Fatal("Agent navigation did not open the native registration form")
	}
}

func TestPrefsAgentActionsFitWindow(t *testing.T) {
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
	ph := prefsAgentHarness(t)
	ph.pt.SetLocale("zh-Hans")
	ph.pt.SetDark(false)
	labels := []string{locales.PrefsAgentAdd(), locales.PrefsAgentImport(), locales.PrefsAgentExport(), locales.PrefsEditInJSON()}
	for _, width := range []int{940, 530} {
		ph.pt.SetSize(width, 600)
		ph.frames()
		var first, last ui.Rect
		for index, label := range labels {
			bounds, found := ph.pt.Find(label)
			if !found || bounds.H < 28 || bounds.W > 190 || bounds.X < prefsSidebarW || bounds.X+bounds.W > float32(width) {
				t.Fatalf("settings action %q in a %d-wide window: %v", label, width, bounds)
			}
			if index == 0 {
				first = bounds
			} else if bounds.H != first.H || (bounds.Y < last.Y+last.H && bounds.X < last.X+last.W) {
				t.Fatalf("settings actions have uneven height or overlap: %v and %v", last, bounds)
			}
			last = bounds
		}
		if width == 940 {
			if first.Y != last.Y {
				t.Fatalf("settings actions should fit one row: %v and %v", first, last)
			}
			ph.shot("settings-agent-actions-zh")
		} else if last.Y < first.Y+first.H {
			t.Fatalf("settings actions should wrap: %v and %v", first, last)
		}
	}
}

func TestPrefsAgentSearchAndChangedCounts(t *testing.T) {
	ph := newPrefsHarness(t)
	w := ph.w
	agentGroup, pluginGroup := prefGroupOf(settings.CatAgent), prefGroupOf(settings.CatPlugins)
	agentBefore, pluginBefore := w.changedInGroup(agentGroup, nil), w.changedInGroup(pluginGroup, nil)
	w.set("agentServers", json.RawMessage(`{"local":{"name":"Local Agent","command":"never-start-this-agent"}}`))
	w.set("agentRoles", json.RawMessage(`{"Reviewer":{"agent":"local","model":"review-model"}}`))
	w.set("agentRolePresets", json.RawMessage(`{"review":{"agent":"local"}}`))
	if got := w.changedInGroup(agentGroup, nil); got != agentBefore+2 {
		t.Errorf("Agent has %d changes, want %d", got, agentBefore+2)
	}
	if got := w.changedInGroup(pluginGroup, nil); got != pluginBefore {
		t.Errorf("Agent configuration changed the Plugins count from %d to %d", pluginBefore, got)
	}
	w.section, w.query = settings.CatPlugins, "agentServers"
	ph.frames()
	fields := w.shown(false)
	if slices.ContainsFunc(fields, func(f settings.Field) bool { return f.Key == "agentRoles" }) {
		t.Fatal("global search still exposes the separate custom role editor")
	}
	for _, key := range []string{"agentServers", "agentRolePresets"} {
		if !slices.ContainsFunc(fields, func(f settings.Field) bool { return f.Key == key && f.Category == settings.CatAgent }) {
			t.Errorf("global search omitted %s from Agent: %+v", key, fields)
		}
		if !ph.pt.HasText(prefTitle(key)) {
			t.Errorf("global search did not render %s", key)
		}
	}
}

func TestPrefsAgentCreateAndPersist(t *testing.T) {
	ph := prefsAgentHarness(t)
	prefsAgentClick(t, ph, locales.PrefsAgentAdd())
	// Add focuses Name; Tab advances to the executable. Static field labels
	// share their accessible names, so a text lookup would click the label.
	ph.pt.Type("Local ACP")
	ph.pt.Key(0, ui.KeyTab)
	ph.pt.Type("/opt/Agent Tools/adapter")
	ph.frames()
	if d := ph.w.agentDraft; d.name != "Local ACP" || d.command != "/opt/Agent Tools/adapter" {
		t.Fatalf("Name and Executable did not receive typed text: %+v", d)
	}
	prefsAgentClick(t, ph, locales.PrefsAgentAddArgument())
	prefsAgentClick(t, ph, locales.PrefsAgentArgument()+" 1")
	ph.pt.Type("  a spaced argument  ")
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsAgentAddArgument()) // A deliberate empty argument.
	prefsAgentClick(t, ph, locales.PrefsAgentAddEnv())
	prefsAgentClick(t, ph, locales.PrefsAgentEnvName()+" 1")
	ph.pt.Type("ACP_MODE")
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsAgentEnvValue()+" 1")
	ph.pt.Type("  first=one\nsecond=two  ")
	ph.frames()
	if len(ph.m.s.AgentServers) != 0 {
		t.Fatal("typing in the form wrote settings before Save")
	}
	prefsAgentClick(t, ph, locales.ButtonSave())
	want := settings.AgentServer{Name: "Local ACP", Command: "/opt/Agent Tools/adapter", Args: []string{"  a spaced argument  ", ""}, Env: map[string]string{"ACP_MODE": "  first=one\nsecond=two  "}}
	got, exists := ph.m.s.AgentServers["custom-local-acp"]
	if !exists || !reflect.DeepEqual(got, want) || ph.w.agentDraft != nil {
		t.Fatalf("saved definition %+v, want %+v; draft=%+v", got, want, ph.w.agentDraft)
	}
	if providers := ph.a.agentProviders(); len(providers) != 1 || providers[0].id != "custom-local-acp" {
		t.Fatalf("new agent missing from the chat choices: %+v", providers)
	}
	if ph.a.agent.client != nil || ph.a.agent.connecting || len(ph.a.agent.messages) != 0 {
		t.Fatal("registering an agent started a connection or a message")
	}
	ph.m.s.Flush()
	reloaded := settings.Load(ph.m.s.Path())
	defer reloaded.Flush()
	if !reflect.DeepEqual(reloaded.AgentServers["custom-local-acp"], want) {
		t.Fatalf("persisted definition %+v, want %+v", reloaded.AgentServers["custom-local-acp"], want)
	}
}

func TestPrefsAgentCancelAndValidation(t *testing.T) {
	ph := prefsAgentHarness(t)
	prefsAgentClick(t, ph, locales.PrefsAgentAdd())
	prefsAgentClick(t, ph, locales.ButtonSave())
	if ph.w.agentDraft == nil || !ph.pt.HasText(locales.PrefsAgentNameRequired()) {
		t.Fatal("a missing name did not preserve the form and explain the failure")
	}
	d := ph.w.agentDraft
	d.name = "Test Agent"
	prefsAgentClick(t, ph, locales.ButtonSave())
	if !ph.pt.HasText(locales.PrefsAgentCommandRequired()) {
		t.Fatal("a missing command has no validation message")
	}
	d.command = "agent --acp"
	if ph.w.saveAgentServer() || ph.w.agentDraft != d {
		t.Fatal("a command containing shell-style arguments was accepted")
	}
	d.command = "agent"
	d.env = []prefAgentVariable{{"BAD NAME", "value"}}
	if ph.w.saveAgentServer() || d.err != locales.PrefsAgentEnvInvalid() {
		t.Fatal("invalid environment name was accepted")
	}
	d.env = []prefAgentVariable{{"NAME", "one"}, {"NAME", "two"}}
	if ph.w.saveAgentServer() || d.err != locales.PrefsAgentDuplicateEnv() {
		t.Fatal("duplicate environment keys were collapsed")
	}
	d.env = nil
	d.args = []string{"nul\x00argument"}
	if ph.w.saveAgentServer() || len(ph.m.s.AgentServers) != 0 {
		t.Fatal("invalid argument was saved")
	}
	ph.frames()
	prefsAgentClick(t, ph, locales.ButtonCancel())
	if ph.w.agentDraft != nil || len(ph.m.s.AgentServers) != 0 {
		t.Fatal("Cancel retained the draft or changed settings")
	}
}

func TestPrefsAgentEditMergesAndDetectsConflict(t *testing.T) {
	ph := prefsAgentHarness(t)
	original := settings.AgentServer{Name: "Original", Command: "agent", Args: []string{"", "a b", "line\nbreak"}, Env: map[string]string{"EMPTY": "", "VALUE": " a=b\n "}}
	ph.m.s.AgentServers = map[string]settings.AgentServer{"stable-id": original}
	ph.w.editAgentServer("stable-id")
	d := ph.w.agentDraft
	if got, ok := d.definition(); !ok || !reflect.DeepEqual(got, original) {
		t.Fatalf("opening an existing definition was lossy: %+v", got)
	}
	d.name = "Renamed"
	ph.m.s.AgentServers["other"] = settings.AgentServer{Command: "other-agent"}
	if !ph.w.saveAgentServer() {
		t.Fatalf("save rejected an unrelated external addition: %q", d.err)
	}
	if ph.m.s.AgentServers["stable-id"].Name != "Renamed" || len(ph.m.s.AgentServers) != 2 {
		t.Fatal("rename lost its ID or another definition")
	}
	ph.w.editAgentServer("stable-id")
	d = ph.w.agentDraft
	d.command = "my-edit"
	external := ph.m.s.AgentServers["stable-id"]
	external.Command = "external-edit"
	ph.m.s.AgentServers["stable-id"] = external
	if ph.w.saveAgentServer() || d.err != locales.PrefsAgentConflict() || ph.m.s.AgentServers["stable-id"].Command != "external-edit" || ph.w.agentDraft != d {
		t.Fatal("an edit overwrote a newer definition or lost the user's draft")
	}
}

func TestPrefsAgentRemoveUndo(t *testing.T) {
	ph := prefsAgentHarness(t)
	ph.m.s.AgentServers = map[string]settings.AgentServer{"custom-test": {Name: "Test", Command: "agent"}}
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsAgentRemove()+": Test")
	if len(ph.m.s.AgentServers) != 0 {
		t.Fatal("Remove did not remove the custom agent")
	}
	ph.m.s.AgentServers["other"] = settings.AgentServer{Command: "other-agent"}
	prefsAgentClick(t, ph, locales.ButtonUndo())
	if len(ph.m.s.AgentServers) != 2 || ph.m.s.AgentServers["custom-test"].Command != "agent" {
		t.Fatal("Undo lost another agent or did not restore the removed definition")
	}
	ph.w.removeAgentServer("custom-test")
	ph.m.s.AgentServers["custom-test"] = settings.AgentServer{Command: "replacement"}
	ph.frames()
	prefsAgentClick(t, ph, locales.ButtonUndo())
	if ph.m.s.AgentServers["custom-test"].Command != "replacement" || !ph.pt.HasText(locales.PrefsAgentUndoConflict()) {
		t.Fatal("Undo overwrote a new definition using the same ID")
	}
}

func TestPrefsAgentResetUndoPreservesNewDefinitions(t *testing.T) {
	for _, conflicting := range []bool{false, true} {
		name := "new ID"
		if conflicting {
			name = "reused ID"
		}
		t.Run(name, func(t *testing.T) {
			ph := prefsAgentHarness(t)
			original := settings.AgentServer{Name: "Original", Command: "original-agent", Args: []string{"argument"}, Env: map[string]string{"MODE": "original"}}
			ph.m.s.AgentServers = map[string]settings.AgentServer{"custom-original": original}
			ph.w.reset("agentServers", nil)
			if len(ph.m.s.AgentServers) != 0 {
				t.Fatal("Reset did not clear custom agents")
			}
			id := "custom-new"
			if conflicting {
				id = "custom-original"
			}
			newer := settings.AgentServer{Name: "Newer", Command: "newer-agent"}
			ph.w.applyAgentServers(map[string]settings.AgentServer{id: newer})
			ph.frames()
			prefsAgentClick(t, ph, locales.ButtonUndo())
			if got := ph.m.s.AgentServers[id]; !reflect.DeepEqual(got, newer) {
				t.Fatalf("Undo overwrote a newer definition: %+v", got)
			}
			if conflicting {
				if len(ph.m.s.AgentServers) != 1 || !ph.pt.HasText(locales.PrefsAgentUndoConflict()) {
					t.Fatal("Undo did not explain the occupied ID or changed settings")
				}
			} else if got := ph.m.s.AgentServers["custom-original"]; !reflect.DeepEqual(got, original) || len(ph.m.s.AgentServers) != 2 {
				t.Fatalf("Undo did not merge the removed definition: %+v", ph.m.s.AgentServers)
			}
		})
	}
}

func TestPrefsAgentWithoutPlugins(t *testing.T) {
	ph := prefsAgentHarness(t)
	old := plugins
	plugins = nil
	t.Cleanup(func() { plugins = old })
	ph.w.query = ""
	ph.frames()
	if !ph.pt.HasText(locales.PrefsAgentAdd()) {
		t.Fatal("custom agent registration depends on the plugin subsystem")
	}
	// Search and the category both expose the same native control.
	ph.w.query = "agentServers"
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsAgentAdd())
	if ph.w.agentDraft == nil {
		t.Fatal("Add only opened JSON")
	}
}

func TestPrefsAgentPluginOverrides(t *testing.T) {
	ph := prefsAgentHarness(t)
	manifest := `{"name":"test-agent","version":"1.0.0","enabled":false,"contributes":{"agentServers":[{"id":"test-agent.acp","name":"Plugin Agent","command":"plugin-agent"}]}}`
	withPlugins(ph.harness, map[string]map[string]string{"test-agent": {"plugin.json": manifest}})
	if !pluginAgentIDs()["test-agent.acp"] {
		t.Fatal("a disabled plugin did not reserve its agent ID")
	}
	ph.w.editAgentServer("")
	ph.w.agentDraft.name, ph.w.agentDraft.command = "Demo", "custom-agent"
	if !ph.w.saveAgentServer() {
		t.Fatal("could not save a new custom agent")
	}
	if _, exists := ph.m.s.AgentServers["custom-demo"]; !exists {
		t.Fatal("new custom ID was not created alongside the plugin")
	}
	ph.m.s.AgentServers["test-agent.acp"] = settings.AgentServer{Name: "Intentional override", Command: "override-agent"}
	ph.frames()
	if !ph.pt.HasText(locales.PrefsAgentOverridesPlugin()) {
		t.Fatal("an existing JSON override does not explain its precedence")
	}
	ph.w.editAgentServer("test-agent.acp")
	ph.w.agentDraft.command = "edited-override"
	if !ph.w.saveAgentServer() || ph.m.s.AgentServers["test-agent.acp"].Command != "edited-override" {
		t.Fatal("editing an existing override changed its ID or lost its precedence")
	}
}

func TestPrefsAgentPluginListUsesEffectiveRegistrations(t *testing.T) {
	ph := prefsAgentHarness(t)
	installTestAgent(t, "plugin-agent")
	withPlugins(ph.harness, map[string]map[string]string{
		"visible": {"plugin.json": `{"name":"visible","version":"1.0.0","contributes":{"agentServers":[{"id":"visible.acp","name":"Plugin Agent","command":"plugin-agent"}]}}`},
		"paused":  {"plugin.json": `{"name":"paused","version":"1.0.0","enabled":false,"contributes":{"agentServers":[{"id":"paused.acp","name":"Paused Agent","command":"paused-agent"}]}}`},
	})
	ph.frames()
	if !ph.pt.HasText("Plugin Agent") || ph.pt.HasText("Paused Agent") || ph.pt.HasText(locales.PrefsAgentEmpty()) || len(ph.m.s.AgentServers) != 0 {
		t.Fatal("listing effective plugins wrote a local registration or showed the wrong agents")
	}
	if _, ok := ph.pt.Find(locales.PrefsAgentRemove() + ": Plugin Agent"); ok {
		t.Fatal("plugin registration offers local removal")
	}
	prefsAgentClick(t, ph, locales.PrefsAgentOpenPlugin("visible"))
	if ph.w.plugins.detail != "visible" {
		t.Fatal("plugin row did not open its owner")
	}
	ph.w.navigate(settings.CatAgent, "agentServers", nil)
	ph.m.s.AgentServers["visible.acp"] = settings.AgentServer{Name: "Local override", Command: "local"}
	ph.frames()
	if ph.pt.HasText("Plugin Agent") || !ph.pt.HasText("Local override") || !ph.pt.HasText(locales.PrefsAgentOverridesPlugin()) {
		t.Fatal("local override did not replace the plugin row")
	}
	prefsAgentClick(t, ph, locales.PrefsAgentRemove()+": Local override")
	if !ph.pt.HasText("Plugin Agent") || len(ph.m.s.AgentServers) != 0 {
		t.Fatal("removing an override did not reveal the underlying plugin registration")
	}
}

func TestPrefsAgentNewIDs(t *testing.T) {
	servers := map[string]settings.AgentServer{"custom-demo": {}, "custom-demo-2": {}}
	before := maps.Clone(servers)
	if got := customAgentID("  Demo! ", servers, map[string]bool{"custom-demo-3": true}); got != "custom-demo-4" {
		t.Errorf("generated ID %q", got)
	}
	if got := customAgentID("自定义助手", nil, nil); got != "custom-agent" {
		t.Errorf("non-ASCII name generated ID %q", got)
	}
	if !reflect.DeepEqual(servers, before) {
		t.Fatal("generating an ID changed settings")
	}
}

func TestShotPrefsAgent(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	for _, state := range []struct {
		name, locale string
		dark         bool
		width        int
	}{
		{"prefs-agent-dark", "en", true, 940},
		{"prefs-agent-zh", "zh-Hans", false, 640},
		{"prefs-agent-ja", "ja", true, 640},
	} {
		t.Run(state.name, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return state.locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			ph := prefsAgentHarness(t)
			ph.pt.SetSize(state.width, 860)
			ph.pt.SetDark(state.dark)
			ph.w.editAgentServer("")
			d := ph.w.agentDraft
			d.name, d.command, d.args = "OpenCode", "opencode", []string{"acp"}
			d.env = []prefAgentVariable{{"ACP_LOG_LEVEL", "info"}}
			if state.locale == "zh-Hans" {
				d.name, d.command, d.args = "Local CLI", "agent-cli", []string{"--print"}
				d.protocol, d.capabilities, d.timeout = "cli", "code, search", "180"
				d.promptArgument, d.disabled, d.env = true, true, nil
			}
			ph.frames()
			labels := []string{locales.PrefsAgentName(), locales.PrefsAgentCommand(), locales.PrefsAgentProtocol(), locales.PrefsAgentArgument() + " 1",
				locales.PrefsAgentWorkingDirectory(), locales.PrefsAgentCapabilities(), locales.PrefsAgentTimeout(), locales.ButtonSave()}
			if len(d.env) > 0 {
				labels = append(labels, locales.PrefsAgentEnvValue()+" 1")
			}
			if d.protocol == "cli" {
				labels = append(labels, locales.PrefsAgentPromptArgument())
			}
			for _, label := range labels {
				bounds, found := ph.pt.Find(label)
				if !found || bounds.X < 0 || bounds.X+bounds.W > float32(state.width)+1 {
					t.Errorf("control %q exceeds the settings window: %+v, found=%v", label, bounds, found)
				}
			}
			ph.shot(state.name)
			// Ensure the advanced representation uses separate argv/env and no shell string.
			server, valid := d.definition()
			raw, err := json.Marshal(server)
			args, _ := json.Marshal(d.args)
			if !valid || err != nil || !strings.Contains(string(raw), `"args":`+string(args)) {
				t.Fatalf("invalid fixture definition: %s %v", raw, err)
			}
		})
	}
}

// Confirm the multiline value widget retains empty entries when rows are removed.
func TestPrefsAgentArgumentRows(t *testing.T) {
	ph := prefsAgentHarness(t)
	ph.w.editAgentServer("")
	d := ph.w.agentDraft
	d.args = []string{"one", "", "third\nline"}
	ph.frames()
	prefsAgentClick(t, ph, locales.PrefsAgentRemoveArgument()+" 1")
	if !reflect.DeepEqual(d.args, []string{"", "third\nline"}) {
		t.Fatalf("removing one argument changed another: %#v", d.args)
	}
}

func TestPrefsAgentCLIOptionsAndLegacyDefaults(t *testing.T) {
	ph := prefsAgentHarness(t)
	ph.w.editAgentServer("")
	d := ph.w.agentDraft
	d.name, d.command = "Local CLI", "cli-agent"
	ph.frames()
	prefsAgentClick(t, ph, "ACP")
	prefsAgentClick(t, ph, "CLI")
	if d.protocol != "cli" || !ph.pt.HasText(locales.PrefsAgentPromptArgument()) {
		t.Fatal("the CLI choice did not expose its prompt argument option")
	}
	d.cwd, d.capabilities, d.timeout = "tools/agent project", "code, search, custom-tag", "45"
	prefsAgentClick(t, ph, locales.PrefsAgentDisabled())
	prefsAgentClick(t, ph, locales.PrefsAgentPromptArgument())
	if !d.disabled || !d.promptArgument {
		t.Fatal("the CLI switches did not change the draft")
	}
	if !ph.w.saveAgentServer() {
		t.Fatalf("could not save CLI options: %s", d.err)
	}
	want := settings.AgentServer{
		Name: "Local CLI", Command: "cli-agent", Protocol: "cli", WorkingDirectory: "tools/agent project",
		Capabilities: []string{"code", "search", "custom-tag"}, Disabled: true, TimeoutSeconds: 45, PromptArgument: true,
	}
	if got := ph.m.s.AgentServers["custom-local-cli"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("saved CLI options = %+v, want %+v", got, want)
	}
	ph.w.editAgentServer("custom-local-cli")
	d = ph.w.agentDraft
	if got, ok := d.definition(); !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("editing lost CLI fields: %+v", got)
	}
	d.original.Capabilities[0] = "draft-only"
	if ph.m.s.AgentServers["custom-local-cli"].Capabilities[0] != "code" {
		t.Fatal("draft capability snapshot aliases saved settings")
	}
	ph.frames()
	prefsAgentClick(t, ph, "CLI")
	prefsAgentClick(t, ph, "ACP")
	if d.promptArgument || ph.pt.HasText(locales.PrefsAgentPromptArgument()) {
		t.Fatal("changing to ACP retained the CLI prompt argument option")
	}
}

func TestPrefsAgentReplyTimeoutValidation(t *testing.T) {
	ph := prefsAgentHarness(t)
	ph.w.editAgentServer("")
	d := ph.w.agentDraft
	d.name, d.command = "Timeout Agent", "agent"
	if d.timeout != "120" {
		t.Fatalf("default reply timeout = %q", d.timeout)
	}
	for _, value := range []string{"", "0", "-1", "3601", "1.5", "not a number"} {
		d.timeout = value
		if ph.w.saveAgentServer() || d.field != "timeout" || d.err != locales.PrefsAgentTimeoutInvalid() || len(ph.m.s.AgentServers) != 0 {
			t.Fatalf("accepted timeout %q or lost its draft error", value)
		}
	}
	d.timeout = "120"
	definition, ok := d.definition()
	if !ok || definition.TimeoutSeconds != 0 || definition.Protocol != "" {
		t.Fatalf("default form rewrote omitted legacy defaults: %+v", definition)
	}
}

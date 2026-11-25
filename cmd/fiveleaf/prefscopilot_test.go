package main

import (
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestCopilotPrefsPersistAndClearModelOnAgentChange(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.w.section, ph.w.query = settings.CatAgent, "copilot"
	ph.m.s.AgentServers = map[string]settings.AgentServer{
		"first": {Command: "first"}, "second": {Command: "second"},
	}
	ph.frames()
	prefsAgentClick(t, ph, locales.CopilotConfigure())
	prefsAgentClick(t, ph, locales.PluginsOff())
	prefsAgentClick(t, ph, "first")
	prefsAgentClick(t, ph, locales.PrefsRoleModel())
	ph.pt.Type("provider/model")
	ph.frames()
	prefsAgentClick(t, ph, locales.ButtonSave())
	ph.m.s.Flush()
	want := settings.CopilotConfig{Agent: "first", Model: "provider/model"}
	if got := settings.Load(ph.m.s.Path()).Copilot; got != want || ph.a.settings.Copilot != want {
		t.Fatalf("saved %v, active %v, want %v", got, ph.a.settings.Copilot, want)
	}
	prefsAgentClick(t, ph, locales.CopilotConfigure())
	prefsAgentClick(t, ph, "first")
	prefsAgentClick(t, ph, "second")
	if ph.w.roleDraft.model != "" {
		t.Fatal("new agent inherited the old agent's model")
	}
	prefsAgentClick(t, ph, locales.ButtonCancel())
	if got := ph.m.s.Copilot; got != want {
		t.Fatalf("cancel changed the selected route to %v", got)
	}
}

func TestCopilotInlinePreferencePersists(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.w.section, ph.w.query = settings.CatAgent, "copilotInlineSuggestions"
	ph.frames()
	if ph.m.s.CopilotInlineSuggestions {
		t.Fatal("automatic suggestions started enabled")
	}
	if !ph.pt.HasText(locales.PrefTitleCopilotInlineSuggestions()) {
		t.Fatal("inline setting is missing from Agent settings")
	}
	toggle := ph.w.firstH.Bounds(ph.c)
	ph.pt.ClickAt(toggle.X+toggle.W/2, toggle.Y+toggle.H/2)
	ph.frames()
	ph.m.s.Flush()
	if !ph.a.settings.CopilotInlineSuggestions || !settings.Load(ph.m.s.Path()).CopilotInlineSuggestions {
		t.Fatal("inline setting did not reach the editor and the settings file")
	}
}

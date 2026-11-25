package settings

import (
	"bytes"
	"reflect"
	"testing"
)

func TestAgentRolePresetsRoundTrip(t *testing.T) {
	prefs := Default().Prefs
	prefs.AgentRoles["review"] = AgentRole{Agent: "other", Prompt: "custom"}
	if err := prefs.Set("agentRolePresets", []byte(`{
		"review":{"agent":"plugin.agent","model":"one"}, "plan":{"prompt":""},
		"requirements":{"prompt":"Agree on scope"}, "interaction":{"prompt":"Check keyboard focus"},
		"compatibility":{"prompt":"Open a saved profile"}, "moderate":{"prompt":"Summarize open questions"}
	}`)); err != nil {
		t.Fatal(err)
	}
	if got := prefs.AgentRolePresets["review"].Resolve("current", "localized"); got != (AgentRole{Agent: "plugin.agent", Model: "one", Prompt: "localized"}) {
		t.Fatalf("assigned role: %+v", got)
	}
	if got := prefs.AgentRolePresets["plan"].Resolve("current", "localized"); got != (AgentRole{Agent: "current"}) {
		t.Fatalf("empty prompt or current agent lost: %+v", got)
	}
	raw, err := prefs.ExportAgents()
	if err != nil {
		t.Fatal(err)
	}
	dest := Default().Prefs
	for range 2 {
		if err := dest.ImportAgents(raw); err != nil || !reflect.DeepEqual(dest.AgentRolePresets, prefs.AgentRolePresets) || dest.AgentRoles["review"] != prefs.AgentRoles["review"] {
			t.Fatalf("import: %+v, %v", dest.AgentRolePresets, err)
		}
	}
	before := dest.Raw("agentRolePresets")
	for _, bad := range []string{`{"unknown":{}}`, `{"review":{"model":"unbound"}}`, `{"plan":{"agent":"bad id"}}`, `{"test":{"prompt":"one","prompt":"two"}}`} {
		if dest.Set("agentRolePresets", []byte(bad)) == nil || !bytes.Equal(before, dest.Raw("agentRolePresets")) {
			t.Fatalf("invalid override not rejected atomically: %s", bad)
		}
	}
	if err := dest.ImportAgents([]byte(`{"version":3,"agentServers":{"new":{"command":"test"}},"agentRolePresets":{"review":{"prompt":"other"}}}`)); err == nil || len(dest.AgentServers) != 0 || !bytes.Equal(before, dest.Raw("agentRolePresets")) {
		t.Fatal("conflict partially imported")
	}
}

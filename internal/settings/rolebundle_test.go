package settings

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRoleBundleRoundTripAndLegacyImport(t *testing.T) {
	source := Default().Prefs
	source.AgentServers["private"] = AgentServer{Command: "private-tool", Env: map[string]string{"TOKEN": "private-token"}}
	source.AgentRoles["审稿"] = AgentRole{Agent: "private", Model: "chosen", Prompt: "保留\n换行"}
	source.AgentRoles["助手"] = AgentRole{FollowCurrentAgent: true}
	empty := ""
	source.AgentRolePresets["review"] = AgentRoleOverride{Prompt: &empty}
	templates := map[string]RolePresetTemplate{"review": {Name: "Reviewer", DefaultPrompt: "Read the changes"}, "plan": {Name: "Planner", DefaultPrompt: "Plan the change"}}
	raw, err := source.ExportRoles(templates)
	if err != nil || bytes.Contains(raw, []byte("private-tool")) || bytes.Contains(raw, []byte("private-token")) || bytes.Contains(raw, []byte("agentServers")) {
		t.Fatalf("role export included a server definition or failed: %v", err)
	}
	var bundle roleBundle
	if err := json.Unmarshal(raw, &bundle); err != nil || bundle.Presets["review"].DefaultPrompt != templates["review"].DefaultPrompt || bundle.Presets["plan"].Name != "Planner" {
		t.Fatalf("export omitted preset templates: %s", raw)
	}
	combined, err := source.ExportAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{raw, combined} {
		dest := Default().Prefs
		dest.AgentServers["private"] = AgentServer{Command: "local-tool"}
		dest.AgentRoles["existing"] = AgentRole{Agent: "local"}
		for range 2 {
			if err := dest.ImportRoles(input); err != nil {
				t.Fatal(err)
			}
		}
		if len(dest.AgentServers) != 1 || dest.AgentServers["private"].Command != "local-tool" || len(dest.AgentRoles) != 3 || dest.AgentRoles["审稿"] != source.AgentRoles["审稿"] || dest.AgentRoles["助手"] != source.AgentRoles["助手"] {
			t.Fatal("roles or existing agent registration changed on round trip")
		}
		if len(dest.AgentRolePresets) != 1 || dest.AgentRolePresets["review"].Resolve("current", "localized").Prompt != "" || dest.AgentRolePresets["plan"].Resolve("current", "別の言語").Prompt != "別の言語" {
			t.Fatal("the import confused a localized default with an explicit empty prompt")
		}
	}
}

func TestRoleBundleRejectsWholeBatch(t *testing.T) {
	prefs := Default().Prefs
	prefs.AgentRoles["keep"] = AgentRole{Agent: "current"}
	original := "original"
	prefs.AgentRolePresets["review"] = AgentRoleOverride{Prompt: &original}
	beforeRoles, beforePresets := prefs.Raw("agentRoles"), prefs.Raw("agentRolePresets")
	for _, raw := range []string{
		`{"version":1,"roles":{"fresh":{"agent":"local"},"keep":{"agent":"other"}}}`,
		`{"version":1,"roles":{"fresh":{"agent":"local"}},"presets":{"review":{"prompt":"different"}}}`,
		`{"version":1,"roles":{"fresh":{"agent":"local"}},"presets":{"typo":{}}}`,
		`{"version":1,"roles":{"fresh":{"agent":"local","command":"tool"}}}`,
		`{"version":1,"roles":{"fresh":{"agent":"local"},"fresh":{"agent":"other"}}}`,
		`{"version":1,"roles":{},"presets":null}`, `{"version":1,"presets":{"review":null}}`,
		`{"version":9,"roles":{}}`, `{"version":1,"roles":{}} {}`, `{"version":1}`,
		strings.Repeat(" ", MaxAgentConfigurationSize+1),
	} {
		if err := prefs.ImportRoles([]byte(raw)); err == nil || !bytes.Equal(beforeRoles, prefs.Raw("agentRoles")) || !bytes.Equal(beforePresets, prefs.Raw("agentRolePresets")) {
			t.Fatalf("invalid role batch was accepted or partially applied (length %d)", len(raw))
		}
	}
}

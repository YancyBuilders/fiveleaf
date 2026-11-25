package settings

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAgentConfigurationDefaultsAndRoundTrip(t *testing.T) {
	legacy := AgentServer{Command: "adapter"}
	if legacy.EffectiveProtocol() != "acp" || legacy.ReplyTimeoutSeconds() != 120 || legacy.Disabled || legacy.Validate() != nil {
		t.Fatalf("legacy defaults changed: %+v", legacy)
	}
	p := Default().Prefs
	raw := json.RawMessage(`{"cli":{"name":"CLI Agent","command":"tool","args":["","a b"],"env":{"PROFILE":" one=two\n "},"protocol":"cli","workingDirectory":"tools/agent project","capabilities":["code","reasoning"],"disabled":true,"timeoutSeconds":45,"promptArgument":true},"legacy":{"command":"adapter"}}`)
	if err := p.Set("agentServers", raw); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	data, _ := json.Marshal(p)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := Load(path)
	if !reflect.DeepEqual(loaded.AgentServers, p.AgentServers) {
		t.Fatalf("load lost agent fields: %+v", loaded.AgentServers)
	}
	exported, err := p.ExportAgents()
	if err != nil {
		t.Fatal(err)
	}
	var portable map[string]json.RawMessage
	if err := json.Unmarshal(exported, &portable); err != nil {
		t.Fatal(err)
	}
	if string(portable["version"]) != "1" || portable["agentRoles"] != nil {
		t.Fatalf("role-free export is no longer version 1: %s", exported)
	}
	other := Default().Prefs
	other.WordWrap = !p.WordWrap
	if err := other.ImportAgents(exported); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(other.AgentServers, p.AgentServers) || other.WordWrap == p.WordWrap {
		t.Fatal("agent import changed fields outside its registrations or lost a launch field")
	}
	if err := other.ImportAgents(exported); err != nil {
		t.Fatalf("reimport of matching definitions failed: %v", err)
	}
}

func TestAgentConfigurationValidationKeepsPreviousSettings(t *testing.T) {
	for _, fields := range []string{
		`"protocol":"http"`, `"protocol":"CLI"`, `"workingDirectory":"bad\u0000path"`,
		`"workingDirectory":"bad\npath"`, `"timeoutSeconds":-1`, `"timeoutSeconds":3601`,
		`"capabilities":[""]`, `"capabilities":["code","code"]`, `"capabilities":["code search"]`,
		`"capabilities":["code,search"]`, `"capabilities":["code\u0000"]`, `"promptArgument":true`,
	} {
		t.Run(fields, func(t *testing.T) {
			p := Default().Prefs
			p.AgentServers["keep"] = AgentServer{Command: "existing"}
			before := bytes.Clone(p.Raw("agentServers"))
			if err := p.Set("agentServers", json.RawMessage(`{"bad":{"command":"tool",`+fields+`}}`)); err == nil {
				t.Fatal("accepted invalid definition")
			}
			if !bytes.Equal(before, p.Raw("agentServers")) {
				t.Fatal("invalid fields changed the registered agents")
			}
		})
	}
	for _, dir := range []string{"", "tools/agent", "../shared agent", "/opt/Agent Tools"} {
		if err := (AgentServer{Command: "tool", WorkingDirectory: dir}).Validate(); err != nil {
			t.Fatalf("valid directory %q: %v", dir, err)
		}
	}
}

func TestAgentConfigurationImportMergesWithoutOverwriting(t *testing.T) {
	p := Default().Prefs
	p.AgentServers = map[string]AgentServer{
		"keep": {Command: "existing"},
		"same": {Command: "tool", Args: []string{}, Env: map[string]string{}, Capabilities: []string{"code", "reasoning"}},
	}
	matching := []byte(`{"version":1,"agentServers":{"new":{"command":"new-tool"},"same":{"command":"tool","protocol":"acp","timeoutSeconds":120,"capabilities":["reasoning","code"]}}}`)
	if err := p.ImportAgents(matching); err != nil {
		t.Fatal(err)
	}
	if len(p.AgentServers) != 3 || p.AgentServers["keep"].Command != "existing" || p.AgentServers["same"].Protocol != "" {
		t.Fatal("merge changed an existing matching definition")
	}
	before := maps.Clone(p.AgentServers)
	conflict := []byte(`{"version":1,"agentServers":{"a-new":{"command":"never-added"},"keep":{"command":"replacement"}}}`)
	if err := p.ImportAgents(conflict); err == nil || !strings.Contains(err.Error(), "keep") {
		t.Fatalf("missing conflicting ID: %v", err)
	}
	if !reflect.DeepEqual(before, p.AgentServers) {
		t.Fatal("failed import partially applied the file")
	}
}

func TestAgentConfigurationImportRejectsInvalidFilesAtomically(t *testing.T) {
	for _, raw := range []string{
		`{"version":2,"agentServers":{},"agentRoles":{"copy":{"followCurrentAgent":true}}}`,
		`{"version":5,"agentServers":{}}`,
		``, `null`, `[]`, `{`, `{"version":3,"agentServers":{}}`, `{"version":1}`, `{"version":1,"agentServers":null}`,
		`{"version":1,"agentServers":{}} {}`, `{"version":1,"agentServers":{},"wordWrap":true}`,
		`{"version":1,"agentServers":{"bad id":{"command":"tool"}}}`,
		`{"version":1,"agentServers":{"bad":{"command":"tool","unknown":true}}}`,
		`{"version":1,"agentServers":{"bad":{"command":"tool","timeoutSeconds":1.5}}}`,
		`{"version":1,"agentServers":{"bad":{"command":"tool","protocol":"http"}}}`,
		`{"version":1,"version":1,"agentServers":{}}`,
		`{"version":1,"agentServers":{"same":{"command":"first"},"same":{"command":"second"}}}`,
		`{"version":1,"agentServers":{"same":{"command":"tool","env":{"KEY":"first","KEY":"second"}}}}`,
		`{"version":1,"agentServers":{},"agentRoles":{}}`,
		`{"version":1,"agentServers":{},"agentRoles":null}`,
		`{"version":2,"agentServers":{},"agentRoles":null}`,
		`{"version":2,"agentServers":{},"agentRoles":[]}`,
		`{"version":2,"agentServers":{},"agentRoles":{"bad role":{"agent":"agent","unknown":true}}}`,
		`{"version":2,"agentServers":{},"agentRoles":{"bad role":{"agent":"agent","model":"bad\nmodel"}}}`,
		`{"version":2,"agentServers":{},"agentRoles":{"bad\nname":{"agent":"agent"}}}`,
		`{"version":2,"agentServers":{},"agentRoles":{"review":{"agent":"agent"},"review":{"agent":"other"}}}`,
		`{"version":2,"agentServers":{},"agentRoles":{"review":{"agent":"agent","prompt":"first","prompt":"second"}}}`,
		strings.Repeat(" ", MaxAgentConfigurationSize+1),
	} {
		p := Default().Prefs
		p.AgentServers["keep"] = AgentServer{Command: "existing"}
		p.AgentRoles["keep"] = AgentRole{Agent: "existing"}
		before := bytes.Clone(p.Raw("agentServers"))
		beforeRoles := bytes.Clone(p.Raw("agentRoles"))
		if err := p.ImportAgents([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid file: %.180s", raw)
		}
		if !bytes.Equal(before, p.Raw("agentServers")) || !bytes.Equal(beforeRoles, p.Raw("agentRoles")) {
			t.Fatal("invalid import changed existing agents or roles")
		}
	}
}

func TestAgentConfigurationFollowingRoleRoundTrip(t *testing.T) {
	source := Default().Prefs
	role := AgentRole{Prompt: "copied", FollowCurrentAgent: true}
	source.AgentRoles["Copy"] = role
	for _, withPreset := range []bool{false, true} {
		if withPreset {
			source.AgentRolePresets["test"] = AgentRoleOverride{Agent: "local"}
		}
		raw, err := source.ExportAgents()
		if err != nil || !bytes.Contains(raw, []byte(`"version": 4`)) {
			t.Fatalf("export following role: %s, %v", raw, err)
		}
		dest := Default().Prefs
		if err := dest.ImportAgents(raw); err != nil || dest.AgentRoles["Copy"] != role || !reflect.DeepEqual(dest.AgentRolePresets, source.AgentRolePresets) {
			t.Fatalf("import following role: %v", err)
		}
	}
}

func TestAgentConfigurationRolesRoundTrip(t *testing.T) {
	source := Default().Prefs
	source.AgentServers["cli"] = AgentServer{Command: "tool", Protocol: "cli"}
	source.AgentRoles["代码 审查"] = AgentRole{Agent: "cli", Model: "provider/model", Prompt: "检查错误\n保留证据"}
	source.AgentRoles["Plugin default"] = AgentRole{Agent: "plugin:not-installed"}
	raw, err := source.ExportAgents()
	if err != nil {
		t.Fatal(err)
	}
	var portable map[string]json.RawMessage
	if err := json.Unmarshal(raw, &portable); err != nil {
		t.Fatal(err)
	}
	if string(portable["version"]) != "2" || portable["agentRoles"] == nil {
		t.Fatalf("roles missing from portable configuration: %s", raw)
	}
	dest := Default().Prefs
	dest.WordWrap = true
	dest.AgentServers["existing"] = AgentServer{Command: "keep"}
	dest.AgentRoles["Existing role"] = AgentRole{Agent: "existing"}
	originalServers, originalRoles := dest.AgentServers, dest.AgentRoles
	if err := dest.ImportAgents(raw); err != nil {
		t.Fatal(err)
	}
	if len(originalServers) != 1 || len(originalRoles) != 1 {
		t.Fatal("import mutated the original maps")
	}
	if len(dest.AgentServers) != 2 || len(dest.AgentRoles) != 3 || !dest.WordWrap || dest.AgentRoles["代码 审查"] != source.AgentRoles["代码 审查"] || dest.AgentRoles["Plugin default"] != source.AgentRoles["Plugin default"] {
		t.Fatalf("import lost roles or changed other settings: %#v", dest.AgentRoles)
	}
	beforeServers, beforeRoles := dest.Raw("agentServers"), dest.Raw("agentRoles")
	if err := dest.ImportAgents(raw); err != nil {
		t.Fatalf("same agent and role definitions could not be reimported: %v", err)
	}
	if !bytes.Equal(beforeServers, dest.Raw("agentServers")) || !bytes.Equal(beforeRoles, dest.Raw("agentRoles")) {
		t.Fatal("reimport changed matching definitions")
	}
}

func TestAgentConfigurationRolesOnlyAndLegacyImports(t *testing.T) {
	source := Default().Prefs
	source.AgentRoles["Imported role"] = AgentRole{Agent: "missing-provider"}
	raw, err := source.ExportAgents()
	if err != nil {
		t.Fatal(err)
	}
	dest := Default().Prefs
	dest.AgentServers["keep"] = AgentServer{Command: "tool"}
	if err := dest.ImportAgents(raw); err != nil {
		t.Fatalf("roles-only version 2 import failed: %v", err)
	}
	if len(dest.AgentServers) != 1 || dest.AgentServers["keep"].Command != "tool" || !reflect.DeepEqual(dest.AgentRoles, source.AgentRoles) {
		t.Fatal("roles-only import changed existing registrations or lost unresolved roles")
	}
	for _, legacy := range []string{
		`{"version":1,"agentServers":{"new":{"command":"new-tool"}}}`,
		`{"version":2,"agentServers":{}}`,
	} {
		if err := dest.ImportAgents([]byte(legacy)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(dest.AgentRoles, source.AgentRoles) {
			t.Fatal("import without roles changed existing roles")
		}
	}
}

func TestAgentConfigurationRoleConflictsAreAtomic(t *testing.T) {
	for _, raw := range []string{
		`{"version":2,"agentServers":{"new":{"command":"new-tool"}},"agentRoles":{"a-new":{"agent":"new"},"keep":{"agent":"replacement"}}}`,
		`{"version":2,"agentServers":{"keep":{"command":"replacement"}},"agentRoles":{"a-new":{"agent":"new"}}}`,
		`{"version":2,"agentServers":{"new":{"command":"new-tool"}},"agentRoles":{"keep":{"agent":"existing","model":"different"}}}`,
		`{"version":2,"agentServers":{"new":{"command":"new-tool"}},"agentRoles":{"keep":{"agent":"existing","prompt":"different"}}}`,
	} {
		p := Default().Prefs
		p.AgentServers["keep"] = AgentServer{Command: "existing"}
		p.AgentRoles["keep"] = AgentRole{Agent: "existing"}
		beforeServers, beforeRoles := p.Raw("agentServers"), p.Raw("agentRoles")
		if err := p.ImportAgents([]byte(raw)); err == nil || !strings.Contains(err.Error(), "keep") {
			t.Fatalf("missing conflicting agent or role name: %v", err)
		}
		if !bytes.Equal(beforeServers, p.Raw("agentServers")) || !bytes.Equal(beforeRoles, p.Raw("agentRoles")) {
			t.Fatal("conflict partially applied agents or roles")
		}
	}
}

func TestAgentConfigurationRoleLimits(t *testing.T) {
	p := Default().Prefs
	p.AgentRoles["Too long"] = AgentRole{Agent: "agent", Prompt: strings.Repeat("p", MaxAgentRolePromptBytes+1)}
	if _, err := p.ExportAgents(); err == nil {
		t.Fatal("export accepted an oversized role prompt")
	}
	raw, err := json.Marshal(agentConfiguration{Version: 2, AgentServers: map[string]AgentServer{"new": {Command: "tool"}}, AgentRoles: p.AgentRoles})
	if err != nil {
		t.Fatal(err)
	}
	dest := Default().Prefs
	if err := dest.ImportAgents(raw); err == nil || len(dest.AgentServers) != 0 || len(dest.AgentRoles) != 0 {
		t.Fatalf("oversized prompt import was not atomic: %v", err)
	}
	p.AgentRoles = map[string]AgentRole{}
	for i := range MaxAgentRoles {
		p.AgentRoles[strings.Repeat("r", i+1)] = AgentRole{Agent: "agent"}
	}
	before := p.Raw("agentRoles")
	raw = []byte(`{"version":2,"agentServers":{"new":{"command":"tool"}},"agentRoles":{"extra":{"agent":"agent"}}}`)
	if err := p.ImportAgents(raw); err == nil || len(p.AgentServers) != 0 || !bytes.Equal(before, p.Raw("agentRoles")) {
		t.Fatalf("merged role count limit was not atomic: %v", err)
	}
	p.AgentRoles = map[string]AgentRole{}
	for i := range MaxAgentConfigurationSize / MaxAgentRolePromptBytes {
		p.AgentRoles[strings.Repeat("r", i+1)] = AgentRole{Agent: "agent", Prompt: strings.Repeat("p", MaxAgentRolePromptBytes)}
	}
	if _, err := p.ExportAgents(); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("export did not bound the entire file: %v", err)
	}
}

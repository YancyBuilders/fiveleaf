package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestAgentRolesRoundTripAndReset(t *testing.T) {
	p := Default().Prefs
	if got := string(p.Raw("agentRoles")); got != "{}" {
		t.Fatalf("default agent roles = %s", got)
	}
	raw := json.RawMessage(`{"代码 审查":{"agent":"plugin:reviewer","model":"provider/model","prompt":"检查错误\n保留证据"},"Default model":{"agent":"not-installed"}}`)
	if err := p.Set("agentRoles", raw); err != nil {
		t.Fatal(err)
	}
	if err := p.Set("wordWrap", json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := Load(path)
	if !reflect.DeepEqual(loaded.AgentRoles, p.AgentRoles) || !loaded.WordWrap {
		t.Fatalf("role settings did not survive normalization and loading: %#v", loaded.AgentRoles)
	}
	p.Reset("agentRoles")
	if len(p.AgentRoles) != 0 || !p.IsDefaultKey("agentRoles") || !p.WordWrap {
		t.Fatalf("reset changed other settings or kept roles: %#v", p.AgentRoles)
	}
}

func TestAgentRoleValidation(t *testing.T) {
	valid := []AgentRole{
		{FollowCurrentAgent: true, Prompt: "Role instructions"},
		{Agent: "plugin:agent"},
		{Agent: "missing", Model: "模型 1", Prompt: "multiple\nlines\t你好"},
		{Agent: strings.Repeat("a", MaxAgentRoleAgentBytes), Model: strings.Repeat("m", MaxAgentRoleModelBytes), Prompt: strings.Repeat("p", MaxAgentRolePromptBytes)},
	}
	for _, role := range valid {
		if err := role.Validate(); err != nil {
			t.Errorf("valid role rejected: %v", err)
		}
	}
	invalid := []AgentRole{
		{Agent: "fixed", FollowCurrentAgent: true},
		{Model: "fixed", FollowCurrentAgent: true},
		{},
		{Agent: "bad id"},
		{Agent: "bad\u2003id"},
		{Agent: "bad\x7fid"},
		{Agent: "bad\x00id"},
		{Agent: string([]byte{0xff})},
		{Agent: strings.Repeat("a", MaxAgentRoleAgentBytes+1)},
		{Agent: "agent", Model: " model"},
		{Agent: "agent", Model: "bad\nmodel"},
		{Agent: "agent", Model: "bad\u0085model"},
		{Agent: "agent", Model: string([]byte{0xff})},
		{Agent: "agent", Model: strings.Repeat("m", MaxAgentRoleModelBytes+1)},
		{Agent: "agent", Prompt: strings.Repeat("p", MaxAgentRolePromptBytes+1)},
		{Agent: "agent", Prompt: string([]byte{0xff})},
	}
	for i, role := range invalid {
		if err := role.Validate(); err == nil {
			t.Errorf("accepted invalid role %d", i)
		}
	}
	for _, name := range []string{"审查", "Code reviewer", strings.Repeat("r", MaxAgentRoleNameBytes)} {
		if err := ValidateAgentRoleName(name); err != nil {
			t.Errorf("valid role name rejected: %v", err)
		}
	}
	for _, name := range []string{"", " ", " name", "name ", "bad\tname", "bad\u0085name", string([]byte{0xff}), strings.Repeat("r", MaxAgentRoleNameBytes+1)} {
		if err := ValidateAgentRoleName(name); err == nil {
			t.Errorf("accepted invalid role name %q", name)
		}
	}
}

func TestAgentRolesValidationKeepsPreviousSettings(t *testing.T) {
	for _, raw := range []string{
		`[]`, `{"":{"agent":"agent"}}`, `{"bad\nname":{"agent":"agent"}}`,
		`{"review":{}}`, `{"review":{"agent":"bad id"}}`, `{"review":{"agent":"a","model":"bad\tmodel"}}`,
		`{"review":{"agent":"a","prompt":true}}`, `{"review":{"agent":"a","unknown":true}}`,
		`{"review":{"agent":"a"},"review":{"agent":"b"}}`, `{"review":{"agent":"a","agent":"b"}}`,
		`{"review":{"agent":"a"}} {}`,
		`{"review":{"agent":"a","prompt":"` + string([]byte{0xff}) + `"}}`,
	} {
		p := Default().Prefs
		p.AgentRoles["keep"] = AgentRole{Agent: "existing"}
		before := bytes.Clone(p.Raw("agentRoles"))
		if err := p.Set("agentRoles", json.RawMessage(raw)); err == nil {
			t.Errorf("accepted invalid roles: %s", raw)
		}
		if !bytes.Equal(before, p.Raw("agentRoles")) {
			t.Fatal("invalid role edit changed previous settings")
		}
	}
	roles := make(map[string]AgentRole, MaxAgentRoles+1)
	for i := range MaxAgentRoles {
		roles[fmt.Sprintf("role %d", i)] = AgentRole{Agent: "agent"}
	}
	if err := ValidateAgentRoles(roles); err != nil {
		t.Fatalf("role count limit rejected: %v", err)
	}
	roles["one too many"] = AgentRole{Agent: "agent"}
	if err := ValidateAgentRoles(roles); err == nil {
		t.Fatal("accepted too many roles")
	}
}

func TestAgentRolesAreUserOnly(t *testing.T) {
	field, ok := FieldOf("agentRoles")
	if !ok || field.Kind != KindJSON || field.Category != CatAgent || field.Folder || slices.Contains(FolderKeys(), "agentRoles") {
		t.Fatalf("agent roles schema = %+v, %v", field, ok)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, FolderDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(FolderPath(dir), []byte(`{"agentRoles":{"user":{"agent":"other","prompt":"injected"}},"wordWrap":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Default().Prefs
	p.AgentRoles["user"] = AgentRole{Agent: "chosen"}
	got := LoadFolder(dir).Apply(p)
	if !got.WordWrap || !reflect.DeepEqual(got.AgentRoles, p.AgentRoles) {
		t.Fatalf("folder changed user roles: %#v", got.AgentRoles)
	}
}

func TestAgentRolesLoadedFromDiskRemainAvailableForValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := []byte(`{"agentRoles":{"Missing provider":{"agent":"not-installed"},"Invalid model":{"agent":"agent","model":"bad\nmodel"}}}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := Load(path)
	if len(loaded.AgentRoles) != 2 || loaded.AgentRoles["Missing provider"].Agent != "not-installed" {
		t.Fatalf("loading silently discarded roles: %#v", loaded.AgentRoles)
	}
	if err := loaded.AgentRoles["Invalid model"].Validate(); err == nil {
		t.Fatal("invalid disk model passed runtime validation")
	}
}

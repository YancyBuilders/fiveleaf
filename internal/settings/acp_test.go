package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestACPAgentServersRoundTrip(t *testing.T) {
	p := Default().Prefs
	if got := string(p.Raw("agentServers")); got != "{}" {
		t.Fatalf("default agent servers = %s", got)
	}
	input := json.RawMessage(`{"local":{"name":"Local Agent","command":"/Applications/Agent Tools/agent","args":["--acp","a b"],"env":{"AGENT_PROFILE":"work"}}}`)
	if err := p.Set("agentServers", input); err != nil {
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
	if !reflect.DeepEqual(loaded.AgentServers, p.AgentServers) {
		t.Fatalf("loaded servers = %#v, want %#v", loaded.AgentServers, p.AgentServers)
	}
	p.Reset("agentServers")
	if len(p.AgentServers) != 0 || !p.IsDefaultKey("agentServers") {
		t.Fatalf("reset servers = %#v", p.AgentServers)
	}
}

func TestACPInvalidAgentServerKeepsPreviousSettings(t *testing.T) {
	p := Default().Prefs
	p.AgentServers["valid"] = AgentServer{Command: "agent"}
	before := string(p.Raw("agentServers"))
	for _, input := range []string{
		`{"":{"command":"agent"}}`,
		`{"bad id":{"command":"agent"}}`,
		`{"test":{"command":" "}}`,
		`{"test":{"command":" agent"}}`,
		`{"test":{"command":"agent --acp"}}`,
		`{"test":{"command":"agent\n"}}`,
		`{"test":{"command":"agent\u0000"}}`,
		`{"test":{"command":"agent","args":["bad\u0000arg"]}}`,
		`{"test":{"command":"agent","env":{"":"x"}}}`,
		`{"test":{"command":"agent","env":{"BAD=KEY":"x"}}}`,
		`{"test":{"command":"agent","env":{"KEY":"bad\u0000value"}}}`,
	} {
		if err := p.Set("agentServers", json.RawMessage(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
		if got := string(p.Raw("agentServers")); got != before {
			t.Fatalf("invalid configuration replaced previous servers: %s", got)
		}
	}
}

func TestACPAgentServersAreUserOnly(t *testing.T) {
	field, ok := FieldOf("agentServers")
	if !ok || field.Kind != KindJSON || field.Category != CatAgent || field.Folder || slices.Contains(FolderKeys(), "agentServers") {
		t.Fatalf("agent server schema = %+v, %v", field, ok)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, FolderDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(FolderPath(dir), []byte(`{"agentServers":{"repo":{"command":"untrusted-agent"}},"wordWrap":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Default().Prefs
	p.AgentServers["user"] = AgentServer{Command: "trusted-agent"}
	got := LoadFolder(dir).Apply(p)
	if !got.WordWrap || !reflect.DeepEqual(got.AgentServers, p.AgentServers) {
		t.Fatalf("folder changed user agent servers: %#v", got.AgentServers)
	}
}

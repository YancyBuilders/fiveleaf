package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
)

func TestAgentPluginRequiresExecutable(t *testing.T) {
	for _, source := range []string{"PATH", "absolute", "plugin-relative"} {
		t.Run(source, func(t *testing.T) {
			reg, host, dirs := newRegistry(t)
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			command := "test-agent.exe"
			executable := filepath.Join(bin, command)
			switch source {
			case "absolute":
				command = executable
			case "plugin-relative":
				command = "./bin/test-agent.exe"
				executable = filepath.Join(dirs.Plugins, "local-agent", command)
			}
			servers := []AgentServerContrib{
				{ID: "local-agent.optional", Name: "Optional", Command: "unused", Disabled: true},
				{ID: "local-agent.chat", Name: "Chat", Command: command},
			}
			manifest, err := json.Marshal(Manifest{Name: "local-agent", Contributes: Contributes{AgentServers: servers}})
			if err != nil {
				t.Fatal(err)
			}
			writePlugin(t, dirs.Plugins, "local-agent", map[string]string{ManifestFile: string(manifest)})
			host.settings["local-agent"] = json.RawMessage(`{"enabled":true}`)
			check := func(wantEnabled bool) {
				t.Helper()
				reg.Load()
				pl := reg.Plugin("local-agent")
				if pl == nil || pl.Stats().Err != "" {
					t.Fatal("expected a valid installed plugin")
				}
				if slices.Contains(reg.Enabled(), pl) != wantEnabled || pl.Stats().Disabled == wantEnabled {
					t.Fatalf("command %q: want enabled=%t, got %v", command, wantEnabled, pl.Stats().State)
				}
			}
			check(false)
			if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(executable, nil, 0o755); err != nil {
				t.Fatal(err)
			}
			check(true)
			if runtime.GOOS != "windows" {
				if err := os.Chmod(executable, 0o644); err != nil {
					t.Fatal(err)
				}
				check(false)
			}
			if err := os.Remove(executable); err != nil {
				t.Fatal(err)
			}
			check(false)
		})
	}
}

func TestACPAgentServerManifestWithoutCode(t *testing.T) {
	m, err := ParseManifest([]byte(`{
		"name":"local-agent","enabled":false,
		"contributes":{
			"agentServers":[{"id":"local-agent.acp","name":"Local Agent","command":"./bin/agent","args":["--acp","a b"],"env":{"AGENT_PROFILE":"work"}}],
			"agents":[{"id":"local-agent.sessions","name":"Terminal Sessions","resume":"agent resume {id}","sessions":{"glob":"~/.local-agent/*.json"}}]
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.HasCode() || m.DefaultOn() {
		t.Fatal("a declarative opt-in agent started a JavaScript runtime or was enabled by default")
	}
	want := AgentServerContrib{ID: "local-agent.acp", Name: "Local Agent", Command: "./bin/agent", Args: []string{"--acp", "a b"}, Env: map[string]string{"AGENT_PROFILE": "work"}}
	if !reflect.DeepEqual(m.Contributes.AgentServers, []AgentServerContrib{want}) {
		t.Fatalf("agent servers = %#v", m.Contributes.AgentServers)
	}
	if len(m.Contributes.Agents) != 1 || m.Contributes.Agents[0].Resume != "agent resume {id}" {
		t.Fatal("ACP changed the terminal session contribution")
	}
}

func TestACPAgentServerManifestValidation(t *testing.T) {
	valid := AgentServerContrib{ID: "local-agent.acp", Name: "Local Agent", Command: "agent"}
	cases := []struct {
		name   string
		change func(*AgentServerContrib)
	}{
		{"foreign id", func(s *AgentServerContrib) { s.ID = "other.acp" }},
		{"empty id suffix", func(s *AgentServerContrib) { s.ID = "local-agent." }},
		{"id whitespace", func(s *AgentServerContrib) { s.ID = "local-agent.bad id" }},
		{"missing name", func(s *AgentServerContrib) { s.Name = " " }},
		{"missing command", func(s *AgentServerContrib) { s.Command = "" }},
		{"command arguments", func(s *AgentServerContrib) { s.Command = "agent --acp" }},
		{"command NUL", func(s *AgentServerContrib) { s.Command = "agent\x00" }},
		{"argument NUL", func(s *AgentServerContrib) { s.Args = []string{"\x00"} }},
		{"environment key", func(s *AgentServerContrib) { s.Env = map[string]string{"BAD=KEY": "value"} }},
		{"environment value", func(s *AgentServerContrib) { s.Env = map[string]string{"KEY": "\x00"} }},
		{"invalid protocol", func(s *AgentServerContrib) { s.Protocol = "http" }},
		{"working directory", func(s *AgentServerContrib) { s.WorkingDirectory = "bad\x00path" }},
		{"timeout", func(s *AgentServerContrib) { s.TimeoutSeconds = 3601 }},
		{"capability", func(s *AgentServerContrib) { s.Capabilities = []string{"code search"} }},
		{"ACP prompt argument", func(s *AgentServerContrib) { s.PromptArgument = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			tc.change(&s)
			data, err := json.Marshal(Manifest{Name: "local-agent", Contributes: Contributes{AgentServers: []AgentServerContrib{s}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseManifest(data); err == nil {
				t.Fatalf("accepted %s", data)
			}
		})
	}
	data, err := json.Marshal(Manifest{Name: "local-agent", Contributes: Contributes{AgentServers: []AgentServerContrib{valid, valid}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseManifest(data); err == nil {
		t.Fatal("accepted duplicate ACP agent ids")
	}
}

func TestAgentServerManifestCLIDefinition(t *testing.T) {
	want := AgentServerContrib{
		ID: "local-agent.cli", Name: "Local CLI", Command: "agent", Args: []string{"--print"},
		Env: map[string]string{"PROFILE": "work"}, Protocol: "cli", WorkingDirectory: "tools",
		Capabilities: []string{"code", "search"}, Disabled: true, TimeoutSeconds: 50, PromptArgument: true,
	}
	raw, err := json.Marshal(Manifest{Name: "local-agent", Contributes: Contributes{AgentServers: []AgentServerContrib{want}}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Contributes.AgentServers, []AgentServerContrib{want}) {
		t.Fatalf("plugin lost CLI fields: %+v", m.Contributes.AgentServers)
	}
}

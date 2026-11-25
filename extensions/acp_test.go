package extensions_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"fiveleaf/internal/plugin"
)

func TestMarketplaceAgentsRequireLocalCLIs(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv("CODEX_PATH", "")
	t.Setenv("PI_ACP_PI_COMMAND", "")
	install := func(name string) string {
		t.Helper()
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	launcher := install("npx")
	reg := plugin.New(plugin.Dirs{Plugins: "."}, nil)
	reg.Load()
	for _, name := range []string{"claude", "codex", "gemini", "kimi", "opencode", "qwen", "pi", "omp", "dsh"} {
		if missing := reg.Plugin("agent-" + name).MissingAgentCommand; missing != name {
			t.Errorf("%s with only npx installed: missing %q, want %q", name, missing, name)
		}
		install(name)
	}
	reg.Load()
	for _, pl := range reg.Plugins() {
		if missing := pl.MissingAgentCommand; missing != "" {
			t.Errorf("%s still missing %q after installation", pl.Name, missing)
		}
	}
	t.Setenv("CODEX_PATH", filepath.Join(bin, "missing-override"))
	reg.Load()
	if missing := reg.Plugin("agent-codex").MissingAgentCommand; missing != "$CODEX_PATH" {
		t.Fatalf("Codex ignored its executable override: %q", missing)
	}
	t.Setenv("CODEX_PATH", install("custom-codex"))
	if !reg.CheckAgentCommands(reg.Plugin("agent-codex")) {
		t.Fatal("Codex rejected its installed executable override")
	}
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	reg.Load()
	if missing := reg.Plugin("agent-codex").MissingAgentCommand; missing != "npx" {
		t.Fatalf("installed Codex missing launcher: %q", missing)
	}
}

func TestMarketplaceACPAgentManifests(t *testing.T) {
	expected := map[string]struct {
		id      string
		name    string
		command string
		args    []string
	}{
		"agent-codex/plugin.json": {
			id: "agent-codex.acp", name: "Codex", command: "npx",
			args: []string{"-y", "@agentclientprotocol/codex-acp@2.7.0"},
		},
		"agent-pi/plugin.json":  {id: "agent-pi.acp", name: "Pi", command: "npx", args: []string{"-y", "pi-acp@0.0.34"}},
		"agent-omp/plugin.json": {id: "agent-omp.acp", name: "OMP", command: "omp", args: []string{"acp"}},
		"agent-dsh/plugin.json": {id: "agent-dsh.acp", name: "DSH", command: "dsh", args: []string{"--profile", "acp"}},
		"agent-claude/plugin.json": {
			id: "agent-claude.acp", name: "Claude Code", command: "npx",
			args: []string{"-y", "@agentclientprotocol/claude-agent-acp@0.107.0"},
		},
		"agent-gemini/plugin.json": {
			id: "agent-gemini.acp", name: "Gemini CLI", command: "gemini",
			args: []string{"--acp"},
		},
		"agent-qwen/plugin.json": {
			id: "agent-qwen.acp", name: "Qwen Code", command: "qwen",
			args: []string{"--acp"},
		},
		"agent-opencode/plugin.json": {
			id: "agent-opencode.acp", name: "OpenCode", command: "opencode",
			args: []string{"acp"},
		},
		"agent-kimi/plugin.json": {
			id: "agent-kimi.acp", name: "Kimi Code", command: "kimi",
			args: []string{"acp"},
		},
	}
	files, err := fs.Glob(os.DirFS("."), "agent-*/plugin.json")
	if err != nil || len(files) != len(expected) {
		t.Fatalf("agent manifests = %v, %v", files, err)
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			want, ok := expected[file]
			if !ok {
				t.Fatalf("unexpected ACP provider: %s", file)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			m, err := plugin.ParseManifest(data)
			if err != nil {
				t.Fatalf("%s: %v", file, err)
			}
			if m.HasCode() || m.DefaultOn() || len(m.Contributes.AgentServers) != 1 || len(m.Contributes.Agents) != 0 {
				t.Fatalf("%s must be an opt-in declarative ACP provider", file)
			}
			s := m.Contributes.AgentServers[0]
			if s.ID != want.id || s.Name != want.name || s.Command != want.command || !reflect.DeepEqual(s.Args, want.args) || len(s.Env) != 0 {
				t.Fatalf("%s has an unexpected ACP launch configuration: %+v", file, s)
			}
		})
	}
}

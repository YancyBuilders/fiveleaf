package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"fiveleaf/internal/settings"
)

func TestAgentCodexAdapterPrefersInstalledCLI(t *testing.T) {
	t.Setenv("CODEX_PATH", "")
	if err := os.Unsetenv("CODEX_PATH"); err != nil {
		t.Fatal(err)
	}
	previous := lookPath
	t.Cleanup(func() { lookPath = previous })
	installed := filepath.Join(t.TempDir(), "tools", "codex")
	lookPath = func(name string) (string, error) {
		if name != "codex" {
			t.Fatalf("unexpected executable lookup: %q", name)
		}
		return installed, nil
	}
	for _, server := range []settings.AgentServer{
		{Command: "npx", Args: []string{"-y", "@agentclientprotocol/codex-acp@2.6.0"}},
		{Command: filepath.Join("tools", "codex-acp")},
		{Command: "codex-acp.cmd"},
		{Command: "codex-acp.exe"},
		{Command: "node", Args: []string{filepath.Join("cache", "node_modules", "@agentclientprotocol", "codex-acp", "dist", "index.js")}},
		{Command: "npx", Args: []string{"@zed-industries/codex-acp@latest"}},
	} {
		t.Run(server.Command, func(t *testing.T) {
			server.Env = map[string]string{"KEEP": "value"}
			server.WorkingDirectory = "tools"
			config := agentACPConfig(agentProvider{name: "Codex", config: server})
			if config.Env["CODEX_PATH"] != installed || config.Env["KEEP"] != "value" || config.Command != server.Command || !reflect.DeepEqual(config.Args, server.Args) || config.WorkingDirectory != "tools" || config.Name != "Codex" {
				t.Fatalf("adapter launch lost the installed CLI or registration: %+v", config)
			}
			if _, mutated := server.Env["CODEX_PATH"]; mutated {
				t.Fatal("runtime CLI discovery changed the saved agent definition")
			}
		})
	}
}

func TestAgentCodexAdapterPreservesExplicitCLI(t *testing.T) {
	previous := lookPath
	t.Cleanup(func() { lookPath = previous })
	lookPath = func(string) (string, error) {
		t.Fatal("an explicit CODEX_PATH must not trigger local discovery")
		return "", nil
	}
	for _, value := range []string{"custom-codex", ""} {
		server := settings.AgentServer{Command: "codex-acp", Env: map[string]string{"CODEX_PATH": value}}
		if got := agentACPConfig(agentProvider{config: server}).Env["CODEX_PATH"]; got != value {
			t.Fatalf("explicit CLI %q changed to %q", value, got)
		}
	}
	for _, value := range []string{"inherited-codex", ""} {
		t.Setenv("CODEX_PATH", value)
		if got := agentACPConfig(agentProvider{config: settings.AgentServer{Command: "codex-acp"}}); got.Env != nil {
			t.Fatalf("runtime discovery overrides inherited CODEX_PATH %q: %+v", value, got.Env)
		}
	}
}

func TestAgentCodexAdapterKeepsBundledFallback(t *testing.T) {
	t.Setenv("CODEX_PATH", "")
	if err := os.Unsetenv("CODEX_PATH"); err != nil {
		t.Fatal(err)
	}
	previous := lookPath
	t.Cleanup(func() { lookPath = previous })
	lookPath = func(string) (string, error) { return "", errors.New("not installed") }
	server := settings.AgentServer{Command: "codex-acp"}
	if config := agentACPConfig(agentProvider{config: server}); config.Env != nil {
		t.Fatalf("missing local CLI disables the adapter's bundled fallback: %+v", config.Env)
	}
	lookPath = func(string) (string, error) {
		t.Fatal("non-Codex ACP agents must not resolve a Codex CLI")
		return "", nil
	}
	for _, server := range []settings.AgentServer{
		{Command: "pi-acp"},
		{Command: "npx", Args: []string{"@agentclientprotocol/codex-acp-other"}},
		{Command: "echo", Args: []string{"@agentclientprotocol/codex-acp@2.6.0"}},
		{Command: "codex", Args: []string{"exec"}, Protocol: "cli"},
		{Command: "codex-acp", Protocol: "cli"},
	} {
		if config := agentACPConfig(agentProvider{config: server}); config.Env != nil {
			t.Fatalf("unrelated agent inherited a Codex override: %+v", config)
		}
	}
}

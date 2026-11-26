package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/agents"
	"fiveleaf/internal/settings"
)

type codexLaunchProfile struct {
	home, userProfile, codexHome, executable string
}

func agentCodexLaunchProfile(config acp.Config) codexLaunchProfile {
	value := func(name string) string {
		if configured, ok := config.Env[name]; ok {
			return configured
		}
		return os.Getenv(name)
	}
	return codexLaunchProfile{home: value("HOME"), userProfile: value("USERPROFILE"), codexHome: value("CODEX_HOME"), executable: value("CODEX_PATH")}
}

func agentACPConfig(provider agentProvider) acp.Config {
	server := provider.config
	config := acp.Config{Name: provider.name, Command: server.Command, Args: server.Args,
		Env: server.Env, WorkingDirectory: server.WorkingDirectory}
	if !isCodexACP(server) {
		return config
	}
	if _, explicit := server.Env["CODEX_PATH"]; explicit {
		return config
	}
	if _, inherited := os.LookupEnv("CODEX_PATH"); inherited {
		return config
	}
	// npm can omit the adapter's optional Codex binary. Prefer the installed
	// CLI when available; otherwise let the adapter use its bundled version.
	if path, err := lookPath("codex"); err == nil && path != "" {
		config.Env = maps.Clone(config.Env)
		if config.Env == nil {
			config.Env = make(map[string]string)
		}
		config.Env["CODEX_PATH"] = path
	}
	return config
}

func resumeACPConfig(provider agentProvider, source agents.Session) acp.Config {
	config := agentACPConfig(provider)
	if source.Root == "" {
		return config
	}
	var key string
	switch source.Harness {
	case agents.Codex:
		key = "CODEX_HOME"
	case agents.Claude:
		key = "CLAUDE_CONFIG_DIR"
	case agents.Pi:
		key = "PI_CODING_AGENT_DIR"
	default:
		return config
	}
	config.Env = maps.Clone(config.Env)
	if config.Env == nil {
		config.Env = make(map[string]string)
	}
	config.Env[key] = source.Root
	return config
}

func isCodexACP(server settings.AgentServer) bool {
	if server.EffectiveProtocol() != "acp" {
		return false
	}
	command := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(server.Command), ".exe"), ".cmd")
	if command == "codex-acp" {
		return true
	}
	switch command {
	case "node", "npx", "npm", "pnpm", "yarn", "bun", "bunx":
	default:
		return false
	}
	for _, argument := range server.Args {
		for _, pkg := range []string{"@agentclientprotocol/codex-acp", "@zed-industries/codex-acp"} {
			if argument == pkg || strings.HasPrefix(argument, pkg+"@") ||
				strings.HasSuffix(filepath.ToSlash(argument), "/"+pkg+"/dist/index.js") {
				return true
			}
		}
	}
	return false
}

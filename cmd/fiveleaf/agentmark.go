package main

import (
	"cmp"
	"embed"
	"path/filepath"
	"strings"

	"fiveleaf/internal/agents"
	"github.com/egoist/mygo/ui"
)

//go:embed agentmarks/*.svg
var chatMarkFiles embed.FS

var chatMarks = func() map[string]*ui.SVG {
	entries, err := chatMarkFiles.ReadDir("agentmarks")
	if err != nil {
		panic(err)
	}
	marks := make(map[string]*ui.SVG, len(entries))
	for _, entry := range entries {
		data, err := chatMarkFiles.ReadFile("agentmarks/" + entry.Name())
		if err != nil {
			panic(err)
		}
		marks[strings.TrimSuffix(entry.Name(), ".svg")] = ui.MustParseSVG(data)
	}
	return marks
}()

func chatAgentKey(name string) string {
	key := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "custom-")
	if strings.HasPrefix(key, "agent-") && strings.HasSuffix(key, ".acp") {
		key = strings.TrimSuffix(strings.TrimPrefix(key, "agent-"), ".acp")
	}
	switch key {
	case "pi", "@earendil-works/pi-coding-agent":
		key = "pi-acp"
	case "oh my pi", "oh-my-pi", "@oh-my-pi/pi-coding-agent":
		key = "omp"
	case "opencode-ai":
		key = "opencode"
	case "claude", "claude code", "claude-code", "claude-agent-acp", "@agentclientprotocol/claude-agent-acp", "@zed-industries/claude-agent-acp", "@anthropic-ai/claude-code":
		key = "claude-acp"
	case "codex", "@openai/codex", "@agentclientprotocol/codex-acp", "@zed-industries/codex-acp":
		key = "codex-acp"
	case "grok", "grok build", "@xai-official/grok":
		key = "grok-build"
	case "qodercli":
		key = "qoder"
	case "gemini cli", "gemini-cli", "@google/gemini-cli":
		key = "gemini"
	case "qwen", "qwen code", "@qwen-code/qwen-code":
		key = "qwen-code"
	case "kimi", "kimi code", "@moonshot-ai/kimi-code":
		key = "kimi-code"
	}
	return key
}

func namedChatMark(pal *palette, name string) *ui.SVG {
	if name == "" {
		return nil
	}
	if mark := chatMarks[chatAgentKey(name)]; mark != nil {
		return mark
	}
	for _, agent := range agents.All() {
		if agent.Icon != "" && (strings.EqualFold(name, string(agent.ID)) || strings.EqualFold(name, agent.App) || strings.EqualFold(name, agent.Name)) {
			if mark, _ := agentIcon(pal, agent.ID); mark != iconAgent {
				return mark
			}
		}
	}
	return nil
}

func (provider agentProvider) commandName() string {
	command := strings.ToLower(filepath.Base(provider.config.Command))
	command = strings.TrimSuffix(strings.TrimSuffix(command, ".exe"), ".cmd")
	switch command {
	case "npx", "npm", "pnpm", "yarn", "bun", "bunx":
		for _, arg := range provider.config.Args {
			switch arg {
			case "-y", "--yes", "exec", "dlx", "x", "--":
				continue
			}
			if version := strings.LastIndexByte(arg, '@'); version > 0 {
				arg = arg[:version]
			}
			command = arg
			break
		}
	}
	return command
}

func (provider agentProvider) chatMark(pal *palette) *ui.SVG {
	if provider.id == "" {
		return iconAgent
	}
	if mark := namedChatMark(pal, provider.id); mark != nil {
		return mark
	}
	if mark := namedChatMark(pal, provider.commandName()); mark != nil {
		return mark
	}
	if isCodexACP(provider.config) {
		return chatMarks["codex-acp"]
	}
	return cmp.Or(namedChatMark(pal, provider.name), iconAgent)
}

func findChatProvider(id string, providers []agentProvider) agentProvider {
	for _, provider := range providers {
		if provider.id == id {
			return provider
		}
	}
	return agentProvider{id: id, name: id}
}

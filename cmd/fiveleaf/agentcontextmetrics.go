package main

import (
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/agents"
	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
)

type agentCompactionPolicy struct {
	known, enabled bool
	reserveTokens  int
	overrides      map[string]agentCompactionOverride
}

type agentCompactionOverride struct {
	known         bool
	reserveTokens int
}

type agentContextAccounting struct {
	session      string
	generation   uint64
	messageStart int
	input        map[string]int
}

var agentContextCategoryKinds = []string{
	"system", "instructions", "skills", "builtin_tools", "custom_tools", "user", "assistant", "thinking", "tool_calls", "tool_output", "unattributed",
}

// This is a display estimate, not a tokenizer: one token per four UTF-8 bytes.
// Only text actually sent in the bounded prompt or exposed by this ACP session
// contributes; adapter system prompts, skills and tool schemas remain unknown.
func agentContextTextTokens(text string) int {
	return (len(text) + 3) / 4
}

func captureAgentContext(s *agentChat, messages []roundtable.Message) string {
	accounting := agentContextAccounting{session: s.sessionID, generation: s.generation,
		messageStart: len(s.messages), input: make(map[string]int)}
	for _, message := range messages {
		kind := message.Role
		if kind == "system" {
			// FiveLeaf sends its custom instructions inside the ACP user prompt;
			// they are not the adapter's private system prompt or instruction files.
			kind = "user"
		}
		if kind == "user" || kind == "assistant" {
			// chatContext already limits history below FormatPrompt's aggregate
			// byte budget; formatting each retained block also respects its caps.
			accounting.input[kind] += agentContextTextTokens(roundtable.FormatPrompt([]roundtable.Message{message}))
		}
	}
	s.contextAccounting = accounting
	return roundtable.FormatPrompt(messages)
}

func captureCodexAgentContext(chat *agentChat, text string) []acp.ContentBlock {
	messages := chatContext(chat)
	last := len(messages) - 1
	if last >= 0 && messages[last].Role == "system" {
		last--
	}
	if last >= 0 && messages[last].Role == "user" && messages[last].Text == text {
		messages = slices.Delete(messages, last, last+1)
	}
	context := captureAgentContext(chat, messages)
	chat.contextAccounting.input["user"] += agentContextTextTokens(text)
	if context == "" && !agents.CodexPromptNeedsSeparateBody(text) {
		return []acp.ContentBlock{acp.TextBlock(text)}
	}
	chat.contextAccounting.input["user"] += agentContextTextTokens(agents.CodexContextHeader)
	return []acp.ContentBlock{acp.TextBlock(agents.CodexContextHeader + context), acp.TextBlock(text)}
}

func (p agentCompactionPolicy) forModel(model string) (known, enabled bool, reserve int) {
	if !p.known {
		return false, false, 0
	}
	reserve = p.reserveTokens
	if override, found := p.overrides[model]; found {
		if !override.known {
			return false, false, 0
		}
		reserve = override.reserveTokens
	}
	return true, p.enabled, reserve
}

func codexCompactionStatus(s *agentChat) agentCompactionStatus {
	if s.sessionID == "" || !s.usesCodexThread() {
		return agentCompactionUnreported
	}
	for _, tool := range slices.Backward(s.tools) {
		if tool == nil || tool.sessionID != s.sessionID || tool.generation != s.generation ||
			!codexCompactionTool(tool) {
			continue
		}
		switch tool.status {
		case "in_progress":
			if s.busy && tool.turn == s.turn {
				return agentCompactionRunning
			}
		case "completed":
			return agentCompactionCompleted
		case "failed":
			return agentCompactionFailed
		}
		break
	}
	return agentCompactionManaged
}

func codexCompactionTool(tool *agentTool) bool {
	return tool.kind == "think" && tool.title == "Compact conversation"
}

func (a *app) contextSnapshot(s *agentChat) agentContextSnapshot {
	model := s.currentModel
	if model == "" {
		model = s.model()
	}
	snapshot := agentContextSnapshot{model: model, compactionStatus: codexCompactionStatus(s)}
	for _, choice := range s.models {
		if choice.ID == snapshot.model && choice.Name != "" {
			snapshot.model = choice.Name
			break
		}
	}
	for _, kind := range agentContextCategoryKinds {
		snapshot.categories = append(snapshot.categories, agentContextCategory{kind: kind})
	}
	usage := s.contextUsage
	if usage == nil || usage.Used < 0 || usage.Size <= 0 {
		return snapshot
	}
	snapshot.known, snapshot.used, snapshot.size = true, usage.Used, usage.Size
	known, enabled, reserve := s.contextPolicy.forModel(model)
	snapshot.compactionKnown, snapshot.compactionEnabled = known, enabled
	if known && enabled {
		snapshot.compactAt = max(0, usage.Size-reserve)
	}
	estimates := make(map[string]int)
	accounting := s.contextAccounting
	if accounting.session != "" && accounting.session == s.sessionID && accounting.generation == s.generation {
		for kind, tokens := range accounting.input {
			estimates[kind] = tokens
		}
		for _, message := range s.messages[min(accounting.messageStart, len(s.messages)):] {
			if message == nil {
				continue
			}
			switch message.role {
			case "assistant", "commentary":
				estimates["assistant"] += agentContextTextTokens(message.text)
			case "thought", "thinking":
				estimates["thinking"] += agentContextTextTokens(message.text)
			}
		}
		// Tool messages and s.tools share pointers; count each tool only here.
		codex := snapshot.compactionStatus != agentCompactionUnreported
		native := codex || s.continuation != nil && s.continuation.native
		for _, tool := range s.tools {
			if tool == nil || tool.sessionID != s.sessionID || tool.generation != s.generation {
				continue
			}
			if native && tool.turn != s.turn {
				continue
			}
			if codex && codexCompactionTool(tool) {
				if tool.status == "completed" {
					clear(estimates)
					break
				}
				continue
			}
			if len(tool.rawInput) > 0 || tool.title != "" {
				estimates["tool_calls"] += agentContextTextTokens(string(tool.rawInput)) + agentContextTextTokens(tool.title)
			}
			if output := tool.outputText(); output != "" {
				estimates["tool_output"] += agentContextTextTokens(output)
			}
		}
	}
	total := 0
	for _, tokens := range estimates {
		total += tokens
	}
	remaining := usage.Used
	for i := range snapshot.categories {
		category := &snapshot.categories[i]
		if category.kind == "unattributed" {
			category.known, category.tokens = true, remaining
			continue
		}
		tokens, found := estimates[category.kind]
		if !found {
			continue
		}
		if total > usage.Used && total > 0 {
			tokens = int(float64(tokens) * float64(usage.Used) / float64(total))
		}
		category.known, category.tokens = true, min(tokens, remaining)
		remaining -= category.tokens
		snapshot.estimated = true
	}
	return snapshot
}

type agentPiSettings struct {
	Compaction          json.RawMessage `json:"compaction"`
	DefaultProjectTrust json.RawMessage `json:"defaultProjectTrust"`
}

func isPiACP(config settings.AgentServer) bool {
	command := filepath.Base(config.Command)
	if command == "pi-acp" || command == "pi-acp.cmd" || command == "pi-acp.exe" {
		return true
	}
	for _, argument := range config.Args {
		if argument == "pi-acp" || strings.HasPrefix(argument, "pi-acp@") {
			return true
		}
	}
	return false
}

// piContextWindow returns the configured context window for a Pi model. Some
// adapters report maxTokens (the output limit) as usage size; that is not the
// context capacity and must not become the progress bar denominator.
func piContextWindow(config settings.AgentServer, root, model string, reported int) (int, bool) {
	if !isPiACP(config) || model == "" {
		return 0, false
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return 0, false
	}
	env := func(name string) string {
		if value, exists := config.Env[name]; exists {
			return value
		}
		return os.Getenv(name)
	}
	home := env("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	agentDir := env("PI_CODING_AGENT_DIR")
	if agentDir == "" {
		agentDir = filepath.Join(home, ".pi", "agent")
	} else if agentDir == "~" || strings.HasPrefix(agentDir, "~/") {
		agentDir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(agentDir, "~"), "/"))
	}
	if !filepath.IsAbs(agentDir) {
		agentDir = filepath.Join(root, agentDir)
	}
	var catalog struct {
		Providers map[string]struct {
			Models []struct {
				ID            string `json:"id"`
				ContextWindow int    `json:"contextWindow"`
				MaxTokens     int    `json:"maxTokens"`
			} `json:"models"`
		} `json:"providers"`
	}
	if !readAgentContextJSON(filepath.Join(agentDir, "models.json"), &catalog) {
		return 0, false
	}
	for _, entry := range catalog.Providers {
		for _, candidate := range entry.Models {
			if candidate.ID != model && !strings.HasSuffix(model, "/"+candidate.ID) {
				continue
			}
			if candidate.ContextWindow > 0 && candidate.MaxTokens > 0 && reported == candidate.MaxTokens && reported != candidate.ContextWindow {
				return candidate.ContextWindow, true
			}
			return 0, false
		}
	}
	return 0, false
}

// Pi's settings are read when its ACP session is created. Only the compaction and
// project-trust projections survive parsing; auth and model files are not read.
func loadAgentCompactionPolicy(config settings.AgentServer, root string) agentCompactionPolicy {
	if !isPiACP(config) || config.EffectiveProtocol() != "acp" {
		return agentCompactionPolicy{}
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return agentCompactionPolicy{}
	}
	env := func(name string) string {
		if value, exists := config.Env[name]; exists {
			return value
		}
		return os.Getenv(name)
	}
	home := env("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	agentDir := env("PI_CODING_AGENT_DIR")
	if agentDir == "" {
		if home == "" {
			return agentCompactionPolicy{}
		}
		agentDir = filepath.Join(home, ".pi", "agent")
	} else if agentDir == "~" || strings.HasPrefix(agentDir, "~/") {
		if home == "" {
			return agentCompactionPolicy{}
		}
		agentDir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(agentDir, "~"), "/"))
	}
	if !filepath.IsAbs(agentDir) {
		agentDir = filepath.Join(root, agentDir)
	}
	var global agentPiSettings
	if !readAgentContextJSON(filepath.Join(agentDir, "settings.json"), &global) {
		return agentCompactionPolicy{}
	}
	trusted, known := agentPiProjectTrusted(agentDir, root, global.DefaultProjectTrust)
	if !known {
		return agentCompactionPolicy{}
	}
	compaction := global.Compaction
	if trusted {
		// Pi ignores project settings before touching storage when the workspace
		// is untrusted; unreadable or special project files must not affect it.
		var project agentPiSettings
		if !readAgentContextJSON(filepath.Join(root, ".pi", "settings.json"), &project) {
			return agentCompactionPolicy{}
		}
		if project.Compaction != nil {
			compaction = mergeAgentCompactionJSON(compaction, project.Compaction)
		}
	}
	return parseAgentCompactionPolicy(compaction)
}

func readAgentContextJSON(path string, value any) bool {
	const limit = 1 << 20
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return false
	}
	// Nonblocking open also covers a file replaced by a FIFO after Stat.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	return err == nil && len(raw) <= limit && json.Unmarshal(raw, value) == nil
}

func agentPiProjectTrusted(agentDir, root string, fallback json.RawMessage) (trusted, known bool) {
	var entries map[string]json.RawMessage
	if !readAgentContextJSON(filepath.Join(agentDir, "trust.json"), &entries) {
		return false, false
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		if raw, exists := entries[dir]; exists && string(raw) != "null" {
			var allowed bool
			if json.Unmarshal(raw, &allowed) != nil {
				return false, false
			}
			return allowed, true
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	if fallback == nil || string(fallback) == "null" {
		return false, true
	}
	var choice string
	if json.Unmarshal(fallback, &choice) != nil {
		return false, false
	}
	switch choice {
	case "always":
		return true, true
	case "never", "ask":
		// pi-acp's RPC child cannot interactively approve project trust.
		return false, true
	default:
		return false, false
	}
}

// Pi deep-merges object settings; a primitive, array or null replaces the old
// value. Restrict this merge to the already-extracted compaction object.
func mergeAgentCompactionJSON(base, overlay json.RawMessage) json.RawMessage {
	var from, into map[string]json.RawMessage
	if json.Unmarshal(base, &from) != nil || from == nil || json.Unmarshal(overlay, &into) != nil || into == nil {
		return overlay
	}
	for key, value := range from {
		if newer, exists := into[key]; exists {
			into[key] = mergeAgentCompactionJSON(value, newer)
		} else {
			into[key] = value
		}
	}
	merged, _ := json.Marshal(into)
	return merged
}

func parseAgentCompactionPolicy(raw json.RawMessage) agentCompactionPolicy {
	policy := agentCompactionPolicy{known: true, enabled: true, reserveTokens: 16384}
	if len(raw) == 0 || string(raw) == "null" {
		return policy
	}
	var config struct {
		Enabled          json.RawMessage            `json:"enabled"`
		ReserveTokens    json.RawMessage            `json:"reserveTokens"`
		KeepRecentTokens json.RawMessage            `json:"keepRecentTokens"`
		ModelOverrides   map[string]json.RawMessage `json:"modelOverrides"`
	}
	if json.Unmarshal(raw, &config) != nil {
		return agentCompactionPolicy{}
	}
	if config.Enabled != nil && string(config.Enabled) != "null" && json.Unmarshal(config.Enabled, &policy.enabled) != nil {
		return agentCompactionPolicy{}
	}
	reserve, valid := agentCompactionTokens(config.ReserveTokens, 16384)
	_, keepValid := agentCompactionTokens(config.KeepRecentTokens, 20000)
	if !valid || !keepValid {
		return agentCompactionPolicy{}
	}
	policy.reserveTokens = reserve
	policy.overrides = make(map[string]agentCompactionOverride, len(config.ModelOverrides))
	for model, value := range config.ModelOverrides {
		var override map[string]json.RawMessage
		if json.Unmarshal(value, &override) != nil || override == nil {
			policy.overrides[model] = agentCompactionOverride{}
			continue
		}
		budget, ok := agentCompactionTokens(override["reserveTokens"], reserve)
		_, keepOK := agentCompactionTokens(override["keepRecentTokens"], 20000)
		policy.overrides[model] = agentCompactionOverride{known: ok && keepOK, reserveTokens: budget}
	}
	return policy
}

func agentCompactionTokens(raw json.RawMessage, fallback int) (int, bool) {
	if len(raw) == 0 {
		return fallback, true
	}
	if string(raw) == "null" {
		return 0, false
	}
	var number float64
	if json.Unmarshal(raw, &number) != nil || number < 0 || number > 9007199254740991 || math.Trunc(number) != number {
		return 0, false
	}
	return int(number), true
}

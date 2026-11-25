package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/settings"
)

type roleRuntimeEvent struct {
	Kind, Session, Model, Mode, GenericMode, Text, Env string
	Args                                               []string
}

func TestAgentRoleRuntimeProcess(t *testing.T) {
	scenario := os.Getenv("FIVELEAF_ROLE_RUNTIME")
	if scenario == "" {
		return
	}
	logEvent := func(event roleRuntimeEvent) {
		raw, _ := json.Marshal(event)
		file, err := os.OpenFile(os.Getenv("FIVELEAF_ROLE_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			os.Exit(2)
		}
		_, err = file.Write(append(raw, '\n'))
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			os.Exit(2)
		}
	}
	logEvent(roleRuntimeEvent{Kind: "start"})
	encoder := json.NewEncoder(os.Stdout)
	if scenario == "cli" {
		var text string
		if os.Getenv("FIVELEAF_ROLE_PROMPT_ARG") == "1" {
			text = os.Args[len(os.Args)-1]
		} else {
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				os.Exit(2)
			}
			text = string(raw)
		}
		event := roleRuntimeEvent{Kind: "cli", Text: text, Args: os.Args[2:], Env: os.Getenv("FIVELEAF_ROLE_LITERAL")}
		logEvent(event)
		encoder.Encode(event)
		os.Exit(0)
	}
	respond := func(id json.RawMessage, value any) {
		encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": value})
	}
	answer := func(id json.RawMessage, session, text string) {
		encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": session, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}},
		}})
		respond(id, map[string]string{"stopReason": "end_turn"})
	}
	options := func(current string) []any {
		return []any{map[string]any{"id": "engine", "category": "model", "type": "select", "name": "Model", "currentValue": current,
			"options": []any{map[string]string{"name": "Default", "value": "default"}, map[string]string{"name": "A", "value": "model-a"}, map[string]string{"name": "B", "value": "model-b"}}}}
	}
	thinkingLevels := func(model string) []string {
		if scenario == "thinking-empty" {
			return []string{"", "low", "medium", "high", "max"}
		}
		if model == "model-b" {
			return []string{"off", "low", "high"}
		}
		return []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
	}
	thinkingOptions := func(model, mode string) []any {
		var values []any
		for _, level := range thinkingLevels(model) {
			name := level
			if level == "" {
				name = "Provider default"
			}
			values = append(values, map[string]string{"name": name, "value": level})
		}
		thought := map[string]any{"id": "thought_level", "category": "thought_level", "name": "Thinking", "type": "select", "currentValue": mode, "options": values}
		if scenario == "thinking-only" {
			return []any{thought}
		}
		return append(options(model), thought)
	}
	updatedOptions := func(current string) []any {
		return []any{map[string]any{"id": "engine", "category": "model", "type": "select", "name": "Model", "currentValue": current,
			"options": []any{map[string]string{"name": "A", "value": "model-a"}, map[string]string{"name": "C", "value": "model-c"}}}}
	}
	count := 0
	authenticated := false
	selected := map[string]string{}
	selectedModes := map[string]string{}
	selectedGenericModes := map[string]string{}
	thinking := strings.HasPrefix(scenario, "thinking")
	thinkingUpdate := func(session string) {
		encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": session, "update": map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": selectedModes[session]},
		}})
		encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": session, "update": map[string]any{"sessionUpdate": "config_option_update", "configOptions": thinkingOptions(selected[session], selectedModes[session])},
		}})
	}
	var pendingID json.RawMessage
	var pendingSession, pendingText string
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage
			Method string
			Params struct {
				SessionID, ConfigID, Value, ModeID string
				Prompt                             []struct{ Type, Text string }
			}
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		if request.Method == "" && string(request.ID) == `"role-permission"` {
			answer(pendingID, pendingSession, pendingText)
			pendingID = nil
			continue
		}
		switch request.Method {
		case "initialize":
			auth := []any{}
			if scenario == "auth-model-unavailable" {
				auth = append(auth, map[string]string{"id": "login", "name": "Log in"})
			}
			respond(request.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": auth})
		case "authenticate":
			authenticated = true
			respond(request.ID, map[string]any{})
		case "session/new":
			if scenario == "auth-model-unavailable" && !authenticated {
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32000, "message": "authentication required"}})
				continue
			}
			count++
			id := fmt.Sprintf("role-%d", count)
			selected[id] = "default"
			selectedModes[id] = "ask"
			selectedGenericModes[id] = "ask"
			logEvent(roleRuntimeEvent{Kind: "new", Session: id})
			if scenario == "composer-commands" {
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": id, "update": map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": []any{
						map[string]string{"name": "status", "description": "Show session status"},
						map[string]any{"name": "skill:review", "description": "Review changes", "input": map[string]string{"hint": "file"}},
					}},
				}})
			}
			if scenario == "welcome-on-open" {
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": id, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "Adapter startup details marker"}},
				}})
			}
			result := map[string]any{"sessionId": id}
			if scenario == "late-startup" {
				result["_meta"] = map[string]any{"piAcp": map[string]string{"startupInfo": "Adapter startup details marker"}}
			}
			if scenario == "mode-resets-model" {
				result["modes"] = map[string]any{"currentModeId": "ask", "availableModes": []any{map[string]string{"id": "ask", "name": "Ask"}, map[string]string{"id": "plan", "name": "Plan"}}}
			}
			if scenario != "missing" && !(scenario == "second-missing" && count == 2) {
				result["configOptions"] = options("default")
				if thinking {
					selectedModes[id] = "medium"
					result["configOptions"] = thinkingOptions("default", "medium")
					var modes []any
					for _, level := range thinkingLevels("default") {
						modes = append(modes, map[string]string{"id": level, "name": "Thinking: " + level})
					}
					result["modes"] = map[string]any{"currentModeId": "medium", "availableModes": modes}
					if scenario == "thinking-mixed" || scenario == "thinking-empty" {
						result["modes"] = map[string]any{"currentModeId": "ask", "availableModes": []any{
							map[string]string{"id": "ask", "name": "Ask"}, map[string]string{"id": "plan", "name": "Plan"},
						}}
					}
				}
				if scenario == "second-model-removed" && count >= 2 {
					result["configOptions"] = []any{map[string]any{"id": "engine", "category": "model", "type": "select", "name": "Model", "currentValue": "model-a",
						"options": []any{map[string]string{"name": "A", "value": "model-a"}}}}
				}
				if scenario == "auth-model-unavailable" {
					result["configOptions"] = []any{map[string]any{"id": "engine", "category": "model", "type": "select", "name": "Model", "currentValue": "model-c",
						"options": []any{map[string]string{"name": "C", "value": "model-c"}}}}
				}
			}
			respond(request.ID, result)
		case "session/set_config_option":
			if thinking && request.Params.ConfigID == "thought_level" {
				if !slices.Contains(thinkingLevels(selected[request.Params.SessionID]), request.Params.Value) {
					os.Exit(3)
				}
				if scenario == "thinking-error" && request.Params.Value == "high" {
					encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32603, "message": "thinking unavailable"}})
					continue
				}
				mode := request.Params.Value
				if scenario == "thinking-clamp" && mode == "max" {
					mode = "high"
				}
				selectedModes[request.Params.SessionID] = mode
				logEvent(roleRuntimeEvent{Kind: "mode", Session: request.Params.SessionID, Model: selected[request.Params.SessionID], Mode: mode})
				if scenario == "thinking-pending" {
					continue
				}
				thinkingUpdate(request.Params.SessionID)
				respond(request.ID, map[string]any{"configOptions": thinkingOptions(selected[request.Params.SessionID], mode)})
				continue
			}
			logEvent(roleRuntimeEvent{Kind: "model", Session: request.Params.SessionID, Model: request.Params.Value})
			if request.Params.ConfigID != "engine" {
				os.Exit(3)
			}
			if scenario == "setter-fails" || scenario == "second-fails" && count == 2 || scenario == "selection-fails" && request.Params.Value == "model-b" {
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32603, "message": "model unavailable"}})
				continue
			}
			if scenario == "wrong-current-options" && request.Params.Value != "model-a" || scenario == "auth-model-unavailable" {
				current := request.Params.Value
				if current == "model-b" {
					current = "model-a"
				}
				selected[request.Params.SessionID] = current
				respond(request.ID, map[string]any{"configOptions": updatedOptions(current)})
				continue
			}
			selected[request.Params.SessionID] = request.Params.Value
			if thinking {
				selectedModes[request.Params.SessionID] = "medium"
				if request.Params.Value == "model-b" {
					selectedModes[request.Params.SessionID] = "low"
				}
				thinkingUpdate(request.Params.SessionID)
				respond(request.ID, map[string]any{"configOptions": thinkingOptions(request.Params.Value, selectedModes[request.Params.SessionID])})
				continue
			}
			respond(request.ID, map[string]any{"configOptions": options(request.Params.Value)})
		case "session/set_mode":
			selected[request.Params.SessionID] = "default"
			if scenario == "thinking-mixed" || scenario == "thinking-empty" {
				selectedGenericModes[request.Params.SessionID] = request.Params.ModeID
				selectedModes[request.Params.SessionID] = "medium"
				logEvent(roleRuntimeEvent{Kind: "generic-mode", Session: request.Params.SessionID, Model: "default", Mode: "medium", GenericMode: request.Params.ModeID})
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": request.Params.SessionID, "update": map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": request.Params.ModeID},
				}})
				thinkingUpdate(request.Params.SessionID)
				respond(request.ID, map[string]any{})
				continue
			}
			selectedModes[request.Params.SessionID] = request.Params.ModeID
			logEvent(roleRuntimeEvent{Kind: "mode", Session: request.Params.SessionID, Model: "default", Mode: request.Params.ModeID})
			respond(request.ID, map[string]any{})
		case "session/prompt":
			if scenario == "roundtable-tool-preamble" {
				for _, update := range []map[string]any{
					{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "Looking up the fallback"}},
					{"sessionUpdate": "tool_call", "toolCallId": "fixture-read", "title": "Read the requirement", "kind": "read", "status": "completed"},
				} {
					encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": request.Params.SessionID, "update": update}})
				}
				answer(request.ID, request.Params.SessionID, "120秒")
				continue
			}
			if scenario == "late-startup" {
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": request.Params.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "Adapter startup details marker"}},
				}})
			}
			if scenario == "model-notify" {
				selected[request.Params.SessionID] = "model-b"
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": request.Params.SessionID, "update": map[string]any{"sessionUpdate": "config_option_update", "configOptions": options("model-b")},
				}})
			}
			text := ""
			for _, block := range request.Params.Prompt {
				text += block.Text
			}
			logEvent(roleRuntimeEvent{Kind: "prompt", Session: request.Params.SessionID, Model: selected[request.Params.SessionID], Mode: selectedModes[request.Params.SessionID], GenericMode: selectedGenericModes[request.Params.SessionID], Text: text})
			if scenario == "composer-commands" && strings.HasPrefix(text, "/skill:review") {
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": request.Params.SessionID, "update": map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": []any{}},
				}})
			}
			if scenario == "copilot-newline" {
				answer(request.ID, request.Params.SessionID, "\n")
				continue
			}
			if scenario == "copilot-inline" {
				answer(request.ID, request.Params.SessionID, os.Getenv("FIVELEAF_TEST_INLINE_REPLY"))
				continue
			}
			if scenario == "roundtable" {
				reply := "Rollback is covered."
				if selected[request.Params.SessionID] == "model-a" {
					reply = "@Reviewer check the rollback plan"
				}
				answer(request.ID, request.Params.SessionID, reply)
				continue
			}
			if scenario == "permission" {
				pendingID, pendingSession, pendingText = slices.Clone(request.ID), request.Params.SessionID, selected[request.Params.SessionID]+" reply: "+text
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": "role-permission", "method": "session/request_permission", "params": map[string]any{
					"sessionId": request.Params.SessionID, "toolCall": map[string]string{"toolCallId": "permission", "title": "Role permission"},
					"options": []any{map[string]string{"optionId": "allow", "name": "Allow", "kind": "allow_once"}},
				}})
				continue
			}
			answer(request.ID, request.Params.SessionID, text)
		}
	}
	os.Exit(0)
}

func agentRoleHarness(t *testing.T, scenario string) (*harness, string) {
	t.Helper()
	hs := newHarness(t, true)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "role-requests.jsonl")
	protocol := "acp"
	if scenario == "cli" {
		protocol = "cli"
	}
	hs.a.settings.AgentServers = map[string]settings.AgentServer{"backend": {
		Name: "Backend", Protocol: protocol, Command: executable, Args: []string{"-test.run=^TestAgentRoleRuntimeProcess$"}, TimeoutSeconds: 30,
		Env: map[string]string{"FIVELEAF_ROLE_RUNTIME": scenario, "FIVELEAF_ROLE_LOG": logPath},
	}}
	hs.a.settings.AgentRoles = map[string]settings.AgentRole{"审阅 角色": {Agent: "backend", Model: "model-a", Prompt: "Check the code carefully."}}
	hs.a.selectAgentRole("审阅 角色")
	t.Cleanup(hs.a.closeAllAgents)
	return hs, logPath
}

func agentRoleEvents(t *testing.T, path string) []roleRuntimeEvent {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	var events []roleRuntimeEvent
	for _, line := range lines[:len(lines)-1] {
		var event roleRuntimeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestAgentRoleModelAppliedBeforeEveryPrompt(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	for _, text := range []string{"first", "second"} {
		lifecycleSend(t, hs, text)
		if hs.a.agent.err != "" {
			t.Fatal(hs.a.agent.err)
		}
	}
	models, prompts := map[string]bool{}, 0
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "model" {
			models[event.Session] = event.Model == "model-a"
		}
		if event.Kind == "prompt" {
			prompts++
			if !models[event.Session] || event.Model != "model-a" || !strings.Contains(event.Text, "[system]\nCheck the code carefully.") {
				t.Fatalf("prompt preceded role model/prompt setup: %+v", event)
			}
		}
	}
	if prompts != 2 || len(models) != 2 {
		t.Fatalf("expected two fresh configured sessions, models=%v prompts=%d", models, prompts)
	}
}

func TestAgentRoleModelFailuresRetainDraftAndSendNoPrompt(t *testing.T) {
	for _, scenario := range []string{"missing", "unsupported-model", "setter-fails", "second-missing", "second-fails"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, scenario)
			if scenario == "unsupported-model" {
				role := hs.a.settings.AgentRoles["审阅 角色"]
				role.Model = "not-advertised"
				hs.a.settings.AgentRoles["审阅 角色"] = role
			}
			wantPrompts := 0
			if strings.HasPrefix(scenario, "second-") {
				lifecycleSend(t, hs, "first succeeds")
				if hs.a.agent.err != "" {
					t.Fatal(hs.a.agent.err)
				}
				wantPrompts = 1
			}
			lifecycleSend(t, hs, "keep this draft")
			if hs.a.agent.err == "" || hs.a.agent.draft != "keep this draft" {
				t.Fatalf("model failure lost error/draft: error=%q draft=%q", hs.a.agent.err, hs.a.agent.draft)
			}
			prompts := 0
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind == "prompt" {
					prompts++
				}
			}
			if prompts != wantPrompts {
				t.Fatalf("model failure still sent prompt: got %d want %d", prompts, wantPrompts)
			}
		})
	}
}

func TestAgentRolesKeepIndependentChatsPermissionsAndReplies(t *testing.T) {
	hs, _ := agentRoleHarness(t, "permission")
	a, one := hs.a, hs.a.agent
	a.settings.AgentRoles["第二角色"] = settings.AgentRole{Agent: "backend", Model: "model-b", Prompt: "Second system"}
	one.draft = "first private request"
	a.sendAgent()
	hs.untilLong("first role permission", func() bool { return len(one.permissions) > 0 || one.err != "" })
	if one.err != "" {
		t.Fatal(one.err)
	}
	one.draft, one.scroll.Y = "next first draft", 73
	a.selectAgentRole("第二角色")
	two := a.agent
	hs.untilLong("second role connection", func() bool { return two.sessionID != "" })
	two.draft = "second private request"
	a.sendAgent()
	hs.untilLong("second role permission", func() bool { return len(two.permissions) > 0 || two.err != "" })
	if two.err != "" || two == one {
		t.Fatalf("second role shared or failed: %s", two.err)
	}
	two.draft, two.scroll.Y = "next second draft", 21
	a.selectAgent("backend")
	direct := a.agent
	direct.draft = "direct draft"
	if a.agentState("backend") != direct || a.agentRoleState("审阅 角色") != one || a.agentRoleState("第二角色") != two || a.agentPermissionCount() != 2 {
		t.Fatal("role/direct identity or hidden permissions were merged")
	}
	one.permissions[0].choose("allow")
	hs.untilLong("hidden first role reply", func() bool { return !one.busy })
	if a.agent != direct || direct.draft != "direct draft" || len(direct.messages) != 0 || !two.busy || len(two.permissions) != 1 {
		t.Fatal("hidden role reply crossed conversation boundaries")
	}
	two.permissions[0].choose("allow")
	hs.untilLong("hidden second role reply", func() bool { return !two.busy })
	if one.draft != "next first draft" || two.draft != "next second draft" || one.scroll.Y != 73 || two.scroll.Y != 21 || !strings.Contains(one.messages[len(one.messages)-1].text, "model-a reply") || !strings.Contains(two.messages[len(two.messages)-1].text, "model-b reply") {
		t.Fatal("role history, drafts, scroll or model lost its owner")
	}
	a.selectAgentRole("第二角色")
	a.newAgentSession()
	if a.agent != two || two.role != "第二角色" || two.provider != "backend" || two.draft != "next second draft" || len(two.messages) != 0 || len(one.messages) == 0 {
		t.Fatal("new role conversation lost identity or cleared another chat")
	}
}

func TestAgentRoleCLIModelsAndPromptsAreLiteral(t *testing.T) {
	for _, promptArgument := range []bool{false, true} {
		t.Run(fmt.Sprint(promptArgument), func(t *testing.T) {
			hs, _ := agentRoleHarness(t, "cli")
			a := hs.a
			server := a.settings.AgentServers["backend"]
			server.Args = append(server.Args, "--", "--engine={model}", "{model}", "$HOME;$(echo literal)")
			server.Env["FIVELEAF_ROLE_LITERAL"] = "env-{model}-$HOME"
			server.PromptArgument = promptArgument
			if promptArgument {
				server.Env["FIVELEAF_ROLE_PROMPT_ARG"] = "1"
			}
			a.settings.AgentServers["backend"] = server
			original := cloneAgentServer(server)
			model := "model x;$(echo literal){prompt}"
			a.settings.AgentRoles["审阅 角色"] = settings.AgentRole{Agent: "backend", Model: model, Prompt: "System {model} $HOME;$(echo literal)"}
			lifecycleSend(t, hs, "User {model} $HOME;$(echo literal)")
			if a.agent.err != "" {
				t.Fatal(a.agent.err)
			}
			var reply roleRuntimeEvent
			if err := json.Unmarshal([]byte(a.agent.messages[len(a.agent.messages)-1].text), &reply); err != nil {
				t.Fatal(err)
			}
			wantArgs := []string{"--", "--engine=" + model, model, "$HOME;$(echo literal)"}
			if promptArgument {
				wantArgs = append(wantArgs, reply.Text)
			}
			if !reflect.DeepEqual(reply.Args, wantArgs) || reply.Env != "env-{model}-$HOME" || !strings.Contains(reply.Text, "[system]\nSystem {model} $HOME;$(echo literal)") || !strings.Contains(reply.Text, "User {model} $HOME;$(echo literal)") {
				t.Fatalf("role arguments/prompt/env were interpreted: %+v", reply)
			}
			generation := a.agent.generation
			a.checkAgentConfig()
			if !reflect.DeepEqual(a.settings.AgentServers["backend"], original) || a.agent.connected == nil || a.agent.generation != generation || !reflect.DeepEqual(a.agent.connected.config, original) {
				t.Fatal("effective model arguments mutated settings or invalidated the original snapshot")
			}
		})
	}
}

func TestAgentRoleInvalidConfigurationNeverLaunches(t *testing.T) {
	for _, scenario := range []string{"no-placeholder", "placeholder-without-model", "missing-agent", "disabled", "invalid-prompt", "deleted-role"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, "cli")
			a := hs.a
			role := a.settings.AgentRoles["审阅 角色"]
			switch scenario {
			case "placeholder-without-model":
				role.Model = ""
				server := a.settings.AgentServers["backend"]
				server.Args = append(server.Args, "--engine={model}")
				a.settings.AgentServers["backend"] = server
			case "missing-agent":
				role.Agent = "missing"
			case "disabled":
				server := a.settings.AgentServers["backend"]
				server.Disabled = true
				a.settings.AgentServers["backend"] = server
			case "invalid-prompt":
				role.Prompt = strings.Repeat("x", settings.MaxAgentRolePromptBytes+1)
			}
			a.settings.AgentRoles["审阅 角色"] = role
			if scenario == "deleted-role" {
				delete(a.settings.AgentRoles, "审阅 角色")
			}
			a.agent.draft = "preserve unavailable draft"
			a.sendAgent()
			if a.agent.err == "" || a.agent.busy || a.agent.connecting || a.agent.draft != "preserve unavailable draft" || a.agent.role != "审阅 角色" || len(agentRoleEvents(t, logPath)) != 0 {
				t.Fatalf("invalid role launched or lost draft: %+v", a.agent)
			}
		})
	}
}

func TestAgentRoleReloadInvalidatesHiddenProjectBeforeFrame(t *testing.T) {
	for _, change := range []string{"prompt", "model", "retarget", "delete", "disable"} {
		t.Run(change, func(t *testing.T) {
			front, other := newAgentConfigurationProjects(t)
			front.settings.AgentRoles = map[string]settings.AgentRole{"Quiet role": {Agent: "quiet", Prompt: "Original prompt"}}
			other.selectAgentRole("Quiet role")
			chat := other.agent
			chat.draft = "configuration-direct"
			other.sendAgent()
			waitForConfigurationCLI(t, other, "direct")
			ctx, generation := chat.ctx, chat.generation
			other.selectAgent("idle")
			other.agent.draft = "keep selected draft"
			chat.draft = "next role draft"
			m := &fiveleaf{s: front.settings}
			m.wins = []*desktop{{m: m, a: front}, {m: m, a: other}}
			prefs := m.s.Prefs
			prefs.AgentRoles = maps.Clone(prefs.AgentRoles)
			role := prefs.AgentRoles["Quiet role"]
			switch change {
			case "prompt":
				role.Prompt = "Changed prompt"
			case "model":
				role.Model = "new-model"
			case "retarget":
				role.Agent = "idle"
			case "disable":
				prefs.AgentServers = maps.Clone(prefs.AgentServers)
				server := prefs.AgentServers["quiet"]
				server.Disabled = true
				prefs.AgentServers["quiet"] = server
			}
			prefs.AgentRoles["Quiet role"] = role
			if change == "delete" {
				delete(prefs.AgentRoles, "Quiet role")
			}
			m.reload(prefs)
			assertConfigurationChatStopped(t, other, chat, ctx, generation)
			if chat.role != "Quiet role" || !strings.Contains(chat.draft, "configuration-direct") || !strings.Contains(chat.draft, "next role draft") || other.agentRoleState("Quiet role") != chat {
				t.Fatal("role invalidation discarded its identity, draft or cached history")
			}
		})
	}
}

func TestAgentRoleDefaultModelNeedsNoSetter(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "missing")
	role := hs.a.settings.AgentRoles["审阅 角色"]
	role.Model = ""
	hs.a.settings.AgentRoles["审阅 角色"] = role
	lifecycleSend(t, hs, "default model")
	if hs.a.agent.err != "" {
		t.Fatal(hs.a.agent.err)
	}
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "model" {
			t.Fatal("empty model triggered a model setter")
		}
	}
}

func TestAgentRoleRenameRetainsOldChatAndCreatesNewIdentity(t *testing.T) {
	hs, _ := agentRoleHarness(t, "missing")
	a, old := hs.a, hs.a.agent
	old.draft = "old role draft"
	old.messages = []*agentMessage{{role: "assistant", text: "old role history"}}
	role := a.settings.AgentRoles["审阅 角色"]
	delete(a.settings.AgentRoles, "审阅 角色")
	a.settings.AgentRoles["New role name"] = role
	a.checkAgentConfig()
	a.sendAgent()
	if old.err == "" || old.busy || old.connecting || old.role != "审阅 角色" || old.draft != "old role draft" || len(old.messages) != 1 {
		t.Fatal("renamed role continued sending or lost the old conversation")
	}
	a.selectAgentRole("New role name")
	fresh := a.agent
	if fresh == old || fresh.draft != "" || len(fresh.messages) != 0 || fresh.role != "New role name" || a.agentRoleState("审阅 角色") != old {
		t.Fatal("role rename implicitly migrated or discarded a conversation")
	}
	a.selectAgentRole("审阅 角色")
	if a.agent != old || old.draft != "old role draft" || old.messages[0].text != "old role history" {
		t.Fatal("retained unavailable conversation could not be reopened")
	}
}

func TestAgentRoleSnapshotsDetectInPlaceProviderChanges(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	a, chat := hs.a, hs.a.agent
	lifecycleSend(t, hs, "first message")
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	generation, oldArg := chat.generation, chat.connected.config.Args[0]
	a.settings.AgentServers["backend"].Args[0] = "changed argument"
	a.checkAgentConfig()
	if chat.connected != nil || chat.client != nil || chat.err == "" || chat.generation <= generation || oldArg == "changed argument" {
		t.Fatal("in-place config edit mutated the connected snapshot or escaped invalidation")
	}
}

func TestAgentRoleModelRestoredAfterModeChange(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "mode-resets-model")
	a, chat := hs.a, hs.a.agent
	a.connectAgent()
	hs.untilLong("role connection", func() bool { return !chat.connecting })
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	a.setAgentMode("plan")
	hs.untilLong("role mode selection", func() bool { return !chat.configuring })
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	lifecycleSend(t, hs, "keep role model")
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	seenPrompt := false
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			seenPrompt = true
			if event.Model != "model-a" {
				t.Fatalf("mode reset the role model: %+v", event)
			}
		}
	}
	if !seenPrompt {
		t.Fatal("no role prompt was sent")
	}
}

func TestAgentRoleRetryDoesNotDuplicateUnsentQuestion(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "second-fails")
	a, chat := hs.a, hs.a.agent
	lifecycleSend(t, hs, "first question")
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	const question = "unique unsent question"
	lifecycleSend(t, hs, question)
	if chat.err == "" || chat.draft != question {
		t.Fatal("expected model setup failure with retained draft")
	}
	for _, message := range chat.messages {
		if message.role == "user" && message.text == question {
			t.Fatal("failed model setup added an unsent question to history")
		}
	}
	// Repair the adapter while retaining this role's transcript and draft.
	server := cloneAgentServer(a.settings.AgentServers["backend"])
	server.Env["FIVELEAF_ROLE_RUNTIME"] = "acp"
	a.settings.AgentServers["backend"] = server
	a.sendAgent()
	hs.untilLong("retried role reply", func() bool { return !chat.busy && !chat.connecting })
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	var retry string
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" && strings.Contains(event.Text, question) {
			retry = event.Text
		}
	}
	if strings.Count(retry, question) != 1 {
		t.Fatalf("retry duplicated unsent question in context: %q", retry)
	}
}

func TestAgentRoleInvalidNameCannotFallBackToDirectAgent(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "cli")
	a, selected := hs.a, hs.a.agent
	selected.draft = "keep role target"
	a.settings.AgentRoles[""] = settings.AgentRole{Agent: "backend", Model: "model-b", Prompt: "must not be used"}
	a.selectAgentRole("")
	if a.agent != selected || selected.role != "审阅 角色" || selected.provider != "backend" || selected.err == "" || selected.draft != "keep role target" {
		t.Fatal("invalid role name changed the selected conversation")
	}
	a.sendAgent()
	if selected.err == "" || selected.connecting || selected.busy || len(agentRoleEvents(t, logPath)) > 0 {
		t.Fatal("invalid role name bypassed the original role's required CLI model")
	}
	role, _, err := a.resolveAgentRole(selected)
	if err != nil || role.Model != "model-a" {
		t.Fatalf("invalid selection changed the selected role model: %+v, %v", role, err)
	}
}

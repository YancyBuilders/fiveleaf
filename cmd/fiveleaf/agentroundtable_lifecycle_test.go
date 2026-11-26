package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type lifecycleEvent struct {
	Kind, Session, Mode string
	Text, Context       string
	Accepted            bool
	PID                 int
}

// This process fixture has independent session modes and can hold session/new
// before its response, exposing the cancellation gap without timing a prompt.
func TestAgentACPLifecycleProcess(t *testing.T) {
	scenario := os.Getenv("FIVELEAF_ACP_LIFECYCLE")
	if scenario == "" {
		return
	}
	reconnecting := strings.HasPrefix(scenario, "codex-reconnect-")
	retryingSetup := scenario == "retry-initialize" || scenario == "retry-session"
	logEvent := func(event lifecycleEvent) {
		event.PID = os.Getpid()
		raw, _ := json.Marshal(event)
		file, err := os.OpenFile(os.Getenv("FIVELEAF_ACP_LIFECYCLE_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			os.Exit(2)
		}
		_, err = file.Write(append(raw, '\n'))
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			os.Exit(2)
		}
	}
	waitForGate := func() {
		deadline := time.Now().Add(25 * time.Second)
		for {
			if _, err := os.Stat(os.Getenv("FIVELEAF_ACP_LIFECYCLE_GATE")); err == nil {
				return
			}
			if time.Now().After(deadline) {
				os.Exit(3)
			}
			time.Sleep(time.Millisecond)
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	respond := func(id json.RawMessage, result any) {
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
			os.Exit(2)
		}
	}
	setupError := func(id json.RawMessage) {
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{
			"code": -32603, "message": "Temporary setup failure",
		}}); err != nil {
			os.Exit(2)
		}
	}
	update := func(session string, fields map[string]any) {
		encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": session, "update": fields}})
	}
	textChunk := func(session, kind, id, text string) {
		update(session, map[string]any{"sessionUpdate": kind, "messageId": id, "content": map[string]string{"type": "text", "text": text}})
	}
	chunk := func(session, text string) {
		textChunk(session, "agent_message_chunk", "", text)
	}
	answer := func(id json.RawMessage, session, text string) {
		chunk(session, text)
		stopReason := "end_turn"
		switch scenario {
		case "cancelled", "refusal", "max_tokens", "max_turn_requests":
			stopReason = scenario
		}
		respond(id, map[string]string{"stopReason": stopReason})
	}
	count, root := 0, ""
	initializeAttempts := 0
	modes := map[string]string{}
	var pendingPrompt json.RawMessage
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage
			Method string
			Params struct {
				Cwd, SessionID, ModeID string
				Prompt                 []struct{ Text string }
			}
			Error json.RawMessage
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		if request.Method == "" && string(request.ID) == `"obsolete-write"` {
			logEvent(lifecycleEvent{Kind: "obsolete-write", Session: "lifecycle-1", Accepted: len(request.Error) == 0 || string(request.Error) == "null"})
			answer(pendingPrompt, "lifecycle-2", "second reply")
			pendingPrompt = nil
			continue
		}
		switch request.Method {
		case "initialize":
			if retryingSetup {
				logEvent(lifecycleEvent{Kind: "initialize"})
				initializeAttempts++
				if scenario == "retry-initialize" && initializeAttempts == 1 {
					setupError(request.ID)
					continue
				}
			}
			capabilities := map[string]any{}
			if reconnecting {
				capabilities["loadSession"] = true
				if scenario != "codex-reconnect-load" {
					capabilities["sessionCapabilities"] = map[string]any{"resume": map[string]any{}}
				}
			}
			result := map[string]any{"protocolVersion": 1, "agentCapabilities": capabilities, "authMethods": []any{}}
			if strings.HasPrefix(scenario, "codex-") {
				result["agentInfo"] = map[string]string{"name": "@agentclientprotocol/codex-acp", "version": "2.7.0"}
			}
			respond(request.ID, result)
		case "session/new":
			count++
			root = request.Params.Cwd
			session := fmt.Sprintf("lifecycle-%d", count)
			if reconnecting {
				session = fmt.Sprintf("lifecycle-%d-%d", os.Getpid(), count)
			}
			logEvent(lifecycleEvent{Kind: "new", Session: session})
			if scenario == "retry-session" && count == 1 {
				setupError(request.ID)
				continue
			}
			modes[session] = "ask"
			if (scenario == "hold-first-new" && count == 1) || (scenario == "hold-new" && count == 2) {
				waitForGate()
			}
			available := []map[string]string{{"id": "ask", "name": "Ask"}, {"id": "plan", "name": "Plan"}}
			if scenario == "missing-mode" && count == 2 {
				available = available[:1]
			}
			respond(request.ID, map[string]any{"sessionId": session, "modes": map[string]any{"currentModeId": "ask", "availableModes": available}})
		case "session/load", "session/resume":
			session := request.Params.SessionID
			logEvent(lifecycleEvent{Kind: strings.TrimPrefix(request.Method, "session/"), Session: session})
			if scenario == "codex-reconnect-missing" {
				if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{
					"code": -32002, "message": "Session not found", "data": map[string]string{"sessionId": session},
				}}); err != nil {
					os.Exit(2)
				}
				continue
			}
			modes[session] = "ask"
			if request.Method == "session/load" {
				textChunk(session, "user_message_chunk", "old-question", "first question")
				textChunk(session, "agent_message_chunk", "old-reply", "replayed reply")
			}
			respond(request.ID, map[string]any{})
		case "session/set_mode":
			modes[request.Params.SessionID] = request.Params.ModeID
			logEvent(lifecycleEvent{Kind: "mode", Session: request.Params.SessionID, Mode: request.Params.ModeID})
			respond(request.ID, map[string]any{})
		case "session/prompt":
			session := request.Params.SessionID
			event := lifecycleEvent{Kind: "prompt", Session: session, Mode: modes[session]}
			if blocks := request.Params.Prompt; len(blocks) > 0 {
				event.Text = blocks[len(blocks)-1].Text
				if len(blocks) > 1 {
					event.Context = blocks[0].Text
				}
			}
			logEvent(event)
			if reconnecting && event.Text == "drop adapter" {
				textChunk(session, "agent_message_chunk", "partial", "reply before exit")
				waitForGate()
				os.Exit(1)
			}
			if (scenario == "codex-stop-ack" || scenario == "codex-timeout-ack") && event.Text == "interrupt this question" {
				pendingPrompt = slices.Clone(request.ID)
				textChunk(session, "agent_message_chunk", "partial", "reply before cancellation")
				continue
			}
			if scenario == "codex-completed-tool-details" {
				textChunk(session, "agent_thought_chunk", "reasoning", "Compare both results.\n\nThe second result confirms the first.")
				update(session, map[string]any{
					"sessionUpdate": "tool_call", "toolCallId": "command-1", "title": "Run fixture", "kind": "execute", "status": "in_progress",
					"rawInput": map[string]any{"command": "printf fixture"}, "content": []any{map[string]any{"type": "terminal", "terminalId": "command-1"}},
				})
				for _, data := range []string{"command result: ", "检查通过\n"} {
					update(session, map[string]any{
						"sessionUpdate": "tool_call_update", "toolCallId": "command-1",
						"_meta": map[string]any{"terminal_output_delta": map[string]any{"terminal_id": "command-1", "data": data}},
					})
				}
				update(session, map[string]any{
					"sessionUpdate": "tool_call_update", "toolCallId": "command-1", "status": "completed", "rawOutput": map[string]int{"exit_code": 0},
				})
				update(session, map[string]any{
					"sessionUpdate": "tool_call", "toolCallId": "lookup-1", "title": "mcp.fixture.lookup", "kind": "execute", "status": "in_progress",
					"rawInput": map[string]any{"server": "fixture", "tool": "lookup", "arguments": map[string]string{"query": "fixture request", "ignored": "not a displayed parameter"}},
				})
				update(session, map[string]any{
					"sessionUpdate": "tool_call_update", "toolCallId": "lookup-1", "_meta": map[string]any{"mcp_output_delta": map[string]any{"data": "waiting for lookup"}},
				})
				update(session, map[string]any{
					"sessionUpdate": "tool_call_update", "toolCallId": "lookup-1", "status": "completed",
					"rawOutput": map[string]any{"result": map[string]any{"content": []any{map[string]string{"type": "text", "text": "MCP lookup returned a match"}}}, "error": nil},
				})
				textChunk(session, "agent_thought_chunk", "reasoning-2", "Check the final result.\n\nThe lookup is consistent with the command output.")
				textChunk(session, "agent_message_chunk", "final", strings.Repeat("The fixture completed.\n\n", 45)+"End of the answer.")
				respond(request.ID, map[string]string{"stopReason": "end_turn"})
				continue
			}
			if strings.HasPrefix(scenario, "codex-output-") {
				textChunk(session, "agent_thought_chunk", "placeholder", "\n\n")
				textChunk(session, "agent_message_chunk", "progress-1", "Checking the service.")
				textChunk(session, "agent_message_chunk", "progress-2", "Found the request.")
				textChunk(session, "agent_message_chunk", "progress-1", " More details.")
				textChunk(session, "agent_thought_chunk", "reasoning", "Comparing two results.")
				textChunk(session, "agent_thought_chunk", "reasoning", "\n\n")
				textChunk(session, "agent_thought_chunk", "reasoning", "One result matches.")
				update(session, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "lookup", "title": "Read request logs", "kind": "search", "status": "in_progress"})
				if scenario == "codex-output-tool-update-tail" {
					textChunk(session, "agent_message_chunk", "progress-3", "The search is still running.")
				}
				update(session, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "lookup", "status": "completed"})
				if !strings.HasPrefix(scenario, "codex-output-tool-") {
					textChunk(session, "agent_message_chunk", "answer", "The routing label caused the failure.")
					chunk(session, "Additional adapter output.")
				}
				stopReason := "end_turn"
				if scenario == "codex-output-incomplete" {
					stopReason = "max_tokens"
				}
				respond(request.ID, map[string]string{"stopReason": stopReason})
				continue
			}
			if scenario == "slow-stream" || scenario == "stalled-after-progress" {
				for range 4 {
					chunk(session, "working ")
					time.Sleep(350 * time.Millisecond)
				}
				if scenario == "stalled-after-progress" {
					continue
				}
			}
			if scenario == "quiet-reply" {
				respond(request.ID, map[string]string{"stopReason": "end_turn"})
				continue
			}
			if scenario == "exit-before-reply" {
				os.Exit(1)
			}
			if scenario == "obsolete-write" && request.Params.SessionID == "lifecycle-2" {
				pendingPrompt = slices.Clone(request.ID)
				encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": "obsolete-write", "method": "fs/write_text_file", "params": map[string]any{
					"sessionId": "lifecycle-1", "path": filepath.Join(root, "lifecycle-target.txt"), "content": "obsolete session edit\n",
				}})
				continue
			}
			answer(request.ID, request.Params.SessionID, "reply in "+modes[request.Params.SessionID])
		case "session/cancel":
			logEvent(lifecycleEvent{Kind: "cancel", Session: request.Params.SessionID})
			if pendingPrompt != nil && (scenario == "codex-stop-ack" || scenario == "codex-timeout-ack") {
				if scenario == "codex-stop-ack" {
					waitForGate()
				}
				respond(pendingPrompt, map[string]string{"stopReason": "cancelled"})
				pendingPrompt = nil
			}
		default:
			if len(request.ID) > 0 {
				respond(request.ID, map[string]any{})
			}
		}
	}
	os.Exit(0)
}

func lifecycleHarness(t *testing.T, scenario string) (*harness, string, string) {
	t.Helper()
	hs := newHarness(t, true)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	logPath, gatePath := filepath.Join(dir, "requests.jsonl"), filepath.Join(dir, "release-new-session")
	hs.a.settings.AgentServers = map[string]settings.AgentServer{"lifecycle": {
		Name: "Lifecycle Agent", Command: executable, Args: []string{"-test.run=^TestAgentACPLifecycleProcess$"}, TimeoutSeconds: 30,
		Env: map[string]string{"FIVELEAF_ACP_LIFECYCLE": scenario, "FIVELEAF_ACP_LIFECYCLE_LOG": logPath, "FIVELEAF_ACP_LIFECYCLE_GATE": gatePath},
	}}
	hs.a.selectAgent("lifecycle")
	return hs, logPath, gatePath
}

func lifecycleEvents(t *testing.T, path string) []lifecycleEvent {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	var events []lifecycleEvent
	// A concurrent append can leave the last record temporarily incomplete.
	for _, line := range lines[:len(lines)-1] {
		var event lifecycleEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func lifecycleHas(events []lifecycleEvent, kind, session string) bool {
	return slices.ContainsFunc(events, func(e lifecycleEvent) bool { return e.Kind == kind && e.Session == session })
}

func lifecycleSend(t *testing.T, hs *harness, text string) {
	t.Helper()
	hs.a.agent.draft = text
	hs.a.sendAgent()
	hs.untilLong("lifecycle reply", func() bool { return !hs.a.agent.busy && !hs.a.agent.connecting })
}

func releaseLifecycleGate(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAgentACPSetupRetryKeepsAdapter(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		events []string
	}{
		{name: "retry-initialize", events: []string{"initialize", "initialize", "new", "prompt"}},
		{name: "retry-session", events: []string{"initialize", "new", "new", "prompt"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			hs, logPath, _ := lifecycleHarness(t, scenario.name)
			app, chat := hs.a, hs.a.agent
			const question = "Retry the draft on the same adapter"
			lifecycleSend(t, hs, question)
			client := chat.client
			if client == nil || chat.sessionID != "" || chat.draft != question || len(chat.messages) != 0 ||
				chat.lastTask != agentTaskFailed || !strings.Contains(chat.err, "Temporary setup failure") {
				t.Fatalf("transient setup failure changed the draft or connection: draft=%q messages=%d state=%v error=%q", chat.draft, len(chat.messages), chat.lastTask, chat.err)
			}
			select {
			case <-client.Done():
				t.Fatal("a temporary setup error closed the adapter")
			default:
			}
			app.sendAgent()
			hs.untilLong("reply after setup retry", func() bool { return !chat.requestInFlight() })
			if chat.client != client || chat.sessionID == "" || chat.draft != "" || chat.err != "" || chat.lastTask != agentTaskCompleted {
				t.Fatalf("Send did not recover on the original adapter: draft=%q session=%q state=%v error=%q", chat.draft, chat.sessionID, chat.lastTask, chat.err)
			}
			events := lifecycleEvents(t, logPath)
			if len(events) != len(scenario.events) {
				t.Fatalf("unexpected requests during setup retry: %+v", events)
			}
			for index, event := range events {
				if event.Kind != scenario.events[index] || event.PID != events[0].PID {
					t.Fatalf("setup retry changed processes or duplicated a prompt: %+v", events)
				}
			}
			prompt := events[len(events)-1]
			if prompt.Session != chat.sessionID || !strings.Contains(prompt.Text, question) || len(chat.messages) != 2 ||
				chat.messages[0].role != "user" || chat.messages[0].text != question || chat.messages[0].failure != nil {
				t.Fatalf("only one restored draft should reach the transcript and adapter: messages=%v events=%+v", chat.messages, events)
			}
		})
	}
}

func TestAgentACPStopKeepsCodexSession(t *testing.T) {
	hs, logPath, gatePath := lifecycleHarness(t, "codex-stop-ack")
	app, chat := hs.a, hs.a.agent
	server := app.settings.AgentServers["lifecycle"]
	server.TimeoutSeconds = 0
	app.settings.AgentServers["lifecycle"] = server
	chat.systemPrompt = "Keep these instructions"
	chat.draft = "interrupt this question"
	app.sendAgent()
	hs.untilLong("partial first reply", func() bool {
		return len(chat.messages) == 2 && chat.messages[1].text == "reply before cancellation"
	})
	client, session := chat.client, chat.sessionID
	app.stopAgent()
	if !chat.busy || !chat.stopping || chat.client != client {
		t.Fatalf("expected Stop to wait for the original reply: busy=%v stopping=%v client=%p", chat.busy, chat.stopping, chat.client)
	}
	hs.untilLong("cancel notification", func() bool { return lifecycleHas(lifecycleEvents(t, logPath), "cancel", session) })
	chat.draft = "next question"
	app.sendAgent()
	if chat.draft != "next question" || chat.pendingPrompt != "interrupt this question" || !chat.stopping {
		t.Fatal("a new send displaced the stopping turn or its draft")
	}
	releaseLifecycleGate(t, gatePath)
	hs.untilLong("original prompt cancellation", func() bool { return !chat.busy })
	if chat.client != client || chat.sessionID != session || chat.stopping || chat.draft != "next question" || chat.lastTask != agentTaskIdle {
		t.Fatalf("after Stop: session=%q stopping=%v draft=%q state=%v err=%q", chat.sessionID, chat.stopping, chat.draft, chat.lastTask, chat.err)
	}
	if failure := chat.messages[0].failure; failure == nil || failure.draft.text != "interrupt this question" {
		t.Fatalf("original cancelled question: %+v", failure)
	}
	if events := lifecycleEvents(t, logPath); len(events) != 3 {
		t.Fatalf("another request was sent before Stop completed: %+v", events)
	}
	lifecycleSend(t, hs, "next question")
	checkLifecycleCancelledThenReplied(t, lifecycleEvents(t, logPath), session)
	if chat.client != client || chat.sessionID != session || chat.err != "" || chat.lastTask != agentTaskCompleted {
		t.Fatalf("following reply: session=%q state=%v err=%q", chat.sessionID, chat.lastTask, chat.err)
	}
}

func TestAgentACPIdleTimeoutKeepsAcknowledgingAdapter(t *testing.T) {
	hs, logPath, _ := lifecycleHarness(t, "codex-timeout-ack")
	app, chat := hs.a, hs.a.agent
	server := app.settings.AgentServers["lifecycle"]
	server.TimeoutSeconds = 1
	app.settings.AgentServers["lifecycle"] = server
	app.connectAgent()
	hs.untilLong("initial connection", func() bool { return !chat.connecting })
	client, session := chat.client, chat.sessionID
	lifecycleSend(t, hs, "interrupt this question")
	if chat.client != client || chat.sessionID != session || chat.lastTask != agentTaskFailed || len(chat.messages) != 2 {
		t.Fatalf("after idle timeout: session=%q state=%v count=%d err=%q", chat.sessionID, chat.lastTask, len(chat.messages), chat.err)
	}
	if failure := chat.messages[0].failure; failure == nil || failure.reason != locales.AgentReplyIdleTimeout(1) || failure.draft.text != "interrupt this question" {
		t.Fatalf("idle timeout did not preserve original question: %+v", failure)
	}
	lifecycleSend(t, hs, "next question")
	checkLifecycleCancelledThenReplied(t, lifecycleEvents(t, logPath), session)
	if chat.client != client || chat.err != "" || chat.lastTask != agentTaskCompleted {
		t.Fatalf("following reply: state=%v err=%q", chat.lastTask, chat.err)
	}
}

func checkLifecycleCancelledThenReplied(t *testing.T, events []lifecycleEvent, session string) {
	t.Helper()
	want := []string{"new", "prompt", "cancel", "prompt"}
	if len(events) != len(want) {
		t.Fatalf("unexpected events: %+v", events)
	}
	for index, event := range events {
		if event.Kind != want[index] || event.Session != session || event.PID != events[0].PID {
			t.Fatalf("expected one process and session through cancellation: %+v", events)
		}
	}
	if events[1].Text != "interrupt this question" || events[3].Text != "next question" || events[3].Context != "" {
		t.Fatalf("unexpected wire prompts: %+v", events)
	}
}

func TestAgentACPCodexReattachesAfterAdapterExit(t *testing.T) {
	for _, scenario := range []struct {
		name, restoreMethod string
		firstTurn, fresh    bool
	}{
		{name: "resume", restoreMethod: "resume"},
		{name: "load", restoreMethod: "load"},
		{name: "changed-profile", fresh: true},
		{name: "missing", restoreMethod: "resume", fresh: true},
		{name: "first-turn", restoreMethod: "resume", firstTurn: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			hs, logPath, gatePath := lifecycleHarness(t, "codex-reconnect-"+scenario.name)
			app, chat := hs.a, hs.a.agent
			server := app.settings.AgentServers["lifecycle"]
			server.Env["CODEX_HOME"] = t.TempDir()
			app.settings.AgentServers["lifecycle"] = server
			chat.systemPrompt = "Keep these instructions"
			var submitted []string
			expectedEvents := []string{"new"}
			if !scenario.firstTurn {
				lifecycleSend(t, hs, "first question")
				if chat.err != "" || chat.sessionID == "" || chat.lastTask != agentTaskCompleted {
					t.Fatalf("initial reply: session=%q state=%v err=%q", chat.sessionID, chat.lastTask, chat.err)
				}
				submitted = append(submitted, "first question")
				expectedEvents = append(expectedEvents, "prompt")
			}
			chat.draft = "drop adapter"
			app.sendAgent()
			hs.untilLong("partial reply before adapter exit", func() bool {
				return slices.ContainsFunc(chat.messages, func(message *agentMessage) bool { return message.text == "reply before exit" })
			})
			session := chat.sessionID
			submitted = append(submitted, "drop adapter")
			expectedEvents = append(expectedEvents, "prompt")
			replacement := len(expectedEvents)
			if scenario.restoreMethod != "" {
				expectedEvents = append(expectedEvents, scenario.restoreMethod)
			}
			if scenario.fresh {
				expectedEvents = append(expectedEvents, "new")
			}
			chat.draft = "next question"
			releaseLifecycleGate(t, gatePath)
			hs.untilLong("disconnected adapter", func() bool { return chat.client == nil && !chat.busy })
			if chat.lastTask != agentTaskFailed || chat.draft != "next question" || chat.err == "" {
				t.Fatalf("after exit: state=%v draft=%q err=%q", chat.lastTask, chat.draft, chat.err)
			}
			if !slices.ContainsFunc(chat.messages, func(message *agentMessage) bool {
				return message.role == "user" && message.text == "drop adapter" && message.failure != nil && message.failure.draft.text == "drop adapter"
			}) {
				t.Fatal("the committed question cannot be restored from its failed turn")
			}
			if scenario.name == "changed-profile" {
				server = cloneAgentServer(server)
				server.Env["CODEX_HOME"] = t.TempDir()
				app.settings.AgentServers["lifecycle"] = server
			}
			if scenario.restoreMethod == "load" {
				app.connectAgent()
				hs.untilLong("explicit reconnection", func() bool { return !chat.connecting })
				if chat.sessionID != session || chat.draft != "next question" || len(lifecycleEvents(t, logPath)) != len(expectedEvents) {
					t.Fatal("explicit reconnect submitted a draft or replayed a prompt")
				}
			}
			app.sendAgent()
			hs.untilLong("next reply", func() bool { return !chat.busy && !chat.connecting })
			submitted = append(submitted, "next question")
			expectedEvents = append(expectedEvents, "prompt")
			events := lifecycleEvents(t, logPath)
			if len(events) != len(expectedEvents) || events[0].PID == events[replacement].PID || chat.err != "" || chat.lastTask != agentTaskCompleted {
				t.Fatalf("unexpected reconnect: state=%v err=%q events=%+v", chat.lastTask, chat.err, events)
			}
			var prompts []string
			for index, event := range events {
				if event.Kind != expectedEvents[index] || (index < replacement && event.PID != events[0].PID) ||
					(index >= replacement && event.PID != events[replacement].PID) {
					t.Fatalf("expected %v on two adapter processes: %+v", expectedEvents, events)
				}
				if event.Kind == "prompt" {
					prompts = append(prompts, event.Text)
				}
			}
			if !slices.Equal(prompts, submitted) || (scenario.restoreMethod != "" && events[replacement].Session != session) {
				t.Fatalf("reconnection lost the session ID or duplicated a submission: %+v", events)
			}
			followUp := events[len(events)-1]
			if scenario.fresh {
				if chat.sessionID == session || !strings.Contains(followUp.Context, chat.systemPrompt) || strings.Contains(followUp.Context, "drop adapter") {
					t.Fatalf("the fallback did not seed a fresh session from completed turns: %+v", events)
				}
			} else if followUp.Session != session || chat.sessionID != session || followUp.Context != "" {
				t.Fatalf("the new adapter did not attach once to the native session: %+v", events)
			}
			var questions []string
			var partial int
			for _, message := range chat.messages {
				if message.role == "user" {
					questions = append(questions, message.text)
				}
				if message.text == "reply before exit" {
					partial++
				}
				if strings.Contains(message.text, "replayed reply") {
					t.Fatal("the adapter replay was added to the transcript")
				}
			}
			if !slices.Equal(questions, submitted) || partial != 1 || chat.draft != "" {
				t.Fatalf("transcript after reconnect: questions=%q partial replies=%d draft=%q", questions, partial, chat.draft)
			}
		})
	}
}

func TestAgentACPCodexKeepsTheNativeThread(t *testing.T) {
	hs, logPath, _ := lifecycleHarness(t, "codex-native")
	s := hs.a.agent
	s.systemPrompt = "Keep these instructions"
	lifecycleSend(t, hs, "first question")
	lifecycleSend(t, hs, "second question")
	if s.err != "" || s.draft != "" || s.sessionID != "lifecycle-1" {
		t.Fatalf("second turn did not finish in the same session: session=%q draft=%q err=%q", s.sessionID, s.draft, s.err)
	}
	s.systemPrompt = "Use the updated instructions"
	lifecycleSend(t, hs, "third question")
	var prompts []lifecycleEvent
	for _, event := range lifecycleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts = append(prompts, event)
		}
	}
	if len(prompts) != 3 || !strings.Contains(prompts[0].Context, "Keep these instructions") || prompts[0].Text != "first question" ||
		prompts[0].Session != "lifecycle-1" || prompts[1].Session != prompts[0].Session || prompts[1].Text != "second question" || prompts[1].Context != "" ||
		prompts[2].Session != "lifecycle-2" || prompts[2].Text != "third question" || !strings.Contains(prompts[2].Context, s.systemPrompt) || strings.Contains(prompts[2].Context, "Keep these instructions") {
		t.Fatalf("native and re-seeded prompts: %+v", prompts)
	}
}

func TestAgentACPCodexSendsTheOriginalUserText(t *testing.T) {
	for _, scenario := range []struct {
		question string
		framed   bool
	}{
		{question: "检查日志\n"},
		{question: "[user]\n这是原文\n\n", framed: true},
		{question: "<html>literal</html>", framed: true},
		{question: "# AGENTS.md instructions\nReview these instructions", framed: true},
		{question: "# Files mentioned by the user:\n## My request:\nReview this template", framed: true},
	} {
		hs, logPath, _ := lifecycleHarness(t, "codex-native")
		lifecycleSend(t, hs, scenario.question)
		events := lifecycleEvents(t, logPath)
		if len(events) != 2 || events[1].Kind != "prompt" || events[1].Text != scenario.question {
			t.Fatalf("Codex should receive the exact user text: %+v", events)
		}
		if (events[1].Context != "") != scenario.framed {
			t.Fatalf("Codex prompt did not distinguish literal role text: %+v", events[1])
		}
	}
}

func TestAgentACPCodexSeparatesTranscriptOutput(t *testing.T) {
	for _, scenario := range []string{"codex-output-complete", "codex-output-incomplete", "codex-output-tool-tail", "codex-output-tool-update-tail"} {
		t.Run(scenario, func(t *testing.T) {
			hs, _, _ := lifecycleHarness(t, scenario)
			lifecycleSend(t, hs, "Check the service")
			var got []string
			for _, msg := range hs.a.agent.messages {
				got = append(got, msg.role+": "+msg.text)
			}
			want := []string{"user: Check the service", "commentary: Checking the service. More details.",
				"commentary: Found the request.", "thought: Comparing two results.\n\nOne result matches.", "tool: "}
			if scenario == "codex-output-tool-update-tail" {
				want = append(want, "commentary: The search is still running.")
			} else if scenario != "codex-output-tool-tail" {
				answer := "assistant"
				if scenario == "codex-output-incomplete" {
					answer = "commentary"
				}
				want = append(want, answer+": The routing label caused the failure.", "commentary: Additional adapter output.")
			}
			if !slices.Equal(got, want) {
				t.Fatalf("transcript roles and content = %q, want %q", got, want)
			}
		})
	}
}

func TestAgentACPLiveProgressExtendsReplyTimeout(t *testing.T) {
	for _, scenario := range []string{"slow-stream", "stalled-after-progress"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath, _ := lifecycleHarness(t, scenario)
			a, s := hs.a, hs.a.agent
			server := a.settings.AgentServers["lifecycle"]
			server.TimeoutSeconds = 1
			a.settings.AgentServers["lifecycle"] = server
			a.openAgentPage()
			hs.untilLong("opened adapter session", func() bool { return s.sessionID != "" && !s.connecting })
			client, session := s.client, s.sessionID
			s.draft = "first question"
			a.sendAgent()
			hs.untilLong("initial streamed reply", func() bool {
				return slices.ContainsFunc(s.messages, func(msg *agentMessage) bool { return msg.role == "assistant" })
			})
			s.draft = "independent draft"
			if scenario == "slow-stream" {
				hs.untilLong("prompt result", func() bool { return !s.busy })
			} else {
				hs.untilLong("idle cancellation request", func() bool {
					return s.stopping && lifecycleHas(lifecycleEvents(t, logPath), "cancel", session)
				})
			}
			if s.draft != "independent draft" || len(s.messages) != 2 || s.messages[0].text != "first question" ||
				!strings.HasPrefix(s.messages[1].text, strings.Repeat("working ", 4)) {
				t.Fatalf("partial reply and draft: %q, %+v", s.draft, s.messages)
			}
			if scenario == "slow-stream" {
				if s.err != "" || s.lastTask != agentTaskCompleted || s.messages[0].failure != nil {
					t.Fatalf("live stream timed out: %q", s.err)
				}
				return
			}
			if s.client != client || s.sessionID != session || !s.busy || !s.stopping {
				t.Fatalf("stalled adapter did not wait for cancellation: busy=%v stopping=%v session=%q", s.busy, s.stopping, s.sessionID)
			}
			select {
			case <-client.Done():
				t.Fatal("the unresponsive adapter exited before Reconnect")
			default:
			}
			a.sendAgent()
			if s.draft != "independent draft" {
				t.Fatalf("sending during cancellation consumed the draft: %q", s.draft)
			}
			hs.settle()
			if err := hs.tt.Click(locales.AgentReconnect()); err != nil {
				t.Fatal(err)
			}
			hs.untilLong("reconnected adapter", func() bool {
				return s.client != nil && s.client != client && s.sessionID != "" && !s.requestInFlight()
			})
			select {
			case <-client.Done():
			default:
				t.Fatal("Reconnect opened a replacement before the old adapter exited")
			}
			events := lifecycleEvents(t, logPath)
			if len(events) != 4 || events[0].Kind != "new" || events[1].Kind != "prompt" || events[2].Kind != "cancel" ||
				events[3].Kind != "new" || events[3].PID == events[0].PID || s.stopping || s.draft != "independent draft" {
				t.Fatalf("Reconnect changed the draft or submitted a prompt: draft=%q events=%+v", s.draft, events)
			}
			if failure := s.messages[0].failure; failure == nil || failure.draft.text != "first question" {
				t.Fatalf("the interrupted prompt is no longer recoverable: %+v", failure)
			}
			if replay := chatContext(s); len(replay) != 0 {
				t.Fatalf("interrupted turn would be replayed: %+v", replay)
			}
		})
	}
}

func TestAgentACPStopWhileCreatingSession(t *testing.T) {
	hs, logPath, gatePath := lifecycleHarness(t, "hold-new")
	a, s := hs.a, hs.a.agent
	lifecycleSend(t, hs, "first prompt")
	if s.err != "" {
		t.Fatal(s.err)
	}
	s.draft = "must never reach the agent"
	a.sendAgent()
	hs.untilLong("second session request", func() bool { return lifecycleHas(lifecycleEvents(t, logPath), "new", "lifecycle-2") })
	client, generation := s.client, s.generation
	a.stopAgent()
	if !s.busy || !s.stopping || s.client != client || s.generation != generation || s.draft != "must never reach the agent" || s.pendingPrompt != "" {
		t.Fatalf("Stop interrupted setup or did not restore the draft: busy=%v stopping=%v draft=%q", s.busy, s.stopping, s.draft)
	}
	releaseLifecycleGate(t, gatePath)
	hs.untilLong("session setup after Stop", func() bool { return !s.requestInFlight() })
	if lifecycleHas(lifecycleEvents(t, logPath), "prompt", "lifecycle-2") {
		t.Fatal("a new prompt was sent after Stop during session creation")
	}
	if s.client != client || s.sessionID != "lifecycle-2" || s.stopping || s.draft != "must never reach the agent" || len(s.permissions) != 0 {
		t.Fatalf("completed setup did not keep the stopped chat ready: session=%q stopping=%v draft=%q", s.sessionID, s.stopping, s.draft)
	}
	lifecycleSend(t, hs, "new question after Stop")
	events := lifecycleEvents(t, logPath)
	if len(events) != 5 {
		t.Fatalf("unexpected requests during stopped setup: %+v", events)
	}
	followUp := events[4]
	if events[3].Kind != "mode" || followUp.Kind != "prompt" || followUp.Mode != "ask" ||
		followUp.Session != "lifecycle-2" || followUp.PID != events[0].PID || !strings.Contains(followUp.Text, "new question after Stop") ||
		strings.Contains(followUp.Text, "must never reach the agent") || s.lastTask != agentTaskCompleted {
		t.Fatalf("the next send did not use the session kept after Stop: state=%v events=%+v", s.lastTask, events)
	}
}

func TestAgentACPModeAcrossNewSession(t *testing.T) {
	for _, scenario := range []string{"modes", "missing-mode"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath, _ := lifecycleHarness(t, scenario)
			a, s := hs.a, hs.a.agent
			a.connectAgent()
			hs.untilLong("initial session", func() bool { return !s.connecting })
			if s.err != "" || s.client == nil {
				t.Fatalf("connect: %s", s.err)
			}
			a.setAgentMode("plan")
			hs.untilLong("selected plan mode", func() bool { return !s.configuring })
			if s.err != "" || s.mode != "plan" {
				t.Fatalf("select mode: %s, %q", s.err, s.mode)
			}
			lifecycleSend(t, hs, "first plan prompt")
			if s.err != "" {
				t.Fatal(s.err)
			}
			lifecycleSend(t, hs, "second plan prompt")
			events := lifecycleEvents(t, logPath)
			if scenario == "missing-mode" {
				if s.err == "" || lifecycleHas(events, "prompt", "lifecycle-2") {
					t.Fatalf("unsupported selected mode silently ran a default-mode prompt: error=%q events=%+v", s.err, events)
				}
				return
			}
			if s.err != "" || s.mode != "plan" || !lifecycleHas(events, "mode", "lifecycle-2") {
				t.Fatalf("new session lost selected mode: error=%q mode=%q events=%+v", s.err, s.mode, events)
			}
			for _, event := range events {
				if event.Kind == "prompt" && event.Mode != "plan" {
					t.Fatalf("prompt used a mode different from the UI selection: %+v", event)
				}
			}
			if !lifecycleHas(events, "prompt", "lifecycle-2") {
				t.Fatal("second prompt never reached the new session")
			}
		})
	}
}

func TestAgentACPThinkingConfirmedAcrossPromptsAndModelChanges(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "thinking")
	a, s := hs.a, hs.a.agent
	a.connectAgent()
	hs.untilLong("thinking session", func() bool { return !s.connecting })
	if s.err != "" || !s.thinkingModes || s.mode != "medium" || len(s.modes) != 7 {
		t.Fatalf("initial thinking = %q, modes=%+v, error=%q", s.mode, s.modes, s.err)
	}
	modelCalls := func() int {
		count := 0
		for _, event := range agentRoleEvents(t, logPath) {
			if event.Kind == "model" {
				count++
			}
		}
		return count
	}
	before := modelCalls()
	a.setChatMode(s, "high")
	hs.untilLong("confirmed high thinking", func() bool { return !s.configuring })
	if s.err != "" || s.mode != "high" || modelCalls() != before {
		t.Fatalf("thinking setter reset model or lost confirmation: mode=%q error=%q", s.mode, s.err)
	}
	for _, text := range []string{"first high prompt", "second high prompt"} {
		lifecycleSend(t, hs, text)
		if s.err != "" || s.mode != "high" {
			t.Fatalf("high thinking not retained: mode=%q error=%q", s.mode, s.err)
		}
	}
	a.setChatModel(s, "model-b")
	hs.untilLong("model-b thinking options", func() bool { return !s.configuring })
	if s.err != "" || s.mode != "low" || len(s.modes) != 3 || s.currentModel != "model-b" {
		t.Fatalf("model-b state not adopted: model=%q mode=%q modes=%+v error=%q", s.currentModel, s.mode, s.modes, s.err)
	}
	a.setChatMode(s, "max")
	hs.untilLong("reject stale max", func() bool { return !s.configuring })
	if s.err == "" || s.mode != "low" {
		t.Fatal("stale max choice replaced confirmed low")
	}
	a.setChatMode(s, "off")
	hs.untilLong("disable thinking", func() bool { return !s.configuring })
	if s.err != "" || s.mode != "off" {
		t.Fatalf("disable thinking: mode=%q error=%q", s.mode, s.err)
	}
	lifecycleSend(t, hs, "off prompt")
	if s.err != "" || s.mode != "off" {
		t.Fatalf("off thinking not retained: mode=%q error=%q", s.mode, s.err)
	}
	var prompts []roleRuntimeEvent
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts = append(prompts, event)
		}
	}
	if len(prompts) != 3 {
		t.Fatalf("prompt count = %d", len(prompts))
	}
	for i, event := range prompts {
		wantModel, wantMode := "model-a", "high"
		if i == 2 {
			wantModel, wantMode = "model-b", "off"
		}
		if event.Model != wantModel || event.Mode != wantMode {
			t.Fatalf("prompt used a different model/thinking from UI: %+v, want %s/%s", event, wantModel, wantMode)
		}
	}
	a.disconnectChat(s)
	if s.thinkingModes || s.mode != "" || len(s.modes) != 0 {
		t.Fatal("disconnect retained stale thinking options")
	}
}

func TestAgentACPThinkingShowsServerClampOrFailure(t *testing.T) {
	for _, scenario := range []string{"thinking-clamp", "thinking-error"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, scenario)
			a, s := hs.a, hs.a.agent
			a.connectAgent()
			hs.untilLong("thinking session", func() bool { return !s.connecting })
			requested, want := "max", "high"
			if scenario == "thinking-error" {
				requested, want = "high", "medium"
			}
			a.setChatMode(s, requested)
			hs.untilLong("thinking refusal", func() bool { return !s.configuring })
			if s.err == "" || s.mode != want || !s.thinkingModes {
				t.Fatalf("unconfirmed thinking shown as selected: mode=%q error=%q", s.mode, s.err)
			}
			lifecycleSend(t, hs, "use confirmed thinking")
			if s.err != "" {
				t.Fatal(s.err)
			}
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind == "prompt" && event.Mode != want {
					t.Fatalf("prompt did not use confirmed thinking: %+v", event)
				}
			}
		})
	}
}

func TestAgentACPThinkingRetainsIndependentGenericMode(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "thinking-mixed")
	a, s := hs.a, hs.a.agent
	a.connectAgent()
	hs.untilLong("mixed selectors", func() bool { return !s.connecting })
	if s.err != "" || !s.thinkingModes || s.mode != "medium" || s.genericMode != "ask" || len(s.genericModes) != 2 {
		t.Fatalf("mixed state: thinking=%q mode=%q options=%+v error=%q", s.mode, s.genericMode, s.genericModes, s.err)
	}
	a.setChatMode(s, "high")
	hs.untilLong("mixed high thinking", func() bool { return !s.configuring })
	if s.err != "" || s.mode != "high" || s.genericMode != "ask" {
		t.Fatalf("thinking changed generic mode: thinking=%q mode=%q error=%q", s.mode, s.genericMode, s.err)
	}
	a.setChatGenericMode(s, "plan")
	hs.untilLong("mixed plan mode", func() bool { return !s.configuring })
	if s.err != "" || s.mode != "high" || s.genericMode != "plan" || s.currentModel != "model-a" {
		t.Fatalf("generic mode reset thinking/model: thinking=%q mode=%q model=%q error=%q", s.mode, s.genericMode, s.currentModel, s.err)
	}
	for _, text := range []string{"first plan high prompt", "second plan high prompt"} {
		lifecycleSend(t, hs, text)
		if s.err != "" || s.mode != "high" || s.genericMode != "plan" {
			t.Fatalf("prompt lost selectors: thinking=%q mode=%q error=%q", s.mode, s.genericMode, s.err)
		}
	}
	count := 0
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			count++
			if event.Mode != "high" || event.GenericMode != "plan" || event.Model != "model-a" {
				t.Fatalf("server prompt lost a selector: %+v", event)
			}
		}
	}
	if count != 2 {
		t.Fatalf("prompt count = %d", count)
	}
}

func TestAgentACPRejectsObsoleteSessionWrite(t *testing.T) {
	hs, logPath, _ := lifecycleHarness(t, "obsolete-write")
	path := hs.path("lifecycle-target.txt")
	if err := os.WriteFile(path, []byte("keep current file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lifecycleSend(t, hs, "first prompt")
	if err := hs.a.agent.err; err != "" {
		t.Fatal(err)
	}
	lifecycleSend(t, hs, "second prompt")
	if err := hs.a.agent.err; err != "" {
		t.Fatal(err)
	}
	events := lifecycleEvents(t, logPath)
	at := slices.IndexFunc(events, func(event lifecycleEvent) bool { return event.Kind == "obsolete-write" })
	if at < 0 || events[at].Accepted {
		t.Fatalf("obsolete session file callback was accepted: %+v", events)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "keep current file\n" {
		t.Fatalf("obsolete session changed the disk: %q, %v", got, err)
	}
	if len(hs.a.agent.tools) != 0 || len(hs.a.agentWrites) != 0 {
		t.Fatal("rejected callback created a new-session review tool or retained its write lock")
	}
}

func TestAgentACPPreparedWriteCannotCrossSessions(t *testing.T) {
	hs := newHarness(t, true)
	a, s := hs.a, hs.a.agent
	path := hs.path("lifecycle-target.txt")
	if err := os.WriteFile(path, []byte("keep prepared file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	hs.settle()
	s.sessionID, s.busy = "old-session", true
	handler := &agentHandler{app: a, chat: s, generation: s.generation, root: a.root}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- handler.WriteTextFile(ctx, acp.WriteRequest{SessionId: "old-session", Path: path, Content: "must not commit\n"})
	}()
	// Run only the preparation callback. Draining the whole queue can also run
	// its eventual commit before the test changes the active session.
	for deadline := time.Now().Add(10 * time.Second); !a.agentWrites[path]; {
		if time.Now().After(deadline) {
			t.Fatal("write did not acquire its preparation lock")
		}
		var next func()
		a.mu.Lock()
		if len(a.queue) > 0 {
			next, a.queue = a.queue[0], a.queue[1:]
		}
		a.mu.Unlock()
		if next != nil {
			next()
		} else {
			time.Sleep(time.Millisecond)
		}
	}
	s.sessionID = "new-session"
	var writeErr error
	hs.untilLong("obsolete prepared write rejection", func() bool {
		select {
		case writeErr = <-done:
			return true
		default:
			return false
		}
	})
	if writeErr == nil {
		t.Fatal("prepared write committed after its session was replaced")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "keep prepared file\n" {
		t.Fatalf("prepared obsolete write changed the disk: %q, %v", got, err)
	}
	if len(s.tools) != 0 || len(a.agentWrites) != 0 {
		t.Fatal("obsolete prepared write left a review tool or write lock")
	}
}

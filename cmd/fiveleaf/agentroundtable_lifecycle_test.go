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
)

type lifecycleEvent struct {
	Kind, Session, Mode string
	Accepted            bool
}

// This process fixture has independent session modes and can hold session/new
// before its response, exposing the cancellation gap without timing a prompt.
func TestAgentACPLifecycleProcess(t *testing.T) {
	scenario := os.Getenv("FIVELEAF_ACP_LIFECYCLE")
	if scenario == "" {
		return
	}
	logEvent := func(event lifecycleEvent) {
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
	encoder := json.NewEncoder(os.Stdout)
	respond := func(id json.RawMessage, result any) {
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
			os.Exit(2)
		}
	}
	answer := func(id json.RawMessage, session, text string) {
		encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": session, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}},
		}})
		stopReason := "end_turn"
		switch scenario {
		case "cancelled", "refusal", "max_tokens", "max_turn_requests":
			stopReason = scenario
		}
		respond(id, map[string]string{"stopReason": stopReason})
	}
	count, root := 0, ""
	modes := map[string]string{}
	var pendingPrompt json.RawMessage
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage
			Method string
			Params struct {
				Cwd, SessionID, ModeID string
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
			respond(request.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}})
		case "session/new":
			count++
			root = request.Params.Cwd
			session := fmt.Sprintf("lifecycle-%d", count)
			modes[session] = "ask"
			logEvent(lifecycleEvent{Kind: "new", Session: session})
			if scenario == "hold-new" && count == 2 {
				deadline := time.Now().Add(25 * time.Second)
				for {
					if _, err := os.Stat(os.Getenv("FIVELEAF_ACP_LIFECYCLE_GATE")); err == nil {
						break
					}
					if time.Now().After(deadline) {
						os.Exit(3)
					}
					time.Sleep(time.Millisecond)
				}
			}
			available := []map[string]string{{"id": "ask", "name": "Ask"}, {"id": "plan", "name": "Plan"}}
			if scenario == "missing-mode" && count == 2 {
				available = available[:1]
			}
			respond(request.ID, map[string]any{"sessionId": session, "modes": map[string]any{"currentModeId": "ask", "availableModes": available}})
		case "session/set_mode":
			modes[request.Params.SessionID] = request.Params.ModeID
			logEvent(lifecycleEvent{Kind: "mode", Session: request.Params.SessionID, Mode: request.Params.ModeID})
			respond(request.ID, map[string]any{})
		case "session/prompt":
			logEvent(lifecycleEvent{Kind: "prompt", Session: request.Params.SessionID, Mode: modes[request.Params.SessionID]})
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
	if s.busy || s.connecting || s.client != nil || s.generation == generation {
		t.Fatal("Stop did not immediately retire the in-flight session and its process")
	}
	if err := os.WriteFile(gatePath, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("stopped ACP process", func() bool {
		select {
		case <-client.Done():
			return true
		default:
			return false
		}
	})
	hs.settle()
	if lifecycleHas(lifecycleEvents(t, logPath), "prompt", "lifecycle-2") {
		t.Fatal("a new prompt was sent after Stop during session creation")
	}
	if s.client != nil || s.sessionID != "" || s.busy || len(s.permissions) > 0 {
		t.Fatal("a callback restored the stopped session")
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

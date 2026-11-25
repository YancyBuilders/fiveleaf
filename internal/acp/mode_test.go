package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func startModeClient(t *testing.T, scenario string) (*Client, context.Context, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	c, err := Start(ctx, Config{Command: os.Args[0], Args: []string{"-test.run=^TestACPModeProcess$"},
		Env: map[string]string{"FIVELEAF_ACP_MODE_TEST": scenario}}, t.TempDir(), &testHandler{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return c, ctx, string(session.SessionId)
}

func TestACPModeStateSnapshotsAndSelection(t *testing.T) {
	for _, scenario := range []string{"category", "id", "grouped", "early", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			c, ctx, id := startModeClient(t, scenario)
			if _, ok := c.ModeState("missing"); ok {
				t.Fatal("unknown session advertised modes")
			}
			if err := c.SetThinkingMode(ctx, "missing", "high"); !errors.Is(err, ErrUnknownSession) {
				t.Fatalf("unknown session = %v", err)
			}
			state, ok := c.ModeState(id)
			if !ok || state.Current != "medium" || len(state.Available) != 5 || state.Thinking != (scenario != "legacy") {
				t.Fatalf("initial modes = %+v, %v", state, ok)
			}
			if len(state.GenericAvailable) != 0 {
				t.Fatal("thinking aliases duplicated into generic modes")
			}
			state.Available[0].ID = "mutated"
			state, _ = c.ModeState(id)
			if state.Available[0].ID != "off" {
				t.Fatal("mode snapshot shares mutable choices")
			}
			if err := c.SetThinkingMode(ctx, id, "auto"); err == nil {
				t.Fatal("unadvertised thinking level accepted")
			}
			if err := c.SetThinkingMode(ctx, id, "high"); err != nil {
				t.Fatal(err)
			}
			state, ok = c.ModeState(id)
			if !ok || state.Current != "high" {
				t.Fatalf("confirmed modes = %+v, %v", state, ok)
			}
			if _, err := c.Prompt(ctx, id, []ContentBlock{TextBlock("verify selected thinking")}); err != nil {
				t.Fatal(err)
			}
			c.Close()
			if _, ok := c.ModeState(id); ok {
				t.Fatal("closed connection advertised modes")
			}
		})
	}
}

func TestACPModeAdoptsModelDependentChoices(t *testing.T) {
	c, ctx, id := startModeClient(t, "category")
	if err := c.SetThinkingMode(ctx, id, "max"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetModel(ctx, id, "model-b"); err != nil {
		t.Fatal(err)
	}
	state, ok := c.ModeState(id)
	if !ok || state.Current != "low" || len(state.Available) != 3 || !state.Thinking {
		t.Fatalf("model changed without refreshing thinking = %+v, %v", state, ok)
	}
	if err := c.SetThinkingMode(ctx, id, "max"); err == nil {
		t.Fatal("stale session/new mode list accepted unsupported max")
	}
	if err := c.SetThinkingMode(ctx, id, "high"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, id, []ContentBlock{TextBlock("verify supported mode")}); err != nil {
		t.Fatal(err)
	}
}

func TestACPModeKeepsConfirmedStateOnClampOrFailure(t *testing.T) {
	for _, scenario := range []string{"response-clamp", "notify-clamp", "legacy-clamp", "error"} {
		t.Run(scenario, func(t *testing.T) {
			c, ctx, id := startModeClient(t, scenario)
			if err := c.SetThinkingMode(ctx, id, "max"); err == nil {
				t.Fatal("unconfirmed mode was accepted")
			}
			want := "high"
			if scenario == "error" {
				want = "medium"
			}
			state, _ := c.ModeState(id)
			if state.Current != want {
				t.Fatalf("mode after clamp/failure = %q, want %q", state.Current, want)
			}
		})
	}
}

func TestACPModeKeepsIndependentGenericSelector(t *testing.T) {
	c, ctx, id := startModeClient(t, "mixed")
	state, _ := c.ModeState(id)
	if state.Current != "medium" || state.GenericCurrent != "ask" || len(state.GenericAvailable) != 2 {
		t.Fatalf("independent mode selector lost: %+v", state)
	}
	state.GenericAvailable[0].ID = "mutated"
	state, _ = c.ModeState(id)
	if state.GenericAvailable[0].ID != "ask" {
		t.Fatal("generic mode snapshot shares mutable choices")
	}
	if err := c.SetMode(ctx, id, "high"); err != nil {
		t.Fatal(err)
	}
	state, _ = c.ModeState(id)
	if state.GenericCurrent != "high" || state.Current != "medium" {
		t.Fatalf("generic high changed thinking with the same ID: %+v", state)
	}
	if err := c.SetThinkingMode(ctx, id, "high"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMode(ctx, id, "ask"); err != nil {
		t.Fatal(err)
	}
	state, _ = c.ModeState(id)
	if state.Current != "high" || state.GenericCurrent != "ask" {
		t.Fatalf("thinking and generic state conflated: %+v", state)
	}
	if err := c.SetThinkingMode(ctx, id, "ask"); err == nil {
		t.Fatal("generic mode accepted as a thinking value")
	}
	if _, err := c.Prompt(ctx, id, []ContentBlock{TextBlock("use both selectors")}); err != nil {
		t.Fatal(err)
	}
}

func TestACPModeAcceptsAdvertisedEmptyThinkingDefault(t *testing.T) {
	c, ctx, id := startModeClient(t, "empty")
	state, ok := c.ModeState(id)
	if !ok || !state.Thinking || state.Current != "" || len(state.Available) != 4 ||
		state.Available[0].ID != "" || state.Available[0].Name != "Provider default" {
		t.Fatalf("empty provider default was dropped: %+v, %v", state, ok)
	}
	for _, value := range []string{"high", ""} {
		if err := c.SetThinkingMode(ctx, id, value); err != nil {
			t.Fatalf("selecting %q: %v", value, err)
		}
		state, ok = c.ModeState(id)
		if !ok || state.Current != value {
			t.Fatalf("confirmed thinking = %q, want %q", state.Current, value)
		}
	}
	if _, err := c.Prompt(ctx, id, []ContentBlock{TextBlock("verify provider default")}); err != nil {
		t.Fatal(err)
	}
}

func TestACPModeProcess(t *testing.T) {
	scenario := os.Getenv("FIVELEAF_ACP_MODE_TEST")
	if scenario == "" {
		return
	}
	enc := json.NewEncoder(os.Stdout)
	send := func(value any) {
		if enc.Encode(value) != nil {
			os.Exit(2)
		}
	}
	reply := func(id json.RawMessage, result any) {
		send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	legacy := strings.HasPrefix(scenario, "legacy")
	configID := "effort"
	if scenario == "id" {
		configID = "thought_level"
	} else if scenario == "empty" {
		configID = "reasoning_effort"
	}
	levels := func(model string) []string {
		if scenario == "empty" {
			return []string{"", "low", "high", "max"}
		}
		if model == "model-b" {
			return []string{"off", "low", "high"}
		}
		return []string{"off", "low", "medium", "high", "max"}
	}
	options := func(model, mode string) []any {
		var values []any
		for _, value := range levels(model) {
			name := value
			if value == "" {
				name = "Provider default"
			}
			values = append(values, map[string]string{"value": value, "name": name})
		}
		if scenario == "grouped" {
			values = []any{map[string]any{"group": "levels", "name": "Levels", "options": values}}
		}
		thought := map[string]any{"id": configID, "name": "Thinking", "type": "select", "currentValue": mode, "options": values}
		if scenario != "id" {
			thought["category"] = "thought_level"
		}
		return []any{map[string]any{"id": "model", "category": "model", "name": "Model", "type": "select", "currentValue": model,
			"options": []any{map[string]string{"name": "A", "value": "model-a"}, map[string]string{"name": "B", "value": "model-b"}}}, thought}
	}
	update := func(value any) {
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "mode-session", "update": value}})
	}
	model, mode, genericMode := "model-a", "medium", "ask"
	if scenario == "empty" {
		mode = ""
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage
			Method string
			Params struct{ ConfigID, Value, ModeID string }
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		switch req.Method {
		case "initialize":
			reply(req.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}})
		case "session/new":
			var modes []any
			for _, value := range levels(model) {
				modes = append(modes, map[string]string{"id": value, "name": "Thinking: " + value})
			}
			result := map[string]any{"sessionId": "mode-session", "modes": map[string]any{"currentModeId": mode, "availableModes": modes}}
			if scenario == "mixed" {
				modes = append(modes, map[string]string{"id": "ask", "name": "Ask"}, map[string]string{"id": "high", "name": "High priority"})
				result["modes"] = map[string]any{"currentModeId": genericMode, "availableModes": modes}
			}
			if !legacy {
				if scenario == "early" {
					update(map[string]any{"sessionUpdate": "config_option_update", "configOptions": options(model, mode)})
				} else {
					result["configOptions"] = options(model, mode)
				}
			}
			reply(req.ID, result)
		case "session/set_config_option", "session/set_mode":
			if scenario == "mixed" && req.Method == "session/set_mode" {
				if req.Params.ModeID != "ask" && req.Params.ModeID != "high" {
					os.Exit(3)
				}
				genericMode = req.Params.ModeID
				update(map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": genericMode})
				reply(req.ID, map[string]any{})
				continue
			}
			if req.Params.ConfigID == "model" {
				model, mode = req.Params.Value, "low"
				update(map[string]any{"sessionUpdate": "config_option_update", "configOptions": options(model, mode)})
				reply(req.ID, map[string]any{"configOptions": options(model, mode)})
				continue
			}
			value := req.Params.Value
			if legacy {
				value = req.Params.ModeID
			}
			if !slices.Contains(levels(model), value) || !legacy && req.Params.ConfigID != configID || legacy != (req.Method == "session/set_mode") {
				os.Exit(3)
			}
			if scenario == "error" {
				send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32603, "message": "thinking unavailable"}})
				continue
			}
			mode = value
			if strings.Contains(scenario, "clamp") {
				mode = "high"
			}
			if legacy {
				if scenario == "legacy-clamp" {
					update(map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": mode})
				}
				reply(req.ID, map[string]any{})
				continue
			}
			if scenario == "notify-clamp" {
				update(map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": mode})
				update(map[string]any{"sessionUpdate": "config_option_update", "configOptions": options(model, mode)})
				reply(req.ID, map[string]any{"configOptions": options(model, value)})
				continue
			}
			reply(req.ID, map[string]any{"configOptions": options(model, mode)})
		case "session/prompt":
			want := "high"
			if scenario == "empty" {
				want = ""
			}
			if mode != want || scenario == "mixed" && genericMode != "ask" {
				os.Exit(4)
			}
			reply(req.ID, map[string]string{"stopReason": "end_turn"})
		}
	}
	os.Exit(0)
}

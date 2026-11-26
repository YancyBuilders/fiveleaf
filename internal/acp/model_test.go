package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

func startModelClient(t *testing.T, scenario string) (*Client, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	c, err := Start(ctx, Config{Command: os.Args[0], Args: []string{"-test.run=^TestACPModelProcess$"},
		Env: map[string]string{"FIVELEAF_ACP_MODEL_TEST": scenario}}, t.TempDir(), &testHandler{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, ctx
}

func TestACPModelSelectionAcrossSessions(t *testing.T) {
	for _, scenario := range []string{"category", "id", "grouped", "early", "legacy", "legacy-early", "legacy-refresh-before-response"} {
		t.Run(scenario, func(t *testing.T) {
			c, ctx := startModelClient(t, scenario)
			if err := c.SetModel(ctx, "missing", "target"); err == nil || !strings.Contains(err.Error(), "initialized") {
				t.Fatalf("uninitialized model setter = %v", err)
			}
			if _, err := c.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			if err := c.SetModel(ctx, "missing", "target"); !errors.Is(err, ErrUnknownSession) {
				t.Fatalf("unknown model session = %v", err)
			}
			for range 2 {
				session, err := c.NewSession(ctx)
				if err != nil {
					t.Fatal(err)
				}
				id := string(session.SessionId)
				if err := c.SetModel(ctx, id, "unadvertised"); err == nil {
					t.Fatal("unadvertised model was accepted")
				}
				if err := c.SetModel(ctx, id, "target"); err != nil {
					t.Fatal(err)
				}
				if _, err := c.Prompt(ctx, id, []ContentBlock{TextBlock("model must be selected")}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestACPModelStateSnapshots(t *testing.T) {
	for _, scenario := range []string{"category", "id", "grouped", "early", "legacy", "legacy-early"} {
		t.Run(scenario, func(t *testing.T) {
			c, ctx := startModelClient(t, scenario)
			if _, ok := c.ModelState("missing"); ok {
				t.Fatal("unknown session advertised a model selector")
			}
			if _, err := c.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			session, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			id := string(session.SessionId)
			state, ok := c.ModelState(id)
			want := ModelChoice{ID: "target", Name: "Target", Description: "Target model"}
			if scenario == "grouped" {
				want.Group = "Provider"
			}
			if !ok || state.Current != "default" || len(state.Available) == 0 || state.Available[0] != want {
				t.Fatalf("initial selector = %+v, %v", state, ok)
			}
			if scenario == "grouped" && (len(state.Available) != 2 || state.Available[1].ID != "other" || state.Available[1].Group != "Other provider") {
				t.Fatalf("model groups were not retained: %+v", state)
			}
			state.Available[0] = ModelChoice{ID: "modified"}
			state, _ = c.ModelState(id)
			if state.Available[0].ID != "target" {
				t.Fatal("snapshot shares mutable model choices")
			}
			if err := c.SetModel(ctx, id, "target"); err != nil {
				t.Fatal(err)
			}
			state, ok = c.ModelState(id)
			if !ok || state.Current != "target" {
				t.Fatalf("confirmed selector = %+v, %v", state, ok)
			}
			c.Close()
			if _, ok := c.ModelState(id); ok {
				t.Fatal("closed connection advertised a model selector")
			}
		})
	}
}

func TestACPModelCachesResponsesAndUpdates(t *testing.T) {
	for _, scenario := range []string{"cache-response", "cache-update", "notify-before-response", "refresh-before-response"} {
		t.Run(scenario, func(t *testing.T) {
			c, ctx := startModelClient(t, scenario)
			if _, err := c.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			session, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			id := string(session.SessionId)
			if err := c.SetModel(ctx, id, "target"); err != nil {
				t.Fatal(err)
			}
			state, ok := c.ModelState(id)
			if !ok || state.Current != "target" {
				t.Fatalf("selected model = %+v, %v", state, ok)
			}
			if scenario == "cache-update" {
				if _, err := c.Prompt(ctx, id, []ContentBlock{TextBlock("update available models")}); err != nil {
					t.Fatal(err)
				}
			}
			if err := c.SetModel(ctx, id, "next"); err != nil {
				t.Fatalf("model advertised in response/update was lost: %v", err)
			}
		})
	}
}

func TestACPModelRejectsUnavailableOrFailedSelection(t *testing.T) {
	for _, scenario := range []string{"none", "setter-fails", "wrong-current"} {
		t.Run(scenario, func(t *testing.T) {
			c, ctx := startModelClient(t, scenario)
			if _, err := c.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			session, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			id := string(session.SessionId)
			if err := c.SetModel(ctx, id, ""); err != nil {
				t.Fatalf("default model sent a setter: %v", err)
			}
			if err := c.SetModel(ctx, id, "target"); err == nil {
				t.Fatal("unavailable or failed model was silently accepted")
			}
			state, ok := c.ModelState(id)
			if scenario == "none" && ok || scenario != "none" && (!ok || state.Current != "default") {
				t.Fatalf("failed selection changed model: %+v, %v", state, ok)
			}
		})
	}
}

func TestACPLegacyModelUpdatesReachHandler(t *testing.T) {
	c, ctx := startModelClient(t, "legacy-early")
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := string(session.SessionId)
	handler := c.handler.(*testHandler)
	handler.mu.Lock()
	updates := append([]Update(nil), handler.updates...)
	handler.mu.Unlock()
	if len(updates) != 3 || updates[0].Update.UsageUpdate == nil || updates[1].Update.ConfigOptionUpdate == nil || updates[2].Update.UsageUpdate == nil {
		t.Fatalf("early notification order = %+v", updates)
	}
	selector := modelConfigOption(updates[1].Update.ConfigOptionUpdate.ConfigOptions)
	if selector == nil || selector.CurrentValue != "default" || !modelConfigHasValue(selector, "target") {
		t.Fatalf("legacy notification lost selector: %+v", selector)
	}
	if usage, ok := c.UsageState(id); !ok || usage != (UsageState{Used: 20, Size: 200}) {
		t.Fatalf("early usage after model change = %+v, %v", usage, ok)
	}
	// The peer exits if the synthesized notification changes the setter method.
	if err := c.SetModel(ctx, id, "target"); err != nil {
		t.Fatal(err)
	}
	handler.mu.Lock()
	last := handler.updates[len(handler.updates)-1]
	handler.mu.Unlock()
	selector = modelConfigOption(last.Update.ConfigOptionUpdate.ConfigOptions)
	if selector == nil || selector.CurrentValue != "target" {
		t.Fatalf("selected legacy model did not reach the handler: %+v", last)
	}
}

func TestACPUsageStateAcrossModelSelection(t *testing.T) {
	for _, scenario := range []string{"usage-early", "usage-switch", "usage-clear", "legacy-usage-switch"} {
		t.Run(scenario, func(t *testing.T) {
			c, ctx := startModelClient(t, scenario)
			if _, ok := c.UsageState("missing"); ok {
				t.Fatal("unknown session reported context usage")
			}
			if _, err := c.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			session, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			id := string(session.SessionId)
			usage, ok := c.UsageState(id)
			if !ok || usage != (UsageState{Used: 10, Size: 100}) {
				t.Fatalf("initial usage = %+v, %v", usage, ok)
			}
			other, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := c.UsageState(string(other.SessionId)); ok {
				t.Fatal("usage leaked into a different session")
			}
			if err := c.SetModel(ctx, id, "target"); err != nil {
				t.Fatal(err)
			}
			usage, ok = c.UsageState(id)
			if strings.HasSuffix(scenario, "usage-switch") {
				if !ok || usage != (UsageState{Used: 20, Size: 200}) {
					t.Fatalf("usage reported during selection was lost: %+v, %v", usage, ok)
				}
			} else if ok {
				t.Fatalf("model change kept stale context usage: %+v", usage)
			}
			c.Close()
			if _, ok := c.UsageState(id); ok {
				t.Fatal("closed connection reported context usage")
			}
		})
	}
}

func TestACPUsageIgnoresInvalidReports(t *testing.T) {
	handler := &testHandler{}
	c := &Client{ctx: context.Background(), handler: handler, sessions: map[string]*sessionState{"known": {}}}
	if _, ok := c.UsageState("known"); ok {
		t.Fatal("unreported usage appeared as an empty context")
	}
	for _, report := range []UsageState{{10, 100}, {-1, 100}, {50, 0}, {50, -1}, {150, 100}} {
		if err := c.update(Update{SessionId: "known", Update: sdk.SessionUpdate{UsageUpdate: &sdk.SessionUsageUpdate{
			SessionUpdate: "usage_update", Used: report.Used, Size: report.Size,
		}}}); err != nil {
			t.Fatal(err)
		}
		want := UsageState{10, 100}
		if report.Used == 150 {
			want = report
		}
		if usage, ok := c.UsageState("known"); !ok || usage != want {
			t.Fatalf("report %+v produced %+v, %v; want %+v", report, usage, ok, want)
		}
	}
	if len(handler.updates) != 2 {
		t.Fatalf("invalid usage reached handler: %d updates", len(handler.updates))
	}
}

func TestACPUsageAfterNewSessionResponseReachesClient(t *testing.T) {
	c, ctx := startModelClient(t, "usage-early")
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage, ok := c.UsageState(string(session.SessionId)); !ok || usage != (UsageState{Used: 10, Size: 100}) {
		t.Fatalf("usage sent before NewSession returned = %+v, %v", usage, ok)
	}
}

func TestACPPromptUsageUsesInputSplit(t *testing.T) {
	c, ctx := startModelClient(t, "usage-early")
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, string(session.SessionId), []ContentBlock{TextBlock("usage")}); err != nil {
		t.Fatal(err)
	}
	if tokens, ok := c.PromptUsage(string(session.SessionId)); !ok || tokens != (PromptUsage{Input: 20343, CachedInput: 4224}) {
		t.Fatalf("prompt input split = %+v, %v", tokens, ok)
	}
}

// The subprocess checks the protocol independently of the SDK encoders.
func TestACPModelProcess(t *testing.T) {
	scenario := os.Getenv("FIVELEAF_ACP_MODEL_TEST")
	if scenario == "" {
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	send := func(value any) {
		if encoder.Encode(value) != nil {
			os.Exit(2)
		}
	}
	respond := func(id json.RawMessage, value any) { send(map[string]any{"jsonrpc": "2.0", "id": id, "result": value}) }
	optionID := "engine"
	if scenario == "id" {
		optionID = "model"
	}
	options := func(current string, extra bool) []any {
		values := []any{map[string]string{"name": "Target", "value": "target", "description": "Target model"}}
		if extra {
			values = append(values, map[string]string{"name": "Next", "value": "next"})
		}
		if scenario == "grouped" {
			values = []any{map[string]any{"group": "provider", "name": "Provider", "options": values}, map[string]any{
				"group": "other", "name": "Other provider", "options": []any{map[string]string{"name": "Other", "value": "other"}},
			}}
		}
		option := map[string]any{"id": optionID, "name": "Model", "type": "select", "currentValue": current, "options": values}
		if scenario != "id" {
			option["category"] = "model"
		}
		if scenario == "category" {
			return []any{map[string]any{"id": "model", "name": "Fallback", "type": "select", "currentValue": "wrong", "options": []any{map[string]string{"name": "Wrong", "value": "wrong"}}}, option}
		}
		return []any{option}
	}
	update := func(session string, config []any) {
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": session, "update": map[string]any{"sessionUpdate": "config_option_update", "configOptions": config},
		}})
	}
	legacyUpdate := func(session, model string) {
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": session, "update": map[string]any{"sessionUpdate": "current_model_update", "currentModelId": model},
		}})
	}
	usageUpdate := func(session string, used, size int) {
		send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": session, "update": map[string]any{"sessionUpdate": "usage_update", "used": used, "size": size},
		}})
	}
	count := 0
	selected := map[string]string{}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage
			Method string
			Params struct{ SessionID, ConfigID, Value, ModelID string }
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		switch req.Method {
		case "initialize":
			respond(req.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}})
		case "session/new":
			count++
			id := fmt.Sprintf("model-%d", count)
			selected[id] = "default"
			result := map[string]any{"sessionId": id}
			switch scenario {
			case "none":
			case "early":
				update(id, options("default", false))
			case "legacy", "legacy-early", "legacy-usage-switch", "legacy-refresh-before-response":
				result["models"] = map[string]any{"currentModelId": "default", "availableModels": []any{map[string]string{"modelId": "target", "name": "Target", "description": "Target model"}}}
				if scenario == "legacy-early" {
					result["models"].(map[string]any)["currentModelId"] = "old"
					usageUpdate(id, 10, 100)
					legacyUpdate(id, "default")
					usageUpdate(id, 20, 200)
				}
			default:
				result["configOptions"] = options("default", false)
			}
			if strings.Contains(scenario, "usage-") && count == 1 {
				usageUpdate(id, 10, 100)
			}
			respond(req.ID, result)
		case "session/set_config_option", "session/set_model":
			if req.Method == "session/set_model" {
				if !strings.HasPrefix(scenario, "legacy") || req.Params.ModelID != "target" {
					os.Exit(3)
				}
				if scenario == "legacy-refresh-before-response" {
					legacyUpdate(req.Params.SessionID, selected[req.Params.SessionID])
				}
				selected[req.Params.SessionID] = req.Params.ModelID
				if scenario == "legacy-early" || scenario == "legacy-usage-switch" {
					legacyUpdate(req.Params.SessionID, req.Params.ModelID)
				}
				if scenario == "legacy-usage-switch" {
					usageUpdate(req.Params.SessionID, 20, 200)
				}
				respond(req.ID, map[string]any{})
				continue
			}
			if req.Params.ConfigID != optionID || req.Params.Value != "target" && req.Params.Value != "next" || scenario == "none" {
				os.Exit(3)
			}
			if scenario == "setter-fails" {
				send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32603, "message": "Model selection failed"}})
				continue
			}
			current := req.Params.Value
			if scenario == "wrong-current" {
				current = "default"
			}
			previous := selected[req.Params.SessionID]
			selected[req.Params.SessionID] = current
			if scenario == "notify-before-response" {
				update(req.Params.SessionID, options(current, true))
				respond(req.ID, map[string]any{"configOptions": options("default", false)})
				continue
			}
			if scenario == "refresh-before-response" {
				update(req.Params.SessionID, options(previous, true))
				respond(req.ID, map[string]any{"configOptions": options(current, true)})
				continue
			}
			if scenario == "usage-switch" || scenario == "usage-clear" {
				update(req.Params.SessionID, options(current, false))
			}
			if scenario == "usage-switch" {
				usageUpdate(req.Params.SessionID, 20, 200)
			}
			respond(req.ID, map[string]any{"configOptions": options(current, scenario == "cache-response" || current == "next")})
		case "session/prompt":
			if scenario != "usage-early" && selected[req.Params.SessionID] != "target" {
				os.Exit(4)
			}
			if scenario == "cache-update" {
				update(req.Params.SessionID, options("target", true))
			}
			result := map[string]any{"stopReason": "end_turn"}
			if scenario == "usage-early" {
				result["_meta"] = map[string]any{"quota": map[string]any{"token_count": map[string]any{
					"inputTokens": 20343, "cachedInputTokens": 4224, "outputTokens": 7,
				}}}
			}
			respond(req.ID, result)
		}
	}
	os.Exit(0)
}

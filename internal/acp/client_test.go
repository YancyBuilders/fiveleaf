package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"
)

type testHandler struct {
	mu         sync.Mutex
	updates    []Update
	readPath   string
	writePath  string
	written    string
	permission func(context.Context, PermissionRequest) (PermissionOutcome, error)
}

func (h *testHandler) Update(p Update) {
	h.mu.Lock()
	h.updates = append(h.updates, p)
	h.mu.Unlock()
}

func (h *testHandler) RequestPermission(ctx context.Context, p PermissionRequest) (PermissionOutcome, error) {
	if h.permission != nil {
		return h.permission(ctx, p)
	}
	return SelectedPermission("allow"), nil
}

func (h *testHandler) ReadTextFile(_ context.Context, p ReadRequest) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readPath = p.Path
	if p.Line != nil || p.Limit != nil {
		return "", errors.New("handler must receive whole-buffer request")
	}
	return "unsaved first\nunsaved second\nunsaved third", nil
}

func (h *testHandler) WriteTextFile(_ context.Context, p WriteRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writePath, h.written = p.Path, p.Content
	return nil
}

func startTestClient(t *testing.T, scenario string, handler *testHandler) (*Client, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	root := t.TempDir()
	c, err := Start(ctx, Config{
		Command: os.Args[0], Args: []string{"-test.run=^TestACPAgentProcess$"},
		Env: map[string]string{"FIVELEAF_ACP_TEST_PROCESS": scenario},
	}, root, handler)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, ctx
}

func readyTestSession(t *testing.T, c *Client, ctx context.Context) string {
	t.Helper()
	result, err := c.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != 1 || !result.AgentCapabilities.PromptCapabilities.EmbeddedContext {
		t.Fatalf("initialize: %+v", result)
	}
	if err := c.Authenticate(ctx, "login"); err != nil {
		t.Fatal(err)
	}
	session, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return string(session.SessionId)
}

func TestACPStdioConversation(t *testing.T) {
	h := &testHandler{}
	c, ctx := startTestClient(t, "conversation", h)
	sessionID := readyTestSession(t, c, ctx)
	if err := c.SetMode(ctx, sessionID, "code"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMode(ctx, sessionID, "unadvertised"); err == nil {
		t.Fatal("accepted an unadvertised mode")
	}
	result, err := c.Prompt(ctx, sessionID, []ContentBlock{TextBlock("test"), ResourceBlock(filepath.Join(c.root, "a #.go"), "buffer")})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("stop reason: %s", result.StopReason)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var text strings.Builder
	for _, update := range h.updates {
		if chunk := update.Update.AgentMessageChunk; chunk != nil && chunk.Content.Text != nil {
			text.WriteString(chunk.Content.Text.Text)
		}
	}
	if text.String() != "ready: hello world" {
		t.Fatalf("stream order = %q", text.String())
	}
	if h.readPath != filepath.Join(c.root, "test.go") || h.writePath != filepath.Join(c.root, "empty.go") || h.written != "" {
		t.Fatalf("callbacks: read=%q write=%q content=%q", h.readPath, h.writePath, h.written)
	}
}

func TestACPCancelPendingPermission(t *testing.T) {
	permissionReady := make(chan struct{}, 2)
	permissionCancelled := make(chan struct{}, 2)
	h := &testHandler{permission: func(ctx context.Context, _ PermissionRequest) (PermissionOutcome, error) {
		permissionReady <- struct{}{}
		<-ctx.Done()
		permissionCancelled <- struct{}{}
		return PermissionOutcome{}, ctx.Err()
	}}
	c, ctx := startTestClient(t, "cancel-many", h)
	sessionID := readyTestSession(t, c, ctx)
	type response struct {
		result PromptResult
		err    error
	}
	finished := make(chan response, 1)
	go func() {
		result, err := c.Prompt(ctx, sessionID, []ContentBlock{TextBlock("wait")})
		finished <- response{result, err}
	}()
	for range 2 {
		select {
		case <-permissionReady:
		case <-ctx.Done():
			t.Fatal("permission did not arrive")
		}
	}
	if err := c.Cancel(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-permissionCancelled:
		case <-ctx.Done():
			t.Fatal("permission callback was not cancelled")
		}
	}
	select {
	case r := <-finished:
		if r.err != nil || r.result.StopReason != "cancelled" {
			t.Fatalf("cancel result = %+v, %v", r.result, r.err)
		}
	case <-ctx.Done():
		t.Fatal("prompt did not finish after cancel")
	}
}

func TestACPCloseCancelsCallbacks(t *testing.T) {
	permissionReady := make(chan struct{})
	h := &testHandler{permission: func(ctx context.Context, _ PermissionRequest) (PermissionOutcome, error) {
		close(permissionReady)
		<-ctx.Done()
		return CancelledPermission(), nil
	}}
	c, ctx := startTestClient(t, "cancel", h)
	sessionID := readyTestSession(t, c, ctx)
	finished := make(chan error, 1)
	go func() {
		_, err := c.Prompt(ctx, sessionID, []ContentBlock{TextBlock("wait")})
		finished <- err
	}()
	select {
	case <-permissionReady:
	case <-ctx.Done():
		t.Fatal("permission did not arrive")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("closed prompt unexpectedly succeeded")
		}
	case <-ctx.Done():
		t.Fatal("close left a pending prompt")
	}
}

func TestACPPromptContextCancellation(t *testing.T) {
	permissionReady := make(chan struct{})
	h := &testHandler{permission: func(ctx context.Context, _ PermissionRequest) (PermissionOutcome, error) {
		close(permissionReady)
		<-ctx.Done()
		return CancelledPermission(), nil
	}}
	c, ctx := startTestClient(t, "cancel", h)
	sessionID := readyTestSession(t, c, ctx)
	promptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := c.Prompt(promptCtx, sessionID, []ContentBlock{TextBlock("wait")})
		finished <- err
	}()
	select {
	case <-permissionReady:
	case <-ctx.Done():
		t.Fatal("permission did not arrive")
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled prompt unexpectedly succeeded")
		}
	case <-ctx.Done():
		t.Fatal("context cancellation left a pending prompt")
	}
	if _, err := c.Prompt(ctx, sessionID, []ContentBlock{TextBlock("again")}); err == nil || !strings.Contains(err.Error(), "acknowledgement") {
		t.Fatalf("unacknowledged turn was reused: %v", err)
	}
}

func TestACPStopsWhenAdapterStopsReading(t *testing.T) {
	c, ctx := startTestClient(t, "stopped-reading", &testHandler{})
	sessionID := readyTestSession(t, c, ctx)
	promptCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := c.Prompt(promptCtx, sessionID, []ContentBlock{TextBlock(strings.Repeat("context ", 1<<18))})
		finished <- err
	}()
	select {
	case err := <-finished:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("prompt error = %v, want deadline exceeded", err)
		}
	case <-ctx.Done():
		t.Fatal("prompt stayed blocked after its deadline")
	}
	select {
	case <-c.Done():
		if err := c.Err(); err == nil || !strings.Contains(err.Error(), "write timed out") {
			t.Fatalf("adapter stopped without a write timeout: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("stalled adapter was not reaped")
	}
}

func TestACPUnsupportedVersionStopsProcess(t *testing.T) {
	c, ctx := startTestClient(t, "wrong-version", &testHandler{})
	_, err := c.Initialize(ctx)
	if err == nil || !strings.Contains(err.Error(), "protocol version 2") {
		t.Fatalf("version error = %v", err)
	}
	select {
	case <-c.Done():
	case <-ctx.Done():
		t.Fatal("incompatible peer was not stopped")
	}
}

func TestACPAgentExitAndStderrLimit(t *testing.T) {
	c, ctx := startTestClient(t, "exit", &testHandler{})
	_, err := c.Initialize(ctx)
	if err == nil {
		t.Fatal("initialize succeeded after agent exit")
	}
	select {
	case <-c.Done():
	case <-ctx.Done():
		t.Fatal("agent was not reaped")
	}
	if c.Err() == nil {
		t.Fatal("missing exit error")
	}
	stderr := c.Stderr()
	if len(stderr) > MaxStderrBytes || !utf8.ValidString(stderr) || !strings.HasSuffix(stderr, "intentional exit\n") || !strings.Contains(stderr, "agent failure\n") {
		t.Fatalf("invalid bounded stderr: %d bytes", len(stderr))
	}
}

func TestACPOversizedMessageDisconnects(t *testing.T) {
	c, ctx := startTestClient(t, "oversized", &testHandler{})
	_, err := c.Initialize(ctx)
	if err == nil {
		t.Fatal("oversized initialize succeeded")
	}
	select {
	case <-c.Done():
	case <-ctx.Done():
		t.Fatal("oversized agent was not stopped")
	}
	if !errors.Is(c.Err(), ErrMessageTooLarge) {
		t.Fatalf("error = %v", c.Err())
	}
}

func TestACPDisconnectIdentifiesIncompleteAdapterCache(t *testing.T) {
	client, ctx := startTestClient(t, "incomplete-npm-cache", &testHandler{})
	_, err := client.Initialize(ctx)
	if err == nil || !strings.Contains(err.Error(), "initialize") || !strings.Contains(err.Error(), "npm adapter cache") || strings.Contains(err.Error(), "private-cache-path") {
		t.Fatalf("expected a startup diagnosis without private stderr, got %v", err)
	}
}

func TestACPAuthenticationRequiredGivesSignInHint(t *testing.T) {
	client, ctx := startTestClient(t, "needs-login", &testHandler{})
	if _, err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := client.NewSession(ctx)
	var rpc *sdk.RequestError
	if !errors.As(err, &rpc) || rpc.Code != -32000 || !strings.Contains(err.Error(), "Sign in") || strings.Contains(err.Error(), "private-account-detail") {
		t.Fatalf("expected an actionable login error that preserves the protocol code, got %v", err)
	}
}

func TestACPUnknownSessionRequestsAreRejected(t *testing.T) {
	c, ctx := startTestClient(t, "unknown-session", &testHandler{})
	sessionID := readyTestSession(t, c, ctx)
	if _, err := c.Prompt(ctx, sessionID, []ContentBlock{TextBlock("test")}); err != nil {
		t.Fatal(err)
	}
}

func TestACPResolvePath(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	c := &Client{root: root}
	inside := filepath.Join(root, "new", "file.go")
	if p, err := c.ResolvePath(inside); err != nil || p != inside {
		t.Fatalf("new path = %q, %v", p, err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative.go", root, root + "-neighbor/file.go", filepath.Join(root, "escape", "new.go"), filepath.Join(root, ".git", "hooks", "post-checkout")} {
		if _, err := c.ResolvePath(path); err == nil {
			t.Errorf("accepted path %q", path)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file.go"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "file.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	if p, err := c.ResolvePath(filepath.Join(root, "link.go")); err != nil || p != filepath.Join(root, "file.go") {
		t.Fatalf("internal symlink = %q, %v", p, err)
	}
}

func TestACPLineRangesAndResourceURI(t *testing.T) {
	for _, test := range []struct {
		line, limit int
		want        string
	}{{1, 1, "one\n"}, {2, 2, "two\nthree"}, {5, 1, ""}, {1, 0, ""}} {
		if got := textLines("one\ntwo\nthree", &test.line, &test.limit); got != test.want {
			t.Errorf("range %d,%d: %q", test.line, test.limit, got)
		}
	}
	b := ResourceBlock("/repo/a #.go", "unsaved")
	if got := b.Resource.Resource.TextResourceContents.Uri; got != "file:///repo/a%20%23.go" {
		t.Fatalf("URI = %q", got)
	}
}

// TestACPAgentProcess is an actual subprocess peer. Its assertions validate
// the wire contract independently of the SDK's generated serialization.
func TestACPAgentProcess(t *testing.T) {
	scenario := os.Getenv("FIVELEAF_ACP_TEST_PROCESS")
	if scenario == "" {
		return
	}
	if err := runTestAgent(scenario); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(8)
	}
	os.Exit(0)
}

type wireMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

func runTestAgent(scenario string) error {
	if scenario == "exit" {
		fmt.Fprint(os.Stderr, strings.Repeat("界", MaxStderrBytes))
		fmt.Fprintln(os.Stderr, "agent failure")
		return errors.New("intentional exit")
	}
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 4096), MaxMessageBytes)
	read := func() (wireMessage, error) {
		var m wireMessage
		if !reader.Scan() {
			return m, errors.New("client disconnected")
		}
		return m, json.Unmarshal(reader.Bytes(), &m)
	}
	enc := json.NewEncoder(os.Stdout)
	send := func(v any) error { return enc.Encode(v) }
	reply := func(id json.RawMessage, result any) error {
		return send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	update := func(text string) error {
		return send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
			"sessionId": "session-1", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": text}},
		}})
	}
	request := func(id, method string, params any) (wireMessage, error) {
		if err := send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			return wireMessage{}, err
		}
		return read()
	}
	var root string
	for {
		m, err := read()
		if err != nil {
			return nil
		}
		switch m.Method {
		case "initialize":
			if scenario == "incomplete-npm-cache" {
				return errors.New("npm error ENOENT: missing private-cache-path/_npx/test/package.json")
			}
			if scenario == "oversized" {
				fmt.Fprint(os.Stdout, strings.Repeat("x", MaxMessageBytes+1))
				continue
			}
			var p sdk.InitializeRequest
			if err := json.Unmarshal(m.Params, &p); err != nil {
				return err
			}
			if p.ProtocolVersion != 1 || !p.ClientCapabilities.Fs.ReadTextFile || !p.ClientCapabilities.Fs.WriteTextFile || p.ClientCapabilities.Terminal {
				return errors.New("wrong initialize capabilities")
			}
			if scenario == "wrong-version" {
				reply(m.ID, map[string]any{"protocolVersion": 2, "authMethods": []any{}})
				continue
			}
			reply(m.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"promptCapabilities": map[string]bool{"embeddedContext": true}}, "authMethods": []any{map[string]string{"id": "login", "name": "Log in"}}})
		case "authenticate":
			reply(m.ID, map[string]any{})
		case "session/new":
			if scenario == "needs-login" {
				send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": sdk.NewAuthRequired("private-account-detail")})
				continue
			}
			var p sdk.NewSessionRequest
			if err := json.Unmarshal(m.Params, &p); err != nil || !filepath.IsAbs(p.Cwd) || p.McpServers == nil {
				return errors.New("invalid new session request")
			}
			root = p.Cwd
			update("ready: ") // Some agents send notifications before returning session/new.
			reply(m.ID, map[string]any{"sessionId": "session-1", "modes": map[string]any{"currentModeId": "ask", "availableModes": []any{map[string]string{"id": "ask", "name": "Ask"}, map[string]string{"id": "code", "name": "Code"}}}})
			if scenario == "stopped-reading" {
				time.Sleep(time.Minute)
				return nil
			}
		case "session/set_mode":
			reply(m.ID, map[string]any{})
		case "session/prompt":
			if scenario == "unknown-session" {
				response, err := request("bad", "fs/read_text_file", map[string]any{"sessionId": "wrong", "path": filepath.Join(root, "test.go")})
				if err != nil || len(response.Error) == 0 {
					return errors.New("unknown session was not rejected")
				}
				reply(m.ID, map[string]any{"stopReason": "end_turn"})
				continue
			}
			if scenario == "cancel" || scenario == "cancel-many" {
				pending := map[string]bool{"permission-cancel": true}
				if scenario == "cancel-many" {
					pending["permission-cancel-2"] = true
				}
				for id := range pending {
					send(map[string]any{"jsonrpc": "2.0", "id": id, "method": "session/request_permission", "params": map[string]any{"sessionId": "session-1", "toolCall": map[string]string{"toolCallId": id}, "options": []any{map[string]string{"optionId": "allow", "name": "Allow", "kind": "allow_once"}}}})
				}
				cancelSeen := false
				for !cancelSeen || len(pending) != 0 {
					next, err := read()
					if err != nil {
						return nil
					}
					if next.Method == "session/cancel" {
						cancelSeen = true
					}
					var id string
					json.Unmarshal(next.ID, &id)
					if pending[id] {
						if !strings.Contains(string(next.Result), `"cancelled"`) {
							return fmt.Errorf("permission not cancelled: %s", next.Result)
						}
						delete(pending, id)
					}
				}
				reply(m.ID, map[string]any{"stopReason": "cancelled"})
				continue
			}
			update("hello ")
			response, err := request("permission", "session/request_permission", map[string]any{"sessionId": "session-1", "toolCall": map[string]string{"toolCallId": "call-1"}, "options": []any{map[string]string{"optionId": "allow", "name": "Allow", "kind": "allow_once"}}})
			if err != nil || !strings.Contains(string(response.Result), `"optionId":"allow"`) {
				return errors.New("permission outcome not selected")
			}
			response, err = request("read", "fs/read_text_file", map[string]any{"sessionId": "session-1", "path": filepath.Join(root, "test.go"), "line": 2, "limit": 1})
			if err != nil || !strings.Contains(string(response.Result), `"content":"unsaved second\n"`) {
				return fmt.Errorf("wrong unsaved read: %s (%v)", response.Result, err)
			}
			response, err = request("write", "fs/write_text_file", map[string]any{"sessionId": "session-1", "path": filepath.Join(root, "empty.go"), "content": ""})
			if err != nil || len(response.Error) != 0 {
				return fmt.Errorf("empty-file write failed: %s", response.Error)
			}
			response, err = request("terminal", "terminal/create", map[string]any{"sessionId": "session-1", "command": "must-not-run"})
			if err != nil || !strings.Contains(string(response.Error), "-32601") {
				return fmt.Errorf("unsupported terminal capability was accepted: %s", response.Error)
			}
			update("world")
			reply(m.ID, map[string]any{"stopReason": "end_turn"})
		}
	}
}

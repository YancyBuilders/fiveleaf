package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	write      func(context.Context, WriteRequest) error
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

func (h *testHandler) WriteTextFile(ctx context.Context, p WriteRequest) error {
	if h.write != nil {
		return h.write(ctx, p)
	}
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

func TestACPReattachSession(t *testing.T) {
	for _, test := range []struct {
		name, wantID, wantError string
	}{
		{name: "resume", wantID: "previous-session"},
		{name: "unsupported", wantID: "session-1"},
		{name: "load", wantID: "previous-session"},
		{name: "both-missing", wantID: "session-1"},
		{name: "missing", wantID: "session-1"},
		{name: "codex-missing", wantID: "session-1"},
		{name: "different-adapter", wantError: "no rollout found for thread id previous-session"},
		{name: "wrong-session", wantError: "missing different session"},
		{name: "auth", wantError: "Sign in using the agent's CLI"},
		{name: "writer", wantError: "thread_active_writer"},
		{name: "config", wantError: "invalid workspace configuration"},
		{name: "unrelated-method", wantError: "nested_method"},
		{name: "transport", wantError: "disconnected during session/resume"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &testHandler{}
			client, ctx := startTestClient(t, "reattach-"+test.name, handler)
			if _, err := client.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			session, err := client.ReattachSession(ctx, "previous-session")
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) || session.SessionId != "" {
					t.Fatalf("reattach hid the original error: session=%+v error=%v", session, err)
				}
				return
			}
			if err != nil || string(session.SessionId) != test.wantID || session.Restored != (test.wantID == "previous-session") || len(session.Replay) != 0 {
				t.Fatalf("reattach = %+v, %v; want %q without replay", session, err, test.wantID)
			}
			if test.name == "load" {
				handler.mu.Lock()
				defer handler.mu.Unlock()
				if len(handler.updates) != 0 {
					t.Fatalf("load fallback forwarded duplicate history: %+v", handler.updates)
				}
			}
		})
	}
}

func TestACPReattachRefusesSessionOwnedByLiveClient(t *testing.T) {
	first, ctx := startTestClient(t, "reattach-resume", &testHandler{})
	if _, err := first.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := first.ReattachSession(ctx, "previous-session")
	if err != nil {
		t.Fatal(err)
	}
	second, secondCtx := startTestClient(t, "reattach-resume", &testHandler{})
	if _, err := second.Initialize(secondCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := second.ReattachSession(secondCtx, string(session.SessionId)); !errors.Is(err, ErrSessionInUse) {
		t.Fatalf("second client claimed the live session: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.Done():
	case <-ctx.Done():
		t.Fatal("first client did not release its session")
	}
	if _, err := second.ReattachSession(secondCtx, string(session.SessionId)); errors.Is(err, ErrSessionInUse) {
		t.Fatalf("a retired owner still blocked the session: %v", err)
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

func TestACPPromptWithStopPreservesSession(t *testing.T) {
	permissionReady, permissionCanceled := make(chan struct{}), make(chan struct{})
	h := &testHandler{permission: func(ctx context.Context, _ PermissionRequest) (PermissionOutcome, error) {
		close(permissionReady)
		<-ctx.Done()
		close(permissionCanceled)
		return CancelledPermission(), nil
	}}
	c, ctx := startTestClient(t, "cooperative-stop", h)
	sessionID := readyTestSession(t, c, ctx)
	stopCtx, stop := context.WithCancel(ctx)
	defer stop()
	type outcome struct {
		result PromptResult
		sent   bool
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, sent, err := c.PromptWithStop(stopCtx, sessionID, []ContentBlock{TextBlock("wait")}, 0, nil)
		finished <- outcome{result, sent, err}
	}()
	select {
	case <-permissionReady:
	case <-ctx.Done():
		t.Fatal("the process did not receive the first prompt")
	}
	stop()
	select {
	case <-permissionCanceled:
	case <-ctx.Done():
		t.Fatal("Stop did not cancel the pending permission")
	}
	if _, sent, err := c.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("too early")}, 0, nil); sent || err == nil || !strings.Contains(err.Error(), "already responding") {
		t.Fatalf("started a second prompt before the original reply: sent=%v, err=%v", sent, err)
	}
	if err := os.WriteFile(filepath.Join(c.root, "cancel-ack"), []byte("allow reply"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-finished:
		if got.err != nil || !got.sent || got.result.StopReason != "cancelled" {
			t.Fatalf("acknowledged Stop = %+v", got)
		}
	case <-ctx.Done():
		t.Fatalf("Stop did not finish after the original prompt response: %s", c.Stderr())
	}
	result, sent, err := c.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("again")}, 0, nil)
	if err != nil || !sent || result.StopReason != "end_turn" {
		t.Fatalf("next prompt = %+v, sent=%v, err=%v", result, sent, err)
	}
}

func TestACPPromptWithStopSkipsCanceledSendAndKeepsRPCRejection(t *testing.T) {
	c, ctx := startTestClient(t, "cooperative-stop", &testHandler{})
	sessionID := readyTestSession(t, c, ctx)
	stopCtx, stop := context.WithCancel(ctx)
	stop()
	if _, sent, err := c.PromptWithStop(stopCtx, sessionID, []ContentBlock{TextBlock("must not reach peer")}, 0, nil); sent || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled prompt: sent=%v, err=%v", sent, err)
	}
	if _, sent, err := c.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("reject")}, 0, nil); !sent || err == nil || !strings.Contains(err.Error(), "rejected by fixture") {
		t.Fatalf("RPC rejection: sent=%v, err=%v", sent, err)
	}
	result, sent, err := c.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("again")}, 0, nil)
	if err != nil || !sent || result.StopReason != "end_turn" {
		t.Fatalf("prompt after rejection = %+v, sent=%v, err=%v, peer=%s", result, sent, err, c.Stderr())
	}
}

func TestACPPromptWithStopCancelsPendingEditorWrite(t *testing.T) {
	writeReady := make(chan struct{}, 1)
	handler := &testHandler{write: func(ctx context.Context, _ WriteRequest) error {
		writeReady <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}}
	client, ctx := startTestClient(t, "cooperative-stop", handler)
	sessionID := readyTestSession(t, client, ctx)
	stopCtx, stop := context.WithCancel(ctx)
	defer stop()
	finished := make(chan error, 1)
	go func() {
		result, sent, err := client.PromptWithStop(stopCtx, sessionID, []ContentBlock{TextBlock("write")}, 0, nil)
		if err != nil || !sent || result.StopReason != "cancelled" {
			finished <- fmt.Errorf("Stop = %+v, sent=%v, err=%v", result, sent, err)
			return
		}
		finished <- nil
	}()
	select {
	case <-writeReady:
	case <-ctx.Done():
		t.Fatal("the agent's file write did not reach the handler")
	}
	stop()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("the agent never got a cancellation response for the write")
	}
	if result, sent, err := client.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("again")}, 0, nil); err != nil || !sent || result.StopReason != "end_turn" {
		t.Fatalf("prompt after canceling a write = %+v, sent=%v, err=%v", result, sent, err)
	}
}

func TestACPPromptWithStopAcknowledgesIdleTimeout(t *testing.T) {
	for _, test := range []struct {
		prompt, stopReason string
		err                error
	}{
		{prompt: "idle", stopReason: "cancelled", err: context.DeadlineExceeded},
		{prompt: "idle-completes", stopReason: "end_turn"},
	} {
		t.Run(test.prompt, func(t *testing.T) {
			client, ctx := startTestClient(t, "cooperative-stop", &testHandler{})
			sessionID := readyTestSession(t, client, ctx)
			var events []string
			result, sent, err := client.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock(test.prompt)}, 200*time.Millisecond, &PromptEvents{
				Written: func() { events = append(events, "written") },
				Idle:    func() { events = append(events, "idle") },
			})
			if !sent || !errors.Is(err, test.err) || string(result.StopReason) != test.stopReason {
				t.Fatalf("idle stop = %+v, sent=%v, err=%v", result, sent, err)
			}
			if got := strings.Join(events, ","); got != "written,idle" {
				t.Fatalf("callbacks received before the prompt returned = %q, want written,idle", got)
			}
			result, sent, err = client.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("again")}, 0, nil)
			if err != nil || !sent || result.StopReason != "end_turn" {
				t.Fatalf("prompt after idle stop = %+v, sent=%v, err=%v", result, sent, err)
			}
		})
	}
}

func TestACPPromptWithStopKeepsAdapterUntilReplyOrClose(t *testing.T) {
	for _, test := range []struct {
		prompt string
		close  bool
	}{
		{prompt: "delayed-ack"},
		{prompt: "stall", close: true},
	} {
		t.Run(test.prompt, func(t *testing.T) {
			client, ctx := startTestClient(t, "cooperative-stop", &testHandler{})
			sessionID := readyTestSession(t, client, ctx)
			idle := make(chan struct{})
			type outcome struct {
				result PromptResult
				sent   bool
				err    error
			}
			finished := make(chan outcome, 1)
			go func() {
				result, sent, err := client.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock(test.prompt)}, 100*time.Millisecond, &PromptEvents{Idle: func() { close(idle) }})
				finished <- outcome{result, sent, err}
			}()
			select {
			case <-idle:
			case <-ctx.Done():
				t.Fatal("the idle callback did not run while the prompt was still pending")
			}
			if _, sent, err := client.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("too early")}, 0, nil); sent || err == nil || !strings.Contains(err.Error(), "already responding") {
				t.Fatalf("another prompt started during the pending cancellation: sent=%v, err=%v", sent, err)
			}
			if test.close {
				if err := client.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				timer := time.NewTimer(6*time.Second + 200*time.Millisecond)
				defer timer.Stop()
				select {
				case <-client.Done():
					t.Fatalf("the pending cancellation stopped the adapter: %v", client.Err())
				case got := <-finished:
					t.Fatalf("prompt returned before the delayed reply: %+v", got)
				case <-timer.C:
				}
			}
			select {
			case got := <-finished:
				if test.close {
					if !got.sent || !errors.Is(got.err, ErrClosed) {
						t.Fatalf("closed prompt requires a transport error: %+v", got)
					}
					return
				}
				if !got.sent || !errors.Is(got.err, context.DeadlineExceeded) || got.result.StopReason != "cancelled" {
					t.Fatalf("delayed reply = %+v", got)
				}
			case <-ctx.Done():
				t.Fatal("the pending prompt did not settle")
			}
			if result, sent, err := client.PromptWithStop(ctx, sessionID, []ContentBlock{TextBlock("again")}, 0, nil); err != nil || !sent || result.StopReason != "end_turn" {
				t.Fatalf("prompt on the original process after late reply = %+v, sent=%v, err=%v", result, sent, err)
			}
		})
	}
}

type gatedPromptWriter struct {
	entered, release chan struct{}
	other            chan []byte
}

func (writer gatedPromptWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte(`"method":"_test/gate"`)) {
		close(writer.entered)
		<-writer.release
	} else {
		writer.other <- bytes.Clone(data)
	}
	return len(data), nil
}

func (writer gatedPromptWriter) Close() error { return nil }

func TestACPPromptWithStopAbortsBeforeQueuedWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	reader, input := io.Pipe()
	defer input.Close()
	writer := gatedPromptWriter{entered: make(chan struct{}), release: make(chan struct{}), other: make(chan []byte, 3)}
	var release sync.Once
	defer release.Do(func() { close(writer.release) })
	const sessionID = "session-1"
	session := &sessionState{}
	client := &Client{stdin: writer, ctx: ctx, initialized: true, sessions: map[string]*sessionState{sessionID: session}}
	client.conn = sdk.NewConnection(nil, messageWriter{client}, reader)
	gateDone := make(chan error, 1)
	go func() { gateDone <- client.conn.SendNotification(ctx, "_test/gate", nil) }()
	select {
	case <-writer.entered:
	case <-ctx.Done():
		t.Fatal("the gate did not occupy the SDK write lock")
	}
	stopCtx, stop := context.WithCancel(ctx)
	defer stop()
	type outcome struct {
		sent bool
		err  error
	}
	finished := make(chan outcome, 1)
	writtenCount := 0
	go func() {
		_, sent, err := client.PromptWithStop(stopCtx, sessionID, []ContentBlock{TextBlock("must not reach the wire")}, 0, &PromptEvents{Written: func() { writtenCount++ }})
		finished <- outcome{sent, err}
	}()
	for {
		client.mu.Lock()
		queued := session.promptStop != nil
		client.mu.Unlock()
		if queued {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("the cooperative prompt did not reach the blocked writer")
		case <-time.After(time.Millisecond):
		}
	}
	stop()
	release.Do(func() { close(writer.release) })
	if err := <-gateDone; err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-finished:
		if got.sent || !errors.Is(got.err, context.Canceled) || writtenCount != 0 {
			t.Fatalf("canceled queued prompt = %+v; Written ran %d times", got, writtenCount)
		}
	case <-ctx.Done():
		t.Fatal("a canceled prompt waited for an adapter response")
	}
	select {
	case unexpected := <-writer.other:
		t.Fatalf("a canceled queued prompt still wrote a frame: %q", unexpected)
	default:
	}
}

func TestACPPromptIdleTimeoutPausesForPermission(t *testing.T) {
	const idle = 400 * time.Millisecond
	h := &testHandler{permission: func(ctx context.Context, _ PermissionRequest) (PermissionOutcome, error) {
		timer := time.NewTimer(2 * idle)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return CancelledPermission(), ctx.Err()
		case <-timer.C:
			return SelectedPermission("allow"), nil
		}
	}}
	c, ctx := startTestClient(t, "conversation", h)
	sessionID := readyTestSession(t, c, ctx)
	result, err := c.PromptWithIdleTimeout(ctx, sessionID, []ContentBlock{TextBlock("wait for permission")}, idle)
	if err != nil || result.StopReason != "end_turn" {
		t.Fatalf("permission wait interrupted a reply: %+v, %v", result, err)
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
	restore, reattaching := strings.CutPrefix(scenario, "reattach-")
	resumeTried, loadTried := false, false
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
			capabilities := map[string]any{"promptCapabilities": map[string]bool{"embeddedContext": true}}
			result := map[string]any{"protocolVersion": 1, "agentCapabilities": capabilities, "authMethods": []any{map[string]string{"id": "login", "name": "Log in"}}}
			if reattaching && restore != "unsupported" {
				capabilities["sessionCapabilities"] = map[string]any{"resume": map[string]any{}}
			}
			if restore == "load" || restore == "both-missing" {
				capabilities["loadSession"] = true
			}
			if restore == "codex-missing" {
				result["agentInfo"] = map[string]string{"name": "@agentclientprotocol/codex-acp", "version": "2.7.0"}
			}
			reply(m.ID, result)
		case "authenticate":
			reply(m.ID, map[string]any{})
		case "session/resume", "session/load":
			if !reattaching {
				return errors.New("unexpected restore")
			}
			var params struct {
				SessionID  string
				Cwd        string
				MCPServers json.RawMessage `json:"mcpServers"`
			}
			if err := json.Unmarshal(m.Params, &params); err != nil || params.SessionID != "previous-session" || !filepath.IsAbs(params.Cwd) {
				return errors.New("attempted to restore the wrong session")
			}
			if string(params.MCPServers) != "[]" {
				return fmt.Errorf("%s omitted its empty mcpServers array", m.Method)
			}
			if m.Method == "session/load" {
				if !resumeTried || loadTried {
					return errors.New("load was not tried once after resume")
				}
				loadTried = true
			} else {
				if resumeTried {
					return errors.New("resume was attempted more than once")
				}
				resumeTried = true
			}
			var rejection *sdk.RequestError
			switch restore {
			case "resume":
			case "load", "both-missing":
				if m.Method == "session/resume" || restore == "both-missing" {
					rejection = sdk.NewMethodNotFound(m.Method)
				} else if err := send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": params.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "cached history"}},
				}}); err != nil {
					return err
				}
			case "missing":
				rejection = &sdk.RequestError{Code: -32002, Message: "missing session", Data: map[string]string{"sessionId": params.SessionID}}
			case "codex-missing", "different-adapter":
				rejection = sdk.NewInternalError(map[string]string{"details": "no rollout found for thread id " + params.SessionID})
			case "wrong-session":
				rejection = &sdk.RequestError{Code: -32002, Message: "missing different session", Data: map[string]string{"sessionId": "another-session"}}
			case "auth":
				rejection = sdk.NewAuthRequired(nil)
			case "writer":
				rejection = sdk.NewInvalidRequest(map[string]string{"reason": "thread_active_writer", "threadId": params.SessionID})
			case "config":
				rejection = sdk.NewInternalError(map[string]string{"details": "invalid workspace configuration"})
			case "unrelated-method":
				rejection = sdk.NewMethodNotFound("nested_method")
			case "transport":
				return nil
			default:
				return errors.New("restore was not advertised")
			}
			if rejection != nil {
				send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": rejection})
			} else {
				reply(m.ID, map[string]any{})
			}
		case "session/new":
			if reattaching {
				switch restore {
				case "unsupported":
				case "missing", "codex-missing", "both-missing":
					if !resumeTried || restore == "both-missing" && !loadTried {
						return errors.New("started a new session before restore methods settled")
					}
				default:
					return errors.New("unexpectedly fell back to a new session")
				}
			}
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
		case "session/cancel":
			if scenario == "cooperative-stop" {
				return errors.New("cancel arrived outside its active prompt")
			}
		case "session/prompt":
			if scenario == "cooperative-stop" {
				var prompt sdk.PromptRequest
				if err := json.Unmarshal(m.Params, &prompt); err != nil || prompt.SessionId != "session-1" || len(prompt.Prompt) != 1 || prompt.Prompt[0].Text == nil {
					return errors.New("invalid cooperative prompt")
				}
				switch text := prompt.Prompt[0].Text.Text; text {
				case "again":
					reply(m.ID, map[string]string{"stopReason": "end_turn"})
				case "reject":
					send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": &sdk.RequestError{Code: -32602, Message: "rejected by fixture"}})
				case "wait", "write", "idle", "idle-completes", "delayed-ack", "stall":
					permissionPending, writePending, cancelSeen := text == "wait", text == "write", false
					if permissionPending {
						send(map[string]any{"jsonrpc": "2.0", "id": "stop-permission", "method": "session/request_permission", "params": map[string]any{"sessionId": "session-1", "toolCall": map[string]string{"toolCallId": "stop-test"}, "options": []any{map[string]string{"optionId": "allow", "name": "Allow", "kind": "allow_once"}}}})
					}
					if writePending {
						send(map[string]any{"jsonrpc": "2.0", "id": "stop-write", "method": "fs/write_text_file", "params": map[string]string{"sessionId": "session-1", "path": filepath.Join(root, "staged.txt"), "content": "fixture"}})
					}
					for permissionPending || writePending || !cancelSeen {
						next, err := read()
						if err != nil {
							return err
						}
						switch {
						case next.Method == "session/cancel" && !cancelSeen:
							var cancel sdk.CancelNotification
							if err := json.Unmarshal(next.Params, &cancel); err != nil || cancel.SessionId != prompt.SessionId {
								return errors.New("cancel had the wrong session ID")
							}
							cancelSeen = true
						case string(next.ID) == `"stop-permission"` && strings.Contains(string(next.Result), `"cancelled"`):
							permissionPending = false
						case string(next.ID) == `"stop-write"` && strings.Contains(string(next.Error), `"code":-32800`):
							writePending = false
						default:
							return fmt.Errorf("unexpected message before prompt stop acknowledgement: %q", next.Method)
						}
					}
					if text == "wait" {
						for deadline := time.Now().Add(5 * time.Second); ; {
							if _, err := os.Stat(filepath.Join(root, "cancel-ack")); err == nil {
								break
							}
							if time.Now().After(deadline) {
								return errors.New("test did not release the cancel acknowledgement")
							}
							time.Sleep(time.Millisecond)
						}
					}
					if text == "idle-completes" {
						reply(m.ID, map[string]string{"stopReason": "end_turn"})
					} else if text != "stall" {
						if text == "delayed-ack" {
							time.Sleep(6*time.Second + 500*time.Millisecond)
						}
						reply(m.ID, map[string]string{"stopReason": "cancelled"})
					}
				default:
					return fmt.Errorf("prompt was unexpectedly written: %q", text)
				}
				continue
			}
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

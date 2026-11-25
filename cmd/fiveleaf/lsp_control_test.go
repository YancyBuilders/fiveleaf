package main

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fiveleaf/internal/lsp"
)

func init() {
	if marker := os.Getenv("FIVELEAF_TEST_BLOCKED_LSP"); marker != "" {
		os.WriteFile(marker, nil, 0o644)
		time.Sleep(time.Hour)
		os.Exit(0)
	}
}

func controlLSPPlugin(t *testing.T, hs *harness, resolve bool) *pluginHost {
	t.Helper()
	server := map[string]any{"id": "fake.server", "languages": []string{"fake"}}
	if resolve {
		server["resolve"] = map[string]any{"command": os.Args[0], "args": []string{"-test.run=^$"}}
	} else {
		t.Setenv("FIVELEAF_FAKE_LSP", "1")
		server["command"], server["args"] = os.Args[0], []string{"-test.run=^$"}
	}
	manifest, _ := json.Marshal(map[string]any{
		"name": "fake",
		"contributes": map[string]any{
			"languages":       []any{map[string]any{"id": "fake", "extensions": []string{".fake"}}},
			"languageServers": []any{server},
		},
	})
	h := withPlugins(hs, map[string]map[string]string{"fake": {"plugin.json": string(manifest)}})
	t.Cleanup(stopServers)
	return h
}

func controlLSPOpen(t *testing.T, hs *harness, name string) string {
	t.Helper()
	path := hs.path(name)
	if err := os.WriteFile(path, []byte("so bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	return path
}

func TestLanguageServerManualStopAndRestart(t *testing.T) {
	hs := newHarness(t, false)
	h := controlLSPPlugin(t, hs, false)
	path := controlLSPOpen(t, hs, "a.fake")
	hs.untilLong("the running server", func() bool { return len(h.diagnosticsOf(path)) == 1 })
	a, d := hs.a, hs.a.activeEditor()
	old := a.lspFor(d.document, false)
	a.stopLanguageServers("fake.server")
	if old.state() != lspStopped || old.live() != nil {
		t.Fatal("Stop did not take effect immediately")
	}
	a.lspDoc("change", d.document)
	a.lspDoc("save", d.document)
	other := controlLSPOpen(t, hs, "b.fake")
	hs.settle()
	if a.lspFor(d.document, true) != old || old.starts != 1 {
		t.Fatal("editing or opening a file restarted a stopped server")
	}
	hs.untilLong("the server's process to exit", func() bool {
		select {
		case <-old.client.Done():
			return true
		default:
			return false
		}
	})
	if len(h.diagnosticsOf(path)) != 0 {
		t.Fatal("stopped server diagnostics stayed visible")
	}
	a.restartLanguageServers("fake.server")
	hs.untilLong("the restarted server's open documents", func() bool {
		return len(h.diagnosticsOf(path)) == 1 && len(h.diagnosticsOf(other)) == 1
	})
	now := a.lspFor(d.document, false)
	if now == old || now.starts != 1 || now.owner == old.owner {
		t.Fatal("Restart did not create a fresh server generation")
	}
	old.clearDiagnostics()
	hs.settle()
	if len(h.diagnosticsOf(path)) != 1 {
		t.Fatal("old process cleanup cleared the new server's diagnostics")
	}

	a.settings.Plugins = maps.Clone(a.settings.Plugins)
	if a.settings.Plugins == nil {
		a.settings.Plugins = map[string]json.RawMessage{}
	}
	a.settings.Plugins["fake"] = json.RawMessage(`{"enabled":false}`)
	h.syncPrefs()
	a.restartLanguageServers()
	lspServers.Lock()
	unchanged := lspServers.m[a.root+"\x00fake.server"] == now
	lspServers.Unlock()
	if !unchanged {
		t.Fatal("Restart revived a disabled plugin")
	}
}

func TestLanguageServerStopWhileStarting(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "resolver-started")
	t.Setenv("FIVELEAF_TEST_BLOCKED_LSP", marker)
	hs := newHarness(t, false)
	controlLSPPlugin(t, hs, true)
	controlLSPOpen(t, hs, "a.fake")
	hs.untilLong("the resolver process", func() bool { _, err := os.Stat(marker); return err == nil })
	s := hs.a.lspFor(hs.a.activeEditor().document, false)
	if s.state() != lspStarting {
		t.Fatal("fixture is not starting")
	}
	started := time.Now()
	hs.a.stopLanguageServers()
	if s.state() != lspStopped {
		t.Fatal("starting server did not stop immediately")
	}
	select {
	case <-s.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("resolver was not cancelled")
	}
	hs.settle()
	if hs.a.fails != 0 || time.Since(started) > 3*time.Second {
		t.Fatalf("cancelled startup reported failure or waited for its timeout: failures=%d", hs.a.fails)
	}
}

func TestLanguageServerControlsStayInWorkspace(t *testing.T) {
	left := newHarness(t, false)
	h := controlLSPPlugin(t, left, false)
	right := newHarness(t, false)
	same := newHarness(t, false)
	same.a.openFolder(left.root)
	same.root = left.root
	h.apps = func() []*app { return []*app{left.a, right.a, same.a} }
	one := controlLSPOpen(t, left, "one.fake")
	two := controlLSPOpen(t, right, "two.fake")
	three := controlLSPOpen(t, same, "three.fake")
	left.untilLong("both workspaces' diagnostics", func() bool {
		right.settle()
		same.settle()
		return len(h.diagnosticsOf(one)) == 1 && len(h.diagnosticsOf(two)) == 1 && len(h.diagnosticsOf(three)) == 1
	})
	rightServer := right.a.lspFor(right.a.activeEditor().document, false)
	left.a.stopLanguageServers()
	left.untilLong("stopped workspace diagnostics to clear", func() bool {
		return len(h.diagnosticsOf(one)) == 0 && len(h.diagnosticsOf(three)) == 0
	})
	if len(h.diagnosticsOf(two)) != 1 || rightServer.state() != lspRunning {
		t.Fatal("Stop All affected another workspace")
	}
	if len(h.diagnosticsOf(one)) != 0 || len(h.diagnosticsOf(three)) != 0 {
		t.Fatal("Stop All did not clear the workspace's other window")
	}
	if got := left.a.lspStatuses(); !slices.Equal(got, []lspServerStatus{{ID: "fake.server", State: lspStopped}}) {
		t.Fatalf("stopped workspace statuses: %+v", got)
	}
	left.a.restartLanguageServers()
	left.untilLong("the restarted workspace's windows", func() bool {
		return len(h.diagnosticsOf(one)) == 1 && len(h.diagnosticsOf(three)) == 1
	})
	if right.a.lspFor(right.a.activeEditor().document, false) != rightServer || len(h.diagnosticsOf(two)) != 1 {
		t.Fatal("Restart All affected another workspace")
	}
}

// controlLSPMainGate models RunOnMain waiting for a busy UI thread. Its
// release posts held callbacks to the harness before letting callers return.
type controlLSPMainGate struct {
	mu      sync.Mutex
	held    bool
	entered chan struct{}
	pending []struct {
		fn   func()
		done chan struct{}
	}
	deliver func(func())
}

func controlLSPGate(t *testing.T, h *pluginHost, after func()) *controlLSPMainGate {
	t.Helper()
	post := h.onMain
	g := &controlLSPMainGate{entered: make(chan struct{}, 1)}
	g.deliver = func(fn func()) {
		post(func() {
			fn()
			if after != nil {
				after()
			}
		})
	}
	h.onMain = func(fn func()) {
		g.mu.Lock()
		if !g.held {
			g.deliver(fn)
			g.mu.Unlock()
			return
		}
		done := make(chan struct{})
		g.pending = append(g.pending, struct {
			fn   func()
			done chan struct{}
		}{fn, done})
		select {
		case g.entered <- struct{}{}:
		default:
		}
		g.mu.Unlock()
		<-done
	}
	t.Cleanup(g.release)
	return g
}

func (g *controlLSPMainGate) hold() {
	g.mu.Lock()
	g.held = true
	g.mu.Unlock()
}

func (g *controlLSPMainGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.held = false
	for _, call := range g.pending {
		g.deliver(call.fn)
		close(call.done)
	}
	g.pending = nil
}

func TestLanguageServerShutdownWhileMainThreadBlocked(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "resolver-started")
	t.Setenv("FIVELEAF_TEST_BLOCKED_LSP", marker)
	hs := newHarness(t, false)
	h := controlLSPPlugin(t, hs, true)
	gate := controlLSPGate(t, h, nil)
	controlLSPOpen(t, hs, "a.fake")
	hs.untilLong("the resolver process", func() bool { _, err := os.Stat(marker); return err == nil })
	s := hs.a.lspFor(hs.a.activeEditor().document, false)
	gate.hold()
	stopped := make(chan struct{})
	go func() {
		stopServers()
		close(stopped)
	}()
	defer func() {
		gate.release()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Error("shutdown did not finish after the UI was released")
		}
	}()
	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled startup did not report to the blocked UI")
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown waited for a UI callback while cancelling startup")
	}
	select {
	case <-s.ready:
	default:
		t.Fatal("shutdown completed before startup published its result")
	}
	gate.release()
	drained := make(chan struct{})
	s.post(func() { close(drained) })
	hs.untilLong("the cancelled server's UI updates", func() bool {
		select {
		case <-drained:
			return true
		default:
			return false
		}
	})
	if hs.a.fails != 0 {
		t.Fatal("cancelled startup reported a failure")
	}
}

func TestLanguageServerReadsWhileDiagnosticsMainThreadBlocked(t *testing.T) {
	hs := newHarness(t, false)
	h := controlLSPPlugin(t, hs, false)
	path := hs.path("a.fake")
	var observed []int
	gate := controlLSPGate(t, h, func() {
		line := -1
		if ds := h.diagnosticsOf(path); len(ds) > 0 {
			line = ds[0].Range.Start.Line
		}
		if len(observed) == 0 || observed[len(observed)-1] != line {
			observed = append(observed, line)
		}
	})
	controlLSPOpen(t, hs, "a.fake")
	hs.untilLong("the running server", func() bool { return len(h.diagnosticsOf(path)) == 1 })
	s := hs.a.lspFor(hs.a.activeEditor().document, false)
	c := s.live()
	idle := make(chan struct{})
	s.post(func() { close(idle) })
	hs.untilLong("the initial server notifications", func() bool {
		select {
		case <-idle:
			return true
		default:
			return false
		}
	})
	observed = nil
	gate.hold()
	c.Open(serverURI(path), "fake", "clean\n")
	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("the diagnostics callback did not reach the blocked UI")
	}
	c.Open(serverURI(path), "fake", "clean\nbad\n")
	c.Open(serverURI(path), "fake", "clean\nclean\nbad\n")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hover, err := c.HoverAt(ctx, serverURI(path), lsp.Position{})
	if err != nil || !strings.Contains(hover, "hello") {
		t.Fatalf("protocol reader waited for the diagnostics UI callback: hover=%q, err=%v", hover, err)
	}
	// The hover response follows all three notifications on the wire, so a
	// final UI marker is queued after every diagnostic even while UI is held.
	drained := make(chan struct{})
	s.post(func() { close(drained) })
	gate.release()
	hs.untilLong("the queued diagnostics", func() bool {
		select {
		case <-drained:
			return true
		default:
			return false
		}
	})
	if !slices.Equal(observed, []int{-1, 1, 2}) {
		t.Fatalf("diagnostic callbacks were lost or reordered: %v", observed)
	}
}

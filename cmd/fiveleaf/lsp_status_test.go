package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/lsp"
	"fiveleaf/internal/plugin"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

// lspStatusFixture gives a menu a state without starting an installed plugin.
// The running state uses the test server so its client has a real lifetime.
func lspStatusFixture(t testing.TB, root, id string, state lspState) *lspServer {
	t.Helper()
	s := &lspServer{root: root, contrib: plugin.ServerContrib{ID: id}, ready: make(chan struct{})}
	switch state {
	case lspRunning:
		command := exec.Command(os.Args[0], "-test.run=^$")
		command.Env = append(os.Environ(), "FIVELEAF_FAKE_LSP=1")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := lsp.Start(ctx, lsp.Options{Cmd: command, RootURI: lsp.URI(root)})
		if err != nil {
			t.Fatal(err)
		}
		s.client = client
		t.Cleanup(s.close)
		close(s.ready)
	case lspFailed:
		s.err = errors.New("Language server executable was not found: missing-server")
		close(s.ready)
	case lspStopped:
		s.stopped.Store(true)
		close(s.ready)
	}
	key := root + "\x00" + id
	lspServers.Lock()
	lspServers.m[key] = s
	lspServers.Unlock()
	t.Cleanup(func() {
		lspServers.Lock()
		if lspServers.m[key] == s {
			delete(lspServers.m, key)
		}
		lspServers.Unlock()
	})
	return s
}

func openLSPStatus(hs *harness) {
	hs.t.Helper()
	hs.settle()
	// With no server in the workspace the status bar leaves the control
	// out, so the command opens the menu as it does from the palette.
	hs.a.run("plugins.languageServers")
	hs.settle()
	if !hs.a.lspMenu.open {
		hs.t.Fatal("the language server button did not open its menu")
	}
}

// A workspace without a language server leaves its status bar control
// out, and shows it again with the first server.
func TestLSPStatusHiddenWithoutServers(t *testing.T) {
	hs := newHarness(t, false)
	hs.settle()
	if _, shown := hs.tt.Find(locales.PluginsLanguageServers()); shown {
		t.Fatal("the status bar shows a language server control with no server")
	}
	lspStatusFixture(t, hs.root, "gopls", lspStopped)
	hs.settle()
	if _, shown := hs.tt.Find(locales.PluginsLanguageServers()); !shown {
		t.Fatal("the status bar keeps the language server control from showing")
	}
}

func TestLSPStatusEmpty(t *testing.T) {
	for _, folder := range []bool{false, true} {
		name := "no-folder"
		if folder {
			name = "empty-workspace"
		}
		t.Run(name, func(t *testing.T) {
			var hs *harness
			message := locales.LspNoServers()
			if folder {
				hs = newHarness(t, false)
			} else {
				s := settings.Default()
				host := &fakeHost{}
				a := newApp(host, s, s.NewWindow())
				t.Cleanup(a.shutdown)
				hs = &harness{t: t, a: a, h: host, tt: ui.NewTester(a.view, 1200, 760)}
				message = locales.LspNoWorkspace()
			}
			openLSPStatus(hs)
			if !hs.tt.HasText(message) {
				t.Fatalf("the empty menu does not explain its state: %q", message)
			}
			for _, label := range []string{locales.LspRestartAll(), locales.LspStopAll()} {
				if err := hs.tt.Click(label); err != nil {
					t.Fatal(err)
				}
				hs.settle()
				if !hs.a.lspMenu.open {
					t.Fatalf("disabled %q executed and closed the empty menu", label)
				}
			}
			hs.key(0, ui.KeyEscape)
			if hs.a.lspMenu.open {
				t.Fatal("Escape did not close the empty menu")
			}
		})
	}
}

func TestLSPStatusServerMenu(t *testing.T) {
	hs := newHarness(t, false)
	lspStatusFixture(t, hs.root, "alpha", lspStarting)
	failed := lspStatusFixture(t, hs.root, "beta", lspFailed)
	lspStatusFixture(t, hs.root, "gamma", lspStopped)
	lspStatusFixture(t, t.TempDir(), "other-workspace", lspStarting)
	openLSPStatus(hs)
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if !hs.tt.HasText(id) {
			t.Fatalf("the workspace's server %q is missing", id)
		}
	}
	if hs.tt.HasText("other-workspace") {
		t.Fatal("the menu includes a server from another workspace")
	}
	if err := hs.tt.Click("beta"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.a.lspMenu.subOpen || !hs.tt.HasText(locales.LspStateFailed()) || !hs.tt.HasText(failed.err.Error()) {
		t.Fatal("the server submenu does not show its failure and reason")
	}
	if err := hs.tt.Click(locales.LspStopServer()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.a.lspMenu.subOpen || failed.state() != lspFailed {
		t.Fatal("Stop ran for a server that is already failed")
	}
	hs.key(0, ui.KeyEscape)
	if hs.a.lspMenu.subOpen || !hs.a.lspMenu.open {
		t.Fatal("Escape should close the server submenu before its parent")
	}
	hs.key(0, ui.KeyEscape)
	if hs.a.lspMenu.open {
		t.Fatal("a second Escape did not close the language server menu")
	}
}

func TestLSPStatusKeyboardNavigation(t *testing.T) {
	hs := newHarness(t, false)
	lspStatusFixture(t, hs.root, "alpha", lspStarting)
	lspStatusFixture(t, hs.root, "beta", lspStopped)
	openLSPStatus(hs)
	if !hs.tt.Focused("alpha") {
		t.Fatal("opening the menu did not focus its first server")
	}
	hs.key(0, ui.KeyDown)
	if !hs.tt.Focused("beta") {
		t.Fatal("Down did not focus the next server")
	}
	hs.key(0, ui.KeyRight)
	if !hs.a.lspMenu.subOpen || hs.a.lspMenu.selected != "beta" || !hs.tt.Focused(locales.LspRestartServer()) {
		t.Fatal("Right did not open and focus the selected server's actions")
	}
	hs.key(0, ui.KeyLeft)
	if hs.a.lspMenu.subOpen || !hs.a.lspMenu.open || !hs.tt.Focused("beta") {
		t.Fatal("Left did not return focus to the selected server")
	}
	hs.key(0, ui.KeyUp)
	if !hs.tt.Focused("alpha") {
		t.Fatal("Up did not focus the previous server")
	}
	hs.key(0, ui.KeyRight)
	hs.key(0, ui.KeyEscape)
	if hs.a.lspMenu.subOpen || !hs.a.lspMenu.open || !hs.tt.Focused("alpha") {
		t.Fatal("Escape did not return focus to the selected server")
	}
	hs.key(0, ui.KeyEscape)
	if hs.a.lspMenu.open {
		t.Fatal("Escape did not close the main menu")
	}
}

func TestLSPStatusStopActions(t *testing.T) {
	hs := newHarness(t, false)
	alpha := lspStatusFixture(t, hs.root, "alpha", lspRunning)
	beta := lspStatusFixture(t, hs.root, "beta", lspRunning)
	failed := lspStatusFixture(t, hs.root, "failed", lspFailed)
	other := lspStatusFixture(t, t.TempDir(), "other", lspRunning)
	openLSPStatus(hs)
	if err := hs.tt.Click("alpha"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if err := hs.tt.Click(locales.LspStopServer()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.lspMenu.open || alpha.state() != lspStopped || beta.state() != lspRunning || other.state() != lspRunning {
		t.Fatal("Stop Server did not close the menu and stop only the selected server")
	}
	openLSPStatus(hs)
	if err := hs.tt.Click(locales.LspStopAll()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.lspMenu.open || beta.state() != lspStopped || other.state() != lspRunning {
		t.Fatal("Stop All did not close the menu and stop only this workspace's servers")
	}
	openLSPStatus(hs)
	if err := hs.tt.Click(locales.LspStopAll()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.a.lspMenu.open {
		t.Fatal("Stop All stayed enabled after all workspace servers stopped")
	}
	if err := hs.tt.Click("failed"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.a.lspMenu.subOpen || !hs.tt.HasText(locales.LspStopped()) || !hs.tt.HasText(failed.err.Error()) {
		t.Fatal("Stop All hid a failed server's error from its stopped submenu")
	}
}

func TestLSPStatusFolderChange(t *testing.T) {
	hs := newHarness(t, false)
	next := workspaceFixture(t)
	lspStatusFixture(t, hs.root, "old-server", lspFailed)
	lspStatusFixture(t, next, "new-server", lspStopped)
	openLSPStatus(hs)
	if err := hs.tt.Click("old-server"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.a.openFolder(next)
	hs.settle()
	if hs.a.lspMenu.open || hs.a.lspMenu.subOpen {
		t.Fatal("changing folders left the old workspace's menu open")
	}
	openLSPStatus(hs)
	if !hs.tt.HasText("new-server") || hs.tt.HasText("old-server") {
		t.Fatal("the reopened menu does not show only the new workspace's servers")
	}
	hs.tt.ClickAt(900, 300)
	hs.settle()
	if hs.a.lspMenu.open {
		t.Fatal("clicking outside the menu did not close it")
	}
}

func TestLSPStatusUnavailableRestart(t *testing.T) {
	hs := newHarness(t, false)
	failed := lspStatusFixture(t, hs.root, "missing-plugin", lspFailed)
	openLSPStatus(hs)
	if err := hs.tt.Click("missing-plugin"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	fails := hs.a.fails
	if err := hs.tt.Click(locales.LspRestartServer()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.fails != fails+1 || !hs.tt.HasText(locales.LspRestartUnavailable()) {
		t.Fatal("an unavailable restart did not explain how to enable the server")
	}
	if !hs.a.lspMenu.open || !hs.a.lspMenu.subOpen || !hs.tt.HasText(failed.err.Error()) {
		t.Fatal("an unavailable restart dismissed the server's menu or error")
	}
}

func TestLSPStatusLayout(t *testing.T) {
	hs := newHarness(t, false)
	hs.a.openFile(hs.path("main.go"), true)
	lspStatusFixture(t, hs.root, "test-server", lspStopped)
	hs.settle()
	for _, size := range []struct{ width, height float32 }{{1200, 760}, {300, 400}} {
		hs.tt.SetSize(int(size.width), int(size.height))
		for _, right := range []bool{false, true} {
			hs.a.settings.SidebarRight = right
			hs.a.settings.TerminalButtonRight = right
			hs.settle()
			before, found := hs.tt.Find(locales.PluginsLanguageServers())
			if !found {
				t.Fatal("the language server control is missing")
			}
			for _, state := range []lspState{lspStarting, lspFailed, lspStopped, lspRunning} {
				lspStatusFixture(t, hs.root, "test-server", state)
				hs.settle()
				bounds, found := hs.tt.Find(locales.PluginsLanguageServers())
				if !found || bounds != before {
					t.Errorf("server state %v moved the status control: %v, want %v", state, bounds, before)
				}
				if bounds.X < 0 || bounds.X+bounds.W > size.width || bounds.Y < size.height-statusBarH || bounds.Y+bounds.H > size.height {
					t.Errorf("server status is outside the bottom bar at %v × %v: %v", size.width, size.height, bounds)
				}
				position, found := hs.tt.Find(locales.StatusPos(1, 1))
				if !found || bounds.X+bounds.W > position.X {
					t.Errorf("server status overlaps the caret position: %v, %v", bounds, position)
				}
				checkStatusControls(t, hs, size.width, size.height)
			}
		}
	}
}

func TestLSPStatusLongFailure(t *testing.T) {
	hs := newHarness(t, false)
	hs.tt.SetSize(360, 400)
	failed := lspStatusFixture(t, hs.root, "rust-analyzer", lspFailed)
	failed.err = errors.New(strings.Repeat("The language server could not load the workspace configuration.\n", 40))
	openLSPStatus(hs)
	if err := hs.tt.Click("rust-analyzer"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText("could not load the workspace configuration") {
		t.Fatal("the failure menu lost the server's error")
	}
	for _, label := range []string{locales.LspRestartServer(), locales.LspStopServer()} {
		bounds, found := hs.tt.Find(label)
		if !found || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > 360 || bounds.Y+bounds.H > 400 {
			t.Errorf("long server error pushed %q outside the window: %v", label, bounds)
		}
	}
}

// LSP_STATUS_SHOT captures only this surface, without running the full gallery.
func TestLSPStatusShots(t *testing.T) {
	dir := os.Getenv("LSP_STATUS_SHOT")
	if dir == "" {
		t.Skip("LSP_STATUS_SHOT is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHOTS", dir)
	hs := newHarness(t, false)
	hs.a.openFile(hs.path("main.go"), true)
	lspStatusFixture(t, hs.root, "gopls", lspRunning)
	lspStatusFixture(t, hs.root, "python", lspStarting)
	lspStatusFixture(t, hs.root, "rust-analyzer", lspFailed)
	lspStatusFixture(t, hs.root, "typescript-language-server", lspStopped)
	for _, dark := range []bool{false, true} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		hs.tt.SetDark(dark)
		openLSPStatus(hs)
		hs.shot("lsp-status-" + mode)
		if err := hs.tt.Click("rust-analyzer"); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		hs.shot("lsp-status-error-" + mode)
		hs.key(0, ui.KeyEscape)
		hs.key(0, ui.KeyEscape)
	}
	t.Logf("LSP status screenshots: %s", filepath.Clean(dir))
}

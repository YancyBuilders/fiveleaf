package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func testPanelSession(t *testing.T, a *app) *session {
	t.Helper()
	conn, peer := net.Pipe()
	t.Cleanup(func() { peer.Close() })
	term, err := terminal.New(terminal.Options{Conn: conn})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	a.nextTerm++
	shell := &session{id: a.nextTerm, dir: a.root, term: term}
	a.terms = append(a.terms, shell)
	a.panelTerm = shell
	return shell
}

func TestPanelCloseDeletesOnlySelectedTerminal(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	file := a.activeTab()
	first, second := testPanelSession(t, a), testPanelSession(t, a)
	first.title, second.title = "build", "shell"
	first.started = time.Now()
	a.showTerminal(first)
	hs.settle()
	if err := hs.tt.Click("Close Terminal"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if len(a.terms) != 1 || a.panelTerm != second || !a.window.PanelOpen || !hs.tt.Focused("Terminal") || a.activeTab() != file {
		t.Fatal("the panel close button didn't delete only the selected session and focus its neighbor")
	}
	notes := len(a.notes)
	a.terminalExited(first, first.term, 0)
	a.terminalToPanel(first)
	if len(a.notes) != notes || a.panelTerm != second {
		t.Fatal("a late event from the closed terminal affected the remaining session")
	}
	a.run("file.close")
	hs.settle()
	if len(a.terms) != 0 || a.window.PanelOpen || a.window.TerminalOpen || a.panelTerm != nil || a.activeTab() != file || len(a.closed) != 0 {
		t.Fatal("Close Tab with the panel focused didn't delete its last terminal and leave the editor intact")
	}
}

func TestTogglePanelReusesEditorTerminals(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	t.Cleanup(a.closeTerminals)
	a.openFile(hs.path("main.go"), true)
	first, second := testPanelSession(t, a), testPanelSession(t, a)
	a.showTerminal(first)
	a.maximizePanel()
	a.run("view.terminal")
	hs.settle()
	if a.panelTerm != first || !a.window.PanelOpen || first.tab != nil || second.tab != nil || len(a.terms) != 2 || len(a.column().tabs) != 1 {
		t.Fatal("opening the dock with multiple editor terminals left both locations active or started a new shell")
	}
	a.maximizePanel()
	a.run("view.newTerminal")
	hs.settle()
	if a.window.PanelOpen || a.panelTerm != nil || len(a.terms) != 3 || a.terms[2].tab == nil || a.activeTab() != a.terms[2].tab {
		t.Fatal("a new terminal didn't join the editor terminals")
	}
}

func TestTerminalLocationIsWindowLocal(t *testing.T) {
	left := newHarness(t, false)
	other := newApp(&fakeHost{}, left.a.settings, left.a.settings.NewWindow())
	t.Cleanup(other.shutdown)
	right := &harness{t: t, a: other, tt: ui.NewTester(other.view, 960, 640)}
	right.settle()
	local, remote := testPanelSession(t, left.a), testPanelSession(t, right.a)
	left.a.showTerminal(local)
	right.a.showTerminal(remote)
	left.a.maximizePanel()
	left.settle()
	right.settle()
	if local.tab == nil || left.a.window.PanelOpen || remote.tab != nil || !right.a.window.PanelOpen || right.a.panelTerm != remote {
		t.Fatal("maximizing a terminal changed the other window's location")
	}
	right.a.terminalToPanel(local)
	left.a.removeTerminal(remote)
	right.a.closeActive()
	right.settle()
	left.settle()
	if len(right.a.terms) != 0 || right.a.window.PanelOpen || len(left.a.terms) != 1 || local.tab == nil || left.a.activeTab() != local.tab {
		t.Fatal("a close or late move in one window affected the other window's terminal")
	}
}

func TestHiddenTerminalExitKeepsEditorFocus(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	file := a.activeTab()
	first, second := testPanelSession(t, a), testPanelSession(t, a)
	a.showTerminal(first)
	a.hidePanel()
	hs.settle()
	a.removeTerminal(first)
	if a.window.PanelOpen || a.panelTerm != second || a.focus != paneEditor || a.activeTab() != file {
		t.Fatal("a hidden terminal exiting stole focus from the editor")
	}
}

// The scroll bar must not take clicks in the lower half of a terminal's
// chip, including after scrolling or resizing the panel.
func TestPanelTerminalChipsStayClickable(t *testing.T) {
	if testing.Short() {
		t.Skip("loads libghostty-vt")
	}
	const narrowWidth = 560
	for _, dark := range []bool{false, true} {
		for _, width := range []int{1000, 640, narrowWidth} {
			t.Run(fmt.Sprintf("dark=%v/width=%d", dark, width), func(t *testing.T) {
				hs := newHarness(t, dark)
				a := hs.a
				for i := range 2 {
					testPanelSession(t, a).title = fmt.Sprintf("terminal-session-%d-with-a-long-title", i+1)
				}
				first, second := a.terms[0], a.terms[1]
				// Render the real panel with the app's scroll bar width, with
				// no shell output or sidebar affecting the fixture's size.
				var terminalBox ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					a.pal = a.applyTheme(c)
					a.panel(c)
					terminalBox = a.panelTerm.view.Bounds(c)
				}, width, 280)
				tt.SetDark(dark)
				hs.tt = tt
				for range 5 {
					step(tt)
				}
				wantY := float32(panelHeadH + 2)
				if width == narrowWidth {
					wantY += 14
				}
				if terminalBox.Y != wantY {
					t.Errorf("terminal starts at %v, want %v; reserve space only for an overflowing strip", terminalBox.Y, wantY)
				}
				chip := func(s *session) ui.Rect {
					t.Helper()
					box, ok := tt.Find(s.title)
					if !ok {
						t.Fatalf("no chip for %q", s.title)
					}
					return box
				}
				click := func(s, other *session, fraction float32) {
					t.Helper()
					a.panelTerm = other
					step(tt)
					box := chip(s)
					tt.ClickAt(box.X+box.W/2, box.Y+box.H*fraction)
					step(tt)
					if a.panelTerm != s {
						t.Errorf("click at %.0f%% of chip height did not select %q", fraction*100, s.title)
					}
				}
				for _, fraction := range []float32{0.1, 0.5, 0.9} {
					click(first, second, fraction)
				}
				before := chip(first)
				secondX := chip(second).X
				x, y := before.X+before.W/2, before.Y+before.H/2
				tt.Move(x, y)
				tt.Scroll(x, y, 120, 0)
				step(tt)
				after := chip(first)
				hs.shot(fmt.Sprintf("terminal-chips-%v-%d", dark, width))
				if width == narrowWidth {
					if chip(second).X >= secondX {
						t.Fatal("overflowing chips did not scroll horizontally")
					}
					click(second, first, 0.9)
					// The separate bottom lane still takes scroll bar drags.
					tt.Scroll(x, y, -1000, 0)
					step(tt)
					before = chip(first)
					secondX = chip(second).X
					barY := float32(panelHeadH) + 7
					tt.Press(before.X+30, barY)
					tt.Move(before.X+90, barY)
					tt.Release(before.X+90, barY)
					step(tt)
					if chip(second).X >= secondX {
						t.Error("the scroll bar no longer drags the chips")
					}
					click(second, first, 0.5)
				} else if after.X != before.X {
					t.Error("chips scrolled when they fit in the header")
				}
				tt.SetSize(1000, 280)
				for range 3 {
					step(tt)
				}
				click(first, second, 0.9)
				if terminalBox.Y != panelHeadH+2 {
					t.Errorf("widening the panel left an empty scroll bar lane: terminal starts at %v", terminalBox.Y)
				}
				// Narrowing it again restores the lane before a click; titles
				// becoming shorter remove it without another window resize.
				tt.SetSize(narrowWidth, 280)
				for range 3 {
					step(tt)
				}
				if terminalBox.Y != panelHeadH+14+2 {
					t.Errorf("narrowing the panel did not reserve the scroll bar lane: terminal starts at %v", terminalBox.Y)
				}
				click(first, second, 0.9)
				first.title, second.title = "one", "two"
				for range 3 {
					step(tt)
				}
				if terminalBox.Y != panelHeadH+2 {
					t.Errorf("shorter titles left an empty scroll bar lane: terminal starts at %v", terminalBox.Y)
				}
				click(first, second, 0.9)
			})
		}
	}
}

// A terminal renamed keeps its name over the titles its programs give
// it, and an empty name shows those again.
func TestRenameTerminal(t *testing.T) {
	a := &app{}
	s := &session{title: "zsh"}
	a.askRenameTerminal(s)
	if a.prompt == nil || a.prompt.value != "zsh" {
		t.Fatalf("prompt = %+v", a.prompt)
	}
	if msg := a.prompt.done("  build  "); msg != "" {
		t.Fatal(msg)
	}
	s.title = "vim main.go"
	if got := s.label(); got != "build" {
		t.Fatalf("label = %q, want build", got)
	}
	a.askRenameTerminal(s)
	if msg := a.prompt.done(""); msg != "" {
		t.Fatal(msg)
	}
	if got := s.label(); got != "vim main.go" {
		t.Fatalf("label = %q, want vim main.go", got)
	}
}

// A shell that exits as it starts keeps its terminal, with what it printed
// and why it stopped, and offers to report it, rather than flash and close
// (#87).
func TestTerminalExitedEarlyStays(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a shell")
	}
	sh := filepath.Join(t.TempDir(), "broken-sh")
	os.WriteFile(sh, []byte("#!/bin/sh\necho 'rc: something is wrong'\nexit 3\n"), 0o755)
	t.Setenv("SHELL", sh)
	hs := newHarness(t, true)
	a := hs.a
	a.run("view.terminalTab")
	hs.settle()
	if len(a.terms) != 1 {
		t.Fatalf("%d terminals", len(a.terms))
	}
	s := a.terms[0]
	hs.until("the note of the exit", func() bool { return strings.Contains(s.term.Text(), "code 3") })
	if !strings.Contains(s.term.Text(), "rc: something is wrong") {
		t.Errorf("what the shell printed is gone: %q", s.term.Text())
	}
	if len(a.terms) != 1 || a.column().tab().term != s {
		t.Fatal("the terminal closed")
	}
	if !hs.tt.HasText(locales.NoteTermExitedEarly(shellName(), "3")) || !hs.tt.HasText(locales.ReportProblem()) {
		t.Fatalf("no toast offers to report it: %q", hs.tt.Texts())
	}
}

func TestReportURL(t *testing.T) {
	previous := repoURL
	t.Cleanup(func() { repoURL = previous })
	repoURL = ""
	if got := reportURL("zsh exited", "Exit code: 3"); got != "" {
		t.Fatalf("unconfigured repository opened %q", got)
	}
	repoURL = "https://github.com/example/FiveLeaf"
	u, err := url.Parse(reportURL("zsh exited", "Exit code: 3"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if !strings.HasPrefix(u.String(), repoURL+"/issues/new?") || q.Get("title") != "zsh exited" ||
		!strings.Contains(q.Get("body"), "Exit code: 3") || !strings.Contains(q.Get("body"), runtime.GOOS) {
		t.Errorf("report URL %s", u)
	}
}

package main

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/remote"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func terminalActionBounds(hs *harness, label string) ui.Rect {
	hs.t.Helper()
	box, ok := hs.tt.Find(label)
	if !ok || box.W <= 0 || box.H <= 0 {
		hs.t.Fatalf("terminal action %q is missing", label)
	}
	return box
}

func TestTerminalTabToolbarUsesItsOwnTerminal(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	t.Cleanup(a.closeTerminals)
	a.openFile(hs.path("main.go"), true)
	source := a.column()
	shell := testPanelSession(t, a)
	shell.title = "source shell"
	reported := hs.path("web")
	shell.term.Feed([]byte("\x1b]7;" + reported + "\a"))
	a.terminalToColumn(shell, source, source.active+1)
	other := a.splitPane(source, splitRight)
	a.openFileIn(other, 0, hs.path("go.mod"), true)
	hs.settle()
	plus := terminalActionBounds(hs, locales.CmdViewNewTerminal())
	split := splitButtonBounds(hs)
	if plus.X+plus.W > split.X || plus.Y != split.Y || a.column() != other {
		t.Fatal("expected New Terminal before Split in the unfocused terminal's toolbar")
	}
	hs.tt.ClickAt(plus.X+plus.W/2, plus.Y+plus.H/2)
	hs.settle()
	fresh := source.tab().term
	if len(a.terms) != 2 || len(source.tabs) != 3 || source.tabs[1] != shell.tab || fresh == nil || fresh == shell || fresh.dir != reported || !fresh.view.FocusWithin(a.review.ctx) || a.window.PanelOpen {
		t.Fatal("New Terminal did not start and focus a sibling of the clicked terminal")
	}
	if runtime.GOOS != "windows" {
		hs.tt.Type(`printf 'terminal-dir=%s\n' "${PWD##*/}"`)
		hs.tt.Key(0, ui.KeyEnter)
		hs.until("the new shell printing its directory", func() bool { return strings.Contains(fresh.term.Text(), "terminal-dir=web") })
	}
	a.activate(other, other.active, true)
	hs.settle()
	hoverSplitButton(hs)
	if err := hs.tt.Click(locales.CmdViewSplitDown()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	added := a.column()
	if len(a.cols) != 3 || added == source || added == other || added.tab().term == nil || added.tab().term.dir != fresh.dir || !added.tab().term.view.FocusWithin(a.review.ctx) || added.body.Bounds(a.review.ctx).Y <= source.body.Bounds(a.review.ctx).Y {
		t.Fatal("Split Down did not focus a new terminal below the clicked pane")
	}
}

func TestTerminalActionDirectoryStaysOnSourceHost(t *testing.T) {
	hs := newHarness(t, false)
	source := testPanelSession(t, hs.a)
	for _, reported := range []string{"relative", hs.path("missing"), hs.path("main.go"), remote.Path("other.invalid", "/srv")} {
		source.term.Feed([]byte("\x1b]7;" + reported + "\a"))
		if dir := source.nextShellDir(); dir != source.dir {
			t.Fatalf("local terminal accepted %q as its directory: %q", reported, dir)
		}
	}
	source.dir = remote.Path("source.invalid", "/project")
	source.term.Feed([]byte("\x1b]7;" + hs.path("web") + "\a"))
	if dir := source.nextShellDir(); dir != source.dir {
		t.Fatalf("remote terminal changed hosts when its reported directory exists locally: %q", dir)
	}
}

func TestDockedTerminalToolbarSplitsInPlace(t *testing.T) {
	hs := newHarness(t, false)
	a, tt := hs.a, hs.tt
	t.Cleanup(a.closeTerminals)
	if runtime.GOOS != "windows" {
		t.Setenv("SHELL", "/bin/sh")
	}
	a.openFile(hs.path("main.go"), true)
	col, file := a.column(), a.activeTab()
	source, other := testPanelSession(t, a), testPanelSession(t, a)
	source.title, other.title = "selected shell", "other shell"
	source.term.Feed([]byte("before split\r\n\x1b]7;" + hs.path("web") + "\a"))
	a.showTerminal(source)
	hs.settle()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	bounds := func(shell *session) ui.Rect { return shell.view.Bounds(a.review.ctx) }
	fileBox, full := col.body.Bounds(a.review.ctx), bounds(source)
	plusLabel := a.withKey(locales.CmdViewNewTerminal(), "view.newTerminal")
	plus, split := terminalActionBounds(hs, plusLabel), terminalActionBounds(hs, locales.CmdViewSplitRight())
	if plus.Y != split.Y || plus.X+plus.W > split.X {
		t.Fatal("expected New Terminal before Split in the dock header")
	}
	click(locales.CmdViewSplitRight())
	if len(a.cols) != 1 || col.tab() != file || col.body.Bounds(a.review.ctx) != fileBox || len(a.panelTerms()) != 3 || !a.window.PanelOpen || !a.window.TerminalOpen {
		t.Fatal("the split moved a docked terminal or displaced the editor")
	}
	right := a.panelTerm
	right.name = "right shell"
	leftBox, rightBox := bounds(source), bounds(right)
	if right == source || right == other || right.dir != hs.path("web") || !right.view.FocusWithin(a.review.ctx) || rightBox.X < leftBox.X+leftBox.W || leftBox.H != rightBox.H || leftBox.W <= 0 || rightBox.W <= 0 {
		t.Fatalf("split did not show and focus a shell to the right: %v, %v", leftBox, rightBox)
	}
	if runtime.GOOS != "windows" {
		tt.Type(`printf 'docked-dir=%s\n' "${PWD##*/}"`)
		tt.Key(0, ui.KeyEnter)
		hs.until("the split PTY printing its directory", func() bool { return strings.Contains(right.term.Text(), "docked-dir=web") })
	}
	tt.ClickAt(leftBox.X+leftBox.W/2, leftBox.Y+leftBox.H/2)
	hs.settle()
	if a.panelTerm != source || !source.view.FocusWithin(a.review.ctx) || a.findTerminal() != source {
		t.Fatal("clicking the left shell did not target it for panel actions")
	}
	tt.Move(split.X+split.W/2, split.Y+split.H/2)
	tt.Advance(hoverDwell + motionShow)
	tt.Frame()
	click(locales.CmdViewSplitUp())
	upper := a.panelTerm
	if upper == source || upper == right || bounds(upper).Y >= bounds(source).Y || bounds(upper).X != leftBox.X || bounds(right) != rightBox || !upper.view.FocusWithin(a.review.ctx) {
		t.Fatal("Split Up did not split just the selected half of the dock")
	}
	click(plusLabel)
	standalone := a.panelTerm
	if len(a.panelTerms()) != 5 || bounds(standalone) != full || standalone.dir != upper.dir || !standalone.view.FocusWithin(a.review.ctx) {
		t.Fatal("New Terminal did not open a focused, full-size dock tab")
	}
	click(other.title)
	if a.panelTerm != other || bounds(other) != full || !other.view.FocusWithin(a.review.ctx) {
		t.Fatal("selecting an unrelated terminal did not show its own tab")
	}
	click(right.name)
	if a.panelTerm != right || bounds(right) != rightBox || !right.view.FocusWithin(a.review.ctx) {
		t.Fatal("selecting a split member did not bring back its group and focus it")
	}
	click(locales.PanelMaximize())
	if a.window.PanelOpen || a.activeTab() != right.tab || len(a.panelTerms()) != 0 || !right.view.FocusWithin(a.review.ctx) {
		t.Fatal("maximize did not move the whole window's terminals and focus the selected one")
	}
	click(locales.PanelRestore())
	if a.panelTerm != right || len(a.panelTerms()) != 5 || col.tab() != file || bounds(right) != rightBox || !right.view.FocusWithin(a.review.ctx) {
		t.Fatal("restore did not return the dock group and file with the selected shell focused")
	}
	click(source.title)
	click(locales.TermClose())
	if a.panelTerm != upper || !upper.view.FocusWithin(a.review.ctx) || bounds(upper) != leftBox || bounds(right) != rightBox || !slices.Equal(a.panelTerms(), []*session{other, right, upper, standalone}) {
		t.Fatal("closing a nested pane did not focus its surviving sibling and recover its space")
	}
	click(locales.TermClose())
	if a.panelTerm != right || bounds(right) != full || !right.view.FocusWithin(a.review.ctx) || col.tab() != file {
		t.Fatal("closing the split did not restore the surviving terminal's full dock")
	}
}

func TestDockedTerminalSplitFailureKeepsDockAndFocus(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	file := a.activeTab()
	other, source := testPanelSession(t, a), testPanelSession(t, a)
	source.dir = remote.Path("terminal-split.invalid", "/project")
	a.showTerminal(source)
	hs.settle()
	fails := a.fails
	t.Setenv("PATH", t.TempDir())
	split := terminalActionBounds(hs, locales.CmdViewSplitRight())
	hs.tt.ClickAt(split.X+split.W/2, split.Y+split.H/2)
	hs.settle()
	if a.fails != fails+1 || !strings.Contains(strings.Join(hs.tt.Texts(), "\n"), "ssh") {
		t.Fatal("the unavailable remote shell was not reported")
	}
	if len(a.cols) != 1 || a.activeTab() != file || !slices.Equal(a.panelTerms(), []*session{other, source}) || a.panelTerm != source || !a.window.PanelOpen || !a.window.TerminalOpen || !source.view.FocusWithin(a.review.ctx) {
		t.Fatal("a failed split changed the layout, docked sessions or keyboard focus")
	}
}

func TestDockedTerminalDividerDragAndClose(t *testing.T) {
	for name, direction := range map[string]splitDirection{"width": splitRight, "height": splitDown} {
		t.Run(name, func(t *testing.T) {
			hs := newHarness(t, false)
			a := hs.a
			t.Cleanup(a.closeTerminals)
			source := testPanelSession(t, a)
			a.showTerminal(source)
			a.window.PanelHeight = 360
			hs.settle()
			full := source.view.Bounds(a.review.ctx)
			a.splitPanelTerminal(source, direction)
			hs.settle()
			fresh := a.panelTerm
			before := source.view.Bounds(a.review.ctx)
			posX, posY := before.X+before.W/2, before.Y+before.H/2
			if direction.vertical() {
				posY = before.Y + before.H + 6
				hs.drag(posX, posY, posX, full.Y+full.H)
			} else {
				posX = before.X + before.W + 6
				hs.drag(posX, posY, full.X+full.W, posY)
			}
			after, remaining := source.view.Bounds(a.review.ctx), fresh.view.Bounds(a.review.ctx)
			if direction.vertical() {
				if after.H <= before.H+20 || remaining.H < 40 || remaining.H > 50 || after.W != before.W {
					t.Fatalf("drag outside vertical divider: %v -> %v, remaining %v", before, after, remaining)
				}
			} else if after.W <= before.W+20 || remaining.W < minPanelW-20 || remaining.W > minPanelW || after.H != before.H {
				t.Fatalf("drag outside horizontal divider: %v -> %v, remaining %v", before, after, remaining)
			}
			if a.panelTerm != fresh || !fresh.view.FocusWithin(a.review.ctx) {
				t.Fatal("dragging the divider stole focus from the selected terminal")
			}
			if err := hs.tt.Click(locales.TermClose()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if len(a.terms) != 1 || a.panelTerm != source || source.view.Bounds(a.review.ctx) != full || !source.view.FocusWithin(a.review.ctx) {
				t.Fatal("closing the resized pane did not give its space and focus to its sibling")
			}
		})
	}
}

func TestDockedTerminalPTYStartFailureRestoresSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the Unix SHELL override")
	}
	hs := newHarness(t, false)
	a := hs.a
	t.Cleanup(a.closeTerminals)
	a.openFile(hs.path("main.go"), true)
	file := a.activeTab()
	source := testPanelSession(t, a)
	a.showTerminal(source)
	hs.settle()
	full, fails := source.view.Bounds(a.review.ctx), a.fails
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "missing-shell"))
	if err := hs.tt.Click(locales.CmdViewSplitRight()); err != nil {
		t.Fatal(err)
	}
	hs.until("the PTY start failure", func() bool { return a.fails != fails })
	hs.settle()
	if a.panelTerm != source || len(a.terms) != 1 || a.activeTab() != file || source.view.Bounds(a.review.ctx) != full || !source.view.FocusWithin(a.review.ctx) {
		t.Fatal("a failed PTY launch left a split or stole the original terminal's focus")
	}
}

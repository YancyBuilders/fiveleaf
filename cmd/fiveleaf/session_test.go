package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
)

// allTabNames are the names of the tabs of every column, a column at a time.
func allTabNames(a *app) []string {
	var out []string
	for _, col := range a.cols {
		for _, t := range col.tabs {
			out = append(out, t.name())
		}
	}
	return out
}

// Switching a window's folder keeps the tabs of the one it had, where
// they were, and the folders open in its explorer, which come back as it
// opens that folder again.
func TestSwitchFolderSwapsSessions(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	s := settings.Default()
	a := newApp(&fakeHost{}, s, s.NewWindow())
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	step(tt)
	a.openFolder(root)
	a.openFile(filepath.Join(root, "main.go"), true)
	a.openFile(filepath.Join(root, "README.md"), true)
	step(tt)
	a.activeEditor().pending = &[2]int{2, 6}
	a.files.Open.Add(filepath.Join(root, "internal"))
	step(tt)
	step(tt)

	a.switchFolder(other)
	step(tt)
	if a.root != other || len(allTabNames(a)) != 0 {
		t.Fatalf("switched: folder %q, tabs %v", a.root, allTabNames(a))
	}
	ses := s.Sessions[root]
	if ses == nil || len(ses.Columns) != 1 || len(ses.Columns[0].Tabs) != 2 || !slices.Equal(ses.Expanded, []string{"internal"}) {
		t.Fatalf("kept %+v", ses)
	}
	a.openFile(filepath.Join(other, "go.mod"), true)
	step(tt)

	a.switchFolder(root)
	step(tt)
	step(tt)
	if a.root != root || !slices.Equal(allTabNames(a), []string{"main.go", "README.md"}) || a.activeEditor().name() != "README.md" {
		t.Fatalf("back: folder %q, tabs %v", a.root, allTabNames(a))
	}
	if sel := a.activeEditor().sel; sel != [2]int{2, 6} {
		t.Errorf("selection %v", sel)
	}
	if !a.files.Open.Has(filepath.Join(root, "internal")) {
		t.Error("the explorer's folder is closed")
	}
	if s.Sessions[root] != nil || s.Sessions[other] == nil || s.Sessions[other].Columns[0].Tabs[0].File != filepath.Join(other, "go.mod") {
		t.Errorf("sessions %v", s.Sessions)
	}
}

func TestSwitchFolderKeepsOneTerminal(t *testing.T) {
	hs := newHarness(t, true)
	application := hs.a
	application.openFile(hs.path("main.go"), true)
	shell := testPanelSession(t, application)
	application.terminalToColumn(shell, application.column(), application.column().active+1)
	other := workspaceFixture(t)
	for _, folder := range []string{other, hs.root} {
		application.switchFolder(folder)
		hs.settle()
		if application.root != folder || len(application.terms) != 1 || application.terms[0] != shell {
			t.Fatalf("switched to %q with %d terminals; want %q with the original shell", application.root, len(application.terms), folder)
		}
		if column, _ := application.locate(shell.tab); column == nil {
			t.Fatal("the original terminal lost its tab")
		}
	}
	if !slices.Contains(allTabNames(application), "main.go") {
		t.Fatal("the file tab did not return with its folder")
	}
}

func TestLegacyTerminalRestoresFreshShell(t *testing.T) {
	taskPTY(t)
	bin := t.TempDir()
	program, log := filepath.Join(bin, "hc"), filepath.Join(bin, "calls")
	if err := os.WriteFile(program, []byte("#!/bin/sh\necho called >> \"$FIVELEAF_TEST_HC_LOG\"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIVELEAF_TEST_HC_LOG", log)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	root := workspaceFixture(t)
	for _, sample := range []struct {
		name, folder string
		panel        bool
	}{
		{"addresses-only", root, false},
		{"open-panel", root, true},
		{"without-folder", "", false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			dir := t.TempDir()
			prefs := fmt.Sprintf(`{"terminalBackend":"hc","terminalBackendHcPath":%q}`, program)
			state := fmt.Sprintf(`{"windows":[{"folder":%q,"panelOpen":true,"terminalOpen":%t,"panelTerms":["host/t2","t3"],"panelTerm":1,"columns":[{"tabs":[{"terminal":true,"hc":"host/t1","agent":"codex","session":"old-session"}]}]}]}`, sample.folder, sample.panel)
			for name, data := range map[string]string{"settings.json": prefs, "state.json": state} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			loaded := settings.Load(filepath.Join(dir, "settings.json"))
			t.Cleanup(loaded.Flush)
			application := newApp(&fakeHost{}, loaded, loaded.Windows[0])
			t.Cleanup(application.shutdown)
			t.Cleanup(application.closeTerminals)
			hs := &harness{t: t, a: application, tt: ui.NewTester(application.view, 1200, 760)}
			hs.settle()
			if len(application.terms) != 1 || application.panelTerm != nil || application.window.PanelOpen || application.window.TerminalOpen || application.activeTab() == nil {
				t.Fatalf("restored %d terminals, panel open %v; want only the saved terminal tab", len(application.terms), application.window.PanelOpen)
			}
			shell := application.activeTab().term
			wantDir := sample.folder
			if wantDir == "" {
				wantDir = os.Getenv("HOME")
			}
			if shell == nil || shell.dir != wantDir || shell.resumed != "" || shell.agent != "" {
				t.Fatalf("expected a fresh shell in %q without the old agent or session", wantDir)
			}
			shell.term.Send([]byte("printf 'fresh:%s\\n' \"$((39+3))\"\r"))
			hs.until("output from the fresh shell", func() bool { return strings.Contains(shell.term.Text(), "fresh:42") })
			application.showSessions()
			hs.settle()
			if !application.sessions.loaded || len(application.sessions.open) != 0 {
				t.Fatal("Sessions did not open with a clean set of live terminals")
			}
			application.removeTerminal(shell)
			hs.settle()
			if _, err := os.Stat(log); !os.IsNotExist(err) {
				t.Fatalf("obsolete terminal backend was invoked, or its log could not be checked: %v", err)
			}
		})
	}
}

// Closing the folder keeps its tabs, which come back as it opens again.
func TestCloseFolderKeepsSession(t *testing.T) {
	root := workspaceFixture(t)
	s := settings.Default()
	a := newApp(&fakeHost{}, s, s.NewWindow())
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	step(tt)
	a.openFolder(root)
	a.openFile(filepath.Join(root, "go.mod"), true)
	step(tt)
	a.run("file.closeFolder")
	step(tt)
	if a.root != "" || len(allTabNames(a)) != 0 || s.Sessions[root] == nil {
		t.Fatalf("closed: folder %q, tabs %v, sessions %v", a.root, allTabNames(a), s.Sessions)
	}
	a.switchFolder(root)
	step(tt)
	if !slices.Equal(allTabNames(a), []string{"go.mod"}) {
		t.Fatalf("opened again: tabs %v", allTabNames(a))
	}
}

// A window whose documents close as it closes, after asking about their
// changes, keeps the tabs it had before.
func TestClosingWindowKeepsItsTabs(t *testing.T) {
	root := workspaceFixture(t)
	s := settings.Default()
	s.AutoSave = false
	h := &fakeHost{choice: 1} // don't save
	w := s.NewWindow()
	a := newApp(h, s, w)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	step(tt)
	a.openFolder(root)
	a.openFile(filepath.Join(root, "main.go"), true)
	step(tt)
	ed := a.activeEditor()
	ed.edit(ed.sel, textEdit{0, 0, "// changed\n"})
	step(tt)
	closed := false
	a.closeKeeping(func() { closed = true }, nil)
	step(tt)
	if !closed || len(allTabNames(a)) != 0 {
		t.Fatalf("closed %v, tabs %v", closed, allTabNames(a))
	}
	a.keepSession()
	s.Forget(w)
	ses := s.TakeSession(root)
	if ses == nil || len(ses.Columns) != 1 || ses.Columns[0].Tabs[0].File != filepath.Join(root, "main.go") {
		t.Fatalf("kept %+v", ses)
	}
}

// The tabs of the last session come back where they were scrolled, with
// their caret and their own choice to wrap, and a diff showing every
// line still does.
func TestRestoresWhereTabsWere(t *testing.T) {
	root := workspaceFixture(t)
	wrap := true
	s := settings.Default()
	w := s.NewWindow()
	w.Folder = root
	w.Columns = []settings.Column{{Tabs: []settings.Tab{
		{File: filepath.Join(root, "main.go"), Sel: [2]int{10, 14}, Wrap: &wrap},
		{File: filepath.Join(root, "go.mod"), Sel: [2]int{999, 999}},
		{Diff: "all", Full: true},
	}, Active: 0, Weight: 1}}
	w.Expanded = []string{"internal/a", "gone"}
	a := newApp(&fakeHost{folder: root}, s, w)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	step(tt)
	step(tt)
	col := a.cols[0]
	if len(col.tabs) != 3 {
		t.Fatalf("tabs %v", allTabNames(a))
	}
	if ed := col.tabs[0].ed; ed.sel != [2]int{10, 14} || !a.wraps(ed) {
		t.Errorf("main.go: selection %v, wraps %v", ed.sel, a.wraps(ed))
	}
	if ed := col.tabs[1].ed; ed.sel[1] != ed.buf.Len() {
		t.Errorf("go.mod: selection %v past its %d characters", ed.sel, ed.buf.Len())
	}
	if !col.tabs[2].diff.full {
		t.Error("the diff folds its unchanged lines")
	}
	if !a.files.Open.Has(filepath.Join(root, "internal", "a")) || a.files.Open.Has(filepath.Join(root, "gone")) {
		t.Error("the explorer's folders")
	}
	a.rememberTabs()
	got := w.Columns[0].Tabs
	if got[0].Sel != [2]int{10, 14} || got[0].Wrap == nil || !*got[0].Wrap || !got[2].Full ||
		!slices.Equal(w.Expanded, []string{"internal/a"}) {
		t.Errorf("remembered %+v, expanded %v", got, w.Expanded)
	}
}

// A restored file with a line too long keeps its error tab without
// laying out the text, which would hang the window.
func TestRestoreRetainsLongLineFailure(t *testing.T) {
	root := workspaceFixture(t)
	long := filepath.Join(root, "index.js.map")
	os.WriteFile(long, []byte(strings.Repeat("x", maxLineBytes+1)), 0o644)
	s := settings.Default()
	w := s.NewWindow()
	w.Folder = root
	w.Columns = []settings.Column{{Tabs: []settings.Tab{
		{File: long},
		{File: filepath.Join(root, "main.go")},
	}, Active: 0, Weight: 1}}
	a := newApp(&fakeHost{folder: root}, s, w)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	hs := &harness{t: t, a: a, tt: tt}
	hs.settle()
	if names := allTabNames(a); !slices.Equal(names, []string{"index.js.map", "main.go"}) {
		t.Fatalf("tabs %v", names)
	}
	if d := a.cols[0].tabs[0].ed; d.sync != syncLoadFailed || d.loadErr != errLongLine || d.buf.Len() != 0 || tt.HasText("Try Again") {
		t.Fatal("restored long line did not retain its failure without loading text")
	}
}

// A tab restored shows its text where it was scrolled, not at its caret.
func TestRestoresTheScroll(t *testing.T) {
	root := workspaceFixture(t)
	long := filepath.Join(root, "long.go")
	os.WriteFile(long, []byte(strings.Repeat("// a line\n", 400)), 0o644)
	s := settings.Default()
	w := s.NewWindow()
	w.Folder = root
	w.Columns = []settings.Column{{Tabs: []settings.Tab{
		{File: long, ScrollY: 1200}, {File: filepath.Join(root, "go.mod")},
	}, Active: 0, Weight: 1}}
	a := newApp(&fakeHost{folder: root}, s, w)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	hs := &harness{t: t, a: a, tt: tt}
	hs.settle()
	if y := a.cols[0].tabs[0].ed.vscroll.Y; y != 1200 {
		t.Errorf("scrolled to %v", y)
	}
}

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/internal/cli"
)

// A command acts on the project whose folder holds the directory it ran
// in, else the one in front, or that of --window.
func TestCommandTarget(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	_, hs, _ := railFixture(t, root, other)
	m := hs[0].m
	for _, c := range []struct {
		req  cli.Request
		want *desktop
	}{
		{cli.Request{Cwd: other}, hs[1]},
		{cli.Request{Cwd: filepath.Join(other, "internal", "a")}, hs[1]},
		{cli.Request{Cwd: t.TempDir()}, hs[0]},
		{cli.Request{Cwd: other, Window: root}, hs[0]},
	} {
		if got, err := m.target(c.req); err != nil || got != c.want {
			t.Errorf("%+v: got %v, %v", c.req, got, err)
		}
	}
	if _, err := m.target(cli.Request{Window: t.TempDir()}); err == nil {
		t.Error("a --window of no project")
	}
}

func TestExternalTargetSkipsDetached(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	_, windows, _ := railFixture(t, root, root, other)
	light, project, manager := windows[0], windows[1], windows[0].m
	light.a.window.Detached = true
	if manager.showing(root) != project || manager.workspaceFor(filepath.Join(root, "main.go")) != project || manager.workspaceFor(t.TempDir()) != project {
		t.Fatal("frontmost detached window claimed a workspace open")
	}
	manager.wins = []*desktop{light, windows[2]}
	if _, err := manager.target(cli.Request{Window: root}); err == nil || manager.showing(root) != nil || manager.workspaceFor(root) != windows[2] {
		t.Fatal("detached editor acted as its closed workspace")
	}
}

// ff run checks every command before running any, and takes the
// arguments of those that ask for one.
func TestCLICommands(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, false)
	a := hs.a
	for _, want := range [][]cli.Command{
		{{ID: "view.zoomIn"}, {ID: "no.such"}},
		{{ID: "view.zoomIn", Arg: "x"}},
		{{ID: "go.line", Arg: "x"}},
		{{ID: "file.openFolder", Arg: "nowhere"}},
		{{ID: "view.selectTheme", Arg: "nope"}},
		{{ID: "file.saveAs", Arg: "x.go"}},
	} {
		if _, err := a.cliCommands(want, hs.root); err == nil {
			t.Errorf("%+v ran", want)
		}
	}
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	runs, err := a.cliCommands([]cli.Command{{ID: "go.line", Arg: "6:2"}, {ID: "view.selectTheme", Arg: "Claude"}}, hs.root)
	if err != nil || len(runs) != 2 {
		t.Fatalf("%d runs: %v", len(runs), err)
	}
	for _, run := range runs {
		run()
	}
	hs.settle()
	d := a.textEditor()
	if s, _ := d.buf.Snapshot().LineRange(5); d.sel[1] != s+1 {
		t.Errorf("caret at %d, not %d", d.sel[1], s+1)
	}
	if a.settings.Theme != themeClaude {
		t.Errorf("theme %q", a.settings.Theme)
	}
	// A theme of the user's or a plugin's may be named with spaces or
	// capitals: the name as it is chooses it, not only the normalized id.
	hs2 := newHarness(t, false)
	withThemeHost(hs2)
	hs2.a.settings.Themes = map[string]json.RawMessage{
		"paper-ink": json.RawMessage(`{"base": "light", "name": "Paper Ink"}`),
	}
	themeFileChanged()
	runs, err = hs2.a.cliCommands([]cli.Command{{ID: "view.selectTheme", Arg: "paper-ink"}}, hs2.root)
	if err != nil || len(runs) != 1 {
		t.Fatalf("a theme by its id: %d runs, %v", len(runs), err)
	}
	runs[0]()
	if hs2.a.settings.Theme != "paper-ink" {
		t.Errorf("theme %q", hs2.a.settings.Theme)
	}
	list := a.commandList()
	if !strings.Contains(list, "file.openFolder\t") || !strings.Contains(list, "\t<folder>\n") {
		t.Errorf("commands:\n%s", list)
	}
}

// ff --wait answers once the file closes.
func TestWaitClosed(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	done := 0
	a.waitClosed(func() { done++ })
	a.openFile(hs.path("go.mod"), true)
	hs.settle()
	a.waitClosed(func() { done++ })
	a.closeActive()
	hs.settle()
	if done != 1 {
		t.Fatalf("%d done after closing one", done)
	}
	a.shutdown()
	if done != 2 {
		t.Errorf("%d done as the project closes", done)
	}
}

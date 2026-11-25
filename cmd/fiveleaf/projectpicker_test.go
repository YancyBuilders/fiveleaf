package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestProjectPickerFiltersAndKeepsOpenActions(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	remote := "/ssh:build-server.example/work/project"
	a.settings.Recent = []string{hs.root, remote}
	a.openProjectPicker()
	hs.settle()
	if !slices.Equal(a.recents.dirs, []string{hs.root, remote}) {
		t.Fatalf("projects = %v", a.recents.dirs)
	}
	hs.tt.Type("build-server")
	hs.settle()
	if len(a.recents.results) != 1 || a.recents.dirs[a.recents.results[0].Index] != remote {
		t.Fatalf("remote host search = %v", a.recents.results)
	}
	if err := hs.tt.Click(revealLabel() + " " + remote); err == nil {
		hs.settle()
	}
	if len(hs.h.revealed) != 0 || !a.recents.open {
		t.Fatal("a remote folder was revealed locally or selected by its disabled action")
	}
	a.recents.query = "no-such-project"
	hs.settle()
	if !hs.tt.HasText(locales.RecentNoMatches()) {
		t.Fatalf("missing empty state: %v", hs.tt.Texts())
	}
	for _, label := range []string{locales.ProjectOpenLocal(), locales.ProjectOpenRemote()} {
		if _, ok := hs.tt.Find(label); !ok {
			t.Errorf("%q disappeared with no matches", label)
		}
	}
	if err := hs.tt.Click(locales.ProjectOpenRemote()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.recents.open || (remoteSupported && !a.remotePick.open) {
		t.Fatal("the remote opening entry did not replace the project picker")
	}
}

func TestProjectPickerActionsOpenInTheChosenWindow(t *testing.T) {
	for _, mode := range []string{"here", "window", "cmd-enter", "local"} {
		t.Run(mode, func(t *testing.T) {
			hs := newHarness(t, true)
			a := hs.a
			other := workspaceFixture(t)
			a.settings.ProjectTabs = true
			a.settings.Recent = []string{other, hs.root}
			hs.h.folder = other
			a.openProjectPicker()
			hs.settle()
			switch mode {
			case "here":
				if err := hs.tt.Click(locales.ProjectOpenHere() + " " + other); err != nil {
					t.Fatal(err)
				}
			case "window":
				if err := hs.tt.Click(locales.ProjectOpenWindow() + " " + other); err != nil {
					t.Fatal(err)
				}
			case "cmd-enter":
				hs.tt.Key(ui.Cmd, ui.KeyEnter)
			case "local":
				if err := hs.tt.Click(locales.ProjectOpenLocal()); err != nil {
					t.Fatal(err)
				}
			}
			hs.settle()
			if a.recents.open || len(hs.h.windows) != 0 {
				t.Fatalf("open=%v, ordinary windows=%v", a.recents.open, hs.h.windows)
			}
			if mode == "window" || mode == "cmd-enter" {
				if a.root != hs.root || !slices.Equal(hs.h.projectWindows, []string{other}) {
					t.Fatalf("window action: root=%q, windows=%v", a.root, hs.h.projectWindows)
				}
			} else if a.root != other || len(hs.h.projectWindows) != 0 {
				t.Fatalf("current action: root=%q, windows=%v", a.root, hs.h.projectWindows)
			}
		})
	}
}

func TestProjectPickerCurrentWindowHonorsDirtyCancel(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.settings.AutoSave = false
	a.settings.ProjectTabs = true
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	ed := a.activeEditor()
	ed.edit(ed.sel, textEdit{0, 0, "// keep this edit\n"})
	hs.h.choice = 2
	other := workspaceFixture(t)
	a.settings.Recent = []string{other, hs.root}
	a.openProjectPicker()
	hs.settle()
	if err := hs.tt.Click(locales.ProjectOpenHere() + " " + other); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.root != hs.root || a.activeEditor() != ed || !ed.dirty() || len(hs.h.asked) != 1 {
		t.Fatalf("Cancel changed the current project: root=%q, prompts=%v", a.root, hs.h.asked)
	}
}

func TestProjectPickerRevealAndRemoveFilteredRecord(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	other := filepath.Join(t.TempDir(), "remove-this-project")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	a.settings.Recent = []string{hs.root, other}
	session := &settings.Session{Columns: []settings.Column{{Tabs: []settings.Tab{{File: filepath.Join(other, "main.go")}}}}}
	a.settings.Sessions = map[string]*settings.Session{other: session}
	a.openProjectPicker()
	hs.settle()
	hs.tt.Type("remove-this")
	hs.settle()
	if err := hs.tt.Click(revealLabel() + " " + other); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !slices.Equal(hs.h.revealed, []string{other}) || !a.recents.open || a.root != hs.root {
		t.Fatal("Reveal also opened the project or closed the picker")
	}
	if err := hs.tt.Click(locales.ProjectRemoveRecent() + " " + other); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !slices.Equal(a.settings.Recent, []string{hs.root}) || a.settings.Sessions[other] != session {
		t.Fatalf("Remove changed the wrong record or session: %v", a.settings.Recent)
	}
	if _, err := os.Stat(other); err != nil || len(hs.h.trashed) != 0 {
		t.Fatal("removing a recent project touched the folder on disk")
	}
	if !a.recents.open || len(a.recents.results) != 0 || a.recents.sel != 0 || a.root != hs.root {
		t.Fatal("the picker did not refresh in place after removing its result")
	}
}

func TestProjectPickerHoverKeepsBoundsAndArrowSelectionShows(t *testing.T) {
	withAppearancePlugins(t)
	hs := themeHarness(t, themeOneDarkPro, true)
	a := hs.a
	other := workspaceFixture(t)
	a.settings.Recent = []string{hs.root, other}
	a.openProjectPicker()
	hs.settle()
	first, ok := hs.tt.Find(locales.RecentOpenLabel(hs.root))
	if !ok {
		t.Fatal("missing current project row")
	}
	second, ok := hs.tt.Find(locales.RecentOpenLabel(other))
	if !ok {
		t.Fatal("missing other project row")
	}
	// The far right of the main button is whitespace filled by the row.
	x := int(first.X + first.W - 5)
	firstColor := hs.tt.Image().RGBAAt(x, int(first.Y+first.H/2))
	otherColor := hs.tt.Image().RGBAAt(x, int(second.Y+second.H/2))
	if firstColor == otherColor {
		hs.shot("project-picker-selection")
		t.Fatalf("the selected project has no visible background: first=%v second=%v sample=%d color=%v", first, second, x, firstColor)
	}
	hs.tt.Move(second.X+10, second.Y+second.H/2)
	hs.settle()
	after, _ := hs.tt.Find(locales.RecentOpenLabel(other))
	if after != second {
		t.Fatalf("hover moved the row: before=%v after=%v", second, after)
	}
	if err := hs.tt.Click(revealLabel() + " " + other); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !slices.Equal(hs.h.revealed, []string{other}) || !a.recents.open || a.recents.sel != 0 {
		t.Fatal("hovering a row did not expose its independent actions")
	}
	// Return keyboard focus to the search after clicking an action.
	if err := hs.tt.Click(locales.ProjectPickerPlaceholder()); err != nil {
		// The input's accessible name is the picker title; the placeholder
		// text need not be a separate hit target in a renderer.
		hs.tt.Press(420, titleBarH+27)
		hs.tt.Release(420, titleBarH+27)
	}
	hs.tt.Move(0, 0)
	hs.tt.Key(0, ui.KeyDown)
	hs.settle()
	if a.recents.sel != 1 {
		t.Fatal("Down did not select the next project")
	}
	selectedColor := hs.tt.Image().RGBAAt(x, int(second.Y+second.H/2))
	if selectedColor == otherColor {
		t.Fatal("Down did not move the selected row's background")
	}
	hs.tt.Key(0, ui.KeyUp)
	hs.settle()
	if a.recents.sel != 0 {
		t.Fatal("Up did not select the previous project")
	}
}

type projectHereHost struct {
	*fakeHost
	desktop *desktop
}

func (h *projectHereHost) openProjectHere(dir string) bool { return h.desktop.openProjectHere(dir) }

func TestProjectPickerMovesExistingProjectAfterConfirmation(t *testing.T) {
	for _, tabs := range []bool{false, true} {
		t.Run(map[bool]string{false: "windows", true: "project-tabs"}[tabs], func(t *testing.T) {
			root, other := workspaceFixture(t), workspaceFixture(t)
			into, hs, tt := railFixture(t, root, other)
			m := into.m
			m.s.AutoSave, m.s.ProjectTabs = false, tabs
			current, incoming := hs[0], hs[1]
			into.releaseProject(incoming)
			from := &frame{m: m}
			from.add(incoming)
			from.active = incoming
			m.frames = []*frame{into, from}
			otherTT := ui.NewTester(from.view, 1200, 760)
			left := &harness{t: t, a: current.a, tt: tt}
			right := &harness{t: t, a: incoming.a, tt: otherTT}
			right.settle()
			current.a.openFile(filepath.Join(root, "main.go"), true)
			incoming.a.openFile(filepath.Join(other, "main.go"), true)
			left.settle()
			right.settle()
			oldEditor, keptEditor := current.a.activeEditor(), incoming.a.activeEditor()
			oldEditor.edit(oldEditor.sel, textEdit{0, 0, "// old edit\n"})
			keptEditor.edit(keptEditor.sel, textEdit{0, 0, "// incoming edit\n"})
			host := current.a.host.(*fakeHost)
			host.holdClose = true
			current.a.host = &projectHereHost{fakeHost: host, desktop: current}
			current.a.chooseProject(other, false)
			left.settle()
			if len(host.held) != 1 || incoming.f != from || into.active != current {
				t.Fatal("the incoming app moved before the unsaved-change answer")
			}
			host.held[0](2)
			left.settle()
			if incoming.f != from || into.active != current || !oldEditor.dirty() || !keptEditor.dirty() {
				t.Fatal("Cancel changed either project")
			}
			current.a.chooseProject(other, false)
			left.settle()
			if len(host.held) != 2 {
				t.Fatal("a second attempt did not ask about the edit")
			}
			host.held[1](1)
			step(tt)
			step(tt)
			if incoming.f != into || into.active != incoming || len(into.tabs) != 1 || len(from.tabs) != 0 || from.life != lifeClosing {
				t.Fatalf("frames after move: into=%d from=%d, active=%p", len(into.tabs), len(from.tabs), into.active)
			}
			if len(m.wins) != 1 || m.wins[0] != incoming || len(m.s.Windows) != 1 || m.s.Windows[0] != incoming.a.window {
				t.Fatal("moving the project duplicated the app or its settings window")
			}
			if incoming.a.activeEditor() != keptEditor || !keptEditor.dirty() || !strings.HasPrefix(keptEditor.buf.String(), "// incoming edit") {
				t.Fatal("moving the incoming app discarded or reloaded its edits")
			}
			if session := m.s.Sessions[root]; session == nil || len(session.Columns) == 0 || len(session.Columns[0].Tabs) == 0 {
				t.Fatal("the replaced project's session was not kept")
			}
		})
	}
}

func TestProjectPickerExistingProjectInSameWindowActivates(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	f, hs, tt := railFixture(t, root, other)
	if !hs[0].openProjectHere(other) {
		t.Fatal("the open project was not handled")
	}
	step(tt)
	if f.active != hs[1] || len(f.tabs) != 2 || len(f.m.wins) != 2 {
		t.Fatal("the existing project was duplicated or removed")
	}
}

func TestProjectPickerSeparatingTabKeepsDirtyApp(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	f, hs, tt := railFixture(t, root, other)
	incoming := hs[1]
	f.m.s.AutoSave = false
	f.activate(incoming)
	step(tt)
	incoming.a.openFile(filepath.Join(other, "main.go"), true)
	h := &harness{t: t, a: incoming.a, tt: tt}
	h.settle()
	ed, state := incoming.a.activeEditor(), incoming.a.window
	ed.edit(ed.sel, textEdit{0, 0, "// unsaved in moved window\n"})
	f.releaseProject(incoming)
	if f.active != hs[0] || len(f.tabs) != 1 || incoming.f != nil || len(f.m.wins) != 2 {
		t.Fatal("detaching the project changed the wrong tab or application registry")
	}
	if incoming.a.window != state || incoming.a.activeEditor() != ed || !ed.dirty() {
		t.Fatal("detaching the project replaced its session or documents")
	}
	if got, want := incoming.a.windowTitle(), "● main.go — "+filepath.Base(other); got != want {
		t.Fatalf("the detached window's initial title = %q, want %q", got, want)
	}
	// A project already alone in a native frame is reused by the strict
	// window action. No second frame or app should be allocated.
	separate := &frame{m: f.m}
	separate.add(incoming)
	separate.active = incoming
	f.m.frames = []*frame{f, separate}
	f.m.openProjectFolder(other, false, true)
	if incoming.f != separate || len(f.m.frames) != 2 || len(f.m.wins) != 2 || !ed.dirty() {
		t.Fatal("the new-window action duplicated an existing independent window")
	}
}

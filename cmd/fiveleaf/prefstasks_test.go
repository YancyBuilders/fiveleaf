package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/keymap"
	"fiveleaf/locales"
)

func newPrefsTasksHarness(t *testing.T) *prefsHarness {
	t.Helper()
	onOS(t, "linux")
	ph := newPrefsHarness(t)
	ph.m.wins[0].f.win = nil
	return ph
}

func TestPrefsTaskShortcutLifecycle(t *testing.T) {
	ph := newPrefsTasksHarness(t)
	ph.pt.SetSize(640, 680)
	const name = `Build "demo"`
	writeTaskDefinitions(t, ph.a.projectTasksPath(), printedTask(name, "RESULT"))
	path := keymap.Path(ph.m.s.Path())
	const original = `// Personal keys
[
  {"key":"f3","command":"task.spawn"},
  {"key":"f8","command":"task.spawn","args":{"task_name":"Test"}},
  {"key":"f9","command":"task.spawn","args":{"task_name":"Build \"demo\""},"when":"editorFocus"},
]`
	writeFile(t, path, []byte(original))
	ph.m.keys, _ = keymap.Load(path)
	ph.a.applyKeymap(ph.m.keys)
	button := locales.KeysRecordTitle(name)
	prefsTasksClick(t, ph, locales.PrefSectionTerminal(), locales.PrefSectionTasks(), button)
	ph.pt.Key(0, ui.KeyF8)
	ph.frames()
	if !ph.pt.HasText(locales.KeysConflict("Test")) {
		t.Fatal("recording did not identify the other task on F8")
	}
	ph.shot("prefs-task-shortcut-conflict")
	ph.pt.Key(0, ui.KeyEscape)
	ph.frames()
	if data, err := os.ReadFile(path); err != nil || string(data) != original {
		t.Fatalf("cancel changed bindings: %q, %v", data, err)
	}
	for _, key := range []ui.Key{ui.KeyF6, ui.KeyF7} {
		prefsTasksClick(t, ph, button)
		ph.pt.Key(0, key)
		ph.pt.Key(0, ui.KeyEnter)
		ph.frames()
		stroke := keymap.Stroke{Key: key}
		binding, _ := ph.a.keys.Lookup([]keymap.Stroke{stroke}, nil)
		if binding == nil || bindingTaskName(binding.Command, binding.Args) != name || !ph.pt.HasText(keyText(stroke.Accel())) {
			t.Fatalf("recorded shortcut not active in editor and settings: %+v", binding)
		}
	}
	ph.shot("prefs-task-shortcut-bound")
	prefsTasksClick(t, ph, button, locales.KeysRemove())
	for _, key := range []ui.Key{ui.KeyF6, ui.KeyF7} {
		if binding, _ := ph.a.keys.Lookup([]keymap.Stroke{{Key: key}}, nil); binding != nil {
			t.Fatalf("clearing keys kept %+v", binding)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte("// Personal keys")) || len(ph.m.keys.Bindings) != 3 {
		t.Fatalf("clearing one task affected other bindings: %q, %v", data, err)
	}
	if ph.a.taskPick.open || len(ph.a.terms) != 0 {
		t.Fatal("recording, cancelling or clearing a shortcut started a task")
	}
}

func prefsTasksClick(t *testing.T, ph *prefsHarness, labels ...string) {
	t.Helper()
	for _, label := range labels {
		if err := ph.pt.Click(label); err != nil {
			t.Fatal(err)
		}
		ph.frames()
	}
}

func prefsTasksMissing(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be absent, Lstat returned %v", path, err)
		}
	}
}

func prefsTasksNoEditor(t *testing.T, editor *harness) {
	t.Helper()
	editor.settle()
	if _, found := editor.tt.Find(locales.CloseName("tasks.json")); found {
		t.Fatalf("unexpected tasks.json tab in window for %s", editor.root)
	}
}

func prefsTasksSave(t *testing.T, editor *harness, path string) {
	t.Helper()
	editor.settle()
	if _, found := editor.tt.Find(locales.CloseName("tasks.json")); !found {
		t.Fatalf("no editor tab opened for %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	const edited = `[{"label":"edited in settings","command":"pwd"}]` + "\n"
	editor.tt.SetClipboard(edited)
	editor.tt.Command("selectAll")
	editor.tt.Command("paste")
	editor.key(ui.Ctrl, ui.KeyS)
	if data, err := os.ReadFile(path); err != nil || string(data) != edited {
		t.Fatalf("saving the active editor to %s produced %q, error %v", path, data, err)
	}
}

func TestPrefsTasksBrowseAndEdit(t *testing.T) {
	for _, route := range []string{"Terminal", "Search"} {
		t.Run(route, func(t *testing.T) {
			ph := newPrefsTasksHarness(t)
			global := filepath.Join(filepath.Dir(ph.m.s.Path()), "tasks.json")
			project := ph.path(".fiveleaf/tasks.json")
			action, want, untouched := locales.TaskOpenProject(), project, global
			if route == "Terminal" {
				prefsTasksClick(t, ph, locales.PrefSectionTerminal(), locales.PrefSectionTasks())
			} else {
				prefsTasksClick(t, ph, locales.PrefsSearch())
				ph.pt.Type("tasks.json")
				ph.frames()
				prefsTasksClick(t, ph, locales.PrefsTaskDefinitions())
				action, want, untouched = locales.TaskOpenGlobal(), global, project
			}
			for _, label := range []string{locales.TaskOpenProject(), locales.TaskOpenGlobal(), "lazygit", "yazi"} {
				if _, found := ph.pt.Find(label); !found {
					t.Fatalf("Tasks page reached via %s is missing %q", route, label)
				}
			}
			prefsTasksNoEditor(t, ph.harness)
			prefsTasksMissing(t, global, project)
			prefsTasksClick(t, ph, action)
			prefsTasksSave(t, ph.harness, want)
			prefsTasksMissing(t, untouched)
		})
	}
}

func TestPrefsTasksSelectedFolder(t *testing.T) {
	for _, action := range []string{locales.TaskOpenProject(), locales.TaskOpenGlobal()} {
		t.Run(action, func(t *testing.T) {
			ph := newPrefsTasksHarness(t)
			folder := t.TempDir()
			window := ph.m.s.NewWindow()
			window.Folder = folder
			host := &fakeHost{folder: folder}
			application := newApp(host, ph.m.s, window)
			t.Cleanup(application.shutdown)
			selected := &harness{t: t, a: application, h: host, root: folder, tt: ui.NewTester(application.view, 1200, 760)}
			selected.settle()
			ph.m.wins = append(ph.m.wins, &desktop{m: ph.m, a: application, f: &frame{m: ph.m}})

			global := filepath.Join(filepath.Dir(ph.m.s.Path()), "tasks.json")
			project := selected.path(".fiveleaf/tasks.json")
			const globalData = `[{"label":"verify","command":"global-check"},{"label":"global only","command":"global-only-check"}]`
			const projectData = `[{"label":"verify","command":"selected-check"}]`
			writeFile(t, global, []byte(globalData))
			writeFile(t, project, []byte(projectData))
			prefsTasksClick(t, ph, locales.PrefSectionTerminal(), locales.PrefSectionTasks())
			if !ph.pt.HasText("global-check") || ph.pt.HasText("selected-check") {
				t.Fatal("initial scope should show the front folder's effective verify command")
			}
			prefsTasksClick(t, ph, locales.PrefsScope(), locales.PrefsFolder(shortenHome(folder)))
			if !ph.pt.HasText("selected-check") || ph.pt.HasText("global-check") || !ph.pt.HasText("global-only-check") {
				t.Fatal("selected project should replace the matching global command and retain the global-only command")
			}
			want, untouched, original := project, global, globalData
			if action == locales.TaskOpenGlobal() {
				want, untouched, original = global, project, projectData
			}
			prefsTasksClick(t, ph, action)
			prefsTasksNoEditor(t, ph.harness)
			prefsTasksSave(t, selected, want)
			prefsTasksMissing(t, ph.path(".fiveleaf/tasks.json"))
			if data, err := os.ReadFile(untouched); err != nil || string(data) != original {
				t.Fatalf("editing %s changed %s to %q, error %v", want, untouched, data, err)
			}
		})
	}
}

func TestPrefsTasksQueuedEditAfterFolderSwitch(t *testing.T) {
	ph := newPrefsTasksHarness(t)
	ph.m.s.AutoSave, ph.m.s.ProjectTabs = false, false
	nextFolder := t.TempDir()
	ph.key(ui.Ctrl, ui.KeyN)
	ph.tt.Type("unsaved before switching folders")
	ph.h.folder, ph.h.holdClose = nextFolder, true
	ph.key(ui.Ctrl, ui.KeyO)
	if len(ph.h.held) != 1 {
		t.Fatal("expected the pending folder switch to ask about the unsaved tab")
	}
	prefsTasksClick(t, ph, locales.PrefSectionTerminal(), locales.PrefSectionTasks(), locales.TaskOpenProject())
	previous := ph.path(".fiveleaf/tasks.json")
	next := filepath.Join(nextFolder, ".fiveleaf", "tasks.json")
	global := filepath.Join(filepath.Dir(ph.m.s.Path()), "tasks.json")
	prefsTasksMissing(t, previous, next, global)
	ph.h.held[0](1)
	prefsTasksNoEditor(t, ph.harness)
	prefsTasksMissing(t, previous, next, global)
	ph.frames()
	prefsTasksClick(t, ph, locales.TaskOpenProject())
	prefsTasksSave(t, ph.harness, next)
	prefsTasksMissing(t, previous, global)
}

func TestPrefsTasksNoLocalFolder(t *testing.T) {
	ph := newPrefsTasksHarness(t)
	ph.key(ui.Ctrl|ui.Shift, ui.KeyP)
	ph.tt.Type(locales.CmdFileCloseFolder())
	ph.key(0, ui.KeyEnter)
	prefsTasksClick(t, ph, locales.PrefSectionTerminal(), locales.PrefSectionTasks())
	if !ph.pt.HasText(locales.PrefsNoFolder()) {
		t.Fatal("project configuration should explain why a local folder is needed")
	}
	global := filepath.Join(filepath.Dir(ph.m.s.Path()), "tasks.json")
	previous := ph.path(".fiveleaf/tasks.json")
	prefsTasksClick(t, ph, locales.TaskOpenProject())
	prefsTasksNoEditor(t, ph.harness)
	prefsTasksMissing(t, previous, global)
	prefsTasksClick(t, ph, locales.TaskOpenGlobal())
	prefsTasksSave(t, ph.harness, global)
	prefsTasksMissing(t, previous)
}

func TestPrefsTasksRefreshRepairedProject(t *testing.T) {
	ph := newPrefsHarness(t)
	global := filepath.Join(filepath.Dir(ph.m.s.Path()), "tasks.json")
	project := ph.path(".fiveleaf/tasks.json")
	const broken = "[\n  {\"label\": \"verify\",\n"
	writeFile(t, global, []byte(`[{"label":"verify","command":"global-fallback"}]`))
	writeFile(t, project, []byte(broken))
	prefsTasksClick(t, ph, locales.PrefSectionTerminal(), locales.PrefSectionTasks())
	problem := locales.TaskConfigError(shortenHome(project), "")
	before := ph.pt.Texts()
	if !ph.pt.HasText(problem) || !slices.Contains(before, locales.TaskGlobal()) || !slices.Contains(before, "global-fallback") {
		t.Fatal("invalid project configuration should show its error and the effective global task")
	}
	if data, err := os.ReadFile(project); err != nil || string(data) != broken {
		t.Fatalf("opening Tasks changed the invalid file to %q, error %v", data, err)
	}
	writeFile(t, project, []byte(`[{"label":"verify","command":"project-repaired"}]`))
	prefsTasksClick(t, ph, locales.Refresh())
	after := ph.pt.Texts()
	if ph.pt.HasText(problem) || slices.Contains(after, locales.TaskGlobal()) || slices.Contains(after, "global-fallback") {
		t.Fatal("Refresh kept the old error or global definition after repairing the project file")
	}
	if !slices.Contains(after, locales.TaskProject()) || !slices.Contains(after, "project-repaired") || !slices.Contains(after, "verify") {
		t.Fatal("Refresh should show verify with the repaired project command and source")
	}
}

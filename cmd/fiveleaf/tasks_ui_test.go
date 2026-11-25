package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/keymap"
	"fiveleaf/internal/settings"
	"fiveleaf/internal/tasks"
	"fiveleaf/locales"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func taskUIHarness(t *testing.T, dark bool) *harness {
	t.Helper()
	root := workspaceFixture(t)
	s := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	s.MarkdownMode, s.AutoSave = "", false
	w := s.NewWindow()
	w.Folder = root
	h := &fakeHost{folder: root}
	a := newApp(h, s, w)
	t.Cleanup(s.Flush)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	tt.SetDark(dark)
	hs := &harness{t: t, a: a, h: h, tt: tt, root: root}
	hs.settle()
	return hs
}

func writeTaskDefinitions(t *testing.T, path string, definitions ...tasks.Task) {
	t.Helper()
	data, err := json.Marshal(definitions)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func taskPTY(t *testing.T) {
	t.Helper()
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("starts a POSIX shell in a PTY")
	}
	library, err := terminal.LibraryPath()
	if err != nil {
		t.Fatal(err)
	}
	// Keep the native cache while isolating the login shell's profile files.
	t.Setenv("MYGO_GHOSTTY_VT", library)
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("HOME", t.TempDir())
}

func printedTask(label, output string) tasks.Task {
	return tasks.Task{Label: label, Command: "printf '%s\\n'", Args: []string{output}}
}

func TestTaskCatalogOverridesAndKeepsValidSources(t *testing.T) {
	dir := t.TempDir()
	global, project := filepath.Join(dir, "global.json"), filepath.Join(dir, ".fiveleaf", "tasks.json")
	writeTaskDefinitions(t, global, printedTask("yazi", "global"), printedTask("test", "global test"))
	writeTaskDefinitions(t, project, printedTask("yazi", "project"), printedTask("build", "project build"))
	entries, problems := readTaskCatalog(global, filepath.Join(dir, "absent.json"), project)
	if len(problems) != 0 || len(entries) != 4 {
		t.Fatalf("catalog = %+v, errors = %+v", entries, problems)
	}
	var labels []string
	for _, entry := range entries {
		labels = append(labels, entry.task.Label)
	}
	if !slices.Equal(labels, []string{"lazygit", "yazi", "test", "build"}) || entries[1].source != project || entries[1].task.Args[0] != "project" {
		t.Fatalf("project override lost order or source: %+v", entries)
	}
	if err := os.WriteFile(project, []byte(`[{"label":`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, problems = readTaskCatalog(global, project)
	if len(problems) != 1 || problems[0].path != project || len(entries) != 3 || entries[0].task.Command != "lazygit" || entries[1].task.Args[0] != "global" {
		t.Fatalf("a broken project hid valid tasks: entries=%+v errors=%+v", entries, problems)
	}
	if err := os.WriteFile(global, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, problems = readTaskCatalog(global, project)
	if len(problems) != 2 || len(entries) != 2 || entries[0].task.Command != "lazygit" || entries[1].task.Command != "yazi" {
		t.Fatalf("broken files hid the built-ins: entries=%+v errors=%+v", entries, problems)
	}
}

func TestTaskPickerSearchAndEnterRunsChosenTask(t *testing.T) {
	taskPTY(t)
	hs := taskUIHarness(t, true)
	a := hs.a
	writeTaskDefinitions(t, a.projectTasksPath(), printedTask("lazygit", "TASK_GIT"), tasks.Task{Label: "yazi", Command: "/bin/sh", Args: []string{"-c", "sleep 0.05; printf 'TASK_YAZI\\n'"}})
	a.run("task.spawn")
	hs.settle()
	if !a.taskPick.open || !hs.tt.Focused(locales.TaskPickerTitle()) || len(a.taskPick.results) != 2 {
		t.Fatal("Run Task did not focus the two-task picker")
	}
	hs.key(0, ui.KeyDown)
	if a.taskPick.sel != 1 {
		t.Fatal("Down did not choose the next task")
	}
	hs.key(0, ui.KeyUp)
	if a.taskPick.sel != 0 {
		t.Fatal("Up did not choose the previous task")
	}
	hs.tt.Type("yaz")
	hs.settle()
	if len(a.taskPick.results) != 1 || a.taskPick.entries[a.taskPick.results[0].Index].task.Label != "yazi" {
		t.Fatalf("search results = %+v", a.taskPick.results)
	}
	hs.key(0, ui.KeyEnter)
	if a.taskPick.open || len(a.terms) != 1 || a.lastTask.label != "yazi" || a.lastTask.root != a.root {
		t.Fatalf("Enter: open=%v terminals=%d last=%q", a.taskPick.open, len(a.terms), a.lastTask.label)
	}
	s := a.terms[0]
	hs.until("chosen task output", func() bool { return s.task.completed })
	if strings.Contains(s.task.output, "TASK_GIT") || s.task.exitCode != 0 || !strings.Contains(s.task.output, "TASK_YAZI") {
		t.Fatalf("wrong task output or exit: %q, code=%d", s.task.output, s.task.exitCode)
	}
	hs.settle()
	if len(a.terms) != 0 || s.tab != nil {
		t.Fatal("the exited task's editor tab stayed open")
	}
}

func TestTaskPickerNoMatchesAndEscapeRestoreEditor(t *testing.T) {
	hs := taskUIHarness(t, false)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	ed := a.activeEditor()
	a.run("task.spawn")
	hs.settle()
	hs.tt.Type("no-such-task-43859")
	hs.settle()
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyUp)
	hs.key(0, ui.KeyEnter)
	if !a.taskPick.open || len(a.taskPick.results) != 0 || len(a.terms) != 0 || !hs.tt.HasText(locales.TaskNoMatches()) {
		t.Fatal("an empty search launched a task or closed the picker")
	}
	hs.key(0, ui.KeyEscape)
	before := ed.buf.String()
	hs.tt.Type("TASK_EDITOR_FOCUS")
	hs.settle()
	if a.taskPick.open || a.activeEditor() != ed || ed.buf.String() == before || !strings.Contains(ed.buf.String(), "TASK_EDITOR_FOCUS") {
		t.Fatal("Escape did not restore editor input")
	}
}

func TestTaskPickerEscapeRestoresTerminal(t *testing.T) {
	taskPTY(t)
	hs := taskUIHarness(t, true)
	a := hs.a
	a.newTerminal()
	hs.settle()
	s := a.panelTerm
	if s == nil {
		t.Fatal("no terminal opened")
	}
	a.run("task.spawn")
	hs.settle()
	hs.key(0, ui.KeyEscape)
	hs.tt.Type("printf 'TASK_TERMINAL_FOCUS\\n'")
	hs.key(0, ui.KeyEnter)
	hs.until("input in the original terminal", func() bool {
		return strings.Contains(s.term.Text(), "TASK_TERMINAL_FOCUS\r\n") || strings.Contains(s.term.Text(), "TASK_TERMINAL_FOCUS\n")
	})
	if a.taskPick.open || a.panelTerm != s || a.focus != paneTerminal || len(a.terms) != 1 {
		t.Fatal("Escape did not return to the original terminal")
	}
}

func TestTaskCommandsAndNamedKeybinding(t *testing.T) {
	taskPTY(t)
	hs := taskUIHarness(t, true)
	a := hs.a
	writeTaskDefinitions(t, a.projectTasksPath(),
		tasks.Task{Label: "lazygit", Command: "/bin/sh", Args: []string{"-c", "read answer && printf 'TASK_BINDING\\n'"}},
		tasks.Task{Label: "yazi", Command: "/bin/sh", Args: []string{"-c", "read answer && printf 'TASK_DIRECT\\n'"}})
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	withKeys(t, a, `[{"key":"ctrl+shift+f8","command":"task.spawn","args":{"task_name":"lazygit"}}]`)
	hs.key(ui.Ctrl|ui.Shift, ui.KeyF8)
	if a.taskPick.open || len(a.terms) != 1 || a.lastTask.label != "lazygit" {
		t.Fatal("the named binding did not directly launch lazygit")
	}
	first := a.terms[0]
	first.term.Send([]byte("continue\r"))
	hs.until("named binding output", func() bool { return strings.Contains(first.task.output, "TASK_BINDING") })
	hs.settle()
	a.run("task.yazi")
	hs.settle()
	if len(a.terms) != 1 || a.lastTask.label != "yazi" {
		t.Fatalf("the direct yazi command did not launch its definition: terminals=%d last=%q fails=%d", len(a.terms), a.lastTask.label, a.fails)
	}
	second := a.terms[0]
	second.term.Send([]byte("continue\r"))
	hs.until("direct command output", func() bool { return strings.Contains(second.task.output, "TASK_DIRECT") })
	hs.settle()
	if len(a.terms) != 0 || first.tab != nil || second.tab != nil {
		t.Fatal("completed direct tasks left their tabs open")
	}
}

func TestTaskFileContextReusesLiveEditorTerminal(t *testing.T) {
	taskPTY(t)
	hs := taskUIHarness(t, true)
	a := hs.a
	a.settings.TerminalLocation = "editor"
	file := hs.path("main.go")
	a.openFile(file, true)
	hs.settle()
	definition := tasks.Task{
		Label: "file task", Command: "/bin/sh",
		Args: []string{"-c", `printf 'file=<%s>\n' "$1"; read answer`, "task", "$FIVELEAF_FILE"},
	}
	writeTaskDefinitions(t, a.projectTasksPath(), definition)
	a.runTaskLabel(definition.Label)
	if len(a.terms) != 1 {
		t.Fatalf("file task did not start: %v", a.errors)
	}
	s := a.terms[0]
	hs.until("file task ready", func() bool { return strings.Contains(s.term.Text(), "file=<"+file+">") })
	if a.activeFile() != "" || s.tab == nil || a.column().tab() != s.tab {
		t.Fatal("the editor terminal did not replace the active file context")
	}
	for _, action := range []struct {
		name string
		run  func()
	}{
		{"named task", func() { a.runTaskLabel(definition.Label) }},
		{"rerun", func() { a.run("task.rerun") }},
		{"picker", func() { a.openTaskPicker(); a.chooseTask(definition.Label) }},
	} {
		t.Run(action.name, func(t *testing.T) {
			fails := a.fails
			previous := s.term
			action.run()
			hs.settle()
			if a.fails != fails || len(a.terms) != 1 || s.term != previous || !taskRunning(s) || a.column().tab() != s.tab || a.lastTask.label != definition.Label {
				t.Fatalf("live file task was not reused: failures=%d terminals=%d", a.fails-fails, len(a.terms))
			}
		})
	}
	// The current definition can opt into concurrent runs, which still need
	// the file context for the new process rather than reusing the old one.
	definition.AllowConcurrentRuns = true
	writeTaskDefinitions(t, a.projectTasksPath(), definition)
	fails := a.fails
	a.runTaskLabel(definition.Label)
	if a.fails != fails+1 || len(a.terms) != 1 || !taskRunning(s) {
		t.Fatal("a new concurrent run did not require its file context")
	}
	a.activate(a.column(), 0, true)
	a.runTaskLabel(definition.Label)
	if len(a.terms) != 2 || a.terms[1] == s || !taskRunning(s) {
		t.Fatal("the updated definition did not start a concurrent task")
	}
}

func TestTaskRerunReloadsSavedDefinitionAndStaysInWorkspace(t *testing.T) {
	taskPTY(t)
	hs := taskUIHarness(t, true)
	a := hs.a
	config := a.projectTasksPath()
	writeTaskDefinitions(t, config, tasks.Task{Label: "check", Command: "/bin/sh", Args: []string{"-c", "printf 'TASK_OLD\\n'; read answer"}})
	a.runTaskLabel("check")
	hs.settle()
	if len(a.terms) != 1 {
		t.Fatalf("initial task did not start: terminals=%d errors=%v last=%q", len(a.terms), a.errors, a.lastTask.label)
	}
	s := a.terms[0]
	hs.until("initial task output", func() bool { return strings.Contains(s.term.Text(), "TASK_OLD") })
	s.term.Send([]byte("done\r"))
	hs.until("initial task completion", func() bool { return s.task.completed })
	firstTerm := s.term
	writeTaskDefinitions(t, config, tasks.Task{Label: "check", Command: "/bin/sh", Args: []string{"-c", "printf 'TASK_NEW\\n'; exit 0"}})
	hs.settle()
	a.run("task.rerun")
	hs.settle()
	if len(a.terms) != 1 || a.terms[0] == s || a.terms[0].term == firstTerm {
		t.Fatal("rerun did not start the task again with its latest definition")
	}
	rerun := a.terms[0]
	hs.until("updated task output", func() bool {
		return strings.Contains(rerun.task.output, "TASK_NEW")
	})
	hs.settle()
	if len(a.terms) != 0 || rerun.tab != nil {
		t.Fatal("a successful task's editor tab stayed open")
	}
	previous := rerun.term
	a.switchFolder(workspaceFixture(t))
	hs.settle()
	a.run("task.rerun")
	hs.settle()
	if !a.taskPick.open || rerun.term != previous || len(a.terms) != 0 {
		t.Fatal("rerun started a task from the previous workspace")
	}
	// A folder switch between choosing and executing invalidates that choice.
	a.chooseTask("yazi")
	a.switchFolder(hs.root)
	hs.settle()
	if len(a.terms) != 0 || rerun.term != previous {
		t.Fatal("a picker choice ran after its workspace changed")
	}
}

func TestTaskRerunKeepsFileContext(t *testing.T) {
	taskPTY(t)
	for _, notification := range []bool{false, true} {
		name := "last task command"
		if notification {
			name = "failure notification"
		}
		t.Run(name, func(t *testing.T) {
			hs := taskUIHarness(t, false)
			a := hs.a
			file := hs.path("main.go")
			a.openFile(file, true)
			hs.settle()
			a.activeEditor().goToLine(3, 4)
			hs.settle()
			definition := tasks.Task{Label: "inspect file", Command: "/bin/sh", Args: []string{
				"-c", `printf 'before=<%s:%s:%s>\n' "$1" "$2" "$3"; exit 7`, "task",
				"$FIVELEAF_FILE", "$FIVELEAF_ROW", "$FIVELEAF_COLUMN",
			}}
			config := a.projectTasksPath()
			writeTaskDefinitions(t, config, definition)
			a.runTaskLabel(definition.Label)
			if len(a.terms) != 1 {
				t.Fatalf("task did not start: %v", a.errors)
			}
			session := a.terms[0]
			hs.until("file task failed", func() bool { return session.task.completed })
			original := session.task
			want := "<" + file + ":3:4>"
			if a.activeFile() != "" || !strings.Contains(original.output, "before="+want) {
				t.Fatalf("active file = %q, original task output = %q", a.activeFile(), original.output)
			}
			definition.Args[1] = `printf 'after=<%s:%s:%s>\n' "$1" "$2" "$3"; exit 9`
			writeTaskDefinitions(t, config, definition)
			if notification {
				if err := hs.tt.Click(locales.TaskRerun()); err != nil {
					t.Fatal(err)
				}
			} else {
				a.run("task.rerun")
			}
			hs.settle()
			if session.task == original || len(a.terms) != 1 {
				t.Fatalf("rerun did not reuse the task terminal: %v", a.errors)
			}
			hs.until("rerun finished", func() bool { return session.task.completed })
			if !strings.Contains(session.task.output, "after="+want) || session.task.exitCode != 9 {
				t.Fatalf("rerun output = %q, exit code = %d", session.task.output, session.task.exitCode)
			}
		})
	}
}

func TestTaskConfigEditorPreservesInvalidFiles(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(map[bool]string{false: "global", true: "project"}[project], func(t *testing.T) {
			hs := taskUIHarness(t, true)
			a := hs.a
			path := a.globalTasksPath()
			if project {
				path = a.projectTasksPath()
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			invalid := `[{"label": "unfinished"`
			if err := os.WriteFile(path, []byte(invalid), 0o644); err != nil {
				t.Fatal(err)
			}
			a.openTaskPicker()
			hs.settle()
			if len(a.taskPick.errors) != 1 || len(a.taskPick.entries) != 2 {
				t.Fatal("invalid configuration did not show an error beside the built-ins")
			}
			a.openTasksConfig(project)
			hs.settle()
			data, err := os.ReadFile(path)
			if err != nil || string(data) != invalid || a.activeFile() != path || a.taskPick.open {
				t.Fatalf("edit changed the file or failed to open it: %q, %v, active=%q", data, err, a.activeFile())
			}
		})
	}
	// Merely opening the picker leaves the project untouched.
	hs := taskUIHarness(t, false)
	hs.a.openTaskPicker()
	if _, err := os.Stat(hs.a.projectTasksPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("opening the picker created a config file: %v", err)
	}
	hs.a.openTasksConfig(true)
	hs.settle()
	definitions, err := readTaskFile(hs.a.projectTasksPath())
	if err != nil || len(definitions) != 2 {
		t.Fatalf("new configuration = %v, %v", definitions, err)
	}
}

func TestTaskShortcutsAndLocalizedGroups(t *testing.T) {
	for _, platform := range []string{"darwin", "linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			onOS(t, platform)
			hs := taskUIHarness(t, true)
			var accel string
			for _, cmd := range hs.a.commands() {
				if cmd.id == "task.spawn" {
					accel = cmd.accel
				}
			}
			want := "Alt+CmdOrCtrl+T"
			if platform != "darwin" {
				want = "Ctrl+Shift+B"
				hs.key(ui.Ctrl|ui.Shift, ui.KeyB)
				if !hs.a.taskPick.open {
					t.Fatal("Ctrl+Shift+B did not open tasks")
				}
				if id, ok := hs.a.shellSkip(keymap.Stroke{Mods: ui.Ctrl | ui.Shift, Key: ui.KeyB}); !ok || id != "task.spawn" {
					t.Fatalf("task shortcut stays in the shell: %q %v", id, ok)
				}
			}
			if accel != want {
				t.Errorf("accelerator = %q, want %q", accel, want)
			}
		})
	}
	hs := taskUIHarness(t, false)
	for _, locale := range []string{"en", "ja", "zh-Hans"} {
		messages, n := locales.In(locale), 0
		for _, cmd := range hs.a.commandsIn(messages) {
			if !strings.HasPrefix(cmd.id, "task.") {
				continue
			}
			n++
			if cmd.label == "" || commandGroup(messages, cmd.id) != messages.MenuTasks() {
				t.Errorf("%s task command lacks a label or group: %+v", locale, cmd)
			}
		}
		if n != 6 {
			t.Errorf("%s has %d task commands, want 6", locale, n)
		}
	}
}

func taskGalleryFixture(hs *harness) {
	pick := taskPicker{open: true, stale: true, root: hs.a.root}
	for _, task := range tasks.Builtins() {
		pick.entries = append(pick.entries, taskEntry{task: task})
		pick.names = append(pick.names, task.Label)
	}
	hs.a.taskPick = pick
}

func TestTaskPickerLayouts(t *testing.T) {
	withAppearancePlugins(t)
	for _, locale := range []string{"en", "zh-Hans"} {
		for _, state := range []string{"tasks", "task-no-match", "task-error"} {
			t.Run(locale+"/"+state, func(t *testing.T) {
				previous := i18n.Locale()
				i18n.SetLocaleFunc(func() string { return locale })
				t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
				hs := taskUIHarness(t, true)
				hs.tt.SetLocale(locale)
				hs.a.setTheme(themeOneDarkPro)
				taskGalleryFixture(hs)
				if state == "task-no-match" {
					hs.a.taskPick.query = "zzzz-no-task"
				}
				if state == "task-error" {
					hs.a.taskPick.errors = []taskConfigError{{path: hs.a.projectTasksPath(), err: errors.New("tasks must be a JSON array")}}
				}
				hs.tt.SetSize(420, 520)
				hs.settle()
				for _, label := range []string{locales.TaskOpenProject(), locales.TaskOpenGlobal()} {
					r, ok := hs.tt.Find(label)
					if !ok || r.X < 0 || r.X+r.W > 420 || r.Y+r.H > 520 {
						t.Errorf("narrow %s %s has clipped action %q: %v", locale, state, label, r)
					}
				}
				hs.shot(state + "-" + locale + "-narrow")
			})
		}
	}
}

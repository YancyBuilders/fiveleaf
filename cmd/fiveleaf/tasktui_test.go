package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/tasks"
)

func isolateTaskTUIs(t *testing.T) {
	t.Helper()
	taskPTY(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	socketDir, err := os.MkdirTemp("/tmp", "fl-yazi-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDir) })
	t.Setenv("TMPDIR", socketDir)
}

// Opt in where the actual tools are installed. Their configuration and state
// stay in the test directory, and only navigation/help/quit keys are sent.
func TestTaskInstalledTUIs(t *testing.T) {
	if os.Getenv("FIVELEAF_TASK_TUI_SMOKE") != "1" || runtime.GOOS == "windows" {
		t.Skip("set FIVELEAF_TASK_TUI_SMOKE=1 with lazygit and yazi installed")
	}
	isolateTaskTUIs(t)
	withAppearancePlugins(t)
	for _, tool := range []string{"lazygit", "yazi"} {
		t.Run(tool, func(t *testing.T) {
			binary := os.Getenv("FIVELEAF_TASK_SMOKE_" + strings.ToUpper(tool))
			if binary == "" {
				var err error
				binary, err = exec.LookPath(tool)
				if err != nil {
					t.Fatal(err)
				}
			}
			hs := taskTermHarness(t)
			hs.gitRepo()
			hs.a.setTheme(themeOneDarkPro)
			config := t.TempDir()
			if tool == "yazi" {
				t.Setenv("TERM", "dumb")
				t.Setenv("NO_COLOR", "1")
				if err := os.WriteFile(filepath.Join(config, "theme.toml"), []byte("[mgr]\ncwd = { fg = \"#17ad68\" }\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			lazygitConfig := filepath.Join(config, "lazygit.yml")
			if err := os.WriteFile(lazygitConfig, []byte("disableStartupPopups: true\ngui:\n  showRandomTip: false\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			definition := tasks.Task{Label: tool, Command: `exec "$FIVELEAF_TASK_BINARY"`, Env: map[string]string{
				"FIVELEAF_TASK_BINARY": binary,
				"XDG_CONFIG_HOME":      config, "XDG_STATE_HOME": config, "XDG_CACHE_HOME": config,
				"YAZI_CONFIG_HOME": config, "YAZI_STATE_HOME": config, "LG_CONFIG_FILE": lazygitConfig,
			}}
			resolved, err := tasks.Resolve(definition, hs.a.taskContext())
			if err != nil {
				t.Fatal(err)
			}
			s := hs.a.launchTask(hs.root, resolved)
			if s == nil {
				t.Fatal(hs.a.errors)
			}
			if s.tab == nil || hs.a.activeTab() != s.tab || hs.a.window.PanelOpen {
				t.Fatalf("%s did not open in an editor tab with the panel closed", tool)
			}
			hs.until(tool+" rendered", func() bool {
				return s.task.completed || len(strings.TrimSpace(s.term.Text())) > 120
			})
			if !taskRunning(s) {
				t.Fatalf("%s exited before interaction: %s", tool, s.term.Text())
			}
			if tool == "yazi" {
				hs.until("Yazi drew its configured directory color", func() bool {
					return strings.Contains(strings.ReplaceAll(string(s.term.Snapshot()), ":", ";"), "23;173;104")
				})
				hs.until("Yazi drew its selected row", func() bool {
					snapshot := strings.ReplaceAll(string(s.term.Snapshot()), ":", ";")
					return strings.Contains(snapshot, "48;5;")
				})
			}
			if len(hs.a.terms) != 1 || hs.a.launchTask(hs.root, resolved) != s {
				t.Fatal("launching the running TUI did not reuse it")
			}
			hs.settle()
			before := s.term.Text()
			key := "?"
			if tool == "yazi" {
				key = "~"
			}
			hs.tt.Type(key)
			hs.until(tool+" accepted keyboard input", func() bool { return s.term.Text() != before })
			if tool == "lazygit" {
				hs.until("lazygit opened help", func() bool { return strings.Contains(s.term.Text(), "Copy path to clipboard") })
			}
			hs.shot("task-" + tool + "-help")
			hs.tt.Key(0, ui.KeyEscape)
			if tool == "lazygit" {
				// The normal footer also says Keybindings; wait for the help's contents to leave.
				hs.until("lazygit closed help", func() bool { return !strings.Contains(s.term.Text(), "Copy path to clipboard") })
			}
			hs.settle()
			hs.tt.SetSize(960, 640)
			hs.settle()
			if !taskRunning(s) {
				t.Fatalf("TUI stopped on resize: %s", s.term.Text())
			}
			hs.shot("task-" + tool)
			hs.tt.Type("q")
			hs.until(tool+" quit", func() bool { return s.task.completed })
			if s.task.exitCode != 0 || len(hs.a.terms) != 1 {
				t.Fatalf("TUI did not keep its successful exit: code=%d terminals=%d output=%s", s.task.exitCode, len(hs.a.terms), s.term.Text())
			}
		})
	}
}

func TestTaskYaziChoosesActiveFile(t *testing.T) {
	if os.Getenv("FIVELEAF_TASK_TUI_SMOKE") != "1" {
		t.Skip("set FIVELEAF_TASK_TUI_SMOKE=1 with yazi installed")
	}
	binary := os.Getenv("FIVELEAF_TASK_SMOKE_YAZI")
	if binary == "" {
		var err error
		binary, err = exec.LookPath("yazi")
		if err != nil {
			t.Fatal(err)
		}
	}
	isolateTaskTUIs(t)
	config := t.TempDir()
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR", "YAZI_CONFIG_HOME", "FIVELEAF_DATA_DIR"} {
		t.Setenv(name, config)
	}
	chooser := filepath.Join(config, "chosen")
	cwdFile := filepath.Join(config, "cwd")
	t.Setenv("FIVELEAF_TASK_BINARY", binary)
	t.Setenv("FIVELEAF_TASK_CHOOSER", chooser)
	t.Setenv("FIVELEAF_TASK_CWD", cwdFile)
	profile := filepath.Join(config, "shrc")
	if err := os.WriteFile(profile, []byte(`yazi() { exec "$FIVELEAF_TASK_BINARY" --chooser-file "$FIVELEAF_TASK_CHOOSER" --cwd-file "$FIVELEAF_TASK_CWD" "$@"; }`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", profile)
	hs := taskUIHarness(t, true)
	a := hs.a
	parent := hs.path("nested files")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	fileA := filepath.Join(parent, "middle A's $HOME.txt")
	fileB := filepath.Join(parent, `z B "$HOME".txt`)
	dotfile := filepath.Join(parent, ".env-$HOME")
	for _, path := range []string{filepath.Join(parent, "00-decoy.txt"), fileA, fileB, dotfile} {
		if err := os.WriteFile(path, []byte("test file\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a.openFile(fileA, true)
	a.run("task.yazi")
	if len(a.terms) != 1 {
		t.Fatalf("direct command: terminals=%d errors=%v", len(a.terms), a.errors)
	}
	first := a.terms[0]
	hs.until("A's parent", func() bool { return strings.Contains(first.term.Text(), "00-decoy.txt") })
	hs.key(0, ui.KeyUp)
	writeTaskDefinitions(t, a.projectTasksPath(), tasks.Task{Label: "yazi", Command: "yazi"})
	a.openFile(fileB, true)
	a.openTaskPicker()
	a.chooseTask("yazi")
	hs.settle()
	if len(a.terms) != 2 || !taskRunning(first) {
		t.Fatalf("A → B: got %d terminals, first running=%v, errors=%v", len(a.terms), taskRunning(first), a.errors)
	}
	second := a.terms[1]
	choose := func(want string, session *session) {
		t.Helper()
		hs.settle()
		if session.tab == nil || a.activeTab() != session.tab || a.window.PanelOpen {
			t.Fatalf("expected the active editor tab to show Yazi for %q with the panel closed", want)
		}
		hs.until("Yazi directory listing", func() bool {
			return session.task.completed || strings.Contains(session.term.Text(), "00-decoy.txt")
		})
		if !taskRunning(session) {
			t.Fatalf("Yazi exited before choosing: %s", session.term.Text())
		}
		hs.key(0, ui.KeyEnter)
		hs.until("Yazi chooser exit", func() bool { return session.task.completed })
		selected, err := os.ReadFile(chooser)
		if err != nil || strings.TrimSuffix(string(selected), "\n") != want || session.task.exitCode != 0 {
			t.Fatalf("Enter chose %q, want %q (exit %d, err %v)", selected, want, session.task.exitCode, err)
		}
		t.Logf("Enter chose %q", string(selected))
		if err := os.Remove(chooser); err != nil {
			t.Fatal(err)
		}
	}
	a.openFile(fileA, true)
	withKeys(t, a, `[{"key":"ctrl+shift+f8","command":"task.spawn","args":{"task_name":"yazi"}}]`)
	hs.key(ui.Ctrl|ui.Shift, ui.KeyF8)
	if len(a.terms) != 3 || !taskRunning(first) || !taskRunning(second) {
		t.Fatalf("reopening A after moving its cursor: terminals=%d, A running=%v, B running=%v", len(a.terms), taskRunning(first), taskRunning(second))
	}
	third := a.terms[2]
	choose(fileA, third)
	previous := third.term
	a.openFile(fileB, true)
	a.run("task.rerun")
	if len(a.terms) != 3 || third.term == previous || !taskRunning(first) || !taskRunning(second) {
		t.Fatal("rerun did not reuse the finished terminal and preserve both live processes")
	}
	choose(fileB, third)
	a.openFile(dotfile, true)
	a.run("task.yazi")
	choose(dotfile, third)
	t.Run("yazi without ya", func(t *testing.T) {
		bin := t.TempDir()
		if err := os.Symlink(binary, filepath.Join(bin, "yazi")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		if err := os.WriteFile(filepath.Join(config, "yazi.toml"), []byte("[mgr]\nshow_hidden = true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		a.openFile(dotfile, true)
		a.run("task.yazi")
		if !taskRunning(third) {
			t.Fatalf("ordinary Yazi did not launch: %v", a.errors)
		}
	})
	choose(dotfile, third)
	a.showTerminal(first)
	choose(filepath.Join(parent, "00-decoy.txt"), first)
	a.showTerminal(second)
	choose(fileB, second)
	t.Cleanup(func() { a.root = hs.root })
	for _, root := range []string{hs.root, ""} {
		a.root = root
		want, err := filepath.EvalSymlinks(a.workDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(want, "cwd-marker.txt"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		a.run("task.yazi")
		hs.settle()
		session := a.activeTab().term
		hs.until("Yazi fallback directory", func() bool { return strings.Contains(session.term.Text(), "cwd-marker.txt") })
		hs.tt.Type("q")
		hs.until("Yazi directory exit", func() bool { return session.task.completed })
		cwd, err := os.ReadFile(cwdFile)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := filepath.EvalSymlinks(strings.TrimSuffix(string(cwd), "\n"))
		if err != nil || actual != want {
			t.Fatalf("without a file: cwd=%q, want %q, err=%v", cwd, want, err)
		}
	}
}

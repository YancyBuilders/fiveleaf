package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/internal/tasks"
	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func TestTaskShellArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs Unix shells")
	}
	for _, shell := range []string{"sh", "bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			bin, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("%s is not installed", shell)
			}
			dir := t.TempDir()
			marker := filepath.Join(dir, "injected")
			args := []string{"", "a b", "single'quote", `double"quote`, `back\slash`, "$HOME", "$(touch " + marker + ")", "`touch " + marker + "`", "semi;colon", "line\nbreak"}
			resolved := tasks.Resolved{Task: tasks.Task{Command: `printf '%s\n'`}, Args: args}
			command, err := taskShellCommand(resolved, "linux", bin)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(command[0], command[1:]...)
			cmd.Env = append(os.Environ(), "HOME="+dir, "ZDOTDIR="+dir, "ENV=/dev/null", "BASH_ENV=/dev/null")
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("shell: %v", err)
			}
			if got, want := string(out), strings.Join(args, "\n")+"\n"; got != want {
				t.Fatalf("literal arguments = %q; want %q", got, want)
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("argument was executed: %v", err)
			}
		})
	}
}

func TestTaskShellSelection(t *testing.T) {
	resolved := tasks.Resolved{Task: tasks.Task{Command: "tool $SOURCE"}, Args: []string{"a b", "single'quote", `$HOME; $(tool)`, `back\slash`}}
	command, err := taskShellCommand(resolved, "darwin", "")
	if err != nil || command[0] != "/bin/zsh" || !strings.HasPrefix(command[len(command)-1], "tool $SOURCE ") {
		t.Fatalf("default shell changed the source: %q, %v", command, err)
	}
	command, err = taskShellCommand(resolved, "linux", "/usr/bin/fish")
	if err != nil || !strings.Contains(command[len(command)-1], `'single\'quote'`) || !strings.Contains(command[len(command)-1], `'back\\slash'`) {
		t.Fatalf("fish quoting: %q, %v", command, err)
	}
	previous := lookPath
	t.Cleanup(func() { lookPath = previous })
	lookPath = func(name string) (string, error) {
		if name == "pwsh.exe" {
			return `C:\Program Files\PowerShell\7\pwsh.exe`, nil
		}
		return "", exec.ErrNotFound
	}
	command, err = taskShellCommand(resolved, "windows", "ignored")
	if err != nil || !strings.Contains(command[0], "pwsh.exe") || !strings.Contains(command[len(command)-1], `'single''quote'`) || !strings.Contains(command[len(command)-1], "exit $LASTEXITCODE") {
		t.Fatalf("PowerShell: %q, %v", command, err)
	}
	line := command[len(command)-1]
	mode := strings.Index(line, "$PSNativeCommandArgumentPassing = 'Standard'")
	if !strings.HasPrefix(line, "if ($PSVersionTable.PSVersion -lt [version]'7.3')") || mode < 0 || mode > strings.Index(line, resolved.Task.Command) {
		t.Fatalf("native argv support was not enforced before the command: %s", line)
	}
	lookPath = func(name string) (string, error) {
		if name == "powershell.exe" {
			return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, nil
		}
		return "", exec.ErrNotFound
	}
	if _, err := taskShellCommand(resolved, "windows", ""); err == nil || !strings.Contains(err.Error(), "7.3") {
		t.Fatal("legacy Windows PowerShell was accepted without pwsh 7.3+")
	}
	if _, err := taskShellCommand(resolved, "linux", "/bin/powershell"); err == nil {
		t.Fatal("legacy PowerShell was accepted")
	}
	if _, err := taskShellCommand(resolved, "linux", "/bin/unsupported"); err == nil {
		t.Fatal("unknown shell was accepted")
	}
	resolved.Args = []string{"bad\x00arg"}
	if _, err := taskShellCommand(resolved, "linux", "/bin/sh"); err == nil {
		t.Fatal("NUL argument was accepted")
	}
}

func TestTaskPowerShellArgvProbe(t *testing.T) {
	if os.Getenv("FIVELEAF_TASK_ARGV_PROBE") != "1" {
		return
	}
	at := slices.Index(os.Args, "--")
	if at < 0 || json.NewEncoder(os.Stdout).Encode(os.Args[at+1:]) != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestTaskPowerShellNativeArguments(t *testing.T) {
	bin, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell 7.3+ is not installed")
	}
	check := exec.Command(bin, "-NoLogo", "-NoProfile", "-Command", "if ($PSVersionTable.PSVersion -lt [version]'7.3') { exit 1 }")
	if err := check.Run(); err != nil {
		t.Skip("PowerShell 7.3+ is not available")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"", "a b", `embedded"quote`, "single'quote", `back\slash\`, "$HOME", "line\nbreak"}
	resolved := tasks.Resolved{
		Task: tasks.Task{Command: "& '" + strings.ReplaceAll(executable, "'", "''") + "'"},
		Args: append([]string{"-test.run=^TestTaskPowerShellArgvProbe$", "--"}, want...),
	}
	command, err := taskShellCommand(resolved, runtime.GOOS, bin)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = append(os.Environ(), "FIVELEAF_TASK_ARGV_PROBE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("native argv probe: %v", err)
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil || !slices.Equal(got, want) {
		t.Fatalf("native args = %q, want %q; decode: %v; output: %q", got, want, err, out)
	}
}

func TestTaskTerminalEnvironmentPreservesWindowsNames(t *testing.T) {
	inherited := []string{"Path=C:\\old", "UserProfile=C:\\Users\\test", "KEEP=value"}
	overrides := []string{"PATH=C:\\tools", "USERPROFILE=C:\\other", "NEW=value"}
	got := taskTerminalEnv(overrides, inherited, "windows")
	want := []string{"Path=C:\\tools", "UserProfile=C:\\other", "NEW=value"}
	if !slices.Equal(got, want) {
		t.Fatalf("Windows names = %q, want %q", got, want)
	}
	if got := taskTerminalEnv(overrides, inherited, "linux"); !slices.Equal(got, overrides) {
		t.Fatalf("POSIX names changed: %q", got)
	}
	aliases := []string{"Path=first", "PATH=second", "path=third", "Path=duplicate", "Term=screen"}
	got = taskTerminalEnv([]string{"PATH=updated", "TERM=custom"}, aliases, "windows")
	want = []string{"Path=updated", "PATH=updated", "path=updated", "Term=custom", "TERM=custom"}
	if !slices.Equal(got, want) {
		t.Fatalf("existing aliases did not receive the same value: %q, want %q", got, want)
	}
}

func TestLocalTerminalColorOverrides(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		launcher string
		extra    []string
		want     string
	}{
		{"dumb", nil, ""},
		{"dumb", []string{"NO_COLOR=task-choice"}, "task-choice"},
		{"xterm-256color", nil, "1"},
	} {
		t.Setenv("TERM", tc.launcher)
		color := os.Getenv("NO_COLOR")
		for _, entry := range localTerminalEnv(tc.extra) {
			if value, ok := strings.CutPrefix(entry, "NO_COLOR="); ok {
				color = value
			}
		}
		if color != tc.want || os.Getenv("NO_COLOR") != "1" {
			t.Errorf("launcher %s, task environment %v: NO_COLOR = %q, want %q", tc.launcher, tc.extra, color, tc.want)
		}
	}
}

func taskTermHarness(t *testing.T) *harness {
	t.Helper()
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("starts Unix tasks in a PTY")
	}
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("ENV", "/dev/null")
	return newHarness(t, true)
}

func TestTaskTerminalRunsWithPTYDirectoryEnvironmentAndArguments(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	dir := hs.path("task's working dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(dir, "task's probe.sh")
	script := "test -t 0 && test -t 1 && printf 'task-tty\\n'\npwd\nprintf 'env=<%s>\\n' \"$FIVELEAF_TASK_PROBE\"\nprintf 'arg=<%s>\\n' \"$@\"\nprintf '\\033]0;changed title\\007'\nexit 7\n"
	if err := os.WriteFile(probe, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := tasks.Resolved{
		Task: tasks.Task{Label: "Task probe", Command: "/bin/sh"},
		Dir:  dir,
		Args: []string{probe, "two words", "single'quote", "$(echo injection)"},
		Env:  []string{"FIVELEAF_TASK_PROBE=literal $value; 'quote'"},
	}
	s := a.launchTask(hs.root, resolved)
	if s == nil {
		t.Fatalf("task did not start: %v", a.errors)
	}
	hs.until("task exit", func() bool { return s.task.completed })
	if s.task.exitCode != 7 || len(a.terms) != 1 || s.tab == nil {
		t.Fatalf("task state: %+v, %d terminals", s.task, len(a.terms))
	}
	text := s.task.output
	for _, want := range []string{"task-tty", dir, "env=<literal $value; 'quote'>", "arg=<two words>", "arg=<single'quote>", "arg=<$(echo injection)>", "[Process exited with code 7]"} {
		if !strings.Contains(text, want) {
			t.Errorf("terminal does not contain %q: %q", want, text)
		}
	}
	if s.label() != "Task probe" {
		t.Fatalf("task title changed to %q", s.label())
	}
	s.name = "My task"
	if s.label() != "My task" {
		t.Fatalf("task ignored its user's name: %q", s.label())
	}
	if hs.tt.HasText(locales.ReportProblem()) || !hs.tt.HasText(locales.TaskRerun()) {
		t.Fatalf("exit did not offer task rerun: %q", hs.tt.Texts())
	}
}

func waitingTask(root, label string) tasks.Resolved {
	return tasks.Resolved{
		Task: tasks.Task{Label: label, Command: "/bin/sh"}, Dir: root,
		Args: []string{"-c", `printf 'task-ready\n'; read answer; printf 'answer=<%s>\n' "$answer"`},
	}
}

func TestTaskTerminalReusesAndRestarts(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.run("view.terminal")
	hs.settle()
	shell := a.panelTerm
	if shell == nil || shell.task != nil || !shell.view.FocusWithin(a.review.ctx) {
		t.Fatalf("opening the panel did not start a shell: %v", a.errors)
	}
	shellID, shellPTY := shell.id, shell.term
	shellPTY.Send([]byte("fiveleaf_shell_probe=preserved\r"))
	checkShell := func(open bool) {
		t.Helper()
		if a.panelTerm != shell || shell.id != shellID || shell.term != shellPTY || shell.task != nil || !slices.Equal(a.panelTerms(), []*session{shell}) {
			t.Fatal("task operation changed the selected panel shell or its PTY")
		}
		if a.window.PanelOpen != open || !a.window.TerminalOpen {
			t.Fatalf("panel open=%v, shell restore=%v, want %v and true", a.window.PanelOpen, a.window.TerminalOpen, open)
		}
	}
	resolved := waitingTask(hs.root, "Reusable")
	s := a.launchTask(hs.root, resolved)
	if s == nil {
		t.Fatal(a.errors)
	}
	hs.settle()
	hs.until("task ready", func() bool { return strings.Contains(s.term.Text(), "task-ready") })
	tab, firstPTY := s.tab, s.term
	if tab == nil || tab.term != s || a.activeTab() != tab || firstPTY == shellPTY {
		t.Fatal("task did not open its own active editor tab and PTY")
	}
	if a.focus != paneEditor || !s.view.FocusWithin(a.review.ctx) {
		t.Fatalf("active task tab focus: pane=%v, task focused=%v, shell focused=%v", a.focus, s.view.FocusWithin(a.review.ctx), shell.view.FocusWithin(a.review.ctx))
	}
	checkShell(true)
	a.hidePanel()
	a.activate(a.column(), 0, true)
	if again := a.launchTask(hs.root, resolved); again != s || s.term != firstPTY || len(a.terms) != 2 || a.activeTab() != tab || a.focus != paneEditor {
		t.Fatal("running task was duplicated or not focused")
	}
	checkShell(false)
	s.term.Send([]byte("done\r"))
	hs.until("first task exit", func() bool { return s.task.completed })
	if s.task.exitCode != 0 || !strings.Contains(s.task.output, "answer=<done>") {
		t.Fatalf("first exit = %d, output = %q", s.task.exitCode, s.task.output)
	}
	old := s.term
	a.run("view.terminal")
	hs.settle()
	checkShell(true)
	hs.settle()
	if len(a.terms) != 1 || a.panelTerm != shell {
		t.Fatal("the exited task kept its editor tab")
	}
	if again := a.launchTask(hs.root, resolved); again == nil || again == s || again.term == old || len(a.terms) != 2 || again.tab == nil || a.activeTab() != again.tab || a.focus != paneEditor {
		t.Fatal("running the completed task did not open a fresh editor tab")
	}
	s = a.terms[1]
	checkShell(true)
	hs.until("restarted task ready", func() bool { return strings.Contains(s.term.Text(), "task-ready") })
	if s.task.completed || strings.Contains(s.term.Text(), "answer=<done>") {
		t.Fatal("previous terminal changed the new task")
	}
	s.term.Send([]byte("again\r"))
	hs.until("second task exit", func() bool { return s.task.completed })
	hs.settle()
	if !strings.Contains(s.task.output, "answer=<again>") {
		t.Fatal("the second task's output was lost")
	}
	a.run("file.close")
	hs.settle()
	if len(a.terms) != 1 || a.terms[0] != shell || a.activeTab() != nil {
		t.Fatal("the exited task disturbed the original shell")
	}
	checkShell(true)
	shellPTY.Send([]byte("printf 'panel-state=<%s>\\n' \"$fiveleaf_shell_probe\"\r"))
	hs.until("original shell state after closing the task", func() bool { return strings.Contains(shellPTY.Text(), "panel-state=<preserved>") })
}

func TestTaskTerminalConcurrencyAndNewTerminal(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	resolved := waitingTask(hs.root, "Concurrent")
	first := a.launchTask(hs.root, resolved)
	if first == nil {
		t.Fatal(a.errors)
	}
	resolved.Task.AllowConcurrentRuns = true
	second := a.launchTask(hs.root, resolved)
	if second == nil || second == first || len(a.terms) != 2 {
		t.Fatal("concurrent task did not get its own terminal")
	}
	resolved.Task.AllowConcurrentRuns = false
	if again := a.launchTask(hs.root, resolved); again != second {
		t.Fatalf("non-concurrent launch did not focus the running task: terms=%d fails=%d", len(a.terms), a.fails)
	}
	first.term.Send([]byte("first\r"))
	hs.until("first task exit", func() bool { return first.task.completed })
	second.term.Send([]byte("second\r"))
	hs.until("second task exit", func() bool { return second.task.completed })
	hs.settle()
	resolved.Task.UseNewTerminal = true
	third := a.launchTask(hs.root, resolved)
	if third == nil || third == first || third == second || len(a.terms) != 1 {
		t.Fatalf("use_new_terminal did not start a fresh task: terms=%d firstRunning=%v secondRunning=%v fails=%d", len(a.terms), taskRunning(first), taskRunning(second), a.fails)
	}
	other := a.launchTask(hs.root+"-other", resolved)
	if other == nil || other == third || len(a.terms) != 2 {
		t.Fatal("tasks from different workspaces shared a terminal")
	}
}

func TestTaskTerminalRestartsBeforeExitCallback(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	resolved := tasks.Resolved{Task: tasks.Task{Label: "Quick task", Command: "exit 6"}, Dir: hs.root}
	s := a.launchTask(hs.root, resolved)
	if s == nil {
		t.Fatal(a.errors)
	}
	select {
	case <-s.term.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("quick task did not exit")
	}
	if s.task.completed {
		t.Fatal("exit callback ran before the next frame")
	}
	old := s.term
	resolved = waitingTask(hs.root, "Quick task")
	if again := a.launchTask(hs.root, resolved); again != s || s.term == old {
		t.Fatal("exited process was treated as a running task")
	}
	hs.until("replacement task ready", func() bool { return strings.Contains(s.term.Text(), "task-ready") })
	if s.task.completed || s.task.exitCode != 0 || !taskRunning(s) {
		t.Fatal("queued exit callback ended the replacement task")
	}
}

func TestTaskTerminalExitClosesOnlyItsTab(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	file := a.activeTab()
	resolved := tasks.Resolved{Task: tasks.Task{Label: "Toast task", Command: "exit 0"}, Dir: hs.root}
	s := a.launchTask(hs.root, resolved)
	if s == nil {
		t.Fatal(a.errors)
	}
	select {
	case <-s.term.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("task did not exit")
	}
	for deadline := time.Now().Add(5 * time.Second); !s.task.completed; {
		a.drain()
		if time.Now().After(deadline) {
			t.Fatal("exit callback did not arrive")
		}
		time.Sleep(time.Millisecond)
	}
	hs.settle()
	if s.task.exitCode != 0 || len(a.terms) != 0 || s.tab != nil || a.activeTab() != file || len(a.notes) != 0 {
		t.Fatal("task exit left its tab or notification behind")
	}
}

func TestTaskTerminalDoesNotRestoreOrReopenAsShell(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	s := a.launchTask(hs.root, waitingTask(hs.root, "Transient task"))
	if s == nil || s.tab == nil || a.activeTab() != s.tab || a.window.PanelOpen || a.window.TerminalOpen {
		t.Fatalf("a lone task should open an editor tab with the shell panel closed: %v", a.errors)
	}
	a.rememberTabs()
	for _, col := range a.window.Columns {
		if slices.ContainsFunc(col.Tabs, func(tab settings.Tab) bool { return tab.Terminal }) {
			t.Fatal("task was saved as an ordinary terminal")
		}
	}
	saved := *a.window
	restoredApp := newApp(&fakeHost{folder: hs.root}, settings.Default(), &saved)
	t.Cleanup(restoredApp.shutdown)
	restored := &harness{t: t, a: restoredApp, root: hs.root, tt: ui.NewTester(restoredApp.view, 1200, 760)}
	restored.settle()
	if restoredApp.activeFile() != hs.path("main.go") {
		t.Fatalf("restored file = %q, want main.go", restoredApp.activeFile())
	}
	if len(restoredApp.terms) != 0 || restoredApp.window.PanelOpen || restoredApp.window.TerminalOpen {
		t.Fatal("restoring a window with only a task terminal reopened a shell")
	}
	a.closeTab(s.tab, nil, nil)
	if len(a.closed) != 0 {
		t.Fatal("task was added to the ordinary shell reopen list")
	}
	hs.settle()
	if len(a.terms) != 0 || hs.tt.HasText(locales.TaskRerun()) {
		t.Fatal("closing a task retained it or announced its exit")
	}
}

func TestTaskTerminalStaysInEditorAcrossPanelActions(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	file := a.activeTab()
	s := a.launchTask(hs.root, waitingTask(hs.root, "Pinned task"))
	if s == nil || s.tab == nil || a.activeTab() != s.tab {
		t.Fatalf("default terminal location did not focus a task tab: %v", a.errors)
	}
	hs.until("task ready", func() bool { return strings.Contains(s.term.Text(), "task-ready") })
	col, tab, taskPTY := a.column(), s.tab, s.term
	_, at := a.locate(file)
	a.activate(col, at, true)
	hs.settle()
	if a.activeEditor() == nil || !a.activeEditor().vscroll.Focused(hs.a.review.ctx) {
		t.Fatal("the editor did not take the keyboard after the task")
	}
	before := len(strings.TrimSpace(a.activeEditor().buf.String()))
	hs.tt.Type("k")
	hs.settle()
	if got := len(strings.TrimSpace(a.activeEditor().buf.String())); got != before+1 || a.activeTab() != file {
		t.Fatal("typing after opening the task went to the task instead of the editor")
	}
	a.showTerminal(s)
	hs.settle()
	checkTask := func() {
		t.Helper()
		if owner, _ := a.locate(tab); owner != col || s.tab != tab || tab.term != s || s.term != taskPTY || !slices.Contains(a.terms, s) || !taskRunning(s) {
			t.Fatal("panel action moved, replaced or stopped the task")
		}
		if a.panelTerm == s || slices.Contains(a.panelTerms(), s) {
			t.Fatal("panel action selected or listed the task in the panel")
		}
	}
	click := func(label string) {
		t.Helper()
		if err := hs.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	a.terminalToPanel(s)
	checkTask()
	if len(a.terms) != 1 || a.window.PanelOpen || a.window.TerminalOpen {
		t.Fatal("attempting to dock a task changed the shell panel")
	}
	openShell := func(command string) *session {
		t.Helper()
		a.run(command)
		hs.settle()
		shell := a.panelTerm
		if shell == nil || shell.task != nil || shell.term == taskPTY || len(a.terms) != 2 {
			t.Fatalf("%s with only a task tab did not open a separate shell", command)
		}
		if !a.window.PanelOpen || !a.window.TerminalOpen || a.activeTab() != tab {
			t.Fatalf("%s did not show a restorable panel shell beside the task tab", command)
		}
		checkTask()
		return shell
	}
	shell := openShell("view.newTerminal")
	click(locales.PanelMaximize())
	if shell.tab == nil || a.activeTab() != shell.tab || a.panelTerm != nil || a.window.PanelOpen || a.window.TerminalOpen {
		t.Fatal("maximizing the shell did not focus its own editor tab and clear the panel restore flag")
	}
	checkTask()
	click(locales.PanelRestore())
	if a.panelTerm != shell || shell.tab != nil || !a.window.PanelOpen || !a.window.TerminalOpen || a.activeTab() != tab {
		t.Fatal("restoring the shell did not return it to the panel beside the task tab")
	}
	checkTask()
	click(locales.TermClose())
	if len(a.terms) != 1 || a.panelTerm != nil || a.window.PanelOpen || a.window.TerminalOpen || a.activeTab() != tab {
		t.Fatal("closing the last panel shell did not leave the task tab with the panel closed")
	}
	checkTask()
	if fresh := openShell("view.terminal"); fresh == shell || fresh.term == shell.term {
		t.Fatal("toggling the panel reused the closed shell")
	}
	a.showTerminal(s)
	hs.settle()
	hs.tt.Type("survived")
	hs.key(0, ui.KeyEnter)
	hs.until("task accepted input after reopening the panel", func() bool { return s.task.completed })
	if s.task.exitCode != 0 || !strings.Contains(s.task.output, "answer=<survived>") {
		t.Fatalf("task exit after panel actions = %d, output = %q", s.task.exitCode, s.task.output)
	}
}

func TestTaskTerminalRejectsRemoteAndMissingShell(t *testing.T) {
	hs := taskTermHarness(t)
	a := hs.a
	resolved := waitingTask(hs.root, "Local task")
	if a.launchTask(remote.Path("remote.invalid", "/srv/project"), resolved) != nil || len(a.terms) != 0 {
		t.Fatal("remote workspace launched a local task")
	}
	resolved.Dir = remote.Path("remote.invalid", "/srv/project")
	if a.launchTask(hs.root, resolved) != nil || len(a.terms) != 0 {
		t.Fatal("remote working directory launched a local task")
	}
	resolved.Dir = hs.root
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "sh"))
	if a.launchTask(hs.root, resolved) != nil || len(a.terms) != 0 {
		t.Fatal("missing shell created a terminal")
	}
	t.Setenv("SHELL", "/bin/sh")
	resolved.Task.Command = "fiveleaf_missing_task_program_123456789"
	resolved.Args = nil
	s := a.launchTask(hs.root, resolved)
	if s == nil {
		t.Fatal("missing program did not leave a task terminal")
	}
	hs.until("missing program exit", func() bool { return s.task.completed })
	if at, _ := a.locate(s.tab); at == nil || !strings.Contains(s.term.Text(), resolved.Task.Command) {
		t.Fatalf("missing program failure was lost: %d, %q", s.task.exitCode, s.term.Text())
	}
}

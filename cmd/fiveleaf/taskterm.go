package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/tasks"
	"fiveleaf/locales"

	"github.com/egoist/mygo/plugins/terminal"
)

type taskRun struct {
	resolved  tasks.Resolved
	root      string
	completed bool
	exitCode  int
	output    string
}

func (run *taskRun) invocation() taskInvocation {
	return taskInvocation{label: run.resolved.Task.Label, root: run.root, context: run.resolved.Context}
}

// taskRunning also checks the process: its exit may still be queued for
// the next frame when another request to run the task arrives.
func taskRunning(s *session) bool {
	if s.task == nil || s.term == nil || s.task.completed {
		return false
	}
	select {
	case <-s.term.Done():
		return false
	default:
		return true
	}
}

func (a *app) launchTask(root string, task tasks.Resolved) *session {
	if remote.Is(root) || remote.Is(task.Dir) {
		a.fail("%s", locales.FailTaskRemote())
		return nil
	}
	if file := task.YaziFile(); file != "" && strings.HasPrefix(filepath.Base(file), ".") {
		return a.launchHiddenYazi(root, task)
	}
	return a.launchTaskTerminal(root, task)
}

func (a *app) launchTaskTerminal(root string, task tasks.Resolved) *session {
	fresh := task.YaziFile() != ""
	if !fresh {
		if s := a.focusRunningTask(root, task.Task); s != nil {
			return s
		}
	}
	var finished *session
	running := false
	for i := len(a.terms) - 1; i >= 0; i-- {
		s := a.terms[i]
		if s.task == nil || s.task.root != root || s.task.resolved.Task.Label != task.Task.Label {
			continue
		}
		if taskRunning(s) {
			running = true
		} else if finished == nil {
			finished = s
		}
	}
	if (fresh || !running) && finished != nil && !task.Task.UseNewTerminal {
		return a.restartTask(finished, root, task)
	}
	s := &session{dir: task.Dir, title: task.Task.Label, task: &taskRun{resolved: task, root: root}}
	if a.addTerminal(s) == nil {
		return nil
	}
	col := a.fileColumn()
	a.terminalTab(col, col.active+1, s, true)
	return s
}

func (a *app) restartTask(s *session, root string, task tasks.Resolved) *session {
	previous, dir := s.task, s.dir
	s.task, s.dir = &taskRun{resolved: task, root: root}, task.Dir
	t, err := a.spawnTerminal(s, task.Dir)
	if err != nil {
		s.task, s.dir = previous, dir
		a.fail("%s", locales.FailStartTask(task.Task.Label, err.Error()))
		return nil
	}
	old := s.term
	s.term, s.started, s.title = t, time.Now(), task.Task.Label
	s.status, s.statusSince, s.statusFrom = terminal.ProgramStatus{}, time.Time{}, terminal.StatusIdle
	s.findOpen, s.findQuery, s.findShook = false, "", time.Time{}
	s.found = terminal.SearchStatus{}
	old.Close()
	a.showTerminal(s)
	return s
}

func (a *app) taskExited(s *session, t *terminal.Terminal, code int) {
	if s.term != t || s.task == nil || s.task.completed || !slices.Contains(a.terms, s) {
		return
	}
	run := s.task
	run.completed, run.exitCode = true, code
	s.status, s.statusSince, s.statusFrom = terminal.ProgramStatus{}, time.Time{}, terminal.StatusIdle
	message := locales.TaskExited(run.resolved.Task.Label, strconv.Itoa(code))
	t.Feed([]byte("\r\n\x1b[0;2m" + message + "\x1b[0m\r\n"))
	run.output = t.Text()
	if code != 0 {
		invocation := run.invocation()
		a.tell(note{message: message, label: locales.TaskRerun(), action: func() {
			if a.root == invocation.root {
				a.runTaskWithContext(invocation.label, invocation.context)
			}
		}})
		return
	}
	a.post(func() { a.closeExitedTask(s, run, t) })
}

func (a *app) closeExitedTask(s *session, run *taskRun, t *terminal.Terminal) {
	if !slices.Contains(a.terms, s) || s.task != run || s.term != t {
		return
	}
	wasActive := s.tab != nil && a.activeTab() == s.tab
	a.removeTerminal(s)
	if wasActive && a.activeTab() == nil {
		a.focusColumn()
	}
}

// The command is shell source; only args are data. Each shell's literal
// quoting keeps paths and selected text from becoming more shell source.
func taskShellCommand(task tasks.Resolved, platform, shell string) ([]string, error) {
	if strings.TrimSpace(task.Task.Command) == "" || strings.ContainsRune(task.Task.Command, '\x00') {
		return nil, fmt.Errorf("task command is empty or contains a NUL byte")
	}
	if platform == "windows" {
		var err error
		shell, err = lookPath("pwsh.exe")
		if err != nil {
			return nil, fmt.Errorf("tasks on Windows require PowerShell 7.3 or newer (pwsh.exe)")
		}
	} else if shell == "" {
		shell = "/bin/sh"
		if platform == "darwin" {
			shell = "/bin/zsh"
		}
	}
	name := strings.TrimSuffix(strings.ToLower(path.Base(strings.ReplaceAll(shell, "\\", "/"))), ".exe")
	var quote func(string) string
	switch name {
	case "sh", "bash", "zsh", "dash", "ash", "ksh", "mksh":
		quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	case "fish":
		quote = func(s string) string {
			s = strings.ReplaceAll(s, "\\", "\\\\")
			return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
		}
	case "pwsh":
		quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	case "powershell":
		return nil, fmt.Errorf("tasks require PowerShell 7.3 or newer (pwsh); Windows PowerShell is unsupported")
	default:
		return nil, fmt.Errorf("tasks do not support shell %q; use sh, bash, zsh, ksh, fish, or PowerShell", shell)
	}
	line := task.Task.Command
	for _, arg := range task.Args {
		if strings.ContainsRune(arg, '\x00') {
			return nil, fmt.Errorf("task argument contains a NUL byte")
		}
		line += " " + quote(arg)
	}
	if name == "pwsh" {
		// Earlier versions lose empty arguments and embedded quotes. Check
		// before running user source, then preserve native exit codes too.
		line = "if ($PSVersionTable.PSVersion -lt [version]'7.3') { [Console]::Error.WriteLine('FiveLeaf tasks require PowerShell 7.3 or newer (pwsh).'); exit 1 }; $PSNativeCommandArgumentPassing = 'Standard'; $global:LASTEXITCODE = $null; " + line + "\n$__fiveleaf_task_ok = $?; if ($__fiveleaf_task_ok) { exit 0 }; if ($LASTEXITCODE -is [int] -and $LASTEXITCODE -ne 0) { exit $LASTEXITCODE }; exit 1"
		return []string{shell, "-NoLogo", "-Command", line}, nil
	}
	return []string{shell, "-l", "-i", "-c", line}, nil
}

func taskProgram(task tasks.Resolved) ([]string, error) {
	if file := task.YaziFile(); file != "" {
		task.Args = slices.Clone(task.Args)
		task.Args[0] = yaziEntryArgument(file, hostOS)
	}
	return taskShellCommand(task, hostOS, os.Getenv("SHELL"))
}

func localTerminalEnv(extra []string) []string {
	if os.Getenv("TERM") == "dumb" && os.Getenv("NO_COLOR") != "" {
		extra = append([]string{"NO_COLOR="}, extra...)
	}
	if hostOS == "windows" {
		return taskTerminalEnv(extra, os.Environ(), hostOS)
	}
	return extra
}

// The terminal merges Env with exact-case keys, while Windows reads them
// without case. Match every existing spelling without adding another alias.
func taskTerminalEnv(extra, inherited []string, platform string) []string {
	if platform != "windows" {
		return extra
	}
	var env []string
	for _, entry := range extra {
		key, value, _ := strings.Cut(entry, "=")
		seen := map[string]bool{}
		for _, existing := range inherited {
			name, _, _ := strings.Cut(existing, "=")
			if strings.EqualFold(name, key) && !seen[name] {
				env = append(env, name+"="+value)
				seen[name] = true
			}
		}
		// The backend also adds these uppercase keys before merging Env.
		for _, name := range []string{"TERM", "COLORTERM"} {
			if strings.EqualFold(name, key) && !seen[name] {
				env = append(env, name+"="+value)
				seen[name] = true
			}
		}
		if len(seen) == 0 {
			env = append(env, entry)
		}
	}
	return env
}

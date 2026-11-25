package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/tasks"
	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

type taskEntry struct {
	task   tasks.Task
	source string // empty for a built-in, otherwise the configuration file
}

type taskInvocation struct {
	label, root string
	context     tasks.Context
}

type taskConfigError struct {
	path string
	err  error
}

type taskPicker struct {
	open, moved, stale bool
	root, query, shown string
	sel                int
	entries            []taskEntry
	names              []string
	results            []workspace.Match
	errors             []taskConfigError
}

func (a *app) globalTasksPath() string {
	return globalTaskFile(a.settings.Path())
}

func globalTaskFile(settingsPath string) string {
	if settingsPath != "" {
		return filepath.Join(filepath.Dir(settingsPath), "tasks.json")
	}
	return ""
}

func (entry taskEntry) sourceLabel(projectPath string) string {
	switch entry.source {
	case "":
		return locales.TaskBuiltin()
	case projectPath:
		return locales.TaskProject()
	default:
		return locales.TaskGlobal()
	}
}

func (entry taskEntry) detail() string {
	if entry.source == "" {
		switch entry.task.Label {
		case "lazygit":
			return locales.TaskLazygitDescription()
		case "yazi":
			return locales.TaskYaziDescription()
		}
	}
	return strings.Join(strings.Fields(entry.task.Command+" "+strings.Join(entry.task.Args, " ")), " ")
}

func (a *app) projectTasksPath() string {
	if a.root == "" || remote.Is(a.root) {
		return ""
	}
	return filepath.Join(a.root, ".fiveleaf", "tasks.json")
}

// Each file is independent: a broken project file leaves global and built-in
// tasks available, with the error alongside them.
func readTaskCatalog(paths ...string) ([]taskEntry, []taskConfigError) {
	groups := [][]tasks.Task{tasks.Builtins()}
	sources := map[string]string{}
	var problems []taskConfigError
	for _, path := range paths {
		if path == "" {
			continue
		}
		ts, err := readTaskFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				problems = append(problems, taskConfigError{path: path, err: err})
			}
			continue
		}
		groups = append(groups, ts)
		for _, task := range ts {
			sources[task.Label] = path
		}
	}
	var entries []taskEntry
	for _, task := range tasks.Merge(groups...) {
		entries = append(entries, taskEntry{task: task, source: sources[task.Label]})
	}
	return entries, problems
}

func readTaskFile(path string) ([]tasks.Task, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("tasks.json must be a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("tasks.json exceeds 1 MiB")
	}
	return tasks.Parse(data)
}

func (a *app) openTaskPicker() {
	if remote.Is(a.root) {
		a.fail("%s", locales.FailTaskRemote())
		return
	}
	a.closeQuick()
	a.closeIconThemePicker(false)
	entries, problems := readTaskCatalog(a.globalTasksPath(), a.projectTasksPath())
	p := taskPicker{open: true, stale: true, root: a.root, entries: entries, errors: problems}
	for _, entry := range entries {
		p.names = append(p.names, entry.task.Label)
	}
	a.taskPick = p
}

func (a *app) closeTaskPicker() {
	a.taskPick.open = false
	if a.focus == paneTerminal {
		a.focusTerminal()
	} else {
		a.focusColumn()
	}
}

func (p *taskPicker) refresh() {
	if !p.stale && p.query == p.shown {
		return
	}
	p.results = workspace.Fuzzy(p.names, strings.TrimSpace(p.query), 200)
	if strings.TrimSpace(p.query) == "" {
		p.results = p.results[:0]
		for i := range p.entries {
			p.results = append(p.results, workspace.Match{Index: i})
		}
	}
	p.sel, p.shown, p.stale, p.moved = 0, p.query, false, true
}

func (a *app) taskContext() tasks.Context {
	ctx := tasks.Context{Root: a.workDir(), File: a.activeFile()}
	if d := a.activeEditor(); d != nil {
		pos := caretPosition(d)
		ctx.Row, ctx.Column = pos.Line+1, pos.Character+1
	}
	return ctx
}

// Refocusing an existing process does not need the file context that was
// available when it started; its editor terminal may now be the active tab.
func (a *app) focusRunningTask(root string, task tasks.Task) *session {
	if task.AllowConcurrentRuns {
		return nil
	}
	for i := len(a.terms) - 1; i >= 0; i-- {
		s := a.terms[i]
		if s.task == nil || s.task.root != root || s.task.resolved.Task.Label != task.Label {
			continue
		}
		run := s.task.resolved
		if run.Task.IsDefaultYazi() == task.IsDefaultYazi() && run.YaziFile() == "" && taskRunning(s) {
			a.showTerminal(s)
			return s
		}
	}
	return nil
}

func (a *app) runTaskLabel(label string) {
	a.runTaskWithContext(label, a.taskContext())
}

func (a *app) runTaskWithContext(label string, context tasks.Context) {
	if remote.Is(a.root) {
		a.fail("%s", locales.FailTaskRemote())
		return
	}
	entries, problems := readTaskCatalog(a.globalTasksPath(), a.projectTasksPath())
	for _, problem := range problems {
		a.tell(note{message: locales.TaskConfigError(problem.path, problem.err.Error()), label: locales.TaskEditConfig(), action: func() {
			a.openFile(problem.path, true)
		}})
	}
	for _, entry := range entries {
		if entry.task.Label != label {
			continue
		}
		if !entry.task.IsDefaultYazi() {
			if running := a.focusRunningTask(a.root, entry.task); running != nil {
				a.lastTask = running.task.invocation()
				return
			}
		}
		context.Environ = os.Environ()
		resolved, err := tasks.Resolve(entry.task, context)
		if err != nil {
			a.fail("%s", locales.FailStartTask(label, err.Error()))
			return
		}
		if session := a.launchTask(a.root, resolved); session != nil {
			a.lastTask = session.task.invocation()
		}
		return
	}
	a.fail("%s", locales.TaskNotFound(label))
}

func (a *app) rerunTask() {
	if a.lastTask.label == "" || a.lastTask.root != a.root {
		a.openTaskPicker()
		return
	}
	a.runTaskWithContext(a.lastTask.label, a.lastTask.context)
}

// Configuration is created only on an explicit edit action. Opening a project
// or listing its tasks never runs a command or writes a configuration file.
const taskTemplate = `[
  {
    "label": "lazygit",
    "command": "lazygit"
  },
  {
    "label": "yazi",
    "command": "yazi"
  }
]
`

func (a *app) openTasksConfig(project bool) {
	path := a.globalTasksPath()
	if project {
		path = a.projectTasksPath()
		if path == "" {
			a.tell(note{message: locales.PrefsNoFolder()})
			return
		}
	}
	if path == "" {
		a.fail("%s", locales.TaskNoConfigDir())
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		a.fail("%s", locales.FailMake(path, err.Error()))
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		_, err = f.WriteString(taskTemplate)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	} else if errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		a.fail("%s", locales.FailMake(path, err.Error()))
		return
	}
	a.closeTaskPicker()
	a.openFile(path, true)
}

func (a *app) taskPickerView(c *ui.Context) {
	tp := &a.taskPick
	if !tp.open {
		return
	}
	if tp.root != a.root {
		a.closeTaskPicker()
		return
	}
	tp.refresh()
	p := a.pal
	prompt := pickerPrompt{label: locales.TaskPickerTitle(), prompt: locales.TaskPlaceholder(), value: &tp.query, close: a.closeTaskPicker}
	choose := func() {
		if tp.sel >= 0 && tp.sel < len(tp.results) {
			a.chooseTask(tp.entries[tp.results[tp.sel].Index].task.Label)
		}
	}
	a.pickerDialog(c.Key("task-picker"), prompt, func(in ui.Element) {
		tp.refresh()
		pickerNavigate(in, &tp.sel, &tp.moved, len(tp.results))
		if in.Submitted() {
			choose()
		}
	}, func() {
		for _, problem := range tp.errors {
			ui.Column(c).Padding(6, 14).Gap(6).Children(func() {
				ui.Text(c, locales.TaskConfigError(problem.path, problem.err.Error())).FontSize(12).TextColor(p.dangerFg)
				if actionButton(c, locales.TaskEditConfig(), iconPencil).AlignSelf(ui.Start).Clicked() {
					a.post(func() { a.closeTaskPicker(); a.openFile(problem.path, true) })
				}
			})
		}
		if len(tp.results) == 0 {
			quickMessage(c, iconTerminal, locales.TaskNoMatches())
		} else {
			a.pickerScroll(c.Key("task-results")).Children(func() {
				for i, match := range tp.results {
					entry := tp.entries[match.Index]
					row := ui.ButtonBase(c.Key("task:"+entry.task.Label)).Padding(9, 10).Gap(10).Radius(p.metricsOr().radius).
						AlignItems(ui.Center).Justify(ui.Start).Label(entry.task.Label)
					a.pickerRow(c, row, i == tp.sel, tp.moved)
					if row.Clicked() {
						a.chooseTask(entry.task.Label)
					}
					row.Children(func() {
						ui.Icon(c, iconTerminal).FontSize(16).TextColor(p.muted).Shrink(0)
						ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
							ui.RichText(c, a.matchSpans(entry.task.Label, 0, match.Positions, p.text)...).FontSize(13.5).SingleLine()
							ui.Text(c, entry.detail()).FontSize(12).TextColor(p.muted).SingleLine()
						})
						ui.Text(c, entry.sourceLabel(a.projectTasksPath())).FontSize(11.5).TextColor(p.faint).Shrink(0)
					})
				}
			})
		}
		tp.moved = false
		ui.Row(c).Padding(8, 12).Gap(8).Children(func() {
			if actionButton(c, locales.TaskOpenProject(), nil).Disabled(a.projectTasksPath() == "").Clicked() {
				a.post(func() { a.openTasksConfig(true) })
			}
			if actionButton(c, locales.TaskOpenGlobal(), nil).Clicked() {
				a.post(func() { a.openTasksConfig(false) })
			}
		})
		a.pickerActions(c, pickerBar{action: locales.TaskRun(), enabled: len(tp.results) > 0, choose: choose, close: prompt.close})
	})
}

func (a *app) chooseTask(label string) {
	root := a.taskPick.root
	a.post(func() {
		if !a.taskPick.open || a.root != root {
			return
		}
		a.closeTaskPicker()
		a.runTaskLabel(label)
	})
}

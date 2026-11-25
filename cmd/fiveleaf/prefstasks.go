package main

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/keymap"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type prefsTaskCatalog struct {
	global, project string
	entries         []taskEntry
	problems        []taskConfigError
	bindings        *keymap.Map
	shortcuts       map[string][]keymap.Stroke
}

func bindingTaskName(command string, args json.RawMessage) string {
	switch command {
	case "task.lazygit", "task.yazi":
		if len(args) == 0 {
			return strings.TrimPrefix(command, "task.")
		}
	case "task.spawn":
		var fields map[string]string
		if json.Unmarshal(args, &fields) == nil && len(fields) == 1 {
			return fields["task_name"]
		}
	}
	return ""
}

func namedTaskKey(name string, keys []keymap.Stroke) keymap.Entry {
	args, _ := json.Marshal(map[string]string{"task_name": name})
	return keymap.Entry{Key: keymap.KeysString(keys), Command: "task.spawn", Args: args}
}

func (w *prefsWindow) taskShortcutKeys() map[string][]keymap.Stroke {
	catalog := w.taskCatalog
	if catalog != nil && catalog.bindings == w.m.keys && catalog.shortcuts != nil {
		return catalog.shortcuts
	}
	shortcuts := make(map[string][]keymap.Stroke)
	for binding := range w.m.keys.Unconditional() {
		name := bindingTaskName(binding.Command, binding.Args)
		if name != "" && shortcuts[name] == nil {
			shortcuts[name] = binding.Keys
		}
	}
	if catalog != nil {
		catalog.bindings, catalog.shortcuts = w.m.keys, shortcuts
	}
	return shortcuts
}

func taskSettingsMatch(query string) bool {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return false
	}
	text := strings.ToLower(strings.Join([]string{"tasks.json task.spawn", prefSection(settings.CatTerminal),
		prefSection(settings.CatTasks), locales.PrefsTaskDefinitions(), locales.TaskOpenGlobal(), locales.TaskOpenProject(), locales.PrefsSetTaskShortcut()}, " "))
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

func (w *prefsWindow) taskSettingsPane(c *ui.Context, scope *prefFolder) {
	var target *desktop
	if scope != nil {
		target = scope.h
	} else if len(w.m.wins) > 0 {
		target = w.m.wins[0]
	}
	global, project := globalTaskFile(w.m.s.Path()), ""
	if target != nil {
		project = target.a.projectTasksPath()
	}
	catalog := w.taskCatalog
	if catalog == nil || catalog.global != global || catalog.project != project {
		entries, problems := readTaskCatalog(global, project)
		catalog = &prefsTaskCatalog{global: global, project: project, entries: entries, problems: problems}
		w.taskCatalog = catalog
	}
	shortcuts := w.taskShortcutKeys()
	p := w.pal
	ui.Scroll(c.Key("prefs-rows-tasks")).Grow(1).MinHeight(0).Children(func() {
		fadePage(ui.Column(c.Key("prefs-page-tasks")).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(prefsPageWidth).
			Padding(20, 28, 32, 28).Gap(8)).Children(func() {
			w.bindFirst = true
			defer func() { w.bindFirst = false }()
			w.sectionLabel(c, "files-heading", locales.PrefsTaskDefinitions(), 0)
			w.card(c, "files", func() {
				w.taskConfigRow(c, target, global, false)
				ui.Row(c).Height(1).Margin(0, 14).Background(p.border)
				w.taskConfigRow(c, target, project, true)
			})
			for _, problem := range catalog.problems {
				w.note(c, problem.path, iconTriangleAlert, p.dangerFg, locales.TaskConfigError(shortenHome(problem.path), problem.err.Error()), nil)
			}
			ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Margin(8, 0, 0, 0).Children(func() {
				w.sectionLabel(c, "list-heading", locales.PrefsTasksAvailable(), len(catalog.entries))
				if w.first(actionButton(c.Key("refresh"), locales.Refresh(), iconRefresh)).Clicked() {
					w.taskCatalog = nil
					w.invalidate()
				}
			})
			ui.Text(c, locales.PrefsTaskPrecedence()).FontSize(12).TextColor(p.muted).Margin(0, 4)
			const rowHeight = 48
			w.card(c, "task-catalog", func() {
				ui.List(c.Key(project), nil, len(catalog.entries), func(index int) {
					entry := catalog.entries[index]
					row := ui.Row(c).Height(rowHeight).Padding(0, 14).AlignItems(ui.Center).Gap(12)
					if index > 0 {
						row.BorderWidth(1, 0, 0, 0).BorderColor(p.border)
					}
					row.Children(func() {
						ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
							ui.Text(c, entry.task.Label).FontSize(13).FontWeight(600).TextColor(p.text).SingleLine().Tooltip(entry.task.Label)
							detail := entry.detail()
							ui.Text(c, detail).FontSize(12).TextColor(p.muted).SingleLine().Tooltip(detail)
						})
						w.pill(c, "source", entry.sourceLabel(project), p.muted, false)
						w.taskShortcutButton(c, entry.task.Label, shortcuts[entry.task.Label])
					})
				}).Height(float32(min(len(catalog.entries), 8) * rowHeight)).Label(locales.PrefsTasksAvailable())
			})
		})
	})
}

func (w *prefsWindow) taskShortcutButton(c *ui.Context, name string, keys []keymap.Stroke) {
	label := locales.PrefsSetTaskShortcut()
	if len(keys) > 0 {
		label = strings.Join(chordCaps(keys), " ")
	}
	title := locales.KeysRecordTitle(name)
	button := actionButton(c.Key("shortcut:"+name), label, iconKeyboard).Label(title).Tooltip(title).
		MaxWidthPercent(45).Disabled(w.m.s.Path() == "")
	if button.Clicked() {
		w.keys.record(command{id: "task.spawn", label: name})
		w.keys.recording.task = name
	}
	if recording := w.keys.recording; recording != nil && recording.task == name {
		button.Bind(&recording.origin)
	}
}

func (w *prefsWindow) taskConfigRow(c *ui.Context, target *desktop, path string, project bool) {
	p := w.pal
	label, hint, icon := locales.TaskOpenGlobal(), locales.TaskNoConfigDir(), iconFileJSON
	if project {
		label, hint, icon = locales.TaskOpenProject(), locales.PrefsNoFolder(), iconFolder
	}
	if path != "" {
		hint = shortenHome(path)
		if project {
			hint = filepath.Join(filepath.Base(target.a.root), ".fiveleaf", "tasks.json")
		}
	}
	enabled := target != nil && path != ""
	row := ui.ButtonBase(c.Key(label)).AlignItems(ui.Center).Justify(ui.Start).Padding(9, 14).Gap(12).
		Label(label).Disabled(!enabled).Transition(hoverFade)
	if enabled {
		w.first(row).Tooltip(shortenHome(path)).Cursor(ui.CursorPointer)
		if row.Hovered() {
			row.Background(p.hover)
		}
	}
	row.Children(func() {
		ui.Icon(c, icon).FontSize(16).TextColor(p.muted).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13).TextColor(p.text).SingleLine()
			line := ui.Text(c, hint).FontSize(12).TextColor(p.muted)
			if path != "" {
				line.SingleLine()
			}
		})
		ui.Icon(c, iconOpenFile).FontSize(14).TextColor(p.faint).Shrink(0)
	})
	if enabled && row.Clicked() {
		open := func(a *app) {
			if !a.loadsClosed && ((!project && a.globalTasksPath() == path) || (project && a.projectTasksPath() == path)) {
				a.openTasksConfig(project)
			}
		}
		if target.a.window.Detached {
			target.inWorkspace(open)
		} else {
			target.do(func() { open(target.a) })
			target.show()
		}
	}
}

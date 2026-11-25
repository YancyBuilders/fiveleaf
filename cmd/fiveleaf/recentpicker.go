package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

// recentPicker is the box of recent folders, found by path. projectMode
// includes this window's project and the actions for managing the list.
type recentPicker struct {
	open        bool
	projectMode bool
	query       string
	sel         int
	moved       bool
	// dirs are the folders as the box opened, names their paths shown,
	// and results those the query matches.
	dirs    []string
	names   []string
	results []workspace.Match
	shown   string // the query the results are of
	stale   bool
}

// openRecentPicker shows the box of the recent folders.
func (a *app) openRecentPicker() {
	r := &a.recents
	*r = recentPicker{open: true, stale: true}
	a.reloadRecentProjects()
}

// openProjectPicker shows the projects and their opening actions in the
// same command box as the recent folders.
func (a *app) openProjectPicker() {
	a.recents = recentPicker{open: true, projectMode: true, stale: true}
	a.reloadRecentProjects()
}

func (a *app) reloadRecentProjects() {
	r := &a.recents
	except := a.root
	if r.projectMode {
		except = ""
	}
	r.dirs = a.settings.RecentExcept(except)
	r.names = make([]string, len(r.dirs))
	for i, dir := range r.dirs {
		r.names[i] = shortenHome(dir)
	}
	r.stale = true
}

func (a *app) closeRecentPicker() {
	a.recents.open = false
	if a.focus != paneTerminal {
		a.focusColumn()
	}
}

// chooseRecent opens the folder of a row in the window, or with newWin in
// a window of its own.
func (a *app) chooseRecent(i int, newWin bool) {
	r := &a.recents
	if i < 0 || i >= len(r.results) {
		return
	}
	dir := r.dirs[r.results[i].Index]
	if r.projectMode {
		a.chooseProject(dir, newWin)
		return
	}
	a.closeRecentPicker()
	if newWin {
		a.host.newWindow(dir)
	} else {
		a.switchFolder(dir)
	}
}

func (a *app) chooseProject(dir string, newWin bool) {
	a.closeRecentPicker()
	if newWin {
		a.host.openProjectWindow(dir)
	} else if dir != a.root && !a.host.openProjectHere(dir) {
		a.switchTo(dir)
	}
}

func (a *app) removeRecentProject(dir string) {
	a.settings.RemoveRecent(dir)
	a.host.recentChanged()
	a.reloadRecentProjects()
}

func (a *app) revealRecentProject(dir string) {
	if !remote.Is(dir) {
		a.host.reveal(dir)
	}
}

// recentPickerView shows the box of the recent folders while it is open.
func (a *app) recentPickerView(c *ui.Context) {
	r := &a.recents
	if !r.open {
		return
	}
	label, placeholder := locales.RecentLabel(), locales.RecentPlaceholder()
	if r.projectMode {
		label, placeholder = locales.ProjectPickerTitle(), locales.ProjectPickerPlaceholder()
	}
	if r.stale || r.shown != r.query {
		r.results = workspace.Fuzzy(r.names, strings.TrimSpace(r.query), 60)
		if strings.TrimSpace(r.query) == "" {
			// Every folder, the one opened last first.
			r.results = r.results[:0]
			for i := range r.names {
				r.results = append(r.results, workspace.Match{Index: i})
			}
		}
		r.shown, r.stale, r.sel = r.query, false, 0
	}
	prompt := pickerPrompt{label: label, prompt: placeholder, value: &r.query, close: a.closeRecentPicker}
	choose := func(newWindow bool) {
		sel := r.sel
		a.post(func() { a.chooseRecent(sel, newWindow) })
	}
	a.pickerDialog(c.Key("recent-picker"), prompt, func(in ui.Element) {
		pickerNavigate(in, &r.sel, &r.moved, len(r.results))
		if in.Shortcut(ui.Cmd, ui.KeyEnter) {
			choose(true)
		} else if in.Submitted() {
			choose(false)
		}
	}, func() {
		if r.projectMode {
			a.pickerSection(c, locales.ProjectPickerRecent())
		}
		if len(r.results) > 0 {
			a.pickerScroll(c.Key("recent-results")).Children(func() {
				for i := range r.results {
					if r.projectMode {
						a.projectRecentRow(c, i)
					} else {
						a.recentRow(c, i)
					}
				}
			})
		} else {
			msg := locales.RecentNoMatches()
			if len(r.dirs) == 0 {
				msg = locales.RecentNone()
			}
			quickMessage(c, iconFolderOpen, msg)
		}
		r.moved = false
		if r.projectMode {
			a.projectPickerActions(c)
		}
		a.pickerActions(c, pickerBar{action: locales.QuickOpenAction(), tip: locales.RecentNewWindowTip(keyText("CmdOrCtrl+Enter")), enabled: len(r.results) > 0, choose: func() { choose(false) }, close: prompt.close})
	})
}

// projectRecentRow keeps room for every action so hovering a row cannot
// move its name or another row's controls.
func (a *app) projectRecentRow(c *ui.Context, i int) {
	r, p := &a.recents, a.pal
	m := r.results[i]
	dir, name := r.dirs[m.Index], r.names[m.Index]
	selected := i == r.sel
	row := ui.Row(c.Key("project:"+dir)).Padding(4, 6).Gap(4).AlignItems(ui.Center)
	hovered := row.Hovered()
	a.pickerRow(c, row, selected, r.moved)
	row.Children(func() {
		open := ui.ButtonBase(c.Key("project-open:"+dir)).Padding(3, 4).Gap(10).Grow(1).MinWidth(0).
			Justify(ui.Start).AlignItems(ui.Center).Label(locales.RecentOpenLabel(dir)).Tooltip(name)
		open.Children(func() {
			icon, tint := a.folderIcon(dir, false)
			iconThemeView(c, p, icon, tint, 16)
			base := strings.LastIndexByte(name, '/') + 1
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.RichText(c, a.matchSpans(name[base:], base, m.Positions, p.text)...).FontSize(13).SingleLine()
				ui.RichText(c, a.matchSpans(name, 0, m.Positions, p.muted)...).FontSize(11.5).SingleLine()
			})
		})
		if open.Clicked() {
			newWin := open.ClickModifiers()&ui.Cmd != 0
			a.post(func() { a.chooseProject(dir, newWin) })
		}
		visible := selected || hovered || row.FocusWithin()
		button := func(icon *ui.SVG, label, tip string, disabled bool, run func()) {
			btn := ui.ButtonBase(c.Key(label)).Size(27, 28).Shrink(0).Radius(4).Justify(ui.Center).AlignItems(ui.Center).
				Label(label + " " + dir).Tooltip(tip).Disabled(disabled || !visible)
			if !visible {
				btn.Opacity(0)
			}
			col := p.muted
			if disabled {
				col = p.faint
			} else if btn.Hovered() {
				btn.Background(p.hover)
				col = p.text
			}
			btn.Transition(hoverFade)
			btn.Children(func() { ui.Icon(c, icon).FontSize(14).TextColor(col) })
			if btn.Clicked() {
				a.post(run)
			}
		}
		button(iconCheckout, locales.ProjectOpenHere(), locales.ProjectOpenHere(), false, func() { a.chooseProject(dir, false) })
		button(iconExternal, locales.ProjectOpenWindow(), locales.ProjectOpenWindow(), false, func() { a.chooseProject(dir, true) })
		revealTip := revealLabel()
		if remote.Is(dir) {
			revealTip = locales.ProjectRevealUnavailable()
		}
		button(iconFolderOpen, revealLabel(), revealTip, remote.Is(dir), func() { a.revealRecentProject(dir) })
		button(iconX, locales.ProjectRemoveRecent(), locales.ProjectRemoveRecent(), false, func() { a.removeRecentProject(dir) })
	})
}

func (a *app) projectPickerActions(c *ui.Context) {
	p := a.pal
	ui.Column(c).Padding(6).BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
		for _, entry := range []struct {
			label string
			icon  *ui.SVG
			run   func()
		}{
			{locales.ProjectOpenLocal(), iconFolderPlus, func() {
				a.closeRecentPicker()
				a.host.openFolder(func(dir string) {
					if dir != "" {
						a.chooseProject(dir, false)
					}
				})
			}},
			{locales.ProjectOpenRemote(), iconCloud, func() {
				a.closeRecentPicker()
				a.openRemotePicker()
			}},
		} {
			row := ui.ButtonBase(c.Key(entry.label)).Padding(7, 10).Gap(10).Radius(4).Justify(ui.Start).
				AlignItems(ui.Center).Label(entry.label)
			if row.Hovered() {
				row.Background(p.hover)
			}
			row.Children(func() {
				ui.Icon(c, entry.icon).FontSize(14).TextColor(p.muted)
				ui.Text(c, entry.label).FontSize(12).TextColor(p.text)
			})
			if row.Clicked() {
				a.post(entry.run)
			}
		}
	})
}

// recentRow is a row of the box of the recent folders: its name and the
// folder it is in, with the characters the query matched in the accent
// color.
func (a *app) recentRow(c *ui.Context, i int) {
	r := &a.recents
	p := a.pal
	m := r.results[i]
	name := r.names[m.Index]
	row := ui.ButtonBase(c.Key("recent:"+name)).Padding(5, 10).Gap(10).Radius(6).Justify(ui.Start).AlignItems(ui.Center).
		Label(locales.RecentOpenLabel(r.dirs[m.Index]))
	a.pickerRow(c, row, i == r.sel, r.moved)
	if row.Clicked() {
		newWin := row.ClickModifiers()&ui.Cmd != 0
		a.post(func() { a.chooseRecent(i, newWin) })
	}
	row.Children(func() {
		icon, tint := a.folderIcon(r.dirs[m.Index], false)
		iconThemeView(c, p, icon, tint, 15)
		base := strings.LastIndexByte(name, '/') + 1
		ui.RichText(c, a.matchSpans(name[base:], base, m.Positions, p.text)...).FontSize(13).SingleLine().Shrink(0)
		if base > 1 {
			ui.RichText(c, a.matchSpans(name[:base-1], 0, m.Positions, p.faint)...).FontSize(12).SingleLine().Shrink(1)
		}
	})
}

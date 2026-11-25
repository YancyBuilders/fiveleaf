package main

import (
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

type leaderMenu struct {
	open         bool
	owner        *editor
	mode         string
	group        rune
	sel          int
	focus        bool
	windowPrefix bool
	// Keep the menu focused until the command key's text and release arrive.
	pending leaderEntry
	key     ui.Key
}

type leaderEntry struct {
	key     rune
	command string
	group   rune
	label   string
}

func (a *app) leaderKeyPath() []string {
	if a.leader.windowPrefix {
		return []string{"Ctrl-w"}
	}
	path := []string{"Space"}
	if a.leader.group != 0 {
		path = append(path, string(a.leader.group))
	}
	return path
}

func (a *app) leaderAtRoot() bool { return a.leader.group == 0 || a.leader.windowPrefix }

func leaderEntries(group rune, t locales.Messages) []leaderEntry {
	switch group {
	case 'f':
		return []leaderEntry{{key: 'f', command: "go.quickOpen"}, {key: 'n', command: "file.new"},
			{key: 's', command: "file.save"}, {key: 'a', command: "file.saveAll"},
			{key: 'r', command: "file.openRecent"}, {key: 'c', command: "file.copyRelativePath"}}
	case 's':
		return []leaderEntry{{key: 'f', command: "edit.find"}, {key: 'w', command: "edit.findInFiles"},
			{key: 's', command: "go.symbol"}, {key: 'l', command: "go.line"}}
	case 'c':
		return []leaderEntry{{key: 'd', command: "go.definition"}, {key: 'h', command: "go.hover"},
			{key: 'f', command: "sel.format"}, {key: 'p', command: "plugins.problems"},
			{key: 'l', command: "plugins.languageServers"}}
	case 'b':
		return []leaderEntry{{key: 'n', command: "go.nextTab"}, {key: 'p', command: "go.prevTab"},
			{key: 'd', command: "file.close"}, {key: 'r', command: "file.reopenClosed"}}
	case 'w':
		return []leaderEntry{{key: 'v', command: "view.splitRight"}, {key: 'h', command: "view.focusLeftColumn"},
			{key: 'l', command: "view.focusRightColumn"}, {key: 'w', command: "view.nextColumn"},
			{key: 'W', command: "view.prevColumn"}, {key: 'q', command: "view.closeColumn"},
			{key: 'o', command: "view.onlyColumn"}, {key: '=', command: "view.balanceColumns"}}
	case 'g':
		return []leaderEntry{{key: 's', command: "view.scm"}, {key: 'd', command: "git.openChanges"},
			{key: 'b', command: "git.switchBranch"}, {key: 'w', command: "git.switchWorktree"}}
	default:
		return []leaderEntry{{key: ' ', command: "go.quickOpen"}, {key: 'p', command: "go.commands"},
			{key: 'f', group: 'f', label: t.MenuFile()}, {key: 's', group: 's', label: t.LeaderSearch()},
			{key: 'c', group: 'c', label: t.LeaderCode()}, {key: 'b', group: 'b', label: t.LeaderBuffers()},
			{key: 'w', group: 'w', label: t.LeaderWindows()}, {key: 'g', group: 'g', label: t.MenuGit()}}
	}
}

func (a *app) leaderReady(d *editor) bool {
	if d == nil || d.preview != nil || d.jumpHints.active {
		return false
	}
	switch editorMode(a.prefs()) {
	case "vim":
		return d.vim.mode != vimInsert && len(d.vim.keys) == 0 && !d.vim.replay && a.vimRec == 0
	case "helix":
		return d.helix.mode != helixInsert && len(d.helix.keys) == 0
	}
	return false
}

func (a *app) openLeaderMenu() {
	d := a.activeEditor()
	if !a.leaderReady(d) || a.overlaid() {
		return
	}
	a.leader = leaderMenu{open: true, owner: d, mode: editorMode(a.prefs()), focus: true}
	a.keyedAt = a.frameAt
	a.invalidate()
}

func (a *app) closeLeaderMenu() {
	d := a.leader.owner
	a.dismissLeaderMenu()
	if d != nil && a.activeEditor() == d {
		d.vscroll.Focus()
	}
}

// Pointer dismissal keeps the focus on the control that was clicked.
func (a *app) dismissLeaderMenu() {
	a.leader = leaderMenu{}
	a.invalidate()
}

func (a *app) chooseLeader(e leaderEntry) {
	if !a.leaderEnabled(e) {
		return
	}
	if e.group != 0 {
		a.leader.group, a.leader.sel, a.leader.focus = e.group, 0, true
		a.invalidate()
		return
	}
	a.closeLeaderMenu()
	a.post(func() {
		a.keyedAt = a.frameAt
		a.run(e.command)
	})
}

func (a *app) leaderEnabled(e leaderEntry) bool {
	switch e.command {
	case "file.save", "sel.format":
		return a.leader.owner != nil && !a.leader.owner.locked()
	case "view.focusLeftColumn":
		return a.horizontalColumn(a.column(), -1, false) != nil
	case "view.focusRightColumn":
		return a.horizontalColumn(a.column(), 1, false) != nil
	case "view.nextColumn", "view.prevColumn":
		return len(a.cols) > 1
	case "view.closeColumn":
		return !a.column().locked && len(a.column().tabs) > 0
	case "view.onlyColumn":
		return a.canKeepOnlyColumn()
	case "view.balanceColumns":
		count := 0
		for _, col := range a.cols {
			if !col.locked {
				count++
			}
		}
		return count > 1
	}
	return true
}

func (a *app) leaderInput(ev ui.InputEvent) bool {
	if !a.leader.open {
		return false
	}
	switch ev.Kind {
	case ui.InputKeyUp:
		a.held = nil
		if m := &a.leader; m.pending.command != "" && ev.Key == m.key {
			entry := m.pending
			m.pending, m.key = leaderEntry{}, 0
			a.chooseLeader(entry)
		}
		return true
	case ui.InputText, ui.InputCompose:
		return true
	case ui.InputKeyDown:
	default:
		return false
	}
	if ev.Repeat && ev.Key != ui.KeyDown && ev.Key != ui.KeyUp {
		return true
	}
	m := &a.leader
	if m.pending.command != "" {
		if ev.Key == ui.KeyEscape && ev.Mods == 0 {
			a.closeLeaderMenu()
		}
		return true
	}
	entries := leaderEntries(m.group, locales.In(i18n.Locale()))
	choose := func(e leaderEntry) {
		if !a.leaderEnabled(e) {
			return
		}
		if e.group != 0 {
			a.chooseLeader(e)
		} else {
			m.pending, m.key = e, ev.Key
			for i, entry := range entries {
				if entry.key == e.key {
					m.sel, m.focus = i, true
					break
				}
			}
		}
	}
	move := func(delta int) {
		for range entries {
			m.sel = (m.sel + delta + len(entries)) % len(entries)
			if a.leaderEnabled(entries[m.sel]) {
				break
			}
		}
		m.focus = true
	}
	if ev.Mods == 0 {
		switch ev.Key {
		case ui.KeyEscape:
			a.closeLeaderMenu()
			return true
		case ui.KeyBackspace, ui.KeyLeft:
			if a.leaderAtRoot() {
				a.closeLeaderMenu()
			} else {
				m.group, m.sel, m.focus = 0, 0, true
			}
			return true
		case ui.KeyDown, ui.KeyUp:
			delta := 1
			if ev.Key == ui.KeyUp {
				delta = -1
			}
			move(delta)
			return true
		case ui.KeyEnter:
			choose(entries[m.sel])
			return true
		}
	}
	if ev.Key == ui.KeyTab && (ev.Mods == 0 || ev.Mods == ui.Shift) {
		delta := 1
		if ev.Mods == ui.Shift {
			delta = -1
		}
		move(delta)
		return true
	}
	if m.group == 'w' && ev.Mods&^ui.Shift == ui.Ctrl {
		ev.Mods &^= ui.Ctrl
	}
	if r, ok := vimRune(ev, false); ok {
		if m.group == 'w' && m.mode == "vim" && r == 'c' {
			r = 'q'
		}
		for _, e := range entries {
			if e.key == r {
				choose(e)
				break
			}
		}
	}
	// An unbound key belongs to the menu too, so it cannot edit the file.
	return true
}

func (a *app) leaderMenuView(c *ui.Context) {
	m := &a.leader
	if !m.open {
		return
	}
	if a.blurred || m.owner != a.activeEditor() || m.mode != editorMode(a.prefs()) || !a.leaderReady(m.owner) {
		a.closeLeaderMenu()
		return
	}
	t := locales.In(i18n.Locale())
	entries := leaderEntries(m.group, t)
	focus, selected := m.focus, m.sel
	m.focus = false
	labels := make(map[string]string)
	for _, command := range a.commandsIn(t) {
		labels[command.id] = command.label
	}
	title := t.LeaderTitle()
	if m.group != 0 {
		for _, e := range leaderEntries(0, t) {
			if e.group == m.group {
				title = e.label
				break
			}
		}
	}
	p, metrics := a.pal, a.pal.metricsOr()
	width, height := c.Size()
	const edge = float32(8)
	room := max(0, height-titleBarH-a.statusHeight()-2*edge)
	ui.Overlay(c.Key("leader-overlay"), func() {
		box := ui.Column(c.Key("leader-menu")).Absolute().Right(edge).Bottom(a.statusHeight()+edge).
			Width(min(320, max(0, width-2*edge))).MaxHeight(room).MinHeight(0).
			Background(p.sidebarSurface()).Border(1, p.border).Radius(metrics.radius).Clip().
			Role(ui.RoleMenu).Label(t.LeaderTitle()).Focusable().FocusRing(false).HandleInput(a.leaderInput).InputMethod(false)
		box.Children(func() {
			ui.Row(c).Height(metrics.control).Shrink(0).Padding(0, metrics.inset).Gap(metrics.gap).
				AlignItems(ui.Center).Children(func() {
				for i, key := range a.leaderKeyPath() {
					if i > 0 {
						ui.Icon(c, iconChevronRight).FontSize(metrics.textMicro).TextColor(p.faint)
					}
					ui.Text(c, key).FontSize(metrics.textSmall).TextColor(p.text).SingleLine().Shrink(0)
				}
				ui.Spacer(c)
				ui.Text(c, title).FontSize(metrics.textSmall).TextColor(p.muted).SingleLine()
			})
			ui.Scroll(c.Key("leader-entries")).MaxHeight(max(0, room-2*metrics.control-2)).MinHeight(0).
				Padding(0, metrics.gap).Children(func() {
				for i, e := range entries {
					label := e.label
					if e.command != "" {
						label = labels[e.command]
					}
					key := string(e.key)
					if e.key == ' ' {
						key = "Space"
					}
					row := ui.ButtonBase(c.Key("leader:"+string(m.group)+":"+key)).Height(metrics.control).Shrink(0).
						Padding(0, metrics.inset-metrics.gap).Gap(metrics.gap * 2).Radius(max(0, metrics.radius-metrics.gap)).
						Justify(ui.Start).AlignItems(ui.Center).Role(ui.RoleMenuItem).Label(label).Description(key).
						Disabled(!a.leaderEnabled(e)).HandleInput(a.leaderInput).InputMethod(false)
					if row.Hovered() || row.Focused() {
						row.Background(p.hover)
					}
					if focus && i == selected {
						row.Focus().ScrollIntoView()
					}
					if !focus && row.Focused() {
						m.sel = i
					}
					row.Children(func() {
						color := p.text
						if !a.leaderEnabled(e) {
							color = p.faint
						}
						ui.Text(c, key).Width(42).Shrink(0).FontSize(metrics.textSmall).TextColor(p.muted).SingleLine()
						ui.Text(c, label).FontSize(metrics.textBody).TextColor(color).SingleLine().Grow(1).MinWidth(0)
						if e.group != 0 {
							ui.Icon(c, iconChevronRight).FontSize(metrics.textSmall).TextColor(p.muted)
						}
					})
					if row.Clicked() {
						a.chooseLeader(e)
					}
				}
			})
			ui.Row(c).Height(metrics.control).Shrink(0).Padding(0, metrics.inset).Gap(metrics.gap).
				AlignItems(ui.Center).Children(func() {
				if !a.leaderAtRoot() {
					ui.Text(c, "← "+t.LeaderBack()).FontSize(metrics.textSmall).TextColor(p.muted).SingleLine()
				}
				ui.Spacer(c)
				ui.Text(c, "esc "+t.QuickClose()).FontSize(metrics.textSmall).TextColor(p.muted).SingleLine()
			})
		})
		if box.PressedOutside() {
			a.dismissLeaderMenu()
		}
	})
}

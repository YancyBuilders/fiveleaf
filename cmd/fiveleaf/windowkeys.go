package main

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/keymap"
)

func (a *app) windowPrefixKey(d *editor, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || ev.Key != ui.KeyW || ev.Mods != ui.Ctrl ||
		len(a.chord) > 0 || d == nil || d.preview != nil || a.overlaid() {
		return false
	}
	switch editorMode(a.prefs()) {
	case "vim":
		if d.vim.mode == vimInsert {
			return false
		}
	case "helix":
		if d.helix.mode == helixInsert {
			return false
		}
	default:
		return false
	}
	// Personal bindings, including chords, win over the modal prefix.
	// Inherited Ctrl-W bindings must not close the tab before it opens.
	stroke := keymap.Stroke{Mods: ev.Mods, Key: ev.Key}
	if binding, more := a.userKeys.Lookup([]keymap.Stroke{stroke}, a.whenContext(placeEditor)); binding != nil || more {
		return false
	}
	// A pending motion or recording keeps its state; Ctrl-W must not fall
	// through to the platform's Close Tab shortcut meanwhile.
	if !ev.Repeat && a.leaderReady(d) {
		a.openLeaderMenu()
		a.leader.group, a.leader.windowPrefix = 'w', true
	}
	return true
}

func (a *app) focusAdjacentColumn(step int) {
	if target := a.horizontalColumn(a.column(), step, false); target != nil {
		a.focusColumnBy(slices.Index(a.cols, target) - a.col)
	}
}

func (a *app) closeCurrentColumn() {
	col := a.column()
	if col.locked {
		return
	}
	a.closeTabs(slices.Clone(col.tabs), func() {
		a.removeColumn(col)
		a.focusColumn()
		a.rememberTabs()
	}, nil)
}

func (a *app) canKeepOnlyColumn() bool {
	return len(a.cols) > 1 && !slices.ContainsFunc(a.cols, func(c *column) bool { return c.locked })
}

func (a *app) keepOnlyColumn() {
	if !a.canKeepOnlyColumn() {
		return
	}
	keep := a.column()
	var tabs []*colTab
	for _, col := range a.cols {
		if col != keep {
			tabs = append(tabs, col.tabs...)
		}
	}
	focus := func() {
		if i := slices.Index(a.cols, keep); i >= 0 {
			a.col = i
			a.focusColumn()
			a.rememberTabs()
		}
	}
	a.closeTabs(tabs, func() {
		for _, col := range slices.Clone(a.cols) {
			if col != keep && len(col.tabs) == 0 {
				a.removeColumn(col)
			}
		}
		focus()
	}, focus)
}

func (a *app) balanceColumns() {
	a.panes.balance()
	a.rememberTabs()
	a.invalidate()
}

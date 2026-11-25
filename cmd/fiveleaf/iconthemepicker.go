package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

type iconThemePicker struct {
	open, stale, moved bool
	query, shown       string
	preview            string
	sel                int
	themes             []iconThemeChoice
	results            []workspace.Match
}

func (a *app) openIconThemePicker() {
	if a.iconPicker.open {
		return
	}
	a.iconPicker = iconThemePicker{open: true, stale: true, preview: a.settings.IconTheme}
	a.refreshIconThemePicker()
}

func (a *app) closeIconThemePicker(keep bool) {
	tp := &a.iconPicker
	if !tp.open {
		return
	}
	if keep {
		choice, ok := tp.choice()
		if !ok {
			return
		}
		// Plugin changes can arrive between a key press and this posted action.
		a.refreshIconThemePicker()
		current, ok := tp.choice()
		if !ok || choice.id != current.id {
			return
		}
		if a.settings.IconTheme != choice.id {
			a.settings.IconTheme = choice.id
			a.settings.Changed()
			a.host.redrawWindows()
		}
	}
	tp.open = false
	a.invalidate()
	if a.focus != paneTerminal {
		a.focusColumn()
	}
}

func (tp *iconThemePicker) choice() (iconThemeChoice, bool) {
	if tp.sel < 0 || tp.sel >= len(tp.results) {
		return iconThemeChoice{}, false
	}
	index := tp.results[tp.sel].Index
	if index < 0 || index >= len(tp.themes) {
		return iconThemeChoice{}, false
	}
	return tp.themes[index], true
}

func (a *app) refreshIconThemePicker() {
	tp := &a.iconPicker
	themes := allIconThemes()
	if !tp.stale && tp.shown == tp.query && slices.Equal(tp.themes, themes) {
		return
	}
	names := make([]string, len(themes))
	for i, theme := range themes {
		names[i] = theme.name
	}
	query := strings.TrimSpace(tp.query)
	tp.results = workspace.Fuzzy(names, query, len(names))
	if query == "" {
		tp.results = tp.results[:0]
		for i := range names {
			tp.results = append(tp.results, workspace.Match{Index: i})
		}
	}
	tp.themes, tp.sel = themes, 0
	for i, match := range tp.results {
		if themes[match.Index].matches(tp.preview) {
			tp.sel = i
			break
		}
	}
	if choice, ok := tp.choice(); ok {
		tp.preview = choice.id
	}
	tp.shown, tp.stale, tp.moved = tp.query, false, true
	a.invalidate()
}

func (a *app) moveIconThemeChoice(to int) {
	tp := &a.iconPicker
	to = min(max(to, 0), len(tp.results)-1)
	if to < 0 || to == tp.sel {
		return
	}
	tp.sel, tp.moved = to, true
	if choice, ok := tp.choice(); ok {
		tp.preview = choice.id
		a.invalidate()
	}
}

func (a *app) keepIconThemeChoice(id string) {
	if !a.iconPicker.open {
		return
	}
	a.refreshIconThemePicker()
	tp := &a.iconPicker
	for i, match := range tp.results {
		if tp.themes[match.Index].id == id {
			tp.sel, tp.preview = i, id
			a.closeIconThemePicker(true)
			return
		}
	}
}

func (a *app) iconThemePickerView(c *ui.Context) {
	tp := &a.iconPicker
	if !tp.open {
		return
	}
	a.refreshIconThemePicker()
	prompt := pickerPrompt{label: locales.PrefTitleIconTheme(), prompt: locales.IconThemePlaceholder(), value: &tp.query, close: func() { a.closeIconThemePicker(false) }}
	choose := func() {
		if choice, ok := tp.choice(); ok {
			a.post(func() { a.keepIconThemeChoice(choice.id) })
		}
	}
	a.pickerDialog(c.Key("icon-theme-picker"), prompt, func(in ui.Element) {
		// The input can change its value while it is built in this frame.
		a.refreshIconThemePicker()
		switch {
		case in.Shortcut(0, ui.KeyDown):
			a.moveIconThemeChoice(tp.sel + 1)
		case in.Shortcut(0, ui.KeyUp):
			a.moveIconThemeChoice(tp.sel - 1)
		case in.Submitted():
			choose()
		}
	}, func() {
		if len(tp.results) == 0 {
			quickMessage(c, iconFolder, locales.IconThemeNoMatches())
		} else {
			a.pickerScroll(c.Key("icon-theme-results")).Children(func() {
				for i := range tp.results {
					a.iconThemePickerRow(c, i)
				}
			})
		}
		tp.moved = false
		a.pickerActions(c, pickerBar{action: locales.ThemeKeep(), enabled: len(tp.results) > 0, choose: choose, close: prompt.close})
	})
}

func (a *app) iconThemePickerRow(c *ui.Context, i int) {
	tp, p := &a.iconPicker, a.pal
	match := tp.results[i]
	choice := tp.themes[match.Index]
	row := ui.ButtonBase(c.Key("icon-theme:"+choice.id)).Padding(9, 10).Gap(12).Justify(ui.Start).AlignItems(ui.Center).Label(choice.name)
	a.pickerRow(c, row, i == tp.sel, tp.moved)
	if row.Clicked() {
		a.post(func() { a.keepIconThemeChoice(choice.id) })
	}
	row.Children(func() {
		ui.RichText(c, a.matchSpans(choice.name, 0, match.Positions, p.text)...).
			FontSize(13.5).FontWeight(550).SingleLine().Grow(1).Shrink(1)
		ui.Row(c).Gap(7).AlignItems(ui.Center).Shrink(0).Children(func() {
			icon, tint := folderIconForTheme(p, choice.id, "src", true)
			iconThemeView(c, p, icon, tint, 18)
			for _, name := range []string{"main.go", "app.tsx", "package.json", "README.md"} {
				icon, tint := fileIconForTheme(p, choice.id, name)
				iconThemeView(c, p, icon, tint, 18)
			}
		})
		ui.Box(c).Size(14, 14).Shrink(0).Children(func() {
			if i == tp.sel {
				mark, _ := a.quickListColors(c)
				ui.Icon(c, iconCheck).FontSize(14).TextColor(mark)
			}
		})
	})
}

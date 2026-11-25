package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

// themeChoice is a theme of the box of the color themes: its id in the
// settings, its name and what it looks like, in a few words.
type themeChoice struct {
	id    string
	name  func() string
	about func() string
}

// themeAppearances are the appearances the box chooses between, as the
// settings name them, in the order of its switch.
var themeAppearances = []string{"", "light", "dark"}

// themePicker is the box of the color themes. The window takes each theme
// as the choice moves to it, and each appearance as it is switched to:
// Return keeps them, and Escape goes back to those the box opened on.
type themePicker struct {
	open  bool
	query string
	sel   int
	moved bool
	// wasTheme and wasAppearance are the settings' as the box opened.
	wasTheme, wasAppearance string
	results                 []workspace.Match // the themes the query matches
	themes                  []themeChoice     // the themes the results index
	shown                   string            // the query the results are of
	built                   uint64            // themeFileGen when the results were built
	stale                   bool
}

// openThemePicker shows the box of the color themes, on the window's.
func (a *app) openThemePicker() {
	a.themes = themePicker{open: true, stale: true, wasTheme: a.settings.Theme, wasAppearance: a.settings.Appearance}
}

// closeThemePicker hides the box: keep keeps the theme and the appearance
// it shows, or else the window goes back to those it opened on.
func (a *app) closeThemePicker(keep bool) {
	tp := &a.themes
	if !tp.open {
		return
	}
	tp.open = false
	switch {
	case keep && (a.settings.Theme != tp.wasTheme || a.settings.Appearance != tp.wasAppearance):
		a.themeChanged()
	case !keep:
		a.previewTheme(tp.wasTheme, tp.wasAppearance)
	}
	if a.focus != paneTerminal {
		a.focusColumn()
	}
}

// previewTheme shows the windows in a theme and an appearance without
// saving them in the settings. This window goes to it at once, rather
// than fading, as the keys go through the themes.
func (a *app) previewTheme(theme, appearance string) {
	if a.settings.Theme == theme && a.settings.Appearance == appearance {
		return
	}
	a.fade.snap = true
	a.settings.Theme, a.settings.Appearance = theme, appearance
	a.host.setTheme(theme, appearance)
}

// previewChoice shows the windows in the theme of a row of the results.
func (a *app) previewChoice(i int) {
	tp := &a.themes
	if i < 0 || i >= len(tp.results) {
		return
	}
	index := tp.results[i].Index
	if index < 0 || index >= len(tp.themes) {
		return
	}
	a.previewTheme(tp.themes[index].id, a.settings.Appearance)
}

// switchAppearance moves the appearance by by along the box's switch.
func (a *app) switchAppearance(by int) {
	i := 0
	for j, ap := range themeAppearances {
		if ap == a.settings.Appearance {
			i = j
		}
	}
	i = min(max(i+by, 0), len(themeAppearances)-1)
	a.previewTheme(a.settings.Theme, themeAppearances[i])
}

// themePickerView shows the box of the color themes while it is open.
func (a *app) themePickerView(c *ui.Context) {
	tp := &a.themes
	if !tp.open {
		return
	}
	gen := themeFileGen()
	if tp.stale || tp.shown != tp.query || tp.built != gen {
		q := strings.TrimSpace(tp.query)
		themes := allThemes()
		names := make([]string, len(themes))
		for i, t := range themes {
			names[i] = t.name()
		}
		tp.results = workspace.Fuzzy(names, q, len(names))
		if q == "" {
			tp.results = tp.results[:0]
			for i := range names {
				tp.results = append(tp.results, workspace.Match{Index: i})
			}
		}
		// The row of the theme shown, or else the first, which the window
		// then shows.
		sel := -1
		for i, m := range tp.results {
			if themes[m.Index].id == themeSettingID(a.settings.Theme) {
				sel = i
			}
		}
		tp.themes = themes
		switch {
		case len(tp.results) == 0:
			tp.sel = 0
		case sel >= 0:
			tp.sel = sel
		default:
			tp.sel = min(max(tp.sel, 0), len(tp.results)-1)
			a.previewChoice(tp.sel)
		}
		// The chosen row comes into view, as the box opens or the query changes.
		tp.moved = true
		tp.shown, tp.built, tp.stale = tp.query, gen, false
	}
	prompt := pickerPrompt{label: locales.ThemeLabel(), prompt: locales.ThemePlaceholder(), value: &tp.query,
		trailing: func() { a.appearanceSwitch(c) }, close: func() { a.closeThemePicker(false) }}
	keep := func() { a.post(func() { a.closeThemePicker(true) }) }
	a.pickerDialog(c.Key("theme-picker"), prompt, func(in ui.Element) {
		// ← and → switch the appearance while there is no query
		// for the caret to move in.
		a.takeKeys(in, placeInput, func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown || ev.Mods != 0 || tp.query != "" {
				return false
			}
			switch ev.Key {
			case ui.KeyLeft:
				a.switchAppearance(-1)
			case ui.KeyRight:
				a.switchAppearance(1)
			default:
				return false
			}
			return true
		})
		move := func(to int) {
			to = min(max(to, 0), len(tp.results)-1)
			if to >= 0 && to != tp.sel {
				tp.sel, tp.moved = to, true
				a.previewChoice(to)
			}
		}
		switch {
		case in.Shortcut(0, ui.KeyDown):
			move(tp.sel + 1)
		case in.Shortcut(0, ui.KeyUp):
			move(tp.sel - 1)
		case in.Submitted():
			keep()
		}
	}, func() {
		if len(tp.results) > 0 {
			a.pickerScroll(c.Key("theme-results")).Children(func() {
				for i := range tp.results {
					a.themeRow(c, i)
				}
			})
		} else {
			quickMessage(c, iconSparkles, locales.ThemeNoMatches())
		}
		tp.moved = false
		a.pickerActions(c, pickerBar{action: locales.ThemeKeep(), enabled: true, choose: keep, close: prompt.close, hints: []pickerHint{{"←→", locales.ThemeAppearance()}}})
	})
}

// appearanceSwitch is the switch of the appearance in the box's head:
// the system's, light or dark. It greys out under the themes that are
// always dark.
func (a *app) appearanceSwitch(c *ui.Context) {
	sel := 0
	for i, ap := range themeAppearances {
		if ap == a.settings.Appearance {
			sel = i
		}
	}
	was := sel
	sw := ui.Segmented(c.Key("theme-appearance"), &sel,
		locales.CmdViewAppearanceSystem(), locales.CmdViewAppearanceLight(), locales.CmdViewAppearanceDark()).
		FontSize(12).Shrink(0).Label(locales.ThemeAppearance())
	if alwaysDark(a.settings.Theme) {
		sw.Disabled(true).Tooltip(locales.ThemeAlwaysDark())
	}
	if sel != was {
		a.previewTheme(a.settings.Theme, themeAppearances[sel])
	}
}

// themeRow is a row of the box of the color themes: a little window in
// the theme's colors, with its character, its name with the characters
// the query matched in the accent color, what it looks like, and a check
// on the theme currently previewed.
func (a *app) themeRow(c *ui.Context, i int) {
	tp := &a.themes
	p := a.pal
	m := tp.results[i]
	t := tp.themes[m.Index]
	row := ui.ButtonBase(c.Key("theme:"+t.id)).Padding(6, 10).Gap(12).Justify(ui.Start).AlignItems(ui.Center).Label(t.name())
	a.pickerRow(c, row, i == tp.sel, tp.moved)
	if row.Clicked() {
		a.post(func() {
			tp.sel = i
			a.previewChoice(i)
			a.closeThemePicker(true)
		})
	}
	row.Children(func() {
		a.themeSwatch(c, t.id, i == tp.sel)
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			ui.RichText(c, a.matchSpans(t.name(), 0, m.Positions, p.text)...).FontSize(13.5).FontWeight(550).SingleLine()
			ui.Text(c, t.about()).FontSize(12).TextColor(p.muted).SingleLine().Shrink(1)
		})
		ui.Box(c).Size(14, 14).Shrink(0).Children(func() {
			if i == tp.sel {
				mark, _ := a.quickListColors(c)
				ui.Icon(c.Key("theme-check"), iconCheck).FontSize(14).TextColor(mark)
			}
		})
	})
}

// Sizes of a theme's little window.
const swatchW, swatchH = 92, 56

// themeSwatch draws a little window in a theme's colors, in the
// appearance the window is in: its sidebar, a few lines of code, the
// caret, and the theme's character at the side. The chosen row's
// character blinks and bobs, as in the status bar.
func (a *app) themeSwatch(c *ui.Context, theme string, chosen bool) {
	dark := a.pal.dark
	switch a.settings.Appearance {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	q := paletteFor(theme, dark)
	m := paletteMascot(q)
	live := chosen && !a.mascotStill(c)
	ui.Box(c.Key("swatch")).Size(swatchW, swatchH).Shrink(0).Radius(7).Background(q.editor).Border(1, q.border).Clip().
		Transition(ui.ElementTransition{Colors: true, Duration: paletteFadeFor}).
		Draw(func(pt *ui.Painter, r ui.Rect) { paintSwatch(pt, r, q, m, 1, live) })
}

// paintSwatch paints a theme's little window in r, of the palette q and
// the character m, nil for none, at k times the size of the box's: live
// has the character blink and bob.
func paintSwatch(pt *ui.Painter, r ui.Rect, q *palette, m *mascot, k float32, live bool) {
	at := func(x, y, w, h float32) ui.Rect { return ui.Rect{X: r.X + x*k, Y: r.Y + y*k, W: w * k, H: h * k} }
	// The sidebar, floating at the side, and the tab strip.
	pt.Fill(ui.Rect{X: r.X + 3*k, Y: r.Y + 3*k, W: 16 * k, H: r.H - 6*k}, q.chrome, 4*k)
	pt.Fill(at(6, 7, 10, 2.5), q.muted.Alpha(0.6), 1.25*k)
	pt.Fill(at(6, 12, 8, 2.5), q.accent, 1.25*k)
	pt.Fill(at(6, 17, 9, 2.5), q.muted.Alpha(0.6), 1.25*k)
	// Code, a run of words to a line in the colors of the syntax.
	s := q.syntax
	lines := [][]struct {
		w   float32
		col ui.Color
	}{
		{{9, s[highlight.Keyword]}, {14, s[highlight.Function]}},
		{{6, s[highlight.Keyword]}, {15, s[highlight.String]}},
		{{8, s[highlight.Variable]}, {10, s[highlight.Type]}, {6, s[highlight.Number]}},
		{{18, s[highlight.Comment]}},
	}
	for li, words := range lines {
		x, y := float32(24), 10+float32(li)*9.5
		if li == 1 || li == 2 {
			x += 4
		}
		for _, wd := range words {
			pt.Fill(at(x, y, wd.w, 3), wd.col, 1.5*k)
			x += wd.w + 2.5
		}
		if li == 2 {
			// The caret, after the last word.
			pt.Fill(at(x, y-1.5, 1.2, 6), q.accent, 0)
		}
	}
	if m == nil {
		return
	}
	cell := 1.25 * k
	mw, mh := m.size(cell)
	var pose mascotPose
	if live {
		var hold time.Duration
		pose, hold = poseAt(pt.Now(), false)
		pt.After(hold)
	}
	m.draw(q, pt, ui.Rect{X: r.X + r.W - mw - 5*k, Y: r.Y + r.H - mh - 3*k, W: mw, H: mh}, cell, q.editor, pose)
}

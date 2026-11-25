package main

import (
	_ "embed"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/settings"
)

// palette is FiveLeaf's look: the colors of its chrome, of the code and of
// the terminal, for a light or a dark window, or for a theme of its own as
// The Matrix. Widgets take theirs from the ui.Theme that applyTheme
// derives from it.
type palette struct {
	dark bool
	// mono draws the whole window in the code's font, and the icons of
	// files in the palette's own colors rather than their languages'.
	mono          bool
	mascotName    string
	activityLabel bool
	accentDialogs bool
	welcomeRain   bool

	editor    ui.Color // the editor and the terminal
	chrome    ui.Color // the sidebar, the tab strip and the panel's header
	titlebar  ui.Color // the title bar, defaulting to chrome
	panel     ui.Color // the bottom panel
	border    ui.Color
	hover     ui.Color
	active    ui.Color // a chosen row
	text      ui.Color
	muted     ui.Color
	faint     ui.Color
	accent    ui.Color
	accentDim ui.Color
	// fill is the accent as a button's background, under fillText, and
	// fillHover and fillPressed the button under the pointer and pressed.
	fill, fillHover, fillPressed, fillText ui.Color
	// dangerFill is danger as a button's background, under dangerText,
	// for a button that loses something, as Discard; dangerHover and
	// dangerPressed are derived from it.
	dangerFill, dangerHover, dangerPressed, dangerText ui.Color
	// The faces of controls, as the theme's Surface, SurfaceHover and
	// SurfacePressed.
	surface, surfaceHover, surfacePressed ui.Color
	scrollbar                             ui.Color
	// inverse fills tooltips and toasts, under inverseText: zero for the
	// theme's own.
	inverse, inverseText ui.Color
	line                 ui.Color // the caret's line
	selection            ui.Color
	// The selection as the editor paints it, opaque over its background:
	// while it has the keyboard, and fainter while it has not.
	selected, selectedDim ui.Color
	gutter                ui.Color // line numbers
	gutterNow             ui.Color // the caret's line number
	status                ui.Color
	statusFg              ui.Color
	modified              ui.Color
	danger                ui.Color
	match                 ui.Color // a match of find
	matchNow              ui.Color // the current match
	matchRing             ui.Color
	guide                 ui.Color // indent guides
	guideNow              ui.Color // the guide of the caret's block
	bracket               ui.Color // the brackets paired at the caret
	word                  ui.Color // the other occurrences of the word at the caret
	cursor                ui.Color // the block cursor of vim's normal mode
	shadow                ui.Color
	added                 ui.Color // lines changed since Git's last commit
	changed               ui.Color
	removed               ui.Color
	// modifiedFg, addedFg, removedFg and dangerFg are those colors as
	// text: the same hues, darker on a light window or lighter on a dark
	// one, as far as they need to read at 4.5:1 on the chrome and the
	// editor. The marks and tints keep the brighter ones.
	modifiedFg, addedFg, removedFg, dangerFg ui.Color
	// rule, when set, is the theme's own hairline along the foot of the
	// title bar and the top of the status bar, in place of the border;
	// gilt, when set, is the light of its gold, which runs to the middle
	// of the line along the top of the tab shown, with a gem there.
	rule, gilt ui.Color
	// card, when set, is the radius of the card the editors and the panel
	// below them sit in, as Qoder's workbench has: the title bar, the
	// sidebar and the status bar are a shell around it in the chrome, with
	// no lines between them, grained as much as grain says, from 0 to 1.
	card, grain float32
	// strip is the tab strip's background: the editor, so the tabs and the
	// code read as one surface, but for a theme that names its own.
	strip ui.Color
	// sentence titles the sidebar and the panel in sentence case, as
	// Explorer, rather than in capitals.
	sentence bool
	// metrics are the gaps, radii and text of the chrome, which the
	// widgets the theme styles take too.
	metrics metrics

	syntax               [highlight.Kinds]ui.Color
	syntaxItalic         [highlight.Kinds]bool
	syntaxExplicit       [highlight.Kinds]bool // roles colored separately from their fallback
	syntaxItalicExplicit [highlight.Kinds]bool
	titlebarExplicit     bool
	// ansi is the terminal's 16 colors: black, red, green, yellow, blue,
	// magenta, cyan and white, then their bright kinds.
	ansi [16]ui.Color
}

// metrics are a palette's numbers: its grid of gaps and paddings, its
// radii and its text. They are a theme's own, so that a theme may be
// tighter or roomier, and the chrome and the theme's widgets share them.
type metrics struct {
	// gap, pad and inset are the unit of the space between two parts, the
	// room around the content of a control, and the room inside a pane;
	// each from 1 to 3 of them is what the chrome asks for.
	gap, pad, inset float32
	// radius of a control, a field, a card, and of a badge or a dot.
	radius, radiusField, radiusCard, radiusFull float32
	// text of the chrome: small, body and title, in DIPs, and the weight
	// of a title and of what is emphasized.
	textMicro, textSmall, textBody, textTitle float32
	weightMedium, weightBold                  int
	// row is the height of a row of a list, and control that of a button
	// or a field.
	row, control float32
}

// defaultMetrics is the grid every theme has but its own: 4 DIPs, at the
// text sizes the chrome is drawn at.
func defaultMetrics() metrics {
	return metrics{
		gap: 4, pad: 6, inset: 10,
		radius: 6, radiusField: 6, radiusCard: 10, radiusFull: 999,
		textMicro: 9.5, textSmall: 11.5, textBody: 12.5, textTitle: 12.5,
		weightMedium: 500, weightBold: 600,
		row: 22, control: 26,
	}
}

//go:embed themes/fiveleaf-light.json
var fiveLeafLight []byte

//go:embed themes/fiveleaf-dark.json
var fiveLeafDark []byte

func lightPalette() *palette { return readBuiltinPalette(fiveLeafLight) }

func darkPalette() *palette { return readBuiltinPalette(fiveLeafDark) }

func readBuiltinPalette(data []byte) *palette {
	file, err := parseThemeFile(data)
	if err != nil {
		panic(err)
	}
	return file.applyTo(palette{}, *file.Dark)
}

var (
	black, white = ui.Hex("#000000"), ui.Hex("#ffffff")
	lightPal     = lightPalette()
	darkPal      = darkPalette()
)

const (
	themeMatrix     = "matrix"
	themeOneDarkPro = "onedarkpro"
	themeAyu        = "ayu"
	themeClaude     = "claude"
	themeQoder      = "qoder"
	themeGrokBot    = "grokbot"
	themeCursor     = "cursor"
	themePurpleGold = "purplegold"
	themePumpkin    = "pumpkin"
	themeWitchsBrew = "witchsbrew"
	themeBloodMoon  = "bloodmoon"
)

func builtinDarkness(theme string, window bool) (bool, bool) {
	switch themeSettingID(theme) {
	case "":
		return window, true
	case "dark":
		return true, true
	case "light":
		return false, true
	}
	return false, false
}

// alwaysDark is whether a theme is dark whatever the Appearance is: its
// own file says so, or the theme it is based on does.
func alwaysDark(theme string) bool {
	return themeDarkness(theme, false, nil)
}

func builtinPalette(theme string, dark bool) *palette {
	useDark, ok := builtinDarkness(theme, dark)
	if !ok {
		return nil
	}
	if useDark {
		return darkPal
	}
	return lightPal
}

// paletteFor returns the palette of a theme of the settings, in a window
// that is dark or not.
func paletteFor(theme string, dark bool) *palette {
	if p := builtinPalette(theme, dark); p != nil {
		return p
	}
	if p := pluginPalette(theme, dark); p != nil {
		return p
	}
	return builtinPalette("", dark)
}

func prefsPalette(prefs *settings.Prefs, dark bool) *palette {
	switch prefs.Appearance {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	return paletteFor(prefs.Theme, dark)
}

// withMetrics returns p with a metrics of its own, for a palette made
// before themes had any: the grid a theme's chrome is drawn on.
func (p *palette) metricsOr() metrics {
	if p.metrics.gap == 0 {
		p.metrics = defaultMetrics()
	}
	return p.metrics
}

// keep returns p as a theme file that says it is not derived leaves it:
// the base palette's colors as they are, with every color that a palette
// needs to draw worked out from them, without changing what it wrote.
func (p *palette) keep() *palette {
	has := func(c ui.Color) bool { return c != (ui.Color{}) }
	set := func(dst *ui.Color, c ui.Color) {
		if !has(*dst) && has(c) {
			*dst = c
		}
	}
	set(&p.fillHover, p.fill.Mix(white, 0.1))
	set(&p.fillPressed, p.fill.Mix(black, 0.15))
	set(&p.dangerHover, p.dangerFill.Mix(white, 0.1))
	set(&p.dangerPressed, p.dangerFill.Mix(black, 0.15))
	set(&p.modifiedFg, p.modified)
	set(&p.addedFg, p.added)
	set(&p.removedFg, p.removed)
	set(&p.dangerFg, p.danger)
	set(&p.selected, p.selection.Over(p.editor))
	set(&p.selectedDim, p.selection.Alpha(0.5).Over(p.editor))
	set(&p.cursor, p.accent.Alpha(0.5))
	set(&p.syntax[highlight.Strong], p.syntax[highlight.Keyword])
	set(&p.syntax[highlight.Emph], p.syntax[highlight.Keyword])
	set(&p.syntax[highlight.Mark], p.text)
	p.deriveSyntaxRoles()
	set(&p.strip, p.editor)
	if !p.titlebarExplicit {
		p.titlebar = p.chrome
	}
	if p.metrics.gap == 0 {
		p.metrics = defaultMetrics()
	}
	return p
}

// themeSource is the light or dark appearance of the native window for a
// theme and an appearance of the settings: dark for the themes that are
// always dark, and as the appearance is for the others, the system's for
// "".
func themeSource(theme, appearance string) mygo.ThemeSource {
	if alwaysDark(theme) {
		return mygo.ThemeDark
	}
	return mygo.ThemeSource(appearance)
}

// derive fills in the colors made from the others.
func (p *palette) derive() *palette {
	if p.dark {
		p.dangerHover, p.dangerPressed = p.dangerFill.Mix(white, 0.1), p.dangerFill.Mix(black, 0.15)
	} else {
		p.dangerHover, p.dangerPressed = p.dangerFill.Mix(black, 0.1), p.dangerFill.Mix(black, 0.22)
	}
	p.modifiedFg = legible(p.modified, p.dark, p.chrome, p.editor, p.panel)
	p.addedFg = legible(p.added, p.dark, p.chrome, p.editor, p.panel)
	p.removedFg = legible(p.removed, p.dark, p.chrome, p.editor, p.panel)
	p.dangerFg = legible(p.danger, p.dark, p.chrome, p.editor, p.panel)
	p.selected = p.selection.Over(p.editor)
	p.selectedDim = p.selection.Alpha(0.5).Over(p.editor)
	p.cursor = p.accent.Alpha(0.5)
	// Markdown's bold and italic are a keyword's color, and highlighted
	// text the text's, on a box the editor paints.
	p.syntax[highlight.Strong], p.syntax[highlight.Emph] = p.syntax[highlight.Keyword], p.syntax[highlight.Keyword]
	p.syntax[highlight.Mark] = p.text
	p.deriveSyntaxRoles()
	if p.strip.A == 0 {
		p.strip = p.editor
	}
	if !p.titlebarExplicit {
		p.titlebar = p.chrome
	}
	if p.metrics.gap == 0 {
		p.metrics = defaultMetrics()
	}
	return p
}

// Additional lexical roles inherit existing theme colors unless a theme
// sets them explicitly. Recompute these after custom syntax overrides too.
func (p *palette) deriveSyntaxRoles() {
	for _, pair := range [][2]highlight.Kind{
		{highlight.Parameter, highlight.Variable}, {highlight.Binding, highlight.Variable},
		{highlight.Property, highlight.Variable}, {highlight.Object, highlight.Variable},
		{highlight.Interpolation, highlight.String}, {highlight.Bracket, highlight.Punctuation},
		{highlight.BuiltinType, highlight.Type}, {highlight.BuiltinFunction, highlight.Builtin},
		{highlight.Assignment, highlight.Operator},
	} {
		kind, parent := pair[0], pair[1]
		if !p.syntaxExplicit[kind] {
			p.syntax[kind] = p.syntax[parent]
		}
		if !p.syntaxItalicExplicit[kind] {
			p.syntaxItalic[kind] = p.syntaxItalic[parent]
		}
	}
}

// hairline is the line along the foot of the title bar and the top of the
// status bar: the theme's own rule, or the border, or none around a card.
func (p *palette) hairline() ui.Color {
	switch {
	case p.card > 0:
		return ui.Color{}
	case p.rule.A == 0:
		return p.border
	}
	return p.rule
}

// title is a title of the sidebar, Source Control or the panel as the
// palette has them: in capitals, or in sentence case.
func (p *palette) title(s string) string {
	if p.sentence {
		return sentenceCase(s)
	}
	return strings.ToUpper(s)
}

// titleLook is the size and the letter spacing of a title whose capitals
// are of size and spacing, as the palette has them: in sentence case, a
// point larger and not spaced.
func (p *palette) titleLook(size, spacing float32) (float32, float32) {
	if p.sentence {
		return size + 1, 0
	}
	return size, spacing
}

// sentenceCase returns s with its first letter alone a capital, as
// Source control for SOURCE CONTROL.
func sentenceCase(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + strings.ToLower(s[n:])
}

// seam is the line between the sidebar or the rail and what is beside
// them: the border, or none in the shell around a card.
func (p *palette) seam() ui.Color {
	if p.card > 0 {
		return ui.Color{}
	}
	return p.border
}

// sidebarSurface is the color of the sidebar's rounded card, and of the
// menus that open over it: the chrome, a step deeper than the editors, so
// the card reads as the menu's own surface.
func (p *palette) sidebarSurface() ui.Color {
	if p.card > 0 {
		return p.panel
	}
	return p.chrome
}

// sidebarRing is the color of the gap around the sidebar's card: the
// window's own surface, so that the gap meets the title bar, the status
// bar and the editors without a seam. It takes the window's opacity as
// they do, so a transparent window's backdrop shows through the gap by
// the same amount as through everything beside it.
func (p *palette) sidebarRing() ui.Color { return p.titlebar }

// legible returns col as text that reads at 4.5:1, WCAG's AA, on each of
// the backgrounds: col itself when it does, or else col mixed toward black
// on a light window, or white on a dark one, no further than it must.
func legible(col ui.Color, dark bool, backs ...ui.Color) ui.Color {
	to := black
	if dark {
		to = white
	}
	reads := func(c ui.Color) bool {
		for _, b := range backs {
			if contrast(c, b) < 4.5 {
				return false
			}
		}
		return true
	}
	for i := 0; i <= 40; i++ {
		if c := col.Mix(to, float32(i)/40); reads(c) || i == 40 {
			return c
		}
	}
	return col
}

// contrast is the WCAG contrast ratio of two opaque colors.
func contrast(x, y ui.Color) float64 {
	lum := func(c ui.Color) float64 {
		ch := func(v uint8) float64 {
			f := float64(v) / 255
			if f <= 0.03928 {
				return f / 12.92
			}
			return math.Pow((f+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	a, b := lum(x)+0.05, lum(y)+0.05
	return max(a, b) / min(a, b)
}

// applyTheme sets the window's theme from the palette of its theme,
// and returns that palette.
func (a *app) applyTheme(c *ui.Context) *palette { return applyTheme(c, &a.settings.Prefs, &a.fade) }

// applyTheme sets the theme of a window from the palette of the theme of
// the preferences, on its way there by fade, and returns that palette.
func applyTheme(c *ui.Context, prefs *settings.Prefs, fade *paletteFade) *palette {
	t := *c.Theme()
	p := fade.step(c, prefsPalette(prefs, t.Dark))
	t.Dark = p.dark
	t.Background = p.editor
	// Every state of the accent, or a hovered button would turn the
	// default theme's blue.
	t.Accent, t.AccentText = p.fill, p.fillText
	t.AccentHover, t.AccentPressed = p.fillHover, p.fillPressed
	t.Danger, t.Warning, t.Success = p.dangerFg, p.modifiedFg, p.addedFg
	t.Selection = p.selection
	t.Border = p.border
	t.Text, t.TextMuted = p.text, p.muted
	t.Focus = p.accentDim
	t.ScrollbarWidth = 8
	t.Surface, t.SurfaceHover, t.SurfacePressed = p.surface, p.surfaceHover, p.surfacePressed
	t.Scrollbar = p.scrollbar
	t.Inverse, t.InverseText = p.inverse, p.inverseText
	// The chrome's grid, so that a widget the theme draws ages the same
	// way the chrome does: the same gaps, radii and text sizes.
	m := p.metricsOr()
	t.Radius = m.radius
	t.Spacing = m.gap
	t.FontSize = m.textBody
	if p.mono {
		t.Font = prefs.FontFamily
	}
	c.SetTheme(&t)
	materialOf(c).configure(prefs)
	return p
}

// paletteFadeFor is how long the window takes from one palette to the
// next.
const paletteFadeFor = motionMove

// paletteFade takes the window from one palette to another: each of its
// colors fades to the new one's, rather than the whole window flashing.
type paletteFade struct {
	from, to *palette
	at       time.Time
	// snap takes the window to the next palette at once, as the keys of
	// the theme picker going from theme to theme want.
	snap bool
}

// step returns the palette to paint with now, on the way to to: to
// itself once there, or at once with less motion or a snap. Each palette on the way
// is new, so that what is cached of a palette is made anew.
func (f *paletteFade) step(c *ui.Context, to *palette) *palette {
	now := c.Now()
	if to != f.to {
		if f.to != nil && !f.snap && !c.Preferences().ReduceMotion {
			// From where the window is, as it may be on its way already.
			f.from, f.at = f.paletteAt(now), now
		} else {
			f.from = nil
		}
		f.to, f.snap = to, false
	}
	p := f.paletteAt(now)
	if p == to {
		f.from = nil
	} else {
		c.AnimationFrame()
	}
	return p
}

// paletteAt returns the palette at a moment of the fade.
func (f *paletteFade) paletteAt(now time.Time) *palette {
	k := float32(now.Sub(f.at)) / float32(paletteFadeFor)
	if f.from == nil || k >= 1 || k < 0 {
		return f.to
	}
	return mixPalettes(f.from, f.to, ui.EaseInOut(k))
}

// mixPalettes returns a new palette a part k of the way from x to y: its
// colors mixed, and what is not a color y's. A color either leaves zero,
// for the theme's own, is y's.
func mixPalettes(x, y *palette, k float32) *palette {
	p := *y
	v := reflect.ValueOf(&p).Elem()
	mix := func(off uintptr) {
		at := func(q *palette) *ui.Color { return (*ui.Color)(unsafe.Add(unsafe.Pointer(q), off)) }
		xc, yc := *at(x), *at(y)
		if xc != (ui.Color{}) && yc != (ui.Color{}) {
			*at(&p) = xc.Mix(yc, k)
		}
	}
	for i := range v.NumField() {
		f := v.Type().Field(i)
		switch {
		case f.Type == colorType:
			mix(f.Offset)
		case f.Type.Kind() == reflect.Array && f.Type.Elem() == colorType:
			for j := range f.Type.Len() {
				mix(f.Offset + uintptr(j)*colorType.Size())
			}
		}
	}
	return &p
}

var colorType = reflect.TypeFor[ui.Color]()

// floating styles a box that floats over the window: a dialog, as quick
// open's, a picker's or a prompt's, or a smaller box over an editor, as
// find's. The shadow is lighter on a light window, where a dark one looks
// smudged. A dialog comes in as it opens: it fades in and grows to its
// size from a little smaller, from its top, where it hangs; with less
// motion, it is there at once.
func (a *app) floating(c *ui.Context, box ui.Element, dialog bool) ui.Element {
	p := a.pal
	if dialog {
		a.dialogEntrance(c, box)
	}
	if material := materialOf(c); material.mica() {
		material.glassCard(box, p, p.metricsOr().radiusCard)
		if dialog && p.accentDialogs {
			box.BorderColor(p.accent.Alpha(0.8))
		}
		return box
	}
	box.Background(c.Theme().Surface).Border(1, p.border)
	if dialog && p.accentDialogs {
		// In the accent, as Claude Code boxes what it asks.
		box.Border(1, p.accent.Alpha(0.8))
	}
	switch {
	case dialog && p.dark:
		box.Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.40))
	case dialog:
		box.Shadow(0, 12, 32, 0, ui.RGBA(20, 20, 40, 0.16))
	case p.dark:
		box.Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.30))
	default:
		box.Shadow(0, 4, 14, 0, ui.RGBA(20, 20, 40, 0.12))
	}
	return box
}

func (a *app) dialogEntrance(c *ui.Context, box ui.Element) {
	progress := box.Keyframes("dialog-in", showIn(a.keyedOpen(c)))
	scale := 0.98 + 0.02*progress
	box.Opacity(progress).TransformOrigin(0.5, 0).Scale(scale, scale)
}

// dialogBack is the layer under a dialog, over the whole window, that
// the dialog is in. It shows at once, as the
// keys that open a dialog want it there to type in.
func (a *app) dialogBack(c *ui.Context) ui.Element {
	return ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).AlignItems(ui.Center).Modal()
}

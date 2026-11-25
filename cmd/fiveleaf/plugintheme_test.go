package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/plugin"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

const nightPlugin = `{
  "name": "night",
  "contributes": {
    "themes": [{"id": "night.dark", "label": "Night", "path": "night.json"}, {"id": "night.paper", "label": "Paper", "path": "paper.json"}],
    "languages": [{"id": "d2", "extensions": [".d2"], "lexer": "yaml", "lineComment": "#"}],
    "snippets": [{"language": "d2", "path": "d2.json"}]
  }
}`

// A VS Code theme, as its marketplace has them.
const nightTheme = `{
  // comments, as VS Code's themes have
  "name": "Night", "type": "dark",
  "colors": {"editor.background": "#282a36", "sideBar.background": "#21222c", "editor.selectionBackground": "#44475a80",
    "terminal.ansiRed": "#ff5555", "statusBar.background": "#191a21"},
  "tokenColors": [
    {"scope": "comment", "settings": {"foreground": "#6272a4"}},
    {"scope": ["keyword", "storage.type"], "settings": {"foreground": "#ff79c6"}},
    {"scope": "entity.name.function", "settings": {"foreground": "#50fa7b"}},
  ]
}`

// FiveLeaf's own format, over Claude's light palette.
const paperTheme = `{"base": "claude", "dark": false, "colors": {"accent": "#c15f3c"}, "syntax": {"string": "#0a7d32"}}`

const d2Snippets = `{
  "Box": {"prefix": ["box", "shape"], "body": ["${1:name}: {", "\tshape: ${2|rectangle,circle|}", "}$0"], "description": "A shape"}
}`

func TestPluginThemesLanguagesSnippets(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := newHarness(t, false)
	withPlugins(hs, map[string]map[string]string{"night": {
		"plugin.json": nightPlugin, "night.json": nightTheme, "paper.json": paperTheme, "d2.json": d2Snippets,
	}})
	withAppearancePlugins(t)
	same := func(a, b ui.Color) bool { return a == b }

	p := paletteFor("night.dark", false)
	if !p.dark || !same(p.editor, ui.Hex("#282a36")) || !same(p.chrome, ui.Hex("#21222c")) || !same(p.status, ui.Hex("#191a21")) {
		t.Fatalf("night palette: dark=%v editor=%v chrome=%v", p.dark, p.editor, p.chrome)
	}
	if !same(p.syntax[highlight.Comment], ui.Hex("#6272a4")) || !same(p.syntax[highlight.Type], ui.Hex("#ff79c6")) ||
		!same(p.syntax[highlight.Function], ui.Hex("#50fa7b")) || !same(p.ansi[1], ui.Hex("#ff5555")) {
		t.Fatal("night's token and terminal colors")
	}
	if !alwaysDark("night.dark") || alwaysDark("night.paper") {
		t.Fatal("always dark")
	}
	paper := paletteFor("night.paper", true)
	if paper.dark || !same(paper.accent, ui.Hex("#c15f3c")) || !same(paper.editor, claudeLightPal.editor) ||
		!same(paper.syntax[highlight.String], ui.Hex("#0a7d32")) {
		t.Fatal("paper palette")
	}
	ids := []string{}
	for _, c := range allThemes() {
		ids = append(ids, c.id)
	}
	if !slices.Contains(ids, "night.dark") || !slices.Contains(ids, "night.paper") {
		t.Fatalf("themes = %v", ids)
	}
	hs.a.run("view.theme.night.dark")
	if hs.a.settings.Theme != "night.dark" {
		t.Fatalf("theme = %q", hs.a.settings.Theme)
	}
	hs.settle() // draws in it

	// A file of the plugin's language.
	path := hs.path("diagram.d2")
	os.WriteFile(path, []byte("a: {\n}\n"), 0o644)
	hs.a.openFile(path, true)
	hs.settle()
	d := hs.a.activeEditor()
	if languageOf(d.document) != "d2" || commentFor(d.document) != "#" || highlight.Name(highlight.Lexer(path)) != "YAML" {
		t.Fatalf("language %q, comment %q, lexer %q", languageOf(d.document), commentFor(d.document), highlight.Name(highlight.Lexer(path)))
	}
	list := snippetsFor("d2")
	if len(list) != 2 || list[0].prefix != "box" || list[1].prefix != "shape" {
		t.Fatalf("snippets = %+v", list)
	}
	text, stop := snippetText(list[0].body, map[string]string{})
	if text != "name: {\n\tshape: rectangle\n}" || text[stop[0]:stop[1]] != "name" {
		t.Fatalf("snippet text %q, stop %v", text, stop)
	}
	// Inserted on an indented line, indented as it is, the placeholder
	// selected.
	os.WriteFile(path, []byte("a: {\n  \n}\n"), 0o644)
	d.buf.Set("a: {\n  \n}\n")
	hs.settle()
	hs.place(d, 7, 7)
	hs.a.queueEdit(d, func(d *editor) { expandSnippet(d, list[0].body, hs.a.snippetVars(d)) })
	hs.settle()
	want := "a: {\n  name: {\n  " + d.indent.unit() + "shape: rectangle\n  }\n}\n"
	if got := d.buf.String(); got != want {
		t.Fatalf("after the snippet: %q, want %q", got, want)
	}
	if s := d.buf.Snapshot(); s.Slice(min(d.sel[0], d.sel[1]), max(d.sel[0], d.sel[1])) != "name" {
		t.Fatalf("selected %v", d.sel)
	}
	if v := snippetVarsText(hs.a, d); v != "diagram" {
		t.Fatalf("TM_FILENAME_BASE = %q", v)
	}
}

func snippetVarsText(a *app, d *editor) string {
	out, _ := snippetText("${TM_FILENAME_BASE}", a.snippetVars(d))
	return out
}

// A theme may be written in settings.json, and is found as one of a file
// is: as a base, in the box, as a command, and in the window.
func TestInlineThemes(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := newHarness(t, false)
	withThemeHost(hs)
	withAppearancePlugins(t)
	fiveleafDark := paletteFor("", true)
	hs.a.settings.Themes = map[string]json.RawMessage{
		"paper": json.RawMessage(`{
  "name": "Paper",
  "base": "light",
  "mono": true,
  "colors": {"editor": "#fdfcfa", "accent": "#c15f3c"},
  "syntax": {"keyword": "#0a7d32"},
  "metrics": {"gap": 6, "text": {"body": 13}}
}`),
		"paper-ink": json.RawMessage(`{"base": "paper", "colors": {"editor": "#f4ecd8"}}`),
		"broken":    json.RawMessage(`{"colors":`),
	}
	ids := []string{}
	for _, c := range allThemes() {
		ids = append(ids, c.id)
	}
	if !slices.Contains(ids, "paper") || !slices.Contains(ids, "paper-ink") || slices.Contains(ids, "broken") {
		t.Fatalf("themes = %v", ids)
	}
	p := paletteFor("paper", true)
	if p == nil || p.dark || !p.mono || p.editor != ui.Hex("#fdfcfa") || p.accent != ui.Hex("#c15f3c") ||
		p.syntax[highlight.Keyword] != ui.Hex("#0a7d32") || p.metricsOr().gap != 6 || p.metricsOr().textBody != 13 {
		t.Fatalf("paper palette: %+v", p)
	}
	child := paletteFor("paper-ink", false)
	if child == nil || child.editor != ui.Hex("#f4ecd8") || !child.mono {
		t.Fatalf("a theme based on a theme of the settings: %+v", child)
	}
	if alwaysDark("paper") || alwaysDark("paper-ink") {
		t.Error("a theme of the settings follows its base's darkness")
	}
	found := false
	for _, c := range hs.a.commands() {
		if c.id == "view.theme.paper" && c.label == "Paper" {
			found = true
		}
	}
	if !found {
		t.Error("a theme of the settings has no command")
	}
	hs.a.setTheme("paper-ink")
	hs.settle()
	if hs.a.pal != paletteFor("paper-ink", false) || hs.a.pal.editor != ui.Hex("#f4ecd8") {
		t.Error("the window is not in the theme of the settings")
	}

	// A save of the settings is read again, as the file's themes are.
	hs.a.settings.Themes["paper"] = json.RawMessage(`{"base": "light", "colors": {"editor": "#fbf7ef"}}`)
	themeFileChanged()
	if p := paletteFor("paper", false); p == nil || p.editor != ui.Hex("#fbf7ef") {
		t.Fatalf("a theme of the settings is not read again: %+v", p)
	}
	hs.a.settings.Themes["late"] = json.RawMessage(`{"base": "dark", "colors": {"editor": "#111318"}}`)
	themeFileChanged()
	hs.a.palette = nil
	found = false
	for _, c := range hs.a.commands() {
		if c.id == "view.theme.late" {
			found = true
		}
	}
	if !found {
		t.Error("a theme saved in the settings has no command")
	}

	// A theme of the settings may not take FiveLeaf's own name.
	hs.a.settings.Themes["matrix"] = json.RawMessage(`{"colors": {"editor": "#000000"}}`)
	if p := paletteFor("matrix", true); !samePalette(p, matrixPal) {
		t.Error("a theme of the settings takes The Matrix's name")
	}
	hs.a.settings.Themes["fiveleaf"] = json.RawMessage(`{"colors": {"editor": "#000000"}}`)
	if p := paletteFor("fiveleaf", true); !samePalette(p, fiveleafDark) {
		t.Error("a theme of the settings takes FiveLeaf's own name")
	}
	if _, ok := inlineThemeByID("fiveleaf"); ok {
		t.Error("a theme of the settings named fiveleaf is found")
	}
	fiveleafCommands := 0
	for _, c := range hs.a.commands() {
		if c.id == "view.theme.fiveleaf" {
			fiveleafCommands++
		}
	}
	if fiveleafCommands != 1 {
		t.Fatalf("%d themes named fiveleaf have a command", fiveleafCommands)
	}
	hs.a.setTheme("fiveleaf")
	hs.a.run("view.theme.fiveleaf")
	if hs.a.settings.Theme != "" {
		t.Errorf("the command of FiveLeaf's theme chose %q", hs.a.settings.Theme)
	}
	delete(hs.a.settings.Themes, "fiveleaf")
	if !sameThemes(hs.a.settings.Themes, hs.a.settings.Themes) ||
		sameThemes(hs.a.settings.Themes, map[string]json.RawMessage{"paper": json.RawMessage(`{}`)}) {
		t.Error("sameThemes")
	}
}

// The Theme menu lists FiveLeaf's themes in their order, then the themes of
// the settings, the user and the plugins, and checks the one the settings
// choose.
func TestThemeMenuListsCustomThemes(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := newHarness(t, false)
	var refreshed int
	a := hs.a
	h := newPluginHost(func() []*app { return []*app{a} }, a.post, func() *settings.Settings { return a.settings }, func() { refreshed++ })
	h.reg = plugin.New(plugin.Dirs{Storage: t.TempDir()}, h)
	h.reg.Version = "dev"
	old := plugins
	plugins = h
	h.syncPrefs()
	t.Cleanup(func() { plugins = old })
	t.Cleanup(themeFileChanged)
	withAppearancePlugins(t)
	hs.settle()
	refreshed = 0
	dir := plugin.UserDirs().Themes
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zebra.json"), []byte(`{"base": "light"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.settings.Themes = map[string]json.RawMessage{
		"paper":  json.RawMessage(`{"base": "light"}`),
		"matrix": json.RawMessage(`{"colors": {"editor": "#000000"}}`),
	}
	hs.a.settings.Theme = "paper"

	var ran []string
	themeMenu := func() *mygo.MenuItem {
		for _, it := range hs.a.menu(false, func(id string, _ *mygo.Window) { ran = append(ran, id) }).Items() {
			if it.Label == locales.MenuView() {
				for _, sub := range it.Submenu {
					if sub.Label == locales.MenuTheme() {
						return sub
					}
				}
			}
		}
		t.Fatal("the View menu has no Theme submenu")
		return nil
	}
	var labels, ids []string
	for _, it := range themeMenu().Submenu {
		if it.Type != mygo.MenuItemRadio || !strings.HasPrefix(it.ID, "view.theme.") {
			continue
		}
		labels = append(labels, it.Label)
		ids = append(ids, it.ID)
	}
	if want := []string{"FiveLeaf", locales.CmdViewThemeMatrix(), "Claude", "Qoder", "Grok Bot", "Cursor",
		locales.CmdViewThemePurpleGold(), locales.CmdViewThemePumpkin(), locales.CmdViewThemeWitchsBrew(),
		locales.CmdViewThemeBloodMoon(), "One Dark Pro", locales.CmdViewThemeAyu(), "paper", "zebra"}; !slices.Equal(labels, want) {
		t.Fatalf("theme menu = %v, want %v", labels, want)
	}
	if want := []string{"view.theme.fiveleaf", "view.theme.matrix", "view.theme.claude", "view.theme.qoder",
		"view.theme.grokbot", "view.theme.cursor", "view.theme.purplegold", "view.theme.pumpkin",
		"view.theme.witchsbrew", "view.theme.bloodmoon", "view.theme.onedarkpro", "view.theme.ayu",
		"view.theme.paper", "view.theme.zebra"}; !slices.Equal(ids, want) {
		t.Fatalf("theme menu commands = %v, want %v", ids, want)
	}

	checked := 0
	var paper *mygo.MenuItem
	for _, it := range themeMenu().Submenu {
		if it.ID == "view.theme.paper" {
			paper = it
		}
		if it.Type != mygo.MenuItemRadio || !strings.HasPrefix(it.ID, "view.theme.") || !it.Checked {
			continue
		}
		checked++
		if it.ID != "view.theme.paper" {
			t.Errorf("%s is checked, not paper", it.ID)
		}
	}
	if checked != 1 {
		t.Fatalf("%d themes are checked", checked)
	}
	paper.Click(paper, nil)
	if want := []string{"view.theme.paper"}; !slices.Equal(ran, want) {
		t.Errorf("the Paper item runs %v, want %v", ran, want)
	}

	// A theme saved in the settings is in the menu built after the change,
	// and the one the settings choose is the one checked.
	hs.a.settings.Themes["late"] = json.RawMessage(`{"base": "dark"}`)
	hs.a.settings.Theme = "late"
	plugins.Changed()
	hs.settle()
	if refreshed != 1 {
		t.Fatalf("the theme's host refreshed the menu %d times", refreshed)
	}
	labels = nil
	checked = 0
	for _, it := range themeMenu().Submenu {
		if it.Type != mygo.MenuItemRadio || !strings.HasPrefix(it.ID, "view.theme.") {
			continue
		}
		labels = append(labels, it.Label)
		if it.Checked {
			checked++
			if it.Label != "late" {
				t.Errorf("%s is checked, not late", it.Label)
			}
		}
	}
	if !slices.Contains(labels, "late") {
		t.Errorf("the menu does not list a theme saved after the refresh: %v", labels)
	}
	if checked != 1 {
		t.Errorf("%d themes are checked after the change", checked)
	}
}

// A plugin's theme comes after the settings' themes and the user's.
func TestThemeMenuListsPluginThemes(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := newHarness(t, false)
	withPlugins(hs, map[string]map[string]string{"night": {
		"plugin.json": `{"name": "night", "contributes": {"themes": [{"id": "night.dark", "label": "Night", "path": "night.json"}]}}`,
		"night.json":  `{"base": "dark"}`,
	}})
	withAppearancePlugins(t)
	var labels []string
	for _, it := range hs.a.menu(false, func(string, *mygo.Window) {}).Items() {
		if it.Label != locales.MenuView() {
			continue
		}
		for _, sub := range it.Submenu {
			if sub.Label != locales.MenuTheme() {
				continue
			}
			for _, th := range sub.Submenu {
				if th.Type == mygo.MenuItemRadio && strings.HasPrefix(th.ID, "view.theme.") {
					labels = append(labels, th.Label)
				}
			}
		}
	}
	if want := []string{"FiveLeaf", locales.CmdViewThemeMatrix(), "Claude", "Qoder", "Grok Bot", "Cursor",
		locales.CmdViewThemePurpleGold(), locales.CmdViewThemePumpkin(), locales.CmdViewThemeWitchsBrew(),
		locales.CmdViewThemeBloodMoon(), "One Dark Pro", locales.CmdViewThemeAyu(), "Night"}; !slices.Equal(labels, want) {
		t.Errorf("theme menu with a plugin = %v, want %v", labels, want)
	}
}

// A color a theme names that is not a hex color is left out, and the base
// palette's color stays: a file may say #zzzzzz, #12345 or #ggg.
func TestInvalidThemeColorsAreIgnored(t *testing.T) {
	withAppearancePlugins(t)
	light := paletteFor("", false)
	for _, bad := range []string{"#zzzzzz", "#12345", "#ggg"} {
		f, err := parseThemeFile([]byte(`{"base": "light", "colors": {"editor": "` + bad + `", "accent": "` + bad + `"},
  "syntax": {"keyword": "` + bad + `"}}`))
		if err != nil {
			t.Fatalf("%s: a theme does not parse: %v", bad, err)
		}
		p := paletteOfThemeFile(f, "bad", false, false, nil)
		if p == nil {
			t.Fatalf("%s: a theme with an invalid color has no palette", bad)
		}
		if p.editor != light.editor || p.accent != light.accent ||
			p.syntax[highlight.Keyword] != light.syntax[highlight.Keyword] {
			t.Errorf("%s: an invalid color changes the palette: editor %v, accent %v", bad, p.editor, p.accent)
		}
	}
}

func TestBasicThemeTitleBarOverrides(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	for _, view := range []struct {
		colors map[string]string
		want   ui.Color
	}{
		{map[string]string{"sideBar.background": "#d0d1d2"}, ui.Hex("#d0d1d2")},
		{map[string]string{"sideBar.background": "#d0d1d2", "titleBar.activeBackground": "#e0e1e2"}, ui.Hex("#e0e1e2")},
		{map[string]string{"chrome": "invalid"}, lightPal.titlebar},
	} {
		file := &themeFile{Base: "light", Colors: view.colors}
		if got := paletteOfThemeFile(file, "custom", false, false, nil).titlebar; got != view.want {
			t.Errorf("theme colors %v give title bar %v, want %v", view.colors, got, view.want)
		}
	}
}

// A theme of the user's own, in ~/.fiveleaf/themes: a file each, named for
// its id, with FiveLeaf's colors, numbers and layout.
func TestUserThemes(t *testing.T) {
	withAppearancePlugins(t)
	fiveleafDark := paletteFor("", true)
	hs := newHarness(t, false)
	dir := plugin.UserDirs().Themes
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("paper.json", `{
  "base": "light",
  "mono": true,
  "sentence": true,
  "card": 10,
  "colors": {"editor": "#fdfcfa", "accent": "#c15f3c", "editor.foreground": "#1a1918"},
  "syntax": {"keyword": "#0a7d32"},
  "metrics": {"gap": 6, "radius": 8, "text": {"body": 13, "title": 14}, "weightBold": 650}
}`)
	write("night.jsonc", `{
  // A dark theme, which follows the window's appearance.
  "type": "dark",
  "colors": {"editor.background": "#282a36", "focusBorder": "#bd93f9"}
}`)

	ids := []string{}
	for _, c := range allThemes() {
		ids = append(ids, c.id)
	}
	if !slices.Contains(ids, "paper") || !slices.Contains(ids, "night") {
		t.Fatalf("themes = %v", ids)
	}
	p := paletteFor("paper", true)
	if p == nil || p.dark || !p.mono || !p.sentence || p.card != 10 {
		t.Fatalf("paper palette: %+v", p)
	}
	if p.editor != ui.Hex("#fdfcfa") || p.accent != ui.Hex("#c15f3c") || p.syntax[highlight.Keyword] != ui.Hex("#0a7d32") {
		t.Fatal("paper's colors")
	}
	if m := p.metricsOr(); m.gap != 6 || m.radius != 8 || m.textBody != 13 || m.textTitle != 14 || m.weightBold != 650 {
		t.Fatalf("paper's metrics: %+v", m)
	}
	n := paletteFor("night", false)
	if n == nil || !n.dark || n.editor != ui.Hex("#282a36") || n.accent != ui.Hex("#bd93f9") {
		t.Fatalf("night palette: %+v", n)
	}
	if !alwaysDark("night") || alwaysDark("paper") {
		t.Fatal("always dark")
	}
	// Darkness comes through a base: Matrix is dark whatever the window,
	// and so is a theme built on it.
	write("matrixish.json", `{"base": "matrix"}`)
	if !alwaysDark("matrixish") {
		t.Fatal("a theme based on The Matrix is not always dark")
	}
	if p := paletteFor("matrixish", false); p == nil || !p.dark || p.editor != matrixPal.editor {
		t.Fatalf("a theme based on The Matrix is not dark: %+v", p)
	}
	// A theme based on "dark" or "light" is too, which the native window
	// follows as the palette does.
	for id, dark := range map[string]bool{"ink.json": true, "snow.json": false} {
		base := "light"
		if dark {
			base = "dark"
		}
		write(id, `{"base": "`+base+`"}`)
		name := strings.TrimSuffix(id, ".json")
		if alwaysDark(name) != dark || paletteFor(name, !dark).dark != dark {
			t.Errorf("%s, based on %s, is dark=%v", name, base, dark)
		}
	}
	// A light base stays light in a dark window, and a dark file passes
	// its darkness through another of the user's themes.
	write("night-child.json", `{"base": "night"}`)
	if !alwaysDark("night-child") {
		t.Fatal("darkness does not pass through another theme")
	}
	if p := paletteFor("night-child", false); p == nil || !p.dark {
		t.Fatal("a theme based on a dark theme is not dark")
	}
	// A theme whose colors are its own keeps them, with the colors the
	// window draws from them filled in.
	write("kept.json", `{
  "base": "light", "derived": false,
  "colors": {"editor": "#ffffff", "text": "#111111", "selection": "#dfe4ff", "accent": "#2244cc",
    "danger": "#b00020", "added": "#117733", "modified": "#aa6600", "removed": "#aa2222"}
}`)
	kept := paletteFor("kept", false)
	if kept == nil || kept.editor != ui.Hex("#ffffff") || kept.text != ui.Hex("#111111") {
		t.Fatalf("kept palette: %+v", kept)
	}
	if kept.cursor == (ui.Color{}) || kept.selected == (ui.Color{}) || kept.dangerFg == (ui.Color{}) ||
		kept.syntax[highlight.Strong] == (ui.Color{}) {
		t.Fatalf("a kept palette leaves colors the window draws: %+v", kept)
	}
	// Two files of one id are one theme, the first by name.
	write("paper.jsonc", `{"colors": {"editor": "#000000"}}`)
	count := 0
	for _, c := range allThemes() {
		if c.id == "paper" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d themes named paper", count)
	}
	hs.a.setTheme("paper")
	if hs.a.settings.Theme != "paper" {
		t.Fatalf("theme = %q", hs.a.settings.Theme)
	}
	hs.settle()

	// A file that does not read leaves FiveLeaf's own theme.
	write("broken.json", `{"colors":`)
	if p := paletteFor("broken", true); !samePalette(p, fiveleafDark) {
		t.Errorf("invalid theme falls back to editor %v, want installed FiveLeaf %v", p.editor, fiveleafDark.editor)
	}
	for _, c := range allThemes() {
		if c.id == "broken" {
			t.Fatal("a theme that does not read is listed")
		}
	}
	// Another theme may be its base.
	write("sepia.json", `{"base": "paper", "colors": {"editor": "#f4ecd8"}}`)
	if p := paletteFor("sepia", false); p == nil || p.editor != ui.Hex("#f4ecd8") || !p.mono {
		t.Fatalf("sepia palette: %+v", p)
	}
	// A save is read again, as the plugin watcher asks for.
	write("sepia.json", `{"base": "paper", "colors": {"editor": "#eee4cf"}}`)
	themeFileChanged()
	if p := paletteFor("sepia", false); p == nil || p.editor != ui.Hex("#eee4cf") {
		t.Fatalf("a saved theme is not read again: %+v", p)
	}
	// A theme based on itself, or on another that leads back to it, is
	// left out rather than read for ever.
	write("loop-a.json", `{"base": "loop-b"}`)
	write("loop-b.json", `{"base": "loop-a"}`)
	for _, c := range allThemes() {
		if c.id == "loop-a" || c.id == "loop-b" {
			t.Fatalf("%s, which has no base, is listed", c.id)
		}
	}
	if p := paletteFor("loop-a", true); !samePalette(p, fiveleafDark) {
		t.Errorf("cyclic theme falls back to editor %v, want installed FiveLeaf %v", p.editor, fiveleafDark.editor)
	}
	// A theme of the user's has a command, as a plugin's does.
	found := false
	for _, c := range hs.a.commands() {
		if c.id == "view.theme.paper" && c.label == "paper" {
			found = true
		}
	}
	if !found {
		t.Error("a theme of the user's has no command")
	}
	// A name FiveLeaf's own themes have is not a theme of the user's.
	write("matrix.json", `{"colors": {"editor": "#000000"}}`)
	if p := paletteFor("matrix", true); !samePalette(p, matrixPal) {
		t.Fatal("a user's theme takes The Matrix's name")
	}
	write("fiveleaf.json", `{"colors": {"editor": "#000000"}}`)
	themeFileChanged()
	if p := paletteFor("fiveleaf", true); !samePalette(p, fiveleafDark) {
		t.Fatal("a user's theme takes FiveLeaf's own name")
	}
	if _, ok := userThemeByID("fiveleaf"); ok {
		t.Fatal("a user's theme named fiveleaf is found")
	}
	fiveleafCommands := 0
	for _, c := range hs.a.commands() {
		if c.id == "view.theme.fiveleaf" {
			fiveleafCommands++
		}
	}
	if fiveleafCommands != 1 {
		t.Fatalf("%d themes named fiveleaf have a command", fiveleafCommands)
	}
	hs.a.setTheme("fiveleaf")
	hs.a.run("view.theme.fiveleaf")
	if hs.a.settings.Theme != "" {
		t.Errorf("the command of FiveLeaf's theme chose %q", hs.a.settings.Theme)
	}
	os.Remove(filepath.Join(dir, "fiveleaf.json"))
	themeFileChanged()
}

// A theme that is not derived keeps the button states of its base,
// unless it is derived, which is the default.
func TestThemeDerivedFalseKeepsButtonStates(t *testing.T) {
	withAppearancePlugins(t)
	dir := plugin.UserDirs().Themes
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("kept-buttons.json", `{"base": "qoder", "derived": false, "colors": {"fill": "#123456"}}`)
	write("derived-buttons.json", `{"base": "qoder", "colors": {"fill": "#123456"}}`)

	kept := paletteFor("kept-buttons", false)
	if kept == nil || kept.fill != ui.Hex("#123456") {
		t.Fatalf("kept palette: %+v", kept)
	}
	if kept.fillHover != qoderLightPal.fillHover || kept.fillPressed != qoderLightPal.fillPressed {
		t.Errorf("derived:false changed the base's button states: hover %v pressed %v",
			kept.fillHover, kept.fillPressed)
	}
	derived := paletteFor("derived-buttons", false)
	if derived == nil {
		t.Fatal("the derived theme has no palette")
	}
	fill := ui.Hex("#123456")
	if derived.fillHover != fill.Mix(white, 0.1) || derived.fillPressed != fill.Mix(black, 0.15) {
		t.Errorf("a derived theme does not derive its button states: hover %v pressed %v",
			derived.fillHover, derived.fillPressed)
	}
}

func TestThemeMetricsBounds(t *testing.T) {
	for _, metric := range []string{
		`{"gap": 0}`, `{"pad": 41}`, `{"inset": -1}`, `{"radius": 25}`,
		`{"text": 29}`, `{"text": {"body": 5}}`, `{"text": {"title": 29}}`,
		`{"weightMedium": 99}`, `{"weightBold": 901}`, `{"row": 11}`, `{"control": 65}`,
	} {
		if _, err := parseThemeFile([]byte(`{"base": "dark", "metrics": ` + metric + `}`)); err == nil {
			t.Errorf("theme metrics %s are accepted", metric)
		}
	}
	f, err := parseThemeFile([]byte(`{"base": "dark", "metrics": {
  "gap": 1, "pad": 0, "inset": 48, "radius": 0, "radiusField": 24, "radiusCard": 40,
  "radiusFull": 1000, "text": {"micro": 6, "small": 28, "body": 12, "title": 13},
  "weightMedium": 100, "weightBold": 900, "row": 12, "control": 64
}}`))
	if err != nil {
		t.Fatalf("a theme within the bounds does not parse: %v", err)
	}
	p := paletteOfThemeFile(f, "bounds", true, true, nil)
	if p == nil {
		t.Fatal("a theme within the bounds has no palette")
	}
	m := p.metricsOr()
	if m.gap != 1 || m.pad != 0 || m.inset != 48 || m.radius != 0 || m.radiusField != 24 ||
		m.radiusCard != 40 || m.radiusFull != 1000 || m.textMicro != 6 || m.textSmall != 28 ||
		m.textBody != 12 || m.textTitle != 13 || m.weightMedium != 100 || m.weightBold != 900 ||
		m.row != 12 || m.control != 64 {
		t.Fatalf("the metrics within the bounds are not applied: %+v", m)
	}

	for name, v := range map[string]float32{
		"NaN": float32(math.NaN()), "+Inf": float32(math.Inf(1)), "-Inf": float32(math.Inf(-1)),
	} {
		m := metricsFile{Gap: &v}
		to := defaultMetrics()
		if err := m.apply(&to); err == nil {
			t.Errorf("%s is accepted as a gap", name)
		}
	}

	for name, theme := range map[string]string{
		"card":  `{"base": "dark", "card": 41}`,
		"grain": `{"base": "dark", "grain": 1.1}`,
	} {
		f, err := parseThemeFile([]byte(theme))
		if err != nil {
			t.Fatalf("%s does not parse: %v", name, err)
		}
		if p := paletteOfThemeFile(f, name, true, true, nil); p != nil {
			t.Errorf("%s out of range is accepted", name)
		}
	}
}

// A plugin theme may not follow a link out of the plugin's folder.
func TestPluginThemeSymlinkCannotEscape(t *testing.T) {
	hs := newHarness(t, false)
	withPlugins(hs, map[string]map[string]string{"links": {
		"plugin.json": `{
  "name": "links",
  "contributes": {"themes": [
    {"id": "links.safe", "label": "Safe", "path": "safe.json"},
    {"id": "links.leak", "label": "Leak", "path": "leak.json"}
  ]}
}`,
	}})
	dir := filepath.Join(plugins.reg.Dirs.Plugins, "links")
	if err := os.WriteFile(filepath.Join(dir, "real.json"), []byte(`{"base": "light"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(plugins.reg.Dirs.Plugins, "outside.json")
	if err := os.WriteFile(outside, []byte(`{"base": "dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.json", filepath.Join(dir, "safe.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "leak.json")); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range allThemes() {
		ids = append(ids, c.id)
	}
	if !slices.Contains(ids, "links.safe") {
		t.Errorf("a link inside the plugin's folder is not read: %v", ids)
	}
	if slices.Contains(ids, "links.leak") {
		t.Errorf("a link out of the plugin's folder is read: %v", ids)
	}
}

// Opening the themes folder reports a folder it cannot make, rather than
// showing one that is not there.
func TestOpenThemesFolderReportsFailure(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIVELEAF_DATA_DIR", blocked)
	hs := newHarness(t, false)
	hs.a.errors = nil
	hs.a.openThemesFolder()
	if len(hs.a.errors) == 0 {
		t.Fatal("opening an impossible themes folder reports nothing")
	}
}

// A theme of the user's in the windows: its colors, its code and its
// numbers, in the window and in the box of the themes.
func TestShotUserTheme(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	withAppearancePlugins(t)
	dir := plugin.UserDirs().Themes
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "paper.json"), []byte(`{
  "name": "Paper",
  "base": "light",
  "colors": {"editor": "#fdfcfa", "chrome": "#f2efe9", "panel": "#f7f4ee", "accent": "#b4552d",
    "selection": "#b4552d33", "status": "#ece7df"},
  "syntax": {"keyword": "#7a3e9d", "string": "#2c7a39", "comment": "#8b8579"},
  "metrics": {"gap": 5, "radius": 8, "text": {"body": 13, "title": 14}}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	hs := newHarness(t, false)
	hs.a.setTheme("paper")
	hs.settle()
	if hs.a.pal.accent != ui.Hex("#b4552d") {
		t.Fatalf("the window is not in Paper: %v", hs.a.pal.accent)
	}
	hs.shot("user-theme")
	hs.a.openThemePicker()
	hs.settle()
	hs.shot("user-theme-picker")
}

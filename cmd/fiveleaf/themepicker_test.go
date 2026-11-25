package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
)

// The box of the color themes shows each theme as the choice moves to it,
// and each appearance as it is switched to: Return keeps them, and Escape
// goes back to those it opened on.
func TestThemePickerPreviewsAndKeeps(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	a := hs.a
	a.run("view.selectTheme")
	hs.settle()
	if !a.themes.open || a.themes.sel != 0 {
		t.Fatalf("the box is not open on FiveLeaf's theme: open %v, row %d", a.themes.open, a.themes.sel)
	}
	hs.shot("theme-picker-fiveleaf")
	for range 4 {
		hs.key(0, ui.KeyDown)
	}
	hs.settle()
	if a.settings.Theme != themeGrokBot || !samePalette(a.pal, grokBotDarkPal) {
		t.Fatalf("the window shows %q, not Grok Bot, as the choice moves to it", a.settings.Theme)
	}
	hs.key(0, ui.KeyLeft)
	hs.settle()
	if a.settings.Appearance != "" {
		t.Errorf("← from the system's appearance gives %q", a.settings.Appearance)
	}
	hs.key(0, ui.KeyRight)
	hs.settle()
	if a.settings.Appearance != "light" || !samePalette(a.pal, grokBotLightPal) {
		t.Fatalf("→ does not show Grok Bot light: %q", a.settings.Appearance)
	}
	hs.shot("theme-picker-grokbot-light")
	hs.key(0, ui.KeyEscape)
	hs.settle()
	if a.themes.open || a.settings.Theme != "" || a.settings.Appearance != "" || !samePalette(a.pal, paletteFor("", true)) {
		t.Fatalf("Escape does not go back to the theme the box opened on: %q %q", a.settings.Theme, a.settings.Appearance)
	}

	a.run("view.selectTheme")
	hs.settle()
	hs.tt.Type("claude")
	hs.settle()
	if a.settings.Theme != themeClaude {
		t.Fatalf("the window does not show the theme the query matches, but %q", a.settings.Theme)
	}
	hs.key(0, ui.KeyEnter)
	hs.settle()
	if a.themes.open || a.settings.Theme != themeClaude || !samePalette(a.pal, claudeDarkPal) {
		t.Errorf("Return does not keep Claude: %q", a.settings.Theme)
	}
}

// The box reads a theme file again while it is open, and keeps its
// chosen row on the theme it shows when the file's neighbors change.
func TestThemePickerReloadsChangedThemes(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	dir := plugin.UserDirs().Themes
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"base": "light"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("alpha.json")
	write("zebra.json")

	hs := newHarness(t, true)
	a := hs.a
	a.run("view.selectTheme")
	hs.settle()
	hs.tt.Type("zebra")
	hs.settle()
	if !a.themes.open || a.settings.Theme != "zebra" {
		t.Fatalf("the box is not on zebra: open %v, theme %q", a.themes.open, a.settings.Theme)
	}

	// Removing a theme before zebra moves its row: the cached results'
	// index would now be out of range.
	if err := os.Remove(filepath.Join(dir, "alpha.json")); err != nil {
		t.Fatal(err)
	}
	themeFileChanged()
	hs.settle()
	themes := allThemes()
	if len(a.themes.results) != 1 {
		t.Fatalf("the box lists %d themes for zebra", len(a.themes.results))
	}
	if a.themes.sel < 0 || a.themes.sel >= len(a.themes.results) {
		t.Fatalf("the chosen row %d is outside the %d results", a.themes.sel, len(a.themes.results))
	}
	if i := a.themes.results[a.themes.sel].Index; i < 0 || i >= len(themes) || themes[i].id != "zebra" {
		t.Fatalf("the chosen row is theme %d of %d, not zebra", i, len(themes))
	}

	// Removing the chosen theme names no row, and the box clamps the one
	// it had into the themes that are left.
	hs.key(ui.Cmd, ui.KeyA)
	hs.key(0, ui.KeyBackspace)
	if a.themes.query != "" {
		t.Fatalf("the query is %q, not empty", a.themes.query)
	}
	if err := os.Remove(filepath.Join(dir, "zebra.json")); err != nil {
		t.Fatal(err)
	}
	themeFileChanged()
	hs.settle()
	themes = allThemes()
	if !a.themes.open || len(a.themes.results) != len(themes) {
		t.Fatalf("the box lists %d of %d themes after zebra is gone", len(a.themes.results), len(themes))
	}
	if a.themes.sel < 0 || a.themes.sel >= len(a.themes.results) {
		t.Fatalf("the chosen row %d is outside the %d results", a.themes.sel, len(a.themes.results))
	}
	if i := a.themes.results[a.themes.sel].Index; i < 0 || i >= len(themes) {
		t.Fatalf("the chosen row is theme %d of %d", i, len(themes))
	}
}

// The window fades from one palette to the next, each color on its way,
// and settles on the new palette itself.
func TestPaletteFade(t *testing.T) {
	withAppearancePlugins(t)
	light, dark := paletteFor("", false), paletteFor("", true)
	mid := mixPalettes(light, dark, 0.5)
	if mid.editor == light.editor || mid.editor == dark.editor {
		t.Errorf("halfway, the editor is %v", mid.editor)
	}
	if mid.syntax[0] == light.syntax[0] || mid.ansi[1] == light.ansi[1] {
		t.Error("the code's and the terminal's colors do not fade")
	}
	if !mid.dark || mid.inverse != dark.inverse {
		t.Error("what is not a color, or a color left to the theme, is not the new palette's")
	}
	if m := mixPalettes(light, dark, 1); m.editor != dark.editor || m.text != dark.text {
		t.Error("the end of the fade is not the new palette")
	}

	hs := newHarness(t, true)
	a := hs.a
	a.setTheme(themeGrokBot)
	step(hs.tt)
	step(hs.tt)
	if samePalette(a.pal, grokBotDarkPal) || samePalette(a.pal, dark) {
		t.Error("the window does not fade to the new theme")
	}
	hs.settle()
	if a.pal != paletteFor(themeGrokBot, true) || !samePalette(a.pal, grokBotDarkPal) {
		t.Error("the window does not settle on the new theme")
	}
}

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestAyuThemePalette(t *testing.T) {
	withAppearancePlugins(t)
	for _, tc := range []struct {
		dark                           bool
		p                              *palette
		editor, chrome, accent         string
		keyword, function, str, number string
		terminalRed, terminalBlue      string
	}{
		{false, ayuLightPal, "#fcfcfc", "#f8f9fa", "#f29718", "#fa8532", "#eba400", "#86b300", "#a37acc", "#f07171", "#22a4e6"},
		{true, ayuDarkPal, "#10141c", "#0d1017", "#e6b450", "#ff8f40", "#ffb454", "#aad94c", "#d2a6ff", "#e6495a", "#17acf2"},
	} {
		p := paletteFor(themeAyu, tc.dark)
		if !samePalette(p, tc.p) || p.dark != tc.dark {
			t.Fatalf("Ayu did not follow dark=%v", tc.dark)
		}
		if p.editor != ui.Hex(tc.editor) || p.chrome != ui.Hex(tc.chrome) || p.accent != ui.Hex(tc.accent) {
			t.Errorf("dark=%v: Ayu lost its official background or amber accent", tc.dark)
		}
		for kind, hex := range map[highlight.Kind]string{
			highlight.Keyword: tc.keyword, highlight.Function: tc.function,
			highlight.String: tc.str, highlight.Number: tc.number,
		} {
			if p.syntax[kind] != ui.Hex(hex) {
				t.Errorf("dark=%v: syntax %d is %v, want %s", tc.dark, kind, p.syntax[kind], hex)
			}
		}
		for kind, col := range p.syntax {
			if col.A != 255 {
				t.Errorf("dark=%v: syntax %d has alpha %d", tc.dark, kind, col.A)
			}
		}
		if p.ansi[1] != ui.Hex(tc.terminalRed) || p.ansi[4] != ui.Hex(tc.terminalBlue) {
			t.Errorf("dark=%v: Ayu lost its terminal colors", tc.dark)
		}
		if th := termTheme(p); th.Background != p.panel || th.Foreground != p.text || th.Palette != p.ansi {
			t.Errorf("dark=%v: the terminal did not receive Ayu's colors", tc.dark)
		}
	}
	for appearance, want := range map[string]mygo.ThemeSource{"": "", "light": mygo.ThemeLight, "dark": mygo.ThemeDark} {
		if got := themeSource(themeAyu, appearance); got != want {
			t.Errorf("Ayu appearance %q is %q, want %q", appearance, got, want)
		}
	}
	if !reservedThemeID(themeAyu) {
		t.Error("Ayu's legacy theme ID is unreserved")
	}
}

func TestAyuThemeContrast(t *testing.T) {
	withAppearancePlugins(t)
	for name, p := range map[string]*palette{"light": ayuLightPal, "dark": ayuDarkPal} {
		for _, back := range []ui.Color{p.editor, p.chrome, p.panel, p.surface, p.surfaceHover, p.surfacePressed} {
			if back.A != 255 {
				t.Errorf("%s: translucent base %v", name, back)
			}
			for _, ink := range []ui.Color{p.text, p.muted} {
				if ratio := contrast(ink, back); ratio < 4.5 {
					t.Errorf("%s: text %v on %v reads at %.2f:1", name, ink, back, ratio)
				}
			}
		}
		for _, pair := range []struct{ ink, back ui.Color }{
			{p.fillText, p.fill}, {p.fillText, p.fillHover}, {p.fillText, p.fillPressed},
			{p.dangerText, p.dangerFill}, {p.dangerText, p.dangerHover}, {p.dangerText, p.dangerPressed},
		} {
			if ratio := contrast(pair.ink, pair.back); ratio < 4.5 {
				t.Errorf("%s: button %v on %v reads at %.2f:1", name, pair.ink, pair.back, ratio)
			}
		}
		for _, ink := range []ui.Color{p.modifiedFg, p.addedFg, p.removedFg, p.dangerFg} {
			for _, back := range []ui.Color{p.chrome, p.editor, p.panel} {
				if ratio := contrast(ink, back); ratio < 4.5 {
					t.Errorf("%s: status text %v on %v reads at %.2f:1", name, ink, back, ratio)
				}
			}
		}
	}
}

func TestAyuThemeSelection(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, false)
	a := hs.a
	a.run("view.selectTheme")
	hs.settle()
	hs.tt.Type("Ayu")
	hs.settle()
	if len(a.themes.results) != 1 || a.settings.Theme != themeAyu || !samePalette(a.pal, ayuLightPal) {
		t.Fatalf("picker did not preview light Ayu: %d results, theme %q", len(a.themes.results), a.settings.Theme)
	}
	// With a query, the arrow keys move its caret. Clear it to switch appearance.
	hs.key(ui.Cmd, ui.KeyA)
	hs.key(0, ui.KeyBackspace)
	hs.key(0, ui.KeyRight)
	hs.key(0, ui.KeyRight)
	hs.settle()
	if a.settings.Appearance != "dark" || !samePalette(a.pal, ayuDarkPal) {
		t.Fatal("picker did not preview Ayu's dark appearance")
	}
	hs.key(0, ui.KeyEscape)
	hs.settle()
	if a.settings.Theme != "" || a.settings.Appearance != "" || !samePalette(a.pal, paletteFor("", false)) {
		t.Fatal("canceling Ayu did not restore the original theme and appearance")
	}
	a.run("view.selectTheme")
	hs.settle()
	hs.tt.Type("Ayu")
	hs.settle()
	hs.key(0, ui.KeyEnter)
	hs.settle()
	if a.themes.open || a.settings.Theme != themeAyu || !samePalette(a.pal, ayuLightPal) {
		t.Fatal("Return did not keep Ayu")
	}
	a.setAppearance("dark")
	hs.settle()
	if !samePalette(a.pal, ayuDarkPal) || a.termTheme != themeAyu {
		t.Fatal("Ayu did not switch the editor and terminal to dark")
	}
	a.setTheme("")
	a.run("view.theme.ayu")
	hs.settle()
	if !a.commandOn("view.theme.ayu") || !samePalette(a.pal, ayuDarkPal) || a.termTheme != themeAyu {
		t.Fatal("Ayu command did not update the editor and terminal")
	}
	var found bool
	var check func([]*mygo.MenuItem)
	check = func(items []*mygo.MenuItem) {
		for _, item := range items {
			if item.Label == locales.CmdViewThemeAyu() {
				found = true
				if !item.Checked || item.Disabled {
					t.Error("Ayu menu entry is not checked and enabled")
				}
			}
			check(item.Submenu)
		}
	}
	check(a.menu(false, func(string, *mygo.Window) {}).Items())
	if !found {
		t.Error("Ayu is missing from the theme menu")
	}
}

func TestAyuThemeSettingsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := settings.Load(path)
	for key, value := range map[string]string{"theme": `"ayu"`, "appearance": `"dark"`} {
		if err := s.Prefs.Set(key, json.RawMessage(value)); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	s.Flush()
	restored := settings.Load(path)
	if restored.Theme != themeAyu || restored.Appearance != "dark" {
		t.Fatal("reopening settings lost Ayu or its appearance")
	}
}

func TestShotAyu(t *testing.T) {
	shotTheme(t, themeAyu)
}

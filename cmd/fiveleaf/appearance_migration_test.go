package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/plugin"
	"fiveleaf/locales"
)

func TestFiveLeafIsTheBuiltinTheme(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := newHarness(t, false)
	check := func() {
		t.Helper()
		choices := allThemes()
		if len(choices) != 1 || choices[0].id != "" || choices[0].name() != "FiveLeaf" || choices[0].about() != locales.ThemeAboutFiveLeaf() {
			t.Fatalf("expected FiveLeaf to be the sole built-in theme, got %v", choices)
		}
		for _, variant := range []struct {
			dark                 bool
			accent, syntax, ansi string
		}{{false, "#6d5bd0", "#2a5bd7", "#2a5bd7"}, {true, "#8b7cf6", "#8fb4ff", "#8fb4ff"}} {
			want := builtinPalette("", variant.dark)
			if want.accent != ui.Hex(variant.accent) || want.syntax[highlight.Function] != ui.Hex(variant.syntax) || want.ansi[4] != ui.Hex(variant.ansi) {
				t.Fatalf("dark=%t: the built-in workbench, code or terminal lost its FiveLeaf colors", variant.dark)
			}
			for _, id := range []string{"", "fiveleaf", "mayoi", "fiveleaf-themes.fiveleaf", "onedarkpro"} {
				if paletteFor(id, variant.dark) != want {
					t.Fatalf("dark=%t: %q did not use the built-in theme", variant.dark, id)
				}
			}
			inherited := paletteOfThemeFile(&themeFile{Base: "fiveleaf-themes.fiveleaf"}, "custom", variant.dark, variant.dark, nil)
			if inherited == nil || inherited.editor != want.editor || inherited.ansi != want.ansi {
				t.Fatalf("dark=%t: a custom theme could not use the legacy FiveLeaf base", variant.dark)
			}
		}
	}
	check()
	withPlugins(hs, map[string]map[string]string{"fiveleaf-themes": {
		"plugin.json": `{"name":"fiveleaf-themes","enabled":true,"contributes":{"themes":[{"id":"fiveleaf-themes.fiveleaf","label":"Old default","path":"old.json"}]}}`,
		"old.json":    `{"colors":{"editor":"#ff0000"}}`,
	}})
	check()
}

func TestAppearancePluginsRestoreSavedChoices(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs := newHarness(t, false)
	host := withPlugins(hs, nil)
	prefs := hs.a.settings
	prefs.Theme, prefs.IconTheme = "onedarkpro", "material"
	hasTheme := func() bool {
		return slices.ContainsFunc(allThemes(), func(choice themeChoice) bool { return choice.id == prefs.Theme })
	}
	install := func(name string) {
		t.Helper()
		copyAppearancePlugin(t, host, name)
		host.reg.Load()
	}
	if hasTheme() || paletteFor(prefs.Theme, false) != lightPal || iconThemeByID(prefs.IconTheme) != nil {
		t.Fatal("an uninstalled appearance is active")
	}
	install("one-dark-pro")
	if len(allThemes()) != 2 || paletteFor("", false) != lightPal || paletteFor("", true) != darkPal {
		t.Fatal("installing One Dark Pro should add just one choice alongside FiveLeaf")
	}
	if !hasTheme() || iconThemeByID(prefs.IconTheme) != nil {
		t.Fatal("installing color themes also changed the file icon choices or omitted a theme")
	}
	if p := paletteFor(prefs.Theme, false); p != paletteFor("fiveleaf-themes.onedarkpro", false) || p != paletteFor("one-dark-pro.theme", false) || !p.dark || p.editor != ui.Hex("#282c34") {
		t.Fatal("the saved One Dark Pro ID and the manifest ID must select the same fixed dark palette")
	}
	install("ayu")
	if light, dark := paletteFor("ayu.theme", false), paletteFor("ayu", true); light.dark || !dark.dark || light.editor == dark.editor || len(allThemes()) != 3 {
		t.Fatal("Ayu did not select its light and dark assets")
	}
	if light, dark := hs.a.termThemes(); light.Background != dark.Background || light.Palette != paletteFor(prefs.Theme, false).ansi {
		t.Fatal("the terminal did not take the fixed dark theme's ANSI palette")
	}
	install("material-icons")
	if pack := iconThemeByID(prefs.IconTheme); pack == nil || pack != iconThemeByID("fiveleaf-icons.material") || pack != iconThemeByID("material-icons.icons") || len(allIconThemes()) != 2 {
		t.Fatal("the saved Material ID did not resolve to the installed icon pack")
	}
	prefs.Prefs.Plugins = map[string]json.RawMessage{"one-dark-pro": json.RawMessage(`{"enabled":false}`)}
	host.syncPrefs()
	host.Changed()
	if hasTheme() || paletteFor(prefs.Theme, false) != lightPal || pluginPalette("ayu", false) == nil || iconThemeByID(prefs.IconTheme) == nil {
		t.Fatal("disabling colors must restore basic colors and keep the selected icon pack")
	}
	delete(prefs.Prefs.Plugins, "one-dark-pro")
	host.syncPrefs()
	host.reg.SetSafe(true)
	host.Changed()
	if hasTheme() || iconThemeByID(prefs.IconTheme) != nil || themeMascotOf("matrix") != nil {
		t.Fatal("safe mode kept an external appearance")
	}
	host.reg.SetSafe(false)
	host.reg.Load()
	if err := plugin.Remove(host.reg.Dirs.Plugins, "material-icons"); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	if !hasTheme() || iconThemeByID(prefs.IconTheme) != nil {
		t.Fatal("removing the icon plugin affected the color theme or left stale icons")
	}
	install("material-icons")
	if prefs.Theme != "onedarkpro" || prefs.IconTheme != "material" || !hasTheme() || iconThemeByID(prefs.IconTheme) == nil {
		t.Fatal("reinstallation did not restore both original settings")
	}
}

func TestAppearanceBrokenVariantMatchesPickerFallback(t *testing.T) {
	withAppearancePlugins(t)
	name := filepath.Join(plugins.reg.Dirs.Plugins, "ayu", "themes", "ayu-dark.json")
	if err := os.WriteFile(name, []byte(`{"colors":`), 0o644); err != nil {
		t.Fatal(err)
	}
	themeFileChanged()
	if slices.ContainsFunc(allThemes(), func(choice themeChoice) bool { return choice.id == "ayu" }) {
		t.Fatal("picker listed Ayu with a broken dark variant")
	}
	for _, dark := range []bool{false, true} {
		if paletteFor("ayu", dark) != paletteFor("", dark) {
			t.Errorf("dark=%t: editor and picker disagree on default fallback", dark)
		}
	}
}

func TestAppearanceIndividualPluginsOverrideOldBundles(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	host := withPlugins(newHarness(t, false), map[string]map[string]string{
		"fiveleaf-themes": {
			"plugin.json": `{"name":"fiveleaf-themes","enabled":true,"contributes":{"themes":[{"id":"fiveleaf-themes.onedarkpro","label":"Legacy color","path":"old.json"}]}}`,
			"old.json":    `{"colors":{"editor":"#123456"}}`,
		},
		"fiveleaf-icons": {
			"plugin.json": `{"name":"fiveleaf-icons","enabled":true,"contributes":{"iconThemes":[{"id":"fiveleaf-icons.ayu","label":"Legacy icons","path":"old.json"}]}}`,
			"old.json":    `{"file":"legacy","folder":"legacy","icons":{"legacy":"old.svg"}}`,
			"old.svg":     testIconSVG,
		},
	})
	check := func(editor, colorLabel, iconLabel, goIcon string) {
		t.Helper()
		colors, icons := allThemes(), allIconThemes()
		if len(colors) != 2 || colors[1].id != "onedarkpro" || colors[1].name() != colorLabel || len(icons) != 2 || icons[1].name != iconLabel {
			t.Fatalf("unexpected picker choices: colors=%v icons=%v", colors, icons)
		}
		if got := paletteFor("fiveleaf-themes.onedarkpro", false); got.editor != ui.Hex(editor) || got != paletteFor("one-dark-pro.theme", false) {
			t.Fatalf("One Dark Pro aliases did not select %s", editor)
		}
		if pack := iconThemeByID("ayu"); pack == nil || pack.fileID("main.go") != goIcon || pack != iconThemeByID("ayu-icons.icons") {
			t.Fatalf("Ayu icon aliases did not select %s", goIcon)
		}
	}
	check("#123456", "Legacy color", "Legacy icons", "legacy")
	for _, name := range []string{"one-dark-pro", "ayu-icons"} {
		copyAppearancePlugin(t, host, name)
	}
	host.reg.Load()
	check("#282c34", "One Dark Pro", "Ayu", "go")
	for _, name := range []string{"one-dark-pro", "ayu-icons"} {
		if err := plugin.Remove(host.reg.Dirs.Plugins, name); err != nil {
			t.Fatal(err)
		}
	}
	host.reg.Load()
	check("#123456", "Legacy color", "Legacy icons", "legacy")
}

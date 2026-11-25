package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestPrefsIconThemeSelection(t *testing.T) {
	withAppearancePlugins(t)
	ph := newPrefsHarness(t)
	ph.w.query = "iconTheme"
	ph.frames()
	if !ph.pt.HasText(prefTitle("iconTheme")) {
		t.Fatal("file icon setting is missing from Appearance search")
	}
	if ph.pt.HasText("Catppuccin Frappé") || ph.pt.HasText("Catppuccin Macchiato") {
		t.Fatal("the default settings include similar Catppuccin variants")
	}
	ph.w.set("iconTheme", json.RawMessage(`"catppuccin-latte"`))
	ph.frames()
	if ph.m.s.IconTheme != "catppuccin-latte" || ph.pt.HasText(locales.PrefIconThemeUnavailable("catppuccin-latte")) {
		t.Fatal("the settings did not recognize the saved Catppuccin Latte alias")
	}
	theme, appearance := ph.m.s.Theme, ph.m.s.Appearance
	if err := ph.pt.Click("Catppuccin Mocha"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if ph.m.s.IconTheme != "fiveleaf-icons.catppuccin-mocha" || ph.a.settings.IconTheme != "fiveleaf-icons.catppuccin-mocha" {
		t.Fatal("choosing the preview did not update the settings and open window")
	}
	got, _ := ph.a.fileIcon("main.go")
	want, _ := fileIconForTheme(ph.a.pal, "catppuccin-mocha", "main.go")
	plain, _ := fileIcon(ph.a.pal, "main.go")
	if got != want || got == plain {
		t.Fatal("the window is not using the chosen icon pack")
	}
	ph.pt.Key(0, ui.KeyRight)
	ph.frames()
	if ph.m.s.IconTheme == "fiveleaf-icons.catppuccin-mocha" {
		t.Fatal("the keyboard cannot change icon themes")
	}
	if ph.m.s.Theme != theme || ph.m.s.Appearance != appearance {
		t.Fatal("switching icons changed the color theme or appearance")
	}
	ph.w.reset("iconTheme", nil)
	ph.frames()
	if ph.m.s.IconTheme != "" {
		t.Fatal("reset did not restore FiveLeaf icons")
	}
}

func TestPrefsUnavailableIconTheme(t *testing.T) {
	withAppearancePlugins(t)
	ph := newPrefsHarness(t)
	ph.w.query = "iconTheme"
	ph.w.set("iconTheme", json.RawMessage(`"missing.icons"`))
	ph.frames()
	if !ph.pt.HasText(locales.PrefIconThemeUnavailable("missing.icons")) {
		t.Fatal("a missing pack has no explanation")
	}
	if ph.m.s.IconTheme != "missing.icons" {
		t.Fatal("rendering removed the saved icon choice")
	}
	if err := ph.pt.Click("FiveLeaf"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if ph.m.s.IconTheme != "fiveleaf-icons.fiveleaf" {
		t.Fatal("choosing FiveLeaf did not replace the unavailable choice with the installed default")
	}
}

func TestPrefsPluginIconTheme(t *testing.T) {
	ph := newPrefsHarness(t)
	files := map[string]string{
		"plugin.json": `{"name":"icon-test","contributes":{"iconThemes":[{"id":"icon-test.bright","label":"Bright icons","path":"theme.json"}]}}`,
	}
	for name, file := range iconThemeFixture() {
		files[name] = string(file.Data)
	}
	withPlugins(ph.harness, map[string]map[string]string{"icon-test": files})
	ph.w.query = "iconTheme"
	ph.frames()
	if err := ph.pt.Click("Bright icons"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if ph.m.s.IconTheme != "icon-test.bright" {
		t.Fatal("the installed icon theme cannot be selected in settings")
	}
	icon, _ := ph.a.fileIcon("unknown")
	if !coloredThemeIcon(icon) {
		t.Fatal("the installed icon theme is not used by the open window")
	}
	ph.w.query = ""
	ph.w.section = prefsPlugins
	ph.w.openDetail("icon-test")
	ph.frames()
	if !ph.pt.HasText(locales.PluginsIconThemes()) || !ph.pt.HasText("Bright icons") {
		t.Fatal("plugin details do not describe the installed icon theme")
	}
}

func TestShotIconThemes(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	withAppearancePlugins(t)
	for _, dark := range []bool{true, false} {
		ph := newPrefsHarness(t)
		ph.pt.SetDark(dark)
		ph.w.section = settings.CatAppearance
		ph.w.query = "iconTheme"
		ph.w.set("iconTheme", json.RawMessage(`"catppuccin-mocha"`))
		ph.frames()
		name := "icon-themes-light"
		if dark {
			name = "icon-themes-dark"
		}
		ph.shot(name)
	}
}

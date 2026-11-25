package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
)

func expectIconThemePreview(t *testing.T, a *app, id string) {
	t.Helper()
	choice, ok := a.iconPicker.choice()
	if !a.iconPicker.open || !ok || choice.id != id || a.iconPicker.preview != id {
		t.Fatalf("picker preview: open=%v selected=%q preview=%q, want %q", a.iconPicker.open, choice.id, a.iconPicker.preview, id)
	}
	file, _ := a.fileIcon("main.go")
	wantFile, _ := fileIconForTheme(a.pal, id, "main.go")
	folder, _ := a.folderIcon("src", true)
	wantFolder, _ := folderIconForTheme(a.pal, id, "src", true)
	if file != wantFile || folder != wantFolder {
		t.Fatal("file and folder lookups did not follow the preview")
	}
}

func TestIconThemePickerStartsOnCurrentAndPreviews(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	a := hs.a
	a.settings.IconTheme = "material"
	a.run("view.selectIconTheme")
	hs.settle()
	expectIconThemePreview(t, a, "fiveleaf-icons.material")
	if a.iconPicker.sel != 3 {
		t.Fatalf("selected row = %d, want saved Material row 3", a.iconPicker.sel)
	}
	if hs.tt.HasText("Catppuccin Frappé") || hs.tt.HasText("Catppuccin Macchiato") {
		t.Fatal("the default picker includes similar Catppuccin variants")
	}
	hs.key(0, ui.KeyUp)
	expectIconThemePreview(t, a, "fiveleaf-icons.catppuccin-mocha")
	hs.shot("icon-theme-picker")
	a.openIconThemePicker()
	expectIconThemePreview(t, a, "fiveleaf-icons.catppuccin-mocha")
	if a.settings.IconTheme != "material" {
		t.Fatal("preview changed the shared settings")
	}
	other := &app{settings: a.settings, pal: a.pal}
	got, _ := other.fileIcon("main.go")
	want, _ := fileIconForTheme(a.pal, "material", "main.go")
	if got != want {
		t.Fatal("preview leaked into another window")
	}
	for range 2 {
		hs.key(0, ui.KeyUp)
	}
	expectIconThemePreview(t, a, "fiveleaf-icons.fiveleaf")
	hs.key(0, ui.KeyEscape)
	got, _ = a.fileIcon("main.go")
	if a.iconPicker.open || a.settings.IconTheme != "material" || got != want {
		t.Fatal("Escape did not discard the preview")
	}
}

func TestIconThemePickerOneDarkProSelectionRemainsVisible(t *testing.T) {
	withAppearancePlugins(t)
	hs := themeHarness(t, themeOneDarkPro, true)
	a := hs.a
	a.settings.IconTheme = "catppuccin-latte"
	a.run("view.selectIconTheme")
	hs.settle()
	for i, choice := range []iconThemeChoice{
		{"catppuccin-latte", "Catppuccin Latte"},
		{"catppuccin-mocha", "Catppuccin Mocha"},
	} {
		if i > 0 {
			hs.key(0, ui.KeyDown)
		}
		expectIconThemePreview(t, a, "fiveleaf-icons."+choice.id)
		hs.shot("icon-theme-picker-one-dark-pro-" + choice.id)
		row, ok := hs.tt.Find(choice.name)
		if !ok || row.W < 400 {
			t.Fatalf("selected row %q has no visible bounds: %v", choice.name, row)
		}
		img := hs.tt.Image()
		titlePixels, coloredPixels := 0, 0
		for y := int(row.Y + 4); y < int(row.Y+row.H-4); y++ {
			for x := int(row.X + 8); x < int(row.X+row.W*0.6); x++ {
				pixel := img.RGBAAt(x, y)
				if pixel.R > 105 && pixel.G > 110 && pixel.B > 115 {
					titlePixels++
				}
			}
			// Leave out the selection checkmark: only the actual SVG samples count.
			for x := int(row.X + row.W*0.7); x < int(row.X+row.W-32); x++ {
				pixel := img.RGBAAt(x, y)
				high, low := max(pixel.R, pixel.G, pixel.B), min(pixel.R, pixel.G, pixel.B)
				if high > 120 && high-low > 35 {
					coloredPixels++
				}
			}
		}
		if titlePixels < 60 || coloredPixels < 30 {
			t.Errorf("%s selection obscures its content: title pixels=%d, colorful icon pixels=%d", choice.name, titlePixels, coloredPixels)
		}
	}
}

func TestIconThemePickerPreviewCannotBeFlushed(t *testing.T) {
	withAppearancePlugins(t)
	ph := newPrefsHarness(t)
	a := ph.a
	host := &statusControlsHost{fakeHost: ph.h}
	a.host = host
	a.settings.IconTheme = "material"
	a.settings.Changed()
	a.settings.Flush()
	a.run("view.selectIconTheme")
	ph.settle()
	ph.tt.Type("Seti")
	ph.settle()
	expectIconThemePreview(t, a, "fiveleaf-icons.seti")
	// Session and other preference changes may flush while the picker is open.
	a.settings.FontSize++
	a.settings.Changed()
	a.settings.Flush()
	readSaved := func() settings.Prefs {
		t.Helper()
		data, err := os.ReadFile(a.settings.Path())
		if err != nil {
			t.Fatal(err)
		}
		var prefs settings.Prefs
		if err := json.Unmarshal(data, &prefs); err != nil {
			t.Fatal(err)
		}
		return prefs
	}
	if got := readSaved().IconTheme; got != "material" {
		t.Fatalf("an unrelated flush persisted the preview %q", got)
	}
	generation, redraws := a.settings.Gen(), host.redraws
	ph.key(0, ui.KeyEnter)
	if a.iconPicker.open || a.settings.IconTheme != "fiveleaf-icons.seti" || a.settings.Gen() != generation+1 || host.redraws != redraws+1 {
		t.Fatal("Enter did not save the chosen theme once and redraw the windows")
	}
	a.settings.Flush()
	if got := readSaved().IconTheme; got != "fiveleaf-icons.seti" {
		t.Fatalf("confirmed theme was not persisted: %q", got)
	}
}

func TestIconThemePickerClickAndOutside(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	a := hs.a
	a.run("view.selectIconTheme")
	hs.settle()
	if err := hs.tt.Click("Material Icon Theme"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.iconPicker.open || a.settings.IconTheme != "fiveleaf-icons.material" {
		t.Fatal("clicking a theme did not keep it")
	}
	a.run("view.selectIconTheme")
	hs.settle()
	hs.key(0, ui.KeyDown)
	expectIconThemePreview(t, a, "fiveleaf-icons.seti")
	hs.tt.Press(10, 700)
	hs.tt.Release(10, 700)
	hs.settle()
	if a.iconPicker.open || a.settings.IconTheme != "fiveleaf-icons.material" {
		t.Fatal("clicking outside did not cancel the preview")
	}
}

func TestIconThemePickerNoMatchesCannotConfirm(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	a := hs.a
	a.run("view.selectIconTheme")
	hs.settle()
	hs.key(0, ui.KeyDown)
	expectIconThemePreview(t, a, "fiveleaf-icons.catppuccin-latte")
	hs.tt.Type("no-such-icon-pack-123456")
	hs.settle()
	if len(a.iconPicker.results) != 0 {
		t.Fatal("impossible query returned matches")
	}
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyUp)
	hs.key(0, ui.KeyEnter)
	if !a.iconPicker.open || a.settings.IconTheme != "" {
		t.Fatal("Enter without matches committed the stale preview or closed the picker")
	}
	hs.key(ui.Cmd, ui.KeyA)
	hs.key(0, ui.KeyBackspace)
	expectIconThemePreview(t, a, "fiveleaf-icons.catppuccin-latte")
	hs.key(0, ui.KeyEnter)
	if a.iconPicker.open || a.settings.IconTheme != "fiveleaf-icons.catppuccin-latte" {
		t.Fatal("clearing the query did not make the selected theme confirmable")
	}
}

func TestIconThemePickerCancelPreservesSavedSettings(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	a := hs.a
	a.settings.IconTheme = "missing.plugin-theme"
	a.run("view.selectIconTheme")
	hs.settle()
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyEscape)
	if a.iconPicker.open || a.settings.IconTheme != "missing.plugin-theme" {
		t.Fatal("cancel lost an unavailable saved theme")
	}
	a.run("view.selectIconTheme")
	hs.settle()
	hs.tt.Type("Seti")
	hs.settle()
	expectIconThemePreview(t, a, "fiveleaf-icons.seti")
	a.settings.IconTheme = "material"
	a.settings.Changed()
	hs.key(0, ui.KeyEscape)
	if a.settings.IconTheme != "material" {
		t.Fatal("cancel overwrote a preference changed in another window")
	}
}

func TestIconThemePickerCommandReplacementCancelsPreview(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	a := hs.a
	a.settings.IconTheme = "material"
	a.run("view.selectIconTheme")
	hs.settle()
	hs.key(0, ui.KeyDown)
	expectIconThemePreview(t, a, "fiveleaf-icons.seti")
	a.run("go.commands")
	hs.settle()
	if a.iconPicker.open || !a.quick.open || a.quick.query != ">" || a.settings.IconTheme != "material" {
		t.Fatal("the command palette did not replace the icon picker without saving its preview")
	}
	got, _ := a.fileIcon("main.go")
	want, _ := fileIconForTheme(a.pal, "material", "main.go")
	if got != want {
		t.Fatal("the replaced picker left its icons visible")
	}
}

func TestIconThemePickerTracksPluginIDs(t *testing.T) {
	hs, h := iconThemePlugin(t)
	a := hs.a
	a.settings.IconTheme = "icon-test.bright"
	a.run("view.selectIconTheme")
	hs.settle()
	expectIconThemePreview(t, a, "icon-test.bright")
	before := a.iconPicker.sel
	manifest := filepath.Join(h.reg.Plugin("icon-test").Dir, "plugin.json")
	if err := os.WriteFile(manifest, []byte(`{"name":"icon-test","contributes":{"iconThemes":[
{"id":"icon-test.alpha","label":"Alpha icons","path":"theme.json"},
{"id":"icon-test.bright","label":"Bright icons","path":"theme.json"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h.reg.Reload("icon-test")
	hs.settle()
	expectIconThemePreview(t, a, "icon-test.bright")
	if a.iconPicker.sel != before+1 {
		t.Fatal("adding a theme before the selection did not retain its stable ID")
	}
	hs.tt.Type("Bright")
	hs.settle()
	expectIconThemePreview(t, a, "icon-test.bright")
	w := &prefsWindow{m: &fiveleaf{s: a.settings}}
	w.setPluginSetting("icon-test", "enabled", false, true)
	hs.settle()
	if len(a.iconPicker.results) != 0 {
		t.Fatal("disabled plugin theme stayed in the search results")
	}
	hs.key(0, ui.KeyEnter)
	if !a.iconPicker.open || a.settings.IconTheme != "icon-test.bright" {
		t.Fatal("empty plugin results changed the saved theme")
	}
	w.setPluginSetting("icon-test", "enabled", true, true)
	hs.settle()
	expectIconThemePreview(t, a, "icon-test.bright")
	hs.key(ui.Cmd, ui.KeyA)
	hs.key(0, ui.KeyBackspace)
	w.setPluginSetting("icon-test", "enabled", false, true)
	hs.settle()
	expectIconThemePreview(t, a, "")
	hs.key(0, ui.KeyEscape)
	if a.settings.IconTheme != "icon-test.bright" {
		t.Fatal("losing the selected plugin changed the saved theme on cancel")
	}
}

func TestIconThemePickerRejectsRemovedChoiceBeforeCommit(t *testing.T) {
	hs, _ := iconThemePlugin(t)
	a := hs.a
	a.run("view.selectIconTheme")
	hs.settle()
	hs.tt.Type("Bright")
	hs.settle()
	expectIconThemePreview(t, a, "icon-test.bright")
	w := &prefsWindow{m: &fiveleaf{s: a.settings}}
	w.setPluginSetting("icon-test", "enabled", false, true)
	// Confirm before the next frame has rebuilt the plugin list.
	a.closeIconThemePicker(true)
	if !a.iconPicker.open || a.settings.IconTheme != "" {
		t.Fatal("a removed choice was committed before the list refreshed")
	}
	a.keepIconThemeChoice("icon-test.bright")
	if !a.iconPicker.open || a.settings.IconTheme != "" {
		t.Fatal("a delayed row click committed a different theme")
	}
	hs.settle()
	hs.key(0, ui.KeyEscape)
}

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func pickerMarkerPixels(img *image.RGBA, r ui.Rect, backgrounds ...ui.Color) int {
	n := 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			p := img.RGBAAt(x, y)
			visible := true
			for _, background := range backgrounds {
				visible = visible && contrast(ui.RGB(p.R, p.G, p.B), background) >= 3
			}
			if visible {
				n++
			}
		}
	}
	return n
}

func assertPickerSelection(t *testing.T, hs *harness, label string, selected bool) ui.Rect {
	t.Helper()
	r, ok := hs.tt.Find(label)
	if !ok || r.W < 400 {
		t.Fatalf("picker row %q has no bounds: %v", label, r)
	}
	img := hs.tt.Image()
	if selected && (r.Y < 0 || r.Y+r.H > float32(img.Bounds().Max.Y)-8) {
		t.Errorf("selected row %q is outside the window: %v in %v", label, r, img.Bounds())
	}
	background := img.RGBAAt(int(r.X+7), int(r.Y+r.H/2))
	outside := img.RGBAAt(int(r.X-2), int(r.Y+r.H/2))
	rowColor, outsideColor := ui.RGB(background.R, background.G, background.B), ui.RGB(outside.R, outside.G, outside.B)
	mark := pickerMarkerPixels(img, ui.Rect{X: r.X + 1, Y: r.Y + r.H/2 - 5, W: 5, H: 10}, rowColor, outsideColor)
	check := pickerMarkerPixels(img, ui.Rect{X: r.X + r.W - 25, Y: r.Y + r.H/2 - 8, W: 17, H: 16}, rowColor)
	if selected {
		if mark < 10 || check < 10 {
			t.Errorf("selected row %q lacks a visible marker/check: mark=%d check=%d", label, mark, check)
		}
	} else if mark > 3 || check > 5 {
		t.Errorf("previous row %q still has a selection marker: mark=%d check=%d", label, mark, check)
	}
	return r
}

func TestPickerSelectionMarksCurrentIcon(t *testing.T) {
	withAppearancePlugins(t)
	for _, tc := range []struct {
		name, theme string
		dark        bool
	}{
		{"light", "", false},
		{"dark", "", true},
		{"one-dark-pro", themeOneDarkPro, true},
		{"ayu-light", themeAyu, false},
		{"ayu-dark", themeAyu, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs := themeHarness(t, tc.theme, tc.dark)
			hs.a.openFile(hs.path("main.go"), false)
			hs.a.settings.IconTheme = "catppuccin-latte"
			hs.a.run("view.selectIconTheme")
			hs.settle()
			before := assertPickerSelection(t, hs, "Catppuccin Latte", true)
			// Do not advance animation time: the marker must follow the key immediately.
			hs.tt.Key(0, ui.KeyDown)
			assertPickerSelection(t, hs, "Catppuccin Mocha", true)
			if after := assertPickerSelection(t, hs, "Catppuccin Latte", false); after != before {
				t.Errorf("selection moved the previous row: %v -> %v", before, after)
			}
			hs.shot("icon-selection-" + tc.name)
			hs.tt.Key(0, ui.KeyUp)
			assertPickerSelection(t, hs, "Catppuccin Latte", true)
			assertPickerSelection(t, hs, "Catppuccin Mocha", false)
		})
	}
}

func TestPickerSelectionMarksCurrentTheme(t *testing.T) {
	withAppearancePlugins(t)
	for _, tc := range []struct {
		name, theme string
		dark        bool
	}{
		{"light", "", false},
		{"dark", "", true},
		{"ayu-light", themeAyu, false},
		{"ayu-dark", themeAyu, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs := themeHarness(t, tc.theme, tc.dark)
			hs.a.openFile(hs.path("main.go"), false)
			hs.a.run("view.selectTheme")
			hs.settle()
			tp := &hs.a.themes
			first := tp.themes[tp.results[tp.sel].Index].name()
			next, move, back := tp.sel+1, ui.KeyDown, ui.KeyUp
			if next == len(tp.results) {
				next, move, back = tp.sel-1, ui.KeyUp, ui.KeyDown
			}
			second := tp.themes[tp.results[next].Index].name()
			before := assertPickerSelection(t, hs, first, true)
			hs.tt.Key(0, move)
			assertPickerSelection(t, hs, second, true)
			if after := assertPickerSelection(t, hs, first, false); after != before {
				t.Errorf("selection moved the previous row: %v -> %v", before, after)
			}
			hs.tt.Key(0, back)
			assertPickerSelection(t, hs, first, true)
			assertPickerSelection(t, hs, second, false)
			hs.shot("theme-selection-" + tc.name)
		})
	}
}

func TestPickerSelectionStaysVisible(t *testing.T) {
	t.Run("plugin icons", func(t *testing.T) {
		t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
		hs, host := iconThemePlugin(t)
		withAppearancePlugins(t)
		hs.a.openFile(hs.path("main.go"), false)
		choices := make([]map[string]string, 20)
		for i := range choices {
			choices[i] = map[string]string{"id": fmt.Sprintf("icon-test.pack-%02d", i), "label": fmt.Sprintf("Pack %02d", i), "path": "theme.json"}
		}
		data, err := json.Marshal(map[string]any{"name": "icon-test", "contributes": map[string]any{"iconThemes": choices}})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(host.reg.Plugin("icon-test").Dir, "plugin.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		host.reg.Reload("icon-test")
		hs.tt.SetSize(800, 480)
		hs.a.run("view.selectIconTheme")
		hs.settle()
		for i := 1; i < len(hs.a.iconPicker.results); i++ {
			hs.tt.Key(0, ui.KeyDown)
		}
		expectIconThemePreview(t, hs.a, "icon-test.pack-19")
		assertPickerSelection(t, hs, "Pack 19", true)
		hs.shot("icon-selection-last-plugin")
		for hs.a.iconPicker.sel > 0 {
			hs.tt.Key(0, ui.KeyUp)
		}
		assertPickerSelection(t, hs, "FiveLeaf", true)
	})
	t.Run("themes", func(t *testing.T) {
		withAppearancePlugins(t)
		hs := newHarness(t, true)
		hs.a.openFile(hs.path("main.go"), false)
		hs.tt.SetSize(800, 480)
		hs.a.run("view.selectTheme")
		hs.settle()
		for i := 1; i < len(hs.a.themes.results); i++ {
			hs.tt.Key(0, ui.KeyDown)
		}
		last := hs.a.themes.themes[hs.a.themes.results[hs.a.themes.sel].Index]
		assertPickerSelection(t, hs, last.name(), true)
		hs.shot("theme-selection-last")
		for hs.a.themes.sel > 0 {
			hs.tt.Key(0, ui.KeyUp)
		}
		assertPickerSelection(t, hs, "FiveLeaf", true)
	})
}

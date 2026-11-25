package main

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestIconThemeFiveLeafGlyphsAndTints(t *testing.T) {
	withAppearancePlugins(t)
	for _, paletteCase := range []struct {
		name string
		pal  *palette
	}{{"light", lightPal}, {"dark", darkPal}, {"mono", matrixPal}} {
		t.Run(paletteCase.name, func(t *testing.T) {
			pal := paletteCase.pal
			for _, sample := range []struct {
				name string
				icon *ui.SVG
				tint ui.Color
			}{
				{`C:\src\GO.SUM`, iconFileCode, langColor(pal, "#00add8", "#00809f")},
				{"module.d.ts", iconFileCode, langColor(pal, "#4d8fdf", "#2f6fc0")},
				{"dir/.settings.json", iconFileJSON, langColor(pal, "#cbcb41", "#9a8a00")},
				{"yarn.lock", iconFileJSON, langColor(pal, "#cbcb41", "#9a8a00")},
				{"LICENSE.MD", iconFileText, langColor(pal, "#d4af37", "#a07d10")},
				{"Dockerfile", iconSettings, langColor(pal, "#e37933", "#b5571a")},
				{"dir/.envrc", iconLock, langColor(pal, "#e8c482", "#a86a0c")},
				{"dir/.ENV.production.json", iconLock, langColor(pal, "#e8c482", "#a86a0c")},
				{"dir/.go", iconSettings, pal.muted}, {"Cargo.lock", iconLock, pal.faint},
				{"script.sh", iconTerminal, langColor(pal, "#89e051", "#4f9a1f")},
				{"cover.png", iconImage, langColor(pal, "#a6c56a", "#5f8a26")},
				{"file.unknown", iconFile, pal.muted},
			} {
				icon, tint := fileIconForTheme(pal, "", sample.name)
				checkIconThemeGlyph(t, pal, sample.name, icon, tint, sample.icon, sample.tint)
			}
			for open, want := range map[bool]*ui.SVG{false: iconFolder, true: iconFolderOpen} {
				icon, tint := folderIconForTheme(pal, "", "src", open)
				checkIconThemeGlyph(t, pal, "src", icon, tint, want, pal.accent)
			}
		})
	}
}

func checkIconThemeGlyph(t *testing.T, pal *palette, name string, icon *ui.SVG, tint ui.Color, wantIcon *ui.SVG, wantTint ui.Color) {
	t.Helper()
	if icon == nil || icon == wantIcon || coloredThemeIcon(icon) || tint != wantTint {
		t.Fatalf("%s: installed glyph=%p, original=%p, tint=%v, want %v", name, icon, wantIcon, tint, wantTint)
	}
	const size = 24
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(0).Children(func() {
			iconThemeView(c, pal, icon, tint, size)
			ui.Icon(c, wantIcon).FontSize(size).TextColor(wantTint).Shrink(0)
			ui.Box(c).Size(size, size).Draw(func(pt *ui.Painter, rect ui.Rect) { paintThemeIcon(pt, pal, icon, tint, rect) })
			ui.Box(c).Size(size, size).Draw(func(pt *ui.Painter, rect ui.Rect) { pt.Icon(wantIcon, rect, wantTint) })
		})
	}, 4*size, size)
	tester.Frame()
	image := tester.Image()
	for row := range size {
		for _, start := range []int{0, 2 * size} {
			offset := image.PixOffset(start, row)
			if !bytes.Equal(image.Pix[offset:offset+4*size], image.Pix[offset+4*size:offset+8*size]) {
				t.Fatalf("%s: FiveLeaf glyph pixels differ from the original at row %d (column %d)", name, row, start)
			}
		}
	}
}

func TestIconThemeCurrentColor(t *testing.T) {
	hs, host := iconThemePlugin(t)
	data := strings.Replace(testIconSVG, `fill="#00ff00"`, `fill="currentColor"`, 1)
	if err := os.WriteFile(filepath.Join(host.reg.Plugin("icon-test").Dir, "icons", "file.svg"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	host.reg.Reload("icon-test")
	for _, dark := range []bool{false, true} {
		icon, tint := fileIconForTheme(hs.a.pal, "icon-test.bright", "unknown")
		var text ui.Color
		tt := ui.NewTester(func(c *ui.Context) {
			text = c.Theme().Text
			ui.Row(c).Gap(0).Children(func() {
				iconThemeView(c, hs.a.pal, icon, tint, 32)
				ui.Box(c).Size(32, 32).Draw(func(pt *ui.Painter, r ui.Rect) {
					paintThemeIcon(pt, hs.a.pal, icon, tint, r)
				})
			})
		}, 64, 32)
		tt.SetDark(dark)
		tt.Frame()
		img := tt.Image()
		for _, x := range []int{0, 32} {
			if got := img.RGBAAt(x+8, 16); got != (color.RGBA{R: 255, A: 255}) {
				t.Errorf("dark=%v x=%d: explicit fill changed to %v", dark, x, got)
			}
			if got, want := img.RGBAAt(x+24, 16), (color.RGBA{R: text.R, G: text.G, B: text.B, A: text.A}); got != want {
				t.Errorf("dark=%v x=%d: currentColor = %v, want %v", dark, x, got, want)
			}
		}
		for y := range 32 {
			for x := range 32 {
				if img.RGBAAt(x, y) != img.RGBAAt(x+32, y) {
					t.Fatalf("dark=%v: element and painter disagree at (%d, %d)", dark, x, y)
				}
			}
		}
	}
}

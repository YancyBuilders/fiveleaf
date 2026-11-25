package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"

	"github.com/egoist/mygo"
)

// A window is opaque, with no material, at full opacity; below it, the
// desktop shows through frosted glass.
func TestWindowOpacityMaterial(t *testing.T) {
	for _, tc := range []struct {
		opacity float64
		want    mygo.Vibrancy
	}{
		{1, mygo.VibrancyNone},
		{0.8, mygo.VibrancyGlass},
		{0.35, mygo.VibrancyGlass},
	} {
		if runtime.GOOS == "windows" {
			tc.want = mygo.VibrancyGlass
		}
		if got := windowVibrancy(tc.opacity); got != tc.want {
			t.Errorf("at %v the material is %q, want %q", tc.opacity, got, tc.want)
		}
	}
}

// The settings window sets the opacity, and the app keeps it: the main
// window is made with the material the opacity asks for.
func TestSettingsSetsWindowOpacity(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.w.set("windowOpacity", json.RawMessage(`0.6`))
	ph.frames()
	if got := ph.m.s.WindowOpacity; got != 0.6 {
		t.Fatalf("the window sets opacity %v", got)
	}
	if f, ok := settings.FieldOf("windowOpacity"); !ok || f.Category != settings.CatAppearance {
		t.Errorf("windowOpacity is not a setting of Appearance: %+v", f)
	}
}

func TestSettingsWindowOpacitySlider(t *testing.T) {
	harness := newPrefsHarness(t)
	harness.w.query = "windowOpacity"
	harness.frames()
	value, found := harness.pt.Find("100%")
	if !found {
		t.Fatal("background opacity value is missing")
	}
	sliderRight, sliderY := value.X-10, value.Y+value.H/2
	harness.pt.Press(sliderRight-168, sliderY)
	harness.pt.Release(sliderRight-168, sliderY)
	harness.frames()
	if got := harness.m.s.WindowOpacity; got != settings.MinWindowOpacity {
		t.Fatalf("slider minimum sets %v, want %v", got, settings.MinWindowOpacity)
	}
	harness.pt.Press(sliderRight-160, sliderY)
	step(harness.pt)
	harness.pt.Move(sliderRight+20, sliderY)
	step(harness.pt)
	harness.pt.Release(sliderRight+20, sliderY)
	harness.frames()
	if got := harness.m.s.WindowOpacity; got != 1 {
		t.Fatalf("slider maximum sets %v, want 1", got)
	}
}

func TestWindowBackgroundOpacity(t *testing.T) {
	withAppearancePlugins(t)
	for _, appearance := range []string{"light", "dark"} {
		for _, theme := range []string{"", themeQoder} {
			t.Run(appearance+"/"+theme, func(t *testing.T) {
				harness := newHarness(t, appearance == "dark")
				harness.a.settings.Theme = theme
				harness.a.settings.WindowOpacity = 0.6
				harness.tt.SetVibrancy(true)
				harness.a.openFile(harness.path("main.go"), true)
				harness.settle()
				// A late file read can finish on the test clock's last frame.
				// Let the editor remove its one-frame scroll-restoration cover
				// before measuring the background and the text underneath it.
				harness.until("the editor's restored scroll position", func() bool {
					d := harness.a.activeEditor()
					return d != nil && d.coverFrame != harness.a.frame
				})
				rendered := harness.tt.Image()
				maxAlpha := uint8(156 + 55*harness.a.pal.grain)
				for _, point := range []image.Point{{X: 1100, Y: 600}, {X: 1100, Y: 20}} {
					alpha := rendered.RGBAAt(point.X, point.Y).A
					if alpha < 150 || alpha > maxAlpha {
						t.Errorf("background at %v has alpha %d, want 153 plus theme grain (up to %d)", point, alpha, maxAlpha)
					}
				}
				// The sidebar is a card of the window's chrome, opaque even
				// while the editors behind the transparent window are not.
				if alpha := rendered.RGBAAt(100, 650).A; alpha != 255 {
					t.Errorf("the sidebar's card has alpha %d, want 255", alpha)
				}
				bounds, found := harness.tt.Find(locales.EditorLabel("main.go"))
				if !found {
					t.Fatal("editor is missing")
				}
				textX := int(bounds.X + harness.a.textEditor().gutterW + gutterGap)
				var textAlpha uint8
				for row := int(bounds.Y) + editorPadY; row < int(bounds.Y)+100; row++ {
					for column := textX; column < textX+300; column++ {
						textAlpha = max(textAlpha, rendered.RGBAAt(column, row).A)
					}
				}
				if textAlpha < 245 {
					t.Errorf("editor text is faded: maximum alpha %d", textAlpha)
				}
				for _, material := range []bool{false, true} {
					harness.tt.SetVibrancy(material)
					if material {
						harness.a.settings.WindowOpacity = 1
					}
					harness.settle()
					if alpha := harness.tt.Image().RGBAAt(1100, 600).A; alpha != 255 {
						t.Errorf("opaque fallback has alpha %d with material %v", alpha, material)
					}
				}
			})
		}
	}
}

// The line numbers of an editor stand on the window's material too: the
// gutter is no denser than the code beside it, with Mica or without.
func TestEditorGutterBackgroundOpacity(t *testing.T) {
	for _, mica := range []bool{false, true} {
		t.Run(fmt.Sprintf("mica=%v", mica), func(t *testing.T) {
			hs := newHarness(t, true)
			hs.a.settings.MicaEnabled = mica
			hs.a.settings.MicaAnimation = false
			hs.a.settings.WindowOpacity = 0.6
			hs.tt.SetVibrancy(true)
			hs.a.openFile(hs.path("main.go"), true)
			hs.settle()
			hs.until("the editor's restored scroll position", func() bool {
				d := hs.a.activeEditor()
				return d != nil && d.coverFrame != hs.a.frame
			})
			bounds, found := hs.tt.Find(locales.EditorLabel("main.go"))
			if !found {
				t.Fatal("editor is missing")
			}
			d := hs.a.activeEditor()
			gutterX := int(bounds.X) + 2
			codeX := int(bounds.X+d.gutterW+gutterGap) + 2
			rendered := hs.tt.Image()
			gutter := rendered.RGBAAt(gutterX, int(bounds.Y)+editorPadY+3).A
			code := rendered.RGBAAt(codeX, int(bounds.Y)+editorPadY+3).A
			want := 156 + 55*hs.a.pal.grain
			if math.Abs(float64(gutter)-float64(want)) > 2 || math.Abs(float64(code)-float64(want)) > 2 {
				t.Fatalf("gutter alpha %d and code alpha %d, want about %.0f", gutter, code, want)
			}
		})
	}
}

func TestSettingsBackgroundOpacity(t *testing.T) {
	harness := newPrefsHarness(t)
	harness.pt.SetVibrancy(true)
	harness.w.set("windowOpacity", json.RawMessage(`0.6`))
	harness.frames()
	for _, point := range []image.Point{{X: 900, Y: 670}, {X: 20, Y: 600}} {
		if alpha := harness.pt.Image().RGBAAt(point.X, point.Y).A; alpha < 150 || alpha > 156 {
			t.Errorf("settings background at %v has alpha %d, want about 153", point, alpha)
		}
	}
}

func TestTerminalBackgroundOpacity(t *testing.T) {
	if testing.Short() {
		t.Skip("loads libghostty-vt")
	}
	for _, dark := range []bool{false, true} {
		harness := newHarness(t, dark)
		harness.a.settings.WindowOpacity = 0.6
		harness.tt.SetVibrancy(true)
		terminalSession := harness.a.startTerminal(harness.root)
		if terminalSession == nil {
			t.Fatalf("terminal did not start: %v", harness.a.errors)
		}
		t.Cleanup(harness.a.closeTerminals)
		harness.a.panelTerm = terminalSession
		harness.a.window.PanelOpen = true
		harness.settle()
		bounds, found := harness.tt.Find("Terminal")
		if !found {
			t.Fatal("terminal is missing")
		}
		rendered := harness.tt.Image()
		alpha := rendered.RGBAAt(int(bounds.X+bounds.W/2), int(bounds.Y+bounds.H/2)).A
		if alpha < 150 || alpha > 156 {
			t.Errorf("terminal background has alpha %d, want about 153", alpha)
		}
	}
}

// The sidebar is a card: it keeps a gap and a rounded corner, and it stays
// opaque even in a transparent window, so no desktop shows through the
// menu. The gap takes the window's surface, so it meets the title bar and
// the editors without a seam.
func TestSidebarCardStaysOpaque(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	hs.a.settings.ProjectTabs = true
	hs.a.settings.Theme = themeOneDarkPro
	hs.a.settings.WindowOpacity = 0.85
	hs.a.settings.MicaEnabled = false
	hs.tt.SetVibrancy(true)
	hs.a.openFile(hs.path("main.go"), true)
	hs.settle()
	img := hs.tt.Image()
	edge := int(hs.a.window.SidebarWidth)
	// The gap composites exactly as the editor beside it does, so no seam
	// parts them: the window's opacity reaches both by the same amount.
	ring := commonColor(img, 0, 200, 4, 400)
	editor := commonColor(img, edge+60, 200, edge+260, 400)
	if ring.A != editor.A || abs(int(ring.R)-int(editor.R)) > 1 ||
		abs(int(ring.G)-int(editor.G)) > 1 || abs(int(ring.B)-int(editor.B)) > 1 {
		t.Fatalf("the gap around the sidebar is %v, the editor beside it %v", ring, editor)
	}
	// The card itself stays opaque: no desktop shows through the menu.
	card := img.RGBAAt(edge/2, titleBarH+40)
	if card.A != 255 {
		t.Fatalf("the sidebar's card has alpha %d, want 255", card.A)
	}
	if ring == card {
		t.Fatalf("the sidebar has no gap around its card: %v", ring)
	}
}

func TestFallbackWorkbenchSurfaces(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	for _, view := range []struct {
		name, theme string
		dark        bool
	}{{"light", "", false}, {"dark", themeOneDarkPro, true}} {
		t.Run(view.name, func(t *testing.T) {
			hs := newHarness(t, view.dark)
			hs.a.settings.Theme = view.theme
			hs.a.settings.MicaEnabled = false
			hs.a.settings.ProjectTabs = true
			hs.a.openFile(hs.path("main.go"), true)
			hs.settle()
			if hs.a.pal != builtinPalette("", view.dark) {
				t.Fatal("expected the basic palette without an installed theme")
			}
			img := hs.tt.Image()
			edge := int(hs.a.window.SidebarWidth)
			editor := commonColor(img, edge+80, 300, edge+240, 400)
			metrics := hs.a.pal.metricsOr()
			inset := int(metrics.pad + metrics.gap)
			bottom := img.Bounds().Dy() - statusBarH - int(metrics.pad)
			for name, point := range map[string]image.Point{
				"gap": {X: 2, Y: 300}, "corner": {X: inset, Y: titleBarH}, "title": {X: 850, Y: 8},
				"below sidebar": {X: edge / 2, Y: bottom},
				"tabs":          {X: 850, Y: titleBarH + 5}, "status": {X: 850, Y: img.Bounds().Dy() - 2},
			} {
				if color := img.RGBAAt(point.X, point.Y); color != editor {
					t.Errorf("%s at %v is %v, want editor %v", name, point, color, editor)
				}
			}
			if card := img.RGBAAt(inset+2, 300); card.A != 255 || card.R >= editor.R {
				t.Errorf("expected an opaque, darker sidebar card, got %v beside %v", card, editor)
			} else if got := img.RGBAAt(edge/2, bottom-1); got != card {
				t.Errorf("last row of sidebar = %v, want %v", got, card)
			}
			hs.shot("workbench-basic-" + view.name)
		})
	}
}

// commonColor is the most common color in a part of img: a row's
// background, where its text is sparse.
func commonColor(img *image.RGBA, x0, y0, x1, y1 int) color.RGBA {
	counts := make(map[color.RGBA]int)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			counts[img.RGBAAt(x, y)]++
		}
	}
	var common color.RGBA
	for c, n := range counts {
		if n > counts[common] {
			common = c
		}
	}
	return common
}

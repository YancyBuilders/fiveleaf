package main

import (
	"bytes"
	"image"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
)

// A small real view exercises the same material, surfaces and clock as the
// editor without spending a whole desktop frame on every animation step.
type micaHarness struct {
	tt    *ui.Tester
	prefs settings.Prefs
	pal   *palette
	mat   *windowMaterial
	fade  paletteFade
}

func newMicaHarness(dark bool) *micaHarness {
	h := &micaHarness{prefs: settings.Default().Prefs}
	h.prefs.MicaEnabled = true
	h.prefs.Appearance = "light"
	if dark {
		h.prefs.Appearance = "dark"
	}
	h.tt = ui.NewTester(func(c *ui.Context) {
		h.pal = applyTheme(c, &h.prefs, &h.fade)
		h.mat = materialOf(c)
		if translucentWindow(c, h.prefs.WindowOpacity) {
			defer c.Root().Background(ui.Transparent)
		}
		box := ui.Row(c).Fill().Background(windowContainer(c, h.prefs.WindowOpacity, h.pal.chrome))
		materialBackdrop(c, box, h.pal, h.prefs.WindowOpacity,
			!h.prefs.MicaAnimation || c.Preferences().ReduceMotion).Children(func() {
			ui.Box(c).Width(60).FillHeight().Background(windowBackground(c, h.prefs.WindowOpacity, h.pal.chrome))
			ui.Box(c).Grow(1).FillHeight().Background(windowBackground(c, h.prefs.WindowOpacity, h.pal.editor))
		})
	}, 180, 100)
	return h
}

func micaSnapshot(tt *ui.Tester) *image.RGBA {
	src := tt.Image()
	img := image.NewRGBA(src.Bounds())
	copy(img.Pix, src.Pix)
	return img
}

func TestMicaBackgroundMovesAndPauses(t *testing.T) {
	for _, pause := range []string{"setting", "reduce-motion"} {
		t.Run(pause, func(t *testing.T) {
			h := newMicaHarness(true)
			first := micaSnapshot(h.tt)
			h.tt.Advance(motionMicaCycle / 12)
			if bytes.Equal(first.Pix, h.tt.Image().Pix) {
				t.Fatal("Mica did not move while the window was active")
			}
			switch pause {
			case "setting":
				h.prefs.MicaAnimation = false
			case "reduce-motion":
				h.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			}
			h.tt.Frame()
			paused := micaSnapshot(h.tt)
			h.tt.Advance(motionMicaCycle)
			if !bytes.Equal(paused.Pix, h.tt.Image().Pix) || h.tt.FramePending() {
				t.Fatal("paused Mica changed its image or continued requesting frames")
			}
			h.prefs.MicaAnimation = true
			h.tt.SetPreferences(ui.Preferences{})
			h.tt.Frame()
			if !bytes.Equal(paused.Pix, h.tt.Image().Pix) {
				t.Fatal("Mica jumped forward when the window resumed")
			}
			h.tt.Advance(motionMicaCycle / 12)
			if bytes.Equal(paused.Pix, h.tt.Image().Pix) {
				t.Fatal("Mica did not resume moving")
			}
		})
	}
}

func TestMicaMaterialIsPerWindow(t *testing.T) {
	light, dark := newMicaHarness(false), newMicaHarness(true)
	light.prefs.MicaAnimation = false
	light.tt.Frame()
	first := micaSnapshot(light.tt)
	dark.tt.Advance(motionMicaCycle / 8)
	light.tt.Frame()
	if light.mat == dark.mat || light.pal.dark || !dark.pal.dark || !bytes.Equal(first.Pix, light.tt.Image().Pix) {
		t.Fatal("one window changed another window's material")
	}
	dark.prefs.MicaEnabled = false
	dark.tt.Frame()
	dark.tt.Advance(paletteFadeFor + motionMicaFrame)
	if dark.tt.FramePending() || dark.mat.mica() {
		t.Fatal("leaving Mica kept its animation or transparent surfaces")
	}
	if got := dark.tt.Image().RGBAAt(120, 50); got.A != 255 {
		t.Fatalf("leaving Mica did not restore the opaque editor: %v", got)
	}
}

func TestMicaKeepsSelectedTheme(t *testing.T) {
	withAppearancePlugins(t)
	for _, theme := range []string{"", themeOneDarkPro, themeAyu, themeCursor} {
		t.Run(theme, func(t *testing.T) {
			h := newMicaHarness(true)
			h.prefs.Theme = theme
			h.prefs.MicaEnabled = false
			h.tt.Frame()
			h.tt.Advance(paletteFadeFor + motionMicaFrame)
			original, originalPalette := micaSnapshot(h.tt), h.pal
			h.prefs.MicaEnabled = true
			h.tt.Frame()
			if !h.mat.mica() || h.pal != originalPalette || h.prefs.Theme != theme {
				t.Fatal("enabling Mica changed the selected palette or did not add the material")
			}
			if bytes.Equal(original.Pix, h.tt.Image().Pix) {
				t.Fatal("enabling Mica did not change the background")
			}
			h.tt.Advance(motionMicaCycle / 12)
			h.prefs.MicaEnabled = false
			h.tt.Frame()
			if !bytes.Equal(original.Pix, h.tt.Image().Pix) || h.tt.FramePending() {
				t.Fatal("disabling Mica did not restore the original theme and stop animation")
			}
		})
	}
}

func TestMicaMaterialOpacity(t *testing.T) {
	for _, dark := range []bool{false, true} {
		h := newMicaHarness(dark)
		h.prefs.MicaAnimation = false
		for _, frost := range []float64{0.15, 0.55, 0.95} {
			h.prefs.MicaFrost = frost
			for _, opacity := range []float64{settings.MinWindowOpacity, 0.6, 1} {
				h.prefs.WindowOpacity = opacity
				h.tt.SetVibrancy(true)
				for _, point := range []image.Point{{20, 50}, {120, 50}} {
					if alpha := h.tt.Image().RGBAAt(point.X, point.Y).A; abs(int(alpha)-int(opacity*255)) > 2 {
						t.Fatalf("Mica at opacity %v has alpha %d at %v", opacity, alpha, point)
					}
				}
			}
		}
		h.prefs.WindowOpacity = 0.6
		h.tt.SetVibrancy(false)
		for _, point := range []image.Point{{20, 50}, {120, 50}} {
			if alpha := h.tt.Image().RGBAAt(point.X, point.Y).A; alpha != 255 {
				t.Fatalf("Mica without system glass is not opaque at %v: alpha %d", point, alpha)
			}
		}
	}
}

func TestMicaWorkbenchOpacity(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	hs.a.settings.Theme = themeOneDarkPro
	hs.a.settings.MicaEnabled = true
	hs.a.settings.MicaAnimation = false
	hs.a.settings.WindowOpacity = 0.6
	hs.tt.SetVibrancy(true)
	hs.a.openFile(hs.path("main.go"), true)
	hs.settle()
	hs.until("the editor's restored scroll position", func() bool {
		d := hs.a.activeEditor()
		return d != nil && d.coverFrame != hs.a.frame
	})
	for _, point := range []image.Point{{1100, 600}, {400, 650}, {1100, 20}} {
		if alpha := hs.tt.Image().RGBAAt(point.X, point.Y).A; abs(int(alpha)-153) > 2 {
			t.Errorf("Mica workbench background at %v has alpha %d, want 153", point, alpha)
		}
	}
	// The sidebar's card stays opaque, so the menu never shows the desktop.
	if alpha := hs.tt.Image().RGBAAt(100, 650).A; alpha != 255 {
		t.Errorf("Mica sidebar card has alpha %d, want 255", alpha)
	}
}

func TestMicaSidebarMeetsEditorAndStatusBar(t *testing.T) {
	hs := newHarness(t, false)
	hs.a.settings.MicaEnabled, hs.a.settings.MicaAnimation = true, false
	hs.a.settings.WindowOpacity = 0.6
	hs.tt.SetVibrancy(true)
	for _, right := range []bool{false, true} {
		hs.a.settings.SidebarRight = right
		hs.settle()
		frame := hs.tt.Image()
		width := int(hs.a.window.SidebarWidth)
		inner, center := width-2, width/2
		if right {
			inner, center = frame.Bounds().Dx()-width+1, frame.Bounds().Dx()-width/2
		}
		middle, bottom := frame.Bounds().Dy()/2, frame.Bounds().Dy()-int(hs.a.statusHeight())-1
		if frame.RGBAAt(inner, middle).A != 255 || frame.RGBAAt(center, bottom).A != 255 {
			t.Fatalf("sidebar on right=%v leaves a gutter next to the editor or status bar", right)
		}
		if frame.RGBAAt(center, middle) == frame.RGBAAt(center, middle+80) {
			t.Fatalf("sidebar on right=%v hides the current behind a flat fill", right)
		}
	}
}

func TestMicaComposerSharesTheCurrentAndBottomEdge(t *testing.T) {
	hs := newHarness(t, false)
	hs.a.settings.MicaEnabled, hs.a.settings.MicaAnimation = true, false
	agentViewFixture(hs)
	hs.settle()
	frame := hs.tt.Image()
	box := hs.a.agent.composeBox.Bounds(hs.a.review.ctx)
	if box.H == 0 || abs(int(box.Y+box.H)-frame.Rect.Dy()+int(hs.a.statusHeight())) > 1 {
		t.Fatalf("the composer does not meet the status bar: %+v", box)
	}
	sample := image.Rect(int(box.X+box.W/3), int(box.Y+8), int(box.X+2*box.W/3), int(box.Y+32))
	before := micaSnapshot(hs.tt)
	hs.a.settings.MicaHue = 25
	hs.settle()
	after := hs.tt.Image()
	for row := sample.Min.Y; row < sample.Max.Y; row++ {
		for column := sample.Min.X; column < sample.Max.X; column++ {
			if before.RGBAAt(column, row) != after.RGBAAt(column, row) {
				return
			}
		}
	}
	t.Fatal("the composer glass did not pick up the changed Mica hue")
}

func TestMicaRenderedTextContrast(t *testing.T) {
	withAppearancePlugins(t)
	for _, dark := range []bool{false, true} {
		h := newMicaHarness(dark)
		for range 16 {
			img := h.tt.Image()
			for y := 10; y < 100; y += 20 {
				for x := 10; x < 180; x += 20 {
					pixel := img.RGBAAt(x, y)
					back := ui.RGBA(pixel.R, pixel.G, pixel.B, 1)
					// The material preserves the theme's syntax colors, whose
					// contrast varies by theme. Check the shared reading text.
					for _, ink := range []ui.Color{h.pal.text, h.pal.muted} {
						if ratio := contrast(ink, back); ratio < 4.5 {
							t.Fatalf("dark=%v ink %v reads at %.2f:1 on rendered background %v at %d,%d", dark, ink, ratio, back, x, y)
						}
					}
				}
			}
			h.tt.Advance(motionMicaCycle / 16)
		}
	}
}

func TestMicaSettingsBackground(t *testing.T) {
	withAppearancePlugins(t)
	ph := newPrefsHarness(t)
	ph.m.s.Theme = themeOneDarkPro
	ph.m.s.MicaEnabled = true
	ph.frames()
	if !samePalette(ph.w.pal, oneDarkProPal) {
		t.Fatal("Mica changed the selected theme")
	}
	before := micaSnapshot(ph.pt)
	ph.pt.Advance(motionMicaCycle / 12)
	// Check the sidebar margin and the clear inner edge of the settings
	// group, so an opaque group cannot hide the material while edges move.
	for _, point := range []image.Point{{4, 400}, {900, 110}} {
		if before.RGBAAt(point.X, point.Y) == ph.pt.Image().RGBAAt(point.X, point.Y) {
			t.Fatalf("settings window's background at %v did not move", point)
		}
	}
	ph.m.s.MicaAnimation = false
	ph.frames()
	before = micaSnapshot(ph.pt)
	ph.pt.Advance(time.Second)
	// The search field can still blink. Compare the clear sidebar margin
	// so ordinary control animations cannot stand in for material motion.
	for y := 400; y < 620; y += 10 {
		if before.RGBAAt(4, y) != ph.pt.Image().RGBAAt(4, y) {
			t.Fatal("settings background moved while animation was disabled")
		}
	}
	ph.m.s.WindowOpacity = 0.6
	ph.pt.SetVibrancy(true)
	ph.frames()
	if alpha := ph.pt.Image().RGBAAt(900, 110).A; abs(int(alpha)-153) > 2 {
		t.Fatalf("settings group has alpha %d, want the window's 153", alpha)
	}
}

func TestShotMica(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("set SHOTS to save Mica screenshots")
	}
	withAppearancePlugins(t)
	src, err := os.ReadFile("mica.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		hs := newHarness(t, dark)
		hs.a.settings.MicaEnabled = true
		if err := os.WriteFile(hs.path("mica.go"), src, 0o644); err != nil {
			t.Fatal(err)
		}
		hs.a.openFile(hs.path("mica.go"), true)
		hs.tt.SetSize(1440, 900)
		hs.settle()
		name := "mica-light"
		if dark {
			name = "mica-dark"
		}
		hs.shot(name)
		hs.tt.SetSize(680, 720)
		hs.settle()
		hs.shot(name + "-narrow")
	}
}

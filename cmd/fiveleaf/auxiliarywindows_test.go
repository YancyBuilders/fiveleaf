package main

import (
	"fmt"
	"image"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestAuxiliaryWindowControls(t *testing.T) {
	for _, platform := range []struct {
		name string
		bar  ui.TitleBar
	}{
		{"macOS", ui.TitleBar{Height: 56, Left: 80}},
		{"Windows", ui.TitleBar{Height: 56, Right: 138}},
		{"Linux", ui.TitleBar{Height: 56, Right: 110}},
	} {
		t.Run(platform.name, func(t *testing.T) {
			check := func(tt *ui.Tester, labels ...string) {
				t.Helper()
				bounds := map[string]ui.Rect{}
				for _, label := range labels {
					r, found := tt.Find(label)
					if !found || r.W <= 0 || r.H <= 0 || r.X < 0 || r.X+r.W > 641 || r.Y < 0 {
						t.Errorf("%q is outside the 640px window: %v (found %v)", label, r, found)
						continue
					}
					if r.Y < platform.bar.Height && (r.X < platform.bar.Left || r.X+r.W > 640-platform.bar.Right+1) {
						t.Errorf("%q overlaps native window buttons: %v, title bar %v", label, r, platform.bar)
					}
					for otherLabel, other := range bounds {
						if overlaps(r, other) {
							t.Errorf("%q overlaps %q: %v, %v", label, otherLabel, r, other)
						}
					}
					bounds[label] = r
				}
			}
			ph, _ := withPluginsPage(t)
			ph.pt.SetSize(640, 680)
			ph.pt.SetTitleBar(platform.bar)
			for _, zoom := range []int{0, 1} {
				ph.m.s.ZoomLevel = float32(zoom)
				t.Logf("settings zoom level %d", zoom)
				for _, page := range []struct {
					section string
					byKeys  bool
					labels  []string
				}{
					{settings.CatAppearance, false, []string{locales.PrefsScope()}},
					{prefsShortcuts, false, []string{locales.KeysSearch(), locales.KeysSearchByKeys()}},
					{prefsShortcuts, true, []string{locales.KeysPressToSearch(), locales.KeysSearchByKeys()}},
					{settings.CatPlugins, false, []string{locales.CmdPluginsReload(), locales.CmdPluginsOpenFolder()}},
				} {
					ph.w.section, ph.w.keys.byKeys = page.section, page.byKeys
					ph.frames()
					check(ph.pt, append([]string{locales.PrefsSearch()}, page.labels...)...)
				}
				ph.w.openDetail("prettier")
				ph.frames()
				check(ph.pt, locales.PrefsSearch(), locales.PluginsBack())
				if err := ph.pt.Click(locales.PluginsBack()); err != nil {
					t.Fatal(err)
				}
				ph.frames()
				if ph.w.plugins.detail != "" {
					t.Fatal("the integrated header's Back control did not leave plugin details")
				}
			}
		})
	}
}

func TestAuxiliaryWindowBackground(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			ph := newPrefsHarness(t)
			for _, window := range []struct {
				name   string
				prefs  *settings.Prefs
				tt     *ui.Tester
				frames func()
				points []image.Point
			}{
				{"settings", &ph.m.s.Prefs, ph.pt, ph.frames, []image.Point{{400, 4}, {4, 400}, {204, 400}}},
			} {
				for _, mode := range []struct {
					mica    bool
					opacity float64
				}{
					{false, 1}, {false, 0.6}, {true, 0.6},
				} {
					t.Run(fmt.Sprintf("%s/mica=%v/opacity=%g", window.name, mode.mica, mode.opacity), func(t *testing.T) {
						window.prefs.Appearance = appearance
						window.prefs.MicaEnabled = mode.mica
						window.prefs.MicaAnimation = false
						window.prefs.WindowOpacity = mode.opacity
						window.tt.SetVibrancy(true)
						window.frames()
						img := window.tt.Image()
						background := img.RGBAAt(window.points[0].X, window.points[0].Y)
						for _, point := range window.points {
							pixel := img.RGBAAt(point.X, point.Y)
							if abs(int(pixel.A)-int(mode.opacity*255)) > 2 {
								t.Errorf("background at %v has alpha %d, want one layer at %g", point, pixel.A, mode.opacity)
							}
							if !mode.mica && pixel != background {
								t.Errorf("background at %v is %v, different from the header's %v", point, pixel, background)
							}
						}
					})
				}
			}
		})
	}
}

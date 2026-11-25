package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"fiveleaf/internal/settings"
)

func TestMicaSliderPreview(t *testing.T) {
	withAppearancePlugins(t)
	ph := newPrefsHarness(t)
	w := ph.w
	w.set("theme", json.RawMessage(`"onedarkpro"`))
	w.set("micaEnabled", json.RawMessage(`true`))
	w.set("micaAnimation", json.RawMessage(`false`))
	w.query = "micaFrost"
	ph.frames()
	background := ph.pt.Image().RGBAAt(20, 400)
	ph.m.s.Flush()
	before, err := os.ReadFile(ph.m.s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !ph.pt.HasText("55%") {
		t.Fatal("no Mica frost slider")
	}
	slider := w.firstH.Bounds(ph.c)
	y, right := slider.Y+slider.H/2, slider.X+slider.W
	ph.pt.Press(slider.X+8, y)
	step(ph.pt)
	ph.pt.Move(right+30, y)
	ph.frames()
	if w.held != "micaFrost" || w.materialPrefs().MicaFrost != 0.95 {
		t.Fatalf("slider did not preview its live value: held %q, frost %v", w.held, w.materialPrefs().MicaFrost)
	}
	if ph.pt.Image().RGBAAt(20, 400) == background {
		t.Error("settings background did not preview frost while dragging")
	}
	if ph.m.s.MicaFrost != 0.55 || ph.a.settings.MicaFrost != 0.55 || ph.m.s.Theme != "onedarkpro" {
		t.Fatal("dragging changed the shared settings or theme")
	}
	whileHeld, err := os.ReadFile(ph.m.s.Path())
	if err != nil || !bytes.Equal(before, whileHeld) {
		t.Fatal("dragging wrote the settings file")
	}
	ph.pt.Release(right+30, y)
	ph.frames()
	ph.m.s.Flush()
	if got := settings.Load(ph.m.s.Path()); w.held != "" || got.MicaFrost != 0.95 || got.Theme != "onedarkpro" {
		t.Fatal("releasing the slider did not save its value independently of the theme")
	}
}

func TestMicaControlsDisabledAndSearchable(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.w.query = "micaHue"
	ph.frames()
	fields := ph.w.shown(false)
	if len(fields) != 1 || fields[0].Key != "micaHue" || !ph.pt.HasText(prefTitle("micaHue")) {
		t.Fatal("a disabled Mica parameter was not searchable")
	}
	if !ph.pt.HasText("177°") {
		t.Fatal("no disabled hue slider")
	}
	slider := ph.w.firstH.Bounds(ph.c)
	y, right := slider.Y+slider.H/2, slider.X+slider.W
	ph.pt.Press(slider.X+8, y)
	step(ph.pt)
	ph.pt.Move(right+30, y)
	step(ph.pt)
	ph.pt.Release(right+30, y)
	ph.frames()
	if ph.m.s.MicaHue != 177 || ph.w.held != "" {
		t.Fatal("a disabled Mica slider accepted input")
	}
	ph.w.set("micaEnabled", json.RawMessage(`true`))
	ph.frames()
	slider = ph.w.firstH.Bounds(ph.c)
	ph.pt.ClickAt(slider.X+slider.W-1, slider.Y+slider.H/2)
	ph.frames()
	if ph.m.s.MicaHue != 360 {
		t.Fatalf("enabled hue slider did not reach its endpoint: %v", ph.m.s.MicaHue)
	}
}

func TestMicaParameterNumbers(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value float64
		want  string
	}{
		{"micaFrost", 0.55, "55%"}, {"micaHue", 177, "177°"},
		{"micaIntensity", 0.8, "80%"}, {"micaSpeed", 1.25, "1.25×"},
	} {
		if got := prefNumber(tc.key, tc.value); got != tc.want {
			t.Errorf("%s reads %q, want %q", tc.key, got, tc.want)
		}
	}
}

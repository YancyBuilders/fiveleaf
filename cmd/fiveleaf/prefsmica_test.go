package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestMicaPresetControlsWhileOff(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.w.query = "micaEnabled"
	ph.frames()
	if ph.m.s.MicaEnabled || !ph.pt.HasText(locales.PrefsMicaImport()) || !ph.pt.HasText(locales.PrefsMicaExport()) {
		t.Fatal("Mica import and export are missing while the material is off")
	}
}

func TestMicaPresetImportUpdatesWindows(t *testing.T) {
	withAppearancePlugins(t)
	ph := newPrefsHarness(t)
	ph.m.s.Theme, ph.m.s.FontSize = "onedarkpro", 17
	ph.frames()
	source := settings.Default().Prefs
	source.MicaEnabled, source.MicaAnimation = true, false
	source.MicaFrost, source.MicaHue, source.MicaIntensity = 0.75, 234, 0.9
	source.MicaSpeed, source.WindowOpacity = 2, 0.8
	raw, err := source.ExportMica()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "saved-mica.json")
	if err := settings.WriteFile(path, raw); err != nil {
		t.Fatal(err)
	}
	read, err := readMicaPreset(path)
	if err != nil || !bytes.Equal(raw, read) {
		t.Fatalf("could not read the exported preset: %v", err)
	}
	ph.w.micaBusy, ph.w.held = true, "micaHue"
	ph.w.finishMicaImport(path, read, nil)
	if ph.w.micaBusy || ph.w.held != "" || len(ph.w.errors) != 0 || len(ph.w.notes) != 1 {
		t.Fatal("import did not finish cleanly or release the live preview")
	}
	ph.frames()
	ph.tt.Frame()
	ph.m.s.Flush()
	for _, p := range []settings.Prefs{ph.m.s.Prefs, ph.a.settings.Prefs, ph.w.materialPrefs(), settings.Load(ph.m.s.Path()).Prefs} {
		if !p.MicaEnabled || p.MicaAnimation || p.MicaHue != 234 || p.MicaFrost != 0.75 || p.MicaIntensity != 0.9 ||
			p.MicaSpeed != 2 || p.WindowOpacity != 0.8 || p.Theme != "onedarkpro" || p.FontSize != 17 {
			t.Fatal("import lost a material setting or changed the selected theme or font")
		}
	}
}

func TestMicaPresetCancelAndErrors(t *testing.T) {
	ph := newPrefsHarness(t)
	w := ph.w
	before := ph.m.s.Prefs
	w.micaBusy = true
	w.finishMicaImport("", nil, nil)
	w.micaBusy = true
	w.finishMicaExport("", nil)
	if w.micaBusy || len(w.notes) != 0 || len(w.errors) != 0 || !reflect.DeepEqual(before, ph.m.s.Prefs) {
		t.Fatal("canceling a file dialog changed settings or reported a result")
	}
	w.finishMicaImport("broken.json", []byte(`{"version":1}`), nil)
	if len(w.errors) != 1 || len(w.notes) != 0 || !reflect.DeepEqual(before, ph.m.s.Prefs) {
		t.Fatal("invalid import changed settings or did not report its error")
	}
	w.finishMicaExport("unwritable.json", errors.New("disk unavailable"))
	if len(w.errors) != 2 || len(w.notes) != 0 {
		t.Fatal("failed export reported success")
	}
	path := filepath.Join(t.TempDir(), "too-large.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte(" "), settings.MaxMicaPresetSize*2), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := readMicaPreset(path)
	if err != nil || len(raw) != settings.MaxMicaPresetSize+1 {
		t.Fatal("import did not bound the file read")
	}
	w.finishMicaImport(path, raw, nil)
	if len(w.errors) != 3 || !reflect.DeepEqual(before, ph.m.s.Prefs) {
		t.Fatal("oversized import changed settings or did not report an error")
	}
	w.finishMicaImport("", nil, errors.New("dialog unavailable"))
	w.finishMicaExport("", errors.New("dialog unavailable"))
	if len(w.errors) != 5 ||
		w.errors[3] != locales.FailWithError(locales.PrefsMicaImportTitle(), "dialog unavailable") ||
		w.errors[4] != locales.FailWithError(locales.PrefsMicaExportTitle(), "dialog unavailable") {
		t.Fatal("a dialog error did not identify its action")
	}
}

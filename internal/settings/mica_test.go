package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMicaPrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	if s.MicaEnabled || !s.MicaAnimation {
		t.Fatal("Mica must be opt-in, with animation available when enabled")
	}
	s.Theme, s.Appearance = "onedarkpro", "dark"
	for key, value := range map[string]string{
		"micaEnabled": "true", "micaAnimation": "false", "micaFrost": "0.75",
		"micaHue": "0", "micaIntensity": "0", "micaSpeed": "2.5",
	} {
		if err := s.Prefs.Set(key, json.RawMessage(value)); err != nil {
			t.Fatal(err)
		}
	}
	s.Flush()
	got := Load(path)
	if !got.MicaEnabled || got.MicaAnimation || got.MicaFrost != 0.75 || got.MicaHue != 0 ||
		got.MicaIntensity != 0 || got.MicaSpeed != 2.5 || got.Theme != "onedarkpro" || got.Appearance != "dark" {
		t.Fatalf("Mica settings did not round-trip: %+v", got.Prefs)
	}
	got.MicaEnabled = false
	got.Flush()
	if off := Load(path); off.MicaEnabled || off.Theme != "onedarkpro" || off.MicaFrost != 0.75 {
		t.Fatal("turning Mica off lost the chosen theme or parameters")
	}
}

func TestMicaPrefsBounds(t *testing.T) {
	for _, tc := range []struct {
		key      string
		min, max float64
	}{
		{"micaFrost", 0.15, 0.95}, {"micaHue", 0, 360},
		{"micaIntensity", 0, 1}, {"micaSpeed", 0.25, 3},
	} {
		t.Run(tc.key, func(t *testing.T) {
			p := Default().Prefs
			for _, value := range []struct {
				raw  string
				want float64
			}{{"-100", tc.min}, {"1000", tc.max}} {
				if err := p.Set(tc.key, json.RawMessage(value.raw)); err != nil {
					t.Fatal(err)
				}
				var got float64
				if err := json.Unmarshal(p.Raw(tc.key), &got); err != nil {
					t.Fatal(err)
				}
				if got != value.want {
					t.Errorf("%s normalized to %v, want %v", value.raw, got, value.want)
				}
			}
		})
	}
}

func TestMicaLegacyTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := []byte(`{"theme":"mica","appearance":"dark","micaAnimation":false}`)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if s.Theme != "" || !s.MicaEnabled || s.Appearance != "dark" || s.MicaAnimation {
		t.Fatal("legacy theme did not become an independent material")
	}
	s.Flush()
	if got := Load(path); got.Theme != "" || !got.MicaEnabled || got.MicaAnimation {
		t.Fatal("migrated material did not persist")
	}
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	if p, ok := s.Edited(); !ok || p.Theme != "" || !p.MicaEnabled || p.MicaAnimation {
		t.Fatal("a watched legacy settings file did not migrate")
	}
}

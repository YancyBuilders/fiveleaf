package settings

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestMicaPresetRoundTrip(t *testing.T) {
	source := Default().Prefs
	source.Theme, source.FontFamily = "onedarkpro", "private-font-choice"
	source.MicaEnabled, source.MicaAnimation = true, false
	source.MicaFrost, source.MicaHue, source.MicaIntensity = 0.95, 0, 0
	source.MicaSpeed, source.WindowOpacity = 3, 0.6
	raw, err := source.ExportMica()
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 8 || fields["theme"] != nil || fields["fontFamily"] != nil {
		t.Fatalf("export includes unrelated preferences: %s", raw)
	}
	target := Default().Prefs
	target.Theme, target.FontSize = "ayu", 19
	want := target
	want.MicaEnabled, want.MicaAnimation = source.MicaEnabled, source.MicaAnimation
	want.MicaFrost, want.MicaHue, want.MicaIntensity = source.MicaFrost, source.MicaHue, source.MicaIntensity
	want.MicaSpeed, want.WindowOpacity = source.MicaSpeed, source.WindowOpacity
	if err := target.ImportMica(raw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(target, want) {
		t.Fatal("import lost a material value or changed an unrelated preference")
	}
	roundTrip, err := target.ExportMica()
	if err != nil || !bytes.Equal(roundTrip, raw) {
		t.Fatalf("preset did not round-trip: %v", err)
	}
}

func TestMicaPresetInvalidIsAtomic(t *testing.T) {
	base := Default().Prefs
	base.MicaEnabled, base.MicaHue = true, 224
	raw, err := base.ExportMica()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"new-version", `"version": 1`, `"version": 2`},
		{"missing", `"micaHue": 224,`, ""},
		{"null", `"micaHue": 224`, `"micaHue": null`},
		{"wrong-type", `"micaEnabled": true`, `"micaEnabled": "true"`},
		{"frost-high", `"micaFrost": 0.55`, `"micaFrost": 1`},
		{"speed-low", `"micaSpeed": 1`, `"micaSpeed": 0`},
		{"hue-high", `"micaHue": 224`, `"micaHue": 361`},
		{"intensity-low", `"micaIntensity": 0.8`, `"micaIntensity": -1`},
		{"opacity-low", `"windowOpacity": 1`, `"windowOpacity": 0`},
		{"unrelated", `"version": 1`, `"version": 1, "theme": "matrix"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := bytes.Replace(raw, []byte(tc.from), []byte(tc.to), 1)
			if bytes.Equal(raw, bad) {
				t.Fatal("fixture did not change")
			}
			p := base
			if err := p.ImportMica(bad); err == nil || !reflect.DeepEqual(p, base) {
				t.Fatalf("bad preset changed preferences or was accepted: %v", err)
			}
		})
	}
	for _, bad := range [][]byte{nil, []byte("null"), []byte("[]"), []byte("{"), append(bytes.Clone(raw), '{', '}'), bytes.Repeat([]byte(" "), MaxMicaPresetSize+1)} {
		p := base
		if err := p.ImportMica(bad); err == nil || !reflect.DeepEqual(p, base) {
			t.Fatalf("malformed preset changed preferences or was accepted: %v", err)
		}
	}
}

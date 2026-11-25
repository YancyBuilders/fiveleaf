package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
)

// MaxMicaPresetSize bounds a small, portable material preset before decoding.
const MaxMicaPresetSize = 16 * 1024

// Pointers distinguish a missing field from a meaningful false or zero.
type micaPreset struct {
	Version       int      `json:"version"`
	MicaEnabled   *bool    `json:"micaEnabled"`
	MicaAnimation *bool    `json:"micaAnimation"`
	MicaFrost     *float64 `json:"micaFrost"`
	MicaHue       *float64 `json:"micaHue"`
	MicaIntensity *float64 `json:"micaIntensity"`
	MicaSpeed     *float64 `json:"micaSpeed"`
	WindowOpacity *float64 `json:"windowOpacity"`
}

// ExportMica includes only material settings. Theme choices, folders and
// private preferences stay out of the portable file.
func (p Prefs) ExportMica() ([]byte, error) {
	preset := micaPreset{
		Version: 1, MicaEnabled: &p.MicaEnabled, MicaAnimation: &p.MicaAnimation,
		MicaFrost: &p.MicaFrost, MicaHue: &p.MicaHue, MicaIntensity: &p.MicaIntensity,
		MicaSpeed: &p.MicaSpeed, WindowOpacity: &p.WindowOpacity,
	}
	if err := preset.validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(preset, "", "  ")
	return append(raw, '\n'), err
}

// ImportMica applies a complete, validated preset in one step. Invalid files
// leave every preference intact; unrelated settings are never imported.
func (p *Prefs) ImportMica(raw []byte) error {
	if len(raw) > MaxMicaPresetSize {
		return errors.New("mica preset is too large")
	}
	var preset micaPreset
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&preset); err != nil {
		return fmt.Errorf("invalid Mica preset: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("mica preset must contain one JSON object")
	}
	if err := preset.validate(); err != nil {
		return err
	}
	p.MicaEnabled, p.MicaAnimation = *preset.MicaEnabled, *preset.MicaAnimation
	p.MicaFrost, p.MicaHue = *preset.MicaFrost, *preset.MicaHue
	p.MicaIntensity, p.MicaSpeed = *preset.MicaIntensity, *preset.MicaSpeed
	p.WindowOpacity = *preset.WindowOpacity
	return nil
}

func (p micaPreset) validate() error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported Mica preset version: %d", p.Version)
	}
	if p.MicaEnabled == nil || p.MicaAnimation == nil {
		return errors.New("mica preset requires micaEnabled and micaAnimation")
	}
	for _, field := range []struct {
		key      string
		value    *float64
		min, max float64
	}{
		{"micaFrost", p.MicaFrost, 0.15, 0.95},
		{"micaHue", p.MicaHue, 0, 360},
		{"micaIntensity", p.MicaIntensity, 0, 1},
		{"micaSpeed", p.MicaSpeed, 0.25, 3},
		{"windowOpacity", p.WindowOpacity, MinWindowOpacity, 1},
	} {
		if field.value == nil {
			return fmt.Errorf("mica preset requires %s", field.key)
		}
		v := *field.value
		if math.IsNaN(v) || math.IsInf(v, 0) || v < field.min || v > field.max {
			return fmt.Errorf("%s must be between %g and %g", field.key, field.min, field.max)
		}
	}
	return nil
}

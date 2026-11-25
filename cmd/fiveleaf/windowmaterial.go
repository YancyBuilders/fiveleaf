package main

import (
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
)

// The material belongs to the window, including when another project takes
// its place. Keeping it on the root also keeps separate windows independent.
type windowMaterialKey struct{}

type windowMaterial struct {
	enabled  bool
	frost    float32
	options  micaOptions
	backdrop micaState
	bounds   ui.Rect
}

func materialOf(c *ui.Context) *windowMaterial {
	return ui.Local(c.Root(), windowMaterialKey{}, func() windowMaterial { return windowMaterial{} })
}

func (m *windowMaterial) mica() bool { return m.enabled }

func (m *windowMaterial) configure(prefs *settings.Prefs) {
	m.enabled = prefs.MicaEnabled
	m.frost = float32(prefs.MicaFrost)
	m.options = micaOptions{hue: prefs.MicaHue, intensity: prefs.MicaIntensity, speed: prefs.MicaSpeed, frost: m.frost}
}

// All panes use one tint strength so the combined background can keep the
// requested window opacity. Their palette colors separate code from chrome.
// The palette stays opaque for selections, text and floating controls.
func (m *windowMaterial) tint(color ui.Color, opacity float32) ui.Color {
	return color.Alpha(m.frost * opacity)
}

func (m *windowMaterial) canvasColor(original ui.Color) ui.Color {
	if m.enabled {
		return ui.Transparent
	}
	return original
}

func (m *windowMaterial) insetColor(original ui.Color) ui.Color {
	if m.enabled {
		return original.Alpha(0.23 + 0.50*m.frost)
	}
	return original
}

func (m *windowMaterial) glassCard(card ui.Element, p *palette, radius float32) {
	tint := p.surface.Mix(white, 0.8)
	edge := p.text.Alpha(0.14)
	shadow := black.Alpha(0.09)
	if p.dark {
		tint = p.chrome.Mix(ui.RGB(34, 38, 47), 0.72)
		edge = p.muted.Alpha(0.24)
		shadow = black.Alpha(0.20)
	}
	veil := tint.Alpha(0.18 + 0.58*m.frost)
	card.Background(tint).Radius(radius).Border(1, edge).Shadow(0, 2, 8, -3, shadow).Draw(func(pt *ui.Painter, bounds ui.Rect) {
		if m.backdrop.canvas == nil {
			return
		}
		inside := ui.Rect{X: bounds.X + 1, Y: bounds.Y + 1, W: bounds.W - 2, H: bounds.H - 2}
		pt.Clip(inside, max(0, radius-1), func() {
			pt.ImageRegion(m.backdrop.canvas.Bitmap(), ui.Rect{}, m.bounds, ui.ImageOptions{})
			pt.Fill(inside, veil, 0)
		})
	})
}

// materialBackdrop draws before the panes, in one coordinate space. System
// glass can show through it, but the local material also works without it.
func materialBackdrop(c *ui.Context, box ui.Element, p *palette, opacity float64, still bool) ui.Element {
	m := materialOf(c)
	if !m.mica() {
		m.backdrop = micaState{}
		return box
	}
	alpha := float32(1)
	if translucentWindow(c, opacity) {
		alpha = float32(opacity)
	}
	// A tint at t over a backdrop at b has alpha t + b*(1-t).
	// Precomposing the light makes b uniform, so each surface ends at
	// exactly the requested opacity instead of getting denser per layer.
	tint := m.frost * alpha
	alpha = (alpha - tint) / (1 - tint)
	options := m.options
	return box.Draw(func(pt *ui.Painter, r ui.Rect) {
		m.bounds = r
		m.backdrop.draw(pt, r, p, options, alpha, still)
	})
}

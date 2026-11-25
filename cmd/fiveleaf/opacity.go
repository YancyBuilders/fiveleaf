package main

import (
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
)

func translucentWindow(c *ui.Context, opacity float64) bool {
	return c.Vibrancy() && opacity > 0 && opacity < 1
}

func windowBackground(c *ui.Context, opacity float64, color ui.Color) ui.Color {
	if material := materialOf(c); material.mica() {
		alpha := float32(1)
		if translucentWindow(c, opacity) {
			alpha = float32(max(opacity, settings.MinWindowOpacity))
		}
		return material.tint(color, alpha)
	}
	if !translucentWindow(c, opacity) {
		return color
	}
	return color.Alpha(float32(max(opacity, settings.MinWindowOpacity)))
}

func windowContainer(c *ui.Context, opacity float64, color ui.Color) ui.Color {
	if materialOf(c).mica() || translucentWindow(c, opacity) {
		return ui.Transparent
	}
	return color
}

// windowSurface is the ground of a pane that is part of the window rather
// than a card in it: opaque under the material, which shows through it, as
// through the editors, so that the pane and what is beside it are one
// surface.
func windowSurface(c *ui.Context, opacity float64, color ui.Color) ui.Color {
	if materialOf(c).mica() {
		return color
	}
	return windowBackground(c, opacity, color)
}

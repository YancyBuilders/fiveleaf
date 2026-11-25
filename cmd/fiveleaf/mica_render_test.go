package main

import (
	"bytes"
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestMicaClockPausesWithoutJumping(t *testing.T) {
	var state micaState
	now := time.Unix(1000, 0)
	state.advance(now, false, 1)
	now = now.Add(motionMicaFrame)
	state.advance(now, false, 1)
	shown := state.elapsed
	if shown != motionMicaFrame {
		t.Fatalf("moving time = %v, want %v", shown, motionMicaFrame)
	}
	state.advance(now, false, 1)
	if state.elapsed != shown {
		t.Fatal("painting twice at the same time advanced the light")
	}
	state.advance(now, true, 1)
	now = now.Add(3 * motionMicaCycle)
	state.advance(now, true, 1)
	if state.moving || state.elapsed != shown {
		t.Fatal("the light moved while paused")
	}
	now = now.Add(motionMicaCycle / 2)
	state.advance(now, false, 1)
	if state.elapsed != shown {
		t.Fatal("resuming jumped across the time spent paused")
	}
	state.advance(now.Add(motionMicaFrame), false, 1)
	if state.elapsed != shown+motionMicaFrame {
		t.Fatal("the resumed light did not continue from its last position")
	}
	now = now.Add(motionMicaFrame)
	for left := motionMicaCycle; left > 0; {
		delta := min(left, motionMicaFrame)
		now = now.Add(delta)
		state.advance(now, false, 1)
		left -= delta
	}
	if state.elapsed != shown+motionMicaFrame {
		t.Fatal("the loop did not meet the same phase")
	}
}

func TestMicaClockResumesAfterMissingFrames(t *testing.T) {
	var state micaState
	now := time.Unix(1000, 0)
	state.advance(now, false, 1)
	now = now.Add(motionMicaFrame)
	state.advance(now, false, 1)
	shown := state.elapsed
	// Occluded native windows can skip every paint while hidden. The
	// first visible frame still says the window is moving.
	now = now.Add(motionMicaCycle / 3)
	state.advance(now, false, 1)
	if !state.moving || state.elapsed != shown+motionMicaMaxStep {
		t.Fatal("returning from a window with no frames did not bound its advance")
	}
	state.advance(now.Add(motionMicaFrame), false, 1)
	if state.elapsed != shown+motionMicaMaxStep+motionMicaFrame {
		t.Fatal("the light did not continue after missing frames")
	}
}

func TestMicaClockSlowFramesAndSpeed(t *testing.T) {
	var state micaState
	now := time.Unix(1000, 0)
	state.advance(now, false, 1)
	// Consistently slow frames must still move. The old gap rejection
	// could leave the field motionless for the entire interaction.
	for range 10 {
		now = now.Add(2 * motionMicaMaxStep)
		state.advance(now, false, 1)
	}
	if state.elapsed != 10*motionMicaMaxStep {
		t.Fatalf("slow frames advanced %v", state.elapsed)
	}
	shown := state.elapsed
	state.advance(now, false, 3)
	if state.elapsed != shown {
		t.Fatal("changing speed changed the existing phase")
	}
	state.advance(now.Add(motionMicaFrame), false, 3)
	if state.elapsed != shown+3*motionMicaFrame {
		t.Fatal("speed did not scale the next interval")
	}
}

func renderMica(p *palette, size image.Point, elapsed time.Duration, opacity float32) *image.RGBA {
	state := micaState{elapsed: elapsed}
	tester := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Draw(func(pt *ui.Painter, r ui.Rect) {
			state.draw(pt, r, p, micaOptions{hue: 177, intensity: 0.8, speed: 1}, opacity, true)
		})
	}, size.X, size.Y)
	return tester.Image()
}

func TestMicaFlowHoldsItsShapeInNarrowWindows(t *testing.T) {
	p := &palette{chrome: ui.Hex("#e7f0f2"), editor: ui.Hex("#f1f6f6")}
	wide := renderMica(p, image.Pt(160, 100), motionMicaCycle/5, 1)
	tall := renderMica(p, image.Pt(100, 160), motionMicaCycle/5, 1)
	for row := range 100 {
		for column := range 100 {
			if wide.RGBAAt(column+30, row) != tall.RGBAAt(column, row+30) {
				t.Fatalf("the same part of the current stretched at %d,%d", column, row)
			}
		}
	}
}

func TestMicaFlowHasNoCutAtLoopBoundary(t *testing.T) {
	p := &palette{chrome: ui.Hex("#111b25"), editor: ui.Hex("#161e29"), dark: true}
	size := image.Pt(160, 100)
	first := renderMica(p, size, 0, 1)
	seam := renderMica(p, size, motionMicaCycle, 1)
	if !bytes.Equal(first.Pix, seam.Pix) {
		t.Fatal("the current reset to a different image at the end of its cycle")
	}
	last := renderMica(p, size, motionMicaCycle-motionMicaFrame, 1)
	for index, channel := range first.Pix {
		if delta := abs(int(channel) - int(last.Pix[index])); delta > 5 {
			t.Fatalf("one frame across the loop changed a channel by %d", delta)
		}
	}
}

func TestMicaRenderHasMovingLight(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := &palette{chrome: ui.Hex("#e7f0f2"), editor: ui.Hex("#f1f6f6"), dark: dark}
		if dark {
			p.chrome, p.editor = ui.Hex("#111b25"), ui.Hex("#161e29")
		}
		size := image.Pt(160, 100)
		first, next := renderMica(p, size, 0, 1), renderMica(p, size, motionMicaCycle/12, 1)
		changed := 0
		for y := range first.Bounds().Dy() {
			for x := range first.Bounds().Dx() {
				before, after := first.RGBAAt(x, y), next.RGBAAt(x, y)
				if before.A != 255 || after.A != 255 {
					t.Fatalf("dark=%v: full-opacity background has a hole at %d,%d", dark, x, y)
				}
				if abs(int(before.R)-int(after.R))+abs(int(before.G)-int(after.G))+abs(int(before.B)-int(after.B)) >= 8 {
					changed++
				}
			}
		}
		if changed < first.Bounds().Dx()*first.Bounds().Dy()/4 {
			t.Errorf("dark=%v: moving the field changed only %d pixels", dark, changed)
		}
		for _, opacity := range []float32{0.35, 0.5, 0.8} {
			glass := renderMica(p, size, 0, opacity)
			want := int(opacity*255 + 0.5)
			for y := range glass.Bounds().Dy() {
				for x := range glass.Bounds().Dx() {
					pixel := glass.RGBAAt(x, y)
					if abs(int(pixel.A)-want) > 2 {
						t.Fatalf("dark=%v: translucent background alpha=%d at %d,%d, want %d", dark, pixel.A, x, y, want)
					}
					if max(pixel.R, pixel.G, pixel.B) > pixel.A {
						t.Fatalf("dark=%v: background is not premultiplied at %d,%d", dark, x, y)
					}
				}
			}
		}
	}
}

func TestMicaOptionsRepaintStaticField(t *testing.T) {
	var state micaState
	options := micaOptions{hue: 177, intensity: 0.8, speed: 1}
	p := &palette{chrome: ui.Hex("#21252b"), editor: ui.Hex("#282c34"), dark: true}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(pt *ui.Painter, r ui.Rect) {
			state.draw(pt, r, p, options, 1, true)
		})
	}, 160, 100)
	before := append([]byte(nil), tt.Image().Pix...)
	options.hue = 35
	tt.Frame()
	if bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("changing the hue left a cached background")
	}
	before = append(before[:0], tt.Image().Pix...)
	options.intensity = 0
	tt.Frame()
	if bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("changing intensity left a cached background")
	}
	if tt.FramePending() {
		t.Fatal("zero intensity kept scheduling animation")
	}
	// With no color, the renderer preserves exactly the theme's base.
	base := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(pt *ui.Painter, r ui.Rect) {
			pt.FillGradient(r, ui.LinearGradient{From: p.chrome, To: p.editor, Angle: 135}, 0)
		})
	}, 160, 100)
	if !bytes.Equal(base.Image().Pix, tt.Image().Pix) {
		t.Fatal("zero intensity did not restore the theme's base colors")
	}
}

func TestMicaColorHueWheel(t *testing.T) {
	for _, pair := range []struct {
		hue   float64
		color ui.Color
	}{{0, ui.RGB(255, 0, 0)}, {120, ui.RGB(0, 255, 0)}, {240, ui.RGB(0, 0, 255)}, {360, ui.RGB(255, 0, 0)}} {
		if got := micaColor(pair.hue, 1, 0.5); got != pair.color {
			t.Errorf("hue %v = %v, want %v", pair.hue, got, pair.color)
		}
	}
}

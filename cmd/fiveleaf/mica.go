package main

import (
	"image"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// micaState belongs to a window. Its clock counts only time spent moving,
// so returning to a window continues the light from where it was left.
type micaState struct {
	elapsed time.Duration
	last    time.Time
	moving  bool
	canvas  *ui.Canvas
	painted micaFrame
	flow    micaFlow
}

type micaFrame struct {
	elapsed        time.Duration
	chrome, editor ui.Color
	text, muted    ui.Color
	dark           bool
	hue, intensity float64
	frost          float32
	width, height  int
}

type micaOptions struct {
	hue, intensity, speed float64
	frost                 float32
}

func (s *micaState) advance(now time.Time, still bool, speed float64) {
	if !still && s.moving && !s.last.IsZero() {
		// A hidden window may miss the paint that would pause it. Bound
		// that gap, but still advance on slow frames instead of freezing.
		if delta := now.Sub(s.last); delta > 0 {
			delta = min(delta, motionMicaMaxStep)
			s.elapsed = (s.elapsed + time.Duration(float64(delta)*speed)) % motionMicaCycle
		}
	}
	s.last, s.moving = now, !still
}

func (s *micaState) draw(pt *ui.Painter, r ui.Rect, p *palette, options micaOptions, opacity float32, still bool) {
	now := pt.Now().Truncate(motionMicaFrame)
	opacity = clamp(opacity, 0, 1)
	if r.W <= 0 || r.H <= 0 || opacity == 0 {
		s.advance(now, true, options.speed)
		return
	}
	s.advance(now, still || options.intensity <= 0, options.speed)
	if s.moving {
		pt.AnimationFrameRate(float32(time.Second) / float32(motionMicaFrame))
	}
	// Compose the lights once at a modest resolution, then apply opacity
	// to the whole material. Fading each light separately would accumulate
	// alpha and make the window more opaque wherever lights overlap.
	scale := min(float32(1), 160/max(r.W, r.H))
	frame := micaFrame{
		elapsed: s.elapsed, chrome: p.chrome, editor: p.editor, text: p.text, muted: p.muted, dark: p.dark,
		hue: options.hue, intensity: options.intensity, frost: options.frost,
		width:  max(1, int(math.Ceil(float64(r.W*scale)))),
		height: max(1, int(math.Ceil(float64(r.H*scale)))),
	}
	if s.canvas == nil || s.painted != frame {
		if s.canvas == nil || s.painted.width != frame.width || s.painted.height != frame.height {
			s.canvas = ui.NewCanvas(frame.width, frame.height)
		}
		s.canvas.Draw(pt, func(cp *ui.Painter) {
			bounds := ui.Rect{W: float32(frame.width), H: float32(frame.height)}
			cp.FillGradient(bounds, ui.LinearGradient{From: p.chrome, To: p.editor, Angle: 135}, 0)
			if frame.intensity > 0 {
				cp.ImageRegion(s.flow.render(frame), ui.Rect{}, bounds, ui.ImageOptions{Tint: white.Alpha(float32(frame.intensity))})
			}
		})
		s.painted = frame
	}
	pt.ImageRegion(s.canvas.Bitmap(), ui.Rect{}, r, ui.ImageOptions{Tint: white.Alpha(opacity)})
}

type micaFlow struct {
	pixels *image.RGBA
	bitmap *ui.Bitmap
	look   micaFrame
	colors [256][4]byte
}

func (flow *micaFlow) render(frame micaFrame) *ui.Bitmap {
	bounds := image.Rect(0, 0, frame.width, frame.height)
	if flow.pixels == nil || flow.pixels.Rect != bounds {
		flow.pixels = image.NewRGBA(bounds)
	}
	look := frame
	look.elapsed = 0
	if flow.look != look {
		flow.look, flow.colors = look, micaFlowColors(look)
	}
	phase := 2 * math.Pi * float64(frame.elapsed) / float64(motionMicaCycle)
	driftX, driftY := math.Sincos(phase)
	spacing := 10.8 / float64(frame.width+frame.height)
	for row := range frame.height {
		vertical := (float64(row)+0.5-float64(frame.height)/2)*spacing + 0.32*driftY
		current := 1.2 * math.Sin(1.3*vertical+phase)
		pixels := flow.pixels.Pix[row*flow.pixels.Stride:][:frame.width*4]
		for column := range frame.width {
			horizontal := (float64(column)+0.5-float64(frame.width)/2)*spacing + 0.43*driftX
			bend := horizontal + current + 0.32*math.Cos(0.85*horizontal-0.64*vertical-2*phase)
			curl := vertical + 1.05*math.Sin(1.4*bend-phase+0.7)
			bend += 0.68 * math.Sin(1.8*curl+2*phase+1.2)
			curl += 0.42 * math.Cos(1.95*bend-phase)
			water := 0.5 + 0.31*math.Sin(1.24*bend+0.64*curl) + 0.19*math.Cos(1.17*curl-0.5*bend+phase)
			rim := max(0, 1-7*math.Abs(water-0.54))
			light := water + 0.17*rim*rim*(0.5+0.5*math.Cos(bend-curl))
			color := flow.colors[int(min(1, light)*255+0.5)]
			copy(pixels[column*4:column*4+4], color[:])
		}
	}
	if flow.bitmap == nil {
		flow.bitmap = ui.NewBitmap(flow.pixels)
	} else {
		flow.bitmap.Update(flow.pixels)
	}
	return flow.bitmap
}

func micaFlowColors(frame micaFrame) [256][4]byte {
	var pool, wash, bloom ui.Color
	if frame.dark {
		pool = frame.editor.Mix(micaColor(frame.hue+18, 0.45, 0.045), 0.75)
		wash = micaTone(frame.hue+18, 0.65, 0.115)
		bloom = micaTone(frame.hue, 0.78, 0.18)
	} else {
		pool = frame.editor.Mix(white, 0.75)
		wash = micaTone(frame.hue+18, 0.45, 0.84)
		bloom = micaTone(frame.hue, 0.66, 0.50)
	}
	pool, wash, bloom = frame.readableTone(pool), frame.readableTone(wash), frame.readableTone(bloom)
	var colors [256][4]byte
	blend := func(from, to, level float32) float32 {
		weight := clamp((level-from)/(to-from), 0, 1)
		return weight * weight * (3 - 2*weight)
	}
	for index := range colors {
		level := float32(index) / 255
		color := pool.Mix(wash, blend(0, 0.78, level)).Mix(bloom, blend(0.36, 1, level))
		colors[index] = [4]byte{color.R, color.G, color.B, 255}
	}
	return colors
}

func (frame micaFrame) readableTone(tone ui.Color) ui.Color {
	if frame.text.A == 0 || frame.muted.A == 0 {
		return tone
	}
	exposure := float32(frame.intensity) * (1 - frame.frost)
	reads := func(color ui.Color) bool {
		for _, base := range [...]ui.Color{frame.chrome, frame.editor} {
			ground := base.Mix(color, exposure)
			if contrast(ground, frame.muted) < 4.55 || contrast(ground, frame.text) < 4.55 {
				return false
			}
		}
		return true
	}
	if reads(tone) {
		return tone
	}
	limit := white
	if frame.dark {
		limit = black
	}
	low, high := float32(0), float32(1)
	for range 8 {
		mid := (low + high) / 2
		if reads(tone.Mix(limit, mid)) {
			high = mid
		} else {
			low = mid
		}
	}
	return tone.Mix(limit, high)
}

// Match each hue's brightness to sea green, so a yellow tint does not
// brighten dark surfaces and blue does not darken light ones.
func micaTone(hue, saturation, lightness float64) ui.Color {
	c := micaColor(hue, saturation, lightness)
	target := contrast(micaColor(177, saturation, lightness), black)
	brighter := contrast(c, black) > target
	to := white
	if brighter {
		to = black
	}
	lo, hi := float32(0), float32(1)
	for range 8 {
		mid := (lo + hi) / 2
		if (contrast(c.Mix(to, mid), black) > target) == brighter {
			lo = mid
		} else {
			hi = mid
		}
	}
	return c.Mix(to, hi)
}

// micaColor uses the same hue wheel as the setting, with lightness kept in
// separate ranges for light and dark themes.
func micaColor(hue, saturation, lightness float64) ui.Color {
	hue = math.Mod(math.Mod(hue, 360)+360, 360) / 30
	a := saturation * min(lightness, 1-lightness)
	channel := func(offset float64) uint8 {
		k := math.Mod(offset+hue, 12)
		v := lightness - a*max(-1, min(k-3, 9-k, 1))
		return uint8(math.Round(v * 255))
	}
	return ui.RGB(channel(0), channel(8), channel(4))
}

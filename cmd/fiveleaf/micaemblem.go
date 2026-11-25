package main

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"fiveleaf/internal/settings"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

const micaEmblemSize = 440

type emblemParticle struct {
	x, y, startX, startY float64
	shade                float64
}

var emblemParticles = sync.OnceValue(func() []emblemParticle {
	art, err := png.Decode(bytes.NewReader(iconDarkArt))
	if err != nil {
		panic(err)
	}
	const grid = 96
	mask := image.NewGray(image.Rect(0, 0, grid, grid))
	draw.BiLinear.Scale(mask, mask.Bounds(), art, art.Bounds(), draw.Src, nil)
	noise := rand.New(rand.NewPCG(12, 57))
	var particles []emblemParticle
	for row := range grid {
		for column := range grid {
			coverage := min(1, (float64(mask.GrayAt(column, row).Y)-32)/146)
			if coverage < 0.10 {
				continue
			}
			angle := noise.Float64() * 2 * math.Pi
			radius := 0.06 + 0.38*math.Sqrt(noise.Float64())
			particles = append(particles, emblemParticle{
				x:      (float64(column) + 0.5 + 0.20*(noise.Float64()-0.5) - grid/2) / grid,
				y:      (float64(row) + 0.5 + 0.20*(noise.Float64()-0.5) - grid/2) / grid,
				startX: radius * math.Cos(angle), startY: radius * math.Sin(angle),
				shade: coverage * (0.68 + 0.32*noise.Float64()),
			})
		}
	}
	return particles
})

type emblemPose struct {
	gather, drift time.Duration
	pointer       ui.Point
	presence      float32
}

type micaEmblem struct {
	last    time.Time
	moving  bool
	pose    emblemPose
	canvas  *ui.Canvas
	painted emblemPose
}

func micaChatViewport(c *ui.Context, reading *ui.Handle, palette *palette, prefs *settings.Prefs, content func()) {
	pane := ui.Column(c.Key("transcript-pane")).FillWidth().Grow(1).MinWidth(0).MinHeight(0)
	attachMicaEmblem(c, pane, reading, palette, prefs)
	pane.Children(func() {
		foreground := ui.Column(c.Key("transcript-foreground")).Fill().MinWidth(0).MinHeight(0)
		if palette.dark && prefs.MicaEnabled && prefs.MicaIntensity > 0 {
			foreground.Cache().Effect(emblemTextHalo{})
		}
		foreground.Children(content)
	})
}

func micaEmblemBounds(rect ui.Rect) ui.Rect {
	size := min(float32(micaEmblemSize), rect.W*0.82, rect.H*0.84)
	return ui.Rect{X: rect.X + (rect.W-size)/2, Y: rect.Y + (rect.H-size)/2, W: size, H: size}
}

type emblemTextHalo struct{}

func (emblemTextHalo) EffectMargin() ui.Point { return ui.Point{} }

func (emblemTextHalo) PaintEffect(pt *ui.Painter, rect ui.Rect, content *ui.Bitmap, inset ui.Point) {
	scale := pt.Scale()
	width, height := content.Size()
	area := ui.Rect{X: rect.X - inset.X/scale, Y: rect.Y - inset.Y/scale, W: float32(width) / scale, H: float32(height) / scale}
	pt.Clip(micaEmblemBounds(rect), 0, func() {
		for _, offset := range [...]ui.Point{{X: -1}, {X: 1}, {Y: -1}, {Y: 1}} {
			shadow := area
			shadow.X += offset.X
			shadow.Y += offset.Y
			pt.ImageRegion(content, ui.Rect{}, shadow, ui.ImageOptions{Tint: black.Alpha(0.8)})
		}
	})
	pt.ImageRegion(content, ui.Rect{}, area, ui.ImageOptions{})
}

func attachMicaEmblem(c *ui.Context, pane ui.Element, reading *ui.Handle, palette *palette, prefs *settings.Prefs) {
	if !prefs.MicaEnabled || prefs.MicaIntensity <= 0 {
		return
	}
	emblem := ui.Local(pane, "mica-emblem", func() micaEmblem { return micaEmblem{} })
	still := !prefs.MicaAnimation || c.Preferences().ReduceMotion
	strength := float32(prefs.MicaIntensity)
	pigment := ui.RGB(91, 94, 100)
	if palette.dark {
		pigment = ui.RGB(219, 224, 230)
	}
	ink := pigment.Alpha(strength * (1 - 0.4*float32(prefs.MicaFrost)))
	readingInk := ink.Alpha(0.72)
	if !palette.dark {
		readingInk = legible(pigment, true, palette.muted, palette.text).Alpha(strength)
	}
	speed := prefs.MicaSpeed
	window := c.Root()
	pane.Draw(func(pt *ui.Painter, rect ui.Rect) {
		bounds := micaEmblemBounds(rect)
		size := bounds.W
		if size < 1 {
			return
		}
		var target ui.Point
		var over bool
		if !still {
			pointerX, pointerY, inside := window.PointerPosition()
			target = ui.Point{X: (pointerX-bounds.X)/size - 0.5, Y: (pointerY-bounds.Y)/size - 0.5}
			over = inside
		}
		emblem.advance(pt.Now().Truncate(motionMicaFrame), still, speed, target, over)
		if emblem.moving {
			pt.AnimationFrameRate(float32(time.Second) / float32(motionMicaFrame))
		}
		color := ink
		if content := reading.Bounds(c); content.H > 0 {
			clearance := max(bounds.Y-content.Y-content.H, content.Y-bounds.Y-bounds.H)
			color = readingInk.Mix(ink, clamp(clearance/min(10, size*0.12), 0, 1))
		}
		if emblem.canvas == nil || emblem.painted != emblem.pose {
			if emblem.canvas == nil {
				emblem.canvas = ui.NewCanvas(micaEmblemSize, micaEmblemSize)
			}
			emblem.canvas.Draw(pt, emblem.pose.paint)
			emblem.painted = emblem.pose
		}
		pt.ImageRegion(emblem.canvas.Bitmap(), ui.Rect{}, bounds, ui.ImageOptions{Tint: color})
	})
}

func (emblem *micaEmblem) advance(now time.Time, still bool, speed float64, target ui.Point, over bool) {
	delta := min(max(now.Sub(emblem.last), 0), motionMicaMaxStep)
	wasMoving := emblem.moving
	emblem.last, emblem.moving = now, !still
	pose := &emblem.pose
	if still {
		pose.gather = motionEmblemGather
		return
	}
	if !wasMoving || delta == 0 {
		return
	}
	pose.gather = min(motionEmblemGather, pose.gather+delta)
	pose.drift = (pose.drift + time.Duration(float64(delta)*speed)) % motionEmblemDrift
	weight := float32(-math.Expm1(-float64(delta) / float64(motionEmblemFollow)))
	presence := float32(0)
	if over {
		presence = 1
		pose.pointer.X += (target.X - pose.pointer.X) * weight
		pose.pointer.Y += (target.Y - pose.pointer.Y) * weight
	}
	pose.presence += (presence - pose.presence) * weight
}

func (pose emblemPose) paint(pt *ui.Painter) {
	gather := float64(ui.EaseOut(float32(pose.gather) / float32(motionEmblemGather)))
	phase := 2 * math.Pi * float64(pose.drift) / float64(motionEmblemDrift)
	lightX := 0.27 + 1.05*float64(clamp(pose.pointer.X, -1, 1))
	breath := 0.009 * math.Sin(phase)
	for index, dot := range emblemParticles() {
		wave := 0.009 * math.Sin(phase+5*dot.x-3*dot.y)
		horizontal := dot.startX*(1-gather) + (dot.x-wave*dot.y)*gather
		vertical := breath + dot.startY*(1-gather) + (dot.y+wave*dot.x)*gather
		awayX, awayY := horizontal-float64(pose.pointer.X), vertical-float64(pose.pointer.Y)
		distance := math.Hypot(awayX, awayY)
		highlight := 0.0
		if distance < 0.30 && pose.presence > 0 && gather > 0.8 {
			falloff := 1 - distance/0.30
			highlight = falloff * falloff * float64(pose.presence) * (gather - 0.8) * 5
			push := 0.15 * falloff * highlight
			sine, cosine := math.Sincos(5 * math.Sin(0.37*float64(index)+phase))
			if distance > 0.0001 {
				awayX, awayY = awayX/distance, awayY/distance
			} else {
				awayX, awayY = 1, 0
			}
			horizontal += (cosine*awayX - sine*awayY) * push
			vertical += (sine*awayX + cosine*awayY) * push
		}
		light := max(0, 1-math.Hypot(horizontal-lightX, vertical+0.33)/0.84)
		alpha := float32(math.Sqrt(dot.shade) * min(1, 0.52+0.48*light*light+0.32*highlight) * (0.50 + 0.50*gather))
		side := float32(2 + 1.4*dot.shade)
		pt.Fill(ui.Rect{X: micaEmblemSize/2 + float32(horizontal)*micaEmblemSize - side/2,
			Y: micaEmblemSize/2 + float32(vertical)*micaEmblemSize - side/2, W: side, H: side}, white.Alpha(alpha), side/2)
	}
}

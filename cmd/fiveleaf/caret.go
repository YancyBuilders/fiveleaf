package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// caretGlide is a caret that goes to a new place over motionCaret, as
// Neovide's does, rather than showing there at once. A move starts from
// where it showed, not from where it was headed, so a key that repeats
// turns the caret back from where the eye sees it.
//
// The places are in the text area's content, as TextLines.Caret returns
// them: a scroll moves the caret on screen without starting a glide.
type caretGlide struct {
	from, to ui.Rect
	start    time.Time
	duration time.Duration
	// glide is whether the move eases, as it does for a key, or is there
	// at once, as for a scroll or Reduce Motion.
	glide bool
	ok    bool
}

// follow starts the caret toward box at now, from where it shows, or puts
// it there at once when motion is disabled. A distant move has more time
// to travel than a nearby move.
func (g *caretGlide) follow(box ui.Rect, now time.Time, glide bool) {
	if !g.ok {
		g.from, g.to, g.start, g.glide, g.ok = box, box, now, false, true
		g.duration = motionCaret
		return
	}
	if box == g.to && glide {
		return
	}
	shown := g.position(now)
	glide = glide && glides(g.to, box) && shown != box
	g.duration = motionCaret
	if math.Abs(float64(shown.Y-box.Y)) > float64(3*shown.H) || math.Abs(float64(shown.X-box.X)) > 240 {
		g.duration = motionCaretJump
	}
	if glide {
		g.from = shown
	} else {
		g.from = box
	}
	g.to, g.start, g.glide = box, now, glide
}

// position returns where the caret shows at now, between the place it came
// from and where it goes, easing out.
func (g *caretGlide) position(now time.Time) ui.Rect {
	if !g.ok {
		return ui.Rect{}
	}
	since := now.Sub(g.start)
	if !g.glide || since < 0 || since >= g.duration {
		return g.to
	}
	return lerpRect(g.from, g.to, ui.EaseOut(float32(since)/float32(g.duration)))
}

// moving reports whether the caret is on its way at now.
func (g *caretGlide) moving(now time.Time) bool {
	return g.ok && g.glide && now.Sub(g.start) < g.duration
}

// glides reports whether the caret has valid places to travel between.
// A place without a height is a new layout, which the caret goes to at
// once.
func glides(r, q ui.Rect) bool {
	if r.H <= 0 || q.H <= 0 {
		return false
	}
	dy, dx := r.Y-q.Y, r.X-q.X
	if dy < 0 {
		dy = -dy
	}
	if dx < 0 {
		dx = -dx
	}
	distance := float64(dx) + float64(dy) + float64(r.H) + float64(q.H)
	return !math.IsNaN(distance) && !math.IsInf(distance, 0)
}

// lerpRect returns the rectangle k of the way from r to q.
func lerpRect(r, q ui.Rect, k float32) ui.Rect {
	return ui.Rect{
		X: r.X + (q.X-r.X)*k,
		Y: r.Y + (q.Y-r.Y)*k,
		W: r.W + (q.W-r.W)*k,
		H: r.H + (q.H-r.H)*k,
	}
}

// caretAt is where a caret at rune offset i shows in the text area's
// content, as wide as the line is drawn.
func caretAt(lines *ui.TextLines, i int, width float32) (ui.Rect, bool) {
	x, y, h, ok := lines.Caret(i)
	if !ok || h <= 0 || math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) ||
		math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) {
		return ui.Rect{}, false
	}
	return ui.Rect{X: x, Y: y, W: width, H: h}, true
}

type caretPainter struct {
	// glide is where the caret goes, in the text area's content.
	glide caretGlide
	// frame is the frame painted in: one frame may paint the area more
	// than once, as it animates, and the caret moves then too.
	frame int
	snap  bool
	trail caretTrail
	path  ui.Path
}

func (p *caretPainter) reset() {
	p.glide, p.trail = caretGlide{}, caretTrail{}
	p.frame, p.snap = 0, false
	p.path.Reset()
}

func (p *caretPainter) follow(box ui.Rect, now time.Time, glide bool, frame int) ui.Rect {
	snap := !p.glide.ok || p.snap || !glide || !glides(p.glide.to, box)
	if !snap {
		p.trail.advance(p.glide, now)
	}
	if !p.glide.ok || frame != p.frame || box != p.glide.to || p.snap || !glide {
		p.frame = frame
		p.glide.follow(box, now, glide && !p.snap)
		p.snap = false
	}
	shown := p.glide.position(now)
	if snap {
		p.trail.reset(shown, now)
	}
	return shown
}

type caretTrail struct {
	points [8]ui.Point
	at     time.Time
	ok     bool
}

const (
	caretTrailHorizontal = 0.50
	caretTrailVertical   = 0.45
)

func caretTrailHead(box ui.Rect) ui.Point {
	return ui.Point{X: box.X + box.W/2, Y: box.Y}
}

func (trail *caretTrail) reset(box ui.Rect, now time.Time) {
	for index := range trail.points {
		trail.points[index] = caretTrailHead(box)
	}
	trail.at, trail.ok = now, true
}

func stepCaretTrail(points *[8]ui.Point, head ui.Point, horizontal, vertical float32) {
	points[0] = head
	for index := 1; index < len(points); index++ {
		previous := points[index-1]
		points[index].X = previous.X + (points[index].X-previous.X)*horizontal
		points[index].Y = previous.Y + (points[index].Y-previous.Y)*vertical
	}
}

func (trail *caretTrail) advance(glide caretGlide, now time.Time) {
	finished := glide.start
	if glide.glide {
		finished = finished.Add(glide.duration)
	}
	if !trail.ok || now.Before(trail.at) || !now.Before(finished.Add(motionCaretTrail)) {
		trail.reset(glide.position(now), now)
		return
	}
	for next := trail.at.Add(motionCaretStep); !next.After(now); next = trail.at.Add(motionCaretStep) {
		stepCaretTrail(&trail.points, caretTrailHead(glide.position(next)), caretTrailHorizontal, caretTrailVertical)
		trail.at = next
	}
}

func (trail *caretTrail) positions(shown ui.Rect, now time.Time) [8]ui.Point {
	points := trail.points
	head := caretTrailHead(shown)
	fraction := float64(now.Sub(trail.at)) / float64(motionCaretStep)
	if fraction > 0 {
		stepCaretTrail(&points, head, float32(math.Pow(caretTrailHorizontal, fraction)), float32(math.Pow(caretTrailVertical, fraction)))
	} else {
		points[0] = head
	}
	return points
}

func (p *caretPainter) paintTrail(pt *ui.Painter, now time.Time, shown ui.Rect, ox, oy float32, col ui.Color, block bool) bool {
	points := p.trail.positions(shown, now)
	moving := false
	for _, point := range points[1:] {
		if math.Abs(float64(point.X-points[0].X)) > 0.25 || math.Abs(float64(point.Y-points[0].Y)) > 0.25 {
			moving = true
			break
		}
	}
	if !moving {
		return false
	}
	width := min(shown.W, shown.H)
	col = col.Alpha(0.75)
	p.path.Reset()
	p.path.Join(ui.RoundJoin)
	p.path.Cap(ui.ButtCap)
	if block {
		for band := range 4 {
			offset := float32(band)*(shown.H-width)/3 + width/2
			p.path.MoveTo(ox+points[0].X, oy+points[0].Y+offset)
			for _, point := range points[1:] {
				p.path.LineTo(ox+point.X, oy+point.Y+offset)
			}
		}
		pt.StrokePath(&p.path, width, col)
		return true
	}
	p.path.MoveTo(ox+points[0].X, oy+points[0].Y)
	for _, point := range points[1:] {
		p.path.LineTo(ox+point.X, oy+point.Y)
	}
	for index := len(points) - 1; index >= 0; index-- {
		p.path.LineTo(ox+points[index].X, oy+points[index].Y+shown.H)
	}
	p.path.Close()
	pt.FillPath(&p.path, col)
	p.path.Reset()
	p.path.Cap(ui.ButtCap)
	offset := (shown.H - shown.W) / 2
	p.path.MoveTo(ox+points[0].X, oy+points[0].Y+offset)
	for _, point := range points[1:] {
		p.path.LineTo(ox+point.X, oy+point.Y+offset)
	}
	pt.StrokePath(&p.path, width, col)
	return true
}

func (p *caretPainter) paint(pt *ui.Painter, now time.Time, lines *ui.TextLines, at int, width, ox, oy float32,
	col ui.Color, glide bool, frame int) bool {
	box, ok := caretAt(lines, at, width)
	if !ok {
		p.reset()
		return false
	}
	shown := p.follow(box, now, glide, frame)
	trailing := p.paintTrail(pt, now, shown, ox, oy, col, false)
	box = ui.Rect{X: ox + shown.X, Y: oy + shown.Y, W: shown.W, H: shown.H}
	pt.Fill(box, col, 0)
	if trailing || p.glide.moving(now) {
		pt.AnimationFrame()
		return true
	}
	return false
}

func (p *caretPainter) block(pt *ui.Painter, box ui.Rect, ox, oy float32, col ui.Color, glide, focused bool, frame int) {
	shown := p.follow(box, pt.Now(), glide && focused, frame)
	trailing := p.paintTrail(pt, pt.Now(), shown, ox, oy, col, true)
	shown.X += ox
	shown.Y += oy
	if focused {
		pt.Fill(shown, col, 1)
	} else {
		pt.Stroke(shown, col, 1, 1)
	}
	if trailing || p.glide.moving(pt.Now()) {
		pt.AnimationFrame()
	}
}

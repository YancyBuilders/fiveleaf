package main

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func caretFollowSamples(rate int) [][8]ui.Point {
	start := time.Unix(1000, 0)
	from := ui.Rect{X: 10, Y: 20, W: 2, H: 18}
	target := ui.Rect{X: 130, Y: 68, W: 2, H: 18}
	var painter caretPainter
	painter.follow(from, start, true, 1)
	painter.follow(target, start, true, 2)
	var samples [][8]ui.Point
	nextFrame, frame := 1, 3
	for _, elapsed := range []time.Duration{motionCaret / 3, motionCaret * 2 / 3, motionCaret, motionCaret * 4 / 3} {
		for time.Second*time.Duration(nextFrame)/time.Duration(rate) < elapsed {
			at := start.Add(time.Second * time.Duration(nextFrame) / time.Duration(rate))
			painter.follow(target, at, true, frame)
			nextFrame++
			frame++
		}
		at := start.Add(elapsed)
		shown := painter.follow(target, at, true, frame)
		samples = append(samples, painter.trail.positions(shown, at))
		frame++
	}
	return samples
}

func TestCaretFollowRefreshRate(t *testing.T) {
	reference := caretFollowSamples(60)
	for _, rate := range []int{30, 120} {
		actual := caretFollowSamples(rate)
		for sampleIndex, sample := range actual {
			for particleIndex, point := range sample {
				expected := reference[sampleIndex][particleIndex]
				if math.Abs(float64(point.X-expected.X)) > 0.001 || math.Abs(float64(point.Y-expected.Y)) > 0.001 {
					t.Fatalf("%dHz changed particle %d at sample %d: %v versus %v", rate, particleIndex, sampleIndex, point, expected)
				}
			}
		}
	}
}

func TestCaretFollowSameTimeFrames(t *testing.T) {
	start := time.Unix(1000, 0)
	var painter caretPainter
	painter.follow(ui.Rect{X: 10, Y: 20, W: 2, H: 18}, start, true, 1)
	target := ui.Rect{X: 130, Y: 68, W: 2, H: 18}
	painter.follow(target, start, true, 2)
	at := start.Add(motionCaret / 2)
	shown := painter.follow(target, at, true, 3)
	before := painter.trail.positions(shown, at)
	for frame := 3; frame < 40; frame++ {
		shown = painter.follow(target, at, true, frame)
		if after := painter.trail.positions(shown, at); after != before {
			t.Fatalf("same-time frame %d advanced particles from %v to %v", frame, before, after)
		}
	}
}

func TestCaretFollowTurnsAndConverges(t *testing.T) {
	start := time.Unix(1000, 0)
	var painter caretPainter
	painter.follow(ui.Rect{X: 10, Y: 20, W: 2, H: 18}, start, true, 1)
	target := ui.Rect{X: 130, Y: 20, W: 2, H: 18}
	painter.follow(target, start, true, 2)
	at := start.Add(motionCaret / 2)
	shown := painter.follow(target, at, true, 3)
	before := painter.trail.positions(shown, at)
	corner := ui.Rect{X: 130, Y: 56, W: 2, H: 18}
	if turned := painter.follow(corner, at, true, 4); turned != shown {
		t.Fatalf("the corner jumped the visible caret from %v to %v", shown, turned)
	}
	if after := painter.trail.positions(shown, at); after != before {
		t.Fatal("the corner reset or advanced the trailing particles")
	}
	at = at.Add(motionCaret / 2)
	shown = painter.follow(corner, at, true, 5)
	before = painter.trail.positions(shown, at)
	back := ui.Rect{X: 20, Y: 56, W: 2, H: 18}
	if turned := painter.follow(back, at, true, 6); turned != shown {
		t.Fatalf("reversing jumped the visible caret from %v to %v", shown, turned)
	}
	if after := painter.trail.positions(shown, at); after != before {
		t.Fatal("reversing reset or advanced the trailing particles")
	}
	at = at.Add(motionCaret + motionCaretTrail)
	shown = painter.follow(back, at, true, 7)
	for index, point := range painter.trail.positions(shown, at) {
		if math.Abs(float64(point.X-(shown.X+shown.W/2))) > 0.1 || math.Abs(float64(point.Y-shown.Y)) > 0.1 {
			t.Fatalf("particle %d did not converge after the caret stopped: %v at %v", index, point, shown)
		}
	}
}

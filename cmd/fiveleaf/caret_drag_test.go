package main

import (
	"image"
	"math"
	"testing"
	"time"
)

func TestCaretDistantDragKeepsJump(t *testing.T) {
	for _, mode := range []string{"source", "live"} {
		for _, delay := range []struct {
			name string
			gap  time.Duration
		}{
			{name: "same frame"},
			{name: "next frame", gap: motionCaretStep},
		} {
			t.Run(mode+"/"+delay.name, func(t *testing.T) {
				hs, editor := caretClickHarness(t, mode)
				origin := caretTrailOrigin(t, hs, editor)
				start := editor.caretPaint.glide.position(hs.tt.Now())
				lineStart, _ := editor.buf.Snapshot().LineRange(9)
				anchor, caret := lineStart+8, lineStart+20
				pressX, pressY, pressH, pressOK := editor.lines.Caret(anchor)
				moveX, moveY, moveH, moveOK := editor.lines.Caret(caret)
				if !pressOK || !moveOK {
					t.Fatal("the tenth line's drag endpoints are not laid out")
				}
				hs.tt.Press(origin.X+pressX, origin.Y+pressY+pressH/2)
				if editor.sel != [2]int{anchor, anchor} || !editor.caretPaint.glide.moving(hs.tt.Now()) {
					t.Fatalf("pressing the tenth line did not immediately place its logical cursor and start the jump: selection=%v glide=%+v", editor.sel, editor.caretPaint.glide)
				}
				if delay.gap > 0 {
					hs.tt.Advance(delay.gap)
					hs.tt.Frame()
				}
				beforeDrag := editor.caretPaint.glide.position(hs.tt.Now())
				hs.tt.Move(origin.X+moveX, origin.Y+moveY+moveH/2)
				if editor.sel != [2]int{anchor, caret} {
					t.Fatalf("dragging the tenth line did not immediately select its text: got=%v want=%v", editor.sel, [2]int{anchor, caret})
				}
				if shown := editor.caretPaint.glide.position(hs.tt.Now()); shown != beforeDrag {
					t.Fatalf("starting the drag interrupted the first-to-tenth-line glide: before=%v shown=%v target=%v", beforeDrag, shown, editor.caretPaint.glide.to)
				}
				hs.tt.Advance(motionCaretJump / 3)
				hs.tt.Frame()
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				target := editor.caretPaint.glide.to
				if !editor.caretPaint.glide.moving(hs.tt.Now()) || shown.Y <= start.Y || shown.Y >= target.Y {
					t.Fatalf("the distant drag has no intermediate caret: start=%v shown=%v target=%v", start, shown, target)
				}
				points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
				moving := caretTrailSnapshot(hs)
				hs.tt.Release(origin.X+moveX, origin.Y+moveY+moveH/2)
				if after := editor.caretPaint.glide.position(hs.tt.Now()); after != shown {
					t.Fatalf("releasing the drag jumped its in-flight caret: before=%v after=%v", shown, after)
				}
				hs.tt.Advance(motionCaretJump + motionCaretTrail)
				hs.tt.Frame()
				if editor.sel != [2]int{anchor, caret} || editor.caretPaint.glide.moving(hs.tt.Now()) || editor.caretPaint.glide.position(hs.tt.Now()) != target {
					t.Fatalf("the released drag did not settle at its selection endpoint: selection=%v shown=%v target=%v", editor.sel, editor.caretPaint.glide.position(hs.tt.Now()), target)
				}
				settled := hs.tt.Image()
				primary := image.Rect(int(math.Floor(float64(origin.X+shown.X))), int(origin.Y+shown.Y+shown.H/3),
					int(math.Ceil(float64(origin.X+shown.X+shown.W))), int(origin.Y+shown.Y+shown.H*2/3))
				if count, _ := caretTrailChanged(moving, settled, primary); count == 0 {
					t.Fatal("the distant drag's intermediate caret is not rendered")
				}
				last := points[len(points)-1]
				left := int(math.Floor(float64(origin.X + min(last.X, points[0].X) - shown.W - 2)))
				right := int(math.Ceil(float64(origin.X + max(last.X, points[0].X) + shown.W + 2)))
				from := int(math.Ceil(float64(origin.Y + last.Y + shown.H + 2)))
				to := int(math.Floor(float64(origin.Y + shown.Y - 2)))
				if to-from < 5 {
					t.Fatalf("the distant drag has no measurable trailing path: %v", points)
				}
				for row := from; row < to; row++ {
					if count, _ := caretTrailChanged(moving, settled, image.Rect(left, row, right, row+1)); count == 0 {
						t.Fatalf("the distant drag's rendered trail has a gap at row %d", row)
					}
				}
				if hs.tt.FramePending() {
					t.Fatal("the distant drag keeps requesting frames after convergence")
				}
			})
		}
	}
}

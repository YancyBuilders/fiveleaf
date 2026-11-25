package main

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func caretClickHarness(t *testing.T, mode string) (*harness, *editor) {
	t.Helper()
	hs, editor := editHarness(t, "click.md", strings.Repeat("a"+strings.Repeat(" ", 40)+"z\n", 40))
	if strings.Contains(mode, "vim") {
		hs.a.settings.Vim = true
	}
	if strings.Contains(mode, "helix") {
		hs.a.settings.EditorMode = "helix"
		hs.a.settings.Vim = true
	}
	if strings.Contains(mode, "live") {
		hs.a.setLive(editor, true)
	}
	hs.a.settings.SmoothCaret = true
	hs.place(editor, 8, 8)
	if hs.a.helixOn() {
		helixSelect(editor, 8, 8)
	}
	editor.vscroll.Focus()
	hs.settle()
	return hs, editor
}

func caretClickLine(t *testing.T, hs *harness, editor *editor, line int) {
	t.Helper()
	start, _ := editor.buf.Snapshot().LineRange(line)
	x, y, height, ok := editor.lines.Caret(start + 8)
	if !ok {
		t.Fatalf("line %d is not laid out", line+1)
	}
	origin := caretTrailOrigin(t, hs, editor)
	hs.tt.ClickAt(origin.X+x, origin.Y+y+height/2)
	hs.tt.Frame()
	if actual := editor.buf.Snapshot().LineAt(editor.sel[1]); actual != line {
		t.Fatalf("the click did not immediately select line %d: selection=%v line=%d", line+1, editor.sel, actual+1)
	}
}

func TestCaretMouseJumpFrames(t *testing.T) {
	for _, mode := range []string{"source", "vim", "helix", "live", "live vim", "live helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, editor := caretClickHarness(t, mode)
			origin := caretTrailOrigin(t, hs, editor)
			start := editor.caretPaint.glide.position(hs.tt.Now())
			caretClickLine(t, hs, editor, 9)
			if editor.vscroll.Y != 0 {
				t.Fatal("the visible line click unexpectedly scrolled")
			}
			hs.tt.Advance(motionCaretJump / 3)
			hs.tt.Frame()
			shown := editor.caretPaint.glide.position(hs.tt.Now())
			target := editor.caretPaint.glide.to
			if !editor.caretPaint.glide.moving(hs.tt.Now()) || shown.Y <= start.Y || shown.Y >= target.Y {
				t.Fatalf("the mouse jump has no intermediate caret: start=%v shown=%v target=%v", start, shown, target)
			}
			points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
			moving := caretTrailSnapshot(hs)
			targetColumn := int(origin.X + target.X + max(1, target.W/2))
			targetRow := int(origin.Y + target.Y + target.H/5)
			background := moving.RGBAAt(targetColumn+int(hs.a.font.charW*2), targetRow)
			if caretTrailDifference(moving.RGBAAt(targetColumn, targetRow), background) > 3 {
				t.Fatal("the destination still paints a static caret or selected block during the jump")
			}
			hs.tt.Advance(motionCaretJump + motionCaretTrail)
			hs.tt.Frame()
			settled := hs.tt.Image()
			primary := image.Rect(int(math.Floor(float64(origin.X+shown.X))), int(origin.Y+shown.Y+shown.H/3),
				int(math.Ceil(float64(origin.X+shown.X+shown.W))), int(origin.Y+shown.Y+shown.H*2/3))
			if count, _ := caretTrailChanged(moving, settled, primary); count == 0 {
				t.Fatal("the intermediate mouse caret is not rendered")
			}
			last := points[len(points)-1]
			left := int(math.Floor(float64(origin.X + min(last.X, points[0].X) - shown.W - 2)))
			right := int(math.Ceil(float64(origin.X + max(last.X, points[0].X) + shown.W + 2)))
			from := int(math.Ceil(float64(origin.Y + last.Y + shown.H + 2)))
			to := int(math.Floor(float64(origin.Y + shown.Y - 2)))
			if to-from < 5 {
				t.Fatalf("the distant click has no measurable trailing path: particles=%v", points)
			}
			for row := from; row < to; row++ {
				band := image.Rect(left, row, right, row+1)
				if count, _ := caretTrailChanged(moving, settled, band); count == 0 {
					t.Fatalf("the mouse jump has a gap in its rendered trail at row %d", row)
				}
			}
			if hs.tt.FramePending() {
				t.Fatal("the mouse jump did not stop requesting frames after convergence")
			}
		})
	}
}

func TestCaretMouseJumpReverses(t *testing.T) {
	for _, mode := range []string{"source", "vim", "helix", "live", "live vim", "live helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, editor := caretClickHarness(t, mode)
			caretClickLine(t, hs, editor, 9)
			hs.tt.Advance(motionCaretJump / 3)
			hs.tt.Frame()
			shown := editor.caretPaint.glide.position(hs.tt.Now())
			points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
			caretClickLine(t, hs, editor, 0)
			if after := editor.caretPaint.glide.position(hs.tt.Now()); after != shown {
				t.Fatalf("the reverse click jumped from %v to %v", shown, after)
			}
			if after := editor.caretPaint.trail.positions(shown, hs.tt.Now()); after != points {
				t.Fatal("the reverse click reset the trailing path")
			}
			hs.tt.Advance(motionCaretJump / 3)
			hs.tt.Frame()
			if after := editor.caretPaint.glide.position(hs.tt.Now()); after.Y >= shown.Y || after.Y <= editor.caretPaint.glide.to.Y {
				t.Fatalf("the reverse click is not returning smoothly: %v from %v", after, shown)
			}
		})
	}
}

func TestCaretMouseJumpPreferences(t *testing.T) {
	for _, mode := range []string{"source", "vim", "helix", "live", "live vim", "live helix"} {
		for _, option := range []string{"disabled", "reduced motion"} {
			t.Run(mode+"/"+option, func(t *testing.T) {
				hs, editor := caretClickHarness(t, mode)
				if option == "disabled" {
					hs.a.settings.SmoothCaret = false
				} else {
					hs.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
				}
				caretClickLine(t, hs, editor, 9)
				if editor.caretPaint.glide.moving(hs.tt.Now()) {
					t.Fatal("an animation preference still allowed a distant mouse glide")
				}
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				if editor.caretPaint.trail.ok {
					for _, point := range editor.caretPaint.trail.positions(shown, hs.tt.Now()) {
						if point != caretTrailHead(shown) {
							t.Fatal("an animation preference still allowed a mouse trailing path")
						}
					}
				}
			})
		}
	}
}

func TestCaretMouseJumpInterruptions(t *testing.T) {
	for _, mode := range []string{"source", "vim", "helix", "live", "live vim", "live helix"} {
		for _, action := range []string{"scroll", "drag"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				hs, editor := caretClickHarness(t, mode)
				caretClickLine(t, hs, editor, 9)
				hs.tt.Advance(motionCaretJump / 3)
				hs.tt.Frame()
				if !editor.caretPaint.glide.moving(hs.tt.Now()) {
					t.Fatal("the interruption did not begin during a mouse glide")
				}
				before := editor.caretPaint.glide.position(hs.tt.Now())
				particles := editor.caretPaint.trail.positions(before, hs.tt.Now())
				origin := caretTrailOrigin(t, hs, editor)
				if action == "scroll" {
					before := editor.vscroll.Y
					hs.tt.Scroll(origin.X+hs.a.font.charW*8, origin.Y+hs.a.font.lineH*9, 0, 4)
					if editor.vscroll.Y <= before {
						t.Fatal("the wheel did not actually scroll the editor")
					}
				} else {
					start, _ := editor.buf.Snapshot().LineRange(8)
					end, _ := editor.buf.Snapshot().LineRange(6)
					startX, startY, startH, startOK := editor.lines.Caret(start + 8)
					endX, endY, endH, endOK := editor.lines.Caret(end + 12)
					if !startOK || !endOK {
						t.Fatal("the drag endpoints are not laid out")
					}
					hs.tt.Press(origin.X+startX, origin.Y+startY+startH/2)
					hs.tt.Frame()
					hs.tt.Move(origin.X+endX, origin.Y+endY+endH/2)
					hs.tt.Frame()
					hs.tt.Release(origin.X+endX, origin.Y+endY+endH/2)
					hs.tt.Frame()
					if abs(editor.sel[1]-editor.sel[0]) < 40 || editor.buf.Snapshot().LineAt(editor.sel[1]) != 6 {
						t.Fatalf("the real pointer drag did not select its text range: %v", editor.sel)
					}
					shown := editor.caretPaint.glide.position(hs.tt.Now())
					if !editor.caretPaint.glide.moving(hs.tt.Now()) || shown != before {
						t.Fatalf("dragging interrupted the mouse glide: before=%v shown=%v", before, shown)
					}
					if editor.caretPaint.trail.positions(shown, hs.tt.Now()) != particles {
						t.Fatal("dragging reset the in-flight mouse trail")
					}
					target := editor.caretPaint.glide.to
					hs.tt.Advance(motionCaretJump + motionCaretTrail)
					hs.tt.Frame()
					if editor.caretPaint.glide.position(hs.tt.Now()) != target {
						t.Fatal("the drag did not finish at its new endpoint")
					}
				}
				hs.tt.Frame()
				if editor.caretPaint.glide.moving(hs.tt.Now()) {
					t.Fatalf("%s left the mouse glide running", action)
				}
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				if editor.caretPaint.trail.ok {
					for _, point := range editor.caretPaint.trail.positions(shown, hs.tt.Now()) {
						if point != caretTrailHead(shown) {
							t.Fatalf("%s left the mouse trailing particles behind", action)
						}
					}
				}
			})
		}
	}
}

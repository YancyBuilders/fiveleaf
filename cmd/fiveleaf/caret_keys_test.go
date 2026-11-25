package main

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func caretKeysHarness(t *testing.T, mode string) (*harness, *editor) {
	t.Helper()
	hs, editor := editHarness(t, "keys.md", strings.Repeat("a"+strings.Repeat(" ", 60)+"z\n", 40))
	hs.a.settings.Vim = true
	if strings.Contains(mode, "helix") {
		hs.a.settings.EditorMode = "helix"
	}
	if strings.Contains(mode, "live") {
		hs.a.setLive(editor, true)
	}
	hs.a.settings.SmoothCaret = true
	start, _ := editor.buf.Snapshot().LineRange(12)
	hs.place(editor, start+25, start+25)
	if hs.a.helixOn() {
		helixSelect(editor, start+25, start+25)
	}
	editor.vscroll.Focus()
	hs.settle()
	if strings.Contains(mode, "visual") {
		caretKeysPress(t, hs, "v")
		hs.settle()
	}
	return hs, editor
}

func caretKeysCursor(hs *harness, editor *editor) int {
	snapshot := editor.buf.Snapshot()
	if hs.a.helixOn() {
		return helixCursor(editor, snapshot)
	}
	return vimCursor(editor, snapshot)
}

func caretKeysPress(t *testing.T, hs *harness, keys string) {
	t.Helper()
	for _, character := range keys {
		modifiers, key := vimKeyOf(t, character)
		hs.tt.Key(modifiers, key)
		hs.tt.Frame()
	}
	hs.tt.Frame()
}

func TestCaretCountedMotionFrames(t *testing.T) {
	for _, mode := range []string{"vim", "helix", "live vim", "live helix", "vim visual", "live vim visual"} {
		for _, direction := range []string{"h", "j", "k", "l"} {
			t.Run(mode+"/10"+direction, func(t *testing.T) {
				hs, editor := caretKeysHarness(t, mode)
				origin := caretTrailOrigin(t, hs, editor)
				start := editor.caretPaint.glide.position(hs.tt.Now())
				cursor := caretKeysCursor(hs, editor)
				scroll := editor.vscroll.Y
				delta := 10
				if direction == "j" || direction == "k" {
					delta *= len("a" + strings.Repeat(" ", 60) + "z\n")
				}
				if direction == "h" || direction == "k" {
					delta = -delta
				}
				caretKeysPress(t, hs, "10"+direction)
				if actual := caretKeysCursor(hs, editor); actual != cursor+delta {
					t.Fatalf("the counted key did not immediately move the logical cursor: got=%d want=%d", actual, cursor+delta)
				}
				if editor.vscroll.Y != scroll {
					t.Fatalf("the visible counted motion unexpectedly scrolled from %v to %v", scroll, editor.vscroll.Y)
				}
				duration := editor.caretPaint.glide.duration
				hs.tt.Advance(duration / 3)
				hs.tt.Frame()
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				target := editor.caretPaint.glide.to
				if !editor.caretPaint.glide.moving(hs.tt.Now()) || shown == start || shown == target {
					t.Fatalf("the counted motion has no intermediate caret: start=%v shown=%v target=%v", start, shown, target)
				}
				points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
				moving := caretTrailSnapshot(hs)
				hs.tt.Advance(duration + motionCaretTrail)
				hs.tt.Frame()
				settled := hs.tt.Image()
				primary := image.Rect(int(math.Floor(float64(origin.X+shown.X))), int(origin.Y+shown.Y+shown.H/3),
					int(math.Ceil(float64(origin.X+shown.X+shown.W))), int(origin.Y+shown.Y+shown.H*2/3))
				if count, _ := caretTrailChanged(moving, settled, primary); count == 0 {
					t.Fatal("the counted motion's intermediate caret is not rendered")
				}
				last := points[len(points)-1]
				if direction == "h" || direction == "l" {
					connected := image.Rect(int(math.Ceil(float64(origin.X+min(last.X, points[0].X)+shown.W/2+2))),
						int(origin.Y+shown.Y), int(math.Floor(float64(origin.X+max(last.X, points[0].X)-shown.W/2-2))),
						int(math.Ceil(float64(origin.Y+shown.Y+shown.H))))
					caretTrailAssertConnected(t, moving, settled, connected)
				} else {
					left := int(math.Floor(float64(origin.X + min(last.X, points[0].X) - shown.W - 2)))
					right := int(math.Ceil(float64(origin.X + max(last.X, points[0].X) + shown.W + 2)))
					from := int(math.Ceil(float64(origin.Y + min(last.Y, points[0].Y) + shown.H + 2)))
					to := int(math.Floor(float64(origin.Y + max(last.Y, points[0].Y) - 2)))
					if to-from < 5 {
						t.Fatalf("the counted motion has no measurable trailing path: %v", points)
					}
					for row := from; row < to; row++ {
						if count, _ := caretTrailChanged(moving, settled, image.Rect(left, row, right, row+1)); count == 0 {
							t.Fatalf("the counted motion's rendered trail has a gap at row %d", row)
						}
					}
				}
				if hs.tt.FramePending() {
					t.Fatal("the counted motion keeps requesting frames after convergence")
				}
			})
		}
	}
}

func TestCaretCountedMotionReverses(t *testing.T) {
	for _, mode := range []string{"vim", "helix", "live vim", "live helix"} {
		for _, directions := range []string{"hl", "jk", "kj", "lh"} {
			t.Run(mode+"/"+directions, func(t *testing.T) {
				hs, editor := caretKeysHarness(t, mode)
				caretKeysPress(t, hs, "10"+directions[:1])
				hs.tt.Advance(editor.caretPaint.glide.duration / 3)
				hs.tt.Frame()
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
				if !editor.caretPaint.glide.moving(hs.tt.Now()) {
					t.Fatal("the first counted motion did not start a glide")
				}
				caretKeysPress(t, hs, "10"+directions[1:])
				if after := editor.caretPaint.glide.position(hs.tt.Now()); after != shown {
					t.Fatalf("the reverse counted motion jumped from %v to %v", shown, after)
				}
				if after := editor.caretPaint.trail.positions(shown, hs.tt.Now()); after != points {
					t.Fatal("the reverse counted motion reset the trailing path")
				}
				hs.tt.Advance(editor.caretPaint.glide.duration / 3)
				hs.tt.Frame()
				if after := editor.caretPaint.glide.position(hs.tt.Now()); !editor.caretPaint.glide.moving(hs.tt.Now()) || after == shown || after == editor.caretPaint.glide.to {
					t.Fatalf("the reverse counted motion has no intermediate position: %v", after)
				}
			})
		}
	}
}

func TestCaretCountedMotionInterruptions(t *testing.T) {
	for _, mode := range []string{"vim", "helix", "live vim", "live helix"} {
		for _, option := range []string{"disabled", "reduced motion", "scroll"} {
			t.Run(mode+"/"+option, func(t *testing.T) {
				hs, editor := caretKeysHarness(t, mode)
				if option == "disabled" {
					hs.a.settings.SmoothCaret = false
				} else if option == "reduced motion" {
					hs.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
				}
				caretKeysPress(t, hs, "10j")
				if option == "scroll" {
					hs.tt.Advance(editor.caretPaint.glide.duration / 3)
					hs.tt.Frame()
					if !editor.caretPaint.glide.moving(hs.tt.Now()) {
						t.Fatal("the wheel interruption did not begin during a glide")
					}
					origin := caretTrailOrigin(t, hs, editor)
					before := editor.vscroll.Y
					hs.tt.Scroll(origin.X+hs.a.font.charW*25, origin.Y+hs.a.font.lineH*22, 0, 4)
					if editor.vscroll.Y <= before {
						t.Fatal("the wheel did not actually scroll the editor")
					}
				}
				hs.tt.Frame()
				if editor.caretPaint.glide.moving(hs.tt.Now()) {
					t.Fatalf("%s did not suppress the counted motion's glide", option)
				}
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				if editor.caretPaint.trail.ok {
					for _, point := range editor.caretPaint.trail.positions(shown, hs.tt.Now()) {
						if point != caretTrailHead(shown) {
							t.Fatalf("%s left counted motion particles behind", option)
						}
					}
				}
			})
		}
	}
}

func TestCaretCountedMotionScrollsIntoView(t *testing.T) {
	for _, mode := range []string{"vim", "helix", "live vim", "live helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, editor := caretKeysHarness(t, mode)
			start, _ := editor.buf.Snapshot().LineRange(28)
			hs.place(editor, start+25, start+25)
			if hs.a.helixOn() {
				helixSelect(editor, start+25, start+25)
			}
			hs.settle()
			beforeScroll := editor.vscroll.Y
			caretKeysPress(t, hs, "10j")
			if editor.vscroll.Y <= beforeScroll {
				t.Fatalf("the near-edge counted motion did not scroll into view: before=%v after=%v", beforeScroll, editor.vscroll.Y)
			}
			if line := editor.buf.Snapshot().LineAt(caretKeysCursor(hs, editor)); line != 38 {
				t.Fatalf("the near-edge counted motion landed on line %d, want 38", line)
			}
			duration := editor.caretPaint.glide.duration
			hs.tt.Advance(duration / 3)
			hs.tt.Frame()
			shown := editor.caretPaint.glide.position(hs.tt.Now())
			if !editor.caretPaint.glide.moving(hs.tt.Now()) || shown == editor.caretPaint.glide.to {
				t.Fatal("automatically scrolling into view removed the counted motion's glide")
			}
			origin := caretTrailOrigin(t, hs, editor)
			moving := caretTrailSnapshot(hs)
			hs.tt.Advance(duration + motionCaretTrail)
			hs.tt.Frame()
			primary := image.Rect(int(math.Floor(float64(origin.X+shown.X))), int(origin.Y+shown.Y+shown.H/3),
				int(math.Ceil(float64(origin.X+shown.X+shown.W))), int(origin.Y+shown.Y+shown.H*2/3))
			if !primary.In(moving.Bounds()) {
				t.Fatalf("the intermediate caret remains outside the visible window: %v", primary)
			}
			if count, _ := caretTrailChanged(moving, hs.tt.Image(), primary); count == 0 {
				t.Fatal("the automatically scrolled intermediate caret is not rendered")
			}
		})
	}
}

func TestCaretCountedMotionWaitsForMovement(t *testing.T) {
	for _, mode := range []string{"vim", "helix", "live vim", "live helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, editor := caretKeysHarness(t, mode)
			cursor := caretKeysCursor(hs, editor)
			shown := editor.caretPaint.glide.position(hs.tt.Now())
			caretKeysPress(t, hs, "10")
			if caretKeysCursor(hs, editor) != cursor || editor.caretPaint.glide.position(hs.tt.Now()) != shown || editor.caretPaint.glide.moving(hs.tt.Now()) {
				t.Fatal("a count prefix moved or animated the caret before its motion key")
			}
			hs.tt.Key(0, ui.KeyEscape)
			hs.tt.Frame()
			hs.place(editor, 0, 0)
			if hs.a.helixOn() {
				helixSelect(editor, 0, 0)
			}
			hs.settle()
			shown = editor.caretPaint.glide.position(hs.tt.Now())
			caretKeysPress(t, hs, "10h")
			if caretKeysCursor(hs, editor) != 0 || editor.caretPaint.glide.position(hs.tt.Now()) != shown || editor.caretPaint.glide.moving(hs.tt.Now()) {
				t.Fatal("a counted motion blocked at the line boundary moved or animated the caret")
			}
		})
	}
}

func TestCaretBatchedNavigationFrames(t *testing.T) {
	for _, mode := range []string{"vim", "helix", "live vim", "live helix"} {
		for _, keys := range []string{"G", "10jG"} {
			t.Run(mode+"/"+keys, func(t *testing.T) {
				hs, editor := caretKeysHarness(t, mode)
				start := editor.caretPaint.glide.position(hs.tt.Now())
				if hs.a.helixOn() {
					for _, character := range keys {
						modifiers, key := vimKeyOf(t, character)
						if !hs.a.helixKey(editor, ui.InputEvent{Kind: ui.InputKeyDown, Mods: modifiers, Key: key}) {
							t.Fatalf("Helix did not consume %q", character)
						}
					}
				} else {
					hs.a.vimFeed(editor, []rune(keys))
				}
				hs.tt.Frame()
				snapshot := editor.buf.Snapshot()
				if line := snapshot.LineAt(caretKeysCursor(hs, editor)); line != vimLast(snapshot) {
					t.Fatalf("the final G landed on line %d, want %d", line, vimLast(snapshot))
				}
				animation := &editor.caretPaint.glide
				target := animation.to
				if !animation.moving(hs.tt.Now()) || animation.position(hs.tt.Now()) != start {
					t.Fatalf("batched navigation did not glide from the visible caret: %+v", animation)
				}
				hs.tt.Advance(animation.duration / 3)
				hs.tt.Frame()
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				if !animation.moving(hs.tt.Now()) || shown == start || shown == target || animation.to != target {
					t.Fatalf("batched navigation lost its final target or intermediate frame: %+v at %v", animation, shown)
				}
				hs.tt.Advance(animation.duration + motionCaretTrail)
				hs.tt.Frame()
				if animation.moving(hs.tt.Now()) || animation.position(hs.tt.Now()) != target {
					t.Fatalf("batched navigation did not settle at the final target: %+v", animation)
				}
				for _, point := range editor.caretPaint.trail.positions(target, hs.tt.Now()) {
					if point != caretTrailHead(target) {
						t.Fatal("batched navigation left a stale trail after settling")
					}
				}
			})
		}
	}
}

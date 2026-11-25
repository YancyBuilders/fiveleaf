package main

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHelixCaretAnimationAndTrail(t *testing.T) {
	for _, view := range []string{"source", "live"} {
		for _, mode := range []string{"normal"} {
			t.Run(view+"/"+mode, func(t *testing.T) {
				hs, editor := helixHarness(t, "helix-trail.md", "a"+strings.Repeat(" ", 60)+"z\n")
				hs.a.settings.Vim = true
				hs.a.settings.SmoothCaret = true
				if view == "live" {
					hs.a.setLive(editor, true)
				}
				editor.vscroll.Focus()
				hs.helix("l")
				hs.settle()
				if !hs.a.helixOn() || editorMode(hs.a.prefs()) != "helix" {
					t.Fatal("the real editorMode setting did not select Helix over legacy vim")
				}
				origin := caretTrailOrigin(t, hs, editor)
				before := caretTrailSnapshot(hs)
				start, ok := caretAt(&editor.lines, editor.helix.cursor, 2)
				if !ok {
					t.Fatal("the starting Helix cursor has no text layout")
				}
				beforeCursor := editor.helix.cursor
				for range 8 {
					hs.tt.Key(0, ui.KeyL)
				}
				hs.tt.Frame()
				if editor.helix.cursor != beforeCursor+8 {
					t.Fatalf("Helix l did not move eight runes: %d to %d", beforeCursor, editor.helix.cursor)
				}
				hs.tt.Advance(motionCaret / 3)
				hs.tt.Frame()
				animation := editor.caretPaint.glide
				shown := animation.position(hs.tt.Now())
				points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
				moving := caretTrailSnapshot(hs)
				target, ok := caretAt(&editor.lines, editor.helix.cursor, 2)
				if !ok {
					t.Fatal("the target Helix cursor has no text layout")
				}
				ghost := image.Rect(int(origin.X+target.X+1), int(origin.Y+target.Y+target.H/5),
					int(origin.X+target.X+hs.a.font.charW-1), int(origin.Y+target.Y+target.H/3))
				if count, _ := caretTrailChanged(moving, before, ghost); count != 0 {
					t.Fatal("the target retains a static Helix selection block during its glide")
				}
				hs.tt.Advance(motionCaret + motionCaretTrail)
				hs.tt.Frame()
				settled := caretTrailSnapshot(hs)
				if !animation.ok || shown.X <= start.X || shown.X >= target.X {
					t.Errorf("the Helix cursor has no intermediate animation: start=%v shown=%v target=%v active=%v", start, shown, target, animation.ok)
				}
				band := caretTrailBand(origin, start, target)
				if count, _ := caretTrailChanged(moving, settled, band); count == 0 {
					t.Fatal("Helix movement has no visible intermediate cursor or trailing path")
				}
				primary := image.Rect(int(math.Ceil(float64(origin.X+shown.X))), int(origin.Y+shown.Y+shown.H/3),
					int(math.Ceil(float64(origin.X+shown.X+shown.W))), int(origin.Y+shown.Y+shown.H*2/3))
				if count, _ := caretTrailChanged(moving, settled, primary); count == 0 {
					t.Fatal("the intermediate Helix cursor is not painted")
				}
				connected := image.Rect(int(math.Ceil(float64(origin.X+points[len(points)-1].X+3))),
					int(origin.Y+shown.Y), int(math.Floor(float64(origin.X+shown.X-2))), int(math.Ceil(float64(origin.Y+shown.Y+shown.H))))
				caretTrailAssertConnected(t, moving, settled, connected)
			})
		}
	}
}

func helixCaretView(t *testing.T, view, text string) (*harness, *editor) {
	t.Helper()
	hs, editor := helixHarness(t, "helix-caret.md", text)
	hs.a.settings.Vim = true
	hs.a.settings.SmoothCaret = true
	if view == "live" {
		hs.a.setLive(editor, true)
	}
	editor.vscroll.Focus()
	hs.settle()
	return hs, editor
}

func TestHelixCaretTransitionsAndPreferences(t *testing.T) {
	for _, view := range []string{"source", "live"} {
		t.Run(view, func(t *testing.T) {
			hs, editor := helixCaretView(t, view, "a"+strings.Repeat(" ", 60)+"z\n")
			hs.helix("lli")
			if editor.helix.mode != helixInsert || editor.sel[0] != editor.sel[1] {
				t.Fatal("i did not enter insert mode at the selection")
			}
			hs.tt.Key(0, ui.KeyRight)
			hs.tt.Advance(motionCaret / 3)
			hs.tt.Frame()
			if !editor.caretPaint.glide.moving(hs.tt.Now()) {
				t.Fatal("Helix insert mode has no caret animation")
			}
			hs.tt.Key(0, ui.KeyEscape)
			hs.tt.Frame()
			if editor.helix.mode != helixNormal || abs(editor.sel[1]-editor.sel[0]) != 1 {
				t.Fatal("Esc did not restore a one-rune normal selection")
			}
			hs.settle()
			if editor.caretPaint.glide.to.W <= 2 {
				t.Fatal("Esc did not restore the normal block cursor")
			}
			for _, preference := range []string{"disabled", "reduced motion"} {
				hs.a.settings.SmoothCaret = true
				hs.tt.SetPreferences(ui.Preferences{})
				hs.tt.Key(0, ui.KeyL)
				hs.tt.Advance(motionCaret / 3)
				if preference == "disabled" {
					hs.a.settings.SmoothCaret = false
				} else {
					hs.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
				}
				hs.tt.Key(0, ui.KeyL)
				hs.tt.Frame()
				if editor.caretPaint.glide.moving(hs.tt.Now()) {
					t.Fatalf("%s did not stop the Helix block immediately", preference)
				}
				shown := editor.caretPaint.glide.position(hs.tt.Now())
				for _, point := range editor.caretPaint.trail.positions(shown, hs.tt.Now()) {
					if point != caretTrailHead(shown) {
						t.Fatalf("%s retained trailing Helix particles", preference)
					}
				}
			}
		})
	}
}

func TestHelixCaretSelectionAndRTLGeometry(t *testing.T) {
	for _, view := range []string{"source", "live"} {
		for _, selection := range [][2]int{{0, 0}, {1, 3}, {3, 1}} {
			t.Run(view+"/"+string(rune('0'+selection[0]))+string(rune('0'+selection[1])), func(t *testing.T) {
				hs, editor := helixCaretView(t, view, "שלום עולם\n")
				helixSelect(editor, 6, 6)
				hs.settle()
				unselected := caretTrailSnapshot(hs)
				helixSelect(editor, selection[0], selection[1])
				hs.settle()
				wantSelection := [2]int{min(selection[0], selection[1]), max(selection[0], selection[1]) + 1}
				if selection[1] < selection[0] {
					wantSelection[0], wantSelection[1] = wantSelection[1], wantSelection[0]
				}
				if editor.sel != wantSelection {
					t.Fatalf("drawing changed the Helix selection: got %v want %v", editor.sel, wantSelection)
				}
				cursor := selection[1]
				firstX, firstY, height, firstOK := editor.lines.Caret(cursor)
				lastX, lastY, _, lastOK := editor.lines.Caret(cursor + 1)
				if !firstOK || !lastOK || firstY != lastY || firstX == lastX {
					t.Fatal("the RTL character does not have measurable visual endpoints")
				}
				if cursor == 0 && firstX == 0 {
					t.Fatal("the RTL fixture did not distinguish visual layout from prefix width")
				}
				if editor.caretPaint.glide.ok {
					t.Errorf("RTL should retain the native selection instead of an ambiguous custom block: %v", editor.caretPaint.glide.to)
				}
				origin := caretTrailOrigin(t, hs, editor)
				selected := image.Rect(int(math.Ceil(float64(origin.X+min(firstX, lastX)+1))),
					int(origin.Y+firstY+height/5), int(math.Floor(float64(origin.X+max(firstX, lastX)-1))),
					int(origin.Y+firstY+height*4/5))
				if count, _ := caretTrailChanged(unselected, hs.tt.Image(), selected); count == 0 {
					t.Fatal("the native selection is missing at the real RTL character")
				}
			})
		}
	}
}

func TestHelixCaretKeepsLTRRange(t *testing.T) {
	for _, view := range []string{"source", "live"} {
		t.Run(view, func(t *testing.T) {
			hs, editor := helixCaretView(t, view, "a"+strings.Repeat(" ", 60)+"z\n")
			for _, selection := range [][2]int{{1, 5}, {5, 1}} {
				helixSelect(editor, 20, 20)
				hs.settle()
				unselected := caretTrailSnapshot(hs)
				helixSelect(editor, selection[0], selection[1])
				hs.settle()
				want := [2]int{1, 6}
				if selection[1] < selection[0] {
					want = [2]int{6, 1}
				}
				if editor.sel != want {
					t.Fatalf("visual block changed the selected range: got %v want %v", editor.sel, want)
				}
				cursorX, cursorY, _, ok := editor.lines.Caret(selection[1])
				if !ok || !editor.caretPaint.glide.ok || math.Abs(float64(editor.caretPaint.glide.to.X-cursorX)) > 0.1 || math.Abs(float64(editor.caretPaint.glide.to.Y-cursorY)) > 0.1 {
					t.Fatalf("the block does not follow the actual range endpoint: %v expected (%v,%v)", editor.caretPaint.glide.to, cursorX, cursorY)
				}
				left, row, height, ok := editor.lines.Caret(3)
				right, _, _, nextOK := editor.lines.Caret(4)
				if !ok || !nextOK {
					t.Fatal("the selected interior rune has no layout")
				}
				origin := caretTrailOrigin(t, hs, editor)
				interior := image.Rect(int(origin.X+left+1), int(origin.Y+row+height/5),
					int(origin.X+right-1), int(origin.Y+row+height/3))
				if count, _ := caretTrailChanged(unselected, hs.tt.Image(), interior); count == 0 {
					t.Fatal("the visual block hid the existing range highlight")
				}
			}
		})
	}
}

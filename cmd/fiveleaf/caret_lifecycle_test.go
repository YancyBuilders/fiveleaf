package main

import (
	"image"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func caretLifecycleHarness(t *testing.T, mode, text string, cursor int) (*harness, *editor) {
	t.Helper()
	hs, editor := editHarness(t, "lifecycle.md", text)
	hs.a.settings.SmoothCaret = true
	hs.a.settings.Vim = strings.Contains(mode, "vim") || strings.Contains(mode, "helix")
	if strings.Contains(mode, "helix") {
		hs.a.settings.EditorMode = "helix"
	}
	if strings.Contains(mode, "live") {
		hs.a.setLive(editor, true)
	}
	hs.place(editor, cursor, cursor)
	if hs.a.helixOn() {
		helixSelect(editor, cursor, cursor)
	}
	editor.vscroll.Focus()
	hs.settle()
	return hs, editor
}

func caretLifecycleCursor(hs *harness, editor *editor) int {
	if hs.a.helixOn() {
		return helixCursor(editor, editor.buf.Snapshot())
	}
	return editor.sel[1]
}

func caretLifecycleAssertMotion(t *testing.T, hs *harness, editor *editor, start ui.Rect, cursor int) {
	t.Helper()
	if actual := caretLifecycleCursor(hs, editor); actual != cursor {
		t.Fatalf("navigation did not immediately move the logical cursor: got=%d want=%d", actual, cursor)
	}
	duration := editor.caretPaint.glide.duration
	hs.tt.Advance(duration / 3)
	hs.tt.Frame()
	shown := editor.caretPaint.glide.position(hs.tt.Now())
	target := editor.caretPaint.glide.to
	if !editor.caretPaint.glide.moving(hs.tt.Now()) || shown == start || shown == target {
		t.Fatalf("navigation has no intermediate caret: start=%v shown=%v target=%v", start, shown, target)
	}
	origin := caretTrailOrigin(t, hs, editor)
	points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
	moving := caretTrailSnapshot(hs)
	hs.tt.Advance(duration + motionCaretTrail)
	hs.tt.Frame()
	settled := hs.tt.Image()
	primary := image.Rect(int(math.Floor(float64(origin.X+shown.X))), int(origin.Y+shown.Y+shown.H/3),
		int(math.Ceil(float64(origin.X+shown.X+shown.W))), int(origin.Y+shown.Y+shown.H*2/3))
	if !primary.In(moving.Bounds()) {
		t.Fatalf("the intermediate caret is outside the visible window: %v", primary)
	}
	if count, _ := caretTrailChanged(moving, settled, primary); count == 0 {
		t.Fatal("the intermediate caret is not rendered")
	}
	last := points[len(points)-1]
	connected := image.Rect(int(math.Ceil(float64(origin.X+min(last.X, points[0].X)+shown.W/2+2))),
		int(origin.Y+shown.Y), int(math.Floor(float64(origin.X+max(last.X, points[0].X)-shown.W/2-2))),
		int(math.Ceil(float64(origin.Y+shown.Y+shown.H))))
	caretTrailAssertConnected(t, moving, settled, connected)
	if actual := caretLifecycleCursor(hs, editor); actual != cursor || editor.caretPaint.glide.position(hs.tt.Now()) != target {
		t.Fatalf("navigation did not finish at its logical cursor: got=%d want=%d shown=%v target=%v", actual, cursor, editor.caretPaint.glide.position(hs.tt.Now()), target)
	}
	if hs.tt.FramePending() {
		t.Fatal("navigation keeps requesting frames after its caret and trail converge")
	}
}

func TestCaretLineNavigationFrames(t *testing.T) {
	text := "a" + strings.Repeat(" ", 80) + "z\n"
	for _, mode := range []string{"source", "live"} {
		for _, movement := range []struct {
			name        string
			key         ui.Key
			mods        ui.Modifiers
			start, want int
		}{
			{name: "End", key: ui.KeyEnd, start: 1, want: len(text) - 1},
			{name: "Home", key: ui.KeyHome, start: len(text) - 1, want: 0},
			{name: "CmdRight", key: ui.KeyRight, mods: ui.Cmd, start: 1, want: len(text) - 1},
		} {
			t.Run(mode+"/"+movement.name, func(t *testing.T) {
				hs, editor := caretLifecycleHarness(t, mode, text, movement.start)
				start := editor.caretPaint.glide.position(hs.tt.Now())
				hs.tt.Key(movement.mods, movement.key)
				hs.tt.Frame()
				caretLifecycleAssertMotion(t, hs, editor, start, movement.want)
			})
		}
	}
}

func TestCaretModalNavigationFrames(t *testing.T) {
	text := strings.Repeat("word ", 15) + "z\n"
	for _, mode := range []string{"vim", "helix", "live vim", "live helix"} {
		for _, movement := range []struct {
			keys        string
			start, want int
		}{
			{keys: "10w", start: 0, want: 50},
			{keys: "0", start: 60, want: 0},
			{keys: "$", start: 1, want: len(text) - 2},
		} {
			t.Run(mode+"/"+movement.keys, func(t *testing.T) {
				hs, editor := caretLifecycleHarness(t, mode, text, movement.start)
				start := editor.caretPaint.glide.position(hs.tt.Now())
				caretKeysPress(t, hs, movement.keys)
				caretLifecycleAssertMotion(t, hs, editor, start, movement.want)
			})
		}
	}
}

func TestCaretSelectionNavigationFrames(t *testing.T) {
	for _, mode := range []string{"source", "live"} {
		t.Run(mode, func(t *testing.T) {
			hs, editor := caretLifecycleHarness(t, mode, "a"+strings.Repeat(" ", 60)+"z\n", 1)
			start := editor.caretPaint.glide.position(hs.tt.Now())
			for range 12 {
				hs.tt.Key(ui.Shift, ui.KeyRight)
			}
			hs.tt.Frame()
			if editor.sel != [2]int{1, 13} {
				t.Fatalf("Shift+Right did not immediately extend the selection: %v", editor.sel)
			}
			caretLifecycleAssertMotion(t, hs, editor, start, 13)
			if editor.sel != [2]int{1, 13} {
				t.Fatalf("the animation changed the selection: %v", editor.sel)
			}
		})
	}
}

func TestCaretDragTracksLiveBlockLayout(t *testing.T) {
	hs := newHarness(t, false)
	if err := os.WriteFile(hs.path("detail.md"), []byte(strings.Repeat("Embedded paragraph.\n\n", 4)), 0o644); err != nil {
		t.Fatal(err)
	}
	text := "before\n\n![[detail]]\n\nafter\n"
	editor := openLive(t, hs, "drag-block.md", text, 1)
	hs.a.settings.SmoothCaret = true
	hs.place(editor, 1, 1)
	editor.vscroll.Focus()
	hs.settle()
	origin := caretTrailOrigin(t, hs, editor)
	end := strings.Index(text, "after") + 1
	startX, startY, startH, startOK := editor.lines.Caret(1)
	endX, endY, endH, endOK := editor.lines.Caret(end)
	if !startOK || !endOK {
		t.Fatal("the drag endpoints are not laid out")
	}
	hs.tt.Press(origin.X+startX, origin.Y+startY+startH/2)
	hs.tt.Move(origin.X+endX, origin.Y+endY+endH/2)
	if editor.sel != [2]int{1, end} {
		t.Fatalf("the real pointer drag did not select across the embedded note: %v", editor.sel)
	}
	first, ok := caretAt(&editor.lines, end, 2)
	if !ok {
		t.Fatal("the drag cursor has no text layout")
	}
	layoutMoved := false
	for frame := range 4 {
		hs.tt.Advance(motionMove / 4)
		continued := editor.caretPaint.glide.position(hs.tt.Now())
		hs.tt.Frame()
		target, ok := caretAt(&editor.lines, editor.sel[1], 2)
		if !ok {
			t.Fatal("the drag cursor lost its text layout")
		}
		layoutMoved = layoutMoved || math.Abs(float64(target.Y-first.Y)) > 1
		shown := editor.caretPaint.glide.position(hs.tt.Now())
		if editor.sel[0] != 1 || editor.sel[1] < end {
			t.Fatalf("a layout frame lost the held drag selection across the embedded note: %v", editor.sel)
		}
		if editor.caretPaint.glide.to != target || shown != continued {
			t.Fatalf("layout frame %d did not continue toward the current drag endpoint: shown=%v continued=%v target=%v animation=%+v", frame+1, shown, continued, target, editor.caretPaint.glide)
		}
	}
	if !layoutMoved {
		t.Fatal("revealing the selected embed did not move the layout below it")
	}
	hs.tt.Release(origin.X+endX, origin.Y+endY+endH/2)
	hs.tt.Advance(motionMove + motionCaretJump + motionCaretTrail)
	hs.tt.Frame()
	target, ok := caretAt(&editor.lines, editor.sel[1], 2)
	if !ok || editor.caretPaint.glide.moving(hs.tt.Now()) || editor.caretPaint.glide.position(hs.tt.Now()) != target {
		t.Fatalf("the released drag did not settle at its endpoint: target=%v animation=%+v", target, editor.caretPaint.glide)
	}
	for _, point := range editor.caretPaint.trail.positions(target, hs.tt.Now()) {
		if point != caretTrailHead(target) {
			t.Fatal("the released drag left a stale trail after its layout settled")
		}
	}
	if hs.tt.FramePending() {
		t.Fatal("the released drag keeps requesting frames after its layout and caret settled")
	}
}

package main

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

// The caret glides from where it showed to where it goes, and is there
// once the glide's time is up.
func TestCaretGlides(t *testing.T) {
	start := time.Now()
	from := ui.Rect{X: 10, Y: 20, W: 2, H: 18}
	to := ui.Rect{X: 110, Y: 20, W: 2, H: 18}
	var g caretGlide
	g.follow(from, start, true)
	g.follow(to, start, true)
	if moving := g.moving(start); !moving {
		t.Fatal("a key's move starts at once")
	}
	mid := g.position(start.Add(motionCaret / 2))
	if mid.X <= from.X || mid.X >= to.X {
		t.Errorf("halfway the caret is at %v, not between %v and %v", mid, from, to)
	}
	if end := g.position(start.Add(motionCaret)); end != to {
		t.Errorf("after motionCaret the caret is at %v, not %v", end, to)
	}
	if moving := g.moving(start.Add(motionCaret)); moving {
		t.Error("the caret still moves after motionCaret")
	}
}

// A move that starts while the caret is on its way starts from where it
// shows, as a key that repeats turns the caret back.
func TestCaretGlideTurnsBack(t *testing.T) {
	start := time.Now()
	from := ui.Rect{X: 0, Y: 0, W: 2, H: 18}
	mid := ui.Rect{X: 60, Y: 0, W: 2, H: 18}
	back := ui.Rect{X: 20, Y: 0, W: 2, H: 18}
	var g caretGlide
	g.follow(from, start, true)
	g.follow(mid, start, true)
	shown := g.position(start.Add(motionCaret / 2))
	turn := start.Add(motionCaret / 2)
	g.follow(back, turn, true)
	if got := g.position(start.Add(motionCaret / 2)); got != shown {
		t.Errorf("the caret jumps to %v from where it showed at %v", got, shown)
	}
	if got := g.position(turn.Add(motionCaret)); got != back {
		t.Errorf("the caret goes to %v, not %v", got, back)
	}
}

func TestCaretSnaps(t *testing.T) {
	now := time.Now()
	at := ui.Rect{X: 4, Y: 4, W: 2, H: 18}
	for _, tc := range []struct {
		name string
		to   ui.Rect
	}{
		{"an immediate move", ui.Rect{X: 200, Y: 4, W: 2, H: 18}},
		{"a line far away", ui.Rect{X: 4, Y: 4 + 80, W: 2, H: 18}},
		{"across the page", ui.Rect{X: 900, Y: 4, W: 2, H: 18}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g caretGlide
			g.follow(at, now, true)
			g.follow(tc.to, now, false)
			if moving := g.moving(now); moving {
				t.Error("the caret glides")
			}
			if got := g.position(now); got != tc.to {
				t.Errorf("the caret is at %v, not %v", got, tc.to)
			}
		})
	}
	// A place with no valid height is a new layout, which the caret goes to at
	// once.
	for _, height := range []float32{0, -1, float32(math.NaN()), float32(math.Inf(1))} {
		var glide caretGlide
		glide.follow(at, now, true)
		laid := ui.Rect{X: 10, Y: 2, W: 2, H: height}
		glide.follow(laid, now, true)
		if moving := glide.moving(now); moving {
			t.Errorf("the caret glides to an invalid height: %v", height)
		}
	}
}

func TestCaretAtRejectsNoPlace(t *testing.T) {
	lines := &ui.TextLines{}
	if _, ok := caretAt(lines, 10, 2); ok {
		t.Error("a caret past the lines has a place")
	}
}

func TestEditorCaretAnimationFrames(t *testing.T) {
	hs, editor := editHarness(t, "caret.txt", "                \n                \n")
	editor.vscroll.Focus()
	hs.settle()
	start := editor.caretPaint.glide.to
	hs.tt.Advance(motionCaret / 10)
	for range 5 {
		hs.tt.Key(0, ui.KeyRight)
	}
	hs.tt.Frame()
	animation := &editor.caretPaint.glide
	if !animation.moving(hs.tt.Now()) || animation.to.X <= start.X {
		t.Fatalf("Right has no caret animation: %+v", animation)
	}
	began := animation.start
	hs.tt.Advance(motionCaret / 2)
	hs.tt.Frame()
	shown := animation.position(hs.tt.Now())
	if animation.start != began || shown.X <= start.X || shown.X >= animation.to.X {
		t.Fatalf("an idle frame restarted the caret: %+v at %v", animation, shown)
	}
	area, found := hs.tt.Find(locales.EditorLabel(editor.name()))
	if !found {
		t.Fatal("no editor bounds")
	}
	originX := area.X + editor.gutterW + gutterGap
	originY := area.Y + editorPadY - editor.vscroll.Y
	image := hs.tt.Image()
	caretColor := color.RGBA{R: hs.a.pal.accent.R, G: hs.a.pal.accent.G, B: hs.a.pal.accent.B, A: 255}
	painted := false
	for column := int(math.Floor(float64(shown.X))); column <= int(math.Ceil(float64(shown.X+shown.W))); column++ {
		pixel := image.RGBAAt(int(originX)+column, int(originY+shown.Y+shown.H/2))
		if pixel == caretColor {
			painted = true
		}
	}
	if !painted {
		t.Fatal("the intermediate caret is not rendered")
	}
	ghost := image.RGBAAt(int(originX+animation.to.X), int(originY+animation.to.Y+animation.to.H/2))
	if ghost == caretColor {
		t.Fatal("the native caret remains at the target during the glide")
	}
	hs.tt.Advance(motionCaret)
	hs.tt.Frame()
	if animation.moving(hs.tt.Now()) || animation.position(hs.tt.Now()) != animation.to {
		t.Fatalf("the caret did not finish: %+v", animation)
	}
}

func TestEditorCaretPreferences(t *testing.T) {
	for _, option := range []string{"disabled", "reduced motion"} {
		t.Run(option, func(t *testing.T) {
			hs, editor := editHarness(t, "caret.txt", "                \n")
			editor.vscroll.Focus()
			hs.settle()
			switch option {
			case "disabled":
				hs.a.settings.SmoothCaret = false
				hs.tt.Advance(motionCaret / 10)
				hs.tt.Key(0, ui.KeyRight)
			case "reduced motion":
				hs.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
				hs.tt.Advance(motionCaret / 10)
				hs.tt.Key(0, ui.KeyRight)
			}
			hs.tt.Frame()
			if editor.caretPaint.glide.moving(hs.tt.Now()) {
				t.Fatalf("%s still animates the caret", option)
			}
			if editor.sel[1] == 0 {
				t.Fatalf("%s did not move the caret", option)
			}
		})
	}
}

func TestVimCaretAnimationFrames(t *testing.T) {
	hs, editor := vimHarness(t, "caret.txt", "one two three\n")
	editor.vscroll.Focus()
	hs.settle()
	start := editor.caretPaint.glide.to
	hs.tt.Advance(motionCaret / 10)
	hs.tt.Key(0, ui.KeyL)
	hs.tt.Frame()
	animation := &editor.caretPaint.glide
	if !animation.moving(hs.tt.Now()) || animation.to.X <= start.X || animation.to.W < hs.a.font.charW/2 {
		t.Fatalf("Vim l has no block animation: %+v", animation)
	}
	began := animation.start
	hs.tt.Advance(motionCaret / 2)
	hs.tt.Frame()
	if shown := animation.position(hs.tt.Now()); animation.start != began || shown.X <= start.X || shown.X >= animation.to.X {
		t.Fatalf("the block did not glide: %+v at %v", animation, shown)
	}
	hs.tt.Advance(motionCaret)
	hs.tt.Frame()
	if animation.moving(hs.tt.Now()) {
		t.Fatal("the block animation did not finish")
	}
}

func TestEditorCaretToggleDuringAnimation(t *testing.T) {
	for _, live := range []bool{false, true} {
		name := "source"
		if live {
			name = "live"
		}
		t.Run(name, func(t *testing.T) {
			hs, editor := editHarness(t, "caret.md", "plain words on a line\n")
			if live {
				hs.a.setLive(editor, true)
			}
			editor.vscroll.Focus()
			hs.settle()
			hs.tt.Advance(motionCaret / 10)
			hs.tt.Key(0, ui.KeyRight)
			hs.tt.Frame()
			animation := &editor.caretPaint.glide
			if !animation.moving(hs.tt.Now()) {
				t.Fatal("Right did not start an animation")
			}
			target := animation.to
			hs.a.settings.SmoothCaret = false
			hs.tt.Frame()
			if animation.moving(hs.tt.Now()) || animation.ok {
				t.Fatal("disabling smooth caret retained its animation")
			}
			hs.a.settings.SmoothCaret = true
			hs.tt.Frame()
			if animation.moving(hs.tt.Now()) || animation.position(hs.tt.Now()) != target {
				t.Fatal("enabling smooth caret resumed a stale animation")
			}
		})
	}
}

func TestLiveCaretAnimationFrames(t *testing.T) {
	hs, editor := editHarness(t, "caret.md", "plain words on a line\n")
	hs.a.setLive(editor, true)
	editor.vscroll.Focus()
	hs.settle()
	start := editor.caretPaint.glide.to
	hs.tt.Advance(motionCaret / 10)
	hs.tt.Key(0, ui.KeyRight)
	hs.tt.Frame()
	animation := &editor.caretPaint.glide
	if !animation.moving(hs.tt.Now()) || animation.to.X <= start.X {
		t.Fatalf("Live Preview Right has no caret animation: %+v", animation)
	}
	hs.tt.Advance(motionCaret)
	hs.tt.Frame()
	if animation.moving(hs.tt.Now()) {
		t.Fatal("Live Preview caret did not finish")
	}
}

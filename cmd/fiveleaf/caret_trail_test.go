package main

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func caretTrailHarness(t *testing.T, mode string) (*harness, *editor) {
	t.Helper()
	hs, editor := editHarness(t, "trail.md", "a"+strings.Repeat(" ", 60)+"z\n")
	if strings.Contains(mode, "vim") {
		hs.a.settings.Vim = true
	}
	if strings.Contains(mode, "live") {
		hs.a.setLive(editor, true)
	}
	hs.place(editor, 1, 1)
	editor.vscroll.Focus()
	hs.settle()
	return hs, editor
}

func caretTrailOrigin(t *testing.T, hs *harness, editor *editor) ui.Point {
	t.Helper()
	area, found := hs.tt.Find(locales.EditorLabel(editor.name()))
	if !found {
		t.Fatal("no editor bounds")
	}
	left := editor.gutterW + gutterGap
	if hs.a.livePreview(editor) {
		left = max(textPadR, (area.W-editor.document.live.measure)/2)
	}
	return ui.Point{X: area.X + left, Y: area.Y + editorPadY - editor.vscroll.Y}
}

func caretTrailSnapshot(hs *harness) *image.RGBA {
	source := hs.tt.Image()
	snapshot := image.NewRGBA(source.Bounds())
	copy(snapshot.Pix, source.Pix)
	return snapshot
}

func caretTrailMove(hs *harness, mode string, key ui.Key) {
	if strings.Contains(mode, "vim") {
		switch key {
		case ui.KeyRight:
			key = ui.KeyL
		case ui.KeyLeft:
			key = ui.KeyH
		}
	}
	for range 8 {
		hs.tt.Key(0, key)
	}
	hs.tt.Frame()
}

func caretTrailDifference(first, second color.RGBA) int {
	return abs(int(first.R)-int(second.R)) + abs(int(first.G)-int(second.G)) + abs(int(first.B)-int(second.B))
}

func caretTrailBand(origin ui.Point, from, to ui.Rect) image.Rectangle {
	return image.Rect(int(math.Ceil(float64(origin.X+from.X+from.W+3))),
		int(origin.Y+from.Y+from.H/3), int(math.Floor(float64(origin.X+to.X-3))),
		int(origin.Y+from.Y+from.H*2/3))
}

func caretTrailChanged(first, second *image.RGBA, band image.Rectangle) (count, strongest int) {
	for row := band.Min.Y; row < band.Max.Y; row++ {
		for column := band.Min.X; column < band.Max.X; column++ {
			difference := caretTrailDifference(first.RGBAAt(column, row), second.RGBAAt(column, row))
			if difference > 3 {
				count++
				strongest = max(strongest, difference)
			}
		}
	}
	return count, strongest
}

func TestCaretTrailFrames(t *testing.T) {
	for _, mode := range []string{"source", "vim", "live", "live vim"} {
		t.Run(mode, func(t *testing.T) {
			hs, editor := caretTrailHarness(t, mode)
			origin := caretTrailOrigin(t, hs, editor)
			start := editor.caretPaint.glide.to
			caretTrailMove(hs, mode, ui.KeyRight)
			hs.tt.Advance(motionCaret / 3)
			hs.tt.Frame()
			hs.tt.Advance(motionCaret / 3)
			hs.tt.Frame()
			shown := editor.caretPaint.glide.position(hs.tt.Now())
			points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
			moving := caretTrailSnapshot(hs)
			band := caretTrailBand(origin, start, shown)
			if band.Dx() < 4 {
				t.Fatalf("the caret did not move far enough to leave a trail: %v", shown)
			}
			for range 3 {
				hs.tt.Frame()
				if count, _ := caretTrailChanged(moving, hs.tt.Image(), band); count != 0 {
					t.Fatal("same-time frames advanced the rendered trailing path")
				}
			}
			hs.tt.Advance(motionCaret + motionCaretTrail)
			hs.tt.Frame()
			settled := caretTrailSnapshot(hs)
			if hs.tt.FramePending() {
				t.Fatal("the caret still requests animation frames past its convergence window")
			}
			changed, trailStrength := caretTrailChanged(moving, settled, band)
			if changed < 3 {
				t.Fatal("the moving caret has no visible trail behind it")
			}
			connected := image.Rect(int(math.Ceil(float64(origin.X+points[len(points)-1].X+3))),
				int(origin.Y+shown.Y), int(math.Floor(float64(origin.X+shown.X-2))),
				int(math.Ceil(float64(origin.Y+shown.Y+shown.H))))
			caretTrailAssertConnected(t, moving, settled, connected)
			primaryBand := image.Rect(int(math.Ceil(float64(origin.X+shown.X))),
				int(origin.Y+shown.Y+shown.H/3), int(math.Ceil(float64(origin.X+shown.X+shown.W))),
				int(origin.Y+shown.Y+shown.H*2/3))
			_, primaryStrength := caretTrailChanged(moving, settled, primaryBand)
			if primaryStrength <= trailStrength {
				t.Fatalf("the primary caret is not clearer than its trail: primary=%d trail=%d origin=%v shown=%v target=%v", primaryStrength, trailStrength, origin, shown, editor.caretPaint.glide.to)
			}
			hs.tt.Advance(motionCaretTrail)
			hs.tt.Frame()
			if count, _ := caretTrailChanged(settled, hs.tt.Image(), band); count != 0 {
				t.Fatal("the converged trail returned in a later frame")
			}
			if hs.tt.FramePending() {
				t.Fatal("the caret still requests animation frames after its trail converged")
			}
		})
	}
}

func TestCaretTrailClearsOnImmediateMoves(t *testing.T) {
	for _, option := range []string{"disabled", "reduced motion", "focus loss", "scroll"} {
		t.Run(option, func(t *testing.T) {
			hs, editor := caretTrailHarness(t, "source")
			origin := caretTrailOrigin(t, hs, editor)
			start := editor.caretPaint.glide.to
			caretTrailMove(hs, "source", ui.KeyRight)
			hs.tt.Advance(motionCaret / 3)
			hs.tt.Frame()
			target := editor.caretPaint.glide.to
			switch option {
			case "disabled":
				hs.a.settings.SmoothCaret = false
			case "reduced motion":
				hs.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			case "focus loss":
				hs.tt.SetFocused(false)
			case "scroll":
				hs.tt.Scroll(origin.X+hs.a.font.charW*14, origin.Y+hs.a.font.lineH/2, 0, 40)
			}
			hs.tt.Frame()
			immediate := caretTrailSnapshot(hs)
			hs.tt.Advance(motionCaret + motionCaretTrail)
			hs.tt.Frame()
			if count, _ := caretTrailChanged(immediate, hs.tt.Image(), caretTrailBand(origin, start, target)); count != 0 {
				t.Fatalf("%s left a visible trail that converged only later", option)
			}
		})
	}
}

func TestCaretTrailSurvivesDirectionChange(t *testing.T) {
	hs, editor := caretTrailHarness(t, "source")
	origin := caretTrailOrigin(t, hs, editor)
	start := editor.caretPaint.glide.to
	caretTrailMove(hs, "source", ui.KeyRight)
	hs.tt.Advance(motionCaret / 2)
	hs.tt.Frame()
	before := editor.caretPaint.glide.position(hs.tt.Now())
	caretTrailMove(hs, "source", ui.KeyLeft)
	after := editor.caretPaint.glide.position(hs.tt.Now())
	if after != before {
		t.Fatalf("changing direction jumped the visible caret from %v to %v", before, after)
	}
	turned := caretTrailSnapshot(hs)
	hs.tt.Advance(motionCaret / 3)
	hs.tt.Frame()
	returning := caretTrailSnapshot(hs)
	shown := editor.caretPaint.glide.position(hs.tt.Now())
	hs.tt.Advance(motionCaret + motionCaretTrail)
	hs.tt.Frame()
	settled := hs.tt.Image()
	if count, _ := caretTrailChanged(turned, settled, caretTrailBand(origin, start, before)); count == 0 {
		t.Fatal("changing direction erased the preceding trail")
	}
	if count, _ := caretTrailChanged(returning, settled, caretTrailBand(origin, shown, before)); count == 0 {
		t.Fatal("the returning caret has no visible trail behind it")
	}
}

func TestCaretTrailClearsOverLiveBlock(t *testing.T) {
	for _, action := range []string{"scroll", "click"} {
		t.Run(action, func(t *testing.T) {
			hs := newHarness(t, false)
			const cellText = "First cell"
			source := "a" + strings.Repeat(" ", 60) + "z\n\n| a | b |\n|---|---|\n| " + cellText + " | 2 |\n\n" + strings.Repeat("more\n", 80)
			editor := openLive(t, hs, "block-trail.md", source, 1)
			hs.place(editor, 1, 1)
			editor.vscroll.Focus()
			hs.settle()
			var table *liveView
			for _, block := range editor.liveViews {
				table = block
			}
			if table == nil || table.at.W <= 0 || table.at.H <= 0 || table.frame != hs.a.frame {
				t.Fatal("the table overlay is not rendered")
			}
			start := editor.caretPaint.glide.to
			caretTrailMove(hs, "live", ui.KeyRight)
			hs.tt.Advance(motionCaret / 2)
			hs.tt.Frame()
			target := editor.caretPaint.glide.to
			beforeScroll := editor.vscroll.Y
			cell := hs.detailBounds(cellText)
			x, y := cell.X+cell.W/2, cell.Y+cell.H/2
			switch action {
			case "scroll":
				hs.tt.Scroll(x, y, 0, 4)
				if editor.vscroll.Y <= beforeScroll {
					t.Fatal("the wheel over the table did not scroll the editor")
				}
			case "click":
				hs.tt.ClickAt(x, y)
				hs.tt.Frame()
				if !hs.tt.Focused(locales.MarkdownTableCell("2", "1")) || !hs.tt.HasText("b") || editor.caretPaint.glide.ok {
					t.Fatal("clicking the grid did not transfer the caret from the document to its cell input")
				}
			}
			hs.tt.Frame()
			origin := caretTrailOrigin(t, hs, editor)
			immediate := caretTrailSnapshot(hs)
			hs.tt.Advance(motionCaretJump + motionCaretTrail)
			hs.tt.Frame()
			band := caretTrailBand(origin, start, target)
			if count, strength := caretTrailChanged(immediate, hs.tt.Image(), band); count != 0 {
				t.Fatalf("%s over the table retained a trailing path: changed=%d strength=%d", action, count, strength)
			}
		})
	}
}

func TestCaretTrailConnectsAroundCorner(t *testing.T) {
	hs, editor := editHarness(t, "corner.txt", strings.Repeat(" ", 60)+"\n"+strings.Repeat(" ", 60)+"\n")
	hs.place(editor, 1, 1)
	editor.vscroll.Focus()
	hs.settle()
	origin := caretTrailOrigin(t, hs, editor)
	caretTrailMove(hs, "source", ui.KeyRight)
	hs.tt.Advance(motionCaret / 2)
	hs.tt.Frame()
	hs.tt.Key(0, ui.KeyDown)
	hs.tt.Advance(motionCaret / 2)
	hs.tt.Frame()
	shown := editor.caretPaint.glide.position(hs.tt.Now())
	points := editor.caretPaint.trail.positions(shown, hs.tt.Now())
	moving := caretTrailSnapshot(hs)
	left, top := points[0].X, points[0].Y
	right, bottom := left, top
	for _, point := range points {
		left, top = min(left, point.X), min(top, point.Y)
		right, bottom = max(right, point.X), max(bottom, point.Y)
	}
	if right-left < 3 || bottom-top < 3 {
		t.Fatalf("the keyboard movement did not produce a corner: %v", points)
	}
	hs.tt.Advance(motionCaret + motionCaretTrail)
	hs.tt.Frame()
	settled := hs.tt.Image()
	bounds := image.Rect(int(math.Floor(float64(origin.X+left-2))), int(math.Floor(float64(origin.Y+top-2))),
		int(math.Ceil(float64(origin.X+right+2))), int(math.Ceil(float64(origin.Y+bottom+shown.H+2))))
	visible := make(map[image.Point]bool)
	for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
		for column := bounds.Min.X; column < bounds.Max.X; column++ {
			if caretTrailDifference(moving.RGBAAt(column, row), settled.RGBAAt(column, row)) > 3 {
				visible[image.Pt(column, row)] = true
			}
		}
	}
	landmarks := make(map[image.Point]bool)
	for _, point := range points {
		pixel := image.Pt(int(math.Round(float64(origin.X+point.X))), int(math.Round(float64(origin.Y+point.Y+shown.H/2))))
		found := false
		for offsetY := -2; offsetY <= 2 && !found; offsetY++ {
			for offsetX := -2; offsetX <= 2; offsetX++ {
				candidate := pixel.Add(image.Pt(offsetX, offsetY))
				if visible[candidate] {
					landmarks[candidate] = true
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("the corner trail is not rendered near particle %v", point)
		}
	}
	var pending []image.Point
	for landmark := range landmarks {
		pending = []image.Point{landmark}
		delete(visible, landmark)
		break
	}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		delete(landmarks, current)
		for offsetY := -1; offsetY <= 1; offsetY++ {
			for offsetX := -1; offsetX <= 1; offsetX++ {
				next := current.Add(image.Pt(offsetX, offsetY))
				if visible[next] {
					delete(visible, next)
					pending = append(pending, next)
				}
			}
		}
	}
	if len(landmarks) != 0 {
		t.Fatal("the corner has disconnected trailing path segments")
	}
}

func caretTrailAssertConnected(t *testing.T, moving, settled *image.RGBA, bounds image.Rectangle) {
	t.Helper()
	if bounds.Dx() < 4 || bounds.Dy() < 2 {
		t.Fatalf("no measurable trailing path: %v", bounds)
	}
	visible := make(map[image.Point]bool)
	for column := bounds.Min.X; column < bounds.Max.X; column++ {
		covered := false
		for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
			if caretTrailDifference(moving.RGBAAt(column, row), settled.RGBAAt(column, row)) > 3 {
				visible[image.Pt(column, row)] = true
				covered = true
			}
		}
		if !covered {
			t.Fatalf("the trailing path has a gap at column %d in %v", column, bounds)
		}
	}
	for seed := range visible {
		pending := []image.Point{seed}
		delete(visible, seed)
		left, right := seed.X, seed.X
		for len(pending) > 0 {
			current := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			left, right = min(left, current.X), max(right, current.X)
			for offsetY := -1; offsetY <= 1; offsetY++ {
				for offsetX := -1; offsetX <= 1; offsetX++ {
					next := current.Add(image.Pt(offsetX, offsetY))
					if visible[next] {
						delete(visible, next)
						pending = append(pending, next)
					}
				}
			}
		}
		if left == bounds.Min.X && right == bounds.Max.X-1 {
			return
		}
	}
	t.Fatal("the trailing path has no connected pixels from its tail to the caret")
}

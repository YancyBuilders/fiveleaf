package main

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

// Draw only the open picker so a row's name cannot find the title bar's
// matching project or branch text instead of the row being measured.
func gitPickerOnly(hs *harness, view func(*ui.Context)) {
	a := hs.a
	hs.tt = ui.NewTester(func(c *ui.Context) {
		a.drain()
		a.pal = a.applyTheme(c)
		ui.Column(c).Fill().Background(a.pal.editor)
		view(c)
	}, 1200, 760)
	hs.tt.SetDark(true)
	hs.settle()
}

// assertGitPickerRow checks the painted text, rather than just the accessible
// label, because an opaque selection layer can cover an otherwise present row.
func assertGitPickerRow(t *testing.T, hs *harness, label string, detail, selected bool) ui.Rect {
	t.Helper()
	r, ok := hs.tt.Find(label)
	if !ok || r.W < 400 || r.H < 20 {
		t.Fatalf("Git picker row %q has no visible bounds: %v", label, r)
	}
	img := hs.tt.Image()
	background := img.RGBAAt(int(r.X+r.W-4), int(r.Y+r.H/2))
	back := ui.RGB(background.R, background.G, background.B)
	textPixels := func(region ui.Rect) int {
		n := 0
		for y := int(region.Y); y < int(region.Y+region.H); y++ {
			for x := int(region.X); x < int(region.X+region.W); x++ {
				if !image.Pt(x, y).In(img.Bounds()) {
					continue
				}
				pixel := img.RGBAAt(x, y)
				if contrast(ui.RGB(pixel.R, pixel.G, pixel.B), back) >= 2 {
					n++
				}
			}
		}
		return n
	}
	text := ui.Rect{X: r.X + 36, Y: r.Y + 4, W: r.W/2 - 36, H: r.H - 8}
	if detail {
		text.H = r.H/2 - 4
	}
	if n := textPixels(text); n < 20 {
		t.Errorf("Git picker row %q title is obscured: %d text pixels", label, n)
	}
	if detail {
		text.Y, text.H = r.Y+r.H/2, r.H/2-4
		if n := textPixels(text); n < 20 {
			t.Errorf("Git picker row %q detail is obscured: %d text pixels", label, n)
		}
	}
	outside := img.RGBAAt(int(r.X-2), int(r.Y+r.H/2))
	mark := pickerMarkerPixels(img, ui.Rect{X: r.X + 1, Y: r.Y + r.H/2 - 5, W: 5, H: 10}, back, ui.RGB(outside.R, outside.G, outside.B))
	if selected && mark < 10 {
		t.Errorf("selected Git picker row %q lacks the leading marker: %d pixels", label, mark)
	} else if !selected && mark > 3 {
		t.Errorf("previous Git picker row %q retains its marker: %d pixels", label, mark)
	}
	return r
}

func TestWorktreePickerOneDarkProSelectionRemainsVisible(t *testing.T) {
	withAppearancePlugins(t)
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	a.settings.ProjectTabs = true
	a.setTheme(themeOneDarkPro)
	hs.settle()
	if err := hs.tt.Click(locales.WorktreeStatusTip("main")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	awaitWorktrees(t, hs)
	gitPickerOnly(hs, a.repository().worktreePickerView)
	current := filepath.Base(a.root)
	first := assertGitPickerRow(t, hs, current, true, true)
	assertGitPickerRow(t, hs, "side", true, false)
	hs.shot("worktree-picker-one-dark-pro-current")
	// Keyboard selection must paint immediately, without waiting for a fade.
	hs.tt.Key(0, ui.KeyDown)
	assertGitPickerRow(t, hs, "side", true, true)
	if got := assertGitPickerRow(t, hs, current, true, false); got != first {
		t.Errorf("selection moved the current worktree row: %v -> %v", first, got)
	}
	hs.shot("worktree-picker-one-dark-pro-linked")
	hs.tt.Key(0, ui.KeyUp)
	hs.tt.Key(0, ui.KeyUp)
	assertGitPickerRow(t, hs, locales.WorktreeCreateEllipsis(), false, true)
	assertGitPickerRow(t, hs, current, true, false)
}

func TestBranchPickerOneDarkProSelectionRemainsVisible(t *testing.T) {
	withAppearancePlugins(t)
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	a.settings.ProjectTabs = true
	a.setTheme(themeOneDarkPro)
	hs.settle()
	if err := hs.tt.Click(locales.BranchLabel("main")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	gitPickerOnly(hs, a.repository().branchPickerView)
	first := assertGitPickerRow(t, hs, "main", true, true)
	hs.shot("branch-picker-one-dark-pro-current")
	hs.tt.Key(0, ui.KeyDown)
	assertGitPickerRow(t, hs, "side", true, true)
	if got := assertGitPickerRow(t, hs, "main", true, false); got != first {
		t.Errorf("selection moved the current branch row: %v -> %v", first, got)
	}
	hs.shot("branch-picker-one-dark-pro-linked")
	hs.tt.Key(0, ui.KeyUp)
	hs.tt.Key(0, ui.KeyUp)
	assertGitPickerRow(t, hs, locales.CreateBranchEllipsis(), false, true)
	assertGitPickerRow(t, hs, "main", true, false)
}

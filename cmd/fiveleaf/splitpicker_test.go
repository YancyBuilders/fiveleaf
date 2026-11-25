package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func splitButtonBounds(hs *harness) ui.Rect {
	hs.t.Helper()
	bounds, ok := hs.tt.Find(hs.a.withKey(locales.CmdViewSplitRight(), "view.splitRight"))
	if !ok || bounds.W <= 0 || bounds.H <= 0 {
		hs.t.Fatal("split button not visible")
	}
	return bounds
}

func hoverSplitButton(hs *harness) ui.Rect {
	hs.t.Helper()
	button := splitButtonBounds(hs)
	hs.tt.Move(button.X+button.W/2, button.Y+button.H/2)
	hs.tt.Advance(hoverDwell + motionShow)
	hs.tt.Frame()
	if _, ok := hs.tt.Find(locales.SplitEditor()); !ok {
		hs.t.Fatal("no direction picker after dwelling on the split button")
	}
	return button
}

func TestSplitButtonClickAndHoverIntent(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	button := splitButtonBounds(hs)
	px, py := button.X+button.W/2, button.Y+button.H/2
	tt.Move(px, py)
	tt.Advance(hoverDwell / 2)
	if _, ok := tt.Find(locales.SplitEditor()); ok {
		t.Fatal("picker opened before hover intent")
	}
	tt.Move(px-150, py+150)
	tt.Advance(hoverDwell)
	if a.splitPicker.open || len(a.cols) != 1 {
		t.Fatal("passing over the button opened the picker or split the pane")
	}
	tt.ClickAt(px, py)
	hs.settle()
	if len(a.cols) != 2 || a.column() != a.cols[1] || a.cols[1].body.Bounds(a.review.ctx).X <= a.cols[0].body.Bounds(a.review.ctx).X || a.splitPicker.open {
		t.Fatal("a direct click did not split right exactly once and leave the picker closed")
	}
	button = hoverSplitButton(hs)
	if len(a.cols) != 2 || a.column() != a.cols[1] || splitButtonBounds(hs) != button {
		t.Fatal("opening the picker moved the button, split the pane or stole focus")
	}
	tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.splitPicker.open || len(a.cols) != 2 {
		t.Fatal("Escape did not dismiss the picker until the pointer leaves")
	}
	tt.Move(0, 0)
	hoverSplitButton(hs)
	tt.Move(100, 300)
	tt.Advance(hoverLeave + time.Millisecond)
	if _, ok := tt.Find(locales.SplitEditor()); ok || len(a.cols) != 2 {
		t.Fatal("leaving the picker did not dismiss it without splitting")
	}
	button = hoverSplitButton(hs)
	tt.ClickAt(button.X+button.W/2, button.Y+button.H/2)
	hs.settle()
	if len(a.cols) != 3 || a.column() != a.cols[1] || a.splitPicker.open {
		t.Fatal("clicking the original button with the picker open did not split its own pane to the right")
	}
}

func TestSplitPickerPlacesNewPane(t *testing.T) {
	for _, choice := range []struct {
		label     string
		direction splitDirection
	}{{locales.CmdViewSplitLeft(), splitLeft}, {locales.CmdViewSplitRight(), splitRight},
		{locales.CmdViewSplitUp(), splitUp}, {locales.CmdViewSplitDown(), splitDown}} {
		t.Run(choice.label, func(t *testing.T) {
			hs := newHarness(t, true)
			a, tt := hs.a, hs.tt
			a.openFile(hs.path("main.go"), true)
			source := a.column()
			a.splitRight()
			unrelated := a.column()
			a.openFile(hs.path("go.mod"), true)
			hs.settle()
			unchanged := unrelated.body.Bounds(a.review.ctx)
			button := hoverSplitButton(hs)
			option, ok := tt.Find(choice.label)
			if !ok || option.Y <= button.Y {
				t.Fatalf("choice %q not in popup: %+v", choice.label, option)
			}
			tt.Move(button.X+button.W/2, button.Y+button.H+1)
			tt.Advance(hoverLeave / 2)
			tt.Move(option.X+option.W/2, option.Y+option.H/2)
			tt.Advance(hoverLeave + motionShow)
			if !a.splitPicker.open {
				t.Fatal("picker closed as the pointer crossed into it")
			}
			if err := tt.Click(choice.label); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			fresh := a.column()
			if len(a.cols) != 3 || a.splitPicker.open || fresh == source || fresh == unrelated || fresh.tab().ed.document != source.tab().ed.document {
				t.Fatal("choice did not split and focus the pane under its button")
			}
			old, added := source.body.Bounds(a.review.ctx), fresh.body.Bounds(a.review.ctx)
			if choice.direction.vertical() {
				if old.X != added.X || math.Abs(float64(old.W-added.W)) > 1 || (added.Y > old.Y) != choice.direction.after() || added.Y == old.Y {
					t.Fatalf("wrong vertical split: %v and %v", old, added)
				}
			} else if old.Y != added.Y || (added.X > old.X) != choice.direction.after() || added.X == old.X {
				t.Fatalf("wrong horizontal split: %v and %v", old, added)
			}
			if after := unrelated.body.Bounds(a.review.ctx); after != unchanged {
				t.Fatalf("unrelated pane moved: %v -> %v", unchanged, after)
			}
		})
	}
}

func TestNestedPaneResizeRestoreAndReopen(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.run("view.splitDown")
	a.run("view.splitLeft")
	hs.settle()
	top, left, right := a.cols[0], a.cols[1], a.cols[2]
	topBox, rightBox := top.body.Bounds(a.review.ctx), right.body.Bounds(a.review.ctx)
	px, py := topBox.X+topBox.W/2, topBox.Y+topBox.H+2
	hs.drag(px, py, px, py+55)
	topAfter := top.body.Bounds(a.review.ctx)
	if topAfter.H-topBox.H < 50 || topAfter.W != topBox.W || right.body.Bounds(a.review.ctx).W != rightBox.W {
		t.Fatalf("horizontal divider did not resize both lower panes: %v -> %v", topBox, topAfter)
	}
	before := []ui.Rect{topAfter, left.body.Bounds(a.review.ctx), right.body.Bounds(a.review.ctx)}
	a.activate(top, top.active, true)
	a.closeTab(top.tab(), nil, nil)
	hs.settle()
	if len(a.cols) != 2 || left.body.Bounds(a.review.ctx).Y != topBox.Y {
		t.Fatal("the lower group did not take the closed pane's space")
	}
	a.reopenClosed()
	hs.settle()
	if len(a.cols) != 3 || a.cols[0] != a.column() || a.column().body.Bounds(a.review.ctx) != before[0] {
		t.Fatal("the closed pane did not return above its two neighbors")
	}
	a.rememberTabs()
	saved, err := json.Marshal(a.window.Session)
	if err != nil {
		t.Fatal(err)
	}
	w := a.settings.NewWindow()
	w.Folder = hs.root
	if err := json.Unmarshal(saved, &w.Session); err != nil {
		t.Fatal(err)
	}
	b := newApp(hs.h, a.settings, w)
	t.Cleanup(b.shutdown)
	tester := ui.NewTester(b.view, 1200, 760)
	step(tester)
	step(tester)
	if len(b.cols) != 3 || b.col != 0 {
		t.Fatalf("restored %d panes with focus %d", len(b.cols), b.col)
	}
	for index, col := range b.cols {
		if box := col.body.Bounds(b.review.ctx); box != before[index] {
			t.Fatalf("restored pane %d: %v, want %v", index, box, before[index])
		}
	}
}

func TestInvalidPaneLayoutFallsBackToColumns(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.splitRight()
	a.restorePaneLayout([]settings.PaneTile{{Vertical: true, Weight: 1, Children: []settings.PaneTile{{}, {}, {}}}}, a.cols)
	if a.panes.savedLayout() != nil || a.cols[0].parent != &a.panes || a.cols[1].parent != &a.panes {
		t.Fatal("invalid layout replaced the working horizontal layout")
	}
}

func TestSplitHorizontalNavigationSkipsStackedPanes(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	top := a.column()
	a.run("view.splitDown")
	lower := a.column()
	a.run("view.focusLeftColumn")
	if a.column() != lower || a.sideColumn(top) != nil {
		t.Fatal("a vertically adjacent pane was treated as being to the side")
	}
	a.openFileToSide(hs.path("go.mod"))
	right := a.column()
	if len(a.cols) != 3 || right == top || a.sideColumn(lower) != right {
		t.Fatal("open to side did not place the file beside the lower pane")
	}
	a.run("view.focusLeftColumn")
	if a.column() != lower {
		t.Fatal("left focus selected a pane above the source")
	}
}

func TestSplitPickerFitsWindowAndDismissesOutside(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.window.SidebarClosed, a.window.PanelOpen = true, false
	a.openFile(hs.path("main.go"), true)
	for _, window := range []struct {
		name  string
		width int
	}{{"narrow", 390}, {"wide", 1440}} {
		tt.SetSize(window.width, 640)
		hs.settle()
		hoverSplitButton(hs)
		for _, label := range []string{locales.CmdViewSplitLeft(), locales.CmdViewSplitRight(), locales.CmdViewSplitUp(), locales.CmdViewSplitDown()} {
			box, ok := tt.Find(label)
			if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > float32(window.width) || box.Y+box.H > 640 {
				t.Fatalf("%s: option %q outside window: %+v", window.name, label, box)
			}
		}
		hs.shot("split-picker-" + window.name)
		tt.ClickAt(55, 310)
		step(tt)
		if _, open := tt.Find(locales.SplitEditor()); open || len(a.cols) != 1 {
			t.Fatalf("%s: outside click did not dismiss without splitting", window.name)
		}
	}
}

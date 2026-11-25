package main

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/git"
	"fiveleaf/internal/settings"
)

func TestPaneHistoryRestoresSuccessiveClosures(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	top := a.column()
	a.run("view.splitDown")
	a.splitRight()
	right := a.column()
	before := a.paneSnapshot()
	a.closeTab(top.tab(), nil, nil)
	a.closeTab(right.tab(), nil, nil)
	if len(a.cols) != 1 {
		t.Fatal("closing two panes did not leave one")
	}
	a.reopenClosed()
	a.reopenClosed()
	if !reflect.DeepEqual(a.paneSnapshot(), before) {
		t.Fatal("reopening two panes did not restore the top pane across the entire row")
	}
}

func TestPaneHistoryPreservesLockedWidths(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	left := a.column()
	a.splitRight()
	middle := a.column()
	a.splitRight()
	a.balanceColumns()
	a.toggleColumnLock(left)
	before := a.paneSnapshot()
	a.closeTab(middle.tab(), nil, nil)
	a.reopenClosed()
	if !reflect.DeepEqual(a.paneSnapshot(), before) {
		t.Fatal("reopening the middle pane took width from its locked neighbor")
	}
}

func TestPaneLayoutKeepsLockedSubtreeInPlace(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.run("view.splitDown")
	lower := a.column()
	a.splitRight()
	locked := a.column()
	a.toggleColumnLock(locked)
	before := locked.layoutBounds()
	a.closeTab(lower.tab(), nil, nil)
	if lower.tab() != nil || lower.parent == nil || locked.layoutBounds() != before {
		t.Fatal("closing the neighboring tab widened a locked pane")
	}
}

func TestPaneLayoutResizeMinimumAndBalanceAfterCollapse(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.splitRight()
	upper := a.column()
	a.run("view.splitDown")
	lowerLeft := a.column()
	a.splitRight()
	lowerRight := a.column()
	hs.settle()
	box := upper.body.Bounds(a.review.ctx)
	hs.drag(box.X+1, box.Y+15, box.X+box.W-10, box.Y+15)
	for _, col := range []*column{lowerLeft, lowerRight} {
		if width := math.Round(float64(col.body.Bounds(a.review.ctx).W)); width != minColumnW-1 {
			t.Fatalf("outer divider ignored inner pane minimum: width = %v", width)
		}
	}
	a.closeTab(upper.tab(), nil, nil)
	a.balanceColumns()
	hs.settle()
	first := a.cols[0].body.Bounds(a.review.ctx)
	for _, col := range a.cols[1:] {
		box := col.body.Bounds(a.review.ctx)
		if box.Y != first.Y || math.Abs(math.Round(float64(box.W))-math.Round(float64(first.W))) > 1 {
			t.Fatalf("balance after collapse: %v, %v", first, box)
		}
	}
}

func TestPaneLayoutStaysWithSwitchedFolder(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.run("view.splitDown")
	a.toggleColumnLock(a.column())
	hs.settle()
	other := workspaceFixture(t)
	a.settings.AddRecent(other)
	a.settings.Stash(&settings.Window{Folder: other, Session: settings.Session{Columns: []settings.Column{
		{Weight: 1, Tabs: []settings.Tab{{File: filepath.Join(other, "README.md")}}},
	}}})
	a.switchTo(other)
	hs.settle()
	if len(a.cols) != 1 || a.column().locked || a.activeTab().name() != "README.md" {
		t.Fatal("the destination folder inherited the previous layout")
	}
	a.switchTo(hs.root)
	hs.settle()
	if len(a.cols) != 2 || !a.cols[1].locked || a.cols[1].layoutBounds().Y <= a.cols[0].layoutBounds().Y {
		t.Fatal("the original folder did not recover its locked lower pane")
	}
}

func TestSplitDiffPaneReopensAndRestores(t *testing.T) {
	for _, kind := range []string{"changes", "commit"} {
		t.Run(kind, func(t *testing.T) {
			hs := newHarness(t, true)
			a := hs.a
			diff := newDiffView(layoutSplit)
			if kind == "commit" {
				diff = newCommitView(layoutSplit, "abc123", git.Commit{})
			}
			diff.repoRoot = hs.path("web")
			source := a.column()
			a.addTab(source, 0, &colTab{diff: diff})
			a.activate(source, 0, true)
			a.run("view.splitDown")
			a.closeTab(a.column().tab(), nil, nil)
			a.reopenClosed()
			if len(a.cols) != 2 || a.column() == source || !a.column().tab().diff.same(diff) {
				t.Fatal("reopening the diff activated its other pane instead")
			}
			w := a.settings.NewWindow()
			w.Folder, w.Session = hs.root, a.session()
			b := newApp(hs.h, a.settings, w)
			t.Cleanup(b.shutdown)
			tester := ui.NewTester(b.view, 1200, 760)
			step(tester)
			if len(b.cols) != 2 || !b.cols[1].tab().diff.same(diff) || b.cols[1].layoutBounds().Y <= b.cols[0].layoutBounds().Y {
				t.Fatal("session restore lost the lower diff pane")
			}
		})
	}
}

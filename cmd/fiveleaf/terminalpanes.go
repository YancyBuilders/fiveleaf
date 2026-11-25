package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fiveleaf/internal/remote"
	"github.com/egoist/mygo/ui"
)

func (s *session) nextShellDir() string {
	if s.term == nil || remote.Is(s.dir) {
		return s.dir
	}
	reported := s.term.Dir()
	if !filepath.IsAbs(reported) || remote.Is(reported) || strings.HasPrefix(filepath.ToSlash(reported), "//") {
		return s.dir
	}
	if info, err := os.Stat(reported); err == nil && info.IsDir() {
		return reported
	}
	return s.dir
}

type terminalPane struct {
	parent, first, second *terminalPane
	shell                 *session
	vertical              bool
	splitShell            bool
	share                 float32
}

func (a *app) splitPanelTerminal(source *session, direction splitDirection) {
	if source == nil || a.panelTerm != source || !a.window.PanelOpen {
		return
	}
	fresh := a.startTerminal(source.nextShellDir())
	if fresh == nil {
		return
	}
	leaf := source.pane
	if leaf == nil {
		leaf = &terminalPane{shell: source}
	}
	source.pane = &terminalPane{parent: leaf, shell: source, splitShell: leaf.splitShell}
	fresh.pane = &terminalPane{parent: leaf, shell: fresh, splitShell: true}
	*leaf = terminalPane{parent: leaf.parent, first: source.pane, second: fresh.pane, vertical: direction.vertical(), share: 0.5}
	if !direction.after() {
		leaf.first, leaf.second = leaf.second, leaf.first
	}
	a.panelTerm = fresh
	a.window.TerminalOpen = true
	a.focusTerminal()
	a.settings.Changed()
}

func (s *session) leavePanelGroup() *session {
	leaf := s.pane
	if leaf == nil {
		return nil
	}
	s.pane = nil
	parent := leaf.parent
	sibling := parent.first
	if sibling == leaf {
		sibling = parent.second
	}
	sibling.parent = parent.parent
	switch {
	case sibling.parent == nil:
		if sibling.shell != nil {
			sibling.shell.pane = nil
		}
	case sibling.parent.first == parent:
		sibling.parent.first = sibling
	default:
		sibling.parent.second = sibling
	}
	for sibling.shell == nil {
		sibling = sibling.first
	}
	return sibling.shell
}

func (a *app) panelTerminalViews(c *ui.Context, selected *session, viewport ui.Rect) {
	bounds := ui.Rect{W: 100, H: 100}
	root := selected.pane
	if root == nil {
		a.panelTerminalLeaf(c, selected, bounds)
		return
	}
	for root.parent != nil {
		root = root.parent
	}
	a.drawTerminalPane(c, root, bounds, viewport)
}

func (pane *terminalPane) minimumExtent(vertical bool) float32 {
	if pane.shell != nil {
		if vertical {
			return 48
		}
		return minPanelW
	}
	first, second := pane.first.minimumExtent(vertical), pane.second.minimumExtent(vertical)
	if pane.vertical == vertical {
		return first + second
	}
	return max(first, second)
}

func (a *app) drawTerminalPane(c *ui.Context, pane *terminalPane, region, viewport ui.Rect) {
	if pane.shell != nil {
		a.panelTerminalLeaf(c, pane.shell, region)
		return
	}
	edge, size := edgeLeft, viewport.W*region.W/100
	if pane.vertical {
		edge, size = edgeTop, viewport.H*region.H/100
	}
	first, second := pane.first.minimumExtent(pane.vertical), pane.second.minimumExtent(pane.vertical)
	share := first / (first + second)
	if size >= first+second {
		share = clamp(pane.share, first/size, 1-second/size)
	}
	axis := paneTile{vertical: pane.vertical}
	secondBox := axis.sliceBounds(region, share, 1)
	a.drawTerminalPane(c, pane.first, axis.sliceBounds(region, 0, share), viewport)
	a.drawTerminalPane(c, pane.second, secondBox, viewport)
	handle := a.splitter(c.Key(pane), edge, func(delta float32) {
		if size >= first+second {
			pane.share = clamp(share+delta/size, first/size, 1-second/size)
		}
	}).KeepFocus().LeftPercent(secondBox.X).TopPercent(secondBox.Y)
	if pane.vertical {
		handle.RightPercent(100 - region.X - region.W)
	} else {
		handle.BottomPercent(100 - region.Y - region.H)
	}
}

func (a *app) panelTerminalLeaf(c *ui.Context, shell *session, region ui.Rect) {
	if shell != a.panelTerm && (shell.view.FocusWithin(c) || shell.findInput.Focused(c)) {
		a.panelTerm, a.focus = shell, paneTerminal
	}
	box := ui.Box(c.Key(shell.id)).Absolute().LeftPercent(region.X).TopPercent(region.Y).
		WidthPercent(region.W).HeightPercent(region.H).Clip().
		BorderWidth(unit(region.Y > 0), 0, 0, unit(region.X > 0)).BorderColor(a.pal.border)
	if item, ok := ui.Drop[dragItem](box); ok && item.file != nil {
		a.post(func() { a.pastePaths(shell, item.file.paths()) })
	}
	if paths := box.DroppedFiles(); paths != nil {
		paths = slices.Clone(paths)
		a.post(func() { a.pastePaths(shell, paths) })
	}
	box.Children(func() {
		a.terminalView(c, shell)
		if box.FileDragOver() {
			a.dropMark(c)
		}
	})
}

package main

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

const debugGutterW = 24

type breakpointGutter struct {
	points          []breakpoint
	paused, hovered int
	debugging       bool
}

func (a *app) debugGutterLine(ed *editor, x, y float32) int {
	if ed.gutterW == 0 || ed.path == "" || ed.locked() || a.font.lineH <= 0 || y < 0 || y >= ed.lastView.H {
		return -1
	}
	if left := x - ed.hscroll.X; left < 0 || left >= debugGutterW {
		return -1
	}
	if row := int(y / a.font.lineH); row < len(ed.sticky) {
		return ed.sticky[row]
	}
	geometry := a.geometry(ed, ed.buf.Snapshot().LineCount())
	contentY := y + ed.vscroll.Y - editorPadY
	if contentY < 0 || contentY >= geometry.height() {
		return -1
	}
	return geometry.at(contentY)
}

func (gutter breakpointGutter) at(line int) (breakpoint, bool) {
	index, found := slices.BinarySearchFunc(gutter.points, line, func(point breakpoint, line int) int { return cmp.Compare(point.line, line) })
	if !found {
		return breakpoint{}, false
	}
	return gutter.points[index], true
}

func (a *app) breakpointGutter(c *ui.Context, ed *editor, area ui.Element) breakpointGutter {
	gutter := breakpointGutter{points: a.debug.at(ed.path), paused: a.debug.pausedLine(ed.path), debugging: a.debug.active(), hovered: -1}
	x, y, over := area.PointerPosition()
	if !over || !area.Hovered() || ed.selecting || c.Modifiers() != 0 {
		return gutter
	}
	line := a.debugGutterLine(ed, x, y)
	if line < 0 {
		return gutter
	}
	gutter.hovered = line
	number := strconv.Itoa(line + 1)
	text := locales.DebugAddBreakpointAt(number)
	if point, exists := gutter.at(line); exists {
		text = locales.DebugRemoveBreakpointAt(number)
		if gutter.debugging && !point.verified {
			text += "\n" + cmp.Or(point.message, locales.DebugUnverifiedBreakpoint())
		}
	}
	area.Cursor(ui.CursorPointer).Description(text)
	anchor := ui.Box(c.Key("breakpoint-tip-anchor")).Absolute().Left(ed.hscroll.X).Top(y).Size(debugGutterW, 1).PassThrough()
	ui.TooltipBase(c, area, func(tip ui.Element) {
		metrics := a.pal.metricsOr()
		tip.AttachTo(anchor, ui.AnchorRight, ui.AnchorLeft).MarginStart(metrics.gap).MaxWidth(c.Theme().Space(80)).
			Padding(metrics.gap, 2*metrics.gap).Radius(metrics.radius).FontSize(metrics.textSmall).
			Background(a.pal.inverse).TextColor(a.pal.inverseText).Children(func() {
			ui.Text(c, text)
		})
	})
	return gutter
}

func (gutter breakpointGutter) paint(pt *ui.Painter, pal *palette, line int, x, centerY float32) {
	point, exists := gutter.at(line)
	dot := ui.Rect{X: x + debugGutterW/2 - 5, Y: centerY - 5, W: 10, H: 10}
	switch {
	case exists && gutter.debugging && !point.verified:
		pt.Stroke(dot, pal.dangerFg, 5, 2)
	case exists:
		pt.Fill(dot, pal.dangerFg, 5)
	case line == gutter.hovered:
		pt.Fill(dot, pal.dangerFg.Alpha(0.32), 5)
	}
	if line == gutter.paused {
		left := x + debugGutterW/2 - 6
		if exists || line == gutter.hovered {
			left = x + debugGutterW - 9
		}
		pt.Icon(iconChevronRight, ui.Rect{X: left, Y: centerY - 6, W: 12, H: 12}, pal.accent)
	}
}

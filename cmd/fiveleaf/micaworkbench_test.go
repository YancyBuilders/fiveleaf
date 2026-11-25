package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMicaWorkbenchSurfacesFollowHue(t *testing.T) {
	meeting := newRoundtableHarness(t, nil)
	hs := meeting.harness
	hs.a.settings.MicaEnabled, hs.a.settings.MicaAnimation = true, false
	hs.a.openFile(hs.path("main.go"), true)
	hs.a.showRoundtable(meeting.w.meeting)
	hs.settle()
	title := "Meeting notification"
	hs.a.fail("%s", title)
	hs.settle()
	check := func(name string, area ui.Rect, before, after *image.RGBA) {
		t.Helper()
		for row := int(area.Y); row < int(area.Y+area.H); row++ {
			for column := int(area.X); column < int(area.X+area.W); column++ {
				if before.RGBAAt(column, row) != after.RGBAAt(column, row) {
					return
				}
			}
		}
		t.Errorf("%s did not show the new Mica hue in %+v", name, area)
	}
	tab := hs.detailBounds(meeting.w.tab.name())
	toast := hs.detailBounds(title)
	pane := meeting.w.tab.tool.pane.Bounds(hs.a.review.ctx)
	center := ui.Rect{X: pane.X + pane.W/3, Y: pane.Y + pane.H/3, W: pane.W / 4, H: pane.H / 4}
	before := micaSnapshot(hs.tt)
	hs.a.settings.MicaHue = 315
	hs.settle()
	after := hs.tt.Image()
	check("selected tab", tab, before, after)
	check("notification", toast, before, after)
	check("roundtable conversation", center, before, after)
	meeting.w.view = roundtableViewGraph
	hs.settle()
	before = micaSnapshot(hs.tt)
	hs.a.settings.MicaHue = 177
	hs.settle()
	check("execution graph", center, before, hs.tt.Image())
}

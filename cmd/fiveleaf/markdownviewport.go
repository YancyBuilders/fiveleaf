package main

import "github.com/egoist/mygo/ui"

type liveViewport struct {
	at                     int
	offset                 float32
	y                      float32
	version                uint64
	parsed                 *liveDoc
	seen, restore, painted bool
}

func (v *liveViewport) target(ed *editor, line int) float32 {
	return max(0, editorPadY+ed.lines.Top(line)-v.offset)
}

func (v *liveViewport) capture(ed *editor, snap ui.TextSnapshot, parsed *liveDoc) {
	top := ed.vscroll.Y - editorPadY
	line := ed.lines.At(top)
	if next := ed.lines.Top(line + 1); top > ed.lines.Top(line) && next > top && next < top+ed.lastView.H/3 && line+1 < snap.LineCount() {
		line = ed.lines.At(next + 0.01)
	}
	v.at, _ = snap.LineRange(line)
	v.offset = editorPadY + ed.lines.Top(line) - ed.vscroll.Y
	v.y, v.version, v.parsed, v.seen = ed.vscroll.Y, snap.Version(), parsed, true
}

func (v *liveViewport) scroll(ed *editor, delta float32) bool {
	if !v.restore || !v.seen || delta == 0 {
		return false
	}
	ed.vscroll.Y = clamp(ed.vscroll.Y+delta, 0, ed.vscroll.MaxY)
	v.capture(ed, ed.buf.Snapshot(), ed.document.live.doc)
	v.painted = false
	return true
}

func (v *liveViewport) restoreLine(ed *editor, snap ui.TextSnapshot, parsed *liveDoc, edited, allow bool) int {
	if !allow || !v.seen || v.painted && !edited && !ed.resync && !near(v.y, ed.vscroll.Y) {
		v.restore = false
	} else if v.seen && near(v.y, ed.vscroll.Y) && v.parsed != parsed && v.version == snap.Version() && ed.typist != ed {
		v.restore = true
	}
	v.painted = false
	if v.restore {
		return snap.LineAt(min(v.at, snap.Len()))
	}
	return -1
}

func (v *liveViewport) afterLayout(c *ui.Context, ed *editor, snap ui.TextSnapshot, parsed *liveDoc) {
	if v.restore {
		line := snap.LineAt(min(v.at, snap.Len()))
		goal := clamp(v.target(ed, line), 0, ed.vscroll.MaxY)
		if !near(goal, ed.vscroll.Y) {
			c.AnimationFrame()
		}
		v.y, v.parsed, v.version, v.painted = ed.vscroll.Y, parsed, snap.Version(), true
	} else if ed.buf.Version() == snap.Version() {
		v.capture(ed, snap, parsed)
	}
}

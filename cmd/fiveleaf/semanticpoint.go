package main

import (
	"sort"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// semanticPoint identifies the rune under the pointer and the word around it.
// Its anchor is the visible part of that word's row, in text-area coordinates.
type semanticPoint struct {
	at, start, end int
	anchor         ui.Rect
}

// Long unbroken text, as in generated files, must not turn a pointer move into
// a scan of the file. Language identifiers fit comfortably within this limit.
const semanticTokenLimit = 4096

func (a *app) semanticPointAt(d *editor, x, y float32) (semanticPoint, bool) {
	if d == nil || a.livePreview(d) || d.lastView.W <= 0 || d.lastView.H <= 0 || a.font.lineH <= 0 {
		return semanticPoint{}, false
	}
	left, right := d.hscroll.X+d.textX, d.hscroll.X+d.lastView.W
	top := float32(len(d.sticky)) * a.font.lineH
	if x < left || x >= right || y < top || y >= d.lastView.H {
		return semanticPoint{}, false
	}
	snap := d.buf.Snapshot()
	tx, ty := x-d.textX, y-editorPadY+d.vscroll.Y
	if snap.Len() == 0 || tx < 0 || ty < 0 || ty >= d.lines.Height() {
		return semanticPoint{}, false
	}
	line := d.lines.At(ty)
	ls, le := snap.LineRange(line)
	rows := d.lines.Rows(line)
	if len(rows) == 0 || ls == le {
		return semanticPoint{}, false
	}
	lineY := d.lines.Top(line)
	row := int((ty - lineY) / a.font.lineH)
	if row < 0 || row >= len(rows) {
		return semanticPoint{}, false
	}
	rowStart, rowEnd := ls+rows[row], le
	if row+1 < len(rows) {
		rowEnd = ls + rows[row+1]
	}
	if rowStart >= rowEnd {
		return semanticPoint{}, false
	}
	rowY := lineY + float32(row)*a.font.lineH
	// A caret at a soft wrap can have upstream affinity. The same boundary
	// has two positions; use the one belonging to the row under the pointer.
	caretX := func(at int) float32 {
		if at == rowStart {
			return 0
		}
		cx, cy, _, ok := d.lines.Caret(at)
		if !ok {
			return -1
		}
		if at == rowEnd && cy != rowY {
			return a.font.width(snap.Slice(rowStart, rowEnd))
		}
		return cx
	}
	// Find the glyph cell, not its nearest insertion caret: the right half
	// of a name's last letter still belongs to the name.
	at := rowStart + sort.Search(rowEnd-rowStart, func(i int) bool {
		return caretX(rowStart+i) > tx
	}) - 1
	if at < rowStart || tx >= caretX(at+1) {
		return semanticPoint{}, false
	}
	wordRune := func(at int) bool {
		r, _ := utf8.DecodeRuneInString(snap.Slice(at, at+1))
		return isWordRune(r)
	}
	if !wordRune(at) {
		return semanticPoint{}, false
	}
	start, end := at, at+1
	for start > ls && at-start < semanticTokenLimit && wordRune(start-1) {
		start--
	}
	for end < le && end-start <= semanticTokenLimit && wordRune(end) {
		end++
	}
	if end-start > semanticTokenLimit {
		return semanticPoint{}, false
	}
	x0 := max(left, d.textX+caretX(max(start, rowStart)))
	x1 := min(right, d.textX+caretX(min(end, rowEnd)))
	y0 := max(top, editorPadY+rowY-d.vscroll.Y)
	y1 := min(d.lastView.H, editorPadY+rowY+a.font.lineH-d.vscroll.Y)
	if x1 <= x0 || y1 <= y0 {
		return semanticPoint{}, false
	}
	return semanticPoint{at: at, start: start, end: end, anchor: ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}}, true
}

package main

import (
	"sort"

	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/segmenter"
)

type livePointerSelection struct {
	active, fromPress bool
	end               int
	line              livePointerLine
	anchorVersion     uint64
	anchorLine        int
	anchorKnown       bool
	anchorNative      bool
}

type livePointerLine struct {
	version    uint64
	start, end int
	native     bool
	boundaries []int
}

func (line *livePointerLine) prepare(snap ui.TextSnapshot, start, end int) {
	if line.version == snap.Version() && line.start == start && line.end == end {
		return
	}
	line.version, line.start, line.end = snap.Version(), start, end
	line.boundaries = line.boundaries[:0]
	text := snap.Slice(start, end)
	line.native = hasRTL(text)
	if line.native || isASCII(text) {
		return
	}
	var breaks segmenter.Segmenter
	breaks.Init([]rune(text))
	clusters := breaks.GraphemeIterator()
	for clusters.Next() {
		line.boundaries = append(line.boundaries, start+clusters.Grapheme().Offset)
	}
	line.boundaries = append(line.boundaries, end)
}

func (line *livePointerLine) offset(index int) int {
	if len(line.boundaries) == 0 {
		return line.start + index
	}
	return line.boundaries[index]
}

func (hit *livePointerSelection) nativeAnchor(snap ui.TextSnapshot, line int) bool {
	if !hit.anchorKnown || hit.anchorVersion != snap.Version() || hit.anchorLine != line {
		hit.anchorKnown, hit.anchorVersion, hit.anchorLine = true, snap.Version(), line
		start, end := snap.LineRange(line)
		hit.anchorNative = end-start > highlightRunes || hasRTL(snap.Slice(start, end))
	}
	return hit.anchorNative
}

func (d *editor) livePointerOffset(x, y float32) int {
	if y < 0 || y >= d.lastView.H || d.lines.Count() == 0 {
		return -1
	}
	x -= d.textX
	y += d.vscroll.Y - editorPadY
	snap := d.buf.Snapshot()
	line := d.lines.At(y)
	start, end := snap.LineRange(line)
	if end-start > highlightRunes {
		return -1
	}
	anchorLine := snap.LineAt(min(d.sel[0], snap.Len()))
	if anchorLine != line && d.liveSelection.nativeAnchor(snap, anchorLine) {
		return -1
	}
	if start == end {
		return start
	}
	row := &d.liveSelection.line
	row.prepare(snap, start, end)
	if row.native {
		return -1
	}
	count := end - start + 1
	if len(row.boundaries) > 0 {
		count = len(row.boundaries)
	}
	next := sort.Search(count, func(index int) bool {
		px, top, height, _ := d.lines.Caret(row.offset(index))
		return top > y || top+height > y && px >= x
	})
	if next == 0 {
		return start
	}
	if next == count {
		return end
	}
	left, right := row.offset(next-1), row.offset(next)
	lx, ly, _, _ := d.lines.Caret(left)
	rx, ry, _, _ := d.lines.Caret(right)
	if ly < ry && y < ry {
		return -1
	}
	if ly == ry && x-lx < rx-x {
		return left
	}
	return right
}

func liveWritingOffset(doc *liveDoc, at, previous int) int {
	if doc == nil {
		return at
	}
	limit := doc.snap.Len()
	for {
		next := at
		doc.visit(max(0, at-1), min(limit, at+1), func(span *liveSpan) {
			if span.kind != liveMark {
				return
			}
			if span.opening && span.start <= at && at < span.end {
				if previous >= span.end && span.start > 0 && doc.snap.Slice(span.start-1, span.start) != "\n" {
					next = span.start
				} else {
					next = span.end
				}
			} else if !span.opening && span.start < at && at < span.end && doc.snap.LineAt(span.from) == doc.snap.LineAt(at) {
				if previous > at {
					next = span.start
				} else {
					next = span.end
				}
			}
		})
		if next == at {
			return at
		}
		at = next
	}
}

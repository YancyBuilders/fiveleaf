package main

import (
	"cmp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/segmenter"
)

func liveDeleteKey(d *editor, back bool) func(*editor) {
	if d.locked() || d.areaInput || len(d.cmds) != 0 || d.document.live == nil {
		return nil
	}
	snap := d.buf.Snapshot()
	doc := d.document.live.mapped
	if doc == nil || doc.snap != snap || snap.Version() != d.version {
		return nil
	}
	sel := d.sel
	from, to := min(sel[0], sel[1]), max(sel[0], sel[1])
	line := snap.LineAt(from)
	code := false
	doc.eachDeco(from, to, func(deco liveDeco) {
		code = code || deco.kind == decoCode && deco.start <= to && from <= deco.end
	})
	if code {
		return nil
	}
	start, end := snap.LineRange(line)
	position := from
	var edits []textEdit
	if from == to {
		onEdge := false
		doc.visit(max(start, from-1), min(end, to+1), func(span *liveSpan) {
			onEdge = onEdge || span.kind == liveMark && (back && !span.opening && span.start == from || !back && span.opening && span.end == to)
		})
		for {
			probe := position
			if back {
				probe--
			}
			if probe < start || probe >= end {
				break
			}
			next := position
			doc.visit(probe, probe, func(span *liveSpan) {
				if (span.kind == liveMark || span.kind == liveObject && span.obj == objExternal) && span.start <= probe && probe < span.end {
					if back {
						next = min(next, span.start)
					} else {
						next = max(next, span.end)
					}
				}
			})
			if next == position {
				break
			}
			position = next
		}
		if position == from && !onEdge {
			return nil
		}
		if back && position == start {
			prefix := snap.Slice(start, from)
			if len(strings.TrimSpace(prefix)) > 0 && strings.Trim(strings.TrimSpace(prefix), "#") == "" {
				hashes := strings.IndexByte(prefix, '#')
				level := len(prefix[hashes:]) - len(strings.TrimLeft(prefix[hashes:], "#"))
				if level <= 6 {
					block := mdHeadingBlock{first: line, marker: start + hashes, level: level}
					edits = block.format(snap, 0, nil)
				}
			}
		}
		if len(edits) == 0 {
			from, to = liveDeleteCluster(snap, start, end, position, back)
			if position == sel[0] {
				edits = liveDeleteSelection(doc, from, to)
				if len(edits) == 0 || len(edits) == 1 && edits[0].start == from && edits[0].end == to {
					return nil
				}
			}
		}
	}
	if len(edits) == 0 {
		edits = liveDeleteSelection(doc, from, to)
	}
	if len(edits) != 0 {
		sortEdits(edits)
		slices.Reverse(edits)
		merged := edits[:1]
		for _, edit := range edits[1:] {
			last := &merged[len(merged)-1]
			if last.end >= edit.start {
				last.end = max(last.end, edit.end)
			} else {
				merged = append(merged, edit)
			}
		}
		edits = merged
	}
	if sel[0] != sel[1] && len(edits) == 1 && edits[0].start == from && edits[0].end == to {
		return nil
	}
	return func(target *editor) {
		if target.buf.Snapshot() != snap || target.sel != sel || len(edits) == 0 {
			return
		}
		caret := edits[0].start
		target.edit([2]int{caret, caret}, edits...)
	}
}

func liveDeleteCluster(snap ui.TextSnapshot, lineStart, lineEnd, caret int, back bool) (int, int) {
	if back && caret == lineStart || !back && caret == lineEnd {
		return caret, caret
	}
	var breaks segmenter.Segmenter
	breaks.Init([]rune(snap.Slice(lineStart, lineEnd)))
	clusters := breaks.GraphemeIterator()
	for clusters.Next() {
		cluster := clusters.Grapheme()
		start := lineStart + cluster.Offset
		end := start + len(cluster.Text)
		if back && start < caret && caret <= end || !back && start <= caret && caret < end {
			return start, end
		}
	}
	return caret, caret
}

func liveDeleteSelection(doc *liveDoc, from, to int) []textEdit {
	type markup struct{ start, end, textStart int }
	var edits []textEdit
	var enclosing []markup
	body := from
	doc.visit(from, to, func(span *liveSpan) {
		if span.kind != liveMark && !(span.kind == liveObject && span.obj == objExternal) {
			return
		}
		if from > span.from || to < span.to {
			if span.end > body && span.start < to {
				if body < span.start {
					edits = append(edits, textEdit{body, span.start, ""})
				}
				body = min(to, span.end)
			}
		}
	})
	if body < to {
		edits = append(edits, textEdit{body, to, ""})
	}
	if len(edits) == 0 || from == to {
		return edits
	}
	lineStart, _ := doc.snap.LineRange(doc.snap.LineAt(from))
	_, lineEnd := doc.snap.LineRange(doc.snap.LineAt(to))
	doc.visit(lineStart, lineEnd, func(span *liveSpan) {
		if span.kind == liveMark && span.opening && span.end < to && span.to > from && span.from >= lineStart && span.to <= lineEnd {
			enclosing = append(enclosing, markup{span.from, span.to, span.end})
		}
	})
	slices.SortFunc(enclosing, func(left, right markup) int { return cmp.Compare(left.end-left.start, right.end-right.start) })
	for _, wrap := range enclosing {
		contentEnd := wrap.end
		doc.visit(wrap.start, wrap.end, func(span *liveSpan) {
			if span.from == wrap.start && span.to == wrap.end && !span.opening && (span.kind == liveMark || span.kind == liveObject && span.obj == objExternal) {
				contentEnd = min(contentEnd, span.start)
			}
		})
		if contentEnd < wrap.end {
			for index := range edits {
				if edits[index].start <= wrap.textStart && edits[index].end >= contentEnd {
					edits[index].start = min(edits[index].start, wrap.start)
					edits[index].end = max(edits[index].end, wrap.end)
					break
				}
			}
		}
	}
	return edits
}

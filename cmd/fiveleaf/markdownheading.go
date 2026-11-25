package main

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark/ast"
)

type mdHeadingBlock struct {
	first, last, marker, level int
	content                    []int
}

func toggleMdHeading(d *editor, level int) {
	snapshot := d.buf.Snapshot()
	first, last := lineSpan(snapshot, d.sel)
	selectionStart, _ := snapshot.LineRange(first)
	_, selectionEnd := snapshot.LineRange(last)
	from := len(snapshot.Slice(0, selectionStart))
	to := from + len(snapshot.Slice(selectionStart, selectionEnd))
	var blocks []mdHeadingBlock
	mappedByte, mappedRune := from, selectionStart
	_, _, _, ok := mdFormattingRange(d, func(heading *ast.Heading, source []byte) {
		pos := heading.Pos()
		if pos < 0 || pos >= len(source) {
			return
		}
		segments := heading.Lines()
		setext := segments.Len() != 0 && segments.At(0).Start == pos
		end := mdFormatLineEnd(source, pos)
		if setext {
			segment := segments.At(segments.Len() - 1)
			end = mdFormatLineEnd(source, min(len(source), mdFormatLineEnd(source, segment.Start)+1))
		}
		if end < from || pos > to {
			return
		}
		if pos >= mappedByte {
			mappedRune += utf8.RuneCount(source[mappedByte:pos])
		} else {
			mappedRune -= utf8.RuneCount(source[pos:mappedByte])
		}
		mappedByte = pos
		marker := mappedRune
		block := mdHeadingBlock{first: snapshot.LineAt(marker), marker: marker, level: heading.Level}
		block.last = block.first
		if setext {
			lastByte, content := pos, marker
			for index := range segments.Len() {
				start := segments.At(index).Start
				content += utf8.RuneCount(source[lastByte:start])
				block.content = append(block.content, content)
				lastByte = start
			}
			block.last = snapshot.LineAt(block.content[len(block.content)-1]) + 1
		}
		blocks = append(blocks, block)
	})
	if !ok || mdLiteralLine(snapshot, first) {
		return
	}
	var edits []textEdit
	for line := first; line <= last; line++ {
		if len(blocks) > 0 && blocks[0].first <= line {
			block := blocks[0]
			blocks = blocks[1:]
			edits = block.format(snapshot, level, edits)
			line = block.last
			continue
		}
		if level == 0 {
			continue
		}
		start, end := snapshot.LineRange(line)
		raw := snapshot.Slice(start, end)
		if first != last && strings.TrimSpace(raw) == "" {
			continue
		}
		parts := parseMdLine(raw)
		at := start + len(parts.quote) + len(parts.indent)
		edits = append(edits, textEdit{at, start + parts.n, strings.Repeat("#", level) + " "})
	}
	if len(edits) == 0 {
		return
	}
	sortEdits(edits)
	selection := d.sel
	for _, edit := range edits {
		length := utf8.RuneCountInString(edit.text)
		for index, pos := range selection {
			if pos >= edit.start && pos <= edit.end {
				if edit.start == edit.end && d.sel[0] != d.sel[1] && pos == min(selection[0], selection[1]) {
					continue
				}
				selection[index] = edit.start + length
			} else if pos > edit.end {
				selection[index] += length - (edit.end - edit.start)
			}
		}
	}
	d.edit(selection, edits...)
}

func (block mdHeadingBlock) format(snap ui.TextSnapshot, level int, edits []textEdit) []textEdit {
	if level == block.level {
		level = 0
	}
	if len(block.content) != 0 {
		barStart, barEnd := snap.LineRange(block.last)
		if level == 1 || level == 2 {
			bar := snap.Slice(barStart, barEnd)
			mark := strings.IndexAny(bar, "=-")
			if mark >= 0 {
				run := len(bar[mark:]) - len(strings.TrimLeft(bar[mark:], bar[mark:mark+1]))
				replacement := "="
				if level == 2 {
					replacement = "-"
				}
				edits = append(edits, textEdit{barStart + mark, barStart + mark + run, strings.Repeat(replacement, run)})
			}
			return edits
		}
		edits = append(edits, textEdit{barStart - 1, barEnd, ""})
		if level > 0 {
			edits = append(edits, textEdit{block.marker, block.marker, strings.Repeat("#", level) + " "})
			for index := 1; index < len(block.content); index++ {
				_, end := snap.LineRange(block.first + index - 1)
				edits = append(edits, textEdit{end, block.content[index], " "})
			}
		}
		return edits
	}
	if level > 0 {
		return append(edits, textEdit{block.marker, block.marker + block.level, strings.Repeat("#", level)})
	}
	_, end := snap.LineRange(block.first)
	start := block.marker + block.level
	rest := snap.Slice(start, end)
	openingEnd := start + len(rest) - len(strings.TrimLeft(rest, " \t"))
	edits = append(edits, textEdit{block.marker, openingEnd, ""})
	trimmed := strings.TrimRight(rest, " \t")
	closing := strings.TrimRight(trimmed, "#")
	if len(closing) < len(trimmed) && len(closing) > 0 && strings.ContainsAny(closing[len(closing)-1:], " \t") {
		textEnd := start + utf8.RuneCountInString(strings.TrimRight(closing, " \t"))
		edits = append(edits, textEdit{max(openingEnd, textEnd), start + utf8.RuneCountInString(trimmed), ""})
	}
	return edits
}

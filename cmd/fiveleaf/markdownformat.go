package main

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func toggleMdBold(d *editor)   { toggleMdDelimiter(d, "**") }
func toggleMdItalic(d *editor) { toggleMdDelimiter(d, "*") }
func toggleMdStrike(d *editor) { toggleMdDelimiter(d, "~~") }

// Code ranges come from the parser so fences nested in quotes and lists are
// protected in the same way as top-level code.
func mdFormatRange(d *editor) (snap ui.TextSnapshot, start, end int, ok bool) {
	return mdFormattingRange(d, nil)
}

func mdFormattingRange(d *editor, heading func(*ast.Heading, []byte)) (snap ui.TextSnapshot, start, end int, ok bool) {
	if !isMarkdown(d.document) || d.locked() {
		return
	}
	snap = d.buf.Snapshot()
	start = min(max(min(d.sel[0], d.sel[1]), 0), snap.Len())
	end = min(max(max(d.sel[0], d.sel[1]), 0), snap.Len())
	source := []byte(snap.String())
	tree := markdownParser(false).Parse(text.NewReader(source))
	from, to := len(snap.Slice(0, start)), len(snap.Slice(0, end))
	ok = true
	_ = ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var first, last int
		switch x := n.(type) {
		case *ast.Heading:
			if heading != nil {
				heading(x, source)
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			first = x.Pos()
			if first < 0 {
				return ast.WalkSkipChildren, nil
			}
			last = mdFormatLineEnd(source, first)
			if lines := x.Lines(); lines.Len() > 0 {
				last = mdFormatLineEnd(source, lines.At(lines.Len()-1).Start)
			}
			if last < len(source) {
				next := mdFormatLineEnd(source, last+1)
				closing := strings.TrimSpace(parseMdLine(string(source[last+1 : next])).body)
				mark := source[first : first+1]
				opening := source[first:mdFormatLineEnd(source, first)]
				size := len(opening) - len(bytes.TrimLeft(opening, string(mark)))
				if len(closing) >= size && strings.Trim(closing, string(mark)) == "" {
					last = next
				}
			}
		case *ast.CodeBlock:
			lines := x.Lines()
			if lines.Len() == 0 {
				return ast.WalkSkipChildren, nil
			}
			first, last = lines.At(0).Start, mdFormatLineEnd(source, lines.At(lines.Len()-1).Start)
		default:
			return ast.WalkContinue, nil
		}
		first = bytes.LastIndexByte(source[:first], '\n') + 1
		if from == to && first <= from && from <= last || from < last && to > first {
			ok = false
			return ast.WalkStop, nil
		}
		return ast.WalkSkipChildren, nil
	})
	return
}

func mdFormatLineEnd(source []byte, from int) int {
	if i := bytes.IndexByte(source[from:], '\n'); i >= 0 {
		return from + i
	}
	return len(source)
}

// A single parsed node must own both delimiters before either is removed.
// Looking only at matching characters would pair two independent spans.
func mdSingleFormat(source, marker string) bool {
	tree := markdownParser(false).Parse(text.NewReader([]byte(source)))
	paragraph, ok := tree.FirstChild().(*ast.Paragraph)
	if !ok || paragraph.NextSibling() != nil {
		return false
	}
	for node := paragraph.FirstChild(); node != nil && node.NextSibling() == nil; node = node.FirstChild() {
		switch x := node.(type) {
		case *ast.Emphasis:
			if marker == "*" && x.Level == 1 || marker == "**" && x.Level == 2 {
				return true
			}
		case *east.Strikethrough:
			return marker == "~~"
		case *ast.CodeSpan:
			return marker == "`"
		default:
			return false
		}
	}
	return false
}

func mdFormatSelection(d *editor, start, end int) [2]int {
	if d.sel[0] > d.sel[1] {
		return [2]int{end, start}
	}
	return [2]int{start, end}
}

func mdRunBefore(snap ui.TextSnapshot, at int, mark string) int {
	start, _ := snap.LineRange(snap.LineAt(at))
	text := snap.Slice(start, at)
	return len(text) - len(strings.TrimRight(text, mark))
}

func mdRunAfter(snap ui.TextSnapshot, at int, mark string) int {
	_, end := snap.LineRange(snap.LineAt(at))
	text := snap.Slice(at, end)
	return len(text) - len(strings.TrimLeft(text, mark))
}

func mdUnescaped(snap ui.TextSnapshot, at int) bool {
	return mdRunBefore(snap, at, `\`)%2 == 0
}

func mdDelimiterMatches(marker string, run int) bool {
	switch marker {
	case "*":
		return run == 1 || run == 3
	case "**":
		return run == 2 || run == 3
	default:
		return run == len(marker)
	}
}

func toggleMdDelimiter(d *editor, marker string) {
	snap, start, end, ok := mdFormatRange(d)
	if !ok {
		return
	}
	text := snap.Slice(start, end)
	if text != "" {
		// CommonMark emphasis cannot open or close against whitespace.
		trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
		start += utf8.RuneCountInString(text) - utf8.RuneCountInString(trimmed)
		text = strings.TrimRightFunc(trimmed, unicode.IsSpace)
		end = start + utf8.RuneCountInString(text)
		if text == "" {
			return
		}
	}
	if strings.ContainsAny(text, "\r\n") {
		return
	}
	n, mark := len(marker), marker[:1]
	left := len(text) - len(strings.TrimLeft(text, mark))
	right := len(text) - len(strings.TrimRight(text, mark))
	if end-start >= 2*n && mdDelimiterMatches(marker, left) && mdDelimiterMatches(marker, right) &&
		mdUnescaped(snap, start) && mdUnescaped(snap, end-n) && mdSingleFormat(text, marker) {
		d.edit(mdFormatSelection(d, start, end-2*n), textEdit{start, start + n, ""}, textEdit{end - n, end, ""})
		return
	}
	left, right = mdRunBefore(snap, start, mark), mdRunAfter(snap, end, mark)
	if mdDelimiterMatches(marker, left) && mdDelimiterMatches(marker, right) &&
		mdUnescaped(snap, start-left) && mdUnescaped(snap, end) &&
		(start == end || mdSingleFormat(snap.Slice(start-n, end+n), marker)) {
		d.edit(mdFormatSelection(d, start-n, end-n), textEdit{start - n, start, ""}, textEdit{end, end + n, ""})
		return
	}
	if start == end {
		d.edit([2]int{start + n, start + n}, textEdit{start, end, marker + marker})
		return
	}
	if !mdSingleFormat(marker+text+marker, marker) {
		return
	}
	d.edit(mdFormatSelection(d, start+n, end+n), textEdit{start, start, marker}, textEdit{end, end, marker})
}

func mdBacktickRun(text string) int {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

func toggleMdCode(d *editor) {
	snap, start, end, ok := mdFormatRange(d)
	if !ok {
		return
	}
	text := snap.Slice(start, end)
	if strings.ContainsAny(text, "\r\n") {
		return
	}
	left := len(text) - len(strings.TrimLeft(text, "`"))
	right := len(text) - len(strings.TrimRight(text, "`"))
	if left > 0 && left == right && end-start >= 2*left && mdUnescaped(snap, start) && mdSingleFormat(text, "`") {
		padding := 0
		inside := strings.Trim(text, "`")
		if strings.HasPrefix(inside, " ") && strings.HasSuffix(inside, " ") && strings.Trim(inside, " ") != "" {
			padding = 1
		}
		n := left + padding
		d.edit(mdFormatSelection(d, start, end-2*n), textEdit{start, start + n, ""}, textEdit{end - n, end, ""})
		return
	}
	for padding := 0; padding <= 1; padding++ {
		if padding == 1 && (start == 0 || end == snap.Len() || snap.Slice(start-1, start) != " " || snap.Slice(end, end+1) != " ") {
			continue
		}
		left, right = mdRunBefore(snap, start-padding, "`"), mdRunAfter(snap, end+padding, "`")
		if left > 0 && left == right && mdUnescaped(snap, start-padding-left) &&
			(start == end || mdSingleFormat(snap.Slice(start-padding-left, end+padding+right), "`")) {
			n := left + padding
			d.edit(mdFormatSelection(d, start-n, end-n), textEdit{start - n, start, ""}, textEdit{end, end + n, ""})
			return
		}
	}
	// Unmatched outside ticks would join the new delimiter, possibly turning
	// the whole line into a fenced block instead of one inline code span.
	if mdRunBefore(snap, start, "`") > 0 || mdRunAfter(snap, end, "`") > 0 {
		return
	}
	marker := strings.Repeat("`", mdBacktickRun(text)+1)
	padding := ""
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") ||
		(strings.HasPrefix(text, " ") && strings.HasSuffix(text, " ") && strings.Trim(text, " ") != "") {
		padding = " "
	}
	opening, closing := marker+padding, padding+marker
	if start == end {
		at := start + len(opening)
		d.edit([2]int{at, at}, textEdit{start, end, opening + closing})
		return
	}
	n := len(opening)
	d.edit(mdFormatSelection(d, start+n, end+n), textEdit{start, start, opening}, textEdit{end, end, closing})
}

func insertMdLink(d *editor)  { insertMdLinkKind(d, false) }
func insertMdImage(d *editor) { insertMdLinkKind(d, true) }

func insertMdLinkKind(d *editor, image bool) {
	snap, start, end, ok := mdFormatRange(d)
	if !ok {
		return
	}
	label := snap.Slice(start, end)
	if strings.ContainsAny(label, "\r\n") {
		return
	}
	prefix := "["
	if image {
		prefix = "!["
	}
	if label == "" {
		label = locales.MarkdownLinkText()
		if image {
			label = locales.MarkdownImageAlt()
		}
	}
	label = strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(label)
	opening := prefix + label + "]("
	url := "https://"
	from := start + utf8.RuneCountInString(opening)
	d.edit([2]int{from, from + len(url)}, textEdit{start, end, opening + url + ")"})
}

// mdBlockEdges separates an inserted block from the paragraphs on either side.
func mdBlockEdges(snap ui.TextSnapshot, start, end int) (before, after string) {
	if start > 0 {
		text := snap.Slice(max(0, start-2), start)
		before = strings.Repeat("\n", 2-(len(text)-len(strings.TrimRight(text, "\n"))))
	}
	if end < snap.Len() {
		text := snap.Slice(end, min(end+2, snap.Len()))
		after = strings.Repeat("\n", 2-(len(text)-len(strings.TrimLeft(text, "\n"))))
	}
	return
}

func insertMdCodeBlock(d *editor) {
	snap, start, end, ok := mdFormatRange(d)
	if !ok {
		return
	}
	text := snap.Slice(start, end)
	fence := strings.Repeat("`", max(3, mdBacktickRun(text)+1))
	before, after := mdBlockEdges(snap, start, end)
	opening := before + fence + "\n"
	closing := "\n" + fence + after
	from := start + len(opening)
	d.edit(mdFormatSelection(d, from, from+utf8.RuneCountInString(text)), textEdit{start, end, opening + text + closing})
}

func insertMdTable(d *editor, rows, columns int) {
	if rows < 1 || rows > maxMdTableDimension || columns < 1 || columns > maxMdTableDimension {
		return
	}
	snap, start, end, ok := mdFormatRange(d)
	if !ok {
		return
	}
	if start == end && d.live {
		line := snap.LineAt(start)
		lineStart, lineEnd := snap.LineRange(line)
		parts := parseMdLine(snap.Line(line))
		if parts.heading > 0 || parts.list() || parts.quote != "" {
			switch {
			case start <= lineStart+parts.n:
				start, end = lineStart, lineStart
			case parts.heading > 0:
				start, end = lineEnd, lineEnd
			}
		}
	}
	first := snap.Slice(start, end)
	if first == "" {
		first = locales.MarkdownTableColumn("1")
	}
	first = strings.NewReplacer(`\`, `\\`, "|", `\|`, "\r\n", "<br>", "\n", "<br>").Replace(first)
	headings := make([]string, columns)
	headings[0] = first
	for column := 1; column < columns; column++ {
		headings[column] = locales.MarkdownTableColumn(strconv.Itoa(column + 1))
	}
	before, after := mdBlockEdges(snap, start, end)
	header := "| " + strings.Join(headings, " | ") + " |"
	separator := "\n|" + strings.Repeat(" --- |", columns)
	row := "\n|" + strings.Repeat("  |", columns)
	text := before + header + separator + strings.Repeat(row, rows-1) + after
	from := start + len(before) + 2
	d.edit([2]int{from, from + utf8.RuneCountInString(first)}, textEdit{start, end, text})
	if d.live && d.preview == nil {
		d.tableFocusAt = from + 1
	}
}

// markdownContextMenu adds the Markdown actions of the editor's menu:
// the inline marks, the blocks that go around a line, and the inserts.
// The actions keep the selection the menu opened on: the menu is built
// again once one is chosen, after the editor rebuilt its area.
func (a *app) markdownContextMenu(m *ui.Menu, d *editor) {
	run := func(edit func(*editor)) {
		sel := d.sel
		a.post(func() {
			if !d.locked() {
				a.queueEdit(d, func(d *editor) {
					d.sel = sel
					edit(d)
				})
			}
		})
	}
	if m.Item(locales.CmdEditUndo()).Disabled(d.locked() || len(d.hist.undo) == 0).Chosen() {
		run(func(d *editor) { d.undo() })
	}
	if m.Item(locales.CmdEditRedo()).Disabled(d.locked() || len(d.hist.redo) == 0).Chosen() {
		run(func(d *editor) { d.redo() })
	}
	m.Separator()
	m.EditItems()
	m.Separator()
	if mdMenuChoice(m.Item(locales.CmdMarkdownBold()).Disabled(d.locked()), a, "markdown.bold").Chosen() {
		run(toggleMdBold)
	}
	if mdMenuChoice(m.Item(locales.CmdMarkdownItalic()).Disabled(d.locked()), a, "markdown.italic").Chosen() {
		run(toggleMdItalic)
	}
	if mdMenuChoice(m.Item(locales.CmdMarkdownStrikethrough()).Disabled(d.locked()), a, "markdown.strikethrough").Chosen() {
		run(toggleMdStrike)
	}
	if mdMenuChoice(m.Item(locales.CmdMarkdownInlineCode()).Disabled(d.locked()), a, "markdown.inlineCode").Chosen() {
		run(toggleMdCode)
	}
	m.Separator()
	m.Submenu(locales.MarkdownParagraph(), func(m *ui.Menu) {
		if mdMenuChoice(m.Item(locales.MarkdownParagraph()).Disabled(d.locked()), a, "markdown.paragraph").Chosen() {
			run(mdHeadingAction(0))
		}
		if m.Item(locales.CmdMarkdownClearFormatting()).Disabled(d.locked()).Chosen() {
			run(clearMdFormatting)
		}
		for level := 1; level <= 6; level++ {
			id := "markdown.heading" + strconv.Itoa(level)
			if mdMenuChoice(m.Item(locales.CmdMarkdownHeading(strconv.Itoa(level))).Disabled(d.locked()), a, id).Chosen() {
				run(mdHeadingAction(level))
			}
		}
	})
	if m.Item(locales.CmdMarkdownToggleList()).Disabled(d.locked()).Chosen() {
		run(toggleMdList)
	}
	if m.Item(locales.CmdMarkdownToggleTask()).Disabled(d.locked()).Chosen() {
		run(toggleMdTask)
	}
	if m.Item(locales.CmdMarkdownToggleQuote()).Disabled(d.locked()).Chosen() {
		run(toggleMdQuote)
	}
	m.Separator()
	if mdMenuChoice(m.Item(locales.CmdMarkdownInsertLink()).Disabled(d.locked()), a, "markdown.insertLink").Chosen() {
		run(insertMdLink)
	}
	if mdMenuChoice(m.Item(locales.CmdMarkdownInsertImage()).Disabled(d.locked()), a, "markdown.insertImage").Chosen() {
		run(insertMdImage)
	}
	if mdMenuChoice(m.Item(locales.CmdMarkdownInsertCodeBlock()).Disabled(d.locked()), a, "markdown.insertCodeBlock").Chosen() {
		run(insertMdCodeBlock)
	}
	if mdMenuChoice(m.Item(locales.CmdMarkdownInsertTable()).Disabled(d.locked()), a, "markdown.insertTable").Chosen() {
		run(a.chooseMdTableSize)
	}
}

// mdMenuChoice shows the shortcut of command id by the chosen item, when
// the person has not bound the command elsewhere.
func mdMenuChoice(it *ui.MenuItem, a *app, id string) *ui.MenuItem {
	for _, cmd := range a.commands() {
		if cmd.id != id {
			continue
		}
		if keys := a.shortcutKeys(cmd); keys != nil {
			if len(keys) == 1 {
				it.Shortcut(keys[0].Mods, keys[0].Key)
			}
			break
		}
		if a.keys != nil && a.keys.Removes(commandNames(id)...) || a.presetSuppresses(cmd) {
			break
		}
		if st, ok := parseAccel(defaultAccel(cmd)); ok && !a.keys.Binds(st) {
			it.Shortcut(st.Mods, st.Key)
		}
		break
	}
	return it
}

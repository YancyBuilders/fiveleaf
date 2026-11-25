package main

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// mdLine is a line of Markdown taken apart: its quotes' >, the indent
// after them, and a heading's # or a list's marker and a task's box,
// before its text.
type mdLine struct {
	quote, indent string
	heading       int
	marker, box   string
	n             int // runes of what comes before body
	body          string
}

// mdLineHead is what starts a line of Markdown, in mdLine's parts.
var mdLineHead = regexp.MustCompile(`^((?:[ \t]*>[ \t]?)*)([ \t]*)(?:(#{1,6})(?:[ \t]+|$)|([-*+]|\d{1,9}[.)])(?:[ \t]+|$)(?:(\[[ xX]\])(?:[ \t]+|$))?)?`)

func parseMdLine(text string) mdLine {
	m := mdLineHead.FindStringSubmatchIndex(text)
	group := func(g int) string {
		if m[2*g] < 0 {
			return ""
		}
		return text[m[2*g]:m[2*g+1]]
	}
	l := mdLine{quote: group(1), indent: group(2), marker: group(4), box: group(5)}
	l.heading = len(group(3))
	l.n = utf8.RuneCountInString(text[:m[1]])
	l.body = text[m[1]:]
	return l
}

// prefix is the text of a line before its body, one space after each
// mark.
func (l mdLine) prefix() string {
	p := l.quote + l.indent
	if l.heading > 0 {
		p += strings.Repeat("#", l.heading) + " "
	}
	if l.marker != "" {
		p += l.marker + " "
	}
	if l.box != "" {
		p += l.box + " "
	}
	return p
}

func (l mdLine) list() bool { return l.marker != "" }

func (l mdLine) ordered() bool { return l.marker != "" && !strings.ContainsAny(l.marker, "-*+") }

// nextMarker is a list's marker for the item step after one with marker
// m: the same bullet, or the number step on, 1 at least.
func nextMarker(m string, step int) string {
	if !strings.ContainsAny(m, ".)") {
		return m
	}
	digits := m[:len(m)-1]
	n, _ := strconv.Atoi(digits)
	if n == 0 && step < 0 {
		return m
	}
	next := strconv.Itoa(max(1, n+step))
	if len(next) > 9 {
		return m
	}
	if digits[0] == '0' && len(next) < len(digits) {
		next = strings.Repeat("0", len(digits)-len(next)) + next
	}
	return next + m[len(m)-1:]
}

var mdEditBlocks = parser.NewParser(parser.WithBlockParsers(parser.DefaultBlockParsers()...))

var mdFrontMatterEnd = regexp.MustCompile(`(?m)^(?:---|\.\.\.)$`)

func mdBlockAt(snap ui.TextSnapshot, at, end int) ([]byte, int, ast.Node) {
	line := snap.LineAt(at)
	current := parseMdLine(snap.Line(line))
	first := line
	for first > 0 {
		first--
		raw := snap.Line(first)
		previous := parseMdLine(raw)
		if strings.TrimSpace(raw) == "" || len(previous.indent) > min(3, len(current.indent)) || strings.ContainsRune(previous.indent, '\t') || len(leading(raw)) >= 4 {
			continue
		}
		if !previous.list() && fenceOpens(previous.body) && fenceCloses(previous.body, previous.body[0], 3) {
			first++
			break
		}
		if previous.list() || previous.heading != 0 || previous.quote != current.quote || first == 0 || strings.TrimSpace(snap.Line(first-1)) == "" {
			break
		}
	}
	start, _ := snap.LineRange(first)
	raw := snap.Slice(start, end)
	source := []byte(raw)
	return source, runeByte(raw, at-start), mdEditBlocks.Parse(text.NewReader(source))
}

func mdListItemAt(snap ui.TextSnapshot, marker, end int) ([]byte, *ast.ListItem) {
	source, markerByte, tree := mdBlockAt(snap, marker, end)
	var item *ast.ListItem
	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Pos() == markerByte {
			if found, ok := node.(*ast.ListItem); ok {
				item = found
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return source, item
}

func mdQuoteInCode(snap ui.TextSnapshot, start, end int, quote string) bool {
	if !strings.Contains(quote, "    ") && !strings.ContainsRune(quote, '\t') {
		return false
	}
	_, marker, tree := mdBlockAt(snap, start+strings.LastIndexByte(quote, '>'), end)
	inside := false
	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Kind() == ast.KindCodeBlock {
			lines := node.Lines()
			if lines.Len() > 0 && lines.At(0).Start <= marker && marker <= lines.At(lines.Len()-1).Stop {
				inside = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return inside
}

func mdShiftNumbers(snap ui.TextSnapshot, source []byte, item *ast.ListItem, markerAt int, marker string, step int) []textEdit {
	previous, _ := strconv.Atoi(marker[:len(marker)-1])
	lastByte := item.Pos()
	var edits []textEdit
	for next := item.NextSibling(); next != nil; next = next.NextSibling() {
		offset := next.Pos()
		if offset <= lastByte || offset >= len(source) {
			break
		}
		digits := 0
		for offset+digits < len(source) && source[offset+digits] >= '0' && source[offset+digits] <= '9' {
			digits++
		}
		number, err := strconv.Atoi(string(source[offset : offset+digits]))
		if err != nil || number != previous+1 || offset+digits >= len(source) || source[offset+digits] != marker[len(marker)-1] {
			break
		}
		old := string(source[offset : offset+digits+1])
		replacement := nextMarker(old, step)
		if replacement == old {
			break
		}
		markerAt += utf8.RuneCount(source[lastByte:offset])
		edits = append(edits, textEdit{markerAt, markerAt + digits + 1, replacement})
		if delta := len(replacement) - len(old); delta != 0 {
			edits = append(edits, mdNumberIndent(snap, markerAt, old, delta)...)
		}
		previous, lastByte = number, offset
	}
	return edits
}

func mdNumberIndent(snap ui.TextSnapshot, markerAt int, marker string, delta int) []textEdit {
	line := snap.LineAt(markerAt)
	raw := snap.Line(line)
	parts := parseMdLine(raw)
	markerEnd := len(parts.quote) + len(parts.indent) + len(marker)
	padding, _ := util.IndentWidth([]byte(raw[markerEnd:]), markerEnd)
	if padding == 0 || padding > 4 {
		padding = 1
	}
	minWidth, _ := util.IndentWidth([]byte(parts.indent), len(parts.quote))
	minWidth += len(marker) + padding
	var edits []textEdit
	for line++; line < snap.LineCount(); line++ {
		raw = snap.Line(line)
		body, ok := strings.CutPrefix(raw, parts.quote)
		if !ok {
			break
		}
		if strings.TrimSpace(body) == "" {
			continue
		}
		width, size := util.IndentWidth([]byte(body), len(parts.quote))
		if width < minWidth {
			break
		}
		start, _ := snap.LineRange(line)
		end := start + len(parts.quote) + size
		if delta > 0 {
			edits = append(edits, textEdit{end, end, strings.Repeat(" ", delta)})
		} else {
			prior, _ := util.IndentWidth([]byte(body[:size-1]), len(parts.quote))
			edits = append(edits, textEdit{end - 1, end, strings.Repeat(" ", width-prior+delta)})
		}
	}
	return edits
}

// mdLiteralLine reports whether the line belongs to front matter or a code fence.
func mdLiteralLine(snap ui.TextSnapshot, line int) bool {
	start, _ := snap.LineRange(line)
	before := snap.Slice(0, start)
	if strings.HasPrefix(before, "---\n") && !mdFrontMatterEnd.MatchString(before[4:]) {
		tail := snap.Slice(start, snap.Len())
		if closing := mdFrontMatterEnd.FindStringIndex(tail); closing != nil {
			if _, skipped := markdownSkipFrontMatter([]byte(before + tail[:closing[1]])); skipped != 0 {
				return true
			}
		}
	}
	if !strings.Contains(before, "```") && !strings.Contains(before, "~~~") {
		return false
	}
	var mark byte
	var length, rowIndex int
	var containers []int
	for row := range strings.SplitSeq(strings.TrimSuffix(before, "\n"), "\n") {
		current := rowIndex
		rowIndex++
		if mark != 0 {
			body, ok := mdFenceBody(row, containers)
			if ok {
				if fenceCloses(strings.TrimLeft(body, " "), mark, length) && len(leading(body)) <= 3 {
					mark = 0
				}
				continue
			}
			mark = 0
		}
		if fenceOpens(row) {
			mark, length, containers = row[0], fenceRun(row), nil
			continue
		}
		if !strings.Contains(row, "```") && !strings.Contains(row, "~~~") {
			continue
		}
		candidate := strings.TrimLeft(row, " \t>0123456789).-+*[]xX")
		if len(candidate) == len(row) || !fenceOpens(candidate) {
			continue
		}
		from, end := snap.LineRange(current)
		_, markerByte, tree := mdBlockAt(snap, from+len(row)-len(candidate), end)
		_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && node.Kind() == ast.KindFencedCodeBlock && node.Pos() == markerByte {
				mark, length, containers = candidate[0], fenceRun(candidate), nil
				for parent := node.Parent(); parent != nil; parent = parent.Parent() {
					switch block := parent.(type) {
					case *ast.ListItem:
						containers = append(containers, block.Offset)
					case *ast.Blockquote:
						containers = append(containers, 0)
					}
				}
				return ast.WalkStop, nil
			}
			return ast.WalkContinue, nil
		})
	}
	if mark == 0 {
		return false
	}
	_, inside := mdFenceBody(snap.Line(line), containers)
	return inside
}

func mdFenceBody(row string, containers []int) (string, bool) {
	column := 0
	for i := len(containers) - 1; i >= 0; i-- {
		if indent := containers[i]; indent > 0 {
			if strings.TrimSpace(row) == "" {
				return "", true
			}
			pos, padding := util.IndentPosition([]byte(row), column, indent)
			if pos < 0 {
				return "", false
			}
			row, column = strings.Repeat(" ", padding)+row[pos:], column+indent
			continue
		}
		width, pos := util.IndentWidth([]byte(row), column)
		if width > 3 || pos >= len(row) || row[pos] != '>' {
			return "", false
		}
		row, column = row[pos+1:], column+width+1
		if strings.HasPrefix(row, " ") {
			row, column = row[1:], column+1
		} else if strings.HasPrefix(row, "\t") {
			row = strings.Repeat(" ", util.TabWidth(column)-1) + row[1:]
			column++
		}
	}
	return row, true
}

// mdItemLine returns the line of the caret when it is an item of a list
// or a quote outside code, unless the caret is in its marks and after is
// set.
func mdItemLine(d *editor, after bool) (s, e int, l mdLine, ok bool) {
	if !isMarkdown(d.document) {
		return 0, 0, l, false
	}
	snap := d.buf.Snapshot()
	a, b := min(d.sel[0], d.sel[1], snap.Len()), min(max(d.sel[0], d.sel[1]), snap.Len())
	line := snap.LineAt(a)
	if snap.LineAt(b) != line {
		return 0, 0, l, false
	}
	s, e = snap.LineRange(line)
	text := snap.Slice(s, e)
	l = parseMdLine(text)
	if !l.list() && l.quote == "" || after && a-s < l.n || liveRule.MatchString(text) {
		return 0, 0, l, false
	}
	if mdLiteralLine(snap, line) || mdQuoteInCode(snap, s, e, l.quote) {
		return 0, 0, l, false
	}
	if l.list() && (strings.ContainsAny(l.indent, "\t") || len(l.indent) >= 4) {
		if _, item := mdListItemAt(snap, s+len(l.quote+l.indent), e); item == nil {
			return 0, 0, l, false
		}
	}
	return s, e, l, true
}

func mdListRenumber(d *editor, s, e int, l mdLine, step int, above bool) (string, []textEdit) {
	marker := nextMarker(l.marker, step)
	if above {
		marker = nextMarker(l.marker, -1)
	}
	if !l.ordered() || !above && e+1 >= d.buf.Len() {
		return marker, nil
	}
	snap := d.buf.Snapshot()
	end := e
	expected, _ := strconv.Atoi(l.marker[:len(l.marker)-1])
	blank := false
	for line := snap.LineAt(s) + 1; line < snap.LineCount(); line++ {
		raw := snap.Line(line)
		if raw != "" && !strings.HasPrefix(raw, l.quote) {
			break
		}
		rest := strings.TrimPrefix(raw, l.quote)
		trimmed := strings.TrimSpace(rest)
		if trimmed != "" && len(leading(rest)) <= len(l.indent) {
			sibling := parseMdLine(raw)
			if !sibling.list() && !blank && !strings.ContainsAny(trimmed[:1], "#>`~|<-*+_") {
				continue
			}
			if !sibling.ordered() || sibling.marker[len(sibling.marker)-1] != l.marker[len(l.marker)-1] {
				break
			}
			number, _ := strconv.Atoi(sibling.marker[:len(sibling.marker)-1])
			if number != expected+1 {
				break
			}
			expected = number
			_, end = snap.LineRange(line)
		}
		blank = trimmed == ""
	}
	if !above && end == e {
		return marker, nil
	}
	markerAt := s + len(l.quote+l.indent)
	source, item := mdListItemAt(snap, markerAt, end)
	if item == nil {
		return marker, nil
	}
	if above && item.PreviousSibling() == nil && marker != l.marker {
		return marker, nil
	}
	edits := mdShiftNumbers(snap, source, item, markerAt, l.marker, step)
	if above {
		marker = l.marker
		replacement := nextMarker(marker, 1)
		edits = append(edits, textEdit{markerAt, markerAt + len(marker), replacement})
		if delta := len(replacement) - len(marker); delta != 0 {
			edits = append(edits, mdNumberIndent(snap, markerAt, marker, delta)...)
		}
	}
	return marker, edits
}

// mdNewline breaks an item of a list or a quote at the caret, the new
// line starting a next item, or on an empty item ends the list, or the
// quote, a level at a time. It reports whether it did.
func mdNewline(d *editor) bool {
	s, e, l, ok := mdItemLine(d, true)
	if !ok {
		return false
	}
	if strings.TrimSpace(l.body) == "" {
		_, edits := mdListRenumber(d, s, e, l, -1, false)
		var p string
		switch {
		case l.list() && l.indent != "":
			l.indent = l.indent[outdentWidth(l.indent, d.indent):]
			p = l.prefix()
		case l.list():
			p = l.quote
		default:
			q := strings.TrimRight(l.quote, " \t")
			p = strings.TrimRight(q[:len(q)-1], " \t")
			if p != "" {
				p += " "
			}
		}
		at := s + utf8.RuneCountInString(p)
		d.edit([2]int{at, at}, append(edits, textEdit{s, e, p})...)
		return true
	}
	next := l
	next.heading = 0
	marker, edits := mdListRenumber(d, s, e, l, 1, false)
	next.marker = marker
	if l.box != "" {
		next.box = "[ ]"
	}
	text := "\n" + next.prefix()
	a, b := min(d.sel[0], d.sel[1]), max(d.sel[0], d.sel[1])
	caret := a + utf8.RuneCountInString(text)
	d.edit([2]int{caret, caret}, append(edits, textEdit{a, b, text})...)
	return true
}

// mdInsertLine opens a line below an item of a list or a quote, or
// above, as the next item, or the one before. It reports whether it did.
func mdInsertLine(d *editor, above bool) bool {
	s, e, l, ok := mdItemLine(d, false)
	if !ok {
		return false
	}
	next := l
	marker, edits := mdListRenumber(d, s, e, l, 1, above)
	next.marker = marker
	if l.box != "" {
		next.box = "[ ]"
	}
	p := next.prefix()
	n := utf8.RuneCountInString(p)
	if above {
		d.edit([2]int{s + n, s + n}, append(edits, textEdit{s, s, p + "\n"})...)
		return true
	}
	d.edit([2]int{e + 1 + n, e + 1 + n}, append(edits, textEdit{e, e, "\n" + p})...)
	return true
}

func mdBackspacePrefix(d *editor) func(*editor) {
	if !isMarkdown(d.document) || d.locked() || d.areaInput || len(d.cmds) != 0 || d.sel[0] != d.sel[1] || d.sel[1] == 0 {
		return nil
	}
	snap := d.buf.Snapshot()
	caret := d.sel[1]
	if snap.Version() != d.version || caret > snap.Len() {
		return nil
	}
	if !strings.ContainsAny(snap.Slice(caret-1, caret), " \t>.)-+*]") {
		return nil
	}
	start, end := snap.LineRange(snap.LineAt(caret))
	line := parseMdLine(snap.Slice(start, end))
	var from int
	switch {
	case line.list() && caret == start+line.n:
		from = start + len(line.quote) + len(line.indent)
		if _, item := mdListItemAt(snap, from, end); item == nil {
			return nil
		}
	case line.quote != "" && caret == start+len(line.quote):
		from = start + strings.LastIndexByte(line.quote, '>')
		if mdQuoteInCode(snap, start, end, line.quote) {
			return nil
		}
	default:
		return nil
	}
	if mdLiteralLine(snap, snap.LineAt(caret)) {
		return nil
	}
	return func(d *editor) {
		if d.buf.Snapshot() == snap && d.sel == [2]int{caret, caret} {
			d.edit([2]int{from, from}, textEdit{from, caret, ""})
		}
	}
}

// mdIndent indents the items of a list a selection covers a level, or
// outdents them, after their quotes' >, wherever the caret is in them.
// It reports whether it did: not unless every line is an item.
func mdIndent(d *editor, outdent bool) bool {
	if !isMarkdown(d.document) {
		return false
	}
	snap := d.buf.Snapshot()
	first, last := lineSpan(snap, d.sel)
	if mdLiteralLine(snap, first) {
		return false
	}
	type item struct {
		s int
		l mdLine
	}
	var items []item
	for i := first; i <= last; i++ {
		s, e := snap.LineRange(i)
		text := snap.Slice(s, e)
		if strings.TrimSpace(text) == "" {
			continue
		}
		l := parseMdLine(text)
		if !l.list() {
			return false
		}
		items = append(items, item{s, l})
	}
	if len(items) == 0 {
		return false
	}
	unit := d.indent.unit()
	sel := d.sel
	var edits []textEdit
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		at := it.s + utf8.RuneCountInString(it.l.quote)
		if outdent {
			if n := outdentWidth(it.l.indent, d.indent); n > 0 {
				edits = append(edits, textEdit{at, at + n, ""})
				sel = shiftSel(sel, at, -n)
			}
			continue
		}
		edits = append(edits, textEdit{at, at, unit})
		sel = shiftSel(sel, at, utf8.RuneCountInString(unit))
	}
	if len(edits) > 0 {
		d.edit(sel, edits...)
	}
	return true
}

// mdRow is a line of a selection being rewritten.
type mdRow struct {
	s     int
	blank bool
	l     mdLine
}

// rewriteMd gives the lines of a selection of Markdown new marks, those
// rewrite returns for them, as one step of undo, keeping the selection on
// their text. rows are the lines but blank ones, which rewrite only gets
// when the selection is of one line, or when blanks is set.
func rewriteMd(d *editor, blanks bool, rewrite func(rows []mdRow) []string) {
	if !isMarkdown(d.document) {
		return
	}
	snap := d.buf.Snapshot()
	first, last := lineSpan(snap, d.sel)
	var rows []mdRow
	for i := first; i <= last; i++ {
		s, e := snap.LineRange(i)
		text := snap.Slice(s, e)
		blank := strings.TrimSpace(text) == ""
		if blank && first != last && !blanks {
			continue
		}
		rows = append(rows, mdRow{s, blank, parseMdLine(text)})
	}
	if len(rows) == 0 {
		return
	}
	prefixes := rewrite(rows)
	sel := d.sel
	ranged := sel[0] != sel[1]
	var edits []textEdit
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		p := prefixes[i]
		old := snap.Slice(r.s, r.s+r.l.n)
		if p == old {
			continue
		}
		edits = append(edits, textEdit{r.s, r.s + r.l.n, p})
		n := utf8.RuneCountInString(p)
		for k, x := range sel {
			switch {
			case x < r.s:
			case ranged && x == r.s && x == min(sel[0], sel[1]):
				// A selection from the start of a line keeps its marks.
			case x >= r.s+r.l.n:
				sel[k] = x + n - r.l.n
			default:
				sel[k] = r.s + min(x-r.s, n)
			}
		}
	}
	if len(edits) > 0 {
		d.edit(sel, edits...)
	}
}

// toggleMdTask makes the lines of a selection tasks, checks them, then
// makes them items of a list again.
func toggleMdTask(d *editor) {
	ticked := -1
	rewriteMd(d, false, func(rows []mdRow) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			l := r.l
			switch l.box {
			case "":
				if !l.list() {
					l.marker = "-"
				}
				l.heading, l.box = 0, "[ ]"
			case "[ ]":
				l.box = "[x]"
			default:
				l.box = ""
			}
			if len(rows) == 1 && r.l.box != "" && l.box != "" {
				ticked = r.s + utf8.RuneCountInString(l.quote+l.indent+l.marker+" ")
			}
			out[i] = l.prefix()
		}
		return out
	})
	if ticked >= 0 {
		d.liveTicked, d.tickNow = ticked, true
	}
}

// mdTaskKey reports whether ⌘Enter toggles a task in an editor: in
// Markdown, its caret outside code.
func mdTaskKey(d *editor) bool {
	if !isMarkdown(d.document) {
		return false
	}
	snap := d.buf.Snapshot()
	return !mdLiteralLine(snap, snap.LineAt(min(d.sel[1], snap.Len())))
}

// toggleMdList makes the lines of a selection items of a bulleted list,
// those of one a numbered list, and those of a numbered list text, as
// its first line is.
func toggleMdList(d *editor) {
	rewriteMd(d, false, func(rows []mdRow) []string {
		first := rows[0].l
		out := make([]string, len(rows))
		for i, r := range rows {
			l := r.l
			l.heading = 0
			switch {
			case first.ordered():
				l.marker, l.box = "", ""
			case first.list():
				l.marker = strconv.Itoa(i+1) + "."
			default:
				l.marker = "-"
			}
			out[i] = l.prefix()
		}
		return out
	})
}

// toggleMdQuote quotes the lines of a selection, or takes a level of
// quote off them when all are quoted.
func toggleMdQuote(d *editor) {
	rewriteMd(d, true, func(rows []mdRow) []string {
		all := true
		for _, r := range rows {
			if !r.blank && r.l.quote == "" {
				all = false
			}
		}
		out := make([]string, len(rows))
		for i, r := range rows {
			l := r.l
			switch {
			case all && l.quote != "":
				l.quote = strings.TrimLeft(l.quote, " \t")[1:]
				l.quote = strings.TrimPrefix(strings.TrimPrefix(l.quote, " "), "\t")
			case all:
			case r.blank && len(rows) > 1:
				l.quote = ">" + l.quote
			default:
				l.quote = "> " + l.quote
			}
			out[i] = l.prefix()
		}
		return out
	})
}

func mdHeadingAction(level int) func(*editor) {
	return func(d *editor) {
		toggleMdHeading(d, level)
	}
}

// clearMdFormatting takes the quotes, heading, list marker and box off
// the lines of a selection.
func clearMdFormatting(d *editor) {
	rewriteMd(d, false, func(rows []mdRow) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			l := r.l
			if l.quote != "" || l.heading > 0 || l.list() {
				l = mdLine{}
			}
			out[i] = l.prefix()
		}
		return out
	})
}

// wikiOpen is the [[ that opens a wiki link, with the ! of an embed.
var wikiOpen = regexp.MustCompile(`!?\[\[`)

// toggleMdEmbed makes the wiki links of the lines of a selection embeds,
// or links again when one of a line's is an embed.
func toggleMdEmbed(d *editor) {
	if !isMarkdown(d.document) {
		return
	}
	snap := d.buf.Snapshot()
	first, last := lineSpan(snap, d.sel)
	sel := d.sel
	var edits []textEdit
	for i := last; i >= first; i-- {
		s, e := snap.LineRange(i)
		text := snap.Slice(s, e)
		ms := wikiOpen.FindAllStringIndex(text, -1)
		embed := strings.Contains(text, "![[")
		for k := len(ms) - 1; k >= 0; k-- {
			at := s + utf8.RuneCountInString(text[:ms[k][0]])
			bang := text[ms[k][0]] == '!'
			switch {
			case embed && bang:
				edits = append(edits, textEdit{at, at + 1, ""})
				sel = shiftSel(sel, at, -1)
			case !embed:
				edits = append(edits, textEdit{at, at, "!"})
				sel = shiftSel(sel, at, 1)
			}
		}
	}
	if len(edits) > 0 {
		d.edit(sel, edits...)
	}
}

// joinSelLines joins the lines of a selection into one, or the caret's
// line with the next, a space between their trimmed text, runs of spaces
// one.
func joinSelLines(d *editor) {
	snap := d.buf.Snapshot()
	first, last := lineSpan(snap, d.sel)
	if first == last {
		if last+1 >= snap.LineCount() {
			return
		}
		last++
	}
	bs, _ := snap.LineRange(first)
	_, be := snap.LineRange(last)
	lines := strings.Split(snap.Slice(bs, be), "\n")
	indent := leading(lines[0])
	var parts []string
	for _, l := range lines {
		if t := strings.Join(strings.Fields(l), " "); t != "" {
			parts = append(parts, t)
		}
	}
	text := indent + strings.Join(parts, " ")
	end := bs + utf8.RuneCountInString(text)
	if d.sel[0] != d.sel[1] {
		d.edit([2]int{bs, end}, textEdit{bs, be, text})
		return
	}
	// The caret goes where the first line meets the next.
	join := bs + utf8.RuneCountInString(indent)
	if len(parts) > 0 {
		join += utf8.RuneCountInString(parts[0])
	}
	d.edit([2]int{join, join}, textEdit{bs, be, text})
}

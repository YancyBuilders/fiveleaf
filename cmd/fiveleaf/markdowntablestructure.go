package main

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"fiveleaf/locales"
)

type tableAction uint8

const (
	tableRowAbove tableAction = iota
	tableRowBelow
	tableColumnLeft
	tableColumnRight
	tableDeleteRow
	tableDeleteColumn
	tableDeleteWhole
)

func (a *app) tableActionsButton(c *ui.Context, d *editor, cell *liveCellEditor) {
	label := locales.MarkdownTableActions()
	button := ui.ButtonBase(c.Key("table-actions:"+strconv.Itoa(d.id))).Height(crumbsH-4).Padding(0, 6).Gap(3).AlignItems(ui.Center).Radius(4).Shrink(0).
		Label(label).Tooltip(label).KeepFocus()
	if button.Hovered() || button.Pressed() {
		button.Background(a.pal.hover)
	}
	button.Transition(hoverFade)
	pressPose(button)
	button.Children(func() {
		ui.Text(c, locales.MarkdownTableMenu()).FontSize(a.pal.metricsOr().textBody).SingleLine().TextColor(a.pal.muted)
		ui.Icon(c, iconChevronDown).FontSize(11).TextColor(a.pal.muted)
	})
	button.Menu(func(menu *ui.Menu) { a.liveTableMenu(menu, d, &cell.block, cell.row, cell.column, cell.snap) })
}

type liveTableMenuTarget struct {
	key         string
	row, column int
	snap        ui.TextSnapshot
}

func (a *app) liveTableMenu(menu *ui.Menu, d *editor, block *liveBlock, row, column int, opened ui.TextSnapshot) {
	cell := liveTableMenuTarget{key: block.key, row: row, column: column, snap: opened}
	chosen := false
	choose := func(label string, action tableAction) {
		if !menu.Item(label).Disabled(!a.writingMarkdown(d)).Chosen() {
			return
		}
		chosen = true
		if d.tableMenuTarget != cell || opened != d.buf.Snapshot() {
			return
		}
		a.post(func() {
			if !a.writingMarkdown(d) || !d.live || d.buf.Snapshot() != opened {
				return
			}
			for index, col := range a.cols {
				if target := col.tab(); target != nil && target.ed == d {
					a.col, a.focus, d.typist = index, paneEditor, d
					a.changeTable(d, block, row, column, action)
					return
				}
			}
		})
	}
	choose(locales.MarkdownTableInsertAbove(), tableRowAbove)
	choose(locales.MarkdownTableInsertBelow(), tableRowBelow)
	choose(locales.MarkdownTableInsertLeft(), tableColumnLeft)
	choose(locales.MarkdownTableInsertRight(), tableColumnRight)
	menu.Separator()
	if len(block.cellRanges) > 1 {
		choose(locales.MarkdownTableDeleteRow(), tableDeleteRow)
	}
	if len(block.cellRanges[0]) > 1 {
		choose(locales.MarkdownTableDeleteColumn(), tableDeleteColumn)
	}
	if len(block.cellRanges) == 1 || len(block.cellRanges[0]) == 1 {
		choose(locales.MarkdownTableDelete(), tableDeleteWhole)
	}
	if !chosen {
		d.tableMenuTarget = cell
	} else {
		d.tableMenuTarget = liveTableMenuTarget{}
	}
}

type tableLine struct {
	prefix, suffix string
	left, right    bool
	cells          []string
}

type insertedTableColumn struct {
	key                string
	column, start, end int
	before             ui.TextSnapshot
}

type tableColumnInsertions struct {
	version uint64
	columns []insertedTableColumn
}

func tableLineChange(start int, before, after string) textEdit {
	left, right := []rune(before), []rune(after)
	first := 0
	for first < len(left) && first < len(right) && left[first] == right[first] {
		first++
	}
	lastLeft, lastRight := len(left), len(right)
	for lastLeft > first && lastRight > first && left[lastLeft-1] == right[lastRight-1] {
		lastLeft--
		lastRight--
	}
	return textEdit{start + first, start + lastLeft, string(right[first:lastRight])}
}

func parseTableLine(line, prefix string) tableLine {
	body := strings.TrimPrefix(line, prefix)
	trimmed := strings.TrimLeft(body, " \t\r")
	content := strings.TrimRight(trimmed, " \t\r")
	result := tableLine{prefix: line[:len(line)-len(trimmed)], suffix: trimmed[len(content):]}
	if result.left = strings.HasPrefix(content, "|"); result.left {
		content = content[1:]
	}
	if result.right = strings.HasSuffix(content, "|") && !strings.HasSuffix(content, `\|`); result.right {
		content = content[:len(content)-1]
	}
	if content == "" && !result.right {
		return result
	}
	start := 0
	for index := 0; index < len(content); index++ {
		if content[index] == '|' && (index == 0 || content[index-1] != '\\') {
			result.cells = append(result.cells, content[start:index])
			start = index + 1
		}
	}
	result.cells = append(result.cells, content[start:])
	return result
}

func tableCellStartsBlock(cell string) bool {
	text := strings.TrimLeft(cell, " \t")
	spaces := cell[:len(cell)-len(text)]
	if text == "" || len(spaces) >= 4 || strings.ContainsRune(spaces, '\t') || strings.ContainsRune("#>`~<*+-_[=", rune(text[0])) {
		return true
	}
	digits := len(text) - len(strings.TrimLeft(text, "0123456789"))
	return digits > 0 && digits <= 9 && digits < len(text) && (text[digits] == '.' || text[digits] == ')')
}

func (line tableLine) markdown() string {
	if len(line.cells) == 0 {
		return line.prefix + "|" + line.suffix
	}
	var text strings.Builder
	text.WriteString(line.prefix)
	if line.left || strings.Trim(line.cells[0], " \t\r") == "" {
		text.WriteByte('|')
	}
	for index, cell := range line.cells {
		if index > 0 {
			if strings.HasSuffix(line.cells[index-1], `\`) {
				text.WriteByte(' ')
			}
			text.WriteByte('|')
		}
		text.WriteString(cell)
	}
	if line.right || strings.Trim(line.cells[len(line.cells)-1], " \t\r") == "" {
		text.WriteByte('|')
	}
	text.WriteString(line.suffix)
	return text.String()
}

func editedTableLayout(source []byte, rows, columns int) (liveBlock, bool) {
	runeAt, lineStart, lineEnd := liveSourceOffsets(source)
	tree := markdownParser(false).Parse(text.NewReader(source))
	var block liveBlock
	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if table, ok := node.(*east.Table); entering && ok {
			block = liveTableBlock(source, table, 0, runeAt, lineStart, lineEnd)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if block.start != 0 || block.end != len(source) || len(block.cellRanges) != rows || len(block.cellRanges[0]) != columns {
		return liveBlock{}, false
	}
	block.end = runeAt(block.end)
	return block, true
}

func preserveTableAssets(block, before *liveBlock, snap ui.TextSnapshot) {
	type cellKey struct {
		source string
		header bool
	}
	loaded := map[cellKey][]markdownInline{}
	for _, row := range before.blocks {
		if (row.kind != markdownTable && row.kind != markdownTableHeader) || row.row >= len(before.cellRanges) {
			continue
		}
		for column, bounds := range before.cellRanges[row.row] {
			if column >= len(row.cells) {
				break
			}
			cell := row.cells[column]
			source := snap.Slice(before.start+bounds.start, before.start+bounds.end)
			if strings.Contains(source, "[[") || slices.ContainsFunc(cell, func(run markdownInline) bool { return run.bitmap != nil || run.math != nil }) {
				loaded[cellKey{source, row.kind == markdownTableHeader}] = cell
			}
		}
	}
	if len(loaded) == 0 {
		return
	}
	source := []rune(block.source)
	for index := range block.blocks {
		row := &block.blocks[index]
		if (row.kind != markdownTable && row.kind != markdownTableHeader) || row.row >= len(block.cellRanges) {
			continue
		}
		for column, bounds := range block.cellRanges[row.row] {
			if column >= len(row.cells) {
				break
			}
			key := cellKey{string(source[bounds.start:bounds.end]), row.kind == markdownTableHeader}
			if runs, ok := loaded[key]; ok {
				row.cells[column] = runs
			} else if runs, ok := loaded[cellKey{key.source, !key.header}]; ok {
				retained := slices.Clone(runs)
				styled := row.cells[column]
				for index := range retained {
					if key.header {
						retained[index].bold = true
					} else if index < len(styled) && retained[index].text == styled[index].text && retained[index].image == styled[index].image {
						retained[index].bold = styled[index].bold
					}
				}
				row.cells[column] = retained
			}
		}
	}
}

func (a *app) changeTable(d *editor, target *liveBlock, row, column int, action tableAction) {
	if !a.writingMarkdown(d) || !d.live || target == nil || target.stale || len(target.cellRanges) == 0 || target.end > d.buf.Len() {
		return
	}
	snap := d.buf.Snapshot()
	lines := strings.Split(snap.Slice(target.start, target.end), "\n")
	rows, columns := len(target.cellRanges), len(target.cellRanges[0])
	if len(lines) != rows+1 || len(target.rowPrefixes) != rows || row < 0 || row >= rows || column < 0 || column >= columns {
		return
	}
	if action == tableDeleteWhole {
		d.tableCell, d.tableFocusAt, d.focusNext = nil, 0, true
		end := target.end
		if end < snap.Len() && snap.Slice(end, end+1) == "\n" {
			end++
			if target.start > 1 && snap.Slice(target.start-2, target.start) == "\n\n" && end < snap.Len() && snap.Slice(end, end+1) == "\n" {
				end++
			}
		}
		d.edit([2]int{target.start, target.start}, textEdit{target.start, end, ""})
		a.invalidate()
		return
	}
	original := slices.Clone(lines)
	starts := make([]int, len(lines))
	starts[0] = target.start
	for index := 1; index < len(lines); index++ {
		starts[index] = starts[index-1] + utf8.RuneCountInString(lines[index-1]) + 1
	}
	var edits []textEdit
	headPrefix := lines[0][:min(target.rowPrefixes[0], len(lines[0]))]
	continuation := lines[1][:len(lines[1])-len(strings.TrimLeft(lines[1], " \t>"))]
	physical := func(row int) int {
		if row == 0 {
			return 0
		}
		return row + 1
	}
	linePrefix := func(index int) string {
		if index == 1 {
			return continuation
		}
		row := max(0, index-1)
		return lines[index][:min(target.rowPrefixes[row], len(lines[index]))]
	}
	viewRow, viewCol := row, column
	wantRows, wantColumns := rows, columns
	insertions := &d.document.live.columnInsertions
	if insertions.version != snap.Version() {
		insertions.columns = nil
	}
	var prior []string
	switch action {
	case tableRowAbove, tableRowBelow:
		wantRows++
		if action == tableRowBelow {
			viewRow++
		}
		blank := "|" + strings.Repeat("  |", columns)
		if viewRow == 0 {
			first := lines[0]
			lines[0] = headPrefix + blank
			lines = slices.Insert(lines, 2, continuation+strings.TrimPrefix(first, headPrefix))
			edits = append(edits, tableLineChange(starts[0], original[0], lines[0]))
		} else {
			lines = slices.Insert(lines, physical(viewRow), continuation+blank)
		}
		index := max(2, physical(viewRow))
		if index < len(original) {
			edits = append(edits, textEdit{starts[index], starts[index], lines[index] + "\n"})
		} else {
			edits = append(edits, textEdit{target.end, target.end, "\n" + lines[index]})
		}
	case tableDeleteRow:
		wantRows--
		if rows <= 1 {
			return
		}
		if row == 0 {
			lines[0] = headPrefix + strings.TrimPrefix(lines[2], linePrefix(2))
			header := parseTableLine(lines[0], headPrefix)
			if len(header.cells) > columns {
				wantColumns = len(header.cells)
				delimiter := parseTableLine(lines[1], continuation)
				for len(delimiter.cells) < len(header.cells) {
					delimiter.cells = append(delimiter.cells, " --- ")
				}
				lines[1] = delimiter.markdown()
				edits = append(edits, tableLineChange(starts[1], original[1], lines[1]))
			}
			edits = append(edits, tableLineChange(starts[0], original[0], lines[0]))
			lines = slices.Delete(lines, 2, 3)
		} else {
			lines = slices.Delete(lines, physical(row), physical(row)+1)
		}
		index := max(2, physical(row))
		if index+1 < len(original) {
			edits = append(edits, textEdit{starts[index], starts[index+1], ""})
		} else {
			edits = append(edits, textEdit{starts[index] - 1, target.end, ""})
		}
		viewRow = min(row, rows-2)
	case tableColumnLeft, tableColumnRight, tableDeleteColumn:
		wantColumns++
		if action == tableColumnRight {
			viewCol++
		}
		if action == tableDeleteColumn {
			wantColumns = columns - 1
			if wantColumns == 0 {
				return
			}
		}
		if action == tableDeleteColumn && len(insertions.columns) != 0 {
			added := insertions.columns[len(insertions.columns)-1]
			if added.key == target.key && added.column == column {
				prior = strings.Split(added.before.Slice(added.start, added.end), "\n")
			}
		}
		for index, raw := range lines {
			if prior != nil {
				lines[index] = prior[index]
			} else {
				line := parseTableLine(raw, linePrefix(index))
				if len(line.cells) < columns && viewCol >= len(line.cells) {
					continue
				}
				if action == tableDeleteColumn {
					line.cells = slices.Delete(line.cells, column, column+1)
					if index != 1 && column == 0 && len(line.cells) != 0 && tableCellStartsBlock(line.cells[0]) {
						line.left = true
					}
					if index == 1 && wantColumns == 1 && !line.left {
						line.right = true
					}
				} else {
					value := "  "
					if index == 1 {
						value = " --- "
					}
					line.cells = slices.Insert(line.cells, viewCol, value)
				}
				lines[index] = line.markdown()
			}
			if lines[index] != raw {
				edits = append(edits, tableLineChange(starts[index], raw, lines[index]))
			}
		}
		if action == tableDeleteColumn {
			viewCol = min(column, columns-2)
		}
	}
	source := []byte(strings.Join(lines, "\n"))
	block, ok := editedTableLayout(source, wantRows, wantColumns)
	if !ok {
		return
	}
	block.blocks = parseMarkdown(source, d.path, a.root, func() bool { return false }, &markdownCache{math: a.prefs().PreviewMath})
	preserveTableAssets(&block, target, snap)
	block.start, block.end, block.key = block.start+target.start, block.end+target.start, target.key
	cell := block.cellRanges[viewRow][viewCol]
	selection := [2]int{block.start + cell.start, block.start + cell.end}
	d.edit(selection, edits...)
	d.quiet = true
	switch {
	case action == tableColumnLeft || action == tableColumnRight:
		if len(insertions.columns) == maxUndo {
			insertions.columns = slices.Delete(insertions.columns, 0, 1)
		}
		insertions.columns = append(insertions.columns, insertedTableColumn{target.key, viewCol, target.start, target.end, snap})
	case prior != nil:
		insertions.columns = slices.Delete(insertions.columns, len(insertions.columns)-1, len(insertions.columns))
	default:
		insertions.columns = nil
	}
	insertions.version = d.buf.Version()
	projection := &liveTableProjection{block: block, snap: d.buf.Snapshot()}
	d.document.live.editedTable = projection
	a.beginLiveCell(d, &projection.block, viewRow, viewCol)
}

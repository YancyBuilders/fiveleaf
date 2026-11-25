package main

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

type liveTableProjection struct {
	block liveBlock
	snap  ui.TextSnapshot
}

type liveCellEditor struct {
	liveTableProjection
	row, column int
	text        string
	focus       bool
	selection   [2]int
	input       ui.Handle
}

func (a *app) enterTableAtCaret(d *editor, block *liveBlock, snap ui.TextSnapshot, caret int) {
	row := min(max(0, snap.LineAt(caret)-snap.LineAt(block.start)-1), len(block.cellRanges)-1)
	cells := block.cellRanges[row]
	column := 0
	for column+1 < len(cells) && block.start+cells[column].end < caret {
		column++
	}
	a.beginLiveCell(d, block, row, column)
}

func eachEditingBlock(doc *liveDoc, d *editor, from, to int, show func(*liveBlock)) {
	if doc.edit == nil && d.document.live.editedTable != nil && d.document.live.editedTable.snap == doc.snap {
		d.document.live.editedTable = nil
	}
	pending := d.document.live.editedTable
	var active *liveBlock
	if edit := d.tableCell; edit != nil && edit.snap == doc.snap && len(edit.block.cellRanges) >= liveTableWindowRows {
		active = &edit.block
	}
	doc.eachBlock(from, to, func(block *liveBlock) {
		if pending != nil && pending.snap == doc.snap && block.key == pending.block.key {
			return
		}
		if active != nil && active.key == block.key {
			active = nil
		}
		show(block)
	})
	if pending != nil && pending.snap == doc.snap && pending.block.end >= from && pending.block.start <= to {
		show(&pending.block)
		if active != nil && pending.block.key == active.key {
			active = nil
		}
	}
	if active != nil {
		show(active)
	}
}

func (a *app) beginLiveCell(d *editor, block *liveBlock, row, column int) {
	if !a.writingMarkdown(d) || row < 0 || row >= len(block.cellRanges) || column < 0 || column >= len(block.cellRanges[row]) {
		return
	}
	for _, col := range a.cols {
		if tab := col.tab(); tab != nil && tab.ed == d {
			a.col, a.focus, d.typist = slices.Index(a.cols, col), paneEditor, d
			break
		}
	}
	cell := block.cellRanges[row][column]
	snap := d.buf.Snapshot()
	start, end := block.start+cell.start, block.start+cell.end
	if start < 0 || end < start || end > snap.Len() {
		return
	}
	edit := &liveCellEditor{liveTableProjection: liveTableProjection{block: *block, snap: snap}, row: row, column: column, focus: true}
	edit.text = strings.ReplaceAll(snap.Slice(start, end), `\|`, "|")
	edit.selection = [2]int{0, utf8.RuneCountInString(edit.text)}
	d.tableCell, d.tableFocusAt, d.focusNext = edit, 0, false
	d.selectRange(start, end, false)
	d.quiet = true
	d.sel = [2]int{start, end}
	a.invalidate()
}

func (a *app) liveCellView(c *ui.Context, d *editor, block *liveBlock, row, column int, box ui.Element, size float32, blockInput func(ui.InputEvent) bool) func() {
	snap := d.buf.Snapshot()
	cell := block.cellRanges[row][column]
	box.ContextMenu(func(menu *ui.Menu) { a.liveTableMenu(menu, d, block, row, column, snap) })
	if cell.missing == 0 && cell.start == cell.end {
		box.Label(locales.MarkdownTableCell(strconv.Itoa(row+1), strconv.Itoa(column+1)))
	}
	if d.tableFocusAt > 0 && d.tableFocusAt == block.start+cell.start+1 {
		a.beginLiveCell(d, block, row, column)
	}
	edit := d.tableCell
	if edit == nil || edit.block.key != block.key || edit.row != row || edit.column != column || !a.writingMarkdown(d) {
		box.Cursor(ui.CursorText).HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputPointerDown && ev.Button == 0 && ev.Clicks == 1 && d.buf.Snapshot() == snap {
				a.beginLiveCell(d, block, row, column)
				return true
			}
			return blockInput(ev)
		})
		return nil
	}
	box.Background(a.pal.editor).BorderColor(a.pal.accent)
	return func() {
		input := ui.TextInputBase(c.Key("cell-input"), &edit.text).Bind(&edit.input).FillWidth().MinWidth(0).
			Height(size * a.settings.LineHeight).FontSize(size).TextColor(a.pal.text).Label(locales.MarkdownTableCell(strconv.Itoa(row+1), strconv.Itoa(column+1)))
		input.ContextMenu(func(menu *ui.Menu) {
			menu.EditItems()
			menu.Separator()
			a.liveTableMenu(menu, d, &edit.block, row, column, snap)
		})
		if edit.focus {
			edit.focus = false
			input.Focus().SetTextSelection(edit.selection[0], edit.selection[1])
		}
		composing := input.Composing()
		a.takeKeys(input, placeInput, func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll {
				return blockInput(ev)
			}
			if d.tableCell != edit || !a.writingMarkdown(d) || composing {
				return false
			}
			return a.liveCellKey(d, edit, ev)
		})
		if !composing && (input.Changed() || input.Focused()) {
			anchor, caret := input.TextAnchorCaret()
			if input.Changed() {
				a.writeLiveCell(d, edit, anchor, caret)
			} else if selection := [2]int{anchor, caret}; edit.selection != selection {
				cell := edit.block.cellRanges[row][column]
				start := edit.block.start + cell.start
				d.selectRange(start+tableCellOffset(edit.text, anchor), start+tableCellOffset(edit.text, caret), false)
				d.quiet = true
			}
			edit.selection = [2]int{anchor, caret}
		}
	}
}

func tableCellMarkdown(value string) string {
	return tableCellEscapes.Replace(value)
}

var tableCellEscapes = strings.NewReplacer("|", `\|`, "\r\n", "<br>", "\n", "<br>", "\r", "<br>")

func tableCellOffset(text string, offset int) int {
	runes := []rune(text)
	return utf8.RuneCountInString(tableCellMarkdown(string(runes[:min(max(offset, 0), len(runes))])))
}

func (a *app) writeLiveCell(d *editor, edit *liveCellEditor, anchor, caret int) {
	if d.tableCell != edit || edit.snap != d.buf.Snapshot() || !a.writingMarkdown(d) {
		return
	}
	block := &edit.block
	cell := block.cellRanges[edit.row][edit.column]
	start, end := block.start+cell.start, block.start+cell.end
	value := tableCellMarkdown(edit.text)
	before, after := "", ""
	if cell.missing > 0 {
		row := strings.TrimRight(edit.snap.Line(edit.snap.LineAt(start)), " \t\r")
		before, after = strings.Repeat("  |", cell.missing-1)+" ", " |"
		if !strings.HasSuffix(row, "|") || strings.HasSuffix(row, `\|`) {
			before = " |" + before
		}
	} else if strings.TrimSpace(value) == "" {
		lineStart, lineEnd := edit.snap.LineRange(edit.snap.LineAt(start))
		if edit.column == 0 && !strings.Contains(edit.snap.Slice(lineStart, start), "|") {
			before = "| "
		}
		if edit.column+1 == len(block.cellRanges[edit.row]) && !strings.Contains(edit.snap.Slice(end, lineEnd), "|") {
			after = " |"
		}
	} else if strings.HasSuffix(value, `\`) && end < edit.snap.Len() && edit.snap.Slice(end, end+1) == "|" {
		after = " "
	}
	if before == "" && after == "" && edit.snap.Slice(start, end) == value {
		return
	}
	inserted := start + len(before)
	d.edit([2]int{inserted + tableCellOffset(edit.text, anchor), inserted + tableCellOffset(edit.text, caret)}, textEdit{start, end, before + value + after})
	d.quiet = true
	delta := d.buf.Len() - edit.snap.Len()
	edit.snap = d.buf.Snapshot()
	edit.resizeCell(delta, len(before), len(after))
	edit.updateProjection(d)
	a.invalidate()
}

func (edit *liveCellEditor) updateProjection(d *editor) {
	block := &edit.block
	if edit.row < len(block.blocks) && edit.column < len(block.blocks[edit.row].cells) {
		block.blocks = slices.Clone(block.blocks)
		rendered := &block.blocks[edit.row]
		rendered.cells = slices.Clone(rendered.cells)
		rendered.cells[edit.column] = []markdownInline{{text: edit.text, bold: rendered.kind == markdownTableHeader}}
	}
	projection := edit.liveTableProjection
	d.document.live.editedTable = &projection
}

func (edit *liveCellEditor) resizeCell(delta, prefix, suffix int) {
	if delta == 0 && prefix == 0 && suffix == 0 {
		return
	}
	block := &edit.block
	cell := block.cellRanges[edit.row][edit.column]
	block.end += delta
	block.cellRanges = slices.Clone(block.cellRanges)
	for row := edit.row; row < len(block.cellRanges); row++ {
		cells := slices.Clone(block.cellRanges[row])
		for column := range cells {
			next := &cells[column]
			switch {
			case row == edit.row && column == edit.column:
				next.start, next.end, next.missing = cell.start+prefix, cell.end+delta-suffix, 0
			case row == edit.row && column < edit.column && next.missing > 0 && next.missing < cell.missing:
				at := cell.start + prefix - 3*(cell.missing-next.missing)
				next.start, next.end, next.missing = at, at, 0
			case row > edit.row || column > edit.column:
				next.start, next.end = next.start+delta, next.end+delta
				if row == edit.row && cell.missing > 0 {
					next.missing -= cell.missing
				}
			}
		}
		block.cellRanges[row] = cells
	}
}

func (a *app) liveCellKey(d *editor, edit *liveCellEditor, ev ui.InputEvent) bool {
	undo := ev.Kind == ui.InputCommand && ev.Text == "undo" || ev.Kind == ui.InputKeyDown && ev.Mods == ui.Cmd && ev.Key == ui.KeyZ
	redo := ev.Kind == ui.InputCommand && ev.Text == "redo" || ev.Kind == ui.InputKeyDown && (ev.Mods == ui.Cmd|ui.Shift && ev.Key == ui.KeyZ || !macKeys() && ev.Mods == ui.Ctrl && ev.Key == ui.KeyY)
	if undo || redo {
		if undo {
			d.undo()
		} else {
			d.redo()
		}
		snap := d.buf.Snapshot()
		if edit.snap == snap {
			return true
		}
		cell := edit.block.cellRanges[edit.row][edit.column]
		start, end := edit.block.start+cell.start, edit.block.start+cell.end
		from, previous, next := snapDiff(edit.snap, snap)
		if from >= start && previous <= end && !strings.ContainsAny(snap.Slice(from, next), "\r\n|") {
			edit.resizeCell(next-previous, 0, 0)
			end += next - previous
			edit.snap, edit.focus = snap, true
			edit.text = strings.ReplaceAll(snap.Slice(start, end), `\|`, "|")
			position := func(at int) int {
				return utf8.RuneCountInString(strings.ReplaceAll(snap.Slice(start, min(max(start, at), end)), `\|`, "|"))
			}
			edit.selection = [2]int{position(d.sel[0]), position(d.sel[1])}
			d.quiet = true
			edit.updateProjection(d)
		} else {
			d.tableCell, d.focusNext = nil, true
		}
		a.invalidate()
		return true
	}
	if ev.Kind != ui.InputKeyDown || ev.Mods != 0 && ev.Mods != ui.Shift {
		return false
	}
	row, column, step := edit.row, edit.column, 1
	if ev.Mods == ui.Shift {
		step = -1
	}
	switch ev.Key {
	case ui.KeyTab:
		column += step
		if column < 0 || column >= len(edit.block.cellRanges[row]) {
			row += step
			column = 0
			if step < 0 && row >= 0 {
				column = len(edit.block.cellRanges[row]) - 1
			}
		}
	case ui.KeyEnter:
		row += step
	case ui.KeyEscape:
		row = len(edit.block.cellRanges)
	default:
		return false
	}
	if row >= 0 && row < len(edit.block.cellRanges) {
		a.beginLiveCell(d, &edit.block, row, column)
	} else {
		d.tableCell = nil
		d.focusNext = true
		if edit.snap == d.buf.Snapshot() {
			leaveLiveTable(d, edit, step)
		}
		a.invalidate()
	}
	return true
}

func leaveLiveTable(d *editor, edit *liveCellEditor, step int) {
	at := edit.block.end
	if step < 0 {
		at = edit.block.start
	}
	snap := edit.snap
	missing := 2
	for line := snap.LineAt(at) + step; missing > 0 && line >= 0 && line < snap.LineCount(); line += step {
		start, end := snap.LineRange(line)
		at = start
		if step < 0 {
			at = end
		}
		missing--
		if !liveBlank(snap, line) {
			break
		}
	}
	if missing == 0 {
		d.selectRange(at, at, false)
		return
	}
	caret := at
	if step > 0 {
		caret += missing
	}
	d.edit([2]int{caret, caret}, textEdit{at, at, strings.Repeat("\n", missing)})
}

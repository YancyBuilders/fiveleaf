package main

import (
	"strconv"
	"strings"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

const maxMdTableDimension = 100

const tableGridColumns, tableGridRows = 10, 8

func (a *app) chooseMdTableSize(d *editor) {
	if a.activeEditor() != d {
		return
	}
	snap, _, _, ok := mdFormatRange(d)
	if !ok {
		return
	}
	selection := d.sel
	rowsText, columnsText := "2", "2"
	rows, columns := 2, 2
	gridPressed, gridSubmit := false, false
	a.present(&confirm{
		title: locales.CmdMarkdownInsertTable(), ok: locales.MarkdownTableInsert(),
		fields: func(c *ui.Context) (valid, submitted bool) {
			width, _ := c.Size()
			pitch := min(float32(24), max(1, (width*.92-32)/tableGridColumns))
			gridWidth, gridHeight := pitch*tableGridColumns, pitch*tableGridRows
			dimensions := locales.MarkdownTableDimensions(columnsText, rowsText)
			grid := ui.Box(c.Key("table-grid")).Size(gridWidth, gridHeight).Focusable().FocusRing(true).AutoFocus().
				Role(ui.RoleButton).Label(locales.MarkdownTableChooseSize()).Description(dimensions + ". " + locales.MarkdownTableGridHint()).Tooltip(locales.MarkdownTableGridHint()).Cursor(ui.CursorPointer)
			grid.HandleInput(func(event ui.InputEvent) bool {
				set := func(r, col int) {
					rows, columns = min(max(1, r), tableGridRows), min(max(1, col), tableGridColumns)
					rowsText, columnsText = strconv.Itoa(rows), strconv.Itoa(columns)
				}
				switch event.Kind {
				case ui.InputPointerMove, ui.InputPointerDown, ui.InputPointerUp:
					if event.Kind != ui.InputPointerMove && event.Button != 0 {
						return false
					}
					inside := event.X >= 0 && event.Y >= 0 && event.X < gridWidth && event.Y < gridHeight
					if inside {
						set(int(event.Y/pitch)+1, int(event.X/pitch)+1)
					}
					if event.Kind == ui.InputPointerUp {
						gridSubmit, gridPressed = gridPressed && inside, false
					}
					if event.Kind == ui.InputPointerDown {
						gridPressed = inside
					}
					return true
				case ui.InputKeyDown:
					if event.Mods != 0 {
						return false
					}
					switch event.Key {
					case ui.KeyLeft:
						set(rows, columns-1)
					case ui.KeyRight:
						set(rows, columns+1)
					case ui.KeyUp:
						set(rows-1, columns)
					case ui.KeyDown:
						set(rows+1, columns)
					case ui.KeyEnter, ui.KeySpace:
						gridSubmit = true
					default:
						return false
					}
					return true
				}
				return false
			})
			grid.Draw(func(painter *ui.Painter, bounds ui.Rect) {
				for row := range tableGridRows {
					for col := range tableGridColumns {
						cell := ui.Rect{X: bounds.X + float32(col)*pitch + 2, Y: bounds.Y + float32(row)*pitch + 2, W: pitch - 4, H: pitch - 4}
						line, fill := a.pal.border, a.pal.editor
						if row < rows && col < columns {
							line, fill = a.pal.accent, a.pal.accent.Alpha(.18)
						}
						painter.Fill(cell, fill, 3)
						painter.Stroke(cell, line, 3, 1)
					}
				}
			})
			ui.Text(c, dimensions).FontSize(a.pal.metricsOr().textBody).TextColor(a.pal.muted)
			submitted, gridSubmit = gridSubmit, false
			field := func(id, label string, text *string) int {
				var input ui.Element
				ui.Row(c.Key(id)).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, label).FontSize(a.pal.metricsOr().textBody).Grow(1)
					input = ui.TextInput(c, text).Label(label).Width(80)
				})
				submitted = submitted || input.Submitted()
				value, err := strconv.Atoi(strings.TrimSpace(*text))
				if err != nil || value < 1 || value > maxMdTableDimension {
					return 0
				}
				return value
			}
			rows = field("rows", locales.MarkdownTableRows(), &rowsText)
			columns = field("columns", locales.MarkdownTableColumns(), &columnsText)
			if rows == 0 || columns == 0 {
				ui.Text(c, locales.MarkdownTableSizeRange(strconv.Itoa(maxMdTableDimension))).FontSize(a.pal.metricsOr().textBody).TextColor(a.pal.dangerFg)
				return false, false
			}
			return true, submitted
		},
		cancel: a.focusColumn,
		run: func() {
			if a.activeEditor() != d || d.locked() {
				return
			}
			if d.buf.Version() != snap.Version() {
				a.fail("%s", locales.MarkdownTableSelectionChanged())
				a.focusColumn()
				return
			}
			d.sel = selection
			insertMdTable(d, rows, columns)
			a.focusColumn()
			a.invalidate()
		},
	})
}

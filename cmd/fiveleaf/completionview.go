package main

import (
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (a *app) wordSuggestionsView(c *ui.Context, doc *editor) {
	request := a.wordSuggest
	if request == nil || request.editor != doc || len(request.items) == 0 {
		return
	}
	caretX, caretY, lineH, ok := doc.lines.Caret(request.selection[1])
	x, y := doc.textX+caretX-doc.hscroll.X, editorPadY+caretY-doc.vscroll.Y
	view := doc.lastView
	if !ok || x < 0 || x >= view.W || y < float32(len(doc.sticky))*a.font.lineH || y+lineH > view.H {
		a.dismissWordSuggestions()
		return
	}
	const margin, rowH = 4, 26
	above, below := y-margin, view.H-y-lineH-margin
	rows := min(6, len(request.items), int((max(above, below)-2*margin)/rowH))
	width := min(float32(320), view.W-2*margin)
	if rows <= 0 || width <= 0 {
		a.dismissWordSuggestions()
		return
	}
	height := float32(rows*rowH + 2*margin)
	top := y + lineH
	if height > below {
		top = y - height
	}
	box := ui.Column(c.Key("word-suggestions")).Absolute().Left(max(margin, min(x, view.W-width-margin))).Top(top).
		Width(width).Padding(margin).Radius(a.pal.metricsOr().radiusCard).Role(ui.RoleList).Label(locales.CompletionPlaceholder())
	a.floating(c, box, false).Children(func() {
		first := max(0, request.selected-rows+1)
		for index := first; index < first+rows; index++ {
			item := request.items[index]
			row := ui.ButtonBase(c.Key(index)).KeepFocus().Height(rowH).Padding(0, 6).Gap(7).Radius(4).
				AlignItems(ui.Center).Justify(ui.Start).Role(ui.RoleListItem).Label(item.Label).Selected(index == request.selected)
			if index == request.selected {
				row.Background(a.pal.accent.Alpha(0.16))
			} else if row.Hovered() {
				row.Background(a.pal.hover)
			}
			row.Children(func() {
				ui.Icon(c, namedIcon(completionIcon(item.Kind))).FontSize(13).TextColor(a.pal.muted).Shrink(0)
				ui.Text(c, item.Label).Font(a.font.family).FontSize(12).TextColor(a.pal.text).SingleLine().Shrink(1).MinWidth(0)
				if item.Detail != "" {
					ui.Text(c, item.Detail).FontSize(11).TextColor(a.pal.muted).SingleLine().Shrink(1).MinWidth(0)
				}
			})
			if row.Clicked() {
				request.accept(a, index)
				a.dismissWordSuggestions()
			}
		}
	})
}

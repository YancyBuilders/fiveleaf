package main

import (
	"strings"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type inlineGhost struct {
	caret      int
	text, next ui.Span
}

func (a *app) inlineGhost(area ui.Element, doc *editor, snap ui.TextSnapshot, font ui.Font) *inlineGhost {
	pending := a.inlineCopilot
	if pending == nil || pending.editor != doc || pending.text == "" {
		return nil
	}
	area.Description(locales.CopilotInlinePreview(pending.text))
	ghost := &inlineGhost{caret: pending.caret, text: ui.Span{Text: strings.ReplaceAll(pending.text, "\t", strings.Repeat(" ", a.font.tabSize)),
		Font: font.Family, Size: font.Size, Weight: font.Weight, Color: a.pal.faint}}
	if _, end := snap.LineRange(snap.LineAt(ghost.caret)); ghost.caret < end {
		ghost.next = ghost.text
		ghost.next.Text = snap.Slice(ghost.caret, ghost.caret+1)
		ghost.next.Color = a.pal.text
		width, _ := ui.ShapeText(ghost.text.Text+ghost.next.Text, font, 0).Size()
		area.TextRanges(ui.TextRange{Start: ghost.caret, End: ghost.caret + 1, ObjectWidth: width, ObjectHeight: font.Size * 0.8, ObjectDescent: font.Size * 0.2})
	}
	return ghost
}

func (ghost *inlineGhost) paint(painter *ui.Painter, doc *editor, originX, originY float32, clip ui.Rect) {
	if ghost == nil {
		return
	}
	x, y, height, ok := doc.lines.Caret(ghost.caret)
	if !ok {
		return
	}
	_, inkHeight := painter.MeasureText(0, ghost.text)
	painter.Clip(clip, 0, func() {
		painter.RichText(originX+x, originY+y+(height-inkHeight)/2, 0, ghost.text, ghost.next)
	})
}

func (a *app) inlineCompletionControls(c *ui.Context, doc *editor) {
	pending := a.inlineCopilot
	if pending == nil || pending.editor != doc || pending.text == "" {
		return
	}
	palette := a.pal
	ui.Row(c.Key("inline-completion-actions")).Absolute().Right(14).Bottom(8).Padding(2).Gap(2).
		Background(palette.surface).Border(1, palette.border).Radius(7).Children(func() {
		action := func(key, label string, icon *ui.SVG) bool {
			button := ui.ButtonBase(c.Key(key)).KeepFocus().Height(23).Padding(0, 6).Gap(4).Radius(5).
				AlignItems(ui.Center).Justify(ui.Center).Label(label).Tooltip(label + "\n" + pending.text)
			ink := palette.muted
			if button.Hovered() || button.FocusVisible() {
				button.Background(palette.surfaceHover)
				ink = palette.text
			}
			button.Children(func() {
				ui.Icon(c, icon).FontSize(12).TextColor(ink)
				ui.Text(c, key).FontSize(palette.metricsOr().textSmall).TextColor(ink)
			})
			return button.Clicked()
		}
		if action("Tab", locales.CopilotInlineAccept(), iconCheck) {
			a.acceptInlineCompletion(pending)
		}
		if action("Esc", locales.CopilotInlineDismiss(), iconX) {
			a.dismissInlineCompletion()
			a.invalidate()
		}
	})
}

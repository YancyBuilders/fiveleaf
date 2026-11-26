package main

import (
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

const (
	chatDraftMinHeight    = 52
	chatDraftMaxHeight    = 144
	chatDraftMeasureBytes = 8 << 10
)

func (a *app) chatComposeBox(c *ui.Context) ui.Element {
	palette := a.pal
	metrics := palette.metricsOr()
	box := ui.Column(c).FillWidth().MinWidth(0).Border(1, palette.border).
		Radius(metrics.radiusCard + metrics.pad).Background(palette.surface).Transition(fieldFocus)
	material := materialOf(c)
	if material.mica() {
		material.glassCard(box, palette, metrics.radiusCard+2*metrics.pad)
	}
	if box.FocusWithin() {
		box.BorderColor(chatComposeInk(palette, palette.accent))
	}
	return box
}

func (a *app) chatDraftInput(c *ui.Context, draft *string, bind *ui.Handle, placeholder string) ui.Element {
	metrics := a.pal.metricsOr()
	paddingX, paddingY := metrics.inset+metrics.gap, metrics.pad
	_, windowHeight := c.Size()
	heightLimit := min(chatDraftMaxHeight, max(chatDraftMinHeight, windowHeight/3))
	input := a.textKeys(ui.TextAreaBase(c, draft)).Bind(bind).Label(locales.AgentDraft()).Placeholder(placeholder).
		FillWidth().MinWidth(0).MinHeight(chatDraftMinHeight).FontSize(metrics.textBody).
		BorderWidth(paddingY, 0).PaddingX(paddingX).Radius(metrics.radiusCard)
	if len(*draft) > chatDraftMeasureBytes {
		input.Height(heightLimit)
	} else {
		input.Lines(2, 8).MaxHeight(heightLimit)
	}
	return a.drawChatDraftCaret(c, input, paddingX, paddingY)
}

func chatComposeInk(palette *palette, color ui.Color) ui.Color {
	color.A = 255
	return legible(color, palette.dark, palette.surface)
}

type chatDraftLayout struct {
	lines  ui.TextLines
	scroll ui.ScrollState
}

func (a *app) drawChatDraftCaret(c *ui.Context, input ui.Element, paddingX, paddingY float32) ui.Element {
	layout := ui.Local(input, "draft-layout", func() chatDraftLayout { return chatDraftLayout{} })
	input.TrackLines(&layout.lines).TrackScroll(&layout.scroll)
	ink := chatComposeInk(a.pal, a.pal.text)
	theme := c.Theme()
	var nativeAccent ui.Color
	var tinted, composing bool
	return input.Draw(func(_ *ui.Painter, _ ui.Rect) {
		if !input.Focused() || input.IsDisabled() || a.chording() {
			return
		}
		nativeAccent, tinted, composing = theme.Accent, true, input.Composing()
		theme.Accent = ui.Transparent
		if composing {
			theme.Accent = ink
		}
	}).DrawOver(func(painter *ui.Painter, bounds ui.Rect) {
		if !tinted {
			return
		}
		theme.Accent, tinted = nativeAccent, false
		if composing {
			return
		}
		_, offset := input.TextAnchorCaret()
		caret, ok := caretAt(&layout.lines, offset, 2)
		if !ok {
			return
		}
		caret.X += bounds.X + paddingX
		caret.Y += bounds.Y + paddingY - layout.scroll.Y
		bounds.Y += paddingY
		bounds.H -= 2 * paddingY
		if caret.Y < bounds.Y+bounds.H && caret.Y+caret.H > bounds.Y {
			painter.Clip(bounds, 0, func() { painter.Fill(caret, ink, 0) })
		}
	})
}

func chatSubmitKey(key ui.TextKey) bool {
	return key.Key == ui.KeyEnter && (key.Mods == 0 || key.Mods == ui.Cmd)
}

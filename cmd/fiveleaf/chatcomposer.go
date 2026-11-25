package main

import (
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (a *app) chatComposeBox(c *ui.Context) ui.Element {
	palette := a.pal
	metrics := palette.metricsOr()
	box := ui.Column(c).FillWidth().MinWidth(0).Border(1, palette.border).
		Radius(metrics.radiusCard + metrics.pad).Background(palette.surface).Transition(fieldFocus)
	material := materialOf(c)
	focus := palette.accentDim
	if material.mica() {
		material.glassCard(box, palette, metrics.radiusCard+2*metrics.pad)
		focus = palette.accent.Mix(palette.muted, 0.45).Alpha(0.6)
	}
	if box.FocusWithin() {
		box.BorderColor(focus)
	}
	return box
}

func (a *app) chatDraftInput(c *ui.Context, draft *string, bind *ui.Handle, placeholder string) ui.Element {
	metrics := a.pal.metricsOr()
	return a.textKeys(ui.TextArea(c, draft)).Bind(bind).Label(locales.AgentDraft()).Placeholder(placeholder).
		FillWidth().Height(52).MinHeight(52).FontSize(metrics.textBody).PaddingY(metrics.pad).PaddingX(metrics.inset + metrics.gap).
		Background(ui.Transparent).BorderWidth(0).Radius(metrics.radiusCard)
}

func chatSubmitKey(key ui.TextKey) bool {
	return key.Key == ui.KeyEnter && (key.Mods == 0 || key.Mods == ui.Cmd)
}

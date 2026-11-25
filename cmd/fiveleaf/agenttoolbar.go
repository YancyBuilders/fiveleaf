package main

import (
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (a *app) chatTabIcon(c *ui.Context, chat *agentChat, title ui.Element, tip string, surface ui.Color) {
	badge := ui.Box(c).Size(20, 20).Shrink(0).Justify(ui.Center).AlignItems(ui.Center)
	ink := a.pal.muted
	if chat == a.agent {
		fill := chatBadgeColor(a.pal.fill, surface)
		ink = chatBadgeColor(a.pal.fillText, fill)
		label := locales.AgentSelectionAutomatic()
		badge.Background(fill).Radius(max(0, a.pal.metricsOr().radius-2)).Role(ui.RoleImage).Label(label)
		if tip != "" {
			tip += "\n"
		}
		tip += label
	}
	badge.Children(func() {
		mark := findChatProvider(chat.provider, a.agentProviders()).chatMark(a.pal)
		ui.Icon(c, mark).FontSize(16).TextColor(ink).Shrink(0)
	})
	if tip != "" {
		tip += "\n"
	}
	status, _, _ := chat.taskState().mark(a.pal)
	title.Tooltip(tip + status)
}

func chatBadgeColor(preferred, surface ui.Color) ui.Color {
	color := preferred.Over(surface)
	if contrast(color, surface) >= 4.5 {
		return color
	}
	return legible(color, contrast(white, surface) > contrast(black, surface), surface)
}

func (a *app) chatTabActions(c *ui.Context, tab *colTab) {
	if tab == nil || tab.tool == nil || tab.tool.chat == nil {
		return
	}
	chat := tab.tool.chat
	ui.Row(c.Key(chat)).Bind(&tab.tool.chatActions).Shrink(0).Children(func() {
		if !a.window.Detached && a.iconButton(c.Key("new-chat"), iconPlus, locales.AgentNewSession(), false).Cursor(ui.CursorPointer).Clicked() {
			a.postChatAction(chat, func() { a.newChatBeside(chat) })
		}
		a.chatDetailsButton(c, tab)
	})
}

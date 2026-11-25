package main

import (
	"cmp"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type notificationReceipt struct {
	store  *settings.Settings
	id     uint64
	redraw func()
}

func rememberNotification(store *settings.Settings, source string, notice ui.Toast, redraw func()) notificationReceipt {
	if store == nil {
		return notificationReceipt{}
	}
	id := store.AddNotification(settings.Notification{
		Time: time.Now(), Source: truncate(cmp.Or(source, "FiveLeaf"), 128), Kind: notice.Type,
		Title: truncate(notice.Title, 512), Description: truncate(notice.Description, 2048),
	})
	if redraw != nil {
		redraw()
	}
	return notificationReceipt{store: store, id: id, redraw: redraw}
}

func (a *app) rememberNotice(source string, notice ui.Toast) notificationReceipt {
	var redraw func()
	if a.host != nil {
		redraw = a.host.redrawWindows
	}
	return rememberNotification(a.settings, source, notice, redraw)
}

func (receipt notificationReceipt) read() {
	if receipt.store != nil && receipt.store.RemoveNotification(receipt.id) && receipt.redraw != nil {
		receipt.redraw()
	}
}

func (a *app) notificationBell(c *ui.Context) {
	palette := a.pal
	metrics := palette.metricsOr()
	label := locales.NotificationHistory()
	button := a.iconButton(c.Key("status:notifications"), iconBell, label, a.noticeHistoryOpen).
		Height(statusBarH - 4).Shrink(0).Cursor(ui.CursorPointer)
	if len(a.settings.Notifications) > 0 {
		button.Children(func() {
			ui.Box(c.Key("unread")).Absolute().Top(1).Right(2).Size(6, 6).Radius(metrics.radiusFull).Background(palette.accent).PassThrough()
		})
	}
	if button.Clicked() {
		a.noticeHistoryOpen = !a.noticeHistoryOpen
	}
	button.Expanded(a.noticeHistoryOpen)
	if !a.noticeHistoryOpen {
		return
	}
	width, height := c.Size()
	ui.PopoverBase(c.Key("notification-history"), button, &a.noticeHistoryOpen, func(panel ui.Element) {
		a.floating(c, panel.Column().Width(min(400, max(0, width-2*metrics.inset))).Radius(metrics.radiusCard).Clip(), false).
			AttachTo(button, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, metrics.gap, 0).Role(ui.RoleDialog).Label(label)
		panel.Children(func() {
			ui.Row(c.Key("header")).FillWidth().AlignItems(ui.Center).Padding(metrics.pad, metrics.inset).
				BorderWidth(0, 0, 1, 0).BorderColor(palette.border).Children(func() {
				ui.Text(c, label).FontSize(metrics.textBody).FontWeight(metrics.weightBold).TextColor(palette.text)
				if len(a.settings.Notifications) == 0 {
					return
				}
				ui.Spacer(c)
				clear := ui.ButtonBase(c.Key("clear")).Height(metrics.control).Padding(0, metrics.pad).Radius(metrics.radius).
					Label(locales.NotificationClearHistory()).Tooltip(locales.NotificationClearHistory()).Cursor(ui.CursorPointer)
				if clear.Hovered() || clear.FocusVisible() {
					clear.Background(palette.hover)
				}
				clear.Children(func() { ui.Text(c, locales.LogClear()).FontSize(metrics.textSmall).TextColor(palette.muted) })
				if clear.Clicked() {
					a.settings.Notifications = nil
					a.settings.Changed()
					a.host.redrawWindows()
				}
			})
			if len(a.settings.Notifications) == 0 {
				ui.Text(c.Key("empty"), locales.NotificationEmpty()).FillWidth().Padding(2*metrics.inset, metrics.inset).
					FontSize(metrics.textBody).TextAlign(ui.Center).TextColor(palette.muted)
				return
			}
			ui.Scroll(c.Key("entries")).FillWidth().MaxHeight(max(0, min(380, height-a.statusHeight()-3*metrics.control))).Children(func() {
				for index := len(a.settings.Notifications) - 1; index >= 0; index-- {
					a.notificationHistoryRow(c, a.settings.Notifications[index])
				}
			})
			c.After(time.Minute)
		})
	})
}

func (a *app) notificationHistoryRow(c *ui.Context, entry settings.Notification) {
	palette := a.pal
	metrics := palette.metricsOr()
	row := ui.Row(c.Key(entry.ID)).FillWidth().AlignItems(ui.Center).Gap(metrics.gap).Padding(0, 0, 0, metrics.pad)
	row.Children(func() {
		remove := notificationRemoveButton(c, ui.ButtonBase(c.Key("remove")), palette, entry.Title)
		message := ui.ButtonBase(c.Key("message")).Grow(1).MinWidth(0).Padding(metrics.pad, metrics.inset, metrics.pad, 0).
			Gap(metrics.inset).AlignItems(ui.Center).Label(entry.Title).Description(entry.Description).Cursor(ui.CursorPointer)
		if message.Hovered() || message.FocusVisible() {
			row.Background(palette.hover)
		}
		message.Children(func() {
			icon, ink := notificationMark(entry.Kind, palette)
			ui.Icon(c, icon).FontSize(16).TextColor(legible(ink, palette.dark, c.Theme().Surface)).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(metrics.gap).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, entry.Source).FontSize(metrics.textSmall).TextColor(palette.muted).SingleLine().Shrink(1).MinWidth(0)
					ui.Spacer(c)
					ui.Text(c, ago(entry.Time, c.Now())).FontSize(metrics.textSmall).TextColor(palette.muted).Tooltip(dateTime(entry.Time.Local())).SingleLine().Shrink(0)
				})
				ui.Text(c, entry.Title).FontSize(metrics.textBody).FontWeight(metrics.weightMedium).TextColor(palette.text).MaxLines(3).Tooltip(entry.Title)
				if entry.Description != "" {
					ui.Text(c, entry.Description).FontSize(metrics.textSmall).TextColor(palette.muted).MaxLines(3).Tooltip(entry.Description)
				}
			})
		})
		if (remove || message.Clicked()) && a.settings.RemoveNotification(entry.ID) {
			a.host.redrawWindows()
		}
	})
}

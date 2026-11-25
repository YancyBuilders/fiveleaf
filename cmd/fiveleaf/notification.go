package main

import (
	"time"

	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"

	"fiveleaf/locales"
)

const actionNoticeTimeout = 8 * time.Second

var toastMotion = ui.ElementTransition{Duration: motionMove, Colors: true, Enter: &ui.Motion{Y: 12}, Exit: &ui.Motion{Y: 6}}

func notificationViewport(c *ui.Context, palette *palette, bottomInset float32, historyOpen bool) {
	metrics := palette.metricsOr()
	margin := 3 * metrics.pad
	width, _ := c.Size()
	ui.ToastViewportBase(c, func(viewport ui.Element, notices []ui.Toast) {
		viewport.Padding(0, margin, bottomInset+margin, margin).AlignItems(ui.End).Gap(2 * metrics.gap)
		for _, notice := range notices {
			receipt, _ := notice.Data.(notificationReceipt)
			if receipt.store != nil && receipt.store.NotificationIndex(receipt.id) < 0 {
				c.CloseToast(notice.ID)
			} else if !historyOpen {
				notificationCard(c, palette, notice, receipt, min(380, max(0, width-2*margin)))
			}
		}
	})
}

func notificationCard(c *ui.Context, palette *palette, notice ui.Toast, receipt notificationReceipt, width float32) {
	metrics := palette.metricsOr()
	surface := c.Theme().Surface
	parts := ui.ToastBase(c, notice)
	card := parts.Root.Row().Width(width).AlignItems(ui.Center).Gap(2*metrics.gap).
		Padding(metrics.inset).Radius(metrics.radiusCard).Background(surface).Border(1, palette.border)
	if material := materialOf(c); material.mica() {
		material.glassCard(card, palette, metrics.radiusCard+metrics.pad)
	} else {
		card.Shadow(0, 4, 16, 0, palette.shadow)
	}
	if !c.Preferences().ReduceMotion {
		card.Transition(toastMotion)
	}
	card.Children(func() {
		if notificationRemoveButton(c, parts.CloseButton(), palette, notice.Title) || card.Shortcut(0, ui.KeyEscape) {
			receipt.read()
		}
		icon, ink := notificationMark(notice.Type, palette)
		var mark ui.Element
		if notice.Type == "progress" && !c.Preferences().ReduceMotion {
			mark = ui.Spinner(c.Key("mark"))
		} else {
			mark = ui.Icon(c.Key("mark"), icon).FontSize(16)
		}
		mark.Size(16, 16).Shrink(0).TextColor(legible(ink, palette.dark, surface))
		message := ui.ButtonBase(c.Key("message")).Column().AlignItems(ui.Stretch).Grow(1).Shrink(1).MinWidth(0).Gap(metrics.gap).Label(notice.Title).Cursor(ui.CursorPointer)
		if message.Clicked() {
			receipt.read()
			c.CloseToast(notice.ID)
		}
		message.Children(func() {
			title := ui.Text(c, notice.Title).FontSize(metrics.textBody).TextColor(palette.text).MaxLines(3).Tooltip(notice.Title)
			if notice.Description != "" {
				title.FontWeight(metrics.weightBold)
				ui.Text(c, notice.Description).FontSize(metrics.textSmall).TextColor(legible(palette.muted, palette.dark, surface)).MaxLines(3).Tooltip(notice.Description)
			}
		})
		if notice.Action != "" {
			button := parts.ActionButton().Height(metrics.control).MaxWidth(width/3).Padding(0, metrics.pad).
				Radius(metrics.radius).Label(notice.Action).Tooltip(notice.Action).Cursor(ui.CursorPointer).Transition(hoverFade)
			if button.Clicked() {
				receipt.read()
			}
			if button.Hovered() || button.FocusVisible() {
				button.Background(palette.hover)
			}
			button.Children(func() {
				ui.Text(c, notice.Action).FontSize(metrics.textBody).FontWeight(metrics.weightMedium).
					TextColor(legible(palette.accent, palette.dark, surface)).Shrink(1).MinWidth(0).SingleLine()
			})
		}
	})
}

func notificationRemoveButton(c *ui.Context, button ui.Element, palette *palette, title string) bool {
	metrics := palette.metricsOr()
	label := locales.NotificationRemove(title)
	button.Size(metrics.control, metrics.control).Shrink(0).Radius(metrics.radiusFull).AlignItems(ui.Center).Justify(ui.Center).
		Label(label).Tooltip(label).Cursor(ui.CursorPointer).Transition(hoverFade)
	ink := palette.muted
	if button.Hovered() || button.FocusVisible() {
		button.Background(palette.hover)
		ink = palette.text
	}
	button.Children(func() {
		ui.Icon(c, uiicons.CircleX()).FontSize(15).TextColor(legible(ink, palette.dark, c.Theme().Surface))
	})
	return button.Clicked()
}

func notificationMark(kind string, palette *palette) (*ui.SVG, ui.Color) {
	switch kind {
	case "progress":
		return iconActivity, palette.accent
	case "approval":
		return iconMessageAlert, palette.modifiedFg
	case "success":
		return iconCircleCheck, palette.addedFg
	case "warning":
		return iconTriangleAlert, palette.modifiedFg
	case "error":
		return iconOctagonAlert, palette.dangerFg
	case "idle":
		return iconMinus, palette.muted
	case "undo":
		return iconUndo, palette.accent
	default:
		return iconInfo, palette.accent
	}
}

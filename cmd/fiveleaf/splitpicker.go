package main

import (
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

type splitPickerState struct {
	owner         *column
	trigger       uint64
	entered, left time.Time
	open          bool
	blocked       ui.Rect
	panel         ui.Rect
}

func (a *app) paneSplitButton(c *ui.Context, col *column) {
	label := a.withKey(locales.CmdViewSplitRight(), "view.splitRight")
	a.splitButton(c, col, label, col.tab().canSplit(), func(direction splitDirection) { a.splitEditor(col, direction) })
}

func (tab *colTab) canSplit() bool {
	return tab != nil && !tab.isTask() && (tab.tool == nil || tab.tool.meeting == nil && tab.tool.toolbox == nil)
}

func (a *app) splitButton(c *ui.Context, col *column, label string, enabled bool, split func(splitDirection)) {
	picker := &a.splitPicker
	button := a.iconButtonBase(c.Key("split-button"), iconColumns, label, false).KeepFocus().Disabled(!enabled)
	trigger := button.ID()
	button.Expanded(picker.trigger == trigger && picker.open)
	if a.blurred || a.overlaid() || !enabled {
		if picker.trigger == trigger {
			*picker = splitPickerState{}
		}
		return
	}
	choose := func(direction splitDirection) {
		*picker = splitPickerState{blocked: button.Bounds()}
		a.post(func() { split(direction) })
	}
	if button.Clicked() {
		choose(splitRight)
		return
	}
	if picker.blocked.W > 0 {
		px, py, inside := c.Root().PointerPosition()
		if inside && picker.blocked.Contains(px, py) {
			return
		}
		picker.blocked = ui.Rect{}
	}
	hovered := button.Hovered()
	if hovered && picker.trigger != trigger {
		*picker = splitPickerState{owner: col, trigger: trigger, entered: c.Now()}
	}
	if picker.trigger != trigger {
		return
	}
	if !picker.open {
		if !hovered || button.Pressed() {
			*picker = splitPickerState{}
			return
		}
		if wait := hoverDwell - c.Now().Sub(picker.entered); wait > 0 {
			c.After(wait)
			return
		}
		picker.open = true
	}
	px, py, inside := c.Root().PointerPosition()
	anchor := button.Bounds()
	bridge := ui.Rect{X: anchor.X, Y: anchor.Y + anchor.H, W: anchor.W, H: picker.panel.Y - anchor.Y - anchor.H}
	if picker.panel.Y < anchor.Y {
		bridge.Y = picker.panel.Y + picker.panel.H
		bridge.H = anchor.Y - bridge.Y
	}
	if hovered || inside && (picker.panel.Contains(px, py) || bridge.Contains(px, py)) {
		picker.left = time.Time{}
	} else {
		if picker.left.IsZero() {
			picker.left = c.Now()
		}
		if wait := hoverLeave - c.Now().Sub(picker.left); wait > 0 {
			c.After(wait)
		} else {
			*picker = splitPickerState{}
			return
		}
	}
	panel := ui.PopoverBase(c.Key("split-picker"), button, &picker.open, func(panel ui.Element) {
		width, _ := c.Size()
		panel.Column().Width(min(200, max(0, width-12))).Padding(5).Gap(2).Radius(8).
			AttachTo(button, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(4, 0, 0, 0).Label(locales.SplitEditor())
		a.floating(c, panel, false)
		panel.Opacity(panel.Keyframes("show", showIn(false)))
		panel.Children(func() {
			ui.Row(c).Gap(2).Children(func() {
				a.splitChoice(c, splitLeft, locales.CmdViewSplitLeft(), choose)
				a.splitChoice(c, splitRight, locales.CmdViewSplitRight(), choose)
			})
			ui.Row(c).Gap(2).Children(func() {
				a.splitChoice(c, splitUp, locales.CmdViewSplitUp(), choose)
				a.splitChoice(c, splitDown, locales.CmdViewSplitDown(), choose)
			})
		})
	})
	if !picker.open {
		*picker = splitPickerState{blocked: button.Bounds()}
		return
	}
	picker.panel = panel.Bounds()
}

func (a *app) splitChoice(c *ui.Context, direction splitDirection, label string, choose func(splitDirection)) {
	pal := a.pal
	button := ui.ButtonBase(c.Key(direction)).Column().Basis(0).Grow(1).MinWidth(0).Height(50).
		Center().Gap(3).Radius(4).Label(label).KeepFocus().Transition(hoverFade)
	ink := pal.muted
	if button.Hovered() || button.FocusVisible() {
		button.Background(pal.hover)
		ink = pal.text
	}
	button.Children(func() {
		preview := ui.Row(c).Size(28, 18).Padding(2).Gap(2).Radius(3).Border(1, ink).AlignItems(ui.Stretch)
		if direction.vertical() {
			preview.Column()
		}
		preview.Children(func() {
			for index := range 2 {
				fill := pal.muted.Alpha(0.25)
				if (index == 1) == direction.after() {
					fill = pal.accent
				}
				ui.Box(c).Grow(1).Basis(0).Radius(1).Background(fill)
			}
		})
		ui.Text(c, label).FontSize(pal.metricsOr().textSmall).TextColor(ink).SingleLine()
	})
	if button.Clicked() {
		choose(direction)
	}
}

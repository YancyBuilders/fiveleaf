package main

import (
	"slices"

	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

type agentMenuItem struct {
	key, label        string
	brand             *ui.SVG
	status            agentStatusMark
	checked, disabled bool
	choose            func()
}

type agentMenuOrigin struct {
	chat     *agentChat
	tree     *workspace.Tree
	root     string
	launcher bool
}

func (origin agentMenuOrigin) valid(a *app) bool {
	if a.loadsClosed || origin.tree != a.tree || origin.root != a.root {
		return false
	}
	if origin.launcher {
		return a.sessionsVisible() && a.agent == origin.chat
	}
	return a.chatShown(origin.chat)
}

type agentMenuState struct {
	agentMenuOrigin
	open, focus bool
}

func (a *app) agentMenuItems(origin agentMenuOrigin) []*agentMenuItem {
	chat := origin.chat
	providers := a.agentProviders()
	items := make([]*agentMenuItem, 0, len(providers))
	for _, provider := range providers {
		target := agentTarget{provider: provider.id}
		items = append(items, &agentMenuItem{
			key: provider.id, label: provider.name, brand: provider.chatMark(a.pal),
			status:  agentTargetStatus(a.chatTargetState(chat, target), provider.config.Disabled, a.pal),
			checked: chat.agentTarget == target, disabled: provider.config.Disabled,
			choose: func() { a.post(func() { a.chooseMenuAgent(origin, target.provider) }) },
		})
	}
	return items
}

func (a *app) chooseMenuAgent(origin agentMenuOrigin, id string) {
	if !origin.valid(a) || !slices.ContainsFunc(a.agentProviders(), func(provider agentProvider) bool {
		return provider.id == id && !provider.config.Disabled
	}) {
		return
	}
	a.selectChatAgent(origin.chat, id)
	if origin.launcher && (a.agent == origin.chat || !a.chatShown(a.agent)) {
		a.openAgentPage()
	}
}

func (a *app) agentMenuButton(c *ui.Context, button ui.Element, chat *agentChat, launcher bool) {
	menu := &a.agentMenu
	origin := agentMenuOrigin{chat: chat, tree: a.tree, root: a.root, launcher: launcher}
	showing := menu.open && menu.agentMenuOrigin == origin
	button.Role(ui.RoleMenuButton).Expanded(showing)
	if !button.IsDisabled() && (button.Clicked() || button.Shortcut(0, ui.KeyDown) || button.Shortcut(0, ui.KeyUp)) {
		*menu = agentMenuState{agentMenuOrigin: origin, open: !showing, focus: true}
		showing = menu.open
	}
	if !showing {
		return
	}
	a.agentMenuPanel(c, button, a.agentMenuItems(origin))
	if !menu.open {
		*menu = agentMenuState{}
	}
}

func (a *app) agentMenuPanel(c *ui.Context, anchor ui.Element, items []*agentMenuItem) {
	menu, pal := &a.agentMenu, a.pal
	width, height := c.Size()
	bounds := anchor.Bounds()
	limit := max(0, max(bounds.Y, height-bounds.Y-bounds.H)-22)
	panel := ui.PopoverBase(c.Key("agent-list-popover"), anchor, &menu.open, func(panel ui.Element) {
		a.floating(c, panel.Width(min(252, max(0, width-12))).Padding(4).Radius(pal.metricsOr().radiusCard), false).
			Role(ui.RoleMenu).Label(locales.AgentRoster()).FocusGroup(ui.Vertical)
		if menu.launcher {
			panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(4, 0, 0, 0)
		} else {
			panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorBottomEnd).Margin(0, 0, 4, 0)
		}
		focusKey := ""
		if menu.focus {
			for _, item := range items {
				if !item.disabled && (focusKey == "" || item.checked) {
					focusKey = item.key
				}
			}
		}
		panel.Children(func() {
			ui.Scroll(c.Key("items")).MaxHeight(limit).Children(func() {
				if len(items) == 0 {
					ui.Text(c, locales.PrefsAgentEmpty()).FontSize(pal.metricsOr().textBody).TextColor(pal.muted).Padding(4, 10)
				}
				for _, item := range items {
					row := a.agentMenuRow(c, item)
					if item.key == focusKey {
						row.Focus().ScrollIntoView()
						menu.focus = false
					}
					if !item.disabled && (row.Clicked() || row.Shortcut(0, ui.KeyEnter) || row.Shortcut(0, ui.KeySpace)) {
						menu.open = false
						item.choose()
					}
				}
			})
		})
		if panel.OverlayShortcut(0, ui.KeyEscape) {
			menu.open = false
			anchor.Focus()
		}
	})
	if panel.Dismissed() && !panel.PressedOutside() {
		anchor.Focus()
	}
}

func (a *app) agentMenuRow(c *ui.Context, item *agentMenuItem) ui.Element {
	pal, metrics := a.pal, a.pal.metricsOr()
	ink := pal.text
	if item.disabled {
		ink = pal.muted
	}
	row := ui.ButtonBase(c.Key(item.key)).Role(ui.RoleMenuItemRadio).Checked(item.checked).Disabled(item.disabled).
		Label(item.label+" · "+item.status.label).Tooltip(item.status.label).
		FillWidth().Height(28).Padding(0, 9, 0, 6).Radius(metrics.radius).Gap(7).AlignItems(ui.Center).Justify(ui.Start)
	if !item.disabled && (row.Hovered() || row.FocusVisible()) {
		row.Background(pal.hover)
	}
	row.Children(func() {
		ui.Box(c).Width(12).Shrink(0).Center().Children(func() {
			if item.checked {
				ui.Icon(c, uiicons.Check()).FontSize(12).TextColor(pal.accent)
			}
		})
		ui.Box(c).Size(17, 17).Shrink(0).Center().Children(func() { ui.Icon(c, item.brand).FontSize(16).TextColor(ink) })
		ui.Text(c, item.label).FontSize(metrics.textBody).TextColor(ink).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, item.status.icon).FontSize(15).TextColor(item.status.color).Shrink(0)
	})
	return row
}

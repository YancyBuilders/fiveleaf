package main

import (
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

type lspMenuState struct {
	open, subOpen   bool
	root, selected  string
	focus, subFocus bool
}

func (a *app) lspStateLook(state lspState) (string, ui.Color) {
	switch state {
	case lspStarting:
		return locales.LspStarting(), a.pal.modifiedFg
	case lspRunning:
		return locales.RunningStateRunning(), a.pal.addedFg
	case lspFailed:
		return locales.LspStateFailed(), a.pal.dangerFg
	default:
		return locales.LspStopped(), a.pal.muted
	}
}

func (a *app) lspStatus(c *ui.Context) {
	k := &a.lspMenu
	if k.open && k.root != a.root {
		*k = lspMenuState{}
	}
	servers := a.lspStatuses()
	// With no server in this workspace, the status bar leaves the menu
	// out: the command and the menu opened from it still show it.
	if len(servers) == 0 && !k.open {
		return
	}
	b := a.iconButton(c.Key("status:lsp"), iconLSP, locales.PluginsLanguageServers(), k.open).
		Height(statusBarH - 4).Shrink(0).Expanded(k.open)
	if b.Clicked() || b.Shortcut(0, ui.KeyDown) || b.Shortcut(0, ui.KeyUp) {
		*k = lspMenuState{open: !k.open, root: a.root, focus: true}
	}
	if k.open {
		a.lspStatusMenu(c, b, servers)
	}
}

func (a *app) lspStatusMenu(c *ui.Context, anchor ui.Element, servers []lspServerStatus) {
	k, p := &a.lspMenu, a.pal
	if !k.open {
		return
	}
	if k.subOpen && !slices.ContainsFunc(servers, func(s lspServerStatus) bool { return s.ID == k.selected }) {
		k.subOpen = false
	}
	m := p.metricsOr()
	width, height := c.Size()
	ui.PopoverBase(c.Key("lsp-menu"), anchor, &k.open, func(panel ui.Element) {
		dropIn(a.floating(c, panel.Width(min(300, width-16)).Radius(m.radiusCard).Padding(4).Gap(0), false), -4).
			Role(ui.RoleMenu).Label(locales.PluginsLanguageServers()).FocusGroup(ui.Vertical)
		panel.Children(func() {
			title := "FiveLeaf"
			if k.root != "" {
				title = filepath.Base(hostPath(k.root))
			}
			ui.Text(c, title).FontSize(m.textSmall).TextColor(p.muted).SingleLine().Padding(4, m.pad)
			if len(servers) == 0 {
				message := locales.LspNoServers()
				if k.root == "" {
					message = locales.LspNoWorkspace()
				}
				ui.Text(c, message).FontSize(m.textBody).TextColor(p.muted).Padding(m.pad).MaxWidthPercent(100)
			}
			ui.Scroll(c.Key("lsp-servers")).MaxHeight(max(48, min(300, height-160))).Children(func() {
				for i, server := range servers {
					state, color := a.lspStateLook(server.State)
					row := a.lspMenuButton(c, server.ID, server.ID, false).Gap(m.gap).Description(state)
					row.Expanded(k.subOpen && k.selected == server.ID)
					if k.focus && (k.selected == server.ID || k.selected == "" && i == 0) {
						row.Focus()
						k.focus = false
					}
					row.Children(func() {
						ui.Box(c).Size(7, 7).Radius(4).Background(color).Shrink(0)
						ui.Text(c, server.ID).FontSize(m.textBody).SingleLine().TextColor(p.text).Shrink(1).MinWidth(0)
						ui.Spacer(c)
						ui.Icon(c, iconChevronRight).FontSize(12).TextColor(p.muted).Shrink(0)
					})
					if row.Clicked() || row.Shortcut(0, ui.KeyRight) {
						k.selected, k.subOpen, k.subFocus = server.ID, true, true
					}
					if k.subOpen && k.selected == server.ID {
						a.lspServerActions(c, row, server)
					}
				}
			})
			ui.Box(c).Height(1).Background(p.border).Margin(4, 0)
			restart := a.lspMenuAction(c, "restart-all", locales.LspRestartAll(), len(servers) == 0)
			if restart.Clicked() {
				a.restartLSPFromMenu()
			}
			active := slices.ContainsFunc(servers, func(s lspServerStatus) bool { return s.State == lspStarting || s.State == lspRunning })
			if a.lspMenuAction(c, "stop-all", locales.LspStopAll(), !active).Clicked() {
				a.stopLanguageServers()
				*k = lspMenuState{}
			}
		})
	})
	if !k.open {
		*k = lspMenuState{}
	}
}

func (a *app) lspMenuButton(c *ui.Context, key, label string, disabled bool) ui.Element {
	p, m := a.pal, a.pal.metricsOr()
	b := ui.ButtonBase(c.Key(key)).Height(28).Padding(0, m.pad).Radius(m.radius).AlignItems(ui.Center).Justify(ui.Start).
		Role(ui.RoleMenuItem).Label(label).Disabled(disabled)
	if b.Hovered() || b.Focused() {
		b.Background(p.hover)
	}
	return b.Transition(hoverFade)
}

func (a *app) lspMenuAction(c *ui.Context, key, label string, disabled bool) ui.Element {
	b := a.lspMenuButton(c, key, label, disabled)
	color := a.pal.text
	if disabled {
		color = a.pal.faint
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(a.pal.metricsOr().textBody).TextColor(color).SingleLine()
	})
	return b
}

func (a *app) lspServerActions(c *ui.Context, anchor ui.Element, server lspServerStatus) {
	k, p := &a.lspMenu, a.pal
	m := p.metricsOr()
	width, height := c.Size()
	ui.PopoverBase(c.Key("lsp-actions:"+server.ID), anchor, &k.subOpen, func(panel ui.Element) {
		a.floating(c, panel.Width(min(260, width-16)).Radius(m.radiusCard).Padding(4).Gap(0), false).
			AttachTo(anchor, ui.AnchorEnd, ui.AnchorStart).MarginStart(4).
			Role(ui.RoleMenu).Label(server.ID).FocusGroup(ui.Vertical)
		panel.Children(func() {
			state, color := a.lspStateLook(server.State)
			ui.Text(c, state).FontSize(m.textSmall).TextColor(color).Padding(4, m.pad)
			if server.Err != "" {
				ui.Scroll(c.Key("lsp-error")).MaxHeight(max(48, min(200, height-160))).Children(func() {
					ui.Text(c, server.Err).FontSize(m.textSmall).TextColor(p.dangerFg).Padding(4, m.pad).MaxWidthPercent(100)
				})
			}
			restart := a.lspMenuAction(c, "restart", locales.LspRestartServer(), false)
			if k.subFocus {
				restart.Focus()
				k.subFocus = false
			}
			if restart.Clicked() {
				a.restartLSPFromMenu(server.ID)
			}
			if a.lspMenuAction(c, "stop", locales.LspStopServer(), server.State != lspStarting && server.State != lspRunning).Clicked() {
				a.stopLanguageServers(server.ID)
				*k = lspMenuState{}
			}
		})
		if panel.OverlayShortcut(0, ui.KeyLeft) {
			k.subOpen = false
		}
	})
	if !k.subOpen {
		k.focus = true
	}
}

func (a *app) restartLSPFromMenu(ids ...string) {
	if a.restartLanguageServers(ids...) {
		a.lspMenu = lspMenuState{}
	} else {
		a.fail("%s", locales.LspRestartUnavailable())
	}
}

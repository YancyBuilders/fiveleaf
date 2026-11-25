package main

import (
	"path/filepath"

	"fiveleaf/internal/git"
	"fiveleaf/internal/remote"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

const (
	chatDetailsWidth     = 336
	chatDetailsMaxHeight = 440
	chatSourcePreview    = 3
)

type chatDetailsState struct {
	inset            ui.Point
	open, allSources bool
}

type chatDetailsDrag struct {
	grab   ui.Point
	active bool
}

type chatDisplayedTotals struct {
	status         *git.Status
	added, removed int
}

func (a *app) hasChatProject() bool { return a.root != "" && !remote.Is(a.root) }

func (a *app) chatDetailsButton(c *ui.Context, tab *colTab) {
	if !a.hasChatProject() {
		return
	}
	chat := tab.tool.chat
	label := locales.AgentChatDetails()
	button := a.iconButtonBase(c.Key("chat-details"), iconMessageText, label, chat.details.open).Cursor(ui.CursorPointer)
	if tab == a.activeTab() {
		button.KeepFocus()
	}
	if button.Clicked() {
		chat.details.open = !chat.details.open
	}
	button.Expanded(chat.details.open)
	if chat.details.open {
		button.Background(a.pal.active).Tooltip(locales.AgentHideChatDetails())
	} else {
		button.Tooltip(label)
	}
}

func (a *app) chatDetailsCard(c *ui.Context, chat *agentChat, pane ui.Rect) {
	metrics := a.pal.metricsOr()
	width := min(chatDetailsWidth, max(0, pane.W-2*metrics.gap))
	height := min(chatDetailsMaxHeight, max(0, pane.H-2*metrics.gap))
	panel := ui.Column(c.Key("chat-details-card")).Absolute().Top(metrics.gap).Right(metrics.gap).Width(width).
		Role(ui.RoleGroup).Label(locales.AgentChatDetails()).Radius(metrics.radiusCard).Clip().Cache()
	limit := ui.Point{X: pane.W - 2*metrics.gap - width, Y: pane.H - 2*metrics.gap - panel.LayoutBounds().H}
	shown := clampChatDetailsInset(chat.details.inset, limit)
	a.floating(c, panel.Translate(-shown.X, shown.Y), false).Children(func() {
		headerHeight := min(metrics.control+2*metrics.pad, height)
		a.chatDetailsHeader(c, chat, headerHeight, shown, limit)
		ui.Scroll(c.Key("body")).FillWidth().MinWidth(0).MaxHeight(max(0, height-headerHeight-2)).Padding(metrics.inset).Children(func() {
			a.chatDetailsChanges(c, a.repository(), chat)
			a.chatExportRow(c, chat)
			a.chatDetailsSources(c, chat)
		})
	})
	if tab := a.activeTab(); tab != nil && tab.tool != nil && tab.tool.chat == chat && panel.OverlayShortcut(0, ui.KeyEscape) {
		chat.details.open = false
	}
	if !chat.details.open && panel.FocusWithin() {
		chat.input.Focus(c)
	}
}

func clampChatDetailsInset(inset, limit ui.Point) ui.Point {
	return ui.Point{X: clamp(inset.X, 0, limit.X), Y: clamp(inset.Y, 0, limit.Y)}
}

func (a *app) chatDetailsHeader(c *ui.Context, chat *agentChat, height float32, shown, limit ui.Point) {
	palette := a.pal
	metrics := palette.metricsOr()
	hint := locales.AgentChatMoveDetails()
	title := ui.Row(c.Key("header")).FillWidth().Height(height).Shrink(0).AlignItems(ui.Center).Gap(metrics.pad).
		Padding(0, metrics.control+2*metrics.pad, 0, metrics.pad).BorderWidth(0, 0, 1, 0).BorderColor(palette.border).KeepFocus().Label(hint)
	drag := ui.Local(title, "drag", func() chatDetailsDrag { return chatDetailsDrag{} })
	title.HandleInput(func(event ui.InputEvent) bool {
		return drag.input(event, shown, limit, &chat.details.inset)
	})
	if drag.active {
		title.Cursor(ui.CursorGrabbing)
	} else {
		title.Cursor(ui.CursorGrab).Tooltip(hint)
	}
	title.Children(func() {
		ui.Text(c, locales.AgentChatDetails()).FontSize(metrics.textBody).FontWeight(metrics.weightMedium).TextColor(palette.text).SingleLine().MinWidth(0)
		root := a.root
		if repo := a.repository().git.repo; repo != nil {
			root = repo.Root
		}
		ui.Text(c, filepath.Base(root)).FontSize(metrics.textSmall).TextColor(palette.muted).SingleLine().MinWidth(0).Shrink(1)
	})
	closeSize := min(metrics.control, height)
	closeButton := a.iconButton(c.Key("close"), iconX, locales.AgentHideChatDetails(), false).Absolute().Right(metrics.pad).
		Top((height-closeSize)/2).Size(closeSize, closeSize).KeepFocus().Cursor(ui.CursorPointer)
	if closeButton.Clicked() {
		chat.details.open = false
	}
}

func (drag *chatDetailsDrag) input(event ui.InputEvent, shown, limit ui.Point, inset *ui.Point) bool {
	if event.Button != 0 {
		return false
	}
	switch event.Kind {
	case ui.InputPointerDown:
		drag.grab = ui.Point{X: event.X, Y: event.Y}
		drag.active = true
	case ui.InputPointerMove, ui.InputPointerUp:
		if !drag.active {
			return false
		}
		dx, dy := event.X-drag.grab.X, event.Y-drag.grab.Y
		*inset = clampChatDetailsInset(ui.Point{X: shown.X - dx, Y: shown.Y + dy}, limit)
		drag.active = event.Kind != ui.InputPointerUp
	default:
		return false
	}
	return true
}

func (a *app) chatDetailsSources(c *ui.Context, chat *agentChat) {
	palette := a.pal
	metrics := palette.metricsOr()
	ui.Box(c).Height(1).Shrink(0).Margin(metrics.pad, metrics.pad).Background(palette.border)
	ui.Row(c.Key("sources-header")).Height(metrics.control).Padding(0, 0, 0, metrics.pad).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.AgentChatSources()).FontSize(metrics.textSmall).TextColor(palette.muted).SingleLine().Grow(1).MinWidth(0)
		if a.iconButton(c, iconPlus, locales.AgentChatAddSource(), false).KeepFocus().Cursor(ui.CursorPointer).Clicked() {
			a.postChatAction(chat, func() { a.chooseChatDetailFiles(chat) })
		}
	})
	sources := chat.detailSources()
	if len(sources) == 0 {
		ui.Text(c, locales.AgentChatSourcesEmpty()).Padding(metrics.pad).FontSize(metrics.textSmall).TextColor(palette.muted)
	}
	visible := len(sources)
	if !chat.details.allSources {
		visible = min(visible, chatSourcePreview)
	}
	for _, source := range sources[:visible] {
		a.chatDetailsSource(c.Key(source.path).Key(source.label), chat, source)
	}
	if len(sources) > chatSourcePreview {
		label, icon := locales.AgentChatAllSources(), iconList
		if chat.details.allSources {
			label, icon = locales.ShowLess(), iconCollapse
		}
		button := a.chatDetailRow(c.Key("all-sources"), label).Expanded(chat.details.allSources)
		button.Children(func() {
			ui.Icon(c, icon).FontSize(14).TextColor(palette.muted)
			ui.Text(c, label).FontSize(metrics.textSmall).TextColor(palette.muted).SingleLine().MinWidth(0)
		})
		if button.Clicked() {
			chat.details.allSources = !chat.details.allSources
		}
	}
}

func (a *app) chatDetailRow(c *ui.Context, label string) ui.Element {
	metrics := a.pal.metricsOr()
	row := ui.ButtonBase(c).FillWidth().MinWidth(0).Height(metrics.control+metrics.gap).Padding(0, metrics.pad).Gap(metrics.pad).
		Justify(ui.Start).AlignItems(ui.Center).Label(label).Radius(metrics.radius).Cursor(ui.CursorPointer).Transition(hoverFade)
	if row.Hovered() || row.FocusVisible() {
		row.Background(a.pal.hover)
	}
	return row
}

func (a *app) chatDetailsChanges(c *ui.Context, ctx *gitContext, chat *agentChat) {
	palette := a.pal
	metrics := palette.metricsOr()
	button := a.chatDetailRow(c.Key("changes"), locales.CmdAgentReviewChanges()).Disabled(ctx.git.repo == nil)
	var added, removed int
	var ready bool
	var err error
	status := locales.PluginViewLoading()
	if ctx.git.repo != nil {
		added, removed, ready, err = ctx.chatChangeTotals()
		button.Tooltip(locales.CmdAgentReviewChanges() + "\n" + shortenHome(ctx.git.repo.Root))
		last := ui.Local(button, "last-counts", func() chatDisplayedTotals { return chatDisplayedTotals{} })
		switch {
		case err != nil:
			*last = chatDisplayedTotals{}
			status = locales.AgentChatTotalsUnavailable()
			button.Tooltip(status + "\n" + err.Error())
		case ready:
			*last = chatDisplayedTotals{status: ctx.git.status, added: added, removed: removed}
		case last.status != nil && last.status == ctx.git.status:
			added, removed, ready = last.added, last.removed, true
		}
	} else if ctx.git.looked {
		status = locales.ScmNoRepo()
	}
	button.Children(func() {
		ui.Icon(c, iconDiff).FontSize(15).TextColor(palette.text).Shrink(0)
		ui.Text(c, locales.DiffChangesName()).FontSize(metrics.textBody).FontWeight(metrics.weightMedium).TextColor(palette.text).Grow(1).MinWidth(0).SingleLine()
		if ready && err == nil {
			format := i18n.NewNumberFormatter(c.Locale(), i18n.NumberOptions{Integer: true})
			ui.Text(c, "+"+format.Format(added)).FontSize(metrics.textSmall).FontFeatures("tnum").TextColor(palette.addedFg).SingleLine()
			ui.Text(c, "−"+format.Format(removed)).FontSize(metrics.textSmall).FontFeatures("tnum").TextColor(palette.removedFg).SingleLine()
		} else {
			ui.Text(c, status).FontSize(metrics.textSmall).TextColor(palette.muted).SingleLine().MinWidth(0)
		}
	})
	if button.Clicked() && ctx.git.repo != nil {
		root := ctx.git.repo.Root
		a.postChatAction(chat, func() {
			if !ctx.live() {
				return
			}
			if a.window.Detached {
				a.host.inWorkspace(func(owner *app) { owner.openChangesTab(true, root) })
			} else {
				ctx.openChangesTab(true)
			}
		})
	}
}

func (a *app) chatDetailsSource(c *ui.Context, chat *agentChat, source chatDetailSource) {
	palette := a.pal
	metrics := palette.metricsOr()
	name := source.label
	var row ui.Element
	if source.path != "" {
		row = a.chatDetailRow(c, locales.AgentChatOpenSource(name)).Tooltip(shortenHome(source.path))
	} else {
		row = ui.Row(c).FillWidth().MinWidth(0).Height(metrics.control+metrics.gap).Padding(0, metrics.pad).Gap(metrics.pad).
			AlignItems(ui.Center).Tooltip(name)
	}
	row.Children(func() {
		switch {
		case source.preview != nil:
			ui.Image(c, source.preview).Size(18, 18).Fit(ui.Contain).Radius(metrics.radius / 2).Shrink(0)
		case source.path != "":
			icon, ink := a.fileIcon(filepath.Base(source.path))
			iconThemeView(c, palette, icon, ink, 16)
		default:
			ui.Icon(c, iconPlug).FontSize(16).TextColor(palette.muted).Shrink(0)
		}
		ui.Text(c, name).FontSize(metrics.textBody).TextColor(palette.text).SingleLine().MinWidth(0).Shrink(1)
	})
	if source.path != "" && row.Clicked() {
		a.postChatAction(chat, func() { a.openFile(source.path, true) })
	}
}

func (a *app) chooseChatDetailFiles(chat *agentChat) {
	folder := a.folderKey()
	a.host.choosePaths(locales.AgentChatAddSource(), locales.DialogChoose(), a.root, false, func(paths []string) {
		if a.loadsClosed || folder != a.folderKey() || !a.chatShown(chat) {
			return
		}
		a.addChatDetailFiles(chat, paths)
	})
}

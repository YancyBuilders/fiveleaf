package main

import (
	"slices"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

func (a *app) agentComposer(c *ui.Context, r *agentChat) {
	p := a.pal
	m := p.metricsOr()
	available := a.agentTargetAvailable(r)
	material := materialOf(c)
	bottom := m.pad
	if material.mica() {
		bottom = 0
	}
	ui.Column(c.Key("agent-composer")).FillWidth().MaxWidth(agentConversationWidth).AlignSelf(ui.Center).
		Padding(m.gap, m.inset, bottom, m.inset).Gap(m.gap / 2).Shrink(0).Children(func() {
		box := a.chatComposeBox(c.Key("agent-compose-box")).Bind(&r.composeBox).Margin(m.row, 0, 0, 0)
		if item, over := a.dragOver(box); box.FileDragOver() || over && (item.file != nil || carriable(item.tab)) {
			box.BorderColor(p.accent)
		}
		paths := box.DroppedFiles()
		if item, dropped := ui.Drop[dragItem](box); dropped {
			switch {
			case item.file != nil:
				paths = item.file.paths()
			case carriable(item.tab):
				paths = []string{item.tab.path()}
			}
		}
		if len(paths) > 0 {
			files := slices.Clone(paths)
			a.postChatAction(r, func() { a.addChatDetailFiles(r, files) })
		}
		width := box.Bounds().W
		if width <= 0 {
			width = 300
		}
		narrow, wide := width < 280, width >= 620
		box.Children(func() {
			a.agentDraftFiles(c, r)
			input := a.chatDraftInput(c.Key("agent-input"), &r.draft, &r.input, locales.AgentPlaceholder()).
				OnPaste(func(text string) bool { return a.pasteChatAttachment(r, text) })
			previous := r.draftState
			if previous.id != 0 && previous.text == r.draft && (previous.id != input.ID() || previous.build+1 < a.builds) {
				input.SetTextSelection(previous.selection[0], previous.selection[1])
			}
			anchor, caret := input.TextAnchorCaret()
			r.draftState = agentDraftState{id: input.ID(), build: a.builds, text: r.draft, selection: [2]int{anchor, caret}}
			if r.focusNext {
				input.Focus()
				r.focusNext = false
			}
			if input.Focused() {
				a.setCurrentChat(r)
			}
			suggestionKey := a.chatSuggestions(c, r, input)
			canConnect := available && !r.requestInFlight() && r.continuationReady()
			ready := canConnect && r.contextLoading == 0 && r.hasDraft()
			root, tree, composing := a.root, a.tree, input.Composing()
			input.OnTextKey(func(key ui.TextKey) bool {
				if suggestionKey != nil && !composing && suggestionKey(key) {
					return true
				}
				if !chatSubmitKey(key) {
					return false
				}
				if composing || !canConnect || a.loadsClosed || a.root != root || a.tree != tree || !a.chatShown(r) {
					return true
				}
				text := key.Text()
				r.draft = text
				a.sendChat(r)
				if r.draft != text {
					key.SetText(r.draft)
				}
				return true
			})
			var modelButton ui.Element
			ui.Column(c.Key("agent-compose-footer")).FillWidth().MinWidth(0).Padding(0, m.inset, m.pad, m.inset).Gap(m.gap).Children(func() {
				if narrow {
					ui.Row(c.Key("model-line")).FillWidth().MinWidth(0).Gap(m.gap).AlignItems(ui.Center).Children(func() {
						a.agentAttachButton(c, r)
						modelButton = a.agentModelButton(c, r)
					})
				}
				toolbar := ui.Row(c.Key("agent-send-toolbar")).FillWidth().MinWidth(0).Gap(m.gap).AlignItems(ui.Center)
				if narrow {
					toolbar.Justify(ui.End).Gap(m.gap / 2)
				}
				toolbar.Children(func() {
					if !narrow {
						a.agentAttachButton(c, r)
						modelButton = a.agentModelButton(c, r)
					}
					if wide {
						ui.Row(c.Key("workspace-directory")).Grow(1).Shrink(1).MinWidth(0).Gap(m.gap).
							AlignItems(ui.Center).Label(locales.AgentWorkspaceDirectory()).Tooltip(a.root).Children(func() {
							ui.Icon(c, iconFolder).FontSize(13).TextColor(p.muted).Shrink(0)
							ui.Text(c, shortenHome(a.root)).Font(a.settings.FontFamily).FontSize(m.textSmall).
								TextColor(p.muted).SingleLine().Grow(1).MinWidth(0)
						})
					} else if !narrow {
						ui.Spacer(c).MinWidth(0)
					}
					a.agentTargetPicker(c, r, width >= 740)
					if len(agentGenericModes(r)) > 0 {
						a.agentModePicker(c, r, wide)
					}
					if canConnect && r.err != "" {
						a.agentReconnectButton(c.Key("reconnect"), r, wide)
					}
					if r.busy || r.connecting {
						stop := a.agentSendButton(c, iconStopSquare, locales.AgentStop(), false).Disabled(r.stopping)
						if stop.Clicked() {
							a.postChatAction(r, func() { a.stopChat(r) })
						}
					} else {
						send := a.agentSendButton(c, iconArrowUp, locales.AgentSend(), ready).Disabled(!ready).
							Tooltip(locales.AgentSend() + " (" + keyText("Enter") + ")")
						if send.Clicked() {
							a.postChatAction(r, func() { a.sendChat(r) })
						}
					}
				})
			})
			// Absolute children paint above the box; the track's two pixels
			// start on its top border without moving the keyed draft input.
			usage := a.agentContextUsage(c, r).Absolute().Top(1 - m.row).Left(0)
			if r.modelPicker.open {
				a.agentModelPopover(c, r, modelButton, usage)
			}
		})
	})
}

func (a *app) agentReconnectButton(c *ui.Context, chat *agentChat, showLabel bool) {
	label := locales.AgentReconnect()
	metrics := a.pal.metricsOr()
	var retry ui.Element
	if showLabel {
		retry = retryButton(c, label).Height(28).Padding(0, metrics.inset)
	} else {
		retry = a.iconButtonBase(c, iconRefresh, label, false).Size(28, 28).Border(1, a.pal.border)
	}
	if retry.Radius(metrics.radiusFull).Shrink(0).KeepFocus().Tooltip(label).Cursor(ui.CursorPointer).Clicked() {
		a.postChatAction(chat, func() { a.beginChat(chat, "") })
	}
}

func (a *app) agentAttachButton(c *ui.Context, chat *agentChat) {
	label := locales.AgentAttachFiles()
	button := a.iconButton(c.Key("attach"), uiicons.Paperclip(), label, false).Size(28, 28).Radius(a.pal.metricsOr().radiusFull).
		Shrink(0).KeepFocus().Cursor(ui.CursorPointer)
	if button.Clicked() {
		a.postChatAction(chat, func() { a.chooseChatDetailFiles(chat) })
	}
}

func (a *app) agentDraftFiles(c *ui.Context, chat *agentChat) {
	draft := &chat.sources.draft
	if !draft.hasFiles() {
		return
	}
	palette := a.pal
	metrics := palette.metricsOr()
	const chipWidth, chipHeight, iconSize = 112, 24, 14
	strip := ui.ScrollHorizontal(c.Key("draft-files")).FillWidth().MinWidth(0).Height(chipHeight + 2*metrics.gap).Shrink(0)
	state := ui.Local(strip, "preview", func() attachmentHover { return attachmentHover{} })
	var active, hovered, focused attachmentAnchor
	strip.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(metrics.gap).Padding(metrics.gap, metrics.inset).Children(func() {
			for index := range draft.refs {
				ref := &draft.refs[index]
				if ref.file == "" {
					continue
				}
				chip := ui.Row(c.Key(ref.file)).Size(chipWidth, chipHeight).Shrink(0).AlignItems(ui.Center).Gap(metrics.gap).
					Padding(0, 2, 0, metrics.pad).Radius(metrics.radius).Background(palette.surfaceHover).Clip().Focusable().Label(ref.label).Description(shortenHome(ref.file))
				chip.Children(func() {
					if ref.preview != nil {
						ui.Image(c, ref.preview).Size(iconSize, iconSize).Fit(ui.Contain).Radius(metrics.radius / 2).Shrink(0)
					} else {
						icon, ink := a.fileIcon(ref.label)
						iconThemeView(c, palette, icon, ink, iconSize)
					}
					ui.Text(c, ref.label).FontSize(metrics.textSmall).TextColor(palette.text).SingleLine().Grow(1).MinWidth(0).Shrink(1)
					label := locales.AgentChatRemoveAttachment(ref.label)
					remove := a.iconButton(c.Key("remove"), iconX, label, false).Size(18, 18).Shrink(0).KeepFocus()
					if remove.Clicked() {
						name := ref.file
						a.postChatAction(chat, func() { chat.removeDraftFile(name) })
					}
					anchor := attachmentAnchor{chip, ref}
					if chip.Hovered() && !remove.Hovered() {
						hovered = anchor
					}
					if chip.FocusVisible() {
						focused = anchor
					}
					if state.file == ref.file {
						active = anchor
					}
				})
			}
		})
	})
	a.attachmentPreview(c, state, active, hovered, focused)
}

func (a *app) agentSendButton(c *ui.Context, icon *ui.SVG, label string, ready bool) ui.Element {
	p, m := a.pal, a.pal.metricsOr()
	button := ui.ButtonBase(c.Key(label)).Size(32, 32).Shrink(0).Radius(m.radiusFull).
		Label(label).Tooltip(label).AlignItems(ui.Center).Justify(ui.Center).Transition(hoverFade)
	background, foreground := p.surfaceHover, p.muted
	if ready {
		background, foreground = p.fill, p.fillText
	} else if button.Hovered() {
		background, foreground = p.hover, p.text
	}
	button.Background(background).Children(func() {
		ui.Icon(c, icon).FontSize(16).TextColor(foreground)
	})
	return button
}

func (a *app) agentModePicker(c *ui.Context, r *agentChat, showName bool) {
	p, m := a.pal, a.pal.metricsOr()
	modes := agentGenericModes(r)
	current := agentGenericMode(r)
	name := locales.AgentMode()
	for _, choice := range modes {
		if choice.id == current {
			name = choice.name
		}
	}
	pick := ui.ButtonBase(c.Key("agent-mode")).Label(locales.AgentMode()).Tooltip(name).
		Height(28).Gap(m.gap).Radius(m.radius).AlignItems(ui.Center).Justify(ui.Center).
		Disabled(r.busy || r.connecting).Shrink(0)
	if showName {
		pick.MaxWidth(120).Padding(0, m.pad).Shrink(1).MinWidth(24)
	} else {
		pick.Width(24)
	}
	if pick.Hovered() {
		pick.Background(p.hover)
	}
	pick.Children(func() {
		ui.Icon(c, iconSparkles).FontSize(14).TextColor(p.muted).Shrink(0)
		if showName {
			ui.Text(c, name).FontSize(m.textSmall).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
		}
	})
	pick.Menu(func(menu *ui.Menu) {
		for _, choice := range modes {
			if menu.Item(choice.name).Checked(choice.id == current).Chosen() {
				id := choice.id
				client, session, generation, root := r.client, r.sessionID, r.generation, a.root
				a.post(func() {
					if a.chatShown(r) && a.root == root && r.client == client && r.sessionID == session && r.generation == generation {
						a.setChatGenericMode(r, id)
					}
				})
			}
		}
	})
}

func (a *app) agentTargetPicker(c *ui.Context, r *agentChat, showName bool) {
	p := a.pal
	m := p.metricsOr()
	providers := a.agentProviders()
	provider := findChatProvider(r.provider, providers)
	name := provider.name
	if name == "" {
		name = locales.AgentChoose()
	}
	if r.hasRole() {
		name = r.roleLabel()
	}
	pick := ui.ButtonBase(c.Key("agent-provider")).Label(locales.AgentChoose()).Tooltip(name).
		Height(28).Gap(m.gap).Radius(m.radius).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
		Disabled(len(providers) == 0)
	if showName {
		pick.MaxWidth(100).Padding(0, m.pad).Shrink(1).MinWidth(24)
	} else {
		pick.Width(24)
	}
	if pick.Hovered() {
		pick.Background(p.hover)
	}
	pick.Children(func() {
		ui.Icon(c, provider.chatMark(p)).FontSize(14).TextColor(p.muted).Shrink(0)
		if showName {
			ui.Text(c, name).FontSize(m.textSmall).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
		}
	})
	a.agentMenuButton(c, pick, r, false)
}

type agentStatusMark struct {
	label string
	icon  *ui.SVG
	color ui.Color
}

func agentTargetStatus(r *agentChat, disabled bool, p *palette) agentStatusMark {
	switch {
	case disabled:
		return agentStatusMark{locales.AgentDisabled(), uiicons.Ban(), p.muted}
	case r != nil && len(r.permissions) > 0:
		return agentStatusMark{locales.AgentPermission(), uiicons.LockKeyhole(), p.modifiedFg}
	case r != nil && r.connecting:
		return agentStatusMark{locales.AgentConnecting(), uiicons.LoaderCircle(), p.modifiedFg}
	case r != nil && r.busy:
		return agentStatusMark{locales.AgentWorking(), uiicons.Activity(), p.accent}
	case r != nil && r.sessionID != "":
		return agentStatusMark{locales.AgentOnline(), uiicons.CircleCheck(), p.addedFg}
	case r != nil && r.err != "":
		return agentStatusMark{locales.AgentStatusFailed(), uiicons.TriangleAlert(), p.dangerFg}
	default:
		return agentStatusMark{locales.AgentOffline(), uiicons.Link2Off(), p.muted}
	}
}

package main

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

const (
	agentConversationWidth = 850
	agentTranscriptInset   = 12
)

func (a *app) agentConversation(c *ui.Context, r *agentChat) {
	pane := ui.Column(c.Key(r)).Fill().MinWidth(0).MinHeight(0)
	pane.Children(func() {
		a.agentConversationBody(c, r, a.pal, pane.Bounds().H)
		if r.details.open && a.hasChatProject() {
			a.chatDetailsCard(c, r, pane.LayoutBounds())
		}
	})
}

func (a *app) agentConversationBody(c *ui.Context, r *agentChat, p *palette, height float32) {
	a.sessionContinuationBanner(c, r)
	micaChatViewport(c, &r.transcriptContent, p, &a.settings.Prefs, func() {
		switch {
		case a.root == "":
			a.noFolder(c)
		case remote.Is(a.root):
			ui.Text(c, locales.AgentRemoteUnavailable()).Bind(&r.transcriptContent).FillWidth().MaxWidth(agentConversationWidth).AlignSelf(ui.Center).
				FontSize(12).TextColor(p.muted).Padding(16, 12)
		default:
			a.agentTranscript(c, r, p)
		}
	})
	if a.hasChatProject() {
		m := p.metricsOr()
		a.agentLatestButton(c, r)
		if len(r.plan) > 0 {
			ui.Column(c.Key("agent-plan-slot")).FillWidth().MaxWidth(agentConversationWidth).AlignSelf(ui.Center).
				MinWidth(0).MinHeight(0).MaxHeightPercent(30).Padding(0, m.inset).Clip().Children(func() {
				a.agentPlanView(c, r, height >= 10*m.control)
			})
		}
		if len(r.permissions) > 0 {
			a.agentPermissionDock(c, r, p, height < 10*m.control)
		}
		a.agentComposer(c, r)
	}
}

func (a *app) agentTranscript(c *ui.Context, r *agentChat, p *palette) {
	// New chunks follow the end while it is visible; reading older
	// messages keeps the user's scroll position.
	if r.readingPrompt != nil && r.scroll.Y != min(r.readingY, r.scroll.MaxY) {
		r.readingPrompt = nil
	}
	if r.observeOutput() {
		r.keepLatestInView()
	}
	reading := !r.followingOutput()
	last := len(r.messages) - 1
	for last >= 0 && !agentMessageVisible(r.messages[last]) {
		last--
	}
	empty := last < 0 && len(r.tools) == 0 && len(r.plan) == 0 && len(r.permissions) == 0 &&
		len(r.authMethods) == 0 && r.err == "" && !r.busy && !r.connecting && r.contextLoading == 0
	needsSetup := !r.hasRole() && len(a.agentProviders()) == 0
	transcript := ui.Scroll(c.Key("agent-transcript")).TrackScroll(&r.scroll).Grow(1).MinHeight(0).Padding(agentTranscriptInset, 0)
	if empty && (needsSetup || a.agentTargetAvailable(r)) {
		transcript.Padding(0)
	}
	var turns []agentTurnAnchor
	transcript.Children(func() {
		if empty && needsSetup {
			a.agentSetupWelcome(c, transcript.Bounds().H, &r.transcriptContent)
			return
		}
		body := ui.Column(c).Bind(&r.transcriptContent).FillWidth().MaxWidth(agentConversationWidth).MinWidth(0).AlignSelf(ui.Center).
			Padding(0, agentTranscriptInset).Gap(14)
		bodyY := body.Bounds().Y
		body.Children(func() {
			if r.hasRole() && !a.agentTargetAvailable(r) {
				_, _, err := a.resolveAgentRole(r)
				if err != nil && r.err != err.Error() {
					ui.Text(c, err.Error()).FontSize(12).TextColor(p.dangerFg)
				}
				if actionButton(c.Key("configure-role"), locales.AgentRolesConfigure(), iconSettings).AlignSelf(ui.Start).Clicked() {
					a.post(func() { a.host.showSettings(settings.CatAgent, "agentRolePresets") })
				}
			} else if needsSetup {
				ui.Text(c, locales.AgentNoProviders()).FontSize(12).TextColor(p.muted)
				a.agentSetupActions(c, ui.Start)
			}
			shownTools := make(map[*agentTool]bool)
			codex := r.usesCodexThread()
			hasActivity := false
			var thought *agentMessage
			var turnStart int
			var hadReply bool
			reviewReasoning := func(turnEnd int) {
				if thought != nil && hadReply {
					a.agentReasoningLink(c.Key(struct{ reasoning *agentMessage }{thought}), r, thought, r.messages[turnStart:turnEnd])
				}
			}
			var tools []*agentTool
			for i := 0; i <= last; {
				msg := r.messages[i]
				if !agentMessageVisible(msg) {
					i++
					continue
				}
				if msg.role == "user" {
					reviewReasoning(i)
					thought, hadReply, turnStart = nil, false, i+1
				}
				if msg.tool != nil {
					hasActivity = hasActivity || codex
					tools = tools[:0]
					for ; i <= last; i++ {
						next := r.messages[i]
						if !agentMessageVisible(next) {
							continue
						}
						if next.tool == nil {
							break
						}
						if !shownTools[next.tool] {
							shownTools[next.tool] = true
							tools = append(tools, next.tool)
						}
					}
					a.agentToolRun(c, tools, p, r.busy && i > last, reading)
					continue
				}
				row := ui.Column(c.Key(msg)).FillWidth().MinWidth(0)
				finalLabel := false
				switch msg.role {
				case "user":
					hasActivity = false
					turns = append(turns, agentTurnAnchor{prompt: msg, offset: row.Bounds().Y - bodyY})
				case "commentary":
					hasActivity = true
				case "thought", "thinking":
					if thought == nil {
						thought = msg
					}
					hasActivity = hasActivity || codex
				case "assistant":
					if strings.TrimSpace(msg.text) != "" {
						hadReply = true
						finalLabel, hasActivity = hasActivity, false
						if len(turns) > 0 {
							turns[len(turns)-1].reply = msg
						}
					}
				}
				row.Children(func() {
					if finalLabel {
						metrics := p.metricsOr()
						ui.Text(c.Key("final-reply"), locales.AgentFinalReply()).FontSize(metrics.textSmall).
							FontWeight(metrics.weightMedium).TextColor(p.text).Margin(0, 0, metrics.gap, 0)
					}
					a.agentMessageView(c, r, msg, r.busy && i == last && msg.role != "user")
				})
				i++
			}
			if !r.busy {
				reviewReasoning(len(r.messages))
			}
			tools = tools[:0]
			for _, tool := range r.tools {
				if tool != nil && !shownTools[tool] {
					tools = append(tools, tool)
				}
			}
			a.agentToolRun(c, tools, p, r.busy, reading)
			a.agentRecovery(c, r, p)
			if r.connecting || r.contextLoading > 0 || r.busy && (len(r.permissions) == 0 || r.stopping) {
				ui.Row(c.Key("agent-progress")).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(12, 12)
					status := locales.AgentWorking()
					if r.connecting {
						status = locales.AgentConnecting()
					}
					if r.contextLoading > 0 {
						status = locales.AgentLoadingContext()
					}
					if r.stopping {
						status = locales.AgentStopping()
					}
					ui.Text(c, status).FontSize(12).TextColor(p.muted)
				})
			}
		})
		if len(turns) > 1 {
			margin := max(0, (transcript.Bounds().W-agentConversationWidth)/2)
			body.Padding(0, agentTranscriptInset, 0, max(agentTranscriptInset, agentTurnRailWidth-margin))
		}
	})
	if len(turns) > 1 {
		a.agentTurnRail(c, r, turns, transcript.Bounds())
	}
}

func (a *app) agentReasoningLink(c *ui.Context, chat *agentChat, first *agentMessage, messages []*agentMessage) {
	palette := a.pal
	metrics := palette.metricsOr()
	label := locales.AgentReviewReasoning()
	ink := agentThoughtColor(palette)
	button := ui.ButtonBase(c).AlignSelf(ui.Start).AlignItems(ui.Center).Gap(metrics.gap).Padding(metrics.gap).
		Label(label).Radius(metrics.radius).Cursor(ui.CursorPointer)
	if button.Hovered() || button.FocusVisible() {
		button.Background(palette.hover)
	}
	button.Children(func() {
		ui.Icon(c, uiicons.ChevronUp()).FontSize(metrics.textSmall).TextColor(ink)
		ui.Text(c, label).FontSize(metrics.textSmall).TextColor(ink)
	})
	if button.Clicked() {
		for _, message := range messages {
			if message != nil && (message.role == "thought" || message.role == "thinking") {
				message.expanded, message.expansionSet = true, true
			}
		}
		chat.readingPrompt, chat.focusThought = nil, first
		a.invalidate()
	}
}

func (a *app) agentRecovery(c *ui.Context, chat *agentChat, p *palette) {
	authenticate := chat.authRequired && len(chat.authMethods) > 0
	if chat.err == "" && !authenticate {
		return
	}
	ui.Column(c.Key("agent-recovery")).MinWidth(0).Gap(p.metricsOr().pad).Children(func() {
		if chat.err != "" {
			ui.Text(c.Key("agent-error"), chat.err).FontSize(12).TextColor(p.dangerFg).Selectable()
		}
		if chat.sessionInUse {
			compactActionRow(c.Key("session-in-use-actions"), ui.Start).Children(func() {
				if retryButton(c.Key("new-thread"), locales.AgentContinueNewThread()).Clicked() {
					a.postChatAction(chat, func() { a.continueChatInNewThread(chat) })
				}
			})
		}
		if !authenticate {
			return
		}
		compactActionRow(c.Key("actions"), ui.Start).Children(func() {
			for _, method := range chat.authMethods {
				label := locales.AgentAuthenticate() + ": " + method.name
				action := actionButton(c.Key("auth:"+method.id), label, uiicons.KeyRound())
				if action.Disabled(chat.connecting).Tooltip(label).Clicked() {
					a.postChatAction(chat, func() { a.authenticateChat(chat, method.id) })
				}
			}
		})
	})
}

func (a *app) agentSetupWelcome(c *ui.Context, height float32, reading *ui.Handle) {
	p := a.pal
	m := p.metricsOr()
	ui.Column(c.Key("agent-setup")).FillWidth().MinHeight(height).Center().Padding(2*m.inset, m.inset).Children(func() {
		ui.Column(c).Bind(reading).Width(420).MaxWidthPercent(100).AlignItems(ui.Center).Gap(2 * m.gap).Children(func() {
			ui.Box(c).Size(52, 52).Radius(m.radiusCard+m.pad).Center().Background(p.surface).Border(1, p.border).
				Margin(0, 0, m.pad, 0).Children(func() { ui.Icon(c, iconAgent).FontSize(28).TextColor(p.text) })
			ui.Text(c, locales.AgentSetupTitle()).FontSize(m.textTitle * 1.7).FontWeight(m.weightBold).
				TextColor(p.text).TextAlign(ui.Center).MaxWidthPercent(100)
			ui.Text(c, locales.AgentSetupDetail()).FontSize(m.textBody).LineHeight(1.45).
				TextColor(p.muted).TextAlign(ui.Center).MaxWidthPercent(100)
			a.agentSetupActions(c, ui.Center)
		})
	})
}

func (a *app) agentSetupActions(c *ui.Context, align ui.Align) {
	m := a.pal.metricsOr()
	compactActionRow(c.Key("agent-setup-actions"), align).Gap(2*m.gap).Margin(m.gap, 0, 0, 0).Children(func() {
		configure := accentActionButton(c.Key("configure"), locales.AgentConfigure(), uiicons.SlidersHorizontal(), a.pal.accent)
		if configure.MinHeight(44).Padding(0, m.inset+m.gap).FontSize(m.textBody).Clicked() {
			a.post(func() { a.host.showSettings(settings.CatAgent, "agentServers") })
		}
		manage := actionButton(c.Key("plugins"), locales.CmdPluginsManage(), uiicons.Puzzle())
		if manage.MinHeight(44).Padding(0, m.inset+m.gap).FontSize(m.textBody).Clicked() {
			a.post(func() { a.host.showSettings(settings.CatPlugins, "") })
		}
	})
}

func agentConversationTitle(r *agentChat) string {
	if r != nil {
		if title := r.transcriptTitle(); title != "" {
			return title
		}
		if title := shortChatTitle(chatQuestionLine(r.pendingPrompt)); title != "" {
			return title
		}
	}
	return locales.AgentThreadTitle()
}

func (chat *agentChat) transcriptTitle() string {
	if chat.continuation != nil && chat.continuation.source.Title != "" {
		return shortChatTitle(chat.continuation.source.Title)
	}
	for _, message := range chat.messages {
		if message != nil && message.role == "user" {
			if title := message.questionTitle(); title != "" {
				return title
			}
		}
	}
	return ""
}

func (message *agentMessage) questionTitle() string {
	if message.title == "" {
		message.title = shortChatTitle(chatQuestionLine(message.text))
	}
	return message.title
}

func chatQuestionLine(text string) string {
	var fallback, fence string
	for text != "" {
		line, rest, _ := strings.Cut(text, "\n")
		line, text = strings.TrimSpace(line), rest
		if line == "" {
			continue
		}
		if fence != "" {
			if fenceCloses(line, fence[0], len(fence)) {
				fence = ""
			}
			continue
		}
		if fenceOpens(line) {
			fence = line[:fenceRun(line)]
			continue
		}
		if fallback == "" {
			fallback = line
		}
		next, _, _ := strings.Cut(rest, "\n")
		if (strings.ContainsAny(line, `/\`) || !strings.ContainsAny(line, " \t")) && fenceOpens(strings.TrimSpace(next)) {
			continue
		}
		return line
	}
	return fallback
}

func shortChatTitle(question string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(question), "\n")
	var title []rune
	space := false
	for _, char := range line {
		if unicode.IsSpace(char) {
			space = len(title) > 0
			continue
		}
		if space {
			title = append(title, ' ')
			space = false
		}
		title = append(title, char)
		if len(title) > 36 {
			return strings.TrimSpace(string(title[:36])) + "…"
		}
		if strings.ContainsRune("。！？!?", char) {
			break
		}
	}
	return string(title)
}

func (a *app) agentPlanView(c *ui.Context, chat *agentChat, open bool) {
	if chat.planExpansionSet {
		open = chat.planExpanded
	}
	p, m := a.pal, a.pal.metricsOr()
	completed := 0
	for _, entry := range chat.plan {
		if entry.status == "completed" {
			completed++
		}
	}
	done, total := strconv.Itoa(completed), strconv.Itoa(len(chat.plan))
	progress := locales.AgentPlanProgress(done, total)
	label := locales.AgentPlan()
	ui.Column(c.Key("agent-plan")).FillWidth().MinWidth(0).MinHeight(0).Radius(m.radiusCard).
		Background(p.surface).Border(1, p.border).Clip().Children(func() {
		toggle := ui.ButtonBase(c.Key("toggle")).FillWidth().MinWidth(0).Height(m.control+m.gap).Shrink(0).
			Padding(0, m.inset).Gap(m.pad).AlignItems(ui.Center).Label(label).Description(progress).
			Tooltip(progress).Cursor(ui.CursorPointer).KeepFocus().Transition(hoverFade)
		if toggle.Hovered() || toggle.FocusVisible() {
			toggle.Background(p.hover)
		}
		if toggle.Clicked() {
			open = !open
			chat.planExpanded, chat.planExpansionSet = open, true
			chat.keepLatestInView()
		}
		toggle.Expanded(open).Children(func() {
			ui.Icon(c, uiicons.ListChecks()).FontSize(14).TextColor(p.muted).Shrink(0)
			ui.Text(c, label).FontSize(m.textSmall).FontWeight(m.weightBold).TextColor(p.text).SingleLine().Grow(1).MinWidth(0)
			ui.Text(c, done+"/"+total).FontSize(m.textSmall).FontFeatures("tnum").TextColor(p.muted).SingleLine().Shrink(0)
			chevron := iconChevronDown
			if !open {
				chevron = iconChevronRight
			}
			ui.Icon(c, chevron).FontSize(12).TextColor(p.muted).Shrink(0)
		})
		if !open {
			return
		}
		ui.Scroll(c.Key("entries")).FillWidth().MinWidth(0).MinHeight(0).MaxHeight(5*m.control).Focusable().Label(label).
			Padding(0, m.inset, m.gap, m.inset).Children(func() {
			for index, entry := range chat.plan {
				status := agentStatusLabel(entry.status)
				ui.Row(c.Key(index)).FillWidth().MinWidth(0).Padding(m.gap/2, 0).Gap(m.pad).
					AlignItems(ui.Start).Role(ui.RoleGroup).Description(status).Tooltip(status).Children(func() {
					icon, ink, text := uiicons.Circle(), p.muted, p.text
					switch entry.status {
					case "completed":
						icon, ink, text = iconCircleCheck, p.addedFg, p.muted
					case "in_progress":
						icon, ink = uiicons.CircleDot(), p.accent
					}
					ui.Icon(c, icon).FontSize(14).TextColor(ink).Margin(1, 0, 0, 0).Shrink(0)
					ui.Text(c, entry.content).FontSize(m.textBody).TextColor(text).Grow(1).MinWidth(0)
				})
			}
		})
	})
}

func agentStatusLabel(status string) string {
	switch status {
	case "in_progress", "running":
		return locales.AgentStatusRunning()
	case "completed":
		return locales.AgentStatusCompleted()
	case "failed":
		return locales.AgentStatusFailed()
	case "cancelled":
		return locales.AgentStatusCancelled()
	default:
		return locales.AgentStatusPending()
	}
}

// Bound rich text work while preserving complete protocol messages in memory.
func agentVisibleText(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit], true
}

package main

import (
	"math"
	"strings"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (s *agentChat) followingOutput() bool {
	return s.readingPrompt == nil && s.scroll.Y >= s.scroll.MaxY-2
}

func (s *agentChat) keepLatestInView() {
	if s.followingOutput() {
		s.scroll.Y = math.MaxFloat32
	}
}

// Reading older content and opening details are independent of new output.
func (s *agentChat) observeOutput() bool {
	changed := s.observedMessages != len(s.messages) || s.observedTools != len(s.tools) || s.observedBusy != s.busy || len(s.permissions) > s.observedPermissions
	s.observedMessages, s.observedTools = len(s.messages), len(s.tools)
	s.observedPermissions = len(s.permissions)
	s.observedBusy = s.busy
	for _, message := range s.messages {
		if message != nil && message.observedText != message.text {
			message.observedText = message.text
			changed = true
		}
	}
	for _, tool := range s.tools {
		if tool != nil && (tool.observedText != tool.text || tool.observedStatus != tool.status) {
			tool.observedText, tool.observedStatus = tool.text, tool.status
			changed = true
		}
	}
	return changed
}

func (a *app) agentMessageContent(c *ui.Context, message *agentMessage, streaming bool) {
	text, clipped := agentVisibleText(message.text, 32*1024)
	if message.render.Text != text {
		message.render = viewNode{Type: "markdown", Text: text}
	}
	sharedAgentMarkdown(c, "message", &message.render, a.pal, a.settings, streaming)
	if clipped {
		ui.Text(c, locales.AgentMoreOutput()).FontSize(a.pal.metricsOr().textSmall).TextColor(a.pal.muted)
	}
}

func (a *app) agentMessageView(c *ui.Context, chat *agentChat, message *agentMessage, streaming bool) {
	p, m := a.pal, a.pal.metricsOr()
	thought := message.role == "thought" || message.role == "thinking"
	if message.role == "system" {
		ui.Text(c, message.text).FontSize(m.textSmall).TextColor(p.muted).Selectable().
			Padding(m.gap, 0).BorderWidth(1, 0, 1, 0).BorderColor(p.border)
		return
	}
	content := func() { a.agentMessageContent(c, message, streaming) }
	if thought {
		label := locales.AgentThought()
		ui.Column(c.Key("body")).Gap(m.gap).FillWidth().MinWidth(0).Children(func() {
			open := streaming
			if message.expansionSet {
				open = message.expanded
			}
			button := ui.ButtonBase(c.Key("details-toggle")).Label(label).Expanded(open).
				FillWidth().MinWidth(0).Padding(m.gap, 0).Gap(m.gap).AlignItems(ui.Center).Radius(m.radius)
			if button.Hovered() {
				button.Background(p.hover)
			}
			button.Children(func() {
				icon := iconChevronRight
				if open {
					icon = iconChevronDown
				}
				ui.Icon(c, icon).FontSize(m.textSmall).TextColor(p.muted).Shrink(0)
				ui.Text(c, label).FontSize(m.textSmall).TextColor(p.muted).Shrink(0)
				if streaming {
					ui.Spinner(c).Size(12, 12).Shrink(0)
				} else if !open {
					line, _, _ := strings.Cut(strings.TrimSpace(message.text), "\n")
					ui.Text(c, line).FontSize(m.textSmall).TextColor(p.muted).SingleLine().Grow(1).MinWidth(0)
				}
			})
			if button.Clicked() {
				message.expanded, message.expansionSet = !open, true
			}
			if open {
				ui.Column(c.Key("message-content")).FillWidth().MinWidth(0).BorderWidth(0, 0, 0, 2).
					BorderColor(p.border).Padding(0, 0, 0, m.inset).Children(content)
			}
		})
		return
	}
	outgoing := message.role == "user"
	row := ui.Row(c.Key("body")).MinWidth(0).Gap(m.gap).AlignItems(ui.Start).Role(ui.RoleGroup)
	if outgoing {
		row.AlignSelf(ui.End).MaxWidthPercent(86).Reverse().Description(locales.AgentYou())
	} else {
		author := chat.roleLabel()
		if author == "" {
			author = locales.AgentTitle()
		}
		row.FillWidth().Description(author)
	}
	row.Children(func() {
		body := ui.Column(c.Key("message-content")).MinWidth(0)
		if outgoing {
			body.Padding(m.pad, m.inset).Radius(m.radiusField).Background(p.hover)
			if chat.readingPrompt == message {
				body.Background(p.hover.Mix(p.accent, 0.18))
			}
		} else {
			body.Grow(1)
		}
		body.Children(content)
		copyButton := a.iconButton(c, iconCopy, locales.AgentCopyMessage(), false).Shrink(0)
		if !row.Hovered() && !row.FocusWithin() {
			copyButton.Opacity(0)
		}
		if copyButton.Clicked() {
			c.WriteClipboard(message.text)
		}
	})
}

func (a *app) agentLatestButton(c *ui.Context, s *agentChat) {
	p := a.pal
	if s.followingOutput() {
		return
	}
	m := p.metricsOr()
	b := ui.ButtonBase(c.Key("agent-jump-latest")).Label(locales.AgentJumpLatest()).
		AlignSelf(ui.Center).Padding(m.gap, m.inset).Gap(m.gap).AlignItems(ui.Center).
		Radius(m.radius).Background(p.surface).Border(1, p.border)
	b.Children(func() {
		ui.Icon(c, iconChevronDown).FontSize(m.textSmall).TextColor(p.text)
		ui.Text(c, locales.AgentJumpLatest()).FontSize(m.textSmall).TextColor(p.text)
	})
	if b.Clicked() {
		s.readingPrompt = nil
		s.scroll.Y = math.MaxFloat32
	}
}

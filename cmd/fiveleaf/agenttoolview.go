package main

import (
	"path/filepath"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func agentToolExpanded(tool *agentTool, grouped, live bool) bool {
	if tool.expansionSet {
		return tool.expanded
	}
	if tool.status == "failed" {
		return true
	}
	return !grouped && live && (tool.status == "in_progress" || tool.status == "running")
}

func agentToolIcon(kind string) *ui.SVG {
	switch kind {
	case "read":
		return iconFileText
	case "search":
		return iconSearch
	case "execute", "terminal":
		return iconTerminal
	case "edit", "delete", "move":
		return iconPencil
	default:
		return iconFileCode
	}
}

type agentToolRunDisclosure struct {
	open, chosen bool
}

func agentToolRunStatus(tools []*agentTool) (status string, open bool) {
	for _, tool := range tools {
		open = open || tool.expansionSet && tool.expanded || len(tool.changes) > 0
		switch tool.status {
		case "failed":
			return "failed", true
		case "in_progress", "running":
			status = "in_progress"
		case "", "pending":
			if status == "" {
				status = "pending"
			}
		}
	}
	return status, open || status != ""
}

func (a *app) agentToolRun(c *ui.Context, tools []*agentTool, p *palette, liveTail, reading bool) {
	if len(tools) == 0 {
		return
	}
	metrics := p.metricsOr()
	group := ui.Column(c.Key(tools[0])).FillWidth().MinWidth(0).Gap(metrics.gap)
	group.Children(func() {
		grouped := len(tools) > 1
		if grouped {
			status, open := agentToolRunStatus(tools)
			disclosure := &tools[0].runDisclosure
			if disclosure.chosen {
				open = disclosure.open
			} else {
				open = open || liveTail || disclosure.open && (reading || group.FocusWithin())
				disclosure.open = open
			}
			if agentToolRunHeader(c, p, len(tools), status, open).Clicked() {
				disclosure.open, disclosure.chosen = !open, true
			}
			if !open {
				return
			}
		}
		items := ui.Column(c.Key("entries")).FillWidth().MinWidth(0).Gap(metrics.gap)
		if grouped {
			items.BorderWidth(0, 0, 0, 1).BorderColor(p.border).Padding(0, 0, 0, metrics.inset)
		}
		items.Children(func() {
			for _, tool := range tools {
				a.agentToolView(c, tool, p, grouped, liveTail && len(tools) == 1)
			}
		})
	})
}

func agentToolRunHeader(c *ui.Context, p *palette, tools int, status string, open bool) ui.Element {
	metrics := p.metricsOr()
	label, count := locales.AgentToolCalls(), locales.AgentToolCallsCount(tools)
	state, ink := "", p.muted
	if status != "" && !open {
		state = agentStatusLabel(status)
		if status == "failed" {
			ink = p.dangerFg
		}
	}
	button := ui.ButtonBase(c.Key("toggle")).AlignSelf(ui.Start).MaxWidthPercent(100).MinWidth(0).MinHeight(metrics.control).
		Label(label).Description(count).Expanded(open).Cursor(ui.CursorPointer).AlignItems(ui.Center).
		Gap(metrics.gap).Padding(metrics.gap).Radius(metrics.radius).Transition(hoverFade)
	if button.Hovered() {
		button.Background(p.hover)
	}
	button.Children(func() {
		ui.Icon(c, iconToolbox).FontSize(metrics.textBody).TextColor(p.muted).Shrink(0)
		ui.Text(c, label).FontSize(metrics.textSmall).FontWeight(metrics.weightMedium).TextColor(p.text).
			SingleLine().MinWidth(0).Shrink(1)
		if state != "" {
			ui.Text(c, state).FontSize(metrics.textSmall).TextColor(ink).SingleLine().MinWidth(0).Shrink(1)
		}
		ui.Text(c, count).FontSize(metrics.textSmall).TextColor(p.muted).FontFeatures("tnum").SingleLine().MinWidth(0).Shrink(1)
		chevron := iconChevronRight
		if open {
			chevron = iconChevronDown
		}
		ui.Icon(c, chevron).FontSize(metrics.textSmall).TextColor(p.muted).Shrink(0)
	})
	return button
}

func (a *app) agentToolView(c *ui.Context, tool *agentTool, p *palette, grouped, live bool) {
	if tool == nil {
		return
	}
	title := tool.title
	if title == "" {
		title = locales.AgentTools()
	}
	output := tool.outputText()
	open := agentToolExpanded(tool, grouped, live)
	ui.Column(c.Key(tool)).FillWidth().MinWidth(0).Gap(4).Children(func() {
		ui.Row(c.Key("header")).FillWidth().MinWidth(0).Gap(2).AlignItems(ui.Center).Children(func() {
			row := ui.ButtonBase(c.Key("toggle")).Grow(1).MinWidth(0).Padding(5, 4).Gap(5).Radius(4).
				AlignItems(ui.Center).Label(title).Tooltip(title).Expanded(open)
			if row.Hovered() {
				row.Background(p.hover)
			}
			row.Children(func() {
				ui.Icon(c, agentToolIcon(tool.kind)).FontSize(13).TextColor(p.muted).Shrink(0)
				ui.Text(c, title).FontSize(12).TextColor(p.text).SingleLine().Grow(1).MinWidth(0)
				color := p.muted
				if tool.status == "failed" {
					color = p.dangerFg
				}
				ui.Text(c, agentStatusLabel(tool.status)).FontSize(10.5).TextColor(color).SingleLine().Shrink(1).MinWidth(0)
				chevron := iconChevronRight
				if open {
					chevron = iconChevronDown
				}
				ui.Icon(c, chevron).FontSize(11).TextColor(p.muted).Shrink(0)
			})
			if row.Clicked() {
				tool.expanded, tool.expansionSet = !open, true
				a.invalidate()
			}
			if output != "" {
				label := locales.AgentCopyToolOutput() + ": " + title
				copy := ui.ButtonBase(c.Key("copy")).Size(24, 24).Radius(4).Shrink(0).
					Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(locales.AgentCopyToolOutput())
				if copy.Hovered() {
					copy.Background(p.hover)
				}
				copy.Children(func() { ui.Icon(c, iconCopy).FontSize(13).TextColor(p.muted) })
				if copy.Clicked() {
					c.WriteClipboard(output)
				}
			}
		})
		if open {
			input := a.agentToolInputSummary(tool)
			if tool.note != "" {
				ui.Text(c, tool.note).FontSize(11.5).TextColor(p.muted).Selectable()
			}
			if input != "" {
				ui.Text(c, locales.AgentToolInput()).FontSize(11).TextColor(p.muted)
				ui.ScrollBoth(c.Key("input")).FillWidth().MinWidth(0).MaxHeight(80).Children(func() {
					ui.Text(c, input).Font(a.settings.FontFamily).FontSize(11).TextColor(p.muted).NoWrap().Selectable()
				})
			}
			if output != "" {
				ui.Text(c, locales.AgentToolOutput()).FontSize(11).TextColor(p.muted)
				text, clipped := agentVisibleText(output, 8*1024)
				ui.ScrollBoth(c.Key("output")).FillWidth().MinWidth(0).MaxHeight(240).
					Label(locales.AgentTools() + ": " + title).Children(func() {
					ui.Text(c, text).Font(a.settings.FontFamily).FontSize(11.5).TextColor(p.muted).NoWrap().Selectable()
				})
				if clipped {
					ui.Text(c, locales.AgentMoreOutput()).FontSize(11).TextColor(p.muted)
				}
			}
			if input == "" && output == "" && tool.note == "" && len(tool.changes) == 0 {
				detail := locales.AgentToolNoDetails()
				switch tool.status {
				case "", "pending", "in_progress", "running":
					detail = locales.AgentToolWaitingDetails()
				}
				ui.Text(c, detail).FontSize(11.5).TextColor(p.muted).MinWidth(0)
			}
		}
		for i, change := range tool.changes {
			b := ui.ButtonBase(c.Key(i)).FillWidth().MinWidth(0).Padding(5, 6).Radius(4).Gap(5).AlignItems(ui.Center).
				Label(locales.AgentReviewChange() + ": " + a.relPath(change.path)).Tooltip(a.relPath(change.path))
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Children(func() {
				ui.Icon(c, iconFileCode).FontSize(12).TextColor(p.muted).Shrink(0)
				ui.Text(c, filepath.Base(change.path)).FontSize(12).TextColor(p.text).SingleLine().Grow(1).MinWidth(0)
				ui.Icon(c, iconArrowRight).FontSize(12).TextColor(p.muted).Shrink(0)
			})
			if b.Clicked() {
				a.post(func() { a.reviewAgentChange(change) })
			}
		}
	})
}

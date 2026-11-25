package main

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func agentToolExpanded(tool *agentTool) bool {
	if tool.expansionSet {
		return tool.expanded
	}
	if tool.status == "failed" {
		return true
	}
	switch tool.kind {
	case "execute", "terminal", "edit":
		return true
	}
	return false
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

func (a *app) agentToolInputSummary(tool *agentTool) string {
	// Large edit inputs can contain an entire file; only inspect small inputs
	// for useful command/path metadata, never render their arbitrary payload.
	if len(tool.rawInput) <= 8*1024 {
		var input map[string]json.RawMessage
		if json.Unmarshal(tool.rawInput, &input) == nil {
			for _, key := range []string{"command", "cmd", "path", "file_path", "filePath"} {
				var value string
				if json.Unmarshal(input[key], &value) == nil && strings.TrimSpace(value) != "" {
					if key == "path" || key == "file_path" || key == "filePath" {
						value = a.relPath(value)
					}
					text, _ := agentVisibleText(value, 1024)
					return text
				}
			}
		}
	}
	if len(tool.locations) > 0 {
		text, _ := agentVisibleText(a.relPath(tool.locations[0].Path), 1024)
		return text
	}
	return ""
}

func (a *app) agentToolView(c *ui.Context, tool *agentTool, p *palette) {
	if tool == nil {
		return
	}
	title := tool.title
	if title == "" {
		title = locales.AgentTools()
	}
	open := agentToolExpanded(tool)
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
			if tool.text != "" {
				label := locales.AgentCopyToolOutput() + ": " + title
				copy := ui.ButtonBase(c.Key("copy")).Size(24, 24).Radius(4).Shrink(0).
					Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(locales.AgentCopyToolOutput())
				if copy.Hovered() {
					copy.Background(p.hover)
				}
				copy.Children(func() { ui.Icon(c, iconCopy).FontSize(13).TextColor(p.muted) })
				if copy.Clicked() {
					c.WriteClipboard(tool.text)
				}
			}
		})
		if open {
			if tool.note != "" {
				ui.Text(c, tool.note).FontSize(11.5).TextColor(p.muted).Selectable()
			}
			if summary := a.agentToolInputSummary(tool); summary != "" && summary != title {
				ui.ScrollHorizontal(c.Key("input")).FillWidth().MinWidth(0).MaxHeight(64).Children(func() {
					ui.Text(c, summary).Font(a.settings.FontFamily).FontSize(11).TextColor(p.muted).NoWrap().Selectable()
				})
			}
			if tool.text != "" {
				text, clipped := agentVisibleText(tool.text, 8*1024)
				ui.ScrollBoth(c.Key("output")).FillWidth().MinWidth(0).MaxHeight(240).
					Label(locales.AgentTools() + ": " + title).Children(func() {
					ui.Text(c, text).Font(a.settings.FontFamily).FontSize(11.5).TextColor(p.muted).NoWrap().Selectable()
				})
				if clipped {
					ui.Text(c, locales.AgentMoreOutput()).FontSize(11).TextColor(p.muted)
				}
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

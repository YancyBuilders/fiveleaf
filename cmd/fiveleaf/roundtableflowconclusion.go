package main

import (
	"cmp"
	"strconv"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func meetingFlowTurnNodes(graph roundtable.Graph, message roundtable.Message) []roundtable.GraphNode {
	if message.Task == nil {
		return nil
	}
	end, found := meetingFlowNode(graph, message)
	if !found {
		return nil
	}
	if turnID := message.Task.TurnID; turnID != "" {
		var nodes []roundtable.GraphNode
		for _, node := range graph.Nodes[:end+1] {
			if node.TurnID == turnID {
				nodes = append(nodes, node)
			}
		}
		return nodes
	}
	start := end
	for start > 0 && graph.Nodes[start].Hop > 1 {
		previous := graph.Nodes[start-1]
		if previous.TurnID != "" || previous.Stage == roundtable.Conclusion || previous.Hop != graph.Nodes[start].Hop-1 {
			break
		}
		start--
	}
	return graph.Nodes[start : end+1]
}

func meetingEvidenceFinding(tool roundtable.ToolRecord) string {
	if tool.InputOnly || tool.Status != roundtable.Completed {
		return ""
	}
	if strings.TrimSpace(tool.Detail) != "" {
		return tool.Detail
	}
	if strings.TrimSpace(tool.Note) != "" {
		return tool.Note
	}
	return ""
}

func meetingFlowEvidence(message roundtable.Message, graph roundtable.Graph) []roundtable.ToolRecord {
	if message.Task == nil {
		return nil
	}
	var evidence []roundtable.ToolRecord
	collect := func(tools []roundtable.ToolRecord) {
		for _, tool := range tools {
			if meetingEvidenceFinding(tool) != "" {
				evidence = append(evidence, tool)
			}
		}
	}
	for _, node := range meetingFlowTurnNodes(graph, message) {
		if node.ID != message.ID {
			collect(node.Tools)
		}
	}
	collect(message.Task.Tools)
	return evidence
}

func (w *roundtableWindow) conclusionCard(c *ui.Context, message roundtable.Message, before []roundtable.Message, graph roundtable.Graph) {
	p := w.pal
	evidence := meetingFlowEvidence(message, graph)
	card := ui.Column(c.Key("conclusion")).FillWidth().MinWidth(0).Radius(9).Clip().Border(1, mix(p.border, p.accent, 0.27)).Background(materialOf(c).insetColor(p.editor))
	card.Children(func() {
		ui.Row(c.Key("conclusion-title")).FillWidth().MinWidth(0).Wrap().AlignItems(ui.Center).Gap(6).Padding(8, 11).Background(p.accent.Alpha(0.07)).Children(func() {
			ui.Box(c).Size(19, 19).Shrink(0).Center().Background(p.fill).Radius(5).Children(func() {
				ui.Icon(c, iconCheck).FontSize(13).TextColor(p.fillText)
			})
			ui.Text(c, locales.RoundtableConclusion()).MinWidth(0).Grow(1).FontSize(12).FontWeight(600).TextColor(p.accent)
			if message.Hop > 0 {
				ui.Text(c, locales.RoundtableRound(message.Hop)).FontSize(10).FontFeatures("tnum").TextColor(p.muted)
			}
		})
		if task := message.Task; task != nil && task.MemoryNotes > 0 {
			ui.Column(c.Key("memory-line")).FillWidth().MinWidth(0).Padding(0, 12, 6, 12).Children(func() {
				w.messageMemoryChip(c, message, locales.RoundtableMemoryCited(task.MemoryNotes))
			})
		}
		section := func(label string, content func()) {
			body := ui.Column(c.Key(label)).FillWidth().MinWidth(0).Gap(6).Padding(9, 12).BorderWidth(1, 0, 0, 0).BorderColor(p.border)
			body.Children(func() {
				ui.Row(c).FillWidth().MinWidth(0).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(2, 10).Radius(1).Shrink(0).Background(p.accent)
					ui.Text(c, label).MinWidth(0).FontSize(10.5).FontWeight(550).TextColor(p.muted)
				})
				content()
			})
		}
		section(locales.RoundtableDecision(), func() {
			ui.Text(c, locales.RoundtableNoMoreHandoffs()).FillWidth().MinWidth(0).FontSize(12).TextColor(p.text)
		})
		if len(evidence) > 0 {
			section(locales.RoundtableEvidence(), func() {
				for index, tool := range evidence {
					w.conclusionEvidenceRow(c, index, tool)
				}
				if len(evidence) >= 2 {
					ui.Text(c, locales.RoundtableCorroborated()).FillWidth().MinWidth(0).FontSize(10.5).TextColor(p.addedFg).Padding(5, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderStyle(ui.BorderDashed).BorderColor(p.border)
				}
			})
		}
		section(locales.RoundtableClosing(), func() {
			w.messageMarkdown(c, message.ID, message.Text)
			if w.meeting != nil && !w.meeting.readOnly && w.meeting.group.ID != "" && message.ID != "" {
				pending := ui.Local(card, "continuation", func() bool { return false })
				w.conclusionContinueButton(c, message, before, graph, pending)
			}
		})
	})
}

func (w *roundtableWindow) conclusionEvidenceRow(c *ui.Context, index int, tool roundtable.ToolRecord) {
	p := w.pal
	source := cmp.Or(tool.Title, tool.Kind, locales.AgentTools())
	detail, note := tool.Detail, tool.Note
	entry := ui.Column(c.Key("evidence").Key(index)).FillWidth().MinWidth(0).Radius(6).Border(1, p.border).Background(materialOf(c).insetColor(p.panel))
	expanded := ui.Local(entry, "expanded", func() bool { return false })
	entry.Children(func() {
		row := ui.ButtonBase(c.Key("source")).Label(source).Expanded(*expanded).FillWidth().MinWidth(0).AlignItems(ui.Start).Gap(7).Padding(6, 8).Cursor(ui.CursorPointer)
		if row.Clicked() {
			*expanded = !*expanded
			row.Expanded(*expanded)
		}
		if row.Hovered() || row.FocusVisible() {
			entry.BorderColor(p.accentDim).Background(mix(p.panel, p.accent, 0.05))
		}
		row.Children(func() {
			ui.Box(c).Size(17, 17).Radius(4).Shrink(0).Center().Border(1, p.border).Background(p.editor).Children(func() {
				ui.Text(c, strconv.Itoa(index+1)).FontSize(9).FontFeatures("tnum").TextColor(p.muted)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(5).AlignItems(ui.Center).Children(func() {
					meetingChip(c, source, p.accent, p.accent.Alpha(0.08))
					ui.Spacer(c).MinWidth(0)
					meetingChip(c, locales.RoundtableEvidenceHit(), p.addedFg, p.added.Alpha(0.08))
					chevron := ui.Icon(c, iconChevronDown).FontSize(11).TextColor(p.muted)
					if *expanded {
						chevron.Rotate(180)
					}
				})
				excerpt, _ := meetingExcerpt(meetingEvidenceFinding(tool))
				ui.Text(c, excerpt).FillWidth().MinWidth(0).MaxLines(2).FontSize(11.5).TextColor(p.text)
			})
		})
		if *expanded {
			ui.Column(c.Key("detail")).FillWidth().MinWidth(0).Gap(4).Padding(6, 8).Background(p.editor).BorderWidth(1, 0, 0, 0).BorderStyle(ui.BorderDashed).BorderColor(p.border).Children(func() {
				if strings.TrimSpace(detail) != "" {
					ui.Text(c, detail).FillWidth().MinWidth(0).FontSize(11.5).TextColor(p.text).Selectable()
				}
				if strings.TrimSpace(note) != "" && note != detail {
					ui.Text(c, note).FillWidth().MinWidth(0).FontSize(11.5).TextColor(p.muted).Selectable()
				}
			})
		}
	})
}

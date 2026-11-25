package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func meetingNodeType(graph roundtable.Graph, node roundtable.GraphNode) string {
	switch {
	case node.ID != "" && len(graph.Order) > 0 && node.ID == graph.Order[0]:
		return locales.RoundtableNodeEntry()
	case len(node.Tools) > 0:
		return locales.RoundtableNodeTool()
	case slices.ContainsFunc(graph.Edges, func(edge roundtable.GraphEdge) bool {
		return edge.From == node.ID && edge.Kind == roundtable.GraphHandoff
	}):
		return locales.RoundtableNodeHandoff()
	case node.MemoryNotes > 0:
		return locales.RoundtableNodeMemory()
	case node.Stage == roundtable.Conclusion:
		return locales.RoundtableNodeDecision()
	default:
		return locales.RoundtableNodeOutput()
	}
}

type nodeDrawerLink struct{ id, label string }

type nodeDrawerLinks struct {
	upstream, downstream []nodeDrawerLink
	edges                []string
}

func nodeDrawerConnections(graph roundtable.Graph, id string) (links nodeDrawerLinks) {
	if id == "" || len(graph.Edges) == 0 {
		return links
	}
	positions := make(map[string]int, len(graph.Order))
	for index, orderedID := range graph.Order {
		positions[orderedID] = index + 1
	}
	nodes := make(map[string]*roundtable.GraphNode, len(graph.Nodes))
	for index := range graph.Nodes {
		node := &graph.Nodes[index]
		nodes[node.ID] = node
	}
	add := func(nearby *[]nodeDrawerLink, node *roundtable.GraphNode) {
		label := meetingNodeTitle(*node)
		if position := positions[node.ID]; position > 0 {
			label = locales.RoundtableNodeNumber(position) + " · " + label
		}
		link := nodeDrawerLink{id: node.ID, label: label}
		if !slices.Contains(*nearby, link) {
			*nearby = append(*nearby, link)
		}
	}
	for _, edge := range graph.Edges {
		if edge.From != id && edge.To != id {
			continue
		}
		from, to := nodes[edge.From], nodes[edge.To]
		if from == nil || to == nil || from.ID == "" || to.ID == "" {
			continue
		}
		if edge.To == id {
			add(&links.upstream, from)
		}
		if edge.From == id {
			add(&links.downstream, to)
		}
		label := cmp.Or(from.Name, from.AgentID, from.ID) + " → " + cmp.Or(to.Name, to.AgentID, to.ID)
		kind := edge.Kind
		switch kind {
		case roundtable.GraphHandoff:
			kind = locales.RoundtableGraphHandoffLegend()
		case roundtable.GraphRead:
			kind = locales.RoundtableGraphReadLegend()
		}
		if kind != "" {
			label += " · " + kind
		}
		links.edges = append(links.edges, label)
	}
	return links
}

func nodeDrawerToolName(tool roundtable.ToolRecord) string {
	return cmp.Or(strings.TrimSpace(tool.Title), strings.TrimSpace(tool.Kind), tool.ID, "—")
}

func nodeDrawerCallLog(tools []roundtable.ToolRecord) string {
	var entries []string
	for _, tool := range tools {
		var lines []string
		for _, text := range []string{tool.Detail, tool.Note} {
			if strings.TrimSpace(text) != "" {
				lines = append(lines, text)
			}
		}
		if len(lines) > 0 {
			entries = append(entries, nodeDrawerToolName(tool)+"\n"+strings.Join(lines, "\n"))
		}
	}
	return strings.Join(entries, "\n\n")
}

func nodeDrawerStatusInk(p *palette, status string) ui.Color {
	switch status {
	case roundtable.Completed:
		return p.addedFg
	case roundtable.Running, "in_progress":
		return p.accent
	case roundtable.Failed:
		return p.dangerFg
	default:
		return p.muted
	}
}

func (w *roundtableWindow) nodeDrawerRow(c *ui.Context, label string, content func()) {
	p := w.pal
	ui.Row(c.Key(label)).FillWidth().MinWidth(0).AlignItems(ui.Start).Gap(8).Padding(4, 0).
		BorderWidth(0, 0, 1, 0).BorderColor(p.border).BorderStyle(ui.BorderDashed).Children(func() {
		ui.Text(c, label).WidthPercent(25).MaxWidth(88).Shrink(0).FontSize(11).TextColor(p.muted)
		ui.Column(c).Basis(0).Grow(1).MinWidth(0).AlignItems(ui.Start).Children(content)
	})
}

func (w *roundtableWindow) nodeDrawerInfo(c *ui.Context, graph roundtable.Graph, node roundtable.GraphNode, mono string) {
	p := w.pal
	field := func(label, value, font string) {
		w.nodeDrawerRow(c, label, func() {
			text := ui.Text(c, value).FillWidth().MinWidth(0).FontSize(11.5).TextColor(p.text).Selectable()
			if font != "" {
				text.Font(font).FontFeatures("tnum")
			}
		})
	}
	position := "—"
	if index := slices.Index(graph.Order, node.ID); index >= 0 {
		position = fmt.Sprintf("%d / %d", index+1, len(graph.Order))
	}
	field(locales.RoundtableNode(), position, mono)
	speaker := w.chatSpeaker(node.AgentID, node.Name)
	w.nodeDrawerRow(c, locales.RoundtableExecutor(), func() {
		ui.Row(c).FillWidth().MinWidth(0).Gap(5).AlignItems(ui.Center).Children(func() {
			meetingDot(c, p, speaker)
			ui.Text(c, cmp.Or(speaker.Name, node.Name, node.AgentID, "—")).Grow(1).MinWidth(0).FontSize(11.5).TextColor(p.text)
		})
	})
	field(locales.RoundtableType(), meetingNodeType(graph, node), "")
	w.nodeDrawerRow(c, locales.RoundtableStatus(), func() {
		label := cmp.Or(node.Status, "—")
		switch node.Status {
		case roundtable.Running, roundtable.Completed, roundtable.Failed, roundtable.Stopped:
			label = taskStatusLabel(node.Status)
		}
		ink := nodeDrawerStatusInk(p, node.Status)
		back := ink.Alpha(0.10).Over(p.editor)
		meetingChip(c, label, legible(ink, p.dark, back), back)
	})
	start, duration := "—", "—"
	if !node.Start.IsZero() {
		start = node.Start.Local().Format("15:04:05")
		if !node.End.IsZero() || node.Status == roundtable.Running {
			duration = meetingElapsed(node.Start, node.End)
		}
	}
	field(locales.RoundtableStartTime(), start, mono)
	field(locales.RoundtableGraphDuration(), duration, mono)
	field(locales.RoundtableModel(), cmp.Or(meetingSpeakerModel(speaker), "—"), mono)
	permission := locales.RoundtableWritable()
	if meetingReadOnlyTools(node.Tools) {
		permission = locales.RoundtableReadOnly()
	}
	field(locales.RoundtablePermissions(), permission, "")
}

func (w *roundtableWindow) nodeDrawerNeighbors(c *ui.Context, label, empty string, links []nodeDrawerLink) {
	p := w.pal
	w.nodeDrawerRow(c, label, func() {
		if len(links) == 0 {
			ui.Text(c, empty).FillWidth().MinWidth(0).FontSize(11.5).TextColor(p.muted)
		}
		for _, link := range links {
			button := ui.ButtonBase(c.Key(link.id)).FillWidth().MinWidth(0).Padding(3, 2).Radius(3).
				Label(link.label).Cursor(ui.CursorPointer)
			if button.Hovered() || button.FocusVisible() {
				button.Background(p.hover)
			}
			button.Children(func() {
				ui.Text(c, link.label).FillWidth().MinWidth(0).FontSize(11.5).TextColor(p.accent)
			})
			if button.Clicked() {
				w.openNodeDrawer(link.id)
			}
		}
	})
}

func (w *roundtableWindow) nodeDrawerTimeline(c *ui.Context, node roundtable.GraphNode) {
	p := w.pal
	step := func(index int, name, status, label string, connects bool) {
		ink := nodeDrawerStatusInk(p, status)
		ui.Row(c.Key(index)).FillWidth().MinWidth(0).Gap(7).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(9).Shrink(0).Padding(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				dot := ui.Box(c).Size(8, 8).Shrink(0).Radius(4).Border(1, ink)
				if status == roundtable.Completed || status == roundtable.Failed || status == roundtable.Running || status == "in_progress" {
					dot.Background(ink)
				}
				if connects {
					ui.Box(c).Width(1).Grow(1).Background(p.border)
				}
			})
			ui.Text(c, name).Basis(0).Grow(1).MinWidth(0).Padding(0, 0, 7, 0).FontSize(11.5).FontWeight(550).TextColor(p.text)
			if label != "" {
				ui.Text(c, label).MaxWidthPercent(40).MinWidth(0).FontSize(10.5).TextColor(ink)
			}
		})
	}
	if len(node.Tools) == 0 {
		label := locales.RoundtableOutputGenerated()
		switch node.Status {
		case roundtable.Running, roundtable.Failed, roundtable.Stopped:
			label = taskStatusLabel(node.Status)
		}
		step(0, label, node.Status, "", false)
	}
	for index, tool := range node.Tools {
		label := tool.Status
		switch tool.Status {
		case "pending", "in_progress", roundtable.Running, roundtable.Completed, roundtable.Failed, "cancelled":
			label = agentStatusLabel(tool.Status)
		}
		step(index, nodeDrawerToolName(tool), tool.Status, label, index+1 < len(node.Tools))
	}
}

func (w *roundtableWindow) nodeDrawerBody(c *ui.Context, graph roundtable.Graph, node roundtable.GraphNode) {
	p := w.pal
	mono := "monospace"
	if w.a != nil && w.a.settings != nil {
		mono = cmp.Or(w.a.settings.FontFamily, mono)
	}
	heading := func(title string) {
		ui.Row(c.Key(title)).FillWidth().MinWidth(0).Gap(6).Margin(10, 0, 4, 0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).MinWidth(0).FontSize(11).FontWeight(600).TextColor(p.muted).Role(ui.RoleHeading).Level(2)
			ui.Box(c).Height(1).Grow(1).Background(p.border)
		})
	}
	quote := func(text string, tint ui.Color) {
		back := tint.Alpha(0.09).Over(p.editor)
		ui.Text(c, text).FillWidth().MinWidth(0).Padding(7, 9).BorderWidth(0, 0, 0, 2).BorderColor(tint.Alpha(0.5)).
			Radius(0, 6, 6, 0).Background(back).FontSize(12).LineHeight(1.5).TextColor(legible(p.text, p.dark, back)).Selectable()
	}
	ui.Column(c.Key("node-body:" + node.ID)).FillWidth().MinWidth(0).Shrink(0).Children(func() {
		heading(locales.RoundtableBasicInfo())
		w.nodeDrawerInfo(c, graph, node, mono)

		heading(locales.RoundtableLinks())
		links := nodeDrawerConnections(graph, node.ID)
		w.nodeDrawerNeighbors(c, locales.RoundtableUpstream(), locales.RoundtableStartNode(), links.upstream)
		if len(links.edges) > 0 {
			w.nodeDrawerRow(c, locales.RoundtableEdges(), func() {
				ui.Text(c, strings.Join(links.edges, "\n")).FillWidth().MinWidth(0).Font(mono).FontSize(10.5).TextColor(p.muted)
			})
		}
		w.nodeDrawerNeighbors(c, locales.RoundtableDownstream(), locales.RoundtableEndNode(), links.downstream)

		heading(locales.RoundtableInput())
		input := node.Title
		if strings.TrimSpace(input) == "" {
			input = locales.RoundtableInheritedContext()
		}
		quote(input, p.changed)
		if len(node.Tools) > 0 {
			tool := node.Tools[0]
			signature := cmp.Or(strings.TrimSpace(tool.Title), tool.ID, strings.TrimSpace(tool.Kind), "—")
			w.nodeDrawerRow(c, locales.RoundtableTaskTools(), func() {
				ui.Text(c, signature).FillWidth().MinWidth(0).Font(mono).FontSize(11).TextColor(p.accent).Selectable()
			})
		}

		heading(locales.RoundtableTimeline())
		w.nodeDrawerTimeline(c, node)
		if log := nodeDrawerCallLog(node.Tools); log != "" {
			heading(locales.RoundtableCallLog())
			back, ink := p.text, p.editor
			if p.dark {
				back, ink = p.panel, p.text
			}
			ui.Text(c, log).FillWidth().MinWidth(0).Padding(8, 9).Radius(6).Background(back).
				Font(mono).FontSize(11).LineHeight(1.5).TextColor(legible(ink, true, back)).Selectable()
		}

		heading(locales.RoundtableTaskOutput())
		output := node.Text
		if strings.TrimSpace(output) == "" {
			output = "—"
		}
		quote(output, p.added)
	})
}

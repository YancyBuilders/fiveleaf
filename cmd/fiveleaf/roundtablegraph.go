package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingGraphLayout struct {
	speakers []meetingSpeaker
	lanes    map[string][]roundtable.GraphNode
	position map[string]int
	pairs    map[[2]string][]string
	incoming map[string]bool
}

func meetingGraphRows(graph roundtable.Graph, roster []meetingSpeaker) meetingGraphLayout {
	layout := meetingGraphLayout{speakers: slices.Clone(roster), lanes: make(map[string][]roundtable.GraphNode),
		position: make(map[string]int, len(graph.Order)), pairs: make(map[[2]string][]string), incoming: make(map[string]bool)}
	known := make(map[string]bool, len(roster))
	for _, speaker := range roster {
		known[speaker.ID] = true
	}
	byID := make(map[string]roundtable.GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		byID[node.ID] = node
	}
	for index, id := range graph.Order {
		node := byID[id]
		layout.position[id] = index + 1
		if !known[node.AgentID] {
			known[node.AgentID] = true
			layout.speakers = append(layout.speakers, meetingSpeaker{Participant: roundtable.Participant{ID: node.AgentID, Name: cmp.Or(node.Name, node.AgentID)}})
		}
		layout.lanes[node.AgentID] = append(layout.lanes[node.AgentID], node)
	}
	for _, edge := range graph.Edges {
		if edge.Kind == roundtable.GraphHandoff {
			pair := [2]string{byID[edge.From].AgentID, byID[edge.To].AgentID}
			layout.pairs[pair] = append(layout.pairs[pair], edge.To)
			layout.incoming[edge.To] = true
		}
	}
	return layout
}

func (w *roundtableWindow) graphView(c *ui.Context, graph roundtable.Graph, paneWidth float32) {
	const inset = 16
	width := max(0, paneWidth-2*inset)
	ui.Scroll(c.Key("roundtable-graph")).TrackScroll(&w.graphScroll).FillWidth().MinWidth(0).Grow(1).MinHeight(0).
		Background(materialOf(c).canvasColor(meetingCanvas(w.pal))).Padding(14, inset, 18, inset).Children(func() {
		body := ui.Column(c.Key("graph-content")).FillWidth().MinWidth(0).Gap(12)
		body.Children(func() {
			duration := meetingGraphDuration(graph)
			layout := meetingGraphRows(graph, w.meeting.speakers)
			w.graphLegend(c, layout.speakers, duration)
			w.graphStats(c, graph, duration, width)
			if len(graph.Nodes) == 0 {
				ui.Text(c.Key("graph-empty"), locales.RoundtableGraphNoNodes()).FillWidth().Padding(18, 12).FontSize(12).TextColor(w.pal.muted).TextAlign(ui.Center)
			}
			if len(layout.speakers) == 0 {
				return
			}
			w.graphLanes(c, layout, width)
			if len(graph.Nodes) > 0 {
				w.graphMatrix(c, layout, width)
			}
		})
	})
}

func (w *roundtableWindow) graphLegend(c *ui.Context, speakers []meetingSpeaker, duration string) {
	p := w.pal
	ui.Row(c.Key("graph-legend")).FillWidth().MinWidth(0).Wrap().AlignItems(ui.Center).Gap(9).Padding(7, 10).
		Border(1, p.border).Radius(7).Background(materialOf(c).insetColor(p.editor)).Children(func() {
		for _, dashed := range []bool{false, true} {
			label, ink := locales.RoundtableGraphHandoffLegend(), p.accent
			if dashed {
				label, ink = locales.RoundtableGraphReadLegend(), p.muted
			}
			ui.Row(c.Key(dashed)).AlignItems(ui.Center).Gap(4).Children(func() {
				swatch := ui.Box(c).Size(16, 1).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(ink)
				if dashed {
					swatch.BorderStyle(ui.BorderDashed)
				}
				ui.Text(c, label).FontSize(10).TextColor(p.muted)
			})
		}
		for _, speaker := range speakers[:min(3, len(speakers))] {
			ui.Row(c.Key("legend:" + speaker.ID)).MaxWidth(126).AlignItems(ui.Center).Gap(4).Tooltip(speaker.Name).Children(func() {
				meetingDot(c, p, speaker)
				ui.Text(c, speaker.Name).MinWidth(0).FontSize(10).TextColor(p.muted)
			})
		}
		ui.Spacer(c).MinWidth(0)
		ui.Row(c.Key("legend-elapsed")).AlignItems(ui.Center).Gap(4).Tooltip(locales.RoundtableGraphDuration()).Children(func() {
			ui.Icon(c, iconClock).FontSize(11).TextColor(p.muted)
			ui.Text(c, duration).FontSize(10).FontFeatures("tnum").TextColor(p.muted)
		})
	})
}

func (w *roundtableWindow) graphStats(c *ui.Context, graph roundtable.Graph, duration string, width float32) {
	p := w.pal
	stats := []struct {
		label, value string
		ink          ui.Color
	}{
		{locales.RoundtableGraphNodes(), strconv.Itoa(len(graph.Nodes)), p.accent},
		{locales.RoundtableGraphHandovers(), strconv.Itoa(graph.Handoffs()), p.addedFg},
		{locales.RoundtableGraphMemoryWrites(), strconv.Itoa(graph.MemoryWrites()), p.modifiedFg},
		{locales.RoundtableGraphDuration(), duration, p.text},
	}
	columns := len(stats)
	if windowWidth, _ := c.Size(); windowWidth <= 860 || width < 440 {
		columns = 2
	}
	ui.Column(c.Key("graph-stats")).FillWidth().MinWidth(0).Gap(5).Children(func() {
		for row := range slices.Chunk(stats, columns) {
			ui.Row(c.Key(row[0].label)).FillWidth().Gap(5).Children(func() {
				for _, stat := range row {
					card := ui.Column(c.Key(stat.label)).Basis(0).Grow(1).MinWidth(0).Gap(3).Padding(8, 7).Radius(7).Border(1, p.border).Background(materialOf(c).insetColor(p.editor))
					if stat.label == locales.RoundtableGraphMemoryWrites() {
						card.Tooltip(locales.RoundtableGraphMemoryScope())
					}
					card.Children(func() {
						ui.Text(c, stat.value).MinWidth(0).FontSize(16).FontWeight(600).FontFeatures("tnum").TextColor(legible(stat.ink, p.dark, p.editor))
						ui.Text(c, stat.label).MinWidth(0).FontSize(8.5).TextColor(p.muted)
					})
				}
			})
		}
	})
}

func (w *roundtableWindow) graphLanes(c *ui.Context, layout meetingGraphLayout, width float32) {
	p := w.pal
	labelWidth := min(float32(132), width*0.4)
	ui.Column(c.Key("graph-lanes")).FillWidth().MinWidth(0).Radius(8).Border(1, p.border).Clip().Background(materialOf(c).insetColor(p.editor)).Children(func() {
		for index, speaker := range layout.speakers {
			nodes := layout.lanes[speaker.ID]
			lane := ui.Row(c.Key("graph-lane:" + speaker.ID)).FillWidth().MinWidth(0).MinHeight(58).AlignItems(ui.Stretch)
			if index > 0 {
				lane.BorderWidth(1, 0, 0, 0).BorderColor(p.border)
			}
			lane.Children(func() {
				ui.Column(c.Key("identity")).Width(labelWidth).Shrink(0).Padding(7, 8).Gap(2).Justify(ui.Center).
					BorderWidth(0, 1, 0, 0).BorderColor(p.border).Background(materialOf(c).insetColor(mix(p.editor, p.panel, 0.45))).Children(func() {
					ui.Row(c).FillWidth().MinWidth(0).Gap(5).AlignItems(ui.Center).Children(func() {
						meetingDot(c, p, speaker)
						ui.Text(c, speaker.Name).Grow(1).MinWidth(0).FontSize(11).FontWeight(600).TextColor(p.text).Tooltip(w.meeting.memberLabel(speaker))
					})
					kind, short, found := strings.Cut(speaker.ID, ":")
					if !found {
						short = speaker.ID
					} else if kind == "preset" {
						short = locales.RoundtableGraphRoleShort(short)
					}
					ui.Text(c, short).MinWidth(0).FontSize(8.5).TextColor(p.muted).Padding(0, 0, 0, 11).Tooltip(speaker.ID)
					ui.Text(c, locales.RoundtableTaskCount(len(nodes))).FontSize(8.5).TextColor(p.muted).Padding(0, 0, 0, 11)
				})
				ui.Row(c.Key("track")).Grow(1).MinWidth(0).Wrap().AlignItems(ui.Center).AlignContent(ui.Center).Gap(4).Padding(7, 8).Children(func() {
					if len(nodes) == 0 {
						ui.Text(c.Key("standby"), locales.RoundtableGraphStandby()).SingleLine().MaxWidthPercent(100).MinWidth(0).FontSize(10).TextColor(p.muted).
							Padding(4, 8).Radius(6).Border(1, p.border).BorderStyle(ui.BorderDashed).Background(p.hover)
					}
					for _, node := range nodes {
						w.graphNode(c, node, layout.position[node.ID], layout.incoming[node.ID], max(1, width-labelWidth-20))
					}
				})
			})
		}
	})
}

func (w *roundtableWindow) graphNode(c *ui.Context, node roundtable.GraphNode, step int, handoff bool, trackWidth float32) {
	p := w.pal
	back, ink := p.added.Alpha(0.10).Over(p.editor), p.addedFg
	readOnly := meetingReadOnlyTools(node.Tools)
	switch {
	case node.Status == roundtable.Failed:
		back, ink = p.dangerFg.Alpha(0.08).Over(p.editor), p.dangerFg
	case node.MemoryNotes > 0:
		back, ink = p.modified.Alpha(0.10).Over(p.editor), p.modifiedFg
	case readOnly || node.Status == roundtable.Stopped:
		back, ink = p.panel, p.muted
	case node.Status != roundtable.Completed:
		back, ink = p.accent.Alpha(0.08).Over(p.editor), p.accent
	}
	ink = legible(ink, p.dark, back)
	ui.Row(c.Key("graph-stop:" + node.ID)).MaxWidth(trackWidth).MinWidth(0).AlignItems(ui.Center).Children(func() {
		if step > 1 {
			line := p.border
			if handoff {
				line = p.accent
			}
			ui.Box(c.Key("lead")).Width(7).Height(1).Shrink(0).Background(line)
		}
		duration := meetingElapsed(node.Start, node.End)
		title, clipped := agentVisibleText(meetingNodeTitle(node), 160)
		if clipped {
			title += "…"
		}
		role := cmp.Or(node.Name, node.AgentID)
		caption := locales.RoundtableGraphNodeTarget(role, title, step)
		button := ui.ButtonBase(c.Key("graph-node:"+node.ID)).MaxWidth(max(1, min(216, trackWidth-7))).MinWidth(0).Padding(4, 8).Gap(5).
			AlignItems(ui.Center).Radius(6).Border(1, mix(p.border, ink, 0.25)).Background(back).
			Label(caption).Tooltip(meetingNodeTitle(node)).Checked(w.graphSel == node.ID).Cursor(ui.CursorPointer)
		if readOnly {
			button.BorderStyle(ui.BorderDashed)
		}
		if button.Hovered() || button.FocusVisible() || w.graphSel == node.ID {
			button.BorderColor(p.accent)
		}
		button.Children(func() {
			ui.Text(c, strconv.Itoa(step)).FontSize(8.5).FontFeatures("tnum").TextColor(ink).Shrink(0)
			ui.Text(c, title).FontSize(10).FontWeight(600).MinWidth(0).TextColor(ink)
			ui.Text(c, duration).FontSize(8.5).FontFeatures("tnum").SingleLine().Shrink(0).TextColor(ink)
		})
		if button.Clicked() {
			w.openNodeDrawer(node.ID)
		}
	})
}

func (w *roundtableWindow) graphMatrix(c *ui.Context, layout meetingGraphLayout, width float32) {
	p := w.pal
	count := len(layout.speakers)
	inner := max(1, width-22)
	nameWidth := min(116, inner*0.32)
	limit := min(count, max(1, int((inner-nameWidth)/59)))
	lastPage := (count - 1) / limit
	page := min(w.graphMatrixPage, lastPage)
	first := page * limit
	last := min(count, first+limit)
	columns := layout.speakers[first:last]
	cellWidth := max(1, (inner-nameWidth)/float32(len(columns)))
	ui.Column(c.Key("graph-matrix")).FillWidth().MinWidth(0).Padding(9, 10).Gap(5).Radius(8).Border(1, p.border).Background(materialOf(c).insetColor(p.editor)).Children(func() {
		ui.Row(c.Key("matrix-title")).FillWidth().AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Text(c, locales.RoundtableGraphMatrix()).Grow(1).MinWidth(0).FontSize(11.5).FontWeight(600).TextColor(p.text).SingleLine()
			if limit < count {
				ui.Text(c, locales.RoundtableGraphMatrixColumns(first+1, last, count)).FontFeatures("tnum").FontSize(10).TextColor(p.muted)
				if roundtableIconButton(c, "graph-matrix-prev", iconChevronLeft, locales.PreviousPage(), p).Disabled(page == 0).Clicked() {
					w.graphMatrixPage = page - 1
				}
				if roundtableIconButton(c, "graph-matrix-next", iconChevronRight, locales.NextPage(), p).Disabled(page == lastPage).Clicked() {
					w.graphMatrixPage = page + 1
				}
			}
		})
		ui.Row(c.Key("matrix-columns")).FillWidth().MinWidth(0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.RoundtableGraphMatrixAxes()).Width(nameWidth).MinWidth(0).FontSize(9).TextColor(p.muted)
			for _, column := range columns {
				ui.Text(c.Key(column.ID), column.Name).Width(cellWidth).MinWidth(0).TextAlign(ui.Center).FontSize(9.5).FontWeight(600).TextColor(p.muted).Tooltip(column.ID)
			}
		})
		for _, from := range layout.speakers {
			ui.Row(c.Key("matrix-row:" + from.ID)).FillWidth().MinWidth(0).AlignItems(ui.Center).Children(func() {
				ui.Row(c.Key("label")).Width(nameWidth).MinWidth(0).AlignItems(ui.Center).Gap(5).Padding(0, 4).Children(func() {
					meetingDot(c, p, from)
					ui.Text(c, from.Name).Grow(1).MinWidth(0).FontSize(9.5).FontWeight(600).TextColor(p.text).Tooltip(from.ID)
				})
				for _, to := range columns {
					ids := layout.pairs[[2]string{from.ID, to.ID}]
					if len(ids) == 0 {
						value := "0"
						if from.ID == to.ID {
							value = "—"
						}
						ui.Text(c.Key(to.ID), value).Width(cellWidth).MinWidth(0).SingleLine().Padding(6, 0).FontSize(10).TextAlign(ui.Center).FontFeatures("tnum").TextColor(p.muted)
						continue
					}
					label := locales.RoundtableGraphPair(w.meeting.memberLabel(from), w.meeting.memberLabel(to), len(ids))
					back := p.accent.Alpha(0.08).Over(p.editor)
					button := ui.ButtonBase(c.Key(to.ID)).Width(cellWidth).MinWidth(0).Height(25).Radius(4).Center().
						Label(label).Tooltip(label).Cursor(ui.CursorPointer).Background(back)
					if button.Hovered() || button.FocusVisible() || slices.Contains(ids, w.graphSel) {
						button.Border(1, p.accentDim)
					}
					button.Children(func() {
						ui.Text(c, strconv.Itoa(len(ids))).FontSize(10.5).FontWeight(600).FontFeatures("tnum").TextColor(legible(p.accent, p.dark, back))
					})
					if button.Clicked() {
						next := (slices.Index(ids, w.graphSel) + 1) % len(ids)
						w.openNodeDrawer(ids[next])
					}
				}
			})
		}
		ui.Text(c.Key("matrix-hint"), locales.RoundtableGraphMatrixHint()).FillWidth().MinWidth(0).FontSize(9.5).TextColor(p.muted)
	})
}

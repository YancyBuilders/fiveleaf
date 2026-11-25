package main

import (
	"cmp"
	"slices"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingDrawerKind uint8

const (
	meetingDrawerClosed meetingDrawerKind = iota
	meetingDrawerNode
	meetingDrawerMemory
	meetingDrawerCollaboration
)

func (w *roundtableWindow) openDrawer(kind meetingDrawerKind) {
	if w.drawer != kind {
		w.drawer, w.drawerFocus = kind, true
	}
}

func (w *roundtableWindow) openNodeDrawer(id string) {
	w.graphSel = id
	w.openDrawer(meetingDrawerNode)
}

func (w *roundtableWindow) closeDrawer() {
	w.drawer, w.drawerFocus = meetingDrawerClosed, false
	w.drawerGraph = roundtable.Graph{}
	w.confirmClear, w.inspector.memory.confirmClear = false, false
}

func meetingDrawerWidth(width float32) float32 {
	switch {
	case width < 860:
		return width
	case width < 1240:
		return 400
	default:
		return 432
	}
}

func (w *roundtableWindow) meetingDrawer(c *ui.Context, snapshot roundtable.Snapshot, graph roundtable.Graph) {
	if w.drawer == meetingDrawerClosed {
		return
	}
	var node roundtable.GraphNode
	var position int
	var label, title, subtitle string
	switch w.drawer {
	case meetingDrawerNode:
		position = slices.Index(graph.Order, w.graphSel)
		if position < 0 {
			graph = w.drawerGraph
			position = slices.Index(graph.Order, w.graphSel)
		}
		index := slices.IndexFunc(graph.Nodes, func(node roundtable.GraphNode) bool { return node.ID == w.graphSel })
		if position < 0 || index < 0 {
			w.closeDrawer()
			return
		}
		node = graph.Nodes[index]
		w.drawerGraph = graph
		label, title = locales.RoundtableNodeDetails(), meetingNodeTitle(node)
		start := "—"
		if !node.Start.IsZero() {
			start = node.Start.Local().Format("15:04:05")
		}
		subtitle = strings.Join([]string{locales.RoundtableNodeNumber(position + 1), start, meetingElapsed(node.Start, node.End), meetingNodeType(graph, node)}, " · ")
	case meetingDrawerMemory:
		title = locales.RoundtableGroupMemory()
		label = title
	case meetingDrawerCollaboration:
		title = locales.RoundtableCollaboration()
		label = title
	}
	pal := w.pal
	ui.Overlay(c, func() {
		back := w.a.dialogBack(c.Key("roundtable-drawer").Key(w.viewGeneration)).Background(ui.RGBA(22, 24, 29, 0.18))
		back.Children(func() {
			dismiss := ui.Box(c.Key("scrim")).Absolute().Left(0).Top(0).Right(0).Bottom(0)
			width, _ := c.Size()
			width = meetingDrawerWidth(width)
			panel := ui.Column(c.Key("panel")).Absolute().Top(0).Right(0).Bottom(0).Width(width).MinWidth(0).MinHeight(0).
				Background(pal.editor).BorderWidth(0, 0, 0, 1).BorderColor(pal.border).Shadow(-8, 0, 26, 0, ui.RGBA(22, 24, 29, 0.10)).
				Role(ui.RoleDialog).Label(label)
			if material := materialOf(c); material.mica() {
				material.glassCard(panel, pal, 0)
			}
			progress := panel.Keyframes("drawer-in", showOver(w.a.keyedOpen(c), motionEnter))
			panel.Translate(width*(1-progress), 0).Children(func() {
				ui.Row(c.Key("drawer-header")).FillWidth().Shrink(0).AlignItems(ui.Center).Padding(10, 12).Gap(8).
					Background(materialOf(c).insetColor(pal.panel)).BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
					if w.drawer == meetingDrawerNode {
						meetingDot(c, pal, w.chatSpeaker(node.AgentID, node.Name))
					} else {
						icon := meetingMemoryIcon
						if w.drawer == meetingDrawerCollaboration {
							icon = iconSettings
						}
						ui.Icon(c, icon).FontSize(16).TextColor(pal.accent)
					}
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						heading := ui.Text(c, title).FillWidth().MaxLines(2).FontSize(13).FontWeight(600).TextColor(pal.text)
						if subtitle != "" {
							heading.Tooltip(title)
							ui.Text(c, subtitle).FillWidth().FontSize(10.5).FontFeatures("tnum").TextColor(pal.muted)
						}
					})
					closeButton := roundtableIconButton(c, "close-drawer", iconX, locales.CloseEsc(), pal)
					if w.drawerFocus {
						closeButton.Focus()
						w.drawerFocus = false
					}
					if closeButton.Clicked() {
						w.closeDrawer()
					}
				})
				switch w.drawer {
				case meetingDrawerNode:
					ui.Scroll(c.Key("node-content").Key(node.ID)).FillWidth().MinWidth(0).Grow(1).MinHeight(0).Padding(12, 14, 18, 14).Children(func() {
						ui.Column(c).FillWidth().MinWidth(0).Gap(6).Children(func() { w.nodeDrawerBody(c, graph, node) })
					})
					w.nodeNavigation(c, graph.Order, position)
				case meetingDrawerMemory:
					w.memoryPanel(c, snapshot)
				case meetingDrawerCollaboration:
					w.collaborationSettings(c, snapshot)
				}
			})
			if dismiss.Clicked() || back.OverlayShortcut(0, ui.KeyEscape) {
				w.closeDrawer()
			}
		})
	})
}

func (w *roundtableWindow) nodeNavigation(c *ui.Context, order []string, position int) {
	pal := w.pal
	ui.Row(c.Key("node-navigation")).FillWidth().Shrink(0).Gap(6).AlignItems(ui.Center).Padding(8, 12).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Background(materialOf(c).insetColor(pal.panel)).Children(func() {
		if actionButton(c.Key("prev"), locales.RoundtablePreviousNode(), iconChevronLeft).Clicked() {
			w.openNodeDrawer(order[(position+len(order)-1)%len(order)])
		}
		if actionButton(c.Key("next"), locales.RoundtableNextNode(), iconChevronRight).Clicked() {
			w.openNodeDrawer(order[(position+1)%len(order)])
		}
		ui.Spacer(c)
		ui.Text(c, locales.RoundtableEscapeHint()).FontSize(10.5).TextColor(pal.muted)
	})
}

func meetingNodeTitle(node roundtable.GraphNode) string {
	return cmp.Or(strings.TrimSpace(node.Title), locales.RoundtableMemberReply())
}

func meetingGraphDuration(graph roundtable.Graph) string {
	if elapsed, ok := graph.Elapsed(); ok {
		return meetingDuration(elapsed)
	}
	if len(graph.Nodes) != 0 {
		return locales.RoundtableInProgress()
	}
	return "—"
}

func meetingReadOnlyTools(tools []roundtable.ToolRecord) bool {
	return len(tools) > 0 && !slices.ContainsFunc(tools, func(tool roundtable.ToolRecord) bool {
		return tool.Kind != "read" && tool.Kind != "search"
	})
}

func meetingRoleTag(speaker meetingSpeaker) string {
	if role, ok := strings.CutPrefix(speaker.ID, "preset:"); ok {
		return locales.RoundtableGraphRoleShort(role)
	}
	return speaker.provider.name
}

func meetingSpeakerModel(speaker meetingSpeaker) string {
	provider := cmp.Or(speaker.provider.name, speaker.Provider)
	if model := speaker.role.Model; model != "" {
		if provider == "" {
			return model
		}
		return provider + " · " + model
	}
	return provider
}

func meetingChip(c *ui.Context, text string, ink, back ui.Color) {
	ui.Text(c, text).MaxWidthPercent(100).MinWidth(0).FontSize(10).FontWeight(550).TextColor(ink).Background(back).Radius(4).Padding(1, 5)
}

package main

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func (w *roundtableWindow) messageFlow(c *ui.Context, snapshot roundtable.Snapshot, graph roundtable.Graph, latestPage bool) {
	messages := snapshot.Messages
	if len(messages) == 0 {
		return
	}
	flow := ui.Column(c.Key("message-flow")).FillWidth().MinWidth(0).Gap(14)
	topics := w.flowTopics(flow, snapshot, latestPage)
	flow.Children(func() {
		date := ""
		for index, message := range messages {
			when := message.Time.Local()
			if key := when.Format("2006-01-02"); !message.Time.IsZero() && key != date {
				date = key
				meetingDay(c, w.pal, when)
			}
			if number := topics[message.ID]; number > 0 {
				w.topicDivider(c, number, message)
			}
			w.messageTurn(c, message, messages[:index], graph)
			if task := message.Task; message.Role == "assistant" && task != nil && task.Stage == roundtable.Coordinate && task.Status == roundtable.Completed && !task.NoAnswer {
				var adjacentID string
				switch {
				case index+1 < len(messages):
					adjacentID = messages[index+1].ID
				case latestPage:
					adjacentID = snapshot.ActiveTask.ID
				case len(w.pageCursors) > 0:
					adjacentID = w.pageCursors[len(w.pageCursors)-1].ID
				}
				w.dispatchStrip(c, message, graph, adjacentID)
			}
		}
		last := messages[len(messages)-1]
		if latestPage && !snapshot.Busy && snapshot.Error == "" && last.Role == "assistant" && !last.ToolOnly && last.Task != nil && last.Task.Stage == roundtable.Conclusion && last.Task.Status == roundtable.Completed && !last.Task.NoAnswer {
			w.roundClosedStrip(c, graph, last)
		}
	})
}

func meetingFlowConclusion(message roundtable.Message) bool {
	return message.HumanConclusion || message.Task != nil && message.Task.Stage == roundtable.Conclusion && message.Task.Status == roundtable.Completed && !message.Task.NoAnswer
}

func meetingTopicTitle(text string) string {
	text = strings.TrimSpace(text)
	text, _ = agentVisibleText(text, 49*utf8.UTFMax)
	for index, char := range text {
		if index > 0 && (strings.ContainsRune("。！？!?\r\n", char) || char == '.' && (index+1 == len(text) || unicode.IsSpace(firstFlowRune(text[index+1:])))) {
			text = text[:index]
			break
		}
	}
	return ellipsize(strings.TrimSpace(text), 48)
}

func firstFlowRune(text string) rune {
	char, _ := utf8.DecodeRuneInString(text)
	return char
}

func (w *roundtableWindow) topicDivider(c *ui.Context, number int, message roundtable.Message) {
	p := w.pal
	ui.Row(c.Key("topic:" + message.ID)).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(7).Children(func() {
		ui.Box(c).Size(4, 4).Shrink(0).Radius(2).Background(p.muted)
		ui.Text(c, locales.RoundtableTopicDivider(number, meetingTopicTitle(message.Text))).MinWidth(0).MaxWidthPercent(90).FontSize(10.5).FontWeight(550).TextColor(p.muted)
		ui.Box(c).Grow(1).MinWidth(0).Height(1).Background(p.border)
	})
}

func (w *roundtableWindow) messageTurn(c *ui.Context, message roundtable.Message, before []roundtable.Message, graph roundtable.Graph) {
	human, conclusion := message.Role == "user", meetingFlowConclusion(message)
	turn := ui.Row(c.Key("turn:" + message.ID)).FillWidth().MinWidth(0).AlignItems(ui.Start)
	if human {
		turn.Justify(ui.End)
	}
	turn.Children(func() {
		body := ui.Column(c.Key("message-body")).FillWidth().MaxWidth(620).MinWidth(0).Gap(5)
		if !conclusion && len(message.Text) < 240 && !strings.Contains(message.Text, "\n") && (message.Task == nil || message.Task.Status == roundtable.Completed && !message.Task.NoAnswer) {
			width, _ := c.MeasureText(0, ui.Span{Text: message.Text, Size: w.a.settings.FontSize})
			minimum := float32(260)
			if human {
				minimum = 120
			}
			body.MaxWidth(min(620, max(minimum, width+34)))
		}
		body.Children(func() {
			w.messageAuthor(c, message, human)
			if conclusion {
				w.conclusionCard(c, message, before, graph)
			} else {
				w.messageBubble(c, message, human)
			}
		})
	})
}

func (w *roundtableWindow) messageAuthor(c *ui.Context, message roundtable.Message, human bool) {
	p := w.pal
	speaker := w.chatSpeaker(message.AgentID, message.Name)
	name := cmp.Or(message.Name, speaker.Name, message.AgentID)
	moderator := !human && message.AgentID == roundtable.ModeratorID
	row := ui.Row(c.Key("author")).FillWidth().MinWidth(0).Wrap().AlignItems(ui.Center).Gap(5)
	if human {
		row.Justify(ui.End)
		name = locales.RoundtableHuman()
	} else if moderator && name == roundtable.ModeratorID {
		name = locales.AgentRoleRoundtableModerator()
	}
	row.Children(func() {
		identity := func() {
			switch {
			case human:
				ui.Icon(c, meetingHumanIcon).FontSize(12).TextColor(p.text).Shrink(0)
			case moderator:
				ui.Icon(c, iconAgent).FontSize(12).TextColor(p.accent).Shrink(0)
			default:
				meetingDot(c, p, speaker)
			}
			ui.Text(c, name).FontSize(12).FontWeight(600).TextColor(p.text).MinWidth(0)
		}
		if !human && message.AgentID != "" {
			author := ui.ButtonBase(c.Key("speaker")).Label(name).Tooltip(locales.RoundtableMemberTasks(name)).AlignItems(ui.Center).Gap(5).MaxWidthPercent(100).MinWidth(0).Cursor(ui.CursorPointer)
			author.Children(identity)
			if author.Clicked() {
				if message.Task != nil {
					w.openMeetingMessage(message)
				} else {
					w.selectMember(message.AgentID)
				}
			}
		} else {
			ui.Row(c.Key("speaker")).MinWidth(0).AlignItems(ui.Center).Gap(5).Children(identity)
		}
		switch {
		case moderator:
			meetingChip(c, locales.RoundtableAIChairTag(), p.accent, p.accent.Alpha(0.1))
		case human && message.HumanConclusion:
			meetingChip(c, locales.RoundtableChairTag(), p.text, p.hover)
		case !human && strings.HasPrefix(speaker.ID, "preset:"):
			if role := meetingRoleTag(speaker); role != "" {
				ink := legible(meetingInk(p, speaker), p.dark, p.editor)
				meetingChip(c, role, ink, ink.Alpha(0.09))
			}
		}
		if task := message.Task; task != nil && meetingReadOnlyTools(task.Tools) {
			meetingChip(c, locales.RoundtableReadOnly(), p.muted, p.hover)
		}
		if !message.Time.IsZero() {
			ui.Text(c, message.Time.Local().Format("15:04")).FontSize(10).FontFeatures("tnum").TextColor(p.muted).Tooltip(dateTime(message.Time.Local())).Shrink(0)
		}
		canPin := !message.ToolOnly && (human || message.Role == "assistant") && message.Text != "" && message.ID != ""
		if task := message.Task; task != nil {
			canPin = canPin && (task.Stage == roundtable.Direct || task.Stage == roundtable.Conclusion) && task.Status == roundtable.Completed && !task.NoAnswer
		} else if !human && strings.HasPrefix(strings.TrimSpace(message.Text), "@") {
			canPin = false
		}
		if canPin && !w.meeting.readOnly && w.meeting.room != nil && roundtableIconButton(c, "pin-memory", meetingPinIcon, locales.RoundtablePinToMemory(), p).Size(17, 17).Disabled(w.meeting.room.Busy()).Clicked() {
			w.beginMemoryNote(message)
		}
	})
}

func meetingFlowNode(graph roundtable.Graph, message roundtable.Message) (int, bool) {
	start, found := slices.BinarySearchFunc(graph.Nodes, message.Time, func(node roundtable.GraphNode, when time.Time) int { return node.Start.Compare(when) })
	if found {
		for index := start; index < len(graph.Nodes) && graph.Nodes[index].Start.Equal(message.Time); index++ {
			if graph.Nodes[index].ID == message.ID {
				return index, true
			}
		}
	}
	return 0, false
}

func meetingDispatchWork(graph roundtable.Graph, message roundtable.Message, adjacentID string) []roundtable.GraphNode {
	position, ok := meetingFlowNode(graph, message)
	if !ok {
		return nil
	}
	after := graph.Nodes[position+1:]
	turnID := message.Task.TurnID
	if turnID == "" {
		if len(after) == 0 || adjacentID == "" || after[0].ID != adjacentID {
			return nil
		}
		after = after[:1]
	}
	var work []roundtable.GraphNode
	for _, node := range after {
		if node.TurnID == turnID && node.Stage == roundtable.Work && node.Hop == message.Hop+1 {
			work = append(work, node)
		}
	}
	return work
}

func (w *roundtableWindow) dispatchTarget(text string, work []roundtable.GraphNode) (meetingSpeaker, string) {
	text = strings.TrimSpace(text)
	match := func(text string) (meetingSpeaker, string) {
		var target meetingSpeaker
		var assignment string
		try := func(speaker meetingSpeaker) {
			if len(speaker.ID) <= len(target.ID) {
				return
			}
			rest, ok := strings.CutPrefix(text, "@"+speaker.ID)
			if ok && unicode.IsSpace(firstFlowRune(rest)) {
				target, assignment = speaker, strings.TrimSpace(rest)
			}
		}
		for _, speaker := range w.meeting.speakers {
			try(speaker)
		}
		for _, node := range work {
			try(w.chatSpeaker(node.AgentID, node.Name))
		}
		return target, assignment
	}
	target, assignment := match(text)
	if target.ID == "" && strings.Contains(text, "@") {
		first, rest, _ := strings.Cut(text, "\n")
		doc := parser.Parse(first)
		if doc.Len() > 1 && doc.Block(1).Kind == parser.Paragraph {
			text = parser.PlainText(doc.Inlines(1)) + "\n" + rest
			target, assignment = match(text)
		}
	}
	if target.ID == "" {
		if mention, ok := strings.CutPrefix(text, "@"); ok {
			if space := strings.IndexFunc(mention, unicode.IsSpace); space > 0 {
				target, assignment = w.chatSpeaker(mention[:space], ""), strings.TrimSpace(mention[space:])
			}
		}
	}
	return target, assignment
}

func (w *roundtableWindow) dispatchStrip(c *ui.Context, message roundtable.Message, graph roundtable.Graph, adjacentID string) {
	work := meetingDispatchWork(graph, message, adjacentID)
	target, assignment := w.dispatchTarget(message.Text, work)
	if target.ID == "" || assignment == "" {
		return
	}
	var next roundtable.GraphNode
	for _, node := range work {
		if node.AgentID == target.ID {
			next = node
			break
		}
	}
	linked := next.ID != ""
	p := w.pal
	sender := w.chatSpeaker(message.AgentID, message.Name)
	label := locales.RoundtableDispatchTarget(w.meeting.memberLabel(sender), w.meeting.memberLabel(target))
	strip := ui.ButtonBase(c.Key("dispatch:"+message.ID)).Label(label).Tooltip(assignment).Disabled(!linked).
		FillWidth().MinWidth(0).Wrap().Gap(7).AlignItems(ui.Center).Padding(6, 9).BorderWidth(0, 0, 0, 2).BorderColor(p.accentDim).Radius(0, 6, 6, 0)
	from := p.accent.Alpha(0.09)
	if linked {
		strip.Cursor(ui.CursorPointer)
		if strip.Hovered() || strip.FocusVisible() {
			from = p.accent.Alpha(0.17)
			strip.BorderColor(p.accent)
		}
	}
	strip.Gradient(from, p.accent.Alpha(0), 90).Children(func() {
		ui.Text(c, label).MaxWidthPercent(100).MinWidth(0).FontSize(11).FontWeight(600).TextColor(p.accent)
		ui.Text(c, ellipsize(strings.Join(strings.Fields(assignment), " "), 76)).Grow(1).MinWidth(0).MaxWidthPercent(100).FontSize(10.5).TextColor(p.muted)
		if linked {
			ink := p.muted
			if next.Status == roundtable.Completed {
				ink = p.addedFg
			}
			meetingChip(c, taskStatusLabel(next.Status), ink, ink.Alpha(0.09))
			ui.Text(c, meetingElapsed(next.Start, next.End)).FontSize(10).FontFeatures("tnum").TextColor(p.muted)
		}
	})
	if linked && strip.Clicked() {
		w.openNodeDrawer(next.ID)
	}
}

func (w *roundtableWindow) roundClosedStrip(c *ui.Context, graph roundtable.Graph, last roundtable.Message) {
	nodes := meetingFlowTurnNodes(graph, last)
	inTurn := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		inTurn[node.ID] = true
	}
	handoffs := 0
	for _, edge := range graph.Edges {
		if edge.Kind == roundtable.GraphHandoff && inTurn[edge.From] && inTurn[edge.To] {
			handoffs++
		}
	}
	p := w.pal
	ui.Row(c.Key("round-closed")).FillWidth().MinWidth(0).Wrap().Gap(8).AlignItems(ui.Center).Padding(9, 12).
		Border(1, p.border).BorderStyle(ui.BorderDashed).Radius(8).Background(p.added.Alpha(0.04)).Children(func() {
		ui.Icon(c, iconCheck).FontSize(15).TextColor(p.addedFg).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).MaxWidthPercent(100).Gap(3).Children(func() {
			ui.Text(c, locales.RoundtableRoundClosed()).FontSize(12).FontWeight(600).TextColor(p.text)
			ui.Text(c, locales.RoundtableConclusionSaved()).MinWidth(0).FontSize(10.5).TextColor(p.muted)
		})
		ui.Text(c, strings.Join([]string{locales.RoundtableGraphCountsNodes(len(nodes)), locales.RoundtableGraphCountsHandoffs(handoffs), meetingGraphDuration(roundtable.Graph{Nodes: nodes})}, " · ")).
			MaxWidthPercent(100).MinWidth(0).FontSize(10).FontFeatures("tnum").TextColor(p.muted)
	})
}

package main

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingMemberWork struct {
	count  int
	latest roundtable.Message
	when   time.Time
}

func meetingMemberTasks(snapshot roundtable.Snapshot) map[string]meetingMemberWork {
	byMember := make(map[string]meetingMemberWork)
	record := func(message roundtable.Message) {
		if message.AgentID == "" || message.Task == nil && message.Role != "assistant" {
			return
		}
		work := byMember[message.AgentID]
		work.count++
		if !message.Time.IsZero() {
			work.when = message.Time
		}
		if task := message.Task; task != nil && message.ID != "" {
			work.latest = message
			if !task.Finished.IsZero() {
				work.when = task.Finished
			}
		}
		byMember[message.AgentID] = work
	}
	for _, message := range snapshot.Messages {
		if snapshot.ActiveTask.ID != "" && message.ID == snapshot.ActiveTask.ID {
			continue
		}
		record(message)
	}
	record(snapshot.ActiveTask)
	return byMember
}

func (w *roundtableWindow) members(c *ui.Context, snapshot roundtable.Snapshot, graph roundtable.Graph) {
	p := w.pal
	ui.Scroll(c.Key("roundtable-members")).TrackScroll(&w.membersScroll).FillWidth().Grow(1).MinHeight(0).MinWidth(0).Padding(0, 11, 12, 11).Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(6).Children(func() {
			heading := func(label string) {
				ui.Text(c, label).FontSize(10.5).FontWeight(550).TextColor(p.muted).Margin(10, 2, 0, 2)
			}
			heading(locales.RoundtableMemberStatus())
			tasks := meetingMemberTasks(snapshot)
			for _, speaker := range w.meeting.speakers {
				w.memberCard(c, speaker, tasks[speaker.ID], snapshot)
			}
			heading(locales.RoundtableGroupStats())
			if snapshot.HasEarlier {
				ui.Text(c, locales.RoundtableLoadedScope()).FillWidth().MinWidth(0).FontSize(10).TextColor(p.muted)
			}
			w.memberStats(c, graph, len(w.meeting.room.Memory().Notes))
			if sources := meetingEvidenceSources(snapshot); len(sources) > 0 {
				heading(locales.RoundtableEvidenceSources())
				for _, source := range sources {
					ui.Row(c.Key(source)).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(6).Padding(6, 8).Radius(6).Border(1, p.border).Children(func() {
						ui.Icon(c, iconFileText).FontSize(12).TextColor(p.muted).Shrink(0)
						ui.Text(c, source).MinWidth(0).SingleLine().FontSize(10.5).TextColor(p.text).Tooltip(source).Selectable()
					})
				}
			}
		})
	})
}

func (w *roundtableWindow) memberCard(c *ui.Context, speaker meetingSpeaker, work meetingMemberWork, snapshot roundtable.Snapshot) {
	p := w.pal
	active := snapshot.Busy && snapshot.ActiveAgent == speaker.ID
	task := work.latest.Task
	status, ink := locales.RoundtableStandby(), p.muted
	switch {
	case active:
		status, ink = locales.RoundtableRunning(), p.accent
		if chat := w.meeting.chat; chat != nil && len(chat.permissions) > 0 {
			status, ink = locales.AgentTaskApproval(), p.modifiedFg
		}
	case task != nil && task.Status == roundtable.Completed:
		status, ink = locales.RoundtableComplete(), p.addedFg
	}
	assignment := locales.RoundtableGraphStandby()
	if active {
		assignment = locales.RoundtableMemberReply()
	}
	if task != nil {
		assignment = cmp.Or(strings.TrimSpace(task.Assignment), locales.RoundtableMemberReply())
		lastInRoom := len(snapshot.Messages) > 0 && snapshot.Messages[len(snapshot.Messages)-1].ID == work.latest.ID
		if lastInRoom && !snapshot.Busy && speaker.ID == roundtable.ModeratorID && task.Stage == roundtable.Conclusion && task.Status == roundtable.Completed && !task.NoAnswer {
			assignment = locales.RoundtableNoMoreHandoffs()
		}
	}
	first, _, multiline := strings.Cut(assignment, "\n")
	excerpt, clipped := agentVisibleText(first, 160)
	if multiline || clipped {
		excerpt = strings.TrimSpace(excerpt) + "…"
	}
	count := locales.RoundtableTaskCount(work.count)
	if snapshot.HasEarlier {
		count = locales.RoundtableRecentTasks(work.count)
	}
	metadata := meetingSpeakerModel(speaker)
	detail := metadata
	if !work.when.IsZero() {
		if metadata != "" {
			metadata += " · "
		}
		when := work.when.Local()
		detail, metadata = metadata+dateTime(when), metadata+when.Format("15:04")
	}
	name := w.meeting.memberLabel(speaker)
	card := ui.ButtonBase(c.Key("member:"+speaker.ID)).Column().FillWidth().MinWidth(0).AlignItems(ui.Stretch).Gap(4).Padding(8, 9).
		Radius(8).Border(1, p.border).Background(materialOf(c).insetColor(p.editor)).Label(name).Description(count + " · " + status).Tooltip(speaker.detail()).Cursor(ui.CursorPointer)
	if !w.meeting.readOnly && w.meeting.group.ID != "" {
		card.ContextMenu(func(menu *ui.Menu) {
			if menu.Item(locales.RoundtableRemoveMember()).Disabled(snapshot.Busy).Chosen() {
				w.post(func() { w.removeParticipant(speaker.ID) })
			}
			if snapshot.Busy {
				menu.Item(locales.RoundtableRemoveAfterTurn()).Disabled(true)
			}
		})
	}
	if card.Hovered() || card.FocusVisible() {
		card.BorderColor(p.accent).Background(materialOf(c).insetColor(mix(p.editor, p.accent, 0.04)))
	}
	card.Children(func() {
		ui.Row(c.Key("identity")).FillWidth().MinWidth(0).Gap(5).AlignItems(ui.Start).Children(func() {
			ui.Row(c).Height(15).Shrink(0).AlignItems(ui.Center).Children(func() { meetingDot(c, p, speaker) })
			ui.Row(c).Basis(0).Grow(1).MinWidth(0).Wrap().Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, speaker.Name).MaxWidthPercent(100).MinWidth(0).FontSize(11.5).FontWeight(600).TextColor(p.text).Tooltip(w.meeting.memberLabel(speaker))
				if role := meetingRoleTag(speaker); role != "" {
					ui.Text(c, role).MaxWidthPercent(100).MinWidth(0).FontSize(9).TextColor(p.muted).Padding(0, 3).Radius(3).Border(1, p.border).Tooltip(role)
				}
			})
			ui.Text(c, count).SingleLine().FontSize(9).FontFeatures("tnum").TextColor(p.muted).Shrink(0)
		})
		ui.Row(c.Key("assignment")).FillWidth().MinWidth(0).Gap(4).AlignItems(ui.Center).Padding(0, 0, 0, 11).Children(func() {
			ui.Text(c, excerpt).Basis(0).Grow(1).MinWidth(0).SingleLine().FontSize(10.5).TextColor(p.text).Tooltip(assignment)
			meetingChip(c, status, ink, ink.Alpha(0.09).Over(p.editor))
		})
		if metadata != "" {
			ui.Text(c.Key("metadata"), metadata).FillWidth().MinWidth(0).Padding(0, 0, 0, 11).FontSize(9.5).FontFeatures("tnum").TextColor(p.muted).Tooltip(detail)
		}
		if speaker.problem != "" {
			ui.Text(c, speaker.problem).MinWidth(0).SingleLine().FontSize(10.5).TextColor(p.dangerFg).Padding(0, 0, 0, 11).Tooltip(speaker.problem)
		}
	})
	if card.Clicked() {
		if work.latest.ID != "" {
			w.openMeetingTask(speaker.ID, work.latest.ID)
		} else {
			w.selectMember(speaker.ID)
		}
	}
}

func (w *roundtableWindow) memberStats(c *ui.Context, graph roundtable.Graph, pinned int) {
	p := w.pal
	for index, row := range [2][2]struct {
		label, value string
		ink          ui.Color
	}{
		{{locales.RoundtableGraphNodes(), strconv.Itoa(len(graph.Nodes)), p.accent}, {locales.RoundtableGraphHandovers(), strconv.Itoa(graph.Handoffs()), p.addedFg}},
		{{locales.RoundtablePinnedCountLabel(), strconv.Itoa(pinned), p.modifiedFg}, {locales.RoundtableGraphDuration(), meetingGraphDuration(graph), p.text}},
	} {
		ui.Row(c.Key("member-stats").Key(index)).FillWidth().MinWidth(0).Gap(6).Children(func() {
			for _, stat := range row {
				ui.Column(c.Key(stat.label)).Basis(0).Grow(1).MinWidth(0).Gap(2).Padding(7, 8).Radius(7).Border(1, p.border).Background(materialOf(c).insetColor(p.panel)).Children(func() {
					ui.Text(c, stat.value).MinWidth(0).SingleLine().FontSize(14).FontWeight(600).FontFeatures("tnum").TextColor(legible(stat.ink, p.dark, p.panel)).Tooltip(stat.value)
					ui.Text(c, stat.label).MinWidth(0).FontSize(9.5).TextColor(p.muted)
				})
			}
		})
	}
}

var meetingEvidenceLocation = regexp.MustCompile(`(?m)^[\t ]*((?:[A-Za-z]:)?[\pL\pN_./\\~+-]+\.[\pL][\pL\pN]*(?::[1-9][0-9]*|#L[1-9][0-9]*(?:-L?[1-9][0-9]*)?))(?::|[\t \r]|$)`)

func meetingEvidenceSources(snapshot roundtable.Snapshot) []string {
	const limit = 6
	var sources []string
	collect := func(message roundtable.Message) {
		if message.Task == nil {
			return
		}
		for _, tool := range message.Task.Tools {
			for line := range strings.Lines(tool.Detail) {
				if !strings.ContainsAny(line, ":#") {
					continue
				}
				if match := meetingEvidenceLocation.FindStringSubmatch(line); match != nil && !slices.Contains(sources, match[1]) {
					sources = append(sources, match[1])
					if len(sources) == limit {
						return
					}
				}
			}
		}
	}
	collect(snapshot.ActiveTask)
	for _, message := range slices.Backward(snapshot.Messages) {
		if len(sources) == limit {
			break
		}
		collect(message)
	}
	return sources
}

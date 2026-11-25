package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func (w *roundtableWindow) selectMember(id string) {
	w.memberID, w.expandedTask, w.openTool = id, "", ""
	w.taskReveal = false
	w.olderTasks, w.pinnedTask = nil, nil
	w.oldestTaskPage, w.taskScroll = roundtable.Message{}, ui.ScrollState{}
	w.taskPageEnd, w.loadingTasks = false, false
	w.taskVisit++
	if id != "" {
		w.inspector.inviting = false
		w.inspector.memory.confirmClear = false
		w.inspector.setVisible(true)
	}
}

func meetingExcerpt(text string) (string, bool) {
	count, lines, boundary := 0, 1, 0
	for index, char := range text {
		if count >= 220 || lines > 5 {
			cut := index
			if boundary > 0 && index-boundary < 160 {
				cut = boundary
			}
			return strings.TrimSpace(text[:cut]) + "…", true
		}
		if char == '\n' {
			lines++
		}
		if strings.ContainsRune("。！？.!?\n", char) {
			boundary = index + utf8.RuneLen(char)
		}
		count++
	}
	return text, false
}

func taskStatusLabel(status string) string {
	switch status {
	case roundtable.Running:
		return locales.AgentTaskRunning()
	case roundtable.Failed:
		return locales.RoundtableFailed()
	case roundtable.Stopped:
		return locales.RoundtableStopped()
	default:
		return locales.RoundtableDone()
	}
}

func (w *roundtableWindow) taskLink(c *ui.Context, message roundtable.Message, label string) {
	if actionButton(c.Key("task-link"), label, iconChevronRight).AlignSelf(ui.Start).Clicked() {
		w.openMeetingMessage(message)
	}
}

func (w *roundtableWindow) openMeetingTask(agentID, taskID string) {
	w.selectMember(agentID)
	w.expandedTask, w.graphSel, w.taskReveal = taskID, taskID, true
}

func (w *roundtableWindow) openMeetingMessage(message roundtable.Message) {
	w.openMeetingTask(message.AgentID, message.ID)
	w.pinnedTask = &message
}

func meetingElapsed(start, end time.Time) string {
	if end.IsZero() {
		return locales.RoundtableInProgress()
	}
	return meetingDuration(end.Sub(start))
}

func meetingDuration(duration time.Duration) string {
	seconds := max(0, duration).Round(time.Second / 10).Seconds()
	formatted := i18n.FormatNumber(i18n.Locale(), seconds, i18n.NumberOptions{MinFractionDigits: 1, MaxFractionDigits: 1})
	return locales.RoundtableElapsedSeconds(formatted)
}

func meetingStage(stage string) string {
	switch stage {
	case roundtable.Coordinate:
		return locales.RoundtableStageCoordinate()
	case roundtable.Work:
		return locales.RoundtableStageWork()
	case roundtable.Conclusion:
		return locales.RoundtableConclusion()
	case roundtable.Direct:
		return locales.RoundtableStageDirect()
	default:
		return locales.RoundtableMemberReply()
	}
}

func (w *roundtableWindow) taskHistory(c *ui.Context, snapshot roundtable.Snapshot) {
	var tasks []roundtable.Message
	if active := snapshot.ActiveTask; active.AgentID == w.memberID && active.ID != "" {
		tasks = append(tasks, active)
	}
	for index := len(snapshot.Messages) - 1; index >= 0; index-- {
		message := snapshot.Messages[index]
		if message.AgentID == w.memberID {
			tasks = append(tasks, message)
		}
	}
	for _, older := range w.olderTasks {
		if !slices.ContainsFunc(tasks, func(message roundtable.Message) bool { return message.ID == older.ID }) {
			tasks = append(tasks, older)
		}
	}
	if pinned := w.pinnedTask; pinned != nil && !slices.ContainsFunc(tasks, func(message roundtable.Message) bool { return message.ID == pinned.ID }) {
		tasks = append(tasks, *pinned)
	}
	ui.Scroll(c.Key("role-tasks:"+w.memberID)).TrackScroll(&w.taskScroll).Grow(1).MinHeight(0).Padding(16, 18).Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(8).Children(func() {
			for _, speaker := range w.meeting.speakers {
				if speaker.ID == w.memberID {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, locales.RoundtableMemberTasks(speaker.Name)).FontSize(15).FontWeight(600).TextColor(w.pal.text).Grow(1).MinWidth(0)
						if !w.meeting.readOnly && speaker.problem == "" && actionButton(c, locales.RoundtableAskMember(), iconMessageText).Clicked() {
							w.returnToChat()
							w.addressMember(speaker)
						}
					})
					break
				}
			}
			if len(tasks) == 0 {
				ui.Text(c, locales.RoundtableNoTasks()).FontSize(12).TextColor(w.pal.muted)
			}
			for _, message := range tasks {
				w.taskRow(c, message)
			}
			if snapshot.HasEarlier && len(snapshot.Messages) > 0 && !w.taskPageEnd {
				if ui.Button(c, locales.RoundtableEarlierTasks()).Disabled(w.loadingTasks).AlignSelf(ui.Start).Clicked() {
					cursor := w.oldestTaskPage
					if cursor.ID == "" {
						cursor = snapshot.Messages[0]
					}
					w.fetchMemberTasks(cursor)
				}
			}
			if w.err != "" {
				ui.Text(c, w.err).FontSize(12).TextColor(w.pal.dangerFg).Selectable()
			}
		})
	})
}

func (w *roundtableWindow) fetchMemberTasks(cursor roundtable.Message) {
	room, visit, member := w.meeting.room, w.taskVisit, w.memberID
	w.loadingTasks = true
	go func() {
		var found []roundtable.Message
		var more bool
		var err error
		for {
			var page []roundtable.Message
			page, more, err = room.Earlier(cursor, meetingPageSize)
			if err != nil || len(page) == 0 {
				break
			}
			cursor = page[0]
			for index := len(page) - 1; index >= 0; index-- {
				if page[index].AgentID == member {
					found = append(found, page[index])
				}
			}
			if len(found) > 0 || !more {
				break
			}
		}
		w.postWindow(func() {
			if w.taskVisit != visit || w.meeting.room != room {
				return
			}
			w.loadingTasks = false
			if err != nil {
				w.err = err.Error()
				return
			}
			w.olderTasks = append(w.olderTasks, found...)
			w.oldestTaskPage, w.taskPageEnd = cursor, !more
		})
	}()
}

func (w *roundtableWindow) taskRow(c *ui.Context, message roundtable.Message) {
	status, title := roundtable.Completed, locales.RoundtableMemberReply()
	if task := message.Task; task != nil {
		status = task.Status
		title = cmp.Or(task.Assignment, title)
		if task.Stage == roundtable.Conclusion {
			title = locales.RoundtableConclusion() + " · " + title
		} else if task.Stage == roundtable.Coordinate && task.Status == roundtable.Completed {
			longest := 0
			for _, speaker := range w.meeting.speakers {
				prefix := "@" + speaker.ID
				if rest, ok := strings.CutPrefix(message.Text, prefix); ok && len(prefix) > longest && len(rest) > 0 && strings.ContainsRune(" \t\r\n", rune(rest[0])) {
					longest = len(prefix)
					title = locales.RoundtableAssignedTo(w.meeting.memberLabel(speaker)) + " · " + strings.TrimSpace(rest)
				}
			}
		}
	}
	open := w.expandedTask == message.ID
	ui.Column(c.Key(message.ID)).FillWidth().MinWidth(0).Gap(5).BorderWidth(0, 0, 1, 0).BorderColor(w.pal.border).Padding(3, 0, 8, 0).Children(func() {
		label := message.Time.Local().Format("15:04:05") + " · " + title
		button := ui.ButtonBase(c.Key("task")).FillWidth().Gap(8).Padding(7, 4).MinHeight(36).AlignItems(ui.Center).Label(label).Expanded(open).Cursor(ui.CursorPointer)
		if open || button.Hovered() || button.FocusVisible() {
			button.Background(w.pal.hover)
		}
		button.Children(func() {
			icon := iconChevronRight
			if open {
				icon = iconChevronDown
			}
			ui.Icon(c, icon).FontSize(12).TextColor(w.pal.muted)
			ui.Column(c).MinWidth(0).Grow(1).Gap(3).Children(func() {
				ui.Text(c, title).FontSize(13).TextColor(w.pal.text).SingleLine()
				ui.Text(c, message.Time.Local().Format("01-02 15:04:05")+" · "+locales.RoundtableHop(message.Hop)).FontSize(11).TextColor(w.pal.muted)
			})
			ink := w.pal.muted
			if status == roundtable.Failed {
				ink = w.pal.dangerFg
			} else if status == roundtable.Running {
				ink = w.pal.accent
			}
			ui.Text(c, taskStatusLabel(status)).FontSize(11).TextColor(ink).SingleLine()
		})
		if open && w.taskReveal {
			button.Focus().ScrollIntoView()
			w.taskReveal = false
		}
		if button.Clicked() {
			w.expandedTask, w.openTool = message.ID, ""
			if open {
				w.expandedTask = ""
			} else {
				w.graphSel = message.ID
			}
		}
		if !open {
			return
		}
		if task := message.Task; task != nil {
			w.taskHeading(c, locales.RoundtableTaskSummary())
			ui.Text(c, strings.Join([]string{taskStatusLabel(task.Status), meetingStage(task.Stage), locales.RoundtableHop(message.Hop)}, " · ")).FontSize(12).TextColor(w.pal.text).Selectable()
			end := locales.RoundtableInProgress()
			if !task.Finished.IsZero() {
				end = task.Finished.Local().Format(time.DateTime)
			}
			ui.Text(c, message.Time.Local().Format(time.DateTime)+" → "+end).FontSize(11).FontFeatures("tnum").TextColor(w.pal.muted).Selectable()
			ui.Text(c, meetingElapsed(message.Time, task.Finished)).FontSize(11).FontFeatures("tnum").TextColor(w.pal.muted)
			if task.Assignment != "" {
				w.taskHeading(c, locales.RoundtableTaskAssignment())
				ui.Text(c, task.Assignment).FillWidth().MinWidth(0).FontSize(12).TextColor(w.pal.muted).Selectable()
			}
			if task.MemoryNotes > 0 || task.MemoryTurns > 0 {
				w.taskHeading(c, locales.RoundtableTaskMemory())
				ui.Row(c.Key("memory-used")).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, meetingMemoryIcon).FontSize(13).TextColor(w.pal.muted).Shrink(0)
					ui.Text(c, locales.RoundtableMemoryUsed(task.MemoryNotes, task.MemoryTurns)).MinWidth(0).FontSize(11.5).TextColor(w.pal.muted)
				})
			}
		}
		w.taskTools(c, message)
		if message.Text != "" {
			w.taskHeading(c, locales.RoundtableTaskOutput())
			w.messageMarkdown(c, message.ID, message.Text)
		}
		if message.Original != "" {
			w.taskHeading(c, locales.RoundtableTaskOriginal())
			if ui.Button(c.Key("copy-original"), locales.RoundtableCopyOriginal()).AlignSelf(ui.Start).Clicked() {
				c.WriteClipboard(message.Original)
			}
		}
	})
}

func (w *roundtableWindow) taskHeading(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(w.pal.text).Padding(5, 0, 0, 0)
}

func (w *roundtableWindow) taskTools(c *ui.Context, message roundtable.Message) {
	if task := message.Task; task != nil && len(task.Tools) > 0 {
		w.taskHeading(c, locales.RoundtableTaskTools())
		for _, tool := range task.Tools {
			w.taskTool(c, message.ID, tool, nil, w.meeting.reviews[message.ID][tool.ID])
		}
		return
	}
	if chat := w.meeting.chat; chat != nil && w.meeting.chatTask == message.ID && len(chat.tools) > 0 {
		w.taskHeading(c, locales.RoundtableTaskTools())
		for index, live := range chat.tools {
			if live == nil {
				continue
			}
			tool := roundtable.ToolRecord{ID: cmp.Or(live.id, strconv.Itoa(index)), Title: live.title, Kind: live.kind, Status: live.status, Detail: w.a.agentToolInputSummary(live), Note: live.note}
			w.taskTool(c, message.ID, tool, live, live.changes)
		}
	}
}

func (w *roundtableWindow) taskTool(c *ui.Context, taskID string, tool roundtable.ToolRecord, live *agentTool, changes []agentChange) {
	key := taskID + "/" + tool.ID
	title := cmp.Or(tool.Title, tool.Kind, locales.AgentTools())
	label := title + " · " + agentStatusLabel(tool.Status)
	ui.Row(c.Key(key)).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(4).Children(func() {
		button := ui.Button(c.Key("toggle"), label).Grow(1).MinWidth(0).Expanded(w.openTool == key)
		if live != nil {
			button.Label(title)
		}
		if button.Clicked() {
			if w.openTool == key {
				w.openTool = ""
			} else {
				w.openTool = key
			}
		}
		if live != nil && live.text != "" && roundtableIconButton(c, "copy", iconCopy, locales.AgentCopyToolOutput()+": "+title, w.pal).Clicked() {
			c.WriteClipboard(live.text)
		}
	})
	if w.openTool != key {
		return
	}
	if tool.Detail != "" {
		ui.Text(c.Key(key+"/detail"), tool.Detail).FillWidth().MinWidth(0).Font(w.a.settings.FontFamily).FontSize(11.5).TextColor(w.pal.muted).Selectable()
	}
	if tool.Note != "" {
		ui.Text(c.Key(key+"/note"), tool.Note).FillWidth().MinWidth(0).FontSize(11.5).TextColor(w.pal.muted).Selectable()
	}
	if live != nil && live.text != "" {
		preview, clipped := agentVisibleText(live.text, 8*1024)
		ui.Text(c.Key(key+"/output"), preview).FillWidth().MinWidth(0).Font(w.a.settings.FontFamily).FontSize(11.5).TextColor(w.pal.muted).Selectable()
		if clipped {
			ui.Text(c, locales.AgentMoreOutput()).FontSize(11).TextColor(w.pal.muted)
		}
	}
	if !w.meeting.readOnly {
		for index, change := range changes {
			if ui.Button(c.Key(key+"/review/"+strconv.Itoa(index)), locales.AgentReviewChange()+" · "+w.a.relPath(change.path)).Clicked() {
				w.a.post(func() { w.a.reviewAgentChange(change) })
			}
		}
	}
}

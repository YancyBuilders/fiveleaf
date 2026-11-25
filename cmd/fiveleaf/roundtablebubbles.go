package main

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func meetingCanvas(p *palette) ui.Color {
	return mix(p.editor, p.accent, 0.035)
}

func meetingInk(p *palette, speaker meetingSpeaker) ui.Color {
	switch speaker.ID {
	case "":
		return p.muted
	case "preset:test", "preset:review", "preset:compatibility":
		return p.changed
	case "preset:implement", "preset:explore", "preset:document":
		return p.added
	case "preset:requirements", "preset:diagnose":
		return p.modified
	}
	return p.fill
}

func meetingDot(c *ui.Context, p *palette, speaker meetingSpeaker) {
	ui.Box(c.Key("role-dot")).Size(6, 6).Shrink(0).Radius(3).Background(meetingInk(p, speaker))
}

func (w *roundtableWindow) chatSpeaker(id, name string) meetingSpeaker {
	if index := slices.IndexFunc(w.meeting.speakers, func(s meetingSpeaker) bool { return s.ID == id }); index >= 0 {
		return w.meeting.speakers[index]
	}
	return meetingSpeaker{Participant: roundtable.Participant{ID: id, Name: cmp.Or(name, id)}}
}

func meetingDay(c *ui.Context, p *palette, when time.Time) {
	label := when.Format("2006-01-02")
	now := c.Now().Local()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if day := dayOf(when, today); !when.After(now) && (day == sessionsToday || day == sessionsYesterday) {
		label = day.name()
	}
	ui.Text(c.Key("date:"+label), label).AlignSelf(ui.Center).FontSize(11).FontFeatures("tnum").TextColor(p.muted).Padding(2, 0)
}

func (w *roundtableWindow) messageBubble(c *ui.Context, message roundtable.Message, human bool) {
	p := w.pal
	question := human && !roundtableHasMedia(message.Text) && !strings.Contains(message.Text, "```") && !strings.Contains(message.Text, "~~~")
	material := materialOf(c)
	bubble := ui.Column(c.Key("bubble")).FillWidth().MinWidth(0).Padding(9, 12).Radius(8).Border(1, p.border).Background(material.insetColor(p.editor))
	if human && material.mica() {
		bubble.BorderColor(p.accent.Alpha(0.26)).Background(material.insetColor(p.editor.Mix(p.accent, 0.20)))
	} else if question {
		bubble.BorderWidth(0).Background(p.fill)
	} else if human {
		bubble.BorderColor(p.accentDim)
	}
	bubble.Children(func() {
		if task := message.Task; task != nil {
			if task.Status != roundtable.Completed {
				w.taskLink(c, message, taskStatusLabel(task.Status))
				return
			}
			if task.NoAnswer {
				w.taskLink(c, message, locales.RoundtableToolOnly())
				return
			}
		}
		w.messageReading(c, bubble, message, question && !material.mica())
		if task := message.Task; task != nil && task.MemoryNotes > 0 {
			w.messageMemoryChip(c, message, locales.RoundtableMemoryChip(task.MemoryNotes))
		}
	})
}

func (w *roundtableWindow) messageReading(c *ui.Context, body ui.Element, message roundtable.Message, question bool) {
	ink := w.pal.text
	if question {
		ink = w.pal.fillText
	}
	preview := ui.Local(body, "preview", func() string { return meetingReadingPreview(message.Text) })
	expanded := ui.Local(body, "expanded", func() bool { return false })
	if *preview != "" && !w.showFullReplies && !*expanded {
		ui.Text(c, *preview).FillWidth().MinWidth(0).FontSize(w.a.settings.FontSize).TextColor(ink).LineHeight(1.5).MaxLines(4).Selectable()
	} else if question {
		w.questionMarkdown(c, message.ID, message.Text)
	} else {
		w.messageMarkdown(c, message.ID, message.Text)
	}
	if *preview == "" || w.showFullReplies {
		return
	}
	label := locales.RoundtableFullResult()
	if *expanded {
		label = locales.RoundtableCollapse()
	}
	button := ui.ButtonBase(c.Key("expand")).Label(label).Expanded(*expanded).AlignSelf(ui.Start).Margin(6, 0, 0, 0).
		Radius(4).Padding(3, 5).Gap(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if button.Hovered() || button.FocusVisible() {
		button.Background(ink.Alpha(0.11))
	}
	button.Children(func() {
		icon := ui.Icon(c, iconChevronDown).FontSize(12).TextColor(ink)
		if *expanded {
			icon.Rotate(180)
		}
		ui.Text(c, label).FontSize(11.5).FontWeight(550).TextColor(ink)
	})
	if button.Clicked() {
		*expanded = !*expanded
	}
}

func (w *roundtableWindow) messageMemoryChip(c *ui.Context, message roundtable.Message, label string) {
	p, task := w.pal, message.Task
	chip := ui.ButtonBase(c.Key("memory-evidence")).Label(label).Tooltip(locales.RoundtableMemoryUsed(task.MemoryNotes, task.MemoryTurns)).
		AlignSelf(ui.Start).MaxWidthPercent(100).MinWidth(0).AlignItems(ui.Center).Gap(4).Padding(2, 5).Margin(5, 0, 0, 0).Radius(4).Background(p.modified.Alpha(0.09)).Cursor(ui.CursorPointer)
	if chip.Hovered() || chip.FocusVisible() {
		chip.Background(p.modified.Alpha(0.16))
	}
	chip.Children(func() {
		ui.Icon(c, meetingMemoryIcon).FontSize(11).TextColor(p.modifiedFg).Shrink(0)
		ui.Text(c, label).FontSize(10.5).TextColor(p.modifiedFg).MinWidth(0)
	})
	if chip.Clicked() {
		w.openMeetingMessage(message)
	}
}

func (w *roundtableWindow) questionMarkdown(c *ui.Context, key, text string) {
	pal := *w.pal
	pal.editor, pal.text, pal.accent = pal.fill, pal.fillText, pal.fillText
	pal.muted, pal.accentDim, pal.border = pal.fillText, pal.fillText.Alpha(0.68), pal.fillText.Alpha(0.35)
	pal.panel, pal.surfaceHover, pal.hover = mix(pal.fill, pal.fillText, 0.04), mix(pal.fill, pal.fillText, 0.10), mix(pal.fill, pal.fillText, 0.06)
	agentMarkdown(c, key, text, &pal, w.a.settings, false)
}

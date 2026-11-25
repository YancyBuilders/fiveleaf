package main

import (
	"strings"
	"unicode/utf8"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

var meetingAtIcon = uiicons.AtSign()

func (w *roundtableWindow) recipientProblem(draft string) string {
	if w.target == "" && w.mentions.selectedToken != "" {
		return locales.RoundtableTargetUnavailable()
	}
	available, moderator := false, false
	for _, speaker := range w.meeting.speakers {
		if speaker.problem == "" {
			if w.target == speaker.ID {
				return ""
			}
			available = true
			moderator = moderator || speaker.ID == roundtable.ModeratorID
		}
	}
	if w.target != "" {
		return locales.RoundtableTargetUnavailable()
	}
	if w.meeting.group.Options.HumanChair && !strings.HasPrefix(strings.TrimSpace(draft), "@") {
		return ""
	}
	if !moderator && !strings.HasPrefix(strings.TrimSpace(draft), "@") {
		return locales.RoundtableModeratorUnavailable()
	}
	if !available {
		return locales.RoundtableNoMembers()
	}
	return ""
}

func (w *roundtableWindow) addressMember(speaker meetingSpeaker) {
	if w.mentions.selectedToken != "" {
		if start := meetingMentionOffset(w.draft, w.mentions.selectedToken); start >= 0 {
			end := start + len(w.mentions.selectedToken)
			if strings.HasPrefix(w.draft[end:], " ") {
				end++
			}
			w.draft = w.draft[:start] + w.draft[end:]
		}
	}
	w.chooseMention(meetingMention{}, speaker)
	w.mentions.restore, w.focusDraft = utf8.RuneCountInString(w.draft), true
}

func (w *roundtableWindow) insertMention(input ui.Element) {
	anchor, caret := input.TextAnchorCaret()
	if _, ok := meetingMentionAt(w.draft, anchor, caret); ok {
		w.mentions.dismissed = false
		input.Focus()
		return
	}
	text := []rune(w.draft)
	start, end := min(anchor, caret), max(anchor, caret)
	prefix := "@"
	if start > 0 && !meetingAtBoundary(text[start-1]) {
		prefix = " @"
	}
	suffix := string(text[end:])
	if end < len(text) && meetingAliasRune(text[end]) {
		suffix = " " + suffix
	}
	w.draft = string(text[:start]) + prefix + suffix
	w.mentions.restore, w.mentions.dismissed = start+utf8.RuneCountInString(prefix), false
}

func meetingComposeAction(c *ui.Context, key, label string, icon *ui.SVG, p *palette) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Size(28, 26).KeepFocus().Shrink(0).Radius(6).Center().Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	ink := p.muted
	if button.Hovered() || button.FocusVisible() {
		button.Background(p.hover)
		ink = p.accent
	}
	button.Children(func() { ui.Icon(c, icon).FontSize(15).TextColor(ink) })
	return button
}

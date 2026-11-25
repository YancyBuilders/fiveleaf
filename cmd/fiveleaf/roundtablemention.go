package main

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingMention struct {
	start, end int
	query      string
}

type meetingMentionPicker struct {
	active                      meetingMention
	open, dismissed             bool
	index, restore              int
	selectedToken, checkedDraft string
}

func meetingAliasRune(char rune) bool {
	return unicode.IsLetter(char) || unicode.IsNumber(char) || strings.ContainsRune("_:-.", char)
}

func meetingAtBoundary(before rune) bool {
	return !(before < utf8.RuneSelf && (meetingAliasRune(before) || before == '+' || before == '@'))
}

func meetingMentionAt(text string, anchor, caret int) (meetingMention, bool) {
	runes := []rune(text)
	if anchor != caret || caret < 1 || caret > len(runes) {
		return meetingMention{}, false
	}
	start := caret
	for start > 0 && meetingAliasRune(runes[start-1]) {
		start--
	}
	if start == 0 || runes[start-1] != '@' || start > 1 && !meetingAtBoundary(runes[start-2]) {
		return meetingMention{}, false
	}
	end := caret
	for end < len(runes) && meetingAliasRune(runes[end]) {
		end++
	}
	return meetingMention{start: start - 1, end: end, query: string(runes[start:caret])}, true
}

func (token meetingMention) replace(text, alias string) (string, int) {
	runes := []rune(text)
	prefix, suffix := string(runes[:token.start])+"@"+alias, runes[token.end:]
	if len(suffix) > 0 && unicode.IsSpace(suffix[0]) {
		return prefix + string(suffix), utf8.RuneCountInString(prefix) + 1
	}
	prefix += " "
	return prefix + string(suffix), utf8.RuneCountInString(prefix)
}

func meetingHasMention(text, token string) bool {
	return meetingMentionOffset(text, token) >= 0
}

func meetingMentionOffset(text, token string) int {
	if token == "" {
		return -1
	}
	for start := 0; start < len(text); {
		offset := strings.Index(text[start:], token)
		if offset < 0 {
			break
		}
		at := start + offset
		before, _ := utf8.DecodeLastRuneInString(text[:at])
		after, _ := utf8.DecodeRuneInString(text[at+len(token):])
		if meetingAtBoundary(before) && !meetingAliasRune(after) {
			return at
		}
		start = at + len(token)
	}
	return -1
}

func (w *roundtableWindow) mentionChoices(query string) []meetingSpeaker {
	var found []meetingSpeaker
	query = strings.ToLower(query)
	for _, speaker := range w.meeting.speakers {
		matches := func(value string) bool { return strings.Contains(strings.ToLower(value), query) }
		if speaker.problem == "" && (matches(speaker.Name) || matches(speaker.ID) || slices.ContainsFunc(speaker.provider.config.Capabilities, matches)) {
			found = append(found, speaker)
		}
	}
	return found
}

func (w *roundtableWindow) chooseMention(token meetingMention, speaker meetingSpeaker) {
	alias := speaker.Name
	if w.meeting.memberLabel(speaker) != alias {
		alias = speaker.ID
	}
	w.draft, w.mentions.restore = token.replace(w.draft, alias)
	w.target, w.mentions.selectedToken, w.mentions.open = speaker.ID, "@"+alias, false
	w.mentions.checkedDraft = w.draft
}

func (w *roundtableWindow) mentionPicker(c *ui.Context, input ui.Element) {
	picker := &w.mentions
	if picker.restore > 0 {
		input.Focus().SetTextSelection(picker.restore, picker.restore)
		picker.restore = 0
	}
	w.syncMentionTarget(w.draft)
	anchor, caret := input.TextAnchorCaret()
	token, ok := meetingMentionAt(w.draft, anchor, caret)
	var items []meetingSpeaker
	text := w.draft
	composing := input.Composing()
	input.OnTextKey(func(key ui.TextKey) bool {
		if composing {
			return key.Key == ui.KeyEnter
		}
		if picker.open && text == key.Text() && key.Mods == 0 {
			switch key.Key {
			case ui.KeyUp, ui.KeyDown:
				if len(items) != 0 {
					delta := 1
					if key.Key == ui.KeyUp {
						delta = -1
					}
					picker.index = (picker.index + len(items) + delta) % len(items)
				}
				return true
			case ui.KeyEnter, ui.KeyTab:
				if len(items) != 0 {
					w.chooseMention(token, items[picker.index])
					key.SetText(w.draft)
				}
				return true
			case ui.KeyEscape:
				picker.open, picker.dismissed = false, true
				return true
			}
		}
		if !chatSubmitKey(key) {
			return false
		}
		draft := key.Text()
		w.syncMentionTarget(draft)
		if strings.TrimSpace(draft) == "" || w.recipientProblem(draft) != "" {
			return true
		}
		w.draft = draft
		w.send()
		key.SetText(w.draft)
		return true
	})
	if !ok || composing || !input.Focused() && !picker.open {
		picker.open, picker.dismissed = false, false
		return
	}
	if picker.active != token {
		picker.active, picker.index, picker.dismissed = token, 0, false
	}
	if picker.dismissed {
		picker.open = false
		return
	}
	picker.open = true
	items = w.mentionChoices(token.query)
	picker.index = max(0, min(picker.index, len(items)-1))
	_, height := c.Size()
	panel := ui.PopoverBase(c.Key("meeting-mentions"), input, &picker.open, func(panel ui.Element) {
		w.meetingPickerPanel(panel, input).Role(ui.RoleList).KeepFocus().Label(locales.RoundtableMembers()).Children(func() {
			ui.Row(c).FillWidth().Padding(5, 6, 7, 6).AlignItems(ui.Center).Gap(7).Children(func() {
				ui.Icon(c, meetingAtIcon).FontSize(14).TextColor(w.pal.accent)
				ui.Text(c, locales.RoundtableMentionSearch()).FontSize(11.5).TextColor(w.pal.muted).Grow(1).MinWidth(0).SingleLine()
				ui.Text(c, "Esc").FontSize(10).TextColor(w.pal.muted).Padding(0, 3).Radius(3).Border(1, w.pal.border)
			})
			if len(items) == 0 {
				ui.Text(c, locales.RoundtableMentionEmpty()).Padding(8).FontSize(12).TextColor(w.pal.muted)
			}
			ui.Scroll(c).MaxHeight(min(240, max(42, height/2-70))).Children(func() {
				for index, item := range items {
					label := w.meeting.memberLabel(item)
					detail := strings.Join(item.provider.config.Capabilities, " · ")
					if detail == "" {
						detail = item.detail()
					}
					row := ui.ButtonBase(c.Key(item.ID)).Role(ui.RoleListItem).Selected(index == picker.index).KeepFocus().Label("@"+label).Description(detail).FillWidth().Height(42).Gap(6).Padding(0, 6).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Justify(ui.Start)
					if index == picker.index {
						row.Background(w.pal.accent.Alpha(0.12)).ScrollIntoView()
					} else if row.Hovered() {
						row.Background(w.pal.hover)
					}
					row.Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								meetingDot(c, w.pal, item)
								ui.Text(c, label).FontSize(12.5).FontWeight(550).TextColor(w.pal.text).SingleLine().MinWidth(0)
							})
							if detail != "" {
								ui.Text(c, detail).FontSize(10.5).TextColor(w.pal.muted).SingleLine().MinWidth(0)
							}
						})
					})
					if row.Clicked() {
						w.chooseMention(token, item)
					}
				}
			})
			if len(items) > 0 {
				ui.Text(c, locales.RoundtableMentionKeys()).FontSize(10.5).TextColor(w.pal.muted).Padding(6, 6, 1, 6).SingleLine()
			}
		})
	})
	if panel.Dismissed() {
		picker.open, picker.dismissed = false, true
	}
}

func (w *roundtableWindow) meetingPickerPanel(panel, input ui.Element) ui.Element {
	width := input.Bounds().W
	if width <= 0 {
		width = 260
	}
	return panel.AttachTo(input, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 5, 0).
		Width(min(322, width)).Padding(5).Radius(10).Border(1, w.pal.border).Background(w.pal.editor)
}

func (w *roundtableWindow) syncMentionTarget(draft string) {
	picker := &w.mentions
	if picker.selectedToken == "" || picker.checkedDraft == draft {
		return
	}
	fragment := strings.TrimSpace(draft)
	if offset := meetingMentionOffset(draft, picker.selectedToken); offset >= 0 {
		fragment = draft[offset:]
	} else if offset := meetingMentionOffset(picker.checkedDraft, picker.selectedToken); offset >= 0 {
		if tail, sameStart := strings.CutPrefix(draft, picker.checkedDraft[:offset]); sameStart && strings.HasPrefix(tail, "@") {
			fragment = tail
		}
	}
	picker.checkedDraft = draft
	old := roundtable.MentionToken(fragment, strings.TrimPrefix(picker.selectedToken, "@"))
	id, token := w.meeting.room.MentionBinding(fragment)
	if len(token) > len(old) {
		w.target, picker.selectedToken = id, token
	} else if old == "" {
		w.target, picker.selectedToken = "", ""
	}
}

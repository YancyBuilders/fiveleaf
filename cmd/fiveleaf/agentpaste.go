package main

import (
	"slices"
	"strings"
	"unicode/utf8"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type chatDraftPaste struct {
	text, preview string
	lines         int
}

func (a *app) pasteChatDraft(chat *agentChat, text string) bool {
	if a.pasteChatAttachment(chat, text) {
		return true
	}
	lines := strings.Count(text, "\n") + 1
	if lines <= 20 && len(text) <= chatDraftMeasureBytes {
		return false
	}
	if chat.draft == "" && strings.HasPrefix(text, "/") {
		name, _ := splitSlashLine(text)
		if name == slashModel || name == slashMode || name == slashThinking || piTerminalPage(name) {
			return false
		}
	}
	anchor, caret := chat.draftState.selection[0], chat.draftState.selection[1]
	if anchor != caret || caret != utf8.RuneCountInString(chat.draft) {
		return false
	}
	a.postChatAction(chat, func() {
		draft := &chat.sources.draft
		bytes := len(text)
		for _, paste := range draft.pastes {
			bytes += len(paste.text) + 2
		}
		if bytes > agentTextLimit {
			chat.err = locales.AgentContextTooLarge()
			return
		}
		preview, clipped := agentVisibleText(text, chatDraftMeasureBytes)
		if clipped {
			preview += "\n…"
		}
		draft.pastes = append(draft.pastes, &chatDraftPaste{text: text, preview: preview, lines: lines})
	})
	return true
}

func (a *app) agentPasteView(c *ui.Context, chat *agentChat) {
	pastes := chat.sources.draft.pastes
	if len(pastes) == 0 {
		return
	}
	palette := a.pal
	metrics := palette.metricsOr()
	ui.ScrollHorizontal(c.Key("draft-pastes")).FillWidth().MinWidth(0).Height(26 + metrics.gap).Shrink(0).Children(func() {
		ui.Row(c).Gap(metrics.gap).AlignItems(ui.Center).Padding(metrics.gap/2, metrics.inset).Children(func() {
			for _, paste := range pastes {
				label := locales.AgentPasteText(paste.lines)
				ui.Row(c.Key(paste)).Height(26).Shrink(0).AlignItems(ui.Center).Gap(metrics.gap/2).
					Padding(0, metrics.gap/2).Radius(metrics.radius).Background(palette.surfaceHover).Children(func() {
					open := chat.expandedPaste == paste
					chevron := iconChevronRight
					if open {
						chevron = iconChevronDown
					}
					toggle := ui.ButtonBase(c.Key("preview")).Label(label).Expanded(open).KeepFocus().Cursor(ui.CursorPointer).
						AlignItems(ui.Center).Gap(metrics.gap).Padding(0, metrics.gap/2)
					toggle.Children(func() {
						ui.Icon(c, chevron).Size(12, 12).TextColor(palette.muted)
						ui.Text(c, label).FontSize(metrics.textSmall).TextColor(palette.text).SingleLine()
					})
					if toggle.Clicked() {
						a.postChatAction(chat, func() {
							if chat.expandedPaste == paste {
								chat.expandedPaste = nil
							} else if slices.Contains(chat.sources.draft.pastes, paste) {
								chat.expandedPaste = paste
							}
						})
					}
					remove := locales.AgentPasteRemove()
					if a.iconButton(c.Key("remove"), iconX, remove, false).Size(20, 20).KeepFocus().Tooltip(remove).Clicked() {
						a.postChatAction(chat, func() {
							if index := slices.Index(chat.sources.draft.pastes, paste); index >= 0 {
								chat.sources.draft.pastes = slices.Delete(chat.sources.draft.pastes, index, index+1)
							}
							if chat.expandedPaste == paste {
								chat.expandedPaste = nil
							}
						})
					}
				})
			}
		})
	})
	if paste := chat.expandedPaste; paste != nil && slices.Contains(pastes, paste) {
		_, windowHeight := c.Size()
		ui.Row(c.Key("paste-preview")).FillWidth().MinWidth(0).AlignItems(ui.Start).Gap(metrics.gap).
			Padding(metrics.gap, metrics.inset).Children(func() {
			ui.ScrollBoth(c.Key("text")).Grow(1).MinWidth(0).MaxHeight(min(110, windowHeight/6)).
				Padding(metrics.pad, metrics.inset).Radius(metrics.radius).Background(palette.surfaceHover).Children(func() {
				ui.Text(c.Key(paste), paste.preview).Font(a.settings.FontFamily).FontSize(metrics.textSmall).
					TextColor(palette.text).Selectable().NoWrap()
			})
			label := locales.AgentPasteEdit()
			if a.iconButton(c.Key("edit"), iconPencil, label, false).Size(26, 26).Shrink(0).Tooltip(label).Clicked() {
				a.postChatAction(chat, func() {
					index := slices.Index(chat.sources.draft.pastes, paste)
					if index < 0 || chat.expandedPaste != paste {
						return
					}
					prefix := chatSourceDraft{pastes: chat.sources.draft.pastes[:index+1]}
					chat.draft = prefix.promptText(chat.draft)
					chat.sources.draft.pastes = slices.Delete(chat.sources.draft.pastes, 0, index+1)
					chat.expandedPaste = nil
					caret := utf8.RuneCountInString(chat.draft)
					chat.draftState = agentDraftState{text: chat.draft, selection: [2]int{caret, caret}, selectionPending: true}
					chat.focusNext = true
				})
			}
		})
	}
}

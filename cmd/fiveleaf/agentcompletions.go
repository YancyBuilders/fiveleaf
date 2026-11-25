package main

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func advertisedChatCommand(commands []acp.Command, text string) bool {
	name, ok := strings.CutPrefix(text, "/")
	if !ok {
		return false
	}
	if end := strings.IndexFunc(name, unicode.IsSpace); end >= 0 {
		name = name[:end]
	}
	return slices.ContainsFunc(commands, func(command acp.Command) bool { return command.Name == name })
}

type chatCompletionToken struct {
	trigger    rune
	start, end int
	query      string
	setting    string
}

func chatCompletionAt(text string, anchor, caret int) chatCompletionToken {
	if anchor != caret || caret <= 0 || !strings.ContainsAny(text, "@/") {
		return chatCompletionToken{}
	}
	runes := []rune(text)
	if caret > len(runes) {
		return chatCompletionToken{}
	}
	for start := caret - 1; start >= 0 && !unicode.IsSpace(runes[start]); start-- {
		trigger := runes[start]
		if trigger == '@' && (start == 0 || meetingAtBoundary(runes[start-1]) && runes[start-1] != '/') || trigger == '/' && start == 0 {
			end := caret
			for end < len(runes) && !unicode.IsSpace(runes[end]) {
				end++
			}
			return chatCompletionToken{trigger: trigger, start: start, end: end, query: string(runes[start+1 : caret])}
		}
	}
	return chatCompletionToken{}
}

func (token chatCompletionToken) insert(text, value string) (string, int) {
	runes := []rune(text)
	prefix := string(runes[:token.start]) + value
	caret := utf8.RuneCountInString(prefix)
	if token.end == len(runes) || !unicode.IsSpace(runes[token.end]) {
		prefix += " "
	}
	return prefix + string(runes[token.end:]), caret + 1
}

type chatSuggestion struct {
	value, detail, display, setting string
	current                         bool
}

type chatSuggestionPicker struct {
	token                  chatCompletionToken
	files                  projectFileSearch
	list                   ui.ListState
	index, restore         int
	open, dismissed, moved bool
}

func (a *app) chatSuggestions(c *ui.Context, chat *agentChat, input ui.Element) func(ui.TextKey) bool {
	picker := ui.Local(input, "suggestions", func() chatSuggestionPicker { return chatSuggestionPicker{} })
	if picker.restore > 0 {
		input.Focus().SetTextSelection(picker.restore, picker.restore)
		picker.restore = 0
	}
	if input.Composing() || !input.Focused() {
		picker.open = false
		return nil
	}
	anchor, caret := input.TextAnchorCaret()
	text := chat.draft
	token, settingValue := chat.slashValueToken(text, anchor, caret)
	if !settingValue {
		token = chatCompletionAt(text, anchor, caret)
	}
	changed := token != picker.token
	if changed {
		picker.token, picker.index, picker.dismissed, picker.moved = token, 0, false, true
	}
	if token.trigger == 0 || picker.dismissed || chat.modelPicker.open {
		picker.open = false
		return nil
	}
	var items []chatSuggestion
	title, empty, icon := locales.AgentCommands(), locales.AgentNoMatchingCommands(), iconCommand
	if token.trigger == '@' {
		title, empty, icon = locales.QuickSearchFiles(), locales.QuickNoFiles(), iconFileText
		if !picker.open || a.indexRead.Stale() {
			a.refreshIndex()
		}
		picker.files.query = token.query
		picker.files.filter(a.index)
		for _, match := range picker.files.results {
			items = append(items, chatSuggestion{value: "@" + picker.files.files[match.Index]})
		}
		if len(items) == 0 && a.indexRead.Running() {
			empty = locales.QuickIndexing()
		}
	} else if settingValue {
		title, empty = slashSettingLabel(token.setting), locales.AgentSlashNoValues()
		items = filterSlashSuggestions(chat.slashValues(token.setting), token.query)
		if changed && token.query == "" {
			picker.index = slices.IndexFunc(items, func(item chatSuggestion) bool { return item.current })
		}
	} else if chat.piTerminalOnly("/" + token.query) {
		empty = locales.AgentSlashPiTerminal("/" + token.query)
	} else {
		items = chat.slashCommands(token.query)
		if len(items) == 0 && token.query == "" {
			empty = locales.AgentNoCommands()
			if chat.connecting {
				empty = locales.AgentConnecting()
			}
		}
	}
	picker.index = max(0, min(picker.index, len(items)-1))
	folder, session, generation, client := a.folderKey(), chat.sessionID, chat.generation, chat.client
	choose := func(item chatSuggestion) {
		if a.loadsClosed || a.folderKey() != folder || !a.chatShown(chat) || chat.draft != text {
			return
		}
		if token.trigger == '/' && (chat.sessionID != session || chat.generation != generation || chat.client != client) {
			return
		}
		switch {
		case settingValue:
			if !a.applySlashValue(chat, token.setting, item.value) {
				return
			}
			picker.open, picker.dismissed = false, true
			return
		case item.setting != "":
			if !chat.offersSlashSetting(item.setting) || chat.requestInFlight() {
				return
			}
			prefix := item.value + " "
			tail := string([]rune(text)[token.end:])
			chat.draft = prefix + strings.TrimLeft(tail, " \t")
			picker.restore = utf8.RuneCountInString(prefix)
			picker.open = false
			return
		case token.trigger == '/':
			if !advertisedChatCommand(chat.commands, item.value) {
				return
			}
		case token.trigger == '@':
			path := chatWorkspaceFile(a.root, filepath.FromSlash(item.value[1:]))
			if path == "" {
				chat.err = locales.AgentFileUnavailable(item.value[1:])
				return
			}
			a.addChatDetailFiles(chat, []string{path})
		}
		chat.draft, picker.restore = token.insert(text, item.value)
		picker.open = false
	}
	picker.open = true
	picker.show(c, input, a, title, empty, icon, items, choose)
	return func(key ui.TextKey) bool {
		if !picker.open || key.Mods != 0 {
			return false
		}
		switch key.Key {
		case ui.KeyEscape:
			picker.open, picker.dismissed = false, true
		case ui.KeyUp, ui.KeyDown:
			if count := len(items); count > 0 {
				delta := 1
				if key.Key == ui.KeyUp {
					delta = -1
				}
				picker.index, picker.moved = (picker.index+delta+count)%count, true
			}
		case ui.KeyEnter, ui.KeyTab:
			if key.Text() == text && len(items) > 0 {
				choose(items[picker.index])
				key.SetText(chat.draft)
			} else if len(items) == 0 {
				picker.open, picker.dismissed = false, true
			}
		default:
			return false
		}
		return true
	}
}

func (picker *chatSuggestionPicker) show(c *ui.Context, input ui.Element, a *app, title, empty string, icon *ui.SVG, items []chatSuggestion, choose func(chatSuggestion)) {
	pal, metrics := a.pal, a.pal.metricsOr()
	bounds := input.Bounds()
	panel := ui.PopoverBase(c.Key("chat-suggestions"), input, &picker.open, func(panel ui.Element) {
		a.floating(c, panel.Width(min(440, bounds.W)).Padding(4).Radius(metrics.radiusCard), false).
			AttachTo(input, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Role(ui.RoleList).Label(title).KeepFocus().Children(func() {
			ui.Row(c).FillWidth().Padding(4, 7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).Grow(1).SingleLine().FontSize(metrics.textSmall).TextColor(pal.muted)
				if picker.token.trigger == '/' && len(items) > 0 {
					ui.Text(c, strconv.Itoa(picker.index+1)+" / "+strconv.Itoa(len(items))).SingleLine().FontSize(metrics.textSmall).FontFeatures("tnum").TextColor(pal.muted)
				}
			})
			if len(items) == 0 {
				ui.Text(c, empty).FontSize(metrics.textBody).TextColor(pal.muted).Padding(6, 7)
				return
			}
			const rowHeight = 30
			if picker.moved {
				picker.list.ScrollIntoView(picker.index)
			}
			ui.List(c.Key("matches"), &picker.list, len(items), func(index int) {
				item := items[index]
				label := cmp.Or(item.display, item.value)
				row := ui.ButtonBase(c.Key(item.value)).FillWidth().MinWidth(0).Height(rowHeight).Padding(0, 6).Gap(6).AlignItems(ui.Center).Justify(ui.Start).
					KeepFocus().Role(ui.RoleListItem).Label(label).Description(item.detail).Selected(index == picker.index).Radius(metrics.radius)
				if index == picker.index {
					row.Background(pal.selectedDim)
				} else if row.Hovered() {
					row.Background(pal.hover)
				}
				row.Tooltip(strings.TrimSpace(label + "\n" + item.detail)).Children(func() {
					mark := icon
					if item.current {
						mark = iconCheck
					} else if item.setting != "" {
						mark = iconSettings
					} else if slashSkill(item.value) {
						mark = iconSparkles
					}
					ui.Icon(c, mark).FontSize(13).TextColor(pal.muted).Shrink(0)
					ui.Text(c, label).MinWidth(0).SingleLine().FontSize(metrics.textBody).TextColor(pal.text)
					if item.detail != "" {
						ui.Text(c, item.detail).Basis(0).Grow(1).MinWidth(0).SingleLine().FontSize(metrics.textSmall).TextColor(pal.muted)
					}
				})
				if row.Clicked() {
					choose(item)
				}
			}).FillWidth().Height(min(float32(rowHeight*len(items)), 235, max(32, bounds.Y-46)))
			picker.moved = false
		})
	})
	if panel.Dismissed() {
		picker.open, picker.dismissed = false, true
	}
}

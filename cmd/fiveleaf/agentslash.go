package main

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"fiveleaf/locales"
)

const (
	slashModel    = "/model"
	slashThinking = "/thinking"
	slashMode     = "/mode"
)

func slashSkill(value string) bool {
	return strings.HasPrefix(value, "/skill:") || strings.HasPrefix(value, "/$")
}

func piTerminalPage(name string) bool {
	return name == "/settings" || name == "/tree" || name == "/scoped-models"
}

func (chat *agentChat) piTerminalOnly(name string) bool {
	return chat.adapterName == "pi-acp" && piTerminalPage(name) && !advertisedChatCommand(chat.commands, name)
}

func (chat *agentChat) offersSlashSetting(name string) bool {
	if chat.client == nil || chat.sessionID == "" || advertisedChatCommand(chat.commands, name) {
		return false
	}
	switch name {
	case slashModel:
		return len(chat.models) > 0
	case slashThinking:
		return len(agentThinkingModes(chat)) > 0
	case slashMode:
		return len(agentGenericModes(chat)) > 0
	}
	return false
}

func slashSettingLabel(name string) string {
	switch name {
	case slashModel:
		return locales.AgentModel()
	case slashThinking:
		return locales.AgentThinking()
	default:
		return locales.AgentMode()
	}
}

func (chat *agentChat) slashCommands(query string) []chatSuggestion {
	var controls, commands, skills []chatSuggestion
	for _, name := range []string{slashModel, slashThinking, slashMode} {
		if chat.offersSlashSetting(name) {
			controls = append(controls, chatSuggestion{value: name, detail: slashSettingLabel(name), setting: name})
		}
	}
	for _, command := range chat.commands {
		item := chatSuggestion{value: "/" + command.Name, detail: command.Description}
		if command.Hint != "" {
			item.detail = strings.TrimSpace(item.detail + "  " + command.Hint)
		}
		if slashSkill(item.value) {
			skills = append(skills, item)
		} else {
			commands = append(commands, item)
		}
	}
	return filterSlashSuggestions(slices.Concat(controls, commands, skills), query)
}

func filterSlashSuggestions(items []chatSuggestion, query string) []chatSuggestion {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return items
	}
	var exact, prefixes, rest []chatSuggestion
	for _, item := range items {
		name := strings.ToLower(strings.TrimPrefix(item.value, "/"))
		switch {
		case name == query:
			exact = append(exact, item)
		case strings.HasPrefix(name, query):
			prefixes = append(prefixes, item)
		case strings.Contains(name, query) || strings.Contains(strings.ToLower(item.detail), query) || strings.Contains(strings.ToLower(item.display), query):
			rest = append(rest, item)
		}
	}
	return slices.Concat(exact, prefixes, rest)
}

func (chat *agentChat) slashValues(name string) []chatSuggestion {
	var result []chatSuggestion
	if name == slashModel {
		for _, model := range chat.models {
			if model.ID != "" {
				detail := model.Description
				if model.Name != model.ID {
					detail = strings.TrimSpace(model.Name + " " + detail)
				}
				result = append(result, chatSuggestion{value: model.ID, detail: detail, current: model.ID == chat.model()})
			}
		}
		return result
	}
	var modes []agentChoice
	var current string
	switch name {
	case slashThinking:
		modes, current = agentThinkingModes(chat), chat.mode
	case slashMode:
		modes, current = agentGenericModes(chat), agentGenericMode(chat)
	}
	for _, mode := range modes {
		item := chatSuggestion{value: mode.id, detail: mode.name, current: mode.id == current}
		if mode.id == "" {
			item.display, item.detail = cmp.Or(mode.name, locales.AgentSlashDefault()), ""
		} else if mode.id == mode.name {
			item.detail = ""
		}
		result = append(result, item)
	}
	return result
}

func (a *app) applySlashValue(chat *agentChat, name, value string) bool {
	if chat.requestInFlight() || !chat.offersSlashSetting(name) || !slices.ContainsFunc(chat.slashValues(name), func(item chatSuggestion) bool { return item.value == value }) {
		return false
	}
	draft := chat.draft
	applied := func() {
		if chat.draft == draft {
			_, chat.draft, _ = strings.Cut(draft, "\n")
		}
	}
	switch name {
	case slashModel:
		a.setChatModelSelection(chat, value, applied)
	case slashThinking:
		a.setChatModeSelection(chat, value, true, applied)
	case slashMode:
		a.setChatModeSelection(chat, value, false, applied)
	}
	return true
}

func splitSlashLine(text string) (name, values string) {
	if gap := strings.IndexFunc(text, unicode.IsSpace); gap >= 0 {
		return text[:gap], text[gap:]
	}
	return text, ""
}

func (chat *agentChat) slashValueToken(text string, anchor, caret int) (chatCompletionToken, bool) {
	if !strings.HasPrefix(text, "/") || anchor != caret {
		return chatCompletionToken{}, false
	}
	line, _, _ := strings.Cut(text, "\n")
	name, arguments := splitSlashLine(line)
	start := utf8.RuneCountInString(name)
	if arguments == "" || caret <= start || caret > utf8.RuneCountInString(line) || !chat.offersSlashSetting(name) {
		return chatCompletionToken{}, false
	}
	runes := []rune(line)
	return chatCompletionToken{trigger: '/', end: len(runes), query: strings.TrimSpace(string(runes[start:caret])), setting: name}, true
}

func (a *app) submitSlashSetting(chat *agentChat) bool {
	if !strings.HasPrefix(chat.draft, "/") {
		return false
	}
	line, _, _ := strings.Cut(chat.draft, "\n")
	name, arguments := splitSlashLine(line)
	if chat.piTerminalOnly(name) {
		chat.err = locales.AgentSlashPiTerminal(name)
		return true
	}
	if !chat.offersSlashSetting(name) {
		return false
	}
	value := strings.TrimSpace(arguments)
	if value == "" {
		if arguments == "" {
			chat.draft = name + " " + strings.TrimPrefix(chat.draft, line)
		}
		chat.focusNext = true
	} else if !a.applySlashValue(chat, name, value) {
		chat.err = locales.AgentSlashUnknownValue(value, name)
	}
	return true
}

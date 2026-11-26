package acp

import (
	"slices"
	"strings"
	"unicode"

	sdk "github.com/coder/acp-go-sdk"
)

type Command struct{ Name, Description, Hint string }

func sessionCommands(advertised []sdk.AvailableCommand) []Command {
	commands := make([]Command, 0, len(advertised))
	seen := make(map[string]bool, len(advertised))
	for _, item := range advertised {
		name := strings.TrimPrefix(item.Name, "/")
		if name == "" || seen[name] || strings.IndexFunc(name, unicode.IsSpace) >= 0 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			continue
		}
		seen[name] = true
		command := Command{Name: name, Description: item.Description}
		if item.Input != nil && item.Input.Unstructured != nil {
			command.Hint = item.Input.Unstructured.Hint
		}
		commands = append(commands, command)
	}
	return commands
}

func (c *Client) Commands(sessionID string) []Command {
	c.mu.Lock()
	defer c.mu.Unlock()
	if session := c.sessions[sessionID]; session != nil {
		return slices.Clone(session.commands)
	}
	return nil
}

// ModelState returns the session's advertised selector without sharing mutable
// option lists with the transport. A false result means selection is unavailable.
func (c *Client) ModelState(sessionID string) (ModelState, bool) {
	if c.ctx.Err() != nil {
		return ModelState{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return sessionModelState(c.sessions[sessionID])
}

func sessionModelState(s *sessionState) (ModelState, bool) {
	if s == nil {
		return ModelState{}, false
	}
	if option := modelConfigOption(s.configOptions); option != nil {
		state := ModelState{Current: string(option.CurrentValue)}
		appendOptions := func(options []sdk.SessionConfigSelectOption, group string) {
			for _, option := range options {
				choice := ModelChoice{ID: string(option.Value), Name: option.Name, Group: group}
				if option.Description != nil {
					choice.Description = *option.Description
				}
				state.Available = append(state.Available, choice)
			}
		}
		if option.Options.Ungrouped != nil {
			appendOptions(*option.Options.Ungrouped, "")
		}
		if option.Options.Grouped != nil {
			for _, group := range *option.Options.Grouped {
				appendOptions(group.Options, group.Name)
			}
		}
		return state, true
	}
	if s.legacyModels != nil {
		return ModelState{Current: s.currentModel, Available: slices.Clone(s.legacyChoices)}, true
	}
	return ModelState{}, false
}

// ModeState prefers the thinking selector when an agent advertises it, since
// its choices can change with the model after session/new.
func (c *Client) ModeState(sessionID string) (ModeState, bool) {
	if c.ctx.Err() != nil {
		return ModeState{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return sessionModeState(c.sessions[sessionID])
}

func sessionModeState(s *sessionState) (ModeState, bool) {
	if s == nil {
		return ModeState{}, false
	}
	if option := thinkingConfigOption(s.configOptions); option != nil {
		state := ModeState{Current: string(option.CurrentValue), Thinking: true}
		appendOptions := func(options []sdk.SessionConfigSelectOption) {
			for _, option := range options {
				choice := ModeChoice{ID: string(option.Value), Name: option.Name}
				if option.Description != nil {
					choice.Description = *option.Description
				}
				state.Available = append(state.Available, choice)
			}
		}
		if option.Options.Ungrouped != nil {
			appendOptions(*option.Options.Ungrouped)
		}
		if option.Options.Grouped != nil {
			for _, group := range *option.Options.Grouped {
				appendOptions(group.Options)
			}
		}
		for _, mode := range s.modes {
			name := strings.ToLower(strings.TrimSpace(mode.Name))
			if strings.HasPrefix(name, "thinking:") || strings.HasPrefix(name, "reasoning:") {
				continue
			}
			state.GenericAvailable = append(state.GenericAvailable, mode)
			if mode.ID == s.currentMode {
				state.GenericCurrent = s.currentMode
			}
		}
		return state, true
	}
	if len(s.modes) > 0 {
		return ModeState{Current: s.currentMode, Available: slices.Clone(s.modes)}, true
	}
	return ModeState{}, false
}

// UsageState returns only usage reported by the agent. Unknown usage must not be
// mistaken for an empty context window.
func (c *Client) UsageState(sessionID string) (UsageState, bool) {
	if c.ctx.Err() != nil {
		return UsageState{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sessions[sessionID]
	if s == nil || s.usage == nil {
		return UsageState{}, false
	}
	return *s.usage, true
}

// PromptUsage returns the most recent request's input split. It is separate
// from UsageState because usage_update.used includes generated output for some
// adapters, which is not context occupancy.
func (c *Client) PromptUsage(sessionID string) (PromptUsage, bool) {
	if c.ctx.Err() != nil {
		return PromptUsage{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sessions[sessionID]
	if s == nil || !s.promptTokensAvailable {
		return PromptUsage{}, false
	}
	return PromptUsage{Input: s.promptTokens.Input, CachedInput: s.promptTokens.CachedInput}, true
}

func legacyModelNotification(sessionID string, s *sessionState) Update {
	values := make(sdk.SessionConfigSelectOptionsUngrouped, 0, len(s.legacyChoices))
	for _, choice := range s.legacyChoices {
		value := sdk.SessionConfigSelectOption{Name: choice.Name, Value: sdk.SessionConfigValueId(choice.ID)}
		if choice.Description != "" {
			value.Description = sdk.Ptr(choice.Description)
		}
		values = append(values, value)
	}
	return Update{SessionId: sdk.SessionId(sessionID), Update: sdk.SessionUpdate{
		ConfigOptionUpdate: &sdk.SessionConfigOptionUpdate{
			SessionUpdate: "config_option_update",
			ConfigOptions: []sdk.SessionConfigOption{{Select: &sdk.SessionConfigOptionSelect{
				Id: "model", Name: "Model", Type: "select", Category: sdk.Ptr(sdk.SessionConfigOptionCategoryModel),
				CurrentValue: sdk.SessionConfigValueId(s.currentModel), Options: sdk.SessionConfigSelectOptions{Ungrouped: &values},
			}}},
		},
	}}
}

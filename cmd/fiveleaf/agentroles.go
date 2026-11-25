package main

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type agentTarget struct {
	provider, role string
	preset         *agentRolePreset
}

func (target agentTarget) hasRole() bool { return target.preset != nil || target.role != "" }

func (target agentTarget) roleLabel() string {
	if target.preset != nil {
		return target.preset.name()
	}
	return target.role
}

func (target agentTarget) key(group int) string {
	if target.preset != nil {
		return chatTargetKey(group, "preset:"+target.preset.id+":"+target.provider)
	}
	if target.role != "" {
		return chatTargetKey(group, "role:"+target.role)
	}
	return chatTargetKey(group, "agent:"+target.provider)
}

func agentChatKey(s *agentChat) string {
	if s.historyKey != "" {
		return chatTargetKey(s.group, s.historyKey)
	}
	return s.agentTarget.key(s.group)
}

func chatTargetKey(group int, target string) string {
	if group != 0 {
		return "page:" + strconv.Itoa(group) + ":" + target
	}
	return target
}

func (a *app) selectAgentRole(name string) {
	a.selectChatRole(a.agent, name)
}

func (a *app) selectChatRole(from *agentChat, name string) {
	if err := settings.ValidateAgentRoleName(name); err != nil {
		from.err = err.Error()
		a.invalidate()
		return
	}
	a.selectAgentTarget(from, agentTarget{role: name})
}

func (a *app) selectAgentTarget(from *agentChat, target agentTarget) {
	if target.preset != nil {
		if assigned := a.settings.AgentRolePresets[target.preset.id].Agent; assigned != "" {
			target.provider = assigned
		}
	} else if role := a.settings.AgentRoles[target.role]; target.role != "" && role.FollowCurrentAgent && target.provider == "" {
		target.provider = from.provider
		if target.provider == "" {
			target.provider = agentPresetProvider(a.agentProviders(), "", role.Model).id
		}
	}
	key := target.key(from.group)
	if key == agentChatKey(from) {
		return
	}
	chat := a.agentChats[key]
	if chat == nil {
		chat = &agentChat{agentTarget: target, group: from.group, contextMessages: 10, focusNext: true}
		if target.preset != nil {
			chat.presetDefaultPrompt = target.preset.prompt()
			role, _ := a.chatRoleDefinition(chat)
			chat.systemPrompt = role.Prompt
		} else if target.role != "" {
			role, _ := a.chatRoleDefinition(chat)
			chat.provider, chat.systemPrompt = role.Agent, role.Prompt
		}
	}
	a.selectChat(from, chat)
}

func (a *app) chatRoleDefinition(chat *agentChat) (settings.AgentRole, bool) {
	if chat.preset != nil {
		return a.settings.AgentRolePresets[chat.preset.id].Resolve(chat.provider, chat.presetDefaultPrompt), true
	}
	role, ok := a.settings.AgentRoles[chat.role]
	return role.ForAgent(chat.provider), ok
}

func (a *app) resolveAgentRole(chat *agentChat) (settings.AgentRole, agentProvider, error) {
	role, ok := a.chatRoleDefinition(chat)
	if !ok {
		return role, agentProvider{}, errors.New(locales.AgentRoleMissing(chat.role))
	}
	if chat.preset == nil {
		if err := settings.ValidateAgentRoleName(chat.role); err != nil {
			return role, agentProvider{}, err
		}
	} else if role.Agent != chat.provider {
		return role, agentProvider{}, errors.New(locales.AgentPresetReassigned(role.Agent))
	} else if override := a.settings.AgentRolePresets[chat.preset.id]; override.Agent == "" && override.Model != "" {
		return role, agentProvider{}, errors.New(locales.PrefsRoleAgentRequired())
	}
	if err := role.Validate(); err != nil {
		return role, agentProvider{}, err
	}
	providers := a.agentProviders()
	i := slices.IndexFunc(providers, func(p agentProvider) bool { return p.id == role.Agent })
	if i < 0 {
		if chat.preset != nil {
			return role, agentProvider{}, errors.New(locales.AgentRoleUnavailable())
		}
		return role, agentProvider{}, errors.New(locales.AgentRoleAgentMissing(role.Agent))
	}
	provider := providers[i]
	provider.config = cloneAgentServer(provider.config)
	if provider.config.Disabled {
		return role, provider, errors.New(locales.AgentDisabled())
	}
	if err := provider.config.Validate(); err != nil {
		return role, provider, err
	}
	if chat.preset != nil && !provider.presetModelCompatible(role.Model) {
		if role.Model == "" {
			return role, provider, errors.New(locales.AgentPresetNeedsModel())
		}
		return role, provider, errors.New(locales.AgentRoleModelPlaceholder())
	}
	return role, provider, nil
}

func (provider agentProvider) presetModelCompatible(model string) bool {
	return provider.config.EffectiveProtocol() != "cli" || (model != "") == slices.ContainsFunc(provider.config.Args, func(arg string) bool {
		return strings.Contains(arg, "{model}")
	})
}

// Keep the saved provider immutable: the effective argv belongs to this call,
// while configuration invalidation compares the original definition.
func agentRoleCLIProvider(provider agentProvider, model string) (agentProvider, error) {
	provider.config = cloneAgentServer(provider.config)
	found := false
	for i, arg := range provider.config.Args {
		if strings.Contains(arg, "{model}") {
			if model == "" {
				return provider, errors.New(locales.AgentRoleModelPlaceholder())
			}
			found = true
			provider.config.Args[i] = strings.ReplaceAll(arg, "{model}", model)
		}
	}
	if model != "" && !found {
		return provider, errors.New(locales.AgentRoleModelPlaceholder())
	}
	return provider, nil
}

func (s *agentChat) model() string {
	if s.modelOverride != "" {
		return s.modelOverride
	}
	if s.connectedRole != nil && s.connectedRole.Model != "" {
		return s.connectedRole.Model
	}
	return s.currentModel
}

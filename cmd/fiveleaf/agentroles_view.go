package main

import (
	"maps"
	"slices"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func (a *app) agentRoleNames() []string {
	names := slices.Sorted(maps.Keys(a.settings.AgentRoles))
	// Empty names loaded from hand-edited settings have no selectable target:
	// the empty chat role identifies a direct Agent conversation.
	return slices.DeleteFunc(names, func(name string) bool { return name == "" })
}

func (a *app) agentTargetAvailable(r *agentChat) bool {
	if r.hasRole() {
		_, _, err := a.resolveAgentRole(r)
		return err == nil
	}
	for _, provider := range a.agentProviders() {
		if !provider.config.Disabled && (r.provider == "" || provider.id == r.provider) {
			return true
		}
	}
	return false
}

func (a *app) agentRoleDetail(role settings.AgentRole) string {
	name := role.Agent
	for _, provider := range a.agentProviders() {
		if provider.id == role.Agent {
			name = provider.name
			break
		}
	}
	model := role.Model
	if model == "" {
		model = locales.AgentDefaultModel()
	}
	return name + " · " + model
}

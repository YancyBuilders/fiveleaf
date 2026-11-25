package main

import (
	"maps"
	"strconv"
	"strings"
	"unicode/utf8"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type prefNamedRole struct {
	name, originalName string
	original           settings.AgentRole
}

func (w *prefsWindow) openNamedRole(name string, role settings.AgentRole, existing bool) {
	if w.agentConfigurationEditing() || w.agentIOBusy {
		return
	}
	named := &prefNamedRole{name: name}
	if existing {
		named.originalName, named.original = name, role
	}
	w.roleDraft = &prefRoleDraft{named: named, agent: role.Agent, model: role.Model, prompt: role.Prompt, focus: true}
}

func (w *prefsWindow) copyRole(name string, role settings.AgentRole) {
	for number := 1; ; number++ {
		suffix := ""
		if number > 1 {
			suffix = " " + strconv.Itoa(number)
		}
		for len(locales.PrefsRoleCopyName(name))+len(suffix) > settings.MaxAgentRoleNameBytes {
			_, size := utf8.DecodeLastRuneInString(name)
			name = name[:len(name)-size]
		}
		candidate := locales.PrefsRoleCopyName(name) + suffix
		if _, used := w.m.s.AgentRoles[candidate]; !used {
			w.openNamedRole(candidate, role, false)
			return
		}
	}
}

func (w *prefsWindow) saveNamedRole(draft *prefRoleDraft) bool {
	named := draft.named
	role := settings.AgentRole{Agent: draft.agent, Model: draft.model, Prompt: draft.prompt, FollowCurrentAgent: draft.agent == ""}
	if strings.TrimSpace(named.name) == "" {
		draft.err = locales.PrefsRoleNameRequired()
		return false
	}
	if err := settings.ValidateAgentRoleName(named.name); err != nil {
		draft.err = err.Error()
		return false
	}
	if err := role.Validate(); err != nil {
		draft.err = err.Error()
		return false
	}
	roles := maps.Clone(w.m.s.AgentRoles)
	if named.originalName != "" {
		if current, exists := roles[named.originalName]; !exists || current != named.original {
			draft.err = locales.PrefsRoleConflict()
			return false
		}
		delete(roles, named.originalName)
	}
	if _, exists := roles[named.name]; exists {
		draft.err = locales.PrefsRoleNameConflict()
		return false
	}
	if roles == nil {
		roles = map[string]settings.AgentRole{}
	}
	roles[named.name] = role
	if !w.applyRoleSetting("agentRoles", roles) {
		return false
	}
	w.closeRoleDraft()
	return true
}

func (w *prefsWindow) removeNamedRole(name string) {
	roles := maps.Clone(w.m.s.AgentRoles)
	role, exists := roles[name]
	if !exists {
		return
	}
	delete(roles, name)
	if !w.applyRoleSetting("agentRoles", roles) {
		return
	}
	w.undoable("remove-role:"+name, locales.PrefsRoleRemoved(name), func() {
		current := maps.Clone(w.m.s.AgentRoles)
		if _, exists := current[name]; exists {
			w.errors = append(w.errors, locales.PrefsRoleNameConflict())
			return
		}
		if current == nil {
			current = map[string]settings.AgentRole{}
		}
		current[name] = role
		w.applyRoleSetting("agentRoles", current)
	})
}

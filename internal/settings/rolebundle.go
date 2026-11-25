package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"unicode/utf8"
)

type RolePresetTemplate struct {
	Name          string `json:"name,omitempty"`
	DefaultPrompt string `json:"defaultPrompt,omitempty"`
	AgentRoleOverride
}

type roleBundle struct {
	Version int                            `json:"version"`
	Roles   map[string]AgentRole           `json:"roles"`
	Presets map[string]*RolePresetTemplate `json:"presets,omitempty"`
}

func (p Prefs) ExportRoles(templates map[string]RolePresetTemplate) ([]byte, error) {
	if err := ValidateAgentRoles(p.AgentRoles); err != nil {
		return nil, err
	}
	if err := ValidateAgentRolePresets(p.AgentRolePresets); err != nil {
		return nil, err
	}
	presets := make(map[string]*RolePresetTemplate, len(templates))
	for id, entry := range templates {
		entry.AgentRoleOverride = p.AgentRolePresets[id]
		presets[id] = &entry
	}
	roles := p.AgentRoles
	if roles == nil {
		roles = map[string]AgentRole{}
	}
	raw, err := json.MarshalIndent(roleBundle{Version: 1, Roles: roles, Presets: presets}, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(raw)+1 > MaxAgentConfigurationSize {
		return nil, errors.New("role bundle is too large")
	}
	return append(raw, '\n'), nil
}

func (p *Prefs) ImportRoles(raw []byte) error {
	if len(raw) > MaxAgentConfigurationSize {
		return errors.New("role bundle is too large")
	}
	if !utf8.Valid(raw) {
		return errors.New("role bundle must be UTF-8 JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if _, legacy := fields["agentServers"]; legacy {
		rolesOnly := Prefs{AgentRoles: p.AgentRoles, AgentRolePresets: p.AgentRolePresets}
		if err := rolesOnly.ImportAgents(raw); err != nil {
			return err
		}
		p.AgentRoles, p.AgentRolePresets = rolesOnly.AgentRoles, rolesOnly.AgentRolePresets
		return nil
	}
	if err := uniqueAgentJSONKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return err
	}
	var bundle roleBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return err
	}
	if bundle.Version != 1 {
		return fmt.Errorf("unsupported role bundle version: %d", bundle.Version)
	}
	for _, key := range []string{"roles", "presets"} {
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return fmt.Errorf("%s must be an object", key)
		}
	}
	if bundle.Roles == nil && bundle.Presets == nil {
		return errors.New("role bundle requires a roles or presets object")
	}
	overrides := make(map[string]AgentRoleOverride, len(bundle.Presets))
	for id, entry := range bundle.Presets {
		if entry == nil {
			return fmt.Errorf("preset %q must be an object", id)
		}
		overrides[id] = entry.AgentRoleOverride
	}
	if err := ValidateAgentRolePresets(overrides); err != nil {
		return err
	}
	maps.DeleteFunc(overrides, func(_ string, override AgentRoleOverride) bool { return override == (AgentRoleOverride{}) })
	return p.mergeRoleDefinitions(bundle.Roles, overrides)
}

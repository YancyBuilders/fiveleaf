package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
)

const MaxAgentConfigurationSize = 1024 * 1024

type agentConfiguration struct {
	Version          int                          `json:"version"`
	AgentServers     map[string]AgentServer       `json:"agentServers"`
	AgentRoles       map[string]AgentRole         `json:"agentRoles,omitempty"`
	AgentRolePresets map[string]AgentRoleOverride `json:"agentRolePresets,omitempty"`
}

func hasFollowingRole(roles map[string]AgentRole) bool {
	for _, role := range roles {
		if role.FollowCurrentAgent {
			return true
		}
	}
	return false
}

// ExportAgents includes roles, local registrations and their explicit
// environment values. Callers should write this file with private permissions.
func (p Prefs) ExportAgents() ([]byte, error) {
	if err := validateAgentServers(p.AgentServers); err != nil {
		return nil, err
	}
	if err := ValidateAgentRoles(p.AgentRoles); err != nil {
		return nil, err
	}
	if err := ValidateAgentRolePresets(p.AgentRolePresets); err != nil {
		return nil, err
	}
	servers := p.AgentServers
	if servers == nil {
		servers = map[string]AgentServer{}
	}
	version := 1
	if len(p.AgentRoles) > 0 {
		version = 2
	}
	if len(p.AgentRolePresets) > 0 {
		version = 3
	}
	if hasFollowingRole(p.AgentRoles) {
		version = 4
	}
	raw, err := json.MarshalIndent(agentConfiguration{version, servers, p.AgentRoles, p.AgentRolePresets}, "", "  ")
	if err == nil && len(raw)+1 > MaxAgentConfigurationSize {
		return nil, errors.New("agent configuration is too large")
	}
	return append(raw, '\n'), err
}

// ImportAgents merges agents and roles only after the entire file has passed
// validation. An occupied ID or name must have the same definition.
func (p *Prefs) ImportAgents(raw []byte) error {
	if len(raw) > MaxAgentConfigurationSize {
		return errors.New("agent configuration is too large")
	}
	if err := uniqueAgentJSONKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return fmt.Errorf("invalid agent configuration: %w", err)
	}
	var config struct {
		Version          int                    `json:"version"`
		AgentServers     map[string]AgentServer `json:"agentServers"`
		AgentRoles       json.RawMessage        `json:"agentRoles"`
		AgentRolePresets json.RawMessage        `json:"agentRolePresets"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&config); err != nil {
		return fmt.Errorf("invalid agent configuration: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("agent configuration must contain one JSON object")
	}
	if config.Version < 1 || config.Version > 4 {
		return fmt.Errorf("unsupported agent configuration version: %d", config.Version)
	}
	if config.AgentServers == nil {
		return errors.New("agent configuration requires an agentServers object")
	}
	if err := validateAgentServers(config.AgentServers); err != nil {
		return err
	}
	var roles map[string]AgentRole
	if config.AgentRoles != nil {
		if config.Version < 2 {
			return errors.New("agentRoles requires agent configuration version 2 or later")
		}
		var err error
		roles, err = decodeAgentRoles(config.AgentRoles)
		if err != nil {
			return fmt.Errorf("invalid agent roles: %w", err)
		}
		if roles == nil {
			return errors.New("agentRoles must be an object")
		}
		if config.Version < 4 && hasFollowingRole(roles) {
			return errors.New("followCurrentAgent requires agent configuration version 4")
		}
	}
	merged := maps.Clone(p.AgentServers)
	if merged == nil {
		merged = map[string]AgentServer{}
	}
	for _, id := range slices.Sorted(maps.Keys(config.AgentServers)) {
		server := config.AgentServers[id]
		if current, exists := merged[id]; exists {
			if !sameAgentDefinition(current, server) {
				return fmt.Errorf("agent %q already has a different configuration", id)
			}
			continue
		}
		merged[id] = server
	}
	if err := validateAgentServers(merged); err != nil {
		return err
	}
	var presets map[string]AgentRoleOverride
	if config.Version == 3 || config.Version == 4 && config.AgentRolePresets != nil {
		var err error
		presets, err = decodeRoleMap(config.AgentRolePresets, ValidateAgentRolePresets)
		if err != nil {
			return fmt.Errorf("invalid built-in roles: %w", err)
		}
		if presets == nil {
			return errors.New("agentRolePresets must be an object")
		}
	} else if config.AgentRolePresets != nil {
		return errors.New("agentRolePresets requires agent configuration version 3 or later")
	}
	if err := p.mergeRoleDefinitions(roles, presets); err != nil {
		return err
	}
	p.AgentServers = merged
	return nil
}

func (p *Prefs) mergeRoleDefinitions(roles map[string]AgentRole, presets map[string]AgentRoleOverride) error {
	mergedRoles := maps.Clone(p.AgentRoles)
	if mergedRoles == nil && roles != nil {
		mergedRoles = map[string]AgentRole{}
	}
	for _, name := range slices.Sorted(maps.Keys(roles)) {
		if current, exists := mergedRoles[name]; exists && current != roles[name] {
			return fmt.Errorf("role %q already has a different configuration", name)
		}
		mergedRoles[name] = roles[name]
	}
	if err := ValidateAgentRoles(mergedRoles); err != nil {
		return err
	}
	mergedPresets := maps.Clone(p.AgentRolePresets)
	if mergedPresets == nil && presets != nil {
		mergedPresets = map[string]AgentRoleOverride{}
	}
	for _, id := range slices.Sorted(maps.Keys(presets)) {
		if current, exists := mergedPresets[id]; exists && !reflect.DeepEqual(current, presets[id]) {
			return fmt.Errorf("built-in role %q already has a different configuration", id)
		}
		mergedPresets[id] = presets[id]
	}
	p.AgentRoles, p.AgentRolePresets = mergedRoles, mergedPresets
	return nil
}

func sameAgentDefinition(a, b AgentServer) bool {
	canonical := func(s AgentServer) []byte {
		s.Protocol, s.TimeoutSeconds = s.EffectiveProtocol(), s.ReplyTimeoutSeconds()
		s.Capabilities = slices.Sorted(slices.Values(s.Capabilities))
		raw, _ := json.Marshal(s)
		return raw
	}
	return bytes.Equal(canonical(a), canonical(b))
}

// encoding/json otherwise silently keeps the last occurrence, which could
// hide a conflicting registration or environment entry inside one import.
func uniqueAgentJSONKeys(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("repeated JSON key %q", key)
			}
			seen[key] = true
			if err := uniqueAgentJSONKeys(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
	case json.Delim('['):
		for d.More() {
			if err := uniqueAgentJSONKeys(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
	}
	return err
}

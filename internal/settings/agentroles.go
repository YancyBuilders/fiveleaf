package settings

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxAgentRoles           = 128
	MaxAgentRoleNameBytes   = 256
	MaxAgentRoleAgentBytes  = 256
	MaxAgentRoleModelBytes  = 1024
	MaxAgentRolePromptBytes = 64 << 10
)

// AgentRole reuses an agent with a model and prompt. An empty model leaves
// the agent's default in place; the prompt may also be empty.
type AgentRole struct {
	Agent              string `json:"agent"`
	Model              string `json:"model,omitempty"`
	Prompt             string `json:"prompt,omitempty"`
	FollowCurrentAgent bool   `json:"followCurrentAgent,omitempty"`
}

type CopilotConfig struct {
	Agent string `json:"agent,omitempty"`
	Model string `json:"model,omitempty"`
}

func (config CopilotConfig) Validate() error {
	if config == (CopilotConfig{}) {
		return nil
	}
	return (AgentRole{Agent: config.Agent, Model: config.Model}).Validate()
}

type AgentRoleOverride struct {
	Agent  string  `json:"agent,omitempty"`
	Model  string  `json:"model,omitempty"`
	Prompt *string `json:"prompt,omitempty"`
}

func (override AgentRoleOverride) Resolve(agent, prompt string) AgentRole {
	if override.Prompt != nil {
		prompt = *override.Prompt
	}
	return AgentRole{Agent: cmp.Or(override.Agent, agent), Model: override.Model, Prompt: prompt}
}

func (override AgentRoleOverride) Validate() error {
	return override.Resolve("", "").validate(true)
}

// Validate checks syntax without requiring the agent to be installed. A role
// may refer to a plugin or an agent configured on another machine.
func (r AgentRole) Validate() error {
	if r.FollowCurrentAgent && r.Agent != "" {
		return fmt.Errorf("a role following the current agent cannot also assign an agent")
	}
	return r.validate(r.FollowCurrentAgent)
}

func (r AgentRole) ForAgent(id string) AgentRole {
	if r.FollowCurrentAgent && r.Agent == "" && r.Model == "" {
		r.Agent, r.FollowCurrentAgent = id, false
	}
	return r
}

func (r AgentRole) validate(followAgent bool) error {
	if followAgent && r.Agent == "" && r.Model != "" {
		return fmt.Errorf("assign an agent before selecting a role model")
	}
	if !followAgent && r.Agent == "" || len(r.Agent) > MaxAgentRoleAgentBytes || !utf8.ValidString(r.Agent) ||
		strings.IndexFunc(r.Agent, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return fmt.Errorf("role agent must be nonempty, contain no whitespace or control characters, and fit in %d bytes", MaxAgentRoleAgentBytes)
	}
	if len(r.Model) > MaxAgentRoleModelBytes || !utf8.ValidString(r.Model) || r.Model != strings.TrimSpace(r.Model) ||
		strings.IndexFunc(r.Model, unicode.IsControl) >= 0 {
		return fmt.Errorf("role model must contain no control characters or surrounding whitespace and fit in %d bytes", MaxAgentRoleModelBytes)
	}
	if len(r.Prompt) > MaxAgentRolePromptBytes {
		return fmt.Errorf("role prompt must fit in %d bytes", MaxAgentRolePromptBytes)
	}
	if !utf8.ValidString(r.Prompt) {
		return fmt.Errorf("role prompt must be UTF-8 text")
	}
	return nil
}

func ValidateAgentRoleName(name string) error {
	if name == "" || name != strings.TrimSpace(name) || len(name) > MaxAgentRoleNameBytes || !utf8.ValidString(name) ||
		strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("role name must be nonempty, contain no control characters or surrounding whitespace, and fit in %d bytes", MaxAgentRoleNameBytes)
	}
	return nil
}

func ValidateAgentRoles(roles map[string]AgentRole) error {
	if len(roles) > MaxAgentRoles {
		return fmt.Errorf("at most %d agent roles may be configured", MaxAgentRoles)
	}
	for name, role := range roles {
		if err := ValidateAgentRoleName(name); err != nil {
			return err
		}
		if err := role.Validate(); err != nil {
			return fmt.Errorf("role %q: %w", name, err)
		}
	}
	return nil
}

func decodeAgentRoles(raw []byte) (map[string]AgentRole, error) {
	return decodeRoleMap(raw, ValidateAgentRoles)
}

func ValidateAgentRolePresets(presets map[string]AgentRoleOverride) error {
	for id, override := range presets {
		switch id {
		case "explore", "plan", "implement", "test", "review", "diagnose", "document",
			"requirements", "interaction", "compatibility", "moderate":
		default:
			return fmt.Errorf("unknown built-in role %q", id)
		}
		if err := override.Validate(); err != nil {
			return fmt.Errorf("built-in role %q: %w", id, err)
		}
	}
	return nil
}

func decodeRoleMap[T any](raw []byte, validate func(map[string]T) error) (map[string]T, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("roles must be UTF-8 JSON")
	}
	if err := uniqueAgentJSONKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var roles map[string]T
	if err := d.Decode(&roles); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("roles must contain one JSON object")
	}
	if err := validate(roles); err != nil {
		return nil, err
	}
	return roles, nil
}

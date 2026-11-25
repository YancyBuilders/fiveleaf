package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"fiveleaf/internal/plugin"
)

type pluginCommandSettings struct {
	declared map[string]plugin.SettingContrib
	explicit map[string]json.RawMessage
}

func (host *pluginHost) commandSettings(provider *plugin.Plugin) pluginCommandSettings {
	settings := pluginCommandSettings{declared: provider.Contributes.Settings}
	if host != nil {
		json.Unmarshal(host.Settings(provider.Name), &settings.explicit)
		delete(settings.explicit, "enabled")
	}
	return settings
}

func (settings pluginCommandSettings) value(key string) json.RawMessage {
	if raw, ok := settings.explicit[key]; ok {
		return raw
	}
	return settings.declared[key].Default
}

func (settings pluginCommandSettings) expand(arg string) (string, error) {
	var out strings.Builder
	for {
		prefix, rest, found := strings.Cut(arg, "${settings.")
		out.WriteString(prefix)
		if !found {
			return out.String(), nil
		}
		key, tail, closed := strings.Cut(rest, "}")
		var value string
		if !closed || json.Unmarshal(settings.value(key), &value) != nil || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("plugin setting %q must be a nonempty string", key)
		}
		out.WriteString(value)
		arg = tail
	}
}

func (settings pluginCommandSettings) appendSettingArgs(args []string, flags map[string]string) ([]string, error) {
	for _, key := range slices.Sorted(maps.Keys(flags)) {
		raw := settings.explicit[key]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		value, err := settings.declared[key].ArgumentValue(raw)
		if err != nil {
			return nil, fmt.Errorf("plugin setting %q: %w", key, err)
		}
		if value != "" {
			args = append(args, strings.ReplaceAll(flags[key], "${value}", value))
		}
	}
	return args, nil
}

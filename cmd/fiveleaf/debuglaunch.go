package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"fiveleaf/internal/plugin"
	"fiveleaf/internal/remote"
)

type debugLaunch struct {
	Command   string                 `json:"command"`
	Args      []string               `json:"args"`
	Env       map[string]string      `json:"env"`
	Transport *plugin.DebugTransport `json:"transport"`
	Launch    map[string]any         `json:"launch"`
	Options   json.RawMessage        `json:"initializationOptions"`
}

type debugWorkspace struct {
	root, file string
	provider   *plugin.Plugin
	settings   pluginCommandSettings
}

func debugRebasePath(file, from, to string) string {
	if from != to && from != "" && to != "" {
		if relative, err := filepath.Rel(from, file); err == nil && filepath.IsLocal(relative) {
			return filepath.Join(to, relative)
		}
	}
	return file
}

func (workspace debugWorkspace) resolve(ctx context.Context, adapter *plugin.DebugAdapterContrib) (debugLaunch, error) {
	launch := debugLaunch{Command: adapter.Command, Args: slices.Clone(adapter.Args), Env: map[string]string{},
		Transport: adapter.Transport, Options: adapter.Options, Launch: map[string]any{}}
	maps.Copy(launch.Env, adapter.Env)
	request, defaults := "launch", adapter.Launch
	if len(defaults) == 0 && len(adapter.Attach) != 0 {
		request, defaults = "attach", adapter.Attach
	}
	if len(defaults) != 0 {
		decoder := json.NewDecoder(bytes.NewReader(defaults))
		decoder.UseNumber()
		if err := decoder.Decode(&launch.Launch); err != nil || launch.Launch == nil {
			return launch, fmt.Errorf("%s arguments must be an object", request)
		}
	}
	if resolver := adapter.Resolve; resolver != nil {
		command, args, env, err := workspace.process(resolver.Command, resolver.Args, resolver.Env)
		if err != nil {
			return launch, err
		}
		explicit := workspace.settings.explicit
		if explicit == nil {
			explicit = map[string]json.RawMessage{}
		}
		input, err := json.Marshal(map[string]any{"root": hostPath(workspace.root), "file": hostPath(workspace.file), "settings": explicit})
		if err != nil {
			return launch, err
		}
		var overrides *debugLaunch
		if err := runPluginResolver(ctx, workspace.root, envList(env), input, command, args, &overrides); err != nil {
			return launch, err
		}
		if overrides == nil {
			return launch, errors.New("debug resolver must return an object")
		}
		if overrides.Command != "" {
			launch.Command = overrides.Command
		}
		if overrides.Args != nil {
			launch.Args = overrides.Args
		}
		if overrides.Transport != nil {
			launch.Transport = overrides.Transport
		}
		if overrides.Options != nil {
			launch.Options = overrides.Options
		}
		maps.Copy(launch.Env, overrides.Env)
		maps.Copy(launch.Launch, overrides.Launch)
	}
	if launch.Transport != nil {
		if err := launch.Transport.Validate(); err != nil {
			return launch, err
		}
	}
	if strings.TrimSpace(launch.Command) == "" {
		if !launch.Transport.Direct() {
			return launch, errors.New("resolved debug adapter has no command")
		}
		if len(launch.Env)+len(launch.Args) > 0 {
			return launch, errors.New("debug adapter arguments and environment require a command")
		}
	}
	var err error
	launch.Command, launch.Args, launch.Env, err = workspace.process(launch.Command, launch.Args, launch.Env)
	if err != nil {
		return launch, err
	}
	if _, err := workspace.expandValue(launch.Launch); err != nil {
		return launch, err
	}
	if _, set := launch.Launch["request"]; !set {
		launch.Launch["request"] = request
	}
	switch launch.Launch["request"] {
	case "launch":
		if _, set := launch.Launch["program"]; !set {
			launch.Launch["program"] = hostPath(workspace.file)
		}
	case "attach":
	default:
		return launch, errors.New("debug request must be launch or attach")
	}
	if _, set := launch.Launch["cwd"]; !set {
		launch.Launch["cwd"] = hostPath(workspace.root)
	}
	return launch, nil
}

func (workspace debugWorkspace) process(command string, args []string, env map[string]string) (string, []string, map[string]string, error) {
	executable, err := workspace.expand(command)
	if err != nil {
		return "", nil, nil, err
	}
	if remote.Is(workspace.root) {
		if strings.ContainsAny(executable, `/\`) && !path.IsAbs(executable) {
			return "", nil, nil, fmt.Errorf("debug adapter executable %q needs a name on PATH or an absolute path on the SSH host", executable)
		}
	} else {
		executable = workspace.provider.DebugCommand(executable)
	}
	arguments := slices.Clone(args)
	for index, value := range args {
		arguments[index], err = workspace.expand(value)
		if err != nil {
			return "", nil, nil, err
		}
	}
	environment := make(map[string]string, len(env))
	for key, value := range env {
		if !validEnvKey(key) {
			return "", nil, nil, fmt.Errorf("invalid debug environment key %q", key)
		}
		environment[key], err = workspace.expand(value)
		if err != nil {
			return "", nil, nil, err
		}
	}
	return executable, arguments, environment, nil
}

func (workspace debugWorkspace) expand(text string) (string, error) {
	var expanded strings.Builder
	file := hostPath(workspace.file)
	for {
		prefix, token, found := strings.Cut(text, "${")
		expanded.WriteString(prefix)
		if !found {
			return expanded.String(), nil
		}
		key, rest, closed := strings.Cut(token, "}")
		if !closed {
			expanded.WriteString("${" + token)
			return expanded.String(), nil
		}
		var value string
		switch key {
		case "file":
			value = file
		case "workspaceFolder":
			value = hostPath(workspace.root)
		case "fileDirname":
			value = filepath.Dir(file)
			if remote.Is(workspace.root) {
				value = path.Dir(file)
			}
		case "fileBasename":
			value = filepath.Base(file)
		case "pluginDir":
			if remote.Is(workspace.root) || workspace.provider.Dir == "" {
				return "", errors.New("${pluginDir} requires an installed plugin and a local workspace")
			}
			value = workspace.provider.Dir
		default:
			var err error
			value, err = workspace.settings.expand("${" + key + "}")
			if err != nil {
				return "", err
			}
		}
		expanded.WriteString(value)
		text = rest
	}
}

func (workspace debugWorkspace) expandValue(value any) (any, error) {
	var err error
	switch nested := value.(type) {
	case string:
		return workspace.expand(nested)
	case []any:
		for index, item := range nested {
			if nested[index], err = workspace.expandValue(item); err != nil {
				return nil, err
			}
		}
	case map[string]any:
		for key, item := range nested {
			if nested[key], err = workspace.expandValue(item); err != nil {
				return nil, err
			}
		}
	}
	return value, nil
}

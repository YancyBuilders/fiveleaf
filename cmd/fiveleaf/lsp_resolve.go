package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"fiveleaf/internal/lsp"
	"fiveleaf/internal/plugin"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/vfs"
)

// serverLaunch is the host-independent result of a plugin's setup command.
// The command exits before the actual language server is started.
type serverLaunch struct {
	Command          string                `json:"command"`
	Args             []string              `json:"args,omitempty"`
	Env              map[string]string     `json:"env,omitempty"`
	UnsetEnv         []string              `json:"unsetEnv,omitempty"`
	Options          json.RawMessage       `json:"initializationOptions,omitempty"`
	Configuration    map[string]any        `json:"configuration,omitempty"`
	WorkspaceFolders []lsp.WorkspaceFolder `json:"workspaceFolders,omitempty"`
	PathMappings     []lsp.PathMapping     `json:"pathMappings,omitempty"`
}

// limitedBuffer caps captured output while continuing to drain the process pipes.
type limitedBuffer struct {
	buffer   bytes.Buffer
	overflow bool
	limit    int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	limit := b.limit
	if limit == 0 {
		limit = 4 << 20
	}
	if available := limit - b.buffer.Len(); len(p) > available {
		b.overflow = true
		p = p[:available]
	}
	b.buffer.Write(p)
	return n, nil
}

func (b *limitedBuffer) String() string { return b.buffer.String() }

func (settings pluginCommandSettings) staticServer(server plugin.ServerContrib) (serverLaunch, error) {
	command, err := settings.expand(server.Command)
	launch := serverLaunch{Command: command, Args: slices.Clone(server.Args), Options: server.Options}
	if err == nil {
		launch.Args, err = settings.appendSettingArgs(launch.Args, server.SettingArgs)
	}
	return launch, err
}

func (s *lspServer) resolve(ctx context.Context) (serverLaunch, error) {
	settings := s.host.commandSettings(s.plugin)
	resolver := s.contrib.Resolve
	if resolver == nil {
		return settings.staticServer(s.contrib)
	}
	var launch serverLaunch
	command, err := settings.expand(resolver.Command)
	if err != nil {
		return launch, err
	}
	args := make([]string, len(resolver.Args))
	for index, arg := range resolver.Args {
		args[index], err = settings.expand(arg)
		if err != nil {
			return launch, err
		}
	}
	root := s.root
	if _, path, ok := remote.Split(root); ok {
		root = path
	}
	explicit := settings.explicit
	if explicit == nil {
		explicit = map[string]json.RawMessage{}
	}
	input, _ := json.Marshal(map[string]any{"root": root, "settings": explicit})
	if err := runPluginResolver(ctx, s.root, nil, input, command, args, &launch); err != nil {
		return launch, err
	}
	return launch, launch.validate()
}

func runPluginResolver(ctx context.Context, root string, env []string, input []byte, command string, args []string, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd, err := vfs.Command(root, env, bytes.NewReader(input), command, args...)
	if err != nil {
		return err
	}
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	if err = cmd.Start(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { cmd.Process.Kill() })
	err = cmd.Wait()
	stop()
	if ctx.Err() != nil {
		return fmt.Errorf("%s did not finish in time: %w", command, ctx.Err())
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w: %s", command, err, msg)
		}
		return fmt.Errorf("%s: %w", command, err)
	}
	if stdout.overflow {
		return fmt.Errorf("%s: resolver output exceeds 4 MiB", command)
	}
	decoder := json.NewDecoder(&stdout.buffer)
	decoder.UseNumber()
	if err = decoder.Decode(result); err != nil {
		return fmt.Errorf("%s: invalid launch description: %w", command, err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("%s: unexpected resolver output", command)
	}
	return nil
}

func (l serverLaunch) validate() error {
	if strings.TrimSpace(l.Command) == "" {
		return fmt.Errorf("resolved language server has no command")
	}
	for key := range l.Env {
		if !validEnvKey(key) {
			return fmt.Errorf("invalid environment key %q", key)
		}
	}
	for _, key := range l.UnsetEnv {
		if !validEnvKey(key) {
			return fmt.Errorf("invalid environment key %q", key)
		}
	}
	fileURI := func(value string) bool {
		u, err := url.Parse(value)
		return err == nil && u.Scheme == "file" && strings.HasPrefix(u.Path, "/")
	}
	for _, f := range l.WorkspaceFolders {
		if !fileURI(f.URI) {
			return fmt.Errorf("invalid workspace folder URI %q", f.URI)
		}
	}
	for _, m := range l.PathMappings {
		if !fileURI(m.ClientURI) || !fileURI(m.ServerURI) {
			return fmt.Errorf("path mappings must contain absolute file URIs")
		}
	}
	return nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func (l serverLaunch) environment() []string {
	keys := make([]string, 0, len(l.Env))
	for key := range l.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+l.Env[key])
	}
	return env
}

// Package tasks reads task definitions and resolves their data before a shell runs them.
package tasks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"fiveleaf/internal/keymap"
	"fiveleaf/internal/remote"
)

type Task struct {
	Label               string            `json:"label"`
	Command             string            `json:"command"`
	Args                []string          `json:"args,omitempty"`
	Cwd                 string            `json:"cwd,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	UseNewTerminal      bool              `json:"use_new_terminal,omitempty"`
	AllowConcurrentRuns bool              `json:"allow_concurrent_runs,omitempty"`
}

func Builtins() []Task {
	return []Task{{Label: "lazygit", Command: "lazygit"}, {Label: "yazi", Command: "yazi"}}
}

func (task Task) IsDefaultYazi() bool {
	return task.Label == "yazi" && task.Command == "yazi" && task.Args == nil &&
		task.Cwd == "" && task.Env == nil && !task.UseNewTerminal && !task.AllowConcurrentRuns
}

// Parse accepts JSON comments and trailing commas. Unknown fields are ignored
// so a task can use the common fields of an existing Zed configuration.
func Parse(data []byte) ([]Task, error) {
	data = bytes.TrimSpace(keymap.StripJSONC(data))
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] != '[' {
		return nil, fmt.Errorf("tasks must be a JSON array")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	tasks := make([]Task, 0, len(entries))
	labels := make(map[string]int, len(entries))
	for i, entry := range entries {
		var task Task
		err := json.Unmarshal(entry, &task)
		if err == nil {
			err = validate(task)
		}
		if err != nil {
			return nil, taskError(i, task.Label, err)
		}
		if first, exists := labels[task.Label]; exists {
			return nil, taskError(i, task.Label, fmt.Errorf("label also appears in task %d", first+1))
		}
		labels[task.Label] = i
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func taskError(index int, label string, err error) error {
	if label == "" {
		return fmt.Errorf("task %d: %w", index+1, err)
	}
	return fmt.Errorf("task %d (%q): %w", index+1, label, err)
}

func validate(task Task) error {
	if strings.TrimSpace(task.Label) == "" {
		return fmt.Errorf("label must not be empty")
	}
	if strings.TrimSpace(task.Command) == "" {
		return fmt.Errorf("command must not be empty")
	}
	for _, field := range []struct{ name, value string }{
		{"label", task.Label}, {"command", task.Command}, {"cwd", task.Cwd},
	} {
		if strings.ContainsRune(field.value, 0) {
			return fmt.Errorf("%s contains a NUL character", field.name)
		}
	}
	for i, arg := range task.Args {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("args[%d] contains a NUL character", i)
		}
	}
	for _, key := range sortedKeys(task.Env) {
		if !variableName(key) {
			return fmt.Errorf("invalid environment variable name %q", key)
		}
		if strings.ContainsRune(task.Env[key], 0) {
			return fmt.Errorf("env.%s contains a NUL character", key)
		}
	}
	return nil
}

// Merge keeps each label's first position and takes its latest definition.
func Merge(groups ...[]Task) []Task {
	var merged []Task
	positions := map[string]int{}
	for _, group := range groups {
		for _, task := range group {
			if at, exists := positions[task.Label]; exists {
				merged[at] = task
			} else {
				positions[task.Label] = len(merged)
				merged = append(merged, task)
			}
		}
	}
	return merged
}

type Context struct {
	Root, File  string
	Row, Column int // One-based; zero starts at the first row or column.
	Environ     []string
}

type Resolved struct {
	Task    Task
	Dir     string
	Args    []string
	Env     []string // Overrides and context variables, sorted by name.
	Context Context
}

func (task Resolved) YaziFile() string {
	if task.Task.IsDefaultYazi() && len(task.Args) > 0 {
		return task.Args[0]
	}
	return ""
}

// Resolve expands argument, directory and environment data once. Command
// remains shell source; callers must quote each resolved argument separately.
func Resolve(task Task, context Context) (Resolved, error) {
	return resolve(task, context, runtime.GOOS == "windows")
}

func resolve(task Task, context Context, foldEnvNames bool) (Resolved, error) {
	if err := validate(task); err != nil {
		return Resolved{}, fmt.Errorf("task %q: %w", task.Label, err)
	}
	if context.Root == "" || !filepath.IsAbs(context.Root) || strings.ContainsRune(context.Root, 0) {
		return Resolved{}, fmt.Errorf("task %q: workspace root must be an absolute path without NUL characters", task.Label)
	}
	if strings.ContainsRune(context.File, 0) {
		return Resolved{}, fmt.Errorf("task %q: file path contains a NUL character", task.Label)
	}
	context.Root = filepath.Clean(context.Root)
	// Windows treats Path and PATH as the same variable in both lookups and
	// the process environment. Other systems keep their distinct spellings.
	envKey := func(name string) string {
		if foldEnvNames {
			return strings.ToUpper(name)
		}
		return name
	}
	vars := make(map[string]string, len(context.Environ)+14)
	for _, entry := range context.Environ {
		if key, value, ok := strings.Cut(entry, "="); ok && key != "" {
			vars[envKey(key)] = value
		}
	}
	values := map[string]string{
		"WORKTREE_ROOT": context.Root,
		"FILE":          "", "DIRNAME": "", "FILENAME": "", "RELATIVE_FILE": "", "ROW": "", "COLUMN": "",
	}
	if context.File != "" {
		file := context.File
		if !filepath.IsAbs(file) {
			file = filepath.Join(context.Root, file)
		}
		file = filepath.Clean(file)
		relative, err := filepath.Rel(context.Root, file)
		if err != nil {
			// A file from a different Windows drive has no relative path.
			relative = file
		}
		values["FILE"] = file
		values["DIRNAME"] = filepath.Dir(file)
		values["FILENAME"] = filepath.Base(file)
		values["RELATIVE_FILE"] = relative
		values["ROW"] = strconv.Itoa(max(context.Row, 1))
		values["COLUMN"] = strconv.Itoa(max(context.Column, 1))
	}
	overrides := map[string]string{}
	for key, value := range values {
		for _, prefix := range []string{"FIVELEAF_", "ZED_"} {
			vars[prefix+key], overrides[prefix+key] = value, value
		}
	}
	lookup := func(name string) (string, error) {
		key := envKey(name)
		if context.File == "" && fileVariable(key) {
			return "", fmt.Errorf("$%s needs an active file", name)
		}
		value, ok := vars[key]
		if !ok {
			return "", fmt.Errorf("undefined variable $%s", name)
		}
		return value, nil
	}
	resolve := func(field, value string) (string, error) {
		expanded, err := expand(value, lookup)
		if err == nil && strings.ContainsRune(expanded, 0) {
			err = fmt.Errorf("value contains a NUL character")
		}
		if err != nil {
			return "", fmt.Errorf("task %q, %s: %w", task.Label, field, err)
		}
		return expanded, nil
	}
	// Environment values use the inherited environment and context, not each
	// other, so PATH can extend itself and map iteration cannot change a task.
	for _, key := range sortedKeys(task.Env) {
		value, err := resolve("env."+key, task.Env[key])
		if err != nil {
			return Resolved{}, err
		}
		overrides[envKey(key)] = value
	}
	for key, value := range overrides {
		vars[key] = value
	}
	resolved := Resolved{Task: task, Args: make([]string, len(task.Args))}
	for i, arg := range task.Args {
		value, err := resolve(fmt.Sprintf("args[%d]", i), arg)
		if err != nil {
			return Resolved{}, err
		}
		resolved.Args[i] = value
	}
	dir, err := resolve("cwd", task.Cwd)
	if err != nil {
		return Resolved{}, err
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") || filepath.Separator == '\\' && strings.HasPrefix(dir, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return Resolved{}, fmt.Errorf("task %q, cwd: home directory is unavailable", task.Label)
		}
		dir = filepath.Join(home, strings.TrimLeft(dir[1:], `/\`))
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(context.Root, dir)
	}
	resolved.Dir = filepath.Clean(dir)
	if file := values["FILE"]; task.IsDefaultYazi() && file != "" && !remote.Is(context.File) {
		if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
			resolved.Args = []string{file}
		}
	}
	for _, key := range sortedKeys(overrides) {
		resolved.Env = append(resolved.Env, key+"="+overrides[key])
	}
	context.File, context.Environ = values["FILE"], nil
	resolved.Context = context
	return resolved, nil
}

func fileVariable(name string) bool {
	for _, prefix := range []string{"FIVELEAF_", "ZED_"} {
		if suffix, ok := strings.CutPrefix(name, prefix); ok {
			switch suffix {
			case "FILE", "DIRNAME", "FILENAME", "RELATIVE_FILE", "ROW", "COLUMN":
				return true
			}
		}
	}
	return false
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func nameStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func namePart(c byte) bool { return nameStart(c) || c >= '0' && c <= '9' }

func variableName(name string) bool {
	if name == "" || !nameStart(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !namePart(name[i]) {
			return false
		}
	}
	return true
}

// Only named variables expand. Shell expressions such as $(), $$ and $?
// remain argument data, as does any dollar sign in a replacement value.
func expand(text string, lookup func(string) (string, error)) (string, error) {
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '$' || i+1 == len(text) {
			out.WriteByte(text[i])
			i++
			continue
		}
		if text[i+1] == '$' {
			out.WriteString("$$")
			i += 2
			continue
		}
		var name string
		end := i + 1
		if text[end] == '{' {
			close := strings.IndexByte(text[end+1:], '}')
			if close < 0 {
				return "", fmt.Errorf("unclosed variable at byte %d", i)
			}
			end += close + 2
			name = text[i+2 : end-1]
			if !variableName(name) {
				return "", fmt.Errorf("invalid variable expression %q", text[i:end])
			}
		} else if nameStart(text[end]) {
			for end < len(text) && namePart(text[end]) {
				end++
			}
			name = text[i+1 : end]
		} else {
			out.WriteByte('$')
			i++
			continue
		}
		value, err := lookup(name)
		if err != nil {
			return "", err
		}
		out.WriteString(value)
		i = end
	}
	return out.String(), nil
}

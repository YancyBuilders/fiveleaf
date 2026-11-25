package tasks

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestParseJSONC(t *testing.T) {
	got, err := Parse([]byte(`// An existing Zed task can retain fields FiveLeaf does not use.
[
  {
    "label": "browse",
    "command": "yazi",
    "args": ["$FIVELEAF_FILE", "// text", "a,]b",],
    "cwd": "$FIVELEAF_WORKTREE_ROOT",
    "env": {"EDITOR": "ff --wait",},
    "use_new_terminal": true,
    "allow_concurrent_runs": true,
    "reveal": "always", /* accepted for compatibility */
  },
]`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Task{{Label: "browse", Command: "yazi", Args: []string{"$FIVELEAF_FILE", "// text", "a,]b"},
		Cwd: "$FIVELEAF_WORKTREE_ROOT", Env: map[string]string{"EDITOR": "ff --wait"},
		UseNewTerminal: true, AllowConcurrentRuns: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %#v, want %#v", got, want)
	}
	for _, data := range []string{"", "// no tasks\n", "[]"} {
		if got, err := Parse([]byte(data)); err != nil || len(got) != 0 {
			t.Errorf("Parse(%q) = %v, %v", data, got, err)
		}
	}
}

func TestParseReportsInvalidTask(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"object", `{}`, "JSON array"},
		{"null", `null`, "JSON array"},
		{"syntax", `[`, "tasks:"},
		{"entry type", `[7]`, "task 1:"},
		{"label", `[{"command":"yazi"}]`, "task 1: label"},
		{"blank command", `[{"label":"browse","command":"  "}]`, `task 1 ("browse"): command`},
		{"command type", `[{"label":"browse","command":9}]`, `task 1 ("browse")`},
		{"duplicate", `[{"label":"browse","command":"a"},{"label":"browse","command":"b"}]`, `task 2 ("browse"): label also appears in task 1`},
		{"NUL command", `[{"label":"browse","command":"a\u0000b"}]`, "command contains a NUL"},
		{"NUL argument", `[{"label":"browse","command":"yazi","args":["a\u0000b"]}]`, "args[0] contains a NUL"},
		{"NUL cwd", `[{"label":"browse","command":"yazi","cwd":"a\u0000b"}]`, "cwd contains a NUL"},
		{"NUL env", `[{"label":"browse","command":"yazi","env":{"FOO":"a\u0000b"}}]`, "env.FOO contains a NUL"},
		{"env equals", `[{"label":"browse","command":"yazi","env":{"FOO=BAR":"x"}}]`, "invalid environment variable name"},
		{"env digit", `[{"label":"browse","command":"yazi","env":{"9FOO":"x"}}]`, "invalid environment variable name"},
		{"env empty", `[{"label":"browse","command":"yazi","env":{"":"x"}}]`, "invalid environment variable name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMergeOverridesWithoutReordering(t *testing.T) {
	builtins := Builtins()
	global := []Task{{Label: "lazygit", Command: "lg"}, {Label: "test", Command: "go test"}}
	project := []Task{{Label: "test", Command: "make test"}, {Label: "build", Command: "make"}}
	got := Merge(builtins, global, project)
	want := []Task{{Label: "lazygit", Command: "lg"}, {Label: "yazi", Command: "yazi"},
		{Label: "test", Command: "make test"}, {Label: "build", Command: "make"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge = %#v, want %#v", got, want)
	}
	if builtins[0].Command != "lazygit" || global[1].Command != "go test" {
		t.Fatal("Merge changed an input definition")
	}
	got[0].Command = "changed"
	if Builtins()[0].Command != "lazygit" {
		t.Fatal("Builtins shared its task slice")
	}
}

func TestResolveKeepsShellSourceAndArgumentDataSeparate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project 'quoted' $(touch marker)")
	file := filepath.Join(root, "src", "hello $USER's.go")
	task := Task{Label: "inspect", Command: `printf '%s\n' "$FIVELEAF_FILE"; echo $UNDEFINED`,
		Args: []string{"$FIVELEAF_FILE", "${ZED_WORKTREE_ROOT}", "${CUSTOM}", "$(touch marker)", "$", "$$", "$$HOME", "$?", ""},
		Cwd:  "${FIVELEAF_DIRNAME}", Env: map[string]string{"CUSTOM": "value 'quoted' $(touch marker) $$"}}
	got, err := Resolve(task, Context{Root: root, File: file, Row: 12, Column: 7, Environ: []string{"USER=someone"}})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{file, root, "value 'quoted' $(touch marker) $$", "$(touch marker)", "$", "$$", "$$HOME", "$?", ""}
	if got.Task.Command != task.Command || !slices.Equal(got.Args, wantArgs) || got.Dir != filepath.Dir(file) {
		t.Fatalf("Resolve = %#v, want unchanged command, %q, dir %q", got, wantArgs, filepath.Dir(file))
	}
	if task.Args[0] != "$FIVELEAF_FILE" || task.Env["CUSTOM"] != "value 'quoted' $(touch marker) $$" {
		t.Fatal("Resolve changed the source task")
	}
	env := envMap(got.Env)
	for _, prefix := range []string{"FIVELEAF_", "ZED_"} {
		for key, want := range map[string]string{
			"WORKTREE_ROOT": root, "FILE": file, "DIRNAME": filepath.Dir(file), "FILENAME": filepath.Base(file),
			"RELATIVE_FILE": filepath.Join("src", filepath.Base(file)), "ROW": "12", "COLUMN": "7",
		} {
			if env[prefix+key] != want {
				t.Errorf("%s = %q, want %q", prefix+key, env[prefix+key], want)
			}
		}
	}
}

func TestResolveEnvironmentOverrides(t *testing.T) {
	root := t.TempDir()
	task := Task{Label: "run", Command: "tool", Args: []string{"$PATH", "${A}", "$B", "${EMPTY}", "$RAW"}, Cwd: "$OUTPUT",
		Env: map[string]string{"PATH": "$PATH:./bin", "A": "$B", "B": "task", "EMPTY": "", "OUTPUT": "output 'quoted'", "RAW": "$INHERITED"}}
	ctx := Context{Root: root, Environ: []string{"PATH=/usr/bin", "B=inherited", "INHERITED=$UNDEFINED $(literal)", "UNCHANGED=retain"}}
	got, err := Resolve(task, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/usr/bin:./bin", "inherited", "task", "", "$UNDEFINED $(literal)"}; !slices.Equal(got.Args, want) {
		t.Fatalf("Args = %q, want %q", got.Args, want)
	}
	if got.Dir != filepath.Join(root, "output 'quoted'") {
		t.Errorf("Dir = %q", got.Dir)
	}
	env := envMap(got.Env)
	if env["PATH"] != "/usr/bin:./bin" || env["A"] != "inherited" || env["B"] != "task" {
		t.Errorf("overrides = %v", got.Env)
	}
	if _, ok := env["UNCHANGED"]; ok {
		t.Error("Env repeats inherited variables")
	}
	keys := make([]string, 0, len(got.Env))
	for _, entry := range got.Env {
		key, _, _ := strings.Cut(entry, "=")
		keys = append(keys, key)
	}
	if !slices.IsSorted(keys) {
		t.Errorf("Env is not sorted by name: %v", got.Env)
	}
	if _, err := Resolve(Task{Label: "run", Command: "tool", Env: map[string]string{"NEW": "x", "OTHER": "$NEW"}}, ctx); err == nil || !strings.Contains(err.Error(), "undefined variable $NEW") {
		t.Errorf("task env unexpectedly expands another task env value: %v", err)
	}
}

func TestResolveDirectories(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct{ cwd, want string }{
		{"", root}, {"sub/folder", filepath.Join(root, "sub", "folder")},
		{"~", home}, {"~/folder", filepath.Join(home, "folder")}, {home, home},
	} {
		got, err := Resolve(Task{Label: "browse", Command: "yazi", Cwd: tc.cwd}, Context{Root: root})
		if err != nil || got.Dir != tc.want {
			t.Errorf("Resolve cwd %q = %q, %v; want %q", tc.cwd, got.Dir, err, tc.want)
		}
	}
	got, err := Resolve(Task{Label: "inspect", Command: "cat", Args: []string{"$FIVELEAF_FILE", "$ZED_ROW", "$ZED_COLUMN"}}, Context{Root: root, File: "unwritten.go"})
	if err != nil || !slices.Equal(got.Args, []string{filepath.Join(root, "unwritten.go"), "1", "1"}) {
		t.Fatalf("relative file and default cursor = %q, %v", got.Args, err)
	}
}

func TestResolveWindowsEnvironmentNames(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "main.go")
	context := Context{Root: root, File: file, Environ: []string{
		"PATH=stale", "Path=C:\\Windows\\System32", "Output=old", "fiveleaf_file=stale.go",
	}}
	task := Task{Label: "run", Command: "tool", Args: []string{"$PATH", "${Path}", "$path", "$fiveleaf_file", "$Zed_File"}, Cwd: "$output",
		Env: map[string]string{"Path": "${pAtH};C:\\tools", "OUTPUT": "output"}}
	got, err := resolve(task, context, true)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := "C:\\Windows\\System32;C:\\tools"
	if want := []string{wantPath, wantPath, wantPath, file, file}; !slices.Equal(got.Args, want) {
		t.Fatalf("Windows arguments = %q, want %q", got.Args, want)
	}
	if got.Dir != filepath.Join(root, "output") {
		t.Errorf("Windows cwd = %q, want task override", got.Dir)
	}
	env := envMap(got.Env)
	if env["PATH"] != wantPath || env["OUTPUT"] != "output" {
		t.Fatalf("Windows overrides = %q", got.Env)
	}
	seen := map[string]bool{}
	for _, entry := range got.Env {
		name, _, _ := strings.Cut(entry, "=")
		key := strings.ToUpper(name)
		if seen[key] {
			t.Errorf("Windows overrides repeat %q: %q", name, got.Env)
		}
		seen[key] = true
	}
	// Even differently spelled keys in one task produce a single stable value.
	task.Env["PATH"] = "earlier spelling"
	got, err = resolve(task, context, true)
	if err != nil || envMap(got.Env)["PATH"] != wantPath || len(got.Env) != len(seen) {
		t.Fatalf("case-variant overrides were not deduplicated: %q, %v", got.Env, err)
	}
}

func TestResolvePOSIXEnvironmentNamesRemainDistinct(t *testing.T) {
	context := Context{Root: t.TempDir(), Environ: []string{"PATH=/usr/bin", "Path=/custom/bin"}}
	task := Task{Label: "run", Command: "tool", Args: []string{"$PATH", "${Path}"},
		Env: map[string]string{"PATH": "$PATH:/bin", "Path": "$Path:./bin"}}
	got, err := resolve(task, context, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/usr/bin:/bin", "/custom/bin:./bin"}; !slices.Equal(got.Args, want) {
		t.Fatalf("POSIX arguments = %q, want %q", got.Args, want)
	}
	if env := envMap(got.Env); env["PATH"] != "/usr/bin:/bin" || env["Path"] != "/custom/bin:./bin" {
		t.Fatalf("POSIX variable spellings were merged: %q", got.Env)
	}
	task.Args = []string{"$path"}
	if _, err := resolve(task, context, false); err == nil || !strings.Contains(err.Error(), "undefined variable $path") {
		t.Fatalf("POSIX accepted a variable with the wrong case: %v", err)
	}
}

func TestResolveWindowsMissingFileContextWithMixedCase(t *testing.T) {
	context := Context{Root: t.TempDir(), Environ: []string{"Fiveleaf_File=stale.go", "Zed_File=stale.go"}}
	for _, name := range []string{"fiveleaf_file", "Zed_File"} {
		_, err := resolve(Task{Label: "run", Command: "tool", Args: []string{"$" + name}}, context, true)
		if err == nil || !strings.Contains(err.Error(), "$"+name+" needs an active file") {
			t.Errorf("Windows missing file variable %q: %v", name, err)
		}
	}
	got, err := resolve(Task{Label: "run", Command: "tool"}, context, true)
	if err != nil {
		t.Fatal(err)
	}
	if env := envMap(got.Env); env["FIVELEAF_FILE"] != "" || env["ZED_FILE"] != "" || len(env) != 14 {
		t.Fatalf("Windows retained stale file context: %q", got.Env)
	}
}

func TestResolveMissingFileContext(t *testing.T) {
	ctx := Context{Root: t.TempDir(), Environ: []string{"FIVELEAF_FILE=inherited.go", "ZED_FILE=inherited.go"}}
	for _, prefix := range []string{"FIVELEAF_", "ZED_"} {
		for _, variable := range []string{"FILE", "DIRNAME", "FILENAME", "RELATIVE_FILE", "ROW", "COLUMN"} {
			for _, field := range []string{"args", "cwd", "env"} {
				task := Task{Label: "run", Command: "tool"}
				value := "$" + prefix + variable
				switch field {
				case "args":
					task.Args = []string{value}
				case "cwd":
					task.Cwd = value
				case "env":
					task.Env = map[string]string{"CUSTOM": value}
				}
				if _, err := Resolve(task, ctx); err == nil || !strings.Contains(err.Error(), value+" needs an active file") {
					t.Errorf("%s %s: %v", field, value, err)
				}
			}
		}
	}
	if _, err := Resolve(Task{Label: "run", Command: `echo "$FIVELEAF_FILE"`}, ctx); err != nil {
		t.Errorf("Resolve inspected shell source: %v", err)
	}
}

func TestResolveErrorsHaveTaskAndField(t *testing.T) {
	ctx := Context{Root: t.TempDir(), Environ: []string{"BAD=has\x00nul"}}
	for _, tc := range []struct {
		task Task
		want string
	}{
		{Task{Label: "run", Command: "tool", Args: []string{"$UNDEFINED"}}, "args[0]: undefined variable $UNDEFINED"},
		{Task{Label: "run", Command: "tool", Cwd: "${UNDEFINED}"}, "cwd: undefined variable $UNDEFINED"},
		{Task{Label: "run", Command: "tool", Env: map[string]string{"X": "$UNDEFINED"}}, "env.X: undefined variable $UNDEFINED"},
		{Task{Label: "run", Command: "tool", Args: []string{"${HOME"}}, "args[0]: unclosed variable"},
		{Task{Label: "run", Command: "tool", Args: []string{"${HOME:-default}"}}, "args[0]: invalid variable expression"},
		{Task{Label: "run", Command: "tool", Args: []string{"$BAD"}}, "args[0]: value contains a NUL"},
		{Task{Label: "run", Command: "tool", Env: map[string]string{"INVALID-NAME": "x"}}, "invalid environment variable name"},
	} {
		_, err := Resolve(tc.task, ctx)
		if err == nil || !strings.Contains(err.Error(), `task "run"`) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Resolve(%+v) = %v, want task and %q", tc.task, err, tc.want)
		}
	}
	for _, root := range []string{"", "relative", string(filepath.Separator) + "has\x00nul"} {
		if _, err := Resolve(Builtins()[0], Context{Root: root}); err == nil || !strings.Contains(err.Error(), "workspace root") {
			t.Errorf("root %q: %v", root, err)
		}
	}
}

func TestResolveClearsInheritedFileContextForShellCommands(t *testing.T) {
	root := t.TempDir()
	ctx := Context{Root: root}
	for _, prefix := range []string{"FIVELEAF_", "ZED_"} {
		for _, suffix := range []string{"WORKTREE_ROOT", "FILE", "DIRNAME", "FILENAME", "RELATIVE_FILE", "ROW", "COLUMN"} {
			ctx.Environ = append(ctx.Environ, prefix+suffix+"=stale-parent-context")
		}
	}
	command := `printf '%s\n' "$FIVELEAF_FILE" "$ZED_ROW"`
	got, err := Resolve(Task{Label: "show context", Command: command}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.Command != command {
		t.Fatal("Resolve changed the shell source")
	}
	env := envMap(got.Env)
	if len(env) != 14 {
		t.Fatalf("only %d context variables override the inherited environment", len(env))
	}
	for _, prefix := range []string{"FIVELEAF_", "ZED_"} {
		for _, suffix := range []string{"FILE", "DIRNAME", "FILENAME", "RELATIVE_FILE", "ROW", "COLUMN"} {
			value, ok := env[prefix+suffix]
			if !ok || value != "" {
				t.Errorf("%s retains inherited context: value=%q set=%v", prefix+suffix, value, ok)
			}
		}
		if env[prefix+"WORKTREE_ROOT"] != root {
			t.Errorf("%sWORKTREE_ROOT = %q, want %q", prefix, env[prefix+"WORKTREE_ROOT"], root)
		}
	}
}

func envMap(entries []string) map[string]string {
	values := map[string]string{}
	for _, entry := range entries {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	return values
}

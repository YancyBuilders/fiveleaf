package tasks

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestResolveYaziActiveFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a file's $HOME.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, definition string
		want             []string
	}{
		{"builtin", "", []string{file}},
		{"old template", `{"label":"yazi","command":"yazi"}`, []string{file}},
		{"other label", `{"label":"browse","command":"yazi"}`, nil},
		{"custom command", `{"label":"yazi","command":"yazi --clear-cache"}`, nil},
		{"explicit empty args", `{"label":"yazi","command":"yazi","args":[]}`, nil},
		{"explicit args", `{"label":"yazi","command":"yazi","args":["fixed"]}`, []string{"fixed"}},
		{"custom cwd", `{"label":"yazi","command":"yazi","cwd":"sub"}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			definitions := Builtins()
			if test.definition != "" {
				var err error
				definitions, err = Parse([]byte("[" + test.definition + "]"))
				if err != nil {
					t.Fatal(err)
				}
			}
			task := definitions[len(definitions)-1]
			resolved, err := Resolve(task, Context{Root: root, File: file})
			if err != nil || !slices.Equal(resolved.Args, test.want) || resolved.Task.Command != task.Command {
				t.Fatalf("Resolve(%s): command %q, args %q, err %v; want %q %q", test.name, resolved.Task.Command, resolved.Args, err, task.Command, test.want)
			}
		})
	}
	for _, unavailable := range []string{"", root, filepath.Join(root, "deleted"), "/ssh:example.invalid/a.txt"} {
		resolved, err := Resolve(Task{Label: "yazi", Command: "yazi"}, Context{Root: root, File: unavailable})
		if err != nil || len(resolved.Args) != 0 || resolved.Dir != root {
			t.Errorf("unavailable file %q: dir %q, args %q, err %v", unavailable, resolved.Dir, resolved.Args, err)
		}
	}
}

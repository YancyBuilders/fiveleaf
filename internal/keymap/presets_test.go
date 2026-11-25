package keymap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPlatformBindings(t *testing.T) {
	data := []byte(`[
		{"key":"f5", "mac":"cmd+k cmd+s", "win":"ctrl+k ctrl+s", "linux":"ctrl+alt+s", "command":"file.keybindings"},
		{"key":"CommandOrControl+/", "command":"sel.comment", "when":"editorTextFocus", "args":{"keep":true}},
		{"key":"f6", "command":"file.save"}
	]`)
	for _, tc := range []struct {
		os, keys string
		primary  ui.Modifiers
	}{
		{"darwin", "cmd+k cmd+s", ui.Super},
		{"windows", "ctrl+k ctrl+s", ui.Ctrl},
		{"linux", "ctrl+alt+s", ui.Ctrl},
		{"freebsd", "f5", ui.Ctrl},
	} {
		t.Run(tc.os, func(t *testing.T) {
			m, err := ParseForOS(data, tc.os)
			if err != nil || len(m.Errors) > 0 || len(m.Bindings) != 3 {
				t.Fatalf("parse: %v, %+v", err, m)
			}
			want, _ := ParseKeys(tc.keys)
			if !slices.Equal(m.Bindings[0].Keys, want) {
				t.Errorf("platform keys %v, want %v", m.Bindings[0].Keys, want)
			}
			b := m.Bindings[1]
			if !slices.Equal(b.Keys, []Stroke{{tc.primary, ui.KeySlash}}) || string(b.Args) != `{"keep":true}` || b.When.Eval(nil) {
				t.Errorf("primary modifier, args or context lost: %+v", b)
			}
			if got := m.Last("file.save"); !slices.Equal(got, []Stroke{{0, ui.KeyF6}}) {
				t.Errorf("fallback keys %v", got)
			}
		})
	}
}

func TestEditPreservesPlatformBindings(t *testing.T) {
	want := Entry{Key: "f5", Mac: "cmd+k cmd+s", Win: "ctrl+k ctrl+s", Linux: "ctrl+alt+s", Command: "file.keybindings"}
	data, err := Edit(nil, nil, []Entry{want})
	if err != nil {
		t.Fatal(err)
	}
	var entries []Entry
	if err := json.Unmarshal(StripJSONC(data), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Key != want.Key || entries[0].Mac != want.Mac || entries[0].Win != want.Win || entries[0].Linux != want.Linux {
		t.Fatalf("platform overrides lost: %s", data)
	}
}

func TestRemovedChordDoesNotCapturePrefix(t *testing.T) {
	m := FromEntries([]Entry{
		{Key: "ctrl+k ctrl+s", Command: "save"},
		{Key: "ctrl+j ctrl+s", Command: "save"},
		{Key: "ctrl+k ctrl+s", Command: "-save", When: "editorTextFocus"},
	}, "linux")
	editor := func(name string) bool { return name == "editorTextFocus" }
	prefix, _ := ParseKeys("ctrl+k")
	if b, more := m.Lookup(prefix, editor); b != nil || more {
		t.Fatalf("removed chord captured prefix: %v, %v", b, more)
	}
	if b, more := m.Lookup(prefix, nil); b != nil || !more {
		t.Fatalf("conditional removal applied outside editor: %v, %v", b, more)
	}
	other, _ := ParseKeys("ctrl+j ctrl+s")
	if b, more := m.Lookup(other, editor); b == nil || b.Command != "save" || more {
		t.Fatalf("removal took a different chord: %v, %v", b, more)
	}
	m.Bindings = append(m.Bindings, FromEntries([]Entry{{Key: "ctrl+k ctrl+s", Command: "save"}}, "linux").Bindings...)
	if b, more := m.Lookup(prefix, editor); b != nil || !more {
		t.Fatal("binding after a removal did not restore the chord")
	}
}

func TestLastSkipsRemovedBindings(t *testing.T) {
	m := FromEntries([]Entry{
		{Key: "f5", Command: "save"},
		{Key: "f6", Command: "save"},
		{Key: "f6", Command: "-save"},
	}, "linux")
	if got := KeysString(m.Last("save")); got != "f5" {
		t.Fatalf("last keys = %q, want f5", got)
	}
	m.Bindings = append(m.Bindings, FromEntries([]Entry{{Key: "f5", Command: "-save"}}, "linux").Bindings...)
	if got := m.Last("save"); got != nil {
		t.Fatalf("still displays removed keys %v", got)
	}
}

func TestInvalidWhenOperators(t *testing.T) {
	for _, expr := range []string{"&", "|", "=", "a & b", "a | b", "a = b", "a && &", "a || |"} {
		t.Run(expr, func(t *testing.T) {
			if _, err := ParseWhen(expr); err == nil {
				t.Fatalf("accepted %q", expr)
			}
		})
	}
	w, err := ParseWhen("editorTextFocus\n&&\r!terminalFocus")
	if err != nil || !w.Eval(func(s string) bool { return s == "editorTextFocus" }) {
		t.Fatalf("multiline condition: %v", err)
	}
}

func marketplacePresets(t *testing.T) []Preset {
	t.Helper()
	var presets []Preset
	for _, name := range []string{"vscode", "jetbrains", "sublime"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "extensions", "keymap-"+name, "plugin.json"))
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct{ Contributes struct{ Keymaps []Preset } }
		if err := json.Unmarshal(data, &manifest); err != nil || len(manifest.Contributes.Keymaps) != 1 {
			t.Fatalf("%s: invalid preset: %v", name, err)
		}
		preset := manifest.Contributes.Keymaps[0]
		if preset.ID != "keymap-"+name+".preset" {
			t.Fatalf("unexpected %s preset ID: %s", name, preset.ID)
		}
		preset.ID = name
		presets = append(presets, preset)
	}
	return presets
}

func TestMarketplacePresets(t *testing.T) {
	var ids []string
	for _, preset := range marketplacePresets(t) {
		if slices.Contains(ids, preset.ID) {
			t.Fatalf("duplicate preset %q", preset.ID)
		}
		ids = append(ids, preset.ID)
		for _, goos := range []string{"darwin", "windows", "linux"} {
			t.Run(preset.Label+"/"+goos, func(t *testing.T) {
				m := FromEntries(preset.Bindings, goos)
				if len(m.Errors) > 0 || len(m.Bindings) != len(preset.Bindings) {
					t.Fatalf("preset did not parse: %v", m.Errors)
				}
				seen := map[string]string{}
				for _, b := range m.Bindings {
					keys := KeysString(b.Keys)
					if other, exists := seen[keys]; exists {
						t.Errorf("%s binds both %s and %s", keys, other, b.Command)
					}
					seen[keys] = b.Command
					if strings.HasPrefix(b.Command, "sel.") && b.When.Eval(func(name string) bool { return name == "terminalFocus" }) {
						t.Errorf("%s runs in the terminal", b.Command)
					}
				}
			})
		}
	}
	if !slices.Equal(ids, []string{"vscode", "jetbrains", "sublime"}) {
		t.Fatalf("presets %q", ids)
	}
}

func TestPresetShortcutsAcrossPlatforms(t *testing.T) {
	for _, tc := range []struct{ preset, goos, key, command string }{
		{"vscode", "darwin", "cmd+k cmd+s", "file.keybindings"},
		{"vscode", "windows", "shift+alt+f", "sel.format"},
		{"vscode", "linux", "ctrl+shift+i", "sel.format"},
		{"jetbrains", "darwin", "cmd+shift+o", "go.quickOpen"},
		{"jetbrains", "windows", "ctrl+shift+n", "go.quickOpen"},
		{"jetbrains", "linux", "ctrl+y", "sel.deleteLine"},
		{"jetbrains", "darwin", "cmd+backspace", "sel.deleteLine"},
		{"sublime", "darwin", "cmd+k cmd+b", "view.sidebar"},
		{"sublime", "windows", "ctrl+shift+d", "sel.copyDown"},
	} {
		t.Run(tc.preset+"/"+tc.goos+"/"+tc.command, func(t *testing.T) {
			presets := marketplacePresets(t)
			i := slices.IndexFunc(presets, func(p Preset) bool { return p.ID == tc.preset })
			m := FromEntries(presets[i].Bindings, tc.goos)
			keys, _ := ParseKeys(tc.key)
			b, prefix := m.Lookup(keys, func(name string) bool { return name == "editorTextFocus" || name == "editorFocus" })
			if b == nil || prefix || b.Command != tc.command {
				t.Fatalf("%s resolved to %v (prefix %v), want %s", tc.key, b, prefix, tc.command)
			}
		})
	}
}

func TestLookupPrioritizesBindingLayers(t *testing.T) {
	chord := Entry{Key: "ctrl+k ctrl+s", Command: "chord"}
	single := Entry{Key: "ctrl+k", Command: "single"}
	conditional := single
	conditional.When = "editorTextFocus"
	for _, tc := range []struct {
		name       string
		entries    []Entry
		priorities []int
		editor     bool
		want       string
		prefix     bool
	}{
		{"user single overrides inherited chord", []Entry{chord, single}, []int{0, 1}, false, "single", false},
		{"inactive user single leaves inherited chord", []Entry{chord, conditional}, []int{0, 1}, false, "", true},
		{"active user single overrides inherited chord", []Entry{chord, conditional}, []int{0, 1}, true, "single", false},
		{"user chord overrides inherited single", []Entry{single, chord}, []int{0, 1}, false, "", true},
		{"priority precedes file order", []Entry{single, chord}, []int{1, 0}, false, "single", false},
		{"same layer keeps chord before single", []Entry{chord, single}, []int{0, 0}, false, "", true},
		{"same layer keeps chord after single", []Entry{single, chord}, []int{0, 0}, false, "", true},
		{"removed user chord leaves inherited single", []Entry{single, chord, {Key: chord.Key, Command: "-chord"}}, []int{0, 1, 1}, false, "single", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := FromEntries(tc.entries, "linux")
			for i := range m.Bindings {
				m.Bindings[i].Priority = tc.priorities[i]
			}
			keys, _ := ParseKeys("ctrl+k")
			b, prefix := m.Lookup(keys, func(name string) bool { return tc.editor && name == "editorTextFocus" })
			got := ""
			if b != nil {
				got = b.Command
			}
			if got != tc.want || prefix != tc.prefix {
				t.Fatalf("got command %q, prefix %v; want command %q, prefix %v", got, prefix, tc.want, tc.prefix)
			}
		})
	}
}

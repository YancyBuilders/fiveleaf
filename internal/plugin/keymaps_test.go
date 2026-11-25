package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/internal/keymap"
)

const keymapManifest = `{
  "name": "team-keys",
  "contributes": {
    "keymaps": [{
      "id": "team-keys.editor", "label": "Team Editor",
      "bindings": [
        {"key": "ctrl+k ctrl+s", "mac": "cmd+k cmd+s", "win": "ctrl+alt+s", "linux": "ctrl+k s", "command": "file.keybindings"},
        {"key": "ctrl+alt+j", "mac": "cmd+alt+j", "command": "runCommands", "when": "editorTextFocus && !editorReadonly", "args": {"commands": ["sel.joinLines", "file.save"]}}
      ]
    }]
  }
}`

func TestKeymapManifest(t *testing.T) {
	m, err := ParseManifest([]byte(keymapManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.HasCode() || len(m.Contributes.Keymaps) != 1 {
		t.Fatalf("static keymap plugin: %+v", m)
	}
	k := m.Contributes.Keymaps[0]
	if k.ID != "team-keys.editor" || k.Label != "Team Editor" || len(k.Bindings) != 2 {
		t.Fatalf("keymap: %+v", k)
	}
	for _, tc := range []struct{ goos, keys string }{
		{"darwin", "cmd+k cmd+s"},
		{"windows", "ctrl+alt+s"},
		{"linux", "ctrl+k s"},
	} {
		parsed := keymap.FromEntries(k.Bindings, tc.goos)
		if got := keymap.KeysString(parsed.Bindings[0].Keys); got != tc.keys {
			t.Errorf("%s: got %q, want %q", tc.goos, got, tc.keys)
		}
		if b := parsed.Bindings[1]; b.When == nil || !b.When.Eval(func(name string) bool { return name == "editorTextFocus" }) || !strings.Contains(string(b.Args), "sel.joinLines") {
			t.Errorf("%s lost condition or args: %+v", tc.goos, b)
		}
	}
}

func TestKeymapManifestChecks(t *testing.T) {
	for _, tc := range []struct {
		name, keymaps, want string
	}{
		{"namespace", `[{"id":"other.editor","label":"Editor"}]`, `must start with "team-keys."`},
		{"empty suffix", `[{"id":"team-keys.","label":"Editor"}]`, `must start with "team-keys."`},
		{"duplicate", `[{"id":"team-keys.editor","label":"Editor"},{"id":"team-keys.editor","label":"Other"}]`, "declared twice"},
		{"label", `[{"id":"team-keys.editor","label":"  "}]`, "has no label"},
		{"command", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"f9"}]}]`, "binding 1 has no command"},
		{"blank command", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"f9","command":"  "}]}]`, "binding 1 has no command"},
		{"empty removal", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"f9","command":"-"}]}]`, "binding 1 has no command"},
		{"key", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"ctrl+no-such-key","command":"file.save"}]}]`, "binding 1 (darwin)"},
		{"missing key", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"command":"file.save"}]}]`, "binding 1 (darwin)"},
		{"mac", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"ctrl+s","mac":"cmd+no-such-key","command":"file.save"}]}]`, "binding 1 (darwin)"},
		{"windows", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"ctrl+s","win":"ctrl+no-such-key","command":"file.save"}]}]`, "binding 1 (windows)"},
		{"linux", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"ctrl+s","linux":"ctrl+no-such-key","command":"file.save"}]}]`, "binding 1 (linux)"},
		{"condition", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"f9","command":"file.save","when":"editorTextFocus && ("}]}]`, "binding 1 (darwin)"},
		{"single operator", `[{"id":"team-keys.editor","label":"Editor","bindings":[{"key":"f9","command":"file.save","when":"editorTextFocus & inputFocus"}]}]`, "binding 1 (darwin)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(`{"name":"team-keys","contributes":{"keymaps":` + tc.keymaps + `}}`))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStaticKeymapPluginLifecycle(t *testing.T) {
	r, h, dirs := newRegistry(t)
	writePlugin(t, dirs.Plugins, "team-keys", map[string]string{ManifestFile: keymapManifest})
	r.Load()
	p := r.Plugin("team-keys")
	if p == nil || p.Stats().State != StateStatic || len(r.Enabled()) != 1 || len(r.Enabled()[0].Contributes.Keymaps) != 1 {
		t.Fatalf("static keymap not enabled: %+v", p)
	}
	h.settings["team-keys"] = json.RawMessage(`{"enabled":false}`)
	r.Toggled("team-keys")
	if len(r.Enabled()) != 0 || p.Stats().State != StateOff {
		t.Fatal("disabled plugin still contributes keymaps")
	}
	h.settings["team-keys"] = json.RawMessage(`{"enabled":true}`)
	r.Toggled("team-keys")
	if len(r.Enabled()) != 1 || p.Stats().State != StateStatic {
		t.Fatal("reenabled static plugin did not contribute keymaps")
	}
	updated := strings.ReplaceAll(keymapManifest, `"Team Editor"`, `"Team Editor 2"`)
	writePlugin(t, dirs.Plugins, "team-keys", map[string]string{ManifestFile: updated})
	r.Load()
	if got := r.Enabled()[0].Contributes.Keymaps[0].Label; got != "Team Editor 2" {
		t.Fatalf("reloaded label: %q", got)
	}
	if err := os.Remove(filepath.Join(dirs.Plugins, "team-keys", ManifestFile)); err != nil {
		t.Fatal(err)
	}
	r.Load()
	if len(r.Enabled()) != 0 {
		t.Fatal("removed manifest still contributes keymaps")
	}
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/keymap"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func presetBinding(t *testing.T, a *app, key string, where keyPlace) *keymap.Binding {
	t.Helper()
	keys, err := keymap.ParseKeys(key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := a.keys.Lookup(keys, a.whenContext(where))
	return b
}

func TestPresetCommandsExist(t *testing.T) {
	hs := newHarness(t, false)
	useKeymapFixtures(hs)
	for _, p := range keymapPresets() {
		for _, e := range p.Bindings {
			if hs.a.commandID(strings.TrimPrefix(e.Command, "-")) == "" {
				t.Errorf("%s refers to unknown command %s", p.ID, e.Command)
			}
		}
	}
}

func TestPrefsKeymapSwitchAndPersonalKeys(t *testing.T) {
	was := hostOS
	hostOS = "darwin"
	t.Cleanup(func() { hostOS = was })
	ph := newPrefsHarness(t)
	useKeymapFixtures(ph.harness)
	k := &ph.w.keys
	ph.w.section = prefsShortcuts
	ph.frames()
	if err := ph.pt.Click("FiveLeaf"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if err := ph.pt.Click("JetBrains"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if ph.m.s.Keymap != "jetbrains" {
		t.Fatalf("selector chose %q", ph.m.s.Keymap)
	}
	ph.shot("keymap-jetbrains")
	var cmd command
	for _, r := range k.rows(ph.a) {
		if r.cmd.id == "go.quickOpen" {
			cmd = r.cmd
			if r.source != keyPreset || keymap.KeysString(r.keys) != "shift+cmd+o" {
				t.Fatalf("quick open row: %+v", r)
			}
		}
	}
	if cmd.id == "" || ph.a.menuAccel(cmd) != "" {
		t.Fatal("old quick open menu shortcut still active")
	}
	ph.tt.Key(ui.Super|ui.Shift, ui.KeyO)
	ph.settle()
	if !ph.a.quick.open {
		t.Fatal("JetBrains Quick Open did not open")
	}
	ph.tt.Key(0, ui.KeyEscape)
	ph.settle()
	custom, _ := keymap.ParseKeys("f6")
	k.setKeys(cmd, custom)
	if b := presetBinding(t, ph.a, "f6", placeNone); b == nil || b.Command != cmd.id {
		t.Fatalf("personal shortcut: %+v", b)
	}
	if b := presetBinding(t, ph.a, "cmd+shift+o", placeNone); b != nil {
		t.Fatalf("inherited shortcut remained: %+v", b)
	}
	path := keymap.Path(ph.m.s.Path())
	before, _ := os.ReadFile(path)
	ph.w.set("keymap", json.RawMessage(`"vscode"`))
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("switching preset rewrote personal bindings")
	}
	if b := presetBinding(t, ph.a, "cmd+p", placeNone); b != nil {
		t.Fatalf("preset overrode custom shortcut: %+v", b)
	}
	k.resetKeys(cmd)
	if b := presetBinding(t, ph.a, "cmd+p", placeNone); b == nil || b.Command != cmd.id {
		t.Fatalf("reset did not restore VS Code: %+v", b)
	}
	k.removeKeys(cmd)
	if b := presetBinding(t, ph.a, "cmd+p", placeNone); b != nil {
		t.Fatalf("removed shortcut remained: %+v", b)
	}
	ph.m.s.Flush()
	if loaded := settings.Load(ph.m.s.Path()); loaded.Keymap != "vscode" {
		t.Fatalf("saved keymap = %q", loaded.Keymap)
	}
}

func TestPresetInstalledAfterSavingSelection(t *testing.T) {
	ph := newPrefsHarness(t)
	host := withPlugins(ph.harness, nil)
	ph.w.set("keymap", json.RawMessage(`"vscode"`))
	if _, ok := selectedKeymap("vscode"); ok || len(ph.a.presetKeys.Bindings) != 0 {
		t.Fatal("a fresh profile already has VS Code bindings")
	}
	installCatalogFixtures(ph.harness, host, "keymap-jetbrains")
	ph.settle()
	if ph.m.s.Keymap != "vscode" || len(ph.a.presetKeys.Bindings) != 0 {
		t.Fatal("installing an unrelated keymap changed the saved selection")
	}
	installCatalogFixtures(ph.harness, host, "keymap-vscode")
	ph.settle()
	preset, ok := selectedKeymap("keymap-vscode.preset")
	if !ok || preset.ID != "vscode" || ph.m.s.Keymap != "vscode" || len(ph.a.presetKeys.Bindings) == 0 {
		t.Fatal("installing the selected plugin did not restore the saved keymap")
	}
}

func TestPresetEditingFocusAndHints(t *testing.T) {
	was := hostOS
	hostOS = "darwin"
	t.Cleanup(func() { hostOS = was })
	hs := newHarness(t, false)
	useKeymapFixtures(hs)
	a := hs.a
	a.settings.Keymap = "jetbrains"
	a.applyKeymap(nil)
	for _, where := range []keyPlace{placeInput, placeTerminal, placeNone} {
		if b := presetBinding(t, a, "cmd+d", where); b != nil {
			t.Fatalf("duplicate line captured outside editor (%v): %+v", where, b)
		}
	}
	if b := presetBinding(t, a, "cmd+d", placeEditor); b == nil || b.Command != "sel.copyDown" {
		t.Fatalf("duplicate line: %+v", b)
	}
	if got := a.keyHint("sel.copyDown"); got != "⌘D" {
		t.Fatalf("conditional preset hint = %q", got)
	}
	if got := a.keyHint("view.sidebar"); got != "" {
		t.Fatalf("shadowed sidebar hint = %q", got)
	}
	a.openFile(hs.path("go.mod"), true)
	hs.settle()
	d := a.activeEditor()
	before := d.buf.String()
	hs.tt.Key(ui.Super, ui.KeyD)
	hs.settle()
	if d.buf.String() == before {
		t.Fatal("JetBrains Duplicate Line did not edit the document")
	}
	before = d.buf.String()
	hs.tt.Key(ui.Alt, ui.KeyDown)
	hs.settle()
	if d.buf.String() != before {
		t.Fatal("old Move Line Down shortcut remained active")
	}
	hs.tt.Key(ui.Alt|ui.Shift, ui.KeyDown)
	hs.settle()
	if d.buf.String() == before {
		t.Fatal("JetBrains Move Line Down did not edit the document")
	}
	withKeys(t, a, `[{"key":"f6","command":"workbench.action.quickOpen"}]`)
	if b := presetBinding(t, a, "cmd+shift+o", placeNone); b != nil {
		t.Fatalf("personal alias did not replace preset: %+v", b)
	}
	if got := a.keyHint("go.quickOpen"); got != "F6" {
		t.Fatalf("personal alias hint = %q", got)
	}
}

const keymapTestPlugin = `{
  "name":"team-keys",
  "contributes":{"keymaps":[{"id":"team-keys.editor","label":"Team Editor","bindings":[
    {"key":"ctrl+k ctrl+j","mac":"cmd+k cmd+j","command":"runCommands","when":"editorTextFocus","args":{"commands":["sel.joinLines","sel.comment"]}},
    {"key":"f6","command":"view.sidebar"}
  ]}]}
}`

func TestPluginKeymapAppliesAndReloads(t *testing.T) {
	was := hostOS
	hostOS = "darwin"
	t.Cleanup(func() { hostOS = was })
	ph := newPrefsHarness(t)
	h := withPlugins(ph.harness, map[string]map[string]string{"team-keys": {"plugin.json": keymapTestPlugin}})
	ph.w.set("keymap", json.RawMessage(`"team-keys.editor"`))
	if _, ok := selectedKeymap("team-keys.editor"); !ok {
		t.Fatal("static plugin preset missing from selector")
	}
	if b := presetBinding(t, ph.a, "f6", placeNone); b == nil || b.Command != "view.sidebar" {
		t.Fatalf("plugin shortcut: %+v", b)
	}
	file := ph.path("keymap.go")
	if err := os.WriteFile(file, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ph.a.openFile(file, true)
	ph.settle()
	ph.tt.Key(ui.Super, ui.KeyK)
	ph.tt.Key(ui.Super, ui.KeyJ)
	ph.settle()
	if got := ph.a.activeEditor().buf.String(); !strings.Contains(got, "// one two") {
		t.Fatalf("plugin chord with args produced %q", got)
	}
	path := filepath.Join(h.reg.Dirs.Plugins, "team-keys", "plugin.json")
	if err := os.WriteFile(path, []byte(strings.Replace(keymapTestPlugin, `"f6"`, `"f7"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	h.reg.Load()
	ph.settle()
	if b := presetBinding(t, ph.a, "f7", placeNone); b == nil {
		t.Fatal("reloaded preset not applied")
	}
	if b := presetBinding(t, ph.a, "f6", placeNone); b != nil {
		t.Fatal("old plugin keys remained")
	}
	ph.w.set("plugins", json.RawMessage(`{"team-keys":{"enabled":false}}`))
	ph.settle()
	if b := presetBinding(t, ph.a, "f7", placeNone); b != nil {
		t.Fatal("disabled static preset still applies")
	}
	if ph.m.s.Keymap != "team-keys.editor" {
		t.Fatal("disabled preset selection was lost")
	}
	ph.w.section = prefsShortcuts
	ph.frames()
	if !ph.pt.HasText(locales.KeysPresetUnavailable()) {
		t.Fatal("unavailable preset has no visible fallback notice")
	}
	ph.w.set("plugins", json.RawMessage(`{"team-keys":{"enabled":true}}`))
	ph.settle()
	if b := presetBinding(t, ph.a, "f7", placeNone); b == nil {
		t.Fatal("re-enabled static preset not applied")
	}
	ph.a.chord, _ = keymap.ParseKeys("cmd+k")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	h.reg.Load()
	ph.settle()
	if len(ph.a.chord) > 0 || len(ph.a.presetKeys.Bindings) > 0 {
		t.Fatal("removed plugin left bindings or a chord")
	}
}

func TestPresetUserRemovalAliases(t *testing.T) {
	hs := newHarness(t, false)
	useKeymapFixtures(hs)
	hs.a.settings.Keymap = "vscode"
	withKeys(t, hs.a, `[{"key":"cmd+k cmd+s","command":"-workbench.action.openGlobalKeybindings"},{"key":"ctrl+k ctrl+s","command":"-workbench.action.openGlobalKeybindings"}]`)
	keys := []keymap.Stroke{{Mods: ui.Cmd, Key: ui.KeyK}}
	// Hover still shares the same prefix, but removing its binding too releases it.
	bindings := slices.DeleteFunc(slices.Clone(hs.a.keys.Bindings), func(b keymap.Binding) bool { return b.Command == "go.hover" || b.Command == "file.saveAll" })
	_, more := (&keymap.Map{Bindings: bindings}).Lookup(keys, hs.a.whenContext(placeEditor))
	if more {
		t.Fatal("removed preset chord still captures its prefix")
	}
}

func TestPresetPersonalSingleOverridesChord(t *testing.T) {
	hs := newHarness(t, false)
	useKeymapFixtures(hs)
	hs.a.settings.Keymap = "vscode"
	withKeys(t, hs.a, `[{"key":"cmdorctrl+k","command":"file.new"}]`)
	hs.tt.Key(ui.Cmd, ui.KeyK)
	hs.settle()
	if len(hs.a.chord) > 0 || hs.a.activeEditor() == nil {
		t.Fatal("personal single key lost to an inherited chord")
	}
}

func TestPluginKeymapRefreshKeepsUnchangedChord(t *testing.T) {
	hs := newHarness(t, false)
	h := withPlugins(hs, map[string]map[string]string{"team-keys": {"plugin.json": keymapTestPlugin}})
	installCatalogFixtures(hs, h, "keymap-vscode")
	hs.a.settings.Keymap = "vscode"
	hs.a.applyKeymap(nil)
	hs.a.openFile(hs.path("go.mod"), true)
	hs.settle()
	if !hs.a.keyStroke(keymap.Stroke{Mods: ui.Cmd, Key: ui.KeyK}, placeEditor) {
		t.Fatal("chord did not start")
	}
	key := ui.KeyK
	hs.a.held = &key
	h.Changed()
	hs.settle()
	if len(hs.a.chord) != 1 || hs.a.held == nil {
		t.Fatal("unrelated plugin change cleared pending key input")
	}
}

func TestPresetRemovalRespectsKeyAndFocus(t *testing.T) {
	for _, tc := range []struct {
		name, binding string
		preset, undo  bool
	}{
		{"other key", `{"key":"f9","command":"-edit.undo"}`, false, true},
		{"other focus", `{"key":"cmdorctrl+z","command":"-edit.undo","when":"terminalFocus"}`, false, true},
		{"matching removal", `{"key":"cmdorctrl+z","command":"-edit.undo"}`, false, false},
		{"preset other key", `{"key":"f9","command":"-edit.undo","when":"terminalFocus"}`, true, true},
		{"preset other focus", `{"key":"cmdorctrl+z","command":"-edit.undo","when":"terminalFocus"}`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs := newHarness(t, false)
			if tc.preset {
				hs.a.settings.Keymap = "undo-keys.editor"
				manifest := `{"name":"undo-keys","contributes":{"keymaps":[{"id":"undo-keys.editor","label":"Undo Keys","bindings":[` + tc.binding + `]}]}}`
				withPlugins(hs, map[string]map[string]string{"undo-keys": {"plugin.json": manifest}})
			} else {
				withKeys(t, hs.a, "["+tc.binding+"]")
			}
			hs.a.openFile(hs.path("go.mod"), true)
			hs.settle()
			d := hs.a.activeEditor()
			before := d.buf.String()
			hs.tt.Type("x")
			hs.settle()
			if d.buf.String() == before {
				t.Fatal("test edit did not apply")
			}
			hs.tt.Key(ui.Cmd, ui.KeyZ)
			hs.settle()
			if undone := d.buf.String() == before; undone != tc.undo {
				t.Fatalf("undo = %v, want %v", undone, tc.undo)
			}
		})
	}
}

func TestPluginKeymapPassesArguments(t *testing.T) {
	hs := newHarness(t, false)
	hs.a.settings.Keymap = "args-keys.editor"
	h := withPlugins(hs, map[string]map[string]string{"args-keys": {
		"plugin.json": `{"name":"args-keys","main":"main.ts","contributes":{
			"commands":[{"id":"args-keys.capture","title":"Capture Arguments"}],
			"statusItems":[{"id":"args-keys.received"}],
			"keymaps":[{"id":"args-keys.editor","label":"Argument Keys","bindings":[
				{"key":"f8","command":"args-keys.capture","args":{"text":"hello","count":2}},
				{"key":"f9","command":"runCommands","args":{"commands":[{"command":"args-keys.capture","args":["nested",3]}]}}
			]}]}}`,
		"main.ts": `import { commands, statusItem } from "fiveleaf";
			commands.register("args-keys.capture", async (args) => {
				await statusItem("args-keys.received").set({ text: JSON.stringify(args) });
			});`,
	}})
	for _, tc := range []struct {
		key  ui.Key
		want string
	}{
		{ui.KeyF8, `{"text":"hello","count":2}`},
		{ui.KeyF9, `["nested",3]`},
	} {
		hs.tt.Key(0, tc.key)
		hs.until("plugin arguments", func() bool {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.status["args-keys.received"].Text == tc.want
		})
	}
}

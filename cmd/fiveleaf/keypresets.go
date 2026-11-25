package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/keymap"
	"fiveleaf/locales"
)

func keymapPresets() []keymap.Preset {
	presets := []keymap.Preset{{Label: "FiveLeaf"}}
	if plugins != nil && plugins.reg != nil {
		for _, p := range plugins.reg.Enabled() {
			for _, k := range p.Contributes.Keymaps {
				id, label := keymapSettingID(k.ID), k.Label
				if id == k.ID {
					label += " (" + k.ID + ")"
				}
				presets = append(presets, keymap.Preset{ID: id, Label: label, Bindings: k.Bindings})
			}
		}
	}
	return presets
}

func keymapSettingID(id string) string {
	switch id {
	case "keymap-vscode.preset":
		return "vscode"
	case "keymap-jetbrains.preset":
		return "jetbrains"
	case "keymap-sublime.preset":
		return "sublime"
	default:
		return id
	}
}

func selectedKeymap(id string) (keymap.Preset, bool) {
	id = keymapSettingID(id)
	for _, p := range keymapPresets() {
		if p.ID == id {
			return p, true
		}
	}
	return keymap.Preset{}, false
}

// Canonical names let a VS Code removal override a FiveLeaf preset binding.
func canonicalBindings(m *keymap.Map) []keymap.Binding {
	if m == nil {
		return nil
	}
	bindings := slices.Clone(m.Bindings)
	for i := range bindings {
		name := strings.TrimPrefix(bindings[i].Command, "-")
		if id, ok := vscodeCommands[name]; ok {
			if bindings[i].Removal() {
				id = "-" + id
			}
			bindings[i].Command = id
		}
	}
	return bindings
}

func personalReplacements(user *keymap.Map) []keymap.Binding {
	var removals []keymap.Binding
	for _, b := range canonicalBindings(user) {
		if !b.Removal() && b.When == nil && len(b.Args) == 0 {
			removals = append(removals, keymap.Binding{Command: "-" + b.Command})
		}
	}
	return removals
}

// Keep the editable file separate: switching or unloading a preset must
// never copy inherited bindings into the person's keybindings.json.
func (a *app) applyKeymap(user *keymap.Map) {
	oldKeys := a.keys
	a.userKeys = user
	a.keys = &keymap.Map{Bindings: canonicalBindings(user)}
	for i := range a.keys.Bindings {
		a.keys.Bindings[i].Priority = 1
	}
	a.presetKeys = &keymap.Map{}
	if p, ok := selectedKeymap(a.settings.Keymap); ok && len(p.Bindings) > 0 {
		preset := keymap.FromEntries(p.Bindings, hostOS)
		bindings := canonicalBindings(preset)
		defaults := map[string]keymap.Stroke{}
		for _, cmd := range a.commands() {
			if st, ok := parseAccel(defaultAccel(cmd)); ok {
				defaults[cmd.id] = st
			}
		}
		for _, b := range bindings {
			if st, ok := defaults[b.Command]; ok && !b.Removal() {
				a.presetKeys.Bindings = append(a.presetKeys.Bindings,
					keymap.Binding{Keys: []keymap.Stroke{st}, Command: "-" + b.Command, When: b.When})
			}
		}
		a.presetKeys.Bindings = append(a.presetKeys.Bindings, bindings...)
		combined := append(slices.Clone(a.presetKeys.Bindings), personalReplacements(user)...)
		a.keys.Bindings = append(combined, a.keys.Bindings...)
	}
	if !reflect.DeepEqual(oldKeys, a.keys) {
		a.chord, a.held = nil, nil
	}
	a.palette, a.hints = nil, nil
}

// Preset editing shortcuts have focus conditions, but still need a label.
// Personal conditional bindings stay in the additional-bindings count.
func (a *app) presetShortcut(id string) []keymap.Stroke {
	shown := &keymap.Map{Bindings: canonicalBindings(a.presetKeys)}
	for i := range shown.Bindings {
		shown.Bindings[i].When = nil
	}
	shown.Bindings = append(shown.Bindings, personalReplacements(a.userKeys)...)
	for _, b := range canonicalBindings(a.userKeys) {
		if b.Removal() {
			shown.Bindings = append(shown.Bindings, b)
		}
	}
	return shown.Last(id)
}

func (a *app) shortcutKeys(cmd command) []keymap.Stroke {
	if keys := a.keys.Last(commandNames(cmd.id)...); keys != nil {
		return keys
	}
	return a.presetShortcut(cmd.id)
}

func (a *app) presetSuppresses(cmd command) bool {
	if a.presetKeys.Removes(commandNames(cmd.id)...) {
		return true
	}
	st, ok := parseAccel(defaultAccel(cmd))
	return ok && a.presetKeys.Binds(st)
}

func (a *app) editorShortcutReplaced(id string, stroke keymap.Stroke) bool {
	ctx := a.whenContext(placeEditor)
	if a.keys != nil {
		for _, b := range a.keys.Bindings {
			if b.Command == "-"+id && b.When.Eval(ctx) &&
				(len(b.Keys) == 0 || len(b.Keys) == 1 && b.Keys[0] == stroke) {
				return true
			}
		}
	}
	if a.presetKeys != nil {
		for _, b := range a.presetKeys.Bindings {
			if b.Command == id && b.When.Eval(ctx) {
				return true
			}
		}
	}
	return false
}

func (w *prefsWindow) keymapControl(c *ui.Context, apply func(any)) {
	presets := keymapPresets()
	labels := make([]string, len(presets))
	chosen := ""
	for i, p := range presets {
		labels[i] = p.Label
		if p.ID == keymapSettingID(w.m.s.Keymap) {
			chosen = p.Label
		}
	}
	missing := chosen == ""
	if missing {
		chosen = w.m.s.Keymap
		labels = append(labels, chosen)
	}
	if w.first(ui.Select(c.Key("keys-preset"), &chosen, labels).Label(prefTitle("keymap")).MinWidth(180)).Changed() {
		if i := slices.Index(labels, chosen); i >= 0 && i < len(presets) {
			apply(presets[i].ID)
		}
	}
	if missing {
		ui.Text(c.Key("keys-preset-unavailable"), locales.KeysPresetUnavailable()).FontSize(11.5).
			TextColor(w.pal.muted).Grow(1).Shrink(1)
	}
}

func (k *keysPane) presetControl(c *ui.Context) {
	ui.Row(c.Key("keys-preset-row")).Padding(10, 24).Gap(12).AlignItems(ui.Center).Shrink(0).
		BorderWidth(0, 0, 1, 0).BorderColor(k.w.pal.border).Children(func() {
		ui.Text(c, prefTitle("keymap")).FontSize(12).TextColor(k.w.pal.text)
		k.w.keymapControl(c, func(v any) {
			raw, _ := json.Marshal(v)
			k.w.set("keymap", raw)
		})
	})
}

func (k *keysPane) inheritedRemovals(cmd command) []keymap.Entry {
	var removals []keymap.Entry
	seen := map[string]bool{}
	add := func(keys []keymap.Stroke) {
		key := keymap.KeysString(keys)
		if len(keys) > 0 && !seen[key] {
			seen[key] = true
			removals = append(removals, keymap.Entry{Key: key, Command: "-" + cmd.id})
		}
	}
	if st, ok := parseAccel(defaultAccel(cmd)); ok {
		add([]keymap.Stroke{st})
	}
	if len(k.w.m.wins) > 0 {
		if preset := k.w.m.wins[0].a.presetKeys; preset != nil {
			for _, b := range preset.Bindings {
				if b.Command == cmd.id {
					add(b.Keys)
				}
			}
		}
	}
	return removals
}

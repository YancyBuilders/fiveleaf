package main

import (
	"cmp"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type prefRoleDraft struct {
	agent, model, prompt string
	preset               *agentRolePreset
	named                *prefNamedRole
	defaultPrompt        string
	originalOverride     settings.AgentRoleOverride
	copilot              *settings.CopilotConfig
	hadOverride          bool
	models               prefRoleModels
	focus                bool
	err                  string
}

func (w *prefsWindow) agentConfigurationEditing() bool {
	return w.agentDraft != nil || w.roleDraft != nil
}

func (w *prefsWindow) editPresetRole(preset *agentRolePreset) {
	if w.agentConfigurationEditing() || w.agentIOBusy {
		return
	}
	override, exists := w.m.s.AgentRolePresets[preset.id]
	defaultPrompt := preset.prompt()
	role := override.Resolve("", defaultPrompt)
	if override.Prompt != nil {
		override.Prompt = &role.Prompt
	}
	w.roleDraft = &prefRoleDraft{preset: preset, agent: role.Agent, model: role.Model, prompt: role.Prompt,
		defaultPrompt: defaultPrompt, focus: true, originalOverride: override, hadOverride: exists}
}

func (w *prefsWindow) closeRoleDraft() {
	if draft := w.roleDraft; draft != nil {
		draft.models.clear()
		w.roleDraft = nil
	}
}

func (d *prefRoleDraft) override() settings.AgentRoleOverride {
	override := settings.AgentRoleOverride{Agent: d.agent, Model: d.model}
	if d.prompt != d.defaultPrompt {
		prompt := d.prompt
		override.Prompt = &prompt
	}
	return override
}

func (w *prefsWindow) roleDraftCurrent() bool {
	d := w.roleDraft
	if d == nil || d.preset == nil {
		return false
	}
	current, exists := w.m.s.AgentRolePresets[d.preset.id]
	if exists != d.hadOverride || !reflect.DeepEqual(current, d.originalOverride) {
		d.err = locales.PrefsRoleConflict()
		return false
	}
	return true
}

func (w *prefsWindow) saveAgentRole() bool {
	if draft := w.roleDraft; draft != nil && draft.named != nil {
		return w.saveNamedRole(draft)
	}
	if !w.roleDraftCurrent() {
		return false
	}
	d := w.roleDraft
	override := d.override()
	if err := override.Validate(); err != nil {
		d.err = err.Error()
		return false
	}
	presets := maps.Clone(w.m.s.AgentRolePresets)
	if presets == nil {
		presets = map[string]settings.AgentRoleOverride{}
	}
	if override == (settings.AgentRoleOverride{}) {
		delete(presets, d.preset.id)
	} else {
		presets[d.preset.id] = override
	}
	if !w.applyRoleSetting("agentRolePresets", presets) {
		return false
	}
	w.closeRoleDraft()
	return true
}

func (w *prefsWindow) applyRoleSetting(key string, definitions any) bool {
	raw, err := json.Marshal(definitions)
	prefs := w.m.s.Prefs
	if err == nil {
		err = prefs.Set(key, raw)
	}
	if err != nil {
		if w.roleDraft != nil {
			w.roleDraft.err = err.Error()
		} else {
			w.errors = append(w.errors, err.Error())
		}
		return false
	}
	w.m.reload(prefs)
	w.m.s.Changed()
	return true
}

func (w *prefsWindow) roleAgentChoices(selected string) []agentChoice {
	choices := []agentChoice{{}}
	for _, provider := range (&app{settings: w.m.s}).agentProviders() {
		label := provider.id
		if provider.name != "" && provider.name != provider.id {
			label = provider.name + " · " + provider.id
		}
		if provider.config.Disabled {
			label += " · " + locales.PrefsAgentDisabled()
		}
		choices = append(choices, agentChoice{id: provider.id, name: label})
	}
	// A missing plugin or removed agent remains a real choice. Opening the
	// editor must never replace its saved ID with the first available agent.
	if selected != "" && !slices.ContainsFunc(choices, func(choice agentChoice) bool { return choice.id == selected }) {
		choices = append(choices, agentChoice{id: selected, name: selected})
	}
	return choices
}

func (w *prefsWindow) resetRolePresets(preset *agentRolePreset) bool {
	remaining := maps.Clone(w.m.s.AgentRolePresets)
	removed := map[string]settings.AgentRoleOverride{}
	for id, override := range remaining {
		if preset == nil || id == preset.id {
			removed[id] = override
			delete(remaining, id)
		}
	}
	if len(removed) == 0 {
		return true
	}
	if !w.applyRoleSetting("agentRolePresets", remaining) {
		return false
	}
	key, name := "reset:agentRolePresets", prefTitle("agentRolePresets")
	if preset != nil {
		key, name = key+":"+preset.id, preset.name()
	}
	w.undoable(key, locales.PrefsWasReset(name), func() {
		current := maps.Clone(w.m.s.AgentRolePresets)
		for id := range removed {
			if _, exists := current[id]; exists {
				w.errors = append(w.errors, locales.PrefsRoleConflict())
				return
			}
		}
		if current == nil {
			current = map[string]settings.AgentRoleOverride{}
		}
		maps.Copy(current, removed)
		w.applyRoleSetting("agentRolePresets", current)
	})
	return true
}

func roleAgentLabel(providers []agentProvider, id string) (string, bool) {
	if id == "" {
		return locales.PrefsRoleCurrentAgent(), true
	}
	for _, provider := range providers {
		if provider.id == id {
			return cmp.Or(provider.name, id), !provider.config.Disabled
		}
	}
	return id, false
}

func (w *prefsWindow) agentRolesControl(c *ui.Context) {
	p := w.pal
	ui.Column(c.Key("prefs-roles")).FillWidth().Children(func() {
		if d := w.roleDraft; d != nil && d.copilot == nil {
			w.agentRoleForm(c)
			return
		}
		providers := (&app{settings: w.m.s}).agentProviders()
		disabled := w.agentConfigurationEditing() || w.agentIOBusy
		width, _ := c.Size()
		wide := width-prefsSidebarW >= 560
		row := func(name string, role settings.AgentRole, preset *agentRolePreset, divider bool) {
			key := "named:" + name
			if preset != nil {
				key = "preset:" + preset.id
			}
			line := ui.Row(c.Key(key)).MinHeight(40).Padding(5, 2).Gap(6).AlignItems(ui.Center)
			w.rowHover(line, divider)
			line.Children(func() {
				label, available := roleAgentLabel(providers, role.Agent)
				model := cmp.Or(role.Model, locales.AgentDefaultModel())
				ui.Column(c).Grow(2.3).Basis(0).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						meetingDot(c, p, meetingSpeaker{Participant: roundtable.Participant{ID: key, Name: name}})
						ui.Text(c, name).FontSize(13).FontWeight(500).TextColor(p.text).SingleLine().MinWidth(0)
					})
					lead, _, _ := strings.Cut(strings.TrimSpace(role.Prompt), "\n")
					ui.Text(c, cmp.Or(lead, locales.PrefsRoleEmptyPrompt())).FontSize(11.5).TextColor(p.muted).SingleLine().Tooltip(role.Prompt)
					if !wide {
						ui.Text(c, label+" · "+model).FontSize(11).TextColor(p.muted).SingleLine()
					}
					if !available {
						ui.Text(c, locales.AgentRoleUnavailable()).FontSize(11).TextColor(p.modifiedFg)
					}
				})
				if wide {
					ui.Text(c, label).Grow(1).Basis(0).MinWidth(0).FontSize(11.5).TextColor(p.muted).SingleLine().Tooltip(label)
					ui.Text(c, model).Grow(1).Basis(0).MinWidth(0).FontSize(11.5).TextColor(p.muted).SingleLine().Tooltip(model)
				}
				ui.Box(c.Key("role-action")).Size(26, 26).Shrink(0).Children(func() {
					if preset == nil {
						if w.iconButton(c, "remove", iconTrash, locales.PrefsRoleRemove()+": "+name, p.dangerFg, disabled) {
							w.removeNamedRole(name)
						}
						return
					}
					if _, changed := w.m.s.AgentRolePresets[preset.id]; changed && w.iconButton(c, "restore", iconUndo, locales.PrefsRoleRestore()+": "+name, p.accent, disabled) {
						w.resetRolePresets(preset)
					}
				})
				if w.iconButton(c, "copy", iconCopy, locales.PrefsRoleCopy()+": "+name, p.accent, disabled) {
					w.copyRole(name, role)
				}
				if w.iconButton(c, "edit", iconPencil, locales.PrefsRoleEdit()+": "+name, p.accent, disabled) {
					if preset != nil {
						w.editPresetRole(preset)
					} else {
						w.openNamedRole(name, role, true)
					}
				}
			})
		}
		for index := range agentRolePresets {
			preset := &agentRolePresets[index]
			row(preset.name(), w.m.s.AgentRolePresets[preset.id].Resolve("", preset.prompt()), preset, index > 0)
		}
		for _, name := range slices.Sorted(maps.Keys(w.m.s.AgentRoles)) {
			row(name, w.m.s.AgentRoles[name], nil, true)
		}
		compactActionRow(c, ui.Start).Padding(8, 0, 0, 0).Children(func() {
			if actionButton(c, locales.PrefsRoleImport(), iconDownload).Disabled(disabled).Clicked() {
				w.importAgentConfiguration(roleFile)
			}
			if actionButton(c, locales.PrefsRoleExport(), iconPush).Disabled(disabled).Clicked() {
				w.exportAgentConfiguration(roleFile)
			}
		})
	})
}

func (w *prefsWindow) agentRoleForm(c *ui.Context) {
	d, p := w.roleDraft, w.pal
	ui.Column(c.Key("prefs-role-form")).Gap(10).Padding(8, 0).Children(func() {
		if named := d.named; named != nil {
			ui.Column(c.Key("name")).Gap(4).Children(func() {
				ui.Text(c, locales.PrefsRoleName()).FontSize(11.5).TextColor(p.muted)
				input := ui.TextInput(c.Key("input"), &named.name).FillWidth().FontSize(12).Label(locales.PrefsRoleName())
				if d.focus {
					input.Focus().ScrollIntoView()
					d.focus = false
				}
			})
		} else {
			ui.Text(c, d.preset.name()).FontSize(13).FontWeight(600).TextColor(p.text)
		}
		w.agentModelFields(c, d, locales.PrefsRoleCurrentAgent())
		ui.Column(c.Key("prompt")).Gap(4).Children(func() {
			ui.Text(c, locales.PrefsRolePrompt()).FontSize(11.5).TextColor(p.muted)
			ui.TextArea(c.Key("input"), &d.prompt).FillWidth().FontSize(12).
				Height(max(128, min(float32(36+strings.Count(d.prompt, "\n")*16), 256))).Label(locales.PrefsRolePrompt())
		})
		if d.err != "" {
			ui.Text(c, d.err).FontSize(12).TextColor(p.dangerFg).Selectable()
		}
		compactActionRow(c, ui.End).Children(func() {
			if d.preset != nil {
				atDefault := !d.hadOverride && d.agent == "" && d.model == "" && d.prompt == d.defaultPrompt
				restore := actionButton(c.Key("defaults"), locales.PrefsRoleRestore(), iconUndo).Disabled(atDefault)
				if restore.Clicked() && w.roleDraftCurrent() && w.resetRolePresets(d.preset) {
					w.closeRoleDraft()
				}
			}
			if actionButton(c.Key("cancel"), locales.ButtonCancel(), nil).Clicked() {
				w.closeRoleDraft()
			}
			if w.primaryButton(c, "save", locales.ButtonSave(), false).Clicked() {
				w.saveAgentRole()
			}
		})
	})
}

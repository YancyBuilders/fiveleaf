package main

import (
	"cmp"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (w *prefsWindow) copilotControl(c *ui.Context) {
	current := w.m.s.Copilot
	if draft := w.roleDraft; draft != nil && draft.copilot != nil {
		ui.Column(c.Key("copilot-form")).FillWidth().Gap(8).Children(func() {
			w.agentModelFields(c, draft, locales.PluginsOff())
			if draft.err != "" {
				ui.Text(c, draft.err).FontSize(12).TextColor(w.pal.dangerFg)
			}
			compactActionRow(c, ui.End).Children(func() {
				if actionButton(c, locales.ButtonCancel(), nil).Clicked() {
					w.closeRoleDraft()
				}
				if w.primaryButton(c, "save", locales.ButtonSave(), false).Clicked() {
					selection := settings.CopilotConfig{Agent: draft.agent, Model: draft.model}
					provider := findChatProvider(selection.Agent, (&app{settings: w.m.s}).agentProviders())
					switch {
					case current != *draft.copilot:
						draft.err = locales.PrefsAgentConflict()
					case !provider.presetModelCompatible(selection.Model):
						draft.err = locales.CopilotCLIModel()
					case w.applyRoleSetting("copilot", selection):
						w.closeRoleDraft()
					}
				}
			})
		})
		return
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		label := locales.PluginsOff()
		if current.Agent != "" {
			name, available := roleAgentLabel((&app{settings: w.m.s}).agentProviders(), current.Agent)
			label = name + " · " + cmp.Or(current.Model, locales.AgentDefaultModel())
			if !available {
				label = locales.CopilotUnavailable(name) + " · " + cmp.Or(current.Model, locales.AgentDefaultModel())
			}
		}
		ui.Text(c, label).FontSize(12).SingleLine().Grow(1).MinWidth(0).TextColor(w.pal.text).Tooltip(label)
		if actionButton(c, locales.CopilotConfigure(), iconSettings).Disabled(w.agentConfigurationEditing() || w.agentIOBusy).Clicked() {
			w.roleDraft = &prefRoleDraft{agent: current.Agent, model: current.Model, copilot: &current, focus: true}
		}
	})
}

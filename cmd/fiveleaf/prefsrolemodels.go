package main

import (
	"context"
	"reflect"
	"slices"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/remote"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type prefRoleModels struct {
	provider *agentProvider
	choices  []acp.ModelChoice
	cancel   context.CancelFunc
	result   <-chan roleModelResult
	err      string
	manual   bool
}

type roleModelResult struct {
	choices []acp.ModelChoice
	err     error
}

func (models *prefRoleModels) clear() {
	if models.cancel != nil {
		models.cancel()
	}
	*models = prefRoleModels{}
}

func (w *prefsWindow) liveRoleModels(provider agentProvider) []acp.ModelChoice {
	for _, window := range w.m.wins {
		for _, chat := range window.a.allAgentChats() {
			if chat != nil && chat.connected != nil && chat.connected.id == provider.id && len(chat.models) != 0 && reflect.DeepEqual(chat.connected.config, provider.config) {
				return chat.models
			}
		}
	}
	return nil
}

func (w *prefsWindow) loadRoleModels(sv ui.Services, d *prefRoleDraft, provider agentProvider) {
	d.models.clear()
	provider.config = cloneAgentServer(provider.config)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	result := make(chan roleModelResult, 1)
	d.models.provider, d.models.cancel, d.models.result = &provider, cancel, result
	root := ""
	if len(w.m.wins) > 0 && !remote.Is(w.m.wins[0].a.root) {
		root = w.m.wins[0].a.root
	}
	go func() {
		defer cancel()
		choices, err := acp.DiscoverModels(ctx, agentACPConfig(provider), root)
		result <- roleModelResult{choices, err}
		sv.Invalidate()
	}()
}

func (w *prefsWindow) roleModelControl(c *ui.Context, d *prefRoleDraft) {
	provider := findChatProvider(d.agent, (&app{settings: w.m.s}).agentProviders())
	models := &d.models
	if models.provider != nil && !reflect.DeepEqual(*models.provider, provider) {
		models.clear()
	}
	select {
	case result := <-models.result:
		models.choices, models.cancel, models.result = result.choices, nil, nil
		if result.err != nil {
			models.err = result.err.Error()
		} else if len(result.choices) == 0 {
			models.err = locales.PrefsRoleNoModels()
		}
	default:
	}
	choices := models.choices
	canLoad := provider.config.Command != "" && !provider.config.Disabled && provider.config.EffectiveProtocol() == "acp" && provider.config.Validate() == nil
	if canLoad && len(choices) == 0 {
		choices = w.liveRoleModels(provider)
	}
	ui.Column(c.Key("model:" + d.agent)).Gap(4).FillWidth().Children(func() {
		ui.Text(c, locales.AgentModel()).FontSize(11.5).TextColor(w.pal.muted)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if len(choices) == 0 || models.manual {
				ui.TextInput(c.Key("id"), &d.model).Label(locales.PrefsRoleModel()).Placeholder(locales.AgentDefaultModel()).Grow(1).MinWidth(0).FontSize(12).Disabled(d.agent == "")
				if len(choices) > 0 && w.iconButton(c, "choices", iconList, locales.AgentModel(), w.pal.accent, false) {
					models.manual = false
				}
			} else {
				labels := []string{locales.AgentDefaultModel()}
				chosen := labels[0]
				if d.model != "" {
					chosen = d.model
				}
				for _, choice := range choices {
					label := choice.ID
					if choice.Name != "" && choice.Name != choice.ID {
						label = choice.Name + " · " + choice.ID
					}
					labels = append(labels, label)
					if choice.ID == d.model {
						chosen = label
					}
				}
				labels = append(labels, locales.PrefsRoleManualModel())
				if ui.Combobox(c.Key("choices"), &chosen, labels).Label(locales.PrefsRoleModel()).Grow(1).MinWidth(0).FontSize(12).Changed() {
					switch index := slices.Index(labels, chosen); {
					case index == 0:
						d.model = ""
					case index == len(labels)-1:
						models.manual = true
					case index > 0:
						d.model = choices[index-1].ID
					}
				}
			}
			if canLoad && w.iconButton(c, "load", iconRefresh, locales.PrefsRoleLoadModels(), w.pal.accent, models.cancel != nil) {
				w.loadRoleModels(c.Services(), d, provider)
			}
		})
		if models.cancel != nil {
			ui.Text(c, locales.Loading()).FontSize(11).TextColor(w.pal.muted)
		} else if models.err != "" {
			ui.Text(c, models.err).FontSize(11).TextColor(w.pal.muted).Selectable()
		}
	})
}

func (w *prefsWindow) agentModelFields(c *ui.Context, d *prefRoleDraft, empty string) {
	ui.Column(c.Key("agent")).Gap(4).Children(func() {
		ui.Text(c, locales.PrefsRoleAgent()).FontSize(11.5).TextColor(w.pal.muted)
		choices := w.roleAgentChoices(d.agent)
		choices[0].name = empty
		labels, chosen := make([]string, len(choices)), empty
		for index, choice := range choices {
			labels[index] = choice.name
			if choice.id == d.agent {
				chosen = choice.name
			}
		}
		selectAgent := ui.Select(c.Key("input"), &chosen, labels).FillWidth().FontSize(12).Label(locales.PrefsRoleAgent())
		if d.focus {
			selectAgent.Focus().ScrollIntoView()
			d.focus = false
		}
		if selectAgent.Changed() {
			if index := slices.Index(labels, chosen); index >= 0 && choices[index].id != d.agent {
				d.models.clear()
				d.agent, d.model, d.err = choices[index].id, "", ""
			}
		}
		if _, available := roleAgentLabel((&app{settings: w.m.s}).agentProviders(), d.agent); d.agent != "" && !available {
			ui.Text(c, locales.AgentRoleUnavailable()).FontSize(11).TextColor(w.pal.muted)
		}
	})
	w.roleModelControl(c, d)
}

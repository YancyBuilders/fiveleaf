package main

import (
	"slices"
	"strings"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

const agentModelRecentGroup = "\x00recent"

type agentModelPickerState struct {
	open, focus, moved, keyed         bool
	root, session                     string
	target                            agentTarget
	query, group, selected, lastQuery string
	generation                        uint64
	client                            *acp.Client
	thinking                          agentThinkingState
}

// The closed control needs only the model's display name. Strip a path
// component only when it is an advertised provider, not an arbitrary slash
// in a model's own name. The popover keeps the full, disambiguated labels.
func agentModelPillLabel(choice acp.ModelChoice, provider string) string {
	name := choice.Name
	if name == "" {
		name = choice.ID
	}
	idProvider, _, hasProvider := strings.Cut(choice.ID, "/")
	for _, prefix := range []string{choice.Group, provider, idProvider} {
		if prefix == "" || prefix == idProvider && !hasProvider {
			continue
		}
		if strings.HasPrefix(name, prefix+"/") {
			return strings.TrimPrefix(name, prefix+"/")
		}
	}
	return name
}

func agentModelGroup(choice acp.ModelChoice, fallback string) string {
	if choice.Group != "" {
		return choice.Group
	}
	if provider, _, ok := strings.Cut(choice.ID, "/"); ok && provider != "" {
		return provider
	}
	return fallback
}

func agentModelGroups(choices []acp.ModelChoice, fallback string) []string {
	var groups []string
	for _, choice := range choices {
		group := agentModelGroup(choice, fallback)
		if !slices.Contains(groups, group) {
			groups = append(groups, group)
		}
	}
	return groups
}

func agentModelMatches(choices []acp.ModelChoice, fallback, group, query string, recent []string) []acp.ModelChoice {
	terms := strings.Fields(strings.ToLower(query))
	var matches []acp.ModelChoice
	for _, choice := range choices {
		if group == agentModelRecentGroup {
			if !slices.Contains(recent, choice.ID) {
				continue
			}
		} else if group != "" && agentModelGroup(choice, fallback) != group {
			continue
		}
		text := strings.ToLower(choice.Name + " " + choice.ID + " " + choice.Description + " " + agentModelGroup(choice, fallback))
		if slices.ContainsFunc(terms, func(term string) bool { return !strings.Contains(text, term) }) {
			continue
		}
		matches = append(matches, choice)
	}
	if group == agentModelRecentGroup {
		slices.SortStableFunc(matches, func(a, b acp.ModelChoice) int { return slices.Index(recent, a.ID) - slices.Index(recent, b.ID) })
		return matches
	}
	groups := agentModelGroups(choices, fallback)
	slices.SortStableFunc(matches, func(a, b acp.ModelChoice) int {
		return slices.Index(groups, agentModelGroup(a, fallback)) - slices.Index(groups, agentModelGroup(b, fallback))
	})
	return matches
}

func (k *agentModelPickerState) valid(a *app, s *agentChat) bool {
	return a.chatShown(s) && k.root == a.root && k.generation == s.generation && k.client == s.client &&
		k.session == s.sessionID && k.target == s.agentTarget
}

func (a *app) chooseAgentModel(s *agentChat, id string) {
	k := s.modelPicker
	if !k.open || !k.valid(a, s) || s.requestInFlight() {
		return
	}
	a.post(func() {
		if k.valid(a, s) && s.modelPicker.open {
			s.modelPicker.focus = true
			a.setChatModel(s, id)
		}
	})
}

func (a *app) agentModelButton(c *ui.Context, s *agentChat) ui.Element {
	p, m, k := a.pal, a.pal.metricsOr(), &s.modelPicker
	model := s.model()
	thinking := agentThinkingModes(s)
	name, tooltip := model, model
	if name == "" {
		name = locales.AgentModel()
		if len(s.models) == 0 && len(thinking) > 0 {
			tooltip = locales.AgentThinking()
		}
	}
	for _, choice := range s.models {
		if choice.ID == model {
			name = agentModelPillLabel(choice, s.provider)
			if choice.Description != "" {
				tooltip += "\n" + choice.Description
			}
			break
		}
	}
	available := s.client != nil && s.sessionID != "" && (len(s.models) > 0 || len(thinking) > 0)
	disabled := !available || s.busy || s.connecting
	if !available {
		tooltip = locales.AgentModelUnavailable()
	}
	if k.open && (!available || s.busy || !k.valid(a, s)) {
		*k = agentModelPickerState{}
	}
	pick := ui.ButtonBase(c.Key("agent-model")).Label(locales.AgentModel()).Tooltip(tooltip).
		Height(28).MaxWidth(196).Padding(0, 9, 0, 10).Gap(6).Radius(14).
		Border(1, p.border).AlignItems(ui.Center).Shrink(1).MinWidth(0).Disabled(disabled).Expanded(k.open)
	if pick.Hovered() || k.open {
		pick.Background(p.hover)
	}
	color := p.addedFg
	if !available {
		color = p.faint
	} else if s.connecting {
		color = p.modifiedFg
	}
	pick.Children(func() {
		ui.Box(c).Size(5, 5).Radius(3).Background(color).Shrink(0)
		ui.Text(c, name).FontSize(m.textSmall).TextColor(p.text).SingleLine().Shrink(1).MinWidth(0)
		ui.Icon(c, iconChevronDown).FontSize(10).TextColor(p.muted).Shrink(0)
	})
	keyed := pick.Shortcut(0, ui.KeyDown) || pick.Shortcut(0, ui.KeyUp)
	if !disabled && (pick.Clicked() || keyed) {
		if k.open {
			*k = agentModelPickerState{}
		} else {
			*k = agentModelPickerState{open: true, focus: true, moved: true, keyed: keyed,
				root: a.root, generation: s.generation, client: s.client, session: s.sessionID,
				target: s.agentTarget, selected: model}
		}
	}
	return pick
}

func (a *app) agentModelPopover(c *ui.Context, s *agentChat, toggle, usage ui.Element) {
	p, m, k := a.pal, a.pal.metricsOr(), &s.modelPicker
	if usage.Pressed() {
		k.open = false
		return
	}
	width, height := c.Size()
	bounds := usage.Bounds()
	room := max(bounds.Y-12, height-bounds.Y-bounds.H-12)
	height = min(380, max(0, room-8))
	thinking := agentThinkingModes(s)
	thinkingOnly := len(s.models) == 0 && len(thinking) > 0
	footerHeight := float32(0)
	if len(thinking) > 0 {
		footerHeight = agentThinkingFooterHeight
	}
	if thinkingOnly {
		height, width = min(height, agentThinkingFooterHeight), min(width, 384)
	}
	groups := agentModelGroups(s.models, s.provider)
	if k.group != "" && k.group != agentModelRecentGroup && !slices.Contains(groups, k.group) {
		k.group = ""
	}
	ui.PopoverBase(c.Key("agent-model-popover"), toggle, &k.open, func(panel ui.Element) {
		a.floating(c, panel.Width(min(500, max(0, width-24))).Height(height).Radius(m.radiusCard), false).
			AttachTo(usage, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 8, 0).Clip().
			Role(ui.RoleDialog).Label(locales.AgentModel())
		if !k.keyed {
			fade := panel.Keyframes("show", []ui.Keyframe{{Value: 0}, {At: motionPopover, Value: 1, Ease: ui.EaseOut}})
			panel.Opacity(fade)
		}
		panel.Children(func() {
			if thinkingOnly {
				a.agentThinkingPickerRow(c, s, thinking)
				return
			}
			ui.Row(c).Height(44).Shrink(0).AlignItems(ui.Center).Padding(4, 6).Children(func() {
				in := a.textKeys(searchInput(c.Key("agent-model-search"), &k.query, locales.AgentModelSearch(), iconSearch)).
					Label(locales.AgentModelSearch()).Height(34).Background(ui.Color{}).Border(0, ui.Color{})
				if k.focus {
					in.Focus()
					k.focus = false
				}
				if k.query != k.lastQuery {
					k.lastQuery, k.selected, k.moved = k.query, "", true
				}
				results := agentModelMatches(s.models, s.provider, k.group, k.query, s.recentModels)
				index := slices.IndexFunc(results, func(choice acp.ModelChoice) bool { return choice.ID == k.selected })
				if index < 0 && len(results) > 0 {
					index, k.selected = 0, results[0].ID
				}
				switch {
				case in.Shortcut(0, ui.KeyDown):
					index, k.moved = min(index+1, len(results)-1), true
				case in.Shortcut(0, ui.KeyUp):
					index, k.moved = max(index-1, 0), true
				case in.Submitted():
					if index >= 0 && index < len(results) {
						a.chooseAgentModel(s, results[index].ID)
					}
				}
				if index >= 0 && index < len(results) {
					k.selected = results[index].ID
				}
			})
			ui.Box(c).Height(1).Shrink(0).Background(p.border)
			ui.Row(c).Height(max(0, height-45-footerHeight)).MinHeight(0).Children(func() {
				ui.Scroll(c.Key("agent-model-groups")).Width(44).FillHeight().Shrink(0).Padding(5).Gap(4).Children(func() {
					a.agentModelGroupButton(c, k, "", locales.AgentModelAllProviders(), iconBox, "")
					if len(s.recentModels) > 0 {
						a.agentModelGroupButton(c, k, agentModelRecentGroup, locales.AgentModelRecent(), iconClock, "")
					}
					for _, group := range groups {
						initials := []rune(strings.ToUpper(group))
						a.agentModelGroupButton(c, k, group, group, nil, string(initials[:min(len(initials), 2)]))
					}
				})
				ui.Box(c).Width(1).Shrink(0).Background(p.border)
				ui.Scroll(c.Key("agent-model-results")).Grow(1).FillHeight().MinWidth(0).Padding(5).Children(func() {
					results := agentModelMatches(s.models, s.provider, k.group, k.query, s.recentModels)
					if len(results) == 0 {
						ui.Text(c, locales.AgentModelNoMatches()).FontSize(m.textBody).TextColor(p.muted).Padding(12, 8)
					}
					previousGroup := ""
					for i, choice := range results {
						group := agentModelGroup(choice, s.provider)
						if k.group != agentModelRecentGroup && (i == 0 || group != previousGroup) {
							ui.Text(c, group).FontSize(m.textSmall).TextColor(p.muted).SingleLine().Padding(6, 9)
						}
						previousGroup = group
						a.agentModelRow(c, s, choice)
					}
				})
			})
			if len(thinking) > 0 {
				a.agentThinkingPickerRow(c, s, thinking)
			}
		})
		if panel.OverlayShortcut(0, ui.KeyEscape) {
			k.open = false
			toggle.Focus()
		}
		k.moved = false
	})
}

func (a *app) agentModelGroupButton(c *ui.Context, k *agentModelPickerState, group, label string, icon *ui.SVG, initials string) {
	p, m := a.pal, a.pal.metricsOr()
	b := ui.ButtonBase(c.Key("model-group:"+group)).Size(32, 32).Shrink(0).Radius(m.radius).
		Label(label).Tooltip(label).Selected(k.group == group).Center().KeepFocus()
	if k.group == group {
		b.Background(p.active)
	} else if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(16).TextColor(p.muted)
		} else {
			ui.Text(c, initials).FontSize(m.textSmall).TextColor(p.muted).SingleLine()
		}
	})
	if b.Clicked() {
		k.group, k.selected, k.moved, k.focus = group, "", true, true
	}
}

func (a *app) agentModelRow(c *ui.Context, s *agentChat, choice acp.ModelChoice) {
	p, m, k := a.pal, a.pal.metricsOr(), &s.modelPicker
	selected := k.selected == choice.ID
	label := agentModelLabel(s.models, choice)
	row := ui.ButtonBase(c.Key("model-choice:"+choice.ID)).Padding(7, 9).Gap(8).Radius(m.radius).
		Border(1, ui.Color{}).AlignItems(ui.Center).Justify(ui.Start).Label(label).Selected(selected).KeepFocus().
		Disabled(s.busy || s.connecting).Tooltip(choice.ID)
	if selected {
		row.Background(p.active).BorderColor(p.accent.Alpha(0.35))
	} else if row.Hovered() {
		row.Background(p.hover)
	}
	if selected && k.moved {
		row.ScrollIntoView()
	}
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(m.textBody).TextColor(p.text).SingleLine()
			detail := choice.Description
			if detail == "" {
				detail = choice.ID
			}
			ui.Text(c, detail).FontSize(m.textSmall).TextColor(p.muted).SingleLine()
		})
		ui.Box(c).Size(16, 16).Shrink(0).Center().Children(func() {
			if choice.ID == s.model() {
				ui.Icon(c, iconCheck).FontSize(14).TextColor(p.addedFg)
			}
		})
	})
	if row.Clicked() {
		a.chooseAgentModel(s, choice.ID)
	}
}

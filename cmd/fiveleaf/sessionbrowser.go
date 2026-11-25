package main

import (
	"slices"
	"strconv"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

type sessionAgentCount struct {
	harness agents.Harness
	count   int
}

func (a *app) sessionsVisible() bool {
	return !a.window.Detached && !a.window.SidebarClosed && (a.window.SidebarView == "sessions" || a.window.SidebarView == "agent")
}

func (a *app) sessionsActions(c *ui.Context) {
	pal, metrics := a.pal, a.pal.metricsOr()
	const iconSize = 14
	labelWidth := func(text string) float32 {
		width, _ := c.MeasureText(0, ui.Span{Text: text, Size: metrics.textSmall, Weight: metrics.weightMedium})
		return width + iconSize + metrics.gap + 2*metrics.pad
	}
	post := func(action func()) {
		folder := a.folderKey()
		a.post(func() {
			if !a.loadsClosed && a.folderKey() == folder && a.sessionsVisible() {
				action()
			}
		})
	}
	button := func(key, label string, surface ui.Color, enabled bool) ui.Element {
		control := ui.ButtonBase(c.Key(key)).Height(metrics.control).Center().Gap(metrics.gap).Padding(0, metrics.pad).
			Label(label).Tooltip(label).Disabled(!enabled).Radius(metrics.radiusField).Background(surface).Transition(hoverFade)
		if enabled {
			control.Cursor(ui.CursorPointer)
			if control.Hovered() || control.FocusVisible() {
				control.Background(pal.hover.Over(surface))
			}
		}
		return control
	}
	labelButton := func(control ui.Element, icon *ui.SVG, text string, width float32, ink ui.Color) {
		control.Children(func() {
			if room := control.LayoutBounds().W; room == 0 || room+1 >= width {
				ui.Icon(c, icon).FontSize(iconSize).Shrink(0).TextColor(ink)
			}
			ui.Text(c, text).FontSize(metrics.textSmall).FontWeight(metrics.weightMedium).TextColor(ink).SingleLine().Shrink(1).MinWidth(0)
		})
	}
	ui.Row(c.Key("session-actions")).FillWidth().Gap(metrics.gap).Wrap().Children(func() {
		tint := pal.accent.Alpha(0.14)
		newLabel, meetingLabel := locales.AgentNewSession(), locales.SessionsRoundtable()
		newWidth, meetingWidth := labelWidth(newLabel), labelWidth(meetingLabel)
		ui.Row(c.Key("start-chat")).Basis(newWidth + metrics.control + 1).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			start := button("new", locales.SessionsStartChat(), tint, true).Grow(1).MinWidth(0).Radius(metrics.radiusField, 0, 0, metrics.radiusField)
			labelButton(start, iconPlus, newLabel, newWidth, pal.text)
			if start.Clicked() {
				post(a.newChatPage)
			}
			more := button("agent-menu", locales.AgentRoster(), tint, true).Width(metrics.control).Shrink(0).Padding(0).Radius(0, metrics.radiusField, metrics.radiusField, 0)
			more.Children(func() { ui.Icon(c, iconChevronDown).FontSize(12).TextColor(pal.muted) })
			a.agentMenuButton(c, more, a.agent, true)
		})
		enabled := a.root != "" && !remote.Is(a.root)
		meeting := button("roundtable", locales.RoundtableOpen(), pal.text.Alpha(0.06), enabled).Basis(meetingWidth).Grow(1).Shrink(1).MinWidth(0)
		ink := pal.text
		if !enabled {
			ink = pal.muted
			meeting.Description(locales.SessionsRoundtableNeedsFolder()).Tooltip(locales.SessionsRoundtableNeedsFolder())
		}
		labelButton(meeting, uiicons.UsersRound(), meetingLabel, meetingWidth, ink)
		if enabled && meeting.Clicked() {
			post(a.openRoundtable)
		}
	})
}

func (a *app) openChatsMenu(menu *ui.Menu) {
	var tabs []*colTab
	counts := map[string]int{}
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool != nil && tab.tool.chat != nil {
				tabs = append(tabs, tab)
				counts[tab.name()]++
			}
		}
	}
	if len(tabs) == 0 {
		menu.Item(locales.SessionsNoOpenChats()).Disabled(true)
	}
	for _, tab := range tabs {
		label := tab.name()
		if counts[label] > 1 {
			label += " · #" + strconv.Itoa(tab.id)
		}
		if len(tab.tool.chat.permissions) > 0 {
			label += " · " + locales.AgentPermission()
		}
		if menu.Item(label).Checked(tab == a.activeTab()).Chosen() {
			a.post(func() {
				if col, index := a.locate(tab); col != nil {
					a.activate(col, index, true)
				}
			})
		}
	}
}

func (a *app) sessionAgentCounts() []sessionAgentCount {
	state := &a.sessions
	key := state.rowsKey
	key.harness = ""
	if state.filterCounts != nil && state.filtersKey == key {
		return state.filterCounts
	}
	counts := map[agents.Harness]int{}
	for index, session := range state.sessions {
		if session.Harness == meetingHistorySource || !key.all && !session.In(a.root) {
			continue
		}
		if len(state.words) > 0 && !agents.HasWords(state.texts[index], state.words) {
			continue
		}
		counts[session.Harness]++
	}
	result := []sessionAgentCount{}
	for harness, count := range counts {
		result = append(result, sessionAgentCount{harness, count})
	}
	slices.SortFunc(result, func(left, right sessionAgentCount) int {
		if left.harness < right.harness {
			return -1
		}
		if left.harness > right.harness {
			return 1
		}
		return 0
	})
	state.filterCounts, state.filtersKey = result, key
	return result
}

func (a *app) sessionsScope(c *ui.Context) {
	palette := a.pal
	metrics := palette.metricsOr()
	inset := metrics.gap / 2
	labels := [...]string{locales.SessionsThisProject(), locales.SessionsAllProjects()}
	parts := ui.SegmentedBase(c, &a.sessions.all, len(labels))
	parts.Track.FillWidth().MinWidth(0).Height(metrics.control + inset).Padding(inset).Gap(inset).
		AlignItems(ui.Stretch).Radius(metrics.radiusField).Background(palette.text.Alpha(0.05)).Label(locales.ProjectPickerTitle()).Children(func() {
		for index, label := range labels {
			button := parts.Segment(index).Basis(0).Grow(1).MinWidth(0).Radius(metrics.radiusField-inset).
				Border(1, ui.Color{}).Label(label).Tooltip(label).Cursor(ui.CursorPointer).Transition(hoverFade)
			ink := palette.muted
			switch {
			case index == a.sessions.all:
				button.Background(palette.active).BorderColor(palette.accent.Alpha(0.35))
				ink = palette.text
			case button.Hovered():
				button.Background(palette.hover)
				ink = palette.text
			}
			button.Children(func() {
				ui.Text(c, label).SingleLine().Shrink(1).MinWidth(0).TextAlign(ui.Center).
					FontSize(metrics.textSmall).FontWeight(metrics.weightMedium).TextColor(ink)
			})
		}
	})
}

func (a *app) sessionsFilters(c *ui.Context) {
	state, palette := &a.sessions, a.pal
	name := locales.SessionsAllAgents()
	icon := iconAgent
	if state.harness == meetingHistorySource {
		name, icon = locales.SessionsMeetingHistory(), uiicons.UsersRound()
	} else if state.harness != "" {
		name = state.harness.Name()
	}
	ui.Row(c.Key("sessions-filters")).Gap(6).AlignItems(ui.Center).Children(func() {
		pick := ui.ButtonBase(c.Key("sessions-agent-filter")).Label(locales.SessionsFilterAgent()).Tooltip(name).
			Height(26).Padding(0, 6).Radius(5).Gap(5).AlignItems(ui.Center).Shrink(1).MinWidth(0)
		if pick.Hovered() {
			pick.Background(palette.hover)
		}
		pick.Children(func() {
			ui.Icon(c, icon).FontSize(12).TextColor(palette.muted).Shrink(0)
			ui.Text(c, name).FontSize(11).TextColor(palette.text).SingleLine().Shrink(1).MinWidth(0)
			ui.Icon(c, iconChevronDown).FontSize(10).TextColor(palette.faint).Shrink(0)
		})
		pick.Menu(func(menu *ui.Menu) {
			if menu.Item(locales.SessionsAllAgents()).Checked(state.harness == "").Chosen() {
				state.harness = ""
			}
			if menu.Item(locales.SessionsMeetingHistory()).Checked(state.harness == meetingHistorySource).Chosen() {
				state.harness = meetingHistorySource
			}
			menu.Separator()
			for _, choice := range a.sessionAgentCounts() {
				if menu.Item(choice.harness.Name() + " · " + strconv.Itoa(choice.count)).Checked(state.harness == choice.harness).Chosen() {
					state.harness = choice.harness
				}
			}
		})
		ui.Spacer(c)
		if state.loaded {
			count := locales.SessionsCount(len(state.rows))
			if state.harness == meetingHistorySource {
				count = locales.SessionsMeetingCount(len(state.rows))
			}
			ui.Text(c, count).FontSize(10.5).FontFeatures("tnum").TextColor(palette.faint).SingleLine().Shrink(0)
		}
		if a.iconButton(c, iconRefresh, locales.Refresh(), false).Size(24, 26).Shrink(0).Clicked() {
			a.post(a.readSessionsNow)
		}
	})
}

func (a *app) sessionContinueMenu(menu *ui.Menu, source agents.Session, action *func()) {
	providers := a.agentProviders()
	for _, provider := range providers {
		unavailable := provider.config.Disabled || provider.config.Validate() != nil || !isFile(source.Transcript)
		if menu.Item(provider.name).Disabled(unavailable).Chosen() {
			id := provider.id
			*action = func() { a.continueSessionWith(source, id) }
		}
	}
	if len(providers) > 0 {
		menu.Separator()
	}
	if menu.Item(locales.AgentConfigure()).Chosen() {
		*action = func() { a.host.showSettings(settings.CatAgent, "agentServers") }
	}
}

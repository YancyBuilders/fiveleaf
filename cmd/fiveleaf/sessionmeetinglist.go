package main

import (
	"strings"
	"time"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/bg"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

type meetingListScope struct {
	project, data string
	archive       roundtable.Library
}

type meetingListRow struct {
	id, topic, roster, search string
	updated                   time.Time
}

type meetingSidebarState struct {
	scope          meetingListScope
	read           bg.Poller[meetingListScope]
	library        roundtable.Library
	rows, filtered []meetingListRow
	query, filter  string
	loaded, again  bool
	generation     uint64
	revision       uint64
	err            error
	list           ui.ListState
	row            int
}

func (a *app) meetingListWindow() *roundtableWindow {
	if window := a.activeRoundtableTab(); window != nil && !window.sessionsSidebar {
		return window
	}
	return nil
}

func (a *app) showMeetingSidebar() {
	if window := a.meetingWindow; window != nil {
		window.sessionsSidebar = false
	}
	if a.window.SidebarClosed || a.window.SidebarView != "sessions" {
		a.window.SidebarClosed, a.window.SidebarView = false, "sessions"
		a.settings.Changed()
	}
	a.sessions.input.CancelFocus()
	a.agent.focusNext, a.focusFiles = false, false
	a.invalidate()
}

func (state *meetingSidebarState) expire() {
	if state.read.Running() {
		state.again = true
	} else {
		state.read.Expire()
	}
}

func (a *app) meetingListScope(window *roundtableWindow) meetingListScope {
	if !window.creating && window.meeting != nil && window.meeting.readOnly {
		return meetingListScope{project: window.meeting.library.Project, archive: window.meeting.library}
	}
	return meetingListScope{project: a.root, data: a.roundtableDataDir()}
}

func readMeetingList(scope meetingListScope) (roundtable.Library, []meetingListRow, error) {
	library := scope.archive
	if library == (roundtable.Library{}) {
		var err error
		library, err = roundtable.ProjectLibrary(scope.data, scope.project)
		if err != nil {
			return library, nil, err
		}
	}
	groups, err := library.List()
	rows := make([]meetingListRow, 0, len(groups))
	for _, group := range groups {
		names := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			names = append(names, member.Name)
		}
		roster := strings.Join(names, " · ")
		rows = append(rows, meetingListRow{id: group.ID, topic: group.Topic, roster: roster, search: strings.ToLower(group.Topic + " " + roster), updated: group.Updated})
	}
	return library, rows, err
}

func (a *app) refreshMeetingList(scope meetingListScope) {
	state := &a.sessions.meetingList
	if state.scope != scope {
		*state = meetingSidebarState{scope: scope, row: -1, generation: state.generation + 1}
	}
	if !state.read.Start(scope, sessionsEvery) {
		return
	}
	generation, revision := state.generation, state.revision
	bg.Go(func() {
		library, rows, err := readMeetingList(scope)
		a.post(func() {
			if a.loadsClosed || generation != state.generation || !state.read.Done(scope) {
				return
			}
			if revision != state.revision {
				state.again = false
				state.read.Expire()
				return
			}
			state.library, state.rows, state.err, state.loaded = library, rows, err, true
			state.filtered = nil
			state.read.Keep(scope, sessionsEvery, a.invalidate)
			if state.again {
				state.again = false
				state.read.Expire()
			}
		})
	})
}

func (state *meetingSidebarState) matches() []meetingListRow {
	if state.filtered != nil && state.filter == state.query {
		return state.filtered
	}
	words := strings.Fields(strings.ToLower(state.query))
	state.filter, state.filtered = state.query, make([]meetingListRow, 0, len(state.rows))
	for _, row := range state.rows {
		if agents.HasWords(row.search, words) {
			state.filtered = append(state.filtered, row)
		}
	}
	return state.filtered
}

func (a *app) meetingSidebar(c *ui.Context, window *roundtableWindow) {
	scope := a.meetingListScope(window)
	a.refreshMeetingList(scope)
	state, palette := &a.sessions.meetingList, a.pal
	ui.Column(c.Key("meeting-list-head")).FillWidth().Padding(5, 10, 8, 10).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Center).Children(func() {
			if a.iconButton(c.Key("back-to-sessions"), iconArrowLeft, locales.MeetingBackToSessions(), false).Size(27, 27).Shrink(0).Clicked() {
				a.post(func() {
					if a.sessionsVisible() && a.meetingListWindow() == window {
						window.sessionsSidebar = true
						a.showSessions()
					}
				})
			}
			ui.Text(c, locales.MeetingListTitle()).FontWeight(600).FontSize(14).TextColor(palette.text).SingleLine().Grow(1).MinWidth(0).Tooltip(scope.project)
			if a.iconButton(c.Key("new-meeting"), iconPlus, locales.RoundtableNew(), false).Size(27, 27).Shrink(0).Disabled(a.root == "" || remote.Is(a.root)).Clicked() {
				a.post(func() {
					if a.meetingListWindow() == window && a.meetingListScope(window) == scope {
						state.query = ""
						if window.creating {
							window.focusTopic = true
						} else {
							window.newGroup()
						}
					}
				})
			}
		})
		query := a.searchField(c, "meeting-query", &state.query, locales.MeetingListFind(), &a.sessions.input, nil)
		if query.Changed() {
			state.row = 0
			state.list.ScrollTo(0, ui.Start)
		}
		rows := state.matches()
		if len(rows) > 0 {
			if query.Shortcut(0, ui.KeyDown) {
				state.row = min(len(rows)-1, state.row+1)
				state.list.ScrollIntoView(state.row)
			} else if query.Shortcut(0, ui.KeyUp) {
				state.row = max(0, state.row-1)
				state.list.ScrollIntoView(state.row)
			}
			if query.Submitted() {
				a.selectMeetingRow(window, scope, rows[min(max(0, state.row), len(rows)-1)].id)
			}
		}
	})
	if state.err != nil {
		ui.Text(c, locales.SessionsMeetingReadFailed(state.err.Error())).FontSize(11).TextColor(palette.dangerFg).Selectable().Padding(5, 12)
		if ui.Button(c, locales.Refresh()).AlignSelf(ui.Start).Margin(3, 12).Clicked() {
			state.expire()
		}
	}
	rows := state.matches()
	if !state.loaded || len(rows) == 0 {
		label := locales.RoundtableNoGroups()
		switch {
		case !state.loaded:
			label = locales.MeetingListLoading()
		case state.err != nil:
			return
		case strings.TrimSpace(state.query) != "":
			label = locales.MeetingListNoMatch()
		}
		ui.Text(c, label).FontSize(12).TextColor(palette.muted).Padding(8, 14).LineHeight(1.45)
		return
	}
	active := ""
	if !window.creating && window.meeting != nil && state.library.Path() == window.meeting.library.Path() {
		active = window.meeting.group.ID
	}
	state.list.Selected = &state.row
	state.list.Key = func(index int) any { return rows[index].id }
	state.list.Label = func(index int) string { return rows[index].topic }
	base := c.Theme()
	quiet := *base
	quiet.Accent, quiet.AccentText, quiet.SurfaceHover = ui.Color{}, palette.text, ui.Color{}
	c.SetTheme(&quiet)
	list := ui.List(c.Key("meeting-list"), &state.list, len(rows), func(index int) {
		record := rows[index]
		library := state.library
		selected := active == record.id
		row := ui.ButtonBase(c.Key(record.id)).FillWidth().Height(50).Radius(8).Gap(8).Padding(5, 7).
			Label(record.topic).Tooltip(record.topic + "\n" + record.roster).Checked(selected).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		row.ContextMenu(func(menu *ui.Menu) {
			if a.meetingDeleteItem(menu, library, record.id) {
				a.post(func() { a.askDeleteMeetingRecord(library, record.id, record.topic) })
			}
		})
		if selected {
			row.Background(palette.accent.Alpha(0.13))
		} else if row.Hovered() || row.FocusVisible() {
			row.Background(palette.hover)
		}
		if state.row == index && state.list.FocusWithin(c) {
			row.Border(1, palette.accentDim)
		}
		row.Children(func() {
			ui.Icon(c, uiicons.MessagesSquare()).FontSize(14).TextColor(palette.muted).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Text(c, record.topic).FontSize(12.5).FontWeight(600).TextColor(palette.text).SingleLine().Grow(1).MinWidth(0)
					ui.Text(c, ago(record.updated, c.Now())).FontSize(10).TextColor(palette.muted).SingleLine().Shrink(0).Tooltip(dateTime(record.updated.Local()))
				})
				label, ink := record.roster, palette.muted
				if work := a.backgroundMeetings[meetingRecordKey{library.Path(), record.id}]; work != nil {
					label, ink = locales.AgentWorking()+" · "+record.roster, palette.accent
					if len(work.chat.permissions) != 0 {
						label, ink = locales.AgentTaskApproval()+" · "+record.roster, palette.modifiedFg
					}
				}
				ui.Text(c, label).FontSize(11).TextColor(ink).SingleLine().MinWidth(0)
			})
		})
		if row.Clicked() {
			state.row = index
			a.selectMeetingRow(window, scope, record.id)
		}
	}).Grow(1).MinHeight(0).Padding(0, 5, 6, 5).Gap(2)
	c.SetTheme(base)
	if list.Submitted() && state.row >= 0 && state.row < len(rows) {
		a.selectMeetingRow(window, scope, rows[state.row].id)
	}
}

func (a *app) selectMeetingRow(window *roundtableWindow, scope meetingListScope, id string) {
	library := a.sessions.meetingList.library
	a.post(func() {
		if a.meetingListWindow() == window && a.meetingListScope(window) == scope && library != (roundtable.Library{}) {
			a.openMeetingRecord(library, id)
		}
	})
}

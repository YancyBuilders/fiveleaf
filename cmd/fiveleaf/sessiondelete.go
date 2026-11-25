package main

import (
	"errors"
	"os"
	"slices"
	"sync"
	"time"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/bg"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (a *app) trashSessionRecord(record agents.Session) {
	a.placeLive(time.Now())
	if a.sessionRecordInUse(record) {
		a.tell(note{message: locales.SessionsHistoryInUse()})
		return
	}
	path, err := record.HistoryTrashPath()
	if err == nil {
		err = a.host.trash(path)
	}
	if err != nil {
		a.fail("%s", locales.FailMoveTo(record.Label(), trashName(), err.Error()))
		a.readSessionsNow()
		return
	}
	for _, project := range a.projects() {
		project.forgetSessionRecord(record.Key())
	}
}

func (a *app) sessionRecordInUse(record agents.Session) bool {
	key := liveKey(record.Harness, record.ID)
	if _, running := a.sessions.running[key]; running {
		return true
	}
	if opened := a.sessions.open[key].t; opened != nil && opened.term != nil {
		select {
		case <-opened.term.Done():
		default:
			return true
		}
	}
	for _, project := range a.projects() {
		for _, chat := range project.allAgentChats() {
			if chat.client != nil && chat.sessionID == record.ID ||
				chat.requestInFlight() && chat.continuation != nil && chat.continuation.resume && chat.continuation.source.Key() == record.Key() {
				return true
			}
		}
	}
	return false
}

func (a *app) removeSessionRecord(record agents.Session) {
	key := record.Key()
	hidden := a.settings.RemovedAgentSessions
	if hidden == nil {
		hidden = make(map[string]bool)
		a.settings.RemovedAgentSessions = hidden
	}
	hidden[key] = true
	a.settings.Changed()
	for _, project := range a.projects() {
		project.forgetSessionRecord(key)
	}
	a.tell(note{message: locales.SessionsRemovedFromHistory(record.Label()), label: locales.ButtonUndo(), action: func() {
		delete(a.settings.RemovedAgentSessions, key)
		a.settings.Changed()
		for _, project := range a.projects() {
			project.readSessionsNow()
		}
	}})
}

func (a *app) forgetSessionRecord(key string) {
	state := &a.sessions
	state.sessions = slices.DeleteFunc(state.sessions, func(record agents.Session) bool { return record.Key() == key })
	state.version++
	a.readSessionsNow()
}

var meetingRecordsDeleting sync.Map

func meetingRecordDeleting(library roundtable.Library, id string) bool {
	_, deleting := meetingRecordsDeleting.Load(meetingRecordKey{library.Path(), id})
	return deleting
}

func (a *app) meetingDeleteItem(menu *ui.Menu, library roundtable.Library, id string) bool {
	return menu.Item(locales.SessionsDeleteMeeting()).
		Disabled(library == (roundtable.Library{}) || meetingRecordDeleting(library, id) || a.meetingRecordBusy(library, id)).Chosen()
}

func (a *app) askDeleteMeetingRecord(library roundtable.Library, id, title string) {
	a.present(&confirm{title: locales.DeleteQuestion(title), message: locales.SessionsDeleteMeetingDetail(),
		ok: locales.ButtonDelete(), danger: true, run: func() { a.deleteMeetingRecord(library, id, title) }})
}

func (a *app) meetingRecordBusy(library roundtable.Library, id string) bool {
	key := meetingRecordKey{library.Path(), id}
	for _, project := range a.projects() {
		if meeting := project.meeting; meeting != nil && meeting.key() == key && meeting.room.Busy() {
			return true
		}
		if meeting := project.backgroundMeetings[key]; meeting != nil && meeting.room.Busy() {
			return true
		}
	}
	return false
}

func (a *app) deleteMeetingRecord(library roundtable.Library, id, title string) {
	if a.meetingRecordBusy(library, id) {
		a.tell(note{message: locales.SessionsMeetingInUse()})
		return
	}
	key := meetingRecordKey{library.Path(), id}
	if _, deleting := meetingRecordsDeleting.LoadOrStore(key, struct{}{}); deleting {
		return
	}
	projects := a.projects()
	for _, project := range projects {
		project.detachMeetingRecord(key)
	}
	closing := pendingMeetingCloses(key)
	bg.Go(func() {
		waitMeetingCloses(closing)
		err := library.Delete(id)
		removed := err == nil
		if err != nil {
			_, readErr := library.Group(id)
			removed = errors.Is(readErr, os.ErrNotExist)
		}
		meetingRecordsDeleting.Delete(key)
		for _, project := range projects {
			project.post(func() {
				if project.loadsClosed {
					return
				}
				if err != nil && project == a {
					project.tell(note{message: locales.FailDelete(title, err.Error()), kind: "error",
						label: locales.PluginsIndexRetry(), action: func() { project.deleteMeetingRecord(library, id, title) }})
				}
				if removed {
					project.forgetMeetingRecord(library, id)
				}
			})
		}
	})
}

func (a *app) detachMeetingRecord(key meetingRecordKey) {
	if meeting := a.meeting; meeting != nil && meeting.key() == key {
		a.leaveMeeting()
	}
	if meeting := a.backgroundMeetings[key]; meeting != nil {
		delete(a.backgroundMeetings, key)
		a.releaseMeeting(meeting)
	}
	window := a.meetingWindow
	if window == nil || window.meeting == nil || window.meeting.key() != key {
		return
	}
	if window.meeting.readOnly && !window.creating {
		a.meetingWindow = nil
		a.dropTab(window.tab)
		window.finish()
		a.invalidate()
		return
	}
	draftKey := window.meeting.library.Key() + "/" + key.id
	if window.draft != "" || window.target != "" {
		window.drafts[draftKey] = meetingDraft{window.draft, window.target, window.mentions.selectedToken}
	} else {
		delete(window.drafts, draftKey)
	}
	window.meeting, window.media, window.confirmClear = nil, nil, false
	window.clearSentDraft()
	if !window.creating {
		window.newGroup()
	}
	a.invalidate()
}

func (a *app) forgetMeetingRecord(library roundtable.Library, id string) {
	state := &a.sessions.meetingList
	state.revision++
	if state.library.Path() == library.Path() {
		if index := slices.IndexFunc(state.matches(), func(row meetingListRow) bool { return row.id == id }); index >= 0 && index < state.row {
			state.row--
		}
		state.rows = slices.DeleteFunc(state.rows, func(row meetingListRow) bool { return row.id == id })
		state.filtered = nil
		state.row = min(state.row, len(state.matches())-1)
	}
	if window := a.meetingWindow; window != nil {
		delete(window.drafts, library.Key()+"/"+id)
	}
	a.forgetSessionRecord((agents.Session{Harness: meetingHistorySource, Root: library.Key(), ID: id}).Key())
}

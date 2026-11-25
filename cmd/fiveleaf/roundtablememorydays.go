package main

import (
	"fiveleaf/internal/bg"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingMemoryDays struct {
	request          *meetingMemoryDaysRequest
	days             []roundtable.MemoryDay
	err              error
	loading, refresh bool
}

type meetingMemoryDaysRequest struct {
	meeting              *agentMeeting
	generation           uint64
	lastMessage, lastPin string
	pins, recent         int
	readOnly             bool
}

func (w *roundtableWindow) loadMemoryDays(snapshot roundtable.Snapshot, memory roundtable.MemorySnapshot) {
	archive := &w.inspector.memory.archive
	if archive.loading {
		return
	}
	key := meetingMemoryDaysRequest{
		meeting: w.meeting, generation: w.viewGeneration, readOnly: w.meeting.readOnly,
		pins: len(memory.Notes), recent: len(memory.Recent),
	}
	if count := len(snapshot.Messages); count > 0 {
		key.lastMessage = snapshot.Messages[count-1].ID
	}
	if key.pins > 0 {
		key.lastPin = memory.Notes[key.pins-1].ID
	}
	if !archive.refresh && archive.request != nil && *archive.request == key {
		return
	}
	backfill := !key.readOnly && (archive.request == nil || archive.request.readOnly || archive.err != nil)
	request := &key
	archive.request, archive.loading, archive.refresh, archive.err = request, true, false, nil
	room := w.meeting.room
	bg.Go(func() {
		var syncErr error
		if backfill {
			syncErr = room.SyncMemoryDays()
		}
		days, err := room.MemoryDays()
		if err == nil {
			err = syncErr
		}
		w.postWindow(func() {
			if archive.request != request {
				return
			}
			archive.loading = false
			if w.meeting != request.meeting || w.viewGeneration != request.generation || w.drawer != meetingDrawerMemory {
				return
			}
			archive.days, archive.err = days, err
		})
	})
}

func (w *roundtableWindow) memoryDayList(c *ui.Context, snapshot roundtable.Snapshot, memory roundtable.MemorySnapshot) {
	w.loadMemoryDays(snapshot, memory)
	state, pal := &w.inspector.memory, w.pal
	archive := &state.archive
	ui.Text(c, locales.RoundtableMemoryArchive()).FontSize(12.5).FontWeight(600).TextColor(pal.text).Padding(8, 0, 0, 0)
	if archive.err != nil {
		ui.Text(c, archive.err.Error()).FontSize(11.5).TextColor(pal.dangerFg).Selectable()
		if ui.Button(c, locales.Refresh()).AlignSelf(ui.Start).Clicked() {
			archive.refresh = true
		}
		if len(archive.days) == 0 {
			return
		}
	}
	if len(archive.days) == 0 {
		label := locales.RoundtableMemoryArchiveEmpty()
		if archive.loading {
			label = locales.Loading()
		}
		ui.Text(c, label).FontSize(11.5).TextColor(pal.muted)
		return
	}
	limit := min(max(6, state.shown), len(archive.days))
	ui.Column(c.Key("memory-days")).FillWidth().MinWidth(0).Children(func() {
		for _, day := range archive.days[:limit] {
			row := ui.ButtonBase(c.Key(day.Date)).Label(day.Date).Tooltip(day.Path).FillWidth().MinHeight(28).Padding(0, 6).Gap(7).Radius(4).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if row.Hovered() || row.FocusVisible() {
				row.Background(pal.hover)
			}
			row.Children(func() {
				ui.Icon(c, iconFileText).FontSize(13).TextColor(pal.muted)
				ui.Text(c, day.Date).Grow(1).SingleLine().FontSize(12).FontFeatures("tnum").TextColor(pal.text)
				ui.Icon(c, iconChevronRight).FontSize(12).TextColor(pal.muted)
			})
			if row.Clicked() {
				w.post(func() {
					if w.drawer == meetingDrawerMemory {
						w.closeDrawer()
						w.a.openFile(day.Path, true)
					}
				})
			}
		}
	})
	if limit < len(archive.days) && ui.Button(c, locales.ShowMore()).AlignSelf(ui.Start).Clicked() {
		state.shown = limit + 6
	}
}

package main

import (
	"slices"
	"strconv"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

var meetingMemoryIcon, meetingPinIcon = uiicons.Brain(), uiicons.Pin()

type meetingMemoryEditor struct {
	draft, source, err                    string
	editing, focus, excerpt, confirmClear bool
	shown                                 int
	archive                               meetingMemoryDays
	scroll                                ui.ScrollState
}

func (w *roundtableWindow) openMeetingMemory() {
	if w.drawer != meetingDrawerMemory {
		w.inspector.memory.archive = meetingMemoryDays{}
	}
	w.openDrawer(meetingDrawerMemory)
	w.inspector.memory.focus = w.inspector.memory.editing
}

func (w *roundtableWindow) pinnedMemoryStrip(c *ui.Context, notes []roundtable.MemoryNote) {
	if len(notes) == 0 {
		return
	}
	p := w.pal
	summary, _ := meetingExcerpt(notes[0].Text)
	strip := ui.Column(c.Key("pinned-memory-strip")).FillWidth().MinWidth(0).Radius(6).Border(1, p.border).Background(materialOf(c).insetColor(p.editor))
	strip.Children(func() {
		ui.Row(c.Key("pin-header")).FillWidth().MinWidth(0).MinHeight(34).AlignItems(ui.Center).Gap(3).Padding(0, 4, 0, 8).Children(func() {
			title := locales.RoundtableMemoryPinned(len(notes))
			open := ui.ButtonBase(c.Key("open-pins")).Label(locales.RoundtableOpenPinned()).Tooltip(title + "\n" + notes[0].Text).KeepFocus().Grow(1).MinWidth(0).MinHeight(32).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			open.Children(func() {
				ui.Icon(c, meetingMemoryIcon).FontSize(13).TextColor(p.accent).Shrink(0)
				ui.Text(c, locales.RoundtableMemoryTab()).FontSize(12).FontWeight(600).TextColor(p.text).SingleLine().Shrink(0)
				ui.Text(c, strconv.Itoa(len(notes))).Description(title).FontSize(10.5).FontFeatures("tnum").TextColor(p.accent).Background(p.accent.Alpha(0.09)).Radius(5).Padding(1, 5).Shrink(0)
				ui.Text(c, summary).FontSize(11.5).TextColor(p.muted).Grow(1).MinWidth(0).SingleLine()
				ui.Text(c, locales.RoundtablePinMaintainer()).FontSize(10.5).TextColor(p.muted).SingleLine().Shrink(0)
			})
			if open.Clicked() {
				w.openMeetingMemory()
			}
			icon := iconChevronRight
			if w.pinsExpanded {
				icon = iconChevronDown
			}
			if roundtableIconButton(c, "toggle-pins", icon, locales.RoundtablePreviewPins(), p).KeepFocus().Expanded(w.pinsExpanded).Clicked() {
				w.pinsExpanded = !w.pinsExpanded
			}
		})
		if !w.pinsExpanded {
			return
		}
		for _, note := range notes[:min(3, len(notes))] {
			source := locales.RoundtableMemoryManual()
			if note.SourceID != "" {
				source = locales.RoundtableMemoryFromChat()
			}
			entry := ui.ButtonBase(c.Key("pin-entry").Key(note.ID)).FillWidth().MinWidth(0).Gap(7).AlignItems(ui.Start).BorderWidth(1, 0, 0, 0).BorderColor(p.border).Padding(6, 9).KeepFocus().Label(note.Text).Cursor(ui.CursorPointer)
			entry.Children(func() {
				ui.Text(c, source).FontSize(10).TextColor(p.muted).Background(p.hover).Radius(4).Padding(2, 4).SingleLine().Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, note.Text).FontSize(12).TextColor(p.text).FillWidth()
					ui.Text(c, dateTime(note.Time.Local())).FontSize(10.5).TextColor(p.muted).FontFeatures("tnum")
				})
			})
			if entry.Clicked() {
				w.openMeetingMemory()
			}
		}
	})
}

func (w *roundtableWindow) beginMemoryNote(message roundtable.Message) {
	if w.meeting == nil || w.meeting.readOnly || w.meeting.changing != nil || w.meeting.room.Busy() {
		return
	}
	w.openMeetingMemory()
	state := &w.inspector.memory
	state.draft, state.excerpt = agentVisibleText(message.Text, roundtable.MaxMemoryNoteBytes)
	state.source, state.editing, state.focus = message.ID, true, true
	state.err, state.confirmClear, state.scroll.Y = "", false, 0
}

func (w *roundtableWindow) changeMemory(change func(*roundtable.Room) error, done func()) {
	w.changeMeeting(meetingMemoryChange, change, func(meeting *agentMeeting, err error) {
		if w.closed || w.creating || w.meeting != meeting {
			return
		}
		state := &w.inspector.memory
		if err != nil {
			state.focus = state.editing
			switch err {
			case roundtable.ErrMemoryBusy:
				state.err = locales.RoundtableMemoryBusy()
			case roundtable.ErrMemoryDuplicate:
				state.err = locales.RoundtableMemoryDuplicate()
			case roundtable.ErrMemoryLimit:
				state.err = locales.RoundtableMemoryLimit(roundtable.MaxMemoryNotes)
			case roundtable.ErrMemoryNote:
				state.err = locales.RoundtableMemoryTooLong(roundtable.MaxMemoryNoteBytes)
			default:
				state.err = err.Error()
			}
			return
		}
		state.err = ""
		if done != nil {
			done()
		}
		state.archive.refresh = true
	})
}

func (w *roundtableWindow) memoryPanel(c *ui.Context, snapshot roundtable.Snapshot) {
	memory := w.meeting.room.Memory()
	state, pal := &w.inspector.memory, w.pal
	editable := !w.meeting.readOnly && !snapshot.Busy && w.meeting.changing == nil
	ui.Scroll(c.Key("memory-content")).Grow(1).FillWidth().MinHeight(0).TrackScroll(&state.scroll).Padding(12).Children(func() {
		ui.Column(c).FillWidth().Gap(9).Children(func() {
			shared := memory.Shared
			ui.Row(c).AlignItems(ui.Center).FillWidth().Gap(8).Children(func() {
				label := ui.Text(c, locales.RoundtableShareMemory()).FontSize(13).FontWeight(600).TextColor(pal.text).Grow(1).MinWidth(0)
				toggle := ui.Switch(c, &shared).Label(locales.RoundtableShareMemory()).Disabled(!editable)
				if editable && (label.Clicked() || toggle.Changed()) {
					w.changeMemory(func(room *roundtable.Room) error { return room.SetMemorySharing(!memory.Shared) }, nil)
				}
			})
			hint := locales.RoundtableMemorySharingOn()
			if !shared {
				hint = locales.RoundtableMemorySharingOff()
			}
			ui.Text(c, hint).FontSize(11.5).TextColor(pal.muted)
			w.memoryWindow(c, memory.Window, editable && shared)
			if snapshot.Busy && !w.meeting.readOnly {
				ui.Text(c, locales.RoundtableMemoryBusy()).FontSize(11.5).TextColor(pal.muted)
			}
			if w.meeting.readOnly {
				ui.Text(c, locales.SessionsMeetingReadOnly()).FontSize(11.5).TextColor(pal.muted)
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(10, 0, 0, 0).Gap(5).Children(func() {
				ui.Icon(c, meetingPinIcon).FontSize(13).TextColor(pal.accent)
				ui.Text(c, locales.RoundtablePinnedNotesHeading()).FontSize(12.5).FontWeight(600).TextColor(pal.text).Grow(1).MinWidth(0)
				if !w.meeting.readOnly && !state.editing && roundtableIconButton(c, "add-note", iconPlus, locales.RoundtableMemoryAdd(), pal).Disabled(!editable).Clicked() {
					w.beginMemoryNote(roundtable.Message{})
				}
			})
			if state.editing && !w.meeting.readOnly {
				w.memoryNoteForm(c, editable)
			}
			if len(memory.Notes) == 0 && !state.editing {
				ui.Text(c, locales.RoundtableMemoryNoNotes()).FontSize(11.5).TextColor(pal.muted)
			}
			for index := len(memory.Notes) - 1; index >= 0; index-- {
				w.pinnedMemoryNote(c, memory.Notes[index], editable)
			}
			w.memoryDayList(c, snapshot, memory)
		})
	})
	if state.err != "" {
		ui.Text(c, state.err).FontSize(12).TextColor(pal.dangerFg).Selectable().Padding(4, 12)
	}
	if w.meeting.readOnly {
		return
	}
	ui.Column(c.Key("memory-footer")).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Padding(8, 12).Gap(5).Children(func() {
		if state.confirmClear {
			ui.Text(c, locales.RoundtableMemoryConfirmClear()).FontSize(12).TextColor(pal.text)
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				if ui.Button(c, locales.ButtonCancel()).Disabled(w.meeting.changing != nil).Clicked() {
					state.confirmClear = false
				}
				if ui.Button(c, locales.RoundtableMemoryClear()).Disabled(!editable).Clicked() {
					w.changeMemory((*roundtable.Room).ClearMemory, func() { *state = meetingMemoryEditor{} })
				}
			})
			return
		}
		if actionButton(c.Key("clear-memory"), locales.RoundtableMemoryClear(), iconTrash).Disabled(!editable).AlignSelf(ui.Start).Clicked() {
			state.confirmClear = true
		}
	})
}

func (w *roundtableWindow) memoryWindow(c *ui.Context, window int, editable bool) {
	values := []int{12, 30, 50}
	if !slices.Contains(values, window) {
		values = append(values, window)
		slices.Sort(values)
	}
	choices := make([]string, len(values))
	for index, value := range values {
		choices[index] = strconv.Itoa(value)
	}
	selected := slices.Index(values, window)
	ui.Text(c, locales.RoundtableMemoryWindow()).FontSize(11.5).TextColor(w.pal.muted)
	if ui.Segmented(c.Key("memory-window"), &selected, choices...).Disabled(!editable).Label(locales.RoundtableMemoryWindow()).Changed() && selected >= 0 && selected < len(values) {
		turns := values[selected]
		w.changeMemory(func(room *roundtable.Room) error { return room.SetMemoryWindow(turns) }, nil)
	}
}

func (w *roundtableWindow) memoryNoteForm(c *ui.Context, editable bool) {
	state := &w.inspector.memory
	field := ui.TextArea(c.Key("add-memory-text"), &state.draft).FillWidth().Height(64).FontSize(12.5).Disabled(!editable).Label(locales.RoundtableMemoryNote()).Placeholder(locales.RoundtableMemoryNote())
	if state.focus && editable {
		field.Focus()
		state.focus = false
	}
	if state.excerpt {
		ui.Text(c, locales.RoundtableMemoryExcerpt()).FontSize(11).TextColor(w.pal.muted)
	}
	tooLong := len(strings.TrimSpace(state.draft)) > roundtable.MaxMemoryNoteBytes
	if tooLong {
		ui.Text(c, locales.RoundtableMemoryTooLong(roundtable.MaxMemoryNoteBytes)).FontSize(11.5).TextColor(w.pal.dangerFg)
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, locales.ButtonSave()).Disabled(!editable || tooLong || strings.TrimSpace(state.draft) == "").Clicked() {
			text, source := state.draft, state.source
			w.changeMemory(func(room *roundtable.Room) error { return room.AddMemory(text, source) }, func() { state.editing, state.draft, state.source = false, "", "" })
		}
		if ui.Button(c, locales.ButtonCancel()).Disabled(w.meeting.changing != nil).Clicked() {
			state.editing, state.draft, state.source, state.err = false, "", "", ""
		}
	})
}

func (w *roundtableWindow) pinnedMemoryNote(c *ui.Context, note roundtable.MemoryNote, editable bool) {
	p := w.pal
	box := ui.Column(c.Key(note.ID)).FillWidth().MinWidth(0).Padding(8, 9).Gap(3).Radius(6).Background(materialOf(c).insetColor(p.surface))
	box.Children(func() {
		ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
			label := locales.RoundtableMemoryManual()
			if note.SourceID != "" {
				label = locales.RoundtableMemoryFromChat()
			}
			ui.Text(c, label).FontSize(10.5).TextColor(p.muted).Grow(1).MinWidth(0)
			ui.Text(c, note.Time.Local().Format("01-02 15:04")).FontSize(10.5).TextColor(p.muted).FontFeatures("tnum").Tooltip(dateTime(note.Time.Local()))
			if !w.meeting.readOnly {
				brief, _ := agentVisibleText(note.Text, 72)
				remove := roundtableIconButton(c, "remove", iconX, locales.RoundtableMemoryRemove(brief), p).Size(22, 22).Disabled(!editable)
				if remove.Clicked() {
					w.changeMemory(func(room *roundtable.Room) error { return room.RemoveMemory(note.ID) }, nil)
				}
			}
		})
		open := ui.Local(box, "note-open", func() bool { return false })
		preview, longer := meetingExcerpt(note.Text)
		if *open {
			preview = note.Text
		}
		ui.Text(c, preview).FontSize(12.5).TextColor(p.text).Selectable()
		if longer {
			label := locales.RoundtableFullResult()
			if *open {
				label = locales.RoundtableCollapse()
			}
			if ui.Button(c, label).Expanded(*open).AlignSelf(ui.Start).Clicked() {
				*open = !*open
			}
		}
	})
}

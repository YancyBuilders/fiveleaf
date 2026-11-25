package main

import (
	"cmp"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (a *app) roundtableDataDir() string {
	dir := cmp.Or(os.Getenv("FIVELEAF_DATA_DIR"), os.Getenv("MAYOI_DATA_DIR"))
	if dir == "" {
		path := a.settings.Path()
		if path == "" {
			path = settingsPath()
		}
		if path != "" {
			dir = filepath.Dir(path)
		}
	}
	return dir
}

func (a *app) roundtableLibrary() (roundtable.Library, error) {
	return roundtable.ProjectLibrary(a.roundtableDataDir(), a.root)
}

func (w *roundtableWindow) newGroup() {
	w.viewGeneration++
	w.closeDrawer()
	w.creating, w.topic, w.brief, w.err = true, "", "", ""
	w.templateBrief = ""
	w.createHumanChair = false
	w.focusTopic = true
	w.selected = map[string]roundtable.Participant{}
	w.inviteScroll = ui.ScrollState{}
}

func (a *app) openMeetingRecord(lib roundtable.Library, id string) {
	a.meetingOpenSerial++
	if meetingRecordDeleting(lib, id) {
		return
	}
	if meeting := a.meeting; meeting != nil && meeting.library == lib && meeting.group.ID == id {
		a.showRoundtable(meeting)
		return
	}
	if closing := pendingMeetingCloses(meetingRecordKey{lib.Path(), id}); len(closing) > 0 {
		serial, root, window := a.meetingOpenSerial, a.root, a.meetingWindow
		var generation uint64
		if window != nil {
			generation = window.viewGeneration
		}
		bg.Go(func() {
			waitMeetingCloses(closing)
			a.post(func() {
				if !a.loadsClosed && a.meetingOpenSerial == serial && a.root == root && a.meetingWindow == window &&
					(window == nil || window.viewGeneration == generation) {
					a.openMeetingRecord(lib, id)
				}
			})
		})
		return
	}
	group, err := lib.Group(id)
	if err == nil {
		var meeting *agentMeeting
		meeting, err = a.startMeeting(lib, group)
		if err == nil {
			a.showRoundtable(meeting)
			return
		}
	}
	a.fail("%s", locales.SessionsContinueFailed(err.Error()))
}

type meetingDraft struct{ text, target, mention string }

func (w *roundtableWindow) enter(meeting *agentMeeting) {
	if w.creating || w.meeting != meeting {
		w.viewGeneration++
	}
	if w.meeting == meeting {
		w.creating = false
		return
	}
	if previous := w.meeting; previous != nil {
		key := previous.library.Key() + "/" + previous.group.ID
		if w.draft == "" && w.target == "" {
			delete(w.drafts, key)
		} else {
			w.drafts[key] = meetingDraft{w.draft, w.target, w.mentions.selectedToken}
		}
	}
	w.meeting, w.creating, w.confirmClear = meeting, false, false
	w.closeDrawer()
	draft := w.drafts[meeting.library.Key()+"/"+meeting.group.ID]
	w.draft, w.target, w.mentions = draft.text, draft.target, meetingMentionPicker{selectedToken: draft.mention}
	w.lastUserMessage, w.err = "", ""
	w.view, w.graphSel, w.pinsExpanded = roundtableViewChat, "", false
	w.graphMatrixPage = 0
	w.graphScroll = ui.ScrollState{}
	w.inspector, w.media, w.focusDraft = meetingInspector{}, nil, false
	w.selectMember("")
	w.page, w.pageCursors, w.pageGraph = nil, nil, roundtable.Graph{}
	w.membersScroll, w.scroll = ui.ScrollState{}, ui.ScrollState{Y: math.MaxFloat32}
}

func (w *roundtableWindow) invitation(choice meetingSpeaker) (roundtable.Participant, bool) {
	if choice.ID == roundtable.ModeratorID {
		return choice.Participant, !w.createHumanChair && choice.problem == ""
	}
	ref, selected := w.selected[choice.ID]
	return ref, selected
}

func (w *roundtableWindow) invitations(choices []meetingSpeaker) []roundtable.Participant {
	var members []roundtable.Participant
	for _, choice := range choices {
		ref, selected := w.invitation(choice)
		if !selected || choice.problem != "" || ref.Provider != choice.Provider {
			continue
		}
		if ref.ID == roundtable.ModeratorID {
			members = slices.Insert(members, 0, ref)
		} else {
			members = append(members, ref)
		}
	}
	return members
}

func (w *roundtableWindow) validInvitations(members []roundtable.Participant) bool {
	if len(members) > roundtable.MaxMembers {
		return false
	}
	if len(members) != 0 && members[0].ID == roundtable.ModeratorID {
		return !w.createHumanChair && len(members) == len(w.selected)+1
	}
	return w.createHumanChair && len(members) == len(w.selected)
}

func (w *roundtableWindow) createGroup() {
	members := w.invitations(w.a.meetingInvitations())
	if !w.validInvitations(members) {
		w.err = locales.RoundtableTargetUnavailable()
		return
	}
	lib, err := w.a.roundtableLibrary()
	if err != nil {
		w.err = err.Error()
		return
	}
	group, err := lib.CreateWithChair(w.topic, members, w.createHumanChair)
	if err == nil {
		w.a.readSessionsNow()
		var meeting *agentMeeting
		meeting, err = w.a.startMeeting(lib, group)
		if err == nil {
			w.enter(meeting)
			return
		}
	}
	w.err = err.Error()
}

func (w *roundtableWindow) createView(c *ui.Context) {
	p := w.pal
	ui.Row(c.Key("create-header")).Height(54).FillWidth().Shrink(0).AlignItems(ui.Center).Gap(10).Padding(0, 16).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
		ui.Icon(c, iconMessageText).FontSize(19).TextColor(p.accent)
		ui.Text(c, locales.RoundtableNew()).FontSize(15).FontWeight(600).TextColor(p.text).SingleLine().Grow(1).MinWidth(0)
		if w.meeting != nil && ui.Button(c, locales.ButtonCancel()).Clicked() {
			w.enter(w.meeting)
		}
	})
	ui.Column(c.Key("create-body")).Grow(1).MinHeight(0).FillWidth().AlignItems(ui.Center).Padding(10, 22, 16, 22).Children(func() {
		w.inviteForm(c)
	})
}

func (w *roundtableWindow) inviteForm(c *ui.Context) {
	p := w.pal
	choices := w.a.meetingInvitations()
	if host := slices.IndexFunc(choices, func(s meetingSpeaker) bool { return s.ID == roundtable.ModeratorID }); host > 0 {
		first := choices[host]
		copy(choices[1:host+1], choices[:host])
		choices[0] = first
	}
	for id := range w.selected {
		if !slices.ContainsFunc(choices, func(speaker meetingSpeaker) bool { return speaker.ID == id }) {
			delete(w.selected, id)
			w.err = locales.RoundtableTargetUnavailable()
		}
	}
	ui.Column(c.Key("invite-form")).Fill().MaxWidth(670).Gap(10).Children(func() {
		ui.Row(c.Key("topic-heading")).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(8).Children(func() {
			label := ui.Text(c, locales.RoundtableTopic()).FontSize(13).FontWeight(600).TextColor(p.text).Grow(1).MinWidth(0)
			if label.Clicked() {
				w.focusTopic = true
			}
			w.templatePicker(c, choices)
		})
		input := ui.TextInput(c.Key("topic"), &w.topic).FillWidth().Height(36).FontSize(14).Label(locales.RoundtableTopic()).Placeholder(locales.RoundtableTopicExample())
		if w.focusTopic {
			input.Focus()
			w.focusTopic = false
		}
		ui.TextArea(c.Key("brief"), &w.brief).FillWidth().Height(62).FontSize(12.5).Label(locales.RoundtableBrief()).Placeholder(locales.RoundtableBrief())
		w.createHumanChair, _ = w.chairPicker(c, w.createHumanChair, false)
		inviteLimit := roundtable.MaxMembers
		if !w.createHumanChair {
			inviteLimit--
		}
		ui.Text(c, locales.RoundtableInvite()).FontSize(13).FontWeight(600).TextColor(p.text).Padding(8, 0, 0, 0)
		ui.Scroll(c.Key("invite-list")).TrackScroll(&w.inviteScroll).Grow(1).MinHeight(0).FillWidth().Children(func() {
			ui.Column(c).Gap(2).FillWidth().Children(func() {
				section := ""
				for _, choice := range choices {
					kind, _, _ := strings.Cut(choice.ID, ":")
					if kind != section {
						section = kind
						if kind == "agent" {
							ui.Text(c, locales.AgentRoster()).FontSize(11).FontWeight(600).TextColor(p.muted).Padding(8, 8, 4, 8)
						} else if kind != "preset" {
							ui.Text(c, locales.AgentRoles()).FontSize(11).FontWeight(600).TextColor(p.muted).Padding(8, 8, 4, 8)
						}
					}
					ref, selected := w.invitation(choice)
					host := choice.ID == roundtable.ModeratorID
					valid := choice.problem == "" && (!selected || ref.Provider == choice.Provider)
					disabled := host || !selected && (!valid || len(w.selected) >= inviteLimit)
					row := ui.ButtonBase(c.Key(choice.ID)).Role(ui.RoleCheckBox).Checked(selected).Label(choice.Name).Description(choice.detail()).Disabled(disabled).
						FillWidth().MinHeight(40).Radius(5).Padding(0, 9).AlignItems(ui.Center).Gap(6)
					ink := p.text
					if selected {
						row.Background(p.accent.Alpha(0.10))
					}
					if !valid {
						ink = p.muted
					} else if !disabled {
						row.Cursor(ui.CursorPointer)
					}
					if row.Hovered() || row.FocusVisible() {
						row.Background(p.hover)
					}
					row.Children(func() {
						meetingDot(c, p, choice)
						ui.Text(c, choice.Name).FontSize(13).SingleLine().MinWidth(0).Grow(1).TextColor(ink)
						detail := choice.detail()
						if host && selected {
							detail = locales.RoundtableAutoJoined()
						}
						ui.Text(c, detail).FontSize(11).SingleLine().MaxWidth(240).TextColor(p.muted).Tooltip(choice.detail())
						mark := ui.Box(c).Size(16, 16).Radius(4).Border(1, p.border).Center()
						if selected {
							mark.Background(p.accent).Children(func() { ui.Icon(c, iconCheck).FontSize(12).TextColor(p.editor) })
						}
					})
					if !disabled && row.Clicked() {
						if selected {
							delete(w.selected, choice.ID)
						} else {
							w.selected[choice.ID] = choice.Participant
						}
					}
				}
			})
		})
		if w.err != "" {
			ui.Text(c, w.err).FontSize(12).TextColor(p.dangerFg).Selectable()
		}
		ui.Row(c.Key("create-footer")).AlignItems(ui.Center).Padding(8, 0, 0, 0).Gap(12).Children(func() {
			members := w.invitations(choices)
			validInvitations := w.validInvitations(members)
			count := len(members)
			label := locales.RoundtableSelection(count)
			if count > roundtable.MaxMembers {
				label += " / " + strconv.Itoa(roundtable.MaxMembers)
			} else if !w.createHumanChair && (count == 0 || members[0].ID != roundtable.ModeratorID) {
				label = locales.RoundtableModeratorUnavailable()
			} else if !validInvitations {
				label = locales.RoundtableInvitationsUnavailable()
			}
			if length := utf8.RuneCountInString(w.topic); length > roundtable.MaxTopicRunes {
				label = locales.RoundtableTopic() + "  " + strconv.Itoa(length) + " / " + strconv.Itoa(roundtable.MaxTopicRunes)
			}
			ui.Text(c, label).FontSize(12).TextColor(p.muted).Grow(1).MinWidth(0)
			invalid := !roundtable.ValidTopic(w.topic) || !validInvitations
			if ui.Button(c, locales.RoundtableCreate()).Disabled(invalid).Clicked() {
				w.post(func() {
					w.createGroup()
					if !w.creating {
						w.draft = w.brief
					}
				})
			}
			if ui.PrimaryButton(c, locales.RoundtableStart()).Disabled(invalid).Clicked() {
				w.post(func() {
					w.createGroup()
					if !w.creating && w.meeting != nil {
						w.draft = cmp.Or(strings.TrimSpace(w.brief), w.meeting.group.Topic)
						w.send()
					}
				})
			}
		})
	})
}

const meetingPageSize = 40

func (w *roundtableWindow) historyPage(c *ui.Context, snapshot roundtable.Snapshot) []roundtable.Message {
	messages := slices.DeleteFunc(slices.Clone(snapshot.Messages), func(message roundtable.Message) bool { return !message.InOverview() })
	earlier := snapshot.HasEarlier || len(messages) > meetingPageSize
	if w.page != nil {
		messages, earlier = w.page, w.moreBefore
	} else if len(messages) > meetingPageSize {
		messages = messages[len(messages)-meetingPageSize:]
	}
	if !earlier && w.page == nil {
		return messages
	}
	ui.Row(c.Key("history-pages")).Gap(8).Padding(5, 14).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, locales.RoundtableEarlier()).FontSize(11.5).Disabled(!earlier).Clicked() && len(messages) > 0 {
			first := messages[0]
			cursor := roundtable.Message{ID: first.ID, Time: first.Time}
			cursors := append(slices.Clone(w.pageCursors), cursor)
			w.post(func() { w.loadPage(cursors) })
		}
		if w.page != nil && ui.Button(c, locales.RoundtableNewer()).FontSize(11.5).Clicked() {
			cursors := w.pageCursors[:len(w.pageCursors)-1]
			w.post(func() { w.loadPage(cursors) })
		}
	})
	return messages
}

func (w *roundtableWindow) loadPage(cursors []roundtable.Message) {
	if len(cursors) == 0 {
		w.page, w.pageCursors, w.pageGraph = nil, nil, roundtable.Graph{}
		w.scroll.Y = math.MaxFloat32
		return
	}
	page, err := w.meeting.room.EarlierOverviewPage(cursors[len(cursors)-1], meetingPageSize)
	if err != nil {
		w.err = err.Error()
		return
	}
	if len(page.Messages) == 0 {
		return
	}
	w.pageGraph = roundtable.BuildTaskGraph(roundtable.Snapshot{Messages: page.TaskContext})
	w.page, w.pageCursors, w.moreBefore = page.Messages, cursors, page.HasEarlier
	w.scroll.Y = 0
}

func (w *roundtableWindow) historyGraph() roundtable.Graph {
	return w.pageGraph
}

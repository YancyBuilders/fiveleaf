package main

import (
	"cmp"
	"errors"
	"slices"
	"strconv"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingInspector struct {
	meetingPanelVisibility
	chatVisibility meetingPanelVisibility
	inviting       bool
	query          string
	memory         meetingMemoryEditor
}

type meetingPanelVisibility struct{ open, hidden bool }

func (panel meetingPanelVisibility) visible(wide bool) bool {
	return panel.open || wide && !panel.hidden
}

func (panel *meetingPanelVisibility) setVisible(open bool) { panel.open, panel.hidden = open, !open }

func (panel *meetingInspector) switchView(view roundtableView) {
	if view == roundtableViewGraph {
		panel.chatVisibility = panel.meetingPanelVisibility
		panel.meetingPanelVisibility = meetingPanelVisibility{}
	} else {
		panel.meetingPanelVisibility = panel.chatVisibility
	}
	panel.inviting = false
}

func (w *roundtableWindow) returnToChat() {
	w.view = roundtableViewChat
	w.selectMember("")
	w.inspector = meetingInspector{memory: w.inspector.memory}
}

func (w *roundtableWindow) inspectorWidth(width float32) float32 {
	if w.memberID != "" {
		return min(408, width*0.44)
	}
	if width <= 1240 {
		return 224
	}
	return 244
}

func (w *roundtableWindow) memberInspector(c *ui.Context, snapshot roundtable.Snapshot, graph roundtable.Graph, width float32, wide bool) {
	p := w.pal
	panel := ui.Column(c.Key("member-inspector")).MinHeight(0).MinWidth(0).Background(p.editor)
	if material := materialOf(c); material.mica() {
		material.glassCard(panel, p, p.metricsOr().radiusCard)
	}
	if wide {
		panel.Width(w.inspectorWidth(width)).Shrink(0).BorderWidth(0, 0, 0, 1).BorderColor(p.border)
	} else {
		panel.Grow(1)
	}
	panel.Children(func() {
		ui.Row(c.Key("members-heading")).FillWidth().AlignItems(ui.Center).Padding(9, 13).Gap(6).Shrink(0).
			BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
			ui.Text(c, locales.RoundtableMembersHeading()).Grow(1).MinWidth(0).FontSize(12).FontWeight(600).TextColor(p.text)
			ui.Text(c, strconv.Itoa(len(w.meeting.speakers))).FontSize(10).FontFeatures("tnum").TextColor(p.muted)
		})
		switch {
		case w.inspector.inviting && !w.meeting.readOnly:
			w.inviteMoreMembers(c, snapshot)
		case w.memberID != "":
			if actionButton(c.Key("back"), locales.RoundtableBackToMembers(), iconChevronLeft).Margin(8, 10, 0, 10).AlignSelf(ui.Start).Clicked() {
				w.selectMember("")
			}
			w.taskHistory(c, snapshot)
		default:
			if !w.meeting.readOnly && w.meeting.group.CanInviteParticipant() {
				invite := actionButton(c.Key("invite"), locales.RoundtableInvite(), iconPlus).Margin(10, 11, 0, 11).MinHeight(30).
					AlignSelf(ui.Stretch).Justify(ui.Center).Shrink(0).Radius(7).Border(1, p.border).BorderStyle(ui.BorderDashed).Background(p.panel).TextColor(p.text)
				if invite.Hovered() || invite.FocusVisible() {
					invite.BorderColor(p.accent).TextColor(p.accent)
				}
				if invite.Clicked() {
					w.inspector.inviting, w.inspector.query = true, ""
				}
			}
			w.members(c, snapshot, graph)
			if w.err != "" && (!wide || w.view == roundtableViewGraph) {
				ui.Text(c, w.err).FillWidth().MinWidth(0).Padding(6, 11).FontSize(12).TextColor(p.dangerFg).Selectable()
			}
		}
	})
}

func (w *roundtableWindow) collaborationSettings(c *ui.Context, snapshot roundtable.Snapshot) {
	p := w.pal
	ui.Scroll(c.Key("collaboration")).Grow(1).MinHeight(0).Padding(14).Children(func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			w.chairSettings(c, snapshot)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				if ui.Text(c, locales.RoundtableExpandReplies()).FontSize(12.5).TextColor(p.text).Grow(1).MinWidth(0).Cursor(ui.CursorPointer).Clicked() {
					w.showFullReplies = !w.showFullReplies
				}
				ui.Switch(c, &w.showFullReplies).Label(locales.RoundtableExpandReplies())
			})
			if w.meeting.readOnly {
				return
			}
			if w.confirmClear {
				w.confirmClearMeeting(c)
			} else if actionButton(c.Key("clear"), locales.AgentClearChat(), iconTrash).AlignSelf(ui.Start).Clicked() {
				w.confirmClear = true
			}
			if w.err != "" {
				ui.Text(c, w.err).FontSize(12).TextColor(p.dangerFg).Selectable()
			}
		})
	})
}

func (w *roundtableWindow) inviteMoreMembers(c *ui.Context, snapshot roundtable.Snapshot) {
	p := w.pal
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Padding(9).Gap(6).Children(func() {
		if actionButton(c, locales.RoundtableBackToMembers(), iconChevronLeft).AlignSelf(ui.Start).Clicked() {
			w.inspector.inviting = false
		}
		ui.TextInput(c.Key("invite-filter"), &w.inspector.query).FillWidth().Label(locales.RoundtableFindMembers()).Placeholder(locales.RoundtableFindMembers()).AutoFocus()
		ui.Scroll(c.Key("invite-more")).Grow(1).MinHeight(0).Children(func() {
			count := 0
			query := strings.ToLower(strings.TrimSpace(w.inspector.query))
			for _, candidate := range w.a.meetingInvitations() {
				if slices.ContainsFunc(w.meeting.speakers, func(s meetingSpeaker) bool { return s.ID == candidate.ID }) || !strings.Contains(strings.ToLower(candidate.Name+" "+candidate.ID+" "+candidate.Provider), query) {
					continue
				}
				count++
				label := locales.RoundtableAddMember(candidate.Name)
				row := ui.ButtonBase(c.Key(candidate.ID)).FillWidth().MinHeight(40).Padding(6).Gap(6).AlignItems(ui.Center).Label(label).Disabled(candidate.problem != "" || snapshot.Busy).Tooltip(candidate.detail())
				if row.Hovered() {
					row.Background(p.hover)
				}
				row.Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							meetingDot(c, p, candidate)
							ui.Text(c, candidate.Name).FontSize(12.5).TextColor(p.text).SingleLine().MinWidth(0)
						})
						ui.Text(c, cmp.Or(candidate.problem, candidate.detail())).FontSize(10.5).TextColor(p.muted).SingleLine()
					})
					ui.Icon(c, iconPlus).FontSize(13).TextColor(p.muted)
				})
				if row.Clicked() {
					w.post(func() { w.addParticipant(candidate.Participant) })
				}
			}
			if count == 0 {
				ui.Text(c, locales.RoundtableNoInvites()).FontSize(12).TextColor(p.muted).Padding(6)
			}
		})
		if snapshot.Busy {
			ui.Text(c, locales.RoundtableInviteAfterTurn()).FontSize(12).TextColor(p.muted)
		}
		if w.err != "" {
			ui.Text(c, w.err).FontSize(12).TextColor(p.dangerFg)
		}
	})
}

func (w *roundtableWindow) addParticipant(ref roundtable.Participant) {
	meeting := w.meeting
	if meeting == nil || meeting.readOnly || w.creating {
		return
	}
	if meeting.room.Snapshot().Busy {
		w.err = locales.RoundtableInviteAfterTurn()
		return
	}
	providers := w.a.agentProviders()
	if candidate := w.a.resolveSpeaker(ref, providers); candidate.problem != "" {
		w.err = candidate.problem
		return
	}
	group, err := meeting.room.InviteParticipant(ref)
	if err != nil {
		w.err = err.Error()
		return
	}
	w.applyMemberGroup(group, providers)
}

func (w *roundtableWindow) removeParticipant(id string) {
	meeting := w.meeting
	if meeting == nil || meeting.readOnly || w.creating {
		return
	}
	if !meeting.library.Owns(w.a.root) {
		w.err = locales.SessionsMeetingReadOnly()
		return
	}
	previousTarget := meeting.room.MentionTarget(w.draft)
	_, previousToken := meeting.room.MentionBinding(w.draft)
	group, err := meeting.room.RemoveParticipant(id)
	if err != nil {
		switch {
		case errors.Is(err, roundtable.ErrTurnActive):
			w.err = locales.RoundtableRemoveAfterTurn()
		case errors.Is(err, roundtable.ErrUnknownAgent):
			w.err = locales.RoundtableTargetUnavailable()
		default:
			w.err = err.Error()
		}
		return
	}
	w.applyMemberGroup(group, w.a.agentProviders())
	if w.target == "" {
		newlyMatched := previousTarget == "" && meeting.room.MentionTarget(w.draft) != ""
		if previousTarget == id || newlyMatched {
			w.target, w.mentions.selectedToken = id, previousToken
		}
	}
	if w.target == id {
		w.viewGeneration++
	}
}

func (w *roundtableWindow) applyMemberGroup(group roundtable.Group, providers []agentProvider) {
	meeting := w.meeting
	meeting.group = group
	meeting.speakers = nil
	meeting.room.SetMembers(w.a.meetingMembers(meeting, providers))
	w.inspector.inviting, w.err = false, ""
	w.a.readSessionsNow()
}

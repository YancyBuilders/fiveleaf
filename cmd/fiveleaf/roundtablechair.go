package main

import (
	"errors"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

var meetingHumanIcon = uiicons.UserRound()

func meetingChairLabel(human bool) string {
	if human {
		return locales.RoundtableHumanChair()
	}
	return locales.AgentRoleRoundtableModerator()
}

func (w *roundtableWindow) chairBadge(c *ui.Context, human bool) {
	name, hint, icon := locales.AgentYou(), locales.RoundtableHumanChairHint(), meetingHumanIcon
	if !human {
		speaker := w.chatSpeaker(roundtable.ModeratorID, locales.AgentRoleRoundtableModerator())
		name, hint, icon = speaker.Name, locales.RoundtableRoutingHint(), iconAgent
		if detail := speaker.detail(); detail != "" {
			hint += "\n" + detail
		}
	}
	label := locales.RoundtableHostedBy(name)
	pal := w.pal
	ui.Row(c.Key("chair-badge")).MinWidth(0).Height(17).Padding(0, 5).Gap(4).AlignItems(ui.Center).Radius(4).
		Background(pal.accent.Alpha(0.08).Over(pal.editor)).Label(label).Tooltip(label + "\n" + hint).Children(func() {
		ui.Icon(c, icon).FontSize(11).Shrink(0).TextColor(pal.accent)
		ui.Text(c, label).SingleLine().MinWidth(0).FontSize(10.5).TextColor(pal.text)
	})
}

func (w *roundtableWindow) chairPicker(c *ui.Context, human, disabled bool) (bool, bool) {
	chosen := human
	ui.Column(c.Key("chair-picker")).FillWidth().Gap(5).Children(func() {
		ui.Text(c, locales.RoundtableChair()).FontSize(12.5).FontWeight(600).TextColor(w.pal.text)
		ui.Row(c).FillWidth().Wrap().Gap(4).Children(func() {
			for _, manual := range []bool{false, true} {
				label := meetingChairLabel(manual)
				button := ui.ButtonBase(c.Key(label)).Label(label).Checked(human == manual).Disabled(disabled).
					Height(27).Padding(0, 9).Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				ink := w.pal.muted
				if manual == human {
					ink = w.pal.accent
					button.Background(ink.Alpha(0.12))
				} else if !disabled && (button.Hovered() || button.FocusVisible()) {
					button.Background(w.pal.hover)
				}
				button.Children(func() { ui.Text(c, label).SingleLine().FontSize(12).TextColor(ink) })
				if button.Clicked() {
					chosen = manual
				}
			}
		})
	})
	return chosen, chosen != human
}

func (w *roundtableWindow) chairSettings(c *ui.Context, snapshot roundtable.Snapshot) {
	human := snapshot.HumanChair
	if selected, changed := w.chairPicker(c, human, snapshot.Busy || w.meeting.readOnly || w.meeting.changing != nil); changed {
		if err := w.changeChair(selected); err != nil {
			switch {
			case errors.Is(err, roundtable.ErrModeratorUnavailable):
				w.err = locales.RoundtableModeratorUnavailable()
			case errors.Is(err, roundtable.ErrTurnActive):
				w.err = locales.RoundtableChairBusy()
			default:
				w.err = err.Error()
			}
		} else {
			human, w.err = selected, ""
		}
	}
	hint := locales.RoundtableRoutingHint()
	if human {
		hint = locales.RoundtableHumanChairHint()
	}
	if snapshot.Busy {
		hint = locales.RoundtableChairBusy()
	}
	ui.Text(c, hint).FontSize(12).TextColor(w.pal.muted)
}

func (w *roundtableWindow) changeChair(human bool) error {
	meeting := w.meeting
	if meeting.readOnly || !meeting.library.Owns(w.a.root) {
		return errors.New(locales.SessionsMeetingReadOnly())
	}
	if !human && meeting.group.HasModerator() {
		if meeting.room.Busy() {
			return roundtable.ErrTurnActive
		}
		w.a.syncMeetingConfig(meeting, w.a.agentProviders())
	}
	err := meeting.room.SetHumanChair(human)
	if err == nil {
		meeting.group.Options.HumanChair = human
		return nil
	}
	if !errors.Is(err, roundtable.ErrModeratorUnavailable) || meeting.group.HasModerator() {
		return err
	}
	moderator := w.a.meetingModerator()
	if moderator.Provider == "" || moderator.problem != "" {
		return roundtable.ErrModeratorUnavailable
	}
	if err := w.a.bindMeetingModerator(meeting, moderator.Participant, true, w.a.agentProviders()); err != nil {
		return err
	}
	w.a.readSessionsNow()
	return nil
}

func (w *roundtableWindow) postHumanConclusion(c *ui.Context, snapshot roundtable.Snapshot) {
	if !snapshot.HumanChair || w.target != "" || strings.HasPrefix(strings.TrimSpace(w.draft), "@") {
		return
	}
	button := ui.Button(c.Key("publish-conclusion"), locales.RoundtablePublishConclusion()).FontSize(11.5).KeepFocus().
		Disabled(snapshot.Busy || w.meeting.changing != nil || strings.TrimSpace(w.draft) == "")
	if button.Clicked() {
		draft := w.draft
		w.changeMeeting(meetingPublish, func(room *roundtable.Room) error { return room.PublishConclusionWithDeferredMemory(draft) }, func(meeting *agentMeeting, err error) {
			if w.closed {
				return
			}
			if w.meeting == meeting {
				if err != nil {
					if !w.creating {
						w.err = err.Error()
					}
				} else if w.draft == draft && w.target == "" {
					w.clearSentDraft()
				}
				return
			}
			key := meeting.library.Key() + "/" + meeting.group.ID
			if saved := w.drafts[key]; err == nil && saved.text == draft && saved.target == "" {
				delete(w.drafts, key)
			}
		})
	}
}

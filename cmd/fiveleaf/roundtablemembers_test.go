package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestMeetingRemoveMemberAndReinviteFromInspector(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.tt.SetSize(1180, 800)
	h.frames()
	meeting := h.w.meeting
	name, id := locales.AgentRoleTester(), "preset:test"
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	invite := func() {
		t.Helper()
		click(locales.RoundtableInvite())
		h.tt.Type(name)
		h.frames()
		click(locales.RoundtableAddMember(name))
	}
	invite()
	click(name)
	click(locales.RoundtableAskMember())
	if h.w.target != id || h.w.draft != "@"+name+" " {
		t.Fatalf("member card did not address the tester: %q", h.w.target)
	}
	index := slices.IndexFunc(meeting.speakers, func(speaker meetingSpeaker) bool { return speaker.ID == id })
	if index < 0 || meeting.speakers[index].adapter == nil {
		t.Fatal("the tester's invitation did not produce a runnable member")
	}
	meeting.speakers[index].adapter.backend = roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
		return roundtable.Message{Text: "Original test result"}, nil
	})
	h.tt.Type("Run the first test")
	click(locales.AgentSend())
	h.until(t, "the tester's recorded reply", func() bool { return !meeting.room.Busy() && len(meeting.room.Snapshot().Messages) == 2 })
	history := meeting.room.Snapshot().Messages
	if history[1].AgentID != id || history[1].Text != "Original test result" {
		t.Fatalf("expected the invited tester's reply: %+v", history)
	}
	h.mentionMember(t, name)
	const question = "A follow-up for the tester only"
	privateDraft := "@" + name + " " + question
	h.tt.Type(question)
	h.frames()
	remove := locales.RoundtableRemoveMember()
	if _, visible := h.tt.Find(remove); visible {
		t.Fatal("member removal is visible outside the context menu")
	}
	if err := h.tt.RightClick(locales.RoundtableTaskCount(1)); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(h.tt.Menu(), remove) || h.w.memberID != "" || !slices.ContainsFunc(meeting.group.Members, func(member roundtable.Participant) bool { return member.ID == id }) {
		t.Fatal("right-click did not open just the tester's menu", h.tt.Menu())
	}
	if err := h.tt.ChooseMenuItem(remove); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.memberID != "" || h.w.expandedTask != "" {
		t.Fatal("the remove action also opened the underlying member history")
	}
	saved, err := meeting.library.Group(meeting.group.ID)
	if err != nil || slices.ContainsFunc(saved.Members, func(member roundtable.Participant) bool { return member.ID == id }) {
		t.Fatalf("the persisted group still includes the tester: %v, %+v", err, saved.Members)
	}
	if _, exists := h.tt.Find(locales.RoundtableRemoveMember()); exists || !h.tt.HasText("Original test result") || !h.tt.HasText(locales.RoundtableTargetUnavailable()) {
		t.Fatal("removed card, past result or unavailable recipient rendered incorrectly", h.tt.Texts())
	}
	click(locales.AgentSend())
	click(locales.AgentDraft())
	h.tt.Key(0, ui.KeyEnter)
	h.frames()
	if h.w.target != id || h.w.draft != privateDraft || len(meeting.room.Snapshot().Messages) != len(history) {
		t.Fatal("sending a directed draft after removal lost the text or redirected the message")
	}
	invite()
	saved, err = meeting.library.Group(meeting.group.ID)
	if err != nil || !slices.ContainsFunc(saved.Members, func(member roundtable.Participant) bool { return member.ID == id }) || h.w.draft != privateDraft || h.w.target != id || h.tt.HasText(locales.RoundtableTargetUnavailable()) {
		t.Fatalf("reinvitation did not restore the original recipient: %v, %+v, %q", err, saved.Members, h.w.draft)
	}
	click(name)
	if h.w.memberID != id || h.w.expandedTask != history[1].ID || !h.tt.HasText("Original test result") {
		t.Fatal("the reinvited role lost its task history")
	}
}

func TestMeetingRemoveMemberPassesChairAndProtectsActiveTurnAndForeignGroup(t *testing.T) {
	h := newRoundtableHarness(t, roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
		<-ctx.Done()
		return roundtable.Message{}, ctx.Err()
	}))
	h.tt.SetSize(605, 700)
	h.frames()
	if err := h.tt.Click(locales.RoundtableMemberPanel()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	local := h.w.meeting
	moderator := locales.AgentRoleRoundtableModerator()
	if err := h.tt.RightClick(moderator); err != nil {
		t.Fatal(err)
	}
	remove := locales.RoundtableRemoveMember()
	if !slices.Contains(h.tt.Menu(), remove) {
		t.Fatal("the automatic moderator's menu did not offer removal")
	}
	if err := h.tt.ChooseMenuItem(remove); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if local.group.HasModerator() || !local.group.Options.HumanChair || !local.room.Snapshot().HumanChair || !h.tt.HasText(locales.RoundtableHostedBy(locales.AgentYou())) {
		t.Fatal("removing the moderator did not switch the meeting to human chair")
	}
	badge, ok := h.tt.Find(locales.RoundtableHostedBy(locales.AgentYou()))
	if !ok {
		t.Fatal("missing chair badge")
	}
	h.tt.Move(badge.X+badge.W/2, badge.Y+badge.H/2)
	h.tt.Advance(time.Second)
	if !h.tt.HasText(locales.RoundtableHumanChairHint()) {
		t.Fatal("the chair badge did not explain the active host on hover", h.tt.Texts())
	}
	before := slices.Clone(local.group.Members)
	if err := local.room.Discuss(local.group.Topic, "Wait", "agent:alpha"); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if err := h.tt.RightClick("Beta"); err != nil {
		t.Fatal(err)
	}
	if menu := h.tt.Menu(); !slices.Contains(menu, remove) || !slices.Contains(menu, locales.RoundtableRemoveAfterTurn()) {
		t.Fatal("busy menu did not explain when removal becomes available", menu)
	}
	if err := h.tt.ChooseMenuItem(remove); err == nil {
		t.Fatal("menu allowed removal during an active turn")
	}
	h.tt.CloseMenu()
	h.frames()
	if h.w.err != "" || h.w.memberID != "" {
		t.Fatal("disabled remove activated an action during a turn")
	}
	h.w.removeParticipant("agent:beta")
	h.frames()
	if !h.tt.HasText(locales.RoundtableRemoveAfterTurn()) || !local.room.Busy() {
		t.Fatal("a removal racing a turn did not explain the block in the narrow member list")
	}
	local.room.Stop()
	saved, err := local.library.Group(local.group.ID)
	if err != nil || !slices.Equal(saved.Members, before) {
		t.Fatalf("protected local roster changed: %v, %+v", err, saved.Members)
	}
	foreign, err := roundtable.ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := foreign.Create("Other project", before)
	if err != nil {
		t.Fatal(err)
	}
	meeting, err := h.a.startMeeting(foreign, group)
	if err != nil {
		t.Fatal(err)
	}
	h.w.enter(meeting)
	h.w.inspector.setVisible(true)
	h.frames()
	if err := h.tt.RightClick("Beta"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(h.tt.Menu(), remove) || !meeting.readOnly {
		t.Fatal("the foreign archive offered a removal action")
	}
	h.tt.CloseMenu()
	h.w.removeParticipant("agent:beta")
	saved, err = foreign.Group(group.ID)
	if err != nil || !slices.Equal(saved.Members, before) {
		t.Fatalf("read-only record changed: %v, %+v", err, saved.Members)
	}
}

func TestMeetingRemoveMemberKeepsLeadingMentionFromMatchingShorterName(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.a.settings.AgentRoles = map[string]settings.AgentRole{"Alpha Team": {Agent: "alpha"}}
	role := roundtable.Participant{ID: "role:Alpha Team", Name: "Alpha Team", Provider: "alpha"}
	h.w.addParticipant(role)
	h.tt.SetSize(1180, 800)
	h.frames()
	if err := h.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	const draft = "@Alpha Team private review"
	h.tt.Type(draft)
	h.frames()
	if h.w.target != "" || h.w.meeting.room.MentionTarget(draft) != role.ID {
		t.Fatal("expected a typed mention of the longer role name", h.w.err)
	}
	if err := h.tt.RightClick(role.Name); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem(locales.RoundtableRemoveMember()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if err := h.tt.Click(locales.AgentSend()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.meeting.room.MentionTarget(draft) != "agent:alpha" || h.w.target != role.ID || h.w.draft != draft || !h.tt.HasText(locales.RoundtableTargetUnavailable()) || len(h.w.meeting.room.Snapshot().Messages) != 0 {
		t.Fatal("removing the longer name redirected a typed mention to Alpha")
	}
}

func TestMeetingRemoveMemberCancelsQueuedSendToMatchingAlias(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	const target = "agent:beta"
	h.a.settings.AgentRoles = map[string]settings.AgentRole{target: {Agent: "alpha"}}
	other := roundtable.Participant{ID: "role:" + target, Name: target, Provider: "alpha"}
	h.w.addParticipant(other)
	h.w.target, h.w.draft = target, "Only send this to Beta"
	h.w.post(func() { h.w.removeParticipant(target) })
	h.w.post(h.w.send)
	h.frames()
	if h.w.meeting.room.MentionTarget("@"+target+" hello") != other.ID || h.w.target != target || h.w.draft != "Only send this to Beta" || len(h.w.meeting.room.Snapshot().Messages) != 0 || !h.tt.HasText(locales.RoundtableTargetUnavailable()) {
		t.Fatal("a pending send for the removed ID disclosed the draft to a member with that display name")
	}
}

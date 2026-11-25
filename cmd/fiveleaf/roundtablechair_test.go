package main

import (
	"bufio"
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
)

func TestMeetingChairControlsRemarksAssignmentAndPublication(t *testing.T) {
	var calls atomic.Int32
	h := newRoundtableHarness(t, roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
		calls.Add(1)
		return roundtable.Message{Text: "@agent:beta check next\nOne response."}, nil
	}))
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	write := func(text string) {
		t.Helper()
		click(locales.AgentDraft())
		h.tt.Type(text)
		h.frames()
	}
	click(locales.RoundtableCollaboration())
	click(locales.RoundtableHumanChair())
	if !h.w.meeting.room.Snapshot().HumanChair || !h.tt.HasText(locales.RoundtableHumanChairHint()) {
		t.Fatal("collaboration drawer did not apply host change", h.tt.Texts())
	}
	click(locales.CloseEsc())
	write("Agenda for the group")
	click(locales.AgentSend())
	if calls.Load() != 0 || !h.tt.HasText("Agenda for the group") {
		t.Fatal("ordinary remark triggered an agent", calls.Load())
	}
	h.mentionMember(t, "Alpha")
	h.tt.Type("Give one finding")
	click(locales.AgentSend())
	h.until(t, "a single invited reply", func() bool { return !h.w.meeting.room.Busy() })
	if calls.Load() != 1 || !h.tt.HasText("One response.") || h.w.target != "" {
		t.Fatal("manual assignment did not stop after the selected response", h.tt.Texts())
	}
	write("Use the submitted value")
	click(locales.RoundtablePublishConclusion())
	h.until(t, "the saved conclusion", func() bool { return h.w.meeting.changing == nil })
	got := h.w.meeting.room.Snapshot()
	message := got.Messages[len(got.Messages)-1]
	if !message.HumanConclusion || message.Role != "user" || !h.tt.HasText(locales.RoundtableConclusion()) || h.w.draft != "" || calls.Load() != 1 {
		t.Fatal(message, calls.Load())
	}
	var text strings.Builder
	out := bufio.NewWriter(&text)
	if err := writeMeetingMessage(out, message, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := out.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## " + locales.RoundtableHuman(), locales.RoundtableConclusion(), message.Text} {
		if !strings.Contains(text.String(), want) {
			t.Fatal(text.String())
		}
	}
	click(locales.RoundtableCollaboration())
	click(locales.AgentRoleRoundtableModerator())
	click(locales.CloseEsc())
	if h.w.meeting.room.Snapshot().HumanChair || h.tt.HasText(locales.RoundtablePublishConclusion()) {
		t.Fatal("switching back did not restore moderated discussion")
	}
}

func TestMeetingChairCreationStartsHumanAndDefaultsEachNewForm(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	h := newRoundtableHarness(t, nil)
	h.w.newGroup()
	h.frames()
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	click(locales.RoundtableHumanChair())
	click(locales.RoundtableTopic())
	h.tt.Type("Human-led review")
	h.frames()
	click(locales.RoundtableStart())
	group := h.w.meeting.group
	if h.w.creating || !group.Options.HumanChair || group.Topic != "Human-led review" {
		t.Fatal("wrong chair at first turn", h.tt.Texts())
	}
	snapshot := h.w.meeting.room.Snapshot()
	if snapshot.Busy || !snapshot.HumanChair || len(snapshot.Messages) != 1 || snapshot.Messages[0].Text != group.Topic {
		t.Fatal(snapshot)
	}
	h.w.newGroup()
	h.frames()
	if h.w.createHumanChair {
		t.Fatal("new form inherited the last meeting's chair")
	}
	click(locales.ButtonCancel())
	if h.w.creating || !h.tt.HasText(locales.RoundtablePublishConclusion()) {
		t.Fatal("cancel did not restore the original chair and meeting")
	}
}

func TestMeetingChairWithoutAvailableBindingAndReadOnlyHistory(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	for index := range h.w.meeting.speakers {
		h.w.meeting.speakers[index].problem = "unavailable"
	}
	h.w.meeting.room.SetMembers(nil)
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	click(locales.RoundtableCollaboration())
	click(locales.RoundtableHumanChair())
	click(locales.CloseEsc())
	click(locales.AgentDraft())
	h.tt.Type("Unassisted minutes")
	h.frames()
	click(locales.AgentSend())
	if m := h.w.meeting.room.Snapshot(); !m.HumanChair || len(m.Messages) != 1 || m.Busy {
		t.Fatal(m)
	}
	h.w.meeting.readOnly = true
	h.frames()
	click(locales.RoundtableCollaboration())
	click(locales.AgentRoleRoundtableModerator())
	click(locales.CloseEsc())
	if !h.w.meeting.room.Snapshot().HumanChair || h.tt.HasText(locales.RoundtablePublishConclusion()) || h.tt.HasText(locales.AgentDraft()) {
		t.Fatal("read-only archive exposed editing controls")
	}
}

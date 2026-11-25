package main

import (
	"slices"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func clickMeetingCreation(t *testing.T, h *roundtableHarness, label string) {
	t.Helper()
	if err := h.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	h.frames()
}

func chooseMeetingTemplate(t *testing.T, h *roundtableHarness, label string) {
	t.Helper()
	clickMeetingCreation(t, h, locales.RoundtableTemplate())
	if err := h.tt.ChooseMenuItem(label); err != nil {
		t.Fatal(err)
	}
	h.frames()
}

func TestRoundtableTemplatesPrefillAndCreate(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	h := newRoundtableHarness(t, nil)
	h.a.settings.AgentServers["review-agent"] = roundtableTestServer(t, "Review Agent", "acp")
	h.a.settings.AgentRolePresets = map[string]settings.AgentRoleOverride{
		"review": {Agent: "review-agent", Model: "model-review"},
	}
	h.w.newGroup()
	h.frames()
	clickMeetingCreation(t, h, locales.RoundtableHumanChair())
	clickMeetingCreation(t, h, locales.RoundtableTopic())
	h.tt.Type("Check the checkout redesign")
	h.frames()

	chooseMeetingTemplate(t, h, locales.RoundtableTemplateDesignReview())
	if h.w.topic != "Check the checkout redesign" || !h.w.createHumanChair || h.w.brief != locales.RoundtableTemplateDesignReviewBrief() {
		t.Fatalf("template changed the meeting topic or chair, or omitted the brief: %q, %t, %q", h.w.topic, h.w.createHumanChair, h.w.brief)
	}
	if h.w.selected["preset:review"].Provider != "review-agent" || h.w.selected["preset:requirements"].ID == "" || h.w.selected["preset:plan"].ID == "" || len(h.w.selected) != 3 {
		t.Fatalf("design review did not select the configured roles: %+v", h.w.selected)
	}
	chooseMeetingTemplate(t, h, locales.RoundtableTemplateDiagnosis())
	if h.w.brief != locales.RoundtableTemplateDiagnosisBrief() || h.w.selected["preset:review"].ID != "" || h.w.selected["preset:diagnose"].ID == "" {
		t.Fatal("switching templates did not replace the untouched brief and members")
	}

	clickMeetingCreation(t, h, locales.RoundtableBrief())
	h.tt.Key(ui.Cmd, ui.KeyA)
	h.tt.Type("Check the keyboard flow before accepting.")
	h.frames()
	chooseMeetingTemplate(t, h, locales.RoundtableTemplateVerification())
	if h.w.brief != "Check the keyboard flow before accepting." {
		t.Fatalf("template replaced the edited brief: %q", h.w.brief)
	}
	clickMeetingCreation(t, h, locales.AgentRoleImplementer())
	clickMeetingCreation(t, h, locales.RoundtableCreate())
	meeting := h.w.meeting
	if h.w.creating || meeting.group.Topic != "Check the checkout redesign" || !meeting.group.Options.HumanChair || h.w.draft != "Check the keyboard flow before accepting." {
		t.Fatal("creating the meeting did not preserve the form choices and draft")
	}
	if len(meeting.group.Members) != 2 || meeting.group.Members[0].ID != "preset:test" || meeting.group.Members[1].ID != "preset:review" {
		t.Fatalf("created roster did not reflect manual invite edits: %+v", meeting.group.Members)
	}
	if reviewer := meeting.speakers[1]; reviewer.Provider != "review-agent" || reviewer.role.Model != "model-review" {
		t.Fatalf("template changed the Reviewer's Agent or model: %+v", reviewer)
	}
	if snapshot := meeting.room.Snapshot(); len(snapshot.Messages) != 0 || snapshot.Busy {
		t.Fatal("creating from a template sent the brief")
	}
}

func TestRoundtableTemplateKeepsUnavailableRoleVisible(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	h := newRoundtableHarness(t, nil)
	h.a.settings.AgentRolePresets = map[string]settings.AgentRoleOverride{"diagnose": {Agent: "missing-provider"}}
	h.w.newGroup()
	h.frames()
	clickMeetingCreation(t, h, locales.RoundtableTopic())
	h.tt.Type("Investigate an intermittent failure")
	h.frames()
	chooseMeetingTemplate(t, h, locales.RoundtableTemplateDiagnosis())
	if h.w.selected["preset:diagnose"].ID == "" || !h.tt.HasText(locales.RoundtableInvitationsUnavailable()) {
		t.Fatal("the template hid the unavailable invitation", h.tt.Texts())
	}
	clickMeetingCreation(t, h, locales.RoundtableCreate())
	if !h.w.creating {
		t.Fatal("the meeting was created with an unavailable invitation")
	}
	clickMeetingCreation(t, h, locales.AgentRoleDiagnostician())
	clickMeetingCreation(t, h, locales.RoundtableCreate())
	if h.w.creating || !h.w.meeting.group.HasModerator() || slices.ContainsFunc(h.w.meeting.group.Members, func(member roundtable.Participant) bool { return member.ID == "preset:diagnose" }) {
		t.Fatal("the form could not create the meeting after removing the unavailable role")
	}
}

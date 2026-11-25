package main

import (
	"slices"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingTemplate struct {
	label, brief func() string
	members      []string
}

var meetingTemplates = [...]meetingTemplate{
	{locales.RoundtableTemplateDesignReview, locales.RoundtableTemplateDesignReviewBrief, []string{"preset:requirements", "preset:plan", "preset:review"}},
	{locales.RoundtableTemplateDiagnosis, locales.RoundtableTemplateDiagnosisBrief, []string{"preset:explore", "preset:diagnose", "preset:test"}},
	{locales.RoundtableTemplateVerification, locales.RoundtableTemplateVerificationBrief, []string{"preset:implement", "preset:test", "preset:review"}},
}

func (w *roundtableWindow) templatePicker(c *ui.Context, choices []meetingSpeaker) {
	actionButton(c.Key("meeting-template"), locales.RoundtableTemplate(), iconClipboardList).Shrink(0).Menu(func(menu *ui.Menu) {
		for _, template := range meetingTemplates {
			if menu.Item(template.label()).Chosen() {
				w.applyMeetingTemplate(template, choices)
			}
		}
	})
}

func (w *roundtableWindow) applyMeetingTemplate(template meetingTemplate, choices []meetingSpeaker) {
	selected := make(map[string]roundtable.Participant, len(template.members))
	for _, id := range template.members {
		index := slices.IndexFunc(choices, func(choice meetingSpeaker) bool { return choice.ID == id })
		if index == -1 {
			w.err = locales.AgentRoleUnavailable()
			return
		}
		selected[id] = choices[index].Participant
	}
	w.selected, w.err = selected, ""
	if w.brief == w.templateBrief {
		w.templateBrief = template.brief()
		w.brief = w.templateBrief
	}
}

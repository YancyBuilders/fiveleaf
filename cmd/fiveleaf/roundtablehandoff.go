package main

import (
	"slices"
	"strings"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (w *roundtableWindow) conclusionContinueButton(c *ui.Context, conclusion roundtable.Message, before []roundtable.Message, graph roundtable.Graph, pending *bool) {
	label := locales.RoundtableContinueWith()
	if *pending {
		label = locales.Loading()
	}
	actionButton(c.Key("continue-conclusion"), label, iconAgent).AlignSelf(ui.Start).Disabled(*pending).Menu(func(menu *ui.Menu) {
		choices := w.a.meetingInvitations()
		choose := func(menu *ui.Menu, speaker meetingSpeaker) {
			if menu.Item(speaker.Name).Disabled(speaker.problem != "").Chosen() {
				w.post(func() { w.continueConclusion(speaker.Participant, conclusion, before, graph, pending) })
			}
		}
		for _, speaker := range choices {
			if strings.HasPrefix(speaker.ID, "agent:") {
				choose(menu, speaker)
			}
		}
		menu.Submenu(locales.AgentRoles(), func(roles *ui.Menu) {
			for _, speaker := range choices {
				if !strings.HasPrefix(speaker.ID, "agent:") {
					choose(roles, speaker)
				}
			}
		})
		menu.Separator()
		if menu.Item(locales.AgentConfigure()).Chosen() {
			w.post(func() { w.a.host.showSettings(settings.CatAgent, "agentServers") })
		}
	})
}

func conclusionGoal(conclusion roundtable.Message, before []roundtable.Message) (goal roundtable.Message, foundBoundary bool) {
	turnID := ""
	if conclusion.Task != nil {
		turnID = conclusion.Task.TurnID
	}
	for _, message := range slices.Backward(before) {
		if meetingFlowConclusion(message) {
			return roundtable.Message{}, true
		}
		if message.Role == "user" && (turnID == "" || message.ID == turnID) {
			return message, true
		}
	}
	return roundtable.Message{}, false
}

func (w *roundtableWindow) continueConclusion(target roundtable.Participant, conclusion roundtable.Message, before []roundtable.Message, graph roundtable.Graph, pending *bool) {
	a, meeting := w.a, w.meeting
	if a.activeRoundtableTab() != w || meeting == nil || meeting.readOnly || *pending {
		return
	}
	if !meeting.library.Owns(a.root) {
		a.fail("%s", locales.SessionsMeetingReadOnly())
		return
	}
	if goal, found := conclusionGoal(conclusion, before); found {
		refs := conclusionEvidenceRefs(conclusionHandoffEvidence(conclusion, goal, graph))
		a.openConclusionChat(meeting, target, conclusion, goal.Text, refs, conclusion.Task != nil && goal.Text == "")
		return
	}
	*pending = true
	root, tree, generation := a.root, a.tree, w.viewGeneration
	go func() {
		previous, more, err := meeting.room.Earlier(conclusion, 2*roundtable.MaxTasksPerTurn+1)
		goal, found := conclusionGoal(conclusion, previous)
		if goal.ID != "" {
			graph = roundtable.BuildTaskGraph(roundtable.Snapshot{Messages: append(previous, conclusion)})
		}
		refs := conclusionEvidenceRefs(conclusionHandoffEvidence(conclusion, goal, graph))
		partial := goal.Text == "" && (err != nil || conclusion.Task != nil || !found && more)
		w.postWindow(func() {
			*pending = false
			if a.root != root || a.tree != tree || w.viewGeneration != generation || a.activeRoundtableTab() != w {
				return
			}
			if !meeting.library.Owns(a.root) {
				a.fail("%s", locales.SessionsMeetingReadOnly())
				return
			}
			a.openConclusionChat(meeting, target, conclusion, goal.Text, refs, partial)
		})
	}()
}

func conclusionHandoffEvidence(conclusion, goal roundtable.Message, graph roundtable.Graph) []roundtable.ToolRecord {
	if !conclusion.HumanConclusion {
		return meetingFlowEvidence(conclusion, graph)
	}
	if goal.ID == "" {
		return nil
	}
	var tools []roundtable.ToolRecord
	for _, node := range graph.Nodes {
		if node.TurnID == goal.ID && node.Start.Before(conclusion.Time) && node.Stage == roundtable.Direct && node.Status == roundtable.Completed {
			tools = append(tools, node.Tools...)
		}
	}
	return tools
}

func conclusionEvidenceRefs(evidence []roundtable.ToolRecord) []string {
	const limit = 6
	var refs []string
	for _, tool := range evidence {
		if meetingEvidenceFinding(tool) == "" {
			continue
		}
		for _, text := range []string{tool.Detail, tool.Note} {
			text, _ = agentVisibleText(text, 16<<10)
			for _, match := range meetingEvidenceLocation.FindAllStringSubmatch(text, limit) {
				if ref := match[1]; len(ref) <= 512 && !slices.Contains(refs, ref) {
					refs = append(refs, ref)
					if len(refs) == limit {
						return refs
					}
				}
			}
		}
	}
	return refs
}

func conclusionHistory(conclusion, goal string, refs []string, partial bool) agents.Handoff {
	goal, clippedGoal := agentVisibleText(strings.TrimSpace(goal), 8<<10)
	conclusion, clippedConclusion := agentVisibleText(strings.TrimSpace(conclusion), 24<<10)
	history := agents.Handoff{Truncated: partial || clippedGoal || clippedConclusion}
	if goal != "" {
		history.Messages = append(history.Messages, agents.HandoffMessage{Role: "user", Text: goal})
	}
	if len(refs) != 0 {
		conclusion += "\n\n" + locales.RoundtableEvidenceSources() + "\n\n- `" + strings.Join(refs, "`\n- `") + "`"
	}
	history.Messages = append(history.Messages, agents.HandoffMessage{Role: "assistant", Text: conclusion})
	return history
}

func (a *app) openConclusionChat(meeting *agentMeeting, ref roundtable.Participant, conclusion roundtable.Message, goal string, evidence []string, partial bool) {
	speaker := a.resolveSpeaker(ref, a.agentProviders())
	if speaker.problem != "" {
		a.fail("%s", speaker.problem)
		return
	}
	target := agentTarget{provider: ref.Provider}
	kind, name, _ := strings.Cut(ref.ID, ":")
	switch kind {
	case "role":
		target.role = name
	case "preset":
		index := slices.IndexFunc(agentRolePresets[:], func(preset agentRolePreset) bool { return preset.id == name })
		if index < 0 {
			return
		}
		target.preset = &agentRolePresets[index]
	}
	chat := a.blankChat()
	chat.agentTarget = target
	if target.preset != nil {
		chat.presetDefaultPrompt = target.preset.prompt()
	}
	chat.systemPrompt, chat.draft = speaker.role.Prompt, locales.RoundtableContinueDraft()
	chat.continuation = &sessionContinuation{source: agents.Session{
		Harness: meetingHistorySource,
		Root:    meeting.library.Key(),
		ID:      meeting.group.ID + "/" + conclusion.ID,
		Title:   meeting.group.Topic,
		Dir:     meeting.library.Project,
	}}
	chat.importContinuation(conclusionHistory(conclusion.Text, goal, evidence, partial))
	a.rememberChat(a.agent)
	a.openChatPage(chat)
}

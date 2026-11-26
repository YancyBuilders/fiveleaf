package main

import (
	"context"
	"slices"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
)

func (a *app) refreshAgentModels(s *agentChat) {
	if s.client == nil || s.sessionID == "" {
		return
	}
	state, ok := s.client.ModelState(s.sessionID)
	a.refreshAgentModes(s)
	a.refreshAgentUsage(s)
	if !ok {
		s.currentModel, s.models = "", nil
		return
	}
	s.currentModel, s.models = state.Current, state.Available
}

func (a *app) refreshAgentModes(s *agentChat) {
	if s.client == nil || s.sessionID == "" {
		return
	}
	state, ok := s.client.ModeState(s.sessionID)
	s.mode, s.modes, s.thinkingModes = "", nil, false
	s.genericMode, s.genericModes = "", nil
	if !ok {
		return
	}
	s.mode, s.thinkingModes = state.Current, state.Thinking
	for _, choice := range state.Available {
		s.modes = append(s.modes, agentChoice{id: choice.ID, name: choice.Name})
	}
	s.genericMode = state.GenericCurrent
	for _, choice := range state.GenericAvailable {
		s.genericModes = append(s.genericModes, agentChoice{id: choice.ID, name: choice.Name})
	}
}

func (a *app) setAgentModel(id string) {
	a.setChatModel(a.agent, id)
}

func (a *app) setChatModel(s *agentChat, id string) {
	a.setChatModelSelection(s, id, nil)
}

func (a *app) setChatModelSelection(s *agentChat, id string, applied func()) {
	if s.client == nil || s.sessionID == "" || s.requestInFlight() {
		return
	}
	if !slices.ContainsFunc(s.models, func(choice acp.ModelChoice) bool { return choice.ID == id }) {
		s.err = locales.AgentModelUnavailable()
		return
	}
	s.configuring = true
	client, session, ctx := s.client, s.sessionID, s.ctx
	h := a.chatHandler(s)
	go func() {
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := client.SetModel(call, session, id)
		h.post(func() {
			if s.client != client || s.sessionID != session {
				return
			}
			s.configuring = false
			h.workspace().refreshAgentModels(s)
			if err != nil {
				s.err = acp.ErrorMessage(err)
				return
			}
			s.modelOverride, s.err = id, ""
			s.recentModels = append([]string{id}, slices.DeleteFunc(s.recentModels, func(recent string) bool { return recent == id })...)
			s.recentModels = s.recentModels[:min(len(s.recentModels), 8)]
			if applied != nil {
				applied()
			}
		})
	}()
}

func agentModelLabel(choices []acp.ModelChoice, choice acp.ModelChoice) string {
	if choice.Name == "" {
		return choice.ID
	}
	for _, other := range choices {
		if other.ID != choice.ID && other.Name == choice.Name {
			return choice.Name + " · " + choice.ID
		}
	}
	return choice.Name
}

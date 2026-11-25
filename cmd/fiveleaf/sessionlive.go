package main

import (
	"maps"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/plugins/terminal"

	"fiveleaf/internal/agents"
)

type agentTerminalKey struct {
	term    *session
	harness agents.Harness
}

type liveAgent struct {
	term    *session
	owner   *app
	cwd     string
	harness agents.Harness
	state   terminal.StatusState
	since   time.Time
}

func liveKey(harness agents.Harness, id string) string { return string(harness) + "\x00" + id }

func (agent liveAgent) record() terminal.StatusRecord {
	if agent.term != nil {
		if record, ok := agent.term.status.Top(); ok {
			return record
		}
	}
	return terminal.StatusRecord{State: agent.state, App: agent.harness.App(), Progress: -1}
}

func placeLoose(running map[string]liveAgent, loose []liveAgent, sessions []agents.Session) map[string]liveAgent {
	matched := make(map[string]liveAgent, len(running)+len(loose))
	maps.Copy(matched, running)
	for _, agent := range loose {
		if agent.cwd == "" {
			continue
		}
		dir := filepath.Clean(agent.cwd)
		var best *agents.Session
		for index := range sessions {
			candidate := &sessions[index]
			if candidate.Harness != agent.harness || candidate.Dir == "" || filepath.Clean(candidate.Dir) != dir {
				continue
			}
			if _, taken := matched[liveKey(candidate.Harness, candidate.ID)]; taken || candidate.Updated.Before(agent.since) {
				continue
			}
			if best == nil || candidate.Updated.After(best.Updated) {
				best = candidate
			}
		}
		if best != nil {
			matched[liveKey(best.Harness, best.ID)] = agent
		}
	}
	return matched
}

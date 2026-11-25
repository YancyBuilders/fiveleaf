package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingSpeaker struct {
	roundtable.Participant
	provider agentProvider
	role     settings.AgentRole
	icon     *ui.SVG
	problem  string
	adapter  *speakerAdapter
}

func (a *app) meetingInvitations() []meetingSpeaker {
	providers := a.agentProviders()
	var choices []meetingSpeaker
	add := func(id, name, provider string) {
		choices = append(choices, a.resolveSpeaker(roundtable.Participant{ID: id, Name: name, Provider: provider}, providers))
	}
	for _, name := range a.agentRoleNames() {
		role := a.settings.AgentRoles[name]
		provider := role.Agent
		if role.FollowCurrentAgent {
			provider = agentPresetProvider(providers, "", role.Model).id
		}
		add("role:"+name, name, provider)
	}
	for _, preset := range agentRolePresets {
		binding := a.settings.AgentRolePresets[preset.id]
		provider := agentPresetProvider(providers, binding.Agent, binding.Model)
		add("preset:"+preset.id, preset.name(), provider.id)
	}
	for _, provider := range providers {
		add("agent:"+provider.id, provider.name, provider.id)
	}
	return choices
}

func (a *app) resolveSpeaker(ref roundtable.Participant, providers []agentProvider) meetingSpeaker {
	speaker := meetingSpeaker{Participant: ref, provider: findChatProvider(ref.Provider, providers), icon: iconComment}
	kind, name, _ := strings.Cut(ref.ID, ":")
	var err error
	switch kind {
	case "role":
		role, found := a.settings.AgentRoles[name]
		role = role.ForAgent(ref.Provider)
		if !found || role.Agent != ref.Provider {
			err = errors.New(locales.AgentRoleUnavailable())
		} else {
			speaker.role, err = role, role.Validate()
		}
	case "preset":
		index := slices.IndexFunc(agentRolePresets[:], func(preset agentRolePreset) bool { return preset.id == name })
		binding := a.settings.AgentRolePresets[name]
		if index < 0 || binding.Agent != "" && binding.Agent != ref.Provider {
			err = errors.New(locales.AgentRoleUnavailable())
		} else {
			preset := agentRolePresets[index]
			speaker.icon = preset.icon
			speaker.role = binding.Resolve(ref.Provider, preset.prompt())
			err = binding.Validate()
		}
	case "agent":
		if name != ref.Provider {
			err = errors.New(locales.AgentRoleUnavailable())
		}
	default:
		err = errors.New(locales.AgentRoleUnavailable())
	}
	if err == nil {
		switch {
		case speaker.provider.config.Command == "":
			err = errors.New(locales.AgentRoleUnavailable())
		case speaker.provider.config.Disabled:
			err = errors.New(locales.AgentDisabled())
		default:
			err = speaker.provider.config.Validate()
		}
	}
	if err == nil && speaker.provider.config.EffectiveProtocol() == "cli" {
		speaker.provider, err = agentRoleCLIProvider(speaker.provider, speaker.role.Model)
	}
	if err != nil {
		speaker.problem = err.Error()
	}
	return speaker
}

type speakerAdapter struct {
	backend        roundtable.Adapter
	header, roster string
}

func (speaker *speakerAdapter) Call(ctx context.Context, messages []roundtable.Message) (roundtable.Message, error) {
	header := speaker.header
	position := 0
	if roundtable.TaskID(ctx) != "" && len(messages) > 0 && messages[0].Role == "system" {
		position = 1
	} else {
		header += "\nInvited members: " + speaker.roster
	}
	return speaker.backend.Call(ctx, slices.Insert(slices.Clone(messages), position, roundtable.Message{Role: "system", Text: header}))
}

func (a *app) meetingMembers(meeting *agentMeeting, providers []agentProvider) []roundtable.Member {
	meeting.providers = cloneAgentProviders(providers)
	meeting.roles = maps.Clone(a.settings.AgentRoles)
	meeting.presets = maps.Clone(a.settings.AgentRolePresets)
	old := meeting.speakers
	meeting.speakers = make([]meetingSpeaker, 0, len(meeting.group.Members))
	var roster []string
	for _, ref := range meeting.group.Members {
		roster = append(roster, fmt.Sprintf("%q (@%s)", ref.Name, ref.ID))
	}
	group := strings.Join(roster, ", ")
	var members []roundtable.Member
	for index, ref := range meeting.group.Members {
		speaker := a.resolveSpeaker(ref, meeting.providers)
		if speaker.problem == "" {
			if index < len(old) && old[index].adapter != nil && old[index].Participant == ref && old[index].role == speaker.role && reflect.DeepEqual(old[index].provider, speaker.provider) {
				speaker.adapter = old[index].adapter
			} else {
				header := fmt.Sprintf("Roundtable topic: %q\nYour role: %q (@%s). Follow the current assignment.\n%s", meeting.group.Topic, ref.Name, ref.ID, speaker.role.Prompt)
				speaker.adapter = &speakerAdapter{backend: a.adapterFor(speaker.provider, meeting, speaker.role.Model), header: header, roster: group}
			}
		}
		meeting.speakers = append(meeting.speakers, speaker)
		members = append(members, roundtable.Member{ID: ref.ID, Name: ref.Name, Capabilities: slices.Clone(speaker.provider.config.Capabilities), Enabled: speaker.problem == "", Adapter: speaker.adapter})
	}
	return members
}

func (speaker meetingSpeaker) detail() string {
	if speaker.problem != "" {
		return speaker.problem
	}
	return meetingSpeakerModel(speaker)
}

func (m *agentMeeting) memberLabel(speaker meetingSpeaker) string {
	for _, other := range m.speakers {
		if other.ID != speaker.ID && strings.EqualFold(other.Name, speaker.Name) {
			return speaker.Name + " · @" + speaker.ID
		}
	}
	return speaker.Name
}

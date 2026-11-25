package main

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func chairCreationForm(t *testing.T) *roundtableHarness {
	t.Helper()
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	harness := newHarness(t, true)
	harness.a.settings.AgentServers = map[string]settings.AgentServer{
		"local": {Name: "Local", Command: "test-cli", Protocol: "cli"},
	}
	harness.a.settings.AgentRoles = map[string]settings.AgentRole{"Observer": {Agent: "local"}}
	window := newRoundtableWindow(harness.a, nil)
	harness.a.meetingWindow = window
	t.Cleanup(window.finish)
	view := &roundtableHarness{harness: harness, w: window, tt: ui.NewTester(meetingTestView(window), 880, 680)}
	view.frames()
	return view
}

func chairCreationClick(t *testing.T, view *roundtableHarness, label string) {
	t.Helper()
	if err := view.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	view.frames()
}

func checkChairRoster(t *testing.T, meeting *agentMeeting, human bool, want ...roundtable.Participant) {
	t.Helper()
	if meeting == nil {
		t.Fatal("meeting did not open")
	}
	saved, err := meeting.library.Group(meeting.group.ID)
	if err != nil {
		t.Fatal(err)
	}
	var speakers []roundtable.Participant
	for _, speaker := range meeting.speakers {
		speakers = append(speakers, speaker.Participant)
	}
	if saved.Options.HumanChair != human || meeting.room.Snapshot().HumanChair != human || meeting.group.Options.HumanChair != human ||
		!slices.Equal(saved.Members, want) || !slices.Equal(meeting.group.Members, want) || !slices.Equal(speakers, want) {
		t.Fatalf("want human=%t, members=%v; saved=%+v, speakers=%v", human, want, saved, speakers)
	}
}

func TestChairCreationToggleKeepsInvitations(t *testing.T) {
	view := chairCreationForm(t)
	check := func(count int, autoJoined bool) {
		t.Helper()
		if !view.tt.HasText(locales.RoundtableSelection(count)) || view.tt.HasText(locales.RoundtableAutoJoined()) != autoJoined {
			t.Fatalf("want %d invitations, auto-joined=%t; form: %v", count, autoJoined, view.tt.Texts())
		}
	}
	check(1, true)
	chairCreationClick(t, view, "Observer")
	check(2, true)
	chairCreationClick(t, view, locales.RoundtableHumanChair())
	check(1, false)
	chairCreationClick(t, view, locales.AgentRoleRoundtableModerator())
	check(2, true)
	chairCreationClick(t, view, locales.RoundtableHumanChair())
	view.w.topic = "Observed review"
	view.frames()
	chairCreationClick(t, view, locales.RoundtableCreate())
	checkChairRoster(t, view.w.meeting, true, roundtable.Participant{ID: "role:Observer", Name: "Observer", Provider: "local"})
}

func TestChairCreationHumanOnlyReopensAndRefreshes(t *testing.T) {
	view := chairCreationForm(t)
	chairCreationClick(t, view, locales.RoundtableHumanChair())
	view.w.topic = "My minutes"
	view.frames()
	chairCreationClick(t, view, locales.RoundtableStart())
	meeting := view.w.meeting
	checkChairRoster(t, meeting, true)
	before := meeting.room.Snapshot().Messages
	if len(before) != 1 || before[0].Text != meeting.group.Topic {
		t.Fatalf("unexpected opening remark: %+v", before)
	}
	view.w.finish()
	window := view.a.settings.NewWindow()
	window.Folder = view.root
	restarted := newApp(&fakeHost{folder: view.root}, view.a.settings, window)
	restarted.root = view.root
	t.Cleanup(restarted.shutdown)
	library, err := restarted.roundtableLibrary()
	if err != nil {
		t.Fatal(err)
	}
	restarted.openMeetingRecord(library, meeting.group.ID)
	meeting = restarted.meeting
	checkChairRoster(t, meeting, true)
	restarted.settings.AgentRolePresets = map[string]settings.AgentRoleOverride{"moderate": {Agent: "local"}}
	restarted.checkMeetingConfig(restarted.agentProviders())
	checkChairRoster(t, meeting, true)
	after := meeting.room.Snapshot().Messages
	if len(after) != 1 || after[0].ID != before[0].ID || after[0].Text != before[0].Text || meeting.room.MentionTarget("@preset:moderate hello") != "" {
		t.Fatalf("reopen or refresh changed the history or routing: %+v", after)
	}
}

func TestChairCreationCollaborationBindsModeratorOnHandoff(t *testing.T) {
	view := chairCreationForm(t)
	worker := roundtable.Participant{ID: "role:Observer", Name: "Observer", Provider: "local"}
	host := roundtable.Participant{ID: roundtable.ModeratorID, Name: locales.AgentRoleRoundtableModerator(), Provider: "chair"}
	view.a.settings.AgentServers["chair"] = settings.AgentServer{Name: "Chair backend", Command: "chair-cli", Protocol: "cli", Disabled: true}
	view.a.settings.AgentRolePresets = map[string]settings.AgentRoleOverride{"moderate": {Agent: host.Provider}}
	library, err := view.a.roundtableLibrary()
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.CreateWithChair("Review", []roundtable.Participant{worker}, true)
	if err != nil {
		t.Fatal(err)
	}
	view.a.openMeetingRecord(library, group.ID)
	view.frames()
	meeting := view.w.meeting
	if err := meeting.room.PublishConclusion("Keep the human decision"); err != nil {
		t.Fatal(err)
	}
	decision := meeting.room.Snapshot().Messages[0]
	chairCreationClick(t, view, locales.RoundtableCollaboration())
	chairCreationClick(t, view, host.Name)
	checkChairRoster(t, meeting, true, worker)
	if !view.tt.HasText(locales.RoundtableModeratorUnavailable()) {
		t.Fatal("unavailable chair binding has no explanation")
	}
	server := view.a.settings.AgentServers[host.Provider]
	server.Disabled = false
	view.a.settings.AgentServers[host.Provider] = server
	view.a.checkMeetingConfig(view.a.agentProviders())
	checkChairRoster(t, meeting, true, worker)
	meeting.speakers[0].adapter.backend = roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
		<-ctx.Done()
		return roundtable.Message{}, ctx.Err()
	})
	if err := meeting.room.Discuss(group.Topic, "Check this", worker.ID); err != nil {
		t.Fatal(err)
	}
	view.frames()
	chairCreationClick(t, view, host.Name)
	checkChairRoster(t, meeting, true, worker)
	if !meeting.room.Busy() {
		t.Fatal("changing chair interrupted the active turn")
	}
	meeting.room.Stop()
	meeting.readOnly = true
	view.frames()
	chairCreationClick(t, view, host.Name)
	checkChairRoster(t, meeting, true, worker)
	meeting.readOnly = false
	view.a.root = t.TempDir()
	view.frames()
	chairCreationClick(t, view, host.Name)
	checkChairRoster(t, meeting, true, worker)
	view.a.root = view.root
	view.frames()
	chairCreationClick(t, view, host.Name)
	checkChairRoster(t, meeting, false, worker, host)
	if meeting.room.MentionTarget("@"+host.ID+" summarize") != host.ID {
		t.Fatal("the new moderator is missing its live binding")
	}
	chairCreationClick(t, view, locales.RoundtableHumanChair())
	chairCreationClick(t, view, host.Name)
	checkChairRoster(t, meeting, false, worker, host)
	view.a.closeMeeting()
	view.a.openMeetingRecord(library, group.ID)
	meeting = view.w.meeting
	checkChairRoster(t, meeting, false, worker, host)
	meeting.speakers[1].adapter.backend = roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
		return roundtable.Message{Text: "FINAL\nKeep the decision"}, nil
	})
	if err := meeting.room.Discuss(group.Topic, "Summarize", ""); err != nil {
		t.Fatal(err)
	}
	view.until(t, "moderator's conclusion", func() bool { return !meeting.room.Busy() })
	history := meeting.room.Snapshot().Messages
	last := history[len(history)-1]
	if last.AgentID != host.ID || last.Task == nil || last.Task.Stage != roundtable.Conclusion {
		t.Fatalf("unexpected chair reply: %+v", last)
	}
	first := history[0]
	if first.ID != decision.ID || first.Text != decision.Text || !first.HumanConclusion || first.Role != "user" {
		t.Fatalf("handoff changed the archived human conclusion: %+v", first)
	}
}

func TestChairCreationFullRosterKeepsEveryInvitee(t *testing.T) {
	view := chairCreationForm(t)
	view.w.createHumanChair = true
	view.w.topic = "Full roster"
	view.a.settings.AgentRoles = map[string]settings.AgentRole{}
	var want []roundtable.Participant
	for index := range roundtable.MaxMembers {
		name := fmt.Sprintf("Guest %02d", index)
		view.a.settings.AgentRoles[name] = settings.AgentRole{Agent: "local"}
		ref := roundtable.Participant{ID: "role:" + name, Name: name, Provider: "local"}
		view.w.selected[ref.ID] = ref
		want = append(want, ref)
	}
	view.frames()
	chairCreationClick(t, view, locales.AgentRoleRoundtableModerator())
	chairCreationClick(t, view, locales.RoundtableCreate())
	if !view.w.creating || len(view.w.selected) != len(want) {
		t.Fatal("the chair change dropped a guest or created an oversized meeting")
	}
	chairCreationClick(t, view, locales.RoundtableHumanChair())
	chairCreationClick(t, view, locales.RoundtableCreate())
	checkChairRoster(t, view.w.meeting, true, want...)
	chairCreationClick(t, view, locales.RoundtableCollaboration())
	chairCreationClick(t, view, locales.AgentRoleRoundtableModerator())
	want = append(want, view.a.meetingModerator().Participant)
	checkChairRoster(t, view.w.meeting, false, want...)
	view.w.removeParticipant(roundtable.ModeratorID)
	checkChairRoster(t, view.w.meeting, true, want[:len(want)-1]...)
	chairCreationClick(t, view, locales.AgentRoleRoundtableModerator())
	checkChairRoster(t, view.w.meeting, false, want...)
}

func TestChairCreationLegacyRefreshUpdatesLiveArchive(t *testing.T) {
	view := chairCreationForm(t)
	worker := roundtable.Participant{ID: "role:Observer", Name: "Observer", Provider: "local"}
	moderator := roundtable.Participant{ID: roundtable.ModeratorID, Name: locales.AgentRoleRoundtableModerator(), Provider: "local"}
	library, err := view.a.roundtableLibrary()
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.Create("Legacy review", []roundtable.Participant{worker})
	if err != nil {
		t.Fatal(err)
	}
	server := view.a.settings.AgentServers["local"]
	delete(view.a.settings.AgentServers, "local")
	view.a.openMeetingRecord(library, group.ID)
	meeting := view.a.meeting
	checkChairRoster(t, meeting, false, worker)
	view.a.settings.AgentServers["local"] = server
	view.a.checkMeetingConfig(view.a.agentProviders())
	if err := meeting.room.SetSystemPrompt("Keep the review"); err != nil {
		t.Fatal(err)
	}
	checkChairRoster(t, meeting, false, worker, moderator)
}

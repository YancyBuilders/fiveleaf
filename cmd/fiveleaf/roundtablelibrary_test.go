package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestRoundtableCreateRoleHandoffAndResume(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs, logPath := agentRoleHarness(t, "roundtable")
	hs.a.settings.AgentRoles = map[string]settings.AgentRole{
		"Planner":  {Agent: "backend", Model: "model-a", Prompt: "Find the milestones."},
		"Reviewer": {Agent: "backend", Model: "model-b", Prompt: "Look for rollback risks."},
	}
	configure := func(owner *app) *roundtableHarness {
		window := newRoundtableWindow(owner, nil)
		owner.meetingWindow = window
		t.Cleanup(window.finish)
		h := &roundtableHarness{harness: hs, w: window, tt: ui.NewTester(meetingTestView(window), 640, 600)}
		h.frames()
		return h
	}
	h := configure(hs.a)
	if _, back := h.tt.Find(locales.RoundtableGroups()); back {
		t.Fatal("the creation form still offers a separate history page")
	}
	if _, configure := h.tt.Find(locales.AgentConfigure()); configure {
		t.Fatal("the invitation form still offers the Agent settings menu")
	}
	click := func(name string) {
		t.Helper()
		if err := h.tt.Click(name); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	click(locales.RoundtableCreate())
	if h.w.meeting != nil {
		t.Fatal("created without a topic and invitations")
	}
	click(locales.RoundtableTopic())
	h.tt.Type("Release review")
	click(locales.AgentRoleRoundtableModerator())
	if !h.tt.HasText(locales.RoundtableAutoJoined()) || !h.tt.HasText(locales.RoundtableSelection(1)) {
		t.Fatal("moderator must stay invited before choosing other roles")
	}
	click("Planner")
	click("Reviewer")
	if !h.tt.HasText(locales.RoundtableSelection(3)) {
		t.Fatal(h.tt.Texts())
	}
	click(locales.RoundtableCreate())
	meeting := h.w.meeting
	if meeting == nil || meeting.group.Topic != "Release review" || len(meeting.speakers) != 3 || meeting.speakers[0].ID != roundtable.ModeratorID || h.tt.HasText(locales.AgentRoleTester()) {
		t.Fatal("the chat roster does not match the invitations", h.tt.Texts())
	}
	if err := meeting.room.SetOptions(roundtable.Options{ContextMessages: 10, MaxHops: 2}); err != nil {
		t.Fatal(err)
	}
	h.mentionMember(t, "Planner")
	h.tt.Type("Begin the release review")
	click(locales.AgentSend())
	h.until(t, "both roles", func() bool { return !meeting.room.Snapshot().Busy })
	before := meeting.room.Snapshot()
	if before.Error != "" || len(before.Messages) != 3 || before.Messages[1].AgentID != "role:Planner" || before.Messages[2].AgentID != "role:Reviewer" || !h.tt.HasText("Rollback is covered.") {
		t.Fatalf("handoff: %+v", before)
	}
	var prompts []roleRuntimeEvent
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts = append(prompts, event)
		}
	}
	if len(prompts) != 2 || prompts[0].Model != "model-a" || prompts[1].Model != "model-b" || !strings.Contains(prompts[0].Text, "Find the milestones.") || strings.Contains(prompts[0].Text, "Look for rollback risks.") || !strings.Contains(prompts[1].Text, "Look for rollback risks.") || !strings.Contains(prompts[1].Text, "Your assignment: check the rollback plan") || !strings.Contains(prompts[1].Text, "Release review") {
		t.Fatal("roles received the wrong model, instructions or shared context", prompts)
	}
	h.w.newGroup()
	h.frames()
	click(locales.RoundtableTopic())
	h.tt.Type("Next quarter")
	click("Reviewer")
	click(locales.RoundtableCreate())
	if len(h.w.meeting.room.Snapshot().Messages) != 0 {
		t.Fatal("group histories mixed")
	}
	h.w.finish()
	window := hs.a.settings.NewWindow()
	window.Folder = hs.root
	fresh := newApp(&fakeHost{folder: hs.root}, hs.a.settings, window)
	fresh.root = hs.root
	t.Cleanup(fresh.shutdown)
	h = configure(fresh)
	if !h.tt.HasText(locales.RoundtableTopic()) || h.tt.HasText("Next quarter") {
		t.Fatal("the roundtable entry must open the creation form even with saved meetings")
	}
	agentHome(t, hs.root)
	browser := &harness{t: t, a: fresh, root: hs.root, tt: ui.NewTester(fresh.view, 1200, 760)}
	fresh.showSessions()
	browser.settle()
	if !browser.tt.HasText("Next quarter") || !browser.tt.HasText("Release review") {
		t.Fatal("saved meetings missing from Sessions after restart", browser.tt.Texts())
	}
	if err := browser.tt.Click(locales.SessionsFind()); err != nil {
		t.Fatal(err)
	}
	browser.tt.Type("Release review")
	browser.settle()
	browser.key(0, ui.KeyEnter)
	h.frames()
	if h.w.meeting == nil {
		t.Fatal("Enter in Sessions did not reopen the saved meeting")
	}
	after := h.w.meeting.room.Snapshot()
	if after.Busy || len(after.Messages) != len(before.Messages) || h.w.meeting.group.Options.MaxHops != 2 {
		t.Fatal(after)
	}
	for i, old := range before.Messages {
		message := after.Messages[i]
		if message.ID != old.ID || message.Name != old.Name || message.Text != old.Text || !message.Time.Equal(old.Time) {
			t.Fatal("restored message changed", old, message)
		}
	}
	delete(fresh.settings.AgentRoles, "Planner")
	fresh.checkAgentConfig()
	h.frames()
	if err := h.w.meeting.room.Send("private target", "role:Planner"); !errors.Is(err, roundtable.ErrUnknownAgent) {
		t.Fatal(err)
	}
	click(locales.AgentDraft())
	h.tt.Type("@Reviewer continue")
	click(locales.AgentSend())
	h.until(t, "resumed reply", func() bool { return !h.w.meeting.room.Snapshot().Busy })
	if snap := h.w.meeting.room.Snapshot(); len(snap.Messages) != 5 || snap.Messages[4].Name != "Reviewer" || snap.Error != "" {
		t.Fatal(snap)
	}
	events := agentRoleEvents(t, logPath)
	last := slices.DeleteFunc(events, func(e roleRuntimeEvent) bool { return e.Kind != "prompt" })
	if len(last) != 3 || !strings.Contains(last[2].Text, before.Messages[2].Text) {
		t.Fatal("reopening replayed a prompt or lost history", last)
	}
}

func TestRoundtableLoadedPagesReturnToLiveMessages(t *testing.T) {
	h := newRoundtableHarness(t, roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
		<-ctx.Done()
		return roundtable.Message{}, ctx.Err()
	}))
	for index := range meetingPageSize + 2 {
		if err := h.w.meeting.room.Send(fmt.Sprintf("message %03d", index), "Alpha"); err != nil {
			t.Fatal(err)
		}
	}
	h.w.meeting.room.Stop()
	h.frames()
	if err := h.tt.Click(locales.RoundtableEarlier()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.tt.HasText("message 000") || len(h.w.page) != 2 {
		t.Fatal("older records missing", h.tt.Texts())
	}
	if err := h.tt.Click(locales.RoundtableNewer()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.page != nil || !h.tt.HasText("message 041") {
		t.Fatal("newest records missing", h.tt.Texts())
	}
}

func TestRoundtableLegacyModeratorMigration(t *testing.T) {
	hs := agentRosterHarness(t)
	for _, foreign := range []bool{true, false} {
		project := hs.root
		if foreign {
			project = t.TempDir()
		}
		lib, err := roundtable.ProjectLibrary(t.TempDir(), project)
		if err != nil {
			t.Fatal(err)
		}
		member := roundtable.Participant{ID: "agent:alpha", Name: "Original speaker", Provider: "alpha"}
		group, err := lib.Create("Existing discussion", []roundtable.Participant{member})
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			meeting, err := hs.a.startMeeting(lib, group)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := lib.Group(group.ID)
			if err != nil {
				t.Fatal(err)
			}
			if meeting.readOnly != foreign || !slices.Equal(stored.Members, meeting.group.Members) || stored.Members[0] != member || meeting.room.Snapshot().Busy {
				t.Fatal("opening changed historical attribution or started a turn", stored)
			}
			if foreign && len(stored.Members) != 1 || !foreign && (len(stored.Members) != 2 || stored.Members[1].ID != roundtable.ModeratorID) {
				t.Fatal("moderator migration must occur exactly once in the owning project", stored)
			}
			hs.a.closeMeeting()
		}
	}
}

func TestRoundtableCLIUsesInvitedRoleAndModel(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "cli")
	server := hs.a.settings.AgentServers["backend"]
	server.Args = append(server.Args, "--", "--model={model}")
	hs.a.settings.AgentServers["backend"] = server
	lib, err := roundtable.ProjectLibrary(t.TempDir(), hs.root)
	if err != nil {
		t.Fatal(err)
	}
	name := "审阅 角色"
	group, err := lib.Create("接口兼容性", []roundtable.Participant{{ID: "role:" + name, Name: name, Provider: "backend"}})
	if err != nil {
		t.Fatal(err)
	}
	meeting, err := hs.a.startMeeting(lib, group)
	if err != nil {
		t.Fatal(err)
	}
	if err := meeting.room.Send("检查公开接口", ""); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("CLI role reply", func() bool { return !meeting.room.Snapshot().Busy })
	snap := meeting.room.Snapshot()
	if len(snap.Messages) != 2 || snap.Messages[1].Name != name || snap.Error != "" {
		t.Fatal(snap)
	}
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "cli" && slices.Contains(event.Args, "--model=model-a") && strings.Contains(event.Text, "接口兼容性") && strings.Contains(event.Text, "Check the code carefully.") {
			return
		}
	}
	t.Fatal("CLI did not receive the invited role’s model and instructions")
}

func roundtableGallery(hs *harness, groups bool) {
	hs.t.Setenv("FIVELEAF_DATA_DIR", hs.t.TempDir())
	hs.a.settings.AgentServers = map[string]settings.AgentServer{"local": {Name: "Pi", Command: "pi", Protocol: "cli"}}
	w := newRoundtableWindow(hs.a, nil)
	hs.a.meetingWindow = w
	hs.t.Cleanup(w.finish)
	w.topic = locales.RoundtableTopicExample()
	for _, speaker := range hs.a.meetingInvitations() {
		if speaker.ID == "preset:plan" || speaker.ID == "preset:review" {
			w.selected[speaker.ID] = speaker.Participant
		}
	}
	if groups {
		lib, err := hs.a.roundtableLibrary()
		if err != nil {
			hs.t.Fatal(err)
		}
		if _, err := lib.Create("Release review", w.invitations(hs.a.meetingInvitations())); err != nil {
			hs.t.Fatal(err)
		}
		hs.a.showSessions()
		hs.a.sessions.harness = meetingHistorySource
		hs.settle()
		return
	}
	hs.tt = ui.NewTester(meetingTestView(w), 820, 620)
	hs.tt.SetDark(hs.a.pal.dark)
	hs.tt.SetLocale(i18n.Locale())
}

func roundtableTasksGallery(hs *harness) {
	roundtableGallery(hs, false)
	window := hs.a.meetingWindow
	window.createGroup()
	meeting := window.meeting
	for _, speaker := range meeting.speakers {
		speaker.adapter.backend = roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
			return roundtable.Message{Text: "The estimate is fixed at submission and survives a refresh."}, nil
		})
	}
	if err := meeting.room.Discuss(meeting.group.Topic, "Check the estimate after a refresh", "preset:review"); err != nil {
		hs.t.Fatal(err)
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for meeting.room.Snapshot().Busy {
		select {
		case <-meeting.room.Events():
		case <-timer.C:
			hs.t.Fatal("gallery task did not complete")
		}
	}
	window.selectMember("preset:review")
	window.expandedTask = meeting.room.Snapshot().Messages[1].ID
	hs.tt.Frame()
}

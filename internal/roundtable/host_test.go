package roundtable

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHumanChairControlsEveryAssignment(t *testing.T) {
	moderator, hostCalls := heldAdapter()
	specialist, workerCalls := heldAdapter()
	room := New([]Member{{ID: ModeratorID, Enabled: true, Adapter: moderator}, {ID: "review", Name: "Reviewer", Enabled: true, Adapter: specialist}}, Options{ContextMessages: 23, MaxHops: 8})
	defer room.Close()
	if err := room.SetHumanChair(true); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss("Planning", "Our agenda", ""); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss("Planning", "@unknown question", ""); !errors.Is(err, ErrUnknownAgent) {
		t.Fatal(err)
	}
	if got := room.Snapshot(); got.Busy || len(got.Messages) != 1 || got.Messages[0].Text != "Our agenda" || !got.HumanChair {
		t.Fatal(got)
	}
	if err := room.Discuss("Planning", "@Reviewer check the estimate", ""); err != nil {
		t.Fatal(err)
	}
	call := nextCall(t, workerCalls)
	if !strings.Contains(FormatPrompt(call.messages), "The human chairs this meeting") {
		t.Fatal(call.messages)
	}
	if err := room.SetHumanChair(false); !errors.Is(err, ErrTurnActive) {
		t.Fatal(err)
	}
	if err := room.PublishConclusion("Too soon"); !errors.Is(err, ErrTurnActive) {
		t.Fatal(err)
	}
	call.answer <- Message{Text: "@" + ModeratorID + " handle the next step\nThe estimate is stable."}
	got := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if len(got.Messages) != 3 || got.Messages[2].AgentID != "review" || got.Messages[2].Task.Stage != Direct || len(hostCalls) != 0 {
		t.Fatal(got)
	}
	if err := room.PublishConclusion("Use the estimate at submission."); err != nil {
		t.Fatal(err)
	}
	got = room.Snapshot()
	last := got.Messages[len(got.Messages)-1]
	if last.Role != "user" || !last.HumanConclusion || !last.InOverview() || len(hostCalls) != 0 || len(workerCalls) != 0 {
		t.Fatal(last)
	}
	if err := room.SetHumanChair(false); err != nil {
		t.Fatal(err)
	}
	if err := room.PublishConclusion("not in charge"); !errors.Is(err, ErrHumanChairRequired) {
		t.Fatal(err)
	}
	if err := room.Discuss("Planning", "Summarize", ""); err != nil {
		t.Fatal(err)
	}
	nextCall(t, hostCalls).answer <- Message{Text: "Final estimate"}
	got = awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if got.HumanChair || got.Messages[len(got.Messages)-1].Task.Stage != Conclusion || room.options.ContextMessages != 23 || room.options.MaxHops != 8 {
		t.Fatal(got, room.options)
	}
}

func TestHumanChairPublishesWithoutModelAndSurvivesReopen(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.CreateWithChair("Release", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	room, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = room.Discuss(group.Topic, "Audience remarks", ""); err != nil {
		t.Fatal(err)
	}
	if err = room.SetHumanChair(false); !errors.Is(err, ErrModeratorUnavailable) {
		t.Fatal(err)
	}
	if err = room.PublishConclusion("Decision recorded"); err != nil {
		t.Fatal(err)
	}
	room.Close()
	saved, err := lib.Group(group.ID)
	if err != nil || !saved.Options.HumanChair {
		t.Fatal(saved, err)
	}
	room, err = lib.OpenRoom(saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	got := room.Snapshot()
	if !got.HumanChair || len(got.Messages) != 2 || !got.Messages[1].HumanConclusion || got.Messages[0].HumanConclusion || got.Busy {
		t.Fatal(got)
	}
}

func TestHumanChairSettingsAndConclusionAreAtomic(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	participants := []Participant{{ID: ModeratorID, Name: "Moderator", Provider: "pi"}}
	group, err := lib.Create("Review", participants)
	if err != nil {
		t.Fatal(err)
	}
	room, err := lib.OpenRoom(group, []Member{{ID: ModeratorID, Enabled: true, Adapter: answer("Ready")}})
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	groupPath := filepath.Join(room.archive.dir, "group.json")
	if err = os.Rename(groupPath, groupPath+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(groupPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err = room.SetHumanChair(true); err == nil || room.Snapshot().HumanChair || room.archive.group.Options.HumanChair {
		t.Fatal("an unpersisted chair became active", err)
	}
	if err = os.Remove(groupPath); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(groupPath+".old", groupPath); err != nil {
		t.Fatal(err)
	}
	if err = room.SetHumanChair(true); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(room.archive.dir, "messages"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = room.PublishConclusion("Write must fail"); err == nil || len(room.Snapshot().Messages) != 0 {
		t.Fatal("lost or partial conclusion", err)
	}
}

func TestHumanChairRejectsLateDirectedReply(t *testing.T) {
	adapter, calls := heldAdapter()
	room := New([]Member{{ID: "review", Enabled: true, Adapter: adapter}, {ID: "disabled", Enabled: false, Adapter: adapter}}, Options{HumanChair: true})
	defer room.Close()
	if err := room.Discuss("Agenda", "Review the first draft", "review"); err != nil {
		t.Fatal(err)
	}
	old := nextCall(t, calls)
	if err := room.Discuss("Agenda", "@disabled invalid", ""); !errors.Is(err, ErrUnknownAgent) || !room.Busy() {
		t.Fatal(err)
	}
	if err := room.Discuss("Agenda", "New conclusion pending", ""); err != nil {
		t.Fatal(err)
	}
	if old.ctx.Err() == nil || room.Busy() {
		t.Fatal("new human note did not end superseded assignment")
	}
	old.answer <- Message{Text: "FINAL\nOld answer"}
	if err := room.Discuss("Agenda", "Review the updated draft", "review"); err != nil {
		t.Fatal(err)
	}
	nextCall(t, calls).answer <- Message{Text: "FINAL\nConfirmed"}
	got := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if len(got.Messages) != 5 || got.Messages[1].Task.Status != Stopped || got.Messages[4].Text != "Confirmed" || got.Messages[4].HumanConclusion || got.Messages[4].Task.Stage != Direct {
		t.Fatal(got)
	}
}

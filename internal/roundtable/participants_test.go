package roundtable

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func TestRemoveParticipantPreservesHistoryAndReinvites(t *testing.T) {
	library, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reviewer := Participant{"role:review", "Reviewer", "local"}
	planner := Participant{"role:plan", "Planner", "local"}
	moderator := Participant{ModeratorID, "Chair", "local"}
	group, err := library.Create("Planning", []Participant{reviewer, planner})
	if err != nil {
		t.Fatal(err)
	}
	other, err := library.Create(group.Topic, group.Members)
	if err != nil {
		t.Fatal(err)
	}
	member := Member{ID: reviewer.ID, Name: reviewer.Name, Enabled: true, Adapter: answer("First review")}
	room, err := library.OpenRoom(group, []Member{member})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(room.Close)
	if err := room.SetSystemPrompt("Keep the record"); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss(group.Topic, "Review the plan", reviewer.ID); err != nil {
		t.Fatal(err)
	}
	history := awaitRoom(t, room, func(snapshot Snapshot) bool { return !snapshot.Busy }).Messages
	if len(history) != 2 || history[1].AgentID != reviewer.ID || history[1].Task == nil {
		t.Fatalf("missing review task: %+v", history)
	}
	if err := room.AddMemory("Remember the review", history[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := library.EnsureModerator(group.ID, moderator); err != nil {
		t.Fatal(err)
	}
	updated, err := room.RemoveParticipant(reviewer.ID)
	if err != nil || !slices.Equal(updated.Members, []Participant{planner, moderator}) {
		t.Fatalf("removing the reviewer lost other invitations: %v, %+v", err, updated)
	}
	if err := room.Discuss(group.Topic, "For the reviewer only", reviewer.ID); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("removed member is still routable: %v", err)
	}
	room.Close()
	saved, err := library.Group(group.ID)
	if err != nil || !slices.Equal(saved.Members, updated.Members) || saved.SystemPrompt != "Keep the record" {
		t.Fatalf("wrong saved group: %v, %+v", err, saved)
	}
	untouched, err := library.Group(other.ID)
	if err != nil || !slices.Equal(untouched.Members, []Participant{reviewer, planner}) {
		t.Fatalf("another meeting changed: %v, %+v", err, untouched)
	}
	reopened, err := library.OpenRoom(saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	old := reopened.Snapshot().Messages
	if len(old) != len(history) || old[0].ID != history[0].ID || old[1].ID != history[1].ID || old[1].Name != reviewer.Name || old[1].Text != "First review" || old[1].Task == nil || old[1].Task.TurnID != old[0].ID {
		t.Fatalf("archive lost the original task or its attribution: %+v", old)
	}
	if notes := reopened.Memory().Notes; len(notes) != 1 || notes[0].SourceID != old[1].ID {
		t.Fatalf("archive lost the pinned review: %+v", notes)
	}
	reinvited, err := reopened.InviteParticipant(reviewer)
	if err != nil || !slices.Equal(reinvited.Members, []Participant{planner, moderator, reviewer}) {
		t.Fatalf("could not reinvite the reviewer: %v, %+v", err, reinvited)
	}
	reopened.SetMembers([]Member{member})
	if err := reopened.Discuss(group.Topic, "Review again", reviewer.ID); err != nil {
		t.Fatal(err)
	}
	latest := awaitRoom(t, reopened, func(snapshot Snapshot) bool { return !snapshot.Busy }).Messages
	if len(latest) != 4 || latest[3].AgentID != reviewer.ID || !reflect.DeepEqual(latest[:2], old) {
		t.Fatalf("reinvitation split or modified the reviewer's history: %+v", latest)
	}
}

func TestRemoveParticipantReclaimsSlotInFullMigratedGroup(t *testing.T) {
	library, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var invited []Participant
	for index := range MaxMembers {
		name := fmt.Sprintf("Reviewer %d", index)
		invited = append(invited, Participant{"role:" + name, name, "local"})
	}
	group, err := library.Create("Original group", invited)
	if err != nil {
		t.Fatal(err)
	}
	group, err = library.EnsureModerator(group.ID, Participant{ModeratorID, "Chair", "local"})
	if err != nil || group.CanInviteParticipant() {
		t.Fatalf("migration did not reserve the moderator seat: %v, %d seats", err, len(group.Members))
	}
	room, err := library.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(room.Close)
	group, err = room.RemoveParticipant(invited[0].ID)
	if err != nil || !group.CanInviteParticipant() {
		t.Fatalf("removal did not release the invitation: %v", err)
	}
	group, err = room.InviteParticipant(invited[0])
	if err != nil || group.CanInviteParticipant() {
		t.Fatalf("reinviting into the released seat: %v", err)
	}
	saved, err := library.Group(group.ID)
	if err != nil || len(saved.Members) != MaxMembers+1 || saved.Members[MaxMembers] != invited[0] {
		t.Fatalf("lost the last invitation: %v, %+v", err, saved.Members)
	}
}

func TestRemoveParticipantLastMemberRequiresHumanChair(t *testing.T) {
	library, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	member := Participant{"role:plan", "Planner", "local"}
	group, err := library.Create("Legacy group", []Participant{member})
	if err != nil {
		t.Fatal(err)
	}
	room, err := library.OpenRoom(group, []Member{{ID: member.ID, Name: member.Name, Enabled: true, Adapter: answer("ok")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(room.Close)
	if _, err := room.RemoveParticipant(member.ID); err == nil || room.MentionTarget("@Planner hello") != member.ID {
		t.Fatal("an invalid removal changed the live member list")
	}
	saved, err := library.Group(group.ID)
	if err != nil || !slices.Equal(saved.Members, group.Members) {
		t.Fatal("an invalid removal changed the saved member list", err)
	}
	if err := room.SetHumanChair(true); err != nil {
		t.Fatal(err)
	}
	if _, err := room.RemoveParticipant(member.ID); err != nil {
		t.Fatal(err)
	}
	saved, err = library.Group(group.ID)
	if err != nil || len(saved.Members) != 0 || !saved.Options.HumanChair {
		t.Fatalf("human-only meeting could not reopen: %v, %+v", err, saved)
	}
}

func TestRemoveParticipantSwitchesFromModeratorToHumanChair(t *testing.T) {
	library, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	moderator := Participant{ModeratorID, "Chair", "local"}
	reviewer := Participant{"role:review", "Reviewer", "local"}
	group, err := library.Create("Review", []Participant{moderator, reviewer})
	if err != nil {
		t.Fatal(err)
	}
	adapter, calls := heldAdapter()
	room, err := library.OpenRoom(group, []Member{{ID: reviewer.ID, Enabled: true, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(room.Close)
	updated, err := room.RemoveParticipant(moderator.ID)
	if err != nil || updated.HasModerator() || !updated.Options.HumanChair || !room.Snapshot().HumanChair {
		t.Fatalf("removing the automatic moderator did not switch to human chair: %v, %+v", err, updated)
	}
	if _, err := room.RemoveParticipant("unknown"); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("unknown participant: %v", err)
	}
	if err := room.Discuss(group.Topic, "Review now", reviewer.ID); err != nil {
		t.Fatal(err)
	}
	call := nextCall(t, calls)
	_, removeErr := room.RemoveParticipant(reviewer.ID)
	call.answer <- Message{Text: "Review complete"}
	if !errors.Is(removeErr, ErrTurnActive) {
		t.Fatalf("removal during a turn: %v", removeErr)
	}
	snapshot := awaitRoom(t, room, func(snapshot Snapshot) bool { return !snapshot.Busy })
	saved, err := library.Group(group.ID)
	if err != nil || !slices.Equal(saved.Members, updated.Members) || !saved.Options.HumanChair || len(snapshot.Messages) != 2 || snapshot.Messages[1].Text != "Review complete" {
		t.Fatalf("rejected removal changed the group or interrupted the turn: %v, %+v, %+v", err, saved, snapshot)
	}
	room.Close()
	if _, err := room.RemoveParticipant(reviewer.ID); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed room: %v", err)
	}
}

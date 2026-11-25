package roundtable

import (
	"slices"
	"strings"
	"testing"
)

func TestChairChangesPreserveGroupMemoryAndTranscript(t *testing.T) {
	library, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	participants := []Participant{
		{ID: ModeratorID, Name: "Moderator", Provider: "local"},
		{ID: "role:analyst", Name: "Analyst", Provider: "local"},
	}
	group, err := library.Create("Estimate review", participants)
	if err != nil {
		t.Fatal(err)
	}
	worker, calls := heldAdapter()
	members := []Member{
		{ID: ModeratorID, Name: "Moderator", Enabled: true, Adapter: answer("Moderator's conclusion")},
		{ID: "role:analyst", Name: "Analyst", Enabled: true, Adapter: worker},
	}
	room, err := library.OpenRoom(group, members)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	const decision = "Keep the estimate fixed at submission (RT-417)."
	const remark = "Review the refresh case on Thursday."
	if err := room.AddMemory(decision, ""); err != nil {
		t.Fatal(err)
	}
	if err := room.SetMemoryWindow(30); err != nil {
		t.Fatal(err)
	}
	if err := room.SetHumanChair(true); err != nil {
		t.Fatal(err)
	}
	if err := room.Discuss(group.Topic, remark, ""); err != nil {
		t.Fatal(err)
	}
	if room.Busy() {
		t.Fatal("a chair's remark unexpectedly started an agent")
	}
	if err := room.Discuss(group.Topic, "Report the criterion", "role:analyst"); err != nil {
		t.Fatal(err)
	}
	request := nextCall(t, calls)
	if prompt := FormatPrompt(request.messages); !strings.Contains(prompt, decision) || !strings.Contains(prompt, remark) {
		t.Fatal("manual assignment lost the pinned criterion or the chair's remark", prompt)
	}
	request.answer <- Message{Text: "Fixed at submission."}
	before := awaitRoom(t, room, func(snapshot Snapshot) bool { return !snapshot.Busy })
	if err := room.SetMemorySharing(false); err != nil {
		t.Fatal(err)
	}
	room.Close()
	saved, err := library.Group(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := library.OpenRoom(saved, members)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	memory := reopened.Memory()
	if !saved.Options.HumanChair || memory.Shared || memory.Window != 30 || len(memory.Notes) != 1 || memory.Notes[0].Text != decision {
		t.Fatal("restoring one control reset another", saved.Options, memory)
	}
	if err := reopened.ClearMemory(); err != nil {
		t.Fatal(err)
	}
	if len(reopened.Memory().Notes) != 0 || len(reopened.Memory().Recent) != 0 {
		t.Fatal("cleared memory still has entries")
	}
	if !slices.EqualFunc(before.Messages, reopened.Snapshot().Messages, func(left, right Message) bool { return left.ID == right.ID && left.Text == right.Text }) {
		t.Fatal("clearing group memory changed the meeting transcript")
	}
	if err := reopened.SetHumanChair(false); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Discuss(group.Topic, "Close the discussion", ""); err != nil {
		t.Fatal(err)
	}
	after := awaitRoom(t, reopened, func(snapshot Snapshot) bool { return !snapshot.Busy })
	if last := after.Messages[len(after.Messages)-1]; last.AgentID != ModeratorID || last.Task == nil || last.Task.Stage != Conclusion {
		t.Fatal("chair was not handed back", last)
	}
}

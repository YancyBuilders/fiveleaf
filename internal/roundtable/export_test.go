package roundtable

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExportHistoryBeyondBothLimits(t *testing.T) {
	lib, err := ProjectLibrary(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	group, err := lib.Create("Export", []Participant{{"planner", "Planner", "test"}})
	if err != nil {
		t.Fatal(err)
	}
	room, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer room.Close()
	start := time.Now()
	var ids []string
	large := strings.Repeat("x", MaxMessageBytes-32)
	for index := range MaxHistoryMessages + 8 {
		id := fmt.Sprintf("message-%03d", index)
		body := id
		if index > MaxHistoryMessages {
			body = large + " last bytes of " + id
		}
		message := Message{ID: id, Role: "user", Text: body, Time: start.Add(time.Duration(index))}
		if index%2 == 0 {
			message.Role = "assistant"
			message.Task = &Task{Stage: Work, Status: Completed}
		}
		room.mu.Lock()
		err = room.recordLocked(message)
		room.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if snapshot := room.Snapshot(); !snapshot.HasEarlier || len(snapshot.Messages) >= MaxHistoryMessages {
		t.Fatal("fixture must exceed both the message and byte caps")
	}
	history := room.ExportHistory()
	room.mu.Lock()
	err = room.recordLocked(Message{ID: "newer", Time: time.Now(), Role: "user", Text: "After selecting Export"})
	room.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	room.Close()
	if err := history.Visit(func(message Message) error {
		if message.ID != ids[0] {
			t.Fatalf("cancel started at %q", message.ID)
		}
		return context.Canceled
	}); err != context.Canceled {
		t.Fatalf("visitor cancellation = %v", err)
	}
	var got []string
	if err := history.Visit(func(message Message) error {
		got = append(got, message.ID)
		if message.ID == ids[len(ids)-1] && message.Text != large+" last bytes of "+message.ID {
			t.Fatal("last message was shortened")
		}
		return nil
	}); err != nil || !slices.Equal(got, ids) {
		t.Fatalf("exported %d of %d messages in order: %v", len(got), len(ids), err)
	}
	reader, err := lib.OpenRoom(group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := reader.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := history.Visit(func(Message) error { return nil }); err == nil {
		t.Fatal("removing the archive during an export reported a complete transcript")
	}
}

func TestExportHistoryKeepsActiveTaskOnInterrupt(t *testing.T) {
	calls := make(chan context.Context, 2)
	adapter := AdapterFunc(func(ctx context.Context, _ []Message) (Message, error) {
		calls <- ctx
		<-ctx.Done()
		return Message{}, ctx.Err()
	})
	room := New([]Member{{ID: "one", Name: "Planner", Enabled: true, Adapter: adapter}}, Options{})
	defer room.Close()
	if err := room.Discuss("Topic", "Original question", "one"); err != nil {
		t.Fatal(err)
	}
	var call context.Context
	select {
	case call = <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("waiting for the running task")
	}
	ReportTool(call, ToolRecord{ID: "tool", Status: "running", Note: "Before save"})
	history := room.ExportHistory()
	ReportTool(call, ToolRecord{ID: "tool", Status: "completed", Note: "After save"})
	if err := room.Send("Later question", "one"); err != nil {
		t.Fatal(err)
	}
	room.Close()
	var got []Message
	if err := history.Visit(func(message Message) error { got = append(got, message); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "Original question" || got[1].Task.Status != Running || got[1].Task.Tools[0].Note != "Before save" {
		t.Fatalf("export changed after interruption: %+v", got)
	}
}

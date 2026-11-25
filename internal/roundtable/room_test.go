package roundtable

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func awaitRoom(t *testing.T, room *Room, condition func(Snapshot) bool) Snapshot {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		snapshot := room.Snapshot()
		if condition(snapshot) {
			return snapshot
		}
		select {
		case _, ok := <-room.Events():
			if !ok {
				t.Fatal("room closed before its expected state")
			}
		case <-timer.C:
			t.Fatalf("room state did not arrive: %+v", snapshot)
		}
	}
}

func sendRoom(t *testing.T, room *Room, text, target string) Snapshot {
	t.Helper()
	if err := room.Send(text, target); err != nil {
		t.Fatal(err)
	}
	return awaitRoom(t, room, func(snapshot Snapshot) bool { return !snapshot.Busy })
}

func answer(text string) Adapter {
	return AdapterFunc(func(context.Context, []Message) (Message, error) { return Message{Text: text}, nil })
}

func TestRoutingMentionsCapabilitiesAndRoundRobin(t *testing.T) {
	members := []Member{
		{ID: "coder", Name: "Code Agent", Capabilities: []string{"code"}, Enabled: true, Adapter: answer("ok")},
		{ID: "research", Name: "Research", Capabilities: []string{"search"}, Enabled: true, Adapter: answer("ok")},
		{ID: "research-long", Name: "Research Agent", Capabilities: []string{"reasoning"}, Enabled: true, Adapter: answer("ok")},
		{ID: "disabled", Name: "Disabled", Capabilities: []string{"code"}, Enabled: false, Adapter: answer("ok")},
	}
	for _, tc := range []struct{ text, want string }{
		{"@Research Agent please implement this", "research-long"},
		{" \n @Research Agent hello", "research-long"},
		{"@RESEARCH please think", "research"},
		{"@research-long, help", "research-long"},
		{"Please search for references", "research"},
		{"分析这个推理问题", "research-long"},
		{"请修复代码", "coder"},
		{"email@ResearchAgent is an address", "coder"},
		{"@Disabled hello", ""},
		{"@ResearchAgent hello", ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			next := 0
			got, ok := routedMember(members, tc.text, &next)
			if ok != (tc.want != "") || got.ID != tc.want {
				t.Fatalf("route = %q, %v; want %q", got.ID, ok, tc.want)
			}
		})
	}
	next := 0
	for _, want := range []string{"coder", "research", "research-long", "coder"} {
		got, ok := routedMember(members, "hello", &next)
		if !ok || got.ID != want {
			t.Fatalf("round robin = %q, want %q", got.ID, want)
		}
	}
}

func TestRoomOnlyLeadingMentionSelectsTarget(t *testing.T) {
	room := New([]Member{
		{ID: "first", Name: "First Agent", Enabled: true, Adapter: answer("first answer")},
		{ID: "second", Name: "Second Agent", Enabled: true, Adapter: answer("second answer")},
	}, Options{MaxHops: 1})
	defer room.Close()
	got := sendRoom(t, room, "Please ask @Second Agent later", "")
	if len(got.Messages) != 2 || got.Messages[1].AgentID != "first" {
		t.Fatalf("embedded mention changed the target: %+v", got)
	}
	got = sendRoom(t, room, "  @Second Agent hello", "")
	if len(got.Messages) != 4 || got.Messages[3].AgentID != "second" {
		t.Fatalf("leading mention did not select its target: %+v", got)
	}
}

func TestMentionOfSameNamedRolesRequiresTheirID(t *testing.T) {
	members := []Member{{ID: "preset:plan", Name: "Planner", Enabled: true, Adapter: answer("first")}, {ID: "role:Planner", Name: "Planner", Enabled: true, Adapter: answer("second")}}
	room := New(members, Options{MaxHops: 1})
	defer room.Close()
	if err := room.Send("@Planner private", ""); !errors.Is(err, ErrUnknownAgent) {
		t.Fatal(err)
	}
	if err := room.Send("private", "Planner"); !errors.Is(err, ErrUnknownAgent) {
		t.Fatal(err)
	}
	got := sendRoom(t, room, "@role:Planner take a look", "")
	if len(got.Messages) != 2 || got.Messages[1].AgentID != "role:Planner" {
		t.Fatal(got)
	}
}

func TestRoomUnknownOrDisabledMentionNeverFallsBack(t *testing.T) {
	for _, text := range []string{"@missing private", "@disabled private", "@Helper Disabled private"} {
		t.Run(text, func(t *testing.T) {
			var firstCalls, otherCalls atomic.Int32
			room := New([]Member{
				{ID: "first", Name: "First", Enabled: true, Adapter: AdapterFunc(func(context.Context, []Message) (Message, error) {
					firstCalls.Add(1)
					return Message{Text: text}, nil
				})},
				{ID: "helper", Name: "Helper", Enabled: true, Adapter: AdapterFunc(func(context.Context, []Message) (Message, error) {
					otherCalls.Add(1)
					return Message{Text: "must not receive private text"}, nil
				})},
				{ID: "disabled", Name: "Helper Disabled", Enabled: false, Adapter: answer("disabled")},
			}, Options{})
			defer room.Close()
			if err := room.Send(text, ""); !errors.Is(err, ErrUnknownAgent) {
				t.Fatalf("unknown initial target = %v", err)
			}
			if got := room.Snapshot(); got.Busy || len(got.Messages) != 0 || firstCalls.Load() != 0 || otherCalls.Load() != 0 {
				t.Fatalf("initial mention was forwarded: %+v", got)
			}
			got := sendRoom(t, room, "hello", "")
			if got.Error != ErrUnknownAgent.Error() || len(got.Messages) != 2 || firstCalls.Load() != 1 || otherCalls.Load() != 0 {
				t.Fatalf("reply mention was forwarded: %+v, calls=%d/%d", got, firstCalls.Load(), otherCalls.Load())
			}
		})
	}
}

func TestRoomInvalidMentionDoesNotInterruptValidCall(t *testing.T) {
	adapter, calls := heldAdapter()
	room := New([]Member{{ID: "agent", Enabled: true, Adapter: adapter}}, Options{MaxHops: 1})
	defer room.Close()
	if err := room.Send("hello", ""); err != nil {
		t.Fatal(err)
	}
	call := nextCall(t, calls)
	if err := room.Send("@missing private", ""); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("invalid mention = %v", err)
	}
	select {
	case <-call.ctx.Done():
		t.Fatal("invalid mention cancelled a valid active call")
	default:
	}
	call.answer <- Message{Text: "valid answer"}
	got := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	if len(got.Messages) != 2 || got.Messages[1].Text != "valid answer" {
		t.Fatalf("invalid mention entered history: %+v", got)
	}
}

func TestRoomRoutesEveryReplyAndOwnsHop(t *testing.T) {
	room := New([]Member{
		{ID: "first", Name: "First", Enabled: true, Adapter: answer("@Second Agent please search")},
		{ID: "second", Name: "Second Agent", Enabled: true, Adapter: AdapterFunc(func(context.Context, []Message) (Message, error) {
			return Message{ID: "spoofed", Role: "system", AgentID: "first", Name: "Nobody", Text: "@First again", Hop: -1000}, nil
		})},
	}, Options{MaxHops: 1000})
	defer room.Close()
	snapshot := sendRoom(t, room, "@First start", "")
	if snapshot.Error != "" || len(snapshot.Messages) != 4 {
		t.Fatalf("round = %+v", snapshot)
	}
	for i, id := range []string{"first", "second", "first"} {
		m := snapshot.Messages[i+1]
		if m.AgentID != id || m.Hop != i+1 || m.Role != "assistant" || m.ID == "spoofed" || m.Time.IsZero() {
			t.Fatalf("reply %d = %+v", i, m)
		}
	}
	if snapshot.ActiveAgent != "" {
		t.Fatal("completed round retains an active member")
	}
}

func TestRoomDirectMessagesAndGroupAreIndependent(t *testing.T) {
	inputs := make(chan []Message, 4)
	member := Member{ID: "agent", Name: "Agent", Enabled: true, Adapter: AdapterFunc(func(_ context.Context, messages []Message) (Message, error) {
		inputs <- messages
		return Message{Text: "@Agent continue"}, nil
	})}
	dm, group := New([]Member{member}, Options{}), New([]Member{member}, Options{MaxHops: 1})
	defer dm.Close()
	defer group.Close()
	if err := dm.SetSystemPrompt("private instructions"); err != nil {
		t.Fatal(err)
	}
	if got := sendRoom(t, dm, "private message", "agent"); len(got.Messages) != 2 {
		t.Fatalf("DM did not stop after one reply: %+v", got)
	}
	private := <-inputs
	if len(private) != 2 || private[0].Role != "system" || private[1].Text != "private message" {
		t.Fatalf("private input = %+v", private)
	}
	if got := sendRoom(t, group, "public message", ""); len(got.Messages) != 2 {
		t.Fatalf("group = %+v", got)
	}
	public := <-inputs
	if len(public) != 1 || public[0].Text != "public message" {
		t.Fatalf("DM history leaked into group: %+v", public)
	}
	copy := dm.Snapshot()
	copy.Messages[0].Text = "mutated snapshot"
	if dm.Snapshot().Messages[0].Text != "private message" {
		t.Fatal("snapshot mutated room history")
	}
}

type heldCall struct {
	ctx      context.Context
	messages []Message
	answer   chan Message
}

func heldAdapter() (Adapter, <-chan heldCall) {
	calls := make(chan heldCall, 8)
	return AdapterFunc(func(ctx context.Context, messages []Message) (Message, error) {
		call := heldCall{ctx: ctx, messages: messages, answer: make(chan Message, 1)}
		calls <- call
		// Deliberately ignore cancellation to prove stale replies are rejected.
		return <-call.answer, nil
	}), calls
}

func nextCall(t *testing.T, calls <-chan heldCall) heldCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("agent was not called")
		return heldCall{}
	}
}

func TestRoomNewHumanMessageCancelsAndRejectsOldReply(t *testing.T) {
	adapter, calls := heldAdapter()
	room := New([]Member{{ID: "agent", Enabled: true, Adapter: adapter}}, Options{MaxHops: 1})
	defer room.Close()
	if err := room.Send("first", ""); err != nil {
		t.Fatal(err)
	}
	old := nextCall(t, calls)
	if err := room.Send("second", ""); err != nil {
		t.Fatal(err)
	}
	current := nextCall(t, calls)
	select {
	case <-old.ctx.Done():
	default:
		t.Fatal("new Send did not cancel the older adapter")
	}
	current.answer <- Message{Text: "new answer"}
	got := awaitRoom(t, room, func(s Snapshot) bool { return !s.Busy })
	old.answer <- Message{Text: "stale answer"}
	// Inject the same old completion through the scheduler boundary as well.
	room.accept(reply{generation: 1, hop: 1, member: Member{ID: "agent"}, message: Message{Text: "stale answer"}})
	if len(got.Messages) != 3 || got.Messages[0].Text != "first" || got.Messages[1].Text != "second" || got.Messages[2].Text != "new answer" {
		t.Fatalf("messages = %+v", got.Messages)
	}
	if !reflect.DeepEqual(got.Messages, room.Snapshot().Messages) {
		t.Fatal("old completion changed the latest conversation")
	}
}

func TestRoomStopClearCloseAndDisabledMemberCancel(t *testing.T) {
	for _, action := range []string{"stop", "clear", "close", "disable", "remove", "system"} {
		t.Run(action, func(t *testing.T) {
			adapter, calls := heldAdapter()
			member := Member{ID: "agent", Enabled: true, Adapter: adapter}
			room := New([]Member{member}, Options{})
			defer room.Close()
			if err := room.Send("hello", "agent"); err != nil {
				t.Fatal(err)
			}
			call := nextCall(t, calls)
			switch action {
			case "stop":
				room.Stop()
			case "clear":
				room.Clear()
			case "close":
				room.Close()
			case "disable":
				member.Enabled = false
				room.SetMembers([]Member{member})
			case "remove":
				room.SetMembers(nil)
			case "system":
				if err := room.SetSystemPrompt("new instructions"); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-call.ctx.Done():
			default:
				t.Fatal("operation did not cancel the active adapter")
			}
			call.answer <- Message{Text: "ignored"}
			if got := room.Snapshot(); got.Busy || got.ActiveAgent != "" || action == "clear" && len(got.Messages) != 0 {
				t.Fatalf("cancelled state = %+v", got)
			}
			if action == "close" && !errors.Is(room.Send("later", "agent"), ErrClosed) {
				t.Fatal("closed room accepted a new message")
			}
		})
	}
}

func TestRoomHistorySystemAndLimits(t *testing.T) {
	inputs := make(chan []Message, 4)
	room := New([]Member{{ID: "agent", Enabled: true, Adapter: AdapterFunc(func(_ context.Context, messages []Message) (Message, error) {
		inputs <- messages
		messages[0].Text = "adapter-mutated copy"
		return Message{Text: "answer"}, nil
	})}}, Options{ContextMessages: 2})
	defer room.Close()
	room.SetSystemPrompt("system instructions")
	sendRoom(t, room, "first", "agent")
	<-inputs
	sendRoom(t, room, "second", "agent")
	got := <-inputs
	if len(got) != 3 || got[0].Role != "system" || got[1].Role != "assistant" || got[2].Text != "second" {
		t.Fatalf("context window = %+v", got)
	}
	if room.Snapshot().SystemPrompt != "system instructions" || room.Snapshot().Messages[0].Text != "first" {
		t.Fatal("adapter mutated the independent system/history state")
	}
	if err := room.Send(strings.Repeat("x", MaxMessageBytes+1), "agent"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("large message = %v", err)
	}
	if err := room.SetSystemPrompt(strings.Repeat("x", MaxSystemPromptBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("large system prompt = %v", err)
	}
	if err := room.Send("hello", "missing"); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("missing agent = %v", err)
	}
	room.mu.Lock()
	for range MaxHistoryMessages + 20 {
		room.appendLocked(Message{Text: "bounded history"})
	}
	if len(room.messages) != MaxHistoryMessages {
		t.Fatalf("history size = %d", len(room.messages))
	}
	for range 8 {
		room.appendLocked(Message{Text: strings.Repeat("x", MaxMessageBytes)})
	}
	bytes := 0
	for _, message := range room.messages {
		bytes += len(message.Text)
	}
	room.mu.Unlock()
	if bytes > MaxHistoryBytes {
		t.Fatalf("history bytes = %d", bytes)
	}
}

func TestRoomFailureStopsWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	room := New([]Member{{ID: "agent", Name: "Agent", Enabled: true, Adapter: AdapterFunc(func(context.Context, []Message) (Message, error) {
		calls.Add(1)
		return Message{}, errors.New("failed once")
	})}}, Options{})
	defer room.Close()
	got := sendRoom(t, room, "hello", "")
	if calls.Load() != 1 || got.Error != "Agent: failed once" || len(got.Messages) != 1 {
		t.Fatalf("failure state = %+v, calls = %d", got, calls.Load())
	}
	room.SetMembers([]Member{{ID: "agent", Adapter: answer("disabled"), Enabled: false}})
	if err := room.Send("hello", ""); !errors.Is(err, ErrNoMembers) {
		t.Fatalf("disabled member = %v", err)
	}
}

func TestFormatPromptKeepsSystemNamesAndUTF8Bounds(t *testing.T) {
	messages := []Message{
		{Role: "system", Text: "independent instructions"},
		{Role: "user", Text: "hello"},
		{Role: "assistant", AgentID: "agent", Name: "Code Agent", Hop: 1, Text: "answer"},
	}
	prompt := FormatPrompt(messages)
	for _, part := range []string{"[system]\nindependent instructions", "[user]\nhello", `name="Code Agent" id="agent" hop=1`, "answer"} {
		if !strings.Contains(prompt, part) {
			t.Fatalf("prompt does not include %q: %q", part, prompt)
		}
	}
	for range 10 {
		messages = append(messages, Message{Role: "user", Text: strings.Repeat("😀", MaxMessageBytes/4)})
	}
	prompt = FormatPrompt(messages)
	if len(prompt) > MaxPromptBytes || !utf8.ValidString(prompt) || !strings.HasPrefix(prompt, "[system]") {
		t.Fatalf("prompt bytes = %d, valid = %v", len(prompt), utf8.ValidString(prompt))
	}
}

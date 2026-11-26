package acp

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"
)

func replayUserChunk(text string) sdk.SessionUpdate {
	return sdk.SessionUpdate{UserMessageChunk: &sdk.SessionUpdateUserMessageChunk{Content: TextBlock(text)}}
}

func replayAnswerChunk(id, text string) sdk.SessionUpdate {
	return sdk.SessionUpdate{AgentMessageChunk: &sdk.SessionUpdateAgentMessageChunk{MessageId: &id, Content: TextBlock(text)}}
}

func replayThoughtChunk(id, text string) sdk.SessionUpdate {
	return sdk.SessionUpdate{AgentThoughtChunk: &sdk.SessionUpdateAgentThoughtChunk{MessageId: &id, Content: TextBlock(text)}}
}

func TestACPSessionReplayPreservesDeliveredThought(t *testing.T) {
	var replay sessionReplay
	for _, chunk := range []sdk.SessionUpdate{
		replayUserChunk("context"), replayUserChunk("question"),
		replayThoughtChunk("empty", "\n \t"), replayThoughtChunk("one", "First detail"),
		replayThoughtChunk("one", "\n\n"), replayThoughtChunk("one", "Second detail"),
		replayThoughtChunk("two", "Another summary"), replayAnswerChunk("final", "Answer"),
		replayUserChunk("Follow-up"), replayThoughtChunk("two", "Follow-up detail"),
	} {
		replay.record(chunk)
	}
	want := []ReplayMessage{
		{Role: "user", Text: "contextquestion", FirstChunkBytes: len("context")},
		{Role: "thought", Text: "First detail\n\nSecond detail", FirstChunkBytes: len("First detail")},
		{Role: "thought", Text: "Another summary", FirstChunkBytes: len("Another summary")},
		{Role: "assistant", Text: "Answer", FirstChunkBytes: len("Answer")},
		{Role: "user", Text: "Follow-up", FirstChunkBytes: len("Follow-up")},
		{Role: "thought", Text: "Follow-up detail", FirstChunkBytes: len("Follow-up detail")},
	}
	if replay.partial || !slices.Equal(replay.messages, want) {
		t.Fatalf("history = %+v, partial=%v; want %+v", replay.messages, replay.partial, want)
	}
}

func TestACPSessionReplaySeparatesCodexReasoningParts(t *testing.T) {
	replay := sessionReplay{codexThoughtParts: true}
	replay.record(replayThoughtChunk("reasoning", "First paragraph.\n"))
	replay.record(replayThoughtChunk("reasoning", "Second paragraph."))
	if len(replay.messages) != 1 || replay.messages[0].Text != "First paragraph.\n\n\nSecond paragraph." ||
		replay.thought.bytes != len(replay.messages[0].Text) {
		t.Fatalf("replayed reasoning parts: %+v", replay.messages)
	}
}

func TestACPSessionReplayThoughtsCannotConsumePublicBudget(t *testing.T) {
	for _, test := range []struct {
		name    string
		count   int
		thought string
	}{
		{name: "thought bytes", count: 1, thought: "x" + strings.Repeat("🙂", 25<<10)},
		{name: "thought rows", count: 120, thought: "An update"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var replay sessionReplay
			question := strings.Repeat("x", 95<<10)
			replay.record(replayUserChunk(question))
			replay.record(replayAnswerChunk("public", "Start"))
			for index := range test.count {
				replay.record(replayThoughtChunk(strconv.Itoa(index), test.thought))
			}
			replay.record(replayAnswerChunk("public", " of answer"))
			for index := 2; index < 96; index++ {
				replay.record(replayAnswerChunk("public-"+strconv.Itoa(index), "More text"))
			}
			replay.record(replayAnswerChunk("public-over-budget", "One more reply"))
			var public []ReplayMessage
			thoughtBytes, thoughtRows := 0, 0
			for _, message := range replay.messages {
				if message.Role == "thought" {
					thoughtRows++
					thoughtBytes += len(message.Text)
					if !utf8.ValidString(message.Text) {
						t.Fatal("a bounded thought ended inside a UTF-8 character")
					}
				} else {
					public = append(public, message)
				}
			}
			if !replay.partial || thoughtRows < 1 || thoughtRows > 24 || thoughtBytes > 24<<10 {
				t.Fatalf("thought budget: rows=%d, bytes=%d, partial=%v", thoughtRows, thoughtBytes, replay.partial)
			}
			if len(public) != 96 || public[0].Text != question || public[1].Text != "Start of answer" || public[95].Text != "More text" {
				t.Fatalf("thoughts changed how much public history was kept: %d rows", len(public))
			}
		})
	}
}

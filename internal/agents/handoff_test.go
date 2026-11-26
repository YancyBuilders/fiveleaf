package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func handoffFixture(t *testing.T, harness Harness, transcript string) Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	write(t, path, transcript)
	return Session{Harness: harness, ID: "handoff-test", Transcript: path}
}

func TestHandoffCodexMirrorsAndRepeatedTurns(t *testing.T) {
	session := handoffFixture(t, Codex, `{"type":"session_meta","payload":{"id":"handoff-test","cwd":"/work","source":"cli"}}
{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"Private instructions"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /work\n\nRules"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>details</environment_context>"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Fix login"}]}}
{"type":"event_msg","payload":{"type":"user_message","message":"Fix login"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","channel":"analysis","content":[{"type":"output_text","text":"Private reasoning"}]}}
{"type":"event_msg","payload":{"type":"agent_message","message":"Done"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","channel":"final","content":[{"type":"output_text","text":"Done"}]}}
{"type":"event_msg","payload":{"type":"user_message","message":"Continue"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Continue"}]}}
{"type":"event_msg","payload":{"type":"agent_message","message":"Ready"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Ready"}]}}
{"type":"event_msg","payload":{"type":"user_message","message":"Continue"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Continue"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Ready"}]}}
{"type":"event_msg","payload":{"type":"agent_message","message":"Ready"}}
`)
	got, err := ReadHandoff(session)
	want := []HandoffMessage{
		{Role: "user", Text: "Fix login"}, {Role: "assistant", Text: "Done"},
		{Role: "user", Text: "Continue"}, {Role: "assistant", Text: "Ready"},
		{Role: "user", Text: "Continue"}, {Role: "assistant", Text: "Ready"},
	}
	if err != nil || got.Truncated || !slices.Equal(got.Messages, want) {
		t.Fatalf("history = %#v, %v; want %#v without truncation", got, err, want)
	}
}

func TestHandoffCodexKeepsPublicAssistantPhase(t *testing.T) {
	const records = `{"type":"response_item","payload":{"type":"message","role":"user","content":"Question"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":"First update"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"First update"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"Second update"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":"Second update"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"analysis","channel":"analysis","content":"Private thought"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"unrecognized","content":"Plain output"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"final_answer","content":"Answer"}}`
	got, err := ReadHandoff(handoffFixture(t, Codex, records))
	want := []HandoffMessage{
		{Role: "user", Text: "Question"},
		{Role: "assistant", Text: "First update", Phase: "commentary"},
		{Role: "assistant", Text: "Second update", Phase: "commentary"},
		{Role: "assistant", Text: "Plain output"},
		{Role: "assistant", Text: "Answer", Phase: "final_answer"},
	}
	if err != nil || got.Truncated || !slices.Equal(got.Messages, want) {
		t.Fatalf("history = %#v, %v; want %#v without truncation", got, err, want)
	}
}

func TestHandoffDecodesFirstFiveLeafCodexPrompt(t *testing.T) {
	const legacyRecord = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"[user]\n  Original question\n\n[user]\nQuoted text\n\n\n"}]}}`
	const legacyEvent = `{"type":"event_msg","payload":{"type":"user_message","message":"[user]\n  Original question\n\n[user]\nQuoted text\n\n\n"}}`
	const legacyBody = "  Original question\n\n[user]\nQuoted text\n"
	const markedRecord = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"FiveLeaf conversation context\n\n"},{"type":"input_text","text":"  Check the logs\n\n"}]}}`
	const markedEvent = `{"type":"event_msg","payload":{"type":"user_message","message":"FiveLeaf conversation context\n\n  Check the logs\n\n"}}`
	for _, test := range []struct {
		name, originator, records, first string
	}{
		{"LegacyResponseFirst", "fiveleaf", legacyRecord + "\n" + legacyEvent, legacyBody},
		{"LegacyEventFirst", "fiveleaf", legacyEvent + "\n" + legacyRecord, legacyBody},
		{"LegacyEventOnly", "fiveleaf", legacyEvent, legacyBody},
		{"ContextAndQuestion", "fiveleaf", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"FiveLeaf conversation context\n\n[system]\nEarlier instructions\n\n[user]\nEarlier question\n\n"},{"type":"input_text","text":"  当前问题\n\n[user]\n引用\n\n"}]}}`, "  当前问题\n\n[user]\n引用\n\n"},
		{"ContextHeaderOnly", "fiveleaf", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"FiveLeaf conversation context\n\n"},{"type":"input_text","text":"[user]\nLiteral tags\n\n"}]}}`, "[user]\nLiteral tags\n\n"},
		{"ContextResponseThenEvent", "fiveleaf", markedRecord + "\n" + markedEvent, "  Check the logs\n\n"},
		{"LiteralInstructions", "fiveleaf", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"FiveLeaf conversation context\n\n"},{"type":"input_text","text":"# AGENTS.md instructions\nReview these instructions"}]}}`, "# AGENTS.md instructions\nReview these instructions"},
		{"PlainFirst", "fiveleaf", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":" \n  A plain question\n  "}]}}`, " \n  A plain question\n  "},
		{"OtherOriginator", "Codex Desktop", legacyRecord + "\n" + legacyEvent, "[user]\n  Original question\n\n[user]\nQuoted text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const later = `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Answer"}]}}
{"type":"event_msg","payload":{"type":"user_message","message":"[user]\nLiteral follow-up\n\n"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"[user]\nLiteral follow-up\n\n"}]}}
`
			data := `{"type":"session_meta","payload":{"id":"handoff-test","cwd":"/work","originator":"` + test.originator + `"}}` + "\n" +
				`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>details</environment_context>"}]}}` + "\n" +
				test.records + "\n" + later
			fixture := handoffFixture(t, Codex, data)
			session, visible := readCodex(fixture.Transcript)
			if !visible {
				t.Fatal("Codex session should be visible")
			}
			session.Transcript = fixture.Transcript
			followUp := "[user]\nLiteral follow-up"
			if test.originator == "fiveleaf" {
				followUp += "\n\n"
			}
			want := []HandoffMessage{{Role: "user", Text: test.first}, {Role: "assistant", Text: "Answer"}, {Role: "user", Text: followUp}}
			for _, source := range []Session{fixture, session} {
				got, err := ReadHandoff(source)
				if err != nil || got.Truncated || !slices.Equal(got.Messages, want) {
					t.Fatalf("history from originator %q = %#v, %v; want %#v without truncation", source.Originator, got, err, want)
				}
			}
			if session.Prompt != oneLine(test.first) {
				t.Fatalf("session prompt = %q; want %q", session.Prompt, oneLine(test.first))
			}
			if session.Originator != test.originator {
				t.Fatalf("originator = %q; want %q", session.Originator, test.originator)
			}
		})
	}
}

func TestHandoffCodexOriginatorMatchesOuterSession(t *testing.T) {
	const records = `{"type":"session_meta","payload":{"id":"handoff-test","originator":"Codex Desktop"}}
{"type":"session_meta","payload":{"id":"inherited","originator":"fiveleaf"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"[user]\nLiteral question\n\n"}]}}`
	session := handoffFixture(t, Codex, records)
	session.ID = "inherited"
	got, err := ReadHandoff(session)
	want := []HandoffMessage{{Role: "user", Text: "[user]\nLiteral question"}}
	if err != nil || !slices.Equal(got.Messages, want) {
		t.Fatalf("using unmatched metadata = %#v, %v; want %#v", got, err, want)
	}
}

func TestHandoffReadableMessages(t *testing.T) {
	for _, test := range []struct {
		name    string
		harness Harness
		data    string
	}{
		{"Claude", Claude, `{"type":"user","isMeta":true,"message":{"role":"user","content":"Injected context"}}
{"type":"user","isSidechain":true,"message":{"role":"user","content":"Helper task"}}
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"<environment_context>cwd</environment_context>"},{"type":"text","text":"Fix it"},{"type":"tool_result","content":"tool output"}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning"},{"type":"tool_use","name":"read"},{"type":"text","text":"Fixed it"}]}}
`},
		{"Qoder", Qoder, `{"type":"user","message":{"content":"Fix it"}}
{"type":"assistant","message":{"content":"Fixed it"}}
`},
		{"Pi", "pi", `{"type":"session","id":"session-id","cwd":"/work"}
{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":[{"type":"text","text":"Fix it"}]}}
{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning"},{"type":"text","text":"Fixed it"}]}}
{"type":"message","id":"tool1","parentId":"a1","message":{"role":"toolResult","content":[{"type":"text","text":"tool output"}]}}
`},
		{"Grok", Grok, `{"role":"system","content":"system prompt"}
{"role":"user","content":"Fix it"}
{"role":"tool","content":"tool output"}
{"role":"assistant","content":"Fixed it"}
`},
		{"PluginJSON", "custom", `{"messages":[
{"role":"system","content":"system prompt"},
{"role":"user","content":"Fix it"},
{"role":"assistant","content":"Fixed it"}]}`},
		{"PluginJSONL", "custom", `{"type":"user","text":"Fix it"}
{"type":"assistant","text":"Fixed it"}
`},
		{"EventOnly", Codex, `{"type":"event_msg","payload":{"type":"user_message","message":"Fix it"}}
{"type":"event_msg","payload":{"type":"agent_reasoning","text":"reasoning"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"Fixed it"}}
`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ReadHandoff(handoffFixture(t, test.harness, test.data))
			want := []HandoffMessage{{Role: "user", Text: "Fix it"}, {Role: "assistant", Text: "Fixed it"}}
			if err != nil || got.Truncated || !slices.Equal(got.Messages, want) {
				t.Fatalf("history = %#v, %v; want %#v", got, err, want)
			}
		})
	}
}

func TestHandoffCodexMixedRecordsKeepDistinctTurns(t *testing.T) {
	session := handoffFixture(t, Codex, `{"type":"event_msg","payload":{"type":"user_message","message":"Continue"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"Done"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Continue"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done"}]}}
`)
	got, err := ReadHandoff(session)
	want := []HandoffMessage{
		{Role: "user", Text: "Continue"}, {Role: "assistant", Text: "Done"},
		{Role: "user", Text: "Continue"}, {Role: "assistant", Text: "Done"},
	}
	if err != nil || !slices.Equal(got.Messages, want) {
		t.Fatalf("history = %#v, %v; want %#v", got, err, want)
	}
}

func TestHandoffPiKeepsActiveBranch(t *testing.T) {
	session := handoffFixture(t, "pi", `{"type":"session","id":"session-id","cwd":"/work"}
{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":"Fix login"}}
{"type":"message","id":"old","parentId":"u1","message":{"role":"assistant","content":"Abandoned approach"}}
{"type":"message","id":"new","parentId":"u1","message":{"role":"assistant","content":"Current approach"}}
{"type":"session_info","id":"label","parentId":"new","name":"Login"}
`)
	got, err := ReadHandoff(session)
	want := []HandoffMessage{{Role: "user", Text: "Fix login"}, {Role: "assistant", Text: "Current approach"}}
	if err != nil || !slices.Equal(got.Messages, want) {
		t.Fatalf("history = %#v, %v; want %#v", got, err, want)
	}
}

func TestHandoffExtractsAttachedFileRequest(t *testing.T) {
	session := handoffFixture(t, Codex, `{"role":"user","content":"# Files mentioned by the user:\n\n## screenshot.png: /private/screenshot.png\n\n## My request:\nFix the menu"}`)
	got, err := ReadHandoff(session)
	if err != nil || len(got.Messages) != 1 || got.Messages[0].Text != "Fix the menu" {
		t.Fatalf("request = %#v, %v", got, err)
	}
}

func TestHandoffLargeTranscriptKeepsGoalAndLatest(t *testing.T) {
	data := `{"role":"user","content":"Original goal"}` + "\n" +
		strings.Repeat(`{"role":"tool","content":"ignored output"}`+"\n", handoffReadLimit/25) +
		`{"role":"user","content":"Latest follow-up"}` + "\n" +
		`{"role":"assistant","content":"Latest progress"}` + "\n"
	got, err := ReadHandoff(handoffFixture(t, Codex, data))
	want := []HandoffMessage{{Role: "user", Text: "Original goal"}, {Role: "user", Text: "Latest follow-up"}, {Role: "assistant", Text: "Latest progress"}}
	if err != nil || !got.Truncated || !slices.Equal(got.Messages, want) {
		t.Fatalf("history = %#v, %v; want %#v with truncation", got, err, want)
	}
}

func TestHandoffOversizedLineAndMalformedRecords(t *testing.T) {
	data := `{"role":"user","content":"Original goal"}` + "\n" +
		`{"role":"assistant","content":"` + strings.Repeat("x", handoffReadLimit+100) + `"}` + "\n" +
		"not JSON\n" + `{"role":"assistant","content":"Latest progress"}`
	got, err := ReadHandoff(handoffFixture(t, Codex, data))
	want := []HandoffMessage{{Role: "user", Text: "Original goal"}, {Role: "assistant", Text: "Latest progress"}}
	if err != nil || !got.Truncated || !slices.Equal(got.Messages, want) {
		t.Fatalf("history = %#v, %v; want %#v with truncation", got, err, want)
	}
}

func TestHandoffTailAtLineBoundaryKeepsFirstMessage(t *testing.T) {
	goal := `{"role":"user","content":"Original goal"}` + "\n"
	latest := `{"role":"user","content":"Latest follow-up"}` + "\n"
	progress := `{"role":"assistant","content":"Latest progress"}` + "\n"
	tail := latest + strings.Repeat("\n", handoffReadLimit/2-len(latest)-len(progress)) + progress
	data := goal + strings.Repeat("\n", handoffReadLimit) + tail
	got, err := ReadHandoff(handoffFixture(t, Codex, data))
	want := []HandoffMessage{{Role: "user", Text: "Original goal"}, {Role: "user", Text: "Latest follow-up"}, {Role: "assistant", Text: "Latest progress"}}
	if err != nil || !got.Truncated || !slices.Equal(got.Messages, want) {
		t.Fatalf("history = %#v, %v; want %#v", got, err, want)
	}
}

func TestHandoffBoundsUTF8AndRecentMessages(t *testing.T) {
	var data strings.Builder
	first, _ := json.Marshal(map[string]string{"role": "user", "content": "最初目标" + strings.Repeat("任务", handoffMessageText)})
	data.Write(first)
	data.WriteByte('\n')
	for index := 0; index < handoffMessageLimit+20; index++ {
		message, _ := json.Marshal(map[string]string{"role": "assistant", "content": strings.Repeat("最近进度🙂", 1000)})
		data.Write(message)
		data.WriteByte('\n')
	}
	data.WriteString(`{"role":"assistant","content":"最后进度"}`)
	got, err := ReadHandoff(handoffFixture(t, Claude, data.String()))
	if err != nil || !got.Truncated || len(got.Messages) == 0 || len(got.Messages) > handoffMessageLimit {
		t.Fatalf("history count = %d, truncated = %v, error = %v", len(got.Messages), got.Truncated, err)
	}
	if !strings.HasPrefix(got.Messages[0].Text, "最初目标") || got.Messages[len(got.Messages)-1].Text != "最后进度" {
		t.Fatal("the goal or final progress was lost")
	}
	total := 0
	for _, message := range got.Messages {
		total += len(message.Text)
		if !utf8.ValidString(message.Text) || len(message.Text) > handoffMessageText {
			t.Fatalf("invalid or oversized message: %d bytes", len(message.Text))
		}
	}
	if total > handoffTextLimit {
		t.Fatalf("history has %d bytes; limit = %d", total, handoffTextLimit)
	}
}

func TestHandoffMissingAndUnreadableHistory(t *testing.T) {
	for _, test := range []struct {
		name    string
		session Session
	}{
		{"NoTranscript", Session{ID: "x", Title: "A title cannot replace history"}},
		{"MissingFile", Session{Transcript: filepath.Join(t.TempDir(), "missing")}},
		{"Directory", Session{Transcript: t.TempDir()}},
		{"Malformed", handoffFixture(t, Codex, "malformed JSON")},
		{"MetadataOnly", handoffFixture(t, Codex, `{"type":"session_meta","payload":{"id":"x"}}`)},
		{"SystemOnly", handoffFixture(t, Codex, `{"role":"system","content":"Instructions"}`)},
		{"Empty", handoffFixture(t, Codex, "")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ReadHandoff(test.session)
			if err == nil || len(got.Messages) != 0 {
				t.Fatalf("history = %#v, %v; want an error", got, err)
			}
		})
	}
}

func TestHandoffDeepContainersAreBounded(t *testing.T) {
	data := strings.Repeat(`{"messages":[`, 100) + `{"role":"user","content":"Nested"}` + strings.Repeat("]}", 100)
	got, err := ReadHandoff(handoffFixture(t, "custom", data))
	if err == nil || !got.Truncated || len(got.Messages) != 0 {
		t.Fatalf("history = %#v, %v; want bounded nesting error", got, err)
	}
}

func TestHandoffDoesNotModifyTranscript(t *testing.T) {
	data := `{"role":"user","content":"Continue task"}`
	session := handoffFixture(t, Claude, data)
	if _, err := ReadHandoff(session); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(session.Transcript)
	if err != nil || string(after) != data {
		t.Fatal("handoff changed its source transcript")
	}
}

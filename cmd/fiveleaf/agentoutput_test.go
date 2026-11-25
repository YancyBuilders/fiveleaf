package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func agentOutputUpdate(t *testing.T, hs *harness, fields map[string]any) {
	t.Helper()
	chat := hs.a.agent
	var update acp.Update
	copyAgentJSON(map[string]any{"sessionId": chat.sessionID, "update": fields}, &update)
	h := agentHandler{app: hs.a, chat: chat, root: hs.a.root, generation: chat.generation}
	h.Update(update)
	hs.settle()
}

func agentOutputChunk(t *testing.T, hs *harness, kind, text string) {
	t.Helper()
	agentOutputUpdate(t, hs, map[string]any{"sessionUpdate": kind, "content": map[string]string{"type": "text", "text": text}})
}

func TestAgentOutputKeepsToolCallsInOrder(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.sessionID, chat.busy = "output-session", true
	chat.messages = []*agentMessage{{role: "user", text: "Check this project"}}
	agentOutputChunk(t, hs, "agent_message_chunk", "I will inspect the project.")
	agentOutputUpdate(t, hs, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "read-1", "title": "Read main.go", "kind": "read", "status": "in_progress"})
	agentOutputChunk(t, hs, "agent_message_chunk", "The entry point is small.")
	agentOutputUpdate(t, hs, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "read-1", "status": "completed", "content": []any{map[string]any{"type": "content", "content": map[string]string{"type": "text", "text": "package main"}}}})
	chat.busy = false
	hs.settle()
	if len(chat.messages) != 4 || len(chat.tools) != 1 || chat.messages[2].tool != chat.tools[0] || chat.messages[2].role != "tool" {
		t.Fatal("tool updates duplicated their entry or lost the original event order")
	}
	if chat.messages[1].text != "I will inspect the project." || chat.messages[3].text != "The entry point is small." {
		t.Fatal("assistant chunks on opposite sides of a tool call were merged")
	}
	context := chatContext(chat)
	if len(context) != 2 || context[1].Text != chat.messages[1].text+chat.messages[3].text {
		t.Fatalf("rendering tool entries changed the next prompt's text context: %+v", context)
	}
	first, okFirst := hs.tt.Find(chat.messages[1].text)
	tool, okTool := hs.tt.Find("Read main.go")
	last, okLast := hs.tt.Find(chat.messages[3].text)
	if !okFirst || !okTool || !okLast || first.Y >= tool.Y || tool.Y >= last.Y {
		t.Fatalf("visible transcript is out of order: before=%+v tool=%+v after=%+v", first, tool, last)
	}
	if err := hs.tt.Click("Read main.go"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText("package main") {
		t.Fatal("the original tool entry did not receive its completed output")
	}
}

func TestAgentOutputToolsKeepContextBudget(t *testing.T) {
	chat := &agentChat{contextMessages: 3, messages: []*agentMessage{{role: "user", text: "Keep the requested constraints."}}}
	var reply strings.Builder
	for i := range 12 {
		text := fmt.Sprintf("Step %d is complete.\n", i)
		reply.WriteString(text)
		chat.messages = append(chat.messages,
			&agentMessage{role: "tool", tool: &agentTool{text: "Internal tool output."}},
			&agentMessage{role: "assistant", text: text})
	}
	chat.messages = append(chat.messages, &agentMessage{role: "user", text: "Continue with the same constraints."})
	context := chatContext(chat)
	if len(context) != 3 || context[0].Text != chat.messages[0].text || context[1].Text != reply.String() || context[2].Text != "Continue with the same constraints." {
		t.Fatalf("display-only tool entries consumed the public context budget: %+v", context)
	}
	chat.contextMessages = 2
	context = chatContext(chat)
	if len(context) != 2 || context[0].Text != reply.String() || context[1].Role != "user" {
		t.Fatalf("public message budget no longer limits complete replies: %+v", context)
	}
}

func TestAgentThoughtFoldsAfterReplyAndKeepsManualChoice(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.sessionID, chat.busy = "thought-session", true
	agentOutputChunk(t, hs, "agent_thought_chunk", "Reviewing the approach.\n\nPrivate reasoning details.")
	thought := chat.messages[0]
	if !hs.tt.HasText("Private reasoning details.") {
		t.Fatal("active thinking did not expand while streaming")
	}
	agentOutputChunk(t, hs, "agent_message_chunk", "Here is the answer.")
	if hs.tt.HasText("Private reasoning details.") || !hs.tt.HasText("Here is the answer.") {
		t.Fatal("finished thinking did not fold when the public reply began")
	}
	if err := hs.tt.Click(locales.AgentThought()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !thought.expansionSet || !thought.expanded || !hs.tt.HasText("Private reasoning details.") {
		t.Fatal("thinking cannot be expanded manually")
	}
	agentOutputChunk(t, hs, "agent_message_chunk", " Another detail.")
	hs.a.newFile()
	hs.settle()
	hs.a.openChatPage(chat)
	hs.settle()
	if !hs.tt.HasText("Private reasoning details.") || !thought.expanded {
		t.Fatal("later output or moving the conversation discarded the manual expansion")
	}
	for _, message := range chatContext(chat) {
		if strings.Contains(message.Text, "Private reasoning") {
			t.Fatal("thinking was included in public recent-message context")
		}
	}
}

func TestAgentThoughtManualCollapseStaysClosedDuringStream(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	hs.a.agent.sessionID, hs.a.agent.busy = "thought", true
	agentOutputChunk(t, hs, "agent_thought_chunk", "Considering.\n\nReasoning body.")
	if err := hs.tt.Click(locales.AgentThought()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	agentOutputChunk(t, hs, "agent_thought_chunk", " More reasoning.")
	if hs.tt.HasText("Reasoning body.") || hs.a.agent.messages[0].expanded {
		t.Fatal("another streamed thinking chunk reopened a manually folded block")
	}
}

func TestAgentOutputCopyPreservesCompleteMarkdown(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	text := "# Result\n\n```go\nfmt.Println(\"你好\")\n```\n" + strings.Repeat("Full output beyond the rendered limit.\n", 1000)
	hs.a.agent.messages = []*agentMessage{{role: "assistant", text: text}}
	hs.settle()
	hs.a.agent.scroll.Y = 0
	hs.settle()
	if err := hs.tt.Click(locales.AgentCopyMessage()); err != nil {
		t.Fatal(err)
	}
	if got := hs.tt.Clipboard(); got != text {
		t.Fatalf("message copy changed Markdown or lost clipped output: copied %d of %d bytes", len(got), len(text))
	}
}

func TestAgentOutputMessageAlignment(t *testing.T) {
	for _, width := range []int{280, 760} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			app := &app{settings: settings.Default(), pal: darkPalette().derive()}
			chat := &agentChat{messages: []*agentMessage{
				{role: "user", text: "收到。"},
				{role: "assistant", text: "下一步只更新变更的文件。"},
				{role: "user", text: strings.Repeat("请保留上下文，并检查每条消息是否在当前聊天页内换行。", 6)},
			}}
			tester := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Gap(14).Children(func() {
					for index, message := range chat.messages {
						ui.Column(c.Key(index)).FillWidth().Children(func() { app.agentMessageView(c, chat, message, false) })
					}
				})
			}, width, 540)
			question, foundQuestion := tester.Find(chat.messages[0].text)
			reply, foundReply := tester.Find(chat.messages[1].text)
			long, foundLong := tester.Find(chat.messages[2].text)
			if !foundQuestion || !foundReply || !foundLong || question.X < float32(width)/2 || reply.X > 2 || long.X >= question.X || long.X < float32(width)/10 || long.H <= question.H ||
				math.Abs(float64((question.X+question.W)-(long.X+long.W))) > 1 || long.X+long.W > float32(width) {
				t.Fatalf("unexpected message layout at width %d: short=%+v reply=%+v long=%+v", width, question, reply, long)
			}
			for _, name := range []string{locales.AgentYou(), locales.AgentTitle()} {
				if _, found := tester.Find(name); found {
					t.Fatalf("redundant sender heading %q", name)
				}
			}
			tester.Move(question.X+question.W/2, question.Y+question.H/2)
			if after, _ := tester.Find(chat.messages[0].text); after != question {
				t.Fatalf("revealing copy moved the message: %+v -> %+v", question, after)
			}
			if err := tester.Click(locales.AgentCopyMessage()); err != nil || tester.Clipboard() != chat.messages[0].text {
				t.Fatalf("could not copy the outgoing message: %v, %q", err, tester.Clipboard())
			}
		})
	}
}

func TestAgentOutputDetailsAndLatestKeepReadingPosition(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	thought := &agentMessage{role: "thought", text: "Reasoning summary.\n\n" + strings.Repeat("Details that stay available.\n\n", 10)}
	reply := &agentMessage{role: "assistant", text: strings.Repeat("A public answer paragraph.\n\n", 50)}
	chat.messages = []*agentMessage{thought, reply}
	hs.settle()
	chat.scroll.Y = 0
	hs.settle()
	if err := hs.tt.Click(locales.AgentThought()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if chat.scroll.Y != 0 {
		t.Fatal("expanding earlier details pulled the reader to the end")
	}
	reply.text += "More incoming output.\n\n"
	hs.settle()
	if chat.scroll.Y != 0 {
		t.Fatal("new output pulled the reader away from expanded details")
	}
	if err := hs.tt.Click(locales.AgentJumpLatest()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if chat.scroll.Y < chat.scroll.MaxY-1 || hs.tt.HasText(locales.AgentJumpLatest()) {
		t.Fatalf("jump to latest did not resume following: %v / %v", chat.scroll.Y, chat.scroll.MaxY)
	}
	reply.text += strings.Repeat("A later answer.\n\n", 5)
	hs.settle()
	if chat.scroll.Y < chat.scroll.MaxY-1 {
		t.Fatal("output stopped following after jumping to the latest reply")
	}
}

func TestAgentOutputStreamCompletionKeepsReadingPosition(t *testing.T) {
	for _, follow := range []bool{false, true} {
		t.Run(fmt.Sprintf("follow-%v", follow), func(t *testing.T) {
			hs := newHarness(t, true)
			agentViewFixture(hs)
			chat := hs.a.agent
			chat.busy = true
			chat.messages = []*agentMessage{{role: "assistant", text: strings.Repeat("Earlier paragraph.\n\n", 40) + "| " + strings.Repeat("pending table row ", 100)}}
			hs.settle()
			if !follow {
				chat.scroll.Y = 0
				hs.settle()
			}
			// Completing a stream reveals its pending table candidate even
			// when the last update contains no additional text.
			beforeMax := chat.scroll.MaxY
			chat.busy = false
			hs.settle()
			if chat.scroll.MaxY <= beforeMax {
				t.Fatal("the pending Markdown tail did not become visible")
			}
			if follow && chat.scroll.Y < chat.scroll.MaxY-1 || !follow && chat.scroll.Y != 0 {
				t.Fatalf("completing a stream changed reading intent: follow=%v, scroll=%v/%v", follow, chat.scroll.Y, chat.scroll.MaxY)
			}
		})
	}
}

func TestShotAgentRichOutput(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	for _, language := range []string{"en", "ja", "zh-Hans"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-dark-%v", language, dark), func(t *testing.T) {
				before := i18n.Locale()
				i18n.SetLocaleFunc(func() string { return language })
				t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
				hs := newHarness(t, dark)
				hs.tt.SetLocale(language)
				agentRichOutputFixture(hs)
				hs.a.openAgentPage()
				hs.settle()
				hs.a.agent.scroll.Y = math.MaxFloat32
				hs.settle()
				hs.shot(fmt.Sprintf("agent-rich-page-%s-dark-%v", language, dark))
				sizeChatWorkspace(hs, 340)
				hs.settle()
				hs.shot(fmt.Sprintf("agent-rich-narrow-page-%s-dark-%v", language, dark))
			})
		}
	}
}

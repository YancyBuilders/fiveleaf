package main

import (
	"fmt"
	"image"
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

func TestAgentACPCompletedToolDetailsAndReasoning(t *testing.T) {
	hs, _, _ := lifecycleHarness(t, "codex-completed-tool-details")
	app, chat := hs.a, hs.a.agent
	app.openAgentPage()
	hs.untilLong("Codex output fixture connection", func() bool { return chat.sessionID != "" })
	lifecycleSend(t, hs, "Check both fixture results")
	const thought = "The second result confirms the first."
	const commandText = "command result: 检查通过\n"
	const mcpText = "MCP lookup returned a match"
	if chat.lastTask != agentTaskCompleted || len(chat.tools) != 2 ||
		chat.tools[0].outputText() != commandText || !strings.Contains(chat.tools[1].outputText(), mcpText) {
		t.Fatalf("completed turn lost details: state=%v, tools=%d", chat.lastTask, len(chat.tools))
	}
	if chat.scroll.MaxY <= 100 || chat.scroll.Y < chat.scroll.MaxY-1 || hs.tt.HasText(thought) {
		t.Fatalf("expected the long completed reply to fill the viewport: scroll=%v/%v", chat.scroll.Y, chat.scroll.MaxY)
	}
	hs.shot("agent-completed-reply-reasoning-link")
	if err := hs.tt.Click(locales.AgentReviewReasoning()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText(thought) || !hs.tt.HasText("The lookup is consistent with the command output.") || chat.scroll.Y >= chat.scroll.MaxY-10 {
		t.Fatal("the reasoning link did not open the earlier content in view")
	}
	hs.clickDetail(locales.AgentToolCalls())
	hs.clickDetail("Run fixture")
	if !hs.tt.HasText(commandText) || !hs.tt.HasText("printf fixture") {
		t.Fatal("the completed command card did not show the input and streamed output")
	}
	hs.clickDetail(locales.AgentCopyToolOutput() + ": Run fixture")
	if hs.tt.Clipboard() != commandText {
		t.Fatalf("command output copy changed its fragments: %q", hs.tt.Clipboard())
	}
	hs.clickDetail("mcp.fixture.lookup")
	if !hs.tt.HasText("fixture request") || hs.tt.HasText("not a displayed parameter") || hs.tt.HasText("waiting for lookup") ||
		!strings.Contains(strings.Join(hs.tt.Texts(), "\n"), mcpText) {
		t.Fatal("the completed MCP card did not show the final result and useful request details")
	}
	hs.shot("agent-completed-tool-and-reasoning-expanded")
}

func TestAgentOutputToolPatchPreservesAndClearsProvidedLists(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.sessionID = "tool-patches"
	agentOutputUpdate(t, hs, map[string]any{
		"sessionUpdate": "tool_call", "toolCallId": "lookup", "title": "Inspect fixture", "status": "in_progress",
		"content": []any{
			map[string]any{"type": "content", "content": map[string]string{"type": "resource_link", "uri": "DATA:image/png;base64,fixture123", "name": "fixture image"}},
			map[string]any{"type": "content", "content": map[string]string{"type": "text", "text": "initial content"}},
		},
		"locations": []any{map[string]any{"path": hs.path("main.go"), "line": 3}},
	})
	agentOutputUpdate(t, hs, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "lookup", "status": "completed"})
	tool := chat.tools[0]
	if tool.outputText() != "fixture image\ninitial content\n" || len(tool.locations) != 1 || tool.status != "completed" || len(tool.sources.content) != 0 {
		t.Fatal("a status-only patch replaced the tool details")
	}
	agentOutputUpdate(t, hs, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "lookup", "content": []any{}, "locations": []any{}})
	if len(chat.tools) != 1 || tool.outputText() != "" || len(tool.locations) != 0 {
		t.Fatal("explicit empty lists did not clear the original tool card")
	}
}

func TestAgentOutputCapsStreamAtUTF8Boundary(t *testing.T) {
	prefix := strings.Repeat("x", agentTextLimit-2)
	var output agentToolOutputBuffer
	output.append(prefix)
	output.append("🧪")
	output.append("ok")
	if output.String() != prefix {
		t.Fatal("truncation should retain a contiguous prefix across later output fragments")
	}
}

func TestAgentOutputSeparatesProgressFromFinal(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.busy = true
	progress := &agentMessage{role: "commentary", text: "Inspecting the project structure."}
	chat.messages = []*agentMessage{{role: "user", text: "Find the source of this error."}, progress, {role: "thought", text: " \t\n"}}
	hs.settle()
	if !hs.tt.HasText(locales.AgentReplyProgress()) || hs.tt.HasText(locales.AgentFinalReply()) || hs.tt.HasText(locales.AgentThought()) {
		t.Fatal("an unfinished update should show progress without empty thought or final sections")
	}
	thought := &agentMessage{role: "thought", text: "Check the exception first.\n\nInspect the fallback path."}
	chat.messages = append(chat.messages, thought, &agentMessage{role: "thought", text: "\u3000"})
	hs.settle()
	if !hs.tt.HasText("Inspect the fallback path.") {
		t.Fatal("an empty update closed active reasoning")
	}
	reply := &agentMessage{role: "assistant", text: "The fallback path produced the error."}
	chat.messages = append(chat.messages, reply)
	hs.settle()
	heading, hasHeading := hs.tt.Find(locales.AgentFinalReply())
	answer, hasAnswer := hs.tt.Find(reply.text)
	update, hasUpdate := hs.tt.Find(progress.text)
	if !hasHeading || !hasAnswer || !hasUpdate || update.Y >= heading.Y || heading.Y >= answer.Y || answer.X >= update.X || hs.tt.HasText("Inspect the fallback path.") {
		t.Fatalf("progress, reasoning and final reply are not separated: progress=%+v final=%+v answer=%+v", update, heading, answer)
	}
	if err := hs.tt.Click(locales.AgentThought()); err != nil {
		t.Fatal(err)
	}
	if !hs.tt.HasText("Inspect the fallback path.") {
		t.Fatal("finished reasoning lost its details")
	}
}

func TestAgentOutputGroupsOnlyAdjacentToolCalls(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	first := &agentTool{title: "Search repository", kind: "search", status: "completed", text: "Search results"}
	second := &agentTool{title: "Read main.go", kind: "read", status: "completed", text: "package main"}
	last := &agentTool{title: "Read README.md", kind: "read", status: "completed", text: "README"}
	update := &agentMessage{role: "commentary", text: "The entry point is located."}
	chat.tools = []*agentTool{first, second, last}
	chat.messages = []*agentMessage{{role: "tool", tool: first}, {role: "thought", text: "\n "}, {role: "tool", tool: second}, update, {role: "tool", tool: last}}
	hs.settle()
	group := locales.AgentToolCalls()
	groupBounds := hs.detailBounds(group)
	if !hs.tt.HasText(locales.AgentToolCallsCount(2)) || hs.tt.HasText(first.title) || hs.tt.HasText(second.title) || hs.tt.HasText(locales.AgentThought()) {
		t.Fatal("completed adjacent calls should collapse into one group across an empty thought")
	}
	updateBounds, lastBounds := hs.detailBounds(update.text), hs.detailBounds(last.title)
	if groupBounds.Y >= updateBounds.Y || updateBounds.Y >= lastBounds.Y {
		t.Fatal("grouping crossed a public update or changed tool order")
	}
	hs.clickDetail(group)
	firstBounds, secondBounds := hs.detailBounds(first.title), hs.detailBounds(second.title)
	if firstBounds.Y >= secondBounds.Y || secondBounds.Y >= hs.detailBounds(update.text).Y {
		t.Fatal("unfolding tools changed their original order")
	}
	hs.clickDetail(second.title)
	if !hs.tt.HasText(second.text) {
		t.Fatal("unfolding a tool group lost the individual disclosure")
	}
	hs.clickDetail(locales.AgentCopyToolOutput() + ": " + second.title)
	if hs.tt.Clipboard() != second.text {
		t.Fatal("tool copy changed its output")
	}
}

func TestAgentOutputLabelsCodexReplyAfterToolOnlyActivity(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	tool := &agentTool{title: "Read main.go", kind: "read", status: "completed"}
	chat.messages = []*agentMessage{{role: "user", text: "Check main.go"}, {role: "tool", tool: tool}, {role: "assistant", text: "Found the entry point."}}
	hs.settle()
	if hs.tt.HasText(locales.AgentFinalReply()) {
		t.Fatal("an unclassified provider's normal reply was marked final")
	}
	chat.adapterName = "@agentclientprotocol/codex-acp"
	hs.settle()
	if !hs.tt.HasText(locales.AgentFinalReply()) {
		t.Fatal("a completed Codex reply following tools lacked a final answer label")
	}
	chat.messages = []*agentMessage{{role: "user", text: "Tell me the date"}, {role: "assistant", text: "Here is the date."}}
	hs.settle()
	if hs.tt.HasText(locales.AgentFinalReply()) {
		t.Fatal("a plain reply gained a redundant label")
	}
}

func TestAgentOutputGroupedToolFailureStaysActionable(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.busy = true
	first := &agentTool{title: "Run search", kind: "execute", status: "completed", text: "Long result from search"}
	last := &agentTool{title: "Update main.go", kind: "edit", status: "running"}
	chat.tools = []*agentTool{first, last}
	chat.messages = []*agentMessage{{role: "tool", tool: first}, {role: "tool", tool: last}}
	hs.settle()
	if !hs.tt.HasText(last.title) || !hs.tt.HasText(locales.AgentStatusRunning()) || hs.tt.HasText(first.text) {
		t.Fatal("the active group should show its tool rows with completed output folded")
	}
	last.status = "completed"
	hs.settle()
	if !hs.tt.HasText(last.title) {
		t.Fatal("tool completion closed an active group that can still grow")
	}
	hs.clickDetail(locales.AgentToolCalls())
	last.status, last.text = "failed", "Tool error details"
	last.changes = []agentChange{{path: hs.path("main.go"), oldText: "before\n", newText: "after\n", existed: true}}
	hs.a.newFile()
	hs.settle()
	hs.a.openChatPage(chat)
	hs.settle()
	if !hs.tt.HasText(locales.AgentStatusFailed()) || hs.tt.HasText(last.title) {
		t.Fatal("the manual collapse was lost or hid the failure status")
	}
	hs.clickDetail(locales.AgentToolCalls())
	if !hs.tt.HasText(last.text) {
		t.Fatal("reopening the group did not show failure details")
	}
	hs.clickDetail(locales.AgentReviewChange() + ": main.go")
	if diff := hs.a.activeTab().diff; diff == nil || len(diff.files) != 1 || diff.files[0].new.text != "after\n" {
		t.Fatal("the group lost the review action or its captured diff")
	}
}

func TestAgentOutputKeepsOpenToolGroupWhileReading(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.busy = true
	first := &agentTool{title: "Read main.go", kind: "read", status: "completed"}
	last := &agentTool{title: "Scan workspace", kind: "search", status: "running"}
	chat.messages = []*agentMessage{
		{role: "assistant", text: strings.Repeat("Earlier output.\n\n", 40)},
		{role: "tool", tool: first}, {role: "tool", tool: last},
	}
	hs.settle()
	chat.scroll.Y = chat.scroll.MaxY - 24
	hs.settle()
	position := chat.scroll.Y
	if chat.followingOutput() || !hs.tt.HasText(last.title) {
		t.Fatal("expected to be reading the open tool group above the latest output")
	}
	last.status = "completed"
	chat.messages = append(chat.messages, &agentMessage{role: "assistant", text: "Latest answer."})
	chat.busy = false
	hs.settle()
	if chat.scroll.Y != position || !hs.tt.HasText(last.title) {
		t.Fatal("finishing the turn closed the tool group the reader was inspecting")
	}
	hs.clickDetail(locales.AgentJumpLatest())
	if hs.tt.HasText(last.title) {
		t.Fatal("returning to the latest answer did not collapse the finished group")
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

func TestAgentThoughtUsesCompactDisclosureAndDistinctInk(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%v", dark), func(t *testing.T) {
			hs := newHarness(t, dark)
			agentViewFixture(hs)
			const heading = "Considering the query."
			const details = "WHY THE QUERY TAKES SO LONG."
			const answer = "THE QUERY REACHED ITS LIMIT."
			thought := &agentMessage{role: "thought", text: heading + "\n\n" + details}
			hs.a.agent.messages = []*agentMessage{thought, {role: "assistant", text: answer}}
			hs.settle()
			toggle := hs.detailBounds(locales.AgentThought())
			preview := hs.detailBounds(heading + "…")
			if toggle.W <= 0 || toggle.W > 30 || toggle.X+toggle.W > preview.X ||
				preview.Y >= toggle.Y+toggle.H || preview.Y+preview.H <= toggle.Y {
				t.Fatalf("expected a compact disclosure beside the preview: button=%+v preview=%+v", toggle, preview)
			}
			hs.tt.ClickAt(preview.X+preview.W/2, preview.Y+preview.H/2)
			hs.settle()
			if thought.expansionSet || hs.tt.HasText(details) {
				t.Fatal("the adjacent preview toggled the disclosure")
			}
			hs.clickDetail(locales.AgentThought())
			if !hs.tt.Focused(locales.AgentThought()) || !hs.tt.HasText(details) {
				t.Fatal("the named disclosure did not focus and expand")
			}
			frame := hs.tt.Image()
			textPixels := func(text string, ink ui.Color) int {
				rect := hs.detailBounds(text)
				area := image.Rect(int(rect.X), int(rect.Y), int(math.Ceil(float64(rect.X+rect.W))), int(math.Ceil(float64(rect.Y+rect.H))))
				return agentMarkdownColorPixels(frame.SubImage(area).(*image.RGBA), ink)
			}
			bodyInk := hs.a.pal.text
			thoughtInk := agentThoughtColor(hs.a.pal)
			if got := textPixels(answer, bodyInk); got < 20 {
				t.Fatalf("the public reply lost its normal text color, only %d pixels", got)
			}
			if got := textPixels(details, thoughtInk); got < 20 {
				t.Fatalf("reasoning was not painted in the theme's distinct color (%d pixels)", got)
			}
			if got := textPixels(details, bodyInk); got >= 20 {
				t.Fatalf("reasoning shares the public reply color (%d pixels)", got)
			}
			hs.key(0, ui.KeyEnter)
			if !hs.tt.Focused(locales.AgentThought()) || hs.tt.HasText(details) {
				t.Fatal("the disclosure did not retain focus and collapse using Enter")
			}
		})
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

func TestShotAgentTurnSections(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	before := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
	withAppearancePlugins(t)
	for _, dark := range []bool{true, false} {
		hs := newHarness(t, dark)
		hs.tt.SetLocale("zh-Hans")
		hs.a.settings.MicaEnabled, hs.a.settings.MicaAnimation = true, false
		if dark {
			hs.a.setTheme(themeOneDarkPro)
		}
		agentViewFixture(hs)
		chat := hs.a.agent
		first := &agentTool{title: "查找导出请求的日志", kind: "search", status: "completed"}
		second := &agentTool{title: "读取查询超时配置", kind: "read", status: "completed", text: "queryTimeout = 30s"}
		chat.tools = []*agentTool{first, second}
		thought := &agentMessage{role: "thought", text: "按请求阶段核对耗时。\n\n先确认导出在哪一步结束。"}
		chat.messages = []*agentMessage{
			{role: "user", text: "帮我看看统计导出为什么超时。"},
			{role: "commentary", text: "我先对照这次请求的时间戳和查询配置。"},
			thought,
			{role: "tool", tool: first},
			{role: "tool", tool: second},
			{role: "assistant", text: "## 超时发生在查询阶段\n\n日志里，导出请求等待明细查询 **30 秒** 后达到时限。\n\n- 导出任务已提交。\n- 查询未能在时限内返回。\n- 文件写入阶段尚未开始。"},
		}
		hs.tt.SetSize(1440, 820)
		hs.settle()
		hs.shot(fmt.Sprintf("agent-turn-sections-dark-%v", dark))
		hs.clickDetail(locales.AgentThought())
		hs.tt.Move(100, 200)
		hs.settle()
		hs.shot(fmt.Sprintf("agent-thought-expanded-dark-%v", dark))
		hs.clickDetail(locales.AgentThought())
		sizeChatWorkspace(hs, 390)
		hs.settle()
		hs.clickDetail(locales.AgentToolCalls())
		hs.shot(fmt.Sprintf("agent-turn-sections-expanded-narrow-dark-%v", dark))
	}
}

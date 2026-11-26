package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"fiveleaf/locales"
)

func agentToolViewHarness(t *testing.T, tool *agentTool) *harness {
	t.Helper()
	hs := newHarness(t, true)
	agentViewFixture(hs)
	hs.a.agent.tools = []*agentTool{tool}
	hs.a.agent.messages = []*agentMessage{{role: "tool", tool: tool}}
	hs.settle()
	return hs
}

func TestAgentToolDisclosureDefaults(t *testing.T) {
	for _, tc := range []struct {
		kind, status string
		open         bool
	}{
		{"read", "completed", false},
		{"search", "in_progress", false},
		{"", "completed", false},
		{"execute", "in_progress", false},
		{"terminal", "completed", false},
		{"edit", "completed", false},
		{"read", "failed", true},
	} {
		t.Run(tc.kind+"-"+tc.status, func(t *testing.T) {
			tool := &agentTool{title: "Inspect output", kind: tc.kind, status: tc.status, text: "Tool result content"}
			hs := agentToolViewHarness(t, tool)
			if hs.tt.HasText(tool.text) != tc.open || tool.expansionSet {
				t.Fatalf("default kind=%q status=%q open=%v, want %v", tc.kind, tc.status, hs.tt.HasText(tool.text), tc.open)
			}
			if err := hs.tt.Click(tool.title); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if !tool.expansionSet || tool.expanded == tc.open || hs.tt.HasText(tool.text) == tc.open {
				t.Fatal("the disclosure did not store and display the user's choice")
			}
		})
	}
}

func TestAgentToolRunningExpandsOnlyWhileLive(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.busy = true
	tool := &agentTool{title: "Run fixture", kind: "execute", status: "in_progress", text: "streaming output"}
	chat.tools = []*agentTool{tool}
	chat.messages = []*agentMessage{{role: "user", text: "Run it"}, {role: "tool", tool: tool}}
	hs.settle()
	if !hs.tt.HasText(tool.text) || tool.expansionSet {
		t.Fatal("the live command should open without becoming a manual choice")
	}
	tool.status = "completed"
	hs.settle()
	if hs.tt.HasText(tool.text) || tool.expansionSet {
		t.Fatal("a completed command should fold unless the user opened it")
	}
	thought := &agentMessage{role: "thought", text: "Checking the result.\n\nIt looks consistent."}
	chat.messages = append(chat.messages, thought)
	hs.settle()
	if !hs.tt.HasText("It looks consistent.") {
		t.Fatal("active reasoning should open while it streams")
	}
	chat.messages = append(chat.messages, &agentMessage{role: "assistant", text: "The result is correct."})
	hs.settle()
	if hs.tt.HasText("It looks consistent.") || !hs.tt.HasText("The result is correct.") {
		t.Fatal("reasoning should fold once the final response starts")
	}
}

func TestAgentToolDisclosureKeepsChoiceAcrossViewsAndUpdates(t *testing.T) {
	tool := &agentTool{title: "Read project", kind: "read", status: "in_progress", text: "Reading project files"}
	hs := agentToolViewHarness(t, tool)
	chat := hs.a.agent
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.a.newFile()
	hs.a.openChatPage(chat)
	hs.settle()
	tool.text, tool.status = "Finished reading the project", "completed"
	hs.settle()
	if !tool.expanded || !hs.tt.HasText(tool.text) {
		t.Fatal("moving the tool to the page or receiving output lost the expanded state")
	}
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	tool.status, tool.text = "failed", "A later tool error"
	hs.a.newFile()
	hs.a.openChatPage(chat)
	hs.settle()
	if tool.expanded || !tool.expansionSet || hs.tt.HasText(tool.text) {
		t.Fatal("an update or moving back overrode the user's collapsed state")
	}
	if !hs.tt.HasText(locales.AgentStatusFailed()) {
		t.Fatal("a collapsed failed tool hid its status")
	}
}

func TestAgentToolCopyPreservesFullOutput(t *testing.T) {
	text := "first\tcolumn\r\n" + strings.Repeat("中文 result\n", 1000) + "END-OF-COMPLETE-OUTPUT\n\n"
	tool := &agentTool{title: "Read complete log", kind: "read", status: "completed", text: text}
	hs := agentToolViewHarness(t, tool)
	label := locales.AgentCopyToolOutput() + ": " + tool.title
	if err := hs.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if hs.tt.Clipboard() != text || tool.expanded {
		t.Fatal("copying a collapsed tool changed the text or expanded it")
	}
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	preview, clipped := agentVisibleText(text, 8*1024)
	if !clipped || !utf8.ValidString(preview) || !hs.tt.HasText(preview) || hs.tt.HasText("END-OF-COMPLETE-OUTPUT") || !hs.tt.HasText(locales.AgentMoreOutput()) {
		t.Fatal("expanded output did not preserve a bounded UTF-8 preview")
	}
	if err := hs.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if hs.tt.Clipboard() != text {
		t.Fatal("copying clipped output lost the original whitespace or tail")
	}
}

func TestAgentToolReviewRemainsVisibleWhenCollapsed(t *testing.T) {
	tool := &agentTool{title: "Update file", kind: "edit", status: "completed", text: "The file was updated"}
	hs := agentToolViewHarness(t, tool)
	tool.changes = []agentChange{{path: hs.path("main.go"), oldText: "before\n", newText: "after\n", existed: true}}
	hs.settle()
	if hs.tt.HasText(tool.text) {
		t.Fatal("a completed edit should start folded")
	}
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText(tool.text) {
		t.Fatal("opening the edit did not show its output")
	}
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.tt.HasText(tool.text) {
		t.Fatal("the opened edit did not fold again")
	}
	if err := hs.tt.Click(locales.AgentReviewChange() + ": main.go"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	v := hs.a.activeTab().diff
	if v == nil || v.snapshot == "" || len(v.files) != 1 || v.files[0].old.text != "before\n" || v.files[0].new.text != "after\n" {
		t.Fatal("reviewing a collapsed tool did not open its preserved diff")
	}
}

func TestAgentToolOutputScrollsWithinNarrowViews(t *testing.T) {
	tool := &agentTool{title: "Execute a command with a long title", kind: "execute", status: "completed", text: strings.Repeat("long-line-", 300) + "\nlast line"}
	hs := agentToolViewHarness(t, tool)
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	outputLabel := locales.AgentTools() + ": " + tool.title
	for _, width := range []float32{340, 200, 160} {
		sizeChatWorkspace(hs, width)
		hs.settle()
		for _, label := range []string{tool.title, locales.AgentCopyToolOutput() + ": " + tool.title, outputLabel, locales.AgentSend()} {
			bounds, found := hs.tt.Find(label)
			if !found || bounds.X < 0 || bounds.X+bounds.W > width+1 || bounds.W <= 0 || bounds.H <= 0 {
				t.Fatalf("%q escaped %v-wide workspace: %+v found=%v", label, width, bounds, found)
			}
		}
	}
	out, _ := hs.tt.Find(outputLabel)
	// Tester.Find reports the clipped text bounds. Compare the actual text
	// inside the viewport, excluding its edges and scrollbar, to prove that
	// a horizontal gesture moves the content and can return to its start.
	pixels := func() []byte {
		img := hs.tt.Image()
		var pixels []byte
		for y := int(out.Y) + 3; y < int(out.Y+out.H)-7; y++ {
			start := img.PixOffset(int(out.X)+3, y)
			end := img.PixOffset(int(out.X+out.W)-7, y)
			pixels = append(pixels, img.Pix[start:end]...)
		}
		return pixels
	}
	hs.tt.Move(out.X+out.W/2, out.Y+out.H/2)
	hs.settle()
	before := pixels()
	hs.tt.Scroll(out.X+out.W/2, out.Y+out.H/2, 180, 0)
	hs.settle()
	if bytes.Equal(before, pixels()) {
		t.Fatal("terminal output did not scroll horizontally")
	}
	hs.tt.Scroll(out.X+out.W/2, out.Y+out.H/2, -180, 0)
	hs.settle()
	if !bytes.Equal(before, pixels()) {
		t.Fatal("scrolling back did not restore the start of the terminal output")
	}
}

func TestAgentToolInputShowsUsefulSummaryOnly(t *testing.T) {
	tool := &agentTool{title: "Run tests", kind: "execute", status: "completed", text: "PASS", rawInput: json.RawMessage(`{"command":"go test ./cmd/fiveleaf -run TestAgentTool","internalPayload":"unused-private-value"}`)}
	hs := agentToolViewHarness(t, tool)
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText("go test ./cmd/fiveleaf -run TestAgentTool") || hs.tt.HasText("internalPayload") || hs.tt.HasText("unused-private-value") {
		t.Fatal("the tool input summary omitted its command or exposed irrelevant JSON")
	}
	tool.rawInput = json.RawMessage(`{"file_path":"main.go","content":"entire replacement file"}`)
	hs.settle()
	if !hs.tt.HasText("main.go") || hs.tt.HasText("entire replacement file") {
		t.Fatal("the file input summary omitted its path or rendered the full replacement")
	}
}

func TestAgentToolExpandedDetailsFollowAdapterUpdates(t *testing.T) {
	tool := &agentTool{title: "Look up a service", kind: "search", status: "pending"}
	hs := agentToolViewHarness(t, tool)
	if hs.tt.HasText(locales.AgentToolWaitingDetails()) {
		t.Fatal("a collapsed call showed its waiting hint")
	}
	if err := hs.tt.Click(tool.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText(locales.AgentToolWaitingDetails()) {
		t.Fatal("an open pending call had no visible details state")
	}
	tool.status = "completed"
	hs.settle()
	if !hs.tt.HasText(locales.AgentToolNoDetails()) || hs.tt.HasText(locales.AgentToolWaitingDetails()) {
		t.Fatal("a completed call without details still looked pending")
	}
	line := 21
	tool.locations = []agentLocation{{Path: hs.path("main.go"), Line: &line}}
	tool.rawInput = json.RawMessage(`{"server":"logs","tool":"search","arguments":{"query":"routing lookup","accessToken":"internal input"}}`)
	tool.progressOutput.WriteString("Matching calls…")
	tool.rawOutput = formatAgentToolOutput(json.RawMessage(`{"result":{"content":[{"type":"text","text":"Matching calls found"},{"type":"image","mimeType":"image/png","data":"private binary payload","uri":"data:image/png;base64,more-binary"}],"structuredContent":{"matches":3}},"error":null}`))
	hs.settle()
	for _, visible := range []string{locales.AgentToolInput(), locales.AgentToolOutput(), "routing lookup", "main.go:21", "Matching calls found", "matches: 3"} {
		if !hs.tt.HasText(visible) {
			t.Fatalf("missing detail %q", visible)
		}
	}
	for _, hidden := range []string{"internal input", "private binary payload", "more-binary", "Matching calls…", locales.AgentToolNoDetails()} {
		if hs.tt.HasText(hidden) {
			t.Fatalf("unexpected competing detail %q", hidden)
		}
	}
	hs.shot("agent-tool-expanded-details")
	label := locales.AgentCopyToolOutput() + ": " + tool.title
	if err := hs.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if hs.tt.Clipboard() != tool.rawOutput {
		t.Fatalf("copied output = %q, want %q", hs.tt.Clipboard(), tool.rawOutput)
	}
	tool.terminalOutput.WriteString("Live terminal stream\n")
	hs.settle()
	if !hs.tt.HasText("Live terminal stream") || hs.tt.HasText("Matching calls found") {
		t.Fatal("terminal output did not replace the raw fallback")
	}
	tool.text = "Final standard content\n"
	hs.settle()
	if !hs.tt.HasText("Final standard content") || hs.tt.HasText("Live terminal stream") {
		t.Fatal("standard content did not take precedence over the fallback stream")
	}
}

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
)

func TestRoundtableAppProcess(t *testing.T) {
	mode := os.Getenv("FIVELEAF_ROUNDTABLE_TEST")
	if mode == "" {
		return
	}
	if mode == "cli" {
		text, _ := io.ReadAll(os.Stdin)
		time.Sleep(20 * time.Millisecond)
		fmt.Printf("%s reply\n%s", os.Getenv("ROUND_MEMBER"), text)
		os.Exit(0)
	}
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	var count int
	var root string
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage
			Method string
			Params struct {
				Cwd, SessionID string
				Prompt         []struct{ Type, Text string }
			}
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		var result any = map[string]any{}
		switch req.Method {
		case "initialize":
			caps := map[string]any{}
			switch os.Getenv("FIVELEAF_TEST_RESTORE") {
			case "resume":
				caps["sessionCapabilities"] = map[string]any{"resume": map[string]any{}}
			case "load":
				caps["loadSession"] = true
			}
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": caps, "authMethods": []any{}}
		case "session/resume", "session/load":
			if req.Method != "session/"+os.Getenv("FIVELEAF_TEST_RESTORE") || req.Params.SessionID != "source-id" {
				os.Exit(3)
			}
			root = req.Params.Cwd
			if req.Method == "session/load" {
				if os.Getenv("FIVELEAF_TEST_CODEX_REPLAY") == "1" {
					for index, text := range []string{agents.CodexContextHeader, "[user]\nKeep the literal role marker", "later message"} {
						messageID := "c23b010e-582a-4d48-bc7d-0f079528b7d1"
						if index == 2 {
							messageID = "f578a16d-7735-4a16-b9de-50ed99217c78"
						}
						enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
							"sessionId": req.Params.SessionID, "update": map[string]any{"sessionUpdate": "user_message_chunk",
								"messageId": messageID, "content": map[string]string{"type": "text", "text": text}},
						}})
					}
				} else {
					enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": req.Params.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "server replay"}}}})
				}
			}
		case "session/new":
			count++
			root = req.Params.Cwd
			result = map[string]string{"sessionId": fmt.Sprintf("test-%d", count)}
		case "session/prompt":
			cwd, _ := os.Getwd()
			text := fmt.Sprintf("session=%s\nprocess=%s\nworkspace=%s\n", req.Params.SessionID, cwd, root)
			for _, block := range req.Params.Prompt {
				text += block.Text
			}
			enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": req.Params.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}}}})
			result = map[string]string{"stopReason": "end_turn"}
		}
		enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}
	os.Exit(0)
}

func roundtableTestServer(t *testing.T, name, protocol string) settings.AgentServer {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return settings.AgentServer{Name: name, Protocol: protocol, Command: exe, Args: []string{"-test.run=^TestRoundtableAppProcess$"},
		Env: map[string]string{"FIVELEAF_ROUNDTABLE_TEST": protocol, "ROUND_MEMBER": name}, TimeoutSeconds: 10}
}

func testAgentMeeting(t *testing.T, a *app) *agentMeeting {
	t.Helper()
	lib, err := roundtable.ProjectLibrary(t.TempDir(), a.root)
	if err != nil {
		t.Fatal(err)
	}
	var members []roundtable.Participant
	for _, provider := range a.agentProviders() {
		members = append(members, roundtable.Participant{ID: "agent:" + provider.id, Name: provider.name, Provider: provider.id})
	}
	group, err := lib.Create("Project discussion", members)
	if err != nil {
		t.Fatal(err)
	}
	meeting, err := a.startMeeting(lib, group)
	if err != nil {
		t.Fatal(err)
	}
	return meeting
}

func TestAgentChatsKeepRepliesAndDraftsOnSwitch(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.settings.AgentServers = map[string]settings.AgentServer{"one": roundtableTestServer(t, "One", "cli"), "two": roundtableTestServer(t, "Two", "cli")}
	a.selectAgent("one")
	one := a.agent
	one.draft = "first private question"
	a.sendAgent()
	if !one.busy {
		t.Fatal("CLI did not enter busy state")
	}
	a.selectAgent("two")
	two := a.agent
	two.draft = "second draft"
	hs.until("first reply", func() bool { return !one.busy })
	if one.err != "" {
		t.Fatal(one.err)
	}
	if a.agent != two || two.draft != "second draft" || len(two.messages) != 0 {
		t.Fatal("background reply changed selected conversation")
	}
	if len(one.messages) != 2 || !strings.Contains(one.messages[1].text, "first private question") {
		t.Fatalf("lost original conversation: %+v", one.messages)
	}
	a.selectAgent("one")
	if a.agent != one {
		t.Fatal("switch replaced conversation identity")
	}
	a.newAgentSession()
	if len(one.messages) != 0 || two.draft != "second draft" {
		t.Fatal("clear crossed conversation boundaries")
	}
	a.closeAllAgents()
}

func TestAgentContextLoadRetainsRecipient(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.selectAgent("one")
	one := a.agent
	path := hs.path("context-one.txt")
	if err := os.WriteFile(path, []byte("private file excerpt"), 0600); err != nil {
		t.Fatal(err)
	}
	a.addAgentFile(path)
	a.selectAgent("two")
	two := a.agent
	two.draft = "keep second draft"
	hs.until("context load", func() bool { return one.contextLoading == 0 })
	if one.draft != "" || !strings.Contains(one.draftPrompt(), "private file excerpt") || two.draftPrompt() != "keep second draft" || a.agent != two {
		t.Fatal("async file context moved to selected conversation")
	}
}

func TestAgentACPContextWindowAndWorkingDirectory(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	other := t.TempDir()
	server := roundtableTestServer(t, "ACP", "acp")
	server.WorkingDirectory = other
	a.settings.AgentServers = map[string]settings.AgentServer{"acp": server}
	a.selectAgent("acp")
	a.agent.contextMessages = 1
	a.agent.systemPrompt = "system instruction"
	a.agent.draft = "private first turn"
	a.sendAgent()
	hs.until("ACP first reply", func() bool { return !a.agent.busy && !a.agent.connecting })
	if a.agent.err != "" {
		t.Fatal(a.agent.err)
	}
	a.agent.draft = "second turn only"
	a.sendAgent()
	hs.until("ACP second reply", func() bool { return !a.agent.busy && !a.agent.connecting })
	if a.agent.err != "" {
		t.Fatal(a.agent.err)
	}
	text := a.agent.messages[len(a.agent.messages)-1].text
	realOther, _ := filepath.EvalSymlinks(other)
	realRoot, _ := filepath.EvalSymlinks(hs.root)
	if strings.Contains(text, "private first turn") || !strings.Contains(text, "system instruction") || !strings.Contains(text, "session=test-2") || !strings.Contains(text, "process="+realOther) || !strings.Contains(text, "workspace="+realRoot) {
		t.Fatalf("context/working-directory isolation failed: %s", text)
	}
	a.closeAllAgents()
}

func TestAgentMeetingKeepsDirectContextSeparate(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.settings.AgentServers = map[string]settings.AgentServer{"one": roundtableTestServer(t, "One", "cli"), "two": roundtableTestServer(t, "Two", "cli")}
	a.selectAgent("one")
	a.agent.messages = []*agentMessage{{role: "user", text: "secret direct message"}}
	m := testAgentMeeting(t, a)
	if err := m.room.Send("roundtable question", ""); err != nil {
		t.Fatal(err)
	}
	hs.until("roundtable completion", func() bool { return !m.room.Snapshot().Busy })
	snap := m.room.Snapshot()
	if snap.Error != "" {
		t.Fatal(snap.Error)
	}
	if len(snap.Messages) != 4 {
		t.Fatalf("got %d messages, want user+3 hops", len(snap.Messages))
	}
	for _, msg := range snap.Messages {
		if strings.Contains(msg.Text, "secret direct message") {
			t.Fatal("private context leaked to group")
		}
	}
	if len(a.agent.messages) != 1 {
		t.Fatal("group wrote to direct chat")
	}
	a.closeAllAgents()
}

func TestAgentMeetingACPUsesEditorPermissionsAndReview(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("agent-target.go")
	if err := os.WriteFile(path, []byte("before group edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	root, _ := filepath.EvalSymlinks(hs.root)
	a.settings.AgentServers = map[string]settings.AgentServer{"acp": {Name: "ACP", Command: exe, Args: []string{"-test.run=^TestACPAppProcess$"}, Env: map[string]string{"FIVELEAF_ACP_UI_TEST": "1", "FIVELEAF_ACP_UI_ROOT": root}}}
	m := testAgentMeeting(t, a)
	if err := m.room.Discuss(m.group.Topic, "edit in group", "acp"); err != nil {
		t.Fatal(err)
	}
	hs.until("group permission", func() bool { return len(m.chat.permissions) > 0 || m.room.Snapshot().Error != "" })
	if err := m.room.Snapshot().Error; err != "" {
		t.Fatal(err)
	}
	if len(a.agent.permissions) != 0 {
		t.Fatal("group permission entered direct chat")
	}
	m.chat.permissions[0].choose("allow")
	hs.until("group ACP result", func() bool { return !m.room.Snapshot().Busy })
	hs.settle()
	if err := m.room.Snapshot().Error; err != "" {
		t.Fatal(err)
	}
	if len(m.room.Snapshot().Messages) != 2 {
		t.Fatal("group reply missing")
	}
	var edits int
	for _, tool := range m.chat.tools {
		edits += len(tool.changes)
	}
	if edits == 0 {
		t.Fatal("group editor write has no review snapshot")
	}
	a.closeAllAgents()
	restored, err := m.library.OpenRoom(m.group, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	result := restored.Snapshot().Messages[1]
	if result.Text != "" || result.Task == nil || !result.Task.NoAnswer {
		t.Fatal("an editor-only response must persist its activity without inventing an answer", result)
	}
	for _, tool := range result.Task.Tools {
		if strings.HasPrefix(tool.ID, "editor:") && strings.HasSuffix(tool.Detail, "agent-target.go") && tool.Status == "completed" {
			return
		}
	}
	t.Fatal("restored assignment lost its committed editor write", result.Task.Tools)
}

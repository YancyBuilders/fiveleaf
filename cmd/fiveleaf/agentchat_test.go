package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

const (
	acpPlanInspectGreeting = "Inspect the original greeting"
	acpPlanReviseGreeting  = "Revise the greeting text"
	acpPlanVerifyGreeting  = "Verify the updated greeting"
)

// This executable acts as a real stdio ACP peer in the UI integration test.
func TestACPAppProcess(t *testing.T) {
	if os.Getenv("FIVELEAF_ACP_UI_TEST") != "1" {
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	write := func(v any) {
		if encoder.Encode(v) != nil {
			os.Exit(2)
		}
	}
	result := func(id json.RawMessage, v any) { write(map[string]any{"jsonrpc": "2.0", "id": id, "result": v}) }
	update := func(v any) {
		write(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "ui-session", "update": v}})
	}
	plan := func(editStatus, checkStatus string) {
		update(map[string]any{"sessionUpdate": "plan", "entries": []map[string]string{
			{"content": acpPlanInspectGreeting, "priority": "medium", "status": "completed"},
			{"content": acpPlanReviseGreeting, "priority": "medium", "status": editStatus},
			{"content": acpPlanVerifyGreeting, "priority": "medium", "status": checkStatus},
		}})
	}
	request := func(id int, method string, params any) {
		write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	}
	var pending json.RawMessage
	path := filepath.Join(os.Getenv("FIVELEAF_ACP_UI_ROOT"), "agent-target.go")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			os.Exit(3)
		}
		switch msg.Method {
		case "initialize":
			result(msg.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}})
		case "session/new":
			result(msg.ID, map[string]any{"sessionId": "ui-session", "modes": map[string]any{"currentModeId": "edit", "availableModes": []any{map[string]string{"id": "edit", "name": "Edit"}}}})
		case "session/prompt":
			pending = msg.ID
			plan("in_progress", "pending")
			update(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "Reviewing "}})
			update(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "the file."}})
			update(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "edit", "title": "Update greeting", "kind": "edit", "status": "in_progress", "rawInput": map[string]string{"path": path, "content": "package demo\n// agent change\n"}, "locations": []any{map[string]any{"path": path, "line": 2}}})
			request(900, "session/request_permission", map[string]any{"sessionId": "ui-session", "toolCall": map[string]any{"toolCallId": "edit"}, "options": []any{map[string]string{"optionId": "allow", "name": "Allow once", "kind": "allow_once"}, map[string]string{"optionId": "deny", "name": "Reject", "kind": "reject_once"}}})
		case "session/cancel":
			result(pending, map[string]string{"stopReason": "cancelled"})
		default:
			if len(msg.Error) != 0 {
				os.Exit(4)
			}
			switch string(msg.ID) {
			case "900":
				if !strings.Contains(string(msg.Result), `"optionId":"allow"`) {
					os.Exit(5)
				}
				request(901, "fs/read_text_file", map[string]any{"sessionId": "ui-session", "path": path})
			case "901":
				request(902, "fs/write_text_file", map[string]any{"sessionId": "ui-session", "path": path, "content": "package demo\n// agent change\n"})
			case "902":
				update(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "edit", "status": "completed"})
				plan("completed", "completed")
				result(pending, map[string]string{"stopReason": "end_turn"})
			}
		}
	}
	os.Exit(0)
}

func TestACPChatEndToEnd(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("agent-target.go")
	const before = "package demo\n// user change already on disk\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	a.openFile(path, true)
	hs.settle()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(hs.root)
	if err != nil {
		t.Fatal(err)
	}
	a.settings.AgentServers = map[string]settings.AgentServer{"test": {
		Name: "Test Agent", Command: exe, Args: []string{"-test.run=^TestACPAppProcess$"},
		Env: map[string]string{"FIVELEAF_ACP_UI_TEST": "1", "FIVELEAF_ACP_UI_ROOT": realRoot},
	}}
	a.showAgent()
	hs.until("automatic connection", func() bool { return a.agent.sessionID != "" })
	a.agent.draft = "Review this function and update the greeting."
	a.sendAgent()
	hs.until("agent permission", func() bool {
		return len(a.agent.permissions) > 0 && a.agent.permissions[0].title == "Update greeting" || a.agent.err != ""
	})
	if a.agent.err != "" {
		t.Fatal(a.agent.err)
	}
	if p := a.agent.permissions[0]; p.title != "Update greeting" || !strings.Contains(p.detail, "agent-target.go:2") || !strings.Contains(p.detail, "agent change") {
		t.Fatalf("permission omitted the pending operation: %+v", p)
	}
	if a.agent.taskState() != agentTaskApproval {
		t.Fatal("the running task did not show its pending approval")
	}
	if len(a.agent.tools) != 1 || a.agent.tools[0].title != "Update greeting" {
		t.Fatal("tool call is missing")
	}
	if len(a.agent.messages) != 3 || a.agent.messages[1].text != "Reviewing the file." || a.agent.messages[2].tool != a.agent.tools[0] {
		t.Fatalf("streamed messages: %+v", a.agent.messages)
	}
	checkPlan := func(progress string, statuses ...string) {
		t.Helper()
		hs.settle()
		plan := a.agent.plan
		if len(plan) != len(statuses) {
			t.Fatalf("unexpected plan: %+v", plan)
		}
		steps := []string{acpPlanInspectGreeting, acpPlanReviseGreeting, acpPlanVerifyGreeting}
		for index, status := range statuses {
			if plan[index].status != status || !hs.tt.HasText(steps[index]) {
				t.Fatalf("plan entry does not show the expected status and text: %+v, want status %s", plan[index], status)
			}
		}
		if !hs.tt.HasText(locales.AgentPlan()) || !hs.tt.HasText(progress) {
			t.Fatalf("plan panel did not display progress %s", progress)
		}
	}
	checkPlan("1/3", "completed", "in_progress", "pending")
	a.agent.permissions[0].choose("allow")
	hs.until("agent prompt completion", func() bool { return !a.agent.busy })
	if a.agent.taskState() != agentTaskCompleted {
		t.Fatal("the approved task did not reach completion")
	}
	if a.agent.err != "" {
		t.Fatal(a.agent.err)
	}
	checkPlan("3/3", "completed", "completed", "completed")
	if text, err := os.ReadFile(path); err != nil || string(text) != "package demo\n// agent change\n" {
		t.Fatalf("write: %q %v", text, err)
	}
	if d := a.agentDocument(path); d == nil || d.buf.String() != "package demo\n// agent change\n" || d.dirty() {
		t.Fatal("the background editor's buffer was not refreshed after the approved write")
	}
	if a.agent.tools[0].title != "Update greeting" || a.agent.tools[0].status != "completed" {
		t.Fatal("partial tool update lost title or status")
	}
	var change agentChange
	for _, tool := range a.agent.tools {
		if len(tool.changes) > 0 {
			change = tool.changes[0]
		}
	}
	if change.oldText != before {
		t.Fatalf("agent baseline attributed earlier edits: %q", change.oldText)
	}
	a.reviewAgentChange(change)
	hs.settle()
	v := a.activeTab().diff
	if v == nil || v.snapshot == "" || len(v.files) != 1 {
		t.Fatal("agent snapshot did not open")
	}
	f := v.files[0]
	if f.old.text != before || len(hunkActions(f)) != 0 || len(hunkArrows(f)) != 0 || f.base != diffSnapshot {
		t.Fatal("snapshot is not immutable or has wrong baseline")
	}
	a.splitRight()
	hs.settle()
	copy := a.activeTab().diff
	if copy.snapshot != v.snapshot || len(copy.files) != 1 || copy.files[0] == f || copy.files[0].old.text != before || len(hunkActions(copy.files[0])) != 0 {
		t.Fatal("splitting an agent snapshot lost its immutable baseline")
	}
	closed := len(a.closed)
	a.rememberClosed(a.activeTab())
	if len(a.closed) != closed {
		t.Fatal("snapshot was kept as an ordinary closed Git tab")
	}
	if col, _ := a.changesTab(); col != nil {
		t.Fatal("agent snapshot was reused as workspace changes")
	}
	n := v.excerpt(f, map[int]bool{}, map[int]bool{1: true})
	if n == nil {
		t.Fatal("no excerpt for review")
	}
	n.text = "Use a named constant here."
	a.review.notes = []*reviewNote{n}
	a.addAgentReview()
	if !strings.Contains(a.agent.draft, n.text) || !strings.Contains(a.agent.draft, "agent-reported change snapshot") || len(a.review.notes) != 0 || a.agent.busy {
		t.Fatalf("review not queued in draft: %q", a.agent.draft)
	}
	a.rememberTabs()
	for _, col := range a.window.Columns {
		for _, tab := range col.Tabs {
			if tab.Diff != "" {
				t.Fatal("snapshot persisted as an unrelated Git diff")
			}
		}
	}
	client := a.agent.client
	a.disconnectAgent()
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("agent process did not exit")
	}
}

func TestACPSelectionPreservesExactUnicodeAndDirtyText(t *testing.T) {
	hs, d := editHarness(t, "context.go", "prefix 中文 suffix\nnext\n")
	d.edit([2]int{7, 9}, textEdit{start: 7, end: 9, text: "修改"})
	d.sel = [2]int{7, 9}
	hs.a.addAgentSelection()
	got := hs.a.agent.draft
	if !strings.Contains(got, "\n修改\n") || strings.Contains(got, "prefix") || !strings.Contains(got, ":1-1") {
		t.Fatalf("selection was expanded or lost: %q", got)
	}
	hs.a.agent.draft = ""
	hs.a.addAgentFile(d.path)
	if hs.a.agent.draft != "" || !strings.Contains(hs.a.agent.draftPrompt(), "prefix 修改 suffix") {
		t.Fatalf("file context ignored dirty text: %q", hs.a.agent.draftPrompt())
	}
	if hs.a.agent.client != nil {
		t.Fatal("context insertion launched an agent")
	}
}

func TestACPDirtyFileReadAndWriteProtection(t *testing.T) {
	hs, d := editHarness(t, "dirty.go", "original\n")
	hs.a.settings.AutoSave = false
	d.edit([2]int{0, 0}, textEdit{start: 0, end: 0, text: "unsaved "})
	h := &agentHandler{app: hs.a, root: hs.root, generation: hs.a.agent.generation}
	path, err := filepath.EvalSymlinks(d.path)
	if err != nil {
		t.Fatal(err)
	}
	read := make(chan string, 1)
	go func() {
		text, err := h.ReadTextFile(context.Background(), acp.ReadRequest{Path: path})
		if err != nil {
			text = err.Error()
		}
		read <- text
	}()
	var text string
	hs.until("buffer read", func() bool {
		select {
		case text = <-read:
			return true
		default:
			return false
		}
	})
	if text != "unsaved original\n" {
		t.Fatalf("read: %q", text)
	}
	written := make(chan error, 1)
	go func() {
		written <- h.WriteTextFile(context.Background(), acp.WriteRequest{Path: path, Content: "agent\n"})
	}()
	var writeErr error
	hs.until("dirty rejection", func() bool {
		select {
		case writeErr = <-written:
			return true
		default:
			return false
		}
	})
	if writeErr == nil || !strings.Contains(writeErr.Error(), "unsaved") || d.buf.String() != text {
		t.Fatalf("dirty write: %v %q", writeErr, d.buf.String())
	}
	if data, _ := os.ReadFile(path); string(data) != "original\n" {
		t.Fatalf("dirty file was overwritten: %q", data)
	}
}

func TestACPAtomicFilesAndWorkspaceBoundary(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "file.txt")
	if err := os.Link(outside, path); err != nil {
		t.Fatal(err)
	}
	change, err := writeAgentFile(context.Background(), root, path, "")
	if err != nil || change.oldText != "keep" {
		t.Fatalf("atomic write: %+v %v", change, err)
	}
	if data, _ := os.ReadFile(outside); string(data) != "keep" {
		t.Fatalf("hardlink target overwritten: %q", data)
	}
	if data, _ := os.ReadFile(path); len(data) != 0 {
		t.Fatal("empty content was not saved")
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(filepath.Dir(outside), link); err != nil {
		t.Fatal(err)
	}
	if _, err := writeAgentFile(context.Background(), root, filepath.Join(link, "outside.txt"), "bad"); err == nil {
		t.Fatal("symlink escaped workspace")
	}
	if _, err := writeAgentFile(context.Background(), root, outside, "bad"); err == nil {
		t.Fatal("absolute path escaped workspace")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writeAgentFile(ctx, root, path, "cancelled"); err == nil {
		t.Fatal("cancelled write proceeded")
	}
}

func writeAgentFile(ctx context.Context, root, path, content string) (agentChange, error) {
	w, err := prepareAgentFile(ctx, root, path, content)
	if err != nil {
		return agentChange{}, err
	}
	defer w.finish(false)
	if err := w.verifyContents(); err != nil {
		return agentChange{}, err
	}
	return w.change, w.finish(true)
}

func TestACPWriteConflictsWhilePreparing(t *testing.T) {
	for _, action := range []string{"typing", "save-as"} {
		t.Run(action, func(t *testing.T) {
			hs, d := editHarness(t, "before.go", "original\n")
			path := d.path
			canonical, err := filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			d.sync = syncSaving
			w, err := prepareAgentFile(context.Background(), hs.root, canonical, "agent\n")
			if err != nil {
				t.Fatal(err)
			}
			defer w.finish(false)
			if action == "typing" {
				d.edit([2]int{0, 0}, textEdit{start: 0, end: 0, text: "user "})
				hs.a.requestSave(d.document, saveWaiter{})
			} else {
				hs.a.saveDocAs(d.document, hs.path("after.go"))
			}
			if err := hs.a.finishAgentWrite(w, d.document, path); err == nil {
				t.Fatal("concurrent edit was not rejected")
			}
			w.finish(false)
			if data, _ := os.ReadFile(path); string(data) != "original\n" {
				t.Fatalf("agent changed disk despite conflict: %q", data)
			}
			d.sync = syncIdle
			hs.a.nextSave(d.document)
			want := "original\n"
			if action == "typing" {
				want = "user original\n"
			}
			if data, _ := os.ReadFile(d.path); string(data) != want {
				t.Fatalf("queued save lost user's text: %q", data)
			}
		})
	}
}

func TestACPWritePreservesExternalSave(t *testing.T) {
	for _, action := range []string{"modify", "replace", "create", "preserve timestamp"} {
		t.Run(action, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "note.txt")
			var oldTime time.Time
			if action != "create" {
				if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				oldTime = info.ModTime()
			}
			write, err := prepareAgentFile(t.Context(), root, path, "agent\n")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { write.finish(false) })
			external := path
			if action == "replace" {
				external = filepath.Join(root, "external.txt")
			}
			saved := "text saved by another editor\n"
			if action == "preserve timestamp" {
				saved = "external\n"
			}
			if err := os.WriteFile(external, []byte(saved), 0o600); err != nil {
				t.Fatal(err)
			}
			if action == "replace" {
				if err := os.Rename(external, path); err != nil {
					t.Fatal(err)
				}
			}
			if action == "preserve timestamp" {
				if err := os.Chtimes(path, oldTime, oldTime); err != nil {
					t.Fatal(err)
				}
			}
			a := &app{root: root, host: &fakeHost{folder: root}}
			commitErr := write.verifyContents()
			if commitErr == nil {
				commitErr = a.finishAgentWrite(write, nil, "")
			}
			data, err := os.ReadFile(path)
			if err != nil || commitErr == nil || string(data) != saved {
				t.Fatalf("write error %v, disk %q (%v), want conflict and %q", commitErr, data, err, saved)
			}
		})
	}
}

func TestACPExclusiveCreatePreservesExistingFile(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.txt")
	write, err := prepareAgentFile(t.Context(), root, path, "first save\n")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { write.finish(false) })
	if err := write.createTarget(); err != nil {
		t.Fatal(err)
	}
	second, err := prepareAgentFile(t.Context(), root, path, "second save\n")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.finish(false) })
	if err := second.createTarget(); err != errAgentWriteConflict {
		t.Fatalf("second create = %v, want a conflict", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first save\n" {
		t.Fatalf("file content = %q, %v", data, err)
	}
}

func TestACPWriteRejectsReplacedParentDirectory(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory, moved := filepath.Join(root, "notes"), filepath.Join(root, "old-notes")
	path := filepath.Join(directory, "note.txt")
	write, err := prepareAgentFile(t.Context(), root, path, "agent\n")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { write.finish(false) })
	if err := os.Rename(directory, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := write.finish(true); err != errAgentWriteConflict {
		t.Fatalf("write in replaced directory = %v, want a conflict", err)
	}
	for _, dir := range []string{directory, moved} {
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Fatalf("%s was not left empty: %v, %v", dir, entries, err)
		}
	}
}

func TestACPAddingFileSurvivesConnectionGeneration(t *testing.T) {
	hs := newHarness(t, true)
	path := hs.path("context.txt")
	if err := os.WriteFile(path, []byte("keep the selected file"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.addAgentFile(path)
	hs.a.disconnectAgent()
	hs.until("file context", func() bool { return hs.a.agent.contextLoading == 0 })
	if hs.a.agent.draft != "" || !strings.Contains(hs.a.agent.draftPrompt(), "keep the selected file") {
		t.Fatalf("context lost during reconnect: %q", hs.a.agent.draftPrompt())
	}
}

func TestACPDisconnectRestoresPendingDraftAndDropsStaleUpdates(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.agent.pendingPrompt = "pending context"
	a.agent.draft = "next question"
	gen := a.agent.generation
	a.postAgent(gen, func() { a.agent.draft = "stale" })
	a.disconnectAgent()
	a.drain()
	if a.agent.draft != "pending context\n\nnext question" {
		t.Fatalf("draft lost: %q", a.agent.draft)
	}
	provider := agentProvider{id: "removed", config: settings.AgentServer{Command: "agent"}}
	a.agent.connected = &provider
	a.agent.connecting = true
	a.drain()
	if a.agent.connecting || a.agent.connected != nil || a.agent.err == "" {
		t.Fatal("hidden project's changed config was not disconnected")
	}
}

func TestACPToolIDsAreScopedToConnectionAndSession(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	for _, title := range []string{"First session", "After reconnect"} {
		a.disconnectAgent()
		a.agent.sessionID = "reused-session-id"
		var u acp.Update
		copyAgentJSON(map[string]any{"sessionId": "reused-session-id", "update": map[string]string{"sessionUpdate": "tool_call", "toolCallId": "call_1", "title": title, "status": "completed"}}, &u)
		h := agentHandler{app: a, generation: a.agent.generation, root: a.root}
		h.Update(u)
		a.drain()
	}
	if len(a.agent.tools) != 2 || a.agent.tools[0].title != "First session" || a.agent.tools[1].title != "After reconnect" {
		t.Fatalf("tool IDs collided across connections: %+v", a.agent.tools)
	}
}

func TestACPPermissionFollowsPartialToolDetails(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.agent.sessionID = "session"
	p := &agentPermission{toolID: "command", sessionID: "session", generation: a.agent.generation}
	explicit := &agentPermission{toolID: "command", sessionID: "session", generation: a.agent.generation, detail: "Explicit permission content"}
	a.agent.permissions = []*agentPermission{p, explicit}
	h := agentHandler{app: a, generation: a.agent.generation, root: a.root}
	update := func(fields map[string]any) {
		t.Helper()
		fields["toolCallId"] = "command"
		var u acp.Update
		copyAgentJSON(map[string]any{"sessionId": "session", "update": fields}, &u)
		h.Update(u)
		a.drain()
	}
	update(map[string]any{
		"sessionUpdate": "tool_call", "title": "Run tests", "kind": "execute", "status": "pending",
		"content": []any{map[string]any{"type": "content", "content": map[string]string{"type": "text", "text": "Inspecting input"}}},
	})
	if p.title != "Run tests" || !strings.Contains(p.detail, "Inspecting input") {
		t.Fatalf("permission missed late tool details: %+v", p)
	}
	update(map[string]any{
		"sessionUpdate": "tool_call_update",
		"rawInput":      map[string]any{"command": "go", "args": []string{"test", "./internal/acp"}},
		"locations":     []any{map[string]any{"path": "/workspace/src/main.go", "line": 7}},
	})
	for _, want := range []string{"main.go:7", `"command": "go"`, "./internal/acp", "Inspecting input"} {
		if !strings.Contains(p.detail, want) {
			t.Fatalf("permission missed %q after partial update: %s", want, p.detail)
		}
	}
	before := p.detail
	update(map[string]any{"sessionUpdate": "tool_call_update", "status": "in_progress"})
	if p.detail != before {
		t.Fatalf("status-only patch lost or duplicated permission details: %s", p.detail)
	}
	if explicit.detail != "Explicit permission content" {
		t.Fatalf("tool update replaced explicit permission details: %s", explicit.detail)
	}
}

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func requireDetailPaths(t *testing.T, chat *agentChat, want ...string) {
	t.Helper()
	var got []string
	for _, source := range chat.detailSources() {
		if source.label == "" {
			t.Fatal("source without a label")
		}
		got = append(got, source.path)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("source paths = %q, want %q", got, want)
	}
}

func TestChatDetailSourcesSelectionReviewAndEdits(t *testing.T) {
	hs, editor := editHarness(t, "selected.go", "package demo\n")
	application, chat := hs.a, hs.a.agent
	tab := application.activeTab()
	editor.sel = [2]int{0, len("package")}
	application.addSelectionToChat(chat, tab)
	selection := chat.draft
	if selection != agentContextText(editor.path, 1, 1, "package") {
		t.Fatalf("selection context changed: %q", selection)
	}
	requireDetailPaths(t, chat, editor.path)
	path := hs.path("other.txt")
	if err := os.WriteFile(path, []byte("another file"), 0600); err != nil {
		t.Fatal(err)
	}
	application.addChatDetailFile(chat, path)
	hs.until("file context", func() bool { return chat.contextLoading == 0 })
	requireDetailPaths(t, chat, path, editor.path)
	application.addFileToChat(chat, editor.path)
	requireDetailPaths(t, chat, editor.path, path)
	chat.draft = "Question\n\n" + chat.draft
	requireDetailPaths(t, chat, editor.path, path)
	chat.draft = "Question\n\n" + selection
	requireDetailPaths(t, chat, editor.path, path)
	chat.removeDraftFile(path)
	chat.removeDraftFile(editor.path)
	requireDetailPaths(t, chat, editor.path)
	chat.draft = ""
	requireDetailPaths(t, chat)
	chat.draft = selection
	requireDetailPaths(t, chat)
	chat.draft = ""
	application.review.notes = []*reviewNote{
		{path: editor.path, from: 0, to: 1, snippet: []string{"package demo"}, text: "First note", staged: true},
		{path: path, from: 0, to: 1, snippet: []string{"another file"}, text: "Second note", staged: true},
	}
	want := application.reviewText()
	application.addReviewToChat(chat)
	if chat.draft != want || len(application.review.notes) != 0 {
		t.Fatalf("review context changed: %q", chat.draft)
	}
	requireDetailPaths(t, chat, path, editor.path)
	_, chat.draft, _ = strings.Cut(chat.draft, "\n\n2. ")
	requireDetailPaths(t, chat, path)
	chat.draft = strings.Repeat("x", agentTextLimit)
	application.addFileToChat(chat, editor.path)
	requireDetailPaths(t, chat)
}

func TestChatDetailSourcesSelectedImageOnly(t *testing.T) {
	hs := newHarness(t, true)
	chat := hs.a.agent
	outside := t.TempDir()
	path := filepath.Join(outside, "clipboard shot.PNG")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 200, 100))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	hs.a.addChatDetailFile(chat, path)
	hs.until("selected image", func() bool { return chat.contextLoading == 0 })
	requireDetailPaths(t, chat, path)
	if chat.draft != "" || chat.draftPrompt() != path+"\n" || chat.detailSources()[0].preview == nil {
		t.Fatalf("missing thumbnail or non-path image prompt: %q", chat.draftPrompt())
	}
	text := filepath.Join(outside, "other.txt")
	if err := os.WriteFile(text, []byte("private text"), 0600); err != nil {
		t.Fatal(err)
	}
	prompt := chat.draftPrompt()
	hs.a.addChatDetailFile(chat, text)
	hs.until("outside text rejected", func() bool { return chat.contextLoading == 0 })
	if chat.draftPrompt() != prompt || chat.err == "" {
		t.Fatalf("ordinary file bypassed the workspace guard: %q, %s", chat.draftPrompt(), chat.err)
	}
	hs.a.openFile(text, true)
	hs.settle()
	hs.a.addChatDetailFile(chat, text)
	hs.until("outside buffer rejected", func() bool { return chat.contextLoading == 0 })
	if chat.draftPrompt() != prompt {
		t.Fatal("picker attached an outside text buffer")
	}
	sibling := &agentChat{draft: prompt, messages: []*agentMessage{{role: "assistant", text: prompt}}}
	requireDetailPaths(t, sibling)
	chat.draft = "question about the image"
	requireDetailPaths(t, chat, path)
	chat.removeDraftFile(path)
	requireDetailPaths(t, chat)
}

func TestChatDetailSourcesOrderedPickerBatch(t *testing.T) {
	hs, editor := editHarness(t, "open.txt", "saved")
	application, original := hs.a, hs.a.agent
	application.settings.AutoSave = false
	editor.edit([2]int{0, 0}, textEdit{start: 0, end: 0, text: "un"})
	disk := hs.path("disk.txt")
	if err := os.WriteFile(disk, []byte("from disk"), 0600); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(t.TempDir(), "large.PNG")
	if err := os.WriteFile(imagePath, []byte("thumbnail unavailable"), 0600); err != nil {
		t.Fatal(err)
	}
	paths := []string{imagePath, hs.path("missing.txt"), editor.path, disk}
	application.addChatDetailFiles(original, paths)
	if original.contextLoading != 1 || original.draft != "" {
		t.Fatal("picker started multiple loads or applied an open buffer before the first file")
	}
	paths[3] = hs.path("changed-after-callback")
	application.newChatPage()
	other := application.agent
	hs.until("ordered picker batch", func() bool { return original.contextLoading == 0 })
	want := imagePath + "\n\n\n" + agentContextText(editor.path, 0, 0, "unsaved") + "\n\n" + agentContextText(disk, 0, 0, "from disk")
	if original.draft != "" || original.draftPrompt() != want || original.err == "" || application.agent != other || other == original {
		t.Fatalf("batch order, error or recipient changed: %q, %s", original.draftPrompt(), original.err)
	}
	requireDetailPaths(t, original, disk, editor.path, imagePath)
	if original.detailSources()[2].preview != nil {
		t.Fatal("invalid image acquired a thumbnail")
	}
	requireDetailPaths(t, other)
}

func TestChatDetailSourcesBatchMovesWithChatOnly(t *testing.T) {
	workspace := newHarness(t, false)
	detached := tabWindowHarness(t, workspace)
	original, moving := workspace.a.agent, detached.a.agent
	paths := []string{workspace.path("main.go"), workspace.path("README.md")}
	detached.a.addChatDetailFiles(moving, paths)
	if moving.contextLoading != 1 || !detached.a.transferTab(detached.a.activeTab(), workspace.a, workspace.a.column(), 0) {
		t.Fatal("did not transfer the chat with its batch in flight")
	}
	workspace.until("transferred batch", func() bool { return moving.contextLoading == 0 })
	requireDetailPaths(t, moving, paths[1], paths[0])
	requireDetailPaths(t, original)
}

func TestChatDetailSourcesPickerDropsStaleProject(t *testing.T) {
	hs := newHarness(t, true)
	old := hs.a.agent
	hs.a.addChatDetailFiles(old, []string{hs.path("main.go"), hs.path("README.md")})
	hs.a.openFolder(workspaceFixture(t))
	hs.until("obsolete batch discarded", func() bool { return old.contextLoading == 0 })
	if old == hs.a.agent || old.draft != "" || hs.a.agent.draft != "" {
		t.Fatal("an obsolete read reached the old draft or its replacement")
	}
	requireDetailPaths(t, old)
	requireDetailPaths(t, hs.a.agent)
}

func TestChatDetailSourcesSentAndCleared(t *testing.T) {
	for _, protocol := range []string{"acp", "cli"} {
		t.Run(protocol, func(t *testing.T) {
			hs := newHarness(t, true)
			application := hs.a
			application.settings.AgentServers = map[string]settings.AgentServer{"test": roundtableTestServer(t, "test", protocol)}
			application.selectAgent("test")
			chat := application.agent
			const question = "看看这个文件"
			chat.draft = question
			path := hs.path("question.txt")
			if err := os.WriteFile(path, []byte("sent context"), 0600); err != nil {
				t.Fatal(err)
			}
			application.addFileToChat(chat, path)
			hs.until("attached before sending", func() bool { return chat.contextLoading == 0 && !chat.connecting })
			if chat.draft != question {
				t.Fatalf("attaching a file changed the typed question: %q", chat.draft)
			}
			prompt := question + "\n\n" + agentContextText(path, 0, 0, "sent context")
			application.sendChat(chat)
			requireDetailPaths(t, chat, path)
			chat.draft = "independent next question"
			hs.untilLong("reply", func() bool { return !chat.requestInFlight() })
			if chat.err != "" || chat.draft != "independent next question" || len(chat.messages) < 2 || chat.messages[0].text != prompt {
				t.Fatalf("send changed: %s, %q, %d messages", chat.err, chat.draft, len(chat.messages))
			}
			if !strings.Contains(chat.messages[len(chat.messages)-1].text, prompt) {
				t.Fatal("the adapter did not receive the question and file snapshot together")
			}
			requireDetailPaths(t, chat, path)
			if protocol == "acp" {
				for _, kind := range []string{"tool_call", "tool_call_update"} {
					locations := []any{}
					if kind == "tool_call" {
						locations = append(locations, map[string]string{"path": hs.path("reported-only.txt")})
					}
					var update acp.Update
					copyAgentJSON(map[string]any{"sessionId": chat.sessionID, "update": map[string]any{"sessionUpdate": kind, "toolCallId": "read", "locations": locations}}, &update)
					application.chatHandler(chat).Update(update)
					application.drain()
					if kind == "tool_call" {
						requireDetailPaths(t, chat, "", path)
					} else {
						requireDetailPaths(t, chat, path)
					}
				}
			}
			chat.draft = ""
			chat.contextMessages = 1
			application.addFileToChat(chat, path)
			hs.until("standalone file", func() bool { return chat.contextLoading == 0 })
			application.sendChat(chat)
			hs.untilLong("standalone file reply", func() bool { return !chat.requestInFlight() })
			if chat.err != "" || chat.hasDraft() || len(chat.messages) < 4 || !strings.Contains(chat.messages[len(chat.messages)-1].text, agentContextText(path, 0, 0, "sent context")) {
				t.Fatal("the adapter did not receive the standalone file")
			}
			application.disconnectChat(chat)
			requireDetailPaths(t, chat, path)
			application.clearChat(chat)
			requireDetailPaths(t, chat)
		})
	}
}

func TestAgentAttachmentAndQuestionShareSendLimit(t *testing.T) {
	hs, editor := editHarness(t, "limit.go", "package demo")
	chat := hs.a.agent
	hs.a.addFileToChat(chat, editor.path)
	fileSize := len(chat.draftPrompt())
	chat.draft = strings.Repeat("x", agentTextLimit-fileSize-1)
	hs.a.sendChat(chat)
	if chat.err != locales.AgentContextTooLarge() || chat.requestInFlight() || len(chat.draftPrompt()) != agentTextLimit+1 {
		t.Fatalf("oversized prompt lost its draft or was sent: %s", chat.err)
	}
	requireDetailPaths(t, chat, editor.path)
}

func TestChatDetailSourcesReportedNotInferred(t *testing.T) {
	hs := newHarness(t, true)
	chat := hs.a.agent
	chat.sessionID = "test"
	path := hs.path("reported.txt")
	if err := os.WriteFile(path, []byte("reported"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	chat.messages = []*agentMessage{{role: "assistant", text: agentContextText(outside, 1, 1, "private")}}
	requireDetailPaths(t, chat)
	report := func(fields map[string]any) {
		fields["toolCallId"] = "real-tool"
		var update acp.Update
		copyAgentJSON(map[string]any{"sessionId": "test", "update": fields}, &update)
		hs.a.chatHandler(chat).Update(update)
		hs.a.drain()
	}
	report(map[string]any{"sessionUpdate": "tool_call", "title": "mcp__looks_like_a_tool", "kind": "execute", "rawInput": map[string]string{"path": outside}})
	requireDetailPaths(t, chat)
	const resource = "mcp://actual-server/project-summary"
	report(map[string]any{"sessionUpdate": "tool_call_update", "locations": []any{map[string]string{"path": path}, map[string]string{"path": path}}, "content": []any{map[string]any{"type": "content", "content": map[string]string{"type": "resource_link", "uri": resource, "name": "Summary"}}}})
	requireDetailPaths(t, chat, "", "")
	if rows := chat.detailSources(); rows[0].label != path || rows[1].label != resource {
		t.Fatalf("structured locations/resources = %+v", rows)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	report(map[string]any{"sessionUpdate": "tool_call_update", "status": "completed"})
	requireDetailPaths(t, chat, "", "")
	report(map[string]any{"sessionUpdate": "tool_call_update", "title": outside, "locations": []any{map[string]string{"path": outside}}})
	rows := chat.detailSources()
	if len(rows) != 2 || rows[0].label != outside || rows[1].label != resource || rows[0].path != "" || rows[0].preview != nil {
		t.Fatalf("stale or untrusted clickable source: %+v", rows)
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = chat.detailSources() }); allocs != 0 {
		t.Fatalf("idle source list allocated %v times per frame", allocs)
	}
	report(map[string]any{"sessionUpdate": "tool_call_update", "locations": []any{}})
	if rows := chat.detailSources(); len(rows) != 1 || rows[0].label != resource || rows[0].path != "" {
		t.Fatalf("clearing locations also cleared independently reported content: %+v", rows)
	}
	report(map[string]any{"sessionUpdate": "tool_call_update", "locations": []any{}, "content": []any{}})
	requireDetailPaths(t, chat)
}

func TestChatDetailSourcesImportAndIsolation(t *testing.T) {
	hs := newHarness(t, true)
	application := hs.a
	application.settings.AgentServers = map[string]settings.AgentServer{"test": roundtableTestServer(t, "Test", "cli")}
	original := application.agent
	source := continuationSource(t, hs.root)
	application.continueSessionWith(source, "test")
	imported := application.agent
	requireDetailPaths(t, imported)
	hs.until("loaded import", func() bool { return imported.contextLoading == 0 })
	requireDetailPaths(t, imported, source.Transcript)
	requireDetailPaths(t, original)
	imported.messages = append(imported.messages, &agentMessage{role: "user", historical: true, text: agentContextText(hs.path("invented.txt"), 1, 1, "fabricated")})
	requireDetailPaths(t, imported, source.Transcript)
}

func TestChatDetailSourcesUnsentRestore(t *testing.T) {
	hs, log, gate := lifecycleHarness(t, "hold-new")
	application, chat := hs.a, hs.a.agent
	lifecycleSend(t, hs, "first question")
	for _, filename := range []string{"pending.txt", "next.txt"} {
		if err := os.WriteFile(hs.path(filename), []byte(filename), 0600); err != nil {
			t.Fatal(err)
		}
	}
	application.addFileToChat(chat, hs.path("pending.txt"))
	hs.until("first attachment", func() bool { return chat.contextLoading == 0 })
	if chat.draft != "" {
		t.Fatal("file-only send must start with an empty composer")
	}
	application.sendChat(chat)
	hs.untilLong("held session", func() bool { return lifecycleHas(lifecycleEvents(t, log), "new", "lifecycle-2") })
	application.addFileToChat(chat, hs.path("next.txt"))
	hs.until("second attachment", func() bool { return chat.contextLoading == 0 })
	chat.draft = "my next question"
	requireDetailPaths(t, chat, hs.path("next.txt"), hs.path("pending.txt"))
	application.stopChat(chat)
	if chat.draft != "my next question" {
		t.Fatalf("restored text = %q", chat.draft)
	}
	if err := os.WriteFile(gate, []byte("released"), 0600); err != nil {
		t.Fatal(err)
	}
	application.clearChat(chat)
	requireDetailPaths(t, chat, hs.path("next.txt"), hs.path("pending.txt"))
	chat.removeDraftFile(hs.path("pending.txt"))
	requireDetailPaths(t, chat, hs.path("next.txt"))
	chat.removeDraftFile(hs.path("next.txt"))
	requireDetailPaths(t, chat)
}

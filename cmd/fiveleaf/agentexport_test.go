package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/internal/agents"
	"fiveleaf/locales"
)

type chatExportDialog struct {
	*fakeHost
	path, ext string
	save      func(string)
}

func (dialog *chatExportDialog) exportAs(path, ext string, done func(string)) {
	dialog.path, dialog.ext, dialog.save = path, ext, done
}

func TestChatExportCurrentTab(t *testing.T) {
	hs := newHarness(t, true)
	dialog := &chatExportDialog{fakeHost: hs.h}
	hs.a.host = dialog
	agentViewFixture(hs)
	first := hs.a.agent
	first.messages = []*agentMessage{{role: "user", text: "Another chat"}}
	hs.a.newChatPage()
	chat := hs.a.agent
	chat.continuation = &sessionContinuation{source: agents.Session{Title: "../复盘: Login"}, loaded: true, truncated: true}
	chat.draft, chat.systemPrompt = "unsent draft", "system prompt"
	question := "  Where is the error?\n\n```go\n  return err\n```"
	reply := "First line.  \nSecond line."
	answer := &agentMessage{role: "assistant", text: reply}
	chat.messages = []*agentMessage{
		{role: "user", text: question, historical: true}, {role: "assistant", text: "Earlier answer", historical: true},
		{role: "user", text: "Follow-up"}, {role: "thought", text: "private thought"},
		{role: "commentary", text: "Public progress"},
		{role: "tool", tool: &agentTool{text: "tool output"}}, {role: "system", text: "system notice"},
		{role: "connection", text: "connection diagnostics"}, answer,
	}
	hs.settle()
	hs.clickDetail(locales.AgentChatDetails())
	hs.clickDetail(locales.AgentChatExport())
	if dialog.save == nil || dialog.ext != "md" || dialog.path != hs.path("fiveleaf-复盘-login.md") {
		t.Fatalf("expected a local Markdown save dialog, got %q (%s)", dialog.path, dialog.ext)
	}
	hs.a.openChatPage(first)
	answer.text += " streamed after export"
	chat.messages = nil
	path := filepath.Join(t.TempDir(), "chosen.md")
	dialog.save(path)
	hs.settle()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# ../复盘: Login\n\n> " + locales.SessionsContinuePartial() + "\n\n## " + locales.AgentYou() + "\n\n" + question +
		"\n\n## " + locales.AgentTitle() + "\n\nEarlier answer\n\n## " + locales.AgentYou() + "\n\nFollow-up" +
		"\n\n## " + locales.AgentTitle() + "\n\nPublic progress\n\n## " + locales.AgentTitle() + "\n\n" + reply + "\n"
	if string(data) != want {
		t.Errorf("unexpected Markdown\ngot %q\nwant %q", data, want)
	}
}

func TestChatExportEmptyCancelAndRetry(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.draft = "draft only"
	chat.messages = []*agentMessage{{role: "connection", text: "welcome"}, {role: "assistant", text: "\n"}}
	hs.settle()
	hs.clickDetail(locales.AgentChatDetails())
	hs.clickDetail(locales.AgentChatExport())
	if len(hs.h.asked) != 0 {
		t.Fatal("a chat without a public message opened a save dialog")
	}
	chat.pendingPrompt = "pending prompt"
	text := strings.Repeat("x", 34*1024) + " complete message"
	chat.messages = append(chat.messages, &agentMessage{role: "assistant", text: text})
	hs.settle()
	fails := hs.a.fails
	hs.clickDetail(locales.AgentChatExport())
	if len(hs.h.asked) != 1 || hs.a.fails != fails {
		t.Fatal("cancelling export should only dismiss the dialog")
	}
	hs.h.saveTo = t.TempDir()
	hs.clickDetail(locales.AgentChatExport())
	if hs.a.fails != fails+1 {
		t.Fatal("a write failure did not reach the user")
	}
	hs.h.saveTo = filepath.Join(hs.h.saveTo, "retry.md")
	hs.clickDetail(locales.AgentChatExport())
	data, err := os.ReadFile(hs.h.saveTo)
	want := "# " + locales.AgentThreadTitle() + "\n\n## " + locales.AgentTitle() + "\n\n" + text + "\n"
	if err != nil || string(data) != want {
		t.Fatalf("retry did not export the full stored reply, got %d bytes: %v", len(data), err)
	}
}

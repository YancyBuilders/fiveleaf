package main

import (
	"slices"
	"strings"
	"testing"

	"fiveleaf/locales"
)

func TestAgentACPQueuedFollowUpPreservesNextDraft(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "permission")
	connectAgentModelTest(t, hs)
	app, chat := hs.a, hs.a.agent
	client := chat.client
	chat.draft = "First question"
	app.sendChat(chat)
	hs.untilLong("first prompt sent", func() bool { return chat.promptWritten && len(chat.permissions) == 1 })
	chat.draft = "Follow-up with a file"
	app.addChatDetailFile(chat, hs.path("go.mod"))
	hs.untilLong("file attachment", func() bool { return chat.contextLoading == 0 && chat.sources.draft.hasFiles() })
	wanted := chat.draftPrompt()
	hs.settle()
	hs.shot("agent-queue-action-and-approval")
	if err := hs.tt.Click(locales.AgentQueueFollowUp()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if chat.queued == nil {
		t.Fatalf("queue action did not stage the draft: active=%v err=%q", chat.canQueueDraft(), chat.err)
	}
	if chat.hasDraft() || !hs.tt.HasText(locales.AgentQueuedFollowUp()) || len(queuedTestPrompts(t, logPath)) != 1 {
		t.Fatal("queueing should clear the composer and wait for the first prompt")
	}
	hs.shot("agent-queued-follow-up")
	hs.tt.Type("Still typing the next draft")
	if chat.draft != "Still typing the next draft" {
		t.Fatalf("queue action did not return keyboard focus to the composer: %q", chat.draft)
	}
	chat.permissions[0].choose("allow")
	hs.untilLong("queued prompt sent", func() bool { return chat.turn == 2 && len(chat.permissions) == 1 })
	prompts := queuedTestPrompts(t, logPath)
	if chat.client != client || chat.queued != nil || chat.draft != "Still typing the next draft" ||
		len(prompts) != 2 || !strings.HasSuffix(prompts[1].Text, "[user]\n"+wanted+"\n\n") || strings.Contains(prompts[1].Text, chat.draft) {
		t.Fatalf("queued follow-up changed connection, draft, or payload: draft=%q prompts=%+v", chat.draft, prompts)
	}
	chat.permissions[0].choose("allow")
	hs.untilLong("follow-up complete", func() bool { return !chat.busy })
	if chat.pendingQueued || len(queuedTestPrompts(t, logPath)) != 2 {
		t.Fatal("the submitted follow-up was left queued or dispatched twice")
	}
}

func TestAgentACPQueuedFollowUpStaysPausedAfterStop(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "permission")
	connectAgentModelTest(t, hs)
	app, chat := hs.a, hs.a.agent
	client := chat.client
	chat.draft = "First question"
	app.sendChat(chat)
	hs.untilLong("waiting on approval", func() bool { return chat.promptWritten && len(chat.permissions) == 1 })
	chat.draft = "Keep this follow-up"
	app.queueChatDraft(chat)
	queued := chat.queued
	if queued == nil {
		t.Fatal("follow-up was not queued")
	}
	app.stopChat(chat)
	hs.untilLong("original turn finished after stop", func() bool { return !chat.busy })
	if chat.client != client || chat.lastTask != agentTaskCompleted || chat.queued != queued ||
		!hs.tt.HasText(locales.AgentQueuePaused()) || len(queuedTestPrompts(t, logPath)) != 1 {
		t.Fatal("Stop should park the follow-up even after a late end_turn")
	}
	chat.draft = "Send this manually"
	app.sendChat(chat)
	hs.untilLong("manual prompt on same connection", func() bool { return chat.turn == 2 && len(chat.permissions) == 1 })
	chat.permissions[0].choose("allow")
	hs.untilLong("manual prompt complete", func() bool { return !chat.busy })
	if chat.queued != queued || len(queuedTestPrompts(t, logPath)) != 2 {
		t.Fatal("an unrelated turn resumed the paused follow-up")
	}
	if err := hs.tt.Click(locales.AgentQueueEdit()); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("queued message restored for editing", func() bool { return chat.queued == nil })
	if chat.draft != "Keep this follow-up" {
		t.Fatalf("edited draft = %q", chat.draft)
	}
}

func queuedTestPrompts(t *testing.T, path string) []roleRuntimeEvent {
	t.Helper()
	return slices.DeleteFunc(agentRoleEvents(t, path), func(event roleRuntimeEvent) bool { return event.Kind != "prompt" })
}

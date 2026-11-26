package main

import (
	"strings"
	"testing"

	"fiveleaf/locales"
)

func TestAgentTurnFailureRestoresDraftExplicitly(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	file := chatTextAttachment("notes.txt", hs.path("notes.txt"), "Original context")
	failure := &agentTurnFailure{
		reason: "The reply stopped before it finished.",
		draft:  chatSourceDraft{text: "Original question", refs: []chatDraftRef{file}},
	}
	chat.messages = []*agentMessage{
		{role: "user", text: failure.draft.text, failure: failure},
		{role: "assistant", text: "Partial reply"},
	}
	hs.settle()
	if chat.hasDraft() || !hs.tt.HasText("Partial reply") || !hs.tt.HasText(failure.reason) {
		t.Fatal("a failed turn should keep its partial reply and show its failure without restoring its draft")
	}

	restore := locales.AgentRestoreFailedDraft()
	chat.draft = "New question"
	hs.settle()
	hs.clickDetail(restore)
	if chat.draft != "New question" || chat.sources.draft.hasFiles() {
		t.Fatal("restoring a failed turn replaced the current draft")
	}

	chat.draft = ""
	chat.contextLoading = 1
	hs.settle()
	hs.clickDetail(restore)
	if chat.hasDraft() {
		t.Fatal("restoring a failed turn while loading a file replaced the current input")
	}

	chat.contextLoading = 0
	hs.settle()
	hs.clickDetail(restore)
	if chat.draft != failure.draft.text || chat.sources.draft.fileIndex(file.file) < 0 || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("explicit restore did not restore the original draft and attachment or focus the composer")
	}
	if chat.turn != 0 || chat.requestInFlight() || len(chat.messages) != 2 {
		t.Fatal("restoring the draft dispatched a new request")
	}
	chat.removeDraftFile(file.file)
	if failure.draft.fileIndex(file.file) < 0 {
		t.Fatal("editing the restored draft changed the stored retry attachment")
	}
}

func TestAgentTurnFailureShownBeforeLongPrompt(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 340)
	chat := hs.a.agent
	const firstLine = "Long prompt"
	failure := &agentTurnFailure{reason: "The reply stopped."}
	chat.messages = []*agentMessage{{role: "user", title: "Long prompt tab", text: firstLine + "\n\n" + strings.Repeat("Log line\n\n", 120), failure: failure}}
	hs.settle()
	chat.scroll.Y = 0
	hs.settle()
	failureTop := hs.detailBounds(failure.reason).Y
	restore := hs.detailBounds(locales.AgentRestoreFailedDraft())
	prompt := hs.detailBounds(firstLine)
	if failureTop >= restore.Y || restore.Y+restore.H > prompt.Y {
		t.Fatalf("failure and restore should precede the prompt: top=%v restore=%+v prompt=%+v", failureTop, restore, prompt)
	}
}

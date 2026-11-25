package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

const sessionContinuationTranscript = `{"role":"user","content":"original task: fix the login flow"}
{"role":"assistant","content":"original progress: the redirect needs a regression test"}
`

func continuationSource(t *testing.T, root string) agents.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte(sessionContinuationTranscript), 0o600); err != nil {
		t.Fatal(err)
	}
	return agents.Session{Harness: agents.Codex, ID: "source-id", Title: "Login repair", Dir: root, Transcript: path}
}

func assertContinuationIdle(t *testing.T, chat *agentChat) {
	t.Helper()
	if chat.busy || chat.turn != 0 || chat.pendingPrompt != "" {
		t.Fatal("opening history sent a prompt")
	}
	for _, message := range chat.messages {
		if !message.historical {
			t.Fatal("opening history added a live message")
		}
	}
	if !chat.continuationReady() && (chat.client != nil || chat.connecting || chat.connectionAttempted) {
		t.Fatal("connection started before history was ready")
	}
}

func TestResumeOriginalACPSession(t *testing.T) {
	for _, capability := range []string{"resume", "load"} {
		t.Run(capability, func(t *testing.T) {
			hs := newHarness(t, true)
			server := roundtableTestServer(t, "Codex", "acp")
			server.Env["FIVELEAF_TEST_RESTORE"] = capability
			hs.a.settings.AgentServers = map[string]settings.AgentServer{"codex": server}
			source := continuationSource(t, hs.root)
			hs.a.resumeSession(source)
			chat := hs.a.agent
			hs.until("restored ACP connection", func() bool { return chat.sessionID != "" || chat.err != "" })
			if chat.sessionID != source.ID || len(chat.messages) != 2 || chat.draft != "" || chat.err != "" {
				t.Fatalf("opened session %q with %d messages and error %q", chat.sessionID, len(chat.messages), chat.err)
			}
			for _, followUp := range []string{"first follow-up", "second follow-up"} {
				chat.draft = followUp
				hs.a.sendAgent()
				hs.until("reply in source session", func() bool { return !chat.busy && !chat.connecting })
				reply := chat.messages[len(chat.messages)-1]
				if chat.sessionID != source.ID || reply.role != "assistant" || !strings.Contains(reply.text, "session="+source.ID) || !strings.Contains(reply.text, followUp) || strings.Contains(reply.text, "original task:") || (followUp == "second follow-up" && strings.Contains(reply.text, "first follow-up")) || chat.err != "" || len(hs.a.terms) != 0 {
					t.Fatalf("follow-up used session %q with reply %q and error %q", chat.sessionID, reply.text, chat.err)
				}
			}
			hs.a.closeAllAgents()
		})
	}
}

func TestResumeSessionWithoutLocalTranscript(t *testing.T) {
	for _, capability := range []string{"load", "resume", ""} {
		t.Run("adapter-"+capability, func(t *testing.T) {
			hs := newHarness(t, true)
			server := roundtableTestServer(t, "Codex", "acp")
			server.Env["FIVELEAF_TEST_RESTORE"] = capability
			hs.a.settings.AgentServers = map[string]settings.AgentServer{"codex": server}
			source := continuationSource(t, hs.root)
			source.Transcript = ""
			hs.a.resumeSession(source)
			chat := hs.a.agent
			hs.until("history lookup", func() bool { return chat.contextLoading == 0 && !chat.connecting })
			if capability == "" {
				if chat.sessionID != "" || chat.continuationReady() || chat.err == "" {
					t.Fatal("a missing transcript became an empty conversation")
				}
				return
			}
			if chat.sessionID != source.ID || !chat.continuationReady() || chat.err != "" {
				t.Fatalf("adapter did not resume by ID: session %q, error %q", chat.sessionID, chat.err)
			}
			if capability == "load" && (len(chat.messages) != 1 || chat.messages[0].text != "server replay" || !chat.messages[0].historical) {
				t.Fatal("the chat did not show the history replayed by the adapter")
			}
			hs.a.closeAllAgents()
		})
	}
}

func TestSessionContinuationPreservesDefaultAndReusesTargets(t *testing.T) {
	hs := newHarness(t, true)
	application := hs.a
	application.settings.AgentServers = map[string]settings.AgentServer{
		"one": roundtableTestServer(t, "One", "cli"),
		"two": roundtableTestServer(t, "Two", "cli"),
	}
	application.selectAgent("one")
	defaultChat := application.agent
	defaultChat.draft, defaultChat.busy, defaultChat.sessionID = "keep default draft", true, "keep-default-session"
	defaultChat.messages = []*agentMessage{{role: "user", text: "keep default history"}}
	source := continuationSource(t, hs.root)
	application.continueSessionWith(source, "one")
	first := application.agent
	if first == defaultChat || first.historyKey == "" || first.contextLoading != 1 {
		t.Fatal("continuation did not get a separate loading conversation")
	}
	first.draft = "edited while loading"
	hs.until("continuation history", func() bool { return first.contextLoading == 0 })
	if first.err != "" || !first.continuation.loaded || first.draft != "edited while loading" || len(first.messages) != 2 {
		t.Fatalf("loaded history = %d messages, draft %q, error %q", len(first.messages), first.draft, first.err)
	}
	assertContinuationIdle(t, first)
	for _, message := range first.messages {
		if !message.historical {
			t.Fatal("imported history can enter the live context twice")
		}
	}
	application.continueSessionWith(source, "one")
	if application.agent != first || len(first.messages) != 2 || first.contextLoading != 0 {
		t.Fatal("reopening the same target duplicated its conversation")
	}
	application.continueSessionWith(source, "two")
	second := application.agent
	hs.until("second target history", func() bool { return second.contextLoading == 0 })
	if second == first || second.historyKey == first.historyKey || second.provider != "two" || second.draft != locales.SessionsContinueDraft() {
		t.Fatal("another target reused the first target's state")
	}
	second.draft = "second target draft"
	application.continueSessionWith(source, "one")
	if application.agent != first || first.draft != "edited while loading" || second.draft != "second target draft" {
		t.Fatal("target switching lost a continuation draft")
	}
	application.selectAgent("one")
	if application.agent != defaultChat || defaultChat.draft != "keep default draft" || !defaultChat.busy || defaultChat.sessionID != "keep-default-session" || len(defaultChat.messages) != 1 {
		t.Fatal("continuation changed the provider's default conversation")
	}
	application.continueSessionWith(source, "one")
	application.newAgentSession()
	if application.agent != defaultChat || first.draft != "edited while loading" || len(first.messages) != 2 {
		t.Fatal("New Session cleared imported history instead of returning to the default chat")
	}
	after, err := os.ReadFile(source.Transcript)
	if err != nil || string(after) != sessionContinuationTranscript {
		t.Fatal("continuation changed the source transcript")
	}
}

func TestSessionContinuationMissingTranscriptCanRetry(t *testing.T) {
	hs := newHarness(t, true)
	application := hs.a
	application.settings.AgentServers = map[string]settings.AgentServer{"one": roundtableTestServer(t, "One", "cli")}
	source := continuationSource(t, hs.root)
	if err := os.Remove(source.Transcript); err != nil {
		t.Fatal(err)
	}
	application.continueSessionWith(source, "one")
	chat := application.agent
	hs.until("failed history load", func() bool { return chat.contextLoading == 0 })
	if chat.err == "" || chat.continuation.loaded || chat.continuation.source != source || len(chat.messages) != 0 {
		t.Fatal("failed import discarded its source or reported a loaded conversation")
	}
	failure := chat.err
	chat.draft = "keep this next request"
	application.sendAgent()
	assertContinuationIdle(t, chat)
	if chat.draft != "keep this next request" || chat.err != failure {
		t.Fatal("sending before import succeeded lost the draft or import error")
	}
	if err := os.WriteFile(source.Transcript, []byte(sessionContinuationTranscript), 0o600); err != nil {
		t.Fatal(err)
	}
	application.loadSessionContinuation(chat)
	hs.until("retried history load", func() bool { return chat.contextLoading == 0 })
	if chat.err != "" || !chat.continuation.loaded || chat.draft != "keep this next request" || len(chat.messages) != 2 {
		t.Fatal("retry failed to load the restored transcript and preserve the draft")
	}
	assertContinuationIdle(t, chat)
}

func TestSessionContinuationContextKeepsHistoryOnce(t *testing.T) {
	history := agents.Handoff{Messages: []agents.HandoffMessage{
		{Role: "user", Text: "original-task-marker"},
		{Role: "assistant", Text: "original-progress-marker"},
	}}
	source := agents.Session{Harness: agents.Codex, ID: "source", Title: "Task", Dir: "/work"}
	for _, limit := range []int{1, 100} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			chat := &agentChat{contextMessages: limit, continuation: &sessionContinuation{
				source: source, loaded: true, context: sessionContinuationContext(source, history),
			}}
			for _, message := range history.Messages {
				chat.messages = append(chat.messages, &agentMessage{role: message.Role, text: message.Text, historical: true})
			}
			for index := 0; index < 110; index++ {
				chat.messages = append(chat.messages, &agentMessage{role: "user", text: "earlier follow-up"})
			}
			chat.messages = append(chat.messages, &agentMessage{role: "user", text: "newest-request-marker"})
			context := chatContext(chat)
			var combined strings.Builder
			for _, message := range context {
				combined.WriteString(message.Text)
				combined.WriteByte('\n')
			}
			for _, marker := range []string{"original-task-marker", "original-progress-marker", "newest-request-marker"} {
				if strings.Count(combined.String(), marker) != 1 {
					t.Fatalf("context at limit %d lost or duplicated %q", limit, marker)
				}
			}
			if len(context) > 100 || context[0].Role != "user" || context[len(context)-1].Text != "newest-request-marker" {
				t.Fatalf("context at limit %d has the wrong order, role or message budget", limit)
			}
			if limit == 1 && strings.Contains(combined.String(), "earlier follow-up") {
				t.Fatal("the context-message limit no longer applies to live follow-ups")
			}
		})
	}
}

func TestSessionContinuationSendsHistoryOnEachTurn(t *testing.T) {
	for _, protocol := range []string{"cli", "acp"} {
		t.Run(protocol, func(t *testing.T) {
			hs := newHarness(t, true)
			application := hs.a
			application.settings.AgentServers = map[string]settings.AgentServer{"target": roundtableTestServer(t, "Target", protocol)}
			application.continueSessionWith(continuationSource(t, hs.root), "target")
			chat := application.agent
			hs.until("history and connection ready", func() bool { return chat.contextLoading == 0 && !chat.connecting })
			if chat.err != "" {
				t.Fatal(chat.err)
			}
			assertContinuationIdle(t, chat)
			chat.contextMessages = 1
			for _, request := range []string{"first-continuation-request", "second-continuation-request"} {
				chat.draft = request
				application.sendAgent()
				hs.until("continuation reply", func() bool { return !chat.busy && !chat.connecting })
				if chat.err != "" {
					t.Fatal(chat.err)
				}
				reply := chat.messages[len(chat.messages)-1]
				if reply.role != "assistant" || reply.historical {
					t.Fatal("no new assistant reply arrived")
				}
				for _, marker := range []string{"original task: fix the login flow", "original progress: the redirect needs a regression test", request} {
					if strings.Count(reply.text, marker) != 1 {
						t.Fatalf("%s prompt lost or duplicated %q: %s", protocol, marker, reply.text)
					}
				}
				if request == "second-continuation-request" && strings.Contains(reply.text, "first-continuation-request") {
					t.Fatal("the previous live turn bypassed the context-message limit")
				}
			}
			application.closeAllAgents()
		})
	}
}

func TestSessionContinuationOversizedDraftStaysEditable(t *testing.T) {
	hs := newHarness(t, true)
	application := hs.a
	application.settings.AgentServers = map[string]settings.AgentServer{"target": roundtableTestServer(t, "Target", "cli")}
	application.continueSessionWith(continuationSource(t, hs.root), "target")
	chat := application.agent
	hs.until("history loaded", func() bool { return chat.contextLoading == 0 })
	if chat.err != "" {
		t.Fatal(chat.err)
	}
	draft := strings.Repeat("x", agentTextLimit-len(chat.continuation.context)+1)
	chat.draft = draft
	application.sendAgent()
	assertContinuationIdle(t, chat)
	if chat.err != locales.AgentContextTooLarge() || chat.draft != draft || len(chat.messages) != 2 {
		t.Fatal("the oversized follow-up was sent, dropped or added to history")
	}
}

func waitContinuationPost(t *testing.T, application *app) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		application.mu.Lock()
		queued := len(application.queue) > 0
		application.mu.Unlock()
		if queued {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("history load did not post its result")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSessionContinuationStaleLoadDoesNotRewriteWorkspace(t *testing.T) {
	for _, action := range []string{"switch", "close"} {
		t.Run(action, func(t *testing.T) {
			hs := newHarness(t, true)
			application := hs.a
			application.settings.AgentServers = map[string]settings.AgentServer{"target": roundtableTestServer(t, "Target", "cli")}
			application.continueSessionWith(continuationSource(t, hs.root), "target")
			oldChat := application.agent
			waitContinuationPost(t, application)
			if action == "switch" {
				application.openFolder(t.TempDir())
			} else {
				application.shutdown()
			}
			current := application.agent
			current.draft = "current workspace draft"
			application.drain()
			if application.agent != current || current.draft != "current workspace draft" || len(current.messages) != 0 || current.continuation != nil {
				t.Fatal("a stale load rewrote the new workspace conversation")
			}
			if oldChat.continuation.loaded || len(oldChat.messages) != 0 {
				t.Fatal("a stale load wrote to a closed conversation")
			}
		})
	}
}

func TestSessionContinuationReloadedWorkspaceCanRetry(t *testing.T) {
	hs := newHarness(t, true)
	application := hs.a
	application.settings.AgentServers = map[string]settings.AgentServer{"target": roundtableTestServer(t, "Target", "cli")}
	application.continueSessionWith(continuationSource(t, hs.root), "target")
	chat := application.agent
	waitContinuationPost(t, application)
	application.openFolder(application.root)
	application.drain()
	if chat.contextLoading != 0 {
		t.Fatal("reloading the same workspace left the continuation permanently loading")
	}
	application.loadSessionContinuation(chat)
	hs.until("history after workspace reload", func() bool { return chat.contextLoading == 0 })
	if !chat.continuation.loaded || chat.err != "" || len(chat.messages) != 2 {
		t.Fatal("a continuation could not retry after the workspace reloaded")
	}
}

func TestSessionContinuationKeepsFileOnlyDraft(t *testing.T) {
	hs, editor := editHarness(t, "context.go", "package demo\n")
	application := hs.a
	application.settings.AgentServers = map[string]settings.AgentServer{"target": roundtableTestServer(t, "Target", "cli")}
	application.continueSessionWith(continuationSource(t, hs.root), "target")
	chat := application.agent
	application.addFileToChat(chat, editor.path)
	hs.until("history and attachment", func() bool { return chat.contextLoading == 0 })
	if !chat.continuationReady() || chat.draft != "" || chat.draftPrompt() != agentContextText(editor.path, 0, 0, "package demo\n") {
		t.Fatalf("import changed the file-only draft: %q", chat.draft)
	}
}

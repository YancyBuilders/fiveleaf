package main

import (
	"strings"
	"testing"

	"fiveleaf/internal/settings"
)

func TestReturnChatGroupKeepsPrivateTargets(t *testing.T) {
	workspace := newHarness(t, false)
	workspace.a.settings.AgentServers = map[string]settings.AgentServer{"same": roundtableTestServer(t, "Same", "acp")}
	detached := tabWindowHarness(t, workspace)
	to, from := workspace.a, detached.a
	openGroup := func(window *harness, draft string) (*agentChat, *agentChat) {
		owner, chat := window.a, window.a.agent
		chat.provider, chat.draft = "same", draft
		owner.openChatPage(chat)
		window.until("live ACP connection", func() bool { return chat.sessionID != "" && !chat.connecting })
		owner.rememberChat(chat)
		role := &agentChat{group: chat.group, agentTarget: agentTarget{provider: chat.provider, role: "Review"}, draft: draft + " role", busy: true}
		owner.rememberChat(role)
		return chat, role
	}
	resident, residentRole := openGroup(workspace, "workspace")
	incoming, incomingRole := openGroup(detached, "detached")
	client, generation := incoming.client, incoming.generation
	const prompt = "reply only to the detached conversation"
	incoming.draft = prompt
	from.sendChat(incoming)
	if !incoming.busy || client == nil || client == resident.client {
		t.Fatal("expected an in-flight turn on a separate ACP connection")
	}
	incoming.draft = "detached"
	unrelated := from.blankChat()
	from.rememberChat(unrelated)
	var delivered *app
	from.chatHandler(incomingRole).dispatch(func(owner *app) { delivered = owner })
	if !from.transferTab(from.activeTab(), to, to.column(), 0) {
		t.Fatal("chat tab did not return")
	}
	to.drain()
	if incoming.group == resident.group || to.agent != incoming || delivered != to || incoming.draft != "detached" {
		t.Fatal("return failed to isolate the live group or deliver its queued callback to the workspace")
	}
	if from.agentChats[agentChatKey(unrelated)] != unrelated || len(from.agentChats) != 1 {
		t.Fatal("return moved an unrelated source chat or left a moved target behind")
	}
	workspace.until("returned ACP reply", func() bool { return !incoming.busy })
	if incoming.client != client || incoming.generation != generation || len(incoming.messages) < 2 || !strings.Contains(incoming.messages[len(incoming.messages)-1].text, prompt) || len(resident.messages) != 0 {
		t.Fatal("return restarted the live connection or delivered its reply to the wrong chat")
	}
	for _, pair := range [][2]*agentChat{{incoming, incomingRole}, {resident, residentRole}} {
		chat, role := pair[0], pair[1]
		to.selectChatRole(chat, "Review")
		if to.agent != role || role.draft != chat.draft+" role" {
			t.Fatal("switching roles selected another group's conversation")
		}
		to.selectChatAgent(role, "same")
		if to.agent != chat || chat.client == nil {
			t.Fatal("switching providers lost the live conversation")
		}
	}
}

func TestReturnChatGroupKeepsLoadingHistory(t *testing.T) {
	workspace := newHarness(t, false)
	workspace.a.settings.AgentServers = map[string]settings.AgentServer{"target": roundtableTestServer(t, "Target", "cli")}
	detached := tabWindowHarness(t, workspace)
	to, from := workspace.a, detached.a
	source := continuationSource(t, workspace.root)
	to.continueSessionWith(source, "target")
	resident := to.agent
	workspace.until("workspace history", resident.continuationReady)
	resident.draft = "workspace draft"
	from.continueSessionWith(source, "target")
	incoming := from.agent
	incoming.draft = "detached draft"
	if incoming.contextLoading != 1 || incoming == resident || incoming.historyKey != resident.historyKey {
		t.Fatal("expected two independent continuations of the same history")
	}
	if !from.transferTab(from.activeTab(), to, to.column(), 0) {
		t.Fatal("loading history did not return")
	}
	workspace.until("returned history callback", func() bool { return incoming.contextLoading == 0 })
	if !incoming.continuationReady() || len(incoming.messages) != 2 || incoming.draft != "detached draft" {
		t.Fatal("the returned continuation dropped its in-flight read or overwrote its draft")
	}
	if to.agentChats[agentChatKey(resident)] != resident || to.agentChats[agentChatKey(incoming)] != incoming || len(resident.messages) != 2 || resident.draft != "workspace draft" {
		t.Fatal("return overwrote the workspace continuation or lost the returned one")
	}
	to.continueSessionWith(source, "target")
	if to.agent != resident {
		t.Fatal("opening history no longer selects the existing workspace continuation")
	}
	to.dropTab(to.activeTab())
	to.continueSessionWith(source, "target")
	if to.agent != incoming || incoming.draft != "detached draft" || incoming.contextLoading != 0 {
		t.Fatal("opening history did not reuse the remaining live continuation")
	}
}

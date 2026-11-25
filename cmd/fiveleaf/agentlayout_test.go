package main

import (
	"math"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestAgentDefaultsToConversation(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.settings.AgentServers = map[string]settings.AgentServer{
		"alpha": roundtableTestServer(t, "Alpha", "acp"),
		"beta":  roundtableTestServer(t, "Beta", "acp"),
	}
	sizeChatWorkspace(hs, 600)
	a.showAgent()
	hs.until("default agent connection", func() bool { return a.agent.sessionID != "" })
	hs.settle()
	if hs.tt.HasText("Beta") {
		t.Fatal("the default conversation still opens target administration")
	}
	input, inputOK := hs.tt.Find(locales.AgentDraft())
	target, targetOK := hs.tt.Find(locales.AgentChoose())
	send, sendOK := hs.tt.Find(locales.AgentSend())
	if !inputOK || !targetOK || !sendOK || target.Y < input.Y+input.H || math.Abs(float64(send.Y+send.H/2-target.Y-target.H/2)) > 1 {
		t.Fatalf("target and send do not share the composer footer: input=%+v target=%+v send=%+v", input, target, send)
	}
	hs.openAgentMenu(locales.AgentRoster())
	for _, label := range []string{locales.AgentConfigure(), locales.AgentRolesConfigure(), locales.CmdAgentReviewChanges()} {
		if _, found := hs.tt.Find(label); found {
			t.Errorf("agent list includes unrelated action %q", label)
		}
	}
	hs.key(0, ui.KeyEscape)
	if _, found := hs.tt.Find(locales.RoundtableOpen()); !found {
		t.Fatal("Sessions has no direct roundtable action")
	}
	if err := hs.tt.Click(locales.AgentChoose()); err != nil {
		t.Fatal(err)
	}
	if !hs.tt.HasText("Alpha · "+locales.AgentOnline()) || !hs.tt.HasText("Beta · "+locales.AgentOffline()) || a.agent.turn != 0 || a.agentState("beta") != nil {
		t.Fatal("opening the target picker hid connection status or started another chat")
	}
}

func TestAgentContextActionsStayOutsideLauncher(t *testing.T) {
	hs, d := editHarness(t, "agent.go", "first()\nsecond()\n")
	hs.place(d, 0, len("first()"))
	hs.a.showAgent()
	hs.settle()
	hs.openAgentMenu(locales.AgentRoster())
	var entries []string
	for _, item := range hs.openAgentMenuItems() {
		if item != nil {
			entries = append(entries, item.label)
		}
	}
	for _, label := range []string{locales.CmdAgentReviewChanges(), locales.CmdAgentAddSelection(), locales.CmdAgentAddFile(), locales.CmdAgentAddReview()} {
		if slices.Contains(entries, label) {
			t.Errorf("the launcher still offers %q", label)
		}
	}
	hs.key(0, ui.KeyEscape)
	hs.a.run("agent.addSelection")
	hs.settle()
	r := hs.a.agent
	if !strings.Contains(r.draft, "first()") || r.client != nil || r.connecting || len(r.messages) != 0 {
		t.Fatal("adding context did not preserve it as an unsent draft")
	}
	if !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("adding context did not return typing to the composer")
	}
}

func TestAgentPageKeepsDraftSelection(t *testing.T) {
	for _, move := range []string{"switch", "split", "reactivate"} {
		t.Run(move, func(t *testing.T) {
			hs := newHarness(t, true)
			agentViewFixture(hs)
			a := hs.a
			hs.settle()
			hs.tt.Type("hello world")
			hs.settle()
			for range 6 {
				hs.key(0, ui.KeyLeft)
			}
			for range 5 {
				hs.key(ui.Shift, ui.KeyLeft)
			}
			switch move {
			case "switch":
				original := a.agent
				a.newChatPage()
				hs.settle()
				a.openChatPage(original)
			case "split":
				original := a.agent
				a.splitRight()
				hs.settle()
				if a.agent == original || a.agent.draft != "" {
					t.Fatal("a split chat borrowed the original draft")
				}
				a.focusColumnBy(-1)
			case "reactivate":
				a.openFile(hs.path("main.go"), true)
				hs.settle()
				a.showAgent()
			}
			hs.settle()
			hs.tt.Type("new")
			hs.settle()
			if a.agent.draft != "new world" {
				t.Fatalf("%s lost the draft selection: %q", move, a.agent.draft)
			}
		})
	}
}

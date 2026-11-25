package main

import (
	"math"
	"slices"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"
)

func (hs *harness) openAgentMenuItems() []*agentMenuItem {
	if !hs.a.agentMenu.open {
		return nil
	}
	return hs.a.agentMenuItems(hs.a.agentMenu.agentMenuOrigin)
}

func (hs *harness) openAgentMenu(button string) {
	hs.t.Helper()
	launcher := button == locales.AgentRoster()
	if launcher && !hs.a.sessionsVisible() {
		hs.a.run("view.sessions")
		hs.settle()
	}
	if err := hs.tt.Click(button); err != nil {
		hs.t.Fatal(err)
	}
	if menu := &hs.a.agentMenu; !menu.open || menu.launcher != launcher {
		hs.t.Fatalf("clicking %q did not open its menu", button)
	}
}

func (hs *harness) chooseSessionsAgentItem(label string) {
	hs.t.Helper()
	if !hs.a.sessionsVisible() {
		hs.t.Fatalf("Sessions must already be visible when choosing %q", label)
	}
	hs.openAgentMenu(locales.AgentRoster())
	if err := hs.tt.Click(label); err != nil {
		hs.t.Fatal(err)
	}
	hs.settle()
}

func TestSessionsAgentMenuOpensOnlyChosenTarget(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	a, original := hs.a, hs.a.agent
	a.settings.AgentServers["local"] = settings.AgentServer{Name: "Local", Protocol: "cli", Command: "local"}
	hs.openAgentMenu(locales.AgentRoster())
	offline := " · " + locales.AgentOffline()
	for _, entry := range []string{"Backend" + offline, "Local" + offline} {
		if _, found := hs.tt.Find(entry); !found {
			t.Fatalf("entry %q missing from launcher", entry)
		}
	}
	var labels []string
	for _, item := range hs.openAgentMenuItems() {
		labels = append(labels, item.label)
	}
	if !slices.Equal(labels, []string{"Backend", "Local"}) {
		t.Fatalf("expected only registered agents, got %q", labels)
	}
	hs.key(0, ui.KeyEscape)
	if agentPageCount(a) != 0 || original.connectionAttempted || a.agent != original {
		t.Fatal("opening the menu alone opened or connected a chat")
	}
	hs.chooseSessionsAgentItem("Local" + offline)
	if a.agent.provider != "local" || a.agent.role != "" || !hs.tt.Focused(locales.AgentDraft()) || agentPageCount(a) != 1 || a.window.SidebarView != "sessions" || original.connectionAttempted {
		t.Fatal("choosing a provider should open only its workspace chat and keep sidebar history")
	}
	requireChatHandshakes(t, logPath, 0)
	localChat := a.agent
	localChat.draft = "keep my draft"
	hs.chooseSessionsAgentItem("Backend" + offline)
	hs.untilLong("selected agent connected", func() bool { return a.agent.sessionID != "" })
	if a.agent == original || a.agent.hasRole() || a.agent.provider != "backend" || agentPageCount(a) != 1 {
		t.Fatal("the Sessions menu did not activate the direct agent chat in the current tab")
	}
	requireChatHandshakes(t, logPath, 1)
	hs.chooseSessionsAgentItem("Local" + offline)
	if a.agent != localChat || localChat.draft != "keep my draft" {
		t.Fatal("switching back did not restore the existing draft")
	}
}

func TestAgentPreferencesOpenFocusedConversation(t *testing.T) {
	ph := newPrefsHarness(t)
	hs, a := ph.harness, ph.a
	a.showSessions()
	agentViewFixture(hs)
	first := a.agent
	first.systemPrompt, first.draft = "first prompt", "first draft"
	a.splitRight()
	second := a.agent
	second.systemPrompt, second.draft = "", "second draft"
	second.messages = []*agentMessage{{role: "user", text: "second question"}}
	hs.settle()
	prefsAgentClick(t, ph, prefSection(settings.CatAgent))
	prefsAgentClick(t, ph, locales.AgentConversationSettings())
	a.openChatPage(first)
	ph.frames()
	if !ph.pt.HasText("second question") || ph.w.conversation.chat != second {
		t.Fatal("the settings window did not retain its original conversation")
	}
	field := ph.w.firstH.Bounds(ph.c)
	ph.pt.ClickAt(field.X+5, field.Y+5)
	ph.pt.Type("second prompt")
	if second.systemPrompt != "second prompt" || first.systemPrompt != "first prompt" {
		t.Fatal("editing the prompt changed the wrong chat")
	}
	ph.pt.Key(0, ui.KeyTab)
	ph.pt.Key(0, ui.KeyUp)
	if second.contextMessages != 11 || first.contextMessages != 10 {
		t.Fatalf("recent message limits are not scoped: first=%d second=%d", first.contextMessages, second.contextMessages)
	}
	if err := ph.pt.Click(locales.AgentClearChat()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if len(second.messages) != 0 || second.draft != "second draft" || first.draft != "first draft" || second.systemPrompt != "second prompt" {
		t.Fatal("clearing the conversation changed other settings or a draft")
	}
	if hs.h.showed != nil {
		t.Fatal("clearing an ordinary chat moved focus away from Settings")
	}
	col, index := a.chatTab(second)
	a.closeTab(col.tabs[index], nil, nil)
	ph.frames()
	if ph.w.conversation != nil || ph.pt.HasText(locales.AgentClearChat()) {
		t.Fatal("closing the target left a stale form or fell back to another chat")
	}
}

func TestConversationSettingsReturnShowsOwningChat(t *testing.T) {
	ph := newPrefsHarness(t)
	project := ph.a
	agentViewFixture(ph.harness)
	direct := project.agent
	direct.draft = "keep my draft"
	project.continueSessionWith(continuationSource(t, ph.root), direct.provider)
	history := project.agent
	ph.until("loaded history", func() bool { return history.continuation.loaded })
	ph.w.navigate(settings.CatAgent, prefsConversationKey, ph.m.wins[0])
	ph.frames()
	if err := ph.pt.Click(locales.SessionsReturnToChat()); err != nil {
		t.Fatal(err)
	}
	ph.settle()
	if ph.h.showed != project || project.agent != direct || !project.chatShown(direct) || direct.draft != "keep my draft" || len(history.messages) != 2 {
		t.Fatal("returning from Settings did not show the original chat with its draft and imported history intact")
	}
}

func TestAgentMenusKeepChoiceAcrossStatusChange(t *testing.T) {
	for _, button := range []string{locales.AgentChoose(), locales.AgentRoster()} {
		t.Run(button, func(t *testing.T) {
			hs := agentWorkspaceHarness(t)
			project := hs.a
			project.settings.AgentServers["other"] = settings.AgentServer{Name: "Other", Command: "other", Protocol: "cli"}
			other := &agentChat{agentTarget: agentTarget{provider: "other"}, group: project.agent.group, contextMessages: 10, draft: "other draft"}
			project.rememberChat(other)
			project.settings.ZoomLevel = 1
			hs.settle()
			hs.openAgentMenu(button)
			bounds, found := hs.tt.Find(button)
			row, visible := hs.tt.Find("Other · " + locales.AgentOffline())
			if !found || !visible || row.W <= 0 {
				t.Fatalf("zoomed menu is missing its trigger or target: button=%v row=%v", bounds, row)
			}
			if button == locales.AgentChoose() {
				if row.Y+row.H > bounds.Y || math.Abs(float64(row.X+row.W-bounds.X-bounds.W)) > 14 {
					t.Fatalf("zoomed row %v is not anchored above composer button %v", row, bounds)
				}
			} else if center := bounds.X + bounds.W/2; row.Y < bounds.Y+bounds.H || center < row.X || center > row.X+row.W {
				t.Fatalf("zoomed row %v is not anchored below Sessions button %v", row, bounds)
			}
			other.busy = true
			hs.settle()
			if err := hs.tt.Click("Other · " + locales.AgentWorking()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if project.agent != other || other.draft != "other draft" || !other.busy {
				t.Fatal("a status update discarded the chosen target or changed its conversation")
			}
		})
	}
}

func TestAgentMenusIgnoreStaleTargets(t *testing.T) {
	for _, button := range []string{locales.AgentChoose(), locales.AgentRoster()} {
		for _, change := range []string{"disable", "switch chat", "change folder"} {
			t.Run(button+"/"+change, func(t *testing.T) {
				hs := agentWorkspaceHarness(t)
				project := hs.a
				other := settings.AgentServer{Name: "Other", Command: "other", Protocol: "cli"}
				project.settings.AgentServers["other"] = other
				hs.settle()
				hs.openAgentMenu(button)
				stale := findMarkedMenuTarget(t, hs.openAgentMenuItems(), "Other", locales.AgentOffline())
				switch change {
				case "disable":
					other.Disabled = true
					project.settings.AgentServers["other"] = other
				case "switch chat":
					project.newChatPage()
				case "change folder":
					project.openFolder(workspaceFixture(t))
				}
				hs.settle()
				current := project.agent
				stale.choose()
				hs.settle()
				if project.agent != current || project.agentState("other") != nil {
					t.Fatal("a stale menu opened a target that is no longer available in this chat")
				}
			})
		}
	}
}

func TestAgentMenusShowTargetStatusWithoutStartingAgents(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	project, chat := hs.a, hs.a.agent
	project.settings.AgentServers["disabled"] = settings.AgentServer{Name: "Disabled", Command: "disabled", Disabled: true}
	project.settings.AgentRoles = map[string]settings.AgentRole{"Review": {Agent: chat.provider}}
	for _, status := range []agentStatusMark{
		{label: locales.AgentOffline(), icon: uiicons.Link2Off()}, {label: locales.AgentConnecting(), icon: uiicons.LoaderCircle()}, {label: locales.AgentOnline(), icon: uiicons.CircleCheck()},
		{label: locales.AgentWorking(), icon: uiicons.Activity()}, {label: locales.AgentStatusFailed(), icon: uiicons.TriangleAlert()},
		{label: locales.AgentPermission(), icon: uiicons.LockKeyhole()},
	} {
		chat.connecting, chat.busy = status.label == locales.AgentConnecting(), status.label == locales.AgentWorking()
		chat.sessionID, chat.err = "", ""
		if status.label == locales.AgentOnline() {
			chat.sessionID = "online"
		}
		if status.label == locales.AgentStatusFailed() {
			chat.err = "connection failed"
		}
		if status.label == locales.AgentPermission() {
			chat.permissions = []*agentPermission{{title: "Approval"}}
		}
		hs.settle()
		for _, menu := range []string{locales.AgentChoose(), locales.AgentRoster()} {
			hs.openAgentMenu(menu)
			items := hs.openAgentMenuItems()
			if len(items) != 2 {
				t.Fatalf("%s: got %d entries for two agents", menu, len(items))
			}
			for _, want := range []struct {
				name, label string
				icon        *ui.SVG
			}{
				{"Demo Agent", status.label, status.icon}, {"Disabled", locales.AgentDisabled(), uiicons.Ban()},
			} {
				item := findMarkedMenuTarget(t, items, want.name, want.label)
				if _, found := hs.tt.Find(want.name + " · " + want.label); !found || item.status.icon != want.icon || item.disabled != (want.name == "Disabled") {
					t.Fatalf("%s: wrong status icon, accessible label or availability for %s", menu, want.name)
				}
			}
			hs.key(0, ui.KeyEscape)
		}
	}
	if chat.client != nil || chat.turn != 0 || agentPageCount(project) != 1 {
		t.Fatal("reading status started an agent or opened another page")
	}
}

func TestAgentMenuMarksAndStatusIcons(t *testing.T) {
	hs := newHarness(t, false)
	project := hs.a
	project.settings.AgentServers = map[string]settings.AgentServer{}
	names := []string{"Pi", "OMP", "OpenCode", "Claude Code", "Codex", "Grok Build"}
	for _, name := range names {
		project.settings.AgentServers[name] = settings.AgentServer{Name: name, Command: "unused", Protocol: "cli"}
	}
	project.settings.AgentRoles = map[string]settings.AgentRole{"Reviewer": {Agent: "Pi"}}
	project.openAgentPage()
	project.selectAgent("Pi")
	hs.settle()
	for _, button := range []string{locales.AgentChoose(), locales.AgentRoster()} {
		hs.openAgentMenu(button)
		items := hs.openAgentMenuItems()
		marks := map[*ui.SVG]string{}
		for _, name := range names {
			item := findMarkedMenuTarget(t, items, name, locales.AgentOffline())
			if other := marks[item.brand]; other != "" || item.checked != (name == "Pi") {
				t.Fatalf("%s: %s shared the mark for %s or lost its selection", button, name, other)
			}
			marks[item.brand] = name
		}
		if len(items) != len(names) {
			t.Fatalf("%s: unexpected entries in the agent list: %d", button, len(items))
		}
		hs.key(0, ui.KeyEscape)
	}
}

func findMarkedMenuTarget(t *testing.T, items []*agentMenuItem, name, status string) *agentMenuItem {
	t.Helper()
	for _, item := range items {
		if item == nil || item.label != name {
			continue
		}
		if item.brand == nil || item.status.icon == nil || item.status.label != status {
			t.Fatalf("%s: expected separate brand and status icons with tooltip %q, got %+v", name, status, item)
		}
		return item
	}
	t.Fatalf("missing %s", name)
	return nil
}

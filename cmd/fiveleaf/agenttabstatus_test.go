package main

import (
	"bytes"
	"image"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestAgentTabTaskLifecycle(t *testing.T) {
	for _, protocol := range []string{"acp", "cli"} {
		t.Run(protocol, func(t *testing.T) {
			hs := newHarness(t, true)
			app, chat := hs.a, hs.a.agent
			working := roundtableTestServer(t, "Demo", protocol)
			broken := working
			broken.Command = filepath.Join(t.TempDir(), "missing-agent")
			app.settings.AgentServers = map[string]settings.AgentServer{"demo": broken}
			const question = "Review this change"
			chat.draft = question
			app.openAgentPage()
			hs.untilLong("initial connection attempt", func() bool { return !chat.connecting })
			if chat.taskState() != agentTaskIdle {
				t.Fatal("opening a chat with a draft started a task")
			}
			app.sendChat(chat)
			if chat.taskState() != agentTaskRunning {
				t.Fatal("submitting a task did not mark it running")
			}
			hs.untilLong("failed send", func() bool { return !chat.requestInFlight() })
			if chat.taskState() != agentTaskFailed || chat.draft != question || chat.err == "" {
				t.Fatalf("failed task: state=%v draft=%q err=%q", chat.taskState(), chat.draft, chat.err)
			}
			if !slices.Contains(hs.tt.Announcements(), locales.AgentStatusFailed()) {
				t.Fatal("the failed send did not show a task notification")
			}
			app.settings.AgentServers["demo"] = working
			app.beginChat(chat, "")
			hs.untilLong("reconnection", func() bool { return !chat.connecting })
			if chat.err != "" || chat.taskState() != agentTaskFailed {
				t.Fatalf("reconnecting changed the task result: state=%v err=%q", chat.taskState(), chat.err)
			}
			app.sendChat(chat)
			if chat.taskState() != agentTaskRunning {
				t.Fatal("retry still shows the failed turn")
			}
			hs.untilLong("reply to retry", func() bool { return !chat.requestInFlight() })
			if !slices.Contains(hs.tt.Announcements(), locales.AgentStatusCompleted()) {
				t.Fatal("the completed retry did not show a task notification")
			}
			app.disconnectChat(chat)
			if chat.taskState() != agentTaskCompleted {
				t.Fatalf("completed turn lost after disconnect: state=%v err=%q", chat.taskState(), chat.err)
			}
			chat.draft = "Stop this turn"
			app.sendChat(chat)
			app.stopChat(chat)
			hs.settle()
			if chat.taskState() != agentTaskIdle || chat.draft != "Stop this turn" {
				t.Fatalf("stopped task: state=%v draft=%q", chat.taskState(), chat.draft)
			}
			if !slices.Contains(hs.tt.Announcements(), locales.AgentTaskIdle()) {
				t.Fatal("stopping the task did not notify that it returned to idle")
			}
		})
	}
}

func TestAgentTabTaskACPStopReason(t *testing.T) {
	for reason, want := range map[string]agentTaskState{
		"cancelled": agentTaskIdle, "refusal": agentTaskFailed, "max_tokens": agentTaskFailed, "max_turn_requests": agentTaskFailed,
	} {
		t.Run(reason, func(t *testing.T) {
			hs, _, _ := lifecycleHarness(t, reason)
			lifecycleSend(t, hs, "Try this task")
			chat := hs.a.agent
			if chat.sessionID == "" || chat.err != "" || chat.taskState() != want {
				t.Fatalf("stop reason %s: state=%v err=%q", reason, chat.taskState(), chat.err)
			}
		})
	}
}

func TestAgentTabTaskReplyAndDisconnect(t *testing.T) {
	for _, scenario := range []string{"quiet-reply", "exit-before-reply"} {
		t.Run(scenario, func(t *testing.T) {
			hs, _, _ := lifecycleHarness(t, scenario)
			app, chat := hs.a, hs.a.agent
			app.connectAgent()
			hs.untilLong("connected", func() bool { return chat.sessionID != "" })
			client := chat.client
			const question = "Last question"
			chat.draft = question
			app.sendAgent()
			want, draft := agentTaskFailed, question
			if scenario == "quiet-reply" {
				want, draft = agentTaskCompleted, ""
				deadline := time.Now().Add(5 * time.Second)
				for {
					if len(chat.messages) == 0 {
						chat.events.drain()
					} else {
						chat.events.mu.Lock()
						queued := len(chat.events.queue) > 0
						chat.events.mu.Unlock()
						if queued {
							break
						}
					}
					if time.Now().After(deadline) {
						t.Fatal("the prompt result did not reach the UI queue")
					}
					time.Sleep(time.Millisecond)
				}
				client.Close()
				app.disconnectExitedChat(chat, client)
			}
			hs.untilLong("disconnected after prompt", func() bool { return chat.client == nil && !chat.busy })
			if chat.taskState() != want || chat.draft != draft || chat.err == "" {
				t.Fatalf("after disconnect: state=%v draft=%q err=%q", chat.taskState(), chat.draft, chat.err)
			}
		})
	}
}

func TestAgentTabTaskIconsHoldStill(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans", "ja"} {
		t.Run(locale, func(t *testing.T) {
			previous := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
			hs := agentWorkspaceHarness(t)
			app := hs.a
			app.window.SidebarClosed = true
			hs.tt.SetScale(2)
			var closeLabels []string
			first := app.activeTab()
			for index, item := range []struct {
				state         agentTaskState
				title, status string
			}{
				{agentTaskRunning, "检查索引", locales.AgentTaskRunning()}, {agentTaskApproval, "更新依赖", locales.AgentTaskApproval()},
				{agentTaskCompleted, "整理日志", locales.AgentStatusCompleted()}, {agentTaskFailed, "运行测试", locales.AgentStatusFailed()}, {agentTaskIdle, "", locales.AgentTaskIdle()},
			} {
				if index > 0 {
					app.newChatPage()
				}
				chat := app.agent
				if item.title != "" {
					chat.messages = []*agentMessage{{role: "user", text: item.title}}
				}
				setTabTaskFixture(chat, item.state)
				last := app.activeTab()
				closeLabels = append(closeLabels, last.closeLabel())
				hs.settle()
				tab, status, close := last.name(), item.status, last.closeLabel()
				bounds := hs.layoutOf(tab, status, close)
				if slot := bounds[status]; slot.W <= 0 || slot.W > 16 || slot.W != slot.H {
					t.Fatalf("status %q does not fit a square icon slot: %v", status, slot)
				}
				if bounds[status].X <= bounds[tab].X || bounds[status].X+bounds[status].W > bounds[close].X || bounds[status].Y >= bounds[tab].Y+bounds[tab].H {
					t.Fatalf("status %q is not between its title and close button: %v", status, bounds)
				}
			}
			last := app.activeTab()
			edges := hs.layoutOf(first.name(), last.closeLabel())
			left, right := edges[first.name()], edges[last.closeLabel()]
			hs.shotRect("agent-tab-tasks-"+locale, image.Rect(int(left.X)-4, int(left.Y)-3, int(right.X+right.W)+8, int(left.Y+left.H)+3), 2)
			rest := hs.layoutOf(closeLabels...)
			for _, state := range []agentTaskState{agentTaskApproval, agentTaskRunning, agentTaskCompleted, agentTaskFailed, agentTaskIdle} {
				setTabTaskFixture(first.tool.chat, state)
				hs.settle()
				label, _, _ := state.mark(app.pal)
				visible := hs.layoutOf(label)[label]
				if visible.X < left.X || visible.X+visible.W > rest[first.closeLabel()].X {
					t.Fatalf("background tab did not update to %q: %v", label, visible)
				}
				sameLayout(t, label, rest, hs.layoutOf(closeLabels...))
			}
		})
	}
}

func TestAgentTabTaskIconMotion(t *testing.T) {
	workbench := &app{pal: darkPal}
	var chat agentChat
	tt := ui.NewTester(func(c *ui.Context) {
		workbench.chatTabStatus(c, &chat, darkPal.strip)
	}, 32, 32)
	for _, test := range []struct {
		state  agentTaskState
		reduce bool
	}{
		{agentTaskRunning, false}, {agentTaskApproval, false}, {agentTaskRunning, false},
		{agentTaskCompleted, false}, {agentTaskFailed, false}, {agentTaskIdle, false}, {agentTaskRunning, true},
	} {
		setTabTaskFixture(&chat, test.state)
		tt.SetPreferences(ui.Preferences{ReduceMotion: test.reduce})
		tt.Frame()
		wantMotion := test.state == agentTaskRunning && !test.reduce
		for range 3 {
			before := bytes.Clone(tt.Image().Pix)
			tt.Advance(125 * time.Millisecond)
			moved := !bytes.Equal(before, tt.Image().Pix)
			if moved != wantMotion {
				t.Fatalf("state %v, reduced motion %v: icon moved = %v, want %v", test.state, test.reduce, moved, wantMotion)
			}
		}
	}
}

func TestAgentTaskNoticesFollowBackgroundChat(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	workspace, tester := hs.a, hs.tt
	chat := workspace.agent
	const title = "Check the imports"
	chat.messages = []*agentMessage{{role: "user", text: title}}
	workspace.newChatPage()
	sibling := workspace.agent
	hs.settle()
	action := locales.AgentViewChat()
	if tester.HasText(action) {
		t.Fatal("opening an idle conversation showed a task notification")
	}
	tester.Announcements()
	handler := workspace.chatHandler(chat)
	for _, state := range []agentTaskState{agentTaskRunning, agentTaskApproval, agentTaskRunning, agentTaskCompleted} {
		handler.post(func() { setTabTaskFixture(chat, state) })
		hs.settle()
		status, _, _ := state.mark(workspace.pal)
		if got := tester.Announcements(); !slices.Equal(got, []string{title, status}) {
			t.Fatalf("notification for %s: got %q", status, got)
		}
		var buttons int
		for _, label := range tester.Texts() {
			if label == action {
				buttons++
			}
		}
		if buttons != 1 || workspace.agent != sibling {
			t.Fatalf("%s: found %d notification actions, selected original chat = %v", status, buttons, workspace.agent == chat)
		}
		handler.post(func() { chat.messages = append(chat.messages, &agentMessage{role: "assistant", text: "More output"}) })
		hs.settle()
		if got := tester.Announcements(); slices.Contains(got, title) {
			t.Fatalf("streaming output repeated the notification: %q", got)
		}
	}
	if err := tester.Click(action); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if workspace.agent != chat || !workspace.chatShown(chat) || tester.HasText(action) {
		t.Fatal("the notification did not open its own chat and dismiss")
	}
}

func TestAgentTaskNoticeFromBackgroundProject(t *testing.T) {
	frame, projects, tester := railFixture(t, t.TempDir(), t.TempDir())
	owner := projects[0].a
	chat := owner.agent
	chat.connectionAttempted = true
	const title = "Review background project"
	chat.messages = []*agentMessage{{role: "user", text: title}}
	owner.openChatPage(chat)
	handler := owner.chatHandler(chat)
	frame.active = projects[1]
	step(tester)
	handler.post(func() { setTabTaskFixture(chat, agentTaskCompleted) })
	step(tester)
	if !tester.HasText(title) || !tester.HasText(locales.AgentStatusCompleted()) || frame.active != projects[1] {
		t.Fatal("the background task notification did not appear in the current project")
	}
	if err := tester.Click(locales.AgentViewChat()); err != nil {
		t.Fatal(err)
	}
	if owner.host.(*fakeHost).showed != owner || !owner.chatShown(chat) {
		t.Fatal("the notification did not bring its owning project and chat forward")
	}
}

func setTabTaskFixture(chat *agentChat, state agentTaskState) {
	chat.busy = state == agentTaskRunning || state == agentTaskApproval
	chat.permissions = nil
	if state == agentTaskApproval {
		chat.permissions = []*agentPermission{{title: "Allow?"}}
	}
	if !chat.busy {
		chat.lastTask = state
	}
}

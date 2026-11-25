package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
)

func agentRoleViewHarness(t *testing.T) *harness {
	t.Helper()
	hs := agentRosterHarness(t)
	hs.a.settings.AgentRoles = map[string]settings.AgentRole{
		"代码审查": {Agent: "alpha", Model: "review-model", Prompt: "Review correctness and cite file paths."},
		"架构设计": {Agent: "alpha", Prompt: "Explain the design and its tradeoffs."},
	}
	hs.settle()
	return hs
}

func TestAgentRoleTargetsKeepIndependentViews(t *testing.T) {
	hs := agentRoleViewHarness(t)
	a := hs.a
	direct := a.agent
	direct.draft = "Direct draft"
	a.selectAgentRole("代码审查")
	hs.settle()
	reviewer := a.agent
	if reviewer == direct || reviewer.role != "代码审查" || reviewer.provider != "alpha" {
		t.Fatal("the role did not open its own conversation")
	}
	if err := hs.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	hs.tt.Type("Review this change")
	reviewer.messages = []*agentMessage{{role: "assistant", text: strings.Repeat("Review paragraph.\n\n", 50)}}
	hs.settle()
	reviewer.scroll.Y = reviewer.scroll.MaxY / 2
	hs.settle()
	scroll := reviewer.scroll.Y
	reviewer.busy = true
	a.selectAgentRole("架构设计")
	hs.settle()
	designer := a.agent
	designer.draft = "Design draft"
	if designer == reviewer || designer.role != "架构设计" || a.agentState("alpha") != direct {
		t.Fatal("roles sharing an Agent mixed with one another or with its direct chat")
	}
	a.selectAgentRole("代码审查")
	hs.settle()
	if a.agent != reviewer || reviewer.draft != "Review this change" || !reviewer.busy || reviewer.scroll.Y != scroll {
		t.Fatalf("role switch lost draft, running reply or scroll: draft=%q busy=%v scroll=%v/%v", reviewer.draft, reviewer.busy, reviewer.scroll.Y, scroll)
	}
	sizeChatWorkspace(hs, 340)
	reviewer.permissions = []*agentPermission{{title: "Waiting", options: []acp.PermissionOption{{OptionId: "once", Name: "Allow", Kind: "allow_once"}}}}
	hs.settle()
	if err := hs.tt.Click(locales.AgentChoose()); err != nil {
		t.Fatal(err)
	}
	if _, found := hs.tt.Find("代码审查 · " + locales.AgentPermission()); found {
		t.Fatal("the agent list includes a role with a pending permission")
	}
	if err := hs.tt.Click("Alpha · " + locales.AgentOnline()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.agent != direct || direct.role != "" || direct.draft != "Direct draft" || designer.draft != "Design draft" {
		t.Fatal("choosing the underlying Agent returned its role or lost a draft")
	}
	for _, state := range []*agentChat{direct, reviewer, designer} {
		if state.turn != 0 || state.pendingPrompt != "" {
			t.Fatal("using the target controls sent a prompt")
		}
	}
}

func TestAgentRoleUnavailableViewKeepsDraft(t *testing.T) {
	for _, reason := range []string{"missing-agent", "disabled-agent", "deleted-role", "invalid-model"} {
		t.Run(reason, func(t *testing.T) {
			hs := agentRoleViewHarness(t)
			a := hs.a
			a.selectAgentRole("代码审查")
			r := a.agent
			r.draft = "Keep this unsent question"
			r.messages = []*agentMessage{{role: "user", text: "Earlier question"}}
			switch reason {
			case "missing-agent":
				delete(a.settings.AgentServers, "alpha")
			case "disabled-agent":
				server := a.settings.AgentServers["alpha"]
				server.Disabled = true
				a.settings.AgentServers["alpha"] = server
			case "deleted-role":
				delete(a.settings.AgentRoles, "代码审查")
			case "invalid-model":
				role := a.settings.AgentRoles["代码审查"]
				role.Model = " bad-model"
				a.settings.AgentRoles["代码审查"] = role
			}
			hs.settle()
			if !hs.tt.HasText("Earlier question") {
				t.Fatal("an unavailable target lost its transcript")
			}
			if _, found := hs.tt.Find(locales.AgentRolesConfigure()); !found {
				t.Fatal("an unavailable role has no configuration action")
			}
			if _, found := hs.tt.Find(locales.AgentConnect()); found {
				t.Fatal("an unavailable role still offers Connect")
			}
			if err := hs.tt.Click(locales.AgentSend()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if r.draft != "Keep this unsent question" || len(r.messages) != 1 || r.client != nil || r.connecting || r.busy {
				t.Fatal("Send used an unavailable role or discarded its draft")
			}
		})
	}
}

func TestAgentRoleViewNarrowLocales(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans", "ja"} {
		t.Run(locale, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			hs := agentRoleViewHarness(t)
			hs.a.selectAgentRole("代码审查")
			hs.until("role connection settled", func() bool { return !hs.a.agent.connecting })
			for _, width := range []float32{160, 340, 600} {
				sizeChatWorkspace(hs, width)
				hs.settle()
				for _, label := range []string{locales.AgentChoose(), locales.AgentDraft(), locales.AgentSend(), locales.AgentNewSession()} {
					bounds, found := hs.tt.Find(label)
					if !found || bounds.X < 0 || bounds.X+bounds.W > width+1 || bounds.H <= 0 {
						t.Errorf("%q is outside a %v-wide role chat: %+v (found=%v)", label, width, bounds, found)
					}
				}
			}
		})
	}
}

func TestAgentRoleConversationUsesSavedPrompt(t *testing.T) {
	ph := newPrefsHarness(t)
	a := ph.a
	a.settings.AgentServers = map[string]settings.AgentServer{"alpha": {Name: "Alpha", Protocol: "cli", Command: "local"}}
	a.settings.AgentRoles = map[string]settings.AgentRole{"代码审查": {Agent: "alpha", Model: "review-model", Prompt: "Review correctness."}}
	a.selectAgentRole("代码审查")
	a.openAgentPage()
	prefsAgentClick(t, ph, prefSection(settings.CatAgent))
	prefsAgentClick(t, ph, locales.AgentConversationSettings())
	if !ph.pt.HasText(a.settings.AgentRoles["代码审查"].Prompt) || !ph.pt.HasText(locales.PrefsRoleEdit()) {
		t.Fatal("the existing role chat's settings should show its saved prompt and role editor")
	}
	a.agent.draft = "Keep draft"
	if err := ph.pt.Click(locales.AgentClearChat()); err != nil {
		t.Fatal(err)
	}
	ph.settle()
	if a.agent.role != "代码审查" || a.agent.draft != "Keep draft" || !ph.pt.HasText("代码审查 · Alpha · review-model") {
		t.Fatal("clearing the role chat lost its target, model or draft")
	}
}

func TestAgentRoleTargetsExcludeEmptyName(t *testing.T) {
	hs := agentRoleViewHarness(t)
	hs.a.settings.AgentRoles[""] = settings.AgentRole{Agent: "beta", Model: "wrong-target", Prompt: "Invalid role"}
	sizeChatWorkspace(hs, 340)
	hs.settle()
	if slices.Contains(hs.a.agentRoleNames(), "") {
		t.Fatal("an empty role name would select a direct Agent instead of a role")
	}
	if choices := hs.a.meetingInvitations(); slices.ContainsFunc(choices, func(s meetingSpeaker) bool { return s.ID == "role:" }) || !slices.ContainsFunc(choices, func(s meetingSpeaker) bool { return s.ID == "role:代码审查" }) {
		t.Fatal("roundtable invitations included an empty role or omitted a named one")
	}
	if hs.a.settings.AgentRoles[""].Agent != "beta" {
		t.Fatal("rendering removed an invalid role instead of leaving it editable in settings")
	}
}

func TestShotAgentRoles(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	withAppearancePlugins(t)
	for _, locale := range []string{"en", "zh-Hans", "ja"} {
		t.Run(locale, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			hs := agentRoleViewHarness(t)
			hs.a.settings.Theme = themeOneDarkPro
			hs.a.openFile(hs.path("main.go"), true)
			hs.a.selectAgentRole("代码审查")
			hs.a.agent.messages = []*agentMessage{
				{role: "user", text: "帮我检查这段代码，重点看边界条件。"},
				{role: "assistant", text: "这段入口函数打印一条问候语。\n\n```go\nfmt.Println(\"hello, fiveleaf\")\n```\n\n可以继续把待审查的选区添加到输入框。"},
			}
			hs.a.agent.draft = "继续检查错误处理"
			hs.settle()
			hs.shot("agent-roles-" + locale)
		})
	}
}

package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestSessionsFilterKeepsSameTitleConversations(t *testing.T) {
	harness := newHarness(t, true)
	app := harness.a
	app.sessions.sessions = []agents.Session{
		{Harness: agents.Codex, ID: "thread-one", Title: "Same title", Dir: app.root},
		{Harness: agents.Codex, ID: "thread-two", Title: "Same title", Dir: app.root},
		{Harness: agents.Pi, ID: "pi-one", Title: "Pi task", Dir: app.root},
		{Harness: agents.Pi, ID: "pi-other", Title: "Other project", Dir: app.root + "-other"},
	}
	rows := app.sessionRows()
	if got := sessionIDs(rows); got != "thread-one thread-two pi-one" {
		t.Fatalf("directory filter returned %q", got)
	}
	if app.sessionMeta(rows[0]) == app.sessionMeta(rows[1]) || !strings.Contains(app.sessionMeta(rows[0]), "read-one") {
		t.Fatal("independent conversations with matching titles cannot be distinguished")
	}
	app.sessions.harness = agents.Pi
	if got := sessionIDs(app.sessionRows()); got != "pi-one" {
		t.Fatalf("Pi filter returned %q", got)
	}
	app.sessions.all = 1
	if got := sessionIDs(app.sessionRows()); got != "pi-one pi-other" {
		t.Fatalf("all-project filter returned %q", got)
	}
	app.sessions.query = "other"
	if got := sessionIDs(app.sessionRows()); got != "pi-other" {
		t.Fatalf("search returned %q", got)
	}
	app.sessionLooks()
	counts := app.sessionAgentCounts()
	if len(counts) != 1 || counts[0].harness != agents.Pi || counts[0].count != 1 {
		t.Fatalf("search counts = %+v", counts)
	}
}

func TestSessionsAgentFilterMenu(t *testing.T) {
	harness := newHarness(t, true)
	app := harness.a
	agentHome(t, harness.root)
	app.showSessions()
	harness.settle()
	app.sessions.sessions = append(app.sessions.sessions, agents.Session{Harness: agents.Pi, ID: "pi-one", Title: "Pi task", Dir: app.root, Updated: time.Now()})
	app.sessions.version++
	harness.settle()
	if err := harness.tt.Click(locales.SessionsFilterAgent()); err != nil {
		t.Fatal(err)
	}
	if err := harness.tt.ChooseMenuItem("Pi · 1"); err != nil {
		t.Fatal(err)
	}
	harness.settle()
	if app.sessions.harness != agents.Pi || !harness.tt.HasText("Pi task") || harness.tt.HasText("fix here") {
		t.Fatalf("filter did not change visible conversations: %v", harness.tt.Texts())
	}
	if err := harness.tt.Click(locales.SessionsFilterAgent()); err != nil {
		t.Fatal(err)
	}
	if err := harness.tt.ChooseMenuItem(locales.SessionsAllAgents()); err != nil {
		t.Fatal(err)
	}
	harness.settle()
	if !harness.tt.HasText("fix here") || !harness.tt.HasText("Pi task") {
		t.Fatal("all-agent filter did not restore both agents")
	}
}

func TestSessionsContinueActionLoadsChosenAgent(t *testing.T) {
	harness := newHarness(t, true)
	app := harness.a
	agentHome(t, harness.root)
	app.settings.AgentServers = map[string]settings.AgentServer{
		"target": roundtableTestServer(t, "Target Agent", "cli"),
	}
	app.showSessions()
	harness.settle()
	other := app.sessions.sessions[0]
	other.ID, other.Title = "another-source", "Another source conversation"
	app.sessions.sessions = append(app.sessions.sessions, other)
	app.sessions.version++
	harness.settle()
	row, found := harness.tt.Find(other.Title)
	if !found {
		t.Fatal("source conversation is missing")
	}
	harness.tt.Move(row.X+row.W/2, row.Y+row.H/2)
	harness.settle()
	if err := harness.tt.Click(locales.SessionsContinueWith()); err != nil {
		t.Fatal(err)
	}
	harness.tt.Move(700, 550)
	harness.tt.Advance(2 * motionHover)
	harness.tt.Frame()
	if err := harness.tt.ChooseMenuItem("Target Agent"); err != nil {
		t.Fatal(err)
	}
	harness.until("conversation loaded", func() bool { return app.agent.continuation != nil && app.agent.continuation.loaded })
	if app.agent.provider != "target" || app.agent.busy || app.agent.client != nil || app.agent.draft != locales.SessionsContinueDraft() {
		t.Fatalf("continuation did not open an unsent draft: %+v", app.agent)
	}
	if !app.isAgentPageVisible() || app.window.SidebarView != "sessions" || !harness.tt.HasText("fix here") || !harness.tt.HasText(locales.SessionsFind()) {
		t.Fatal("continuation history is not visible in the chat")
	}
}

func TestSessionsContinueInSourceProject(t *testing.T) {
	first, second := newHarness(t, true), newHarness(t, true)
	first.a.host.(*fakeHost).projects = []*app{first.a, second.a}
	second.a.settings.AgentServers = map[string]settings.AgentServer{
		"target": roundtableTestServer(t, "Target", "acp"),
	}
	source := continuationSource(t, second.root)
	previous := first.a.agent
	first.a.continueSessionWith(source, "target")
	second.until("source project continuation", func() bool {
		return second.a.agent.continuation != nil && second.a.agent.continuation.loaded && !second.a.agent.connecting
	})
	if first.a.host.(*fakeHost).showed != second.a || first.a.agent != previous || second.a.agent.provider != "target" {
		t.Fatal("continuation did not preserve the original project and activate the source project")
	}
	second.a.agent.draft = "continue in the source project"
	second.a.sendAgent()
	second.until("source project reply", func() bool { return !second.a.agent.busy && !second.a.agent.connecting })
	chat := second.a.agent
	actualRoot, err := filepath.EvalSymlinks(second.root)
	if err != nil {
		t.Fatal(err)
	}
	reply := chat.messages[len(chat.messages)-1].text
	if chat.err != "" || !strings.Contains(reply, "workspace="+actualRoot) {
		t.Fatalf("target agent did not receive the source workspace: %s\n%s", chat.err, reply)
	}
	second.a.closeAllAgents()
}

func TestSessionsResumeButtonOpensHistoryAndKeepsProviderChoiceExplicit(t *testing.T) {
	hs := newHarness(t, false)
	agentHome(t, hs.root)
	hs.a.settings.AgentServers = map[string]settings.AgentServer{"other": roundtableTestServer(t, "Other Agent", "cli")}
	hs.a.showSessions()
	hs.settle()
	row, ok := hs.tt.Find("fix here")
	if !ok {
		t.Fatal("missing source session")
	}
	hs.tt.Move(row.X+row.W/2, row.Y+row.H/2)
	hs.settle()
	if err := hs.tt.Click(locales.SessionsResume()); err != nil {
		t.Fatal(err)
	}
	hs.until("history shown", func() bool { return hs.a.agent.continuation != nil && hs.a.agent.continuation.loaded })
	chat := hs.a.agent
	if !hs.a.isAgentPageVisible() || len(hs.a.terms) != 0 || chat.provider == "other" || chat.connectionAttempted || !hs.tt.HasText(locales.SessionsResumeUnavailable("Claude Code")) {
		t.Fatal("Resume must open the chat and ask before using a different agent")
	}
	hs.tt.Move(700, 400)
	hs.settle()
	if err := hs.tt.Click(locales.SessionsContinueWith()); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.ChooseMenuItem("Other Agent"); err != nil {
		t.Fatal(err)
	}
	hs.until("chosen agent ready", func() bool { return hs.a.agent != chat && hs.a.agent.continuationReady() })
	if hs.a.agent.provider != "other" || len(hs.a.agent.messages) != 1 || len(hs.a.terms) != 0 {
		t.Fatal("the chosen agent did not receive the source history in a chat")
	}
}

func TestSessionsFiltersFitLanguages(t *testing.T) {
	for _, language := range []string{"en", "ja", "zh-Hans"} {
		t.Run(language, func(t *testing.T) {
			previous := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return language })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
			harness := newHarness(t, true)
			app := harness.a
			agentHome(t, harness.root)
			harness.tt.SetLocale(language)
			app.showSessions()
			for _, width := range []float32{180, 280, 420} {
				app.window.SidebarWidth = width
				harness.settle()
				query, foundQuery := harness.tt.Find(locales.SessionsFind())
				current, foundCurrent := harness.tt.Find(locales.SessionsThisProject())
				all, foundAll := harness.tt.Find(locales.SessionsAllProjects())
				if !foundQuery || !foundCurrent || !foundAll || current.W <= 0 || math.Abs(float64(current.W-all.W)) > 1 ||
					current.X+current.W > all.X || math.Abs(float64(current.X-query.X)) > 10 ||
					math.Abs(float64(all.X+all.W-query.X-query.W)) > 10 {
					t.Fatalf("scope buttons must share the search row width equally at %v: query=%+v current=%+v all=%+v", width, query, current, all)
				}
				var controls []ui.Rect
				for _, label := range []string{locales.SessionsFilterAgent(), locales.SessionsCount(1), locales.Refresh()} {
					bounds, found := harness.tt.Find(label)
					if !found || bounds.W < 8 || bounds.X < 0 || bounds.X+bounds.W > width+1 {
						t.Fatalf("%s does not fit width %v: %+v", label, width, bounds)
					}
					for _, previous := range controls {
						if min(bounds.X+bounds.W, previous.X+previous.W)-max(bounds.X, previous.X) > 1 {
							t.Fatalf("filter controls overlap at width %v: %+v / %+v", width, previous, bounds)
						}
					}
					controls = append(controls, bounds)
				}
			}
			now := time.Now()
			app.sessions.sessions = []agents.Session{
				{Harness: agents.Codex, ID: "thread-9ad62f01", Title: "Fix login redirect", Dir: app.root, Updated: now},
				{Harness: agents.Claude, ID: "thread-4bc62837", Title: "Fix login redirect", Dir: app.root, Updated: now.Add(-time.Hour)},
				{Harness: agents.Pi, ID: "thread-7e085cf2", Title: "Review API changes", Dir: app.root, Updated: now.Add(-2 * time.Hour)},
			}
			app.sessions.version++
			harness.settle()
			harness.shot(fmt.Sprintf("sessions-sidebar-%s", language))
			app.openAgentPage()
			app.showSessions()
			harness.settle()
			harness.shot(fmt.Sprintf("sessions-page-%s", language))
		})
	}
}

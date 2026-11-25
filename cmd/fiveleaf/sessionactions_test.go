package main

import (
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestSessionsStartChatFromEmptyAndFilteredHistory(t *testing.T) {
	hs := newHarness(t, true)
	app := hs.a
	agentHome(t, t.TempDir())
	app.settings.AgentServers = map[string]settings.AgentServer{"demo": {Protocol: "cli", Command: "demo-agent"}}
	app.showSessions()
	hs.settle()
	if len(app.sessionRows()) != 0 || agentPageCount(app) != 0 {
		t.Fatal("fixture should start with empty project history and no chat tabs")
	}
	start := func() {
		t.Helper()
		if err := hs.tt.Click(locales.SessionsStartChat()); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	start()
	first := app.agent
	if agentPageCount(app) != 1 || !hs.tt.Focused(locales.AgentDraft()) || !app.sessionsVisible() {
		t.Fatal("starting the first chat should focus its draft and keep Sessions open")
	}
	hs.tt.Type("Keep this draft")
	app.sessions.all, app.sessions.query = 1, "no such conversation"
	hs.settle()
	start()
	if agentPageCount(app) != 2 || app.agent == first || app.agent.draft != "" || first.draft != "Keep this draft" || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("starting another chat should focus an independent, empty draft")
	}
	if app.sessions.all != 1 || app.sessions.query != "no such conversation" || len(app.sessionRows()) != 0 {
		t.Fatal("starting a chat changed the history filters")
	}
}

func TestSessionsActionsFitSidebar(t *testing.T) {
	for _, language := range galleryLocales {
		t.Run(language, func(t *testing.T) {
			previous := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return language })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
			hs := newHarness(t, true)
			agentHome(t, hs.root)
			hs.tt.SetLocale(language)
			hs.a.showSessions()
			for _, right := range []bool{false, true} {
				hs.a.settings.SidebarRight = right
				for _, width := range []float32{160, 248, 420} {
					hs.a.window.SidebarWidth = width
					hs.settle()
					query, _ := hs.tt.Find(locales.SessionsFind())
					start, _ := hs.tt.Find(locales.SessionsStartChat())
					more, _ := hs.tt.Find(locales.AgentRoster())
					meeting, _ := hs.tt.Find(locales.RoundtableOpen())
					for _, control := range []ui.Rect{start, more, meeting} {
						if control.W < 20 || control.H < 20 || control.X < query.X-10 || control.X+control.W > query.X+query.W+10 || control.Y+control.H > query.Y {
							t.Fatalf("control outside header at width %g, right=%t: %+v, search %+v", width, right, control, query)
						}
					}
					wrapped := meeting.Y >= start.Y+start.H
					if start.Y != more.Y || start.X+start.W > more.X+1 || (!wrapped && meeting.X < more.X+more.W-1) || (width == 160 && !wrapped) || (width == 420 && wrapped) {
						t.Fatalf("actions did not share or wrap their row at width %g: %+v %+v %+v", width, start, more, meeting)
					}
					if right && language == "zh-Hans" && (width == 160 || width == 248) {
						name := "sessions-actions-right"
						if width == 160 {
							name += "-compact"
						}
						hs.shot(name)
					}
				}
			}
		})
	}
}

func TestSessionsMenuClosesWhenSidebarChanges(t *testing.T) {
	hs := newHarness(t, true)
	agentHome(t, hs.root)
	app := hs.a
	app.showSessions()
	hs.settle()
	for _, leave := range []func(){app.showExplorer, app.toggleSidebar} {
		hs.openAgentMenu(locales.AgentRoster())
		leave()
		hs.settle()
		if app.agentMenu.open {
			t.Fatal("the Agent menu outlived its sidebar control")
		}
		app.showSessions()
		hs.settle()
		if app.agentMenu.open {
			t.Fatal("returning to Sessions reopened a dismissed menu")
		}
	}
}

package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func agentNavigationHarness(t *testing.T, root, savedView string) *harness {
	t.Helper()
	agentHome(t, root)
	s := settings.Default()
	s.MarkdownMode = ""
	w := s.NewWindow()
	w.Folder, w.SidebarView = root, savedView
	h := &fakeHost{folder: root}
	a := newApp(h, s, w)
	t.Cleanup(a.shutdown)
	hs := &harness{t: t, a: a, h: h, root: root, tt: ui.NewTester(a.view, 1200, 760)}
	hs.tt.SetDark(true)
	hs.settle()
	return hs
}

func requireAgentNavigation(t *testing.T, hs *harness, page string) {
	t.Helper()
	a, tt := hs.a, hs.tt
	if a.window.SidebarView != page || a.window.SidebarClosed {
		t.Fatalf("Agent shows %q, closed=%v, want %q open", a.window.SidebarView, a.window.SidebarClosed, page)
	}
	chrome := hs.layoutOf(locales.CmdViewSessions(), locales.PrefsTitle())
	activity, gear := chrome[locales.CmdViewSessions()], chrome[locales.PrefsTitle()]
	if gear.Y < 0 || gear.Y+gear.H > titleBarH || activity.Y < titleBarH {
		t.Fatalf("expected Settings in the title bar and Sessions in the sidebar: settings=%v sessions=%v", gear, activity)
	}
	labels := []string{locales.SessionsStartChat(), locales.AgentRoster(), locales.RoundtableOpen(), locales.SessionsFind()}
	if page != "sessions" {
		for _, label := range labels {
			if bounds, found := tt.Find(label); found {
				t.Errorf("Sessions control %q is visible on page %q: %v", label, page, bounds)
			}
		}
		return
	}
	controls := hs.layoutOf(labels...)
	controls[locales.CmdViewSessions()] = activity
	left := float32(0)
	if a.settings.SidebarRight {
		width, _ := a.review.ctx.Size()
		left = width - a.window.SidebarWidth
	}
	for label, bounds := range controls {
		if bounds.W <= 0 || bounds.H <= 0 || bounds.Y < titleBarH || bounds.X < left || bounds.X+bounds.W > left+a.window.SidebarWidth+1 {
			t.Fatalf("%q falls outside the Sessions sidebar at x=%v, width=%v: %v", label, left, a.window.SidebarWidth, bounds)
		}
	}
	start, menu := controls[locales.SessionsStartChat()], controls[locales.AgentRoster()]
	gap := menu.X - (start.X + start.W)
	if menu.Y < activity.Y+activity.H || menu.Y != start.Y || gap < 0 || gap > 12 {
		t.Fatalf("the Sessions launcher must be beside New Chat: new=%v launcher=%v", start, menu)
	}
	img := tt.Image()
	visible := 0
	for y := int(math.Ceil(float64(activity.Y + activity.H*0.8))); y < int(activity.Y+activity.H-1); y++ {
		for x := int(activity.X + 1); x < int(activity.X+activity.W-1); x++ {
			pixel := img.RGBAAt(x, y)
			if contrast(ui.RGBA(pixel.R, pixel.G, pixel.B, 1), a.pal.chrome) >= 3 {
				visible++
			}
		}
	}
	if visible < 8 {
		t.Errorf("the Sessions activity has only %d selection-marker pixels on page %q", visible, page)
	}
}

func TestAgentNavigationRestoresSessions(t *testing.T) {
	for _, hasFolder := range []bool{true, false} {
		name, root := "without-folder", ""
		if hasFolder {
			name, root = "with-folder", workspaceFixture(t)
		}
		t.Run(name, func(t *testing.T) {
			for _, saved := range []string{"sessions", "agent"} {
				hs := agentNavigationHarness(t, root, saved)
				requireAgentNavigation(t, hs, "sessions")
				if _, shown := hs.tt.Find(locales.NoFolderOpen()); shown {
					t.Error("the restored sessions page shows the no-folder placeholder")
				}
			}
		})
	}
}

func TestAgentNavigationActivityKeepsCurrentPage(t *testing.T) {
	hs := newHarness(t, true)
	agentHome(t, hs.root)
	hs.a.run("view.sessions")
	for range 2 {
		hs.a.run("view.agent")
		hs.settle()
		hs.a.run("view.agentPage")
		hs.settle()
		requireAgentNavigation(t, hs, "sessions")
		if agentPageCount(hs.a) != 1 || !hs.tt.Focused(locales.AgentDraft()) {
			t.Fatal("launching again did not reuse and focus the open workspace chat")
		}
	}
}

func TestAgentNavigationWithoutFolder(t *testing.T) {
	hs := agentNavigationHarness(t, "", "")
	hs.a.run("view.agent")
	hs.settle()
	requireAgentNavigation(t, hs, "")
	hs.a.run("view.sessions")
	hs.settle()
	requireAgentNavigation(t, hs, "sessions")
	if !hs.a.isAgentPageVisible() || !hs.tt.HasText(locales.NoFolderOpen()) {
		t.Fatal("a chat without a workspace did not show its empty state beside history")
	}
}

func TestAgentNavigationCommandsAndMenu(t *testing.T) {
	hs := newHarness(t, true)
	agentHome(t, hs.root)
	a := hs.a
	registered := map[string]int{}
	for _, cmd := range a.commands() {
		registered[cmd.id]++
	}
	for _, id := range []string{"view.agent", "view.sessions"} {
		if registered[id] != 1 {
			t.Errorf("command %q is registered %d times", id, registered[id])
		}
	}
	for _, page := range []string{"", "search", "git", "agent", "sessions"} {
		a.window.SidebarView = page
		if a.commandOn("view.agent") {
			t.Errorf("sidebar %q alone marked a workspace chat as open", page)
		}
		if got, want := a.commandOn("view.sessions"), page == "sessions" || page == "agent"; got != want {
			t.Errorf("Sessions command selected=%v for page %q, want %v", got, page, want)
		}
	}
	var viewMenu *mygo.MenuItem
	for _, item := range a.menu(false, func(id string, _ *mygo.Window) { a.run(id) }).Items() {
		if item.Label == locales.MenuView() {
			viewMenu = item
			break
		}
	}
	if viewMenu == nil {
		t.Fatal("the View menu is missing")
	}
	shown := map[string]int{}
	for _, item := range viewMenu.Submenu {
		shown[item.ID]++
	}
	if shown["view.agent"] != 0 || shown["view.sessions"] != 1 {
		t.Fatalf("View menu has %d Agent and %d Sessions entries", shown["view.agent"], shown["view.sessions"])
	}
	a.window.SidebarClosed = true
	a.run("view.agent")
	hs.settle()
	if !a.commandOn("view.agent") || !a.window.SidebarClosed || a.commandOn("view.sessions") {
		t.Fatal("launching a workspace chat changed the closed sidebar")
	}
	a.run("view.sessions")
	hs.settle()
	requireAgentNavigation(t, hs, "sessions")
}

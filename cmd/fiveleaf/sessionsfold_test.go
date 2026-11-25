package main

import (
	"testing"
	"time"

	"fiveleaf/internal/agents"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestSessionsDateGroupsFoldIndependently(t *testing.T) {
	hs := newHarness(t, true)
	state := &hs.a.sessions
	if !state.read.Start(hs.root, 0) {
		t.Fatal("cannot hold the scanner while testing fixed date groups")
	}
	t.Cleanup(func() { state.read.Done(hs.root) })
	now := hs.tt.Now().Local()
	year, month, day := now.Date()
	today := time.Date(year, month, day, 12, 0, 0, 0, now.Location())
	state.loaded, state.all = true, 1
	state.sessions = []agents.Session{
		{Harness: agents.Codex, ID: "build", Title: "Build the patch", Dir: hs.root, Updated: today},
		{Harness: meetingHistorySource, ID: "review", Title: "Review the design", Dir: hs.root, Updated: today.Add(-time.Hour)},
		{Harness: agents.Codex, ID: "plan", Title: "Plan the release", Dir: hs.root, Updated: today.AddDate(0, 0, -1)},
	}
	state.version++
	hs.a.showSessions()
	hs.settle()

	checkGroup := func(day sessionDay, count int) {
		t.Helper()
		if _, found := hs.tt.Find(day.name()); !found {
			t.Fatalf("date group %q disappeared", day.name())
		}
		for _, line := range state.shown {
			if line.head && line.day == day && line.gone.IsZero() && line.count == count {
				return
			}
		}
		t.Fatalf("date group %q should count %d sessions", day.name(), count)
	}
	checkRows := func(today, yesterday bool) {
		t.Helper()
		for _, title := range []string{"Build the patch", "Review the design"} {
			if hs.tt.HasText(title) != today {
				t.Fatalf("today's record %q visibility should be %v", title, today)
			}
		}
		if hs.tt.HasText("Plan the release") != yesterday {
			t.Fatalf("yesterday's record visibility should be %v", yesterday)
		}
	}

	checkRows(true, true)
	clickSessionHistoryButton(t, hs, locales.SessionsToday())
	checkRows(false, true)
	checkGroup(sessionsToday, 2)
	if !hs.tt.Focused(locales.SessionsToday()) {
		t.Fatal("the clicked date group did not receive focus")
	}
	hs.key(0, ui.KeyEnter)
	checkRows(true, true)
	if hs.a.isAgentPageVisible() {
		t.Fatal("Enter on the date group also opened a session")
	}
	clickSessionHistoryButton(t, hs, locales.SessionsToday())
	clickSessionHistoryButton(t, hs, locales.SessionsFind())
	hs.key(0, ui.KeyUp)
	if chosen := chosenID(state); chosen != "plan" {
		t.Fatalf("keyboard chose %q with Today folded, want plan", chosen)
	}

	clickSessionHistoryButton(t, hs, locales.SessionsYesterday())
	checkRows(false, false)
	checkGroup(sessionsToday, 2)
	checkGroup(sessionsYesterday, 1)
	clickSessionHistoryButton(t, hs, locales.SessionsFind())
	hs.key(0, ui.KeyEnter)
	if chosenID(state) != "" || hs.a.isAgentPageVisible() || hs.a.activeRoundtableTab() != nil {
		t.Fatal("Enter selected or opened a record while all date groups were folded")
	}

	clickSessionHistoryButton(t, hs, locales.SessionsToday())
	checkRows(true, false)
	clickSessionHistoryButton(t, hs, locales.SessionsYesterday())
	checkRows(true, true)
}

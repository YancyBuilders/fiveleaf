package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func agentUsageUpdate(session string, used, size int) acp.Update {
	var update acp.Update
	copyAgentJSON(map[string]any{
		"sessionId": session,
		"update": map[string]any{
			"sessionUpdate": "usage_update", "used": used, "size": size,
		},
	}, &update)
	return update
}

func assertAgentContextDetails(hs *harness, used, size int) {
	hs.t.Helper()
	if !hs.tt.HasText(locales.AgentContextCapacity(agentTokenCount(size))) {
		hs.t.Fatal("the context bar lost the reported capacity")
	}
	if err := hs.tt.Click(locales.AgentContext()); err != nil {
		hs.t.Fatal(err)
	}
	hs.settle()
	percent := agentContextPercentage(acp.UsageState{Used: used, Size: size})
	if !hs.tt.HasText(agentContextTokenCount(used)) || !hs.tt.HasText(locales.AgentContextUsageSummary(agentContextTokenCount(size), percent)) {
		hs.t.Fatal("context details lost the reported usage or percentage")
	}
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
}

func TestAgentContextUsageFollowsItsConversation(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	a, chat := hs.a, hs.a.agent
	chat.sessionID = "first"
	h := agentHandler{app: a, chat: chat, root: a.root, generation: chat.generation}
	hs.settle()
	if !hs.tt.HasText(locales.AgentContextCapacity("—")) {
		t.Fatal("unreported context is not shown as unknown")
	}
	update := func(session string, used, size int) {
		t.Helper()
		h.Update(agentUsageUpdate(session, used, size))
		hs.settle()
	}
	update("first", 0, 1000)
	if chat.contextUsage == nil {
		t.Fatal("a reported zero was confused with unknown context")
	}
	assertAgentContextDetails(hs, 0, 1000)
	update("first", 500, 1000)
	assertAgentContextDetails(hs, 500, 1000)
	for _, invalid := range [][2]int{{-1, 1000}, {1, 0}, {1, -1}} {
		update("first", invalid[0], invalid[1])
	}
	update("other-session", 900, 1000)
	if got := chat.contextUsage; got == nil || *got != (acp.UsageState{Used: 500, Size: 1000}) {
		t.Fatalf("invalid or unrelated usage replaced valid context: %+v", got)
	}
	update("first", 125, 1000)
	if chat.contextUsage.Used != 125 {
		t.Fatal("context usage accumulated instead of replacing the previous occupancy")
	}
	update("first", 1539, 1048576)
	assertAgentContextDetails(hs, 1539, 1048576)
	update("first", 1, 1048576)
	assertAgentContextDetails(hs, 1, 1048576)
	a.settings.AgentServers["other"] = settings.AgentServer{Name: "Other", Command: "other-agent"}
	a.selectAgent("other")
	other := a.agent
	update("first", 250, 1000)
	if chat.contextUsage.Used != 250 || other.contextUsage != nil || !hs.tt.HasText(locales.AgentContextCapacity("—")) {
		t.Fatal("a hidden conversation's usage leaked into the selected chat")
	}
	a.selectAgent("demo")
	a.openAgentPage()
	hs.settle()
	if a.agent != chat {
		t.Fatal("opening the workspace page lost reported context")
	}
	assertAgentContextDetails(hs, 250, 1000)
	a.newFile()
	a.openChatPage(chat)
	hs.settle()
	assertAgentContextDetails(hs, 250, 1000)
	// A queued event from a disconnected process must not restore its usage.
	h.Update(agentUsageUpdate("first", 750, 1000))
	a.disconnectChat(chat)
	hs.settle()
	if chat.contextUsage != nil || !hs.tt.HasText(locales.AgentContextCapacity("—")) {
		t.Fatal("disconnect retained context or accepted a stale process update")
	}
}

func TestAgentContextUsageRejectsStaleSessionsAndProjects(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	a, chat := hs.a, hs.a.agent
	chat.sessionID = "old"
	h := agentHandler{app: a, chat: chat, root: a.root, generation: chat.generation}
	h.Update(agentUsageUpdate("old", 500, 1000))
	chat.sessionID = "new"
	hs.settle()
	if chat.contextUsage != nil {
		t.Fatal("a queued usage update from the previous turn reached its new session")
	}
	oldProject := h
	oldProject.root = a.root + "-closed"
	oldProject.Update(agentUsageUpdate("new", 500, 1000))
	hs.settle()
	if chat.contextUsage != nil {
		t.Fatal("a previous project's usage reached the current project")
	}
	h.Update(agentUsageUpdate("new", 250, 1000))
	hs.settle()
	if chat.contextUsage == nil {
		t.Fatal("the active session's usage was ignored")
	}
	a.newAgentSession()
	hs.settle()
	if chat.contextUsage != nil {
		t.Fatal("New Chat retained the previous context occupancy")
	}
}

func TestAgentContextUpdatesKeepDraftSelection(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	hs.settle()
	hs.tt.Type("hello world")
	hs.settle()
	for range 5 {
		hs.key(ui.Shift, ui.KeyLeft)
	}
	s := hs.a.agent
	before := s.draftState
	for i, width := range []float32{160, 340, 180, 340} {
		sizeChatWorkspace(hs, width)
		s.acceptContextUsage(acp.UsageState{Used: i * 10000, Size: 1000000})
		hs.settle()
		if s.draftState.id != before.id || s.draftState.selection != before.selection || s.draft != "hello world" || !hs.tt.Focused(locales.AgentDraft()) {
			t.Fatalf("context update changed the draft identity or selection (width=%v): before=%+v after=%+v focused=%v", width, before, s.draftState, hs.tt.Focused(locales.AgentDraft()))
		}
	}
	hs.tt.Type("there")
	hs.settle()
	if s.draft != "hello there" {
		t.Fatalf("typing did not replace the preserved selection: %q", s.draft)
	}
}

func TestAgentContextUsageFitsNarrowViews(t *testing.T) {
	for _, language := range []string{"en", "ja", "zh-Hans"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%v", language, dark), func(t *testing.T) {
				before := i18n.Locale()
				i18n.SetLocaleFunc(func() string { return language })
				t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
				hs := newHarness(t, dark)
				agentViewFixture(hs)
				hs.a.agent.acceptContextUsage(acp.UsageState{Used: 90000, Size: 100000})
				for _, width := range []float32{340, 180, 160} {
					sizeChatWorkspace(hs, width)
					hs.settle()
					for _, label := range []string{locales.AgentContext(), locales.AgentDraft(), locales.AgentSend()} {
						bounds, found := hs.tt.Find(label)
						if !found || bounds.X < 0 || bounds.X+bounds.W > width+1 || bounds.W <= 0 || bounds.H <= 0 {
							t.Errorf("%q does not fit width %v: %+v (found=%v)", label, width, bounds, found)
						}
					}
					if !hs.tt.HasText(locales.AgentContextCapacity("100k")) || !hs.tt.HasText("90%") {
						t.Fatal("the context bar must show its percentage and capacity")
					}
				}
				sizeChatWorkspace(hs, 340)
				hs.settle()
				hs.shot(fmt.Sprintf("agent-context-%s-dark-%v", language, dark))
				hs.a.agent.acceptContextUsage(acp.UsageState{Used: 125000, Size: 100000})
				hs.settle()
				assertAgentContextDetails(hs, 125000, 100000)
			})
		}
	}
}

func TestAgentContextUsageBar(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := lightPalette().derive()
		if dark {
			p = darkPalette().derive()
		}
		for _, width := range []int{160, 180, 340} {
			for _, tc := range []struct {
				name    string
				usage   *acp.UsageState
				percent string
				fill    float64
				color   ui.Color
			}{
				{name: "unknown"},
				{name: "zero", usage: &acp.UsageState{Used: 0, Size: 100000}, percent: "0", color: p.accent},
				{name: "quarter", usage: &acp.UsageState{Used: 25000, Size: 100000}, percent: "25", fill: 0.25, color: p.accent},
				{name: "below-warning", usage: &acp.UsageState{Used: 84000, Size: 100000}, percent: "84", fill: 0.84, color: p.accent},
				{name: "warning", usage: &acp.UsageState{Used: 85000, Size: 100000}, percent: "85", fill: 0.85, color: p.modifiedFg},
				{name: "below-danger", usage: &acp.UsageState{Used: 94000, Size: 100000}, percent: "94", fill: 0.94, color: p.modifiedFg},
				{name: "danger", usage: &acp.UsageState{Used: 95000, Size: 100000}, percent: "95", fill: 0.95, color: p.dangerFg},
				{name: "over-capacity", usage: &acp.UsageState{Used: 125000, Size: 100000}, percent: "125", fill: 1, color: p.dangerFg},
			} {
				t.Run(fmt.Sprintf("dark=%v/width=%d/%s", dark, width, tc.name), func(t *testing.T) {
					a := &app{pal: p, settings: settings.Default()}
					chat := &agentChat{contextUsage: tc.usage}
					capacity, tooltip := "—", locales.AgentContextUnavailable()
					if tc.usage != nil {
						capacity = agentTokenCount(tc.usage.Size)
						tooltip = locales.AgentContextUsage(fmt.Sprint(tc.usage.Used), fmt.Sprint(tc.usage.Size)) + "\n" + locales.AgentContextUsageHint()
					}
					var capacityWidth float32
					tt := ui.NewTester(func(c *ui.Context) {
						capacityWidth, _ = c.MeasureText(0, ui.Span{Text: capacity, Size: p.metricsOr().textSmall, Features: "tnum"})
						ui.Column(c).Fill().Background(p.chrome).Padding(p.metricsOr().inset).Children(func() {
							a.agentContextUsage(c, chat)
						})
					}, width, 100)
					tt.SetDark(dark)
					step(tt)
					bounds, found := tt.Find(locales.AgentContext())
					if !found || bounds.W <= 0 || bounds.H <= 2 || bounds.H > p.metricsOr().row+p.metricsOr().gap {
						t.Fatalf("context bar is missing or not compact: %+v (found=%v)", bounds, found)
					}
					label, found := tt.Find(locales.AgentContextCapacity(capacity))
					if !found || label.W+0.1 < capacityWidth || label.X < bounds.X || label.X+label.W > bounds.X+bounds.W {
						t.Fatalf("context capacity is truncated or escapes its bar: %+v in %+v (text width=%v, found=%v)", label, bounds, capacityWidth, found)
					}
					if tt.HasText(locales.AgentContextPercent(tc.percent)) || tt.HasText(locales.AgentContextUnknown()) {
						t.Fatal("the compact context bar still shows a usage label")
					}
					if tc.usage != nil {
						percentage, found := tt.Find(tc.percent + "%")
						if !found || percentage.X < bounds.X || percentage.X+percentage.W >= label.X {
							t.Fatalf("context percentage escapes its bar or overlaps capacity: percentage=%+v capacity=%+v bounds=%+v", percentage, label, bounds)
						}
					} else if tt.HasText("0%") {
						t.Fatal("unknown context usage must not display zero percent")
					}
					img := tt.Image()
					y := int(math.Ceil(float64(bounds.Y + bounds.H - 2)))
					m := p.metricsOr()
					inset := min(m.radiusCard+m.pad+m.gap/2, bounds.W/2)
					trackStart, trackEnd := float64(bounds.X+inset), float64(bounds.X+bounds.W-inset)
					for x := int(math.Ceil(float64(bounds.X))); x < int(bounds.X+bounds.W); x++ {
						fillEnd := trackStart + (trackEnd-trackStart)*tc.fill
						if tc.fill > 0 && tc.fill < 1 && math.Abs(float64(x)-fillEnd) < 1 {
							continue
						}
						want := p.border
						if float64(x)+0.5 < trackStart || float64(x)+0.5 >= trackEnd {
							want = p.chrome
						} else if float64(x)+0.5 < fillEnd {
							want = tc.color
						}
						pixel := img.RGBAAt(x, y)
						if abs(int(pixel.R)-int(want.R)) > 2 || abs(int(pixel.G)-int(want.G)) > 2 || abs(int(pixel.B)-int(want.B)) > 2 {
							t.Fatalf("context track at (%d,%d) is %v, want %v for %.0f%% occupancy", x, y, pixel, want, tc.fill*100)
						}
					}
					x := int(bounds.X + bounds.W/2)
					for _, y := range []int{int(bounds.Y + bounds.H - 4), int(math.Ceil(float64(bounds.Y + bounds.H)))} {
						pixel := img.RGBAAt(x, y)
						if pixel.R != p.chrome.R || pixel.G != p.chrome.G || pixel.B != p.chrome.B {
							t.Fatalf("context track paints outside its two-pixel height at (%d,%d): %v", x, y, pixel)
						}
					}
					(&harness{t: t, tt: tt}).shot(fmt.Sprintf("agent-context-bar-dark-%v-width-%d-%s", dark, width, tc.name))
					tt.Move(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
					tt.Advance(time.Second)
					tt.Frame()
					if !tt.HasText(tooltip) {
						t.Fatalf("context tooltip lost the exact occupancy or availability hint: %q", tooltip)
					}
				})
			}
		}
	}
}

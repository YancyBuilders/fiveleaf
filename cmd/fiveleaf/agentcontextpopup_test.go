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

func agentContextPopupFixture() agentContextSnapshot {
	return agentContextSnapshot{
		known: true, used: 38000, size: 1000000, model: "DeepSeek-V4.1-Flash",
		compactionKnown: true, compactionEnabled: true, compactAt: 983616, estimated: true,
		categories: []agentContextCategory{
			{kind: "system", known: true, tokens: 25000},
			{kind: "instructions", known: true, tokens: 383},
			{kind: "skills", known: true, tokens: 1300},
			{kind: "builtin_tools", known: true, tokens: 11000},
			{kind: "custom_tools", known: true},
			{kind: "user", known: true},
			{kind: "assistant", known: true, tokens: 317},
			{kind: "thinking", known: true},
			{kind: "tool_calls", known: true},
			{kind: "tool_output", known: true},
		},
	}
}

func agentContextPopupTester(dark bool, width, height int, snapshot *agentContextSnapshot) (*ui.Tester, *agentChat) {
	a := &app{settings: settings.Default()}
	chat := &agentChat{contextDetailsOpen: true}
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = a.applyTheme(c)
		ui.Box(c).Fill().Background(a.pal.chrome)
		a.agentContextPopup(c, chat, *snapshot)
	}, width, height)
	tt.SetDark(dark)
	tt.Advance(time.Second)
	tt.Frame()
	return tt, chat
}

func TestAgentContextPopupLayout(t *testing.T) {
	for _, language := range []string{"en", "ja", "zh-Hans"} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{360, 1200} {
				t.Run(fmt.Sprintf("%s/dark=%v/width=%d", language, dark, width), func(t *testing.T) {
					before := i18n.Locale()
					i18n.SetLocaleFunc(func() string { return language })
					t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
					snapshot := agentContextPopupFixture()
					tt, _ := agentContextPopupTester(dark, width, 680, &snapshot)
					bounds, found := tt.Find(locales.AgentContextDetails())
					if !found || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > float32(width) || bounds.Y+bounds.H > 680 || bounds.W <= 0 || bounds.H <= 0 {
						t.Fatalf("context details escape the window: %+v (found=%v)", bounds, found)
					}
					for _, label := range []string{locales.ButtonClose(), "38,000", locales.AgentContextUsageSummary("1,000,000", "3.8")} {
						r, found := tt.Find(label)
						if !found || r.X < bounds.X || r.Y < bounds.Y || r.X+r.W > bounds.X+bounds.W+0.1 || r.Y+r.H > bounds.Y+bounds.H+0.1 || r.W <= 0 || r.H <= 0 {
							t.Errorf("%q escapes the context dialog: %+v in %+v (found=%v)", label, r, bounds, found)
						}
					}
					for _, category := range agentContextCategoryRows(snapshot) {
						label := agentContextCategoryLabel(category.kind)
						r, found := tt.Find(label)
						if !found || r.W <= 0 || r.X < bounds.X || r.X+r.W > bounds.X+bounds.W+0.1 {
							t.Errorf("context category %q is missing or escapes horizontally: %+v (found=%v)", label, r, found)
						}
					}
					remaining := locales.AgentContextBeforeCompaction("945,616", "98.4")
					for _, label := range []string{remaining, locales.AgentContextCompactBuffer(), locales.AgentContextFreeSpace(), locales.AgentContextEstimated(), locales.AgentContextCompactionHint(), locales.AgentContextReserveHint()} {
						if !tt.HasText(label) {
							t.Errorf("context details lost %q", label)
						}
					}
					if tt.HasText(locales.AgentContextUnattributed()) {
						t.Fatal("a complete breakdown added an unreported remainder")
					}
					(&harness{t: t, tt: tt}).shot(fmt.Sprintf("agent-context-popup-%s-dark-%v-width-%d", language, dark, width))
				})
			}
		}
	}
}

func TestAgentContextPopupUnknownAndDisabled(t *testing.T) {
	snapshot := agentContextSnapshot{model: "Demo"}
	tt, _ := agentContextPopupTester(true, 360, 520, &snapshot)
	if !tt.HasText(locales.AgentContextCompactionUnavailable()) || tt.HasText("0%") || tt.HasText("0 ·") || tt.HasText(locales.AgentContextEstimated()) {
		t.Fatal("unknown context invents usage, a compaction threshold or an estimate")
	}
	unknown := 0
	for _, text := range tt.Texts() {
		if text == locales.AgentContextUnreported() {
			unknown++
		}
	}
	if unknown < 12 {
		t.Fatalf("unknown categories, buffer and free space are not explicit: %d unreported rows", unknown)
	}
	snapshot = agentContextPopupFixture()
	snapshot.compactionEnabled = false
	snapshot.categories = []agentContextCategory{{kind: "unattributed", known: true, tokens: snapshot.used}}
	snapshot.estimated = false
	step(tt)
	if !tt.HasText(locales.AgentContextCompactionDisabled()) || !tt.HasText(locales.AgentContextUnattributed()) || !tt.HasText(agentContextPart(962000, 1000000)) {
		t.Fatal("disabled compaction lost the real free capacity or unattributed usage")
	}
	if tt.HasText(locales.AgentContextEstimated()) {
		t.Fatal("reported unattributed occupancy is described as an estimate")
	}
	if !tt.HasText(locales.AgentContextBreakdownUnavailable()) {
		t.Fatal("missing category details have no explanation")
	}
}

func TestAgentContextPopupUsageAndReserveStayDistinct(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.contextPolicy = agentCompactionPolicy{known: true, enabled: true, reserveTokens: 16384}
	for _, tc := range []struct {
		name, percentage, free, remaining string
		used                              int
	}{
		{name: "empty", percentage: "0", free: "1,048,576 · 100%", remaining: "1,032,192"},
		{name: "reported", used: 32768, percentage: "3.1", free: "1,015,808 · 96.9%", remaining: "999,424"},
		{name: "past-threshold", used: 1040384, percentage: "99.2", free: "8,192 · 0.8%", remaining: "0"},
		{name: "over-capacity", used: 1310720, percentage: "125", free: "0 · 0%", remaining: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chat.contextDetailsOpen = false
			chat.acceptContextUsage(acp.UsageState{Used: tc.used, Size: 1048576})
			hs.settle()
			if !hs.tt.HasText(tc.percentage + "%") {
				t.Fatalf("context bar does not show the reported used percentage %s%%", tc.percentage)
			}
			if err := hs.tt.Click(locales.AgentContext()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			for _, text := range []string{
				locales.AgentContextUsageSummary("1,048,576", tc.percentage),
				tc.free, "16,384 · 1.6%",
				locales.AgentContextBeforeCompaction(tc.remaining, "98.4"),
				locales.AgentContextReserveHint(),
			} {
				if !hs.tt.HasText(text) {
					t.Errorf("context details do not distinguish usage from the compaction reserve: missing %q", text)
				}
			}
			free, hasFree := hs.tt.Find(locales.AgentContextFreeSpace())
			reserve, hasReserve := hs.tt.Find(locales.AgentContextCompactBuffer())
			if !hasFree || !hasReserve || reserve.Y <= free.Y+free.H+hs.a.pal.metricsOr().gap {
				t.Fatalf("compaction policy is not separated from the usage breakdown: free=%+v reserve=%+v", free, reserve)
			}
			hs.shot("agent-context-consistency-" + tc.name)
		})
	}
}

func TestAgentContextPopupScrollsWithinShortWindows(t *testing.T) {
	snapshot := agentContextPopupFixture()
	tt, _ := agentContextPopupTester(true, 360, 360, &snapshot)
	closeBefore, found := tt.Find(locales.ButtonClose())
	if !found || closeBefore.Y+closeBefore.H > 360 {
		t.Fatalf("the close action is unavailable in a short window: %+v", closeBefore)
	}
	tt.Scroll(180, 180, 0, 1000)
	step(tt)
	closeAfter, _ := tt.Find(locales.ButtonClose())
	if closeAfter != closeBefore {
		t.Fatalf("scrolling context details moved the close action: %+v -> %+v", closeBefore, closeAfter)
	}
	free, found := tt.Find(locales.AgentContextFreeSpace())
	if !found || free.Y < 0 || free.Y+free.H > closeAfter.Y {
		t.Fatalf("scrolling cannot reach the final context row: %+v (found=%v)", free, found)
	}
}

func TestAgentContextPopupOpensAndCloses(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.draft, chat.modelOverride = "keep the draft", "provider/model"
	chat.acceptContextUsage(acp.UsageState{Used: 38000, Size: 1000000})
	hs.settle()
	open := func() {
		t.Helper()
		bounds, found := hs.tt.Find(locales.AgentContext())
		if !found {
			t.Fatal("context bar is missing")
		}
		hs.tt.ClickAt(bounds.X+bounds.W/3, bounds.Y+bounds.H-1)
		hs.settle()
		if !chat.contextDetailsOpen || !hs.tt.HasText(locales.AgentContextFreeSpace()) {
			t.Fatal("clicking the context track did not open its details")
		}
	}
	assertClosed := func(method string) {
		t.Helper()
		hs.settle()
		if chat.contextDetailsOpen || hs.tt.HasText(locales.AgentContextFreeSpace()) {
			hs.shot("agent-context-close-" + method)
			t.Fatalf("context details did not close with %s (open=%v)", method, chat.contextDetailsOpen)
		}
		if chat.draft != "keep the draft" || chat.modelOverride != "provider/model" {
			t.Fatal("context details changed the draft or selected model")
		}
	}
	open()
	hs.tt.Key(0, ui.KeyEscape)
	assertClosed("escape")
	open()
	if err := hs.tt.Click(locales.ButtonClose()); err != nil {
		t.Fatal(err)
	}
	assertClosed("button")
	open()
	hs.tt.ClickAt(1190, 740)
	assertClosed("outside")
}

func TestAgentContextCompactionMarker(t *testing.T) {
	for _, reserve := range []int{0, 100, 1000, 2000} {
		t.Run(fmt.Sprint(reserve), func(t *testing.T) {
			p := darkPalette().derive()
			a := &app{pal: p, settings: settings.Default()}
			chat := &agentChat{contextUsage: &acp.UsageState{Used: 200, Size: 1000}, contextPolicy: agentCompactionPolicy{known: true, enabled: true, reserveTokens: reserve}}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Background(p.chrome).Padding(p.metricsOr().inset).Children(func() { a.agentContextUsage(c, chat) })
			}, 180, 100)
			tt.SetDark(true)
			step(tt)
			bounds, _ := tt.Find(locales.AgentContext())
			m := p.metricsOr()
			inset := m.radiusCard + m.pad + m.gap/2
			track := ui.Rect{X: bounds.X + inset, Y: bounds.Y + bounds.H - 2, W: bounds.W - 2*inset, H: 2}
			threshold := float32(max(0, 1000-reserve)) / 1000
			x := max(track.X, min(track.X+track.W-1, track.X+track.W*threshold-0.5))
			pixel := tt.Image().RGBAAt(int(math.Round(float64(x))), int(track.Y-1))
			if abs(int(pixel.R)-int(p.modifiedFg.R)) > 2 || abs(int(pixel.G)-int(p.modifiedFg.G)) > 2 || abs(int(pixel.B)-int(p.modifiedFg.B)) > 2 {
				t.Fatalf("real compaction marker is missing at %v: %v", x, pixel)
			}
			chat.contextPolicy.known = false
			step(tt)
			pixel = tt.Image().RGBAAt(int(math.Round(float64(x))), int(track.Y-1))
			if pixel.R != p.chrome.R || pixel.G != p.chrome.G || pixel.B != p.chrome.B {
				t.Fatalf("unknown compaction retains a threshold marker: %v", pixel)
			}
		})
	}
}

func TestAgentContextSegmentColors(t *testing.T) {
	p := darkPalette().derive()
	a := &app{pal: p}
	snapshot := agentContextSnapshot{known: true, used: 600, size: 1000, compactionKnown: true, compactionEnabled: true, compactAt: 800,
		categories: []agentContextCategory{{kind: "system", known: true, tokens: 100}, {kind: "skills", known: true, tokens: 200}, {kind: "builtin_tools", known: true, tokens: 300}}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Background(p.chrome).Draw(func(pt *ui.Painter, _ ui.Rect) {
			a.paintAgentContextSegments(pt, ui.Rect{X: 10, Y: 10, W: 300, H: 8}, snapshot)
		})
	}, 320, 30)
	for _, sample := range []struct {
		x     int
		color ui.Color
	}{{25, p.accent}, {70, p.addedFg}, {145, p.modifiedFg}, {220, p.border}} {
		pixel := tt.Image().RGBAAt(sample.x, 14)
		if abs(int(pixel.R)-int(sample.color.R)) > 2 || abs(int(pixel.G)-int(sample.color.G)) > 2 || abs(int(pixel.B)-int(sample.color.B)) > 2 {
			t.Fatalf("context category at %d has color %v, want %v", sample.x, pixel, sample.color)
		}
	}
	colors := make(map[[3]uint8]bool)
	for x := 260; x < 300; x++ {
		pixel := tt.Image().RGBAAt(x, 14)
		colors[[3]uint8{pixel.R, pixel.G, pixel.B}] = true
	}
	if len(colors) < 2 {
		(&harness{t: t, tt: tt}).shot("agent-context-segment-colors")
		t.Fatal("the real compaction reserve is not striped")
	}
}

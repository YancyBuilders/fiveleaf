package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestAgentThinkingRecognizesAdvertisedChoices(t *testing.T) {
	ordinary := []agentChoice{{id: "ask", name: "Ask"}, {id: "low", name: "Low"}, {id: "plan", name: "Plan"}}
	s := &agentChat{modes: ordinary}
	if got := agentThinkingModes(s); len(got) != 0 || !slices.Equal(agentGenericModes(s), ordinary) {
		t.Fatal("generic agent modes were reclassified as thinking")
	}
	s.modes = append(s.modes, agentChoice{id: "high", name: "Thinking: high"})
	if got := agentThinkingModes(s); len(got) != 1 || got[0].id != "high" || !slices.Equal(agentGenericModes(s), ordinary) {
		t.Fatal("legacy thinking choices were not separated from generic modes")
	}
	s = &agentChat{thinkingModes: true, modes: []agentChoice{{id: "off", name: "Off"}, {id: "high", name: "High"}}}
	if got := agentThinkingModes(s); !slices.Equal(got, s.modes) || len(agentGenericModes(s)) != 0 {
		t.Fatal("explicit thinking capability did not preserve exactly its advertised choices")
	}
	for _, choice := range agentThinkingModes(s) {
		if choice.id == "auto" {
			t.Fatal("Auto was invented for an adapter that does not advertise it")
		}
	}
	s.genericMode, s.genericModes = "ask", ordinary
	if !slices.Equal(agentGenericModes(s), ordinary) || agentGenericMode(s) != "ask" {
		t.Fatal("independent generic modes were hidden by thought_level choices")
	}
	if got := agentThinkingLabel(agentChoice{id: "adaptive", name: "Adaptive"}); got != "Adaptive" {
		t.Fatalf("unknown advertised thinking level was renamed: %q", got)
	}
}

func TestAgentThinkingRailMotionAndColors(t *testing.T) {
	withAppearancePlugins(t)
	for _, dark := range []bool{false, true} {
		p := paletteFor("", dark)
		state := agentThinkingState{}
		visible, blurred := true, false
		choices := make([]agentChoice, 7)
		choices[6].id = "low"
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Background(p.editor).Children(func() {
				if visible {
					ui.Box(c).Width(320).Height(28).Draw(func(pt *ui.Painter, r ui.Rect) {
						state.advance(pt.Now(), blurred || c.Preferences().ReduceMotion)
						if state.moving {
							pt.AnimationFrameRate(float32(time.Second) / float32(motionAgentThinkingFrame))
						}
						paintAgentThinkingRail(pt, r, p, choices, 6, state.elapsed, false, false)
					})
				}
			})
		}, 320, 28)
		first := micaSnapshot(tt)
		tt.Advance(motionAgentThinkingCycle / 3)
		if bytes.Equal(first.Pix, tt.Image().Pix) || !tt.FramePending() {
			t.Fatal("visible thinking rail did not move and request its next frame")
		}
		for _, pause := range []string{"reduce", "blur", "hide"} {
			switch pause {
			case "reduce":
				tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			case "blur":
				blurred = true
			case "hide":
				visible = false
			}
			tt.Frame()
			before := micaSnapshot(tt)
			tt.Advance(motionAgentThinkingCycle)
			if !bytes.Equal(before.Pix, tt.Image().Pix) || tt.FramePending() {
				t.Fatalf("%s thinking rail still moves or schedules frames", pause)
			}
			tt.SetPreferences(ui.Preferences{})
			blurred, visible = false, true
			tt.Frame()
		}
		tt.SetPreferences(ui.Preferences{ReduceMotion: true})
		images := map[string][]byte{}
		for _, id := range []string{"auto", "off", "minimal", "low", "medium", "high", "xhigh", "max"} {
			choices[6].id = id
			tt.Frame()
			image := micaSnapshot(tt)
			for other, before := range images {
				if bytes.Equal(before, image.Pix) {
					t.Fatalf("thinking levels %s and %s have identical colors", other, id)
				}
			}
			images[id] = image.Pix
			if dir := os.Getenv("SHOTS"); dir != "" {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("thinking-rail-%v-%s.png", dark, id)))
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(f, image); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
		}
	}
}

func thinkingRailTest(t *testing.T, hs *harness) ui.Rect {
	t.Helper()
	if !hs.a.agent.modelPicker.open {
		if err := hs.tt.Click(locales.AgentModel()); err != nil {
			t.Fatal(err)
		}
	}
	hs.settle()
	r, ok := hs.tt.Find(agentThinkingName)
	if !ok || r.W < 40 || r.H != 24 {
		t.Fatalf("thinking control is absent or clipped: %+v found=%v open=%v texts=%v",
			r, ok, hs.a.agent.modelPicker.open, hs.tt.Texts())
	}
	return r
}

func chooseThinkingTest(t *testing.T, hs *harness, id string) {
	t.Helper()
	choices := agentThinkingModes(hs.a.agent)
	index := slices.IndexFunc(choices, func(choice agentChoice) bool { return choice.id == id })
	if index < 0 {
		t.Fatalf("thinking level %q is not advertised: %v", id, choices)
	}
	r := thinkingRailTest(t, hs)
	x := r.X + 12 + (r.W-24)*float32(index)/float32(max(1, len(choices)-1))
	hs.tt.ClickAt(x, r.Y+r.H/2)
	hs.untilLong("thinking choice", func() bool { return !hs.a.agent.configuring && hs.a.agent.modelPicker.thinking.pending == nil })
	hs.settle()
}

func TestAgentThinkingPickerFirstSelection(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "thinking")
	hs.a.openAgentPage()
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	s.draft = "keep this question"
	if s.modelPicker.valid(hs.a, s) || s.modelPicker.open {
		t.Fatal("test opened the model picker before choosing thinking")
	}
	before := len(agentRoleEvents(t, logPath))
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	chooseThinkingTest(t, hs, "high")
	if s.mode != "high" || s.err != "" || s.draft != "keep this question" || !s.modelPicker.open {
		t.Fatalf("thinking choice failed: mode=%q error=%q draft=%q open=%v", s.mode, s.err, s.draft, s.modelPicker.open)
	}
	if events := agentRoleEvents(t, logPath)[before:]; len(events) != 1 || events[0].Kind != "mode" || events[0].Mode != "high" {
		t.Fatalf("thinking choice did not submit exactly one mode change: %+v", events)
	}
	hs.key(0, ui.KeyEscape)
	hs.settle()
	if s.mode != "high" || s.draft != "keep this question" || len(agentRoleEvents(t, logPath)) != before+1 {
		t.Fatal("dismissing the thinking picker changed the mode or draft")
	}
}

func TestAgentThinkingPickerDragKeyboardAndDraft(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "thinking")
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	s.draft, s.modelPicker.focus = "keep this question", true
	hs.tt = ui.NewTester(func(c *ui.Context) {
		hs.a.setServices(c.Services())
		hs.a.drain()
		ui.Column(c).Fill().Children(func() { hs.a.agentThinkingPickerRow(c, s, agentThinkingModes(s)) })
	}, 400, 60)
	hs.settle()
	r, found := hs.tt.Find(agentThinkingName)
	if !found || r.H != 24 || !hs.tt.Focused(agentThinkingName) {
		t.Fatalf("thinking rail is not keyboard accessible: %+v", r)
	}
	assertLevel := func(level string) {
		t.Helper()
		rail := hs.detailBounds(agentThinkingName)
		label := hs.detailBounds(level)
		if rail != r || label.X <= rail.X+rail.W || label.Y >= rail.Y+rail.H || label.Y+label.H <= rail.Y || hs.tt.HasText("· "+s.provider) {
			t.Errorf("expected only %q beside a stable rail: rail=%+v initial=%+v label=%+v texts=%v", level, rail, r, label, hs.tt.Texts())
		}
	}
	assertLevel("Medium")
	before := len(agentRoleEvents(t, logPath))
	y := r.Y + r.H/2
	for _, x := range []float32{r.X + 2, r.X + r.W/2, r.X + r.W - 2} {
		hs.tt.Move(x, y)
		if hs.tt.Cursor() != ui.CursorPointer {
			t.Errorf("no hand cursor at x=%.1f on rail %+v", x, r)
		}
	}
	at := func(level float32) float32 { return r.X + 12 + (r.W-24)*level/float32(len(s.modes)-1) }
	hs.tt.Press(at(0), y)
	probeX, probeY := int(at(1.1)), int(y)
	unfilled := hs.tt.Image().RGBAAt(probeX, probeY)
	for _, reduceMotion := range []bool{true, false} {
		hs.tt.SetPreferences(ui.Preferences{ReduceMotion: reduceMotion})
		for _, level := range []float32{0.65, 1.35, 0.65} {
			hs.tt.Move(at(level), y)
			filled := hs.tt.Image().RGBAAt(probeX, probeY) != unfilled
			if !hs.tt.HasText("Minimal") || filled != (level > 1.1) {
				t.Errorf("reduce motion=%v: drag to %.2f did not move the fill within the Minimal stop", reduceMotion, level)
			}
		}
	}
	releaseX := at(float32(len(s.modes)-1) - 0.35)
	for _, point := range []ui.Point{
		{X: r.X + r.W + 24, Y: r.Y + r.H/2},
		{X: r.X + r.W/2, Y: r.Y - 24},
		{X: releaseX, Y: y},
	} {
		hs.tt.Move(point.X, point.Y)
		hs.settle()
		if s.mode != "medium" || s.requestInFlight() || len(agentRoleEvents(t, logPath)) != before {
			t.Fatalf("dragging at %+v submitted before pointer release", point)
		}
	}
	endX := int(r.X + r.W - 4)
	beforeSnap := hs.tt.Image().RGBAAt(endX, probeY)
	hs.tt.Release(releaseX, y)
	hs.untilLong("thinking drag", func() bool { return !s.configuring && s.mode == "max" })
	hs.settle()
	assertLevel("Max")
	if hs.tt.Image().RGBAAt(endX, probeY) == beforeSnap {
		t.Fatal("releasing between stops did not settle the fill at Max")
	}
	if s.draft != "keep this question" || !hs.tt.Focused(agentThinkingName) {
		t.Fatal("thinking choice lost its draft or slider focus")
	}
	if events := agentRoleEvents(t, logPath)[before:]; len(events) != 1 || events[0].Kind != "mode" || events[0].Mode != "max" {
		t.Fatalf("drag committed more than one mode change: %+v", events)
	}
	for _, step := range []struct {
		key  ui.Key
		want string
	}{{ui.KeyHome, "off"}, {ui.KeyRight, "minimal"}, {ui.KeyEnd, "max"}, {ui.KeyLeft, "xhigh"}} {
		hs.key(0, step.key)
		hs.untilLong("thinking key", func() bool { return !s.configuring && s.mode == step.want })
		hs.settle()
	}
	for _, event := range agentRoleEvents(t, logPath)[before:] {
		if event.Kind == "model" {
			t.Fatal("thinking choices unnecessarily reselected and reset the model")
		}
	}
	s.busy = true
	hs.tt.Frame()
	hs.tt.Move(r.X+r.W/2, y)
	if hs.tt.Cursor() == ui.CursorPointer {
		t.Fatal("disabled thinking rail still shows a hand cursor")
	}
}

func TestAgentThinkingKeepsGenericModesOutside(t *testing.T) {
	hs, _ := agentRoleHarness(t, "mode-resets-model")
	connectAgentModelTest(t, hs)
	if _, found := hs.tt.Find(locales.AgentMode()); !found || len(agentGenericModes(hs.a.agent)) != 2 {
		t.Fatal("generic Ask and Plan modes disappeared from the composer")
	}
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if _, found := hs.tt.Find(agentThinkingName); found {
		t.Fatal("generic modes invented a thinking slider")
	}
}

func TestAgentThinkingReleaseKeepsChatOnline(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "thinking-pending")
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	chat.draft = "keep this draft"
	hs.tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	rail := thinkingRailTest(t, hs)
	modelRow, found := hs.tt.Find("model-b")
	if !found {
		t.Fatal("missing model row")
	}
	height := rail.Y + rail.H/2
	hs.tt.Press(rail.X+rail.W/2, height)
	hs.tt.Move(rail.X+rail.W-2, height)
	hs.settle()
	before := micaSnapshot(hs.tt)
	hs.tt.Release(rail.X+rail.W-2, height)
	hs.untilLong("adapter received thinking choice", func() bool {
		return slices.ContainsFunc(agentRoleEvents(t, logPath), func(event roleRuntimeEvent) bool { return event.Kind == "mode" && event.Mode == "max" })
	})
	hs.settle()
	status := agentTargetStatus(chat, false, hs.a.pal).label
	_, send := hs.tt.Find(locales.AgentSend())
	_, stop := hs.tt.Find(locales.AgentStop())
	if status != locales.AgentOnline() || hs.tt.HasText(locales.AgentConnecting()) || !send || stop {
		t.Errorf("changing thinking disrupted the chat: status=%q send=%v stop=%v", status, send, stop)
	}
	if !chat.modelPicker.open || chat.modelPicker.thinking.pending == nil {
		t.Fatal("the picker did not keep the unconfirmed choice")
	}
	after := hs.tt.Image()
	probeX, probeY := int(rail.X+rail.W/4), int(height)
	if before.RGBAAt(probeX, probeY) != after.RGBAAt(probeX, probeY) {
		t.Error("waiting for confirmation dimmed the slider fill")
	}
	for row := int(modelRow.Y) + 1; row < int(modelRow.Y+modelRow.H)-1; row++ {
		start, end := before.PixOffset(int(modelRow.X)+1, row), before.PixOffset(int(modelRow.X+modelRow.W)-1, row)
		if !bytes.Equal(before.Pix[start:end], after.Pix[start:end]) {
			t.Errorf("the unrelated model row flashed on slider release")
			break
		}
	}
	hs.a.sendChat(chat)
	if chat.draft != "keep this draft" || chat.busy {
		t.Fatal("a prompt started before its thinking level was confirmed")
	}
}

func TestAgentThinkingWithoutModelsRemainsAccessible(t *testing.T) {
	for _, scenario := range []string{"thinking-only", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			hs, _ := agentRoleHarness(t, scenario)
			role := hs.a.settings.AgentRoles[hs.a.agent.role]
			role.Model = ""
			hs.a.settings.AgentRoles[hs.a.agent.role] = role
			hs.a.showAgent()
			hs.a.connectAgent()
			hs.untilLong("no model connection", func() bool { return !hs.a.agent.connecting })
			hs.settle()
			s := hs.a.agent
			if s.err != "" || s.sessionID == "" || len(s.models) != 0 {
				t.Fatalf("no model connection: error=%q session=%q models=%v", s.err, s.sessionID, s.models)
			}
			if scenario == "missing" {
				hs.tt.Click(locales.AgentModel())
				hs.settle()
				if s.modelPicker.open {
					t.Fatal("an agent without model or thinking choices opened an empty picker")
				}
				return
			}
			chooseThinkingTest(t, hs, "high")
			if s.mode != "high" || s.err != "" || !s.modelPicker.open {
				t.Fatalf("thinking-only selection failed: mode=%q error=%q open=%v", s.mode, s.err, s.modelPicker.open)
			}
			hs.settle()
			if _, found := hs.tt.Find(locales.AgentModelSearch()); found || !s.modelPicker.open || !hs.tt.Focused(agentThinkingName) {
				t.Fatal("thinking-only popup is not keyboard accessible")
			}
			hs.key(0, ui.KeyEnd)
			hs.untilLong("thinking-only keyboard choice", func() bool { return !s.configuring && s.mode == "max" })
			hs.key(0, ui.KeyEscape)
			hs.settle()
			if s.modelPicker.open || !hs.tt.Focused(locales.AgentModel()) {
				t.Fatal("Escape did not return focus to the model control")
			}
			hs.shot("thinking-only-picker")
		})
	}
}

func TestAgentThinkingAndGenericModesOperateIndependently(t *testing.T) {
	hs, _ := agentRoleHarness(t, "thinking-mixed")
	hs.a.openAgentPage()
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	if agentGenericMode(s) != "ask" || len(agentGenericModes(s)) != 2 {
		t.Fatalf("independent Ask/Plan modes missing: current=%q options=%v", agentGenericMode(s), agentGenericModes(s))
	}
	chooseThinkingTest(t, hs, "high")
	hs.key(0, ui.KeyEscape)
	if err := hs.tt.Click(locales.AgentMode()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hs.tt.Menu(), []string{"Ask", "Plan"}) {
		t.Fatalf("generic menu includes thought levels or lost choices: %v", hs.tt.Menu())
	}
	if err := hs.tt.ChooseMenuItem("Plan"); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("generic Plan choice", func() bool { return !s.configuring && agentGenericMode(s) == "plan" })
	hs.settle()
	if s.mode != "high" || s.err != "" || !hs.tt.HasText("Plan") {
		t.Fatalf("Plan reset thinking or displayed the wrong current mode: thinking=%q generic=%q error=%q", s.mode, agentGenericMode(s), s.err)
	}
	chooseThinkingTest(t, hs, "low")
	if agentGenericMode(s) != "plan" || s.mode != "low" || s.err != "" {
		t.Fatalf("thinking change reset Plan: thinking=%q generic=%q error=%q", s.mode, agentGenericMode(s), s.err)
	}
}

func TestAgentThinkingPickerFailureRestoresConfirmedMode(t *testing.T) {
	hs, _ := agentRoleHarness(t, "thinking-error")
	connectAgentModelTest(t, hs)
	chooseThinkingTest(t, hs, "high")
	s := hs.a.agent
	if s.err == "" || s.mode != "medium" || s.modelPicker.thinking.pending != nil ||
		int(s.modelPicker.thinking.value) != 3 {
		t.Fatalf("failed thinking choice did not restore medium: mode=%s pending=%v value=%v open=%v error=%q",
			s.mode, s.modelPicker.thinking.pending, s.modelPicker.thinking.value, s.modelPicker.open, s.err)
	}
}

func TestAgentThinkingAdvertisedEmptyDefault(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "thinking-empty")
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	chooseThinkingTest(t, hs, "high")
	chooseThinkingTest(t, hs, "")
	if s.err != "" || s.mode != "" || !hs.tt.HasText("Provider default") {
		t.Fatalf("empty thinking default was not confirmed and displayed: mode=%q error=%q", s.mode, s.err)
	}
	chooseThinkingTest(t, hs, "high")
	hs.a.chooseAgentThinking(s, "")
	pending := s.modelPicker.thinking.pending
	if pending == nil || *pending != "" || !s.modelPicker.thinking.queued {
		t.Fatal("empty thinking value was treated as no pending choice")
	}
	hs.a.chooseAgentThinking(s, "max")
	if s.modelPicker.thinking.pending != pending {
		t.Fatal("another choice replaced the pending provider default")
	}
	hs.a.drain()
	hs.untilLong("empty thinking choice", func() bool { return !s.configuring && s.mode == "" })
	hs.settle()
	if s.modelPicker.thinking.pending != nil {
		t.Fatal("confirmed empty choice remained pending")
	}
	hs.key(0, ui.KeyEscape)
	if err := hs.tt.Click(locales.AgentMode()); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.ChooseMenuItem("Plan"); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("Plan with provider default", func() bool { return !s.configuring && agentGenericMode(s) == "plan" })
	if s.err != "" || s.mode != "" {
		t.Fatalf("generic selection reset provider default: mode=%q error=%q", s.mode, s.err)
	}
	for _, text := range []string{"use default", "keep default"} {
		lifecycleSend(t, hs, text)
		if s.err != "" || s.mode != "" {
			t.Fatalf("new prompt reset provider default: mode=%q error=%q", s.mode, s.err)
		}
	}
	var prompts int
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts++
			if event.Mode != "" {
				t.Fatalf("prompt sent without provider default: %+v", event)
			}
		}
	}
	if prompts != 2 {
		t.Fatalf("expected two confirmed prompts, got %d", prompts)
	}
}

func TestAgentThinkingPickerRejectsStaleSelection(t *testing.T) {
	for _, change := range []string{"root", "generation", "session", "client", "role", "provider", "chat"} {
		t.Run(change, func(t *testing.T) {
			hs, _ := agentRoleHarness(t, "thinking")
			connectAgentModelTest(t, hs)
			s := hs.a.agent
			hs.a.chooseAgentThinking(s, "max")
			switch change {
			case "root":
				hs.a.root += "/other"
			case "generation":
				s.generation++
			case "session":
				s.sessionID += "-other"
			case "client":
				client := s.client
				s.client = nil
				defer func() { s.client = client }()
			case "role":
				role := hs.a.settings.AgentRoles[s.role]
				s.role += "-other"
				hs.a.settings.AgentRoles[s.role] = role
			case "provider":
				s.provider += "-other"
			case "chat":
				hs.a.selectAgent("backend")
			}
			hs.a.drain()
			if s.requestInFlight() || s.mode != "medium" || s.modelPicker.thinking.pending != nil {
				t.Fatalf("stale thinking selection changed the session: configuring=%v mode=%q pending=%v", s.configuring, s.mode, s.modelPicker.thinking.pending)
			}
		})
	}
}

func TestAgentThinkingPickerFitsAndKeepsFooter(t *testing.T) {
	hs, _ := agentRoleHarness(t, "thinking")
	hs.a.window.SidebarClosed = true
	connectAgentModelTest(t, hs)
	chat := hs.a.agent
	for _, size := range [][2]int{{1000, 760}, {500, 480}, {360, 360}} {
		if size[0] == 360 {
			chat.attachFile(chatDraftRef{file: "notes.md", chatDetailSource: chatDetailSource{label: "notes.md"}})
		}
		hs.tt.SetSize(size[0], size[1])
		hs.settle()
		r := thinkingRailTest(t, hs)
		draft, found := hs.tt.Find(locales.AgentDraft())
		search, searched := hs.tt.Find(locales.AgentModelSearch())
		if !found || !searched || r.X < 0 || r.X+r.W > float32(size[0]) || r.Y < search.Y+search.H || r.Y+r.H > draft.Y {
			t.Fatalf("thinking footer does not fit above draft in %v: rail=%+v draft=%+v search=%+v", size, r, draft, search)
		}
		chooseThinkingTest(t, hs, "max")
		chat.acceptContextUsage(acp.UsageState{Used: 7000, Size: 1000000})
		hs.settle()
		usage := hs.detailBounds(locales.AgentContext())
		percentage := hs.detailBounds("0.7%")
		if r.Y+r.H >= usage.Y {
			t.Errorf("thinking rail overlaps context usage in %v: rail=%+v context=%+v", size, r, usage)
		}
		hs.shot(fmt.Sprintf("thinking-picker-%d-max", size[0]))
		percentageCenter := percentage.X + percentage.W/2
		hs.tt.Move(percentageCenter, usage.Y+1)
		if hs.tt.Cursor() != ui.CursorPointer {
			t.Errorf("picker covers the top of the context button in %v", size)
		}
		hs.tt.ClickAt(percentageCenter, percentage.Y+percentage.H/2)
		hs.settle()
		if chat.modelPicker.open || !chat.contextDetailsOpen {
			t.Fatalf("clicking context usage with the picker open in %v: picker=%v details=%v", size, chat.modelPicker.open, chat.contextDetailsOpen)
		}
		hs.key(0, ui.KeyEscape)
		hs.settle()
	}
}

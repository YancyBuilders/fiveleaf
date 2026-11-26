package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestAgentComposerKeyboardSubmit(t *testing.T) {
	for _, shortcut := range []struct {
		name string
		mods ui.Modifiers
	}{{"Enter", 0}, {"CmdOrCtrlEnter", ui.Cmd}} {
		t.Run(shortcut.name, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, "permission")
			connectAgentModelTest(t, hs)
			chat := hs.a.agent
			hs.clickDetail(locales.AgentDraft())
			hs.tt.Type("  ")
			hs.key(shortcut.mods, ui.KeyEnter)
			if chat.turn != 0 || chat.draft != "  " {
				t.Fatalf("blank submit: turn=%d draft=%q", chat.turn, chat.draft)
			}

			hs.tt.Key(ui.Cmd, ui.KeyA)
			hs.tt.Type("first line")
			hs.tt.Key(ui.Shift, ui.KeyEnter)
			hs.tt.Type("second ")
			hs.tt.Compose("hang", 4)
			hs.key(shortcut.mods, ui.KeyEnter)
			if chat.turn != 0 || chat.draft != "first line\nsecond " {
				t.Fatalf("Enter during composition: turn=%d draft=%q", chat.turn, chat.draft)
			}
			hs.tt.Type("行")
			const message = "first line\nsecond 行"
			if chat.draft != message {
				t.Fatalf("committed draft = %q, want %q", chat.draft, message)
			}
			chat.draft = ""
			hs.key(shortcut.mods, ui.KeyEnter)
			if chat.turn != 1 || chat.draft != "" {
				t.Fatalf("submit: turn=%d draft=%q", chat.turn, chat.draft)
			}
			hs.until("ACP permission request", func() bool { return len(chat.permissions) == 1 })
			hs.tt.Type("next draft")
			hs.key(shortcut.mods, ui.KeyEnter)
			if !chat.busy || chat.turn != 1 || chat.draft != "next draft" {
				t.Fatalf("Enter while pending: busy=%t turn=%d draft=%q", chat.busy, chat.turn, chat.draft)
			}
			requireComposerPrompt(t, logPath, message)
		})
	}
}

func TestAgentComposerEnterSendsFocusedAttachment(t *testing.T) {
	hs, logPath := agentRoleHarness(t, "acp")
	connectAgentModelTest(t, hs)
	sibling := hs.a.agent
	hs.clickDetail(locales.AgentDraft())
	hs.tt.Type("keep this draft")
	hs.a.splitRight()
	hs.settle()
	chat := hs.a.agent
	if chat == sibling || !chat.input.Focused(hs.a.review.ctx) {
		t.Fatal("expected a separate focused chat in the split")
	}
	hs.until("focused chat connection", func() bool { return chat.sessionID != "" })
	hs.h.clipboard = []string{hs.path("go.mod")}
	hs.tt.SetClipboard("go.mod")
	hs.command("paste")
	hs.until("pasted attachment", func() bool { return chat.contextLoading == 0 })
	message := chat.draftPrompt()
	if chat.draft != "" || !strings.Contains(message, "module example") {
		t.Fatalf("file paste: draft=%q prompt=%q", chat.draft, message)
	}
	hs.key(0, ui.KeyEnter)
	hs.until("attachment reply", func() bool { return chat.turn == 1 && !chat.busy })
	if chat.hasDraft() || sibling.turn != 0 || sibling.draft != "keep this draft" {
		t.Fatalf("attachment send: hasDraft=%t sibling turn=%d draft=%q", chat.hasDraft(), sibling.turn, sibling.draft)
	}
	requireComposerPrompt(t, logPath, message)
}

func requireComposerPrompt(t *testing.T, logPath, message string) {
	t.Helper()
	var prompts []string
	for _, event := range agentRoleEvents(t, logPath) {
		if event.Kind == "prompt" {
			prompts = append(prompts, event.Text)
		}
	}
	if len(prompts) != 1 || !strings.HasSuffix(prompts[0], "[user]\n"+message+"\n\n") {
		t.Fatalf("recorded ACP prompts = %q, want one ending with %q", prompts, message)
	}
}

func TestAgentComposerContextOverlaysInputBorder(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%v", dark), func(t *testing.T) {
			hs := newHarness(t, dark)
			agentViewFixture(hs)
			s := hs.a.agent
			s.acceptContextUsage(acp.UsageState{Used: 250000, Size: 1000000})
			s.contextPolicy = agentCompactionPolicy{known: true, enabled: true, reserveTokens: 150000}
			for _, width := range []float32{160, 180, 340, 900} {
				sizeChatWorkspace(hs, width)
				hs.settle()
				usage, okUsage := hs.tt.Find(locales.AgentContext())
				input, okInput := hs.tt.Find(locales.AgentDraft())
				send, okSend := hs.tt.Find(locales.AgentSend())
				if !okUsage || !okInput || !okSend || input.H != 52 || send.Y < input.Y+input.H {
					t.Fatalf("context/input/footer layout is wrong (width=%v): usage=%+v input=%+v send=%+v", width, usage, input, send)
				}
				if input.Y < 500 || send.Y > 730 {
					t.Fatalf("empty composer does not stay compact at the bottom: input=%+v send=%+v", input, send)
				}
				p, m := hs.a.pal, hs.a.pal.metricsOr()
				img := hs.tt.Image()
				box := s.composeBox.Bounds(hs.a.review.ctx)
				if gap := float32(img.Bounds().Dy()-statusBarH) - (box.Y + box.H); math.Abs(float64(gap-m.pad)) > 0.01 {
					t.Fatalf("width=%v: gap between composer and status bar = %v, want %v", width, gap, m.pad)
				}
				inset := m.radiusCard + m.pad + m.gap/2
				track := ui.Rect{X: usage.X + inset, Y: usage.Y + usage.H - 2, W: usage.W - 2*inset, H: 2}
				if math.Abs(float64(track.Y-(input.Y-1))) > 0.01 {
					t.Fatalf("context rail does not overlap the top border (width=%v): track=%+v input=%+v", width, track, input)
				}
				if track.X < input.X-1+m.radiusCard+m.pad || track.X+track.W > input.X+input.W+1-m.radiusCard-m.pad {
					t.Fatalf("context rail reaches the rounded corners: track=%+v input=%+v", track, input)
				}
				capacity, found := hs.tt.Find(locales.AgentContextCapacity("1M"))
				if !found || math.Abs(float64(capacity.X+capacity.W-track.X-track.W)) > 0.1 || capacity.Y+capacity.H > track.Y-m.gap/2+0.1 {
					t.Fatalf("capacity does not sit above the rail's right end: capacity=%+v track=%+v", capacity, track)
				}
				percentage, found := hs.tt.Find("25%")
				if !found || math.Abs(float64(percentage.X-track.X)) > 0.1 || percentage.X+percentage.W >= capacity.X || percentage.Y+percentage.H > track.Y-m.gap/2+0.1 {
					t.Fatalf("percentage does not sit above the rail's left end: percentage=%+v capacity=%+v track=%+v", percentage, capacity, track)
				}
				assertPixel := func(x, y float32, want ui.Color) {
					t.Helper()
					pixel := img.RGBAAt(int(math.Round(float64(x))), int(math.Round(float64(y))))
					if abs(int(pixel.R)-int(want.R)) > 2 || abs(int(pixel.G)-int(want.G)) > 2 || abs(int(pixel.B)-int(want.B)) > 2 {
						t.Fatalf("rail at (%v,%v) is %v, want %v (width=%v)", x, y, pixel, want, width)
					}
				}
				for _, y := range []float32{input.Y - 1, input.Y} {
					assertPixel(track.X+track.W/8, y, p.accent)
					assertPixel(track.X+track.W/2, y, p.border)
				}
				assertPixel(track.X+track.W*0.85-0.5, track.Y-1, p.modifiedFg)
				for _, hit := range []ui.Rect{percentage, track, capacity, usage} {
					hs.tt.Move(hit.X+hit.W/2, hit.Y+hit.H/2)
					if cursor := hs.tt.Cursor(); cursor != ui.CursorPointer {
						t.Fatalf("context bar cursor at %+v = %v, want a hand (width=%v)", hit, cursor, width)
					}
				}
				hs.tt.Move(input.X+input.W/2, input.Y+input.H/2)
				if hs.tt.Cursor() == ui.CursorPointer {
					t.Fatal("the context hand cursor extends into the draft")
				}
				t.Logf("width=%v input=%+v context rail=%+v capacity=%+v", width, input, track, capacity)
				hs.shot(fmt.Sprintf("agent-context-edge-dark-%v-width-%v", dark, width))
				// The visible rail is still the details button, including the
				// pixel it shares with the compose border.
				hs.tt.ClickAt(track.X+track.W/2, track.Y+0.5)
				hs.settle()
				if !s.contextDetailsOpen || !hs.tt.HasText(locales.ButtonClose()) {
					t.Fatal("clicking the rail on the input border did not open context details")
				}
				hs.key(0, ui.KeyEscape)
				if width == 900 {
					cwd, found := hs.tt.Find(locales.AgentWorkspaceDirectory())
					if !found || cwd.Y < input.Y+input.H || !hs.tt.HasText(shortenHome(hs.root)) {
						t.Fatal("wide composer omitted the actual ACP workspace directory")
					}
				}
			}
		})
	}
}

func TestAgentComposerFooterFitsWithoutOverlap(t *testing.T) {
	hs, _ := agentRoleHarness(t, "mode-resets-model")
	connectAgentModelTest(t, hs)
	for _, width := range []float32{160, 180, 280, 340, 600, 800} {
		sizeChatWorkspace(hs, width)
		hs.settle()
		var controls []ui.Rect
		for _, label := range []string{locales.AgentAttachFiles(), locales.AgentModel(), locales.AgentChoose(), locales.AgentMode(), locales.AgentSend()} {
			r, found := hs.tt.Find(label)
			if !found || r.X < 0 || r.X+r.W > width+1 || r.W < 15 || r.H <= 0 {
				t.Fatalf("%q is not reachable at width %v: %+v", label, width, r)
			}
			for _, previous := range controls {
				if min(r.X+r.W, previous.X+previous.W)-max(r.X, previous.X) > 1 && min(r.Y+r.H, previous.Y+previous.H)-max(r.Y, previous.Y) > 1 {
					t.Fatalf("footer controls overlap at width %v: %+v / %+v", width, previous, r)
				}
			}
			controls = append(controls, r)
		}
	}
}

func TestAgentComposerCaretAndFocusStayVisible(t *testing.T) {
	withAppearancePlugins(t)
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%v", dark), func(t *testing.T) {
			hs := newHarness(t, dark)
			if dark {
				hs.a.setTheme(themeOneDarkPro)
			}
			hs.a.settings.MicaEnabled, hs.a.settings.MicaAnimation = true, false
			agentViewFixture(hs)
			hs.settle()
			hs.clickDetail(locales.AgentDraft())
			hs.tt.Type("draft")
			hs.tt.Frame()
			for _, delay := range []time.Duration{0, 700 * time.Millisecond, 31 * time.Second} {
				hs.tt.Advance(delay)
				hs.tt.Frame()
				caret, ok := hs.tt.TextCaret()
				if !ok || !hs.tt.Focused(locales.AgentDraft()) {
					t.Fatal("the draft lost text input focus")
				}
				frame := hs.tt.Image()
				pixel := func(x, y int) ui.Color {
					color := frame.RGBAAt(x, y)
					return ui.RGB(color.R, color.G, color.B)
				}
				left := int(math.Round(float64(caret.X)))
				middle := int(caret.Y + caret.H/2)
				background := pixel(left+4, middle)
				for column := left; column < left+2; column++ {
					if ratio := contrast(pixel(column, middle), background); ratio < 4.5 {
						t.Errorf("at %v, caret column %d has contrast %.2f:1", delay, column-left, ratio)
					}
				}
				box := hs.a.agent.composeBox.Bounds(hs.a.review.ctx)
				edge, row := int(math.Round(float64(box.X))), int(box.Y+box.H/2)
				if ratio := contrast(pixel(edge, row), pixel(edge+4, row)); ratio < 3 {
					t.Errorf("at %v, focus edge has contrast %.2f:1", delay, ratio)
				}
			}
			hs.shot(fmt.Sprintf("agent-composer-caret-mica-dark-%v", dark))
		})
	}
}

func TestAgentComposerLongPastesStayEditable(t *testing.T) {
	for _, specimen := range []struct{ name, text string }{
		{"multiline", strings.Repeat("2026-11-25 INFO handled /workspace/request\n", 90)},
		{"unbroken", strings.Repeat("abc0123456789界", 1024)},
	} {
		t.Run(specimen.name, func(t *testing.T) {
			hs := newHarness(t, true)
			agentViewFixture(hs)
			sizeChatWorkspace(hs, 600)
			hs.settle()
			hs.clickDetail(locales.AgentDraft())
			const draft = "replace this draft"
			hs.tt.Type(draft)
			hs.command("selectAll")
			before := hs.detailBounds(locales.AgentDraft())
			beforeSend := hs.detailBounds(locales.AgentSend())
			hs.tt.SetClipboard(specimen.text)
			hs.command("paste")
			input := hs.detailBounds(locales.AgentDraft())
			send := hs.detailBounds(locales.AgentSend())
			if input.H < before.H+40 || input.H > 150 || send.Y < input.Y+input.H || send.Y != beforeSend.Y {
				t.Fatalf("long paste should expand upward with a bounded height: before=%+v input=%+v send=%+v", before, input, send)
			}
			chat := hs.a.agent
			if chat.draft != specimen.text || len(chat.sources.draft.refs) != 0 {
				t.Fatalf("paste changed the text or made it an attachment, draft bytes=%d", len(chat.draft))
			}
			hs.shot("agent-composer-long-" + specimen.name)
			hs.key(ui.Ctrl, ui.KeyHome)
			if caret, ok := hs.tt.TextCaret(); !ok || caret.Y < input.Y || caret.Y+caret.H > input.Y+input.H {
				t.Fatalf("the start of the pasted draft isn't in view: caret=%+v input=%+v", caret, input)
			}
			hs.tt.Type("备注 ")
			hs.settle()
			if chat.draft != "备注 "+specimen.text {
				t.Fatal("could not edit the start of the pasted draft")
			}
			hs.command("undo")
			hs.command("undo")
			if chat.draft != draft || hs.detailBounds(locales.AgentDraft()).H != before.H {
				t.Fatal("undo did not restore the original draft and input height")
			}
		})
	}
}

func TestAgentComposerFoldsLongPasteAndSendsOriginal(t *testing.T) {
	hs, logPath, _ := lifecycleHarness(t, "codex-native")
	hs.a.openAgentPage()
	chat := hs.a.agent
	hs.untilLong("Codex session connected", func() bool { return chat.sessionID != "" })
	hs.clickDetail(locales.AgentDraft())
	const note = "Compare these logs"
	hs.tt.Type(note)
	before := hs.detailBounds(locales.AgentDraft())
	const start = "first line with leading whitespace\t  "
	text := start + "\n" + strings.Repeat("2026-11-25 看请求日志 🧪\n", 150) + "last line\n\n"
	hs.tt.SetClipboard(text)
	hs.command("paste")
	label := locales.AgentPasteText(strings.Count(text, "\n") + 1)
	hs.until("folded paste", func() bool { return len(chat.sources.draft.pastes) == 1 && hs.tt.HasText(label) })
	if chat.draft != note || hs.detailBounds(locales.AgentDraft()).H != before.H {
		t.Fatal("folded paste changed the text or height of the draft input")
	}
	hs.shot("agent-paste-folded")
	hs.clickDetail(label)
	if chat.expandedPaste == nil || !strings.HasPrefix(chat.expandedPaste.preview, start) {
		t.Fatal("the pasted text preview did not open")
	}
	hs.shot("agent-paste-expanded")
	hs.clickDetail(locales.AgentPasteRemove())
	if len(chat.sources.draft.pastes) != 0 || chat.draft != note || chat.expandedPaste != nil {
		t.Fatal("removing a paste changed the draft or left the preview open")
	}
	hs.tt.SetClipboard(text)
	hs.command("paste")
	hs.until("reattached paste", func() bool { return len(chat.sources.draft.pastes) == 1 })
	hs.key(0, ui.KeyEnter)
	hs.untilLong("reply containing pasted text", func() bool { return chat.turn == 1 && !chat.requestInFlight() })
	events := lifecycleEvents(t, logPath)
	if len(events) != 2 || events[1].Kind != "prompt" || events[1].Text != note+"\n\n"+text || chat.hasDraft() {
		t.Fatalf("sending folded paste changed the content or left a draft: events=%d draft=%q error=%q", len(events), chat.draft, chat.err)
	}
}

func TestAgentComposerRestoresPastedTextBeforeDispatch(t *testing.T) {
	text := strings.Repeat("original context \n", 30)
	first := &chatDraftPaste{text: text}
	second := &chatDraftPaste{text: "new context\n"}
	chat := &agentChat{}
	chat.sources.draft.pastes = []*chatDraftPaste{first}
	prompt := chat.draftPrompt()
	chat.stageDetailSources()
	chat.draft = "new draft"
	chat.sources.draft.pastes = []*chatDraftPaste{second}
	restoreChatDraft(chat, prompt)
	if chat.draft != "new draft" || chat.draftPrompt() != "new draft\n\n"+text+"\n\n"+second.text {
		t.Fatalf("restored paste duplicated or displaced content: draft=%q prompt=%q", chat.draft, chat.draftPrompt())
	}
}

func TestAgentComposerEditingPastePreservesBlockOrder(t *testing.T) {
	hs, logPath, _ := lifecycleHarness(t, "codex-native")
	hs.a.openAgentPage()
	chat := hs.a.agent
	hs.untilLong("Codex session connected", func() bool { return chat.sessionID != "" })
	hs.clickDetail(locales.AgentDraft())
	const note = "Read these in order"
	hs.tt.Type(note)
	first, second, third := strings.Repeat("first \n", 21), strings.Repeat("second\n", 22), strings.Repeat("third\n", 23)
	for _, text := range []string{first, second, third} {
		hs.tt.SetClipboard(text)
		hs.command("paste")
	}
	hs.clickDetail(locales.AgentPasteText(23))
	hs.clickDetail(locales.AgentPasteEdit())
	if chat.draft != note+"\n\n"+first+"\n\n"+second || len(chat.sources.draft.pastes) != 1 || chat.sources.draft.pastes[0].text != third {
		t.Fatal("editing the middle attachment moved it before an earlier paste")
	}
	hs.key(0, ui.KeyEnter)
	hs.untilLong("reply containing ordered pastes", func() bool { return chat.turn == 1 && !chat.requestInFlight() })
	events := lifecycleEvents(t, logPath)
	if len(events) != 2 || events[1].Text != note+"\n\n"+first+"\n\n"+second+"\n\n"+third {
		t.Fatal("the ACP prompt did not receive all paste blocks in order")
	}
}

func TestShotAgentTernComposer(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	for _, language := range []string{"en", "ja", "zh-Hans"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-dark-%v", language, dark), func(t *testing.T) {
				previous := i18n.Locale()
				i18n.SetLocaleFunc(func() string { return language })
				t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
				hs, _ := agentRoleHarness(t, "acp")
				hs.tt.SetLocale(language)
				hs.tt.SetDark(dark)
				hs.a.selectAgent("backend")
				connectAgentModelTest(t, hs)
				s := hs.a.agent
				s.models = []acp.ModelChoice{
					{ID: "magpie/deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", Group: "Magpie"},
					{ID: "magpie/glm-5.3-flash", Name: "GLM 5.3 Flash", Group: "Magpie"},
					{ID: "magpie/kimi-k3.1-pro", Name: "Kimi K3.1 Pro", Group: "Magpie"},
					{ID: "tencent/claude-opus-5.5", Name: "Claude Opus 5.5", Group: "Tencent"},
					{ID: "tencent/gemini-3.5-flash", Name: "Gemini 3.5 Flash", Group: "Tencent"},
					{ID: "tencent/glm-5.3-flash", Name: "GLM 5.3 Flash", Group: "Tencent"},
				}
				s.currentModel = s.models[0].ID
				s.recentModels = []string{s.models[0].ID, s.models[1].ID}
				s.modes = []agentChoice{{id: "low"}, {id: "high"}, {id: "max"}}
				s.thinkingModes, s.mode = true, "max"
				s.acceptContextUsage(acp.UsageState{Used: 40000, Size: 1000000})
				s.contextPolicy = agentCompactionPolicy{known: true, enabled: true, reserveTokens: 150000}
				s.messages = []*agentMessage{{role: "assistant", text: "## Ready to continue\n\nCode stays readable, with **syntax highlighting** and `copy` controls.\n\n```go\nfunc greet(name string) string {\n    // Keep the conversation moving.\n    return \"Hello, \" + name\n}\n```"}}
				hs.a.openAgentPage()
				hs.settle()
				s.scroll.Y = math.MaxFloat32
				hs.settle()
				hs.shot(fmt.Sprintf("tern-composer-page-%s-dark-%v", language, dark))
				if err := hs.tt.Click(locales.AgentModel()); err != nil {
					t.Fatal(err)
				}
				hs.settle()
				hs.shot(fmt.Sprintf("tern-picker-page-%s-dark-%v", language, dark))
				hs.key(0, ui.KeyEscape)
				sizeChatWorkspace(hs, 340)
				hs.settle()
				hs.shot(fmt.Sprintf("tern-composer-narrow-%s-dark-%v", language, dark))
				if err := hs.tt.Click(locales.AgentModel()); err != nil {
					t.Fatal(err)
				}
				hs.settle()
				hs.shot(fmt.Sprintf("tern-picker-narrow-%s-dark-%v", language, dark))
			})
		}
	}
}

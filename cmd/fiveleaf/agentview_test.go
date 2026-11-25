package main

import (
	"fmt"
	"image"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestAgentViewKeepsDraftWithoutLaunching(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	a.showAgent()
	hs.settle()
	if !hs.tt.HasText(locales.AgentSetupTitle()) {
		t.Fatal("the unconfigured view does not explain what is missing")
	}
	if err := hs.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	hs.tt.Type("Explain this project")
	hs.settle()
	if a.agent.draft != "Explain this project" || a.agent.client != nil || a.agent.connecting {
		t.Fatal("opening and typing in the agent view started a process or lost the draft")
	}
	for _, label := range []string{locales.AgentConfigure(), locales.CmdPluginsManage()} {
		if err := hs.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	if !slices.Equal(hs.h.settingsShown, []string{settings.CatAgent + ":agentServers", settings.CatPlugins + ":"}) {
		t.Fatalf("setup opened the wrong settings: %v", hs.h.settingsShown)
	}
	a.showExplorer()
	hs.settle()
	a.showAgent()
	hs.settle()
	if a.agent.draft != "Explain this project" {
		t.Fatal("switching sidebar views lost the draft")
	}
	agentViewFixture(hs)
	hs.settle()
	if hs.tt.HasText(locales.AgentSetupTitle()) || a.agent.draft != "Explain this project" {
		t.Fatal("configuring an agent should retire the setup and keep the draft")
	}
	a.agent.messages = []*agentMessage{{role: "assistant", text: "Existing reply"}}
	a.settings.AgentServers = nil
	hs.settle()
	if hs.tt.HasText(locales.AgentSetupTitle()) || !hs.tt.HasText("Existing reply") || !hs.tt.HasText(locales.AgentNoProviders()) {
		t.Fatal("removing the last agent should leave the transcript visible with inline setup actions")
	}
}

func TestAgentSetupWelcomeLayout(t *testing.T) {
	for _, view := range []struct {
		name, language string
		width          int
		wide, wrapped  bool
	}{
		{"desktop", "zh-Hans", 1440, true, false},
		{"compact", "zh-Hans", 390, false, false},
		{"narrow", "ja", 180, false, true},
	} {
		t.Run(view.name, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return view.language })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			hs := newHarness(t, view.wide)
			agentHome(t, hs.root)
			if view.wide {
				hs.a.showSessions()
			}
			hs.a.openAgentPage()
			hs.a.window.SidebarClosed = !view.wide
			hs.tt.SetLocale(view.language)
			hs.tt.SetSize(view.width, 760)
			hs.settle()
			if view.wide {
				if err := hs.tt.Click(locales.AgentChatDetails()); err != nil {
					t.Fatal(err)
				}
				hs.settle()
			}
			boxes := hs.layoutOf(locales.AgentConfigure(), locales.CmdPluginsManage(), locales.AgentDraft(), locales.AgentSetupTitle())
			primary, secondary := boxes[locales.AgentConfigure()], boxes[locales.CmdPluginsManage()]
			draft, title := boxes[locales.AgentDraft()], boxes[locales.AgentSetupTitle()]
			centerDelta := (title.X + title.W/2) - (draft.X + draft.W/2)
			if centerDelta < -2 || centerDelta > 2 || title.Y < draft.Y/4 || title.Y > draft.Y*0.8 {
				t.Fatalf("setup is not centered in the chat transcript: title=%v draft=%v", title, draft)
			}
			for _, control := range []ui.Rect{primary, secondary} {
				if control.W <= 0 || control.W > 200 || control.H < 44 || control.X < draft.X-1 ||
					control.X+control.W > draft.X+draft.W+1 || control.Y+control.H >= draft.Y {
					t.Fatalf("setup action is stretched or outside the chat: action=%v draft=%v", control, draft)
				}
			}
			if view.wrapped && secondary.Y < primary.Y+primary.H || !view.wrapped && (secondary.Y != primary.Y || secondary.X <= primary.X+primary.W) {
				t.Fatalf("incorrect action alignment: primary=%v secondary=%v want wrapped=%v", primary, secondary, view.wrapped)
			}
			hs.shot("new-chat-" + view.name)
			if view.name == "compact" {
				hs.shotRect("agent-setup-actions", image.Rect(0, int(title.Y)-78, view.width, int(secondary.Y+secondary.H)+18), 1)
			}
		})
	}
}

func TestAgentViewStates(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	r := hs.a.agent
	for _, state := range []struct {
		connecting, busy, stopping bool
		label                      string
	}{
		{connecting: true, label: locales.AgentConnecting()},
		{busy: true, label: locales.AgentWorking()},
		{busy: true, stopping: true, label: locales.AgentStopping()},
	} {
		r.connecting, r.busy, r.stopping = state.connecting, state.busy, state.stopping
		hs.settle()
		if !hs.tt.HasText(state.label) {
			t.Errorf("state %q is absent", state.label)
		}
		if _, found := hs.tt.Find(locales.AgentStop()); !found {
			t.Errorf("state %q has no stop action", state.label)
		}
	}
	r.connecting, r.busy, r.stopping = false, false, false
	r.err = "The agent closed the connection."
	hs.settle()
	if !hs.tt.HasText(r.err) {
		t.Fatal("the connection error disappeared")
	}
	if _, found := hs.tt.Find(locales.AgentReconnect()); !found {
		t.Fatal("a connection failure has no next action")
	}
}

func TestAgentRecoveryActionLayout(t *testing.T) {
	for _, view := range []struct {
		name, language string
		width          int
		dark           bool
	}{{"desktop", "zh-Hans", 1280, false}, {"split", "en", 210, true}, {"minimum", "ja", 160, false}} {
		t.Run(view.name, func(t *testing.T) {
			previous := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return view.language })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
			hs := newHarness(t, view.dark)
			agentViewFixture(hs)
			hs.a.window.SidebarClosed = true
			hs.tt.SetLocale(view.language)
			hs.tt.SetSize(view.width, 520)
			chat := hs.a.agent
			chat.err = `start ACP agent: exec: "opencode": executable file not found in $PATH`
			chat.draft = "Keep my question"
			chat.modes = []agentChoice{{id: "plan", name: "Plan"}}
			hs.settle()
			message := hs.detailBounds(chat.err)
			reconnect := hs.detailBounds(locales.AgentReconnect())
			draft := hs.detailBounds(locales.AgentDraft())
			send := hs.detailBounds(locales.AgentSend())
			if reconnect.Y < draft.Y+draft.H || reconnect.X < draft.X || reconnect.X+reconnect.W > send.X || send.X+send.W > draft.X+draft.W ||
				reconnect.Y >= send.Y+send.H || send.Y >= reconnect.Y+reconnect.H || reconnect.H < 26 {
				t.Fatalf("reconnect should sit beside send in the composer: reconnect=%v send=%v draft=%v", reconnect, send, draft)
			}
			if (view.name == "desktop") != (reconnect.W > reconnect.H+1) {
				t.Fatalf("unexpected reconnect button size in %s: %v", view.name, reconnect)
			}
			for _, label := range []string{locales.AgentAttachFiles(), locales.AgentModel(), locales.AgentChoose(), locales.AgentMode()} {
				if control := hs.detailBounds(label); overlaps(control, reconnect) || overlaps(control, send) {
					t.Fatalf("%s overlaps a composer action: control=%v reconnect=%v send=%v", label, control, reconnect, send)
				}
			}
			hs.shot("reconnect-" + view.name)
			if view.name == "desktop" {
				hs.shotRect("reconnect-composer", image.Rect(int(reconnect.X)-157, int(draft.Y)-10, int(draft.X+draft.W)+10, int(send.Y+send.H)+16), 1)
			}
			hs.tt.ClickAt(message.X+2, message.Y+message.H+6)
			hs.settle()
			if chat.err == "" {
				t.Fatal("clicking below the error reconnected")
			}
			method := "Sign in with an organization-managed account"
			chat.authMethods = []agentChoice{{id: "browser", name: method}}
			hs.settle()
			auth := hs.detailBounds(locales.AgentAuthenticate() + ": " + method)
			if auth.X < message.X-1 || auth.X+auth.W > message.X+message.W+1 || auth.H < 26 {
				t.Fatalf("authentication should fit the transcript: action=%v message=%v", auth, message)
			}
			if auth.Y < message.Y+message.H-1 || auth.Y+auth.H >= draft.Y {
				t.Fatalf("sign-in should stay with the error: auth=%v message=%v draft=%v", auth, message, draft)
			}
			if err := hs.tt.Click(locales.AgentReconnect()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if chat.err != "" || chat.draft != "Keep my question" {
				t.Fatalf("reconnect left error %q and draft %q", chat.err, chat.draft)
			}
		})
	}
}

func TestAgentPermissionChoices(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	for _, choice := range []struct{ label, id string }{{"Allow once", "once"}, {locales.ButtonCancel(), ""}} {
		called, selected := false, "unset"
		hs.a.agent.permissions = []*agentPermission{{
			title:   "Write main.go",
			detail:  "main.go:7\n{\n  \"command\": \"go test ./cmd/fiveleaf -run TestAgentView\"\n}",
			options: []acp.PermissionOption{{OptionId: "once", Kind: "allow_once", Name: "Allow once"}, {OptionId: "deny", Kind: "reject_once", Name: "Reject"}},
			choose:  func(id string) { called, selected = true, id },
		}}
		hs.settle()
		if !hs.tt.HasText("main.go:7") || !hs.tt.HasText("go test ./cmd/fiveleaf -run TestAgentView") {
			t.Fatal("permission details omit the path or command")
		}
		if err := hs.tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		if !called || selected != choice.id {
			t.Fatalf("permission choice %q returned %q (called %v)", choice.label, selected, called)
		}
	}
}

func TestAgentDraftContextMenu(t *testing.T) {
	const text = "first()\nsecond()\n"
	for _, tc := range []struct {
		name                 string
		anchor, caret, click int
		wantSelection        bool
	}{
		{"caret", 3, 3, 3, false},
		{"inside backward selection", len("first()\nsecond"), 1, 3, true},
		{"outside selection", 0, len("first"), 7, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs, d := editHarness(t, "agent.go", text)
			hs.place(d, tc.anchor, tc.caret)
			bounds, found := hs.tt.Find(locales.EditorLabel(d.name()))
			if !found {
				t.Fatal("the editor is missing")
			}
			hs.tt.RightClickAt(bounds.X+d.gutterW+gutterGap+float32(tc.click)*hs.a.font.charW, bounds.Y+editorPadY+hs.a.font.lineH/2)
			want, hidden := locales.CmdAgentAddFile(), locales.CmdAgentAddSelection()
			draft := agentContextText(d.path, 0, 0, text)
			if tc.wantSelection {
				want, hidden = hidden, want
				draft = agentContextText(d.path, 1, 2, "irst()\nsecond")
			}
			if menu := hs.tt.Menu(); !slices.Contains(menu, want) || slices.Contains(menu, hidden) {
				t.Fatalf("menu = %v; want %q without %q", menu, want, hidden)
			}
			if err := hs.tt.ChooseMenuItem(want); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if !hs.a.isAgentPageVisible() || hs.a.agent.draftPrompt() != draft || !tc.wantSelection && hs.a.agent.draft != "" {
				t.Fatalf("draft = %q, prompt = %q, want %q", hs.a.agent.draft, hs.a.agent.draftPrompt(), draft)
			}
			if len(hs.a.agent.messages) != 0 || hs.a.agent.busy || hs.a.agent.connecting {
				t.Fatal("adding context sent it without a send action")
			}
		})
	}
}

func TestAgentViewNarrowLayout(t *testing.T) {
	hs := newHarness(t, false)
	hs.a.openFile(hs.path("main.go"), true)
	agentViewFixture(hs)
	hs.a.agent.messages = []*agentMessage{{role: "connection", text: "Adapter ready"}}
	hs.settle()
	for _, width := range []float32{340, 200, 160} {
		sizeChatWorkspace(hs, width)
		hs.settle()
		for _, label := range []string{locales.AgentDraft(), locales.AgentSend(), locales.AgentChatDetails(), locales.AgentNewSession(), locales.AgentChoose(), locales.AgentSelectionAutomatic()} {
			bounds, found := hs.tt.Find(label)
			if !found || bounds.X < 0 || bounds.X+bounds.W > width+1 || bounds.H <= 0 {
				t.Errorf("%q is outside a %v-wide agent view: %+v (found %v)", label, width, bounds, found)
			}
		}
	}
	hs.shot("primary-chat-narrow")
	mark, _ := hs.tt.Find(locales.AgentSelectionAutomatic())
	status, hasStatus := hs.tt.Find(locales.AgentTaskIdle())
	close, hasClose := hs.tt.Find(hs.a.activeTab().closeLabel())
	if !hasStatus || !hasClose || status.X < mark.X+mark.W || status.X+status.W > close.X+1 || close.X+close.W > 160 {
		t.Fatalf("compact task status does not fit between the agent and close icons: %v %v %v", mark, status, close)
	}
	hs.tt.Move(mark.X+mark.W/2, mark.Y+mark.H/2)
	hs.tt.Advance(time.Second)
	hs.tt.Frame()
	if !hs.tt.HasText(agentConversationTitle(hs.a.agent) + "\n" + locales.AgentSelectionAutomatic() + "\n" + locales.AgentTaskIdle()) {
		t.Fatal("the compact chat tab lost its title, selection hint or task status on hover")
	}
	for _, removed := range []string{"Add Context", "Conversation Actions", "Pin this chat for code selections"} {
		if _, found := hs.tt.Find(removed); found {
			t.Errorf("unwanted chat control %q remains", removed)
		}
	}
}

func TestAgentSelectionIndicatorFollowsSplit(t *testing.T) {
	withAppearancePlugins(t)
	for _, theme := range []struct {
		name, id string
		dark     bool
	}{{"light", "", false}, {"ayu-light", themeAyu, false}, {"dark", "", true}} {
		t.Run(theme.name, func(t *testing.T) {
			checkAgentSelectionInSplit(t, themeHarness(t, theme.id, theme.dark))
		})
	}
}

func checkAgentSelectionInSplit(t *testing.T, hs *harness) {
	t.Helper()
	agentViewFixture(hs)
	left := hs.a.agent
	left.messages = []*agentMessage{{role: "user", text: "Left question"}}
	hs.a.splitRight()
	right := hs.a.agent
	right.messages = []*agentMessage{{role: "user", text: "Right question"}}
	hs.settle()
	marker := locales.AgentSelectionAutomatic()
	icons := map[*agentChat]ui.Rect{}
	check := func(chat *agentChat) {
		t.Helper()
		var shown int
		for _, label := range hs.tt.Texts() {
			if label == marker {
				shown++
			}
		}
		bounds, found := hs.tt.Find(marker)
		tab, named := hs.tt.Find(agentConversationTitle(chat))
		if hs.a.agent != chat || shown != 1 || !found || !named || bounds.W != bounds.H || bounds.H <= 0 ||
			bounds.X < tab.X || bounds.X+bounds.W > tab.X+tab.W/2 || bounds.Y <= tab.Y || bounds.Y+bounds.H >= tab.Y+tab.H {
			t.Fatalf("expected one leading icon to mark the receiving chat: markers=%d icon=%v tab=%v", shown, bounds, tab)
		}
		if previous, ok := icons[chat]; ok && bounds != previous {
			t.Fatalf("switching chats moved the icon from %v to %v", previous, bounds)
		}
		icons[chat] = bounds
		for owner, slot := range icons {
			checkChatTabBadge(t, hs, slot, owner == chat)
		}
	}
	check(right)
	input := left.input.Bounds(hs.a.review.ctx)
	hs.tt.ClickAt(input.X+input.W/2, input.Y+input.H/2)
	hs.settle()
	check(left)
	hs.a.focusColumnBy(1)
	hs.settle()
	check(right)
	left.messages = append(left.messages, &agentMessage{role: "connection", text: "Left adapter log"})
	hs.settle()
	logBounds := hs.detailBounds("Left adapter log")
	if !left.scroll.Bounds(hs.a.review.ctx).Contains(logBounds.X, logBounds.Y) || hs.a.agent != right {
		t.Fatal("connection output did not appear in its own pane while the other chat was active")
	}
	leftColumn, _ := hs.a.chatTab(left)
	actions := leftColumn.tab().tool.chatActions.Bounds(hs.a.review.ctx)
	hs.tt.ClickAt(actions.X+actions.W-actions.H/2, actions.Y+actions.H/2)
	hs.settle()
	check(left)
	if hs.a.activeTab().tool.chat != left || !hs.tt.HasText(locales.AgentChatSourcesEmpty()) {
		t.Fatal("opening a background pane's conversation details did not focus that pane")
	}
}

func checkChatTabBadge(t *testing.T, hs *harness, slot ui.Rect, receiving bool) {
	t.Helper()
	frame := hs.tt.Image()
	area := image.Rect(int(slot.X), int(slot.Y), int(slot.X+slot.W), int(slot.Y+slot.H))
	badge := frame.SubImage(area).(*image.RGBA)
	paint := commonColor(frame, area.Min.X, area.Min.Y, area.Max.X, area.Max.Y)
	fill := ui.RGB(paint.R, paint.G, paint.B)
	beside := frame.RGBAAt(area.Min.X-2, (area.Min.Y+area.Max.Y)/2)
	surface := ui.RGB(beside.R, beside.G, beside.B)
	if !receiving {
		if fill != surface || agentMarkdownColorPixels(badge, hs.a.pal.muted) == 0 {
			t.Fatalf("non-receiving chat should have a muted mark on its tab, got fill %v on %v", fill, surface)
		}
		return
	}
	if ratio := contrast(fill, surface); ratio < 4.5 || agentMarkdownColorPixels(badge, fill) < area.Dx()*area.Dy()/2 {
		t.Fatalf("receiving chat lacks a solid, contrasting badge: fill %v on %v (%.2f:1)", fill, surface, ratio)
	}
	strokes := 0
	inner := area.Inset(3)
	for row := inner.Min.Y; row < inner.Max.Y; row++ {
		for col := inner.Min.X; col < inner.Max.X; col++ {
			pixel := frame.RGBAAt(col, row)
			if contrast(ui.RGB(pixel.R, pixel.G, pixel.B), fill) >= 4.5 {
				strokes++
			}
		}
	}
	if strokes < 5 {
		t.Fatalf("too few contrasting pixels inside the agent mark: %d", strokes)
	}
}

func TestAgentConnectionOutputInTranscript(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	chat := hs.a.agent
	initial := &agentMessage{role: "connection", text: "Initial connection"}
	reply := &agentMessage{role: "assistant", text: "Conversation reply"}
	chat.messages = []*agentMessage{initial, nil, reply}
	chat.draft = "Keep my draft"
	hs.settle()
	label := locales.AgentChatDetails()
	toolbarButton := hs.detailBounds(label)
	before := hs.detailBounds(reply.text)
	reconnect := &agentMessage{role: "connection", text: "Reconnected later"}
	chat.messages = append(chat.messages, reconnect)
	hs.settle()
	actions := []string{locales.AgentNewSession(), label, hs.a.withKey(locales.CmdViewSplitRight(), "view.splitRight")}
	toolbar := hs.layoutOf(actions...)
	if toolbar[label] != toolbarButton {
		t.Fatal("connection logs added or moved a toolbar control")
	}
	for index, next := range actions[1:] {
		previous := actions[index]
		left, right := toolbar[previous], toolbar[next]
		if left.W <= 0 || right.W <= 0 || left.Y != right.Y || left.X+left.W > right.X {
			t.Fatalf("expected %q before %q on the chat toolbar, got %v and %v", previous, next, left, right)
		}
	}
	checkTranscript := func() {
		t.Helper()
		view := chat.scroll.Bounds(hs.a.review.ctx)
		previousBottom := view.Y
		counts := make(map[string]int)
		for _, label := range hs.tt.Texts() {
			counts[label]++
		}
		for _, message := range []*agentMessage{initial, reply, reconnect} {
			bounds := hs.detailBounds(message.text)
			if bounds.Y < previousBottom || bounds.Y+bounds.H > view.Y+view.H || counts[message.text] != 1 {
				t.Fatalf("expected one %q in transcript order, got bounds %v in %v", message.text, bounds, view)
			}
			previousBottom = bounds.Y + bounds.H
		}
		if hs.detailBounds(reply.text) != before || chat.draft != "Keep my draft" {
			t.Fatal("inspecting conversation details moved the reply or changed the draft")
		}
	}
	checkTranscript()
	hs.clickDetail(label)
	if !hs.tt.HasText(locales.AgentChatSourcesEmpty()) {
		t.Fatal("opening conversation details did not show sources")
	}
	checkTranscript()
	hs.clickDetail(locales.AgentHideChatDetails())
	checkTranscript()
}

func TestAgentToolbarInDetachedChat(t *testing.T) {
	src := newHarness(t, false)
	agentViewFixture(src)
	chat := src.a.agent
	chat.messages = []*agentMessage{{role: "connection", text: "Detached adapter log"}}
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	dst.settle()
	mark, marked := dst.tt.Find(locales.AgentSelectionAutomatic())
	title, named := dst.tt.Find(dst.a.windowTitle())
	button, found := dst.tt.Find(locales.AgentChatDetails())
	back, canReturn := dst.tt.Find(locales.MoveToMainWindow())
	if !marked || !named || !found || !canReturn || mark.W != mark.H || mark.X+mark.W > title.X || mark.Y+mark.H > titleBarH || button.X+button.W > back.X || button.Y != back.Y || dst.tt.HasText(locales.AgentNewSession()) {
		t.Fatalf("detached chat controls do not fit its title bar: icon=%v title=%v logs=%v return=%v", mark, title, button, back)
	}
	checkChatTabBadge(t, dst, mark, true)
	status, found := dst.tt.Find(locales.AgentTaskIdle())
	if !found || status.X < title.X+title.W || status.X+status.W > button.X {
		t.Fatalf("detached title does not leave room for its task status: %v %v %v", title, status, button)
	}
	dst.tt.Move(mark.X+mark.W/2, mark.Y+mark.H/2)
	dst.tt.Advance(time.Second)
	dst.tt.Frame()
	if !dst.tt.HasText(locales.ReturnTabHint() + "\n" + locales.AgentSelectionAutomatic() + "\n" + locales.AgentTaskIdle()) {
		t.Fatal("the detached icon lost its return hint, selection hint or task status")
	}
	if !dst.tt.HasText(chat.messages[0].text) {
		t.Fatal("detaching the chat hid its connection output")
	}
	if err := dst.tt.Click(locales.AgentChatDetails()); err != nil {
		t.Fatal(err)
	}
	dst.settle()
	if !dst.tt.HasText(locales.AgentChatSourcesEmpty()) {
		t.Fatal("cannot open conversation details after detaching a chat")
	}
	dst.key(0, ui.KeyEscape)
	if err := dst.tt.Click(locales.MoveToMainWindow()); err != nil {
		t.Fatal(err)
	}
	src.settle()
	if tab := src.a.activeTab(); tab == nil || tab.tool == nil || tab.tool.chat != chat || !src.tt.HasText(locales.AgentSelectionAutomatic()) || !src.tt.HasText(locales.AgentChatDetails()) || !src.tt.HasText(chat.messages[0].text) {
		t.Fatal("returning a chat lost its toolbar or connection output")
	}
}

func TestAgentVisibleTextPreservesUTF8(t *testing.T) {
	text := strings.Repeat("代码", 20)
	for _, size := range []int{1, 2, 3, 10, len(text), len(text) + 1} {
		got, clipped := agentVisibleText(text, size)
		if !utf8.ValidString(got) || len(got) > size || !strings.HasPrefix(text, got) || clipped != (len(text) > size) {
			t.Fatalf("text limit %d returned %q, clipped %v", size, got, clipped)
		}
	}
}

func TestAgentViewStreamKeepsReadingPosition(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	r := hs.a.agent
	message := &agentMessage{role: "assistant", text: strings.Repeat("A paragraph about the project.\n\n", 40)}
	r.messages = []*agentMessage{message}
	hs.settle()
	if r.scroll.MaxY <= 0 || r.scroll.Y < r.scroll.MaxY-1 {
		t.Fatalf("new output did not reveal its end: %v/%v", r.scroll.Y, r.scroll.MaxY)
	}
	input, ok := hs.tt.Find(locales.AgentDraft())
	if !ok {
		t.Fatal("no workspace composer")
	}
	hs.tt.Scroll(input.X+input.W/2, input.Y-80, 0, -200)
	hs.settle()
	before := r.scroll.Y
	if before >= r.scroll.MaxY-1 {
		t.Fatal("the transcript cannot scroll back")
	}
	message.text += "Another streamed paragraph.\n\n"
	message.render = viewNode{Text: message.text}
	hs.settle()
	if r.scroll.Y != before {
		t.Fatalf("streaming moved the reader from %v to %v", before, r.scroll.Y)
	}
	r.scroll.Y = r.scroll.MaxY
	hs.settle()
	message.text += strings.Repeat("More output.\n\n", 6)
	message.render = viewNode{Text: message.text}
	hs.settle()
	if r.scroll.Y < r.scroll.MaxY-1 {
		t.Fatalf("streaming no longer follows the end: %v/%v", r.scroll.Y, r.scroll.MaxY)
	}
}

func TestAgentViewContextLoading(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	r := hs.a.agent
	r.contextLoading, r.draft = 1, "Keep my draft"
	hs.settle()
	if !hs.tt.HasText(locales.AgentLoadingContext()) {
		t.Fatal("loading a file has no visible feedback")
	}
	if err := hs.tt.Click(locales.AgentSend()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if r.client != nil || r.connecting || r.draft != "Keep my draft" {
		t.Fatal("send ran while file context was still loading")
	}
}

func TestAgentPlanPanelKeepsProgressInView(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 180)
	chat := hs.a.agent
	chat.sessionID, chat.busy = "plan-session", true
	chat.messages = []*agentMessage{{role: "assistant", text: strings.Repeat("A paragraph about the current project.\n\n", 40)}}
	hs.settle()
	label := locales.AgentPlan()
	if hs.tt.HasText(label) || chat.scroll.MaxY == 0 || chat.scroll.Y != chat.scroll.MaxY {
		t.Fatal("the empty plan should stay hidden while the transcript follows new output")
	}

	entries := make([]map[string]string, 18)
	for index := range entries {
		entries[index] = map[string]string{"content": fmt.Sprintf("Check requirement %02d", index+1), "priority": "medium", "status": "pending"}
	}
	update := func(entries []map[string]string) {
		agentOutputUpdate(t, hs, map[string]any{"sessionUpdate": "plan", "entries": entries})
	}
	update(entries)
	header, headerFound := hs.tt.Find(label)
	first, firstFound := hs.tt.Find(entries[0]["content"])
	input, inputFound := hs.tt.Find(locales.AgentDraft())
	last := entries[len(entries)-1]["content"]
	lastBounds, _ := hs.tt.Find(last)
	if !headerFound || !firstFound || !inputFound || header.Y+header.H > first.Y || first.Y+first.H > input.Y || lastBounds.H > 0 {
		t.Fatalf("plan and input should fit above the fold: panel=%+v first=%+v input=%+v", header, first, input)
	}
	if chat.scroll.Y != chat.scroll.MaxY {
		t.Fatal("the arriving plan displaced the end of the transcript")
	}
	if err := hs.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	hs.key(ui.Shift, ui.KeyTab)
	if !hs.tt.Focused(entries[0]["content"]) {
		t.Fatal("the task list cannot receive keyboard focus")
	}
	hs.key(0, ui.KeyEnd)
	hs.settle()
	lastBounds, _ = hs.tt.Find(last)
	if lastBounds.H <= 0 || lastBounds.Y+lastBounds.H > input.Y || chat.scroll.Y != chat.scroll.MaxY {
		t.Fatal("scrolling the task list did not reveal its last entry independently")
	}

	chat.scroll.Y = 0
	hs.settle()
	if err := hs.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	update([]map[string]string{{"content": "Replacement requirement", "priority": "medium", "status": "completed"}})
	if chat.scroll.Y != 0 || !hs.tt.HasText("1/1") || hs.tt.HasText("Replacement requirement") {
		t.Fatal("an update while collapsed lost the progress, disclosure state or transcript position")
	}
	if err := hs.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText("Replacement requirement") || hs.tt.HasText(last) || chat.scroll.Y != 0 {
		t.Fatal("reopening did not reveal the current plan at the same transcript position")
	}
	update([]map[string]string{})
	if hs.tt.HasText(label) || chat.scroll.Y != 0 {
		t.Fatal("an empty plan update did not clear the panel at the same transcript position")
	}
}

func TestAgentPlanPanelFitsShortDetachedChat(t *testing.T) {
	src := newHarness(t, false)
	agentViewFixture(src)
	chat := src.a.agent
	chat.sessionID, chat.busy = "detached-plan", true
	chat.messages = []*agentMessage{{role: "assistant", text: "Checking results."}}
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	dst.tt.SetSize(320, 220)
	dst.settle()
	stopBefore, found := dst.tt.Find(locales.AgentStop())
	workingBefore, _ := dst.tt.Find(locales.AgentWorking())
	if !found || stopBefore.H == 0 || workingBefore.H == 0 {
		t.Fatal("the short chat has no stop button or progress line")
	}
	entries := make([]map[string]string, 12)
	for index := range entries {
		entries[index] = map[string]string{"content": fmt.Sprintf("Verify section %02d", index+1), "priority": "medium", "status": "pending"}
	}
	agentOutputUpdate(t, dst, map[string]any{"sessionUpdate": "plan", "entries": entries})
	label := locales.AgentPlan()
	header, _ := dst.tt.Find(label)
	working, _ := dst.tt.Find(locales.AgentWorking())
	if header.H == 0 || working.H != workingBefore.H || working.Y+working.H > header.Y || dst.tt.HasText(entries[0]["content"]) {
		t.Fatalf("the short chat should fold its plan and keep the live transcript visible: header=%+v working=%+v", header, working)
	}
	if err := dst.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	dst.settle()
	stopAfter, _ := dst.tt.Find(locales.AgentStop())
	first, _ := dst.tt.Find(entries[0]["content"])
	if stopAfter != stopBefore || first.H == 0 {
		t.Fatalf("the expanded plan displaced chat controls or clipped its first entry: stop=%+v (was %+v), first=%+v", stopAfter, stopBefore, first)
	}
}

func TestShotAgentView(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	for _, state := range []struct {
		name, locale string
		dark         bool
		width        float32
		permission   bool
	}{
		{name: "agent-dark", locale: "en", dark: true, width: 340},
		{name: "agent-light-zh", locale: "zh-Hans", width: 340},
		{name: "agent-permission", locale: "en", dark: true, width: 340, permission: true},
		{name: "agent-narrow-zh", locale: "zh-Hans", width: 180, permission: true},
	} {
		t.Run(state.name, func(t *testing.T) {
			locale := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return state.locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return locale }) })
			hs := newHarness(t, state.dark)
			hs.a.openFile(hs.path("main.go"), true)
			hs.settle()
			agentViewFixture(hs)
			sizeChatWorkspace(hs, state.width)
			r := hs.a.agent
			r.sessionID = "preview"
			r.messages = []*agentMessage{
				{role: "user", text: "Explain this function and review the greeting."},
				{role: "assistant", text: "The entry point writes a greeting to standard output.\n\n```go\nfmt.Println(\"hello, fiveleaf\")\n```\n\nThere are no arguments or error paths in this example."},
			}
			r.plan = []agentPlan{
				{content: "Read main.go", status: "completed"},
				{content: "Review the greeting and its output", status: "in_progress"},
				{content: "Run focused tests", status: "pending"},
			}
			r.tools = []*agentTool{{id: "edit", title: "Update main.go", status: "completed", changes: []agentChange{{path: hs.path("main.go"), existed: true, oldText: "hello", newText: "hello, fiveleaf"}}}}
			if state.permission {
				r.permissions = []*agentPermission{{
					title:   "Run focused Go checks",
					detail:  "cmd/fiveleaf/agentview.go:23\n{\n  \"command\": \"go test ./cmd/fiveleaf -run TestAgentView\",\n  \"cwd\": \"/workspace/fiveleaf\"\n}",
					options: []acp.PermissionOption{{OptionId: "once", Kind: "allow_once", Name: "Allow once"}, {OptionId: "reject", Kind: "reject_once", Name: "Reject"}},
					choose:  func(string) {},
				}}
			}
			hs.settle()
			hs.shot(state.name)
		})
	}
}

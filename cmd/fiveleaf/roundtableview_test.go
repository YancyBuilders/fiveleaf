package main

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

type roundtableHarness struct {
	*harness
	w  *roundtableWindow
	tt *ui.Tester
}

func newRoundtableHarness(t *testing.T, adapter roundtable.Adapter) *roundtableHarness {
	t.Helper()
	hs := agentRosterHarness(t)
	a := hs.a
	if adapter == nil {
		adapter = roundtable.AdapterFunc(func(_ context.Context, messages []roundtable.Message) (roundtable.Message, error) {
			return roundtable.Message{Text: "Reply: " + messages[len(messages)-1].Text}, nil
		})
	}
	m := testAgentMeeting(t, a)
	for _, speaker := range m.speakers {
		if speaker.adapter != nil {
			speaker.adapter.backend = adapter
		}
	}
	w := newRoundtableWindow(a, m)
	a.meetingWindow = w
	t.Cleanup(w.finish)
	h := &roundtableHarness{harness: hs, w: w}
	h.tt = ui.NewTester(meetingTestView(w), 1040, 780)
	h.tt.SetDark(true)
	h.frames()
	return h
}

func meetingTestView(w *roundtableWindow) func(*ui.Context) {
	var fade paletteFade
	return func(c *ui.Context) {
		w.a.pal = applyTheme(c, &w.a.settings.Prefs, &fade)
		w.a.drain()
		w.render(c)
	}
}

func (h *roundtableHarness) frames() {
	for range 20 {
		step(h.tt)
	}
}

func (h *roundtableHarness) until(t *testing.T, description string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !done() && time.Now().Before(deadline) {
		h.frames()
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if !done() {
		t.Fatal("timed out waiting for " + description)
	}
	h.frames()
}

func TestRoundtableViewSendsWithoutChangingSingleChat(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	a := h.a
	a.agent.draft = "Keep the IDE draft"
	mainServices := a.services
	if err := h.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("Review this function")
	if err := h.tt.Click(locales.AgentSend()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	h.until(t, "a conclusion", func() bool { return !h.w.meeting.room.Snapshot().Busy })
	if !h.tt.HasText("Reply: Review this function") || !h.tt.HasText(locales.RoundtableConclusion()) || h.tt.HasText(locales.RoundtableHop(1)) || h.w.draft != "" {
		t.Fatal("the meeting overview must show the conclusion and keep contributions in member history")
	}
	if err := h.tt.Click(locales.AgentRoleRoundtableModerator()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.tt.HasText(locales.RoundtableMemberTasks(locales.AgentRoleRoundtableModerator())) || !h.tt.HasText(locales.RoundtableHop(1)) || len(h.w.meeting.room.Snapshot().Messages) != 2 {
		t.Fatal("the moderator's immediate conclusion must be recorded as its own task", h.tt.Texts())
	}
	if a.agent.draft != "Keep the IDE draft" || len(a.agent.messages) != 0 || a.services != mainServices {
		t.Fatal("the independent window changed the IDE chat or stole its UI services")
	}
	if err := h.tt.Click(locales.RoundtableCollaboration()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if err := h.tt.Click(locales.AgentClearChat()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if len(h.w.meeting.room.Snapshot().Messages) == 0 || !h.tt.HasText(locales.RoundtableClearConfirm()) {
		t.Fatal("clear discarded saved history before confirmation")
	}
	if err := h.tt.Click(locales.LogClear()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	h.until(t, "the saved history clear", func() bool { return h.w.meeting.changing == nil })
	if len(h.w.meeting.room.Snapshot().Messages) != 0 || !h.tt.HasText(locales.RoundtableEmpty()) {
		t.Fatal("Clear did not reset the roundtable transcript")
	}
}

func TestRoundtablePendingPublicationAllowsCloseAndReopen(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	meeting := h.w.meeting
	if err := meeting.room.SetHumanChair(true); err != nil {
		t.Fatal(err)
	}
	const conclusion = "Keep the accepted decision."
	h.w.draft = conclusion
	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	safety := time.AfterFunc(3*time.Second, unblock)
	defer safety.Stop()
	defer unblock()
	completed := make(chan error, 1)
	h.w.changeMeeting(meetingPublish, func(room *roundtable.Room) error {
		close(started)
		<-release
		return room.PublishConclusionWithDeferredMemory(conclusion)
	}, func(_ *agentMeeting, err error) { completed <- err })
	h.until(t, "the blocked publication", func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	})
	if err := h.tt.Click(locales.RoundtableGraphView()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.view != roundtableViewGraph || h.w.draft != conclusion || meeting.changing == nil {
		t.Fatal("the pending publication blocked navigation or discarded the draft")
	}
	if err := h.tt.Click(locales.RoundtableChatView()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if err := h.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	h.tt.Type(" Follow up")
	h.frames()
	if !strings.Contains(h.w.draft, "Follow up") {
		t.Fatal("publishing froze edits to the next draft")
	}
	h.a.closeRoundtable()
	h.a.openMeetingRecord(meeting.library, meeting.group.ID)
	if h.a.meeting != nil || len(pendingMeetingCloses(meeting.key())) != 1 {
		t.Fatal("reopening read the archive before the accepted publication finished")
	}
	select {
	case <-release:
		t.Fatal("the UI waited for the blocked publication")
	default:
	}
	unblock()
	h.harness.until("the same meeting reopened after the close", func() bool {
		return h.a.meeting != nil && h.a.meeting != meeting && h.a.meeting.key() == meeting.key() && len(completed) != 0
	})
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	messages := h.a.meeting.room.Snapshot().Messages
	if len(messages) != 1 || messages[0].Text != conclusion || !messages[0].HumanConclusion {
		t.Fatalf("reopened archive did not contain exactly one accepted conclusion: %v", messages)
	}
}

func TestRoundtablePendingMemoryKeepsComposerAvailable(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	meeting := h.w.meeting
	if err := meeting.room.SetHumanChair(true); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	h.w.changeMemory(func(room *roundtable.Room) error {
		<-release
		return room.AddMemory("A saved note", "")
	}, nil)
	h.until(t, "pending memory update", func() bool { return meeting.changing != nil })
	if err := h.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	const remark = "A remark while saving"
	h.tt.Type(remark)
	h.frames()
	if err := h.tt.Click(locales.AgentSend()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	messages := meeting.room.Snapshot().Messages
	if meeting.changing == nil || len(messages) != 1 || messages[0].Text != remark || h.w.draft != "" {
		t.Fatal("the memory update prevented sending the remark")
	}
	unblock()
	h.until(t, "saved memory", func() bool { return meeting.changing == nil })
	if notes := meeting.room.Memory().Notes; len(notes) != 1 || notes[0].Text != "A saved note" {
		t.Fatalf("sending changed the pending update: %v", notes)
	}
}

func TestRoundtableViewBusyKeepsCompletedReplies(t *testing.T) {
	for _, tc := range []struct{ name, text, visible, code string }{
		{name: "pending-table", text: "Finished reply.\n\n| A completed standalone line.", visible: "A completed standalone line."},
		{name: "eof-code", text: "```go\nfunc main() {}\n", visible: "func main() {}", code: "func main() {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs := newHarness(t, true)
			w := &roundtableWindow{a: hs.a, pal: hs.a.pal, meeting: &agentMeeting{chat: &agentChat{}}}
			// Snapshot messages have already completed, even while another
			// member of the roundtable is preparing its response.
			snapshot := roundtable.Snapshot{Busy: true, Messages: []roundtable.Message{{ID: "reply", Role: "assistant", Name: "Alpha", Text: tc.text}}}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Children(func() { w.transcript(c, snapshot, roundtable.BuildGraph(snapshot)) })
			}, 540, 600)
			tt.SetLocale("en")
			if !tt.HasText(tc.visible) {
				t.Fatalf("waiting for the next member hid completed output: %q", tt.Texts())
			}
			if tc.code != "" {
				if err := tt.Click("Copy code"); err != nil {
					t.Fatal(err)
				}
				if tt.Clipboard() != tc.code {
					t.Fatal("waiting for another member disabled copying completed code")
				}
			}
		})
	}
}

func TestRoundtableLiveTurnAtSnapshotBoundary(t *testing.T) {
	messages := []roundtable.Message{
		pageTask("dispatch", "question", roundtable.Coordinate, 1),
		pageTask("finding", "question", roundtable.Work, 2),
		pageTask("closing", "question", roundtable.Conclusion, 3),
	}
	messages = append(messages, pageQuestions("later", roundtable.MaxHistoryMessages-1)...)
	room, archive := openRoundtablePageArchive(t, messages)
	snapshot := room.Snapshot()
	if !snapshot.HasEarlier || snapshot.Messages[0].ID != "closing" {
		t.Fatal("the retained snapshot should begin at the conclusion")
	}
	hs := newHarness(t, true)
	window := &roundtableWindow{a: hs.a, meeting: &agentMeeting{room: room}}
	files, err := filepath.Glob(filepath.Join(archive, "*-finding.json"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	var prior []roundtable.Message
	tester := ui.NewTester(func(c *ui.Context) {
		window.a.drain()
		prior = window.liveTurnPrefix(c, ui.Column(c), snapshot)
	}, 320, 200)
	deadline := time.Now().Add(3 * time.Second)
	for window.err == "" && time.Now().Before(deadline) {
		step(tester)
		runtime.Gosched()
	}
	if window.err == "" {
		t.Fatal("the failed archive read was not reported")
	}
	if err := os.WriteFile(files[0], content, 0600); err != nil {
		t.Fatal(err)
	}
	tester.Advance(time.Second)
	until := time.Now().Add(3 * time.Second)
	for len(prior) == 0 && time.Now().Before(until) {
		step(tester)
		runtime.Gosched()
	}
	if window.err != "" || len(prior) != 2 || prior[0].ID != "dispatch" || prior[1].ID != "finding" {
		t.Fatal("the latest page lost the beginning of its first turn", prior)
	}
}

func TestRoundtableViewHumanInputInterrupts(t *testing.T) {
	started, canceled := make(chan struct{}, 1), make(chan struct{}, 1)
	adapter := roundtable.AdapterFunc(func(ctx context.Context, messages []roundtable.Message) (roundtable.Message, error) {
		if messages[len(messages)-1].Text == "first" {
			started <- struct{}{}
			<-ctx.Done()
			canceled <- struct{}{}
			return roundtable.Message{}, ctx.Err()
		}
		return roundtable.Message{Text: "new reply"}, nil
	})
	h := newRoundtableHarness(t, adapter)
	h.w.draft = "first"
	h.w.send()
	h.until(t, "first call", func() bool { return len(started) > 0 })
	if !h.tt.HasText(locales.AgentStop()) {
		t.Fatal("an active roundtable has no Stop action")
	}
	if err := h.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("second")
	if err := h.tt.Click(locales.AgentSend()); err != nil {
		t.Fatal(err)
	}
	h.until(t, "interruption and replacement reply", func() bool {
		return len(canceled) > 0 && len(h.w.meeting.room.Snapshot().Messages) >= 3
	})
	if !h.tt.HasText("new reply") || h.w.draft != "" {
		t.Fatal("the composer was disabled while a reply was running")
	}
}

func TestRoundtableViewPermissionsUseMeetingState(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	chosen := ""
	if err := h.w.meeting.room.Discuss("Review", "Read the fixture", "Alpha"); err != nil {
		t.Fatal(err)
	}
	h.until(t, "assignment", func() bool { return !h.w.meeting.room.Snapshot().Busy })
	message := h.w.meeting.room.Snapshot().Messages[1]
	h.w.meeting.chatTask = message.ID
	h.w.meeting.chat.permissions = []*agentPermission{{title: "Roundtable edit", detail: "main.go:1", options: []acp.PermissionOption{{OptionId: "yes", Kind: "allow_once", Name: "Allow once"}}, choose: func(id string) { chosen = id }}}
	h.w.meeting.chat.tools = []*agentTool{{title: "Read main.go", status: "completed", text: "roundtable tool output"}}
	h.frames()
	if !h.tt.HasText("Roundtable edit") || h.tt.HasText("roundtable tool output") || !h.tt.HasText("main.go:1") {
		t.Fatal("the roundtable did not keep its tool collapsed and permission details visible")
	}
	if h.tt.HasText("Read main.go") {
		t.Fatal("tool details leaked into the overview")
	}
	if err := h.tt.Click(locales.RoundtableGraphView()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if _, found := h.tt.Find("Allow once"); found || h.tt.HasText("Roundtable edit") || chosen != "" {
		t.Fatal("graph exposed or consumed the pending approval")
	}
	if err := h.tt.Click(locales.RoundtableChatView()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if err := h.tt.Click("Alpha"); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.expandedTask != message.ID || !h.tt.HasText(locales.RoundtableTaskTools()) {
		t.Fatal("the conversation entry did not open the recorded task")
	}
	if err := h.tt.Click(locales.AgentCopyToolOutput() + ": Read main.go"); err != nil {
		t.Fatal(err)
	}
	if h.tt.Clipboard() != "roundtable tool output" || h.harness.tt.Clipboard() != "" {
		t.Fatal("copying a roundtable tool did not use the meeting window's clipboard")
	}
	if err := h.tt.Click("Read main.go"); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.tt.HasText("roundtable tool output") {
		t.Fatal("the roundtable tool disclosure did not reveal its output")
	}
	if err := h.tt.Click("Allow once"); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if chosen != "yes" || len(h.a.agent.permissions) != 0 || len(h.a.agent.tools) != 0 {
		t.Fatal("the meeting approval changed the selected single chat")
	}
}

func TestRoundtableMenuAndClose(t *testing.T) {
	h := agentRosterHarness(t)
	a := h.a
	a.openFile(h.path("main.go"), true)
	h.settle()
	before := a.activeEditor()
	meeting := testAgentMeeting(t, a)
	a.showRoundtable(meeting)
	w := a.activeRoundtableTab()
	a.closeActive()
	h.settle()
	if a.activeEditor() != before || a.meetingWindow != nil || a.meeting != nil {
		t.Fatal("closing the meeting tab did not restore the editor and release its runtime")
	}
	if err := w.meeting.room.Send("after close", ""); !errors.Is(err, roundtable.ErrClosed) {
		t.Fatalf("closing left its scheduler accepting work: %v", err)
	}
	w.post(func() { t.Error("closed meeting accepted a late UI callback") })
	h.settle()
}

func TestRoundtableReadingControlsAndNarrowLayout(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.tt.SetSize(640, 480)
	h.frames()
	if _, found := h.tt.Find(locales.AgentConversationSettings()); found || h.tt.HasText(locales.AgentSystemPrompt()) {
		t.Fatal("meeting still exposes the retired prompt editor")
	}
	for _, label := range []string{locales.AgentDraft(), locales.AgentSend(), locales.RoundtableMemberPanel(), locales.RoundtableExport()} {
		bounds, found := h.tt.Find(label)
		if !found || bounds.X < 0 || bounds.X+bounds.W > 641 || bounds.Y < 0 || bounds.Y+bounds.H > 481 {
			t.Errorf("narrow roundtable control %q is out of bounds: %+v", label, bounds)
		}
	}
}

func TestRoundtableSendErrorKeepsDraft(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.w.meeting.room.SetMembers(nil)
	h.w.draft = "Keep this question"
	h.w.send()
	h.frames()
	if h.w.draft != "Keep this question" || h.w.err == "" || !h.tt.HasText(h.w.err) {
		t.Fatal("a rejected send lost its draft or hid the error")
	}
}

func TestRoundtableUnavailableTargetKeepsDirectedDraft(t *testing.T) {
	for _, removed := range []bool{false, true} {
		name := "disabled"
		if removed {
			name = "removed"
		}
		t.Run(name, func(t *testing.T) {
			calls := make(chan string, 1)
			adapter := roundtable.AdapterFunc(func(_ context.Context, messages []roundtable.Message) (roundtable.Message, error) {
				calls <- messages[len(messages)-1].Text
				return roundtable.Message{Text: "Beta reply"}, nil
			})
			h := newRoundtableHarness(t, adapter)
			h.mentionMember(t, "Alpha")
			h.tt.Type("A question only for Alpha")
			h.frames()
			if removed {
				delete(h.a.settings.AgentServers, "alpha")
			} else {
				server := h.a.settings.AgentServers["alpha"]
				server.Disabled = true
				h.a.settings.AgentServers["alpha"] = server
			}
			h.a.checkAgentConfig()
			h.frames()
			if err := h.tt.Click(locales.AgentSend()); err != nil {
				t.Fatal(err)
			}
			if err := h.tt.Click(locales.AgentDraft()); err != nil {
				t.Fatal(err)
			}
			mods := ui.Ctrl
			if runtime.GOOS == "darwin" {
				mods = ui.Cmd
			}
			h.tt.Key(mods, ui.KeyEnter)
			h.frames()
			if h.w.target != "agent:alpha" || h.w.draft != "@Alpha A question only for Alpha" || len(calls) != 0 || len(h.w.meeting.room.Snapshot().Messages) != 0 {
				t.Fatal("an unavailable directed target lost its draft or silently sent to another member")
			}
			if !h.tt.HasText(locales.RoundtableTargetUnavailable()) {
				t.Fatal("the unavailable target has no recovery hint")
			}
			h.tt.Command("selectAll")
			h.tt.Type("A question")
			h.frames()
			if !h.tt.HasText(locales.RoundtableModeratorUnavailable()) {
				t.Fatal("a moderator with an unavailable saved binding needs a recovery hint")
			}
			h.mentionMember(t, "Beta")
			if err := h.tt.Click(locales.AgentSend()); err != nil {
				t.Fatal(err)
			}
			h.until(t, "explicitly rerouted reply", func() bool { return len(h.w.meeting.room.Snapshot().Messages) == 2 })
			if h.w.target != "" || h.w.draft != "" || len(calls) != 1 {
				t.Fatal("explicitly selecting an available member did not resume sending")
			}
		})
	}
}

func TestRoundtableNativeEditMenuUsesFocusedInput(t *testing.T) {
	h := agentRosterHarness(t)
	h.a.openFile(h.path("main.go"), true)
	h.settle()
	ed := h.a.activeEditor()
	before := ed.buf.String()
	h.a.showRoundtable(testAgentMeeting(t, h.a))
	h.settle()
	w := h.a.activeRoundtableTab()
	menu := h.a.menu(false, func(id string, _ *mygo.Window) {
		t.Errorf("native edit action reached the editor command %s", id)
	})
	roles := map[mygo.MenuRole]bool{}
	for _, section := range menu.Items() {
		if section.Label == locales.MenuEdit() {
			for _, item := range section.Submenu {
				switch item.Role {
				case mygo.RoleUndo, mygo.RoleRedo, mygo.RoleCut, mygo.RoleCopy, mygo.RolePaste, mygo.RoleSelectAll:
					roles[item.Role] = true
					if item.Click != nil {
						t.Fatalf("native edit role %s also invokes an editor callback", item.Role)
					}
				}
			}
		}
	}
	for _, role := range []mygo.MenuRole{mygo.RoleUndo, mygo.RoleRedo, mygo.RoleCut, mygo.RoleCopy, mygo.RolePaste, mygo.RoleSelectAll} {
		if !roles[role] {
			t.Fatalf("the roundtable menu has no native %s role", role)
		}
	}
	// MyGo sends native edit roles as commands to the focused UI surface.
	if err := h.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("Roundtable draft")
	h.tt.Command("undo")
	h.settle()
	if w.draft != "" {
		t.Fatal("native Undo did not reach the roundtable input")
	}
	h.tt.Command("redo")
	h.settle()
	if w.draft != "Roundtable draft" {
		t.Fatal("native Redo did not restore the roundtable input")
	}
	h.tt.Command("selectAll")
	h.tt.Command("cut")
	h.settle()
	if w.draft != "" || ed.buf.String() != before {
		t.Fatal("native Select All/Cut changed the background editor or missed the input")
	}
}

func TestShotRoundtable(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	for _, state := range []struct {
		name, locale string
		width        int
		dark         bool
	}{
		{"roundtable-dark", "en", 1040, true},
		{"roundtable-zh", "zh-Hans", 640, false},
		{"roundtable-ja", "ja", 640, true},
	} {
		t.Run(state.name, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return state.locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			adapter := roundtable.AdapterFunc(func(_ context.Context, _ []roundtable.Message) (roundtable.Message, error) {
				return roundtable.Message{Text: "## Review\n\nThe entry point preserves the greeting.\n\n```go\nfmt.Println(\"hello, fiveleaf\")\n```\n\nThe focused checks passed."}, nil
			})
			h := newRoundtableHarness(t, adapter)
			h.tt.SetSize(state.width, 820)
			h.tt.SetDark(state.dark)
			h.w.draft = "@Alpha review the current changes."
			h.w.send()
			h.until(t, "preview reply", func() bool { return len(h.w.meeting.room.Snapshot().Messages) >= 2 })
			h.w.draft = "Explain the tradeoff and suggest a focused check."
			h.frames()
			f, err := os.Create(filepath.Join(os.Getenv("SHOTS"), state.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, h.tt.Image()); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(h.w.meeting.room.Snapshot().Messages[1].Text, "focused checks") {
				t.Fatal("screenshot lacks its real scheduler reply")
			}
		})
	}
}

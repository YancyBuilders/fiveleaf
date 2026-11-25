package main

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestMeetingInspectorInvitesAndReturnsToChatInNarrowPane(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.tt.SetSize(605, 620)
	h.frames()
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	if _, ok := h.tt.Find(locales.RoundtableMembersHeading()); ok {
		t.Fatal("narrow chat should start with its inspector folded")
	}
	click(locales.RoundtableMemberPanel())
	click(locales.RoundtableInvite())
	click(locales.RoundtableBackToMembers())
	click(locales.RoundtableInvite())
	if !h.tt.Focused(locales.RoundtableFindMembers()) {
		t.Fatal("the invitation filter did not receive focus")
	}
	h.tt.Type(locales.AgentRoleTester())
	h.frames()
	click(locales.RoundtableAddMember(locales.AgentRoleTester()))
	meeting := h.w.meeting
	saved, err := meeting.library.Group(meeting.group.ID)
	if err != nil || !slices.ContainsFunc(saved.Members, func(ref roundtable.Participant) bool { return ref.ID == "preset:test" }) {
		t.Fatal("missing saved invitation", err, saved.Members)
	}
	click(locales.AgentRoleTester())
	if h.w.expandedTask != "" || !h.tt.HasText(locales.RoundtableMemberTasks(locales.AgentRoleTester())) || !h.tt.HasText(locales.RoundtableNoTasks()) {
		t.Fatal("invited member has no task history")
	}
	click(locales.RoundtableBackToMembers())
	click(locales.RoundtableGraphView())
	click(locales.RoundtableMemberPanel())
	click(locales.AgentRoleTester())
	click(locales.RoundtableAskMember())
	if h.w.view != roundtableViewChat || h.w.target != "preset:test" || !h.tt.Focused(locales.AgentDraft()) {
		t.Fatal("direct question did not return to the editor composer")
	}
	click(locales.RoundtableCollaboration())
	click(locales.RoundtableExpandReplies())
	if !h.w.showFullReplies {
		t.Fatal("reply preference did not change")
	}
	click(locales.AgentClearChat())
	if !h.tt.HasText(locales.RoundtableClearConfirm()) {
		t.Fatal("the collaboration drawer did not ask for confirmation")
	}
	h.tt.Key(0, ui.KeyEscape)
	h.frames()
	click(locales.RoundtableMemberPanel())
	if h.tt.HasText(locales.RoundtableClearConfirm()) || h.tt.HasText(locales.AgentClearChat()) {
		t.Fatal("closing the drawer left its destructive action in the inspector")
	}
}

func TestMeetingInspectorCardsStatsAndHistory(t *testing.T) {
	room := roundtable.New(nil, roundtable.Options{})
	t.Cleanup(room.Close)
	const analyst = "preset:requirements"
	roster := []meetingSpeaker{
		{Participant: roundtable.Participant{ID: roundtable.ModeratorID, Name: "Chair"}},
		{Participant: roundtable.Participant{ID: analyst, Name: "Linh"}, provider: agentProvider{name: "Provider A"}},
		{Participant: roundtable.Participant{ID: "agent:offline", Name: "Remy"}, problem: locales.AgentDisabled()},
	}
	roster[1].role.Model = "Model B"
	w := &roundtableWindow{a: &app{}, pal: lightPalette().derive(), meeting: &agentMeeting{room: room, speakers: roster, chat: &agentChat{}}}
	var snapshot roundtable.Snapshot
	var panelWidth float32
	tester := ui.NewTester(func(c *ui.Context) {
		width, _ := c.Size()
		ui.Row(c).Fill().Children(func() {
			panelWidth = width - ui.Spacer(c).LayoutBounds().W
			w.memberInspector(c, snapshot, roundtable.BuildGraph(snapshot), width, true)
		})
	}, 1240, 800)
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		tester.Frame()
	}
	stat := func(label, want string) {
		t.Helper()
		texts := tester.Texts()
		pos := slices.Index(texts, label)
		if pos < 1 || texts[pos-1] != want {
			t.Fatalf("want %q above %q in %v", want, label, texts)
		}
	}
	tester.Frame()
	sections := []string{locales.RoundtableMembersHeading(), locales.RoundtableInvite(), locales.RoundtableMemberStatus(), locales.RoundtableGroupStats()}
	last := -1
	for _, label := range sections {
		index := slices.Index(tester.Texts(), label)
		if index <= last {
			t.Fatalf("missing or misplaced %q in %v", label, tester.Texts())
		}
		last = index
	}
	for _, label := range []string{locales.RoundtableMemoryTab(), locales.RoundtableCollaboration(), locales.RoundtableEvidenceSources()} {
		if _, found := tester.Find(label); found {
			t.Fatalf("unexpected inspector section %q", label)
		}
	}
	stat(locales.RoundtableGraphNodes(), "0")
	stat(locales.RoundtableGraphHandovers(), "0")
	stat(locales.RoundtablePinnedCountLabel(), "0")
	stat(locales.RoundtableGraphDuration(), "—")
	if panelWidth != 224 || !tester.HasText(locales.RoundtableStandby()) {
		t.Fatalf("empty inspector at 1240: width %v, %v", panelWidth, tester.Texts())
	}
	tester.SetSize(1241, 800)
	tester.Frame()
	if panelWidth != 244 {
		t.Fatalf("wide inspector width %v, want 244", panelWidth)
	}
	start := time.Date(2026, 6, 1, 9, 43, 0, 0, time.Local)
	task := func(id, member, assignment string, offset time.Duration) roundtable.Message {
		when := start.Add(offset)
		return roundtable.Message{ID: id, AgentID: member, Role: "assistant", Time: when, Task: &roundtable.Task{Stage: roundtable.Work, Status: roundtable.Completed, Assignment: assignment, Finished: when.Add(time.Second)}}
	}
	latest := task("latest", analyst, "Check the selected entry", 4*time.Second)
	latest.Task.Tools = []roundtable.ToolRecord{{Kind: "search", Detail: "docs/review.md:18:matching entry\ndocs/review.md:18:duplicate\nlib/config.go#L9", Note: "other.go:99"}}
	closed := task("closing", roundtable.ModeratorID, "Summarize", 12*time.Second)
	closed.Task.Stage, closed.Task.MemoryNotes = roundtable.Conclusion, 5
	snapshot.HasEarlier = true
	snapshot.Messages = []roundtable.Message{
		task("older", analyst, "Earlier assignment", 0), task("handoff", roundtable.ModeratorID, "Delegate", time.Second), latest,
		{ID: "legacy", Role: "assistant", AgentID: analyst}, closed,
	}
	if err := room.AddMemory("One actual pin", ""); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	stat(locales.RoundtableGraphNodes(), "4")
	stat(locales.RoundtableGraphHandovers(), "3")
	stat(locales.RoundtablePinnedCountLabel(), "1")
	stat(locales.RoundtableGraphDuration(), meetingDuration(13*time.Second))
	for _, label := range []string{latest.Task.Assignment, locales.RoundtableGraphRoleShort("requirements"), "Provider A · Model B · 09:43", locales.RoundtableRecentTasks(3), locales.RoundtableComplete(), locales.RoundtableNoMoreHandoffs(), locales.AgentDisabled(), locales.RoundtableEvidenceSources(), "docs/review.md:18", "lib/config.go#L9"} {
		if _, found := tester.Find(label); !found {
			t.Fatalf("missing %q in %v", label, tester.Texts())
		}
	}
	if tester.HasText("other.go:99") || w.membersScroll.MaxX > 0 {
		t.Fatal("tool note became a source or the member inspector overflowed")
	}
	click("Remy")
	if w.memberID != "agent:offline" || w.expandedTask != "" || tester.HasText(locales.RoundtableAskMember()) {
		t.Fatal("unavailable member history allowed a direct question")
	}
	click(locales.RoundtableBackToMembers())
	w.meeting.readOnly = true
	tester.Frame()
	if _, found := tester.Find(locales.RoundtableInvite()); found {
		t.Fatal("read-only roster allowed invitations")
	}
	click("Linh")
	if w.expandedTask != latest.ID || w.memberID != analyst || !tester.HasText(locales.RoundtableTaskAssignment()) || tester.HasText(locales.RoundtableAskMember()) {
		t.Fatalf("read-only card did not reveal the latest concrete task: %s, %v", w.expandedTask, tester.Texts())
	}
	click(locales.RoundtableBackToMembers())
	w.meeting.readOnly = false
	snapshot.Busy, snapshot.ActiveAgent = true, analyst
	snapshot.ActiveTask = task("active", analyst, "Check live changes", 30*time.Second)
	snapshot.ActiveTask.Task.Status, snapshot.ActiveTask.Task.Finished = roundtable.Running, time.Time{}
	tester.Frame()
	if !tester.HasText(locales.RoundtableRunning()) || !tester.HasText(snapshot.ActiveTask.Task.Assignment) || !tester.HasText(locales.RoundtableRecentTasks(4)) {
		t.Fatal("running assignment or task count missing", tester.Texts())
	}
	w.meeting.chat.permissions = []*agentPermission{{title: "Review changes"}}
	tester.Frame()
	if !tester.HasText(locales.AgentTaskApproval()) || tester.HasText(locales.RoundtableRunning()) {
		t.Fatal("approval did not replace the running status", tester.Texts())
	}
	click("Linh")
	if w.expandedTask != snapshot.ActiveTask.ID || w.graphSel != snapshot.ActiveTask.ID {
		t.Fatal("active member card opened an older task", w.expandedTask)
	}
}

func TestMeetingInspectorLongCardText(t *testing.T) {
	for _, sample := range []struct {
		width        int
		name, locale string
		dark         bool
	}{
		{224, "基础设施兼容性与无障碍评审负责人", "zh-Hans", false},
		{244, "Cross-platform compatibility and migration reviewer", "en", true},
	} {
		t.Run(strconv.Itoa(sample.width), func(t *testing.T) {
			previous := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return sample.locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
			room := roundtable.New(nil, roundtable.Options{})
			t.Cleanup(room.Close)
			speaker := meetingSpeaker{Participant: roundtable.Participant{ID: "preset:review", Name: "Kai"}, provider: agentProvider{name: "Studio"}}
			meeting := &agentMeeting{room: room, speakers: []meetingSpeaker{speaker}, chat: &agentChat{permissions: []*agentPermission{{}}}}
			w := &roundtableWindow{a: &app{}, meeting: meeting}
			const first = "Check the release manifest and identify migration risks."
			const assignment = first + "\nVerify recovery before publishing."
			live := roundtable.Message{ID: "review", AgentID: speaker.ID, Time: time.Date(2026, 6, 1, 14, 23, 0, 0, time.Local), Task: &roundtable.Task{
				Assignment: assignment, Status: roundtable.Running,
				Tools: []roundtable.ToolRecord{{Title: "title.md:42", Detail: "docs/review.md\nhttps://example.com:443\n14:23", Note: "note.md:15"}},
			}}
			snapshot := roundtable.Snapshot{HasEarlier: true, Busy: true, ActiveAgent: speaker.ID, ActiveTask: live}
			windowWidth := float32(1240)
			if sample.width == 244 {
				windowWidth++
			}
			prefs := settings.Default()
			prefs.Appearance, prefs.Theme = "light", ""
			if sample.dark {
				prefs.Appearance = "dark"
			}
			var fade paletteFade
			tester := ui.NewTester(func(c *ui.Context) {
				w.pal = applyTheme(c, &prefs.Prefs, &fade)
				ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() { w.memberInspector(c, snapshot, roundtable.BuildGraph(snapshot), windowWidth, true) })
			}, sample.width, 440)
			tester.SetDark(sample.dark)
			tester.SetLocale(sample.locale)
			bounds := func(label string) ui.Rect {
				t.Helper()
				rect, found := tester.Find(label)
				if !found || rect.W <= 0 || rect.X < 0 || rect.X+rect.W > float32(sample.width)+1 {
					t.Fatalf("%q outside %d px inspector: %+v", label, sample.width, rect)
				}
				return rect
			}
			count := locales.RoundtableRecentTasks(1)
			status := locales.AgentTaskApproval()
			shortCount, shortStatus := bounds(count), bounds(status)
			singleLine := bounds(meetingSpeakerModel(speaker) + " · 14:23").H
			member := &meeting.speakers[0]
			member.Name, member.role.Model = sample.name, "InternationalCompatibilityReviewerContextWindow65536Preview"
			tester.Frame()
			if dir := os.Getenv("SHOTS"); dir != "" {
				file, err := os.Create(filepath.Join(dir, "roundtable-members-"+strconv.Itoa(sample.width)+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tester.Image())
				if closeErr := file.Close(); err != nil || closeErr != nil {
					t.Fatalf("saving inspector render: %v, %v", err, closeErr)
				}
			}
			longStatus, longCount := bounds(status), bounds(count)
			model := bounds(meetingSpeakerModel(*member) + " · 14:23")
			if longStatus.Y-longCount.Y <= shortStatus.Y-shortCount.Y+8 || model.H <= singleLine+8 || longCount.W < shortCount.W-1 || longStatus.W < shortStatus.W-1 || w.membersScroll.MaxX > 0 {
				t.Fatalf("long identity or model lost space: status %+v → %+v, count %+v → %+v, model %+v", shortStatus, longStatus, shortCount, longCount, model)
			}
			if tester.HasText(locales.RoundtableEvidenceSources()) {
				t.Fatal("unlocated tool details produced an evidence section")
			}
			excerpt := bounds(first + "…")
			tester.Move(excerpt.X+excerpt.W/2, excerpt.Y+excerpt.H/2)
			tester.Advance(time.Second)
			if !tester.HasText(assignment) {
				t.Fatal("the assignment tooltip did not reveal the complete task")
			}
		})
	}
}

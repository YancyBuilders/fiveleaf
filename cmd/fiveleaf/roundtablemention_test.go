package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (h *roundtableHarness) mentionMember(t *testing.T, name string) {
	t.Helper()
	if err := h.tt.Click(locales.RoundtableMention()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if err := h.tt.Click("@" + name); err != nil {
		t.Fatal(err)
	}
	h.frames()
}

func (h *roundtableHarness) indexMeetingFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, name := range paths {
		file := h.path(name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("Review"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h.a.indexRead.Invalidate()
	h.until(t, "project files indexed", func() bool {
		h.a.refreshIndex()
		return slices.Contains(h.a.index, paths[len(paths)-1])
	})
}

func TestRoundtableMentionEditing(t *testing.T) {
	for _, sample := range []string{"abc@exam", "user+tag@host", "no recipient", "@Beta "} {
		end := utf8.RuneCountInString(sample)
		if _, ok := meetingMentionAt(sample, end, end); ok {
			t.Fatalf("unexpected completion in %q", sample)
		}
	}
	const draft = "请@需求分析 后面保留"
	token, ok := meetingMentionAt(draft, 4, 4)
	if !ok || token.query != "需求" {
		t.Fatal(token)
	}
	text, caret := token.replace(draft, "需求分析师")
	if text != "请@需求分析师 后面保留" || string([]rune(text)[caret:]) != "后面保留" || !meetingHasMention(text, "@需求分析师") || meetingHasMention(text, "@需求") {
		t.Fatal(text, caret)
	}
	window := &roundtableWindow{meeting: &agentMeeting{speakers: []meetingSpeaker{
		{Participant: roundtable.Participant{ID: "role:a", Name: "评审"}},
		{Participant: roundtable.Participant{ID: "role:b", Name: "评审"}},
		{Participant: roundtable.Participant{ID: "role:c", Name: "评审"}, problem: "disabled"},
	}}, draft: "@评"}
	items := window.mentionChoices("评")
	if len(items) != 2 {
		t.Fatal(items)
	}
	window.chooseMention(meetingMention{start: 0, end: 2}, items[1])
	if window.draft != "@role:b " || window.target != "role:b" {
		t.Fatal(window.draft, window.target)
	}
}

func TestRoundtableMentionPickerMouseAndKeys(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.tt.SetSize(640, 480)
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	click(locales.AgentDraft())
	h.tt.Type("Ask @")
	h.frames()
	for _, label := range []string{"@Alpha", "@Beta"} {
		bounds, found := h.tt.Find(label)
		if !found || bounds.Y < 0 || bounds.X+bounds.W > 640 || bounds.Y+bounds.H > 480 {
			t.Fatalf("candidate %s is not visible: %+v %v", label, bounds, h.tt.Texts())
		}
	}
	h.tt.Key(0, ui.KeyEscape)
	h.frames()
	if _, ok := h.tt.Find("@Beta"); ok {
		t.Fatal("Escape did not dismiss mentions")
	}
	h.tt.Type("Be")
	h.frames()
	h.tt.Key(0, ui.KeyTab)
	h.frames()
	if h.w.draft != "Ask @Beta " || h.w.target != "agent:beta" || !h.tt.Focused(locales.AgentDraft()) {
		t.Fatal(h.w.draft, h.w.target)
	}
	h.tt.Type("also @Al")
	h.frames()
	click("@Alpha")
	if h.w.draft != "Ask @Beta also @Alpha " || h.w.target != "agent:alpha" {
		t.Fatal(h.w.draft, h.w.target)
	}
	click(locales.AgentSend())
	h.until(t, "directed answer", func() bool { return !h.w.meeting.room.Snapshot().Busy })
	messages := h.w.meeting.room.Snapshot().Messages
	if len(messages) != 2 || messages[1].AgentID != "agent:alpha" || h.w.target != "" {
		t.Fatalf("a selected inline mention must direct exactly this question: %+v", messages)
	}
}

func TestRoundtableEditedMentionSendsToNamedRole(t *testing.T) {
	for _, name := range []string{"Alpha.Review", "Alpha Team"} {
		t.Run(name, func(t *testing.T) {
			h := newRoundtableHarness(t, nil)
			h.a.settings.AgentRoles = map[string]settings.AgentRole{name: {Agent: "alpha"}}
			role := roundtable.Participant{ID: "role:" + name, Name: name, Provider: "alpha"}
			h.w.addParticipant(role)
			for _, speaker := range h.w.meeting.speakers {
				if speaker.adapter != nil {
					speaker.adapter.backend = roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
						return roundtable.Message{Text: "Received"}, nil
					})
				}
			}
			h.mentionMember(t, "Alpha")
			h.tt.Command("selectAll")
			h.tt.Type("@" + name + " check this")
			h.frames()
			if err := h.tt.Click(locales.AgentSend()); err != nil {
				t.Fatal(err)
			}
			h.until(t, "the named member receives the message", func() bool { return !h.w.meeting.room.Busy() })
			messages := h.w.meeting.room.Snapshot().Messages
			if len(messages) != 2 || messages[1].AgentID != role.ID {
				t.Fatalf("edited mention was sent to the wrong member: %+v", messages)
			}
		})
	}
}

func TestRoundtableChatAlignmentAndExpansion(t *testing.T) {
	answer := strings.Repeat("A useful finding.\n\n", 21) + "**Final evidence**"
	h := newRoundtableHarness(t, roundtable.AdapterFunc(func(context.Context, []roundtable.Message) (roundtable.Message, error) {
		return roundtable.Message{Text: answer}, nil
	}))
	h.w.draft, h.w.target = "A quick question", "agent:alpha"
	h.w.send()
	h.until(t, "long answer", func() bool { return !h.w.meeting.room.Snapshot().Busy })
	question, ok := h.tt.Find("A quick question")
	author, hasAuthor := h.tt.Find(locales.RoundtableHuman())
	reply, hasReply := h.tt.Find(meetingReadingPreview(answer))
	if !ok || !hasAuthor || !hasReply || author.X <= reply.X || question.X <= reply.X {
		t.Fatal(question, author, reply, h.tt.Texts())
	}
	if err := h.tt.Click(locales.RoundtableFullResult()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.memberID != "" || !h.tt.HasText("Final evidence") || !h.tt.HasText(locales.RoundtableCollapse()) {
		t.Fatal("expanded message lost its reading position or its full Markdown")
	}
}

func TestRoundtableComposeToolsAndEnter(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	for index := range h.w.meeting.speakers {
		if h.w.meeting.speakers[index].ID == "agent:beta" {
			h.w.meeting.speakers[index].provider.config.Capabilities = []string{"检索"}
		}
	}
	click(locales.AgentDraft())
	h.tt.Type("查")
	click(locales.RoundtableMention())
	if h.w.draft != "查@" || !h.w.mentions.open {
		t.Fatal(h.w.draft, h.w.mentions.open)
	}
	h.tt.Type("检索")
	h.frames()
	if _, found := h.tt.Find("@Beta"); !found {
		t.Fatal("capability did not select Beta", h.tt.Texts())
	}
	h.tt.Key(0, ui.KeyEnter)
	h.frames()
	if h.w.target != "agent:beta" || len(h.w.meeting.room.Snapshot().Messages) != 0 {
		t.Fatal("candidate key sent the question", h.w.target)
	}
	h.tt.Key(ui.Shift, ui.KeyEnter)
	h.frames()
	if !strings.HasSuffix(h.w.draft, "\n") {
		t.Fatal("missing soft line break", h.w.draft)
	}
	h.tt.Type("More evidence")
	h.frames()
	h.tt.Key(0, ui.KeyEnter)
	h.frames()
	h.until(t, "enter dispatched selected role", func() bool { return !h.w.meeting.room.Snapshot().Busy })
	got := h.w.meeting.room.Snapshot().Messages
	if len(got) != 2 || got[1].AgentID != "agent:beta" || got[0].Text != "查@Beta \nMore evidence" {
		t.Fatal(got)
	}
	h.indexMeetingFiles(t, "src/main.go", "docs/review notes.md")
	click(locales.RoundtableReferenceFile())
	h.tt.Type("review")
	h.frames()
	if !h.tt.Focused(locales.QuickSearchFiles()) || h.tt.HasText("src/main.go") || !h.tt.HasText("docs/review notes.md") || h.w.draft != "" {
		t.Fatal("file search did not stay separate from the draft", h.tt.Texts(), h.w.draft)
	}
	h.tt.Key(0, ui.KeyEnter)
	h.frames()
	const reference = "`docs/review notes.md` "
	if h.w.draft != reference || !h.tt.Focused(locales.AgentDraft()) || len(h.w.meeting.room.Snapshot().Messages) != 2 {
		t.Fatal("choosing the file did not insert its path and return to the draft", h.w.draft, h.tt.Texts())
	}
	accelerator := ui.Ctrl
	if runtime.GOOS == "darwin" {
		accelerator = ui.Cmd
	}
	h.tt.Key(accelerator, ui.KeyEnter)
	h.frames()
	h.until(t, "shortcut sent file reference", func() bool { return !h.w.meeting.room.Snapshot().Busy })
	if got := h.w.meeting.room.Snapshot().Messages; len(got) != 4 || strings.TrimSpace(got[2].Text) != strings.TrimSpace(reference) {
		t.Fatal(got)
	}
}

func TestRoundtableFilePickerKeepsTargetAndDraft(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.w.target, h.w.draft = "agent:beta", "核对记录"
	h.indexMeetingFiles(t, "src/指南.md", "tests/指南.md")
	h.frames()
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
	}
	click(locales.RoundtableReferenceFile())
	h.tt.Type("未找到的文件")
	h.frames()
	if !h.tt.HasText(locales.QuickNoFiles()) {
		t.Fatal(h.tt.Texts())
	}
	h.tt.Key(0, ui.KeyEscape)
	h.frames()
	if h.w.draft != "核对记录" || h.w.target != "agent:beta" || !h.tt.Focused(locales.AgentDraft()) {
		t.Fatal("canceling a search changed the message or its recipient")
	}
	click(locales.RoundtableReferenceFile())
	click("tests/指南.md")
	if !strings.Contains(h.w.draft, "`tests/指南.md`") || h.w.target != "agent:beta" || len(h.w.meeting.room.Snapshot().Messages) != 0 {
		t.Fatal(h.w.draft, h.w.target)
	}
}

func TestRoundtableComposerFollowsItsPane(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	const left, width = 420, 310
	view := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Width(left).Shrink(0)
			ui.Column(c).Width(width).MinWidth(0).Children(func() {
				msg := roundtable.Message{ID: "human", Role: "user", Text: strings.Repeat("a narrow question ", 8)}
				h.w.transcript(c, roundtable.Snapshot{Messages: []roundtable.Message{msg}}, roundtable.Graph{})
				h.w.composer(c, roundtable.Snapshot{})
			})
		})
	}, 1200, 660)
	for range 4 {
		step(view)
	}
	if err := view.Click(locales.RoundtableMention()); err != nil {
		t.Fatal(err)
	}
	for range 6 {
		step(view)
	}
	for _, label := range []string{locales.AgentSend(), locales.AgentDraft(), "@Alpha", locales.RoundtableHuman()} {
		r, ok := view.Find(label)
		if !ok || r.X < left || r.X+r.W > left+width+1 {
			t.Fatalf("%q outside its 310pt editor pane: %v, %v", label, r, ok)
		}
	}
}

func TestRoundtableComposerWaitsForIMECommit(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	if err := h.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("问题：")
	h.tt.Compose("ni", 2)
	h.frames()
	h.tt.Key(0, ui.KeyEnter)
	h.frames()
	if len(h.w.meeting.room.Snapshot().Messages) != 0 {
		t.Fatal("the IME confirmation sent the draft")
	}
	h.tt.Type("你")
	h.frames()
	if h.w.draft != "问题：你" {
		t.Fatal(h.w.draft)
	}
}

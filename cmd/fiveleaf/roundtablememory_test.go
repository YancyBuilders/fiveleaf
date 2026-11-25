package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
)

func TestRoundtableMemoryArchiveOpensProjectMarkdown(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	h := newRoundtableHarness(t, nil)
	h.tt.SetSize(600, 720)
	h.w.inspector.setVisible(true)
	h.frames()
	if err := h.tt.Click(locales.RoundtableOpenMemory()); err != nil {
		t.Fatal(err)
	}
	h.until(t, "empty daily notes", func() bool { return h.tt.HasText(locales.RoundtableMemoryArchiveEmpty()) })
	meeting := h.w.meeting
	if err := meeting.room.SetHumanChair(true); err != nil {
		t.Fatal(err)
	}
	const question = "Which public draft should we submit?"
	const conclusion = "Submit the revised draft."
	if err := meeting.room.Discuss(meeting.group.Topic, question, ""); err != nil {
		t.Fatal(err)
	}
	if err := meeting.room.PublishConclusion(conclusion); err != nil {
		t.Fatal(err)
	}
	days, err := meeting.room.MemoryDays()
	if err != nil || len(days) != 1 {
		t.Fatalf("expected the conclusion's daily document, got %v, %v", days, err)
	}
	day := days[0]
	relative, err := filepath.Rel(meeting.library.Project, day.Path)
	if err != nil || !filepath.IsLocal(relative) || filepath.Ext(day.Path) != ".md" {
		t.Fatalf("expected a project Markdown file, got %q", day.Path)
	}
	h.until(t, "new date in the open drawer", func() bool { return h.tt.HasText(day.Date) })
	if !h.tt.HasText(locales.RoundtableMemoryArchive()) || h.tt.HasText(question) || h.tt.HasText(conclusion) {
		t.Fatalf("expected only a date for the public conversation: %q", h.tt.Texts())
	}
	content, err := os.ReadFile(day.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click(day.Date); err != nil {
		t.Fatal(err)
	}
	h.until(t, "daily Markdown in the editor", func() bool {
		editor := h.a.activeEditor()
		return editor != nil && editor.path == day.Path && !editor.unread()
	})
	if h.w.drawer != meetingDrawerClosed || h.a.activeEditor().buf.String() != string(content) {
		t.Fatal("the date did not dismiss the drawer and open its saved file")
	}
}

func TestRoundtableMemoryArchiveDatesStayWithTheirMeeting(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	h := newRoundtableHarness(t, nil)
	empty := h.w.meeting
	room, _ := openRoundtablePageArchive(t, []roundtable.Message{{ID: "early", Role: "user", Text: "Earlier decision", HumanConclusion: true}})
	older := room.Snapshot().Messages[0].Time.Local().Format(time.DateOnly)
	archiveTexts := func() []string {
		texts := h.tt.Texts()
		heading := slices.Index(texts, locales.RoundtableMemoryArchive())
		if heading < 0 {
			return nil
		}
		return texts[heading+1:]
	}
	hasDate := func(date string) bool { return slices.Contains(archiveTexts(), date) }
	foreign := &agentMeeting{room: room, readOnly: true}
	h.w.enter(foreign)
	h.w.openMeetingMemory()
	h.until(t, "unmigrated read-only archive", func() bool { return h.tt.HasText(locales.RoundtableMemoryArchiveEmpty()) })
	if days, err := room.MemoryDays(); err != nil || len(days) != 0 {
		t.Fatalf("the foreign read-only drawer backfilled project files: %v, %v", days, err)
	}
	h.w.enter(&agentMeeting{room: room, chat: &agentChat{}})
	h.w.openMeetingMemory()
	h.until(t, "writable drawer backfill", func() bool { return hasDate(older) })
	if err := room.PublishConclusion("Later decision"); err != nil {
		t.Fatal(err)
	}
	days, err := room.MemoryDays()
	if err != nil || len(days) != 2 {
		t.Fatalf("expected two dated documents, got %v, %v", days, err)
	}
	newer := max(days[0].Date, days[1].Date)
	h.w.enter(foreign)
	h.w.openMeetingMemory()
	h.until(t, "archived dates", func() bool { return hasDate(older) && hasDate(newer) })
	texts := archiveTexts()
	if slices.Index(texts, newer) >= slices.Index(texts, older) {
		t.Fatalf("expected %s before %s in the read-only drawer: %q", newer, older, texts)
	}
	if err := os.Rename(days[0].Path, days[0].Path+".bak"); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !hasDate(days[0].Date) {
		t.Fatal("idle frames reread the archive")
	}
	h.w.closeDrawer()
	h.w.openMeetingMemory()
	h.until(t, "refreshed file listing on reopen", func() bool { return !hasDate(days[0].Date) && hasDate(days[1].Date) })
	h.w.closeDrawer()
	h.w.openMeetingMemory()
	h.w.loadMemoryDays(room.Snapshot(), room.Memory())
	h.w.enter(empty)
	h.harness.settle()
	if len(h.w.inspector.memory.archive.days) != 0 {
		t.Fatal("a queued listing reached the next meeting")
	}
	h.w.openMeetingMemory()
	h.until(t, "other meeting's empty archive", func() bool { return h.tt.HasText(locales.RoundtableMemoryArchiveEmpty()) })
	if hasDate(newer) || hasDate(older) {
		t.Fatal("switching meetings retained dates from the previous group")
	}
}

func TestRoundtableMemoryPanelPinsAndClearsContext(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.tt.SetSize(1100, 760)
	h.frames()
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		h.frames()
		if h.w.meeting.changing != nil {
			h.until(t, "the meeting memory change", func() bool { return h.w.meeting.changing == nil })
		}
	}
	room := h.w.meeting.room
	const question = "Use the submission snapshot."
	click(locales.AgentDraft())
	h.tt.Type(question)
	click(locales.AgentSend())
	h.until(t, "public reply", func() bool { return !room.Busy() && len(room.Snapshot().Messages) == 2 })
	click(locales.RoundtablePinToMemory())
	if h.w.drawer != meetingDrawerMemory || h.w.inspector.memory.draft != question || !h.tt.Focused(locales.RoundtableMemoryNote()) {
		t.Fatal("pinning a chat message did not open the memory editor")
	}
	click(locales.ButtonSave())
	mem := room.Memory()
	if len(mem.Notes) != 1 || mem.Notes[0].Text != question || mem.Notes[0].SourceID != room.Snapshot().Messages[0].ID {
		t.Fatal(mem.Notes)
	}
	click(locales.RoundtableMemoryAdd())
	h.tt.Type("Round up to five seconds.")
	click(locales.ButtonSave())
	if notes := room.Memory().Notes; len(notes) != 2 || notes[1].SourceID != "" {
		t.Fatal(notes)
	}
	click(locales.RoundtableMemoryRemove("Round up to five seconds."))
	if len(room.Memory().Notes) != 1 {
		t.Fatal("removing a note changed the wrong record")
	}
	click("30")
	if room.Memory().Window != 30 {
		t.Fatal("memory window did not update")
	}
	click(locales.RoundtableShareMemory())
	if room.Memory().Shared || !h.tt.HasText(locales.RoundtableMemorySharingOff()) {
		t.Fatal("sharing did not pause")
	}
	click(locales.RoundtableShareMemory())
	click(locales.RoundtableMemoryClear())
	if !h.tt.HasText(locales.RoundtableMemoryConfirmClear()) || len(room.Memory().Recent) != 2 {
		t.Fatal("memory changed before confirmation")
	}
	click(locales.ButtonCancel())
	click(locales.RoundtableMemoryClear())
	click(locales.RoundtableMemoryClear())
	mem = room.Memory()
	if len(mem.Notes) != 0 || len(mem.Recent) != 0 || len(room.Snapshot().Messages) != 2 {
		t.Fatal("clearing memory did not keep the transcript")
	}
}

func TestRoundtableMemoryProvidedCountsAndReadOnly(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	h.tt.SetSize(1040, 720)
	room := h.w.meeting.room
	if err := room.AddMemory("Keep estimates fixed.", ""); err != nil {
		t.Fatal(err)
	}
	h.w.target, h.w.draft = "agent:alpha", "Give a short answer."
	h.w.send()
	h.until(t, "reply with memory", func() bool { return !room.Busy() && len(room.Snapshot().Messages) == 2 })
	if err := h.tt.Click(locales.RoundtableMemoryChip(1)); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.tt.HasText(locales.RoundtableMemoryUsed(1, 0)) {
		t.Fatal("the chip did not open the task's recorded context")
	}
	h.w.meeting.readOnly = true
	h.tt.SetSize(600, 520)
	h.frames()
	if err := h.tt.Click(locales.RoundtableOpenMemory()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.tt.HasText("Keep estimates fixed.") || !h.tt.HasText(locales.SessionsMeetingReadOnly()) {
		t.Fatal("read-only memory missing")
	}
	for _, label := range []string{locales.RoundtableMemoryAdd(), locales.RoundtableMemoryClear(), locales.RoundtableMemoryRemove("Keep estimates fixed.")} {
		if _, found := h.tt.Find(label); found {
			t.Fatal("archive exposed mutation", label)
		}
	}
	if err := h.tt.Click(locales.RoundtableShareMemory()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !room.Memory().Shared {
		t.Fatal("read-only archive changed memory sharing")
	}
}

func TestRoundtableMemoryPanelGuardsBusyAndOversizedDraft(t *testing.T) {
	h := newRoundtableHarness(t, roundtable.AdapterFunc(func(ctx context.Context, _ []roundtable.Message) (roundtable.Message, error) {
		<-ctx.Done()
		return roundtable.Message{}, ctx.Err()
	}))
	h.w.draft = "Hold the current turn"
	h.w.send()
	h.until(t, "busy", func() bool { return h.w.meeting.room.Busy() })
	if err := h.tt.Click(locales.RoundtableOpenMemory()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if err := h.tt.Click(locales.RoundtableMemoryAdd()); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click(locales.RoundtableShareMemory()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if h.w.inspector.memory.editing || !h.w.meeting.room.Memory().Shared || !h.tt.HasText(locales.RoundtableMemoryBusy()) {
		t.Fatal("busy controls changed memory")
	}
	h.w.meeting.room.Stop()
	h.frames()
	if err := h.tt.Click(locales.RoundtableMemoryAdd()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	h.tt.Type(strings.Repeat("a", roundtable.MaxMemoryNoteBytes+1))
	h.frames()
	if !h.tt.HasText(locales.RoundtableMemoryTooLong(roundtable.MaxMemoryNoteBytes)) {
		t.Fatal("no size limit hint")
	}
	if err := h.tt.Click(locales.ButtonSave()); err != nil {
		t.Fatal(err)
	}
	h.frames()
	if !h.w.inspector.memory.editing || len(h.w.meeting.room.Memory().Notes) != 0 {
		t.Fatal("invalid memory draft was discarded or submitted")
	}
}

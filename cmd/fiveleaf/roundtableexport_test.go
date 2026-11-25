package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
)

func TestRoundtableExportReadOnlyFromEarlierPage(t *testing.T) {
	hs := newHarness(t, true)
	dialog := &chatExportDialog{fakeHost: hs.h}
	hs.a.host = dialog
	project := t.TempDir()
	library, err := roundtable.ProjectLibrary(t.TempDir(), project)
	if err != nil {
		t.Fatal(err)
	}
	group, err := library.Create("Planning", []roundtable.Participant{{ID: "plan", Name: "Planner", Provider: "private-provider"}})
	if err != nil {
		t.Fatal(err)
	}
	fence := "```mermaid\nflowchart LR\n  Writer --> Reader\n```"
	legacyAnswer := "**Public historical answer**\n\n" + fence
	legacyRaw := "pi v1.5.2\n---\n\n## Skills\n- /private/skills/export/SKILL.md\n\n" + legacyAnswer
	answer := strings.Repeat("long finding ", 8000) + "end of result\n\n" + fence + "\n\n![diagram](assets/chart%20one.png)"
	adapter := roundtable.AdapterFunc(func(ctx context.Context, messages []roundtable.Message) (roundtable.Message, error) {
		if roundtable.TaskID(ctx) == "" {
			if messages[len(messages)-1].Text == "old question" {
				return roundtable.Message{Text: legacyRaw}, nil
			}
			<-ctx.Done()
			return roundtable.Message{}, ctx.Err()
		}
		roundtable.ReportTool(ctx, roundtable.ToolRecord{ID: "read", Kind: "read", Status: "completed", Title: "Read spec", Detail: "files/spec.md\nAuthorization: Bearer simulated-secret", Note: "Check scope\nAPI_KEY=never-export-this"})
		return roundtable.Message{Text: answer}, nil
	})
	active, err := library.OpenRoom(group, []roundtable.Member{{ID: "plan", Name: "Planner", Enabled: true, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	if err := active.SetSystemPrompt("private instructions"); err != nil {
		t.Fatal(err)
	}
	waitForReply := func() {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for active.Snapshot().Busy {
			select {
			case <-active.Events():
			case <-deadline:
				t.Fatal("waiting for the saved result")
			}
		}
	}
	if err := active.Send("old question", "plan"); err != nil {
		t.Fatal(err)
	}
	waitForReply()
	for index := range meetingPageSize + 2 {
		if err := active.Send(fmt.Sprintf("entry %03d", index), "plan"); err != nil {
			t.Fatal(err)
		}
	}
	if err := active.Discuss(group.Topic, "Give the final plan", "plan"); err != nil {
		t.Fatal(err)
	}
	waitForReply()
	active.Close()
	loaded, err := library.Group(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	meeting, err := hs.a.startMeeting(library, loaded)
	if err != nil || !meeting.readOnly {
		t.Fatal("opening historical meeting", err)
	}
	window := newRoundtableWindow(hs.a, meeting)
	reopened := meeting.room.Snapshot().Messages
	if reopened[1].Original != legacyRaw || reopened[1].Text != legacyAnswer {
		t.Fatal("historical record must retain its original and its complete public answer")
	}
	window.loadPage([]roundtable.Message{reopened[4]})
	if len(window.page) != 4 {
		t.Fatal("fixture did not load just the earliest messages")
	}
	window.exportMeeting()
	if dialog.path != hs.path("fiveleaf-roundtable-planning.md") || dialog.ext != "md" || dialog.save == nil {
		t.Fatal("missing Markdown dialog", dialog.path, dialog.ext)
	}
	hs.a.closeMeeting()
	window.meeting, window.page, window.draft = nil, nil, "unsaved later draft"
	dir := t.TempDir()
	path := filepath.Join(dir, "minutes.md")
	dialog.save(path)
	hs.settle()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	physicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(physicalDir, filepath.Join(library.Project, "assets", "chart%20one.png"))
	if err != nil {
		t.Fatal(err)
	}
	result := string(raw)
	for _, want := range []string{"# Planning\n", locales.RoundtableMembers() + ": Planner", "## " + locales.RoundtableHuman(), "## Planner", legacyAnswer, "entry 000", "entry 041", "Give the final plan", locales.RoundtableMemberReply(), locales.RoundtableDone(), strings.Repeat("long finding ", 8000) + "end of result", fence, "![diagram](" + filepath.ToSlash(relative) + ")", "Read spec", "files/spec.md", "Check scope"} {
		if !strings.Contains(result, want) {
			t.Fatalf("missing %.90q in export (%d bytes)", want, len(raw))
		}
	}
	for _, private := range []string{"pi v1.5.2", "/private/skills", "private instructions", "private-provider", "unsaved later draft", "simulated-secret", "never-export-this"} {
		if strings.Contains(result, private) {
			t.Fatalf("export includes %q", private)
		}
	}
	if strings.Index(result, "entry 000") >= strings.Index(result, "entry 041") || strings.Count(result, "entry 041") != 1 {
		t.Fatal("history is out of order or duplicated")
	}
	if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("export is not owner-private", err)
	}
}

func TestRoundtableExportCancelAndAtomicFailure(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	dialog := &chatExportDialog{fakeHost: h.h}
	h.a.host = dialog
	fails := h.a.fails
	h.w.exportMeeting()
	dialog.save("")
	h.settle()
	if _, err := os.Stat(dialog.path); !errors.Is(err, os.ErrNotExist) || h.a.fails != fails {
		t.Fatal("cancelling wrote a file or reported an error", err)
	}
	dir := t.TempDir()
	dialog.save(dir)
	h.settle()
	if h.a.fails != fails+1 {
		t.Fatal("save failure was not delivered to the owner")
	}
	path := filepath.Join(dir, "existing.md")
	if err := os.WriteFile(path, []byte("old minutes"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := streamPrivateExport(path, func(output io.Writer) error {
		if _, err := fmt.Fprint(output, "incomplete minutes"); err != nil {
			return err
		}
		return context.Canceled
	})
	if raw, readErr := os.ReadFile(path); !errors.Is(err, context.Canceled) || readErr != nil || string(raw) != "old minutes" {
		t.Fatal("a partial export replaced the last good copy", err, readErr)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 1 {
		t.Fatal("partial export file was left behind", files, err)
	}
}

func TestRoundtableExportImageDestinationsAndLiteralExamples(t *testing.T) {
	markdown := "![same](a.png) `![same](a.png)` ![same](<a b.png> \"title\")\n\n" +
		"![ref][plot]\n\n[plot]: xyz%20plot.png\n\n" +
		"```mermaid\n![same](a.png)\n```\n\n![remote](https://example.test/x.png) ![remote](//example.test/x.png) ![data](data:image/png;base64,AA==)"
	want := strings.NewReplacer("![same](a.png) `", "![same](../project/a.png) `", "<a b.png>", "<../project/a%20b.png>", "[plot]: xyz%20plot.png", "[plot]: ../project/xyz%20plot.png").Replace(markdown)
	base := t.TempDir()
	if got := rebaseMeetingImages(markdown, filepath.Join(base, "project"), filepath.Join(base, "export")); got != want {
		t.Fatalf("destinations or surrounding Markdown changed\n%s\nwant\n%s", got, want)
	}
}

func TestRoundtableExportConclusionsAndTaskOutcomes(t *testing.T) {
	start := time.Date(2026, 11, 19, 14, 21, 0, 0, time.FixedZone("UTC+8", 8*3600))
	for _, entry := range []struct{ status, stage, stageLabel, statusLabel string }{
		{roundtable.Failed, roundtable.Work, locales.MenuTasks(), locales.RoundtableFailed()},
		{roundtable.Stopped, roundtable.Conclusion, locales.RoundtableConclusion(), locales.RoundtableStopped()},
		{roundtable.Running, roundtable.Direct, locales.RoundtableMemberReply(), locales.AgentTaskRunning()},
	} {
		var output strings.Builder
		writer := bufio.NewWriter(&output)
		message := roundtable.Message{Role: "task", Name: "Alice", Time: start, Hop: 2, Task: &roundtable.Task{Assignment: "line one\nline two", Stage: entry.stage, Status: entry.status, Finished: start.Add(time.Minute), NoAnswer: true}}
		if err := writeMeetingMessage(writer, message, "", ""); err != nil {
			t.Fatal(err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"## Alice", "2026-11-19T14:21:00+08:00", "2026-11-19T14:22:00+08:00", locales.RoundtableHop(2), entry.stageLabel, entry.statusLabel, "> line one\n> line two", locales.RoundtableToolOnly()} {
			if !strings.Contains(output.String(), want) {
				t.Fatalf("missing %q in %q", want, output.String())
			}
		}
	}
}

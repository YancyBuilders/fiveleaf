package main

import (
	"slices"
	"testing"

	"fiveleaf/locales"
)

func TestMarkdownContextMenuFormatsAndInserts(t *testing.T) {
	hs, d := editHarness(t, "menu.md", "中文")
	hs.place(d, 0, 2)
	menuPoint := func() (float32, float32) {
		x, y, height, ok := d.lines.Caret(1)
		if !ok {
			t.Fatal("the selection has no caret geometry")
		}
		origin := caretTrailOrigin(t, hs, d)
		return origin.X + x, origin.Y + y + height/2
	}
	x, y := menuPoint()
	hs.tt.RightClickAt(x, y)
	for _, label := range []string{
		locales.CmdMarkdownBold(), locales.CmdMarkdownItalic(), locales.CmdMarkdownStrikethrough(),
		locales.CmdMarkdownInlineCode(), locales.CmdMarkdownInsertLink(), locales.CmdMarkdownInsertTable(),
		locales.MarkdownParagraph(), locales.CmdEditUndo(), locales.CmdEditRedo(),
	} {
		if !slices.Contains(hs.tt.Menu(), label) {
			t.Fatalf("markdown menu lacks %q: %v", label, hs.tt.Menu())
		}
	}
	if err := hs.tt.ChooseMenuItem(locales.CmdMarkdownBold()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, d, "**中文**")
	if !hs.tt.Focused(locales.EditorLabel(d.name())) {
		t.Fatal("a Markdown menu action did not return focus to its editor")
	}
	hs.place(d, 2, 4)
	x, y, height, ok := d.lines.Caret(3)
	if !ok {
		t.Fatal("the formatted text has no caret geometry")
	}
	origin := caretTrailOrigin(t, hs, d)
	hs.tt.RightClickAt(origin.X+x, origin.Y+y+height/2)
	if err := hs.tt.ChooseMenuItem(locales.CmdMarkdownInsertLink()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, d, "**[中文](https://)**")
	if got := d.buf.Slice(min(d.sel[0], d.sel[1]), max(d.sel[0], d.sel[1])); got != "https://" {
		t.Fatalf("link destination is not selected: %q", got)
	}
}

func TestMarkdownMenuReplacesToolbarAndReadingControls(t *testing.T) {
	hs, d := editHarness(t, "plain.md", "# Heading\n\nText")
	for _, cmd := range hs.a.commands() {
		if cmd.id == "view.markdownOpen.reading" {
			t.Fatal("Markdown still offers a Reading mode")
		}
	}
	hs.a.setMarkdownMode("live")
	hs.settle()
	if !hs.a.livePreview(d) {
		t.Fatal("Live Preview did not open")
	}
}

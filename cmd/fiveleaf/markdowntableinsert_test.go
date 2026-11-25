package main

import (
	"os"
	"strings"
	"testing"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func TestMarkdownInsertTablePaletteDimensions(t *testing.T) {
	hs, doc := editHarness(t, "table.md", "标题")
	hs.place(doc, 2, 0)
	hs.a.run("go.commands")
	hs.settle()
	hs.tt.Type(locales.CmdMarkdownInsertTable())
	hs.key(0, ui.KeyEnter)
	if hs.a.confirm == nil || !hs.tt.Focused(locales.MarkdownTableChooseSize()) {
		t.Fatal("Insert Table did not focus the dimensions grid")
	}
	hs.key(0, ui.KeyTab)
	hs.key(ui.Cmd, ui.KeyA)
	hs.tt.Type("3")
	hs.key(0, ui.KeyTab)
	if !hs.tt.Focused(locales.MarkdownTableColumns()) {
		t.Fatal("Tab did not focus the column count")
	}
	hs.key(ui.Cmd, ui.KeyA)
	hs.tt.Type("4")
	hs.key(0, ui.KeyEnter)
	if hs.a.confirm != nil {
		t.Fatal("Enter did not finish inserting the table")
	}
	blocks := parseMarkdown([]byte(doc.buf.String()), doc.path, "", nil)
	if len(blocks) != 3 || blocks[0].kind != markdownTableHeader || markdownRunsText(blocks[0].cells[0]) != "标题" {
		t.Fatalf("unexpected table: %q", doc.buf.String())
	}
	for index, block := range blocks {
		if len(block.cells) != 4 {
			t.Fatalf("row %d has %d columns", index, len(block.cells))
		}
	}
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, doc, "标题")
	expectSel(t, doc, 2, 0)
}

func TestMarkdownInsertTableMenuCancelAndMinimum(t *testing.T) {
	hs, doc := editHarness(t, "single.md", "原文")
	hs.place(doc, 0, 2)
	x, y, height, ok := doc.lines.Caret(1)
	if !ok {
		t.Fatal("no caret")
	}
	origin := caretTrailOrigin(t, hs, doc)
	hs.tt.RightClickAt(origin.X+x, origin.Y+y+height/2)
	if err := hs.tt.ChooseMenuItem(locales.CmdMarkdownInsertTable()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.key(0, ui.KeyTab)
	hs.key(ui.Cmd, ui.KeyA)
	hs.tt.Type("0")
	hs.key(0, ui.KeyEnter)
	if hs.a.confirm == nil || !hs.tt.HasText(locales.MarkdownTableSizeRange("100")) {
		t.Fatal("invalid table dimensions were accepted")
	}
	hs.key(0, ui.KeyEscape)
	expectText(t, doc, "原文")
	expectSel(t, doc, 0, 2)
	if len(doc.hist.undo) != 0 || hs.a.confirm != nil {
		t.Fatal("canceling created undo history or kept the dialog open")
	}
	hs.a.run("markdown.insertTable")
	hs.settle()
	hs.key(0, ui.KeyTab)
	hs.key(ui.Cmd, ui.KeyA)
	hs.tt.Type("1")
	hs.key(0, ui.KeyTab)
	hs.key(ui.Cmd, ui.KeyA)
	hs.tt.Type("1")
	hs.settle()
	if err := hs.tt.Click(locales.MarkdownTableInsert()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, doc, "| 原文 |\n| --- |")
}

func TestMarkdownInsertTableDoesNotFollowTabOrReplaceStaleSelection(t *testing.T) {
	hs, first := editHarness(t, "first.md", "原文")
	hs.place(first, 0, 2)
	hs.a.run("markdown.insertTable")
	hs.settle()
	path := hs.path("other.md")
	if err := os.WriteFile(path, []byte("其他"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	other := hs.a.activeEditor()
	hs.key(0, ui.KeyEnter)
	expectText(t, first, "原文")
	expectText(t, other, "其他")
	hs.a.run("markdown.insertTable")
	hs.settle()
	other.buf.Replace(0, other.buf.Len(), "新的正文")
	hs.key(0, ui.KeyEnter)
	expectText(t, other, "新的正文")
}

func TestMarkdownTableGridSelectsRectangle(t *testing.T) {
	for _, gesture := range []string{"click", "drag", "keys"} {
		t.Run(gesture, func(t *testing.T) {
			hs, doc := editHarness(t, "grid.md", "")
			hs.a.run("markdown.insertTable")
			hs.settle()
			grid := hs.detailBounds(locales.MarkdownTableChooseSize())
			x, y := grid.X+grid.W*3.5/tableGridColumns, grid.Y+grid.H*2.5/tableGridRows
			switch gesture {
			case "click":
				hs.tt.Move(x, y)
				hs.settle()
				if !hs.tt.HasText(locales.MarkdownTableDimensions("4", "3")) {
					t.Fatal("hover did not show four columns by three rows")
				}
				hs.tt.ClickAt(x, y)
			case "drag":
				hs.tt.Press(grid.X+4, grid.Y+4)
				hs.tt.Move(x, y)
				hs.tt.Release(x, y)
			case "keys":
				hs.key(0, ui.KeyRight)
				hs.key(0, ui.KeyRight)
				hs.key(0, ui.KeyDown)
				hs.key(0, ui.KeyEnter)
			}
			hs.settle()
			blocks := parseMarkdown([]byte(doc.buf.String()), doc.path, "", nil)
			if hs.a.confirm != nil || len(blocks) != 3 || len(blocks[0].cells) != 4 {
				t.Fatalf("grid selection produced %q", doc.buf.String())
			}
		})
	}
}

func TestMarkdownInsertTableKeepsCurrentHeading(t *testing.T) {
	const source = "## 项目清单\n"
	for _, caret := range []int{3, 5} {
		hs, doc := editHarness(t, "heading.md", source)
		hs.a.setMarkdownMode("live")
		hs.place(doc, caret, caret)
		hs.a.run("markdown.insertTable")
		hs.settle()
		hs.key(0, ui.KeyEnter)
		result := doc.buf.String()
		blocks := parseMarkdown([]byte(result), doc.path, "", nil)
		if len(blocks) != 3 || !strings.Contains(result, "## 项目清单") {
			t.Fatalf("inserting at rune %d broke the heading: %q", caret, result)
		}
		headingAt := 0
		if caret == 3 {
			headingAt = 2
		}
		if blocks[headingAt].kind != markdownHeading || blocks[headingAt].level != 2 {
			t.Fatalf("expected the intact level-two heading at block %d: %q", headingAt, result)
		}
	}
}

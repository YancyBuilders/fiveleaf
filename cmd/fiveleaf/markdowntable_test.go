package main

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func TestLiveTableEditClickedCell(t *testing.T) {
	const source = "---\ntitle: 草稿\n---\n\n# 笔记\n\n项目 | 状态\n--- | ---:\n左\\|列 | **待校对📖**\n\n完\n"
	hs, d := editHarness(t, "notes.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("table cell", func() bool {
		_, found := hs.tt.Find("待校对📖")
		return found
	})
	hs.settle()
	if err := hs.tt.Click("待校对📖"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	label := locales.MarkdownTableCell("2", "2")
	if !hs.tt.Focused(label) || !hs.tt.HasText("项目") || !hs.tt.HasText("左|列") {
		t.Fatalf("editing the cell hid the grid or lost focus: %v", hs.tt.Texts())
	}
	const original = "**待校对📖**"
	start := utf8.RuneCountInString(source[:strings.Index(source, original)])
	expectSel(t, d, start, start+utf8.RuneCountInString(original))
	expectText(t, d, source)
	hs.tt.Compose("已", 1)
	hs.settle()
	expectText(t, d, source)
	hs.tt.Type("已完成")
	hs.settle()
	want := strings.Replace(source, original, "已完成", 1)
	expectText(t, d, want)
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, source)
	hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
	hs.a.run("file.save")
	hs.settle()
	got, err := os.ReadFile(d.path)
	if err != nil || string(got) != want {
		t.Fatalf("saved table = %q, error %v", got, err)
	}
}

func TestLiveTableNavigateAndPaste(t *testing.T) {
	const source = "前言\n\n| 甲 | 乙 |\n| -- | -- |\n| 一 | 二 |\n| 三 | 四 |\n\n后记"
	hs, d := editHarness(t, "cells.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("table", func() bool { return hs.tt.HasText("甲") })
	if err := hs.tt.Click("甲"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.key(0, ui.KeyTab)
	if !hs.tt.Focused(locales.MarkdownTableCell("1", "2")) || !hs.tt.HasText("三") {
		t.Fatal("Tab did not focus the next cell in the grid")
	}
	hs.key(0, ui.KeyEnter)
	if !hs.tt.Focused(locales.MarkdownTableCell("2", "2")) {
		t.Fatal("Enter did not move down one row")
	}
	hs.tt.SetClipboard("新|列")
	hs.command("paste")
	expectText(t, d, strings.Replace(source, "二", `新\|列`, 1))
	hs.key(ui.Shift, ui.KeyTab)
	if !hs.tt.Focused(locales.MarkdownTableCell("2", "1")) {
		t.Fatal("Shift+Tab did not move back a cell")
	}
	hs.key(0, ui.KeyEscape)
	if !hs.tt.Focused(locales.EditorLabel(d.name())) || !hs.tt.HasText("新|列") {
		t.Fatal("leaving the table did not return to the document")
	}
}

func TestLiveTableExitWritesOutsideGrid(t *testing.T) {
	const table = "| 顶部 |\n| --- |\n| 底部 |"
	for _, test := range []struct {
		name, source, cell, want string
		mods                     ui.Modifiers
		key                      ui.Key
	}{
		{"escape at end", table, "底部", table + "\n\n续写", 0, ui.KeyEscape},
		{"enter with final newline", table + "\n", "底部", table + "\n\n续写", 0, ui.KeyEnter},
		{"tab at end", table, "底部", table + "\n\n续写", 0, ui.KeyTab},
		{"reverse tab at start", table, "顶部", "续写\n\n" + table, ui.Shift, ui.KeyTab},
		{"existing following paragraph", table + "\n\n原文", "底部", table + "\n\n续写原文", 0, ui.KeyTab},
		{"existing preceding paragraph", "原文\n\n" + table, "顶部", "原文续写\n\n" + table, ui.Shift, ui.KeyTab},
	} {
		t.Run(test.name, func(t *testing.T) {
			hs, d := editHarness(t, "exit.md", test.source)
			hs.a.setMarkdownMode("live")
			hs.until("rendered table", func() bool { return hs.tt.HasText(test.cell) })
			if err := hs.tt.Click(test.cell); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			hs.key(test.mods, test.key)
			afterExit := d.buf.String()
			if afterExit == test.source && len(d.hist.undo) != 0 {
				t.Fatal("moving to the adjacent paragraph created an undo step")
			}
			hs.tt.Type("续写")
			hs.settle()
			expectText(t, d, test.want)
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, afterExit)
			if afterExit != test.source {
				hs.key(ui.Cmd, ui.KeyZ)
				expectText(t, d, test.source)
			}
		})
	}
}

func TestLiveTableRetainsFocusAndSpacesAcrossParses(t *testing.T) {
	const source = "前言\n\n| 左 | 右 |\n| --- | --- |\n| 测试 | 原文 |\n"
	hs, d := editHarness(t, "spaces.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("parsed table", func() bool { return hs.tt.HasText("测试") })
	if err := hs.tt.Click("原文"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	for _, fragment := range []string{"hello", " ", "世界", "!"} {
		hs.tt.Type(fragment)
		hs.settle()
		if !hs.tt.Focused(locales.MarkdownTableCell("2", "2")) || !hs.tt.HasText("测试") {
			t.Fatal("a reparse dropped the input or the other cells")
		}
	}
	want := strings.Replace(source, "原文", "hello 世界!", 1)
	expectText(t, d, want)
	if err := hs.tt.Click(hs.a.withKey(locales.ShowSource(), "view.toggleLivePreview")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if d.live || d.tableCell != nil || !hs.tt.Focused(locales.EditorLabel(d.name())) {
		t.Fatal("switching to source left the cell editor active")
	}
	expectText(t, d, want)
	if err := hs.tt.Click(hs.a.withKey(locales.ShowPreview(), "view.toggleLivePreview")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.tt.HasText("测试") || !d.live {
		t.Fatal("switching back did not restore the grid")
	}
}

func TestLiveTableClearsUnframedHeaderWithoutRemovingGrid(t *testing.T) {
	const source = "文档\n\n甲 | 乙\n--- | ---\n一 | 二\n"
	hs, d := editHarness(t, "headers.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("parsed table", func() bool { return hs.tt.HasText("二") })
	for _, label := range []string{"甲", "乙"} {
		if err := hs.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		hs.key(0, ui.KeyBackspace)
		if !hs.tt.HasText("一") || !hs.tt.HasText("二") {
			t.Fatalf("clearing header %q removed the grid: %q", label, d.buf.String())
		}
	}
	blocks := parseMarkdown([]byte(d.buf.String()), d.path, "", nil)
	if header := blocks[1]; header.kind != markdownTableHeader || len(header.cells) != 2 {
		t.Fatalf("clearing both headers damaged the table: %q", d.buf.String())
	}
}

func TestLiveTableHeaderWithoutBody(t *testing.T) {
	const source = "前言\n\n> | 左 | 右 |\n> | --- | --- |\n\n后记\n"
	hs, d := editHarness(t, "notes.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("parsed table", func() bool { return d.document.live != nil && d.document.live.doc != nil && !d.document.live.quick })
	hs.settle()
	if err := hs.tt.Click("右"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	start := utf8.RuneCountInString(source[:strings.Index(source, "右")])
	expectSel(t, d, start, start+1)
}

func TestLiveTableCellSourceRanges(t *testing.T) {
	const source = "序言\r\n\r\n> | 甲 | 乙 | 丙 |\r\n> | - | - | - |\r\n> |  | a\\|b | `😀` |\r\n> | 末行 |\r\n"
	doc := parseLive(source, "note.md", "", true, &markdownCache{})
	if len(doc.blocks) != 1 {
		t.Fatalf("found %d table blocks", len(doc.blocks))
	}
	block := doc.blocks[0]
	text := []rune(source)
	want := [][]string{{"甲", "乙", "丙"}, {"", `a\|b`, "`😀`"}, {"末行", "", ""}}
	if len(block.cellRanges) != len(want) {
		t.Fatalf("got %d rows", len(block.cellRanges))
	}
	for row, cells := range block.cellRanges {
		if len(cells) != len(want[row]) {
			t.Fatalf("row %d has %d cells", row, len(cells))
		}
		for column, cell := range cells {
			from, to := block.start+cell.start, block.start+cell.end
			if got := string(text[from:to]); got != want[row][column] {
				t.Errorf("cell (%d,%d) = %q, want %q", row, column, got, want[row][column])
			}
			if row == 2 && column > 0 && (from != block.end || to != block.end) {
				t.Errorf("missing cell (%d,%d) opens at %d:%d, want end of row %d", row, column, from, to, block.end)
			}
		}
	}
}

func TestLiveTableAddMissingCell(t *testing.T) {
	for _, row := range []string{"| 简记 |", "简记", `简记\|`, "| 简记 ||", "||", "|||", `| 简记\||`} {
		t.Run(row, func(t *testing.T) {
			const header = "导言\n\n| 甲 | 乙 | 丙 |\n| - | - | - |\n"
			source := header + row + "\n"
			hs, d := editHarness(t, "short.md", source)
			hs.a.setMarkdownMode("live")
			label := locales.MarkdownAddTableCell("3")
			hs.until("missing third cell", func() bool { _, found := hs.tt.Find(label); return found })
			if err := hs.tt.Click(label); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			hs.tt.Type("更新")
			hs.settle()
			parsed := parseMarkdown([]byte(d.buf.String()), d.path, "", nil)
			last := parsed[len(parsed)-1]
			if len(last.cells) != 3 || markdownRunsText(last.cells[1]) != "" || markdownRunsText(last.cells[2]) != "更新" {
				t.Fatalf("wrong column after typing: %q", d.buf.String())
			}
			hs.key(ui.Cmd, ui.KeyZ)
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, source)
		})
	}
}

func TestLiveTableCompactEmptyCell(t *testing.T) {
	const source = "开头\n\n| 甲 | 乙 | 丙 |\n| - | - | - |\n| 简记 ||\n"
	hs, d := editHarness(t, "compact.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("compact table", func() bool { _, found := hs.tt.Find("简记"); return found })
	column, _ := hs.tt.Find("乙")
	row, _ := hs.tt.Find("简记")
	hs.tt.ClickAt(column.X+column.W/2, row.Y+row.H/2)
	hs.settle()
	expectText(t, d, source)
	at := utf8.RuneCountInString(source) - 2
	expectSel(t, d, at, at)
	hs.tt.Type("更新")
	hs.settle()
	expectText(t, d, strings.Replace(source, "简记 ||", "简记 |更新|", 1))
}

func TestLiveTableEditInOtherSplit(t *testing.T) {
	const table = "\n\n| 章节 | 状态 |\n| --- | --- |\n| 日志 | 初稿 |\n\n"
	source := "笔记" + table + strings.Repeat("段落\n\n", 60)
	hs, left, right := splitHarness(t, func(hs *harness) string {
		path := hs.path("split.md")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	})
	hs.a.setLive(left, true)
	right.vscroll.Focus()
	hs.until("left table", func() bool { return hs.tt.HasText("初稿") && hs.a.activeEditor() == right })
	hs.settle()
	cell, ok := hs.tt.Find("初稿")
	if !ok {
		t.Fatal("body cell is missing")
	}
	x, y := cell.X+cell.W/2, cell.Y+cell.H/2
	hs.tt.Scroll(x, y, 0, 32)
	hs.settle()
	if left.vscroll.Y == 0 || right.vscroll.Y != 0 {
		t.Fatalf("cell wheel scrolled left=%v, right=%v", left.vscroll.Y, right.vscroll.Y)
	}
	if err := hs.tt.Click("初稿"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.activeEditor() != left {
		t.Fatal("cell click left focus in the source split")
	}
	start := utf8.RuneCountInString(source[:strings.Index(source, "初稿")])
	expectSel(t, left, start, start+2)
	expectSel(t, right, 0, 0)
}

func TestLiveTableExternalSplitEditEndsStaleCell(t *testing.T) {
	const source = "前言\n\n| 甲 | 乙 |\n| -- | -- |\n| 左 | 旧值 |\n"
	hs, left, right := splitHarness(t, func(hs *harness) string {
		path := hs.path("both.md")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	})
	hs.a.setLive(left, true)
	hs.until("left table", func() bool { return hs.tt.HasText("旧值") })
	if err := hs.tt.Click("旧值"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	cell := left.tableCell
	if cell == nil {
		t.Fatal("the left cell was not editable")
	}
	start := utf8.RuneCountInString(source[:strings.Index(source, "旧值")])
	hs.a.queueEdit(right, func(editor *editor) { editor.edit([2]int{start + 2, start + 2}, textEdit{start, start + 2, "新值"}) })
	hs.settle()
	if left.tableCell != nil || !hs.tt.HasText("新值") {
		t.Fatal("the left grid did not adopt the other split's edit")
	}
	cell.text = "过期"
	hs.a.writeLiveCell(left, cell, 2, 2)
	expectText(t, right, strings.Replace(source, "旧值", "新值", 1))
}

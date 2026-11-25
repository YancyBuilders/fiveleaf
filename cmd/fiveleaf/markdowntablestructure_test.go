package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestMarkdownTableStructureKeepsLoadedPictures(t *testing.T) {
	const source = "| Item | State |\n| --- | --- |\n| ![picture](picture.png) | Ready |"
	root := t.TempDir()
	picture := filepath.Join(root, "picture.png")
	writePNG(t, picture, 16, 24)
	a := &app{root: root, settings: &settings.Settings{}}
	d := newEditor(newDocument(1, filepath.Join(root, "pictures.md"), source, nil))
	d.live, d.document.live = true, &liveState{}
	parsed := parseLive(source, d.path, root, true, &markdownCache{})
	if len(parsed.blocks) != 1 || parsed.blocks[0].blocks[1].cells[0][0].bitmap == nil {
		t.Fatal("did not load the table's picture")
	}
	if err := os.Remove(picture); err != nil {
		t.Fatal(err)
	}
	a.changeTable(d, &parsed.blocks[0], 1, 0, tableColumnLeft)
	if d.tableCell == nil || d.tableCell.column != 0 {
		t.Fatal("did not focus the inserted column")
	}
	run := d.tableCell.block.blocks[1].cells[1][0]
	if run.bitmap == nil || run.w != 16 || run.h != 24 {
		t.Fatal("changing the table dropped the already rendered picture")
	}
	d.undo()
	expectText(t, d, source)
}

func TestMarkdownTableStructureMenu(t *testing.T) {
	const source = "引言\n\n> | 功能 | 状态 |\n> | :--- | ---: |\n> | a\\|b | **完成** |\n> | 尾 | 正文 |\n\n结语"
	for _, test := range []struct {
		name, label, command, want string
		row, column                int
	}{
		{"above first", "功能", locales.MarkdownTableInsertAbove(), "> |  |  |\n> | :--- | ---: |\n> | 功能 | 状态 |", 1, 1},
		{"below", "完成", locales.MarkdownTableInsertBelow(), "> | a\\|b | **完成** |\n> |  |  |\n> | 尾 | 正文 |", 3, 2},
		{"left", "状态", locales.MarkdownTableInsertLeft(), "> | 功能 |  | 状态 |\n> | :--- | --- | ---: |\n> | a\\|b |  | **完成** |", 1, 2},
		{"right", "完成", locales.MarkdownTableInsertRight(), "> | 功能 | 状态 |  |\n> | :--- | ---: | --- |\n> | a\\|b | **完成** |  |", 2, 3},
		{"remove header", "功能", locales.MarkdownTableDeleteRow(), "> | a\\|b | **完成** |\n> | :--- | ---: |\n> | 尾 | 正文 |", 1, 1},
		{"remove row", "完成", locales.MarkdownTableDeleteRow(), "> | :--- | ---: |\n> | 尾 | 正文 |", 2, 2},
		{"remove column", "功能", locales.MarkdownTableDeleteColumn(), "> | 状态 |\n> | ---: |\n> | **完成** |\n> | 正文 |", 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			hs, d := editHarness(t, "table.md", source)
			hs.a.setMarkdownMode("live")
			hs.until("rendered cell", func() bool { return hs.tt.HasText(test.label) })
			cell := hs.detailBounds(test.label)
			hs.tt.RightClickAt(cell.X+cell.W/2, cell.Y+cell.H/2)
			if err := hs.tt.ChooseMenuItem(test.command); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if !strings.Contains(d.buf.String(), test.want) || d.tableCell == nil || d.tableCell.row+1 != test.row || d.tableCell.column+1 != test.column {
				t.Fatalf("%s produced %q, focus %+v", test.name, d.buf.String(), d.tableCell)
			}
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, source)
		})
	}
}

func TestMarkdownTableStructureInsideActiveCellAndUndoDelete(t *testing.T) {
	const source = "before\n\n| 单列 |\n| :--- |\n\nafter"
	hs, d := editHarness(t, "one.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("single header", func() bool { return hs.tt.HasText("单列") })
	if err := hs.tt.Click("单列"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.tt.Type("已编辑")
	hs.settle()
	cell := hs.detailBounds(locales.MarkdownTableCell("1", "1"))
	hs.tt.RightClickAt(cell.X+cell.W/2, cell.Y+cell.H/2)
	if err := hs.tt.ChooseMenuItem(locales.MarkdownTableDelete()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, d, "before\n\nafter")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, strings.Replace(source, "单列", "已编辑", 1))
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, source)
}

func TestMarkdownTableStructurePreservesNestedSparseData(t *testing.T) {
	const source = "before\n\n- | A | B | C |\n  | :- | :-: | -: |\n  | 甲 |\n  | 乙 | `a\\|b` | **c** |\n\nafter"
	hs, d := editHarness(t, "nested.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("nested table", func() bool { return hs.tt.HasText("甲") })
	box := hs.detailBounds("B")
	hs.tt.RightClickAt(box.X+box.W/2, box.Y+box.H/2)
	if err := hs.tt.ChooseMenuItem(locales.MarkdownTableInsertLeft()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	const want = "- | A |  | B | C |\n  | :- | --- | :-: | -: |\n  | 甲 |\n  | 乙 |  | `a\\|b` | **c** |"
	if !strings.Contains(d.buf.String(), want) {
		t.Fatalf("nested cells changed: %q", d.buf.String())
	}
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, source)
}

func TestMarkdownTableColumnSourceFidelity(t *testing.T) {
	const source = "正文\n\n> A  |　保留　\t|尾\u00a0  \n> :---- | :---: |----:   \n> | a\\|b | \u00a0内容\u00a0 | `c\\|d` |\t \n> | 稀疏  |  \n> |左|中|右|超宽|\n\n后文"
	for _, test := range []struct {
		name   string
		column int
		action tableAction
	}{
		{"interior", 1, tableColumnLeft}, {"before unframed header", 0, tableColumnLeft}, {"after unframed header", 2, tableColumnRight},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := &app{settings: &settings.Settings{}}
			d := newEditor(newDocument(1, "source.md", source, nil))
			d.live, d.document.live = true, &liveState{}
			parsed := parseLive(source, d.path, "", true, &markdownCache{})
			if len(parsed.blocks) != 1 || len(parsed.blocks[0].cellRanges[0]) != 3 {
				t.Fatal("invalid test table")
			}
			a.changeTable(d, &parsed.blocks[0], 0, test.column, test.action)
			if d.tableCell == nil || len(d.tableCell.block.cellRanges[0]) != 4 {
				t.Fatal("column was not inserted")
			}
			inserted := d.buf.String()
			for _, authored := range []string{"　保留　\t", "尾\u00a0", "a\\|b", "\u00a0内容\u00a0", "`c\\|d`", "|\t \n", "| 稀疏  |", "超宽"} {
				if !strings.Contains(inserted, authored) {
					t.Fatalf("inserting a column changed %q in %q", authored, inserted)
				}
			}
			if test.name == "interior" && !strings.Contains(inserted, "> A  |  |　保留　\t|尾\u00a0  \n> :---- | --- | :---: |----:   \n") {
				t.Fatalf("inner-column insert changed the framing or alignment: %q", inserted)
			}
			cell := d.tableCell
			a.changeTable(d, &cell.block, cell.row, cell.column, tableDeleteColumn)
			expectText(t, d, source)
			d.undo()
			expectText(t, d, inserted)
			d.undo()
			expectText(t, d, source)
		})
	}
}

func TestMarkdownTableDeletingInsertedColumnKeepsLaterEdits(t *testing.T) {
	const source = "A| B  |　C　\n:-| :-: |--:\n|x|old\\|text|last|\n|sparse|\n"
	a := &app{settings: &settings.Settings{}}
	d := newEditor(newDocument(1, "later.md", source, nil))
	d.live, d.document.live = true, &liveState{}
	parsed := parseLive(source, d.path, "", true, &markdownCache{})
	a.changeTable(d, &parsed.blocks[0], 0, 1, tableColumnLeft)
	inserted := d.buf.String()
	a.beginLiveCell(d, &d.tableCell.block, 1, 2)
	cell := d.tableCell
	cell.text = "仍然|保留"
	a.writeLiveCell(d, cell, 5, 5)
	edited := d.buf.String()
	a.changeTable(d, &cell.block, 0, 1, tableDeleteColumn)
	expectText(t, d, strings.Replace(source, `old\|text`, `仍然\|保留`, 1))
	d.undo()
	expectText(t, d, edited)
	d.undo()
	expectText(t, d, inserted)
}

func TestMarkdownTableRepeatedEdgeColumnEdits(t *testing.T) {
	const source = "一 | 二\n:---|---:\n甲 | 乙\n"
	a := &app{settings: &settings.Settings{}}
	d := newEditor(newDocument(1, "edges.md", source, nil))
	d.live, d.document.live = true, &liveState{}
	parsed := parseLive(source, d.path, "", true, &markdownCache{})
	a.changeTable(d, &parsed.blocks[0], 0, 0, tableColumnLeft)
	first := d.buf.String()
	a.changeTable(d, &d.tableCell.block, 0, 2, tableColumnRight)
	a.changeTable(d, &d.tableCell.block, 0, 3, tableDeleteColumn)
	expectText(t, d, first)
	a.changeTable(d, &d.tableCell.block, 0, 0, tableDeleteColumn)
	expectText(t, d, source)
}

func TestMarkdownTableDeletingFirstColumnPreservesBlockLikeCells(t *testing.T) {
	const source = "首先| # 标题\n---|---\n数据|    内容\n"
	a := &app{settings: &settings.Settings{}}
	d := newEditor(newDocument(1, "punctuation.md", source, nil))
	d.live, d.document.live = true, &liveState{}
	parsed := parseLive(source, d.path, "", true, &markdownCache{})
	a.changeTable(d, &parsed.blocks[0], 0, 0, tableDeleteColumn)
	if d.tableCell == nil || len(d.tableCell.block.cellRanges[0]) != 1 || d.tableCell.text != "# 标题" {
		t.Fatalf("deleting a column exposed block syntax: %q", d.buf.String())
	}
	if !strings.Contains(d.buf.String(), "    内容") {
		t.Fatalf("removing the first column discarded body cell spacing: %q", d.buf.String())
	}
	d.undo()
	expectText(t, d, source)
}

func TestMarkdownTableMenuGuardsStaleAndLockedDocuments(t *testing.T) {
	const source = "Intro\n\n| A | B |\n| --- | --- |\n| left | right |\n"
	hs, d := editHarness(t, "stable.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("parsed table", func() bool { return hs.tt.HasText("right") })
	open := func() {
		cell := hs.detailBounds("right")
		hs.tt.RightClickAt(cell.X+cell.W/2, cell.Y+cell.H/2)
	}
	open()
	d.buf.Replace(d.buf.Len(), d.buf.Len(), "\nExternal note")
	hs.settle()
	if len(hs.tt.Menu()) > 0 {
		_ = hs.tt.ChooseMenuItem(locales.MarkdownTableDeleteRow())
	}
	hs.settle()
	expectText(t, d, source+"\nExternal note")
	local := d.path
	d.path = remote.Path("table-offline.invalid", "/stable.md")
	hs.settle()
	open()
	if err := hs.tt.ChooseMenuItem(locales.MarkdownTableInsertAbove()); err == nil {
		t.Fatal("offline table offered a writable structure command")
	}
	hs.tt.CloseMenu()
	d.path = local
	hs.settle()
	expectText(t, d, source+"\nExternal note")
}

func TestMarkdownTableMenuDoesNotRetargetToAnotherCell(t *testing.T) {
	const source = "前文\n\n| A | B |\n| --- | --- |\n| left | right |\n"
	hs, d := editHarness(t, "target.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("rendered table", func() bool { return hs.tt.HasText("right") })
	if err := hs.tt.Click("left"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if err := hs.tt.Click(locales.MarkdownTableActions()); err != nil {
		t.Fatal(err)
	}
	hs.a.beginLiveCell(d, &d.tableCell.block, 1, 1)
	hs.settle()
	if len(hs.tt.Menu()) != 0 {
		if err := hs.tt.ChooseMenuItem(locales.MarkdownTableDeleteColumn()); err != nil {
			t.Fatal(err)
		}
	}
	hs.settle()
	expectText(t, d, source)
}

func TestMarkdownTableEditOtherSplitKeepsEarlierSelection(t *testing.T) {
	const source = "Intro\n\n| A | B |\n| --- | --- |\n| left | right |\n"
	hs, left, right := splitHarness(t, func(hs *harness) string {
		path := hs.path("split.md")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	})
	hs.a.setLive(left, true)
	before := strings.Index(source, "right")
	hs.place(right, before, before+5)
	right.vscroll.Focus()
	hs.settle()
	cell := hs.detailBounds("right")
	hs.tt.RightClickAt(cell.X+cell.W/2, cell.Y+cell.H/2)
	if err := hs.tt.ChooseMenuItem(locales.MarkdownTableInsertRight()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.activeEditor() != left || left.tableCell == nil || left.tableCell.column != 2 {
		t.Fatalf("the clicked split lost focus: left active=%v, source=%q", hs.a.activeEditor() == left, left.buf.String())
	}
	after := strings.Index(right.buf.String(), "right")
	expectSel(t, right, after, after+5)
	if left.buf != right.buf || !strings.Contains(right.buf.String(), "| left | right |  |") {
		t.Fatal("the other split did not receive the new column")
	}
}

func TestMarkdownTableActiveCellShowsStructureButton(t *testing.T) {
	hs, d := editHarness(t, "actions.md", "before\n\n| 标题 | 名称 |\n| --- | --- |\n| 内容 | 状态 |\n\nafter")
	hs.a.setMarkdownMode("live")
	hs.until("table", func() bool { return hs.tt.HasText("内容") })
	if hs.tt.HasText(locales.MarkdownTableMenu()) {
		t.Fatal("the table button appeared before entering a cell")
	}
	if err := hs.tt.Click("状态"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if err := hs.tt.Click(locales.MarkdownTableActions()); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.ChooseMenuItem(locales.MarkdownTableInsertBelow()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if d.tableCell == nil || d.tableCell.row != 2 || d.tableCell.column != 1 {
		t.Fatalf("the table button lost the intended cell: %q", d.buf.String())
	}
	hs.key(0, ui.KeyEscape)
	if hs.tt.HasText(locales.MarkdownTableMenu()) {
		t.Fatal("the contextual button stayed after leaving the grid")
	}
}

func TestMarkdownTableHeaderBreadcrumbSequenceSavesValidGrid(t *testing.T) {
	const intro, end = "前文\n\n", "\n\n后文\n"
	const original = intro + "| 项目 | 状态 |\n| :--- | ---: |\n| 解析 | 正常 |" + end
	const wideHeader = "| 项目 |  | 状态 |\n| :--- | --- | ---: |\n"
	const wideBody = "| 解析 |  | 正常 |"
	const wide = intro + wideHeader + wideBody + end
	hs, d := editHarness(t, "sequence.md", original)
	hs.a.setMarkdownMode("live")
	hs.until("header", func() bool { return hs.tt.HasText("项目") })
	if err := hs.tt.Click("项目"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	for _, step := range []struct {
		action, want            string
		rows, columns, row, col int
	}{
		{locales.MarkdownTableInsertRight(), wide, 2, 3, 0, 1},
		{locales.MarkdownTableInsertBelow(), intro + wideHeader + "|  |  |  |\n" + wideBody + end, 3, 3, 1, 1},
		{locales.MarkdownTableDeleteRow(), wide, 2, 3, 1, 1},
		{locales.MarkdownTableDeleteColumn(), original, 2, 2, 1, 1},
	} {
		if err := hs.tt.Click(locales.MarkdownTableActions()); err != nil {
			t.Fatalf("%s: %v", step.action, err)
		}
		if err := hs.tt.ChooseMenuItem(step.action); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		if cell := d.tableCell; cell == nil || cell.row != step.row || cell.column != step.col {
			t.Fatalf("%s did not focus cell (%d, %d): %q", step.action, step.row, step.col, d.buf.String())
		}
		expectText(t, d, step.want)
		hs.a.run("file.save")
		hs.until("table saved", func() bool { return !d.dirty() })
		data, err := os.ReadFile(d.path)
		if err != nil || string(data) != step.want {
			t.Fatalf("%s saved %q, want %q (error %v)", step.action, data, step.want, err)
		}
		parsed := parseLive(string(data), d.path, "", true, &markdownCache{})
		if len(parsed.blocks) != 1 || len(parsed.blocks[0].cellRanges) != step.rows {
			t.Fatalf("%s did not save one table with %d rows: %q", step.action, step.rows, data)
		}
		for _, cells := range parsed.blocks[0].cellRanges {
			if len(cells) != step.columns {
				t.Fatalf("%s saved a row with %d columns, want %d", step.action, len(cells), step.columns)
			}
		}
	}
}

func TestMarkdownTablePromotedWideRowKeepsItsCells(t *testing.T) {
	const source = "前文\n\n| 旧 | 标题 |\n| :--- | ---: |\n| 新 | 标题二 | 原文第三列 |\n| 数据 | 项目 | 更多 |\n"
	hs, d := editHarness(t, "wide.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("table", func() bool { return hs.tt.HasText("旧") })
	cell := hs.detailBounds("旧")
	hs.tt.RightClickAt(cell.X+cell.W/2, cell.Y+cell.H/2)
	if err := hs.tt.ChooseMenuItem(locales.MarkdownTableDeleteRow()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	const want = "| 新 | 标题二 | 原文第三列 |\n| :--- | ---: | --- |\n| 数据 | 项目 | 更多 |"
	if !strings.Contains(d.buf.String(), want) {
		t.Fatalf("promoting a wide header lost its cells: %q", d.buf.String())
	}
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, source)
}

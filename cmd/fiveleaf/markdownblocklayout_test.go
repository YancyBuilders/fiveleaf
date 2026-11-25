package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestLiveTableKeepsFollowingTextBelowCells(t *testing.T) {
	const detail = "包含多组中文参数、可选字段和上线后的核查说明，continued English text "
	var text strings.Builder
	text.WriteString(strings.Repeat("文档前言\n\n", 165))
	for section := range 3 {
		fmt.Fprintf(&text, "## 第%d节\n\n| 风险 | 控制措施 |\n| --- | --- |\n", section)
		for row := range 17 {
			fmt.Fprintf(&text, "| 条目%d | %s |\n", row, strings.Repeat(detail, 2+row%2))
		}
		fmt.Fprintf(&text, "| 结束单元%d | 最后一行说明 |\n\n## 表格之后%d\n\n1. 保留列表\n2. 检查条目\n\n", section, section)
	}
	source := text.String()
	hs, ed := editHarness(t, "table-layout.md", source)
	hs.tt.SetSize(830, 760)
	hs.a.setMarkdownMode("live")
	hs.until("parsed tables", func() bool {
		live := ed.document.live
		return live != nil && !live.quick && live.doc != nil && len(live.doc.blocks) == 3
	})
	const label = "结束单元0"
	cellLabel := label
	heading := utf8.RuneCountInString(source[:strings.Index(source, "## 表格之后0")])
	snap := ed.buf.Snapshot()
	line := snap.LineAt(heading)
	var observations, scrolled int
	check := func() {
		t.Helper()
		table := &ed.document.live.doc.blocks[0]
		block := ed.liveViews[table.key]
		if block == nil || !block.measured || block.frame != hs.a.frame {
			return
		}
		first, last := snap.LineAt(table.start), snap.LineAt(table.end)
		if reserved := ed.lines.Top(last+1) - ed.lines.Top(first); reserved+1 < block.height+hs.a.settings.FontSize {
			t.Fatalf("rendered table needs %.1f plus margin, but source lines %d–%d reserve %.1f", block.height, first, last, reserved)
		}
		cell, present := hs.tt.Find(cellLabel)
		view := ed.lastView
		if !present || cell.H <= 0 || cell.Y+cell.H <= view.Y || cell.Y >= view.Y+view.H {
			return
		}
		_, cy, _, _ := ed.lines.Caret(heading)
		headingY := ed.liveAt.Y + editorPadY + cy - ed.vscroll.Y
		if cell.Y+cell.H > headingY+1 {
			t.Fatalf("last cell ends at %.1f, following heading starts at %.1f", cell.Y+cell.H, headingY)
		}
		observations++
		if block.at.Y < view.Y {
			scrolled++
		}
	}
	for _, width := range []int{830, 670, 1030} {
		hs.tt.SetSize(width, 760)
		for range 5 {
			ed.vscroll.Y = clamp(ed.lines.Top(line)-ed.lastView.H/2, 0, ed.vscroll.MaxY)
			hs.tt.Frame()
			check()
		}
		for _, delta := range []float32{-88, -50, 65, 113, 60, -100} {
			view := hs.detailBounds(locales.EditorLabel(ed.name()))
			hs.tt.Scroll(view.X+view.W/2, view.Y+view.H/2, 0, delta)
			check()
		}
	}
	if observations == 0 || scrolled == 0 {
		t.Fatalf("no visible end of a partially scrolled table: checked=%d scrolled=%d", observations, scrolled)
	}
	if err := hs.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	hs.tt.Type("已更新单元")
	hs.settle()
	expectText(t, ed, strings.Replace(source, label, "已更新单元", 1))
	hs.place(ed, heading, heading)
	cellLabel = "已更新单元"
	hs.until("edited table rendered again", func() bool { return hs.tt.HasText(cellLabel) })
	hs.settle()
	check()
}

func TestLiveBlockReservesSameHeightAtFirstLine(t *testing.T) {
	const rows = 24
	buf := ui.NewTextBuffer("前言\n| A | B |\n|---|---|\n" + strings.Repeat("| cell | row |\n", rows) + "\n## Heading\n")
	var lines ui.TextLines
	var scroll ui.ScrollState
	block := ui.TextBlock{First: 1, Last: 2 + rows}
	var header, tail float32
	captured, live := false, false
	tester := ui.NewTester(func(c *ui.Context) {
		area := ui.TextAreaBufferBase(c, buf).Fill().FixedLineHeight(20).TrackLines(&lines).TrackScroll(&scroll)
		if live {
			reserveLiveBlock(area, &lines, block)
			if !captured {
				captured = true
				header = lines.Top(block.First+1) - lines.Top(block.First)
				tail = lines.Top(block.Last+1) - lines.Top(block.First+1)
			}
		}
	}, 360, 150)
	scroll.Y = lines.Top(block.Last) - 60
	tester.Frame()
	block.Height = lines.Top(block.Last+1) - lines.Top(block.First)
	if lines.At(scroll.Y) <= block.First || block.Height <= 150 {
		t.Fatal("table header must be above the viewport in source mode")
	}
	live = true
	tester.Frame()
	if !captured || !near(header, block.Height) || tail > 0.5 {
		t.Fatalf("first rendered frame reserved %.1f at the header and %.1f in its tail; want %.1f at the header", header, tail, block.Height)
	}
}

func TestLiveTableWrapsAtEachSplitWidth(t *testing.T) {
	const detail = "检查每个任务的预计时长和执行说明，核对最近一次更新的时间。"
	source := "预览\n\n| Name | Description |\n| --- | --- |\n" + strings.Repeat("| Test | "+detail+detail+" |\n", 8) + "\n## 文末\n"
	hs, left, right := splitHarness(t, func(hs *harness) string {
		path := hs.path("two-widths.md")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	})
	hs.tt.SetSize(1440, 850)
	hs.a.cols[0].weight, hs.a.cols[1].weight = 0.3, 0.7
	hs.a.setLive(left, true)
	hs.a.setLive(right, true)
	hs.until("table measured at both widths", func() bool {
		live := left.document.live
		if live == nil || live.quick || live.doc == nil || len(live.doc.blocks) != 1 {
			return false
		}
		key := live.doc.blocks[0].key
		narrow, wide := left.liveViews[key], right.liveViews[key]
		return narrow != nil && wide != nil && narrow.measured && wide.measured
	})
	hs.settle()
	key := left.document.live.doc.blocks[0].key
	narrow, wide := left.liveViews[key], right.liveViews[key]
	if narrow.at.W >= wide.at.W || narrow.height <= wide.height+10 {
		t.Fatalf("unexpected layout for narrow and wide tables: %v and %v", narrow.at, wide.at)
	}
	for _, ed := range []*editor{left, right} {
		block := ed.liveViews[key]
		height := ed.lines.Top(12) - ed.lines.Top(2)
		if height < block.height || height > block.height+2*hs.a.settings.FontSize {
			t.Fatalf("pane %.1f reserves %.1f for its %.1f table", block.at.W, height, block.height)
		}
	}
}

func TestLiveTableKeepsGridHeightWhenWidened(t *testing.T) {
	url := "https://example.com/" + strings.Repeat("long-path-segment/", 45)
	source := "前言\n\n| 章节 | 链接 |\n| --- | --- |\n" + strings.Repeat("| 条目 | [短]("+url+") |\n", 6) + "\n## 下节\n"
	hs, ed := editHarness(t, "links.md", source)
	hs.tt.SetSize(600, 800)
	hs.a.setMarkdownMode("live")
	hs.until("rendered links", func() bool { return hs.tt.HasText("条目") && ed.document.live != nil && !ed.document.live.quick })
	hs.settle()
	key := ed.document.live.doc.blocks[0].key
	at := utf8.RuneCountInString(source[:strings.LastIndex(source, "[短]")])
	heading := utf8.RuneCountInString(source[:strings.Index(source, "## 下节")])
	block := ed.liveViews[key]
	checkGrid := func() {
		t.Helper()
		hs.place(ed, at, at)
		hs.settle()
		if block == nil || block.frame != hs.a.frame {
			t.Fatal("moving to the link in the cell hid its grid")
		}
		room := ed.lines.Top(10) - ed.lines.Top(2)
		if room < block.height || room > block.height+hs.a.settings.FontSize+1 {
			t.Fatalf("rendered table needs %.1f plus margin, reserves %.1f", block.height, room)
		}
		if rest := ed.lines.Top(10) - ed.lines.Top(3); rest > 1 {
			t.Fatalf("the source rows still occupy %.1f below the grid", rest)
		}
		hs.place(ed, heading, heading)
		hs.settle()
	}
	checkGrid()
	hs.tt.SetSize(1440, 800)
	checkGrid()
	hs.a.settings.LineHeight = 1.1
	checkGrid()
}

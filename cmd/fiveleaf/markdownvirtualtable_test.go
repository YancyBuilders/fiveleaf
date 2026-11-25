package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func markdownGridFixture(rows, columns int) string {
	var source strings.Builder
	source.WriteString("前言\n\n")
	cells := make([]string, columns)
	writeRow := func() { source.WriteString("| " + strings.Join(cells, " | ") + " |\n") }
	for index := range cells {
		cells[index] = "标题" + strconv.Itoa(index+1)
	}
	writeRow()
	for index := range cells {
		cells[index] = "---"
	}
	writeRow()
	for row := 1; row < rows; row++ {
		for column := range cells {
			cells[column] = strconv.Itoa((row + column) % 10)
		}
		switch row {
		case 1:
			cells[0] = "首行"
		case rows - 1:
			cells[0] = "末行"
		}
		writeRow()
	}
	source.WriteString("\n## 表格之后\n\n" + strings.Repeat("后续段落\n\n", 40))
	return source.String()
}

func TestLiveTableVirtualRowsKeepFocusAndHitTargets(t *testing.T) {
	const totalRows = 100
	source := strings.Replace(markdownGridFixture(totalRows, 20), "| 9 | 0 |", "| 很长的参数名与说明需要在单元格中换行 | 0 |", 1)
	hs, ed := editHarness(t, "rows.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("first grid row", func() bool { return hs.tt.HasText("首行") })
	hs.settle()
	block := &ed.document.live.doc.blocks[0]
	view := ed.liveViews[block.key]
	if view.rows == nil || len(view.rows.mounted) >= totalRows/2 || hs.tt.HasText("末行") {
		t.Fatal("the initial viewport built distant table rows")
	}
	if err := hs.tt.Click("首行"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	first, last := ed.buf.Snapshot().LineAt(block.start), ed.buf.Snapshot().LineAt(block.end)
	for _, width := range []int{1200, 830} {
		hs.tt.SetSize(width, 760)
		ed.vscroll.Y = clamp(ed.lines.Top(last+1)-ed.lastView.H/2, 0, ed.vscroll.MaxY)
		hs.tt.Frame()
		hs.settle()
		if !hs.tt.Focused(locales.MarkdownTableCell("2", "1")) || !slices.Contains(view.rows.mounted, 1) || len(view.rows.mounted) >= totalRows/2 {
			t.Fatalf("the edited row lost its input or the row window grew: %v", view.rows.mounted)
		}
		cell, found := hs.tt.Find("末行")
		if !found || cell.Y < ed.lastView.Y || cell.Y+cell.H > ed.lastView.Y+ed.lastView.H {
			t.Fatalf("the final cell disappeared after scrolling at width %d: %+v, viewport %+v", width, cell, ed.lastView)
		}
		reserved := ed.lines.Top(last+1) - ed.lines.Top(first)
		if reserved+1 < view.height+hs.a.settings.FontSize {
			t.Fatalf("%.1f DIPs reserved for %.1f DIPs of table", reserved, view.height)
		}
	}
	ed.vscroll.Y = ed.vscroll.MaxY
	hs.tt.Frame()
	hs.settle()
	if !hs.tt.Focused(locales.MarkdownTableCell("2", "1")) || !slices.Equal(view.rows.mounted, []int{1}) {
		t.Fatalf("scrolling past the table dropped its active edit or mounted extra rows: %v", view.rows.mounted)
	}
	ed.vscroll.Y = clamp(ed.lines.Top(last+1)-ed.lastView.H/2, 0, ed.vscroll.MaxY)
	hs.tt.Frame()
	hs.settle()
	if err := hs.tt.Click("末行"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if ed.tableCell == nil || ed.tableCell.row != totalRows-1 {
		t.Fatal("a click on the final row hit the wrong source cell")
	}
	for range 8 {
		hs.tt.Key(ui.Shift, ui.KeyEnter)
	}
	hs.settle()
	if input := hs.detailBounds(locales.MarkdownTableCell("92", "1")); input.Y < ed.lastView.Y-1 || input.Y+input.H > ed.lastView.Y+ed.lastView.H+1 {
		t.Fatalf("keyboard navigation put the focused cell outside the viewport: %+v / %+v", input, ed.lastView)
	}
	for range 8 {
		hs.tt.Key(0, ui.KeyEnter)
	}
	hs.settle()
	hs.tt.Type("已更新")
	hs.settle()
	expectText(t, ed, strings.Replace(source, "末行", "已更新", 1))
}

func BenchmarkLiveTableWheel(b *testing.B) {
	for _, columns := range []int{20, 100} {
		b.Run(fmt.Sprintf("100x%d", columns), func(b *testing.B) {
			hs := newHarness(b, false)
			path := hs.path("scroll.md")
			if err := os.WriteFile(path, []byte(markdownGridFixture(100, columns)), 0o600); err != nil {
				b.Fatal(err)
			}
			hs.a.settings.MarkdownMode = "live"
			hs.a.openFile(path, true)
			hs.until("first grid row", func() bool { return hs.tt.HasText("首行") })
			if err := hs.tt.Click("首行"); err != nil {
				b.Fatal(err)
			}
			hs.settle()
			view := hs.a.activeEditor().lastView
			x, y := view.X+view.W/2, view.Y+view.H/2
			delta := float32(44)
			b.ReportAllocs()
			for b.Loop() {
				hs.tt.Scroll(x, y, 0, delta)
				delta = -delta
			}
		})
	}
}

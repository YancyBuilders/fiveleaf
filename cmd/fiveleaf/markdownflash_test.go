package main

import (
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func markdownPixelInk(frame *image.RGBA, bounds ui.Rect) float64 {
	var ink float64
	for y := int(bounds.Y); y < int(bounds.Y+bounds.H); y++ {
		background := frame.RGBAAt(int(bounds.X)-1, y)
		for x := int(bounds.X); x < int(bounds.X+bounds.W); x++ {
			ink += float64(caretTrailDifference(frame.RGBAAt(x, y), background))
		}
	}
	return ink
}

func TestLiveTypingKeepsRenderedTableVisible(t *testing.T) {
	const cell = "始终可见的文字"
	const source = "# 稳定标题\n\n正在输入\n\n| 状态 | 说明 |\n| --- | --- |\n| 第一行 | " + cell + " |\n\n后续段落\n"
	hs, ed := editHarness(t, "stable.md", source)
	hs.a.setMarkdownMode("live")
	hs.until("rendered table", func() bool { return hs.tt.HasText(cell) })
	at := len([]rune("# 稳定标题\n\n正在输入"))
	hs.place(ed, at, at)
	hs.settle()
	bounds := hs.detailBounds(cell)
	before := markdownPixelInk(hs.tt.Image(), bounds)
	if before < 1000 {
		t.Fatalf("reference cell has too little text contrast: %.1f", before)
	}
	origin := caretTrailOrigin(t, hs, ed)
	x0, rowY, rowHeight, _ := ed.lines.Caret(2)
	x1, _, _, _ := ed.lines.Caret(len([]rune("# 稳定标题")))
	headingBox := ui.Rect{X: origin.X + x0, Y: origin.Y + rowY, W: x1 - x0, H: rowHeight}
	headingInk := markdownPixelInk(hs.tt.Image(), headingBox)
	if headingInk < 1000 {
		t.Fatal("reference heading is missing")
	}
	minimum := 1.0
	check := func(phase string) {
		t.Helper()
		box, ok := hs.tt.Find(cell)
		if !ok {
			t.Fatalf("%s: unchanged table cell disappeared", phase)
		}
		now := markdownPixelInk(hs.tt.Image(), box)
		minimum = min(minimum, now/before)
		if now < before*0.9 {
			t.Fatalf("%s: unchanged table text retained %.1f%% of its original ink", phase, 100*now/before)
		}
		if now := markdownPixelInk(hs.tt.Image(), headingBox); now < headingInk*0.9 {
			t.Fatalf("%s: unchanged heading retained %.1f%% of its ink", phase, 100*now/headingInk)
		}
	}
	awaitReparse := func() {
		t.Helper()
		for start := time.Now(); ; {
			hs.tt.Frame()
			check("reparse")
			if ed.document.live.doc.snap == ed.buf.Snapshot() {
				return
			}
			if time.Since(start) > settleTimeout {
				t.Fatal("document did not reparse")
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	for _, value := range []string{"x", "y", "中文"} {
		if value == "中文" {
			hs.tt.Compose("zhong", 5)
			check("IME composition")
		}
		hs.tt.Type(value)
		check("input")
		awaitReparse()
	}
	if err := hs.tt.Click("第一行"); err != nil {
		t.Fatal(err)
	}
	hs.tt.Type("更新行")
	check("editing sibling cell")
	awaitReparse()
	t.Logf("minimum unchanged cell ink across edits was %.1f%% of the settled frame", minimum*100)
}

func TestLiveTableKeysFollowTheirTables(t *testing.T) {
	const original = "| 状态 | 说明 |\n|---|---|\n| 第一行 | B |\n"
	const added = "| 新表 | 说明 |\n|---|---|\n| C | D |\n"
	beforeText := "旧文章\n\n" + original + "\n分界\n\n" + original
	var next uint64
	before := liveParse(beforeText)
	before.keepBlockKeys(nil, &next)
	first, second := before.blocks[0].key, before.blocks[1].key
	if first == second {
		t.Fatal("duplicate source tables share an identity")
	}
	prependedText := "前置段落\n\n" + added + "\n" + beforeText
	prepended := liveParse(prependedText)
	prepended.keepBlockKeys(before, &next)
	fresh := prepended.blocks[0].key
	if fresh == first || fresh == second || prepended.blocks[1].key != first || prepended.blocks[2].key != second {
		t.Fatal("inserting a table reassigned existing table identities")
	}
	modifiedText := strings.Replace(prependedText, "第一行", "更长的名称", 1)
	modified := liveParse(modifiedText)
	modified.keepBlockKeys(prepended, &next)
	for index, block := range prepended.blocks {
		if modified.blocks[index].key != block.key {
			t.Fatalf("editing a cell remounted table %d", index)
		}
	}
	removed := liveParse(strings.Replace(modifiedText, added+"\n", "", 1))
	removed.keepBlockKeys(modified, &next)
	if removed.blocks[0].key != first || removed.blocks[1].key != second {
		t.Fatal("deleting a table reassigned its identity to a neighbor")
	}
}

func TestLivePeerKeepsTableWhileItsSourceReparses(t *testing.T) {
	const unchanged = "一直可见"
	const source = "普通正文\n\n| 列一 | 列二 |\n| --- | --- |\n| 旧值 | " + unchanged + " |\n\n下一段\n"
	hs, left, right := markdownScrollSplit(t, source, "普通正文")
	hs.settle()
	body := hs.detailBounds(unchanged)
	before := markdownPixelInk(hs.tt.Image(), body)
	at := len([]rune(source[:strings.Index(source, "旧值")]))
	hs.place(left, at, at+2)
	hs.tt.Type("新值")
	for start := time.Now(); ; {
		cell, ok := hs.tt.Find(unchanged)
		if !ok || markdownPixelInk(hs.tt.Image(), cell) < before*0.9 {
			t.Fatal("source typing temporarily replaced the unchanged rendered cells with Markdown pipes or a blank table")
		}
		if right.document.live.doc.snap == right.buf.Snapshot() {
			break
		}
		if time.Since(start) > settleTimeout {
			t.Fatal("source edit did not reparse")
		}
		time.Sleep(2 * time.Millisecond)
		hs.tt.Frame()
	}
	if !hs.tt.HasText("新值") {
		t.Fatal("rendered cell did not receive the fresh source content")
	}
}

func TestLivePeerKeepsDiagramAndMathWhileTyping(t *testing.T) {
	for _, tc := range []struct{ name, definition, old, new string }{
		{"mermaid", "```mermaid\nflowchart LR\nA[First] --> B[Second]\n```", "First", "Third"},
		{"formula", "$$\nx^2+y^2\n$$", "x^2", "z^3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "上方正文\n\n" + tc.definition + "\n\n下一段\n"
			hs, left, right := markdownScrollSplit(t, source, "上方正文")
			hs.until("ready rendered block", func() bool { return len(right.liveViews) == 1 })
			hs.settle()
			initial := right.document.live.doc.blocks[0]
			view := right.liveViews[initial.key]
			box := view.at
			if tc.name == "mermaid" {
				box = hs.detailBounds("mermaid")
			}
			before := markdownPixelInk(hs.tt.Image(), box)
			if !view.measured || before < 1000 {
				t.Fatalf("no reference drawing, height %.1f contrast %.1f", box.H, before)
			}
			caret := len([]rune(source[:strings.Index(source, tc.old)]))
			hs.place(left, caret, caret+len([]rune(tc.old)))
			hs.tt.Type(tc.new)
			if tc.name == "mermaid" {
				if !view.v.markdown.stale {
					t.Fatal("expected the previous diagram while parsing the edit")
				}
				x, y := box.X+box.W/2, box.Y+box.H/2
				hs.tt.Move(x, y)
				if hs.tt.HasText(locales.OpenDiagram()) {
					t.Fatal("stale diagram exposed its toolbar")
				}
				hs.tt.ClickAt(x, y)
				hs.tt.ClickAt(x, y)
				if hs.a.diagram != nil || right.pending != nil {
					t.Fatal("stale diagram click opened an overlay or jumped to obsolete source")
				}
			}
			for start := time.Now(); ; {
				seen := false
				right.document.live.mapped.eachBlock(0, right.buf.Len(), func(block *liveBlock) {
					if block.key == initial.key {
						seen = true
					}
				})
				if !seen || view.frame != hs.a.frame {
					t.Fatal("rendered block disappeared while its definition was being edited")
				}
				ink := markdownPixelInk(hs.tt.Image(), box)
				if ink < before*0.65 {
					t.Fatalf("drawing lost contrast: %.1f%% of reference", 100*ink/before)
				}
				if right.document.live.doc.snap == right.buf.Snapshot() {
					break
				}
				if time.Since(start) > settleTimeout {
					t.Fatal("definition did not reparse")
				}
				time.Sleep(2 * time.Millisecond)
				hs.tt.Frame()
			}
			current := right.document.live.doc.blocks[0]
			if current.key != initial.key || !strings.Contains(current.source, tc.new) || right.liveViews[current.key] != view || &current.blocks[0] == &initial.blocks[0] {
				t.Fatalf("updated definition did not replace drawing in place: %q", current.source)
			}
			hs.a.activate(hs.a.cols[0], 0, true)
			left.vscroll.Focus()
			hs.settle()
			firstWord := strings.Index(source, tc.definition)
			start := len([]rune(source[:firstWord]))
			end := start + len([]rune(current.source))
			hs.place(left, start, end)
			hs.tt.Type("普通正文")
			hs.until("removed rendered block", func() bool {
				return right.document.live.doc.snap == right.buf.Snapshot() && len(right.document.live.doc.blocks) == 0
			})
			hs.settle()
			if view.frame == hs.a.frame {
				t.Fatal("removed definition is still drawn as an overlay")
			}
		})
	}
}

func TestLiveSplitKeepsTableBetweenTwoSourceEdits(t *testing.T) {
	const copied = "| 状态 | 说明 |\n| --- | --- |\n| 完成 | 未修改 |\n"
	source := "上方 A\n\n" + copied + "\n下方 B\n\n" + copied + "\n末尾 C\n"
	hs, left, right := markdownScrollSplit(t, source, "上方")
	hs.settle()
	snap := left.buf.Snapshot()
	previous := right.document.live.doc
	tail := snap.Len() - 2
	left.edit([2]int{1, 1}, textEdit{1, 1, "中"}, textEdit{tail, tail, "文"})
	hs.tt.Frame()
	for start := time.Now(); ; {
		index := 0
		right.document.live.mapped.eachBlock(0, right.buf.Len(), func(block *liveBlock) {
			if index >= len(previous.blocks) || block.key != previous.blocks[index].key {
				t.Fatalf("duplicate table identities changed around disjoint edits")
			}
			index++
		})
		if index != 2 {
			t.Fatalf("%d unchanged tables remained visible between disjoint source edits, want 2", index)
		}
		if right.document.live.doc.snap == right.buf.Snapshot() {
			break
		}
		if time.Since(start) > settleTimeout {
			t.Fatal("did not reparse")
		}
		time.Sleep(2 * time.Millisecond)
		hs.tt.Frame()
	}
	var next uint64 = 50
	ambiguous := liveParse("改写后的正文\n\n" + copied + "\n" + copied + "\n" + copied + "\n新的结尾")
	ambiguous.keepBlockKeys(previous, &next)
	for _, block := range ambiguous.blocks {
		if slices.ContainsFunc(previous.blocks, func(old liveBlock) bool { return old.key == block.key }) {
			t.Fatal("an ambiguous duplicate inherited another table's controls")
		}
	}
	embeddedText := liveParse("改写 " + copied + "普通正文结束")
	pending := previous.mapTo(embeddedText.snap)
	pending.eachBlock(0, embeddedText.snap.Len(), func(block *liveBlock) { t.Fatal("text embedded in a different source line retained a stale table") })
}

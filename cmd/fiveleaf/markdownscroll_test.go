package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func markdownScrollFixture() string {
	var source strings.Builder
	for section := range 42 {
		fmt.Fprintf(&source, "## Section %d\n\n", section)
		source.WriteString("中英文说明 Overview；本段落用于定位长文档中的标题。\n\n| 任务 | 说明 |\n|---|---|\n")
		for row := range 7 {
			fmt.Fprintf(&source, "| 任务 %d.%d | %s |\n", section, row, strings.Repeat("中英文混排的表格内容，preview and scroll。", 2))
		}
		if section%4 == 0 {
			source.WriteString("\n```mermaid\ngraph LR\nA-->B\n```\n")
		}
		source.WriteString("\n列表与后续段落\n\n1. 保持位置\n2. 编辑另一栏\n\n")
	}
	return source.String()
}

func markdownScrollDocument(t testing.TB) (source, marker string) {
	t.Helper()
	if path := os.Getenv("FIVELEAF_MEASURE_SCROLL_MD"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if title := os.Getenv("FIVELEAF_SCROLL_HEADING"); title != "" {
			return string(data), title
		}
		return string(data), "## 14."
	}
	return markdownScrollFixture(), "## Section 28\n"
}

func markdownScrollSplit(t testing.TB, source, marker string) (*harness, *editor, *editor) {
	t.Helper()
	hs := newHarness(t, true)
	path := hs.path("scrolling.md")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	hs.a.run("view.splitRight")
	hs.settle()
	left, right := hs.a.cols[0].tab().ed, hs.a.cols[1].tab().ed
	hs.a.setLive(left, false)
	hs.a.setLive(right, true)
	hs.a.activate(hs.a.cols[0], 0, true)
	hs.place(left, 0, 0)
	left.vscroll.Focus()
	hs.until("independent source and live preview", func() bool {
		return hs.a.activeEditor() == left && right.document.live != nil && !right.document.live.quick && right.document.live.doc != nil
	})
	markerIndex := strings.Index(source, marker)
	if markerIndex < 0 {
		t.Fatalf("section %q is missing", marker)
	}
	line := strings.Count(source[:markerIndex], "\n")
	for range 9 {
		right.vscroll.Y = clamp(right.lines.Top(line)-95, 0, right.vscroll.MaxY)
		hs.tt.Frame()
	}
	hs.settle()
	return hs, left, right
}

func scrollMarkerY(t testing.TB, d *editor, marker string) float32 {
	t.Helper()
	source := d.buf.String()
	index := strings.Index(source, marker)
	if index < 0 {
		t.Fatal("missing marker")
	}
	at := utf8.RuneCountInString(source[:index])
	_, y, _, ok := d.lines.Caret(at)
	if !ok {
		t.Fatal("no marker caret")
	}
	return d.liveAt.Y + editorPadY + y - d.vscroll.Y
}

func TestLiveSplitKeepsSectionThroughSourceEdits(t *testing.T) {
	source, marker := markdownScrollDocument(t)
	hs, _, right := markdownScrollSplit(t, source, marker)
	start := scrollMarkerY(t, right, marker)
	if start < right.lastView.Y || start > right.lastView.Y+right.lastView.H {
		t.Fatalf("marker not initially in view: %.2f, %v", start, right.lastView)
	}
	maxDrift := 0.0
	check := func() { maxDrift = max(maxDrift, math.Abs(float64(scrollMarkerY(t, right, marker)-start))) }
	for _, text := range []string{"a", "b", "中文", "\n", "\n", "早先的标题\n", "d"} {
		hs.tt.Type(text)
		check()
	}
	hs.until("source edits parsed", func() bool { return right.document.live.doc.snap.Version() == right.buf.Version() })
	check()
	hs.settle()
	check()
	t.Logf("max section drift across typing and reparse: %.2f px", maxDrift)
	if maxDrift > 2 {
		t.Fatalf("section moved %.2f px while editing the source", maxDrift)
	}
	x, y := right.lastView.X+right.lastView.W/2, right.lastView.Y+right.lastView.H/2
	hs.tt.Scroll(x, y, 0, 85)
	hs.settle()
	afterWheel := scrollMarkerY(t, right, marker)
	if math.Abs(float64(start-afterWheel-85)) > 2 {
		t.Fatalf("85px wheel moved the preview %.2fpx", start-afterWheel)
	}
	hs.tt.Type("typing again")
	hs.settle()
	if now := scrollMarkerY(t, right, marker); !near(now, afterWheel) {
		t.Fatalf("typing moved the wheeled position %.2f → %.2f", afterWheel, now)
	}
	left := hs.a.cols[0].tab().ed
	maxY := right.vscroll.MaxY
	view := right.lastView
	thumb := max(24, (view.H-8)*view.H/(maxY+view.H))
	x = view.X + view.W - 8
	y = view.Y + 4 + (view.H-8-thumb)*right.vscroll.Y/maxY + thumb/2
	hs.tt.Move(x, y)
	before := right.liveViewport.at
	hs.tt.Press(x, y)
	hs.tt.Move(x, y-64)
	hs.tt.Release(x, y-64)
	hs.settle()
	dragged := right.liveViewport
	if abs(before-dragged.at) < 10 {
		t.Fatalf("thumb drag did not move content: rune %d → %d", before, dragged.at)
	}
	hs.a.activate(hs.a.cols[0], 0, true)
	left.vscroll.Focus()
	hs.settle()
	beforeText := left.buf.Snapshot()
	hs.tt.Type("x")
	hs.settle()
	if right.liveViewport.at != dragged.at+1 || !near(right.liveViewport.offset, dragged.offset) {
		t.Fatalf("thumb-selected position was replaced after typing: %d/%.1f → %d/%.1f", dragged.at, dragged.offset, right.liveViewport.at, right.liveViewport.offset)
	}
	hs.key(ui.Cmd, ui.KeyZ)
	hs.settle()
	afterUndo := left.buf.Snapshot()
	expected := dragged.at + afterUndo.Len() - beforeText.Len()
	if right.liveViewport.at != expected || !near(right.liveViewport.offset, dragged.offset) {
		t.Fatalf("undo lost the viewport: %d/%.1f → %d/%.1f", dragged.at, dragged.offset, right.liveViewport.at, right.liveViewport.offset)
	}
	hs.tt.SetSize(1050, 750)
	hs.settle()
	resized := right.liveViewport
	hs.place(left, left.buf.Len(), left.buf.Len())
	hs.tt.Type("\n文末更新。")
	hs.settle()
	if right.liveViewport.at != resized.at || !near(right.liveViewport.offset, resized.offset) {
		t.Fatalf("edit below the resized pane moved its viewport: %d/%.1f → %d/%.1f", resized.at, resized.offset, right.liveViewport.at, right.liveViewport.offset)
	}
}

func BenchmarkLiveSplitInput(b *testing.B) {
	source, marker := markdownScrollDocument(b)
	for _, action := range []string{"wheel", "source-typing"} {
		b.Run(action, func(b *testing.B) {
			hs, _, right := markdownScrollSplit(b, source, marker)
			x, y := right.lastView.X+right.lastView.W/2, right.lastView.Y+right.lastView.H/2
			b.ReportAllocs()
			b.ResetTimer()
			builds := hs.a.builds
			var peak time.Duration
			for index := range b.N {
				start := time.Now()
				switch action {
				case "wheel":
					dy := float32(44)
					if index/8%2 == 1 {
						dy = -dy
					}
					hs.tt.Scroll(x, y, 0, dy)
				case "source-typing":
					hs.tt.Type("x")
				}
				peak = max(peak, time.Since(start))
			}
			b.StopTimer()
			b.ReportMetric(float64(hs.a.builds-builds)/float64(b.N), "builds/op")
			b.ReportMetric(float64(peak.Microseconds())/1000, "peak-ms")
		})
	}
}

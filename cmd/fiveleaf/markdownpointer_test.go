package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func livePointerTestPoint(t *testing.T, hs *harness, d *editor, at int) ui.Point {
	t.Helper()
	x, y, height, ok := d.lines.Caret(at)
	if !ok {
		t.Fatalf("no caret at %d", at)
	}
	origin := caretTrailOrigin(t, hs, d)
	return ui.Point{X: origin.X + x, Y: origin.Y + y + height/2}
}

func TestLivePointerUsesRenderedLayout(t *testing.T) {
	const source = "引言\n\n# Mixed 标题文字\n\n- alpha bravo charlie\n- [ ] 更新编辑体验\n\n段落中含有 **粗体** 和 `code` 以及尾部\n"
	for _, test := range []struct{ name, first, after string }{
		{"bullet", "alpha", " charlie"},
		{"task", "更新", "体验"},
		{"heading", "Mixed", "文字"},
		{"inline", "段落", "尾部"},
		{"backward", "尾部", "中含有"},
		{"multiline", "bravo", "编辑"},
	} {
		for _, shift := range []bool{false, true} {
			gesture := "drag"
			if shift {
				gesture = "shift-click"
			}
			t.Run(test.name+"/"+gesture, func(t *testing.T) {
				hs, d := editHarness(t, "pointer.md", source)
				hs.a.setMarkdownMode("live")
				hs.place(d, 0, 0)
				hs.until("styled Markdown", func() bool { return d.document.live != nil && d.document.live.doc != nil && !d.document.live.quick })
				start := utf8.RuneCountInString(source[:strings.Index(source, test.first)])
				end := utf8.RuneCountInString(source[:strings.Index(source, test.after)])
				from := livePointerTestPoint(t, hs, d, start)
				if shift {
					hs.tt.ClickAt(from.X, from.Y)
				} else {
					hs.tt.Press(from.X, from.Y)
				}
				to := livePointerTestPoint(t, hs, d, end)
				if shift {
					hs.tt.ClickAtWith(ui.Shift, to.X, to.Y)
				} else {
					hs.tt.Move(to.X, to.Y)
					expectSel(t, d, start, end)
					hs.tt.Release(to.X, to.Y)
				}
				hs.settle()
				expectSel(t, d, start, end)
				hs.tt.Type("替换")
				hs.settle()
				runes := []rune(source)
				expectText(t, d, string(runes[:min(start, end)])+"替换"+string(runes[max(start, end):]))
			})
		}
	}
}

func TestLivePointerWrappedGraphemes(t *testing.T) {
	const cluster = "👩‍💻"
	source := "起点\n\n- " + strings.Repeat("中英 mixed ", 26) + cluster + " 后面\n"
	hs, d := editHarness(t, "wrap.md", source)
	hs.tt.SetSize(730, 780)
	hs.a.setMarkdownMode("live")
	hs.place(d, 0, 0)
	hs.until("wrapped Markdown", func() bool { return len(d.lines.Rows(2)) > 3 && d.document.live != nil && !d.document.live.quick })
	start := utf8.RuneCountInString(source[:strings.Index(source, "中英")]) + 1
	clusterStart := utf8.RuneCountInString(source[:strings.Index(source, cluster)])
	clusterEnd := clusterStart + utf8.RuneCountInString(cluster)
	from := livePointerTestPoint(t, hs, d, start)
	hs.tt.Press(from.X, from.Y)
	left, right := livePointerTestPoint(t, hs, d, clusterStart), livePointerTestPoint(t, hs, d, clusterEnd)
	if right.Y != left.Y || right.X <= left.X || left.Y <= from.Y {
		t.Fatalf("expected the emoji on a later wrapped row, got from=%v, emoji=%v to %v", from, left, right)
	}
	for _, fraction := range []float32{0.7, 0.3} {
		hs.tt.Move(left.X+(right.X-left.X)*fraction, left.Y)
		end := clusterStart
		if fraction > 0.5 {
			end = clusterEnd
		}
		hs.tt.Advance(motionHover)
		expectSel(t, d, start, end)
	}
	hs.tt.Release(left.X+(right.X-left.X)*0.3, left.Y)
	expectSel(t, d, start, clusterStart)
	hs.place(d, clusterEnd, clusterEnd)
	from = livePointerTestPoint(t, hs, d, clusterEnd)
	hs.tt.Press(from.X, from.Y)
	to := livePointerTestPoint(t, hs, d, start)
	hs.tt.Move(to.X, to.Y)
	hs.tt.Release(to.X, to.Y)
	expectSel(t, d, clusterEnd, start)
	hs.command("copy")
	if got, want := hs.tt.Clipboard(), d.buf.Snapshot().Slice(start, clusterEnd); got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

func TestLivePointerKeepsBidiSelection(t *testing.T) {
	const source = "abc שלום def\n\n- ordinary item\n"
	hs, d := editHarness(t, "bidi.md", source)
	hs.a.setMarkdownMode("live")
	hs.place(d, 0, 0)
	from := livePointerTestPoint(t, hs, d, 1)
	to := livePointerTestPoint(t, hs, d, 6)
	hs.tt.Press(from.X, from.Y)
	hs.tt.Move(to.X, to.Y)
	if !d.liveSelection.active || d.liveSelection.end >= 0 {
		t.Fatal("the native bidirectional selection was overridden")
	}
	hs.tt.Release(to.X, to.Y)
	hs.command("copy")
	if got, want := hs.tt.Clipboard(), "bc ום"; got != want {
		t.Fatalf("copied visual range %q, want %q", got, want)
	}
}

func TestLivePointerUnicodeBidiEndpoints(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"override", "\u202eabc\u202c"},
		{"isolate", "\u2068abc\u2069"},
		{"direction mark", "\u200e"},
		{"Adlam", "\U0001e900\U0001e901"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			const prefix = "plain line\n\n"
			hs, d := editHarness(t, "unicode.md", prefix+"abc "+sample.text+" tail\n")
			hs.a.setMarkdownMode("live")
			plain, bidi := 2, len(prefix)+2
			for _, gesture := range [][2]int{{plain, bidi}, {bidi, plain}} {
				hs.place(d, gesture[0], gesture[0])
				from := livePointerTestPoint(t, hs, d, gesture[0])
				to := livePointerTestPoint(t, hs, d, gesture[1])
				hs.tt.Press(from.X, from.Y)
				hs.tt.Move(to.X, to.Y)
				if !d.liveSelection.active || d.liveSelection.end >= 0 {
					t.Errorf("drag %v replaced the native bidirectional selection", gesture)
				}
				hs.tt.Release(to.X, to.Y)
			}
		})
	}
}

func TestLivePointerAnchorFollowsBidiEdits(t *testing.T) {
	buf := ui.NewTextBuffer("\u202eabc\u202c\nplain")
	var hit livePointerSelection
	if !hit.nativeAnchor(buf.Snapshot(), 0) || hit.nativeAnchor(buf.Snapshot(), 1) || !hit.nativeAnchor(buf.Snapshot(), 0) {
		t.Fatal("anchor fallback did not follow the selected paragraph")
	}
	buf.Replace(0, 5, "abcde")
	if hit.nativeAnchor(buf.Snapshot(), 0) {
		t.Fatal("old bidi direction survived an edit at the anchor")
	}
}

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func semanticPointRune(t testing.TB, d *editor, at int, fraction float32) (float32, float32) {
	t.Helper()
	x0, y, h, ok := d.lines.Caret(at)
	x1, y1, _, ok1 := d.lines.Caret(at + 1)
	if !ok || !ok1 || y1 != y || x1 <= x0 {
		t.Fatalf("rune %d does not occupy one row: (%v,%v)-(%v,%v)", at, x0, y, x1, y1)
	}
	return d.textX + x0 + (x1-x0)*fraction, editorPadY + y + h*.85 - d.vscroll.Y
}

func TestSemanticPointGlyphsAndEmptySpace(t *testing.T) {
	hs, d := editHarness(t, "symbols.txt", "alpha beta\n\t世界变量()\n  after")
	a := hs.a
	for _, tc := range []struct {
		at, start, end int
	}{
		{0, 0, 5}, {4, 0, 5}, {6, 6, 10}, {9, 6, 10},
		{12, 12, 16}, {15, 12, 16},
	} {
		x, y := semanticPointRune(t, d, tc.at, .9)
		got, ok := a.semanticPointAt(d, x, y)
		if !ok || got.at != tc.at || got.start != tc.start || got.end != tc.end || !got.anchor.Contains(x, y) {
			t.Errorf("rune %d: point=%+v, ok=%v", tc.at, got, ok)
		}
	}
	for _, at := range []int{5, 11, 16, 17, 19, 20} {
		x, y := semanticPointRune(t, d, at, .9)
		if got, ok := a.semanticPointAt(d, x, y); ok {
			t.Errorf("whitespace or punctuation at %d hit %+v", at, got)
		}
	}
	for _, p := range []ui.Point{
		{X: d.textX - 1, Y: editorPadY + a.font.lineH/2},
		{X: d.textX + a.font.charW, Y: editorPadY - 1},
		{X: d.textX + 11*a.font.charW, Y: editorPadY + a.font.lineH/2},
		{X: d.textX + a.font.charW, Y: editorPadY + d.lines.Height() + 1},
		{X: d.lastView.W + 1, Y: editorPadY + a.font.lineH/2},
		{X: d.textX + a.font.charW, Y: d.lastView.H + 1},
	} {
		if got, ok := a.semanticPointAt(d, p.X, p.Y); ok {
			t.Errorf("empty or clipped point %+v hit %+v", p, got)
		}
	}
	d.sticky = []int{0}
	x, _ := semanticPointRune(t, d, 0, .5)
	y := a.font.lineH / 2
	if got, ok := a.semanticPointAt(d, x, y); ok {
		t.Fatalf("sticky scope header hit covered text %+v", got)
	}
}

func TestSemanticPointWrappedRows(t *testing.T) {
	word := strings.Repeat("symbol", 40)
	hs, d := editHarness(t, "wrapped.txt", word+"\nnext")
	a := hs.a
	a.settings.WordWrap = true
	hs.settle()
	rows := d.lines.Rows(0)
	if len(rows) < 3 {
		t.Fatalf("test name did not wrap: %v", rows)
	}
	for row := 0; row < len(rows); row++ {
		at := rows[row] + 1
		x, y := semanticPointRune(t, d, at, .85)
		got, ok := a.semanticPointAt(d, x, y)
		if !ok || got.at != at || got.start != 0 || got.end != len(word) || !got.anchor.Contains(x, y) {
			t.Errorf("row %d lower half: point=%+v, ok=%v, want rune %d", row, got, ok, at)
		}
		if ok && got.anchor.H > a.font.lineH {
			t.Errorf("row %d anchor spans another row: %+v", row, got.anchor)
		}
		if row+1 < len(rows) {
			// The wrap boundary has two caret positions. Its preceding
			// letter still belongs to this row, including its right half.
			last := rows[row+1] - 1
			cx, cy, h, valid := d.lines.Caret(last)
			if !valid {
				t.Fatal("last letter has no caret")
			}
			x, y = d.textX+cx+a.font.charW*.85, editorPadY+cy+h*.85-d.vscroll.Y
			got, ok = a.semanticPointAt(d, x, y)
			if !ok || got.at != last || !got.anchor.Contains(x, y) {
				t.Errorf("row %d last letter: point=%+v, ok=%v, want rune %d", row, got, ok, last)
			}
		}
	}
	// A visible caret with upstream affinity at a soft break must not
	// change how the next row's name is hit.
	hs.place(d, rows[1], rows[1])
	x, y := semanticPointRune(t, d, rows[1]+1, .85)
	if got, ok := a.semanticPointAt(d, x, y); !ok || got.at != rows[1]+1 {
		t.Fatalf("caret at wrap changed the hit: %+v, ok=%v", got, ok)
	}
}

func TestSemanticPointHorizontalScroll(t *testing.T) {
	prefix := strings.Repeat("padding ", 80)
	hs, d := editHarness(t, "horizontal.txt", prefix+"target()")
	a := hs.a
	start := len(prefix)
	hs.place(d, start, start)
	if d.hscroll.X <= 0 {
		t.Fatal("test did not scroll horizontally")
	}
	x, y := semanticPointRune(t, d, start+2, .4)
	// A genuine pointer click uses the same area-local coordinates as
	// semantic hit testing; it puts the caret at the left of this cell.
	hs.tt.ClickAt(d.lastView.X-d.hscroll.X+x, d.lastView.Y+y)
	hs.frames()
	if d.sel[1] != start+2 {
		t.Fatalf("native click at area-local (%v,%v) put caret at %d, want %d", x, y, d.sel[1], start+2)
	}
	got, ok := a.semanticPointAt(d, x, y)
	if !ok || got.at != start+2 || got.start != start || got.end != start+len("target") {
		t.Fatalf("scrolled hit: %+v, ok=%v", got, ok)
	}
	if _, ok := a.semanticPointAt(d, d.hscroll.X+d.textX-1, y); ok {
		t.Fatal("text behind the stationary gutter was hit")
	}
	if got.anchor.X < d.hscroll.X+d.textX || got.anchor.X+got.anchor.W > d.hscroll.X+d.lastView.W {
		t.Fatalf("anchor extends outside the visible text: %+v", got.anchor)
	}
}

func TestSemanticPointLongFile(t *testing.T) {
	text := "first target()\n" + strings.Repeat("more text on another line\n", 20_000)
	hs, d := editHarness(t, "large.txt", text)
	hs.a.settings.WordWrap = true
	hs.settle()
	x, y := semanticPointRune(t, d, 7, .9)
	if got, ok := hs.a.semanticPointAt(d, x, y); !ok || got.at != 7 || got.start != 6 || got.end != 12 {
		t.Fatalf("large wrapped file: %+v, ok=%v", got, ok)
	}
}

func BenchmarkSemanticPoint(b *testing.B) {
	for _, lines := range []int{10, 20_000} {
		b.Run(fmt.Sprintf("lines_%d", lines), func(b *testing.B) {
			hs := newHarness(b, false)
			path := hs.path("large.txt")
			if err := os.WriteFile(path, []byte("first target()\n"+strings.Repeat("more text on another line\n", lines)), 0o644); err != nil {
				b.Fatal(err)
			}
			hs.a.settings.WordWrap = true
			hs.a.openFile(path, true)
			hs.settle()
			d := hs.a.activeEditor()
			x, y := semanticPointRune(b, d, 7, .9)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, ok := hs.a.semanticPointAt(d, x, y); !ok {
					b.Fatal("missing target")
				}
			}
		})
	}
}

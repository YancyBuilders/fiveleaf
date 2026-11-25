package main

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func TestParseMdLine(t *testing.T) {
	for _, c := range []struct {
		text, quote, indent, marker, box, body string
		heading                                int
	}{
		{"plain", "", "", "", "", "plain", 0},
		{"- item", "", "", "-", "", "item", 0},
		{"  * [x] done", "", "  ", "*", "[x]", "done", 0},
		{"12) twelve", "", "", "12)", "", "twelve", 0},
		{"> > - [ ] task", "> > ", "", "-", "[ ]", "task", 0},
		{"## Title", "", "", "", "", "Title", 2},
		{"#tag", "", "", "", "", "#tag", 0},
		{"-dash", "", "", "", "", "-dash", 0},
		{"1.5 apples", "", "", "", "", "1.5 apples", 0},
		{"- ", "", "", "-", "", "", 0},
	} {
		l := parseMdLine(c.text)
		if l.quote != c.quote || l.indent != c.indent || l.marker != c.marker || l.box != c.box || l.body != c.body || l.heading != c.heading {
			t.Errorf("%q: %+v", c.text, l)
		}
	}
	if got := nextMarker("9.", 1); got != "10." {
		t.Errorf("nextMarker 9. = %q", got)
	}
	if got := nextMarker("1)", -1); got != "1)" {
		t.Errorf("nextMarker 1) back = %q", got)
	}
}

func TestMarkdownEnterContinuesLists(t *testing.T) {
	for _, c := range []struct {
		name, text string
		at         int
		want       string
		caret      int
	}{
		{"bullet", "- one", 5, "- one\n- ", 8},
		{"number", "  9. nine", 9, "  9. nine\n  10. ", 16},
		{"task", "- [x] done", 10, "- [x] done\n- [ ] ", 17},
		{"quote", "> said", 6, "> said\n> ", 9},
		{"split", "- onetwo", 5, "- one\n- two", 8},
		{"ends list", "- one\n- ", 8, "- one\n", 6},
		{"outdents", "- one\n  - ", 10, "- one\n- ", 8},
		{"ends quote", "> > ", 4, "> ", 2},
		{"list in quote ends", "> - ", 4, "> ", 2},
		{"before marker", "- one", 0, "\n- one", 1},
		{"in fence", "```\n- code", 10, "```\n- code\n", 11},
		{"rule", "* * *", 5, "* * *\n", 6},
	} {
		t.Run(c.name, func(t *testing.T) {
			hs, d := editHarness(t, "notes.md", c.text)
			hs.place(d, c.at, c.at)
			hs.key(0, ui.KeyEnter)
			expectText(t, d, c.want)
			expectSel(t, d, c.caret, c.caret)
		})
	}
}

func TestEnterOutsideMarkdownKeepsIndentOnly(t *testing.T) {
	hs, d := editHarness(t, "notes.txt", "- one")
	hs.place(d, 5, 5)
	hs.key(0, ui.KeyEnter)
	expectText(t, d, "- one\n")
}

func TestMarkdownNumberedSiblings(t *testing.T) {
	for _, tc := range []struct{ name, before, after string }{
		{"nested and restarted", "> 08) 甲|乙\n>     1. 内层\n> 09) 中文\n> 10) 下个\n> 03) 自定\n> 04) 保留", "> 08) 甲\n> 09) |乙\n>     1. 内层\n> 10) 中文\n> 11) 下个\n> 03) 自定\n> 04) 保留"},
		{"empty item", "1. first\n2. |\n3. third", "1. first\n|\n2. third"},
		{"literal numbers", "1. one|\n\n   ```text\n   2. literal\n   ```\n\n2. two\n\nnew paragraph\n\n3. separate", "1. one\n2. |\n\n   ```text\n   2. literal\n   ```\n\n3. two\n\nnew paragraph\n\n3. separate"},
		{"list after unclosed nested fence", "- sample\n  ```js\n  code\n\n1. outside|\n2. next", "- sample\n  ```js\n  code\n\n1. outside\n2. |\n3. next"},
		{"loose siblings", "1. first|\n\n   - child\n\n2. second\n\n3. third", "1. first\n2. |\n\n   - child\n\n3. second\n\n4. third"},
		{"marker grows", "8. 之前|\n9. 带孩子\n   - nested\n10. 下个", "8. 之前\n9. |\n10. 带孩子\n    - nested\n11. 下个"},
		{"marker shrinks", "8. 之前\n9. |\n10. 带孩子\n    - nested\n11. 下个", "8. 之前\n|\n9. 带孩子\n   - nested\n10. 下个"},
		{"lazy continuation", "1. first|\nlazy text\n2. second", "1. first\n2. |\nlazy text\n3. second"},
		{"repeated numbers", "1. one|\n1. two\n1. three", "1. one\n2. |\n1. two\n1. three"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"source", "live"} {
				before, caret := mdTypingPosition(tc.before)
				after, result := mdTypingPosition(tc.after)
				hs, d := editHarness(t, "写作.md", before)
				hs.a.setMarkdownMode(mode)
				hs.place(d, caret, caret)
				hs.key(0, ui.KeyEnter)
				expectText(t, d, after)
				expectSel(t, d, result, result)
				hs.key(ui.Cmd, ui.KeyZ)
				expectText(t, d, before)
				expectSel(t, d, caret, caret)
				hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
				expectText(t, d, after)
			}
		})
	}
}

func mdTypingPosition(marked string) (string, int) {
	before, after, _ := strings.Cut(marked, "|")
	return before + after, utf8.RuneCountInString(before)
}

func TestMarkdownMarkerBackspace(t *testing.T) {
	for _, tc := range []struct{ name, file, before, after string }{
		{"task", "notes.md", "> - [x] |保留", "> |保留"},
		{"nested quote", "notes.md", "> > |保留", "> |保留"},
		{"nested item", "notes.md", "* parent\n  1) |child", "* parent\n  |child"},
		{"middle of text", "notes.md", "- 中|文", "- |文"},
		{"inside marker", "notes.md", "- [x|] 保留", "- [|] 保留"},
		{"code", "notes.md", "> ```sh\n> - |literal\n> ```", "> ```sh\n> -|literal\n> ```"},
		{"after nested fence", "notes.md", "- sample\n  ```js\n  code\n\n1. |outside\n2. next", "- sample\n  ```js\n  code\n\n|outside\n2. next"},
		{"after closed fence", "notes.md", "```\n- literal\n```\n1. |outside", "```\n- literal\n```\n|outside"},
		{"front matter", "notes.md", "---\ntags:\n  - |literal\n---", "---\ntags:\n  -|literal\n---"},
		{"indented code", "notes.md", "    - |literal", "    -|literal"},
		{"indented literal quote", "notes.md", "    > |literal", "    >|literal"},
		{"plain text", "notes.txt", "- |literal", "-|literal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, caret := mdTypingPosition(tc.before)
			after, result := mdTypingPosition(tc.after)
			hs, d := editHarness(t, tc.file, before)
			hs.place(d, caret, caret)
			hs.key(0, ui.KeyBackspace)
			expectText(t, d, after)
			expectSel(t, d, result, result)
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, before)
		})
	}
}

func BenchmarkMarkdownListEnter(b *testing.B) {
	source := string(benchMarkdown(b)) + "\n\n> 1. 编写说明\n> 2. 检查格式\n> 3. 发布"
	d := newEditor(newDocument(1, "writing.md", source, nil))
	snap := d.buf.Snapshot()
	_, caret := snap.LineRange(snap.LineCount() - 3)
	d.sel = [2]int{caret, caret}
	b.ReportAllocs()
	for b.Loop() {
		newline(d)
		d.undo()
	}
}

func TestMarkdownPrefixKeysKeepSelectionAndReadOnly(t *testing.T) {
	hs, d := editHarness(t, "notes.md", "- [x] 正文")
	hs.place(d, 6, 5)
	hs.key(0, ui.KeyBackspace)
	expectText(t, d, "- [x]正文")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, "- [x] 正文")
	hs.place(d, 6, 6)
	d.sync, d.loadErr = syncLoadFailed, errors.New("read-only test document")
	hs.settle()
	hs.key(0, ui.KeyBackspace)
	hs.key(0, ui.KeyEnter)
	expectText(t, d, "- [x] 正文")
	expectSel(t, d, 6, 6)
}

func TestMarkdownInsertLineAboveCarriesMarker(t *testing.T) {
	hs, d := editHarness(t, "notes.md", "3. three")
	hs.place(d, 3, 3)
	hs.key(ui.Cmd|ui.Shift, ui.KeyEnter)
	expectText(t, d, "2. \n3. three")
	expectSel(t, d, 3, 3)
	for _, tc := range []struct{ before, after string }{
		{"1. 最|初\n2. 下个", "1. |\n2. 最初\n3. 下个"},
		{"1. 最初\n2. 中|间\n3. 下个", "1. 最初\n2. |\n3. 中间\n4. 下个"},
		{"1. first\n   - child\n2. sec|ond\n3. third", "1. first\n   - child\n2. |\n3. second\n4. third"},
	} {
		before, at := mdTypingPosition(tc.before)
		after, want := mdTypingPosition(tc.after)
		hs, d := editHarness(t, "notes.md", before)
		hs.place(d, at, at)
		hs.key(ui.Cmd|ui.Shift, ui.KeyEnter)
		expectText(t, d, after)
		expectSel(t, d, want, want)
		hs.key(ui.Cmd, ui.KeyZ)
		expectText(t, d, before)
		expectSel(t, d, at, at)
	}
}

func TestMarkdownCmdEnterTogglesTask(t *testing.T) {
	hs, d := editHarness(t, "notes.md", "- one")
	hs.place(d, 5, 5)
	for _, want := range []string{"- [ ] one", "- [x] one", "- one"} {
		hs.key(ui.Cmd, ui.KeyEnter)
		expectText(t, d, want)
		expectSel(t, d, len(want), len(want))
	}

	hs, d = editHarness(t, "notes.md", "plain")
	hs.place(d, 0, 0)
	hs.key(ui.Cmd, ui.KeyEnter)
	expectText(t, d, "- [ ] plain")

	hs, d = editHarness(t, "notes.md", "```\n- code")
	hs.place(d, 10, 10)
	hs.key(ui.Cmd, ui.KeyEnter)
	expectText(t, d, "```\n- code\n")
}

func TestMarkdownTabIndentsItem(t *testing.T) {
	hs, d := editHarness(t, "notes.md", "- one\n- two")
	d.indent = indentStyle{size: 2}
	hs.place(d, 9, 9)
	hs.key(0, ui.KeyTab)
	expectText(t, d, "- one\n  - two")
	expectSel(t, d, 11, 11)
	hs.key(ui.Shift, ui.KeyTab)
	expectText(t, d, "- one\n- two")

	hs, d = editHarness(t, "quote.md", "> - one")
	d.indent = indentStyle{size: 2}
	hs.place(d, 7, 7)
	hs.key(0, ui.KeyTab)
	expectText(t, d, ">   - one")

	// Text that is no item takes a tab at the caret.
	hs, d = editHarness(t, "text.md", "ab")
	d.indent = indentStyle{size: 2}
	hs.place(d, 1, 1)
	hs.key(0, ui.KeyTab)
	expectText(t, d, "a b")
}

func TestMarkdownToggleCommands(t *testing.T) {
	run := func(t *testing.T, text string, anchor, caret int, ids ...string) (*harness, *editor) {
		t.Helper()
		hs, d := editHarness(t, "notes.md", text)
		hs.place(d, anchor, caret)
		for _, id := range ids {
			hs.a.run(id)
			hs.settle()
		}
		return hs, d
	}
	t.Run("task cycles", func(t *testing.T) {
		hs, d := run(t, "buy", 3, 3, "markdown.toggleTask")
		expectText(t, d, "- [ ] buy")
		expectSel(t, d, 9, 9)
		hs.a.run("markdown.toggleTask")
		hs.settle()
		expectText(t, d, "- [x] buy")
		if d.liveTicked != 2 {
			t.Fatalf("ticked %d", d.liveTicked)
		}
		hs.a.run("markdown.toggleTask")
		hs.settle()
		expectText(t, d, "- buy")
	})
	t.Run("list cycles and numbers", func(t *testing.T) {
		hs, d := run(t, "a\n\nb", 0, 4, "markdown.toggleList")
		expectText(t, d, "- a\n\n- b")
		hs.a.run("markdown.toggleList")
		hs.settle()
		expectText(t, d, "1. a\n\n2. b")
		hs.a.run("markdown.toggleList")
		hs.settle()
		expectText(t, d, "a\n\nb")
	})
	t.Run("quote", func(t *testing.T) {
		hs, d := run(t, "a\n\nb", 0, 4, "markdown.toggleQuote")
		expectText(t, d, "> a\n>\n> b")
		hs.a.run("markdown.toggleQuote")
		hs.settle()
		expectText(t, d, "a\n\nb")
	})
	t.Run("heading", func(t *testing.T) {
		hs, d := run(t, "- Title", 7, 7, "markdown.heading2")
		expectText(t, d, "## Title")
		expectSel(t, d, 8, 8)
		hs.a.run("markdown.heading3")
		hs.settle()
		expectText(t, d, "### Title")
		hs.a.run("markdown.heading3")
		hs.settle()
		expectText(t, d, "Title")
	})
	t.Run("clear", func(t *testing.T) {
		_, d := run(t, "> - [x] done\n## H\nplain", 0, 20, "markdown.clearFormatting")
		expectText(t, d, "done\nH\nplain")
	})
	t.Run("embed", func(t *testing.T) {
		hs, d := run(t, "see [[a]] and [[b.png]]", 0, 0, "markdown.toggleEmbed")
		expectText(t, d, "see ![[a]] and ![[b.png]]")
		hs.a.run("markdown.toggleEmbed")
		hs.settle()
		expectText(t, d, "see [[a]] and [[b.png]]")
	})
	t.Run("not markdown", func(t *testing.T) {
		hs, d := editHarness(t, "notes.txt", "buy")
		hs.a.run("markdown.toggleTask")
		hs.settle()
		expectText(t, d, "buy")
	})
}

func TestJoinLines(t *testing.T) {
	hs, d := editHarness(t, "a.txt", "  one  \n   two   three\n\nfour\nfive")
	hs.place(d, 0, 0)
	hs.a.run("sel.joinLines")
	hs.settle()
	expectText(t, d, "  one two three\n\nfour\nfive")
	expectSel(t, d, 5, 5)
	hs.place(d, 0, 22)
	hs.a.run("sel.joinLines")
	hs.settle()
	expectText(t, d, "  one two three four\nfive")
	expectSel(t, d, 0, 20)
}

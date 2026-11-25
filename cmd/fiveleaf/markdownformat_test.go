package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func TestMarkdownFormatToggle(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		sel, after       [2]int
		run              func(*editor)
	}{
		{"bold Unicode reverse", "前中文后", "前**中文**后", [2]int{3, 1}, [2]int{5, 3}, toggleMdBold},
		{"italic", "words", "*words*", [2]int{0, 5}, [2]int{1, 6}, toggleMdItalic},
		{"strike", "old", "~~old~~", [2]int{0, 3}, [2]int{2, 5}, toggleMdStrike},
		{"bold empty", "ab", "a****b", [2]int{1, 1}, [2]int{3, 3}, toggleMdBold},
		{"italic empty", "ab", "a**b", [2]int{1, 1}, [2]int{2, 2}, toggleMdItalic},
		{"strike empty", "ab", "a~~~~b", [2]int{1, 1}, [2]int{3, 3}, toggleMdStrike},
		{"bold in italic", "*words*", "***words***", [2]int{1, 6}, [2]int{3, 8}, toggleMdBold},
		{"italic in bold", "**words**", "***words***", [2]int{2, 7}, [2]int{3, 8}, toggleMdItalic},
		{"code Unicode", "前文😀后", "前`文😀`后", [2]int{3, 1}, [2]int{4, 2}, toggleMdCode},
		{"code empty", "ab", "a``b", [2]int{1, 1}, [2]int{2, 2}, toggleMdCode},
		{"code embedded backtick", "one`two", "``one`two``", [2]int{0, 7}, [2]int{2, 9}, toggleMdCode},
		{"code edge backtick", "`one", "`` `one ``", [2]int{0, 4}, [2]int{3, 7}, toggleMdCode},
		{"code spaces", " one ", "`  one  `", [2]int{0, 5}, [2]int{2, 7}, toggleMdCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs, d := editHarness(t, "notes.md", tc.text)
			hs.place(d, tc.sel[0], tc.sel[1])
			hs.a.queueEdit(d, tc.run)
			hs.settle()
			expectText(t, d, tc.want)
			expectSel(t, d, tc.after[0], tc.after[1])
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, tc.text)
			expectSel(t, d, tc.sel[0], tc.sel[1])
			hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
			expectText(t, d, tc.want)
			hs.a.queueEdit(d, tc.run)
			hs.settle()
			expectText(t, d, tc.text)
			expectSel(t, d, tc.sel[0], tc.sel[1])
		})
	}
}

func TestMarkdownFormatSelectedMarkers(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		run              func(*editor)
	}{
		{"bold", "**中文**", "中文", toggleMdBold},
		{"italic", "*中文*", "中文", toggleMdItalic},
		{"strike", "~~中文~~", "中文", toggleMdStrike},
		{"bold in both", "***中文***", "*中文*", toggleMdBold},
		{"italic in both", "***中文***", "**中文**", toggleMdItalic},
		{"italic around bold", "**中文**", "***中文***", toggleMdItalic},
		{"code", "`中文`", "中文", toggleMdCode},
		{"code with backtick", "`` `中文 ``", "`中文", toggleMdCode},
		{"code whitespace", "`  中文  `", " 中文 ", toggleMdCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs, d := editHarness(t, "notes.md", tc.text)
			hs.place(d, utf8.RuneCountInString(tc.text), 0)
			hs.a.queueEdit(d, tc.run)
			hs.settle()
			expectText(t, d, tc.want)
			if d.sel[0] <= d.sel[1] {
				t.Fatalf("reversed selection lost: %v", d.sel)
			}
		})
	}
}

func TestMarkdownFormatWhitespace(t *testing.T) {
	hs, d := editHarness(t, "notes.md", "before  中文 \n after")
	hs.place(d, 6, 12)
	hs.a.queueEdit(d, toggleMdBold)
	hs.settle()
	expectText(t, d, "before  **中文** \n after")
	expectSel(t, d, 10, 12)
}

func TestMarkdownFormatLinks(t *testing.T) {
	for _, image := range []bool{false, true} {
		for _, label := range []string{"", "中 [文]\\路径"} {
			t.Run(strings.Join([]string{map[bool]string{false: "link", true: "image"}[image], label}, "/"), func(t *testing.T) {
				hs, d := editHarness(t, "notes.md", label)
				hs.place(d, utf8.RuneCountInString(label), 0)
				run, prefix, fallback := insertMdLink, "[", locales.MarkdownLinkText()
				if image {
					run, prefix, fallback = insertMdImage, "![", locales.MarkdownImageAlt()
				}
				hs.a.queueEdit(d, run)
				hs.settle()
				wantLabel := `中 \[文\]\\路径`
				if label == "" {
					wantLabel = fallback
				}
				expectText(t, d, prefix+wantLabel+"](https://)")
				snap := d.buf.Snapshot()
				if got := snap.Slice(d.sel[0], d.sel[1]); got != "https://" {
					t.Fatalf("URL was not selected: %q", got)
				}
				hs.tt.Type("https://example.com/文")
				hs.settle()
				expectText(t, d, prefix+wantLabel+"](https://example.com/文)")
			})
		}
	}
}

func TestMarkdownFormatCodeBlock(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		sel, after       [2]int
	}{
		{"empty", "", "```\n\n```", [2]int{}, [2]int{4, 4}},
		{"middle of paragraph", "前中文后", "前\n\n```\n中文\n```\n\n后", [2]int{3, 1}, [2]int{9, 7}},
		{"existing blank lines", "前\n\n中文\n\n后", "前\n\n```\n中文\n```\n\n后", [2]int{3, 5}, [2]int{7, 9}},
		{"one newline", "前\n中文\n后", "前\n\n```\n中文\n```\n\n后", [2]int{2, 4}, [2]int{7, 9}},
		{"safe fence", "x ```` y", "`````\nx ```` y\n`````", [2]int{0, 8}, [2]int{6, 14}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs, d := editHarness(t, "notes.md", tc.text)
			hs.place(d, tc.sel[0], tc.sel[1])
			hs.a.queueEdit(d, insertMdCodeBlock)
			hs.settle()
			expectText(t, d, tc.want)
			expectSel(t, d, tc.after[0], tc.after[1])
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, tc.text)
			expectSel(t, d, tc.sel[0], tc.sel[1])
			hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
			expectText(t, d, tc.want)
		})
	}
}

func insertTwoByTwoMdTable(d *editor) { insertMdTable(d, 2, 2) }

func TestMarkdownFormatTable(t *testing.T) {
	hs, d := editHarness(t, "notes.md", "前后")
	hs.place(d, 1, 1)
	hs.a.queueEdit(d, insertTwoByTwoMdTable)
	hs.settle()
	first, second := locales.MarkdownTableColumn("1"), locales.MarkdownTableColumn("2")
	expectText(t, d, "前\n\n| "+first+" | "+second+" |\n| --- | --- |\n|  |  |\n\n后")
	expectSel(t, d, 5, 5+utf8.RuneCountInString(first))
	hs.tt.Type("标题")
	hs.settle()
	if !strings.Contains(d.buf.String(), "| 标题 | "+second+" |") {
		t.Fatalf("could not edit first header: %q", d.buf.String())
	}
	hs.key(ui.Cmd, ui.KeyZ)
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, "前后")

	hs, d = editHarness(t, "notes.md", "a|b\n中文")
	hs.place(d, 0, d.buf.Len())
	hs.a.queueEdit(d, insertTwoByTwoMdTable)
	hs.settle()
	expectText(t, d, "| a\\|b<br>中文 | "+second+" |\n| --- | --- |\n|  |  |")
}

func TestMarkdownFormatGuards(t *testing.T) {
	commands := []func(*editor){toggleMdBold, toggleMdItalic, toggleMdStrike, toggleMdCode, insertMdLink, insertMdImage, insertMdCodeBlock, insertTwoByTwoMdTable}
	for _, tc := range []struct {
		name, path, text string
		start, end       int
		locked           bool
	}{
		{"non Markdown", "notes.txt", "text", 0, 4, false},
		{"locked", "notes.md", "text", 0, 4, true},
		{"fenced code", "notes.md", "```\ntext\n```", 4, 8, false},
		{"fence start", "notes.md", "```\ntext\n```", 0, 0, false},
		{"crosses fence", "notes.md", "text\n```\ncode\n```", 0, 13, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs, d := editHarness(t, tc.path, tc.text)
			hs.place(d, tc.start, tc.end)
			d.offline = tc.locked
			before := len(d.hist.undo)
			for _, command := range commands {
				command(d)
				expectText(t, d, tc.text)
				expectSel(t, d, tc.start, tc.end)
			}
			if len(d.hist.undo) != before {
				t.Fatal("a rejected command added an undo entry")
			}
		})
	}
}

func TestMarkdownFormatSeparateSpans(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		run              func(*editor)
	}{
		{"bold", "**one** and **two**", "****one** and **two****", toggleMdBold},
		{"italic", "*one* and *two*", "**one* and *two**", toggleMdItalic},
		{"strike", "~~one~~ and ~~two~~", "~~one~~ and ~~two~~", toggleMdStrike},
		{"code", "`one` and `two`", "`` `one` and `two` ``", toggleMdCode},
		{"nested bold", "**one *and* two**", "one *and* two", toggleMdBold},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs, d := editHarness(t, "notes.md", tc.text)
			hs.place(d, 0, d.buf.Len())
			hs.a.queueEdit(d, tc.run)
			hs.settle()
			expectText(t, d, tc.want)
			blocks := parseMarkdown([]byte(d.buf.String()), "notes.md", "", nil)
			if len(blocks) != 1 || blocks[0].kind != markdownParagraph {
				t.Fatalf("formatting no longer parses as one paragraph: %+v", blocks)
			}
			var rendered strings.Builder
			for _, run := range blocks[0].runs {
				rendered.WriteString(run.text)
				if tc.name == "bold" && !run.bold || tc.name == "italic" && !run.italic || tc.name == "code" && !run.code {
					t.Fatalf("formatted content has an unstyled run: %+v", run)
				}
			}
			wantRendered := "one and two"
			if tc.name == "code" {
				wantRendered = tc.text
			}
			if got := rendered.String(); got != wantRendered {
				t.Fatalf("rendered text %q, want %q", got, wantRendered)
			}
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, tc.text)
		})
	}
}

func TestMarkdownFormatPartialSeparateSpans(t *testing.T) {
	for _, tc := range []struct {
		marker string
		run    func(*editor)
	}{
		{"**", toggleMdBold}, {"*", toggleMdItalic}, {"~~", toggleMdStrike}, {"`", toggleMdCode},
	} {
		t.Run(tc.marker, func(t *testing.T) {
			text := tc.marker + "one" + tc.marker + " and " + tc.marker + "two" + tc.marker
			hs, d := editHarness(t, "notes.md", text)
			hs.place(d, len(tc.marker), len(text)-len(tc.marker))
			before := len(d.hist.undo)
			hs.a.queueEdit(d, tc.run)
			hs.settle()
			expectText(t, d, text)
			if len(d.hist.undo) != before {
				t.Fatal("an ambiguous marker range added undo history")
			}
		})
	}
}

func TestMarkdownFormatAcrossBlocks(t *testing.T) {
	for _, text := range []string{"one\n\ntwo", "one\n# heading", "- one\n- two", "> one\n\n> two"} {
		t.Run(text, func(t *testing.T) {
			hs, d := editHarness(t, "notes.md", text)
			hs.place(d, 0, d.buf.Len())
			before := len(d.hist.undo)
			for _, command := range []func(*editor){toggleMdBold, toggleMdItalic, toggleMdStrike, toggleMdCode, insertMdLink, insertMdImage} {
				hs.a.queueEdit(d, command)
				hs.settle()
				expectText(t, d, text)
			}
			if len(d.hist.undo) != before {
				t.Fatal("unsupported multi-line formatting added undo history")
			}
		})
	}
}

func TestMarkdownFormatNestedCodeGuard(t *testing.T) {
	for _, text := range []string{
		"> ~~~\n> code\n> ~~~",
		"> ```go\n> code\n> ```",
		"- ~~~\n  code\n  ~~~",
		"> - ~~~\n>   code\n>   ~~~",
		"    code",
	} {
		t.Run(text, func(t *testing.T) {
			hs, d := editHarness(t, "notes.md", text)
			at := strings.Index(text, "code")
			hs.place(d, at, at+4)
			before := len(d.hist.undo)
			for _, command := range []func(*editor){toggleMdBold, toggleMdItalic, toggleMdStrike, toggleMdCode, insertMdLink, insertMdImage, insertMdCodeBlock, insertTwoByTwoMdTable} {
				hs.a.queueEdit(d, command)
				hs.settle()
				expectText(t, d, text)
				expectSel(t, d, at, at+4)
			}
			if len(d.hist.undo) != before {
				t.Fatal("formatting code added undo history")
			}
		})
	}
}

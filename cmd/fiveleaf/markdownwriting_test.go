package main

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"fiveleaf/internal/highlight"
	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func TestLiveWritingKeepsRenderedMarks(t *testing.T) {
	const source = "# 标题\n\n> 中文 **重点** 与 `code`\n\n- [ ] 任务\n"
	hs, d := editHarness(t, "write.md", source)
	hs.a.setMarkdownMode("live")
	checkHidden := func(offset int) {
		t.Helper()
		for _, segment := range d.liveRanges {
			if segment.Hidden && segment.Start <= offset && offset < segment.End {
				return
			}
		}
		t.Fatalf("Markdown delimiter at %d became visible while writing", offset)
	}
	for _, word := range []string{"标题", "重点", "code", "任务"} {
		start := utf8.RuneCountInString(source[:strings.Index(source, word)])
		hs.place(d, start+1, start+1)
		hs.tt.Type("新")
		hs.settle()
		checkHidden(0)
		if word == "重点" || word == "code" {
			checkHidden(start - 1)
		}
		if word == "任务" && len(d.liveHits) == 0 {
			t.Fatal("the checkbox disappeared while editing its task")
		}
		hs.key(ui.Cmd, ui.KeyZ)
		expectText(t, d, source)
	}
}

func TestLiveTypeAtHiddenHeadingStart(t *testing.T) {
	hs, d := editHarness(t, "heading.md", "## 标题\n")
	hs.a.setMarkdownMode("live")
	hs.place(d, 0, 0)
	hs.tt.Type("新")
	hs.settle()
	expectText(t, d, "## 新标题\n")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, "## 标题\n")
}

func TestLiveHeadingMarksInsideContainers(t *testing.T) {
	const source = "> ## 标题\n\n- ### 次级标题\n\n正文标题\n--------\n"
	doc := liveParse(source)
	spans := liveSpans(doc, source)
	for _, mark := range []string{"## ", "### ", "--------"} {
		if !hasSpan(spans, "mark:"+mark) {
			t.Fatalf("missing heading mark %q in %q", mark, spans)
		}
	}
}

func TestLiveFenceLanguagePicker(t *testing.T) {
	const source = "例子\n\n~~~~text title=example\ndef 函数():\n    return 42\n~~~~\n"
	hs, d := editHarness(t, "code.md", source)
	hs.a.setMarkdownMode("live")
	word := utf8.RuneCountInString(source[:strings.Index(source, "函数")])
	hs.place(d, word+1, word+1)
	if err := hs.chooseButtonMenu(locales.MarkdownFenceLanguage(locales.MarkdownFencePlain()), "Python"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	want := strings.Replace(source, "~~~~text", "~~~~python", 1)
	expectText(t, d, want)
	expectSel(t, d, word+3, word+3)
	hs.until("Python tokens", func() bool {
		live := d.document.live.doc
		if live == nil || live.snap.Version() != d.buf.Version() {
			return false
		}
		for _, span := range live.spans {
			if span.look.syntax == highlight.Keyword && d.buf.Slice(span.start, span.end) == "return" {
				return true
			}
		}
		return false
	})
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, source)
	hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
	expectText(t, d, want)
}

func TestLiveEmptyFenceLanguage(t *testing.T) {
	hs, d := editHarness(t, "empty.md", "```\n```\n")
	hs.a.setMarkdownMode("live")
	hs.settle()
	if err := hs.chooseButtonMenu(locales.MarkdownFenceLanguage(locales.MarkdownFencePlain()), "Go"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, d, "```go\n```\n")
}

func TestLiveFenceLanguageAliasesAndDelimiters(t *testing.T) {
	const source = "开头\n\n> ~~~~go\tfile=示例\n> func main() {}\n> ~~~~\n\n````python\ndef f():\n    return 42\n```"
	doc := liveParse(source)
	runes := []rune(source)
	var goKeyword, pythonKeyword, quoteClose bool
	for _, span := range doc.spans {
		value := string(runes[span.start:span.end])
		if span.look.syntax == highlight.Keyword {
			goKeyword = goKeyword || value == "func"
			pythonKeyword = pythonKeyword || value == "return"
		}
		quoteClose = quoteClose || span.kind == liveMark && value == "~~~~"
		if span.kind == liveMark && value == "```" {
			t.Fatal("a shorter run inside the unclosed block was hidden as a closing fence")
		}
	}
	if !goKeyword || !pythonKeyword || !quoteClose {
		t.Fatalf("go keyword=%v python keyword=%v quoted closing fence=%v", goKeyword, pythonKeyword, quoteClose)
	}
}

func TestLiveFenceLanguageStaysWithSplit(t *testing.T) {
	const source = "开头\n\n```text\nprint(123)\n```\n"
	hs, left, right := splitHarness(t, func(hs *harness) string {
		path := hs.path("split.md")
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	})
	hs.a.setLive(left, true)
	right.vscroll.Focus()
	hs.settle()
	label := locales.MarkdownFenceLanguage(locales.MarkdownFencePlain())
	if err := hs.chooseButtonMenu(label, "Python"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.activeEditor() != left {
		t.Fatal("the language menu left focus in the other pane")
	}
	expectText(t, right, strings.Replace(source, "text", "python", 1))
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, right, source)
}

package main

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func TestMarkdownHeadingKeys(t *testing.T) {
	for _, system := range []string{"darwin", "windows", "linux"} {
		t.Run(system, func(t *testing.T) {
			onOS(t, system)
			hs, ed := editHarness(t, "写作.md", "段落")
			hs.a.settings.ZoomLevel = 2
			primary := markdownPrimaryModifier()
			for level := 1; level <= 6; level++ {
				id := fmt.Sprintf("markdown.heading%d", level)
				if hs.a.menu(false, nil).ItemByID(id).Accelerator == "" {
					t.Fatal("the Markdown menu omitted the heading shortcut")
				}
				hs.key(primary, ui.Key1+ui.Key(level-1))
				expectText(t, ed, strings.Repeat("#", level)+" 段落")
				if hint := hs.a.keyHint(id); hint != keyText(fmt.Sprintf("CmdOrCtrl+%d", level)) {
					t.Fatalf("level %d shortcut hint %q", level, hint)
				}
			}
			hs.key(primary, ui.Key0)
			expectText(t, ed, "段落")
			if hs.a.settings.ZoomLevel != 2 {
				t.Fatal("paragraph reset the window zoom")
			}
			hs.a.openFile(hs.path("main.go"), true)
			hs.settle()
			plain := hs.a.activeEditor()
			before := plain.buf.String()
			hs.key(primary, ui.Key2)
			expectText(t, plain, before)
			if hint := hs.a.menu(false, nil).ItemByID("view.zoomReset").Accelerator; hint == "" {
				t.Fatal("non-Markdown editor lost Reset Zoom")
			}
			if system == "darwin" {
				hs.key(tabMod(), ui.Key1)
				if hs.a.activeEditor() != ed {
					t.Fatal("non-Markdown editor lost tab switching")
				}
			} else {
				hs.a.openFile(ed.path, true)
				hs.settle()
			}
			hs.a.openFind(false)
			hs.settle()
			hs.key(primary, ui.Key2)
			expectText(t, ed, "段落")
			if hint := hs.a.menu(false, nil).ItemByID("view.zoomReset").Accelerator; hint == "" {
				t.Fatal("Find lost Reset Zoom")
			}
			hs.a.closeFind()
			hs.settle()
			shell := testPanelSession(t, hs.a)
			t.Cleanup(hs.a.closeTerminals)
			hs.a.terminalToColumn(shell, hs.a.column(), len(hs.a.column().tabs))
			hs.settle()
			shell.view.Focus()
			hs.settle()
			if !shell.view.FocusWithin(hs.a.review.ctx) {
				t.Fatalf("terminal did not take focus: %v", hs.a.keyFocus)
			}
			hs.key(primary, ui.Key1)
			expectText(t, ed, "段落")
		})
	}
}

func TestMarkdownHeadingConversions(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		level               int
	}{
		{"setext level", "正文\n\n中文|标题\n===\n\n尾巴", "正文\n\n中文|标题\n---\n\n尾巴", 2},
		{"setext underline", "正文\n\n标题\n=|==\n\n尾巴", "正文\n\n### 标题|\n\n尾巴", 3},
		{"setext multiline", "> 中**文**\n> 第二|行\n> ----\n\n尾巴", "> #### 中**文** 第二|行\n\n尾巴", 4},
		{"setext body", "> 中**文**\n> 第二|行\n> ----\n\n尾巴", "> 中**文**\n> 第二|行\n\n尾巴", 0},
		{"hash in setext", "#tag|\n===", "#### #tag|", 4},
		{"nested ATX", "- > ## 中文| **字** ##", "- > #### 中文| **字** ##", 4},
		{"close ATX", "## 中文| **字** ##\n", "中文| **字**\n", 0},
		{"escaped ATX", "## 中文| \\#\n", "中文| \\#\n", 0},
		{"separator", "中文|\n\n---\n", "## 中文|\n\n---\n", 2},
		{"plain paragraph", "  - [x] 文本|\n", "  - [x] 文本|\n", 0},
		{"code", "> ```go\n> ## 文|本\n> ```", "> ```go\n> ## 文|本\n> ```", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, at := mdTypingPosition(tc.before)
			after, want := mdTypingPosition(tc.after)
			hs, ed := editHarness(t, "headings.md", before)
			hs.a.setMarkdownMode("live")
			hs.place(ed, at, at)
			hs.key(markdownPrimaryModifier(), ui.Key0+ui.Key(tc.level))
			expectText(t, ed, after)
			expectSel(t, ed, want, want)
			if before != after {
				hs.key(ui.Cmd, ui.KeyZ)
				expectText(t, ed, before)
				expectSel(t, ed, at, at)
			}
		})
	}
	const before = "前面\n\n一级\n===\n\n## 二级 ##\n\n尾部"
	hs, ed := editHarness(t, "selection.md", before)
	start := utf8.RuneCountInString("前面\n\n一")
	end := utf8.RuneCountInString("前面\n\n一级\n===\n\n## 二")
	hs.place(ed, end, start)
	hs.key(markdownPrimaryModifier(), ui.Key3)
	expectText(t, ed, "前面\n\n### 一级\n\n### 二级 ##\n\n尾部")
	expectSel(t, ed, utf8.RuneCountInString("前面\n\n### 一级\n\n### 二"), utf8.RuneCountInString("前面\n\n### 一"))
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, ed, before)
	expectSel(t, ed, end, start)
}

func TestMarkdownHeadingBindingAndModalMode(t *testing.T) {
	hs, ed := editHarness(t, "keys.md", "标题")
	hs.a.openFile(hs.path("main.go"), true)
	hs.a.openFile(ed.path, true)
	hs.settle()
	withKeys(t, hs.a, `[{"key":"cmd+2","command":"-markdown.heading2"},{"key":"f6","command":"markdown.heading2"}]`)
	hs.settle()
	hs.key(ui.Super, ui.Key2)
	if hs.a.activeEditor() == ed {
		t.Fatal("removing the heading key did not return it to tab switching")
	}
	hs.a.openFile(ed.path, true)
	hs.settle()
	hs.key(0, ui.KeyF6)
	expectText(t, ed, "## 标题")
	if hs.a.keyHint("markdown.heading2") != "F6" {
		t.Fatal("the palette lost the personal heading shortcut")
	}
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, ed := editHarness(t, mode+".md", "标题")
			hs.a.settings.EditorMode = mode
			hs.settle()
			hs.key(ui.Super, ui.Key3)
			expectText(t, ed, "标题")
			hs.key(0, ui.KeyI)
			hs.key(ui.Super, ui.Key3)
			expectText(t, ed, "### 标题")
		})
	}
}

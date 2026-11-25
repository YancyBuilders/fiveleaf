package main

import (
	"os"
	"testing"

	"fiveleaf/locales"

	"github.com/egoist/mygo/ui"
)

func TestMarkdownWritingShortcuts(t *testing.T) {
	for _, mode := range []string{"source", "live"} {
		t.Run(mode, func(t *testing.T) {
			hs, doc := editHarness(t, "writing.md", "中文")
			if mode == "live" {
				hs.a.setMarkdownMode(mode)
			}
			hs.place(doc, 2, 0)
			closed := hs.a.window.SidebarClosed
			for _, test := range []struct {
				key     ui.Key
				mods    ui.Modifiers
				text    string
				selects [2]int
			}{
				{ui.KeyB, ui.Super, "**中文**", [2]int{4, 2}},
				{ui.KeyI, ui.Super, "*中文*", [2]int{3, 1}},
				{ui.KeyX, ui.Super | ui.Shift, "~~中文~~", [2]int{4, 2}},
				{ui.KeyE, ui.Super, "`中文`", [2]int{3, 1}},
				{ui.KeyK, ui.Super, "[中文](https://)", [2]int{5, 13}},
				{ui.KeyL, ui.Super | ui.Shift, "![中文](https://)", [2]int{6, 14}},
				{ui.KeyK, ui.Super | ui.Shift, "```\n中文\n```", [2]int{6, 4}},
			} {
				hs.key(test.mods, test.key)
				expectText(t, doc, test.text)
				expectSel(t, doc, test.selects[0], test.selects[1])
				hs.key(ui.Cmd, ui.KeyZ)
				expectText(t, doc, "中文")
				expectSel(t, doc, 2, 0)
			}
			if hs.a.window.SidebarClosed != closed || !hs.tt.Focused(locales.EditorLabel(doc.name())) {
				t.Fatal("formatting changed the sidebar or editor focus")
			}
		})
	}
}

func TestMarkdownWritingKeysRespectUserBindings(t *testing.T) {
	hs, doc := editHarness(t, "keys.md", "中文")
	hs.place(doc, 0, 2)
	withKeys(t, hs.a, `[
		{"key":"cmd+k b", "command":"markdown.strikethrough", "when":"editorTextFocus"},
		{"key":"cmd+b", "command":"-markdown.bold"},
		{"key":"f6", "command":"markdown.bold"},
		{"key":"cmd+i", "command":"-markdown.italic"}
	]`)
	hs.key(ui.Super, ui.KeyK)
	expectText(t, doc, "中文")
	if len(hs.a.chord) != 1 {
		t.Fatal("inserting a link claimed a personal chord")
	}
	hs.tt.TypeKey(0, ui.KeyB, "b")
	hs.settle()
	expectText(t, doc, "~~中文~~")
	hs.key(ui.Cmd, ui.KeyZ)
	hs.key(ui.Super, ui.KeyI)
	expectText(t, doc, "中文")
	hs.key(ui.Super, ui.KeyB)
	expectText(t, doc, "中文")
	if item := hs.a.menu(false, nil).ItemByID("view.sidebar"); item.Accelerator == "" {
		t.Fatal("removing Bold also removed the sidebar's shortcut")
	}
	hs.key(0, ui.KeyF6)
	expectText(t, doc, "**中文**")
}

func TestMarkdownWritingKeysFollowFocus(t *testing.T) {
	for _, system := range []string{"darwin", "windows", "linux"} {
		t.Run(system, func(t *testing.T) {
			onOS(t, system)
			hs, doc := editHarness(t, "focus.md", "正文")
			hs.place(doc, 0, 2)
			sidebarShortcut := func() string {
				return hs.a.menu(false, nil).ItemByID("view.sidebar").Accelerator
			}
			if got := sidebarShortcut(); got != "" {
				t.Fatalf("system menu claims %q in Markdown", got)
			}
			mods := ui.Ctrl
			if system == "darwin" {
				mods = ui.Super
			}
			closed := hs.a.window.SidebarClosed
			hs.key(mods, ui.KeyB)
			expectText(t, doc, "**正文**")
			if hs.a.window.SidebarClosed != closed {
				t.Fatal("the sidebar took the editor's key")
			}
			hs.a.openFind(false)
			hs.settle()
			if sidebarShortcut() == "" {
				t.Fatal("the sidebar lost its shortcut in Find")
			}
			hs.key(mods, ui.KeyB)
			expectText(t, doc, "**正文**")
			if system != "darwin" {
				hs.a.closeFind()
				hs.settle()
				hs.a.showExplorer()
				hs.settle()
				hs.key(mods, ui.KeyB)
				if !hs.a.window.SidebarClosed {
					t.Fatalf("the global shortcut did not run in Explorer (focus %d, accelerator %q, doc %q)", hs.a.keyFocus, sidebarShortcut(), doc.buf.String())
				}
			}
		})
	}
}

func TestMarkdownWritingMenuFollowsConditionalBindings(t *testing.T) {
	hs, doc := editHarness(t, "conditions.md", "正文")
	withKeys(t, hs.a, `[{"key":"cmd+b", "command":"-markdown.bold", "when":"editorHasSelection"}]`)
	hs.settle()
	host := &statusControlsHost{fakeHost: hs.h}
	hs.a.host = host
	for _, selected := range []bool{true, false} {
		before := host.menuChanges
		caret := 0
		if selected {
			caret = 2
		}
		hs.place(doc, 0, caret)
		if host.menuChanges == before {
			t.Fatalf("selection %v did not refresh the installed native menu", doc.sel)
		}
		item := hs.a.menu(false, nil).ItemByID("view.sidebar")
		if (item.Accelerator != "") != selected {
			t.Fatalf("selection %v gives the sidebar accelerator %q", doc.sel, item.Accelerator)
		}
		if selected {
			before = host.menuChanges
			hs.place(doc, 0, 1)
			if host.menuChanges != before {
				t.Fatal("resizing the selection rebuilt an identical menu")
			}
		}
	}
}

func TestMarkdownWritingKeysTargetFocusedSplit(t *testing.T) {
	hs, left, right := splitHarness(t, func(hs *harness) string {
		path := hs.path("split.md")
		if err := os.WriteFile(path, []byte("甲乙\n\n丙丁"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	})
	hs.place(right, 4, 6)
	hs.place(left, 2, 0)
	left.vscroll.Focus()
	hs.settle()
	hs.key(ui.Super, ui.KeyB)
	expectText(t, left, "**甲乙**\n\n丙丁")
	expectSel(t, left, 4, 2)
	expectSel(t, right, 8, 10)
}

func TestMarkdownWritingKeysRespectModalEditors(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, doc := editHarness(t, "modal.md", "正文")
			hs.a.settings.EditorMode = mode
			hs.settle()
			hs.key(ui.Super, ui.KeyK)
			expectText(t, doc, "正文")
			hs.key(0, ui.KeyI)
			hs.key(ui.Super, ui.KeyK)
			if doc.buf.String() == "正文" {
				t.Fatal("insert mode did not receive the link shortcut")
			}
		})
	}
}

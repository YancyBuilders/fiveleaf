package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/remote"
	"fiveleaf/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestMarkdownEditingKeepsHistoryAndSaves(t *testing.T) {
	source := "# 文档\n\n中文段落\n\n- 清单\n"
	hs, d := editHarness(t, "writing.md", source)
	a, tt := hs.a, hs.tt
	if err := tt.Click(a.withKey(locales.ShowPreview(), "view.toggleLivePreview")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !a.livePreview(d) {
		t.Fatal("Edit did not open Live Preview")
	}
	at := len([]rune(source[:strings.Index(source, "中文")]))
	hs.place(d, at, at+2)
	menuPoint := func(at int) (float32, float32) {
		x, y, height, ok := d.lines.Caret(at)
		if !ok {
			t.Fatal("the selection has no caret geometry")
		}
		origin := caretTrailOrigin(t, hs, d)
		return origin.X + x, origin.Y + y + height/2
	}
	mx, my := menuPoint(at + 1)
	tt.RightClickAt(mx, my)
	if err := tt.ChooseMenuItem(locales.CmdMarkdownBold()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	bold := strings.Replace(source, "中文", "**中文**", 1)
	expectText(t, d, bold)
	expectSel(t, d, at+2, at+4)
	if !tt.Focused(locales.EditorLabel(d.name())) {
		t.Fatal("formatting did not return focus to the editor")
	}
	tt.Type("正文")
	hs.settle()
	want := strings.Replace(source, "中文", "**正文**", 1)
	expectText(t, d, want)
	hs.shot("markdown-editing")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, bold)
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, source)
	hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
	expectText(t, d, bold)
	hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
	expectText(t, d, want)
	selection := d.sel
	if err := tt.Click(a.withKey(locales.ShowSource(), "view.toggleLivePreview")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if d.live || d.preview != nil {
		t.Fatal("Source did not leave live editing")
	}
	expectSel(t, d, selection[0], selection[1])
	a.run("file.save")
	hs.settle()
	got, err := os.ReadFile(d.path)
	if err != nil || string(got) != want || d.dirty() {
		t.Fatalf("saved Markdown = %q, dirty=%v, err=%v", got, d.dirty(), err)
	}
}

func TestMarkdownContextMenuKeepsSelectionAndFocus(t *testing.T) {
	hs, d := editHarness(t, "links.md", "中文")
	hs.a.setMarkdownMode("live")
	hs.settle()
	hs.place(d, 0, 2)
	x, y, height, ok := d.lines.Caret(0)
	if !ok {
		t.Fatal("the selection has no caret geometry")
	}
	origin := caretTrailOrigin(t, hs, d)
	hs.tt.RightClickAt(origin.X+x, origin.Y+y+height/2)
	if err := hs.tt.ChooseMenuItem(locales.CmdMarkdownInsertLink()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, d, "[中文](https://)")
	if selected := d.buf.Slice(min(d.sel[0], d.sel[1]), max(d.sel[0], d.sel[1])); selected != "https://" {
		t.Fatalf("link destination is not selected: %q", selected)
	}
	if !hs.tt.Focused(locales.EditorLabel(d.name())) {
		t.Fatal("link menu did not return focus to its editor")
	}
	hs.tt.Type("https://example.com")
	hs.settle()
	expectText(t, d, "[中文](https://example.com)")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, "[中文](https://)")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, "中文")
	hs.tt.RightClickAt(origin.X+x, origin.Y+y+height/2)
	if err := hs.tt.ChooseMenuItem(locales.MarkdownParagraph(), locales.CmdMarkdownHeading("2")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, d, "## 中文")
	if !hs.tt.Focused(locales.EditorLabel(d.name())) {
		t.Fatal("paragraph menu did not return focus to its editor")
	}
}

func TestMarkdownContextMenuDoesNotFollowAnotherTab(t *testing.T) {
	hs, original := editHarness(t, "original.md", "Original")
	hs.a.setMarkdownMode("live")
	hs.place(original, 0, original.buf.Len())
	ox, oy, oh, ok := original.lines.Caret(original.buf.Len())
	if !ok {
		t.Fatal("the selection has no caret geometry")
	}
	origin := caretTrailOrigin(t, hs, original)
	hs.tt.RightClickAt(origin.X+ox, origin.Y+oy+oh/2)
	hs.settle()
	if len(hs.tt.Menu()) == 0 {
		t.Fatal("formatting menu did not open")
	}
	if err := os.WriteFile(hs.path("other.md"), []byte("Other"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.settings.MarkdownMode = "live"
	hs.a.openFile(hs.path("other.md"), true)
	hs.settle()
	other := hs.a.activeEditor()
	if other == original {
		t.Fatal("second tab did not open")
	}
	// Dismissing the old menu is valid; a queued choice must never edit
	// the document that took its place.
	if len(hs.tt.Menu()) > 0 {
		if err := hs.tt.ChooseMenuItem(locales.CmdMarkdownInsertLink()); err != nil {
			t.Fatal(err)
		}
	}
	hs.settle()
	expectText(t, other, "Other")
}

func TestMarkdownContextMenuTargetsItsOwnSplit(t *testing.T) {
	hs, left, right := splitHarness(t, func(hs *harness) string {
		path := hs.path("split.md")
		if err := os.WriteFile(path, []byte("左侧\n\n右侧"), 0o644); err != nil {
			t.Fatal(err)
		}
		hs.a.settings.MarkdownMode = "live"
		return path
	})
	hs.place(left, 0, 2)
	hs.place(right, 4, 6)
	right.vscroll.Focus()
	hs.settle()
	if hs.a.activeEditor() != right {
		t.Fatal("test did not start with the other split focused")
	}
	lx, ly, lh, ok := left.lines.Caret(1)
	if !ok {
		t.Fatal("the other split has no caret geometry")
	}
	leftOrigin := caretTrailOrigin(t, hs, left)
	hs.tt.RightClickAt(leftOrigin.X+lx, leftOrigin.Y+ly+lh/2)
	if err := hs.tt.ChooseMenuItem(locales.CmdMarkdownBold()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.activeEditor() != left {
		t.Fatal("the menu kept the keyboard in the other split")
	}
	expectText(t, left, "**左侧**\n\n右侧")
	expectText(t, right, "**左侧**\n\n右侧")
	expectSel(t, left, 2, 4)
	expectSel(t, right, 8, 10)
	hs.tt.Type("本栏")
	hs.settle()
	expectText(t, left, "**本栏**\n\n右侧")
}

func TestMarkdownPreviewButtonTargetsItsOwnSplit(t *testing.T) {
	hs, left, right := splitHarness(t, func(hs *harness) string {
		return hs.path("README.md")
	})
	app, tester := hs.a, hs.tt
	app.setMarkdownMode("live")
	hs.place(left, 2, 5)
	hs.place(right, 8, 11)
	right.vscroll.Focus()
	hs.settle()
	if err := tester.Click(app.withKey(locales.ShowPreview(), "view.toggleLivePreview")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if app.activeEditor() != left || !app.livePreview(left) || !app.livePreview(right) {
		t.Fatal("clicking the left button toggled the focused split")
	}
	expectSel(t, left, 2, 5)
	expectSel(t, right, 8, 11)
	app.run("view.togglePreview")
	hs.settle()
	if app.livePreview(left) || !app.livePreview(right) {
		t.Fatal("the menu did not target the split whose switch was clicked")
	}
}

func TestMarkdownOpenAsMenuSetsDefault(t *testing.T) {
	hs, current := editHarness(t, "current.md", "current")
	for _, target := range []struct {
		command, file string
		live          bool
	}{
		{"view.markdownOpen.live", "new-live.markdown", true},
		{"view.markdownOpen.source", "new-source.mdx", false},
	} {
		menu := hs.a.menu(false, func(id string, _ *mygo.Window) { hs.a.run(id) })
		item := menu.ItemByID(target.command)
		if item == nil || item.Click == nil {
			t.Fatalf("Open Markdown As lacks %s", target.command)
		}
		wasLive := current.live
		item.Click(item, nil)
		hs.settle()
		if current.live != wasLive || !hs.a.menu(false, nil).ItemByID(target.command).Checked {
			t.Fatal("choosing an opening preference should mark it and preserve the current tab")
		}
		path := hs.path(target.file)
		if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}
		hs.a.openFile(path, true)
		hs.settle()
		current = hs.a.activeEditor()
		if current.live != target.live || current.preview != nil {
			t.Fatalf("%s ignored %s", target.file, target.command)
		}
	}
}

func TestMarkdownContextUnavailableOutsideEditableMarkdown(t *testing.T) {
	hs, d := editHarness(t, "offline.md", "中文")
	hs.a.setMarkdownMode("live")
	hs.place(d, 0, 2)
	path := d.path
	d.path = remote.Path("markdown-offline.invalid", "/notes.md")
	hs.settle()
	if !d.locked() {
		t.Fatal("disconnected document is not read-only")
	}
	x, y, height, ok := d.lines.Caret(1)
	if !ok {
		t.Fatal("the document has no caret geometry")
	}
	origin := caretTrailOrigin(t, hs, d)
	hs.tt.RightClickAt(origin.X+x, origin.Y+y+height/2)
	if err := hs.tt.ChooseMenuItem(locales.CmdMarkdownBold()); err == nil {
		t.Fatal("offline Markdown offered an enabled format action")
	}
	hs.tt.CloseMenu()
	expectText(t, d, "中文")
	hs.a.run("markdown.bold")
	hs.settle()
	expectText(t, d, "中文")
	d.path = path
	hs.a.openFile(hs.path("main.go"), true)
	hs.settle()
	r := hs.detailBounds(locales.EditorLabel("main.go"))
	hs.tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
	if slices.Contains(hs.tt.Menu(), locales.CmdMarkdownBold()) {
		t.Fatal("non-Markdown editor offers Markdown formatting")
	}
}

func TestMarkdownEditingRenderedTextWithPointer(t *testing.T) {
	source := "# 标题\n\n中文段落\n\n- 清单\n"
	for _, word := range []string{"标题", "段落", "清单"} {
		t.Run(word, func(t *testing.T) {
			hs, d := editHarness(t, "pointer.md", source)
			hs.a.setMarkdownMode("live")
			hs.place(d, d.buf.Len(), d.buf.Len())
			at := len([]rune(source[:strings.Index(source, word)])) + 1
			x, y, height, ok := d.lines.Caret(at)
			if !ok {
				t.Fatal("rendered text has no caret geometry")
			}
			origin := caretTrailOrigin(t, hs, d)
			hs.tt.ClickAt(origin.X+x, origin.Y+y+height/2)
			hs.settle()
			expectSel(t, d, at, at)
			hs.tt.Type("改")
			hs.settle()
			runes := []rune(source)
			expectText(t, d, string(runes[:at])+"改"+string(runes[at:]))
			hs.key(0, ui.KeyBackspace)
			expectText(t, d, source)
			hs.tt.SetClipboard("粘贴")
			hs.command("paste")
			expectText(t, d, string(runes[:at])+"粘贴"+string(runes[at:]))
		})
	}
}

func TestMarkdownEditingPointerSelectionsStayExpanded(t *testing.T) {
	const source = "# Intro\n\nalpha bravo charlie\n\nend\n"
	for _, gesture := range []string{"drag", "shift click", "double click", "drag then focus loss", "drag then source"} {
		t.Run(gesture, func(t *testing.T) {
			hs, d := editHarness(t, "selection.md", source)
			hs.a.setMarkdownMode("live")
			hs.place(d, d.buf.Len(), d.buf.Len())
			point := func(at int) ui.Point {
				t.Helper()
				x, y, h, ok := d.lines.Caret(at)
				if !ok {
					t.Fatalf("offset %d has no rendered caret", at)
				}
				origin := caretTrailOrigin(t, hs, d)
				return ui.Point{X: origin.X + x, Y: origin.Y + y + h/2}
			}
			start := strings.Index(source, "alpha")
			end := strings.Index(source, "bravo") + len("bravo")
			from := point(start)
			switch gesture {
			case "shift click":
				hs.tt.ClickAt(from.X, from.Y)
				hs.settle()
				to := point(end)
				hs.tt.ClickAtWith(ui.Shift, to.X, to.Y)
			case "double click":
				start = strings.Index(source, "bravo")
				inside := point(start + 2)
				hs.tt.ClickAt(inside.X, inside.Y)
				hs.tt.ClickAt(inside.X, inside.Y)
			default:
				hs.tt.Press(from.X, from.Y)
				to := point(end)
				hs.tt.Move(to.X, to.Y)
				hs.tt.Release(to.X, to.Y)
			}
			hs.settle()
			expectSel(t, d, start, end)
			switch gesture {
			case "drag then focus loss":
				hs.tt.SetFocused(false)
				hs.settle()
				hs.tt.SetFocused(true)
				hs.settle()
			case "drag then source":
				hs.a.setMarkdownMode("")
				hs.settle()
				hs.a.setMarkdownMode("live")
				hs.settle()
			}
			expectSel(t, d, start, end)
			hs.tt.Type("replacement")
			hs.settle()
			expectText(t, d, source[:start]+"replacement"+source[end:])
		})
	}
}

func TestMarkdownContextMenuFitsNarrowEditor(t *testing.T) {
	hs, _ := editHarness(t, "narrow.md", "# Narrow\n\nText")
	hs.a.run("view.sidebar")
	hs.a.setMarkdownMode("live")
	for _, width := range []int{240, 180, 160} {
		hs.tt.SetSize(width, 500)
		hs.settle()
		editor := hs.detailBounds(locales.EditorLabel("narrow.md"))
		hs.tt.RightClickAt(editor.X+editor.W/2, editor.Y+editor.H/2)
		for _, label := range []string{locales.MarkdownParagraph(), locales.CmdMarkdownBold(), locales.CmdMarkdownItalic(), locales.CmdMarkdownInlineCode()} {
			if !slices.Contains(hs.tt.Menu(), label) {
				t.Fatalf("width %d: menu lacks %q: %v", width, label, hs.tt.Menu())
			}
		}
		hs.tt.CloseMenu()
	}
}

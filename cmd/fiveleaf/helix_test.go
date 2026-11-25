package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

// helixHarness opens a file of text in helix mode, its first rune
// selected.
func helixHarness(t *testing.T, name, text string) (*harness, *editor) {
	t.Helper()
	hs, d := editHarness(t, name, text)
	hs.a.settings.EditorMode = "helix"
	hs.a.settings.Vim = false
	hs.frames()
	return hs, d
}

// helix presses keys as Helix names them: a character, or <Esc>, <CR>,
// <BS>, <Tab> and <C-x>.
func (hs *harness) helix(keys string) {
	hs.t.Helper()
	for len(keys) > 0 {
		var mods ui.Modifiers
		var key ui.Key
		if end := strings.IndexByte(keys, '>'); keys[0] == '<' && end > 2 && vimKeyName(keys[1:end]) {
			switch name := keys[1:end]; {
			case name == "Esc":
				key = ui.KeyEscape
			case name == "CR":
				key = ui.KeyEnter
			case name == "BS":
				key = ui.KeyBackspace
			case name == "Tab":
				key = ui.KeyTab
			case strings.HasPrefix(name, "C-") && len(name) == 3:
				mods, key = ui.Ctrl, ui.KeyA+ui.Key(name[2]-'a')
			default:
				hs.t.Fatalf("unknown key <%s>", name)
			}
			keys = keys[end+1:]
		} else {
			mods, key = vimKeyOf(hs.t, rune(keys[0]))
			keys = keys[1:]
		}
		hs.tt.Key(mods, key)
		hs.frames()
	}
}

// TestHelixSelectionMoves: a motion keeps a selection, and the selection
// follows the rune it moves over.
func TestHelixSelectionMoves(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two three\nfour\n")
	if !hs.tt.HasText("NORMAL") {
		t.Fatal("the status bar does not show normal mode")
	}
	hs.helix("l")
	if d.sel != [2]int{1, 2} {
		t.Fatalf("after l: selection %v, want [1 2]", d.sel)
	}
	hs.helix("l")
	if d.sel != [2]int{2, 3} {
		t.Fatalf("after ll: selection %v, want [2 3]", d.sel)
	}
	hs.helix("w")
	if d.sel != [2]int{4, 5} {
		t.Fatalf("after w: selection %v, want [4 5]", d.sel)
	}
	hs.helix("b")
	if d.sel != [2]int{0, 1} {
		t.Fatalf("after b: selection %v, want [0 1]", d.sel)
	}
	hs.helix("$")
	if d.sel != [2]int{12, 13} {
		t.Fatalf("after $: selection %v, want [12 13]", d.sel)
	}
}

// TestHelixInsertAndEsc: i inserts before the selection, a after it, and
// Esc leaves the typed rune selected.
func TestHelixInsertAndEsc(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two\n")
	hs.helix("ll")
	hs.helix("i")
	if d.helix.mode != helixInsert || !hs.tt.HasText("INSERT") {
		t.Fatal("i does not show insert mode")
	}
	hs.insert("X")
	hs.helix("<Esc>")
	expectText(t, d, "onXe two\n")
	if d.helix.mode != helixNormal || d.sel != [2]int{2, 3} {
		t.Fatalf("Esc left mode %v and selection %v", d.helix.mode, d.sel)
	}
	hs.helix("a")
	hs.insert("Y")
	hs.helix("<Esc>")
	expectText(t, d, "onXYe two\n")
}

// TestHelixDeleteAndUndo: x deletes the selected runes, and u brings them
// back.
func TestHelixDeleteAndUndo(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two\n")
	hs.helix("w")
	hs.helix("x")
	expectText(t, d, "one wo\n")
	hs.helix("u")
	expectText(t, d, "one two\n")
}

// TestHelixYankAndPaste: y keeps the selection, and p puts it after.
func TestHelixYankAndPaste(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two\n")
	hs.helix("w")
	hs.helix("y")
	if d.helix.yank != "t" {
		t.Fatalf("yank %q", d.helix.yank)
	}
	hs.helix("$")
	hs.helix("p")
	expectText(t, d, "one twot\n")
}

// TestHelixChange: c changes the selection and enters insert mode.
func TestHelixChange(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two\n")
	helixSelect(d, 4, 6)
	hs.frames()
	hs.helix("c")
	hs.insert("TWO")
	hs.helix("<Esc>")
	expectText(t, d, "one TWO\n")
	if d.helix.mode != helixNormal {
		t.Fatalf("mode %v", d.helix.mode)
	}
}

// TestHelixOff: with the setting off, the editor types as usual.
func TestHelixOff(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "hello\n")
	hs.a.toggleHelix()
	hs.settle()
	hs.tt.Type("x")
	hs.settle()
	expectText(t, d, "xello\n")
	if hs.tt.HasText("NORMAL") {
		t.Fatal("the status bar shows a mode with helix mode off")
	}
}

// TestHelixCommandKeys: the keys of normal mode that edit, and the keys
// that open a box, are taken from the text area.
func TestHelixCommandKeys(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two\n")
	hs.helix("A")
	hs.insert("X")
	hs.helix("<Esc>")
	expectText(t, d, "one twoX\n")
	hs.helix("0")
	if d.sel != [2]int{0, 1} {
		t.Fatalf("0 selected %v", d.sel)
	}
	hs.helix("  ")
	hs.frames()
	if !hs.a.quick.open {
		t.Fatal("Space Space does not open Quick Open")
	}
	hs.a.closeQuick()
	hs.frames()
	hs.helix("3l")
	if d.sel != [2]int{3, 4} {
		t.Fatalf("3l selected %v, want [3 4]", d.sel)
	}
}

// TestHelixCommandOn: the command palette checks Helix Keys with the
// editor mode, and Vim Mode only when it is the one on.
func TestHelixCommandOn(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one two\n")
	if hs.a.commandOn("view.vim") || !hs.a.commandOn("view.helix") {
		t.Fatal("helix mode does not show in the command palette")
	}
	hs.a.toggleVim()
	if !hs.a.commandOn("view.vim") || hs.a.commandOn("view.helix") {
		t.Fatal("vim mode does not show in the command palette")
	}
}

// TestHelixClickSelects: a click in normal mode selects the rune the
// area puts the caret on, rather than leave the selection where it was.
func TestHelixClickSelects(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two three\nfour\n")
	d.vscroll.Focus()
	hs.settle()
	r, ok := hs.tt.Find(locales.EditorLabel("a.txt"))
	if !ok {
		t.Fatal("no editor")
	}
	// Click the first rune of the second line: one row down from the text
	// origin, past the gutter and the gap after it.
	x := r.X + hs.a.editorGutterWidth(2) + gutterGap + hs.a.font.charW/2
	y := r.Y + editorPadY + hs.a.font.lineH*1.5
	hs.tt.ClickAt(x, y)
	hs.settle()
	if want := [2]int{14, 15}; d.sel != want {
		t.Fatalf("a click selected %v, want %v", d.sel, want)
	}
	// What follows is a command of the click's selection, not of the
	// first rune.
	hs.helix("x")
	expectText(t, d, "one two three\nour\n")
}

// TestEditorMode: editorMode reads the mode, the older vim setting means
// vim, and the settings offer both.
func TestEditorMode(t *testing.T) {
	for _, tc := range []struct {
		mode string
		vim  bool
		want string
	}{
		{"", false, ""},
		{"", true, "vim"},
		{"vim", false, "vim"},
		{"helix", false, "helix"},
		{"helix", true, "helix"},
		{"other", true, "vim"},
	} {
		p := &settings.Prefs{EditorMode: tc.mode, Vim: tc.vim}
		if got := editorMode(p); got != tc.want {
			t.Errorf("editorMode(%q, vim %v) = %q, want %q", tc.mode, tc.vim, got, tc.want)
		}
	}
	f, ok := settings.FieldOf("editorMode")
	if !ok || f.Category != settings.CatVim || !f.Folder {
		t.Fatalf("editorMode is not a preference of vim the folder may choose: %+v", f)
	}
	if want := []string{"", "vim", "helix"}; !slices.Equal(f.Options, want) {
		t.Errorf("editorMode offers %q, want %q", f.Options, want)
	}
}

// TestToggleEditorMode: vim and helix turn each other off, and each says
// which is on in the menu.
func TestToggleEditorMode(t *testing.T) {
	ph := newPrefsHarness(t)
	a := ph.a
	a.toggleHelix()
	if a.prefs().EditorMode != "helix" || a.prefs().Vim {
		t.Fatalf("helix chose %q with vim %v", a.prefs().EditorMode, a.prefs().Vim)
	}
	a.toggleVim()
	if a.prefs().EditorMode != "" || !a.prefs().Vim {
		t.Fatalf("vim chose %q with vim %v", a.prefs().EditorMode, a.prefs().Vim)
	}
	a.toggleHelix()
	if a.prefs().EditorMode != "helix" || a.prefs().Vim {
		t.Fatalf("helix after vim chose %q with vim %v", a.prefs().EditorMode, a.prefs().Vim)
	}
	a.toggleHelix()
	if a.prefs().EditorMode != "" || a.prefs().Vim {
		t.Fatalf("helix off chose %q with vim %v", a.prefs().EditorMode, a.prefs().Vim)
	}

	check := func(vim, helix bool) {
		t.Helper()
		for _, it := range a.menu(false, func(string, *mygo.Window) {}).Items() {
			if it.Label != locales.MenuView() {
				continue
			}
			got := map[string]bool{}
			for _, sub := range it.Submenu {
				if sub.ID == "view.vim" || sub.ID == "view.helix" {
					got[sub.ID] = sub.Checked
				}
			}
			if got["view.vim"] != vim || got["view.helix"] != helix {
				t.Errorf("the menu checks vim %v and helix %v", got["view.vim"], got["view.helix"])
			}
			return
		}
		t.Fatal("the View menu is missing")
	}
	check(false, false)
	a.toggleHelix()
	check(false, true)
	a.toggleHelix()
	a.toggleVim()
	check(true, false)
}

// TestHelixSetting: editorMode=helix is read from the settings, an older
// vim=true still means vim, and the folder may choose the mode.
func TestHelixSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"editorMode": "helix"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := settings.Load(path)
	if got := editorMode(&s.Prefs); got != "helix" {
		t.Errorf("editorMode from the settings = %q", got)
	}
	if err := os.WriteFile(path, []byte(`{"vim": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s = settings.Load(path)
	if got := editorMode(&s.Prefs); got != "vim" {
		t.Errorf("the older vim setting = %q", got)
	}

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, settings.FolderDir), 0o755)
	f := settings.LoadFolder(dir)
	if err := f.Set("editorMode", json.RawMessage(`"helix"`)); err != nil {
		t.Fatal(err)
	}
	p := f.Apply(settings.Default().Prefs)
	if got := editorMode(&p); got != "helix" {
		t.Errorf("the folder's editorMode = %q", got)
	}
}

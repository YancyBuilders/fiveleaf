package main

import (
	"os"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestLeaderMenuModes(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		for _, live := range []bool{false, true} {
			name := mode
			if live {
				name += "-live"
			}
			t.Run(name, func(t *testing.T) {
				hs, d := editHarness(t, "note.md", "one two three\n")
				hs.a.settings.EditorMode = mode
				if live {
					hs.a.toggleLivePreview()
				}
				hs.frames()
				before := d.sel
				hs.tt.TypeKey(0, ui.KeySpace, " ")
				hs.frames()
				if !hs.a.leader.open || hs.a.quick.open || !hs.tt.HasText(locales.LeaderTitle()) {
					t.Fatal("Space did not open the leader menu")
				}
				hs.tt.TypeKey(0, ui.KeyC, "c")
				hs.frames()
				if hs.a.leader.group != 'c' || !hs.tt.HasText(locales.CmdGoDefinition()) {
					t.Fatal("code submenu is missing")
				}
				hs.key(0, ui.KeyLeft)
				if hs.a.leader.group != 0 || !hs.a.leader.open {
					t.Fatal("Left did not return to the root menu")
				}
				hs.key(0, ui.KeyEscape)
				if hs.a.leader.open || d.sel != before {
					t.Fatalf("cancel changed the selection: %v -> %v", before, d.sel)
				}
				expectText(t, d, "one two three\n")
				hs.key(0, ui.KeyL)
				if d.sel == before {
					t.Fatal("cancel did not return focus to the editor")
				}
			})
		}
	}
}

func TestLeaderMenuCommands(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := editHarness(t, "a.txt", "one two\n")
			hs.a.settings.EditorMode = mode
			hs.frames()
			hs.vim(" ff")
			if hs.a.leader.open || !hs.a.quick.open || hs.a.quick.query != "" {
				t.Fatal("Space f f did not open the file picker")
			}
			hs.key(0, ui.KeyEscape)
			hs.vim(" p")
			if !hs.a.quick.open || hs.a.quick.query != ">" {
				t.Fatal("Space p did not open the command palette")
			}
			hs.key(0, ui.KeyEscape)
			hs.vim(" cl")
			if !hs.a.lspMenu.open || hs.a.leader.open {
				t.Fatal("Space c l did not open language server states")
			}
			hs.key(0, ui.KeyEscape)
			expectText(t, d, "one two\n")
		})
	}
}

func TestLeaderMenuCommandKeysDoNotTypeInPicker(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := editHarness(t, "a.txt", "one two\n")
			hs.a.settings.EditorMode = mode
			hs.frames()
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			hs.tt.TypeKey(0, ui.KeyP, "p")
			hs.frames()
			if hs.a.leader.open || !hs.a.quick.open || hs.a.quick.query != ">" {
				t.Fatalf("Space p leaked into the palette: %q", hs.a.quick.query)
			}
			hs.tt.TypeKey(0, ui.KeyX, "x")
			hs.frames()
			if hs.a.quick.query != ">x" {
				t.Fatalf("palette input remained held: %q", hs.a.quick.query)
			}
			hs.key(0, ui.KeyEscape)
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			if hs.a.leader.open || !hs.a.quick.open || hs.a.quick.query != "" {
				t.Fatalf("Space Space leaked into the file picker: %q", hs.a.quick.query)
			}
			hs.tt.TypeKey(0, ui.KeyX, "x")
			hs.frames()
			if hs.a.quick.query != "x" {
				t.Fatalf("file picker input remained held: %q", hs.a.quick.query)
			}
			expectText(t, d, "one two\n")
		})
	}
}

func TestLeaderMenuMouseAndArrows(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one\n")
	hs.helix(" ")
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyEnter)
	if !hs.a.quick.open || hs.a.quick.query != ">" {
		t.Fatal("Down/Enter did not choose the command palette")
	}
	hs.key(0, ui.KeyEscape)
	hs.helix(" ")
	if err := hs.tt.Click(locales.LeaderCode()); err != nil {
		t.Fatal(err)
	}
	hs.frames()
	if hs.a.leader.group != 'c' {
		t.Fatal("click did not enter the code submenu")
	}
	hs.key(0, ui.KeyBackspace)
	if !hs.a.leader.open || hs.a.leader.group != 0 {
		t.Fatal("Backspace did not return to the root menu")
	}
	if err := hs.tt.Click(locales.CmdGoCommands()); err != nil {
		t.Fatal(err)
	}
	hs.frames()
	if !hs.a.quick.open || hs.a.leader.open {
		t.Fatal("click did not execute the command")
	}
}

func TestLeaderMenuPreservesPendingVimKeys(t *testing.T) {
	for _, tc := range []struct {
		keys, text string
		cursor     int
	}{
		{"f ", "one two\n", 3},
		{"r ", " ne two\n", 0},
		{"d ", "ne two\n", 0},
		{"2 ", "one two\n", 2},
	} {
		t.Run(tc.keys, func(t *testing.T) {
			hs, d := vimHarness(t, "a.txt", "one two\n")
			hs.vim(tc.keys)
			if hs.a.leader.open {
				t.Fatal("Space interrupted a pending Vim command")
			}
			expectText(t, d, tc.text)
			expectSel(t, d, tc.cursor, tc.cursor)
		})
	}
}

func TestLeaderMenuPreservesVisualSelection(t *testing.T) {
	hs, d := vimHarness(t, "a.txt", "one two\n")
	hs.vim("vll")
	before, mode := d.sel, d.vim.mode
	hs.vim(" c<Esc>")
	if d.sel != before || d.vim.mode != mode {
		t.Fatalf("menu changed visual selection: %v %v -> %v %v", before, mode, d.sel, d.vim.mode)
	}
	hs.vim("y")
	if d.vim.mode != vimNormal {
		t.Fatal("visual mode did not retain the keyboard after cancellation")
	}
}

func TestLeaderMenuPreservesPendingHelixKeys(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one two\n")
	hs.helix("r ")
	if hs.a.leader.open {
		t.Fatal("Space interrupted Helix replacement")
	}
	expectText(t, d, " ne two\n")
	hs.helix("2 ")
	if hs.a.leader.open || !hs.a.quick.open {
		t.Fatal("counted Space no longer follows the existing Helix command")
	}
}

func TestLeaderMenuLeavesTextInputsAlone(t *testing.T) {
	for _, mode := range []string{"", "vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := editHarness(t, "a.txt", "one\n")
			hs.a.settings.EditorMode = mode
			hs.frames()
			if mode != "" {
				hs.vim("i")
			}
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			expectText(t, d, " one\n")
			if hs.a.leader.open {
				t.Fatal("typing a space opened the leader menu")
			}
			if mode != "" {
				hs.vim("<Esc>")
			}
			hs.a.openQuick(">")
			hs.frames()
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			if hs.a.quick.query != "> " || hs.a.leader.open {
				t.Fatalf("Space in a text field: query %q, menu %v", hs.a.quick.query, hs.a.leader.open)
			}
		})
	}
}

func TestLeaderMenuCustomSpaceBinding(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one\n")
	withKeys(t, hs.a, `[{"key":"space","command":"go.commands","when":"editorTextFocus"}]`)
	hs.helix(" ")
	if hs.a.leader.open || !hs.a.quick.open || hs.a.quick.query != ">" {
		t.Fatal("the default leader overrode a custom Space binding")
	}
}

func TestLeaderMenuTextAndRepeatDoNotRunCommands(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one\n")
	hs.helix(" ")
	for _, ev := range []ui.InputEvent{
		{Kind: ui.InputKeyDown, Key: ui.KeySpace, Repeat: true},
		{Kind: ui.InputKeyDown, Key: ui.KeyF, Repeat: true},
		{Kind: ui.InputText, Text: " "},
		{Kind: ui.InputCompose, Text: "字"},
	} {
		if !hs.a.leaderInput(ev) || !hs.a.leader.open || hs.a.leader.group != 0 {
			t.Fatalf("event changed the menu: %+v", ev)
		}
	}
	hs.frames()
	hs.key(0, ui.KeyZ)
	if !hs.a.leader.open {
		t.Fatal("an unbound key closed the menu")
	}
	expectText(t, d, "one\n")
}

func TestLeaderMenuCancelsACommandKeyBeforeRelease(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one\n")
	hs.helix(" ")
	hs.tt.KeyDown(ui.KeyP)
	hs.frames()
	if !hs.a.leader.open || hs.a.quick.open {
		t.Fatal("the menu released focus before the command key was released")
	}
	hs.key(0, ui.KeyEscape)
	hs.tt.KeyUp(ui.KeyP)
	hs.frames()
	if hs.a.leader.open || hs.a.quick.open {
		t.Fatal("a cancelled command ran when its key was released")
	}
}

func TestLeaderMenuClosesWhenEditorChanges(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one\n")
	hs.helix(" ")
	hs.a.settings.EditorMode = "vim"
	hs.frames()
	if hs.a.leader.open {
		t.Fatal("leader menu survived a change of key mode")
	}
	hs.vim(" ")
	hs.a.newFile()
	hs.frames()
	if hs.a.leader.open {
		t.Fatal("leader menu survived a change of editor")
	}
}

func TestLeaderMenuExternalCommandClosesMenu(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one\n")
	hs.helix(" ")
	hs.a.run("go.commands")
	hs.frames()
	if hs.a.leader.open || !hs.a.quick.open {
		t.Fatal("an external command left the leader menu over the palette")
	}
}

func TestLeaderMenuLockedDocument(t *testing.T) {
	hs, d := helixHarness(t, "a.txt", "one\n")
	hs.helix(" c")
	d.sync = syncLoading
	hs.frames()
	if hs.a.leaderEnabled(leaderEntry{command: "sel.format"}) {
		t.Fatal("format is enabled for a locked document")
	}
	hs.helix("f")
	if !hs.a.leader.open {
		t.Fatal("a disabled format command ran")
	}
	// The arrows pass over the disabled Format entry.
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyDown)
	if hs.a.leader.sel != 3 {
		t.Fatalf("navigation stopped on a disabled command: %d", hs.a.leader.sel)
	}
	hs.key(0, ui.KeyEscape)
	expectText(t, d, "one\n")
}

func TestLeaderMenuReleasesCommandWhenDocumentLocks(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := editHarness(t, "a.txt", "one\n")
			hs.a.settings.EditorMode = mode
			hs.frames()
			hs.vim(" c")
			hs.tt.KeyDown(ui.KeyF)
			hs.frames()
			if hs.a.leader.pending.command != "sel.format" {
				t.Fatal("format was not waiting for its command key to be released")
			}
			d.sync = syncLoading
			hs.frames()
			hs.tt.KeyUp(ui.KeyF)
			hs.frames()
			if !hs.a.leader.open || hs.a.leader.group != 'c' {
				t.Fatal("format ran after the document became locked")
			}
			hs.key(0, ui.KeyDown)
			if hs.a.leader.sel != 3 {
				t.Fatal("the released command still blocked menu navigation")
			}
			hs.key(0, ui.KeyLeft)
			if !hs.a.leader.open || hs.a.leader.group != 0 {
				t.Fatal("the released command still blocked returning to the root menu")
			}
			expectText(t, d, "one\n")
		})
	}
}

func TestLeaderMenuCommandsExist(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one\n")
	known := map[string]bool{}
	for _, command := range hs.a.commands() {
		known[command.id] = true
	}
	for _, group := range []rune{0, 'f', 's', 'c', 'b', 'w', 'g'} {
		seen := map[rune]bool{}
		for _, entry := range leaderEntries(group, locales.In(i18n.Locale())) {
			if seen[entry.key] || entry.command != "" && !known[entry.command] {
				t.Fatalf("invalid entry in %q: %+v", group, entry)
			}
			seen[entry.key] = true
		}
	}
}

func TestLeaderMenuShot(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("set SHOTS for leader menu screenshots")
	}
	for _, dark := range []bool{false, true} {
		hs := newHarness(t, dark)
		hs.a.settings.EditorMode = "helix"
		hs.a.openFile(hs.path("main.go"), true)
		hs.settle()
		hs.helix(" ")
		name := "leader-light"
		if dark {
			name = "leader-dark"
		}
		hs.shot(name)
		hs.helix("c")
		hs.shot(name + "-code")
	}
}

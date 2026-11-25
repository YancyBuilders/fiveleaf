package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func windowKeysHarness(t *testing.T, mode string) (*harness, *editor) {
	t.Helper()
	hs, d := editHarness(t, "window.txt", "one two three\n")
	hs.a.settings.EditorMode = mode
	hs.a.settings.AutoSave = false
	hs.frames()
	return hs, d
}

func windowKey(hs *harness, mods ui.Modifiers, key ui.Key, text string) {
	hs.t.Helper()
	hs.tt.Key(ui.Ctrl, ui.KeyW)
	hs.frames()
	if !hs.a.leader.open || !hs.a.leader.windowPrefix || !hs.tt.HasText("Ctrl-w") {
		hs.t.Fatalf("Ctrl-W did not show the window keys: open=%v prefix=%v columns=%d docs=%d", hs.a.leader.open, hs.a.leader.windowPrefix, len(hs.a.cols), len(hs.a.docs))
	}
	hs.tt.TypeKey(mods, key, text)
	hs.frames()
}

func TestWindowKeysSplitAndFocus(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, original := windowKeysHarness(t, mode)
			a := hs.a
			before := original.sel
			windowKey(hs, 0, ui.KeyV, "v")
			if len(a.cols) != 2 || a.col != 1 || a.activeEditor().document != original.document || a.leader.open {
				t.Fatal("window v did not split the document into a new IDE column")
			}
			windowKey(hs, ui.Ctrl, ui.KeyH, "")
			if a.col != 0 || original.sel != before {
				t.Fatal("Ctrl-held h did not focus the left column without moving its selection")
			}
			windowKey(hs, 0, ui.KeyH, "h")
			if a.col != 0 || !a.leader.open {
				t.Fatal("left boundary wrapped or ran a disabled action")
			}
			hs.key(0, ui.KeyEscape)
			windowKey(hs, 0, ui.KeyL, "l")
			if a.col != 1 {
				t.Fatal("l did not focus the right column")
			}
			windowKey(hs, 0, ui.KeyL, "l")
			if a.col != 1 || !a.leader.open {
				t.Fatal("right boundary wrapped or ran a disabled action")
			}
			hs.key(0, ui.KeyEscape)
			windowKey(hs, ui.Ctrl, ui.KeyW, "")
			if a.col != 0 {
				t.Fatal("Ctrl-W Ctrl-W did not cycle past the last column")
			}
			windowKey(hs, ui.Shift, ui.KeyW, "W")
			if a.col != 1 {
				t.Fatal("W did not cycle to the previous column")
			}
			expectText(t, original, "one two three\n")
		})
	}
}

func TestWindowKeysReturnAndSpaceMenu(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := windowKeysHarness(t, mode)
			if mode == "vim" {
				hs.vim("vll")
			}
			selection := d.sel
			for _, key := range []ui.Key{ui.KeyEscape, ui.KeyBackspace, ui.KeyLeft} {
				hs.tt.Key(ui.Ctrl, ui.KeyW)
				hs.frames()
				hs.key(0, key)
				if hs.a.leader.open || d.sel != selection {
					t.Fatal("leaving Ctrl-W moved the selection or returned to the Space menu")
				}
			}
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			hs.tt.TypeKey(0, ui.KeyW, "w")
			hs.frames()
			if hs.a.leader.group != 'w' || hs.a.leader.windowPrefix || hs.a.leaderAtRoot() {
				t.Fatal("Space w did not enter the ordinary window submenu")
			}
			hs.key(0, ui.KeyLeft)
			if !hs.a.leader.open || hs.a.leader.group != 0 {
				t.Fatal("Space w did not return to the Leader root")
			}
			hs.key(0, ui.KeyEscape)
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			hs.tt.TypeKey(0, ui.KeyW, "w")
			hs.frames()
			hs.tt.TypeKey(0, ui.KeyV, "v")
			hs.frames()
			if len(hs.a.cols) != 2 {
				t.Fatal("Space w v did not use the same window action")
			}
		})
	}
}

func TestWindowKeysCloseColumn(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := windowKeysHarness(t, mode)
			a := hs.a
			windowKey(hs, ui.Ctrl, ui.KeyV, "")
			a.openFile(hs.path("main.go"), true)
			hs.settle()
			if len(a.column().tabs) != 2 {
				t.Fatal("test needs two tabs in the column being closed")
			}
			dirty := a.activeEditor()
			dirty.edit([2]int{1, 1}, textEdit{0, 0, "x"})
			hs.h.choice = 2
			windowKey(hs, 0, ui.KeyQ, "q")
			if len(a.cols) != 2 || len(a.column().tabs) != 2 || !dirty.dirty() || len(hs.h.asked) != 1 {
				t.Fatal("cancel did not preserve the changed column")
			}
			hs.h.choice = 1
			key, text := ui.KeyQ, "q"
			if mode == "vim" {
				key, text = ui.KeyC, "c"
			}
			windowKey(hs, 0, key, text)
			if len(a.cols) != 1 || a.activeEditor() != d || len(a.cols[0].tabs) != 1 || hs.h.closed {
				t.Fatal("close did not close the whole column while preserving its shared document")
			}
			windowKey(hs, ui.Ctrl, ui.KeyQ, "")
			if len(a.cols) != 1 || len(a.cols[0].tabs) != 0 || hs.h.closed {
				t.Fatal("closing the last editor column closed the IDE window")
			}
		})
	}
}

func TestWindowKeysOnlyColumn(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := windowKeysHarness(t, mode)
			a := hs.a
			windowKey(hs, 0, ui.KeyV, "v")
			keep := a.column()
			a.openFile(hs.path("main.go"), true)
			hs.settle()
			a.closeTab(keep.tabs[0], nil, nil)
			d.edit([2]int{1, 1}, textEdit{0, 0, "x"})
			hs.frames()
			hs.h.choice = 2
			windowKey(hs, 0, ui.KeyO, "o")
			if len(a.cols) != 2 || a.column() != keep || !d.dirty() || len(hs.h.asked) != 1 {
				t.Fatal("Only Column did not preserve the other column when its close was cancelled")
			}
			hs.h.choice = 1
			windowKey(hs, ui.Ctrl, ui.KeyO, "")
			if len(a.cols) != 1 || a.column() != keep || hs.h.closed {
				t.Fatal("Only Column did not retain the chosen IDE column")
			}
		})
	}
}

func TestWindowKeysLockedColumnsAndBalance(t *testing.T) {
	hs, _ := windowKeysHarness(t, "vim")
	a := hs.a
	windowKey(hs, 0, ui.KeyV, "v")
	windowKey(hs, 0, ui.KeyV, "v")
	left, middle, right := a.cols[0], a.cols[1], a.cols[2]
	a.toggleColumnLock(right)
	left.weight, middle.weight, right.weight = 1, 3, 5
	hs.frames()
	windowKey(hs, 0, ui.KeyQ, "q")
	if len(a.cols) != 3 || !a.leader.open || !right.locked {
		t.Fatal("window q closed a locked column")
	}
	hs.key(0, ui.KeyEscape)
	a.focusAdjacentColumn(-1)
	hs.frames()
	windowKey(hs, 0, ui.KeyO, "o")
	if len(a.cols) != 3 || !a.leader.open || !right.locked {
		t.Fatal("Only Column promised to close a protected column")
	}
	hs.key(0, ui.KeyEscape)
	windowKey(hs, 0, ui.KeyEqual, "=")
	if left.weight != 2 || middle.weight != 2 || right.weight != 5 || a.column() != middle {
		t.Fatalf("balance changed locked width or focus: %v, %v, %v", left.weight, middle.weight, right.weight)
	}
}

func TestWindowKeysPersonalBindings(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		for _, tc := range []struct {
			name, bindings string
			before, after  ui.Key
			palette        bool
		}{
			{"single", `[{"key":"ctrl+w","command":"go.commands","when":"editorTextFocus"}]`, 0, 0, true},
			{"prefix", `[{"key":"ctrl+w x","command":"view.splitRight","when":"editorTextFocus"}]`, 0, ui.KeyX, false},
			{"continuation", `[{"key":"ctrl+k ctrl+w","command":"view.splitRight","when":"editorTextFocus"}]`, ui.KeyK, 0, false},
		} {
			t.Run(goos+"/"+tc.name, func(t *testing.T) {
				onOS(t, goos)
				hs, _ := windowKeysHarness(t, "helix")
				useKeymapFixtures(hs)
				hs.a.settings.Keymap = "vscode"
				withKeys(t, hs.a, tc.bindings)
				if tc.before != 0 {
					hs.key(ui.Ctrl, tc.before)
				}
				hs.key(ui.Ctrl, ui.KeyW)
				if hs.a.leader.open {
					t.Fatal("window prefix overrode a personal binding")
				}
				if tc.after != 0 {
					hs.tt.TypeKey(0, tc.after, "x")
					hs.frames()
				}
				if tc.palette {
					if !hs.a.quick.open || hs.a.quick.query != ">" {
						t.Fatal("personal Ctrl-W did not run its command")
					}
				} else if len(hs.a.cols) != 2 || len(hs.a.chord) != 0 {
					t.Fatal("personal chord did not finish")
				}
			})
		}
	}
	hs, _ := windowKeysHarness(t, "vim")
	withKeys(t, hs.a, `[{"key":"ctrl+w","command":"go.commands","when":"terminalFocus"}]`)
	windowKey(hs, 0, ui.KeyV, "v")
	if len(hs.a.cols) != 2 {
		t.Fatal("an inactive personal condition suppressed the window prefix")
	}
}

func TestWindowKeysPreservePlatformCloseTab(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		for _, mode := range []string{"", "vim", "helix"} {
			t.Run(goos+"/"+mode, func(t *testing.T) {
				onOS(t, goos)
				hs, d := windowKeysHarness(t, mode)
				if mode != "" {
					hs.vim("i")
				}
				hs.key(ui.Ctrl, ui.KeyW)
				if hs.a.leader.open || hs.a.activeEditor() == d || hs.h.closed {
					t.Fatal("nonmodal or insert-mode Ctrl-W no longer closes its tab")
				}
			})
		}
	}
}

func TestWindowKeysPlatformPresets(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		for _, preset := range []string{"", "vscode", "sublime"} {
			for _, mode := range []string{"vim", "helix"} {
				t.Run(goos+"/"+preset+"/"+mode, func(t *testing.T) {
					onOS(t, goos)
					hs, d := windowKeysHarness(t, mode)
					useKeymapFixtures(hs)
					hs.a.settings.Keymap = preset
					hs.a.applyKeymap(nil)
					windowKey(hs, ui.Ctrl, ui.KeyV, "")
					if len(hs.a.cols) != 2 || hs.a.activeEditor().document != d.document || hs.h.closed {
						t.Fatal("the platform or preset took Ctrl-W as Close Tab")
					}
					windowKey(hs, ui.Ctrl, ui.KeyH, "")
					if hs.a.col != 0 || hs.a.find.open {
						t.Fatal("the platform or preset took Ctrl-H instead of focusing left")
					}
					windowKey(hs, ui.Ctrl, ui.KeyL, "")
					if hs.a.col != 1 {
						t.Fatal("the platform or preset took Ctrl-L instead of focusing right")
					}
					windowKey(hs, ui.Ctrl, ui.KeyW, "")
					if hs.a.col != 0 || len(hs.a.cols) != 2 || hs.h.closed {
						t.Fatal("the platform or preset took the second Ctrl-W instead of cycling columns")
					}
				})
			}
		}
	}
}

func TestWindowKeysPreservePendingAndRecording(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			onOS(t, "windows")
			hs, d := windowKeysHarness(t, mode)
			useKeymapFixtures(hs)
			hs.a.settings.Keymap = "vscode"
			hs.a.applyKeymap(nil)
			hs.tt.TypeKey(0, ui.KeyR, "r")
			hs.frames()
			hs.key(ui.Ctrl, ui.KeyW)
			if hs.a.leader.open || hs.a.activeEditor() != d {
				t.Fatal("Ctrl-W interrupted a pending replacement or closed its tab")
			}
			hs.tt.TypeKey(0, ui.KeyX, "x")
			hs.frames()
			expectText(t, d, "xne two three\n")
			if mode == "vim" {
				hs.vim("qa")
				hs.key(ui.Ctrl, ui.KeyW)
				if hs.a.leader.open || hs.a.activeEditor() != d || hs.a.vimRec != 'a' {
					t.Fatal("Ctrl-W interrupted recording or closed its tab")
				}
				hs.vim("lq")
			}
		})
	}
}

func TestWindowKeysLeaveOtherInputAlone(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := windowKeysHarness(t, mode)
			hs.vim("i")
			hs.key(ui.Ctrl, ui.KeyW)
			if hs.a.leader.open {
				t.Fatal("insert-mode Ctrl-W opened window keys")
			}
			hs.tt.TypeKey(0, ui.KeyX, "x")
			hs.frames()
			if d.buf.String() != "xone two three\n" {
				t.Fatal("insert input did not keep working")
			}
			hs.vim("<Esc>")
			hs.a.openQuick(":")
			hs.frames()
			hs.key(ui.Ctrl, ui.KeyW)
			if hs.a.leader.open || !hs.a.quick.open {
				t.Fatal("a text field opened window keys")
			}
			hs.tt.TypeKey(0, ui.Key1, "1")
			hs.frames()
			if hs.a.quick.query != ":1" {
				t.Fatal("text input did not keep working")
			}
		})
	}
	// The terminal's input path uses takeKeys, not editorKeys, even while
	// an editor elsewhere in the same window is in a modal mode.
	hs, _ := windowKeysHarness(t, "vim")
	var received []ui.Key
	var terminal ui.Handle
	tt := ui.NewTester(func(c *ui.Context) {
		hs.a.takeKeys(ui.Box(c).Bind(&terminal).Width(200).Height(100).Focusable(), placeTerminal, func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown {
				received = append(received, ev.Key)
			}
			return true
		})
	}, 200, 100)
	tt.Frame()
	terminal.Focus()
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyW)
	if hs.a.leader.open || !slices.Contains(received, ui.KeyW) {
		t.Fatal("terminal input did not receive Ctrl-W")
	}
}

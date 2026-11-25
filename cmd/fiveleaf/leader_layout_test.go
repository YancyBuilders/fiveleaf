package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestLeaderHintPanelLayout(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans"} {
		t.Run(locale, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			hs, _ := helixHarness(t, "a.txt", "one two\n")
			hs.tt.SetLocale(locale)
			for _, size := range []struct{ width, height int }{{1200, 760}, {300, 400}, {300, 220}, {260, 180}} {
				t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
					hs.tt.SetSize(size.width, size.height)
					for _, group := range []rune{0, 'c', 'w'} {
						hs.a.openLeaderMenu()
						hs.a.leader.group = group
						hs.frames()
						box, found := hs.tt.Find(locales.LeaderTitle())
						if !found {
							t.Fatal("the leader hint panel is missing")
						}
						if box.X+box.W != float32(size.width)-8 || box.Y+box.H != float32(size.height)-statusBarH-8 {
							t.Errorf("group %q is not above the bottom-right status bar: %v", group, box)
						}
						if box.X < 8 || box.Y < titleBarH+8 || box.W > 320 {
							t.Errorf("group %q escaped the available window: %v", group, box)
						}
						hs.key(0, ui.KeyEscape)
					}
				})
			}
		})
	}
}

func TestLeaderHintPanelShortWindowNavigation(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one two\n")
	hs.tt.SetSize(300, 220)
	hs.helix(" ")
	hs.key(0, ui.KeyUp)
	box, found := hs.tt.Find(locales.LeaderTitle())
	if !found {
		t.Fatal("the leader hint panel is missing")
	}
	row, found := hs.tt.Find(locales.MenuGit())
	if !found || row.Y < box.Y || row.Y+row.H > box.Y+box.H {
		t.Fatalf("the last menu row did not scroll into the panel: panel %v, row %v", box, row)
	}
	hs.key(0, ui.KeyEnter)
	if hs.a.leader.group != 'g' {
		t.Fatal("the last entry could not be chosen in a short window")
	}
	hs.key(0, ui.KeyLeft)
	if !hs.a.leader.open || hs.a.leader.group != 0 {
		t.Fatal("returning from a short submenu did not retain the panel")
	}
}

func TestLeaderHintPanelOutsideClick(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one two\n")
	before := hs.a.window.SidebarClosed
	hs.helix(" ")
	if err := hs.tt.Click(hs.a.withKey(locales.CmdViewSidebar(), "view.sidebar")); err != nil {
		t.Fatal(err)
	}
	hs.frames()
	if hs.a.leader.open || hs.a.window.SidebarClosed == before {
		t.Fatal("the hint panel blocked the status control behind it")
	}
}

func TestLeaderHintPanelOutsideInputKeepsFocus(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, d := editHarness(t, "a.txt", "one two\n")
			hs.a.settings.EditorMode = mode
			hs.a.run("edit.find")
			hs.settle()
			hs.a.find.query = ""
			d.vscroll.Focus()
			hs.frames()
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			if !hs.a.leader.open {
				t.Fatal("Space did not open the leader menu with Find visible")
			}
			box, found := hs.tt.Find(locales.FindLabel())
			if !found {
				t.Fatal("Find is not visible")
			}
			hs.tt.ClickAt(box.X+70, box.Y+18)
			hs.tt.TypeKey(0, ui.KeyX, "x")
			hs.frames()
			if hs.a.leader.open || hs.a.find.query != "x" {
				t.Fatalf("clicking Find lost its input focus: leader=%v query=%q", hs.a.leader.open, hs.a.find.query)
			}
			expectText(t, d, "one two\n")
		})
	}
}

func TestLeaderHintPanelOtherEditorKeepsFocus(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs, left, right := splitHarness(t, func(hs *harness) string { return hs.path("main.go") })
			hs.a.settings.EditorMode = mode
			hs.frames()
			hs.tt.TypeKey(0, ui.KeySpace, " ")
			hs.frames()
			rightBefore := right.sel
			hs.tt.ClickAt(left.lastView.X+left.gutterW+4, left.lastView.Y+editorPadY+hs.a.font.lineH/2)
			hs.frames()
			leftBefore := left.sel
			hs.tt.TypeKey(0, ui.KeyL, "l")
			hs.frames()
			if hs.a.leader.open || hs.a.activeEditor() != left || left.sel == leftBefore || right.sel != rightBefore {
				t.Fatalf("clicking the other editor did not retain its keyboard focus: leader=%v column=%d left=%v→%v right=%v→%v",
					hs.a.leader.open, hs.a.col, leftBefore, left.sel, rightBefore, right.sel)
			}
		})
	}
}

func TestLeaderHintPanelChromeKeepsKeyboard(t *testing.T) {
	for _, target := range []string{"header", "footer"} {
		t.Run(target, func(t *testing.T) {
			hs, _ := helixHarness(t, "a.txt", "one two\n")
			hs.helix(" ")
			box, found := hs.tt.Find(locales.LeaderTitle())
			if !found {
				t.Fatal("the leader hint panel is missing")
			}
			y := box.Y + 13
			if target == "footer" {
				y = box.Y + box.H - 13
			}
			hs.tt.ClickAt(box.X+20, y)
			hs.key(0, ui.KeyEscape)
			if hs.a.leader.open {
				t.Fatalf("clicking the %s prevented Escape from closing the menu", target)
			}
		})
	}
}

func TestLeaderHintPanelDisabledEntryKeepsKeyboard(t *testing.T) {
	hs, _ := helixHarness(t, "a.txt", "one two\n")
	hs.key(ui.Ctrl, ui.KeyW)
	if err := hs.tt.Click(locales.CmdViewFocusLeftColumn()); err != nil {
		t.Fatal(err)
	}
	hs.tt.TypeKey(0, ui.KeyV, "v")
	hs.frames()
	if hs.a.leader.open || len(hs.a.cols) != 2 {
		t.Fatal("clicking the disabled left-column action prevented the split shortcut")
	}
}

func TestLeaderHintPanelShot(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("set SHOTS for leader hint panel screenshots")
	}
	for _, locale := range []string{"en", "zh-Hans"} {
		for _, dark := range []bool{false, true} {
			name := "leader-hint-" + locale + "-light"
			if dark {
				name = "leader-hint-" + locale + "-dark"
			}
			t.Run(name, func(t *testing.T) {
				before := i18n.Locale()
				i18n.SetLocaleFunc(func() string { return locale })
				t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
				hs := newHarness(t, dark)
				hs.tt.SetLocale(locale)
				hs.a.settings.EditorMode = "helix"
				hs.a.openFile(hs.path("main.go"), true)
				hs.settle()
				hs.helix(" ")
				hs.shot(name + "-root")
				hs.helix("c")
				hs.shot(name + "-code")
				hs.key(0, ui.KeyLeft)
				hs.tt.SetSize(300, 220)
				hs.frames()
				hs.shot(name + "-small")
				hs.key(0, ui.KeyEscape)
				hs.tt.SetSize(1200, 760)
				hs.a.splitRight()
				hs.settle()
				hs.key(ui.Ctrl, ui.KeyW)
				if len(hs.a.cols) != 2 || !hs.a.leader.open || !hs.a.leaderAtRoot() {
					t.Fatal("Ctrl-W did not open the window menu above two editor columns")
				}
				hs.shot(name + "-windows")
			})
		}
	}
}

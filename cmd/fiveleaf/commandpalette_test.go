package main

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestCommandPaletteLayout(t *testing.T) {
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
	for _, tc := range []struct {
		name          string
		width, height int
		dark          bool
	}{{"light", 1440, 900, false}, {"dark", 1440, 900, true}, {"narrow", 390, 400, true}} {
		t.Run(tc.name, func(t *testing.T) {
			hs := newHarness(t, tc.dark)
			a, tt := hs.a, hs.tt
			tt.SetSize(tc.width, tc.height)
			a.openFile(hs.path("main.go"), true)
			for _, id := range []string{"go.quickOpen", "file.save", "view.wordWrap"} {
				a.rememberCommand(id)
			}
			a.run("go.commands")
			hs.settle()
			hs.shot("palette-" + tc.name)
			if dialog, ok := tt.Find(locales.PaletteLabel()); ok && tc.width > 1000 {
				hs.shotRect("palette-detail-"+tc.name, image.Rect(int(dialog.X)-20, int(dialog.Y)-12, int(dialog.X+dialog.W)+20, int(dialog.Y+dialog.H)+24), 1)
			}
			if !tt.HasText(locales.PalettePlaceholder()) || !tt.HasText(locales.PickRecent()) {
				t.Error("the command prompt or the recent section is missing")
			}
			footer, ok := tt.Find(locales.TaskRun())
			if !ok || footer.Y+footer.H > float32(tc.height) {
				t.Errorf("run action is outside a %dpx window: %+v", tc.height, footer)
			}
			tt.Compose("bao", 3)
			if tt.HasText(locales.PalettePlaceholder()) {
				t.Error("prompt overlaps IME composition")
			}
			tt.Type("保存")
			hs.settle()
			hs.shot("palette-search-" + tc.name)
			if len(a.quick.results) < 2 || tt.HasText(locales.PalettePlaceholder()) || tt.HasText(locales.PickRecent()) {
				t.Fatalf("a typed search still shows the prompt or history, or lost matches (%d)", len(a.quick.results))
			}
			marker := func(index int) int {
				title := a.paletteEntries()[a.quick.results[index].Index].title
				row, ok := tt.Find(title)
				if !ok {
					t.Fatalf("missing result %q", title)
				}
				img := tt.Image()
				middle := row.Y + row.H/2
				face := img.RGBAAt(int(row.X+8), int(middle))
				return pickerMarkerPixels(img, ui.Rect{X: row.X + 1, Y: middle - 5, W: 5, H: 10}, ui.RGB(face.R, face.G, face.B))
			}
			if marker(0) < 10 || marker(1) > 3 {
				t.Fatal("only the initial Return target should have a leading marker")
			}
			tt.Key(0, ui.KeyDown)
			if a.quick.sel != 1 || marker(0) > 3 || marker(1) < 10 {
				t.Fatal("Down did not move the visible marker to the second match")
			}
			tt.Type(strings.Repeat("z", 80))
			hs.settle()
			hs.shot("palette-empty-" + tc.name)
			tt.Key(0, ui.KeyEnter)
			if !a.quick.open || !tt.HasText(locales.PaletteNoMatches()) {
				t.Fatal("Enter on an empty result lost the search")
			}
			tt.Key(0, ui.KeyEscape)
			if a.quick.open {
				t.Fatal("Escape did not dismiss the empty search")
			}
		})
	}
}

func TestCommandPaletteRunsACommand(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.run("go.commands")
	hs.settle()
	if !a.quick.open || a.quick.query != ">" {
		t.Fatalf("palette open %v with %q", a.quick.open, a.quick.query)
	}
	tt.Type("wordwrap")
	hs.settle()
	hs.shot("command-palette")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if a.quick.open {
		t.Error("the palette stayed open")
	}
	if !a.settings.WordWrap {
		t.Error("View: Word Wrap did not turn word wrap on")
	}
	// The command run last comes first.
	a.run("go.commands")
	hs.settle()
	if e := a.paletteEntries()[a.quick.results[0].Index]; e.id != "view.wordWrap" {
		t.Errorf("first command %s, want the one run last", e.id)
	}
	tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.quick.open {
		t.Error("Escape left the palette open")
	}
}

func TestCommandPaletteControlsAndPrefix(t *testing.T) {
	hs := newHarness(t, false)
	a, tt := hs.a, hs.tt
	a.openQuick("")
	hs.settle()
	tt.Type(">")
	hs.settle()
	if !tt.HasText(locales.PalettePlaceholder()) {
		t.Fatal("typing > in Quick Open did not show the command prompt")
	}
	tt.Key(0, ui.KeyBackspace)
	hs.settle()
	if !a.quick.open || a.quick.query != "" || !tt.Focused(locales.QuickOpenLabel()) {
		t.Fatal("deleting the prefix did not return focus to file search")
	}
	tt.Type(">wordwrap")
	hs.settle()
	if err := tt.Click(locales.TaskRun()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.quick.open || !a.settings.WordWrap {
		t.Fatal("the run button did not execute the selected command")
	}
	a.run("go.commands")
	hs.settle()
	if err := tt.Click(locales.ButtonClose()); err != nil {
		t.Fatal(err)
	}
	if a.quick.open || !a.settings.WordWrap {
		t.Fatal("closing the palette ran a command or left the palette open")
	}
}

func TestCommandPaletteRowClickAndBackdrop(t *testing.T) {
	hs, doc := editHarness(t, "note.txt", "")
	a, tt := hs.a, hs.tt
	a.rememberCommand("view.wordWrap")
	a.rememberCommand("file.save")
	a.run("go.commands")
	hs.settle()
	second := a.paletteEntries()[a.quick.results[1].Index]
	if second.id != "view.wordWrap" || a.quick.sel != 0 {
		t.Fatalf("expected Word Wrap below the selected command, got %q at selection %d", second.id, a.quick.sel)
	}
	if err := tt.Click(second.title); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.quick.open || !a.settings.WordWrap || a.quick.used[0] != second.id {
		t.Fatal("clicking the second row did not run that command and move it to the top of history")
	}
	a.run("go.commands")
	hs.settle()
	panel, ok := tt.Find(locales.PaletteLabel())
	if !ok {
		t.Fatal("missing command palette")
	}
	tt.ClickAt(panel.X-4, panel.Y+20)
	hs.settle()
	if a.quick.open || !a.settings.WordWrap {
		t.Fatal("clicking the backdrop should dismiss the palette without running the selected command")
	}
	tt.Type("x")
	expectText(t, doc, "x")
}

func TestAccelCaps(t *testing.T) {
	for _, c := range []struct {
		accel string
		mac   bool
		want  []string
	}{
		{"Shift+CmdOrCtrl+P", true, []string{"⇧", "⌘", "P"}},
		{"Alt+CmdOrCtrl+Right", true, []string{"⌥", "⌘", "→"}},
		{"Ctrl+`", true, []string{"⌃", "`"}},
		{"Shift+Alt+F5", true, []string{"⌥", "⇧", "F5"}},
		{"CmdOrCtrl+=", true, []string{"⌘", "="}},
		{"Shift+CmdOrCtrl+P", false, []string{"Ctrl", "Shift", "P"}},
		{"", true, nil},
	} {
		if got := accelCaps(c.accel, c.mac); !slices.Equal(got, c.want) {
			t.Errorf("accelCaps(%q, %v) = %q, want %q", c.accel, c.mac, got, c.want)
		}
	}
}

// Every command of the palette names its group, in every language, and
// none shows a colon with nothing before it.
func TestPaletteTitlesHaveGroups(t *testing.T) {
	locale := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return locale }) })
	for _, loc := range []string{"en", "ja", "zh-Hans"} {
		i18n.SetLocaleFunc(func() string { return loc })
		hs := newHarness(t, false)
		hs.a.palette = nil
		for _, e := range hs.a.paletteEntries() {
			if strings.HasPrefix(e.title, ":") || e.group == 0 {
				t.Errorf("%s: %s shows no group: %q", loc, e.id, e.title)
			}
			if strings.HasPrefix(e.id, "account.") {
				t.Errorf("%s: %s is still listed", loc, e.id)
			}
		}
	}
}

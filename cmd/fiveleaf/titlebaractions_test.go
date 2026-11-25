package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestTitleBarQuickActions(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	hs, _ := iconThemePlugin(t)
	withAppearancePlugins(t)
	a, tt := hs.a, hs.tt
	hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0"}
	hs.settle()
	for _, label := range []string{locales.CmdViewSelectTheme(), a.withKey(locales.CmdViewSelectTheme(), "view.selectTheme")} {
		if r, ok := tt.Find(label); ok {
			t.Fatalf("the main window still has a separate theme button: %v", r)
		}
	}
	r, ok := tt.Find(locales.PrefsTitle())
	if !ok || r.X < 600 || r.Y+r.H > titleBarH {
		t.Fatalf("the settings button is not at the top right: %v (shown %v)", r, ok)
	}
	if err := tt.Click(locales.PrefsTitle()); err != nil {
		t.Fatal(err)
	}
	wantMenu := []string{
		locales.CmdGoCommands(), "-",
		locales.CmdFileSettingsJSON(), locales.CmdFileSettings(), locales.CmdPluginsManage(), locales.CmdFileKeybindings(),
		locales.MenuTheme(), locales.MenuView(), "-", locales.MenuUpdateTo("0.22.0"),
	}
	if !slices.Equal(tt.Menu(), wantMenu) {
		t.Fatalf("settings menu = %q, want grouped entries %q", tt.Menu(), wantMenu)
	}
	tt.CloseMenu()
	// The welcome page no longer duplicates the bottom layout controls.
	checkStatusControls(t, hs, 1200, 760)
	choose := func(path ...string) {
		t.Helper()
		if err := tt.Click(locales.PrefsTitle()); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(path...); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	choose(locales.MenuTheme(), locales.CmdViewSelectTheme())
	if !a.themes.open {
		t.Fatal("the settings menu did not open the theme picker")
	}
	hs.key(0, ui.KeyEscape)
	hs.settle()

	theme, appearance := a.settings.Theme, a.settings.Appearance
	for _, choice := range []struct{ label, id string }{
		{"Catppuccin Mocha", "fiveleaf-icons.catppuccin-mocha"},
		{"Ayu", "fiveleaf-icons.ayu"},
		{"Bright icons", "icon-test.bright"},
		{"FiveLeaf", "fiveleaf-icons.fiveleaf"},
	} {
		gen := a.settings.Gen()
		choose(locales.MenuTheme(), locales.CmdViewSelectIconTheme())
		if !a.iconPicker.open {
			t.Fatal("the settings menu did not open the file icon picker")
		}
		tt.Type(choice.label)
		hs.settle()
		hs.key(0, ui.KeyEnter)
		hs.settle()
		if a.settings.IconTheme != choice.id || a.settings.Gen() == gen {
			t.Fatalf("the settings menu selected %q, want %q, settings generation %d → %d", a.settings.IconTheme, choice.id, gen, a.settings.Gen())
		}
		got, _ := a.fileIcon("main.go")
		want, _ := fileIconForTheme(a.pal, choice.id, "main.go")
		if got != want {
			t.Fatalf("the window did not switch to icon theme %q", choice.id)
		}
	}
	if a.settings.Theme != theme || a.settings.Appearance != appearance {
		t.Fatal("the icon menu changed the color theme or appearance")
	}

	for _, label := range []string{locales.CmdFileSettings(), locales.CmdFileKeybindings(), locales.CmdPluginsManage()} {
		choose(label)
	}
	if want := []string{":", prefsShortcuts + ":", prefsPlugins + ":"}; !slices.Equal(hs.h.settingsShown, want) {
		t.Fatalf("settings routes = %q, want %q", hs.h.settingsShown, want)
	}
	closed := a.window.SidebarClosed
	choose(locales.MenuView(), locales.CmdViewSidebar())
	if a.window.SidebarClosed == closed {
		t.Error("the settings menu did not toggle the sidebar")
	}
	choose(locales.MenuUpdateTo("0.22.0"))
	if !a.updatesOpen {
		t.Fatal("the settings menu did not open the update dialog")
	}
	hs.key(0, ui.KeyEscape)
	choose(locales.CmdGoCommands())
	if !a.quick.open || a.quick.query != ">" {
		t.Fatal("the settings menu did not open the command palette")
	}
}

func TestTitleBarActionsFitWindowControls(t *testing.T) {
	for _, platform := range []struct {
		os  string
		bar ui.TitleBar
	}{
		{"darwin", ui.TitleBar{Height: titleBarH, Left: 80}},
		{"windows", ui.TitleBar{Height: titleBarH, Right: 138}},
		{"linux", ui.TitleBar{Height: titleBarH, Right: 110}},
	} {
		t.Run(platform.os, func(t *testing.T) {
			onOS(t, platform.os)
			hs := newHarness(t, true)
			hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0"}
			tt := hs.tt
			for _, width := range []float32{640, 1200} {
				tt.SetSize(int(width), 760)
				tt.SetTitleBar(platform.bar)
				hs.settle()
				if launcher, found := tt.Find(locales.AgentRoster()); found && launcher.Y < titleBarH {
					t.Errorf("width %v: agent list is in the title bar: %v", width, launcher)
				}
				labels := []string{locales.PrefsTitle(), locales.UpdateAvailable("0.22.0")}
				if platform.os != "darwin" {
					labels = append(labels, locales.Menu())
				}
				bounds := hs.layoutOf(labels...)
				for i, label := range labels {
					r := bounds[label]
					if r.X < platform.bar.Left || r.X+r.W > width-platform.bar.Right || r.Y < 0 || r.Y+r.H > titleBarH {
						t.Errorf("width %v: %q overlaps window controls: %v", width, label, r)
					}
					for _, other := range labels[i+1:] {
						if overlaps(r, bounds[other]) {
							t.Errorf("width %v: %q overlaps %q: %v, %v", width, label, other, r, bounds[other])
						}
					}
				}
			}
		})
	}
}

package main

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestSidebarActivityTooltipClearsTitleBar(t *testing.T) {
	previous := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
	hs := newHarness(t, false)
	bar := ui.TitleBar{Height: 28, Left: 80}
	hs.tt.SetTitleBar(bar)
	hs.tt.SetLocale("zh-Hans")
	hs.a.settings.MicaEnabled = false
	hs.a.settings.SidebarRight = false
	hs.a.window.SidebarWidth = 240
	hs.settle()
	label := hs.a.withKey(locales.SidebarExplorer(), "view.explorer")
	button := hs.detailBounds(label)
	before := hs.tt.Image().RGBAAt(40, 15)
	hs.tt.Move(button.X+button.W/2, button.Y+button.H/2)
	hs.tt.Advance(time.Second)
	step(hs.tt)
	matches := 0
	for _, text := range hs.tt.Texts() {
		if text == label {
			matches++
		}
	}
	if matches != 2 {
		t.Fatalf("expected the Explorer button and one tooltip, found %d labels", matches)
	}
	img := hs.tt.Image()
	for y := 1; y < int(bar.Height)-1; y++ {
		for x := 1; x < int(bar.Left)-1; x++ {
			if img.RGBAAt(x, y) != before {
				t.Fatalf("the activity tooltip covers the window controls at (%d, %d)", x, y)
			}
		}
	}
	hs.shotRect("sidebar-activity-tooltip", image.Rect(0, 0, 410, 235), 1)
	hs.a.agent.permissions = []*agentPermission{{title: "Approval"}}
	hs.settle()
	sessions := hs.detailBounds(locales.CmdViewSessions())
	hs.tt.Move(sessions.X+sessions.W/2, sessions.Y+sessions.H/2)
	hs.tt.Advance(time.Second)
	step(hs.tt)
	permission := hs.detailBounds(locales.CmdViewSessions() + " · " + locales.AgentPermission())
	if permission.Y < sessions.Y+sessions.H {
		t.Fatalf("the permission tooltip is not below Sessions: tip=%v button=%v", permission, sessions)
	}
}

func TestSidebarActivitiesAlignWithWorkspaceTabs(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	hs.a.openFile(hs.path("main.go"), true)
	for _, theme := range []string{"", themeQoder} {
		hs.a.settings.Theme = theme
		for _, right := range []bool{false, true} {
			hs.a.settings.SidebarRight = right
			hs.settle()
			activity, foundActivity := hs.tt.Find(hs.a.withKey(locales.SidebarExplorer(), "view.explorer"))
			tab, foundTab := hs.tt.Find(locales.CloseName("main.go"))
			if !foundActivity || !foundTab || math.Abs(float64(activity.Y+activity.H/2-tab.Y-tab.H/2)) > 1 {
				t.Fatalf("theme=%q right=%v: sidebar activity %+v and workspace tab %+v are not aligned", theme, right, activity, tab)
			}
		}
	}
}

// The icons identify the sidebar's views without a title, and keep their
// centered positions when changing views or revealing their hover actions.
func TestSidebarIconsStayCentered(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	// The sidebar at the left: the icons center in its own width,
	// whatever the side the setting puts it on.
	a.settings.SidebarRight = false
	hs.settle()
	labels := []string{
		a.withKey(locales.SidebarExplorer(), "view.explorer"),
		a.withKey(locales.SidebarSearch(), "edit.findInFiles"),
		a.withKey(locales.SidebarScm(), "view.scm"),
		locales.CmdViewSessions(),
	}
	noTitle := func() {
		t.Helper()
		for _, title := range []string{"EXPLORER", "SOURCE CONTROL", "GIT", "SEARCH", "SESSIONS"} {
			if _, shown := tt.Find(title); shown {
				t.Errorf("the sidebar still shows title %q", title)
			}
		}
	}
	hs.gitRepo()
	hs.settleGit()
	for _, width := range []float32{a.window.SidebarWidth, 200, 160} {
		a.window.SidebarWidth = width
		a.showExplorer()
		tt.Move(1100, 400)
		hs.settle()
		rest := hs.layoutOf(labels...)
		first, last := rest[labels[0]], rest[labels[len(labels)-1]]
		m := a.pal.metricsOr()
		if center := (first.X + last.X + last.W) / 2; math.Abs(float64(center-width/2)) > 1 {
			t.Errorf("the icons center at %v in a sidebar %v wide", center, width)
		}
		if first.X < m.pad || last.X+last.W > width-m.pad {
			t.Errorf("the icons extend from %v to %v outside the inset sidebar %v wide", first.X, last.X+last.W, width)
		}
		for _, view := range []struct{ id, command string }{
			{"", "view.explorer"},
			{"search", "edit.findInFiles"},
			{"git", "view.scm"},
			{"sessions", "view.sessions"},
		} {
			a.run(view.command)
			tt.Move(1100, 400)
			hs.settleGit()
			if a.window.SidebarView != view.id {
				t.Fatalf("%s shows sidebar %q", view.command, a.window.SidebarView)
			}
			noTitle()
			sameLayout(t, view.command, rest, hs.layoutOf(labels...))
			tt.Move(100, 150)
			hs.settle()
			sameLayout(t, view.command+" hovered", rest, hs.layoutOf(labels...))
			if view.id == "" {
				if _, ok := tt.Find("New File…"); !ok {
					t.Error("the Explorer's buttons do not show under the pointer")
				}
			}
		}
	}
}

func TestSidebarActivitiesScrollWithinNarrowSurface(t *testing.T) {
	hs := newHarness(t, true)
	withPlugins(hs, map[string]map[string]string{
		"lint": {"plugin.json": lintPlugin, "main.ts": lintMain},
		"notes": {
			"plugin.json": `{"name":"notes","main":"main.ts","contributes":{"views":[{"id":"notes.view","title":"Notes","icon":"check"}]}}`,
			"main.ts":     "",
		},
	})
	a, tt := hs.a, hs.tt
	// At the left: the activities scroll from its own left inset.
	a.settings.SidebarRight = false
	a.window.SidebarWidth = 160
	hs.settle()
	a.showExplorer()
	hs.settle()
	labels := []string{a.withKey(locales.SidebarExplorer(), "view.explorer"), "Lint", locales.CmdViewSessions()}
	rest := hs.layoutOf(labels...)
	first, last := rest[labels[0]], rest[labels[2]]
	m := a.pal.metricsOr()
	if first.W != m.control || first.X != m.pad+2*m.gap {
		t.Fatalf("the overflowing activities do not start at the left inset: %v", first)
	}
	if last.W >= m.control {
		t.Fatalf("the plugin fixture does not clip the last activity: %v", rest)
	}

	// A button hidden past the surface cannot take a click in the editor.
	x, y := last.X+m.control/2, last.Y+last.H/2
	tt.Press(x, y)
	tt.Release(x, y)
	hs.settle()
	if a.window.SidebarView != "" {
		t.Fatalf("the clipped Sessions button took a click outside the sidebar: %q", a.window.SidebarView)
	}

	tt.Wheel(first.X+first.W/2, y, 1000, 0)
	hs.settle()
	scrolled := hs.layoutOf(labels...)
	last = scrolled[labels[2]]
	if scrolled[labels[0]].X >= first.X || last.W != m.control || last.X < m.pad || last.X+last.W > a.window.SidebarWidth-m.pad {
		t.Fatalf("scrolling did not bring the last activity into the sidebar: first %v, last %v", scrolled[labels[0]], last)
	}
	if err := tt.Click(labels[2]); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.window.SidebarView != "sessions" {
		t.Errorf("the scrolled Sessions button shows %q", a.window.SidebarView)
	}

	a.run("view.explorer")
	hs.settle()
	if got, ok := tt.Find(labels[0]); !ok || got.W != m.control || got.X < m.pad || got.X+got.W > a.window.SidebarWidth-m.pad {
		t.Errorf("choosing Explorer by command left its button clipped: %v", got)
	}
	a.run("view.sessions")
	hs.settle()
	if got, ok := tt.Find(labels[2]); !ok || got.W != m.control || got.X < m.pad || got.X+got.W > a.window.SidebarWidth-m.pad {
		t.Errorf("choosing Sessions by command left its button clipped: %v", got)
	}

	// Revealing a selection once still lets the user scroll away from it.
	tt.Wheel(last.X+last.W/2, y, -1000, 0)
	hs.settle()
	if got, ok := tt.Find(labels[0]); !ok || got != first {
		t.Errorf("scrolling back left the Explorer at %v, want %v", got, first)
	}
	a.toggleSidebar()
	hs.settle()
	a.toggleSidebar()
	hs.settle()
	if got, ok := tt.Find(labels[2]); !ok || got.W != m.control || got.X < m.pad || got.X+got.W > a.window.SidebarWidth-m.pad {
		t.Errorf("reopening the sidebar left the selected Sessions button clipped: %v", got)
	}
	a.window.SidebarWidth = 240
	hs.settle()
	expanded := hs.layoutOf(labels...)
	first, last = expanded[labels[0]], expanded[labels[2]]
	if center := (first.X + last.X + last.W) / 2; math.Abs(float64(center-a.window.SidebarWidth/2)) > 1 {
		t.Errorf("expanding the sidebar left its activity group centered at %v", center)
	}
}

// The guides of a row go down the middle of the arrows of the folders
// above it, as the outline places them treeStep apart.
func TestTreeGuidesFollowTheArrows(t *testing.T) {
	const arrow, gap, left = 16, 4, 10
	// arrowAt is where the arrow of a row depth deep starts: its
	// indentation and a gap after the row's start, on deeper rows.
	arrowAt := func(depth int) float32 {
		if depth == 0 {
			return left
		}
		return left + float32(depth)*treeStep + gap
	}
	for depth := 1; depth <= 5; depth++ {
		content := arrowAt(depth) + arrow + gap
		for k, x := range treeGuideX(content, depth, arrow, gap) {
			if want := arrowAt(k) + arrow/2; x != want {
				t.Errorf("depth %d, guide %d at %v, want %v", depth, k, x, want)
			}
		}
	}
}

func TestSidebarSwitchIconsRemainVisible(t *testing.T) {
	withAppearancePlugins(t)
	for _, theme := range []struct {
		name string
		pal  *palette
	}{
		{"onedarkpro", oneDarkProPal},
		{"dark", paletteFor("", true)},
		{"light", paletteFor("", false)},
	} {
		t.Run(theme.name, func(t *testing.T) {
			for _, icon := range []struct {
				name string
				svg  *ui.SVG
			}{
				{"explorer", iconFiles},
				{"search", iconSearch},
				{"scm", iconBranch},
				{"sessions", iconClock},
			} {
				t.Run(icon.name, func(t *testing.T) {
					a := &app{pal: theme.pal}
					on := false
					tt := ui.NewTester(func(c *ui.Context) {
						ui.Box(c).Fill().Background(a.pal.chrome).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
							a.sidebarSwitch(c, icon.svg, icon.name, "", on, 0, func() {})
						})
					}, 64, 48)
					tt.SetDark(a.pal.dark)
					for _, selected := range []bool{false, true, false} {
						on = selected
						tt.Frame()
						tt.Advance(motionHover)
						tt.Frame()
						bounds, ok := tt.Find(icon.name)
						if !ok {
							t.Fatal("the sidebar switch is missing")
						}
						img := tt.Image()
						background := img.RGBAAt(int(bounds.X+2), int(bounds.Y+bounds.H/2))
						foreground := a.pal.muted
						if selected {
							foreground = a.pal.text
						}
						// Count ink in the icon's interior, away from the focus ring
						// and corners, so a selection background cannot pass as an icon.
						centerX, centerY := int(bounds.X+bounds.W/2), int(bounds.Y+bounds.H/2)
						visible := 0
						for y := centerY - 7; y < centerY+7; y++ {
							for x := centerX - 7; x < centerX+7; x++ {
								pixel := img.RGBAAt(x, y)
								toForeground := abs(int(pixel.R)-int(foreground.R)) + abs(int(pixel.G)-int(foreground.G)) + abs(int(pixel.B)-int(foreground.B))
								toBackground := abs(int(pixel.R)-int(background.R)) + abs(int(pixel.G)-int(background.G)) + abs(int(pixel.B)-int(background.B))
								if toForeground < toBackground {
									visible++
								}
							}
						}
						if visible < 12 {
							(&harness{t: t, tt: tt}).shot("sidebar-switch-" + theme.name + "-" + icon.name + "-hidden")
							t.Errorf("selected=%v: only %d foreground pixels remain in the sidebar icon, want at least 12", selected, visible)
						}
					}
				})
			}
		})
	}
}

func TestSidebarSwitchSelectionRemainsVisible(t *testing.T) {
	withAppearancePlugins(t)
	compact := *oneDarkProPal
	compact.metrics = compact.metricsOr()
	compact.metrics.control = metricsBounds.control[0]
	for _, theme := range []struct {
		name string
		pal  *palette
	}{
		{"onedarkpro", oneDarkProPal},
		{"onedarkpro-compact", &compact},
		{"dark", paletteFor("", true)},
		{"light", paletteFor("", false)},
	} {
		t.Run(theme.name, func(t *testing.T) {
			a := &app{pal: theme.pal}
			selected := "search"
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).Fill().Background(a.pal.chrome).Justify(ui.Center).AlignItems(ui.Center).Gap(2).Children(func() {
					a.sidebarSwitch(c, iconFiles, "explorer", "", selected == "explorer", 0, func() { selected = "explorer" })
					a.sidebarSwitch(c, iconSearch, "search", "", selected == "search", 0, func() { selected = "search" })
				})
			}, 100, 48)
			tt.SetDark(a.pal.dark)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			settle := func() {
				tt.Frame()
				tt.Advance(motionHover)
				tt.Frame()
			}
			checkMarker := func(label, state string, want bool) {
				t.Helper()
				bounds, ok := tt.Find(label)
				if !ok {
					t.Fatalf("the %s switch is missing", label)
				}
				img := tt.Image()
				visible, strongest := 0, 0.0
				// The bottom of the button is outside the icon and focus ring.
				// Sample its whole width without requiring a particular marker shape.
				for y := int(math.Ceil(float64(bounds.Y + bounds.H*0.8))); y < int(bounds.Y+bounds.H-1); y++ {
					for x := int(bounds.X + 1); x < int(bounds.X+bounds.W-1); x++ {
						pixel := img.RGBAAt(x, y)
						ratio := contrast(ui.RGBA(pixel.R, pixel.G, pixel.B, 1), a.pal.chrome)
						strongest = max(strongest, ratio)
						if ratio >= 3 {
							visible++
						}
					}
				}
				if want && visible < 8 {
					t.Errorf("%s %s: only %d marker pixels reach 3:1 contrast, want at least 8 (strongest %.2f:1)", label, state, visible, strongest)
				}
				if !want && visible != 0 {
					t.Errorf("%s %s: an unselected switch still has %d marker pixels", label, state, visible)
				}
			}
			bounds, _ := tt.Find("explorer")
			tt.Move(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
			settle()
			checkMarker("explorer", "hovered", false)
			if err := tt.Click("explorer"); err != nil {
				t.Fatal(err)
			}
			settle()
			checkMarker("explorer", "selected", true)
			tt.Move(0, 0)
			settle()
			checkMarker("explorer", "pointer away", true)
			if err := tt.Click("search"); err != nil {
				t.Fatal(err)
			}
			tt.Move(0, 0)
			settle()
			checkMarker("explorer", "switched away", false)
			checkMarker("search", "selected", true)
		})
	}
}

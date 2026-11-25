package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func checkStatusControls(test *testing.T, fixture *harness, width, height float32) {
	test.Helper()
	find := func(label string) ui.Rect {
		test.Helper()
		bounds, found := fixture.tt.Find(label)
		if !found {
			test.Fatalf("status control %q is not shown", label)
		}
		if bounds.X < 0 || bounds.X+bounds.W > width || bounds.Y < height-statusBarH || bounds.Y+bounds.H > height {
			test.Errorf("status control %q is outside the bottom of %v × %v: %v", label, width, height, bounds)
		}
		return bounds
	}
	sidebar := find(fixture.a.withKey(locales.CmdViewSidebar(), "view.sidebar"))
	terminal := find(fixture.a.withKey(locales.CmdViewTerminal(), "view.terminal"))
	controls := []struct {
		name   string
		bounds ui.Rect
		right  bool
	}{
		{"sidebar", sidebar, fixture.a.settings.SidebarRight},
		{"terminal", terminal, fixture.a.settings.TerminalButtonRight},
	}
	for _, control := range controls {
		if control.right && control.bounds.X < width/2 || !control.right && control.bounds.X+control.bounds.W > width/2 {
			test.Errorf("%s status control is not on right=%v: %v", control.name, control.right, control.bounds)
		}
	}
	if fixture.a.settings.SidebarRight == fixture.a.settings.TerminalButtonRight {
		if fixture.a.settings.SidebarRight && terminal.X+terminal.W > sidebar.X {
			test.Errorf("right status controls are not terminal then sidebar: %v, %v", terminal, sidebar)
		} else if !fixture.a.settings.SidebarRight && sidebar.X+sidebar.W > terminal.X {
			test.Errorf("left status controls are not sidebar then terminal: %v, %v", sidebar, terminal)
		}
	}
	if fixture.a.activeEditor() != nil {
		position, found := fixture.tt.Find(locales.StatusPos(1, 1))
		if !found || position.X < 0 || position.X+position.W > width {
			test.Fatalf("the editor position does not fit beside the status controls: %v (shown %v)", position, found)
		}
		for _, control := range controls {
			if control.right && position.X+position.W > control.bounds.X || !control.right && position.X < control.bounds.X+control.bounds.W {
				test.Errorf("the editor position overlaps the %s status control: %v, %v", control.name, position, control.bounds)
			}
		}
	}
}

func TestStatusControlsIndependentSides(test *testing.T) {
	for _, content := range []string{"welcome", "editor", "viewer", "diff", "terminal"} {
		test.Run(content, func(test *testing.T) {
			var fixture *harness
			if content == "welcome" {
				preferences := settings.Default()
				host := &fakeHost{}
				application := newApp(host, preferences, preferences.NewWindow())
				test.Cleanup(application.shutdown)
				fixture = &harness{t: test, a: application, h: host, tt: ui.NewTester(application.view, 1200, 760)}
				fixture.tt.SetDark(true)
			} else {
				fixture = newHarness(test, true)
				switch content {
				case "editor":
					fixture.a.openFile(fixture.path("main.go"), true)
				case "viewer":
					path := fixture.path("art.png")
					writePNG(test, path, 40, 30)
					fixture.a.openFile(path, true)
					fixture.until("the picture", func() bool {
						viewer := fixture.a.activeViewer()
						return viewer != nil && viewer.src != nil
					})
				case "diff":
					fixture.a.openChangesTab(true)
				case "terminal":
					terminal := testPanelSession(test, fixture.a)
					fixture.a.terminalToColumn(terminal, fixture.a.column(), 0)
				}
			}
			for _, sidebarRight := range []bool{false, true} {
				for _, terminalRight := range []bool{false, true} {
					fixture.a.settings.SidebarRight = sidebarRight
					fixture.a.settings.TerminalButtonRight = terminalRight
					fixture.tt.SetSize(1200, 760)
					for _, position := range []string{"", "left", "right"} {
						fixture.a.settings.PanelPosition = position
						fixture.settle()
						checkStatusControls(test, fixture, 1200, 760)
					}
					fixture.tt.SetSize(300, 400)
					fixture.settle()
					checkStatusControls(test, fixture, 300, 400)
				}
			}
		})
	}
}

func TestStatusControlsMenusMoveIndependently(test *testing.T) {
	fixture := newHarness(test, true)
	application, tester := fixture.a, fixture.tt
	application.settings.PanelPosition = "left"
	checkPosition := func(sidebarRight, terminalRight bool) {
		test.Helper()
		if application.settings.SidebarRight != sidebarRight || application.settings.TerminalButtonRight != terminalRight {
			test.Fatalf("sidebarRight = %v, terminalButtonRight = %v, want %v, %v", application.settings.SidebarRight, application.settings.TerminalButtonRight, sidebarRight, terminalRight)
		}
		checkStatusControls(test, fixture, 1200, 760)
		explorer, found := tester.Find(application.withKey(locales.SidebarExplorer(), "view.explorer"))
		if !found || (explorer.X >= 600) != sidebarRight {
			test.Errorf("the sidebar did not follow its control to the right=%v: %v (shown %v)", sidebarRight, explorer, found)
		}
		if application.window.SidebarClosed || application.window.PanelOpen || application.settings.PanelPosition != "left" {
			test.Error("moving a status control toggled or moved the terminal panel")
		}
	}
	for _, control := range []struct {
		label    string
		terminal bool
	}{
		{application.withKey(locales.CmdViewSidebar(), "view.sidebar"), false},
		{application.withKey(locales.CmdViewTerminal(), "view.terminal"), true},
	} {
		for _, otherRight := range []bool{false, true} {
			application.settings.SidebarRight = otherRight
			application.settings.TerminalButtonRight = otherRight
			fixture.settle()
			for _, choice := range []struct {
				label string
				right bool
			}{
				{locales.StatusControlsRight(), true},
				{locales.StatusControlsRight(), true},
				{locales.StatusControlsLeft(), false},
				{locales.StatusControlsLeft(), false},
			} {
				if err := tester.RightClick(control.label); err != nil {
					test.Fatal(err)
				}
				if menu := tester.Menu(); !slices.Contains(menu, locales.StatusControlsLeft()) || !slices.Contains(menu, locales.StatusControlsRight()) {
					test.Fatalf("status control %q has no position choices: %q", control.label, menu)
				}
				if err := tester.ChooseMenuItem(choice.label); err != nil {
					test.Fatal(err)
				}
				fixture.settle()
				if control.terminal {
					checkPosition(otherRight, choice.right)
				} else {
					checkPosition(choice.right, otherRight)
				}
			}
		}
	}
	application.run("view.sidebarRight")
	fixture.settle()
	checkPosition(false, false)
}

type statusControlsHost struct {
	*fakeHost
	redraws     int
	menuChanges int
}

func (host *statusControlsHost) redrawWindows() { host.redraws++ }
func (host *statusControlsHost) menuChanged()   { host.menuChanges++ }

func TestStatusControlsUpdateAllWindows(test *testing.T) {
	fixture := newHarness(test, true)
	application := fixture.a
	host := &statusControlsHost{fakeHost: fixture.h}
	application.host = host
	other := newApp(host, application.settings, application.settings.NewWindow())
	test.Cleanup(other.shutdown)
	otherFixture := &harness{t: test, a: other, h: fixture.h, tt: ui.NewTester(other.view, 1200, 760)}
	otherFixture.tt.SetDark(true)
	for _, change := range []struct {
		set   func(bool)
		get   func() bool
		right bool
	}{
		{application.setTerminalButtonRight, func() bool { return application.settings.TerminalButtonRight }, false},
		{application.setSidebarRight, func() bool { return application.settings.SidebarRight }, false},
		{application.setTerminalButtonRight, func() bool { return application.settings.TerminalButtonRight }, true},
		{application.setSidebarRight, func() bool { return application.settings.SidebarRight }, true},
	} {
		redraws, menuChanges, generation := host.redraws, host.menuChanges, application.settings.Gen()
		same := change.get() == change.right
		change.set(change.right)
		fixture.settle()
		otherFixture.settle()
		checkStatusControls(test, fixture, 1200, 760)
		checkStatusControls(test, otherFixture, 1200, 760)
		if same && (host.redraws != redraws || host.menuChanges != menuChanges || application.settings.Gen() != generation) {
			test.Fatal("choosing the same status control side changed the settings again")
		}
		if !same && (host.redraws != redraws+1 || host.menuChanges != menuChanges+1 || application.settings.Gen() != generation+1) {
			test.Fatal("changing a status control side did not save and refresh all windows and menus")
		}
		redraws, menuChanges, generation = host.redraws, host.menuChanges, application.settings.Gen()
		change.set(change.right)
		if host.redraws != redraws || host.menuChanges != menuChanges || application.settings.Gen() != generation {
			test.Fatal("choosing the same status control side changed the settings again")
		}
	}
}

func TestStatusControlsKeepToggleBehavior(test *testing.T) {
	fixture := newHarness(test, true)
	application, tester := fixture.a, fixture.tt
	sidebarLabel := application.withKey(locales.CmdViewSidebar(), "view.sidebar")
	for _, closed := range []bool{true, false} {
		if err := tester.Click(sidebarLabel); err != nil {
			test.Fatal(err)
		}
		fixture.settle()
		if application.window.SidebarClosed != closed {
			test.Errorf("sidebarClosed = %v after clicking its status control, want %v", application.window.SidebarClosed, closed)
		}
	}
	application.openFile(fixture.path("main.go"), true)
	fixture.settle()
	terminal := testPanelSession(test, application)
	terminalLabel := application.withKey(locales.CmdViewTerminal(), "view.terminal")
	if err := tester.Click(terminalLabel); err != nil {
		test.Fatal(err)
	}
	fixture.settle()
	if !application.window.PanelOpen || application.focus != paneTerminal {
		test.Fatal("the terminal status control did not open and focus the panel")
	}
	application.focusColumn()
	fixture.settle()
	if err := tester.Click(terminalLabel); err != nil {
		test.Fatal(err)
	}
	fixture.settle()
	if !application.window.PanelOpen || application.focus != paneTerminal {
		test.Fatal("the terminal status control did not refocus the open panel")
	}
	if err := tester.Click(terminalLabel); err != nil {
		test.Fatal(err)
	}
	fixture.settle()
	if application.window.PanelOpen {
		test.Error("the terminal status control did not hide the focused panel")
	}
	if len(application.terms) != 1 || application.panelTerm != terminal {
		test.Error("toggling the terminal status control replaced its session")
	}
}

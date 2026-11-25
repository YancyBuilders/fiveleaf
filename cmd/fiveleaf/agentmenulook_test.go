package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestAgentMenuStatusColumn(t *testing.T) {
	hs := agentRosterHarness(t)
	for _, trigger := range []string{locales.AgentChoose(), locales.AgentRoster()} {
		hs.openAgentMenu(trigger)
		hs.settle()
		first, online := hs.tt.Find("Alpha · " + locales.AgentOnline())
		second, offline := hs.tt.Find("Beta · " + locales.AgentOffline())
		if !online || !offline || first.X != second.X || first.W != second.W || second.Y <= first.Y {
			t.Fatalf("%s: missing aligned target rows: %+v, %+v", trigger, first, second)
		}
		hs.tt.ClickAt(2, 100)
		hs.settle()
	}
}

func TestAgentMenuKeyboardAndNarrowLayout(t *testing.T) {
	hs := agentRosterHarness(t)
	longName := strings.Repeat("My custom Agent ", 6)
	hs.a.settings.AgentServers["long"] = settings.AgentServer{Name: longName, Command: "unused", Protocol: "cli"}
	hs.a.window.SidebarClosed = true
	hs.tt.SetSize(310, 420)
	hs.settle()
	if err := hs.tt.Click(locales.AgentChoose()); err != nil {
		t.Fatal(err)
	}
	label := longName + " · " + locales.AgentOffline()
	if row, ok := hs.tt.Find(label); !ok || row.X < 0 || row.X+row.W > 310 || row.H != 28 {
		t.Fatalf("long target row falls outside the narrow menu: %+v", row)
	}
	hs.key(0, ui.KeyEnd)
	if !hs.tt.Focused(label) {
		t.Fatal("End did not focus the last agent")
	}
	hs.key(0, ui.KeyEnter)
	if hs.a.agentMenu.open || hs.a.agent.provider != "long" {
		t.Fatal("Enter did not select the focused agent")
	}
	hs.openAgentMenu(locales.AgentChoose())
	hs.key(0, ui.KeyEscape)
	if hs.a.agentMenu.open || !hs.tt.Focused(locales.AgentChoose()) {
		t.Fatal("Escape from the main menu did not return to the composer button")
	}
}

func TestAgentMenuStatusAppearance(t *testing.T) {
	before := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			hs := newHarness(t, theme == "dark")
			project := hs.a
			hs.tt.SetScale(2)
			project.settings.AgentServers = map[string]settings.AgentServer{}
			names := []string{"OpenCode", "Claude Code", "Codex", "Grok Build", "OMP", "Pi"}
			for _, name := range names {
				project.settings.AgentServers[name] = settings.AgentServer{Name: name, Command: "unused", Protocol: "cli"}
			}
			project.openAgentPage()
			project.selectAgent(names[0])
			project.agent.sessionID = "online"
			for _, name := range names[1:] {
				chat := &agentChat{agentTarget: agentTarget{provider: name}, group: project.agent.group}
				switch name {
				case "Claude Code":
					chat.busy = true
				case "Codex":
					chat.err = "connection failed"
				case "OMP":
					chat.connecting = true
				case "Pi":
					chat.permissions = []*agentPermission{{title: "Approval"}}
				}
				project.rememberChat(chat)
			}
			hs.settle()
			if err := hs.tt.Click(locales.AgentChoose()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			frame := hs.tt.Image()
			var region image.Rectangle
			for _, item := range hs.openAgentMenuItems() {
				if item == nil {
					continue
				}
				label := item.label
				if item.status.icon != nil {
					label += " · " + item.status.label
				}
				row, found := hs.tt.Find(label)
				if !found || row.H == 0 || row.W == 0 {
					t.Fatalf("missing visible menu row %q: %+v", label, row)
				}
				bounds := image.Rect(int(row.X*2), int(row.Y*2), int((row.X+row.W)*2), int((row.Y+row.H)*2))
				region = region.Union(bounds)
				if item.status.icon == nil {
					continue
				}
				bg := frame.RGBAAt(bounds.Max.X-66, bounds.Min.Y+bounds.Dy()/2)
				background := ui.RGB(bg.R, bg.G, bg.B)
				ink := 0
				for col := bounds.Max.X - 54; col < bounds.Max.X-10; col++ {
					for scan := bounds.Min.Y + 6; scan < bounds.Max.Y-6; scan++ {
						pixel := frame.RGBAAt(col, scan)
						tone := ui.RGB(pixel.R, pixel.G, pixel.B)
						if contrast(tone, background) >= 3 && contrast(tone, item.status.color) < 1.3 {
							ink++
						}
					}
				}
				if ink < 16 {
					t.Errorf("%s: only %d visible pixels of the status color in the trailing column", label, ink)
				}
			}
			if region.Empty() {
				t.Fatal("the menu did not open")
			}
			if dir := os.Getenv("SHOTS"); dir != "" {
				file, err := os.Create(filepath.Join(dir, "agent-status-"+theme+".png"))
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if err := png.Encode(file, frame.SubImage(region.Inset(-20))); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

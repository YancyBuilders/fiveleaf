package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/agents"
	"fiveleaf/locales"
)

func TestSessionsSelectionKeepsTextVisible(t *testing.T) {
	withAppearancePlugins(t)
	for _, theme := range []struct {
		name string
		pal  *palette
	}{{"onedarkpro", oneDarkProPal}, {"dark", paletteFor("", true)}, {"light", paletteFor("", false)}} {
		for _, state := range []string{"chosen", "picked", "hovered", "picked-hovered"} {
			t.Run(theme.name+"/"+state, func(t *testing.T) {
				a := newHarness(t, theme.pal.dark).a
				a.pal = theme.pal
				line := sessionLine{s: agents.Session{
					Harness: agents.Codex, ID: "visible", Title: "Readable session", Dir: a.root, Updated: time.Now(),
				}}
				var choice *sessionChoice
				if state == "chosen" {
					choice = &sessionChoice{held: true, there: 1}
				}
				if state == "picked" || state == "picked-hovered" {
					a.sessions.selection.Add(line.key())
				}
				var action func()
				var rowBounds ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Column(c).Fill().Padding(8).Background(a.pal.chrome).Children(func() {
						rowBounds = a.sessionRow(c, line, c.Now(), choice, &action).Bounds()
					})
				}, 400, 64)
				tt.SetDark(a.pal.dark)
				if state == "hovered" || state == "picked-hovered" {
					tt.Move(20, 20)
				}
				tt.Advance(motionHover + motionShow)
				tt.Frame()
				if state == "picked-hovered" {
					resume, ok := tt.Find(locales.SessionsResume())
					if !ok {
						t.Fatal("the hovered row has no resume action")
					}
					// Sample beside the row and button ink, away from rounded corners.
					img := tt.Image()
					y := int(rowBounds.Y + rowBounds.H/2)
					body := img.RGBAAt(int(rowBounds.X+3), y)
					actions := img.RGBAAt(int(resume.X-1), y)
					if abs(int(body.R)-int(actions.R)) > 2 || abs(int(body.G)-int(actions.G)) > 2 || abs(int(body.B)-int(actions.B)) > 2 || body.A != actions.A {
						t.Errorf("the picked-hover background %v differs from the actions background %v", body, actions)
					}
					want := a.pal.hover.Over(a.pal.active.Over(a.pal.chrome))
					if abs(int(body.R)-int(want.R)) > 2 || abs(int(body.G)-int(want.G)) > 2 || abs(int(body.B)-int(want.B)) > 2 {
						t.Errorf("the picked-hover background %v does not preserve both alpha layers, want %v", body, want)
					}
				}
				for _, text := range []struct {
					label string
					color ui.Color
				}{{line.s.Title, a.pal.text}, {a.sessionMeta(line.s), a.pal.faint}} {
					bounds, ok := tt.Find(text.label)
					if !ok {
						t.Fatalf("the row is missing %q", text.label)
					}
					img := tt.Image()
					back := img.RGBAAt(int(bounds.X), int(bounds.Y))
					visible := 0
					for y := int(bounds.Y); y < int(bounds.Y+bounds.H); y++ {
						for x := int(bounds.X); x < int(bounds.X+bounds.W); x++ {
							pixel := img.RGBAAt(x, y)
							toText := abs(int(pixel.R)-int(text.color.R)) + abs(int(pixel.G)-int(text.color.G)) + abs(int(pixel.B)-int(text.color.B))
							toBack := abs(int(pixel.R)-int(back.R)) + abs(int(pixel.G)-int(back.G)) + abs(int(pixel.B)-int(back.B))
							if toText < toBack {
								visible++
							}
						}
					}
					if visible < 12 {
						t.Errorf("%q has only %d foreground pixels above the row background", text.label, visible)
					}
				}
			})
		}
	}
}

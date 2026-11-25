package main

import (
	"bytes"
	"strings"
	"testing"

	"fiveleaf/internal/settings"
	"github.com/egoist/mygo/ui"
)

func TestMicaEmblemConversationLifecycle(t *testing.T) {
	prefs := settings.Default().Prefs
	prefs.MicaEnabled, prefs.MicaAnimation = false, false
	var reading ui.Handle
	pal := &palette{text: white, dark: true}
	tester := ui.NewTester(func(c *ui.Context) {
		pane := ui.Column(c).Fill().Background(black)
		attachMicaEmblem(c, pane, &reading, pal, &prefs)
	}, 520, 380)
	home := micaSnapshot(tester)
	prefs.MicaEnabled = true
	tester.Frame()
	asleep := micaSnapshot(tester)
	if bytes.Equal(asleep.Pix, home.Pix) || tester.FramePending() {
		t.Fatal("an empty conversation should show the settled emblem when animation is off")
	}
	prefs.MicaAnimation = true
	tester.Move(275, 190)
	tester.Advance(motionEmblemFollow * 3)
	if bytes.Equal(asleep.Pix, tester.Image().Pix) {
		t.Fatal("the particles stayed put when the pointer crossed the emblem")
	}
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	paused := micaSnapshot(tester)
	tester.Move(180, 130)
	tester.Advance(motionEmblemFollow * 3)
	if !bytes.Equal(paused.Pix, tester.Image().Pix) || tester.FramePending() {
		t.Fatal("the emblem kept reacting to the pointer with Reduce Motion on")
	}
	prefs.MicaEnabled = false
	tester.Frame()
	if !bytes.Equal(home.Pix, tester.Image().Pix) {
		t.Fatal("turning Mica off left the emblem visible")
	}
	prefs.MicaEnabled, prefs.MicaIntensity = true, 0
	tester.Frame()
	if !bytes.Equal(home.Pix, tester.Image().Pix) {
		t.Fatal("zero Mica intensity left the emblem visible")
	}
}

func TestMicaEmblemOnOpen(t *testing.T) {
	h := newRoundtableHarness(t, nil)
	a := h.a
	empty, greeting := a.blankChat(), a.blankChat()
	a.initChatTarget(empty)
	a.initChatTarget(greeting)
	greeting.messages = []*agentMessage{{role: "assistant", text: strings.Repeat("- An installed skill\n", 40)}}
	setup := newRoundtableWindow(a, nil)
	graph := newRoundtableWindow(a, h.w.meeting)
	graph.view = roundtableViewGraph
	prefs := &a.settings.Prefs
	prefs.MicaEnabled, prefs.MicaAnimation, prefs.Appearance = true, false, "dark"
	for _, view := range []struct {
		name    string
		draw    func(*ui.Context)
		visible bool
	}{
		{"empty chat", func(c *ui.Context) { a.agentConversation(c, empty) }, true},
		{"agent greeting", func(c *ui.Context) { a.agentConversation(c, greeting) }, true},
		{"roundtable setup", setup.render, false},
		{"empty roundtable", h.w.render, true},
		{"execution graph", graph.render, false},
	} {
		t.Run(view.name, func(t *testing.T) {
			var fade paletteFade
			prefs.MicaIntensity = 0
			tester := ui.NewTester(func(c *ui.Context) {
				a.pal = applyTheme(c, prefs, &fade)
				ui.Column(c).Fill().Background(a.pal.editor).Children(func() { view.draw(c) })
			}, 900, 680)
			tester.Frame()
			before := micaSnapshot(tester)
			prefs.MicaIntensity = 0.8
			tester.Frame()
			after := tester.Image()
			visible, changed := 0, 0
			for row := 150; row < 510; row++ {
				for column := 230; column < 670; column++ {
					old, now := before.RGBAAt(column, row), after.RGBAAt(column, row)
					if old != now {
						changed++
					}
					lift := min(int(now.R)-int(old.R), int(now.G)-int(old.G), int(now.B)-int(old.B))
					if lift >= 32 && abs(int(now.B)-int(now.R)) < 22 {
						visible++
					}
				}
			}
			if view.visible && visible < 400 {
				t.Fatalf("only %d visibly silver particle pixels before the first message", visible)
			}
			if !view.visible && changed != 0 {
				t.Fatalf("%d pixels changed outside a conversation", changed)
			}
		})
	}
}

func TestMicaEmblemPointerRipplesLocally(t *testing.T) {
	var reading ui.Handle
	prefs := settings.Default().Prefs
	prefs.MicaEnabled = true
	pal := &palette{text: white, dark: true}
	var scene *micaEmblem
	tester := ui.NewTester(func(c *ui.Context) {
		pane := ui.Column(c).Fill().Background(black)
		attachMicaEmblem(c, pane, &reading, pal, &prefs)
		scene = ui.Local(pane, "mica-emblem", func() micaEmblem { return micaEmblem{} })
	}, 480, 340)
	const mouseX, mouseY = 318, 146
	tester.Move(mouseX, mouseY)
	tester.Advance(motionEmblemGather + motionEmblemFollow)
	tester.Frame()
	if scene.pose.presence < 0.95 {
		t.Fatal("the transcript did not pick up the pointer")
	}
	near := micaSnapshot(tester)
	scene.pose.presence = 0
	tester.Frame()
	faint := tester.Image()
	changed, scattered := 0, 0
	for row := range 340 {
		for column := range 480 {
			if near.RGBAAt(column, row) == faint.RGBAAt(column, row) {
				continue
			}
			changed++
			if abs(column-mouseX) > 90 || abs(row-mouseY) > 90 {
				t.Fatalf("the local pointer wake moved a distant particle at %d,%d", column, row)
			}
			if near.RGBAAt(column, row).R < 50 {
				continue
			}
			clear := true
			for sampleY := row - 6; clear && sampleY <= row+6; sampleY++ {
				for sampleX := column - 6; sampleX <= column+6; sampleX++ {
					if faint.RGBAAt(sampleX, sampleY).R > 12 {
						clear = false
						break
					}
				}
			}
			if clear {
				scattered++
			}
		}
	}
	if changed < 150 {
		t.Fatalf("the pointer changed only %d pixels at the same drift phase", changed)
	}
	if scattered < 10 {
		t.Fatalf("only %d bright pixels moved into the space around the leaf", scattered)
	}
}

func TestMicaEmblemTracksWindowPointer(t *testing.T) {
	prefs := settings.Default().Prefs
	prefs.MicaEnabled = true
	var reading ui.Handle
	var scene *micaEmblem
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() {
			ui.Box(c).Width(160).FillHeight()
			pane := ui.Column(c).Grow(1).FillHeight()
			attachMicaEmblem(c, pane, &reading, darkPal, &prefs)
			scene = ui.Local(pane, "mica-emblem", func() micaEmblem { return micaEmblem{} })
			pane.Children(func() { ui.Scroll(c).Fill().Children(func() { ui.Text(c, "Selectable transcript").Selectable() }) })
		})
	}, 640, 340)
	tester.Move(90, 130)
	tester.Advance(motionEmblemGather)
	if scene.pose.pointer.X > -0.9 || scene.pose.presence < 0.95 {
		t.Fatalf("moving over the sidebar did not move the light: %+v", scene.pose)
	}
	tester.Move(470, 130)
	tester.Advance(6 * motionEmblemFollow)
	if scene.pose.pointer.X < 0.22 || scene.pose.pointer.X > 0.27 || scene.pose.pointer.Y > -0.12 || scene.pose.pointer.Y < -0.16 {
		t.Fatalf("the pointer missed the leaf in an offset pane: %+v", scene.pose.pointer)
	}
}

func TestMicaEmblemBehindTranscriptStaysReadable(t *testing.T) {
	for _, dark := range []bool{false, true} {
		prefs := settings.Default().Prefs
		prefs.MicaEnabled, prefs.MicaAnimation = true, false
		prefs.MicaFrost, prefs.MicaIntensity = 0.15, 1
		pal := builtinPalette("", dark)
		var reading ui.Handle
		tester := ui.NewTester(func(c *ui.Context) {
			materialOf(c).configure(&prefs)
			materialBackdrop(c, ui.Column(c).Fill(), pal, 1, true).Children(func() {
				ui.Column(c).Fill().Background(windowBackground(c, 1, pal.editor)).Children(func() {
					micaChatViewport(c, &reading, pal, &prefs, func() {
						ui.Box(c).Fill().Bind(&reading).Draw(func(pt *ui.Painter, rect ui.Rect) {
							for row := 4; row < int(rect.H)-1; row += 8 {
								ink := pal.muted
								if row%16 == 4 {
									ink = pal.text
								}
								pt.Fill(ui.Rect{X: rect.X, Y: rect.Y + float32(row), W: rect.W, H: 1}, ink, 0)
							}
						})
					})
				})
			})
		}, 280, 220)
		tester.Frame()
		frame := tester.Image()
		for row := 4; row < frame.Rect.Dy()-1; row += 8 {
			for column := range frame.Rect.Dx() {
				stroke := frame.RGBAAt(column, row)
				for _, neighbor := range []int{row - 1, row + 1} {
					ground := frame.RGBAAt(column, neighbor)
					if ratio := contrast(ui.RGB(stroke.R, stroke.G, stroke.B), ui.RGB(ground.R, ground.G, ground.B)); ratio < 4.5 {
						t.Fatalf("dark=%v: stroke contrast %.2f at %d,%d", dark, ratio, column, row)
					}
				}
			}
		}
	}
}

package main

import (
	"crypto/sha256"
	"net"
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/settings"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func TestTerminalSymbolRendering(t *testing.T) {
	families := ui.FontFamilies()
	has := func(name string) bool {
		return slices.ContainsFunc(families, func(family ui.FontFamily) bool { return strings.EqualFold(family.Name, name) })
	}
	var symbols string
	for _, family := range []string{"Symbols Nerd Font Mono", "JetBrainsMono Nerd Font Mono", "FiraCode Nerd Font Mono"} {
		if has(family) {
			symbols = family
			break
		}
	}
	if symbols == "" {
		t.Skip("install Symbols Nerd Font Mono to compare the rendered icon cells")
	}
	conn, peer := net.Pipe()
	t.Cleanup(func() { peer.Close() })
	term, err := terminal.New(terminal.Options{Conn: conn})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	prefs := settings.Default()
	prefs.TerminalFontSize = 18
	workbench := &app{settings: prefs, terms: []*session{{term: term}}}
	view := ui.NewTester(func(c *ui.Context) { terminal.View(c, term).Fill() }, 480, 80)
	type frame struct {
		pixels     [sha256.Size]byte
		cols, rows int
	}
	render := func(row string) frame {
		term.Feed([]byte("\x1b[?25l\x1b[2J\x1b[H" + row))
		view.Frame()
		cols, rows := term.Size()
		return frame{sha256.Sum256(view.Image().Pix), cols, rows}
	}
	primaries := []string{"monospace"}
	if has("Fira Code") {
		primaries = append(primaries, "Fira Code")
	}
	for _, primary := range primaries {
		t.Run(primary, func(t *testing.T) {
			prefs.TerminalFontFamily = primary
			workbench.fontTerminals()
			const icons = "A\ue5ff|\ue627|\U000f00ba|中文|Z"
			const plain = "AM|中|文|=>|\x1b[1mBold\x1b[0m|Z"
			gotIcons, gotPlain := render(icons), render(plain)
			base := termFont(prefs)
			term.SetFont(base)
			if want := render(plain); gotPlain != want {
				t.Error("primary text, CJK fallback or grid dimensions changed")
			}
			base.Family += ", " + symbols
			term.SetFont(base)
			if render("\ue5ff") == render("\ue627") {
				t.Fatalf("%s renders identical pixels for the folder and Go file", symbols)
			}
			if want := render(icons); gotIcons != want {
				t.Errorf("terminal icons differ from the explicit %s fallback, grid %dx%d (want %dx%d)", symbols, gotIcons.cols, gotIcons.rows, want.cols, want.rows)
			}
		})
	}
}

func TestTerminalCursorChangesPixels(t *testing.T) {
	conn, peer := net.Pipe()
	t.Cleanup(func() { peer.Close() })
	term, err := terminal.New(terminal.Options{Conn: conn})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	prefs := settings.Default()
	prefs.TerminalFontSize = 24
	term.SetFont(termFont(prefs))
	term.SetTheme(termTheme(lightPal), termTheme(darkPal))
	view := ui.NewTester(func(c *ui.Context) { terminal.View(c, term).Fill() }, 320, 40)
	term.Feed([]byte("\x1b[2J\x1b[Habc"))
	view.Frame()
	before := sha256.Sum256(view.Image().Pix)
	term.Feed([]byte("\x1b[1;2H"))
	view.Frame()
	if after := sha256.Sum256(view.Image().Pix); after == before {
		t.Fatal("moving the cursor did not change the terminal image")
	}
}

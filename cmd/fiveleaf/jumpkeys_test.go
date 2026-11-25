package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestJumpKeysParseVim(t *testing.T) {
	command, status := parseVim([]rune("gd"), vimNormal)
	if status != vimDone || command.key != "gd" {
		t.Errorf("gd: command %+v, status %d", command, status)
	}
	for _, mode := range []vimMode{vimNormal, vimVisual, vimVisualLine} {
		for _, keys := range []string{"gs", "gS", "3gs", "3gS"} {
			command, status := parseVim([]rune(keys), mode)
			if status != vimDone || command.key != keys[len(keys)-2:] || command.op != "" {
				t.Errorf("%v %q: command %+v, status %d", mode, keys, command, status)
			}
		}
		if _, status := parseVim([]rune("g"), mode); status != vimMore {
			t.Errorf("%v g: status %d, want pending", mode, status)
		}
	}
	for _, keys := range []string{"dgs", "cgS", "ygs"} {
		if _, status := parseVim([]rune(keys), vimNormal); status != vimBad {
			t.Errorf("operator %q: status %d, want unsupported", keys, status)
		}
	}
}

func TestJumpKeysParseHelix(t *testing.T) {
	command, status := parseHelix([]rune("gd"))
	if status != vimDone || command.key != "gd" {
		t.Errorf("gd: command %+v, status %d", command, status)
	}
	for _, keys := range []string{"gw", "3gw"} {
		command, status := parseHelix([]rune(keys))
		if status != vimDone || command.key != "gw" || command.op != "" {
			t.Errorf("%q: command %+v, status %d", keys, command, status)
		}
	}
	if _, status := parseHelix([]rune("g")); status != vimMore {
		t.Errorf("g: status %d, want pending", status)
	}
}

func TestJumpHintCommandBindings(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			const text = "alpha beta gamma\nsecond line\n"
			var hs *harness
			var document *editor
			var command string
			if mode == "vim" {
				hs, document = vimHarness(t, "jump.txt", text)
				command = "go.hopWord"
			} else {
				hs, document = helixHarness(t, "jump.txt", text)
				command = "go.helixWord"
			}
			withKeys(t, hs.a, `[{"key":"ctrl+shift+j","command":"`+command+`","when":"editorTextFocus"}]`)
			hs.tt.Key(ui.Ctrl|ui.Shift, ui.KeyJ)
			hs.frames()
			if !document.jumpHints.active {
				t.Fatalf("%s binding did not open hints", command)
			}
			expectText(t, document, text)
		})
	}
}

func TestJumpHintExCommands(t *testing.T) {
	for _, command := range []string{"HopWord", "HopLine", "HopChar1", "HopChar2"} {
		t.Run(command, func(t *testing.T) {
			hs, document := vimHarness(t, "jump.txt", "alpha beta\nsecond line\n")
			hs.a.vimEx(document, command)
			hs.frames()
			if !document.jumpHints.active {
				t.Fatalf(":%s did not open hints", command)
			}
			expectText(t, document, "alpha beta\nsecond line\n")
		})
	}
	for _, command := range []string{"HopChar3", "HopChar12", "HopWord!", "HopLine extra"} {
		t.Run(command, func(t *testing.T) {
			hs, document := vimHarness(t, "jump.txt", "alpha beta\nsecond line\n")
			hs.a.vimEx(document, command)
			hs.frames()
			if document.jumpHints.active {
				t.Fatalf("unsupported :%s opened hints", command)
			}
			expectText(t, document, "alpha beta\nsecond line\n")
		})
	}
}

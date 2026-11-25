package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func typeJumpKeys(harness *harness, keys string) {
	harness.t.Helper()
	for _, character := range keys {
		modifiers, key := vimKeyOf(harness.t, character)
		harness.tt.TypeKey(modifiers, key, string(character))
		harness.frames()
	}
}

func jumpTargetAt(t *testing.T, editor *editor, at int) jumpHintTarget {
	t.Helper()
	if !editor.jumpHints.active {
		t.Fatal("jump labels are not active")
	}
	for _, target := range editor.jumpHints.targets {
		if target.start == at {
			return target
		}
	}
	t.Fatalf("no jump label at rune %d: %+v", at, editor.jumpHints.targets)
	return jumpHintTarget{}
}

func hasJumpTarget(editor *editor, at int) bool {
	for _, target := range editor.jumpHints.targets {
		if target.start == at {
			return true
		}
	}
	return false
}

func TestJumpHintsModalInput(t *testing.T) {
	var source strings.Builder
	for line := range 18 {
		fmt.Fprintf(&source, "first%02d second%02d third%02d\n", line, line, line)
	}
	text := source.String()
	for _, mode := range []string{"vim", "helix"} {
		for _, live := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/live=%v", mode, live), func(t *testing.T) {
				harness, editor := editHarness(t, "jumps.md", text)
				harness.a.settings.EditorMode = mode
				harness.a.settings.Vim = mode == "vim"
				harness.a.settings.SmoothCaret = true
				harness.a.setLive(editor, live)
				editor.vscroll.Focus()
				harness.settle()
				origin := editor.caretPaint.glide.position(harness.tt.Now())
				keys := "gs"
				if mode == "helix" {
					keys = "gw"
				}
				typeJumpKeys(harness, keys)
				start, _ := editor.buf.Snapshot().LineRange(9)
				target := jumpTargetAt(t, editor, start)
				if mode == "helix" {
					for _, candidate := range editor.jumpHints.targets {
						if len(candidate.label) != 2 {
							t.Fatalf("Helix label is not two letters: %q", candidate.label)
						}
					}
				}
				closed := harness.a.window.SidebarClosed
				withKeys(t, harness.a, fmt.Sprintf(`[{"key":%q,"command":"workbench.action.toggleSidebarVisibility"}]`, target.label[:1]))
				typeJumpKeys(harness, target.label)
				if editor.jumpHints.active {
					t.Fatal("accepting a label kept the overlay open")
				}
				if harness.a.window.SidebarClosed != closed {
					t.Fatal("a label key ran its user keybinding")
				}
				end := target.start
				if mode == "helix" {
					end = target.end
				}
				expectSel(t, editor, target.start, end)
				expectText(t, editor, text)
				harness.tt.Advance(motionCaretJump / 3)
				harness.tt.Frame()
				shown := editor.caretPaint.glide.position(harness.tt.Now())
				arrival := editor.caretPaint.glide.to
				if !editor.caretPaint.glide.moving(harness.tt.Now()) || shown.Y <= origin.Y || shown.Y >= arrival.Y {
					t.Fatalf("the jump lost its intermediate caret: origin=%v shown=%v target=%v", origin, shown, arrival)
				}
				harness.tt.Advance(motionCaretJump + motionCaretTrail)
				harness.tt.Frame()
				if editor.caretPaint.glide.moving(harness.tt.Now()) || editor.caretPaint.glide.position(harness.tt.Now()) != arrival {
					t.Fatal("the jump did not settle at its selected target")
				}
			})
		}
	}
}

func TestJumpHintsVimVisual(t *testing.T) {
	text := "one two three\nfour five\n"
	harness, editor := vimHarness(t, "visual.txt", text)
	harness.vim("wvgs")
	target := jumpTargetAt(t, editor, strings.Index(text, "five"))
	typeJumpKeys(harness, target.label)
	if editor.vim.mode != vimVisual {
		t.Fatalf("a visual jump changed the mode to %v", editor.vim.mode)
	}
	expectSel(t, editor, 4, target.start+1)
	expectText(t, editor, text)
	selection := editor.sel
	harness.vim("gs<Esc>")
	if editor.jumpHints.active || editor.vim.mode != vimVisual || editor.sel != selection {
		t.Fatalf("canceling a visual jump changed its selection: active=%v mode=%v selection=%v", editor.jumpHints.active, editor.vim.mode, editor.sel)
	}
}

func TestJumpHintsVisibleWords(t *testing.T) {
	text := "origin nearward " + strings.Repeat("middle ", 90) + "rightward\nlast\n"
	harness, editor := vimHarness(t, "visible.txt", text)
	typeJumpKeys(harness, "gs")
	near := strings.Index(text, "nearward")
	far := strings.Index(text, "rightward")
	jumpTargetAt(t, editor, near)
	if hasJumpTarget(editor, far) {
		t.Fatal("an offscreen word received a jump label")
	}
	viewport := editor.lastView
	harness.tt.Scroll(viewport.X+viewport.W/2, viewport.Y+viewport.H/2, 240, 0)
	harness.frames()
	if editor.jumpHints.active {
		t.Fatal("scrolling kept stale jump labels")
	}
	if editor.hscroll.X <= 0 {
		t.Fatal("the visibility test did not scroll horizontally")
	}
	typeJumpKeys(harness, "gs")
	if hasJumpTarget(editor, near) {
		t.Fatal("a word scrolled beneath the gutter received a jump label")
	}
	harness.vim("<Esc>")
	harness.a.settings.WordWrap = true
	harness.settle()
	typeJumpKeys(harness, "gs")
	jumpTargetAt(t, editor, far)
	if len(editor.lines.Rows(0)) < 2 {
		t.Fatal("the wrapping test did not create another visual row")
	}
}

func TestJumpHintsLiveHiddenText(t *testing.T) {
	harness := newHarness(t, true)
	text := "intro anchor\n\n[display](https://hidden.example/secretvalue)\n\n| HeaderAlpha | HeaderBeta |\n| --- | --- |\n| CellAlpha | CellBeta |\n\n> [!faq]- Question?\n> ConcealedAlpha\n> ConcealedBeta\n\nending visible\n"
	editor := openLive(t, harness, "hidden.md", text, 1)
	harness.a.settings.Vim = true
	editor.vscroll.Focus()
	harness.settle()
	typeJumpKeys(harness, "gs")
	for _, word := range []string{"display", "ending"} {
		jumpTargetAt(t, editor, strings.Index(text, word))
	}
	for _, word := range []string{"secretvalue", "HeaderAlpha", "CellAlpha", "ConcealedAlpha", "ConcealedBeta"} {
		if hasJumpTarget(editor, strings.Index(text, word)) {
			t.Errorf("hidden source %q received a jump label", word)
		}
	}
}

func TestJumpHintsCommandPalette(t *testing.T) {
	harness, editor := vimHarness(t, "palette.txt", "one two three\nfour five\n")
	title := ""
	for _, entry := range harness.a.paletteEntries() {
		if entry.id == "go.hopWord" {
			title = entry.title
			break
		}
	}
	if title == "" {
		t.Fatal("the Hop Word command is not in the palette")
	}
	harness.a.run("go.commands")
	harness.settle()
	harness.tt.Type(title)
	harness.settle()
	if len(harness.a.quick.results) == 0 || harness.a.paletteEntries()[harness.a.quick.results[0].Index].id != "go.hopWord" {
		t.Fatalf("the palette did not find %q", title)
	}
	harness.tt.Key(0, ui.KeyEnter)
	harness.frames()
	if harness.a.quick.open {
		t.Fatal("starting Hop Word left the palette open")
	}
	target := jumpTargetAt(t, editor, 8)
	typeJumpKeys(harness, target.label)
	expectCursor(t, editor, target.start)
	expectText(t, editor, "one two three\nfour five\n")
}

func TestJumpHintsCharacterQuery(t *testing.T) {
	text := "origin alpha alpha\nending alpha\n"
	harness, editor := vimHarness(t, "characters.txt", text)
	harness.vim(":")
	harness.tt.Type("HopChar2")
	harness.tt.Key(0, ui.KeyEnter)
	harness.frames()
	if !editor.jumpHints.active || editor.jumpHints.kind != jumpHintChar2 {
		t.Fatal("the command line did not start HopChar2")
	}
	typeJumpKeys(harness, "al")
	if len(editor.jumpHints.targets) != 3 {
		t.Fatalf("expected three character-pair matches, got %+v; active=%v query=%q text=%q focus=%v held=%v", editor.jumpHints.targets,
			editor.jumpHints.active, string(editor.jumpHints.query), editor.jumpHints.queryText, editor.jumpHints.queryFocused, harness.a.held)
	}
	harness.tt.Key(0, ui.KeyBackspace)
	harness.frames()
	if string(editor.jumpHints.query) != "a" || len(editor.jumpHints.targets) != 0 {
		t.Fatal("Backspace did not reopen the character query")
	}
	typeJumpKeys(harness, "l")
	target := jumpTargetAt(t, editor, strings.LastIndex(text, "alpha"))
	typeJumpKeys(harness, target.label)
	expectCursor(t, editor, target.start)
	expectText(t, editor, text)
	harness.vim("<C-o>")
	expectCursor(t, editor, 0)
}

func TestJumpHintsCancelStaleTargets(t *testing.T) {
	for _, change := range []string{"text", "focus", "mode", "hidden"} {
		t.Run(change, func(t *testing.T) {
			harness, editor := vimHarness(t, "cancel.txt", "alpha beta gamma\n")
			typeJumpKeys(harness, "gs")
			jumpTargetAt(t, editor, 6)
			switch change {
			case "text":
				editor.edit(editor.sel, textEdit{0, 0, "new "})
			case "focus":
				harness.a.run("go.commands")
			case "mode":
				harness.a.settings.Vim = false
			case "hidden":
				harness.a.hideEditor(editor)
			}
			harness.frames()
			if editor.jumpHints.active {
				t.Fatalf("%s change kept stale labels", change)
			}
		})
	}
}

func TestJumpHintsLiveLayoutChanges(t *testing.T) {
	harness := newHarness(t, true)
	if err := os.WriteFile(harness.path("detail.md"), []byte(strings.Repeat("Embedded paragraph.\n\n", 4)), 0o644); err != nil {
		t.Fatal(err)
	}
	text := "intro anchor\n\n![[detail]]\n\nending visible\n"
	editor := openLive(t, harness, "moving.md", text, 1)
	harness.a.settings.Vim = true
	inside := strings.Index(text, "detail")
	harness.place(editor, inside, inside)
	editor.vscroll.Focus()
	harness.settle()
	editor.selectRange(0, 0, false)
	harness.tt.Frame()
	harness.tt.TypeKey(0, ui.KeyG, "g")
	harness.tt.TypeKey(0, ui.KeyS, "s")
	ending := jumpTargetAt(t, editor, strings.Index(text, "ending"))
	harness.tt.Advance(motionMove / 2)
	harness.frames()
	_, currentY, _, ok := editor.lines.Caret(ending.start)
	if !ok || currentY+editor.jumpHints.view.origin.Y == ending.box.Y {
		t.Fatal("the rendered block did not move the target layout")
	}
	if editor.jumpHints.active {
		t.Fatal("a moving Live Preview layout left labels at stale positions")
	}
}

func TestJumpHintsQueryCancellationRemovesInput(t *testing.T) {
	for _, change := range []string{"escape", "focus", "text", "no-match"} {
		t.Run(change, func(t *testing.T) {
			harness, editor := vimHarness(t, "query-cancel.txt", "alpha beta gamma\n")
			harness.a.startJumpHints(editor, jumpHintChar2)
			harness.frames()
			if _, visible := harness.tt.Find("HopChar2"); !visible {
				t.Fatal("the query input was not shown")
			}
			switch change {
			case "escape":
				harness.tt.Key(0, ui.KeyEscape)
			case "focus":
				harness.a.run("go.commands")
			case "text":
				editor.edit(editor.sel, textEdit{0, 0, "new "})
			case "no-match":
				harness.tt.Type("zz")
			}
			harness.settle()
			if _, visible := harness.tt.Find("HopChar2"); editor.jumpHints.active || visible {
				t.Fatalf("%s cancellation left the query input visible", change)
			}
		})
	}
}

func TestJumpHintsQueryScrollRestoresEditor(t *testing.T) {
	text := strings.Repeat("alpha beta gamma\n", 70)
	harness, editor := vimHarness(t, "query-scroll.txt", text)
	harness.a.startJumpHints(editor, jumpHintChar2)
	harness.frames()
	harness.tt.Type("a")
	harness.frames()
	viewport := editor.lastView
	harness.tt.Scroll(viewport.X+viewport.W/2, viewport.Y+viewport.H/2, 0, 100)
	harness.frames()
	if _, visible := harness.tt.Find("HopChar2"); editor.jumpHints.active || visible {
		t.Fatal("scrolling did not dismiss the character query")
	}
	typeJumpKeys(harness, "j")
	line, _ := editor.buf.Snapshot().LineRange(1)
	expectCursor(t, editor, line)
	expectText(t, editor, text)
}

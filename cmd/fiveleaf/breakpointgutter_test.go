package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/keymap"
	"fiveleaf/locales"
)

func TestBreakpointGutterClickAndHover(t *testing.T) {
	harness, ed := editHarness(t, "points.go", "package main\nfirst()\nsecond()\nthird()")
	app, tester := harness.a, harness.tt
	app.settings.EditorAgentContext = true
	harness.place(ed, 0, 7)
	view, height := ed.lastView, app.font.lineH
	x, y := view.X+debugGutterW-1, view.Y+editorPadY+2.5*height
	dotX, dotY := int(view.X+debugGutterW/2), int(y)
	before := tester.Image().RGBAAt(dotX, dotY)
	tester.Move(x, y)
	harness.settle()
	tester.Advance(time.Second)
	tester.Frame()
	preview := tester.Image().RGBAAt(dotX, dotY)
	if preview == before || tester.Cursor() != ui.CursorPointer || !tester.HasText(locales.DebugAddBreakpointAt("3")) {
		t.Fatalf("missing breakpoint preview or hint on line 3: %v → %v, cursor %v", before, preview, tester.Cursor())
	}
	if button, ok := tester.Find(locales.AgentContextComment()); !ok || button.X < view.X+debugGutterW {
		t.Fatalf("comment button overlaps the breakpoint lane: %v", button)
	}
	tester.ClickAt(x, y)
	harness.settle()
	if !slices.Equal(app.debugBreakpoints(ed.path), []int{2}) || app.review.draft != nil || ed.sel != [2]int{0, 7} {
		t.Fatalf("click changed breakpoints to %v, selection to %v, draft %v", app.debugBreakpoints(ed.path), ed.sel, app.review.draft)
	}
	tester.Move(view.X+ed.textX+30, y)
	harness.settle()
	if set := tester.Image().RGBAAt(dotX, dotY); set == before || set == preview {
		t.Fatalf("saved breakpoint looks like the empty or hovered lane: %v", set)
	}
	tester.Move(x, y)
	harness.settle()
	tester.Advance(time.Second)
	tester.Frame()
	if !tester.HasText(locales.DebugRemoveBreakpointAt("3")) {
		t.Fatal("hovering a saved breakpoint does not offer to remove it")
	}
	tester.ClickAt(x, y)
	harness.settle()
	tester.ClickAt(x, view.Y+1)
	tester.ClickAt(x, view.Y+editorPadY+6*height)
	harness.settle()
	if got := app.debugBreakpoints(ed.path); len(got) != 0 {
		t.Fatalf("clicking outside the source lines set breakpoints at %v", got)
	}
}

func TestBreakpointGutterWrappedAndScrolled(t *testing.T) {
	source := "package main\nfunc main() {\n\t" + strings.Repeat("hello(1); ", 60) + "\n" + strings.Repeat("\tprint(1)\n", 80) + "}"
	harness, ed := editHarness(t, "wrapped.go", source)
	app, tester := harness.a, harness.tt
	app.run("view.wordWrap")
	harness.settle()
	if rows := ed.lines.Rows(2); len(rows) < 2 {
		t.Fatal("third source line did not wrap")
	}
	view, height := ed.lastView, app.font.lineH
	x := view.X + debugGutterW/2
	tester.ClickAt(x, view.Y+editorPadY+ed.lines.Top(2)+1.5*height)
	harness.settle()
	if !slices.Equal(app.debugBreakpoints(ed.path), []int{2}) {
		t.Fatalf("wrapped row selected %v", app.debugBreakpoints(ed.path))
	}
	app.run("view.wordWrap")
	harness.settle()
	tester.Scroll(view.X+ed.textX+24, view.Y+8*height, 120, 0)
	tester.Scroll(view.X+ed.textX+24, view.Y+8*height, 0, 16*height)
	harness.settle()
	if ed.hscroll.X <= 0 || ed.vscroll.Y <= 0 || !slices.Contains(ed.sticky, 1) {
		t.Fatalf("expected a horizontally and vertically scrolled function: offsets %v %v, sticky %v", ed.hscroll.X, ed.vscroll.Y, ed.sticky)
	}
	tester.ClickAt(x, view.Y+height/2)
	harness.settle()
	if !slices.Equal(app.debugBreakpoints(ed.path), []int{1, 2}) {
		t.Fatalf("sticky header click selected %v", app.debugBreakpoints(ed.path))
	}
	line := int((ed.vscroll.Y + 6*height - editorPadY) / height)
	tester.ClickAt(x, view.Y+6*height)
	harness.settle()
	if !app.debug.hasBreakpoint(ed.path, line) {
		t.Fatalf("scrolled gutter click missed line %d: %v", line, app.debugBreakpoints(ed.path))
	}
}

func TestBreakpointGutterVerification(t *testing.T) {
	gutter := breakpointGutter{points: []breakpoint{{line: 0}, {line: 1, verified: true}, {line: 2, verified: true}}, debugging: true, paused: 2, hovered: -1}
	const rowHeight = 20
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Background(lightPal.editor).Draw(func(pt *ui.Painter, rect ui.Rect) {
			for line := range 3 {
				gutter.paint(pt, lightPal, line, rect.X, rect.Y+float32(line*rowHeight)+rowHeight/2)
			}
		})
	}, debugGutterW, 3*rowHeight)
	tester.Frame()
	frame := tester.Image()
	center := debugGutterW / 2
	pending := frame.RGBAAt(center, rowHeight/2)
	solid := frame.RGBAAt(center, rowHeight+rowHeight/2)
	if pending != frame.RGBAAt(1, rowHeight/2) || pending == frame.RGBAAt(center-4, rowHeight/2) || pending == solid {
		t.Fatal("pending and verified breakpoints need a hollow and a filled circle")
	}
	if frame.RGBAAt(center, 2*rowHeight+rowHeight/2) != solid {
		t.Fatal("the paused instruction covers the breakpoint")
	}
	arrow := false
	for row := range rowHeight {
		for column := center + 5; column < debugGutterW; column++ {
			arrow = arrow || frame.RGBAAt(column, 2*rowHeight+row) != frame.RGBAAt(column, rowHeight+row)
		}
	}
	if !arrow {
		t.Fatal("the paused instruction has no marker beside its breakpoint")
	}
}

func TestBreakpointShortcutUsesEditorBinding(t *testing.T) {
	harness, ed := editHarness(t, "points.go", "package main\n")
	app := harness.a
	if hint := app.keyHint("debug.toggleBreakpoint"); hint != "F9" {
		t.Fatalf("shortcut hint = %q", hint)
	}
	bindings, err := keymap.Parse([]byte(`[{"key":"f9","command":"-debug.toggleBreakpoint","when":"editorTextFocus"},{"key":"f8","command":"debug.toggleBreakpoint","when":"editorTextFocus"}]`))
	if err != nil {
		t.Fatal(err)
	}
	app.applyKeymap(bindings)
	harness.key(0, ui.KeyF9)
	if app.debug.hasBreakpoint(ed.path, 0) {
		t.Fatal("F9 still toggled a breakpoint after removing its binding")
	}
	harness.key(0, ui.KeyF8)
	if !app.debug.hasBreakpoint(ed.path, 0) {
		t.Fatal("the replacement shortcut did not set a breakpoint at the caret")
	}
}

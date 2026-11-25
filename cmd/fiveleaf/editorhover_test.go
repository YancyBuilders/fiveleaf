package main

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/lsp"
	"fiveleaf/locales"
)

func pointerHoverRune(t testing.TB, d *editor, at int) (float32, float32) {
	t.Helper()
	x, y := semanticPointRune(t, d, at, .4)
	return d.lastView.X - d.hscroll.X + x, d.lastView.Y + y
}

// Keep the tester's clock still while the real LSP subprocess answers.
func pointerHoverUntil(t *testing.T, hs *harness, what string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s (dwell elapsed %v): pointer=%+v query=%+v texts=%q", what, hs.tt.Now().Sub(hs.a.pointerHover.since), hs.a.pointerHover, hs.a.hoverQuery, hs.tt.Texts())
		}
		hs.tt.Frame()
		time.Sleep(time.Millisecond)
	}
}

func pointerHoverReply(t *testing.T, hs *harness, client *lsp.Client, method string, params any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := client.Call(ctx, method, params)
	if err != nil || string(raw) != "true" {
		t.Fatalf("release held reply: result=%s err=%v", raw, err)
	}
	// A snapshot is a wire barrier, including when cancellation removed the
	// original request's waiter and no UI callback can follow its late reply.
	sourceNavigationRequests(t, client)
	hs.tt.Frame()
}

func pointerHoverStart(t *testing.T, hs *harness, d *editor, client *lsp.Client) (float32, float32) {
	t.Helper()
	x, y := pointerHoverRune(t, d, 9)
	hs.tt.Move(x, y)
	hs.tt.Frame()
	hs.tt.Advance(hoverDwell)
	hs.tt.Frame()
	pointerHoverUntil(t, hs, "hover request", func() bool {
		_, count := sourceNavigationRequests(t, client)
		return count == 1
	})
	return x, y
}

func TestPointerHoverDwellAndSameSymbol(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	x, y := pointerHoverRune(t, d, 9)
	hs.tt.Move(x, y)
	hs.tt.Frame()
	hs.tt.Advance(hoverDwell / 2)
	hs.tt.Frame()
	if _, count := sourceNavigationRequests(t, client); count != 0 {
		t.Fatalf("hover requested before dwell: %d", count)
	}
	// Moving within second must neither restart the dwell nor issue a second
	// request once its documentation is showing.
	x2, y2 := pointerHoverRune(t, d, 11)
	hs.tt.Move(x2, y2)
	hs.tt.Advance(hoverDwell/2 - time.Millisecond)
	hs.tt.Frame()
	if _, count := sourceNavigationRequests(t, client); count != 0 {
		t.Fatalf("hover requested before the whole dwell: %d", count)
	}
	hs.tt.Advance(time.Millisecond)
	hs.tt.Frame()
	pointerHoverUntil(t, hs, "one hover after dwell", func() bool {
		_, count := sourceNavigationRequests(t, client)
		return count == 1
	})
	pointerHoverReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": "Documentation for second"})
	pointerHoverUntil(t, hs, "hover documentation", func() bool { return hs.a.hover != nil })
	x3, y3 := pointerHoverRune(t, d, 12)
	hs.tt.Move(x3, y3)
	hs.tt.Advance(2 * hoverDwell)
	hs.tt.Frame()
	if _, count := sourceNavigationRequests(t, client); count != 1 {
		t.Fatalf("motion in the same symbol repeated hover: %d", count)
	}
	if hs.a.hover == nil || hs.a.hover.tree.Text != "Documentation for second" {
		t.Fatal("same-symbol motion dismissed documentation")
	}
}

func TestPointerHoverCmdFeedbackAndClick(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	hs.place(d, 0, 5)
	wantSelection := d.sel
	x, y := pointerHoverRune(t, d, 9)
	hs.tt.Move(x, y)
	hs.tt.Frame()
	if hs.tt.Cursor() == ui.CursorPointer {
		t.Fatal("unmodified code hover uses a hand")
	}
	hs.tt.HoldModifiers(ui.Cmd)
	hs.tt.Frame()
	if hs.tt.Cursor() != ui.CursorPointer {
		t.Fatalf("holding Cmd without moving did not show a hand: %v", hs.tt.Cursor())
	}
	hs.tt.HoldModifiers(0)
	hs.tt.Frame()
	if hs.tt.Cursor() == ui.CursorPointer {
		t.Fatal("releasing Cmd left the hand cursor")
	}
	hs.tt.ClickAtWith(ui.Cmd, x, y)
	pointerHoverUntil(t, hs, "definition click", func() bool {
		count, _ := sourceNavigationRequests(t, client)
		return count == 1
	})
	if d.sel != wantSelection || d.selecting {
		t.Fatalf("Cmd-click changed selection or started dragging: selection=%v want=%v selecting=%v", d.sel, wantSelection, d.selecting)
	}
	if got := sourceNavigationState(t, client).Definition.Position; got != (position{1, 1}) {
		t.Fatalf("Cmd-click requested %+v, want line 1 column 1", got)
	}
	if hs.a.definitionPending == nil {
		t.Fatal("definition click lost its pending request")
	}
}

func TestPointerHoverBlankSpaceDoesNotNavigate(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	points := []ui.Point{
		{X: d.lastView.X + d.textX + 20*hs.a.font.charW, Y: d.lastView.Y + editorPadY + hs.a.font.lineH/2},
		{X: d.lastView.X + d.textX + hs.a.font.charW, Y: d.lastView.Y + editorPadY + d.lines.Height() + 30},
	}
	for _, p := range points {
		hs.tt.Move(p.X, p.Y)
		hs.tt.HoldModifiers(ui.Cmd)
		hs.tt.Frame()
		if hs.tt.Cursor() == ui.CursorPointer {
			t.Fatal("blank space has a navigation hand")
		}
		hs.tt.ClickAtWith(ui.Cmd, p.X, p.Y)
		hs.tt.HoldModifiers(0)
		hs.tt.Advance(hoverDwell)
		hs.tt.Frame()
	}
	if definitions, hovers := sourceNavigationRequests(t, client); definitions != 0 || hovers != 0 {
		t.Fatalf("blank space issued semantic requests: definitions=%d hovers=%d", definitions, hovers)
	}
}

func TestPointerHoverCardKeepsEditorFocus(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	hs.place(d, 0, 5)
	wantSelection := d.sel
	pointerHoverStart(t, hs, d, client)
	pointerHoverReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": "Readable documentation"})
	pointerHoverUntil(t, hs, "hover card bounds", func() bool {
		return hs.a.hover != nil && hs.a.hoverQuery != nil && hs.a.hoverQuery.box.W > 0 && hs.a.hoverQuery.box.H > 0
	})
	if !hs.tt.Focused(locales.EditorLabel(d.name())) || d.sel != wantSelection {
		t.Fatalf("hover stole focus or selection: focused=%v selection=%v", hs.tt.Focused(locales.EditorLabel(d.name())), d.sel)
	}
	q := hs.a.hoverQuery
	bridgeX := q.anchor.X + q.anchor.W/2
	hs.tt.Move(bridgeX, q.anchor.Y+q.anchor.H+3)
	hs.tt.Advance(2 * hoverLeave)
	hs.tt.Frame()
	if hs.a.hoverQuery != q {
		t.Fatal("crossing the gap to the card dismissed it")
	}
	hs.tt.Move(q.box.X+q.box.W/2, q.box.Y+q.box.H/2)
	hs.tt.Advance(2 * hoverLeave)
	hs.tt.Frame()
	if hs.a.hoverQuery != q || hs.a.hover == nil {
		t.Fatal("entering the card dismissed it")
	}
	if d.sel != wantSelection || !hs.tt.Focused(locales.EditorLabel(d.name())) {
		t.Fatal("moving into the card changed editor selection or focus")
	}
	hs.tt.Type("replacement")
	hs.tt.Frame()
	if hs.a.hoverQuery != nil || d.buf.Slice(0, len("replacement")) != "replacement" {
		t.Fatal("typing with the card open did not edit and dismiss it")
	}
}

func TestPointerHoverDismissesPendingReplies(t *testing.T) {
	for _, action := range []string{"leave", "escape", "edit", "scroll", "blur", "tab"} {
		t.Run(action, func(t *testing.T) {
			hs, d, client := sourceNavigationHarness(t)
			x, y := pointerHoverStart(t, hs, d, client)
			switch action {
			case "leave":
				hs.tt.Move(d.lastView.X-20, d.lastView.Y+d.lastView.H-20)
				hs.tt.Advance(hoverLeave + time.Millisecond)
			case "escape":
				hs.tt.Key(0, ui.KeyEscape)
			case "edit":
				hs.tt.Type("x")
			case "scroll":
				hs.tt.Scroll(x, y, 0, 50)
			case "blur":
				hs.tt.SetFocused(false)
				// The desktop's blur callback updates application state in
				// addition to the native surface focus notification.
				hs.a.post(func() { hs.a.blurred = true })
			case "tab":
				hs.a.openFile(hs.path("README.md"), true)
			}
			hs.tt.Frame()
			if hs.a.hoverQuery != nil || hs.a.hover != nil {
				t.Fatal("dismissal left the pending hover alive")
			}
			pointerHoverReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": "Stale documentation"})
			hs.tt.Advance(2 * hoverDwell)
			hs.tt.Frame()
			if hs.a.hoverQuery != nil || hs.a.hover != nil || slices.Contains(hs.tt.Texts(), "Stale documentation") {
				t.Fatalf("late reply or stationary pointer reopened a dismissed hover: query=%+v hover=%+v", hs.a.hoverQuery, hs.a.hover)
			}
		})
	}
}

func TestPointerHoverEmptyResultIsSilent(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	pointerHoverStart(t, hs, d, client)
	pointerHoverReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": ""})
	pointerHoverUntil(t, hs, "empty hover result", func() bool { return hs.a.hoverQuery != nil && hs.a.hoverQuery.pending == 0 })
	hs.tt.Advance(3 * hoverDwell)
	hs.tt.Frame()
	if hs.a.hover != nil || slices.Contains(hs.tt.Texts(), locales.HoverNone()) || len(hs.a.notes) != 0 {
		t.Fatal("empty passive hover showed a card or notification")
	}
	if _, count := sourceNavigationRequests(t, client); count != 1 {
		t.Fatalf("empty hover retried without a new target: %d", count)
	}
}

func TestPointerHoverDoesNotSupersedeDefinition(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	pointerHoverStart(t, hs, d, client)
	x, y := pointerHoverRune(t, d, 1)
	hs.tt.ClickAtWith(ui.Cmd, x, y)
	pointerHoverUntil(t, hs, "definition after hover", func() bool {
		count, _ := sourceNavigationRequests(t, client)
		return count == 1
	})
	hs.tt.HoldModifiers(0)
	x, y = pointerHoverRune(t, d, 11)
	hs.tt.Move(x, y)
	hs.tt.Advance(2 * hoverDwell)
	hs.tt.Frame()
	pointerHoverReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": "Obsolete documentation"})
	if hs.a.hover != nil || hs.a.definitionPending == nil {
		t.Fatal("hover response interfered with the pending click")
	}
	pointerHoverReply(t, hs, client, "test/replyDefinition", map[string]any{"index": 0, "uri": serverURI(d.path), "line": 2, "character": 5})
	pointerHoverUntil(t, hs, "definition landing", func() bool { return caretPosition(d) == (position{2, 5}) })
}

func TestPointerHoverReenterSamePoint(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	x, y := pointerHoverStart(t, hs, d, client)
	hs.tt.Move(d.lastView.X-20, d.lastView.Y+d.lastView.H-20)
	hs.tt.Advance(hoverLeave + time.Millisecond)
	hs.tt.Frame()
	if hs.a.hoverQuery != nil {
		t.Fatal("leaving did not dismiss the first hover")
	}
	hs.tt.Move(x, y)
	hs.tt.Frame()
	hs.tt.Advance(hoverDwell - time.Millisecond)
	hs.tt.Frame()
	if _, count := sourceNavigationRequests(t, client); count != 1 {
		t.Fatalf("returning to a symbol skipped its dwell: %d requests", count)
	}
	hs.tt.Advance(time.Millisecond)
	hs.tt.Frame()
	pointerHoverUntil(t, hs, "hover after returning to the same point", func() bool {
		_, count := sourceNavigationRequests(t, client)
		return count == 2
	})
}

func TestPointerHoverEscapeSurvivesModifierChanges(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	pointerHoverStart(t, hs, d, client)
	hs.tt.Key(0, ui.KeyEscape)
	hs.tt.HoldModifiers(ui.Cmd)
	hs.tt.Frame()
	if hs.tt.Cursor() != ui.CursorPointer {
		t.Fatal("Cmd hand feedback stopped after dismissing hover")
	}
	hs.tt.HoldModifiers(0)
	hs.tt.Advance(2 * hoverDwell)
	hs.tt.Frame()
	if _, count := sourceNavigationRequests(t, client); count != 1 || hs.a.hoverQuery != nil {
		t.Fatalf("modifier changes reopened dismissed hover without pointer movement: count=%d query=%+v", count, hs.a.hoverQuery)
	}
}

func TestPointerHoverCardFitsLowerEdge(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	const width, height = 720, 240
	hs.tt.SetSize(width, height)
	hs.frames()
	at := posRune(d.buf.Snapshot(), position{3, 6})
	x, y := pointerHoverRune(t, d, at)
	hs.tt.Move(x, y)
	hs.tt.Advance(hoverDwell)
	hs.tt.Frame()
	pointerHoverUntil(t, hs, "hover near lower edge", func() bool {
		_, count := sourceNavigationRequests(t, client)
		return count == 1
	})
	pointerHoverReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": strings.Repeat("A long documentation paragraph.\n\n", 30)})
	pointerHoverUntil(t, hs, "lower-edge card layout", func() bool {
		return hs.a.hover != nil && hs.a.hoverQuery != nil && hs.a.hoverQuery.box.H > 0
	})
	q := hs.a.hoverQuery
	if q.box.Y >= q.anchor.Y || q.box.Y < titleBarH || q.box.Y+q.box.H > height-statusBarH || q.box.X < 0 || q.box.X+q.box.W > width {
		t.Fatalf("lower-edge card did not flip and stay inside the window: box=%+v anchor=%+v", q.box, q.anchor)
	}
	x, y = q.box.X+q.box.W/2, q.box.Y+q.box.H/2
	hs.tt.Move(x, y)
	hs.tt.Scroll(x, y, 0, 100)
	hs.tt.Advance(2 * hoverLeave)
	hs.tt.Frame()
	if hs.a.hoverQuery != q || hs.a.hover == nil {
		t.Fatal("scrolling inside the hover card dismissed it")
	}
}

func TestPointerHoverDiagnosticsWithoutServer(t *testing.T) {
	hs, d := editHarness(t, "diagnostic.txt", "alpha beta\n")
	h := withPlugins(hs, nil)
	h.setDiagnosticList("lint", d.path, "Test linter", false, []diagnostic{{
		Range: textRange{Start: position{0, 0}, End: position{0, 5}}, Severity: "warning", Message: "A useful warning", Code: "W001",
	}})
	x, y := pointerHoverRune(t, d, 1)
	hs.tt.Move(x, y)
	hs.tt.Advance(hoverDwell)
	hs.tt.Frame()
	pointerHoverUntil(t, hs, "diagnostic hover without a language server", func() bool {
		return slices.Contains(hs.tt.Texts(), "A useful warning")
	})
	if !slices.Contains(hs.tt.Texts(), "Test linter W001") {
		t.Fatal("diagnostic hover lost its source or code")
	}
	if hs.a.hoverQuery == nil || hs.a.hoverQuery.pending != 0 {
		t.Fatal("diagnostic-only hover waits for a nonexistent language server")
	}
}

func TestPointerHoverDoesNotPassThroughDialog(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	hs.a.openQuick("")
	hs.tt.Frame()
	if !hs.a.quick.open {
		t.Fatal("test did not open Quick Open")
	}
	x, y := pointerHoverRune(t, d, 9)
	hs.tt.Move(x, y)
	hs.tt.Advance(2 * hoverDwell)
	hs.tt.Frame()
	if _, count := sourceNavigationRequests(t, client); count != 0 || hs.a.hoverQuery != nil {
		t.Fatalf("mouse hover passed through Quick Open: requests=%d query=%+v", count, hs.a.hoverQuery)
	}
}

func TestPointerHoverDoesNotReplaceManualHover(t *testing.T) {
	for _, target := range []string{"blank", "punctuation", "gutter"} {
		t.Run(target, func(t *testing.T) {
			hs, d, client := sourceNavigationHarness(t)
			hs.place(d, 1, 1)
			x, y := pointerHoverRune(t, d, 5)
			switch target {
			case "blank":
				x = d.lastView.X + d.textX + 25*hs.a.font.charW
			case "gutter":
				x = d.lastView.X + d.textX - 8
			}
			hs.tt.Move(x, y)
			hs.a.showHover()
			q := hs.a.hoverQuery
			hs.tt.Frame()
			if q == nil || q.pointer || hs.a.hoverQuery != q {
				t.Fatal("pointer over non-symbol text dismissed the manual hover request")
			}
			pointerHoverUntil(t, hs, "manual hover request", func() bool {
				_, count := sourceNavigationRequests(t, client)
				return count == 1
			})
			pointerHoverReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": "Manually requested documentation"})
			pointerHoverUntil(t, hs, "manual documentation", func() bool { return hs.a.hover != nil })
			// The last source line lies below the card, so it is an actual
			// second semantic target rather than a pointer inside the card.
			at := posRune(d.buf.Snapshot(), position{3, 6})
			x, y = pointerHoverRune(t, d, at)
			hs.tt.Move(x, y)
			hs.tt.Advance(2 * hoverDwell)
			hs.tt.Frame()
			if hs.a.hoverQuery != q || hs.a.hover == nil || hs.a.hover.tree.Text != "Manually requested documentation" {
				t.Fatal("moving to another symbol replaced or dismissed manual documentation")
			}
			if _, count := sourceNavigationRequests(t, client); count != 1 {
				t.Fatalf("passive hover superseded manual request: %d requests", count)
			}
			hs.tt.Move(d.lastView.X-20, d.lastView.Y+d.lastView.H-20)
			hs.tt.Advance(2 * hoverLeave)
			hs.tt.Frame()
			if hs.a.hoverQuery != q {
				t.Fatal("leaving the editor dismissed manual documentation")
			}
		})
	}
}

func TestPointerHoverWrappedIdentifierAnchor(t *testing.T) {
	hs, d, client := sourceNavigationHarness(t)
	word := strings.Repeat("identifier", 40)
	hs.a.settings.WordWrap = true
	d.edit([2]int{0, 0}, textEdit{0, d.buf.Snapshot().Len(), word})
	hs.settle()
	rows := d.lines.Rows(0)
	if len(rows) < 3 {
		t.Fatalf("identifier did not wrap across three rows: %v", rows)
	}
	x0, y0 := pointerHoverRune(t, d, 1)
	x1, y1 := pointerHoverRune(t, d, rows[1]+1)
	x2, y2 := pointerHoverRune(t, d, rows[2]+1)
	hs.tt.Move(x0, y0)
	hs.tt.Frame()
	started := hs.a.pointerHover.since
	hs.tt.Advance(hoverDwell / 2)
	hs.tt.Move(x1, y1)
	hs.tt.Frame()
	if p := hs.a.pointerHover; p.since != started || !p.point.anchor.Contains(x1-d.lastView.X+d.hscroll.X, y1-d.lastView.Y) {
		t.Fatalf("same identifier's second row reset dwell or retained old anchor: since=%v want=%v anchor=%+v", p.since, started, p.point.anchor)
	}
	hs.tt.Advance(hoverDwell / 2)
	hs.tt.Frame()
	pointerHoverUntil(t, hs, "hover after crossing a soft wrap", func() bool {
		_, count := sourceNavigationRequests(t, client)
		return count == 1
	})
	// Tester.Move sends unmodified pointer events; reapply Cmd at each
	// endpoint to inspect the navigation underline on that visual row.
	for _, point := range []ui.Point{{X: x0, Y: y0}, {X: x2, Y: y2}} {
		hs.tt.Move(point.X, point.Y)
		hs.tt.HoldModifiers(ui.Cmd)
		hs.tt.Frame()
		anchor := hs.a.pointerHover.point.anchor
		if hs.tt.Cursor() != ui.CursorPointer || !anchor.Contains(point.X-d.lastView.X+d.hscroll.X, point.Y-d.lastView.Y) {
			t.Fatalf("Cmd underline did not follow the hovered wrapped row: cursor=%v anchor=%+v point=%+v", hs.tt.Cursor(), anchor, point)
		}
	}
}

func TestPointerHoverDefinitionCancellation(t *testing.T) {
	for _, action := range []string{"blur", "quick open"} {
		for _, reply := range []string{"late success", "queued success", "queued cancellation"} {
			t.Run(action+"/"+reply, func(t *testing.T) {
				hs, d, client := sourceNavigationHarness(t)
				hs.place(d, 9, 9)
				wantTab, wantSelection := hs.a.activeTab(), d.sel
				hs.a.goToDefinition()
				pointerHoverUntil(t, hs, "held definition", func() bool {
					count, _ := sourceNavigationRequests(t, client)
					return count == 1
				})
				if reply != "late success" {
					if reply == "queued cancellation" {
						hs.a.navigationCancel()
					} else {
						ctx, cancel := context.WithTimeout(context.Background(), time.Second)
						raw, err := client.Call(ctx, "test/replyDefinition", map[string]any{"index": 0, "uri": serverURI(d.path), "line": 2, "character": 5})
						cancel()
						if err != nil || string(raw) != "true" {
							t.Fatalf("queue successful definition reply: %s, %v", raw, err)
						}
					}
					deadline := time.Now().Add(5 * time.Second)
					for {
						hs.a.mu.Lock()
						queued := len(hs.a.queue) > 0
						hs.a.mu.Unlock()
						if queued {
							break
						}
						if time.Now().After(deadline) {
							t.Fatal("definition completion did not queue its callback")
						}
						time.Sleep(time.Millisecond)
					}
				}
				// Change the surface state before the next UI drain, as a
				// modal or focus change can happen before a posted reply runs.
				if action == "blur" {
					hs.a.blurred = true
					hs.tt.SetFocused(false)
				} else {
					hs.a.openQuick("")
				}
				hs.tt.Frame()
				if reply != "queued success" {
					pointerHoverReply(t, hs, client, "test/replyDefinition", map[string]any{"index": 0, "uri": serverURI(d.path), "line": 2, "character": 5})
				}
				hs.settle()
				if hs.a.definitionPending != nil || hs.a.activeTab() != wantTab || d.sel != wantSelection {
					t.Fatalf("canceled definition stayed active or navigated: pending=%v selection=%v want=%v", hs.a.definitionPending, d.sel, wantSelection)
				}
				if slices.Contains(hs.tt.Texts(), locales.DefinitionFailed(context.Canceled.Error())) || len(hs.a.notes) > 0 {
					t.Fatal("canceling a definition showed a failure notification")
				}
			})
		}
	}
}

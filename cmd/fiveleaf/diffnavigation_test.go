package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/lsp"
	"fiveleaf/locales"
)

func TestDiffNavigationUnchangedLine(t *testing.T) {
	for _, tc := range []struct {
		name, before, current string
		line, want            int
		ok                    bool
	}{
		{"same", "a\ncall()\nz\n", "a\ncall()\nz\n", 1, 1, true},
		{"insert before", "a\ncall()\nz\n", "intro\na\ncall()\nz\n", 1, 2, true},
		{"delete before", "intro\na\ncall()\nz\n", "a\ncall()\nz\n", 2, 1, true},
		{"changed", "a\ncall()\nz\n", "a\nother()\nz\n", 1, 0, false},
		{"deleted", "a\ncall()\nz\n", "a\nz\n", 1, 0, false},
		{"absent", "a\ncall()\nz\n", "", 1, 0, false},
		{"ambiguous duplicate", "call()\ncall()\n", "call()\n", 0, 0, false},
		{"unique context", "one\ncall()\ntwo\ncall()\n", "intro\none\ncall()\ntwo\ncall()\n", 1, 2, true},
		{"line endings", "a\r\ncall()\r\nz\r\n", "intro\na\ncall()\nz\n", 1, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := unchangedDiffLine(tc.before, tc.current, tc.line)
			if ok != tc.ok || ok && got != tc.want {
				t.Fatalf("mapped = %d, %v; want %d, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

type diffNavRequest struct {
	TextDocument struct{ URI string }
	Position     position
}

type diffNavDocument struct {
	TextDocument   struct{ URI, Text string }
	ContentChanges []struct{ Text string }
}

func (d diffNavDocument) text() string {
	if len(d.ContentChanges) > 0 {
		return d.ContentChanges[0].Text
	}
	return d.TextDocument.Text
}

type diffNavSnapshot struct {
	Definition, Hover                 diffNavRequest
	DefinitionCount, HoverCount       int
	DefinitionDocument, HoverDocument diffNavDocument
}

// diffNavHarness starts the actual test-binary language server, with its
// ordinary fixture defaults unchanged for the other LSP tests.
func diffNavHarness(t *testing.T, hold bool) (*harness, string, string) {
	t.Helper()
	t.Setenv("FIVELEAF_FAKE_LSP", "1")
	hs := newHarness(t, false)
	// The headless harness has no native macOS menu accelerator dispatch.
	// Exercise the actual focused keymap route instead.
	hs.a.settings.Keymap = "vscode"
	hs.a.applyKeymap(hs.a.userKeys)
	path, target := hs.path("source.fake"), hs.path("definition.fake")
	manifest, _ := json.Marshal(map[string]any{
		"name": "fake",
		"contributes": map[string]any{
			"languages": []any{map[string]any{"id": "fake", "extensions": []string{".fake"}}},
			"languageServers": []any{map[string]any{"id": "fake.server", "languages": []string{"fake"},
				"command": os.Args[0], "args": []string{"-test.run=^$"}, "initializationOptions": map[string]any{
					"definitionUri": serverURI(target), "definitionLine": 1, "definitionCharacter": 3, "holdDefinitions": hold,
				}}},
		},
	})
	host := withPlugins(hs, map[string]map[string]string{"fake": {"plugin.json": string(manifest)}})
	installCatalogFixtures(hs, host, "keymap-vscode")
	t.Cleanup(stopServers)
	if err := os.WriteFile(target, []byte("// target\n😀 target()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return hs, path, target
}

func diffNavClient(hs *harness, path string) *lsp.Client {
	i := hs.a.docIndex(path)
	if i < 0 {
		return nil
	}
	return hs.a.lspFor(hs.a.docs[i], false).live()
}

func diffNavState(t *testing.T, c *lsp.Client) diffNavSnapshot {
	t.Helper()
	if c == nil {
		t.Fatal("no language server")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := c.Call(ctx, "test/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	var s diffNavSnapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func diffNavRow(t *testing.T, v *diffView, side, line int) int {
	t.Helper()
	for i, r := range v.rows {
		n := r.old
		if side == 1 {
			n = r.new
		}
		if r.kind == rowLine && n == line {
			return i
		}
	}
	t.Fatalf("no side %d line %d", side, line)
	return 0
}

func diffNavAt(t *testing.T, hs *harness, v *diffView, row, side, col int) (float32, float32) {
	t.Helper()
	rc, ok := hs.tt.Find(locales.DiffPaneLabel())
	if !ok {
		t.Fatal("no diff pane")
	}
	g := hs.a.diffGeometry(v, v.split, v.viewW)
	top := editorPadY + float32(v.tops[row])*hs.a.font.lineH
	x := g.textX
	if side >= 0 {
		x += float32(side) * g.right()
		top, _ = hs.a.sideRowY(v, side, row)
	}
	text, _ := v.selText(row, side)
	shown, _ := expandTabs(text[:runeByte(text, col)])
	return rc.X + x + hs.a.font.width(shown) - v.scroll.X + 1,
		rc.Y + top - v.scroll.Y + hs.a.font.lineH/2
}

func diffNavReview(hs *harness, path, old, after string, layout diffLayout) *diffView {
	hs.a.reviewAgentChange(agentChange{path: path, oldText: old, newText: after, existed: true})
	v := hs.a.activeTab().diff
	v.full = true
	hs.a.setDiffLayout(v, layout)
	v.changed()
	hs.settle()
	return v
}

// A before/after snapshot maps to the unsaved working document, including
// UTF-16 columns, and the definition opens in a source tab beside the review.
func TestDiffNavigationDefinitionCurrentBuffer(t *testing.T) {
	for _, tc := range []struct {
		layout diffLayout
		side   int
	}{{layoutInline, -1}, {layoutSplit, 0}, {layoutSplit, 1}} {
		t.Run(tc.layout.key()+string(rune('0'+tc.side+1)), func(t *testing.T) {
			hs, path, target := diffNavHarness(t, false)
			a := hs.a
			a.settings.AutoSave = false
			old := "// before\n😀 call()\nend()\n"
			after := "// after\n😀 call()\nend()\n"
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			a.openFile(path, true)
			hs.settle()
			d := a.activeEditor()
			a.queueEdit(d, func(ed *editor) { ed.edit([2]int{}, textEdit{0, 0, "// unsaved\n"}) })
			hs.settle()
			current := d.buf.String()
			if !d.dirty() {
				t.Fatal("the working document is not unsaved")
			}
			v := diffNavReview(hs, path, old, after, tc.layout)
			review := a.activeTab()
			side := tc.side
			row := diffNavRow(t, v, max(0, side), 1)
			p := diffPoint{row, 2} // one astral rune, one space, then call
			v.textSel = &diffTextSel{side: side, from: p, to: p}
			v.pane.Focus()
			hs.settle()
			hs.tt.Key(0, ui.KeyF12)
			hs.untilLong("definition source tab", func() bool { return a.activeEditor() != nil && a.activeEditor().path == target })
			hs.settle()
			s := diffNavState(t, diffNavClient(hs, path))
			if s.DefinitionCount != 1 || s.Definition.TextDocument.URI != serverURI(path) || s.Definition.Position != (position{2, 3}) {
				t.Fatalf("definition request = %+v (%d)", s.Definition, s.DefinitionCount)
			}
			if got := s.DefinitionDocument.text(); got != current {
				t.Fatalf("server document = %q, want current unsaved %q", got, current)
			}
			if col, _ := a.locate(review); col == nil || review.diff != v || v.files[0].old.text != old {
				t.Fatal("navigation replaced or modified the review")
			}
			if got := caretPosition(a.activeEditor()); got != (position{1, 3}) {
				t.Fatalf("definition caret = %+v", got)
			}
		})
	}
}

func TestDiffNavigationRejectsChangedAndDeletedLines(t *testing.T) {
	for _, current := range []string{"other()\n", ""} {
		t.Run(current, func(t *testing.T) {
			hs, path, _ := diffNavHarness(t, false)
			if err := os.WriteFile(path, []byte(current), 0o644); err != nil {
				t.Fatal(err)
			}
			a := hs.a
			v := diffNavReview(hs, path, "call()\n", "new()\n", layoutSplit)
			row := diffNavRow(t, v, 0, 0)
			a.diffNavigate(v, v.navigationPoint(diffPoint{row, 1}, 0), false)
			hs.until("unavailable navigation", func() bool {
				return hs.tt.HasText(locales.DiffNavigationUnavailable())
			})
			hs.untilLong("server ready", func() bool { return diffNavClient(hs, path) != nil })
			if s := diffNavState(t, diffNavClient(hs, path)); s.DefinitionCount != 0 {
				t.Fatalf("queried a changed or deleted historical line: %+v", s.Definition)
			}
		})
	}
}

func TestDiffNavigationMenuAndModifierClick(t *testing.T) {
	hs, path, target := diffNavHarness(t, false)
	a := hs.a
	old, after := "old()\ncall()\n", "new()\ncall()\n"
	if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
		t.Fatal(err)
	}
	v := diffNavReview(hs, path, old, after, layoutSplit)
	review := a.activeTab()
	row := diffNavRow(t, v, 0, 1)
	x, y := diffNavAt(t, hs, v, row, 0, 1)
	hs.tt.RightClickAt(x, y)
	hs.settle()
	if !slices.Contains(hs.tt.Menu(), locales.CmdGoDefinition()) || !slices.Contains(hs.tt.Menu(), locales.CmdGoHover()) {
		t.Fatalf("navigation menu = %v", hs.tt.Menu())
	}
	if err := hs.tt.ChooseMenuItem(locales.CmdGoHover()); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("hover in review", func() bool { return a.hover != nil })
	if a.activeTab() != review || v.editing != nil || v.files[0].new.text != after {
		t.Fatal("right click changed the view or its contents")
	}
	c := diffNavClient(hs, path)
	if s := diffNavState(t, c); s.HoverCount != 1 || s.Hover.TextDocument.URI != serverURI(path) || s.Hover.Position != (position{1, 1}) || s.HoverDocument.text() != after {
		t.Fatalf("hover request = %+v", s)
	}
	a.hover = nil
	hs.settle()
	x, y = diffNavAt(t, hs, v, row, 1, 2)
	hs.tt.ClickAtWith(ui.Cmd, x, y)
	hs.untilLong("modifier click definition", func() bool { return a.activeEditor() != nil && a.activeEditor().path == target })
	if s := diffNavState(t, c); s.DefinitionCount != 1 || s.Definition.Position != (position{1, 2}) {
		t.Fatalf("modifier click queried %+v (%d)", s.Definition, s.DefinitionCount)
	}
	if v.editing != nil || v.files[0].old.text != old || v.files[0].new.text != after {
		t.Fatal("modifier click edited the immutable review")
	}
	// The review's hidden source document is released with the review.
	a.dropTab(review)
	hs.settle()
	if a.docIndex(path) >= 0 {
		t.Fatal("the review leaked its hidden source document")
	}
}

func TestDiffNavigationLocalRightUsesPeer(t *testing.T) {
	hs, path, _ := diffNavHarness(t, false)
	a := hs.a
	peer := hs.path("peer.fake")
	if err := os.WriteFile(peer, []byte("rightCall()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := diffNavReview(hs, path, "leftCall()\n", "rightCall()\n", layoutSplit)
	f := v.files[0]
	f.base, f.peerPath = diffLocal, peer
	row := diffNavRow(t, v, 1, 0)
	a.diffNavigate(v, v.navigationPoint(diffPoint{row, 3}, 1), true)
	hs.untilLong("right file hover", func() bool { return a.hover != nil })
	if s := diffNavState(t, diffNavClient(hs, peer)); s.Hover.TextDocument.URI != serverURI(peer) || s.HoverDocument.text() != "rightCall()\n" {
		t.Fatalf("right side queried the wrong file: %+v", s)
	}
}

func TestDiffNavigationIgnoresStaleDefinition(t *testing.T) {
	for _, changed := range []string{"document", "tab", "workspace"} {
		t.Run(changed, func(t *testing.T) {
			hs, path, target := diffNavHarness(t, true)
			a := hs.a
			text := "call()\n"
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			v := diffNavReview(hs, path, "old()\n", text, layoutInline)
			row := diffNavRow(t, v, 1, 0)
			a.diffNavigate(v, v.navigationPoint(diffPoint{row, 1}, -1), false)
			hs.untilLong("held definition request", func() bool {
				c := diffNavClient(hs, path)
				return c != nil && diffNavState(t, c).DefinitionCount == 1
			})
			c := diffNavClient(hs, path)
			switch changed {
			case "document":
				d := a.docs[a.docIndex(path)]
				d.buf.Set("other()\n")
			case "tab":
				a.openFile(path, true)
			case "workspace":
				a.root += "-changed"
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := c.Call(ctx, "test/releaseDefinitions", nil); err != nil {
				t.Fatal(err)
			}
			// The fake responds before the release response. Drain the app once
			// its query callback has posted, without relying on a timing delay.
			if changed == "document" {
				hs.until("stale navigation feedback", func() bool {
					return hs.tt.HasText(locales.DiffNavigationUnavailable())
				})
			} else {
				hs.settle()
			}
			if ed := a.activeEditor(); ed != nil && ed.path == target {
				t.Fatal("stale definition changed the active tab")
			}
		})
	}
}

func TestDiffNavigationEditingF12(t *testing.T) {
	hs, path, target := diffNavHarness(t, false)
	a := hs.a
	text := "call()\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	v := diffNavReview(hs, path, "old()\n", text, layoutSplit)
	f := v.files[0]
	f.base = diffIndex
	f.gitStale, f.diskStale = false, false
	row := diffNavRow(t, v, 1, 0)
	if !a.beginDiffEdit(v, row) {
		t.Fatalf("could not edit diff: errors=%v file=%+v", a.errors, f)
	}
	v.editing.ed.selectRange(1, 1, false)
	v.pane.Focus()
	hs.settle()
	hs.tt.Key(0, ui.KeyF12)
	hs.untilLong("definition from editable diff", func() bool { return a.activeEditor() != nil && a.activeEditor().path == target })
	if got := v.editing.doc.buf.String(); got != text {
		t.Fatalf("F12 edited the buffer: %q", got)
	}
}

func TestDiffNavigationPointersPreserveEditableSelection(t *testing.T) {
	hs, v, path, at := diffSelTab(t, layoutSplit)
	a := hs.a
	row := diffEditRow(v, path, 25)
	if !a.beginDiffEdit(v, row) {
		t.Fatal("could not edit diff")
	}
	ed := v.editing.ed
	ed.selectRange(2, 6, false)
	before, selection := ed.buf.String(), ed.sel
	hs.settle()
	x, y := at(diffEditRow(v, path, 26), 1, 2)
	hs.tt.RightClickAt(x, y)
	hs.settle()
	if err := hs.tt.ChooseMenuItem(locales.CmdGoDefinition()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.tt.ClickAtWith(ui.Cmd, x, y)
	hs.settle()
	if ed.buf.String() != before || ed.sel != selection || v.editing.ed != ed || v.editing.drag {
		t.Fatalf("navigation changed editable selection: %v -> %v", selection, ed.sel)
	}
	if disk, err := os.ReadFile(path); err != nil || string(disk) != before {
		t.Fatalf("navigation wrote a hunk to disk: %q, %v", disk, err)
	}
}

func TestDiffNavigationReleaseKeepsEditsAndPendingSaves(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "edited", true: "pending save"}[pending], func(t *testing.T) {
			hs, path, _ := diffNavHarness(t, false)
			a := hs.a
			if err := os.WriteFile(path, []byte("call()\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			v := diffNavReview(hs, path, "old()\n", "call()\n", layoutSplit)
			review := a.activeTab()
			row := diffNavRow(t, v, 1, 0)
			a.diffNavigate(v, v.navigationPoint(diffPoint{row, 1}, 1), true)
			hs.untilLong("review's current document", func() bool { return a.hover != nil })
			a.hover = nil
			d := a.docs[a.docIndex(path)]
			if pending {
				d.sync = syncSaving
			} else {
				d.buf.Set("unsaved()\n")
			}
			a.dropTab(review)
			if a.docIndex(path) < 0 {
				t.Fatal("closing the review lost a document with work to retain")
			}
			if ed := a.activeEditor(); ed != nil && ed.document == d {
				t.Fatal("review release mutated the tab array synchronously")
			}
			hs.settle()
			if ed := a.activeEditor(); ed == nil || ed.document != d {
				t.Fatal("retained document did not open in an ordinary source tab")
			}
			if !pending && d.buf.String() != "unsaved()\n" || pending && d.sync != syncSaving {
				t.Fatal("review release changed the retained document")
			}
		})
	}
}

func TestDiffNavigationSourceCoordinates(t *testing.T) {
	text := "\t😀 identifier()\n"
	f := agentSnapshotFile(agentChange{path: "/source.fake", oldText: "before()\n", newText: text, existed: true})
	v := newDiffView(layoutInline)
	v.files = []*diffFile{f}
	v.rows = []diffRow{{kind: rowLine, file: 0, old: -1, new: 0}}
	col := utf8.RuneCountInString(strings.Split(text, "identifier")[0])
	p := v.navigationPoint(diffPoint{0, col}, -1)
	if p == nil || p.line != 0 || p.col != 3 || p.text != text || !p.newSide {
		t.Fatalf("point = %+v", p)
	}
	if p := v.navigationPoint(diffPoint{0, 0}, -1); p != nil {
		t.Fatalf("indent became a symbol: %+v", p)
	}
}

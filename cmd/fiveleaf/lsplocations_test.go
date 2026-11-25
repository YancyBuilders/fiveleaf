package main

import (
	"os"
	"strings"
	"testing"

	"fiveleaf/internal/lsp"
	"fiveleaf/locales"
)

func definitionTargetsHarness(t *testing.T) (*harness, *editor) {
	t.Helper()
	hs := newHarness(t, false)
	hs.a.settings.AutoSave = false
	path := hs.path("definition-source.txt")
	if err := os.WriteFile(path, []byte("first()\nsecond()\nthird()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	return hs, hs.a.activeEditor()
}

func definitionTarget(path string, line, character int) lsp.Location {
	pos := lsp.Position{Line: line, Character: character}
	return lsp.Location{URI: serverURI(path), Range: lsp.Range{Start: pos, End: pos}}
}

func TestDefinitionTargetsDeduplicate(t *testing.T) {
	hs, ed := definitionTargetsHarness(t)
	a := hs.a
	n := a.navigationAt(ed.document, caretPosition(ed))
	loc := definitionTarget(ed.path, 1, 2)
	duplicate := loc
	duplicate.Range.End.Character = 10
	a.presentDefinitions(n, []lsp.Location{{URI: "https://example.test/source"}, loc, duplicate})
	hs.settle()
	if a.pluginPick.open {
		t.Fatal("identical destinations opened a picker")
	}
	if got := caretPosition(ed); got != (position{Line: 1, Character: 2}) {
		t.Fatalf("position = %+v, want line 1, character 2", got)
	}
}

func TestDefinitionTargetsChoose(t *testing.T) {
	hs, ed := definitionTargetsHarness(t)
	a := hs.a
	path := hs.path("other.txt")
	if err := os.WriteFile(path, []byte("😀name\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := a.navigationAt(ed.document, caretPosition(ed))
	a.presentDefinitions(n, []lsp.Location{definitionTarget(ed.path, 1, 2), definitionTarget(path, 0, 2)})
	if !a.pluginPick.open || len(a.pluginPick.items) != 2 || a.activeEditor() != ed {
		t.Fatal("multiple destinations did not wait for a choice")
	}
	if item := a.pluginPick.items[1]; !strings.Contains(item.Label, "other.txt:1:3") || item.Description != "other.txt" {
		t.Fatalf("destination lacks its file and position: %+v", item)
	}
	a.closePluginPicker(1)
	hs.settle()
	dest := a.activeEditor()
	if dest == nil || dest.path != path {
		t.Fatalf("choice opened %+v", dest)
	}
	if dest.sel != [2]int{1, 1} {
		t.Fatalf("UTF-16 destination selected runes %v, want [1 1]", dest.sel)
	}
}

func TestDefinitionTargetsCancel(t *testing.T) {
	hs, ed := definitionTargetsHarness(t)
	a := hs.a
	sel := ed.sel
	n := a.navigationAt(ed.document, caretPosition(ed))
	a.presentDefinitions(n, []lsp.Location{definitionTarget(ed.path, 1, 2), definitionTarget(ed.path, 2, 1)})
	a.closePluginPicker(-1)
	hs.settle()
	if a.activeEditor() != ed || ed.sel != sel || a.pluginPick.open {
		t.Fatal("cancelled choice navigated")
	}
}

func TestDefinitionTargetsUnsupported(t *testing.T) {
	hs, ed := definitionTargetsHarness(t)
	a := hs.a
	sel := ed.sel
	n := a.navigationAt(ed.document, caretPosition(ed))
	a.presentDefinitions(n, []lsp.Location{{URI: "https://example.test/source"}, {URI: "untitled:source"}})
	if a.pluginPick.open || a.activeEditor() != ed || ed.sel != sel {
		t.Fatal("unsupported destination navigated")
	}
	if len(a.notes) == 0 || a.notes[len(a.notes)-1].message != locales.DefinitionNone() {
		t.Fatal("unsupported destinations produced no feedback")
	}
}

func TestDefinitionTargetsStaleChoice(t *testing.T) {
	for _, change := range []string{"query", "edit", "tab", "workspace"} {
		t.Run(change, func(t *testing.T) {
			hs, ed := definitionTargetsHarness(t)
			a := hs.a
			n := a.navigationAt(ed.document, caretPosition(ed))
			a.presentDefinitions(n, []lsp.Location{definitionTarget(ed.path, 1, 2), definitionTarget(ed.path, 2, 1)})
			switch change {
			case "query":
				a.navigationAt(ed.document, position{})
			case "edit":
				ed.buf.Set("changed\n")
			case "tab":
				a.openFile(hs.path("README.md"), true)
			case "workspace":
				a.root = hs.path("web")
				defer func() { a.root = n.root }()
			}
			tab, sel := a.activeTab(), ed.sel
			a.closePluginPicker(1)
			if a.activeTab() != tab || ed.sel != sel || ed.pending != nil {
				t.Fatal("stale choice moved the selection or changed the tab")
			}
		})
	}
}

func TestDefinitionTargetsDelayedRead(t *testing.T) {
	for _, change := range []string{"query", "tab", "edit"} {
		t.Run(change, func(t *testing.T) {
			hs, ed := definitionTargetsHarness(t)
			a := hs.a
			path := hs.path("target.txt")
			if err := os.WriteFile(path, []byte("first\nsecond target\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			release, _ := blockLocalReads(t, a)
			n := a.navigationAt(ed.document, caretPosition(ed))
			a.presentDefinitions(n, []lsp.Location{definitionTarget(path, 1, 7)})
			dest := a.activeEditor()
			if dest == ed || !dest.unread() {
				t.Fatal("test did not hold the destination read")
			}
			switch change {
			case "query":
				a.navigationAt(ed.document, position{})
			case "tab":
				a.openFile(ed.path, true)
			case "edit":
				ed.buf.Set("changed\n")
			}
			tab := a.activeTab()
			release()
			hs.settle()
			if a.activeTab() != tab || dest.sel != [2]int{} || !dest.landAt.IsZero() {
				t.Fatal("late destination read applied a stale landing")
			}
		})
	}
}

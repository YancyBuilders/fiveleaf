package main

import (
	"fmt"
	"path/filepath"
	"slices"

	"fiveleaf/internal/lsp"
	"fiveleaf/locales"
)

// presentDefinitions keeps distinct destinations when a server resolves a
// symbol to more than one declaration, such as an overloaded function.
func (a *app) presentDefinitions(n lspNavigation, locs []lsp.Location) {
	if !a.navigationCurrent(n) {
		return
	}
	type target struct {
		path string
		pos  lsp.Position
	}
	seen := make(map[target]bool, len(locs))
	targets := make([]target, 0, len(locs))
	for _, loc := range locs {
		t := target{serverPath(n.root, loc.URI), loc.Range.Start}
		if t.path == "" || t.pos.Line < 0 || t.pos.Character < 0 || seen[t] {
			continue
		}
		seen[t] = true
		targets = append(targets, t)
	}
	open := func(i int) {
		if i < 0 || i >= len(targets) || !a.navigationCurrent(n) {
			return
		}
		t := targets[i]
		if n.tab != nil {
			a.keepTab(n.tab)
		}
		a.openFileAt(t.path, t.pos.Line, true)
		tab := a.activeTab()
		if ed := a.activeEditor(); ed != nil && ed.path == t.path {
			ed.whenRead(func(ed *editor) {
				// Opening the destination changes the active tab deliberately.
				// A later query, edit or tab switch still cancels this landing.
				if a.navigationSerial != n.serial || a.root != n.root || a.activeTab() != tab || a.activeEditor() != ed ||
					n.doc.path != n.path || n.doc.buf.Version() != n.version || !slices.Contains(a.docs, n.doc) {
					return
				}
				at := posRune(ed.buf.Snapshot(), fromLSP(t.pos))
				ed.selectRange(at, at, true)
				ed.lightLanding()
			})
		}
	}
	switch len(targets) {
	case 0:
		a.tell(note{message: locales.DefinitionNone()})
	case 1:
		open(0)
	default:
		items := make([]pickItem, len(targets))
		for i, t := range targets {
			items[i] = pickItem{
				Label:       fmt.Sprintf("%s:%d:%d", filepath.Base(t.path), t.pos.Line+1, t.pos.Character+1),
				Description: a.relOrPath(t.path),
				Icon:        "file-code",
			}
		}
		a.openPluginPicker(items, locales.CmdGoDefinition(), open)
	}
}

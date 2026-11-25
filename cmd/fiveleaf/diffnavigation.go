package main

import (
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/highlight"
	"fiveleaf/internal/linediff"
	"fiveleaf/locales"
)

// diffNavigationPoint keeps the exact version and column under the pointer.
// LSP only sees the current document; this version is used solely to map an
// unchanged line into that document.
type diffNavigationPoint struct {
	file       *diffFile
	path, text string
	line, col  int
	newSide    bool
}

type diffNavigationDocument struct {
	doc         *document
	navigation  *editor
	inlineViews int
	discard     bool // the normal close confirmation approved discarding this buffer
}

func navigationSavePending(d *document) bool {
	return d.sync == syncSaving || d.sync == syncFormatting || d.sync == syncConflict || len(d.waiters)+len(d.queued) > 0
}

func (held *diffNavigationDocument) views() int {
	return held.inlineViews + btoi(held.navigation != nil)
}

func (held *diffNavigationDocument) ownsLastViews() bool {
	owned := held.views()
	return owned > 0 && owned == len(held.doc.views)
}

func (a *app) diffDocumentView(v *diffView, doc *document) *editor {
	held := a.retainDiffDocument(v, doc)
	if held.navigation == nil {
		held.navigation = a.editorForDocument(doc)
	}
	return held.navigation
}

func (v *diffView) canDetach() bool {
	return v.editing == nil && !v.navigationClosing && !slices.ContainsFunc(v.navigationDocs, func(held *diffNavigationDocument) bool {
		return held.views() > 0 && (held.doc.dirty() || navigationSavePending(held.doc))
	})
}

func (a *app) retainDiffDocument(v *diffView, d *document) *diffNavigationDocument {
	for _, held := range v.navigationDocs {
		if held.doc == d {
			return held
		}
	}
	held := &diffNavigationDocument{doc: d}
	root := a.root
	v.navigationDocs = append(v.navigationDocs, held)
	v.navigationReleases = append(v.navigationReleases, func() {
		if held.views() == 0 {
			return
		}
		if held.navigation != nil {
			held.navigation.release()
		}
		if len(d.views) != 0 {
			return
		}
		if d.dirty() && !held.discard || navigationSavePending(d) {
			path := d.path
			a.post(func() {
				if a.root == root && !a.loadsClosed && d.path == path && slices.Contains(a.docs, d) && len(d.views) == 0 {
					a.openText(func() { a.openFile(path, false) })
				}
			})
			return
		}
		d.hl.Close()
		a.docs = slices.DeleteFunc(a.docs, func(doc *document) bool { return doc == d })
		a.docClosed(d)
		if a.tree != nil {
			a.tree.Unwatch(d.path)
		}
	})
	return held
}

// closeDiffNavigation applies the ordinary save/discard/cancel lifecycle to
// the last view of each current document retained by a review. Hidden views
// count too: the last review or source tab closed owns the confirmation.
func (a *app) closeDiffNavigation(tab *colTab, done, cancelled func()) {
	v, root := tab.diff, a.root
	if v.navigationClosing {
		cancelled()
		return
	}
	v.navigationClosing = true
	type discardVersion struct {
		version uint64
		crlf    bool
	}
	discarded := map[*document]discardVersion{}
	approved := func(d *document) bool {
		ver, ok := discarded[d]
		return ok && ver == (discardVersion{d.buf.Version(), d.crlf})
	}
	stop := func() {
		v.navigationClosing = false
		cancelled()
	}
	current := func() bool {
		col, _ := a.locate(tab)
		return a.root == root && col != nil
	}
	var next func(int)
	next = func(i int) {
		if !current() {
			stop()
			return
		}
		if i == len(v.navigationDocs) {
			// Another document's save or confirmation can take time. Recheck
			// earlier decisions so later plugin edits are never discarded by
			// an approval for an older buffer (or by an earlier successful save).
			for at, held := range v.navigationDocs {
				d := held.doc
				if held.ownsLastViews() && slices.Contains(a.docs, d) &&
					(navigationSavePending(d) || d.dirty() && !approved(d)) {
					next(at)
					return
				}
			}
			for _, held := range v.navigationDocs {
				held.discard = approved(held.doc)
			}
			v.navigationClosing = false
			done()
			return
		}
		held := v.navigationDocs[i]
		d := held.doc
		if !held.ownsLastViews() || !slices.Contains(a.docs, d) {
			next(i + 1)
			return
		}
		if navigationSavePending(d) {
			// Observe the save already requested, without saving newer edits.
			a.requestSave(d, saveWaiter{waitOnly: true, done: func() { next(i) }, failed: func(error) { stop() }})
			return
		}
		if !d.dirty() || approved(d) {
			next(i + 1)
			return
		}
		save := func() { a.saveDocElse(d, func() { next(i + 1) }, stop) }
		if a.autoSaves(d) {
			save()
			return
		}
		version := discardVersion{d.buf.Version(), d.crlf}
		a.host.confirmClose(d.name(), func(choice int) {
			if !current() {
				stop()
				return
			}
			switch choice {
			case 0:
				save()
			case 1:
				discarded[d] = version
				next(i + 1)
			default:
				stop()
			}
		})
	}
	next(0)
}

func (v *diffView) navigationPoint(p diffPoint, side int) *diffNavigationPoint {
	if p.row < 0 || p.row >= len(v.rows) {
		return nil
	}
	r := v.rows[p.row]
	if r.kind != rowLine || r.file < 0 || r.file >= len(v.files) {
		return nil
	}
	f := v.files[r.file]
	line, newSide := r.line()
	if side >= 0 {
		line, newSide = r.old, side == 1
		if newSide {
			line = r.new
		}
	}
	s := f.side(newSide)
	if !f.loaded || f.binary || !s.ok || line < 0 || line >= len(s.lines) {
		return nil
	}
	col, ok := diffSymbolColumn(s.lines[line], p.col)
	if !ok {
		return nil
	}
	path := f.path
	if f.base == diffLocal && newSide {
		path = f.peerPath
	}
	return &diffNavigationPoint{f, path, s.text, line, col, newSide}
}

// diffSymbolColumn accepts a symbol, including a caret just past its end.
// Whitespace elsewhere and gutters do not silently navigate another token.
func diffSymbolColumn(text string, col int) (int, bool) {
	rs := []rune(text)
	if col < 0 || col > len(rs) {
		return 0, false
	}
	if col < len(rs) && isWordRune(rs[col]) {
		return col, true
	}
	if col > 0 && isWordRune(rs[col-1]) {
		return col - 1, true
	}
	return 0, false
}

func (v *diffView) selectedNavigationPoint() *diffNavigationPoint {
	if s := v.textSel; s != nil {
		lo, _ := s.ordered()
		return v.navigationPoint(lo, s.side)
	}
	return nil
}

func (a *app) diffNavigateSelection(v *diffView, hover bool) {
	if e := v.editing; e != nil && v.textSel == nil {
		// This caret belongs to the live editor, including an edit whose diff
		// rows have not caught up yet. Its position needs no historical map.
		v.navigationSerial++
		serial := v.navigationSerial
		n := a.navigationAt(e.doc, caretPosition(e.ed))
		n.valid = func() bool { return v.editing == e && v.navigationSerial == serial }
		if hover {
			a.queryHover(n)
		} else {
			a.queryDefinition(n)
		}
		return
	}
	a.diffNavigate(v, v.selectedNavigationPoint(), hover)
}

// unchangedDiffLine maps only exact unchanged lines. A duplicate line needs
// unique unchanged surrounding context too, so a deletion among repeated
// calls cannot make a historical call point at a different surviving one.
func unchangedDiffLine(before, current string, line int) (int, bool) {
	before, current = strings.ReplaceAll(before, "\r\n", "\n"), strings.ReplaceAll(current, "\r\n", "\n")
	a, b := splitLines(before, true), splitLines(current, true)
	if line < 0 || line >= len(a) {
		return 0, false
	}
	if before == current {
		return line, true
	}
	delta, lo, hi := 0, 0, len(a)
	for _, h := range linediff.Hunks(a, b) {
		if line < h.A {
			hi = h.A
			break
		}
		if line < h.A+h.ALen {
			return 0, false
		}
		delta += h.BLen - h.ALen
		lo = h.A + h.ALen
	}
	mapped := line + delta
	if mapped < 0 || mapped >= len(b) || a[line] != b[mapped] {
		return 0, false
	}
	// At most two unchanged lines on either side establish identity. If that
	// context is repeated, refusing is safer than choosing a Myers alignment.
	from, to := max(lo, line-2), min(hi, line+3)
	block := a[from:to]
	unique := func(lines []string) bool {
		found := false
		for i := 0; i+len(block) <= len(lines); i++ {
			if slices.Equal(lines[i:i+len(block)], block) {
				if found {
					return false
				}
				found = true
			}
		}
		return found
	}
	return mapped, unique(a) && unique(b)
}

func (a *app) diffNavigate(v *diffView, p *diffNavigationPoint, hover bool) {
	tab, root := a.activeTab(), a.root
	if tab == nil || tab.diff != v {
		return
	}
	if p == nil || p.path == "" {
		a.tell(note{message: locales.DiffNavigationSelectSymbol()})
		return
	}
	v.navigationSerial++
	serial := v.navigationSerial
	sameView := func() bool { return a.root == root && a.activeTab() == tab && v.navigationSerial == serial }
	valid := func() bool {
		return sameView() && slices.Contains(v.files, p.file) && p.file.side(p.newSide).text == p.text
	}
	owner, d := a.tabDocument(p.path)
	newDoc := d == nil
	if newDoc {
		a.nextID++
		d = newDocument(a.nextID, p.path, "", highlight.New(highlight.Lexer(p.path), a.invalidate))
		a.docs = append(a.docs, d)
	} else {
		a.shareTabDocument(owner, d)
	}
	a.diffDocumentView(v, d)
	if newDoc {
		a.loadFile(d)
		if a.tree != nil {
			a.tree.Watch(d.path)
		}
	}
	ready := func() {
		if !sameView() {
			return
		}
		if !valid() || d.locked() || !slices.Contains(a.docs, d) {
			a.tell(note{message: locales.DiffNavigationUnavailable()})
			return
		}
		current := d.buf.String()
		n := a.navigationAt(d, position{})
		n.valid = valid
		bg.Go(func() {
			line, ok := unchangedDiffLine(p.text, current, p.line)
			col := 0
			if ok {
				for i, r := range []rune(splitLines(p.text, true)[p.line]) {
					if i >= p.col {
						break
					}
					col += utf16.RuneLen(r)
				}
			}
			a.post(func() {
				if !a.navigationCurrent(n) {
					return
				}
				if !ok {
					a.tell(note{message: locales.DiffNavigationUnavailable()})
					return
				}
				n.pos = position{Line: line, Character: col}
				if hover {
					a.queryHover(n)
				} else {
					a.queryDefinition(n)
				}
			})
		})
	}
	if d.sync == syncLoading {
		d.readWaits = append(d.readWaits, ready)
	} else {
		ready()
	}
}

// diffNavigationInput runs before editing or hunk handling. Secondary clicks
// only capture the exact menu target; the menu system still receives them.
func (a *app) diffNavigationInput(v *diffView, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputPointerDown || ev.Button != 1 && !(ev.Button == 0 && ev.Mods == ui.Cmd) {
		return false
	}
	v.navigationMenu = nil
	l := a.diffGeometry(v, v.split, v.viewW)
	side, ok := onDiffText(v, l, ev.X)
	if !ok || a.overDiffControl(v, ev.X, ev.Y) {
		return ev.Button == 0
	}
	point, ok := a.diffTextAt(v, l, ev.X, ev.Y, side)
	if !ok {
		return ev.Button == 0
	}
	p := v.navigationPoint(point, side)
	if ev.Button == 1 {
		v.navigationMenu = p
		return false
	}
	v.textDrag = false
	v.pane.Focus()
	a.diffNavigate(v, p, false)
	return true
}

func (a *app) diffNavigationMenu(v *diffView, m *ui.Menu) {
	p := v.navigationMenu
	if m.Item(locales.CmdGoDefinition()).Disabled(p == nil).Chosen() {
		a.post(func() { a.diffNavigate(v, p, false) })
	}
	if m.Item(locales.CmdGoHover()).Disabled(p == nil).Chosen() {
		a.post(func() { a.diffNavigate(v, p, true) })
	}
}

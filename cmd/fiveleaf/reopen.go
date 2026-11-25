package main

import (
	"os"
	"reflect"
	"slices"

	"fiveleaf/internal/git"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
)

// maxClosed is how many closed tabs the window remembers.
const maxClosed = 30

// closedTab keeps the content and position needed to reopen a tab with ⇧⌘T.
type closedTab struct {
	chat      *agentChat
	path      string
	col, at   int // the column's id and the tab's place there
	colAt     int
	neighbor  int
	direction splitDirection
	share     float32
	panes     *closedPaneChange
	preview   bool // a document showed as a rendered preview
	live      bool
	// diff is set for the tab of the changes, laid out as layout; full is
	// set for every unchanged line shown.
	diff, full bool
	layout     diffLayout
	// commit is the commit whose changes the tab showed, "" for those of
	// the work tree.
	commit, repoRoot string
	sel              [2]int
	caret            int
	x, y             float32
	wrap             *bool
	wrapGen          int
}

type closedPaneChange struct{ before, after paneSnapshot }

type paneSnapshot struct {
	groups  []settings.PaneTile
	columns []paneStamp
}

type paneStamp struct {
	id     int
	weight float32
	locked bool
}

func (a *app) paneSnapshot() paneSnapshot {
	snapshot := paneSnapshot{groups: a.panes.savedLayout(), columns: make([]paneStamp, len(a.cols))}
	for index, col := range a.cols {
		snapshot.columns[index] = paneStamp{col.id, col.weight, col.locked}
	}
	return snapshot
}

// rememberClosed keeps a tab about to close, to open again. An untitled
// document's tab is not kept: its text goes with it.
func (a *app) rememberClosed(t *colTab) {
	if t.diff != nil && t.diff.snapshot != "" {
		return
	}
	if t.term != nil {
		return
	}
	col, i := a.locate(t)
	if col == nil {
		return
	}
	c := closedTab{col: col.id, at: i, colAt: slices.Index(a.cols, col)}
	if siblings := col.parent.children; len(siblings) > 1 {
		neighbor := slices.Index(siblings, &col.paneTile) - 1
		c.direction = splitRight
		if col.parent.vertical {
			c.direction = splitDown
		}
		if neighbor < 0 {
			neighbor = 1
			c.direction = splitLeft
			if col.parent.vertical {
				c.direction = splitUp
			}
		}
		adjacent := siblings[neighbor]
		c.neighbor = adjacent.outerLeaf(!c.direction.after()).id
		c.share = col.weight / (col.weight + adjacent.weight)
	}
	switch {
	case t.tool != nil && t.tool.chat != nil:
		c.chat = t.tool.chat
	case t.diff != nil:
	case t.path() == "":
		return
	default:
		c.path = t.path()
	}
	if v := t.diff; v != nil {
		c.diff, c.layout, c.full = true, v.layout, v.full
		c.repoRoot = v.repoRoot
		if v.commit != nil {
			c.commit = v.commit.key()
		}
		c.y, c.wrap, c.wrapGen = v.scroll.Y, v.wrap, v.wrapGen
	}
	if ed := t.ed; ed != nil {
		c.preview = ed.preview != nil
		c.live = ed.live
		c.sel, c.caret = ed.sel, ed.caret
		c.x, c.y = ed.hscroll.X, ed.vscroll.Y
		c.wrap, c.wrapGen = ed.wrap, ed.wrapGen
	}
	a.closed = append(a.closed, c)
	if len(a.closed) > maxClosed {
		evicted := a.closed[0].chat
		a.closed = slices.Delete(a.closed, 0, 1)
		a.releaseUnusedChatGroup(evicted)
	}
}

// reopenClosed opens the tab closed last again, in its column and place,
// making the column again where it was when its last tab closed. Files
// gone from the disk are passed over.
func (a *app) reopenClosed() {
	for len(a.closed) > 0 {
		c := a.closed[len(a.closed)-1]
		a.closed = slices.Delete(a.closed, len(a.closed)-1, len(a.closed))
		if c.chat != nil {
			if col, index := a.chatTab(c.chat); col != nil {
				a.activate(col, index, true)
				return
			}
			col, _ := a.closedColumn(c)
			a.openChatPageIn(col, c.at, c.chat)
			return
		}
		if c.path != "" {
			if _, err := os.Stat(c.path); err != nil && !remote.Is(c.path) {
				continue
			}
		}
		col, made := a.closedColumn(c)
		if c.diff {
			v := newDiffView(c.layout)
			if c.commit != "" {
				v = newCommitView(c.layout, c.commit, git.Commit{})
			}
			v.repoRoot = c.repoRoot
			if index := col.showingDiff(v); index >= 0 {
				a.activate(col, index, true)
				return
			}
			v.scroll.Y, v.full, v.wrap, v.wrapGen = c.y, c.full, c.wrap, c.wrapGen
			at := min(max(c.at, 0), len(col.tabs))
			a.addTab(col, at, &colTab{diff: v})
			a.activate(col, at, true)
			return
		}
		n := len(col.tabs)
		// A file shown already keeps the preview it has; a fresh open is
		// text first, its saved preview restored below.
		if col.showingFile(c.path) < 0 {
			a.openText(func() { a.openFileIn(col, c.at, c.path, true) })
		} else {
			a.openFileIn(col, c.at, c.path, true)
		}
		if len(col.tabs) == n {
			a.dropColumn(col, made)
			// The column showed it already, or it could not open.
			return
		}
		if ed := col.tab().ed; ed != nil {
			a.setLive(ed, c.live && isMarkdown(ed.document))
			restore := func(ed *editor) {
				ed.sel, ed.caret = c.sel, c.caret
				ed.vscroll.Y, ed.hscroll.X = c.y, c.x
				if ed.hidden {
					ed.hiddenY, ed.hiddenPlaced = c.y, false
				}
				ed.wrap, ed.wrapGen = c.wrap, c.wrapGen
				if !isMarkdown(ed.document) {
					a.setPreview(ed, c.preview)
				}
			}
			if ed.unread() {
				ed.loadedTarget = restore
			} else {
				restore(ed)
			}
			a.focusTab(col.tab())
		}
		return
	}
}

// closedColumn returns the column a closed tab goes back to: its own, or
// one made again in its place when it went with its last tab, which the
// other tabs closed from it go back to too. made reports the latter.
func (a *app) closedColumn(c closedTab) (col *column, made bool) {
	if i := slices.IndexFunc(a.cols, func(k *column) bool { return k.id == c.col }); i >= 0 {
		return a.cols[i], false
	}
	if change := c.panes; change != nil && reflect.DeepEqual(change.after, a.paneSnapshot()) {
		col = a.newColumn()
		col.id = c.col
		a.cols = slices.Insert(a.cols, c.colAt, col)
		if a.col >= c.colAt {
			a.col++
		}
		for index, leaf := range a.cols {
			leaf.weight = change.before.columns[index].weight
		}
		a.flatPaneTree()
		a.restorePaneLayout(change.before.groups, a.cols)
		return col, true
	}
	if c.colAt < 0 || len(a.cols) == 1 && len(a.cols[0].tabs) == 0 {
		// The window's only column is empty: it takes the column's place.
		col = a.column()
	} else {
		col = a.newColumn()
		index := slices.IndexFunc(a.cols, func(other *column) bool { return other.id == c.neighbor })
		if index < 0 {
			index = min(max(c.colAt-1, 0), len(a.cols)-1)
			c.direction = splitRight
			if c.colAt == 0 {
				c.direction = splitLeft
			}
		}
		col.id = c.col
		a.insertPane(&a.cols[index].paneTile, col, c.direction, c.share)
		made = true
	}
	for i := range a.closed {
		if a.closed[i].col == c.col {
			a.closed[i].col = col.id
		}
		if a.closed[i].neighbor == c.col {
			a.closed[i].neighbor = col.id
		}
	}
	return col, made
}

// dropColumn takes away a column made again for a tab that did not open.
func (a *app) dropColumn(col *column, made bool) {
	if made && len(col.tabs) == 0 {
		a.removeColumn(col)
	}
}

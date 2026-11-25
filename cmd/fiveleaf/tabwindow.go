package main

import (
	"image"
	"slices"
	"strings"
	"sync"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo"
	mtransfer "github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

var tabDragData = mtransfer.New(mtransfer.NewItem(mtransfer.Bytes("application/x-fiveleaf-tab", nil)))

type tabHome struct {
	window *settings.Window
	column int
	index  int
}

func (a *app) dragTab(tab ui.Element, t *colTab) {
	if a.window.Detached {
		tab.Drag(dragItem{tab: t, detached: a})
		if a.host.nativeDrags() {
			tab.DragData(tabDragData)
		}
		return
	}
	tab.Drag(dragItem{tab: t})
	if a.host.nativeDrags() && a.canDetachTab(t) {
		tab.DragData(tabDragData, mtransfer.DragOptions{Done: func(result mtransfer.Result) { a.tabDragDone(t, result) }})
	}
}

func (a *app) tabDragDone(t *colTab, result mtransfer.Result) {
	if result.Operation == mtransfer.None && result.Canceled && !result.Escaped && result.Err == nil {
		a.post(func() { a.detachTab(t, &result.Point) })
	}
}

func screenRect(r mygo.Rectangle) image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.Width, r.Y+r.Height)
}

func (h *desktop) newTabWindow(drop *image.Point) *app {
	if h.f == nil || h.f.life != lifeOpen || h.m.life != lifeOpen || h.f.win.IsDestroyed() {
		return nil
	}
	if drop != nil && drop.In(screenRect(h.f.win.Bounds())) {
		return nil
	}
	w := h.m.s.NewWindow()
	w.Folder, w.Detached = h.a.folder(), true
	dest := h.m.openWindow(w)
	win := dest.f.win
	if win.IsDestroyed() {
		return nil
	}
	if drop != nil {
		area := screenRect(mygo.Screen.DisplayNearestPoint(mygo.Point{X: drop.X, Y: drop.Y}).WorkArea)
		width, height := win.Size()
		x := max(area.Min.X, min(drop.X-width/2, area.Max.X-width))
		y := max(area.Min.Y, min(drop.Y-titleBarH/2, area.Max.Y-height))
		win.SetPosition(x, y)
	}
	if !dest.a.started {
		dest.a.started = true
		dest.a.start()
	}
	dest.show()
	return dest.a
}

func (h *desktop) inWorkspace(fn func(*app)) {
	m, dir := h.m, h.a.folder()
	if m.life != lifeOpen || h.f.life != lifeOpen {
		return
	}
	target := m.of(h.a.tabHome.window)
	if target == nil || target.a.window.Detached || target.a.folder() != dir {
		target = m.showing(dir)
	}
	if target == nil && dir != "" {
		m.openFolder(dir, true)
		target = m.showing(dir)
	}
	if target == nil {
		target = m.openWindow(m.s.NewWindow())
	}
	if target.f.life != lifeOpen {
		return
	}
	target.do(func() {
		if target.f.life == lifeOpen && h.f.life == lifeOpen && !target.a.loadsClosed && !h.a.loadsClosed {
			fn(target.a)
		}
	})
	target.show()
}

func (a *app) outsideTab(path string) bool {
	if !a.window.Detached {
		return false
	}
	t := a.activeTab()
	return t != nil && (t.ed == nil && t.view == nil || t.path() != path)
}

func (a *app) showsDetachedFile(path string) bool {
	if !a.window.Detached || a.loadsClosed || path == "" {
		return false
	}
	t := a.activeTab()
	return t != nil && (t.ed != nil || t.view != nil) && t.path() == path
}

func (a *app) acceptsReturn(it dragItem) bool {
	return it.tab != nil && it.detached != nil && it.detached.window.Detached && it.detached.folder() == a.folder() &&
		!a.window.Detached && !a.loadsClosed && !it.detached.loadsClosed && it.detached.activeTab() == it.tab
}

func (a *app) tabWindowTitleBar(c *ui.Context, t *colTab, update updateStatus) {
	p, bar := a.pal, c.TitleBar()
	m := p.metricsOr()
	actionsWidth := m.control
	chatTab := t != nil && t.tool != nil && t.tool.chat != nil
	if chatTab && a.hasChatProject() {
		actionsWidth += m.control
	}
	leadingWidth := bar.Left
	if update.phase != updateDisabled {
		leadingWidth += m.control
	}
	inset := max(leadingWidth, bar.Right+actionsWidth) + m.inset
	row := ui.Row(c.Key("document-title")).Height(max(bar.Height, titleBarH)).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).
		Padding(0, inset).Background(windowBackground(c, a.settings.WindowOpacity, p.titlebar)).DragWindow()
	row.ContextMenu(a.tabWindowMenu).Children(func() {
		if update.phase != updateDisabled {
			ui.Row(c.Key("update-control")).Absolute().Top(0).Bottom(0).Left(bar.Left + m.inset).AlignItems(ui.Center).
				Children(func() { a.updateTitleButton(c, update) })
		}
		if t == nil {
			return
		}
		name := a.windowTitle()
		title := ui.Row(c.Key("name")).Height(tabHeight).MaxWidthPercent(100).MinWidth(0).Padding(0, m.gap).Gap(m.gap).AlignItems(ui.Center)
		available := a.canDetachTab(t)
		tip := ""
		if available {
			tip = locales.ReturnTabHint()
			a.dragTab(title, t)
		}
		title.Children(func() {
			if chatTab {
				a.chatTabIcon(c, t.tool.chat, title, tip, p.titlebar)
			} else if available {
				title.Tooltip(tip)
			}
			ui.Text(c, name).SingleLine().MinWidth(0).FontSize(m.textBody).TextColor(p.text)
			if chatTab {
				a.chatTabStatus(c, t.tool.chat, p.titlebar)
			}
		})
		ui.Row(c.Key("return")).Absolute().Top(0).Bottom(0).Right(bar.Right + m.inset).AlignItems(ui.Center).Children(func() {
			a.chatTabActions(c, t)
			button := a.iconButtonBase(c.Key("return-tab"), iconImport, locales.MoveToMainWindow(), false).Disabled(!available).Tooltip(locales.ReturnTabHint())
			if available {
				a.dragTab(button, t)
			}
			if button.Clicked() {
				a.post(a.returnTab)
			}
		})
	})
}

func (a *app) tabWindowMenu(m *ui.Menu) {
	if m.Item(locales.MoveToMainWindow()).Disabled(!a.canDetachTab(a.activeTab())).Chosen() {
		a.post(a.returnTab)
	}
}

func tabWindowCommand(id string) bool {
	group, _, _ := strings.Cut(id, ".")
	switch group {
	case "sel", "editor", "markdown", "image", "mermaid":
		return true
	case "edit":
		return id != "edit.findInFiles" && id != "edit.replaceInFiles"
	}
	switch id {
	case "file.new", "file.newWindow", "file.openFolder", "file.save", "file.saveAs", "file.close", "file.settings",
		"file.copyPath", "file.copyRelativePath", "file.reveal", "go.line", "go.bracket", "go.hover", "go.definition",
		"view.wordWrapTab", "view.togglePreview", "view.toggleLivePreview", "view.zoomIn", "view.zoomOut", "view.zoomReset":
		return true
	}
	return false
}

func (a *app) canDetachTab(t *colTab) bool {
	if t == nil {
		return false
	}
	var doc *document
	switch {
	case t.tool != nil && (t.tool.meeting != nil || t.tool.toolbox != nil):
		return false
	case t.ed != nil:
		doc = t.ed.document
	case t.merge != nil:
		doc = t.merge.doc
	case t.diff != nil:
		return t.diff.canDetach()
	case t.tool != nil && t.tool.chat != nil:
		return len(a.agentWrites) == 0
	}
	return doc == nil || doc.sync == syncIdle || doc.sync == syncLoadFailed || doc.sync == syncJSONLPreview
}

func (a *app) detachTab(t *colTab, drop *image.Point) {
	from, index := a.locate(t)
	if a.window.Detached || from == nil || a.loadsClosed || !a.canDetachTab(t) {
		return
	}
	to := a.host.newTabWindow(drop)
	if to == nil {
		return
	}
	if a.transferTab(t, to, to.column(), 0) {
		to.tabHome = tabHome{window: a.window, column: from.id, index: index}
	} else if to.window.Detached && to.activeTab() == nil {
		to.host.closeWindow()
	}
}

func (a *app) returnTab() {
	t := a.activeTab()
	if !a.window.Detached || a.loadsClosed || !a.canDetachTab(t) {
		return
	}
	a.host.inWorkspace(func(to *app) {
		col := to.column()
		at := col.active + 1
		if to.window == a.tabHome.window {
			for _, candidate := range to.cols {
				if candidate.id == a.tabHome.column {
					col, at = candidate, a.tabHome.index
					break
				}
			}
		}
		a.transferTab(t, to, col, at)
	})
}

func (a *app) transferTab(t *colTab, to *app, col *column, at int) bool {
	from, _ := a.locate(t)
	if from == nil || a.loadsClosed || to.loadsClosed || a.window.Detached == to.window.Detached ||
		!slices.Contains(to.cols, col) || !a.canDetachTab(t) {
		return false
	}
	var doc *document
	var replace *colTab
	switch {
	case t.ed != nil:
		doc = t.ed.document
		if index := col.showing(doc); index >= 0 {
			replace = col.tabs[index]
		}
	case t.merge != nil:
		doc = t.merge.doc
	case t.view != nil:
		if index := col.showingImage(t.view.path); index >= 0 {
			replace = col.tabs[index]
		}
	}
	if doc != nil {
		if index := to.docIndex(doc.path); index >= 0 && to.docs[index] != doc {
			to.tell(note{message: locales.ReturnTabConflict(doc.name())})
			return false
		}
	}
	moved := &colTab{}
	currentChat := t.tool != nil && t.tool.chat != nil && a.agent == t.tool.chat
	switch {
	case t.ed != nil:
		moved.ed = to.detachedEditor(a, t.ed)
	case t.view != nil:
		v := *t.view
		v.pane, v.grab = ui.Handle{}, false
		moved.view = &v
		if to.tree != nil {
			to.tree.Watch(v.path)
		}
		if !v.loaded {
			to.loadImage(&v)
		}
	case t.tool != nil && t.tool.chat != nil:
		chat := t.tool.chat
		to.takeChatGroup(a, chat)
		moved.tool = &toolView{kind: toolAgent, chat: chat}
	case t.tool != nil:
		moved.tool = t.tool.clone()
	case t.term != nil:
		s := t.term
		s.leavePanelGroup()
		s.tabEvents(a).move(to)
		a.terms = slices.DeleteFunc(a.terms, func(other *session) bool { return other == s })
		if a.keyTerm == s {
			a.keyTerm = nil
		}
		if s.isShell() {
			at = to.movePanelTerminals(col, at)
		}
		to.nextTerm++
		s.id, s.view, s.findInput = to.nextTerm, ui.Handle{}, ui.Handle{}
		s.tab, moved.term = moved, s
		to.terms = append(to.terms, s)
	case t.merge != nil:
		v := *t.merge
		v.panes = [3]paneScroll{}
		if v.ed != nil {
			v.ed = to.detachedEditor(a, v.ed)
			v.doc = v.ed.document
		}
		moved.merge = &v
	case t.diff != nil:
		moved.diff = t.diff.cloneForPane()
	}
	to.addTab(col, at, moved)
	if replace != nil {
		to.dropTab(replace)
	}
	a.dropTab(t)
	if currentChat {
		if next := a.lastOpenChat(); next != nil {
			a.setCurrentChat(next)
		}
	}
	if t.view != nil && a.tree != nil && !slices.ContainsFunc(a.imageViewers(), func(v *viewer) bool { return v.path == t.view.path }) {
		a.tree.Unwatch(t.view.path)
	}
	a.find.stale = true
	a.rememberTabs()
	_, index := to.locate(moved)
	to.activate(col, index, true)
	if v := moved.merge; v != nil && v.doc == nil && !v.binary && v.err == "" {
		v.loading = false
		to.refreshMerge(v)
	}
	a.invalidate()
	to.invalidate()
	return true
}

func (a *app) detachedEditor(from *app, src *editor) *editor {
	doc := src.document
	doc.observe()
	if len(doc.views) == 1 && doc.shared == nil {
		from.docs = slices.DeleteFunc(from.docs, func(other *document) bool { return other == doc })
		if from.shownDoc == doc {
			from.shownDoc = nil
		}
		if from.tree != nil && doc.path != "" {
			from.tree.Unwatch(doc.path)
		}
		from.moveDocumentWaiters(doc, a)
		doc.hl.Close()
		doc.hl = highlight.New(doc.hl.Lexer(), a.invalidate)
		if doc.autoSave.timer != nil {
			doc.autoSave.timer.Stop()
		}
		doc.autoSave = autoSave{}
		a.registerTabDocument(doc)
	} else {
		a.shareTabDocument(from, doc)
	}
	ed := cloneEditorOf(src, doc)
	ed.owner = a
	if src.hidden {
		ed.vscroll.Y = src.hiddenY
	}
	ed.hidden, ed.hiddenY, ed.resync = true, ed.vscroll.Y, true
	return ed
}

func (a *app) tabDocument(path string) (*app, *document) {
	if path == "" {
		return nil, nil
	}
	if index := a.docIndex(path); index >= 0 {
		return a, a.docs[index]
	}
	for _, other := range a.host.everyProject() {
		if other == a || other.loadsClosed || !other.window.Detached && !a.window.Detached || other.folder() != a.folder() {
			continue
		}
		if index := other.docIndex(path); index >= 0 && len(other.docs[index].views) != 0 {
			return other, other.docs[index]
		}
	}
	return nil, nil
}

func (a *app) shareTabDocument(owner *app, doc *document) {
	if owner == a {
		return
	}
	if doc.shared == nil {
		for _, ed := range doc.views {
			ed.owner = owner
		}
		doc.shared = &sharedTabDocument{}
		doc.shared.windows.Store(owner, true)
		doc.hl.Close()
		doc.hl = highlight.New(doc.hl.Lexer(), doc.shared.invalidate)
	}
	a.registerTabDocument(doc)
	doc.shared.windows.Store(a, true)
}

func (a *app) registerTabDocument(doc *document) {
	if slices.Contains(a.docs, doc) {
		return
	}
	if slices.ContainsFunc(a.docs, func(other *document) bool { return other.id == doc.id }) {
		next := a.nextID + 1
		if doc.shared != nil {
			doc.shared.windows.Range(func(key, _ any) bool { next = max(next, key.(*app).nextID+1); return true })
			doc.shared.windows.Range(func(key, _ any) bool { key.(*app).nextID = next; return true })
		}
		doc.id = next
	}
	a.nextID = max(a.nextID, doc.id)
	a.untitled = max(a.untitled, doc.number)
	a.docs = append(a.docs, doc)
	if a.tree != nil && doc.path != "" {
		a.tree.Watch(doc.path)
	}
}

type sharedTabDocument struct {
	windows sync.Map
	version uint64
	dirty   bool
}

func (shared *sharedTabDocument) invalidate() {
	shared.windows.Range(func(key, _ any) bool {
		key.(*app).invalidate()
		return true
	})
}

func (a *app) moveDocumentWaiters(doc *document, to *app) {
	a.waits = slices.DeleteFunc(a.waits, func(wait docWait) bool {
		if wait.doc != doc {
			return false
		}
		to.waits = append(to.waits, wait)
		return true
	})
}

func (a *app) leaveSharedDocument(doc *document) {
	shared := doc.shared
	shared.windows.Delete(a)
	if len(doc.views) != 0 {
		remaining := doc.views[0].owner
		a.moveDocumentWaiters(doc, remaining)
		if !slices.ContainsFunc(doc.views, func(ed *editor) bool { return ed.owner != remaining }) {
			doc.shared = nil
			doc.hl.Close()
			doc.hl = highlight.New(doc.hl.Lexer(), remaining.invalidate)
		}
	}
	a.docs = slices.DeleteFunc(a.docs, func(other *document) bool { return other == doc })
	if a.tree != nil && doc.path != "" {
		a.tree.Unwatch(doc.path)
	}
	if a.shownDoc == doc {
		a.shownDoc = nil
	}
	if doc.autoSave.timer != nil {
		doc.autoSave.timer.Stop()
	}
	doc.autoSave = autoSave{}
	shared.invalidate()
}

type tabEvents struct {
	mu       sync.Mutex
	window   *app
	queue    []func(*app)
	dispatch func(func())
}

func (e *tabEvents) post(fn func(*app)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.queue = append(e.queue, fn)
	if len(e.queue) == 1 {
		if e.dispatch != nil {
			e.dispatch(func() {
				e.drain()
				e.window.invalidate()
			})
		} else {
			e.window.post(e.drain)
		}
	}
}

func (e *tabEvents) move(to *app) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.window = to
	if len(e.queue) != 0 && e.dispatch == nil {
		to.post(e.drain)
	}
}

func (e *tabEvents) drain() {
	e.mu.Lock()
	queue := e.queue
	e.queue = nil
	e.mu.Unlock()
	for _, fn := range queue {
		fn(e.window)
	}
}

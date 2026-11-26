package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/git"
	"fiveleaf/internal/highlight"
	"fiveleaf/internal/remote"
	"fiveleaf/locales"
)

const (
	titleBarH  = 32
	tabStripH  = 32
	crumbsH    = 24
	statusBarH = 24
	panelHeadH = 34
	// panelTitleW is the least width of a panel at a side that still
	// shows its title, past the room its terminals and tools need.
	panelTitleW = 300
)

// view builds the window: a title bar, the explorer beside the editor
// and the terminal panel below it, and a status bar.
func (a *app) view(c *ui.Context) {
	// The window's, which may be another than the frame before's as the
	// project moves to another window.
	a.setServices(c.Services())
	a.review.ctx = c
	// Restoring a preview uses font metrics to recover its scroll position.
	if p := a.settings.Prefs; !a.font.is(p) {
		a.font = newCodeFont(p)
	}
	if !a.started {
		// Before what was posted meanwhile, as opening the file the app was
		// started with: it opens in the columns of the last session.
		a.started = true
		a.start()
	}
	a.drain()
	update := a.host.update()
	if update.minimum != "" {
		a.updatesOpen = false
	}
	if a.agentMenu.open && (a.blurred || !a.agentMenu.valid(a)) {
		a.agentMenu = agentMenuState{}
	}
	a.pumpChain()
	// The key the keymap took is let go of where its release does not
	// come: in a place with no text, or another window.
	if a.held != nil && (a.blurred || a.keyFocus != placeEditor && a.keyFocus != placeInput) {
		a.held = nil
	}
	// The places that take the keys say so as they are built.
	a.keyFocus = placeNone
	if translucentWindow(c, a.settings.WindowOpacity) {
		defer c.Root().Background(ui.Transparent)
	}
	a.pal = a.applyTheme(c)
	updateLinuxControlsTheme(a.fade.to)
	a.themeTerminals()
	a.fontTerminals()
	a.copyTerminals()
	p := a.pal
	a.builds++
	if now := c.Now(); now != a.frameAt {
		a.frameAt = now
		a.frame++
		a.dragging, a.dragNow = a.dragNow, false
	}
	a.copyDrop = c.Modifiers()&ui.Alt != 0
	if update.minimum == "" {
		a.shortcuts(c)
	}
	c.SetZoom(float32(math.Pow(1.2, float64(a.settings.ZoomLevel))))
	// Which pane has the keyboard focus, as the frame before left it, and
	// which column: its editor is the one typing is of.
	for i, col := range a.cols {
		switch t := col.tab(); {
		case t == nil:
		case t.ed != nil && (t.ed.vscroll.FocusWithin(c) || t.ed.tableCell != nil && t.ed.tableCell.input.Focused(c)):
			a.focus, a.col = paneEditor, i
			t.ed.typist = t.ed
			a.rememberAgentContextSource(t)
		case t.term != nil && (t.term.view.FocusWithin(c) || t.term.findInput.Focused(c)):
			a.focus, a.col = paneEditor, i
		case t.view != nil && t.view.pane.FocusWithin(c):
			a.focus, a.col = paneEditor, i
			a.rememberAgentContextSource(t)
		case t.diff != nil && t.diff.pane.FocusWithin(c):
			a.focus, a.col = paneEditor, i
			a.rememberAgentContextSource(t)
		case t.tool != nil && (t.tool.pane.FocusWithin(c) || t.tool.chatActions.FocusWithin(c)):
			if t.tool.meeting != nil && a.col != i {
				a.showMeetingSidebar()
			}
			a.focus, a.col = paneEditor, i
			if t.tool.chat != nil {
				a.setCurrentChat(t.tool.chat)
			}
		case t.ed != nil && t.ed.preview != nil && t.ed.preview.pane.FocusWithin(c):
			a.focus, a.col = paneEditor, i
			a.rememberAgentContextSource(t)
		}
	}
	a.autoSaveTick()
	switch {
	case a.panelTerm != nil && (a.panelTerm.view.FocusWithin(c) || a.panelTerm.findInput.Focused(c)):
		a.focus = paneTerminal
	case a.files.List.FocusWithin(c):
		a.focus = paneFiles
	}
	if request := a.wordSuggest; request != nil && !request.current(a) {
		a.dismissWordSuggestions()
	}

	back := p.editor
	if p.card > 0 && !a.window.Detached {
		back = p.chrome
	}
	shell := ui.Column(c).Fill().Background(windowContainer(c, a.settings.WindowOpacity, back))
	materialBackdrop(c, shell, p, a.settings.WindowOpacity,
		!a.settings.MicaAnimation || c.Preferences().ReduceMotion).Children(func() {
		if update.minimum != "" {
			a.titleBar(c, update)
			a.requiredUpdateView(c, update)
			return
		}
		if a.window.Detached {
			col := a.column()
			a.findCol = col
			a.tabWindowTitleBar(c, col.tab(), update)
			ui.Box(c.Key("body")).Bind(&col.body).Grow(1).MinHeight(0).Clip().Children(func() { a.tabPage(c, col, col.tab()) })
			return
		}
		a.titleBar(c, update)
		// The panes keep their places in the tree, by keys, as the sidebar
		// and the panel come and go, so that the editors keep their state.
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.rail != nil {
				a.rail(c)
			}
			s, w := a.settings, a.window
			// The panel is at the window's edge, past the sidebar, so
			// that with both on a side the panel is the outer one.
			side := ""
			if w.PanelOpen {
				side = a.panelSide()
			}
			sidebar := func() {
				if w.SidebarClosed {
					a.sidebarRevealed = ""
					return
				}
				// Its splitter is along the edge facing the editors.
				at, dir := edgeRight, float32(1)
				box := ui.Column(c.Key("sidebar")).Width(w.SidebarWidth).Shrink(0).BorderColor(p.seam())
				// The gap around the sidebar's card is the window's own
				// surface, so it meets the title bar and the editors
				// without a seam.
				box.Background(windowBackground(c, a.settings.WindowOpacity, p.sidebarRing()))
				a.grained(box)
				if s.SidebarRight {
					at, dir = edgeLeft, -1
				}
				if p.card > 0 {
					if s.SidebarRight {
						box.BorderWidth(0, 0, 0, 1)
					} else {
						box.BorderWidth(0, 1, 0, 0)
					}
				}
				box.Children(func() {
					a.sidebar(c)
					a.splitter(c, at, func(d float32) {
						width, _ := c.Size()
						w.SidebarWidth = clamp(w.SidebarWidth+dir*d, 160, max(width-320, 160))
					})
				})
			}
			if side == "left" {
				a.sidePanel(c, false)
			}
			if !s.SidebarRight {
				sidebar()
			}
			main := ui.Column(c.Key("main")).MinWidth(0).Grow(1)
			left := a.rail == nil && side != "left" && (w.SidebarClosed || s.SidebarRight)
			right := side != "right" && (w.SidebarClosed || !s.SidebarRight)
			a.asCard(main, left, right)
			main.Children(func() { a.main(c) })
			if s.SidebarRight {
				sidebar()
			}
			if side == "right" {
				a.sidePanel(c, true)
			}
		})
		a.statusBar(c)
	})
	if update.minimum == "" {
		a.quickOpenView(c)
		a.leaderMenuView(c)
		a.branchPickerView(c)
		a.stashPickerView(c)
		a.prPickerView(c)
		a.prChecksView(c)
		a.worktreePickerView(c)
		a.repositoryPickerView(c)
		a.recentPickerView(c)
		a.pluginPickerView(c)
		a.hoverView(c)
		a.themePickerView(c)
		a.iconThemePickerView(c)
		a.remotePickerView(c)
		a.taskPickerView(c)
		a.conflictsView(c)
		a.diagramOverlayView(c)
		a.updateView(c, update)
	}
	a.promptView(c)
	a.confirmView(c)
	if update.minimum == "" {
		a.windowKeys(c)
	}
	inEditor := a.keyFocus == placeEditor
	markdownKeys := a.activeMarkdownKeys()
	inMeeting := a.activeRoundtableTab() != nil
	taskTab := a.activeTab().isTask()
	taskFocus := taskTab && a.focus != paneTerminal
	canReturn := a.window.Detached && a.canDetachTab(a.activeTab())
	if inEditor != a.cutInEditor || markdownKeys != a.menuMarkdown || inMeeting != a.menuInMeeting || canReturn != a.menuCanReturn || taskTab != a.menuTaskTab || taskFocus != a.menuTaskFocus {
		a.menuMarkdown = markdownKeys
		a.cutInEditor, a.menuInMeeting, a.menuCanReturn = inEditor, inMeeting, canReturn
		a.menuTaskTab, a.menuTaskFocus = taskTab, taskFocus
		a.host.menuChanged()
	}
	for _, msg := range a.errors {
		notice := ui.Toast{ID: msg, Title: msg, Type: "error"}
		notice.Data = a.rememberNotice("FiveLeaf", notice)
		c.AddToast(notice)
	}
	a.errors = a.errors[:0]
	if len(a.notes) > 0 {
		// The character holds up its megaphone for a while.
		a.toldAt = c.Now()
	}
	for _, n := range a.notes {
		c.AddToast(n.toast())
	}
	clear(a.notes)
	a.notes = a.notes[:0]
	a.showChatTaskNotices(c, true)
	notificationViewport(c, p, a.statusHeight(), a.noticeHistoryOpen)
	a.pluginTick()
	a.tick()
}

// main is the columns of editors and, below them, the panel when it is
// at the bottom.
func (a *app) main(c *ui.Context) {
	p, w := a.pal, a.window
	panel := w.PanelOpen
	a.remoteBanner(c)
	editor := ui.Column(c.Key("editor")).Grow(1).MinHeight(0).Background(windowContainer(c, a.settings.WindowOpacity, p.editor)).Clip()
	editor.Children(func() { a.columns(c) })
	if !panel || a.panelSide() != "bottom" {
		return
	}
	_, h := c.Size()
	room := h - titleBarH - a.statusHeight()
	height := clamp(w.PanelHeight, 80, max(room-80, 80))
	box := ui.Column(c.Key("panel")).Height(height).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(p.border)
	box.Children(func() {
		a.panel(c)
		a.splitter(c, edgeTop, func(d float32) {
			w.PanelHeight = clamp(height-d, 80, max(room-80, 80))
		})
	})
}

// panelSide is the side the terminal panel is on: "bottom", "left" or
// "right".
func (a *app) panelSide() string {
	switch a.settings.PanelPosition {
	case "left", "right":
		return a.settings.PanelPosition
	}
	return "bottom"
}

// sidePanel is the terminal panel at a side of the window, past the
// sidebar, its splitter along the edge facing the editors.
func (a *app) sidePanel(c *ui.Context, right bool) {
	p, w := a.pal, a.window
	width, _ := c.Size()
	at, dir := edgeRight, float32(1)
	box := ui.Column(c.Key("panel")).Shrink(0).BorderColor(p.border)
	if right {
		at, dir = edgeLeft, -1
		box.BorderWidth(0, 0, 0, 1)
	} else {
		box.BorderWidth(0, 1, 0, 0)
	}
	// It leaves the editors 320 points beside the sidebar and the rail,
	// as the sidebar leaves them, however narrow the window.
	room := width - 320
	if !w.SidebarClosed {
		room -= w.SidebarWidth
	}
	if a.rail != nil {
		room -= railW
	}
	room = max(room, minPanelW)
	size := clamp(w.PanelWidth, minPanelW, room)
	box.Width(size).Children(func() {
		a.panel(c)
		a.splitter(c, at, func(d float32) {
			w.PanelWidth = clamp(size+dir*d, minPanelW, room)
		})
	})
}

// edge is an edge of a pane, which its splitter is along.
type edge int

const (
	edgeRight edge = iota // the sidebar's, on the left
	edgeTop               // the panel's
	edgeLeft              // a column's, after the first, or the sidebar's on the right
)

// splitter is the handle along an edge of a pane that resizes it. drag
// gets how far the pointer moved, across the edge.
func (a *app) splitter(c *ui.Context, at edge, drag func(d float32)) ui.Element {
	h := ui.Box(c).Absolute()
	switch at {
	case edgeTop:
		h.Top(0).Left(0).Right(0).Height(5).Cursor(ui.CursorResizeNS)
	case edgeLeft:
		h.Top(0).Bottom(0).Left(0).Width(5).Cursor(ui.CursorResizeEW)
	default:
		h.Top(0).Bottom(0).Right(0).Width(5).Cursor(ui.CursorResizeEW)
	}
	// The line shows once the pointer rests on the edge, as one crossing
	// it on its way elsewhere does not light it, and fades in and out.
	// Held, it shows at once.
	var lit float32
	if h.Pressed() || h.Dragging() {
		lit = h.Jump("lit", 1)
	} else {
		show := ui.Animation{Duration: motionShow}
		if h.Hovered() {
			show.Delay = 250 * time.Millisecond
		}
		lit = h.AnimateTo("lit", unit(h.Hovered()), show)
	}
	h.Children(func() {
		if lit < 0.01 {
			return
		}
		line := ui.Box(c).Absolute()
		if a.pal.card > 0 {
			// Around a card, it lights around the pointer, as Qoder's do.
			px, py, _ := h.PointerPosition()
			col := a.pal.accent.Alpha(lit)
			line.Draw(func(pt *ui.Painter, r ui.Rect) {
				if at == edgeTop {
					drawSeamLight(pt, r, true, r.X+px, col)
				} else {
					drawSeamLight(pt, r, false, r.Y+py, col)
				}
			})
		} else {
			line.Background(a.pal.accentDim.Alpha(lit))
		}
		switch at {
		case edgeTop:
			line.Top(0).Left(0).Right(0).Height(2)
		case edgeLeft:
			line.Top(0).Bottom(0).Left(0).Width(2)
		default:
			line.Top(0).Bottom(0).Right(0).Width(2)
		}
	})
	dx, dy, held := h.Dragged()
	if held && (dx != 0 || dy != 0) {
		if at == edgeTop {
			drag(dy)
		} else {
			drag(dx)
		}
		a.settings.Changed()
	}
	return h
}

func (a *app) titleBar(c *ui.Context, update updateStatus) {
	p := a.pal
	m := p.metricsOr()
	bar := c.TitleBar()
	left, right := bar.Left+m.inset, bar.Right+m.inset
	row := ui.Row(c).Height(max(bar.Height, titleBarH)).Padding(0, right, 0, left).AlignItems(ui.Center).
		Background(windowBackground(c, a.settings.WindowOpacity, p.titlebar)).BorderWidth(0, 0, 1, 0).BorderColor(ui.Transparent).DragWindow()
	a.grained(row)
	row.Children(func() {
		minWidth := a.titleBarProjectWidth()
		if update.phase != updateDisabled {
			minWidth += m.control + 2
		}
		ui.Row(c).Basis(0).Grow(1).MinWidth(minWidth).Gap(2).AlignItems(ui.Center).Children(func() {
			if hostOS != "darwin" {
				menu := a.iconButton(c, iconMenu, locales.Menu(), false).Disabled(update.minimum != "")
				if update.minimum == "" {
					menu.Menu(a.appUIMenu)
				}
			}
			a.updateTitleButton(c, update)
			if update.minimum == "" {
				a.titleBarProjects(c)
			}
		})
		if update.minimum == "" {
			a.iconButton(c, iconGear, locales.PrefsTitle(), false).Shrink(0).Menu(a.quickSettingsMenu)
		}
	})
}

// iconButton is a small square button showing an icon, its tooltip the
// label.
func (a *app) iconButton(c *ui.Context, icon *ui.SVG, label string, on bool) ui.Element {
	return a.iconButtonBase(c, icon, label, on).Tooltip(label)
}

func (a *app) iconButtonBase(c *ui.Context, icon *ui.SVG, label string, on bool) ui.Element {
	p := a.pal
	m := p.metricsOr()
	b := ui.ButtonBase(c).Size(m.control, 24).Radius(m.radius).Justify(ui.Center).AlignItems(ui.Center).Label(label)
	col := p.muted
	switch {
	case b.Pressed():
		b.Background(p.hover)
		col = p.text
	case b.Hovered():
		b.Background(p.hover)
		col = p.text
	}
	if on {
		col = p.text
	}
	b.Transition(revealFade)
	pressPose(b)
	b.Children(func() { ui.Icon(c, icon).FontSize(15).TextColor(col) })
	return b
}

// minColumnW is the least width a column is dragged to.
const minColumnW = 160

// minPanelW is the least width the panel is dragged to at a side.
const minPanelW = 160

// Keep the pane elements under one keyed parent even as their split groups change.
func (a *app) columns(c *ui.Context) {
	a.findCol = a.column()
	ui.Row(c.Key("columns")).Bind(&a.colsView).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		a.drawPaneGroup(c, &a.panes, ui.Rect{W: 100, H: 100}, a.colsView.Bounds(c))
	})
}

// columnView is a column: its tab strip, the breadcrumbs, and the editor
// or the terminal of the tab shown, or the welcome page.
func (a *app) columnView(c *ui.Context, col *column) {
	m := a.pal.metricsOr()
	if len(col.tabs) == 0 {
		if col.locked {
			a.tabStrip(c, col)
		}
		body := ui.Column(c).Bind(&col.body).Grow(1).MinHeight(0)
		over := a.takeFiles(body, col)
		returned, returning := ui.DragOver[dragItem](body)
		if it, ok := ui.Drop[dragItem](body); ok {
			a.post(func() { a.dropInColumn(it, col, 0) })
		}
		body.Children(func() {
			if col.locked {
				ui.Column(c).Fill().Center().Gap(12).Background(windowBackground(c, a.settings.WindowOpacity, a.pal.editor)).Children(func() {
					ui.Text(c, locales.LockedColumn()).FontSize(m.textBody + 1).TextColor(a.pal.muted)
					ui.Column(c).Width(180).MaxWidthPercent(100).Children(func() {
						a.welcomeAction(c, iconPlus, locales.CmdViewNewTerminal(), "", func() {
							a.newTerminalTab(col, a.workDir(), true)
						})
					})
				})
			} else {
				a.welcome(c)
			}
			a.dropZones(c, col, over)
			if returning && a.acceptsReturn(returned) {
				a.dropMark(c)
			}
		})
		return
	}
	a.tabStrip(c, col)
	shown := col.tab()
	if shown != nil && shown.term == nil && shown.tool == nil {
		a.breadcrumbs(c, col, shown)
	}
	for _, t := range col.tabs {
		if t != shown && t.ed != nil {
			a.hideEditor(t.ed)
		}
	}
	body := ui.Box(c.Key("body")).Bind(&col.body).Grow(1)
	over := a.takeFiles(body, col)
	body.Children(func() {
		a.tabPage(c, col, shown)
		a.dropZones(c, col, over)
		a.keepEditors(c, col, shown)
	})
}

func (a *app) tabPage(c *ui.Context, col *column, shown *colTab) {
	if shown == nil {
		return
	}
	page := ui.Column(c.Key(shown.id)).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	if ed := shown.ed; ed != nil {
		a.modeFade(c, page, ed)
	}
	page.Children(func() {
		switch {
		case shown.term != nil:
			a.terminalView(c, shown.term)
		case shown.view != nil:
			a.viewerView(c, shown.view, shown.name())
		case shown.diff != nil:
			a.diffViewUI(c, shown.diff)
			if col == a.findCol {
				a.diffFindView(c, shown.diff)
			} else {
				shown.diff.find.on = false
			}
		case shown.merge != nil:
			a.mergeViewUI(c, shown.merge)
		case shown.tool != nil:
			a.toolViewUI(c, shown.tool)
		case shown.ed.unread():
			a.hideEditor(shown.ed)
			if shown.ed.sync == syncJSONLPreview {
				a.jsonlPreviewView(c, shown.ed)
			} else {
				a.loadingView(c, shown.ed)
			}
		case shown.ed.preview != nil:
			a.hideEditor(shown.ed)
			v := shown.ed.preview
			if col == a.findCol && v.markdown != nil && a.find.open {
				a.findPreview = v.markdown
				a.searchPreview(v.markdown)
			}
			a.viewerView(c, v, shown.name())
			if col == a.findCol && v.markdown != nil {
				a.previewFindView(c, v)
			}
		default:
			if a.editorReviewView(c, shown.ed) {
				break
			}
			shown.ed.areaKept = shown.ed.areaShown(a, col)
			if ed := shown.ed; !ed.areaKept && ed.areaCol != nil && ed.areaCol != col {
				// Moved to another column, it gets a new text area: it
				// comes back to its place as a tab shown again does.
				a.hideEditor(ed)
			}
			shown.ed.areaCol, shown.ed.areaBuild, shown.ed.seenBuild = col, a.builds, a.builds
			a.editorView(c, shown.ed)
			a.inlineCompletionControls(c, shown.ed)
			a.wordSuggestionsView(c, shown.ed)
			a.hunkPeekView(c, shown.ed)
			a.reviewBoxView(c, shown.ed)
			a.agentContextView(c, shown.ed)
			if col == a.findCol {
				a.findView(c, shown.ed)
			}
			a.vimLineView(c, shown.ed)
			a.jumpHintQueryView(c, shown.ed)
		}
	})
}

// keepTabs is how many of a column's tabs out of sight keep their text
// areas: those shown last. The others come back with a new one.
const keepTabs = 8

// keepEditors builds the text areas of a column's tabs that showed and are
// out of sight now, hidden, where they show: they stay as they were, the
// heights of their rows and their place among them, for when the tab comes
// back, rather than start anew from estimates and scroll to their caret.
// Only the keepTabs shown last do, as an area keeps the layout of its text.
func (a *app) keepEditors(c *ui.Context, col *column, shown *colTab) {
	kept := a.keptTabs[:0]
	for _, t := range col.tabs {
		if d := t.ed; t != shown && d != nil && d.areaShown(a, col) {
			kept = append(kept, t)
		}
	}
	if len(kept) > keepTabs {
		slices.SortStableFunc(kept, func(x, y *colTab) int { return y.ed.seenBuild - x.ed.seenBuild })
		kept = kept[:keepTabs]
	}
	for _, t := range kept {
		d := t.ed
		d.areaCol, d.areaBuild = col, a.builds
		ui.Column(c.Key(t.id)).Hidden(true).Children(func() {
			ui.ScrollHorizontal(c).Children(func() {
				ui.TextAreaBufferBase(c, d.buf)
			})
		})
	}
	clear(kept)
	a.keptTabs = kept[:0]
}

func (a *app) hideEditor(d *editor) {
	if !d.hidden {
		d.liveViewport = liveViewport{}
		d.hidden, d.hiddenY = true, d.vscroll.Y
		d.caretPaint.reset()
		d.jumpHints.clear()
		if d.areaCol == nil {
			return // never shown: no area laid its lines out
		}
		snap := d.buf.Snapshot()
		g := a.geometry(d, snap.LineCount())
		top := g.at(d.vscroll.Y - editorPadY)
		d.hiddenAt, _ = snap.LineRange(top)
		d.hiddenOff, d.hiddenPlaced = editorPadY+g.top(top)-d.vscroll.Y, true
	}
}

// dragItem is what is dragged to a column: one of its tabs, or of
// another's, a terminal of the panel, or a file of the sidebar.
type dragItem struct {
	tab      *colTab
	term     *session
	file     *fileDrag
	detached *app
}

// owns reports whether what was dragged is this window's to take: a tab or
// a terminal dragged from another window stays there, as its view binds
// controls of that window, or a file, which opens in any.
func (a *app) owns(it dragItem) bool {
	switch {
	case it.file != nil:
		return true
	case it.term != nil:
		return slices.Contains(a.terms, it.term)
	case it.tab != nil:
		col, _ := a.locate(it.tab)
		return col != nil
	}
	return false
}

// dropInColumn puts what was dragged at place at of a column. A tab of
// another project opens its file there.
func (a *app) dropInColumn(it dragItem, col *column, at int) {
	if slices.Index(a.cols, col) < 0 {
		return
	}
	if it.detached != nil {
		if a.acceptsReturn(it) && it.detached.transferTab(it.tab, a, col, at) {
			a.host.showApp(a)
		}
		return
	}
	if from := a.carrier(it); from != nil {
		a.takeTab(from, it.tab, col, at, a.copyDrop)
		return
	}
	if !a.owns(it) {
		return
	}
	switch {
	case it.file != nil:
		a.dropFile(it.file, col, at)
	case it.term != nil:
		a.terminalToColumn(it.term, col, at)
	case it.tab != nil:
		a.moveTab(it.tab, col, at)
	}
}

// dropBeside puts what was dragged in a column of its own, before a
// column or after it.
func (a *app) dropBeside(it dragItem, col *column, after bool) {
	if slices.Index(a.cols, col) < 0 {
		return
	}
	if from := a.carrier(it); from != nil {
		nc := a.splitColumn(col, after)
		a.takeTab(from, it.tab, nc, 0, a.copyDrop)
		if len(nc.tabs) == 0 {
			// It could not be opened.
			a.removeColumn(nc)
		}
		return
	}
	if !a.owns(it) {
		return
	}
	if it.file != nil {
		a.dropFileBeside(it.file, col, after)
		return
	}
	if from, _ := a.locate(it.tab); from == col && len(col.tabs) == 1 {
		return // it is the column
	}
	a.dropInColumn(it, a.splitColumn(col, after), 0)
}

// takeFiles makes a column's body take files dragged from another app,
// which open in it, and reports whether they are over it.
func (a *app) takeFiles(body ui.Element, col *column) bool {
	if paths := body.DroppedFiles(); paths != nil {
		paths = slices.Clone(paths)
		a.post(func() { a.dropOnColumn(paths, col) })
	}
	return body.FileDragOver()
}

// dropZones are where a tab or files dragged over a column are dropped:
// on a side, for a column of their own there, or between them, in this
// one. They are there only while something is dragged over the editor,
// a tab of another project too; over says files are over the column's
// body.
func (a *app) dropZones(c *ui.Context, col *column, over bool) {
	files := over || col.filesOver
	if !a.dragging && a.carried == nil && !files {
		return
	}
	w := col.body.Bounds(c).W
	side := min(w/4, 140)
	var bottom float32
	if tab := col.tab(); tab != nil && tab.tool != nil && tab.tool.chat != nil {
		if composer := tab.tool.chat.composeBox.Bounds(c); composer.H > 0 {
			body := col.body.Bounds(c)
			bottom = max(0, body.Y+body.H-composer.Y)
		}
	}
	zone := func(key string, left, width, markLeft, markW float32, drop func(it dragItem), dropFiles func([]string)) {
		z := ui.Box(c.Key(key)).Absolute().Top(0).Bottom(bottom).Left(left).Width(width)
		_, ok := a.dragOver(z)
		if files && z.FileDragOver() {
			ok, over = true, true
		}
		if ok {
			ui.Box(c.Key(key+"-mark")).Absolute().Top(0).Bottom(bottom).Left(markLeft).Width(markW).PassThrough().
				Background(a.pal.accent.Alpha(0.14)).Border(1, a.pal.accentDim)
		}
		if it, ok := ui.Drop[dragItem](z); ok {
			a.post(func() { drop(it) })
		}
		if !files {
			return
		}
		if paths := z.DroppedFiles(); paths != nil {
			paths = slices.Clone(paths)
			a.post(func() { dropFiles(paths) })
		}
	}
	zone("drop-before", 0, side, 0, w/2, func(it dragItem) { a.dropBeside(it, col, false) },
		func(paths []string) { a.dropFilesBeside(paths, col, false) })
	zone("drop-in", side, w-2*side, 0, w, func(it dragItem) {
		if it.file != nil {
			// As files from another app: after the tab shown.
			a.dropFileOnColumn(it.file, col)
			return
		}
		a.dropInColumn(it, col, len(col.tabs))
	},
		func(paths []string) { a.dropOnColumn(paths, col) })
	zone("drop-after", w-side, side, w/2, w/2, func(it dragItem) { a.dropBeside(it, col, true) },
		func(paths []string) { a.dropFilesBeside(paths, col, true) })
	if col.filesOver != over {
		col.filesOver = over
		if !over {
			// One more frame, without the zones.
			a.invalidate()
		}
	}
}

// dropMark tints a pane that takes what is dragged over it.
func (a *app) dropMark(c *ui.Context) {
	ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).Right(0).PassThrough().
		Background(a.pal.accent.Alpha(0.14)).Border(1, a.pal.accentDim)
}

// breadcrumbs name the file a tab of column col shows and, in an editor,
// the blocks its caret is in. A folder of the file's path shows its files
// in a box, as clicked. The file opens its symbols on a left click and
// copies its path on a right click. An SVG can show as a picture or as text.
func (a *app) breadcrumbs(c *ui.Context, col *column, t *colTab) {
	p := a.pal
	m := p.metricsOr()
	d := t.ed
	var parts []crumb
	if path := t.path(); path != "" {
		parts = crumbsOf(path, a.root, homeDir())
	} else if t.diff != nil && t.diff.commit != nil && t.diff.commit.pr != nil {
		parts = []crumb{{locales.PullRequest(), ""}, {t.name(), ""}}
	} else if t.diff != nil && t.diff.commit != nil && t.diff.commit.from != "" {
		parts = []crumb{{locales.Comparison(), ""}, {t.name(), ""}}
	} else if t.diff != nil && t.diff.commit != nil {
		parts = []crumb{{locales.DiffCommitName(git.ShortHash(t.diff.commit.hash)), ""}, {t.name(), ""}}
	} else {
		parts = []crumb{{t.name(), ""}}
	}
	if k := &a.crumbs; k.open && k.col == col && !a.crumbsValid() {
		a.closeCrumbs()
	}
	// The blocks the caret is in, after the file: the innermost two.
	var snap ui.TextSnapshot
	var scope []int
	if d != nil && d.preview == nil && isCode(d.document) {
		snap = d.buf.Snapshot()
		scope = d.scopesOf(snap, snap.LineAt(min(d.sel[1], snap.Len())))
		scope = scope[max(0, len(scope)-2):]
	}
	bar := ui.Row(c).Height(crumbsH).Padding(0, m.inset).Gap(m.gap).AlignItems(ui.Center).Background(windowBackground(c, a.settings.WindowOpacity, p.editor)).Clip()
	compactDiff := false
	if t.diff != nil {
		width := bar.Bounds().W
		compactDiff = width < 340 || width < 520 && (t.diff.commit != nil || len(a.review.notes) > 0)
		if compactDiff {
			parts = parts[len(parts)-1:]
		}
	}
	bar.Children(func() {
		for i, part := range parts {
			last := i == len(parts)-1
			if last && (d != nil || t.path() != "") {
				a.crumbFile(c, col, t, part.name)
				if len(scope) > 0 {
					ui.Icon(c, iconChevronRight).FontSize(11).TextColor(p.faint)
				}
				continue
			}
			if last {
				icon, col := a.fileIcon(part.name)
				switch {
				case t.diff != nil && t.diff.commit != nil && t.diff.commit.pr != nil:
					icon, col = iconPullRequest, p.accent
				case t.diff != nil && t.diff.commit != nil:
					icon, col = iconCommit, p.accent
				case t.diff != nil:
					icon, col = iconDiff, p.changed
				}
				iconThemeView(c, p, icon, col, 13)
			}
			if part.dir != "" && t.diff == nil {
				// A folder: its files in a box below it.
				k := &a.crumbs
				on := k.open && k.tab == t && k.dir == part.dir
				b := ui.ButtonBase(c.Key("crumb:"+part.dir)).Height(crumbsH-6).Padding(0, 4).Margin(0, -4).Radius(m.radius).
					AlignItems(ui.Center).Shrink(0).Label(locales.CrumbsFolderTip(part.name))
				if on || b.Hovered() {
					b.Background(p.hover)
				}
				b.Transition(hoverFade)
				b.Children(func() {
					col := p.muted
					if on || b.Hovered() {
						col = p.text
					}
					ui.Text(c, part.name).FontSize(m.textBody).SingleLine().TextColor(col)
				})
				if b.Clicked() {
					a.openCrumbs(col, t, part.dir)
				}
				if on {
					a.crumbsBox(c, b)
				}
			} else {
				t := ui.Text(c, part.name).FontSize(m.textBody).SingleLine().TextColor(p.muted).Shrink(0)
				if last {
					// A long name, as a commit's subject, ends in an ellipsis.
					t.TextColor(p.text).Shrink(1).MinWidth(24)
				}
			}
			if !last || len(scope) > 0 {
				ui.Icon(c, iconChevronRight).FontSize(11).TextColor(p.faint)
			}
		}
		for i, line := range scope {
			label := scopeLabel(snap, line)
			b := ui.ButtonBase(c.Key(fmt.Sprintf("scope:%d", i))).Height(crumbsH-6).Padding(0, 5).Radius(m.radius).
				AlignItems(ui.Center).Shrink(1).MinWidth(0).Label(locales.GoToScope(label))
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Transition(hoverFade)
			b.Children(func() {
				ui.Text(c, label).FontSize(m.textBody).Font(a.font.family).SingleLine().TextColor(p.muted)
			})
			if b.Clicked() {
				a.queueEdit(d, func(d *editor) { a.jumpToScope(d, line) })
			}
			if i < len(scope)-1 {
				ui.Icon(c, iconChevronRight).FontSize(11).TextColor(p.faint)
			}
		}
		if d != nil && (canPreview(d.document) || a.settings.EditorAgentContext) {
			ui.Spacer(c)
			if a.settings.EditorAgentContext {
				a.reviewSendButton(c)
				if len(a.review.notes) > 0 {
					b := ui.ButtonBase(c.Key("agent-context-list")).Size(26, crumbsH-6).Radius(4).
						AlignItems(ui.Center).Justify(ui.Center).Label(locales.AgentContextList()).Tooltip(locales.AgentContextList())
					b.Children(func() { ui.Icon(c, iconMore).FontSize(14).TextColor(p.muted) })
					b.Menu(a.reviewMenu)
				}
			}
			if cell := d.tableCell; cell != nil && a.activeEditor() == d && a.writingMarkdown(d) {
				a.tableActionsButton(c, d, cell)
			}
			if canPreview(d.document) {
				a.previewButton(c, t, d)
			}
		}
		if t.diff != nil {
			ui.Spacer(c)
			a.diffButtons(c, t.diff, compactDiff)
		}
	})
}

// previewButton switches between a document's preview and source.
func (a *app) previewButton(c *ui.Context, t *colTab, d *editor) {
	p := a.pal
	isMD := isMarkdown(d.document)
	command, label, icon := "view.togglePreview", locales.ShowPreview(), iconImage
	if isMD {
		command, icon = "view.toggleLivePreview", iconFileText
	}
	showingPreview := d.preview != nil || a.livePreview(d)
	if showingPreview {
		label, icon = locales.ShowSource(), iconFileCode
	}
	label = a.withKey(label, command)
	b := ui.ButtonBase(c.Key("preview")).Height(crumbsH-4).Padding(0, 6).Radius(4).Shrink(0).
		AlignItems(ui.Center).Label(label).Tooltip(label)
	var caption string
	textSize := p.metricsOr().textBody
	if isMD {
		preview, source := locales.PreviewWord(), locales.MarkdownModeSource()
		previewWidth, _ := c.MeasureText(0, ui.Span{Text: preview, Size: textSize})
		sourceWidth, _ := c.MeasureText(0, ui.Span{Text: source, Size: textSize})
		b.Width(12 + 13 + 4 + max(previewWidth, sourceWidth)).Gap(4).Justify(ui.Center)
		caption = preview
		if showingPreview {
			caption = source
		}
	}
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Transition(hoverFade)
	pressPose(b)
	// The icon turns into the other's, the preview's or the source's.
	b.Children(func() {
		ui.Icon(c, icon).FontSize(13).TextColor(p.muted).ContentTransition(ui.ContentTransition{})
		if caption != "" {
			ui.Text(c, caption).FontSize(textSize).TextColor(p.muted).SingleLine()
		}
	})
	if b.Clicked() {
		a.post(func() {
			if col, index := a.locate(t); col != nil && col.tab() == t && t.ed == d {
				a.activate(col, index, false)
				a.togglePreview()
			}
		})
	}
}

// statusVisibility is which of the parts of the status bar that can go
// show.
type statusVisibility struct {
	prDetail bool // the checks and the review after the number
	terminal bool
	lines    bool
	indent   bool
	encoding bool
	eol      bool
	language bool
}

// statusFit returns which parts of the status bar show in width: as the
// window narrows, the least needed go first, each only once the bar
// would not hold it. The position is measured at its widest, so that the
// parts stay as the caret moves.
func (a *app) statusFit(c *ui.Context, width float32) statusVisibility {
	measure := func(s string, icon bool) float32 {
		w, _ := c.MeasureText(0, ui.Span{Text: s, Size: 11.5, Features: "tnum"})
		w += 2*6 + 2 // the item's padding, and the gap after it
		if icon {
			w += 12 + 5
		}
		return w
	}
	var v statusVisibility
	used := float32(2*6) + 3*(a.pal.metricsOr().control+2)
	if a.lspMenu.open || len(a.lspStatuses()) > 0 {
		used += a.pal.metricsOr().control + 2 // the language server menu
	}
	if a.debug.active() {
		used += measure(a.debug.statusText(), true)
	}
	if a.settings.SidebarRight != a.settings.TerminalButtonRight {
		used += 2
	}
	type part struct {
		on *bool
		w  float32
	}
	// In the order they go.
	var parts []part
	add := func(on *bool, w float32) { parts = append(parts, part{on, w}) }
	if m := a.themeMascot(); m != nil {
		w, _ := m.size(1)
		used += w + 12 + 2
	}
	if busy := a.repository().git.label(); busy != "" {
		used += measure(busy, true)
	}
	if a.conn != nil {
		used += measure("SSH: "+a.conn.Host, true) + 2
		if l := portsLabel(a.conn.Forwards()); l != "" {
			used += measure(l, true)
		}
	}
	if x := a.transferText(); x != "" {
		used += measure(x, true)
	}
	if k := len(a.review.notes); k > 0 {
		used += measure(fmt.Sprint(k), true)
	}
	if len(a.chord) > 0 {
		used += measure(locales.ChordWaiting(strings.Join(chordCaps(a.chord), " ")), true)
	}
	t := a.activeTab()
	switch d := a.activeEditor(); {
	case a.activeViewer() != nil, t != nil && t.diff != nil, d == nil:
	case t != nil && t.term != nil:
		dir, size := t.term.term.Dir(), ""
		if cols, rows := t.term.term.Size(); cols > 0 {
			size = fmt.Sprintf("%d × %d", cols, rows)
		}
		if dir != "" || size != "" {
			add(&v.terminal, measure(shortenHome(dir), true)+measure(size, false))
		}
	default:
		snap := d.buf.Snapshot()
		n := snap.LineCount()
		used += measure(locales.StatusPos(n, 999), false)
		if editorMode(a.prefs()) == "vim" {
			// The mode's badge, and the keys typed after it.
			used += measure(d.vim.mode.String(), false) + 2*6
			if k := d.vim.pending(); k != "" {
				used += measure(k, false)
			}
			if a.vimRec != 0 {
				used += measure(locales.VimRecording(string(a.vimRec)), false) + 6 + 5
			}
			if m := a.vimMsg; m.text != "" && time.Since(m.at) < vimMessageFor {
				used += min(measure(m.text, false), 420) + 6
			}
		} else if a.helixOn() {
			used += measure(d.helix.mode.String(), false) + 2*6
			if k := string(d.helix.keys); k != "" {
				used += measure(k, false)
			}
		}
		if d.diskGone {
			used += measure(locales.DeletedOnDisk(), true)
		}
		eol := "LF"
		if d.crlf {
			eol = "CRLF"
		}
		add(&v.encoding, measure("UTF-8", false))
		add(&v.lines, measure(locales.LinesCount(n), false))
		add(&v.eol, measure(eol, false))
		add(&v.indent, measure(d.indent.String(), false))
		add(&v.language, measure(highlight.DisplayName(d.hl.Lexer()), false))
	}
	if pr := a.repository().currentPR(); pr != nil {
		// The pull request number stays when its details no longer fit.
		number := measure("#"+strconv.Itoa(pr.Number), false)
		used += number
		add(&v.prDetail, measure(prBadgeLabel(pr), false)-number+4*4)
	}
	for _, p := range parts {
		*p.on = true
		used += p.w
	}
	for _, p := range parts {
		if used <= width {
			break
		}
		*p.on = false
		used -= p.w
	}
	return v
}

// vimBadge shows vim mode's mode in the status bar, as a badge colored
// by it: plain in normal mode, the accent in insert mode, and the color
// of a change in visual mode; then the keys of a command typed so far.
func (a *app) vimBadge(c *ui.Context, v *vimState, text func(string) ui.Element) {
	p := a.pal
	m := p.metricsOr()
	bg, fg := p.text.Alpha(0.09), p.statusFg
	switch {
	case v.mode == vimInsert:
		bg, fg = p.fill, p.fillText
	case v.mode.visual():
		bg, fg = p.modified.Alpha(0.22), p.modifiedFg
	}
	row := ui.Row(c.Key("status:vim")).Height(a.statusHeight()).Padding(0, 6).Gap(6).AlignItems(ui.Center)
	row.Children(func() {
		ui.Text(c, v.mode.String()).FontSize(m.textSmall-1).FontWeight(650).LetterSpacing(0.6).TextColor(fg).SingleLine().
			Padding(1, 6).Radius(m.radius).Background(bg)
		if a.vimRec != 0 {
			// A register recording, as vim says: a red dot, and its name.
			ui.Row(c.Key("status:vim-rec")).Gap(5).AlignItems(ui.Center).
				Transition(ui.ElementTransition{Duration: motionShow, Enter: &ui.Motion{Scale: 0.8}, Exit: &ui.Motion{Scale: 0.8}}).
				Children(func() {
					ui.Box(c).Width(6).Height(6).Radius(3).Background(p.dangerFg)
					ui.Text(c, locales.VimRecording(string(a.vimRec))).FontSize(m.textSmall).TextColor(p.dangerFg).SingleLine()
				})
		}
		if k := v.pending(); k != "" {
			text(k)
		}
		// What the last command said, as a search that went round, for a
		// moment; an error in the color of one.
		if msg := a.vimMsg; msg.text != "" && time.Since(msg.at) < vimMessageFor {
			col := p.statusFg
			if msg.err {
				col = p.dangerFg
			}
			ui.Text(c.Key("status:vim-msg"), msg.text).FontSize(m.textSmall).TextColor(col).SingleLine().MaxWidth(420).
				Transition(ui.ElementTransition{Duration: motionMove, Colors: true, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).
				ContentTransition(ui.ContentTransition{})
		}
	})
}

func (a *app) statusControls(c *ui.Context, right bool) {
	if a.settings.SidebarRight != right && a.settings.TerminalButtonRight != right {
		return
	}
	key := "status:layout-controls-left"
	if right {
		key = "status:layout-controls-right"
	}
	ui.Row(c.Key(key)).Gap(2).Shrink(0).AlignItems(ui.Center).Children(func() {
		sidebar := func() {
			if a.settings.SidebarRight != right {
				return
			}
			icon := iconSidebar
			if right {
				icon = iconSidebarRight
			}
			button := a.iconButton(c.Key("status:sidebar"), icon, a.withKey(locales.CmdViewSidebar(), "view.sidebar"), !a.window.SidebarClosed).
				Height(statusBarH - 4).ContextMenu(func(menu *ui.Menu) {
				a.statusControlsMenu(menu, a.settings.SidebarRight, a.setSidebarRight)
			})
			if button.Clicked() {
				a.toggleSidebar()
			}
		}
		if !right {
			sidebar()
		}
		if a.settings.TerminalButtonRight == right {
			button := a.iconButton(c.Key("status:terminal"), iconTerminal, a.withKey(locales.CmdViewTerminal(), "view.terminal"), a.window.PanelOpen).
				Height(statusBarH - 4).ContextMenu(func(menu *ui.Menu) {
				a.statusControlsMenu(menu, a.settings.TerminalButtonRight, a.setTerminalButtonRight)
			})
			if runtime.GOOS == "linux" {
				button.Padding(1, 0, 0, 0)
			}
			if it, dropped := ui.Drop[dragItem](button); dropped && it.tab != nil && it.tab.term.isShell() && a.owns(it) {
				a.post(func() { a.terminalToPanel(it.tab.term) })
			} else if button.Clicked() {
				a.togglePanel()
			}
		}
		if right {
			sidebar()
		}
	})
}

func (a *app) statusControlsMenu(menu *ui.Menu, right bool, set func(bool)) {
	if menu.Item(locales.StatusControlsLeft()).Checked(!right).Chosen() {
		a.post(func() { set(false) })
	}
	if menu.Item(locales.StatusControlsRight()).Checked(right).Chosen() {
		a.post(func() { set(true) })
	}
}

func (a *app) statusHeight() float32 {
	if a.settings.MicaEnabled {
		return statusBarH - 4
	}
	return statusBarH
}

func (a *app) statusBar(c *ui.Context) {
	p := a.pal
	m := p.metricsOr()
	repo := a.repository()
	width, _ := c.Size()
	visible := a.statusFit(c, width)
	bar := ui.Row(c).Height(a.statusHeight()).Padding(0, 6).Gap(2).AlignItems(ui.Center).Background(windowBackground(c, a.settings.WindowOpacity, p.status)).
		BorderWidth(1, 0, 0, 0).BorderColor(ui.Transparent)
	a.grained(bar)
	bar.Children(func() {
		a.statusControls(c, false)
		a.lspStatus(c)
		a.debugStatus(c)
		defer a.notificationBell(c)
		defer a.statusControls(c, true)
		// Its digits are of one width, so that the caret's line and column,
		// changing with each key, do not shift the items after them.
		text := func(s string) ui.Element {
			return ui.Text(c, s).FontSize(m.textSmall).FontFeatures("tnum").TextColor(p.statusFg).SingleLine()
		}
		// item is a part of the bar, a button when it does something.
		item := func(key, label, tip string, icon *ui.SVG, fn func()) {
			if fn == nil {
				// It does nothing, but tells what it is under the pointer, as
				// the plugins' items do.
				r := ui.Row(c.Key("status:"+key)).Height(a.statusHeight()).Padding(0, 6).Gap(5).AlignItems(ui.Center)
				if tip != "" {
					r.Label(tip).Tooltip(tip)
				}
				r.Children(func() {
					if icon != nil {
						ui.Icon(c, icon).FontSize(12).TextColor(p.statusFg)
					}
					text(label)
				})
				return
			}
			b := ui.ButtonBase(c.Key("status:"+key)).Height(statusBarH-6).Padding(0, m.pad).Gap(5).Radius(m.radius).
				AlignItems(ui.Center).Label(tip).Tooltip(tip)
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Transition(hoverFade)
			b.Children(func() {
				if icon != nil {
					ui.Icon(c, icon).FontSize(12).TextColor(p.statusFg)
				}
				text(label)
			})
			if b.Clicked() {
				fn()
			}
		}
		// menuItem is a part of the bar that opens a menu.
		menuItem := func(show bool, key, label, tip string, build func(m *ui.Menu)) {
			if !show {
				return
			}
			b := ui.ButtonBase(c.Key("status:"+key)).Height(statusBarH-6).Padding(0, m.pad).Radius(m.radius).
				AlignItems(ui.Center).Label(tip).Tooltip(tip)
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Transition(hoverFade)
			b.Children(func() { text(label) })
			b.Menu(build)
		}
		if m := a.themeMascot(); m != nil {
			a.statusMascot(c, m)
		} else {
			// None shows: a repaint of the frame before, which had one,
			// may have set it moving as the theme changed.
			a.mascot = mascotState{}
		}
		a.remoteStatus(c, text)
		a.portsStatus(c, text)
		if repo.branch() != "" {
			repo.prBadge(c, true, !visible.prDetail)
		}
		a.transferStatus(c, text)
		if busy := repo.git.label(); busy != "" && p.activityLabel {
			a.claudeSpinner(c, busy)
		} else if busy != "" {
			ui.Row(c.Key("status:busy")).Height(a.statusHeight()).Padding(0, 6).Gap(5).AlignItems(ui.Center).Children(func() {
				// In a theme with a character, it hops for it instead.
				if a.themeMascot() == nil {
					ui.Spinner(c).Size(11, 11)
				}
				text(busy)
			})
		}
		if len(a.chord) > 0 {
			item("chord", locales.ChordWaiting(strings.Join(chordCaps(a.chord), " ")), "", iconKeyboard, nil)
		}
		a.reviewStatus(c, text)
		a.pluginStatusItems(c, false, item, text)
		if t := a.activeTab(); t != nil && t.ed != nil && t.diff == nil && t.term == nil && a.activeViewer() == nil {
			if editorMode(a.prefs()) == "vim" {
				a.vimBadge(c, &t.ed.vim, text)
			} else if a.helixOn() {
				a.helixBadge(c, &t.ed.helix, text)
			}
		}
		ui.Spacer(c)
		a.pluginStatusItems(c, true, item, text)
		if v := a.activeViewer(); v != nil {
			a.viewerStatus(c, v, item, text)
			return
		}
		if t := a.activeTab(); t != nil && t.diff != nil {
			if s := t.diff.diffStatus(); s != "" {
				item("diff", s, "", iconDiff, nil)
			}
			return
		}
		if t := a.activeTab(); t != nil && t.term != nil {
			// A terminal: where its shell is, and its size in cells.
			term := t.term.term
			if dir := term.Dir(); dir != "" && visible.terminal {
				item("cwd", shortenHome(dir), "", iconTerminal, nil)
			}
			if cols, rows := term.Size(); cols > 0 && visible.terminal {
				item("size", fmt.Sprintf("%d × %d", cols, rows), "", nil, nil)
			}
			return
		}
		d := a.activeEditor()
		if d == nil {
			return
		}
		if d.diskGone {
			ui.Row(c).Padding(0, 6).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconTrash).FontSize(12).TextColor(p.danger)
				text(locales.DeletedOnDisk()).TextColor(p.dangerFg)
			})
		}
		snap := d.buf.Snapshot()
		caret := min(d.caret, snap.Len())
		if editorMode(a.prefs()) == "vim" && d.vim.mode.visual() {
			caret = vimCursor(d, snap)
		}
		line := snap.LineAt(caret)
		s, _ := snap.LineRange(line)
		pos := locales.StatusPos(line+1, columns(snap.Slice(s, caret))+1)
		if n := abs(d.sel[1] - d.sel[0]); n > 0 {
			pos += locales.StatusSelected(n)
		}
		item("pos", pos, a.withKey(locales.GoToLineLabel(), "go.line"), nil, func() { a.openQuick(":") })
		if visible.lines {
			item("lines", locales.LinesCount(snap.LineCount()), "", nil, nil)
		}
		menuItem(visible.indent, "indent", d.indent.String(), locales.Indentation(), func(m *ui.Menu) {
			if m.Item(locales.CmdSelIndentTabs()).Checked(d.indent.tabs).Chosen() {
				d.indent.tabs = true
			}
			for _, n := range []int{2, 4, 8} {
				if m.Item(locales.CmdSelIndentSpaces(n)).Checked(!d.indent.tabs && d.indent.size == n).Chosen() {
					d.indent = indentStyle{size: n}
				}
			}
			m.Separator()
			if m.Item(locales.ConvertToTabs()).Chosen() {
				a.editActive(func(d *editor) { reindent(d, indentStyle{tabs: true, size: tabWidth()}) })()
			}
			if m.Item(locales.ConvertToSpaces()).Chosen() {
				size := d.indent.size
				if d.indent.tabs {
					size = tabWidth()
				}
				a.editActive(func(d *editor) { reindent(d, indentStyle{size: size}) })()
			}
		})
		if visible.encoding {
			item("enc", "UTF-8", "", nil, nil)
		}
		eol := "LF"
		if d.crlf {
			eol = "CRLF"
		}
		menuItem(visible.eol, "eol", eol, locales.LineEndings(), func(m *ui.Menu) {
			for _, crlf := range []bool{false, true} {
				label := "LF"
				if crlf {
					label = "CRLF"
				}
				if m.Item(label).Checked(d.crlf == crlf).Chosen() {
					d.crlf = crlf
				}
			}
		})
		if visible.language {
			item("lang", highlight.DisplayName(d.hl.Lexer()), "", nil, nil)
		}
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// iconArt and iconDarkArt are the app icon's tiles for a light and a
// dark window, as the welcome page shows them.
//
//go:embed icon.png
var iconArt []byte

//go:embed icon-dark.png
var iconDarkArt []byte

// logo is FiveLeaf's mark, as the welcome page shows it in a dark window,
// decoded the first time the page shows. iconLogo is the light one.
var logo = sync.OnceValue(func() *ui.Bitmap { return iconBitmap(iconDarkArt) })
var iconLogo = sync.OnceValue(func() *ui.Bitmap { return iconBitmap(iconArt) })

func iconBitmap(data []byte) *ui.Bitmap {
	art, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return ui.NewBitmap(art)
}

// welcome shows while no file is open: what to start with, and the
// folders opened last.
func (a *app) welcome(c *ui.Context) {
	p := a.pal
	m := p.metricsOr()
	// Centered in the view, and scrolled when the window is too short.
	view := a.welcomeView.Bounds(c)
	page := ui.Scroll(c.Key("welcome")).Bind(&a.welcomeView).Fill().Background(windowBackground(c, a.settings.WindowOpacity, p.editor))
	if p.welcomeRain {
		still := a.blurred || c.Preferences().ReduceMotion
		page.Draw(func(pt *ui.Painter, r ui.Rect) { a.drawRain(pt, r, still) })
	}
	page.Children(func() {
		ui.Column(c).FillWidth().MinHeight(view.H).Center().Padding(40, 24, 64).Children(func() {
			content := ui.Column(c).Width(380).MaxWidthPercent(100).Gap(32)
			if p.welcomeRain {
				// Over the rain, which would run through the words.
				content.Width(380+2*24).Padding(24).Radius(12).Background(p.editor.Alpha(0.88)).Border(1, p.border)
			}
			content.Children(func() {
				// Inset as the rows below, so the logo lines up with their icons.
				riseIn(ui.Row(c.Key("welcome:head")).Padding(0, 10).Gap(16).AlignItems(ui.Center), 0).Children(func() {
					if m := a.themeMascot(); m != nil {
						a.welcomeMascot(c, m)
					} else {
						mark := iconLogo()
						if p.dark {
							mark = logo()
						}
						ui.Image(c, mark).Size(52, 52)
					}
					ui.Column(c).Gap(3).Children(func() {
						ui.Text(c, "FiveLeaf").FontSize(m.textTitle * 2).FontWeight(m.weightBold).LetterSpacing(-0.3).TextColor(p.text)
						ui.Text(c, locales.WelcomeTagline()).FontSize(m.textBody).TextColor(p.muted)
					})
				})
				if v := a.host.update().version; v != "" {
					a.welcomeSection(c, locales.WelcomeUpdate(), 30*time.Millisecond, func() {
						a.welcomeAction(c, iconUpdate, locales.UpdateAvailable(v), "", a.openUpdates)
					})
				}
				a.welcomeSection(c, locales.WelcomeStart(), 60*time.Millisecond, func() {
					if a.root == "" {
						a.welcomeAction(c, iconFolderOpen, locales.CmdFileOpenFolder(), a.keyHint("file.openFolder"), a.chooseFolder)
						if remoteSupported {
							a.welcomeAction(c, iconServer, locales.CmdRemoteConnect(), a.keyHint("remote.connect"), a.openRemotePicker)
						}
					} else {
						a.welcomeAction(c, iconFileSearch, locales.CmdGoQuickOpen(), a.keyHint("go.quickOpen"), func() { a.openQuick("") })
						a.welcomeAction(c, iconSearch, locales.CmdEditFindInFiles(), a.keyHint("edit.findInFiles"), func() { a.showSearch(false) })
					}
					a.welcomeAction(c, iconCommand, locales.CmdGoCommands(), a.keyHint("go.commands"), func() { a.openQuick(">") })
					a.welcomeAction(c, iconFilePlus, locales.CmdFileNew(), a.keyHint("file.new"), a.newFile)
				})
				var recent []string
				if a.root == "" {
					recent = a.settings.RecentExcept("")
				}
				if len(recent) > 0 {
					a.welcomeSection(c, locales.WelcomeRecent(), 120*time.Millisecond, func() {
						for _, dir := range recent[:min(6, len(recent))] {
							r := ui.ButtonBase(c.Key("recent:"+dir)).FillWidth().Height(welcomeRowH).Padding(0, 10).Radius(6).Gap(10).
								Justify(ui.Start).AlignItems(ui.Center).Label(locales.RecentOpenLabel(dir)).Tooltip(shortenHome(dir))
							if r.Hovered() {
								r.Background(p.hover)
							}
							r.Transition(hoverFade)
							r.Children(func() {
								icon := iconFolder
								if remote.Is(dir) {
									icon = iconServer
								}
								ui.Icon(c, icon).FontSize(15).TextColor(p.accent)
								ui.Text(c, filepath.Base(dir)).FontSize(m.textBody).TextColor(p.text).SingleLine().Shrink(0)
								ui.Text(c, shortenHome(filepath.Dir(dir))).FontSize(m.textBody).TextColor(p.faint).SingleLine().Shrink(1)
							})
							// With Cmd, in a window of its own.
							switch {
							case r.Focused() && c.Shortcut(ui.Cmd, ui.KeyEnter):
								a.host.newWindow(dir)
							case r.Clicked():
								if r.ClickModifiers()&ui.Cmd != 0 {
									a.host.newWindow(dir)
								} else {
									a.switchFolder(dir)
								}
							}
						}
						// The rest, in Open Recent's picker.
						if len(recent) > 6 {
							a.welcomeAction(c, iconMore, locales.MenuOpenRecentMore(), a.keyHint("file.openRecent"), a.openRecentPicker)
						}
					})
				}
			})
		})
	})
}

// welcomeRowH is the height of every row of the welcome page, with keys
// or without, so that they keep one rhythm.
const welcomeRowH = 30

// welcomeSection is a titled group of the welcome page.
// It rises in after delay as the page shows.
func (a *app) welcomeSection(c *ui.Context, title string, delay time.Duration, rows func()) {
	p := a.pal
	m := p.metricsOr()
	riseIn(ui.Column(c.Key("welcome:"+title)).Gap(2), delay).Children(func() {
		// As the sidebar titles its views.
		ui.Text(c, strings.ToUpper(title)).FontSize(m.textSmall).FontWeight(m.weightBold).LetterSpacing(0.7).TextColor(p.muted).Padding(0, 10, 6)
		rows()
	})
}

// welcomeAction is a row of the welcome page that runs a command: its
// icon, its name and its keys.
func (a *app) welcomeAction(c *ui.Context, icon *ui.SVG, label, keys string, fn func()) {
	p := a.pal
	m := p.metricsOr()
	r := ui.ButtonBase(c.Key("action:"+label)).FillWidth().Height(welcomeRowH).Padding(0, 10).Radius(6).Gap(10).
		Justify(ui.Start).AlignItems(ui.Center).Label(label)
	hovered := r.Hovered()
	if hovered {
		r.Background(p.hover)
	}
	r.Transition(hoverFade)
	col := p.muted
	if hovered {
		col = p.accent
	}
	col = r.AnimateColor("icon", col, ui.Animation{Duration: hoverFade.Duration})
	r.Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(col)
		ui.Text(c, label).FontSize(m.textBody).TextColor(p.text).Grow(1).SingleLine()
		a.keycaps(c, keys)
	})
	if r.Clicked() {
		fn()
	}
}

// keycaps shows keys as the caps of a keyboard.
func (a *app) keycaps(c *ui.Context, keys string) { a.caps(c, splitCaps(keys)) }

// splitCaps splits keys as keyText writes them into their caps: those of
// "Ctrl+Shift+P" at the pluses, those of "⇧⌘P" a symbol each, with a
// named key, as "esc" or "F3", one cap, and the strokes of a chord apart
// by spaces one after the other.
func splitCaps(keys string) []string {
	var caps []string
	for _, stroke := range strings.Fields(keys) {
		if len(stroke) > 1 && strings.Contains(stroke, "+") {
			// The key may be "+" itself, after the modifiers.
			plus := strings.HasSuffix(stroke, "++")
			for _, k := range strings.Split(strings.TrimSuffix(stroke, "++"), "+") {
				if k != "" {
					caps = append(caps, k)
				}
			}
			if plus {
				caps = append(caps, "+")
			}
			continue
		}
		word := ""
		for _, r := range stroke {
			if r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
				word += string(r)
				continue
			}
			if word != "" {
				caps, word = append(caps, word), ""
			}
			caps = append(caps, string(r))
		}
		if word != "" {
			caps = append(caps, word)
		}
	}
	return caps
}

// caps shows keys as the caps of a keyboard.
func (a *app) caps(c *ui.Context, caps []string) {
	p := a.pal
	m := p.metricsOr()
	ui.Row(c).Gap(3).Shrink(0).Children(func() {
		for _, k := range caps {
			ui.Text(c, k).FontSize(m.textSmall).TextColor(p.muted).TextAlign(ui.Center).
				MinWidth(18).Height(18).Padding(1, 4, 0).Radius(4).Background(p.hover).Border(1, p.border)
		}
	})
}

func (a *app) toggleSidebar() {
	a.window.SidebarClosed = !a.window.SidebarClosed
	a.settings.Changed()
}

// toggleSidebarRight moves the sidebar and its button to the other side.
func (a *app) toggleSidebarRight() {
	a.setSidebarRight(!a.settings.SidebarRight)
}

func (a *app) setSidebarRight(right bool) {
	if a.settings.SidebarRight == right {
		return
	}
	a.settings.SidebarRight = right
	a.settings.Changed()
	a.host.redrawWindows()
	a.host.menuChanged()
}

func (application *app) setTerminalButtonRight(right bool) {
	if application.settings.TerminalButtonRight == right {
		return
	}
	application.settings.TerminalButtonRight = right
	application.settings.Changed()
	application.host.redrawWindows()
	application.host.menuChanged()
}

// togglePanel shows or hides the terminal panel, starting a terminal the
// first time it shows.
func (a *app) togglePanel() {
	if a.window.PanelOpen {
		if a.focus != paneTerminal && a.panelTerm != nil {
			a.focusTerminal()
		} else {
			a.hidePanel()
		}
		return
	}
	if a.panelTerm != nil {
		a.terminalToPanel(a.panelTerm)
	} else if tab := a.activeTab(); tab != nil && tab.term.isShell() {
		a.terminalToPanel(tab.term)
	} else if s := a.reviewTerminal(); s != nil {
		a.terminalToPanel(s)
	} else {
		a.newTerminal()
	}
}

// toggleTerminalLocation has new terminals open in the editor area, or in
// the panel again.
func (a *app) toggleTerminalLocation() {
	if a.settings.TerminalLocation == "" {
		a.settings.TerminalLocation = "editor"
	} else {
		a.settings.TerminalLocation = ""
	}
	a.settings.Changed()
}

// setPanelPosition moves the terminal panel to a side of the window, or
// back below the editors for "", and shows it when it was hidden.
func (a *app) setPanelPosition(pos string) {
	a.settings.PanelPosition = pos
	a.settings.Changed()
	if !a.window.PanelOpen {
		a.togglePanel()
	}
}

func shortenHome(p string) string {
	if remote.Is(p) {
		return remote.Display(p)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return shortenIn(p, home)
	}
	return p
}

// shortenIn is p with ~ for the home folder home when p is in it, or is
// it: of a folder, not of a name home only begins, and on Windows
// whatever the case of its letters, as C: and c: are one disk.
func shortenIn(p, home string) string {
	same := func(a, b string) bool { return a == b || hostOS == "windows" && strings.EqualFold(a, b) }
	home = strings.TrimSuffix(home, string(filepath.Separator))
	if home == "" || len(p) < len(home) || !same(p[:len(home)], home) {
		return p
	}
	if rest := p[len(home):]; rest == "" || strings.HasPrefix(rest, string(filepath.Separator)) {
		return "~" + rest
	}
	return p
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }

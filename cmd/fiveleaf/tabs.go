package main

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/remote"
	"fiveleaf/locales"
)

const (
	tabHeight    = 26
	tabTitleMaxW = 180
)

// crossFade is how a tab's cross comes and goes with the pointer.
var crossFade = ui.ElementTransition{Duration: motionTap, Enter: &ui.Motion{}, Exit: &ui.Motion{}}

// tabWheelGlide is how a notch of the wheel scrolls the tabs, as it scrolls
// a container elsewhere; tabWheelLand is a touchpad's move, which already
// follows the fingers, and so lands in the frame after it.
var (
	tabWheelGlide = ui.Animation{Duration: motionShow, Ease: ui.EaseOut}
	tabWheelLand  = ui.Animation{Duration: time.Millisecond, Ease: ui.EaseOut}
)

// tabStrip shows a tab for each tab of a column: a click shows it, the
// cross closes it, and a dot marks unsaved changes. A tab is dragged
// along the strip, or to another column, and dropped where it goes.
func (a *app) tabStrip(c *ui.Context, col *column) {
	p := a.pal
	m := p.metricsOr()
	material := materialOf(c)
	var closing *colTab
	// The column that has the keyboard shows it on its tab.
	focused := col == a.column()
	drop := func(e ui.Element, before *colTab) {
		if it, ok := ui.Drop[dragItem](e); ok {
			a.post(func() {
				at := len(col.tabs)
				if _, i := a.locate(before); i >= 0 {
					at = i
				}
				a.dropInColumn(it, col, at)
			})
		}
		// Files from another app, there too.
		if paths := e.DroppedFiles(); paths != nil {
			paths = slices.Clone(paths)
			a.post(func() {
				at := len(col.tabs)
				if _, i := a.locate(before); i >= 0 {
					at = i
				}
				a.dropFiles(paths, col, at)
			})
		}
	}
	ui.Row(c).Height(tabStripH).Background(windowBackground(c, a.settings.WindowOpacity, p.strip)).AlignItems(ui.Center).
		ContextMenu(func(m *ui.Menu) { a.columnLockMenu(m, col) }).Children(func() {
		strip := ui.ScrollHorizontal(c.Key("tabs")).TrackScroll(&col.tabScroll).
			Grow(1).Height(tabStripH).Margin(0, m.inset).ScrollbarInsets(0, 0, 0, 0)
		strip = strip.HandleInput(func(ev ui.InputEvent) bool { return a.tabWheel(col, ev) })
		viewport := strip.LayoutBounds().W
		narrowed := viewport > 0 && viewport < col.tabViewport
		col.tabViewport = viewport
		// Past the last tab: there.
		drop(strip, nil)
		_, atEnd := a.dragOver(strip)
		atEnd = atEnd || strip.FileDragOver()
		strip.Children(func() {
			ui.Row(c).Height(tabStripH).AlignItems(ui.Center).Children(func() {
				for i, t := range col.tabs {
					var gapOver bool
					var insertLeft float32
					if i > 0 {
						// The space between tabs inserts before the next tab too.
						gap := ui.Box(c.Key(struct{ gap int }{t.id})).Size(m.gap, tabStripH).Shrink(0)
						drop(gap, t)
						_, gapOver = a.dragOver(gap)
						gapOver = gapOver || gap.FileDragOver()
						insertLeft = -m.gap/2 - 1
					}
					active := i == col.active
					tip := locales.TabTerminal()
					switch {
					case t.isTask():
						tip = locales.MenuTasks() + " · " + t.term.task.resolved.Task.Label
					case t.tool != nil:
						tip = t.name()
					case t.term == nil:
						tip = a.relOrPath(t.path())
					}
					// Reserve the border in every state so choosing a tab never
					// moves its title, close button or neighbours.
					tab := ui.ButtonBase(c.Key(t.id)).Height(tabHeight).Padding(0, m.gap, 0, m.pad).Gap(m.gap).
						AlignItems(ui.Center).Radius(m.radius).Border(1, ui.Transparent).
						Label(t.name()).Role(ui.RoleTab).Selected(active).
						Transition(tabMotion)
					chatTab := t.tool != nil && t.tool.chat != nil
					compactChat := chatTab && viewport > 0 && viewport < 3*m.control
					if chatTab && viewport > 0 {
						tab.MaxWidth(viewport)
					}
					if compactChat {
						tab.Padding(0, m.gap).Gap(m.gap / 2)
					}
					a.dragTab(tab, t)
					// The middle button closes the tab, as in a browser.
					box := tab.LayoutBounds()
					tab = tab.HandleInput(func(ev ui.InputEvent) bool {
						return a.tabMiddle(t, ev, box) || a.tabWheel(col, ev)
					})
					if tab.Dragging() {
						a.dragNow = true
					}
					drop(tab, t)
					_, over := a.dragOver(tab)
					over = over || tab.FileDragOver() || gapOver
					hovered := tab.Hovered()
					under := p.strip
					switch {
					case active:
						under = p.active.Over(p.strip)
						border := p.border.Alpha(0.5)
						if focused {
							border = p.border
						}
						if material.mica() {
							material.glassCard(tab, p, m.radius)
							if focused {
								tab.BorderColor(p.accent.Mix(p.muted, 0.52).Alpha(0.55))
							}
						} else {
							tab.Background(under).BorderColor(border)
						}
					case hovered:
						under = p.hover.Over(p.strip)
						tab.Background(material.insetColor(under))
					}
					if tab.FocusVisible() {
						tab.BorderColor(p.accent)
					}
					if a.middle != t && (tab.Pressed() || tab.Clicked()) {
						a.activate(col, i, true)
					}
					if tab.DoubleClicked() {
						a.keepTab(t)
					}
					if active && (col.revealed != t.id || chatTab && narrowed) {
						// Bring the tab shown into view, once as it is chosen.
						tab.ScrollIntoView()
						col.revealed = t.id
					}
					tab.ContextMenu(func(m *ui.Menu) { a.tabMenu(m, col, t, &closing) })
					tab.Children(func() {
						if over {
							// Where the tab dragged goes: before this one.
							ui.Box(c).Absolute().Top(0).Bottom(0).Left(insertLeft).Width(2).Radius(1).Background(p.accent)
						}
						if chatTab {
							a.chatTabIcon(c, t.tool.chat, tab, tip, under)
						} else {
							a.tabStripIcon(c, t, under)
							tab.Tooltip(tip)
						}
						name := ui.Text(c, t.name()).FontSize(m.textBody).SingleLine().MaxWidth(tabTitleMaxW)
						if chatTab {
							name.MinWidth(0).Shrink(1).Hidden(compactChat)
						}
						if t.transient {
							// A preview, which the next file chosen replaces.
							name.Italic()
						}
						switch {
						case t.ed != nil && t.ed.diskGone, t.view != nil && t.view.diskGone:
							name.TextColor(p.dangerFg).Strikethrough()
						case active:
							// In color alone: a heavier weight would widen the tab
							// and move those after it.
							name.TextColor(p.text)
						default:
							name.TextColor(p.muted)
						}
						if chatTab {
							a.chatTabStatus(c, t.tool.chat, under)
						}
						// The dot of unsaved changes turns into the cross under the
						// pointer.
						x := ui.ButtonBase(c).Size(18, 18).Radius(m.radius).Justify(ui.Center).AlignItems(ui.Center).Label(t.closeLabel())
						if x.Hovered() {
							x.Background(p.hover)
						}
						x.Transition(hoverFade)
						x.Children(func() {
							switch {
							case t.ed != nil && t.ed.dirty() && !x.Hovered():
								ui.Box(c.Key("dot")).Size(8, 8).Radius(4).Background(p.text.Alpha(0.7)).Transition(dotPop)
							case active || hovered || x.Hovered():
								ui.Icon(c.Key("x"), iconX).FontSize(13).TextColor(p.muted).Transition(crossFade)
							}
						})
						if x.Clicked() {
							closing = t
						}
					})
				}
				if atEnd {
					ui.Box(c).Size(2, tabHeight).Radius(1).Background(p.accent)
				}
			})
		})
		ui.Row(c).Padding(0, 6, 0, 4).Children(func() {
			a.chatTabActions(c, col.tab())
			if col.locked {
				if a.iconButton(c, iconLock, locales.UnlockColumn(), true).Clicked() {
					a.post(func() { a.toggleColumnLock(col) })
				}
			}
			if tab := col.tab(); tab != nil && tab.term.isShell() {
				if a.iconButton(c.Key("new-terminal"), iconPlus, locales.CmdViewNewTerminal(), false).KeepFocus().Clicked() {
					a.post(func() {
						if col.tab() == tab && slices.Contains(a.cols, col) {
							a.newTerminalTab(col, tab.term.nextShellDir(), true)
						}
					})
				}
			}
			a.paneSplitButton(c, col)
		})
	})
	if closing != nil {
		// After the frame, which is still drawing the columns: the column
		// may go with its last tab.
		a.post(func() { a.closeTab(closing, nil, nil) })
	}
}

func (tab *colTab) isTask() bool { return tab != nil && tab.term != nil && tab.term.task != nil }

func (a *app) tabStripIcon(c *ui.Context, tab *colTab, under ui.Color) {
	p := a.pal
	var icon *ui.SVG
	var tint ui.Color
	switch {
	case tab.isTask():
		icon, tint = iconClipboardList, p.muted
	case tab.term != nil:
		a.termIcon(c, tab.term, 14, p.muted, under)
		return
	case tab.diff != nil && tab.diff.commit != nil && tab.diff.commit.pr != nil:
		icon, tint = iconPullRequest, p.accent
	case tab.diff != nil && tab.diff.commit != nil:
		icon, tint = iconCommit, p.accent
	case tab.diff != nil:
		icon, tint = iconDiff, p.changed
	case tab.tool != nil:
		icon, tint = tab.tool.icon(), p.accent
	default:
		icon, tint = a.fileIcon(tab.name())
	}
	iconThemeView(c, p, icon, tint, 14)
}

// middleButton is the middle mouse button, as InputEvent.Button names it:
// 0 the primary button and 1 the secondary.
const middleButton = 2

// tabMiddle takes the input of the middle button on a tab, which closes
// the tab rather than shows it, as a browser's tabs do. The press is
// taken, so that the release comes back here wherever in the tab it is,
// and the tab it is on is remembered while it is held, so that the frames
// between do not show it. Let go of off the tab, box as the last frame
// laid it out, it closes nothing, as in a browser.
func (a *app) tabMiddle(t *colTab, ev ui.InputEvent, box ui.Rect) bool {
	if ev.Button != middleButton {
		// A press of another button ends one of the middle whose release
		// never came, which would keep the tab from showing.
		if ev.Kind == ui.InputPointerDown {
			a.middle = nil
		}
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		a.middle = t
		return true
	case ui.InputPointerUp:
		on := box.W == 0 || ev.X >= 0 && ev.Y >= 0 && ev.X < box.W && ev.Y < box.H
		if a.middle == t && on {
			a.post(func() { a.closeTab(t, nil, nil) })
		}
		a.middle = nil
		return true
	}
	return false
}

// tabWheel scrolls a column's tabs with the wheel: a plain wheel, which a
// container that only scrolls sideways leaves alone, scrolls them as
// browsers scroll their tabs. A sideways wheel, or one with Shift, which
// turns it sideways, is the container's own.
func (a *app) tabWheel(col *column, ev ui.InputEvent) bool {
	// swipeSideways the other way around: the wheel goes down as the tabs go
	// sideways, and a touchpad's swipe carries a little of the other way.
	if ev.Kind != ui.InputScroll || ev.Mods&ui.Shift != 0 || max(ev.DY, -ev.DY) <= max(ev.DX, -ev.DX) {
		return false
	}
	// A notch glides there, as it scrolls the containers the wheel is left;
	// a touchpad's moves follow the fingers, and land at once. Notches on
	// their way pile up from where the last aimed, since the glide does not
	// tell where it goes.
	s := &col.tabScroll
	if !s.Scrolling() {
		col.tabWheelTo = s.X
	}
	col.tabWheelTo = clamp(col.tabWheelTo+ev.DY, 0, s.MaxX)
	slide := tabWheelGlide
	if ev.Precise {
		slide = tabWheelLand
	}
	s.ScrollTo(col.tabWheelTo, s.Y, slide)
	return true
}

// tabMenu is the menu of a tab.
func (a *app) tabMenu(m *ui.Menu, col *column, t *colTab, closing **colTab) {
	// show makes the tab the one commands are for.
	show := func(fn func()) {
		a.post(func() {
			if col, i := a.locate(t); col != nil {
				a.activate(col, i, true)
				fn()
			}
		})
	}
	if m.Item(locales.TabClose()).Chosen() {
		*closing = t
	}
	if m.Item(locales.TabCloseOthers()).Disabled(len(col.tabs) < 2).Chosen() {
		a.post(func() { a.closeOthers(col, t) })
	}
	if m.Item(locales.TabCloseAll()).Chosen() {
		a.post(func() { a.closeTabs(slices.Clone(col.tabs), nil, nil) })
	}
	m.Separator()
	a.pluginMenuItems(m, "tab/context", t.path())
	a.columnLockMenu(m, col)
	if m.Item(locales.CmdViewSplitRight()).Disabled(!t.canSplit()).Chosen() {
		show(a.splitRight)
	}
	at := slices.Index(a.cols, col)
	alone := len(col.tabs) < 2
	if m.Item(locales.MoveToNextColumn()).Disabled(alone && at == len(a.cols)-1).Chosen() {
		show(func() { a.moveTabBy(1) })
	}
	if m.Item(locales.MoveToPrevColumn()).Disabled(alone && at == 0).Chosen() {
		show(func() { a.moveTabBy(-1) })
	}
	if m.Item(locales.MoveToNewWindow()).Disabled(!a.canDetachTab(t)).Chosen() {
		a.post(func() { a.detachTab(t, nil) })
	}
	a.moveToProjectMenu(m, t)
	if t.term != nil {
		if m.Item(locales.TermMoveLockedRight()).Chosen() {
			a.post(func() { a.terminalToLockedRight(t.term) })
		}
		if t.term.isShell() && m.Item(locales.MoveToPanel()).Chosen() {
			a.post(func() { a.terminalToPanel(t.term) })
		}
		if m.Item(locales.TermRename()).Chosen() {
			a.post(func() { a.askRenameTerminal(t.term) })
		}
		return
	}
	m.Separator()
	if d := t.ed; d != nil && canPreview(d.document) && !isMarkdown(d.document) {
		if m.Item(locales.CmdViewPreviewToSide()).Chosen() {
			show(a.previewToSide)
		}
	}
	path := t.path()
	if t.diff != nil {
		if m.Item(locales.OpenFile()).Chosen() {
			show(a.openChanges)
		}
	} else if t.ed != nil && path != "" && a.repositoryForPath(path).git.repo != nil && m.Item(locales.CmdGitOpenChanges()).Chosen() {
		show(a.openChanges)
	}
	if m.Item(locales.ScmRevealInExplorer()).Disabled(path == "" || a.root == "").Chosen() {
		a.post(func() { a.showExplorer(); a.revealInTree(path) })
	}
	if m.Item(revealLabel()).Disabled(path == "" || remote.Is(path)).Chosen() {
		a.host.reveal(path)
	}
	if m.Item(locales.CopyPath()).Disabled(path == "").Chosen() {
		a.copyPath(path, false)
	}
	if m.Item(locales.CopyRelativePath()).Disabled(path == "").Chosen() {
		a.copyPath(path, true)
	}
	if path != "" && a.repositoryForPath(path).git.repo != nil {
		m.Separator()
		if m.Item(locales.CmdGitOpenFileOnRemote()).Chosen() {
			a.post(func() { a.fileOnRemote(path, t.ed, false) })
		}
		if m.Item(locales.CmdGitCopyPermalink()).Chosen() {
			a.post(func() { a.fileOnRemote(path, t.ed, true) })
		}
	}
}

func (a *app) columnLockMenu(m *ui.Menu, col *column) {
	label := locales.LockColumn()
	if col.locked {
		label = locales.UnlockColumn()
	}
	if m.Item(label).Chosen() {
		a.post(func() { a.toggleColumnLock(col) })
	}
}

package main

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/lsp"
	"fiveleaf/internal/plugin"
	"fiveleaf/locales"
)

type editorPointerHover struct {
	ed                     *editor
	point                  semanticPoint
	x, y, scrollX, scrollY float32
	view                   ui.Rect
	version                uint64
	seen                   int
	since                  time.Time
	hit, over              bool
	attempted, suppressed  bool
}

type hoverCard struct {
	tree *viewNode
}

// A pointer query never advances navigationSerial: reading another symbol must
// not invalidate a definition that the user explicitly asked to visit.
type hoverQuery struct {
	nav               lspNavigation
	ed                *editor
	pointer           bool
	cancel            context.CancelFunc
	parts             []string
	diagnostics       []diagnostic
	pending           int
	anchor, box, view ui.Rect
	scrollX, scrollY  float32
	sel               [2]int
	started, leftAt   time.Time
}

func (a *app) dismissHover() {
	if q := a.hoverQuery; q != nil {
		q.cancel()
	}
	a.hoverQuery, a.hover = nil, nil
}

func (a *app) suppressHover() {
	a.dismissHover()
	a.pointerHover.suppressed = true
	a.pointerHover.attempted = true
}

func (a *app) cancelDefinition() {
	if a.definitionPending != nil {
		a.navigationSerial++
		a.definitionPending = nil
	}
	if a.navigationCancel != nil {
		a.navigationCancel()
		a.navigationCancel = nil
	}
}

func (a *app) semanticOverlayOpen() bool {
	return a.quick.open || a.leader.open || a.pluginPick.open || a.prompt != nil || a.confirm != nil ||
		a.branches.open || a.stashes.open || a.prs.open || a.checks.open || a.worktrees.open ||
		a.recents.open || a.themes.open || a.iconPicker.open || a.remotePick.open || a.taskPick.open ||
		a.repositories.picker.open || a.conflicts.open || a.diagram != nil
}

// editorSemanticPointer returns the visible word segment to underline. The
// cached hit makes ordinary repaints cost nothing, even on a very long line.
func (a *app) editorSemanticPointer(c *ui.Context, d *editor, area ui.Element) *ui.Rect {
	if d != a.activeEditor() || a.livePreview(d) || d.locked() || a.blurred || a.semanticOverlayOpen() {
		return nil
	}
	hovered := area.Hovered()
	x, y, over := area.PointerPosition()
	if q := a.hoverQuery; q != nil && !q.pointer {
		return nil
	}

	p := &a.pointerHover
	if !over && p.ed == d && a.hoverQuery == nil {
		p.hit = false
	}
	over = over && !d.pressing && !d.selecting
	if p.ed == d {
		p.seen, p.over = a.builds, false
	}
	if !over {
		return nil
	}
	view, version := area.Bounds(), d.buf.Version()
	if q := a.hoverQuery; q != nil && q.pointer && !q.anchor.Contains(view.X+x, view.Y+y) && a.hoverPointerInside(q, view.X+x, view.Y+y) {
		return nil
	}
	changed := p.ed != d || !p.hit || p.x != x || p.y != y || p.version != version || p.view != view ||
		p.scrollX != d.hscroll.X || p.scrollY != d.vscroll.Y
	if changed {
		hit, ok := a.semanticPointAt(d, x, y)
		same := ok && p.hit && p.ed == d && p.version == version && p.point.start == hit.start && p.point.end == hit.end &&
			p.view == view && p.scrollX == d.hscroll.X && p.scrollY == d.vscroll.Y
		moved := p.x != x || p.y != y
		suppressed := p.suppressed && !moved && p.ed == d
		if !same {
			// The card and the short bridge to it belong to the current target.
			if q := a.hoverQuery; q != nil && q.pointer && a.hoverPointerInside(q, view.X+x, view.Y+y) {
				return nil
			}
			a.dismissHover()
			*p = editorPointerHover{ed: d, point: hit, hit: ok, since: c.Now(), suppressed: suppressed}
		} else if p.suppressed && moved {
			p.suppressed, p.attempted, p.since = false, false, c.Now()
		}
		p.point = hit
		if q := a.hoverQuery; same && hovered && q != nil && q.pointer {
			q.anchor = hit.anchor
			q.anchor.X, q.anchor.Y = q.anchor.X+view.X, q.anchor.Y+view.Y
		}
		p.x, p.y, p.view, p.version = x, y, view, version
		p.scrollX, p.scrollY = d.hscroll.X, d.vscroll.Y
	}
	p.seen, p.over = a.builds, p.hit && hovered
	if !p.hit {
		return nil
	}
	// Hovered follows the previous hit chain, which can lag a newly laid-out
	// text area. Start the dwell from the geometric hit, then require the
	// topmost hit before offering a card or a navigation cursor.
	if !hovered {
		if !p.suppressed && !p.attempted {
			c.After(max(motionHover, hoverDwell-c.Now().Sub(p.since)))
		}
		return nil
	}
	if c.Modifiers() == ui.Cmd {
		area.Cursor(ui.CursorPointer)
		if a.hoverQuery != nil && a.hoverQuery.pointer {
			a.dismissHover()
		}
		p.attempted, p.since = false, c.Now()
		return &p.point.anchor
	}
	if c.Modifiers() != 0 || p.suppressed || p.attempted || a.definitionPending != nil {
		return nil
	}
	if remaining := hoverDwell - c.Now().Sub(p.since); remaining > 0 {
		c.After(remaining)
		return nil
	}
	p.attempted = true
	anchor := p.point.anchor
	anchor.X, anchor.Y = anchor.X+view.X, anchor.Y+view.Y
	n := lspNavigation{doc: d.document, pos: positionOf(d.buf.Snapshot(), p.point.at), tab: a.activeTab(),
		root: a.root, path: d.path, version: version, serial: a.navigationSerial}
	a.startHover(n, true, anchor)
	return nil
}

func (a *app) hoverCurrent(q *hoverQuery) bool {
	if a.hoverQuery != q || a.blurred || a.semanticOverlayOpen() || !a.navigationCurrent(q.nav) {
		return false
	}
	return q.ed == nil || q.ed.sel == q.sel && q.ed.vscroll.Y == q.scrollY &&
		q.ed.hscroll.X == q.scrollX && q.ed.lastView == q.view
}

func (a *app) navigationAnchor(n lspNavigation) ui.Rect {
	if d := a.activeEditor(); d != nil && d.document == n.doc {
		at := posRune(d.buf.Snapshot(), n.pos)
		if x, y, h, ok := d.lines.Caret(at); ok {
			return ui.Rect{X: d.lastView.X + d.textX + x - d.hscroll.X,
				Y: d.lastView.Y + editorPadY + y - d.vscroll.Y, W: a.font.charW, H: h}
		}
	}
	return ui.Rect{X: 40, Y: titleBarH + tabStripH + crumbsH, W: 1, H: a.font.lineH}
}

func (a *app) startHover(n lspNavigation, pointer bool, anchor ui.Rect) {
	a.dismissHover()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	q := &hoverQuery{nav: n, pointer: pointer, cancel: cancel, anchor: anchor, started: a.frameAt}
	if d := a.activeEditor(); d != nil && d.document == n.doc {
		q.ed, q.sel, q.view, q.scrollX, q.scrollY = d, d.sel, d.lastView, d.hscroll.X, d.vscroll.Y
	}
	a.hoverQuery = q
	for _, dg := range plugins.diagnosticsOf(n.path) {
		start, end := posRune(n.doc.buf.Snapshot(), dg.Range.Start), posRune(n.doc.buf.Snapshot(), dg.Range.End)
		if start > end {
			start, end = end, start
		}
		at := posRune(n.doc.buf.Snapshot(), n.pos)
		if at >= start && at < max(start+1, end) {
			q.diagnostics = append(q.diagnostics, dg)
		}
	}
	lang, uri := languageOf(n.doc), serverURI(n.path)
	var providers []*plugin.Plugin
	if plugins != nil {
		for _, p := range plugins.enabled() {
			if slices.Contains(p.HoverLanguages(), lang) {
				providers = append(providers, p)
			}
		}
	}
	s := a.lspFor(n.doc, false)
	q.parts = make([]string, len(providers)+1)
	q.pending = len(providers)
	if s != nil {
		q.pending++
	}
	// Each provider publishes independently, so a slow extension cannot hide
	// documentation that has already arrived from the language server.
	deliver := func(slot int, text string) {
		a.post(func() {
			if !a.hoverCurrent(q) {
				return
			}
			q.parts[slot] = strings.TrimSpace(text)
			q.pending--
			a.updateHover(q)
		})
	}
	if len(providers) > 0 {
		doc := docJSON(n.doc)
		for i, p := range providers {
			go func() {
				res, err := p.Request(ctx, "hover", map[string]any{"language": lang, "document": doc, "position": n.pos})
				var text string
				if err == nil {
					json.Unmarshal(res, &text)
				}
				deliver(i+1, text)
			}()
		}
	}
	if s != nil {
		go func() {
			select {
			case <-ctx.Done():
				deliver(0, "")
				return
			case <-s.ready:
			}
			a.post(func() {
				if !a.hoverCurrent(q) {
					return
				}
				client := s.live()
				if client == nil || !client.Hover() || ctx.Err() != nil {
					deliver(0, "")
					return
				}
				synced := client.OpenAsync(ctx, uri, lang, n.doc.buf.String())
				go func() {
					if err := <-synced; err != nil {
						deliver(0, "")
						return
					}
					text, _ := client.HoverAt(ctx, uri, lsp.Position(n.pos))
					deliver(0, text)
				}()
			})
		}()
	}
	a.updateHover(q)
}

func (a *app) updateHover(q *hoverQuery) {
	parts := make([]string, 0, len(q.parts))
	for _, text := range q.parts {
		if text != "" && !slices.Contains(parts, text) {
			parts = append(parts, text)
		}
	}
	if len(parts) > 0 || len(q.diagnostics) > 0 {
		a.hover = &hoverCard{tree: &viewNode{Type: "markdown", Text: strings.Join(parts, "\n\n---\n\n")}}
	} else if q.pending == 0 && !q.pointer {
		a.tell(note{message: locales.HoverNone()})
	}
	if q.pending == 0 {
		q.cancel()
	}
	a.invalidate()
}

func (a *app) hoverPointerInside(q *hoverQuery, x, y float32) bool {
	if q.anchor.Contains(x, y) || q.box.Contains(x, y) {
		return true
	}
	if q.box.W <= 0 {
		return false
	}
	// A narrow bridge lets the pointer cross the gap without making nearby
	// code part of the hover target.
	top, bottom := q.anchor.Y+q.anchor.H, q.box.Y
	if q.box.Y < q.anchor.Y {
		top, bottom = q.box.Y+q.box.H, q.anchor.Y
	}
	return x >= q.anchor.X && x <= q.anchor.X+q.anchor.W && y >= top && y <= bottom
}

func (a *app) hoverView(c *ui.Context) {
	q := a.hoverQuery
	if q != nil && !a.hoverCurrent(q) {
		a.suppressHover()
		q = nil
	}
	if n := a.definitionPending; n != nil {
		if !a.navigationCurrent(*n) || a.blurred || a.semanticOverlayOpen() {
			a.cancelDefinition()
		} else {
			a.semanticCard(c, a.navigationAnchor(*n), nil, func() {
				ui.Text(c, locales.DefinitionLoading()).FontSize(12).TextColor(a.pal.muted)
			}, a.cancelDefinition)
		}
		return
	}
	if q == nil {
		return
	}
	if q.pointer {
		x, y, inside := c.Root().PointerPosition()
		p := &a.pointerHover
		if inside && (a.hoverPointerInside(q, x, y) || p.seen == a.builds && p.over) {
			q.leftAt = time.Time{}
		} else {
			if q.leftAt.IsZero() {
				q.leftAt = c.Now()
			}
			if remaining := hoverLeave - c.Now().Sub(q.leftAt); remaining > 0 {
				c.After(remaining)
			} else {
				a.dismissHover()
				p.hit = false
				return
			}
		}
	}
	if a.hover == nil {
		if q.pending == 0 {
			return
		}
		if remaining := readingShows - c.Now().Sub(q.started); q.pointer && remaining > 0 {
			c.After(remaining)
			return
		}
	}
	a.semanticCard(c, q.anchor, &q.box, func() {
		for i, dg := range q.diagnostics {
			ui.Column(c.Key(i)).Gap(4).Children(func() {
				ui.Text(c, dg.Message).FontSize(13).TextColor(a.pal.diagColor(dg.Severity)).Selectable()
				if source := strings.TrimSpace(dg.source + " " + dg.Code); source != "" {
					ui.Text(c, source).FontSize(11).TextColor(a.pal.muted).Selectable()
				}
			})
		}
		if h := a.hover; h != nil && h.tree.Text != "" {
			a.drawViewNode(c, "hover-content", h.tree, nil)
		} else if len(q.diagnostics) == 0 {
			ui.Text(c, locales.Loading()).FontSize(12).TextColor(a.pal.muted)
		}
	}, a.suppressHover)
}

// semanticCard occupies only its own bounds, leaving the editor and its focus
// available. The popup flips above its symbol when there is more room there.
func (a *app) semanticCard(c *ui.Context, anchor ui.Rect, bounds *ui.Rect, content func(), close func()) {
	w, h := c.Size()
	width := min(float32(560), max(0, w-16))
	below, above := max(0, h-a.statusHeight()-anchor.Y-anchor.H-12), max(0, anchor.Y-titleBarH-12)
	showBelow := below >= min(float32(240), above)
	available := above
	if showBelow {
		available = below
	}
	maxH := min(float32(360), available)
	left := clamp(anchor.X, 8, max(8, w-width-8))
	ui.Overlay(c, func() {
		box := a.floating(c, ui.Column(c.Key("semantic-card")).Absolute().Left(left).Width(width).
			MaxHeight(maxH).Radius(a.pal.metricsOr().radiusCard).Clip().Role(ui.RoleTooltip).Label(locales.CmdGoHover()), false)
		if showBelow {
			box.Top(anchor.Y + anchor.H + 6)
		} else {
			box.Bottom(max(8, h-anchor.Y+6))
		}
		box.Children(func() {
			ui.Scroll(c.Key("semantic-content")).MaxHeight(maxH).Padding(12, 14).Gap(10).Background(a.pal.editor).Children(content)
		})
		if bounds != nil {
			*bounds = box.Bounds()
		}
		box.PointerPosition()
		if bounds != nil && box.PressedOutside() || box.OverlayShortcut(0, ui.KeyEscape) {
			close()
		}
	})
}

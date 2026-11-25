package main

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
)

type jumpHintKind uint8

const (
	jumpHintWords jumpHintKind = iota
	jumpHintLines
	jumpHintChar1
	jumpHintChar2
	jumpHintHelixWords
)

const hopHintAlphabet = "etovxqpdygfblzhckisuran"
const helixHintAlphabet = "abcdefghijklmnopqrstuvwxyz"

type jumpHintView struct {
	bounds      ui.Rect
	origin      ui.Point
	first, last int
	hidden      [][2]int
	focused     bool
	live        bool
	font        ui.Font
}

type jumpHintTarget struct {
	start, end int
	label      string
	box        ui.Rect
}

type jumpHintState struct {
	active         bool
	waiting        bool
	kind           jumpHintKind
	prefix         string
	query          []rune
	queryText      string
	queryInput     ui.Handle
	queryFocusNext bool
	queryFocused   bool
	queryReady     bool
	targets        []jumpHintTarget
	view           jumpHintView
	valid          bool
	basis          jumpHintView
	version        uint64
	mode           string
	vimMode        vimMode
	helixMode      helixMode
	selection      [2]int
	cursor         int
	scroll         ui.Point
}

func (state *jumpHintState) cancel() {
	state.queryInput.CancelFocus()
	*state = jumpHintState{view: state.view, valid: state.valid}
}

func (state *jumpHintState) clear() {
	state.queryInput.CancelFocus()
	*state = jumpHintState{}
}

func (state *jumpHintState) queryLength() int {
	switch state.kind {
	case jumpHintChar1:
		return 1
	case jumpHintChar2:
		return 2
	}
	return 0
}

func (state *jumpHintState) querying() bool {
	return state.active && len(state.query) < state.queryLength()
}

func (state *jumpHintState) current(app *app, doc *editor) bool {
	return doc.buf.Version() == state.version && editorMode(app.prefs()) == state.mode &&
		doc.vim.mode == state.vimMode && doc.helix.mode == state.helixMode &&
		doc.sel == state.selection && doc.vim.cursor == state.cursor &&
		doc.vscroll.Y == state.scroll.Y && doc.hscroll.X == state.scroll.X &&
		app.livePreview(doc) == state.basis.live && !doc.hidden && doc.preview == nil
}

func sameJumpHintView(before, after jumpHintView) bool {
	return before.bounds == after.bounds && before.origin == after.origin &&
		before.first == after.first && before.last == after.last &&
		before.live == after.live && before.font == after.font && slices.Equal(before.hidden, after.hidden)
}

func (app *app) startJumpHints(doc *editor, kind jumpHintKind) {
	if doc == nil || doc.buf == nil {
		return
	}
	state := &doc.jumpHints
	state.cancel()
	mode := editorMode(app.prefs())
	if !state.valid || doc.hidden || doc.preview != nil ||
		(kind == jumpHintHelixWords && (mode != "helix" || doc.helix.mode != helixNormal)) ||
		(kind != jumpHintHelixWords && (mode != "vim" || doc.vim.mode == vimInsert)) {
		return
	}
	state.waiting, state.kind, state.mode = true, kind, mode
	state.vimMode, state.helixMode = doc.vim.mode, doc.helix.mode
	doc.vim.keys, doc.helix.keys = nil, nil
	app.invalidate()
}

func (app *app) activateJumpHints(doc *editor) {
	state := &doc.jumpHints
	state.waiting, state.active = false, true
	state.version = doc.buf.Version()
	state.selection, state.cursor = doc.sel, doc.vim.cursor
	state.scroll = ui.Point{X: doc.hscroll.X, Y: doc.vscroll.Y}
	state.basis = state.view
	state.basis.hidden = slices.Clone(state.view.hidden)
	if state.kind != jumpHintChar1 && state.kind != jumpHintChar2 {
		app.gatherJumpHints(doc)
	} else {
		state.queryFocusNext = true
	}
	app.invalidate()
}

func (app *app) gatherJumpHints(doc *editor) {
	state := &doc.jumpHints
	snap := doc.buf.Snapshot()
	targets := jumpHintCandidates(snap, state.view.first, state.view.last, state.kind, state.query)
	kept := targets[:0]
	for _, target := range targets {
		if jumpHintHidden(state.view.hidden, target.start, target.end) {
			continue
		}
		box, ok := jumpHintBox(doc, state.view, target.start)
		if !ok {
			continue
		}
		target.box = box
		kept = append(kept, target)
	}
	cursor := vimCursor(doc, snap)
	if state.kind == jumpHintHelixWords {
		cursor = helixCursor(doc, snap)
	}
	state.targets = orderJumpHints(kept, cursor, state.kind)
	if len(state.targets) == 0 {
		state.cancel()
	}
}

func jumpHintCandidates(snap ui.TextSnapshot, first, last int, kind jumpHintKind, query []rune) []jumpHintTarget {
	if (kind == jumpHintChar1 || kind == jumpHintChar2) && len(query) == 0 {
		return nil
	}
	var targets []jumpHintTarget
	for line := max(first, 0); line <= min(last, snap.LineCount()-1); line++ {
		start, end := snap.LineRange(line)
		text := []rune(snap.Slice(start, end))
		switch kind {
		case jumpHintLines:
			for index, glyph := range text {
				if !unicode.IsSpace(glyph) {
					targets = append(targets, jumpHintTarget{start: start + index, end: start + index + 1})
					break
				}
			}
		case jumpHintChar1, jumpHintChar2:
			for index := 0; index+len(query) <= len(text); index++ {
				if slices.Equal(text[index:index+len(query)], query) {
					targets = append(targets, jumpHintTarget{start: start + index, end: start + index + len(query)})
				}
			}
		default:
			for index := 0; index < len(text); {
				if !jumpHintWordRune(text[index]) || unicode.IsMark(text[index]) {
					index++
					continue
				}
				wordStart, graphemes := index, 0
				for index < len(text) && jumpHintWordRune(text[index]) {
					if !unicode.IsMark(text[index]) {
						graphemes++
					}
					index++
				}
				if kind != jumpHintHelixWords || graphemes >= 2 {
					targets = append(targets, jumpHintTarget{start: start + wordStart, end: start + index})
				}
			}
		}
	}
	return targets
}

func jumpHintWordRune(glyph rune) bool {
	return glyph == '_' || unicode.IsLetter(glyph) || unicode.IsDigit(glyph) || unicode.IsMark(glyph)
}

func jumpHintHidden(hidden [][2]int, start, end int) bool {
	for _, span := range hidden {
		if start < span[1] && end > span[0] {
			return true
		}
	}
	return false
}

func jumpHintVisible(bounds, box ui.Rect) bool {
	return bounds.W > 0 && bounds.H > 0 && box.H > 0 &&
		box.X >= bounds.X && box.X < bounds.X+bounds.W &&
		box.Y >= bounds.Y && box.Y+box.H <= bounds.Y+bounds.H
}

func jumpHintBox(doc *editor, view jumpHintView, offset int) (ui.Rect, bool) {
	left, top, height, ok := doc.lines.Caret(offset)
	box := ui.Rect{X: view.origin.X + left, Y: view.origin.Y + top, W: 1, H: height}
	return box, ok && jumpHintVisible(view.bounds, box)
}

func jumpHintsPlaced(doc *editor, targets []jumpHintTarget, view jumpHintView) bool {
	for _, target := range targets {
		box, ok := jumpHintBox(doc, view, target.start)
		if !ok || box != target.box {
			return false
		}
	}
	return true
}

func orderJumpHints(targets []jumpHintTarget, cursor int, kind jumpHintKind) []jumpHintTarget {
	if kind == jumpHintHelixWords {
		forward := sort.Search(len(targets), func(index int) bool { return targets[index].end > cursor })
		backward := forward - 1
		ordered := make([]jumpHintTarget, 0, min(len(targets), len(helixHintAlphabet)*len(helixHintAlphabet)))
		for len(ordered) < cap(ordered) {
			if forward < len(targets) {
				ordered = append(ordered, targets[forward])
				forward++
			}
			if backward >= 0 && len(ordered) < cap(ordered) {
				ordered = append(ordered, targets[backward])
				backward--
			}
		}
		for index := range ordered {
			ordered[index].label = jumpHintLabel(index, 2, helixHintAlphabet)
		}
		return ordered
	}
	sort.SliceStable(targets, func(left, right int) bool {
		leftDistance, rightDistance := abs(targets[left].start-cursor), abs(targets[right].start-cursor)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		return targets[left].start > targets[right].start
	})
	labels := hopHintLabels(len(targets))
	for index := range targets {
		targets[index].label = labels[index]
	}
	return targets
}

func hopHintLabels(count int) []string {
	if count <= 0 {
		return nil
	}
	base, length := len(hopHintAlphabet), 1
	capacity := base
	for capacity < count {
		capacity *= base
		length++
	}
	short := 0
	if length > 1 {
		short = (capacity - count) / (base - 1)
	}
	labels := make([]string, 0, count)
	for index := range short {
		labels = append(labels, jumpHintLabel(index, length-1, hopHintAlphabet))
	}
	for index := 0; index < count-short; index++ {
		labels = append(labels, jumpHintLabel(short*base+index, length, hopHintAlphabet))
	}
	return labels
}

func jumpHintLabel(index, length int, alphabet string) string {
	label := make([]byte, length)
	for position := length - 1; position >= 0; position-- {
		label[position] = alphabet[index%len(alphabet)]
		index /= len(alphabet)
	}
	return string(label)
}

func (app *app) jumpHintInput(doc *editor, event ui.InputEvent) bool {
	state := &doc.jumpHints
	if !state.active && !state.waiting {
		return false
	}
	switch event.Kind {
	case ui.InputPointerDown, ui.InputScroll, ui.InputZoom, ui.InputSelect:
		if state.queryFocused && (event.Kind == ui.InputScroll || event.Kind == ui.InputZoom) {
			doc.focusNext = true
		}
		state.cancel()
		app.invalidate()
		return false
	}
	if state.waiting {
		if event.Kind == ui.InputKeyDown && (event.Key == ui.KeyEscape || event.Key == ui.KeyBracketLeft && event.Mods == ui.Ctrl) {
			state.cancel()
			app.invalidate()
		}
		return event.Kind == ui.InputKeyDown || event.Kind == ui.InputText || event.Kind == ui.InputCompose || event.Kind == ui.InputCommand
	}
	if !state.current(app, doc) {
		state.cancel()
		app.invalidate()
		return event.Kind == ui.InputKeyDown || event.Kind == ui.InputText || event.Kind == ui.InputCompose || event.Kind == ui.InputCommand
	}
	switch event.Kind {
	case ui.InputKeyDown:
		if event.Key == ui.KeyEscape || event.Key == ui.KeyBracketLeft && event.Mods == ui.Ctrl {
			state.cancel()
		} else if event.Key == ui.KeyBackspace && event.Mods == 0 {
			if len(state.prefix) > 0 {
				state.prefix = state.prefix[:len(state.prefix)-1]
			} else if len(state.query) > 0 {
				state.query = state.query[:len(state.query)-1]
				state.queryText = string(state.query)
				state.queryFocusNext, state.queryReady = true, false
				state.targets = nil
			}
		} else if state.querying() || state.queryReady {
			state.queryFocusNext = state.querying()
		} else if event.Mods&(ui.Ctrl|ui.Alt|ui.Super) != 0 {
			state.cancel()
		} else if event.Key != ui.KeyUnknown || event.PhysicalKey != ui.KeyUnknown {
			key := event.Key
			if key == ui.KeyUnknown {
				key = event.PhysicalKey
			}
			_, punctuation := vimPunct[key]
			printable := key >= ui.KeyA && key <= ui.KeyZ || key >= ui.Key0 && key <= ui.Key9 || key == ui.KeySpace || punctuation
			if glyph, ok := vimRune(event, false); printable && ok {
				app.typeJumpHint(doc, glyph)
			} else {
				state.cancel()
			}
		}
		app.invalidate()
		return true
	case ui.InputText:
		if app.held == nil && !state.querying() && !state.queryReady {
			for _, glyph := range event.Text {
				app.typeJumpHint(doc, glyph)
				if !state.active {
					break
				}
			}
		}
		app.invalidate()
		return true
	case ui.InputCompose, ui.InputCommand:
		return true
	}
	return false
}

func (app *app) typeJumpHint(doc *editor, glyph rune) {
	state := &doc.jumpHints
	if state.querying() || state.queryReady {
		return
	}
	state.prefix += string(glyph)
	matched := false
	for _, target := range state.targets {
		if target.label == state.prefix {
			app.acceptJumpHint(doc, target)
			return
		}
		matched = matched || strings.HasPrefix(target.label, state.prefix)
	}
	if !matched {
		state.cancel()
	}
}

func (app *app) acceptJumpHint(doc *editor, target jumpHintTarget) {
	kind := doc.jumpHints.kind
	doc.jumpHints.cancel()
	if kind == jumpHintHelixWords {
		helixSelect(doc, target.start, target.end-1)
		return
	}
	snap := doc.buf.Snapshot()
	from := vimCursor(doc, snap)
	if target.start != from {
		vimPushJump(doc, from)
	}
	doc.vim.hasWant = false
	if doc.vim.mode.visual() {
		doc.vim.cursor = target.start
		app.vimShowVisual(doc, doc.vim.mode)
	} else {
		vimPut(doc, vimClamp(snap, target.start))
	}
}

func (app *app) paintJumpHints(doc *editor, painter *ui.Painter, view jumpHintView) {
	state := &doc.jumpHints
	if state.waiting {
		state.view, state.valid = view, view.bounds.W > 0 && view.bounds.H > 0 && view.first <= view.last
		if !view.focused || editorMode(app.prefs()) != state.mode || doc.vim.mode != state.vimMode || doc.helix.mode != state.helixMode {
			state.cancel()
			painter.AnimationFrame()
			return
		}
		if len(doc.cmds) != 0 || doc.pending != nil || doc.areaInput {
			painter.AnimationFrame()
			return
		}
		app.activateJumpHints(doc)
	}
	if state.active && (!view.focused || !state.current(app, doc) || !sameJumpHintView(state.basis, view) || !jumpHintsPlaced(doc, state.targets, view)) {
		if view.focused && state.queryFocused {
			doc.focusNext = true
		}
		state.cancel()
		painter.AnimationFrame()
	}
	state.view, state.valid = view, view.bounds.W > 0 && view.bounds.H > 0 && view.first <= view.last
	if state.active && state.queryReady {
		state.queryReady = false
		app.gatherJumpHints(doc)
		if !state.active {
			doc.focusNext = true
			painter.AnimationFrame()
		}
	}
	if !state.active {
		return
	}
	palette := app.pal
	fontSize := view.font.Size
	if fontSize == 0 {
		fontSize = 14
	}
	span := ui.Span{Font: view.font.Family, Size: fontSize, Weight: 600, Color: palette.fillText, Features: view.font.Features}
	painter.Clip(view.bounds, 0, func() {
		for _, target := range state.targets {
			if !strings.HasPrefix(target.label, state.prefix) {
				continue
			}
			span.Text = target.label[len(state.prefix):]
			matched := span
			matched.Text, matched.Color = state.prefix, palette.fillText.Alpha(0.55)
			width, height := painter.MeasureText(0, matched, span)
			box := ui.Rect{W: width + 4, H: max(height+2, target.box.H)}
			box.X = max(view.bounds.X, min(target.box.X-2, view.bounds.X+view.bounds.W-box.W))
			box.Y = max(view.bounds.Y, min(target.box.Y, view.bounds.Y+view.bounds.H-box.H))
			painter.Fill(box, palette.fill, 2)
			painter.RichText(box.X+2, box.Y+(box.H-height)/2, 0, matched, span)
		}
	})
}

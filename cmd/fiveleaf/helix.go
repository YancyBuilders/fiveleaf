package main

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
)

// helixMode is the mode of an editor in helix mode.
type helixMode uint8

const (
	helixNormal helixMode = iota
	helixInsert
)

func (m helixMode) String() string {
	if m == helixInsert {
		return "INSERT"
	}
	return "NORMAL"
}

// helixState is an editor's state of helix mode. Its zero value is normal
// mode at the start of the text, with that rune selected.
type helixState struct {
	mode helixMode
	// anchor and cursor are the runes at either end of the selection, both
	// included. The selection is never empty in normal mode.
	anchor, cursor int
	// started is set once the first frame selected a rune, as Helix's
	// normal mode does.
	started bool
	// keys are the keys of a command typed so far.
	keys []rune
	// yank is the text yanked last, and whether it is whole lines. flash is
	// where that text shows for a moment from flashAt.
	yank      string
	yankLines bool
	flash     [2]int
	flashAt   time.Time
}

// editorMode is the normal mode of the editors: "vim", "helix", or "".
// The older `vim` setting still means vim.
func editorMode(p *settings.Prefs) string {
	switch p.EditorMode {
	case "vim", "helix":
		return p.EditorMode
	}
	if p.Vim {
		return "vim"
	}
	return ""
}

// helixOn reports whether the editor mode is Helix's.
func (a *app) helixOn() bool { return editorMode(a.prefs()) == "helix" }

// helixTakesKeys reports whether the editor with the keyboard takes its
// keys as Helix's commands.
func (a *app) helixTakesKeys(c *ui.Context) bool {
	d := a.activeEditor()
	return a.helixOn() && d != nil && d.preview == nil && d.helix.mode != helixInsert && d.vscroll.Focused(c)
}

// helixKey takes a key of normal or insert mode, reporting whether it took
// ev.
func (a *app) helixKey(d *editor, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	if d.helix.mode == helixInsert {
		return a.helixInsertKey(d, ev)
	}
	r, ok := vimRune(ev, a.vimCtrl())
	if !ok {
		return false
	}
	d.helix.keys = append(d.helix.keys, r)
	cmd, st := parseHelix(d.helix.keys)
	switch st {
	case vimMore:
		return true
	case vimBad:
		d.helix.keys = d.helix.keys[:0]
		return true
	}
	d.helix.keys = d.helix.keys[:0]
	a.helixRun(d, cmd)
	return true
}

// helixInsertKey takes the keys of insert mode that are Helix's rather
// than typing: Esc, and Control's word deletes where Control is not the
// platform's key of shortcuts.
func (a *app) helixInsertKey(d *editor, ev ui.InputEvent) bool {
	ctrl := ev.Mods == ui.Ctrl && ui.Cmd != ui.Ctrl
	switch {
	case ev.Mods == 0 && ev.Key == ui.KeyEscape, ev.Mods == ui.Ctrl && ev.Key == ui.KeyBracketLeft,
		ctrl && ev.Key == ui.KeyC:
		d.helix.mode = helixNormal
		a.queueEdit(d, a.helixEndInsert)
		return true
	case ctrl && (ev.Key == ui.KeyW || ev.Key == ui.KeyU):
		word := ev.Key == ui.KeyW
		a.queueEdit(d, func(d *editor) { vimDeleteBack(d, word) })
		return true
	}
	return false
}

// parseHelix reads a count and a key of normal mode, or g and the key
// after it. r waits for the character it replaces the selection with.
func parseHelix(keys []rune) (vimCmd, int) {
	var c vimCmd
	i := 0
	for i < len(keys) && keys[i] >= '0' && keys[i] <= '9' && (c.count > 0 || keys[i] != '0') {
		c.count = c.count*10 + int(keys[i]-'0')
		i++
	}
	if i == len(keys) {
		return c, vimMore
	}
	k := keys[i]
	i++
	switch k {
	case 'g':
		if i == len(keys) {
			return c, vimMore
		}
		name := "g" + string(keys[i])
		switch name {
		case "gg", "ge", "gh", "gl", "gw", "gd":
			c.key = name
			return c, vimDone
		}
		return c, vimBad
	case 'r':
		if i == len(keys) {
			return c, vimMore
		}
		if keys[i] < ' ' || keys[i] == '\x7f' {
			return c, vimBad
		}
		c.key, c.arg = "r", keys[i]
		return c, vimDone
	}
	if strings.ContainsRune("hjklwbeWBE0$^GxXDCYypPoOiIaAscSuU.~<>{}[]()%", k) || k == ' ' {
		c.key = string(k)
		return c, vimDone
	}
	return c, vimBad
}

// helixSpan is the selected runes, both ends included, as offsets from up
// to to, the cursor at the start or the end.
func helixSpan(d *editor) (from, to int, forward bool) {
	lo, hi := min(d.sel[0], d.sel[1]), max(d.sel[0], d.sel[1])
	return lo, hi, d.sel[1] >= d.sel[0]
}

func helixCursor(d *editor, snap ui.TextSnapshot) int {
	cursor := d.sel[1]
	if cursor > d.sel[0] {
		cursor--
	}
	return min(max(cursor, 0), max(snap.Len()-1, 0))
}

func helixBlockVisible(d *editor, snap ui.TextSnapshot) bool {
	start, end := snap.LineRange(snap.LineAt(helixCursor(d, snap)))
	return !hasRTL(snap.Slice(start, end))
}

// helixSelect selects from anchor to cursor, both runes included. The
// cursor stays where it was put, at the start or the end.
func helixSelect(d *editor, anchor, cursor int) {
	snap := d.buf.Snapshot()
	n := snap.Len()
	anchor, cursor = min(max(anchor, 0), n), min(max(cursor, 0), n)
	if n == 0 {
		d.helix.anchor, d.helix.cursor = 0, 0
		d.selectRange(0, 0, false)
		d.sel = [2]int{0, 0}
		return
	}
	anchor, cursor = min(anchor, n-1), min(cursor, n-1)
	for anchor > 0 {
		_, e := snap.LineRange(snap.LineAt(anchor))
		if anchor != e {
			break
		}
		anchor--
	}
	from, to := min(anchor, cursor), min(max(anchor, cursor)+1, n)
	d.helix.anchor, d.helix.cursor = anchor, cursor
	if cursor < anchor {
		d.selectRange(to, from, false)
		d.sel = [2]int{to, from}
		return
	}
	d.selectRange(from, to, false)
	d.sel = [2]int{from, to}
}

// helixStartInsert begins insert mode at the start or the end of the
// selection, its caret there and nothing selected.
func (a *app) helixStartInsert(d *editor, at int) {
	d.helix.mode = helixInsert
	at = min(max(at, 0), d.buf.Snapshot().Len())
	d.selectRange(at, at, false)
	d.sel = [2]int{at, at}
	d.focusNext = true
	a.invalidate()
}

// helixEndInsert leaves insert mode with the rune before the caret
// selected, as Helix does.
func (a *app) helixEndInsert(d *editor) {
	d.observe()
	at := min(d.sel[1], d.buf.Snapshot().Len())
	if at > 0 {
		at--
	}
	helixSelect(d, at, at)
	d.hist.group, d.hist.typing = groupNone, false
}

// helixRun runs a command of normal mode.
func (a *app) helixRun(d *editor, c vimCmd) {
	snap := d.buf.Snapshot()
	h := &d.helix
	cnt := max(c.count, 1)
	line := snap.LineAt(h.cursor)
	ls, le := snap.LineRange(line)
	switch c.key {
	case "gw":
		a.startJumpHints(d, jumpHintHelixWords)
	case "gd":
		a.goToDefinition()
	case "h":
		helixSelect(d, h.cursor-cnt, h.cursor-cnt)
	case "l":
		helixSelect(d, h.cursor+cnt, h.cursor+cnt)
	case "j", "k":
		by := cnt
		if c.key == "k" {
			by = -cnt
		}
		col := vimCol(snap, h.cursor)
		at := vimAtCol(snap, min(max(line+by, 0), vimLast(snap)), col)
		helixSelect(d, at, at)
	case "w":
		t := newVText(snap)
		to := h.cursor
		for range cnt {
			to = wordFwd(t, to, false)
		}
		helixSelect(d, to, to)
	case "b":
		t := newVText(snap)
		to := h.cursor
		for range cnt {
			to = wordBack(t, to, false)
		}
		helixSelect(d, to, to)
	case "e":
		t := newVText(snap)
		to := h.cursor
		for range cnt {
			to = wordEnd(t, to, false)
		}
		helixSelect(d, to, to)
	case "W", "B", "E":
		t := newVText(snap)
		to := h.cursor
		for range cnt {
			switch c.key {
			case "W":
				to = wordFwd(t, to, true)
			case "B":
				to = wordBack(t, to, true)
			case "E":
				to = wordEnd(t, to, true)
			}
		}
		helixSelect(d, to, to)
	case "0":
		helixSelect(d, ls, ls)
	case "^", "gh":
		helixSelect(d, firstNonBlank(snap, line), firstNonBlank(snap, line))
	case "$", "gl":
		helixSelect(d, max(le-1, ls), max(le-1, ls))
	case "G":
		helixSelect(d, firstNonBlank(snap, vimLast(snap)), firstNonBlank(snap, vimLast(snap)))
	case "gg":
		helixSelect(d, firstNonBlank(snap, 0), firstNonBlank(snap, 0))
	case "ge":
		t := newVText(snap)
		to := h.cursor
		for range cnt {
			to = wordEndBack(t, to, false)
		}
		helixSelect(d, to, to)
	case "i":
		from, _, _ := helixSpan(d)
		a.helixStartInsert(d, from)
	case "a":
		_, to, _ := helixSpan(d)
		a.helixStartInsert(d, to)
	case "I":
		a.helixStartInsert(d, firstNonBlank(snap, line))
	case "A":
		a.helixStartInsert(d, le)
	case "o":
		a.helixOpenLine(d, true)
	case "O":
		a.helixOpenLine(d, false)
	case "x":
		helixDeleteSelection(d)
	case "s":
		from, to, _ := helixSpan(d)
		d.helix.yank, d.helix.yankLines = snap.Slice(from, to), false
		d.edit([2]int{from, from}, textEdit{from, to, ""})
		a.helixStartInsert(d, from)
	case "c":
		from, to, _ := helixSpan(d)
		d.helix.yank, d.helix.yankLines = snap.Slice(from, to), false
		d.edit([2]int{from, from}, textEdit{from, to, ""})
		a.helixStartInsert(d, from)
	case "d":
		helixDeleteSelection(d)
	case "D":
		from, to, _ := helixSpan(d)
		_, e := snap.LineRange(snap.LineAt(to))
		d.edit([2]int{from, from}, textEdit{from, e, ""})
		helixSelect(d, min(from, max(d.buf.Len()-1, 0)), min(from, max(d.buf.Len()-1, 0)))
	case "y":
		from, to, _ := helixSpan(d)
		h.yank, h.yankLines = snap.Slice(from, to), false
		h.flash, h.flashAt = [2]int{from, to}, time.Now()
	case "Y":
		from, to, _ := helixSpan(d)
		s, _ := snap.LineRange(snap.LineAt(from))
		_, e := snap.LineRange(snap.LineAt(to))
		h.yank, h.yankLines = snap.Slice(s, e)+"\n", true
		h.flash, h.flashAt = [2]int{s, min(e, snap.Len())}, time.Now()
	case "p":
		a.helixPaste(d, false)
	case "P":
		a.helixPaste(d, true)
	case "r":
		from, to, _ := helixSpan(d)
		repl := strings.Map(func(r rune) rune {
			if r == '\n' {
				return r
			}
			return c.arg
		}, snap.Slice(from, to))
		d.edit([2]int{from, from}, textEdit{from, to, repl})
	case "~":
		from, to, _ := helixSpan(d)
		text := snap.Slice(from, to)
		if conv := toggleCase(text); conv != text {
			d.edit([2]int{from, from}, textEdit{from, to, conv})
		}
	case ">":
		d.sel = [2]int{ls, le}
		indentLines(d, false)
		if after := d.buf.Snapshot(); after.Len() > 0 {
			helixSelect(d, min(firstNonBlank(after, min(line, vimLast(after))), after.Len()-1),
				min(firstNonBlank(after, min(line, vimLast(after))), after.Len()-1))
		}
	case "<":
		d.sel = [2]int{ls, le}
		indentLines(d, true)
	case "{", "}":
		helixParagraph(d, c.key == "}")
	case "(", ")", "[", "]", "<>":
		helixSurround(d, c.key)
	case "%":
		if _, pair, ok := matchBracket(snap, h.cursor+1); ok {
			helixSelect(d, pair, pair)
		}
	case " ":
		a.openQuick("")
	case "u":
		for range cnt {
			if !d.undo() {
				break
			}
		}
		helixSelect(d, min(d.sel[0], max(d.buf.Len()-1, 0)), min(d.sel[0], max(d.buf.Len()-1, 0)))
	case "U":
		for range cnt {
			if !d.redo() {
				break
			}
		}
	}
}

// helixDeleteSelection deletes the selection and puts the next rune in
// its place, selected.
func helixDeleteSelection(d *editor) {
	snap := d.buf.Snapshot()
	from, to, _ := helixSpan(d)
	d.helix.yank, d.helix.yankLines = snap.Slice(from, to), false
	d.edit([2]int{from, from}, textEdit{from, to, ""})
	after := d.buf.Snapshot()
	helixSelect(d, min(from, max(after.Len()-1, 0)), min(from, max(after.Len()-1, 0)))
}

// helixPaste puts what was yanked after the selection, or before it.
func (a *app) helixPaste(d *editor, before bool) {
	h := &d.helix
	if h.yank == "" {
		return
	}
	snap := d.buf.Snapshot()
	from, to, _ := helixSpan(d)
	if h.yankLines {
		from, _ = snap.LineRange(snap.LineAt(from))
		if !before {
			_, to = snap.LineRange(snap.LineAt(min(to, snap.Len())))
		}
	}
	at := from
	if !before {
		at = min(to, snap.Len())
	}
	body := h.yank
	if h.yankLines && !before {
		body = "\n" + strings.TrimSuffix(body, "\n")
		at = to
	}
	n := utf8.RuneCountInString(body)
	d.edit([2]int{at + n, at + n}, textEdit{at, at, body})
	helixSelect(d, at, min(at+max(n-1, 0), max(d.buf.Len()-1, 0)))
}

// helixOpenLine opens a line below or above the selection, in insert
// mode.
func (a *app) helixOpenLine(d *editor, below bool) {
	snap := d.buf.Snapshot()
	s, e := snap.LineRange(snap.LineAt(d.helix.cursor))
	if below {
		d.edit([2]int{e, e}, textEdit{e, e, "\n"})
		a.helixStartInsert(d, e+1)
		return
	}
	d.edit([2]int{s, s}, textEdit{s, s, "\n"})
	a.helixStartInsert(d, s)
}

// helixParagraph selects the next, or previous, run of blank or non-blank
// lines, as { and } do.
func helixParagraph(d *editor, forward bool) {
	snap := d.buf.Snapshot()
	line := snap.LineAt(d.helix.cursor)
	last := vimLast(snap)
	blank := func(l int) bool {
		s, e := snap.LineRange(l)
		return strings.TrimSpace(snap.Slice(s, e)) == ""
	}
	kind := blank(line)
	if forward {
		for line < last {
			line++
			if blank(line) != kind {
				break
			}
		}
	} else {
		for line > 0 {
			line--
			if blank(line) != kind {
				break
			}
		}
	}
	helixSelect(d, firstNonBlank(snap, line), firstNonBlank(snap, line))
}

// helixSurround wraps the selection in a pair, or takes that pair off.
func helixSurround(d *editor, kind string) {
	open, close := kind[0], kind[0]
	if len(kind) == 1 {
		pair := map[byte][2]byte{'(': {'(', ')'}, ')': {'(', ')'}, '[': {'[', ']'}, ']': {'[', ']'}}[kind[0]]
		open, close = pair[0], pair[1]
	}
	snap := d.buf.Snapshot()
	from, to, _ := helixSpan(d)
	if from > 0 && to < snap.Len() && snap.Slice(from-1, from) == string(open) && snap.Slice(to, to+1) == string(close) {
		d.edit([2]int{to - 1, to}, textEdit{to, to + 1, ""}, textEdit{from - 1, from, ""})
		helixSelect(d, from-1, max(to-2, from-1))
		return
	}
	body := snap.Slice(from, to)
	d.edit([2]int{to + 2, to + 2}, textEdit{to, to, string(close)}, textEdit{from, from, string(open)})
	helixSelect(d, from+1, from+utf8.RuneCountInString(body))
}

// helixBadge shows helix mode's mode in the status bar.
func (a *app) helixBadge(c *ui.Context, h *helixState, text func(string) ui.Element) {
	p := a.pal
	m := p.metricsOr()
	bg, fg := p.text.Alpha(0.09), p.statusFg
	if h.mode == helixInsert {
		bg, fg = p.fill, p.fillText
	}
	row := ui.Row(c.Key("status:helix")).Height(a.statusHeight()).Padding(0, 6).Gap(6).AlignItems(ui.Center)
	row.Children(func() {
		ui.Text(c, h.mode.String()).FontSize(m.textSmall-1).FontWeight(650).LetterSpacing(0.6).TextColor(fg).SingleLine().
			Padding(1, 6).Radius(m.radius).Background(bg)
		if len(h.keys) > 0 {
			text(string(h.keys))
		}
	})
}

// helixReset leaves helix mode's state, as helix mode is turned off.
func helixReset(d *editor) {
	d.helix.mode = helixNormal
	d.helix.keys = d.helix.keys[:0]
}

// toggleHelix turns Helix's keys on, or off. Vim's keys go off with it.
func (a *app) toggleHelix() {
	a.setPref("editorMode", func(p *settings.Prefs) {
		if editorMode(p) == "helix" {
			p.EditorMode = ""
		} else {
			p.EditorMode, p.Vim = "helix", false
		}
	})
	a.invalidate()
}

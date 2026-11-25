package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/symbols"
	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

// quickOpen is the state of the quick open box: files by fuzzy name or
// by path, ":n" for a line of the document shown, or "@" for its symbols.
type quickOpen struct {
	syms     symbolCache
	open     bool
	query    string
	results  []workspace.Match
	commands []paletteEntry
	list     ui.ListState
	sel      int
	shown    string // the query the results are of
	path     string // the file the query shown names by its path, if any
	pathAt   [2]int // the line and column after that path, if any
	probe    quickPathProbe
	stale    bool
	moved    bool     // the choice changed, to scroll into view once
	used     []string // the commands the palette ran last, the last first
	// tools lists the tools alone, as the Toolbox button opens them, and
	// nothing else until the quick open closes.
	tools bool
	// shook is when Return found nothing to open, which shakes the box.
	shook time.Time
}

// resetResults reveals the new Return target after filtering, including
// when the old list was scrolled far below its first row.
func (q *quickOpen) resetResults(query string) {
	q.shown, q.stale, q.sel, q.moved = query, false, 0, true
	q.commands = nil
}

func (a *app) openQuick(query string) {
	a.closeIconThemePicker(false)
	q := &a.quick
	q.probe.epoch++
	q.open, q.query, q.sel, q.stale, q.tools = true, query, 0, true, false
	q.commands = nil
	if a.index == nil {
		a.indexRead.Invalidate()
	}
	a.refreshIndex()
}

func (a *app) closeQuick() {
	a.quick.probe.epoch++
	a.quick.open = false
	if a.focus != paneTerminal {
		a.focusColumn()
	}
}

// gotoLine parses ":n" or ":n:c", 1-based.
func gotoLine(query string) (line, col int, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(query), ":")
	if !found {
		return 0, 0, false
	}
	parts := strings.SplitN(rest, ":", 2)
	n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, rest == ""
	}
	if len(parts) == 2 {
		col, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	return n, col, true
}

// quickPathName is the file of this machine that a query names by an
// absolute or ~/ path, which quick open offers in place of the folder's
// matches once probeQuickPath finds it, with a :line or :line:column
// after it. The path may be quoted or shell-escaped, as pasted from a
// terminal. It is "" for any other query, or a folder on a host, whose
// paths are the host's. It only parses text; it must never touch the
// filesystem.
func quickPathName(root, query string) string {
	if remote.Is(root) {
		return ""
	}
	name := strings.TrimSpace(query)
	if len(name) >= 2 && (name[0] == '\'' || name[0] == '"') && name[len(name)-1] == name[0] {
		name = name[1 : len(name)-1]
	} else if filepath.Separator != '\\' && strings.Contains(name, `\`) {
		var b strings.Builder
		for i := 0; i < len(name); i++ {
			if name[i] == '\\' && i+1 < len(name) {
				i++
			}
			b.WriteByte(name[i])
		}
		name = b.String()
	}
	if rest, ok := strings.CutPrefix(name, "~"); ok && rest != "" && os.IsPathSeparator(rest[0]) {
		home := homeDir()
		if home == "" {
			return ""
		}
		name = filepath.Join(home, rest[1:])
	}
	if !filepath.IsAbs(name) {
		return ""
	}
	return filepath.Clean(name)
}

func (a *app) quickOpenView(c *ui.Context) {
	q := &a.quick
	a.refreshQuickPath()
	if !q.open {
		return
	}
	query := strings.TrimLeft(q.query, " ")
	if q.tools {
		a.quickTools(c, query)
		return
	}
	if symQuery, isSym := strings.CutPrefix(query, "@"); isSym {
		a.quickSymbols(c, symQuery)
		return
	}
	if cmdQuery, isCmd := strings.CutPrefix(query, ">"); isCmd {
		a.quickCommands(c, cmdQuery)
		return
	}
	if toolQuery, isTool := strings.CutPrefix(query, "$"); isTool {
		a.quickTools(c, toolQuery)
		return
	}
	if line, column, isLine := gotoLine(query); isLine {
		a.quickLine(c, line, column)
		return
	}
	a.quickFiles(c)
}

func (a *app) quickFiles(c *ui.Context) {
	q := &a.quick
	if q.stale || q.shown != q.query || q.commands != nil {
		q.path, q.pathAt = q.probe.result.name, q.probe.result.at
		if q.path != "" {
			q.results = []workspace.Match{{Index: 0}}
		} else {
			// A path not found yet, or not a file, still matches the
			// folder's files, which keep their place while it is checked.
			q.results = workspace.Fuzzy(a.index, q.query, 60)
			if strings.TrimSpace(q.query) == "" {
				q.results = a.recentFirst(q.results)
			}
		}
		q.resetResults(q.query)
	}
	files := a.index
	if q.path != "" {
		files = []string{filepath.ToSlash(shortenIn(q.path, homeDir()))}
	}
	// choose opens a file of the results, to the side with side.
	choose := func(i int, side bool) {
		// Bound input can change during this build, before the next refresh.
		if q.probe.key != a.quickPathKey() || q.probe.pending {
			return
		}
		if i < 0 || i >= len(q.results) {
			q.shook = c.Now()
			return
		}
		name := q.path
		if name == "" {
			name = inDir(a.root, files[q.results[i].Index])
		}
		q.open = false
		q.tools = false
		if side {
			a.openFileToSide(name)
		} else {
			a.openFile(name, true)
		}
		if line := q.pathAt[0]; q.path != "" && line > 0 {
			if d := a.activeEditor(); d != nil && d.path == name {
				a.goToLine(line, q.pathAt[1])
			}
		}
	}
	prompt := locales.QuickSearchFiles()
	if a.root == "" {
		prompt = locales.QuickTypeForCommands()
	}
	mode := quickMode{label: locales.QuickOpenLabel(), prompt: prompt, count: locales.FilesCount(len(q.results)),
		action: locales.QuickOpenAction(), enabled: len(q.results) > 0 && !q.probe.pending,
		choose: func(index int) { choose(index, false) }, openSide: func(index int) { choose(index, true) }}
	a.quickPicker(c, mode, func() { a.quickFileResults(c, files, mode.choose) })
}

func (a *app) quickFileResults(c *ui.Context, files []string, choose func(int)) {
	q := &a.quick
	if len(q.results) == 0 {
		msg := locales.QuickNoFiles()
		switch {
		case q.probe.pending:
			msg = locales.QuickPathChecking()
		case q.probe.result.err != nil:
			msg = locales.QuickPathError()
		case q.probe.name != "":
			msg = locales.QuickPathMissing()
		case a.root == "":
			msg = locales.QuickNoFolder()
		case a.indexRead.Running() && a.index == nil:
			msg = locales.QuickIndexing()
		}
		quickMessage(c, iconSearch, msg)
		return
	}
	items := make([]string, len(q.results))
	for index, match := range q.results {
		items[index] = files[match.Index]
	}
	metrics := a.pal.metricsOr()
	rowHeight := metrics.control + metrics.gap
	mark, muted := a.quickListColors(c)
	a.quickList(c, items, float32(len(items))*rowHeight, func(c *ui.Context, index int) {
		rel := items[index]
		a.quickResultRow(c, index, rel, rowHeight, mark, choose, func() {
			a.quickRow(c, rel, q.results[index].Positions, mark, muted)
		})
	})
}

func (a *app) quickLine(c *ui.Context, line, column int) {
	doc := a.activeEditor()
	mode := quickMode{label: locales.QuickOpenLabel(), prompt: locales.QuickLineHint(), prefix: ":",
		action: locales.QuickGoToLine(), enabled: doc != nil && line > 0}
	mode.choose = func(int) {
		if !mode.enabled {
			a.quick.shook = c.Now()
			return
		}
		a.quick.open = false
		a.goToLine(line, column)
	}
	a.quickPicker(c, mode, func() {
		var hint string
		switch {
		case doc == nil:
			hint = locales.QuickLineNoFile()
		case line <= 0:
			hint = locales.QuickLineRange(doc.buf.LineCount())
		case column > 0:
			hint = locales.QuickGoToLineCol(line, column)
		default:
			hint = locales.QuickGoToLineN(line)
		}
		metrics := a.pal.metricsOr()
		ui.Text(c, hint).FontSize(metrics.textBody+1).TextColor(a.pal.muted).Padding(metrics.inset, metrics.inset+metrics.pad)
	})
}

// quickRow shows a file of the results: its name with the characters the
// query matched in the accent color, and its folder.
func (a *app) quickRow(c *ui.Context, rel string, positions []int, mark, muted ui.Color) {
	p := a.pal
	metrics := p.metricsOr()
	base := strings.LastIndexByte(rel, '/') + 1
	icon, col := a.fileIcon(rel[base:])
	iconThemeView(c, p, icon, col, 15)
	ui.RichText(c, matchTextSpans(rel[base:], base, positions, p.text, mark)...).FontSize(metrics.textBody + 1).SingleLine().MinWidth(0)
	if dir := path.Dir(rel); dir != "." {
		ui.RichText(c, matchTextSpans(dir, 0, positions, muted, mark)...).FontSize(metrics.textSmall).SingleLine().MaxWidthPercent(45).MinWidth(0)
	}
}

// matchSpans is s, which starts at from of the text matched, with the
// characters at positions in the accent color. The positions are byte
// offsets, in order: walk them along the text.
func (a *app) matchSpans(s string, from int, positions []int, normal ui.Color) []ui.Span {
	return matchTextSpans(s, from, positions, normal, a.pal.accent)
}

func matchTextSpans(s string, from int, positions []int, normal, accent ui.Color) []ui.Span {
	if len(positions) == 0 && s != "" {
		return []ui.Span{{Text: s, Color: normal}}
	}
	out := make([]ui.Span, 0, 4)
	k := 0
	for i, r := range s {
		for k < len(positions) && positions[k] < from+i {
			k++
		}
		on := k < len(positions) && positions[k] == from+i
		if n := len(out); n > 0 && (out[n-1].Weight == 650) == on {
			out[n-1].Text += string(r)
			continue
		}
		sp := ui.Span{Text: string(r), Color: normal}
		if on {
			sp.Color, sp.Weight = accent, 650
		}
		out = append(out, sp)
	}
	return out
}

// goToLine moves the caret to a line (and column) of the document shown,
// 1-based.
func (a *app) goToLine(line, col int) {
	d := a.textEditor()
	if d == nil {
		return
	}
	d.live = false
	d.goToLine(line, col)
}

func (d *editor) goToLine(line, col int) {
	if d.unread() {
		// Loading may finish after the user has focused another view.
		d.loadedTarget = func(ed *editor) { ed.selectLine(line, col) }
		return
	}
	d.selectLine(line, col)
	d.vscroll.Focus()
}

// whenRead runs fn on the editor now, or once its file is read, in place
// of a target asked for before.
func (d *editor) whenRead(fn func(*editor)) {
	if d.unread() {
		d.loadedTarget = fn
		return
	}
	fn(d)
}

func (d *editor) selectLine(line, col int) {
	snap := d.buf.Snapshot()
	l := max(0, min(line-1, snap.LineCount()-1))
	s, e := snap.LineRange(l)
	at := min(s+max(col-1, 0), e)
	d.selectRange(at, at, true)
	d.lightLanding()
}

// recentFirst puts the files shown last before the others, as quick open
// lists them before anything is typed, the one shown now after the rest of
// them, as it is the one the user is leaving.
func (a *app) recentFirst(results []workspace.Match) []workspace.Match {
	// Find the recent files in the index, without a map of all of it.
	at := make(map[string]int, len(a.recent))
	for _, p := range a.recent {
		at[a.tree.Rel(p)] = -1
	}
	for i, f := range a.index {
		if j, ok := at[f]; ok && j < 0 {
			at[f] = i
		}
	}
	for f, i := range at {
		if i < 0 {
			delete(at, f)
		}
	}
	var out []workspace.Match
	seen := map[int]bool{}
	for n, p := range a.recent {
		if n == 0 && len(a.recent) > 1 {
			continue
		}
		if i, ok := at[a.tree.Rel(p)]; ok && !seen[i] {
			out = append(out, workspace.Match{Index: i})
			seen[i] = true
		}
	}
	if len(a.recent) > 1 {
		if i, ok := at[a.tree.Rel(a.recent[0])]; ok {
			out = append(out, workspace.Match{Index: i})
			seen[i] = true
		}
	}
	for _, m := range results {
		if !seen[m.Index] {
			out = append(out, m)
		}
	}
	return out[:min(len(out), 60)]
}

// quickSymbols is quick open going to a symbol of the document shown: its
// declarations, matched by name, in the order of the document until a name
// is typed.
func (a *app) quickSymbols(c *ui.Context, query string) {
	q := &a.quick
	syms, names := a.documentSymbols()
	key := "@" + query
	if q.stale || q.shown != key {
		q.results = workspace.Fuzzy(names, query, 200)
		q.resetResults(key)
	}
	choose := func(i int) {
		if i < 0 || i >= len(q.results) {
			q.shook = c.Now()
			return
		}
		q.open = false
		if d := a.activeEditor(); d != nil {
			sym := syms[q.results[i].Index]
			a.editActive(func(d *editor) {
				// To the line, with the name selected.
				a.jumpToScope(d, sym.Line)
				s, _ := d.buf.Snapshot().LineRange(sym.Line)
				d.selectRange(s+sym.Col, s+sym.Col+utf8.RuneCountInString(sym.Name), false)
			})()
		}
	}
	mode := quickMode{label: locales.SymbolLabel(), prompt: locales.SymbolPlaceholder(), prefix: "@",
		count: locales.ItemsCount(len(q.results)), action: locales.QuickGoToAction(), enabled: len(q.results) > 0, choose: choose}
	a.quickPicker(c, mode, func() {
		if a.activeEditor() == nil {
			quickMessage(c, iconHash, locales.SymbolNoFile())
			return
		}
		if len(q.results) == 0 {
			quickMessage(c, iconHash, locales.SymbolNoMatches())
			return
		}
		items := make([]string, len(q.results))
		for index, match := range q.results {
			items[index] = "@" + strconv.Itoa(match.Index)
		}
		metrics := a.pal.metricsOr()
		rowHeight := metrics.control + metrics.gap
		mark, muted := a.quickListColors(c)
		a.quickList(c, items, float32(len(items))*rowHeight, func(c *ui.Context, index int) {
			match := q.results[index]
			sym := syms[match.Index]
			a.quickResultRow(c, index, sym.Name, rowHeight, mark, choose, func() {
				a.symbolRow(c, sym, match.Positions, query == "", mark, muted)
			})
		})
	})
}

// symbolRow shows a symbol of the results: its kind, its name with the
// characters the query matched in the accent color, its declaration and
// its line. Listed in order, symbols are indented as in the document.
func (a *app) symbolRow(c *ui.Context, sym symbols.Symbol, positions []int, nested bool, mark, muted ui.Color) {
	p := a.pal
	metrics := p.metricsOr()
	if nested && sym.Depth > 0 {
		ui.Box(c).Width(float32(min(sym.Depth, 24)) * 4).Shrink(0)
	}
	icon, col := a.symbolIcon(sym.Kind)
	ui.Icon(c, icon).FontSize(14).TextColor(col)
	ui.RichText(c, matchTextSpans(sym.Name, 0, positions, p.text, mark)...).FontSize(metrics.textBody + 1).SingleLine().MinWidth(0)
	ui.Text(c, sym.Detail).FontSize(metrics.textSmall).Font(a.font.family).TextColor(muted).SingleLine().Grow(1).MinWidth(0)
	ui.Text(c, fmt.Sprint(sym.Line+1)).FontFeatures("tnum").FontSize(metrics.textSmall).TextColor(muted).SingleLine().Shrink(0)
}

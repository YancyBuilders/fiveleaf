package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/smallnest/uix/components/kbd"

	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

// paletteEntry is a command as the command palette lists it: its title,
// as "View: Toggle Sidebar", and the keys of its shortcut. en is its title
// in English, which the palette matches too, when the language shown is
// another; keywords are more words it matches, as a tool's features.
type paletteEntry struct {
	id, title, en, keywords string
	group                   int // the length of the title's "View: "
	shortcut                string
	// secondary names what the row shows after its title, in place of
	// its group, as the features of a tool.
	secondary string
}

func (e paletteEntry) category() string { return strings.TrimSuffix(e.title[:e.group], ": ") }

// commandGroup names the menu of a command, by the prefix of its id, in
// the language of t.
func commandGroup(t locales.Messages, id string) string {
	if strings.HasPrefix(id, toolboxCommand) {
		return t.ToolboxTitle()
	}
	switch {
	case strings.HasPrefix(id, "view.appearance."), strings.HasPrefix(id, "view.theme."):
		return t.MenuTheme()
	case strings.HasPrefix(id, localeCommand):
		return t.MenuLanguage()
	}
	switch id[:strings.IndexByte(id, '.')] {
	case "file":
		return t.MenuFile()
	case "edit":
		return t.MenuEdit()
	case "sel":
		return t.MenuSelection()
	case "markdown":
		return t.MenuMarkdown()
	case "go":
		return t.MenuGo()
	case "view", "editor", "image", "mermaid":
		return t.MenuView()
	case "task":
		return t.MenuTasks()
	case "debug":
		return t.MenuRun()
	case "agent":
		return t.AgentTitle()
	case "remote":
		return t.MenuRemoteSSH()
	case "git":
		return t.MenuGit()
	case "review":
		return t.MenuReview()
	case "help":
		return t.MenuHelp()
	case "plugins":
		return t.MenuPlugins()
	case "script":
		return t.PluginScripts()
	}
	if plugins != nil {
		if p := plugins.reg.CommandPlugin(id); p != nil {
			return p.Title()
		}
	}
	return ""
}

// groupTitle is a command's title in the palette: its group, then its
// label; the label alone for a command of no group.
func groupTitle(group, label string) string {
	if group == "" {
		return label
	}
	return group + ": " + label
}

// viewKeys are the keys of the commands that the editor or the view
// takes itself, which their menu items leave out, for the palette to show.
var viewKeys = map[string]string{
	"edit.undo": "CmdOrCtrl+Z", "edit.redo": "Shift+CmdOrCtrl+Z",
	"sel.moveUp": "Alt+Up", "sel.moveDown": "Alt+Down",
	"sel.copyUp": "Shift+Alt+Up", "sel.copyDown": "Shift+Alt+Down",
	"sel.lineAbove":          "Shift+CmdOrCtrl+Enter",
	"markdown.toggleTask":    "CmdOrCtrl+Enter",
	"file.openRecent":        "Ctrl+R",
	"sel.complete":           "Ctrl+Space",
	"debug.toggleBreakpoint": "F9",
}

// paletteEntries are the commands the palette runs, by title, made once
// for the window, and again when the language changes: the commands do
// not change.
func (a *app) paletteEntries() []paletteEntry {
	if a.palette != nil {
		return a.palette
	}
	shown, en := locales.In(i18n.Locale()), locales.In("en")
	english := map[string]string{}
	for _, cmd := range a.commandsIn(en) {
		english[cmd.id] = groupTitle(commandGroup(en, cmd.id), cmd.label)
	}
	for _, cmd := range a.commandsIn(shown) {
		if cmd.id == "go.commands" {
			continue
		}
		group := commandGroup(shown, cmd.id)
		title := groupTitle(group, cmd.label)
		e := paletteEntry{id: cmd.id, title: title, group: len(title) - len(cmd.label), shortcut: a.keyHint(cmd.id)}
		if english[cmd.id] != title {
			e.en = english[cmd.id]
		}
		a.palette = append(a.palette, e)
	}
	a.palette = append(a.palette, a.toolboxEntries()...)
	slices.SortStableFunc(a.palette, func(x, y paletteEntry) int { return strings.Compare(x.title, y.title) })
	return a.palette
}

// commandOn reports whether a command that turns something on or off,
// or picks a theme, has it on.
func (a *app) commandOn(id string) bool {
	s := a.prefs()
	switch id {
	case "file.autoSave":
		return s.AutoSave
	case "view.wordWrap":
		return s.WordWrap
	case "view.vim":
		return editorMode(s) == "vim"
	case "view.helix":
		return editorMode(s) == "helix"
	case "view.sidebarRight":
		return s.SidebarRight
	case "view.compactFolders":
		return s.ExplorerCompactFolders
	case "view.agent", "view.agentPage":
		return a.isAgentPageVisible()
	case "view.sessions":
		return a.sessionsVisible()
	case "view.terminalLocation":
		return s.TerminalLocation == "editor"
	case "git.blame":
		return s.Blame
	case "view.appearance.system":
		return s.Appearance == ""
	case "view.theme.fiveleaf":
		return themeSettingID(s.Theme) == ""
	}
	if pos, ok := strings.CutPrefix(id, "view.panelPosition."); ok {
		return a.panelSide() == pos
	}
	if tag, ok := strings.CutPrefix(id, localeCommand); ok {
		return a.localeOn(tag)
	}
	if ap, ok := strings.CutPrefix(id, "view.appearance."); ok {
		return s.Appearance == ap
	}
	if theme, ok := strings.CutPrefix(id, "view.theme."); ok {
		return themeSettingID(s.Theme) == themeSettingID(theme)
	}
	return false
}

// keyNames are the caps of the keys named in accelerators that have
// signs of their own: macOS's sign, and the name elsewhere.
var keyNames = map[string][2]string{
	"Left": {"←", "Left"}, "Right": {"→", "Right"}, "Up": {"↑", "Up"}, "Down": {"↓", "Down"},
	"Tab": {"⇥", "Tab"}, "Enter": {"⏎", "Enter"}, "Return": {"⏎", "Enter"},
	"Escape": {"esc", "Esc"}, "Backspace": {"⌫", "Backspace"}, "Delete": {"⌦", "Delete"},
}

// accelCaps returns the keys of an accelerator as the caps of a keyboard:
// as macOS shows them with mac, as ⇧⌘P, and otherwise by name.
func accelCaps(accel string, mac bool) []string {
	if accel == "" {
		return nil
	}
	var ctrl, alt, shift, cmd bool
	key := ""
	// The key may be "+" itself, after the modifiers.
	parts := strings.Split(accel, "+")
	if strings.HasSuffix(accel, "++") {
		parts = append(parts[:len(parts)-2], "+")
	}
	for _, p := range parts {
		switch p {
		case "CmdOrCtrl", "CommandOrControl":
			if mac {
				cmd = true
			} else {
				ctrl = true
			}
		case "Cmd", "Command", "Super", "Meta":
			cmd = true
		case "Ctrl", "Control":
			ctrl = true
		case "Alt", "Option":
			alt = true
		case "Shift":
			shift = true
		default:
			key = p
		}
	}
	var caps []string
	if mac {
		for _, m := range []struct {
			on  bool
			sym string
		}{{ctrl, "⌃"}, {alt, "⌥"}, {shift, "⇧"}, {cmd, "⌘"}} {
			if m.on {
				caps = append(caps, m.sym)
			}
		}
	} else {
		for _, m := range []struct {
			on   bool
			name string
		}{{ctrl, "Ctrl"}, {alt, "Alt"}, {shift, "Shift"}, {cmd, superName()}} {
			if m.on {
				caps = append(caps, m.name)
			}
		}
	}
	if n, ok := keyNames[key]; ok {
		if mac {
			key = n[0]
		} else {
			key = n[1]
		}
	}
	return append(caps, key)
}

// rememberCommand puts a command first among those run last, which the
// palette lists first.
func (a *app) rememberCommand(id string) {
	q := &a.quick
	q.used = slices.DeleteFunc(q.used, func(u string) bool { return u == id })
	q.used = slices.Insert(q.used, 0, id)
	q.used = q.used[:min(len(q.used), 8)]
}

func (q *quickOpen) matchCommands(entries []paletteEntry, key, query string, recent bool) bool {
	changed := q.commands == nil || !slices.Equal(q.commands, entries)
	if !q.stale && q.shown == key && !changed {
		return true
	}
	var selected string
	if changed && q.commands != nil && q.shown == key && q.sel >= 0 && q.sel < len(q.results) {
		selected = q.commands[q.results[q.sel].Index].id
	}
	q.results = paletteMatches(entries, query)
	if recent && query == "" {
		var first []workspace.Match
		for _, id := range q.used {
			if i := slices.IndexFunc(entries, func(e paletteEntry) bool { return e.id == id }); i >= 0 {
				first = append(first, workspace.Match{Index: i})
			}
		}
		rest := slices.DeleteFunc(q.results, func(m workspace.Match) bool {
			return slices.ContainsFunc(first, func(f workspace.Match) bool { return f.Index == m.Index })
		})
		q.results = append(first, rest...)
	}
	q.resetResults(key)
	q.commands = entries
	if selected == "" {
		return true
	}
	if index := slices.IndexFunc(q.results, func(match workspace.Match) bool { return entries[match.Index].id == selected }); index >= 0 {
		q.sel = index
		return true
	}
	return false
}

// quickCommands is quick open running a command by name: those run last
// first until a name is typed, then the others by title.
func (a *app) quickCommands(c *ui.Context, query string) {
	q := &a.quick
	key := ">" + query
	tools := strings.HasPrefix(query, "$")
	query = strings.TrimPrefix(query, "$")
	var entries []paletteEntry
	if tools {
		entries = a.toolboxEntries()
	} else {
		entries = a.paletteEntries()
	}
	query = strings.TrimSpace(query)
	selectionValid := q.matchCommands(entries, key, query, true)
	choose := func(i int) {
		if !selectionValid || i < 0 || i >= len(q.results) {
			q.shook = c.Now()
			return
		}
		id := entries[q.results[i].Index].id
		a.rememberCommand(id)
		a.closeQuick()
		a.post(func() { a.run(id) })
	}
	label, prompt, prefix, count, action := locales.PaletteLabel(), locales.PalettePlaceholder(), ">",
		locales.PaletteCommandCount(len(q.results)), locales.TaskRun()
	if tools {
		label, prompt, prefix, count, action = locales.ToolboxTitle(), locales.ToolboxPickerPlaceholder(), "",
			locales.ItemsCount(len(q.results)), locales.QuickOpenAction()
	}
	mode := quickMode{label: label, prompt: prompt, prefix: prefix, count: count, action: action,
		enabled: len(q.results) > 0, choose: choose}
	a.quickPicker(c, mode, func() {
		if len(q.results) == 0 {
			quickMessage(c, iconSearch, locales.PaletteNoMatches())
			return
		}
		width, _ := c.Size()
		a.commandResults(c, entries, query == "", width < 520, choose)
	})
}

func (a *app) commandResults(c *ui.Context, entries []paletteEntry, browsing, narrow bool, choose func(int)) {
	q, p := &a.quick, a.pal
	metrics := p.metricsOr()
	mark, muted := a.quickListColors(c)
	rowHeight, headingHeight := metrics.control+metrics.gap, metrics.row+metrics.gap
	items := make([]string, len(q.results))
	headings := map[int]string{}
	lastGroup := ""
	showGroup := func(entry paletteEntry) bool {
		return entry.group > 0 && (entry.secondary != "" || !browsing || slices.Contains(q.used, entry.id))
	}
	heightFor := func(entry paletteEntry) float32 {
		if narrow && showGroup(entry) {
			return rowHeight + metrics.textSmall + metrics.gap
		}
		return rowHeight
	}
	var contentHeight float32
	for index, match := range q.results {
		entry := entries[match.Index]
		items[index] = entry.id
		contentHeight += heightFor(entry)
		if browsing {
			group := entry.category()
			if slices.Contains(q.used, entry.id) {
				group = locales.PickRecent()
			}
			if group != "" && group != lastGroup {
				headings[index] = group
				contentHeight += headingHeight
			}
			lastGroup = group
		}
	}
	a.quickList(c, items, contentHeight, func(c *ui.Context, index int) {
		match := q.results[index]
		entry := entries[match.Index]
		if heading := headings[index]; heading != "" {
			a.pickerSection(c, heading)
		}
		a.quickResultRow(c, index, entry.title, heightFor(entry), mark, choose, func() {
			a.commandRow(c, entry, match.Positions, showGroup(entry), narrow, mark, muted)
		})
	})
}

// paletteMatches are the commands that query matches, best first: by their
// titles, by their English ones, whose matched characters do not show, or
// by the words that list their tools' features.
func paletteMatches(entries []paletteEntry, query string) []workspace.Match {
	titles := make([]string, len(entries))
	english := make([]string, len(entries))
	other := false
	for i, e := range entries {
		titles[i], english[i] = e.title, e.en
		other = other || e.en != ""
	}
	matches := workspace.Fuzzy(titles, query, len(entries))
	if query == "" {
		return matches
	}
	found := map[int]bool{}
	for _, m := range matches {
		found[m.Index] = true
	}
	if other {
		for _, m := range workspace.Fuzzy(english, query, len(entries)) {
			if !found[m.Index] && english[m.Index] != "" {
				m.Positions = nil
				matches = append(matches, m)
			}
		}
	}
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return matches
	}
	for i, e := range entries {
		if found[i] || e.keywords == "" {
			continue
		}
		hay := strings.ToLower(e.title + " " + e.en + " " + e.keywords)
		if slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(hay, w) }) {
			continue
		}
		found[i] = true
		matches = append(matches, workspace.Match{Index: i})
	}
	slices.SortStableFunc(matches, func(x, y workspace.Match) int { return y.Score - x.Score })
	return matches
}

func (a *app) commandRow(c *ui.Context, e paletteEntry, positions []int, showGroup, narrow bool, mark, muted ui.Color) {
	p := a.pal
	metrics := p.metricsOr()
	content := ui.Row(c).AlignItems(ui.Center).Gap(metrics.inset)
	if narrow && showGroup {
		content.Column().AlignItems(ui.Stretch).Gap(0)
	}
	content.Grow(1).MinWidth(0).Children(func() {
		name := ui.RichText(c, matchTextSpans(e.title[e.group:], e.group, positions, p.text, mark)...).
			FontSize(metrics.textBody + 1).SingleLine().MinWidth(0)
		if !narrow || !showGroup {
			name.Grow(1)
		}
		if showGroup {
			side := e.category()
			if e.secondary != "" {
				side = e.secondary
			}
			group := ui.RichText(c, matchTextSpans(side, 0, positions, muted, mark)...).
				FontSize(metrics.textSmall).SingleLine().MinWidth(0)
			if !narrow {
				group.MaxWidthPercent(35)
			}
		}
	})
	on := a.commandOn(e.id)
	if on || e.shortcut != "" || showGroup && !narrow {
		keys := ui.Row(c).Justify(ui.End).AlignItems(ui.Center).Gap(metrics.pad).MaxWidthPercent(40).Shrink(0)
		if showGroup && !narrow {
			keys.MinWidth(80)
		}
		keys.Children(func() {
			if on {
				ui.Icon(c, iconCheck).FontSize(14).TextColor(mark).Shrink(0)
			}
			if e.shortcut != "" {
				kbd.Kbd(c, kbd.Props{Label: e.shortcut}).MaxWidthPercent(100).MinWidth(0).Clip()
			}
		})
	}
}

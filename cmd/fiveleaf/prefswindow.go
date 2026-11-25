package main

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

var iconPalette = lucide(`<path d="M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z"/><circle cx="13.5" cy="6.5" r=".5" fill="currentColor"/><circle cx="17.5" cy="10.5" r=".5" fill="currentColor"/><circle cx="6.5" cy="12.5" r=".5" fill="currentColor"/><circle cx="8.5" cy="7.5" r=".5" fill="currentColor"/>`)

const (
	prefsShortcuts      = "shortcuts"
	prefsVersionControl = "versionControl"
	prefsHeaderH        = 56
)

// Navigation groups leave the schema's categories intact for field links,
// search and each page's scroll position.
type prefGroup struct {
	id         string
	categories []string
}

var prefsGroups = []prefGroup{
	{settings.CatGeneral, []string{settings.CatGeneral}},
	{settings.CatAppearance, []string{settings.CatAppearance}},
	{settings.CatEditor, []string{settings.CatEditor, settings.CatFonts, settings.CatMarkdown, settings.CatVim}},
	{settings.CatTerminal, []string{settings.CatTerminal, settings.CatTasks}},
	{prefsVersionControl, []string{settings.CatGit, settings.CatDiff}},
	{prefsShortcuts, []string{prefsShortcuts}},
	{settings.CatAgent, []string{settings.CatAgent}},
	{settings.CatPlugins, []string{settings.CatPlugins}},
}

// prefsAppearanceCards split the Appearance category into the cards of
// the page, in the order they show: what colors windows, what shapes them,
// what material they use, and where their parts sit.
var prefsAppearanceCards = []struct {
	key, label string
	fields     []string
}{
	{"theme", "theme", []string{"theme", "iconTheme", "themes", "appearance"}},
	{"window", "window", []string{"windowOpacity", "zoomLevel"}},
	{"material", "material", []string{"micaEnabled", "micaAnimation", "micaFrost", "micaHue", "micaIntensity", "micaSpeed"}},
	{"layout", "layout", []string{"sidebarRight", "hideToolboxButton", "terminalButtonRight", "panelPosition", "projectTabs", "projectTabStyle", "explorerCompactFolders", "searchTree"}},
}

// prefCardOf is the card of the appearance page that shows the
// preference of key.
func prefCardOf(key string) string {
	for _, card := range prefsAppearanceCards {
		if slices.Contains(card.fields, key) {
			return card.key
		}
	}
	return ""
}

func prefGroupOf(section string) prefGroup {
	for _, group := range prefsGroups {
		if group.id == section || slices.Contains(group.categories, section) {
			return group
		}
	}
	return prefGroup{id: section, categories: []string{section}}
}

func prefTabTitle(category string) string {
	switch category {
	case settings.CatEditor:
		return prefText("prefTab", "editing")
	case settings.CatVim:
		return prefText("prefTab", "keyModes")
	case settings.CatTerminal:
		return prefSection(settings.CatGeneral)
	default:
		return prefSection(category)
	}
}

// prefsIcons are the icons of the sections of the settings window.
var prefsIcons = map[string]*ui.SVG{
	settings.CatAppearance: iconPalette, settings.CatFonts: iconType, settings.CatEditor: iconFileCode,
	settings.CatMarkdown: iconFileText, settings.CatTerminal: iconTerminal, settings.CatDiff: iconDiff,
	settings.CatGit: iconBranch, settings.CatVim: iconCommand, settings.CatAgent: iconAgent,
	settings.CatPlugins: iconPlug, settings.CatGeneral: iconSettings,
	settings.CatTasks: iconClipboardList,
	prefsShortcuts:    iconKeyboard, prefsVersionControl: iconBranch,
}

// prefsWindow is the settings window, of which the app has one: the
// preferences by their categories, the folders' own, and the keyboard
// shortcuts.
type prefsWindow struct {
	m    *fiveleaf
	win  *mygo.Window
	pal  *palette
	fade paletteFade
	// section is the category shown, or prefsShortcuts, and query what
	// the search finds the preferences of, in every category.
	section string
	query   string
	// folder is the folder whose own preferences show, "" for the
	// person's.
	folder string
	// jump is the preference to bring into view and mark, as a link to
	// it asks, from jumpAt.
	jump   string
	jumpAt time.Time
	// texts and numbers are what the fields of the preferences hold, by
	// key, and editing the keys of those that have the keyboard: they
	// follow the preferences while they don't.
	texts   map[string]*string
	numbers map[string]*float64
	editing map[string]bool
	// held is the key of the number whose slider is being dragged, which
	// applies as it is let go.
	held   string
	errors []string
	// micaBusy prevents overlapping native import and export dialogs.
	micaBusy bool
	// notes are the toasts to show next, as a reset's with its Undo.
	notes []ui.Toast
	keys  keysPane
	// plugins is what the Plugins page keeps.
	plugins pluginsPage
	// agentDraft keeps a custom agent definition until it is saved or cancelled.
	agentDraft   *prefAgentDraft
	roleDraft    *prefRoleDraft
	agentIOBusy  bool
	conversation *prefsConversation
	taskCatalog  *prefsTaskCatalog
	// searchH and sectionsH are the search and the sections of the
	// sidebar, and sectionH each section, of whose place the mark under
	// the one chosen slides.
	searchH, sectionsH ui.Handle
	sectionH           map[string]*ui.Handle
	// firstH is the first control of the page, which bindFirst binds as
	// the page builds.
	firstH    ui.Handle
	bindFirst bool
	// finding asks for the keyboard to go to the search.
	finding bool
	// families are the monospaced font families installed, which the
	// fonts offer once listed, as familiesAsked asked.
	families      []string
	familiesAsked bool
	// peek is the font family the picker of the fonts is on, which the
	// code of the fonts shows, "" for none.
	peek string
	// drafts apply what was typed in the text fields of the keys, not yet
	// applied, and drawn are the keys of those the frame drew.
	drafts map[string]func()
	drawn  map[string]bool
	// materialOpen shows the Mica parameters while the material is off,
	// as the person asks to tune them before turning it on.
	materialOpen bool
	// micaAsk is the jump to the material's parameters that the Tune
	// button asked for before the material is on.
	micaAsk bool
}

// newPrefsWindow is the settings window of m, at its first section, yet
// to show.
func newPrefsWindow(m *fiveleaf) *prefsWindow {
	w := &prefsWindow{m: m, section: settings.CatGeneral, texts: map[string]*string{}, numbers: map[string]*float64{},
		editing: map[string]bool{}, drafts: map[string]func(){}, drawn: map[string]bool{}}
	w.keys.w = w
	return w
}

// showPrefs opens the settings window, or brings it to the front, at
// section and the preference of key, "" for none: of the folder of from
// when key is a preference a folder chooses there.
func (m *fiveleaf) showPrefs(section, key string, from *desktop) {
	w := m.prefs
	if w == nil {
		w = newPrefsWindow(m)
		w.win = mygo.NewWindow(mygo.WindowOptions{
			Title: locales.PrefsTitle(), Width: 940, Height: 680, MinWidth: 640, MinHeight: 420,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: prefsHeaderH,
			TrafficLightPosition: &mygo.Point{X: 14, Y: (prefsHeaderH - 14) / 2},
			StateKey:             "settings", Hidden: true, Transparent: true, BackgroundColor: "#131317",
			Opacity: 1, Vibrancy: windowVibrancy(m.s.WindowOpacity), Content: ui.View(w.view),
		})
		w.win.OnFocus(func() {
			w.taskCatalog = nil
			w.invalidate()
			m.refreshMenu()
		})
		w.win.OnClosed(func() {
			w.closeRoleDraft()
			if m.prefs == w {
				m.prefs = nil
			}
		})
		m.prefs = w
	}
	w.navigate(section, key, from)
	if w.win.IsMinimized() {
		w.win.Restore()
	}
	if from != nil && from.f != nil {
		centerAuxiliaryWindow(w.win, from.f.win)
	}
	w.win.Show()
	w.win.Focus()
	w.win.Invalidate()
}

func centerAuxiliaryWindow(window, parent *mygo.Window) {
	if parent == nil || parent.IsDestroyed() || window.IsMaximized() || window.IsFullScreen() {
		return
	}
	bounds := parent.Bounds()
	workArea := mygo.Screen.DisplayMatching(bounds).WorkArea
	width, height := window.Size()
	window.SetPosition(prefsWindowPosition(bounds, workArea, width, height))
}

func prefsWindowPosition(parent, workArea mygo.Rectangle, width, height int) (int, int) {
	x := parent.X + (parent.Width-width)/2
	y := parent.Y + (parent.Height-height)/2
	if workArea.Width > 0 && workArea.Height > 0 {
		x = max(workArea.X, min(x, workArea.X+workArea.Width-width))
		y = max(workArea.Y, min(y, workArea.Y+workArea.Height-height))
	}
	return x, y
}

func (w *prefsWindow) navigate(section, key string, from *desktop) {
	w.conversation = nil
	w.materialOpen, w.micaAsk = false, false
	if key == prefsConversationKey && from != nil {
		w.conversation = &prefsConversation{project: from.a, chat: from.a.agent}
		w.section, w.query, w.folder, w.jump = settings.CatAgent, "", "", ""
		return
	}
	if section != "" {
		w.section, w.query = section, ""
		if section == prefsVersionControl {
			w.section = settings.CatGit
		}
	}
	if section == settings.CatPlugins && key == toolboxCategoryKey {
		if w.plugins.detail != "" {
			w.closeDetail()
		}
		w.plugins.filters = pluginFilters{kind: slices.IndexFunc(pluginCategories, func(category pluginCategory) bool {
			return category.id == "tools"
		})}
		w.plugins.listScroll.Y = 0
		w.jump = ""
		return
	}
	// The details of a plugin, on the Plugins page.
	if name, ok := strings.CutPrefix(key, pluginDetailKey); ok {
		w.section, w.query, key = prefsPlugins, "", ""
		w.openDetail(name)
	}
	if key != "" {
		if f, ok := settings.FieldOf(key); ok {
			w.section, w.query = f.Category, ""
			w.folder = ""
			if from != nil && f.Folder && from.a.folderPrefs != nil && from.a.folderPrefs.f.Has(key) {
				w.folder = from.a.root
			}
		}
		w.jump, w.jumpAt = key, time.Now()
	}
}

// prefsChanged draws the settings window anew, as the preferences or the
// shortcuts changed.
func (m *fiveleaf) prefsChanged() {
	if m.prefs != nil {
		m.prefs.invalidate()
	}
}

// closePrefs closes the settings window.
func (m *fiveleaf) closePrefs() {
	if m.prefs != nil && m.prefs.win != nil {
		m.prefs.win.Close()
	}
}

// invalidate draws the window anew.
func (w *prefsWindow) invalidate() {
	if w.win != nil {
		w.win.Invalidate()
	}
}

// prefText is a message of the settings window by its key, as those of
// each preference are: "prefTitleFontSize" for fontSize's title.
func prefText(prefix, name string) string {
	return locales.Catalog.Format(i18n.Locale(), prefKey(prefix, name), nil)
}

// prefKey is the key of the message of prefix for name, a key of the
// preferences or a category: "prefTitle" and "fontSize" are
// "prefTitleFontSize".
func prefKey(prefix, name string) string {
	name = strings.NewReplacer("-", "", ".", "").Replace(name)
	if name == "" {
		return prefix
	}
	return prefix + strings.ToUpper(name[:1]) + name[1:]
}

func prefTitle(key string) string    { return prefText("prefTitle", key) }
func prefDesc(key string) string     { return prefText("prefDesc", key) }
func prefSection(name string) string { return prefText("prefSection", name) }

// prefOption is how an option of a preference of options reads: the
// languages in themselves.
func prefOption(key, opt string) string {
	if key == "locale" && opt != "" {
		return i18n.Autonym(opt)
	}
	if opt == "" {
		return prefText("prefDefault", key)
	}
	return prefText(prefKey("prefOpt", key), opt)
}

// prefFolder is a folder whose own preferences the window can show: an
// open window's.
type prefFolder struct {
	dir string
	h   *desktop
	f   *settings.Folder
}

// prefFolders are the folders whose own preferences the window can show.
func (w *prefsWindow) prefFolders() []prefFolder {
	var out []prefFolder
	for _, h := range w.m.wins {
		if fp := h.a.folderPrefs; fp != nil && !slices.ContainsFunc(out, func(o prefFolder) bool { return o.dir == h.a.root }) {
			out = append(out, prefFolder{h.a.root, h, fp.f})
		}
	}
	slices.SortFunc(out, func(x, y prefFolder) int { return strings.Compare(x.dir, y.dir) })
	return out
}

// scope is the folder whose own preferences show, nil for the person's.
func (w *prefsWindow) scope() *prefFolder {
	if w.folder == "" {
		return nil
	}
	for _, f := range w.prefFolders() {
		if f.dir == w.folder {
			return &f
		}
	}
	w.folder = ""
	return nil
}

func prefInWindow(f settings.Field) bool {
	return !f.Memory && f.Key != "agentRoles"
}

// shown are the preferences the window shows: those of its category, or
// those the search finds, of a folder those it may choose.
func (w *prefsWindow) shown(folder bool) []settings.Field {
	words := strings.Fields(strings.ToLower(w.query))
	var out []settings.Field
	for _, f := range settings.Schema {
		if !prefInWindow(f) || folder && !f.Folder {
			continue
		}
		if len(words) == 0 {
			if f.Category == w.section {
				out = append(out, f)
			}
			continue
		}
		text := strings.ToLower(f.Key + " " + prefTitle(f.Key) + " " + prefDesc(f.Key) + " " +
			prefSection(f.Category) + " " + prefTabTitle(f.Category) + " " + prefSection(prefGroupOf(f.Category).id))
		if f.Key == "agentRolePresets" {
			text += " agentservers agentroles " + strings.ToLower(locales.PrefsRoleAdd())
			for _, preset := range agentRolePresets {
				text += " " + strings.ToLower(preset.id+" "+preset.name())
			}
			for name := range w.m.s.AgentRoles {
				text += " " + strings.ToLower(name)
			}
		} else if f.Key == "agentServers" {
			for _, provider := range (&app{settings: w.m.s}).agentProviders() {
				text += " " + strings.ToLower(provider.id+" "+provider.name)
			}
		}
		if !slices.ContainsFunc(words, func(word string) bool { return !strings.Contains(text, word) }) {
			out = append(out, f)
		}
	}
	if len(words) == 0 && w.section == settings.CatAppearance {
		order := map[string]int{}
		for i, card := range prefsAppearanceCards {
			for j, key := range card.fields {
				order[key] = i*100 + j
			}
		}
		out = out[:0]
		for _, f := range settings.Schema {
			if f.Category != settings.CatAppearance || !prefInWindow(f) || folder && !f.Folder {
				continue
			}
			if _, ok := order[f.Key]; ok {
				out = append(out, f)
			}
		}
		slices.SortStableFunc(out, func(a, b settings.Field) int {
			return order[a.Key] - order[b.Key]
		})
	}
	// Schema fields may be interleaved; each search category needs one card.
	if len(words) > 0 {
		slices.SortStableFunc(out, func(a, b settings.Field) int {
			return slices.Index(settings.Categories, a.Category) - slices.Index(settings.Categories, b.Category)
		})
	}
	return out
}

func (w *prefsWindow) view(c *ui.Context) {
	m := w.m
	if m.s == nil {
		return
	}
	if translucentWindow(c, m.s.WindowOpacity) {
		defer c.Root().Background(ui.Transparent)
	}
	materialPrefs := w.materialPrefs()
	w.pal = applyTheme(c, &materialPrefs, &w.fade)
	p := w.pal
	c.SetZoom(float32(math.Pow(1.2, float64(m.s.ZoomLevel))))
	if title := locales.PrefsTitle(); w.win != nil && w.win.Title() != title {
		w.win.SetTitle(title)
	}
	// As a window of FiveLeaf's, Close closes it and Find finds: the menu
	// bar of macOS takes their shortcuts first.
	if !macKeys() && c.Shortcut(ui.Cmd, ui.KeyW) {
		w.m.closePrefs()
	}
	if !macKeys() && c.Shortcut(ui.Cmd, ui.KeyF) {
		w.finding = true
	}
	w.focusSearch(c)
	if w.section != settings.CatTasks || w.query != "" {
		w.taskCatalog = nil
	}
	// The details of a plugin close as the Plugins page does.
	if pg := &w.plugins; (pg.detail != "" || pg.back) && (w.section != settings.CatPlugins || w.query != "") {
		pg.detail, pg.back, pg.info = "", false, pluginInfo{}
	}
	shell := ui.Box(c).Fill().Background(windowContainer(c, m.s.WindowOpacity, p.editor))
	materialBackdrop(c, shell, p, m.s.WindowOpacity,
		!m.s.MicaAnimation || c.Preferences().ReduceMotion).Children(func() {
		ui.Row(c.Key("prefs-body")).Fill().AlignItems(ui.Stretch).
			Background(windowBackground(c, m.s.WindowOpacity, p.editor)).Children(func() {
			w.sidebar(c)
			ui.Column(c.Key("prefs-main")).Grow(1).MinWidth(0).Children(func() {
				if target := w.conversation; target != nil {
					if w.section == settings.CatAgent && w.query == "" && target.available() {
						w.conversationPane(c, target)
						return
					}
					w.conversation = nil
				}
				if w.section == prefsShortcuts && w.query == "" {
					w.keys.view(c)
				} else if w.section == settings.CatPlugins && w.query == "" && w.scope() == nil {
					w.pluginsPane(c)
				} else {
					w.prefsPane(c)
				}
			})
		})
	})
	w.keys.recordView(c)
	w.settleDrafts()
	for _, msg := range w.errors {
		w.notes = append(w.notes, ui.Toast{ID: msg, Title: msg, Type: "error"})
	}
	for _, t := range w.notes {
		t.Data = rememberNotification(m.s, locales.PrefsTitle(), t, m.redrawWindows)
		c.AddToast(t)
	}
	clear(w.notes)
	w.errors, w.notes = w.errors[:0], w.notes[:0]
	notificationViewport(c, p, 0, false)
}

// find asks for the keyboard to go to the search of what shows, its text
// chosen, as the window is drawn next.
func (w *prefsWindow) find() {
	w.finding = true
	w.invalidate()
}

// focusSearch gives the keyboard to the search, as find asked: the
// shortcuts' while they show, else the preferences'.
func (w *prefsWindow) focusSearch(c *ui.Context) {
	if !w.finding {
		return
	}
	w.finding = false
	if w.section == prefsShortcuts && w.query == "" {
		w.keys.byKeys, w.keys.found = false, nil
		w.keys.searchH.SelectAll(c)
	} else if plugins != nil && w.section == prefsPlugins && w.plugins.detail == "" && w.query == "" && w.scope() == nil {
		w.plugins.searchH.SelectAll(c)
	} else if w.noFirstControl() {
		w.sectionsH.Focus(c)
	} else {
		w.searchH.SelectAll(c)
	}
}

// focusContent gives the keyboard to what the section shows, from the
// sidebar: the list of the shortcuts, or the first control of the
// preferences.
func (w *prefsWindow) focusContent(c *ui.Context) {
	if w.section == prefsShortcuts && w.query == "" {
		w.keys.listH.Focus(c)
	} else if plugins != nil && w.section == prefsPlugins && w.plugins.detail == "" && w.query == "" && w.scope() == nil {
		w.plugins.searchH.Focus(c)
	} else if w.noFirstControl() {
		w.sectionsH.Focus(c)
	} else {
		w.firstH.Focus(c)
	}
}

// noFirstControl says a page shows no preference, as a folder's page
// has none it may choose: its empty state takes no keyboard.
func (w *prefsWindow) noFirstControl() bool {
	if w.section == settings.CatTasks && w.query == "" {
		return false
	}
	return w.query != "" || len(w.shown(w.scope() != nil)) == 0
}

// first binds e to the handle of the first control of the page, while
// the page builds it.
func (w *prefsWindow) first(e ui.Element) ui.Element {
	if w.bindFirst {
		w.bindFirst = false
		e.Bind(&w.firstH)
	}
	return e
}

// sidebar is the window's search and its sections. Down goes from the
// search to the sections, whose Right and Enter go on to the section.
func (w *prefsWindow) sidebar(c *ui.Context) {
	p := w.pal
	m := p.metricsOr()
	bar := c.TitleBar()
	top := float32(14)
	if bar.Left > 0 {
		top = max(prefsHeaderH, bar.Height) + 8
	}
	ui.Column(c.Key("prefs-sidebar")).Width(prefsSidebarW).Shrink(0).Padding(top, 10, 10, 10).Gap(10).
		DragWindow().Children(func() {
		searchSurface(c, ui.SearchField(c.Key("prefs-search"), &w.query)).Height(32).Padding(6, 10).
			Label(locales.PrefsSearch()).Placeholder(locales.PrefsSearch()).AutoFocus().
			Bind(&w.searchH)
		w.searchH.OnShortcut(c, 0, ui.KeyDown, func() {
			if w.query == "" {
				w.sectionsH.Focus(c)
			} else {
				w.firstH.Focus(c)
			}
		})
		// No section is chosen while the search shows what it finds of
		// them all.
		section := prefGroupOf(w.section).id
		if w.query != "" {
			section = ""
		}
		// The sections sit on the sidebar itself, not on a box of their
		// own, as the app's sidebar lists sit on its chrome. The one chosen
		// sits on a mark that slides to the next chosen, and takes the
		// accent while the sections have the keyboard.
		scope := w.scope()
		base := c.Theme()
		quiet := *base
		quiet.AccentText = p.accent
		c.SetTheme(&quiet)
		focused := w.sectionsH.Focused(c)
		mark := w.sectionMark(c, section, focused)
		list := ui.Sidebar(c.Key("prefs-sections-list"), &section, func() {
			for _, group := range prefsGroups {
				item := ui.SidebarItem(c.Key("prefs-section-"+group.id), group.id, prefsIcons[group.id], prefSection(group.id)).
					Height(m.control + m.pad).Radius(m.radius).Bind(w.sectionHandle(group.id))
				if group.id == section && mark != nil {
					item.Background(ui.Color{}).TextColor(p.text)
				}
				if n := w.changedInGroup(group, scope); n > 0 {
					item.Children(func() {
						ui.Text(c.Key("count"), fmt.Sprint(n)).FontSize(10.5).FontWeight(600).TextColor(p.muted).
							Padding(0, 6).Radius(8).MinWidth(18).Shrink(0).TextAlign(ui.Center).Background(p.text.Alpha(0.07)).
							Tooltip(locales.PrefsChangedCount(n)).ContentTransition(ui.ContentTransition{Numeric: true}).Transition(dotPop)
					})
				}
			}
		}).Background(ui.Color{}).Padding(0).Gap(1).Grow(1).MinHeight(0).Bind(&w.sectionsH)
		c.SetTheme(base)
		if mark != nil {
			list.Draw(mark)
		}
		w.sectionsH.OnShortcut(c, 0, ui.KeyRight, func() { w.focusContent(c) })
		w.sectionsH.OnShortcut(c, 0, ui.KeyEnter, func() { w.focusContent(c) })
		if section != "" && (section != prefGroupOf(w.section).id || w.query != "") {
			w.section, w.query = prefGroupOf(section).categories[0], ""
		}
		ui.Column(c.Key("prefs-links")).Gap(1).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
			w.footLink(c, "json", "settings.json", locales.CmdFileSettingsJSON(), func() { w.inFront(func(a *app) { a.openSettings() }) })
			w.footLink(c, "keys-json", "keybindings.json", locales.CmdFileKeybindingsJSON(), func() { w.inFront(func(a *app) { a.openKeybindings() }) })
		})
	})
}

// prefsSidebarW is how wide the sidebar of the window is.
const prefsSidebarW = 200

// sectionHandle is the handle of the item of the section cat.
func (w *prefsWindow) sectionHandle(cat string) *ui.Handle {
	if w.sectionH == nil {
		w.sectionH = map[string]*ui.Handle{}
	}
	h := w.sectionH[cat]
	if h == nil {
		h = new(ui.Handle)
		w.sectionH[cat] = h
	}
	return h
}

// sectionMark paints the mark under the section chosen, as the sections
// were laid out: it slides to the one chosen, as its color fades to the
// accent's while the sections have the keyboard. It is nil while none is
// chosen, or before the sections were laid out.
func (w *prefsWindow) sectionMark(c *ui.Context, section string, focused bool) func(*ui.Painter, ui.Rect) {
	p := w.pal
	list, item := w.sectionsH.Bounds(c), w.sectionHandle(section).Bounds(c)
	if section == "" || list.W == 0 || item.W == 0 {
		return nil
	}
	fill, edge := p.active, ui.Color{}
	if focused {
		fill, edge = p.accent.Alpha(0.16), p.accent.Alpha(0.5)
	}
	host := ui.Box(c.Key("prefs-section-mark")).Absolute().Size(0, 0)
	y := host.Animate("y", item.Y-list.Y, motionMove)
	fill = host.AnimateColor("fill", fill, ui.Animation{Duration: motionHover})
	edge = host.AnimateColor("edge", edge, ui.Animation{Duration: motionHover})
	x, h := item.X-list.X, item.H
	return func(pt *ui.Painter, r ui.Rect) {
		box := ui.Rect{X: r.X + x, Y: r.Y + y, W: item.W, H: h}
		pt.Fill(box, fill, 6)
		if edge.A > 0 {
			pt.Stroke(box, edge, 6, 1)
		}
	}
}

func (w *prefsWindow) changedInGroup(group prefGroup, scope *prefFolder) int {
	n := 0
	for _, category := range group.categories {
		n += w.changedIn(category, scope)
	}
	return n
}

// changedIn is how many of the person's preferences of the category are
// not as a first launch has them, or the folder chooses while its own
// show, or of the shortcuts how many commands have keys of the person's.
func (w *prefsWindow) changedIn(cat string, scope *prefFolder) int {
	n := 0
	if cat == prefsShortcuts {
		if w.m.s.Keymap != "" {
			n++
		}
		if km := w.m.keys; km != nil {
			seen := map[string]bool{}
			for _, b := range km.Bindings {
				name := strings.TrimPrefix(b.Command, "-")
				if b.When == nil && len(b.Args) == 0 && !seen[name] {
					seen[name] = true
					n++
				}
			}
		}
		return n
	}
	user := w.m.s.Prefs
	for _, f := range settings.Schema {
		if f.Category != cat || !prefInWindow(f) {
			continue
		}
		if scope != nil && f.Folder && scope.f.Has(f.Key) || scope == nil && (!user.IsDefaultKey(f.Key) || f.Key == "agentRolePresets" && len(user.AgentRoles) > 0) {
			n++
		}
	}
	return n
}

// footLink is a link of the foot of the sidebar to the file name, which
// label says what opening does.
func (w *prefsWindow) footLink(c *ui.Context, key, name, label string, fn func()) {
	p := w.pal
	b := ui.ButtonBase(c.Key("prefs-link-"+key)).Padding(6, 8).Gap(8).Radius(6).AlignItems(ui.Center).Label(label).Tooltip(label)
	col := p.muted
	if b.Hovered() {
		b.Background(p.hover)
		col = p.text
	}
	b.Transition(hoverFade)
	b.Children(func() {
		ui.Icon(c, iconFileJSON).FontSize(13).TextColor(col).Shrink(0)
		ui.Text(c, name).Font(w.monoFont()).FontSize(11.5).TextColor(col).SingleLine().Grow(1).Shrink(1)
		ui.Icon(c, iconExternal).FontSize(11).TextColor(p.faint).Shrink(0)
	})
	if b.Clicked() {
		fn()
	}
}

// inFront runs fn on the window in front, which comes forward.
func (w *prefsWindow) inFront(fn func(a *app)) {
	if len(w.m.wins) == 0 {
		return
	}
	h := w.m.wins[0]
	if h.a.window.Detached {
		h.inWorkspace(fn)
		return
	}
	h.do(func() { fn(h.a) })
	h.show()
}

// prefsPane is the preferences of the section, or those the search
// finds, with the choice of whose they are.
func (w *prefsWindow) prefsPane(c *ui.Context) {
	p := w.pal
	scope := w.scope()
	group := prefGroupOf(w.section)
	var fields []settings.Field
	taskMatch := taskSettingsMatch(w.query)
	title, icon := prefSection(group.id), prefsIcons[group.id]
	if w.query != "" {
		fields = w.shown(scope != nil)
		count := len(fields)
		if taskMatch {
			count++
		}
		title, icon = locales.PrefsFound(count), iconSearch
	}
	w.head(c, "prefs-head", icon, title, func() {
		if w.section == settings.CatAgent && w.query == "" && len(w.m.wins) > 0 {
			from := w.m.wins[0]
			if scope != nil {
				from = scope.h
			}
			available := !from.a.loadsClosed && from.a.agent != nil && from.a.root != "" && !remote.Is(from.a.root)
			if actionButton(c, locales.AgentConversationSettings(), iconComment).Disabled(!available).Clicked() {
				w.navigate(settings.CatAgent, prefsConversationKey, from)
			}
		}
		w.scopePicker(c, scope)
	})
	if w.query == "" {
		w.categoryTabs(c, group)
		if w.section == settings.CatTasks {
			w.taskSettingsPane(c, scope)
			return
		}
		fields = w.shown(scope != nil)
	}
	if scope != nil {
		ui.Row(c.Key("prefs-folder-note")).Padding(10, 24).Gap(8).AlignItems(ui.Center).Background(p.accent.Alpha(0.08)).
			BorderWidth(0, 0, 1, 0).BorderColor(p.border).Transition(popIn).Children(func() {
			ui.Icon(c, iconFolder).FontSize(13).TextColor(p.accent).Shrink(0)
			ui.Text(c, locales.PrefsFolderNote(filepath.Base(filepath.Dir(scope.f.Path()))+"/"+settings.FolderFile)).FontSize(12).TextColor(p.text).Grow(1).Shrink(1)
			if actionButton(c.Key("prefs-folder-open"), locales.PrefsOpenFile(), iconOpenFile).Clicked() {
				h := scope.h
				h.do(h.a.openFolderSettings)
				h.show()
			}
		})
	}
	// A page of its own for each section and for the search, so that it
	// fades in as it shows, and keeps where it was scrolled to.
	page := w.section
	if w.query != "" {
		page = "search"
	}
	ui.Scroll(c.Key("prefs-rows-" + page)).Grow(1).MinHeight(0).Children(func() {
		fadePage(ui.Column(c.Key("prefs-page-"+page)).Padding(20, 28, 32, 28).Gap(8).AlignSelf(ui.Center).
			WidthPercent(100).MaxWidth(prefsPageWidth)).Children(func() {
			if len(fields) == 0 && !taskMatch {
				w.noneFound(c, locales.PrefsNoneFound())
				return
			}
			w.bindFirst = true
			defer func() { w.bindFirst = false }()
			if taskMatch {
				if w.first(actionButton(c.Key("task-settings-result"), locales.PrefsTaskDefinitions(), iconClipboardList)).AlignSelf(ui.Start).Clicked() {
					w.navigate(settings.CatTasks, "", nil)
				}
			}
			if w.query == "" && w.section == settings.CatFonts && scope == nil {
				w.fontPreview(c)
			}
			for len(fields) > 0 {
				// The search finds them by category, each under its name.
				n := 1
				for n < len(fields) && fields[n].Category == fields[0].Category {
					n++
				}
				if w.query == "" {
					n = len(fields)
				} else {
					cat := fields[0].Category
					ui.Row(c.Key("prefs-cat-"+cat)).Gap(6).AlignItems(ui.Center).Padding(10, 4, 0, 4).
						Transition(revealFade).Children(func() {
						ui.Icon(c, prefsIcons[cat]).FontSize(11.5).TextColor(p.faint)
						ui.Text(c, strings.ToUpper(prefSection(cat))).FontSize(10.5).FontWeight(600).
							LetterSpacing(0.7).TextColor(p.faint).SingleLine()
					})
				}
				group := fields[:n]
				fields = fields[n:]
				if w.query == "" && w.section == settings.CatAppearance {
					w.appearanceCards(c, group, scope)
				} else {
					w.card(c, "prefs-card-"+group[0].Category, func() {
						for i, f := range group {
							if i > 0 {
								ui.Row(c.Key("prefs-rule-"+f.Key)).Height(1).Margin(0, 16).Background(p.border).Shrink(0)
							}
							w.row(c, f, scope)
						}
					})
				}
			}
		})
	})
}

// appearanceCards draws the Appearance page as the cards of
// prefsAppearanceCards, each under its own label.
func (w *prefsWindow) appearanceCards(c *ui.Context, fields []settings.Field, scope *prefFolder) {
	for _, card := range prefsAppearanceCards {
		var group []settings.Field
		for _, f := range fields {
			if prefCardOf(f.Key) == card.key {
				group = append(group, f)
			}
		}
		if card.key == "material" && !w.materialParametersShown(scope) {
			group = slices.DeleteFunc(group, func(f settings.Field) bool { return micaParameter(f.Key) })
		}
		if len(group) == 0 {
			continue
		}
		w.sectionLabel(c, "appearance-"+card.key, prefSection("appearance"+strings.ToUpper(card.label[:1])+card.label[1:]), 0)
		w.card(c, "prefs-card-"+card.key, func() {
			for i, f := range group {
				if i > 0 {
					ui.Row(c.Key("prefs-rule-"+f.Key)).Height(1).Margin(0, 16).Background(w.pal.border).Shrink(0)
				}
				w.row(c, f, scope)
			}
			if card.key == "material" && scope == nil && !w.materialParametersShown(scope) {
				ui.Row(c.Key("prefs-rule-material-more")).Height(1).Margin(0, 16).Background(w.pal.border).Shrink(0)
				if actionButton(c.Key("material-more"), locales.PrefsMicaTune(), iconSliders).AlignSelf(ui.Start).Clicked() {
					w.materialOpen, w.micaAsk = true, true
				}
			}
		})
	}
}

// materialParametersShown says the Mica parameters show: while the
// material is on, as they do, or as the person opened them to tune it
// before turning it on, or as one of them is what the page came for.
func (w *prefsWindow) materialParametersShown(scope *prefFolder) bool {
	if scope != nil {
		return scope.f.Apply(w.m.s.Prefs).MicaEnabled
	}
	return w.materialOpen || w.m.s.MicaEnabled || w.query != "" || micaParameter(w.jump) || w.micaAsk
}

func (w *prefsWindow) categoryTabs(c *ui.Context, group prefGroup) {
	if len(group.categories) < 2 {
		return
	}
	p := w.pal
	m := p.metricsOr()
	selected := slices.Index(group.categories, w.section)
	ui.Row(c.Key("prefs-tabs-"+group.id)).Height(44).Padding(0, 24).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
		ui.ScrollHorizontal(c.Key("scroll")).Grow(1).MinWidth(0).Children(func() {
			tabs := ui.TabsBase(c.Key("tabs"), &selected, len(group.categories))
			tabs.List.Gap(m.gap).Label(prefSection(group.id)).Children(func() {
				for i, category := range group.categories {
					tab := tabs.Tab(i).Height(28).Padding(0, m.inset).Radius(m.radius).
						Label(prefTabTitle(category)).Border(1, ui.Transparent).Transition(hoverFade)
					text := p.muted
					if i == selected {
						tab.Background(p.active)
						text = p.text
					} else if tab.Hovered() {
						tab.Background(p.hover)
						text = p.text
					}
					if tab.FocusVisible() {
						tab.BorderColor(p.accent)
						tab.ScrollIntoView()
					}
					tab.Children(func() {
						ui.Text(c, prefTabTitle(category)).FontSize(m.textBody).TextColor(text).SingleLine()
					})
				}
			})
		})
	})
	w.section = group.categories[selected]
}

// prefsPageWidth is how wide the preferences grow, at most: wider, a line
// of them is too long to read across.
const prefsPageWidth = 760

// head is the head of a pane of the window: its icon and title, and the
// controls fn adds at its end.
func (w *prefsWindow) head(c *ui.Context, key string, icon *ui.SVG, title string, fn func()) {
	w.responsiveHead(c, key, icon, title, 440, fn)
}

func (w *prefsWindow) responsiveHead(c *ui.Context, key string, icon *ui.SVG, title string, minContentWidth float32, fn func()) {
	p := w.pal
	bar := c.TitleBar()
	width, _ := c.Size()
	left, right := 24+max(0, bar.Left-prefsSidebarW), 20+bar.Right
	narrow := width-prefsSidebarW-left-right < minContentWidth
	header := ui.Row(c.Key(key)).Shrink(0).DragWindow()
	if narrow {
		header.Column().AlignItems(ui.Stretch)
	} else {
		header.Height(max(prefsHeaderH, bar.Height)).Padding(0, right, 0, left).Gap(10).AlignItems(ui.Center)
	}
	header.Children(func() {
		heading := ui.Row(c.Key("heading")).Gap(10).AlignItems(ui.Center).MinWidth(0)
		if narrow {
			heading.Height(max(prefsHeaderH, bar.Height)).Padding(0, right, 0, left).Shrink(0)
		} else {
			heading.Grow(1)
		}
		heading.Children(func() {
			ui.Box(c.Key("icon")).Size(26, 26).Radius(7).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).
				Background(p.accent.Alpha(0.14)).Children(func() {
				ui.Icon(c, icon).FontSize(14).TextColor(p.accent)
			})
			ui.Text(c.Key("title"), title).FontSize(16).FontWeight(600).TextColor(p.text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		})
		actions := ui.Row(c.Key("actions")).Gap(10).AlignItems(ui.Center).MinWidth(0)
		if narrow {
			actions.Padding(0, 20, 12, 24).Wrap()
		} else {
			actions.Shrink(0)
		}
		actions.Children(fn)
	})
}

// card is a box of rows of the window, as a group of System Settings.
func (w *prefsWindow) card(c *ui.Context, key string, fn func()) {
	p := w.pal
	ui.Column(c.Key(key)).Radius(p.metricsOr().radiusCard).Clip().
		Background(windowContainer(c, w.m.s.WindowOpacity, p.surface)).Border(1, p.border).Children(fn)
}

// noneFound says a search found nothing.
func (w *prefsWindow) noneFound(c *ui.Context, text string) {
	p := w.pal
	ui.Column(c.Key("none-found")).Padding(48, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		popUp(ui.Box(c.Key("badge")).Size(44, 44).Radius(22).Background(p.text.Alpha(0.06)).Justify(ui.Center).AlignItems(ui.Center)).Children(func() {
			ui.Icon(c, iconSearch).FontSize(19).TextColor(p.muted)
		})
		ui.Text(c, text).FontSize(13).TextColor(p.muted).TextAlign(ui.Center)
	})
}

// scopePicker chooses whose preferences show: the person's, or a
// folder's own, of the folders open.
func (w *prefsWindow) scopePicker(c *ui.Context, scope *prefFolder) {
	folders := w.prefFolders()
	if len(folders) == 0 {
		return
	}
	labels := []string{locales.PrefsUser()}
	chosen := labels[0]
	for _, f := range folders {
		label := locales.PrefsFolder(shortenHome(f.dir))
		labels = append(labels, label)
		if scope != nil && f.dir == scope.dir {
			chosen = label
		}
	}
	if ui.Select(c.Key("prefs-scope"), &chosen, labels).Label(locales.PrefsScope()).Width(260).MinWidth(0).Shrink(1).Changed() {
		w.folder = ""
		if i := slices.Index(labels, chosen); i > 0 {
			w.folder = folders[i-1].dir
		}
	}
}

// row is a preference: its name and what it does, and its control, with
// a mark while it is not as a first launch has it, or a folder chooses
// it, and the button that takes that back.
func (w *prefsWindow) row(c *ui.Context, f settings.Field, scope *prefFolder) {
	p := w.pal
	m := w.m
	user := m.s.Prefs
	eff, changed := user, !user.IsDefaultKey(f.Key)
	if scope != nil {
		eff, changed = scope.f.Apply(user), scope.f.Has(f.Key)
	}
	raw := eff.Raw(f.Key)
	row := ui.Column(c.Key("prefs-row-"+f.Key)).Padding(14, 16).Gap(10).Transition(ui.ElementTransition{
		Colors: true, Duration: motionShow, Enter: &ui.Motion{Y: 4},
	})
	if micaParameter(f.Key) {
		row.Disabled(!eff.MicaEnabled)
	}
	if f.Key == w.jump {
		row.ScrollIntoView()
		// The row a link asked for glows, and fades back as it is seen.
		if since := c.Now().Sub(w.jumpAt); since < 1600*time.Millisecond && !c.Preferences().ReduceMotion {
			row.Background(p.accent.Alpha(0.14 * float32(1-since.Seconds()/1.6)))
			c.AnimationFrame()
		} else {
			w.jump = ""
		}
	}
	agentList := f.Key == "agentRolePresets" || f.Key == "agentServers"
	wide := f.Kind == settings.KindList || f.Kind == settings.KindStyles || f.Key == "theme" || f.Key == "iconTheme" || f.Key == "copilot" || agentList
	width, _ := c.Size()
	if width-prefsSidebarW < 560 && f.Kind != settings.KindBool {
		wide = true
	}
	row.Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(3).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Text(c, prefTitle(f.Key)).FontSize(13).FontWeight(500).TextColor(p.text).Shrink(1)
					if changed || f.Key == "agentRolePresets" && len(user.AgentRoles) > 0 {
						ui.Box(c.Key("changed")).Size(6, 6).Radius(3).Shrink(0).Background(p.accent).Transition(dotPop)
					}
				})
				if agentList {
					return
				}
				ui.Text(c, prefDesc(f.Key)).FontSize(12).LineHeight(1.4).TextColor(p.muted)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, f.Key).Font(w.monoFont()).FontSize(10.5).TextColor(p.faint).SingleLine().Shrink(1)
					if scope != nil && !changed {
						ui.Text(c, "·").FontSize(11).TextColor(p.faint)
						ui.Text(c, locales.PrefsInherited()).FontSize(11).TextColor(p.faint).SingleLine().Shrink(1)
					}
				})
			})
			w.resetButton(c, f, scope, changed)
			if f.Key == "agentRolePresets" && w.roleDraft == nil {
				if accentActionButton(c.Key("add-role"), locales.PrefsRoleAdd(), iconPlus, p.accent).Disabled(w.agentConfigurationEditing() || w.agentIOBusy).Clicked() {
					w.openNamedRole("", settings.AgentRole{}, false)
				}
			}
			if !wide {
				ui.Row(c).Shrink(0).Children(func() { w.control(c, f, raw, scope) })
			}
		})
		if wide {
			w.control(c, f, raw, scope)
		}
		if f.Key == "micaEnabled" && scope == nil {
			w.micaPresetControls(c)
		}
	})
}

// monoFont is the code's font, for the keys of the preferences.
func (w *prefsWindow) monoFont() string { return w.m.s.FontFamily }

// resetButton takes the preference back to its default, or the folder's
// choice of it away: it shows while there is something to take back.
func (w *prefsWindow) resetButton(c *ui.Context, f settings.Field, scope *prefFolder, changed bool) {
	if !changed {
		return
	}
	p := w.pal
	label := locales.PrefsReset()
	if scope != nil {
		label = locales.PrefsInherit()
	} else if f.Key == "agentRolePresets" {
		label = locales.PrefsRoleRestoreAll()
	}
	b := ui.ButtonBase(c.Key("prefs-reset-"+f.Key)).Size(26, 26).Radius(6).Shrink(0).
		AlignItems(ui.Center).Justify(ui.Center).Label(label).Tooltip(label)
	if f.Key == "agentServers" || f.Key == "agentRolePresets" || f.Key == "copilot" {
		b.Disabled(w.agentConfigurationEditing() || w.agentIOBusy)
	}
	col := p.faint
	if b.Hovered() {
		b.Background(p.hover)
		col = p.text
	}
	b.Transition(revealFade)
	pressPose(b)
	b.Children(func() {
		ui.Icon(c, iconUndo).FontSize(13).TextColor(col)
	})
	if b.Clicked() {
		w.reset(f.Key, scope)
	}
}

// reset takes the preference of key back to its default, or the folder's
// choice of it away, with a toast that undoes it.
func (w *prefsWindow) reset(key string, scope *prefFolder) {
	name := prefTitle(key)
	if scope != nil {
		dir, was := scope.dir, scope.f.Raw(key)
		w.setFolder(scope, key, nil)
		w.undoable("reset:"+key, locales.PrefsWasInherited(name), func() {
			for _, f := range w.prefFolders() {
				if f.dir == dir {
					w.setFolder(&f, key, was)
				}
			}
		})
		return
	}
	if key == "agentServers" {
		w.resetAgentServers()
		return
	}
	if key == "agentRolePresets" {
		w.resetRolePresets(nil)
		return
	}
	was := w.m.s.Prefs.Raw(key)
	d := settings.Default().Prefs
	w.set(key, d.Raw(key))
	w.undoable("reset:"+key, locales.PrefsWasReset(name), func() { w.set(key, was) })
}

// control is the control that changes a preference.
func (w *prefsWindow) control(c *ui.Context, f settings.Field, raw json.RawMessage, scope *prefFolder) {
	k := c.Key("prefs-control-" + f.Key)
	apply := func(v any) {
		data, err := json.Marshal(v)
		if err != nil {
			return
		}
		if scope != nil {
			w.setFolder(scope, f.Key, data)
		} else {
			w.set(f.Key, data)
		}
	}
	if f.Key == "keymap" {
		w.keymapControl(k, apply)
		return
	}
	if f.Key == "formatOnSave" {
		var rules map[string]bool
		json.Unmarshal(raw, &rules)
		on := rules["*"]
		ui.Row(k).Gap(8).AlignItems(ui.Center).Children(func() {
			if w.first(w.toggle(k.Key("all"), &on).Label(locales.FormatAllLanguages())).Changed() {
				if rules == nil {
					rules = make(map[string]bool)
				}
				if on {
					rules["*"] = true
				} else {
					delete(rules, "*")
				}
				apply(rules)
			}
			if actionButton(k.Key("json"), locales.PrefsEditInJSON(), iconPencil).Clicked() {
				w.editSettingsJSON(scope)
			}
		})
		return
	}
	if f.Key == "agentServers" {
		w.agentServersControl(k)
		return
	}
	if f.Key == "copilot" {
		w.copilotControl(k)
		return
	}
	if f.Key == "agentRolePresets" {
		w.agentRolesControl(k)
		return
	}
	if f.Key == "theme" {
		var chosen string
		json.Unmarshal(raw, &chosen)
		w.themeTiles(c, chosen, apply)
		return
	}
	if f.Key == "iconTheme" {
		var chosen string
		json.Unmarshal(raw, &chosen)
		w.iconThemeTiles(k, chosen, apply)
		return
	}
	switch f.Kind {
	case settings.KindBool:
		var on bool
		json.Unmarshal(raw, &on)
		if w.first(w.toggle(k, &on).Label(prefTitle(f.Key))).Changed() {
			apply(on)
		}
	case settings.KindEnum:
		var v string
		json.Unmarshal(raw, &v)
		labels := make([]string, len(f.Options))
		chosen := ""
		for i, o := range f.Options {
			labels[i] = prefOption(f.Key, o)
			if o == v {
				chosen = labels[i]
			}
		}
		if chosen == "" {
			// A value of the file the window doesn't offer, as "zh".
			chosen = v
			labels = append(labels, v)
		}
		disabled := f.Key == "appearance" && alwaysDark(w.m.s.Theme)
		if segmentedFits(labels) {
			if i, ok := w.segmented(c, "prefs-control-"+f.Key, slices.Index(labels, chosen), labels, prefTitle(f.Key), disabled); ok && i < len(f.Options) {
				apply(f.Options[i])
			}
			return
		}
		sel := w.first(ui.Select(k, &chosen, labels).Label(prefTitle(f.Key)).MinWidth(180))
		if disabled {
			sel.Disabled(true)
		}
		if sel.Changed() {
			if i := slices.Index(labels, chosen); i >= 0 && i < len(f.Options) {
				apply(f.Options[i])
			}
		}
	case settings.KindNumber:
		var v float64
		json.Unmarshal(raw, &v)
		if prefSliders[f.Key] {
			w.sliderControl(c, f, v, apply)
			return
		}
		buf := w.numbers[f.Key]
		if buf == nil {
			buf = new(float64)
			w.numbers[f.Key] = buf
		}
		if !w.editing[f.Key] {
			*buf = v
		}
		in := w.first(ui.NumberInput(k, buf, f.Min, f.Max, f.Step).Label(prefTitle(f.Key)).Width(132))
		w.editing[f.Key] = in.Focused()
		if in.Changed() && *buf != v {
			apply(*buf)
		}
	case settings.KindText:
		var v string
		json.Unmarshal(raw, &v)
		buf := w.textBuffer(f.Key, v)
		if prefFonts[f.Key] {
			w.fontPicker(c, f, v, buf, apply)
			return
		}
		in := w.first(ui.TextInput(k, buf).Label(prefTitle(f.Key)).Width(240))
		if ph := prefText("prefHint", f.Key); ph != prefKey("prefHint", f.Key) {
			in.Placeholder(ph)
		}
		w.draft(f.Key, v, buf, in.Focused(), in.Submitted(), apply)
	case settings.KindList:
		var tokens []string
		json.Unmarshal(raw, &tokens)
		if w.first(ui.TokenField(k, &tokens, w.listSuggestions(f.Key)).Label(prefTitle(f.Key))).Changed() {
			if tokens == nil {
				tokens = []string{}
			}
			apply(tokens)
		}
	case settings.KindStyles:
		w.stylesControl(c, raw)
	case settings.KindJSON:
		if actionButton(k, locales.PrefsEditInJSON(), iconPencil).AlignSelf(ui.Start).Clicked() {
			w.editSettingsJSON(scope)
		}
	}
}

func (w *prefsWindow) editSettingsJSON(scope *prefFolder) {
	if scope == nil {
		w.inFront(func(a *app) { a.openSettings() })
		return
	}
	scope.h.do(scope.h.a.openFolderSettings)
	scope.h.show()
}

// textBuffer is what the field of the text preference of key holds: v,
// unless the field has the keyboard.
func (w *prefsWindow) textBuffer(key, v string) *string {
	buf := w.texts[key]
	if buf == nil {
		buf = new(string)
		w.texts[key] = buf
	}
	if !w.editing[key] {
		*buf = v
	}
	return buf
}

// draft applies what is typed in the field of the text preference of key
// as Enter is pressed or the field is left, rather than at each key,
// which would lay every window out anew: a field that goes away with its
// page applies it as the window is drawn without it.
func (w *prefsWindow) draft(key, v string, buf *string, focused, submitted bool, apply func(any)) {
	was := w.editing[key]
	w.editing[key] = focused
	w.drawn[key] = true
	delete(w.drafts, key)
	switch {
	case *buf == v:
	case submitted || was && !focused:
		apply(*buf)
	case focused:
		w.drafts[key] = func() { apply(*buf) }
	}
}

// settleDrafts applies what was typed in the fields the window no longer
// shows.
func (w *prefsWindow) settleDrafts() {
	for key, apply := range w.drafts {
		if !w.drawn[key] {
			delete(w.drafts, key)
			w.editing[key] = false
			apply()
		}
	}
	clear(w.drawn)
}

// listSuggestions are what a list of a preference offers.
func (w *prefsWindow) listSuggestions(key string) []string {
	switch key {
	case "fontFeatures":
		return []string{"calt=0", "liga=0", "zero", "ss01", "ss02", "ss03", "ss04", "ss05", "cv01", "cv02"}
	}
	return nil
}

// stylesControl edits the looks of the projects on the rail: a label and
// a color for each folder that has one, and what takes it away.
func (w *prefsWindow) stylesControl(c *ui.Context, raw json.RawMessage) {
	p := w.pal
	var styles map[string]settings.TabStyle
	json.Unmarshal(raw, &styles)
	dirs := slices.Sorted(func(yield func(string) bool) {
		for d := range styles {
			if !yield(d) {
				return
			}
		}
	})
	ui.Column(c.Key("prefs-styles")).Gap(6).Children(func() {
		if len(dirs) == 0 {
			ui.Text(c, locales.PrefsNoStyles()).FontSize(12).TextColor(p.faint)
			return
		}
		colors, colorLabels := []string{""}, []string{locales.PrefsStyleOwnColor()}
		for _, rc := range railColors {
			colors, colorLabels = append(colors, rc.name), append(colorLabels, rc.label())
		}
		for _, dir := range dirs {
			st := styles[dir]
			ui.Row(c.Key("prefs-style-"+dir)).Gap(8).AlignItems(ui.Center).Padding(6, 10).Radius(6).
				Background(p.hover).Children(func() {
				ui.Text(c, shortenHome(dir)).FontSize(12).TextColor(p.text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
				buf := w.texts["style:"+dir]
				if buf == nil {
					buf = new(string)
					w.texts["style:"+dir] = buf
				}
				if !w.editing["style:"+dir] {
					*buf = st.Label
				}
				in := ui.TextInput(c.Key("prefs-style-label-"+dir), buf).Label(locales.PrefsStyleLabel()).
					Placeholder(locales.PrefsStyleLabel()).Width(72)
				w.editing["style:"+dir] = in.Focused()
				if in.Changed() {
					st.Label = railLabel(*buf)
					w.setTabStyle(dir, st)
				}
				chosen := colorLabels[0]
				if i := slices.Index(colors, st.Color); i >= 0 {
					chosen = colorLabels[i]
				}
				if ui.Select(c.Key("prefs-style-color-"+dir), &chosen, colorLabels).Label(locales.PrefsStyleColor()).Width(130).Changed() {
					if i := slices.Index(colorLabels, chosen); i >= 0 {
						st.Color = colors[i]
						w.setTabStyle(dir, st)
					}
				}
				if st.Icon != "" {
					ui.Icon(c, iconImage).FontSize(13).TextColor(p.muted).Tooltip(st.Icon)
				}
				x := ui.ButtonBase(c.Key("prefs-style-x-" + dir)).Width(24).Height(24).Radius(5).AlignItems(ui.Center).
					Justify(ui.Center).Label(locales.PrefsStyleRemove()).Tooltip(locales.PrefsStyleRemove())
				if x.Hovered() {
					x.Background(p.surfaceHover)
				}
				x.Children(func() { ui.Icon(c, iconX).FontSize(12).TextColor(p.muted) })
				if x.Clicked() {
					w.setTabStyle(dir, settings.TabStyle{})
				}
			})
		}
	})
}

// setTabStyle gives the project of dir its look on the rail, letting go
// of the icon it no longer shows.
func (w *prefsWindow) setTabStyle(dir string, st settings.TabStyle) {
	old := w.m.s.ProjectTabStyle[dir].Icon
	w.m.s.SetTabStyle(dir, st)
	if st.Icon != old {
		w.m.forgetIcon(old)
	}
	for _, h := range w.m.wins {
		h.a.invalidate()
	}
}

// set makes raw the person's preference of key, in every window at once.
func (w *prefsWindow) set(key string, raw json.RawMessage) {
	p := w.m.s.Prefs
	if err := p.Set(key, raw); err != nil {
		w.errors = append(w.errors, err.Error())
		return
	}
	w.m.reload(p)
	w.m.s.Changed()
}

// setFolder has the folder choose raw for the preference of key, or no
// longer choose it for nil.
func (w *prefsWindow) setFolder(scope *prefFolder, key string, raw json.RawMessage) {
	if err := scope.f.Set(key, raw); err != nil {
		w.errors = append(w.errors, locales.FailSave(scope.f.Path(), err.Error()))
	}
	a := scope.h.a
	a.post(a.folderPrefsChanged)
	w.invalidate()
}

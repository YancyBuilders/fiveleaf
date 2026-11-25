package main

import (
	"cmp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
	"fiveleaf/locales"
)

const (
	pluginsAll = iota
	pluginsInstalled
	pluginsAvailable
)

type pluginFilters struct {
	query        string
	status, kind int
}

type pluginCategory struct {
	label func() string
	// id is the plugin.json category, empty for All.
	id string
	// declared are the contributions that imply this category for a
	// plugin whose manifest has no category.
	declared []string
	// tags are index tags that imply this category for an entry that
	// gives none.
	tags []string
	// whenPresent shows the category only while a plugin, installed or
	// indexed, has it: a new category need not leave an empty tab.
	whenPresent bool
}

var pluginCategories = []pluginCategory{
	{label: locales.PluginsAllCategories},
	{label: locales.PluginsLanguages, id: "languages", declared: []string{"languages", "languageservers"}, tags: []string{"language", "languages", "lsp", "languageserver", "languageservers"}},
	{label: locales.PluginsThemes, id: "themes", declared: []string{"themes"}, tags: []string{"themes", "theme", "colortheme"}},
	{label: locales.PluginsIconThemes, id: "icon-themes", declared: []string{"iconthemes"}, tags: []string{"icontheme", "iconthemes", "icons"}},
	{label: locales.PluginsDebugAdapters, id: "debug-adapters", declared: []string{"debugadapters"}, tags: []string{"debugadapter", "dap"}, whenPresent: true},
	{label: locales.PluginsMCPServers, id: "mcp-servers", tags: []string{"mcp", "mcpserver", "mcpservers", "mcp-servers"}, whenPresent: true},
	{label: locales.PluginsAgents, id: "agents", declared: []string{"agents", "agentservers"}, tags: []string{"agent", "acp"}},
	{label: locales.PluginsSnippets, id: "snippets", declared: []string{"snippets"}, tags: []string{"snippets"}},
	{label: locales.PluginsFormatters, id: "formatters", declared: []string{"formatters"}, tags: []string{"formatter", "format"}},
	{label: locales.PrefTitleKeymap, id: "keymaps", declared: []string{"keymaps"}, tags: []string{"keymap"}},
	{label: locales.PluginsTools, id: "tools", declared: []string{"toolbox"}, tags: []string{"toolbox"}},
}

// pluginTagKey normalizes a tag before it is compared with a category's
// contributions.
var pluginTagKey = strings.NewReplacer("-", "", "_", "", " ", "")

func (category pluginCategory) matches(id string) bool {
	return category.id == "" || category.id == id
}

// implied reports whether a plugin that declares contributions, and no
// category, belongs to this one.
func (category pluginCategory) implied(ct *plugin.Contributes) bool {
	for _, kind := range category.declared {
		switch kind {
		case "languages":
			if len(ct.Languages) > 0 {
				return true
			}
		case "languageservers":
			if len(ct.LanguageServers) > 0 {
				return true
			}
		case "themes":
			if len(ct.Themes) > 0 {
				return true
			}
		case "iconthemes":
			if len(ct.IconThemes) > 0 {
				return true
			}
		case "debugadapters":
			if len(ct.DebugAdapters) > 0 {
				return true
			}
		case "agentservers":
			if len(ct.Agents)+len(ct.AgentServers) > 0 {
				return true
			}
		case "snippets":
			if len(ct.Snippets) > 0 {
				return true
			}
		case "formatters":
			if len(ct.Formatters) > 0 {
				return true
			}
		case "keymaps":
			if len(ct.Keymaps) > 0 {
				return true
			}
		case "toolbox":
			if ct.Toolbox != nil {
				return true
			}
		}
	}
	return false
}

// pluginCategoryLabel is the name of a category in the person's language,
// or "" for one this build does not know.
func pluginCategoryLabel(id string) string {
	index := slices.IndexFunc(pluginCategories, func(category pluginCategory) bool { return category.id == id })
	if index < 0 {
		return ""
	}
	return pluginCategories[index].label()
}

func (pg *pluginsPage) setIndex(items []indexItem) {
	pg.index, pg.shown, pg.shownFor = items, nil, pluginFilters{}
	pg.categories = pg.categories[:0]
	for index, category := range pluginCategories {
		indexed := slices.ContainsFunc(items, func(entry indexItem) bool {
			return category.matches(entry.Category) || entry.Category == "" && category.impliedTags(entry.kinds)
		})
		installed := category.id != "" && slices.ContainsFunc(plugins.reg.Plugins(), func(p *plugin.Plugin) bool {
			return category.matches(p.Category) || p.Category == "" && category.implied(&p.Contributes)
		})
		if index == 0 || !category.whenPresent || indexed || installed {
			pg.categories = append(pg.categories, index)
		}
	}
	if !slices.Contains(pg.categories, pg.filters.kind) {
		pg.filters.kind = 0
	}
}

// impliedTags reports whether an index entry that gives no category names
// this one in its tags.
func (category pluginCategory) impliedTags(kinds []string) bool {
	return slices.ContainsFunc(category.tags, func(tag string) bool { return slices.Contains(kinds, tag) })
}

type pluginCatalogRow struct {
	name, title string
	installed   *pluginItem
	indexed     *indexItem
}

func (entry *indexItem) replacesStock(item *pluginItem) bool {
	if entry == nil || !item.Bundled || item.ToolboxPage() == "" || !slices.Contains(entry.Tags, "toolbox") {
		return false
	}
	release, compatible := entry.Newest()
	stock, stockOK := plugin.ParseSemver(item.Version)
	replacement, replacementOK := plugin.ParseSemver(release.Version)
	return compatible && stockOK && replacementOK && plugin.CompareSemver(replacement, stock) > 0
}

func (row pluginCatalogRow) matches(category pluginCategory, words []string) bool {
	id := ""
	ct := (*plugin.Contributes)(nil)
	var kinds []string
	if row.installed != nil {
		id = row.installed.Category
		ct = &row.installed.Contributes
	} else if row.indexed != nil {
		id = row.indexed.Category
		kinds = row.indexed.kinds
	}
	implied := ct != nil && category.implied(ct) || category.impliedTags(kinds)
	if !category.matches(id) && (id != "" || !implied) {
		return false
	}
	for _, word := range words {
		local := row.installed != nil && strings.Contains(row.installed.hay, word)
		remote := row.indexed != nil && strings.Contains(row.indexed.hay, word)
		if !local && !remote {
			return false
		}
	}
	return true
}

func (pg *pluginsPage) catalogRows() []pluginCatalogRow {
	if pg.shown != nil && pg.filters == pg.shownFor {
		return pg.shown
	}
	remaining := make(map[string]*indexItem, len(pg.index))
	for index := range pg.index {
		entry := &pg.index[index]
		remaining[entry.Name] = entry
	}
	words := strings.Fields(strings.ToLower(pg.filters.query))
	category := pluginCategories[pg.filters.kind]
	shown := make([]pluginCatalogRow, 0, len(pg.items)+len(remaining))
	for index := range pg.items {
		item := &pg.items[index]
		row := pluginCatalogRow{item.Name, item.Title(), item, remaining[item.Name]}
		if pg.filters.status == pluginsAvailable && row.indexed.replacesStock(item) {
			continue
		}
		delete(remaining, item.Name)
		if pg.filters.status != pluginsAvailable && row.matches(category, words) {
			shown = append(shown, row)
		}
	}
	if pg.filters.status != pluginsInstalled {
		for _, entry := range remaining {
			row := pluginCatalogRow{name: entry.Name, title: entry.title, indexed: entry}
			if row.matches(category, words) {
				shown = append(shown, row)
			}
		}
	}
	slices.SortFunc(shown, func(left, right pluginCatalogRow) int {
		return cmp.Or(strings.Compare(strings.ToLower(left.title), strings.ToLower(right.title)), strings.Compare(left.name, right.name))
	})
	pg.shown, pg.shownFor = shown, pg.filters
	return shown
}

func (w *prefsWindow) pluginCatalogActions(c *ui.Context) {
	pg := &w.plugins
	if actionButton(c.Key("plugins-dev"), locales.PluginsInstallDev(), iconFolderPlus).
		Disabled(plugins == nil || pg.busy != "").Clicked() {
		w.chooseDevPlugin()
	}
	more := ui.Button(c.Key("plugins-more"), "").Size(28, 28).Padding(0).Justify(ui.Center).AlignItems(ui.Center).
		Label(locales.PluginsActions()).Tooltip(locales.PluginsActions())
	more.Children(func() { ui.Icon(c, iconMore).FontSize(15).TextColor(w.pal.muted) })
	more.Menu(func(menu *ui.Menu) {
		if menu.Item(locales.PluginsInstallGit()).Disabled(plugins == nil || pg.busy != "").Chosen() {
			pg.installOpen, pg.listScroll.Y = true, 0
		}
		if menu.Item(locales.CmdPluginsReload()).Disabled(plugins == nil || pg.busy != "").Chosen() {
			go plugins.reloadAll()
			if !pg.reading && pg.filters.status != pluginsInstalled {
				w.readIndex()
			}
		}
		if menu.Item(locales.CmdPluginsOpenFolder()).Chosen() {
			w.inFront(func(a *app) { a.openPluginsFolder() })
		}
	})
}

func (w *prefsWindow) pluginCatalogToolbar(c *ui.Context) {
	pg, palette := &w.plugins, w.pal
	if pg.categories == nil {
		pg.setIndex(pg.index)
	}
	width, _ := c.Size()
	compact := width-prefsSidebarW < 560
	ui.Column(c.Key("plugins-toolbar")).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(palette.border).Children(func() {
		ui.Column(c).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(prefsPageWidth).Padding(0, 24, 10, 24).Gap(8).Children(func() {
			bar := ui.Row(c).Gap(8).AlignItems(ui.Center)
			if compact {
				bar.Column().AlignItems(ui.Stretch)
			}
			bar.Children(func() {
				field := ui.Box(c.Key("plugin-search")).MinWidth(0)
				if !compact {
					field.Grow(1).Basis(0)
				}
				field.Children(func() {
					search := searchInput(c.Key("query"), &pg.filters.query, locales.PluginsIndexFilter(), iconSearch).
						Height(30).Padding(4, 30, 4, 35).FontSize(12.5).Label(locales.PluginsIndexFilter()).Bind(&pg.searchH)
					if pg.filters.query != "" {
						ui.Box(c.Key("clear")).Absolute().Right(2).Top(2).Children(func() {
							if w.iconButton(c, "clear-query", iconX, locales.LogClear(), palette.muted, false) || search.Shortcut(0, ui.KeyEscape) {
								pg.filters.query = ""
								pg.searchH.Focus(c)
							}
						})
					}
				})
				ui.Segmented(c.Key("plugin-status"), &pg.filters.status, locales.KeysFilterAll(), locales.PluginsInstalledMark(), locales.PluginsNotInstalled()).
					Label(locales.PluginsInstallStatus()).AlignSelf(ui.Start).Shrink(0)
			})
			selected := slices.Index(pg.categories, pg.filters.kind)
			tabs := ui.TabsBase(c.Key("plugin-categories"), &selected, len(pg.categories))
			tabs.List.Wrap().Gap(3).Label(locales.PluginsAllCategories()).Children(func() {
				for index, kind := range pg.categories {
					label := pluginCategories[kind].label()
					tab := tabs.Tab(index).Height(24).Padding(0, 7).Radius(5).Border(1, ui.Transparent).Label(label).Transition(hoverFade)
					color := palette.muted
					if index == selected {
						color = palette.accent
						tab.Background(palette.active)
					} else if tab.Hovered() {
						color = palette.text
						tab.Background(palette.hover)
					}
					if tab.FocusVisible() {
						tab.BorderColor(palette.accent)
					}
					tab.Children(func() { ui.Text(c, label).FontSize(12).TextColor(color).SingleLine() })
				}
			})
			pg.filters.kind = pg.categories[selected]
		})
	})
	if pg.filters != pg.shownFor {
		pg.listScroll.Y, pg.confirm = 0, ""
	}
}

func (w *prefsWindow) pluginCatalogResults(c *ui.Context) {
	pg := &w.plugins
	withIndex := pg.filters.status != pluginsInstalled
	if withIndex && !pg.indexOK && !pg.reading {
		w.readIndex()
	}
	rows := pg.catalogRows()
	ui.Row(c.Key("plugin-count")).Height(24).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, locales.RunningCount(len(rows))).FontSize(12).FontWeight(500).FontFeatures("tnum").TextColor(w.pal.muted)
		if withIndex && pg.reading && len(rows) > 0 {
			ui.Spinner(c).Size(12, 12).Label(locales.PluginsIndexReading())
		}
		if pg.filters != (pluginFilters{}) {
			ui.Spacer(c)
			if actionButton(c.Key("reset"), locales.LogClearFilters(), nil).MinHeight(22).Padding(2, 8).Clicked() {
				pg.filters, pg.listScroll.Y, pg.confirm = pluginFilters{}, 0, ""
			}
		}
	})
	if withIndex && pg.indexErr != "" {
		w.note(c, "index-err", iconCloudAlert, w.pal.dangerFg, pg.indexErr, nil)
		if retryButton(c.Key("index-retry"), locales.PluginsIndexRetry()).AlignSelf(ui.Start).Disabled(pg.reading).Clicked() {
			w.readIndex()
		}
		if len(rows) == 0 {
			return
		}
	}
	w.card(c, "plugins-list", func() {
		for index, row := range rows {
			if row.installed != nil {
				w.pluginRow(c, row.installed, row.indexed, index > 0)
			} else {
				w.indexRow(c, row.indexed, index > 0)
			}
		}
		if len(rows) > 0 {
			return
		}
		switch {
		case withIndex && pg.reading:
			w.indexPlaceholders(c)
		case strings.TrimSpace(pg.filters.query) != "":
			w.emptyState(c, "plugins-no-match", iconSearch, locales.PluginsIndexNoMatch(strings.TrimSpace(pg.filters.query)))
		case pg.filters.kind != 0:
			w.emptyState(c, "plugins-no-match", iconSearch, locales.PluginsCategoryEmpty())
		case pg.filters.status == pluginsAvailable:
			w.emptyState(c, "plugins-none", iconBox, locales.PluginsNoAvailable())
		default:
			w.emptyState(c, "plugins-none", iconPlug, locales.PluginsNone())
		}
	})
}

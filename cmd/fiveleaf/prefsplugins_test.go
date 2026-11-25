package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

const prettierManifest = `{
  "name": "prettier", "displayName": "Prettier", "version": "3.1.0", "author": "FiveLeaf Editor",
  "description": "Formats JavaScript, TypeScript, CSS and Markdown with Prettier",
  "main": "main.ts",
  "contributes": {
    "commands": [{"id": "prettier.format", "title": "Format with Prettier"}, {"id": "prettier.check", "title": "Check Formatting"}],
    "keybindings": [{"key": "ctrl+alt+shift+p", "command": "prettier.format"}],
    "menus": {"editor/context": [{"command": "prettier.format"}]},
    "statusItems": [{"id": "prettier.status", "align": "left"}],
    "formatters": [{"id": "prettier.code", "title": "Prettier", "languages": ["javascript", "typescript", "css", "markdown"]}],
    "settings": {
      "printWidth": {"type": "number", "default": 100, "description": "The line length to wrap at"},
      "semi": {"type": "boolean", "default": true, "description": "Semicolons at the ends of statements"}
    }
  }
}`

// withPluginsPage opens the Plugins page of the settings window over
// plugins of each kind: from Git with an update waiting, copied in,
// broken, and those FiveLeaf comes with; and an index of three.
func withPluginsPage(t *testing.T) (*prefsHarness, *pluginHost) {
	ph := newPrefsHarness(t)
	h := withPlugins(ph.harness, map[string]map[string]string{
		"prettier": {"plugin.json": prettierManifest, "main.ts": ``},
		"broken":   {"plugin.json": `{"name": "broken", "version": "0.1.0", "description": "Its manifest names no file", "main": 3}`},
		"rainbow":  {"plugin.json": `{"name": "rainbow", "displayName": "Rainbow Brackets", "version": "1.2.0", "description": "Colors brackets by how deep they are", "main": "main.ts"}`, "main.ts": ``},
	})
	h.setPrefs = ph.w.set
	plugin.WriteRecords(h.reg.Dirs.Plugins, plugin.Records{"prettier": {URL: "https://github.com/mayoi-editor/plugin-prettier", Commit: "abc"}})
	installCatalogFixtures(ph.harness, h, "agent-*", "lsp-*", "word-count", "todos", "format-*")
	h.mu.Lock()
	h.updates = map[string]string{"prettier": "def"}
	h.mu.Unlock()
	ph.w.setPluginSetting("rainbow", "enabled", false, true)
	pg := &ph.w.plugins
	pg.setIndex(indexItems([]plugin.IndexEntry{
		{Name: "prettier", DisplayName: "Prettier", Description: "Formats code with Prettier", Repository: "https://github.com/mayoi-editor/plugin-prettier", Tags: []string{"format"}},
		{Name: "eslint", DisplayName: "ESLint", Description: "Finds problems in JavaScript and TypeScript", Author: "FiveLeaf Editor", Repository: "https://github.com/mayoi-editor/plugin-eslint", Tags: []string{"lint"}},
		{Name: "git-blame", Description: "Who changed the line at the caret, in the status bar", Repository: "https://github.com/ann/git-blame"},
	}))
	pg.indexOK = true
	ph.w.section = settings.CatPlugins
	ph.frames()
	return ph, h
}

func installTestAgent(t *testing.T, name string) string {
	t.Helper()
	bin := t.TempDir()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable := filepath.Join(bin, name)
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return executable
}

func TestAgentPluginEnablePreflight(t *testing.T) {
	ph, host := withPluginsPage(t)
	t.Setenv("PATH", t.TempDir())
	host.reg.Load()
	const name = "agent-opencode"
	ph.w.plugins.filters.query = name
	ph.frames()
	label := locales.PluginsEnable("OpenCode Agent")
	warning := locales.PluginsMissingAgentCommand("opencode")
	check := func(enabled, warned bool) {
		t.Helper()
		if ph.pt.HasText(warning) != warned {
			t.Fatalf("want warning=%t in %q", warned, ph.pt.Texts())
		}
		if stored := host.Settings(name); (len(stored) != 0) != enabled {
			t.Fatalf("want enabled=%t, stored %s", enabled, stored)
		}
		listed := slices.ContainsFunc(ph.a.agentProviders(), func(provider agentProvider) bool { return provider.plugin == name })
		if listed != enabled {
			t.Fatalf("want Agent selectable=%t, got %t", enabled, listed)
		}
	}
	check(false, true)
	prefsAgentClick(t, ph, label)
	check(false, true)
	ph.w.openDetail(name)
	ph.frames()
	prefsAgentClick(t, ph, label)
	check(false, true)

	executable := installTestAgent(t, "opencode")
	host.reg.Load()
	ph.frames()
	check(false, false)
	prefsAgentClick(t, ph, label)
	check(true, false)
	prefsAgentClick(t, ph, label)
	check(false, false)

	ph.w.closeDetail()
	ph.frames()
	if err := os.Remove(executable); err != nil {
		t.Fatal(err)
	}
	prefsAgentClick(t, ph, label)
	check(false, true)
	host.setEnabled(name, true)
	check(false, true)
}

// Scroll the chosen control into the list's viewport independently of how
// many bundled plugins precede it.
func prefsPluginScrollTo(t *testing.T, ph *prefsHarness, label string) {
	t.Helper()
	bounds, found := ph.pt.Find(label)
	if !found {
		t.Fatalf("plugin list has no %q", label)
	}
	ph.w.plugins.listScroll.Y += bounds.Y - 240
	ph.frames()
}

// The page lists where each plugin came from as the plugins change, asks
// before it removes one, and filters the index.
func TestPluginsPage(t *testing.T) {
	ph, h := withPluginsPage(t)
	pg := &ph.w.plugins
	sources := map[string]string{}
	for _, it := range pg.items {
		sources[it.Name] = it.source
	}
	if sources["prettier"] != "mayoi-editor/plugin-prettier" || sources["rainbow"] != locales.PluginsLocal() ||
		sources["word-count"] != locales.PluginsLocal() {
		t.Errorf("sources %v", sources)
	}
	if !ph.pt.HasText(locales.PluginsBy("FiveLeaf Editor")) || !ph.pt.HasText(locales.PluginsUpdateWaits()) {
		t.Errorf("no author or update shows: %q", ph.pt.Texts())
	}
	rows := pg.catalogRows()
	prettier := slices.IndexFunc(rows, func(row pluginCatalogRow) bool { return row.name == "prettier" })
	if len(rows) != len(pg.items)+2 || prettier < 0 || rows[prettier].indexed == nil || rows[prettier].installed == nil {
		t.Fatalf("installed and indexed plugins did not share one row: %v", rows)
	}

	// The list is kept while the plugins don't change.
	items := pg.items
	ph.frames()
	if &pg.items[0] != &items[0] {
		t.Error("the list was made anew without a change")
	}

	pg.filters.query = "broken"
	ph.frames()
	prefsPluginScrollTo(t, ph, locales.PluginsRemove())
	if err := ph.pt.Click(locales.PluginsRemove()); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if pg.confirm != "broken" || !ph.pt.HasText(locales.PluginsRemoveAsk("broken")) {
		t.Fatalf("no confirm of the removal: %q, %q", pg.confirm, ph.pt.Texts())
	}
	if err := ph.pt.Click(locales.ButtonCancel()); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if pg.confirm != "" {
		t.Errorf("Cancel left the confirm of %s", pg.confirm)
	}
	if _, err := os.Stat(h.reg.Plugin("broken").Dir); err != nil {
		t.Errorf("the plugin is gone: %v", err)
	}

	// The settings show in the details, which the row's link opens.
	if ph.pt.HasText("printWidth") {
		t.Error("the row shows the plugin's settings")
	}
	pg.filters.query = "word-count"
	ph.frames()
	prefsPluginScrollTo(t, ph, locales.PluginsSettingsCount(1))
	if err := ph.pt.Click(locales.PluginsSettingsCount(1)); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if pg.detail != "word-count" {
		t.Fatalf("the settings link opened %q", pg.detail)
	}
	ph.w.closeDetail()
	ph.frames()

	pg.filters.query = "LINT"
	ph.frames()
	if got := pg.catalogRows(); len(got) != 1 || got[0].name != "eslint" || got[0].installed != nil {
		t.Errorf("the filter finds %v", got)
	}
	pg.filters.query = "nothing like it"
	ph.frames()
	if !ph.pt.HasText(locales.PluginsIndexNoMatch("nothing like it")) {
		t.Errorf("no word of nothing found: %q", ph.pt.Texts())
	}
}

func TestPinnedPluginDoesNotOfferUpdate(t *testing.T) {
	ph, host := withPluginsPage(t)
	const name = "prettier"
	dir := host.reg.Dirs.Plugins
	records := plugin.ReadRecords(dir)
	src := records[name]
	src.Version = "3.1.0"
	records[name] = src
	if err := plugin.WriteRecords(dir, records); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	ph.w.openDetail(name)
	ph.frames()
	if _, shown := ph.pt.Find(locales.PluginsUpdate()); shown || ph.pt.HasText(locales.PluginsUpdateWaits()) {
		t.Fatal("a fixed version still offers an update")
	}
}

func TestPluginCatalogFilters(t *testing.T) {
	ph, host := withPluginsPage(t)
	pg := &ph.w.plugins
	click := func(label string) {
		t.Helper()
		if err := ph.pt.Click(label); err != nil {
			t.Fatal(err)
		}
		ph.frames()
	}
	click(locales.PluginsInstalledMark())
	click(locales.PluginsAgents())
	if len(pg.catalogRows()) == 0 || !ph.pt.HasText("Codex Agent") || ph.pt.HasText("Go Language Server") {
		t.Fatal("installed Agent results did not match the manifest contributions")
	}
	ph.w.finding = true
	ph.frames()
	ph.pt.Type("CoDeX")
	ph.frames()
	if rows := pg.catalogRows(); len(rows) != 1 || rows[0].name != "agent-codex" || !rows[0].installed.Stats().Disabled {
		t.Fatalf("search for a disabled bundled agent: %+v", rows)
	}
	click("Codex Agent")
	if pg.detail != "agent-codex" || !ph.pt.HasText("agent-codex.acp") {
		t.Fatal("the Agent result did not open its contributed server")
	}
	ph.pt.Key(0, ui.KeyEscape)
	ph.frames()
	if pg.filters.query != "CoDeX" || pg.filters.status != pluginsInstalled || len(pg.catalogRows()) != 1 {
		t.Fatal("returning from details lost the search or filters")
	}
	click(locales.LogClearFilters())
	click(locales.PluginsNotInstalled())
	if len(pg.catalogRows()) != 2 || ph.pt.HasText("Prettier") || !ph.pt.HasText("ESLint") {
		t.Fatal("not-installed results included a local plugin or omitted an index entry")
	}
	click(locales.PluginsIconThemes())
	if !ph.pt.HasText(locales.PluginsCategoryEmpty()) {
		t.Fatal("no empty state for a category without matches")
	}
	pg.setIndex(append(pg.index, indexItems([]plugin.IndexEntry{{Name: "paper", DisplayName: "Paper Icons", Tags: []string{"Icon_Themes"}}})...))
	ph.frames()
	if !ph.pt.HasText("Paper Icons") || len(pg.catalogRows()) != 1 {
		t.Fatal("the index tag did not match its category")
	}
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, plugin.ManifestFile), []byte(`{"name":"paper","displayName":"Paper Icons","contributes":{"iconThemes":[{"id":"paper.icons","label":"Paper","path":"icons.json"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := host.reg.Dirs.LinkDev(folder); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	ph.frames()
	if len(pg.catalogRows()) != 0 {
		t.Fatal("a newly linked plugin stayed in not-installed results")
	}
	pg.indexErr = "Index offline"
	click(locales.PluginsInstalledMark())
	if !ph.pt.HasText("Paper Icons") || !ph.pt.HasText(locales.PluginsDev()) || ph.pt.HasText(pg.indexErr) {
		t.Fatal("the installed list did not work independently of the index")
	}
	click(locales.PluginsRemove())
	if !ph.pt.HasText(locales.PluginsUnlinkAsk("Paper Icons")) {
		t.Fatal("removing a development link did not describe what happens to the source")
	}
	click(locales.KeysFilterAll())
	if !ph.pt.HasText(pg.indexErr) || !ph.pt.HasText(locales.PluginsIndexRetry()) || !ph.pt.HasText("Paper Icons") || pg.confirm != "" {
		t.Fatal("changing filters kept a removal confirmation or hid offline plugins and retry")
	}
}

func TestPluginCatalogIndexOutage(t *testing.T) {
	ph, _ := withPluginsPage(t)
	pg := &ph.w.plugins
	pg.filters.status = pluginsAll
	pg.setIndex(nil)
	saved := indexItems([]plugin.IndexEntry{{Name: "saved-theme", DisplayName: "Saved Theme"}})
	live := indexItems([]plugin.IndexEntry{{Name: "latest-theme", DisplayName: "Latest Theme"}})
	outage := errors.New("HTTP 503 Service Unavailable")
	pg.finishIndexRead(saved, outage)
	ph.frames()
	if !ph.pt.HasText("Saved Theme") || !ph.pt.HasText(locales.PluginsIndexSaved(outage.Error())) || !ph.pt.HasText(locales.PluginsIndexRetry()) {
		t.Fatal("first load during a server outage should show the bundled list and a retry button")
	}
	pg.finishIndexRead(live, nil)
	ph.frames()
	if !ph.pt.HasText("Latest Theme") || ph.pt.HasText("Saved Theme") || ph.pt.HasText(locales.PluginsIndexRetry()) {
		t.Fatal("a successful refresh should replace the bundled list and clear the notice")
	}
	pg.finishIndexRead(saved, outage)
	ph.frames()
	if !ph.pt.HasText("Latest Theme") || ph.pt.HasText("Saved Theme") || !ph.pt.HasText(locales.PluginsIndexRetry()) {
		t.Fatal("a failed refresh should keep the last live list and offer retry")
	}
	ph.w.installPlugin("saved-theme@v1", nil)
	if pg.busy != "" || pg.err != pg.indexErr {
		t.Fatal("a typed name absent from the saved live list should report the outage before installing")
	}
	pg.finishIndexRead(indexItems([]plugin.IndexEntry{}), nil)
	pg.finishIndexRead(saved, outage)
	if len(pg.index) != 0 {
		t.Fatal("a failed refresh repopulated an empty live index")
	}
	pg.finishIndexRead(indexItems(nil), nil)
	pg.finishIndexRead(saved, outage)
	if len(pg.index) != 1 || pg.index[0].Name != "saved-theme" {
		t.Fatal("an earlier 404 hid the bundled catalog during a server outage")
	}
}

func TestPluginCatalogIndexCategories(t *testing.T) {
	ph, _ := withPluginsPage(t)
	pg := &ph.w.plugins
	if ph.pt.HasText(locales.PluginsMCPServers()) || ph.pt.HasText(locales.PluginsDebugAdapters()) {
		t.Fatal("index-only categories showed without an entry")
	}
	pg.filters = pluginFilters{}
	entries := indexItems([]plugin.IndexEntry{
		{Name: "mcp-remote", Category: "mcp-servers"},
		{Name: "go-debug", Category: "debug-adapters"},
	})
	pg.index, pg.shown = entries, nil
	pg.shownFor = pluginFilters{}
	pg.setIndex(entries)
	ph.frames()
	for _, choice := range []struct{ category, plugin string }{{locales.PluginsMCPServers(), "mcp-remote"}, {locales.PluginsDebugAdapters(), "go-debug"}} {
		if err := ph.pt.Click(choice.category); err != nil {
			t.Fatal(err)
		}
		ph.frames()
		kind := slices.IndexFunc(pluginCategories, func(category pluginCategory) bool {
			return category.id == map[string]string{locales.PluginsMCPServers(): "mcp-servers", locales.PluginsDebugAdapters(): "debug-adapters"}[choice.category]
		})
		pg.filters.kind = kind
		pg.shown = nil
		pg.shown, pg.shownFor = nil, pluginFilters{}
		ph.frames()
		if rows := pg.catalogRows(); len(rows) != 1 || rows[0].name != choice.plugin {
			t.Fatalf("%s lists %v", choice.category, rows)
		}
	}
	pg.setIndex(nil)
	ph.frames()
	if pg.filters.kind != 0 || !ph.pt.HasText("Rainbow Brackets") {
		t.Fatal("a removed index category left the catalog on a hidden tab")
	}
	if err := ph.pt.Click(locales.PluginsActions()); err != nil {
		t.Fatal(err)
	}
	if err := ph.pt.ChooseMenuItem(locales.PluginsInstallGit()); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if !pg.installOpen || !ph.pt.HasText(locales.PluginsInstallTitle()) {
		t.Fatal("the Git install form did not open from Plugin Actions")
	}
}

// Languages and language servers are one filter: a plugin that declares
// only one of them still shows, and the old separate tabs are gone.
func TestPluginCatalogLanguageFilterMergesServers(t *testing.T) {
	ph, _ := withPluginsPage(t)
	pg := &ph.w.plugins
	pg.setIndex(indexItems([]plugin.IndexEntry{
		{Name: "syntax-only", DisplayName: "Syntax Only", Tags: []string{"language"}},
		{Name: "server-only", DisplayName: "Server Only", Tags: []string{"lsp"}},
		{Name: "both", DisplayName: "Both", Tags: []string{"language", "lsp"}},
	}))
	ph.frames()
	if ph.pt.HasText(locales.PluginsLanguageServers()) {
		t.Fatal("the separate language server filter still shows")
	}
	if err := ph.pt.Click(locales.PluginsLanguages()); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	rows := pg.catalogRows()
	for _, name := range []string{"syntax-only", "server-only", "both"} {
		if !slices.ContainsFunc(rows, func(row pluginCatalogRow) bool { return row.name == name }) {
			t.Errorf("merged language filter omitted %s", name)
		}
	}
}

// An installed plugin that declares a debug adapter gets the filter even
// when the index only holds other tags.
func TestPluginCatalogDebugAdapterContribution(t *testing.T) {
	ph := newPrefsHarness(t)
	withPlugins(ph.harness, map[string]map[string]string{
		"go-debug": {"plugin.json": `{"name":"go-debug","contributes":{"debugAdapters":[{"id":"go-debug.dlv","languages":["go"],"command":"dlv","args":["dap"]}]}}`},
	})
	ph.frames()
	pg := &ph.w.plugins
	pg.setIndex(nil)
	if !slices.ContainsFunc(pg.categories, func(index int) bool {
		return pluginCategories[index].label() == locales.PluginsDebugAdapters()
	}) {
		t.Fatal("the debug adapter filter is missing")
	}
}

// A row opens the details of its plugin, which list what it adds once,
// read how it does only while they show, and go back to the list where
// it was with Esc.
func TestPluginDetail(t *testing.T) {
	ph, h := withPluginsPage(t)
	pg := &ph.w.plugins
	redrawn := 0
	h.prefsWin = func() { redrawn++ }

	prefsPluginScrollTo(t, ph, locales.PluginsRemove())
	if err := ph.pt.Click(locales.PluginsRemove()); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if pg.detail != "" {
		t.Fatalf("Remove opened the details of %s", pg.detail)
	}
	pg.confirm = ""
	ph.pt.SetSize(940, 520)
	ph.frames()
	prefsPluginScrollTo(t, ph, "Rainbow Brackets")
	scrolled := pg.listScroll.Y
	if scrolled == 0 {
		t.Fatal("the list does not scroll")
	}

	if err := ph.pt.Click("Rainbow Brackets"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if pg.detail != "rainbow" || !ph.pt.HasText(locales.PluginsBack()) {
		t.Fatalf("the row opened no details: %q", pg.detail)
	}
	ph.pt.Key(0, ui.KeyEscape)
	ph.frames()
	if pg.detail != "" {
		t.Fatal("Esc left the details")
	}
	if pg.listScroll.Y != scrolled {
		t.Errorf("the list came back scrolled to %v, not %v", pg.listScroll.Y, scrolled)
	}

	ph.w.openDetail("prettier")
	ph.frames()
	for _, want := range []string{strings.ToUpper(locales.PluginsHealth()), locales.PluginsMemory(), locales.PluginsCommands(), "Format with Prettier",
		"prettier.check", locales.PluginsMenus(), "editor/context", locales.PluginsStatusItems(), locales.PluginsFormatters(),
		"javascript, typescript, css, markdown", "printWidth", locales.PluginsLogOpen()} {
		if !ph.pt.HasText(want) {
			t.Errorf("the details show no %q", want)
		}
	}
	for _, none := range []string{locales.PluginsThemes(), locales.PluginsLanguageServers(), locales.PluginsSnippets()} {
		if ph.pt.HasText(none) {
			t.Errorf("the details show the empty group %q", none)
		}
	}
	d := &pg.info
	groups, at := &d.groups[0], d.at
	ph.frames()
	if &d.groups[0] != groups {
		t.Error("what the plugin adds was worked out anew without a change")
	}
	if d.at != at {
		t.Error("the health was read again before its time")
	}

	pl := h.reg.Plugin("prettier")
	pl.Log("error", "could not find prettier")
	ph.settle()
	ph.pt.Advance(pluginHealthEvery)
	ph.frames()
	if !ph.pt.HasText("could not find prettier") {
		t.Errorf("the log shows no new line: %q", ph.pt.Texts())
	}
	if redrawn == 0 {
		t.Error("an error logged did not draw the settings window")
	}

	ph.w.closeDetail()
	ph.frames()
	if pg.info.name != "" {
		t.Error("the details kept what they read once closed")
	}
}

func TestFormatterPluginSettingControls(t *testing.T) {
	ph, host := withPluginsPage(t)
	ph.pt.SetSize(940, 820)
	ph.w.openDetail("format-ruff")
	ph.frames()
	clickControl := func(key string) {
		t.Helper()
		reset, ok := ph.pt.Find(locales.PrefsReset() + " · " + key)
		if !ok {
			t.Fatalf("missing control for %s", key)
		}
		ph.pt.ClickAt(reset.X-20, reset.Y+reset.H/2)
		ph.frames()
	}
	stored := func(key string) string {
		t.Helper()
		var values map[string]json.RawMessage
		json.Unmarshal(host.Settings("format-ruff"), &values)
		return string(values[key])
	}
	clickControl("lineLength")
	ph.pt.Type("96")
	ph.pt.Key(0, ui.KeyEnter)
	ph.frames()
	if got := stored("lineLength"); got != "96" {
		t.Fatalf("lineLength = %q", got)
	}
	ph.pt.Key(ui.Cmd, ui.KeyA)
	ph.pt.Type("96abc")
	ph.pt.Key(0, ui.KeyEnter)
	ph.frames()
	if got := stored("lineLength"); got != "96" {
		t.Fatalf("invalid input replaced the saved number with %q", got)
	}
	clickControl("quoteStyle")
	if err := ph.pt.Click("single"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if got := stored("quoteStyle"); got != `"single"` {
		t.Fatalf("quoteStyle = %q", got)
	}
	if err := ph.pt.Click(locales.PrefsReset() + " · lineLength"); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	ph.m.s.Flush()
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(settings.Load(ph.m.s.Path()).Plugins["format-ruff"], &saved); err != nil || saved["lineLength"] != nil || string(saved["quoteStyle"]) != `"single"` {
		t.Fatalf("saved Ruff settings = %v, %v", saved, err)
	}
	clickControl("quoteStyle")
	if err := ph.pt.Click(locales.KeysDefault()); err != nil {
		t.Fatal(err)
	}
	ph.frames()
	if got := stored("quoteStyle"); got != "" {
		t.Fatalf("Default kept the quote override %s", got)
	}
	ph.pt.SetSize(640, 760)
	ph.w.openDetail("format-clang")
	ph.frames()
	if reset, ok := ph.pt.Find(locales.PrefsReset() + " · fallbackStyle"); !ok || reset.X < prefsSidebarW || reset.X+reset.W > 640 {
		t.Fatalf("narrow window clipped the style selector: %v", reset)
	}
}

func TestShotPluginsPage(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	for _, dark := range []bool{true, false} {
		suffix := "-light"
		if dark {
			suffix = "-dark"
		}
		ph, h := withPluginsPage(t)
		ph.pt.SetDark(dark)
		ph.pt.SetSize(940, 1500)
		ph.frames()
		ph.shot("plugins" + suffix)
		if r, ok := ph.pt.Find(locales.PluginsRemove()); ok {
			ph.pt.Move(r.X+r.W/2, r.Y+r.H/2)
		}
		ph.frames()
		ph.shot("plugins-hover" + suffix)
		ph.pt.Click(locales.PluginsRemove())
		ph.frames()
		ph.shot("plugins-confirm" + suffix)
		pg := &ph.w.plugins
		pg.confirm = ""
		pg.err, pg.errFor = "git clone failed: repository not found", ""
		pg.filters.status = pluginsAvailable
		pg.reading = true
		ph.pt.SetSize(940, 1100)
		ph.frames()
		ph.shot("plugins-reading" + suffix)
		pg.reading, pg.indexErr = false, locales.PluginsIndexFailed("dial tcp: no such host")
		ph.frames()
		ph.shot("plugins-index-failed" + suffix)
		pg.indexErr = ""
		pg.setIndex(nil)
		ph.frames()
		ph.shot("plugins-index-empty" + suffix)
		pg.err = ""
		pl := h.reg.Plugin("prettier")
		pl.Log("info", "activated")
		pl.Log("warn", "no .prettierrc found, using the defaults")
		pl.Log("error", "could not parse src/app.ts: Unexpected token (12:4)")
		ph.w.openDetail("prettier")
		ph.pt.SetSize(940, 1500)
		ph.frames()
		ph.shot("plugin-detail" + suffix)
		ph.w.openDetail("word-count")
		ph.frames()
		ph.shot("plugin-detail-bundled" + suffix)
		ph.pt.Key(0, ui.KeyEscape)
		ph.frames()
		ph.shot("plugins-back" + suffix)
	}
}

func TestShotPluginCatalog(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	for _, width := range []int{940, 640} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			previous := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return "zh-Hans" })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return previous }) })
			ph, _ := withPluginsPage(t)
			ph.pt.SetLocale("zh-Hans")
			ph.pt.SetDark(width == 640)
			ph.pt.SetSize(width, 680)
			ph.frames()
			for _, label := range []string{locales.PluginsInstallDev(), locales.PluginsIndexFilter(), locales.PluginsNotInstalled(), locales.PrefTitleKeymap()} {
				bounds, found := ph.pt.Find(label)
				if !found || bounds.X < prefsSidebarW || bounds.X+bounds.W > float32(width) || bounds.Y+bounds.H > 250 {
					t.Errorf("clipped catalog control %q at %v", label, bounds)
				}
			}
			if err := ph.pt.Click(locales.PluginsLanguages()); err != nil {
				t.Fatal(err)
			}
			ph.frames()
			ph.shot("plugin-catalog-" + strconv.Itoa(width))
		})
	}
}

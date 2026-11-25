package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func toolboxTestPlugins(hs *harness, enabled ...string) *pluginHost {
	hs.t.Helper()
	host := withPlugins(hs, nil)
	host.reg.Dirs.Bundled = plugin.UserDirs().Bundled
	host.reg.Locale = i18n.Locale
	if hs.a.settings.Plugins == nil {
		hs.a.settings.Plugins = map[string]json.RawMessage{}
	}
	for _, name := range enabled {
		hs.a.settings.Plugins["toolbox-"+name] = json.RawMessage(`{"enabled":true}`)
	}
	host.syncPrefs()
	host.reg.Load()
	hs.settle()
	return host
}

func TestToolboxLauncherAndCategory(t *testing.T) {
	ph := newPrefsHarness(t)
	host := toolboxTestPlugins(ph.harness)
	host.setPrefs = ph.w.set
	ph.w.plugins.indexOK = true
	entries := indexItems([]plugin.IndexEntry{
		{Name: "toolbox-base64", Category: "tools", Version: "1.1.0", Tags: []string{"toolbox"}},
		{Name: "external", Category: "tools"},
		{Name: "other", Category: "themes"},
	})
	ph.w.plugins.index, ph.w.plugins.shown = entries, nil
	ph.w.plugins.setIndex(entries)
	ph.w.navigate(settings.CatPlugins, toolboxCategoryKey, nil)
	ph.frames()
	if rows := ph.w.plugins.catalogRows(); len(rows) != 5 {
		t.Fatalf("toolbox category: %+v", rows)
	}
	if !ph.pt.HasText(locales.PluginsInstall() + " 1.1.0") {
		t.Fatal("the installed stock tool has no marketplace installation action")
	}
	ph.w.plugins.filters.status = pluginsAvailable
	if rows := ph.w.plugins.catalogRows(); !slices.ContainsFunc(rows, func(row pluginCatalogRow) bool { return row.name == "toolbox-base64" && row.installed == nil }) {
		t.Fatal("the marketplace release is missing from available tools")
	}
	ph.w.plugins.filters.status = pluginsAll
	ph.frames()
	if _, found := ph.tt.Find(locales.ToolboxTitle()); found {
		t.Fatal("launcher shown before a tool was enabled")
	}
	for _, on := range []bool{true, false} {
		if err := ph.pt.Click(locales.PluginsEnable("Base64")); err != nil {
			t.Fatal(err)
		}
		ph.frames()
		ph.settle()
		// The settings window is in front; the sidebar is the main window's.
		if ph.w.win != nil {
			ph.w.win.Hide()
		}
		ph.frames()
		button, found := ph.tt.Find(locales.ToolboxTitle())
		if found != on {
			t.Fatalf("launcher visible = %t after setting enabled = %t", found, on)
		}
		if on {
			sessions, ok := ph.tt.Find(locales.CmdViewSessions())
			if !ok || button.X < sessions.X+sessions.W-1 || button.Y != sessions.Y {
				t.Fatalf("launcher %v should follow Sessions %v", button, sessions)
			}
			before := ph.a.window.SidebarView
			previousTab := ph.a.activeTab()
			ph.tt.ClickAt(button.X+button.W/2, button.Y+button.H/2)
			ph.settle()
			if !ph.a.quick.open || !ph.a.quick.tools || ph.a.quick.query != "" || ph.a.window.SidebarView != before || ph.a.activeTab() != previousTab {
				t.Fatalf("launcher left open=%v tools=%v query=%q view=%q", ph.a.quick.open, ph.a.quick.tools, ph.a.quick.query, ph.a.window.SidebarView)
			}
			ph.a.closeQuick()
			ph.frames()
		}
	}
}

// Hiding the Toolbox button leaves the tools in the command palette.
func TestToolboxButtonHides(t *testing.T) {
	hs := newHarness(t, true)
	host := toolboxTestPlugins(hs, "base64", "json")
	if len(host.toolboxPlugins()) != 2 {
		t.Fatalf("enabled tools: %v", host.toolboxPlugins())
	}
	if _, found := hs.tt.Find(locales.ToolboxTitle()); !found {
		t.Fatal("the Toolbox button is missing")
	}
	hs.a.settings.HideToolboxButton = true
	hs.settle()
	if _, found := hs.tt.Find(locales.ToolboxTitle()); found {
		t.Fatal("the Toolbox button stays after it is hidden")
	}
	if sessions, found := hs.tt.Find(locales.CmdViewSessions()); !found || sessions.X > 170 {
		t.Fatalf("the hidden Toolbox button left a gap: %v", sessions)
	}
	hs.a.openQuick(">")
	hs.settle()
	var tools int
	for _, match := range hs.a.quick.results {
		if strings.HasPrefix(hs.a.paletteEntries()[match.Index].id, toolboxCommand) {
			tools++
		}
	}
	if tools != 2 {
		t.Fatalf("the palette lists %d tools of 2", tools)
	}
}

// The button's switch is in the settings' Layout card, beside the
// sidebar's.
func TestToolboxButtonSetting(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.w.navigate(settings.CatAppearance, "hideToolboxButton", nil)
	ph.frames()
	label := prefTitle("hideToolboxButton")
	if !ph.pt.HasText(label) {
		t.Fatalf("Appearance omits %q", label)
	}
	ph.w.set("hideToolboxButton", json.RawMessage(`true`))
	ph.frames()
	if !ph.a.settings.HideToolboxButton || !ph.m.s.HideToolboxButton {
		t.Fatal("the switch did not hide the Toolbox button")
	}
	ph.shot("prefs-toolbox-button")
}

// The tools list is the command palette's own, not the toolbox window's
// card: the same dialog, prompt field and list of rows.
func TestToolboxListLooksLikeThePalette(t *testing.T) {
	for _, dark := range []bool{false, true} {
		hs := newHarness(t, dark)
		toolboxTestPlugins(hs, "base64", "json", "timestamp", "url")
		a, tt := hs.a, hs.tt
		tt.SetSize(960, 720)
		a.quickToolsPicker()
		hs.settle()
		if _, found := tt.Find(locales.PaletteLabel()); found {
			t.Fatal("the tools list is titled as the palette, not as the toolbox")
		}
		if tt.HasText("$") {
			t.Fatal("the tools list shows its mode marker as text")
		}
		messages := locales.In(i18n.Locale())
		if !tt.Focused(messages.ToolboxTitle()) || !tt.HasText(messages.QuickOpenAction()) || !tt.HasText(messages.ToolboxTitle()+": Base64") {
			t.Fatalf("the tools list is not a command palette dialog: %q", tt.Texts())
		}
		rows := 0
		for _, match := range a.quick.results {
			entry := a.toolboxEntries()[match.Index]
			if _, ok := tt.Find(entry.title); !ok {
				t.Fatalf("missing tool row %q", entry.title)
			}
			rows++
		}
		if rows != 4 {
			t.Fatalf("the tools list shows %d tools of 4", rows)
		}
		hs.shot(fmt.Sprintf("toolbox-palette-dark-%t", dark))
		if dialog, ok := tt.Find(messages.ToolboxPickerPlaceholder()); ok {
			hs.shotRect(fmt.Sprintf("toolbox-palette-detail-dark-%t", dark),
				image.Rect(int(dialog.X)-40, int(dialog.Y)-120, int(dialog.X+dialog.W)+560, int(dialog.Y+dialog.H)+360), 1)
		}
		tt.Key(0, ui.KeyEscape)
	}
}

// The tool a palette row names opens the toolbox at its page.
func TestToolboxPaletteOpensTool(t *testing.T) {
	hs := newHarness(t, true)
	toolboxTestPlugins(hs, "base64", "json")
	hs.a.quickToolsPicker()
	hs.settle()
	hs.tt.Type("json")
	hs.settle()
	if len(hs.a.quick.results) != 1 {
		t.Fatalf("tools for json: %v", hs.a.quick.results)
	}
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	selected := hs.a.activeTab()
	if selected == nil || selected.tool == nil || selected.tool.toolbox == nil || selected.tool.toolbox.provider.Name != "toolbox-json" {
		t.Fatalf("the palette selected %+v", selected)
	}
}

func TestToolboxPickerRefreshesBeforePluginChangeCallback(t *testing.T) {
	for _, query := range []string{"$", ">$base64"} {
		t.Run(query, func(t *testing.T) {
			hs := newHarness(t, false)
			host := toolboxTestPlugins(hs, "base64", "json")
			hs.a.openQuick(query)
			hs.settle()
			if _, ok := hs.tt.Find(groupTitle(locales.ToolboxTitle(), "Base64")); !ok {
				t.Fatal("Base64 was not listed before the plugin change")
			}
			delete(hs.a.settings.Plugins, "toolbox-base64")
			host.syncPrefs()
			onMain := host.onMain
			var apply func()
			host.onMain = func(change func()) { apply = change }
			host.Changed()
			host.onMain = onMain
			hs.tt.Key(0, ui.KeyEnter)
			if !hs.a.quick.open {
				t.Fatal("confirming the removed tool selected a different tool")
			}
			if _, ok := hs.tt.Find(groupTitle(locales.ToolboxTitle(), "Base64")); ok {
				t.Fatal("picker still listed a disabled tool")
			}
			if _, ok := hs.tt.Find(groupTitle(locales.ToolboxTitle(), "JSON")); ok != (query == "$") {
				t.Fatal("picker used matches from the earlier list")
			}
			apply()
		})
	}
}

func TestToolboxEditorTabLifecycle(t *testing.T) {
	harness := newHarness(t, false)
	owner := harness.a
	host := toolboxTestPlugins(harness, "base64", "json")
	owner.openFile(harness.path("main.go"), true)
	harness.settle()
	source := owner.activeTab()
	first := owner.column()
	owner.openToolboxTab("toolbox-json")
	jsonTab := owner.activeTab()
	if jsonTab == nil || jsonTab.tool == nil || jsonTab.tool.toolbox == nil || jsonTab.name() != "JSON" {
		t.Fatal("the tool did not open as a named editor tab")
	}
	page := jsonTab.tool.toolbox
	page.surface.pal = darkPal
	owner.openToolboxPicker()
	harness.settle()
	if !owner.quick.tools || !owner.quick.open || owner.activeTab() != jsonTab {
		t.Fatal("switching tools lost the current tool tab")
	}
	owner.closeQuick()
	owner.openToolboxTab("toolbox-base64")
	second := owner.splitColumn(first, true)
	owner.moveTab(jsonTab, second, 0)
	owner.activate(first, 0, true)
	owner.focusToolboxTab(page)
	if owner.column() != second || owner.focus != paneEditor {
		t.Fatal("keyboard focus within the page did not select its editor column")
	}
	owner.activate(first, 0, true)
	owner.openToolboxTab("toolbox-json")
	harness.settle()
	if owner.activeTab() != jsonTab || owner.column() != second || len(second.tabs) != 1 || jsonTab.tool.toolbox != page || page.surface.pal != darkPal {
		t.Fatal("reopening a moved tool must reuse its existing page and column")
	}
	if owner.canDetachTab(jsonTab) || jsonTab.canSplit() {
		t.Fatal("a single toolbox page can be duplicated or detached")
	}
	if _, err := host.reg.Dirs.LinkDev(filepath.Join("../../extensions", "toolbox-json")); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	harness.settle()
	if owner.activeTab() != jsonTab || page.provider != host.reg.Plugin("toolbox-json") || page.provider.Bundled || page.surface.pal != nil {
		t.Fatal("installed edition did not replace the stock page in its original tab")
	}
	if err := plugin.Remove(host.reg.Dirs.Plugins, "toolbox-json"); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	harness.settle()
	if owner.activeTab() != jsonTab || !page.provider.Bundled {
		t.Fatal("removal did not restore the stock edition in its tab")
	}
	page.surface.pal = darkPal
	owner.closeToolboxTab(page)
	harness.settle()
	if col, _ := owner.locate(jsonTab); col != nil || page.surface.pal != nil {
		t.Fatal("the page's Close action left its tab open")
	}
	owner.openToolboxTab("toolbox-base64")
	other := owner.activeTab()
	delete(owner.settings.Plugins, "toolbox-base64")
	host.syncPrefs()
	host.Changed()
	harness.settle()
	if col, _ := owner.locate(other); col != nil || owner.activeTab() != source {
		t.Fatal("disabling the tool did not return to the neighboring file")
	}
}

func TestToolboxTabsBelongToTheirProjects(t *testing.T) {
	first := newHarness(t, false)
	second := newHarness(t, false)
	toolboxTestPlugins(first, "json")
	first.a.openToolboxTab("toolbox-json")
	second.a.openToolboxTab("toolbox-json")
	firstTab, secondTab := first.a.activeTab(), second.a.activeTab()
	firstTab.tool.toolbox.surface.pal = darkPal
	secondTab.tool.toolbox.surface.pal = lightPal
	if firstTab.tool.toolbox == secondTab.tool.toolbox {
		t.Fatal("two projects shared a toolbox page")
	}
	second.a.closeToolboxTab(firstTab.tool.toolbox)
	if second.a.activeTab() != secondTab {
		t.Fatal("another project's page closed the active toolbox tab")
	}
	first.a.openFolder(t.TempDir())
	if col, _ := first.a.locate(firstTab); col != nil || second.a.activeTab() != secondTab || firstTab.tool.toolbox.surface.pal != nil || secondTab.tool.toolbox.surface.pal == nil {
		t.Fatal("replacing the project did not release just its own toolbox tab")
	}
}

func TestToolboxPageTracksEnabledPlugins(t *testing.T) {
	hs := newHarness(t, false)
	host := toolboxTestPlugins(hs, "base64", "json")
	page, err := url.Parse(toolboxURL(host.toolboxProvider("toolbox-base64")))
	if err != nil || page.Host != "toolbox-base64.localhost" || page.Query().Get("tools") != "base64" || page.Fragment != "/tool/base64" {
		t.Fatalf("enabled toolbox pages: %v, %v", page, err)
	}
	delete(hs.a.settings.Plugins, "toolbox-base64")
	host.syncPrefs()
	host.Changed()
	if host.toolboxProvider("toolbox-base64") != nil || host.toolboxProvider("toolbox-json") == nil {
		t.Fatal("enabled toolbox providers are stale after disabling Base64")
	}
}

func TestToolboxThemeUsesEditorPalette(t *testing.T) {
	for _, test := range []struct {
		appearance string
		systemDark bool
		want       *palette
	}{
		{"light", true, lightPal}, {"dark", false, darkPal}, {"", true, darkPal}, {"", false, lightPal},
	} {
		theme := webToolboxTheme(prefsPalette(&settings.Prefs{Appearance: test.appearance}, test.systemDark))
		if theme.Dark != test.want.dark || theme.Colors["editor"] != hexColor(test.want.editor) || theme.Colors["active"] != hexColor(test.want.active.Over(test.want.editor)) {
			t.Errorf("appearance %q, system dark %t: %+v", test.appearance, test.systemDark, theme)
		}
	}
	withPlugins(newHarness(t, false), map[string]map[string]string{"night": {"plugin.json": nightPlugin, "night.json": nightTheme, "paper.json": paperTheme}})
	theme := webToolboxTheme(prefsPalette(&settings.Prefs{Theme: "night.dark"}, false))
	if !theme.Dark || theme.Colors["editor"] != "#282a36" || theme.Colors["keyword"] != "#ff79c6" {
		t.Fatalf("plugin palette and syntax: %+v", theme)
	}
}

func TestToolboxPackagedPage(t *testing.T) {
	toolboxTestPlugins(newHarness(t, false), "json")
	files := toolboxFiles()
	get := func(path string) string {
		t.Helper()
		response := httptest.NewRecorder()
		files.ServeHTTP(response, httptest.NewRequest("GET", toolboxScheme+"://toolbox-json.localhost"+path, nil))
		if response.Code != 200 {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		return response.Body.String()
	}
	index := get("/")
	assets := regexp.MustCompile(`(?:src|href)="\./(assets/[^"?]+)"`).FindAllStringSubmatch(index, -1)
	if len(assets) == 0 || !strings.Contains(index, `id="devtool-append"`) {
		t.Fatal("missing original DevTool entry point")
	}
	for _, asset := range assets {
		get("/" + asset[1])
	}
	var source struct{ Features map[string][]string }
	if err := json.Unmarshal([]byte(get("/source.json")), &source); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"time": {"timestamp", "timezone", "calculator"}, "base64": {"encoder", "decoder"},
		"url": {"encoder", "decoder"}, "json": {"common", "io", "object"},
	}
	if len(source.Features) != len(want) {
		t.Fatalf("bundled tools: %v", source.Features)
	}
	for tool, features := range want {
		if !slices.Equal(source.Features[tool], features) {
			t.Errorf("%s feature tabs = %v, want %v", tool, source.Features[tool], features)
		}
	}
}

func TestToolboxMarketplacePages(t *testing.T) {
	harness := newHarness(t, false)
	names := []string{"base64", "json", "timestamp", "url"}
	host := toolboxTestPlugins(harness, names...)
	for _, tool := range names {
		if _, err := host.reg.Dirs.LinkDev(filepath.Join("../../extensions", "toolbox-"+tool)); err != nil {
			t.Fatal(err)
		}
	}
	host.reg.Load()
	files := toolboxFiles()
	links := regexp.MustCompile(`(?:src|href)="(\./[^"?#]+)"`)
	for _, tool := range names {
		provider := host.toolboxProvider("toolbox-" + tool)
		if provider == nil || provider.Bundled || !host.reg.Origin(provider.Name).Linked {
			t.Fatalf("expected marketplace plugin for %s", tool)
		}
		address, err := url.Parse(toolboxURL(provider))
		if err != nil || address.Fragment != "/tool/"+provider.ToolboxPage() || address.Query().Get("tools") != provider.ToolboxPage() {
			t.Fatalf("%s did not open its DevTool route: %v, %v", tool, address, err)
		}
		address.Fragment = ""
		entry := httptest.NewRecorder()
		files.ServeHTTP(entry, httptest.NewRequest("GET", address.String(), nil))
		resources := links.FindAllStringSubmatch(entry.Body.String(), -1)
		if entry.Code != http.StatusOK || len(resources) == 0 || !strings.Contains(entry.Body.String(), `id="devtool-append"`) {
			t.Fatalf("%s returned %d with no plugin page resources", address, entry.Code)
		}
		for _, link := range resources {
			asset := strings.TrimPrefix(link[1], "./")
			response := httptest.NewRecorder()
			assetURL, err := address.Parse(link[1])
			if err != nil {
				t.Fatal(err)
			}
			files.ServeHTTP(response, httptest.NewRequest("GET", assetURL.String(), nil))
			if response.Code != http.StatusOK || response.Body.Len() == 0 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s%s returned %d and %d bytes", address, asset, response.Code, response.Body.Len())
			}
			if strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("%s returned HTML for %s", tool, asset)
			}
		}
	}
}

// A tool of a user's plugin is served from the plugin's folder, at the
// route of its id, with its own files.
func TestToolboxPluginPage(t *testing.T) {
	hs := newHarness(t, false)
	host := withPlugins(hs, map[string]map[string]string{
		"hello-tool": {
			"plugin.json":      `{"name":"hello-tool","version":"1.0.0","category":"tools","contributes":{"toolbox":{"id":"hello.tool","title":"Hello","page":"pages/index.html","features":["upper"]}}}`,
			"pages/index.html": `<link rel="stylesheet" href="style.css"><script src="app.js"></script>`,
			"pages/style.css":  `main { color: red }`,
			"pages/app.js":     `document.title = 'Hello'`,
			"other.txt":        "private file",
		},
	})
	if p := host.reg.Plugin("hello-tool"); p == nil {
		t.Fatal("the plugin did not load")
	} else if err := p.Stats().Err; err != "" {
		t.Fatalf("the plugin did not load: %s", err)
	}
	if len(host.toolboxPlugins()) != 1 || host.toolboxPlugins()[0].ToolboxPage() != "hello.tool" {
		t.Fatalf("toolbox plugins: %+v", host.toolboxPlugins())
	}
	files := toolboxFiles()
	if got := hs.a.toolboxEntries()[0].title; got != groupTitle(locales.ToolboxTitle(), "Hello") {
		t.Fatalf("custom tool title = %q", got)
	}
	get := func(path string) string {
		t.Helper()
		response := httptest.NewRecorder()
		files.ServeHTTP(response, httptest.NewRequest("GET", toolboxScheme+"://hello-tool.localhost"+path, nil))
		if response.Code != 200 {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		return response.Body.String()
	}
	if page := get("/"); !strings.Contains(page, "style.css") {
		t.Fatalf("the tool's page: %q", page)
	}
	if css := get("/style.css"); !strings.Contains(css, "color: red") {
		t.Fatalf("the tool's stylesheet: %q", css)
	}
	if js := get("/app.js"); !strings.Contains(js, "Hello") {
		t.Fatalf("the tool's script: %q", js)
	}
	provider := host.toolboxProvider("hello-tool")
	if err := os.Symlink("../other.txt", filepath.Join(provider.Dir, "pages", "escape.txt")); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"hello-tool.localhost/escape.txt", "hello-tool.localhost/%2e%2e/other.txt", "hello-tool.localhost/plugin.json", "hello-tool.localhost/.git/config", "localhost/", "unknown.localhost/"} {
		response := httptest.NewRecorder()
		files.ServeHTTP(response, httptest.NewRequest("GET", toolboxScheme+"://"+route, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s returned %d", route, response.Code)
		}
	}
	if err := os.WriteFile(filepath.Join(provider.Dir, "pages", "app.js"), []byte("changed script"), 0o600); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	if host.toolboxProvider(provider.Name) == provider || get("/app.js") != "changed script" {
		t.Fatal("page remained cached after a development edit")
	}
}

func TestToolboxTimestampLayout(t *testing.T) {
	browser := os.Getenv("FIVELEAF_TEST_CHROMIUM")
	if browser == "" {
		t.Skip("set FIVELEAF_TEST_CHROMIUM to a Chromium executable for the toolbox layout check")
	}
	toolboxTestPlugins(newHarness(t, false), "timestamp")
	mux := http.NewServeMux()
	files := toolboxFiles()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		request.Host = "toolbox-timestamp.localhost"
		files.ServeHTTP(writer, request)
	})
	mux.HandleFunc("GET /layout", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "testdata/toolbox-time.html")
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	page, err := exec.CommandContext(ctx, browser, "--headless", "--disable-background-networking", "--no-first-run",
		"--user-data-dir="+t.TempDir(), "--virtual-time-budget=12000", "--dump-dom", server.URL+"/layout").CombinedOutput()
	if err != nil || !strings.Contains(string(page), `<output>pass</output>`) {
		t.Fatalf("timestamp layout: %v\n%s", err, page)
	}
}

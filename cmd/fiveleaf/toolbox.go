package main

import (
	"archive/zip"
	"bytes"
	"cmp"
	_ "embed"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/plugin"
	"fiveleaf/locales"
)

const toolboxCategoryKey = "category:toolbox"
const toolboxScheme = "fiveleaf-toolbox"

//go:embed toolboxweb/devtool.zip
var toolboxBundle []byte

func toolboxFiles() http.Handler {
	assets, err := zip.NewReader(bytes.NewReader(toolboxBundle), int64(len(toolboxBundle)))
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		id, ok := strings.CutSuffix(req.Host, ".localhost")
		provider := plugins.toolboxProvider(id)
		if !ok || provider == nil {
			http.NotFound(writer, req)
			return
		}
		name := strings.TrimPrefix(req.URL.Path, "/")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		if provider.DevToolPage() != "" {
			name = cmp.Or(name, "index.html")
			data, err := fs.ReadFile(assets, name)
			if err != nil {
				http.NotFound(writer, req)
				return
			}
			serveToolboxAsset(writer, req, name, bytes.NewReader(data))
			return
		}
		tool := provider.Contributes.Toolbox
		file, err := tool.OpenAsset(provider.Dir, name)
		if err != nil {
			http.NotFound(writer, req)
			return
		}
		defer file.Close()
		serveToolboxAsset(writer, req, cmp.Or(name, path.Base(tool.Page)), file)
	})
}

func (host *pluginHost) toolboxPlugins() []*plugin.Plugin {
	if host == nil {
		return nil
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.toolboxList == nil || host.toolboxGen != host.gen {
		host.toolboxList = make([]*plugin.Plugin, 0)
		for _, provider := range host.enabled() {
			if provider.ToolboxPage() != "" {
				host.toolboxList = append(host.toolboxList, provider)
			}
		}
		host.toolboxGen = host.gen
	}
	return host.toolboxList
}

func (host *pluginHost) toolboxProvider(name string) *plugin.Plugin {
	for _, provider := range host.toolboxPlugins() {
		if provider.Name == name {
			return provider
		}
	}
	return nil
}

func toolboxURL(provider *plugin.Plugin) string {
	target := url.URL{Scheme: toolboxScheme, Host: provider.Name + ".localhost", Path: "/"}
	if id := provider.DevToolPage(); id != "" {
		target.RawQuery = url.Values{"tools": {id}}.Encode()
		target.Fragment = "/tool/" + id
	}
	return target.String()
}

type toolboxTheme struct {
	Dark   bool              `json:"dark"`
	Colors map[string]string `json:"colors"`
}

var toolboxThemeChanged = mygo.NewEvent[toolboxTheme]("toolbox-theme")

// toolboxCommand prefixes the commands of the palette that open a tool.
const toolboxCommand = "toolbox:"

// toolboxEntries are the tools of the enabled plugins, as the palette
// groups them under Toolbox, matched by their English names, features
// and descriptions too.
func (a *app) toolboxEntries() []paletteEntry {
	providers := plugins.toolboxPlugins()
	if len(providers) == 0 {
		return nil
	}
	group := locales.ToolboxTitle()
	out := make([]paletteEntry, 0, len(providers))
	for _, provider := range providers {
		name := toolboxTitle(provider)
		title := groupTitle(group, name)
		english := cmp.Or(provider.DisplayName, provider.Name)
		words := []string{provider.Name, provider.Description, english}
		if contrib := provider.Contributes.Toolbox; contrib != nil {
			words = append(words, contrib.ID, contrib.Page)
			words = append(words, contrib.Features...)
			if contrib.Description != "" {
				words = append(words, contrib.Description)
			}
		}
		out = append(out, paletteEntry{id: toolboxCommand + provider.Name, title: title,
			en: groupTitle(group, english), group: len(title) - len(name),
			keywords: strings.Join(words, " "), secondary: toolboxFeatures(provider)})
	}
	slices.SortStableFunc(out, func(x, y paletteEntry) int { return strings.Compare(x.title, y.title) })
	return out
}

// toolboxFeatures names what a tool does, after its name in the list.
func toolboxFeatures(provider *plugin.Plugin) string {
	if id := provider.DevToolPage(); id != "" {
		switch id {
		case "time":
			return locales.ToolboxFeaturesTime()
		case "base64":
			return locales.ToolboxFeaturesBase64()
		case "json":
			return locales.ToolboxFeaturesJSON()
		case "url":
			return locales.ToolboxFeaturesURL()
		}
	}
	if contrib := provider.Contributes.Toolbox; contrib != nil {
		if contrib.Description != "" {
			return contrib.Description
		}
		return strings.Join(contrib.Features, " · ")
	}
	return ""
}

func toolboxTitle(provider *plugin.Plugin) string {
	tool := provider.Contributes.Toolbox
	if tool != nil && tool.Title != "" && tool.Title != provider.DisplayName {
		return tool.Title
	}
	return provider.Title()
}

func (a *app) openToolboxTool(name string) {
	if name == "" {
		a.quickToolsPicker()
		return
	}
	a.openToolboxTab(name)
}

// quickToolsPicker lists the tools, as the sidebar's button does: the
// command palette's own list, narrowed to it.
func (a *app) quickToolsPicker() {
	a.openQuick("")
	a.quick.tools = true
}

// quickTools is quick open listing the tools alone: each tool's name,
// its features, and the plugin that adds it.
func (a *app) quickTools(c *ui.Context, query string) {
	q := &a.quick
	entries := a.toolboxEntries()
	key := "tools:" + query
	query = strings.TrimSpace(query)
	selectionValid := q.matchCommands(entries, key, query, false)
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
	mode := quickMode{label: locales.ToolboxTitle(), prompt: locales.ToolboxPickerPlaceholder(), prefix: "$",
		hiddenPrefix: true, count: locales.ItemsCount(len(q.results)), action: locales.QuickOpenAction(),
		enabled: len(q.results) > 0, choose: choose}
	a.quickPicker(c, mode, func() {
		if len(q.results) == 0 {
			quickMessage(c, iconToolbox, locales.ToolboxNoMatches())
			return
		}
		width, _ := c.Size()
		a.commandResults(c, entries, query == "", width < 520, choose)
	})
}

func webToolboxTheme(p *palette) toolboxTheme {
	return toolboxTheme{Dark: p.dark, Colors: map[string]string{
		"editor": hexColor(p.editor), "panel": hexColor(p.panel), "surface": hexColor(p.surface),
		"text": hexColor(p.text), "muted": hexColor(p.muted), "border": hexColor(p.border.Over(p.editor)),
		"hover": hexColor(p.surfaceHover), "active": hexColor(p.active.Over(p.editor)),
		"accent": hexColor(p.accent), "fill": hexColor(p.fill), "fill-hover": hexColor(p.fillHover), "on-fill": hexColor(p.fillText),
		"success": hexColor(p.addedFg), "warning": hexColor(p.modifiedFg), "danger": hexColor(p.dangerFg),
		"selection": hexColor(p.selected), "gutter": hexColor(p.gutter), "line": hexColor(p.line.Over(p.editor)),
		"key": hexColor(p.syntax[highlight.Attribute]), "string": hexColor(p.syntax[highlight.String]),
		"number": hexColor(p.syntax[highlight.Number]), "keyword": hexColor(p.syntax[highlight.Keyword]), "comment": hexColor(p.syntax[highlight.Comment]),
	}}
}

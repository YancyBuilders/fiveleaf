package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"math"
	"os"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
	"fiveleaf/locales"
)

const defaultIconThemeID = "fiveleaf-icons.fiveleaf"

var iconThemeAliases = [...]appearanceAlias{
	{defaultIconThemeID, "fiveleaf-file-icons.icons"}, {"fiveleaf-icons.catppuccin-latte", "catppuccin-latte-icons.icons"},
	{"fiveleaf-icons.catppuccin-mocha", "catppuccin-mocha-icons.icons"}, {"fiveleaf-icons.material", "material-icons.icons"},
	{"fiveleaf-icons.seti", "seti-icons.icons"}, {"fiveleaf-icons.ayu", "ayu-icons.icons"},
}

type iconThemeChoice struct{ id, name string }

func canonicalIconThemeID(id string) string {
	if id == "" {
		return defaultIconThemeID
	}
	if !strings.Contains(id, ".") {
		return "fiveleaf-icons." + id
	}
	if index := appearanceIndex(iconThemeAliases[:], id); index >= 0 {
		return iconThemeAliases[index].saved
	}
	return id
}

func (choice iconThemeChoice) matches(id string) bool {
	return choice.id == id || choice.id == canonicalIconThemeID(id)
}

type iconThemeFile struct {
	File             string              `json:"file"`
	Dotfile          string              `json:"dotfile"`
	Folder           string              `json:"folder"`
	FolderOpen       string              `json:"folderOpen"`
	FileNames        map[string]string   `json:"fileNames"`
	FileNamePrefixes map[string]string   `json:"fileNamePrefixes"`
	FileExtensions   map[string]string   `json:"fileExtensions"`
	FolderNames      map[string]string   `json:"folderNames"`
	FolderNamesOpen  map[string]string   `json:"folderNamesOpen"`
	Icons            map[string]string   `json:"icons"`
	Tints            map[string]iconTint `json:"tints"`
}

type iconTint struct {
	Role  string `json:"role"`
	Dark  string `json:"dark"`
	Light string `json:"light"`
}

func (tint iconTint) valid() bool {
	if tint.Role != "" {
		return tint.Dark == "" && tint.Light == "" && themeField(&palette{}, tint.Role) != nil
	}
	_, dark := parseThemeColor(tint.Dark)
	_, light := parseThemeColor(tint.Light)
	return dark && light
}

type iconTheme struct {
	iconThemeFile
	icons    map[string]*ui.SVG
	prefixes []string
}

var iconThemes = struct {
	sync.RWMutex
	loaded  bool
	host    *pluginHost
	gen     uint64
	list    []iconThemeChoice
	packs   map[string]*iconTheme
	colored map[*ui.SVG]bool
}{}

func allIconThemes() []iconThemeChoice {
	loadIconThemes()
	iconThemes.RLock()
	defer iconThemes.RUnlock()
	choices := slices.Clone(iconThemes.list)
	if choices[0].id == "" {
		choices[0].name = locales.KeysDefault()
	}
	return choices
}

// A plugin generation replaces the whole snapshot, so a removed or disabled
// pack cannot leave some of its icons active in another window.
func loadIconThemes() {
	host, gen := plugins, pluginGen()
	iconThemes.RLock()
	current := iconThemes.loaded && iconThemes.host == host && iconThemes.gen == gen
	iconThemes.RUnlock()
	if current {
		return
	}
	packs := map[string]*iconTheme{}
	var ordered [len(iconThemeAliases)]iconThemeChoice
	var other []iconThemeChoice
	if host != nil && host.reg != nil {
		for _, p := range host.enabled() {
			for _, contribution := range p.Contributes.IconThemes {
				id := canonicalIconThemeID(contribution.ID)
				if contribution.ID == "" || (packs[id] != nil && contribution.ID == id) {
					continue
				}
				pack, err := readPluginIconTheme(p, contribution.Path)
				if err != nil {
					p.Log("error", fmt.Sprintf("Icon theme %s: %v", contribution.ID, err))
					continue
				}
				packs[id] = pack
				choice := iconThemeChoice{id, contribution.Label}
				if index := appearanceIndex(iconThemeAliases[:], id); index >= 0 {
					ordered[index] = choice
				} else {
					other = append(other, choice)
				}
			}
		}
	}
	list := []iconThemeChoice{ordered[0]}
	for _, choice := range ordered[1:] {
		if choice.id != "" {
			list = append(list, choice)
		}
	}
	list = append(list, other...)
	colored := map[*ui.SVG]bool{}
	for _, pack := range packs {
		for id, icon := range pack.icons {
			if _, tinted := pack.Tints[id]; !tinted {
				colored[icon] = true
			}
		}
	}
	iconThemes.Lock()
	iconThemes.loaded, iconThemes.host, iconThemes.gen = true, host, gen
	iconThemes.list, iconThemes.packs, iconThemes.colored = list, packs, colored
	iconThemes.Unlock()
}

func readPluginIconTheme(p *plugin.Plugin, name string) (*iconTheme, error) {
	if p.Bundled {
		return readIconTheme(p.FS(), name)
	}
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return readIconTheme(root.FS(), name)
}

const maxIconFileBytes = 1 << 20

func readIconFile(fsys fs.FS, name string) ([]byte, error) {
	if !validIconPath(name) {
		return nil, fmt.Errorf("invalid icon path %q", name)
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxIconFileBytes+1))
	if err == nil && len(data) > maxIconFileBytes {
		err = fmt.Errorf("%s exceeds the 1 MiB icon limit", name)
	}
	return data, err
}

func validIconPath(name string) bool {
	return name != "." && fs.ValidPath(name) && !strings.ContainsAny(name, `\:`)
}

func readIconTheme(fsys fs.FS, name string) (*iconTheme, error) {
	data, err := readIconFile(fsys, name)
	if err != nil {
		return nil, err
	}
	var f iconThemeFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if f.File == "" || f.Folder == "" {
		return nil, fmt.Errorf("%s needs file and folder icons", name)
	}
	if len(f.Icons) == 0 || len(f.Icons) > 2048 {
		return nil, fmt.Errorf("%s must define from 1 to 2048 icons", name)
	}
	if f.FolderOpen == "" {
		f.FolderOpen = f.Folder
	}
	pack := &iconTheme{iconThemeFile: f, icons: map[string]*ui.SVG{}}
	bytes := 0
	for id, file := range f.Icons {
		if id == "" || !validIconPath(file) || !strings.EqualFold(path.Ext(file), ".svg") {
			return nil, fmt.Errorf("invalid SVG for icon %q", id)
		}
		data, err := readIconFile(fsys, path.Join(path.Dir(name), file))
		if err != nil {
			return nil, err
		}
		bytes += len(data)
		if bytes > 32<<20 {
			return nil, fmt.Errorf("%s exceeds the 32 MiB icon theme limit", name)
		}
		icon, err := parseIconSVG(data)
		if err != nil {
			return nil, fmt.Errorf("icon %s: %w", id, err)
		}
		w, h := icon.Size()
		if w <= 0 || h <= 0 || w > 4096 || h > 4096 || math.IsNaN(float64(w)) || math.IsNaN(float64(h)) {
			return nil, fmt.Errorf("icon %s has invalid dimensions", id)
		}
		pack.icons[id] = icon
	}
	for _, id := range []string{f.File, f.Folder, f.FolderOpen} {
		if pack.icons[id] == nil {
			return nil, fmt.Errorf("undefined icon %q", id)
		}
	}
	if f.Dotfile != "" && pack.icons[f.Dotfile] == nil {
		return nil, fmt.Errorf("undefined dotfile icon %q", f.Dotfile)
	}
	for id, tint := range f.Tints {
		if pack.icons[id] == nil || !tint.valid() {
			return nil, fmt.Errorf("invalid tint for icon %q", id)
		}
	}
	for _, field := range []struct {
		name string
		data *map[string]string
		ext  bool
	}{
		{"fileNames", &pack.FileNames, false},
		{"fileNamePrefixes", &pack.FileNamePrefixes, false},
		{"fileExtensions", &pack.FileExtensions, true},
		{"folderNames", &pack.FolderNames, false},
		{"folderNamesOpen", &pack.FolderNamesOpen, false},
	} {
		normalized := map[string]string{}
		for key, id := range *field.data {
			key = strings.ToLower(key)
			if field.ext {
				key = strings.TrimPrefix(key, ".")
			}
			if key == "" || strings.ContainsAny(key, `/\`) || pack.icons[id] == nil || normalized[key] != "" {
				return nil, fmt.Errorf("invalid %s mapping %q to %q", field.name, key, id)
			}
			normalized[key] = id
		}
		*field.data = normalized
	}
	pack.prefixes = slices.Collect(maps.Keys(pack.FileNamePrefixes))
	slices.SortFunc(pack.prefixes, func(first, second string) int { return len(second) - len(first) })
	return pack, nil
}

// MyGo tolerates incomplete XML, which could turn a broken installed asset
// into an empty icon. Validate the document before handing it to the renderer.
func parseIconSVG(data []byte) (*ui.SVG, error) {
	if err := plugin.ValidateIconSVG(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return ui.ParseSVG(data)
}

func iconThemeByID(id string) *iconTheme {
	loadIconThemes()
	iconThemes.RLock()
	defer iconThemes.RUnlock()
	return iconThemes.packs[canonicalIconThemeID(id)]
}

func selectedIconTheme(id string) *iconTheme {
	if pack := iconThemeByID(id); pack != nil || canonicalIconThemeID(id) == defaultIconThemeID {
		return pack
	}
	return iconThemeByID(defaultIconThemeID)
}

func iconBaseName(name string) string {
	return strings.ToLower(path.Base(strings.ReplaceAll(name, `\`, "/")))
}

func (pack *iconTheme) fileID(name string) string {
	name = iconBaseName(name)
	if id := pack.FileNames[name]; id != "" {
		return id
	}
	for _, prefix := range pack.prefixes {
		if strings.HasPrefix(name, prefix) {
			return pack.FileNamePrefixes[prefix]
		}
	}
	if pack.Dotfile != "" && strings.HasPrefix(name, ".") && !strings.Contains(name[1:], ".") {
		return pack.Dotfile
	}
	// Check compound suffixes first: declaration.d.ts before ts.
	for rest := name; ; {
		i := strings.IndexByte(rest, '.')
		if i < 0 {
			break
		}
		rest = rest[i+1:]
		if id := pack.FileExtensions[rest]; id != "" {
			return id
		}
	}
	return pack.File
}

func (pack *iconTheme) folderID(name string, open bool) string {
	name = iconBaseName(name)
	if open {
		if id := pack.FolderNamesOpen[name]; id != "" {
			return id
		}
	}
	if id := pack.FolderNames[name]; id != "" {
		return id
	}
	if open {
		return pack.FolderOpen
	}
	return pack.Folder
}

func (pack *iconTheme) icon(p *palette, id string, fallback ui.Color) (*ui.SVG, ui.Color) {
	tint, ok := pack.Tints[id]
	if !ok {
		return pack.icons[id], fallback
	}
	if tint.Role != "" {
		return pack.icons[id], *themeField(p, tint.Role)
	}
	return pack.icons[id], langColor(p, tint.Dark, tint.Light)
}

func fileIconForTheme(p *palette, themeID, name string) (*ui.SVG, ui.Color) {
	if pack := selectedIconTheme(themeID); pack != nil {
		return pack.icon(p, pack.fileID(name), p.muted)
	}
	return iconFile, p.muted
}

func folderIconForTheme(p *palette, themeID, name string, open bool) (*ui.SVG, ui.Color) {
	if pack := selectedIconTheme(themeID); pack != nil {
		return pack.icon(p, pack.folderID(name, open), p.accent)
	}
	if open {
		return iconFolderOpen, p.accent
	}
	return iconFolder, p.accent
}

func (a *app) fileIcon(name string) (*ui.SVG, ui.Color) {
	theme := a.settings.IconTheme
	if a.iconPicker.open {
		theme = a.iconPicker.preview
	}
	return fileIconForTheme(a.pal, theme, name)
}

func (a *app) folderIcon(name string, open bool) (*ui.SVG, ui.Color) {
	theme := a.settings.IconTheme
	if a.iconPicker.open {
		theme = a.iconPicker.preview
	}
	return folderIconForTheme(a.pal, theme, name, open)
}

func coloredThemeIcon(icon *ui.SVG) bool {
	iconThemes.RLock()
	defer iconThemes.RUnlock()
	return iconThemes.colored[icon]
}

func iconThemeView(c *ui.Context, p *palette, icon *ui.SVG, tint ui.Color, size float32) ui.Element {
	if coloredThemeIcon(icon) {
		// Painter.Image resolves currentColor against the UI text color too.
		return ui.Image(c, icon).Size(size, size).Shrink(0).TextColor(c.Theme().Text).Opacity(float32(tint.A) / 255)
	}
	return ui.Icon(c, icon).FontSize(size).TextColor(tint).Shrink(0)
}

func paintThemeIcon(pt *ui.Painter, p *palette, icon *ui.SVG, tint ui.Color, r ui.Rect) {
	if coloredThemeIcon(icon) {
		pt.Image(icon, r, ui.Contain)
		return
	}
	pt.Icon(icon, r, tint)
}

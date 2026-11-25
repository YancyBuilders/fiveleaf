package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/plugin"
	"fiveleaf/locales"
)

// The themes plugins contribute: a palette in JSON, over one of FiveLeaf's,
// or a VS Code theme, whose colors and token colors map to FiveLeaf's.

// themeFile is a theme of a plugin as its file says it, FiveLeaf's format and
// VS Code's at once: a VS Code theme has "type" and colors with dots, as
// "editor.background", and "tokenColors".
type themeFile struct {
	// Name is what the theme picker shows.
	Name string `json:"name"`
	// Base is the palette the theme changes: "dark" or "light", FiveLeaf's
	// own, or a theme's id, as "claude".
	Base          string            `json:"base"`
	Dark          *bool             `json:"dark"`
	Mono          *bool             `json:"mono"`
	Colors        map[string]string `json:"colors"`
	Syntax        map[string]string `json:"syntax"`
	SyntaxItalic  map[string]bool   `json:"syntaxItalic"`
	Ansi          []string          `json:"ansi"`
	Metrics       *metricsFile      `json:"metrics"`
	Card          *float32          `json:"card"`
	Grain         *float32          `json:"grain"`
	Sentence      *bool             `json:"sentence"`
	Derived       *bool             `json:"derived"`
	Mascot        *string           `json:"mascot"`
	ActivityLabel *bool             `json:"activityLabel"`
	AccentDialogs *bool             `json:"accentDialogs"`
	WelcomeRain   *bool             `json:"welcomeRain"`
	// VS Code's.
	Type        string `json:"type"`
	TokenColors []struct {
		Scope    json.RawMessage `json:"scope"`
		Settings struct {
			Foreground string  `json:"foreground"`
			FontStyle  *string `json:"fontStyle"`
		} `json:"settings"`
	} `json:"tokenColors"`
}

// metricsFile is a theme's grid of gaps, radii and text, as its file says
// it: a number, as "gap", or all the numbers of the text, as at once.
type metricsFile struct {
	Gap, Pad, Inset                 *float32
	Radius, RadiusField, RadiusCard *float32
	RadiusFull                      *float32
	TextMicro, TextSmall            *float32
	TextBody, TextTitle, TextSize   *float32
	WeightMedium, WeightBold        *int
	Row, Control                    *float32
}

// metricsBounds are the least and most of each number a theme file may
// say: enough for a tighter or roomier theme, and not so much that it
// breaks the window's layout.
var metricsBounds = struct {
	gap, pad, inset                 [2]float32
	radius, radiusField, radiusCard [2]float32
	radiusFull                      [2]float32
	text, row, control              [2]float32
	weight                          [2]int
}{
	gap: [2]float32{1, 24}, pad: [2]float32{0, 40}, inset: [2]float32{0, 48},
	radius: [2]float32{0, 24}, radiusField: [2]float32{0, 24}, radiusCard: [2]float32{0, 40},
	radiusFull: [2]float32{0, 1000},
	text:       [2]float32{6, 28}, row: [2]float32{12, 64}, control: [2]float32{14, 64},
	weight: [2]int{100, 900},
}

// validate reports a metric that is not finite or is out of range.
func (m *metricsFile) validate() error {
	for _, f := range []struct {
		name   string
		v      *float32
		lo, hi float32
	}{
		{"gap", m.Gap, metricsBounds.gap[0], metricsBounds.gap[1]},
		{"pad", m.Pad, metricsBounds.pad[0], metricsBounds.pad[1]},
		{"inset", m.Inset, metricsBounds.inset[0], metricsBounds.inset[1]},
		{"radius", m.Radius, metricsBounds.radius[0], metricsBounds.radius[1]},
		{"radiusField", m.RadiusField, metricsBounds.radiusField[0], metricsBounds.radiusField[1]},
		{"radiusCard", m.RadiusCard, metricsBounds.radiusCard[0], metricsBounds.radiusCard[1]},
		{"radiusFull", m.RadiusFull, metricsBounds.radiusFull[0], metricsBounds.radiusFull[1]},
		{"textMicro", m.TextMicro, metricsBounds.text[0], metricsBounds.text[1]},
		{"textSmall", m.TextSmall, metricsBounds.text[0], metricsBounds.text[1]},
		{"textBody", m.TextBody, metricsBounds.text[0], metricsBounds.text[1]},
		{"textTitle", m.TextTitle, metricsBounds.text[0], metricsBounds.text[1]},
		{"text", m.TextSize, metricsBounds.text[0], metricsBounds.text[1]},
		{"row", m.Row, metricsBounds.row[0], metricsBounds.row[1]},
		{"control", m.Control, metricsBounds.control[0], metricsBounds.control[1]},
	} {
		if err := validateMetric(f.name, f.v, f.lo, f.hi); err != nil {
			return err
		}
	}
	for _, f := range []struct {
		name   string
		v      *int
		lo, hi int
	}{
		{"weightMedium", m.WeightMedium, metricsBounds.weight[0], metricsBounds.weight[1]},
		{"weightBold", m.WeightBold, metricsBounds.weight[0], metricsBounds.weight[1]},
	} {
		if f.v != nil && (*f.v < f.lo || *f.v > f.hi) {
			return fmt.Errorf("metrics.%s is %v, which is not from %v to %v", f.name, *f.v, f.lo, f.hi)
		}
	}
	return nil
}

// validateMetric reports v when it is set but not finite or not between
// lo and hi.
func validateMetric(name string, v *float32, lo, hi float32) error {
	if v == nil {
		return nil
	}
	if !finiteIn(*v, lo, hi) {
		return fmt.Errorf("metrics.%s is %v, which is not from %v to %v", name, *v, lo, hi)
	}
	return nil
}

// finiteIn reports whether v is a finite number from lo to hi.
func finiteIn(v, lo, hi float32) bool {
	x := float64(v)
	return !math.IsNaN(x) && !math.IsInf(x, 0) && v >= lo && v <= hi
}

// UnmarshalJSON reads a theme's metrics: "text": 13 is the body and the
// title, and "text": {"body": 13, "title": 14} each of them.
func (m *metricsFile) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	type plain metricsFile
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if err := (*metricsFile)(&p).validate(); err != nil {
		return err
	}
	*m = metricsFile(p)
	if text := raw["text"]; text != nil {
		var size float32
		if json.Unmarshal(text, &size) == nil {
			if err := validateMetric("text", &size, metricsBounds.text[0], metricsBounds.text[1]); err != nil {
				return err
			}
			m.TextSize = &size
			return nil
		}
		var group struct {
			Micro, Small, Body, Title *float32
		}
		if err := json.Unmarshal(text, &group); err != nil {
			return err
		}
		lo, hi := metricsBounds.text[0], metricsBounds.text[1]
		if err := validateMetric("text.micro", group.Micro, lo, hi); err != nil {
			return err
		}
		if err := validateMetric("text.small", group.Small, lo, hi); err != nil {
			return err
		}
		if err := validateMetric("text.body", group.Body, lo, hi); err != nil {
			return err
		}
		if err := validateMetric("text.title", group.Title, lo, hi); err != nil {
			return err
		}
		m.TextMicro, m.TextSmall = group.Micro, group.Small
		m.TextBody, m.TextTitle = group.Body, group.Title
	}
	return nil
}

// pluginTheme is a theme a plugin contributes.
type pluginTheme struct {
	plugin  *plugin.Plugin
	contrib plugin.ThemeContrib
}

type appearanceAlias struct{ saved, current string }

func appearanceIndex(aliases []appearanceAlias, id string) int {
	return slices.IndexFunc(aliases, func(alias appearanceAlias) bool { return id == alias.saved || id == alias.current })
}

var colorThemeAliases = [...]appearanceAlias{
	{themeMatrix, "the-matrix.theme"}, {themeClaude, "claude.theme"}, {themeQoder, "qoder.theme"},
	{themeGrokBot, "grok-bot.theme"}, {themeCursor, "cursor.theme"}, {themePurpleGold, "purple-gold.theme"},
	{themePumpkin, "jack-o-lantern.theme"}, {themeWitchsBrew, "witchs-brew.theme"}, {themeBloodMoon, "blood-moon.theme"},
	{themeOneDarkPro, "one-dark-pro.theme"}, {themeAyu, "ayu.theme"},
}

func colorThemeIndex(id string) int {
	if short, bundled := strings.CutPrefix(id, "fiveleaf-themes."); bundled {
		if strings.Contains(short, ".") {
			return -1
		}
		id = short
	}
	return appearanceIndex(colorThemeAliases[:], id)
}

// inlineTheme is a theme written in settings.json, by its id.
type inlineTheme struct {
	id   string
	file *themeFile
}

// inlineThemes are the themes the settings write, by id.
func inlineThemes() []inlineTheme {
	if plugins == nil || plugins.prefs == nil {
		return nil
	}
	s := plugins.prefs()
	if s == nil || len(s.Themes) == 0 {
		return nil
	}
	inlineThemeCache.Lock()
	if sameThemes(inlineThemeCache.themes, s.Themes) {
		out := inlineThemeCache.list
		inlineThemeCache.Unlock()
		return out
	}
	inlineThemeCache.Unlock()
	ids := make([]string, 0, len(s.Themes))
	for id := range s.Themes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := make([]inlineTheme, 0, len(ids))
	for _, id := range ids {
		f, err := parseThemeFile(s.Themes[id])
		if err != nil {
			continue
		}
		out = append(out, inlineTheme{id: id, file: f})
	}
	inlineThemeCache.Lock()
	inlineThemeCache.themes = maps.Clone(s.Themes)
	inlineThemeCache.list = out
	inlineThemeCache.Unlock()
	return out
}

// inlineThemeCache is the themes the settings wrote as they were last
// read, so that a frame, or a picker's row, does not read them again.
var inlineThemeCache struct {
	sync.Mutex
	themes map[string]json.RawMessage
	list   []inlineTheme
}

// inlineThemeByID is the theme the settings write of id.
func inlineThemeByID(id string) (inlineTheme, bool) {
	if reservedThemeID(id) {
		return inlineTheme{}, false
	}
	for _, t := range inlineThemes() {
		if t.id == id {
			return t, true
		}
	}
	return inlineTheme{}, false
}

// sameThemes reports whether two sets of themes the settings write are
// the same, by id and by the bytes of each.
func sameThemes(a, b map[string]json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for id, v := range a {
		w, ok := b[id]
		if !ok || !bytes.Equal(v, w) {
			return false
		}
	}
	return true
}

// pluginThemes are the themes of the plugins that are on.
func pluginThemes() []pluginTheme {
	if plugins == nil || plugins.reg == nil {
		return nil
	}
	var ordered [len(colorThemeAliases)]pluginTheme
	var other []pluginTheme
	for _, p := range plugins.enabled() {
		for _, t := range p.Contributes.Themes {
			if _, builtin := builtinDarkness(t.ID, false); builtin {
				continue
			}
			theme := pluginTheme{p, t}
			if index := colorThemeIndex(t.ID); index >= 0 {
				if ordered[index].plugin == nil || t.ID == colorThemeAliases[index].current {
					ordered[index] = theme
				}
			} else {
				other = append(other, theme)
			}
		}
	}
	var out []pluginTheme
	for _, theme := range ordered {
		if theme.plugin != nil {
			out = append(out, theme)
		}
	}
	return append(out, other...)
}

// userTheme is a theme of a file in the user's themes folder.
type userTheme struct {
	id   string
	file string
}

// themeFiles keeps what the themes folder said as it was last read, so
// that a frame paints without reading every file again. A file is read
// anew once its size or its time on disk changes, as a save changes both.
var themeFiles = struct {
	sync.Mutex
	byPath   map[string]themeFileCache
	dir      string
	dirTime  time.Time
	dirTheme []userTheme
	gen      uint64
}{byPath: map[string]themeFileCache{}}

type themeFileCache struct {
	size  int64
	mtime time.Time
	file  *themeFile
	// err is why the file could not be read or parsed.
	err error
}

// userThemes are the themes of the user's themes folder, ~/.fiveleaf/themes,
// a file each: its id is the file's name without .json, and what it
// shows is its name in the file, or that.
func userThemes() []userTheme {
	dir := plugin.UserDirs().Themes
	if dir == "" {
		return nil
	}
	st, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	themeFiles.Lock()
	if themeFiles.dir == dir && themeFiles.dirTime.Equal(st.ModTime()) {
		out := themeFiles.dirTheme
		themeFiles.Unlock()
		return out
	}
	themeFiles.Unlock()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []userTheme
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !isThemeFileName(name) {
			continue
		}
		id := strings.TrimSuffix(name, filepath.Ext(name))
		out = append(out, userTheme{id: id, file: filepath.Join(dir, name)})
	}
	slices.SortFunc(out, func(a, b userTheme) int {
		if c := strings.Compare(a.id, b.id); c != 0 {
			return c
		}
		return strings.Compare(a.file, b.file)
	})
	themeFiles.Lock()
	themeFiles.dir, themeFiles.dirTime, themeFiles.dirTheme = dir, st.ModTime(), out
	themeFiles.Unlock()
	return out
}

// readUserThemeFile reads a theme file of the user's, from the cache
// while the file is as it was.
func readUserThemeFile(name string) (*themeFile, error) {
	st, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	themeFiles.Lock()
	if c, ok := themeFiles.byPath[name]; ok && c.size == st.Size() && c.mtime.Equal(st.ModTime()) {
		themeFiles.Unlock()
		return c.file, c.err
	}
	themeFiles.Unlock()
	data, err := os.ReadFile(name)
	var f *themeFile
	if err == nil {
		f, err = parseThemeFile(data)
	}
	themeFiles.Lock()
	themeFiles.byPath[name] = themeFileCache{size: st.Size(), mtime: st.ModTime(), file: f, err: err}
	themeFiles.Unlock()
	return f, err
}

// themeFileChanged forgets what a theme file said, so that the next read
// of it is from the disk: a save may keep its size and its time, and a
// folder may change without its own time changing.
func themeFileChanged() {
	themeFiles.Lock()
	themeFiles.byPath = map[string]themeFileCache{}
	themeFiles.dir, themeFiles.dirTheme = "", nil
	themeFiles.gen++
	themeFiles.Unlock()
}

// themeFileGen counts the changes of the themes of the user's.
func themeFileGen() uint64 {
	themeFiles.Lock()
	defer themeFiles.Unlock()
	return themeFiles.gen
}

// isThemeFileName reports whether name is a theme file: .json, or .jsonc
// for comments.
func isThemeFileName(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".json") || strings.EqualFold(filepath.Ext(name), ".jsonc")
}

// allThemes are FiveLeaf's themes, then those of the settings, the user's
// folder and the plugins'.
func allThemes() []themeChoice {
	out := []themeChoice{{"", func() string { return "FiveLeaf" }, locales.ThemeAboutFiveLeaf}}
	seen := map[string]bool{}
	var other []themeChoice
	for _, theme := range pluginThemes() {
		id := themeSettingID(theme.contrib.ID)
		if !validPluginTheme(theme) {
			continue
		}
		choice := themeChoice{id,
			func() string { return localizedThemeText(theme.contrib.Labels, theme.contrib.Label) },
			func() string {
				return localizedThemeText(theme.contrib.Descriptions, locales.ThemeAboutPlugin(theme.plugin.Title()))
			},
		}
		if id != theme.contrib.ID {
			out = append(out, choice)
		} else {
			other = append(other, choice)
		}
	}
	for _, t := range inlineThemes() {
		if seen[t.id] || reservedThemeID(t.id) {
			continue
		}
		if !validThemeFile(t.file, t.id, nil) {
			continue
		}
		id, label := t.id, idLabel(t.id, t.file.Name)
		out = append(out, themeChoice{id, label, locales.ThemeAboutSettings})
		seen[t.id] = true
	}
	for _, t := range userThemes() {
		if seen[t.id] || reservedThemeID(t.id) {
			continue
		}
		f, err := readUserThemeFile(t.file)
		if err != nil || !validThemeFile(f, t.id, nil) {
			continue
		}
		id, label := t.id, idLabel(t.id, f.Name)
		out = append(out, themeChoice{id, label, locales.ThemeAboutUser})
		seen[t.id] = true
	}
	for _, choice := range other {
		if !seen[choice.id] {
			out = append(out, choice)
		}
	}
	return out
}

func localizedThemeText(labels map[string]string, fallback string) string {
	tag := i18n.Locale()
	if label := labels[tag]; label != "" {
		return label
	}
	base, _, _ := strings.Cut(tag, "-")
	if label := labels[base]; label != "" {
		return label
	}
	if label := labels["en"]; label != "" {
		return label
	}
	return fallback
}

func validPluginTheme(theme pluginTheme) bool {
	file, err := readThemeFile(theme, false)
	if err != nil || !validThemeFile(file, theme.contrib.ID, nil) {
		return false
	}
	if theme.contrib.DarkPath != "" {
		file, err = readThemeFile(theme, true)
		return err == nil && validThemeFile(file, theme.contrib.ID, nil)
	}
	return true
}

// pluginPalette is the palette of a plugin's theme, in a window that is
// dark or not, nil when no plugin has the theme.
func pluginPalette(id string, dark bool) *palette {
	id = themeSettingID(id)
	files, generation := themeFileGen(), pluginGen()
	host := plugins
	key := paletteCacheKey{id: id, dark: dark}
	contributedPalettes.Lock()
	// A palette is made of the files and the plugins as they are now, so
	// the palettes of the last generations are dropped as they change.
	if contributedPalettes.files != files || contributedPalettes.plugins != generation || contributedPalettes.host != host {
		contributedPalettes.files, contributedPalettes.plugins, contributedPalettes.host = files, generation, host
		contributedPalettes.pals = map[paletteCacheKey]*palette{}
	}
	if p, ok := contributedPalettes.pals[key]; ok {
		contributedPalettes.Unlock()
		return p
	}
	contributedPalettes.Unlock()
	theme := contributedTheme(id)
	var p *palette
	if theme != nil {
		p = theme.palette(dark)
	}
	contributedPalettes.Lock()
	if contributedPalettes.files == files && contributedPalettes.plugins == generation && contributedPalettes.host == host {
		if p != nil || reservedThemeID(id) {
			contributedPalettes.pals[key] = p
		}
		if p != nil && theme.plugin != nil && theme.plugin.contrib.Dark {
			contributedPalettes.pals[paletteCacheKey{id: id, dark: !dark}] = p
		}
	}
	contributedPalettes.Unlock()
	return p
}

// contributedPalettes keeps the palettes of the themes of the user's and
// the plugins', so that a frame paints with the same one and the fade to
// another can reach it: a palette made anew each frame would look like a
// new theme each time.
var contributedPalettes = struct {
	sync.Mutex
	files, plugins uint64
	host           *pluginHost
	pals           map[paletteCacheKey]*palette
}{pals: map[paletteCacheKey]*palette{}}

type paletteCacheKey struct {
	id   string
	dark bool
}

// pluginGen counts the changes of the plugins, which their themes are
// made anew for.
func pluginGen() uint64 {
	if plugins == nil {
		return 0
	}
	plugins.mu.Lock()
	defer plugins.mu.Unlock()
	return plugins.gen
}

// contributedTheme finds a theme the settings, the user or a plugin
// contributes: FiveLeaf's own have their palettes already made.
func contributedTheme(id string) *contributedPalette {
	id = themeSettingID(id)
	if t, ok := inlineThemeByID(id); ok {
		return &contributedPalette{inline: &t}
	}
	if t, ok := userThemeByID(id); ok {
		return &contributedPalette{user: &t}
	}
	for _, t := range pluginThemes() {
		if themeSettingID(t.contrib.ID) == id {
			t := t
			return &contributedPalette{plugin: &t}
		}
	}
	return nil
}

// contributedPalette is a theme a file says: the settings', the user's
// or a plugin's.
type contributedPalette struct {
	inline *inlineTheme
	user   *userTheme
	plugin *pluginTheme
}

// palette is the theme in a window that is dark or not.
func (s *contributedPalette) palette(dark bool) *palette {
	switch {
	case s.inline != nil:
		return paletteOfThemeFile(s.inline.file, s.inline.id, themeDarkness(s.inline.id, dark, nil), dark, nil)
	case s.user != nil:
		return loadThemeFile(s.user.file, dark)
	case s.plugin != nil:
		return loadPluginTheme(*s.plugin, dark)
	}
	return nil
}

// userThemeByID is the user's theme of id, if it has one.
func userThemeByID(id string) (userTheme, bool) {
	if reservedThemeID(id) {
		return userTheme{}, false
	}
	for _, t := range userThemes() {
		if t.id == id {
			return t, true
		}
	}
	return userTheme{}, false
}

// reservedThemeID reports whether id names one of FiveLeaf's own themes, or
// the window's own, which another may not take.
func reservedThemeID(id string) bool {
	id = themeSettingID(id)
	return id == "" || id == "dark" || id == "light" || colorThemeIndex(id) >= 0
}

func themeSettingID(id string) string {
	switch id {
	case "", "fiveleaf", "mayoi", "fiveleaf-themes.fiveleaf":
		return ""
	}
	if strings.Contains(id, ".") {
		if index := colorThemeIndex(id); index >= 0 {
			return colorThemeAliases[index].saved
		}
	}
	return id
}

// idLabel is a theme's name in the picker: its file's name, or the name
// in it.
func idLabel(id, name string) func() string {
	if name == "" {
		name = id
	}
	return func() string { return name }
}

func readThemeFile(t pluginTheme, dark bool) (*themeFile, error) {
	name := t.contrib.Path
	if dark && t.contrib.DarkPath != "" {
		name = t.contrib.DarkPath
	}
	if !fs.ValidPath(name) || name == "." {
		return nil, fmt.Errorf("theme path %q is not a file in the plugin's folder", t.contrib.Path)
	}
	key := pluginThemeFileKey{t.plugin, name}
	generation := themeFileGen()
	pluginThemeFiles.Lock()
	if pluginThemeFiles.generation != generation || pluginThemeFiles.files == nil {
		pluginThemeFiles.generation = generation
		pluginThemeFiles.files = make(map[pluginThemeFileKey]themeFileCache)
	}
	if cached, ok := pluginThemeFiles.files[key]; ok {
		pluginThemeFiles.Unlock()
		return cached.file, cached.err
	}
	pluginThemeFiles.Unlock()
	file, err := readPluginThemeFile(t.plugin, name)
	pluginThemeFiles.Lock()
	if pluginThemeFiles.generation == generation && err == nil {
		pluginThemeFiles.files[key] = themeFileCache{file: file, err: err}
	}
	pluginThemeFiles.Unlock()
	return file, err
}

type pluginThemeFileKey struct {
	plugin *plugin.Plugin
	name   string
}

var pluginThemeFiles struct {
	sync.Mutex
	generation uint64
	files      map[pluginThemeFileKey]themeFileCache
}

func readPluginThemeFile(plugin *plugin.Plugin, name string) (*themeFile, error) {
	// A plugin may be bundled, where its files come from an fs.FS rather
	// than a folder on disk.
	if plugin.Dir == "" {
		data, err := fs.ReadFile(plugin.FS(), name)
		if err != nil {
			return nil, err
		}
		return parseThemeFile(data)
	}
	root, err := os.OpenRoot(plugin.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := root.ReadFile(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	return parseThemeFile(data)
}

// parseThemeFile reads a theme file, FiveLeaf's or VS Code's, with comments
// as VS Code's themes have them.
func parseThemeFile(data []byte) (*themeFile, error) {
	var f themeFile
	if err := plugin.DecodeColorTheme(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// themeDarkness tells whether a theme is dark in a window that is dark or
// not: as its file says, or as the theme it is based on says.
func themeDarkness(id string, window bool, seen []string) bool {
	id = themeSettingID(id)
	if slices.Contains(seen, id) {
		return window
	}
	seen = append(slices.Clone(seen), id)
	if dark, ok := builtinDarkness(id, window); ok {
		return dark
	}
	t := contributedTheme(id)
	if t == nil {
		return window
	}
	var f *themeFile
	var err error
	switch {
	case t.inline != nil:
		f = t.inline.file
	case t.user != nil:
		f, err = readUserThemeFile(t.user.file)
	case t.plugin != nil:
		if t.plugin.contrib.Dark {
			return true
		}
		if t.plugin.contrib.DarkPath != "" {
			return window
		}
		f, err = readThemeFile(*t.plugin, window)
	}
	if err != nil {
		return window
	}
	switch {
	case f.Dark != nil:
		return *f.Dark
	case f.Type != "":
		return f.Type != "light" && f.Type != "hcLight"
	}
	if f.Base == "" {
		return window
	}
	return themeDarkness(f.Base, window, seen)
}

// loadPluginTheme makes the palette of a plugin's theme.
func loadPluginTheme(t pluginTheme, window bool) *palette {
	if t.contrib.DarkPath != "" && !validPluginTheme(t) {
		return nil
	}
	f, err := readThemeFile(t, window)
	if err != nil {
		return nil
	}
	dark := themeDarkness(t.contrib.ID, window, nil)
	return paletteOfThemeFile(f, t.contrib.ID, dark, window, nil)
}

// loadThemeFile makes the palette of a theme file of the user's; a file
// that does not read is FiveLeaf's own palette.
func loadThemeFile(name string, window bool) *palette {
	f, err := readUserThemeFile(name)
	if err != nil {
		return nil
	}
	id := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	return paletteOfThemeFile(f, id, themeDarkness(id, window, nil), window, nil)
}

// paletteOfThemeFile makes the palette a theme file says: its base
// palette, with its colors, syntax, terminal colors and metrics changed.
func paletteOfThemeFile(f *themeFile, id string, dark, window bool, seen []string) *palette {
	if !validThemeFile(f, id, seen) {
		return nil
	}
	base := builtinPalette("", dark)
	if _, builtin := builtinDarkness(f.Base, dark); !builtin {
		base = paletteFrom(f.Base, dark, append(seen, id))
		if base == nil {
			return nil
		}
	}
	return f.applyTo(*base, dark)
}

func (f *themeFile) applyTo(p palette, dark bool) *palette {
	p.dark = dark
	if f.Mono != nil {
		p.mono = *f.Mono
	}
	if f.Sentence != nil {
		p.sentence = *f.Sentence
	}
	if f.Card != nil {
		p.card = *f.Card
	}
	if f.Grain != nil {
		p.grain = *f.Grain
	}
	if f.Mascot != nil {
		p.mascotName = *f.Mascot
	}
	if f.ActivityLabel != nil {
		p.activityLabel = *f.ActivityLabel
	}
	if f.AccentDialogs != nil {
		p.accentDialogs = *f.AccentDialogs
	}
	if f.WelcomeRain != nil {
		p.welcomeRain = *f.WelcomeRain
	}
	if f.Metrics != nil {
		if err := f.Metrics.apply(&p.metrics); err != nil {
			return nil
		}
	}
	_, chrome := parseThemeColor(f.Colors["chrome"])
	_, sidebar := parseThemeColor(f.Colors["sideBar.background"])
	if chrome || sidebar {
		p.titlebarExplicit = false
	}
	for k, v := range f.Colors {
		c, ok := parseThemeColor(v)
		if !ok {
			continue
		}
		if field := themeField(&p, k); field != nil {
			*field = c
			if k == "titlebar" {
				p.titlebarExplicit = true
			}
		} else if fields := vscodeColors[k]; fields != nil {
			for _, name := range fields {
				if field := themeField(&p, name); field != nil {
					*field = c
					if name == "titlebar" {
						p.titlebarExplicit = true
					}
				}
			}
		}
	}
	for k, v := range f.Syntax {
		if kind, ok := syntaxKinds[strings.ToLower(k)]; ok {
			if c, ok := parseThemeColor(v); ok {
				p.syntax[kind] = c
				p.syntaxExplicit[kind] = true
			}
		}
	}
	for name, italic := range f.SyntaxItalic {
		if kind, ok := syntaxKinds[strings.ToLower(name)]; ok {
			p.syntaxItalic[kind], p.syntaxItalicExplicit[kind] = italic, true
		}
	}
	for _, tc := range f.TokenColors {
		c, colorOK := parseThemeColor(tc.Settings.Foreground)
		if !colorOK && tc.Settings.FontStyle == nil {
			continue
		}
		for _, scope := range scopesOf(tc.Scope) {
			if kind, ok := scopeKind(scope); ok {
				if colorOK {
					p.syntax[kind], p.syntaxExplicit[kind] = c, true
				}
				if style := tc.Settings.FontStyle; style != nil {
					p.syntaxItalic[kind] = slices.Contains(strings.Fields(*style), "italic")
					p.syntaxItalicExplicit[kind] = true
				}
			}
		}
	}
	vscodeAnsi(&p, f.Colors)
	for i, v := range f.Ansi {
		if c, ok := parseThemeColor(v); ok && i < len(p.ansi) {
			p.ansi[i] = c
		}
	}
	if fg, ok := f.Colors["editor.foreground"]; ok {
		if c, ok := parseThemeColor(fg); ok {
			p.syntax[highlight.Plain] = c
		}
	}
	if f.Derived == nil || *f.Derived {
		if _, explicit := parseThemeColor(f.Colors["fillHover"]); !explicit {
			p.fillHover = p.fill.Mix(white, 0.1)
		}
		if _, explicit := parseThemeColor(f.Colors["fillPressed"]); !explicit {
			p.fillPressed = p.fill.Mix(black, 0.15)
		}
		return p.derive()
	}
	return p.keep()
}

// paletteFrom returns the palette of a theme a theme file is based on,
// with the themes it is based on already seen, so that a cycle of bases
// ends rather than reads for ever.
func paletteFrom(id string, dark bool, seen []string) *palette {
	id = themeSettingID(id)
	if slices.Contains(seen, id) {
		return nil
	}
	if t := contributedTheme(id); t != nil {
		dark = themeDarkness(id, dark, seen)
		switch {
		case t.inline != nil:
			return paletteOfThemeFile(t.inline.file, id, dark, dark, seen)
		case t.user != nil:
			f, err := readUserThemeFile(t.user.file)
			if err != nil {
				return nil
			}
			return paletteOfThemeFile(f, id, dark, dark, seen)
		case t.plugin != nil:
			f, err := readThemeFile(*t.plugin, dark)
			if err != nil {
				return nil
			}
			return paletteOfThemeFile(f, id, dark, dark, seen)
		}
	}
	return builtinPalette(id, dark)
}

// validThemeFile reports whether a theme file can make a palette: it
// parses, its base is one FiveLeaf has or a theme's, and no theme is based
// on itself, directly or through another.
func validThemeFile(f *themeFile, id string, seen []string) bool {
	if f.Card != nil && !finiteIn(*f.Card, 0, 40) || f.Grain != nil && !finiteIn(*f.Grain, 0, 1) {
		return false
	}
	switch f.Base {
	case "", "dark", "light":
		return true
	}
	if f.Base == id || slices.Contains(seen, f.Base) {
		return false
	}
	base := contributedTheme(f.Base)
	if base == nil {
		return builtinPalette(f.Base, false) != nil
	}
	var next *themeFile
	var err error
	switch {
	case base.inline != nil:
		next = base.inline.file
	case base.user != nil:
		next, err = readUserThemeFile(base.user.file)
	case base.plugin != nil:
		next, err = readThemeFile(*base.plugin, false)
	}
	if err != nil {
		return false
	}
	return validThemeFile(next, f.Base, append(slices.Clone(seen), id))
}

// apply sets the metrics a theme file names, over those of the palette.
// A number that is not finite or is out of range is an error, and leaves
// the palette as it was: one theme file may not break the window's
// layout for every theme.
func (m *metricsFile) apply(to *metrics) error {
	if err := m.validate(); err != nil {
		return err
	}
	for _, f := range []struct {
		dst *float32
		src *float32
	}{
		{&to.gap, m.Gap}, {&to.pad, m.Pad}, {&to.inset, m.Inset},
		{&to.radius, m.Radius}, {&to.radiusField, m.RadiusField}, {&to.radiusCard, m.RadiusCard},
		{&to.radiusFull, m.RadiusFull},
		{&to.textMicro, m.TextMicro}, {&to.textSmall, m.TextSmall},
		{&to.textBody, m.TextBody}, {&to.textTitle, m.TextTitle},
		{&to.row, m.Row}, {&to.control, m.Control},
	} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	for _, f := range []struct {
		dst *int
		src *int
	}{
		{&to.weightMedium, m.WeightMedium}, {&to.weightBold, m.WeightBold},
	} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if m.TextSize != nil {
		to.textBody, to.textTitle = *m.TextSize, *m.TextSize
	}
	return nil
}

// parseThemeColor reads #rgb, #rrggbb or #rrggbbaa.
func parseThemeColor(s string) (ui.Color, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return ui.Color{}, false
	}
	h := s[1:]
	switch len(h) {
	case 3, 6, 8:
		for i := 0; i < len(h); i++ {
			c := h[i]
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
				return ui.Color{}, false
			}
		}
		return ui.Hex(s), true
	}
	return ui.Color{}, false
}

// themeField is the color of a palette a theme file names, as "editor",
// nil for a name it does not have.
func themeField(p *palette, name string) *ui.Color {
	switch name {
	case "editor":
		return &p.editor
	case "chrome":
		return &p.chrome
	case "titlebar":
		return &p.titlebar
	case "panel":
		return &p.panel
	case "border":
		return &p.border
	case "hover":
		return &p.hover
	case "active":
		return &p.active
	case "text":
		return &p.text
	case "muted":
		return &p.muted
	case "faint":
		return &p.faint
	case "accent":
		return &p.accent
	case "accentDim":
		return &p.accentDim
	case "fill":
		return &p.fill
	case "fillHover":
		return &p.fillHover
	case "fillPressed":
		return &p.fillPressed
	case "fillText":
		return &p.fillText
	case "dangerFill":
		return &p.dangerFill
	case "dangerText":
		return &p.dangerText
	case "surface":
		return &p.surface
	case "surfaceHover":
		return &p.surfaceHover
	case "surfacePressed":
		return &p.surfacePressed
	case "scrollbar":
		return &p.scrollbar
	case "inverse":
		return &p.inverse
	case "inverseText":
		return &p.inverseText
	case "line":
		return &p.line
	case "selection":
		return &p.selection
	case "gutter":
		return &p.gutter
	case "gutterNow":
		return &p.gutterNow
	case "status":
		return &p.status
	case "statusFg":
		return &p.statusFg
	case "modified":
		return &p.modified
	case "danger":
		return &p.danger
	case "match":
		return &p.match
	case "matchNow":
		return &p.matchNow
	case "matchRing":
		return &p.matchRing
	case "guide":
		return &p.guide
	case "guideNow":
		return &p.guideNow
	case "bracket":
		return &p.bracket
	case "word":
		return &p.word
	case "shadow":
		return &p.shadow
	case "added":
		return &p.added
	case "changed":
		return &p.changed
	case "removed":
		return &p.removed
	case "strip":
		return &p.strip
	case "rule":
		return &p.rule
	case "gilt":
		return &p.gilt
	}
	return nil
}

// vscodeColors are the colors of a VS Code theme FiveLeaf takes, and the
// colors of its palette each sets.
var vscodeColors = map[string][]string{
	"editor.background":                       {"editor"},
	"titleBar.activeBackground":               {"titlebar"},
	"sideBar.background":                      {"chrome", "strip"},
	"editorGroupHeader.tabsBackground":        {"strip"},
	"panel.background":                        {"panel"},
	"editorGroup.border":                      {"border"},
	"panel.border":                            {"border"},
	"sideBar.border":                          {"border"},
	"list.hoverBackground":                    {"hover"},
	"list.activeSelectionBackground":          {"active"},
	"list.inactiveSelectionBackground":        {"active"},
	"foreground":                              {"text"},
	"editor.foreground":                       {"text"},
	"descriptionForeground":                   {"muted"},
	"disabledForeground":                      {"faint"},
	"focusBorder":                             {"accent"},
	"textLink.foreground":                     {"accent"},
	"button.background":                       {"fill"},
	"button.foreground":                       {"fillText"},
	"input.background":                        {"surface"},
	"dropdown.background":                     {"surface"},
	"scrollbarSlider.background":              {"scrollbar"},
	"editorWidget.background":                 {"inverse"},
	"editorWidget.foreground":                 {"inverseText"},
	"editor.lineHighlightBackground":          {"line"},
	"editor.selectionBackground":              {"selection"},
	"editorLineNumber.foreground":             {"gutter"},
	"editorLineNumber.activeForeground":       {"gutterNow"},
	"statusBar.background":                    {"status"},
	"statusBar.foreground":                    {"statusFg"},
	"editorWarning.foreground":                {"modified"},
	"errorForeground":                         {"danger"},
	"editorError.foreground":                  {"danger"},
	"editor.findMatchHighlightBackground":     {"match"},
	"editor.findMatchBackground":              {"matchNow"},
	"editor.findMatchBorder":                  {"matchRing"},
	"editorIndentGuide.background":            {"guide"},
	"editorIndentGuide.background1":           {"guide"},
	"editorIndentGuide.activeBackground":      {"guideNow"},
	"editorIndentGuide.activeBackground1":     {"guideNow"},
	"editorBracketMatch.border":               {"bracket"},
	"editor.wordHighlightBackground":          {"word"},
	"widget.shadow":                           {"shadow"},
	"editorGutter.addedBackground":            {"added"},
	"editorGutter.modifiedBackground":         {"changed"},
	"editorGutter.deletedBackground":          {"removed"},
	"gitDecoration.addedResourceForeground":   {"added"},
	"gitDecoration.deletedResourceForeground": {"removed"},
}

// syntaxKinds are the kinds of tokens a theme file's syntax names.
var syntaxKinds = map[string]highlight.Kind{
	"plain": highlight.Plain, "keyword": highlight.Keyword, "type": highlight.Type, "function": highlight.Function,
	"string": highlight.String, "number": highlight.Number, "comment": highlight.Comment, "operator": highlight.Operator,
	"punctuation": highlight.Punctuation, "constant": highlight.Constant, "builtin": highlight.Builtin, "tag": highlight.Tag,
	"attribute": highlight.Attribute, "variable": highlight.Variable, "heading": highlight.Heading,
	"deleted": highlight.Deleted, "inserted": highlight.Inserted,
	"parameter": highlight.Parameter, "binding": highlight.Binding, "property": highlight.Property,
	"interpolation": highlight.Interpolation, "bracket": highlight.Bracket, "object": highlight.Object,
	"builtintype": highlight.BuiltinType, "builtinfunction": highlight.BuiltinFunction, "assignment": highlight.Assignment,
}

// scopesOf reads a token color's scope: a string, with scopes separated by
// commas, or a list of them.
func scopesOf(raw json.RawMessage) []string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.Split(s, ",")
	}
	var list []string
	json.Unmarshal(raw, &list)
	return list
}

// scopeKind is the kind of token of a TextMate scope, by its start, the
// most particular first.
func scopeKind(scope string) (highlight.Kind, bool) {
	scope = strings.TrimSpace(scope)
	if strings.Contains(scope, " ") {
		return 0, false // a scope within another: too particular
	}
	for _, m := range []struct {
		prefix string
		kind   highlight.Kind
	}{
		{"punctuation.definition.template-expression", highlight.Interpolation},
		{"punctuation.section", highlight.Bracket},
		{"variable.parameter", highlight.Parameter},
		{"variable.other.constant.property", highlight.Property},
		{"variable.other.property", highlight.Property},
		{"variable.other.object", highlight.Object},
		{"variable.other.constant", highlight.Binding},
		{"meta.property.object", highlight.Property},
		{"comment", highlight.Comment},
		{"string", highlight.String},
		{"constant.numeric", highlight.Number},
		{"constant.language", highlight.Constant},
		{"constant.character", highlight.String},
		{"constant", highlight.Constant},
		{"keyword.operator", highlight.Operator},
		{"keyword", highlight.Keyword},
		{"storage.type", highlight.Type},
		{"storage", highlight.Keyword},
		{"entity.name.function", highlight.Function},
		{"support.function", highlight.Builtin},
		{"meta.function-call", highlight.Function},
		{"entity.name.type", highlight.Type},
		{"entity.name.class", highlight.Type},
		{"support.type", highlight.Type},
		{"support.class", highlight.Type},
		{"entity.name.tag", highlight.Tag},
		{"entity.other.attribute-name", highlight.Attribute},
		{"variable.language", highlight.Builtin},
		{"variable", highlight.Variable},
		{"support.constant", highlight.Builtin},
		{"punctuation", highlight.Punctuation},
		{"markup.heading", highlight.Heading},
		{"markup.deleted", highlight.Deleted},
		{"markup.inserted", highlight.Inserted},
	} {
		if scope == m.prefix || strings.HasPrefix(scope, m.prefix+".") {
			return m.kind, true
		}
	}
	return 0, false
}

// ansiNames are the terminal colors of a VS Code theme, in FiveLeaf's order.
var ansiNames = []string{
	"terminal.ansiBlack", "terminal.ansiRed", "terminal.ansiGreen", "terminal.ansiYellow",
	"terminal.ansiBlue", "terminal.ansiMagenta", "terminal.ansiCyan", "terminal.ansiWhite",
	"terminal.ansiBrightBlack", "terminal.ansiBrightRed", "terminal.ansiBrightGreen", "terminal.ansiBrightYellow",
	"terminal.ansiBrightBlue", "terminal.ansiBrightMagenta", "terminal.ansiBrightCyan", "terminal.ansiBrightWhite",
}

// vscodeAnsi fills a palette's terminal colors from a VS Code theme's.
func vscodeAnsi(p *palette, colors map[string]string) {
	for i, name := range ansiNames {
		if c, ok := parseThemeColor(colors[name]); ok {
			p.ansi[i] = c
		}
	}
}

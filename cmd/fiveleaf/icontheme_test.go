package main

import (
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
)

const testIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path fill="#ff0000" d="M0 0h8v16H0z"/><path fill="#00ff00" d="M8 0h8v16H8z"/></svg>`

func iconThemeFixture() fstest.MapFS {
	return fstest.MapFS{
		"theme.json": {Data: []byte(`{"file":"file","folder":"folder","folderOpen":"open",
"fileNames":{"PACKAGE.JSON":"named"},"fileExtensions":{".ts":"typescript","d.ts":"declaration","json":"json"},
"folderNames":{"src":"source"},"folderNamesOpen":{"src":"source-open"},
"icons":{"file":"icons/file.svg","folder":"icons/folder.svg","open":"icons/open.svg","named":"icons/named.svg",
"typescript":"icons/typescript.svg","declaration":"icons/declaration.svg","json":"icons/json.svg","source":"icons/source.svg","source-open":"icons/source-open.svg"}}`)},
		"icons/file.svg":        {Data: []byte(testIconSVG)},
		"icons/folder.svg":      {Data: []byte(testIconSVG)},
		"icons/open.svg":        {Data: []byte(testIconSVG)},
		"icons/named.svg":       {Data: []byte(testIconSVG)},
		"icons/typescript.svg":  {Data: []byte(testIconSVG)},
		"icons/declaration.svg": {Data: []byte(testIconSVG)},
		"icons/json.svg":        {Data: []byte(testIconSVG)},
		"icons/source.svg":      {Data: []byte(testIconSVG)},
		"icons/source-open.svg": {Data: []byte(testIconSVG)},
	}
}

func TestIconThemeMatching(t *testing.T) {
	pack, err := readIconTheme(iconThemeFixture(), "theme.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, id string }{
		{"/workspace/PACKAGE.JSON", "named"},
		{`C:\workspace\PACKAGE.JSON`, "named"},
		{"other.json", "json"},
		{"types/module.D.TS", "declaration"},
		{"main.ts", "typescript"},
		{"archive.unknown", "file"},
		{"README", "file"},
	} {
		if got := pack.icons[pack.fileID(tc.name)]; got != pack.icons[tc.id] {
			t.Errorf("%s did not select %s", tc.name, tc.id)
		}
	}
	for _, tc := range []struct {
		name string
		open bool
		id   string
	}{
		{"/workspace/SRC", false, "source"}, {"src", true, "source-open"},
		{"other", false, "folder"}, {"other", true, "open"},
	} {
		if got := pack.icons[pack.folderID(tc.name, tc.open)]; got != pack.icons[tc.id] {
			t.Errorf("folder %s open=%v did not select %s", tc.name, tc.open, tc.id)
		}
	}
	delete(pack.FolderNamesOpen, "src")
	if pack.icons[pack.folderID("src", true)] != pack.icons["source"] {
		t.Fatal("named open folder did not fall back to its closed icon")
	}
	files := iconThemeFixture()
	files["theme.json"].Data = []byte(strings.Replace(string(files["theme.json"].Data), `,"folderOpen":"open"`, "", 1))
	pack, err = readIconTheme(files, "theme.json")
	if err != nil || pack.icons[pack.folderID("other", true)] != pack.icons["folder"] {
		t.Fatalf("missing folderOpen: %v", err)
	}
}

func TestIconThemePrefixAndDotfileSelectors(t *testing.T) {
	files := iconThemeFixture()
	files["theme.json"].Data = []byte(strings.Replace(string(files["theme.json"].Data), `"fileNames":{`,
		`"fileNamePrefixes":{".ENV":"source",".env.loc":"declaration"},"dotfile":"named","fileNames":{".env.local":"json",`, 1))
	pack, err := readIconTheme(files, "theme.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		".ENV.local": "json", `C:\repo\.ENV.LOCAL.TS`: "declaration", ".env-other.TS": "source",
		"repo/.ENVRC": "source", ".ts": "named", ".other.ts": "typescript", "other": "file",
	} {
		if got := pack.fileID(name); got != want {
			t.Errorf("%s selected %s, want %s", name, got, want)
		}
	}
}

func TestIconThemeRejectsInvalidAssets(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(fstest.MapFS)
	}{
		{"missing SVG", func(f fstest.MapFS) { delete(f, "icons/file.svg") }},
		{"broken SVG", func(f fstest.MapFS) { f["icons/file.svg"].Data = []byte(`<svg><path></svg>`) }},
		{"incomplete SVG", func(f fstest.MapFS) { f["icons/file.svg"].Data = []byte(`<svg>`) }},
		{"non-SVG XML", func(f fstest.MapFS) { f["icons/file.svg"].Data = []byte(`<html/>`) }},
		{"zero dimensions", func(f fstest.MapFS) { f["icons/file.svg"].Data = []byte(`<svg width="0" height="0"/>`) }},
		{"oversized SVG", func(f fstest.MapFS) { f["icons/file.svg"].Data = []byte(strings.Repeat(" ", maxIconFileBytes+1)) }},
		{"undefined mapping", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `".ts":"typescript"`, `".ts":"missing"`, 1))
		}},
		{"duplicate normalized mapping", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `".ts":"typescript"`, `".ts":"typescript","TS":"json"`, 1))
		}},
		{"asset traversal", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `icons/file.svg`, `../file.svg`, 1))
		}},
		{"asset backslash", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `icons/file.svg`, `icons\\file.svg`, 1))
		}},
		{"absolute asset", func(f fstest.MapFS) {
			f["theme.json"].Data = []byte(strings.Replace(string(f["theme.json"].Data), `icons/file.svg`, `/file.svg`, 1))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := iconThemeFixture()
			tc.edit(files)
			pack, err := readIconTheme(files, "theme.json")
			if err == nil || pack != nil {
				t.Fatalf("invalid theme was partially loaded: pack=%v err=%v", pack, err)
			}
		})
	}
	for _, name := range []string{"../theme.json", "/theme.json", `C:\theme.json`, `icons\theme.json`} {
		if _, err := readIconTheme(iconThemeFixture(), name); err == nil {
			t.Errorf("accepted theme path %q", name)
		}
	}
	for _, field := range []string{
		`"dotfile":"absent"`, `"fileNamePrefixes":{".ENV":"open",".env":"file"}`,
		`"fileNamePrefixes":{".env":"absent"}`, `"tints":{"absent":{"role":"accent"}}`,
		`"tints":{"file":{"role":"missing"}}`, `"tints":{"file":{"dark":"#123"}}`,
		`"tints":{"file":{"dark":"#123","light":"invalid"}}`, `"tints":{"file":{"role":"text","dark":"#123"}}`,
	} {
		files := iconThemeFixture()
		files["theme.json"].Data = []byte("{" + field + "," + string(files["theme.json"].Data[1:]))
		if pack, err := readIconTheme(files, "theme.json"); err == nil || pack != nil {
			t.Errorf("accepted invalid icon theme field %s", field)
		}
	}
}

func TestIconThemePluginRootContainment(t *testing.T) {
	dir := t.TempDir()
	for name, file := range iconThemeFixture() {
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, file.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := &plugin.Plugin{Dir: dir}
	if _, err := readPluginIconTheme(p, "theme.json"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.svg")
	if err := os.WriteFile(outside, []byte(testIconSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(dir, "icons", "file.svg")
	if err := os.Remove(asset); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, asset); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if pack, err := readPluginIconTheme(p, "theme.json"); err == nil || pack != nil {
		t.Fatalf("read an asset outside the plugin root: %v", err)
	}
	outsideTheme := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(outsideTheme, iconThemeFixture()["theme.json"].Data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideTheme, filepath.Join(dir, "linked.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := readPluginIconTheme(p, "linked.json"); err == nil {
		t.Fatal("read a theme JSON outside the plugin root")
	}
}

func iconThemePlugin(t *testing.T) (*harness, *pluginHost) {
	t.Helper()
	files := map[string]string{
		"plugin.json": `{"name":"icon-test","contributes":{"iconThemes":[{"id":"icon-test.bright","label":"Bright icons","path":"theme.json"}]}}`,
	}
	for name, file := range iconThemeFixture() {
		files[name] = string(file.Data)
	}
	hs := newHarness(t, true)
	h := withPlugins(hs, map[string]map[string]string{"icon-test": files})
	return hs, h
}

func TestIconThemePluginLifecycle(t *testing.T) {
	hs, h := iconThemePlugin(t)
	a := hs.a
	a.settings.IconTheme = "icon-test.bright"
	first, _ := a.fileIcon("unknown")
	if first == iconFile || !coloredThemeIcon(first) {
		t.Fatal("plugin theme was not loaded")
	}
	if same, _ := a.fileIcon("unknown"); same != first {
		t.Fatal("unchanged generation reparsed the SVG")
	}
	w := &prefsWindow{m: &fiveleaf{s: a.settings}}
	w.setPluginSetting("icon-test", "enabled", false, true)
	if got, _ := a.fileIcon("unknown"); got != iconFile || a.settings.IconTheme != "icon-test.bright" {
		t.Fatal("disabling the plugin did not fall back while retaining its selection")
	}
	if slices.ContainsFunc(allIconThemes(), func(c iconThemeChoice) bool { return c.id == "icon-test.bright" }) {
		t.Fatal("disabled plugin remains selectable")
	}
	w.setPluginSetting("icon-test", "enabled", true, true)
	if got, _ := a.fileIcon("unknown"); got == iconFile || got == first {
		t.Fatal("re-enabling the plugin did not load its icons again")
	}
	dir := h.reg.Plugin("icon-test").Dir
	asset := filepath.Join(dir, "icons", "file.svg")
	if err := os.WriteFile(asset, []byte(strings.Replace(testIconSVG, "#ff0000", "#0000ff", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	h.reg.Reload("icon-test")
	replaced, tint := a.fileIcon("unknown")
	if replaced == iconFile || replaced == first {
		t.Fatal("valid replacement did not replace its cached SVG")
	}
	tt := ui.NewTester(func(c *ui.Context) { iconThemeView(c, a.pal, replaced, tint, 32) }, 32, 32)
	tt.Frame()
	if pixel := tt.Image().RGBAAt(4, 16); pixel.B < 240 || pixel.R > 15 || pixel.G > 15 {
		t.Fatalf("same-path SVG replacement did not change the rendered color: %v", pixel)
	}
	saved := filepath.Join(t.TempDir(), "saved-icon-test")
	if err := os.Rename(dir, saved); err != nil {
		t.Fatal(err)
	}
	h.reg.Load()
	if got, _ := a.fileIcon("unknown"); got != iconFile || a.settings.IconTheme != "icon-test.bright" {
		t.Fatal("removing the plugin did not fall back while retaining its selection")
	}
	if slices.ContainsFunc(allIconThemes(), func(c iconThemeChoice) bool { return c.id == "icon-test.bright" }) {
		t.Fatal("removed plugin remains selectable")
	}
	if err := os.Rename(saved, dir); err != nil {
		t.Fatal(err)
	}
	h.reg.Load()
	if got, _ := a.fileIcon("unknown"); got == iconFile || got == replaced || a.settings.IconTheme != "icon-test.bright" {
		t.Fatal("reinstalling the plugin did not restore its saved selection")
	}
	if err := os.WriteFile(asset, []byte("<svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.reg.Reload("icon-test")
	if got, _ := a.fileIcon("unknown"); got != iconFile {
		t.Fatal("broken replacement left its old icons active")
	}
	if slices.ContainsFunc(allIconThemes(), func(c iconThemeChoice) bool { return c.id == "icon-test.bright" }) {
		t.Fatal("broken theme remains selectable")
	}
}

func TestIconThemeFullColorRendering(t *testing.T) {
	hs, _ := iconThemePlugin(t)
	withAppearancePlugins(t)
	for _, p := range []*palette{hs.a.pal, matrixPal} {
		t.Run(fmt.Sprint("mono=", p.mono), func(t *testing.T) {
			icon, tint := fileIconForTheme(p, "icon-test.bright", "unknown")
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).Gap(0).Children(func() {
					iconThemeView(c, p, icon, tint, 32)
					ui.Box(c).Size(32, 32).Draw(func(pt *ui.Painter, r ui.Rect) { paintThemeIcon(pt, p, icon, tint, r) })
				})
			}, 64, 32)
			tt.Frame()
			img := tt.Image()
			for _, region := range []image.Rectangle{image.Rect(0, 0, 32, 32), image.Rect(32, 0, 64, 32)} {
				red, green := 0, 0
				for y := region.Min.Y; y < region.Max.Y; y++ {
					for x := region.Min.X; x < region.Max.X; x++ {
						pixel := img.RGBAAt(x, y)
						if pixel.R > 240 && pixel.G < 15 && pixel.B < 15 {
							red++
						}
						if pixel.G > 240 && pixel.R < 15 && pixel.B < 15 {
							green++
						}
					}
				}
				if red < 200 || green < 200 {
					t.Errorf("%v lost the SVG's own colors: red=%d green=%d", region, red, green)
				}
			}
		})
	}
}

func TestIconThemeAppearancePluginLifecycle(t *testing.T) {
	hs := newHarness(t, true)
	host := withPlugins(hs, nil)
	a := hs.a
	a.settings.IconTheme = "material"
	checkGeneric := func() {
		t.Helper()
		if choices := allIconThemes(); len(choices) != 1 || choices[0].id != "" || a.settings.IconTheme != "material" {
			t.Fatalf("unavailable packs: choices=%v, saved=%q", choices, a.settings.IconTheme)
		}
		for _, id := range []string{"", a.settings.IconTheme, "fiveleaf-icons.fiveleaf", "fiveleaf-icons.ayu"} {
			for _, name := range []string{"main.go", ".env.local", "go.sum", "README.md", "unknown"} {
				if icon, tint := fileIconForTheme(a.pal, id, name); icon != iconFile || tint != a.pal.muted || coloredThemeIcon(icon) {
					t.Fatalf("%q: %s did not use the generic file symbol", id, name)
				}
			}
			for open, want := range map[bool]*ui.SVG{false: iconFolder, true: iconFolderOpen} {
				if icon, tint := folderIconForTheme(a.pal, id, "src", open); icon != want || tint != a.pal.accent {
					t.Fatalf("%q: generic folder open=%t did not match", id, open)
				}
			}
		}
	}
	checkInstalled := func() *ui.SVG {
		t.Helper()
		pack := iconThemeByID("fiveleaf-icons.material")
		icon, _ := a.fileIcon("main.go")
		if pack == nil || pack.icons[pack.fileID("main.go")] != icon || !coloredThemeIcon(icon) || a.settings.IconTheme != "material" {
			t.Fatal("the saved Material choice did not select the installed pack")
		}
		return icon
	}
	checkGeneric()
	copyAppearancePlugin(t, host, "material-icons")
	host.reg.Load()
	first := checkInstalled()
	prefs := &prefsWindow{m: &fiveleaf{s: a.settings}}
	prefs.setPluginSetting("material-icons", "enabled", false, true)
	checkGeneric()
	if coloredThemeIcon(first) {
		t.Fatal("disabled pack left its icons in the renderer cache")
	}
	prefs.setPluginSetting("material-icons", "enabled", true, true)
	if checkInstalled() == first {
		t.Fatal("re-enabled pack reused its disabled snapshot")
	}
	dir := host.reg.Plugin("material-icons").Dir
	removed := filepath.Join(t.TempDir(), "material-icons")
	if err := os.Rename(dir, removed); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	checkGeneric()
	if err := os.Rename(removed, dir); err != nil {
		t.Fatal(err)
	}
	host.reg.Load()
	checkInstalled()
}

func TestIconThemeInstalledAssets(t *testing.T) {
	withAppearancePlugins(t)
	choices := []iconThemeChoice{
		{"fiveleaf-icons.fiveleaf", "FiveLeaf"}, {"fiveleaf-icons.catppuccin-latte", "Catppuccin Latte"},
		{"fiveleaf-icons.catppuccin-mocha", "Catppuccin Mocha"}, {"fiveleaf-icons.material", "Material Icon Theme"},
		{"fiveleaf-icons.seti", "Seti"}, {"fiveleaf-icons.ayu", "Ayu"},
	}
	if got := allIconThemes(); !slices.Equal(got, choices) {
		t.Fatalf("installed icon theme choices = %v, want %v", got, choices)
	}
	for index, choice := range choices {
		name, _, _ := strings.Cut(iconThemeAliases[index].current, ".")
		installed := plugins.reg.Plugin(name)
		if installed == nil || installed.Bundled || installed.HasCode() || len(installed.Contributes.IconThemes) != 1 {
			t.Fatalf("expected one file icon set from %s", name)
		}
		pack := iconThemeByID(choice.id)
		if pack == nil || iconThemeByID(installed.Contributes.IconThemes[0].ID) != pack || iconThemeByID(strings.TrimPrefix(choice.id, "fiveleaf-icons.")) != pack {
			t.Fatalf("%s or its legacy setting did not select the installed pack", choice.id)
		}
		for _, name := range []string{"main.go", "App.tsx", "package.json", "Dockerfile", "README.md", "image.png", "unknown"} {
			if icon, _ := fileIconForTheme(darkPal, choice.id, name); icon == nil || icon != pack.icons[pack.fileID(name)] {
				t.Errorf("%s: no icon for %s", choice.id, name)
			}
		}
		for _, open := range []bool{false, true} {
			if icon, _ := folderIconForTheme(darkPal, choice.id, "src", open); icon == nil || icon != pack.icons[pack.folderID("src", open)] {
				t.Errorf("%s: missing folder icon for open=%t", choice.id, open)
			}
		}
		if err := fs.WalkDir(installed.FS(), "assets", func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(name, ".svg") {
				return nil
			}
			data, err := fs.ReadFile(installed.FS(), name)
			if err != nil {
				return err
			}
			if _, err := parseIconSVG(data); err != nil {
				t.Errorf("%s: %v", name, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"", "fiveleaf-icons.fiveleaf", "uninstalled.theme"} {
		got, tint := fileIconForTheme(darkPal, id, "main.go")
		if got != iconThemeByID("fiveleaf-icons.fiveleaf").icons["go"] || tint != ui.Hex("#00add8") {
			t.Errorf("%q did not preserve FiveLeaf's file icons", id)
		}
	}
}

func TestIconThemeJSONRelativeAssets(t *testing.T) {
	files := iconThemeFixture()
	nested := fstest.MapFS{}
	for name, f := range files {
		nested["themes/"+name] = f
	}
	if _, err := readIconTheme(nested, "themes/theme.json"); err != nil {
		t.Fatal(err)
	}
}

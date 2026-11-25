package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIncompleteAppearanceUpdateKeepsPreviousInstall(t *testing.T) {
	const manifest = `{"name":"paper","version":"1.0.0","contributes":{
		"themes":[{"id":"paper.color","label":"Color","path":"light.jsonc","darkPath":"dark.jsonc"}],
		"iconThemes":[{"id":"paper.icons","label":"Icons","path":"packs/icons.json"}]}}`
	const palette = `{// VS Code theme
		"colors":{"editor.background":"#112233",}, "tokenColors":[],
	}`
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M0 0h8v8z"/></svg>`
	files := map[string]string{
		ManifestFile:          manifest,
		"light.jsonc":         palette,
		"dark.jsonc":          palette,
		"packs/icons.json":    `{"file":"file","folder":"file","icons":{"file":"file.svg","extra":"art/extra.SVG"}}`,
		"packs/file.svg":      svg,
		"packs/art/extra.SVG": svg,
	}
	for _, release := range []struct{ path, defect, content string }{
		{"packs/art/extra.SVG", "missing", ""}, {"packs/icons.json", "missing", ""},
		{"light.jsonc", "missing", ""}, {"dark.jsonc", "missing", ""},
		{"packs/file.svg", "empty", ""}, {"packs/file.svg", "broken XML", "<svg><path></svg>"},
		{"dark.jsonc", "broken JSON", `{"colors":`}, {"light.jsonc", "null", `null`},
	} {
		t.Run(release.path+"/"+release.defect, func(t *testing.T) {
			repo, dir := gitRepo(t, files), t.TempDir()
			if _, err := Install(t.Context(), dir, repo, ""); err != nil {
				t.Fatal(err)
			}
			recordFile := filepath.Join(dir, RecordsFile)
			before, err := os.ReadFile(recordFile)
			if err != nil {
				t.Fatal(err)
			}
			if release.defect == "missing" {
				sourceGit(t, repo, "rm", "--", release.path)
			} else {
				writePlugin(t, repo, "", map[string]string{release.path: release.content})
			}
			writePlugin(t, repo, "", map[string]string{ManifestFile: strings.Replace(manifest, `"version":"1.0.0"`, `"version":"2.0.0"`, 1)})
			sourceGit(t, repo, "commit", "-qam", "Release with an invalid asset")
			if changed, err := Update(t.Context(), dir, "paper"); changed || err == nil || !strings.Contains(filepath.ToSlash(err.Error()), release.path) {
				t.Errorf("update %s: changed=%t, err=%v", release.path, changed, err)
			}
			if after, err := os.ReadFile(recordFile); err != nil || !bytes.Equal(before, after) {
				t.Errorf("records changed from %s to %s, err=%v", before, after, err)
			}
			for name, want := range files {
				if got, err := os.ReadFile(filepath.Join(dir, "paper", name)); err != nil || string(got) != want {
					t.Errorf("installed %s = %q, err=%v; want original contents", name, got, err)
				}
			}
			fresh := t.TempDir()
			if _, err := Install(t.Context(), fresh, repo, ""); err == nil {
				t.Error("installed the incomplete release")
			}
			if entries, err := os.ReadDir(fresh); err != nil || len(entries) != 0 {
				t.Errorf("failed install left %v, err=%v", entries, err)
			}
		})
	}
}

func TestAppearanceIconAssetPathsAndLimits(t *testing.T) {
	const pack = "packs/theme.json"
	const asset = "art/file.SVG"
	const drawing = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"/>`
	svg := drawing + strings.Repeat(" ", maxIconAssetBytes-len(drawing))
	dir := t.TempDir()
	writePlugin(t, dir, "packs", map[string]string{
		asset:                svg,
		"small.svg":          drawing,
		"large.svg":          svg + " ",
		"directory.svg/file": "",
	})
	manifest := &Manifest{Contributes: Contributes{IconThemes: []IconThemeContrib{{Path: pack}}}}
	iconMap := func(ref string, count int) string {
		icons := make(map[string]string, count)
		for index := range count {
			icons[strconv.Itoa(index)] = ref
		}
		data, err := json.Marshal(icons)
		if err != nil {
			t.Fatal(err)
		}
		return `{"file":"0","folder":"0","icons":` + string(data) + `}`
	}
	check := func(body string) error {
		writePlugin(t, dir, "", map[string]string{pack: body})
		return checkAppearanceAssets(dir, manifest)
	}
	for _, ref := range []string{"../file.svg", "art/../file.svg", "./art/file.SVG", "art//file.SVG", "/file.svg", `art\file.svg`, "C:/file.svg", "https://example.com/file.svg", "file.png", "file\x00.svg"} {
		if err := check(iconMap(ref, 1)); err == nil || !strings.Contains(err.Error(), "invalid SVG path") {
			t.Errorf("SVG path %q: %v", ref, err)
		}
	}
	if err := check(iconMap(asset, 32)); err != nil {
		t.Fatalf("relative SVGs at the size limits: %v", err)
	}
	for name, body := range map[string]string{
		"non-JSON icon pack": `{// comment` + "\n" + `"icons":{"file":"art/file.SVG"}}`,
		"oversized icon map": strings.Repeat(" ", maxIconAssetBytes) + iconMap(asset, 1),
		"too many icons":     iconMap("small.svg", maxIconsPerPack+1),
		"oversized SVG":      iconMap("large.svg", 1),
		"oversized total":    iconMap(asset, 33),
		"SVG directory":      iconMap("directory.svg", 1),
	} {
		if err := check(body); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
}

func TestAppearanceAssetsStayWithinPlugin(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writePlugin(t, outside, "", map[string]string{
		"color.jsonc": "{}", "icons.json": `{"icons":{"file":"file.svg"}}`, "file.svg": "<svg/>",
	})
	writePlugin(t, dir, "", map[string]string{"color.jsonc": "{}", "icons.json": `{"icons":{"file":"linked/file.svg"}}`})
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for label, contributions := range map[string]Contributes{
		"palette":      {Themes: []ThemeContrib{{Path: "linked/color.jsonc"}}},
		"dark palette": {Themes: []ThemeContrib{{Path: "color.jsonc", DarkPath: "linked/color.jsonc"}}},
		"icon pack":    {IconThemes: []IconThemeContrib{{Path: "linked/icons.json"}}},
		"SVG":          {IconThemes: []IconThemeContrib{{Path: "icons.json"}}},
	} {
		if err := checkAppearanceAssets(dir, &Manifest{Contributes: contributions}); err == nil {
			t.Errorf("read %s outside plugin root", label)
		}
	}
}

func TestAppearanceAssetsRequireReadableFile(t *testing.T) {
	dir := t.TempDir()
	const name = "color.jsonc"
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, []byte("{}"), 0); err != nil {
		t.Fatal(err)
	}
	if probe, err := os.Open(file); err == nil {
		probe.Close()
		t.Skip("current user can read mode-000 files")
	}
	manifest := &Manifest{Contributes: Contributes{Themes: []ThemeContrib{{Path: name}}}}
	if err := checkAppearanceAssets(dir, manifest); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable palette: %v", err)
	}
}

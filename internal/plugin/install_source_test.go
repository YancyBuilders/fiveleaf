package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sourceGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := git(t.Context(), repo, append([]string{"-c", "user.name=FiveLeaf", "-c", "user.email=tests@localhost"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMarketplaceSubdirectoryPlugins(t *testing.T) {
	const repository = "https://github.com/YancyBuilders/fiveleaf-plugins.git"
	const index = "https://raw.githubusercontent.com/YancyBuilders/fiveleaf-plugins/main/index.json"
	if IndexURL != index {
		t.Fatalf("marketplace index = %q, want %q", IndexURL, index)
	}
	catalog, err := os.ReadFile("../../extensions/index.json")
	if err != nil {
		t.Fatal(err)
	}
	served := catalog
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(served)
	}))
	t.Cleanup(server.Close)
	IndexURL = server.URL
	t.Cleanup(func() { IndexURL = index })
	entries, err := FetchIndex(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	appearance := map[string]string{}
	for _, name := range strings.Fields("one-dark-pro ayu the-matrix claude qoder grok-bot cursor purple-gold jack-o-lantern witchs-brew blood-moon") {
		appearance[name] = "themes"
	}
	for _, name := range strings.Fields("fiveleaf-file-icons material-icons catppuccin-latte-icons catppuccin-mocha-icons seti-icons ayu-icons") {
		appearance[name] = "icon-themes"
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if seen[entry.Name] || entry.Name == "fiveleaf-themes" || entry.Name == "fiveleaf-icons" {
			t.Fatalf("duplicate or retired catalog entry %q", entry.Name)
		}
		seen[entry.Name] = true
		dir := filepath.Join("../../extensions", entry.Name)
		manifest, err := ReadManifest(dir)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Name != entry.Name || entry.DisplayName == "" || manifest.DisplayName != entry.DisplayName || len(entry.Tags) == 0 || entry.Category == "" {
			t.Fatalf("catalog does not match %s: %+v", dir, entry)
		}
		files := []string{"README.md", "LICENSE"}
		if manifest.HasCode() {
			files = append(files, manifest.Main)
		}
		for _, name := range files {
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				t.Fatalf("%s has an invalid %s: %v", dir, name, err)
			}
		}
		tag, official := appearance[entry.Name]
		if !official {
			continue
		}
		colors, icons := len(manifest.Contributes.Themes), len(manifest.Contributes.IconThemes)
		if entry.Category != tag || colors+icons != 1 || (tag == "themes") != (colors == 1) {
			t.Fatalf("%s contributes %d color themes and %d icon themes", entry.Name, colors, icons)
		}
		if err := checkAppearanceAssets(dir, manifest); err != nil {
			t.Fatalf("%s: %v", entry.Name, err)
		}
	}
	manifests, err := filepath.Glob("../../extensions/*/plugin.json")
	if err != nil || len(manifests) != len(entries) {
		t.Fatalf("found %d plugin directories, catalog lists %d: %v", len(manifests), len(entries), err)
	}
	for name := range appearance {
		if !seen[name] {
			t.Fatalf("missing %s from catalog", name)
		}
	}
	repo := gitRepo(t, map[string]string{"index.json": string(catalog)})
	for _, name := range []string{"one-dark-pro", "material-icons"} {
		for _, entry := range entries {
			if entry.Name == name {
				release, _ := entry.Newest()
				if err := os.CopyFS(filepath.Join(repo, filepath.FromSlash(release.Path)), os.DirFS(filepath.Join("../../extensions", name))); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
	}
	sourceGit(t, repo, "add", ".")
	sourceGit(t, repo, "commit", "-qm", "Add official plugins")
	fixtureCommit := sourceGit(t, repo, "rev-parse", "HEAD")
	for name, commit := range map[string]string{"one-dark-pro": fixtureCommit, "material-icons": fixtureCommit} {
		for index := range entries {
			if entries[index].Name != name {
				continue
			}
			for version := range entries[index].Versions {
				entries[index].Versions[version].Commit = commit
			}
		}
	}
	catalog, _ = json.Marshal(entries)
	served = catalog
	localURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(repo)}).String()
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "url."+localURL+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", repository)
	t.Setenv("GIT_CONFIG_KEY_1", "url."+localURL+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_1", strings.TrimSuffix(repository, ".git"))
	registry, _, dirs := newRegistry(t)
	run := func(args ...string) string {
		t.Helper()
		var output, problems strings.Builder
		if code := RunCLI(args, dirs, &output, &problems); code != 0 {
			t.Fatalf("plugin %v: %s", args, problems.String())
		}
		return output.String()
	}
	for title, name := range map[string]string{"One Dark Pro": "one-dark-pro", "Material Icon Theme": "material-icons"} {
		if result := run("search", title); !strings.HasPrefix(result, name+" ") || !strings.Contains(result, title) || strings.Count(result, "\n") != 1 {
			t.Fatalf("search %q = %s", title, result)
		}
	}
	run("install", "one-dark-pro")
	run("install", "YancyBuilders/fiveleaf-plugins#plugins/icon-themes/material-icons")
	run("install", repository+"#plugins/icon-themes/material-icons")
	registry.Load()
	first := sourceGit(t, repo, "rev-parse", "HEAD")
	want := Records{
		"one-dark-pro":   {URL: repository + "#plugins/themes/one-dark-pro", Ref: "one-dark-pro@v1.0.0", Commit: first, Version: "1.0.0", TrackRelease: true},
		"material-icons": {URL: repository + "#plugins/icon-themes/material-icons", Commit: first},
	}
	if got := ReadRecords(dirs.Plugins); !reflect.DeepEqual(got, want) {
		t.Fatalf("records = %+v, want %+v", got, want)
	}
	listing := run("list")
	for name, src := range want {
		if origin := registry.Origin(name); !origin.FromGit || origin.Source != src || !strings.Contains(listing, src.URL) {
			t.Fatalf("%s origin = %+v, list = %s", name, origin, listing)
		}
		for _, excluded := range []string{".git", "index.json", "material-icons", "one-dark-pro"} {
			if _, err := os.Lstat(filepath.Join(dirs.Plugins, name, excluded)); !os.IsNotExist(err) {
				t.Fatalf("%s installed unexpected %s: %v", name, excluded, err)
			}
		}
	}
	themes, icons := registry.Plugin("one-dark-pro"), registry.Plugin("material-icons")
	if themes == nil || len(themes.Contributes.Themes) != 1 || icons == nil || len(icons.Contributes.IconThemes) != 1 {
		t.Fatal("installed plugins must each contribute their named theme")
	}
	asset := themes.Contributes.Themes[0].Path
	writePlugin(t, repo, "plugins/themes/one-dark-pro", map[string]string{asset + ".new": `{"name":"Updated"}`})
	writePlugin(t, repo, "plugins/themes/one-dark-pro", map[string]string{ManifestFile: `{"name":"one-dark-pro","version":"1.1.0","category":"themes","contributes":{"themes":[{"id":"one-dark-pro.theme","label":"One Dark Pro","path":"themes/onedarkpro.json"}]}}`})
	sourceGit(t, repo, "add", ".")
	sourceGit(t, repo, "commit", "-qm", "Update a color theme")
	second := sourceGit(t, repo, "rev-parse", "HEAD")
	var raw []map[string]any
	if err := json.Unmarshal(catalog, &raw); err != nil {
		t.Fatal(err)
	}
	for _, entry := range raw {
		if entry["name"] == "one-dark-pro" {
			entry["version"] = "1.1.0"
			entry["versions"] = []any{map[string]any{"version": "1.1.0", "path": "plugins/themes/one-dark-pro", "commit": second}}
		}
	}
	served, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	run("update", "one-dark-pro")
	if got, err := os.ReadFile(filepath.Join(dirs.Plugins, "one-dark-pro", asset+".new")); string(got) != `{"name":"Updated"}` || err != nil {
		t.Fatalf("updated theme = %q, %v", got, err)
	}
	records := ReadRecords(dirs.Plugins)
	if records["material-icons"] != want["material-icons"] || records["one-dark-pro"].Commit != second || records["one-dark-pro"].Version != "1.1.0" {
		t.Fatalf("updated records = %+v", records)
	}
	other := t.TempDir()
	installed, _, err := SyncInstall(t.Context(), other, records)
	if err != nil || len(installed) != 2 || !reflect.DeepEqual(ReadRecords(other), records) {
		t.Fatalf("sync = %v, %+v, %v", installed, ReadRecords(other), err)
	}
}

func TestMarketplaceUpdatesRespectInstallSource(t *testing.T) {
	const subdir, nextSubdir = "plugins/paper", "plugins/new-paper"
	const firstManifest = `{"name":"paper","version":"1.0.0"}`
	repo := gitRepo(t, map[string]string{subdir + "/plugin.json": firstManifest})
	first := sourceGit(t, repo, "rev-parse", "HEAD")
	sourceGit(t, repo, "branch", "stable")
	entry := IndexEntry{Name: "paper", Category: "themes", Repository: repo, Version: "1.0.0",
		Versions: []VersionEntry{{Version: "1.0.0", Path: subdir, Commit: first}}}
	tracked, pinned, branch, legacy, reinstalled := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	for _, install := range []struct{ dir, ref string }{{tracked, ""}, {pinned, "1.0.0"}, {branch, "stable"}} {
		if _, err := entry.Install(t.Context(), install.dir, install.ref); err != nil {
			t.Errorf("install @%s: %v", install.ref, err)
		}
	}
	for _, dir := range []string{legacy, reinstalled} {
		writePlugin(t, dir, "paper", map[string]string{ManifestFile: firstManifest})
		if err := WriteRecords(dir, Records{"paper": {URL: repo + "#" + subdir, Version: "1.0.0", Commit: first}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := entry.Install(t.Context(), reinstalled, ""); err != nil || !ReadRecords(reinstalled)[entry.Name].TrackRelease {
		t.Fatalf("reinstalling a legacy record by name did not resume following the index: %v", err)
	}
	sourceGit(t, repo, "mv", subdir, nextSubdir)
	writePlugin(t, repo, nextSubdir, map[string]string{ManifestFile: `{"name":"paper","version":"1.1.0"}`})
	sourceGit(t, repo, "commit", "-qam", "Publish a release")
	trunk := sourceGit(t, repo, "rev-parse", "HEAD")
	sourceGit(t, repo, "checkout", "-q", "stable")
	writePlugin(t, repo, subdir, map[string]string{ManifestFile: `{"name":"paper","version":"1.0.1"}`})
	sourceGit(t, repo, "commit", "-qam", "Update stable branch")
	stable := sourceGit(t, repo, "rev-parse", "HEAD")
	entry.Version = "2.0.0"
	entry.Versions = append([]VersionEntry{
		{Version: "2.0.0", Path: nextSubdir, FiveLeaf: ">=99.0.0"},
		{Version: "1.1.0", Path: nextSubdir, Commit: trunk},
	}, entry.Versions...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]IndexEntry{entry})
	}))
	t.Cleanup(server.Close)
	oldURL, oldVersion := IndexURL, IndexVersion
	t.Cleanup(func() { IndexURL, IndexVersion = oldURL, oldVersion })
	IndexURL, IndexVersion = server.URL, "1.0.0"
	for _, test := range []struct {
		name, dir, commit, version, subdir string
		outdated                           bool
	}{
		{"tracked", tracked, trunk, "1.1.0", nextSubdir, true},
		{"reinstalled", reinstalled, trunk, "1.1.0", nextSubdir, true},
		{"pinned", pinned, first, "1.0.0", subdir, false},
		{"branch", branch, stable, "1.0.1", subdir, false},
		{"legacy", legacy, first, "1.0.0", subdir, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dirs := Dirs{Plugins: test.dir}
			var output, problems strings.Builder
			if code := RunCLI([]string{"outdated"}, dirs, &output, &problems); code != 0 || strings.Contains(output.String(), "1.1.0") != test.outdated || strings.Contains(output.String(), "2.0.0") {
				t.Errorf("outdated = %d, %q, %q", code, &output, &problems)
			}
			output.Reset()
			problems.Reset()
			if code := RunCLI([]string{"update"}, dirs, &output, &problems); code != 0 {
				t.Errorf("update = %d, %q, %q", code, &output, &problems)
			}
			if src := ReadRecords(dirs.Plugins)[entry.Name]; src.Commit != test.commit || src.URL != repo+"#"+test.subdir {
				t.Errorf("updated source = %+v, want %s at commit %s", src, test.subdir, test.commit)
			}
			if manifest, err := ReadManifest(filepath.Join(dirs.Plugins, entry.Name)); err != nil || manifest.Version != test.version {
				t.Errorf("updated manifest = %+v, %v, want version %s", manifest, err, test.version)
			}
		})
	}
	previous := Source{URL: repo + "#" + subdir, Commit: first, Version: "1.0.0", TrackRelease: true}
	t.Run("other repository", func(t *testing.T) {
		foreign := entry
		foreign.Repository = t.TempDir()
		if _, ok := previous.NewerRelease(foreign); ok {
			t.Fatal("a release from another repository was offered as an update")
		}
	})
	t.Run("occupied path", func(t *testing.T) {
		dir := t.TempDir()
		writePlugin(t, dir, entry.Name, map[string]string{ManifestFile: firstManifest})
		records := Records{entry.Name: previous, "other": {URL: repo + "#" + nextSubdir}}
		if err := WriteRecords(dir, records); err != nil {
			t.Fatal(err)
		}
		if changed, err := Update(t.Context(), dir, entry.Name); changed || err == nil || !strings.Contains(err.Error(), `was installed as "other"`) {
			t.Fatalf("the update did not report the occupied source: %v, %v", changed, err)
		}
		if got := ReadRecords(dir); !reflect.DeepEqual(got, records) {
			t.Fatalf("records changed after a conflicting update: %+v", got)
		}
		if manifest, err := ReadManifest(filepath.Join(dir, entry.Name)); err != nil || manifest.Version != previous.Version {
			t.Fatalf("plugin changed after a conflicting update: %+v, %v", manifest, err)
		}
	})
}

func TestGitSubdirectorySelection(t *testing.T) {
	for _, spec := range []struct{ input, url, ref string }{
		{"owner/repo#packs/color", "https://github.com/owner/repo#packs/color", ""},
		{"owner/repo#packs/color@release/v1", "https://github.com/owner/repo#packs/color", "release/v1"},
		{"owner/repo@v1#packs/color", "https://github.com/owner/repo#packs/color", "v1"},
		{"git@github.com:owner/repo.git#packs/color@v1", "git@github.com:owner/repo.git#packs/color", "v1"},
		{"ssh://user@host/repo.git#icons", "ssh://user@host/repo.git#icons", ""},
		{"./repo#packs/color", "./repo#packs/color", ""},
	} {
		if url, ref, byName := SplitSpec(spec.input); url != spec.url || ref != spec.ref || byName {
			t.Errorf("%s parsed as %q, %q, byName=%t", spec.input, url, ref, byName)
		}
	}
	for _, path := range []string{"", ".", "..", "../colors", "theme/../colors", "theme//colors", "theme/./colors", "/colors", `theme\..\colors`, "C:/colors", ".git", ".GIT/objects", "a\x00b", ".. /colors"} {
		if _, _, err := splitRepository("repo#" + path); err == nil {
			t.Errorf("accepted subdirectory %q", path)
		}
	}
	repo := gitRepo(t, map[string]string{"packs/color/plugin.json": `{"name":"paper"}`})
	first := sourceGit(t, repo, "rev-parse", "HEAD")
	sourceGit(t, repo, "tag", "-a", "v1", "-m", "Release")
	sourceGit(t, repo, "branch", "release/v1")
	writePlugin(t, repo, "packs/color", map[string]string{ManifestFile: `{"name":"paper","version":"2.0.0"}`})
	sourceGit(t, repo, "commit", "-qam", "Advance main")
	t.Chdir(filepath.Dir(repo))
	for _, ref := range []string{"v1", "release/v1", first} {
		dir := t.TempDir()
		if _, err := Install(t.Context(), dir, "./"+filepath.Base(repo)+"#packs/color", ref); err != nil {
			t.Fatal(err)
		}
		src := ReadRecords(dir)["paper"]
		if !filepath.IsAbs(src.URL) || src.Ref != ref || src.Commit != first {
			t.Fatalf("relative source at %s = %+v", ref, src)
		}
		if latest, err := Latest(t.Context(), src); err != nil || latest != first {
			t.Errorf("latest at %s = %q, %v", ref, latest, err)
		}
	}
}

func TestGitInstallRejectsLinkedPaths(t *testing.T) {
	const manifest = `{"name":"paper"}`
	repo := gitRepo(t, map[string]string{
		"packs/colors/paper/plugin.json": manifest,
		"linked-asset/plugin.json":       manifest,
		"root-manifest":                  manifest,
	})
	outside := t.TempDir()
	writePlugin(t, outside, "", map[string]string{ManifestFile: manifest})
	for path, target := range map[string]string{
		"packs/alias":               "colors",
		"packs/outside":             outside,
		"manifest-link/plugin.json": "../root-manifest",
		"linked-asset/palettes":     outside,
		"linked-file/plugin.json":   filepath.Join(outside, ManifestFile),
	} {
		link := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	sourceGit(t, repo, "add", ".")
	sourceGit(t, repo, "commit", "-qm", "Add links")
	dir := t.TempDir()
	for _, path := range []string{"packs/alias/paper", "packs/outside", "manifest-link", "linked-asset", "linked-file", "missing", "root-manifest"} {
		if _, err := Install(t.Context(), dir, repo+"#"+path, ""); err == nil {
			t.Errorf("installed %q", path)
		}
	}
	if items, err := os.ReadDir(dir); err != nil || len(items) != 0 {
		t.Fatalf("failed installs left %v, %v", items, err)
	}
	if _, err := Install(t.Context(), dir, repo+"#packs/colors/paper", ""); err != nil {
		t.Fatalf("install beside an unrelated link: %v", err)
	}
	linkedDir := filepath.Join(t.TempDir(), "plugins")
	if err := os.Symlink(outside, linkedDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(t.Context(), linkedDir, repo+"#packs/colors/paper", ""); err == nil {
		t.Error("installed through a linked plugins directory")
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.symlinks")
	t.Setenv("GIT_CONFIG_VALUE_0", "false")
	if _, err := Install(t.Context(), t.TempDir(), repo+"#linked-asset", ""); err == nil {
		t.Error("accepted a tracked link checked out as a plain file")
	}
}

func TestGitInstallPreservesFailedSyncAndRecords(t *testing.T) {
	dir := t.TempDir()
	repo := gitRepo(t, map[string]string{"theme/plugin.json": `{"name":"paper"}`})
	if _, err := Install(t.Context(), dir, repo+"#theme", ""); err != nil {
		t.Fatal(err)
	}
	records := ReadRecords(dir)
	if _, removed, err := SyncInstall(t.Context(), dir, Records{"renamed": records["paper"]}); err == nil || len(removed) != 0 {
		t.Fatalf("invalid sync removed %v, err = %v", removed, err)
	}
	if _, err := ReadManifest(filepath.Join(dir, "paper")); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"null", "broken"} {
		writePlugin(t, dir, "", map[string]string{RecordsFile: body})
		if _, err := Install(t.Context(), dir, repo+"#theme", ""); err == nil {
			t.Errorf("replaced invalid records %q", body)
		}
		if data, err := os.ReadFile(filepath.Join(dir, RecordsFile)); err != nil || string(data) != body {
			t.Fatalf("records now %q, %v", data, err)
		}
	}
}

func TestGitInstallRollback(t *testing.T) {
	dir, transaction, staged := t.TempDir(), t.TempDir(), t.TempDir()
	writePlugin(t, dir, "paper", map[string]string{"version": "old"})
	writePlugin(t, staged, "", map[string]string{"version": "new"})
	if err := os.Mkdir(filepath.Join(dir, RecordsFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replacePlugin(dir, transaction, "paper", staged, true, Records{"paper": {URL: "example"}}); err == nil {
		t.Fatal("replacement succeeded without committing records")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "paper/version")); err != nil || string(data) != "old" {
		t.Fatalf("restored plugin = %q, %v", data, err)
	}
}

func TestGitInstallLegacyLocalRecord(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "plugins")
	repo := gitRepo(t, map[string]string{ManifestFile: `{"name":"paper"}`})
	writePlugin(t, dir, "paper", map[string]string{ManifestFile: `{"name":"paper"}`})
	relative, err := filepath.Rel(dir, repo)
	if err != nil {
		t.Fatal(err)
	}
	src := Source{URL: relative, Commit: sourceGit(t, repo, "rev-parse", "HEAD")}
	if err := WriteRecords(dir, Records{"paper": src}); err != nil {
		t.Fatal(err)
	}
	if latest, err := Latest(t.Context(), ReadRecords(dir)["paper"]); err != nil || latest != src.Commit {
		t.Fatalf("legacy latest = %q, %v", latest, err)
	}
	if changed, err := Update(t.Context(), dir, "paper"); err != nil || changed {
		t.Fatalf("legacy update = %v, %v", changed, err)
	}
}

func TestGitInstallChecksIndexName(t *testing.T) {
	repo := gitRepo(t, map[string]string{"material-icons/plugin.json": `{"name":"material-icons"}`})
	entry := IndexEntry{Name: "one-dark-pro", Category: "themes", Version: "1.0.0", Repository: repo + "#material-icons"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]IndexEntry{entry})
	}))
	t.Cleanup(server.Close)
	oldURL := IndexURL
	IndexURL = server.URL
	t.Cleanup(func() { IndexURL = oldURL })
	dirs := Dirs{Plugins: t.TempDir()}
	var output, problems strings.Builder
	const mismatch = `expected plugin "one-dark-pro"`
	if code := RunCLI([]string{"install", entry.Name}, dirs, &output, &problems); code == 0 || !strings.Contains(problems.String(), mismatch) {
		t.Fatalf("install %s = %d, %s", entry.Name, code, problems.String())
	}
	if _, err := entry.Install(t.Context(), dirs.Plugins, ""); err == nil || !strings.Contains(err.Error(), mismatch) {
		t.Fatalf("install index row: %v", err)
	}
	if _, err := InstallSpec(t.Context(), dirs.Plugins, entry.Name, []IndexEntry{entry}); err == nil || !strings.Contains(err.Error(), mismatch) {
		t.Fatalf("install cached index name: %v", err)
	}
	if entries, err := os.ReadDir(dirs.Plugins); err != nil || len(entries) != 0 {
		t.Fatalf("incorrect index entry installed %v, %v", entries, err)
	}
	if manifest, err := InstallSpec(t.Context(), dirs.Plugins, entry.Repository, nil); err != nil || manifest.Name != "material-icons" {
		t.Fatalf("install direct Git source: %+v, %v", manifest, err)
	}
}

func TestGitInstallPreservesPluginIdentity(t *testing.T) {
	dir := t.TempDir()
	repos := map[string]string{}
	for _, name := range []string{"colors", "icons"} {
		body, err := json.Marshal(Manifest{Name: name, Version: "1.0.0", Category: "themes"})
		if err != nil {
			t.Fatal(err)
		}
		repos[name] = gitRepo(t, map[string]string{ManifestFile: string(body)})
		if _, err := Install(t.Context(), dir, repos[name], ""); err != nil {
			t.Fatal(err)
		}
	}
	before := ReadRecords(dir)
	writePlugin(t, repos["colors"], "", map[string]string{ManifestFile: `{"name":"icons","version":"2.0.0"}`})
	sourceGit(t, repos["colors"], "commit", "-qam", "Rename colors")
	if _, err := Update(t.Context(), dir, "colors"); err == nil {
		t.Error("update accepted a renamed manifest")
	}
	if _, err := Install(t.Context(), dir, repos["colors"], ""); err == nil {
		t.Error("reinstall accepted a renamed manifest")
	}
	if got := ReadRecords(dir); !reflect.DeepEqual(got, before) {
		t.Errorf("records changed from %+v to %+v", before, got)
	}
	for name := range before {
		if manifest, err := ReadManifest(filepath.Join(dir, name)); err != nil || manifest.Version != "1.0.0" || manifest.Name != name {
			t.Errorf("%s changed: %+v, %v", name, manifest, err)
		}
	}
}

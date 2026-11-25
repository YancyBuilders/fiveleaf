package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "", files)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "one"}} {
		if _, err := git(context.Background(), dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestInstallUpdateRemove(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	oldIndex := IndexURL
	t.Cleanup(func() { IndexURL = oldIndex })
	IndexURL = server.URL
	_, _, dirs := newRegistry(t)
	repo := gitRepo(t, map[string]string{"plugin.json": `{"name": "hello", "version": "1.0.0"}`})
	m, err := Install(context.Background(), dirs.Plugins, repo, "")
	if err != nil || m.Name != "hello" {
		t.Fatalf("install: %v %v", m, err)
	}
	recs := ReadRecords(dirs.Plugins)
	if recs["hello"].URL != repo || len(recs["hello"].Commit) != 40 {
		t.Fatalf("records = %+v", recs)
	}
	if changed, err := Update(context.Background(), dirs.Plugins, "hello"); err != nil || changed {
		t.Fatalf("update with nothing new: %v %v", changed, err)
	}
	writePlugin(t, repo, "", map[string]string{"plugin.json": `{"name": "hello", "version": "1.1.0"}`})
	git(context.Background(), repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qam", "two")
	if latest, _ := Latest(context.Background(), recs["hello"]); latest == recs["hello"].Commit {
		t.Fatal("Latest did not see the new commit")
	}
	if changed, err := Update(context.Background(), dirs.Plugins, "hello"); err != nil || !changed {
		t.Fatalf("update: %v %v", changed, err)
	}
	var out, errs strings.Builder
	if code := RunCLI([]string{"update"}, dirs, &out, &errs); code != 0 {
		t.Fatalf("batch update: %d, %s", code, &errs)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("the Git update made %d marketplace requests", got)
	}
	if m, _ := ReadManifest(filepath.Join(dirs.Plugins, "hello")); m.Version != "1.1.0" {
		t.Fatalf("version after update = %s", m.Version)
	}
	// Another machine's records install what is missing and remove the rest.
	other := t.TempDir()
	installed, removed, err := SyncInstall(context.Background(), other, ReadRecords(dirs.Plugins))
	if err != nil || len(installed) != 1 || len(removed) != 0 {
		t.Fatalf("sync: %v %v %v", installed, removed, err)
	}
	_, removed, _ = SyncInstall(context.Background(), other, Records{})
	if len(removed) != 1 {
		t.Fatalf("sync removal: %v", removed)
	}
	if err := Remove(dirs.Plugins, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadRecords(dirs.Plugins)["hello"]; ok {
		t.Fatal("record kept")
	}
}

func TestFetchIndexNotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	old := IndexURL
	IndexURL = srv.URL
	defer func() { IndexURL = old }()
	entries, err := FetchIndex(context.Background(), nil)
	if err != nil || len(entries) != 0 {
		t.Fatalf("FetchIndex = %v, %v; want no entries and no error", entries, err)
	}
}

func TestSplitSpec(t *testing.T) {
	if url, ref, byName := SplitSpec("me/thing@v2"); url != "https://github.com/me/thing" || ref != "v2" || byName {
		t.Fatalf("SplitSpec = %s %s %v", url, ref, byName)
	}
	if url, ref, _ := SplitSpec("git@github.com:me/thing.git"); url != "git@github.com:me/thing.git" || ref != "" {
		t.Fatalf("SplitSpec ssh = %s %s", url, ref)
	}
	if _, _, byName := SplitSpec("prettier"); !byName {
		t.Fatal("a bare name is of the index")
	}
}

func TestCLIHelp(t *testing.T) {
	dirs := Dirs{Plugins: t.TempDir()}
	var out, errs strings.Builder
	if code := RunCLI([]string{"--help"}, dirs, &out, &errs); code != 0 {
		t.Fatalf("--help: %d %s", code, errs.String())
	}
	for _, want := range []string{"Usage: ff plugin ", "ff plugin install YancyBuilders/fiveleaf-plugins#material-icons"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q", want)
		}
	}
	if code := RunCLI([]string{"unknown"}, dirs, &out, &errs); code != 1 || errs.String() != "ff plugin: no command unknown: ff plugin --help lists them\n" {
		t.Errorf("unknown: %d %q", code, errs.String())
	}
}

func TestCLINewAndDev(t *testing.T) {
	r, _, dirs := newRegistry(t)
	work := filepath.Join(t.TempDir(), "greet")
	var out, errb strings.Builder
	if code := RunCLI([]string{"new", "greet", work}, dirs, &out, &errb); code != 0 {
		t.Fatalf("new: %s", errb.String())
	}
	if !strings.Contains(out.String(), "&& ff plugin dev\n") {
		t.Errorf("new instructions: %s", out.String())
	}
	if data, _ := os.ReadFile(filepath.Join(work, ManifestFile)); !strings.Contains(string(data), `"author":`) {
		t.Errorf("the scaffold's manifest has no author: %s", data)
	}
	if _, err := os.Stat(filepath.Join(work, "fiveleaf.d.ts")); err != nil {
		t.Fatalf("the scaffold's types: %v", err)
	}
	theme := filepath.Join(t.TempDir(), "midnight")
	if code := RunCLI([]string{"new", "midnight", theme, "--theme"}, dirs, &out, &errb); code != 0 {
		t.Fatalf("new --theme: %s", errb.String())
	}
	m, err := ReadManifest(theme)
	if err != nil || len(m.Contributes.Themes) != 1 || m.Contributes.Themes[0].ID != "midnight.theme" || m.Main != "" {
		t.Fatalf("theme manifest %+v, %v", m, err)
	}
	if data, err := os.ReadFile(filepath.Join(theme, "theme.json")); err != nil || !strings.Contains(string(data), `"base": "dark"`) {
		t.Fatalf("theme file %q, %v", data, err)
	}
	toolDir := filepath.Join(t.TempDir(), "my-tool")
	if code := RunCLI([]string{"new", "my-tool", toolDir, "--tool"}, dirs, &out, &errb); code != 0 {
		t.Fatalf("new --tool: %s", errb.String())
	}
	tool, err := ReadManifest(toolDir)
	if err != nil || tool.Contributes.Toolbox == nil || tool.Contributes.Toolbox.ID != "my-tool.tool" || tool.Main != "" {
		t.Fatalf("tool manifest %+v, %v", tool, err)
	}
	if data, err := os.ReadFile(filepath.Join(toolDir, "index.html")); err != nil || !strings.Contains(string(data), "app.js") {
		t.Fatalf("tool page %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(toolDir, "app.js")); err != nil || !strings.Contains(string(data), "Toolbox.Copy") {
		t.Fatalf("tool script %q, %v", data, err)
	}
	out.Reset()
	if code := RunCLI([]string{"dev", work}, dirs, &out, &errb); code != 0 {
		t.Fatalf("dev: %s", errb.String())
	}
	if !strings.Contains(out.String(), "ff plugin remove greet") {
		t.Errorf("unlink instructions: %s", out.String())
	}
	out.Reset()
	RunCLI([]string{"list"}, dirs, &out, &errb)
	if !strings.Contains(out.String(), "greet") || !strings.Contains(out.String(), "dev:") {
		t.Fatalf("list = %s", out.String())
	}
	r.Load()
	if _, err := r.Run("greet.hello", nil); err != nil {
		t.Fatalf("the scaffold's command: %v", err)
	}
	if code := RunCLI([]string{"remove", "greet"}, dirs, &out, &errb); code != 0 {
		t.Fatalf("remove: %s", errb.String())
	}
	if _, err := os.Stat(work); err != nil {
		t.Fatal("removing the link removed the folder")
	}
}

func TestCLIDebugAdapterScaffold(t *testing.T) {
	registry, _, dirs := newRegistry(t)
	root := t.TempDir()
	folder := filepath.Join(root, "sample debug")
	var output, problems strings.Builder
	if code := RunCLI([]string{"new", "sample-debug", "--debug-adapter", folder}, dirs, &output, &problems); code != 0 {
		t.Fatal(problems.String())
	}
	if code := RunCLI([]string{"dev", folder}, dirs, &output, &problems); code != 0 {
		t.Fatal(problems.String())
	}
	registry.Load()
	provider := registry.Plugin("sample-debug")
	if provider == nil || !registry.Origin("sample-debug").Linked || provider.Stats().Err != "" || provider.Category != "debug-adapters" || provider.HasCode() {
		t.Fatal("expected a linked declarative debug adapter")
	}
	if len(provider.Contributes.Languages) != 1 || len(provider.Contributes.DebugAdapters) != 1 {
		t.Fatalf("contributions = %+v", provider.Contributes)
	}
	language := provider.Contributes.Languages[0]
	adapter := provider.Contributes.DebugAdapters[0]
	if len(language.Extensions) != 1 || len(adapter.Languages) != 1 || adapter.Languages[0] != language.ID || adapter.ID != "sample-debug.debug" || adapter.AdapterID != "python" {
		t.Fatalf("language = %+v, adapter = %+v", language, adapter)
	}
	if adapter.Command != "${settings.pythonPath}" || string(provider.Contributes.Settings["pythonPath"].Default) != `"python3"` {
		t.Fatalf("adapter executable = %q, settings = %+v", adapter.Command, provider.Contributes.Settings)
	}
	if _, err := os.Stat(filepath.Join(folder, "example"+language.Extensions[0])); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{{"--theme", "--debug-adapter"}, {"--debug-adpater"}} {
		if code := RunCLI(append([]string{"new", "invalid", filepath.Join(root, "invalid")}, flags...), dirs, &output, &problems); code == 0 {
			t.Errorf("accepted %v", flags)
		}
	}
}

func TestDevelopmentPluginFolder(t *testing.T) {
	registry, _, dirs := newRegistry(t)
	dirs.Bundled = fstest.MapFS{"stock/plugin.json": {Data: []byte(`{"name":"stock"}`)}}
	source := t.TempDir()
	writePlugin(t, source, "", map[string]string{ManifestFile: `{"name":"stock"}`})
	if _, err := dirs.LinkDev(source); err == nil {
		t.Fatal("a development plugin shadowed a bundled plugin")
	}
	writePlugin(t, source, "", map[string]string{ManifestFile: `{"name":"paper","version":"1.0.0"}`})
	if _, err := dirs.LinkDev(source); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(dirs.Plugins, "paper")
	if _, err := dirs.LinkDev(linked); err != nil {
		t.Fatal(err)
	}
	registry.Load()
	if item := registry.Plugin("paper"); item == nil || item.Version != "1.0.0" || !registry.Origin("paper").Linked {
		t.Fatal("selecting the existing link lost the development plugin")
	}
	stop, err := registry.Watch()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	writePlugin(t, source, "", map[string]string{ManifestFile: `{"name":"paper","version":"1.12.0"}`})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if item := registry.Plugin("paper"); item != nil && item.Version == "1.12.0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("saving in the source folder did not reload the linked plugin")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := Remove(dirs.Plugins, "paper"); err != nil {
		t.Fatal(err)
	}
	if manifest, err := ReadManifest(source); err != nil || manifest.Version != "1.12.0" {
		t.Fatal("unlinking the development plugin damaged its source folder")
	}
}

func TestToolboxStockCanBeOverriddenAndRestored(t *testing.T) {
	registry, _, dirs := newRegistry(t)
	const name = "toolbox-json"
	stock := `{"name":"toolbox-json","version":"1.0.0","contributes":{"toolbox":{"id":"json","page":"index.html"}}}`
	market := `{"name":"toolbox-json","version":"1.1.0","contributes":{"toolbox":{"id":"toolbox-json.tool","page":"public/main.html"}}}`
	dirs.Bundled = fstest.MapFS{name + "/plugin.json": {Data: []byte(stock)}}
	registry.Dirs = dirs
	check := func(bundled bool, version string) {
		t.Helper()
		registry.Load()
		providers := registry.Enabled()
		if len(providers) != 1 || providers[0].Name != name || providers[0].Version != version || providers[0].Bundled != bundled {
			t.Fatalf("expected bundled=%v version=%s, got %+v", bundled, version, providers)
		}
	}
	check(true, "1.0.0")
	repo := gitRepo(t, map[string]string{"plugin.json": market, "public/main.html": "market version"})
	if _, err := Install(t.Context(), dirs.Plugins, repo, ""); err != nil {
		t.Fatal(err)
	}
	check(false, "1.1.0")
	registry.SetSafe(true)
	check(true, "1.0.0")
	registry.SetSafe(false)
	check(false, "1.1.0")
	if _, err := dirs.LinkDev(repo); err == nil {
		t.Fatal("overwrote an installed copy with a development link")
	}
	if err := Remove(dirs.Plugins, name); err != nil {
		t.Fatal(err)
	}
	check(true, "1.0.0")
	if _, err := dirs.LinkDev(repo); err != nil {
		t.Fatal(err)
	}
	check(false, "1.1.0")
	if !registry.Origin(name).Linked {
		t.Fatal("the development version is not linked")
	}
	if err := Remove(dirs.Plugins, name); err != nil {
		t.Fatal(err)
	}
	check(true, "1.0.0")
	broken := gitRepo(t, map[string]string{"plugin.json": market})
	if _, err := Install(t.Context(), dirs.Plugins, broken, ""); err == nil {
		t.Fatal("accepted a release missing its page")
	}
	check(true, "1.0.0")
}

func TestUserDirsCompatibility(t *testing.T) {
	t.Setenv("FIVELEAF_DATA_DIR", "")
	t.Setenv("MAYOI_DATA_DIR", t.TempDir())
	if got := UserDirs().Plugins; got != filepath.Join(os.Getenv("MAYOI_DATA_DIR"), "plugins") {
		t.Fatalf("legacy override = %s", got)
	}
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	if got := UserDirs().Plugins; got != filepath.Join(os.Getenv("FIVELEAF_DATA_DIR"), "plugins") {
		t.Fatalf("current override = %s", got)
	}
	home := t.TempDir()
	current, old := filepath.Join(home, ".fiveleaf"), filepath.Join(home, ".mayoi")
	if got := userPluginDir(home); got != current {
		t.Fatalf("fresh install = %s", got)
	}
	if err := os.Mkdir(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := userPluginDir(home); got != old {
		t.Fatalf("existing legacy directory = %s", got)
	}
	if err := os.Mkdir(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := userPluginDir(home); got != current {
		t.Fatalf("current directory takes priority = %s", got)
	}
}

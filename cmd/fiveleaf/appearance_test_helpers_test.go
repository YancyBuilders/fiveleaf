package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fiveleaf/internal/plugin"
	"fiveleaf/internal/settings"
)

func copyAppearancePlugin(t testing.TB, host *pluginHost, name string) {
	t.Helper()
	if err := os.CopyFS(filepath.Join(host.reg.Dirs.Plugins, name), os.DirFS(filepath.Join("..", "..", "extensions", name))); err != nil {
		t.Fatal(err)
	}
}

func withAppearancePlugins(t testing.TB) {
	t.Helper()
	t.Setenv("FIVELEAF_DATA_DIR", t.TempDir())
	host := plugins
	if host == nil {
		prefs := settings.Default()
		host = newPluginHost(func() []*app { return nil }, func(run func()) { run() }, func() *settings.Settings { return prefs }, func() {})
		host.reg = plugin.New(plugin.UserDirs(), host)
		host.reg.Version = "dev"
		host.syncPrefs()
		plugins = host
		t.Cleanup(func() {
			host.reg.Close()
			plugins = nil
			themeFileChanged()
		})
	} else {
		previousDir := host.reg.Dirs.Plugins
		if previousDir == "" {
			host.reg.Dirs.Plugins = plugin.UserDirs().Plugins
		}
		t.Cleanup(func() {
			host.reg.Dirs.Plugins = previousDir
			host.reg.Load()
		})
	}

	if err := os.MkdirAll(host.reg.Dirs.Plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "extensions", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []plugin.IndexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !slices.Contains(entry.Tags, "themes") && !slices.Contains(entry.Tags, "icon-themes") {
			continue
		}
		name := entry.Name
		names = append(names, name)
		if host.reg.Plugin(name) != nil {
			continue
		}
		dir := filepath.Join(host.reg.Dirs.Plugins, name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Error(err)
			}
		})
		copyAppearancePlugin(t, host, name)
	}
	host.reg.Load()
	for _, name := range names {
		pack := host.reg.Plugin(name)
		if pack == nil {
			t.Fatalf("installed appearance plugin %q is missing", name)
		}
		if state := pack.Stats(); pack.Bundled || !pack.DefaultOn() || state.Disabled || state.Err != "" {
			t.Fatalf("loading %s: bundled=%v defaultOn=%v state=%+v", name, pack.Bundled, pack.DefaultOn(), state)
		}
	}
	if paletteFor("", false) != lightPal || paletteFor("", true) != darkPal {
		t.Fatal("installing appearance plugins replaced the built-in FiveLeaf palettes")
	}

	for _, binding := range []struct {
		target **palette
		id     string
		dark   bool
	}{
		{&matrixPal, themeMatrix, true}, {&oneDarkProPal, themeOneDarkPro, true},
		{&ayuLightPal, themeAyu, false}, {&ayuDarkPal, themeAyu, true},
		{&claudeLightPal, themeClaude, false}, {&claudeDarkPal, themeClaude, true},
		{&qoderLightPal, themeQoder, false}, {&qoderDarkPal, themeQoder, true},
		{&grokBotLightPal, themeGrokBot, false}, {&grokBotDarkPal, themeGrokBot, true},
		{&cursorLightPal, themeCursor, false}, {&cursorDarkPal, themeCursor, true},
		{&purpleGoldPal, themePurpleGold, true}, {&pumpkinPal, themePumpkin, true},
		{&witchsBrewPal, themeWitchsBrew, true}, {&bloodMoonPal, themeBloodMoon, true},
	} {
		loaded := paletteFor(binding.id, binding.dark)
		if !samePalette(loaded, pluginPalette(binding.id, binding.dark)) {
			t.Fatalf("installed theme %q did not provide its palette (dark=%v)", binding.id, binding.dark)
		}
		previous := *binding.target
		*binding.target = loaded
		t.Cleanup(func() { *binding.target = previous })
	}
}

func samePalette(got, want *palette) bool {
	return got != nil && want != nil && *got == *want
}

package main

import (
	"os"
	"path/filepath"
)

func installCatalogFixtures(hs *harness, host *pluginHost, patterns ...string) {
	hs.t.Helper()
	for _, pattern := range patterns {
		dirs, err := filepath.Glob(filepath.Join("..", "..", "extensions", pattern))
		if err != nil || len(dirs) == 0 {
			hs.t.Fatalf("catalog fixture %q: %v", pattern, err)
		}
		for _, dir := range dirs {
			dest := filepath.Join(host.reg.Dirs.Plugins, filepath.Base(dir))
			if err := os.CopyFS(dest, os.DirFS(dir)); err != nil {
				hs.t.Fatal(err)
			}
		}
	}
	host.reg.Load()
}

func useKeymapFixtures(hs *harness) {
	hs.t.Helper()
	host := withPlugins(hs, nil)
	installCatalogFixtures(hs, host, "keymap-*")
}

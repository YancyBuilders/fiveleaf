package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/plugin"
)

func withMarketplaceLanguageServers(hs *harness) *pluginHost {
	h := withPlugins(hs, nil)
	installCatalogFixtures(hs, h, "lsp-*")
	h.reg.Load()
	hs.a.settings.Prefs.Plugins = map[string]json.RawMessage{}
	for _, p := range h.reg.Plugins() {
		if len(p.Contributes.LanguageServers) > 0 {
			hs.a.settings.Prefs.Plugins[p.Name] = json.RawMessage(`{"enabled":true}`)
		}
	}
	h.syncPrefs()
	return h
}

func TestMarketplaceLanguageServerRouting(t *testing.T) {
	hs := newHarness(t, false)
	h := withMarketplaceLanguageServers(hs)
	for _, tc := range []struct{ path, language, server string }{
		{"app.ts", "typescript", "typescript"},
		{"module.mts", "typescript", "typescript"},
		{"app.tsx", "typescriptreact", "typescript"},
		{"app.jsx", "javascriptreact", "typescript"},
		{"app.js", "javascript", "typescript"},
		{"module.cjs", "javascript", "typescript"},
		{"index.html", "html", "html"},
		{"main.css", "css", "css"},
		{"main.scss", "scss", "css"},
		{"main.less", "less", "css"},
		{"App.vue", "vue", "vue"},
		{"App.svelte", "svelte", "svelte"},
		{"main.go", "go", "go"},
		{"go.mod", "gomod", "go"},
		{"go.work", "gowork", "go"},
		{"main.py", "python", "python"},
		{"main.rs", "rust", "rust"},
		{"main.c", "c", "clangd"},
		{"main.cpp", "cpp", "clangd"},
		{"types.hpp", "cpp", "clangd"},
		{"Main.java", "java", "java"},
		{"Main.cs", "csharp", "csharp"},
		{"index.php", "php", "php"},
		{"app.rb", "ruby", "ruby"},
		{"Gemfile", "ruby", "ruby"},
		{"init.lua", "lua", "lua"},
		{"build.sh", "shellscript", "bash"},
		{".bashrc", "shellscript", "bash"},
		{"package.json", "json", "json"},
		{"tsconfig.json", "jsonc", "json"},
		{"settings.jsonc", "jsonc", "json"},
		{"pipeline.yaml", "yaml", "yaml"},
		{"Cargo.toml", "toml", "toml"},
		{"Cargo.lock", "toml", "toml"},
		{"Dockerfile", "dockerfile", "docker"},
		{"Containerfile", "dockerfile", "docker"},
		{"image.dockerfile", "dockerfile", "docker"},
		{"main.tf", "terraform", "terraform"},
		{"dev.tfvars", "terraform-vars", "terraform"},
		{"README.md", "markdown", "markdown"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			language := languageOf(&document{path: hs.path(tc.path)})
			if language != tc.language {
				t.Fatalf("language = %q; want %q", language, tc.language)
			}
			p, server := serverContrib(language)
			want := "lsp-" + tc.server
			if p == nil || server == nil || p.Name != want || server.ID != want+".server" {
				t.Fatalf("server for %q: %v, %+v", language, p, server)
			}
			if p.Started() {
				t.Fatal("a declarative language plugin started a JS runtime")
			}
		})
	}
	hs.a.settings.Prefs.Plugins["lsp-typescript"] = json.RawMessage(`{"enabled":false}`)
	h.syncPrefs()
	if _, server := serverContrib("typescriptreact"); server != nil {
		t.Fatal("disabled language server still selected")
	}
}

func TestMarketplaceTypeScriptReactSelectionScript(t *testing.T) {
	hs := newHarness(t, false)
	data, err := os.ReadFile("../../extensions/lsp-typescript/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := plugin.ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the real language metadata without launching an installed server.
	m.Enabled, m.Contributes.LanguageServers = nil, nil
	data, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	h := withPlugins(hs, map[string]map[string]string{m.Name: {"plugin.json": string(data)}})
	col := hs.a.fileColumn()
	hs.a.addUntitled(col, 1, `import { storage } from "fiveleaf";
export default function answer(): number { const value: number = 42; storage.set("answer", value); return value; }`)
	hs.a.activate(col, len(col.tabs)-1, true)
	d := hs.a.activeEditor()
	d.path = hs.path("selection.tsx")
	hs.settle()
	if languageOf(d.document) != "typescriptreact" || commentFor(d.document) != "//" {
		t.Fatalf("TSX language/comments: %s / %s", languageOf(d.document), commentFor(d.document))
	}
	hs.a.runSelectionScript()
	var result struct{ Answer int }
	hs.until("TypeScript selection script result", func() bool {
		data, err := os.ReadFile(filepath.Join(h.reg.Dirs.Storage, "selection", "global.json"))
		return err == nil && json.Unmarshal(data, &result) == nil && result.Answer == 42 || hs.a.fails > 0
	})
	if hs.a.fails > 0 {
		t.Fatalf("TypeScript selection script: %v", hs.a.errors)
	}
}

func TestMarketplaceLanguageServerDidOpen(t *testing.T) {
	t.Setenv("FIVELEAF_FAKE_LSP", "1")
	hs := newHarness(t, false)
	h := enableMarketplacePlugins(hs, "lsp-typescript")
	t.Cleanup(stopServers)
	setCommand := func(command string) {
		raw := withPluginSetting(hs.a.settings.Plugins, "lsp-typescript", "command", command, nil)
		if err := hs.a.settings.Prefs.Set("plugins", raw); err != nil {
			t.Fatal(err)
		}
		h.syncPrefs()
	}
	setCommand(hs.path("missing ts server"))
	path := hs.path("App.tsx")
	if err := os.WriteFile(path, []byte("const element = <div>bad</div>;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.untilLong("the configured executable failing to start", func() bool { return hs.a.fails == 1 })
	setCommand(os.Args[0])
	hs.a.restartLanguageServers("lsp-typescript.server")
	hs.untilLong("TSX language server diagnostics", func() bool { return len(h.diagnosticsOf(path)) == 1 })
	s := hs.a.lspFor(hs.a.activeEditor().document, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := s.live().Call(ctx, "test/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Initialize struct {
			Options struct {
				TSServer struct{ UseSyntaxServer string } `json:"tsserver"`
			} `json:"initializationOptions"`
		} `json:"initialize"`
		Document struct {
			TextDocument struct {
				LanguageID string `json:"languageId"`
				URI        string `json:"uri"`
			} `json:"textDocument"`
		} `json:"document"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Document.TextDocument; got.LanguageID != "typescriptreact" || got.URI != serverURI(path) {
		t.Fatalf("TSX didOpen = %s", raw)
	}
	if snapshot.Initialize.Options.TSServer.UseSyntaxServer != "never" {
		t.Fatalf("TSX initialization options = %s", raw)
	}
}

func TestMarketplaceServerSettingArguments(t *testing.T) {
	hs := newHarness(t, false)
	host := withMarketplaceLanguageServers(hs)
	for _, provider := range host.reg.Plugins() {
		for _, server := range provider.Contributes.LanguageServers {
			if server.Resolve != nil {
				continue
			}
			settings := host.commandSettings(provider)
			launch, err := settings.staticServer(server)
			if err != nil || launch.Command == "" || strings.Contains(launch.Command, "${") || !slices.Equal(launch.Args, server.Args) {
				t.Fatalf("%s defaults: %+v, %v", provider.Name, launch, err)
			}
			settings.explicit = map[string]json.RawMessage{"command": json.RawMessage(`"./my tools/${settings.command}"`)}
			if provider.Name == "lsp-clangd" {
				settings.explicit["compileCommandsDir"] = json.RawMessage(`"build dirs/${value}"`)
				settings.explicit["headerInsertion"] = json.RawMessage(`"never"`)
			}
			launch, err = settings.staticServer(server)
			if err != nil || launch.Command != "./my tools/${settings.command}" {
				t.Fatalf("%s override: %+v, %v", provider.Name, launch, err)
			}
			if provider.Name == "lsp-clangd" && !slices.Equal(launch.Args, []string{"--compile-commands-dir=build dirs/${value}", "--header-insertion=never"}) {
				t.Fatal(launch.Args)
			}
		}
	}
}

package extensions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/plugin"
)

func TestMarketplaceLanguageServerManifests(t *testing.T) {
	files, err := fs.Glob(os.DirFS("."), "lsp-*/plugin.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("language server manifests: %v, %v", files, err)
	}
	languages := map[string]string{}
	extensions := map[string]string{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m, err := plugin.ParseManifest(data)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if m.HasCode() || m.DefaultOn() {
			t.Errorf("%s should declare an opt-in language server without a JS runtime", file)
		}
		declared := map[string]bool{}
		for _, l := range m.Contributes.Languages {
			declared[l.ID] = true
			for _, ext := range l.Extensions {
				ext = strings.ToLower(ext)
				if other := extensions[ext]; other != "" {
					t.Errorf("%s and %s both claim %s", other, m.Name, ext)
				}
				extensions[ext] = m.Name
			}
		}
		for _, server := range m.Contributes.LanguageServers {
			for _, language := range server.Languages {
				if !declared[language] {
					t.Errorf("%s cannot identify its language %s", m.Name, language)
				}
				if other := languages[language]; other != "" {
					t.Errorf("%s and %s both serve %s", other, server.ID, language)
				}
				languages[language] = server.ID
			}
		}
	}
}

// The bundled catalog's Toolbox tools keep their ids and pages apart, so
// the window can route one to a plugin.
func TestMarketplaceToolboxManifests(t *testing.T) {
	data, err := os.ReadFile("index.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []plugin.IndexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"toolbox-base64": true, "toolbox-json": true, "toolbox-timestamp": true, "toolbox-url": true}
	for _, entry := range entries {
		if entry.Category != "tools" {
			continue
		}
		// The fallback catalog mirrors each plugin here, so the test does
		// not need a checkout of the marketplace repository.
		m, err := plugin.ReadManifest(entry.Name)
		if err != nil {
			t.Fatalf("%s: %v", entry.Name, err)
		}
		if m.Contributes.Toolbox == nil {
			continue
		}
		release, ok := entry.Newest()
		if !ok || release.Path != "plugins/tools/"+m.Name || release.Version != m.Version || release.Commit == "" {
			t.Errorf("%s has no matching pinned marketplace release: %+v", m.Name, release)
		}
		if want[m.Name] {
			if m.Contributes.Toolbox.Builtin != "devtool" || m.Contributes.Toolbox.Page != "" {
				t.Errorf("%s does not use its original DevTool page", entry.Name)
			}
		} else if info, err := os.Stat(filepath.Join(entry.Name, m.Contributes.Toolbox.Page)); err != nil || !info.Mode().IsRegular() {
			t.Errorf("%s has no plugin page: %v", entry.Name, err)
		}
		delete(want, m.Name)
	}
	if len(want) > 0 {
		t.Errorf("missing marketplace tools: %v", want)
	}
}

func TestVueResolver(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed to exercise the Vue launch resolver")
	}
	data, err := os.ReadFile("lsp-vue/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := plugin.ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	resolver := m.Contributes.LanguageServers[0].Resolve
	root := t.TempDir()
	resolve := func(root string, settings map[string]string) ([]byte, string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, node, resolver.Args...)
		input, err := json.Marshal(map[string]any{"root": root, "settings": settings})
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		return out, stderr.String(), err
	}
	if _, stderr, err := resolve(root, nil); err == nil || !strings.Contains(stderr, "npm install -D typescript") {
		t.Fatalf("missing TypeScript: %v, %s", err, stderr)
	}
	lib := filepath.Join(root, "node_modules", "typescript", "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "typescript.js"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "..", "package.json"), []byte(`{"name":"typescript","main":"lib/typescript.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, command, tsdk string
		settings            map[string]string
	}{
		{"workspace", "vue-language-server", lib, nil},
		{"override", "custom-vue-server", filepath.Join(root, "custom", "lib"), map[string]string{"command": "custom-vue-server", "tsdkPath": "custom/lib"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, stderr, err := resolve(root, tc.settings)
			if err != nil {
				t.Fatalf("resolve: %v, %s", err, stderr)
			}
			var launch struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
				Options struct {
					TypeScript struct{ Tsdk string }     `json:"typescript"`
					Vue        struct{ HybridMode bool } `json:"vue"`
				} `json:"initializationOptions"`
			}
			if err := json.Unmarshal(out, &launch); err != nil {
				t.Fatal(err)
			}
			// Node resolves symlinks while finding the package, including macOS /var.
			want := tc.tsdk
			if tc.settings == nil {
				want, err = filepath.EvalSymlinks(want)
				if err != nil {
					t.Fatal(err)
				}
			}
			if launch.Command != tc.command || len(launch.Args) != 1 || launch.Args[0] != "--stdio" ||
				launch.Options.TypeScript.Tsdk != want || launch.Options.Vue.HybridMode {
				t.Fatalf("launch = %s", out)
			}
		})
	}
}

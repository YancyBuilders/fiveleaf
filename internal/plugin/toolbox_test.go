package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevToolBuiltinAndInstalledRelease(t *testing.T) {
	registry, _, dirs := newRegistry(t)
	const name = "toolbox-timestamp"
	writePlugin(t, dirs.Plugins, name, map[string]string{
		ManifestFile: `{"name":"toolbox-timestamp","version":"1.1.0","contributes":{"toolbox":{"id":"time","page":"index.html"}}}`,
		"index.html": "old page",
	})
	const pinned = "161648266c015e7e1a1bb46271188cb372627270"
	const official = "https://github.com/YancyBuilders/fiveleaf-plugins.git#plugins/tools/toolbox-timestamp"
	for _, sample := range []struct{ repository, commit, want string }{
		{official, pinned, "time"}, {official, "another-commit", ""}, {"https://github.com/another/toolbox", pinned, ""},
	} {
		if err := WriteRecords(dirs.Plugins, Records{name: {URL: sample.repository, Commit: sample.commit, Version: "1.1.0"}}); err != nil {
			t.Fatal(err)
		}
		registry.Load()
		if got := registry.Plugin(name).DevToolPage(); got != sample.want {
			t.Errorf("%s@%s page = %q, want %q", sample.repository, sample.commit, got, sample.want)
		}
	}
	for _, contribution := range []string{`{"builtin":"external","id":"json"}`, `{"builtin":"devtool","id":"unknown"}`, `{"builtin":"devtool","id":"json","page":"index.html"}`} {
		if _, err := ParseManifest([]byte(`{"name":"bad-tool","contributes":{"toolbox":` + contribution + `}}`)); err == nil {
			t.Errorf("accepted %s", contribution)
		}
	}
}

func TestToolboxFrameworkTemplates(t *testing.T) {
	for _, template := range []struct{ flag, component string }{{"--tool=vue", "App.vue"}, {"--tool=react", "main.jsx"}} {
		t.Run(template.flag, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "my-tool")
			var output strings.Builder
			if RunCLI([]string{"new", "my-tool", dir, template.flag}, Dirs{}, &output, &output) != 0 {
				t.Fatal(output.String())
			}
			manifest, err := ReadManifest(dir)
			if err != nil || manifest.Contributes.Toolbox.Page != "dist/index.html" || !strings.Contains(output.String(), "npm run build") {
				t.Fatalf("framework entry %v, %v: %s", manifest, err, output.String())
			}
			for _, file := range []string{"index.html", "package.json", "style.css", template.component} {
				if info, err := os.Stat(filepath.Join(dir, file)); err != nil || info.Size() == 0 {
					t.Errorf("missing template file %s: %v", file, err)
				}
			}
		})
	}
}

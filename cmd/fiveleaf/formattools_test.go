package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/plugin"
	"fiveleaf/internal/remote"
	"github.com/egoist/mygo/ui"
)

func enableMarketplacePlugins(hs *harness, names ...string) *pluginHost {
	host := withPlugins(hs, nil)
	installCatalogFixtures(hs, host, names...)
	hs.a.settings.Plugins = map[string]json.RawMessage{}
	for _, name := range names {
		hs.a.settings.Plugins[name] = json.RawMessage(`{"enabled":true}`)
	}
	host.syncPrefs()
	return host
}

func TestMarketplaceFormatterRouting(t *testing.T) {
	hs := newHarness(t, false)
	enableMarketplacePlugins(hs, "format-prettier", "format-ruff", "format-clang")
	for _, sample := range []struct{ path, language, tool string }{
		{"App.tsx", "typescriptreact", "format-prettier"}, {"View.jsx", "javascriptreact", "format-prettier"},
		{"esm.mts", "typescript", "format-prettier"}, {"worker.cjs", "javascript", "format-prettier"},
		{"theme.scss", "scss", "format-prettier"}, {"App.vue", "vue", "format-prettier"},
		{"tsconfig.json", "jsonc", "format-prettier"}, {"ci.yml", "yaml", "format-prettier"},
		{"README.md", "markdown", "format-prettier"}, {"pkg.pyi", "python", "format-ruff"},
		{"view.pyw", "python", "format-ruff"}, {"point.hpp", "cpp", "format-clang"}, {"point.h", "c", "format-clang"},
	} {
		doc := &document{path: hs.path(sample.path)}
		provider, _ := hs.a.formatterFor(doc)
		if language, _ := formatterLanguages(doc); language != sample.language || provider == nil || provider.Name != sample.tool {
			t.Errorf("%s resolved to %s with provider %v, want %s / %s", sample.path, language, provider != nil, sample.language, sample.tool)
		}
		if _, server := serverContrib(sample.language); server != nil {
			t.Errorf("%s activated a language server", sample.path)
		}
	}
	if provider, _ := hs.a.formatterFor(&document{path: hs.path("unrelated.txt")}); provider != nil {
		t.Fatal("an unrelated document acquired a formatter")
	}
	jsx := &document{path: hs.path("App.jsx")}
	hs.a.settings.FormatOnSave = map[string]bool{lexerLanguage(jsx): true}
	if !hs.a.formatsOnSave(jsx) {
		t.Fatal("lost the existing JSX format-on-save setting")
	}
	hs.a.settings.FormatOnSave["javascriptreact"] = false
	if hs.a.formatsOnSave(jsx) {
		t.Fatal("ignored the explicit language setting")
	}
}

func TestFormatterRunsOnDocumentHost(t *testing.T) {
	decl := &plugin.FormatterContrib{Command: "prettier", Args: []string{"--stdin-filepath", "${file}", "--tab-width=${tabSize}", "--use-tabs=${useTabs}"}}
	for _, sample := range []struct{ root, file, dir, native string }{
		{remote.Path("work", "/src"), remote.Path("work", "/src/pkg/a b.ts"), remote.Path("work", "/src"), "/src/pkg/a b.ts"},
		{remote.Path("other", "/src"), remote.Path("work", "/src/pkg/a b.ts"), remote.Path("work", "/src/pkg"), "/src/pkg/a b.ts"},
		{"", remote.Path("work", "/pkg/a b.ts"), remote.Path("work", "/pkg"), "/pkg/a b.ts"},
	} {
		tool := formatterProgram(sample.root, sample.file, decl, 4, true)
		if tool.dir != sample.dir || len(tool.args) != 4 || tool.args[1] != sample.native || tool.args[2] != "--tab-width=4" || tool.args[3] != "--use-tabs=true" {
			t.Fatalf("formatter for %s: %+v", sample.file, tool)
		}
	}
}

func TestFormatterRejectsInvalidStyleSetting(t *testing.T) {
	hs := newHarness(t, false)
	host := enableMarketplacePlugins(hs, "format-ruff")
	provider := host.reg.Plugin("format-ruff")
	flags := provider.Contributes.Formatters[0].SettingArgs
	for _, invalid := range []struct{ key, raw string }{
		{"quoteStyle", `"typo"`}, {"lineLength", `"96"`}, {"lineLength", `0`}, {"lineLength", `1.5`}, {"lineLength", `65536`},
	} {
		settings := host.commandSettings(provider)
		settings.explicit = map[string]json.RawMessage{invalid.key: json.RawMessage(invalid.raw)}
		if _, err := settings.appendSettingArgs(nil, flags); err == nil || !strings.Contains(err.Error(), invalid.key) {
			t.Errorf("%s = %s: %v", invalid.key, invalid.raw, err)
		}
	}
}

func TestUnavailableFormatterSavesUnsavedText(t *testing.T) {
	hs, doc := editHarness(t, "work.py", "pass\n")
	host := enableMarketplacePlugins(hs, "format-ruff")
	config, err := json.Marshal(map[string]any{"enabled": true, "command": hs.path("missing-ruff")})
	if err != nil {
		t.Fatal(err)
	}
	hs.a.settings.Plugins["format-ruff"] = config
	host.syncPrefs()
	const pending = "work={  'pending': True }\n"
	doc.edit(doc.sel, textEdit{0, doc.buf.Len(), pending})
	before := hs.a.fails
	finished := false
	hs.a.format(doc, time.Second, false, func() { finished = true })
	hs.until("the formatter error", func() bool { return finished })
	if hs.a.fails == before || !doc.dirty() {
		t.Fatal("missing tool didn't report an error and keep the pending edit")
	}
	expectText(t, doc, pending)
	hs.a.settings.FormatOnSave = map[string]bool{"python": true}
	finished = false
	hs.a.saveDoc(doc.document, func() { finished = true })
	hs.until("saving after the formatter failed", func() bool { return finished })
	if data, err := os.ReadFile(doc.path); err != nil || string(data) != pending {
		t.Fatalf("saved %q, %v", data, err)
	}
}

func TestFormatterEmptyOutput(t *testing.T) {
	_, doc := editHarness(t, "blank.ts", " \t\n")
	snap := doc.buf.Snapshot()
	if changes := pluginFormatEdits(snap, json.RawMessage("null")); len(changes) != 0 {
		t.Fatal("null modified a whitespace-only document")
	}
	applyFormatEdits(doc, pluginFormatEdits(snap, json.RawMessage(`""`)))
	expectText(t, doc, "")
}

func TestInstalledFormatterCommands(t *testing.T) {
	if os.Getenv("FIVELEAF_TEST_REAL_FORMATTERS") == "" {
		t.Skip("set FIVELEAF_TEST_REAL_FORMATTERS to use the installed formatting tools")
	}
	for _, sample := range []struct{ plugin, command, name, config, settings, source, want string }{
		{"format-prettier", "prettier", "User Card.tsx", ".prettierrc", `{"singleQuote":true,"semi":false,"endOfLine":"crlf"}`, `export const card=<div title={"Ada"}/>;`, "export const card = <div title={'Ada'} />\n"},
		{"format-prettier", "prettier", "Card.vue", ".prettierrc", `{}`, `<template><button   id="card">Ada</button></template>`, "<template><button id=\"card\">Ada</button></template>\n"},
		{"format-prettier", "prettier", "index.html", ".prettierrc", `{}`, `<section   id="app"><h1>Ada</h1></section>`, "<section id=\"app\"><h1>Ada</h1></section>\n"},
		{"format-prettier", "prettier", "settings.jsonc", ".prettierrc", `{"trailingComma":"none"}`, "{// stays\n\"user\":{\"name\":\"Ada\"}}", "{\n  // stays\n  \"user\": { \"name\": \"Ada\" }\n}\n"},
		{"format-prettier", "prettier", "theme.scss", ".prettierrc", `{"tabWidth":4}`, ".card{color:red;}", ".card {\n    color: red;\n}\n"},
		{"format-prettier", "prettier", "site.yml", ".prettierrc", `{}`, "names: [Ada,Bob]", "names: [Ada, Bob]\n"},
		{"format-prettier", "prettier", "guide.md", ".prettierrc", `{}`, "-   Ada\n-   Bob", "- Ada\n- Bob\n"},
		{"format-ruff", "ruff", "user model.py", "ruff.toml", "[format]\nquote-style = 'single'\n", `user={"name":"Ada"}`, "user = {'name': 'Ada'}\n"},
		{"format-clang", "clang-format", "user model.hpp", ".clang-format", "BasedOnStyle: LLVM\nPointerAlignment: Left\n", "int*  user( );", "int* user();\n"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			if _, err := exec.LookPath(sample.command); err != nil {
				t.Fatal(err)
			}
			hs := newHarness(t, false)
			enableMarketplacePlugins(hs, sample.plugin)
			dir := hs.path("nested")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, sample.name)
			for file, data := range map[string]string{path: "\r\n", filepath.Join(dir, sample.config): sample.settings} {
				if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			hs.a.openFile(path, true)
			hs.settle()
			doc := hs.a.activeEditor()
			doc.edit(doc.sel, textEdit{0, doc.buf.Len(), sample.source + "\n"})
			hs.a.run("sel.format")
			hs.until("the formatted unsaved buffer", func() bool { return doc.buf.String() == sample.want || hs.a.fails > 0 })
			if hs.a.fails > 0 {
				t.Fatal(hs.a.errors)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "\r\n" {
				t.Fatalf("manual format changed the disk: %q, %v", data, err)
			}
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, doc, sample.source+"\n")
			hs.a.settings.FormatOnSave = map[string]bool{languageOf(doc.document): true}
			saved := false
			hs.a.saveDoc(doc.document, func() { saved = true })
			hs.until("the formatted save", func() bool { return saved })
			if data, err := os.ReadFile(path); err != nil || string(data) != strings.ReplaceAll(sample.want, "\n", "\r\n") {
				t.Fatalf("formatted save = %q, %v", data, err)
			}
		})
	}
}

func TestInstalledFormatterStyleSettings(t *testing.T) {
	if os.Getenv("FIVELEAF_TEST_REAL_FORMATTERS") == "" {
		t.Skip("set FIVELEAF_TEST_REAL_FORMATTERS to use the installed formatting tools")
	}
	const prettierProject = "export const card = { name: \"Ada\", role: \"guest\" };\n"
	const ruffPersonal = "value = build(\n    'alpha',\n    'beta',\n    'gamma',\n)\n"
	const clangProject = "class Card {\npublic:\n  int value;\n};"
	for _, sample := range []struct {
		plugin, file, source, userSettings, configFile, config string
		personal, withProject, afterReset                      string
	}{
		{
			"format-prettier", "user ${settings.command}.ts", `export const card={name:"Ada",role:"guest"};`,
			`{"enabled":true,"singleQuote":true,"semi":false,"printWidth":24}`, ".prettierrc", `{"singleQuote":false,"semi":true,"printWidth":100}`,
			"export const card = {\n  name: 'Ada',\n  role: 'guest',\n}\n", prettierProject, prettierProject,
		},
		{
			"format-ruff", "user ${settings.command}.py", `value=build("alpha","beta","gamma")`,
			`{"enabled":true,"lineLength":18,"quoteStyle":"single"}`, "ruff.toml", "line-length = 100\n[format]\nquote-style = 'double'\n",
			ruffPersonal, ruffPersonal, "value = build(\"alpha\", \"beta\", \"gamma\")\n",
		},
		{
			"format-clang", "user ${settings.command}.hpp", "class Card{public:int value;};",
			`{"enabled":true,"fallbackStyle":"Microsoft"}`, ".clang-format", "BasedOnStyle: LLVM\n",
			"class Card\n{\n  public:\n    int value;\n};", clangProject, clangProject,
		},
	} {
		t.Run(sample.plugin, func(t *testing.T) {
			hs, doc := editHarness(t, sample.file, "\n")
			host := enableMarketplacePlugins(hs, sample.plugin)
			format := func(want string) {
				t.Helper()
				doc.edit(doc.sel, textEdit{0, doc.buf.Len(), sample.source})
				done := false
				hs.a.format(doc, 4*time.Second, false, func() { done = true })
				hs.until("formatting the current buffer", func() bool { return done })
				if hs.a.fails > 0 {
					t.Fatal(hs.a.errors)
				}
				expectText(t, doc, want)
			}
			hs.a.settings.Plugins[sample.plugin] = json.RawMessage(sample.userSettings)
			host.syncPrefs()
			format(sample.personal)
			if err := os.WriteFile(hs.path(sample.configFile), []byte(sample.config), 0o600); err != nil {
				t.Fatal(err)
			}
			format(sample.withProject)
			hs.a.settings.Plugins[sample.plugin] = json.RawMessage(`{"enabled":true}`)
			host.syncPrefs()
			format(sample.afterReset)
			if data, err := os.ReadFile(doc.path); err != nil || string(data) != "\n" {
				t.Fatalf("manual formatting changed the disk: %q, %v", data, err)
			}
		})
	}
}

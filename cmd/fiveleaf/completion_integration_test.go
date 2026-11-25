package main

import (
	"context"
	"go/format"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/lsp"
	"github.com/egoist/mygo/ui"
)

func TestGoplsCaretCompletionAndFormatting(t *testing.T) {
	if os.Getenv("FIVELEAF_TEST_REAL_GOPLS") == "" {
		t.Skip("set FIVELEAF_TEST_REAL_GOPLS to test the installed gopls")
	}
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Fatal(err)
	}
	hs := newHarness(t, false)
	enableMarketplacePlugins(hs, "lsp-go")
	t.Cleanup(stopServers)
	const source = "package main\n\ntype address struct { PostalCode int }\nfunc postal(home address) int { return home }\n"
	path := hs.path("postal.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	doc := hs.a.activeEditor()
	hs.untilLong("gopls member completion", func() bool {
		client := hs.a.lspFor(doc.document, false).live()
		return client != nil && client.TriggersCompletion(".") && client.Formatting()
	})
	caret := strings.LastIndex(source, "home") + len("home")
	hs.place(doc, caret, caret)
	hs.tt.Type(".")
	hs.untilLong("PostalCode beside the caret", func() bool {
		_, found := hs.tt.Find("PostalCode")
		return found && !hs.a.pluginPick.open
	})
	hs.tt.Type("P")
	hs.untilLong("the first member character keeps PostalCode", func() bool {
		_, found := hs.tt.Find("PostalCode")
		return found && hs.a.wordSuggest != nil && hs.a.wordSuggest.version == doc.buf.Version()
	})
	hs.key(0, ui.KeyTab)
	completed := strings.Replace(source, "return home", "return home.PostalCode", 1)
	expectText(t, doc, completed)
	formatted, err := format.Source([]byte(completed))
	if err != nil {
		t.Fatal(err)
	}
	hs.a.run("sel.format")
	hs.until("gopls document formatting", func() bool { return doc.buf.String() == string(formatted) })
}

func TestInstalledLanguageMemberCompletion(t *testing.T) {
	if os.Getenv("FIVELEAF_TEST_REAL_COMPLETIONS") == "" {
		t.Skip("set FIVELEAF_TEST_REAL_COMPLETIONS to use the installed language servers")
	}
	for _, sample := range []struct {
		plugin, command, file, source string
		dependencies                  map[string]string
	}{
		{"lsp-typescript", "typescript-language-server", "app.ts", "import {home} from './model';\nconst code = home\n", map[string]string{"model.ts": "export const home = { postalCode: 42 };\n", "tsconfig.json": `{"compilerOptions":{"module":"esnext","moduleResolution":"bundler"}}`}},
		{"lsp-typescript", "typescript-language-server", "app.js", "import {home} from './model.js';\nconst code = home\n", map[string]string{"model.js": "export const home = { postalCode: 42 };\n"}},
		{"lsp-python", "pyright-langserver", "app.py", "from model import Address\nhome = Address()\ncode = home\n", map[string]string{"model.pyi": "class Address:\n    postalCode: int\n"}},
		{"lsp-clangd", "clangd", "app.cpp", "#include \"model.hpp\"\nint code(Address home) { return home; }\n", map[string]string{"model.hpp": "struct Address { int postalCode; };\n"}},
		{"lsp-rust", "rust-analyzer", "lib.rs", "mod model;\nuse model::Address;\npub fn code(home: Address)->u32{home}\n", map[string]string{
			"Cargo.toml": "[package]\nname='postal'\nversion='0.1.0'\nedition='2021'\n[lib]\npath='lib.rs'\n",
			"model.rs":   "pub struct Address { pub postalCode: u32 }\n",
		}},
	} {
		t.Run(sample.file, func(t *testing.T) {
			if _, err := exec.LookPath(sample.command); err != nil {
				t.Fatal(err)
			}
			hs := newHarness(t, false)
			host := enableMarketplacePlugins(hs, sample.plugin)
			t.Cleanup(stopServers)
			t.Cleanup(func() {
				if t.Failed() {
					t.Log(host.reg.Plugin(sample.plugin).Logs())
				}
			})
			path := hs.path(sample.file)
			sample.dependencies[sample.file] = sample.source
			for name, source := range sample.dependencies {
				if err := os.WriteFile(hs.path(name), []byte(source), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			hs.a.openFile(path, true)
			hs.settle()
			doc := hs.a.activeEditor()
			hs.untilLong("the server's dot completion capability", func() bool {
				server := hs.a.lspFor(doc.document, false)
				if server != nil && server.state() == lspFailed {
					t.Fatalf("start %s: %v", sample.command, server.err)
				}
				client := server.live()
				return client != nil && client.TriggersCompletion(".")
			})
			if sample.plugin == "lsp-rust" {
				client := hs.a.lspFor(doc.document, false).live()
				pos := lsp.Position(positionOf(doc.buf.Snapshot(), strings.Index(sample.source, "Address")+1))
				model, err := os.Stat(hs.path("model.rs"))
				if err != nil {
					t.Fatal(err)
				}
				hs.untilLong("rust-analyzer resolving the model in this crate", func() bool {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					locations, _ := client.DefinitionAt(ctx, serverURI(path), pos)
					if len(locations) == 0 {
						return false
					}
					found, err := os.Stat(lsp.Path(locations[0].URI))
					return err == nil && os.SameFile(model, found)
				})
			}
			caret := strings.LastIndex(sample.source, "home") + len("home")
			hs.place(doc, caret, caret)
			hs.tt.Type(".")
			hs.untilLong("the postalCode member after a dot", func() bool {
				return hs.a.wordSuggest != nil && len(hs.a.wordSuggest.items) > 0
			})
			hs.tt.Type("pos")
			hs.untilLong("the filtered member beside the caret", func() bool {
				suggestions := hs.a.wordSuggest
				return suggestions != nil && suggestions.version == doc.buf.Version() && len(suggestions.items) > 0 && strings.TrimSpace(suggestions.items[0].Label) == "postalCode"
			})
			hs.key(0, ui.KeyTab)
			expectText(t, doc, sample.source[:caret]+".postalCode"+sample.source[caret:])
			if sample.plugin == "lsp-rust" {
				hs.a.run("sel.format")
				hs.until("rustfmt through rust-analyzer", func() bool { return strings.Contains(doc.buf.String(), "-> u32 {\n    home.postalCode\n}") })
			}
		})
	}
}

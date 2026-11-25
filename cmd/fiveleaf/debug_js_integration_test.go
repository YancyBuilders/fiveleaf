package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMarketplaceJavaScriptDebugger(t *testing.T) {
	server := os.Getenv("FIVELEAF_TEST_JS_DEBUG")
	if server == "" {
		t.Skip("set FIVELEAF_TEST_JS_DEBUG to the standalone js-debug/src/dapDebugServer.js")
	}
	server, err := filepath.Abs(server)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(server); err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct{ name, body string }{
		{"program.js", "const base = 2\nconst value = base * 3\nconsole.log(value)\n"},
		{"program.ts", "const base: number = 2\nconst value = base * 3\nconsole.log(value)\n"},
	} {
		t.Run(source.name, func(t *testing.T) {
			if filepath.Ext(source.name) == ".ts" && exec.Command("node", "-e", "process.exit(process.features.typescript ? 0 : 1)").Run() != nil {
				t.Skip("requires Node.js with TypeScript type stripping enabled")
			}
			harness := newHarness(t, false)
			host := withPlugins(harness, nil)
			installCatalogFixtures(harness, host, "debug-js")
			dir := filepath.Join(host.reg.Dirs.Plugins, "debug-js", "js-debug")
			if err := os.CopyFS(dir, os.DirFS(filepath.Dir(filepath.Dir(server)))); err != nil {
				t.Fatal(err)
			}
			app := harness.a
			if app.settings.Plugins == nil {
				app.settings.Plugins = map[string]json.RawMessage{}
			}
			app.settings.Plugins["debug-js"] = json.RawMessage(`{"enabled":true}`)
			host.syncPrefs()
			file := harness.path(source.name)
			if err := os.WriteFile(file, []byte(source.body), 0o644); err != nil {
				t.Fatal(err)
			}
			app.openFile(file, true)
			harness.settle()
			app.toggleBreakpointAt(file, 1)
			app.run("debug.start")
			t.Cleanup(app.debugStop)
			harness.untilLong("the JavaScript child session to stop on source line 2", func() bool {
				if app.fails != 0 {
					t.Fatalf("debugger errors: %v", app.errors)
				}
				if !app.debug.active() {
					t.Fatalf("debugger exited before the breakpoint: %s; logs: %+v", app.debug.output, host.reg.Plugin("debug-js").Logs())
				}
				return app.debug.pausedLine(file) == 1
			})
			app.run("debug.stepOver")
			harness.until("the JavaScript child session to step to line 3", func() bool { return app.debug.pausedLine(file) == 2 })
			app.run("debug.continue")
			harness.untilLong("the continued JavaScript program to exit", func() bool { return !app.debug.active() })
			if app.fails != 0 {
				t.Fatal(app.errors)
			}
		})
	}
}

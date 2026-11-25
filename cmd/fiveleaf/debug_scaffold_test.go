package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
)

func TestScaffoldedDebugAdapterStopsAtSourceBreakpoint(t *testing.T) {
	python := os.Getenv("FIVELEAF_TEST_DEBUGPY")
	if python == "" || testing.Short() {
		t.Skip("set FIVELEAF_TEST_DEBUGPY to a Python interpreter with debugpy")
	}
	harness := newHarness(t, false)
	host := withPlugins(harness, nil)
	folder := filepath.Join(t.TempDir(), "adapter with spaces")
	var output strings.Builder
	if code := plugin.RunCLI([]string{"new", "sample-debug", folder, "--debug-adapter"}, host.reg.Dirs, &output, &output); code != 0 {
		t.Fatal(output.String())
	}
	if code := plugin.RunCLI([]string{"dev", folder}, host.reg.Dirs, &output, &output); code != 0 {
		t.Fatal(output.String())
	}
	prefs, err := json.Marshal(map[string]string{"pythonPath": python})
	if err != nil {
		t.Fatal(err)
	}
	harness.a.settings.Plugins = map[string]json.RawMessage{"sample-debug": prefs}
	host.syncPrefs()
	host.reg.Load()
	const filename = "example.sample-debug"
	sample, err := os.ReadFile(filepath.Join(folder, filename))
	if err != nil {
		t.Fatal(err)
	}
	file := harness.path(filename)
	if err = os.WriteFile(file, sample, 0o600); err != nil {
		t.Fatal(err)
	}
	harness.a.openFile(file, true)
	harness.settle()
	editor := harness.a.activeEditor()
	if languageOf(editor.document) != "sample-debug" {
		t.Fatal("the linked plugin did not register its own language")
	}
	editor.goToLine(2, 1)
	harness.settle()
	harness.key(0, ui.KeyF9)
	debug := &harness.a.debug
	if !debug.hasBreakpoint(file, 1) {
		t.Fatalf("F9 set breakpoints %v at selection %v", harness.a.debugBreakpoints(file), editor.sel)
	}
	t.Cleanup(harness.a.debugStop)
	harness.a.run("debug.start")
	harness.untilLong("the generated adapter to stop with its local variable", func() bool {
		if harness.a.fails != 0 {
			t.Fatalf("debug.start failed: %v", harness.a.errors)
		}
		if debug.pausedLine(file) != 1 {
			return false
		}
		debug.mu.Lock()
		defer debug.mu.Unlock()
		return slices.ContainsFunc(debug.vars, func(variable dapVariable) bool {
			return variable.Name == "message" && strings.Contains(variable.Value, "Hello from FiveLeaf")
		})
	})
	harness.a.run("debug.continue")
	harness.untilLong("the sample to finish", func() bool { return !debug.active() })
}

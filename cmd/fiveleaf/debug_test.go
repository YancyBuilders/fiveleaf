package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/plugin"
)

func init() {
	if os.Getenv("FIVELEAF_TEST_DEBUG_RESOLVER") != "1" {
		return
	}
	switch os.Getenv("FIVELEAF_TEST_DEBUG_RESOLVER_MODE") {
	case "malformed":
		fmt.Fprint(os.Stdout, `{invalid`)
		os.Exit(0)
	case "extra":
		fmt.Fprint(os.Stdout, `{"command":"server"} {}`)
		os.Exit(0)
	case "exit":
		fmt.Fprint(os.Stderr, "resolver setup failed")
		os.Exit(3)
	case "no-command":
		fmt.Fprint(os.Stdout, `{"args":["server"]}`)
		os.Exit(0)
	case "timeout":
		time.Sleep(time.Hour)
	}
	var request struct {
		Root     string                     `json:"root"`
		File     string                     `json:"file"`
		Settings map[string]json.RawMessage `json:"settings"`
	}
	data, _ := io.ReadAll(os.Stdin)
	if json.Unmarshal(data, &request) != nil || request.Root == "" {
		os.Exit(2)
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{
		"command":   os.Args[0],
		"args":      []string{"-test.run=^$"},
		"env":       map[string]string{"FIVELEAF_DEBUG_RESOLVED": request.File},
		"transport": map[string]any{"kind": "tcp", "ready": "listening: "},
		"launch":    map[string]any{"program": request.File, "mode": "debug"},
	})
	os.Exit(0)
}

func TestDebugBreakpointsAndView(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	path := hs.path("main.go")
	a.openFile(path, true)
	hs.settle()

	if a.debug.hasBreakpoint(path, 2) {
		t.Fatal("a breakpoint existed before one was set")
	}
	d := a.activeEditor()
	tt := hs.tt
	tt.Key(0, ui.KeyF9)
	hs.settle()
	line := d.buf.Snapshot().LineAt(d.sel[1])
	if !a.debug.hasBreakpoint(path, line) {
		t.Fatal("the gutter did not set a breakpoint")
	}
	if a.debug.toggle(path, line) {
		t.Fatal("the gutter's toggle did not report the breakpoint it removed")
	}
	if a.debug.hasBreakpoint(path, line) {
		t.Fatal("the breakpoint was not removed")
	}

	a.run("view.debug")
	hs.settle()
	if a.window.SidebarView != "debug" {
		t.Fatalf("sidebar view = %q", a.window.SidebarView)
	}
	if _, ok := hs.tt.Find("BREAKPOINTS"); !ok {
		t.Fatal("the Debug view does not show its breakpoints section")
	}
	if _, ok := hs.tt.Find("No breakpoints. Click in the editor's margin, or press F9."); !ok {
		t.Fatal("the Debug view does not say it has no breakpoints")
	}
}

func TestResolveDebugAdapter(t *testing.T) {
	t.Setenv("FIVELEAF_TEST_DEBUG_RESOLVER", "1")
	hs := newHarness(t, false)
	file := hs.path("main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := &plugin.DebugAdapterContrib{
		ID: "test.dap", Languages: []string{"go"}, Command: "fallback", Args: []string{"dap"},
		Env:       map[string]string{"DECLARED": "yes"},
		Options:   json.RawMessage(`{"declared":true}`),
		Transport: &plugin.DebugTransport{Kind: "tcp", Ready: "declared: "},
		Resolve:   &plugin.DebugResolver{Command: os.Args[0]},
	}
	workspace := debugWorkspace{root: hs.a.root, file: file, provider: &plugin.Plugin{}}
	launch, err := workspace.resolve(t.Context(), adapter)
	if err != nil {
		t.Fatal(err)
	}
	if launch.Command != os.Args[0] || launch.Env["FIVELEAF_DEBUG_RESOLVED"] != file || launch.Env["DECLARED"] != "yes" {
		t.Fatalf("resolved launch: %+v", launch)
	}
	if launch.Transport == nil || launch.Transport.Ready != "listening: " {
		t.Fatalf("resolved transport: %+v", launch.Transport)
	}
	if launch.Launch["mode"] != "debug" || launch.Launch["program"] != file {
		t.Fatalf("resolved launch arguments: %+v", launch.Launch)
	}
	if len(launch.Options) == 0 || !strings.Contains(string(launch.Options), "declared") {
		t.Fatalf("resolved options: %s", launch.Options)
	}
}

func TestResolveDebugAdapterFailures(t *testing.T) {
	t.Setenv("FIVELEAF_TEST_DEBUG_RESOLVER", "1")
	hs := newHarness(t, false)
	adapter := &plugin.DebugAdapterContrib{
		ID: "test.dap", Languages: []string{"go"},
		Resolve: &plugin.DebugResolver{Command: os.Args[0]},
	}
	for _, tc := range []struct{ mode, want string }{
		{"malformed", "invalid launch description"},
		{"extra", "unexpected resolver output"},
		{"exit", "resolver setup failed"},
		{"no-command", "has no command"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("FIVELEAF_TEST_DEBUG_RESOLVER_MODE", tc.mode)
			workspace := debugWorkspace{root: hs.a.root, file: hs.path("main.go"), provider: &plugin.Plugin{}}
			if _, err := workspace.resolve(t.Context(), adapter); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("resolve: %v; want %q", err, tc.want)
			}
		})
	}
}

func TestDebugResolverManifest(t *testing.T) {
	for _, body := range []string{
		`{"id":"test.dap","languages":["java"],"resolve":{"command":"helper","args":["resolve"]}}`,
		`{"id":"test.dap","languages":["java"],"command":"java"}`,
	} {
		data := fmt.Sprintf(`{"name":"test","contributes":{"debugAdapters":[%s]}}`, body)
		if _, err := plugin.ParseManifest([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`{"id":"test.dap","languages":["java"],"resolve":{}}`} {
		data := fmt.Sprintf(`{"name":"test","contributes":{"debugAdapters":[%s]}}`, body)
		if _, err := plugin.ParseManifest([]byte(data)); err == nil {
			t.Fatal("accepted", body)
		}
	}
}

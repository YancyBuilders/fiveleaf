package dap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func launchAtBreakpoint(ctx context.Context, client *Client, arguments map[string]any, source string, line int) error {
	return client.Launch(ctx, "launch", arguments, func(ctx context.Context) error {
		body, err := client.Call(ctx, "setBreakpoints", map[string]any{
			"source": map[string]any{"path": source}, "breakpoints": []any{map[string]any{"line": line}},
		})
		if err != nil {
			return err
		}
		var response struct {
			Breakpoints []struct {
				Verified bool `json:"verified"`
				Line     int  `json:"line"`
			} `json:"breakpoints"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}
		if len(response.Breakpoints) != 1 || !response.Breakpoints[0].Verified || (response.Breakpoints[0].Line != 0 && response.Breakpoints[0].Line != line) {
			return fmt.Errorf("breakpoint on line %d: %s", line, body)
		}
		return nil
	})
}

func TestDelveTCPLaunchStopsAtBreakpoint(t *testing.T) {
	if _, err := exec.LookPath("dlv"); err != nil {
		t.Skip("dlv is not installed")
	}
	if testing.Short() {
		t.Skip("a real adapter")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("go.mod", "module probe\n\ngo 1.24\n")
	source := write("main.go", "package main\n\nimport \"fmt\"\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc main() {\n\tfmt.Println(add(2, 3))\n}\n")
	build := exec.Command("go", "build", "-gcflags=all=-N -l", "-o", filepath.Join(dir, "app"), ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	cmd := exec.Command("dlv", "dap")
	cmd.Dir = dir
	stopped := make(chan Event, 4)
	c, err := Start(context.Background(), Options{Cmd: cmd, AdapterID: "go", Ready: "DAP server listening at: ", Host: "127.0.0.1",
		Handlers: Handlers{Event: func(e Event) {
			if e.Event == "stopped" {
				stopped <- e
			}
		}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := launchAtBreakpoint(ctx, c, map[string]any{"request": "launch", "mode": "exec", "program": filepath.Join(dir, "app"), "cwd": dir}, source, 6); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-stopped:
		var body struct {
			ThreadID int    `json:"threadId"`
			Reason   string `json:"reason"`
		}
		json.Unmarshal(e.Body, &body)
		if body.Reason == "" || body.ThreadID == 0 {
			t.Fatalf("stopped = %s", e.Body)
		}
	case <-ctx.Done():
		t.Fatal("no stopped event")
	}
}

func TestPythonLaunchStopsAtBreakpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("a real adapter")
	}
	python := os.Getenv("FIVELEAF_TEST_DEBUGPY")
	if python == "" {
		t.Skip("set FIVELEAF_TEST_DEBUGPY to the Python that has debugpy")
	}
	if err := exec.Command(python, "-c", "import debugpy.adapter").Run(); err != nil {
		t.Fatalf("FIVELEAF_TEST_DEBUGPY cannot import debugpy.adapter: %v", err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "main.py")
	if err := os.WriteFile(source, []byte("def add(a, b):\n    return a + b\n\nprint(add(2, 3))\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan Event, 4)
	cmd := exec.Command(python, "-m", "debugpy.adapter")
	cmd.Dir = dir
	c, err := Start(context.Background(), Options{Cmd: cmd, AdapterID: "python", Handlers: Handlers{Event: func(e Event) {
		if e.Event == "stopped" {
			stopped <- e
		}
	}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := launchAtBreakpoint(ctx, c, map[string]any{"request": "launch", "program": source, "cwd": dir, "console": "internalConsole", "justMyCode": false, "python": []string{python}}, source, 2); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-stopped:
		var body struct {
			ThreadID int `json:"threadId"`
		}
		json.Unmarshal(e.Body, &body)
		if body.ThreadID == 0 {
			t.Fatalf("stopped = %s", e.Body)
		}
	case <-ctx.Done():
		t.Fatal("no stopped event")
	}
}

func TestLLDBDAPLaunchStopsAtBreakpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("a real adapter")
	}
	lldb, err := exec.LookPath("lldb-dap")
	if err != nil {
		lldb = "/Applications/Xcode.app/Contents/Developer/usr/bin/lldb-dap"
		if _, statErr := os.Stat(lldb); statErr != nil {
			t.Skip("lldb-dap is not installed")
		}
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc is not installed")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "main.c")
	if err := os.WriteFile(source, []byte("int add(int a, int b) {\n    return a + b;\n}\n\nint main(void) {\n    return add(2, 3);\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "probe")
	if out, err := exec.Command(cc, "-g", "-O0", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	stopped := make(chan Event, 4)
	cmd := exec.Command(lldb)
	cmd.Dir = dir
	c, err := Start(context.Background(), Options{Cmd: cmd, AdapterID: "lldb", Handlers: Handlers{Event: func(e Event) {
		if e.Event == "stopped" {
			stopped <- e
		}
	}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := launchAtBreakpoint(ctx, c, map[string]any{"request": "launch", "program": binary, "cwd": dir, "stopOnEntry": false}, source, 2); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("no stopped event")
	}
}

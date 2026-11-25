package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"fiveleaf/internal/dap"
	"fiveleaf/internal/plugin"
	"fiveleaf/internal/remote"
	"fiveleaf/locales"
)

func TestDebugAdapterSelectionAndBreakpointChanges(t *testing.T) {
	harness := newHarness(t, false)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	manifest := fmt.Sprintf(`{"name":"acme","contributes":{"languages":[{"id":"acme","extensions":[".acme"]}],"debugAdapters":[
		{"id":"acme.other","name":"Other adapter","languages":["acme"],"command":"unselected"},
		{"id":"acme.tcp","name":"Acme debugger","languages":["acme"],"transport":{"kind":"tcp","address":%q},"attach":{"session":42}}
	]}}`, listener.Addr().String())
	withPlugins(harness, map[string]map[string]string{"acme": {"plugin.json": manifest}})
	file := harness.path("program.acme")
	if err := os.WriteFile(file, []byte("first\nsecond\nthird\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	physicalFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan []int, 16)
	firstChange := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(12 * time.Second))
			err = breakpointTestAdapter(conn, physicalFile, received, firstChange)
		}
		finished <- err
	}()
	app := harness.a
	app.openFile(file, true)
	harness.settle()
	app.toggleBreakpointAt(file, 1)
	app.run("debug.start")
	if !app.pluginPick.open || len(app.pluginPick.items) != 2 || app.pluginPick.items[1].Label != "Acme debugger" {
		t.Fatalf("adapter choices: %v", app.pluginPick.items)
	}
	app.closePluginPicker(-1)
	if app.debug.active() {
		t.Fatal("cancelled adapter selection started a session")
	}
	app.run("debug.start")
	app.closePluginPicker(1)
	t.Cleanup(app.debugStop)
	waitFor := func(want []int) {
		t.Helper()
		select {
		case got := <-received:
			if !slices.Equal(got, want) {
				t.Fatalf("setBreakpoints = %v, want %v", got, want)
			}
		case err := <-finished:
			t.Fatalf("adapter quit before receiving breakpoints: %v", err)
		case <-time.After(4 * time.Second):
			t.Fatal("adapter did not receive setBreakpoints")
		}
	}
	waitFor([]int{2})
	app.toggleBreakpointAt(file, 2)
	close(firstChange)
	waitFor([]int{2, 3})
	harness.until("the adapter's breakpoint confirmation", func() bool {
		list := app.debug.at(file)
		return len(list) == 2 && list[0].verified && list[1].verified && app.debug.statusText() == locales.DebugRunning()
	})
	app.removeDebugBreakpoint(file, 1)
	waitFor([]int{3})
	app.toggleBreakpointAt(file, 2)
	waitFor([]int{})
	app.run("debug.pause")
	harness.until("pause of the target thread", func() bool { return app.debug.statusText() == locales.DebugPaused() })
	app.run("debug.continue")
	harness.until("continue response without a continued event", func() bool { return app.debug.statusText() == locales.DebugRunning() })
	app.debugStop()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("adapter did not receive disconnect")
	}
}

func breakpointTestAdapter(conn net.Conn, file string, received chan<- []int, changed <-chan struct{}) error {
	reader := textproto.NewReader(bufio.NewReader(conn))
	write := func(message any) error {
		body, err := json.Marshal(message)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(conn, "Content-Length: %d\r\n\r\n%s", len(body), body)
		return err
	}
	var attachSeq, updates int
	for {
		header, err := reader.ReadMIMEHeader()
		if err != nil {
			return err
		}
		length, err := strconv.Atoi(header.Get("Content-Length"))
		if err != nil || length < 0 || length > 1<<20 {
			return fmt.Errorf("unexpected DAP frame %v", header)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(reader.R, body); err != nil {
			return err
		}
		var request struct {
			Seq       int
			Command   string
			Arguments struct {
				Source      struct{ Path string }
				Breakpoints []struct{ Line int }
				Session     int
				ThreadID    int
			}
		}
		if err := json.Unmarshal(body, &request); err != nil {
			return err
		}
		response := dap.Response{Type: "response", RequestSeq: request.Seq, Command: request.Command, Success: true}
		switch request.Command {
		case "initialize":
			response.Body = json.RawMessage(`{"supportsConfigurationDoneRequest":true}`)
		case "attach":
			if request.Arguments.Session != 42 {
				return fmt.Errorf("attach arguments: %s", body)
			}
			attachSeq = request.Seq
			if err := write(dap.Event{Type: "event", Event: "initialized"}); err != nil {
				return err
			}
			continue
		case "setBreakpoints":
			if request.Arguments.Source.Path != file || request.Arguments.Breakpoints == nil {
				return fmt.Errorf("breakpoint arguments: %s", body)
			}
			lines := make([]int, len(request.Arguments.Breakpoints))
			verified := make([]map[string]any, len(lines))
			for index, bp := range request.Arguments.Breakpoints {
				lines[index] = bp.Line
				verified[index] = map[string]any{"id": bp.Line, "verified": true, "line": bp.Line}
			}
			response.Body, _ = json.Marshal(map[string]any{"breakpoints": verified})
			received <- lines
			updates++
			if updates == 1 {
				select {
				case <-changed:
				case <-time.After(4 * time.Second):
					return fmt.Errorf("missing second edit during launch")
				}
			}
		case "configurationDone":
			if attachSeq == 0 || updates != 2 {
				return fmt.Errorf("configurationDone before synchronizing the second edit")
			}
			if err := write(dap.Response{Type: "response", RequestSeq: attachSeq, Command: "attach", Success: true}); err != nil {
				return err
			}
		case "threads":
			response.Body = json.RawMessage(`{"threads":[{"id":17,"name":"main"}]}`)
		case "pause", "continue":
			if request.Arguments.ThreadID != 17 {
				return fmt.Errorf("invalid thread for %s: %s", request.Command, body)
			}
			if request.Command == "pause" {
				if err := write(dap.Event{Type: "event", Event: "stopped", Body: json.RawMessage(`{"reason":"pause","threadId":17}`)}); err != nil {
					return err
				}
			}
		case "stackTrace":
			response.Body = json.RawMessage(`{"stackFrames":[]}`)
		case "disconnect":
			return write(response)
		default:
			return fmt.Errorf("unexpected client request: %s", request.Command)
		}
		if err := write(response); err != nil {
			return err
		}
	}
}

func TestDebugLaunchExpansion(t *testing.T) {
	workspace := debugWorkspace{
		root: t.TempDir(), provider: &plugin.Plugin{Dir: filepath.Join(t.TempDir(), "plugin dir")},
		settings: pluginCommandSettings{explicit: map[string]json.RawMessage{"runtime": json.RawMessage(`"plain-runtime"`)}},
	}
	workspace.file = filepath.Join(workspace.root, "${settings.untouched}.sample")
	adapter := &plugin.DebugAdapterContrib{Command: "${settings.runtime}", Args: []string{"${pluginDir}/adapter.js"},
		Launch: json.RawMessage(`{"args":["${file}", {"path":"${workspaceFolder}"}], "handle":9007199254740993}`)}
	launch, err := workspace.resolve(t.Context(), adapter)
	if err != nil {
		t.Fatal(err)
	}
	args := launch.Launch["args"].([]any)
	if launch.Command != "plain-runtime" || launch.Args[0] != workspace.provider.Dir+"/adapter.js" || args[0] != workspace.file || args[1].(map[string]any)["path"] != workspace.root || launch.Launch["handle"] != json.Number("9007199254740993") {
		t.Fatalf("expanded configuration: %+v", launch)
	}
	workspace.root, workspace.file = remote.Path("test-host", "/project"), remote.Path("test-host", "/project/test.sample")
	if _, err := workspace.resolve(t.Context(), adapter); err == nil {
		t.Fatal("used a client plugin folder on SSH")
	}
	adapter.Args = nil
	launch, err = workspace.resolve(t.Context(), adapter)
	if err != nil || launch.Launch["cwd"] != "/project" || launch.Launch["program"] != "/project/test.sample" {
		t.Fatalf("host paths = %+v, %v", launch, err)
	}
}

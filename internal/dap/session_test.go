package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

// fakeAdapter is a subprocess that speaks enough DAP to test the client:
// initialize, launch, setBreakpoints, configurationDone, and events.
func TestClientSpeaksDAP(t *testing.T) {
	if os.Getenv("FIVELEAF_DAP_FAKE") == "1" {
		fakeAdapter(os.Stdin, os.Stdout, "")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestClientSpeaksDAP")
	cmd.Env = append(os.Environ(), "FIVELEAF_DAP_FAKE=1")
	events := make(chan Event, 16)
	c, err := Start(context.Background(), Options{Cmd: cmd, AdapterID: "fake", InitializationOptions: json.RawMessage(`{"mode":"test"}`), Handlers: Handlers{
		Event: func(e Event) { events <- e },
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, "launch", map[string]any{"program": "main.go"}); err != nil {
		t.Fatal(err)
	}
	body, err := c.Call(ctx, "setBreakpoints", map[string]any{"source": map[string]any{"path": "/tmp/main.go"}, "breakpoints": []any{map[string]any{"line": 9}}})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Breakpoints []struct {
			Verified bool `json:"verified"`
			Line     int  `json:"line"`
		} `json:"breakpoints"`
	}
	if err := json.Unmarshal(body, &out); err != nil || len(out.Breakpoints) != 1 || !out.Breakpoints[0].Verified || out.Breakpoints[0].Line != 9 {
		t.Fatalf("breakpoints = %s, %v", body, err)
	}
	select {
	case e := <-events:
		if e.Event != "stopped" {
			t.Fatalf("event = %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no stopped event")
	}
}

func TestThirdPartyAdapterHandshake(t *testing.T) {
	for _, order := range []string{"respond-after-configuration", "respond-before-initialized"} {
		t.Run(order, func(t *testing.T) {
			clientSocket, adapterSocket := net.Pipe()
			defer clientSocket.Close()
			go func() { defer adapterSocket.Close(); fakeAdapter(adapterSocket, adapterSocket, order) }()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			client, err := Start(ctx, Options{Address: "127.0.0.1:4711", InitializationOptions: json.RawMessage(`{"mode":"test"}`), DialContext: func(context.Context, string, string) (net.Conn, error) {
				return clientSocket, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			var configured bool
			err = client.Launch(ctx, "attach", nil, func(ctx context.Context) error {
				_, err := client.Call(ctx, "setBreakpoints", nil)
				configured = true
				return err
			})
			if err != nil || !configured {
				t.Fatalf("attach configured=%v: %v", configured, err)
			}
		})
	}
}

func TestChildSessionRequestKeepsReading(t *testing.T) {
	clientSocket, adapterSocket := net.Pipe()
	defer clientSocket.Close()
	go func() { defer adapterSocket.Close(); fakeAdapter(adapterSocket, adapterSocket, "child-request") }()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var client *Client
	startChild := func(request string, configuration map[string]any) error {
		if request != "attach" || configuration["id"] != json.Number("9007199254740993") {
			return fmt.Errorf("bad child request %q: %v", request, configuration)
		}
		_, err := client.Call(ctx, "threads", nil)
		return err
	}
	var err error
	client, err = Start(ctx, Options{Address: "127.0.0.1:4711", InitializationOptions: json.RawMessage(`{"mode":"test"}`),
		Handlers: Handlers{StartDebugging: startChild}, DialContext: func(context.Context, string, string) (net.Conn, error) { return clientSocket, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.Address() != "127.0.0.1:4711" {
		t.Fatal(client.Address())
	}
	if err := client.Launch(ctx, "launch", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func fakeAdapter(in io.Reader, out io.Writer, order string) {
	r := bufio.NewReader(in)
	write := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(data))
		out.Write(data)
	}
	seq := 0
	var pendingAttach map[string]any
	var childRequest int
	var breakpointsSet bool
	read := func() (map[string]json.RawMessage, bool) {
		n := -1
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return nil, false
			}
			fmt.Sscanf(line, "Content-Length: %d", &n)
			if line == "\r\n" || line == "\n" {
				break
			}
		}
		if n < 0 {
			return nil, false
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, false
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(body, &m) != nil {
			return nil, false
		}
		return m, true
	}
	for {
		m, ok := read()
		if !ok {
			return
		}
		var typ, command string
		var reqSeq int
		json.Unmarshal(m["type"], &typ)
		if typ == "response" && childRequest != 0 && pendingAttach != nil {
			if string(m["request_seq"]) != fmt.Sprint(childRequest) || string(m["success"]) != "true" {
				return
			}
			seq++
			pendingAttach["seq"] = seq
			write(pendingAttach)
			pendingAttach, childRequest = nil, 0
		}
		if typ != "request" {
			continue
		}
		json.Unmarshal(m["seq"], &reqSeq)
		json.Unmarshal(m["command"], &command)
		seq++
		resp := map[string]any{"seq": seq, "type": "response", "request_seq": reqSeq, "success": true, "command": command}
		if command == "initialize" {
			var args struct {
				InitializationOptions struct{ Mode string } `json:"initializationOptions"`
				RunInTerminal         bool                  `json:"supportsRunInTerminalRequest"`
				StartDebugging        bool                  `json:"supportsStartDebuggingRequest"`
			}
			json.Unmarshal(m["arguments"], &args)
			if args.InitializationOptions.Mode != "test" || args.RunInTerminal || args.StartDebugging != (order == "child-request") {
				return
			}
			seq++
			write(map[string]any{"seq": seq, "type": "request", "command": "runInTerminal"})
			reverse, ok := read()
			if !ok || string(reverse["success"]) != "false" || string(reverse["request_seq"]) != fmt.Sprint(seq) {
				return
			}
			seq++
			resp["seq"] = seq
			resp["body"] = map[string]any{"supportsConfigurationDoneRequest": order == "respond-after-configuration"}
		}
		if command == "launch" && order == "child-request" {
			pendingAttach = resp
			seq++
			childRequest = seq
			write(map[string]any{"seq": seq, "type": "request", "command": "startDebugging", "arguments": map[string]any{
				"request": "attach", "configuration": map[string]any{"id": json.Number("9007199254740993")},
			}})
			continue
		}
		if command == "attach" {
			if order == "respond-before-initialized" {
				write(resp)
			} else {
				pendingAttach = resp
			}
			seq++
			write(map[string]any{"seq": seq, "type": "event", "event": "initialized"})
			continue
		}
		if command == "setBreakpoints" {
			breakpointsSet = true
			resp["body"] = map[string]any{"breakpoints": []any{map[string]any{"verified": true, "line": 9}}}
		}
		if command == "configurationDone" && (!breakpointsSet || pendingAttach == nil) {
			resp["success"] = false
		}
		write(resp)
		if command == "configurationDone" && pendingAttach != nil {
			seq++
			pendingAttach["seq"] = seq
			write(pendingAttach)
			pendingAttach = nil
		}
		if command == "disconnect" {
			return
		}
		if command == "launch" {
			seq++
			write(map[string]any{"seq": seq, "type": "event", "event": "stopped", "body": map[string]any{"reason": "entry"}})
		}
	}
}

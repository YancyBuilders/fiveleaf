package dap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAnnouncedTCPPort(t *testing.T) {
	if os.Getenv("FIVELEAF_DAP_ANNOUNCE") == "1" {
		preamble, err := io.ReadAll(os.Stdin)
		if err != nil || string(preamble) != "launcher" {
			t.Fatal("lost the launcher input")
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		fmt.Fprintf(os.Stderr, "ready on %d\n", listener.Addr().(*net.TCPAddr).Port)
		for _, stream := range []*os.File{os.Stdout, os.Stderr} {
			fmt.Fprint(stream, strings.Repeat("some adapter diagnostics\n", 36000))
		}
		conn, err := listener.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		fakeAdapter(conn, conn, "respond-after-configuration")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAnnouncedTCPPort$")
	cmd.Env = append(os.Environ(), "FIVELEAF_DAP_ANNOUNCE=1")
	cmd.Stdin = strings.NewReader("launcher")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := Start(ctx, Options{Cmd: cmd, Ready: "ready on ", InitializationOptions: json.RawMessage(`{"mode":"test"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if host, _, err := net.SplitHostPort(client.Address()); err != nil || host != "127.0.0.1" {
		t.Fatalf("announced endpoint = %q", client.Address())
	}
	client.Close()
	if cmd.ProcessState == nil {
		t.Fatal("adapter process is still running")
	}
}

func TestCancelAdapterStartupReapsProcess(t *testing.T) {
	if os.Getenv("FIVELEAF_DAP_STALL") == "1" {
		time.Sleep(time.Minute)
		return
	}
	for _, kind := range []string{"stdio", "tcp"} {
		t.Run(kind, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestCancelAdapterStartupReapsProcess$")
			cmd.Env = append(os.Environ(), "FIVELEAF_DAP_STALL=1")
			reader, writer := io.Pipe()
			defer reader.Close()
			cmd.Stdin = io.MultiReader(strings.NewReader("launcher"), reader)
			opts := Options{Cmd: cmd, Stdin: writer}
			if kind == "tcp" {
				opts.Ready = "ready on "
			}
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()
			_, err := Start(ctx, opts)
			if !errors.Is(err, context.DeadlineExceeded) || cmd.ProcessState == nil {
				t.Fatalf("canceled adapter process state %v, error %v", cmd.ProcessState, err)
			}
		})
	}
}

func TestTCPCommandMayListenAfterStart(t *testing.T) {
	clientSocket, serverSocket := net.Pipe()
	defer serverSocket.Close()
	attempts := 0
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		attempts++
		if attempts == 1 {
			return nil, syscall.ECONNREFUSED
		}
		return clientSocket, nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	conn, err := dialAdapter(ctx, Options{DialContext: dial}, "127.0.0.1:4711", make(chan struct{}))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if attempts != 2 {
		t.Fatalf("connection attempts = %d", attempts)
	}
}

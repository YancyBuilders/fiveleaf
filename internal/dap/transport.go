package dap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type adapterTransport struct {
	input   io.WriteCloser
	output  io.Reader
	close   func()
	address string
}

func openTransport(ctx context.Context, opts Options) (*adapterTransport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tcp := opts.Address != "" || opts.Ready != ""
	if opts.Cmd == nil {
		if opts.Address == "" {
			return nil, errors.New("a debug adapter needs a command or a TCP address")
		}
		conn, err := dialAdapter(ctx, opts, opts.Address, nil)
		if err != nil {
			return nil, err
		}
		return &adapterTransport{input: conn, output: conn, close: sync.OnceFunc(func() { conn.Close() }), address: opts.Address}, nil
	}
	stdin := opts.Stdin
	var err error
	if !tcp && stdin == nil {
		if stdin, err = opts.Cmd.StdinPipe(); err != nil {
			return nil, err
		}
	}
	closeInput := func() {
		if stdin != nil {
			stdin.Close()
		}
	}
	stdout, err := opts.Cmd.StdoutPipe()
	if err != nil {
		closeInput()
		return nil, err
	}
	stderr, err := opts.Cmd.StderrPipe()
	if err != nil {
		closeInput()
		stdout.Close()
		return nil, err
	}
	if err := opts.Cmd.Start(); err != nil {
		closeInput()
		return nil, err
	}
	stop := func() {
		closeInput()
		opts.Cmd.Process.Kill()
		stdout.Close()
		stderr.Close()
	}
	if !tcp {
		go readAdapterLog(stderr, opts, nil)
		return &adapterTransport{input: stdin, output: stdout, close: sync.OnceFunc(func() { stop(); opts.Cmd.Wait() })}, nil
	}
	closeInput()
	ready := make(chan string, 1)
	var logs sync.WaitGroup
	logs.Go(func() { readAdapterLog(stdout, opts, ready) })
	logs.Go(func() { readAdapterLog(stderr, opts, ready) })
	exited := make(chan struct{})
	go func() { logs.Wait(); opts.Cmd.Wait(); close(exited) }()
	address := opts.Address
	if address == "" {
		select {
		case address = <-ready:
		case <-exited:
			err = fmt.Errorf("debug adapter exited before reporting %q", opts.Ready)
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	var conn net.Conn
	if err == nil {
		conn, err = dialAdapter(ctx, opts, address, exited)
	}
	if err != nil {
		stop()
		<-exited
		return nil, err
	}
	return &adapterTransport{input: conn, output: conn, close: sync.OnceFunc(func() { conn.Close(); stop(); <-exited }), address: address}, nil
}

func readAdapterLog(stream io.Reader, opts Options, ready chan<- string) {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if opts.Handlers.Log != nil {
			opts.Handlers.Log("debug", line)
		}
		if ready != nil && opts.Ready != "" {
			if address, ok := strings.CutPrefix(line, opts.Ready); ok {
				address = strings.TrimSpace(address)
				if !strings.Contains(address, ":") {
					host := opts.Host
					if host == "" {
						host = "127.0.0.1"
					}
					address = net.JoinHostPort(host, address)
				}
				select {
				case ready <- address:
				default:
				}
			}
		}
	}
	if scanner.Err() != nil {
		io.Copy(io.Discard, stream)
	}
}

func dialAdapter(ctx context.Context, opts Options, address string, exited <-chan struct{}) (net.Conn, error) {
	_, port, err := net.SplitHostPort(address)
	value, numberErr := strconv.Atoi(port)
	if err != nil || numberErr != nil || value < 1 || value > 65535 {
		return nil, fmt.Errorf("invalid debug adapter address %q", address)
	}
	dial := opts.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	for {
		conn, err := dial(ctx, "tcp", address)
		if err == nil {
			return conn, nil
		}
		if exited == nil || !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("dialing the adapter at %s: %w", address, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-exited:
			return nil, fmt.Errorf("debug adapter exited before accepting connections at %s", address)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

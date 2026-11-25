// Package dap is a client of the Debug Adapter Protocol: it starts an
// adapter, speaks DAP with it over its standard input and output, and
// keeps what the adapter says of a session: its threads, stack, scopes,
// variables, breakpoints and output.
package dap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Request is a DAP request. Arguments hold the command's arguments.
type Request struct {
	Seq       int    `json:"seq"`
	Type      string `json:"type"`
	Command   string `json:"command"`
	Arguments any    `json:"arguments,omitempty"`
}

// Response is a DAP response to a request of Seq.
type Response struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	RequestSeq int             `json:"request_seq"`
	Success    bool            `json:"success"`
	Command    string          `json:"command"`
	Message    string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// Event is a DAP event, as stopped, continued or terminated.
type Event struct {
	Seq   int             `json:"seq"`
	Type  string          `json:"type"`
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// Handlers are what the client does with what the adapter sends unasked.
type Handlers struct {
	// Event receives an event, on the goroutine that reads the adapter.
	Event func(Event)
	// Reverse receives a request of the adapter's, and returns its body and
	// whether it succeeded. A nil handler rejects reverse requests.
	Reverse        func(command string, arguments json.RawMessage) (body any, ok bool)
	StartDebugging func(request string, configuration map[string]any) error
	// Log is a message of the adapter's stderr, or of the client's about it.
	Log func(level, msg string)
}

// Options start a client.
type Options struct {
	// Cmd runs the adapter; its standard input and output are the client's.
	Cmd *exec.Cmd
	// Stdin, when set, is what writes to Cmd's standard input, which Cmd
	// reads from a pipe of the caller's, as a command of an SSH host does.
	Stdin io.WriteCloser
	// AdapterID is the adapter's own ID, as "go", "python" or "node".
	AdapterID string
	// InitializationOptions is the adapter's initializationOptions.
	InitializationOptions json.RawMessage
	Handlers              Handlers
	// Address is the host and port of a TCP adapter, as "127.0.0.1:57726".
	// When set, the adapter's output is read for logging only.
	Address string
	// Ready is the text an address-printing adapter writes before it
	// serves, as "DAP server listening at: ".
	Ready string
	// Host is the host of Ready's address, when Ready prints only its
	// port, as "127.0.0.1".
	Host        string
	DialContext func(context.Context, string, string) (net.Conn, error)
}

// Client is a running adapter.
type Client struct {
	in      io.WriteCloser
	stop    func()
	h       Handlers
	mu      sync.Mutex
	seq     int
	done    chan struct{}
	err     error
	address string

	waiting map[int]chan Response
	writing chan struct{}
	caps    struct {
		ConfigurationDone bool `json:"supportsConfigurationDoneRequest"`
		Cancel            bool `json:"supportsCancelRequest"`
	}
	configured chan struct{}
	configOnce sync.Once
	closeOnce  sync.Once
}

// Start opens the transport and waits for the adapter's initialize response.
func Start(ctx context.Context, opts Options) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if len(opts.InitializationOptions) > 0 && !json.Valid(opts.InitializationOptions) {
		return nil, errors.New("invalid initializationOptions")
	}
	transport, err := openTransport(ctx, opts)
	if err != nil {
		return nil, err
	}
	c := &Client{
		in: transport.input, stop: transport.close, h: opts.Handlers, address: transport.address,
		done: make(chan struct{}), waiting: map[int]chan Response{}, writing: make(chan struct{}, 1), configured: make(chan struct{}),
	}
	go func() {
		readErr := c.read(bufio.NewReader(transport.output))
		c.stop()
		c.mu.Lock()
		c.err = fmt.Errorf("debug adapter disconnected: %w", readErr)
		for id, reply := range c.waiting {
			reply <- Response{RequestSeq: id, Message: c.err.Error()}
			delete(c.waiting, id)
		}
		close(c.done)
		c.mu.Unlock()
	}()
	initialize := map[string]any{
		"clientID":               "fiveleaf",
		"clientName":             "FiveLeaf",
		"adapterID":              opts.AdapterID,
		"pathFormat":             "path",
		"linesStartAt1":          true,
		"columnsStartAt1":        true,
		"supportsVariableType":   true,
		"supportsVariablePaging": true,
	}
	if len(opts.InitializationOptions) > 0 {
		initialize["initializationOptions"] = opts.InitializationOptions
	}
	if c.address != "" && c.h.StartDebugging != nil {
		initialize["supportsStartDebuggingRequest"] = true
	}
	body, err := c.Call(ctx, "initialize", initialize)
	if err == nil && len(body) > 0 {
		err = json.Unmarshal(body, &c.caps)
	}
	if err != nil {
		c.stop()
		return nil, err
	}
	return c, nil
}

// Done is closed once the adapter stopped.
func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) Address() string { return c.address }

// Configured is closed when the adapter sends its initialized event.
func (c *Client) Configured() <-chan struct{} { return c.configured }

func (c *Client) markConfigured() {
	c.configOnce.Do(func() { close(c.configured) })
}

// Err is why the adapter stopped, nil while it runs.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close asks the adapter to disconnect, and stops it after a moment.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c.Call(ctx, "disconnect", nil)
		c.in.Close()
		select {
		case <-c.done:
		case <-ctx.Done():
		}
		c.stop()
	})
}

// Call sends a request and returns its body. A response with success
// false is an error.
func (c *Client) Call(ctx context.Context, command string, arguments any) (json.RawMessage, error) {
	id, reply, err := c.request(ctx, command, arguments)
	if err != nil {
		return nil, err
	}
	select {
	case response := <-reply:
		return response.Body, response.failure(command)
	case <-ctx.Done():
		c.forget(id)
		if c.caps.Cancel && command != "cancel" {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				c.Call(ctx, "cancel", map[string]any{"requestId": id})
			}()
		}
		return nil, ctx.Err()
	}
}

func (response Response) failure(command string) error {
	if response.Success {
		return nil
	}
	if response.Message != "" {
		return errors.New(response.Message)
	}
	return fmt.Errorf("%s failed", command)
}

func (c *Client) Launch(ctx context.Context, request string, arguments any, configure func(context.Context) error) error {
	if request != "launch" && request != "attach" {
		return fmt.Errorf("unknown debug request %q", request)
	}
	id, reply, err := c.request(ctx, request, arguments)
	if err != nil {
		return err
	}
	defer c.forget(id)
	var initialized <-chan struct{}
	if configure != nil || c.caps.ConfigurationDone {
		initialized = c.Configured()
	}
	for initialized != nil || reply != nil {
		select {
		case <-initialized:
			initialized = nil
			if configure != nil {
				if err := configure(ctx); err != nil {
					return err
				}
			}
			if c.caps.ConfigurationDone {
				if _, err := c.Call(ctx, "configurationDone", nil); err != nil {
					return err
				}
			}
		case response := <-reply:
			reply = nil
			if err := response.failure(request); err != nil {
				return err
			}
		case <-c.done:
			if initialized != nil {
				return c.Err()
			}
			return (<-reply).failure(request)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *Client) request(ctx context.Context, command string, arguments any) (int, <-chan Response, error) {
	reply := make(chan Response, 1)
	id, err := c.write(ctx, reply, func(seq int) any {
		return Request{Seq: seq, Type: "request", Command: command, Arguments: arguments}
	})
	return id, reply, err
}

func (c *Client) forget(id int) {
	c.mu.Lock()
	delete(c.waiting, id)
	c.mu.Unlock()
}

func (c *Client) write(ctx context.Context, reply chan Response, makeMessage func(int) any) (int, error) {
	select {
	case c.writing <- struct{}{}:
		defer func() { <-c.writing }()
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-c.done:
		return 0, c.Err()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return 0, err
	}
	c.seq++
	id := c.seq
	if reply != nil {
		c.waiting[id] = reply
	}
	c.mu.Unlock()
	data, err := json.Marshal(makeMessage(id))
	if err == nil {
		stop := context.AfterFunc(ctx, c.stop)
		frame := append(fmt.Appendf(nil, "Content-Length: %d\r\n\r\n", len(data)), data...)
		var written int
		written, err = c.in.Write(frame)
		if err == nil && written != len(frame) {
			err = io.ErrShortWrite
		}
		if !stop() {
			err = ctx.Err()
		}
		if err != nil {
			c.stop()
		}
	}
	if err != nil {
		c.forget(id)
	}
	return id, err
}

func (c *Client) read(r *bufio.Reader) error {
	for {
		n := -1
		for {
			header, err := r.ReadSlice('\n')
			if err != nil {
				return err
			}
			line := strings.TrimSpace(string(header))
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(strings.ToLower(line), "content-length:"); ok {
				n, err = strconv.Atoi(strings.TrimSpace(v))
				if err != nil {
					return fmt.Errorf("invalid Content-Length: %w", err)
				}
			}
		}
		if n < 0 || n > 64<<20 {
			return fmt.Errorf("invalid Content-Length %d", n)
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return err
		}
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(body, &kind); err != nil {
			return fmt.Errorf("invalid DAP packet: %w", err)
		}
		switch kind.Type {
		case "response":
			var m Response
			if json.Unmarshal(body, &m) != nil {
				continue
			}
			if c.h.Log != nil && !m.Success {
				c.h.Log("debug", "response "+m.Command+" failed: "+m.Message)
			}
			c.mu.Lock()
			ch := c.waiting[m.RequestSeq]
			delete(c.waiting, m.RequestSeq)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		case "event":
			var e Event
			if json.Unmarshal(body, &e) != nil {
				continue
			}
			if e.Event == "initialized" {
				c.markConfigured()
			}
			if c.h.Event != nil {
				c.h.Event(e)
			}
		case "request":
			var req struct {
				Seq     int             `json:"seq"`
				Command string          `json:"command"`
				Args    json.RawMessage `json:"arguments"`
			}
			if json.Unmarshal(body, &req) != nil {
				continue
			}
			if req.Command == "startDebugging" && c.h.StartDebugging != nil && c.address != "" {
				go c.startDebugging(req.Seq, req.Args)
				continue
			}
			var result any
			ok := false
			if c.h.Reverse != nil {
				result, ok = c.h.Reverse(req.Command, req.Args)
			}
			var failure error
			if !ok {
				failure = fmt.Errorf("unsupported reverse request: %s", req.Command)
			}
			if err := c.reply(req.Seq, req.Command, result, failure); err != nil {
				return err
			}
		}
	}
}

func (c *Client) startDebugging(seq int, data json.RawMessage) {
	var params struct {
		Request       string         `json:"request"`
		Configuration map[string]any `json:"configuration"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err := decoder.Decode(&params)
	if err == nil {
		if (params.Request != "launch" && params.Request != "attach") || params.Configuration == nil {
			err = errors.New("startDebugging requires launch or attach and a configuration object")
		} else {
			err = c.h.StartDebugging(params.Request, params.Configuration)
		}
	}
	c.reply(seq, "startDebugging", nil, err)
}

func (c *Client) reply(id int, command string, body any, failure error) error {
	response := Response{Type: "response", RequestSeq: id, Command: command}
	if failure == nil && body != nil {
		response.Body, failure = json.Marshal(body)
	}
	response.Success = failure == nil
	if failure != nil {
		response.Message = failure.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.write(ctx, nil, func(seq int) any { response.Seq = seq; return response })
	return err
}

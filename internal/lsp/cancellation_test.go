package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

func readClientMessage(t *testing.T, reader *bufio.Reader) message {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var size int
	if _, err := fmt.Sscanf(line, "Content-Length: %d", &size); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(reader, body); err != nil {
		t.Fatal(err)
	}
	var msg message
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestCallCancellationDoesNotWaitForNotification(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { reader.Close(); writer.Close() })
	c := &Client{in: writer, waiting: map[int]chan response{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	go func() { _, err := c.Call(ctx, "test/noReply", nil); returned <- err }()
	stream := bufio.NewReader(reader)
	request := readClientMessage(t, stream)
	cancel()
	select {
	case err := <-returned:
		if err != context.Canceled {
			t.Fatalf("cancelled request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request waited for the server to read its cancellation")
	}
	if len(c.waiting) != 0 {
		t.Fatal("cancelled request leaked")
	}
	// The server resumes reading later; the cancelled call has already returned.
	notification := readClientMessage(t, stream)
	var params struct{ ID json.RawMessage }
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatal(err)
	}
	if notification.Method != "$/cancelRequest" || string(params.ID) != string(*request.ID) {
		t.Fatalf("cancellation = %+v; request = %+v", notification, request)
	}
	go func() { returned <- c.Notify("next", nil) }()
	if next := readClientMessage(t, stream); next.Method != "next" {
		t.Fatalf("next notification = %+v", next)
	}
	if err := <-returned; err != nil {
		t.Fatalf("connection after cancellation: %v", err)
	}
}

type trackedPipeWriter struct {
	*io.PipeWriter
	closed chan struct{}
	once   sync.Once
}

func (w *trackedPipeWriter) Close() error {
	err := w.PipeWriter.Close()
	w.once.Do(func() { close(w.closed) })
	return err
}

func TestCallCancellationStopsBlockedNotification(t *testing.T) {
	reader, pipeWriter := io.Pipe()
	writer := &trackedPipeWriter{PipeWriter: pipeWriter, closed: make(chan struct{})}
	t.Cleanup(func() { reader.Close(); writer.Close() })
	c := &Client{in: writer, waiting: map[int]chan response{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	go func() { _, err := c.Call(ctx, "test/noReply", nil); returned <- err }()
	readClientMessage(t, bufio.NewReader(reader))
	cancel()
	select {
	case err := <-returned:
		if err != context.Canceled {
			t.Fatalf("cancelled request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request did not return")
	}
	// A server that never reads again must not leave the notification writer
	// blocked forever. Closing an unfinished frame also prevents stream reuse.
	select {
	case <-writer.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked cancellation kept the transport open")
	}
	if err := c.Notify("next", nil); err == nil {
		t.Fatal("reused a transport with an unfinished cancellation frame")
	}
}

func TestCallCancellationFinishesStartedRequest(t *testing.T) {
	c, stream, writer := documentSyncPipe(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	requestParams := map[string]any{"value": "original"}
	go func() { _, err := c.Call(ctx, "test/backpressure", requestParams); returned <- err }()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("request write did not start")
	}
	cancel()
	awaitDocumentSync(t, returned, context.Canceled)
	requestParams["value"] = "reused after cancellation"
	select {
	case <-writer.closed:
		t.Fatal("caller cancellation closed a started request's transport")
	default:
	}
	c.mu.Lock()
	waiting := len(c.waiting)
	c.mu.Unlock()
	if waiting != 0 {
		t.Fatal("cancelled request leaked its response waiter")
	}
	// Once the server resumes reading, it must receive the complete request
	// before its cancellation. A later message can reuse the same stream.
	request := readClientMessage(t, stream)
	if request.Method != "test/backpressure" || request.ID == nil {
		t.Fatalf("first message = %+v; want cancelled request", request)
	}
	var sentParams map[string]any
	if err := json.Unmarshal(request.Params, &sentParams); err != nil {
		t.Fatal(err)
	}
	if sentParams["value"] != "original" {
		t.Fatalf("request reused its caller's parameters: %s", request.Params)
	}
	notification := readClientMessage(t, stream)
	var params struct{ ID json.RawMessage }
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		t.Fatal(err)
	}
	if notification.Method != "$/cancelRequest" || string(params.ID) != string(*request.ID) {
		t.Fatalf("cancellation = %+v; request = %+v", notification, request)
	}
	go func() { returned <- c.Notify("test/next", nil) }()
	if next := readClientMessage(t, stream); next.Method != "test/next" {
		t.Fatalf("next notification = %+v", next)
	}
	awaitDocumentSync(t, returned, nil)
}

type callParamsSnapshot struct {
	started chan struct{}
	release chan struct{}
	values  map[string]string
}

func (p *callParamsSnapshot) MarshalJSON() ([]byte, error) {
	close(p.started)
	<-p.release
	return json.Marshal(p.values)
}

func TestCallCancellationSnapshotsParametersBeforeReturning(t *testing.T) {
	c, _, _ := documentSyncPipe(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	params := &callParamsSnapshot{
		started: make(chan struct{}),
		release: make(chan struct{}),
		values:  map[string]string{"value": "original"},
	}
	defer func() {
		select {
		case <-params.release:
		default:
			close(params.release)
		}
	}()
	returned := make(chan error, 1)
	go func() { _, err := c.Call(ctx, "test/parameters", params); returned <- err }()
	select {
	case <-params.started:
	case <-time.After(time.Second):
		t.Fatal("parameter encoding did not start")
	}
	cancel()
	select {
	case err := <-returned:
		t.Fatalf("Call returned %v while a worker still owned its parameters", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(params.release)
	awaitDocumentSync(t, returned, context.Canceled)
	// Call's return transfers ownership back to its caller, including after
	// cancellation. No background worker may still read this map under -race.
	params.values["value"] = "reused after cancellation"
}

func TestCallCancellationSkipsQueuedRequest(t *testing.T) {
	c, stream, writer := documentSyncPipe(t)
	preceding := make(chan error, 1)
	go func() { preceding <- c.Notify("test/preceding", nil) }()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("preceding write did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	go func() { _, err := c.Call(ctx, "test/queued", nil); returned <- err }()
	// Wait until Call has registered its request while the preceding frame
	// still owns the writer, then cancel before that frame can finish.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(time.Second)
	for {
		c.mu.Lock()
		waiting := len(c.waiting)
		c.mu.Unlock()
		if waiting != 0 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("queued request did not register")
		}
	}
	cancel()
	awaitDocumentSync(t, returned, context.Canceled)
	select {
	case <-writer.closed:
		t.Fatal("queued request cancellation closed another writer's transport")
	default:
	}
	if first := readClientMessage(t, stream); first.Method != "test/preceding" {
		t.Fatalf("preceding notification = %+v", first)
	}
	awaitDocumentSync(t, preceding, nil)
	go func() { returned <- c.Notify("test/next", nil) }()
	if next := readClientMessage(t, stream); next.Method != "test/next" {
		t.Fatalf("cancelled queued request emitted an unexpected message: %+v", next)
	}
	awaitDocumentSync(t, returned, nil)
}

func TestCallTransportDeadlineStopsBlockedRequest(t *testing.T) {
	c, _, writer := documentSyncPipe(t)
	returned := make(chan error, 1)
	go func() { _, err := c.Call(context.Background(), "test/unresponsive", nil); returned <- err }()
	select {
	case err := <-returned:
		if err != context.DeadlineExceeded {
			t.Fatalf("blocked request: got %v, want transport deadline", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("unresponsive server retained a request write indefinitely")
	}
	select {
	case <-writer.closed:
	default:
		t.Fatal("transport deadline left an incomplete request frame open")
	}
	c.mu.Lock()
	waiting := len(c.waiting)
	c.mu.Unlock()
	if waiting != 0 {
		t.Fatal("timed-out request leaked its response waiter")
	}
}

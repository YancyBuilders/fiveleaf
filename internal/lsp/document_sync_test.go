package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

type documentSyncWriter struct {
	*trackedPipeWriter
	started chan struct{}
	once    sync.Once
}

func (w *documentSyncWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	return w.PipeWriter.Write(data)
}

func documentSyncPipe(t *testing.T) (*Client, *bufio.Reader, *documentSyncWriter) {
	t.Helper()
	reader, pipeWriter := io.Pipe()
	writer := &documentSyncWriter{
		trackedPipeWriter: &trackedPipeWriter{PipeWriter: pipeWriter, closed: make(chan struct{})},
		started:           make(chan struct{}),
	}
	// Bound reads as well as result waits so a missing notification reports a
	// local test failure instead of waiting for the package timeout.
	timer := time.AfterFunc(10*time.Second, func() { reader.Close(); writer.Close() })
	t.Cleanup(func() { timer.Stop(); reader.Close(); writer.Close() })
	return &Client{in: writer, waiting: map[int]chan response{}}, bufio.NewReader(reader), writer
}

func awaitDocumentSync(t *testing.T, result <-chan error, want error) {
	t.Helper()
	select {
	case err := <-result:
		if err != want {
			t.Fatalf("document sync: got %v, want %v", err, want)
		}
	case <-time.After(time.Second):
		t.Fatal("document sync did not return")
	}
}

func checkDocumentSyncMessage(t *testing.T, msg message, method, uri string, version int, text string) {
	t.Helper()
	var params struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
			Text    string `json:"text"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatal(err)
	}
	gotText := params.TextDocument.Text
	if msg.Method == "textDocument/didChange" && len(params.ContentChanges) == 1 {
		gotText = params.ContentChanges[0].Text
	}
	if msg.Method != method || params.TextDocument.URI != uri || params.TextDocument.Version != version || gotText != text {
		t.Fatalf("notification = %s %s; want %s uri=%q version=%d text=%q", msg.Method, msg.Params, method, uri, version, text)
	}
}

func TestOpenAsyncCancellationFinishesStartedWrite(t *testing.T) {
	c, stream, writer := documentSyncPipe(t)
	const uri = "file:///blocked.go"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan (<-chan error), 1)
	go func() { returned <- c.OpenAsync(ctx, uri, "go", "first") }()
	var result <-chan error
	select {
	case result = <-returned:
	case <-time.After(time.Second):
		t.Fatal("OpenAsync waited for the server to read")
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("document write did not start")
	}
	select {
	case err := <-result:
		t.Fatalf("unread document unexpectedly finished: %v", err)
	default:
	}
	cancel()
	awaitDocumentSync(t, result, context.Canceled)
	select {
	case <-writer.closed:
		t.Fatal("caller cancellation closed a started document's transport")
	default:
	}
	if c.IsOpen(uri) {
		t.Fatal("unread document committed an open version")
	}
	// Save and close must also return while the server is not reading. The
	// started write retains its slot and version even though its caller left.
	queued := make(chan []<-chan error, 1)
	go func() {
		queued <- []<-chan error{
			c.OpenAsync(context.Background(), uri, "go", "second"),
			c.SavedAsync(context.Background(), uri),
			c.ClosedAsync(context.Background(), uri),
			c.OpenAsync(context.Background(), uri, "go", "reopened"),
		}
	}()
	var results []<-chan error
	select {
	case results = <-queued:
	case <-time.After(time.Second):
		t.Fatal("document lifecycle call waited for the preceding write")
	}
	for _, want := range []struct {
		method  string
		version int
		text    string
	}{
		{"textDocument/didOpen", 1, "first"},
		{"textDocument/didChange", 2, "second"},
		{"textDocument/didSave", 0, ""},
		{"textDocument/didClose", 0, ""},
		{"textDocument/didOpen", 1, "reopened"},
	} {
		checkDocumentSyncMessage(t, readClientMessage(t, stream), want.method, uri, want.version, want.text)
	}
	for _, result := range results {
		awaitDocumentSync(t, result, nil)
	}
	if !c.IsOpen(uri) {
		t.Fatal("reopened document missing from client state")
	}
}

func TestOpenAsyncTransportDeadlineStopsBlockedWrite(t *testing.T) {
	c, _, writer := documentSyncPipe(t)
	const uri = "file:///unresponsive.go"
	result := c.OpenAsync(context.Background(), uri, "go", "unread")
	select {
	case err := <-result:
		if err != context.DeadlineExceeded {
			t.Fatalf("blocked document sync: got %v, want transport deadline", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("unresponsive server retained a document write indefinitely")
	}
	select {
	case <-writer.closed:
	default:
		t.Fatal("transport deadline left an incomplete frame open")
	}
	if c.IsOpen(uri) {
		t.Fatal("timed-out document write committed an open version")
	}
}

func TestDocumentSyncPreservesOpenSaveCloseOrder(t *testing.T) {
	c, stream, _ := documentSyncPipe(t)
	const uri = "file:///ordered.go"
	first := c.OpenAsync(context.Background(), uri, "go", "first")
	second := c.OpenAsync(context.Background(), uri, "go", "second")
	done := make(chan error, 1)
	go func() {
		c.Open(uri, "go", "third")
		c.Saved(uri)
		c.Closed(uri)
		done <- <-c.OpenAsync(context.Background(), uri, "go", "reopened")
	}()
	for _, want := range []struct {
		method  string
		version int
		text    string
	}{
		{"textDocument/didOpen", 1, "first"},
		{"textDocument/didChange", 2, "second"},
		{"textDocument/didChange", 3, "third"},
		{"textDocument/didSave", 0, ""},
		{"textDocument/didClose", 0, ""},
		{"textDocument/didOpen", 1, "reopened"},
	} {
		checkDocumentSyncMessage(t, readClientMessage(t, stream), want.method, uri, want.version, want.text)
	}
	awaitDocumentSync(t, first, nil)
	awaitDocumentSync(t, second, nil)
	awaitDocumentSync(t, done, nil)
	if !c.IsOpen(uri) {
		t.Fatal("reopened document missing from client state")
	}
}

func TestOpenAsyncQueuedCancellationKeepsTransportAndVersion(t *testing.T) {
	c, stream, writer := documentSyncPipe(t)
	const precedingURI = "file:///preceding.go"
	const uri = "file:///queued.go"
	preceding := c.OpenAsync(context.Background(), precedingURI, "go", "preceding")
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("preceding document write did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelled := c.OpenAsync(ctx, uri, "go", "stale")
	fresh := c.OpenAsync(context.Background(), uri, "go", "fresh")
	cancel()
	// Cancellation must return while the preceding server write is blocked.
	awaitDocumentSync(t, cancelled, context.Canceled)
	select {
	case <-writer.closed:
		t.Fatal("queued cancellation closed another document's transport")
	default:
	}
	if c.IsOpen(uri) {
		t.Fatal("cancelled queued document committed an open version")
	}
	checkDocumentSyncMessage(t, readClientMessage(t, stream), "textDocument/didOpen", precedingURI, 1, "preceding")
	checkDocumentSyncMessage(t, readClientMessage(t, stream), "textDocument/didOpen", uri, 1, "fresh")
	awaitDocumentSync(t, preceding, nil)
	awaitDocumentSync(t, fresh, nil)
	changed := c.OpenAsync(context.Background(), uri, "go", "changed")
	checkDocumentSyncMessage(t, readClientMessage(t, stream), "textDocument/didChange", uri, 2, "changed")
	awaitDocumentSync(t, changed, nil)
}

func TestDocumentSyncQueuedLifecycleCancellationKeepsDocumentOpen(t *testing.T) {
	c, stream, writer := documentSyncPipe(t)
	const uri = "file:///lifecycle.go"
	first := c.OpenAsync(context.Background(), uri, "go", "first")
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("preceding document write did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	saved := c.SavedAsync(ctx, uri)
	closed := c.ClosedAsync(ctx, uri)
	changed := c.OpenAsync(context.Background(), uri, "go", "changed")
	cancel()
	awaitDocumentSync(t, saved, context.Canceled)
	awaitDocumentSync(t, closed, context.Canceled)
	checkDocumentSyncMessage(t, readClientMessage(t, stream), "textDocument/didOpen", uri, 1, "first")
	checkDocumentSyncMessage(t, readClientMessage(t, stream), "textDocument/didChange", uri, 2, "changed")
	awaitDocumentSync(t, first, nil)
	awaitDocumentSync(t, changed, nil)
	if !c.IsOpen(uri) {
		t.Fatal("cancelled queued close removed the open document")
	}
}

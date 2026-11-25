package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/lsp"
)

type lspDocumentMessage struct {
	Method string `json:"method"`
	Params struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
			Text    string `json:"text"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	} `json:"params"`
}

// The client writes each header and JSON body separately. Hold its next write
// before any bytes reach the server, then record the completed JSON bodies.
type lspDocumentWriter struct {
	io.WriteCloser
	mu       sync.Mutex
	gate     <-chan struct{}
	entered  chan struct{}
	messages []lspDocumentMessage
	closed   chan struct{}
	once     sync.Once
}

func (w *lspDocumentWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	gate, entered := w.gate, w.entered
	w.gate = nil
	w.mu.Unlock()
	if gate != nil {
		close(entered)
		select {
		case <-gate:
		case <-w.closed:
			return 0, io.ErrClosedPipe
		}
	}
	n, err := w.WriteCloser.Write(p)
	if err == nil && n == len(p) {
		var msg lspDocumentMessage
		if json.Unmarshal(p, &msg) == nil && msg.Method != "" {
			w.mu.Lock()
			w.messages = append(w.messages, msg)
			w.mu.Unlock()
		}
	}
	return n, err
}

func (w *lspDocumentWriter) Close() error {
	w.once.Do(func() { close(w.closed) })
	return w.WriteCloser.Close()
}

func TestLSPDocumentEventsDoNotBlockUI(t *testing.T) {
	t.Setenv("FIVELEAF_FAKE_LSP", "1")
	hs := newHarness(t, false)
	withPlugins(hs, map[string]map[string]string{"fake": {"plugin.json": `{
		"name":"fake", "contributes":{
			"languages":[{"id":"fake","extensions":[".fake"]}],
			"languageServers":[{"id":"fake.server","languages":["fake"],"command":"unused-test-server"}]
		}
	}`}})
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	w := &lspDocumentWriter{WriteCloser: stdin, closed: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := lsp.Start(ctx, lsp.Options{Cmd: cmd, Stdin: w, RootURI: lsp.URI(hs.root)})
	if err != nil {
		w.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close(); client.Close() })
	p, sc := serverContrib("fake")
	server := newLSPServer(p, *sc, hs.root, 1)
	server.client = client
	close(server.ready)
	key := hs.root + "\x00" + sc.ID
	lspServers.Lock()
	lspServers.m[key] = server
	lspServers.Unlock()
	t.Cleanup(func() {
		server.stop()
		lspServers.Lock()
		delete(lspServers.m, key)
		lspServers.Unlock()
	})
	path := hs.path("active.fake")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	active := hs.a.activeEditor().document
	inactive := &document{path: hs.path("inactive.fake"), buf: ui.NewTextBuffer("inactive")}
	fresh := &document{path: hs.path("fresh.fake"), buf: ui.NewTextBuffer("fresh")}
	// This completion is also a barrier for the active editor's initial open.
	if err := <-client.OpenAsync(ctx, serverURI(inactive.path), "fake", inactive.buf.String()); err != nil {
		t.Fatal(err)
	}
	gate, entered := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	defer release()
	w.mu.Lock()
	w.gate, w.entered, w.messages = gate, entered, nil
	w.mu.Unlock()
	blocked := client.OpenAsync(ctx, serverURI(active.path), "fake", "blocked snapshot")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("document write did not reach the controlled backpressure")
	}
	for _, event := range []struct {
		name string
		doc  *document
	}{
		{"close", inactive},
		{"save", active},
		{"change", active},
		{"open", fresh},
	} {
		if event.name == "change" {
			active.buf.Set("changed snapshot")
		}
		returned := make(chan struct{})
		go func() { hs.a.lspDoc(event.name, event.doc); close(returned) }()
		select {
		case <-returned:
		case <-time.After(time.Second):
			release()
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("document event stayed blocked after releasing the writer")
			}
			t.Fatalf("lspDoc(%q) blocked the UI behind another document write", event.name)
		}
	}
	select {
	case err := <-blocked:
		t.Fatalf("held document write completed before release: %v", err)
	default:
	}
	barrierURI := serverURI(hs.path("barrier.fake"))
	flushed := client.OpenAsync(ctx, barrierURI, "fake", "barrier")
	release()
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	messages := append([]lspDocumentMessage(nil), w.messages...)
	w.mu.Unlock()
	want := []struct {
		method, uri, text string
		version           int
	}{
		{"textDocument/didChange", serverURI(active.path), "blocked snapshot", 2},
		{"textDocument/didClose", serverURI(inactive.path), "", 0},
		{"textDocument/didSave", serverURI(active.path), "", 0},
		{"textDocument/didChange", serverURI(active.path), "changed snapshot", 3},
		{"textDocument/didOpen", serverURI(fresh.path), "fresh", 1},
		{"textDocument/didOpen", barrierURI, "barrier", 1},
	}
	if len(messages) != len(want) {
		t.Fatalf("document messages = %+v; want %d", messages, len(want))
	}
	for i, expected := range want {
		got := messages[i]
		doc, text := got.Params.TextDocument, got.Params.TextDocument.Text
		if len(got.Params.ContentChanges) == 1 {
			text = got.Params.ContentChanges[0].Text
		}
		if got.Method != expected.method || doc.URI != expected.uri || doc.Version != expected.version || text != expected.text {
			t.Fatalf("document message %d = %+v; want %+v", i, got, expected)
		}
	}
}

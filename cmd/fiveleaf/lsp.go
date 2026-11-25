package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/lsp"
	"fiveleaf/internal/plugin"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/vfs"
	"fiveleaf/locales"
)

// The language servers plugins declare: started for a folder as a
// document of one of their languages opens there, on the folder's machine,
// and told of the documents of their languages as they open, change, save
// and close.

// lspServer is a language server of a folder.
type lspServer struct {
	plugin    *plugin.Plugin
	contrib   plugin.ServerContrib
	root      string
	client    *lsp.Client
	ready     chan struct{} // closed once started, or failed to
	err       error
	failedAt  time.Time // when it could not start
	starts    int       // since its plugin was last loaded or turned on
	host      *pluginHost
	owner     string // diagnostics belong to this start, not a later restart
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   atomic.Bool // an explicit stop suppresses automatic retries
	closeOnce sync.Once
	postsMu   sync.Mutex
	posts     []func()
	posting   bool
}

// lspState is where a language server is in its life.
type lspState int

const (
	lspStarting lspState = iota // its command starts, and it is greeted
	lspRunning                  // client talks to it
	lspFailed                   // it could not start, as err says
	lspStopped                  // explicitly stopped, or its command ended
)

// state works out the server's state from ready and its client, which
// start writes before it closes ready.
func (s *lspServer) state() lspState {
	if s.stopped.Load() {
		return lspStopped
	}
	select {
	case <-s.ready:
	default:
		return lspStarting
	}
	if s.client == nil {
		return lspFailed
	}
	select {
	case <-s.client.Done():
		return lspStopped
	default:
		return lspRunning
	}
}

var lspServers = struct {
	sync.Mutex
	m map[string]*lspServer // by folder and server id
}{m: map[string]*lspServer{}}

var lspGeneration atomic.Uint64

type lspServerStatus struct {
	ID    string
	State lspState
	Err   string
}

func (a *app) lspStatuses() []lspServerStatus {
	lspServers.Lock()
	defer lspServers.Unlock()
	var out []lspServerStatus
	for _, s := range lspServers.m {
		if s.root != a.root {
			continue
		}
		status := lspServerStatus{ID: s.contrib.ID, State: s.state()}
		select {
		case <-s.ready:
			if s.err != nil {
				status.Err = s.err.Error()
			}
		default:
		}
		out = append(out, status)
	}
	slices.SortFunc(out, func(a, b lspServerStatus) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func newLSPServer(p *plugin.Plugin, sc plugin.ServerContrib, root string, starts int) *lspServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &lspServer{
		plugin: p, contrib: sc, root: root, ready: make(chan struct{}), starts: starts,
		host: plugins, owner: fmt.Sprintf("lsp:%s\x00%s\x00%d", root, sc.ID, lspGeneration.Add(1)),
		ctx: ctx, cancel: cancel,
	}
}

func (s *lspServer) current() bool {
	lspServers.Lock()
	defer lspServers.Unlock()
	return !s.stopped.Load() && lspServers.m[s.root+"\x00"+s.contrib.ID] == s
}

func (s *lspServer) stop() {
	s.stopped.Store(true)
	if s.cancel != nil {
		s.cancel()
	}
}

// close waits off the UI thread, including for an interrupted startup.
func (s *lspServer) close() {
	s.stop()
	<-s.ready
	s.closeOnce.Do(func() {
		if s.client != nil {
			s.client.Close()
		}
	})
}

// enqueue keeps UI calls and plugin logging in order without holding up
// the protocol reader or a shutdown waiting for startup to finish.
func (s *lspServer) enqueue(fn func()) {
	s.postsMu.Lock()
	s.posts = append(s.posts, fn)
	if s.posting {
		s.postsMu.Unlock()
		return
	}
	s.posting = true
	s.postsMu.Unlock()
	go func() {
		for {
			s.postsMu.Lock()
			batch := s.posts
			s.posts = nil
			if len(batch) == 0 {
				s.posting = false
				s.postsMu.Unlock()
				return
			}
			s.postsMu.Unlock()
			for _, post := range batch {
				post()
			}
		}
	}()
}

func (s *lspServer) log(level, message string) {
	s.enqueue(func() { s.plugin.Log(level, message) })
}

func (s *lspServer) post(fn func()) {
	if h := s.host; h != nil {
		s.enqueue(func() { h.onMain(fn) })
	}
}

func (s *lspServer) changed() {
	if h := s.host; h != nil {
		s.post(func() {
			for _, a := range h.apps() {
				if a.root == s.root {
					a.invalidate()
				}
			}
		})
	}
}

func (s *lspServer) clearDiagnostics() {
	if h := s.host; h != nil {
		s.post(func() { h.setDiagnosticList(s.owner, "", "", true, nil) })
	}
}

func (a *app) stopLanguageServers(ids ...string) {
	lspServers.Lock()
	var servers []*lspServer
	for _, s := range lspServers.m {
		if s.root == a.root && (len(ids) == 0 || slices.Contains(ids, s.contrib.ID)) {
			s.stop()
			servers = append(servers, s)
		}
	}
	lspServers.Unlock()
	for _, s := range servers {
		s.clearDiagnostics()
		s.changed()
		go s.close()
	}
	a.invalidate()
}

func (a *app) restartLanguageServers(ids ...string) bool {
	if plugins == nil {
		return false
	}
	enabled := plugins.enabled()
	lspServers.Lock()
	var restarts []struct{ old, next *lspServer }
	for key, old := range lspServers.m {
		if old.root != a.root || old.plugin == nil || len(ids) > 0 && !slices.Contains(ids, old.contrib.ID) {
			continue
		}
		for _, p := range enabled {
			if p.Name != old.plugin.Name {
				continue
			}
			for _, sc := range p.Contributes.LanguageServers {
				if sc.ID != old.contrib.ID {
					continue
				}
				old.stop()
				s := newLSPServer(p, sc, old.root, 1)
				lspServers.m[key] = s
				restarts = append(restarts, struct{ old, next *lspServer }{old, s})
				break
			}
			break
		}
	}
	lspServers.Unlock()
	for _, restart := range restarts {
		restart.old.clearDiagnostics()
		restart.next.changed()
		go func() {
			restart.old.close()
			restart.next.start(a)
		}()
	}
	a.invalidate()
	return len(restarts) > 0
}

// maxServerStarts is how many times a server is started, after it stops
// or fails to start, until its plugin is reloaded or turned on again.
const maxServerStarts = 3

// lspRetryPause is how long after a server fails to start a document of
// its language opening starts it again.
var lspRetryPause = 30 * time.Second

// serverContrib is the server a plugin declares for a language, nil for
// none.
func serverContrib(lang string) (*plugin.Plugin, *plugin.ServerContrib) {
	if plugins == nil || lang == "" {
		return nil, nil
	}
	for _, p := range plugins.enabled() {
		for i, s := range p.Contributes.LanguageServers {
			if slices.Contains(s.Languages, lang) {
				return p, &p.Contributes.LanguageServers[i]
			}
		}
	}
	return nil, nil
}

// serverURI is the URI a server of root knows a path by: on an SSH host,
// the path there.
func serverURI(path string) string {
	if _, rp, ok := remote.Split(path); ok {
		return lsp.URI(rp)
	}
	return lsp.URI(path)
}

// serverPath is the path of a URI of a server of root.
func serverPath(root, uri string) string {
	p := lsp.Path(uri)
	if p == "" {
		return ""
	}
	if host, _, ok := remote.Split(root); ok {
		return remote.Path(host, p)
	}
	return p
}

// lspFor is the server of a document's language in the folder, started
// in the background the first time; nil when no plugin declares one.
// opening is whether the document is opening, which starts again one that
// failed to start a while ago.
func (a *app) lspFor(d *document, opening bool) *lspServer {
	if a.root == "" || d.path == "" {
		return nil
	}
	p, sc := serverContrib(languageOf(d))
	if p == nil {
		return nil
	}
	key := a.root + "\x00" + sc.ID
	lspServers.Lock()
	defer lspServers.Unlock()
	s := lspServers.m[key]
	// One that stopped is started again, a few times, and one that could
	// not start as well, as a document opens a while after.
	if s != nil {
		switch st := s.state(); {
		case s.stopped.Load(), s.starts >= maxServerStarts, st == lspStarting, st == lspRunning:
			return s
		case st == lspFailed && (!opening || time.Since(s.failedAt) < lspRetryPause):
			return s
		}
	}
	starts := 0
	if s != nil {
		starts = s.starts
	}
	s = newLSPServer(p, *sc, a.root, starts+1)
	lspServers.m[key] = s
	go s.start(a)
	return s
}

// start starts the server on the folder's machine.
func (s *lspServer) start(a *app) {
	defer func() {
		// One that could not start is told of once, in the window that
		// started it.
		if s.client == nil && !s.stopped.Load() {
			s.failedAt = time.Now()
			msg := locales.LspFailed(firstLine(s.err.Error()))
			a.post(func() {
				if a.root == s.root && s.current() {
					a.fail("%s", msg)
				}
			})
		}
		close(s.ready)
		if s.client != nil {
			s.openDocuments()
		}
		s.changed()
	}()
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	defer s.cancel()
	if s.stopped.Load() {
		return
	}
	launch, err := s.resolve(ctx)
	if err != nil {
		s.err = err
		s.log("error", "could not resolve "+s.contrib.ID+": "+err.Error())
		return
	}
	pr, pw := io.Pipe()
	cmd, err := vfs.CommandEnvironment(s.root, launch.environment(), launch.UnsetEnv, pr, launch.Command, launch.Args...)
	if err != nil {
		pr.Close()
		pw.Close()
		s.err = err
		return
	}
	root := s.root
	rootURI := serverURI(root)
	if len(launch.WorkspaceFolders) > 0 {
		rootURI = launch.WorkspaceFolders[0].URI
	}
	c, err := lsp.Start(ctx, lsp.Options{Cmd: cmd, Stdin: pw, RootURI: rootURI, InitOptions: launch.Options,
		Configuration: launch.Configuration, WorkspaceFolders: launch.WorkspaceFolders, PathMappings: launch.PathMappings,
		Handlers: lsp.Handlers{
			Diagnostics: func(uri string, items []lsp.Diagnostic) {
				path := serverPath(root, uri)
				if path == "" || s.host == nil {
					return
				}
				list := make([]diagnostic, len(items))
				for i, it := range items {
					sev := []string{"error", "error", "warning", "info", "hint"}[min(max(it.Severity, 0), 4)]
					code := strings.Trim(string(it.Code), `"`)
					list[i] = diagnostic{Range: textRange{fromLSP(it.Range.Start), fromLSP(it.Range.End)}, Severity: sev, Message: it.Message, Code: code}
				}
				source := s.contrib.ID
				if len(items) > 0 && items[0].Source != "" {
					source = items[0].Source
				}
				s.post(func() {
					if s.current() {
						s.host.setDiagnosticList(s.owner, path, source, false, list)
					}
				})
			},
			Log: func(level, msg string) { s.log(level, s.contrib.ID+": "+msg) },
		}})
	if err != nil {
		pr.Close()
		pw.Close()
		s.err = fmt.Errorf("%s: %v", launch.Command, err)
		s.log("error", "could not start "+s.contrib.ID+": "+err.Error())
		return
	}
	s.client = c
	s.log("info", "started "+s.contrib.ID)
	go func() {
		<-c.Done()
		s.log("warn", s.contrib.ID+" stopped")
		s.clearDiagnostics()
		s.changed()
	}()
}

// openDocuments runs after ready publishes the client, using the windows
// still showing this workspace when the main thread receives the result.
func (s *lspServer) openDocuments() {
	s.post(func() {
		c := s.live()
		if !s.current() || c == nil {
			return
		}
		for _, a := range s.host.apps() {
			if a.root != s.root {
				continue
			}
			for _, d := range a.docs {
				if d.path != "" && slices.Contains(s.contrib.Languages, languageOf(d)) {
					c.OpenAsync(context.Background(), serverURI(d.path), languageOf(d), d.buf.String())
				}
			}
		}
	})
}

// forgetFailedServers forgets the servers of the plugins of names, or of
// every plugin for none, that could not start, so that a document of their
// languages opening starts them afresh.
func forgetFailedServers(names ...string) {
	lspServers.Lock()
	defer lspServers.Unlock()
	for key, s := range lspServers.m {
		if s.state() == lspFailed && (len(names) == 0 || slices.Contains(names, s.plugin.Name)) {
			delete(lspServers.m, key)
		}
	}
}

// startServers starts the servers of the documents open, those that
// failed forgotten.
func (h *pluginHost) startServers() {
	for _, a := range h.apps() {
		for _, d := range a.docs {
			a.lspFor(d, true)
		}
	}
}

// fromLSP is a position of LSP's as plugins have it, the same.
func fromLSP(p lsp.Position) position { return position(p) }

// client is the server's client once it runs, nil before or after.
func (s *lspServer) live() *lsp.Client {
	if s == nil || s.state() != lspRunning {
		return nil
	}
	return s.client
}

// lspDoc tells a document's server what happened to it: "open", "change",
// "save" or "close".
func (a *app) lspDoc(event string, d *document) {
	if event == "close" {
		lspServers.Lock()
		servers := make([]*lspServer, 0, len(lspServers.m))
		for _, s := range lspServers.m {
			servers = append(servers, s)
		}
		lspServers.Unlock()
		for _, s := range servers {
			if c := s.live(); c != nil {
				c.ClosedAsync(context.Background(), serverURI(d.path))
			}
		}
		return
	}
	s := a.lspFor(d, event == "open")
	c := s.live()
	if c == nil {
		return // the server opens it as it starts
	}
	uri := serverURI(d.path)
	switch event {
	case "save":
		c.SavedAsync(context.Background(), uri)
	default:
		c.OpenAsync(context.Background(), uri, languageOf(d), d.buf.String())
	}
}

// stopServers stops every language server, as the app quits.
func stopServers() {
	lspServers.Lock()
	servers := make([]*lspServer, 0, len(lspServers.m))
	for _, s := range lspServers.m {
		s.stop()
		servers = append(servers, s)
	}
	lspServers.m = map[string]*lspServer{}
	lspServers.Unlock()
	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.close()
		}()
	}
	wg.Wait()
}

// caretPosition is where the caret of an editor is, as LSP has it. In a
// modal mode the selection's active end is the cursor, which is not
// always the end of the selection the text area reports.
func caretPosition(d *editor) position {
	snap := d.buf.Snapshot()
	at := min(d.sel[1], snap.Len())
	if d.vim.mode.visual() {
		at = vimCursor(d, snap)
	} else if d.helix.started && d.helix.mode != helixInsert {
		at = helixCursor(d, snap)
	}
	return positionOf(snap, at)
}

// lspNavigation binds a semantic query to the current document and view.
// Waiting for a server or a reply must not navigate after either changes
// or a newer query starts.
type lspNavigation struct {
	doc     *document
	pos     position
	tab     *colTab
	root    string
	path    string
	version uint64
	serial  uint64
	valid   func() bool
}

func (a *app) navigationAt(d *document, pos position) lspNavigation {
	a.cancelDefinition()
	a.dismissHover()
	a.pointerHover.suppressed = true
	a.navigationSerial++
	return lspNavigation{doc: d, pos: pos, tab: a.activeTab(), root: a.root, path: d.path, version: d.buf.Version(), serial: a.navigationSerial}
}

// navigationCurrent runs on the UI thread, as document state belongs to it.
func (a *app) navigationCurrent(n lspNavigation) bool {
	if a.navigationSerial != n.serial || a.root != n.root || a.activeTab() != n.tab {
		return false
	}
	if n.doc.path != n.path || n.doc.buf.Version() != n.version || n.doc.locked() ||
		!slices.Contains(a.docs, n.doc) || n.valid != nil && !n.valid() {
		if n.valid != nil {
			a.tell(note{message: locales.DiffNavigationUnavailable()})
		}
		return false
	}
	return true
}

// prepareNavigation waits off the UI thread, then lets the query capture the
// current buffer on the UI thread. A diff's historical text is never sent as
// the contents of the working file's URI.
func (a *app) prepareNavigation(ctx context.Context, n lspNavigation, s *lspServer, wait time.Duration, run func(*lsp.Client)) {
	go func() {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-s.ready:
		case <-timer.C:
		}
		a.post(func() {
			if ctx.Err() != nil || !a.navigationCurrent(n) {
				return
			}
			run(s.live())
		})
	}()
}

// showHover asks the server and the plugins of the document's language
// what is at the caret, and shows it.
func (a *app) showHover() {
	if tab := a.activeTab(); tab != nil && tab.diff != nil {
		a.diffNavigateSelection(tab.diff, true)
		return
	}
	d := a.textEditor()
	if d == nil {
		return
	}
	a.queryHover(a.navigationAt(d.document, caretPosition(d)))
}

func (a *app) queryHover(n lspNavigation) {
	a.startHover(n, false, a.navigationAnchor(n))
}

// goToDefinition goes to where what is at the caret is defined, as the
// document's language server says.
func (a *app) goToDefinition() {
	if tab := a.activeTab(); tab != nil && tab.diff != nil {
		a.diffNavigateSelection(tab.diff, false)
		return
	}
	d := a.textEditor()
	if d == nil {
		return
	}
	a.queryDefinition(a.navigationAt(d.document, caretPosition(d)))
}

// definitionAtClick goes to the definition of what a Cmd-click at x, y in
// the text area is on, as a Cmd-click in a code editor does.
func (a *app) definitionAtClick(d *editor, x, y float32) {
	if tab := a.activeTab(); tab == nil || tab.diff != nil || tab.ed != d {
		return
	}
	if hit, ok := a.semanticPointAt(d, x, y); ok {
		a.queryDefinition(a.navigationAt(d.document, positionOf(d.buf.Snapshot(), hit.at)))
	}
}

func (a *app) queryDefinition(n lspNavigation) {
	a.dismissHover()
	s := a.lspFor(n.doc, false)
	if s == nil {
		a.tell(note{message: locales.LspNone(languageOf(n.doc))})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	a.navigationCancel = cancel
	a.definitionPending = &n
	a.invalidate()
	uri := serverURI(n.path)
	a.prepareNavigation(ctx, n, s, 3*time.Second, func(c *lsp.Client) {
		if c == nil || !c.Definition() {
			cancel()
			a.definitionPending = nil
			if c != nil {
				a.tell(note{message: locales.DefinitionNone()})
			} else {
				a.lspFailed(s)
			}
			return
		}
		synced := c.OpenAsync(ctx, uri, languageOf(n.doc), n.doc.buf.String())
		go func() {
			defer cancel()
			var locs []lsp.Location
			err := <-synced
			if err == nil {
				locs, err = c.DefinitionAt(ctx, uri, lsp.Position(n.pos))
			}
			a.post(func() {
				if a.definitionPending != &n {
					return
				}
				if !a.navigationCurrent(n) || a.blurred || a.semanticOverlayOpen() {
					a.cancelDefinition()
					return
				}
				a.definitionPending = nil
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						a.tell(note{message: locales.DefinitionFailed(err.Error())})
					}
					return
				}
				a.presentDefinitions(n, locs)
			})
		}()
	})
}

// lspFailed tells why a language server did not answer: one that could
// not start was told of as it failed.
func (a *app) lspFailed(s *lspServer) {
	if s != nil && (s.state() == lspFailed || s.state() == lspStopped) {
		a.tell(note{message: locales.LspDown()})
		return
	}
	a.tell(note{message: locales.LspNotReady()})
}

func sortKey(it lsp.CompletionItem) string {
	if it.SortText != "" {
		return it.SortText
	}
	return it.Label
}

// applyCompletion puts a completion in place: its edit, or its text in
// place of the word before the caret.
func applyCompletion(d *editor, it lsp.CompletionItem) {
	snap := d.buf.Snapshot()
	text := it.InsertText
	if text == "" {
		text = it.Label
	}
	var s, e int
	if it.TextEdit != nil {
		s, e = posRune(snap, fromLSP(it.TextEdit.Range.Start)), posRune(snap, fromLSP(it.TextEdit.Range.End))
		text = it.TextEdit.NewText
	} else {
		e = min(d.sel[1], snap.Len())
		prefix := completionPrefix(snap, e)
		s = e - utf8.RuneCountInString(prefix)
		if rest, ok := completionRemainder(text, prefix); ok {
			after := snap.Slice(e, min(snap.Len(), e+utf8.RuneCountInString(rest)))
			for _, char := range after {
				next, size := utf8.DecodeRuneInString(rest)
				if !isWordRune(char) || unicode.ToLower(char) != unicode.ToLower(next) {
					break
				}
				e++
				rest = rest[size:]
			}
		}
	}
	at := s + utf8.RuneCountInString(text)
	selection := [2]int{at, at}
	if it.InsertTextFormat == 2 {
		text, selection = snippetReplacement(d, s, text, nil)
	}
	edits := []textEdit{{s, e, text}}
	for _, extra := range it.AdditionalTextEdits {
		start, end := posRune(snap, fromLSP(extra.Range.Start)), posRune(snap, fromLSP(extra.Range.End))
		if start > end || start == s || (start < e && end > s) {
			return
		}
		edits = append(edits, textEdit{start, end, extra.NewText})
		if end <= s {
			shift := utf8.RuneCountInString(extra.NewText) - (end - start)
			selection[0] += shift
			selection[1] += shift
		}
	}
	if !editsOverlap(edits) && s <= e {
		d.edit(selection, edits...)
	}
}

// completionIcon is the icon of a kind of completion.
func completionIcon(kind int) string {
	switch kind {
	case 2, 3, 4: // method, function, constructor
		return "function"
	case 5, 6, 10: // field, variable, property
		return "hash"
	case 7, 8, 22: // class, interface, struct
		return "box"
	case 9: // module
		return "package"
	case 14: // keyword
		return "keyboard"
	case 15: // snippet
		return "braces"
	case 17: // file
		return "file"
	case 19: // folder
		return "folder"
	}
	return "code"
}

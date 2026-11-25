package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/lsp"
)

func sourceNavigationHarness(t *testing.T) (*harness, *editor, *lsp.Client) {
	t.Helper()
	t.Setenv("FIVELEAF_FAKE_LSP", "1")
	hs := newHarness(t, false)
	manifest, err := json.Marshal(map[string]any{
		"name": "fake",
		"contributes": map[string]any{
			"languages": []any{map[string]any{"id": "fake", "extensions": []string{".fake"}}},
			"languageServers": []any{map[string]any{
				"id": "fake.server", "languages": []string{"fake"}, "command": os.Args[0], "args": []string{"-test.run=^$"},
				"initializationOptions": map[string]any{"holdDefinitions": true, "holdHovers": true, "holdCompletions": true},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	withPlugins(hs, map[string]map[string]string{"fake": {"plugin.json": string(manifest)}})
	t.Cleanup(stopServers)
	path := hs.path("source.fake")
	if err := os.WriteFile(path, []byte("first()\nsecond()\nfunc first() {}\nfunc second() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	ed := hs.a.activeEditor()
	var client *lsp.Client
	hs.untilLong("source navigation server", func() bool {
		client = hs.a.lspFor(ed.document, false).live()
		return client != nil
	})
	return hs, ed, client
}

func sourceNavigationRequests(t *testing.T, c *lsp.Client) (definitions, hovers int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := c.Call(ctx, "test/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	var state struct{ DefinitionCount, HoverCount int }
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return state.DefinitionCount, state.HoverCount
}

// Nothing else is pending when a held reply is released. Wait for its UI
// callback to be queued before drawing, including a callback that rejects it.
func sourceNavigationReply(t *testing.T, hs *harness, c *lsp.Client, method string, params any) {
	t.Helper()
	hs.settle()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := c.Call(ctx, method, params)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "true" {
		t.Fatalf("held reply was not released: %s", raw)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		hs.a.mu.Lock()
		queued := len(hs.a.queue) > 0
		hs.a.mu.Unlock()
		if queued {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("navigation reply did not post its UI callback")
		}
		time.Sleep(time.Millisecond)
	}
	hs.settle()
}

// Cancelled requests have no waiter left to post a UI callback. The command's
// reply follows the discarded result on the same wire, so it is the barrier.
func sourceNavigationDiscardedReply(t *testing.T, hs *harness, c *lsp.Client, method string, params any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := c.Call(ctx, method, params)
	if err != nil || string(raw) != "true" {
		t.Fatalf("discarded reply: %s, %v", raw, err)
	}
	hs.settle()
}

func TestSourceNavigationHoverKeepsNewestReply(t *testing.T) {
	hs, ed, client := sourceNavigationHarness(t)
	a := hs.a
	tab, version := a.activeTab(), ed.buf.Version()
	hs.place(ed, 1, 1)
	a.showHover()
	hs.until("first held hover", func() bool {
		_, count := sourceNavigationRequests(t, client)
		return count == 1
	})
	hs.place(ed, 9, 9)
	a.showHover()
	hs.until("second held hover", func() bool {
		_, count := sourceNavigationRequests(t, client)
		return count == 2
	})
	sourceNavigationReply(t, hs, client, "test/replyHover", map[string]any{"index": 1, "text": "newer hover"})
	if a.hover == nil || a.hover.tree.Text != "newer hover" {
		t.Fatal("the newer hover was not shown")
	}
	sourceNavigationDiscardedReply(t, hs, client, "test/replyHover", map[string]any{"index": 0, "text": "older hover"})
	if a.activeTab() != tab || ed.buf.Version() != version {
		t.Fatal("the test changed tabs or the document version")
	}
	if a.hover == nil || a.hover.tree.Text != "newer hover" {
		t.Fatalf("older reply replaced the newest hover: %+v", a.hover)
	}
}

func TestSourceNavigationDefinitionKeepsNewestReply(t *testing.T) {
	hs, ed, client := sourceNavigationHarness(t)
	a := hs.a
	tab, version := a.activeTab(), ed.buf.Version()
	hs.place(ed, 1, 1)
	a.goToDefinition()
	hs.until("first held definition", func() bool {
		count, _ := sourceNavigationRequests(t, client)
		return count == 1
	})
	hs.place(ed, 9, 9)
	a.goToDefinition()
	hs.until("second held definition", func() bool {
		count, _ := sourceNavigationRequests(t, client)
		return count == 2
	})
	sourceNavigationReply(t, hs, client, "test/replyDefinition", map[string]any{
		"index": 1, "uri": serverURI(ed.path), "line": 3, "character": 5,
	})
	want := position{Line: 3, Character: 5}
	if got := caretPosition(ed); got != want {
		t.Fatalf("newer definition position = %+v, want %+v", got, want)
	}
	sourceNavigationDiscardedReply(t, hs, client, "test/replyDefinition", map[string]any{
		"index": 0, "uri": serverURI(ed.path), "line": 2, "character": 5,
	})
	if a.activeTab() != tab || ed.buf.Version() != version {
		t.Fatal("the test changed tabs or the document version")
	}
	if got := caretPosition(ed); got != want {
		t.Fatalf("older definition moved the caret to %+v, want %+v", got, want)
	}
}

type sourceNavigationSnapshot struct {
	Definition diffNavRequest
}

func sourceNavigationState(t *testing.T, c *lsp.Client) sourceNavigationSnapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := c.Call(ctx, "test/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	var s sourceNavigationSnapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// gd is the key of the definition in Helix and vim, and the request asks
// for the name the cursor is on rather than a position after it.
func TestSourceNavigationGd(t *testing.T) {
	for _, mode := range []string{"helix", "vim"} {
		t.Run(mode, func(t *testing.T) {
			hs, ed, client := sourceNavigationHarness(t)
			a := hs.a
			a.settings.EditorMode = mode
			a.settings.Vim = mode == "vim"
			hs.frames()
			// second, on line 1, selected from its first rune.
			hs.place(ed, 8, 8)
			if mode == "helix" {
				helixSelect(ed, 8, 8)
			}
			hs.settle()
			hs.tt.Key(0, ui.KeyG)
			hs.frames()
			hs.tt.Key(0, ui.KeyD)
			hs.until("gd definition request", func() bool {
				count, _ := sourceNavigationRequests(t, client)
				return count == 1
			})
			if got := sourceNavigationState(t, client).Definition.Position; got != (position{1, 0}) {
				t.Fatalf("gd asked for %+v, want the name at line 1, column 0", got)
			}
		})
	}
}

// A Cmd-click on a name goes to its definition, as one in a code editor
// does, and asks for the rune the click is on.
func TestSourceNavigationCmdClick(t *testing.T) {
	hs, ed, client := sourceNavigationHarness(t)
	a := hs.a
	hs.place(ed, 0, 0)
	hs.settle()
	// second, on line 1, its first rune.
	view := ed.lastView
	x := view.X + ed.textX + a.font.charW*1.5
	y := view.Y + editorPadY + a.font.lineH*1.5 - ed.vscroll.Y
	hs.tt.ClickAtWith(ui.Cmd, x, y)
	hs.until("Cmd-click definition request", func() bool {
		count, _ := sourceNavigationRequests(t, client)
		return count == 1
	})
	if got := sourceNavigationState(t, client).Definition.Position; got != (position{1, 1}) {
		t.Fatalf("Cmd-click asked for %+v, want the name at line 1, column 1", got)
	}
}

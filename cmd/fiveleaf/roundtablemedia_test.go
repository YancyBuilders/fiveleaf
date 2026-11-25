package main

import (
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestRoundtableLocalMedia(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inside, outside := filepath.Join(root, "图 sample.png"), filepath.Join(t.TempDir(), "outside.png")
	writePNG(t, inside, 37, 22)
	writePNG(t, outside, 2, 2)
	for name, target := range map[string]string{"local.png": inside, "escape.png": outside} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	fileURL := (&url.URL{Scheme: "file", Path: inside}).String()
	for _, dest := range []string{"图%20sample.png", "/图%20sample.png", "local.png", fileURL, inside} {
		view, err := roundtableLoadImage(root, dest)
		if err != nil || view.src == nil || view.w != 37 || view.h != 22 {
			t.Errorf("project image %q = %v, %v", dest, view, err)
		}
	}
	for _, dest := range []string{"escape.png", outside, "../outside.png", "file://unexpected/file.png", "javascript:alert(1)", "//outside.test/pic.png"} {
		if _, err := roundtableLoadImage(root, dest); err == nil {
			t.Errorf("unexpected image from %q", dest)
		}
	}
	if _, err := roundtableLoadImage("", inside); err == nil {
		t.Error("a history record without a project loaded a file")
	}
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := roundtableLoadImage(root, filepath.Join(alias, filepath.Base(inside))); err != nil {
		t.Errorf("absolute project alias failed: %v", err)
	}
	large, err := os.ReadFile(inside)
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint32(large[16:20], 12000)
	binary.BigEndian.PutUint32(large[20:24], 12000)
	binary.BigEndian.PutUint32(large[29:33], crc32.ChecksumIEEE(large[12:29]))
	if err := os.WriteFile(filepath.Join(root, "large.png"), large, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := roundtableLoadImage(root, "large.png"); err == nil || err.Error() != locales.ErrImageTooLarge() {
		t.Errorf("oversized bitmap returned %v", err)
	}
}

func roundtableMediaTester(t *testing.T, root string, body *string) (*roundtableWindow, *ui.Tester) {
	t.Helper()
	pal := darkPalette().derive()
	window := &roundtableWindow{a: &app{settings: settings.Default(), pal: pal}, pal: pal, meeting: &agentMeeting{library: roundtable.Library{Project: root}}}
	tester := ui.NewTester(func(c *ui.Context) {
		window.a.mu.Lock()
		queue := window.a.queue
		window.a.queue = nil
		window.a.mu.Unlock()
		for _, action := range queue {
			action()
		}
		ui.Column(c).Fill().Children(func() { window.messageMarkdown(c, "reply", *body) })
		window.mediaOverlay(c)
	}, 650, 600)
	t.Cleanup(window.finish)
	return window, tester
}

func waitRoundtableMedia(t *testing.T, tester *ui.Tester, label string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		step(tester)
		if _, ok := tester.Find(label); ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("missing %q in %v", label, tester.Texts())
}

func TestRoundtableMediaMixedImagesAndZoom(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "diagram.png"), 1100, 700)
	body := "**Before** ![Output](diagram.png) after.\n\n![Missing](deleted.png)"
	window, tester := roundtableMediaTester(t, root, &body)
	action := locales.PreviewLabel("Output")
	waitRoundtableMedia(t, tester, action)
	waitRoundtableMedia(t, tester, locales.CannotShow("Missing"))
	if !tester.HasText("Before [Output] after.") {
		t.Fatalf("mixed paragraph was lost: %v", tester.Texts())
	}
	thumb, _ := tester.Find(action)
	if thumb.W > 461 || thumb.H > 211 || thumb.W < 100 {
		t.Fatalf("large image thumbnail has bounds %+v", thumb)
	}
	if err := tester.Click(action); err != nil {
		t.Fatal(err)
	}
	waitRoundtableMedia(t, tester, locales.FitToView())
	view := window.media.view
	pane, ok := tester.Find(locales.ImageLabel("Output"))
	if !ok || view.w != 1100 || view.h != 700 || view.paneW <= 0 || view.paneH <= 0 {
		t.Fatalf("full image has bounds %+v, viewer %+v", pane, view)
	}
	tester.Pinch(pane.X+pane.W/2, pane.Y+pane.H/2, 2)
	if view.zoom == 0 {
		t.Fatal("pinch did not zoom the media preview")
	}
	if err := tester.Click(locales.FitToView()); err != nil || view.zoom != 0 {
		t.Fatalf("fit left zoom %v: %v", view.zoom, err)
	}
	if err := tester.Click(locales.CmdImageZoomIn()); err != nil || view.zoom == 0 {
		t.Fatalf("zoom button left zoom %v: %v", view.zoom, err)
	}
	tester.Key(0, ui.KeyEscape)
	step(tester)
	if window.media != nil || !tester.HasText("Before [Output] after.") {
		t.Fatal("Escape did not return to the message")
	}
}

func TestRoundtableMediaExternalRequiresClick(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		resp.Header().Set("Content-Type", "image/svg+xml")
		_, _ = resp.Write([]byte(testSVG))
	}))
	defer server.Close()
	body := "![External](" + server.URL + "/chart)"
	window, tester := roundtableMediaTester(t, t.TempDir(), &body)
	host := strings.TrimPrefix(server.URL, "http://")
	button := locales.DownloadTitle("External · " + host)
	waitRoundtableMedia(t, tester, button)
	for range 6 {
		step(tester)
	}
	if requests.Load() != 0 {
		t.Fatal("the diagram downloaded before consent")
	}
	if err := tester.Click(button); err != nil {
		t.Fatal(err)
	}
	waitRoundtableMedia(t, tester, locales.PreviewLabel("External"))
	if err := tester.Click(locales.PreviewLabel("External")); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || window.media == nil || window.media.view.w != 64 {
		t.Fatalf("got %d downloads, preview %+v", requests.Load(), window.media)
	}
	window.viewGeneration++
	step(tester)
	if window.media != nil {
		t.Fatal("another meeting retained the old media preview")
	}
}

func TestRoundtableMediaMermaidDiagramAndFallback(t *testing.T) {
	body := "```mErMaId\nflowchart LR\nA[Start] --> B[End]\n```"
	window, tester := roundtableMediaTester(t, t.TempDir(), &body)
	waitRoundtableMedia(t, tester, locales.OpenDiagram())
	if err := tester.Click(locales.OpenDiagram()); err != nil {
		t.Fatal(err)
	}
	if window.media == nil || window.media.view.diagram == nil || len(window.media.view.diagram.labels) == 0 {
		t.Fatal("Mermaid did not produce a real diagram with labels")
	}
	waitRoundtableMedia(t, tester, locales.CloseEsc())
	if err := tester.Click(locales.CloseEsc()); err != nil || window.media != nil {
		t.Fatalf("close did not return to the diagram: %v", err)
	}
	body = "```mermaid\nnot a diagram\n```"
	waitRoundtableMedia(t, tester, "not a diagram")
	if _, ok := tester.Find(locales.OpenDiagram()); ok {
		t.Fatal("an invalid diagram kept the previous thumbnail")
	}
	body = "```mermaid\nflowchart LR\nA --> B\n```"
	window.a.settings.PreviewMermaid = false
	waitRoundtableMedia(t, tester, "flowchart LR\nA --> B")
}

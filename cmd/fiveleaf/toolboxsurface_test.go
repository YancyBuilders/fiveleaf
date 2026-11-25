package main

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestToolboxTabFrameAndPage(t *testing.T) {
	harness := newHarness(t, false)
	toolboxTestPlugins(harness, "base64", "json")
	files := toolboxSurfaceFiles(toolboxFiles())
	get := func(raw string) *httptest.ResponseRecorder {
		reply := httptest.NewRecorder()
		files.ServeHTTP(reply, httptest.NewRequest(http.MethodGet, raw, nil))
		return reply
	}
	frame := get(toolboxFrameURL("toolbox-base64"))
	tool := toolboxURL(plugins.toolboxProvider("toolbox-base64"))
	source, origin := toolboxFrameSource(tool, runtime.GOOS)
	if frame.Code != http.StatusOK || !strings.Contains(frame.Body.String(), `src="`+source+`"`) || !strings.Contains(frame.Header().Get("Content-Security-Policy"), "frame-src "+origin+";") {
		t.Fatalf("frame response %d: %s", frame.Code, frame.Body.String())
	}
	if frame := get(toolboxFrameURL("toolbox-url")); frame.Code != http.StatusNotFound {
		t.Fatalf("disabled tool's frame = %d", frame.Code)
	}
	page := get(tool)
	bridge := strings.Index(page.Body.String(), `<script src="`+toolboxBridgePath+`"`)
	entry := strings.Index(page.Body.String(), `<script type="module"`)
	if page.Code != http.StatusOK || bridge < 0 || entry <= bridge || !strings.HasPrefix(strings.ToLower(page.Body.String()), "<!doctype html>") {
		t.Fatal("DevTool entry point lost its doctype, bridge, or original module")
	}
	for _, address := range []string{toolboxFrameURL("toolbox-base64") + "tab-host.js", toolboxScheme + "://toolbox-base64.localhost" + toolboxBridgePath} {
		reply := get(address)
		if reply.Code != http.StatusOK || !strings.Contains(reply.Header().Get("Content-Type"), "javascript") {
			t.Fatalf("bridge script %s = %d %v", address, reply.Code, reply.Header())
		}
	}
}

func TestToolboxWebView2FrameSource(t *testing.T) {
	page, origin := toolboxFrameSource("fiveleaf-toolbox://base64.localhost/?tools=base64#/tool/base64", "windows")
	if origin != "http://base64.localhost.fiveleaf-toolbox.localhost" || page != origin+"/?tools=base64#/tool/base64" {
		t.Fatalf("WebView2 frame source %q, CSP origin %q", page, origin)
	}
}

func TestToolboxTabPreservesPluginMarkup(t *testing.T) {
	for _, original := range []string{
		"\xef\xbb\xbf<!-- plugin --><!DOCTYPE html>\n<html lang='en'><head data-comment='>'><script src='live.js'></script></head></html>",
		"<!doctype html>\n<script>window.boot()</script>",
		"<div>hello</div>",
	} {
		reply := httptest.NewRecorder()
		serveToolboxAsset(reply, httptest.NewRequest(http.MethodGet, "/", nil), "example.html", strings.NewReader(original))
		injected := `<script src="` + toolboxBridgePath + `"></script>`
		if reply.Code != http.StatusOK || !strings.Contains(reply.Body.String(), injected) || strings.Replace(reply.Body.String(), injected, "", 1) != original {
			t.Fatalf("markup changed: %s", reply.Body.String())
		}
	}
}

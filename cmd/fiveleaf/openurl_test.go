package main

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"fiveleaf/internal/cli"
)

func TestOpenURLTargetQuery(t *testing.T) {
	file := filepath.Join(t.TempDir(), "src", "App.tsx")
	raw := "fiveleaf://open?file=" + url.QueryEscape(file) + "&line=12&column=5"
	want := cli.Target{Path: filepath.Clean(file), Line: 12, Col: 5}
	if got, ok := openURLTarget(raw); !ok || got != want {
		t.Fatalf("openURLTarget(%q) = %+v, %v; want %+v", raw, got, ok, want)
	}
	raw = "fiveleaf://open?file=" + url.QueryEscape(file)
	want = cli.Target{Path: filepath.Clean(file)}
	if got, ok := openURLTarget(raw); !ok || got != want {
		t.Fatalf("openURLTarget(%q) = %+v, %v; want %+v", raw, got, ok, want)
	}
}

func TestOpenURLTargetFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows path is tested by TestOpenURLTargetFileWindows")
	}
	file := filepath.Join(t.TempDir(), "My App", "src", "App.tsx")
	escaped := strings.ReplaceAll(filepath.ToSlash(file), " ", "%20")
	for _, raw := range []string{
		"fiveleaf://file" + escaped + ":12:5",
		"fiveleaf://file/" + escaped + ":12:5",
	} {
		want := cli.Target{Path: filepath.Clean(file), Line: 12, Col: 5}
		if got, ok := openURLTarget(raw); !ok || got != want {
			t.Fatalf("openURLTarget(%q) = %+v, %v; want %+v", raw, got, ok, want)
		}
	}
}

func TestOpenURLTargetFileUnescaped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows path is tested by TestOpenURLTargetFileWindows")
	}
	dir := filepath.ToSlash(t.TempDir())
	for _, name := range []string{"a#b.tsx", "a?b.tsx", "100%.tsx", "[slug].tsx"} {
		raw := "fiveleaf://file" + dir + "/" + name + ":3:1"
		want := cli.Target{Path: filepath.Join(dir, name), Line: 3, Col: 1}
		if got, ok := openURLTarget(raw); !ok || got != want {
			t.Fatalf("openURLTarget(%q) = %+v, %v; want %+v", raw, got, ok, want)
		}
	}
}

func TestIsFiveLeafURL(t *testing.T) {
	for arg, want := range map[string]bool{
		"fiveleaf://file/a.go:1:2": true,
		"FIVELEAF://open?file=/a":  true,
		"/Users/me/fiveleaf":       false,
		`C:\Users\me`:              false,
		"fiveleaf.go":              false,
	} {
		if got := isFiveLeafURL(arg); got != want {
			t.Errorf("isFiveLeafURL(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestOpenURLTargetFileWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("a Windows path")
	}
	file := filepath.Join(t.TempDir(), "My App", "src", "App.tsx")
	raw := "fiveleaf://file/" + filepath.ToSlash(file) + ":12:5"
	want := cli.Target{Path: filepath.Clean(file), Line: 12, Col: 5}
	if got, ok := openURLTarget(raw); !ok || got != want {
		t.Fatalf("openURLTarget(%q) = %+v, %v; want %+v", raw, got, ok, want)
	}
}

func TestOpenURLTargetRejects(t *testing.T) {
	file := filepath.Join(t.TempDir(), "App.tsx")
	for _, raw := range []string{
		"https://example.com/App.tsx:1:2",
		"fiveleaf://other?file=" + url.QueryEscape(file),
		"fiveleaf://file",
		"fiveleaf://files/" + filepath.ToSlash(file),
		"fiveleaf://open?file=",
		"fiveleaf://open?file=" + url.QueryEscape(file) + "&line=0",
		"fiveleaf://open?file=" + url.QueryEscape(file) + "&column=-1",
		"fiveleaf://open?file=" + url.QueryEscape(file) + "&line=x",
		"fiveleaf://open?file=src/App.tsx",
	} {
		if got, ok := openURLTarget(raw); ok {
			t.Fatalf("openURLTarget(%q) = %+v, true; want false", raw, got)
		}
	}
}

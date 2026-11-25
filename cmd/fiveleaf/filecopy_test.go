package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"fiveleaf/locales"
)

func TestFreeName(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.go", "a copy.go", ".env", "lib.d"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	for _, c := range []struct {
		name  string
		isDir bool
		want  string
	}{
		{"new.go", false, "new.go"},
		{"a.go", false, "a copy 2.go"},
		{".env", false, ".env copy"},
		{"lib.d", true, "lib.d copy"},
	} {
		if got := filepath.Base(freeName(dir, c.name, c.isDir)); got != c.want {
			t.Errorf("freeName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCopyInto(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(src, "pkg/sub"), 0o755)
	os.WriteFile(filepath.Join(src, "pkg/sub/run.sh"), []byte("echo hi\n"), 0o755)
	os.Symlink("sub/run.sh", filepath.Join(src, "pkg/link"))

	to, err := copyInto(filepath.Join(src, "pkg"), dst)
	if err != nil || to != filepath.Join(dst, "pkg") {
		t.Fatalf("copyInto = %q, %v", to, err)
	}
	info, err := os.Stat(filepath.Join(to, "sub/run.sh"))
	// Windows has no permission bits to copy.
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("run.sh: %v %v", info, err)
	}
	// Windows' links keep backslashes.
	if target, err := os.Readlink(filepath.Join(to, "link")); err != nil || filepath.ToSlash(target) != "sub/run.sh" {
		t.Errorf("link: %q %v", target, err)
	}
	// Into itself, or a folder within it: refused.
	for _, dir := range []string{to, filepath.Join(to, "sub")} {
		if _, err := copyInto(to, dir); err == nil {
			t.Errorf("pasted %s into %s", to, dir)
		}
	}
}

func TestCopyIntoRejectsLinkedDescendant(t *testing.T) {
	for _, same := range []bool{true, false} {
		name := "descendant"
		if same {
			name = "same directory"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			// A long name bounds the depth if a regression starts copying
			// the destination back into itself.
			src := filepath.Join(root, strings.Repeat("source", 30))
			dir := src
			if !same {
				dir = filepath.Join(src, "child")
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(root, "alias")
			if err := os.Symlink(dir, alias); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			to, err := copyInto(src, alias)
			if err == nil || err.Error() != locales.ErrPasteIntoItself() || to != "" {
				t.Errorf("copyInto = %q, %v; want refusal before copying", to, err)
			}
			after, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != len(before) {
				t.Errorf("refused copy left %d new entries", len(after)-len(before))
			}
		})
	}
}

func TestCopyIntoThroughLinkedParent(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "source-other")
	if err := os.MkdirAll(filepath.Join(src, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "note.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	linkedSource := filepath.Join(alias, "source")
	if to, err := copyInto(linkedSource, filepath.Join(src, "child")); err == nil || to != "" {
		t.Fatalf("copied through a linked source parent: %q, %v", to, err)
	}
	// A shared name prefix is not a descendant, and a valid destination
	// reached through a link still reports the path the caller supplied.
	linkedTarget := filepath.Join(alias, "source-other")
	to, err := copyInto(src, linkedTarget)
	if err != nil || to != filepath.Join(linkedTarget, "source") {
		t.Fatalf("copy into linked sibling = %q, %v", to, err)
	}
	if data, err := os.ReadFile(filepath.Join(to, "note.txt")); err != nil || string(data) != "keep me" {
		t.Fatalf("copied file = %q, %v", data, err)
	}
}

func TestCopyIntoRejectsCaseAlias(t *testing.T) {
	root := t.TempDir()
	name := strings.Repeat("source", 30)
	src := filepath.Join(root, name)
	dir := filepath.Join(src, "child")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	caseDir := filepath.Join(root, strings.ToUpper(name), "child")
	caseInfo, err := os.Stat(caseDir)
	if os.IsNotExist(err) {
		t.Skip("filesystem distinguishes case")
	}
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || !os.SameFile(info, caseInfo) {
		t.Fatalf("case alias does not name the same directory: %v", err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(caseDir, alias); err != nil {
		t.Fatal(err)
	}
	to, err := copyInto(src, alias)
	if err == nil || err.Error() != locales.ErrPasteIntoItself() || to != "" {
		t.Errorf("copyInto = %q, %v; want refusal before copying", to, err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("refused copy changed destination: %d entries, %v", len(entries), err)
	}
}

func TestExplorerCopyPaste(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	// ⌘C copies the file chosen.
	if err := hs.tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.tt.Command("copy")
	if !slices.Equal(hs.h.clipboard, []string{hs.path("main.go")}) {
		t.Fatalf("clipboard %q", hs.h.clipboard)
	}

	// ⌘V into the folder chosen: a file of the Finder, and the one copied;
	// pasted again, it is named as a copy.
	other := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(other, []byte("hi\n"), 0o644)
	hs.h.clipboard = []string{other, hs.path("main.go")}
	a.revealInTree(hs.path("web/index.html"))
	hs.settle()
	hs.tt.Command("paste")
	hs.settle()
	for _, p := range []string{"web/notes.txt", "web/main.go"} {
		if _, err := os.Stat(hs.path(p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if got := a.files.Item(a.fileRow); got != hs.path("web/notes.txt") {
		t.Errorf("chosen %q", got)
	}
	hs.tt.Command("paste")
	hs.settle()
	if _, err := os.Stat(hs.path("web/main copy.go")); err != nil {
		t.Error(err)
	}
}

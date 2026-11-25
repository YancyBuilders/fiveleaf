package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// showTree spells the rows of a tree of files, a folder's name ending in
// a slash, each indented by its depth.
func showTree(rows []treeRow) string {
	var out []string
	for _, r := range rows {
		s := strings.Repeat(" ", r.depth)
		if r.dir != "" {
			s += r.name + "/"
		} else {
			s += filepath.Base(r.e.Path)
		}
		out = append(out, s)
	}
	return strings.Join(out, "|")
}

// The tab of a commit hides the panel of its files at first; shown, it
// lists them beside the diff, in a tree of their folders: a click on one
// scrolls the diff to it, a folder closes, and the panel hides again.
func TestCommitFilesPanel(t *testing.T) {
	hs := newHarness(t, true)
	hs.gitRepo()
	a, tt := hs.a, hs.tt
	a.settings.ScmTree = true
	a.window.SidebarClosed = true // the Explorer's folders are named as the panel's
	a.showCommit(a.git.history[0], true)
	hs.settleGit()
	hs.settle()
	v := a.activeTab().diff
	if v == nil || len(v.files) == 0 {
		t.Fatalf("no files in the commit's tab")
	}
	if a.window.CommitFilesOpen || tt.HasText("Hide Files") {
		t.Fatalf("the panel shows at first")
	}
	a.run("git.toggleCommitFiles")
	hs.settle()
	rows := v.tree.rows
	var trees []treeRow
	for _, r := range rows {
		trees = append(trees, r.treeRow)
	}
	if got, want := showTree(trees), "internal/| a/|  a.go| b/|  b.ts|web/| index.html| style.css|.gitignore|README.md|go.mod|main.go"; got != want {
		t.Fatalf("rows\n%s, want\n%s", got, want)
	}
	if !tt.HasText("Files") {
		t.Errorf("no panel: %q", tt.Texts())
	}
	hs.shot("commit-files-panel")
	// A file clicked is the one the diff shows at its top.
	if err := tt.Click("style.css"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	k := slices.IndexFunc(v.files, func(f *diffFile) bool { return filepath.Base(f.path) == "style.css" })
	if k < 0 || v.cursor != v.heads[k] {
		t.Errorf("cursor %d, not the header %v of style.css", v.cursor, v.heads)
	}
	if v.tree.top != v.files[k] {
		t.Errorf("the file at the top is not style.css")
	}
	// A folder closed hides its files.
	if err := tt.Click("internal"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if got, want := len(v.tree.rows), len(rows)-4; got != want {
		t.Errorf("%d rows with internal closed, want %d", got, want)
	}
	// Hidden, the diff takes the tab; shown, the panel is back.
	a.run("git.toggleCommitFiles")
	hs.settle()
	if a.window.CommitFilesOpen || tt.HasText("Hide Files") {
		t.Errorf("the panel did not hide")
	}
	a.run("git.toggleCommitFiles")
	hs.settle()
	if !a.window.CommitFilesOpen || !tt.HasText("Hide Files") {
		t.Errorf("the panel did not show again")
	}
	// As a list, the files go by path.
	a.settings.ScmTree = false
	hs.settle()
	if got := len(v.tree.rows); got != len(v.files) {
		t.Errorf("%d rows as a list, want %d", got, len(v.files))
	}
}

func TestCommitFilesFromGraph(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	a, tt := hs.a, hs.tt
	a.settings.ScmTree = true
	os.WriteFile(hs.path("internal/a/a.go"), []byte("package a\n\nconst Answer = 43\n"), 0o644)
	os.WriteFile(hs.path("notes.txt"), []byte("notes\n"), 0o644)
	git("add", ".")
	git("commit", "-q", "-m", "second")
	a.run("view.scm")
	hs.settleGit()
	a.run("git.openGraph")
	hs.settle()
	if err := tt.Click("Open Changes"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	view := a.activeTab().diff
	if view == nil || view.commit == nil || view.commit.hash != a.git.history[0].Hash {
		t.Fatal("the graph did not open the selected commit")
	}
	a.run("git.toggleCommitFiles")
	hs.settle()
	var rows []treeRow
	for _, row := range view.tree.rows {
		rows = append(rows, row.treeRow)
	}
	if got := showTree(rows); got != "internal/a/| a.go|notes.txt" {
		t.Fatalf("files in the selected commit: %s", got)
	}
	if err := tt.Click("notes.txt"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	index := slices.IndexFunc(view.files, func(file *diffFile) bool { return filepath.Base(file.path) == "notes.txt" })
	if index < 0 || view.cursor != view.heads[index] {
		t.Fatalf("cursor %d, expected the header of notes.txt in %v", view.cursor, view.heads)
	}
}

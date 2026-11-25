package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/git"
	"fiveleaf/locales"
)

func TestDiffCompactToolbarFitsCommit(t *testing.T) {
	hs := newHarness(t, false)
	gitCmd := hs.gitRepo()
	hs.a.showCommit(git.Commit{Hash: strings.TrimSpace(gitCmd("rev-parse", "HEAD"))}, true)
	tab := hs.a.activeTab()
	hs.settleDiff(tab.diff)
	for _, width := range []int{160, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			bar := ui.NewTester(func(context *ui.Context) { hs.a.breadcrumbs(context, hs.a.column(), tab) }, width, 60)
			step(bar)
			for _, label := range []string{locales.SearchCollapseAll(), locales.DiffMoreActions()} {
				button, found := bar.Find(label)
				if !found || button.X < 0 || button.W <= 0 || button.X+button.W > float32(width) {
					t.Fatalf("%s outside %dpx toolbar: %+v", label, width, button)
				}
			}
			if err := bar.Click(locales.DiffMoreActions()); err != nil {
				t.Fatal(err)
			}
			step(bar)
			if err := bar.ChooseMenuItem(locales.DiffNextChange()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDiffToolbarFoldToggle(t *testing.T) {
	hs := newHarness(t, false)
	hs.gitRepo()
	for _, name := range []string{"one.txt", "two.txt"} {
		if err := os.WriteFile(hs.path(name), []byte("added\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hs.settleGit()
	hs.a.run("git.openAllChanges")
	view := hs.a.activeTab().diff
	hs.settleDiff(view)
	if len(view.heads) != 2 {
		t.Fatalf("expected two file headers, got %v", view.heads)
	}
	hs.a.activateRow(view, view.heads[0], false)
	hs.settle()
	for _, folded := range []bool{true, false} {
		label := locales.SearchExpandAll()
		if folded {
			label = locales.SearchCollapseAll()
		}
		if err := hs.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		for _, file := range view.files {
			if file.folded != folded {
				t.Fatalf("%s after %q: folded=%v", file.path, label, file.folded)
			}
		}
	}
}

// A bulk choice made before the commit's file list arrives must survive
// both that list and the first read of a large file's contents.
func TestDiffFoldAllBeforeCommitLoads(t *testing.T) {
	for _, folded := range []bool{true, false} {
		name, command := "expand", "git.expandAllFiles"
		if folded {
			name, command = "collapse", "git.collapseAllFiles"
		}
		t.Run(name, func(t *testing.T) {
			hs := newHarness(t, true)
			gitCmd := hs.gitRepo()
			for path, text := range map[string]string{
				"a.txt": strings.Repeat("line\n", foldedLines+1),
				"b.txt": "small\n",
			} {
				if err := os.WriteFile(hs.path(path), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			gitCmd("add", ".")
			gitCmd("commit", "-qm", "two files")
			hs.a.showCommit(git.Commit{Hash: strings.TrimSpace(gitCmd("rev-parse", "HEAD"))}, true)
			v := hs.a.activeTab().diff
			hs.a.run(command)
			hs.settleDiff(v)
			if len(v.files) != 2 {
				t.Fatalf("files: %d, want 2", len(v.files))
			}
			for _, f := range v.files {
				if f.folded != folded {
					t.Errorf("%s folded=%v, want %v", f.path, f.folded, folded)
				}
			}
		})
	}
}

// User shortcuts run the same commands as the menu and palette. They
// retain the current file, affect only this tab, and leave Git untouched.
func TestDiffFoldAllCommands(t *testing.T) {
	hs := newHarness(t, true)
	gitCmd := hs.gitRepo()
	a, tt := hs.a, hs.tt
	for _, path := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(hs.path(path), []byte(strings.Repeat("line\n", 60)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitCmd("add", ".")
	gitCmd("commit", "-qm", "two files")
	a.compareFiles(hs.path("a.txt"), hs.path("b.txt"))
	other := a.activeTab().diff
	hs.settleDiff(other)
	a.showCommit(git.Commit{Hash: strings.TrimSpace(gitCmd("rev-parse", "HEAD"))}, true)
	v := a.activeTab().diff
	hs.settleDiff(v)
	withKeys(t, a, `[
		{"key":"ctrl+shift+f9","command":"git.collapseAllFiles"},
		{"key":"ctrl+shift+f10","command":"git.expandAllFiles"}
	]`)
	for i, r := range v.rows {
		if r.file == 1 && r.kind == rowLine {
			v.setCursor(i)
			v.jumpRow(i, jumpTop)
			break
		}
	}
	hs.settle()
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyF9)
	hs.settle()
	for _, f := range v.files {
		if !f.folded {
			t.Fatalf("shortcut left %s expanded", f.path)
		}
	}
	for _, r := range v.rows {
		if r.kind == rowLine {
			t.Fatal("collapsed diff still shows file contents")
		}
	}
	if v.cursor < 0 || v.rows[v.cursor].kind != rowFile || v.rows[v.cursor].file != 1 {
		t.Fatalf("collapse lost the current file: cursor %d", v.cursor)
	}
	if y := editorPadY + float32(v.tops[v.cursor])*a.font.lineH - v.scroll.Y; y < 0 || y >= v.viewH {
		t.Fatalf("collapsed file header is outside the view at %v", y)
	}
	a.setDiffLayout(v, layoutSplit)
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyF10)
	hs.settle()
	for _, f := range v.files {
		if f.folded {
			t.Fatalf("shortcut left %s collapsed", f.path)
		}
	}
	if other.foldFiles != nil || other.files[0].folded {
		t.Fatal("bulk folding changed another diff tab")
	}
	a.openFile(hs.path("a.txt"), true)
	hs.settle()
	a.run("git.collapseAllFiles")
	for _, f := range v.files {
		if f.folded {
			t.Fatal("the command changed an inactive diff from a text tab")
		}
	}
	if status := gitCmd("status", "--porcelain"); status != "" {
		t.Fatalf("view commands changed Git state: %s", status)
	}
}

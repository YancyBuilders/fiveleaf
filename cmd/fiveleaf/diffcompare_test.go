package main

import (
	"os"
	"slices"
	"testing"

	"github.com/egoist/mygo"

	"fiveleaf/locales"
)

func TestCompareLocalFilesCommandLocation(t *testing.T) {
	hs := newHarness(t, false)
	label := locales.CmdGitCompareFiles()
	var menus, categories []string
	bar := hs.a.menu(false, func(id string, _ *mygo.Window) { hs.a.run(id) })
	for _, menu := range bar.Items() {
		for _, item := range menu.Submenu {
			if item.Label == label {
				menus = append(menus, menu.Label)
				item.Click(item, nil)
			}
		}
	}
	for _, entry := range hs.a.paletteEntries() {
		if entry.title[entry.group:] == label {
			categories = append(categories, entry.category())
		}
	}
	want := []string{locales.MenuFile()}
	if !slices.Equal(menus, want) || !slices.Equal(categories, want) {
		t.Errorf("Compare Two Files menus %v, palette categories %v, want %v", menus, categories, want)
	}
	if !slices.Contains(hs.h.asked, locales.CompareTwoFiles()) {
		t.Error("the menu command did not open the file picker")
	}
	if got := hs.a.commandID("git.compareFiles"); got != "file.compareFiles" {
		t.Errorf("existing shortcut resolves to %q, want file.compareFiles", got)
	}
}

func TestCompareLocalFilesAcceptsEitherSide(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.settings.AutoSave = false
	left, right := hs.path("left.txt"), hs.path("right.txt")
	if err := os.WriteFile(left, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("one\nchanged\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.h.chosen = []string{left, right}
	a.run("git.compareFiles")
	tab := a.activeTab()
	if tab == nil || tab.diff == nil {
		t.Fatal("the previous compare command did not open a diff tab")
	}
	v := tab.diff
	hs.settleDiff(v)
	if len(v.files) != 1 || len(v.files[0].hunks) != 1 {
		t.Fatalf("local diff hunks: %#v", v.files)
	}
	f := v.files[0]
	ars := hunkArrows(f)
	if len(ars) != 2 || ars[0].dir != 1 || ars[1].dir != -1 {
		t.Fatalf("local arrows: %#v", ars)
	}
	if err := os.Chmod(left, 0o444); err != nil {
		t.Fatal(err)
	}
	if ro := hunkArrows(f); len(ro) != 1 || ro[0].dir != 1 {
		t.Fatalf("read-only left arrows: %#v", ro)
	}
	if err := os.Chmod(left, 0o644); err != nil {
		t.Fatal(err)
	}
	h := f.hunks[0]

	// Accept Left opens and edits the right document, and the diff keyboard
	// can undo and redo that edit as one history step.
	a.applyArrow(v, f, h, ars[0])
	rightDoc := a.docs[a.docIndex(right)]
	if got := rightDoc.buf.String(); got != "one\ntwo\nthree\n" {
		t.Fatalf("right after Accept Left: %q", got)
	}
	a.undoDiff(v, false)
	hs.settle()
	if got := rightDoc.buf.String(); got != "one\nchanged\nthree\n" {
		t.Fatalf("right after undo: %q", got)
	}
	a.undoDiff(v, true)
	hs.settle()
	if got := rightDoc.buf.String(); got != "one\ntwo\nthree\n" {
		t.Fatalf("right after redo: %q", got)
	}

	// Undo the right edit, then Accept Right writes the right side into left.
	a.undoDiff(v, false)
	hs.settle()
	a.applyArrow(v, f, h, ars[1])
	leftDoc := a.docs[a.docIndex(left)]
	if got := leftDoc.buf.String(); got != "one\nchanged\nthree\n" {
		t.Fatalf("left after Accept Right: %q", got)
	}
	a.saveDoc(leftDoc, nil)
	if data, err := os.ReadFile(left); err != nil || string(data) != "one\nchanged\nthree\n" {
		t.Fatalf("saved left: %q, %v", data, err)
	}
}

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestCrumbsOf(t *testing.T) {
	join := filepath.Join
	// The folders of this machine as it spells them, with backslashes on
	// Windows.
	w, u := filepath.FromSlash("/w"), filepath.FromSlash("/u")
	cases := []struct {
		path, root, home string
		want             []crumb
	}{
		{join(w, "src", "app", "main.go"), w, u,
			[]crumb{{"w", w}, {"src", join(w, "src")}, {"app", join(w, "src", "app")}, {"main.go", ""}}},
		{join(w, "main.go"), w, u, []crumb{{"w", w}, {"main.go", ""}}},
		{join(u, "notes", "a b", "中文.md"), w, u,
			[]crumb{{"~", u}, {"notes", join(u, "notes")}, {"a b", join(u, "notes", "a b")}, {"中文.md", ""}}},
		{join("/tmp", "FiveLeaf", "settings.json"), "", u,
			[]crumb{{"tmp", join("/tmp")}, {"FiveLeaf", join("/tmp", "FiveLeaf")}, {"settings.json", ""}}},
	}
	if remoteSupported {
		cases = append(cases, []struct {
			path, root, home string
			want             []crumb
		}{
			{"/ssh:box/home/me/x.go", w, u,
				[]crumb{{"box", "/ssh:box"}, {"home", "/ssh:box/home"}, {"me", "/ssh:box/home/me"}, {"x.go", ""}}},
			{"/ssh:box/srv/app/x.go", "/ssh:box/srv", u,
				[]crumb{{"srv", "/ssh:box/srv"}, {"app", "/ssh:box/srv/app"}, {"x.go", ""}}},
			{"/ssh:box/x.go", "/ssh:box", u,
				[]crumb{{"box", "/ssh:box"}, {"x.go", ""}}},
		}...)
	}
	for _, c := range cases {
		if got := crumbsOf(c.path, c.root, c.home); !slices.Equal(got, c.want) {
			t.Errorf("crumbsOf(%q, %q) = %v, want %v", c.path, c.root, got, c.want)
		}
	}
}

// crumbsHarness opens internal/a/a.go with the Explorer hidden, so that
// the breadcrumbs' names are the only ones on screen.
func crumbsHarness(t *testing.T) *harness {
	hs := newHarness(t, false)
	hs.a.openFile(hs.path("internal/a/a.go"), true)
	hs.settle()
	hs.a.run("view.sidebar")
	hs.settle()
	return hs
}

// A folder of the breadcrumbs shows its files in a box: its folders open
// in it, and a file chosen opens and closes the box, leaving the Explorer
// as it was.
func TestCrumbsBrowseAndOpen(t *testing.T) {
	hs := crumbsHarness(t)
	a, tt := hs.a, hs.tt
	explorerOpen := slices.Sorted(a.files.Open.All())
	if err := tt.Click(locales.CrumbsFolderTip(filepath.Base(a.root))); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !a.crumbs.open || a.crumbs.dir != a.root {
		t.Fatalf("no box for the project root: %+v", a.crumbs)
	}
	tt.Key(0, ui.KeyEscape)
	hs.settle()
	if err := tt.Click("internal"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !a.crumbs.open || a.crumbs.dir != hs.path("internal") {
		t.Fatalf("no box for internal: %+v", a.crumbs.open)
	}
	if !tt.HasText("a") || !tt.HasText("b") {
		t.Fatalf("the box does not list a and b: %v", tt.Texts())
	}
	// The folder on the way to the file is chosen, and has the keyboard.
	if a.crumbs.sel < 0 || a.crumbs.tree.Item(a.crumbs.sel) != hs.path("internal/a") {
		t.Errorf("the folder of the file is not chosen: row %d", a.crumbs.sel)
	}
	if err := tt.Click("b"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !tt.HasText("b.ts") {
		t.Fatalf("b does not open in the box: %v", tt.Texts())
	}
	if err := tt.Click("b.ts"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if got := a.activeTab().path(); got != hs.path("internal/b/b.ts") {
		t.Fatalf("the file shown is %s", got)
	}
	if a.crumbs.open {
		t.Error("the box stays open after a file is chosen")
	}
	if got := slices.Sorted(a.files.Open.All()); !slices.Equal(got, explorerOpen) {
		t.Errorf("the Explorer's open folders changed: %v, were %v", got, explorerOpen)
	}
}

// The box closes with Escape, and with a second click on its folder.
func TestCrumbsClose(t *testing.T) {
	hs := crumbsHarness(t)
	a, tt := hs.a, hs.tt
	tt.Click("internal")
	hs.settle()
	tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.crumbs.open {
		t.Error("Escape leaves the box open")
	}
	tt.Click("internal")
	hs.settle()
	if !a.crumbs.open {
		t.Fatal("no box")
	}
	tt.Click("internal")
	hs.settle()
	if a.crumbs.open {
		t.Error("a second click leaves the box open")
	}
	// A read that comes back after the box closed is dropped.
	a.openCrumbs(a.column(), a.activeTab(), hs.path("web"))
	a.closeCrumbs()
	hs.settle()
	if a.crumbs.kids != nil {
		t.Error("a read for a closed box was kept")
	}
}

// The keys move in the box: Up and Down choose, Right opens the folder
// chosen, Enter shows the file chosen.
func TestCrumbsKeys(t *testing.T) {
	hs := crumbsHarness(t)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("internal/b/b.ts"), true)
	hs.settle()
	tt.Click("internal")
	hs.settle()
	// b, on the way to the file, is chosen: Up goes to a.
	for _, k := range []ui.Key{ui.KeyUp, ui.KeyRight, ui.KeyDown} {
		tt.Key(0, k)
		hs.settle()
	}
	if a.crumbs.sel < 0 || a.crumbs.tree.Item(a.crumbs.sel) != hs.path("internal/a/a.go") {
		t.Fatalf("the keys did not reach a.go: row %d of %d", a.crumbs.sel, a.crumbs.tree.Rows())
	}
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if got := a.activeTab().path(); got != hs.path("internal/a/a.go") || a.crumbs.open {
		t.Errorf("Enter shows %s, the box open: %v", got, a.crumbs.open)
	}
}

// A file chosen goes to the column whose breadcrumbs opened the box, not
// to the one with the keyboard.
func TestCrumbsOpenInTheirColumn(t *testing.T) {
	hs := crumbsHarness(t)
	a, tt := hs.a, hs.tt
	a.run("view.splitRight")
	hs.settle()
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	if len(a.cols) != 2 || a.col != 1 {
		t.Fatalf("%d columns, the keyboard in %d", len(a.cols), a.col)
	}
	left := a.cols[0]
	a.openCrumbs(left, left.tab(), hs.path("internal"))
	hs.settle()
	tt.Click("b")
	hs.settle()
	tt.Click("b.ts")
	hs.settle()
	if left.tab().path() != hs.path("internal/b/b.ts") {
		t.Errorf("the left column shows %s", left.tab().path())
	}
	if a.cols[1].showingFile(hs.path("internal/b/b.ts")) >= 0 {
		t.Error("the file opened in the right column")
	}
}

// An empty folder and one that cannot be read say so, rather than show
// nothing, and the box goes as its column shows another file.
func TestCrumbsEmptyAndUnreadable(t *testing.T) {
	hs := crumbsHarness(t)
	a, tt := hs.a, hs.tt
	os.Mkdir(hs.path("internal/empty"), 0o755)
	tt.Click("internal")
	hs.settle()
	tt.Click("empty")
	hs.settle()
	if !tt.HasText(locales.CrumbsEmpty()) {
		t.Errorf("an empty folder does not say so: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	hs.settle()
	if err := os.Chmod(hs.path("internal/a"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(hs.path("internal/a"), 0o755) })
	if _, err := os.ReadDir(hs.path("internal/a")); err == nil {
		t.Skip("the folder reads without permission")
	}
	tt.Click("a")
	hs.settle()
	if !slices.ContainsFunc(tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "Could not read a") }) {
		t.Errorf("a folder that cannot be read does not say so: %v", tt.Texts())
	}
	os.Chmod(hs.path("internal/a"), 0o755)
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	if a.crumbs.open {
		t.Error("the box stays open over another file")
	}
}

func TestCrumbsFileCopiesPathDirectly(t *testing.T) {
	hs, d := editHarness(t, "中文 note.go", "package a\n\nfunc first() {}\n")
	r, ok := hs.tt.Find(locales.SymbolLabel())
	if !ok {
		t.Fatal("no filename button in the breadcrumbs")
	}
	for _, x := range []float32{r.X + 8, r.X + r.W - 8} {
		hs.tt.SetClipboard("before")
		hs.tt.RightClickAt(x, r.Y+r.H/2)
		hs.settle()
		if got := hs.tt.Clipboard(); got != d.path {
			t.Fatalf("copied %q, want %q", got, d.path)
		}
		if menu := hs.tt.Menu(); menu != nil {
			t.Fatalf("right-click opened a menu: %q", menu)
		}
		if hs.a.crumbs.open || d.sel != [2]int{} {
			t.Fatalf("right-click browsed or moved the caret: open=%v selection=%v", hs.a.crumbs.open, d.sel)
		}
	}
}

func TestCrumbsSymbolsJumpInTheirColumn(t *testing.T) {
	const source = "package a\n\nfunc first() {}\n\ntype second struct{}\n\nfunc (s second) 方法() {}\n"
	hs, d := editHarness(t, "symbols.go", source)
	a, tt := hs.a, hs.tt
	a.run("view.splitRight")
	hs.settle()
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	if len(a.cols) != 2 || a.col != 1 {
		t.Fatalf("%d columns, active column %d", len(a.cols), a.col)
	}
	right := a.activeEditor()
	rightSelection := right.sel
	a.documentSymbols()
	if err := tt.Click(locales.SymbolLabel()); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"first · 3", "second · 5", "方法 · 7"}; !slices.Equal(got, want) {
		t.Fatalf("symbols of the left file: %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("方法 · 7"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	at := len([]rune(source[:strings.Index(source, "方法")]))
	if a.col != 0 || a.activeEditor() != d || d.sel != [2]int{at, at + 2} {
		t.Fatalf("jumped in column %d to %v, want left file at %d", a.col, d.sel, at)
	}
	if right.sel != rightSelection {
		t.Errorf("the other column moved from %v to %v", rightSelection, right.sel)
	}
	if !tt.Focused("Editor symbols.go") {
		t.Error("the selected symbol's editor did not get the keyboard")
	}
}

func TestCrumbsSymbolsEmpty(t *testing.T) {
	hs, d := editHarness(t, "note.txt", "Plain text without declarations.\n")
	if err := hs.tt.Click(locales.SymbolLabel()); err != nil {
		t.Fatal(err)
	}
	if got, want := hs.tt.Menu(), []string{locales.SymbolNoMatches()}; !slices.Equal(got, want) {
		t.Fatalf("empty symbols menu: %q, want %q", got, want)
	}
	if err := hs.tt.ChooseMenuItem(locales.SymbolNoMatches()); err == nil {
		t.Fatal("the empty-state item is enabled")
	}
	hs.tt.CloseMenu()
	if err := hs.tt.RightClick(locales.SymbolLabel()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.tt.Clipboard() != d.path || hs.tt.Menu() != nil {
		t.Fatalf("a file without symbols cannot be copied directly: clipboard=%q menu=%q", hs.tt.Clipboard(), hs.tt.Menu())
	}
}

func TestCrumbsSymbolsFromLivePreview(t *testing.T) {
	hs, d := editHarness(t, "headings.md", "# Top\n\n## Target\n\nText.\n")
	hs.a.setMarkdownMode("live")
	hs.until("Markdown Live Preview", func() bool { return hs.a.livePreview(d) })
	hs.settle()
	if err := hs.tt.Click(locales.SymbolLabel()); err != nil {
		t.Fatal(err)
	}
	if err := hs.tt.ChooseMenuItem("Target · 3"); err != nil {
		t.Fatalf("%v: %q", err, hs.tt.Menu())
	}
	hs.settle()
	if d.buf.Snapshot().LineAt(d.sel[0]) != 2 {
		t.Fatalf("did not go to the heading: selection=%v", d.sel)
	}
	if !hs.tt.Focused("Editor headings.md") {
		t.Error("the heading's source editor did not get the keyboard")
	}
}

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func writeLayoutFile(t *testing.T, root, rel, text string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSearchSidebarFileLayouts(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.settings = settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	api := writeLayoutFile(t, hs.root, "api/v1/config.txt", "api needleview\n")
	web := writeLayoutFile(t, hs.root, "web/ui/config.txt", "web needleview\n")
	writeLayoutFile(t, hs.root, "00-root.txt", "root needleview\n")
	a.indexRead.Invalidate()
	hs.searchFor("needleview")
	s := &a.inFiles
	gen := s.gen
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	wantRows := func(want ...string) {
		t.Helper()
		var got []string
		for _, row := range a.searchRows() {
			switch {
			case row.folder != nil:
				got = append(got, row.folder.name+"/")
			case row.match < 0:
				got = append(got, row.file.rel)
			default:
				got = append(got, row.file.matches[row.match].pre+row.file.matches[row.match].hit)
			}
		}
		if !slices.Equal(got, want) || s.count != 3 || s.gen != gen {
			t.Fatalf("rows %q, matches %d, generation %d; want %q, 3, %d", got, s.count, s.gen, want, gen)
		}
	}
	wantRows("00-root.txt", "root needleview", "api/v1/config.txt", "api needleview", "web/ui/config.txt", "web needleview")
	click(locales.SidebarFolderTree())
	wantRows("api/v1/", "api/v1/config.txt", "api needleview", "web/ui/", "web/ui/config.txt", "web needleview", "00-root.txt", "root needleview")
	hs.shot("sidebar-search-tree")
	click("api/v1")
	s.input.Focus()
	hs.settle()
	hs.key(0, ui.KeyDown)
	if row := a.searchRows()[s.row]; row.file.path != web || row.match != 0 {
		t.Fatalf("Down after folding api/v1 chose %+v", row)
	}
	click(locales.SidebarFlatList())
	if row := a.searchRows()[s.row]; row.file.path != web || row.match != 0 {
		t.Fatalf("the selected match changed in flat layout: %+v", row)
	}
	click(locales.SidebarFolderTree())
	wantRows("api/v1/", "web/ui/", "web/ui/config.txt", "web needleview", "00-root.txt", "root needleview")
	click(locales.SearchCollapseAll())
	wantRows("api/v1/", "web/ui/", "00-root.txt")
	s.input.Focus()
	hs.settle()
	hs.key(0, ui.KeyDown)
	if !tt.Focused(locales.CmdViewSearch()) {
		t.Fatal("Down left the query without a visible match")
	}
	click(locales.SearchExpandAll())
	s.input.Focus()
	hs.settle()
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyEnter)
	doc := a.activeEditor()
	if doc == nil || doc.path != api || doc.buf.Slice(doc.sel[0], doc.sel[1]) != "needleview" {
		t.Fatal("Enter did not open the first visible match in api/v1/config.txt")
	}
	s.replacing, s.with = true, "updated"
	hs.settle()
	bounds, ok := tt.Find("web needleview")
	if !ok {
		t.Fatal("web match is missing")
	}
	tt.Move(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
	hs.settle()
	click(locales.SearchReplaceMatch())
	hs.until("replace the match in web/ui/config.txt", func() bool { return mustRead(t, web) == "web updated\n" })
	hs.settleSearch()
	if got := mustRead(t, web); got != "web updated\n" || mustRead(t, api) != "api needleview\n" {
		t.Fatalf("Replace changed the wrong file: web=%q", got)
	}
	a.settings.Flush()
	saved := settings.Load(a.settings.Path())
	if !saved.SearchTree || saved.ScmTree {
		t.Fatalf("saved search=%v, git=%v", saved.SearchTree, saved.ScmTree)
	}
}

func TestGitSidebarFileLayouts(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	run := hs.gitRepo()
	a.settings.SidebarRight = false
	first := writeLayoutFile(t, hs.root, "one/nested/config.txt", "staged\n")
	run("add", "one/nested/config.txt")
	writeLayoutFile(t, hs.root, "one/nested/config.txt", "staged\nplus working change\n")
	writeLayoutFile(t, hs.root, "two/nested/config.txt", "new file\n")
	a.showSourceControl()
	hs.settleGit()
	for _, width := range []float32{160, 340} {
		a.window.SidebarWidth = width
		hs.settle()
		fitsSidebar(t, tt, width, []control{
			{locales.SidebarFlatList(), iconLeast}, {locales.SidebarFolderTree(), iconLeast},
			{locales.CmdGitFetch(), iconLeast}, {locales.ScmRemoteActions(), iconLeast}, {locales.ScmMoreActions(), iconLeast},
		})
	}
	if err := tt.Click(locales.SidebarFolderTree()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !a.settings.ScmTree || a.settings.SearchTree {
		t.Fatal("Git and Search must remember their choices independently")
	}
	hs.shot("sidebar-git-tree")
	var stagedDir, workingDir scmRow
	for _, row := range a.scmRows() {
		if row.name != "one/nested" {
			continue
		}
		switch row.group {
		case groupStaged:
			stagedDir = row
		case groupChanges:
			workingDir = row
		}
	}
	if stagedDir.key == "" || workingDir.key == "" || stagedDir.key == workingDir.key {
		t.Fatal("staged and working directories lost their separate identities")
	}
	a.rootRepository().toggleDir(stagedDir)
	rows := a.scmRows()
	if slices.ContainsFunc(rows, func(row scmRow) bool { return row.group == groupStaged && row.e.Path == first }) || !slices.ContainsFunc(rows, func(row scmRow) bool { return row.group == groupChanges && row.e.Path == first }) {
		t.Fatal("folding the staged folder also changed the working folder")
	}
	if err := tt.Click(locales.SidebarFlatList()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if slices.ContainsFunc(a.scmRows(), func(row scmRow) bool { return row.dir != "" }) {
		t.Fatal("flat Git layout still contains directory rows")
	}
	if err := tt.Click(locales.SidebarFolderTree()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	a.rootRepository().toggleDir(workingDir)
	hs.settle()
	if err := tt.ClickWith(ui.Shift, "config.txt"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectScmSelection(t, hs, "two/nested/config.txt")
	hs.key(0, ui.KeySpace)
	hs.settleGit()
	if got := run("diff", "--cached", "--name-only"); got != "one/nested/config.txt\ntwo/nested/config.txt\n" {
		t.Fatalf("stage selected the wrong file: %q", got)
	}
}

func TestRepositorySidebarFileLayouts(t *testing.T) {
	hs, module, parent := submoduleHarness(t)
	a := hs.a
	hs.tt.SetSize(1200, 1000)
	a.window.ScmChangesRatio = 0.8
	writeLayoutFile(t, hs.root, "lib/core/parent.txt", "parent\n")
	file := writeLayoutFile(t, module.git.repo.Root, "lib/core/module.txt", "module\n")
	a.gitChanged()
	settleRepositories(t, hs, 2)
	if err := hs.tt.Click(locales.SidebarFolderTree()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	for _, repo := range []*gitContext{a.rootRepository(), module} {
		if !slices.ContainsFunc(repo.changesContext().scmRows(), func(row scmRow) bool { return row.name == "lib/core" }) {
			t.Fatalf("folder missing from %q", repo.label())
		}
	}
	if err := hs.tt.ClickWith(ui.Shift, "module.txt"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.key(0, ui.KeySpace)
	settleRepositories(t, hs, 2)
	staged := module.groupEntries()[groupStaged]
	if len(staged) != 1 || staged[0].Path != file || parent("diff", "--cached", "--name-only") != "" {
		t.Fatalf("tree action escaped the selected repository: %v", staged)
	}
	if err := hs.tt.Click(locales.SidebarFlatList()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if slices.ContainsFunc(module.changesContext().scmRows(), func(row scmRow) bool { return row.dir != "" }) {
		t.Fatal("module did not switch back to flat layout")
	}
}

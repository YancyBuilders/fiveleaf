package main

import (
	"slices"
	"strings"
	"testing"

	"fiveleaf/internal/git"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func graphHistoryFixture(hs *harness) func(...string) string {
	run := hs.gitRepo()
	run("switch", "-q", "-c", "feature/graph")
	run("commit", "-q", "--allow-empty", "--author=Mina <mina@example.org>", "-m", "Draw merge lanes")
	run("update-ref", "refs/remotes/origin/review", "HEAD")
	run("commit", "-q", "--allow-empty", "--author=Mina <mina@example.org>", "-m", "Label branch tips")
	run("switch", "-q", "main")
	run("commit", "-q", "--allow-empty", "--author=Kai <kai@example.org>", "-m", "Keep the selected commit")
	run("merge", "-q", "--no-ff", "refs/remotes/origin/review", "-m", "Merge graph rendering", "-m", "Connect the history rows across both branches.")
	run("tag", "v1.0", "HEAD")
	hs.a.showSourceControl()
	hs.settleGit()
	return run
}

func TestCommitGraphSidebarAndFilter(t *testing.T) {
	hs := newHarness(t, true)
	run := graphHistoryFixture(hs)
	run("tag", "feature/graph", "HEAD")
	app := hs.a
	if hs.tt.HasText(app.pal.title(locales.GroupCommits())) || hs.tt.HasText("Merge graph rendering") {
		t.Fatal("the commit graph is still rendered in the Git sidebar")
	}
	click := func(label string) {
		t.Helper()
		if err := hs.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	click(locales.CmdGitOpenGraph())
	tab := app.activeTab()
	if tab == nil || tab.tool == nil || tab.tool.graph == nil {
		t.Fatal("the source control button did not open an editor graph")
	}
	graph := tab.tool.graph
	if graph.root != hs.root || len(graph.commits) != 5 || graph.lanes < 2 || !hs.tt.HasText(locales.CommitGraphAuthor()) {
		t.Fatalf("graph for %s has %d commits and %d lanes: %v", graph.root, len(graph.commits), graph.lanes, hs.tt.Texts())
	}
	app.run("git.openGraph")
	if app.activeTab() != tab {
		t.Fatal("opening the same repository's graph replaced its tab")
	}
	checkFilter := func(query, ref string, count int) {
		t.Helper()
		click(locales.CommitGraphFilter())
		hs.tt.Type(query)
		hs.tt.Key(0, ui.KeyEnter)
		hs.settle()
		if graph.ref != ref || len(graph.commits) != count || graph.err != nil {
			t.Fatalf("filter %q: ref %q, %d commits, error %v", query, graph.ref, len(graph.commits), graph.err)
		}
	}
	checkFilter("feature/graph", "refs/heads/feature/graph", 3)
	if graph.commits[0].Subject != "Label branch tips" {
		t.Fatalf("the branch/tag collision resolved to %s", graph.commits[0].Subject)
	}
	checkFilter("origin/review", "refs/remotes/origin/review", 2)
	if graph.commits[0].Subject != "Draw merge lanes" || strings.TrimSpace(run("symbolic-ref", "HEAD")) != "refs/heads/main" {
		t.Fatal("the remote filter did not show its own tip and preserve the checkout")
	}
	checkFilter(locales.PrefTitleScmAllBranches(), "", 5)
	hs.shot("commit-graph")
	app.window.SidebarClosed = true
	hs.tt.SetSize(360, 540)
	hs.settle()
	if hs.tt.HasText(locales.CommitGraphAuthor()) || !hs.tt.HasText(locales.CommitGraphLanes()) || !hs.tt.HasText("Label branch tips") {
		t.Fatal("the narrow editor did not keep the lanes and subjects in view")
	}
	click("Label branch tips")
	if graph.selected < 0 || graph.commits[graph.selected].Subject != "Label branch tips" {
		t.Fatal("selecting a graph row did not select its details")
	}
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	diff := app.activeTab().diff
	if diff == nil || diff.repoRoot != hs.root || diff.commit.key() != strings.TrimSpace(run("rev-parse", "refs/heads/feature/graph")) {
		t.Fatal("Enter on a graph row did not open its commit diff")
	}
}

func TestCommitGraphRepositoryAndSplit(t *testing.T) {
	hs, sub, _ := submoduleHarness(t)
	app := hs.a
	app.selectRepository(sub.git.repo.Root)
	hs.settle()
	if hs.tt.HasText(app.pal.title(locales.GroupCommits())) || hs.tt.HasText("module first") || hs.tt.HasText(locales.ScmResizeChanges()) {
		t.Fatal("the multi-repository sidebar still reserves a pane for the commit graph")
	}
	if err := hs.tt.Click(locales.CmdGitOpenGraph()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	subTab := app.activeTab()
	graph := subTab.tool.graph
	if graph.root != sub.git.repo.Root || len(graph.commits) != 1 || graph.commits[0].Subject != "module first" {
		t.Fatal("multi-repository graph did not follow the repository picker")
	}
	graph.selectRef("refs/heads/main")
	app.splitRight()
	hs.settle()
	split := app.activeTab().tool.graph
	if split == graph || split.root != graph.root || split.ref != graph.ref {
		t.Fatal("the split lost the repository or did not keep an independent filter")
	}
	split.selectRef("")
	if graph.ref == split.ref {
		t.Fatal("filtering the split also filtered the original graph")
	}
	app.selectRepository("")
	if app.repository() != sub {
		t.Fatal("Auto did not follow the active graph's repository")
	}
	app.selectRepository(app.git.repo.Root)
	hs.settle()
	if err := hs.tt.Click(locales.CmdGitOpenChanges()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if diff := app.activeTab().diff; diff == nil || diff.repoRoot != sub.git.repo.Root {
		t.Fatal("the details button followed the sidebar instead of the graph")
	}
	app.run("git.openGraph")
	parentTab := app.activeTab()
	dest, _ := app.locate(subTab)
	app.moveTab(parentTab, dest, len(dest.tabs))
	if app.activeTab() != parentTab || !slices.Contains(dest.tabs, subTab) || parentTab.tool.graph.root != app.git.repo.Root {
		t.Fatal("moving the parent graph onto the module graph lost a repository tab")
	}
}

func TestCommitGraphEmptyPagingAndRefresh(t *testing.T) {
	hs := newHarness(t, true)
	gitIn(t, hs.root, "init", "-q", "-b", "main")
	hs.a.openFolder(hs.root)
	hs.settleGit()
	hs.a.run("git.openGraph")
	hs.settle()
	graph := hs.a.activeTab().tool.graph
	if !hs.tt.HasText(locales.PanelNoCommits()) || graph.err != nil {
		t.Fatalf("empty repository: %v", graph.err)
	}
	for _, subject := range []string{"first", "second", "third"} {
		gitIn(t, hs.root, "commit", "-q", "--allow-empty", "-m", subject)
	}
	graph.limit = 2
	if err := hs.tt.Click(locales.Refresh()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !graph.more || len(graph.commits) != 2 {
		t.Fatal("refresh did not load a bounded page")
	}
	if err := hs.tt.Click(locales.ScmShowMoreCommits()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if graph.more || len(graph.commits) != 3 {
		t.Fatal("the final history page is incomplete")
	}
	ctx := hs.a.repository()
	graph.selectRef("refs/heads/missing")
	graph.load(ctx)
	graph.selectRef("refs/heads/main")
	graph.load(ctx)
	hs.settle()
	if graph.err != nil || !slices.ContainsFunc(graph.commits, func(commit git.Commit) bool { return commit.Subject == "third" }) {
		t.Fatalf("a stale filter replaced the latest history: %v", graph.err)
	}
	graph.selectRef("refs/heads/missing")
	hs.settle()
	if graph.err == nil || !hs.tt.HasText(locales.PluginsIndexRetry()) {
		t.Fatal("a deleted branch did not show a recoverable error")
	}
	gitIn(t, hs.root, "branch", "missing")
	if err := hs.tt.Click(locales.PluginsIndexRetry()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if graph.err != nil || len(graph.commits) != 3 {
		t.Fatalf("retry did not recover the branch: %v", graph.err)
	}
}

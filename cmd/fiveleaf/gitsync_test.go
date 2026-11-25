package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func checkFetchCounts(t *testing.T, tt *ui.Tester, behind, ahead string) {
	t.Helper()
	fetch, ok := tt.Find(locales.CmdGitFetch())
	if !ok {
		t.Fatal("missing Fetch button")
	}
	left := fetch.X
	for _, text := range []string{behind, ahead} {
		bounds, ok := tt.Find(text)
		if !ok || bounds.W <= 0 || bounds.X < left || bounds.X+bounds.W > fetch.X+fetch.W || bounds.Y < fetch.Y || bounds.Y+bounds.H > fetch.Y+fetch.H {
			t.Fatalf("%q at %v (shown %v) is not in order inside Fetch %v", text, bounds, ok, fetch)
		}
		left = bounds.X + bounds.W
	}
}

func TestGitSyncCountsInFetchButton(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs := newHarness(t, false)
	run := hs.gitRepo()
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	run("remote", "add", "origin", remote)
	run("push", "-qu", "origin", "main")
	run("branch", "-m", "feature_T12436745")
	for i := range 8 {
		run("commit", "-q", "--allow-empty", "-m", fmt.Sprintf("local %d", i))
	}
	hs.settleGit()
	a, tt := hs.a, hs.tt
	ctx := a.repository()
	if ctx.branch() != "feature_T12436745" || ctx.git.status.Ahead != 8 {
		t.Fatalf("branch/counts: %q, %+v", ctx.branch(), ctx.git.status)
	}
	if _, ok := tt.Find("Synchronize Changes"); ok {
		t.Fatalf("the status bar still shows Git sync: %q", tt.Texts())
	}
	if r, ok := tt.Find(locales.CmdGitFetch()); ok && r.Y < titleBarH {
		t.Fatalf("the title bar still has the Fetch action at %v", r)
	}
	openSCM(hs)
	checkFetchCounts(t, tt, "↓0", "↑8")
	peer := filepath.Join(t.TempDir(), "peer")
	gitIn(t, filepath.Dir(peer), "clone", "-q", remote, peer)
	gitIn(t, peer, "commit", "-q", "--allow-empty", "-m", "remote change")
	gitIn(t, peer, "push", "-q")
	if err := tt.Click("↑8"); err != nil {
		t.Fatal(err)
	}
	hs.settleGit()
	if len(a.errors) > 0 || a.git.pending != 0 || ctx.git.status.Behind != 1 || ctx.git.status.Ahead != 8 {
		t.Fatalf("clicking the counts did not fetch the new remote commit: %+v, errors %q", ctx.git.status, a.errors)
	}
	checkFetchCounts(t, tt, "↓1", "↑8")
	hs.shot("scm-fetch-counts")
	a.openFile(hs.path("main.go"), false)
	const narrow = 220
	tt.SetSize(narrow, 300)
	hs.settle()
	if b, ok := tt.Find(locales.StatusPos(1, 1)); !ok || b.X < 0 || b.X+b.W > narrow {
		t.Fatalf("narrow caret position is clipped: %v %v", b, ok)
	}
	tt.SetSize(1200, 760)
	hs.settleGit()
}

func pushFromSCM(t *testing.T, hs *harness) {
	t.Helper()
	// A narrow row moves push into the repository menu; a wide one keeps
	// it beside the branch.
	if err := hs.tt.Click(locales.ScmPushCount(8)); err != nil {
		if err := hs.tt.Click(locales.ScmMoreActions()); err != nil {
			t.Fatal(err)
		}
		if err := hs.tt.ChooseMenuItem(locales.CmdGitPush()); err != nil {
			t.Fatal(err)
		}
	}
	hs.settle()
	if hs.a.git.pending == 0 && hs.a.git.status.Ahead == 8 {
		hs.a.push()
		hs.settleGit()
	}
}

func pullFromSCM(t *testing.T, hs *harness, count int) {
	t.Helper()
	if err := hs.tt.Click(locales.ScmPullCount(count)); err != nil {
		if err := hs.tt.Click(locales.ScmMoreActions()); err != nil {
			t.Fatal(err)
		}
		if err := hs.tt.ChooseMenuItem(locales.CmdGitPull()); err != nil {
			t.Fatal(err)
		}
	}
	hs.settle()
}

func TestGitPushPullFromSourceControl(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs := newHarness(t, true)
	run := hs.gitRepo()
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	run("remote", "add", "origin", remote)
	run("push", "-qu", "origin", "main")
	for i := range 8 {
		run("commit", "-q", "--allow-empty", "-m", fmt.Sprintf("local %d", i))
	}
	hs.settleGit()
	a, tt := hs.a, hs.tt
	a.showSourceControl()
	hs.settle()
	pushFromSCM(t, hs)
	hs.settleGit()
	if a.git.status.Ahead != 0 || a.git.status.Behind != 0 || strings.TrimSpace(gitIn(t, remote, "rev-parse", "main")) != strings.TrimSpace(run("rev-parse", "HEAD")) {
		t.Fatalf("push did not send local commits: %+v; errors: %q", a.git.status, a.errors)
	}
	if !tt.HasText(locales.PushedTo("main", "origin/main")) {
		t.Fatalf("push result not shown: %q", tt.Texts())
	}
	peer := filepath.Join(t.TempDir(), "peer")
	gitIn(t, filepath.Dir(peer), "clone", "-q", remote, peer)
	gitIn(t, peer, "commit", "-q", "--allow-empty", "-m", "remote change")
	gitIn(t, peer, "push", "-q")
	if out := gitIn(t, remote, "log", "--format=%H %s", "-3", "main"); !strings.Contains(out, "local 7") {
		t.Fatalf("the remote did not receive the pushed commits:\n%s", out)
	}
	pullFromSCM(t, hs, 1)
	hs.settleGit()
	if strings.TrimSpace(run("rev-parse", "HEAD")) != strings.TrimSpace(gitIn(t, peer, "rev-parse", "HEAD")) {
		t.Fatal("pulling at 0/0 did not bring in new remote commits")
	}
}

func TestGitPublishFromSourceControl(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs := newHarness(t, true)
	hs.a.showSourceControl()
	hs.settle()
	assertNoSync := func() {
		t.Helper()
		if _, ok := hs.tt.Find("Synchronize Changes"); ok {
			t.Fatal("the status bar still shows Git sync")
		}
	}
	assertNoSync()
	gitIn(t, hs.root, "init", "-q", "-b", "main")
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	gitIn(t, hs.root, "remote", "add", "origin", remote)
	hs.a.openFolder(hs.root)
	hs.settleGit()
	assertNoSync()
	gitIn(t, hs.root, "add", ".")
	gitIn(t, hs.root, "commit", "-q", "-m", "first")
	hs.settleGit()
	if err := hs.tt.Click(locales.ScmPublishBranch()); err != nil {
		t.Fatal(err)
	}
	hs.settleGit()
	if hs.a.git.status.Upstream != "origin/main" || strings.TrimSpace(gitIn(t, remote, "rev-parse", "main")) != strings.TrimSpace(gitIn(t, hs.root, "rev-parse", "HEAD")) {
		t.Fatalf("publish did not establish upstream: %+v", hs.a.git.status)
	}
	gitIn(t, hs.root, "checkout", "-q", "--detach")
	hs.settleGit()
	assertNoSync()
	gitIn(t, hs.root, "switch", "-q", "main")
	gitIn(t, hs.root, "remote", "remove", "origin")
	hs.settleGit()
	assertNoSync()
}

func TestGitSyncFailureReleasesBusyState(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs := newHarness(t, true)
	run := hs.gitRepo()
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	run("remote", "add", "origin", remote)
	run("push", "-qu", "origin", "main")
	run("commit", "-q", "--allow-empty", "-m", "pending push")
	run("remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	hs.settleGit()
	hs.a.showSourceControl()
	hs.settle()
	pushFromSCM(t, hs)
	hs.settleGit()
	if hs.a.fails == 0 || hs.a.git.pending != 0 || hs.a.git.busy != "" || hs.a.git.status.Ahead != 1 {
		t.Fatalf("failure not reported or state not released: fails=%d pending=%d busy=%q status=%+v", hs.a.fails, hs.a.git.pending, hs.a.git.busy, hs.a.git.status)
	}
	run("remote", "set-url", "origin", remote)
	pushFromSCM(t, hs)
	hs.settleGit()
	if hs.a.git.status.Ahead != 0 {
		t.Fatalf("push could not retry after failure: %+v", hs.a.git.status)
	}
}

func TestGitSyncKeepsSelectedRepository(t *testing.T) {
	hs, sub, parent := submoduleHarness(t)
	root := sub.git.repo.Root
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	gitIn(t, root, "switch", "-q", "-c", "sync-topic")
	gitIn(t, root, "remote", "set-url", "origin", remote)
	gitIn(t, root, "push", "-qu", "origin", "sync-topic")
	gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "module sync")
	hs.a.gitChanged()
	settleRepositories(t, hs, 2)
	hs.a.selectRepository(root)
	hs.settle()
	before := strings.TrimSpace(parent("rev-parse", "HEAD"))
	hs.a.repository().push()
	hs.a.selectRepository(hs.a.git.repo.Root)
	settleRepositories(t, hs, 2)
	if strings.TrimSpace(gitIn(t, remote, "rev-parse", "sync-topic")) != strings.TrimSpace(gitIn(t, root, "rev-parse", "HEAD")) {
		t.Fatal("repository switch redirected the pending push")
	}
	if strings.TrimSpace(parent("rev-parse", "HEAD")) != before || hs.a.git.pending != 0 || sub.git.pending != 0 || sub.git.busy != "" {
		t.Fatal("module push changed parent or leaked pending state")
	}
}

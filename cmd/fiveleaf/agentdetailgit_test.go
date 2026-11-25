package main

import (
	"os"
	"path/filepath"
	"testing"

	"fiveleaf/internal/git"
)

func TestChatChangeTotalsRefreshesEqualStatus(t *testing.T) {
	hs := newHarness(t, true)
	command := hs.gitRepo()
	path := hs.path("lines")
	writeFile(t, path, []byte("seed\n"))
	command("add", "lines")
	command("commit", "-qm", "base")
	writeFile(t, path, []byte("seed\nab\n"))
	hs.settleGit()
	ctx := hs.a.repository()
	if ctx.git.chatTotals != nil {
		t.Fatal("ordinary SCM refresh started chat statistics")
	}
	check := func(want int) {
		t.Helper()
		if added, removed, ready, err := ctx.chatChangeTotals(); ready || err != nil || added != 0 || removed != 0 {
			t.Fatalf("first frame = +%d -%d, ready=%t, %v", added, removed, ready, err)
		}
		job := ctx.git.chatTotals
		hs.until("chat totals", func() bool {
			added, removed, ready, err := ctx.chatChangeTotals()
			if err != nil || ready && (added != want || removed != 0) {
				t.Fatalf("totals = +%d -%d, %v; want +%d -0", added, removed, err, want)
			}
			return ready
		})
		for range 10 {
			added, _, ready, err := ctx.chatChangeTotals()
			if ctx.git.chatTotals != job || !ready || added != want || err != nil {
				t.Fatal("subsequent frames did not reuse the result")
			}
		}
	}
	check(1)
	oldStatus := ctx.git.status
	writeFile(t, path, []byte("seed\na\n\n"))
	hs.settleGit()
	if ctx.git.status != oldStatus {
		t.Fatal("expected SCM to reuse the same modified status")
	}
	check(2)
}

func TestChatChangeTotalsDropsOldWorkspaceResult(t *testing.T) {
	window := &app{}
	open := func(text string) *gitContext {
		t.Helper()
		root := t.TempDir()
		gitIn(t, root, "init", "-q")
		repository, err := git.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, repository.Abs("lines"), []byte(text))
		window.resetRepositories()
		window.git = gitState{repo: repository}
		ctx := window.rootRepository()
		if _, _, ready, err := ctx.chatChangeTotals(); ready || err != nil || ctx.git.chatTotals != nil {
			t.Fatal("missing status started a job or returned totals")
		}
		status, err := repository.Status()
		if err != nil {
			t.Fatal(err)
		}
		ctx.setStatus(status)
		if _, _, ready, err := ctx.chatChangeTotals(); ready || err != nil {
			t.Fatal("expected a background read")
		}
		return ctx
	}
	takeResult := func() func() {
		t.Helper()
		waitGitPost(t, window)
		window.mu.Lock()
		defer window.mu.Unlock()
		post := window.queue[0]
		window.queue = window.queue[1:]
		return post
	}
	old := open("one\n")
	oldResult := takeResult()
	current := open("one\ntwo\nthree\n")
	newResult := takeResult()
	oldResult()
	if _, _, ready, err := old.chatChangeTotals(); ready || err != nil {
		t.Fatal("a stale controller returned totals")
	}
	if _, _, ready, err := current.chatChangeTotals(); ready || err != nil {
		t.Fatal("an old result completed the new workspace's read")
	}
	newResult()
	if added, removed, ready, err := current.chatChangeTotals(); !ready || err != nil || added != 3 || removed != 0 {
		t.Fatalf("new workspace = +%d -%d, ready=%t, %v", added, removed, ready, err)
	}
}

func TestChatChangeTotalsStatusErrorAndRecovery(t *testing.T) {
	hs := newHarness(t, true)
	hs.gitRepo()
	ctx := hs.a.repository()
	index := filepath.Join(ctx.git.repo.Dir, "index")
	healthy, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	refresh := func() {
		t.Helper()
		ctx.git.statusRead.Invalidate()
		ctx.refreshGit()
		hs.until("SCM read", func() bool { return !ctx.git.statusRead.Running() && !ctx.git.statusRead.Stale() })
	}
	checkFailure := func() {
		t.Helper()
		writeFile(t, index, []byte("invalid Git index"))
		refresh()
		if added, removed, ready, err := ctx.chatChangeTotals(); ready || err == nil || added != 0 || removed != 0 {
			t.Fatalf("failed status = +%d -%d, ready=%t, %v", added, removed, ready, err)
		}
	}
	ctx.git.status = nil
	checkFailure()
	for count := 1; count <= 2; count++ {
		writeFile(t, index, healthy)
		writeFile(t, hs.path("lines"), []byte("a\nb\n")[:2*count])
		refresh()
		hs.until("recovered chat totals", func() bool {
			added, removed, ready, err := ctx.chatChangeTotals()
			if err != nil || ready && (added != count || removed != 0) {
				t.Fatalf("recovered status = +%d -%d, %v; want +%d -0", added, removed, err, count)
			}
			return ready
		})
		if count == 1 {
			checkFailure()
		}
	}
}

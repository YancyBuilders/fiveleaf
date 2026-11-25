package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/git"
	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

// worktreeFixture is the harness's repository with a linked worktree
// named side, on a branch of the same name, and the box of the worktrees
// open on it.
func worktreeFixture(t *testing.T) (*harness, func(args ...string) string, string) {
	t.Helper()
	hs := newHarness(t, true)
	run := hs.gitRepo()
	side := filepath.Join(t.TempDir(), "side")
	run("worktree", "add", "-b", "side", side)
	return hs, run, side
}

// openWorktrees clicks the worktree button of the window's folder in the
// title bar and waits until the worktrees are read.
func openWorktrees(t *testing.T, hs *harness) {
	t.Helper()
	if err := hs.tt.Click(locales.WorktreeStatusTip(hs.a.repository().worktreeLabel())); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	awaitWorktrees(t, hs)
}

// awaitWorktrees draws frames until the worktrees the box opened on are read.
func awaitWorktrees(t *testing.T, hs *harness) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for hs.a.worktrees.loading && time.Now().Before(deadline) {
		hs.settle()
	}
	if hs.a.worktrees.loading {
		t.Fatal("the worktree box never finished reading the worktrees")
	}
}

// The worktree box opens from the title bar, shuts on Escape, and lets go of
// the worktrees it read.
func TestTitleBarWorktreeBox(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	openWorktrees(t, hs)
	if !a.worktrees.open || len(a.worktrees.trees) != 2 {
		t.Fatalf("box open %v with %d worktrees", a.worktrees.open, len(a.worktrees.trees))
	}
	// The one open comes first, and has its check; the other is by its folder.
	if first := a.worktrees.trees[0]; !first.Current || first.Branch != "main" {
		t.Fatalf("first worktree %+v is not the one open", first)
	}
	if !hs.tt.HasText("side") {
		t.Fatal("box does not name the linked worktree")
	}
	// Escape shuts the box and lets go of what it read.
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.worktrees.open || a.worktrees.trees != nil || a.worktrees.names != nil {
		t.Fatal("closed box kept its worktrees")
	}
}

func TestWorktreeBoxSwitchesToLinked(t *testing.T) {
	hs, run, side := worktreeFixture(t)
	openWorktrees(t, hs)
	if err := hs.tt.Click("side"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	want, err := filepath.EvalSymlinks(side)
	if err != nil {
		t.Fatal(err)
	}
	if hs.a.root != want {
		t.Fatalf("window shows %q, want %q", hs.a.root, want)
	}
	if strings.TrimSpace(run("symbolic-ref", "--short", "HEAD")) != "main" || hs.a.fails != 0 {
		t.Fatalf("switching changed the branch or failed %d times", hs.a.fails)
	}
	// The title bar names the linked folder it is in.
	if !hs.tt.HasText(filepath.Base(want)) {
		t.Fatal("title bar does not name the linked worktree")
	}
}

func TestWorktreeBoxCurrentOnlyCloses(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	root := a.root
	openWorktrees(t, hs)
	// Enter on the first row, the one open, shuts the box and does nothing else.
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	if a.worktrees.open || a.root != root || a.fails != 0 {
		t.Fatalf("choosing the open worktree: open %v root %q failures %d", a.worktrees.open, a.root, a.fails)
	}
}

func TestWorktreeBoxMissingFolderIsReported(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	gone := filepath.Join(t.TempDir(), "gone")
	run("worktree", "add", "-b", "gone", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	root, fails := a.root, a.fails
	openWorktrees(t, hs)
	if err := hs.tt.Click("gone"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	// The failure is shown as a message, which the frames clear: it counts.
	if a.root != root || a.fails != fails+1 {
		t.Fatalf("missing folder: root %q, failures %d before %d", a.root, a.fails, fails)
	}
	// Nothing was pruned: Git still lists the worktree, and the branch
	// stays where it was.
	if list := run("worktree", "list"); !strings.Contains(list, "gone") {
		t.Fatalf("worktree list lost the missing worktree:\n%s", list)
	}
	if strings.TrimSpace(run("symbolic-ref", "--short", "HEAD")) != "main" {
		t.Fatal("missing folder changed the branch")
	}
}

func TestWorktreeBoxFuzzyAndCreate(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	openWorktrees(t, hs)
	// A query naming an existing worktree offers no creation.
	hs.tt.Type("side")
	hs.settle()
	for _, ch := range a.repository().worktreeChoices() {
		if ch.create != "" {
			t.Fatal("offered to create a worktree named like an existing one")
		}
	}
	if len(a.repository().worktreeChoices()) == 0 {
		t.Fatal("query side matched no worktree")
	}
	hs.tt.Key(ui.Cmd, ui.KeyA)
	hs.tt.Type("feature/new")
	hs.settle()
	if err := hs.tt.Click(locales.WorktreeCreateLabel("feature/new")); err != nil {
		t.Fatal(err)
	}
	hs.settleGit()
	if a.worktrees.open {
		t.Fatal("box still open after creating")
	}
	if !slices.Contains(strings.Fields(run("branch", "--format=%(refname:short)")), "feature/new") {
		t.Fatal("branch feature/new was not made")
	}
	if !strings.HasSuffix(a.root, filepath.Join(".worktrees", "feature-new")) {
		t.Fatalf("window shows %q, not the new worktree folder", a.root)
	}
	if strings.TrimSpace(run("rev-parse", "HEAD")) != strings.TrimSpace(run("rev-parse", "feature/new")) {
		t.Fatal("new worktree does not start from the commit HEAD was at")
	}
}

func TestWorktreeBoxCreatePrompt(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	before := strings.TrimSpace(run("rev-parse", "HEAD"))
	openWorktrees(t, hs)
	if !hs.tt.HasText(locales.WorktreeCreateEllipsis()) || !hs.tt.HasText(locales.QuickSwitchAction()) {
		t.Fatal("the empty query lacks a create entry or no longer defaults to the current worktree")
	}
	hs.tt.Key(0, ui.KeyUp)
	hs.settle()
	if !hs.tt.HasText(locales.QuickCreateAction()) {
		t.Fatal("the create entry does not say what Enter will do")
	}
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	if a.worktrees.open || a.prompt == nil || a.prompt.hint != locales.WorktreeFrom("main ("+git.ShortHash(before)+")") {
		t.Fatal("the create entry did not ask for a worktree name at the opening commit")
	}
	for _, tc := range []struct{ value, want string }{
		{"bad..name", locales.BranchInvalidName()},
		{"main", locales.WorktreeNameTaken("main")},
		{"side", locales.WorktreeNameTaken("side")},
	} {
		hs.tt.Key(ui.Cmd, ui.KeyA)
		hs.tt.Type(tc.value)
		hs.tt.Key(0, ui.KeyEnter)
		hs.settle()
		if a.prompt == nil || a.prompt.err != tc.want {
			t.Fatalf("name %q did not retain the prompt with error %q", tc.value, tc.want)
		}
	}
	// The prompt must retain its original base even if Git moves before submit.
	run("commit", "-q", "--allow-empty", "-m", "moved after opening")
	a.gitChanged()
	hs.settleGit()
	hs.tt.Key(ui.Cmd, ui.KeyA)
	hs.tt.Type("feature/from-prompt")
	hs.tt.Key(0, ui.KeyEnter)
	hs.settleGit()
	if a.prompt != nil || a.worktrees.open || !strings.HasSuffix(a.root, filepath.Join(".worktrees", "feature-from-prompt")) {
		t.Fatalf("the prompt did not create and open its worktree: root %q", a.root)
	}
	if got := strings.TrimSpace(run("rev-parse", "feature/from-prompt")); got != before {
		t.Fatalf("new worktree starts at %s, want the opening commit %s", got, before)
	}
}

func TestWorktreeCreatePromptRetainsRepository(t *testing.T) {
	hs, run, side := worktreeFixture(t)
	openWorktrees(t, hs)
	if err := hs.tt.Click(locales.WorktreeCreateEllipsis()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.prompt == nil {
		t.Fatal("no create prompt")
	}
	done := hs.a.prompt.done
	hs.a.switchFolder(side)
	hs.settleGit()
	if got := done("stale-prompt"); got != locales.PanelRepoChanged() {
		t.Fatalf("stale prompt returned %q, want repository-change error", got)
	}
	if got := strings.TrimSpace(run("branch", "--list", "stale-prompt")); got != "" {
		t.Fatal("a stale prompt created a branch after the window changed repositories")
	}
}

func TestWorktreeCreatePromptWaitsForGit(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	openWorktrees(t, hs)
	if err := hs.tt.Click(locales.WorktreeCreateEllipsis()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.prompt == nil {
		t.Fatal("no create prompt")
	}
	hs.a.git.busy = "held operation"
	if got := hs.a.prompt.done("wait-for-git"); got != "held operation" {
		t.Fatalf("busy prompt returned %q", got)
	}
	hs.a.git.busy = ""
	hs.a.git.status.Op = git.OpMerge
	if got := hs.a.prompt.done("wait-for-git"); got != locales.ScmOpInProgress(opTitle(git.OpMerge)) {
		t.Fatalf("prompt during merge returned %q", got)
	}
	hs.a.git.status.Op = ""
}

func TestWorktreeBoxBusyDisabled(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	hs.a.git.busy = "held operation"
	hs.settle()
	if err := hs.tt.Click(locales.WorktreeStatusTip("main")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if hs.a.worktrees.open {
		t.Fatal("busy repository opened the worktree box")
	}
	hs.a.git.busy = ""
}

func TestWorktreeBoxStaysWithItsRepository(t *testing.T) {
	hs, sub, _ := submoduleHarness(t)
	a := hs.a
	parentRoot := a.repository().git.repo.Root
	if err := hs.tt.Click(locales.WorktreeStatusTip("main")); err != nil {
		t.Fatal(err)
	}
	// Selecting the module before the worktrees arrive must not move the
	// box to it: its results belong to the repository it opened on.
	a.selectRepository(sub.git.repo.Root)
	hs.settle()
	awaitWorktrees(t, hs)
	if a.worktrees.repoRoot != parentRoot || len(a.worktrees.trees) != 1 {
		t.Fatalf("box of %q has %d worktrees", a.worktrees.repoRoot, len(a.worktrees.trees))
	}
}

// Opening and shutting the box, and switching between the worktrees, many
// times leaves no goroutine and no heap behind: what the box read is let go
// when it shuts.
func TestWorktreeBoxDoesNotLeak(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	cycle := func(n int) {
		for range n {
			openWorktrees(t, hs)
			hs.tt.Key(0, ui.KeyEscape)
			hs.settle()
		}
	}
	// Warm up what the first use of the box allocates once.
	cycle(2)
	base, baseHeap := settledGoroutines(hs), heapInUse()
	cycle(50)
	main := mustEval(t, hs.root)
	for range 5 {
		openWorktrees(t, hs)
		if err := hs.tt.Click("side"); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		openWorktrees(t, hs)
		if err := hs.tt.Click(filepath.Base(hs.root)); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	if a.root != main {
		t.Fatalf("switching back did not return to the main worktree: %q", a.root)
	}
	got := settledGoroutines(hs)
	grown := int64(heapInUse()) - int64(baseHeap)
	t.Logf("goroutines %d before, %d after 55 box cycles; heap in use %+d KB", base, got, grown>>10)
	if got > base {
		t.Fatalf("goroutines %d after the cycles, %d before", got, base)
	}
	if grown > 8<<20 {
		t.Fatalf("heap grew by %d KB over the cycles", grown>>10)
	}
	if a.worktrees.trees != nil || a.worktrees.names != nil {
		t.Fatal("closed box kept what it read")
	}
}

// settledGoroutines draws frames until background work ends, and reports
// how many goroutines run then.
func settledGoroutines(hs *harness) int {
	for range 20 {
		hs.settle()
	}
	time.Sleep(50 * time.Millisecond)
	return runtime.NumGoroutine()
}

// heapInUse collects garbage, and reports the heap in use after it.
func heapInUse() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Reading a worktree's folder holds no git command busy, and a switch made
// before the reading ends wins: the reading does not switch the window back.
func TestWorktreeOpenHoldsNoGit(t *testing.T) {
	hs, run, side := worktreeFixture(t)
	a := hs.a
	other := filepath.Join(t.TempDir(), "other")
	run("worktree", "add", "-b", "other", other)
	pending := a.git.pending
	a.repository().openWorktree(side)
	if a.git.busy != "" || a.git.pending != pending {
		t.Fatalf("opening a worktree holds git: busy %q, pending %d, was %d", a.git.busy, a.git.pending, pending)
	}
	a.switchFolder(other)
	settledGoroutines(hs)
	if a.root != other {
		t.Fatalf("window shows %q, want %q: the reading of the folder switched it back", a.root, other)
	}
}

// A query that only a worktree's folder path contains is not a name of it:
// Enter creates the branch it names, not switch to the worktree whose path
// it happens to be in.
func TestWorktreeBoxPathOnlyCreates(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	main := a.root
	// A linked worktree whose path has the query, and whose folder and branch do not.
	dir := filepath.Join(t.TempDir(), "pathonly")
	run("worktree", "add", "-b", "side2", filepath.Join(dir, "side2"))
	openWorktrees(t, hs)
	// The corpora are built here, not read from the box, which clears them when
	// Enter closes it: folder and branch alone match nothing, the path does.
	var names, legacy []string
	for _, tr := range a.worktrees.trees {
		names = append(names, worktreeName(tr)+" "+tr.Branch)
		legacy = append(legacy, worktreeName(tr)+" "+tr.Branch+" "+tr.Path)
	}
	if len(workspace.Fuzzy(names, "pathonly", len(names))) != 0 {
		t.Fatal("setup: a worktree's folder or branch matches the query")
	}
	if len(workspace.Fuzzy(legacy, "pathonly", len(legacy))) == 0 {
		t.Fatal("setup: the query matches no worktree path")
	}
	// The worktree is made beside the main one, from the commit HEAD is at.
	want := filepath.Join(filepath.Dir(main), filepath.Base(main)+".worktrees", "pathonly")
	base := strings.TrimSpace(run("rev-parse", "HEAD"))
	hs.tt.Type("pathonly")
	hs.settle()
	hs.tt.Key(0, ui.KeyEnter)
	hs.settleGit()
	if !slices.Contains(strings.Fields(run("branch", "--format=%(refname:short)")), "pathonly") {
		t.Fatal("Enter did not create branch pathonly")
	}
	wantEval, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, a.root); got != wantEval {
		t.Fatalf("window shows %q, want the new worktree folder %q", got, wantEval)
	}
	if got := strings.TrimSpace(run("rev-parse", "pathonly")); got != base {
		t.Fatalf("branch pathonly starts at %s, want the commit HEAD was at, %s", got, base)
	}
}

// Enter switches to a worktree whose folder has the query as typed, and
// creates for a new name that matches a worktree only by scattered letters.
func TestWorktreeBoxScatteredMatchCreates(t *testing.T) {
	hs, run, side := worktreeFixture(t)
	a := hs.a
	openWorktrees(t, hs)
	hs.tt.Type("sd")
	hs.settle()
	if len(a.worktrees.results) == 0 {
		t.Fatal("setup: sd matches no worktree")
	}
	hs.tt.Key(0, ui.KeyEnter)
	hs.settleGit()
	if !slices.Contains(strings.Fields(run("branch", "--format=%(refname:short)")), "sd") {
		t.Fatal("Enter on sd did not create branch sd")
	}
	if filepath.Base(a.root) != "sd" {
		t.Fatalf("window shows %q, want the new worktree sd", a.root)
	}
	openWorktrees(t, hs)
	hs.tt.Type("sid")
	hs.settle()
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	if got := mustEval(t, a.root); got != mustEval(t, side) {
		t.Fatalf("Enter on sid shows %q, want the worktree side", got)
	}
}

// A local branch that no worktree has checked out is named as one to check
// out with Switch Branch…, and offers no worktree; a branch a worktree has
// checked out is not.
func TestWorktreeBoxExistingBranchHint(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	run("branch", "orphan")
	hs.a.gitChanged()
	hs.settleGit()
	openWorktrees(t, hs)
	hs.tt.Type("orphan")
	hs.settle()
	if !hs.tt.HasText(locales.WorktreeBranchExists("orphan")) {
		t.Fatal("no hint for a branch no worktree has checked out")
	}
	for _, ch := range hs.a.repository().worktreeChoices() {
		if ch.create != "" {
			t.Fatal("offered to create a worktree on an existing branch")
		}
	}
	for _, name := range []string{"side", "main"} {
		hs.tt.Key(ui.Cmd, ui.KeyA)
		hs.tt.Type(name)
		hs.settle()
		if hs.tt.HasText(locales.WorktreeBranchExists(name)) {
			t.Fatalf("hint for %s, which a worktree has checked out", name)
		}
	}
}

// With HEAD detached, the row to create says which commit it starts from,
// by its short hash.
func TestWorktreeBoxCreateRowDetached(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	run("checkout", "-q", "--detach")
	hs.a.gitChanged()
	hs.settleGit()
	short := git.ShortHash(strings.TrimSpace(run("rev-parse", "HEAD")))
	openWorktrees(t, hs)
	hs.tt.Type("newone")
	hs.settle()
	if !hs.tt.HasText(locales.WorktreeFrom(short)) {
		t.Fatalf("create row does not say it is based on %s", short)
	}
}

// openSCM shows source control and waits for its frames.
func openSCM(hs *harness) {
	hs.a.showSourceControl()
	hs.settle()
}

// The title bar has the worktree and branch controls; source control repeats
// the branch with its remote actions.
func TestWorktreeAndBranchInTitleBar(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	hs.tt.SetSize(1200, 760)
	hs.settle()
	titleTip := locales.WorktreeStatusTip("main")
	branch := locales.BranchLabel("main")
	if r, ok := hs.tt.Find(titleTip); !ok || r.Y+r.H > titleBarH {
		t.Fatalf("title bar has no main worktree button: %v (shown %v), repository %+v", r, ok, hs.a.repository().git.repo)
	}
	if err := hs.tt.Click(titleTip); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	awaitWorktrees(t, hs)
	if !hs.a.worktrees.open {
		t.Fatal("title bar worktree button did not open the picker")
	}
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
	if r, ok := hs.tt.Find(branch); !ok || r.Y+r.H > titleBarH {
		t.Fatalf("title bar has no branch button: %v (shown %v)", r, ok)
	}
	if err := hs.tt.Click(branch); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !hs.a.branches.open {
		t.Fatal("title bar branch button did not open the picker")
	}
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
	openSCM(hs)
	if r, ok := hs.tt.Find(titleTip); !ok || r.Y+r.H > titleBarH {
		t.Fatalf("worktree button left the title bar: %v (shown %v)", r, ok)
	}
	if r, ok := hs.tt.Find(titleTip); !ok || r.Y+r.H > titleBarH {
		t.Fatalf("the title bar lost the worktree button: %v (shown %v)", r, ok)
	}
	r, ok := hs.tt.Find(locales.ScmMoreActions())
	if !ok || r.Y <= titleBarH {
		t.Fatalf("source control lost its remote actions: %v (shown %v)", r, ok)
	}
}

// The worktree button names a linked worktree's folder, and the main one
// shows its icon alone, in the title bar.
func TestTitleBarWorktreeButtonNamesLinked(t *testing.T) {
	hs, _, side := worktreeFixture(t)
	hs.tt.SetSize(1200, 760)
	if _, ok := hs.tt.Find(locales.WorktreeStatusTip("main")); !ok {
		t.Fatal("the title bar has no worktree button in the main worktree")
	}
	if hs.tt.HasText(filepath.Base(side)) {
		t.Fatal("the main worktree's button names a linked folder")
	}
	hs.a.switchFolder(side)
	hs.settleGit()
	if _, ok := hs.tt.Find(locales.WorktreeStatusTip(filepath.Base(side))); !ok || !hs.tt.HasText(filepath.Base(side)) {
		t.Fatal("the linked worktree's button does not name its folder")
	}
}

// control is a button that must be shown whole at a sidebar's width: its
// visible rectangle is at least least wide, which a clipped one is not.
type control struct {
	label string
	least float32
}

// Least widths of the controls: an icon with its padding, and a branch with
// its icon, name and chevron.
const (
	iconLeast   = 20
	branchLeast = 40
)

// fitsSidebar checks that the controls are shown whole at width w, inside the
// sidebar and clear of each other.
func fitsSidebar(t *testing.T, tt *ui.Tester, w float32, controls []control) {
	t.Helper()
	var rs []ui.Rect
	for _, c := range controls {
		r, ok := tt.Find(c.label)
		if !ok {
			t.Fatalf("sidebar %v: %q is not shown", w, c.label)
		}
		if r.W < c.least || r.X < 0 || r.X+r.W > w {
			t.Fatalf("sidebar %v: %q shown at %v, least width %v", w, c.label, r, c.least)
		}
		for _, o := range rs {
			if overlaps(o, r) {
				t.Fatalf("sidebar %v: %q at %v overlaps %v", w, c.label, r, o)
			}
		}
		rs = append(rs, r)
	}
}

// A worktree in a source control row narrowed to the sidebar's least width:
// the branch and Fetch stay in the row at every width, and the menu keeps the
// other remote actions.
func TestSourceControlRowFitsNarrowSidebar(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	m := a.pal.metricsOr()
	wide := float32(400) + 2*(m.pad+m.gap)
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	run("remote", "add", "origin", remote)
	run("push", "-q", "-u", "origin", "main")
	long := strings.Repeat("F", 48)
	branch := strings.Repeat("b", 48)
	dir := filepath.Join(t.TempDir(), long)
	run("worktree", "add", "-b", branch, dir)
	gitIn(t, dir, "push", "-q", "-u", "origin", branch)
	a.switchFolder(dir)
	hs.settleGit()
	openSCM(hs)
	hs.settleGit()
	// Draw the sidebar alone so polling cannot replace the test's status,
	// and labels cannot resolve to the title bar's matching buttons.
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(a.window.SidebarWidth).Height(760).Children(func() { a.sidebar(c) })
	}, 600, 760)
	draw := func() {
		for range 3 {
			step(tt)
		}
	}
	for _, w := range []float32{160, 240, wide} {
		a.window.SidebarWidth = w
		draw()
		controls := []control{
			{locales.CmdGitFetch(), iconLeast}, {locales.ScmMoreActions(), iconLeast},
		}
		fitsSidebar(t, tt, w, controls)
	}
	checkFetchCounts(t, tt, "↓0", "↑0")
	a.window.SidebarWidth = 160
	a.git.status.Behind = 69
	draw()
	checkFetchCounts(t, tt, "↓69", "↑0")
	fitsSidebar(t, tt, 160, []control{
		{locales.CmdGitFetch(), iconLeast}, {locales.ScmMoreActions(), iconLeast},
	})
	a.git.status.Behind, a.git.status.Ahead = 67890, 12345
	draw()
	checkFetchCounts(t, tt, "↓99+", "↑99+")
	fitsSidebar(t, tt, 160, []control{
		{locales.CmdGitFetch(), iconLeast}, {locales.ScmMoreActions(), iconLeast},
	})
	count, _ := tt.Find("↓99+")
	tt.Move(count.X+count.W/2, count.Y+count.H/2)
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText(locales.ScmUpstreamCounts(a.git.status.Upstream, 67890, 12345)) {
		t.Fatal("the compact counts lost their exact values in the Fetch tooltip")
	}
	tt.Move(0, 0)
	a.window.SidebarWidth = wide
	draw()
	checkFetchCounts(t, tt, "↓67890", "↑12345")
	a.git.status.Ahead, a.git.status.Behind = 0, 0

	// A branch without an upstream keeps Fetch and the menu; Publish is in the
	// menu.
	dir2 := filepath.Join(t.TempDir(), long+"2")
	run("worktree", "add", "-b", branch+"2", dir2)
	a.switchFolder(dir2)
	hs.settleGit()
	openSCM(hs)
	for _, w := range []float32{160, 240, wide} {
		a.window.SidebarWidth = w
		draw()
		if tt.HasText("↓0") || tt.HasText("↑0") {
			t.Fatalf("sidebar %v: branch without an upstream shows sync counts", w)
		}
		controls := []control{
			{locales.CmdGitFetch(), iconLeast}, {locales.ScmMoreActions(), iconLeast},
		}
		fitsSidebar(t, tt, w, controls)
	}
	// The menu has every remote action.
	a.window.SidebarWidth = 160
	draw()
	if err := tt.Click(locales.ScmMoreActions()); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{locales.CmdGitFetch(), locales.CmdGitPull(), locales.CmdGitPush()} {
		if !slices.Contains(tt.Menu(), label) {
			t.Fatalf("the More Actions menu has no %q: %q", label, tt.Menu())
		}
	}
	tt.CloseMenu()
}

// overlaps reports whether two rectangles share more than a sub-pixel edge.
func overlaps(a, b ui.Rect) bool {
	const eps = 0.5
	return a.X+eps < b.X+b.W && b.X+eps < a.X+a.W && a.Y+eps < b.Y+b.H && b.Y+eps < a.Y+a.H
}

// With no row to move among, the arrows and Enter do nothing, rather than
// choose a row that is not there.
func TestWorktreeBoxNoRowsArrowsAndEnter(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	openWorktrees(t, hs)
	hs.tt.Type("bad..no-match-xyz")
	hs.settle()
	for _, k := range []ui.Key{ui.KeyDown, ui.KeyUp, ui.KeyDown} {
		hs.tt.Key(0, k)
		hs.settle()
	}
	if !hs.a.worktrees.open || hs.a.fails != 0 {
		t.Fatalf("no rows: failures %d, open %v", hs.a.fails, hs.a.worktrees.open)
	}
	hs.tt.Key(0, ui.KeyEnter)
	hs.settle()
	if hs.a.fails != 0 {
		t.Fatalf("Enter with no rows: failures %d", hs.a.fails)
	}
}

// The row that creates a worktree names the base it starts from as the box
// opened, even when the branch changes while the box is open.
func TestWorktreeBoxCreateRowFrozenAtOpening(t *testing.T) {
	hs, run, _ := worktreeFixture(t)
	before := git.ShortHash(strings.TrimSpace(run("rev-parse", "HEAD")))
	openWorktrees(t, hs)
	run("checkout", "-q", "-b", "elsewhere")
	run("commit", "-q", "--allow-empty", "-m", "moved")
	hs.a.gitChanged()
	hs.settleGit()
	hs.tt.Type("newone")
	hs.settle()
	if !hs.tt.HasText(locales.WorktreeFrom("main (" + before + ")")) {
		t.Fatalf("create row does not keep the base of the opening: %v", hs.tt.Texts())
	}
}

// The headers of the repositories in a multi-repository source control do
// not repeat the worktree and branch controls of the title bar.
func TestRepositoryHeadersOmitBranchControls(t *testing.T) {
	hs, sub, _ := submoduleHarness(t)
	a := hs.a
	for _, ctx := range []*gitContext{a.repositoryForRoot(a.repository().git.repo.Root), a.repositoryForRoot(sub.git.repo.Root)} {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(400).Children(func() { ctx.repositoryHeader(c) })
		}, 600, 100)
		for range 3 {
			step(tt)
		}
		for _, label := range []string{
			locales.WorktreeStatusTip(filepath.Base(ctx.git.repo.Root)),
			locales.BranchLabel(ctx.branch()),
		} {
			if r, ok := tt.Find(label); ok {
				t.Fatalf("repository header still shows %q at %v", label, r)
			}
		}
	}
}

// The header of each repository in a multi-repository source control is
// rendered alone, so each of its controls is the one its label finds. At each
// sidebar width its controls are shown whole, and the submodule's wide header
// shows its remote buttons, counts included, and its more menu lists them.
func TestRepositoryHeaderFitsNarrowSidebar(t *testing.T) {
	hs, sub, _ := submoduleHarness(t)
	a := hs.a
	parentRoot := a.repository().git.repo.Root
	parent := a.repositoryForRoot(parentRoot)
	// The submodule gets a branch of its own and a remote with an upstream, so
	// its header has the remote actions to show.
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	gitIn(t, sub.git.repo.Root, "checkout", "-q", "-b", "sub-work")
	gitIn(t, sub.git.repo.Root, "remote", "set-url", "origin", remote)
	gitIn(t, sub.git.repo.Root, "push", "-q", "-u", "origin", "sub-work")
	sub.gitChanged()
	hs.settleGit()
	checkHeader(t, parent, false)
	checkHeader(t, sub, true)
}

// checkHeader renders ctx's header alone at each sidebar width, and checks its
// controls there; at 360 and wider, with remote, it checks the actions too, and
// the more menu lists them.
func checkHeader(t *testing.T, ctx *gitContext, remote bool) {
	t.Helper()
	width := float32(160)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(width).Children(func() { ctx.repositoryHeader(c) })
	}, 600, 100)
	name := ctx.label()
	base := filepath.Base(ctx.git.repo.Root)
	controls := []control{
		{locales.ScmToggleRepository(name), iconLeast},
		{locales.ScmSelectRepository(name), iconLeast},
		{locales.ScmMoreActions(), iconLeast},
	}
	for _, w := range []float32{160, 240, 360, 400} {
		width = w
		for range 3 {
			step(tt)
		}
		shown := controls
		if w >= 360 && remote {
			shown = append(append([]control{}, controls...),
				control{locales.CmdGitFetch(), iconLeast}, control{locales.ScmPullCount(10), iconLeast}, control{locales.ScmPushCount(10), iconLeast})
		}
		if ctx.git.status != nil {
			ctx.git.status.Ahead, ctx.git.status.Behind = 10, 10
		}
		for range 3 {
			step(tt)
		}
		for _, label := range []string{locales.WorktreeStatusTip(base), locales.BranchLabel(ctx.branch())} {
			if r, ok := tt.Find(label); ok {
				t.Fatalf("width %v: repository header still shows %q at %v", w, label, r)
			}
		}
		fitsSidebar(t, tt, w, shown)
	}
	if !remote {
		return
	}
	expanded := ctx.changesExpanded()
	if err := tt.Click(locales.ScmMoreActions()); err != nil {
		t.Fatal(err)
	}
	step(tt)
	if ctx.changesExpanded() != expanded {
		t.Fatal("more-actions button toggled the repository row")
	}
	for _, label := range []string{locales.CmdGitFetch(), locales.CmdGitPull(), locales.CmdGitPush()} {
		if !slices.Contains(tt.Menu(), label) {
			t.Fatalf("the more menu of the header lacks %q: %q", label, tt.Menu())
		}
	}
}

// The source control row has the branch on the left and Fetch with the other
// remote actions on the right, at every sidebar width.
func TestSourceControlRowShowsBranchAndFetch(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	run("remote", "add", "origin", remote)
	run("push", "-q", "-u", "origin", "main")
	branch := strings.Repeat("b", 48)
	dir := filepath.Join(t.TempDir(), strings.Repeat("F", 48))
	run("worktree", "add", "-b", branch, dir)
	gitIn(t, dir, "push", "-q", "-u", "origin", branch)
	a.switchFolder(dir)
	hs.settleGit()
	ctx := a.repository()
	width := float32(400)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(width).Children(func() { ctx.scmActionRow(c) })
	}, 600, 60)
	draw := func() {
		for range 3 {
			step(tt)
		}
	}
	check := func(where string) {
		t.Helper()
		draw()
		fetch, ok := tt.Find(locales.CmdGitFetch())
		if !ok {
			t.Fatalf("%s: Fetch is not in the source control row", where)
		}
		for _, label := range []string{branch, locales.BranchLabel(branch)} {
			if r, ok := tt.Find(label); ok {
				t.Fatalf("%s: the branch is still in the source control row at %v", where, r)
			}
		}
		if _, ok := tt.Find(locales.ScmMoreActions()); !ok {
			t.Fatalf("%s: the repository menu is missing", where)
		}
		if fetch.X < 0 || fetch.X+fetch.W > width {
			t.Fatalf("%s: Fetch at %v is outside %v", where, fetch, width)
		}
		fitsSidebar(t, tt, width, []control{
			{locales.CmdGitFetch(), iconLeast},
			{locales.ScmMoreActions(), iconLeast},
		})
	}
	check("wide")
	width = 160
	check("narrow")
	width = 400
	check("wide again")
}

// Fetch and its menu keep apart, so neither hides the other at any width.
func TestSourceControlFetchMenuDoesNotOverlap(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	ctx := hs.a.repository()
	var tt *ui.Tester
	for _, width := range []float32{160, 220, 400} {
		tt = ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(width).Children(func() { ctx.scmActionRow(c) })
		}, 600, 60)
		for range 3 {
			step(tt)
		}
		main, ok := tt.Find(locales.CmdGitFetch())
		if !ok {
			t.Fatalf("width %v: Fetch is not shown", width)
		}
		more, ok := tt.Find(locales.ScmRemoteActions())
		if !ok {
			t.Fatalf("width %v: the Fetch menu is not shown", width)
		}
		if overlaps(main, more) {
			t.Fatalf("width %v: Fetch %v overlaps its menu %v", width, main, more)
		}
		if main.X < 0 || more.X+more.W > width {
			t.Fatalf("width %v: Fetch %v or its menu %v leaves the sidebar", width, main, more)
		}
	}
}

// The commit message sits below the list, with its button inside it, and no
// fold arrow.
func TestSourceControlCommitControlsAtBottom(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	os.WriteFile(hs.path("main.go"), []byte("package main\n\nfunc main() { println(1) }\n"), 0o644)
	os.WriteFile(hs.path("notes.txt"), []byte("hi\n"), 0o644)
	hs.a.run("view.scm")
	hs.settleGit()
	if n := len(hs.a.git.status.Entries); n == 0 {
		t.Fatalf("no changes to show: %v", hs.tt.Texts())
	}
	msg, ok := hs.tt.Find(locales.ScmCommitMessage())
	if !ok {
		t.Fatal("the commit message is not shown")
	}
	list, ok := hs.tt.Find(groupName(groupChanges))
	if !ok {
		list, ok = hs.tt.Find(strings.ToUpper(groupName(groupChanges)))
	}
	if !ok {
		t.Fatalf("the changes list is not shown: %q", hs.tt.Texts())
	}
	if msg.Y <= list.Y {
		t.Fatalf("the commit message at %v is not below the list at %v", msg, list)
	}
	commit, ok := hs.tt.Find(locales.CmdGitCommit())
	if !ok {
		commit, ok = hs.tt.Find(locales.ScmCommitAll())
	}
	if !ok {
		t.Fatal("the commit button is not shown")
	}
	if commit.Y < msg.Y || commit.Y+commit.H > msg.Y+msg.H {
		t.Fatalf("the commit button at %v is not inside the message at %v", commit, msg)
	}
}

// A pull request of the checked-out branch does not add controls to the source
// control row, which keeps Fetch and the repository menu.
func TestPullRequestKeepsSourceControlRowClear(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs, run, _ := worktreeFixture(t)
	a := hs.a
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	run("remote", "add", "origin", remote)
	run("push", "-q", "-u", "origin", "main")
	branch := strings.Repeat("b", 48)
	dir := filepath.Join(t.TempDir(), strings.Repeat("F", 48))
	run("worktree", "add", "-b", branch, dir)
	gitIn(t, dir, "push", "-q", "-u", "origin", branch)
	a.switchFolder(dir)
	hs.settleGit()
	openSCM(hs)
	hs.settleGit()
	// Draw the sidebar alone so polling cannot replace the test's status,
	// and labels cannot resolve to the status bar's matching buttons.
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(a.window.SidebarWidth).Height(760).Children(func() { a.sidebar(c) })
	}, 600, 760)
	draw := func() {
		for range 3 {
			step(tt)
		}
	}
	for _, w := range []float32{200, 400} {
		a.window.SidebarWidth = w
		draw()
		a.git.pr, a.git.prRepo, a.git.prBranch = &git.PR{Number: 93, State: "OPEN", URL: "https://example.com/pull/93", Review: "APPROVED", Checks: git.ChecksPass, Passed: 3}, a.git.repo, branch
		draw()
		controls := []control{
			{locales.CmdGitFetch(), iconLeast}, {locales.ScmMoreActions(), iconLeast},
		}
		fitsSidebar(t, tt, w, controls)
		for _, label := range []string{branch, locales.BranchLabel(branch)} {
			if r, ok := tt.Find(label); ok && r.Y <= titleBarH {
				continue
			}
			if r, ok := tt.Find(label); ok {
				t.Fatalf("sidebar %v: the branch is in the source control row at %v", w, r)
			}
		}
		a.git.pr, a.git.prRepo, a.git.prBranch = nil, nil, ""
	}
}

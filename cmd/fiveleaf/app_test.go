package main

import (
	"encoding/json"
	"fiveleaf/internal/bg"
	"fiveleaf/internal/settings"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

// fakeHost answers the app's dialogs as a test says.
type fakeHost struct {
	folder string
	// projects are what everyProject says, and showed the project
	// showApp showed last.
	projects []*app
	showed   *app
	saveTo   string
	choice   int
	asked    []string
	// holdClose keeps the questions about changes open, their answers
	// waiting in held.
	holdClose bool
	held      []func(int)
	title     string
	locale    string // the language setLocale chose
	closed    bool
	trashed   []string
	// revealed are the paths asked to show in the Finder.
	revealed []string
	// others are the folders other windows have open, and windows those
	// asked to open in a window of their own.
	others         []string
	windows        []string
	projectWindows []string
	// blankWindows counts the windows asked for that show nothing.
	blankWindows int
	tabWindow    func(*image.Point) *app
	// folders are those asked to open as `ff <dir>` opens them.
	folders []string
	// recentChanges counts the changes of the recent folders.
	recentChanges int
	// clipboard are the files on the clipboard, and image its image.
	clipboard []string
	image     []byte
	// chosen are the paths the dialog of choosePaths answers.
	chosen         []string
	updateState    updateStatus
	updateChecks   int
	updateInstalls int
	updateRestarts int
	// settingsShown are the sections and keys the settings window opened
	// at, as "section:key".
	settingsShown []string
}

func (h *fakeHost) openFolder(done func(string)) { done(h.folder) }
func (h *fakeHost) saveAs(name string, done func(string)) {
	h.asked = append(h.asked, "save "+name)
	done(h.saveTo)
}
func (h *fakeHost) confirmClose(name string, done func(int)) {
	h.asked = append(h.asked, "close "+name)
	if h.holdClose {
		h.held = append(h.held, done)
		return
	}
	done(h.choice)
}
func (h *fakeHost) trash(path string) error {
	h.trashed = append(h.trashed, path)
	return os.RemoveAll(path)
}
func (h *fakeHost) reveal(path string)    { h.revealed = append(h.revealed, path) }
func (h *fakeHost) setTitle(title string) { h.title = title }
func (h *fakeHost) copyFiles(paths []string) error {
	h.clipboard = paths
	return nil
}
func (h *fakeHost) clipboardFiles() []string { return h.clipboard }
func (h *fakeHost) clipboardImage() []byte   { return h.image }
func (h *fakeHost) copyImage(png []byte) error {
	h.image = png
	return nil
}
func (h *fakeHost) exportAs(path, _ string, done func(string)) {
	h.asked = append(h.asked, "export "+filepath.Base(path))
	if h.saveTo != "" {
		done(h.saveTo)
	}
}
func (h *fakeHost) choosePaths(title, _, _ string, _ bool, done func([]string)) {
	h.asked = append(h.asked, title)
	if len(h.chosen) > 0 {
		done(h.chosen)
	}
}
func (h *fakeHost) closeWindow()         { h.closed = true }
func (h *fakeHost) setTheme(_, _ string) {}
func (h *fakeHost) setLocale(tag string) { h.locale = tag }
func (h *fakeHost) showFolder(dir string) bool {
	return slices.Contains(h.others, dir)
}
func (h *fakeHost) newWindow(dir string)         { h.windows = append(h.windows, dir) }
func (h *fakeHost) openProjectWindow(dir string) { h.projectWindows = append(h.projectWindows, dir) }
func (h *fakeHost) openProjectHere(string) bool  { return false }
func (h *fakeHost) newBlankWindow()              { h.blankWindows++ }
func (h *fakeHost) inWorkspace(fn func(*app)) {
	for _, project := range h.projects {
		if !project.window.Detached {
			h.showApp(project)
			fn(project)
			return
		}
	}
}
func (h *fakeHost) newTabWindow(drop *image.Point) *app {
	if h.tabWindow != nil {
		return h.tabWindow(drop)
	}
	return nil
}
func (h *fakeHost) openFolders(dirs []string) { h.folders = append(h.folders, dirs...) }
func (h *fakeHost) recentChanged()            { h.recentChanges++ }
func (h *fakeHost) menuChanged()              {}
func (h *fakeHost) showSettings(section, key string) {
	h.settingsShown = append(h.settingsShown, section+":"+key)
}
func (h *fakeHost) redrawWindows()       {}
func (h *fakeHost) installCLI()          {}
func (h *fakeHost) uninstallCLI()        {}
func (h *fakeHost) update() updateStatus { return h.updateState }
func (h *fakeHost) restartForUpdate()    { h.updateRestarts++ }
func (h *fakeHost) checkForUpdates() {
	h.updateChecks++
	h.updateState.phase = updateChecking
}
func (h *fakeHost) installUpdate() {
	h.updateInstalls++
	h.updateState.phase = updateDownloading
}
func (h *fakeHost) stepProject(int)              {}
func (h *fakeHost) showProject(int)              {}
func (h *fakeHost) closeProject()                {}
func (h *fakeHost) projectTabsChanged()          {}
func (h *fakeHost) nativeDrags() bool            { return false }
func (h *fakeHost) otherProjects() []*app        { return nil }
func (h *fakeHost) projectOf(*colTab) *app       { return nil }
func (h *fakeHost) carryTab(*colTab, *app, bool) {}
func (h *fakeHost) everyProject() []*app         { return h.projects }
func (h *fakeHost) showApp(to *app)              { h.showed = to }

// Off screen, the pull request of the branch is not asked of gh.
func (h *fakeHost) shown() bool { return false }

// workspaceFixture makes a folder of a few files.
// fixtureFiles are the files of a test's folder, by their paths in it.
var fixtureFiles = map[string]string{
	"main.go":             "package main\n\nimport \"fmt\"\n\n// main says hello.\nfunc main() {\n\tfmt.Println(\"hello, fiveleaf\")\n}\n",
	"README.md":           "# FiveLeaf\n\nA small editor.\n",
	"internal/a/a.go":     "package a\n\nconst Answer = 42\n",
	"internal/b/b.ts":     "export const greet = (name: string) => `hi ${name}`;\n",
	"web/index.html":      "<!doctype html>\n<title>FiveLeaf</title>\n",
	"web/style.css":       "body { color: #333; }\n",
	"node_modules/x/x.js": "module.exports = 1\n",
	"go.mod":              "module example\n\ngo 1.27\n",
}

func workspaceFixture(t testing.TB) string {
	root := t.TempDir()
	if err := writeFixture(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFixture(root string) error {
	for name, body := range fixtureFiles {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

type harness struct {
	t    testing.TB
	a    *app
	h    *fakeHost
	tt   *ui.Tester
	root string
	// held counts the background work a test holds on purpose, which
	// settle does not wait for.
	held int64
}

func newHarness(t testing.TB, dark bool) *harness {
	root := workspaceFixture(t)
	s := settings.Default()
	// The tests open Markdown as text, the editor they drive, in case
	// markdownMode's default changes.
	s.MarkdownMode = ""
	w := s.NewWindow()
	w.Folder = root
	h := &fakeHost{folder: root}
	a := newApp(h, s, w)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	tt.SetDark(dark)
	hs := &harness{t: t, a: a, h: h, tt: tt, root: root}
	hs.settle()
	return hs
}

// step draws a frame of a test a frame's time, 16 ms, after the last:
// the tester's clock stands still otherwise, and the app counts its frames
// by the time they are drawn at.
func step(tt *ui.Tester) {
	tt.Advance(16 * time.Millisecond)
	tt.Frame()
}

const settleTimeout = 30 * time.Second

// settle draws frames until the background work (reading directories,
// highlighting, indexing, git) is done: half a second of the tester's
// clock, which the animations, the double clicks and the waits of the app
// count by, and frames at the same time for as long as some is running or
// posted, which on a busy machine is longer than on an idle one. While
// work only runs, no frame is drawn, to leave the machine to it. Idle
// counts for two frames in a row, as a frame can start work that one
// finished asks for.
func (hs *harness) settle() {
	hs.t.Helper()
	hs.a.gitClock = hs.tt.Now
	// Advance before waiting: a final frame after the wait can start work.
	// The first frames see what the test changed, and start what moves of
	// it, as a fade, a frame's time apart, as the app counts its frames;
	// the clock then draws only the frames something asks for, which
	// drawing every 16 ms of it spent most of the tests' time on.
	const seen = 3
	for range seen {
		step(hs.tt)
	}
	hs.tt.Advance((31 - seen) * 16 * time.Millisecond)
	hs.tt.Frame()
	for idle, deadline := 0, time.Now().Add(settleTimeout); idle < 2; {
		running, queued := hs.busy()
		if time.Now().After(deadline) {
			hs.t.Fatalf("settle: %d background jobs still run after %s (queued: %v)\n%s", bg.Running()-hs.held, settleTimeout, queued, backgroundStacks())
		}
		switch {
		case queued:
			idle = 0
			hs.tt.Frame()
		case running:
			idle = 0
			time.Sleep(time.Millisecond)
		default:
			hs.tt.Frame()
			if hs.idle() {
				idle++
			} else {
				idle = 0
			}
		}
	}
}

// backgroundStacks returns the stacks of the goroutines bg started, to
// tell which of its jobs a settle waited on in vain.
func backgroundStacks() string {
	buf := make([]byte, 1<<22)
	buf = buf[:runtime.Stack(buf, true)]
	var jobs []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "fiveleaf/internal/bg.") {
			jobs = append(jobs, g)
		}
	}
	return strings.Join(jobs, "\n\n")
}

// busy reports whether background work runs, and whether some it posted
// waits for a frame. Whether the view asks for a frame does not count: it
// always does while something on screen moves.
func (hs *harness) busy() (running, queued bool) {
	// A job posts before it stops counting: sample the work before its queue.
	running = bg.Running() > hs.held
	// A parse of Live Preview waits on a timer of the wall clock for the
	// edits to pause, which the tester's clock does not count: it is work
	// to come, as the parse it starts.
	for _, doc := range hs.a.docs {
		if l := doc.live; l != nil && (l.pending || l.running) {
			running = true
		}
	}
	hs.a.mu.Lock()
	n := len(hs.a.queue)
	hs.a.mu.Unlock()
	return running, n > 0
}

// idle reports whether no background work runs and none posted waits.
func (hs *harness) idle() bool {
	running, queued := hs.busy()
	return !running && !queued
}

func (hs *harness) shot(name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		hs.t.Fatal(err)
	}
	png.Encode(f, hs.tt.Image())
	f.Close()
}

func (hs *harness) path(rel string) string { return filepath.Join(hs.root, filepath.FromSlash(rel)) }

// TestStatusBarDropsTheLeastNeededFirst: as the window narrows, the
// status bar's parts go one by one, the least needed first, each only
// once the bar would not hold it, and those left fit.
func TestStatusBarDropsTheLeastNeededFirst(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	branch := "feature/responsive-title-bar-polish"
	git("checkout", "-q", "-b", branch)
	hs.a.openFile(hs.path("main.go"), true)
	hs.settleGit()
	// In the order they go.
	parts := []string{"UTF-8", "9 lines", "LF", "Tabs", "Go"}
	gone := 0
	for w := 1200; w >= 300; w -= 20 {
		hs.tt.SetSize(w, 400)
		hs.settle()
		for i, s := range parts {
			box, shown := hs.tt.Find(s)
			switch {
			case !shown:
				gone = max(gone, i+1)
			case i < gone:
				t.Errorf("at %d, %q is back", w, s)
			case box.X+box.W > float32(w):
				t.Errorf("at %d, %q does not fit", w, s)
			}
		}
		if box, ok := hs.tt.Find("Ln 1, Col 1"); !ok || box.X+box.W > float32(w) {
			t.Fatalf("at %d the position does not fit: %v", w, box)
		}
		switch {
		case w == 760 && gone > 0:
			t.Errorf("at 760, with room for it, the bar leaves out %q", parts[gone-1])
		case w == 300 && gone == 0:
			t.Error("at 300 the status bar has not dropped any optional details")
		}
	}
}

func TestTitleBarOmitsSearchAndKeepsQuickOpen(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			onOS(t, platform)
			hs := newHarness(t, true)
			a, tt := hs.a, hs.tt
			a.openFile(hs.path("main.go"), true)
			for _, width := range []int{640, 1200} {
				tt.SetSize(width, 760)
				hs.settle()
				if r, ok := tt.Find(locales.QuickOpenLabel()); ok {
					t.Fatalf("width %d: the title bar still has a search entry: %v", width, r)
				}
			}
			// The native menu dispatches default accelerators; the UI
			// tester only sends input to the view, without that menu.
			mods := ui.Ctrl
			if macKeys() {
				mods = ui.Cmd
			}
			for _, cmd := range a.commands() {
				if cmd.id == "go.quickOpen" {
					stroke, ok := parseAccel(a.menuAccel(cmd))
					if !ok || stroke.Mods != mods || stroke.Key != ui.KeyP {
						t.Fatalf("Quick Open lost its platform shortcut: %q", a.menuAccel(cmd))
					}
					cmd.run()
					break
				}
			}
			hs.settle()
			if !a.quick.open || a.quick.query != "" {
				t.Fatal("Quick Open is no longer reachable with its shortcut")
			}
		})
	}
}

func TestOpenFolderShowsTreeAndOpensFiles(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	for _, name := range []string{"internal", "web", "main.go", "README.md", "go.mod"} {
		if !tt.HasText(name) {
			t.Errorf("tree lacks %q: %q", name, tt.Texts())
		}
	}
	if tt.HasText("node_modules") == false {
		t.Error("node_modules is hidden from the tree; it should show (only quick open skips it)")
	}
	hs.shot("01-welcome")

	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	d := a.activeEditor()
	if d == nil || d.path != hs.path("main.go") {
		t.Fatalf("active doc %v after clicking main.go", d)
	}
	// A folder opens with a click, and shows its children.
	if err := tt.Click("internal"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !tt.HasText("a") || !tt.HasText("b") {
		t.Errorf("internal did not open: %q", tt.Texts())
	}
	hs.shot("02-editor-dark")
	if !strings.Contains(hs.h.title, "main.go") {
		t.Errorf("title %q", hs.h.title)
	}
}

func TestEditSaveAndDirtyClose(t *testing.T) {
	hs := newHarness(t, false)
	a, tt := hs.a, hs.tt
	a.settings.AutoSave = false // the prompts are for files that do not save themselves
	a.openFile(hs.path("README.md"), true)
	hs.settle()
	tt.Type("Hello ")
	hs.settle()
	d := a.activeEditor()
	if !d.dirty() || !strings.HasPrefix(d.buf.String(), "Hello # FiveLeaf") {
		t.Fatalf("typed: dirty %v text %q", d.dirty(), d.buf.String())
	}
	if !strings.Contains(hs.h.title, "●") {
		t.Errorf("title %q lacks the dot of changes", hs.h.title)
	}
	a.run("file.save")
	hs.settle()
	data, _ := os.ReadFile(hs.path("README.md"))
	if !strings.HasPrefix(string(data), "Hello # FiveLeaf") || d.dirty() {
		t.Fatalf("saved %q, dirty %v", data, d.dirty())
	}
	// Our own save comes back from the watcher without changing anything.
	hs.settle()
	if d.dirty() {
		t.Error("dirty after the watcher saw the save")
	}

	// Closing a changed tab asks; Cancel keeps it.
	tt.Type("more ")
	hs.settle()
	hs.h.choice = 2
	a.run("file.close")
	hs.settle()
	if len(a.docs) != 1 || len(hs.h.asked) != 1 {
		t.Fatalf("cancel: %d docs, asked %v", len(a.docs), hs.h.asked)
	}
	// Don't Save closes it, and leaves the file.
	hs.h.choice = 1
	a.run("file.close")
	hs.settle()
	if len(a.docs) != 0 {
		t.Fatalf("don't save: %d docs", len(a.docs))
	}
	data, _ = os.ReadFile(hs.path("README.md"))
	if strings.Contains(string(data), "more") {
		t.Error("discarded changes were saved")
	}
}

func TestExternalChangeReloadsCleanDocument(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	a.openFile(hs.path("go.mod"), true)
	hs.settle()
	os.WriteFile(hs.path("go.mod"), []byte("module changed\n"), 0o644)
	deadline := time.Now().Add(5 * time.Second)
	for a.activeEditor().buf.String() != "module changed\n" && time.Now().Before(deadline) {
		hs.settle()
	}
	if got := a.activeEditor().buf.String(); got != "module changed\n" {
		t.Errorf("not reloaded: %q", got)
	}
}

// A file changed on disk, lines added above the view, keeps the editor on
// the text it showed, and the caret on its text, where Select All and an
// edit would scroll to the caret.
func TestReloadKeepsTheViewAndTheCaret(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		hs := newHarness(t, true)
		a := hs.a
		a.settings.WordWrap = wrap
		p := hs.longFile("long.go", 400)
		a.openFile(p, true)
		hs.settle()
		d := a.activeEditor()
		s, _ := d.buf.Snapshot().LineRange(39) // "\treturn 9"
		hs.place(d, s+2, s+2)
		d.vscroll.Y = editorPadY + 300*a.font.lineH
		hs.settle()
		topLine := func() int {
			return a.geometry(d, d.buf.Snapshot().LineCount()).at(d.vscroll.Y - editorPadY)
		}
		if got := topLine(); got != 300 {
			t.Fatalf("wrap %v: line %d at the top", wrap, got)
		}
		data, _ := os.ReadFile(p)
		os.WriteFile(p, []byte("// a\n// b\n// c\n"+string(data)), 0o644)
		deadline := time.Now().Add(5 * time.Second)
		for !strings.HasPrefix(d.buf.String(), "// a") && time.Now().Before(deadline) {
			hs.settle()
		}
		hs.settle()
		snap := d.buf.Snapshot()
		if s, _ := snap.LineRange(42); d.caret != s+2 || d.sel[0] != d.sel[1] {
			t.Errorf("wrap %v: selection %v on line %d, want the caret at %d", wrap, d.sel, snap.LineAt(d.caret), s+2)
		}
		if got := topLine(); got != 303 {
			t.Errorf("wrap %v: line %d at the top, want 303", wrap, got)
		}
		if d.dirty() {
			t.Errorf("wrap %v: dirty after the reload", wrap)
		}
	}
}

func TestReloadEdit(t *testing.T) {
	for _, c := range []struct {
		old, now string
		want     textEdit
		from, to int
	}{
		{"a\nb\nc\n", "a\nB\nc\n", textEdit{2, 3, "B"}, 1, 1},
		{"a\nb\n", "x\na\nb\n", textEdit{0, 0, "x\n"}, 0, 0},
		{"日本\n語\n", "日本\n五語\n", textEdit{3, 3, "五"}, 1, 1},
		{"é", "è", textEdit{0, 1, "è"}, 0, 0},
		{"a\nb\nc", "a\nc", textEdit{2, 4, ""}, 1, 2},
	} {
		e, from, to := reloadEdit(c.old, c.now)
		if e != c.want || from != c.from || to != c.to {
			t.Errorf("reloadEdit(%q, %q) = %+v, %d, %d; want %+v, %d, %d", c.old, c.now, e, from, to, c.want, c.from, c.to)
		}
	}
}

func TestQuickOpenAndGoToLine(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.run("go.quickOpen")
	deadline := time.Now().Add(5 * time.Second)
	for a.index == nil && time.Now().Before(deadline) {
		hs.settle()
	}
	for _, f := range a.index {
		if strings.HasPrefix(f, "node_modules/") {
			t.Errorf("index has %s", f)
		}
	}
	tt.Type("bts")
	hs.settle()
	hs.shot("03-quick-open")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if d := a.activeEditor(); d == nil || filepath.Base(d.path) != "b.ts" {
		t.Fatalf("quick open opened %v", d)
	}
	// As the Explorer spells it, with Windows' backslashes there, so that
	// the file opens once whichever way it is opened.
	if d := a.activeEditor(); d.path != filepath.Clean(d.path) {
		t.Errorf("quick open opened %q, not %q", d.path, filepath.Clean(d.path))
	}
	if a.quick.open {
		t.Error("quick open stayed open")
	}

	a.openFile(hs.path("main.go"), true)
	hs.settle()
	a.run("go.line")
	hs.settle()
	tt.Type("6")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	d := a.activeEditor()
	if line := d.buf.Snapshot().LineAt(d.caret); line != 5 {
		t.Errorf("caret on line %d, want 5", line)
	}
}

func TestQuickOpenToSide(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	quickOpenSide := func(query string) {
		t.Helper()
		a.run("go.quickOpen")
		hs.settle()
		deadline := time.Now().Add(5 * time.Second)
		for a.index == nil && time.Now().Before(deadline) {
			hs.settle()
		}
		tt.Type(query)
		hs.settle()
		hs.key(ui.Cmd, ui.KeyEnter)
		if a.quick.open {
			t.Fatal("quick open stayed open")
		}
	}

	quickOpenSide("bts")
	if len(a.cols) != 2 {
		t.Fatalf("%d columns, want 2", len(a.cols))
	}
	if a.col != 1 || filepath.Base(a.activeEditor().path) != "b.ts" {
		t.Errorf("column %d shows %s, want b.ts in column 1", a.col, a.activeEditor().path)
	}
	if len(a.cols[0].tabs) != 1 {
		t.Errorf("the first column has %d tabs, want 1", len(a.cols[0].tabs))
	}

	// From the column on the right, the side is the one on its left.
	quickOpenSide("gomod")
	if len(a.cols) != 2 {
		t.Fatalf("%d columns, want 2", len(a.cols))
	}
	if a.col != 0 || filepath.Base(a.activeEditor().path) != "go.mod" {
		t.Errorf("column %d shows %s, want go.mod in column 0", a.col, a.activeEditor().path)
	}
}

func TestFindAndReplace(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	a.run("edit.replace")
	hs.settle()
	tt.Type("main")
	hs.settle()
	if n := len(a.find.matches); n != 3 {
		t.Fatalf("%d matches of main, want 3", n)
	}
	hs.shot("04-find")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if a.find.cur != 1 {
		t.Errorf("current match %d after Enter", a.find.cur)
	}
	a.find.with = "entry"
	a.replaceAll(a.activeEditor())
	hs.settle()
	if got := a.activeEditor().buf.String(); strings.Contains(got, "main") || strings.Count(got, "entry") != 3 {
		t.Errorf("after replace all: %q", got)
	}
	// Regular expressions, with groups.
	a.find.regex, a.find.query, a.find.with, a.find.stale = true, `fmt\.(\w+)`, "log.$1", true
	a.replaceAll(a.activeEditor())
	if got := a.activeEditor().buf.String(); !strings.Contains(got, "log.Println") {
		t.Errorf("regexp replace: %q", got)
	}
	tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.find.open {
		t.Error("Escape left find open")
	}
}

func TestNewFileAndRenameFromTheTree(t *testing.T) {
	for _, preview := range []bool{false, true} {
		t.Run(fmt.Sprintf("markdownPreview=%v", preview), func(t *testing.T) {
			hs := newHarness(t, false)
			a, tt := hs.a, hs.tt
			a.settings.MarkdownMode = map[bool]string{true: "live"}[preview]
			a.askNewEntry(hs.root, false)
			hs.settle()
			tt.Type("notes/todo.md")
			tt.Key(0, ui.KeyEnter)
			hs.settle()
			if _, err := os.Stat(hs.path("notes/todo.md")); err != nil {
				t.Fatal(err)
			}
			d := a.activeEditor()
			if d == nil || d.path != hs.path("notes/todo.md") {
				t.Fatalf("new file not open: %v", d)
			}
			if a.livePreview(d) != preview {
				t.Fatalf("new Markdown file opened with live=%v, want %v", d.live, preview)
			}
			tt.Type("# hello")
			hs.settle()
			if got := d.buf.String(); got != "# hello" {
				t.Fatalf("the new file holds %q", got)
			}
			// An ordinary open still follows the preference.
			a.openFile(hs.path("README.md"), true)
			if preview {
				hs.until("README Live Preview", func() bool { return a.activeEditor().live })
			} else {
				hs.settle()
			}
			if got := a.activeEditor().live; got != preview {
				t.Fatalf("an ordinary open used Live Preview %v, want %v", got, preview)
			}
			if i := a.column().showingFile(hs.path("notes/todo.md")); i < 0 {
				t.Fatal("the new file's tab is gone")
			} else {
				a.activate(a.column(), i, true)
			}
			a.askRename(hs.path("notes/todo.md"))
			hs.settle()
			hs.shot("05-rename")
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("done.md")
			tt.Key(0, ui.KeyEnter)
			hs.settle()
			if _, err := os.Stat(hs.path("notes/done.md")); err != nil {
				t.Fatal(err)
			}
			if d := a.activeEditor(); d.path != hs.path("notes/done.md") {
				t.Errorf("open document did not follow the rename: %s", d.path)
			}
		})
	}
}

// openText opens a file for its text only while it runs, restoring the
// flag it found, nested calls and failed opens included.
func TestOpenTextRestoresState(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	if a.openingText {
		t.Fatal("openingText begins set")
	}
	var seen []bool
	a.openText(func() {
		seen = append(seen, a.openingText)
		a.openText(func() { seen = append(seen, a.openingText) })
		seen = append(seen, a.openingText)
	})
	if a.openingText || !slices.Equal(seen, []bool{true, true, true}) {
		t.Fatalf("openingText %v after, the calls saw %v", a.openingText, seen)
	}
	// A flag set outside stays set: a nested open restores it.
	a.openingText = true
	a.openText(func() { a.openFile(hs.path("README.md"), true) })
	if !a.openingText {
		t.Fatal("an openText inside a set openingText reset it")
	}
	a.openingText = false
	// A file that does not open restores the flag too.
	a.openText(func() { a.openFile(hs.path("missing.md"), true) })
	if a.openingText {
		t.Fatal("a failed open left openingText set")
	}
	// The flag, not the file, decides: source under it, preview after.
	a.settings.MarkdownMode = "live"
	a.openText(func() { a.openFile(hs.path("README.md"), true) })
	hs.settle()
	if d := a.activeEditor(); d == nil || d.live {
		t.Fatal("openText did not open the source")
	}
	os.WriteFile(hs.path("plain.md"), []byte("# Plain\n"), 0o644)
	a.openFile(hs.path("plain.md"), true)
	hs.until("Live Preview", func() bool {
		d := a.activeEditor()
		return d != nil && d.path == hs.path("plain.md") && d.live
	})
}

func TestTabsKeepTheirState(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	tt.Type("x")
	a.openFile(hs.path("README.md"), true)
	hs.settle()
	tt.Type("y")
	a.activate(a.column(), 0, true)
	hs.settle()
	// Undo in the first tab undoes its own edit, after the other tab's.
	tt.Key(ui.Cmd, ui.KeyZ)
	hs.settle()
	if got := a.docs[0].buf.String(); strings.HasPrefix(got, "x") {
		t.Errorf("undo in the first tab: %q", got[:10])
	}
	if !strings.HasPrefix(a.docs[1].buf.String(), "y") {
		t.Error("the second tab lost its edit")
	}
	a.cycleTab(1)
	if got := a.column().active; got != 1 {
		t.Errorf("active %d after the next tab", got)
	}
	hs.shot("06-tabs")
}

func TestPanelWithTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a shell")
	}
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	a.run("view.terminal")
	hs.settle()
	if len(a.terms) != 1 || !a.window.PanelOpen {
		t.Fatalf("terminal: %d sessions, panel %v", len(a.terms), a.window.PanelOpen)
	}
	if a.focus != paneTerminal {
		t.Fatal("opening a terminal just after a file left the keyboard in the editor")
	}
	tt.Type("echo fiveleaf-$((40+2))")
	tt.Key(0, ui.KeyEnter)
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(a.terms[0].term.Text(), "fiveleaf-42") && time.Now().Before(deadline) {
		hs.settle()
	}
	if !strings.Contains(a.terms[0].term.Text(), "fiveleaf-42") {
		t.Errorf("terminal text %q", a.terms[0].term.Text())
	}
	a.run("view.newTerminal")
	hs.settle()
	if len(a.terms) != 2 || a.panelTerm != a.terms[1] {
		t.Errorf("second terminal: %d, the panel shows the first", len(a.terms))
	}
	hs.shot("07-terminal")
	a.closeTerminals()
}

func TestLargeFileStaysQuick(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	var b strings.Builder
	for i := range 100_000 {
		fmt.Fprintf(&b, "func f%d(x int) int { return x * %d } // line %d\n", i, i, i)
	}
	big := hs.path("big.go")
	os.WriteFile(big, []byte(b.String()), 0o644)
	start := time.Now()
	a.openFile(big, true)
	step(tt)
	t.Logf("open 100k lines: %v", time.Since(start))
	hs.settle()
	start = time.Now()
	const n = 100
	for range n {
		tt.Scroll(700, 400, 0, 120)
		step(tt)
	}
	t.Logf("scroll: %.2f ms/frame", float64(time.Since(start).Microseconds())/1000/n)
	start = time.Now()
	for range n {
		tt.Type("a")
		step(tt)
	}
	t.Logf("type: %.2f ms/frame", float64(time.Since(start).Microseconds())/1000/n)
	hs.settle()
	hs.shot("08-big-file")
}

// Showing and hiding the sidebar and the panel keeps the editor's state:
// its caret, its place and its undo history.
func TestLayoutChangesKeepEditorState(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	tt.Type("// edit\n")
	hs.settle()
	caret := a.activeEditor().caret
	a.run("view.sidebar")
	hs.settle()
	shell := testPanelSession(t, a)
	a.showTerminal(shell)
	hs.settle()
	a.maximizePanel()
	hs.settle()
	a.terminalToPanel(shell)
	a.hidePanel()
	a.run("view.sidebar")
	hs.settle()
	a.run("view.sidebarRight")
	hs.settle()
	a.run("view.sidebarRight")
	hs.settle()
	d := a.activeEditor()
	if d.caret != caret {
		t.Errorf("caret %d after layout changes, was %d", d.caret, caret)
	}
	d.vscroll.Focus()
	hs.settle()
	tt.Key(ui.Cmd, ui.KeyZ)
	hs.settle()
	if strings.Contains(d.buf.String(), "// edit") {
		t.Error("undo history lost across layout changes")
	}
}

func TestSplittersResize(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	// The sidebar at the left, its splitter along its right edge.
	a.settings.SidebarRight = false
	hs.settle()
	w := a.window.SidebarWidth
	x := w - 3 // the sidebar's right edge
	tt.Press(x, 300)
	tt.Move(x+40, 300)
	tt.Move(x+60, 300)
	tt.Release(x+60, 300)
	hs.settle()
	if got := a.window.SidebarWidth; got < w+50 || got > w+70 {
		t.Errorf("sidebar %v after dragging 60 from %v", got, w)
	}
}

// On the right, the sidebar's splitter is along its left edge, and
// dragging it left widens it.
func TestSidebarOnTheRight(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.settings.SidebarRight = true
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	hs.shot("sidebar-right")
	w := a.window.SidebarWidth
	x := 1200 - w + 2 // the sidebar's left edge
	tt.Press(x, 300)
	tt.Move(x-40, 300)
	tt.Move(x-60, 300)
	tt.Release(x-60, 300)
	hs.settle()
	if got := a.window.SidebarWidth; got < w+50 || got > w+70 {
		t.Errorf("sidebar %v after dragging 60 left from %v", got, w)
	}
}

// The terminal panel moves to either side of the window, past the
// sidebar, so that with both on a side the panel is the outer one, and
// its splitter, along the edge facing the editors, resizes it.
func TestTerminalPanelAtASide(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a shell")
	}
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	at := func(name string) ui.Rect {
		box, ok := tt.Find(name)
		if !ok {
			t.Fatalf("%s is not shown", name)
		}
		return box
	}
	// The status bar's buttons keep their own sides.
	termBtn, sideBtn := a.withKey("Toggle Terminal", "view.terminal"), a.withKey("Toggle Sidebar", "view.sidebar")
	explorerBtn := a.withKey(locales.SidebarExplorer(), "view.explorer")
	buttons := func() (term, side ui.Rect) { return at(termBtn), at(sideBtn) }
	width := float32(1200)
	// With the sidebar and its button at the left, the panel and its
	// button go right of them.
	a.settings.SidebarRight, a.settings.TerminalButtonRight = false, false
	a.openFile(hs.path("main.go"), true)
	a.run("view.terminal")
	hs.settle()
	// At the bottom, the panel is under the editors, right of the sidebar,
	// and both buttons stay at the bottom left with the sidebar's first.
	if panel, explorer := at("TERMINAL"), at(explorerBtn); panel.Y < explorer.Y || panel.X < explorer.X {
		t.Errorf("the panel at the bottom: %v, the sidebar %v", panel, explorer)
	}
	if term, side := buttons(); term.X > width/2 || side.X > term.X || term.Y < 760-statusBarH || side.Y < 760-statusBarH {
		t.Errorf("the buttons at the bottom: the terminal's %v, the sidebar's %v", term, side)
	}
	// On the left, past the sidebar, at the window's own edge, the panel
	// keeps its button beside the sidebar's at the bottom left.
	a.run("view.panelPosition.left")
	hs.settle()
	if panel, explorer := at("TERMINAL"), at(explorerBtn); panel.X > explorer.X || panel.X > 40 {
		t.Errorf("the panel on the left: %v, the sidebar %v", panel, explorer)
	}
	if term, side := buttons(); term.X > width/2 || side.X > term.X {
		t.Errorf("the buttons with the panel on the left: the terminal's %v, the sidebar's %v", term, side)
	}
	// Its splitter is on its right, and dragging it right widens it.
	size := a.window.PanelWidth
	edge := size - 2
	tt.Press(edge, 300)
	tt.Move(edge+40, 300)
	tt.Move(edge+60, 300)
	tt.Release(edge+60, 300)
	hs.settle()
	if got := a.window.PanelWidth; got < size+50 || got > size+70 {
		t.Errorf("the panel %v after dragging 60 from %v", got, size)
	}
	// With the sidebar on the right, its button follows while the
	// terminal's button and panel stay on the left.
	a.run("view.sidebarRight")
	hs.settle()
	if term, side := buttons(); term.X > width/2 || side.X < width/2 {
		t.Errorf("the buttons with the panel left, the sidebar right: the terminal's %v, the sidebar's %v", term, side)
	}
	// The panel on the right, with the sidebar there too, is still the
	// outer one, at the window's own edge, with its button still at the left.
	a.run("view.panelPosition.right")
	hs.settle()
	panel, explorer := at("TERMINAL"), at(explorerBtn)
	if panel.X < explorer.X || panel.X < width-a.window.PanelWidth || panel.X > width-a.window.PanelWidth+40 {
		t.Errorf("the panel on the right: %v, the sidebar %v, width %v", panel, explorer, a.window.PanelWidth)
	}
	if term, _ := buttons(); term.X > width/2 {
		t.Errorf("the terminal's button with the panel on the right: %v", term)
	}
	// In a narrow window it gives way, so the editors keep 320 points
	// beside the sidebar, and takes its width back as the window widens.
	a.run("view.sidebarRight")
	a.run("view.panelPosition.left")
	hs.tt.SetSize(760, 800)
	hs.settle()
	if closeButton := at(locales.TermClose()); closeButton.X+closeButton.W > 760-320-a.window.SidebarWidth {
		t.Errorf("the panel in a narrow window reaches %v, past %v", closeButton.X+closeButton.W, 760-320-a.window.SidebarWidth)
	}
	hs.tt.SetSize(int(width), 800)
	hs.settle()
	if closeButton := at(locales.TermClose()); closeButton.X+closeButton.W < a.window.PanelWidth-40 {
		t.Errorf("the panel widened back to %v, not %v", closeButton.X+closeButton.W, a.window.PanelWidth)
	}
	// Hidden, moving it shows it again.
	a.hidePanel()
	hs.settle()
	if a.window.PanelOpen {
		t.Fatal("the panel stays open after hiding it")
	}
	a.run("view.panelPosition.left")
	hs.settle()
	if !a.window.PanelOpen || a.settings.PanelPosition != "left" {
		t.Fatalf("a hidden panel moved: open %v, position %q", a.window.PanelOpen, a.settings.PanelPosition)
	}
}

func TestMaximizePanelMovesTerminalAndRestoresDock(t *testing.T) {
	if testing.Short() {
		t.Skip("loads libghostty-vt")
	}
	for _, dock := range []struct {
		side  string
		width int
	}{{"bottom", 1200}, {"left", 1200}, {"right", 1200}, {"bottom", 390}} {
		side := dock.side
		name := fmt.Sprintf("%s-%d", side, dock.width)
		t.Run(name, func(t *testing.T) {
			hs := newHarness(t, side != "bottom")
			a, tt := hs.a, hs.tt
			if dock.width == 390 {
				a.window.SidebarClosed = true
				tt.SetSize(dock.width, 640)
			}
			a.openFile(hs.path("main.go"), true)
			file := a.activeTab()
			if dock.width == 1200 {
				a.openFile(hs.path("README.md"), true)
				a.activate(a.column(), 0, true)
			}
			var other *session
			if side != "bottom" {
				other = testPanelSession(t, a)
				other.title = "build"
			}
			shell := testPanelSession(t, a)
			shell.title = "workspace shell"
			original := shell.term
			original.Feed([]byte("output before maximize\r\n"))
			a.showTerminal(shell)
			a.setPanelPosition(side)
			a.window.PanelHeight, a.window.PanelWidth = 203, 384
			hs.settle()
			if _, duplicate := tt.Find("Move Terminal to Editor Area"); duplicate {
				t.Error("the redundant move-to-editor toolbar button is still visible")
			}
			if side == "left" {
				if err := tt.RightClick("TERMINAL"); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem(locales.PanelMaximize()); err != nil {
					t.Fatal(err)
				}
			} else if err := tt.Click(locales.PanelMaximize()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if a.window.PanelOpen || a.window.TerminalOpen || a.panelTerm != nil || len(a.panelTerms()) != 0 || shell.tab == nil || a.activeTab() != shell.tab || a.focus != paneEditor || !tt.Focused("Terminal") {
				t.Fatal("maximize did not move the selected terminal into a focused editor tab and hide the dock")
			}
			if other != nil && (other.tab == nil || !slices.Contains(a.column().tabs, other.tab)) {
				t.Fatal("maximize left another terminal behind in the dock")
			}
			terminalBox, found := tt.Find("Terminal")
			restore, visible := tt.Find(locales.PanelRestore())
			if !found || !visible || restore.X < terminalBox.X+terminalBox.W/2 || restore.X+restore.W > terminalBox.X+terminalBox.W || restore.Y < terminalBox.Y+terminalBox.H/2 || restore.Y+restore.H > terminalBox.Y+terminalBox.H {
				t.Fatalf("floating restore %v (%v) is not on the right of terminal %v (%v)", restore, visible, terminalBox, found)
			}
			hs.shot("terminal-maximized-" + name)
			if err := tt.Click(locales.PanelRestore()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if !a.window.PanelOpen || a.panelTerm != shell || shell.tab != nil || a.focus != paneTerminal || !tt.Focused("Terminal") || a.activeTab() != file || a.panelSide() != side || a.window.PanelHeight != 203 || a.window.PanelWidth != 384 {
				t.Fatal("restore did not return the same terminal, dock size, side, editor and focus")
			}
			if shell.term != original || !strings.Contains(shell.term.Text(), "output before maximize") || (other != nil && !slices.Equal(a.panelTerms(), []*session{other, shell})) {
				t.Fatal("moving the terminal lost its output or changed the other panel terminals")
			}
			if _, visible := tt.Find(locales.PanelRestore()); visible {
				t.Fatal("the restore button stayed on the docked terminal")
			}
		})
	}
}

func TestRestoresTheActiveTab(t *testing.T) {
	root := workspaceFixture(t)
	// The tabs as a version without columns kept them.
	files := []string{filepath.Join(root, "main.go"), filepath.Join(root, "README.md"), filepath.Join(root, "go.mod")}
	data, _ := json.Marshal(map[string]any{"folder": root, "openFiles": files, "activeFile": files[1]})
	path := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(path, data, 0o644)
	s := settings.Load(path)
	a := newApp(&fakeHost{folder: root}, s, s.Windows[0])
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	step(tt)
	step(tt)
	if d := a.activeEditor(); d == nil || d.path != files[1] {
		t.Fatalf("active %v, want README.md", d)
	}
	tabs := a.column().tabs
	if len(a.cols) != 1 || len(tabs) != 3 || tabs[0].ed.path != files[0] || tabs[2].ed.path != files[2] {
		t.Fatalf("tabs out of order")
	}
}

func TestReopenClosedTab(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	os.WriteFile(hs.path("README.md"), []byte(strings.Repeat("a line of text\n", 300)), 0o644)
	for _, f := range []string{"main.go", "README.md", "go.mod"} {
		a.openFile(hs.path(f), true)
	}
	hs.settle()
	a.activate(a.column(), 1, true)
	a.activeEditor().selectRange(3000, 3000, true)
	hs.settle()
	d := a.activeEditor()
	y, caret := d.vscroll.Y, d.caret
	if y == 0 || caret != 3000 {
		t.Fatalf("scrolled to %v, caret %d before closing", y, caret)
	}
	a.closeActive()
	hs.settle()
	if n := len(a.column().tabs); n != 2 {
		t.Fatalf("%d tabs after closing", n)
	}
	a.run("file.reopenClosed")
	hs.settle()
	tabs := a.column().tabs
	if len(tabs) != 3 || tabs[1].path() != hs.path("README.md") || a.column().active != 1 {
		t.Fatalf("reopened at %d of %d tabs", a.column().active, len(tabs))
	}
	if d := a.activeEditor(); d.vscroll.Y != y || d.caret != caret {
		t.Errorf("reopened scrolled to %v, caret %d; want %v, %d", d.vscroll.Y, d.caret, y, caret)
	}

	// Close All, then each comes back, the last closed first.
	a.run("file.reopenClosed") // nothing left: nothing happens
	a.closeTabs(slices.Clone(a.column().tabs), nil, nil)
	hs.settle()
	if n := len(a.editorTabs()); n != 0 {
		t.Fatalf("%d tabs after Close All", n)
	}
	// A file gone from the disk is passed over.
	os.Remove(hs.path("go.mod"))
	a.run("file.reopenClosed")
	a.run("file.reopenClosed")
	a.run("file.reopenClosed")
	hs.settle()
	var got []string
	for _, t := range a.column().tabs {
		got = append(got, filepath.Base(t.path()))
	}
	if strings.Join(got, " ") != "main.go README.md" {
		t.Errorf("tabs %v after reopening", got)
	}

	// Another folder forgets them.
	a.closeActive()
	a.openFolder(t.TempDir())
	a.run("file.reopenClosed")
	if len(a.closed) != 0 || len(a.column().tabs) != 1 {
		t.Errorf("closed tabs kept across folders: %d", len(a.closed))
	}
}

func TestReopenClosedTabInItsColumn(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.splitRight()
	a.openFile(hs.path("README.md"), true)
	hs.settle()
	right := a.column()
	a.closeActive()
	a.activate(a.cols[0], 0, true)
	a.run("file.reopenClosed")
	hs.settle()
	if a.column() != right || right.tab().path() != hs.path("README.md") || len(right.tabs) != 2 {
		t.Errorf("reopened in column %d, not the right one", a.col)
	}
}

func TestReopenClosedTabBringsBackItsColumn(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.openFile(hs.path("go.mod"), true)
	a.splitRight()
	a.openFile(hs.path("README.md"), true)
	hs.settle()
	right := a.column()
	weight := right.weight
	// The right column goes with its last tab.
	a.closeActive()
	a.closeActive()
	hs.settle()
	if len(a.cols) != 1 {
		t.Fatalf("%d columns after closing the right one's tabs", len(a.cols))
	}
	a.run("file.reopenClosed")
	hs.settle()
	if len(a.cols) != 2 || a.col != 1 || a.column().weight != weight {
		t.Fatalf("%d columns, column %d has the keyboard", len(a.cols), a.col)
	}
	// The other tab closed from it goes back to it too, where it was.
	a.run("file.reopenClosed")
	hs.settle()
	var got []string
	for _, t := range a.cols[1].tabs {
		got = append(got, filepath.Base(t.path()))
	}
	if len(a.cols) != 2 || strings.Join(got, " ") != "go.mod README.md" {
		t.Errorf("%d columns, the right one has %v", len(a.cols), got)
	}
}

func TestClosedTerminalIsDeleted(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a shell")
	}
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.run("view.terminalTab")
	hs.settle()
	shell := a.activeTab().term
	if err := hs.tt.Click(shell.tab.closeLabel()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	a.run("file.reopenClosed")
	hs.settle()
	if len(a.terms) != 0 || len(a.closed) != 0 || len(a.column().tabs) != 1 || a.activeTab().path() != hs.path("main.go") || a.window.PanelOpen {
		t.Fatal("closing an editor terminal left a terminal to reopen or displaced the file")
	}
	hs.until("closed shell process", func() bool {
		select {
		case <-shell.term.Done():
			return true
		default:
			return false
		}
	})
}

// With terminalLocation "editor", New Terminal opens a tab of the editor
// area, while Toggle Terminal still shows the panel. Set back, New
// Terminal opens in the panel again.
func TestTerminalLocation(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a shell")
	}
	hs := newHarness(t, true)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	a.run("view.terminalLocation")
	if a.settings.TerminalLocation != "editor" {
		t.Fatalf("terminalLocation %q after the toggle", a.settings.TerminalLocation)
	}
	a.run("view.newTerminal")
	hs.settle()
	tabs := a.column().tabs
	if len(tabs) != 2 || tabs[1].term == nil || tabs[1].term.dir != hs.root || a.column().active != 1 {
		t.Fatalf("no terminal tab: %d tabs", len(tabs))
	}
	if a.panelTerm != nil || a.window.PanelOpen {
		t.Errorf("the panel opened too: open %v", a.window.PanelOpen)
	}
	a.openTerminal(filepath.Join(hs.root, "web"))
	hs.settle()
	if tabs := a.column().tabs; len(tabs) != 3 || tabs[2].term == nil || tabs[2].term.dir != filepath.Join(hs.root, "web") {
		t.Fatalf("Open in Terminal: %d tabs", len(tabs))
	}
	selected := a.activeTab().term
	a.run("view.terminal")
	hs.settle()
	if a.panelTerm != selected || !a.window.PanelOpen || len(a.column().tabs) != 1 || len(a.panelTerms()) != 2 {
		t.Fatal("Toggle Terminal didn't dock the existing terminals with the active one selected")
	}
	a.run("view.terminalLocation")
	panel := a.panelTerm
	a.run("view.newTerminal")
	hs.settle()
	if a.settings.TerminalLocation != "" || a.panelTerm == panel || len(a.column().tabs) != 1 || len(a.panelTerms()) != 3 {
		t.Errorf("New Terminal with terminalLocation %q: %d tabs", a.settings.TerminalLocation, len(a.column().tabs))
	}
	a.closeTerminals()
}

// The panel's header has a menu that moves the panel to a side, and back.
func TestTerminalPanelHeaderMenu(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a shell")
	}
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.run("view.terminal")
	hs.settle()
	if err := tt.RightClick("TERMINAL"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "Panel Position") || !slices.Contains(menu, "Hide Panel") {
		t.Fatalf("the panel's menu: %q", menu)
	}
	if err := tt.ChooseMenuItem("Panel Position", "Right"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.panelSide() != "right" || !a.window.PanelOpen {
		t.Fatalf("the panel is at %s, open %v", a.panelSide(), a.window.PanelOpen)
	}
	if err := tt.RightClick("TERMINAL"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Panel Position", "Bottom"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.panelSide() != "bottom" || a.settings.PanelPosition != "" {
		t.Fatalf("the panel is at %s (%q)", a.panelSide(), a.settings.PanelPosition)
	}
}

// Off macOS the title bar's menu has the menu bar's menus, without the
// edit roles it cannot send to the focused element, and runs their items.
func TestTitleBarMenu(t *testing.T) {
	onOS(t, "linux")
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	if err := tt.Click("Menu"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "File") || !slices.Contains(menu, "View") {
		t.Fatalf("the title bar's menu: %q", menu)
	}
	if err := tt.ChooseMenuItem("Edit", "Copy"); err == nil {
		t.Error("Copy is in the Edit menu")
	}
	if err := tt.Click("Menu"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("View", "Word Wrap"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !a.settings.WordWrap {
		t.Error("Word Wrap is not on")
	}
}

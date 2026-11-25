package main

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo"
	mtransfer "github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func tabWindowHarness(t *testing.T, src *harness) *harness {
	t.Helper()
	state := src.a.settings.NewWindow()
	state.Folder, state.Detached = src.a.root, true
	host := &fakeHost{}
	a := newApp(host, src.a.settings, state)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1040, 720)
	dst := &harness{t: t, a: a, h: host, root: state.Folder, tt: tt}
	dst.settle()
	src.h.tabWindow = func(*image.Point) *app { return a }
	src.h.projects, host.projects = []*app{src.a, a}, []*app{a, src.a}
	return dst
}

func TestDetachTabShowsOnlyEditableContent(t *testing.T) {
	src := newHarness(t, false)
	src.a.settings.AutoSave = false
	path := src.path("main.go")
	src.a.openFile(path, true)
	src.settle()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	dst.a.window.SidebarClosed, dst.a.window.PanelOpen = false, true
	dst.settle()
	col := dst.a.column()
	if _, ok := dst.tt.Find("Editor main.go"); !ok {
		t.Fatal("the detached window has no editor")
	}
	if body := col.body.Bounds(dst.a.review.ctx); body != (ui.Rect{Y: titleBarH, W: 1040, H: 720 - titleBarH}) {
		t.Fatalf("the tab content does not fill the window below its title: %+v", body)
	}
	for _, label := range []string{col.tab().closeLabel(), locales.PrefsTitle(), "Ln 1, Col 1", dst.a.withKey(locales.CmdViewSplitRight(), "view.splitRight")} {
		if _, present := dst.tt.Find(label); present {
			t.Errorf("workspace control %q in detached editor", label)
		}
	}
	dst.tt.Type("// detached\n")
	dst.settle()
	if dst.h.title != "● main.go" || !strings.Contains(col.tab().ed.buf.String(), "// detached") {
		t.Fatalf("detached editor did not take input or show the modified title: %q", dst.h.title)
	}
	dst.a.run("file.save")
	dst.settle()
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "// detached") || dst.h.title != "main.go" {
		t.Fatalf("file not saved from detached window: %q, %v, title %q", data, err, dst.h.title)
	}
}

func TestDetachTabMenuPreservesDirtyBackgroundEditor(t *testing.T) {
	src := newHarness(t, false)
	a := src.a
	a.settings.AutoSave = false
	a.openFile(src.path("main.go"), true)
	src.settle()
	tab := a.activeTab()
	before := tab.ed.buf.String()
	tab.ed.edit([2]int{0, 0}, textEdit{start: 0, end: 0, text: "// draft\n"})
	waited := false
	a.waitClosed(func() { waited = true })
	a.newFile()
	active := a.activeTab()
	src.settle()
	dst := tabWindowHarness(t, src)
	x, y := src.tabCenter(tab.name())
	src.tt.RightClickAt(x, y)
	src.settle()
	if err := src.tt.ChooseMenuItem(locales.MoveToNewWindow()); err != nil {
		t.Fatal(err)
	}
	src.settle()
	dst.settle()
	ed := dst.a.activeEditor()
	if waited || a.activeTab() != active || len(a.column().tabs) != 1 || len(dst.a.column().tabs) != 1 || ed == nil || !ed.dirty() || ed.buf.String() != "// draft\n"+before {
		t.Fatal("moving the background tab changed the active tab, closed the document, or lost the edit")
	}
	if !ed.undo() || ed.buf.String() != before || ed.dirty() {
		t.Fatal("undo in the new window did not restore the saved file")
	}
	ed.edit([2]int{0, 0}, textEdit{start: 0, end: 0, text: "more changes"})
	dst.h.choice = 2
	dst.a.run("file.close")
	dst.settle()
	if waited || dst.h.closed || dst.a.activeEditor() != ed || !ed.dirty() {
		t.Fatal("canceling close lost the detached editor or completed its waiter")
	}
	dst.h.choice = 1
	dst.a.closeActive()
	dst.settle()
	if !waited || !dst.h.closed || src.h.closed || a.activeTab() != active {
		t.Fatal("closing the detached document did not close only its window and finish its CLI waiter")
	}
}

func TestDetachTabKeepsWorkspaceActionsOut(t *testing.T) {
	src := newHarness(t, false)
	src.a.openFile(src.path("main.go"), true)
	src.settle()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	tab := dst.a.activeTab()
	for _, id := range []string{"view.splitRight", "view.splitDown", "view.sidebar", "view.newTerminal", "agent.newChat"} {
		dst.a.run(id)
	}
	dst.a.run("file.new")
	if ed := src.a.activeEditor(); ed == nil || ed.path != "" || dst.h.showed != src.a {
		t.Fatal("new file did not open in the workspace")
	}
	dst.a.openFileAt(src.path("go.mod"), 1, true)
	src.settle()
	dst.settle()
	if src.a.activeTab().name() != "go.mod" || len(dst.a.cols) != 1 || len(dst.a.column().tabs) != 1 || dst.a.activeTab() != tab || dst.a.window.PanelOpen || len(dst.a.terms) != 0 {
		t.Fatal("workspace command or file navigation changed the single-tab window")
	}
}

func TestDetachTabSplitSharesEditsUntilLastWindowCloses(t *testing.T) {
	for _, closeSource := range []bool{true, false} {
		src := newHarness(t, false)
		src.a.settings.AutoSave = false
		path := src.path("main.go")
		src.a.openFile(path, true)
		src.settle()
		src.a.splitRight()
		src.settle()
		waited := false
		src.a.waitClosed(func() { waited = true })
		dst := tabWindowHarness(t, src)
		src.a.detachTab(src.a.activeTab(), nil)
		first, second := src.a.activeEditor(), dst.a.activeEditor()
		if first == nil || second == nil || first == second || first.document != second.document {
			t.Fatal("tearing a split created divergent buffers")
		}
		first.edit(first.sel, textEdit{0, 0, "// first\n"})
		second.edit(second.sel, textEdit{0, 0, "// second\n"})
		remaining := src
		if closeSource {
			src.a.shutdown()
			remaining = dst
		} else {
			dst.a.closeActive()
		}
		if waited || len(first.views) != 1 || !strings.HasPrefix(remaining.a.activeEditor().buf.String(), "// second\n// first\n") {
			t.Fatal("closing one window lost edits or finished the CLI waiter early")
		}
		remaining.a.saveActive()
		remaining.settle()
		if data, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(data), "// second\n// first\n") {
			t.Fatalf("saving the remaining view lost an edit: %q, %v", data, err)
		}
		remaining.a.closeActive()
		if !waited {
			t.Fatal("closing the last view did not finish its waiter")
		}
	}
}

func TestDetachTabDropAndCancellation(t *testing.T) {
	src := newHarness(t, false)
	src.a.newFile()
	tab := src.a.activeTab()
	tab.ed.buf.Set("unsaved scratch")
	dst := tabWindowHarness(t, src)
	outside := image.Pt(-40, 160)
	opened := 0
	src.h.tabWindow = func(drop *image.Point) *app {
		if drop == nil || *drop != outside {
			t.Fatal("not the outside release point")
		}
		opened++
		return dst.a
	}
	for _, result := range []mtransfer.Result{
		{Operation: mtransfer.Copy, Point: outside}, {Operation: mtransfer.Move, Point: outside},
		{Canceled: true, Escaped: true, Point: outside}, {Err: errors.New("drag failed"), Point: outside},
	} {
		src.a.tabDragDone(tab, result)
	}
	src.a.drain()
	if opened != 0 || src.a.activeTab() != tab {
		t.Fatal("accepted drop, cancellation or error detached the tab")
	}
	for range 2 {
		src.a.tabDragDone(tab, mtransfer.Result{Canceled: true, Point: outside})
	}
	src.a.drain()
	dst.settle()
	if opened != 1 || src.a.activeTab() != nil || dst.a.activeEditor().buf.String() != "unsaved scratch" || !dst.a.activeEditor().dirty() {
		t.Fatal("outside release did not move the scratch tab exactly once")
	}
}

func TestDetachTabUnavailableDestinationKeepsTab(t *testing.T) {
	src := newHarness(t, false)
	src.a.newFile()
	tab := src.a.activeTab()
	src.a.detachTab(tab, nil)
	if src.a.activeTab() != tab || len(src.a.docs) != 1 {
		t.Fatal("source tab was removed before the destination existed")
	}
}

func TestReturnTabMenuRestoresPositionAndLiveDocument(t *testing.T) {
	src := newHarness(t, false)
	src.a.settings.AutoSave = false
	for _, name := range []string{"go.mod", "main.go", "README.md"} {
		src.a.openFile(src.path(name), true)
	}
	src.settle()
	col := src.a.column()
	left, moving, right := col.tabs[0], col.tabs[1], col.tabs[2]
	before, doc := moving.ed.buf.String(), moving.ed.document
	src.a.activate(col, 1, true)
	closed := false
	src.a.waitClosed(func() { closed = true })
	dst := tabWindowHarness(t, src)
	src.a.detachTab(moving, nil)
	dst.settle()
	ed := dst.a.activeEditor()
	ed.edit(ed.sel, textEdit{0, 0, "// draft\n"})
	dst.settle()
	box, ok := dst.tt.Find("Editor main.go")
	if !ok {
		t.Fatal("no detached editor")
	}
	dst.tt.RightClickAt(box.X+50, box.Y+40)
	dst.settle()
	if err := dst.tt.ChooseMenuItem(locales.MoveToMainWindow()); err != nil {
		t.Fatal(err)
	}
	dst.settle()
	src.settle()
	back := src.a.activeEditor()
	if back == nil || back.document != doc || !back.dirty() || len(col.tabs) != 3 || col.tabs[0] != left || col.tabs[2] != right || col.active != 1 {
		t.Fatal("return did not put the live editor between its original neighbours")
	}
	if !dst.h.closed || src.h.closed || dst.a.activeTab() != nil || len(dst.a.docs) != 0 || closed || len(doc.views) != 1 || dst.h.showed != src.a {
		t.Fatal("return closed a waiter or retained an extra window or document view")
	}
	if !back.undo() || back.buf.String() != before || back.dirty() || !back.redo() {
		t.Fatal("the undo history did not travel back with the editor")
	}
	src.a.saveActive()
	src.settle()
	if data, err := os.ReadFile(doc.path); err != nil || string(data) != "// draft\n"+before {
		t.Fatalf("save after docking: %q, %v", data, err)
	}
	src.a.closeActive()
	if !closed {
		t.Fatal("the CLI waiter did not follow the returned document")
	}
}

func TestReturnTabDropPlacementAndSharedSplit(t *testing.T) {
	src := newHarness(t, false)
	src.a.settings.AutoSave = false
	src.a.openFile(src.path("main.go"), true)
	src.settle()
	col, doc := src.a.column(), src.a.activeEditor().document
	src.a.splitRight()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	src.a.openFile(src.path("go.mod"), true)
	src.settle()
	stay := src.a.activeTab()
	returned := dst.a.activeTab()
	returned.ed.edit(returned.ed.sel, textEdit{0, 0, "draft"})
	returned.ed.sel = [2]int{2, 4}
	drag := dragItem{tab: returned, detached: dst.a}
	if !src.a.acceptsReturn(drag) || dst.a.acceptsReturn(drag) {
		t.Fatal("only the workspace should accept this drag")
	}
	for range 2 {
		src.a.dropInColumn(drag, col, len(col.tabs))
	}
	if len(col.tabs) != 2 || col.tabs[0] != stay || len(src.a.docs) != 2 || len(doc.views) != 1 || doc.views[0].owner != src.a || len(dst.a.docs) != 0 {
		t.Fatal("dropping beside an existing shared copy registered duplicate editors or documents")
	}
	back := src.a.activeEditor()
	if back.document != doc || back.sel != [2]int{2, 4} || !strings.HasPrefix(back.buf.String(), "draft") {
		t.Fatal("the dropped copy lost its selection or shared edits")
	}
	dst.settle()
	if !dst.h.closed || src.h.closed || src.h.showed != src.a {
		t.Fatal("drop did not leave the workspace in front")
	}
}

func TestReturnTabWithoutDestinationStaysOpen(t *testing.T) {
	src := newHarness(t, false)
	src.a.newFile()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	dst.settle()
	tab := dst.a.activeTab()
	tab.ed.buf.Set("unsaved scratch")
	dst.h.projects = []*app{dst.a}
	dst.a.run("file.returnTab")
	dst.settle()
	if dst.a.activeTab() != tab || dst.h.closed || !tab.ed.dirty() {
		t.Fatal("an unavailable destination discarded the detached scratch editor")
	}
	dst.h.projects = []*app{dst.a, src.a}
	if err := dst.tt.Click(locales.MoveToMainWindow()); err != nil {
		t.Fatal(err)
	}
	dst.settle()
	src.settle()
	if !dst.h.closed || src.a.activeEditor().document != tab.ed.document || src.a.activeEditor().buf.String() != "unsaved scratch" {
		t.Fatal("the title bar action did not move an untitled tab into its empty workspace")
	}
}

func TestReturnTabFileMenuTracksAvailability(t *testing.T) {
	src := newHarness(t, false)
	src.a.newFile()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	doc := dst.a.activeEditor().document
	menuItem := func() *mygo.MenuItem {
		return dst.a.menu(false, func(id string, _ *mygo.Window) { dst.a.run(id) }).ItemByID("file.returnTab")
	}
	doc.sync = syncSaving
	step(dst.tt)
	if item := menuItem(); item == nil || !item.Disabled || dst.a.menuCanReturn {
		t.Fatal("the detached File menu offered a document mid-save")
	}
	doc.sync = syncIdle
	step(dst.tt)
	item := menuItem()
	if item == nil || item.Disabled || !dst.a.menuCanReturn {
		t.Fatal("the detached File menu did not enable return when the save ended")
	}
	item.Click(item, nil)
	dst.settle()
	if !dst.h.closed || src.a.activeEditor().document != doc {
		t.Fatal("the native File menu did not return the live document")
	}
}

func TestReturnTabToReopenedWorkspace(t *testing.T) {
	src := newHarness(t, false)
	src.a.settings.AutoSave = false
	src.a.openFile(src.path("main.go"), true)
	src.settle()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	src.a.shutdown()
	returning := dst.a.activeEditor().document
	returning.buf.Set("detached draft")
	reopened := newHarness(t, false)
	reopened.a.settings.AutoSave = false
	reopened.a.openFolder(src.root)
	reopened.a.openFile(src.path("go.mod"), true)
	reopened.settle()
	resident := reopened.a.activeEditor().document
	if resident.id != returning.id {
		t.Fatal("fixture requires independently allocated documents with the same ID")
	}
	dst.h.projects = []*app{dst.a, reopened.a}
	dst.a.returnTab()
	dst.settle()
	reopened.settle()
	if !dst.h.closed || reopened.a.activeEditor().document != returning || returning.buf.String() != "detached draft" || resident.id == returning.id || len(reopened.a.docs) != 2 {
		t.Fatal("return to a reopened workspace lost the draft or reused another document's ID")
	}
}

func TestReturnTabConflictKeepsBothBuffers(t *testing.T) {
	src := newHarness(t, false)
	src.a.settings.AutoSave = false
	path := src.path("main.go")
	src.a.openFile(path, true)
	src.settle()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	src.h.projects = nil
	src.a.openFile(path, true)
	src.settle()
	resident, moving := src.a.activeEditor(), dst.a.activeEditor()
	resident.buf.Set("local draft")
	moving.buf.Set("separate draft")
	src.h.projects = []*app{src.a, dst.a}
	dst.a.returnTab()
	if dst.h.closed || src.a.activeEditor() != resident || dst.a.activeEditor() != moving || resident.buf.String() != "local draft" || moving.buf.String() != "separate draft" || len(src.a.notes) == 0 {
		t.Fatal("return silently replaced an independently edited copy of the same path")
	}
}

func TestReturnTabDiffUsesDetachedSource(t *testing.T) {
	src := newHarness(t, false)
	src.gitRepo()
	src.a.settings.AutoSave = false
	path := src.path("live.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src.settleGit()
	src.a.openFile(path, true)
	src.settle()
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	source := dst.a.activeEditor()
	source.edit(source.sel, textEdit{0, 0, "source "})
	src.a.showChanges(diffTarget{path: path, base: diffIndex}, true)
	review := src.a.activeTab()
	src.settleDiff(review.diff)
	if src.a.docIndex(path) >= 0 {
		t.Fatal("displaying a diff gave it an unowned editing buffer")
	}
	inline := diffEditOf(t, src, review.diff, path, 0)
	if inline.doc != source.document || inline.ed.owner != src.a {
		t.Fatal("editing after detach reread the file instead of using its live text")
	}
	inline.diffReplace("diff ")
	if source.buf.String() != "diff source original\n" {
		t.Fatalf("edits diverged before docking: %q", source.buf.String())
	}
	dst.a.returnTab()
	dst.settle()
	src.settle()
	back := src.a.activeEditor()
	if back == nil || back.document != inline.doc || len(src.a.docs) != 1 || !dst.h.closed {
		t.Fatal("docking duplicated or discarded the diff's document")
	}
	src.a.saveActive()
	src.settle()
	if data, err := os.ReadFile(path); err != nil || string(data) != "diff source original\n" {
		t.Fatalf("saving after docking lost one side's edits: %q, %v", data, err)
	}
}

func TestReturnTabMergeKeepsDetachedDraftAndWaiter(t *testing.T) {
	src := newHarness(t, false)
	src.a.settings.AutoSave = false
	path := src.path("main.go")
	src.a.openFile(path, true)
	src.settle()
	closed := false
	src.a.waitClosed(func() { closed = true })
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	source := dst.a.activeEditor()
	source.buf.Set("merge draft")
	merge := &mergeView{path: path, loaded: true}
	src.a.makeMergeResult(merge, []byte("disk text"), nil)
	if merge.doc != source.document || !source.dirty() || source.buf.String() != "merge draft" {
		t.Fatal("opening a merge replaced the detached draft with disk contents")
	}
	col := src.a.column()
	mergeTab := &colTab{merge: merge}
	src.a.addTab(col, 0, mergeTab)
	dst.a.returnTab()
	src.a.closeActive()
	if closed || len(merge.doc.views) != 1 {
		t.Fatal("closing the returned source closed a document still open in the merge")
	}
	src.h.choice = 1
	src.a.closeTab(mergeTab, nil, nil)
	if !closed || src.a.docIndex(path) >= 0 {
		t.Fatal("the last merge view failed to release its document and CLI waiter")
	}
}

func TestDetachTabKeepsTerminal(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("starts a POSIX shell")
	}
	t.Setenv("SHELL", "/bin/sh")
	src := newHarness(t, false)
	dst := tabWindowHarness(t, src)
	t.Cleanup(src.a.closeTerminals)
	t.Cleanup(dst.a.closeTerminals)
	src.a.run("view.terminalTab")
	src.settle()
	tab := src.a.activeTab()
	if tab == nil || tab.term == nil {
		t.Fatal("no terminal tab")
	}
	shell := tab.term.term
	shell.Send([]byte("tab_value=kept\n"))
	src.a.detachTab(tab, nil)
	src.a.terminalToPanel(tab.term)
	if src.a.panelTerm != nil || tab.term.tab != dst.a.activeTab() {
		t.Fatal("a pending restore in the source window reclaimed the detached terminal")
	}
	src.a.closeTerminals()
	dst.settle()
	if _, shown := dst.tt.Find(locales.PanelRestore()); shown {
		t.Fatal("the detached terminal has a workspace panel restore button")
	}
	if len(src.a.terms) != 0 || len(dst.a.terms) != 1 || dst.a.activeTab().term.term != shell {
		t.Fatal("moving a terminal restarted or left behind its shell")
	}
	shell.Send([]byte("printf 'moved:%s\\n' \"$tab_value\"\n"))
	dst.until("output from the moved shell", func() bool { return strings.Contains(shell.Text(), "moved:kept") })
	resident := testPanelSession(t, src.a)
	src.a.showTerminal(resident)
	src.settle()
	if !src.a.window.PanelOpen || len(dst.a.terms) != 1 || dst.a.activeTab().term.term != shell {
		t.Fatal("opening a workspace terminal disturbed the independent window")
	}
	dst.a.returnTab()
	dst.settle()
	dst.a.closeTerminals()
	if !dst.h.closed || len(src.a.terms) != 2 || src.a.activeTab().term.term != shell || len(dst.a.terms) != 0 || src.a.window.PanelOpen || src.a.panelTerm != nil || resident.tab == nil {
		t.Fatal("docking lost or restarted the shell")
	}
	shell.Send([]byte("printf 'returned:%s\\n' \"$tab_value\"\n"))
	src.until("output back in the workspace", func() bool { return strings.Contains(shell.Text(), "returned:kept") })
	if _, shown := src.tt.Find(locales.PanelRestore()); !shown {
		t.Fatal("the returned workspace terminal has no panel restore button")
	}
}

func TestReturnTaskTabLeavesPanelShellInPlace(t *testing.T) {
	source := taskTermHarness(t)
	workspace := source.a
	workspace.openFile(source.path("main.go"), true)
	workspace.newTerminal()
	shell := workspace.panelTerm
	task := workspace.launchTask(source.root, waitingTask(source.root, "Detached task"))
	if shell == nil || task == nil || task.tab == nil {
		t.Fatal("expected a panel shell and an editor task")
	}
	process := task.term
	detached := tabWindowHarness(t, source)
	workspace.detachTab(task.tab, nil)
	detached.settle()
	if detached.a.activeTab() != task.tab || detached.a.reviewTerminal() != nil || workspace.panelTerm != shell || !workspace.window.PanelOpen {
		t.Fatal("detaching the task changed the shell or exposed the task as a shell")
	}
	detached.a.returnTab()
	detached.settle()
	source.settle()
	if workspace.activeTab() != task.tab || task.term != process || !taskRunning(task) || !detached.h.closed {
		t.Fatal("returning the task did not preserve its live editor session")
	}
	if workspace.panelTerm != shell || !workspace.window.PanelOpen || shell.tab != nil || len(workspace.panelTerms()) != 1 {
		t.Fatal("returning the task moved the workspace's panel shell")
	}
	if _, shown := source.tt.Find(locales.PanelRestore()); shown || workspace.reviewTerminal() != shell {
		t.Fatal("the task was offered for docking or as a shell paste target")
	}
}

func TestDetachTabKeepsRunningAgent(t *testing.T) {
	src := newHarness(t, true)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(src.root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "agent-target.go")
	if err := os.WriteFile(target, []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src.a.settings.AgentServers = map[string]settings.AgentServer{"test": {Name: "Test Agent", Command: exe,
		Args: []string{"-test.run=^TestACPAppProcess$"}, Env: map[string]string{"FIVELEAF_ACP_UI_TEST": "1", "FIVELEAF_ACP_UI_ROOT": root}}}
	src.a.showAgent()
	chat := src.a.agent
	src.until("agent connection", func() bool { return chat.sessionID != "" })
	chat.draft = "Review the greeting."
	src.a.sendAgent()
	src.until("permission", func() bool { return len(chat.permissions) == 1 })
	chat.draft = "A follow-up draft"
	client, gen := chat.client, chat.generation
	dst := tabWindowHarness(t, src)
	src.a.detachTab(src.a.activeTab(), nil)
	src.a.shutdown()
	dst.settle()
	if dst.a.activeTab().tool.chat != chat || dst.a.agent != chat || chat.client != client || chat.generation != gen || chat.draft != "A follow-up draft" || len(chat.permissions) != 1 {
		t.Fatal("the running agent, draft or pending permission was lost on detaching")
	}
	chat.permissions[0].choose(string(chat.permissions[0].options[0].OptionId))
	dst.until("reply after source closed", func() bool { return !chat.busy })
	if got, err := os.ReadFile(target); err != nil || string(got) != "package demo\n// agent change\n" || chat.err != "" || chat.client != client {
		t.Fatalf("detached permission did not complete on its original connection: %q %v %s", got, err, chat.err)
	}
}

func TestDetachTabHistoryTargetOpensInWorkspace(t *testing.T) {
	src := newHarness(t, false)
	dst := tabWindowHarness(t, src)
	history := dst.a.blankChat()
	history.continuation = &sessionContinuation{loaded: true}
	history.draft = "my follow-up"
	dst.a.openChatPage(history)
	dst.a.selectChatAgent(history, "alternate")
	if len(dst.a.column().tabs) != 1 || dst.a.activeTab().tool.chat != history || history.draft != "my follow-up" {
		t.Fatal("choosing another agent displaced the detached history or its draft")
	}
	if tab := src.a.activeTab(); tab == nil || tab.tool == nil || tab.tool.chat.provider != "alternate" {
		t.Fatal("the selected agent did not open in the workspace")
	}
}

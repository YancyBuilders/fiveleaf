package main

import (
	"errors"
	"os"
	"slices"
	"testing"
)

func diffNavigationCloseFixture(t *testing.T) (*harness, *colTab, *document) {
	t.Helper()
	hs := newHarness(t, false)
	hs.a.settings.AutoSave = false
	path := hs.path("close.txt")
	if err := os.WriteFile(path, []byte("call()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := diffNavReview(hs, path, "old()\n", "call()\n", layoutSplit)
	tab := hs.a.activeTab()
	row := diffNavRow(t, v, 1, 0)
	hs.a.diffNavigate(v, v.navigationPoint(diffPoint{row, 1}, 1), false)
	hs.settle()
	i := hs.a.docIndex(path)
	if i < 0 || hs.a.docs[i].locked() || len(v.navigationDocs) != 1 {
		t.Fatal("review did not retain its current document")
	}
	return hs, tab, hs.a.docs[i]
}

func TestDiffNavigationCloseLifecycle(t *testing.T) {
	for choice, name := range []string{"save", "discard", "cancel"} {
		t.Run(name, func(t *testing.T) {
			hs, review, d := diffNavigationCloseFixture(t)
			a := hs.a
			d.buf.Set("changed()\n")
			hs.h.choice = choice
			done, canceled := false, false
			a.closeAll(func() { done = true }, func() { canceled = true })
			hs.settle()
			if len(hs.h.asked) != 1 {
				t.Fatalf("save confirmations = %v", hs.h.asked)
			}
			col, _ := a.locate(review)
			if choice == 2 {
				if done || !canceled || col == nil || a.docIndex(d.path) < 0 || d.buf.String() != "changed()\n" {
					t.Fatal("Cancel failed to retain the review and its changed document")
				}
			} else if !done || canceled || col != nil || a.docIndex(d.path) >= 0 {
				t.Fatal("closing retained or reopened the document after Save/Discard")
			}
			want := "call()\n"
			if choice == 0 {
				want = "changed()\n"
			}
			if disk, err := os.ReadFile(d.path); err != nil || string(disk) != want {
				t.Fatalf("disk = %q, %v; want %q", disk, err, want)
			}
		})
	}
}

func TestDiffNavigationLastSharedViewConfirmsClose(t *testing.T) {
	hs, first, d := diffNavigationCloseFixture(t)
	a := hs.a
	v := diffNavReview(hs, d.path, "old()\n", "call()\n", layoutSplit)
	second := a.activeTab()
	row := diffNavRow(t, v, 1, 0)
	a.diffNavigate(v, v.navigationPoint(diffPoint{row, 1}, 1), false)
	hs.settle()
	if len(d.views) != 2 || len(v.navigationDocs) != 1 || v.navigationDocs[0].doc != d {
		t.Fatal("the two reviews do not share one current document")
	}
	a.openFile(d.path, true)
	source := a.activeTab()
	d.buf.Set("changed()\n")
	a.closeTab(source, nil, nil)
	a.closeTab(first, nil, nil)
	if len(hs.h.asked) != 0 || len(d.views) != 1 {
		t.Fatal("a non-final view asked to discard the shared document")
	}
	hs.h.holdClose = true
	done, canceled := false, false
	a.closeAll(func() { done = true }, func() { canceled = true })
	if done || canceled || len(hs.h.held) != 1 {
		t.Fatal("closeAll did not wait for the final review's confirmation")
	}
	hs.h.held[0](2)
	if col, _ := a.locate(second); col == nil || done || !canceled || d.buf.String() != "changed()\n" {
		t.Fatal("Cancel lost the last review or its document")
	}
	hs.h.holdClose, hs.h.choice = false, 1
	a.closeAll(func() { done = true }, nil)
	hs.settle()
	if !done || a.docIndex(d.path) >= 0 {
		t.Fatal("Discard reopened the changed document")
	}
}

func TestDiffNavigationPendingSaveDoesNotSaveNewerDiscardedText(t *testing.T) {
	hs, _, d := diffNavigationCloseFixture(t)
	a := hs.a
	saving := d.buf.Snapshot()
	d.sync = syncSaving // an ACP write uses nextSave when its disk work ends
	d.buf.Set("newer unsaved text\n")
	hs.h.choice = 1
	done := false
	a.closeAll(func() { done = true }, nil)
	if done || len(hs.h.asked) != 0 || hasQueuedSave(d) || len(d.queued) != 1 {
		t.Fatal("closing started a save or skipped the pending one")
	}
	d.wrote(saving, false)
	d.sync = syncIdle
	a.nextSave(d)
	hs.settle()
	if !done || len(hs.h.asked) != 1 || a.docIndex(d.path) >= 0 {
		t.Fatal("the pending save did not resume normal close confirmation")
	}
	if disk, err := os.ReadFile(d.path); err != nil || string(disk) != "call()\n" {
		t.Fatalf("Discard saved text written after the pending save began: %q, %v", disk, err)
	}
}

func TestDiffNavigationDiscardRechecksEarlierDocuments(t *testing.T) {
	hs, review, first := diffNavigationCloseFixture(t)
	a, v := hs.a, review.diff
	path := hs.path("second.txt")
	if err := os.WriteFile(path, []byte("second()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := agentSnapshotFile(agentChange{path: path, oldText: "old()\n", newText: "second()\n", existed: true})
	v.files = append(v.files, f)
	a.diffNavigate(v, &diffNavigationPoint{file: f, path: path, text: f.new.text, col: 1, newSide: true}, false)
	hs.settle()
	second := a.docs[a.docIndex(path)]
	first.buf.Set("first edit\n")
	second.buf.Set("second edit\n")
	hs.h.holdClose = true
	done, canceled := false, false
	a.closeAll(func() { done = true }, func() { canceled = true })
	if len(hs.h.held) != 1 {
		t.Fatal("did not ask about the first changed document")
	}
	hs.h.held[0](1) // approve discarding the first edit
	if len(hs.h.held) != 2 {
		t.Fatal("did not ask about the second changed document")
	}
	first.buf.Set("a later edit that was not approved\n")
	hs.h.held[1](1)
	if done || len(hs.h.held) != 3 {
		t.Fatal("an older Discard approval discarded a later edit")
	}
	hs.h.held[2](2)
	if done || !canceled || first.buf.String() != "a later edit that was not approved\n" || second.buf.String() != "second edit\n" {
		t.Fatal("Cancel failed to preserve the documents after a later edit")
	}
	if col, _ := a.locate(review); col == nil {
		t.Fatal("Cancel closed the review")
	}
}

func TestDiffNavigationDiscardRechecksDocumentChangedDuringConfirmation(t *testing.T) {
	hs, review, doc := diffNavigationCloseFixture(t)
	doc.buf.Set("first edit\n")
	hs.h.holdClose = true
	done, canceled := false, false
	hs.a.closeAll(func() { done = true }, func() { canceled = true })
	if len(hs.h.held) != 1 {
		t.Fatal("did not ask to discard the first edit")
	}
	doc.buf.Set("later edit\n")
	hs.h.held[0](1)
	if done || canceled || len(hs.h.held) != 2 {
		t.Fatal("Discard did not ask again after the document changed during confirmation")
	}
	hs.h.held[1](2)
	if col, _ := hs.a.locate(review); col == nil || !canceled || doc.buf.String() != "later edit\n" {
		t.Fatal("Cancel did not retain the review and the later edit")
	}
}

func TestSaveObserversPreserveExplicitQueuedSave(t *testing.T) {
	hs, _, d := diffNavigationCloseFixture(t)
	a := hs.a
	d.sync = syncSaving
	d.buf.Set("explicitly queued text\n")
	var calls []string
	a.requestSave(d, saveWaiter{done: func() { calls = append(calls, "save") }})
	a.requestSave(d, saveWaiter{waitOnly: true, done: func() { calls = append(calls, "observer") }})
	if !hasQueuedSave(d) {
		t.Fatal("an observer hid an explicitly requested save")
	}
	d.sync = syncIdle
	a.nextSave(d)
	if !slices.Equal(calls, []string{"save", "observer"}) || d.dirty() {
		t.Fatalf("save callbacks = %v, dirty %v", calls, d.dirty())
	}
	if disk, err := os.ReadFile(d.path); err != nil || string(disk) != "explicitly queued text\n" {
		t.Fatalf("explicit save was skipped: %q, %v", disk, err)
	}
}

func TestSaveObserversFollowFailureAndConflictCancellation(t *testing.T) {
	for _, state := range []syncState{syncSaving, syncConflict} {
		hs, _, d := diffNavigationCloseFixture(t)
		d.sync = state
		want := errors.New("write failed")
		if state == syncConflict {
			want = errNotSaved
		}
		var result []error
		d.waiters = []saveWaiter{{failed: func(err error) { result = append(result, err) }}}
		hs.a.requestSave(d, saveWaiter{waitOnly: true, failed: func(err error) { result = append(result, err) }})
		if hasQueuedSave(d) || len(d.queued) != 0 || len(d.waiters) != 2 {
			t.Fatalf("observer became an additional save: queued=%v waiters=%d", d.queued, len(d.waiters))
		}
		hs.a.answerSaves(d, want)
		if len(result) != 2 || result[0] != want || result[1] != want {
			t.Fatalf("pending save did not notify its observer: %v", result)
		}
	}
}

func inlineCloseFixture(t *testing.T, detached bool) (*harness, *colTab, *diffEdit) {
	t.Helper()
	hs := newHarness(t, false)
	hs.gitRepo()
	path := hs.path("close.txt")
	if err := os.WriteFile(path, []byte("call()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.settleGit()
	hs.a.settings.AutoSave = false
	hs.a.showChanges(diffTarget{path: path, base: diffIndex}, true)
	if detached {
		dst := tabWindowHarness(t, hs)
		hs.a.detachTab(hs.a.activeTab(), nil)
		hs = dst
	}
	tab := hs.a.activeTab()
	if tab == nil || tab.diff == nil || len(hs.a.column().tabs) != 1 {
		t.Fatal("expected a single working tree diff tab")
	}
	hs.settleDiff(tab.diff)
	return hs, tab, diffEditOf(t, hs, tab.diff, path, 0)
}

func TestDiffInlineCloseLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name       string
		choice     int
		detached   bool
		navigation bool
	}{
		{"detached save", 0, true, false},
		{"detached discard", 1, true, false},
		{"detached cancel", 2, true, false},
		{"tab discard with navigation", 1, false, true},
		{"tab cancel with navigation", 2, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hs, tab, edit := inlineCloseFixture(t, tc.detached)
			a, view, doc := hs.a, tab.diff, edit.doc
			if tc.navigation {
				row := diffEditRow(view, doc.path, 0)
				a.diffNavigate(view, view.navigationPoint(diffPoint{row, 1}, 1), false)
				if len(doc.views) != 2 {
					t.Fatalf("inline and navigation own %d views; want 2", len(doc.views))
				}
			}
			edit.diffReplace("draft ")
			a.endDiffEdit(view)
			hs.h.holdClose = true
			done, cancelled := 0, 0
			closed, stopped := func() { done++ }, func() { cancelled++ }
			if tc.detached {
				a.closeKeeping(closed, stopped)
			} else {
				a.closeTab(tab, closed, stopped)
			}
			if done != 0 || cancelled != 0 || a.activeTab() != tab || len(hs.h.held) != 1 {
				t.Fatalf("before answering: done=%d cancelled=%d prompts=%v", done, cancelled, hs.h.asked)
			}
			hs.h.held[0](tc.choice)
			hs.settle()
			if !slices.Equal(hs.h.asked, []string{"close close.txt"}) {
				t.Fatalf("unexpected confirmations: %v", hs.h.asked)
			}
			if tc.choice == 2 {
				if done != 0 || cancelled != 1 || a.activeTab() != tab || hs.h.closed || a.docIndex(doc.path) < 0 || doc.buf.String() != "draft call()\n" || view.navigationClosing {
					t.Fatal("Cancel did not keep the tab, draft and window ready for another close")
				}
			} else if done != 1 || cancelled != 0 || a.activeTab() != nil || hs.h.closed != tc.detached || a.docIndex(doc.path) >= 0 || len(doc.views) != 0 {
				t.Fatal("Save/Discard did not release the last document and finish the close exactly once")
			}
			want := "call()\n"
			if tc.choice == 0 {
				want = "draft call()\n"
			}
			if disk, err := os.ReadFile(doc.path); err != nil || string(disk) != want {
				t.Fatalf("disk = %q, %v; want %q", disk, err, want)
			}
		})
	}
}

func TestDiffCanDetachAfterInlineEdit(t *testing.T) {
	hs, tab, edit := inlineCloseFixture(t, false)
	a, view, doc := hs.a, tab.diff, edit.doc
	if view.canDetach() {
		t.Fatal("active inline editor allowed detach")
	}
	a.endDiffEdit(view)
	if !view.canDetach() {
		t.Fatal("clean buffer after leaving inline editing blocked detach")
	}
	edit.diffReplace("draft ")
	if view.canDetach() {
		t.Fatal("unsaved inline edits allowed detach")
	}
	a.saveDoc(doc, nil)
	if !view.canDetach() {
		t.Fatal("saved inline edits blocked detach")
	}
	doc.sync = syncSaving
	if view.canDetach() {
		t.Fatal("an in-flight write of a clean buffer allowed detach")
	}
	done, cancelled := 0, 0
	a.closeTab(tab, func() { done++ }, func() { cancelled++ })
	if done != 0 || cancelled != 0 || len(hs.h.asked) != 0 || hasQueuedSave(doc) {
		t.Fatal("close did not wait for the existing write")
	}
	doc.buf.Set("newer unsaved text\n")
	hs.h.holdClose = true
	doc.sync = syncIdle
	a.nextSave(doc)
	if done != 0 || cancelled != 0 || len(hs.h.held) != 1 {
		t.Fatal("close did not ask about the edits made during the write")
	}
	hs.h.held[0](1)
	hs.settle()
	if done != 1 || cancelled != 0 || a.docIndex(doc.path) >= 0 {
		t.Fatal("Discard did not finish the pending close")
	}
	if disk, err := os.ReadFile(doc.path); err != nil || string(disk) != "draft call()\n" {
		t.Fatalf("Discard wrote newer edits: disk=%q, err=%v", disk, err)
	}
}

func TestDiffInlineCloseKeepsOtherWindow(t *testing.T) {
	hs, tab, edit := inlineCloseFixture(t, false)
	a, view, doc := hs.a, tab.diff, edit.doc
	if edit.ed.owner != a {
		t.Fatal("inline editor has no owning window")
	}
	a.openFile(doc.path, true)
	other := tabWindowHarness(t, hs)
	a.detachTab(a.activeTab(), nil)
	source := other.a.activeEditor()
	if source == nil || source.document != doc || a.activeTab() != tab {
		t.Fatal("source in detached window did not share the inline document")
	}
	row := diffEditRow(view, doc.path, 0)
	a.diffNavigate(view, view.navigationPoint(diffPoint{row, 1}, 1), false)
	if len(doc.views) != 3 || slices.ContainsFunc(doc.views, func(ed *editor) bool { return ed != source && ed.owner != a }) {
		t.Fatal("inline and navigation views must belong to the diff's window")
	}
	edit.diffReplace("draft ")
	done, cancelled := 0, 0
	a.closeTab(tab, func() { done++ }, func() { cancelled++ })
	hs.settle()
	if done != 1 || cancelled != 0 || len(hs.h.asked) != 0 || a.docIndex(doc.path) >= 0 || !slices.Equal(doc.views, []*editor{source}) {
		t.Fatal("closing the diff did not release just its own views without prompting")
	}
	other.h.choice = 2
	other.a.closeActive()
	other.settle()
	if len(other.h.asked) != 1 || other.h.closed || other.a.activeEditor() != source || source.buf.String() != "draft call()\n" {
		t.Fatal("the last window did not retain the shared draft on Cancel")
	}
}

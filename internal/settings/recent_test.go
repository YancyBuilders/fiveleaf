package settings

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestRemoveRecentKeepsSessionAndWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	s.Recent = []string{first, second}
	w := s.NewWindow()
	w.Folder = second
	ses := &Session{Columns: []Column{{Tabs: []Tab{{File: filepath.Join(second, "main.go")}}}}}
	s.Sessions = map[string]*Session{second: ses}
	s.RemoveRecent(second)
	if !slices.Equal(s.Recent, []string{first}) || s.Sessions[second] != ses || len(s.Windows) != 1 || s.Windows[0] != w {
		t.Fatal("removing a recent entry changed a session or open window")
	}
	s.AddRecent(first)
	if s.Sessions[second] != ses {
		t.Fatal("opening another project deleted a removed project's session")
	}
	s.Flush()
	saved := Load(path)
	if !slices.Equal(saved.Recent, []string{first}) || saved.Sessions[second] == nil || len(saved.Windows) != 1 {
		t.Fatal("the recent removal or kept session did not survive saving")
	}
}

func TestRemoveRecentOpenProjectKeepsSessionOnClose(t *testing.T) {
	s := Default()
	dir, other := filepath.Join(t.TempDir(), "project"), filepath.Join(t.TempDir(), "other")
	w := s.NewWindow()
	w.Folder = dir
	s.AddRecent(dir)
	s.RemoveRecent(dir)
	s.AddRecent(other)
	// The window updates its live session after the recent entry went.
	w.Columns = []Column{{Tabs: []Tab{{File: filepath.Join(dir, "later.go"), Sel: [2]int{5, 7}}}}}
	s.Forget(w)
	ses := s.TakeSession(dir)
	if ses == nil || len(ses.Columns) != 1 || len(ses.Columns[0].Tabs) != 1 || ses.Columns[0].Tabs[0].Sel != [2]int{5, 7} {
		t.Fatalf("closing the removed project lost its current session: %#v", ses)
	}
	if !slices.Equal(s.Recent, []string{other}) {
		t.Fatal("keeping the session put the removed project back in the list")
	}
}

func TestDetachedWindowKeepsSeparateState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	dir := t.TempDir()
	s.AddRecent(dir)
	project := s.NewWindow()
	project.Folder = dir
	project.Columns = []Column{{Tabs: []Tab{{File: "project.go"}}}}
	doc := s.NewWindow()
	doc.Folder, doc.Detached = dir, true
	doc.Columns = []Column{{Tabs: []Tab{{File: "moved.go"}}}}
	empty := s.NewWindow()
	empty.Folder, empty.Detached = dir, true
	s.RemoveRecent(dir)
	s.Stash(doc)
	if stored := s.TakeSession(dir); stored == nil || stored.Columns[0].Tabs[0].File != "project.go" {
		t.Fatal("detached window overwrote the folder session")
	}
	s.Flush()
	restored := Load(path)
	if !slices.ContainsFunc(restored.Windows, func(w *Window) bool { return w.ID == doc.ID && w.Detached && w.Columns[0].Tabs[0].File == "moved.go" }) || slices.ContainsFunc(restored.Windows, func(w *Window) bool { return w.ID == empty.ID }) {
		t.Fatal("restart lost a detached document or restored an empty window")
	}
}

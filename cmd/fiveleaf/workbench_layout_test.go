package main

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestTitleBarOmitsFileCaption(t *testing.T) {
	for _, platform := range []struct {
		os  string
		bar ui.TitleBar
	}{
		{"darwin", ui.TitleBar{Height: titleBarH, Left: 80}},
		{"windows", ui.TitleBar{Height: titleBarH, Right: 138}},
		{"linux", ui.TitleBar{Height: titleBarH, Right: 110}},
	} {
		t.Run(platform.os, func(t *testing.T) {
			onOS(t, platform.os)
			hs := newHarness(t, true)
			a, tt := hs.a, hs.tt
			run := hs.gitRepo()
			branch := "feature/" + strings.Repeat("long-navigation-", 4)
			run("branch", "-m", branch)
			a.openFile(hs.path("main.go"), true)
			hs.settleGit()
			tt.SetTitleBar(platform.bar)
			for _, width := range []int{1200, 640} {
				tt.SetSize(width, 760)
				hs.settle()
				if caption, shown := tt.Find(a.windowTitle()); shown {
					t.Fatalf("width %d: the title bar repeats the file name: %v", width, caption)
				}
				labels := []string{locales.ProjectPickerTitle(), locales.WorktreeStatusTip("main"), locales.BranchLabel(branch), locales.PrefsTitle()}
				bounds := hs.layoutOf(labels...)
				for i, label := range labels {
					r := bounds[label]
					if math.IsNaN(float64(r.X+r.W)) || r.X < platform.bar.Left || r.X+r.W > float32(width)-platform.bar.Right {
						t.Errorf("window controls overlap %q: %v", label, r)
					}
					for _, other := range labels[i+1:] {
						if overlaps(r, bounds[other]) {
							t.Errorf("%q overlaps %q: %v / %v", label, other, r, bounds[other])
						}
					}
				}
				a.settings.AutoSave = false
				hs.insert("x")
				if tt.HasText(a.windowTitle()) || !strings.HasPrefix(a.windowTitle(), "● ") {
					t.Fatal("the file caption returned or the native window title lost its dirty marker")
				}
				sameLayout(t, "dirty file", bounds, hs.layoutOf(labels...))
			}
		})
	}
}

func TestEditorModeStaysOnLeftOfStatusBar(t *testing.T) {
	for _, mode := range []string{"vim", "helix"} {
		t.Run(mode, func(t *testing.T) {
			hs := newHarness(t, true)
			a, tt := hs.a, hs.tt
			a.settings.EditorMode = mode
			a.openFile(hs.path("main.go"), true)
			hs.settle()
			for _, width := range []int{1200, 640, 390} {
				tt.SetSize(width, 760)
				hs.settle()
				badge, ok := tt.Find("NORMAL")
				if !ok || badge.X+badge.W > float32(width)/2 || badge.Y < 760-statusBarH {
					t.Fatalf("width %d: mode is not in the left status group: %v", width, badge)
				}
				pos, ok := tt.Find(a.withKey(locales.GoToLineLabel(), "go.line"))
				if !ok || pos.X < badge.X+badge.W || pos.X+pos.W > float32(width) {
					t.Fatalf("width %d: file position overlaps the mode or window edge: %v / %v", width, badge, pos)
				}
			}
			path := hs.path("art.png")
			writePNG(t, path, 40, 30)
			a.openFile(path, true)
			hs.until("the picture", func() bool { return a.activeViewer() != nil })
			hs.settle()
			if tt.HasText("NORMAL") {
				t.Fatal("a picture shows an editor mode")
			}
			a.openFile(hs.path("README.md"), true)
			hs.settle()
			a.setPreview(a.activeEditor(), true)
			hs.settle()
			if a.activeViewer() == nil || tt.HasText("NORMAL") {
				t.Fatal("a reading preview shows an editor mode")
			}
		})
	}
}

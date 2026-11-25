package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestTitleBarProjects(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	hs.settle()
	project := locales.ProjectPickerTitle()
	if r, ok := tt.Find(project); !ok || r.Y < 0 || r.Y+r.H > titleBarH {
		t.Fatalf("project entry is not in the title bar: %v %v", r, ok)
	}
	if _, ok := tt.Find(revealFolderLabel(shortenHome(hs.root))); ok {
		t.Fatal("the old status bar folder entry is still present")
	}
	if a.repository().worktreeLabel() != "" {
		t.Fatal("a folder without Git is presented as a worktree")
	}
	if err := tt.Click(project); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !a.recents.open || !tt.HasText(locales.ProjectOpenLocal()) || !tt.HasText(locales.ProjectOpenRemote()) {
		t.Fatal("the project entry did not open the project picker with open-folder actions")
	}
	hs.key(0, ui.KeyEscape)
	run := hs.gitRepo()
	run("branch", "-m", "master")
	hs.settleGit()
	if a.repository().worktreeLabel() != "main" || a.repository().branch() != "master" {
		t.Fatalf("main worktree and actual branch were conflated: %s / %s", a.repository().worktreeLabel(), a.repository().branch())
	}
	labels := []string{project, locales.WorktreeStatusTip("main"), locales.BranchLabel("master")}
	bounds := hs.layoutOf(labels...)
	for _, label := range labels {
		r := bounds[label]
		if r.Y < 0 || r.Y+r.H > titleBarH {
			t.Errorf("%q is not in the title bar: %v", label, r)
		}
	}
	for i := 1; i < len(labels); i++ {
		if bounds[labels[i-1]].X+bounds[labels[i-1]].W > bounds[labels[i]].X {
			t.Fatalf("project/worktree/branch order overlaps: %v", bounds)
		}
	}
	tt.Move(bounds[project].X+3, bounds[project].Y+3)
	hs.settle()
	sameLayout(t, "hovered project", bounds, hs.layoutOf(labels...))
	hs.shot("titlebar-projects")
}

func TestTitleBarProjectsFit(t *testing.T) {
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
			run := hs.gitRepo()
			branch := "feature/" + strings.Repeat("long-project-navigation-", 4)
			run("branch", "-m", branch)
			hs.settleGit()
			hs.tt.SetTitleBar(platform.bar)
			for _, width := range []int{640, 1200} {
				hs.tt.SetSize(width, 760)
				hs.settle()
				labels := []string{locales.ProjectPickerTitle(), locales.WorktreeStatusTip("main"), locales.BranchLabel(branch), locales.PrefsTitle()}
				if platform.os != "darwin" {
					labels = append(labels, locales.Menu())
				}
				bounds := hs.layoutOf(labels...)
				for i, label := range labels {
					r := bounds[label]
					if r.W < 20 || r.X < platform.bar.Left || r.X+r.W > float32(width)-platform.bar.Right || r.Y < 0 || r.Y+r.H > titleBarH {
						t.Errorf("width %d: %q overlaps window controls: %v", width, label, r)
					}
					for _, other := range labels[i+1:] {
						if overlaps(r, bounds[other]) {
							t.Errorf("width %d: %q overlaps %q: %v / %v", width, label, other, r, bounds[other])
						}
					}
				}
				hs.shot(fmt.Sprintf("titlebar-projects-%s-%d", platform.os, width))
			}
		})
	}
}

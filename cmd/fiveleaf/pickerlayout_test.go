package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func TestWindowPickersShareCommandPrompt(t *testing.T) {
	hs, _, _ := worktreeFixture(t)
	a := hs.a
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	gitPickerOnly(hs, func(c *ui.Context) {
		a.quickOpenView(c)
		a.branchPickerView(c)
		a.worktreePickerView(c)
		a.recentPickerView(c)
		a.repositoryPickerView(c)
		a.themePickerView(c)
		a.iconThemePickerView(c)
		a.taskPickerView(c)
		a.pluginPickerView(c)
		a.remotePickerView(c)
		a.stashPickerView(c)
		a.prPickerView(c)
		a.prChecksView(c)
		a.promptView(c)
	})
	tt := hs.tt
	for _, width := range []int{1200, 390} {
		tt.SetSize(width, 480)
		a.openQuick(">")
		hs.settle()
		reference, found := tt.Find(locales.PaletteLabel())
		caret, editing := tt.TextCaret()
		if !found || !editing {
			t.Fatal("command palette did not focus its prompt")
		}
		hs.key(0, ui.KeyEscape)
		for _, entry := range []struct {
			label string
			open  func()
		}{
			{locales.BranchSwitchLabel(), a.repository().openBranchPicker},
			{locales.WorktreeSwitchLabel(), a.repository().openWorktreePicker},
			{locales.ProjectPickerTitle(), a.openProjectPicker},
			{locales.RecentLabel(), a.openRecentPicker},
			{locales.RepositorySelect(), a.openRepositoryPicker},
			{locales.ThemeLabel(), a.openThemePicker},
			{locales.PrefTitleIconTheme(), a.openIconThemePicker},
			{locales.TaskPickerTitle(), a.openTaskPicker},
			{"Choose a plugin item", func() { a.openPluginPicker(nil, "Choose a plugin item", nil) }},
			{locales.PickConnectTitle(), a.openRemotePicker},
			{locales.CmdGitStashes(), func() { a.stashes = stashPicker{open: true, repoRoot: a.git.repo.Root} }},
			{locales.CmdGitOpenPullRequest(), func() { a.prs = prPicker{open: true, repoRoot: a.git.repo.Root} }},
			{locales.ChecksTitle("#1"), func() { a.checks = checksPicker{open: true, repoRoot: a.git.repo.Root, number: 1} }},
			{locales.CreateBranchEllipsis(), func() { a.repository().askNewBranch() }},
		} {
			entry.open()
			hs.settle()
			panel, ok := tt.Find(entry.label)
			position, editing := tt.TextCaret()
			if !ok || panel.X != reference.X || panel.Y != reference.Y || panel.W != reference.W || panel.Y+panel.H > 480 || !editing || position.Y != caret.Y {
				t.Errorf("%s at width %d: panel %+v, caret %+v (active %v), want command bounds %+v and caret Y %v", entry.label, width, panel, position, editing, reference, caret.Y)
			}
			hs.key(0, ui.KeyEscape)
		}
	}
}

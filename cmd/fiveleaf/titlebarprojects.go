package main

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

const (
	titleBarProjectMinWidth   float32 = 64
	titleBarControlMinWidth   float32 = 36
	titleBarControlHeight     float32 = 22
	titleBarProjectMaxWidth   float32 = 160
	titleBarProjectNameBold   float32 = 0.5
	titleBarButtonIconSize    float32 = 12
	titleBarButtonTextPadding float32 = 5
)

func (a *app) titleBarProjectWidth() float32 {
	width := titleBarControlMinWidth
	if a.repository().git.repo != nil {
		width += 2 * titleBarControlMinWidth
	}
	if len(a.repositories.contexts) > 1 {
		width += titleBarControlHeight + 2
	}
	if hostOS != "darwin" {
		width += titleBarControlHeight + 2
	}
	return width
}

func (a *app) titleBarProjects(c *ui.Context) {
	p := a.pal
	m := p.metricsOr()
	repo := a.repository()
	name, tip := locales.ProjectPickerTitle(), locales.ProjectPickerTitle()
	if a.root != "" {
		name = filepath.Base(hostPath(a.root))
		tip += " · " + shortenHome(a.root)
	}
	button := func(key, name, label, tip string, icon *ui.SVG, disabled bool, width float32) ui.Element {
		b := ui.ButtonBase(c.Key(key)).Height(titleBarControlHeight).MinWidth(width).MaxWidth(titleBarProjectMaxWidth).Shrink(0).
			Padding(0, titleBarButtonTextPadding).Gap(4).Radius(m.radius).AlignItems(ui.Center).
			Label(label).Tooltip(tip).Disabled(disabled)
		if b.Hovered() || b.Pressed() {
			b.Background(p.hover)
		}
		b.Transition(hoverFade)
		b.Children(func() {
			if icon != nil {
				ui.Icon(c, icon).FontSize(titleBarButtonIconSize).Shrink(0).TextColor(p.muted)
			}
			text := ui.Text(c, name).FontSize(m.textBody).SingleLine().Shrink(1).MinWidth(0).TextColor(p.text)
			if icon == nil {
				text.FontSize(m.textBody + titleBarProjectNameBold).FontWeight(m.weightMedium)
			}
		})
		return b
	}
	if button("title:project", name, locales.ProjectPickerTitle(), tip, nil, false, titleBarProjectMinWidth).Clicked() {
		a.post(a.openProjectPicker)
	}
	if len(a.repositories.contexts) > 1 {
		label := locales.RepositorySelect() + " " + repo.label()
		if a.iconButton(c.Key("title:repository"), iconFolderGit, label, false).Clicked() {
			a.post(a.openRepositoryPicker)
		}
	}
	if repo.git.repo == nil {
		return
	}
	worktree := repo.worktreeLabel()
	worktreeTip := locales.WorktreeStatusTip(worktree)
	if button("title:worktree", worktree, worktreeTip, worktreeTip, iconFolderGit, !repo.git.canMutate(), titleBarControlMinWidth).Clicked() {
		a.post(repo.openWorktreePicker)
	}
	if branch := repo.branch(); branch != "" {
		ui.Text(c, "/").FontSize(m.textBody).TextColor(p.faint).Shrink(0)
		if button("title:branch", branch, locales.BranchLabel(branch), locales.BranchStatusTip(branch), iconBranch, !repo.git.canMutate(), titleBarControlMinWidth).Clicked() {
			a.post(repo.openBranchPicker)
		}
	}
}

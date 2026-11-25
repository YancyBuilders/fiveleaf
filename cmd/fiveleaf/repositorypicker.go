package main

import (
	"path/filepath"
	"strings"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type scmSection int

const (
	scmFull scmSection = iota
	scmChanges
	scmRepository
)

type repositoryPicker struct {
	open  bool
	query string
	sel   int
	moved bool
}

func (a *app) selectRepository(root string) {
	if root != "" && a.repositoryForRoot(root).git.repo == nil {
		return
	}
	a.repositories.selected = root
	a.repositories.picker.open = false
	a.invalidate()
}

func (a *app) openRepositoryPicker() {
	a.repositories.picker = repositoryPicker{open: true}
}

func (a *app) repositoryLabel() string {
	label := a.repository().label()
	if a.repositories.selected == "" {
		label += " · " + locales.RepositoryAutomatic()
	}
	return label
}

// The picker controls history and window Git commands. Commit boxes in the
// changes sections always retain the repository named by their own header.
func (a *app) repositoryButton(c *ui.Context, status bool) bool {
	ctx := a.repository()
	if ctx.git.repo == nil || len(a.repositories.contexts) < 2 {
		return false
	}
	label := a.repositoryLabel()
	tip := locales.RepositorySelect()
	b := ui.ButtonBase(c.Key("repository-picker")).Height(20).Padding(0, 5).Gap(4).Radius(4).AlignItems(ui.Center).Label(tip).Tooltip(tip)
	fg := a.pal.muted
	if status {
		b.Label(tip + " " + ctx.label())
		fg = a.pal.statusFg
	}
	if b.Hovered() {
		b.Background(a.pal.hover)
	}
	b.Transition(hoverFade)
	b.Children(func() {
		ui.Icon(c, iconFolder).FontSize(12).TextColor(fg)
		ui.Text(c, label).FontSize(11).TextColor(fg).SingleLine().Shrink(1)
	})
	if b.Clicked() {
		a.post(a.openRepositoryPicker)
	}
	return b.Clicked()
}

func (a *app) repositoryChoices() []string {
	// Either slash finds a repository, whichever its root is spelled with.
	query := filepath.ToSlash(strings.ToLower(strings.TrimSpace(a.repositories.picker.query)))
	var roots []string
	if query == "" || strings.Contains(strings.ToLower(locales.RepositoryAutomatic()), query) {
		roots = append(roots, "")
	}
	for _, ctx := range a.rootRepositoryList() {
		if ctx.git.repo != nil && (query == "" || strings.Contains(filepath.ToSlash(strings.ToLower(ctx.git.repo.Root)), query)) {
			roots = append(roots, ctx.git.repo.Root)
		}
	}
	return roots
}

func (a *app) repositoryPickerView(c *ui.Context) {
	pick := &a.repositories.picker
	if !pick.open {
		return
	}
	roots := a.repositoryChoices()
	pick.sel = min(max(pick.sel, 0), max(len(roots)-1, 0))
	p := a.pal
	prompt := pickerPrompt{label: locales.RepositorySelect(), prompt: locales.RepositorySelect(), value: &pick.query, close: func() { pick.open = false }}
	choose := func() {
		if pick.sel >= 0 && pick.sel < len(roots) {
			root := roots[pick.sel]
			a.post(func() { a.selectRepository(root) })
		}
	}
	a.pickerDialog(c.Key("repository-picker-dialog"), prompt, func(in ui.Element) {
		if in.Changed() {
			pick.sel, pick.moved = 0, true
		}
		pickerNavigate(in, &pick.sel, &pick.moved, len(roots))
		if in.Submitted() {
			choose()
		}
	}, func() {
		if len(roots) == 0 {
			quickMessage(c, iconFolderGit, locales.FindNoResults())
		} else {
			a.pickerScroll(c.Key("repository-results")).Children(func() {
				for i, root := range roots {
					label, detail := locales.RepositoryAutomatic(), locales.RepositoryFollowEditor()
					if root != "" {
						label = filepath.Base(root)
						detail = root
					}
					b := ui.ButtonBase(c.Key("repository:"+root)).FillWidth().Padding(7, 10).Radius(5).Label(label + " " + detail)
					a.pickerRow(c, b, i == pick.sel, pick.moved)
					b.Children(func() {
						ui.Column(c).FillWidth().Gap(3).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								icon, tint := iconFolder, p.muted
								if root != "" {
									icon, _ = a.folderIcon(root, false)
								}
								iconThemeView(c, p, icon, tint, 13)
								ui.Text(c, label).FontSize(13).TextColor(p.text).SingleLine()
								if a.repositories.selected == root {
									ui.Icon(c, iconCheck).FontSize(12).TextColor(p.accent)
								}
							})
							ui.Text(c, detail).FontSize(11).TextColor(p.muted).SingleLine().Shrink(1)
						})
					})
					if b.Clicked() {
						a.post(func() { a.selectRepository(root) })
					}
				}
			})
		}
		pick.moved = false
		a.pickerActions(c, pickerBar{action: locales.QuickChooseAction(), enabled: len(roots) > 0, choose: choose, close: prompt.close})
	})
}

// changesContext keeps file selection separate from the history/panel state.
// The canonical scmState is always the one used by panel commands and reads.
func (a *gitContext) changesContext() *gitContext {
	if len(a.app.repositories.contexts) < 2 {
		return a
	}
	if a.changesUI == nil {
		a.changesUI = &scmState{section: scmChanges}
	}
	view := *a
	view.scm = a.changesUI
	return &view
}

func (a *gitContext) hasCommitControls() bool {
	return a.changeCount() > 0 || a.git.amend || a.git.status != nil && a.git.status.Op != ""
}

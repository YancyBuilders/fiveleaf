package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/git"
	"fiveleaf/internal/vfs"
	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

// worktreePicker is the box that switches worktrees: the checkouts of the
// repository, the one open first, found by folder and branch, and
// the name typed, to create a worktree on a new branch of.
type worktreePicker struct {
	repoRoot string
	open     bool
	query    string
	sel      int
	moved    bool
	loading  bool
	// head is the commit the work tree is at when the box opened, which a
	// worktree created starts from; "" before the first commit.
	head string
	// from names head for the row that creates a worktree: the branch checked
	// out when the box opened and its short hash, or the hash alone detached.
	from string
	// trees are those read when the box opened, the current one first;
	// names are what the query matches, the folder and branch of each.
	trees   []git.Worktree
	names   []string
	results []workspace.Match
	shown   string // the query the results are of
	stale   bool
}

// worktreeName is the name a worktree shows by: the folder it is in.
func worktreeName(t git.Worktree) string { return filepath.Base(t.Path) }

// worktreeLabel distinguishes the main checkout from its branch without
// reading Git while the title bar is drawn.
func (a *gitContext) worktreeLabel() string {
	if a.git.repo == nil {
		return ""
	}
	if a.git.repo.Dir == a.git.repo.Common {
		return "main"
	}
	return filepath.Base(hostPath(a.git.repo.Root))
}

// openWorktreePicker shows the box of the worktrees, reading them and the
// commit HEAD is at in the background.
func (a *gitContext) openWorktreePicker() {
	if !a.live() || !a.git.canMutate() {
		return
	}
	r := a.git.repo
	if r == nil {
		return
	}
	w := &a.app.worktrees
	*w = worktreePicker{open: true, loading: true, repoRoot: r.Root}
	branch := ""
	if st := a.git.status; st != nil {
		branch = st.Branch
	}
	bg.Go(func() {
		trees, err := r.Worktrees()
		head := r.Head()
		a.app.post(func() {
			if !w.open || w.repoRoot != r.Root || a.git.repo != r {
				return
			}
			if err != nil {
				a.closeWorktreePicker()
				a.gitFailed(err)
				return
			}
			w.loading = false
			w.head = head
			w.from = git.ShortHash(head)
			if branch != "" {
				w.from = branch + " (" + w.from + ")"
			}
			// A bare repository is not a checkout to switch to.
			w.trees = slices.DeleteFunc(trees, func(t git.Worktree) bool { return t.Bare })
			slices.SortStableFunc(w.trees, func(x, y git.Worktree) int { return btoi(!x.Current) - btoi(!y.Current) })
			w.names = make([]string, len(w.trees))
			for i, t := range w.trees {
				w.names[i] = strings.TrimSpace(worktreeName(t) + " " + t.Branch)
			}
			w.stale = true
		})
	})
}

// closeWorktreePicker shuts the box and lets go of the worktrees it read.
func (a *gitContext) closeWorktreePicker() {
	a.app.worktrees = worktreePicker{}
	if a.app.focus != paneTerminal {
		a.app.focusColumn()
	}
}

// worktreeChoice is a row of the box: a worktree, a name to create, or a
// prompt for a name.
type worktreeChoice struct {
	prompt bool   // ask for the name of a new worktree
	create string // the name of the branch to create a worktree on, "" for a worktree
	at     int    // the result shown, for a worktree
}

// worktreeChoices returns matching worktrees with a creation row when the
// query is empty or names an available branch and folder.
func (a *gitContext) worktreeChoices() []worktreeChoice {
	w := &a.app.worktrees
	var out []worktreeChoice
	if name := strings.TrimSpace(w.query); !w.loading && w.head != "" {
		if name == "" {
			out = append(out, worktreeChoice{prompt: true})
		} else if validBranch(name) && !a.worktreeTaken(name) {
			out = append(out, worktreeChoice{create: name})
		}
	}
	for i := range w.results {
		out = append(out, worktreeChoice{at: i})
	}
	return out
}

// worktreeTaken reports whether a branch or a folder of worktrees has the
// name, as a worktree made of it would need both new.
func (a *gitContext) worktreeTaken(name string) bool {
	return worktreeNameTaken(name, a.git.branches, a.app.worktrees.trees)
}

func worktreeNameTaken(name string, branches []string, trees []git.Worktree) bool {
	folder := strings.ReplaceAll(name, "/", "-")
	if slices.Contains(branches, name) {
		return true
	}
	for _, t := range trees {
		if t.Branch == name || worktreeName(t) == folder {
			return true
		}
	}
	return false
}

// chooseWorktree does what a row of the box does: switches to its
// worktree, or creates the worktree typed from the commit HEAD is at.
func (a *gitContext) chooseWorktree(ch worktreeChoice) {
	w := &a.app.worktrees
	if !a.live() || !a.git.canMutate() || !w.open || w.repoRoot != a.git.repo.Root {
		return
	}
	if ch.prompt {
		base, from, trees := w.head, w.from, w.trees
		a.closeWorktreePicker()
		a.askNewWorktree(base, from, trees)
		return
	}
	if ch.create != "" {
		base := w.head
		a.closeWorktreePicker()
		a.createWorktree(ch.create, base)
		return
	}
	if ch.at < 0 || ch.at >= len(w.results) {
		return
	}
	t := w.trees[w.results[ch.at].Index]
	a.closeWorktreePicker()
	if !t.Current {
		a.openWorktree(t.Path)
	}
}

func (a *gitContext) askNewWorktree(base, from string, trees []git.Worktree) {
	if !a.live() || !a.git.canMutate() || base == "" {
		return
	}
	defer a.bindPanelDialogs(a.git.repo)
	a.app.prompt = &prompt{title: locales.WorktreeCreateEllipsis(), hint: locales.WorktreeFrom(from), done: func(name string) string {
		if busy := a.git.label(); busy != "" {
			return busy
		}
		if st := a.git.status; st != nil && st.Op != "" {
			return locales.ScmOpInProgress(opTitle(st.Op))
		}
		name = strings.TrimSpace(name)
		if !validBranch(name) {
			return locales.BranchInvalidName()
		}
		if worktreeNameTaken(name, a.git.branches, trees) {
			return locales.WorktreeNameTaken(name)
		}
		a.createWorktree(name, base)
		return ""
	}}
}

// openWorktree switches the window to a worktree's folder, once it is
// found to be a folder. A folder that is gone is reported, not pruned:
// the branch stays checked out where it is.
func (a *gitContext) openWorktree(path string) {
	if !a.live() {
		return
	}
	if !a.app.online() {
		a.app.fail("%s", locales.NotConnectedTo(a.app.conn.Host))
		return
	}
	r := a.git.repo
	bg.Go(func() {
		var found error
		info, err := vfs.Stat(path)
		switch {
		case err != nil:
			found = err
		case !info.IsDir():
			found = errors.New(locales.ErrWorktreeNotFolder(path))
		}
		a.app.post(func() {
			// The window moved to another repository while the folder was read.
			if a.git.repo != r {
				return
			}
			if errors.Is(found, fs.ErrNotExist) {
				a.app.fail("%s", locales.WorktreeMissing(shortenHome(path)))
				return
			}
			if found != nil {
				a.gitFailed(found)
				return
			}
			a.app.switchFolder(path)
		})
	})
}

// createWorktree makes a worktree on a new branch from base, beside the
// main one, and switches to it. Once made, it stays made whatever happens
// to opening it.
func (a *gitContext) createWorktree(name, base string) {
	var path string
	var created error
	a.gitDo(locales.BusyCreatingWorktree(name), func(r *git.Repo) error {
		path, created = r.AddWorktree(name, base)
		return nil
	}, func() {
		var exists *git.WorktreeExists
		switch {
		case errors.As(created, &exists):
			a.app.fail("%s", locales.WorktreeExists(shortenHome(exists.Path)))
			return
		case created != nil:
			a.gitFailed(created)
			return
		}
		a.app.tell(note{message: locales.WorktreeCreated(shortenHome(path))})
		a.app.switchFolder(path)
	})
}

// worktreePickerView shows the box of the worktrees while it is open.
func (a *gitContext) worktreePickerView(c *ui.Context) {
	w := &a.app.worktrees
	if !w.open {
		return
	}
	p := a.app.pal
	changed := w.stale || w.shown != w.query
	if changed {
		w.results = workspace.Fuzzy(w.names, strings.TrimSpace(w.query), len(w.names))
		w.shown, w.stale, w.sel = w.query, false, 0
	}
	choices := a.worktreeChoices()
	if changed && len(w.results) > 0 && len(choices) > len(w.results) &&
		strings.Contains(strings.ToLower(w.names[w.results[0].Index]), strings.ToLower(strings.TrimSpace(w.query))) {
		// Enter switches to the first match when its folder or branch has the
		// query as typed; a new name that only matches scattered letters, as
		// api does feature-login-page, creates.
		w.sel = 1
	}
	prompt := pickerPrompt{label: locales.WorktreeSwitchLabel(), prompt: locales.WorktreePlaceholder(), value: &w.query,
		disabled: !a.git.canMutate(), close: a.closeWorktreePicker}
	choose := func() {
		if w.sel >= 0 && w.sel < len(choices) {
			choice := choices[w.sel]
			a.app.post(func() { a.chooseWorktree(choice) })
		}
	}
	a.app.pickerDialog(c.Key("worktree-picker"), prompt, func(in ui.Element) {
		pickerNavigate(in, &w.sel, &w.moved, len(choices))
		if in.Submitted() {
			choose()
		}
	}, func() {
		switch {
		case len(choices) > 0:
			a.app.pickerScroll(c.Key("worktree-results")).Children(func() {
				for i, ch := range choices {
					a.worktreeRow(c, ch, i)
					if i == 0 && (ch.prompt || ch.create != "") && len(choices) > 1 {
						ui.Box(c).Height(1).Background(p.border).Margin(4, 6)
					}
				}
			})
		case w.loading:
			quickMessage(c, iconFolderGit, locales.WorktreeReading())
		default:
			quickMessage(c, iconFolderGit, locales.WorktreeNoMatches())
		}
		// A local branch that no worktree has checked out cannot be made a
		// worktree now, so the box points to checking it out.
		if name := strings.TrimSpace(w.query); !w.loading && validBranch(name) && slices.Contains(a.git.branches, name) &&
			!slices.ContainsFunc(w.trees, func(t git.Worktree) bool { return t.Branch == name }) {
			ui.Text(c, locales.WorktreeBranchExists(name)).FontSize(12).TextColor(p.faint).Padding(6, 16)
		}
		if !w.loading && w.head == "" {
			ui.Text(c, locales.WorktreeNoCommit()).FontSize(12).TextColor(p.faint).Padding(6, 16)
		}
		w.moved = false
		action := locales.QuickSwitchAction()
		if w.sel < len(choices) && (choices[w.sel].prompt || choices[w.sel].create != "") {
			action = locales.QuickCreateAction()
		}
		a.app.pickerActions(c, pickerBar{action: action, enabled: len(choices) > 0 && !prompt.disabled, choose: choose, close: prompt.close})
	})
}

// worktreeRow is a row of the box of the worktrees: a worktree, with the
// characters the query matched in the accent color, a check by the one
// open, its branch, the short hash of its commit and its path; or the
// worktree to create.
func (a *gitContext) worktreeRow(c *ui.Context, ch worktreeChoice, i int) {
	w := &a.app.worktrees
	p := a.app.pal
	if ch.prompt || ch.create != "" {
		label := locales.WorktreeCreateEllipsis()
		if ch.create != "" {
			label = locales.WorktreeCreateLabel(ch.create)
		}
		row := ui.ButtonBase(c.Key("create")).Padding(5, 10).Gap(10).Radius(6).Justify(ui.Start).AlignItems(ui.Center).
			Label(label).Disabled(!a.git.canMutate())
		a.app.pickerRow(c, row, i == w.sel, w.moved)
		if row.Clicked() {
			a.app.post(func() { a.chooseWorktree(ch) })
		}
		row.Children(func() {
			ui.Icon(c, iconPlus).FontSize(14).TextColor(p.accent)
			if ch.prompt {
				ui.Text(c, label).FontSize(13).TextColor(p.text).SingleLine().Shrink(1)
			} else {
				ui.RichText(c, ui.Span{Text: locales.WorktreeCreatePrefix()}, ui.Span{Text: ch.create, Weight: 650}).FontSize(13).
					TextColor(p.text).SingleLine().Shrink(1)
			}
			ui.Spacer(c)
			ui.Text(c, locales.WorktreeFrom(w.from)).FontSize(12).TextColor(p.faint).SingleLine().Shrink(0)
		})
		return
	}
	t := w.trees[w.results[ch.at].Index]
	name := worktreeName(t)
	row := ui.ButtonBase(c.Key("worktree:"+t.Path)).Padding(5, 10).Gap(10).Radius(6).Justify(ui.Start).AlignItems(ui.Center).
		Label(name).Tooltip(shortenHome(t.Path)).Disabled(!a.git.canMutate())
	a.app.pickerRow(c, row, i == w.sel, w.moved)
	if row.Clicked() {
		a.app.post(func() { a.chooseWorktree(ch) })
	}
	row.Children(func() {
		icon, col := iconFolderGit, p.muted
		if t.Current {
			icon, col = iconCheck, p.accent
		}
		ui.Icon(c, icon).FontSize(14).TextColor(col)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.RichText(c, matchSpans(name, w.results[ch.at].Positions, p)...).FontSize(13).SingleLine()
			// The branch, or the hash of a detached commit, which is not
			// shown twice.
			parts := []string{t.Branch, git.ShortHash(t.Head)}
			if t.Detached || t.Branch == "" {
				parts = []string{git.ShortHash(t.Head)}
			}
			parts = append(parts, shortenHome(t.Path))
			if t.Prunable {
				parts = append(parts, locales.WorktreeMissingHint())
			}
			if t.Locked {
				parts = append(parts, locales.WorktreeLockedHint())
			}
			ui.Text(c, strings.Join(parts, " • ")).FontSize(12).TextColor(p.faint).SingleLine()
		})
	})
}

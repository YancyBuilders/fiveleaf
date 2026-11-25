package main

import (
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/git"
	"fiveleaf/locales"
)

const (
	// stashListN is how many stashes are read with the status, which
	// Stashes… lists; stashMenuN how many the menu of the view lists.
	stashListN = 100
	stashMenuN = 10
)

// stashPicker is the box of the stashes, which applies, pops or drops one.
type stashPicker struct {
	repoRoot string
	open     bool
	query    string
	sel      int
	moved    bool
}

// stashLabel names a stash in a menu: its message, and its branch.
func stashLabel(s git.Stash) string {
	if s.Branch == "" {
		return s.Message
	}
	return s.Message + " · " + s.Branch
}

// canStash reports whether there is something to stash: a tracked file
// changed, or any file when untracked.
func (a *gitContext) canStash(untracked bool) bool {
	groups := a.groupEntries()
	n := len(groups[groupStaged]) + len(groups[groupChanges]) + len(groups[groupMerge])
	if untracked {
		n += len(groups[groupUntracked])
	}
	return a.git.canMutate() && n > 0
}

// askStash asks for the message of a stash of the changes, and of the
// untracked files too when untracked, then makes it. Changed files are
// saved before.
func (a *gitContext) askStash(untracked bool) {
	if !a.live() || !a.canStash(untracked) {
		return
	}
	title := locales.CmdGitStash()
	if untracked {
		title = locales.CmdGitStashUntracked()
	}
	// The repository goes in the hint: the button is named by the title.
	hint := locales.StashMessageHint()
	if repo := strings.TrimPrefix(a.repositoryQuestion(""), " · "); repo != "" {
		hint = repo + " · " + hint
	}
	defer a.bindPanelDialogs(a.git.repo)
	a.app.prompt = &prompt{title: title, hint: hint, optional: true, done: func(msg string) string {
		if busy := a.git.label(); busy != "" {
			return busy
		}
		a.stash(msg, untracked)
		return ""
	}}
}

func (a *gitContext) stash(msg string, untracked bool) {
	for _, d := range a.app.docs {
		if d.path != "" && d.dirty() && a.owns(d.path) {
			a.app.saveDoc(d, nil)
		}
	}
	var hash string
	a.gitDo(locales.BusyStashing(), func(r *git.Repo) error {
		var err error
		hash, err = r.Stash(msg, untracked)
		if errors.Is(err, git.ErrNothingToStash) {
			return errors.New(locales.StashNothing())
		}
		return err
	}, func() {
		a.app.tell(note{message: locales.StashMade(), label: locales.StashPop(), action: func() { a.popStash(hash) }})
	})
}

// applyStash applies the stash hash, keeping it.
func (a *gitContext) applyStash(hash string) {
	a.gitDo(locales.BusyApplyingStash(), func(r *git.Repo) error { return r.StashApply(hash) }, nil)
}

// popStash applies the stash hash and drops it, unless its changes
// conflict, when Git keeps it.
func (a *gitContext) popStash(hash string) {
	a.gitDo(locales.BusyApplyingStash(), func(r *git.Repo) error { return r.StashPop(hash) }, nil)
}

// dropStash deletes the stash s, once asked.
func (a *gitContext) dropStash(s git.Stash) {
	if !a.live() {
		return
	}
	defer a.bindPanelDialogs(a.git.repo)
	a.app.present(&confirm{title: a.repositoryQuestion(locales.StashDropQuestion(s.Message)), message: locales.StashDropDetail(),
		ok: locales.StashDrop(), danger: true, run: func() {
			a.gitDo(locales.BusyDroppingStash(), func(r *git.Repo) error { return r.StashDrop(s.Hash) }, nil)
		}})
}

// stashMenu lists the newest stashes, each with what can be done with it,
// and Stashes… for the rest.
func (a *gitContext) stashMenu(m *ui.Menu) {
	stashes := a.git.stashes
	for _, s := range stashes[:min(len(stashes), stashMenuN)] {
		m.Submenu(stashLabel(s), func(m *ui.Menu) {
			busy := !a.git.canMutate()
			if m.Item(locales.StashApply()).Disabled(busy).Chosen() {
				a.app.post(func() { a.applyStash(s.Hash) })
			}
			if m.Item(locales.StashPop()).Disabled(busy).Chosen() {
				a.app.post(func() { a.popStash(s.Hash) })
			}
			m.Separator()
			if m.Item(locales.StashDropEllipsis()).Disabled(busy).Chosen() {
				a.app.post(func() { a.dropStash(s) })
			}
		})
	}
	m.Separator()
	if m.Item(locales.CmdGitStashes()).Chosen() {
		a.app.post(a.openStashPicker)
	}
}

// openStashPicker shows the box of the stashes.
func (a *gitContext) openStashPicker() {
	if !a.live() {
		return
	}
	if len(a.git.stashes) == 0 {
		a.app.fail("%s", locales.StashNone())
		return
	}
	a.app.stashes = stashPicker{open: true, repoRoot: a.git.repo.Root}
}

// stashChoices are the stashes whose message, branch or name has the
// words of the query.
func (a *gitContext) stashChoices() []git.Stash {
	words := strings.Fields(strings.ToLower(a.app.stashes.query))
	var out []git.Stash
	for _, s := range a.git.stashes {
		text := strings.ToLower(s.Message + " " + s.Branch + " " + s.Ref())
		ok := true
		for _, w := range words {
			ok = ok && strings.Contains(text, w)
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

// stashPickerView is the box of the stashes: ↵ applies the one chosen,
// ⌘↵ pops it and ⌘⌫ drops it, as the buttons of its row do.
func (a *app) stashPickerView(c *ui.Context) {
	pick := &a.stashes
	if !pick.open {
		return
	}
	g := a.repositoryForRoot(pick.repoRoot)
	if g.git.repo == nil || g.git.repo.Root != pick.repoRoot {
		pick.open = false
		return
	}
	stashes := g.stashChoices()
	pick.sel = min(max(pick.sel, 0), max(len(stashes)-1, 0))
	p := a.pal
	run := func(s git.Stash, do func(git.Stash)) {
		pick.open = false
		a.post(func() { do(s) })
	}
	apply := func(s git.Stash) { g.applyStash(s.Hash) }
	pop := func(s git.Stash) { g.popStash(s.Hash) }
	prompt := pickerPrompt{label: locales.CmdGitStashes(), prompt: locales.StashFind(), value: &pick.query, close: func() { pick.open = false }}
	choose := func(action func(git.Stash)) {
		if pick.sel >= 0 && pick.sel < len(stashes) && g.git.canMutate() {
			run(stashes[pick.sel], action)
		}
	}
	a.pickerDialog(c.Key("stash-picker"), prompt, func(in ui.Element) {
		if in.Changed() {
			pick.sel, pick.moved = 0, true
		}
		pickerNavigate(in, &pick.sel, &pick.moved, len(stashes))
		switch {
		case in.Shortcut(ui.Cmd, ui.KeyEnter):
			choose(pop)
		case in.Shortcut(ui.Cmd, ui.KeyBackspace):
			choose(g.dropStash)
		case in.Submitted():
			choose(apply)
		}
	}, func() {
		if len(stashes) == 0 {
			quickMessage(c, iconBox, locales.StashNoMatch())
		} else {
			a.pickerScroll(c.Key("stash-results")).Children(func() {
				for i, s := range stashes {
					row := ui.ButtonBase(c.Key("stash:"+s.Hash)).FillWidth().Padding(5, 10).Gap(10).Radius(6).AlignItems(ui.Center).Label(s.Message)
					a.pickerRow(c, row, i == pick.sel, pick.moved)
					row.Children(func() {
						ui.Icon(c, iconBox).FontSize(14).TextColor(p.muted)
						ui.Text(c, s.Message).FontSize(13).TextColor(p.text).SingleLine().Shrink(1).MinWidth(0)
						ui.Text(c, s.Branch).FontSize(12).TextColor(p.faint).SingleLine().Shrink(1).Grow(1).MinWidth(0)
						if !s.When.IsZero() {
							ui.Text(c, shortAgo(s.When, time.Now())).FontSize(11.5).TextColor(p.faint).SingleLine().Shrink(0)
						}
						if !g.git.canMutate() || i != pick.sel && !row.Hovered() {
							return
						}
						if a.smallButton(c, iconCheck, locales.StashApply()) {
							run(s, apply)
						}
						if a.smallButton(c, iconArrowUp, locales.StashPop()) {
							run(s, pop)
						}
						if a.smallButton(c, iconTrash, locales.StashDropEllipsis()) {
							run(s, g.dropStash)
						}
					})
					if row.Clicked() && g.git.canMutate() {
						pick.sel = i
					}
				}
			})
		}
		pick.moved = false
		a.pickerActions(c, pickerBar{action: locales.StashApply(), enabled: len(stashes) > 0 && g.git.canMutate(), choose: func() { choose(apply) }, close: prompt.close,
			hints: []pickerHint{{keyText("CmdOrCtrl+Enter"), locales.StashPop()}, {keyText("CmdOrCtrl+Backspace"), locales.StashDrop()}}})
	})
}

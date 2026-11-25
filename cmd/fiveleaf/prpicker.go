package main

import (
	"errors"
	"strconv"
	"strings"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/git"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

// prPicker is the box of the pull requests open, whose changes ↵ shows; a
// number typed opens that one, whatever its state.
type prPicker struct {
	repoRoot string
	open     bool
	query    string
	sel      int
	moved    bool
	// prs are those open as gh listed them, loading set while it runs,
	// and err why it failed.
	prs     []git.PRListed
	loading bool
	err     string
}

// prChoice is a row of the box: a pull request listed, or the one of the
// number typed.
type prChoice struct {
	number  int
	title   string
	detail  string
	current bool
	typed   bool // the number typed, which the title names
}

// openPRPicker shows the box of the pull requests, reading those open.
func (a *gitContext) openPRPicker() {
	if !a.live() {
		return
	}
	if !a.app.online() {
		a.app.fail("%s", locales.NotConnectedTo(a.app.conn.Host))
		return
	}
	pick := &a.app.prs
	*pick = prPicker{open: true, repoRoot: a.git.repo.Root, loading: true}
	r, g := a.git.repo, a.git
	bg.Go(func() {
		defer recoverCrash()
		prs, err := r.PullRequests()
		a.app.post(func() {
			if pick.repoRoot != r.Root {
				return
			}
			pick.loading = false
			switch {
			case err == nil:
				pick.prs, g.ghOK = prs, true
			case errors.Is(err, git.ErrNoGH):
				g.ghOK = false
				pick.err = locales.PrNoGitHub()
			default:
				pick.err = err.Error()
			}
		})
	})
}

// prChoices are the rows of the box: the pull request of the number
// typed, then the branch's, then those open whose number, title, author
// or branch has the words of the query.
func (a *gitContext) prChoices() []prChoice {
	pick := &a.app.prs
	query := strings.TrimSpace(pick.query)
	var out []prChoice
	seen := map[int]bool{}
	if n, err := strconv.Atoi(strings.TrimPrefix(query, "#")); err == nil && n > 0 {
		out = append(out, prChoice{number: n, title: locales.PrPickerNumber("#" + strconv.Itoa(n)), typed: true})
		seen[n] = true
	}
	words := strings.Fields(strings.ToLower(query))
	match := func(s string) bool {
		s = strings.ToLower(s)
		for _, w := range words {
			if !strings.Contains(s, strings.TrimPrefix(w, "#")) {
				return false
			}
		}
		return true
	}
	if pr := a.currentPR(); pr != nil && !seen[pr.Number] {
		title := ""
		for _, l := range pick.prs {
			if l.Number == pr.Number {
				title = l.Title
			}
		}
		if match("#" + strconv.Itoa(pr.Number) + " " + title + " " + a.branch()) {
			out = append(out, prChoice{number: pr.Number, title: title, detail: a.branch(), current: true})
			seen[pr.Number] = true
		}
	}
	for _, l := range pick.prs {
		if seen[l.Number] || !match("#"+strconv.Itoa(l.Number)+" "+l.Title+" "+l.Author+" "+l.HeadRef) {
			continue
		}
		detail := l.HeadRef
		if l.Author != "" {
			detail = l.Author + " · " + detail
		}
		if l.Draft {
			detail = locales.PrDraft() + " · " + detail
		}
		out = append(out, prChoice{number: l.Number, title: l.Title, detail: detail})
	}
	return out
}

// prPickerView is the box of the pull requests: ↵ opens the changes of the
// one chosen in a tab.
func (a *app) prPickerView(c *ui.Context) {
	pick := &a.prs
	if !pick.open {
		return
	}
	g := a.repositoryForRoot(pick.repoRoot)
	if g.git.repo == nil || g.git.repo.Root != pick.repoRoot {
		pick.open = false
		return
	}
	choices := g.prChoices()
	pick.sel = min(max(pick.sel, 0), max(len(choices)-1, 0))
	p := a.pal
	open := func(n int) {
		pick.open = false
		a.post(func() { g.openPullRequest(n) })
	}
	prompt := pickerPrompt{label: locales.CmdGitOpenPullRequest(), prompt: locales.PrPickerFind(), value: &pick.query, close: func() { pick.open = false }}
	choose := func() {
		if pick.sel >= 0 && pick.sel < len(choices) {
			open(choices[pick.sel].number)
		}
	}
	a.pickerDialog(c.Key("pr-picker"), prompt, func(in ui.Element) {
		if in.Changed() {
			pick.sel, pick.moved = 0, true
		}
		pickerNavigate(in, &pick.sel, &pick.moved, len(choices))
		if in.Submitted() {
			choose()
		}
	}, func() {
		if len(choices) == 0 {
			message := locales.PrPickerNone()
			if pick.loading {
				message = locales.PrPickerLoading()
			} else if pick.err != "" {
				message = pick.err
			}
			quickMessage(c, iconPullRequest, message)
		} else {
			a.pickerScroll(c.Key("pr-results")).Children(func() {
				for i, ch := range choices {
					number := "#" + strconv.Itoa(ch.number)
					row := ui.ButtonBase(c.Key("pr:"+number)).FillWidth().Padding(5, 10).Gap(10).Radius(6).AlignItems(ui.Center).Label(number + " " + ch.title)
					a.pickerRow(c, row, i == pick.sel, pick.moved)
					row.Children(func() {
						ui.Icon(c, iconPullRequest).FontSize(14).TextColor(p.muted)
						if !ch.typed {
							ui.Text(c, number).FontSize(13).TextColor(p.muted).SingleLine().Shrink(0)
						}
						ui.Text(c, ch.title).FontSize(13).TextColor(p.text).SingleLine().Shrink(1).MinWidth(0)
						ui.Text(c, ch.detail).FontSize(12).TextColor(p.faint).SingleLine().Shrink(1).Grow(1).MinWidth(0)
						if ch.current {
							ui.Text(c, locales.PrPickerCurrent()).FontSize(11).TextColor(p.accent).SingleLine().Shrink(0)
						}
					})
					if row.Clicked() {
						open(ch.number)
					}
				}
			})
		}
		pick.moved = false
		a.pickerActions(c, pickerBar{action: locales.QuickOpenAction(), enabled: len(choices) > 0, choose: choose, close: prompt.close})
	})
}

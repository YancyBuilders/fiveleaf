package main

import (
	"time"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/git"
)

type chatChangeRead struct {
	status         *git.Status
	read           time.Time
	added, removed int
	err            error
	done           bool
}

// chatChangeTotals is polled only by the visible panel. Equal porcelain
// statuses reuse their pointer, but each SCM read can see different contents.
func (ctx *gitContext) chatChangeTotals() (added, removed int, ready bool, err error) {
	state := ctx.git
	if !ctx.live() {
		return 0, 0, false, nil
	}
	if state.statusErr != nil || state.status == nil {
		return 0, 0, false, state.statusErr
	}
	if len(state.status.Entries) == 0 {
		return 0, 0, true, nil
	}
	if cached := state.chatTotals; cached != nil {
		if !cached.done {
			return 0, 0, false, nil
		}
		if cached.status == state.status && cached.read == state.read {
			return cached.added, cached.removed, cached.err == nil, cached.err
		}
	}
	repository := state.repo
	job := &chatChangeRead{status: state.status, read: state.read}
	state.chatTotals = job
	bg.Go(func() {
		defer recoverCrash()
		var plus, minus int
		var countErr error
		readGit(func() { plus, minus, countErr = repository.ChangeTotals(job.status) })
		ctx.app.post(func() {
			if ctx.live() && state.chatTotals == job {
				job.added, job.removed, job.err, job.done = plus, minus, countErr, true
			}
		})
	})
	return 0, 0, false, nil
}

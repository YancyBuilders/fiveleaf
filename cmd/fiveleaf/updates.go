package main

import (
	"cmp"
	"context"
	"errors"
	"log"
	"time"

	"github.com/egoist/mygo"

	"fiveleaf/locales"
)

const (
	updateInterval      = time.Hour
	updateFirstWait     = 10 * time.Second
	updateTimeout       = time.Minute
	updatePolicyTimeout = 5 * time.Second
	updateInstallLimit  = 15 * time.Minute
)

type updatePhase uint8

const (
	updateDisabled updatePhase = iota
	updateIdle
	updateChecking
	updateCurrent
	updateAvailable
	updateDownloading
	updateInstalled
	updateCheckFailed
	updateInstallFailed
)

type updateStatus struct {
	phase                   updatePhase
	version, notes, minimum string
	progress                int
	moreAfterRestart        bool
}

type appUpdater struct {
	ctx     context.Context
	status  updateStatus
	release *mygo.Update
	minimum string
}

func (m *fiveleaf) startUpdates() {
	if !mygo.Updater.Enabled() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.updates.ctx = ctx
	m.setUpdateStatus(updateStatus{phase: updateIdle})
	wake := make(chan struct{}, 1)
	off := mygo.Power.OnResume(func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	})
	mygo.App.OnQuit(func() {
		off()
		cancel()
	})
	go func() {
		defer recoverCrash()
		timer := time.NewTimer(updateFirstWait)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				mygo.RunOnMain(m.checkUpdate)
				timer.Reset(updateInterval)
			case <-wake:
				timer.Reset(updateFirstWait)
			}
		}
	}()
}

func (m *fiveleaf) checkUpdate() {
	previous := m.updates.status
	switch previous.phase {
	case updateDisabled, updateChecking, updateDownloading:
		return
	case updateInstalled:
		if updatePolicyURL == "" {
			return
		}
	}
	if m.life == lifeClosing {
		return
	}
	checking := previous
	checking.phase = updateChecking
	m.setUpdateStatus(checking)
	ctx, running := m.updates.ctx, mygo.App.Version()
	minimum := m.updates.minimum
	go func() {
		defer recoverCrash()
		checkCtx, cancel := context.WithTimeout(ctx, updateTimeout)
		release, err := mygo.Updater.Check(checkCtx)
		cancel()
		if release != nil || previous.minimum != "" || previous.phase == updateInstalled {
			policyCtx, cancel := context.WithTimeout(ctx, updatePolicyTimeout)
			fetched, policyErr := fetchUpdateMinimum(policyCtx, updatePolicyClient, updatePolicyURL)
			cancel()
			if policyErr != nil {
				log.Printf("update policy: %v", policyErr)
			} else {
				minimum = fetched
			}
		}
		if ctx.Err() == nil {
			mygo.RunOnMain(func() { m.finishUpdateCheck(previous, running, release, minimum, err) })
		}
	}()
}

func (m *fiveleaf) finishUpdateCheck(previous updateStatus, running string, release *mygo.Update, minimum string, err error) {
	disabled := errors.Is(err, mygo.ErrUpdatesDisabled)
	if disabled && previous.phase != updateInstalled {
		m.updates.release = nil
		m.setUpdateStatus(updateStatus{phase: updateDisabled})
		return
	}
	m.updates.minimum = minimum
	if err != nil && !disabled {
		log.Printf("checking for updates: %v", err)
		release = m.updates.release
	}
	required := ""
	if release != nil && (err == nil || previous.minimum != "") {
		required = requiredUpdateMinimum(running, release.Version, minimum)
	}
	if previous.phase == updateInstalled {
		m.updates.release = release
		stagedMinimum := ""
		if !disabled {
			stagedMinimum = requiredUpdateMinimum(running, previous.version, minimum)
		}
		previous.minimum = cmp.Or(stagedMinimum, required)
		previous.moreAfterRestart = required != "" && stagedMinimum == ""
		m.setUpdateStatus(previous)
		return
	}
	if err != nil {
		if release == nil {
			previous.phase = updateCheckFailed
		}
		previous.minimum = required
		m.setUpdateStatus(previous)
		return
	}
	m.updates.release = release
	if release == nil {
		m.setUpdateStatus(updateStatus{phase: updateCurrent})
		return
	}
	m.setUpdateStatus(updateStatus{
		phase:   updateAvailable,
		version: release.Version,
		notes:   release.Notes,
		minimum: required,
	})
}

func (m *fiveleaf) setUpdateStatus(status updateStatus) {
	previous := m.updates.status
	if status == previous {
		return
	}
	m.updates.status = status
	for _, w := range m.wins {
		w.a.invalidate()
	}
	if previous.version != status.version || (previous.phase == updateDisabled) != (status.phase == updateDisabled) {
		m.refreshMenu()
	}
}

func (m *fiveleaf) focusRequiredUpdate() bool {
	if m.updates.status.minimum == "" {
		return false
	}
	if len(m.wins) == 0 {
		m.openBlankWindow()
	} else {
		m.wins[0].show()
	}
	return true
}

// offerNativeBuild tells, at each launch of the Intel build on a Mac with
// Apple Silicon, that Rosetta runs it slower and crashes it at times, with
// the download of the Apple Silicon build, which updates install too
// (mygo.Updater.Check).
func (m *fiveleaf) offerNativeBuild(h *desktop) {
	if repoURL == "" || !mygo.App.IsTranslated() {
		return
	}
	h.do(func() {
		h.a.tell(note{message: locales.NoteIntelBuild(), label: locales.DownloadButton(), action: func() {
			h.a.services.OpenURL(repoURL + "/releases/latest")
		}})
	})
}

func (h *desktop) update() updateStatus { return h.m.updates.status }

func (h *desktop) checkForUpdates() { h.m.checkUpdate() }

func (h *desktop) installUpdate() {
	m := h.m
	status, release := m.updates.status, m.updates.release
	if release == nil || (status.phase != updateAvailable && status.phase != updateInstallFailed) || m.life != lifeOpen {
		return
	}
	status.phase, status.progress = updateDownloading, -1
	m.setUpdateStatus(status)
	ctx := m.updates.ctx
	go func() {
		defer recoverCrash()
		installCtx, cancel := context.WithTimeout(ctx, updateInstallLimit)
		defer cancel()
		last := -1
		err := release.Install(installCtx, func(downloaded, total int64) {
			if total <= 0 || ctx.Err() != nil {
				return
			}
			progress := int(min(100, 100*downloaded/total))
			if progress == last {
				return
			}
			last = progress
			mygo.RunOnMain(func() {
				status.progress = progress
				m.setUpdateStatus(status)
			})
		})
		if ctx.Err() != nil {
			return
		}
		status.phase, status.progress = updateInstalled, 100
		if err != nil {
			log.Printf("installing update %s: %v", release.Version, err)
			status.phase, status.progress = updateInstallFailed, -1
		}
		mygo.RunOnMain(func() { m.setUpdateStatus(status) })
	}()
}

func (h *desktop) restartForUpdate() {
	m := h.m
	if m.updates.status.phase == updateInstalled && m.life != lifeClosing && m.prepareQuit(mygo.App.Relaunch) {
		mygo.App.Relaunch()
	}
}

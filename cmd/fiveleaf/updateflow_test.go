package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"fiveleaf/internal/cli"
)

func TestUpdateCheckAppliesAndRelaxesPolicy(t *testing.T) {
	const running = "0.20.1"
	release := &mygo.Update{Version: "0.21.0", Notes: "Release notes"}
	m := &fiveleaf{}
	m.finishUpdateCheck(updateStatus{phase: updateIdle}, running, nil, "", errors.New("offline"))
	if got := m.updates.status; got.phase != updateCheckFailed || got.minimum != "" {
		t.Fatalf("first failed check: %#v", got)
	}
	m.finishUpdateCheck(m.updates.status, running, release, "0.21.0", nil)
	required := m.updates.status
	if required.phase != updateAvailable || required.minimum != "0.21.0" || required.notes != release.Notes {
		t.Fatalf("check did not make update required: %#v", required)
	}
	m.finishUpdateCheck(required, running, nil, "0.21.0", errors.New("offline again"))
	if m.updates.release != release || m.updates.status != required {
		t.Fatalf("failed check lost the offer: %#v", m.updates.status)
	}
	m.finishUpdateCheck(required, running, nil, "", errors.New("release withdrawn"))
	if got := m.updates.status; got.phase != updateAvailable || got.minimum != "" || got.version != release.Version {
		t.Fatalf("publisher could not lift the requirement: %#v", got)
	}
	m.finishUpdateCheck(required, running, nil, "0.21.0", nil)
	if got := m.updates.status; got != (updateStatus{phase: updateCurrent}) || m.updates.release != nil {
		t.Fatalf("withdrawn release still offered an update: %#v", got)
	}
	m.finishUpdateCheck(required, running, nil, "0.21.0", mygo.ErrUpdatesDisabled)
	if got := m.updates.status; got != (updateStatus{phase: updateDisabled}) || m.updates.release != nil {
		t.Fatalf("unsupported installation kept an update: %#v", got)
	}
}

func TestUpdatePolicyAppliesToStagedRelease(t *testing.T) {
	release := &mygo.Update{Version: "0.21.0"}
	newer := &mygo.Update{Version: "0.22.0"}
	installed := updateStatus{phase: updateInstalled, progress: 100, version: release.Version}
	m := &fiveleaf{updates: appUpdater{release: release, status: installed}}
	m.finishUpdateCheck(installed, "0.20.1", newer, "0.21.0", nil)
	if got := m.updates.status; got.phase != updateInstalled || got.minimum != "0.21.0" || got.version != release.Version {
		t.Fatalf("new policy did not require restart after staging: %#v", got)
	}
	m.finishUpdateCheck(m.updates.status, "0.20.1", newer, "0.22.0", nil)
	if got := m.updates.status; got.phase != updateInstalled || got.minimum != "0.22.0" || got.version != release.Version || !got.moreAfterRestart {
		t.Fatalf("higher minimum did not require continuing after restart: %#v", got)
	}
	m.finishUpdateCheck(m.updates.status, "0.20.1", nil, "", errors.New("release withdrawn"))
	if m.updates.status != installed {
		t.Fatalf("relaxing the policy lost the staged update: %#v", m.updates.status)
	}
}

func TestUpdateRestartSettlesChanges(t *testing.T) {
	frame, host, tester := settleFixture(t)
	m := frame.m
	frame.active.a.updatesOpen = true
	finished := false
	finish := func() { finished = true }
	if m.prepareQuit(finish) {
		t.Fatal("restart bypassed the save question")
	}
	step(tester)
	if len(host.held) != 1 || finished || frame.active.a.updatesOpen {
		t.Fatal("restart did not wait for a save choice")
	}
	host.held[0](2)
	step(tester)
	if finished || m.life != lifeOpen {
		t.Fatal("cancelled restart did not leave the app open")
	}
	if m.prepareQuit(finish) {
		t.Fatal("restart retry bypassed the save question")
	}
	step(tester)
	if len(host.held) != 2 {
		t.Fatal("restart retry did not ask again")
	}
	host.held[1](0)
	deadline := time.Now().Add(5 * time.Second)
	for !finished && time.Now().Before(deadline) {
		step(tester)
		time.Sleep(time.Millisecond)
	}
	data, err := os.ReadFile(filepath.Join(frame.active.a.root, "main.go"))
	if !finished || m.life != lifeClosing || err != nil || !strings.HasPrefix(string(data), "// changed\n") {
		t.Fatalf("restart did not follow saving: finished=%t life=%v file=%q err=%v", finished, m.life, data, err)
	}
}

func TestRequiredUpdateRejectsWorkspaceIPC(t *testing.T) {
	m := &fiveleaf{updates: appUpdater{status: updateStatus{phase: updateAvailable, version: "0.21.0", minimum: "0.21.0"}}}
	var response cli.Reply
	m.serve(cli.Request{Paths: []cli.Target{{Path: "/example.txt"}}, Wait: true}, func(reply cli.Reply) { response = reply })
	if !strings.Contains(response.Error, "requires version 0.21.0") {
		t.Fatalf("workspace request was not rejected: %#v", response)
	}
}

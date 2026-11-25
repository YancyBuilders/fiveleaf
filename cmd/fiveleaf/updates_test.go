package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

const testUpdateNotes = "## Improvements\n\n- **Restore** the last workspace more quickly.\n- Open recent projects from the title bar."

func TestUpdateIndicator(t *testing.T) {
	hs := newHarness(t, false)
	check := locales.CmdHelpCheckForUpdates()
	if _, ok := hs.tt.Find(check); ok {
		t.Fatal("build without updates has an update control")
	}
	hs.h.updateState.phase = updateIdle
	hs.settle()
	left, ok := hs.tt.Find(check)
	if !ok || left.X > 150 || left.Y >= titleBarH {
		t.Fatalf("check for updates button is outside the top-left corner: %+v", left)
	}
	clickUpdateControl(hs, check)
	if hs.h.updateChecks != 1 || !hs.tt.HasText(locales.UpdateChecking()) {
		t.Fatal("clicking the update icon did not start a visible check")
	}
	hs.h.updateState.phase = updateCurrent
	hs.settle()
	if !hs.tt.HasText(locales.UpdateCurrent()) {
		t.Fatal("up-to-date check result is missing")
	}
	hs.key(0, ui.KeyEscape)
	hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0", notes: testUpdateNotes}
	hs.settle()
	available := locales.UpdateAvailable("0.22.0")
	where, ok := hs.tt.Find(available)
	if !ok || where != left {
		t.Fatalf("update available indicator moved: %+v, want %+v", where, left)
	}
	clickUpdateControl(hs, available)
	if !hs.tt.HasText(locales.UpdateReleaseNotes()) || !hs.tt.HasText("the last workspace more quickly") {
		t.Fatal("clicking the update icon did not show the changelog")
	}
	if hs.h.updateChecks != 1 || hs.h.updateInstalls != 0 {
		t.Fatal("viewing a found release repeated the check or started installation")
	}
	clickUpdateControl(hs, locales.UpdateLater())
	if hs.a.updatesOpen {
		t.Fatal("Later did not dismiss an optional update")
	}
}

func TestUpdateDialogInstallAndRestart(t *testing.T) {
	hs := newHarness(t, true)
	hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0", notes: testUpdateNotes}
	hs.a.run("help.checkForUpdates")
	hs.settle()
	clickUpdateControl(hs, locales.UpdateInstall("0.22.0"))
	if hs.h.updateInstalls != 1 || !hs.tt.HasText(locales.UpdateDownloading()) {
		t.Fatal("install action did not show download progress")
	}
	hs.h.updateState.phase = updateInstallFailed
	hs.settle()
	if !hs.tt.HasText(locales.UpdateInstallFailed()) {
		t.Fatal("the install failure is missing")
	}
	clickUpdateControl(hs, locales.UpdateRetry())
	if hs.h.updateInstalls != 2 {
		t.Fatal("Retry did not attempt installation again")
	}
	hs.h.updateState.phase = updateInstalled
	hs.settle()
	if !hs.tt.HasText(locales.UpdateReady("0.22.0")) {
		t.Fatal("the restart prompt is missing")
	}
	clickUpdateControl(hs, locales.UpdateRestart())
	if hs.h.updateRestarts != 1 {
		t.Fatal("Restart did not request a relaunch")
	}
}

func TestRequiredUpdateGate(t *testing.T) {
	testUpdatePublisher(t)
	for _, test := range []struct {
		name     string
		detached bool
	}{{"workspace", false}, {"detached", true}} {
		t.Run(test.name, func(t *testing.T) {
			hs := newHarness(t, false)
			hs.a.window.Detached = test.detached
			if test.detached {
				hs.tt.SetSize(320, 220)
			}
			path := hs.path("main.go")
			hs.a.openFile(path, true)
			hs.settle()
			hs.insert("// pending save\n")
			if !test.detached {
				hs.a.openQuick("")
			}
			hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0", minimum: "0.21.0", notes: testUpdateNotes}
			hs.settle()
			if !hs.tt.HasText(locales.UpdateMinimum("0.21.0")) || hs.tt.HasText(locales.UpdateLater()) || hs.tt.HasText("main.go") {
				t.Fatal("the required update did not replace the workspace")
			}
			hs.key(0, ui.KeyEscape)
			hs.tt.ClickAt(8, 80)
			hs.tt.Type("unwanted edit")
			hs.a.run("file.new")
			hs.settle()
			if !hs.tt.HasText(locales.UpdateMinimum("0.21.0")) || len(hs.a.docs) != 1 {
				t.Fatal("a shortcut or a click bypassed the required update")
			}
			hs.h.updateState.phase = updateInstallFailed
			hs.settle()
			clickUpdateControl(hs, locales.UpdateDownloadGitHub())
			if got := hs.tt.OpenedURLs(); !slices.Equal(got, []string{repoURL + "/releases/latest"}) {
				t.Fatalf("manual download opened %q", got)
			}
			clickUpdateControl(hs, locales.CmdFileSaveAll())
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), "// pending save") || strings.Contains(string(data), "unwanted edit") {
				t.Fatalf("Save All under the update lock: %q, err %v", data, err)
			}
			clickUpdateControl(hs, locales.UpdateRecheck())
			if hs.h.updateChecks != 1 || !hs.tt.HasText(locales.UpdateMinimum("0.21.0")) {
				t.Fatal("rechecking did not keep the gate while the check was pending")
			}
			hs.h.updateState.phase = updateInstalled
			hs.settle()
			clickUpdateControl(hs, locales.UpdateRecheck())
			if hs.h.updateChecks != 2 || hs.h.updateRestarts != 0 || !hs.tt.HasText(locales.UpdateMinimum("0.21.0")) {
				t.Fatal("a staged update could not recheck the minimum without restarting")
			}
			hs.h.updateState.minimum = ""
			hs.h.updateState.phase = updateAvailable
			hs.settle()
			if hs.tt.HasText(locales.UpdateMinimum("0.21.0")) || !hs.tt.HasText("main.go") {
				t.Fatal("the workspace did not return after the minimum was lifted")
			}
		})
	}
}

func testUpdatePublisher(t testing.TB) {
	t.Helper()
	previousRepo, previousPolicy := repoURL, updatePolicyURL
	repoURL = "https://github.com/example/FiveLeaf"
	updatePolicyURL = "https://raw.githubusercontent.com/example/FiveLeaf/main/update-policy.json"
	t.Cleanup(func() { repoURL, updatePolicyURL = previousRepo, previousPolicy })
}

func TestRequiredUpdateRemoteSaveAs(t *testing.T) {
	t.Setenv("FIVELEAF_SSH_TEST_HOST", "")
	hs, conn, dir := remoteHarness(t)
	hs.a.newFile()
	hs.settle()
	hs.insert("pending save\n")
	hs.h.updateState = updateStatus{phase: updateAvailable, version: "0.22.0", minimum: "0.21.0"}
	hs.settle()
	clickUpdateControl(hs, locales.CmdFileSaveAll())
	if hs.a.prompt == nil || !hs.tt.HasText(locales.SaveAsEllipsis()) {
		t.Fatal("the remote save prompt did not appear over the update gate")
	}
	hs.key(0, ui.KeyEscape)
	if hs.a.prompt != nil || !hs.a.anyDirty() || !hs.tt.HasText(locales.UpdateMinimum("0.21.0")) {
		t.Fatal("canceling the save prompt hid the gate or lost the unsaved document")
	}
	clickUpdateControl(hs, locales.CmdFileSaveAll())
	path := dir + "/before-update.txt"
	hs.tt.Type(path)
	hs.key(0, ui.KeyEnter)
	hs.untilLong("the remote save", func() bool { return !hs.a.anyDirty() })
	data, err := conn.ReadFile(path, 0)
	if err != nil || string(data) != "pending save\n" || hs.h.updateInstalls != 0 || !hs.tt.HasText(locales.UpdateMinimum("0.21.0")) {
		t.Fatalf("remote save under the update lock: %q, err %v", data, err)
	}
}

func clickUpdateControl(hs *harness, label string) {
	hs.t.Helper()
	if err := hs.tt.Click(label); err != nil {
		hs.t.Fatal(err)
	}
	hs.settle()
}

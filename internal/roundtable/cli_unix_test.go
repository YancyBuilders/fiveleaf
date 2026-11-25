//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package roundtable

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCLIAdapterStopsDescendantsHoldingPipes(t *testing.T) {
	for _, mode := range []string{"descendant-parent", "descendant-exit"} {
		t.Run(mode, func(t *testing.T) {
			ready := filepath.Join(t.TempDir(), "child-pid")
			adapter := helperCLI(mode)
			adapter.Env["FIVELEAF_ROUNDTABLE_TEST_READY"] = ready
			if mode == "descendant-parent" {
				adapter.Timeout = 600 * time.Millisecond
			}
			started := time.Now()
			_, err := adapter.Call(context.Background(), nil)
			if mode == "descendant-parent" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("parent timeout = %v", err)
			}
			if mode == "descendant-exit" && !errors.Is(err, exec.ErrWaitDelay) {
				t.Fatalf("child pipe timeout = %v", err)
			}
			if time.Since(started) > 3*time.Second {
				t.Fatalf("descendant kept command open for %v", time.Since(started))
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(waitForFile(t, ready))))
			if err != nil {
				t.Fatal(err)
			}
			// A killed orphan can remain briefly as a zombie until the OS reaps it.
			// Signal 0 must eventually report that the descendant no longer exists.
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("descendant %d survived its parent call", pid)
		})
	}
}

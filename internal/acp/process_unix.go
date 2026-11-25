//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package acp

import (
	"os/exec"
	"syscall"
)

func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopProcess(cmd *exec.Cmd) {
	// Adapters commonly start a second runtime (for example npx -> node).
	// Closing the IDE must also stop children that still hold the stdio pipes.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}

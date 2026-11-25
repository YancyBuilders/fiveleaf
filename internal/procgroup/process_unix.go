//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package procgroup

import (
	"os/exec"
	"syscall"
)

func Prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func Stop(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	groupErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	err := cmd.Process.Kill()
	if groupErr == nil {
		return nil
	}
	return err
}

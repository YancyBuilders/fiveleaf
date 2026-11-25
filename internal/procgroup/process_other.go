//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package procgroup

import "os/exec"

func Prepare(cmd *exec.Cmd) {}

func Stop(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package acp

import "os/exec"

func prepareProcess(cmd *exec.Cmd) {}

func stopProcess(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

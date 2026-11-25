//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package plugin

import (
	"os"

	"golang.org/x/sys/unix"
)

func tryLockPluginFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func unlockPluginFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

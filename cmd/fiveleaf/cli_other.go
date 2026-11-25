//go:build !darwin && !linux && !windows

package main

import "errors"

// commandLine reports whether FiveLeaf was started as the ff command,
// which there is none of here.
func commandLine() bool { return false }

func attachConsole() {}

func startApp([]string) error { return errors.New("the ff command is not supported here") }

func refreshCLI() {}

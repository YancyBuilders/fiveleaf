package main

import "os"

// commandLine reports whether FiveLeaf was started as the ff command,
// which on Linux is FiveLeaf itself: with arguments, unless detach or a
// snapshot started it.
func commandLine() bool {
	return os.Getenv(cliEnv) != "" || len(os.Args) > 1 && os.Getenv(detachedEnv) == "" && os.Getenv("FIVELEAF_SNAPSHOT") == ""
}

// attachConsole does nothing: Linux starts with the command's handles.
func attachConsole() {}

// startApp starts FiveLeaf apart from the command, opening paths.
func startApp(paths []string) error { return startDetached(paths, detachedEnv+"=1") }

// refreshCLI does nothing: the command is FiveLeaf itself.
func refreshCLI() {}

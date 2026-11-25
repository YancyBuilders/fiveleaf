package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fiveleaf/internal/cli"

	"github.com/egoist/mygo"
)

// commandLine reports whether the ff script started FiveLeaf, as the
// command.
func commandLine() bool { return os.Getenv(cliEnv) != "" }

// attachConsole does nothing: the shell script starts a console executable.
func attachConsole() {}

// startApp starts the app the command is in, opening paths, with open, as
// the Finder would; a development build, in no app, runs itself again.
func startApp(paths []string) error {
	app := appLocation()
	if app == "" {
		return startDetached(paths)
	}
	if out, err := exec.Command("open", append([]string{"-a", app}, paths...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("open: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// refreshCLI updates previously installed shell commands.
func refreshCLI() {
	app := appLocation()
	if app == "" || !mygo.App.IsPackaged() {
		return
	}
	home, _ := os.UserHomeDir()
	go func() { cli.Refresh(cli.InstalledDirs(home, filepath.SplitList(os.Getenv("PATH"))), app) }()
}

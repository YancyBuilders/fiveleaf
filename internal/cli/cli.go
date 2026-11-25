// Package cli is the ff shell command, which opens folders and files
// in the app and runs its commands: it installs and uninstalls the
// command, reads its arguments, and carries them to the running app over
// a socket.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// bundleID is the app's identifier, as mygo.json gives it.
const bundleID = "dev.fiveleaf.editor"

const commandName = "ff"
const legacyName = "fiveleaf"

func scriptMark(name string) string {
	return "Installed by FiveLeaf: File > Install '" + name + "' Command."
}

// script is the shell command. It runs the app's own executable, which,
// told so by FIVELEAF_CLI, asks the running app over its socket, or starts
// it. On macOS, app is its .app, else the system finds it by identifier,
// so it keeps working when the app moves; on Windows, app is FiveLeaf.exe.
func script(name, app string) string {
	if runtime.GOOS == "windows" {
		// A percent is doubled in a batch file, and delayed expansion would
		// consume exclamation marks in the executable's path.
		app = strings.ReplaceAll(app, "%", "%%")
		return strings.ReplaceAll(`@echo off
REM ff [options] [folder or file]... opens them in FiveLeaf: ff --help says more.
REM `+scriptMark(name)+`
setlocal DisableDelayedExpansion
set "FIVELEAF_CLI=1"
if not exist "`+app+`" (
	echo ff: FiveLeaf.exe is not found. Run File ^> Install 'ff' Command again. 1>&2
	exit /b 1
)
"`+app+`" %*
exit /b %ERRORLEVEL%
`, "\n", "\r\n")
	}
	return `#!/bin/sh
# ff [options] [folder or file]... opens them in FiveLeaf: ff --help says more.
# ` + scriptMark(name) + `
app=` + shellQuote(app) + `
if [ ! -x "$app/Contents/MacOS/FiveLeaf" ]; then
	app=$(mdfind "kMDItemCFBundleIdentifier == '` + bundleID + `'" 2>/dev/null | head -n 1)
fi
if [ ! -x "$app/Contents/MacOS/FiveLeaf" ]; then
	echo "ff: FiveLeaf.app is not found" >&2
	exit 1
fi
FIVELEAF_CLI=1 exec "$app/Contents/MacOS/FiveLeaf" "$@"
`
}

// commandPath is the command's file in dir: a batch file on Windows, a
// shell script elsewhere.
func commandPath(dir, name string) string {
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	return filepath.Join(dir, name)
}

func readScript(path, name string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	header, err := io.ReadAll(io.LimitReader(file, 512))
	if err != nil || !bytes.Contains(header, []byte(scriptMark(name))) {
		return "", err
	}
	tail, err := io.ReadAll(file)
	return string(header) + string(tail), err
}

func foreignCommand(dirs []string) string {
	for _, dir := range dirs {
		if runtime.GOOS == "windows" {
			powershell := filepath.Join(dir, commandName+".ps1")
			if info, err := os.Stat(powershell); err == nil && !info.IsDir() {
				return powershell
			}
		}
		path, err := exec.LookPath(filepath.Join(dir, commandName))
		if err != nil {
			continue
		}
		if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".cmd") {
			return path
		}
		if contents, err := readScript(path, commandName); err != nil || contents == "" {
			return path
		}
	}
	return ""
}

// installDir is FiveLeaf's own Windows command folder, not another tool's.
func installDir(home string) string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "FiveLeaf", "bin")
	}
	return filepath.Join(home, "AppData", "Local", "FiveLeaf", "bin")
}

// Dirs are installation folders the command interpreter searches. Windows
// uses FiveLeaf's own folder, which AddToUserPath puts on the user's PATH;
// elsewhere the known user folders come first, then the system-wide one.
func Dirs(home string, path []string) []string {
	if runtime.GOOS == "windows" {
		return []string{installDir(home)}
	}
	var dirs []string
	path = cleanPath(path)
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"), "/usr/local/bin"} {
		if slices.Contains(path, filepath.Clean(dir)) {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// InstalledDirs also finds copies in older or custom locations, so they
// can still be refreshed or removed after the installation policy changes.
func InstalledDirs(home string, path []string) []string {
	var dirs []string
	for _, dir := range cleanPath(path) {
		if filepath.IsAbs(dir) {
			dirs = append(dirs, dir)
		}
	}
	if runtime.GOOS == "windows" {
		if dir := installDir(home); !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
		return dirs
	}
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"), "/usr/local/bin"} {
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// ErrCanceled is the user declining to give the password.
var ErrCanceled = errors.New("canceled")

// InstallOnPath puts ff on the shell's path and returns where it went.
func InstallOnPath(home string, path []string, app string) (string, error) {
	path = cleanPath(path)
	dirs := Dirs(home, path)
	if other := foreignCommand(slices.Concat(path, dirs)); other != "" {
		return "", fmt.Errorf("%s is another program's", other)
	}
	for _, dir := range path {
		if !filepath.IsAbs(dir) || slices.Contains(dirs, dir) {
			continue
		}
		if contents, err := readScript(commandPath(dir, commandName), commandName); err == nil && contents != "" {
			dirs = append(dirs, dir)
		}
	}
	return Install(dirs, app)
}

func Install(dirs []string, app string) (string, error) {
	guidance := "add ~/.local/bin to your shell's PATH and try again"
	if runtime.GOOS == "windows" {
		guidance = `check that %LOCALAPPDATA%\FiveLeaf\bin is writable and try again`
	}
	var candidates, installed []string
	// Check every candidate before writing, so another program's command
	// is neither replaced nor shadowed. Skip folders we cannot inspect.
	for _, dir := range dirs {
		p := commandPath(dir, commandName)
		contents, err := readScript(p, commandName)
		if err != nil {
			if os.IsNotExist(err) {
				candidates = append(candidates, dir)
				continue
			}
			if errors.Is(err, os.ErrPermission) {
				continue
			}
			if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
				continue
			}
			return "", err
		}
		if contents == "" {
			return "", fmt.Errorf("%s is another program's", p)
		}
		installed = append(installed, dir)
	}
	// Reinstall in place rather than leave a second copy earlier on PATH.
	if len(installed) > 0 {
		candidates = installed
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no installation directory available on your PATH; %s", guidance)
	}
	var err error
	var admin bool
	for _, dir := range candidates {
		p := commandPath(dir, commandName)
		if err = write(p, script(commandName, app), len(installed) > 0); err == nil {
			return p, nil
		}
		if dir == "/usr/local/bin" && errors.Is(err, os.ErrPermission) {
			admin = true
		}
	}
	// Ask only after trying every user-writable destination, and only for
	// the traditional system-wide destination when it is on PATH.
	if admin {
		if err = writeAsAdmin("/usr/local/bin", app, len(installed) > 0); err == nil {
			return commandPath("/usr/local/bin", commandName), nil
		}
		if errors.Is(err, ErrCanceled) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not install ff; %s: %w", guidance, err)
}

func write(path, contents string, replace bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var file *os.File
	var err error
	if replace {
		file, err = os.CreateTemp(dir, ".ff-*")
	} else {
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	}
	if err != nil {
		return err
	}
	_, err = file.WriteString(contents)
	if replace && err == nil {
		err = file.Chmod(0o755)
	}
	if err = errors.Join(err, file.Close()); err != nil {
		os.Remove(file.Name())
		return err
	}
	if replace {
		defer os.Remove(file.Name())
		return os.Rename(file.Name(), path)
	}
	return nil
}

func writeAsAdmin(dir, app string, replace bool) error {
	f, err := os.CreateTemp("", "fiveleaf-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(script(commandName, app))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	dest := shellQuote(commandPath(dir, commandName))
	redirect := ">"
	if replace {
		redirect = ">|"
	}
	return runAsAdmin("set -C; mkdir -p " + shellQuote(dir) + " && cat " + shellQuote(f.Name()) + " " + redirect + dest + " && chmod 755 " + dest)
}

// Refresh writes the command of the app at app over the copies of
// FiveLeaf's in dirs that an older FiveLeaf wrote, where no password is needed.
func Refresh(dirs []string, app string) {
	var legacyDirs []string
	hasCommand := false
	for _, dir := range dirs {
		for _, name := range []string{commandName, legacyName} {
			path := commandPath(dir, name)
			old, err := readScript(path, name)
			if err != nil || old == "" {
				continue
			}
			if name == legacyName {
				legacyDirs = append(legacyDirs, dir)
			} else {
				hasCommand = true
			}
			if updated := script(name, app); old != updated {
				write(path, updated, true)
			}
		}
	}
	if !hasCommand && len(legacyDirs) > 0 && foreignCommand(dirs) == "" {
		for _, dir := range legacyDirs {
			if write(commandPath(dir, commandName), script(commandName, app), false) == nil {
				break
			}
		}
	}
}

// Uninstall removes the current and legacy launchers installed by FiveLeaf.
func Uninstall(dirs []string) ([]string, error) {
	var removed []string
	for _, dir := range dirs {
		for _, name := range []string{commandName, legacyName} {
			path := commandPath(dir, name)
			contents, err := readScript(path, name)
			if err != nil || contents == "" {
				continue
			}
			err = os.Remove(path)
			if errors.Is(err, os.ErrPermission) {
				err = runAsAdmin("rm -f " + shellQuote(path))
			}
			if err != nil {
				return removed, err
			}
			removed = append(removed, path)
		}
	}
	return removed, nil
}

// runAsAdmin runs a shell command as an administrator, which macOS asks
// the password for.
func runAsAdmin(cmd string) error {
	script := `do shell script "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(cmd) + `" with administrator privileges`
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "(-128)") {
			return ErrCanceled
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// ShellPath is where the user's shell looks for commands, which an app
// opened from the Finder does not inherit: the login shell's folders
// first, in its order, then those of PATH it lacks.
func ShellPath() []string {
	return cleanPath(append(LoginPath(), filepath.SplitList(os.Getenv("PATH"))...))
}

// LoginPath is where the command interpreter looks. On Windows it is the
// process's PATH; elsewhere only the login shell's, not Finder-only
// fallback directories that the user's terminal cannot find.
func LoginPath() []string {
	if runtime.GOOS == "windows" {
		return cleanPath(filepath.SplitList(os.Getenv("PATH")))
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const mark = "FIVELEAF_PATH="
	// printenv preserves PATH's colon separators in fish as well as sh/zsh.
	// A shell may exit non-zero after printing; keep any usable output.
	out, _ := exec.CommandContext(ctx, sh, "-ilc", `printf '`+mark+`'; /usr/bin/printenv PATH`).Output()
	var all []string
	for line := range strings.Lines(string(out)) {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), mark); ok {
			all = append(all, filepath.SplitList(p)...)
		}
	}
	return cleanPath(all)
}

func cleanPath(all []string) []string {
	var path []string
	for _, p := range all {
		if p = filepath.Clean(p); p != "." && !slices.Contains(path, p) {
			path = append(path, p)
		}
	}
	return path
}

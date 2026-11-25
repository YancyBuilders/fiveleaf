package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The ff command runs the app's executable with its arguments, told it
// is the command.
func TestScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	app := filepath.Join(dir, "My FiveLeaf.app")
	moved := filepath.Join(dir, "Moved.app")
	for _, a := range []string{app, moved} {
		exe := filepath.Join(a, "Contents", "MacOS", "FiveLeaf")
		os.MkdirAll(filepath.Dir(exe), 0o755)
		os.WriteFile(exe, []byte("#!/bin/sh\necho \"$0\" \"$FIVELEAF_CLI\"\nfor a in \"$@\"; do echo \"$a\"; done\n"), 0o755)
	}
	if _, err := Install([]string{bin}, app); err != nil {
		t.Fatal(err)
	}
	// mdfind knows the app that moved.
	os.WriteFile(filepath.Join(bin, "mdfind"), []byte("#!/bin/sh\necho '"+moved+"'\n"), 0o755)
	run := func(args ...string) []string {
		cmd := exec.Command(filepath.Join(bin, "ff"), args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin")
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	}
	exe := filepath.Join(app, "Contents", "MacOS", "FiveLeaf")
	if got := run(); !slices.Equal(got, []string{exe + " 1"}) {
		t.Errorf("no args: %v", got)
	}
	got := run("-g", "a b.go:3", "run")
	if want := []string{exe + " 1", "-g", "a b.go:3", "run"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	os.RemoveAll(app)
	got = run("x")
	if want := []string{filepath.Join(moved, "Contents", "MacOS", "FiveLeaf") + " 1", "x"}; !slices.Equal(got, want) {
		t.Errorf("moved: got %v, want %v", got, want)
	}
}

// The Windows command is a batch file that runs the app's executable,
// preserving its arguments and exit status.
func TestWindowsScript(t *testing.T) {
	if os.Getenv("FIVELEAF_TEST_HELPER") != "" {
		fmt.Println(os.Getenv("FIVELEAF_CLI"))
		for _, arg := range os.Args[1:] {
			fmt.Printf("%q\n", arg)
		}
		os.Exit(7)
	}
	if runtime.GOOS != "windows" {
		t.Skip("a Windows batch file")
	}
	comspec := os.Getenv("COMSPEC")
	if comspec == "" {
		comspec = `C:\Windows\System32\cmd.exe`
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	app := filepath.Join(dir, "My 100% FiveLeaf", "FiveLeaf.exe")
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(app), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app, data, 0o755); err != nil {
		t.Fatal(err)
	}
	localData := filepath.Join(dir, "Local")
	t.Setenv("LOCALAPPDATA", localData)
	bin := filepath.Join(localData, "FiveLeaf", "bin")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", filepath.Join(dir, "missing"))
	p, err := InstallOnPath(dir, LoginPath(), app)
	if err != nil || p != filepath.Join(bin, "ff.cmd") {
		t.Fatalf("installed at %q: %v", p, err)
	}
	if _, err := InstallOnPath(dir, LoginPath(), app); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	args := []string{"/d", "/c", p, "-test.run=TestWindowsScript", "--", "-g", "a b.go:3"}
	cmd := exec.Command(comspec, args...)
	cmd.Env = append(os.Environ(), "FIVELEAF_TEST_HELPER=1")
	out, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 7 {
		t.Fatalf("command exit: %v\n%s", err, out)
	}
	got := strings.ReplaceAll(string(out), "\r\n", "\n")
	want := "1\n\"-test.run=TestWindowsScript\"\n\"--\"\n\"-g\"\n\"a b.go:3\"\n"
	if got != want {
		t.Fatalf("command output:\n%q\nwant:\n%q", got, want)
	}
	if err := os.Remove(app); err != nil {
		t.Fatal(err)
	}
	out, err = exec.Command(comspec, "/d", "/c", p).CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || !strings.Contains(string(out), "FiveLeaf.exe is not found") {
		t.Fatalf("missing app exit: %v\n%s", err, out)
	}
	removed, err := Uninstall([]string{bin})
	if err != nil || !slices.Equal(removed, []string{p}) {
		t.Fatalf("removed %v: %v", removed, err)
	}
}

// Refresh brings an older FiveLeaf's command up to date, and leaves another
// program's.
func TestRefresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	old, foreign := filepath.Join(dir, "old"), filepath.Join(dir, "foreign")
	os.MkdirAll(old, 0o755)
	os.MkdirAll(foreign, 0o755)
	os.WriteFile(filepath.Join(old, "ff"), []byte("#!/bin/sh\n# "+scriptMark("ff")+"\nopen -b x \"$@\"\n"), 0o755)
	os.WriteFile(filepath.Join(foreign, "ff"), []byte("#!/bin/sh\necho other\n"), 0o755)
	Refresh([]string{old, foreign, filepath.Join(dir, "none")}, "/Apps/FiveLeaf.app")
	if data, _ := os.ReadFile(filepath.Join(old, "ff")); string(data) != script("ff", "/Apps/FiveLeaf.app") {
		t.Errorf("not refreshed: %s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(foreign, "ff")); string(data) != "#!/bin/sh\necho other\n" {
		t.Errorf("another program's changed: %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "none", "ff")); err == nil {
		t.Error("installed where there was none")
	}
}

func TestInstallAndUninstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	p, err := Install([]string{first, second}, "")
	if err != nil || p != filepath.Join(first, "ff") {
		t.Fatalf("installed at %q: %v", p, err)
	}
	if info, err := os.Stat(p); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("not executable: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(first, "fiveleaf")); !os.IsNotExist(err) {
		t.Fatalf("fresh installation created a legacy command: %v", err)
	}
	// Again, over its own.
	if _, err := Install([]string{first}, ""); err != nil {
		t.Fatal(err)
	}
	// Another program's command stays.
	os.MkdirAll(second, 0o755)
	foreign := filepath.Join(second, "ff")
	os.WriteFile(foreign, []byte("#!/bin/sh\necho other\n"), 0o755)
	if _, err := Install([]string{second}, ""); err == nil {
		t.Error("overwrote another program's ff")
	}
	removed, err := Uninstall([]string{first, second})
	if err != nil || !slices.Equal(removed, []string{p}) {
		t.Fatalf("removed %v: %v", removed, err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Error("removed another program's ff")
	}
}

func TestLegacyLauncherUpgrade(t *testing.T) {
	for _, name := range []string{"available", "occupied", "foreign legacy"} {
		t.Run(name, func(t *testing.T) {
			dir, other := t.TempDir(), t.TempDir()
			legacy := commandPath(dir, "fiveleaf")
			current := commandPath(dir, "ff")
			original := "# Installed by FiveLeaf: File > Install 'fiveleaf' Command.\n"
			if name == "foreign legacy" {
				original = "another tool\n"
			}
			if err := os.WriteFile(legacy, []byte(original), 0o755); err != nil {
				t.Fatal(err)
			}
			foreign := commandPath(other, "ff")
			if name == "occupied" {
				if err := os.WriteFile(foreign, []byte("another tool\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			dirs := []string{dir, other}
			Refresh(dirs, "Moved FiveLeaf")
			Refresh(dirs, "Moved FiveLeaf")
			if name == "available" {
				if got, err := os.ReadFile(current); err != nil || string(got) != script("ff", "Moved FiveLeaf") {
					t.Fatalf("new ff launcher: %s, %v", got, err)
				}
			} else if _, err := os.Lstat(current); !os.IsNotExist(err) {
				t.Fatalf("unexpected ff launcher: %v", err)
			}
			if name != "foreign legacy" {
				if got, err := os.ReadFile(legacy); err != nil || string(got) != script("fiveleaf", "Moved FiveLeaf") {
					t.Fatalf("legacy launcher: %s, %v", got, err)
				}
			}
			removed, err := Uninstall(dirs)
			if err != nil || slices.Contains(removed, current) != (name == "available") || slices.Contains(removed, legacy) != (name != "foreign legacy") {
				t.Fatalf("removed %v: %v", removed, err)
			}
			if name == "occupied" || name == "foreign legacy" {
				if name == "foreign legacy" {
					foreign = legacy
				}
				if got, err := os.ReadFile(foreign); err != nil || string(got) != "another tool\n" {
					t.Fatalf("changed another tool: %s, %v", got, err)
				}
			}
		})
	}
}

func TestInstallOnPathPreservesOtherFF(t *testing.T) {
	home, other := t.TempDir(), t.TempDir()
	t.Setenv("LOCALAPPDATA", home)
	foreign := filepath.Join(other, "ff")
	if runtime.GOOS == "windows" {
		foreign += ".exe"
	}
	if err := os.WriteFile(foreign, []byte("another tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := []string{filepath.Join(home, "bin"), other}
	if _, err := InstallOnPath(home, path, "FiveLeaf"); err == nil || !strings.Contains(err.Error(), foreign) {
		t.Fatalf("expected a conflict at %s: %v", foreign, err)
	}
	for _, dir := range Dirs(home, path) {
		if _, err := os.Lstat(commandPath(dir, "ff")); !os.IsNotExist(err) {
			t.Fatalf("shadowed %s: %v", foreign, err)
		}
	}
}

func TestDirs(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(home, ".local", "bin")
	bin := filepath.Join(home, "bin")
	custom := filepath.Join(home, ".nvm", "versions", "node", "bin")
	if runtime.GOOS == "windows" {
		localData := filepath.Join(home, "Local")
		t.Setenv("LOCALAPPDATA", localData)
		own := filepath.Join(localData, "FiveLeaf", "bin")
		if got := Dirs(home, []string{custom}); !slices.Equal(got, []string{own}) {
			t.Fatalf("Windows installation uses another tool's folder: %v", got)
		}
		if got := InstalledDirs(home, []string{custom}); !slices.Equal(got, []string{custom, own}) {
			t.Fatalf("Windows management destinations: %v", got)
		}
		return
	}
	for _, tt := range []struct {
		name       string
		path, want []string
	}{
		{"prefer user folders", []string{"/usr/local/bin", bin, local, local}, []string{local, bin, "/usr/local/bin"}},
		{"tool folders", []string{custom, "/opt/homebrew/bin", "/Applications/Other.app/Contents/bin", local}, []string{local}},
		{"relative paths", []string{".", "bin", ""}, nil},
		{"empty PATH", nil, nil},
		{"fish-style output", []string{local + " /usr/bin /bin"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Dirs(home, tt.path); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	dirs := InstalledDirs(home, []string{custom})
	if want := []string{custom, local, bin, "/usr/local/bin"}; !slices.Equal(dirs, want) {
		t.Fatalf("management destinations: %v, want %v", dirs, want)
	}
}

func TestInstallReachableCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	home := t.TempDir()
	if _, err := Install(Dirs(home, []string{"/usr/bin", "/bin", "/opt/homebrew/bin"}), ""); err == nil || !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("missing PATH guidance: %v", err)
	}
	local := filepath.Join(home, ".local", "bin")
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := InstallOnPath(home, []string{local, bin}, "/Applications/FiveLeaf.app")
	if err != nil || p != filepath.Join(bin, "ff") {
		t.Fatalf("user installation: %q, %v", p, err)
	}
	Refresh([]string{bin}, "/New/FiveLeaf.app")
	if data, _ := os.ReadFile(p); string(data) != script("ff", "/New/FiveLeaf.app") {
		t.Fatalf("command was not refreshed: %s", data)
	}
	removed, err := Uninstall([]string{bin})
	if err != nil || !slices.Contains(removed, p) {
		t.Fatalf("command was not removed: %v, %v", removed, err)
	}
}

func TestInstallReusesExistingCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	home := t.TempDir()
	local := filepath.Join(home, ".local", "bin")
	bin := filepath.Join(home, "bin")
	if _, err := Install([]string{bin}, "/Old/FiveLeaf.app"); err != nil {
		t.Fatal(err)
	}
	p, err := InstallOnPath(home, []string{local, bin}, "/New/FiveLeaf.app")
	if err != nil || p != filepath.Join(bin, "ff") {
		t.Fatalf("reinstalled at %q: %v", p, err)
	}
	if data, _ := os.ReadFile(p); string(data) != script("ff", "/New/FiveLeaf.app") {
		t.Fatalf("existing command not updated: %s", data)
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("created another installation directory: %v", err)
	}
}

func TestInstallOnPathReusesCustomLauncher(t *testing.T) {
	home, custom := t.TempDir(), t.TempDir()
	t.Setenv("LOCALAPPDATA", home)
	existing, err := Install([]string{custom}, "Old FiveLeaf")
	if err != nil {
		t.Fatal(err)
	}
	path := []string{filepath.Join(home, "bin"), custom}
	installed, err := InstallOnPath(home, path, "New FiveLeaf")
	if err != nil || installed != existing {
		t.Fatalf("installed at %s, want %s: %v", installed, existing, err)
	}
	if contents, err := os.ReadFile(existing); err != nil || string(contents) != script("ff", "New FiveLeaf") {
		t.Fatalf("custom launcher was not updated: %v", err)
	}
}

func TestInstallSkipsUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("Unix directory permissions")
	}
	dir := t.TempDir()
	blocked, writable := filepath.Join(dir, "blocked"), filepath.Join(dir, "writable")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(blocked, 0o755) })
	if p, err := Install([]string{blocked, writable}, ""); err != nil || p != filepath.Join(writable, "ff") {
		t.Fatalf("unreadable directory blocked installation: %q, %v", p, err)
	}
	// A readable, unwritable folder must give useful guidance too.
	if err := os.Chmod(blocked, 0o555); err != nil {
		t.Fatal(err)
	}
	if _, err := Install([]string{blocked}, ""); err == nil || !strings.Contains(err.Error(), "add ~/.local/bin") {
		t.Fatalf("missing failure guidance: %v", err)
	}
}

func TestPreserveHomebrewCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "brew-wrapper")
	data := []byte("#!/bin/sh\n# Managed by Homebrew; use brew uninstall --cask fiveleaf to remove it.\n")
	if err := os.WriteFile(wrapper, data, 0o755); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(dir, "ff")
	if err := os.Symlink(wrapper, command); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	if _, err := Install([]string{empty, dir}, ""); err == nil || !strings.Contains(err.Error(), "another program") {
		t.Fatalf("expected ownership protection: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(empty, "ff")); !os.IsNotExist(err) {
		t.Fatalf("shadowed Homebrew's command: %v", err)
	}
	Refresh([]string{dir}, "/Applications/FiveLeaf.app")
	if removed, err := Uninstall([]string{dir}); err != nil || len(removed) != 0 {
		t.Fatalf("removed Homebrew command: %v, %v", removed, err)
	}
	if target, err := os.Readlink(command); err != nil || target != wrapper {
		t.Fatalf("changed Homebrew symlink: %q, %v", target, err)
	}
	if got, _ := os.ReadFile(wrapper); !slices.Equal(got, data) {
		t.Fatalf("changed Homebrew wrapper: %s", got)
	}
}

func TestPreserveBrokenCommandLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "ff")
	target := filepath.Join(dir, "missing")
	if err := os.Symlink(target, p); err != nil {
		t.Fatal(err)
	}
	if _, err := Install([]string{dir}, ""); err == nil {
		t.Fatal("replaced an unowned broken symlink")
	}
	if got, err := os.Readlink(p); err != nil || got != target {
		t.Fatalf("changed broken symlink: %q, %v", got, err)
	}
}

// Opened from the Finder, the app has the system's folders only: the login
// shell's come first, and none twice.
func TestShellPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	tools := t.TempDir()
	sh := filepath.Join(t.TempDir(), "zsh")
	script := "#!/bin/sh\nPATH=" + tools + ":/usr/bin:$PATH; export PATH; shift; exec /bin/sh -c \"$1\"\n"
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", sh)
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	want := []string{tools, "/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	if got := ShellPath(); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoginPathExcludesFinderFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	sh := filepath.Join(dir, "shell")
	if err := os.WriteFile(sh, []byte("#!/bin/sh\necho FIVELEAF_PATH=/usr/bin:/bin\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", sh)
	t.Setenv("PATH", "/usr/local/bin:/usr/bin:/bin")
	if got := Dirs(dir, LoginPath()); len(got) != 0 {
		t.Fatalf("installation chose a Finder-only directory: %v", got)
	}
	if got := ShellPath(); !slices.Contains(got, "/usr/local/bin") {
		t.Fatalf("application lost its fallback PATH: %v", got)
	}
	t.Setenv("SHELL", filepath.Join(dir, "missing"))
	if got := LoginPath(); len(got) != 0 {
		t.Fatalf("failed shell produced installation candidates: %v", got)
	}
}

func TestLoginPathKeepsOutputOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	sh := filepath.Join(t.TempDir(), "shell")
	if err := os.WriteFile(sh, []byte("#!/bin/sh\necho FIVELEAF_PATH=/usr/bin:/bin\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", sh)
	if got := LoginPath(); !slices.Equal(got, []string{"/usr/bin", "/bin"}) {
		t.Fatalf("discarded usable output: %v", got)
	}
}

func TestLoginPathFish(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish is not installed")
	}
	sh := filepath.Join(t.TempDir(), "shell")
	// Suppress the user's fish configuration while running the actual query.
	if err := os.WriteFile(sh, []byte("#!/bin/sh\nexec "+shellQuote(fish)+" --no-config -c \"$2\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "Home with spaces")
	local := filepath.Join(home, ".local", "bin")
	t.Setenv("SHELL", sh)
	t.Setenv("PATH", local+":/usr/bin:/bin")
	if got := LoginPath(); !slices.Equal(got, []string{local, "/usr/bin", "/bin"}) {
		t.Fatalf("fish PATH lost directory boundaries: %q", got)
	}
	p, err := InstallOnPath(home, LoginPath(), "/Applications/FiveLeaf.app")
	if err != nil || p != filepath.Join(local, "ff") {
		t.Fatalf("fish installation: %q, %v", p, err)
	}
}

package plugin

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"fiveleaf/internal/plugin/bundled"
)

// UserDirs are the folders of the user's plugins. Existing Mayoi data is
// reused only until a FiveLeaf folder is created.
func UserDirs() Dirs {
	base := os.Getenv("FIVELEAF_DATA_DIR")
	if base == "" {
		base = os.Getenv("MAYOI_DATA_DIR")
	}
	if base == "" {
		home, _ := os.UserHomeDir()
		base = userPluginDir(home)
	}
	return Dirs{
		Bundled: bundled.Tools,
		Plugins: filepath.Join(base, "plugins"),
		Scripts: filepath.Join(base, "scripts"),
		Storage: filepath.Join(base, "storage"),
		Themes:  filepath.Join(base, "themes"),
	}
}

func userPluginDir(home string) string {
	base := filepath.Join(home, ".fiveleaf")
	if _, err := os.Stat(base); os.IsNotExist(err) {
		old := filepath.Join(home, ".mayoi")
		if info, err := os.Stat(old); err == nil && info.IsDir() {
			return old
		}
	}
	return base
}

// CLIUsage is what ff plugin --help prints.
const CLIUsage = `Usage: ff plugin <command> [arguments]

Manages the plugins in %s. A running FiveLeaf loads changes at once.

Commands:
  new <name> [folder]       Write a new plugin into folder, ./<name> by default
  new <name> --theme        Write a theme plugin, with no code
  new <name> --tool         Write a Toolbox tool, with an HTML page
  new <name> --tool=vue     Write a Vue Toolbox page with a Vite build
  new <name> --tool=react   Write a React Toolbox page with a Vite build
  new <name> --debug-adapter  Write a debug adapter plugin and a sample program
  dev [folder]              Load a plugin from its folder, . by default, and again on each save
  install <source>[@ref]    Install by index name, Git URL, or GitHub owner/repo (add #subdirectory to select a plugin)
  update [name]...          Update plugins installed from Git, all by default
  outdated                  List installed plugins with a newer release
  remove <name>...          Remove plugins, or the links of ff plugin dev
  list                      List the installed plugins
  search [text]             Search the index of plugins
  pack [folder]             Copy the npm packages a plugin imports into vendor/

Examples:
  ff plugin new word-count
  ff plugin new midnight --theme
  ff plugin new my-tool --tool
  ff plugin new sample-debug --debug-adapter
  ff plugin install one-dark-pro
  ff plugin install YancyBuilders/fiveleaf-plugins#material-icons
  ff plugin install https://github.com/me/my-plugin.git@v1.2.0
`

// RunCLI runs ff plugin with its arguments, and returns the status to
// exit with.
func RunCLI(args []string, dirs Dirs, stdout, stderr io.Writer) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "ff plugin: "+format+"\n", a...)
		return 1
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintf(stdout, CLIUsage, shortHome(dirs.Plugins))
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sub, rest := args[0], args[1:]
	switch sub {
	case "new":
		if len(rest) == 0 {
			return fail("new needs a name")
		}
		name, dir, kind := rest[0], "", ScaffoldPlugin
		kinds := map[string]ScaffoldKind{"--theme": ScaffoldTheme, "--tool": ScaffoldToolbox, "--tool=vue": ScaffoldVueTool, "--tool=react": ScaffoldReactTool, "--debug-adapter": ScaffoldDebugAdapter}
		for _, arg := range rest[1:] {
			if choice, ok := kinds[arg]; ok {
				if kind != ScaffoldPlugin {
					return fail("choose one plugin template")
				}
				kind = choice
			} else if strings.HasPrefix(arg, "--") {
				return fail("unknown template option %s", arg)
			} else if dir == "" {
				dir = arg
			} else {
				return fail("new takes one folder")
			}
		}
		if dir == "" {
			dir = name
		}
		if err := ScaffoldKindOf(dir, name, kind); err != nil {
			return fail("%v", err)
		}
		if kind == ScaffoldVueTool || kind == ScaffoldReactTool {
			fmt.Fprintf(stdout, "Wrote %s. In that folder, run:\n  npm install\n  npm run build\n  ff plugin dev .\n", dir)
		} else {
			fmt.Fprintf(stdout, "Wrote %s. Try it with:\n  cd %s && ff plugin dev\n", dir, dir)
		}
	case "dev":
		dir := "."
		if len(rest) > 0 {
			dir = rest[0]
		}
		m, err := dirs.LinkDev(dir)
		if err != nil {
			return fail("%v", err)
		}
		fmt.Fprintf(stdout, "Linked %s: FiveLeaf loads it from %s, and again on each save.\n", m.Name, dir)
		fmt.Fprintf(stdout, "Its log is in FiveLeaf under Plugins: Show Log. Unlink it with ff plugin remove %s.\n", m.Name)
	case "install":
		if len(rest) != 1 {
			return fail("install needs one source")
		}
		m, err := InstallSpec(ctx, dirs.Plugins, rest[0], nil)
		if err != nil {
			return fail("%v", err)
		}
		fmt.Fprintf(stdout, "Installed %s %s.\n", m.Name, m.Version)
	case "update":
		records, err := readRecords(dirs.Plugins)
		if err != nil {
			return fail("%v", err)
		}
		names := rest
		if len(names) == 0 {
			for name := range records {
				names = append(names, name)
			}
			sort.Strings(names)
		}
		catalog := sync.OnceValues(func() ([]IndexEntry, error) { return fetchUpdateIndex(ctx) })
		status := 0
		for _, name := range names {
			src := records[name]
			if src.Pinned() {
				fmt.Fprintf(stdout, "Kept %s at %s.\n", name, src.Version)
				continue
			}
			changed, err := updateSource(ctx, dirs.Plugins, name, src, catalog)
			switch {
			case err != nil:
				status = fail("%s: %v", name, err)
			case changed:
				fmt.Fprintf(stdout, "Updated %s.\n", name)
			default:
				fmt.Fprintf(stdout, "%s is up to date.\n", name)
			}
		}
		return status
	case "outdated":
		records, err := readRecords(dirs.Plugins)
		if err != nil {
			return fail("%v", err)
		}
		names := make([]string, 0, len(records))
		for name, src := range records {
			if src.TrackRelease {
				names = append(names, name)
			}
		}
		var entries []IndexEntry
		if len(names) > 0 {
			entries, err = fetchUpdateIndex(ctx)
			if err != nil {
				return fail("reading the index: %v", err)
			}
		}
		sort.Strings(names)
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		n := 0
		for _, name := range names {
			entry, ok := FindIndex(entries, name)
			release, available := records[name].NewerRelease(entry)
			if !ok || !available {
				continue
			}
			n++
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, records[name].Version, release.Version, entry.Category)
		}
		tw.Flush()
		if n == 0 {
			fmt.Fprintln(stdout, "All plugins are up to date.")
		}
	case "remove", "uninstall":
		if len(rest) == 0 {
			return fail("remove needs a name")
		}
		status := 0
		for _, name := range rest {
			if err := Remove(dirs.Plugins, name); err != nil {
				status = fail("%v", err)
				continue
			}
			fmt.Fprintf(stdout, "Removed %s.\n", name)
		}
		return status
	case "list", "ls":
		recs := ReadRecords(dirs.Plugins)
		entries, _ := os.ReadDir(dirs.Plugins)
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		n := 0
		for _, e := range entries {
			dir := filepath.Join(dirs.Plugins, e.Name())
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				continue
			}
			n++
			m, err := ReadManifest(dir)
			if err != nil {
				fmt.Fprintf(tw, "%s\t\t%v\n", e.Name(), err)
				continue
			}
			from := recs[m.Name].URL
			if fi, _ := os.Lstat(dir); fi != nil && fi.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(dir)
				from = "dev: " + shortHome(target)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.Name, m.Version, orDash(m.Category), from, orDash(recs[m.Name].Version))
		}
		tw.Flush()
		if n == 0 {
			fmt.Fprintf(stdout, "No plugins in %s.\n", shortHome(dirs.Plugins))
		}
	case "search":
		entries, err := FetchIndex(ctx, nil)
		if err != nil {
			if len(entries) == 0 {
				return fail("reading the index: %v", err)
			}
			fmt.Fprintf(stderr, "Showing the bundled plugin list (%v).\n", err)
		}
		q := strings.ToLower(strings.Join(rest, " "))
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		for _, e := range entries {
			hay := strings.ToLower(e.Name + " " + e.DisplayName + " " + e.Description + " " + e.Category + " " + strings.Join(e.Tags, " "))
			if q == "" || strings.Contains(hay, q) {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Name, e.DisplayName, orDash(e.Version), orDash(e.Category), e.Description)
			}
		}
		tw.Flush()
	case "pack":
		dir := "."
		if len(rest) > 0 {
			dir = rest[0]
		}
		res, err := Pack(dir)
		if err != nil {
			return fail("%v", err)
		}
		for _, w := range res.Warnings {
			fmt.Fprintf(stderr, "warning: %s\n", w)
		}
		fmt.Fprintf(stdout, "Copied %d files into %s and pointed %d of the plugin's files at them.\n", len(res.Files), VendorDir, len(res.Rewrote))
	default:
		return fail("no command %s: ff plugin --help lists them", sub)
	}
	return 0
}

func shortHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

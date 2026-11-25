// FiveLeaf is a small, quick code editor: a file tree, tabs of editors and a
// terminal panel, drawn by MyGo's native UI.
package main

import (
	"cmp"
	"errors"
	"fiveleaf/internal/cli"
	"fiveleaf/internal/keymap"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// desktop is a project of the app, a folder and what shows of it, in a
// window of its own or, with project tabs, one of a window's: the host of
// its native dialogs, the trash.
type desktop struct {
	m *fiveleaf
	a *app
	f *frame // the window it is in
	// logoDir is the folder the logo scan is for, logoIcon the icon of
	// its own it tried first, logoSrc its favicon or logo once decoded,
	// logoDone whether it was scanned, logoScannedAt when, and logoGen
	// drops the scans a newer one superseded.
	logoDir       string
	logoIcon      string
	logoSrc       ui.ImageSource
	logoDone      bool
	logoScannedAt time.Time
	logoGen       int
}

// lifecycle is how far the app, or a window, is along closing: open,
// settling the changes of its projects, or closing.
type lifecycle int

const (
	lifeOpen lifecycle = iota
	lifeSettling
	lifeClosing
)

// fiveleaf is the app's projects, the focused one last first, and its
// windows.
type fiveleaf struct {
	s      *settings.Settings
	keys   *keymap.Map // the shortcuts of keybindings.json
	wins   []*desktop
	frames []*frame
	// pending are the folders and files the app was asked to open before
	// it was ready, and pendingTargets the files a URL asked for with a
	// line and column.
	ready          bool
	pending        []string
	pendingTargets []cli.Target
	// life is how far the app is along quitting: closing once it quits,
	// so that the windows it closes stay in the settings, to open again
	// at the next start.
	life lifecycle
	// asking is the project whose question about its changes shows while
	// the app, or a window, settles them.
	asking *desktop
	// menuKey is the key (menuKey) of the menu last set, which is set
	// again only when it changes.
	menuKey      string
	menuRevision uint64 // rejects deferred updates superseded by a newer menu
	// icons is the folder of the icons chosen for projects on the rail,
	// beside the settings, "" without a settings file.
	icons   string
	updates appUpdater
	// prefs is the settings window, nil while it is closed.
	prefs *prefsWindow
	// sidebar is what of the sidebar every project shares, once one has
	// (sidebarSeen).
	sidebar     sidebarShare
	sidebarSeen bool
}

func (h *desktop) openFolder(done func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: h.f.win, Title: locales.DialogOpenFolder(), ButtonLabel: locales.DialogOpen(), Directory: true, CreateDirectories: true,
		})
		if err == nil && len(paths) > 0 {
			h.a.post(func() { done(paths[0]) })
		}
	}()
}

func (h *desktop) choosePaths(title, button, dir string, folders bool, done func([]string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: h.f.win, Title: title, ButtonLabel: button, DefaultPath: dir,
			Directory: folders, Multiple: !folders, CreateDirectories: folders,
		})
		if err == nil && len(paths) > 0 {
			h.a.post(func() { done(paths) })
		}
	}()
}

// chooseIcon asks for a picture to show for the project of dir on the
// rail, starting in its folder.
func (h *desktop) chooseIcon(dir string, done func(string)) {
	exts := make([]string, len(iconExts))
	for i, e := range iconExts {
		exts[i] = strings.TrimPrefix(e, ".")
	}
	if remote.Is(dir) {
		dir = ""
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: h.f.win, Title: locales.DialogChooseIcon(), ButtonLabel: locales.DialogChoose(), DefaultPath: dir,
			Filters: []mygo.FileFilter{{Name: locales.DialogImages(), Extensions: exts}},
		})
		if err == nil && len(paths) > 0 {
			h.a.post(func() { done(paths[0]) })
		}
	}()
}

func (h *desktop) saveAs(name string, done func(string)) {
	dir := h.a.root
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: h.f.win, Title: locales.DialogSaveAs(), DefaultPath: filepath.Join(dir, name), CreateDirectories: true,
		})
		if err != nil {
			path = ""
		}
		h.a.post(func() { done(path) })
	}()
}

func (h *desktop) exportAs(path, ext string, done func(string)) {
	go func() {
		selected, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: h.f.win, Title: locales.DialogExport(), DefaultPath: path, CreateDirectories: true,
			Filters: []mygo.FileFilter{{Name: strings.ToUpper(ext), Extensions: []string{ext}}},
		})
		if err != nil {
			h.a.post(func() { h.a.fail("%s", locales.FailSave(filepath.Base(path), err.Error())) })
		} else if selected != "" {
			h.a.post(func() { done(selected) })
		}
	}()
}

func (h *desktop) confirmClose(name string, done func(int)) {
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: h.f.win, Type: mygo.MessageWarning,
			Message: locales.SaveChangesQuestion(name),
			Detail:  locales.SaveChangesDetail(),
			Buttons: []string{locales.ButtonSave(), locales.ButtonDontSave(), locales.ButtonCancel()}, DefaultButton: 0, CancelButton: 2,
		})
		choice := 2
		if err == nil {
			choice = res.Button
		}
		h.a.post(func() { done(choice) })
	}()
}

func (h *desktop) trash(path string) error { return mygo.Shell.TrashItem(path) }
func (h *desktop) nativeDrags() bool       { return true }
func (h *desktop) shown() bool {
	return h.f.active == h && h.f.win.IsVisible() && !h.f.win.IsMinimized()
}
func (h *desktop) reveal(path string) {
	if !remote.Is(path) {
		mygo.Shell.ShowItemInFolder(path)
	}
}
func (h *desktop) copyFiles(paths []string) error {
	return mygo.Clipboard.WriteFiles(paths...)
}
func (h *desktop) clipboardFiles() []string {
	paths, _ := mygo.Clipboard.ReadFiles()
	return paths
}
func (h *desktop) clipboardImage() []byte     { return mygo.Clipboard.ReadImage() }
func (h *desktop) copyImage(png []byte) error { return mygo.Clipboard.WriteImage(png) }

// setTitle titles the window, while it shows the project.
func (h *desktop) setTitle(title string) {
	if h.f.active == h {
		h.f.win.SetTitle(title)
	}
}

// closeWindow closes the project, and the window with its last.
func (h *desktop) closeWindow() {
	if !h.a.window.Detached || h.f.life == lifeOpen && h.m.life == lifeOpen {
		h.closeProject()
	}
}
func (h *desktop) setTheme(theme, appearance string) {
	setThemeSource(themeSource(theme, appearance))
	// From one theme to another the native appearance may stay, so the
	// other windows would not draw anew.
	for _, w := range h.m.wins {
		w.a.invalidate()
	}
	// The appearances are greyed out under The Matrix.
	h.m.refreshMenu()
}

func (h *desktop) setLocale(tag string) {
	mygo.App.SetLocale(tag)
	// The language may show as it did, as English for the system's
	// English: the menu's choice changes all the same.
	h.m.refreshMenu()
}

func (h *desktop) showFolder(dir string) bool {
	if w := h.m.showing(dir); w != nil && w != h {
		w.show()
		return true
	}
	return false
}

func (h *desktop) newWindow(dir string) {
	h.m.openFolder(filepath.Clean(dir), false)
}

func (h *desktop) openProjectWindow(dir string) {
	h.m.openProjectFolder(filepath.Clean(dir), false, true)
}

func (h *desktop) newBlankWindow() { h.m.openBlankWindow() }

func (h *desktop) openFolders(dirs []string) {
	for _, dir := range dirs {
		h.m.openFolder(dir, true)
	}
}

func (h *desktop) recentChanged() { h.m.refreshMenu() }
func (h *desktop) menuChanged()   { h.m.refreshMenu() }

func (h *desktop) showSettings(section, key string) { h.m.showPrefs(section, key, h) }

func (h *desktop) redrawWindows() {
	h.m.redrawWindows()
}

func (m *fiveleaf) redrawWindows() {
	for _, w := range m.wins {
		w.a.invalidate()
	}
	m.prefsChanged()
}

func (h *desktop) installCLI() {
	go func() {
		home, _ := os.UserHomeDir()
		p, err := cli.InstallOnPath(home, cli.LoginPath(), appLocation())
		if err == nil {
			err = cli.AddToUserPath(home, p)
		}
		switch {
		case errors.Is(err, cli.ErrCanceled):
		case err != nil:
			h.tell(mygo.MessageWarning, locales.CliInstallFailed(), err.Error())
		default:
			// Terminals that were open keep the PATH they started with.
			detail := locales.CliInstalledDetail()
			if runtime.GOOS == "windows" {
				detail = locales.CliInstalledDetailWindows()
			}
			h.tell(mygo.MessageInfo, locales.CliInstalled(shortenHome(filepath.Dir(p))), detail)
		}
	}()
}

func (h *desktop) uninstallCLI() {
	go func() {
		home, _ := os.UserHomeDir()
		// PATH is the login shell's already, as main set it at start.
		removed, err := cli.Uninstall(cli.InstalledDirs(home, filepath.SplitList(os.Getenv("PATH"))))
		if err == nil && len(removed) > 0 {
			err = cli.RemoveFromUserPath(home)
		}
		switch {
		case errors.Is(err, cli.ErrCanceled):
		case err != nil:
			h.tell(mygo.MessageWarning, locales.CliUninstallFailed(), err.Error())
		case len(removed) == 0:
			h.tell(mygo.MessageInfo, locales.CliNotInstalled(), "")
		default:
			h.tell(mygo.MessageInfo, locales.CliUninstalled(), locales.CliUninstalledDetail(shortenHome(filepath.Dir(removed[0]))))
		}
	}()
}

// tell shows a message on the window.
func (h *desktop) tell(kind mygo.MessageType, message, detail string) {
	mygo.Dialog.Message(mygo.MessageOptions{Parent: h.f.win, Type: kind, Message: message, Detail: detail})
}

// show brings the window to the front, showing the project.
func (h *desktop) show() {
	h.f.activate(h)
	h.f.show()
}

// do runs fn on the window's app before its next frame.
func (h *desktop) do(fn func()) {
	h.a.post(fn)
	if h.f.win != nil {
		h.f.win.Invalidate()
	}
}

// settingsPath is where the settings are kept.
func settingsPath() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "settings.json")
}

func main() {
	mygo.App.SetName("FiveLeaf")
	keyRepeat()
	// FIVELEAF_DATA_DIR keeps the settings and window state elsewhere, such as
	// scripts/dev.sh's FiveLeaf-dev, so a development app leaves FiveLeaf's alone.
	if dir := cmp.Or(os.Getenv("FIVELEAF_DATA_DIR"), os.Getenv("MAYOI_DATA_DIR")); dir != "" {
		mygo.App.SetPath(mygo.PathUserData, dir)
	}
	// Started as the ff command, FiveLeaf asks the app that runs, and
	// exits.
	commandLineClient()
	// From a terminal on Linux, FiveLeaf runs on apart from it; before the
	// lock, which the FiveLeaf it starts takes.
	if detach() {
		return
	}
	// One FiveLeaf runs: the folders and files another is started with open
	// in this one.
	if os.Getenv("FIVELEAF_SNAPSHOT") == "" && !mygo.App.RequestSingleInstanceLock() {
		return
	}
	// After the lock, so that a second FiveLeaf leaves the first's crash.log
	// alone; a snapshot, which takes no lock, leaves it alone too.
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && os.Getenv("FIVELEAF_SNAPSHOT") == "" {
		keepCrashes(dir)
		keepStderr(dir)
	}
	// Opened from the Finder or the Dock, FiveLeaf has the system's PATH
	// only; git, its hooks and what else FiveLeaf runs look for commands
	// where the user's shell does. In the background, as a slow shell
	// would hold up the window.
	shellPathReady := make(chan struct{})
	if runtime.GOOS != "windows" && os.Getenv("FIVELEAF_SNAPSHOT") == "" {
		go func() {
			os.Setenv("PATH", strings.Join(cli.ShellPath(), string(os.PathListSeparator)))
			close(shellPathReady)
		}()
	} else {
		close(shellPathReady)
	}
	m := &fiveleaf{}
	mygo.App.OnSecondInstance(m.secondInstance)
	// The folders and files of `fiveleaf` and the Finder, also those the app
	// starts with.
	mygo.App.OnOpenFile(func(path string) {
		if !m.ready {
			m.pending = append(m.pending, path)
			return
		}
		m.open([]string{path})
	})
	// A fiveleaf:// URL, as LocatorJS opens a file, once the windows are
	// ready to show it.
	mygo.App.OnOpenURL(func(rawURL string) {
		t, ok := openURLTarget(rawURL)
		if !ok {
			return
		}
		if !m.ready {
			m.pendingTargets = append(m.pendingTargets, t)
			return
		}
		m.openURL(t)
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !m.beforeQuit() {
			e.PreventDefault()
		}
	})
	mygo.App.OnQuit(func() {
		waitForClosingMeetings()
		// A quit before the app was ready has no settings to save, as
		// one that Ctrl+C ends as it starts.
		if m.s != nil {
			m.s.Flush()
		}
		// The ssh masters, which would outlive the app.
		remote.Shutdown()
	})
	mygo.App.WhenReady(func() {
		if err := mygo.Protocol.Handle(toolboxScheme, toolboxSurfaceFiles(toolboxFiles())); err != nil {
			panic(err)
		}
		mygo.BindAs("Toolbox", toolboxService{m})
		m.s = settings.Load(settingsPath())
		if p := settingsPath(); p != "" {
			m.icons = filepath.Join(filepath.Dir(p), "project-icons")
		}
		setThemeSource(themeSource(m.s.Theme, m.s.Appearance))
		// Once the appearance is set, and the app is ready to take one.
		mygo.Theme.OnUpdated(setDockIcon)
		setDockIcon()
		// The language of the settings, unless MYGO_LOCALE (mygo dev -lang)
		// chose one for the run.
		if os.Getenv("MYGO_LOCALE") == "" {
			applyLocale(m.s.Locale)
		}
		mygo.App.OnLocaleChanged(func(string) { m.localeChanged() })
		// A menu or a button changing a preference shows in the settings
		// window.
		m.s.OnPrefsChanged(m.prefsChanged)
		if stop, err := m.s.Watch(func(p settings.Prefs) { mygo.RunOnMain(func() { m.reload(p) }) }); err == nil {
			mygo.App.OnQuit(stop)
		}
		keysPath := keymap.Path(m.s.Path())
		keys, keysErr := keymap.Load(keysPath)
		m.keys = keys
		if stop, err := keymap.Watch(keysPath, func(km *keymap.Map, err error) { mygo.RunOnMain(func() { m.reloadKeys(km, err) }) }); err == nil {
			mygo.App.OnQuit(stop)
		}
		wd, _ := os.Getwd()
		m.ready = true
		m.launch(append(m.pending, argPaths(os.Args[1:], wd)...))
		m.pending = nil
		for _, t := range m.pendingTargets {
			m.openURL(t)
		}
		m.pendingTargets = nil
		m.listenCLI()
		pathAtLoad := os.Getenv("PATH")
		m.startPlugins(slices.Contains(os.Args[1:], "--disable-plugins"))
		reg := plugins.reg
		go func() {
			<-shellPathReady
			if os.Getenv("PATH") != pathAtLoad {
				reg.Load()
			}
			refreshCLI()
		}()
		// A plugin's theme may be dark, which its plugin tells.
		setThemeSource(themeSource(m.s.Theme, m.s.Appearance))
		m.startUpdates()
		if len(m.wins) == 0 {
			return
		}
		first := m.wins[0]
		first.a.keysFailed(keys, keysErr)
		m.setMenu(first.a)
		if os.Getenv("FIVELEAF_SNAPSHOT") == "" {
			m.offerCrashes(first)
			m.offerNativeBuild(first)
		}
		if out := os.Getenv("FIVELEAF_SNAPSHOT"); out != "" {
			// Capture the real window once it settles. A sequence also checks
			// native animations without pointer or keyboard events.
			a, win := first.a, first.f.win
			go func() {
				// FIVELEAF_SNAPSHOT_RUN names commands to run first.
				for _, id := range strings.Fields(os.Getenv("FIVELEAF_SNAPSHOT_RUN")) {
					time.Sleep(600 * time.Millisecond)
					a.post(func() { a.run(id) })
				}
				time.Sleep(2500 * time.Millisecond)
				frames, _ := strconv.Atoi(os.Getenv("FIVELEAF_SNAPSHOT_FRAMES"))
				for i := range min(max(frames, 1), 10) {
					if i > 0 {
						time.Sleep(2 * time.Second)
					}
					path := out
					if i > 0 {
						ext := filepath.Ext(out)
						path = fmt.Sprintf("%s-%02d%s", strings.TrimSuffix(out, ext), i+1, ext)
					}
					if png, err := win.CapturePage(); err == nil {
						if err := os.WriteFile(path, png, 0o644); err != nil {
							log.Printf("snapshot: %v", err)
						}
					} else {
						log.Printf("snapshot: %v", err)
					}
				}
				mygo.App.Quit()
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// argPaths are the folders and files of a command line, absolute.
func argPaths(args []string, wd string) []string {
	var paths []string
	for _, arg := range args {
		// A fiveleaf:// URL, which Windows and Linux start the app with,
		// reaches OnOpenURL.
		if arg == "" || strings.HasPrefix(arg, "-") || isFiveLeafURL(arg) {
			continue
		}
		// A file:// URL, which Linux file managers pass for %U.
		if u, err := url.Parse(arg); err == nil && u.Scheme == "file" {
			arg = filepath.FromSlash(u.Path)
			if runtime.GOOS == "windows" {
				arg = strings.TrimPrefix(arg, `\`) // file:///C:/x
			}
		}
		if !filepath.IsAbs(arg) {
			arg = filepath.Join(wd, arg)
		}
		paths = append(paths, arg)
	}
	return paths
}

// launch opens the windows the app starts with: one for each folder it
// was given, or else those of the last session. The files it was given
// open in them.
func (m *fiveleaf) launch(paths []string) {
	folders, files := splitPaths(paths)
	switch {
	case m.s.ProjectTabs:
		// Every project of the last session, in the rail's order, the one
		// focused last shown, and then the folders it was given.
		var front *settings.Window
		if len(m.s.Windows) > 0 {
			front = m.s.Windows[0]
		}
		for _, w := range m.s.InRail() {
			m.openWindow(w)
		}
		if h := m.of(front); h != nil {
			h.show()
		}
		for _, dir := range folders {
			m.openFolder(dir, true)
		}
	case len(folders) == 0:
		// The one focused last ends up in front.
		for _, w := range slices.Backward(slices.Clone(m.s.Windows)) {
			m.openWindow(w)
		}
	default:
		// A folder of the last session opens as it was left. The other
		// windows of the session are let go, keeping their folders'
		// sessions.
		var keep []*settings.Window
		for _, w := range m.s.Windows {
			if w.Detached || slices.Contains(folders, filepath.Clean(w.Folder)) && !slices.ContainsFunc(keep, func(k *settings.Window) bool { return !k.Detached && k.Folder == w.Folder }) {
				keep = append(keep, w)
			} else {
				m.s.Stash(w)
			}
		}
		m.s.Windows = keep
		for _, w := range slices.Clone(keep) {
			if w.Detached {
				m.openWindow(w)
			}
		}
		for _, dir := range slices.Backward(folders) {
			m.openFolder(dir, false)
		}
	}
	if len(m.wins) == 0 {
		m.openBlankWindow()
	}
	for _, p := range files {
		m.openFile(p)
	}
}

// secondInstance opens the folders and files another FiveLeaf was started
// with, or brings the first window to the front. Before the app is
// ready, when it has no settings yet, they wait for launch.
func (m *fiveleaf) secondInstance(args []string, wd string) {
	paths := argPaths(args, wd)
	if !m.ready {
		m.pending = append(m.pending, paths...)
		return
	}
	if len(paths) > 0 {
		m.open(paths)
	} else if len(m.wins) > 0 {
		m.wins[0].show()
	}
}

// open opens folders and files the app is asked to while it runs.
func (m *fiveleaf) open(paths []string) {
	if m.focusRequiredUpdate() {
		return
	}
	folders, files := splitPaths(paths)
	for _, dir := range folders {
		m.openFolder(dir, true)
	}
	for _, p := range files {
		m.openFile(p)
	}
}

// splitPaths sorts paths into folders and files, cleaned, without those
// that are neither.
func splitPaths(paths []string) (folders, files []string) {
	for _, p := range paths {
		info, err := os.Stat(p)
		switch p = filepath.Clean(p); {
		case err != nil:
		case info.IsDir():
			if !slices.Contains(folders, p) {
				folders = append(folders, p)
			}
		case info.Mode().IsRegular():
			// MyGo also hands the files of its associations to
			// OnOpenFile.
			if !slices.Contains(files, p) {
				files = append(files, p)
			}
		}
	}
	return folders, files
}

// openFolder brings the window that has dir open to the front, or opens
// it in a new window. reuse lets a window that shows nothing take it
// instead, as the one an app opens with.
func (m *fiveleaf) openFolder(dir string, reuse bool) {
	m.openProjectFolder(dir, reuse, false)
}

// openProjectFolder can require a native window of the project's own,
// without changing how the ordinary Open Folder command uses tabs.
func (m *fiveleaf) openProjectFolder(dir string, reuse, separate bool) {
	if w := m.showing(dir); w != nil {
		if separate && w.f != nil && len(w.f.tabs) > 1 {
			w.f.releaseProject(w)
			m.newFrame(w)
		}
		w.show()
		return
	}
	if reuse {
		for _, w := range m.wins {
			if a := w.a; a.blank() {
				// Its folder at once, so that the next folder looks for
				// another window.
				a.window.Folder = dir
				w.do(func() { a.openSession(dir) })
				w.show()
				return
			}
		}
	}
	for _, w := range m.s.Windows {
		if !w.Detached && filepath.Clean(w.Folder) == dir && m.of(w) == nil {
			// As the last session left it.
			m.openProjectState(w, separate)
			return
		}
	}
	// With the tabs it had when a window last closed it.
	w := m.s.NewWindow()
	w.Folder = dir
	if ses := m.s.TakeSession(dir); ses != nil {
		w.Session = *ses
	}
	m.openProjectState(w, separate)
}

// openFile opens a file in the window whose folder holds it, else in the
// window focused last, else in a new window.
func (m *fiveleaf) openFile(path string) { m.openFileIn(nil, path, nil) }

// openFileIn opens a file as openFile does, or in win when it is not nil,
// then calls then, if not nil, with its tab shown.
func (m *fiveleaf) openFileIn(in *desktop, path string, then func(a *app)) {
	win := in
	for _, other := range m.wins {
		if other.a.showsDetachedFile(path) && (in == nil || in.a.docIndex(path) < 0) {
			win = other
			break
		}
	}
	if win == nil || win.a.window.Detached && !win.a.showsDetachedFile(path) {
		win = m.workspaceFor(path)
	}
	if root := win.a.folder(); root != "" && inFolder(root, path) {
		// As the window spells its folder, which on Windows a path given
		// in other letters, c:\proj for C:\Proj, is too: the file opens
		// once, as the Explorer has it.
		path = filepath.Join(root, path[len(strings.TrimSuffix(root, string(filepath.Separator))):])
	}
	a := win.a
	win.do(func() {
		a.openFile(path, true)
		if then != nil {
			then(a)
		}
	})
	win.show()
}

func (m *fiveleaf) workspaceFor(path string) *desktop {
	var target *desktop
	deepest := 0
	for _, h := range m.wins {
		if h.a.window.Detached {
			continue
		}
		if target == nil {
			target = h
		}
		if root := h.a.folder(); len(root) > deepest && (root == path || inFolder(root, path)) {
			target, deepest = h, len(root)
		}
	}
	if target == nil {
		target = m.openWindow(m.s.NewWindow())
	}
	return target
}

// showing returns the window that has dir open.
func (m *fiveleaf) showing(dir string) *desktop {
	for _, w := range m.wins {
		if !w.a.window.Detached && w.a.folder() == dir {
			return w
		}
	}
	return nil
}

// of returns the window whose state is st.
func (m *fiveleaf) of(st *settings.Window) *desktop {
	for _, w := range m.wins {
		if w.a.window == st {
			return w
		}
	}
	return nil
}

// setThemeSource sets the light or dark appearance of the native windows,
// as the preferences change: the tests, which have none, leave it.
var setThemeSource = func(s mygo.ThemeSource) {
	mygo.Theme.SetSource(s)
	setDockIcon()
}

// reload applies the preferences of the settings file, which a person
// changed as FiveLeaf runs.
func (m *fiveleaf) reload(p settings.Prefs) {
	old := m.s.SetPrefs(p)
	if p.Appearance != old.Appearance || p.Theme != old.Theme {
		setThemeSource(themeSource(p.Theme, p.Appearance))
	}
	if p.WindowOpacity != old.WindowOpacity {
		m.applyWindowOpacity()
	}
	if p.ProjectTabs != old.ProjectTabs {
		m.projectTabsChanged()
	}
	if p.Locale != old.Locale {
		applyLocale(p.Locale)
	}
	for _, w := range m.wins {
		if p.AutoSave && !old.AutoSave {
			w.a.autoSaveAll()
		}
		w.a.checkAgentConfig()
		w.a.invalidate()
	}
	if !sameThemes(p.Themes, old.Themes) {
		themeFileChanged()
		for _, w := range m.wins {
			w.a.palette, w.a.hints = nil, nil
			w.a.invalidate()
		}
	}
	if p.Appearance != old.Appearance || p.Theme != old.Theme {
		setDockIcon()
	}
	m.refreshMenu()
	m.prefsChanged()
	if plugins != nil {
		plugins.syncPrefs()
		plugins.toggled(old.Plugins, p.Plugins)
		plugins.reg.Emit("settings.changed", map[string]any{})
		if p.Theme != old.Theme || p.Appearance != old.Appearance {
			plugins.reg.Emit("theme.changed", map[string]any{"theme": p.Theme, "appearance": p.Appearance})
		}
	}
	if p.Keymap != old.Keymap {
		for _, w := range m.wins {
			w.a.applyKeymap(m.keys)
			w.a.invalidate()
		}
		m.refreshMenu()
	}
}

// applyWindowOpacity shows the desktop through the windows as
// WindowOpacity says, behind a frosted material, and keeps them opaque
// at 1.
func (m *fiveleaf) applyWindowOpacity() {
	opacity := m.s.WindowOpacity
	for _, w := range m.frames {
		if w != nil && w.win != nil {
			setWindowOpacity(w.win, opacity)
		}
	}
	if m.prefs != nil && m.prefs.win != nil {
		setWindowOpacity(m.prefs.win, opacity)
	}
}

// setWindowOpacity puts a frosted material behind a window that shows the
// desktop through it, and takes it away at 1.
func setWindowOpacity(win *mygo.Window, opacity float64) {
	win.SetOpacity(1)
	win.SetVibrancy(windowVibrancy(opacity))
	win.Invalidate()
}

// windowVibrancy is the material of a window of this opacity: a frosted
// glass, which keeps text legible over the desktop, or none at 1.
func windowVibrancy(opacity float64) mygo.Vibrancy {
	if opacity >= 1 && runtime.GOOS != "windows" {
		return mygo.VibrancyNone
	}
	return mygo.VibrancyGlass
}

// dockIcon reads the app icon of theme: the bundle's own light tile for a
// light window, or the dark tile that ships beside it, which macOS would
// not pick by itself.
func dockIcon(dark bool) []byte {
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return nil
	}
	name := "icon.png"
	if dark {
		name = "icon-dark.png"
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil
	}
	return data
}

// dockIconCache holds the tiles read so far, so that a theme change does
// not read them again.
var dockIconCache struct {
	sync.Mutex
	tiles [2][]byte
}

// cachedDockIcon is dockIcon, which keeps what it read.
func cachedDockIcon(dark bool) []byte {
	i := 0
	if dark {
		i = 1
	}
	dockIconCache.Lock()
	defer dockIconCache.Unlock()
	if icon := dockIconCache.tiles[i]; icon != nil {
		return icon
	}
	icon := dockIcon(dark)
	dockIconCache.tiles[i] = icon
	return icon
}

// setDockIcon shows the app icon of the light or dark appearance on the
// Dock.
func setDockIcon() {
	if !mygo.App.IsReady() {
		return
	}
	dark := false
	if mygo.Theme.IsDark() {
		dark = true
	}
	icon := cachedDockIcon(dark)
	if len(icon) > 0 {
		mygo.App.Dock.SetIcon(icon)
	}
}

// reloadKeys applies the shortcuts of keybindings.json, which a person
// saved as FiveLeaf runs: those of a file that does not parse stay.
func (m *fiveleaf) reloadKeys(km *keymap.Map, err error) {
	if err == nil {
		m.keys = km
		for _, w := range m.wins {
			w.a.applyKeymap(km)
			w.a.invalidate()
		}
		m.refreshMenu()
	}
	if len(m.wins) == 0 {
		return
	}
	front := m.wins[0]
	front.do(func() { front.a.keysFailed(km, err) })
}

// applyLocale shows the app in the language of tag, or of the system's
// languages for "" or a tag that names none.
func applyLocale(tag string) {
	if i18n.Normalize(tag) == "" {
		tag = ""
	}
	mygo.App.SetLocale(tag)
}

// localeChanged says anew what the windows made once in the language they
// showed: the menus and the command palette. The windows draw anew
// themselves.
func (m *fiveleaf) localeChanged() {
	for _, w := range m.wins {
		w.a.palette, w.a.quick.stale = nil, true
		w.a.invalidate()
	}
	m.menuKey = ""
	m.refreshMenu()
	m.prefsChanged()
}

// refreshMenu builds the menus anew on the window in front, whose folder
// the recent folders of the File menu leave out.
func (m *fiveleaf) refreshMenu() {
	m.prefsChanged()
	if !m.ready || len(m.wins) == 0 {
		return
	}
	front := m.wins[0]
	front.do(func() { m.setMenu(front.a) })
}

// setMenu sets the app's menu as a's window has it, and the Dock's menu of
// a new window and the recent folders.
func (m *fiveleaf) setMenu(a *app) {
	if runtime.GOOS != "darwin" {
		return
	}
	// Setting a menu bar on Linux makes it anew, which closes a menu that
	// is open, as when the window has the focus again from a submenu's
	// grab: one that does not change stays.
	menu := a.menu(runtime.GOOS == "darwin" && mygo.App.IsPackaged(), m.run)
	if apply := m.menuUpdate(menu); apply != nil {
		if runtime.GOOS == "linux" {
			// The view drains app.post during GTK's draw/render callback.
			// SetMenu destroys the old menu bar, which GTK may still have
			// in its list of children to draw. Dispatch from another
			// goroutine so RunOnMain queues the update instead of running
			// it inline inside that traversal. Do not wait here.
			go mygo.RunOnMain(apply)
		} else {
			apply()
		}
	}
	mygo.App.Dock.SetMenu(m.dockMenu())
}

// menuUpdate captures the menu on the UI thread. Its returned function
// must also run there, after drawing on Linux. Dispatching goroutines may
// reach the main loop out of order, so only the latest revision applies.
func (m *fiveleaf) menuUpdate(menu *mygo.Menu) func() {
	key := menuKey(menu.Items())
	if key == m.menuKey {
		return nil
	}
	m.menuKey = key
	m.menuRevision++
	revision := m.menuRevision
	return func() {
		if revision == m.menuRevision && m.life != lifeClosing {
			mygo.App.SetMenu(menu)
		}
	}
}

// dockMenu is the menu of the Dock icon: a window that shows nothing, and
// the recent folders, each in the window in front, which comes forward.
func (m *fiveleaf) dockMenu() *mygo.Menu {
	items := []*mygo.MenuItem{{Label: locales.CmdFileNewWindow(), Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
		m.openBlankWindow()
	}}}
	recent := m.s.RecentExcept("")
	if len(recent) > 0 {
		items = append(items, mygo.Separator())
	}
	for _, dir := range recent {
		id := recentCommand + dir
		items = append(items, &mygo.MenuItem{Label: shortenHome(dir), Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
			if len(m.wins) > 0 {
				m.wins[0].show()
			}
			m.run(id, nil)
		}})
	}
	return mygo.NewMenu(items)
}

// menuKey writes out what items show of themselves, their submenus'
// too, the same for menus that look and do the same: an item runs the
// command of its label, in the window it is chosen in.
func menuKey(items []*mygo.MenuItem) string {
	var b strings.Builder
	var write func(items []*mygo.MenuItem)
	write = func(items []*mygo.MenuItem) {
		for _, it := range items {
			fmt.Fprintf(&b, "%q %q %v %v %q %t %t %t %q [", it.ID, it.Label, it.Type, it.Role, it.Accelerator,
				it.Disabled, it.Hidden, it.Checked, it.ToolTip)
			write(it.Submenu)
			b.WriteString("]")
		}
	}
	write(items)
	return b.String()
}

// run runs a command of the menu in the window it was chosen in, or the
// one focused last.
func (m *fiveleaf) run(id string, win *mygo.Window) {
	if len(m.wins) == 0 && id == "help.checkForUpdates" && m.life == lifeOpen {
		m.openBlankWindow()
	}
	if len(m.wins) == 0 {
		return
	}
	// The menu toggled the item chosen, which the command may leave as
	// it was: the menu is set anew as the next one is made.
	m.menuKey = ""
	// The settings window takes the shortcut of a menu as a key pressed
	// while it records keys, closes as a tab would, and finds in its
	// search.
	if w := m.prefs; w != nil && win == w.win {
		switch {
		case w.keys.recordAccel(id):
			return
		case id == "file.close":
			w.win.Close()
			return
		case id == "file.settings":
			return
		case id == "edit.find":
			w.find()
			return
		case strings.HasPrefix(id, "edit.") || strings.HasPrefix(id, "sel."):
			// The editing of the window behind it.
			return
		}
	}
	h := m.wins[0]
	for _, f := range m.frames {
		if f.win == win && f.active != nil {
			h = f.active
		}
	}
	a := h.a
	h.do(func() { a.run(id) })
}

// beforeQuit tells whether the app may quit now. With changes to ask
// about, it asks about those of each window first, then quits again.
// Asked again meanwhile, or while a window settles its changes, it
// brings the question shown to the front.
func (m *fiveleaf) beforeQuit() bool {
	return m.prepareQuit(mygo.App.Quit)
}

func (m *fiveleaf) prepareQuit(finish func()) bool {
	switch {
	case m.life == lifeClosing:
		return true
	case m.settling():
		m.frontAsking()
		return false
	}
	for _, w := range m.wins {
		w.a.keepUntitled()
	}
	if !m.anyUnkept() {
		m.life = lifeClosing
		return true
	}
	m.life = lifeSettling
	m.settle(slices.Clone(m.wins), true, func() {
		m.life = lifeClosing
		finish()
	}, func() { m.life = lifeOpen })
	return false
}

// settling reports whether the app, or a window, is asking about
// changes before it closes.
func (m *fiveleaf) settling() bool {
	return m.life == lifeSettling || slices.ContainsFunc(m.frames, func(f *frame) bool { return f.life == lifeSettling })
}

// frontAsking brings the window whose question about changes shows to
// the front.
func (m *fiveleaf) frontAsking() {
	if m.asking != nil {
		m.asking.show()
	}
}

// anyUnkept reports whether quitting loses changes, those of documents
// other than the untitled ones kept.
func (m *fiveleaf) anyUnkept() bool {
	return slices.ContainsFunc(m.wins, func(w *desktop) bool { return w.a.anyUnkept() })
}

// settle saves the files of wins that save themselves, asks about the
// other changes of each, a window at a time, then calls done, or
// cancelled when a change was kept. As the app quits, the untitled
// documents are kept rather than asked about.
func (m *fiveleaf) settle(wins []*desktop, quit bool, done, cancelled func()) {
	if len(wins) == 0 {
		m.asking = nil
		done()
		return
	}
	w, a := wins[0], wins[0].a
	if !a.started {
		// Not shown since the app started, it has nothing open.
		m.settle(wins[1:], quit, done, cancelled)
		return
	}
	next := func() { m.settle(wins[1:], quit, done, cancelled) }
	stop := func() {
		m.asking = nil
		cancelled()
	}
	w.do(func() {
		a.updatesOpen = false
		a.autoSaveAll()
		if quit {
			a.keepUntitled()
		}
		if !a.anyDirty() || quit && !a.anyUnkept() {
			next()
			return
		}
		m.asking = w
		w.show()
		if quit {
			a.closeQuitting(next, stop)
		} else {
			a.closeKeeping(next, stop)
		}
	})
}

// openWindow opens a window for st, which restores what st has of it:
// with project tabs, a tab of the window in front, which shows it.
func (m *fiveleaf) openWindow(st *settings.Window) *desktop {
	return m.openProjectState(st, st.Detached)
}

func (m *fiveleaf) projectFrame() *frame {
	for _, h := range m.wins {
		if !h.a.window.Detached && h.f != nil {
			return h.f
		}
	}
	return nil
}

func (m *fiveleaf) openProjectState(st *settings.Window, separate bool) *desktop {
	h := &desktop{m: m}
	a := newApp(h, m.s, st)
	a.applyKeymap(m.keys)
	h.a = a
	if f := m.projectFrame(); !separate && m.s.ProjectTabs && f != nil {
		f.add(h)
		m.wins = append(m.wins, h)
		f.activate(h)
		return h
	}
	m.newFrame(h)
	m.wins = slices.Insert(m.wins, 0, h)
	m.s.Focused(st)
	return h
}

// openBlankWindow opens a window that shows nothing, which the welcome
// page fills with the recent folders and the hosts. It is the window the
// app starts with when it was given nothing to open, and the one the Dock
// icon's and the File menu's New Window open.
func (m *fiveleaf) openBlankWindow() { m.openWindow(m.s.NewWindow()) }

func (a *app) anyDirty() bool {
	for _, d := range a.docs {
		if d.dirty() {
			return true
		}
	}
	return false
}

// appLocation is the running executable on Windows, else the .app it is
// in on macOS, "" when it is in none.
func appLocation() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "windows" {
		return exe
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	if app, _, ok := strings.Cut(exe, ".app/Contents/MacOS/"); ok {
		return app + ".app"
	}
	return ""
}

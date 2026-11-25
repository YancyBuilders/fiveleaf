package main

import (
	"cmp"
	"fiveleaf/internal/agents"
	"fiveleaf/internal/keymap"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	mtransfer "github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// session is a terminal: one of the panel, or one a tab of a column
// shows.
type session struct {
	events *tabEvents
	id     int
	term   *terminal.Terminal
	title  string
	dir    string
	view   ui.Handle // its view, which takes the keyboard
	tab    *colTab   // the tab that shows it, nil for one of the panel
	pane   *terminalPane
	// lost is set while a terminal of a host waits for the connection
	// that ended it, to start again.
	lost bool
	// waitSince is when its SSH connection went away.
	waitSince time.Time
	// find is its find bar's: open, what it looks for, how the search
	// goes, as OnSearch tells, and back set when the last step went
	// toward newer output, which rolls the count down.
	findOpen  bool
	findQuery string
	found     terminal.SearchStatus
	findInput ui.Handle
	findShook time.Time
	// status is what its programs say they do (OSC 7501), statusSince
	// when the record most in need of the user last changed, and
	// statusFrom the state that record was in before.
	status      terminal.ProgramStatus
	statusSince time.Time
	statusFrom  terminal.StatusState
	// name is what the user renamed it to, "" for none, shown over the
	// title its programs give it.
	name string
	// started is when its program started, to tell a shell that exits
	// as it starts from one the user ended.
	started time.Time
	// agent is the agent the Sessions view resumed in it, or showed it
	// for, and resumed the key of the session, "" for none.
	agent   agents.Harness
	resumed string
	// sessionTitle is the title of the session resumed in it, which it is
	// called until the agent names it, "" once it did, and program the
	// program of the command that resumed it.
	sessionTitle, program string
	task                  *taskRun
}

func (s *session) tabEvents(a *app) *tabEvents {
	if s.events == nil {
		s.events = &tabEvents{window: a}
	}
	return s.events
}

func (s *session) isShell() bool { return s != nil && s.task == nil }

func (s *session) inPanel() bool { return s.isShell() && s.tab == nil }

// label is what the terminal is called: the name the user gave it, or
// the title of the session resumed in it, until the agent names it, or
// the title its programs give it.
func (s *session) label() string {
	if s.task != nil {
		return cmp.Or(s.name, s.task.resolved.Task.Label)
	}
	return cmp.Or(s.name, s.sessionTitle, s.title)
}

// retitle takes a title its programs give the terminal, which is the
// agent's own, ending the session's title, unless it is as a shell names
// a terminal: by a shell, a folder (as "~/src" or "me@host: ~/src"), or
// the command line the shell runs (as "claude --resume …").
func (s *session) retitle(title string) {
	s.title = title
	if s.sessionTitle == "" {
		return
	}
	t := strings.TrimSpace(title)
	shellish := t == "" || isShellName(t) || strings.HasPrefix(t, "~") || strings.HasPrefix(t, "/") ||
		(strings.Contains(t, "@") && strings.Contains(t, ":"))
	if !shellish && s.program != "" {
		shellish = strings.HasPrefix(t, s.program) || strings.Contains(t, s.program+" ")
	}
	if !shellish {
		s.sessionTitle = ""
	}
}

type panelHeaderLayout struct {
	windowWidth, panelWidth float32
	side, titles, font      string
}

// termTheme is the terminal's colors for a palette: its background the
// panel's, so that the panel and the editor are of a piece.
func termTheme(p *palette) *terminal.Theme {
	return &terminal.Theme{
		Foreground: p.text, Background: p.panel,
		Cursor: p.accent,
		// The matches of find as the editor's, made opaque.
		Search: p.match.Over(p.panel), SearchText: p.text,
		SearchSelected: p.matchNow.Over(p.panel), SearchSelectedText: p.text,
		Palette: p.ansi,
	}
}

// termThemes are the terminal's colors in a light window and in a dark
// one, for the theme of the settings.
func (a *app) termThemes() (light, dark *terminal.Theme) {
	th := a.settings.Theme
	return termTheme(paletteFor(th, false)), termTheme(paletteFor(th, true))
}

// themeTerminals gives the terminals the colors of the theme, once it
// changes, as they follow the window's light and dark themselves.
func (a *app) themeTerminals() {
	generation := themeFileGen()
	if a.settings.Theme == a.termTheme && generation == a.termThemeGen {
		return
	}
	a.termTheme, a.termThemeGen = a.settings.Theme, generation
	if len(a.terms) == 0 {
		return
	}
	light, dark := a.termThemes()
	for _, s := range a.terms {
		s.term.SetTheme(light, dark)
	}
}

// workDir is where a terminal starts: the workspace, or the home
// directory.
func (a *app) workDir() string {
	if a.root != "" {
		return a.root
	}
	dir, _ := os.UserHomeDir()
	return dir
}

// startTerminal starts a terminal in dir, nil when it cannot.
func (a *app) startTerminal(dir string) *session {
	s := &session{dir: dir, title: shellName()}
	if c := sessionConn(s); c != nil {
		s.title = c.Host
	}
	return a.addTerminal(s)
}

// addTerminal starts the program of a terminal, nil when it cannot.
func (a *app) addTerminal(s *session) *session {
	a.nextTerm++
	s.id = a.nextTerm
	term, err := a.spawnTerminal(s, s.dir)
	if err != nil {
		if s.task != nil {
			a.fail("%s", locales.FailStartTask(s.task.resolved.Task.Label, err.Error()))
		} else {
			a.fail("%s", locales.FailStartTerminal(err.Error()))
		}
		return nil
	}
	s.term, s.started = term, time.Now()
	a.terms = append(a.terms, s)
	return s
}

// spawnTerminal starts a task, or the shell in dir, on the host in a
// remote folder.
func (a *app) spawnTerminal(s *session, dir string) (*terminal.Terminal, error) {
	events := s.tabEvents(a)
	light, dark := a.termThemes()
	var command, env []string
	if s.task != nil {
		var err error
		command, err = taskProgram(s.task.resolved)
		if err != nil {
			return nil, err
		}
		env = s.task.resolved.Env
	} else if c, rdir, ok := remote.Lookup(dir); ok {
		// In a remote folder, a shell of its host, over the connection.
		command = c.TerminalCommand(rdir)
		dir, _ = os.UserHomeDir()
	}
	if !remote.Is(s.dir) {
		env = localTerminalEnv(env)
	}
	// term is the terminal made, which OnExit tells from one that took
	// its place, and in what its programs last said that waits for the
	// app.
	var term *terminal.Terminal
	var in statusIn
	term, err := terminal.New(terminal.Options{
		Transparent: true,
		Command:     command,
		Env:         env,
		Dir:         dir,
		Font:        termFont(a.settings),
		Theme:       light,
		DarkTheme:   dark,
		Cursor:      terminal.CursorBar,
		// Selecting copies, as in Ghostty and iTerm2.
		CopyOnSelect: copyOnSelect(a.prefs().TerminalCopyOnSelect),
		OnTitle: func(title string) {
			events.post(func(*app) {
				if s.term == term {
					s.retitle(title)
				}
			})
		},
		// A terminal of a host gets files and images pasted as copies
		// uploaded to it.
		OnPaste: func(d mtransfer.Data) bool {
			text, _ := d.Read(mtransfer.Text)
			return events.window.terminalPaste(s, string(text))
		},
		// Local file paths open in the editor; links to a port of a
		// remote host open the port forwarded to it here.
		OpenLink: func(link string) bool { return events.window.terminalLink(s, link) },
		// The shortcuts of keybindings.json, before the shell, and on
		// Windows and Linux those of FiveLeaf's it leaves to FiveLeaf.
		OnKey: func(mods ui.Modifiers, key ui.Key) bool {
			a := events.window
			if s.isShell() {
				a.keyTerm = s
			}
			st := keymap.Stroke{Mods: mods, Key: key}
			if a.keyStroke(st, placeTerminal) {
				return true
			}
			id, ok := a.shellSkip(st)
			if !ok {
				a.seeProgramStatus(s)
			}
			if ok {
				a.post(func() {
					if id == "" {
						a.showTabKey(key)
					} else {
						a.run(id)
					}
				})
			}
			return ok
		},
		// CmdOrCtrl+= and CmdOrCtrl+- scale the whole window: Command on
		// macOS, Control on Windows/Linux. Ignore terminal-local zoom so
		// scrolling with the modifier held cannot change its font size.
		OnZoom: func(terminal.Zoom) bool { return true },
		OnSearch: func(st terminal.SearchStatus) {
			events.post(func(*app) {
				if s.term == term {
					s.found = st
				}
			})
		},
		OnExit: func(code int) {
			events.post(func(a *app) { a.terminalExited(s, term, code) })
		},
		// A program done in a project out of sight, as a build or an
		// agent, has the rail mark the project's square.
		OnBell: func() {
			events.post(func(a *app) {
				if s.term == term {
					a.alert, a.alertState = locales.BellFrom(s.label()), terminal.StatusIdle
				}
			})
		},
		OnNotify: func(title, body string) {
			events.post(func(a *app) {
				if s.term != term {
					return
				}
				a.alert, a.alertState = strings.TrimSpace(title+" "+body), terminal.StatusIdle
				if a.alert == "" {
					a.alert = locales.NotificationFrom(s.label())
				}
			})
		},
		OnProgramStatus: func(st terminal.ProgramStatus) {
			// Once a frame however often the program says it.
			if in.put(st) {
				events.post(func(a *app) {
					if st, ok := in.take(); ok {
						if s.task == nil || !s.task.completed {
							a.programStatus(s, term, st)
						}
					}
				})
			}
		},
	})
	return term, err
}

// newTerminal starts a terminal in the panel and shows it.
func (a *app) newTerminal() { a.newTerminalIn(a.workDir()) }

// newTerminalIn starts a terminal of the panel in dir and shows it.
func (a *app) newTerminalIn(dir string) {
	if s := a.startTerminal(dir); s != nil {
		a.terminalToPanel(s)
	}
}

func (a *app) newTerminalsInEditor() bool {
	for _, s := range a.terms {
		if s.isShell() {
			return s.tab != nil
		}
	}
	return a.settings.TerminalLocation == "editor"
}

// openTerminal starts a terminal beside the window's other terminals.
func (a *app) openTerminal(dir string) {
	if a.newTerminalsInEditor() {
		a.newTerminalTab(a.fileColumn(), dir, true)
		return
	}
	a.newTerminalIn(dir)
}

// newTerminalTab starts a terminal in dir in a new tab of a column, after
// the one shown, and reports whether it did.
func (a *app) newTerminalTab(col *column, dir string, focus bool) bool {
	return a.terminalTab(col, col.active+1, a.startTerminal(dir), focus)
}

func (a *app) terminalTab(col *column, at int, s *session, focus bool) bool {
	if s == nil || !slices.Contains(a.terms, s) || !slices.Contains(a.cols, col) {
		return false
	}
	if s.isShell() {
		a.movePanelTerminals(col, at)
	} else if s.tab == nil {
		s.tab = &colTab{term: s}
		a.addTab(col, at, s.tab)
	}
	owner, index := a.locate(s.tab)
	a.activate(owner, index, focus)
	return true
}

func (a *app) movePanelTerminals(col *column, at int) int {
	at = min(max(at, 0), len(col.tabs))
	for _, s := range a.terms {
		if s.inPanel() {
			s.tab = &colTab{term: s}
			a.addTab(col, at, s.tab)
			at++
		}
	}
	a.panelTerm = nil
	a.window.PanelOpen, a.window.TerminalOpen = false, false
	return at
}

// lookPath finds a program on the path, as exec.LookPath, which tests
// replace.
var lookPath = exec.LookPath

// shellName is the name of the shell a terminal runs: $SHELL's, and on
// Windows, which has none, the first of those the terminal looks for that
// is there: PowerShell's, then the command prompt.
func shellName() string {
	if hostOS == "windows" {
		for _, name := range []string{"pwsh.exe", "powershell.exe", os.Getenv("COMSPEC"), "cmd.exe"} {
			if _, err := lookPath(name); name != "" && err == nil {
				return strings.TrimSuffix(strings.ToLower(filepath.Base(name)), ".exe")
			}
		}
		return "shell"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return filepath.Base(sh)
	}
	return "shell"
}

// shellNames are the shells a terminal may run besides the user's own.
var shellNames = []string{"sh", "bash", "zsh", "fish", "dash", "ksh", "tcsh", "csh", "nu", "pwsh", "powershell", "cmd"}

// isShellName reports whether a title is a shell's name, as "zsh" or
// "pwsh.exe".
func isShellName(title string) bool {
	name := strings.TrimSuffix(strings.ToLower(title), ".exe")
	return name == shellName() || slices.Contains(shellNames, name)
}

// panelTerms returns the terminals of the panel.
func (a *app) panelTerms() []*session {
	var out []*session
	for _, s := range a.terms {
		if s.inPanel() {
			out = append(out, s)
		}
	}
	return out
}

// removeTerminal drops a terminal whose program ended, or that the user
// killed: its tab closes, or the panel hides after its last.
func (a *app) removeTerminal(s *session) {
	i := slices.Index(a.terms, s)
	if i < 0 {
		return
	}
	if a.keyTerm == s {
		a.keyTerm = nil
	}
	a.terms = slices.Delete(a.terms, i, i+1)
	neighbor := s.leavePanelGroup()
	s.term.Close()
	if t := s.tab; t != nil {
		active := a.activeTab() == t && (s.isShell() || a.focus == paneEditor)
		s.tab = nil
		a.dropTab(t)
		if active {
			a.focusColumn()
		}
	} else {
		panel := a.panelTerms()
		a.window.TerminalOpen = len(panel) > 0
		switch {
		case len(panel) == 0:
			a.panelTerm = nil
			if a.window.PanelOpen {
				a.hidePanel()
			}
		case a.panelTerm == s:
			next := slices.IndexFunc(a.terms[i:], (*session).inPanel)
			switch {
			case neighbor != nil:
				a.panelTerm = neighbor
			case next >= 0:
				a.panelTerm = a.terms[i+next]
			default:
				a.panelTerm = panel[len(panel)-1]
			}
			if a.window.PanelOpen && a.focus == paneTerminal {
				a.focusTerminal()
			}
		}
	}
	a.rememberTabs()
}

// terminalToColumn moves a terminal to a tab at place at of a column,
// from the panel or from another tab.
func (a *app) terminalToColumn(s *session, col *column, at int) {
	if s.tab != nil {
		a.moveTab(s.tab, col, at)
		return
	}
	a.terminalTab(col, at, s, true)
}

func (a *app) maximizePanel() {
	if a.panelTerm == nil || !a.window.PanelOpen {
		return
	}
	col := a.column()
	a.terminalToColumn(a.panelTerm, col, col.active+1)
}

// terminalToPanel docks the window's terminals and selects s.
func (a *app) terminalToPanel(s *session) {
	if a.window.Detached || !s.isShell() || !slices.Contains(a.terms, s) {
		return
	}
	for _, other := range a.terms {
		if tab := other.tab; other.isShell() && tab != nil {
			col, index := a.locate(tab)
			if col.active == index && index > 0 {
				col.active--
			}
			other.tab = nil
			a.dropTab(tab)
		}
	}
	a.panelTerm = s
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	a.rememberTabs()
	a.focusTerminal()
}

// focusTerminal gives the keyboard to the terminal the panel shows.
func (a *app) focusTerminal() {
	if s := a.panelTerm; s != nil {
		a.cancelPendingTabFocus()
		s.view.Focus()
	}
	a.focus = paneTerminal
}

// closeTerminals hangs up every terminal, as the window closes.
func (a *app) closeTerminals() {
	for _, s := range a.terms {
		s.term.Close()
	}
	a.terms, a.panelTerm = nil, nil
}

// hidePanel hides the panel, and gives the keyboard back to the column.
func (a *app) hidePanel() {
	a.window.PanelOpen = false
	a.settings.Changed()
	a.focusColumn()
}

// panelMenu is the menu of the panel's header: where the panel sits,
// and its size.
func (a *app) panelMenu(m *ui.Menu) {
	m.Submenu(locales.MenuPanelPosition(), func(m *ui.Menu) {
		for _, pos := range []struct{ label, id, side string }{
			{locales.PanelBottom(), "", "bottom"},
			{locales.PanelLeft(), "left", "left"},
			{locales.PanelRight(), "right", "right"},
		} {
			if m.Item(pos.label).Checked(a.panelSide() == pos.side).Chosen() {
				a.post(func() { a.setPanelPosition(pos.id) })
			}
		}
	})
	m.Separator()
	if m.Item(locales.PanelMaximize()).Disabled(a.panelTerm == nil).Chosen() {
		a.post(a.maximizePanel)
	}
	if m.Item(locales.PanelHide()).Chosen() {
		a.post(a.hidePanel)
	}
}

// panel is the terminal panel: a header of its terminals and its tools,
// and the terminal shown. A terminal dragged from a tab is dropped on it,
// and a file dragged from the sidebar types its path.
func (a *app) panel(c *ui.Context) {
	p := a.pal
	m := p.metricsOr()
	terms := a.panelTerms()
	// MyGo's scroll bar takes the thumb's width plus six DIPs for its
	// track. Keep that whole hit area below overflowing chips; a strip
	// that fits needs no extra space.
	scrollbarH := float32(0)
	if a.panelScroll.MaxX > 0.5 {
		scrollbarH = c.Theme().ScrollbarWidth + 6
	}
	box := ui.Column(c).Fill().Background(windowContainer(c, a.settings.WindowOpacity, p.panel))
	if it, ok := ui.Drop[dragItem](box); ok {
		switch s := a.panelTerm; {
		case it.tab != nil && it.tab.term.isShell() && a.owns(it):
			a.post(func() { a.terminalToPanel(it.tab.term) })
		case it.file != nil && s != nil:
			// A file of the sidebar types its path, as one from the Finder.
			a.post(func() { a.pastePaths(s, it.file.paths()) })
		}
	}
	over, _ := a.dragOver(box)
	// A panel at a side, dragged narrow, leaves out its title so that
	// its terminals and its tools keep their room.
	width := box.Bounds().W
	var titles strings.Builder
	for _, s := range terms {
		titles.WriteString(s.label())
		titles.WriteByte(0)
	}
	windowWidth, _ := c.Size()
	layout := panelHeaderLayout{windowWidth, width, a.panelSide(), titles.String(), c.Theme().Font}
	if layout != a.panelLayout {
		a.panelLayout = layout
		c.AnimationFrame()
	}
	titled := a.panelSide() == "bottom" || width == 0 || width >= panelTitleW
	box.Children(func() {
		head := ui.Row(c).Height(panelHeadH+scrollbarH).Padding(0, 8, 0, 14).Gap(m.gap).AlignItems(ui.Center).
			Background(windowBackground(c, a.settings.WindowOpacity, p.panel))
		head.ContextMenu(a.panelMenu)
		head.Children(func() {
			if titled {
				ui.Column(c).Height(panelHeadH).Margin(0, 0, scrollbarH, 0).Justify(ui.Center).Children(func() {
					size, spacing := p.titleLook(m.textSmall-0.5, 0.7)
					ui.Text(c, p.title(locales.TabTerminal())).FontSize(size).FontWeight(m.weightBold).LetterSpacing(spacing).TextColor(p.text)
					ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(1.5).Radius(1).Background(p.accent)
				})
			}
			// The terminals, as chips: a click shows one, and dragging one
			// to a column makes a tab of it.
			// One terminal shows of the panel's: chips in a radio group, as
			// MyGo's Segmented reads, so that assistive technology sees the
			// choice, and the chosen chip carries the role.
			ui.ScrollHorizontal(c.Key("terms")).TrackScroll(&a.panelScroll).Grow(1).Height(panelHeadH+scrollbarH).Padding(0, 0, scrollbarH, 0).Children(func() {
				ui.Row(c).Gap(2).AlignItems(ui.Center).Role(ui.RoleRadioGroup).Children(func() {
					for i, s := range terms {
						on := s == a.panelTerm
						chip := ui.ButtonBase(c.Key(s.id)).Padding(3, 8).Gap(6).Radius(m.radius).AlignItems(ui.Center).Label(s.label()).
							Role(ui.RoleRadio).Selected(on).Drag(dragItem{term: s}).Transition(tabMotion)
						if chip.Dragging() {
							a.dragNow = true
						}
						under := p.panel
						if on {
							chip.Background(p.active)
							under = p.active.Over(p.panel)
						} else if chip.Hovered() {
							chip.Background(p.hover)
							under = p.hover.Over(p.panel)
						}
						chip.Children(func() {
							a.termIcon(c, s, 12, p.muted, under)
							col := p.muted
							if on {
								col = p.text
							}
							ui.Text(c, fmt.Sprintf("%d: %s", i+1, s.label())).FontSize(m.textBody).TextColor(col).SingleLine().MaxWidth(180)
						})
						if chip.Clicked() {
							a.panelTerm = s
							a.focusTerminal()
						}
						chip.ContextMenu(func(m *ui.Menu) {
							if m.Item(locales.TermMoveLockedRight()).Chosen() {
								a.post(func() { a.terminalToLockedRight(s) })
							}
							if m.Item(locales.TermMoveEditorArea()).Chosen() {
								a.post(func() { a.terminalToColumn(s, a.column(), a.column().active+1) })
							}
							if m.Item(locales.TermRename()).Chosen() {
								a.post(func() { a.askRenameTerminal(s) })
							}
							if m.Item(locales.TermClose()).Chosen() {
								a.post(func() { a.removeTerminal(s) })
							}
						})
					}
				})
			})
			ui.Row(c).Gap(2).AlignItems(ui.Center).Margin(0, 0, scrollbarH, 0).Children(func() {
				source := a.panelTerm
				if a.iconButton(c, iconPlus, a.withKey(locales.CmdViewNewTerminal(), "view.newTerminal"), false).KeepFocus().Clicked() {
					if source != nil {
						a.newTerminalIn(source.nextShellDir())
					} else {
						a.newTerminal()
					}
				}
				a.splitButton(c, nil, locales.CmdViewSplitRight(), source != nil, func(direction splitDirection) {
					a.splitPanelTerminal(source, direction)
				})
				if a.iconButton(c, iconMaximize, locales.PanelMaximize(), false).Disabled(a.panelTerm == nil).Clicked() {
					a.post(a.maximizePanel)
				}
				if s := a.panelTerm; a.iconButton(c, iconX, locales.TermClose(), false).Disabled(s == nil).Clicked() {
					a.post(func() { a.removeTerminal(s) })
				}
			})
		})
		body := ui.Box(c).Grow(1).MinHeight(0)
		body.Children(func() {
			s := a.panelTerm
			if s == nil {
				ui.Column(c).Fill().Background(windowBackground(c, a.settings.WindowOpacity, p.panel)).Center().Children(func() {
					if ui.Button(c, locales.CmdViewNewTerminal()).Clicked() {
						a.newTerminal()
					}
				})
			} else {
				a.panelTerminalViews(c, s, body.Bounds())
			}
			if over.tab != nil && over.tab.term.isShell() {
				a.dropMark(c)
			}
		})
	})
}

// terminalView shows a terminal, in the panel or in a tab: faded while it
// waits for its SSH connection, as what it shows may be old.
func (a *app) terminalView(c *ui.Context, s *session) {
	ui.Box(c).Fill().Padding(2, 4, 4, 14).Background(windowBackground(c, a.settings.WindowOpacity, a.pal.panel)).Children(func() {
		v := terminal.View(c.Key(s.id), s.term).Fill().Bind(&s.view)
		shown := float32(1)
		if s.lost {
			shown = 0.55
		}
		if o := v.Animate("shown", shown, motionMove); o < 1 {
			v.Opacity(o)
		}
		a.waitPill(c, s)
		a.termFindView(c, s)
		if s.isShell() && s.tab != nil && !a.window.Detached {
			m := a.pal.metricsOr()
			box := ui.Box(c.Key("restore-panel")).Absolute().Right(14).Bottom(12).Radius(m.radius + 1)
			a.floating(c, box, false).Children(func() {
				if a.iconButton(c, iconMinimize, locales.PanelRestore(), false).Size(32, 32).KeepFocus().Clicked() {
					a.post(func() { a.terminalToPanel(s) })
				}
			})
		}
	})
}

// termFont is the terminals' font: the settings' own for the terminal,
// else the code's, a little smaller.
func termFont(s *settings.Settings) terminal.Font {
	f := terminal.Font{Family: s.TerminalFontFamily, Size: s.TerminalFontSize, Weight: s.FontWeight, Features: s.FontFeatures}
	if strings.TrimSpace(f.Family) == "" {
		f.Family = s.FontFamily
	}
	f.Family = terminalFontFamily(f.Family)
	if f.Size == 0 {
		f.Size = max(s.FontSize-0.5, 9)
	}
	return f
}

// fontTerminals gives the terminals the font of the settings, once it
// changes.
func (a *app) fontTerminals() {
	f := termFont(a.settings)
	if sameTermFont(f, a.termFont) {
		return
	}
	a.termFont = f
	for _, s := range a.terms {
		s.term.SetFont(f)
	}
}

// copyTerminals has the terminals copy what is selected, or not, as the
// settings say, once they change.
func (a *app) copyTerminals() {
	on := a.prefs().TerminalCopyOnSelect
	if on == a.termCopy {
		return
	}
	a.termCopy = on
	for _, s := range a.terms {
		s.term.SetCopyOnSelect(copyOnSelect(on))
	}
}

// copyOnSelect is where the terminal copies a selection when the setting
// is on or off.
func copyOnSelect(on bool) terminal.CopyOnSelect {
	if on {
		return terminal.CopyOnSelectClipboard
	}
	return terminal.CopyOnSelectNone
}

func sameTermFont(a, b terminal.Font) bool {
	return a.Family == b.Family && a.Size == b.Size && a.Weight == b.Weight && slices.Equal(a.Features, b.Features)
}

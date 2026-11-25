package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/smallnest/uix/components/button"

	"fiveleaf/internal/dap"
	"fiveleaf/internal/plugin"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/vfs"
	"fiveleaf/locales"
)

// breakpoint is a line of a file where a debugger stops, counted from 0
// as the editor counts.
type breakpoint struct {
	line int
	// verified is what the adapter said of it, once it was told.
	verified bool
	message  string
}

type fileBreakpoint struct {
	path string
	breakpoint
}

// debugState is the app's debugger: its session, the breakpoints, and
// what the Debug view shows of them.
type debugState struct {
	app      *app
	mu       sync.Mutex
	session  *debugSession
	target   *debugSession
	sessions []*debugSession
	status   string
	stopped  *stoppedEvent
	threads  []dapThread
	frames   []dapFrame
	scopes   []dapScope
	vars     []dapVariable
	output   string
	thread   *int
	frame    int

	// breakpoints are by path, latest first, for the Debug view.
	breakpoints map[string][]breakpoint
	// closed are the sections of the view closed, by their key.
	closed map[string]bool
	// scrolled is the Debug view's list.
	scroll ui.ScrollState
}

type stoppedEvent struct {
	ThreadID *int   `json:"threadId"`
	Reason   string `json:"reason"`
	Text     string `json:"text"`
}

type dapThread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type dapFrame struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Source struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"source"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type dapScope struct {
	Name               string `json:"name"`
	VariablesReference int    `json:"variablesReference"`
	Expensive          bool   `json:"expensive"`
}

type dapVariable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type"`
	VariablesReference int    `json:"variablesReference"`
}

// debugSession is one adapter, launched for one file.
type debugSession struct {
	ctx         context.Context
	client      *dap.Client
	cancel      context.CancelFunc
	root        string
	adapterRoot string
	pending     map[string]bool
	changes     chan struct{}
	parent      *debugSession
	stopped     *stoppedEvent
	ready       bool
	confirmed   map[string]map[int]dapBreakpoint
}

// at is the breakpoint store's view of a path.
func (d *debugState) at(path string) []breakpoint {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.breakpointsForDisplay(path)
}

func (d *debugState) breakpointsForDisplay(path string) []breakpoint {
	list := slices.Clone(d.breakpoints[path])
	if d.target != nil {
		for index, bp := range list {
			state := d.target.confirmed[path][bp.line]
			list[index].verified, list[index].message = state.Verified, state.Message
		}
	}
	return list
}

// toggle turns the breakpoint at a line of a path on or off, and returns
// whether one is there now.
func (d *debugState) toggle(path string, line int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.breakpoints == nil {
		d.breakpoints = map[string][]breakpoint{}
	}
	list := d.breakpoints[path]
	for i, bp := range list {
		if bp.line == line {
			d.breakpoints[path] = append(list[:i], list[i+1:]...)
			if len(d.breakpoints[path]) == 0 {
				delete(d.breakpoints, path)
			}
			return false
		}
	}
	d.breakpoints[path] = append(list, breakpoint{line: line})
	sort.Slice(d.breakpoints[path], func(i, j int) bool { return d.breakpoints[path][i].line < d.breakpoints[path][j].line })
	return true
}

// hasBreakpoint reports whether path stops at line.
func (d *debugState) hasBreakpoint(path string, line int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, bp := range d.breakpoints[path] {
		if bp.line == line {
			return true
		}
	}
	return false
}

// pausedLine is the line of path the session stopped at, -1 for none.
func (d *debugState) pausedLine(path string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session == nil || d.stopped == nil || len(d.frames) == 0 {
		return -1
	}
	top := d.frames[0]
	for _, frame := range d.frames {
		if frame.ID == d.frame {
			top = frame
			break
		}
	}
	if top.Source.Path != "" && filepath.Clean(top.Source.Path) == filepath.Clean(path) {
		return top.Line - 1
	}
	return -1
}

// active reports whether an adapter is running.
func (d *debugState) active() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.session != nil
}

// statusText is what the view and the status bar say of the session.
func (d *debugState) statusText() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

// showDebug opens the Debug view in the sidebar.
func (a *app) showDebug() {
	a.window.SidebarClosed, a.window.SidebarView = false, "debug"
	a.settings.Changed()
}

type debugAdapterChoice struct {
	provider *plugin.Plugin
	adapter  *plugin.DebugAdapterContrib
}

// startDebug launches the adapter of the active file's language.
func (a *app) startDebug() {
	if a.debug.active() {
		a.debugContinue()
		return
	}
	d := a.activeEditor()
	if d == nil || d.path == "" {
		a.fail("%s", locales.DebugNeedsFile())
		return
	}
	file, language := d.path, languageOf(d.document)
	var choices []debugAdapterChoice
	for _, provider := range plugins.enabled() {
		for index := range provider.Contributes.DebugAdapters {
			adapter := &provider.Contributes.DebugAdapters[index]
			if slices.Contains(adapter.Languages, language) {
				choices = append(choices, debugAdapterChoice{provider, adapter})
			}
		}
	}
	switch len(choices) {
	case 0:
		a.fail("%s", locales.DebugNoAdapter(language))
	case 1:
		a.startDebugAdapter(choices[0], file)
	default:
		items := make([]pickItem, len(choices))
		for index, choice := range choices {
			items[index] = pickItem{Label: cmp.Or(choice.adapter.Name, choice.provider.Title()), Description: choice.adapter.ID}
		}
		a.openPluginPicker(items, locales.PluginsDebugAdapters(), func(index int) {
			if index >= 0 && !a.debug.active() && slices.Contains(plugins.enabled(), choices[index].provider) {
				a.startDebugAdapter(choices[index], file)
			}
		})
	}
}

func (a *app) startDebugAdapter(choice debugAdapterChoice, file string) {
	root := cmp.Or(a.root, filepath.Dir(file))
	if remote.HostOf(root) != remote.HostOf(file) {
		root = filepath.Dir(file)
	}
	settings := plugins.commandSettings(choice.provider)
	debug := &a.debug
	debug.mu.Lock()
	session := debug.newSession(nil, root)
	debug.session, debug.target, debug.status, debug.output = session, session, locales.DebugStarting(), ""
	debug.mu.Unlock()
	a.showDebug()
	a.invalidate()
	go func() {
		err := a.runDebugSession(session, choice, file, settings)
		a.post(func() { a.endDebugTarget(session, err) })
	}()
}

func (a *app) runDebugSession(session *debugSession, choice debugAdapterChoice, file string, settings pluginCommandSettings) error {
	startup, cancel := context.WithTimeout(session.ctx, 30*time.Second)
	defer cancel()
	adapterRoot := session.root
	if !remote.Is(session.root) {
		if resolved, err := filepath.EvalSymlinks(session.root); err == nil {
			adapterRoot = resolved
		}
	}
	a.debug.mu.Lock()
	session.adapterRoot = adapterRoot
	a.debug.mu.Unlock()
	workspace := debugWorkspace{root: adapterRoot, file: debugRebasePath(file, session.root, adapterRoot), provider: choice.provider, settings: settings}
	launch, err := workspace.resolve(startup, choice.adapter)
	if err != nil {
		return err
	}
	options := dap.Options{AdapterID: cmp.Or(choice.adapter.AdapterID, choice.adapter.ID), InitializationOptions: launch.Options,
		Handlers: dap.Handlers{Log: choice.provider.Log}}
	var input io.Reader
	connection, _, isRemote := remote.Lookup(session.root)
	if launch.Transport != nil && launch.Transport.Kind == "tcp" {
		options.Address, options.Ready, options.Host = launch.Transport.Address, launch.Transport.Ready, "127.0.0.1"
		if isRemote {
			options.DialContext = connection.DialContext
		}
	} else if isRemote {
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		input, options.Stdin = reader, writer
	}
	if launch.Command != "" {
		options.Cmd, err = vfs.Command(session.root, envList(launch.Env), input, launch.Command, launch.Args...)
		if err != nil {
			return err
		}
	}
	return a.runDebugTarget(startup, session, options, launch.Launch["request"].(string), launch.Launch)
}

func (a *app) runDebugTarget(startup context.Context, session *debugSession, options dap.Options, request string, launch map[string]any) error {
	options.Handlers.Event = func(event dap.Event) { a.debugEvent(session, event) }
	if options.Ready != "" || options.Address != "" {
		options.Handlers.StartDebugging = func(childRequest string, config map[string]any) error {
			return a.startChildDebugger(session, options, childRequest, config)
		}
	}
	client, err := dap.Start(startup, options)
	if err != nil {
		return err
	}
	defer client.Close()
	debug := &a.debug
	debug.mu.Lock()
	if !slices.Contains(debug.sessions, session) || session.ctx.Err() != nil {
		debug.mu.Unlock()
		return context.Canceled
	}
	session.client = client
	debug.mu.Unlock()
	if err := client.Launch(startup, request, launch, func(ctx context.Context) error {
		for {
			if err := debug.sendBreakpoints(ctx, session); err != nil {
				return err
			}
			debug.mu.Lock()
			pending := len(session.pending)
			debug.mu.Unlock()
			if pending == 0 {
				return nil
			}
		}
	}); err != nil {
		return err
	}
	a.post(func() {
		if !debug.owns(session) {
			return
		}
		session.ready = true
		if debug.stopped == nil && (debug.target == session || debug.target == session.parent) {
			debug.focusTarget(session)
		}
		a.invalidate()
	})
	for {
		select {
		case <-session.ctx.Done():
			return nil
		case <-client.Done():
			return client.Err()
		case <-session.changes:
			ctx, cancel := context.WithTimeout(session.ctx, 10*time.Second)
			err := debug.sendBreakpoints(ctx, session)
			cancel()
			if err != nil {
				a.post(func() {
					if debug.owns(session) {
						a.fail("%s", locales.DebugCommandFailed("setBreakpoints", err.Error()))
					}
				})
			}
		}
	}
}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

// stop ends the session, and the process it started.
func (d *debugState) stop() {
	d.mu.Lock()
	s := d.session
	d.session, d.status, d.stopped = nil, locales.DebugStopped(), nil
	d.target, d.sessions = nil, nil
	d.threads, d.frames, d.scopes, d.vars = nil, nil, nil, nil
	d.thread, d.frame = nil, 0
	d.mu.Unlock()
	if s != nil {
		s.cancel()
	}
}

func (d *debugState) owns(session *debugSession) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Contains(d.sessions, session) && session.ctx.Err() == nil
}

func (d *debugState) sessionNow() *debugSession {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.target == nil || d.target.client == nil {
		return nil
	}
	return d.target
}

// debugEvent takes an event of the adapter.
func (a *app) debugEvent(session *debugSession, event dap.Event) {
	debug := &a.debug
	a.post(func() {
		if !debug.owns(session) {
			return
		}
		switch event.Event {
		case "stopped":
			var stopped stoppedEvent
			if json.Unmarshal(event.Body, &stopped) != nil {
				return
			}
			session.stopped = &stopped
			debug.focusTarget(session)
		case "continued":
			debug.continued(session)
		case "output":
			var body struct{ Output string }
			if json.Unmarshal(event.Body, &body) == nil {
				debug.mu.Lock()
				debug.output += body.Output
				debug.mu.Unlock()
			}
		case "breakpoint":
			var body struct {
				Reason     string
				Breakpoint dapBreakpoint
			}
			if json.Unmarshal(event.Body, &body) == nil && body.Breakpoint.ID != nil {
				debug.mu.Lock()
				for _, list := range session.confirmed {
					for line, bp := range list {
						if bp.ID != nil && *bp.ID == *body.Breakpoint.ID {
							body.Breakpoint.Verified = body.Reason != "removed" && body.Breakpoint.Verified
							list[line] = body.Breakpoint
						}
					}
				}
				debug.mu.Unlock()
			}
		case "terminated":
			a.endDebugTarget(session, nil)
		default:
			return
		}
		a.invalidate()
	})
}

// call runs a request of the adapter, and calls fn on the main thread
// with its body.
func (d *debugState) call(a *app, command string, args any, fn func(json.RawMessage)) {
	s := d.sessionNow()
	if s == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
		defer cancel()
		body, err := s.client.Call(ctx, command, args)
		a.post(func() {
			if d.sessionNow() != s {
				return
			}
			if err != nil {
				a.fail("%s", locales.DebugCommandFailed(command, err.Error()))
			} else if fn != nil {
				fn(body)
			}
			a.invalidate()
		})
	}()
}

func (d *debugState) refreshThreads() {
	stopped := d.stopped
	d.call(d.app, "threads", nil, func(body json.RawMessage) {
		var out struct {
			Threads []dapThread `json:"threads"`
		}
		if d.stopped != stopped || json.Unmarshal(body, &out) != nil {
			return
		}
		d.mu.Lock()
		d.threads = out.Threads
		needFrame := d.stopped != nil && d.thread == nil && len(out.Threads) > 0
		if needFrame {
			d.thread = &out.Threads[0].ID
		}
		d.mu.Unlock()
		if needFrame {
			d.refreshFrames()
		}
	})
}

func (d *debugState) refreshFrames() {
	s := d.sessionNow()
	if s == nil {
		return
	}
	d.mu.Lock()
	thread, stopped := d.thread, d.stopped
	d.mu.Unlock()
	if thread == nil {
		return
	}
	d.call(d.app, "stackTrace", map[string]any{"threadId": *thread, "startFrame": 0, "levels": 30}, func(body json.RawMessage) {
		var out struct {
			StackFrames []dapFrame `json:"stackFrames"`
		}
		if d.stopped != stopped || json.Unmarshal(body, &out) != nil {
			return
		}
		for index := range out.StackFrames {
			frame := &out.StackFrames[index]
			if frame.Source.Path != "" {
				frame.Source.Path = remote.On(s.root, debugRebasePath(frame.Source.Path, s.adapterRoot, s.root))
			}
		}
		d.mu.Lock()
		d.frames = out.StackFrames
		if len(out.StackFrames) > 0 {
			d.frame = out.StackFrames[0].ID
		}
		d.mu.Unlock()
		if len(out.StackFrames) > 0 {
			d.openFrame(out.StackFrames[0])
		}
	})
}

// openFrame shows a frame in the editor, and its scopes.
func (d *debugState) openFrame(frame dapFrame) {
	s := d.sessionNow()
	if s == nil || d.stopped == nil {
		return
	}
	if frame.Source.Path != "" {
		d.app.openFileAt(frame.Source.Path, max(frame.Line-1, 0), true)
	}
	d.mu.Lock()
	stopped := d.stopped
	d.frame, d.scopes, d.vars = frame.ID, nil, nil
	d.mu.Unlock()
	d.call(d.app, "scopes", map[string]any{"frameId": frame.ID}, func(body json.RawMessage) {
		var out struct {
			Scopes []dapScope `json:"scopes"`
		}
		if d.stopped != stopped || d.frame != frame.ID || json.Unmarshal(body, &out) != nil {
			return
		}
		d.mu.Lock()
		d.scopes = out.Scopes
		d.mu.Unlock()
		if len(out.Scopes) > 0 {
			d.refreshVariables(out.Scopes[0].VariablesReference)
		}
	})
}

func (d *debugState) refreshVariables(ref int) {
	s := d.sessionNow()
	if s == nil || ref == 0 {
		return
	}
	frame, stopped := d.frame, d.stopped
	d.call(d.app, "variables", map[string]any{"variablesReference": ref}, func(body json.RawMessage) {
		var out struct {
			Variables []dapVariable `json:"variables"`
		}
		if d.frame != frame || d.stopped != stopped || json.Unmarshal(body, &out) != nil {
			return
		}
		d.mu.Lock()
		d.vars = out.Variables
		d.mu.Unlock()
	})
}

func (d *debugState) queueBreakpoints(path string, line int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, session := range d.sessions {
		if remote.HostOf(path) != remote.HostOf(session.root) {
			continue
		}
		session.pending[path] = true
		delete(session.confirmed[path], line)
		select {
		case session.changes <- struct{}{}:
		default:
		}
	}
}

type dapBreakpoint struct {
	ID       *int   `json:"id"`
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

func (d *debugState) sendBreakpoints(ctx context.Context, session *debugSession) error {
	d.mu.Lock()
	pending := make(map[string][]breakpoint, len(session.pending))
	for path := range session.pending {
		pending[path] = slices.Clone(d.breakpoints[path])
	}
	clear(session.pending)
	d.mu.Unlock()
	var failures []error
	for _, path := range slices.Sorted(maps.Keys(pending)) {
		list := pending[path]
		requested := make([]map[string]int, len(list))
		for index, bp := range list {
			requested[index] = map[string]int{"line": bp.line + 1}
		}
		args := map[string]any{"source": map[string]string{"path": hostPath(debugRebasePath(path, session.root, session.adapterRoot))}, "breakpoints": requested}
		body, err := session.client.Call(ctx, "setBreakpoints", args)
		var response struct{ Breakpoints []dapBreakpoint }
		if err == nil {
			err = json.Unmarshal(body, &response)
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		d.mu.Lock()
		if !session.pending[path] {
			results := make(map[int]dapBreakpoint, len(list))
			for index := range min(len(response.Breakpoints), len(list)) {
				results[list[index].line] = response.Breakpoints[index]
			}
			if len(results) == 0 {
				delete(session.confirmed, path)
			} else {
				session.confirmed[path] = results
			}
		}
		d.mu.Unlock()
	}
	if len(pending) > 0 {
		d.app.invalidate()
	}
	return errors.Join(failures...)
}

// toggleBreakpoint turns the breakpoint at the caret on or off.
func (a *app) toggleBreakpoint() {
	d := a.activeEditor()
	if d == nil || d.path == "" {
		return
	}
	line := d.buf.Snapshot().LineAt(d.sel[1])
	a.toggleBreakpointAt(d.path, line)
}

// toggleBreakpointAt is the gutter's way in.
func (a *app) toggleBreakpointAt(path string, line int) {
	a.debug.toggle(path, line)
	a.debug.queueBreakpoints(path, line)
	a.invalidate()
}

// removeBreakpointAt removes the breakpoint of a line, without adding one.
func (d *debugState) removeBreakpointAt(path string, line int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	list := d.breakpoints[path]
	for i, bp := range list {
		if bp.line == line {
			d.breakpoints[path] = append(list[:i], list[i+1:]...)
			if len(d.breakpoints[path]) == 0 {
				delete(d.breakpoints, path)
			}
			return
		}
	}
}

func (a *app) debugContinue() {
	a.debugStep("continue")
}

func (a *app) debugPause() {
	debug := &a.debug
	if debug.stopped != nil || debug.sessionNow() == nil {
		return
	}
	debug.call(a, "threads", nil, func(body json.RawMessage) {
		var result struct{ Threads []dapThread }
		if debug.stopped == nil && json.Unmarshal(body, &result) == nil && len(result.Threads) > 0 {
			debug.call(a, "pause", map[string]int{"threadId": result.Threads[0].ID}, nil)
		}
	})
}

func (a *app) debugStep(command string) {
	debug := &a.debug
	stopped := debug.stopped
	if stopped != nil && debug.thread != nil {
		debug.call(a, command, map[string]int{"threadId": *debug.thread}, func(json.RawMessage) {
			if debug.stopped == stopped {
				debug.continued(debug.target)
			}
		})
	}
}

func (d *debugState) continued(session *debugSession) {
	session.stopped = nil
	if d.target == session {
		d.focusOtherTarget(session)
	}
}

func (a *app) debugStepOver() { a.debugStep("next") }
func (a *app) debugStepInto() { a.debugStep("stepIn") }
func (a *app) debugStepOut()  { a.debugStep("stepOut") }

// debugBreakpoints stops at the lines of a file, from the editor's
// margin and the Debug view alike.
func (a *app) debugBreakpoints(path string) []int {
	var lines []int
	for _, bp := range a.debug.at(path) {
		lines = append(lines, bp.line)
	}
	return lines
}

// removeDebugBreakpoint removes the breakpoint the Debug view shows.
func (a *app) removeDebugBreakpoint(path string, line int) {
	a.debug.removeBreakpointAt(path, line)
	a.debug.queueBreakpoints(path, line)
	a.invalidate()
}

// openDebugBreakpoint shows a file at a breakpoint's line.
func (a *app) openDebugBreakpoint(path string, line int) {
	a.openFileAt(path, max(line, 0), true)
}

func (a *app) debugStop() {
	a.debug.stop()
	a.invalidate()
}

// debugView is the sidebar's Run and Debug view: its controls, its call
// stack, its variables, its breakpoints and its console.
func (a *app) debugView(c *ui.Context) {
	p, m := a.pal, a.pal.metricsOr()
	d := &a.debug
	d.mu.Lock()
	status, stopped, frames, scopes, vars, output := d.status, d.stopped, slices.Clone(d.frames), slices.Clone(d.scopes), slices.Clone(d.vars), d.output
	active := d.session != nil
	var bps []fileBreakpoint
	for _, path := range slices.Sorted(maps.Keys(d.breakpoints)) {
		for _, bp := range d.breakpointsForDisplay(path) {
			bps = append(bps, fileBreakpoint{path, bp})
		}
	}
	d.mu.Unlock()
	ui.Scroll(c.Key("debug-scroll")).Grow(1).MinHeight(0).TrackScroll(&d.scroll).Children(func() {
		ui.Column(c).Padding(m.pad, 0, m.pad, 0).Gap(m.gap).Children(func() {
			ui.Row(c.Key("debug-controls")).Padding(0, 2, 4, 2).Gap(4).AlignItems(ui.Center).Children(func() {
				if active {
					if stopped != nil && a.iconButton(c, iconPlay, locales.DebugContinue(), false).Clicked() {
						a.debugContinue()
					}
					if stopped == nil && status == locales.DebugRunning() && a.iconButton(c, iconPause, locales.DebugPause(), false).Clicked() {
						a.debugPause()
					}
					if a.iconButton(c, iconStopSquare, locales.DebugStop(), false).Clicked() {
						a.debugStop()
					}
				} else if compactActionButton(c, locales.DebugStart(), iconPlay, button.Primary).Clicked() {
					a.startDebug()
				}
				ui.Spacer(c)
				ui.Text(c, status).FontSize(m.textSmall).TextColor(p.muted).SingleLine()
			})
			if stopped != nil {
				ui.Row(c.Key("debug-steps")).Gap(3).AlignItems(ui.Center).Children(func() {
					if a.iconButton(c, iconStepOver, locales.DebugStepOver(), false).Clicked() {
						a.debugStepOver()
					}
					if a.iconButton(c, iconStepInto, locales.DebugStepInto(), false).Clicked() {
						a.debugStepInto()
					}
					if a.iconButton(c, iconStepOut, locales.DebugStepOut(), false).Clicked() {
						a.debugStepOut()
					}
					if stopped.Text != "" {
						ui.Text(c, stopped.Text).FontSize(m.textSmall).TextColor(p.modifiedFg).SingleLine().Grow(1).MinWidth(0)
					}
				})
			}
			a.debugSection(c, "debug-frames", locales.DebugCallStack(), len(frames), func() {
				if len(frames) == 0 {
					a.debugEmpty(c, "debug-no-frames", locales.DebugNoFrames())
					return
				}
				for i, fr := range frames {
					frame := fr
					name := frame.Name
					if last := filepath.Base(frame.Source.Path); frame.Source.Path != "" {
						name = last + ":" + strconv.Itoa(frame.Line) + " · " + name
					}
					a.debugRow(c, "frame-"+strconv.Itoa(i), name, func() bool {
						d.mu.Lock()
						defer d.mu.Unlock()
						return d.frame == frame.ID
					}(), func() { d.openFrame(frame) })
				}
			})
			a.debugSection(c, "debug-vars", locales.DebugVariables(), len(vars), func() {
				if len(scopes) > 0 {
					ui.Text(c, scopes[0].Name).FontSize(m.textSmall).TextColor(p.faint).Padding(2, 8, 0, 6)
				}
				if len(vars) == 0 {
					a.debugEmpty(c, "debug-no-vars", locales.DebugNoVariables())
					return
				}
				for i, v := range vars {
					variable := v
					a.debugRow(c, "var-"+strconv.Itoa(i), variable.Name+" = "+variable.Value, variable.VariablesReference > 0, func() {
						if variable.VariablesReference > 0 {
							d.refreshVariables(variable.VariablesReference)
						}
					})
				}
			})
			a.debugSection(c, "debug-breakpoints", locales.DebugBreakpoints(), len(bps), func() {
				if len(bps) == 0 {
					a.debugEmpty(c, "debug-no-bps", locales.DebugNoBreakpoints())
					return
				}
				for _, bp := range bps {
					a.debugBreakpointRow(c, bp.path, bp.breakpoint, active)
				}
			})
			a.debugSection(c, "debug-output", locales.DebugOutput(), len(output), func() {
				if output == "" {
					a.debugEmpty(c, "debug-no-output", locales.DebugNoOutput())
					return
				}
				ui.Text(c, output).Font(a.settings.FontFamily).FontSize(11.5).TextColor(p.text).Selectable()
			})
		})
	})
}

// debugSection is a titled group of the Debug view, open unless its
// title was clicked closed.
func (a *app) debugSection(c *ui.Context, key, title string, count int, body func()) {
	p := a.pal
	label := p.title(title)
	ui.Column(c.Key(key)).Shrink(0).Children(func() {
		head := ui.Row(c.Key("head")).Height(scmRowHeight).Padding(0, 2).Gap(4).AlignItems(ui.Center).Background(p.chrome)
		if head.Clicked() {
			if a.debug.closed == nil {
				a.debug.closed = make(map[string]bool)
			}
			a.debug.closed[key] = !a.debug.closed[key]
		}
		head.Children(func() {
			chevron(c, !a.debug.closed[key]).FontSize(12).TextColor(p.muted)
			size, spacing := p.titleLook(11, 0.4)
			ui.Text(c, label).FontSize(size).FontWeight(600).LetterSpacing(spacing).TextColor(p.muted).SingleLine()
			ui.Spacer(c)
			ui.Text(c, strconv.Itoa(count)).FontSize(10.5).TextColor(p.faint)
		})
		if !a.debug.closed[key] {
			body()
		}
	})
}

func (a *app) debugRow(c *ui.Context, key, label string, selected bool, run func()) {
	p, m := a.pal, a.pal.metricsOr()
	row := ui.Row(c.Key(key)).Height(scmRowHeight).Gap(6).AlignItems(ui.Center).Padding(0, 8).Label(label)
	if selected {
		row.Background(p.active)
	} else if row.Hovered() {
		row.Background(p.hover)
	}
	row.Children(func() {
		ui.Text(c, label).FontSize(m.textSmall).TextColor(p.text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
	})
	if row.Clicked() {
		run()
	}
}

// debugBreakpointRow is a breakpoint in the Debug view: its file and line
// open, and its own button removes it.
func (a *app) debugBreakpointRow(c *ui.Context, path string, bp breakpoint, active bool) {
	p, m := a.pal, a.pal.metricsOr()
	label := filepath.Base(path) + ":" + strconv.Itoa(bp.line+1)
	row := ui.Row(c.Key(path).Key(bp.line)).Height(scmRowHeight).Gap(6).AlignItems(ui.Center).Padding(0, 8).Label(label).Tooltip(cmp.Or(bp.message, path))
	if row.Hovered() {
		row.Background(p.hover)
	}
	row.Children(func() {
		dot := ui.Box(c.Key("dot")).Size(8, 8).Radius(4).Shrink(0).Border(1, p.dangerFg)
		if !active || bp.verified {
			dot.Background(p.dangerFg)
		}
		ui.Text(c, label).FontSize(m.textSmall).TextColor(p.text).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if row.Hovered() {
			if a.iconButton(c, iconX, locales.DebugRemoveBreakpoint(), false).Clicked() {
				a.removeDebugBreakpoint(path, bp.line)
			}
		}
	})
	if row.Clicked() {
		a.openDebugBreakpoint(path, bp.line)
	}
}

func (a *app) debugEmpty(c *ui.Context, key, text string) {
	p, m := a.pal, a.pal.metricsOr()
	ui.Text(c.Key(key), text).FontSize(m.textSmall).LineHeight(1.4).TextColor(p.faint).Padding(4, 8).MaxWidth(240)
}

// debugStatus is the session in the status bar: its state, and a click
// opens the Debug view.
func (a *app) debugStatus(c *ui.Context) {
	if !a.debug.active() {
		return
	}
	p := a.pal
	status := a.debug.statusText()
	b := ui.ButtonBase(c.Key("status:debug")).Height(statusBarH-6).Padding(0, p.metricsOr().pad).Gap(5).
		Radius(p.metricsOr().radius).AlignItems(ui.Center).Label(locales.DebugShow()).Tooltip(status)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Transition(hoverFade)
	b.Children(func() {
		ui.Icon(c, iconBug).FontSize(12).TextColor(p.accent)
		ui.Text(c, status).FontSize(p.metricsOr().textSmall).TextColor(p.statusFg).SingleLine()
	})
	if b.Clicked() {
		a.showDebug()
	}
}

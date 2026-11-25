package main

import (
	"context"
	"errors"
	"slices"
	"time"

	"fiveleaf/internal/dap"
	"fiveleaf/internal/remote"
	"fiveleaf/locales"
)

func (d *debugState) newSession(parent *debugSession, root string) *debugSession {
	lifetime := context.Background()
	if parent != nil {
		lifetime = parent.ctx
	}
	ctx, cancel := context.WithCancel(lifetime)
	session := &debugSession{ctx: ctx, cancel: cancel, root: root, parent: parent, changes: make(chan struct{}, 1),
		pending: make(map[string]bool), confirmed: make(map[string]map[int]dapBreakpoint)}
	for path := range d.breakpoints {
		if remote.HostOf(path) == remote.HostOf(root) {
			session.pending[path] = true
		}
	}
	d.sessions = append(d.sessions, session)
	return session
}

func (a *app) startChildDebugger(parent *debugSession, options dap.Options, request string, launch map[string]any) error {
	debug := &a.debug
	debug.mu.Lock()
	if parent.ctx.Err() != nil || !slices.Contains(debug.sessions, parent) || parent.client == nil {
		debug.mu.Unlock()
		return errors.New("parent debug session is unavailable")
	}
	options.Address = parent.client.Address()
	child := debug.newSession(parent, parent.root)
	child.adapterRoot = parent.adapterRoot
	debug.mu.Unlock()
	options.Cmd, options.Stdin, options.Ready = nil, nil, ""
	go func() {
		startup, cancel := context.WithTimeout(child.ctx, 30*time.Second)
		defer cancel()
		err := a.runDebugTarget(startup, child, options, request, launch)
		a.post(func() { a.endDebugTarget(child, err) })
	}()
	return nil
}

func (a *app) endDebugTarget(session *debugSession, err error) {
	debug := &a.debug
	if !debug.owns(session) {
		return
	}
	if session.parent == nil {
		debug.stop()
	} else {
		debug.mu.Lock()
		session.cancel()
		debug.sessions = slices.DeleteFunc(debug.sessions, func(target *debugSession) bool { return target.ctx.Err() != nil })
		needsFocus := !slices.Contains(debug.sessions, debug.target)
		debug.mu.Unlock()
		if needsFocus {
			debug.focusOtherTarget(session.parent)
		}
	}
	if err != nil {
		a.fail("%s", locales.DebugLaunchFailed(err.Error()))
	}
	a.invalidate()
}

func (d *debugState) focusOtherTarget(fallback *debugSession) {
	d.mu.Lock()
	for index := len(d.sessions) - 1; index >= 0; index-- {
		if d.sessions[index].stopped != nil {
			fallback = d.sessions[index]
			break
		}
	}
	d.mu.Unlock()
	d.focusTarget(fallback)
}

func (d *debugState) focusTarget(session *debugSession) {
	d.mu.Lock()
	d.target, d.stopped = session, session.stopped
	d.thread, d.frame = nil, 0
	d.frames, d.threads, d.scopes, d.vars = nil, nil, nil, nil
	switch {
	case session.stopped != nil:
		d.status, d.thread = locales.DebugPaused(), session.stopped.ThreadID
	case session.ready:
		d.status = locales.DebugRunning()
	default:
		d.status = locales.DebugStarting()
	}
	d.mu.Unlock()
	if session.stopped != nil {
		d.refreshThreads()
		d.refreshFrames()
	}
}

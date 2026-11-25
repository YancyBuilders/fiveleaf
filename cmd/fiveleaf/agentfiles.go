package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"

	"fiveleaf/internal/acp"
)

// ACP paths are checked both by the transport and at the file handle. Root keeps
// a symlink changed between validation and I/O from redirecting a write outside
// the workspace. Agent processes themselves still run with the user's privileges.
func openAgentRoot(root, path string) (*os.Root, string, error) {
	if !filepath.IsAbs(path) {
		return nil, "", errors.New("ACP requires an absolute file path")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", err
	}
	rel, err := filepath.Rel(realRoot, filepath.Clean(path))
	if err != nil || !filepath.IsLocal(rel) {
		return nil, "", errors.New("file is outside the workspace")
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.EqualFold(part, ".git") {
			return nil, "", errors.New("file requests cannot access Git metadata")
		}
	}
	fs, err := os.OpenRoot(realRoot)
	return fs, rel, err
}

func readAgentFile(root, path string) (string, error) {
	data, _, err := readAgentFileData(root, path, agentTextLimit)
	if err != nil {
		return "", err
	}
	return agentFileText(data)
}

func agentFileText(data []byte) (string, error) {
	if !utf8.Valid(data) || isBinary(data) {
		return "", errors.New("file is too large or is not UTF-8 text")
	}
	return string(data), nil
}

var errAgentFileTooLarge = errors.New("file exceeds the agent context limit")

func readAgentFileData(root, path string, limit int64) ([]byte, int64, error) {
	fs, rel, err := openAgentRoot(root, path)
	if err != nil {
		return nil, 0, err
	}
	defer fs.Close()
	file, info, err := openAgentFileAt(fs, rel)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	return readAgentFileContents(file, info, limit)
}

func openAgentFileAt(fs *os.Root, rel string) (*os.File, os.FileInfo, error) {
	file, err := fs.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, errors.New("only regular text files can be read")
	}
	return file, info, nil
}

func readAgentFileContents(file *os.File, info os.FileInfo, limit int64) ([]byte, int64, error) {
	if info.Size() > limit {
		head, err := io.ReadAll(io.LimitReader(file, 512))
		if err != nil {
			return nil, 0, err
		}
		return head, info.Size(), errAgentFileTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(data)) > limit {
		return data[:min(len(data), 512)], int64(len(data)), errAgentFileTooLarge
	}
	return data, int64(len(data)), nil
}

func (a *app) agentDocument(path string) *document {
	for _, d := range a.docs {
		if d.path == path {
			return d
		}
		// An editor may have opened the workspace through a symlink. The
		// protocol supplies canonical paths, but must still see its buffer.
		if d.path != "" {
			real, err := filepath.EvalSymlinks(d.path)
			if err == nil && real == path {
				return d
			}
		}
	}
	return nil
}

func (a *app) agentFileWindows() []*app {
	windows := []*app{a}
	for _, other := range a.host.everyProject() {
		if other != a && !other.loadsClosed && a.root != "" && other.root == a.root {
			windows = append(windows, other)
		}
	}
	return windows
}

func (h *agentHandler) ReadTextFile(ctx context.Context, request acp.ReadRequest) (string, error) {
	var text string
	var found bool
	err := h.onMain(ctx, func() error {
		if string(request.SessionId) != h.state().sessionID {
			return acp.ErrUnknownSession
		}
		for _, window := range h.workspace().agentFileWindows() {
			if d := window.agentDocument(request.Path); d != nil {
				if d.locked() {
					return errors.New("document is still loading or unavailable")
				}
				text, found = d.buf.String(), true
				if d.dirty() {
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found {
		if len(text) > agentTextLimit {
			return "", errors.New("document exceeds the agent context limit")
		}
		return text, nil
	}
	return readAgentFile(h.root, request.Path)
}

func (h *agentHandler) WriteTextFile(ctx context.Context, request acp.WriteRequest) error {
	if len(request.Content) > agentTextLimit || !utf8.ValidString(request.Content) || isBinary([]byte(request.Content)) || hasLongLine([]byte(request.Content)) {
		return errors.New("agent writes require UTF-8 text within the editor's size limits")
	}
	var d *document
	var docWindow *app
	var docPath string
	err := h.onMain(ctx, func() error {
		a := h.workspace()
		if string(request.SessionId) != h.state().sessionID {
			return acp.ErrUnknownSession
		}
		for _, window := range a.agentFileWindows() {
			if window.agentWrites[request.Path] {
				return errors.New("another agent is preparing a write to this file; retry after it finishes")
			}
			if doc := window.agentDocument(request.Path); doc != nil {
				if doc.dirty() || doc.sync != syncIdle || doc.offline {
					return fmt.Errorf("%s has unsaved changes or a pending save; save or revert it before the agent writes", request.Path)
				}
				if d == nil {
					d, docWindow = doc, window
				}
			}
		}
		if d != nil {
			docPath = d.path
			d.sync = syncSaving
		}
		if a.agentWrites == nil {
			a.agentWrites = map[string]bool{}
		}
		a.agentWrites[request.Path] = true
		return nil
	})
	if err != nil {
		return err
	}
	// Keep filesystem work off the UI thread. A user can continue typing;
	// newer buffer changes are retained when this write finishes.
	write, prepareErr := prepareAgentFile(ctx, h.root, request.Path, request.Content)
	if prepareErr == nil {
		prepareErr = write.verifyContents()
	}
	done := make(chan error, 1)
	h.dispatch(func(a *app) {
		delete(a.agentWrites, request.Path)
		err := prepareErr
		if err == nil {
			if ctx.Err() != nil || !h.valid() || string(request.SessionId) != h.state().sessionID {
				err = context.Canceled
			} else {
				err = a.finishAgentWrite(write, d, docPath)
			}
		}
		if err != nil && write != nil {
			write.finish(false)
		}
		if d != nil && slices.Contains(docWindow.docs, d) {
			d.sync = syncIdle
			docWindow.nextSave(d)
		}
		if err == nil && h.valid() {
			a.diskChanged([]string{request.Path}, nil)
			tool := &agentTool{
				id: "editor:" + rand.Text(), sessionID: h.state().sessionID, generation: h.generation, title: request.Path, kind: "edit", status: "completed",
				changes: []agentChange{write.change},
			}
			h.state().tools = append(h.state().tools, tool)
			if h.toolChanged != nil {
				h.toolChanged(tool)
			}
		}
		done <- err
	})
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if write != nil {
			write.finish(false)
		}
		return ctx.Err()
	}
}

// Publish only while the editor still holds the clean document on which the
// request was based. This final check and rename share one UI turn, so autosave
// cannot write an old buffer over an agent edit completed in the background.
func (a *app) finishAgentWrite(w *agentFileWrite, original *document, docPath string) error {
	windows := a.agentFileWindows()
	found := false
	for _, window := range windows {
		d := window.agentDocument(w.change.path)
		if d != nil && (d.dirty() || d.locked() || d != original && d.sync != syncIdle) {
			return errors.New("document changed while the agent prepared its edit; save or revert it before retrying")
		}
		found = found || d == original
	}
	if original != nil && (original.path != docPath || !found) {
		return errors.New("document was moved or closed while the agent prepared its edit; retry the edit")
	}
	if err := w.finish(true); err != nil {
		return err
	}
	for _, window := range windows {
		if d := window.agentDocument(w.change.path); d != nil {
			d.takeText([]byte(w.change.newText))
			window.invalidate()
		}
	}
	return nil
}

type agentFileWrite struct {
	root              *os.Root
	staging           *os.Root
	temporary, target string
	directory         os.FileInfo
	original          os.FileInfo
	change            agentChange
	once              sync.Once
	err               error
}

var errAgentWriteConflict = errors.New("file changed on disk while the agent prepared its edit")

func sameAgentFileVersion(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Mode() == after.Mode() &&
		before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

func (w *agentFileWrite) verifyContents() error {
	if w.original == nil {
		return nil
	}
	file, opened, err := openAgentFileAt(w.root, w.target)
	if errors.Is(err, os.ErrNotExist) {
		return errAgentWriteConflict
	}
	if err != nil {
		return err
	}
	defer file.Close()
	if !sameAgentFileVersion(w.original, opened) {
		return errAgentWriteConflict
	}
	data, _, err := readAgentFileContents(file, opened, agentTextLimit)
	if err != nil {
		return err
	}
	if string(data) != w.change.oldText {
		return errAgentWriteConflict
	}
	return nil
}

func (w *agentFileWrite) publish() error {
	directory, err := w.root.Stat(filepath.Dir(w.target))
	if errors.Is(err, os.ErrNotExist) || err == nil && !os.SameFile(w.directory, directory) {
		return errAgentWriteConflict
	}
	if err != nil {
		return err
	}
	if w.original != nil {
		return w.replaceTarget(w.original)
	}
	err = w.root.Link(w.temporary, w.target)
	if errors.Is(err, os.ErrExist) {
		return errAgentWriteConflict
	}
	if errors.Is(err, errors.ErrUnsupported) || errors.Is(err, syscall.EPERM) {
		return w.createTarget()
	}
	return err
}

func (w *agentFileWrite) replaceTarget(expected os.FileInfo) error {
	current, err := w.root.Lstat(w.target)
	if errors.Is(err, os.ErrNotExist) || err == nil && !sameAgentFileVersion(expected, current) {
		return errAgentWriteConflict
	}
	if err != nil {
		return err
	}
	return w.root.Rename(w.temporary, w.target)
}

func (w *agentFileWrite) createTarget() error {
	file, err := w.root.OpenFile(w.target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return errAgentWriteConflict
	}
	if err != nil {
		return err
	}
	created, statErr := file.Stat()
	err = errors.Join(statErr, file.Close())
	if err == nil {
		err = w.replaceTarget(created)
	}
	if err != nil && created != nil {
		if current, statErr := w.root.Lstat(w.target); statErr == nil && sameAgentFileVersion(created, current) {
			_ = w.root.Remove(w.target)
		}
	}
	return err
}

func (w *agentFileWrite) finish(commit bool) error {
	w.once.Do(func() {
		if commit {
			w.err = w.publish()
		} else {
			w.err = context.Canceled
		}
		if w.staging != nil {
			if w.temporary != "" {
				_ = w.staging.Remove(filepath.Base(w.temporary))
			}
			_ = w.staging.Close()
		}
		_ = w.root.Close()
	})
	return w.err
}

func prepareAgentFile(ctx context.Context, root, path, content string) (*agentFileWrite, error) {
	change := agentChange{path: path, newText: content, existed: true}
	fs, rel, err := openAgentRoot(root, path)
	if err != nil {
		return nil, err
	}
	w := &agentFileWrite{root: fs, target: rel, change: change}
	prepared := false
	defer func() {
		if !prepared {
			w.finish(false)
		}
	}()
	mode := os.FileMode(0o644)
	if info, statErr := fs.Lstat(rel); errors.Is(statErr, os.ErrNotExist) {
		w.change.existed = false
	} else if statErr != nil {
		return nil, statErr
	} else if !info.Mode().IsRegular() {
		return nil, errors.New("only regular text files can be written")
	} else {
		file, opened, err := openAgentFileAt(fs, rel)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if !sameAgentFileVersion(info, opened) {
			return nil, errAgentWriteConflict
		}
		data, _, err := readAgentFileContents(file, opened, agentTextLimit)
		if err != nil {
			return nil, err
		}
		w.change.oldText, err = agentFileText(data)
		if err != nil {
			return nil, err
		}
		w.original, mode = opened, opened.Mode().Perm()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory := filepath.Dir(rel)
	if err := fs.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	w.staging, err = fs.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	w.directory, err = w.staging.Stat(".")
	if err != nil {
		return nil, err
	}
	tmp := ".fiveleaf-acp-" + rand.Text()
	f, err := w.staging.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return nil, err
	}
	w.temporary = filepath.Join(directory, tmp)
	_, writeErr := io.WriteString(f, content)
	closeErr := f.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared = true
	return w, nil
}

func agentContextText(path string, from, to int, text string) string {
	where := path
	if from > 0 {
		where += fmt.Sprintf(":%d-%d", from, to)
	}
	f := fence(strings.Split(text, "\n"))
	return where + "\n" + f + "\n" + text + "\n" + f
}

package agents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrHistoryChanged = errors.New("session history changed")
	errHistoryTarget  = errors.New("the session has no single supported history record")
)

type historyFile struct {
	path       string
	info       os.FileInfo
	directory  os.FileInfo
	transcript os.FileInfo
}

func (session Session) CanTrashHistory() bool { return session.history != nil }

func (session Session) HistoryTrashPath() (string, error) {
	if session.history == nil {
		return "", errHistoryTarget
	}
	history := session.history
	resolved, err := filepath.EvalSymlinks(history.path)
	if err != nil {
		return "", err
	}
	if resolved != history.path {
		return "", ErrHistoryChanged
	}
	info, err := os.Lstat(history.path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || !sameFileVersion(history.info, info) {
		return "", ErrHistoryChanged
	}
	if history.directory == nil {
		return history.path, nil
	}
	directory, transcript, err := grokHistoryFiles(history.path, info)
	if err != nil {
		return "", err
	}
	if !sameFileVersion(history.directory, directory) || !sameFileVersion(history.transcript, transcript) {
		return "", ErrHistoryChanged
	}
	return filepath.Dir(history.path), nil
}

func sameFileVersion(previous, current os.FileInfo) bool {
	if previous == nil || current == nil {
		return previous == nil && current == nil
	}
	return previous.Size() == current.Size() && previous.ModTime().Equal(current.ModTime()) && os.SameFile(previous, current)
}

func nativeHistoryProfile(env *Env, harness Harness) string {
	switch harness {
	case Codex:
		return env.CodexRoot()
	case Pi:
		return env.PiRoot()
	case Qoder:
		return filepath.Join(env.Home, ".qoder")
	case Grok:
		return filepath.Join(env.Home, ".grok")
	}
	return ""
}

func nativeHistoryPath(path, resolved, profile, resolvedProfile string) string {
	if profile == "" || !filepath.IsAbs(resolved) {
		return ""
	}
	relative, err := filepath.Rel(profile, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	if resolved != filepath.Join(resolvedProfile, relative) {
		return ""
	}
	return resolved
}

func historyFileFor(path string, info os.FileInfo, session Session) *historyFile {
	if path == "" || !info.Mode().IsRegular() {
		return nil
	}
	history := &historyFile{path: path, info: info}
	if session.Harness == Grok {
		dir := filepath.Dir(path)
		if filepath.Base(dir) != session.ID || pathWithin(canonicalPath(session.Dir), dir) {
			return nil
		}
		directory, transcript, err := grokHistoryFiles(path, info)
		if err != nil {
			return nil
		}
		history.directory, history.transcript = directory, transcript
	}
	return history
}

func grokHistoryFiles(summaryPath string, summary os.FileInfo) (os.FileInfo, os.FileInfo, error) {
	dir := filepath.Dir(summaryPath)
	directory, err := os.Lstat(dir)
	if err != nil {
		return nil, nil, err
	}
	if !directory.IsDir() {
		return nil, nil, ErrHistoryChanged
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	foundSummary := false
	var transcript os.FileInfo
	for _, entry := range entries {
		if entry.Name() != "summary.json" && entry.Name() != "chat_history.jsonl" {
			return nil, nil, ErrHistoryChanged
		}
		info, err := entry.Info()
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, nil, ErrHistoryChanged
		}
		if entry.Name() == "summary.json" {
			foundSummary = sameFileVersion(summary, info)
		} else {
			transcript = info
		}
	}
	if !foundSummary {
		return nil, nil, ErrHistoryChanged
	}
	return directory, transcript, nil
}

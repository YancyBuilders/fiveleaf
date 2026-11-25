package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (request *copilotRequest) completeText(ctx context.Context, snapshot ui.TextSnapshot, start, end int, language string, inline bool) (string, error) {
	fragment, err := json.Marshal(struct{ Language, Before, After string }{
		language, snapshot.Slice(max(0, start-2048), start), snapshot.Slice(end, min(snapshot.Len(), end+1024)),
	})
	if err != nil {
		return "", err
	}
	instruction := "Complete the code or prose between Before and After. Return only the text to insert, with its exact spacing and indentation. Do not use tools or wrap the answer in Markdown."
	if inline {
		instruction += " Suggest only the rest of the current line. Omit text already present in After."
	}
	return request.generate(ctx, instruction+"\n"+string(fragment))
}

func (a *app) suggestWithCopilot() {
	editorView := a.textEditor()
	if editorView == nil || editorView.locked() {
		return
	}
	snapshot := editorView.buf.Snapshot()
	selection, path := editorView.sel, editorView.path
	start, end := min(selection[0], selection[1]), max(selection[0], selection[1])
	if start < 0 || end > snapshot.Len() {
		return
	}
	valid := func() bool {
		return a.activeEditor() == editorView && !editorView.locked() && editorView.preview == nil &&
			editorView.sel == selection && editorView.path == path && editorView.buf.Version() == snapshot.Version()
	}
	language := languageOf(editorView.document)
	a.runCopilot(valid, func(ctx context.Context, request *copilotRequest) (string, error) {
		return request.completeText(ctx, snapshot, start, end, language, false)
	}, func(request *copilotRequest, text string) {
		if a.overlaid() {
			return
		}
		request.preview = true
		label := text
		if strings.TrimSpace(label) == "" {
			label = strconv.Quote(label)
		}
		a.openPluginPicker([]pickItem{{Label: label, Icon: "sparkles"}}, locales.CopilotAccept(), func(index int) {
			request.preview = false
			if a.copilot != request {
				return
			}
			if index != 0 || !request.current(a, a.agentProviders()) {
				a.cancelCopilot()
				return
			}
			a.queueEdit(editorView, func(target *editor) {
				if a.copilot != request {
					return
				}
				a.cancelCopilot()
				if request.current(a, a.agentProviders()) {
					caret := start + utf8.RuneCountInString(text)
					target.edit([2]int{caret, caret}, textEdit{start, end, text})
				}
			})
		})
	})
}

func (repository *gitContext) draftWithCopilot() {
	if !repository.live() || repository.git.commanding() {
		return
	}
	state := repository.git
	repo, draft, amend := state.repo, state.message, state.amend
	indexPath := os.Getenv("GIT_INDEX_FILE")
	if indexPath == "" {
		indexPath = filepath.Join(repo.Dir, "index")
	} else if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repo.Root, indexPath)
	}
	var indexBeforeVerify os.FileInfo
	excluded := slices.Clone(repository.app.settings.AISecretFiles)
	valid := func() bool {
		return repository.live() && state.repo == repo && state.message == draft && state.amend == amend && !state.commanding() &&
			slices.Equal(excluded, repository.app.settings.AISecretFiles)
	}
	repository.app.runCopilot(valid, func(ctx context.Context, request *copilotRequest) (string, error) {
		const maxDiffBytes = 48 << 10
		diff, staged, err := repo.MessageDiffContext(ctx, amend, maxDiffBytes, excluded)
		if err != nil {
			return "", err
		}
		if diff == "" {
			return "", errors.New(locales.NoChangesToCommit())
		}
		text, err := request.generate(ctx, "Draft a Git commit message describing the supplied changes. Start with a subject of at most 72 characters. Return only the message. Do not use tools.\n\nAuthor's notes:\n"+draft+"\n\nChanges:\n"+diff)
		if err != nil {
			return "", err
		}
		indexBeforeVerify, err = os.Stat(indexPath)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		current, nowStaged, err := repo.MessageDiffContext(ctx, amend, maxDiffBytes, excluded)
		if err == nil && (current != diff || nowStaged != staged) {
			err = errors.New(locales.CopilotDiffChanged())
		}
		return strings.TrimSpace(text), err
	}, func(_ *copilotRequest, text string) {
		if sameCopilotIndex(indexPath, indexBeforeVerify) {
			state.message = text
		} else {
			repository.app.tell(note{message: locales.CopilotDiffChanged()})
		}
	})
}

func sameCopilotIndex(path string, before os.FileInfo) bool {
	after, err := os.Stat(path)
	if before == nil {
		return os.IsNotExist(err)
	}
	return err == nil && os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

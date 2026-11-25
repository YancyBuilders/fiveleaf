package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/git"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"github.com/egoist/mygo/ui"
)

const inlineCompletionPause = 400 * time.Millisecond

type inlineCompletion struct {
	editor    *editor
	folder    folderKey
	path      string
	snapshot  ui.TextSnapshot
	caret     int
	selection settings.CopilotConfig
	due       time.Time
	request   *copilotRequest
	excluded  []string
	text      string
}

func (a *app) dismissInlineCompletion() {
	if pending := a.inlineCopilot; pending != nil {
		a.inlineCopilot = nil
		if pending.request != nil {
			pending.request.cancel()
		}
	}
}

func (a *app) inlineCompletionFailed(err error) {
	a.dismissInlineCompletion()
	if !errors.Is(err, context.Canceled) && a.inlineFailureFor != a.settings.Copilot {
		a.inlineFailureFor = a.settings.Copilot
		a.copilotError(err)
	}
}

func (a *app) canCompleteInline(doc *editor) bool {
	return a.settings.CopilotInlineSuggestions && a.settings.Copilot.Agent != "" && a.copilot == nil &&
		(a.wordSuggest == nil || len(a.wordSuggest.items) == 0) &&
		!a.loadsClosed && !a.blurred && a.conn == nil && a.root != "" && !remote.Is(a.root) &&
		a.focus == paneEditor && a.activeEditor() == doc && doc.preview == nil && !doc.locked() &&
		doc.sel[0] == doc.sel[1] && doc.pending == nil && !doc.jumpHints.active && !a.chording() && !a.overlaid() &&
		(editorMode(a.prefs()) != "vim" || doc.vim.mode == vimInsert) && (!a.helixOn() || doc.helix.mode == helixInsert)
}

func (pending *inlineCompletion) sameTarget(a *app) bool {
	doc := pending.editor
	return a.canCompleteInline(doc) && a.folderKey() == pending.folder && a.settings.Copilot == pending.selection &&
		doc.path == pending.path && (pending.request == nil || slices.Equal(pending.excluded, a.settings.AISecretFiles))
}

func (pending *inlineCompletion) samePosition() bool {
	return pending.editor.buf.Version() == pending.snapshot.Version() && pending.editor.sel[1] == pending.caret
}

func (pending *inlineCompletion) current(a *app) bool {
	return pending.samePosition() && pending.sameTarget(a)
}

func (pending *inlineCompletion) followTyping(now ui.TextSnapshot, caret int) bool {
	old, start := pending.snapshot, pending.caret
	added := caret - start
	if added <= 0 || added != now.Len()-old.Len() || added > utf8.RuneCountInString(pending.text) {
		return false
	}
	typed := now.Slice(start, caret)
	if !strings.HasPrefix(pending.text, typed) ||
		old.Slice(max(0, start-2048), start) != now.Slice(max(0, start-2048), start) ||
		old.Slice(start, min(old.Len(), start+1024)) != now.Slice(caret, min(now.Len(), caret+1024)) {
		return false
	}
	pending.text, pending.caret, pending.snapshot = strings.TrimPrefix(pending.text, typed), caret, now
	return pending.text != ""
}

func (a *app) inlineCompletionFrame(c *ui.Context, doc *editor, snapshot ui.TextSnapshot, focused, typed bool) {
	pending := a.inlineCopilot
	if !typed && (pending == nil || pending.editor != doc) {
		return
	}
	if !focused || !a.canCompleteInline(doc) || doc.buf.Version() != snapshot.Version() {
		if pending != nil && pending.editor == doc {
			a.dismissInlineCompletion()
		}
		return
	}
	if typed {
		if pending != nil && pending.text != "" && pending.sameTarget(a) && pending.followTyping(snapshot, doc.sel[1]) {
			return
		}
		a.dismissInlineCompletion()
		if snapshot.Len() == 0 {
			return
		}
		pending = &inlineCompletion{editor: doc, folder: a.folderKey(), path: doc.path, selection: a.settings.Copilot,
			snapshot: snapshot, caret: doc.sel[1], due: c.Now().Add(inlineCompletionPause)}
		a.inlineCopilot = pending
	}
	if !pending.current(a) {
		a.dismissInlineCompletion()
		return
	}
	if pending.request != nil {
		return
	}
	if remaining := pending.due.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	provider, err := a.copilotProvider()
	if err != nil {
		a.inlineCompletionFailed(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(provider.config.ReplyTimeoutSeconds())*time.Second)
	request := &copilotRequest{selection: pending.selection, provider: provider, folder: pending.folder, cancel: cancel,
		valid: func() bool { return pending.current(a) }}
	pending.request, pending.excluded = request, slices.Clone(a.settings.AISecretFiles)
	language := languageOf(doc.document)
	go func() {
		defer cancel()
		var suggestion string
		var generationErr error
		allowed := inlineFileAllowed(pending.folder.root, pending.path, pending.excluded) && ctx.Err() == nil
		if allowed {
			var text string
			text, generationErr = request.completeText(ctx, snapshot, pending.caret, pending.caret, language, true)
			if generationErr == nil {
				after := snapshot.Slice(pending.caret, min(snapshot.Len(), pending.caret+256))
				suggestion = inlineSuggestionText(text, after)
			}
		}
		a.post(func() {
			if a.inlineCopilot != pending {
				return
			}
			if !allowed || !request.current(a, a.agentProviders()) {
				a.dismissInlineCompletion()
				return
			}
			if generationErr != nil {
				a.inlineCompletionFailed(generationErr)
				return
			}
			a.inlineFailureFor = settings.CopilotConfig{}
			if suggestion == "" {
				a.dismissInlineCompletion()
				return
			}
			pending.text = suggestion
		})
	}()
}

func inlineFileAllowed(root, path string, excluded []string) bool {
	if path == "" {
		return true
	}
	allowed := func(root, path string) bool {
		relative, err := filepath.Rel(root, path)
		if err != nil || !filepath.IsLocal(relative) {
			return false
		}
		name := filepath.ToSlash(relative)
		return !slices.ContainsFunc(strings.Split(name, "/"), func(part string) bool { return strings.EqualFold(part, ".git") }) && !git.SecretFile(name, excluded)
	}
	if !allowed(root, path) {
		return false
	}
	realRoot, rootErr := filepath.EvalSymlinks(root)
	realPath, pathErr := filepath.EvalSymlinks(path)
	return rootErr == nil && pathErr == nil && allowed(realRoot, realPath)
}

func inlineSuggestionText(reply, after string) string {
	line, _, _ := strings.Cut(reply, "\n")
	line = strings.TrimSuffix(line, "\r")
	if strings.HasPrefix(strings.TrimSpace(line), "```") || utf8.RuneCountInString(line) > 256 || strings.IndexFunc(line, func(char rune) bool { return char != '\t' && unicode.IsControl(char) }) >= 0 {
		return ""
	}
	for offset := range line {
		if strings.HasPrefix(after, line[offset:]) {
			line = line[:offset]
			break
		}
	}
	if strings.TrimSpace(line) == "" {
		return ""
	}
	return line
}

func (a *app) acceptInlineCompletion(pending *inlineCompletion) bool {
	if a.inlineCopilot != pending || pending.text == "" || !pending.request.current(a, a.agentProviders()) {
		return false
	}
	a.dismissInlineCompletion()
	a.queueEdit(pending.editor, func(target *editor) {
		if pending.request.current(a, a.agentProviders()) {
			end := pending.caret + utf8.RuneCountInString(pending.text)
			target.edit([2]int{end, end}, textEdit{pending.caret, pending.caret, pending.text})
		}
	})
	return true
}

func (a *app) inlineCompletionInput(doc *editor, event ui.InputEvent) bool {
	pending := a.inlineCopilot
	if pending == nil || pending.editor != doc {
		return false
	}
	switch event.Kind {
	case ui.InputKeyDown:
		switch event.Key {
		case ui.KeyTab:
			if event.Mods == 0 && a.acceptInlineCompletion(pending) {
				return true
			}
		case ui.KeyEscape:
			visible := pending.text != ""
			a.dismissInlineCompletion()
			return visible && event.Mods == 0
		case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd, ui.KeyPageUp, ui.KeyPageDown, ui.KeyEnter:
		default:
			return false
		}
		a.dismissInlineCompletion()
	case ui.InputPointerDown, ui.InputCompose, ui.InputSelect, ui.InputCommand:
		a.dismissInlineCompletion()
	}
	return false
}

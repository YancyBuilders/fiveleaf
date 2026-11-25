package main

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/lsp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

const wordSuggestPause = 180 * time.Millisecond

type wordSuggestions struct {
	editor     *editor
	root, path string
	version    uint64
	selection  [2]int
	manual     bool
	due        time.Time
	cancel     context.CancelFunc
	items      []lsp.CompletionItem
	selected   int
}

func (a *app) newWordSuggestions(doc *editor, manual bool) *wordSuggestions {
	a.dismissWordSuggestions()
	request := &wordSuggestions{editor: doc, root: a.root, path: doc.path, version: doc.buf.Version(), selection: doc.sel, manual: manual}
	a.wordSuggest = request
	return request
}

func (a *app) dismissWordSuggestions() {
	if request := a.wordSuggest; request != nil {
		a.wordSuggest = nil
		if request.cancel != nil {
			request.cancel()
		}
	}
}

func (request *wordSuggestions) current(a *app) bool {
	doc := request.editor
	return !a.blurred && !a.loadsClosed && !a.overlaid() && a.focus == paneEditor && a.root == request.root && a.activeEditor() == doc &&
		doc.path == request.path && doc.buf.Version() == request.version && doc.sel == request.selection &&
		doc.pending == nil && doc.preview == nil && !doc.locked() && (request.manual || a.canSuggestWords(doc))
}

func (a *app) canSuggestWords(doc *editor) bool {
	return a.prefs().QuickSuggestions && a.focus == paneEditor && doc.sel[0] == doc.sel[1] && !a.livePreview(doc) &&
		!doc.jumpHints.active && !a.chording() && (editorMode(a.prefs()) != "vim" || doc.vim.mode == vimInsert) &&
		(!a.helixOn() || doc.helix.mode == helixInsert)
}

func (a *app) complete() {
	if doc := a.textEditor(); doc != nil && !doc.locked() {
		a.focusColumn()
		a.fetchWords(a.newWordSuggestions(doc, true))
	}
}

func (a *app) wordSuggestionsFrame(c *ui.Context, doc *editor, snap ui.TextSnapshot, focused, typed bool) {
	request := a.wordSuggest
	if typed && focused && a.canSuggestWords(doc) && !a.overlaid() && snap.Version() == doc.buf.Version() {
		request = a.newWordSuggestions(doc, false)
		request.due = c.Now().Add(wordSuggestPause)
	}
	if request == nil || request.editor != doc || request.manual {
		return
	}
	if !focused || !request.current(a) {
		a.dismissWordSuggestions()
		return
	}
	if !request.due.IsZero() {
		if remaining := request.due.Sub(c.Now()); remaining > 0 {
			c.After(remaining)
			return
		}
		request.due = time.Time{}
		a.fetchWords(request)
	}
}

func completionPrefix(snap ui.TextSnapshot, caret int) string {
	start, _ := snap.LineRange(snap.LineAt(caret))
	before := snap.Slice(max(start, caret-semanticTokenLimit), caret)
	if split := strings.LastIndexFunc(before, func(char rune) bool { return !isWordRune(char) }); split >= 0 {
		_, size := utf8.DecodeRuneInString(before[split:])
		return before[split+size:]
	}
	return before
}

func completionRemainder(text, prefix string) (string, bool) {
	for _, typed := range prefix {
		char, size := utf8.DecodeRuneInString(text)
		if size == 0 || unicode.ToLower(char) != unicode.ToLower(typed) {
			return "", false
		}
		text = text[size:]
	}
	return text, true
}

func documentWords(snap ui.TextSnapshot, caret int, prefix string) []lsp.CompletionItem {
	if prefix == "" {
		return nil
	}
	const radius = 16 << 10
	left, right := max(0, caret-radius), min(snap.Len(), caret+radius)
	start := caret - utf8.RuneCountInString(prefix)
	before, after := snap.Slice(left, start), snap.Slice(caret, right)
	separator := func(char rune) bool { return !isWordRune(char) }
	tail := strings.TrimLeftFunc(after, isWordRune)
	if tail == "" && right < snap.Len() {
		return nil
	}
	end := caret + utf8.RuneCountInString(after[:len(after)-len(tail)])
	if left > 0 {
		before = strings.TrimLeftFunc(before, isWordRune)
	}
	if right < snap.Len() {
		tail = strings.TrimRightFunc(tail, isWordRune)
	}
	var items []lsp.CompletionItem
	seen := map[string]bool{prefix: true}
	var run lsp.Range
	for _, section := range []string{before, tail} {
		for word := range strings.FieldsFuncSeq(section, separator) {
			if !seen[word] && strings.HasPrefix(word, prefix) {
				if len(items) == 0 {
					run = lsp.Range{Start: lsp.Position(positionOf(snap, start)), End: lsp.Position(positionOf(snap, end))}
				}
				seen[word] = true
				items = append(items, lsp.CompletionItem{Label: word, TextEdit: &lsp.TextEdit{Range: run, NewText: word}})
			}
		}
	}
	return items
}

func (a *app) fetchWords(request *wordSuggestions) {
	doc := request.editor
	snap, caret := doc.buf.Snapshot(), request.selection[1]
	prefix := completionPrefix(snap, caret)
	server := a.lspFor(doc.document, false)
	trigger := ""
	if !request.manual && utf8.RuneCountInString(prefix) == 1 {
		client := server.live()
		if caret < 2 || client == nil || !client.TriggersCompletion(snap.Slice(caret-2, caret-1)) {
			a.dismissWordSuggestions()
			return
		}
	}
	if !request.manual && prefix == "" {
		if caret == 0 {
			a.dismissWordSuggestions()
			return
		}
		trigger = snap.Slice(caret-1, caret)
		if char, _ := utf8.DecodeRuneInString(trigger); unicode.IsSpace(char) {
			a.dismissWordSuggestions()
			return
		}
	}
	show := func(items []lsp.CompletionItem, err error) {
		if a.wordSuggest == request && request.current(a) {
			a.showWords(request, snap, prefix, items, err)
		}
	}
	if server == nil || (!request.manual && snap.ByteLen() > widestWhole) {
		show(nil, nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	request.cancel = cancel
	query := func() {
		if a.wordSuggest != request || !request.current(a) {
			cancel()
			return
		}
		if err := ctx.Err(); err != nil {
			show(nil, err)
			return
		}
		client := server.live()
		if client == nil || !client.Completion() || trigger != "" && !client.TriggersCompletion(trigger) {
			cancel()
			show(nil, nil)
			return
		}
		uri, pos := serverURI(doc.path), lsp.Position(caretPosition(doc))
		synced := client.OpenAsync(ctx, uri, languageOf(doc.document), snap.String())
		go func() {
			defer cancel()
			var items []lsp.CompletionItem
			err := <-synced
			if err == nil {
				items, err = client.Complete(ctx, uri, pos, trigger)
			}
			a.post(func() { show(items, err) })
		}()
	}
	if request.manual && server.state() == lspStarting {
		go func() {
			select {
			case <-ctx.Done():
			case <-server.ready:
			}
			a.post(query)
		}()
	} else {
		query()
	}
}

func (a *app) showWords(request *wordSuggestions, snap ui.TextSnapshot, prefix string, items []lsp.CompletionItem, err error) {
	if !request.manual && prefix != "" {
		items = slices.DeleteFunc(items, func(item lsp.CompletionItem) bool {
			filter := strings.TrimLeftFunc(cmp.Or(item.FilterText, item.Label), func(char rune) bool { return !isWordRune(char) })
			_, matches := completionRemainder(filter, prefix)
			return !matches
		})
	}
	if len(items) == 0 {
		items = documentWords(snap, request.selection[1], prefix)
	}
	if len(items) == 0 {
		a.dismissWordSuggestions()
		if request.manual {
			if err != nil {
				a.fail("%s", locales.LspFailed(firstLine(err.Error())))
			} else {
				a.tell(note{message: locales.CompletionNone()})
			}
		}
		return
	}
	slices.SortStableFunc(items, func(left, right lsp.CompletionItem) int { return strings.Compare(sortKey(left), sortKey(right)) })
	request.items = items[:min(len(items), 500)]
	a.dismissInlineCompletion()
	if !request.manual {
		return
	}
	a.dismissWordSuggestions()
	picks := make([]pickItem, len(request.items))
	for index, item := range request.items {
		picks[index] = pickItem{Label: item.Label, Description: item.Detail, Icon: completionIcon(item.Kind)}
	}
	a.openPluginPicker(picks, locales.CompletionPlaceholder(), func(index int) { request.accept(a, index) })
}

func (request *wordSuggestions) accept(a *app, index int) {
	if index < 0 || index >= len(request.items) || !request.current(a) {
		return
	}
	a.queueEdit(request.editor, func(doc *editor) {
		if request.current(a) {
			applyCompletion(doc, request.items[index])
		}
	})
}

func (a *app) wordSuggestionsInput(doc *editor, event ui.InputEvent) bool {
	request := a.wordSuggest
	if request == nil || request.editor != doc {
		return false
	}
	visible := len(request.items) > 0 && request.current(a)
	switch event.Kind {
	case ui.InputKeyDown:
		if visible && event.Mods == 0 {
			switch event.Key {
			case ui.KeyUp:
				request.selected = max(0, request.selected-1)
			case ui.KeyDown:
				request.selected = min(len(request.items)-1, request.selected+1)
			case ui.KeyTab:
				request.accept(a, request.selected)
				a.dismissWordSuggestions()
			case ui.KeyEscape:
				a.dismissWordSuggestions()
			default:
				a.dismissWordSuggestions()
				return false
			}
			a.invalidate()
			return true
		}
		a.dismissWordSuggestions()
	case ui.InputText, ui.InputCompose, ui.InputSelect, ui.InputCommand, ui.InputPointerDown, ui.InputScroll, ui.InputZoom:
		a.dismissWordSuggestions()
	}
	return false
}

package main

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"fiveleaf/internal/linediff"
	"fiveleaf/internal/remote"
	"fiveleaf/locales"
)

type agentContextSource struct {
	tab  *colTab
	root string
}

func (a *app) rememberAgentContextSource(t *colTab) {
	if t != nil && (t.ed != nil || t.diff != nil || t.view != nil) {
		a.agentContextSource = agentContextSource{tab: t, root: a.root}
	}
}

func (a *app) agentContextTab() *colTab {
	if t := a.activeTab(); t != nil && (t.ed != nil || t.diff != nil || t.view != nil) {
		return t
	}
	source := a.agentContextSource
	if source.root == a.root {
		if col, _ := a.locate(source.tab); col != nil {
			return source.tab
		}
	}
	return nil
}

func (a *app) appendAgentContext(text string) bool {
	a.showAgent()
	return a.appendChatContext(a.agent, text)
}

func (a *app) appendChatContext(s *agentChat, text string, refs ...chatDraftRef) bool {
	if text == "" {
		return false
	}
	if len(text)+s.sources.draft.promptSize(s.draft)+2 > agentTextLimit {
		s.err = locales.AgentContextTooLarge()
		return false
	}
	if s.sources.draft.sync(s.draft) || len(refs) != 0 {
		s.sources.valid = false
	}
	if s.draft != "" {
		s.draft += "\n\n"
	}
	offset := len(s.draft)
	s.draft += text
	s.sources.draft.append(s.draft, offset, refs)
	return true
}

func (a *app) appendChatFileContext(chat *agentChat, name, path string, from, to int, content string) {
	if from == 0 {
		chat.attachFile(chatTextAttachment(name, path, content))
		return
	}
	text := agentContextText(name, from, to, content)
	end := len(name) + strings.IndexByte(text[len(name):], '\n')
	source := chatDetailSource{label: filepath.Base(name) + text[len(name):end], path: path}
	a.appendChatContext(chat, text, chatDraftRef{chatDetailSource: source, end: end + 1})
}

func (chat *agentChat) attachFile(ref chatDraftRef) {
	draft := &chat.sources.draft
	if draft.sync(chat.draft) {
		chat.sources.valid = false
	}
	size := draft.promptSize(chat.draft)
	if index := draft.fileIndex(ref.file); index >= 0 {
		size -= len(draft.refs[index].content)
	} else if size > 0 {
		size += 2
	}
	if size+len(ref.content) > agentTextLimit {
		chat.err = locales.AgentContextTooLarge()
		if ref.temporary {
			chat.sources.removePastedImage(ref.file)
		}
		return
	}
	if ref.temporary {
		if chat.sources.pastedImages == nil {
			chat.sources.pastedImages = make(map[string]bool)
		}
		chat.sources.pastedImages[ref.file] = false
	}
	draft.append(chat.draft, 0, []chatDraftRef{ref})
	chat.sources.valid = false
}

func (a *app) addAgentSelection() {
	a.addSelectionToChat(a.agent, a.agentContextTab())
}

func (a *app) addSelectionToChat(chat *agentChat, t *colTab) {
	if col, _ := a.locate(t); t == nil || col == nil {
		return
	}
	a.setCurrentChat(chat)
	a.showAgent()
	if t.diff != nil {
		lo, hi := t.diff.selection()
		if lo >= 0 {
			a.appendChatReview(chat, t.diff.excerpts(lo, hi))
		}
		return
	}
	d := t.ed
	if d == nil || d.unread() {
		return
	}
	snap := d.buf.Snapshot()
	lo, hi := max(0, min(d.sel[0], d.sel[1], snap.Len())), max(0, min(max(d.sel[0], d.sel[1]), snap.Len()))
	if lo == hi {
		lo, hi = snap.LineRange(snap.LineAt(lo))
	}
	name := d.path
	if name == "" {
		name = d.name() + " (unsaved)"
	}
	last := snap.LineAt(hi)
	if start, _ := snap.LineRange(last); hi > lo && start == hi {
		last--
	}
	path := d.path
	if !chatLocalFile(path) {
		path = ""
	}
	a.appendChatFileContext(chat, name, path, snap.LineAt(lo)+1, last+1, snap.Slice(lo, hi))
}

func (a *app) addAgentFile(path string) {
	if path == "" {
		if t := a.agentContextTab(); t != nil {
			path = t.path()
		}
	}
	a.addFileToChat(a.agent, path)
}

func (a *app) addFileToChat(s *agentChat, path string) {
	a.addChatFiles(s, []string{path}, false)
}

func (a *app) addChatFiles(s *agentChat, paths []string, picked bool) {
	paths = slices.DeleteFunc(slices.Clone(paths), func(path string) bool {
		return path == "" || picked && (!filepath.IsAbs(path) || remote.Is(path))
	})
	if s == nil || len(paths) == 0 {
		return
	}
	a.setCurrentChat(s)
	a.showAgent()
	var next func(*app)
	next = func(owner *app) {
		for len(paths) > 0 {
			path := paths[0]
			paths = paths[1:]
			if load := owner.prepareChatAttachment(s, path, picked); load != nil {
				owner.loadChatAttachment(s, load, next)
				return
			}
		}
	}
	next(a)
}

func (a *app) prepareChatAttachment(chat *agentChat, path string, picked bool) func() (chatDraftRef, error) {
	root := a.root
	readDisk := func() (chatDraftRef, error) { return readChatAttachment(root, path) }
	imageFile := isChatImage(path)
	if imageFile && !strings.EqualFold(filepath.Ext(path), ".svg") || chatDocumentType(strings.ToLower(filepath.Ext(path))) {
		return readDisk
	}
	doc := a.agentDocument(path)
	if doc == nil || doc.unread() || imageFile && !doc.dirty() {
		return readDisk
	}
	local := path
	if picked {
		if chatWorkspaceFile(a.root, path) == "" {
			return readDisk
		}
	} else if !chatLocalFile(path) {
		local = ""
	}
	if imageFile {
		body := doc.buf.String()
		return func() (chatDraftRef, error) {
			if len(body) > agentTextLimit {
				return chatDraftRef{}, errAgentFileTooLarge
			}
			ref := chatTextAttachment(path, local, body)
			ref.preview = decodeChatImage([]byte(body), ".svg")
			return ref, nil
		}
	}
	a.appendChatFileContext(chat, path, local, 0, 0, doc.buf.String())
	return nil
}

func (a *app) loadChatAttachment(s *agentChat, read func() (chatDraftRef, error), next func(*app)) {
	root, tree := a.root, a.tree
	h := a.chatHandler(s)
	s.contextLoading++
	go func() {
		ref, err := read()
		h.dispatch(func(owner *app) {
			s.contextLoading--
			current := owner.root == root && !owner.loadsClosed && (owner != a || owner.tree == tree)
			if !current || owner.agentChats[agentChatKey(s)] != s && !owner.retainsChat(func(chat *agentChat) bool { return chat == s }) {
				if ref.temporary {
					s.sources.removePastedImage(ref.file)
				}
				return
			}
			if err != nil {
				s.err = err.Error()
			} else {
				s.attachFile(ref)
			}
			if next != nil {
				next(owner)
			}
		})
	}()
}

func (a *app) addAgentReview() {
	a.addReviewToChat(a.agent)
}

func (a *app) addReviewToChat(chat *agentChat) {
	a.setCurrentChat(chat)
	a.showAgent()
	a.keepDraft()
	if len(a.review.notes) == 0 {
		return
	}
	for _, note := range a.review.notes {
		a.followNote(note)
	}
	if a.appendChatReview(chat, a.review.notes) {
		a.dropReview()
	}
}

func (a *app) appendChatReview(chat *agentChat, notes []*reviewNote) bool {
	var text strings.Builder
	var refs []chatDraftRef
	for index, note := range notes {
		if index > 0 {
			text.WriteString("\n\n")
		}
		if len(notes) > 1 {
			fmt.Fprintf(&text, "%d. ", index+1)
		}
		start := text.Len()
		text.WriteString(formatReview([]*reviewNote{note}, a.relPath))
		source := chatDetailSource{label: note.ref(filepath.Base)}
		if !strings.HasPrefix(note.commit, "agent:") {
			source.path = chatWorkspaceFile(a.root, note.path)
		}
		refs = append(refs, chatDraftRef{chatDetailSource: source, start: start, end: start + len(note.ref(a.relPath)) + 1})
	}
	return a.appendChatContext(chat, text.String(), refs...)
}

// A tool diff is an immutable before/after snapshot. It can be reviewed even
// without Git, and never exposes stage/revert actions on an unrelated worktree.
func (a *app) reviewAgentChange(change agentChange) {
	if len(change.oldText)+len(change.newText) > 2*agentTextLimit {
		return
	}
	if a.window.Detached {
		a.host.inWorkspace(func(workspace *app) { workspace.reviewAgentChange(change) })
		return
	}
	v := newDiffView(parseDiffLayout(a.settings.DiffLayout))
	v.snapshot = "agent:" + rand.Text()
	v.snapshotName = locales.AgentReviewChange() + ": " + filepath.Base(change.path)
	v.files, v.near = []*diffFile{agentSnapshotFile(change)}, [2]int{0, 1}
	v.changed()
	col := a.column()
	at := min(col.active+1, len(col.tabs))
	a.addTab(col, at, &colTab{diff: v})
	a.activate(col, at, true)
}

func agentSnapshotFile(change agentChange) *diffFile {
	f := &diffFile{path: change.path, base: diffSnapshot, loaded: true, letter: 'M', group: groupChanges}
	old, now := splitLines(change.oldText, change.existed), splitLines(change.newText, true)
	setDiffSide(&f.old, diffSide{text: change.oldText, ok: change.existed}, old)
	setDiffSide(&f.new, diffSide{text: change.newText, ok: true}, now)
	f.hunks = linediff.Hunks(old, now)
	f.unified, f.pairs = lineRows(old, now, f.hunks)
	for _, text := range append(append([]string{}, old...), now...) {
		shown, _ := expandTabs(clipLine(text))
		f.widest = max(f.widest, len([]rune(shown)))
	}
	return f
}

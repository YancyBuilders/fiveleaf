package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fiveleaf/internal/remote"
	sdk "github.com/coder/acp-go-sdk"
	"github.com/egoist/mygo/ui"
)

type chatDetailSource struct {
	label, path string
	preview     ui.ImageSource
}

type chatDraftRef struct {
	chatDetailSource
	start, end    int
	file, content string
	previewText   string
	bytes         int64
	temporary     bool
}

type chatSourceDraft struct {
	text   string
	refs   []chatDraftRef
	pastes []*chatDraftPaste
}

type chatSourceState struct {
	draft, pending chatSourceDraft
	pastedImages   map[string]bool
	sent           bool
	history        []chatSourceEntry
	cached         []chatDetailSource
	valid          bool
	imported       *sessionContinuation
}

type chatSourceEntry struct {
	tool *agentTool
	rows []chatDetailSource
}

// Keep only reference headers outside the edited byte range. Text typed or
// pasted into the composer cannot itself create an openable source.
func (draft *chatSourceDraft) sync(text string) bool {
	if len(draft.refs) == 0 || draft.text == text {
		return false
	}
	if !slices.ContainsFunc(draft.refs, func(ref chatDraftRef) bool { return ref.file == "" }) {
		draft.text = text
		return false
	}
	old := draft.text
	prefix, suffix, limit := 0, 0, min(len(old), len(text))
	for prefix < limit && old[prefix] == text[prefix] {
		prefix++
	}
	for suffix < limit && old[len(old)-1-suffix] == text[len(text)-1-suffix] {
		suffix++
	}
	kept := draft.refs[:0]
	for _, ref := range draft.refs {
		switch {
		case ref.file != "":
		case ref.end <= prefix:
		case ref.start >= len(old)-suffix:
			ref.start += len(text) - len(old)
			ref.end += len(text) - len(old)
		default:
			continue
		}
		kept = append(kept, ref)
	}
	removed := len(kept) != len(draft.refs)
	clear(draft.refs[len(kept):])
	if len(kept) == 0 {
		draft.text, draft.refs = "", nil
	} else {
		draft.text, draft.refs = text, kept
	}
	return removed
}

func (draft *chatSourceDraft) append(text string, offset int, refs []chatDraftRef) {
	for _, ref := range refs {
		if ref.file == "" {
			ref.start += offset
			ref.end += offset
		} else if index := draft.fileIndex(ref.file); index >= 0 {
			draft.refs[index] = ref
			continue
		}
		draft.refs = append(draft.refs, ref)
	}
	if len(draft.refs) != 0 {
		draft.text = text
	}
}

func (draft *chatSourceDraft) fileIndex(name string) int {
	return slices.IndexFunc(draft.refs, func(ref chatDraftRef) bool { return ref.file == name })
}

func (draft *chatSourceDraft) hasFiles() bool {
	return slices.ContainsFunc(draft.refs, func(ref chatDraftRef) bool { return ref.file != "" })
}

func (draft *chatSourceDraft) hasAttachments() bool {
	return len(draft.pastes) > 0 || draft.hasFiles()
}

func (draft *chatSourceDraft) promptSize(text string) int {
	size := len(text)
	for _, paste := range draft.pastes {
		if size > 0 {
			size += 2
		}
		size += len(paste.text)
	}
	for _, ref := range draft.refs {
		if ref.file != "" {
			if size > 0 {
				size += 2
			}
			size += len(ref.content)
		}
	}
	return size
}

func (chat *agentChat) draftPrompt() string {
	return chat.sources.draft.promptText(chat.draft)
}

func (draft *chatSourceDraft) promptText(text string) string {
	if !draft.hasAttachments() {
		return text
	}
	var prompt strings.Builder
	prompt.Grow(draft.promptSize(text))
	prompt.WriteString(text)
	appendBlock := func(block string) {
		if prompt.Len() > 0 {
			prompt.WriteString("\n\n")
		}
		prompt.WriteString(block)
	}
	for _, paste := range draft.pastes {
		appendBlock(paste.text)
	}
	for _, ref := range draft.refs {
		if ref.file != "" {
			appendBlock(ref.content)
		}
	}
	return prompt.String()
}

func (chat *agentChat) hasDraft() bool {
	return strings.TrimSpace(chat.draft) != "" || chat.sources.draft.hasAttachments()
}

func (chat *agentChat) removeDraftFile(name string) {
	draft := &chat.sources.draft
	if index := draft.fileIndex(name); index >= 0 {
		draft.refs = slices.Delete(draft.refs, index, index+1)
		if len(draft.refs) == 0 {
			draft.text = ""
		}
		chat.sources.valid = false
		chat.removeUnusedPastedImage(name)
	}
}

func (chat *agentChat) removeUnusedPastedImage(name string) {
	state := &chat.sources
	if sent, owned := state.pastedImages[name]; owned && !sent && state.draft.fileIndex(name) < 0 && state.pending.fileIndex(name) < 0 &&
		(chat.queued == nil || chat.queued.draft.fileIndex(name) < 0) {
		state.removePastedImage(name)
	}
}

func (state *chatSourceState) removePastedImage(path string) {
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		delete(state.pastedImages, path)
	}
}

func (chat *agentChat) stageDetailSources() {
	chat.sources.pending, chat.sources.sent = chat.takeChatDraft(), false
}

func (chat *agentChat) takeChatDraft() chatSourceDraft {
	state := &chat.sources
	state.draft.sync(chat.draft)
	state.draft.text = chat.draft
	draft := state.draft
	state.draft = chatSourceDraft{}
	chat.draft, chat.expandedPaste = "", nil
	state.valid = false
	return draft
}

func (chat *agentChat) recordDetailSources() {
	state := &chat.sources
	if len(state.pending.refs) != 0 {
		var rows []chatDetailSource
		for _, ref := range slices.Backward(state.pending.refs) {
			rows = append(rows, ref.chatDetailSource)
			if _, owned := state.pastedImages[ref.file]; owned {
				state.pastedImages[ref.file] = true
			}
		}
		state.history = append(state.history, chatSourceEntry{rows: rows})
	}
	state.sent, state.valid = true, false
}

func (chat *agentChat) restoreDetailSources(prompt, next, combined string) {
	state := &chat.sources
	state.draft.sync(next)
	restored := state.pending
	if restored.text != prompt {
		restored = chatSourceDraft{}
	}
	restored.append(combined, len(combined)-len(next), state.draft.refs)
	restored.pastes = append(slices.Clone(restored.pastes), state.draft.pastes...)
	state.draft, state.pending, state.sent, state.valid = restored, chatSourceDraft{}, false, false
}

type chatToolSources struct {
	locations, content []chatDetailSource
}

func chatToolSourcePatch(locations []sdk.ToolCallLocation, contents []sdk.ToolCallContent) chatToolSources {
	var patch chatToolSources
	if locations != nil {
		patch.locations = make([]chatDetailSource, 0, len(locations))
		for _, location := range locations {
			if location.Path != "" {
				patch.locations = append(patch.locations, chatDetailSource{label: location.Path})
			}
		}
	}
	if contents != nil {
		patch.content = make([]chatDetailSource, 0, len(contents))
		for _, part := range contents {
			var label string
			switch {
			case part.Diff != nil:
				label = part.Diff.Path
			case part.Content != nil:
				block := part.Content.Content
				if block.ResourceLink != nil {
					label = block.ResourceLink.Uri
				} else if block.Resource != nil {
					resource := block.Resource.Resource
					if resource.TextResourceContents != nil {
						label = resource.TextResourceContents.Uri
					} else if resource.BlobResourceContents != nil {
						label = resource.BlobResourceContents.Uri
					}
				}
			}
			if label != "" && !agentToolDataURI(label) {
				patch.content = append(patch.content, chatDetailSource{label: label})
			}
		}
	}
	return patch
}

func (state *chatSourceState) updateTool(tool *agentTool, patch chatToolSources) {
	old := tool.sources
	if patch.locations == nil {
		patch.locations = old.locations
	}
	if patch.content == nil {
		patch.content = old.content
	}
	if slices.Equal(old.locations, patch.locations) && slices.Equal(old.content, patch.content) {
		return
	}
	tool.sources = patch
	if len(old.locations)+len(old.content) > 0 {
		index := slices.IndexFunc(state.history, func(entry chatSourceEntry) bool { return entry.tool == tool })
		if index >= 0 {
			state.history = slices.Delete(state.history, index, index+1)
		}
	}
	if len(patch.locations)+len(patch.content) > 0 {
		state.history = append(state.history, chatSourceEntry{tool: tool, rows: slices.Concat(patch.locations, patch.content)})
	}
	state.valid = false
}

// The UI borrows this read-only list. No transcript text or filesystem is
// consulted by the frame; only edits to a tracked draft reconcile its ranges.
func (chat *agentChat) detailSources() []chatDetailSource {
	state := &chat.sources
	removed := state.draft.sync(chat.draft)
	imported := chat.continuation
	if imported != nil && !imported.loaded {
		imported = nil
	}
	if state.valid && !removed && state.imported == imported {
		return state.cached
	}
	var rows []chatDetailSource
	seen := make(map[string]int)
	add := func(source chatDetailSource) {
		if source.label == "" {
			return
		}
		key := source.path
		if key == "" {
			key = source.label
		}
		if index, exists := seen[key]; exists {
			if rows[index].path == "" && source.path != "" {
				rows[index] = source
			}
			if rows[index].preview == nil {
				rows[index].preview = source.preview
			}
			return
		}
		seen[key] = len(rows)
		rows = append(rows, source)
	}
	for _, ref := range slices.Backward(state.draft.refs) {
		add(ref.chatDetailSource)
	}
	if chat.pendingPrompt != "" && !state.sent {
		for _, ref := range slices.Backward(state.pending.refs) {
			add(ref.chatDetailSource)
		}
	}
	for _, entry := range slices.Backward(state.history) {
		for _, source := range entry.rows {
			add(source)
		}
	}
	if imported != nil {
		source := imported.source
		row := chatDetailSource{label: source.Label()}
		if filepath.IsAbs(source.Transcript) && !remote.Is(source.Transcript) {
			row.path = source.Transcript
			if row.label == "" {
				row.label = filepath.Base(row.path)
			}
		}
		add(row)
	}
	state.cached, state.valid, state.imported = rows, true, imported
	return rows
}

func chatLocalFile(path string) bool {
	if !filepath.IsAbs(path) || remote.Is(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func chatWorkspaceFile(root, path string) string {
	if !filepath.IsAbs(root) || remote.Is(root) || path == "" || remote.Is(path) {
		return ""
	}
	if !filepath.IsAbs(path) {
		if !filepath.IsLocal(path) {
			return ""
		}
		path = filepath.Join(root, path)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	fs, rel, err := openAgentRoot(root, canonical)
	if err != nil {
		return ""
	}
	defer fs.Close()
	if info, err := fs.Stat(rel); err == nil && info.Mode().IsRegular() {
		return filepath.Join(root, rel)
	}
	return ""
}

func (a *app) addChatDetailFile(chat *agentChat, path string) {
	a.addChatDetailFiles(chat, []string{path})
}

func (a *app) addChatDetailFiles(chat *agentChat, paths []string) {
	a.addChatFiles(chat, paths, true)
}

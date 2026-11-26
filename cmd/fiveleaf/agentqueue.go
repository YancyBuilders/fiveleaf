package main

import (
	"cmp"
	"strings"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type agentQueuedDraft struct {
	draft                   chatSourceDraft
	generation, turn        uint64
	instructions, sessionID string
}

func (chat *agentChat) canQueueDraft() bool {
	if !chat.busy || !chat.promptWritten || chat.stopping || chat.reconnecting || chat.sessionID == "" ||
		chat.client == nil || chat.queued != nil || chat.pendingQueued || chat.contextLoading != 0 || !chat.hasDraft() {
		return false
	}
	select {
	case <-chat.client.Done():
		return false
	default:
	}
	name, _ := splitSlashLine(chat.draft)
	return !chat.offersSlashSetting(name) && !chat.piTerminalOnly(name)
}

func (a *app) queueChatDraft(chat *agentChat) {
	a.checkAgentConfig()
	if !chat.canQueueDraft() {
		return
	}
	if chat.sources.draft.promptSize(chat.draft) > chat.chatDraftLimit() {
		chat.err = locales.AgentContextTooLarge()
		return
	}
	chat.queued = &agentQueuedDraft{
		draft: chat.takeChatDraft(), generation: chat.generation, turn: chat.turn,
		instructions: chat.instructions(), sessionID: chat.sessionID,
	}
	chat.focusNext = true
}

func (queue *agentQueuedDraft) matches(chat *agentChat) bool {
	return queue.generation == chat.generation && queue.turn == chat.turn && queue.sessionID == chat.sessionID &&
		queue.instructions == chat.instructions()
}

func (a *app) followQueuedChat(chat *agentChat) {
	queue := chat.queued
	if queue == nil || !queue.matches(chat) || a.loadsClosed || chat.requestInFlight() {
		return
	}
	a.checkAgentConfig()
	if chat.client == nil || !queue.matches(chat) || queue.draft.promptSize(queue.draft.text) > chat.chatDraftLimit() {
		return
	}
	select {
	case <-chat.client.Done():
		return
	default:
	}
	defer a.recordTaskChange(chat, chat.taskState())
	chat.queued = nil
	chat.pendingQueued = true
	chat.sources.pending, chat.sources.sent, chat.sources.valid = queue.draft, false, false
	a.promptChat(chat, queue.draft.promptText(queue.draft.text))
}

func (queue *agentQueuedDraft) preview() string {
	visible, clipped := agentVisibleText(queue.draft.text, 240)
	line, _, moreLines := strings.Cut(strings.TrimSpace(visible), "\n")
	if line = strings.TrimSpace(line); line != "" {
		if clipped || moreLines {
			return line + "…"
		}
		return line
	}
	if len(queue.draft.pastes) > 0 {
		return locales.AgentPasteText(queue.draft.pastes[0].lines)
	}
	for _, ref := range queue.draft.refs {
		if ref.label != "" {
			return ref.label
		}
	}
	return ""
}

func (chat *agentChat) queuedEditBlock() string {
	if chat.contextLoading > 0 {
		return locales.AgentLoadingContext()
	}
	if chat.hasDraft() || chat.draft != "" {
		return locales.AgentRestoreFailedDraftHasDraft()
	}
	return ""
}

func (a *app) agentQueueView(c *ui.Context, chat *agentChat) {
	queue := chat.queued
	if queue == nil {
		return
	}
	palette := a.pal
	metrics := palette.metricsOr()
	status := locales.AgentQueuedFollowUp()
	if !chat.busy || chat.stopping || !queue.matches(chat) {
		status = locales.AgentQueuePaused()
	}
	preview := queue.preview()
	ui.Row(c.Key("agent-queued-draft")).FillWidth().MinWidth(0).Gap(metrics.gap).AlignItems(ui.Center).
		Padding(0, metrics.inset).Role(ui.RoleGroup).Label(status).Children(func() {
		ui.Text(c, status).FontSize(metrics.textSmall).TextColor(palette.muted).SingleLine().MinWidth(0).MaxWidthPercent(46).Tooltip(status)
		ui.Text(c, preview).FontSize(metrics.textSmall).TextColor(palette.text).SingleLine().Grow(1).MinWidth(0).Tooltip(preview)
		editLabel := locales.AgentQueueEdit()
		blocked := chat.queuedEditBlock()
		edit := a.iconButtonBase(c.Key("edit"), iconPencil, editLabel, false).Shrink(0).KeepFocus().
			Disabled(blocked != "").Tooltip(cmp.Or(blocked, editLabel))
		if edit.Clicked() {
			a.postChatAction(chat, func() {
				if chat.queued == queue && chat.queuedEditBlock() == "" {
					chat.queued = nil
					chat.sources.draft, chat.sources.valid = queue.draft, false
					chat.draft, chat.focusNext = queue.draft.text, true
				}
			})
		}
		remove := a.iconButton(c.Key("remove"), iconX, locales.AgentQueueRemove(), false).Shrink(0).KeepFocus()
		if remove.Clicked() {
			a.postChatAction(chat, func() {
				if chat.queued == queue {
					chat.queued = nil
					for _, ref := range queue.draft.refs {
						if ref.file != "" {
							chat.removeUnusedPastedImage(ref.file)
						}
					}
				}
			})
		}
	})
}

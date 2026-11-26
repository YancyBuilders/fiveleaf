package main

import (
	"cmp"
	"slices"
	"strings"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type agentTurnFailure struct {
	reason string
	draft  chatSourceDraft
}

func (chat *agentChat) noteEmptyReply() {
	if chat.pendingMessage == nil || len(chat.plan) > 0 {
		return
	}
	for _, message := range slices.Backward(chat.messages) {
		if message == chat.pendingMessage {
			chat.messages = append(chat.messages, &agentMessage{role: "system", text: locales.AgentEmptyReply()})
			return
		}
		if message != nil && (message.role == "tool" || (message.role == "assistant" || message.role == "commentary") && strings.TrimSpace(message.text) != "") {
			return
		}
	}
}

func agentIncompleteReply(stopReason string) string {
	switch stopReason {
	case "max_tokens":
		return locales.AgentReplyTokenLimit()
	case "max_turn_requests":
		return locales.AgentReplyRequestLimit()
	case "refusal":
		return locales.AgentReplyDeclined()
	default:
		return locales.AgentTurnIncomplete()
	}
}

func (chat *agentChat) failedDraftBlockReason() string {
	switch {
	case chat.draft != "" || chat.sources.draft.hasAttachments():
		return locales.AgentRestoreFailedDraftHasDraft()
	case chat.contextLoading > 0:
		return locales.AgentLoadingContext()
	case chat.requestInFlight():
		return locales.AgentWorking()
	default:
		return ""
	}
}

func (a *app) agentTurnFailureView(c *ui.Context, chat *agentChat, failure *agentTurnFailure) {
	metrics := a.pal.metricsOr()
	ui.Column(c).MinWidth(0).Gap(metrics.gap).Margin(0, 0, metrics.pad, 0).Children(func() {
		ui.Text(c.Key("reason"), failure.reason).MinWidth(0).FontSize(metrics.textSmall).TextColor(a.pal.dangerFg).Selectable()
		label := locales.AgentRestoreFailedDraft()
		blocked := chat.failedDraftBlockReason()
		button := actionButton(c.Key("restore-draft"), label, iconUndo).AlignSelf(ui.Start).
			Disabled(blocked != "").Tooltip(cmp.Or(blocked, label))
		if button.Clicked() {
			a.postChatAction(chat, func() {
				if chat.failedDraftBlockReason() == "" {
					draft := failure.draft
					draft.refs = slices.Clone(draft.refs)
					draft.pastes = slices.Clone(draft.pastes)
					chat.sources.draft = draft
					chat.sources.valid = false
					chat.draft, chat.focusNext = draft.text, true
				}
			})
		}
	})
}

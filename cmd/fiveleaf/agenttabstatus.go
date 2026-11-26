package main

import (
	"errors"
	"fmt"
	"log"
	"slices"

	"fiveleaf/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (state agentTaskState) notification() (label, kind string) {
	switch state {
	case agentTaskRunning:
		return locales.AgentTaskRunning(), "progress"
	case agentTaskApproval:
		return locales.AgentTaskApproval(), "approval"
	case agentTaskCompleted:
		return locales.AgentStatusCompleted(), "success"
	case agentTaskFailed:
		return locales.AgentStatusFailed(), "error"
	default:
		return locales.AgentTaskIdle(), "idle"
	}
}

func (state agentTaskState) mark(p *palette) (string, *ui.SVG, ui.Color) {
	label, kind := state.notification()
	icon, ink := notificationMark(kind, p)
	return label, icon, ink
}

func (a *app) chatTabStatus(c *ui.Context, chat *agentChat, surface ui.Color) {
	const size = 14
	state := chat.taskState()
	label, icon, ink := state.mark(a.pal)
	slot := c.Key("task-status")
	var indicator ui.Element
	if state == agentTaskRunning && !c.Preferences().ReduceMotion {
		indicator = ui.Spinner(slot)
	} else {
		indicator = ui.Icon(slot, icon).FontSize(size).Role(ui.RoleImage)
	}
	indicator.Size(size, size).Shrink(0).TextColor(chatBadgeColor(ink, surface)).Label(label).Tooltip(label)
}

func (a *app) showChatTaskNotices(c *ui.Context, foreground bool) {
	for _, chat := range a.allAgentChats() {
		notice := chat.taskNoticePending
		if notice == nil {
			continue
		}
		if foreground && a.chatShown(chat) && chat.connected != nil && chat.connected.config.EffectiveProtocol() == "acp" && (notice.Type == "approval" || notice.Type == "progress") {
			c.CloseToast(notice.ID)
		} else {
			c.AddToast(*notice)
		}
		chat.taskNoticePending = nil
	}
}

func (a *app) openNotifiedChat(chat *agentChat) {
	owner := a
	if chat.events != nil {
		owner = chat.events.window
	}
	if !owner.loadsClosed && slices.Contains(owner.allAgentChats(), chat) {
		owner.host.showApp(owner)
		owner.openChatPage(chat)
		owner.invalidate()
	}
}

type agentTaskNotification struct {
	latest  *mygo.Notification
	sending bool
}

func (a *app) recordTaskChange(chat *agentChat, previous agentTaskState) {
	state := chat.taskState()
	if state == previous {
		return
	}
	status, kind := state.notification()
	toast := ui.Toast{ID: fmt.Sprintf("agent-task:%p", chat), Type: kind, Title: agentConversationTitle(chat), Description: status,
		Timeout: actionNoticeTimeout, Action: locales.AgentViewChat(), OnAction: func() { a.openNotifiedChat(chat) }}
	receipt := a.rememberNotice(locales.PrefSectionAgent(), toast)
	toast.Data = receipt
	chat.taskNoticePending = &toast
	if chat.systemNotice == nil {
		if !mygo.App.IsReady() || !mygo.NotificationsSupported() {
			return
		}
		chat.systemNotice = &agentTaskNotification{}
	}
	notice := chat.systemNotice
	var id string
	if notice.latest != nil {
		id = notice.latest.ID()
	}
	entry := mygo.NewNotification(mygo.NotificationOptions{
		ID: id, Title: status, Body: agentConversationTitle(chat),
		Silent: state != agentTaskApproval && state != agentTaskFailed,
	})
	entry.OnClick(func() {
		receipt.read()
		entry.Close()
		if chat.systemNotice == notice {
			a.openNotifiedChat(chat)
		}
	})
	notice.latest = entry
	if !notice.sending {
		notice.sending = true
		mygo.App.WhenReady(notice.sendLatest)
	}
}

func (notice *agentTaskNotification) sendLatest() {
	entry := notice.latest
	if entry == nil {
		return
	}
	go func() {
		if err := entry.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
			log.Printf("fiveleaf: agent task notification: %v", err)
		}
		mygo.RunOnMain(func() {
			switch notice.latest {
			case nil:
				entry.Close()
			case entry:
				notice.sending = false
			default:
				notice.sendLatest()
			}
		})
	}()
}

func (a *app) discardChat(chat *agentChat, keepDraft bool) {
	a.disconnectChat(chat)
	for path := range chat.sources.pastedImages {
		if keepDraft && chat.sources.draft.fileIndex(path) >= 0 {
			chat.sources.pastedImages[path] = false
		} else {
			chat.sources.removePastedImage(path)
		}
	}
	if notice := chat.systemNotice; notice != nil {
		notice.latest.Close()
		notice.latest = nil
		chat.systemNotice = nil
	}
}

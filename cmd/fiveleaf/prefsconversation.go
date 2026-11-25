package main

import (
	"strconv"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

const prefsConversationKey = "conversationSettings"

type prefsConversation struct {
	project *app
	chat    *agentChat
}

func (target *prefsConversation) available() bool {
	project := target.project
	if project.loadsClosed || target.chat == nil {
		return false
	}
	col, _ := project.chatTab(target.chat)
	return col != nil || project.agent == target.chat
}

func (w *prefsWindow) conversationPane(c *ui.Context, target *prefsConversation) {
	p, chat := w.pal, target.chat
	w.head(c, "conversation-head", iconAgent, locales.AgentConversationSettings(), func() {
		if actionButton(c, locales.AgentConfigure(), iconSettings).Clicked() {
			w.navigate(settings.CatAgent, "agentServers", nil)
		}
	})
	ui.Scroll(c.Key(chat)).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).WidthPercent(100).MaxWidth(prefsPageWidth).AlignSelf(ui.Center).Padding(20, 28).Gap(10).Children(func() {
			ui.Text(c, agentConversationTitle(chat)).FontSize(14).FontWeight(600).TextColor(p.text).SingleLine()
			ui.Text(c, locales.AgentSystemPrompt()).FontSize(12).TextColor(p.muted)
			if !chat.hasRole() {
				ui.TextArea(c.Key("system"), &chat.systemPrompt).Label(locales.AgentSystemPrompt()).Bind(&w.firstH).FillWidth().Height(80)
			} else {
				role, ok := target.project.chatRoleDefinition(chat)
				if ok {
					ui.Text(c, chat.roleLabel()+" · "+target.project.agentRoleDetail(role)).FontSize(12).TextColor(p.text)
					if role.Prompt != "" {
						text, clipped := agentVisibleText(role.Prompt, 32*1024)
						ui.Text(c, text).FontSize(12).TextColor(p.text).Selectable()
						if clipped {
							ui.Text(c, locales.AgentMoreOutput()).FontSize(12).TextColor(p.muted)
						}
					}
				}
				if ok {
					if actionButton(c, locales.PrefsRoleEdit(), iconPencil).Bind(&w.firstH).AlignSelf(ui.Start).Disabled(w.agentConfigurationEditing() || w.agentIOBusy).Clicked() {
						if chat.preset != nil {
							w.editPresetRole(chat.preset)
						} else {
							w.openNamedRole(chat.role, role, true)
						}
						w.navigate(settings.CatAgent, "agentRolePresets", nil)
					}
				} else {
					if actionButton(c, locales.AgentRolesConfigure(), iconSettings).Bind(&w.firstH).AlignSelf(ui.Start).Clicked() {
						w.navigate(settings.CatAgent, "agentRolePresets", nil)
					}
				}
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AgentContextMessages()).FontSize(12).TextColor(p.text).Grow(1)
				ui.Text(c, strconv.Itoa(chat.contextMessages)).FontSize(12).FontFeatures("tnum").TextColor(p.text)
				value := float64(chat.contextMessages)
				if ui.Stepper(c, &value, 1, 100, 1).Label(locales.AgentContextMessages()).Changed() {
					chat.contextMessages = int(value)
				}
			})
			clearLabel := locales.AgentClearChat()
			if chat.historyKey != "" {
				clearLabel = locales.SessionsReturnToChat()
			}
			if actionButton(c, clearLabel, nil).AlignSelf(ui.Start).Disabled(!canClearChat(chat)).Clicked() {
				target.project.post(func() {
					if target.available() && canClearChat(chat) {
						target.project.clearChat(chat)
						if chat.historyKey != "" && target.project.agent != chat {
							target.project.host.showApp(target.project)
						}
					}
				})
			}
		})
	})
}

func canClearChat(chat *agentChat) bool {
	return !chat.requestInFlight() && chat.contextLoading == 0
}

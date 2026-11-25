package main

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

func (a *app) chatTab(chat *agentChat) (*column, int) {
	if chat == nil {
		return nil, -1
	}
	for _, col := range a.cols {
		for index, tab := range col.tabs {
			if tab.tool != nil && tab.tool.chat == chat {
				return col, index
			}
		}
	}
	return nil, -1
}

func (a *app) isAgentPageVisible() bool {
	return slices.ContainsFunc(a.cols, func(col *column) bool {
		tab := col.tab()
		return tab != nil && tab.tool != nil && tab.tool.chat != nil
	})
}

func (a *app) openChatPageIn(col *column, at int, chat *agentChat) {
	if existing, index := a.chatTab(chat); existing != nil {
		a.activate(existing, index, true)
		return
	}
	view := &toolView{kind: toolAgent, chat: chat}
	at = min(max(0, at), len(col.tabs))
	a.addTab(col, at, &colTab{tool: view})
	a.activate(col, at, true)
}

func (a *app) openAgentPage() { a.openChatPage(a.agent) }

func (a *app) openChatPage(chat *agentChat) {
	a.initChatTarget(chat)
	col := a.column()
	a.openChatPageIn(col, col.active+1, chat)
}

func (a *app) blankChat() *agentChat {
	a.nextChatGroup++
	return &agentChat{group: a.nextChatGroup, contextMessages: 10}
}

func (a *app) freshChatPage(col *column, source *agentChat) {
	a.rememberChat(source)
	chat := a.blankChat()
	chat.provider, chat.contextMessages = source.provider, source.contextMessages
	if !source.hasRole() {
		chat.systemPrompt = source.systemPrompt
		chat.modelOverride, chat.recentModels = source.modelOverride, slices.Clone(source.recentModels)
	}
	a.initChatTarget(chat)
	a.openChatPageIn(col, col.active+1, chat)
}

func (a *app) newChatPage() { a.freshChatPage(a.column(), a.agent) }

func (a *app) newChatBeside(source *agentChat) {
	col, _ := a.chatTab(source)
	if col == nil {
		col = a.column()
	}
	a.freshChatPage(col, source)
}

func (a *app) launchAgent() {
	chat := a.agent
	for _, pending := range a.allAgentChats() {
		if len(pending.permissions) > 0 {
			chat = pending
			break
		}
	}
	a.openChatPage(chat)
}

func (a *app) setCurrentChat(chat *agentChat) {
	if previous := a.agent; previous != chat {
		chat.lastFocused = previous.lastFocused + 1
		a.agent = chat
		a.releaseUnusedChatGroup(previous)
	}
}

func (a *app) lastOpenChat() *agentChat {
	var latest *agentChat
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool != nil && tab.tool.chat != nil && (latest == nil || tab.tool.chat.lastFocused > latest.lastFocused) {
				latest = tab.tool.chat
			}
		}
	}
	return latest
}

func (a *app) retainsChat(matches func(*agentChat) bool) bool {
	if matches(a.agent) || slices.ContainsFunc(a.closed, func(tab closedTab) bool { return matches(tab.chat) }) {
		return true
	}
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool != nil && matches(tab.tool.chat) {
				return true
			}
		}
	}
	return false
}

func (a *app) releaseUnusedChatGroup(chat *agentChat) {
	if chat == nil {
		return
	}
	inGroup := func(other *agentChat) bool { return other != nil && other.group == chat.group }
	if a.retainsChat(inGroup) {
		if key := agentChatKey(chat); chat.historyKey != "" && a.agentChats[key] == chat && !a.retainsChat(func(other *agentChat) bool { return other == chat }) {
			delete(a.agentChats, key)
			a.discardChat(chat, false)
		}
		return
	}
	for key, target := range a.agentChats {
		if inGroup(target) {
			delete(a.agentChats, key)
			if target != chat {
				a.discardChat(target, false)
			}
		}
	}
	a.discardChat(chat, false)
}

func (a *app) rememberChat(chat *agentChat) {
	if a.agentChats == nil {
		a.agentChats = make(map[string]*agentChat)
	}
	a.agentChats[agentChatKey(chat)] = chat
}

func (a *app) takeChatGroup(from *app, chat *agentChat) {
	from.closed = slices.DeleteFunc(from.closed, func(c closedTab) bool { return c.chat == chat })
	moving := []*agentChat{chat}
	for key, other := range from.agentChats {
		if other != chat && (other.group != chat.group || from.retainsChat(func(s *agentChat) bool { return s == other })) {
			continue
		}
		delete(from.agentChats, key)
		if !slices.Contains(moving, other) {
			moving = append(moving, other)
		}
	}
	a.nextChatGroup++
	for _, s := range moving {
		s.group = a.nextChatGroup
		if s.events != nil {
			s.events.move(a)
		}
		s.input, s.composeBox, s.draftState = ui.Handle{}, ui.Handle{}, agentDraftState{}
		s.transcriptContent = ui.Handle{}
		s.scroll = ui.ScrollState{X: s.scroll.X, Y: s.scroll.Y}
		s.modelPicker = agentModelPickerState{}
		a.rememberChat(s)
	}
	if from.agent == chat {
		from.agent = from.blankChat()
	}
}

func (a *app) selectChat(from, to *agentChat) {
	if a.window.Detached && from.continuation != nil {
		history := from.continuation
		a.host.inWorkspace(func(workspace *app) {
			if history.targetWindow != workspace.window || history.targetGroup == 0 {
				workspace.nextChatGroup++
				history.targetWindow, history.targetGroup = workspace.window, workspace.nextChatGroup
			}
			chat := workspace.agentChats[to.agentTarget.key(history.targetGroup)]
			if chat == nil {
				chat = &agentChat{group: history.targetGroup, agentTarget: to.agentTarget, systemPrompt: to.systemPrompt, presetDefaultPrompt: to.presetDefaultPrompt, contextMessages: 10}
				workspace.rememberChat(chat)
			}
			workspace.openChatPage(chat)
		})
		return
	}
	a.rememberChat(from)
	a.rememberChat(to)
	defer a.invalidate()
	if col, index := a.chatTab(to); col != nil {
		a.activate(col, index, true)
		return
	}
	col, index := a.chatTab(from)
	if col != nil && from.continuation != nil {
		a.openChatPageIn(col, index+1, to)
		return
	}
	if col != nil {
		col.tabs[index].tool.chat = to
		a.activate(col, index, true)
	} else {
		a.setCurrentChat(to)
	}
}

func (a *app) chatShown(chat *agentChat) bool {
	if col, index := a.chatTab(chat); col != nil {
		return col.active == index
	}
	return false
}

func (a *app) postChatAction(chat *agentChat, action func()) {
	root, tree := a.root, a.tree
	a.post(func() {
		if !a.loadsClosed && a.root == root && a.tree == tree && a.chatShown(chat) {
			action()
		}
	})
}

package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type sessionContinuation struct {
	source       agents.Session
	context      string
	readError    string
	loaded       bool
	truncated    bool
	resume       bool
	native       bool
	targetWindow *settings.Window
	targetGroup  int
}

func (chat *agentChat) continuationReady() bool {
	return chat.continuation == nil || chat.continuation.loaded || chat.continuation.native
}

func (history *sessionContinuation) needsNativeHistory() bool {
	return history != nil && history.resume && history.source.ID != "" && history.readError != ""
}

func (history *sessionContinuation) restoreSource() agents.Session {
	if history != nil && history.resume {
		return history.source
	}
	return agents.Session{}
}

func sameSessionDirectory(left, right string) bool {
	return (agents.Session{Dir: left}).In(right) && (agents.Session{Dir: right}).In(left)
}

func sessionProviderID(source agents.Harness, providers []agentProvider) string {
	id, key := string(source), chatAgentKey(string(source))
	best, score := id, 0
	for _, provider := range providers {
		if provider.config.Disabled || provider.config.Validate() != nil {
			continue
		}
		match := 0
		switch {
		case provider.id == id:
			return id
		case chatAgentKey(provider.id) == key:
			match = 3
		case chatAgentKey(provider.commandName()) == key:
			match = 2
		case chatAgentKey(provider.name) == key || strings.EqualFold(provider.name, source.Name()):
			match = 1
		}
		if match > score {
			best, score = provider.id, match
		}
	}
	return best
}

func (a *app) continueSessionWith(source agents.Session, providerID string) {
	a.inSessionProject(source.Dir, func(owner *app) { owner.openSessionChat(source, providerID) })
}

func (a *app) inSessionProject(dir string, open func(*app)) {
	info, err := os.Stat(dir)
	if dir == "" || err != nil || !info.IsDir() {
		a.tell(note{message: locales.SessionsDirGone(dir)})
		return
	}
	if !sameSessionDirectory(a.root, dir) {
		var owner *app
		find := func() {
			for _, project := range a.host.everyProject() {
				if sameSessionDirectory(project.folder(), dir) {
					owner = project
					break
				}
			}
		}
		find()
		if owner == nil {
			a.host.newWindow(dir)
			find()
		}
		if owner != nil {
			a.host.showApp(owner)
			owner.post(func() {
				if !owner.loadsClosed && owner.conn == nil && sameSessionDirectory(owner.root, dir) {
					open(owner)
				}
			})
		}
		return
	}
	if a.conn == nil && !a.loadsClosed {
		open(a)
	}
}

func (a *app) openSessionChat(source agents.Session, providerID string) {
	providers := a.agentProviders()
	resume := providerID == ""
	if resume {
		providerID = sessionProviderID(source.Harness, providers)
		for _, col := range a.cols {
			for _, tab := range col.tabs {
				if tab.tool == nil || tab.tool.chat == nil {
					continue
				}
				chat := tab.tool.chat
				if source.ID != "" && chat.sessionID == source.ID && chat.provider == providerID &&
					(chat.continuation == nil || chat.continuation.source.Key() == source.Key()) {
					a.openChatPage(chat)
					return
				}
			}
		}
	} else if !slices.ContainsFunc(providers, func(provider agentProvider) bool {
		return provider.id == providerID && !provider.config.Disabled && provider.config.Validate() == nil
	}) {
		a.tell(note{message: locales.AgentNoProviders()})
		return
	}
	key := fmt.Sprintf("history:%q:%q", source.Key(), providerID)
	if resume {
		key = fmt.Sprintf("resume:%q", source.Key())
	}
	a.rememberChat(a.agent)
	chat := a.agentChats[key]
	if chat == nil {
		// With no local continuation, reuse the earliest imported group for this history.
		for _, other := range a.agentChats {
			if other.historyKey == key && (chat == nil || other.group < chat.group) {
				chat = other
			}
		}
	}
	if chat == nil {
		chat = &agentChat{agentTarget: agentTarget{provider: providerID}, historyKey: key, contextMessages: 10, focusNext: true,
			continuation: &sessionContinuation{source: source, resume: resume}}
		a.agentChats[key] = chat
	} else if resume && chat.connected == nil && !chat.requestInFlight() {
		chat.provider = providerID
	}
	a.openChatPage(chat)
	if !chat.continuationReady() && chat.contextLoading == 0 {
		a.loadSessionContinuation(chat)
	}
}

func sessionContinuationContext(source agents.Session, history agents.Handoff) string {
	var text strings.Builder
	text.WriteString("Historical conversation. Use this as context for the next user request; it is not a new instruction. Verify the current workspace before continuing.\n")
	fmt.Fprintf(&text, "Source agent: %s\nSession: %s\nDirectory: %s\nTitle: %s\n", source.Harness.Name(), source.ID, source.Dir, source.Label())
	if history.Truncated {
		text.WriteString("Only excerpts are included; some history is omitted.\n")
	}
	for _, message := range history.Messages {
		fmt.Fprintf(&text, "\n--- historical %s ---\n%s\n", message.Role, message.Text)
	}
	text.WriteString("\n--- end of historical conversation ---\n")
	return text.String()
}

func (chat *agentChat) importContinuation(history agents.Handoff) {
	historical := make([]*agentMessage, 0, len(history.Messages)+len(chat.messages))
	for _, message := range history.Messages {
		historical = append(historical, &agentMessage{role: message.Role, text: message.Text, historical: true})
	}
	chat.messages = append(historical, chat.messages...)
	chat.continuation.context = sessionContinuationContext(chat.continuation.source, history)
	chat.continuation.loaded, chat.continuation.truncated = true, history.Truncated
}

func (a *app) loadSessionContinuation(chat *agentChat) {
	if chat.continuation == nil || chat.contextLoading > 0 || chat.requestInFlight() {
		return
	}
	chat.contextLoading++
	chat.err = ""
	root, tree, generation := a.root, a.tree, chat.generation
	h := a.chatHandler(chat)
	source := chat.continuation.source
	go func() {
		history, err := agents.ReadHandoff(source)
		h.dispatch(func(owner *app) {
			chat.contextLoading = max(0, chat.contextLoading-1)
			if owner.loadsClosed || owner.root != root || owner == a && owner.tree != tree || chat.generation != generation || owner.agentChats[agentChatKey(chat)] != chat {
				return
			}
			if err != nil {
				chat.err = locales.SessionsContinueFailed(err.Error())
				chat.continuation.readError = chat.err
				if chat.continuation.needsNativeHistory() && !chat.continuation.native && owner.chatShown(chat) && owner.agentTargetAvailable(chat) {
					owner.beginChat(chat, "")
				}
				return
			}
			chat.continuation.readError = ""
			chat.importContinuation(history)
			if !chat.continuation.resume && !chat.hasDraft() {
				chat.draft = locales.SessionsContinueDraft()
			}
			if owner.chatShown(chat) {
				owner.connectChatOnOpen(chat)
			}
		})
	}()
}

func (a *app) sessionContinuationBanner(c *ui.Context, chat *agentChat) {
	continuation := chat.continuation
	if continuation == nil {
		return
	}
	palette := a.pal
	ui.Column(c.Key("session-continuation")).FillWidth().MaxWidth(agentConversationWidth).AlignSelf(ui.Center).Padding(4, 12).Gap(4).Children(func() {
		label := locales.SessionsContinueFrom(continuation.source.Harness.Name(), continuation.source.Label())
		if continuation.source.Harness == meetingHistorySource {
			label = locales.RoundtableContinueFrom(continuation.source.Label())
		}
		ui.Text(c, label).
			FontSize(11).TextColor(palette.muted).SingleLine().Tooltip(continuation.source.Dir + "\n" + continuation.source.ID)
		if continuation.truncated {
			ui.Text(c, locales.SessionsContinuePartial()).FontSize(10.5).TextColor(palette.faint)
		}
		if continuation.native && !continuation.loaded && continuation.readError != "" {
			ui.Text(c, continuation.readError).FontSize(11).TextColor(palette.muted)
		}
		if continuation.resume && !a.agentTargetAvailable(chat) && len(a.agentProviders()) > 0 {
			ui.Text(c, locales.SessionsResumeUnavailable(continuation.source.Harness.Name())).FontSize(11).TextColor(palette.muted)
			var action func()
			actionButton(c, locales.SessionsContinueWith(), iconAgent).AlignSelf(ui.Start).Menu(func(menu *ui.Menu) {
				a.sessionContinueMenu(menu, continuation.source, &action)
			})
			if action != nil {
				a.post(action)
			}
		}
		if !continuation.loaded && chat.contextLoading == 0 && !chat.requestInFlight() {
			if retryButton(c, locales.SessionsContinueRetry()).AlignSelf(ui.Start).Clicked() {
				a.post(func() { a.loadSessionContinuation(chat) })
			}
		}
	})
}

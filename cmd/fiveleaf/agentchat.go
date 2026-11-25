package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const agentTextLimit = 2 << 20

type agentChoice struct{ id, name string }
type agentPlan struct{ content, status string }
type agentMessage struct {
	role, text string
	title      string
	historical bool
	render     viewNode
	// Tool entries share the tool list's identity so updates stay in place.
	tool                   *agentTool
	expanded, expansionSet bool
	observedText           string
}
type agentChange struct {
	path, oldText, newText string
	existed                bool
}
type agentLocation struct {
	Path string
	Line *int
}
type agentTool struct {
	id, title, kind, status, text string
	note                          string
	expanded, expansionSet        bool
	changes                       []agentChange
	rawInput                      json.RawMessage
	locations                     []agentLocation
	sources                       chatToolSources
	sessionID                     string
	generation                    uint64
	observedText, observedStatus  string
}
type agentPermission struct {
	title                  string
	detail                 string
	preview                permissionPreview
	expanded, expansionSet bool
	detailFromTool         bool
	toolID, sessionID      string
	generation             uint64
	options                []acp.PermissionOption
	choose                 func(string)
}
type agentProvider struct {
	id, name, plugin string
	config           settings.AgentServer
}

type agentDraftState struct {
	id        uint64
	build     int
	text      string
	selection [2]int
}

type agentTaskState uint8

const (
	agentTaskIdle agentTaskState = iota
	agentTaskRunning
	agentTaskApproval
	agentTaskCompleted
	agentTaskFailed
)

// agentChat belongs to one workspace. All fields are confined to the UI thread;
// a generation prevents callbacks from a closed connection reaching its successor.
type agentChat struct {
	agentTarget
	events                                  *tabEvents
	draft, err, sessionID, mode             string
	adapterName                             string
	group                                   int
	lastFocused                             uint64
	historyKey                              string
	continuation                            *sessionContinuation
	modelOverride, currentModel             string
	models                                  []acp.ModelChoice
	commands                                []acp.Command
	modelPicker                             agentModelPickerState
	recentModels                            []string
	connecting, configuring, busy, stopping bool
	connectionAttempted                     bool
	messages                                []*agentMessage
	tools                                   []*agentTool
	permissions                             []*agentPermission
	modes, authMethods                      []agentChoice
	thinkingModes                           bool
	genericMode                             string
	genericModes                            []agentChoice
	plan                                    []agentPlan
	planExpanded, planExpansionSet          bool
	scroll                                  ui.ScrollState
	readingPrompt                           *agentMessage
	readingY                                float32
	details                                 chatDetailsState
	input                                   ui.Handle
	draftState                              agentDraftState
	composeBox                              ui.Handle
	transcriptContent                       ui.Handle
	focusNext                               bool
	client                                  *acp.Client
	ctx                                     context.Context
	cancel                                  context.CancelFunc
	generation                              uint64
	connected                               *agentProvider
	connectedRole                           *settings.AgentRole
	pendingPrompt                           string
	sources                                 chatSourceState
	lastTask                                agentTaskState
	taskNoticePending                       *ui.Toast
	systemNotice                            *agentTaskNotification
	contextLoading                          int
	turn                                    uint64
	systemPrompt, presetDefaultPrompt       string
	contextMessages                         int
	contextUsage                            *acp.UsageState
	contextDetailsOpen                      bool
	contextPolicy                           agentCompactionPolicy
	contextAccounting                       agentContextAccounting
	replyCancel                             context.CancelFunc
	observedMessages, observedTools         int
	observedPermissions                     int
	observedBusy                            bool
}

func (s *agentChat) requestInFlight() bool {
	return s.connecting || s.configuring || s.busy
}

func (s *agentChat) taskState() agentTaskState {
	switch {
	case len(s.permissions) > 0:
		return agentTaskApproval
	case s.busy || s.pendingPrompt != "":
		return agentTaskRunning
	default:
		return s.lastTask
	}
}

func (a *app) agentProviders() []agentProvider {
	var out []agentProvider
	for _, id := range slices.Sorted(maps.Keys(a.settings.AgentServers)) {
		c := a.settings.AgentServers[id]
		name := c.Name
		if name == "" {
			name = id
		}
		out = append(out, agentProvider{id: id, name: name, config: c})
	}
	if plugins != nil {
		for _, p := range plugins.enabled() {
			for _, c := range p.Contributes.AgentServers {
				if slices.ContainsFunc(out, func(o agentProvider) bool { return o.id == c.ID }) {
					continue
				}
				out = append(out, agentProvider{id: c.ID, name: cmp.Or(c.Name, c.ID), plugin: p.Name, config: settings.AgentServer{
					Name: c.Name, Command: p.AgentCommand(c.Command), Args: c.Args, Env: c.Env,
					Protocol: c.Protocol, WorkingDirectory: c.WorkingDirectory, Capabilities: c.Capabilities,
					Disabled: c.Disabled, TimeoutSeconds: c.TimeoutSeconds, PromptArgument: c.PromptArgument,
				}})
			}
		}
	}
	return out
}

func (a *app) showAgent() {
	a.openAgentPage()
}

func (a *app) initChatTarget(chat *agentChat) {
	if chat.provider == "" && !chat.hasRole() {
		for _, provider := range a.agentProviders() {
			if !provider.config.Disabled {
				chat.provider = provider.id
				break
			}
		}
	}
}

func (a *app) selectAgent(id string) {
	a.selectChatAgent(a.agent, id)
}

func (a *app) selectChatAgent(from *agentChat, id string) {
	a.selectAgentTarget(from, agentTarget{provider: id})
}

func (a *app) newAgentSession() {
	a.clearChat(a.agent)
}

func (a *app) clearChat(s *agentChat) {
	if s.contextLoading > 0 {
		return
	}
	if s.historyKey != "" {
		a.selectChatAgent(s, s.provider)
		return
	}
	a.discardChat(s, true)
	*s = agentChat{events: s.events, group: s.group, lastFocused: s.lastFocused, agentTarget: s.agentTarget, draft: s.draft, generation: s.generation, focusNext: true,
		sources:      chatSourceState{draft: s.sources.draft, pastedImages: s.sources.pastedImages},
		systemPrompt: s.systemPrompt, presetDefaultPrompt: s.presetDefaultPrompt, contextMessages: s.contextMessages, modelOverride: s.modelOverride, recentModels: s.recentModels}
}

func (a *app) disconnectAgent() { a.disconnectChat(a.agent) }

func (a *app) disconnectChat(s *agentChat) {
	defer a.recordTaskChange(s, s.taskState())
	s.restorePendingTask(agentTaskFailed)
	s.generation++
	if s.replyCancel != nil {
		s.replyCancel()
		s.replyCancel = nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.client != nil {
		client := s.client
		s.client = nil
		go client.Close()
	}
	s.connecting, s.configuring, s.busy, s.stopping = false, false, false, false
	s.sessionID, s.mode, s.adapterName = "", "", ""
	if s.continuation != nil {
		s.continuation.native = false
	}
	s.contextUsage = nil
	s.contextDetailsOpen = false
	s.contextPolicy = agentCompactionPolicy{}
	s.contextAccounting = agentContextAccounting{}
	s.permissions, s.modes, s.authMethods, s.connected = nil, nil, nil, nil
	s.thinkingModes = false
	s.genericMode, s.genericModes = "", nil
	s.currentModel, s.models = "", nil
	s.commands = nil
	s.modelPicker = agentModelPickerState{}
	s.connectedRole = nil
}

// checkAgentConfig is cheap while disconnected. An enabled plugin can disappear
// while the sidebar is hidden, so the workspace frame checks its active definition.
func (a *app) checkAgentConfig() {
	providers := a.agentProviders()
	if pending := a.inlineCopilot; pending != nil && (!pending.sameTarget(a) || pending.request != nil && !pending.request.sameProvider(providers) || pending.text == "" && !pending.samePosition()) {
		a.dismissInlineCompletion()
	}
	if request := a.copilot; request != nil && !request.current(a, providers) {
		a.cancelCopilot()
	}
	for _, s := range a.allAgentChats() {
		if s.sources.draft.sync(s.draft) {
			s.sources.valid = false
		}
		if old := s.connected; old != nil {
			match := slices.ContainsFunc(providers, func(p agentProvider) bool {
				return p.id == old.id && !p.config.Disabled && reflect.DeepEqual(p.config, old.config)
			})
			if s.hasRole() {
				role, exists := a.chatRoleDefinition(s)
				match = match && exists && s.connectedRole != nil && role == *s.connectedRole && role.Validate() == nil
			}
			if !match {
				a.disconnectChat(s)
				s.err = locales.AgentConfigurationChanged()
			}
		}
		if s.role != "" && s.connected == nil {
			if role, exists := a.chatRoleDefinition(s); exists {
				s.provider, s.systemPrompt = role.Agent, role.Prompt
			}
		} else if s.preset != nil && s.connected == nil {
			if role, _ := a.chatRoleDefinition(s); role.Agent == s.provider {
				s.systemPrompt = role.Prompt
			}
		}
	}
	a.checkMeetingConfig(providers)
}

func (a *app) connectAgent() { a.beginAgent("") }

func (a *app) beginAgent(prompt string) { a.beginChat(a.agent, prompt) }

func (a *app) connectChatOnOpen(s *agentChat) {
	if s.connectionAttempted || s.client != nil || s.connected != nil || s.requestInFlight() || (!s.continuationReady() && !s.continuation.needsNativeHistory()) ||
		a.loadsClosed || a.root == "" || a.conn != nil || remote.Is(a.root) {
		return
	}
	a.initChatTarget(s)
	if a.agentTargetAvailable(s) {
		a.beginChat(s, "")
	}
}

func (a *app) beginChat(s *agentChat, prompt string) {
	if s.requestInFlight() || (prompt != "" && s.contextLoading > 0) || (!s.continuationReady() && (prompt != "" || !s.continuation.needsNativeHistory())) {
		return
	}
	if a.root == "" || a.conn != nil || remote.Is(a.root) {
		s.err = locales.AgentRemoteUnavailable()
		return
	}
	s.connectionAttempted = true
	var p agentProvider
	var role *settings.AgentRole
	if s.hasRole() {
		resolved, provider, err := a.resolveAgentRole(s)
		if err != nil {
			s.err = err.Error()
			return
		}
		role, p = &resolved, provider
		s.provider, s.systemPrompt = resolved.Agent, resolved.Prompt
	} else {
		all := a.agentProviders()
		if s.provider == "" && len(all) > 0 {
			s.provider = all[0].id
		}
		i := slices.IndexFunc(all, func(p agentProvider) bool { return p.id == s.provider })
		if i < 0 {
			s.err = locales.AgentNoProviders()
			return
		}
		p = all[i]
		p.config = cloneAgentServer(p.config)
	}
	if p.config.Disabled {
		s.err = locales.AgentDisabled()
		return
	}
	if p.config.EffectiveProtocol() == "cli" {
		if s.continuation.needsNativeHistory() {
			s.err = s.continuation.readError
			return
		}
		a.sendCLIChat(s, p, role, prompt)
		return
	}
	if err := p.config.Validate(); err != nil {
		s.err = err.Error()
		return
	}
	var slashAfterConnect string
	if prompt != "" && strings.HasPrefix(s.draft, "/") {
		name, _ := splitSlashLine(s.draft)
		if name == slashModel || name == slashThinking || name == slashMode || piTerminalPage(name) {
			slashAfterConnect = prompt
			prompt = ""
		}
	}
	a.disconnectChat(s)
	s.err, s.connecting, s.connected, s.connectedRole = "", true, &p, role
	model := s.model()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	ctx, gen, root := s.ctx, s.generation, a.root
	if prompt != "" {
		s.stageDetailSources()
		s.draft, s.pendingPrompt = "", prompt
	}
	handler := a.chatHandler(s)
	source := s.continuation.restoreSource()
	needHistory := s.continuation.needsNativeHistory()
	go func() {
		contextPolicy := loadAgentCompactionPolicy(p.config, root)
		client, err := acp.Start(ctx, resumeACPConfig(p, source), root, handler)
		if err != nil {
			handler.post(func() {
				s.connecting = false
				s.err = err.Error()
				s.restorePendingTask(agentTaskFailed)
			})
			return
		}
		setup, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		info, err := client.Initialize(setup)
		if err != nil {
			client.Close()
			handler.post(func() {
				s.connecting = false
				s.err = err.Error()
				s.restorePendingTask(agentTaskFailed)
			})
			return
		}
		var auth []agentChoice
		for _, m := range info.AuthMethods {
			if m.Agent != nil {
				auth = append(auth, agentChoice{string(m.Agent.Id), m.Agent.Name})
			}
		}
		session, sessionErr := client.OpenSession(setup, source.ID, needHistory)
		created := sessionErr == nil
		if sessionErr == nil && model != "" {
			sessionErr = client.SetModel(setup, string(session.SessionId), model)
		}
		if ctx.Err() != nil {
			client.Close()
			return
		}
		handler.dispatch(func(a *app) {
			if s.generation != gen || a.root != root {
				go client.Close()
				return
			}
			s.client, s.connecting = client, false
			if info.AgentInfo != nil {
				s.adapterName = info.AgentInfo.Name
			}
			s.contextPolicy = contextPolicy
			s.authMethods = auth
			if sessionErr != nil {
				if created {
					a.updateChatSession(s, session, false)
				}
				s.err = sessionErr.Error()
				s.restorePendingTask(agentTaskFailed)
			} else {
				a.acceptChatSession(s, session)
				s.pendingPrompt = ""
				if prompt != "" {
					a.promptChat(s, prompt)
				} else if slashAfterConnect != "" && s.draftPrompt() == slashAfterConnect {
					a.sendChat(s)
				}
			}
		})
		go func() {
			<-client.Done()
			handler.post(func() {
				handler.workspace().disconnectExitedChat(s, client)
			})
		}()
	}()
}

func (a *app) disconnectExitedChat(s *agentChat, client *acp.Client) {
	if s.client != client || s.busy {
		return
	}
	select {
	case <-client.Done():
		if s.err == "" {
			s.err = locales.AgentDisconnected()
			if err := client.Err(); err != nil {
				s.err = err.Error()
			}
		}
		a.disconnectChat(s)
	default:
	}
}

func copyAgentJSON(from, to any) {
	data, err := json.Marshal(from)
	if err == nil {
		_ = json.Unmarshal(data, to)
	}
}

func (a *app) acceptChatSession(s *agentChat, session acp.Session) {
	a.updateChatSession(s, session, true)
}

func (a *app) updateChatSession(s *agentChat, session acp.Session, notice bool) {
	if s.continuation != nil {
		s.continuation.native = session.Restored
		if !s.continuation.loaded && len(session.Replay) > 0 {
			historical := make([]*agentMessage, 0, len(session.Replay)+len(s.messages))
			for _, message := range session.Replay {
				historical = append(historical, &agentMessage{role: message.Role, text: message.Text, historical: true})
			}
			s.messages = append(historical, s.messages...)
			s.continuation.loaded, s.continuation.truncated = true, session.PartialReplay
		}
	}
	if notice && !session.Restored && slices.ContainsFunc(s.messages, func(msg *agentMessage) bool { return msg != nil && !msg.historical && msg.role != "connection" }) {
		s.messages = append(s.messages, &agentMessage{role: "system", text: locales.AgentNewSessionNotice()})
	}
	s.sessionID, s.err = string(session.SessionId), ""
	if s.client != nil {
		s.commands = s.client.Commands(s.sessionID)
	}
	s.contextAccounting = agentContextAccounting{}
	s.mode, s.modes, s.thinkingModes = "", nil, false
	a.refreshAgentModels(s)
}

func restoreChatDraft(s *agentChat, text string) {
	if text == "" {
		return
	}
	if s.sources.pending.hasFiles() {
		text = s.sources.pending.text
	}
	prompt := text
	if text == "" {
		text = s.draft
	} else if s.draft != "" {
		text += "\n\n" + s.draft
	}
	s.restoreDetailSources(prompt, s.draft, text)
	s.draft = text
}

func (s *agentChat) restorePendingTask(result agentTaskState) {
	if s.pendingPrompt != "" {
		restoreChatDraft(s, s.pendingPrompt)
		s.lastTask, s.pendingPrompt = result, ""
	}
}

func (a *app) sendAgent() {
	a.sendChat(a.agent)
}

func (a *app) sendChat(s *agentChat) {
	a.checkAgentConfig()
	if s.requestInFlight() || s.contextLoading > 0 || !s.continuationReady() || !s.hasDraft() {
		return
	}
	if a.submitSlashSetting(s) {
		return
	}
	defer a.recordTaskChange(s, s.taskState())
	limit := agentTextLimit
	if s.continuation != nil && !s.continuation.native {
		limit -= len(s.continuation.context)
	}
	if s.sources.draft.promptSize(s.draft) > limit {
		s.err = locales.AgentContextTooLarge()
	} else if s.client == nil || s.sessionID == "" {
		a.beginChat(s, s.draftPrompt())
	} else {
		text := s.draftPrompt()
		s.stageDetailSources()
		s.draft = ""
		a.promptChat(s, text)
	}
	if s.err != "" {
		s.lastTask = agentTaskFailed
	}
}

func (a *app) promptChat(s *agentChat, text string) {
	if s.hasRole() {
		role, provider, err := a.resolveAgentRole(s)
		if err != nil || s.connectedRole == nil || role != *s.connectedRole || s.connected == nil || !reflect.DeepEqual(provider.config, s.connected.config) {
			a.disconnectChat(s)
			s.lastTask = agentTaskFailed
			s.err = locales.AgentConfigurationChanged()
			if err != nil {
				s.err = err.Error()
			}
			restoreChatDraft(s, text)
			return
		}
	}
	s.turn++
	s.busy, s.stopping, s.err = true, false, ""
	s.pendingPrompt = text
	s.plan = nil
	client, session, ctx, root := s.client, s.sessionID, s.ctx, a.root
	timeout := settings.DefaultAgentTimeoutSeconds
	if s.connected != nil {
		timeout = s.connected.config.ReplyTimeoutSeconds()
	}
	call, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	s.replyCancel = cancel
	restored := s.continuation != nil && s.continuation.native
	isCommand := advertisedChatCommand(s.commands, text)
	newSession := s.turn > 1 && !restored && !isCommand
	mode := s.mode
	thinking := chatModeIsThinking(s, mode)
	genericMode := s.genericMode
	model := s.model()
	turn, contextPolicy := s.turn, s.contextPolicy
	h := a.chatHandler(s)
	var contextConfig settings.AgentServer
	if s.connected != nil {
		contextConfig = cloneAgentServer(s.connected.config)
	}
	if newSession {
		// No previous-session callbacks may mutate a new turn while setup is
		// pending. The new ID is published only after its mode is established.
		s.sessionID = ""
		s.commands = nil
		s.contextUsage = nil
		s.contextAccounting = agentContextAccounting{}
		s.contextPolicy = agentCompactionPolicy{}
	}
	go func() {
		defer cancel()
		// A fresh ACP session makes the recent-message limit real rather than
		// adding bounded text to a server-side conversation of unlimited length.
		var err error
		var created acp.Session
		createdOK := false
		if newSession {
			contextPolicy = loadAgentCompactionPolicy(contextConfig, root)
			created, err = client.NewSession(call)
			if err == nil {
				createdOK = true
				session = string(created.SessionId)
				if genericMode != "" {
					err = client.SetMode(call, session, genericMode)
				} else if mode != "" && !thinking {
					err = client.SetMode(call, session, mode)
				}
			}
		}
		if err == nil && model != "" {
			// Generic modes can reset the model; models can reset thinking.
			err = client.SetModel(call, session, model)
		}
		if err == nil && thinking {
			err = client.SetThinkingMode(call, session, mode)
		}
		var prompt string
		if err == nil {
			err = h.onMain(call, func() error {
				a := h.workspace()
				if s.client != client || s.turn != turn {
					return context.Canceled
				}
				if newSession {
					a.updateChatSession(s, created, false)
					s.contextPolicy = contextPolicy
				}
				a.refreshAgentModels(s)
				// Session setup can fail without sending the question. Keep it
				// out of history until then so retrying cannot duplicate it.
				s.messages = append(s.messages, &agentMessage{role: "user", text: text})
				s.recordDetailSources()
				if restored || isCommand {
					s.contextAccounting = agentContextAccounting{session: s.sessionID, generation: s.generation, messageStart: len(s.messages), input: map[string]int{"user": agentContextTextTokens(text)}}
					prompt = text
				} else {
					prompt = captureAgentContext(s, chatContext(s))
				}
				return nil
			})
		}
		var reply acp.PromptResult
		if err == nil {
			reply, err = client.Prompt(call, session, []acp.ContentBlock{acp.TextBlock(prompt)})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			client.Close()
		}
		h.post(func() {
			a := h.workspace()
			if a.root != root || s.client != client || s.turn != turn {
				return
			}
			s.busy, s.stopping = false, false
			s.pendingPrompt = ""
			switch {
			case errors.Is(err, context.Canceled), err == nil && reply.StopReason == "cancelled":
				s.lastTask = agentTaskIdle
			case err == nil && reply.StopReason == "end_turn":
				s.lastTask = agentTaskCompleted
			default:
				s.lastTask = agentTaskFailed
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				if createdOK && s.sessionID == "" {
					// A rejected model remains selected until the user changes it;
					// the new session's choices make that correction possible.
					a.updateChatSession(s, created, false)
					s.contextPolicy = contextPolicy
				}
				a.refreshAgentModels(s)
				s.err = err.Error()
				restoreChatDraft(s, text)
			}
			s.sources.pending = chatSourceDraft{}
			s.permissions = nil
			a.disconnectExitedChat(s, client)
			a.gitChanged()
		})
	}()
}

func (a *app) stopAgent() {
	a.stopChat(a.agent)
}

func (a *app) stopChat(s *agentChat) {
	if s.busy || s.connecting {
		defer a.recordTaskChange(s, s.taskState())
		// Closing the owning connection stops a pending session/new and agents
		// that ignore session/cancel. A later send reconnects.
		s.restorePendingTask(agentTaskIdle)
		a.disconnectChat(s)
		s.err = locales.AgentStopped()
	}
}

func (a *app) expireAgentStop(gen, turn uint64) { a.expireChatStop(a.agent, gen, turn) }

func (a *app) expireChatStop(s *agentChat, gen, turn uint64) {
	if s.generation == gen && s.turn == turn && s.stopping {
		a.stopChat(s)
	}
}

func (a *app) setAgentMode(id string) {
	a.setChatMode(a.agent, id)
}

func chatModeIsThinking(s *agentChat, id string) bool {
	if s.thinkingModes {
		return slices.ContainsFunc(s.modes, func(choice agentChoice) bool { return choice.id == id })
	}
	for _, choice := range s.modes {
		if choice.id == id {
			name := strings.ToLower(strings.TrimSpace(choice.name))
			return strings.HasPrefix(name, "thinking:") || strings.HasPrefix(name, "reasoning:")
		}
	}
	return false
}

func (a *app) setChatMode(s *agentChat, id string) {
	a.setChatModeSelection(s, id, chatModeIsThinking(s, id), nil)
}

func (a *app) setChatGenericMode(s *agentChat, id string) {
	a.setChatModeSelection(s, id, false, nil)
}

func (a *app) setChatModeSelection(s *agentChat, id string, thinking bool, applied func()) {
	if s.client == nil || s.sessionID == "" || s.requestInFlight() {
		return
	}
	s.configuring = true
	client, session, ctx, model := s.client, s.sessionID, s.ctx, s.model()
	h := a.chatHandler(s)
	currentThinking := s.mode
	restoreThinking := s.thinkingModes && !thinking && chatModeIsThinking(s, currentThinking)
	go func() {
		call, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		var err error
		if thinking {
			err = client.SetThinkingMode(call, session, id)
		} else {
			err = client.SetMode(call, session, id)
		}
		if err == nil && model != "" && !thinking {
			err = client.SetModel(call, session, model)
		}
		if err == nil && restoreThinking {
			err = client.SetThinkingMode(call, session, currentThinking)
		}
		h.post(func() {
			if s.client != client || s.sessionID != session {
				return
			}
			s.configuring = false
			h.workspace().refreshAgentModels(s)
			if err != nil {
				s.err = err.Error()
			} else {
				s.err = ""
				if applied != nil {
					applied()
				}
			}
		})
	}()
}

func (a *app) authenticateAgent(id string) {
	a.authenticateChat(a.agent, id)
}

func (a *app) authenticateChat(s *agentChat, id string) {
	if s.client == nil || s.requestInFlight() {
		return
	}
	s.connecting, s.err = true, ""
	client, ctx, model := s.client, s.ctx, s.model()
	h := a.chatHandler(s)
	root, previousSession := a.root, s.sessionID
	source := s.continuation.restoreSource()
	needHistory := s.continuation.needsNativeHistory()
	var contextConfig settings.AgentServer
	if s.connected != nil {
		contextConfig = cloneAgentServer(s.connected.config)
	}
	go func() {
		call, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		err := client.Authenticate(call, id)
		var session acp.Session
		var contextPolicy agentCompactionPolicy
		created := false
		if err == nil && (previousSession == "" || previousSession != source.ID) {
			contextPolicy = loadAgentCompactionPolicy(contextConfig, root)
			session, err = client.OpenSession(call, source.ID, needHistory)
			created = err == nil
		}
		if created && model != "" {
			err = client.SetModel(call, string(session.SessionId), model)
		}
		h.post(func() {
			a := h.workspace()
			if a.root != root || s.client != client || s.sessionID != previousSession {
				return
			}
			s.connecting = false
			if err != nil {
				if created {
					a.updateChatSession(s, session, false)
					s.contextPolicy = contextPolicy
				}
				s.err = err.Error()
			} else if created {
				a.acceptChatSession(s, session)
				s.contextPolicy = contextPolicy
			} else {
				s.err = ""
			}
		})
	}()
}

func (a *app) postAgent(gen uint64, fn func()) { a.postChat(a.agent, gen, fn) }

func (a *app) postChat(s *agentChat, gen uint64, fn func()) {
	a.post(func() {
		if s.generation == gen {
			fn()
		}
	})
}

type agentHandler struct {
	app         *app
	events      *tabEvents
	chat        *agentChat
	generation  uint64
	root        string
	meeting     bool
	meetingLife context.Context
	toolChanged func(*agentTool)
}

func (a *app) chatHandler(s *agentChat) *agentHandler {
	if s.events == nil {
		s.events = &tabEvents{window: a}
		if mygo.App.IsReady() {
			s.events.dispatch = mygo.App.WhenReady
		}
	}
	return &agentHandler{app: a, events: s.events, chat: s, generation: s.generation, root: a.root}
}

func (h *agentHandler) workspace() *app {
	if h.events != nil {
		return h.events.window
	}
	return h.app
}

func (h *agentHandler) dispatch(fn func(*app)) {
	deliver := func(owner *app) {
		if !h.meeting {
			chat := h.state()
			defer owner.recordTaskChange(chat, chat.taskState())
		}
		fn(owner)
	}
	if h.events != nil {
		h.events.post(deliver)
	} else {
		h.app.post(func() { deliver(h.app) })
	}
}

func (h *agentHandler) state() *agentChat {
	if h.chat != nil {
		return h.chat
	}
	return h.app.agent
}
func (h *agentHandler) valid() bool {
	return (h.meetingLife == nil || h.meetingLife.Err() == nil) && h.workspace().root == h.root && h.state().generation == h.generation
}
func (h *agentHandler) post(fn func()) {
	h.dispatch(func(*app) {
		if h.valid() {
			fn()
		}
	})
}

// Updates are patches: absent fields keep the previous tool title and content.
func (h *agentHandler) Update(update acp.Update) {
	if update.Update.AvailableCommandsUpdate != nil {
		session := string(update.SessionId)
		h.post(func() {
			if chat := h.state(); chat.client != nil && chat.sessionID == session {
				chat.commands = chat.client.Commands(session)
			}
		})
		return
	}
	if usage := update.Update.UsageUpdate; usage != nil {
		value, sessionID := acp.UsageState{Used: usage.Used, Size: usage.Size}, string(update.SessionId)
		h.post(func() {
			if s := h.state(); s.sessionID == sessionID {
				s.acceptContextUsage(value)
			}
		})
		return
	}
	if plan := update.Update.Plan; plan != nil {
		sessionID := string(update.SessionId)
		h.post(func() {
			s := h.state()
			if sessionID == "" || s.sessionID != sessionID {
				return
			}
			s.keepLatestInView()
			s.plan = make([]agentPlan, len(plan.Entries))
			for index, entry := range plan.Entries {
				s.plan[index] = agentPlan{content: entry.Content, status: string(entry.Status)}
			}
		})
		return
	}
	var n struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Type                string          `json:"sessionUpdate"`
			Content             json.RawMessage `json:"content"`
			RawInput            json.RawMessage `json:"rawInput"`
			Locations           []agentLocation `json:"locations"`
			ID                  string          `json:"toolCallId"`
			Title, Kind, Status *string
			CurrentModeID       string `json:"currentModeId"`
		} `json:"update"`
	}
	copyAgentJSON(update, &n)
	var sourcePatch chatToolSources
	if call := update.Update.ToolCall; call != nil {
		sourcePatch = chatToolSourcePatch(call.Locations, call.Content)
	} else if patch := update.Update.ToolCallUpdate; patch != nil {
		sourcePatch = chatToolSourcePatch(patch.Locations, patch.Content)
	}
	h.post(func() {
		a, u := h.workspace(), n.Update
		s := h.state()
		if n.SessionID != s.sessionID && s.sessionID != "" {
			return
		}
		switch u.Type {
		case "agent_message_chunk", "agent_thought_chunk":
			var block struct{ Type, Text string }
			_ = json.Unmarshal(u.Content, &block)
			if block.Type != "text" {
				return
			}
			role := "assistant"
			if u.Type == "agent_thought_chunk" {
				role = "thought"
			}
			if !h.meeting && (!s.busy || s.sessionID == "") {
				role = "connection"
			}
			var m *agentMessage
			if len(s.messages) > 0 {
				if last := s.messages[len(s.messages)-1]; last != nil && !last.historical && last.role == role {
					m = last
				}
			}
			if m == nil {
				m = &agentMessage{role: role}
				s.messages = append(s.messages, m)
			}
			m.text = boundedAgentText(m.text + block.Text)
			m.render = viewNode{Text: m.text}
		case "tool_call", "tool_call_update":
			var tool *agentTool
			for _, t := range s.tools {
				if t.id == u.ID && t.sessionID == n.SessionID && t.generation == h.generation {
					tool = t
					break
				}
			}
			if tool == nil {
				tool = &agentTool{id: u.ID, sessionID: n.SessionID, generation: h.generation, status: "pending"}
				s.tools = append(s.tools, tool)
				s.messages = append(s.messages, &agentMessage{role: "tool", tool: tool})
			}
			if u.Title != nil {
				tool.title = *u.Title
			}
			if u.Kind != nil {
				tool.kind = *u.Kind
			}
			if u.Status != nil {
				tool.status = *u.Status
			}
			if u.RawInput != nil {
				tool.rawInput = u.RawInput
			}
			if u.Locations != nil {
				tool.locations = u.Locations
			}
			if u.Content != nil {
				var contents []struct {
					Type, Path, NewText string
					OldText             *string
					Content             struct{ Type, Text string }
				}
				_ = json.Unmarshal(u.Content, &contents)
				tool.text, tool.changes = "", nil
				for _, c := range contents {
					switch c.Type {
					case "content":
						tool.text = boundedAgentText(tool.text + c.Content.Text + "\n")
					case "diff":
						if len(c.NewText) > agentTextLimit || c.OldText != nil && len(*c.OldText) > agentTextLimit {
							continue
						}
						change := agentChange{path: c.Path, newText: c.NewText, existed: c.OldText != nil}
						if c.OldText != nil {
							change.oldText = *c.OldText
						}
						tool.changes = append(tool.changes, change)
					}
				}
			}
			if n.SessionID != "" && n.SessionID == s.sessionID {
				s.sources.updateTool(tool, sourcePatch)
			}
			for _, p := range s.permissions {
				fillChatPermission(s, p)
			}
			if h.toolChanged != nil {
				h.toolChanged(tool)
			}
			a.gitChanged()
		case "current_mode_update":
			a.refreshAgentModes(s)
		case "config_option_update":
			a.refreshAgentModels(s)
		}
	})
}

func boundedAgentText(s string) string {
	if len(s) <= agentTextLimit {
		return s
	}
	return strings.ToValidUTF8(s[:agentTextLimit], "")
}

func (h *agentHandler) RequestPermission(ctx context.Context, request acp.PermissionRequest) (acp.PermissionOutcome, error) {
	var r struct {
		ToolCall struct {
			Title     string
			ID        string          `json:"toolCallId"`
			RawInput  json.RawMessage `json:"rawInput"`
			Content   json.RawMessage `json:"content"`
			Locations []agentLocation
		} `json:"toolCall"`
	}
	copyAgentJSON(request, &r)
	answer := make(chan string, 1)
	p := &agentPermission{title: r.ToolCall.Title, toolID: r.ToolCall.ID, sessionID: string(request.SessionId), generation: h.generation, options: request.Options}
	p.detail = formatAgentToolDetail(r.ToolCall.Locations, r.ToolCall.RawInput, r.ToolCall.Content)
	p.preview = previewPermission(r.ToolCall.RawInput, r.ToolCall.Locations, p.title, len(request.ToolCall.Content) > 0)
	p.choose = func(id string) {
		select {
		case answer <- id:
		default:
		}
	}
	h.post(func() {
		fillChatPermission(h.state(), p)
		if p.title == "" {
			p.title = r.ToolCall.ID
		}
		h.state().permissions = append(h.state().permissions, p)
		if a := h.workspace(); !h.meeting && a.agent == h.state() && !a.chatShown(h.state()) {
			a.showAgent()
		}
	})
	id := ""
	select {
	case id = <-answer:
	case <-ctx.Done():
	}
	h.post(func() {
		h.state().permissions = slices.DeleteFunc(h.state().permissions, func(o *agentPermission) bool { return o == p })
	})
	if id == "" {
		return acp.CancelledPermission(), nil
	}
	return acp.SelectedPermission(id), nil
}

func formatAgentToolDetail(locations []agentLocation, values ...json.RawMessage) string {
	var detail bytes.Buffer
	for _, location := range locations {
		detail.WriteString(location.Path)
		if location.Line != nil {
			fmt.Fprintf(&detail, ":%d", *location.Line)
		}
		detail.WriteByte('\n')
	}
	for _, raw := range values {
		if len(raw) != 0 && string(raw) != "null" && json.Indent(&detail, raw, "", "  ") == nil {
			detail.WriteByte('\n')
		}
	}
	return boundedAgentText(detail.String())
}

func fillChatPermission(s *agentChat, p *agentPermission) {
	for _, tool := range s.tools {
		if tool.id != p.toolID || tool.sessionID != p.sessionID || tool.generation != p.generation {
			continue
		}
		if p.title == "" || p.title == p.toolID {
			p.title = tool.title
		}
		if p.detail == "" || p.detailFromTool {
			p.detail = formatAgentToolDetail(tool.locations, tool.rawInput) + tool.text
			for _, change := range tool.changes {
				p.detail += agentContextText(change.path, 0, 0, change.newText) + "\n"
			}
			p.detail = boundedAgentText(p.detail)
			p.preview = previewPermission(tool.rawInput, tool.locations, p.title, tool.text != "" || len(tool.changes) > 0)
			p.detailFromTool = true
		}
	}
}

func (h *agentHandler) onMain(ctx context.Context, fn func() error) error {
	done := make(chan error, 1)
	h.dispatch(func(*app) {
		if err := ctx.Err(); err != nil {
			done <- err
			return
		}
		if !h.valid() {
			done <- context.Canceled
			return
		}
		done <- fn()
	})
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

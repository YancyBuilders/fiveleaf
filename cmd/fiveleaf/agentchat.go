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
	sdk "github.com/coder/acp-go-sdk"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const agentTextLimit = 2 << 20

type agentChoice struct{ id, name string }
type agentPlan struct{ content, status string }
type agentMessage struct {
	role, text string
	title      string
	streamID   string
	historical bool
	render     viewNode
	failure    *agentTurnFailure
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
	id, title, kind, status, text  string
	note                           string
	expanded, expansionSet         bool
	runDisclosure                  agentToolRunDisclosure
	changes                        []agentChange
	rawInput                       json.RawMessage
	rawOutput                      string
	terminalOutput, progressOutput agentToolOutputBuffer
	locations                      []agentLocation
	sources                        chatToolSources
	sessionID                      string
	generation, turn               uint64
	observedText, observedStatus   string
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

type codexChatRecovery struct {
	root, sessionID string
	provider        *agentProvider
	role            *settings.AgentRole
	profile         codexLaunchProfile
	instructions    *string
}

type agentDraftState struct {
	id               uint64
	build            int
	text             string
	selection        [2]int
	selectionPending bool
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
	reconnecting                            bool
	connectionAttempted                     bool
	messages                                []*agentMessage
	tools                                   []*agentTool
	permissions                             []*agentPermission
	modes, authMethods                      []agentChoice
	authRequired                            bool
	thinkingModes                           bool
	genericMode                             string
	genericModes                            []agentChoice
	plan                                    []agentPlan
	planExpanded, planExpansionSet          bool
	scroll                                  ui.ScrollState
	readingPrompt                           *agentMessage
	focusThought                            *agentMessage
	readingY                                float32
	details                                 chatDetailsState
	input                                   ui.Handle
	draftState                              agentDraftState
	composeBox                              ui.Handle
	transcriptContent                       ui.Handle
	focusNext                               bool
	client                                  *acp.Client
	transportDone                           <-chan struct{}
	ctx                                     context.Context
	cancel                                  context.CancelFunc
	generation                              uint64
	connected                               *agentProvider
	connectedRole                           *settings.AgentRole
	pendingPrompt                           string
	pendingMessage                          *agentMessage
	promptWritten                           bool
	queued                                  *agentQueuedDraft
	pendingQueued                           bool
	expandedPaste                           *chatDraftPaste
	sessionPrompted                         bool
	sessionInstructions                     *string
	codexSeedAttempt                        *string
	codexProfile                            codexLaunchProfile
	codexRecovery                           *codexChatRecovery
	sessionInUse                            bool
	sources                                 chatSourceState
	lastTask                                agentTaskState
	taskNoticePending                       *ui.Toast
	systemNotice                            *agentTaskNotification
	contextLoading                          int
	turn                                    uint64
	replyBoundary                           int
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

func (s *agentChat) usesCodexThread() bool {
	return s.connected != nil && isCodexACP(s.connected.config) ||
		s.adapterName == "@agentclientprotocol/codex-acp" || s.adapterName == "@zed-industries/codex-acp"
}

func (s *agentChat) usesPiThread() bool {
	return s.connected != nil && isPiACP(s.connected.config) ||
		s.adapterName == "pi-acp"
}

func (s *agentChat) instructions() string {
	if s.connectedRole != nil {
		return s.connectedRole.Prompt
	}
	return s.systemPrompt
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
	if a.settings != nil {
		for _, id := range slices.Sorted(maps.Keys(a.settings.AgentServers)) {
			c := a.settings.AgentServers[id]
			name := c.Name
			if name == "" {
				name = id
			}
			out = append(out, agentProvider{id: id, name: name, config: c})
		}
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
		transportDone: s.transportDone,
		sources:       chatSourceState{draft: s.sources.draft, pastedImages: s.sources.pastedImages},
		systemPrompt:  s.systemPrompt, presetDefaultPrompt: s.presetDefaultPrompt, contextMessages: s.contextMessages, modelOverride: s.modelOverride, recentModels: s.recentModels}
}

func (a *app) disconnectAgent() { a.disconnectChat(a.agent) }

func (a *app) disconnectChat(s *agentChat) {
	a.closeChatConnection(s, agentTaskFailed, cmp.Or(s.err, locales.AgentDisconnected()))
}

func (a *app) closeChatConnection(s *agentChat, task agentTaskState, reason string) {
	defer a.recordTaskChange(s, s.taskState())
	if s.sessionID != "" && s.continuation == nil && s.connected != nil && s.usesCodexThread() {
		s.codexRecovery = &codexChatRecovery{root: a.root, sessionID: s.sessionID, provider: s.connected, role: s.connectedRole,
			profile: s.codexProfile, instructions: cmp.Or(s.sessionInstructions, s.codexSeedAttempt)}
	}
	if s.pendingPrompt != "" || task == agentTaskIdle {
		s.finishPendingTask(task, reason)
	}
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
	s.reconnecting, s.promptWritten = false, false
	s.sessionID, s.mode, s.adapterName = "", "", ""
	s.sessionPrompted = false
	s.sessionInstructions, s.codexSeedAttempt = nil, nil
	s.codexProfile = codexLaunchProfile{}
	s.sessionInUse = false
	if s.continuation != nil {
		s.continuation.native = false
	}
	s.contextUsage = nil
	s.contextDetailsOpen = false
	s.contextPolicy = agentCompactionPolicy{}
	s.contextAccounting = agentContextAccounting{}
	s.permissions, s.modes, s.authMethods, s.connected = nil, nil, nil, nil
	s.authRequired = false
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
	if a.settings == nil {
		return
	}
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
				s.err = locales.AgentConfigurationChanged()
				a.disconnectChat(s)
				s.codexRecovery = nil
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

func (a *app) reconnectChat(s *agentChat) {
	if s.reconnecting {
		return
	}
	if s.stopping {
		if s.pendingMessage != nil && s.client != nil {
			s.reconnecting = true
			client := s.client
			go client.Close()
			return
		}
		a.disconnectChat(s)
	}
	a.beginChat(s, "")
}

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
	newTransport := s.client == nil || prompt == ""
	var slashAfterConnect string
	if prompt != "" && strings.HasPrefix(s.draft, "/") {
		name, _ := splitSlashLine(s.draft)
		if name == slashModel || name == slashThinking || name == slashMode || piTerminalPage(name) {
			slashAfterConnect = prompt
			prompt = ""
		}
	}
	var previousDone <-chan struct{}
	var transportDone chan struct{}
	if newTransport {
		a.disconnectChat(s)
		previousDone = s.transportDone
		transportDone = make(chan struct{})
		s.transportDone = transportDone
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}
	if recovery := s.codexRecovery; recovery != nil && (recovery.root != a.root || !reflect.DeepEqual(recovery.provider, &p) ||
		!reflect.DeepEqual(recovery.role, role) || recovery.instructions != nil && *recovery.instructions != s.instructions()) {
		s.codexRecovery = nil
	}
	recovery := s.codexRecovery
	client, profile := s.client, s.codexProfile
	s.err, s.connecting, s.connected, s.connectedRole = "", true, &p, role
	model := s.model()
	ctx, gen, root := s.ctx, s.generation, a.root
	if prompt != "" {
		s.stageDetailSources()
		s.pendingPrompt = prompt
	}
	handler := a.chatHandler(s)
	source := s.continuation.restoreSource()
	needHistory := s.continuation.needsNativeHistory()
	go func() {
		if newTransport {
			defer close(transportDone)
		}
		if previousDone != nil {
			<-previousDone
		}
		if ctx.Err() != nil {
			return
		}
		contextPolicy := loadAgentCompactionPolicy(p.config, root)
		if newTransport {
			config := resumeACPConfig(p, source)
			profile = agentCodexLaunchProfile(config)
			var err error
			client, err = acp.Start(ctx, config, root, handler)
			if err != nil {
				handler.post(func() {
					s.connecting, s.stopping = false, false
					s.failChatConnection(err)
				})
				return
			}
		}
		reattach := recovery != nil && recovery.profile == profile
		if reattach {
			if err := client.ClaimSession(recovery.sessionID); err != nil {
				if newTransport {
					client.Close()
				}
				handler.post(func() {
					s.connecting, s.stopping = false, false
					s.sessionInUse = errors.Is(err, acp.ErrSessionInUse)
					s.failChatConnection(err)
				})
				return
			}
			defer client.ReleaseSession(recovery.sessionID)
		}
		setup, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		info, sessionErr := client.Initialize(setup)
		var auth []agentChoice
		for _, m := range info.AuthMethods {
			if m.Agent != nil {
				auth = append(auth, agentChoice{string(m.Agent.Id), m.Agent.Name})
			}
		}
		var session acp.Session
		if sessionErr == nil {
			if reattach {
				session, sessionErr = client.ReattachSession(setup, recovery.sessionID)
			} else {
				session, sessionErr = client.OpenSession(setup, source.ID, needHistory)
			}
		}
		created := sessionErr == nil
		if sessionErr == nil && model != "" {
			sessionErr = client.SetModel(setup, string(session.SessionId), model)
		}
		handler.post(func() {
			if s.client == client || s.connecting {
				s.authMethods, s.authRequired = auth, engineAuthRequired(sessionErr)
			}
		})
		if ctx.Err() != nil {
			if newTransport {
				client.Close()
			}
			return
		}
		handler.dispatch(func(a *app) {
			if s.generation != gen || a.root != root {
				if newTransport {
					go client.Close()
				}
				return
			}
			stopped := s.stopping
			s.client, s.connecting, s.stopping = client, false, false
			s.sessionInUse = false
			s.codexProfile = profile
			if !reattach {
				s.codexRecovery = nil
			}
			if info.AgentInfo != nil {
				s.adapterName = info.AgentInfo.Name
			}
			s.contextPolicy = contextPolicy
			s.authMethods = auth
			if sessionErr != nil {
				if created {
					a.updateChatSession(s, session, false)
				}
				s.failChatConnection(sessionErr)
			} else {
				s.authRequired = false
				a.acceptChatSession(s, session)
				s.pendingPrompt = ""
				if stopped {
					return
				}
				if prompt != "" {
					a.promptChat(s, prompt)
				} else if slashAfterConnect != "" && s.draftPrompt() == slashAfterConnect {
					a.sendChat(s)
				}
			}
		})
		if !newTransport {
			return
		}
		cancel()
		<-client.Done()
		handler.post(func() {
			handler.workspace().disconnectExitedChat(s, client)
		})
	}()
}

func (s *agentChat) failChatConnection(err error) {
	s.err = acp.ErrorMessage(err)
	s.sessionInUse = errors.Is(err, acp.ErrSessionInUse)
	s.authRequired = engineAuthRequired(err)
	if errors.Is(err, context.DeadlineExceeded) {
		s.err = locales.AgentConnectionTimeout()
	}
	if s.pendingPrompt != "" {
		s.finishPendingTask(agentTaskFailed, s.err)
	}
}

func engineAuthRequired(err error) bool {
	var rpc *sdk.RequestError
	return errors.As(err, &rpc) && rpc.Code == -32000
}

func (a *app) continueChatInNewThread(chat *agentChat) {
	if chat.requestInFlight() {
		return
	}
	chat.codexRecovery, chat.sessionInUse = nil, false
	chat.sessionInstructions, chat.codexSeedAttempt = nil, nil
	a.beginChat(chat, "")
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
				s.err = acp.ErrorMessage(err)
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
	previousSession := s.sessionID
	if s.continuation != nil {
		s.continuation.native = session.Restored
		if !s.continuation.loaded && len(session.Replay) > 0 {
			historical := make([]*agentMessage, 0, len(session.Replay)+len(s.messages))
			firstUser := true
			for _, message := range session.Replay {
				text := message.Text
				if firstUser && message.Role == "user" {
					firstUser = false
					if decoded, ok := s.continuation.source.DecodeCodexFirstPrompt(text, message.FirstChunkBytes); ok {
						text = decoded
					}
				}
				historical = append(historical, &agentMessage{role: message.Role, text: text, historical: true})
			}
			s.messages = append(historical, s.messages...)
			s.continuation.loaded, s.continuation.truncated = true, session.PartialReplay
		}
	}
	if notice && !session.Restored && slices.ContainsFunc(s.messages, func(msg *agentMessage) bool { return msg != nil && !msg.historical && msg.role != "connection" }) {
		s.messages = append(s.messages, &agentMessage{role: "system", text: locales.AgentNewSessionNotice()})
	}
	s.sessionID, s.err = string(session.SessionId), ""
	if s.sessionID != previousSession {
		s.contextUsage = nil
	}
	s.sessionPrompted = false
	s.sessionInstructions, s.codexSeedAttempt = nil, nil
	if session.Restored && s.codexRecovery != nil && s.codexRecovery.sessionID == s.sessionID {
		s.sessionInstructions = s.codexRecovery.instructions
	}
	s.codexRecovery = nil
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
	if s.sources.pending.hasAttachments() {
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

func (s *agentChat) finishPendingTask(result agentTaskState, reason string) {
	if s.pendingMessage != nil {
		s.recordDetailSources()
	}
	if reason != "" {
		if s.pendingMessage != nil {
			s.pendingMessage.failure = &agentTurnFailure{reason: reason, draft: s.sources.pending}
		} else if s.pendingQueued {
			s.queued = &agentQueuedDraft{draft: s.sources.pending}
		} else {
			restoreChatDraft(s, s.pendingPrompt)
		}
		toolStatus := "failed"
		if result == agentTaskIdle {
			toolStatus = "cancelled"
		}
		for _, tool := range s.tools {
			if tool != nil && tool.generation == s.generation && tool.turn == s.turn {
				switch tool.status {
				case "", "pending", "running", "in_progress":
					tool.status = toolStatus
				}
			}
		}
		s.plan = nil
	}
	s.lastTask, s.pendingPrompt, s.pendingMessage = result, "", nil
	s.pendingQueued = false
	s.sources.pending = chatSourceDraft{}
}

func (a *app) sendAgent() {
	a.sendChat(a.agent)
}

func (a *app) sendChat(s *agentChat) {
	a.checkAgentConfig()
	if s.requestInFlight() || s.contextLoading > 0 || !s.continuationReady() || !s.hasDraft() {
		return
	}
	if s.client != nil {
		a.disconnectExitedChat(s, s.client)
	}
	if a.submitSlashSetting(s) {
		return
	}
	defer a.recordTaskChange(s, s.taskState())
	if s.sources.draft.promptSize(s.draft) > s.chatDraftLimit() {
		s.err = locales.AgentContextTooLarge()
	} else if s.client == nil || s.sessionID == "" {
		a.beginChat(s, s.draftPrompt())
	} else {
		text := s.draftPrompt()
		s.stageDetailSources()
		a.promptChat(s, text)
	}
	if s.err != "" {
		s.lastTask = agentTaskFailed
	}
}

func (s *agentChat) chatDraftLimit() int {
	if s.continuation != nil && !s.continuation.native {
		return agentTextLimit - len(s.continuation.context)
	}
	return agentTextLimit
}

func (a *app) promptChat(s *agentChat, text string) {
	if s.hasRole() {
		role, provider, err := a.resolveAgentRole(s)
		if err != nil || s.connectedRole == nil || role != *s.connectedRole || s.connected == nil || !reflect.DeepEqual(provider.config, s.connected.config) {
			s.pendingPrompt = text
			s.err = locales.AgentConfigurationChanged()
			if err != nil {
				s.err = err.Error()
			}
			a.disconnectChat(s)
			s.codexRecovery = nil
			return
		}
	}
	s.turn++
	s.replyBoundary = len(s.messages)
	s.busy, s.stopping, s.err = true, false, ""
	s.promptWritten = false
	s.pendingPrompt = text
	s.plan = nil
	client, session, ctx, root := s.client, s.sessionID, s.ctx, a.root
	timeout := 0
	if s.connected != nil {
		timeout = s.connected.config.TimeoutSeconds
	}
	call, cancel := context.WithCancel(ctx)
	s.replyCancel = cancel
	restored := s.continuation != nil && s.continuation.native
	isCommand := advertisedChatCommand(s.commands, text)
	codex := s.usesCodexThread()
	instructions := s.instructions()
	seeded := s.sessionInstructions != nil && *s.sessionInstructions == instructions
	newSession := !restored && !isCommand && ((!codex && s.sessionPrompted) || (codex && s.sessionInstructions != nil && !seeded))
	mode := s.mode
	thinking := chatModeIsThinking(s, mode)
	genericMode := s.genericMode
	model := s.model()
	previousSession := session
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
	}
	go func() {
		defer cancel()
		setupTimeout := cmp.Or(timeout, settings.DefaultAgentTimeoutSeconds)
		setup, cancelSetup := context.WithTimeout(call, time.Duration(setupTimeout)*time.Second)
		defer cancelSetup()
		var err error
		var created acp.Session
		createdOK := false
		if newSession {
			contextPolicy = loadAgentCompactionPolicy(contextConfig, root)
			created, err = client.NewSession(setup)
			if err == nil {
				createdOK = true
				session = string(created.SessionId)
				if genericMode != "" {
					err = client.SetMode(setup, session, genericMode)
				} else if mode != "" && !thinking {
					err = client.SetMode(setup, session, mode)
				}
			}
		}
		if err == nil && model != "" {
			// Generic modes can reset the model; models can reset thinking.
			err = client.SetModel(setup, session, model)
		}
		if err == nil && thinking {
			err = client.SetThinkingMode(setup, session, mode)
		}
		cancelSetup()
		var initialUsage *acp.UsageState
		if err == nil {
			err = h.onMain(call, func() error {
				if s.client != client {
					return context.Canceled
				}
				if newSession {
					a.updateChatSession(s, created, false)
					s.contextPolicy = contextPolicy
				}
				a.refreshAgentModels(s)
				if usage, ok := client.UsageState(s.sessionID); ok {
					initialUsage = &usage
				}
				return nil
			})
		}
		var prompt []acp.ContentBlock
		if err == nil {
			err = h.onMain(call, func() error {
				if s.client != client || s.turn != turn {
					return context.Canceled
				}
				s.contextUsage = initialUsage
				if s.stopping {
					return context.Canceled
				}
				// Session setup can fail without sending the question. Keep it
				// out of history until then so retrying cannot duplicate it.
				s.pendingMessage = &agentMessage{role: "user", text: text}
				s.messages = append(s.messages, s.pendingMessage)
				if restored || isCommand || codex && seeded {
					s.contextAccounting = agentContextAccounting{session: s.sessionID, generation: s.generation, messageStart: len(s.messages), input: map[string]int{"user": agentContextTextTokens(text)}}
					prompt = []acp.ContentBlock{acp.TextBlock(text)}
				} else if codex {
					prompt = captureCodexAgentContext(s, text)
				} else {
					prompt = []acp.ContentBlock{acp.TextBlock(captureAgentContext(s, chatContext(s)))}
				}
				return nil
			})
		}
		var reply acp.PromptResult
		var sent bool
		if err == nil {
			events := acp.PromptEvents{
				Written: func() {
					h.post(func() {
						if s.client == client && s.turn == turn && s.busy {
							s.promptWritten = true
						}
					})
				},
				Idle: func() {
					h.post(func() {
						if s.client == client && s.turn == turn && !s.stopping {
							s.stopping, s.permissions = true, nil
							s.err = locales.AgentReplyIdleTimeout(timeout)
						}
					})
				},
			}
			reply, sent, err = client.PromptWithStop(call, session, prompt, time.Duration(timeout)*time.Second, &events)
		}
		h.post(func() {
			a := h.workspace()
			if a.root != root || s.client != client || s.turn != turn {
				return
			}
			stopRequested := s.stopping
			reconnect := s.reconnecting
			stopReason := s.err
			s.busy, s.stopping, s.replyCancel = false, false, nil
			s.reconnecting, s.promptWritten = false, false
			if stopRequested {
				s.err = ""
			}
			if !sent && s.pendingMessage != nil {
				s.messages = slices.DeleteFunc(s.messages, func(message *agentMessage) bool { return message == s.pendingMessage })
				s.pendingMessage = nil
				s.contextAccounting = agentContextAccounting{}
			}
			s.sessionPrompted = s.sessionPrompted || sent
			if sent && codex && !restored && !isCommand && !seeded {
				s.codexSeedAttempt = &instructions
				if reply.StopReason != "" {
					s.sessionInstructions = &instructions
				}
			}
			if newSession && s.sessionID == "" {
				if createdOK {
					a.updateChatSession(s, created, false)
					s.contextPolicy = contextPolicy
				} else {
					s.sessionID = previousSession
					s.commands = client.Commands(previousSession)
				}
			}
			if s.client == client {
				a.refreshAgentUsage(s)
			}
			result, reason := agentTaskFailed, ""
			switch {
			case stopRequested && (!sent || errors.Is(err, context.Canceled)), err == nil && reply.StopReason == "cancelled":
				result, reason = agentTaskIdle, locales.AgentStatusCancelled()
			case reconnect && errors.Is(err, acp.ErrClosed):
				result, reason = agentTaskIdle, locales.AgentStatusCancelled()
				if stopReason != "" {
					result, reason = agentTaskFailed, stopReason
				}
			case err == nil && reply.StopReason == "end_turn":
				result = agentTaskCompleted
				if codex && s.pendingMessage != nil {
					s.finishCodexReply()
				}
				s.noteEmptyReply()
			case err == nil:
				reason = agentIncompleteReply(string(reply.StopReason))
			default:
				reason = acp.ErrorMessage(err)
				if errors.Is(err, context.DeadlineExceeded) {
					reason = locales.AgentSetupTimeout()
					if sent {
						reason = locales.AgentReplyIdleTimeout(timeout)
					}
				}
				a.refreshAgentModels(s)
				s.err = reason
			}
			s.finishPendingTask(result, reason)
			s.permissions = nil
			if reconnect {
				a.beginChat(s, "")
			} else {
				a.disconnectExitedChat(s, client)
				if result == agentTaskCompleted && !stopRequested {
					a.followQueuedChat(s)
				}
			}
			a.gitChanged()
		})
	}()
}

func (a *app) stopAgent() {
	a.stopChat(a.agent)
}

func (a *app) stopChat(s *agentChat) {
	if s.stopping || !s.busy && !s.connecting {
		return
	}
	if s.connected != nil && s.connected.config.EffectiveProtocol() == "acp" {
		defer a.recordTaskChange(s, s.taskState())
		s.stopping = true
		s.permissions = nil
		if s.pendingMessage != nil && s.replyCancel != nil {
			s.replyCancel()
		} else if s.pendingPrompt != "" {
			s.finishPendingTask(agentTaskIdle, locales.AgentStatusCancelled())
		}
		return
	}
	s.err = locales.AgentStopped()
	a.closeChatConnection(s, agentTaskIdle, s.err)
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
				s.err = acp.ErrorMessage(err)
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
	recovery := s.codexRecovery
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
			if recovery != nil {
				session, err = client.ReattachSession(call, recovery.sessionID)
			} else {
				session, err = client.OpenSession(call, source.ID, needHistory)
			}
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
			s.connecting, s.stopping = false, false
			if err != nil {
				s.authRequired = engineAuthRequired(err)
				if created {
					a.updateChatSession(s, session, false)
					s.contextPolicy = contextPolicy
				}
				s.err = acp.ErrorMessage(err)
			} else if created {
				s.authRequired = false
				a.acceptChatSession(s, session)
				s.contextPolicy = contextPolicy
			} else {
				s.authRequired = false
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

func (h *agentHandler) appendOutput(session, role string, messageID *string, content acp.ContentBlock) {
	if content.Text == nil || content.Text.Text == "" {
		return
	}
	text, id := content.Text.Text, ""
	if messageID != nil {
		id = *messageID
	}
	h.post(func() {
		chat := h.state()
		if chat.sessionID != "" && chat.sessionID != session {
			return
		}
		lateThought := !h.meeting && role == "thought" && !chat.busy
		if !h.meeting {
			if !chat.busy || chat.sessionID == "" {
				if role == "thought" {
					if id == "" || chat.sessionID == "" {
						return
					}
				} else {
					role = "connection"
				}
			} else if role == "assistant" && chat.usesCodexThread() {
				role = "commentary"
			}
		}
		var message *agentMessage
		for _, previous := range slices.Backward(chat.messages) {
			if previous == nil || previous.role == "user" || previous.historical {
				break
			}
			if previous.role == role && previous.streamID == id {
				message = previous
				break
			}
			if id == "" || role == "connection" {
				break
			}
		}
		if message == nil {
			if lateThought || role == "thought" && strings.TrimSpace(text) == "" {
				return
			}
			message = &agentMessage{role: role, streamID: id}
			chat.messages = append(chat.messages, message)
		}
		message.text = boundedAgentText(message.text + text)
		message.render = viewNode{Text: message.text}
		if !h.meeting && chat.busy && role == "thought" && strings.TrimSpace(text) != "" {
			chat.replyBoundary = len(chat.messages)
		}
	})
}

func (chat *agentChat) finishCodexReply() {
	for _, message := range slices.Backward(chat.messages[min(chat.replyBoundary, len(chat.messages)):]) {
		if message == nil {
			continue
		}
		switch message.role {
		case "user", "tool", "thought", "assistant":
			return
		case "commentary":
			if message.streamID != "" && strings.TrimSpace(message.text) != "" {
				message.role = "assistant"
				return
			}
		}
	}
}

func (h *agentHandler) Update(update acp.Update) {
	if chunk := update.Update.AgentMessageChunk; chunk != nil {
		h.appendOutput(string(update.SessionId), "assistant", chunk.MessageId, chunk.Content)
		return
	}
	if chunk := update.Update.AgentThoughtChunk; chunk != nil {
		h.appendOutput(string(update.SessionId), "thought", chunk.MessageId, chunk.Content)
		return
	}
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
				if value.Used == 0 && !s.sessionPrompted && s.contextUsage == nil {
					return
				}
				if s.client != nil && s.usesCodexThread() {
					if tokens, ok := s.client.PromptUsage(sessionID); ok {
						value = agentContextOccupancy(true, tokens, value)
					}
				}
				if s.usesPiThread() && s.connected != nil {
					if size, found := piContextWindow(s.connected.config, h.root, s.model(), value.Size); found {
						value.Size = size
					}
				}
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
			if !h.meeting && s.busy && len(plan.Entries) > 0 {
				s.replyBoundary = len(s.messages)
			}
			s.plan = make([]agentPlan, len(plan.Entries))
			for index, entry := range plan.Entries {
				s.plan[index] = agentPlan{content: entry.Content, status: string(entry.Status)}
			}
		})
		return
	}
	if tool := update.Update.ToolCall; tool != nil {
		h.addAgentTool(string(update.SessionId), tool)
		return
	}
	if patch := update.Update.ToolCallUpdate; patch != nil {
		h.patchAgentTool(string(update.SessionId), patch)
		return
	}
	if update.Update.CurrentModeUpdate != nil || update.Update.ConfigOptionUpdate != nil {
		h.post(func() {
			a, s := h.workspace(), h.state()
			if s.sessionID != "" && s.sessionID != string(update.SessionId) {
				return
			}
			if update.Update.CurrentModeUpdate != nil {
				a.refreshAgentModes(s)
			} else {
				a.refreshAgentModels(s)
			}
		})
	}
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
		chat := h.state()
		if ctx.Err() != nil || chat.stopping {
			return
		}
		fillChatPermission(chat, p)
		if p.title == "" {
			p.title = r.ToolCall.ID
		}
		chat.permissions = append(chat.permissions, p)
		if a := h.workspace(); !h.meeting && a.agent == chat && !a.chatShown(chat) {
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
			output := tool.outputText()
			p.detail = formatAgentToolDetail(tool.locations, tool.rawInput) + output
			for _, change := range tool.changes {
				p.detail += agentContextText(change.path, 0, 0, change.newText) + "\n"
			}
			p.detail = boundedAgentText(p.detail)
			p.preview = previewPermission(tool.rawInput, tool.locations, p.title, output != "" || len(tool.changes) > 0)
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

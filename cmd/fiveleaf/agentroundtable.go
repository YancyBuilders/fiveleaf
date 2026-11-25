package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/bg"
	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type agentMeeting struct {
	room      *roundtable.Room
	chat      *agentChat
	chatTask  string
	reviews   map[string]map[string][]agentChange
	library   roundtable.Library
	readOnly  bool
	group     roundtable.Group
	speakers  []meetingSpeaker
	providers []agentProvider
	roles     map[string]settings.AgentRole
	presets   map[string]settings.AgentRoleOverride
	cancel    context.CancelFunc
	changing  <-chan struct{}
	change    meetingChange
}

type meetingChange uint8

const (
	meetingMemoryChange meetingChange = iota
	meetingPublish
	meetingClear
)

type meetingRecordKey struct {
	archive, id string
}

func (m *agentMeeting) key() meetingRecordKey { return meetingRecordKey{m.library.Path(), m.group.ID} }

var closingMeetings = struct {
	sync.Mutex
	rooms map[meetingRecordKey][]<-chan struct{}
}{rooms: make(map[meetingRecordKey][]<-chan struct{})}

func pendingMeetingCloses(key meetingRecordKey) []<-chan struct{} {
	closingMeetings.Lock()
	defer closingMeetings.Unlock()
	return slices.Clone(closingMeetings.rooms[key])
}

func waitMeetingCloses(closes []<-chan struct{}) {
	for _, closed := range closes {
		<-closed
	}
}

func waitForClosingMeetings() {
	closingMeetings.Lock()
	var pending []<-chan struct{}
	for _, closes := range closingMeetings.rooms {
		pending = append(pending, closes...)
	}
	closingMeetings.Unlock()
	waitMeetingCloses(pending)
}

func (a *app) allAgentChats() []*agentChat {
	chats := []*agentChat{a.agent}
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool != nil && tab.tool.chat != nil && !slices.Contains(chats, tab.tool.chat) {
				chats = append(chats, tab.tool.chat)
			}
		}
	}
	for _, closed := range a.closed {
		if closed.chat != nil && !slices.Contains(chats, closed.chat) {
			chats = append(chats, closed.chat)
		}
	}
	for _, s := range a.agentChats {
		if s != nil && !slices.Contains(chats, s) {
			chats = append(chats, s)
		}
	}
	return chats
}

func (a *app) agentState(id string) *agentChat {
	return a.chatTargetState(a.agent, agentTarget{provider: id})
}

func (a *app) agentRoleState(name string) *agentChat {
	return a.chatTargetState(a.agent, agentTarget{role: name})
}

func (a *app) chatTargetState(chat *agentChat, target agentTarget) *agentChat {
	key := target.key(chat.group)
	if key == chat.agentTarget.key(chat.group) {
		return chat
	}
	return a.agentChats[key]
}

func (a *app) agentPermissionCount() int {
	count := 0
	for _, s := range a.allAgentChats() {
		count += len(s.permissions)
	}
	return count
}

func (a *app) closeAllAgents() {
	a.cancelCopilot()
	a.closeRoundtable()
	a.closeMeeting()
	for _, s := range a.allAgentChats() {
		a.discardChat(s, false)
	}
	a.agentChats = nil
	a.agent = &agentChat{contextMessages: 10}
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool != nil && tab.tool.chat != nil {
				chat := a.blankChat()
				chat.lastFocused = tab.tool.chat.lastFocused
				tab.tool.chat = chat
			}
		}
	}
	if chat := a.lastOpenChat(); chat != nil {
		a.agent = chat
	}
}

func (a *app) setAgentEnabled(id string, enabled bool) {
	for _, p := range a.agentProviders() {
		if p.id != id {
			continue
		}
		servers := maps.Clone(a.settings.AgentServers)
		if servers == nil {
			servers = map[string]settings.AgentServer{}
		}
		p.config.Disabled = !enabled
		servers[id] = p.config
		raw, err := json.Marshal(servers)
		if err == nil {
			err = a.settings.Prefs.Set("agentServers", raw)
		}
		if err != nil {
			a.agent.err = err.Error()
			return
		}
		a.settings.Changed()
		a.checkAgentConfig()
		a.invalidate()
		// Definitions are shared by all projects. Check them now: a hidden
		// project may have no frame to cancel a silent subprocess until later.
		for _, other := range a.host.everyProject() {
			if other != a {
				other.checkAgentConfig()
				other.invalidate()
			}
		}
		return
	}
}

func cloneAgentProviders(in []agentProvider) []agentProvider {
	out := slices.Clone(in)
	for i := range out {
		out[i].config = cloneAgentServer(out[i].config)
	}
	return out
}

func (a *app) startMeeting(lib roundtable.Library, group roundtable.Group) (*agentMeeting, error) {
	key := meetingRecordKey{lib.Path(), group.ID}
	if running := a.backgroundMeetings[key]; running != nil {
		delete(a.backgroundMeetings, key)
		a.leaveMeeting()
		a.meeting = running
		return running, nil
	}
	m := &agentMeeting{group: group, library: lib, readOnly: !lib.Owns(a.root), chat: &agentChat{}}
	var members []roundtable.Member
	if m.readOnly {
		for _, ref := range group.Members {
			m.speakers = append(m.speakers, meetingSpeaker{Participant: ref, icon: iconComment})
		}
	} else {
		if moderator, ok := a.legacyMeetingModerator(m); ok {
			updated, err := lib.EnsureModerator(group.ID, moderator)
			if err != nil {
				return nil, err
			}
			m.group = updated
		}
		members = a.meetingMembers(m, a.agentProviders())
	}
	room, err := lib.OpenRoom(m.group, members)
	if err != nil {
		return nil, err
	}
	a.leaveMeeting()
	m.room = room
	a.watchMeeting(m)
	if !m.readOnly {
		bg.Go(func() {
			if err := room.SyncMemoryDays(); err != nil && !errors.Is(err, roundtable.ErrClosed) {
				a.post(func() {
					if a.meeting == m && !a.loadsClosed {
						a.fail("%s: %s", locales.RoundtableGroupMemory(), err.Error())
					}
				})
			}
		})
	}
	return m, nil
}

func (a *app) meetingModerator() meetingSpeaker {
	for _, choice := range a.meetingInvitations() {
		if choice.ID == roundtable.ModeratorID {
			return choice
		}
	}
	return meetingSpeaker{}
}

func (a *app) legacyMeetingModerator(meeting *agentMeeting) (roundtable.Participant, bool) {
	if meeting.group.Options.HumanChair || meeting.group.HasModerator() {
		return roundtable.Participant{}, false
	}
	choice := a.meetingModerator()
	return choice.Participant, choice.Provider != ""
}

func (a *app) bindMeetingModerator(meeting *agentMeeting, moderator roundtable.Participant, takeChair bool, providers []agentProvider) error {
	return meeting.room.BindModerator(moderator, takeChair, func(group roundtable.Group) []roundtable.Member {
		meeting.group = group
		if takeChair {
			meeting.speakers = nil
		}
		return a.meetingMembers(meeting, providers)
	})
}

func (a *app) watchMeeting(m *agentMeeting) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	a.meeting = m
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-m.room.Events():
				if !ok {
					return
				}
				a.post(func() { a.releaseFinishedMeeting(m) })
			}
		}
	}()
}

func (a *app) releaseFinishedMeeting(m *agentMeeting) {
	key := m.key()
	if a.backgroundMeetings[key] == m && m.changing == nil && !m.room.Busy() {
		delete(a.backgroundMeetings, key)
		a.releaseMeeting(m)
		a.readSessionsNow()
	}
}

func (a *app) leaveMeeting() {
	m := a.meeting
	if m == nil {
		return
	}
	a.meeting = nil
	if m.changing == nil && !m.room.Busy() {
		a.releaseMeeting(m)
		return
	}
	if a.backgroundMeetings == nil {
		a.backgroundMeetings = make(map[meetingRecordKey]*agentMeeting)
	}
	a.backgroundMeetings[m.key()] = m
}

func (a *app) releaseMeeting(m *agentMeeting) {
	m.cancel()
	a.disconnectChat(m.chat)
	key, changing := m.key(), m.changing
	closed := make(chan struct{})
	closingMeetings.Lock()
	closingMeetings.rooms[key] = append(closingMeetings.rooms[key], closed)
	closingMeetings.Unlock()
	var flushed <-chan struct{}
	if changing == nil {
		flushed = m.room.CloseAsync()
	}
	bg.Go(func() {
		if changing == nil {
			<-flushed
		} else {
			<-changing
			m.room.Close()
		}
		closingMeetings.Lock()
		remaining := slices.DeleteFunc(closingMeetings.rooms[key], func(ticket <-chan struct{}) bool { return ticket == closed })
		if len(remaining) == 0 {
			delete(closingMeetings.rooms, key)
		} else {
			closingMeetings.rooms[key] = remaining
		}
		closingMeetings.Unlock()
		close(closed)
	})
}

func (a *app) closeMeeting() {
	a.meetingOpenSerial++
	if m := a.meeting; m != nil {
		a.meeting = nil
		a.releaseMeeting(m)
	}
	for key, m := range a.backgroundMeetings {
		delete(a.backgroundMeetings, key)
		a.releaseMeeting(m)
	}
}

func (a *app) ownsMeeting(m *agentMeeting) bool {
	return m != nil && (a.meeting == m || a.backgroundMeetings[m.key()] == m)
}

func (a *app) checkMeetingConfig(providers []agentProvider) {
	a.syncMeetingConfig(a.meeting, providers)
	for _, m := range a.backgroundMeetings {
		a.syncMeetingConfig(m, providers)
	}
}

func (a *app) syncMeetingConfig(m *agentMeeting, providers []agentProvider) {
	if m != nil && !m.readOnly && (!reflect.DeepEqual(m.providers, providers) || !maps.Equal(m.roles, a.settings.AgentRoles) || !reflect.DeepEqual(m.presets, a.settings.AgentRolePresets)) {
		if !m.library.Owns(a.root) {
			return
		}
		if moderator, ok := a.legacyMeetingModerator(m); ok {
			if err := a.bindMeetingModerator(m, moderator, false, providers); err != nil {
				a.fail("%s", err.Error())
			} else {
				return
			}
		}
		m.room.SetMembers(a.meetingMembers(m, providers))
	}
}

func agentWorkingDirectory(root string, server settings.AgentServer) string {
	if server.WorkingDirectory == "" {
		return root
	}
	if filepath.IsAbs(server.WorkingDirectory) {
		return server.WorkingDirectory
	}
	return filepath.Join(root, server.WorkingDirectory)
}

func cliAgentAdapter(root string, provider agentProvider) *roundtable.CLIAdapter {
	server := provider.config
	return &roundtable.CLIAdapter{Command: server.Command, Args: slices.Clone(server.Args), Env: maps.Clone(server.Env),
		Dir: agentWorkingDirectory(root, server), Timeout: time.Duration(server.ReplyTimeoutSeconds()) * time.Second,
		PromptArgument: server.PromptArgument}
}

func (a *app) adapterFor(p agentProvider, m *agentMeeting, model string) roundtable.Adapter {
	root := a.root
	if p.config.EffectiveProtocol() == "cli" {
		return cliAgentAdapter(root, p)
	}
	return roundtable.AdapterFunc(func(ctx context.Context, messages []roundtable.Message) (roundtable.Message, error) {
		if err := p.config.Validate(); err != nil {
			return roundtable.Message{}, err
		}
		call, cancel := context.WithTimeout(ctx, time.Duration(p.config.ReplyTimeoutSeconds())*time.Second)
		defer cancel()
		s := &agentChat{agentTarget: agentTarget{provider: p.id}, generation: 1, busy: true, connected: &p}
		h := &meetingACPHandler{agentHandler: &agentHandler{app: a, chat: s, root: root, generation: s.generation, meeting: true, meetingLife: call}}
		if roundtable.TaskID(call) != "" {
			h.toolChanged = func(tool *agentTool) {
				detail := a.agentToolInputSummary(tool)
				if len(tool.changes) != 0 {
					detail = a.relPath(tool.changes[0].path)
				}
				if tool.text != "" {
					preview, _ := agentVisibleText(tool.text, 4096)
					detail = strings.TrimSpace(detail + "\n" + preview)
				}
				h.mu.Lock()
				note := h.toolNotes[tool.id]
				h.mu.Unlock()
				tool.note = note
				if len(tool.changes) != 0 && a.ownsMeeting(m) && m.chat == s {
					id := roundtable.TaskID(call)
					if m.reviews == nil {
						m.reviews = map[string]map[string][]agentChange{}
					}
					if m.reviews[id] == nil {
						m.reviews[id] = map[string][]agentChange{}
					}
					m.reviews[id][tool.id] = slices.Clone(tool.changes)
				}
				roundtable.ReportTool(call, roundtable.ToolRecord{ID: tool.id, Title: tool.title, Kind: tool.kind, Status: tool.status, Detail: detail, Note: note, InputOnly: strings.TrimSpace(tool.text) == "" && len(tool.changes) == 0})
			}
		}
		if err := h.onMain(call, func() error {
			if !a.ownsMeeting(m) || a.root == "" || a.conn != nil {
				return context.Canceled
			}
			m.chat, m.chatTask = s, roundtable.TaskID(call)
			return nil
		}); err != nil {
			return roundtable.Message{}, err
		}
		defer a.post(func() {
			s.busy = false
			s.permissions = nil
			s.generation++
			a.gitChanged()
		})
		client, err := acp.Start(call, agentACPConfig(p), root, h)
		if err != nil {
			return roundtable.Message{}, err
		}
		defer client.Close()
		if _, err := client.Initialize(call); err != nil {
			return roundtable.Message{}, err
		}
		session, err := client.NewSession(call)
		if err != nil {
			return roundtable.Message{}, err
		}
		if model != "" {
			if err := client.SetModel(call, string(session.SessionId), model); err != nil {
				return roundtable.Message{}, err
			}
		}
		if err := h.onMain(call, func() error { a.acceptChatSession(s, session); return nil }); err != nil {
			return roundtable.Message{}, err
		}
		h.mu.Lock()
		h.session, h.prelude = session.SessionId, acp.StartupInfo(session)
		h.mu.Unlock()
		result, err := client.Prompt(call, string(session.SessionId), []acp.ContentBlock{acp.TextBlock(roundtable.FormatPrompt(messages))})
		if err != nil {
			return roundtable.Message{}, err
		}
		if result.StopReason != "end_turn" {
			return roundtable.Message{}, fmt.Errorf("agent stopped: %s", result.StopReason)
		}
		var usedTools bool
		if err := h.onMain(call, func() error {
			usedTools = len(s.tools) != 0
			return nil
		}); err != nil {
			return roundtable.Message{}, err
		}
		h.mu.Lock()
		text := strings.TrimPrefix(h.text.String(), h.prelude)
		overflow := h.overflow
		h.mu.Unlock()
		if overflow {
			return roundtable.Message{}, roundtable.ErrTooLarge
		}
		return roundtable.Message{Role: "assistant", AgentID: p.id, Name: p.name, Text: text, ToolOnly: usedTools && strings.TrimSpace(text) == ""}, nil
	})
}

// Only public answer chunks enter the shared transcript. Thoughts, tools and
// permissions retain their existing editor UI and are not routed as speech.
type meetingACPHandler struct {
	*agentHandler
	mu        sync.Mutex
	session   acp.SessionID
	prelude   string
	text      strings.Builder
	toolNotes map[string]string
	overflow  bool
}

func (h *meetingACPHandler) Update(update acp.Update) {
	if chunk := update.Update.AgentMessageChunk; chunk != nil && chunk.Content.Text != nil {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.session != "" && h.session == update.SessionId && !h.overflow {
			text := chunk.Content.Text.Text
			if h.prelude != "" && text == h.prelude {
				h.prelude = ""
				return
			}
			if h.text.Len()+len(text) > roundtable.MaxMessageBytes {
				h.overflow = true
			} else {
				h.text.WriteString(text)
			}
		}
		return
	}
	if tool := update.Update.ToolCall; tool != nil {
		h.mu.Lock()
		if h.session != "" && update.SessionId == h.session && h.text.Len() != 0 {
			if h.toolNotes == nil {
				h.toolNotes = make(map[string]string)
			}
			if len(h.toolNotes) < 64 {
				note, _ := agentVisibleText(strings.TrimSpace(strings.TrimPrefix(h.text.String(), h.prelude)), 1024)
				h.toolNotes[string(tool.ToolCallId)] = note
			}
			h.text.Reset()
			h.overflow = false
		}
		h.mu.Unlock()
	}
	h.agentHandler.Update(update)
}

func chatContext(s *agentChat) []roundtable.Message {
	var messages []roundtable.Message
	previousRole := ""
	for _, msg := range s.messages {
		if msg == nil || msg.role == "tool" || msg.historical {
			continue
		}
		if msg.role == "user" || msg.role == "assistant" {
			// Tool rows split the display, not the reply's context budget.
			if msg.role == "assistant" && previousRole == "assistant" {
				last := &messages[len(messages)-1]
				last.Text = boundedAgentText(last.Text + msg.text)
			} else {
				messages = append(messages, roundtable.Message{Role: msg.role, Text: msg.text})
			}
		}
		previousRole = msg.role
	}
	n := s.contextMessages
	if n <= 0 {
		n = 10
	}
	handoff := ""
	if s.continuation != nil {
		handoff = s.continuation.context
	}
	limit := 100
	if handoff != "" {
		limit--
	}
	messages = messages[max(0, len(messages)-min(n, limit)):]
	// Also bound bytes: a handful of large code excerpts must not produce an
	// unbounded subprocess prompt. Keep complete newest messages in order.
	total, from := 0, len(messages)
	for from > 0 && total+len(messages[from-1].Text) <= agentTextLimit-len(handoff) {
		from--
		total += len(messages[from].Text)
	}
	messages = slices.Clone(messages[from:])
	if handoff != "" {
		messages = append([]roundtable.Message{{Role: "user", Text: handoff}}, messages...)
	}
	systemPrompt := s.systemPrompt
	if s.connectedRole != nil {
		systemPrompt = s.connectedRole.Prompt
	}
	if systemPrompt != "" {
		messages = append([]roundtable.Message{{Role: "system", Text: boundedAgentText(systemPrompt)}}, messages...)
	}
	return messages
}

func (a *app) sendCLIChat(s *agentChat, p agentProvider, role *settings.AgentRole, text string) {
	if err := p.config.Validate(); err != nil {
		s.err = err.Error()
		return
	}
	effective := p
	if role != nil {
		var err error
		effective, err = agentRoleCLIProvider(p, role.Model)
		if err != nil {
			s.err = err.Error()
			return
		}
	}
	a.disconnectChat(s)
	s.connected, s.connectedRole, s.err = &p, role, ""
	if strings.TrimSpace(text) == "" {
		return
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.stageDetailSources()
	s.busy, s.draft = true, ""
	s.pendingPrompt = text
	s.turn++
	s.messages = append(s.messages, &agentMessage{role: "user", text: text})
	s.recordDetailSources()
	ctx, messages := s.ctx, chatContext(s)
	h := a.chatHandler(s)
	adapter := a.adapterFor(effective, nil, "")
	go func() {
		reply, err := adapter.Call(ctx, messages)
		h.post(func() {
			s.busy = false
			s.pendingPrompt = ""
			switch {
			case errors.Is(err, context.Canceled):
				s.lastTask = agentTaskIdle
			case err != nil:
				s.lastTask = agentTaskFailed
				s.err = err.Error()
				restoreChatDraft(s, text)
			default:
				s.lastTask = agentTaskCompleted
				s.messages = append(s.messages, &agentMessage{role: "assistant", text: boundedAgentText(reply.Text)})
			}
			s.sources.pending = chatSourceDraft{}
			h.workspace().gitChanged()
		})
	}()
}

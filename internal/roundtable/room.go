// Package roundtable schedules bounded conversations through independent agent adapters.
package roundtable

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	MaxMessageBytes      = 2 << 20
	MaxSystemPromptBytes = 64 << 10
	MaxHistoryBytes      = 8 << 20
	MaxHistoryMessages   = 200
	MaxContextMessages   = 100
	MaxMembers           = 64
	MaxHops              = 3
	MaxTasksPerTurn      = 12
)

var (
	ErrClosed               = errors.New("roundtable is closed")
	ErrNoMembers            = errors.New("no enabled agent is available")
	ErrUnknownAgent         = errors.New("target agent is not available")
	ErrModeratorUnavailable = errors.New("the roundtable moderator is not available")
	ErrInvalidAssignment    = errors.New("the moderator must name one available member by exact ID and give a specific assignment")
	ErrConclusionRequired   = errors.New("the moderator must give the final answer")
	ErrNoAnswer             = errors.New("agent returned an empty reply")
	ErrTooLarge             = errors.New("message exceeds the size limit")
	ErrTurnActive           = errors.New("finish or stop the current turn first")
	ErrHumanChairRequired   = errors.New("the human must chair the meeting to publish a conclusion")
)

type Message struct {
	ID, Role, AgentID, Name, Text string
	Time                          time.Time
	Hop                           int
	Task                          *Task  `json:",omitempty"`
	Original                      string `json:"-"`
	ToolOnly                      bool   `json:"-"`
	HumanConclusion               bool   `json:"humanConclusion,omitempty"`
}

type Adapter interface {
	Call(context.Context, []Message) (Message, error)
}

type AdapterFunc func(context.Context, []Message) (Message, error)

func (f AdapterFunc) Call(ctx context.Context, messages []Message) (Message, error) {
	return f(ctx, messages)
}

type Member struct {
	ID, Name     string
	Capabilities []string
	Enabled      bool
	Adapter      Adapter
}

type Options struct {
	ContextMessages int
	MaxHops         int
	HumanChair      bool `json:"humanChair,omitempty"`
}

type Snapshot struct {
	Messages     []Message
	ActiveTask   Message
	HasEarlier   bool
	Busy         bool
	HumanChair   bool
	ActiveAgent  string
	Error        string
	SystemPrompt string
}

type chain struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	target     string
	text       string
	lastMember string
	hop        int
	inflight   bool
	discussion *discussion
	task       Message
}

type reply struct {
	generation uint64
	member     Member
	hop        int
	message    Message
	err        error
}

// Room owns its history and cancellation lifetime. Public methods never wait for
// an adapter; a wake channel coalesces scheduling work on the room's goroutine.
type Room struct {
	mu         sync.Mutex
	members    []Member
	options    Options
	memory     memoryRecord
	daily      dailyMemory
	dayChanges memoryDayChanges
	recent     []Message
	messages   []Message
	archive    *archive
	earlier    bool
	system     string
	active     *Member
	err        string
	chain      *chain
	generation uint64
	next       int
	closeOnce  sync.Once
	wake       chan struct{}
	checkpoint chan struct{}
	results    chan reply
	events     chan struct{}
	done       chan struct{}
	flushed    chan struct{}
}

func New(members []Member, opts Options) *Room {
	r := newRoom(members, opts)
	go r.run()
	return r
}

func newRoom(members []Member, opts Options) *Room {
	return &Room{
		members: cloneMembers(members), options: normalizeOptions(opts),
		wake: make(chan struct{}, 1), checkpoint: make(chan struct{}, 1), results: make(chan reply, 1),
		events: make(chan struct{}, 1), done: make(chan struct{}), flushed: make(chan struct{}),
	}
}

func normalizeOptions(opts Options) Options {
	if opts.ContextMessages <= 0 {
		opts.ContextMessages = 10
	}
	if opts.MaxHops <= 0 {
		opts.MaxHops = MaxHops
	}
	opts.ContextMessages = min(opts.ContextMessages, MaxContextMessages)
	opts.MaxHops = min(opts.MaxHops, MaxTasksPerTurn)
	return opts
}

func cloneMembers(members []Member) []Member {
	limit := MaxMembers
	if slices.ContainsFunc(members, func(member Member) bool { return member.ID == ModeratorID }) {
		limit++
	}
	result := make([]Member, 0, min(len(members), limit))
	seen := map[string]bool{}
	for _, member := range members {
		if len(result) == limit {
			break
		}
		if strings.TrimSpace(member.ID) == "" || seen[member.ID] || len(member.ID)+len(member.Name) > 4096 {
			continue
		}
		seen[member.ID] = true
		member.Capabilities = slices.Clone(member.Capabilities)
		if member.Name == "" {
			member.Name = member.ID
		}
		result = append(result, member)
	}
	return result
}

func available(member Member) bool {
	if !member.Enabled || member.Adapter == nil {
		return false
	}
	v := reflect.ValueOf(member.Adapter)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}

func sameAdapter(a, b Adapter) bool {
	return a != nil && b != nil && reflect.TypeOf(a) == reflect.TypeOf(b) && reflect.TypeOf(a).Comparable() && a == b
}

// Send interrupts an older chain only after the new text and target are valid.
// A named target in a saved group can explicitly hand off to another member.
func (r *Room) Send(text, target string) error {
	return r.send(text, target, nil)
}

func (r *Room) Discuss(topic, text, target string) error {
	return r.send(text, target, &discussion{topic: topic, goal: text, assignment: text})
}

func validateHumanText(text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("message is empty")
	}
	if len(text) > MaxMessageBytes {
		return ErrTooLarge
	}
	if !utf8.ValidString(text) {
		return errors.New("message must be UTF-8 text")
	}
	return nil
}

func (r *Room) send(text, target string, work *discussion) error {
	if err := validateHumanText(text); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed() {
		return ErrClosed
	}
	if target != "" {
		member, ok := directMember(r.members, target)
		if !ok {
			return ErrUnknownAgent
		}
		target = member.ID
	} else if hasLeadingMention(text) {
		index := mentionedMember(r.members, text)
		if index < 0 {
			return ErrUnknownAgent
		}
		if work != nil {
			target = r.members[index].ID
		}
	} else if work != nil {
		if _, ok := exactMember(r.members, ModeratorID); !ok && !r.options.HumanChair {
			return ErrModeratorUnavailable
		}
	} else if !slices.ContainsFunc(r.members, available) {
		return ErrNoMembers
	}
	user, err := r.recordHumanLocked(text, false)
	if err != nil {
		return err
	}
	if work != nil && r.options.HumanChair && target == "" {
		r.notifyLocked()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	if work != nil {
		work.next, work.stage = target, Direct
		if target == "" {
			work.next, work.stage = ModeratorID, Coordinate
		}
		work.turnID, work.remaining = user.ID, r.options.MaxHops-1
		work.humanChair = r.options.HumanChair
		if work.humanChair {
			work.remaining = 0
		}
	}
	r.chain = &chain{ctx: ctx, cancel: cancel, generation: r.generation, target: target, text: text, discussion: work}
	r.notifyLocked()
	r.scheduleLocked()
	return nil
}

func (r *Room) recordHumanLocked(text string, conclusion bool) (Message, error) {
	user := Message{ID: rand.Text(), Role: "user", Text: text, Time: r.orderedTime(time.Now()), HumanConclusion: conclusion}
	if r.chain != nil && !user.Time.After(r.chain.task.Time) {
		user.Time = r.chain.task.Time.Add(time.Nanosecond)
	}
	if err := r.persistLocked(user); err != nil {
		return Message{}, err
	}
	r.stopLocked()
	r.appendLocked(user)
	r.err = ""
	return user, nil
}

func (r *Room) PublishConclusion(text string) error {
	return r.publishConclusion(text, r.flushMemoryDayChanges)
}

func (r *Room) PublishConclusionWithDeferredMemory(text string) error {
	return r.publishConclusion(text, func() { go r.flushMemoryDayChanges() })
}

func (r *Room) publishConclusion(text string, flush func()) error {
	if err := validateHumanText(text); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.unlockMemoryDayChangesWith(flush)
	switch {
	case r.closed():
		return ErrClosed
	case !r.options.HumanChair:
		return ErrHumanChairRequired
	case r.chain != nil:
		return ErrTurnActive
	}
	if _, err := r.recordHumanLocked(text, true); err != nil {
		return err
	}
	r.notifyLocked()
	return nil
}

func (r *Room) SetMembers(members []Member) {
	members = cloneMembers(members)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed() {
		return
	}
	r.setMembersLocked(members)
}

func (r *Room) setMembersLocked(members []Member) {
	r.members = members
	if current := r.chain; current != nil && current.discussion != nil && current.discussion.stage != Direct {
		if _, ok := exactMember(members, ModeratorID); !ok {
			r.err = ErrModeratorUnavailable.Error()
			r.stopLocked()
		}
	}
	if r.chain != nil && r.active != nil {
		member, ok := exactMember(members, r.active.ID)
		if !ok || !sameAdapter(member.Adapter, r.active.Adapter) {
			r.stopLocked()
		}
	}
	r.notifyLocked()
	r.scheduleLocked()
}

func (r *Room) SetOptions(opts Options) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setOptionsLocked(opts)
}

func (r *Room) SetHumanChair(human bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	options := r.options
	options.HumanChair = human
	return r.setOptionsLocked(options)
}

func (r *Room) setOptionsLocked(opts Options) error {
	if r.closed() {
		return ErrClosed
	}
	if r.options.HumanChair != opts.HumanChair {
		if r.chain != nil {
			return ErrTurnActive
		}
		if _, ok := exactMember(r.members, ModeratorID); !opts.HumanChair && !ok {
			return ErrModeratorUnavailable
		}
	}
	opts = normalizeOptions(opts)
	if r.chain != nil && r.memory.Window == 0 && opts.ContextMessages != r.options.ContextMessages {
		return ErrMemoryBusy
	}
	if opts != r.options {
		if r.archive != nil {
			if err := r.archive.configure(opts, r.system); err != nil {
				return err
			}
		}
		r.options = opts
		r.notifyLocked()
	}
	return nil
}

func (r *Room) SetSystemPrompt(text string) error {
	if len(text) > MaxSystemPromptBytes {
		return ErrTooLarge
	}
	if !utf8.ValidString(text) {
		return errors.New("system prompt must be UTF-8 text")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed() {
		return ErrClosed
	}
	if r.system != text {
		if r.archive != nil {
			if err := r.archive.configure(r.options, text); err != nil {
				return err
			}
		}
		r.stopLocked()
		r.system = text
		r.err = ""
		r.notifyLocked()
	}
	return nil
}

func (r *Room) Clear() error {
	r.daily.mu.Lock()
	defer r.daily.mu.Unlock()
	r.mu.Lock()
	defer func() {
		r.mu.Unlock()
		_ = r.syncMemoryDays(false)
	}()
	if r.closed() {
		return ErrClosed
	}
	cleared := r.clearedMemoryLocked()
	if r.archive != nil {
		if err := r.archive.clear(cleared); err != nil {
			return err
		}
	}
	r.memory = cleared
	if r.chain != nil {
		r.chain.task = Message{}
	}
	r.stopLocked()
	r.messages, r.recent, r.err, r.next, r.earlier = nil, nil, "", 0, false
	r.dayChanges = memoryDayChanges{reset: true}
	r.notifyLocked()
	return nil
}

func (r *Room) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed() {
		r.stopLocked()
		r.notifyLocked()
	}
}

func (r *Room) Close() {
	<-r.CloseAsync()
}

func (r *Room) CloseAsync() <-chan struct{} {
	r.closeOnce.Do(func() {
		close(r.done)
		go func() {
			r.mu.Lock()
			r.stopLocked()
			close(r.events)
			r.mu.Unlock()
			r.flushMemoryDayChanges()
			close(r.flushed)
		}()
	})
	return r.flushed
}

func (r *Room) closed() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

func (r *Room) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := Snapshot{Messages: slices.Clone(r.messages), HasEarlier: r.earlier, Busy: r.chain != nil, HumanChair: r.options.HumanChair, Error: r.err, SystemPrompt: r.system}
	if r.active != nil {
		snapshot.ActiveAgent = r.active.ID
		snapshot.ActiveTask = r.chain.task
	}
	return snapshot
}

func (r *Room) Busy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.chain != nil
}

// Events is a coalesced wake signal: read Snapshot for current state. It closes
// when the room closes; callers must stop listening when a receive returns false.
func (r *Room) Events() <-chan struct{} { return r.events }

func (r *Room) stopLocked() {
	r.generation++
	if r.chain != nil {
		r.chain.cancel()
		r.finishTaskLocked(Stopped, "")
	}
	r.chain, r.active = nil, nil
}

func (r *Room) notifyLocked() {
	select {
	case r.events <- struct{}{}:
	default:
	}
}

func (r *Room) scheduleLocked() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Room) recordLocked(message Message) error {
	message.Time = r.orderedTime(message.Time)
	if err := r.persistLocked(message); err != nil {
		return err
	}
	r.appendLocked(message)
	return nil
}

func (r *Room) orderedTime(now time.Time) time.Time {
	if !now.After(r.memory.After) {
		now = r.memory.After.Add(time.Nanosecond)
	}
	if count := len(r.messages); count > 0 && !now.After(r.messages[count-1].Time) {
		return r.messages[count-1].Time.Add(time.Nanosecond)
	}
	return now
}

func (r *Room) persistLocked(message Message) error {
	if r.archive != nil {
		if err := r.archive.append(message); err != nil {
			return fmt.Errorf("saving roundtable message: %w", err)
		}
		r.queueConclusionDayLocked(message)
	}
	return nil
}

func messageSize(message Message) int {
	return max(len(message.Text), len(message.Original)) + len(message.ID) + len(message.AgentID) + len(message.Name) + len(message.Role) + taskSize(message.Task)
}

func (r *Room) appendLocked(message Message) {
	r.rememberLocked(message)
	r.messages = append(r.messages, message)
	bytes, start := 0, len(r.messages)
	for start > 0 && len(r.messages)-start < MaxHistoryMessages {
		size := messageSize(r.messages[start-1])
		if bytes+size > MaxHistoryBytes {
			break
		}
		bytes, start = bytes+size, start-1
	}
	if start > 0 {
		r.earlier = r.archive != nil
		r.messages = slices.Clone(r.messages[start:])
	}
}

func (r *Room) contextLocked() []Message {
	var messages []Message
	for index := len(r.messages) - 1; index >= 0 && len(messages) < r.options.ContextMessages; index-- {
		message := r.messages[index]
		if !message.Time.After(r.memory.After) {
			break
		}
		clean, ok := memoryMessage(message)
		if !ok {
			continue
		}
		message.Task, message.Original = nil, ""
		if message.Role == "assistant" && r.messages[index].Task == nil {
			message.Text = legacyPiAnswer(message.Text)
		}
		messages = append(messages, message)
		if r.memory.Paused && clean.Role == "user" {
			break
		}
	}
	slices.Reverse(messages)
	bytes := 0
	for i := len(messages) - 1; i >= 0; i-- {
		bytes += len(messages[i].Text)
		if bytes > MaxPromptBytes-MaxSystemPromptBytes-4096 {
			messages = messages[i+1:]
			break
		}
	}
	if r.system != "" {
		messages = append([]Message{{ID: "system", Role: "system", Text: r.system}}, messages...)
	}
	return messages
}

func (r *Room) run() {
	for {
		select {
		case <-r.done:
			return
		case result := <-r.results:
			r.accept(result)
		case <-r.wake:
			r.dispatch()
		case <-r.checkpoint:
			r.saveProgress()
		}
	}
}

func (r *Room) dispatch() {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.chain
	if r.closed() || c == nil || c.inflight {
		return
	}
	if c.discussion == nil && c.hop >= min(MaxHops, r.options.MaxHops) {
		r.stopLocked()
		r.notifyLocked()
		return
	}
	var member Member
	var ok bool
	var stage string
	if c.discussion != nil {
		member, ok = exactMember(r.members, c.discussion.next)
		stage = c.discussion.dispatchStage(r.members)
	} else if c.target != "" {
		member, ok = directMember(r.members, c.target)
	} else {
		member, ok = routeAfter(r.members, c.text, &r.next, c.lastMember)
	}
	if !ok {
		if stage == Coordinate || stage == Conclusion {
			r.err = ErrModeratorUnavailable.Error()
		} else if c.target != "" || hasLeadingMention(c.text) || c.discussion != nil {
			r.err = ErrUnknownAgent.Error()
		} else if c.lastMember == "" {
			r.err = ErrNoMembers.Error()
		}
		r.stopLocked()
		r.notifyLocked()
		return
	}
	c.hop++
	c.inflight = true
	r.active = &member
	var messages []Message
	call := c.ctx
	if work := c.discussion; work != nil {
		current := Message{ID: work.turnID, Role: "user", Text: work.goal}
		var notes, priorTurns int
		messages, notes, priorTurns = r.priorContextLocked(work.turnID, current)
		if stage == Coordinate {
			work.roster = r.members
		}
		c.task = Message{ID: rand.Text(), Time: r.orderedTime(time.Now()), Role: "task", AgentID: member.ID, Name: member.Name, Hop: c.hop,
			Task: &Task{TurnID: work.turnID, Assignment: work.assignment, Stage: stage, Status: Running, MemoryNotes: notes, MemoryTurns: priorTurns}}
		if r.archive != nil {
			if err := r.archive.append(c.task); err != nil {
				c.task = Message{}
				r.err = err.Error()
				r.stopLocked()
				r.notifyLocked()
				return
			}
		}
		call = context.WithValue(call, taskContextKey{}, taskReporter{r, c.task.ID})
		messages = append([]Message{{Role: "system", Text: work.brief(stage, r.members)}}, messages...)
	} else {
		messages = r.contextLocked()
	}
	r.notifyLocked()
	result := reply{generation: c.generation, member: member, hop: c.hop}
	go func() {
		result.message, result.err = callAdapter(call, member.Adapter, messages)
		select {
		case r.results <- result:
		case <-c.ctx.Done():
		case <-r.done:
		}
	}()
}

func callAdapter(ctx context.Context, adapter Adapter, messages []Message) (message Message, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("agent adapter panicked: %v", value)
		}
	}()
	return adapter.Call(ctx, messages)
}

func (r *Room) accept(result reply) {
	r.mu.Lock()
	defer r.unlockMemoryDayChanges()
	c := r.chain
	if r.closed() || c == nil || result.generation != c.generation || result.hop != c.hop || !c.inflight {
		return
	}
	err := result.err
	if err == nil {
		switch {
		case len(result.message.Text) > MaxMessageBytes:
			err = ErrTooLarge
		case !utf8.ValidString(result.message.Text):
			err = errors.New("agent returned invalid UTF-8")
		case strings.TrimSpace(result.message.Text) == "" && !(c.discussion != nil && result.message.ToolOnly):
			err = ErrNoAnswer
		}
	}
	if err != nil {
		r.err = result.member.Name + ": " + boundedText(err.Error(), MaxStderrBytes)
		r.finishTaskLocked(Failed, boundedText(err.Error(), MaxStderrBytes))
		r.stopLocked()
		r.notifyLocked()
		return
	}
	if c.discussion != nil {
		if result.message.ToolOnly {
			result.message.Text = ""
			task := *c.task.Task
			task.NoAnswer = true
			c.task.Task = &task
		}
		r.acceptTaskLocked(result.message.Text)
		r.notifyLocked()
		return
	}
	// Agent-supplied identity, timestamps and hop numbers are not authoritative.
	message := Message{ID: rand.Text(), Role: "assistant", AgentID: result.member.ID, Name: result.member.Name,
		Text: result.message.Text, Time: time.Now(), Hop: result.hop}
	if r.archive != nil && hasLeadingMention(message.Text) {
		c.target = ""
	}
	if err := r.recordLocked(message); err != nil {
		r.err = err.Error()
		r.stopLocked()
	} else if c.target != "" || c.hop >= min(MaxHops, r.options.MaxHops) {
		r.stopLocked()
	} else {
		c.text, c.inflight, c.lastMember, r.active = message.Text, false, result.member.ID, nil
		r.scheduleLocked()
	}
	r.notifyLocked()
}

package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"
)

const (
	// MaxMessageBytes bounds one inbound or outbound JSON-RPC message.
	MaxMessageBytes = 8 << 20
	MaxTextBytes    = 4 << 20
	MaxStderrBytes  = 64 << 10
	writeTimeout    = 5 * time.Second
)

var (
	ErrMessageTooLarge = errors.New("ACP message exceeds 8 MiB")
	ErrTextTooLarge    = errors.New("ACP text exceeds 4 MiB")
	ErrUnknownSession  = errors.New("unknown ACP session")
	ErrClosed          = errors.New("ACP connection is closed")
	errWriteTimeout    = errors.New("ACP transport write timed out")
)

type sessionState struct {
	active             bool
	cancelled          bool
	interrupted        bool
	permissions        map[*permissionWait]struct{}
	modes              []ModeChoice
	currentMode        string
	settingMode        bool
	modeRevision       uint64
	legacyModeRevision uint64
	configOptions      []sdk.SessionConfigOption
	legacyModels       map[string]bool
	legacyChoices      []ModelChoice
	currentModel       string
	settingModel       bool
	modelRevision      uint64
	usage              *UsageState
	usageRevision      uint64
	commands           []Command
}

type permissionWait struct{ cancel context.CancelFunc }

// Client owns one agent subprocess and its sessions. Run its blocking methods
// off the UI thread. The context passed to Start owns the process lifetime.
type Client struct {
	cmd     *exec.Cmd
	conn    *sdk.Connection
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	root    string
	handler Handler
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	tail    tailBuffer
	stop    sync.Once

	mu           sync.Mutex
	initialized  bool
	initResult   InitializeResult
	sessions     map[string]*sessionState
	userClosed   bool
	transportErr error
	exitErr      error

	setupMu sync.Mutex
	// updateMu preserves order while initial updates wait for session/new.
	updateMu sync.Mutex
	creating bool
	loading  string
	replay   *sessionReplay
	early    []earlySessionUpdate
}

func Start(ctx context.Context, config Config, cwd string, handler Handler) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if config.Command == "" || handler == nil {
		return nil, errors.New("ACP command and editor handler are required")
	}
	if !filepath.IsAbs(cwd) {
		return nil, errors.New("ACP workspace must be an absolute path")
	}
	root, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, fmt.Errorf("ACP workspace: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("ACP workspace must be an existing directory")
	}
	life, cancel := context.WithCancel(ctx)
	c := &Client{root: root, handler: handler, ctx: life, cancel: cancel,
		done: make(chan struct{}), sessions: make(map[string]*sessionState)}
	c.cmd = exec.Command(config.Command, config.Args...)
	c.cmd.Dir = root
	if config.WorkingDirectory != "" {
		dir := config.WorkingDirectory
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		c.cmd.Dir = dir
	}
	c.cmd.Env = os.Environ()
	for name, value := range config.Env {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, '\x00') {
			cancel()
			return nil, errors.New("invalid ACP environment variable")
		}
		c.cmd.Env = append(c.cmd.Env, name+"="+value)
	}
	c.cmd.Stderr = &c.tail
	c.cmd.WaitDelay = 2 * time.Second
	prepareProcess(c.cmd)
	if c.stdin, err = c.cmd.StdinPipe(); err != nil {
		cancel()
		return nil, err
	}
	if c.stdout, err = c.cmd.StdoutPipe(); err != nil {
		c.stdin.Close()
		cancel()
		return nil, err
	}
	if err := c.cmd.Start(); err != nil {
		c.stdin.Close()
		c.stdout.Close()
		cancel()
		return nil, fmt.Errorf("start ACP agent: %w", err)
	}
	// Hold reads until the SDK logger is installed; protocol payloads can
	// contain source code or credentials and must not reach the default log.
	ready := make(chan struct{})
	r := &messageReader{reader: c.stdout, ready: ready, failed: c.recordTransportError}
	c.conn = sdk.NewConnection(c.handle, messageWriter{c}, r)
	c.conn.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	close(ready)
	go c.wait()
	go func() {
		select {
		case <-life.Done():
			c.shutdown(true)
		case <-c.conn.Done():
			c.mu.Lock()
			if !c.userClosed && c.transportErr == nil {
				c.transportErr = errors.New("ACP agent disconnected")
			}
			c.mu.Unlock()
			c.shutdown(false)
		}
	}()
	return c, nil
}

func (c *Client) wait() {
	err := c.cmd.Wait()
	c.mu.Lock()
	if !c.userClosed {
		if err != nil {
			c.exitErr = fmt.Errorf("ACP agent exited: %w", err)
		} else if c.transportErr == nil {
			c.exitErr = errors.New("ACP agent exited")
		}
	}
	c.mu.Unlock()
	c.cancel()
	c.stdin.Close()
	c.stdout.Close()
	close(c.done)
}

func (c *Client) shutdown(user bool) {
	if user {
		c.mu.Lock()
		c.userClosed = true
		c.mu.Unlock()
	}
	c.stop.Do(func() {
		c.cancel()
		stopProcess(c.cmd)
		c.stdin.Close()
		c.stdout.Close()
	})
}

// Close cancels editor callbacks and reaps the subprocess. It is idempotent.
func (c *Client) Close() error {
	c.shutdown(true)
	<-c.done
	return nil
}

func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return errors.Join(c.transportErr, c.exitErr)
}

// Stderr returns only the last 64 KiB, preserving valid UTF-8.
func (c *Client) Stderr() string { return c.tail.String() }

func (c *Client) recordTransportError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transportErr == nil {
		c.transportErr = err
	}
}

type requestHint struct {
	cause   *sdk.RequestError
	message string
}

func (hint *requestHint) Error() string { return hint.message }
func (hint *requestHint) Unwrap() error { return hint.cause }

func request[Response any](c *Client, ctx context.Context, method string, params any) (Response, error) {
	if err := ctx.Err(); err != nil {
		var result Response
		return result, err
	}
	result, err := sdk.SendRequest[Response](c.conn, ctx, method, params)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, ctxErr
		}
		if errors.Is(c.Err(), errWriteTimeout) {
			return result, errWriteTimeout
		}
	}
	var rpc *sdk.RequestError
	if !errors.As(err, &rpc) {
		return result, err
	}
	if rpc.Code == -32000 {
		return result, &requestHint{rpc, "Sign in using the agent's CLI or configure its provider credentials, then reconnect."}
	}
	if rpc.Code != -32603 {
		return result, err
	}
	data, _ := rpc.Data.(map[string]any)
	if data["error"] != "peer disconnected before response" {
		return result, err
	}
	if errors.Is(c.Err(), ErrMessageTooLarge) {
		return result, ErrMessageTooLarge
	}
	launcher := filepath.Base(c.cmd.Path)
	diagnostic := fmt.Sprintf("%s disconnected during %s. Check the agent's launch command and reconnect.", launcher, method)
	stderr := c.Stderr()
	if strings.Contains(stderr, "ENOENT") && strings.Contains(stderr, "_npx") && strings.Contains(stderr, "package.json") {
		diagnostic = fmt.Sprintf("%s disconnected during %s because its npm adapter cache is incomplete. Update the agent plugin or use a fresh npm cache, then reconnect.", launcher, method)
	}
	return result, &requestHint{rpc, diagnostic}
}

func (c *Client) checkReady() error {
	if c.ctx.Err() != nil {
		return ErrClosed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.initialized {
		return errors.New("ACP connection is not initialized")
	}
	return nil
}

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	return c.initialize(ctx, sdk.ClientCapabilities{Fs: sdk.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}})
}

func (c *Client) initialize(ctx context.Context, capabilities sdk.ClientCapabilities) (InitializeResult, error) {
	c.setupMu.Lock()
	defer c.setupMu.Unlock()
	c.mu.Lock()
	if c.initialized {
		result := c.initResult
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()
	result, err := request[InitializeResult](c, ctx, "initialize", sdk.InitializeRequest{
		ProtocolVersion:    sdk.ProtocolVersionNumber,
		ClientInfo:         &sdk.Implementation{Name: "fiveleaf", Version: "1"},
		ClientCapabilities: capabilities,
	})
	if err != nil {
		return result, err
	}
	if result.ProtocolVersion != sdk.ProtocolVersionNumber {
		c.recordTransportError(fmt.Errorf("unsupported ACP protocol version %d", result.ProtocolVersion))
		c.shutdown(false)
		return result, c.Err()
	}
	c.mu.Lock()
	c.initialized, c.initResult = true, result
	c.mu.Unlock()
	return result, nil
}

func (c *Client) Authenticate(ctx context.Context, methodID string) error {
	if err := c.checkReady(); err != nil {
		return err
	}
	c.mu.Lock()
	methods := c.initResult.AuthMethods
	c.mu.Unlock()
	for _, method := range methods {
		if method.Agent != nil && string(method.Agent.Id) == methodID {
			_, err := request[sdk.AuthenticateResponse](c, ctx, "authenticate", sdk.AuthenticateRequest{MethodId: methodID})
			return err
		}
	}
	return errors.New("agent does not offer this protocol authentication method")
}

func (c *Client) NewSession(ctx context.Context) (Session, error) {
	return c.OpenSession(ctx, "", false)
}

func (c *Client) OpenSession(ctx context.Context, previousID string, needHistory bool) (Session, error) {
	c.setupMu.Lock()
	defer c.setupMu.Unlock()
	if err := c.checkReady(); err != nil {
		return Session{}, err
	}
	caps := c.initResult.AgentCapabilities
	method := "session/new"
	var params any = sdk.NewSessionRequest{Cwd: c.root, McpServers: []sdk.McpServer{}}
	if previousID != "" {
		switch {
		case caps.LoadSession && (needHistory || caps.SessionCapabilities.Resume == nil):
			method, params = "session/load", sdk.LoadSessionRequest{Cwd: c.root, SessionId: sdk.SessionId(previousID), McpServers: []sdk.McpServer{}}
		case caps.SessionCapabilities.Resume != nil:
			method, params = "session/resume", sdk.ResumeSessionRequest{Cwd: c.root, SessionId: sdk.SessionId(previousID), McpServers: []sdk.McpServer{}}
		case needHistory:
			return Session{}, errors.New("the agent cannot restore this session without a local transcript")
		}
	}
	c.updateMu.Lock()
	c.creating, c.early = true, nil
	if method == "session/load" {
		c.loading = previousID
		if needHistory {
			c.replay = &sessionReplay{}
		}
	}
	c.updateMu.Unlock()
	result, err := request[Session](c, ctx, method, params)
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	if c.replay != nil {
		result.Replay, result.PartialReplay = c.replay.messages, c.replay.partial
	}
	c.creating, c.loading, c.replay = false, "", nil
	defer func() { c.early = nil }()
	if err != nil {
		return result, err
	}
	if method != "session/new" {
		result.SessionId, result.Restored = sdk.SessionId(previousID), true
	}
	if result.SessionId == "" {
		return result, errors.New("agent returned an empty ACP session ID")
	}
	state := &sessionState{permissions: make(map[*permissionWait]struct{})}
	state.configOptions = result.ConfigOptions
	if result.Models != nil {
		state.legacyModels = make(map[string]bool)
		state.currentModel = result.Models.CurrentModelId
		for _, model := range result.Models.AvailableModels {
			state.legacyModels[model.ModelId] = true
			choice := ModelChoice{ID: model.ModelId, Name: model.Name}
			if model.Description != nil {
				choice.Description = *model.Description
			}
			state.legacyChoices = append(state.legacyChoices, choice)
		}
	}
	if result.Modes != nil {
		state.currentMode = string(result.Modes.CurrentModeId)
		for _, mode := range result.Modes.AvailableModes {
			choice := ModeChoice{ID: string(mode.Id), Name: mode.Name}
			if mode.Description != nil {
				choice.Description = *mode.Description
			}
			state.modes = append(state.modes, choice)
		}
	}
	c.mu.Lock()
	if _, exists := c.sessions[string(result.SessionId)]; exists {
		c.mu.Unlock()
		return result, errors.New("agent reused an ACP session ID")
	}
	c.sessions[string(result.SessionId)] = state
	c.mu.Unlock()
	for _, pending := range c.early {
		if pending.legacy != nil {
			if pending.legacy.SessionID == string(result.SessionId) {
				c.deliverLegacyModel(*pending.legacy)
			}
		} else if pending.update.SessionId == result.SessionId && c.cacheSessionUpdate(pending.update) {
			c.handler.Update(pending.update)
		}
	}
	return result, nil
}

func (c *Client) Prompt(ctx context.Context, sessionID string, blocks []ContentBlock) (PromptResult, error) {
	if err := c.checkReady(); err != nil {
		return PromptResult{}, err
	}
	if len(blocks) == 0 {
		return PromptResult{}, errors.New("ACP prompt is empty")
	}
	params := sdk.PromptRequest{SessionId: sdk.SessionId(sessionID), Prompt: blocks}
	encoded, err := json.Marshal(params)
	if err != nil {
		return PromptResult{}, err
	}
	if len(encoded) > MaxMessageBytes-1024 {
		return PromptResult{}, ErrMessageTooLarge
	}
	c.mu.Lock()
	caps := c.initResult.AgentCapabilities.PromptCapabilities
	for _, block := range blocks {
		if err := block.Validate(); err != nil {
			c.mu.Unlock()
			return PromptResult{}, err
		}
		if block.Resource != nil && !caps.EmbeddedContext || block.Image != nil && !caps.Image || block.Audio != nil && !caps.Audio {
			c.mu.Unlock()
			return PromptResult{}, errors.New("agent does not support this prompt content type")
		}
	}
	s, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return PromptResult{}, ErrUnknownSession
	}
	if s.active {
		c.mu.Unlock()
		return PromptResult{}, errors.New("ACP session is already responding")
	}
	if s.settingModel || s.settingMode {
		c.mu.Unlock()
		return PromptResult{}, errors.New("ACP session is still being configured")
	}
	if s.interrupted {
		c.mu.Unlock()
		return PromptResult{}, errors.New("ACP turn was interrupted without acknowledgement; start a new session")
	}
	s.active, s.cancelled = true, false
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		s.active = false
		c.mu.Unlock()
		c.cancelPermissions(sessionID)
	}()
	// Cancel UI permissions even when the peer does not acknowledge stopping.
	cancelSent := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(cancelSent)
		_ = c.Cancel(c.ctx, sessionID)
	})
	defer func() {
		if !stop() {
			<-cancelSent
		}
	}()
	result, err := request[PromptResult](c, ctx, "session/prompt", params)
	if ctx.Err() != nil && err != nil {
		// Without the prompt response the old turn may still be running. Do
		// not silently send a concurrent prompt into the same agent session.
		c.mu.Lock()
		s.interrupted = true
		c.mu.Unlock()
	}
	return result, err
}

func (c *Client) cancelPermissions(sessionID string) {
	c.mu.Lock()
	var pending []*permissionWait
	if s := c.sessions[sessionID]; s != nil {
		s.cancelled = true
		for wait := range s.permissions {
			pending = append(pending, wait)
		}
	}
	c.mu.Unlock()
	for _, wait := range pending {
		wait.cancel()
	}
}

func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	if err := c.checkSession(sessionID); err != nil {
		return err
	}
	c.cancelPermissions(sessionID)
	return c.conn.SendNotification(ctx, "session/cancel", sdk.CancelNotification{SessionId: sdk.SessionId(sessionID)})
}

func (c *Client) SetMode(ctx context.Context, sessionID, modeID string) error {
	return c.setMode(ctx, sessionID, modeID, false)
}

// SetThinkingMode disambiguates thinking values from generic modes with the
// same ID when an adapter exposes both selectors.
func (c *Client) SetThinkingMode(ctx context.Context, sessionID, modeID string) error {
	return c.setMode(ctx, sessionID, modeID, true)
}

func (c *Client) setMode(ctx context.Context, sessionID, modeID string, preferThinking bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.checkSession(sessionID); err != nil {
		return err
	}
	c.mu.Lock()
	s := c.sessions[sessionID]
	if s.active || s.settingModel || s.settingMode || s.interrupted {
		c.mu.Unlock()
		return errors.New("ACP session is not ready to select a mode")
	}
	state, _ := sessionModeState(s)
	option := thinkingConfigOption(s.configOptions)
	generic := slices.ContainsFunc(state.GenericAvailable, func(choice ModeChoice) bool { return choice.ID == modeID })
	thinking := option != nil && modelConfigHasValue(option, modeID) && (preferThinking || !generic)
	available := state.Available
	if option != nil && !thinking {
		available = state.GenericAvailable
	}
	if preferThinking && option != nil && !thinking {
		available = nil
	}
	if !slices.ContainsFunc(available, func(choice ModeChoice) bool { return choice.ID == modeID }) {
		c.mu.Unlock()
		return errors.New("agent did not advertise this session mode")
	}
	var configID sdk.SessionConfigId
	if thinking {
		configID = option.Id
		if configID == "" {
			c.mu.Unlock()
			return errors.New("agent advertised a thinking selector without an ID")
		}
	}
	s.settingMode = true
	modeRevision, usageRevision := s.modeRevision, s.usageRevision
	legacyModeRevision := s.legacyModeRevision
	previousModel, _ := sessionModelState(s)
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		s.settingMode = false
		c.mu.Unlock()
	}()
	if thinking {
		result, err := request[sdk.SetSessionConfigOptionResponse](c, ctx, "session/set_config_option", sdk.SetSessionConfigOptionRequest{
			ValueId: &sdk.SetSessionConfigOptionValueId{SessionId: sdk.SessionId(sessionID), ConfigId: configID, Value: sdk.SessionConfigValueId(modeID)},
		})
		if err != nil {
			return err
		}
		c.mu.Lock()
		if s.modeRevision == modeRevision {
			s.configOptions = result.ConfigOptions
		}
		currentModel, _ := sessionModelState(s)
		if previousModel.Current != currentModel.Current && s.usageRevision == usageRevision {
			s.usage = nil
		}
		c.mu.Unlock()
	} else {
		_, err := request[sdk.SetSessionModeResponse](c, ctx, "session/set_mode", sdk.SetSessionModeRequest{SessionId: sdk.SessionId(sessionID), ModeId: sdk.SessionModeId(modeID)})
		if err != nil {
			return err
		}
		c.mu.Lock()
		if s.legacyModeRevision == legacyModeRevision {
			s.currentMode = modeID
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	selected, known := sessionModeState(s)
	if !thinking {
		selected.Current = s.currentMode
		known = len(s.modes) > 0
	}
	c.mu.Unlock()
	if !known || selected.Current != modeID {
		return fmt.Errorf("agent did not confirm requested mode %q", modeID)
	}
	return nil
}

// SetModel only uses selectors advertised by this session. An explicit model
// must never silently run with the agent's default when selection is unavailable.
func (c *Client) SetModel(ctx context.Context, sessionID, modelID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.checkSession(sessionID); err != nil {
		return err
	}
	if modelID == "" {
		return nil
	}
	c.mu.Lock()
	s := c.sessions[sessionID]
	if s.active || s.settingModel || s.settingMode || s.interrupted {
		c.mu.Unlock()
		return errors.New("ACP session is not ready to select a model")
	}
	option := modelConfigOption(s.configOptions)
	legacy := option == nil && s.legacyModels != nil
	if option == nil && !legacy {
		c.mu.Unlock()
		return errors.New("agent did not advertise model selection for this session")
	}
	if option != nil && !modelConfigHasValue(option, modelID) || legacy && !s.legacyModels[modelID] {
		c.mu.Unlock()
		return fmt.Errorf("agent did not advertise model %q for this session", modelID)
	}
	var configID sdk.SessionConfigId
	if option != nil {
		configID = option.Id
		if configID == "" {
			c.mu.Unlock()
			return errors.New("agent advertised a model selector without an ID")
		}
	}
	s.settingModel = true
	previous, _ := sessionModelState(s)
	modelRevision, usageRevision := s.modelRevision, s.usageRevision
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		s.settingModel = false
		c.mu.Unlock()
	}()
	if legacy {
		_, err := request[struct{}](c, ctx, "session/set_model", struct {
			SessionID string `json:"sessionId"`
			ModelID   string `json:"modelId"`
		}{sessionID, modelID})
		if err == nil {
			c.mu.Lock()
			if s.modelRevision == modelRevision {
				s.currentModel = modelID
			}
			if previous.Current != s.currentModel && s.usageRevision == usageRevision {
				s.usage = nil
			}
			selected := s.currentModel
			c.mu.Unlock()
			if selected != modelID {
				return fmt.Errorf("agent did not confirm requested model %q", modelID)
			}
		}
		return err
	}
	result, err := request[sdk.SetSessionConfigOptionResponse](c, ctx, "session/set_config_option", sdk.SetSessionConfigOptionRequest{
		ValueId: &sdk.SetSessionConfigOptionValueId{SessionId: sdk.SessionId(sessionID), ConfigId: configID, Value: sdk.SessionConfigValueId(modelID)},
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	// A notification that changes the current model takes precedence. Refreshes
	// of choices or other options do not replace the setter's confirmation.
	if s.modelRevision == modelRevision {
		s.configOptions = result.ConfigOptions
	}
	selected, known := sessionModelState(s)
	if previous.Current != selected.Current && s.usageRevision == usageRevision {
		s.usage = nil
	}
	c.mu.Unlock()
	if !known || selected.Current != modelID {
		return fmt.Errorf("agent did not confirm requested model %q", modelID)
	}
	return nil
}

func modelConfigOption(options []sdk.SessionConfigOption) *sdk.SessionConfigOptionSelect {
	for _, option := range options {
		if s := option.Select; s != nil && s.Category != nil && *s.Category == sdk.SessionConfigOptionCategoryModel {
			return s
		}
	}
	for _, option := range options {
		if s := option.Select; s != nil && s.Id == "model" {
			return s
		}
	}
	return nil
}

func thinkingConfigOption(options []sdk.SessionConfigOption) *sdk.SessionConfigOptionSelect {
	for _, option := range options {
		if s := option.Select; s != nil && s.Category != nil && *s.Category == sdk.SessionConfigOptionCategoryThoughtLevel {
			return s
		}
	}
	for _, option := range options {
		if s := option.Select; s != nil && s.Id == "thought_level" {
			return s
		}
	}
	return nil
}

func modelConfigHasValue(option *sdk.SessionConfigOptionSelect, value string) bool {
	if option.Options.Ungrouped != nil {
		for _, item := range *option.Options.Ungrouped {
			if string(item.Value) == value {
				return true
			}
		}
	}
	if option.Options.Grouped != nil {
		for _, group := range *option.Options.Grouped {
			for _, item := range group.Options {
				if string(item.Value) == value {
					return true
				}
			}
		}
	}
	return false
}

func (c *Client) checkSession(sessionID string) error {
	if err := c.checkReady(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.sessions[sessionID]; !ok {
		return ErrUnknownSession
	}
	return nil
}

type messageWriter struct{ client *Client }

func (w messageWriter) Write(p []byte) (int, error) {
	if len(p) > MaxMessageBytes {
		return 0, ErrMessageTooLarge
	}
	if w.client.ctx.Err() != nil {
		return 0, ErrClosed
	}
	deadline := time.AfterFunc(writeTimeout, func() {
		w.client.recordTransportError(errWriteTimeout)
		w.client.shutdown(false)
	})
	written, err := w.client.stdin.Write(p)
	if !deadline.Stop() {
		return written, errWriteTimeout
	}
	if err != nil && w.client.ctx.Err() == nil {
		w.client.recordTransportError(fmt.Errorf("write ACP message: %w", err))
		w.client.shutdown(false)
	}
	return written, err
}

type messageReader struct {
	reader io.Reader
	ready  <-chan struct{}
	bytes  int
	failed func(error)
}

func (r *messageReader) Read(p []byte) (int, error) {
	<-r.ready
	n, err := r.reader.Read(p)
	for i, b := range p[:n] {
		if b == '\n' {
			r.bytes = 0
		} else {
			r.bytes++
			if r.bytes > MaxMessageBytes {
				r.failed(ErrMessageTooLarge)
				return i, ErrMessageTooLarge
			}
		}
	}
	return n, err
}

type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(p)
	if len(p) >= MaxStderrBytes {
		t.b = append(t.b[:0], p[len(p)-MaxStderrBytes:]...)
	} else {
		if extra := len(t.b) + len(p) - MaxStderrBytes; extra > 0 {
			copy(t.b, t.b[extra:])
			t.b = t.b[:len(t.b)-extra]
		}
		t.b = append(t.b, p...)
	}
	return n, nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.b
	for len(b) > 0 && !utf8.RuneStart(b[0]) {
		b = b[1:]
	}
	return strings.ToValidUTF8(string(b), "�")
}

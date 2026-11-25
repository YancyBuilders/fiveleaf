package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"
)

func (c *Client) handle(ctx context.Context, method string, raw json.RawMessage) (any, *sdk.RequestError) {
	var result any
	var err error
	switch method {
	case "session/update":
		var legacy struct {
			SessionID string `json:"sessionId"`
			Update    struct {
				Type           string `json:"sessionUpdate"`
				CurrentModelID string `json:"currentModelId"`
			} `json:"update"`
		}
		if json.Unmarshal(raw, &legacy) == nil && legacy.Update.Type == "current_model_update" {
			err = c.updateLegacyModel(legacyModelUpdate{legacy.SessionID, legacy.Update.CurrentModelID})
			break
		}
		var p Update
		if err = json.Unmarshal(raw, &p); err == nil {
			err = c.update(p)
		}
	case "session/request_permission":
		var p PermissionRequest
		if err = json.Unmarshal(raw, &p); err == nil {
			result, err = c.permission(ctx, p)
		}
	case "fs/read_text_file":
		var p ReadRequest
		if err = json.Unmarshal(raw, &p); err == nil {
			result, err = c.read(ctx, p)
		}
	case "fs/write_text_file":
		// The SDK's generated validator rejects an empty string, although ACP
		// permits empty files. Check field presence instead of string length.
		var fields struct {
			Content *string `json:"content"`
		}
		var p WriteRequest
		if err = json.Unmarshal(raw, &p); err == nil {
			err = json.Unmarshal(raw, &fields)
			if err == nil && fields.Content == nil {
				err = errors.New("content is required")
			}
			if err == nil {
				result, err = c.write(ctx, p)
			}
		}
	default:
		return nil, sdk.NewMethodNotFound(method)
	}
	if err == nil {
		return result, nil
	}
	var requestError *sdk.RequestError
	if errors.As(err, &requestError) {
		return nil, requestError
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, sdk.NewRequestCancelled(nil)
	}
	return nil, sdk.NewInvalidParams(map[string]any{"error": err.Error()})
}

func (c *Client) update(p Update) error {
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	if c.ctx.Err() != nil {
		return nil
	}
	c.mu.Lock()
	_, known := c.sessions[string(p.SessionId)]
	c.mu.Unlock()
	if !known {
		if c.loading != "" && c.loading == string(p.SessionId) && p.Update.ConfigOptionUpdate == nil && p.Update.CurrentModeUpdate == nil && p.Update.UsageUpdate == nil && p.Update.AvailableCommandsUpdate == nil {
			if c.replay != nil {
				c.replay.record(p.Update)
			}
			return nil
		}
		if c.creating && len(c.early) < 64 {
			c.early = append(c.early, earlySessionUpdate{update: p})
			return nil
		}
		return ErrUnknownSession
	}
	if c.cacheSessionUpdate(p) {
		c.handler.Update(p)
	}
	return nil
}

func (c *Client) cacheSessionUpdate(p Update) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.sessions[string(p.SessionId)]; s != nil {
		if list := p.Update.AvailableCommandsUpdate; list != nil {
			s.commands = sessionCommands(list.AvailableCommands)
		}
		if config := p.Update.ConfigOptionUpdate; config != nil {
			previous, _ := sessionModelState(s)
			previousMode, _ := sessionModeState(s)
			s.configOptions = config.ConfigOptions
			current, _ := sessionModelState(s)
			currentMode, _ := sessionModeState(s)
			if previous.Current != current.Current {
				s.modelRevision++
				s.usage = nil
			}
			if previousMode.Current != currentMode.Current {
				s.modeRevision++
			}
		}
		if mode := p.Update.CurrentModeUpdate; mode != nil {
			id := string(mode.CurrentModeId)
			state, _ := sessionModeState(s)
			generic := slices.ContainsFunc(state.GenericAvailable, func(choice ModeChoice) bool { return choice.ID == id })
			if option := thinkingConfigOption(s.configOptions); option != nil && modelConfigHasValue(option, id) && !generic {
				option.CurrentValue = sdk.SessionConfigValueId(id)
				s.modeRevision++
			}
			if generic || len(state.GenericAvailable) == 0 {
				s.currentMode = id
				s.legacyModeRevision++
			}
		}
		if usage := p.Update.UsageUpdate; usage != nil {
			if usage.Used < 0 || usage.Size <= 0 {
				return false
			}
			s.usage = &UsageState{Used: usage.Used, Size: usage.Size}
			s.usageRevision++
		}
	}
	return true
}

type legacyModelUpdate struct{ SessionID, CurrentModelID string }

type earlySessionUpdate struct {
	update Update
	legacy *legacyModelUpdate
}

func (c *Client) updateLegacyModel(update legacyModelUpdate) error {
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	if c.ctx.Err() != nil {
		return nil
	}
	c.mu.Lock()
	_, known := c.sessions[update.SessionID]
	c.mu.Unlock()
	if known {
		c.deliverLegacyModel(update)
		return nil
	}
	if c.creating && len(c.early) < 64 {
		c.early = append(c.early, earlySessionUpdate{legacy: &update})
		return nil
	}
	return ErrUnknownSession
}

func (c *Client) deliverLegacyModel(update legacyModelUpdate) {
	c.mu.Lock()
	s := c.sessions[update.SessionID]
	if s.currentModel != update.CurrentModelID {
		s.modelRevision++
		s.usage = nil
	}
	s.currentModel = update.CurrentModelID
	notification := legacyModelNotification(update.SessionID, s)
	c.mu.Unlock()
	// Notify the UI without caching this synthesized selector: the peer still
	// needs session/set_model rather than session/set_config_option.
	c.handler.Update(notification)
}

func (c *Client) permission(ctx context.Context, p PermissionRequest) (sdk.RequestPermissionResponse, error) {
	if err := c.checkSession(string(p.SessionId)); err != nil {
		return sdk.RequestPermissionResponse{}, err
	}
	if len(p.Options) == 0 {
		return sdk.RequestPermissionResponse{}, errors.New("permission request has no options")
	}
	permissionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	wait := &permissionWait{cancel: cancel}
	c.mu.Lock()
	s := c.sessions[string(p.SessionId)]
	s.permissions[wait] = struct{}{}
	cancelled := s.cancelled
	c.mu.Unlock()
	if cancelled {
		cancel()
	}
	defer func() {
		c.mu.Lock()
		delete(s.permissions, wait)
		c.mu.Unlock()
	}()
	if permissionCtx.Err() != nil {
		return sdk.RequestPermissionResponse{Outcome: CancelledPermission()}, nil
	}
	outcome, err := c.handler.RequestPermission(permissionCtx, p)
	if permissionCtx.Err() != nil || errors.Is(err, context.Canceled) {
		return sdk.RequestPermissionResponse{Outcome: CancelledPermission()}, nil
	}
	if err != nil {
		return sdk.RequestPermissionResponse{}, err
	}
	if outcome.Cancelled != nil && outcome.Selected == nil {
		return sdk.RequestPermissionResponse{Outcome: CancelledPermission()}, nil
	}
	if outcome.Selected != nil && outcome.Cancelled == nil {
		for _, option := range p.Options {
			if option.OptionId == outcome.Selected.OptionId {
				return sdk.RequestPermissionResponse{Outcome: SelectedPermission(string(option.OptionId))}, nil
			}
		}
	}
	return sdk.RequestPermissionResponse{}, errors.New("permission result does not select an offered option")
}

func (c *Client) read(ctx context.Context, p ReadRequest) (sdk.ReadTextFileResponse, error) {
	if err := c.checkSession(string(p.SessionId)); err != nil {
		return sdk.ReadTextFileResponse{}, err
	}
	if p.Line != nil && *p.Line < 1 || p.Limit != nil && *p.Limit < 0 {
		return sdk.ReadTextFileResponse{}, errors.New("invalid text line range")
	}
	path, err := c.ResolvePath(p.Path)
	if err != nil {
		return sdk.ReadTextFileResponse{}, err
	}
	line, limit := p.Line, p.Limit
	p.Path, p.Line, p.Limit = path, nil, nil
	ctx, cancel := c.callbackContext(ctx)
	defer cancel()
	text, err := c.handler.ReadTextFile(ctx, p)
	if err != nil {
		return sdk.ReadTextFileResponse{}, err
	}
	if len(text) > MaxTextBytes {
		return sdk.ReadTextFileResponse{}, ErrTextTooLarge
	}
	encoded, _ := json.Marshal(text)
	if len(encoded) > MaxMessageBytes-1024 {
		return sdk.ReadTextFileResponse{}, ErrMessageTooLarge
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return sdk.ReadTextFileResponse{}, errors.New("file is not UTF-8 text")
	}
	if line != nil || limit != nil {
		text = textLines(text, line, limit)
	}
	return sdk.ReadTextFileResponse{Content: text}, nil
}

func (c *Client) write(ctx context.Context, p WriteRequest) (sdk.WriteTextFileResponse, error) {
	if err := c.checkSession(string(p.SessionId)); err != nil {
		return sdk.WriteTextFileResponse{}, err
	}
	if len(p.Content) > MaxTextBytes {
		return sdk.WriteTextFileResponse{}, ErrTextTooLarge
	}
	if !utf8.ValidString(p.Content) || strings.ContainsRune(p.Content, '\x00') {
		return sdk.WriteTextFileResponse{}, errors.New("content is not UTF-8 text")
	}
	path, err := c.ResolvePath(p.Path)
	if err != nil {
		return sdk.WriteTextFileResponse{}, err
	}
	p.Path = path
	ctx, cancel := c.callbackContext(ctx)
	defer cancel()
	return sdk.WriteTextFileResponse{}, c.handler.WriteTextFile(ctx, p)
}

func (c *Client) callbackContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(c.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}

func textLines(text string, line, limit *int) string {
	start := 1
	if line != nil {
		start = *line
	}
	for n := 1; n < start; n++ {
		_, rest, found := strings.Cut(text, "\n")
		if !found {
			return ""
		}
		text = rest
	}
	if limit == nil {
		return text
	}
	if *limit == 0 {
		return ""
	}
	end := 0
	for n := 0; n < *limit; n++ {
		i := strings.IndexByte(text[end:], '\n')
		if i == -1 {
			return text
		}
		end += i + 1
	}
	return text[:end]
}

// ResolvePath accepts paths within this workspace, resolves existing symlinks,
// and also handles new files by resolving their closest existing parent.
// Call again immediately before using a path if it waited on a UI interaction.
func (c *Client) ResolvePath(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return "", errors.New("ACP file path must be absolute")
	}
	path = filepath.Clean(path)
	probe := path
	var suffix []string
	for {
		_, err := os.Lstat(probe)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(probe)
			if err != nil {
				return "", fmt.Errorf("resolve ACP path: %w", err)
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			rel, err := filepath.Rel(c.root, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return "", errors.New("ACP file path is outside the workspace")
			}
			if rel == "." {
				return "", errors.New("ACP file path names the workspace directory")
			}
			// Git metadata is not editor document content and may configure hooks.
			for _, part := range strings.Split(rel, string(filepath.Separator)) {
				if strings.EqualFold(part, ".git") {
					return "", errors.New("ACP file access to .git is not supported")
				}
			}
			if info, err := os.Stat(resolved); err == nil && !info.Mode().IsRegular() {
				return "", errors.New("ACP path is not a regular file")
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", errors.New("cannot resolve ACP file path")
		}
		suffix = append(suffix, filepath.Base(probe))
		probe = parent
	}
}

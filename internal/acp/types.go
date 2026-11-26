// Package acp connects the editor to ACP v1 agents over stdio.
// It advertises editor file access, but not terminal execution or terminal login.
package acp

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"
)

// Config launches an ACP adapter directly, without interpreting a shell command.
// Environment values override inherited variables and are never logged here.
type Config struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
	// WorkingDirectory changes only the process directory; session file access
	// remains rooted in the IDE workspace passed to Start.
	WorkingDirectory string
}

type (
	InitializeResult   = sdk.InitializeResponse
	ContentBlock       = sdk.ContentBlock
	Update             = sdk.SessionNotification
	PermissionRequest  = sdk.RequestPermissionRequest
	PermissionOutcome  = sdk.RequestPermissionOutcome
	PermissionOption   = sdk.PermissionOption
	ReadRequest        = sdk.ReadTextFileRequest
	WriteRequest       = sdk.WriteTextFileRequest
	AuthMethod         = sdk.AuthMethod
	SessionID          = sdk.SessionId
	ModeID             = sdk.SessionModeId
	PermissionOptionID = sdk.PermissionOptionId
)

// PromptResult keeps the protocol fields FiveLeaf uses plus the request's
// token split from adapter metadata.
type PromptResult struct {
	Meta          map[string]any `json:"_meta,omitempty"`
	StopReason    sdk.StopReason `json:"stopReason"`
	Usage         *sdk.Usage     `json:"usage,omitempty"`
	UserMessageID *string        `json:"userMessageId,omitempty"`
	tokens        promptUsage
}

// Session keeps the stable ACP session fields and the legacy model selector
// that some adapters still advertise instead of configOptions.
type Session struct {
	Restored      bool                      `json:"-"`
	Replay        []ReplayMessage           `json:"-"`
	PartialReplay bool                      `json:"-"`
	Meta          map[string]any            `json:"_meta,omitempty"`
	ConfigOptions []sdk.SessionConfigOption `json:"configOptions,omitempty"`
	Modes         *sdk.SessionModeState     `json:"modes,omitempty"`
	SessionId     sdk.SessionId             `json:"sessionId"`
	Models        *SessionModelState        `json:"models,omitempty"`
}

type ReplayMessage struct {
	Role, Text      string
	FirstChunkBytes int
}

type sessionReplay struct {
	messages          []ReplayMessage
	public, thought   replayStream
	partial           bool
	codexThoughtParts bool
}

type replayStream struct {
	current   strings.Builder
	messageID string
	last      int
	rows      int
	bytes     int
	full      bool
}

func (replay *sessionReplay) record(update sdk.SessionUpdate) {
	role := "user"
	var block ContentBlock
	var messageID *string
	switch {
	case update.UserMessageChunk != nil:
		block = update.UserMessageChunk.Content
		messageID = update.UserMessageChunk.MessageId
	case update.AgentMessageChunk != nil:
		block, role = update.AgentMessageChunk.Content, "assistant"
		messageID = update.AgentMessageChunk.MessageId
	case update.AgentThoughtChunk != nil:
		block, role = update.AgentThoughtChunk.Content, "thought"
		messageID = update.AgentThoughtChunk.MessageId
	default:
		return
	}
	if block.Text == nil || block.Text.Text == "" || replay.public.full {
		return
	}
	stream, maxMessages, maxBytes := &replay.public, 96, 96<<10
	if role == "thought" {
		stream, maxMessages, maxBytes = &replay.thought, 24, 24<<10
	}
	id := ""
	if messageID != nil {
		id = *messageID
	}
	text := block.Text.Text
	last := stream.last
	newMessage := last == 0 || replay.messages[last-1].Role != role || id != "" && stream.messageID != "" && id != stream.messageID
	if newMessage && role == "thought" && strings.TrimSpace(text) == "" {
		return
	}
	if stream.full || stream.bytes == maxBytes || newMessage && stream.rows == maxMessages {
		stream.full, replay.partial = true, true
		return
	}
	room := maxBytes - stream.bytes
	separator := ""
	if role == "thought" && replay.codexThoughtParts && !newMessage && strings.TrimSpace(text) != "" {
		separator = "\n\n"
		room -= len(separator)
	}
	if room <= 0 {
		stream.full, replay.partial = true, true
		return
	}
	if len(text) > room {
		for room > 0 && !utf8.RuneStart(text[room]) {
			room--
		}
		text, stream.full, replay.partial = text[:room], true, true
		if text == "" {
			return
		}
	}
	if role != "thought" {
		replay.thought.last = 0
	}
	if newMessage {
		stream.current.Reset()
		stream.messageID = ""
		replay.messages = append(replay.messages, ReplayMessage{Role: role})
		stream.last = len(replay.messages)
		stream.rows++
	}
	if id != "" {
		stream.messageID = id
	}
	stream.bytes += len(separator) + len(text)
	message := &replay.messages[stream.last-1]
	if newMessage {
		message.FirstChunkBytes = len(text)
	}
	stream.current.WriteString(separator)
	stream.current.WriteString(text)
	message.Text = stream.current.String()
}

type SessionModelState struct {
	CurrentModelId  string             `json:"currentModelId"`
	AvailableModels []SessionModelInfo `json:"availableModels"`
}

type SessionModelInfo struct {
	ModelId     string  `json:"modelId"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type ModelChoice struct {
	ID, Name, Description, Group string
}

type ModelState struct {
	Current   string
	Available []ModelChoice
}

type ModeChoice struct {
	ID, Name, Description string
}

type ModeState struct {
	Current          string
	Available        []ModeChoice
	Thinking         bool
	GenericCurrent   string
	GenericAvailable []ModeChoice
}

type UsageState struct {
	Used, Size int
}

// PromptUsage is the input split of the most recent request.
type PromptUsage struct {
	Input, CachedInput int
}

// promptUsage is the most recent request's token split. Adapters that report a
// context window separately can use it to derive occupancy without counting
// generated output as context.
type promptUsage struct {
	Input, CachedInput int
}

func capturePromptUsage(sessionID string, result PromptResult) PromptResult {
	result.tokens = promptUsage{}
	if usage := result.Usage; usage != nil {
		result.tokens.Input, result.tokens.CachedInput = usage.InputTokens, intValue(usage.CachedReadTokens)+intValue(usage.CachedWriteTokens)
		return result
	}
	meta, ok := result.Meta["quota"].(map[string]any)
	if !ok {
		return result
	}
	raw, ok := meta["token_count"].(map[string]any)
	if !ok {
		return result
	}
	result.tokens.Input = jsonInt(raw["inputTokens"])
	result.tokens.CachedInput = jsonInt(raw["cachedInputTokens"]) + jsonInt(raw["cacheWriteInputTokens"])
	return result
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func jsonInt(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

// Handler bridges to editor state. Calls arrive on background goroutines.
// Update must return promptly and is delivered in wire order. Other callbacks
// may wait for the UI but must return when their context is cancelled.
// ReadTextFile returns the complete buffer, including unsaved changes; the
// client applies the requested line range. Paths have been checked and resolved
// inside the workspace. WriteTextFile must protect newer, unsaved editor edits.
type Handler interface {
	Update(Update)
	RequestPermission(context.Context, PermissionRequest) (PermissionOutcome, error)
	ReadTextFile(context.Context, ReadRequest) (string, error)
	WriteTextFile(context.Context, WriteRequest) error
}

func TextBlock(text string) ContentBlock { return sdk.TextBlock(text) }

// ResourceBlock embeds the current editor contents. Use only when the agent
// advertises promptCapabilities.embeddedContext; otherwise send a TextBlock.
func ResourceBlock(path, text string) ContentBlock {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return sdk.ResourceBlock(sdk.EmbeddedResourceResource{TextResourceContents: &sdk.TextResourceContents{
		Uri: u.String(), MimeType: sdk.Ptr("text/plain"), Text: text,
	}})
}

func SelectedPermission(id string) PermissionOutcome {
	return PermissionOutcome{Selected: &sdk.RequestPermissionOutcomeSelected{
		Outcome: "selected", OptionId: sdk.PermissionOptionId(id),
	}}
}

func CancelledPermission() PermissionOutcome {
	return PermissionOutcome{Cancelled: &sdk.RequestPermissionOutcomeCancelled{Outcome: "cancelled"}}
}

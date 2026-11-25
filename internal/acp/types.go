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
	PromptResult       = sdk.PromptResponse
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

type ReplayMessage struct{ Role, Text string }

type sessionReplay struct {
	messages []ReplayMessage
	current  strings.Builder
	bytes    int
	partial  bool
}

func (replay *sessionReplay) record(update sdk.SessionUpdate) {
	role := "user"
	var block ContentBlock
	switch {
	case update.UserMessageChunk != nil:
		block = update.UserMessageChunk.Content
	case update.AgentMessageChunk != nil:
		block, role = update.AgentMessageChunk.Content, "assistant"
	default:
		return
	}
	if block.Text == nil || block.Text.Text == "" {
		return
	}
	const maxMessages, maxBytes = 96, 96 << 10
	if replay.partial || replay.bytes == maxBytes {
		replay.partial = true
		return
	}
	if len(replay.messages) == 0 || replay.messages[len(replay.messages)-1].Role != role {
		if len(replay.messages) == maxMessages {
			replay.partial = true
			return
		}
		replay.current.Reset()
		replay.messages = append(replay.messages, ReplayMessage{Role: role})
	}
	text := block.Text.Text
	if room := maxBytes - replay.bytes; len(text) > room {
		for room > 0 && !utf8.RuneStart(text[room]) {
			room--
		}
		text, replay.partial = text[:room], true
	}
	replay.bytes += len(text)
	replay.current.WriteString(text)
	replay.messages[len(replay.messages)-1].Text = replay.current.String()
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

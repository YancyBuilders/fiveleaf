package main

import (
	"cmp"
	"encoding/json"
	"strings"

	sdk "github.com/coder/acp-go-sdk"
)

func (h *agentHandler) addAgentTool(session string, call *sdk.SessionUpdateToolCall) {
	patch := &sdk.SessionToolCallUpdate{
		ToolCallId: call.ToolCallId, Title: &call.Title, Content: call.Content, Locations: call.Locations,
		RawInput: call.RawInput, RawOutput: call.RawOutput, Meta: call.Meta,
	}
	if call.Kind != "" {
		patch.Kind = &call.Kind
	}
	if call.Status != "" {
		patch.Status = &call.Status
	}
	h.patchAgentTool(session, patch)
}

func (h *agentHandler) patchAgentTool(session string, patch *sdk.SessionToolCallUpdate) {
	id := string(patch.ToolCallId)
	if id == "" {
		return
	}
	var input json.RawMessage
	if patch.RawInput != nil {
		input, _ = json.Marshal(patch.RawInput)
	}
	output := formatAgentToolOutput(patch.RawOutput)
	text, changes := agentToolContent(patch.Content)
	sources := chatToolSourcePatch(patch.Locations, patch.Content)
	terminal := agentToolOutputDelta(patch.Meta, "terminal_output_delta")
	progress := agentToolOutputDelta(patch.Meta, "mcp_output_delta")
	if patch.RawOutput == nil {
		if exit, ok := patch.Meta["terminal_exit"].(map[string]any); ok && exit["exit_code"] != nil {
			output = formatAgentToolOutput(map[string]any{"exit_code": exit["exit_code"]})
		}
	}
	h.post(func() {
		a, chat := h.workspace(), h.state()
		if chat.sessionID != "" && chat.sessionID != session {
			return
		}
		var tool *agentTool
		for _, previous := range chat.tools {
			if previous != nil && previous.id == id && previous.sessionID == session && previous.generation == h.generation {
				tool = previous
				break
			}
		}
		if tool == nil {
			tool = &agentTool{id: id, sessionID: session, generation: h.generation, turn: chat.turn, status: "pending"}
			chat.tools = append(chat.tools, tool)
			chat.messages = append(chat.messages, &agentMessage{role: "tool", tool: tool})
		}
		if patch.Title != nil {
			tool.title = *patch.Title
		}
		if patch.Kind != nil {
			tool.kind = string(*patch.Kind)
		}
		if patch.Status != nil {
			tool.status = string(*patch.Status)
		}
		if input != nil {
			tool.rawInput = input
		}
		if patch.RawOutput != nil || output != "" {
			tool.rawOutput = output
		}
		if patch.Locations != nil {
			tool.locations = make([]agentLocation, len(patch.Locations))
			for index, location := range patch.Locations {
				tool.locations[index] = agentLocation{Path: location.Path, Line: location.Line}
			}
		}
		if patch.Content != nil {
			tool.text, tool.changes = text, changes
		}
		if progress != "" && tool.progressOutput.Len() != 0 {
			tool.progressOutput.append("\n")
		}
		tool.progressOutput.append(progress)
		tool.terminalOutput.append(terminal)
		if session != "" && session == chat.sessionID {
			chat.sources.updateTool(tool, sources)
			if !h.meeting && chat.busy {
				chat.replyBoundary = len(chat.messages)
			}
		}
		for _, permission := range chat.permissions {
			fillChatPermission(chat, permission)
		}
		if h.toolChanged != nil {
			h.toolChanged(tool)
		}
		a.gitChanged()
	})
}

func agentToolOutputDelta(meta map[string]any, key string) string {
	fields, _ := meta[key].(map[string]any)
	text, _ := fields["data"].(string)
	return text
}

type agentToolOutputBuffer struct {
	strings.Builder
	clipped bool
}

func (output *agentToolOutputBuffer) append(text string) {
	if output.clipped || text == "" {
		return
	}
	prefix, clipped := agentVisibleText(text, agentTextLimit-output.Len())
	output.WriteString(prefix)
	output.clipped = clipped
}

func agentToolContent(contents []sdk.ToolCallContent) (string, []agentChange) {
	var text agentToolOutputBuffer
	var changes []agentChange
	for _, content := range contents {
		if diff := content.Diff; diff != nil {
			old := ""
			if diff.OldText != nil {
				old = *diff.OldText
			}
			if len(old) <= agentTextLimit && len(diff.NewText) <= agentTextLimit {
				changes = append(changes, agentChange{path: diff.Path, oldText: old, newText: diff.NewText, existed: diff.OldText != nil})
			}
		}
		if content.Content == nil {
			continue
		}
		block := content.Content.Content
		value := ""
		switch {
		case block.Text != nil:
			value = block.Text.Text
		case block.ResourceLink != nil:
			link := block.ResourceLink
			value = agentToolResourceLabel(link.Uri, link.Name)
			if value == "" && link.MimeType != nil {
				value = *link.MimeType
			}
		case block.Resource != nil && block.Resource.Resource.TextResourceContents != nil:
			value = block.Resource.Resource.TextResourceContents.Text
		}
		if value != "" {
			text.append(value)
			text.append("\n")
		}
	}
	return text.String(), changes
}

func agentToolDataURI(value string) bool {
	return len(value) >= len("data:") && strings.EqualFold(value[:len("data:")], "data:")
}

func agentToolResourceLabel(uri, name string) string {
	if agentToolDataURI(uri) {
		uri = ""
	}
	if agentToolDataURI(name) {
		name = ""
	}
	return cmp.Or(uri, name)
}

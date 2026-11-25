package roundtable

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	MaxPromptBytes = 4 << 20
	MaxOutputBytes = MaxMessageBytes
	MaxStderrBytes = 64 << 10
)

func boundedText(text string, limit int) string {
	if len(text) > limit {
		text = text[:limit]
	}
	for !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}
	return text
}

// FormatPrompt shares the same bounded text format with CLI and ACP adapters.
// System instructions remain separate; the most recent conversation is retained.
func FormatPrompt(messages []Message) string {
	format := func(m Message) string {
		role := m.Role
		if role != "system" && role != "user" && role != "assistant" {
			role = "message"
		}
		header := "[" + role
		if m.AgentID != "" || m.Name != "" {
			header += fmt.Sprintf(" name=%q id=%q hop=%d", boundedText(m.Name, 512), boundedText(m.AgentID, 512), m.Hop)
		}
		return header + "]\n" + boundedText(m.Text, MaxMessageBytes) + "\n\n"
	}
	var system strings.Builder
	for _, m := range messages {
		if m.Role == "system" && system.Len() < MaxSystemPromptBytes {
			system.WriteString(boundedText(format(m), MaxSystemPromptBytes-system.Len()))
		}
	}
	remaining := MaxPromptBytes - system.Len()
	var blocks []string
	for i := len(messages) - 1; i >= 0 && len(blocks) < MaxContextMessages && remaining > 0; i-- {
		if messages[i].Role == "system" {
			continue
		}
		block := boundedText(format(messages[i]), remaining)
		blocks = append(blocks, block)
		remaining -= len(block)
	}
	slices.Reverse(blocks)
	return system.String() + strings.Join(blocks, "")
}

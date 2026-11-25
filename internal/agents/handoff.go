package agents

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	handoffReadLimit    = 2 << 20
	handoffTextLimit    = 96 << 10
	handoffMessageText  = 24 << 10
	handoffMessageLimit = 96
)

type HandoffMessage struct {
	Role, Text string
}

type Handoff struct {
	Messages  []HandoffMessage
	Truncated bool
}

type handoffEnvelope struct {
	Type, Role, Channel, Text string
	ID, ParentID              string
	Content, Message, Payload json.RawMessage
	Messages                  []json.RawMessage
	IsMeta, IsSidechain       bool
}

type handoffEntry struct {
	HandoffMessage
	origin  byte
	digest  [32]byte
	id      string
	dropped bool
}

type handoffReader struct {
	entries   []handoffEntry
	truncated bool
	pi        bool
	parents   map[string]string
	leaf      string
}

func ReadHandoff(session Session) (Handoff, error) {
	if session.Transcript == "" {
		return Handoff{}, errors.New("this session has no transcript")
	}
	info, err := os.Stat(session.Transcript)
	if err != nil {
		return Handoff{}, fmt.Errorf("read session transcript: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Handoff{}, errors.New("the session transcript is not a regular file")
	}
	file, err := os.Open(session.Transcript)
	if err != nil {
		return Handoff{}, fmt.Errorf("open session transcript: %w", err)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return Handoff{}, fmt.Errorf("read session transcript: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Handoff{}, errors.New("the session transcript is not a regular file")
	}
	reader := handoffReader{pi: string(session.Harness) == "pi", parents: map[string]string{}}
	if info.Size() <= handoffReadLimit {
		data, readErr := io.ReadAll(io.LimitReader(file, handoffReadLimit+1))
		if readErr != nil {
			return Handoff{}, fmt.Errorf("read session transcript: %w", readErr)
		}
		if len(data) > handoffReadLimit {
			data = data[:handoffReadLimit]
			reader.truncated = true
			data = handoffCompleteHead(data)
		}
		reader.read(data)
	} else {
		reader.truncated = true
		head := make([]byte, handoffReadLimit/2-1)
		count, readErr := file.ReadAt(head, 0)
		if readErr != nil && readErr != io.EOF {
			return Handoff{}, fmt.Errorf("read session transcript: %w", readErr)
		}
		reader.read(handoffCompleteHead(head[:count]))
		tail := make([]byte, handoffReadLimit/2)
		tailOffset := info.Size() - int64(len(tail))
		count, readErr = file.ReadAt(tail, tailOffset)
		if readErr != nil && readErr != io.EOF {
			return Handoff{}, fmt.Errorf("read session transcript: %w", readErr)
		}
		tail = tail[:count]
		var boundary [1]byte
		_, readErr = file.ReadAt(boundary[:], tailOffset-1)
		if readErr != nil && readErr != io.EOF {
			return Handoff{}, fmt.Errorf("read session transcript: %w", readErr)
		}
		if boundary[0] == '\n' {
			reader.read(tail)
		} else if newline := bytes.IndexByte(tail, '\n'); newline >= 0 {
			reader.read(tail[newline+1:])
		}
	}
	result := reader.finish()
	if len(result.Messages) == 0 {
		return result, errors.New("the transcript contains no readable conversation messages")
	}
	return result, nil
}

func handoffCompleteHead(data []byte) []byte {
	if newline := bytes.LastIndexByte(data, '\n'); newline >= 0 {
		return data[:newline+1]
	}
	return nil
}

func (reader *handoffReader) read(data []byte) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return
	}
	if json.Valid(data) {
		reader.record(data, 0)
		return
	}
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if !json.Valid(line) {
			reader.truncated = true
			continue
		}
		reader.record(line, 0)
	}
}

func (reader *handoffReader) record(data []byte, depth int) {
	if depth > 8 {
		reader.truncated = true
		return
	}
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		var messages []json.RawMessage
		if json.Unmarshal(data, &messages) == nil {
			for _, message := range messages {
				reader.record(message, depth+1)
			}
		}
		return
	}
	var envelope handoffEnvelope
	if json.Unmarshal(data, &envelope) != nil || envelope.IsMeta || envelope.IsSidechain {
		return
	}
	if reader.pi && envelope.ID != "" && envelope.Type != "session" {
		reader.parents[envelope.ID] = envelope.ParentID
		reader.leaf = envelope.ID
	}
	switch envelope.Type {
	case "response_item":
		var message handoffEnvelope
		if json.Unmarshal(envelope.Payload, &message) == nil && message.Type == "message" {
			reader.message(message, 'r', "")
		}
	case "event_msg":
		var event struct{ Type, Message string }
		if json.Unmarshal(envelope.Payload, &event) != nil {
			return
		}
		role := ""
		switch event.Type {
		case "user_message":
			role = "user"
		case "agent_message":
			role = "assistant"
		}
		if role != "" {
			reader.add(role, handoffCleanText(event.Message), 'e', "")
		}
	case "", "message", "user", "assistant":
		if len(envelope.Message) > 0 {
			var message handoffEnvelope
			if json.Unmarshal(envelope.Message, &message) == nil {
				if message.Role == "" && (envelope.Type == "user" || envelope.Type == "assistant") {
					message.Role = envelope.Type
				}
				reader.message(message, 0, envelope.ID)
			}
		} else {
			if envelope.Role == "" && (envelope.Type == "user" || envelope.Type == "assistant") {
				envelope.Role = envelope.Type
			}
			reader.message(envelope, 0, envelope.ID)
		}
		for _, message := range envelope.Messages {
			reader.record(message, depth+1)
		}
	}
}

func (reader *handoffReader) message(message handoffEnvelope, origin byte, id string) {
	if message.IsMeta || message.IsSidechain || message.Channel == "analysis" || message.Channel == "summary" {
		return
	}
	if message.Role != "user" && message.Role != "assistant" {
		return
	}
	text := ""
	if len(message.Content) > 0 {
		if json.Unmarshal(message.Content, &text) != nil {
			var parts []struct{ Type, Text string }
			if json.Unmarshal(message.Content, &parts) != nil {
				return
			}
			var texts []string
			for _, part := range parts {
				if part.Type == "text" || part.Type == "input_text" || part.Type == "output_text" {
					if cleaned := handoffCleanText(part.Text); cleaned != "" {
						texts = append(texts, cleaned)
					}
				}
			}
			text = strings.Join(texts, "\n\n")
		}
	} else {
		text = message.Text
	}
	reader.add(message.Role, handoffCleanText(text), origin, id)
}

func handoffCleanText(text string) string {
	text = strings.TrimSpace(strings.ToValidUTF8(text, ""))
	for _, prefix := range []string{
		"# AGENTS.md instructions", "<environment_context>", "<INSTRUCTIONS>",
		"<turn_aborted>", "<permissions instructions>", "<system-reminder>",
		"<system_reminder>", "<user_instructions>", "<ide_opened_file>",
		"<local-command-caveat>", "<local-command-stdout>", "<command-name>",
	} {
		if strings.HasPrefix(text, prefix) {
			return ""
		}
	}
	if strings.HasPrefix(text, "# Files mentioned by the user:") {
		if _, request, found := strings.Cut(text, "## My request:"); found {
			text = strings.TrimSpace(request)
		}
	}
	return text
}

func (reader *handoffReader) add(role, text string, origin byte, id string) {
	if text == "" {
		return
	}
	digest := sha256.Sum256([]byte(text))
	if origin != 0 {
		for previous := len(reader.entries) - 1; previous >= max(0, len(reader.entries)-8); previous-- {
			entry := &reader.entries[previous]
			if entry.dropped {
				continue
			}
			if entry.Role != role {
				break
			}
			if entry.origin == 0 || entry.origin == origin || entry.digest != digest {
				continue
			}
			if origin == 'e' {
				entry.origin = 0
				return
			}
			entry.dropped = true
			origin = 0
			break
		}
	}
	if len(text) > handoffMessageText {
		reader.truncated = true
		text = handoffShorten(text, handoffMessageText)
	}
	reader.entries = append(reader.entries, handoffEntry{
		HandoffMessage: HandoffMessage{Role: role, Text: text}, origin: origin, digest: digest, id: id,
	})
}

func handoffShorten(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	const gap = "\n[…]\n"
	budget := limit - len(gap)
	start, end := budget/3, len(text)-(budget-budget/3)
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	return text[:start] + gap + text[end:]
}

func (reader *handoffReader) finish() Handoff {
	var active map[string]bool
	if reader.pi && reader.leaf != "" {
		active = map[string]bool{}
		for current := reader.leaf; current != "" && !active[current]; current = reader.parents[current] {
			active[current] = true
		}
	}
	var messages []HandoffMessage
	for _, entry := range reader.entries {
		if entry.dropped {
			continue
		}
		if active != nil && entry.id != "" && !active[entry.id] {
			if !reader.truncated || len(messages) > 0 || entry.Role != "user" {
				continue
			}
		}
		messages = append(messages, entry.HandoffMessage)
	}
	firstUser := slices.IndexFunc(messages, func(message HandoffMessage) bool { return message.Role == "user" })
	result := Handoff{Truncated: reader.truncated}
	remaining := handoffTextLimit
	if firstUser >= 0 {
		remaining -= len(messages[firstUser].Text)
	}
	var recent []HandoffMessage
	for index := len(messages) - 1; index >= 0; index-- {
		if index == firstUser {
			break
		}
		message := messages[index]
		if len(recent) >= handoffMessageLimit-1 || len(message.Text) > remaining {
			result.Truncated = true
			break
		}
		remaining -= len(message.Text)
		recent = append(recent, message)
	}
	if firstUser >= 0 {
		result.Messages = append(result.Messages, messages[firstUser])
	}
	slices.Reverse(recent)
	result.Messages = append(result.Messages, recent...)
	return result
}

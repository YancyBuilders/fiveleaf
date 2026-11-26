package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
)

var agentToolInputFields = []struct {
	keys  []string
	paths bool
}{
	{keys: []string{"command", "cmd", "argv", "args"}},
	{keys: []string{"query", "queries", "searchQuery", "search_query", "pattern", "regex", "keyword", "glob", "include"}},
	{keys: []string{"path", "file", "filepath", "file_path", "filePath", "filename", "fileName", "paths", "cwd", "directory", "dir"}, paths: true},
	{keys: []string{"url", "uri", "service", "serviceName", "service_name", "environment", "project", "namespace"}},
}

func (a *app) agentToolInputSummary(tool *agentTool) string {
	const maxLines = 12
	var lines []string
	add := func(text string) {
		if text = strings.TrimSpace(text); text == "" || len(lines) == maxLines {
			return
		}
		line, clipped := agentVisibleText(text, 1024)
		if clipped {
			line += "…"
		}
		if !slices.Contains(lines, line) {
			lines = append(lines, line)
		}
	}
	if len(tool.rawInput) <= 8*1024 {
		var input map[string]json.RawMessage
		if json.Unmarshal(tool.rawInput, &input) == nil {
			scopes := []map[string]json.RawMessage{input}
			for _, key := range []string{"arguments", "action"} {
				var fields map[string]json.RawMessage
				if json.Unmarshal(input[key], &fields) == nil && fields != nil {
					scopes = append(scopes, fields)
				}
			}
			for _, fields := range scopes {
				for _, group := range agentToolInputFields {
					for _, key := range group.keys {
						for _, value := range agentToolInputValues(fields[key]) {
							if group.paths && value != "" {
								value = a.relPath(value)
							}
							add(value)
						}
					}
				}
			}
		}
	}
	for _, location := range tool.locations[:min(len(tool.locations), maxLines)] {
		if location.Path == "" {
			continue
		}
		label := a.relPath(location.Path)
		if location.Line != nil {
			label += ":" + strconv.Itoa(*location.Line)
		}
		add(label)
	}
	if len(lines) == 0 {
		for _, change := range tool.changes[:min(len(tool.changes), maxLines)] {
			if change.path != "" {
				add(a.relPath(change.path))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func agentToolInputValues(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []string{text}
	}
	var values []string
	if json.Unmarshal(raw, &values) == nil {
		return values
	}
	return nil
}

func (tool *agentTool) outputText() string {
	for _, text := range []string{tool.text, tool.terminalOutput.String(), tool.rawOutput, tool.progressOutput.String()} {
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func formatAgentToolOutput(value any) string {
	if raw, ok := value.(json.RawMessage); ok {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return ""
		}
	}
	var output bytes.Buffer
	appendAgentToolValue(&output, value, "", 0)
	return output.String()
}

func appendAgentToolValue(output *bytes.Buffer, value any, label string, depth int) {
	const maxDepth = 8
	if value == nil || output.Len() >= agentTextLimit || depth > maxDepth {
		return
	}
	prefix := strings.Repeat("  ", depth)
	if label != "" {
		prefix += label + ": "
	}
	write := func(text string) {
		writeAgentToolOutput(output, prefix, text)
	}
	children := func() int {
		if label == "" {
			return depth
		}
		writeAgentToolOutput(output, "", strings.TrimSuffix(prefix, " "))
		return depth + 1
	}
	switch value := value.(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			write(value)
		}
	case bool:
		write(strconv.FormatBool(value))
	case float64:
		write(strconv.FormatFloat(value, 'g', -1, 64))
	case json.Number:
		write(value.String())
	case []any:
		if len(value) == 0 {
			return
		}
		start := output.Len()
		depth = children()
		contentStart := output.Len()
		for _, entry := range value {
			appendAgentToolValue(output, entry, "", depth)
		}
		if contentStart == output.Len() {
			output.Truncate(start)
		}
	case map[string]any:
		kind, _ := value["type"].(string)
		switch kind {
		case "text":
			appendAgentToolValue(output, value["text"], label, depth)
			return
		case "image", "audio", "resource_link":
			uri, _ := value["uri"].(string)
			name, _ := value["name"].(string)
			if resource := agentToolResourceLabel(uri, name); resource != "" {
				write(resource)
			} else if mime, _ := value["mimeType"].(string); mime != "" {
				write(mime)
			}
			return
		}
		if len(value) == 0 {
			return
		}
		start := output.Len()
		depth = children()
		contentStart := output.Len()
		_, imageResult := value["revisedPrompt"]
		for _, key := range slices.Sorted(maps.Keys(value)) {
			if key != "_meta" && key != "blob" && !(imageResult && key == "result") {
				appendAgentToolValue(output, value[key], key, depth)
			}
		}
		if contentStart == output.Len() {
			output.Truncate(start)
		}
	}
}

func writeAgentToolOutput(output *bytes.Buffer, prefix, text string) {
	if output.Len() > 0 {
		output.WriteByte('\n')
	}
	prefix, _ = agentVisibleText(prefix, min(1024, agentTextLimit-output.Len()))
	output.WriteString(prefix)
	text, _ = agentVisibleText(text, agentTextLimit-output.Len())
	output.WriteString(text)
}

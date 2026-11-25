package roundtable

import (
	"regexp"
	"strings"
)

var piBanner = regexp.MustCompile(`^pi v[0-9]+\.[0-9]+\.[0-9]+[^\s]*\r?\n---\r?\n`)

func legacyPiAnswer(text string) string {
	prefix := piBanner.FindString(text)
	if prefix == "" {
		return text
	}
	rest := strings.TrimLeft(text[len(prefix):], "\r\n")
	seen := false
	for {
		heading, body, _ := strings.Cut(rest, "\n")
		switch strings.TrimSuffix(heading, "\r") {
		case "## Context", "## Skills", "## Prompts", "## Extensions":
		default:
			if !seen {
				return text
			}
			return rest
		}
		items := 0
		for strings.HasPrefix(body, "- ") || strings.HasPrefix(body, "  - ") {
			line, tail, _ := strings.Cut(body, "\n")
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			if !piInventoryItem(strings.TrimSpace(heading), value) {
				break
			}
			items++
			body = tail
		}
		if items == 0 {
			return text
		}
		seen = true
		rest = strings.TrimLeft(body, "\r\n")
	}
}

func piInventoryItem(heading, value string) bool {
	path := strings.ReplaceAll(value, `\`, "/")
	switch heading {
	case "## Context":
		return strings.HasSuffix(path, "/AGENTS.md")
	case "## Skills":
		return strings.Contains(path, "/skills/") && strings.HasSuffix(path, ".md")
	case "## Prompts":
		command, ok := strings.CutPrefix(path, "/")
		return ok && command != "" && !strings.ContainsAny(command, "/ \t.")
	case "## Extensions":
		return strings.Contains(path, "/extensions/") && (strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".js")) || strings.HasPrefix(path, "npm:") || path == "index.ts"
	}
	return false
}

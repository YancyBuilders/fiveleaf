package roundtable

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

func directMember(members []Member, target string) (Member, bool) {
	target = strings.TrimPrefix(strings.TrimSpace(target), "@")
	for _, member := range members {
		if strings.EqualFold(member.ID, target) {
			return member, available(member)
		}
	}
	var match Member
	for _, member := range members {
		if strings.EqualFold(member.Name, target) {
			if match.ID != "" {
				return Member{}, false
			}
			match = member
		}
	}
	return match, available(match)
}

func routedMember(members []Member, text string, next *int) (Member, bool) {
	return routeAfter(members, text, next, "")
}

func routeAfter(members []Member, text string, next *int, lastMember string) (Member, bool) {
	if len(members) == 0 {
		return Member{}, false
	}
	choose := func(i int) (Member, bool) {
		*next = (i + 1) % len(members)
		return members[i], true
	}
	if hasLeadingMention(text) {
		if index := mentionedMember(members, text); index >= 0 {
			return choose(index)
		}
		return Member{}, false
	}
	if capability := keywordCapability(text); capability != "" {
		for offset := range len(members) {
			i := (*next + offset) % len(members)
			if members[i].ID != lastMember && available(members[i]) && slices.ContainsFunc(members[i].Capabilities, func(s string) bool {
				return strings.EqualFold(strings.TrimSpace(s), capability)
			}) {
				return choose(i)
			}
		}
	}
	for offset := range len(members) {
		i := (*next + offset) % len(members)
		if members[i].ID != lastMember && available(members[i]) {
			return choose(i)
		}
	}
	return Member{}, false
}

func mentionWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
}

func hasLeadingMention(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "@")
}

// Only a leading mention addresses an agent. Find the longest matching alias
// before checking availability so a disabled long name cannot select a shorter
// enabled name and disclose a message intended for the disabled member.
func mentionedMember(members []Member, text string) int {
	index, _ := matchMemberMention(members, text)
	if index >= 0 && available(members[index]) {
		return index
	}
	return -1
}

func MentionToken(text, alias string) string {
	text = strings.TrimLeftFunc(text, unicode.IsSpace)
	if alias == "" || !strings.HasPrefix(text, "@") {
		return ""
	}
	end := 1
	for range alias {
		if end >= len(text) {
			return ""
		}
		_, width := utf8.DecodeRuneInString(text[end:])
		end += width
	}
	if !strings.EqualFold(text[1:end], alias) {
		return ""
	}
	after, width := utf8.DecodeRuneInString(text[end:])
	if after == '.' || after == ':' {
		after, _ = utf8.DecodeRuneInString(text[end+width:])
	}
	if mentionWord(after) {
		return ""
	}
	return text[:end]
}

func matchMemberMention(members []Member, text string) (int, string) {
	best, longest := -1, ""
	idMatch := false
	for position, member := range members {
		for index, alias := range []string{member.ID, member.Name} {
			token := MentionToken(text, alias)
			if token == "" || len(token) < len(longest) {
				continue
			}
			if len(token) > len(longest) || !idMatch && index == 0 {
				best, longest, idMatch = position, token, index == 0
			} else if !idMatch && best != position {
				best = -1
			}
		}
	}
	return best, longest
}

func keywordCapability(text string) string {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, group := range []struct {
		capability string
		words      []string
		phrases    []string
	}{
		{"code", []string{"code", "coding", "implement", "debug", "refactor", "fix"}, []string{"代码", "编程", "实现", "修复"}},
		{"search", []string{"search", "lookup", "find", "research"}, []string{"搜索", "查找", "检索", "调研"}},
		{"reasoning", []string{"reasoning", "reason", "analyze", "analysis", "think", "plan"}, []string{"推理", "分析", "思考", "规划"}},
	} {
		for _, word := range group.words {
			if slices.Contains(words, word) {
				return group.capability
			}
		}
		for _, phrase := range group.phrases {
			if strings.Contains(text, phrase) {
				return group.capability
			}
		}
	}
	return ""
}

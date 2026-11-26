package acp

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"
)

const errorMessageLimit = 1024

var errorEscapeSequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

// ErrorMessage formats an ACP error for display while leaving the original error intact.
func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var hint *requestHint
	if errors.As(err, &hint) && hint != nil && hint.message != "" {
		return boundedErrorText(hint.message, errorMessageLimit)
	}
	var rpc *sdk.RequestError
	if !errors.As(err, &rpc) || rpc == nil {
		return boundedErrorText(err.Error(), errorMessageLimit)
	}
	code := strconv.Itoa(rpc.Code)
	suffix := " (ACP " + code + ")"
	limit := errorMessageLimit - len(suffix)
	message := boundedErrorText(rpc.Message, limit)
	if genericRPCMessage(rpc.Code, message) {
		if detail := rpcErrorDetail(rpc.Data, limit); detail != "" {
			message = detail
		}
	}
	if message == "" {
		return "ACP error " + code
	}
	return message + suffix
}

func genericRPCMessage(code int, message string) bool {
	switch code {
	case -32600:
		return message == "" || message == "Invalid request"
	case -32602:
		return message == "" || message == "Invalid params"
	case -32603:
		return message == "" || message == "Internal error"
	default:
		return false
	}
}

func rpcErrorDetail(data any, limit int) string {
	for _, key := range []string{"details", "error"} {
		var text string
		switch fields := data.(type) {
		case map[string]any:
			text, _ = fields[key].(string)
		case map[string]string:
			text = fields[key]
		}
		text = strings.TrimSpace(text)
		if text == "" || text[0] == '{' || text[0] == '[' && strings.HasSuffix(text, "]") {
			continue
		}
		if text = boundedErrorText(text, limit); text != "" {
			return text
		}
	}
	return ""
}

func boundedErrorText(text string, limit int) string {
	text = errorEscapeSequence.ReplaceAllString(text, "")
	text = strings.TrimSpace(strings.Map(func(character rune) rune {
		if character != '\n' && character != '\t' && (unicode.IsControl(character) || unicode.In(character, unicode.Bidi_Control)) {
			return -1
		}
		return character
	}, text))
	if len(text) <= limit {
		return text
	}
	end := limit - len("…")
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return strings.TrimSpace(text[:end]) + "…"
}

package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"
)

func TestACPErrorMessage(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "none"},
		{name: "typed", err: context.DeadlineExceeded, want: "context deadline exceeded"},
		{name: "hint", err: fmt.Errorf("opening session: %w", &requestHint{sdk.NewAuthRequired("private-key"), "Sign in first."}), want: "Sign in first."},
		{name: "known details", err: fmt.Errorf("prompt: %w", sdk.NewInternalError(map[string]any{"details": "The provider rejected the account.", "key": "unused"})), want: "The provider rejected the account. (ACP -32603)"},
		{name: "known error", err: sdk.NewInternalError(map[string]any{"error": "Failed to read provider configuration."}), want: "Failed to read provider configuration. (ACP -32603)"},
		{name: "specific message", err: &sdk.RequestError{Code: -32002, Message: "Session not found", Data: map[string]any{"details": "never render"}}, want: "Session not found (ACP -32002)"},
		{name: "generic data", err: sdk.NewInternalError(map[string]any{"details": map[string]any{"raw": "never render"}, "error": "{\"nested\":\"never render\"}"}), want: "Internal error (ACP -32603)"},
		{name: "unknown generic message", err: &sdk.RequestError{Code: -32040, Message: "Internal error", Data: map[string]any{"details": "never render"}}, want: "Internal error (ACP -32040)"},
		{name: "codes only", err: &sdk.RequestError{Code: -32040}, want: "ACP error -32040"},
		{name: "control characters", err: &sdk.RequestError{Code: -32602, Message: "Invalid params", Data: map[string]string{"details": "\x1b[31m消息\nFailed\x00"}}, want: "消息\nFailed (ACP -32602)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ErrorMessage(test.err); got != test.want {
				t.Fatalf("ErrorMessage = %q; want %q", got, test.want)
			}
		})
	}
}

func TestACPErrorMessageBoundsUTF8AndRetainsCode(t *testing.T) {
	rpc := sdk.NewInternalError(map[string]any{"details": strings.Repeat("检查😀", 2048)})
	wrapped := fmt.Errorf("prompt: %w", rpc)
	got := ErrorMessage(wrapped)
	if !utf8.ValidString(got) || len(got) > 1024 || !strings.HasSuffix(got, "… (ACP -32603)") {
		t.Fatalf("formatter lost UTF-8, truncation or diagnostic code: len=%d, valid=%v", len(got), utf8.ValidString(got))
	}
	var kept *sdk.RequestError
	if !errors.As(wrapped, &kept) || kept != rpc || !strings.Contains(kept.Error(), strings.Repeat("检查😀", 2048)) {
		t.Fatal("formatting changed the original typed RPC error")
	}
}

package acp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/coder/acp-go-sdk"
)

type textReply struct {
	catalogHandler
	mu       sync.Mutex
	session  SessionID
	prelude  string
	text     strings.Builder
	overflow bool
}

func (reply *textReply) Update(update Update) {
	chunk := update.Update.AgentMessageChunk
	if chunk == nil || chunk.Content.Text == nil {
		return
	}
	reply.mu.Lock()
	defer reply.mu.Unlock()
	if update.SessionId != reply.session || reply.overflow {
		return
	}
	text := chunk.Content.Text.Text
	if reply.prelude != "" && text == reply.prelude {
		reply.prelude = ""
		return
	}
	if reply.text.Len()+len(text) > 64<<10 {
		reply.overflow = true
		return
	}
	reply.text.WriteString(text)
}

func GenerateText(ctx context.Context, config Config, workspace, model, prompt string) (string, error) {
	reply := &textReply{}
	client, err := Start(ctx, config, workspace, reply)
	if err != nil {
		return "", err
	}
	defer client.Close()
	if _, err = client.initialize(ctx, sdk.ClientCapabilities{}); err != nil {
		return "", err
	}
	session, err := client.NewSession(ctx)
	if err != nil {
		return "", err
	}
	id := string(session.SessionId)
	if err := client.SetModel(ctx, id, model); err != nil {
		return "", err
	}
	reply.mu.Lock()
	reply.session = session.SessionId
	reply.prelude = StartupInfo(session)
	reply.mu.Unlock()
	result, err := client.Prompt(ctx, id, []ContentBlock{TextBlock(prompt)})
	if err != nil {
		return "", err
	}
	if result.StopReason != "end_turn" {
		return "", fmt.Errorf("agent stopped: %s", result.StopReason)
	}
	reply.mu.Lock()
	defer reply.mu.Unlock()
	if reply.overflow {
		return "", fmt.Errorf("agent reply exceeds 64 KiB")
	}
	return reply.text.String(), nil
}

func StartupInfo(session Session) string {
	pi, _ := session.Meta["piAcp"].(map[string]any)
	text, _ := pi["startupInfo"].(string)
	return text
}

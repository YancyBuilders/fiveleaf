package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

var errPromptIdle = errors.New("ACP prompt is idle")

type stoppablePrompt struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	idle    time.Duration
	events  PromptEvents
	written chan struct{}
	sent    bool
}

func (c *Client) stoppablePromptForFrame(frame []byte) (*sessionState, *stoppablePrompt) {
	if !bytes.Contains(frame, []byte(`"method":"session/prompt"`)) {
		return nil, nil
	}
	var request struct {
		Method string
		Params struct{ SessionID string }
	}
	if json.Unmarshal(frame, &request) != nil || request.Method != "session/prompt" {
		return nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	session := c.sessions[request.Params.SessionID]
	if session == nil {
		return nil, nil
	}
	return session, session.promptStop
}

func (c *Client) finishPromptWrite(session *sessionState, stop *stoppablePrompt) {
	c.mu.Lock()
	if stop.idle > 0 && stop.ctx.Err() == nil {
		c.watchPromptIdle(session, stop.idle, func() { stop.cancel(errPromptIdle) })
	}
	c.mu.Unlock()
	if stop.events.Written != nil {
		stop.events.Written()
	}
	close(stop.written)
}

func (c *Client) requestStoppablePrompt(params sdk.PromptRequest, prompt *stoppablePrompt) (PromptResult, bool, error) {
	promptDone, stopDone := make(chan struct{}), make(chan struct{})
	var cancelSent bool
	stop := context.AfterFunc(prompt.ctx, func() {
		defer close(stopDone)
		select {
		case <-prompt.written:
		case <-promptDone:
			return
		case <-c.ctx.Done():
			return
		}
		select {
		case <-promptDone:
			return
		case <-c.ctx.Done():
			return
		default:
		}
		if context.Cause(prompt.ctx) == errPromptIdle && prompt.events.Idle != nil {
			prompt.events.Idle()
		}
		cancelSent = c.Cancel(c.ctx, string(params.SessionId)) == nil
	})
	result, err := request[PromptResult](c, c.ctx, "session/prompt", params)
	if err == nil {
		result = capturePromptUsage(string(params.SessionId), result)
		c.mu.Lock()
		if session := c.sessions[string(params.SessionId)]; session != nil {
			session.promptTokens = result.tokens
			session.promptTokensAvailable = result.tokens.Input+result.tokens.CachedInput > 0
		}
		c.mu.Unlock()
	}
	close(promptDone)
	if !stop() {
		<-stopDone
	}
	if cause := context.Cause(prompt.ctx); cause != nil {
		if cause == errPromptIdle {
			cause = context.DeadlineExceeded
		}
		var rpc *sdk.RequestError
		cancelled := cancelSent && errors.As(err, &rpc) && rpc.Code == sdk.NewRequestCancelled(nil).Code
		idle := cancelSent && err == nil && result.StopReason == "cancelled" && errors.Is(cause, context.DeadlineExceeded)
		if !prompt.sent || cancelled || idle {
			return result, prompt.sent, cause
		}
	}
	if c.ctx.Err() != nil && errors.Is(err, c.ctx.Err()) {
		err = c.Err()
		if err == nil {
			err = ErrClosed
		}
	}
	return result, prompt.sent, err
}

type promptIdleWatch struct {
	timer    *time.Timer
	timeout  time.Duration
	deadline time.Time
}

func (c *Client) watchPromptIdle(session *sessionState, timeout time.Duration, onIdle func()) {
	watch := &promptIdleWatch{timeout: timeout, deadline: time.Now().Add(timeout)}
	session.promptIdle = watch
	watch.timer = time.AfterFunc(timeout, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if session.promptIdle != watch || len(session.permissions) > 0 {
			return
		}
		if remaining := time.Until(watch.deadline); remaining > 0 {
			watch.timer.Reset(remaining)
		} else {
			onIdle()
		}
	})
}

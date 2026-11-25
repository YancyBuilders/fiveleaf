package roundtable

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fiveleaf/internal/procgroup"
)

var ErrOutputTooLarge = errors.New("agent output exceeds the size limit")

// CLIAdapter runs one noninteractive CLI call per reply. Args are passed
// directly to the executable; neither prompts nor arguments go through a shell.
type CLIAdapter struct {
	Command        string
	Args           []string
	Env            map[string]string
	Dir            string
	Timeout        time.Duration
	PromptArgument bool
}

func (a CLIAdapter) Call(ctx context.Context, messages []Message) (Message, error) {
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	if strings.TrimSpace(a.Command) == "" || strings.ContainsRune(a.Command, '\x00') {
		return Message{}, errors.New("agent command is required")
	}
	args := slices.Clone(a.Args)
	for _, arg := range args {
		if strings.ContainsRune(arg, '\x00') {
			return Message{}, errors.New("agent argument contains NUL")
		}
	}
	environment := os.Environ()
	for name, value := range a.Env {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, '\x00') {
			return Message{}, errors.New("agent environment variable is invalid")
		}
		environment = append(environment, name+"="+value)
	}
	prompt := FormatPrompt(messages)
	if a.PromptArgument {
		args = append(args, prompt)
	}
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(callCtx, a.Command, args...)
	cmd.Dir, cmd.Env = a.Dir, environment
	if !a.PromptArgument {
		cmd.Stdin = strings.NewReader(prompt)
	}
	procgroup.Prepare(cmd)
	cmd.Cancel = func() error { return procgroup.Stop(cmd) }
	cmd.WaitDelay = time.Second
	stdout := &limitedOutput{limit: MaxOutputBytes, overflow: cancel}
	stderr := &limitedOutput{limit: MaxStderrBytes, tail: true}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	// A wrapper can exit while a child still owns a pipe. WaitDelay bounds the
	// wait; this also tears down its group after a normal or failed wrapper exit.
	_ = procgroup.Stop(cmd)
	message := Message{Role: "assistant", Text: stdout.String(), Time: time.Now()}
	if stdout.exceeded {
		err = ErrOutputTooLarge
	} else if callCtx.Err() != nil {
		err = callCtx.Err()
	}
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return message, fmt.Errorf("%s: %w\n%s", filepath.Base(a.Command), err, detail)
		}
		return message, fmt.Errorf("%s: %w", filepath.Base(a.Command), err)
	}
	return message, nil
}

// Each stream has one os/exec copy goroutine. The buffers are read only after
// Run has joined those goroutines, including its WaitDelay cleanup.
type limitedOutput struct {
	data     []byte
	limit    int
	tail     bool
	exceeded bool
	overflow func()
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.data)+n > b.limit && !b.exceeded {
		b.exceeded = true
		if b.overflow != nil {
			b.overflow()
		}
	}
	if b.tail {
		if n >= b.limit {
			b.data = append(b.data[:0], p[n-b.limit:]...)
		} else {
			if excess := len(b.data) + n - b.limit; excess > 0 {
				copy(b.data, b.data[excess:])
				b.data = b.data[:len(b.data)-excess]
			}
			b.data = append(b.data, p...)
		}
	} else if remaining := b.limit - len(b.data); remaining > 0 {
		b.data = append(b.data, p[:min(n, remaining)]...)
	}
	return n, nil
}

func (b *limitedOutput) String() string { return boundedText(string(b.data), b.limit) }

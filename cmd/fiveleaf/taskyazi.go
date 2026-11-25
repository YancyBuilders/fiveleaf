package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/tasks"
	"fiveleaf/locales"
)

func yaziEntryArgument(file, platform string) string {
	switch {
	case platform == "windows" && strings.Contains(file, "%"):
		return "%FIVELEAF_FILE%"
	case platform != "windows" && strings.Contains(file, "$"):
		return "$FIVELEAF_FILE"
	default:
		return file
	}
}

func (a *app) launchHiddenYazi(root string, task tasks.Resolved) *session {
	ya, err := exec.LookPath("ya")
	if err != nil {
		return a.launchTaskTerminal(root, task)
	}
	id := strconv.FormatUint(rand.Uint64()|1<<63, 10)
	entry := yaziEntryArgument(task.YaziFile(), hostOS)
	task.Args = []string{task.YaziFile(), "--client-id", id}
	s := a.launchTaskTerminal(root, task)
	if s != nil {
		term := s.term
		bg.Go(func() {
			if err := revealHiddenYazi(ya, id, entry, term.Done()); err != nil {
				a.post(func() {
					if s.term == term && taskRunning(s) {
						a.fail("%s", locales.FailStartTask(task.Task.Label, err.Error()))
					}
				})
			}
		})
	}
	return s
}

func revealHiddenYazi(ya, id, entry string, done <-chan struct{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	emit := func(name, arg string) error {
		output, err := exec.CommandContext(ctx, ya, "emit-to", id, name, arg).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ya emit-to %s %s: %w %s", id, name, err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	for {
		err := emit("hidden", "show")
		if err == nil {
			return emit("reveal", entry)
		}
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return err
		case <-time.After(100 * time.Millisecond):
		}
	}
}

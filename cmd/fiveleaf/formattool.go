package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/plugin"
	"fiveleaf/internal/procgroup"
	"fiveleaf/internal/vfs"
	"github.com/egoist/mygo/ui"
)

type formatterTool struct {
	dir         string
	command     string
	args        []string
	settingArgs map[string]string
}

func formatterProgram(root, file string, formatter *plugin.FormatterContrib, tabSize int, useTabs bool) formatterTool {
	dir := root
	if file != "" && (root == "" || !inFolder(root, file)) {
		dir = filepath.Dir(file)
	}
	expand := strings.NewReplacer("${file}", hostPath(file), "${tabSize}", strconv.Itoa(tabSize), "${useTabs}", strconv.FormatBool(useTabs))
	args := make([]string, len(formatter.Args))
	for index, arg := range formatter.Args {
		args[index] = expand.Replace(arg)
	}
	return formatterTool{dir: dir, command: formatter.Command, args: args, settingArgs: formatter.SettingArgs}
}

func (tool formatterTool) format(ctx context.Context, settings pluginCommandSettings, snap ui.TextSnapshot) ([]textEdit, error) {
	command, err := settings.expand(tool.command)
	if err != nil {
		return nil, err
	}
	args, err := settings.appendSettingArgs(tool.args, tool.settingArgs)
	if err != nil {
		return nil, err
	}
	source := snap.String()
	cmd, err := vfs.Command(tool.dir, nil, strings.NewReader(source), command, args...)
	if err != nil {
		return nil, err
	}
	stdout := limitedBuffer{limit: min(maxOpenBytes, max(4<<20, 4*len(source)))}
	stderr := limitedBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 200 * time.Millisecond
	procgroup.Prepare(cmd)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { procgroup.Stop(cmd) })
	err = cmd.Wait()
	stop()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if reason := strings.TrimSpace(stderr.String()); reason != "" {
			return nil, fmt.Errorf("%s: %w", firstLine(reason), err)
		}
		return nil, err
	}
	if stdout.overflow || !utf8.Valid(stdout.buffer.Bytes()) {
		return nil, fmt.Errorf("%s returned oversized or invalid UTF-8 output", command)
	}
	return formattedTextEdits(snap, stdout.String()), nil
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"go/format"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/linediff"
	"fiveleaf/internal/lsp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func builtinFormatter(language string, indent indentStyle) (string, func([]byte) ([]byte, error)) {
	switch language {
	case "go":
		return "gofmt", format.Source
	case "json":
		return "JSON", func(source []byte) ([]byte, error) {
			var out bytes.Buffer
			if err := json.Indent(&out, bytes.Trim(source, " \t\r\n"), "", indent.unit()); err != nil {
				return nil, err
			}
			if bytes.HasSuffix(source, []byte("\n")) {
				out.WriteByte('\n')
			}
			return out.Bytes(), nil
		}
	}
	return "", nil
}

func (a *app) formatCurrent(doc *editor, snap ui.TextSnapshot, path string) bool {
	return doc.path == path && doc.buf.Version() == snap.Version() && !doc.locked() && slices.Contains(a.docs, doc.document)
}

func (a *app) formatLanguage(doc *editor, limit time.Duration, quiet bool, then func()) {
	snap, path, language := doc.buf.Snapshot(), doc.path, languageOf(doc.document)
	server := a.lspFor(doc.document, false)
	builtinName, builtin := builtinFormatter(language, doc.indent)
	name := builtinName
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	finished := false
	finish := func(apply func(), err error) {
		if finished {
			return
		}
		finished = true
		if expired := ctx.Err(); expired != nil {
			err = expired
		}
		cancel()
		defer then()
		if !a.formatCurrent(doc, snap, path) {
			return
		}
		if err != nil {
			if !quiet {
				a.fail("%s", locales.FormatFailed(name, firstLine(err.Error())))
			}
		} else if apply != nil {
			apply()
		}
	}
	stop := context.AfterFunc(ctx, func() { a.post(func() { finish(nil, ctx.Err()) }) })
	complete := func(apply func(), err error) { stop(); finish(apply, err) }
	run := func() {
		if finished || ctx.Err() != nil || !a.formatCurrent(doc, snap, path) {
			complete(nil, ctx.Err())
			return
		}
		if client := server.live(); client != nil && client.Formatting() {
			name = server.contrib.ID
			uri, size, spaces := serverURI(path), max(doc.indent.cols(), 1), !doc.indent.tabs
			synced := client.OpenAsync(ctx, uri, language, snap.String())
			go func() {
				var edits []lsp.TextEdit
				err := <-synced
				if err == nil {
					edits, err = client.Format(ctx, uri, size, spaces)
				}
				a.post(func() { complete(func() { applyLSPEdits(doc, snap, edits) }, err) })
			}()
			return
		}
		if builtin == nil {
			if !quiet {
				a.fail("%s", locales.FormatNone(language))
			}
			complete(nil, nil)
			return
		}
		name = builtinName
		go func() {
			text, err := builtin([]byte(snap.String()))
			var changes []textEdit
			if err == nil && ctx.Err() == nil {
				changes = formattedTextEdits(snap, string(text))
			}
			a.post(func() { complete(func() { applyFormatEdits(doc, changes) }, err) })
		}()
	}
	if server != nil && server.state() == lspStarting && (!quiet || builtin == nil) {
		name = server.contrib.ID
		go func() {
			select {
			case <-server.ready:
				a.post(run)
			case <-ctx.Done():
			}
		}()
		return
	}
	run()
}

func applyLSPEdits(doc *editor, snap ui.TextSnapshot, edits []lsp.TextEdit) {
	changes := make([]textEdit, 0, len(edits))
	for _, edit := range edits {
		start, end := posRune(snap, fromLSP(edit.Range.Start)), posRune(snap, fromLSP(edit.Range.End))
		if start > end {
			return
		}
		changes = append(changes, textEdit{start, end, edit.NewText})
	}
	applyFormatEdits(doc, changes)
}

func formattedTextEdits(snap ui.TextSnapshot, text string) []textEdit {
	old := snap.String()
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == old || (text == "" && strings.TrimSpace(old) != "") {
		return nil
	}
	if edits, ok := whitespaceEdits(old, text); ok {
		return edits
	}
	before, after := strings.SplitAfter(old, "\n"), strings.SplitAfter(text, "\n")
	var edits []textEdit
	for _, hunk := range linediff.Hunks(before, after) {
		original := strings.Join(before[hunk.A:hunk.A+hunk.ALen], "")
		formatted := strings.Join(after[hunk.B:hunk.B+hunk.BLen], "")
		base := snap.Len()
		if hunk.A < snap.LineCount() {
			base, _ = snap.LineRange(hunk.A)
		}
		start, oldEnd, newEnd := commonEnds(original, formatted)
		edits = append(edits, textEdit{base + utf8.RuneCountInString(original[:start]), base + utf8.RuneCountInString(original[:oldEnd]), formatted[start:newEnd]})
	}
	return edits
}

func whitespaceEdits(before, after string) ([]textEdit, bool) {
	var edits []textEdit
	at := 0
	for len(before) > 0 || len(after) > 0 {
		left, right := strings.TrimLeft(before, " \t\r\n"), strings.TrimLeft(after, " \t\r\n")
		removed, added := before[:len(before)-len(left)], after[:len(after)-len(right)]
		if removed != added {
			edits = append(edits, textEdit{at, at + len(removed), added})
		}
		at += len(removed)
		if len(left) == 0 || len(right) == 0 {
			return edits, len(left) == len(right)
		}
		char, size := utf8.DecodeRuneInString(left)
		next, width := utf8.DecodeRuneInString(right)
		if char != next || left[:size] != right[:width] {
			return nil, false
		}
		at++
		before, after = left[size:], right[width:]
	}
	return edits, true
}

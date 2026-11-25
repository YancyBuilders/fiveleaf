package main

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type meetingFilePicker struct {
	projectFileSearch
	open, focus bool
	start, end  int
}

type projectFileSearch struct {
	query, matched string
	index          int
	moved          bool
	files          []string
	results        []workspace.Match
}

func (w *roundtableWindow) startFilePicker(input ui.Element, picker *meetingFilePicker) {
	anchor, caret := input.TextAnchorCaret()
	*picker = meetingFilePicker{open: true, focus: true, start: min(anchor, caret), end: max(anchor, caret)}
	w.mentions.open, w.mentions.dismissed = false, true
	if w.a.index == nil {
		w.a.indexRead.Invalidate()
	}
	w.a.refreshIndex()
}

func (picker *projectFileSearch) filter(files []string) {
	if picker.matched == picker.query && len(files) == len(picker.files) && (len(files) == 0 || &files[0] == &picker.files[0]) {
		return
	}
	picker.files, picker.matched = files, picker.query
	picker.results = workspace.Fuzzy(files, picker.query, 60)
	picker.index, picker.moved = 0, true
}

func (w *roundtableWindow) insertFileReference(picker *meetingFilePicker, index int) {
	if index < 0 || index >= len(picker.results) {
		return
	}
	path := filepath.ToSlash(picker.files[picker.results[index].Index])
	delimiter := strings.Repeat("`", mdBacktickRun(path)+1)
	padding := ""
	if strings.HasPrefix(path, "`") || strings.HasSuffix(path, "`") || strings.HasPrefix(path, " ") && strings.HasSuffix(path, " ") {
		padding = " "
	}
	ref := delimiter + padding + path + padding + delimiter
	text := []rune(w.draft)
	start, end := min(max(0, picker.start), len(text)), min(max(0, picker.end), len(text))
	if start > 0 && !unicode.IsSpace(text[start-1]) {
		ref = " " + ref
	}
	if end == len(text) || !unicode.IsSpace(text[end]) {
		ref += " "
	}
	w.draft = string(text[:start]) + ref + string(text[end:])
	w.mentions.restore = start + utf8.RuneCountInString(ref)
	picker.open = false
}

func (w *roundtableWindow) filePicker(c *ui.Context, input ui.Element, picker *meetingFilePicker) {
	_, height := c.Size()
	pal := w.pal
	panel := ui.PopoverBase(c.Key("meeting-files"), input, &picker.open, func(panel ui.Element) {
		w.meetingPickerPanel(panel, input).Label(locales.RoundtableReferenceFile()).Gap(4).Children(func() {
			field := ui.TextInput(c.Key("file-query"), &picker.query).FillWidth().MinWidth(0).Label(locales.QuickSearchFiles()).Placeholder(locales.QuickSearchFiles())
			if picker.focus {
				field.Focus()
				picker.focus = false
			}
			picker.filter(w.a.index)
			composing := field.Composing()
			field.OnTextKey(func(key ui.TextKey) bool {
				if composing {
					return key.Key == ui.KeyEnter
				}
				if key.Mods != 0 {
					return false
				}
				switch key.Key {
				case ui.KeyUp, ui.KeyDown:
					delta := 1
					if key.Key == ui.KeyUp {
						delta = -1
					}
					if count := len(picker.results); count > 0 {
						picker.index, picker.moved = (picker.index+delta+count)%count, true
					}
				case ui.KeyEnter, ui.KeyTab:
					picker.query = key.Text()
					picker.filter(w.a.index)
					w.insertFileReference(picker, picker.index)
				case ui.KeyEscape:
					picker.open, w.focusDraft = false, true
				default:
					return false
				}
				return true
			})
			if len(picker.results) == 0 {
				message := locales.QuickNoFiles()
				if w.a.index == nil && w.a.indexRead.Running() {
					message = locales.QuickIndexing()
				}
				ui.Text(c, message).FillWidth().Padding(8).FontSize(12).TextColor(pal.muted)
				return
			}
			ui.Scroll(c.Key("candidates")).FillWidth().MaxHeight(min(220, max(42, height/2-78))).Children(func() {
				for index, match := range picker.results {
					path := picker.files[match.Index]
					row := ui.ButtonBase(c.Key(path)).Label(path).Tooltip(path).Selected(index == picker.index).KeepFocus().FillWidth().MinWidth(0).MinHeight(27).Padding(4, 6).Gap(6).Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if index == picker.index {
						row.Background(pal.accent.Alpha(0.12))
						if picker.moved {
							row.ScrollIntoView()
						}
					} else if row.Hovered() {
						row.Background(pal.hover)
					}
					row.Children(func() {
						ui.Icon(c, iconFileText).FontSize(13).TextColor(pal.muted).Shrink(0)
						ui.Text(c, path).Grow(1).MinWidth(0).FontSize(12).TextColor(pal.text)
					})
					if row.Clicked() {
						w.insertFileReference(picker, index)
					}
				}
			})
			picker.moved = false
			ui.Text(c, locales.RoundtableFileKeys()).FontSize(10.5).TextColor(pal.muted).Padding(3, 6)
		})
	})
	if panel.Dismissed() {
		picker.open = false
	}
}

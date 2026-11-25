package main

import (
	"fiveleaf/internal/keymap"

	"github.com/egoist/mygo/ui"
)

type markdownKeyBinding struct {
	stroke keymap.Stroke
	id     string
	edit   func(*editor)
}

var markdownWritingKeys = [...]markdownKeyBinding{
	{keymap.Stroke{Key: ui.Key0}, "markdown.paragraph", mdHeadingAction(0)},
	{keymap.Stroke{Key: ui.Key1}, "markdown.heading1", mdHeadingAction(1)},
	{keymap.Stroke{Key: ui.Key2}, "markdown.heading2", mdHeadingAction(2)},
	{keymap.Stroke{Key: ui.Key3}, "markdown.heading3", mdHeadingAction(3)},
	{keymap.Stroke{Key: ui.Key4}, "markdown.heading4", mdHeadingAction(4)},
	{keymap.Stroke{Key: ui.Key5}, "markdown.heading5", mdHeadingAction(5)},
	{keymap.Stroke{Key: ui.Key6}, "markdown.heading6", mdHeadingAction(6)},
	{keymap.Stroke{Key: ui.KeyB}, "markdown.bold", toggleMdBold},
	{keymap.Stroke{Key: ui.KeyE}, "markdown.inlineCode", toggleMdCode},
	{keymap.Stroke{Key: ui.KeyI}, "markdown.italic", toggleMdItalic},
	{keymap.Stroke{Key: ui.KeyK}, "markdown.insertLink", insertMdLink},
	{keymap.Stroke{Mods: ui.Shift, Key: ui.KeyX}, "markdown.strikethrough", toggleMdStrike},
	{keymap.Stroke{Mods: ui.Shift, Key: ui.KeyL}, "markdown.insertImage", insertMdImage},
	{keymap.Stroke{Mods: ui.Shift, Key: ui.KeyK}, "markdown.insertCodeBlock", insertMdCodeBlock},
}

type markdownKeySet [len(markdownWritingKeys)]bool

func init() {
	for _, binding := range markdownWritingKeys {
		viewKeys[binding.id] = "CmdOrCtrl+" + binding.stroke.Accel()
	}
}

func (a *app) writingMarkdown(d *editor) bool {
	if d == nil || d.preview != nil || !isMarkdown(d.document) || d.locked() {
		return false
	}
	switch editorMode(a.prefs()) {
	case "vim":
		return d.vim.mode == vimInsert
	case "helix":
		return d.helix.mode == helixInsert
	}
	return true
}

func markdownPrimaryModifier() ui.Modifiers {
	if macKeys() {
		return ui.Super
	}
	return ui.Ctrl
}

func markdownKeyIndex(stroke keymap.Stroke) int {
	primary := markdownPrimaryModifier()
	if stroke.Mods&primary == 0 {
		return -1
	}
	stroke.Mods &^= primary
	for index, binding := range markdownWritingKeys {
		if binding.stroke == stroke {
			return index
		}
	}
	return -1
}

func (a *app) markdownMenuAccel(id string) string {
	for index, binding := range markdownWritingKeys {
		if a.menuMarkdown[index] && binding.id == id {
			stroke := binding.stroke
			stroke.Mods |= markdownPrimaryModifier()
			return stroke.Accel()
		}
	}
	return ""
}

func (a *app) usesMarkdownKey(binding markdownKeyBinding, stroke keymap.Stroke) bool {
	if a.keys == nil || len(a.keys.Bindings) == 0 {
		return true
	}
	return a.keys.Last(binding.id) == nil && !a.editorShortcutReplaced(binding.id, stroke)
}

func (a *app) activeMarkdownKeys() (active markdownKeySet) {
	if a.keyFocus != placeEditor || a.blurred || !a.writingMarkdown(a.activeEditor()) {
		return active
	}
	for index, binding := range markdownWritingKeys {
		stroke := binding.stroke
		stroke.Mods |= markdownPrimaryModifier()
		active[index] = a.usesMarkdownKey(binding, stroke)
	}
	return active
}

func (a *app) menuKeyTaken(stroke keymap.Stroke, active markdownKeySet) bool {
	index := markdownKeyIndex(stroke)
	if index < 0 {
		return a.keys.Binds(stroke)
	}
	if active[index] {
		return true
	}
	binding := markdownWritingKeys[index]
	if a.keys != nil {
		for _, rule := range a.keys.Bindings {
			if len(rule.Keys) > 0 && rule.Keys[0] == stroke && rule.Command != "-"+binding.id {
				return true
			}
		}
	}
	return false
}

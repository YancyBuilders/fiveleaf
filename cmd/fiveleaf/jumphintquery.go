package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (app *app) jumpHintsFocused(context *ui.Context, doc *editor, focused bool) bool {
	state := &doc.jumpHints
	return focused || doc.vscroll.Focused(context) ||
		state.active && (state.queryFocused || state.queryInput.Focused(context))
}

func (app *app) jumpHintQueryView(context *ui.Context, doc *editor) {
	state := &doc.jumpHints
	state.queryFocused = false
	if !state.querying() || state.queryReady {
		return
	}
	palette, font := app.pal, app.font
	command := "HopChar1"
	if state.kind == jumpHintChar2 {
		command = "HopChar2"
	}
	bar := app.floating(context, ui.Row(context.Key("jump-query")).Absolute().Bottom(12).Left(16).Right(16).MaxWidth(320).Height(34).
		Padding(0, 10, 0, 10).Gap(10).AlignItems(ui.Center).Radius(8), false)
	bar.Role(ui.RoleToolbar).Label(command)
	bar.Children(func() {
		ui.Text(context, command).Font(font.family).FontSize(12).FontWeight(600).TextColor(palette.accent)
		input := ui.TextInputBase(context.Key("jump-query-input"), &state.queryText).
			Bind(&state.queryInput).Grow(1).Font(font.family).FontSize(font.size).TextColor(palette.text).
			InputMethod(true).Label(command).Placeholder(strings.Repeat("_", state.queryLength()))
		if app.chording() {
			input.ReadOnly(true)
		}
		input.HandleInput(func(event ui.InputEvent) bool {
			if event.Kind == ui.InputKeyUp && app.held != nil && *app.held == event.Key {
				app.held = nil
			}
			if event.Kind != ui.InputKeyDown {
				return false
			}
			if event.Key == ui.KeyEscape || event.Mods == ui.Ctrl && event.Key == ui.KeyBracketLeft {
				state.cancel()
				doc.focusNext = true
				app.invalidate()
				return true
			}
			return false
		})
		if state.queryFocusNext {
			state.queryFocusNext = false
			doc.focusNext = false
			doc.vscroll.CancelFocus(context)
			state.queryInput.Focus(context)
			input.Focus().SetTextSelection(len(state.query), len(state.query))
		}
		state.queryFocused = input.Focused()
		if state.queryFocused {
			app.keyFocus = placeInput
		}
		if input.Composing() {
			return
		}
		query := []rune(state.queryText)
		state.query = query[:min(len(query), state.queryLength())]
		if len(state.query) == state.queryLength() {
			state.queryText = string(state.query)
			state.queryReady = true
			doc.focusNext = true
			app.invalidate()
		}
	})
}

package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/smallnest/uix/components/button"
	"github.com/smallnest/uix/components/empty"
	"github.com/smallnest/uix/components/input"
	"github.com/smallnest/uix/components/kbd"
	"github.com/smallnest/uix/components/list"
	"github.com/smallnest/uix/components/separator"

	"fiveleaf/locales"
)

type quickMode struct {
	label, prompt, prefix string
	count, action         string
	// hiddenPrefix leaves the prefix out of the field: a mode the button
	// that opens it says, as the tools the Toolbox button lists.
	hiddenPrefix     bool
	enabled          bool
	choose, openSide func(int)
}

type pickerPrompt struct {
	label, prompt, prefix, count string
	hiddenPrefix                 bool
	value                        *string
	disabled                     bool
	trailing                     func()
	close, escape                func()
	shook                        time.Time
}

type pickerHint struct{ key, label string }

type pickerBar struct {
	action, tip, cancel string
	enabled, hideNav    bool
	choose, close       func()
	hints               []pickerHint
}

func (a *app) pickerDialog(c *ui.Context, prompt pickerPrompt, keys func(ui.Element), results func()) ui.Element {
	metrics := a.pal.metricsOr()
	_, height := c.Size()
	top := titleBarH + 2*metrics.inset
	open := true
	dismiss := prompt.close
	panel := ui.DialogBase(c, &open, func(back, panel ui.Element) {
		back.Justify(ui.Start)
		panel.Column().Width(576).MaxWidthPercent(92).MaxHeight(max(0, height-top-metrics.inset)).
			Margin(top, 0, 0, 0).Gap(metrics.pad).Label(prompt.label)
		a.dialogEntrance(c, panel)
		shake(c, panel, prompt.shook)
		if panel.Dismissed() && prompt.escape != nil && !back.Clicked() {
			dismiss = prompt.escape
		}
		a.pickerSearch(c, prompt, keys)
		menu := a.floating(c, ui.Column(c.Key("quick-menu")).MinHeight(0).Radius(metrics.radiusCard), false).Clip()
		menu.Children(results)
	})
	if panel.Dismissed() {
		dismiss()
	}
	return panel
}

func (a *app) pickerSearch(c *ui.Context, prompt pickerPrompt, keys func(ui.Element)) {
	metrics, theme := a.pal.metricsOr(), c.Theme()
	size := metrics.textBody + 3
	header := a.floating(c, ui.Row(c), false).Height(metrics.control + 2*metrics.inset).Shrink(0).
		PaddingX(metrics.inset + metrics.pad).Gap(metrics.inset).AlignItems(ui.Center).Radius(metrics.radiusCard).Transition(fieldFocus)
	header.Children(func() {
		if prompt.value != nil {
			ui.Box(c).Grow(1).MinWidth(0).FillHeight().Clip().Children(func() {
				field := input.Input(c.Key("quick-input"), input.Props{Value: prompt.value, Placeholder: prompt.prompt, Disabled: prompt.disabled}).Fill().Padding(0).Border(0, theme.Border)
				field = a.textKeys(field).Font(a.font.family).FontSize(size).AutoFocus().Label(prompt.label).Description(prompt.prompt)
				if field.Focused() {
					header.BorderColor(theme.Accent)
				}
				// A tool's "$" is its mode, not a word to search by; a
				// command's ">" and the others name what is typed after
				// them in their prompt.
				if prompt.prefix != "" && !prompt.hiddenPrefix && strings.TrimLeft(*prompt.value, " ") == prompt.prefix && !field.Composing() {
					prefixWidth, _ := c.MeasureText(0, ui.Span{Text: *prompt.value, Font: a.font.family, Size: size})
					ui.Row(c).Absolute().Left(prefixWidth + metrics.gap).Right(0).Top(0).Bottom(0).AlignItems(ui.Center).PassThrough().Children(func() {
						ui.Text(c, prompt.prompt).FontSize(size).TextColor(theme.TextMuted).SingleLine().PassThrough()
					})
				}
				keys(field)
			})
		}
		if prompt.trailing != nil {
			prompt.trailing()
		}
		if prompt.count != "" {
			ui.Text(c, prompt.count).FontFeatures("tnum").FontSize(metrics.textSmall).TextColor(theme.TextMuted).SingleLine().Shrink(0)
		}
	})
}

func (a *app) pickerActions(c *ui.Context, bar pickerBar) {
	metrics := a.pal.metricsOr()
	width, _ := c.Size()
	separator.Separator(c, separator.Props{})
	ui.Row(c).Height(metrics.control+2*metrics.gap).Shrink(0).Padding(0, metrics.inset).Gap(metrics.pad).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Clip().Gap(metrics.pad).AlignItems(ui.Center).Children(func() {
			hint := func(key, label string) {
				kbd.Kbd(c, kbd.Props{Label: key})
				ui.Text(c, label).FontSize(metrics.textSmall).TextColor(c.Theme().TextMuted).SingleLine()
			}
			if !bar.hideNav {
				hint("↑↓", locales.QuickNavigate())
			}
			if width >= 560 {
				for _, item := range bar.hints {
					hint(item.key, item.label)
				}
				if bar.tip != "" {
					ui.Text(c, bar.tip).FontSize(metrics.textSmall).TextColor(c.Theme().TextMuted).MinWidth(0).SingleLine().Tooltip(bar.tip)
				}
			}
		})
		cancel := bar.cancel
		if cancel == "" {
			cancel = locales.ButtonClose()
		}
		button.Button(c, button.Props{Label: keyText("Escape") + "  " + cancel, Variant: button.Ghost, Size: button.Sm,
			OnClick: bar.close}).Label(cancel).Shrink(0)
		if bar.action != "" && actionButton(c.Key("quick-confirm"), keyText("Enter")+"  "+bar.action, nil).Disabled(!bar.enabled).Label(bar.action).Shrink(0).Clicked() {
			bar.choose()
		}
	})
}

func pickerNavigate(field ui.Element, selected *int, moved *bool, count int) {
	if field.Shortcut(0, ui.KeyDown) {
		*selected, *moved = max(0, min(*selected+1, count-1)), true
	}
	if field.Shortcut(0, ui.KeyUp) {
		*selected, *moved = max(*selected-1, 0), true
	}
}

func (a *app) quickPicker(c *ui.Context, mode quickMode, results func()) {
	quick := &a.quick
	prompt := pickerPrompt{label: mode.label, prompt: mode.prompt, prefix: mode.prefix, hiddenPrefix: mode.hiddenPrefix, count: mode.count,
		value: &quick.query, close: a.closeQuick, shook: quick.shook}
	choose := func() { mode.choose(quick.sel) }
	a.pickerDialog(c.Key("quick-picker"), prompt, func(field ui.Element) {
		if mode.prefix != ":" {
			pickerNavigate(field, &quick.sel, &quick.moved, len(quick.results))
		}
		if mode.openSide != nil && field.Shortcut(ui.Cmd, ui.KeyEnter) {
			mode.openSide(quick.sel)
		} else if field.Submitted() {
			choose()
		}
	}, func() {
		results()
		quick.moved = false
		bar := pickerBar{action: mode.action, enabled: mode.enabled, hideNav: mode.prefix == ":", choose: choose, close: a.closeQuick}
		if mode.openSide != nil {
			if strings.TrimSpace(quick.query) == "" && a.root != "" {
				bar.hints = []pickerHint{{">", locales.QuickCommands()}, {":", locales.QuickLine()}, {"@", locales.QuickSymbol()}, {"$", locales.ToolboxTitle()}}
			} else if mode.enabled {
				bar.hints = []pickerHint{{keyText("CmdOrCtrl+Enter"), locales.QuickOpenToSide()}}
			}
		}
		a.pickerActions(c, bar)
	})
}

const pickerResultsHeight = 320

func (a *app) pickerScroll(c *ui.Context) ui.Element {
	metrics := a.pal.metricsOr()
	return ui.Scroll(c).MaxHeight(pickerResultsHeight+2*metrics.gap).MinHeight(0).Padding(metrics.gap, metrics.pad)
}

func (a *app) pickerSection(c *ui.Context, title string) {
	metrics, theme := a.pal.metricsOr(), c.Theme()
	ui.Row(c).Height(metrics.row + metrics.gap).PaddingX(metrics.inset).Gap(metrics.pad).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(metrics.textSmall).FontWeight(metrics.weightBold).TextColor(theme.TextMuted).SingleLine()
		ui.Box(c).Height(1).Grow(1).Background(theme.Border)
	})
}

func quickMessage(c *ui.Context, icon *ui.SVG, message string) {
	empty.Empty(c, empty.Props{Icon: icon, Title: message}).Height(108).MinHeight(0)
}

func (a *app) quickList(c *ui.Context, items []string, height float32, row func(*ui.Context, int)) {
	quick, metrics := &a.quick, a.pal.metricsOr()
	quick.list.Key = func(index int) any { return items[index] }
	if quick.moved {
		quick.list.ScrollIntoView(quick.sel)
	}
	list.List(c.Key("quick-results"), list.Props{State: &quick.list, Items: items, Row: row}).
		Height(min(pickerResultsHeight, height)+2*metrics.gap).MinHeight(0).Padding(metrics.gap, metrics.pad)
}

func (a *app) quickListColors(c *ui.Context) (mark, muted ui.Color) {
	pal, surface := a.pal, c.Theme().Surface
	active, hover := pal.active.Over(surface), pal.hover.Over(surface)
	return legible(pal.accent, pal.dark, surface, active, hover), legible(pal.muted, pal.dark, surface, active, hover)
}

func (a *app) quickResultRow(c *ui.Context, index int, label string, height float32, mark ui.Color, choose func(int), content func()) {
	metrics := a.pal.metricsOr()
	row := ui.ButtonBase(c).Height(height).Padding(0, metrics.inset).Gap(metrics.inset).Radius(metrics.radius).
		Justify(ui.Start).AlignItems(ui.Center).Label(label).Tooltip(label)
	a.pickerHighlight(c, row, index == a.quick.sel, mark)
	row.Children(content)
	if row.Clicked() {
		choose(index)
	}
}

func (a *app) pickerHighlight(c *ui.Context, row ui.Element, selected bool, mark ui.Color) {
	metrics := a.pal.metricsOr()
	row.Radius(metrics.radius).Selected(selected)
	if selected {
		row.Background(a.pal.active)
		row.Children(func() {
			ui.Box(c.Key("selection-marker")).Absolute().Left(2).Width(3).Top(metrics.pad).Bottom(metrics.pad).Radius(metrics.radiusFull).Background(mark).PassThrough()
		})
	} else if row.Hovered() {
		row.Background(a.pal.hover)
	}
}

func (a *app) pickerRow(c *ui.Context, row ui.Element, selected, moved bool) {
	mark := a.pal.accent
	if selected {
		mark, _ = a.quickListColors(c)
		if moved {
			row.ScrollIntoView()
		}
	}
	a.pickerHighlight(c, row, selected, mark)
}

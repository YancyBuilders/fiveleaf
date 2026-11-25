package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/smallnest/uix/components/button"
	"github.com/smallnest/uix/components/input"
)

func searchSurface(c *ui.Context, field ui.Element) ui.Element {
	t := c.Theme()
	return field.FillWidth().MinWidth(0).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Transition(fieldFocus)
}

// Return the editable node so each picker's focus, shortcuts and submit
// handling stay attached to the input rather than a surrounding row.
func searchInput(c *ui.Context, value *string, placeholder string, leading *ui.SVG) ui.Element {
	t := c.Theme()
	in := searchSurface(c, input.Input(c, input.Props{Value: value, Placeholder: placeholder})).
		Height(36).Padding(8, 12, 8, 36).FontSize(14)
	in.DrawOver(func(p *ui.Painter, r ui.Rect) {
		color := t.TextMuted
		if in.Focused() {
			color = t.Accent
		}
		p.Icon(leading, ui.Rect{X: r.X + 12, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, color)
	})
	return in
}

func actionButton(c *ui.Context, label string, icon *ui.SVG) ui.Element {
	return compactActionButton(c, label, icon, button.Secondary)
}

func accentActionButton(c *ui.Context, label string, icon *ui.SVG, accent ui.Color) ui.Element {
	base := c.Theme()
	tinted := *base
	tinted.Accent = base.Surface.Mix(accent, 0.12)
	tinted.AccentHover = base.Surface.Mix(accent, 0.19)
	tinted.AccentPressed = base.Surface.Mix(accent, 0.26)
	tinted.AccentText = legible(accent, base.Dark, tinted.Accent, tinted.AccentHover, tinted.AccentPressed)
	c.SetTheme(&tinted)
	defer c.SetTheme(base)
	return compactActionButton(c, label, icon, button.Primary).Border(1, base.Border.Mix(accent, 0.32)).FontWeight(500)
}

func retryButton(c *ui.Context, label string) ui.Element {
	return actionButton(c, label, iconRefresh)
}

func compactActionRow(c *ui.Context, align ui.Align) ui.Element {
	return ui.Row(c).FillWidth().MinWidth(0).Wrap().Justify(align).AlignItems(ui.Center).Gap(c.Theme().Space(1.5))
}

func compactActionButton(c *ui.Context, label string, icon *ui.SVG, variant button.Variant) ui.Element {
	var control ui.Element
	if variant == button.Primary {
		control = button.Button(c, button.Props{Variant: button.Primary, Size: button.Sm})
	} else {
		control = ui.Button(c, "")
	}
	return control.Radius(c.Theme().Radius).Clip().MinHeight(28).Padding(5, 10).Gap(6).FontSize(12).
		MaxWidthPercent(100).Label(label).Cursor(ui.CursorPointer).Transition(hoverFade).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(14).Shrink(0)
		}
		ui.Text(c, label).SingleLine().MinWidth(0)
	})
}

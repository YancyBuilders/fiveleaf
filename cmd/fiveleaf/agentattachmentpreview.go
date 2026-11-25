package main

import (
	"strings"
	"time"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type attachmentAnchor struct {
	chip ui.Element
	ref  *chatDraftRef
}

type attachmentHover struct {
	file          string
	entered, left time.Time
	open, blocked bool
	panel         ui.Rect
}

func (a *app) attachmentPreview(c *ui.Context, state *attachmentHover, active, hovered, focused attachmentAnchor) {
	if a.blurred || a.overlaid() {
		*state = attachmentHover{}
		return
	}
	target := hovered
	if target.ref == nil {
		target = focused
	}
	if state.open {
		x, y, inside := c.Root().PointerPosition()
		if inside && state.panel.Contains(x, y) {
			target = active
		}
	}
	if target.ref != nil && target.ref.file != state.file {
		*state = attachmentHover{file: target.ref.file, entered: c.Now()}
		active = target
	}
	if active.ref == nil || !state.open && target.ref == nil {
		*state = attachmentHover{}
		return
	}
	if !state.open {
		if state.blocked {
			return
		}
		if remaining := hoverDwell - c.Now().Sub(state.entered); remaining > 0 && focused.ref != active.ref {
			c.After(remaining)
			return
		}
		state.open = true
	}
	metrics := a.pal.metricsOr()
	panel := ui.PopoverBase(c.Key(active.ref.file), active.chip, &state.open, func(panel ui.Element) {
		width, height := c.Size()
		width = min(368, width-2*metrics.gap)
		panel.Column().Width(width).KeepFocus().AttachTo(active.chip, ui.AnchorTop, ui.AnchorBottom).Padding(0, 0, metrics.gap, 0)
		card := ui.Column(c).FillWidth().Padding(metrics.inset).Gap(metrics.gap).Radius(metrics.radiusCard).Clip()
		a.floating(c, card, false).Children(func() {
			ref := active.ref
			ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(metrics.gap).Children(func() {
				ui.Text(c, ref.label).SingleLine().MinWidth(0).Grow(1).FontSize(metrics.textBody).FontWeight(metrics.weightBold).TextColor(a.pal.text)
				ui.Text(c, byteSize(ref.bytes)).SingleLine().Shrink(0).FontSize(metrics.textSmall).TextColor(a.pal.muted)
			})
			a.attachmentPreviewBody(c, card, ref, width-2*metrics.inset-2, min(260, height/2))
			ui.Text(c, shortenHome(ref.file)).SingleLine().MinWidth(0).FontSize(metrics.textSmall).TextColor(a.pal.muted)
		})
	})
	if !state.open {
		state.blocked = true
		return
	}
	state.panel = panel.Bounds()
	if target.ref != nil || panel.Hovered() {
		state.left = time.Time{}
		return
	}
	if state.left.IsZero() {
		state.left = c.Now()
	}
	if remaining := hoverLeave - c.Now().Sub(state.left); remaining > 0 {
		c.After(remaining)
	} else {
		*state = attachmentHover{}
		a.invalidate()
	}
}

func (a *app) attachmentPreviewBody(c *ui.Context, card ui.Element, ref *chatDraftRef, width, maxHeight float32) {
	metrics := a.pal.metricsOr()
	if ref.preview != nil {
		var naturalW, naturalH float32
		switch source := ref.preview.(type) {
		case *ui.Bitmap:
			pixelsW, pixelsH := source.Size()
			naturalW, naturalH = float32(pixelsW), float32(pixelsH)
		case *ui.SVG:
			naturalW, naturalH = source.Size()
		}
		height := min(188, maxHeight)
		if naturalW > 0 && naturalH > 1.5*naturalW {
			height = min(naturalH, width*naturalH/naturalW)
		}
		ui.Scroll(c.Key("picture")).FillWidth().Height(min(height, maxHeight)).Children(func() {
			ui.Image(c, ref.preview).FillWidth().Height(height).Shrink(0).Fit(ui.Contain).Radius(metrics.radius)
		})
		return
	}
	if ref.previewText == "" {
		ui.Text(c, locales.PreviewEmpty()).FontSize(metrics.textSmall).TextColor(a.pal.muted)
		return
	}
	text := ui.Local(card, "text", func() attachmentTextView { return attachmentTextView{} })
	if text.source != ref.previewText {
		*text = attachmentTextView{source: ref.previewText, chunks: previewTextChunks(ref.previewText)}
	}
	draw := func(chunk string) {
		ui.Text(c, chunk).FillWidth().MinWidth(0).Shrink(0).Font(a.settings.FontFamily).FontSize(metrics.textSmall).TextColor(a.pal.text)
	}
	if len(text.chunks) == 1 {
		ui.Scroll(c.Key("excerpt")).FillWidth().MaxHeight(maxHeight).Children(func() { draw(text.chunks[0]) })
		return
	}
	ui.List(c.Key("lines"), &text.list, len(text.chunks), func(index int) { draw(text.chunks[index]) }).FillWidth().Height(maxHeight)
}

type attachmentTextView struct {
	source string
	chunks []string
	list   ui.ListState
}

func previewTextChunks(text string) []string {
	var chunks []string
	for text != "" {
		chunk, clipped := agentVisibleText(text, 1536)
		end := 0
		for line := range 8 {
			at := strings.IndexByte(chunk[end:], '\n')
			if at < 0 {
				if clipped && end > 0 {
					chunk = chunk[:end]
				}
				break
			}
			end += at + 1
			if line == 7 {
				chunk = chunk[:end]
			}
		}
		chunks = append(chunks, strings.TrimSuffix(chunk, "\n"))
		text = text[len(chunk):]
	}
	return chunks
}

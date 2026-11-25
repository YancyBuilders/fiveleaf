package main

import (
	"bytes"
	"cmp"
	"errors"
	"image"
	"net/url"
	"path/filepath"
	"strings"

	"fiveleaf/internal/bg"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type roundtableMediaPreview struct {
	view       *viewer
	label      string
	generation uint64
	focused    bool
}

type roundtableMediaLoad struct {
	started bool
	view    *viewer
	err     error
}

var roundtableMediaReads = make(chan struct{}, 2)

func roundtableHasMedia(text string) bool {
	if strings.Contains(text, "![") {
		return true
	}
	hasFence := strings.Contains(text, "```") || strings.Contains(text, "~~~")
	return hasFence && strings.Contains(strings.ToLower(text), "mermaid")
}

func (w *roundtableWindow) messageMarkdown(c *ui.Context, key any, text string) {
	if !roundtableHasMedia(text) {
		agentMarkdown(c, key, text, w.pal, w.a.settings, false)
		return
	}
	agentMarkdown(c, key, text, w.pal, w.a.settings, false, func(options *markdown.Options) {
		options.Block = w.mediaImages
		fallback := options.CodeBlock
		options.CodeBlock = func(c *ui.Context, block *markdown.CodeBlockView) {
			if !w.a.settings.PreviewMermaid || !strings.EqualFold(block.Language, "mermaid") {
				fallback(c, block)
				return
			}
			if !w.mediaDiagram(c, block.Code()) {
				fallback(c, block)
			}
		}
	})
}

func (w *roundtableWindow) mediaImages(c *ui.Context, doc *parser.Document, index int) bool {
	block := doc.Block(index)
	if block.Kind == parser.Table {
		for child := index + 1; child < block.Next; child++ {
			if doc.Block(child).Kind == parser.TableCell {
				w.mediaInlines(c, doc.Inlines(child))
			}
		}
		return false
	}
	if block.Kind != parser.Paragraph && block.Kind != parser.Heading {
		return false
	}
	imagesOnly := w.mediaInlines(c, doc.Inlines(index))
	return imagesOnly && block.Kind == parser.Paragraph
}

func (w *roundtableWindow) mediaInlines(c *ui.Context, inlines []parser.Inline) bool {
	images, prose := false, false
	for index := 0; index < len(inlines); index++ {
		inline := inlines[index]
		switch inline.Kind {
		case parser.Image:
			label := parser.PlainText(inlines[index+1 : inline.Next])
			w.mediaImage(c, inline.Start, inline.Dest, label)
			images = true
			index = inline.Next - 1
		case parser.SoftBreak, parser.LineBreak:
		case parser.Text:
			prose = prose || strings.TrimSpace(inline.Text) != ""
		default:
			prose = true
		}
	}
	return images && !prose
}

func (w *roundtableWindow) mediaLoad(state *roundtableMediaLoad, load func() (*viewer, error)) {
	if state.started {
		return
	}
	state.started = true
	generation := w.viewGeneration
	bg.Go(func() {
		defer recoverCrash()
		roundtableMediaReads <- struct{}{}
		defer func() { <-roundtableMediaReads }()
		view, err := load()
		w.postWindow(func() {
			if generation == w.viewGeneration {
				state.view, state.err = view, err
			}
		})
	})
}

func (w *roundtableWindow) mediaImage(c *ui.Context, offset int, dest, alt string) {
	var root string
	if w.meeting != nil {
		root = w.meeting.library.Project
	}
	key := struct {
		offset     int
		root, dest string
	}{offset, root, dest}
	box := ui.Column(c.Key(key)).FillWidth().MinWidth(0)
	state := ui.Local(box, "load", func() roundtableMediaLoad { return roundtableMediaLoad{} })
	address, parseErr := url.Parse(dest)
	remote := parseErr == nil && (address.Scheme == "http" || address.Scheme == "https") && address.Hostname() != "" && address.User == nil
	name := cmp.Or(alt, dest, locales.MarkdownImageAlt())
	load := func() (*viewer, error) { return roundtableLoadImage(root, dest) }
	box.Children(func() {
		if remote && !state.started {
			title := locales.DownloadTitle(name + " · " + address.Host)
			if ui.Button(c.Key("load-image"), title).Tooltip(dest).MaxWidthPercent(100).AlignSelf(ui.Start).Clicked() {
				w.mediaLoad(state, load)
			}
			return
		}
		w.mediaLoad(state, load)
		switch {
		case state.err != nil:
			ui.Text(c, locales.CannotShow(name)).Selectable().FontSize(12).TextColor(w.pal.muted).Tooltip(dest)
		case state.view == nil:
			ui.Text(c, name+" · "+locales.Loading()).FontSize(12).TextColor(w.pal.muted)
		default:
			w.mediaThumbnail(c, state.view, name, locales.PreviewLabel(name))
		}
	})
}

func roundtableLoadImage(root, dest string) (*viewer, error) {
	address, err := url.Parse(dest)
	if err != nil || address.User != nil {
		return nil, errors.New(locales.ErrNotImage())
	}
	var data []byte
	switch address.Scheme {
	case "https", "http":
		if address.Hostname() == "" {
			return nil, errors.New(locales.ErrNotImage())
		}
		data, err = markdownHTTPGet(dest)
	case "", "file":
		localHost := address.Host == "" || address.Scheme == "file" && strings.EqualFold(address.Host, "localhost")
		if root == "" || !localHost || address.Path == "" {
			return nil, errors.New(locales.ErrNotImage())
		}
		path := filepath.FromSlash(address.Path)
		if address.Scheme == "" {
			if filepath.IsAbs(path) {
				if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
					if relative, relErr := filepath.Rel(root, resolved); relErr == nil && filepath.IsLocal(relative) {
						path = relative
					}
				}
			}
			path = filepath.Join(root, path)
		}
		if !filepath.IsAbs(path) {
			return nil, errors.New(locales.ErrNotImage())
		}
		canonical, pathErr := filepath.EvalSymlinks(path)
		if pathErr != nil {
			return nil, pathErr
		}
		data, _, err = readAgentFileData(root, canonical, maxOpenBytes)
	default:
		return nil, errors.New(locales.ErrNotImage())
	}
	if err != nil {
		return nil, err
	}
	svg := strings.EqualFold(filepath.Ext(address.Path), ".svg") || bytes.Contains(data[:min(len(data), 1024)], []byte("<svg"))
	if !svg {
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		const maxPixels = 32 << 20
		if config.Height <= 0 || config.Width <= 0 || config.Width > maxPixels/config.Height {
			return nil, errors.New(locales.ErrImageTooLarge())
		}
	}
	picture := markdownDecode(data, svg)
	if picture.bitmap == nil {
		return nil, errors.New(locales.ErrNotImage())
	}
	return &viewer{loaded: true, src: picture.bitmap, w: picture.w, h: picture.h}, nil
}

func (w *roundtableWindow) mediaDiagram(c *ui.Context, source string) bool {
	style := w.a.mermaidStyle()
	key := struct {
		source string
		style  mermaidStyle
	}{source, style}
	box := ui.Column(c.Key(key)).FillWidth().MinWidth(0)
	state := ui.Local(box, "load", func() roundtableMediaLoad { return roundtableMediaLoad{} })
	w.mediaLoad(state, func() (*viewer, error) {
		diagram := renderMermaid(source, style)
		if diagram.err != "" {
			return nil, errors.New(diagram.err)
		}
		return &viewer{loaded: true, src: diagram.svg, w: diagram.w, h: diagram.h, diagram: diagram, style: style}, nil
	})
	box.Children(func() {
		switch {
		case state.err != nil:
			ui.Text(c, locales.MermaidCannotDraw(state.err.Error())).Selectable().FontSize(12).TextColor(w.pal.muted)
		case state.view != nil:
			w.mediaThumbnail(c, state.view, "Mermaid", locales.OpenDiagram())
		default:
			ui.Text(c, locales.Loading()).FontSize(12).TextColor(w.pal.muted)
		}
	})
	return state.err == nil
}

func (w *roundtableWindow) mediaThumbnail(c *ui.Context, view *viewer, title, action string) {
	scale := min(1, 460/view.w, 210/view.h)
	button := ui.ButtonBase(c.Key("preview")).AlignSelf(ui.Start).Width(max(40, view.w*scale)).MaxWidthPercent(100).
		Height(max(32, view.h*scale)).Radius(6).Clip().Background(w.pal.panel).Cursor(ui.CursorPointer).Label(action).Tooltip(action)
	button.Draw(func(painter *ui.Painter, area ui.Rect) {
		scale := min(area.W/view.w, area.H/view.h)
		width, height := view.w*scale, view.h*scale
		roundtableDrawMedia(painter, view, ui.Rect{X: area.X + (area.W-width)/2, Y: area.Y + (area.H-height)/2, W: width, H: height})
	})
	if button.Clicked() {
		full := &viewer{loaded: true, src: view.src, w: view.w, h: view.h}
		if view.diagram != nil {
			overlay := diagramOverlay{view: full}
			overlay.show(view.diagram, view.style)
		}
		w.media = &roundtableMediaPreview{view: full, label: title, generation: w.viewGeneration}
	}
}

func roundtableDrawMedia(painter *ui.Painter, view *viewer, area ui.Rect) {
	if view.diagram != nil {
		painter.Fill(area, view.style.bg, 0)
		view.diagram.draw(painter, area)
		return
	}
	painter.Image(view.src, area, ui.FillBox)
}

func (w *roundtableWindow) mediaOverlay(c *ui.Context) {
	preview := w.media
	if preview == nil {
		return
	}
	if preview.generation != w.viewGeneration {
		w.media = nil
		return
	}
	p, view := w.pal, preview.view
	ui.Overlay(c, func() {
		back := w.a.dialogBack(c)
		back.Children(func() {
			box := ui.Column(c.Key("roundtable-media-overlay")).Grow(1).AlignSelf(ui.Stretch).Margin(20).Radius(10).Clip().
				Background(p.editor).Border(1, p.border).Role(ui.RoleDialog).Label(locales.PreviewLabel(preview.label))
			box.Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(3).Padding(6, 8, 6, 12).Children(func() {
					ui.Text(c, preview.label).Grow(1).MinWidth(0).SingleLine().FontSize(13).TextColor(p.text)
					if roundtableIconButton(c, "zoom-out", iconMinus, locales.CmdImageZoomOut(), p).Clicked() {
						view.zoomBy(-1)
					}
					fit := ui.ButtonBase(c.Key("fit")).Height(28).MinWidth(52).AlignItems(ui.Center).Justify(ui.Center).Label(locales.FitToView()).Tooltip(locales.FitToView())
					fit.Children(func() { ui.Text(c, view.zoomPercent()).FontSize(12).TextColor(p.muted).SingleLine() })
					if fit.Clicked() {
						view.zoomBy(0)
					}
					if roundtableIconButton(c, "zoom-in", iconPlus, locales.CmdImageZoomIn(), p).Clicked() {
						view.zoomBy(1)
					}
					if roundtableIconButton(c, "close", iconX, locales.CloseEsc(), p).Clicked() {
						w.media = nil
					}
				})
				pane := ui.Box(c.Key("canvas")).Bind(&view.pane).Grow(1).MinHeight(0).FillWidth().Clip().Focusable().FocusRing(false).Label(locales.ImageLabel(preview.label))
				bounds := pane.Bounds()
				view.paneW, view.paneH = bounds.W, bounds.H
				pane.HandleInput(func(event ui.InputEvent) bool { return w.a.viewerInput(view, event) })
				if view.grab {
					pane.Cursor(ui.CursorGrabbing)
				} else if view.zoom > 0 {
					pane.Cursor(ui.CursorGrab)
				}
				pane.Draw(func(painter *ui.Painter, area ui.Rect) {
					at := view.place(area.W, area.H)
					at.X, at.Y = area.X+at.X, area.Y+at.Y
					roundtableDrawMedia(painter, view, at)
				})
				if !preview.focused {
					preview.focused = true
					view.pane.Focus()
				}
			})
			if box.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
				w.media = nil
			}
		})
	})
}

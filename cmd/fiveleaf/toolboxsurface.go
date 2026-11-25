package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	mtransfer "github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	xhtml "golang.org/x/net/html"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

const toolboxFrameDomain = ".tab.localhost"
const toolboxBridgePath = "/.fiveleaf/toolbox-bridge.js"

//go:embed toolboxweb/tab-host.js
var toolboxHostScript []byte

//go:embed toolboxweb/tab-bridge.js
var toolboxPageScript []byte

type toolboxSurface struct {
	view     *mygo.WebView
	pal      *palette
	zoom     float32
	err      error
	overlaid bool
	yielded  time.Time
}

func (tab *toolboxTab) release() {
	if view := tab.surface.view; view != nil && !view.IsDestroyed() {
		view.Destroy()
	}
	tab.surface = toolboxSurface{}
}

func (tab *toolboxTab) draw(c *ui.Context, owner *app) {
	desktop, ok := owner.host.(*desktop)
	if !ok || desktop.f == nil || desktop.f.win == nil {
		return
	}
	surface := &tab.surface
	win := desktop.f.win
	if surface.view != nil && (surface.view.Window() != win || surface.view.IsDestroyed()) {
		tab.release()
	}
	if surface.view == nil && surface.err == nil {
		surface.pal, surface.zoom = owner.pal, owner.settings.ZoomLevel
		surface.view, surface.err = win.NewWebView(mygo.WebViewOptions{
			Page: mygo.PageOptions{ZoomFactor: math.Pow(1.2, float64(surface.zoom))}, BackgroundColor: hexColor(owner.pal.editor),
		})
		if surface.err == nil {
			view := surface.view
			page := view.Page()
			frameURL := toolboxFrameURL(tab.provider.Name)
			page.OnWillNavigate(func(event *mygo.NavigateEvent) {
				if event.URL != frameURL {
					event.PreventDefault()
				}
			})
			page.SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
			page.OnDidFailLoad(func(failure *mygo.LoadError) {
				if surface.view == view && failure.URL == frameURL {
					surface.err = errors.New(failure.Description)
					owner.invalidate()
				}
			})
			surface.err = page.LoadURL(frameURL)
		}
	}
	if surface.err != nil {
		ui.Column(c).Fill().Padding(12, 16).Gap(8).Children(func() {
			ui.Text(c, locales.PluginWebviewFailed(surface.err.Error())).TextColor(owner.pal.dangerFg)
			if retryButton(c, locales.PluginsIndexRetry()).Clicked() {
				tab.release()
				owner.invalidate()
			}
		})
		return
	}
	page := surface.view.Page()
	if surface.pal != owner.pal {
		surface.pal = owner.pal
		surface.view.SetBackgroundColor(hexColor(owner.pal.editor))
		toolboxThemeChanged.EmitPage(page, webToolboxTheme(owner.pal))
	}
	if surface.zoom != owner.settings.ZoomLevel {
		surface.zoom = owner.settings.ZoomLevel
		page.SetZoomFactor(math.Pow(1.2, float64(surface.zoom)))
	}
	if overlaid := owner.overlaid(); overlaid != surface.overlaid {
		surface.overlaid = overlaid
		if overlaid {
			surface.yielded = c.Now()
			owner.invalidate()
		}
	}
	if surface.overlaid && surface.yielded.Equal(c.Now()) {
		ui.Box(c).Fill().Background(owner.pal.editor)
		return
	}
	ui.WebView(c.Key("toolbox-page:"+tab.provider.Name), surface.view).Fill().MinWidth(0).MinHeight(0)
}

func toolboxFrameURL(name string) string {
	return (&url.URL{Scheme: toolboxScheme, Host: name + toolboxFrameDomain, Path: "/"}).String()
}

func toolboxFrameSource(raw string, platform string) (source, origin string) {
	page, _ := url.Parse(raw)
	if platform == "windows" {
		page.Host += "." + page.Scheme + ".localhost"
		page.Scheme = "http"
	}
	return page.String(), page.Scheme + "://" + page.Host
}

func toolboxSurfaceFiles(assets http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		name, frame := strings.CutSuffix(request.Host, toolboxFrameDomain)
		if !frame && request.URL.Path != toolboxBridgePath {
			assets.ServeHTTP(writer, request)
			return
		}
		if !frame {
			var ok bool
			name, ok = strings.CutSuffix(request.Host, ".localhost")
			if !ok {
				http.NotFound(writer, request)
				return
			}
		}
		provider := plugins.toolboxProvider(name)
		if provider == nil {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		switch {
		case frame && request.URL.Path == "/":
			source, origin := toolboxFrameSource(toolboxURL(provider), runtime.GOOS)
			writer.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; frame-src "+origin+"; frame-ancestors 'none'; base-uri 'none'")
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(writer, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>html,body,iframe{border:0;margin:0;width:100%%;height:100%%;display:block;overflow:hidden}</style><script src="/tab-host.js"></script></head><body><iframe title="%s" sandbox="allow-scripts allow-same-origin allow-forms allow-downloads" src="%s"></iframe></body></html>`, html.EscapeString(toolboxTitle(provider)), html.EscapeString(source))
		case frame && request.URL.Path == "/tab-host.js":
			writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			writer.Write(toolboxHostScript)
		case !frame:
			writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			writer.Write(toolboxPageScript)
		default:
			http.NotFound(writer, request)
		}
	})
}

func serveToolboxAsset(writer http.ResponseWriter, request *http.Request, name string, file io.ReadSeeker) {
	if strings.EqualFold(path.Ext(name), ".html") || strings.EqualFold(path.Ext(name), ".htm") {
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		start := toolboxBridgeOffset(data)
		var page bytes.Buffer
		page.Grow(len(data) + len(toolboxBridgePath) + 24)
		page.Write(data[:start])
		fmt.Fprintf(&page, `<script src="%s"></script>`, toolboxBridgePath)
		page.Write(data[start:])
		file = bytes.NewReader(page.Bytes())
	}
	http.ServeContent(writer, request, name, time.Time{}, file)
}

func toolboxBridgeOffset(page []byte) int {
	start := len(page) - len(bytes.TrimPrefix(page, []byte{0xef, 0xbb, 0xbf}))
	tokens := xhtml.NewTokenizer(bytes.NewReader(page[start:]))
	for {
		switch tokens.Next() {
		case xhtml.CommentToken, xhtml.DoctypeToken:
		case xhtml.TextToken:
			if len(bytes.TrimSpace(tokens.Raw())) != 0 {
				return start
			}
		case xhtml.StartTagToken:
			name, _ := tokens.TagName()
			if string(name) != "html" && string(name) != "head" {
				return start
			}
		default:
			return start
		}
		start += len(tokens.Raw())
	}
}

type toolboxService struct{ app *fiveleaf }

func (service toolboxService) withTab(ctx context.Context, action func(*app, *toolboxTab) error) error {
	err := errors.New("toolbox tab is unavailable")
	mygo.RunOnMain(func() {
		page := mygo.CallerPage(ctx)
		if ctx.Err() != nil || page == nil {
			return
		}
		for _, desktop := range service.app.wins {
			if desktop.f == nil || desktop.f.active != desktop || desktop.f.win != mygo.CallerWindow(ctx) {
				continue
			}
			for _, col := range desktop.a.cols {
				entry := col.tab()
				if entry == nil || entry.tool == nil || entry.tool.toolbox == nil {
					continue
				}
				tab := entry.tool.toolbox
				if view := tab.surface.view; view != nil && !view.IsDestroyed() && view.Page() == page && plugins.toolboxProvider(tab.provider.Name) == tab.provider {
					err = action(desktop.a, tab)
					return
				}
			}
		}
	})
	return err
}

func (service toolboxService) Copy(ctx context.Context, text string) error {
	return service.withTab(ctx, func(*app, *toolboxTab) error { return mygo.Clipboard.Write(mtransfer.TextData(text)) })
}

func (service toolboxService) Theme(ctx context.Context) (toolboxTheme, error) {
	var palette toolboxTheme
	err := service.withTab(ctx, func(owner *app, _ *toolboxTab) error { palette = webToolboxTheme(owner.pal); return nil })
	return palette, err
}

func (service toolboxService) SetDetail(ctx context.Context, detail bool) error {
	if !detail {
		return service.OpenTools(ctx)
	}
	return service.withTab(ctx, func(*app, *toolboxTab) error { return nil })
}

func (service toolboxService) ManagePlugins(ctx context.Context) error {
	return service.withTab(ctx, func(owner *app, _ *toolboxTab) error {
		owner.host.showSettings(settings.CatPlugins, toolboxCategoryKey)
		return nil
	})
}

func (service toolboxService) OpenTools(ctx context.Context) error {
	return service.withTab(ctx, func(owner *app, tab *toolboxTab) error {
		owner.focusToolboxTab(tab)
		owner.openToolboxPicker()
		return nil
	})
}

func (service toolboxService) Focus(ctx context.Context) error {
	return service.withTab(ctx, func(owner *app, tab *toolboxTab) error { owner.focusToolboxTab(tab); return nil })
}

func (service toolboxService) Close(ctx context.Context) error {
	return service.withTab(ctx, func(owner *app, tab *toolboxTab) error { owner.closeToolboxTab(tab); return nil })
}

package main

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"fiveleaf/internal/highlight"
	"fiveleaf/locales"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark/ast"
)

func liveFenceTokens(doc *liveDoc, block *ast.FencedCodeBlock, body []byte, offset int, runeAt func(int) int, previous, used map[[2]string][]highlight.Line) {
	language := markdownFenceLanguage(block, body)
	segments := block.Lines()
	if language == "" || segments.Len() == 0 {
		return
	}
	key := [2]string{language, string(segments.Value(body))}
	lines, found := used[key]
	if !found {
		lines, found = previous[key]
		if !found {
			if lexer := lexers.Get(language); lexer != nil {
				lines = highlight.Lex(lexer, key[1])
			}
		}
		used[key] = lines
	}
	for index := range min(segments.Len(), len(lines)) {
		segment := segments.At(index)
		start, end := runeAt(offset+segment.Start), runeAt(offset+segment.Stop)
		for _, token := range lines[index].Tokens {
			from := max(start, start-segment.Padding+token.Start)
			to := min(end, start-segment.Padding+token.End)
			if to > from {
				doc.spans = append(doc.spans, liveSpan{kind: liveStyle, start: from, end: to, from: from, to: to, look: liveLook{syntax: token.Kind}})
			}
		}
	}
}

type mdFenceLanguage struct {
	start, end int
	name       string
}

func fenceLanguageName(info string) string {
	if space := strings.IndexAny(info, " \t"); space >= 0 {
		return info[:space]
	}
	return info
}

func markdownFenceLanguage(block *ast.FencedCodeBlock, source []byte) string {
	if block.Info == nil {
		return ""
	}
	return fenceLanguageName(string(block.Info.Segment.Value(source)))
}

func fenceLanguageAt(snap ui.TextSnapshot, marker int) (mdFenceLanguage, bool) {
	if marker < 0 || marker >= snap.Len() {
		return mdFenceLanguage{}, false
	}
	_, end := snap.LineRange(snap.LineAt(marker))
	line := snap.Slice(marker, end)
	if len(line) < 3 || line[0] != '`' && line[0] != '~' {
		return mdFenceLanguage{}, false
	}
	fence := len(line) - len(strings.TrimLeft(line, line[:1]))
	if fence < 3 {
		return mdFenceLanguage{}, false
	}
	rest := strings.TrimLeft(line[fence:], " \t")
	name := fenceLanguageName(rest)
	start := marker + utf8.RuneCountInString(line[:len(line)-len(rest)])
	return mdFenceLanguage{start: start, end: start + utf8.RuneCountInString(name), name: name}, true
}

var commonFenceLanguages = []struct{ label, id string }{
	{"Go", "go"}, {"Python", "python"}, {"JavaScript", "javascript"}, {"TypeScript", "typescript"},
	{"Shell", "bash"}, {"JSON", "json"}, {"YAML", "yaml"}, {"SQL", "sql"}, {"HTML", "html"}, {"CSS", "css"},
	{"C", "c"}, {"C++", "cpp"}, {"Java", "java"}, {"Rust", "rust"}, {"Swift", "swift"}, {"Mermaid", "mermaid"},
}

func (a *app) liveFenceLanguageMenu(c *ui.Context, d *editor, snap ui.TextSnapshot, code liveDeco, top, right float32) {
	lang, ok := fenceLanguageAt(snap, code.start+code.fenceOffset)
	if !ok {
		return
	}
	caption := lang.name
	if caption == "" || strings.EqualFold(caption, "text") {
		caption = locales.MarkdownFencePlain()
	}
	for _, entry := range commonFenceLanguages {
		if entry.id == strings.ToLower(lang.name) {
			caption = entry.label
			break
		}
	}
	size := max(9, a.settings.FontSize*0.8)
	width, _ := c.MeasureText(0, ui.Span{Text: caption, Size: size})
	width = min(width+22, max(32, right-d.textX))
	label := locales.MarkdownFenceLanguage(caption)
	button := ui.ButtonBase(c.Key("fence-language:" + strconv.Itoa(code.start) + ":" + strconv.FormatUint(snap.Version(), 10))).Absolute().Top(top).Left(right - width).Width(width).
		Height(max(16, a.font.lineH)).Row().Gap(3).AlignItems(ui.Center).Justify(ui.Center).KeepFocus().Label(label).Tooltip(label).Disabled(d.locked())
	button.Children(func() {
		ui.Text(c, caption).SingleLine().FontSize(size).TextColor(a.pal.muted)
		ui.Icon(c, iconChevronDown).FontSize(10).TextColor(a.pal.muted)
	})
	version := snap.Version()
	apply := func(name string) {
		for _, col := range a.cols {
			if tab := col.tab(); tab != nil && tab.ed == d && d.buf.Version() == version && !d.locked() && a.livePreview(d) {
				a.activate(col, col.active, true)
				if name != lang.name {
					_, rowEnd := snap.LineRange(snap.LineAt(lang.start))
					if name == "" && strings.TrimSpace(snap.Slice(lang.end, rowEnd)) != "" {
						name = "text"
					}
					start, end := d.sel[0], d.sel[1]
					length := utf8.RuneCountInString(name)
					d.edit([2]int{movedBy(start, lang.start, lang.end, length), movedBy(end, lang.start, lang.end, length)}, textEdit{lang.start, lang.end, name})
				}
				a.invalidate()
				return
			}
		}
	}
	button.Menu(func(menu *ui.Menu) {
		if menu.Item(locales.MarkdownFencePlain()).Checked(lang.name == "" || strings.EqualFold(lang.name, "text")).Chosen() {
			a.post(func() { apply("") })
		}
		for _, entry := range commonFenceLanguages {
			if menu.Item(entry.label).Checked(strings.EqualFold(lang.name, entry.id)).Chosen() {
				a.post(func() { apply(entry.id) })
			}
		}
		menu.Separator()
		if menu.Item(locales.MarkdownFenceOther()).Chosen() {
			a.post(func() {
				a.prompt = &prompt{title: locales.MarkdownFenceOther(), value: lang.name, done: func(value string) string {
					name := strings.TrimSpace(value)
					if strings.IndexFunc(name, unicode.IsSpace) >= 0 || lexers.Get(name) == nil && !strings.EqualFold(name, "mermaid") {
						return locales.MarkdownFenceUnknown()
					}
					apply(name)
					return ""
				}}
			})
		}
	})
}

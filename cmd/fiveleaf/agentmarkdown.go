package main

import (
	"fiveleaf/internal/highlight"
	"fiveleaf/internal/settings"
	"github.com/egoist/mygo/ui"
	mdeditor "github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

type agentMarkdownKey struct{}

type agentMarkdownState struct {
	options markdown.Options
	theme   markdown.Theme
	code    *mdeditor.Theme
}

func sharedAgentMarkdown(c *ui.Context, key any, node *viewNode, p *palette, s *settings.Settings, streaming bool, configure ...func(*markdown.Options)) {
	node.parsed = true
	agentMarkdown(c, key, node.Text, p, s, streaming, configure...)
}

func agentThoughtMarkdown(p *palette) func(*markdown.Options) {
	return func(options *markdown.Options) {
		if options.Theme == nil {
			return
		}
		theme := *options.Theme
		theme.FontSize = max(11, theme.FontSize-1)
		theme.Text, theme.Strong = p.muted, p.muted
		for index := range theme.Headings {
			theme.Headings[index].Color = p.muted
		}
		options.Theme = &theme
	}
}

// Keep the parser and highlighted code with the message's UI identity. A
// streaming update replaces its viewNode, but should only reparse the new tail.
func agentMarkdown(c *ui.Context, key any, text string, p *palette, s *settings.Settings, streaming bool, configure ...func(*markdown.Options)) {
	root := ui.Column(c.Key(key)).FillWidth().MinWidth(0)
	state := ui.Local(root, agentMarkdownKey{}, func() agentMarkdownState {
		return agentMarkdownState{options: markdown.Options{
			Reveal: &markdown.Reveal{Unit: markdown.AtOnce, Fade: -1},
		}}
	})
	code := agentMarkdownCodeTheme(p)
	// The Markdown renderer uses the theme's identity to invalidate cached
	// spans. Preserve it across chunks, and replace it when colors change.
	if state.code == nil || *state.code != code {
		state.code = &code
	}
	state.theme = agentMarkdownTheme(p, s, state.code)
	state.options.Theme = &state.theme
	state.options.Block = nil
	state.options.CodeBlock = func(c *ui.Context, block *markdown.CodeBlockView) {
		// An EOF fence is valid finished Markdown. The parser still marks
		// it open, but copying should become available when the reply ends.
		view := *block
		view.Open = streaming && block.Open
		markdown.DefaultCodeBlock(c, &view)
	}
	for _, apply := range configure {
		apply(&state.options)
	}
	root.Children(func() {
		if streaming {
			markdown.Stream(c, text, &state.options).FillWidth().MinWidth(0)
		} else {
			markdown.View(c, text, &state.options).FillWidth().MinWidth(0)
		}
	})
}

func agentMarkdownTheme(p *palette, s *settings.Settings, code *mdeditor.Theme) markdown.Theme {
	theme := markdown.Theme{
		FontSize:            s.FontSize,
		CodeFont:            s.FontFamily,
		LineHeight:          max(s.LineHeight, 1.45),
		CodeLineHeight:      max(s.LineHeight, 1.4),
		Text:                p.text,
		Muted:               p.muted,
		Link:                p.accent,
		Strong:              p.text,
		StrongWeight:        600,
		Code:                p.text,
		CodeBackground:      p.surfaceHover,
		CodeSize:            0.94,
		BlockSpacing:        0.65,
		ItemSpacing:         0.2,
		ListIndent:          1.6,
		QuoteBar:            p.accentDim,
		QuoteText:           p.muted,
		Rule:                p.border,
		CodeBlockBackground: p.panel,
		CodeBlockBorder:     p.border,
		CodeBlockRadius:     6,
		CodeBlockPadding:    0.8,
		CodeColors:          code,
		TableBorder:         p.border,
		TableHeader:         p.surfaceHover,
		TableStripe:         p.hover,
		TableCellPadding:    [2]float32{0.65, 0.4},
		Mark:                p.modified.Alpha(0.22),
		Checkbox:            p.accent,
	}
	if p.mono {
		theme.Font = s.FontFamily
	}
	for i, size := range [...]float32{1.5, 1.3, 1.15, 1.05, 1, 1} {
		theme.Headings[i] = markdown.HeadingStyle{
			Size: size, Weight: 600, Color: p.text, Top: 1.1, Bottom: 0.45,
		}
	}
	return theme
}

var agentMarkdownTokenKinds = [...]highlight.Kind{
	mdeditor.TokenText:        highlight.Plain,
	mdeditor.TokenKeyword:     highlight.Keyword,
	mdeditor.TokenType:        highlight.Type,
	mdeditor.TokenFunction:    highlight.Function,
	mdeditor.TokenVariable:    highlight.Variable,
	mdeditor.TokenProperty:    highlight.Property,
	mdeditor.TokenConstant:    highlight.Constant,
	mdeditor.TokenNumber:      highlight.Number,
	mdeditor.TokenString:      highlight.String,
	mdeditor.TokenEscape:      highlight.Interpolation,
	mdeditor.TokenRegexp:      highlight.String,
	mdeditor.TokenComment:     highlight.Comment,
	mdeditor.TokenOperator:    highlight.Operator,
	mdeditor.TokenPunctuation: highlight.Punctuation,
	mdeditor.TokenTag:         highlight.Tag,
	mdeditor.TokenAttribute:   highlight.Attribute,
	mdeditor.TokenMeta:        highlight.Builtin,
	mdeditor.TokenHeading:     highlight.Heading,
	mdeditor.TokenEmphasis:    highlight.Emph,
	mdeditor.TokenStrong:      highlight.Strong,
	mdeditor.TokenLink:        highlight.String,
	mdeditor.TokenCode:        highlight.String,
	mdeditor.TokenInserted:    highlight.Inserted,
	mdeditor.TokenDeleted:     highlight.Deleted,
	mdeditor.TokenInvalid:     highlight.Plain,
}

func agentMarkdownCodeTheme(p *palette) mdeditor.Theme {
	theme := mdeditor.Theme{Text: p.text}
	for token, kind := range agentMarkdownTokenKinds {
		color := p.syntax[kind]
		if color == (ui.Color{}) {
			color = p.text
		}
		theme.Tokens[token] = mdeditor.TokenStyle{Color: color, Italic: p.syntaxItalic[kind]}
	}
	theme.Tokens[mdeditor.TokenEmphasis].Italic = true
	theme.Tokens[mdeditor.TokenStrong].Bold = true
	theme.Tokens[mdeditor.TokenHeading].Bold = true
	theme.Tokens[mdeditor.TokenLink].Color = p.accent
	theme.Tokens[mdeditor.TokenInvalid].Color = p.dangerFg
	return theme
}

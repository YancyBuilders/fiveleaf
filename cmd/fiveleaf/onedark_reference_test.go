package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
)

func referenceSyntaxRanges(t *testing.T, source string, p *palette) []ui.TextRange {
	t.Helper()
	return referenceSyntaxRangesForFile(t, "reference.ts", source, p)
}

func referenceSyntaxRangesForFile(t *testing.T, name, source string, p *palette) []ui.TextRange {
	t.Helper()
	snap := ui.NewTextBuffer(source).Snapshot()
	ready := make(chan struct{}, 1)
	hl := highlight.New(highlight.Lexer(name), func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	defer hl.Close()
	hl.Request(snap, 0, snap.LineCount()-1)
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("syntax highlighting did not finish")
	}
	var cache colorCache
	return cache.ranges(hl, p, snap, 0, snap.LineCount()-1, 700)
}

const oneDarkGoReference = `package main

import "sync"

func GoAndWaitWithAnts(concurrencyNum int, handlers ...func() error) error {
	var (
		wg   sync.WaitGroup
		once sync.Once
		err  error
	)
	pool, _ := ants.NewPool(concurrencyNum)
	defer pool.Release()

	for _, f := range handlers {
		wg.Add(1)
		handler := f // 避免闭包中的 f 被重复使用
		pool.Submit(func() {
			defer func() {
				if e := recover(); e != nil {
					buf := make([]byte, 1024)
					buf = buf[:runtime.Stack(buf, false)]
					once.Do(func() {
						err = errs.New("panic found in call handlers")
					})
				}
			}()
			handler()
			wg.Done()
		})
	}
	wg.Wait()
	return err
}
`

func TestOneDarkProGoReferenceSyntax(t *testing.T) {
	withAppearancePlugins(t)
	ranges := referenceSyntaxRangesForFile(t, "reference.go", oneDarkGoReference, oneDarkProPal)
	for _, tc := range []struct {
		fragment, word, color string
		italic                bool
	}{
		{"func GoAndWaitWithAnts", "func", "#c678dd", false},
		{"func GoAndWaitWithAnts", "GoAndWaitWithAnts", "#61afef", false},
		{"concurrencyNum int", "concurrencyNum", "#e06c75", true},
		{"concurrencyNum int", "int", "#c678dd", false},
		{"handlers ...func", "handlers", "#e06c75", true},
		{"func() error", "error", "#c678dd", false},
		{"wg   sync.WaitGroup", "wg", "#e06c75", false},
		{"wg   sync.WaitGroup", "sync", "#e5c07b", false},
		{"wg   sync.WaitGroup", "WaitGroup", "#e5c07b", false},
		{"once sync.Once", "Once", "#e5c07b", false},
		{"pool, _ :=", "pool", "#e06c75", false},
		{"pool, _ :=", ":=", "#e5c07b", false},
		{"ants.NewPool", "ants", "#e06c75", false},
		{"ants.NewPool", "NewPool", "#61afef", false},
		{"NewPool(concurrencyNum)", "concurrencyNum", "#e06c75", false},
		{"pool.Release", "Release", "#61afef", false},
		{"// 避免", "//", "#7f848e", true},
		{"e := recover", "recover", "#61afef", false},
		{"e != nil", "!=", "#56b6c2", false},
		{"e != nil", "nil", "#d19a66", false},
		{"make([]byte", "make", "#61afef", false},
		{"make([]byte", "byte", "#c678dd", false},
		{"byte, 1024", "1024", "#d19a66", false},
		{"buf = buf", "=", "#e5c07b", false},
		{"runtime.Stack", "Stack", "#61afef", false},
		{"\"panic found", "panic found", "#98c379", false},
	} {
		fragment := strings.Index(oneDarkGoReference, tc.fragment)
		if fragment < 0 || !strings.Contains(tc.fragment, tc.word) {
			t.Fatalf("missing reference fragment %q / %q", tc.fragment, tc.word)
		}
		at := len([]rune(oneDarkGoReference[:fragment+strings.Index(tc.fragment, tc.word)]))
		got := ui.TextRange{Color: oneDarkProPal.text}
		for _, r := range ranges {
			if r.Start <= at && r.End > at {
				got = r
				break
			}
		}
		if got.Color != ui.Hex(tc.color) || got.Italic != tc.italic {
			t.Errorf("%s in %q = %+v, want %s italic=%v", tc.word, tc.fragment, got, tc.color, tc.italic)
		}
	}
}

func TestGoSyntaxRoleThemeFallbacks(t *testing.T) {
	withAppearancePlugins(t)
	for _, p := range []*palette{paletteFor("", false), paletteFor("", true), matrixPal, ayuLightPal, ayuDarkPal,
		claudeLightPal, claudeDarkPal, qoderLightPal, qoderDarkPal, grokBotLightPal, grokBotDarkPal,
		cursorLightPal, cursorDarkPal, purpleGoldPal, pumpkinPal, witchsBrewPal, bloodMoonPal} {
		for _, pair := range [][2]highlight.Kind{
			{highlight.BuiltinType, highlight.Type}, {highlight.BuiltinFunction, highlight.Builtin},
			{highlight.Assignment, highlight.Operator},
		} {
			if p.syntax[pair[0]].A == 0 || p.syntax[pair[0]] != p.syntax[pair[1]] {
				t.Errorf("role %d lost the base theme's color", pair[0])
			}
		}
	}
	for _, derived := range []bool{true, false} {
		for _, explicit := range []bool{false, true} {
			syntax := `"type":"#aabbcc", "builtin":"#ddeeff", "operator":"#ccddaa"`
			want := []string{"#aabbcc", "#ddeeff", "#ccddaa"}
			if explicit {
				syntax += `, "builtinType":"#112233", "builtinFunction":"#445566", "assignment":"#778899"`
				want = []string{"#112233", "#445566", "#778899"}
			}
			raw := fmt.Sprintf(`{"base":"dark", "derived":%v, "syntax":{%s}}`, derived, syntax)
			file, err := parseThemeFile([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			p := paletteOfThemeFile(file, "go-roles", true, true, nil)
			for i, kind := range []highlight.Kind{highlight.BuiltinType, highlight.BuiltinFunction, highlight.Assignment} {
				if p.syntax[kind] != ui.Hex(want[i]) {
					t.Errorf("derived=%v explicit=%v role %d = %v, want %s", derived, explicit, kind, p.syntax[kind], want[i])
				}
			}
		}
	}
}

func TestShotOneDarkProGoReference(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	withAppearancePlugins(t)
	hs := themeHarness(t, themeOneDarkPro, true)
	path := hs.path("main.go")
	if err := os.WriteFile(path, []byte(oneDarkGoReference), 0o644); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	at := len([]rune(oneDarkGoReference[:strings.Index(oneDarkGoReference, "for _, f")]))
	hs.place(hs.a.activeEditor(), at, at)
	hs.settle()
	hs.shot("onedarkpro-go-reference")
}

func TestOneDarkProReferenceComments(t *testing.T) {
	withAppearancePlugins(t)
	source := "const hue = 177; // Material preview\n"
	for _, sameColor := range []bool{false, true} {
		p := *oneDarkProPal
		if sameColor {
			p.syntax[highlight.Comment] = p.text
		}
		ranges := referenceSyntaxRanges(t, source, &p)
		at := strings.Index(source, "//")
		found := false
		for _, r := range ranges {
			if r.Start <= at && r.End > at {
				found = true
				if r.Color != p.syntax[highlight.Comment] || !r.Italic {
					t.Errorf("comment styling = %+v, want the reference's italic comment", r)
				}
			}
		}
		if !found {
			t.Errorf("comment style disappeared when its color equals body text: %v", sameColor)
		}
	}
}

func TestOneDarkProReferenceItalicRangesStaySeparate(t *testing.T) {
	withAppearancePlugins(t)
	p := *oneDarkProPal
	p.syntax[highlight.Comment] = p.syntax[highlight.Keyword]
	ranges := referenceSyntaxRanges(t, "return /* note */\n", &p)
	if len(ranges) != 2 || ranges[0].Italic || !ranges[1].Italic || ranges[0].End > ranges[1].Start {
		t.Fatalf("equal-colored keyword and comment lost their separate styles: %+v", ranges)
	}
	for _, r := range referenceSyntaxRanges(t, "return /* note */\n", paletteFor("", true)) {
		if r.Italic {
			t.Fatal("One Dark Pro's comment style leaked into the default theme")
		}
	}
}

func TestOneDarkProReferenceSyntax(t *testing.T) {
	withAppearancePlugins(t)
	source := "// Material preview\n" +
		"interface Palette { tone: number; }\n" +
		"const ocean: Palette = { tone: 177, enabled: true };\n" +
		"export function label(value: Palette): string {\n" +
		"  const strength = 0.8;\n" +
		"  return `${value.tone}: ${strength}`;\n" +
		"}\nconsole.log(label(ocean));\n"
	ranges := referenceSyntaxRanges(t, source, oneDarkProPal)
	for _, tc := range []struct {
		fragment, word, color string
		italic                bool
	}{
		{"// Material", "//", "#7f848e", true},
		{"interface Palette", "interface", "#c678dd", false},
		{"interface Palette", "Palette", "#e5c07b", false},
		{"tone: number", "tone", "#e06c75", false},
		{"tone: number", "number", "#e5c07b", false},
		{"const ocean", "ocean", "#e5c07b", false},
		{"tone: 177", "177", "#d19a66", false},
		{"enabled: true", "true", "#d19a66", false},
		{"function label", "label", "#61afef", false},
		{"value: Palette", "value", "#e06c75", true},
		{"const strength", "strength", "#e5c07b", false},
		{"${value.tone}", "${", "#c678dd", false},
		{"${value.tone}", "value", "#e5c07b", false},
		{"${value.tone}", "tone", "#e06c75", false},
		{"${strength}", "strength", "#e06c75", false},
		{"console.log", "console", "#e5c07b", false},
		{"console.log", "log", "#61afef", false},
		{"label(ocean)", "label", "#61afef", false},
		{"label(ocean)", "ocean", "#e06c75", false},
	} {
		at := strings.Index(source, tc.fragment) + strings.Index(tc.fragment, tc.word)
		got := ui.TextRange{Color: oneDarkProPal.text}
		for _, r := range ranges {
			if r.Start <= at && r.End > at {
				got = r
				break
			}
		}
		if got.Color != ui.Hex(tc.color) || got.Italic != tc.italic {
			t.Errorf("%s in %q = %+v, want %s italic=%v", tc.word, tc.fragment, got, tc.color, tc.italic)
		}
	}
}

func TestOneDarkProReferenceWorkbench(t *testing.T) {
	withAppearancePlugins(t)
	p := oneDarkProPal
	// Main panes share a background; the inset sidebar has a darker face.
	if p.titlebar != p.editor || p.strip != p.editor || p.status != p.editor || p.panel != p.editor {
		t.Fatal("workbench surfaces do not share the editor background")
	}
	if p.chrome != ui.Hex("#21252b") {
		t.Fatal("the inset sidebar lost its darker background")
	}
	if p.active != p.surfaceHover || p.hover != p.surface || p.active == p.hover {
		t.Fatal("control states do not share distinct selected and hovered surfaces")
	}
	// FiveLeaf's own title bar and status bar take the editor's surface,
	// so the window reads as one page; a theme that names its own keeps it.
	for _, own := range []*palette{paletteFor("", true), paletteFor("", false)} {
		if own.titlebar != own.editor || own.strip != own.editor || own.status != own.editor {
			t.Error("FiveLeaf's title bar, tab strip and status bar do not share the editor's surface")
		}
	}
	for _, other := range []*palette{qoderDarkPal, qoderLightPal} {
		if other.titlebar != other.chrome {
			t.Error("another theme's title bar changed")
		}
	}
}

func TestSyntaxRoleThemeFallbacks(t *testing.T) {
	withAppearancePlugins(t)
	for _, derived := range []bool{true, false} {
		raw := fmt.Sprintf(`{
		  "base": "dark", "derived": %v,
		  "colors": {"chrome":"#202122"},
		  "syntax": {"variable":"#aabbcc", "punctuation":"#ddeeff", "string":"#ccddaa", "property":"#ffaabb"},
		  "tokenColors": [{"scope":"variable.parameter", "settings":{"fontStyle":"italic"}}]
		}`, derived)
		file, err := parseThemeFile([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		p := paletteOfThemeFile(file, "reference-fallbacks", true, true, nil)
		for kind, want := range map[highlight.Kind]string{
			highlight.Binding: "#aabbcc", highlight.Parameter: "#aabbcc", highlight.Object: "#aabbcc",
			highlight.Property: "#ffaabb", highlight.Bracket: "#ddeeff", highlight.Interpolation: "#ccddaa",
		} {
			if p.syntax[kind] != ui.Hex(want) {
				t.Errorf("derived=%v role %d = %v, want %s", derived, kind, p.syntax[kind], want)
			}
		}
		if !p.syntaxItalic[highlight.Parameter] || p.syntaxItalic[highlight.Property] || p.titlebar != p.chrome {
			t.Errorf("derived=%v lost a role style or the title bar's fallback", derived)
		}
	}
}

func TestOneDarkProReferenceStyleOverrides(t *testing.T) {
	withAppearancePlugins(t)
	file, err := parseThemeFile([]byte(`{
	  "base": "onedarkpro",
	  "colors": {"titleBar.activeBackground":"#30343c"},
	  "tokenColors": [
	    {"scope":"comment", "settings":{"fontStyle":""}},
	    {"scope":"variable.parameter", "settings":{"foreground":"#aabbcc", "fontStyle":""}}
	  ]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	p := paletteOfThemeFile(file, "reference-overrides", true, true, nil)
	if p.syntaxItalic[highlight.Comment] || p.syntaxItalic[highlight.Parameter] {
		t.Error("an empty fontStyle did not clear inherited italics")
	}
	if p.syntax[highlight.Comment] != oneDarkProPal.syntax[highlight.Comment] || p.syntax[highlight.Parameter] != ui.Hex("#aabbcc") {
		t.Error("style overrides lost inherited or explicit token colors")
	}
	if p.titlebar != ui.Hex("#30343c") || p.chrome != oneDarkProPal.chrome {
		t.Error("the title bar override changed the sidebar")
	}
}

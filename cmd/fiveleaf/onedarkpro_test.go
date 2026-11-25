package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/highlight"
)

// This subset comes from the installed official VS Code extension, rather
// than FiveLeaf's implementation. Its version, source hash and license live
// beside the fixture so a later upstream change can be reviewed explicitly.
func TestOneDarkProVSCodeReference(t *testing.T) {
	withAppearancePlugins(t)
	raw, err := os.ReadFile("testdata/onedarkpro/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Colors      map[string]string `json:"colors"`
		TokenColors map[string]string `json:"tokenColors"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	p := oneDarkProPal
	// Borders and selections use FiveLeaf's shared control colors; the other
	// colors still follow the upstream file, including the darker sidebar.
	workbench := map[string]string{
		"editorGroup.border":             "#3e4451",
		"list.activeSelectionBackground": "#323842",
	}
	for key, got := range map[string]ui.Color{
		"editor.background": p.editor, "editor.foreground": p.text,
		"sideBar.background":   p.chrome,
		"statusBar.foreground": p.statusFg, "editorGroup.border": p.border,
		"list.hoverBackground": p.hover, "list.activeSelectionBackground": p.active,
		"editor.lineHighlightBackground": p.line, "editor.selectionBackground": p.selection,
		"editorLineNumber.foreground": p.gutter, "editorLineNumber.activeForeground": p.gutterNow,
		"editor.findMatchHighlightBackground": p.match, "editor.findMatchBackground": p.matchNow,
		"editor.findMatchBorder": p.matchRing, "editorIndentGuide.background1": p.guide,
		"editorIndentGuide.activeBackground1": p.guideNow, "editorBracketMatch.background": p.bracket,
		"editor.wordHighlightBackground": p.word, "scrollbarSlider.background": p.scrollbar,
		"editorGutter.addedBackground": p.added, "editorGutter.modifiedBackground": p.changed,
		"editorGutter.deletedBackground": p.removed,
	} {
		want, ok := ref.Colors[key]
		if !ok {
			t.Fatalf("reference is missing %s", key)
		}
		if override, ok := workbench[key]; ok {
			want = override
		}
		if got != ui.Hex(want) {
			t.Errorf("%s = %v, want %s", key, got, want)
		}
	}
	for scope, kind := range map[string]highlight.Kind{
		"keyword": highlight.Keyword, "string": highlight.String,
		"entity.name.function": highlight.Function, "entity.name.type": highlight.Type,
		"constant.numeric": highlight.Number, "comment": highlight.Comment,
		"variable": highlight.Variable, "entity.name.tag": highlight.Tag,
		"entity.other.attribute-name": highlight.Attribute, "markup.heading": highlight.Heading,
		"markup.inserted": highlight.Inserted, "markup.deleted": highlight.Deleted,
	} {
		want, ok := ref.TokenColors[scope]
		if !ok {
			t.Fatalf("reference is missing %s", scope)
		}
		if got := p.syntax[kind]; got != ui.Hex(want) {
			t.Errorf("%s = %v, want upstream %s", scope, got, want)
		}
	}
	terminal := termTheme(p)
	for i, name := range []string{
		"Black", "Red", "Green", "Yellow", "Blue", "Magenta", "Cyan", "White",
		"BrightBlack", "BrightRed", "BrightGreen", "BrightYellow", "BrightBlue", "BrightMagenta", "BrightCyan", "BrightWhite",
	} {
		want, ok := ref.Colors["terminal.ansi"+name]
		if !ok {
			t.Fatalf("reference is missing terminal.ansi%s", name)
		}
		if got := terminal.Palette[i]; got != ui.Hex(want) {
			t.Errorf("terminal %s = %v, want upstream %s", name, got, want)
		}
	}
	if p.selected != p.selection.Over(p.editor) || p.selectedDim != p.selection.Alpha(0.5).Over(p.editor) {
		t.Error("editor selections lost the upstream tint or inactive attenuation")
	}
}

func TestOneDarkProControlReadability(t *testing.T) {
	withAppearancePlugins(t)
	p := oneDarkProPal
	for _, back := range []ui.Color{p.editor, p.chrome, p.panel, p.active, p.hover, p.surface, p.surfaceHover} {
		if ratio := contrast(p.text, back); ratio < 4.5 {
			t.Errorf("normal and active control labels on %v read at %.2f:1", back, ratio)
		}
	}
	// Git's subtle line marks need legible status text. Syntax colors follow
	// the upstream fixture; its deletion red has a lower contrast than UI text.
	for _, ink := range []ui.Color{p.modifiedFg, p.addedFg, p.removedFg} {
		for _, back := range []ui.Color{p.chrome, p.editor, p.panel} {
			if ratio := contrast(ink, back); ratio < 4.5 {
				t.Errorf("Git text %v on %v reads at %.2f:1", ink, back, ratio)
			}
		}
	}
}

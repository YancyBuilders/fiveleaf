package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/lsp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestWordSuggestionsAtCaret(t *testing.T) {
	const source = "projectOne projectTwo projectOne\n"
	hs, doc := editHarness(t, "words.go", source)
	hs.place(doc, len(source), len(source))
	hs.tt.Type("pr")
	step(hs.tt)
	if _, visible := hs.tt.Find("projectOne"); visible {
		t.Fatal("suggested before the typing pause")
	}
	hs.tt.Advance(wordSuggestPause)
	hs.settle()
	if _, visible := hs.tt.Find("projectOne"); !visible || hs.a.pluginPick.open || len(hs.a.wordSuggest.items) != 2 {
		t.Fatalf("missing caret suggestions: %+v", hs.a.wordSuggest)
	}
	hs.shot("editor-completion")
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyTab)
	expectText(t, doc, source+"projectTwo")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, doc, source+"pr")
	hs.tt.Type("o")
	hs.settle()
	hs.key(0, ui.KeyEscape)
	expectText(t, doc, source+"pro")
	if hs.a.wordSuggest != nil {
		t.Fatal("Escape kept the suggestions open")
	}
	hs.key(0, ui.KeyEnter)
	expectText(t, doc, source+"pro\n")
}

func TestWordSuggestionsDiscardAfterSplit(t *testing.T) {
	hs, doc, client := sourceNavigationHarness(t)
	caret := len("first")
	hs.place(doc, caret, caret)
	hs.tt.Type(".")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hs.until("member completion request", func() bool {
		raw, err := client.Call(ctx, "test/snapshot", nil)
		if err != nil {
			t.Fatal(err)
		}
		var state struct {
			Completion *struct {
				Context struct {
					TriggerKind      int
					TriggerCharacter string
				}
			}
		}
		if err := json.Unmarshal(raw, &state); err != nil {
			t.Fatal(err)
		}
		return state.Completion != nil && state.Completion.Context.TriggerKind == 2 && state.Completion.Context.TriggerCharacter == "."
	})
	hs.a.splitRight()
	hs.settle()
	if hs.a.activeEditor() == doc || hs.a.activeEditor().document != doc.document {
		t.Fatal("expected another view of the same buffer")
	}
	reply, err := client.Call(ctx, "test/replyCompletion", nil)
	if err != nil || string(reply) != "true" {
		t.Fatalf("release: %s, %v", reply, err)
	}
	hs.settle()
	if _, visible := hs.tt.Find(locales.CompletionPlaceholder()); visible || hs.a.wordSuggest != nil {
		t.Fatal("late completion appeared in another split")
	}
}

func TestWordSuggestionsRespectCompositionAndSettings(t *testing.T) {
	const source = "projectOne 变量名称\n"
	hs, doc := editHarness(t, "words.go", source)
	caret := utf8.RuneCountInString(source)
	hs.place(doc, caret, caret)
	hs.tt.Compose("变量", 2)
	hs.settle()
	if hs.a.wordSuggest != nil {
		t.Fatal("suggested during composition")
	}
	hs.a.settings.QuickSuggestions = false
	hs.tt.Type("变量")
	hs.settle()
	if hs.a.wordSuggest != nil {
		t.Fatal("automatic suggestions disabled")
	}
	expectText(t, doc, source+"变量")
}

func TestManualWordCompletionAndStaleAcceptance(t *testing.T) {
	const source = "😀 变量名称\n变量占位"
	hs, doc := editHarness(t, "words.txt", source)
	hs.a.settings.QuickSuggestions = false
	at := utf8.RuneCountInString("😀 变量名称\n变量")
	hs.place(doc, at, at)
	hs.key(ui.Ctrl, ui.KeySpace)
	if !hs.a.pluginPick.open || len(hs.a.pluginPick.items) != 1 || hs.a.pluginPick.items[0].Label != "变量名称" {
		t.Fatalf("manual candidates: %v", hs.a.pluginPick.items)
	}
	hs.a.closePluginPicker(0)
	hs.settle()
	expectText(t, doc, "😀 变量名称\n变量名称")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, doc, source)
	hs.place(doc, at, at)
	hs.a.complete()
	hs.a.closePluginPicker(0)
	doc.selectRange(0, 0, false)
	hs.settle()
	expectText(t, doc, source)
}

func TestWordSuggestionsClickAndEnter(t *testing.T) {
	const source = "projectName projectRoot\n"
	hs, doc := editHarness(t, "words.go", source)
	hs.place(doc, len(source), len(source))
	hs.tt.Type("proj")
	hs.settle()
	if err := hs.tt.Click("projectRoot"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	expectText(t, doc, source+"projectRoot")
	hs.key(ui.Cmd, ui.KeyZ)
	hs.tt.Type("e")
	hs.settle()
	if _, shown := hs.tt.Find("projectName"); !shown {
		t.Fatal("expected a suggestion before Enter")
	}
	hs.key(0, ui.KeyEnter)
	expectText(t, doc, source+"proje\n")
}

func TestCompletionSnippetAndImportUndo(t *testing.T) {
	const source = "// 😀\npackage demo\n\nfunc f() { fm }\n"
	hs, doc := editHarness(t, "demo.go", source)
	at := utf8.RuneCountInString(strings.Split(source, "fm")[0]) + 2
	hs.place(doc, at, at)
	point := lsp.Position{Line: 2}
	item := lsp.CompletionItem{Label: "fmt.Println", InsertTextFormat: 2, InsertText: "fmt.Println(${1:value})$0",
		AdditionalTextEdits: []lsp.TextEdit{{Range: lsp.Range{Start: point, End: point}, NewText: "import \"fmt\"\n"}}}
	applyCompletion(doc, item)
	want := "// 😀\npackage demo\nimport \"fmt\"\n\nfunc f() { fmt.Println(value) }\n"
	expectText(t, doc, want)
	if got := doc.buf.Snapshot().Slice(doc.sel[0], doc.sel[1]); got != "value" {
		t.Fatalf("selected %q", got)
	}
	doc.undo()
	expectText(t, doc, source)
	expectSel(t, doc, at, at)
}

func TestCompletionReusesMatchingSuffix(t *testing.T) {
	hs, doc := editHarness(t, "word.go", "print")
	hs.place(doc, 3, 3)
	applyCompletion(doc, lsp.CompletionItem{Label: "println", InsertText: "println()"})
	expectText(t, doc, "println()")
	doc.undo()
	expectText(t, doc, "print")
	expectSel(t, doc, 3, 3)
	applyCompletion(doc, lsp.CompletionItem{Label: "Print"})
	expectText(t, doc, "Print")
}

func TestCompletionFiltersMemberAccessors(t *testing.T) {
	hs, doc := editHarness(t, "user.ts", "user?.po")
	hs.place(doc, doc.buf.Len(), doc.buf.Len())
	for _, filter := range []string{".postalCode", "?.postalCode", "postalCode"} {
		request := hs.a.newWordSuggestions(doc, false)
		hs.a.showWords(request, doc.buf.Snapshot(), "po", []lsp.CompletionItem{
			{Label: "postalCode", FilterText: filter}, {Label: "port", FilterText: ".address"},
		}, nil)
		if len(request.items) != 1 || request.items[0].Label != "postalCode" {
			t.Fatalf("filter %q: %+v", filter, request.items)
		}
	}
}

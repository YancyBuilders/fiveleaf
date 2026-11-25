package main

import (
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestBuiltinFormatCommand(t *testing.T) {
	for _, test := range []struct{ name, source, formatted string }{
		{"file.go", "package example\nfunc add(a,b int)int{return a+b}\n", "package example\n\nfunc add(a, b int) int { return a + b }\n"},
		{"file.json", `{"z":9007199254740993,"a":"你好","a":4}`, "{\n  \"z\": 9007199254740993,\n  \"a\": \"你好\",\n  \"a\": 4\n}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hs, doc := editHarness(t, test.name, test.source)
			item := hs.a.menu(false, func(id string, _ *mygo.Window) { hs.a.run(id) }).ItemByID("sel.format")
			if item == nil || item.Disabled || item.Accelerator != "Shift+Alt+F" {
				t.Fatalf("Format Document menu command: %+v", item)
			}
			item.Click(item, nil)
			hs.until("built-in document formatting", func() bool { return doc.buf.String() == test.formatted })
			version := doc.buf.Version()
			var finished bool
			hs.a.format(doc, time.Second, false, func() { finished = true })
			hs.until("idempotent formatting", func() bool { return finished })
			if doc.buf.Version() != version {
				t.Fatal("formatting an already formatted file edited it")
			}
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, doc, test.source)
		})
	}
}

func TestBuiltinFormatSave(t *testing.T) {
	hs, doc := editHarness(t, "object.json", "{\"large\":12345678901234567890}\r\n")
	a := hs.a
	a.settings.FormatOnSave = map[string]bool{"*": true, "json": false}
	if a.formatsOnSave(doc.document) {
		t.Fatal("wildcard overrode the language's explicit opt-out")
	}
	delete(a.settings.FormatOnSave, "json")
	doc.indent = indentStyle{tabs: true}
	var saved bool
	a.saveDoc(doc.document, func() { saved = true })
	hs.until("formatted save", func() bool { return saved })
	disk, err := os.ReadFile(doc.path)
	if err != nil || string(disk) != "{\r\n\t\"large\": 12345678901234567890\r\n}\r\n" || doc.dirty() {
		t.Fatalf("saved %q, dirty %v, error %v", disk, doc.dirty(), err)
	}
	doc.edit(doc.sel, textEdit{0, doc.buf.Len(), "{\n\t// work in progress\n}"})
	saved = false
	a.saveDoc(doc.document, func() { saved = true })
	hs.until("saving incomplete JSON", func() bool { return saved })
	disk, err = os.ReadFile(doc.path)
	if err != nil || strings.ReplaceAll(string(disk), "\r\n", "\n") != doc.buf.String() || !strings.Contains(doc.buf.String(), "// work in progress") {
		t.Fatalf("invalid JSON save lost work: %q, %v", disk, err)
	}
}

func TestBuiltinFormatDiscardsObsoleteResults(t *testing.T) {
	hs, doc := editHarness(t, "object.json", `{"a":2}`)
	var finished bool
	hs.a.format(doc, time.Second, false, func() { finished = true })
	doc.edit(doc.sel, textEdit{0, doc.buf.Len(), `{"new":3}`})
	hs.until("stale formatter", func() bool { return finished })
	expectText(t, doc, `{"new":3}`)
	finished = false
	hs.a.format(doc, 0, true, func() { finished = true })
	hs.until("expired formatter", func() bool { return finished })
	expectText(t, doc, `{"new":3}`)
}

func TestFormatPreservesSelectionBetweenChanges(t *testing.T) {
	for _, test := range []struct{ path, source string }{
		{"main.go", "package main\n\nimport (\n\"unicode\"\n\"fmt\"\n)\n\nvar 年份 = 2026\n\nfunc main(){fmt.Println(年份, unicode.MaxRune)}\n"},
		{"file.json", `{"one":1,"年份":2,"three":3}`},
	} {
		hs, doc := editHarness(t, test.path, test.source)
		start := utf8.RuneCountInString(strings.Split(test.source, "年份")[0])
		hs.place(doc, start+2, start)
		var done bool
		hs.a.format(doc, time.Second, false, func() { done = true })
		hs.until("formatting with an active selection", func() bool { return done })
		if doc.buf.String() == test.source || doc.buf.Snapshot().Slice(doc.sel[1], doc.sel[0]) != "年份" {
			t.Fatalf("%s selection after format: %v in %q", test.path, doc.sel, doc.buf.String())
		}
	}
}

func TestColdLanguageServerFormatOnSave(t *testing.T) {
	hs := newHarness(t, false)
	withMarketplaceLanguageServers(hs)
	starting := lspStatusFixture(t, hs.root, "lsp-go.server", lspStarting)
	defer close(starting.ready)
	const source = "package main\nfunc main(){}\n"
	path := hs.path("cold.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	hs.a.openFile(path, true)
	hs.settle()
	hs.a.settings.FormatOnSave = map[string]bool{"go": true}
	var saved bool
	hs.a.saveDoc(hs.a.activeEditor().document, func() { saved = true })
	hs.until("formatted save during language server startup", func() bool { return saved })
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != "package main\n\nfunc main() {}\n" {
		t.Fatalf("saved %q, %v", disk, err)
	}
}

func TestFormatOnSaveSwitchPreservesLanguageChoices(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.a.settings.FormatOnSave = map[string]bool{"go": true, "json": false}
	prefsAgentClick(t, ph, prefSection(settings.CatPlugins))
	for _, on := range []bool{true, false} {
		prefsAgentClick(t, ph, locales.FormatAllLanguages())
		rules := ph.a.settings.FormatOnSave
		if rules["*"] != on || !rules["go"] || rules["json"] || len(rules) < 2 {
			t.Fatalf("toggled to %v: %v", on, rules)
		}
	}
}

func TestFormatSettingsJSONUsesFolderScope(t *testing.T) {
	ph := newPrefsHarness(t)
	ph.w.folder = ph.root
	scope := ph.w.scope()
	if scope == nil {
		t.Fatal("missing folder scope")
	}
	scope.h.f.win, scope.h.f.active = nil, scope.h
	ph.w.editSettingsJSON(scope)
	ph.settle()
	if got := ph.a.activeFile(); got != settings.FolderPath(ph.root) {
		t.Fatalf("editing format rules opened %q", got)
	}
}

func TestFormatUTF16Offsets(t *testing.T) {
	_, doc := editHarness(t, "unicode.json", "😀abc\n中😀文\n")
	snap := doc.buf.Snapshot()
	for at := range snap.Len() + 1 {
		if got := posRune(snap, positionOf(snap, at)); got != at {
			t.Fatalf("rune %d became %d", at, got)
		}
	}
	for input, want := range map[position]int{{Line: 1, Character: 2}: 7, {Line: 1, Character: -3}: 5, {Line: 1, Character: 100}: 8, {Line: 99}: snap.Len()} {
		if got := posRune(snap, input); got != want {
			t.Fatalf("%+v maps to %d, want %d", input, got, want)
		}
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiveleaf/locales"
)

func TestJSONLPagePreservesUTF8AtEveryBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	var source strings.Builder
	for index, suffix := range []string{"界", "🙂", "é"} {
		source.WriteString(strings.Repeat("x", jsonlPageBytes*(index+1)-1-source.Len()))
		source.WriteString(suffix)
	}
	source.WriteString("\r\nlast line")
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	for number := int64(0); number < 4; number++ {
		page, err := readJSONLPage(path, number)
		if err != nil || page.number != number || page.last != 3 {
			t.Fatalf("read page %d: number=%d last=%d err=%v", number, page.number, page.last, err)
		}
		joined.WriteString(page.text)
	}
	if joined.String() != source.String() {
		t.Fatal("page boundaries lost or duplicated bytes")
	}
}

func TestJSONLPreviewOpensOversizedFileAndRefreshes(t *testing.T) {
	hs := newHarness(t, true)
	path := hs.path("large.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	block := strings.Repeat("{} \n", jsonlPageBytes/4)
	for written := 0; written < maxOpenBytes && err == nil; written += len(block) {
		_, err = file.WriteString(block)
	}
	if err == nil {
		_, err = file.WriteString(`{"end":true}`)
	}
	if closeErr := file.Close(); err != nil || closeErr != nil {
		t.Fatalf("create transcript: write=%v close=%v", err, closeErr)
	}
	hs.a.openFile(path, true)
	hs.settle()
	hs.clickDetail(locales.LastPage())
	if !hs.tt.HasText(`{"end":true}`) {
		t.Fatal("could not reach the last record beyond the editable file limit")
	}
	if err := os.WriteFile(path, []byte(`{"new":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	hs.clickDetail(locales.Refresh())
	if !hs.tt.HasText(`{"new":true}`) || !hs.tt.HasText(locales.JsonlPage("1", "1")) {
		t.Fatal("refreshing a shortened transcript did not return to its available page")
	}
	hs.a.dropTab(hs.a.activeTab())
	hs.a.openFile(path, true)
	hs.settle()
	if ed := hs.a.activeEditor(); ed.locked() || ed.buf.String() != `{"new":true}` {
		t.Fatal("a short JSONL file should open in the ordinary editor")
	}
}

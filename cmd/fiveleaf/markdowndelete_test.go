package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLiveDeleteHiddenMarkup(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		key                 ui.Key
	}{
		{"heading", "## |正文", "|正文", ui.KeyBackspace},
		{"heading closing hashes", "  ## |正文 ##", "  |正文", ui.KeyBackspace},
		{"bold suffix", "前**中文**|后", "前**中|**后", ui.KeyBackspace},
		{"inline code grapheme", "`中👩🏽‍💻`|后", "`中|`后", ui.KeyBackspace},
		{"link label back", "前[文é](https://x)|后", "前[文|](https://x)后", ui.KeyBackspace},
		{"link label forward", "前[中文|](a.md)后", "前[中文](a.md)|", ui.KeyDelete},
		{"last grapheme", "前**👩🏽‍💻**|后", "前|后", ui.KeyBackspace},
		{"last visible grapheme", "前`字|`后", "前|后", ui.KeyBackspace},
		{"nested last grapheme", "前**`字`**|后", "前|后", ui.KeyBackspace},
		{"fenced literal", "```md\n**正文**|后\n```", "```md\n**正文*|后\n```", ui.KeyBackspace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, at := mdTypingPosition(tc.before)
			after, want := mdTypingPosition(tc.after)
			hs, d := editHarness(t, "writing.md", before)
			hs.a.setMarkdownMode("live")
			hs.place(d, at, at)
			hs.key(0, tc.key)
			expectText(t, d, after)
			expectSel(t, d, want, want)
			hs.key(ui.Cmd, ui.KeyZ)
			expectText(t, d, before)
			expectSel(t, d, at, at)
			hs.key(ui.Cmd|ui.Shift, ui.KeyZ)
			expectText(t, d, after)
		})
	}
}

func TestLiveLinkStartInsertionAndDeletion(t *testing.T) {
	const source = "甲[链接](a.md)乙"
	hs, d := editHarness(t, "link.md", source)
	hs.a.setMarkdownMode("live")
	hs.place(d, 1, 1)
	hs.tt.Compose("zh", 2)
	hs.settle()
	expectText(t, d, source)
	hs.tt.Type("新")
	hs.settle()
	expectText(t, d, "甲[新链接](a.md)乙")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, source)
	hs.place(d, 2, 2)
	hs.key(0, ui.KeyBackspace)
	expectText(t, d, "[链接](a.md)乙")
	hs.key(ui.Cmd, ui.KeyZ)
	expectSel(t, d, 2, 2)
}

func TestLiveDeleteAcrossInlineMarkup(t *testing.T) {
	const source = "前**甲乙**后"
	for _, selection := range [][2]int{{4, 8}, {8, 4}} {
		hs, d := editHarness(t, "selection.md", source)
		hs.a.setMarkdownMode("live")
		hs.place(d, selection[0], selection[1])
		hs.key(0, ui.KeyBackspace)
		expectText(t, d, "前**甲**")
		expectSel(t, d, 4, 4)
		hs.key(ui.Cmd, ui.KeyZ)
		expectText(t, d, source)
		expectSel(t, d, selection[0], selection[1])
	}
	hs, d := editHarness(t, "source.md", source)
	hs.place(d, 7, 7)
	hs.key(0, ui.KeyBackspace)
	expectText(t, d, "前**甲乙*后")
	hs, d = editHarness(t, "multiline.md", "**甲乙**\n丙丁")
	hs.a.setMarkdownMode("live")
	hs.place(d, 8, 3)
	hs.key(0, ui.KeyDelete)
	expectText(t, d, "**甲**丁")
	hs.key(ui.Cmd, ui.KeyZ)
	expectText(t, d, "**甲乙**\n丙丁")
	expectSel(t, d, 8, 3)
}

package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func TestHopHintLabels(t *testing.T) {
	base := len(hopHintAlphabet)
	for _, count := range []int{0, 1, base - 1, base, base + 1, base*base - 1, base * base, base*base + 1, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			labels := hopHintLabels(count)
			if len(labels) != count {
				t.Fatalf("got %d labels, want %d", len(labels), count)
			}
			seen := make(map[string]bool, count)
			for index, label := range labels {
				if label == "" || seen[label] {
					t.Fatalf("empty or duplicate label %q", label)
				}
				seen[label] = true
				if index > 0 && len(label) < len(labels[index-1]) {
					t.Fatalf("nearer target has longer label: %q, %q", labels[index-1], label)
				}
				for _, glyph := range label {
					if !strings.ContainsRune(hopHintAlphabet, glyph) {
						t.Fatalf("label %q contains invalid letter %q", label, glyph)
					}
				}
			}
			for _, label := range labels {
				for length := 1; length < len(label); length++ {
					if seen[label[:length]] {
						t.Fatalf("label %q is prefixed by %q", label, label[:length])
					}
				}
			}
		})
	}
}

func TestJumpHintCandidatesUnicode(t *testing.T) {
	snap := ui.NewTextBuffer("a e\u0301 café 中文 snake_case 12 👋\n  next\n").Snapshot()
	words := jumpHintCandidates(snap, 0, 0, jumpHintWords, nil)
	var names []string
	for _, target := range words {
		names = append(names, snap.Slice(target.start, target.end))
	}
	want := []string{"a", "e\u0301", "café", "中文", "snake_case", "12"}
	if !slices.Equal(names, want) {
		t.Fatalf("Hop words %q, want %q", names, want)
	}
	words = jumpHintCandidates(snap, 0, 0, jumpHintHelixWords, nil)
	names = names[:0]
	for _, target := range words {
		names = append(names, snap.Slice(target.start, target.end))
	}
	want = []string{"café", "中文", "snake_case", "12"}
	if !slices.Equal(names, want) {
		t.Fatalf("Helix words %q, want %q", names, want)
	}
}

func TestJumpHintCandidateLinesAndQueries(t *testing.T) {
	snap := ui.NewTextBuffer("offscreen\n  alpha ababa 中文\n\t beta\n  \nlast").Snapshot()
	lines := jumpHintCandidates(snap, 1, 3, jumpHintLines, nil)
	if len(lines) != 2 || snap.Slice(lines[0].start, lines[0].end) != "a" || snap.Slice(lines[1].start, lines[1].end) != "b" {
		t.Fatalf("first nonblank visible lines: %+v", lines)
	}
	chars := jumpHintCandidates(snap, 1, 1, jumpHintChar2, []rune("ab"))
	if len(chars) != 2 || chars[1].start-chars[0].start != 2 {
		t.Fatalf("overlapping line-local query: %+v", chars)
	}
	chars = jumpHintCandidates(snap, 1, 1, jumpHintChar1, []rune("文"))
	if len(chars) != 1 || snap.Slice(chars[0].start, chars[0].end) != "文" {
		t.Fatalf("Unicode query: %+v", chars)
	}
	chars = jumpHintCandidates(snap, 1, 4, jumpHintChar2, []rune("a\n"))
	if len(chars) != 0 {
		t.Fatalf("query crosses a line: %+v", chars)
	}
}

func TestJumpHintVisibleAndHidden(t *testing.T) {
	bounds := ui.Rect{X: 20, Y: 30, W: 120, H: 60}
	for _, test := range []struct {
		name string
		box  ui.Rect
		want bool
	}{
		{"inside", ui.Rect{X: 20, Y: 30, H: 20}, true},
		{"bottom", ui.Rect{X: 80, Y: 70, H: 20}, true},
		{"gutter", ui.Rect{X: 19, Y: 30, H: 20}, false},
		{"horizontal scroll", ui.Rect{X: 140, Y: 30, H: 20}, false},
		{"sticky row", ui.Rect{X: 20, Y: 29, H: 20}, false},
		{"clipped row", ui.Rect{X: 20, Y: 71, H: 20}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := jumpHintVisible(bounds, test.box); got != test.want {
				t.Fatalf("visible = %t, want %t", got, test.want)
			}
		})
	}
	hidden := [][2]int{{4, 9}, {20, 30}}
	for _, span := range [][2]int{{4, 5}, {8, 10}, {3, 5}, {18, 32}} {
		if !jumpHintHidden(hidden, span[0], span[1]) {
			t.Fatalf("range %v overlaps hidden text", span)
		}
	}
	for _, span := range [][2]int{{0, 4}, {9, 20}, {30, 31}} {
		if jumpHintHidden(hidden, span[0], span[1]) {
			t.Fatalf("range %v is visible", span)
		}
	}
}

func TestJumpHintPriorityAndHelixLimit(t *testing.T) {
	nearby := []jumpHintTarget{{start: 0, end: 3}, {start: 10, end: 13}, {start: 20, end: 23}, {start: 30, end: 33}}
	hop := orderJumpHints(slices.Clone(nearby), 21, jumpHintWords)
	if hop[0].start != 20 || hop[1].start != 30 || hop[2].start != 10 {
		t.Fatalf("Hop does not prioritize nearest targets: %+v", hop)
	}
	helix := orderJumpHints(slices.Clone(nearby), 21, jumpHintHelixWords)
	for index, want := range []int{20, 10, 30, 0} {
		if helix[index].start != want {
			t.Fatalf("Helix target %d starts at %d, want %d", index, helix[index].start, want)
		}
	}
	many := make([]jumpHintTarget, 1000)
	for index := range many {
		many[index] = jumpHintTarget{start: index * 3, end: index*3 + 2}
	}
	helix = orderJumpHints(many, 1500, jumpHintHelixWords)
	if len(helix) != 676 || helix[0].label != "aa" || helix[675].label != "zz" {
		t.Fatalf("Helix label range: count %d, first %q, last %q", len(helix), helix[0].label, helix[len(helix)-1].label)
	}
	for _, target := range helix {
		if len(target.label) != 2 {
			t.Fatalf("Helix label %q is not two characters", target.label)
		}
	}
}

func TestJumpHintCancelKeepsViewport(t *testing.T) {
	view := jumpHintView{bounds: ui.Rect{W: 200, H: 100}, first: 2, last: 6}
	state := jumpHintState{active: true, view: view, valid: true, query: []rune("a"), prefix: "e", targets: []jumpHintTarget{{start: 1, end: 3}}}
	state.cancel()
	if state.active || len(state.query) != 0 || state.prefix != "" || len(state.targets) != 0 || !state.valid || !sameJumpHintView(state.view, view) {
		t.Fatalf("cancel state: %+v", state)
	}
	state.clear()
	if state.valid || state.view.bounds.W != 0 {
		t.Fatalf("clear keeps stale viewport: %+v", state)
	}
}

func TestJumpHintsUnknownKeyUnicodeSearch(t *testing.T) {
	for _, kind := range []jumpHintKind{jumpHintChar1, jumpHintChar2} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			text := "alpha 中文 beta 中心\n"
			harness, editor := vimHarness(t, "unicode.txt", text)
			harness.a.startJumpHints(editor, kind)
			harness.frames()
			if active, noInputMethod := harness.tt.TextInputActive(); !active || noInputMethod {
				t.Fatalf("query input is not available to input methods: active=%t disabled=%t", active, noInputMethod)
			}
			harness.tt.TypeKey(0, ui.KeyUnknown, "中")
			harness.frames()
			if !editor.jumpHints.active || string(editor.jumpHints.query) != "中" {
				t.Fatalf("unknown key lost committed Unicode: active=%t query=%q", editor.jumpHints.active, string(editor.jumpHints.query))
			}
			wantTargets := 2
			if kind == jumpHintChar2 {
				if len(editor.jumpHints.targets) != 0 {
					t.Fatal("two-character search shows labels before its second character")
				}
				harness.tt.TypeKey(0, ui.KeyUnknown, "文")
				harness.frames()
				wantTargets = 1
			}
			if len(editor.jumpHints.targets) != wantTargets {
				t.Fatalf("Unicode search found %d targets, want %d", len(editor.jumpHints.targets), wantTargets)
			}
			target := editor.jumpHints.targets[0]
			harness.vim(target.label)
			expectCursor(t, editor, target.start)
			expectText(t, editor, text)
		})
	}
}

func TestJumpHintsComposedUnicodeSearch(t *testing.T) {
	text := "alpha 中文 beta 中心\n"
	harness, editor := vimHarness(t, "composed.txt", text)
	harness.a.startJumpHints(editor, jumpHintChar2)
	harness.frames()
	for index, step := range []struct {
		composition string
		committed   string
	}{{"zhong", "中"}, {"wen", "文"}} {
		harness.tt.Compose(step.composition, len(step.composition))
		harness.frames()
		if !editor.jumpHints.active || len(editor.jumpHints.query) != index || len(editor.jumpHints.targets) != 0 {
			t.Fatalf("composition created a query or target: active=%t query=%q targets=%d", editor.jumpHints.active, string(editor.jumpHints.query), len(editor.jumpHints.targets))
		}
		expectText(t, editor, text)
		harness.tt.Type(step.committed)
		harness.frames()
	}
	if string(editor.jumpHints.query) != "中文" || len(editor.jumpHints.targets) != 1 {
		t.Fatalf("committed query did not find its target: query=%q targets=%d", string(editor.jumpHints.query), len(editor.jumpHints.targets))
	}
	target := editor.jumpHints.targets[0]
	harness.vim(target.label)
	expectCursor(t, editor, target.start)
	expectText(t, editor, text)
}

func TestJumpHintsQueryBindingDoesNotType(t *testing.T) {
	text := "alpha beta\n"
	harness, editor := vimHarness(t, "binding.txt", text)
	withKeys(t, harness.a, `[{"key":"q","command":"go.hopChar1","when":"editorTextFocus"}]`)
	harness.tt.TypeKey(0, ui.KeyQ, "q")
	harness.frames()
	if !editor.jumpHints.querying() || editor.jumpHints.queryText != "" || len(editor.jumpHints.query) != 0 {
		t.Fatalf("command key leaked into its query: active=%t text=%q query=%q", editor.jumpHints.active, editor.jumpHints.queryText, string(editor.jumpHints.query))
	}
	harness.tt.TypeKey(0, ui.KeyB, "b")
	harness.frames()
	if !editor.jumpHints.active || len(editor.jumpHints.targets) != 1 || string(editor.jumpHints.query) != "b" {
		t.Fatalf("query did not resume after command key release: active=%t query=%q targets=%d", editor.jumpHints.active, string(editor.jumpHints.query), len(editor.jumpHints.targets))
	}
	target := editor.jumpHints.targets[0]
	harness.vim(target.label)
	expectCursor(t, editor, target.start)
	expectText(t, editor, text)
}

func TestJumpHintsSourceWithLivePreference(t *testing.T) {
	harness, editor := vimHarness(t, "source.txt", "one two three\n")
	editor.live = true
	harness.frames()
	harness.vim("gs")
	if !editor.jumpHints.active || len(editor.jumpHints.targets) != 3 {
		t.Fatalf("a non-Markdown live preference canceled source hints: active=%t targets=%d", editor.jumpHints.active, len(editor.jumpHints.targets))
	}
}

func TestJumpHintsHopPrefixes(t *testing.T) {
	const text = `# FiveLeaf 跳转测试

Vim: gs labels visible words. gS labels visible lines.
Helix: gw shows two letters and selects the target word.

alpha bravo charlie delta echo
foxtrot golf hotel india juliet
kilo lima mike november oscar
papa quebec romeo sierra tango
uniform victor whiskey xray yankee zulu

Jump across the screen while keeping the smooth cursor trail.
中文字符查询：中文 中心 中国。

Use :HopChar1 to search for one character.
Use :HopChar2 to search for a pair of characters.
Press Escape to cancel, or Backspace to correct the query.

## Live Preview

Visible **words** and [link text](https://example.com/hidden-source) receive labels.

> [!faq]- Folded content
> These hidden words should not receive labels.

ending visible target
`
	for _, input := range []string{"native-text", "key-only"} {
		t.Run(input, func(t *testing.T) {
			harness, editor := vimHarness(t, "prefixes.md", text)
			harness.a.setLive(editor, false)
			editor.vscroll.Focus()
			harness.settle()
			typeJumpKeys(harness, "gs")
			uniform := utf8.RuneCountInString(text[:strings.Index(text, "uniform")])
			target := jumpTargetAt(t, editor, uniform)
			t.Logf("uniform label=%q candidates=%d", target.label, len(editor.jumpHints.targets))
			if len(target.label) < 2 {
				t.Fatalf("fixture did not assign a multi-character label: %q among %d", target.label, len(editor.jumpHints.targets))
			}
			initial := editor.sel
			labels := slices.Clone(editor.jumpHints.targets)
			for index, character := range target.label {
				modifiers, key := vimKeyOf(t, character)
				if input == "native-text" {
					harness.tt.TypeKey(modifiers, key, string(character))
				} else {
					harness.tt.Key(modifiers, key)
				}
				harness.frames()
				if index+1 < len(target.label) {
					if !editor.jumpHints.active || editor.jumpHints.prefix != target.label[:index+1] || editor.sel != initial {
						t.Fatalf("prefix %q lost hints or moved: active=%t prefix=%q selection=%v target=%q", target.label[:index+1], editor.jumpHints.active, editor.jumpHints.prefix, editor.sel, target.label)
					}
					if !slices.Equal(labels, editor.jumpHints.targets) {
						t.Fatal("typing a prefix changed target labels or positions")
					}
				}
			}
			expectCursor(t, editor, uniform)
			expectText(t, editor, text)
		})
	}
}

func TestJumpHintsBatchedNavigation(t *testing.T) {
	var source strings.Builder
	for line := range 80 {
		fmt.Fprintf(&source, "line%02d alpha bravo charlie delta\n", line)
	}
	for _, mode := range []string{"vim", "helix"} {
		for _, motion := range []string{"gg", "G"} {
			t.Run(mode+"/"+motion, func(t *testing.T) {
				harness, editor := editHarness(t, "batched.txt", source.String())
				harness.a.settings.EditorMode = mode
				harness.a.settings.Vim = mode == "vim"
				harness.frames()
				if mode == "helix" {
					helixSelect(editor, 9, 9)
					harness.frames()
				} else {
					harness.place(editor, 9, 9)
				}
				keys := motion + "gs"
				if mode == "helix" {
					keys = motion + "gw"
				}
				for _, character := range keys {
					modifiers, key := vimKeyOf(t, character)
					event := ui.InputEvent{Kind: ui.InputKeyDown, Mods: modifiers, Key: key}
					if mode == "helix" {
						harness.a.helixKey(editor, event)
					} else {
						harness.a.vimKey(editor, event)
					}
				}
				harness.tt.Frame()
				harness.frames()
				if !editor.jumpHints.active || editor.jumpHints.waiting || len(editor.jumpHints.targets) == 0 {
					t.Fatalf("batched %s lost labels: active=%t waiting=%t targets=%d", keys, editor.jumpHints.active, editor.jumpHints.waiting, len(editor.jumpHints.targets))
				}
				snapshot := editor.buf.Snapshot()
				cursor := vimCursor(editor, snapshot)
				if mode == "helix" {
					cursor = helixCursor(editor, snapshot)
				}
				wantLine := 0
				if motion == "G" {
					wantLine = vimLast(snapshot)
				}
				if line := snapshot.LineAt(cursor); line != wantLine {
					t.Fatalf("motion landed on line %d, want %d", line, wantLine)
				}
				if editor.jumpHints.selection != editor.sel || !sameJumpHintView(editor.jumpHints.basis, editor.jumpHints.view) {
					t.Fatal("labels captured an old selection or viewport")
				}
				for _, target := range editor.jumpHints.targets {
					line := snapshot.LineAt(target.start)
					if line < editor.jumpHints.view.first || line > editor.jumpHints.view.last {
						t.Fatalf("target on line %d is outside current viewport %d:%d", line, editor.jumpHints.view.first, editor.jumpHints.view.last)
					}
				}
			})
		}
	}
}

func TestJumpHintsSelectionChangeStillCancels(t *testing.T) {
	harness, editor := vimHarness(t, "selection.txt", "alpha beta gamma\n")
	harness.vim("gs")
	if !editor.jumpHints.active {
		t.Fatal("labels were not shown")
	}
	harness.place(editor, 6, 6)
	if editor.jumpHints.active || editor.jumpHints.waiting {
		t.Fatal("changing the selection kept labels captured from an older caret")
	}
}

package main

import (
	"image"
	"slices"
	"strconv"
	"strings"
	"testing"

	"fiveleaf/internal/highlight"
	"fiveleaf/internal/settings"
	"github.com/egoist/mygo/ui"
)

func TestAgentMarkdownRichContent(t *testing.T) {
	node := &viewNode{Text: "# Result\n\nSome **bold** and `inline code`.\n\n" +
		"- first item\n- second item\n\n3. third step\n4. fourth step\n\n" +
		"> quoted explanation\n\n| Model | Context |\n|---|---|\n| Pi | 128k |\n"}
	p, s := darkPalette().derive(), settings.Default()
	tt := ui.NewTester(func(c *ui.Context) {
		sharedAgentMarkdown(c, "message", node, p, s)
	}, 540, 850)
	for _, text := range []string{"Result", "Some bold and inline code.", "first item", "second item", "3.", "4.", "quoted explanation", "Model", "Context", "Pi", "128k"} {
		if !tt.HasText(text) {
			t.Errorf("missing %q in %q", text, tt.Texts())
		}
	}
	if !node.parsed {
		t.Fatal("rendered message still looks unparsed to the conversation scroll state")
	}
}

func TestAgentMarkdownCodeCopyLocales(t *testing.T) {
	for _, test := range []struct {
		locale, copy, copied string
	}{
		{"en", "Copy code", "Copied"},
		{"zh-Hans", "复制代码", "已复制"},
		{"ja", "コードをコピー", "コピーしました"},
	} {
		t.Run(test.locale, func(t *testing.T) {
			code := "func main() {\n\tprintln(\"hello\")\n}\n"
			node := &viewNode{Text: "```go\n" + code + "```\n"}
			p, s := darkPalette().derive(), settings.Default()
			tt := ui.NewTester(func(c *ui.Context) {
				sharedAgentMarkdown(c, "message", node, p, s)
			}, 400, 300)
			tt.SetLocale(test.locale)
			if err := tt.Click(test.copy); err != nil {
				t.Fatal(err)
			}
			if got := tt.Clipboard(); got != code {
				t.Errorf("copied %q, want %q", got, code)
			}
			if !tt.HasText(test.copied) {
				t.Errorf("missing localized feedback %q in %q", test.copied, tt.Texts())
			}
		})
	}
}

func TestAgentMarkdownStreamingFence(t *testing.T) {
	node := &viewNode{Text: "Before the code.\n\n```go\nconst answer = \"hel"}
	p, s := darkPalette().derive(), settings.Default()
	streaming := true
	tt := ui.NewTester(func(c *ui.Context) {
		sharedAgentMarkdown(c, "message", node, p, s, streaming)
	}, 400, 400)
	tt.SetLocale("en")
	if !tt.HasText("const answer") {
		t.Fatalf("unfinished code disappeared: %q", tt.Texts())
	}
	if err := tt.Click("Copy code"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "" {
		t.Fatalf("unfinished block copied partial output %q", got)
	}
	// Agent events replace viewNode as text grows; the keyed renderer must
	// keep its parser and selection through those replacements.
	node = &viewNode{Text: node.Text + "lo\"\n```\n\nAfter the code."}
	streaming = false
	tt.Frame()
	if !tt.HasText("After the code.") || !tt.HasText("const answer = \"hello\"") {
		t.Fatalf("completed stream lost content: %q", tt.Texts())
	}
	if err := tt.Click("Copy code"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "const answer = \"hello\"\n" {
		t.Fatalf("completed block copied %q", got)
	}
}

func TestAgentMarkdownFinishedUnclosedFenceCopy(t *testing.T) {
	for _, language := range []string{"go", "custom-language"} {
		for _, stopped := range []bool{false, true} {
			name := language + "/completed"
			if stopped {
				name = language + "/stopped"
			}
			t.Run(name, func(t *testing.T) {
				code := "const answer = \"hello\""
				node := &viewNode{Text: "```" + language + "\n" + code}
				p, s := darkPalette().derive(), settings.Default()
				streaming := stopped
				tt := ui.NewTester(func(c *ui.Context) {
					sharedAgentMarkdown(c, "message", node, p, s, streaming)
				}, 400, 300)
				tt.SetLocale("en")
				if streaming {
					if err := tt.Click("Copy code"); err != nil {
						t.Fatal(err)
					}
					if got := tt.Clipboard(); got != "" {
						t.Fatalf("streaming unclosed fence copied %q", got)
					}
					streaming = false
					tt.Frame()
				}
				if err := tt.Click("Copy code"); err != nil {
					t.Fatal(err)
				}
				if got := tt.Clipboard(); got != code+"\n" {
					t.Fatalf("finished unclosed fence copied %q, want %q", got, code+"\n")
				}
			})
		}
	}
}

func TestAgentMarkdownSafeLinks(t *testing.T) {
	node := &viewNode{Text: "Open [Docs](https://go.dev).\n\nBlocked [Script](javascript:alert(1)).\n\nBlocked [File](file:///tmp/test.sh).\n\nBlocked [Custom](fiveleaf://run).\n"}
	p, s := darkPalette().derive(), settings.Default()
	tt := ui.NewTester(func(c *ui.Context) {
		sharedAgentMarkdown(c, "message", node, p, s)
	}, 400, 400)
	for _, label := range []string{"Docs", "Script", "File", "Custom"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if got := tt.OpenedURLs(); !slices.Equal(got, []string{"https://go.dev"}) {
		t.Fatalf("opened unsafe destinations: %q", got)
	}
}

func TestAgentMarkdownCodeFitsNarrowConversation(t *testing.T) {
	for _, width := range []float32{180, 340, 780} {
		t.Run(strconv.Itoa(int(width)), func(t *testing.T) {
			line := "const value = \"" + strings.Repeat("long output ", 40) + "\""
			node := &viewNode{Text: "```typescript\n" + line + "\n```\n\nAfter the code.\n\n| Name | Value |\n|---|---|\n| Long table | " + strings.Repeat("wide ", 40) + " |\n\nAfter the table."}
			p, s := darkPalette().derive(), settings.Default()
			tt := ui.NewTester(func(c *ui.Context) {
				sharedAgentMarkdown(c, "message", node, p, s)
			}, int(width), 800)
			tt.SetLocale("en")
			for _, label := range []string{"Code, typescript", "Copy code", "After the code.", "After the table."} {
				r, ok := tt.Find(label)
				if !ok || r.X < 0 || r.X+r.W > width+1 {
					t.Errorf("%q escapes a %vpx conversation: %+v (found=%v)", label, width, r, ok)
				}
			}
			before, ok := tt.Find(line)
			if !ok {
				t.Fatal("long code line is missing")
			}
			copyBefore, _ := tt.Find("Copy code")
			tt.Scroll(width/2, before.Y+before.H/2, 180, 0)
			step(tt)
			after, _ := tt.Find(line)
			copyAfter, _ := tt.Find("Copy code")
			if after.X >= before.X {
				t.Errorf("long code does not scroll horizontally: %+v -> %+v", before, after)
			}
			if copyAfter != copyBefore {
				t.Errorf("horizontal code scrolling moved its copy control: %+v -> %+v", copyBefore, copyAfter)
			}
		})
	}
}

func TestAgentMarkdownSyntaxColorsRefresh(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "light"
		p := lightPalette().derive()
		if dark {
			name, p = "dark", darkPalette().derive()
		}
		t.Run(name, func(t *testing.T) {
			keyword, literal, comment := ui.Hex("#ff2020"), ui.Hex("#20ff20"), ui.Hex("#2020ff")
			p.syntax[highlight.Keyword] = keyword
			p.syntax[highlight.String] = literal
			p.syntax[highlight.Comment] = comment
			s := settings.Default()
			s.FontSize = 20
			node := &viewNode{Text: "```go\nconst message = \"hello\" // ready\n```"}
			tt := ui.NewTester(func(c *ui.Context) {
				sharedAgentMarkdown(c, "message", node, p, s)
			}, 600, 250)
			tt.SetDark(dark)
			for name, color := range map[string]ui.Color{"keyword": keyword, "string": literal, "comment": comment} {
				if n := agentMarkdownColorPixels(tt.Image(), color); n < 5 {
					t.Errorf("%s syntax is not painted in the palette color (%d pixels)", name, n)
				}
			}
			updated := ui.Hex("#ff20ff")
			p.syntax[highlight.Keyword] = updated
			tt.Frame()
			if n := agentMarkdownColorPixels(tt.Image(), updated); n < 5 {
				t.Errorf("cached code did not adopt the new theme (%d pixels)", n)
			}
			if n := agentMarkdownColorPixels(tt.Image(), keyword); n != 0 {
				t.Errorf("cached code retains the old theme (%d pixels)", n)
			}
		})
	}
}

func agentMarkdownColorPixels(img *image.RGBA, color ui.Color) int {
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			pixel := img.RGBAAt(x, y)
			if abs(int(pixel.R)-int(color.R)) <= 8 && abs(int(pixel.G)-int(color.G)) <= 8 && abs(int(pixel.B)-int(color.B)) <= 8 {
				count++
			}
		}
	}
	return count
}

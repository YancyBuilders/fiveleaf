package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func testMermaidStyle() mermaidStyle {
	p := darkPalette()
	return mermaidStyle{
		bg: p.editor, fg: p.text, muted: p.muted, line: p.muted, surface: p.surface, surfaceAlt: p.surfaceHover,
		border: p.border, accent: p.accent, danger: p.danger, mono: "monospace", monoWeight: 400,
	}
}

func requireMermaidContrast(t *testing.T, part string, ink, surface ui.Color, minimum float64) {
	t.Helper()
	if ratio := contrast(ink, surface); ratio < minimum {
		t.Errorf("%s contrast %.2f:1, want at least %.1f:1 (%v on %v)", part, ratio, minimum, ink, surface)
	}
}

func TestMermaidThemeForActorsAndClasses(t *testing.T) {
	for _, theme := range []*palette{lightPalette(), darkPalette()} {
		application := &app{pal: theme, settings: settings.Default()}
		style := application.mermaidStyle()
		if style.surface.B <= style.surface.R {
			t.Errorf("dark=%v, expected a cool card surface, got %v", theme.dark, style.surface)
		}
		requireMermaidContrast(t, "card caption", style.fg, style.surface, 4.5)
		requireMermaidContrast(t, "card border", style.border, style.surface, 3)
		requireMermaidContrast(t, "connector", style.line, style.bg, 3)
		for _, source := range []string{"sequenceDiagram\nA->>B: message", "classDiagram\nclass A\nA <|-- B", "stateDiagram-v2\n[*] --> Saved\nSaved --> [*]"} {
			diagram := renderMermaid(source, style)
			if diagram.err != "" || !bytes.Contains(diagram.shapes, []byte(`fill="`+hexColor(style.surface)+`" stroke="`+hexColor(style.border)+`"`)) {
				t.Fatalf("dark=%v, themed card missing from %q: %s", theme.dark, source, diagram.err)
			}
			for _, label := range diagram.labels {
				requireMermaidContrast(t, label.text, label.color, style.surface, 4.5)
			}
		}
		chart := renderMermaid("pie title Scores\n\"First\": 1\n\"Second\": 2", style)
		if !bytes.Contains(chart.shapes, []byte("#4c78a8")) || !bytes.Contains(chart.shapes, []byte("#f58518")) {
			t.Fatal("data series lost their distinct colors")
		}
	}
}

func TestRenderMermaid(t *testing.T) {
	st := testMermaidStyle()
	for name, src := range map[string]string{
		"flowchart": "flowchart LR\nA[Start] --> B{Ok?}\nB -- yes --> C[Done]\nB -- no --> A",
		"sequence":  "sequenceDiagram\nAlice->>Bob: Hello\nBob-->>Alice: Hi",
		"class":     "classDiagram\nclass Animal{\n+String name\n}\nAnimal <|-- Duck",
		"gantt":     "gantt\ntitle Plan\ndateFormat YYYY-MM-DD\nsection A\nTask :a1, 2024-01-01, 30d",
		"pie":       "pie title Pets\n\"Dogs\" : 386\n\"Cats\" : 85",
		"git":       "gitGraph\ncommit\nbranch dev\ncommit\ncheckout main\nmerge dev",
	} {
		d := renderMermaid(src, st)
		if d.err != "" {
			t.Errorf("%s: %s", name, d.err)
			continue
		}
		if d.svg == nil || d.w <= 0 || d.h <= 0 || len(d.labels) == 0 {
			t.Errorf("%s: svg %v %vx%v, %d labels", name, d.svg != nil, d.w, d.h, len(d.labels))
		}
		for _, l := range d.labels {
			if l.x < -d.w || l.x > 2*d.w || l.y < -d.h || l.y > 2*d.h {
				t.Errorf("%s: a label at %v,%v outside %vx%v", name, l.x, l.y, d.w, d.h)
			}
		}
	}
}

func TestRenderMermaidTiles(t *testing.T) {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for i := range 30 {
		fmt.Fprintf(&b, "N%d[Step %d<br/>more] --> N%d\n", i, i, i+1)
	}
	d := renderMermaid(b.String(), testMermaidStyle())
	if d.err != "" {
		t.Fatal(d.err)
	}
	if d.h <= mermaidTileSide || len(d.tiles) < 2 {
		t.Fatalf("%vx%v in %d tiles", d.w, d.h, len(d.tiles))
	}
	area := float32(0)
	for _, tile := range d.tiles {
		w, h := tile.svg.Size()
		if tile.w > mermaidTileSide || tile.h > mermaidTileSide || w != tile.w || h != tile.h {
			t.Errorf("tile at %v,%v is %vx%v, its SVG %vx%v", tile.x, tile.y, tile.w, tile.h, w, h)
		}
		if tile.x+tile.w > d.w+0.01 || tile.y+tile.h > d.h+0.01 {
			t.Errorf("tile at %v,%v %vx%v past %vx%v", tile.x, tile.y, tile.w, tile.h, d.w, d.h)
		}
		area += tile.w * tile.h
	}
	if math.Abs(float64(area-d.w*d.h)) > 1 {
		t.Errorf("tiles cover %v of %v", area, d.w*d.h)
	}
	if small := renderMermaid("flowchart LR\nA --> B", testMermaidStyle()); len(small.tiles) != 1 || small.tiles[0].svg != small.svg {
		t.Errorf("a small diagram in %d tiles", len(small.tiles))
	}
}

func TestRenderMermaidError(t *testing.T) {
	d := renderMermaid("sequenceDiagram\nA->>B hi", testMermaidStyle())
	if d.err == "" || strings.Contains(d.err, "\n") || !strings.Contains(d.err, "line 2") {
		t.Errorf("err = %q", d.err)
	}
	if d := renderMermaid("not a diagram", testMermaidStyle()); d.err == "" {
		t.Error("no error for text that is no diagram")
	}
}

func TestMermaidSVG(t *testing.T) {
	st := testMermaidStyle()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50" style="--_bg:var(--pele-bg,#fff);--_s:var(--pele-surface,#f3f3f3);--_a:var(--pele-nope,#808080);--_r:var(--pele-radius,4px);font:16px sans-serif" fill="var(--_fg,#123456)">` +
		`<g transform="translate(10,20)"><rect x="0" y="0" width="10" height="10" fill="var(--_s)" style="rx:var(--_r)"/>` +
		`<rect fill="color-mix(in srgb,var(--_a) 50%,#000000)"/>` +
		`<text x="5" y="6" text-anchor="middle">A &amp; B</text>` +
		`<text text-anchor="middle" xml:space="preserve"><tspan x="0" y="-4" font-weight="var(--_w)">one</tspan><tspan font-family="var(--_fm,monospace)"> two</tspan><tspan x="0" y="16" font-size="12">three</tspan></text></g>` +
		`<g transform="rotate(90 0 0)"><text x="1" y="2">turned</text></g></svg>`
	out, texts, err := mermaidSVG(svg, st)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`fill="` + hexColor(st.surface) + `"`, ` rx="4"`, `fill="#404040"`, `fill="#123456"`} {
		if !strings.Contains(s, want) {
			t.Errorf("no %s in %s", want, s)
		}
	}
	for _, gone := range []string{"<text", "tspan", "var(", "--_"} {
		if strings.Contains(s, gone) {
			t.Errorf("%s left in %s", gone, s)
		}
	}
	if _, err := ui.ParseSVG(out); err != nil {
		t.Fatal(err)
	}
	if len(texts) != 4 {
		t.Fatalf("%d chunks: %+v", len(texts), texts)
	}
	if r := texts[0].runs; len(r) != 1 || r[0].text != "A & B" || r[0].size != 16 || texts[0].anchor != "middle" {
		t.Errorf("first chunk %+v", texts[0])
	}
	if x, y := texts[0].m.apply(texts[0].x, texts[0].y); x != 15 || y != 26 {
		t.Errorf("first chunk at %v,%v", x, y)
	}
	if r := texts[1].runs; len(r) != 2 || r[0].style != pelebold || r[1].style != peleMono || r[1].text != " two" || texts[1].y != -4 {
		t.Errorf("second chunk %+v", texts[1])
	}
	if r := texts[2].runs; len(r) != 1 || r[0].size != 12 || texts[2].y != 16 {
		t.Errorf("third chunk %+v", texts[2])
	}
	if l := texts[3].shape(st); len(l) != 1 || l[0].angle != 90 {
		t.Errorf("turned %+v", l)
	}
}

func TestMermaidStyledFillLabels(t *testing.T) {
	st := testMermaidStyle()
	source := "flowchart LR\n subgraph S[\"sub\"]\n A[aa]\n end\n A --> B[bb] --> C[cc] --> D[dd]\n" +
		"style S fill:#ffe\n style B fill:#fdd,stroke:#c00\n style C fill:#fdd,color:#f00\n style D fill:#333\n"
	svg, err := peleRender(source, st)
	if err != nil {
		t.Fatal(err)
	}
	_, texts, err := mermaidSVG(svg, st)
	if err != nil {
		t.Fatal(err)
	}
	colors := map[string]ui.Color{}
	for _, x := range texts {
		for _, r := range x.runs {
			colors[r.text] = r.color
		}
	}
	for text, want := range map[string]ui.Color{"sub": st.bg, "aa": ui.Hex(st.pelePalette(true)["--flow-node-label"]), "bb": st.bg, "cc": ui.Hex("#ff0000"), "dd": st.fg} {
		if got := colors[text]; got != want {
			t.Errorf("%s in %+v, want %+v", text, got, want)
		}
	}
}

func TestMermaidFlowchartPresentation(t *testing.T) {
	const source = `flowchart TB
A["执行原有生成流程"] --> B{"首次成功且<br/>产物有效？"}
B -->|"是"| C["在成功状态写入中记录 generation_completed_at"]
B -. "只提供统计候选值" .-> D{"自定义样式？"}
style D fill:#ffeedd,stroke:#a10,stroke-width:3px,color:#112233
linkStyle 1 stroke:#008855,stroke-width:3px,color:#005533`
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			style := testMermaidStyle()
			if mode == "light" {
				style.bg, style.fg = ui.Hex("#ffffff"), ui.Hex("#252526")
			}
			diagram := renderMermaid(source, style)
			if diagram.err != "" {
				t.Fatal(diagram.err)
			}
			palette := style.pelePalette(true)
			shapes := string(diagram.shapes)
			for _, want := range []string{`class="fiveleaf-elk pele-flowchart"`, `rx="16"`, `rx="13"`, `stroke="` + palette["--flow-edge-outline"] + `"`, `stroke-dasharray="3 4"`, `stroke="#008855"`, `fill:#ffeedd`} {
				if !strings.Contains(shapes, want) {
					t.Errorf("missing SVG property %s", want)
				}
			}
			if count := strings.Count(shapes, `stroke-dasharray="2 2"`); count != 1 || strings.Contains(shapes, `<polygon`) {
				t.Fatalf("%d default decision outlines, want one", count)
			}
			labels := map[string]mermaidLabel{}
			for _, label := range diagram.labels {
				labels[strings.TrimSpace(label.text)] = label
			}
			first, second := labels["在成功状态写入中记录"], labels["generation_completed_at"]
			if first.text == "" || second.text == "" || second.y <= first.y {
				t.Fatalf("bilingual node was not set on two lines: %+v", labels)
			}
			if labels["只提供统计候选值"].text == "" || labels["是"].color != ui.Hex("#005533") || labels["自定义样式？"].color != ui.Hex("#112233") {
				t.Fatal("missing edge label or source-specified text color")
			}
			edgeText := labels["只提供统计候选值"].color
			edgeFill := ui.Hex(palette["--flow-edge-surface"])
			if edgeText != ui.Hex(palette["--flow-edge-label"]) || !strings.Contains(shapes, `fill="`+hexColor(edgeFill)+`"`) {
				t.Fatalf("edge label or fill did not adopt the theme: %v on %v", edgeText, edgeFill)
			}
			if !strings.Contains(shapes, `stroke="`+palette["--pele-line"]+`"`) {
				t.Fatal("the default connector lost its theme color")
			}
			requireMermaidContrast(t, "flowchart connector", ui.Hex(palette["--pele-line"]), style.bg, 3)
			requireMermaidContrast(t, "edge label", edgeText, edgeFill, 4.5)
			requireMermaidContrast(t, "edge outline", ui.Hex(palette["--flow-edge-outline"]), edgeFill, 3)
			fill := ui.Hex(palette["--pele-surface"])
			requireMermaidContrast(t, "process label", first.color, fill, 4.5)
			if first.font.Weight != 600 || first.font.Size != 14 {
				t.Fatalf("unexpected flowchart label font: %+v", first.font)
			}
			exported := string(diagram.exportSVG())
			if !strings.Contains(exported, `>`+second.text+`</text>`) || !strings.Contains(exported, `font-weight="600"`) || !strings.Contains(exported, `stroke-dasharray="2 2"`) {
				t.Fatal("the SVG export lost diagram labels or decision styling")
			}
		})
	}
}

func mermaidNodeShapes(t *testing.T, source []byte) map[string]xml.StartElement {
	t.Helper()
	var svg struct {
		Groups []struct {
			ID   string `xml:"data-id,attr"`
			Rect struct {
				Attrs []xml.Attr `xml:",any,attr"`
			} `xml:"rect"`
		} `xml:"g"`
	}
	if err := xml.Unmarshal(source, &svg); err != nil {
		t.Fatal(err)
	}
	result := map[string]xml.StartElement{}
	for _, group := range svg.Groups {
		result[group.ID] = xml.StartElement{Attr: group.Rect.Attrs}
	}
	return result
}

func TestMermaidFlowchartRoleColors(t *testing.T) {
	const source = `flowchart LR
A[Start] --> A2[Process]
B[Input] --> B2[Validate]
C[Schedule] --> C2[Prepare]
A2 --> Join[Shared]
B2 --> Join
C2 --> Join
Join --> Check{Proceed?}
Check --> Custom{Override}
style Custom stroke:#ab3470
classDef branded color:#974862
class B branded
style C fill:#f9f2e9,fill-opacity:0.12`
	for _, theme := range []*palette{lightPalette(), darkPalette()} {
		style := (&app{pal: theme, settings: settings.Default()}).mermaidStyle()
		diagram := renderMermaid(source, style)
		if diagram.err != "" {
			t.Fatal(diagram.err)
		}
		vars := style.pelePalette(true)
		shapes := mermaidNodeShapes(t, diagram.shapes)
		labels := map[string]ui.Color{}
		for _, label := range diagram.labels {
			labels[label.text] = label.color
		}
		seen := map[string]bool{}
		for _, part := range []struct{ node, caption, fill, border, ink string }{
			{"A2", "Process", "--pele-surface", "--pele-border", "--flow-node-label"},
			{"B2", "Validate", "--flow-teal", "--flow-teal-border", "--flow-teal-label"},
			{"C2", "Prepare", "--flow-sage", "--flow-sage-border", "--flow-sage-label"},
			{"Join", "Shared", "--flow-shared", "--flow-shared-border", "--flow-shared-label"},
			{"Check", "Proceed?", "--flow-decision", "--flow-decision-border", "--flow-decision-label"},
		} {
			shape := shapes[part.node]
			fill, stroke := mermaidAttribute(shape, "fill"), mermaidAttribute(shape, "stroke")
			if fill == "" || fill != vars[part.fill] || stroke == "" || stroke != vars[part.border] || seen[fill] || labels[part.caption] != ui.Hex(vars[part.ink]) {
				t.Fatalf("%s has fill %q, stroke %q and ink %v (dark=%v)", part.node, fill, stroke, labels[part.caption], theme.dark)
			}
			seen[fill] = true
			requireMermaidContrast(t, part.caption, labels[part.caption], ui.Hex(fill), 4.5)
			requireMermaidContrast(t, part.node+" outline", ui.Hex(stroke), ui.Hex(fill), 3)
		}
		inks := map[ui.Color]bool{labels["Process"]: true, labels["Validate"]: true, labels["Prepare"]: true, labels["Shared"]: true}
		if theme.dark && len(inks) != 4 || !theme.dark && len(inks) != 1 {
			t.Errorf("dark=%v: unexpected default label colors %v", theme.dark, inks)
		}
		paint := mermaidAttribute(shapes["C"], "style")
		if labels["Input"] != ui.Hex("#974862") || !strings.Contains(paint, "fill:#f9f2e9") || !strings.Contains(paint, "fill-opacity:0.12") {
			t.Fatalf("authored class text or translucent fill lost: %v, %q", labels["Input"], paint)
		}
		requireMermaidContrast(t, "translucent entry", labels["Schedule"], ui.Hex("#f9f2e9").Alpha(.12).Over(style.bg), 4.5)
		custom := shapes["Custom"]
		if !strings.Contains(mermaidAttribute(custom, "style"), "stroke:#ab3470") && mermaidAttribute(custom, "stroke") != "#ab3470" {
			t.Fatal("decision styling overwrote the authored stroke")
		}
		if mermaidAttribute(custom, "fill") != vars["--flow-decision"] || labels["Override"] != ui.Hex(vars["--flow-decision-label"]) {
			t.Fatal("a stroke-only override changed the decision fill or label")
		}
	}
}

func TestMermaidFlowchartKeepsSemantics(t *testing.T) {
	style := testMermaidStyle()
	const label = "مرحبابالجميع"
	unicode := renderMermaid(`flowchart LR;A["`+strings.Repeat("中", 23)+label+` 👩🏽‍💻"]-->B`, style)
	if unicode.err != "" || !slices.ContainsFunc(unicode.labels, func(run mermaidLabel) bool { return strings.Contains(run.text, label) }) || !slices.ContainsFunc(unicode.labels, func(run mermaidLabel) bool { return strings.Contains(run.text, "👩🏽‍💻") }) {
		t.Fatalf("the wrapper split an Arabic word or emoji cluster: %+v (%s)", unicode.labels, unicode.err)
	}
	for _, test := range []struct{ name, source, want string }{
		{"subgraph", "flowchart TB\nsubgraph External\nA[one]-->B[two]\nend", "External"},
		{"circle", "flowchart LR\nA((start))-->B{ok}", "start"},
		{"inline formatting", "flowchart TB\nA[\"`**Bold**`\"] --> B[ok]", "Bold"},
		{"named and numeric entities", `flowchart LR;A["#quot;Hi#quot; #35;"]-->B`, `"Hi" #`},
		{"inline accessible title", "flowchart LR\nA[Label]-->B;accTitle: Overview", "Label"},
		{"inline accessible description", "flowchart LR\nA[Label]-->B;accDescr: More information", "Label"},
		{"inline tooltip", `flowchart LR;A[Label]-->B;click A doSomething "Details"`, "Label"},
		{"relative font size", "flowchart LR\nA[Label]-->B\nstyle A font-size:2em", "Label"},
		{"edge fill style", "flowchart LR\nA-->|Choice|B\nlinkStyle 0 fill:#ffee00,stroke:#f00", "Choice"},
		{"sequence", "sequenceDiagram\nA->>B: message", "message"},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagram := renderMermaid(test.source, style)
			if diagram.err != "" || bytes.Contains(diagram.shapes, []byte("fiveleaf-elk")) {
				t.Fatalf("expected the full Mermaid renderer: %s", diagram.err)
			}
			if !slices.ContainsFunc(diagram.labels, func(label mermaidLabel) bool { return strings.Contains(label.text, test.want) }) {
				t.Fatalf("missing %q in %+v", test.want, diagram.labels)
			}
			if test.name == "inline tooltip" && !bytes.Contains(diagram.shapes, []byte("<title>Details</title>")) {
				t.Fatal("the tooltip was lost")
			}
		})
	}
	for _, direction := range []string{"TB", "BT", "LR", "RL"} {
		diagram := renderMermaid("flowchart "+direction+"\nA[Left] --> B[Right]", style)
		if diagram.err != "" || !bytes.Contains(diagram.shapes, []byte("fiveleaf-elk")) || len(diagram.labels) != 2 {
			t.Fatalf("%s: %q, %d labels", direction, diagram.err, len(diagram.labels))
		}
		first, second := diagram.labels[0], diagram.labels[1]
		if direction == "TB" && first.y >= second.y || direction == "BT" && first.y <= second.y || direction == "LR" && first.x >= second.x || direction == "RL" && first.x <= second.x {
			t.Fatalf("%s: %q at %v,%v and %q at %v,%v", direction, first.text, first.x, first.y, second.text, second.x, second.y)
		}
	}
}

func TestCSSResolve(t *testing.T) {
	vars := map[string]string{"--a": "var(--b)", "--b": "#112233"}
	for in, want := range map[string]string{
		"var(--a)":                               "#112233",
		"var(--x,#fff)":                          "#fff",
		"var(--x, var(--a))":                     "#112233",
		"1px solid var(--b)":                     "1px solid #112233",
		"color-mix(in srgb,#ffffff 25%,#000000)": "#404040",
		"none":                                   "none",
	} {
		if got := cssResolve(in, vars); got != want {
			t.Errorf("cssResolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTransform(t *testing.T) {
	m := parseTransform("translate(10, 5) scale(2)")
	if x, y := m.apply(1, 1); x != 12 || y != 7 {
		t.Errorf("translate scale: %v,%v", x, y)
	}
	m = parseTransform("rotate(90 10 10)")
	if x, y := m.apply(20, 10); math.Abs(float64(x-10)) > 1e-4 || math.Abs(float64(y-20)) > 1e-4 {
		t.Errorf("rotate: %v,%v", x, y)
	}
}

const mermaidMarkdown = "# Diagrams\n\n```mermaid\nflowchart LR\n  A[Write **Markdown**] --> B{Has a diagram?}\n  B -- yes --> C[Draw it]\n  B -- no --> D[Show text]\n  C --> E((Done))\n```\n\n```mermaid\nsequenceDiagram\n  participant E as Editor\n  participant P as Pele\n  E->>P: render(source)\n  P-->>E: SVG\n  Note over E,P: labels drawn by FiveLeaf\n```\n\n```mermaid\nsequenceDiagram\n  A->>B hi\n```\n"

// TestMermaidPreview: a Markdown preview draws its blocks of Mermaid as
// diagrams, says why one cannot be drawn, shows one's source on asking,
// and shows them all as source with previewMermaid off.
func TestMermaidPreview(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("diagrams.md")
	os.WriteFile(path, []byte(mermaidMarkdown), 0o644)
	a.openFile(path, true)
	d := a.activeEditor()
	a.setPreview(d, true)
	hs.until("diagrams", func() bool { return d.preview != nil && d.preview.loaded })
	hs.settle()
	m := d.preview.markdown
	var diagrams []markdownBlock
	for _, b := range m.blocks {
		if b.diagram != nil {
			diagrams = append(diagrams, b)
		}
	}
	if len(diagrams) != 3 || diagrams[0].diagram.err != "" || diagrams[1].diagram.err != "" || diagrams[2].diagram.err == "" {
		t.Fatalf("diagrams %+v", diagrams)
	}
	hs.shot("mermaid-markdown")
	if !hs.tt.HasText(locales.MermaidCannotDraw(diagrams[2].diagram.err)) {
		t.Error("no error under the diagram that cannot be drawn")
	}
	if hs.tt.HasText("flowchart LR") {
		t.Error("the source of a diagram shows")
	}
	m.sources = map[int]bool{diagrams[0].line: true}
	hs.settle()
	if !hs.tt.HasText("flowchart LR") {
		t.Error("the source asked for does not show")
	}

	a.settings.PreviewMermaid = false
	hs.until("source", func() bool {
		for _, b := range d.preview.markdown.blocks {
			if b.diagram != nil {
				return false
			}
		}
		return true
	})
}

// TestMermaidFilePreview: a .mmd file previews as its diagram, drawn
// again as it is edited.
func TestMermaidFilePreview(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	path := hs.path("flow.mmd")
	os.WriteFile(path, []byte("stateDiagram-v2\n  [*] --> Idle\n  Idle --> Busy: start\n  Busy --> Idle: done\n  Busy --> [*]\n"), 0o644)
	a.openFile(path, true)
	a.run("view.togglePreview")
	d := a.activeEditor()
	hs.until("diagram", func() bool { return d.preview != nil && d.preview.diagram != nil })
	hs.settle()
	hs.shot("mermaid-file")
	first := d.preview.diagram
	d.buf.Replace(d.buf.Len(), d.buf.Len(), "  Idle --> Off: stop\n")
	hs.until("drawn again", func() bool { return d.preview.diagram != first })
	if d.preview.err != "" {
		t.Fatal(d.preview.err)
	}

	// It copies as an image, and exports as PNG and SVG.
	hs.until("drawn", func() bool { return d.preview.diagram != nil && !d.preview.drawing && d.preview.wait == nil })
	a.run("mermaid.copyImage")
	if !bytes.HasPrefix(hs.h.image, []byte("\x89PNG")) {
		t.Fatalf("copied %d bytes, not a PNG", len(hs.h.image))
	}
	for _, ext := range []string{"png", "svg"} {
		hs.h.saveTo = hs.path("out." + ext)
		a.run("mermaid.export" + strings.ToUpper(ext))
		if !slices.Contains(hs.h.asked, "export flow."+ext) {
			t.Errorf("asked %q", hs.h.asked)
		}
		if b, err := os.ReadFile(hs.h.saveTo); err != nil || len(b) == 0 {
			t.Errorf("exported %s: %v", ext, err)
		}
	}
}

// TestMermaidExport: a diagram copied or exported is drawn as it shows,
// on the theme's background, its SVG whole, with its labels as text.
func TestMermaidExport(t *testing.T) {
	st := testMermaidStyle()
	d := renderMermaid("flowchart LR\nA[Start] --> B{Ok?}\nB -- yes --> C[Done]\nstyle C fill:#fff", st)
	if d.err != "" {
		t.Fatal(d.err)
	}
	b, err := d.png()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	w, h := int(math.Ceil(float64(d.w+2*mermaidExportPad))), int(math.Ceil(float64(d.h+2*mermaidExportPad)))
	if got := img.Bounds().Size(); got.X != w*mermaidExportScale || got.Y != h*mermaidExportScale {
		t.Fatalf("PNG %v, want %dx%d", got, w*mermaidExportScale, h*mermaidExportScale)
	}
	if r, g, bl, _ := img.At(1, 1).RGBA(); uint8(r>>8) != st.bg.R || uint8(g>>8) != st.bg.G || uint8(bl>>8) != st.bg.B {
		t.Errorf("corner %v, want the background %v", img.At(1, 1), st.bg)
	}
	surface := ui.Hex(st.pelePalette(true)["--pele-surface"])
	wantPixel := color.RGBA{R: surface.R, G: surface.G, B: surface.B, A: 255}
	painted := 0
	for row := 0; row < img.Bounds().Dy(); row += 8 {
		for column := 0; column < img.Bounds().Dx(); column += 8 {
			if color.RGBAModel.Convert(img.At(column, row)) == wantPixel {
				painted++
			}
		}
	}
	if painted < 10 {
		t.Errorf("PNG contains only %d sampled process-surface pixels in %v", painted, surface)
	}
	svg := string(d.exportSVG())
	for _, want := range []string{">Start</text>", ">Done</text>", `fill="` + hexColor(st.bg) + `"`, `fill="` + hexColor(surface) + `"`, `fill="` + st.pelePalette(true)["--flow-node-label"] + `"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG has no %s", want)
		}
	}
	if strings.Contains(svg, "var(") {
		t.Error("SVG keeps CSS variables")
	}
	// Done's label reads on its white fill in the dark theme.
	if !slices.ContainsFunc(d.labels, func(label mermaidLabel) bool { return label.text == "Done" && label.color == st.bg }) {
		t.Errorf("Done drawn in another color than the background: %s", svg)
	}
	if _, err := ui.ParseSVG([]byte(svg)); err != nil {
		t.Errorf("SVG does not parse: %v", err)
	}
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("SVG is not XML: %v", err)
		}
	}
}

// TestMermaidOverlay: a diagram of a Markdown preview opens over the
// window from its button or a double click, zooms there, closes with
// Escape, and closes as its block stops drawing a diagram.
func TestMermaidOverlay(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	path := hs.path("diagrams.md")
	os.WriteFile(path, []byte(mermaidMarkdown), 0o644)
	a.openFile(path, true)
	d := a.activeEditor()
	a.setPreview(d, true)
	hs.until("diagrams", func() bool { return d.preview != nil && d.preview.loaded })
	hs.settle()
	box, ok := hs.tt.Find("mermaid")
	if !ok {
		t.Fatal("no diagram shows")
	}
	x, y := box.X+box.W/2, box.Y+box.H/2
	hs.tt.Move(x, y)
	hs.settle()
	if err := hs.tt.Click(locales.OpenDiagram()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	o := a.diagram
	if o == nil || !hs.tt.HasText(locales.CloseDiagram()) {
		t.Fatal("the diagram did not open over the window")
	}
	hs.shot("mermaid-overlay")
	if !hs.tt.HasText(locales.ExportDiagram()) {
		t.Error("the overlay has no export button")
	}
	if err := hs.tt.Click(locales.CopyAsImage()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !bytes.HasPrefix(hs.h.image, []byte("\x89PNG")) {
		t.Fatalf("copied %d bytes, not a PNG", len(hs.h.image))
	}
	if o.view.zoom != 0 {
		t.Fatalf("opened at zoom %v, not fitted", o.view.zoom)
	}
	a.run("image.zoomIn")
	if o.view.zoom == 0 {
		t.Error("zooming in left the diagram fitted")
	}
	hs.tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.diagram != nil {
		t.Fatal("Escape left the diagram open")
	}

	hs.tt.ClickAt(x, y)
	hs.tt.ClickAt(x, y)
	hs.settle()
	if a.diagram == nil {
		t.Fatal("a double click did not open the diagram")
	}
	a.settings.PreviewMermaid = false
	hs.until("closed", func() bool { return a.diagram == nil })
}

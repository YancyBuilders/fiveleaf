package highlight

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
)

func scriptKindAt(t *testing.T, file, source, word string) Kind {
	t.Helper()
	at := strings.Index(source, word)
	if at < 0 {
		t.Fatalf("missing %q in fixture", word)
	}
	row := strings.Count(source[:at], "\n")
	lineStart := strings.LastIndex(source[:at], "\n") + 1
	col := len([]rune(source[lineStart:at]))
	for _, token := range Lex(Lexer(file), source)[row].Tokens {
		if token.Start <= col && col < token.End {
			return token.Kind
		}
	}
	return Plain
}

func TestScriptFunctionsAreRecognized(t *testing.T) {
	for _, file := range []string{"example.ts", "example.js", "example.jsx"} {
		if got := scriptKindAt(t, file, "function describeTheme(value) { return value; }", "describeTheme"); got != Function {
			t.Errorf("%s: function name kind %v, want %v", file, got, Function)
		}
	}
}

func TestScriptIdentifierRoles(t *testing.T) {
	for _, c := range []struct {
		file, source, word string
		want               Kind
	}{
		{"a.ts", "interface Palette { tone: number }", "Palette", Type},
		{"a.ts", "interface Palette { tone: number }", "tone", Property},
		{"a.ts", "const config: Palette = { active: true };", "config", Binding},
		{"a.ts", "const config: Palette = { active: true };", "Palette", Type},
		{"a.ts", "const config: Palette = { active: true };", "active", Property},
		{"a.ts", "const config: Palette = { active: true };", "true", Constant},
		{"a.ts", "const config = { count: 3 };", "3", Number},
		{"a.ts", "const config = { record: source.current };", "source", Object},
		{"a.ts", "const config = { record: source.current };", "current", Property},
		{"a.ts", "function paint(input: Palette): string { return input.tone; }", "input", Parameter},
		{"a.ts", "function paint(input: Palette): string { return input.tone; }", "Palette", Type},
		{"a.ts", "function paint(input: Palette): string { return input.tone; }", "string", Type},
		{"a.ts", "function paint(input: Palette): string { return input.tone; }", "input.tone", Object},
		{"a.ts", "function paint(input: Palette): string { return input.tone; }", "tone", Property},
		{"a.ts", "console.log(paint(config));", "console", Object},
		{"a.ts", "console.log(paint(config));", "log", Function},
		{"a.ts", "console.log(paint(config));", "paint", Function},
		{"a.ts", "console.log(paint(config));", "config", Variable},
		{"a.ts", "const result = `tone ${source.tone}, ${paint(config)}`;", "${", Interpolation},
		{"a.ts", "const result = `tone ${source.tone}, ${paint(config)}`;", "source", Object},
		{"a.ts", "const result = `tone ${source.tone}, ${paint(config)}`;", "tone}", Property},
		{"a.ts", "const result = `tone ${source.tone}, ${paint(config)}`;", "paint", Function},
		{"a.ts", "const 客户 = {名字: 'Ada'};", "客户", Binding},
		{"a.ts", "const 客户 = {名字: 'Ada'};", "名字", Property},
		{"a.js", "const render = (entry, flag) => entry.label;", "entry", Parameter},
		{"a.js", "const render = (entry, flag) => entry.label;", "flag", Parameter},
		{"a.js", "const render = entry => entry.label;", "entry", Parameter},
		{"a.js", "const first = 1, second = 2;", "second", Binding},
		{"a.js", "const {name, count: total} = source;", "name", Binding},
		{"a.js", "const {name, count: total} = source;", "count", Property},
		{"a.js", "const {name, count: total} = source;", "total", Binding},
		{"a.js", "const [first, second] = source;", "second", Binding},
		{"a.js", "const {name = fallback} = source;", "fallback", Variable},
		{"a.js", "function render({name: label}) { return label; }", "label", Parameter},
		{"a.js", "if (source.active) { render(source); }", "source.active", Object},
		{"a.js", "if (source.active) { render(source); }", "active", Property},
		{"a.js", "const source = data?.record;", "data", Object},
		{"a.js", "const source = data?.record;", "record", Property},
		{"a.js", "const re = /function f\\(x\\)/;", "function", String},
		{"a.js", "const text = 'function fake(value)';", "fake", String},
		{"a.js", "// function fake(value)\nconst real = 1;", "fake", Comment},
		{"a.jsx", "const view = <Card title='name'>Hello {source.name}</Card>;", "Card", Tag},
		{"a.jsx", "const view = <Card title='name'>Hello {source.name}</Card>;", "title", Attribute},
		{"a.jsx", "const view = <Card title='name'>Hello {source.name}</Card>;", "Hello", Plain},
		{"a.jsx", "const view = <Card title='name'>Hello {source.name}</Card>;", "source", Object},
		{"a.tsx", "const view = <Card title='name'>{source.name}</Card>;", "Card", Tag},
		{"a.tsx", "const view = <Card title='name'>{source.name}</Card>;", "title", Attribute},
		{"a.jsx", "const view = <>Hello {source.name}</>;", "Hello", Plain},
		{"a.jsx", "const view = <>return true</>;", "return", Plain},
		{"a.jsx", "const view = <>return true</>;", "true", Plain},
		{"a.jsx", "const view = <><>inner</>outer</>; const after = 1;", "outer", Plain},
		{"a.tsx", "const view = <><>inner</>outer</>; const after = 1;", "outer", Plain},
		{"a.jsx", "const view = <>before<>inner</></>; const after = 1;", "after", Binding},
		{"a.tsx", "const view = <>before<>inner</></>; const after = 1;", "after", Binding},
		{"a.jsx", "const view = <Card ok={count > 0} />; const after = 1;", "after", Binding},
		{"a.tsx", "const view = <Card ok={count > 0} />; const after = 1;", "after", Binding},
		{"a.ts", "const value = identity<number>(42);\nconst after = 1;", "after", Binding},
		{"a.ts", "const value = identity<Result<User>>(42);\nconst after = 1;", "after", Binding},
		{"a.jsx", "const view = <Card child={<Inner />}>Hello</Card>;", "Hello", Plain},
		{"a.jsx", "const view = <Card child={<Inner />}>Hello</Card>; const after = 1;", "after", Binding},
		{"a.ts", "type User = string\nconst result = fetchUser()", "fetchUser", Function},
	} {
		t.Run(c.file+"/"+c.source+"/"+c.word, func(t *testing.T) {
			if got := scriptKindAt(t, c.file, c.source, c.word); got != c.want {
				t.Errorf("kind = %v, want %v", got, c.want)
			}
		})
	}
}

func TestScriptMultilineAndNestedTemplates(t *testing.T) {
	source := "function transform(\n  entry: Record,\n  enabled = true,\n): string {\n  return `first ${({ key: entry.name }).key}\nnext ${transform(entry)}`;\n}\n"
	for _, c := range []struct {
		word string
		want Kind
	}{{"entry", Parameter}, {"Record", Type}, {"enabled", Parameter}, {"true", Constant}, {"entry.name", Object}, {"name", Property}, {"key}\n", Property}, {"next", String}, {"transform(entry)", Function}} {
		if got := scriptKindAt(t, "a.ts", source, c.word); got != c.want {
			t.Errorf("%s: kind %v, want %v", c.word, got, c.want)
		}
	}
	for _, file := range []string{"a.ts", "a.js", "a.jsx", "a.tsx"} {
		it, err := Lexer(file).Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, source)
		if err != nil {
			t.Fatal(err)
		}
		var restored strings.Builder
		for token := it(); token != chroma.EOF; token = it() {
			restored.WriteString(token.Value)
		}
		if restored.String() != source {
			t.Fatalf("%s: adapter changed source text", file)
		}
	}
}

func TestScriptAdjacentNestedInterpolations(t *testing.T) {
	source := "const a = `${first}${`\n${second}`}`, b = 1;\n"
	for _, file := range []string{"a.js", "a.ts"} {
		if got := scriptKindAt(t, file, source, "b ="); got != Binding {
			t.Errorf("%s: binding after nested template = %v, want %v", file, got, Binding)
		}
		if lines := Lex(Lexer(file), source); !lines[0].open {
			t.Errorf("%s: template newline lost its continuing context", file)
		}
	}
}

func TestScriptRelexKeepsRoles(t *testing.T) {
	for _, file := range []string{"a.js", "a.ts"} {
		source := "const settings = { value: true };\n" + strings.Repeat("// padding\n", 40) + "function render(\n  input,\n  flag = true\n) {\n" + strings.Repeat("  // padding\n", 40) + "  const text = `hello ${({name: input.name}).name}\nworld ${flag}`;\n  return text;\n}\n"
		l := Lexer(file)
		src := newSource(strings.Split(source, "\n"), 1)
		prev := lexRange(l, src, 0, len(src.lines))
		for _, replace := range [][2]string{{"const settings", "let settings"}, {"input,", "record,"}, {"input.name", "record.name"}, {"function render(", "function render"}} {
			source = strings.Replace(source, replace[0], replace[1], 1)
			src = newSource(strings.Split(source, "\n"), src.version+1)
			prev = relexLikeLexingAll(t, l, src, prev)
		}
	}
}

func TestScriptJSXRelexKeepsRoles(t *testing.T) {
	for _, file := range []string{"a.jsx", "a.tsx"} {
		source := "const view = <Card ok={count > 0} child={<Inner />}>\n<>\n" + strings.Repeat("Hello return true\n", 40) + "{source.name}\n</>\n</Card>;\nconst after = fetchUser();\n"
		l := Lexer(file)
		src := newSource(strings.Split(source, "\n"), 1)
		prev := lexRange(l, src, 0, len(src.lines))
		for _, replace := range [][2]string{{"count > 0", "count >= 0"}, {"Hello return true", "Hello class false"}, {"{source.name}", "{source.label}"}, {"child={<Inner />}", "child={<>Inner</>}"}} {
			source = strings.Replace(source, replace[0], replace[1], 1)
			src = newSource(strings.Split(source, "\n"), src.version+1)
			prev = relexLikeLexingAll(t, l, src, prev)
		}
	}
}

func TestScriptHighlighterViewFirst(t *testing.T) {
	source := "function render(input) {\n  return `name ${input.name}`;\n}\n"
	lines := strings.Split(strings.Repeat(source, 500), "\n")
	passes := make(chan []Line, 8)
	var h *Highlighter
	h = New(Lexer("a.ts"), func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		passes <- h.lines
	})
	defer h.Close()
	src := newSource(lines, 1)
	h.Request(src, 1000, 1040)
	next := func() []Line {
		t.Helper()
		select {
		case lines := <-passes:
			return lines
		case <-time.After(5 * time.Second):
			t.Fatal("script highlighter did not publish its next pass")
			return nil
		}
	}
	if first := next(); len(first) >= len(lines) || len(first) < 41 {
		t.Fatalf("first pass of %d lines, of %d", len(first), len(lines))
	}
	if all := next(); len(all) != len(lines) {
		t.Fatalf("second pass of %d lines, of %d", len(all), len(lines))
	}
	if !slices.Equal(h.Line(1001, lines[1001]), lexRange(Lexer("a.ts"), src, 0, len(lines))[1001].Tokens) {
		t.Error("visible tokens differ from lexing all")
	}
}

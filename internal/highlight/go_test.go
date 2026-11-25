package highlight

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestGoIdentifierRoles(t *testing.T) {
	for _, tc := range []struct {
		source, word string
		want         Kind
	}{
		{"package main\n", "main", Plain},
		{"import alias \"sync\"\n", "alias", Plain},
		{"import (\n alias \"sync\"\n)\n", "alias", Plain},
		{"func Run(concurrencyNum int, handlers ...func() error) error {", "Run", Function},
		{"func Run(concurrencyNum int, handlers ...func() error) error {", "concurrencyNum", Parameter},
		{"func Run(concurrencyNum int, handlers ...func() error) error {", "handlers", Parameter},
		{"func Run(concurrencyNum int, handlers ...func() error) error {", "int", BuiltinType},
		{"func Run(concurrencyNum int, handlers ...func() error) error {", "error", BuiltinType},
		{"func f(first, second *sync.WaitGroup) {}", "first", Parameter},
		{"func f(first, second *sync.WaitGroup) {}", "second", Parameter},
		{"func f(first, second *sync.WaitGroup) {}", "sync", Type},
		{"func f(first, second *sync.WaitGroup) {}", "WaitGroup", Type},
		{"func (pool *Pool) Run(value Value) Result {", "pool", Parameter},
		{"func (pool *Pool) Run(value Value) Result {", "Pool", Type},
		{"func (pool *Pool) Run(value Value) Result {", "Run", Function},
		{"func (pool *Pool) Run(value Value) Result {", "value", Parameter},
		{"func (pool *Pool) Run(value Value) Result {", "Value", Type},
		{"func (pool *Pool) Run(value Value) Result {", "Result", Type},
		{"var wg sync.WaitGroup", "wg", Variable},
		{"var wg sync.WaitGroup", "sync", Type},
		{"var wg sync.WaitGroup", "WaitGroup", Type},
		{"var (\n once sync.Once\n err error\n)", "once", Variable},
		{"var (\n once sync.Once\n err error\n)", "Once", Type},
		{"var (\n once sync.Once\n err error\n)", "err error", Variable},
		{"var a, b []Value", "b", Variable},
		{"var a, b []Value", "Value", Type},
		{"var a, b = first, second.Call()", "second", Variable},
		{"var a, b = first, second.Call()", "Call", Function},
		{"var x = outer(inner(arg))", "inner", Function},
		{"var x = outer(inner(arg))", "arg", Variable},
		{"var x = f(a + b)", "b", Variable},
		{"}\nvar x = outer(inner(arg))", "arg", Variable},
		{"type Alias = pkg.Value", "pkg", Type},
		{"type Alias = pkg.Value", "Value", Type},
		{"pool, _ := ants.NewPool(concurrencyNum)", "pool", Variable},
		{"pool, _ := ants.NewPool(concurrencyNum)", "ants", Variable},
		{"pool, _ := ants.NewPool(concurrencyNum)", "NewPool", Function},
		{"pool, _ := ants.NewPool(concurrencyNum)", "concurrencyNum", Variable},
		{"pool, _ := ants.NewPool(concurrencyNum)", ":=", Assignment},
		{"buf = make([]byte, 1024)", "make", BuiltinFunction},
		{"buf = make([]byte, 1024)", "byte", BuiltinType},
		{"buf = make([]byte, 1024)", "=", Assignment},
		{"count := int(value)", "int", BuiltinType},
		{"count := float32(value)", "float32", BuiltinType},
		{"count := object.int(value)", "int", Function},
		{"count := object.make(value)", "make", Function},
		{"if e := recover(); e != nil {", "recover", BuiltinFunction},
		{"if e := recover(); e != nil {", "!=", Operator},
		{"if e := recover(); e != nil {", "nil", Constant},
		{"runtime.Stack(buf, false)", "false", Constant},
		{"value += 2", "+=", Assignment},
		{"value ^= 2", "^=", Assignment},
		{"type Worker struct { Name string; Group *sync.WaitGroup }", "Worker", Type},
		{"type Worker struct { Name string; Group *sync.WaitGroup }", "Group", Variable},
		{"type Worker struct { Name string; Group *sync.WaitGroup }", "WaitGroup", Type},
		{"type Handler func(context.Context, *Request) error", "context", Type},
		{"type Handler func(context.Context, *Request) error", "Request", Type},
		{"type Callback func()\nvar x = call(arg)", "arg", Variable},
		{"type Callback func()\nvar x = obj.Method(value)", "value", Variable},
		{"type Worker interface { Run(value Value) error }", "Run", Function},
		{"type Worker interface { Run(value Value) error }", "value", Parameter},
		{"func f(值 int, 处理器 ...func() error) { 处理器[0]() }", "值", Parameter},
		{"func f(值 int, 处理器 ...func() error) { 处理器[0]() }", "处理器", Parameter},
		{"// var fake sync.Once\nvar real = \"func fake(x int)\"", "sync", Comment},
		{"var text = \"func fake(x int)\"", "fake", String},
		{"func unfinished(\n value *sync.WaitGroup,\n", "value", Parameter},
		{"func unfinished(\n value *sync.WaitGroup,\n", "WaitGroup", Type},
	} {
		t.Run(tc.source+"/"+tc.word, func(t *testing.T) {
			if got := scriptKindAt(t, "reference.go", tc.source, tc.word); got != tc.want {
				t.Errorf("kind = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGoAdapterPreservesSource(t *testing.T) {
	source := "package 测试\n/* 多行\n注释 */\nfunc 处理(值 string) {\n raw := `第一行\n第二行`\n 值 += raw\n}\n"
	it, err := Lexer("reference.go").Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, source)
	if err != nil {
		t.Fatal(err)
	}
	var restored strings.Builder
	for tok := it(); tok != chroma.EOF; tok = it() {
		restored.WriteString(tok.Value)
	}
	if restored.String() != source {
		t.Fatal("Go adapter changed source text")
	}
	if got := scriptKindAt(t, "reference.go", source, "第二行"); got != String {
		t.Errorf("raw string line kind = %v, want String", got)
	}
}

func TestGoRelexKeepsRoles(t *testing.T) {
	source := "package sample\n" + strings.Repeat("// padding\n", 40) +
		"func Run(\n first, second *sync.WaitGroup,\n handlers ...func() error,\n) error {\n" +
		strings.Repeat(" // padding\n", 40) +
		" var (\n  once sync.Once\n  text = `raw\nsecond line`\n )\n first.Add(1)\n return handlers[0]()\n}\n" +
		strings.Repeat("var value = 1\n", 180)
	l := Lexer("reference.go")
	src := newSource(strings.Split(source, "\n"), 1)
	prev := lexRange(l, src, 0, len(src.lines))
	for _, edit := range [][2]string{{"first, second", "first, other"}, {"sync.Once", "sync.WaitGroup"}, {"first.Add(1)", "other.Add(2)"}, {"second line", "第二行"}, {"func Run(", "func Run"}} {
		source = strings.Replace(source, edit[0], edit[1], 1)
		src = newSource(strings.Split(source, "\n"), src.version+1)
		prev = relexLikeLexingAll(t, l, src, prev)
	}
}

func TestGoContinuationContexts(t *testing.T) {
	for _, tc := range []struct {
		source string
		open   []bool
	}{
		{"func f() {\n return call(arg)\n}\n", []bool{false, false, false}},
		{"var x = outer(\n inner(arg),\n)\n", []bool{true, true, false}},
		{"func f(\n input pkg.Type,\n) (\n result pkg.Type,\n) {\n return input\n}\n", []bool{true, true, true, true, false, false, false}},
		{"var x = struct {\n value pkg.Type\n}{\n value: source,\n}\n", []bool{true, true, true, true, false}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			lines := Lex(Lexer("reference.go"), tc.source)
			for i, open := range tc.open {
				if lines[i].open != open {
					t.Errorf("line %d continuation = %v, want %v", i, lines[i].open, open)
				}
			}
		})
	}
}

func TestGoRelexMultilineContexts(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		edits        [][2]string
	}{
		{
			"initializer-past-patch-boundary",
			"package sample\nvar cached = outer(\n" + strings.Repeat(" inner(arg),\n", 200) + ")\nvar after = source\n",
			[][2]string{{"inner(arg)", "inner(next)"}, {")\nvar after", "\nvar after"}, {"\nvar after", ")\nvar after"}},
		},
		{
			"composite-and-return-types",
			"package sample\nfunc work(\n input pkg.Value,\n) (\n result pkg.Value,\n err error,\n) {\n" +
				strings.Repeat(" consume(input)\n", 40) +
				" var value = struct {\n  member pkg.Value\n }{\n  member: input,\n }\n" +
				" result = wrap(\n  convert(value),\n )\n return result, err\n}\n",
			[][2]string{{"result pkg.Value", "result pkg.Other"}, {"member pkg.Value", "member pkg.Other"}, {"member: input", "member: result"}, {"convert(value)", "convert(input)"}, {"func work(", "func work"}, {"func work\n", "func work(\n"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source + strings.Repeat("var value = 1\n", 180)
			l := Lexer("reference.go")
			src := newSource(strings.Split(source, "\n"), 1)
			prev := lexRange(l, src, 0, len(src.lines))
			for _, edit := range tc.edits {
				if !strings.Contains(source, edit[0]) {
					t.Fatalf("edit target %q missing", edit[0])
				}
				source = strings.Replace(source, edit[0], edit[1], 1)
				src = newSource(strings.Split(source, "\n"), src.version+1)
				prev = relexLikeLexingAll(t, l, src, prev)
			}
		})
	}
}

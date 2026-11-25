package highlight

import (
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
)

const (
	goBuiltinType chroma.TokenType = 90100 + iota
	goBuiltinFunction
	goAssignment
)

var goLexers sync.Map

func goSource(l chroma.Lexer) chroma.Lexer {
	if cached, ok := goLexers.Load(l); ok {
		return cached.(chroma.Lexer)
	}
	result, _ := goLexers.LoadOrStore(l, &goLexer{l})
	return result.(chroma.Lexer)
}

type goLexer struct{ chroma.Lexer }

func (l *goLexer) Tokenise(options *chroma.TokeniseOptions, text string) (chroma.Iterator, error) {
	it, err := l.Lexer.Tokenise(options, text)
	if err != nil {
		return nil, err
	}
	s := &goStream{it: it, stack: []goFrame{{}}}
	return s.next, nil
}

type goFrame struct {
	close, role, decl string
	phase             int // field names, type, or initializer
	function, results bool
	composite         bool
}

type goStream struct {
	it    chroma.Iterator
	raw   chroma.Token
	queue []chroma.Token
	stack []goFrame
	prev  chroma.Token
}

func (s *goStream) atom() chroma.Token {
	if s.raw.Value == "" {
		s.raw = s.it()
	}
	tok := s.raw
	if tok.Type == chroma.Text || tok.Type == chroma.TextWhitespace {
		if i := strings.IndexByte(tok.Value, '\n'); i >= 0 {
			if i == 0 {
				i++
			}
			tok.Value, s.raw.Value = tok.Value[:i], tok.Value[i:]
			return tok
		}
	}
	s.raw = chroma.Token{}
	return tok
}

func (s *goStream) take() chroma.Token {
	if len(s.queue) == 0 {
		return s.atom()
	}
	tok := s.queue[0]
	s.queue = s.queue[1:]
	return tok
}

func (s *goStream) ahead() []chroma.Token {
	var out []chroma.Token
	for i, bytes := 0, 0; i < 64 && bytes < 4096; i++ {
		if i == len(s.queue) {
			s.queue = append(s.queue, s.atom())
		}
		tok := s.queue[i]
		if tok == chroma.EOF {
			break
		}
		bytes += len(tok.Value)
		if !scriptTrivia(tok) {
			out = append(out, tok)
		}
	}
	return out
}

func goName(t chroma.Token) bool {
	return t.Type.InCategory(chroma.Name) || t.Type == chroma.KeywordType
}

// Named parameters may share a type: a, b *T. A qualified name or a
// generic type without a following type expression is an unnamed field.
func goNamedField(ahead []chroma.Token) bool {
	for len(ahead) >= 2 && ahead[0].Value == "," && goName(ahead[1]) {
		ahead = ahead[2:]
	}
	if len(ahead) == 0 {
		return false
	}
	if ahead[0].Value == "[" {
		depth := 0
		for i, tok := range ahead {
			switch tok.Value {
			case "[":
				depth++
			case "]":
				depth--
				if depth == 0 {
					return i == 1 || i+1 < len(ahead) && goTypeStart(ahead[i+1])
				}
			}
		}
		return true
	}
	return goTypeStart(ahead[0])
}

func goTypeStart(tok chroma.Token) bool {
	if goName(tok) {
		return true
	}
	switch tok.Value {
	case "*", "[", "...", "map", "chan", "<-", "func", "struct", "interface":
		return true
	}
	return false
}

func (f *goFrame) fields() bool {
	return f.role == "params" || f.role == "fields" || f.role == "vars" || f.role == "types" || f.decl == "var" || f.decl == "const" || f.decl == "type"
}

func (f *goFrame) typeMode() bool {
	return f.role == "type" || f.results || f.fields() && f.phase == 1
}

func (s *goStream) next() chroma.Token {
	tok := s.take()
	if tok == chroma.EOF {
		return tok
	}
	f := &s.stack[len(s.stack)-1]
	if scriptTrivia(tok) {
		if tok.Value == "\n" {
			if s.prev.Value != "," && s.prev.Value != "=" && s.prev.Value != ":=" && !s.prev.Type.InCategory(chroma.Operator) {
				if f.role == "vars" || f.role == "types" || f.role == "fields" {
					f.phase = 0
				}
				f.decl = ""
				f.results = false
			}
			for _, frame := range s.stack {
				if frame.role != "" || frame.decl != "" || frame.function || frame.results || frame.composite {
					tok.Type = scriptContinue
					break
				}
			}
		}
		return tok
	}
	if tok.Type.InSubCategory(chroma.LiteralString) || tok.Type.InSubCategory(chroma.LiteralNumber) || tok.Type == chroma.KeywordConstant {
		s.prev = tok
		return tok
	}
	if tok.Value == "^" && len(s.queue) > 0 && s.queue[0].Value == "=" {
		tok.Value += s.take().Value
	} else if tok.Value == "^" && len(s.queue) == 0 {
		s.queue = append(s.queue, s.atom())
		if s.queue[0].Value == "=" {
			tok.Value += s.take().Value
		}
	}
	switch tok.Value {
	case "var", "const", "type", "import", "package":
		f.decl, f.phase = tok.Value, 0
	case "func":
		f.function, f.results = true, false
		if f.fields() {
			f.phase = 1
		}
	case "struct", "interface":
		f.composite = true
	case "(", "[", "{":
		role, close := "", map[string]string{"(": ")", "[": "]", "{": "}"}[tok.Value]
		switch {
		case tok.Value == "(" && f.function:
			role, f.function = "params", false
		case tok.Value == "(" && f.results:
			role, f.results = "params", false
		case tok.Value == "(" && f.phase == 0 && (f.decl == "var" || f.decl == "const"):
			role, f.decl = "vars", ""
		case tok.Value == "(" && f.phase == 0 && f.decl == "type":
			role, f.decl = "types", ""
		case tok.Value == "(" && f.decl == "import":
			role, f.decl = "imports", ""
		case tok.Value == "{" && f.composite:
			role, f.composite = "fields", false
		case tok.Value != "{" && (f.typeMode() || f.function):
			role = "type"
		}
		if tok.Value == "{" {
			f.results, f.function = false, false
		}
		s.stack = append(s.stack, goFrame{close: close, role: role})
	case ")", "]", "}":
		if len(s.stack) > 1 && tok.Value == f.close {
			params := f.role == "params"
			s.stack = s.stack[:len(s.stack)-1]
			if params {
				s.stack[len(s.stack)-1].results = true
			}
		}
	case ",", ";":
		if f.fields() && (f.phase != 2 || tok.Value == ";") {
			f.phase = 0
		}
		f.results = false
		if tok.Value == ";" {
			f.decl = ""
		}
	case "=", ":=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", "&^=":
		tok.Type = goAssignment
		if f.fields() {
			f.phase = 2
			if f.decl == "type" || f.role == "types" {
				f.phase = 1
			}
		}
	default:
		if goName(tok) {
			switch {
			case f.decl == "package" || f.decl == "import" || f.role == "imports":
				tok.Type = chroma.NameOther
			case f.function:
				tok.Type = chroma.NameFunction
			case f.results && tok.Type == chroma.NameFunction:
				tok.Type, f.function, f.results = chroma.NameFunction, true, false
			case f.role == "fields" && f.phase == 0 && tok.Type == chroma.NameFunction:
				f.function = true
			case f.fields() && f.phase == 0:
				if f.decl == "type" || f.role == "types" {
					tok.Type, f.phase = chroma.NameClass, 1
				} else if f.decl == "var" || f.decl == "const" || f.role == "vars" || goNamedField(s.ahead()) {
					tok.Type = chroma.NameVariable
					if f.role == "params" {
						tok.Type = scriptParameter
					}
					f.phase = 1
				} else {
					f.phase = 1
					tok.Type = goTypeToken(tok)
				}
			case f.typeMode():
				tok.Type = goTypeToken(tok)
			case tok.Type == chroma.KeywordType:
				tok.Type = goBuiltinType
			case tok.Type == chroma.NameBuiltin && s.prev.Value == ".":
				tok.Type = chroma.NameFunction
			case tok.Type == chroma.NameBuiltin:
				tok.Type = goBuiltinFunction
				if goBuiltinTypeName(tok.Value) {
					tok.Type = goBuiltinType
				}
			case tok.Type == chroma.NameOther:
				tok.Type = chroma.NameVariable
			}
		}
	}
	s.prev = tok
	return tok
}

func goTypeToken(tok chroma.Token) chroma.TokenType {
	if tok.Type == chroma.KeywordType {
		return goBuiltinType
	}
	return chroma.NameClass
}

func goBuiltinTypeName(name string) bool {
	switch name {
	case "bool", "byte", "complex64", "complex128", "error", "float", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	}
	return false
}

package highlight

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
)

// These are private to the script adapter and never escape as Chroma tokens.
const (
	scriptParameter chroma.TokenType = 90000 + iota
	scriptBinding
	scriptProperty
	scriptInterpolation
	scriptBracket
	scriptObject
	scriptContinue
	scriptFragmentOpen
	scriptFragmentClose
)

var scriptLexers sync.Map

// script keeps Chroma's string, regexp, comment and JSX states, and adds
// the local identifier roles its JavaScript and TypeScript lexers omit.
func script(l chroma.Lexer) chroma.Lexer {
	switch l.Config().Name {
	case "TypeScript", "JavaScript", "react":
	default:
		return chroma.Coalesce(l)
	}
	if cached, ok := scriptLexers.Load(l); ok {
		return cached.(chroma.Lexer)
	}
	base := l
	if re, ok := l.(*chroma.RegexLexer); ok {
		if rules, err := re.Rules(); err == nil {
			rules = rules.Clone()
			if _, ok := rules["jsx"]; ok {
				rules["jsx"] = append([]chroma.Rule{
					{Pattern: `<>`, Type: scriptFragmentOpen},
					{Pattern: `</>`, Type: scriptFragmentClose},
				}, rules["jsx"]...)
			}
			// An object's closing brace inside ${...} belongs to the
			// object, not to the interpolation that contains it.
			rules["interp-inside"] = append([]chroma.Rule{{Pattern: `\{`, Type: chroma.Punctuation, Mutator: chroma.Push("script-braces")}}, rules["interp-inside"]...)
			rules["script-braces"] = []chroma.Rule{
				{Pattern: `\{`, Type: chroma.Punctuation, Mutator: chroma.Push()},
				{Pattern: `\}`, Type: chroma.Punctuation, Mutator: chroma.Pop(1)},
				chroma.Include("root"),
			}
			base = chroma.MustNewLexer(l.Config(), func() chroma.Rules { return rules })
		}
	}
	result := &scriptLexer{Lexer: chroma.Coalesce(base)}
	actual, _ := scriptLexers.LoadOrStore(l, result)
	return actual.(chroma.Lexer)
}

type scriptLexer struct{ chroma.Lexer }

func (l *scriptLexer) Tokenise(options *chroma.TokeniseOptions, text string) (chroma.Iterator, error) {
	it, err := l.Lexer.Tokenise(options, text)
	if err != nil {
		return nil, err
	}
	s := &scriptStream{atoms: scriptAtoms{it: it}}
	return s.next, nil
}

type scriptAtom struct {
	chroma.Token
	open bool // the newline continues the original multiline token
}

// Chroma sometimes returns `receiver.member`, `name() {` or ` : ` in
// one token. Split only code tokens; quoted text and JSX text stay text.
type scriptAtoms struct {
	it  chroma.Iterator
	raw chroma.Token
	at  int
}

func (a *scriptAtoms) next() scriptAtom {
	for a.at == len(a.raw.Value) {
		a.raw, a.at = a.it(), 0
		if a.raw == chroma.EOF {
			return scriptAtom{}
		}
	}
	start, value, kind := a.at, a.raw.Value, a.raw.Type
	r, n := utf8.DecodeRuneInString(value[start:])
	if r == '\n' {
		a.at += n
		return scriptAtom{Token: chroma.Token{Type: kind, Value: value[start:a.at]}, open: a.at < len(value) && kind != chroma.Text && kind != chroma.TextWhitespace}
	}
	if kind == chroma.LiteralStringInterpol {
		// Adjacent interpolations can be coalesced into `}${`. Keep each
		// delimiter separate so both transitions reach the context stack.
		a.at += n
		if strings.HasPrefix(value[start:], "${") {
			a.at = start + 2
		}
		return scriptAtom{Token: chroma.Token{Type: kind, Value: value[start:a.at]}}
	}
	if kind == scriptFragmentOpen || kind == scriptFragmentClose {
		a.at += 2 // <>
		if kind == scriptFragmentClose {
			a.at++ // </>
		}
		return scriptAtom{Token: chroma.Token{Type: kind, Value: value[start:a.at]}}
	}
	code := kind == chroma.NameOther || kind == chroma.KeywordType || kind == chroma.Punctuation || kind.InCategory(chroma.Operator) || kind == chroma.Error
	if !code {
		a.at = len(value)
		if end := strings.IndexByte(value[start:], '\n'); end >= 0 {
			a.at = start + end
		}
		part := value[start:a.at]
		if kind == chroma.Text && strings.TrimSpace(part) == ":" {
			// The TypeScript annotation rule puts its colon into Text.
			if r == ' ' || r == '\t' {
				a.at = start + strings.Index(part, ":")
			} else {
				a.at, kind = start+n, chroma.Punctuation
			}
		}
		return scriptAtom{Token: chroma.Token{Type: kind, Value: value[start:a.at]}}
	}
	a.at += n
	if scriptNameStart(r) {
		for a.at < len(value) {
			r, n = utf8.DecodeRuneInString(value[a.at:])
			if !scriptNamePart(r) {
				break
			}
			a.at += n
		}
		if kind != chroma.KeywordType {
			kind = chroma.NameOther
		}
	} else if unicode.IsDigit(r) {
		kind = chroma.LiteralNumberInteger
		for a.at < len(value) {
			r, n = utf8.DecodeRuneInString(value[a.at:])
			if !unicode.IsDigit(r) && r != '.' {
				break
			}
			a.at += n
		}
	} else if unicode.IsSpace(r) {
		kind = chroma.Text
		for a.at < len(value) && (value[a.at] == ' ' || value[a.at] == '\t') {
			a.at++
		}
	} else {
		kind = chroma.Punctuation
		if strings.ContainsRune("=+-*/%!~&|^<>", r) {
			kind = chroma.Operator
			for _, operator := range []string{"===", "!==", ">>>", "**=", "&&=", "||=", "??=", "=>", "==", "!=", "<=", ">=", "++", "--", "**", "&&", "||", "<<", ">>", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^="} {
				if strings.HasPrefix(value[start:], operator) {
					a.at = start + len(operator)
					break
				}
			}
		}
	}
	return scriptAtom{Token: chroma.Token{Type: kind, Value: value[start:a.at]}}
}

func scriptNameStart(r rune) bool { return r == '$' || r == '_' || unicode.IsLetter(r) }
func scriptNamePart(r rune) bool {
	return scriptNameStart(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

type scriptFrame struct {
	close    string
	role     string
	param    bool
	jsxDepth int
	binding  chroma.TokenType
}

type scriptStream struct {
	atoms scriptAtoms
	queue []scriptAtom
	stack []scriptFrame
	prev  chroma.Token
	// Declaration state is local to the surrounding delimiter depth.
	decl       string
	declDepth  int
	bind       bool
	typeName   bool
	typeBody   bool
	typeAlias  bool
	typeMode   bool
	function   bool
	functionID bool
	jsxDepth   int
	tags       []scriptTag
	generic    int
}

type scriptTag struct {
	depth   int
	closing bool
}

func (s *scriptStream) inTag() bool {
	return len(s.tags) != 0 && s.tags[len(s.tags)-1].depth == len(s.stack)
}

func (s *scriptStream) take() scriptAtom {
	if len(s.queue) == 0 {
		return s.atoms.next()
	}
	a := s.queue[0]
	s.queue = s.queue[1:]
	return a
}

// Peeking is bounded even on an unfinished parameter list or a long
// comment. A distant or ambiguous role falls back to an ordinary name.
func (s *scriptStream) ahead() []chroma.Token {
	out := make([]chroma.Token, 0, 8)
	bytes := 0
	for i := 0; i < 64 && bytes < 4096; i++ {
		if i == len(s.queue) {
			s.queue = append(s.queue, s.atoms.next())
		}
		tok := s.queue[i].Token
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

func scriptTrivia(t chroma.Token) bool {
	return t.Type.InCategory(chroma.Comment) || strings.TrimSpace(t.Value) == ""
}

func (s *scriptStream) top() *scriptFrame {
	if len(s.stack) == 0 {
		return nil
	}
	return &s.stack[len(s.stack)-1]
}

func (s *scriptStream) next() chroma.Token {
	a := s.take()
	tok := a.Token
	if tok == chroma.EOF {
		return tok
	}
	if s.generic > 0 && len(tok.Value) > 1 && strings.Trim(tok.Value, ">") == "" {
		s.queue = append([]scriptAtom{{Token: chroma.Token{Type: tok.Type, Value: tok.Value[1:]}}}, s.queue...)
		tok.Value = ">"
	}
	if tok.Type == scriptFragmentOpen || tok.Type == scriptFragmentClose {
		if tok.Type == scriptFragmentOpen {
			s.jsxDepth++
		} else {
			s.jsxDepth = max(0, s.jsxDepth-1)
		}
		tok.Type = scriptBracket
		s.prev = tok
		return tok
	}
	if tok.Type == chroma.NameTag {
		if s.generic != 0 {
			tok.Type = chroma.NameClass
		} else {
			s.tags = append(s.tags, scriptTag{depth: len(s.stack), closing: s.prev.Value == "/"})
		}
		s.prev = tok
		return tok
	}
	if tok.Type == chroma.NameAttribute {
		if s.generic != 0 {
			tok.Type = chroma.NameClass
		}
		s.prev = tok
		return tok
	}
	if s.jsxText() && !s.inTag() && strings.TrimSpace(tok.Value) != "" && tok.Value != "<" && tok.Value != "/" && tok.Value != "{" {
		tok.Type = chroma.Text
		s.prev = tok
		return tok
	}
	if scriptTrivia(tok) {
		if tok.Value == "\n" {
			if s.decl != "" && len(s.stack) == s.declDepth && !s.bind && s.prev.Value != "=" && s.prev.Value != "," {
				s.decl = ""
			}
			if a.open || len(s.stack) != 0 || s.jsxDepth != 0 || len(s.tags) != 0 || s.generic != 0 || s.decl != "" || s.function || s.typeName || s.typeMode {
				tok.Type = scriptContinue
			}
		}
		return tok
	}
	if tok.Type == chroma.LiteralStringBacktick {
		for i, r := range tok.Value {
			if r != '`' {
				continue
			}
			slashes := 0
			for j := i - 1; j >= 0 && tok.Value[j] == '\\'; j-- {
				slashes++
			}
			if slashes%2 != 0 {
				continue
			}
			if top := s.top(); top != nil && top.role == "template" {
				s.stack = s.stack[:len(s.stack)-1]
			} else {
				s.stack = append(s.stack, scriptFrame{close: "`", role: "template"})
			}
		}
		s.prev = tok
		return tok
	}
	if tok.Type.InSubCategory(chroma.LiteralString) && tok.Type != chroma.LiteralStringInterpol {
		s.prev = tok
		return tok
	}
	if tok.Type == chroma.LiteralStringInterpol {
		tok.Type = scriptInterpolation
		if tok.Value == "${" {
			s.stack = append(s.stack, scriptFrame{close: "}", role: "interpolation"})
		} else if top := s.top(); top != nil && top.role == "interpolation" {
			s.stack = s.stack[:len(s.stack)-1]
		}
		s.prev = tok
		return tok
	}
	if tok.Type.InCategory(chroma.Keyword) && tok.Type != chroma.KeywordType {
		s.keyword(tok.Value)
	} else if tok.Type == chroma.NameOther || tok.Type == chroma.KeywordType {
		tok.Type = s.name(tok)
	} else {
		s.punctuation(&tok)
	}
	s.prev = tok
	return tok
}

func (s *scriptStream) jsxText() bool {
	if s.jsxDepth == 0 {
		return false
	}
	for i := len(s.stack) - 1; i >= 0; i-- {
		if s.stack[i].role == "jsx" && s.stack[i].jsxDepth == s.jsxDepth {
			return false
		}
	}
	return true
}

func (s *scriptStream) keyword(word string) {
	switch word {
	case "function":
		s.function, s.functionID = true, true
	case "class", "interface", "enum":
		s.typeName, s.typeBody = true, true
	case "type":
		s.typeName, s.typeAlias = true, true
	case "const", "let", "var":
		s.decl, s.declDepth, s.bind, s.typeMode = word, len(s.stack), true, false
		s.typeAlias = false
	case "as", "implements", "extends", "new":
		s.typeMode = true
	}
}

func (s *scriptStream) name(tok chroma.Token) chroma.TokenType {
	if tok.Value == "true" || tok.Value == "false" || tok.Value == "null" || tok.Value == "undefined" || tok.Value == "NaN" || tok.Value == "Infinity" {
		return chroma.KeywordConstant
	}
	if s.typeName {
		s.typeName = false
		return chroma.NameClass
	}
	if s.functionID {
		s.functionID = false
		return chroma.NameFunction
	}
	next := s.ahead()
	value := func(i int) string {
		if i >= len(next) {
			return ""
		}
		return next[i].Value
	}
	if s.bind && len(s.stack) == s.declDepth {
		s.bind = false
		if s.decl == "const" {
			return scriptBinding
		}
		return chroma.NameVariable
	}
	if top := s.top(); top != nil && top.role == "parameters" && top.param {
		top.param = false
		return scriptParameter
	}
	if top := s.top(); top != nil && top.role == "bindings" && top.param {
		if top.close == "}" && value(0) == ":" {
			return scriptProperty
		}
		top.param = false
		return top.binding
	}
	if s.typeMode || tok.Type == chroma.KeywordType && scriptPrimitive(tok.Value) {
		return chroma.NameClass
	}
	if value(0) == "=>" {
		return scriptParameter
	}
	if value(0) == "(" || value(0) == "<" && scriptGenericCall(next) {
		return chroma.NameFunction
	}
	if value(0) == "." || value(0) == "?" && value(1) == "." {
		return scriptObject
	}
	if s.prev.Value == "." || value(0) == ":" || value(0) == "?" && value(1) == ":" {
		return scriptProperty
	}
	return chroma.NameVariable
}

func scriptPrimitive(word string) bool {
	switch word {
	case "string", "number", "boolean", "bool", "any", "never", "unknown", "object", "symbol", "bigint", "void":
		return true
	}
	return false
}

func scriptGenericCall(tokens []chroma.Token) bool {
	for i, tok := range tokens {
		if tok.Value == ">" {
			return i+1 < len(tokens) && tokens[i+1].Value == "("
		}
		if tok.Value == ";" || tok.Value == "{" || tok.Value == "=" {
			return false
		}
	}
	return false
}

func (s *scriptStream) parametersAhead() bool {
	depth := 1
	tokens := s.ahead()
	for i, tok := range tokens {
		switch tok.Value {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		}
		if depth == 0 {
			return i+1 < len(tokens) && (tokens[i+1].Value == "=>" || s.prev.Type == chroma.NameFunction && tokens[i+1].Value == "{")
		}
	}
	return false
}

func (s *scriptStream) punctuation(tok *chroma.Token) {
	switch tok.Value {
	case "(":
		role := "group"
		if s.function || s.parametersAhead() {
			role = "parameters"
		}
		s.stack = append(s.stack, scriptFrame{close: ")", role: role, param: true})
		s.function, s.functionID, s.typeMode = false, false, false
		tok.Type = scriptBracket
	case "{":
		role := "block"
		binding := s.bindingPattern()
		if binding != 0 {
			role = "bindings"
		} else if s.jsxText() || s.inTag() {
			role = "jsx"
		} else if s.typeBody {
			role = "type"
		} else if s.prev.Value == "=" || s.prev.Value == "(" || s.prev.Value == ":" || s.prev.Value == "return" || s.prev.Value == "[" || s.prev.Value == "," {
			role = "object"
		}
		s.stack = append(s.stack, scriptFrame{close: "}", role: role, jsxDepth: s.jsxDepth, param: true, binding: binding})
		s.typeBody, s.typeMode = false, false
		tok.Type = scriptBracket
	case "[":
		role, binding := "array", s.bindingPattern()
		if binding != 0 {
			role = "bindings"
		}
		s.stack = append(s.stack, scriptFrame{close: "]", role: role, binding: binding, param: true})
		tok.Type = scriptBracket
	case ")", "}", "]":
		if top := s.top(); top != nil && top.close == tok.Value {
			s.stack = s.stack[:len(s.stack)-1]
		}
		s.typeMode = false
		tok.Type = scriptBracket
	case ":":
		top := s.top()
		s.typeMode = s.prev.Type == scriptBinding || s.prev.Type == scriptParameter || s.prev.Value == ")" || top != nil && (top.role == "parameters" || top.role == "type")
	case "=":
		s.typeMode = s.typeAlias
	case ",":
		s.typeMode = false
		if top := s.top(); top != nil && (top.role == "parameters" || top.role == "bindings") {
			top.param = true
		}
		if s.decl != "" && len(s.stack) == s.declDepth {
			s.bind = true
		}
	case ";":
		s.decl, s.typeMode, s.typeAlias = "", false, false
	case "=>":
		s.typeMode = false
	case "<":
		next := s.ahead()
		if s.generic != 0 || s.prev.Type.InCategory(chroma.Name) && s.prev.Type != chroma.NameTag && !s.jsxText() && len(next) > 0 && next[0].Type == chroma.NameTag {
			s.generic++
		}
	case ">":
		if s.generic != 0 {
			s.generic--
		} else if s.inTag() {
			if s.tags[len(s.tags)-1].closing {
				s.jsxDepth = max(0, s.jsxDepth-1)
			} else if s.prev.Value != "/" {
				s.jsxDepth++
			}
			s.tags = s.tags[:len(s.tags)-1]
		}
	}
}

func (s *scriptStream) bindingPattern() chroma.TokenType {
	if s.bind && len(s.stack) == s.declDepth {
		s.bind = false
		if s.decl == "const" {
			return scriptBinding
		}
		return chroma.NameVariable
	}
	if top := s.top(); top != nil && top.param {
		switch top.role {
		case "parameters":
			top.param = false
			return scriptParameter
		case "bindings":
			top.param = false
			return top.binding
		}
	}
	return 0
}

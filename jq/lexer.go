package jq

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Lexer tokenises a jq source string, tracking line numbers so the parser can
// distinguish trailing comments (same line as preceding code) from leading
// comments (on their own line before the next expression).
type Lexer struct {
	src    string
	pos    int
	line   int // 1-based current line number (incremented on each \n)
	peeked *Token
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: src, line: 1}
}

func (l *Lexer) Next() Token {
	if l.peeked != nil {
		t := *l.peeked
		l.peeked = nil
		return t
	}
	return l.next()
}

func (l *Lexer) Peek() Token {
	if l.peeked == nil {
		t := l.next()
		l.peeked = &t
	}
	return *l.peeked
}

func (l *Lexer) next() Token {
	l.skipWhitespace()
	if l.pos >= len(l.src) {
		return l.tok(EOF, "")
	}

	at := Pos(l.pos)
	line := l.line
	ch := l.src[l.pos]

	if ch == '#' {
		return l.lexComment(at, line)
	}
	if ch == '"' {
		return l.lexString(at, line)
	}
	if ch >= '0' && ch <= '9' {
		return l.lexNumber(at, line)
	}
	if ch == '$' {
		return l.lexVar(at, line)
	}
	if ch == '@' {
		return l.lexFormat(at, line)
	}
	if isIdentStart(ch) {
		return l.lexIdent(at, line)
	}

	// Multi-char operators — longest match first.
	switch {
	case l.has("?//"):
		return l.advance(ALTALT, 3, line)
	case l.has("//="):
		return l.advance(ALTEQ, 3, line)
	case l.has("|="):
		return l.advance(PIPEEQ, 2, line)
	case l.has("+="):
		return l.advance(PLUSEQ, 2, line)
	case l.has("-="):
		return l.advance(MINUSEQ, 2, line)
	case l.has("*="):
		return l.advance(STAREQ, 2, line)
	case l.has("/="):
		return l.advance(SLASHEQ, 2, line)
	case l.has("%="):
		return l.advance(PERCENTEQ, 2, line)
	case l.has("//"):
		return l.advance(ALT, 2, line)
	case l.has("=="):
		return l.advance(EQ, 2, line)
	case l.has("!="):
		return l.advance(NEQ, 2, line)
	case l.has("<="):
		return l.advance(LTEQ, 2, line)
	case l.has(">="):
		return l.advance(GTEQ, 2, line)
	case l.has(".."):
		return l.advance(DOTDOT, 2, line)
	}

	l.pos++
	k, ok := singleChar[ch]
	if !ok {
		panic(fmt.Sprintf("jq lexer: unexpected character %q at offset %d", ch, at))
	}

	// A bare dot followed by an identifier is a FIELD token.
	if k == DOT && l.pos < len(l.src) && isIdentStart(l.src[l.pos]) {
		name := l.readIdent()
		return Token{Kind: FIELD, Text: "." + name, At: at, Line: line}
	}

	return Token{Kind: k, Text: string(ch), At: at, Line: line}
}

var singleChar = map[byte]Kind{
	'|': PIPE,
	',': COMMA,
	';': SEMI,
	':': COLON,
	'.': DOT,
	'?': QUEST,
	'=': ASSIGN,
	'+': PLUS,
	'-': MINUS,
	'*': STAR,
	'/': SLASH,
	'%': PERCENT,
	'<': LT,
	'>': GT,
	'(': LPAREN,
	')': RPAREN,
	'[': LBRACKET,
	']': RBRACKET,
	'{': LBRACE,
	'}': RBRACE,
}

func (l *Lexer) lexComment(at Pos, line int) Token {
	start := l.pos
	l.pos++ // consume #
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\n' {
			if l.pos > start && l.src[l.pos-1] == '\\' {
				l.pos++
				l.line++
				continue
			}
			break
		}
		l.pos++
	}
	return Token{Kind: COMMENT, Text: l.src[start:l.pos], At: at, Line: line}
}

func (l *Lexer) lexString(at Pos, line int) Token {
	start := l.pos
	l.pos++ // opening "
	depth := 0
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		switch {
		case ch == '"' && depth == 0:
			l.pos++
			return Token{Kind: STR, Text: l.src[start:l.pos], At: at, Line: line}
		case ch == '\\' && l.pos+1 < len(l.src):
			l.pos++
			next := l.src[l.pos]
			l.pos++
			if next == '(' {
				depth++
			} else if next == '\n' {
				l.line++
			}
		case ch == '\n':
			l.line++
			l.pos++
		case ch == '(' && depth > 0:
			depth++
			l.pos++
		case ch == ')' && depth > 0:
			depth--
			l.pos++
		default:
			_, size := utf8.DecodeRuneInString(l.src[l.pos:])
			l.pos += size
		}
	}
	return Token{Kind: STR, Text: l.src[start:l.pos], At: at, Line: line}
}

func (l *Lexer) lexNumber(at Pos, line int) Token {
	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
		l.pos++
	}
	if l.pos < len(l.src) && l.src[l.pos] == '.' &&
		l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9' {
		l.pos++
		for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
			l.pos++
		}
	}
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		l.pos++
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.pos++
		}
		for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
			l.pos++
		}
	}
	return Token{Kind: NUMBER, Text: l.src[start:l.pos], At: at, Line: line}
}

func (l *Lexer) lexVar(at Pos, line int) Token {
	l.pos++ // consume $
	if l.has("__loc__") {
		l.pos += len("__loc__")
		return Token{Kind: KWLOC, Text: "$__loc__", At: at, Line: line}
	}
	name := l.readIdent()
	return Token{Kind: VAR, Text: "$" + name, At: at, Line: line}
}

func (l *Lexer) lexFormat(at Pos, line int) Token {
	l.pos++ // consume @
	name := l.readIdent()
	return Token{Kind: FORMAT, Text: "@" + name, At: at, Line: line}
}

func (l *Lexer) lexIdent(at Pos, line int) Token {
	name := l.readIdent()
	if kw, ok := keywords[name]; ok {
		return Token{Kind: kw, Text: name, At: at, Line: line}
	}
	return Token{Kind: IDENT, Text: name, At: at, Line: line}
}

func (l *Lexer) readIdent() string {
	start := l.pos
	for l.pos < len(l.src) && isIdentContinue(l.src[l.pos]) {
		l.pos++
	}
	return l.src[start:l.pos]
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\n' {
			l.line++
			l.pos++
		} else if ch == ' ' || ch == '\t' || ch == '\r' {
			l.pos++
		} else {
			break
		}
	}
}

func (l *Lexer) has(s string) bool {
	return strings.HasPrefix(l.src[l.pos:], s)
}

func (l *Lexer) advance(k Kind, n int, line int) Token {
	at := Pos(l.pos)
	text := l.src[l.pos : l.pos+n]
	l.pos += n
	return Token{Kind: k, Text: text, At: at, Line: line}
}

func (l *Lexer) tok(k Kind, text string) Token {
	return Token{Kind: k, Text: text, At: Pos(l.pos), Line: l.line}
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentContinue(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9') || ch == '-'
}

// Tokens returns all tokens from src as a slice (useful for testing).
func Tokens(src string) []Token {
	l := NewLexer(src)
	var ts []Token
	for {
		t := l.Next()
		ts = append(ts, t)
		if t.Kind == EOF {
			break
		}
	}
	return ts
}

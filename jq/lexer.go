package jq

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Lexer tokenises a jq source string.
//
// Comments are emitted as COMMENT tokens so the formatter can preserve them.
// String interpolations are lexed as a single STR token containing the raw
// source text (quotes included); the formatter does not need to rewrite the
// content of strings.
type Lexer struct {
	src    string
	pos    int
	peeked *Token
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: src}
}

// Next returns the next token, consuming it.
func (l *Lexer) Next() Token {
	if l.peeked != nil {
		t := *l.peeked
		l.peeked = nil
		return t
	}
	return l.next()
}

// Peek returns the next token without consuming it.
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
	ch := l.src[l.pos]

	if ch == '#' {
		return l.lexComment()
	}
	if ch == '"' {
		return l.lexString()
	}
	if ch >= '0' && ch <= '9' {
		return l.lexNumber()
	}
	if ch == '$' {
		return l.lexVar()
	}
	if ch == '@' {
		return l.lexFormat()
	}
	if isIdentStart(ch) {
		return l.lexIdent()
	}

	// Multi-char operators — longest match first.
	switch {
	case l.has("?//"):
		return l.advance(ALTALT, 3)
	case l.has("//="):
		return l.advance(ALTEQ, 3)
	case l.has("|="):
		return l.advance(PIPEEQ, 2)
	case l.has("+="):
		return l.advance(PLUSEQ, 2)
	case l.has("-="):
		return l.advance(MINUSEQ, 2)
	case l.has("*="):
		return l.advance(STAREQ, 2)
	case l.has("/="):
		return l.advance(SLASHEQ, 2)
	case l.has("%="):
		return l.advance(PERCENTEQ, 2)
	case l.has("//"):
		return l.advance(ALT, 2)
	case l.has("=="):
		return l.advance(EQ, 2)
	case l.has("!="):
		return l.advance(NEQ, 2)
	case l.has("<="):
		return l.advance(LTEQ, 2)
	case l.has(">="):
		return l.advance(GTEQ, 2)
	case l.has(".."):
		return l.advance(DOTDOT, 2)
	}

	l.pos++
	k, ok := singleChar[ch]
	if !ok {
		panic(fmt.Sprintf("jq lexer: unexpected character %q at offset %d", ch, at))
	}

	// A bare dot followed by an identifier is a FIELD token.
	if k == DOT && l.pos < len(l.src) && isIdentStart(l.src[l.pos]) {
		name := l.readIdent()
		return Token{Kind: FIELD, Text: "." + name, At: at}
	}

	return Token{Kind: k, Text: string(ch), At: at}
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

func (l *Lexer) lexComment() Token {
	at := Pos(l.pos)
	start := l.pos
	l.pos++ // consume #
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\n' {
			// check for \ continuation
			if l.pos > start && l.src[l.pos-1] == '\\' {
				l.pos++
				continue
			}
			break
		}
		l.pos++
	}
	return Token{Kind: COMMENT, Text: l.src[start:l.pos], At: at}
}

// lexString lexes a complete double-quoted string including any \(...) interpolations.
func (l *Lexer) lexString() Token {
	at := Pos(l.pos)
	start := l.pos
	l.pos++ // consume opening "
	depth := 0
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		switch {
		case ch == '"' && depth == 0:
			l.pos++
			return Token{Kind: STR, Text: l.src[start:l.pos], At: at}
		case ch == '\\' && l.pos+1 < len(l.src):
			l.pos++
			next := l.src[l.pos]
			l.pos++
			if next == '(' {
				depth++
			}
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
	return Token{Kind: STR, Text: l.src[start:l.pos], At: at}
}

func (l *Lexer) lexNumber() Token {
	at := Pos(l.pos)
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
	return Token{Kind: NUMBER, Text: l.src[start:l.pos], At: at}
}

func (l *Lexer) lexVar() Token {
	at := Pos(l.pos)
	l.pos++ // consume $
	if l.has("__loc__") {
		l.pos += len("__loc__")
		return Token{Kind: KWLOC, Text: "$__loc__", At: at}
	}
	name := l.readIdent()
	return Token{Kind: VAR, Text: "$" + name, At: at}
}

func (l *Lexer) lexFormat() Token {
	at := Pos(l.pos)
	l.pos++ // consume @
	name := l.readIdent()
	return Token{Kind: FORMAT, Text: "@" + name, At: at}
}

func (l *Lexer) lexIdent() Token {
	at := Pos(l.pos)
	name := l.readIdent()
	if kw, ok := keywords[name]; ok {
		return Token{Kind: kw, Text: name, At: at}
	}
	return Token{Kind: IDENT, Text: name, At: at}
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
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			l.pos++
		} else {
			break
		}
	}
}

func (l *Lexer) has(s string) bool {
	return strings.HasPrefix(l.src[l.pos:], s)
}

func (l *Lexer) advance(k Kind, n int) Token {
	at := Pos(l.pos)
	text := l.src[l.pos : l.pos+n]
	l.pos += n
	return Token{Kind: k, Text: text, At: at}
}

func (l *Lexer) tok(k Kind, text string) Token {
	return Token{Kind: k, Text: text, At: Pos(l.pos)}
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

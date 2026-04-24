package jq

import "fmt"

// Pos is a byte offset into the source.
type Pos int

// Token is a single lexical unit from a jq source file.
type Token struct {
	Kind Kind
	Text string // exact source text
	At   Pos
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q)@%d", t.Kind, t.Text, t.At)
}

// Kind is a token kind.  ALL_CAPS constants avoid collision with the PascalCase
// AST node types in the same package (following the go/token convention).
type Kind int

const (
	EOF Kind = iota

	COMMENT // # ...  (text includes # but not the trailing newline)

	IDENT   // foo, empty, not, …
	FIELD   // .foo  (dot + name)
	VAR     // $foo
	FORMAT  // @base64
	NUMBER  // 42, 3.14
	STR     // complete string literal "…" (raw text, interpolations not parsed)

	PIPE    // |
	COMMA   // ,
	SEMI    // ;
	COLON   // :
	DOT     // .
	DOTDOT  // ..
	QUEST   // ?

	ASSIGN    // =
	PIPEEQ    // |=
	PLUSEQ    // +=
	MINUSEQ   // -=
	STAREQ    // *=
	SLASHEQ   // /=
	PERCENTEQ // %=
	ALTEQ     // //=
	ALT       // //
	ALTALT    // ?//

	PLUS    // +
	MINUS   // -
	STAR    // *
	SLASH   // /
	PERCENT // %
	EQ      // ==
	NEQ     // !=
	LT      // <
	GT      // >
	LTEQ    // <=
	GTEQ    // >=

	LPAREN   // (
	RPAREN   // )
	LBRACKET // [
	RBRACKET // ]
	LBRACE   // {
	RBRACE   // }

	// Keywords
	KWIF      // if
	KWTHEN    // then
	KWELSE    // else
	KWELIF    // elif
	KWEND     // end
	KWAS      // as
	KWDEF     // def
	KWIMPORT  // import
	KWINCLUDE // include
	KWMODULE  // module
	KWREDUCE  // reduce
	KWFOREACH // foreach
	KWTRY     // try
	KWCATCH   // catch
	KWLABEL   // label
	KWBREAK   // break
	KWAND     // and
	KWOR      // or
	KWLOC     // $__loc__
)

var kindNames = map[Kind]string{
	EOF:       "EOF",
	COMMENT:   "Comment",
	IDENT:     "Ident",
	FIELD:     "Field",
	VAR:       "Var",
	FORMAT:    "Format",
	NUMBER:    "Number",
	STR:       "Str",
	PIPE:      "|",
	COMMA:     ",",
	SEMI:      ";",
	COLON:     ":",
	DOT:       ".",
	DOTDOT:    "..",
	QUEST:     "?",
	ASSIGN:    "=",
	PIPEEQ:    "|=",
	PLUSEQ:    "+=",
	MINUSEQ:   "-=",
	STAREQ:    "*=",
	SLASHEQ:   "/=",
	PERCENTEQ: "%=",
	ALTEQ:     "//=",
	ALT:       "//",
	ALTALT:    "?//",
	PLUS:      "+",
	MINUS:     "-",
	STAR:      "*",
	SLASH:     "/",
	PERCENT:   "%",
	EQ:        "==",
	NEQ:       "!=",
	LT:        "<",
	GT:        ">",
	LTEQ:      "<=",
	GTEQ:      ">=",
	LPAREN:    "(",
	RPAREN:    ")",
	LBRACKET:  "[",
	RBRACKET:  "]",
	LBRACE:    "{",
	RBRACE:    "}",
	KWIF:      "if",
	KWTHEN:    "then",
	KWELSE:    "else",
	KWELIF:    "elif",
	KWEND:     "end",
	KWAS:      "as",
	KWDEF:     "def",
	KWIMPORT:  "import",
	KWINCLUDE: "include",
	KWMODULE:  "module",
	KWREDUCE:  "reduce",
	KWFOREACH: "foreach",
	KWTRY:     "try",
	KWCATCH:   "catch",
	KWLABEL:   "label",
	KWBREAK:   "break",
	KWAND:     "and",
	KWOR:      "or",
	KWLOC:     "$__loc__",
}

func (k Kind) String() string {
	if s, ok := kindNames[k]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

var keywords = map[string]Kind{
	"if":      KWIF,
	"then":    KWTHEN,
	"else":    KWELSE,
	"elif":    KWELIF,
	"end":     KWEND,
	"as":      KWAS,
	"def":     KWDEF,
	"import":  KWIMPORT,
	"include": KWINCLUDE,
	"module":  KWMODULE,
	"reduce":  KWREDUCE,
	"foreach": KWFOREACH,
	"try":     KWTRY,
	"catch":   KWCATCH,
	"label":   KWLABEL,
	"break":   KWBREAK,
	"and":     KWAND,
	"or":      KWOR,
}

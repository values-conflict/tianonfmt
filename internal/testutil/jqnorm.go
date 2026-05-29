package testutil

import (
	"regexp"
	"strings"
)

// ScanJQStr scans a jq string literal beginning at src[start] (a '"') and
// returns the index just past the closing '"'.  It correctly handles \(…)
// interpolation by delegating to ScanJQInterp so that a '"' inside \(…) is
// treated as a nested string, not as the end of the outer string.
func ScanJQStr(src string, start int) int {
	i := start + 1
	for i < len(src) {
		switch {
		case src[i] == '\\' && i+1 < len(src) && src[i+1] == '(':
			i = ScanJQInterp(src, i+2)
		case src[i] == '\\' && i+1 < len(src):
			i += 2
		case src[i] == '"':
			return i + 1
		default:
			i++
		}
	}
	return i
}

// ScanJQInterp scans the expression inside a \(…) interpolation starting at
// src[start] (the byte immediately after the opening '\(') and returns the
// index just past the matching ')'.
func ScanJQInterp(src string, start int) int {
	i := start
	depth := 1
	for i < len(src) && depth > 0 {
		switch {
		case src[i] == '(':
			depth++
			i++
		case src[i] == ')':
			depth--
			i++
		case src[i] == '"':
			i = ScanJQStr(src, i)
		case src[i] == '\\' && i+1 < len(src):
			i += 2
		default:
			i++
		}
	}
	return i
}

func jqIsAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func jqIsDigit(c byte) bool { return c >= '0' && c <= '9' }
func jqIsIdent(c byte) bool { return jqIsAlpha(c) || jqIsDigit(c) || c == '_' }

// TokenizeJQ splits jq source into a flat slice of non-whitespace,
// non-comment tokens.  String literals (including \(…) interpolations) are
// returned as opaque tokens.
func TokenizeJQ(src string) []string {
	var tokens []string
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '"':
			j := ScanJQStr(src, i)
			tokens = append(tokens, src[i:j])
			i = j
		case c == '@':
			j := i + 1
			for j < len(src) && jqIsIdent(src[j]) {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case c == '$':
			j := i + 1
			for j < len(src) && jqIsIdent(src[j]) {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case jqIsAlpha(c) || c == '_':
			j := i
			for j < len(src) && jqIsIdent(src[j]) {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case jqIsDigit(c):
			j := i
			for j < len(src) && (jqIsDigit(src[j]) || src[j] == '.' || src[j] == 'e' || src[j] == 'E') {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case c == '.' && i+1 < len(src) && src[i+1] == '.':
			tokens = append(tokens, "..")
			i += 2
		case c == '.' && i+1 < len(src) && jqIsDigit(src[i+1]):
			j := i
			for j < len(src) && (jqIsDigit(src[j]) || src[j] == '.') {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		default:
			tokens = append(tokens, string(c))
			i++
		}
	}
	return tokens
}

// JQBareKeyRE matches valid jq bare-identifier object keys.
var JQBareKeyRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// NormalizeJQ produces a canonical token sequence for semantic comparison of
// jq source.  It applies three mechanical rewrites beyond pure whitespace:
//  1. Object key unquoting: "foo" → foo when "foo" is a valid identifier and
//     the next token is ":".
//  2. Trailing comma removal: "," immediately before "}" → remove.
//  3. Trailing-empty removal: "," "empty" at the end of a comma chain → remove.
//     The formatter adds `empty` to chains with ≥ 6 flat items as the
//     jq trailing-comma idiom; this strip ensures token-level comparison
//     treats the added `empty` as a whitespace-equivalent transformation.
//
// normalizeInterpInStr normalizes \(...) blocks within a string token so that
// whitespace changes inside interpolations compare equal.  The token is the
// full raw string literal including surrounding quotes.
func normalizeInterpInStr(tok string) string {
	if !strings.Contains(tok, `\(`) {
		return tok
	}
	var b strings.Builder
	i := 0
	for i < len(tok) {
		if tok[i] == '\\' && i+1 < len(tok) && tok[i+1] == '(' {
			end := ScanJQInterp(tok, i+2)
			b.WriteString(`\(`)
			if end >= 0 && end <= len(tok) {
				b.WriteString(NormalizeJQ(tok[i+2 : end-1]))
				b.WriteByte(')')
				i = end
			} else {
				// Unterminated — copy verbatim.
				b.WriteString(tok[i+2:])
				break
			}
		} else {
			b.WriteByte(tok[i])
			i++
		}
	}
	return b.String()
}

func NormalizeJQ(src string) string {
	toks := TokenizeJQ(src)

	// Normalize \(...) content within string tokens so that whitespace changes
	// inside interpolations compare equal across input and formatted output.
	for i, tok := range toks {
		if strings.HasPrefix(tok, `"`) {
			toks[i] = normalizeInterpInStr(tok)
		}
	}

	for i := 0; i+1 < len(toks); i++ {
		if !strings.HasPrefix(toks[i], `"`) || toks[i+1] != ":" {
			continue
		}
		raw := toks[i]
		content := raw[1 : len(raw)-1]
		ok := true
		for _, b := range []byte(content) {
			if b == '\\' || b > 127 {
				ok = false
				break
			}
		}
		if ok && JQBareKeyRE.MatchString(content) {
			toks[i] = content
		}
	}

	result := toks[:0]
	for i, tok := range toks {
		// Trailing comma before }
		if tok == "," && i+1 < len(toks) && toks[i+1] == "}" {
			continue
		}
		// Trailing-empty: strip ", empty" at end of comma chain (before ) or ])
		if tok == "," && i+1 < len(toks) && toks[i+1] == "empty" &&
			i+2 < len(toks) && (toks[i+2] == ")" || toks[i+2] == "]") {
			continue
		}
		if tok == "empty" && i > 0 && toks[i-1] == "," &&
			i+1 < len(toks) && (toks[i+1] == ")" || toks[i+1] == "]") {
			continue
		}
		result = append(result, tok)
	}

	return strings.Join(result, " ")
}

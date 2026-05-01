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
// jq source.  It applies two mechanical rewrites beyond pure whitespace:
//  1. Object key unquoting: "foo" → foo when "foo" is a valid identifier and
//     the next token is ":".
//  2. Trailing comma removal: "," immediately before "}" → remove.
func NormalizeJQ(src string) string {
	toks := TokenizeJQ(src)

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
		if tok == "," && i+1 < len(toks) && toks[i+1] == "}" {
			continue
		}
		result = append(result, tok)
	}

	return strings.Join(result, " ")
}

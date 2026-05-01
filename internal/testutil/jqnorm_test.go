package testutil_test

import (
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"
)

// TestTokenizeJQ exercises edge cases in the jq tokenizer that the fixture-
// driven TestFormatPreservesTokens tests don't reach.
func TestTokenizeJQ(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		// Whitespace and comments are discarded.
		{"  \t\n# comment\n", nil},
		// Plain identifier.
		{"foo", []string{"foo"}},
		// $variable and @format.
		{"$x @base64", []string{"$x", "@base64"}},
		// .. recurse operator (distinct from two . tokens).
		{"..", []string{".."}},
		// Leading-dot float (.5 is a valid jq number).
		{".5 + 1.0", []string{".5", "+", "1.0"}},
		// Number with exponent.
		{"1e10", []string{"1e10"}},
		// String with \( ) interpolation — ScanJQStr calls ScanJQInterp.
		{`"\(.x)"`, []string{`"\(.x)"`}},
		// Nested string inside interpolation — ScanJQInterp calls ScanJQStr.
		{`"\("inner")"`, []string{`"\("inner")"`}},
		// Escape sequence inside string.
		{`"\n\t"`, []string{`"\n\t"`}},
		// Unterminated string — scanner reaches end of input.
		{`"unterminated`, []string{`"unterminated`}},
		// Unterminated interpolation inside string.
		{`"\(no_close`, []string{`"\(no_close`}},
		// Backslash-escape followed by non-( inside interpolation.
		{`"\(\n)"`, []string{`"\(\n)"`}},
		// Multiple tokens with various types.
		{`def f($x): $x | not;`, []string{"def", "f", "(", "$x", ")", ":", "$x", "|", "not", ";"}},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := testutil.TokenizeJQ(c.in)
			if len(got) != len(c.want) {
				t.Errorf("TokenizeJQ(%q) = %v, want %v", c.in, got, c.want)
				return
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("TokenizeJQ(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestNormalizeJQ(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Key unquoting: "identifier": → identifier:
		{`{"foo": .}`, `{ foo : . }`},
		// Key unquoting skips non-identifiers and strings with escapes.
		{`{"foo-bar": .}`, `{ "foo-bar" : . }`},
		{`{"fo\no": .}`, `{ "fo\no" : . }`},
		// Trailing comma removal.
		{`{foo: .bar,}`, `{ foo : . bar }`},
		// Both rewrites together.
		{`{"x": .y,}`, `{ x : . y }`},
		// String in value position not unquoted.
		{`. | {"k": "v"}`, `. | { k : "v" }`},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := testutil.NormalizeJQ(c.in)
			if got != c.want {
				t.Errorf("NormalizeJQ(%q)\ngot:  %q\nwant: %q", c.in, got, c.want)
			}
		})
	}
}

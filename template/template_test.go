package template_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"

	"github.com/tianon/fmt/tianonfmt/jq"
	"github.com/tianon/fmt/tianonfmt/template"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// realJQFmt mirrors the jqFmtFunc used by the CLI.
func realJQFmt(expr string, inline bool) string {
	node, err := jq.ParseExpr(strings.TrimSpace(expr))
	if err != nil {
		f, ferr := jq.ParseFile(strings.TrimSpace(expr))
		if ferr != nil {
			return ""
		}
		return jq.FormatFile(f)
	}
	if inline {
		return jq.FormatNodeInline(node)
	}
	return jq.FormatNode(node)
}

// ── Format ────────────────────────────────────────────────────────────────────

func TestFormat(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.template", []testutil.Case{
		{Out: "output.template", Fn: func(src string) (string, error) {
			return template.Format(src, realJQFmt), nil
		}, Idem: true},
	})
}

// ── IsTemplate ────────────────────────────────────────────────────────────────

func TestIsTemplate(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"FROM debian\nRUN echo hi\n", false},
		{"FROM {{ .base }}\n", true},
		{"{{ .foo }}\n", true},
		{"no blocks here\n", false},
		{"", false},
	}
	for _, tt := range cases {
		if got := template.IsTemplate(tt.src); got != tt.want {
			t.Errorf("IsTemplate(%q) = %v, want %v", tt.src, got, tt.want)
		}
	}
}

// ── Parse ─────────────────────────────────────────────────────────────────────

func TestParse_Empty(t *testing.T) {
	if segs := template.Parse(""); len(segs) != 0 {
		t.Errorf("expected empty segments, got %d", len(segs))
	}
}

func TestParse_TextOnly(t *testing.T) {
	segs := template.Parse("FROM debian\nRUN echo hi\n")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if _, ok := segs[0].(template.TextSeg); !ok {
		t.Errorf("expected TextSeg, got %T", segs[0])
	}
}

func TestParse_JQBlock(t *testing.T) {
	segs := template.Parse("FROM {{ .base }}\n")
	if len(segs) < 2 {
		t.Fatalf("expected at least 2 segments, got %d", len(segs))
	}
	var found bool
	for _, seg := range segs {
		if j, ok := seg.(template.JQSeg); ok {
			if strings.TrimSpace(j.Expr) == ".base" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected JQSeg with expr=.base in %v", segs)
	}
}

func TestParse_EatEOL(t *testing.T) {
	segs := template.Parse("FROM {{ .base -}}\n")
	var found bool
	for _, seg := range segs {
		if j, ok := seg.(template.JQSeg); ok && j.EatEOL {
			found = true
		}
	}
	if !found {
		t.Error("expected EatEOL=true for -}} marker")
	}
}

// ── Format edge cases ─────────────────────────────────────────────────────────

func TestFormat_NilJQFmt(t *testing.T) {
	// jqFmt=nil: jq expressions pass through verbatim.
	src := "FROM {{ .base }}\n"
	out := template.Format(src, nil)
	if !strings.Contains(out, ".base") {
		t.Errorf("expression not preserved with nil jqFmt: %q", out)
	}
}

func TestFormat_Comment(t *testing.T) {
	// Comment blocks ({{# ... }}) are emitted as-is without calling jqFmt.
	called := false
	jqFmt := func(expr string, _ bool) string {
		called = true
		return expr
	}
	src := "{{# this is a comment -}}\nFROM debian\n"
	out := template.Format(src, jqFmt)
	if called {
		t.Error("jqFmt should not be called for comment blocks")
	}
	if !strings.Contains(out, "# this is a comment") {
		t.Errorf("comment not preserved: %q", out)
	}
}

func TestFormat_EatEOLPreserved(t *testing.T) {
	// {{ expr -}} is a formatting marker (consumed by the evaluator, not the
	// formatter).  The formatter's job is only to preserve -}} in the output.
	src := "FROM {{ .base -}}\nEXTRA\n"
	out := template.Format(src, func(expr string, _ bool) string { return expr })
	if !strings.Contains(out, "-}}") {
		t.Errorf("EatEOL marker -}} not preserved in output: %q", out)
	}
}

// ── token-level format preservation ──────────────────────────────────────────
//
// The jq tokenizer and normalizer below are a local copy of the equivalent
// helpers in jq/jq_test.go.  They live here rather than in internal/ because
// they are test-only code, and the CLAUDE.md preference is to share via
// internal/ only for production helpers.  If the two diverge, reconcile by
// reviewing the jq package's version.

func tmplScanJQStr(src string, start int) int {
	i := start + 1
	for i < len(src) {
		switch {
		case src[i] == '\\' && i+1 < len(src) && src[i+1] == '(':
			i = tmplScanJQInterp(src, i+2)
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

func tmplScanJQInterp(src string, start int) int {
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
			i = tmplScanJQStr(src, i)
		case src[i] == '\\' && i+1 < len(src):
			i += 2
		default:
			i++
		}
	}
	return i
}

func tmplIsAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func tmplIsDigit(c byte) bool { return c >= '0' && c <= '9' }
func tmplIsIdent(c byte) bool { return tmplIsAlpha(c) || tmplIsDigit(c) || c == '_' }

func tmplTokenizeJQ(src string) []string {
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
			j := tmplScanJQStr(src, i)
			tokens = append(tokens, src[i:j])
			i = j
		case c == '@':
			j := i + 1
			for j < len(src) && tmplIsIdent(src[j]) {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case c == '$':
			j := i + 1
			for j < len(src) && tmplIsIdent(src[j]) {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case tmplIsAlpha(c) || c == '_':
			j := i
			for j < len(src) && tmplIsIdent(src[j]) {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case tmplIsDigit(c):
			j := i
			for j < len(src) && (tmplIsDigit(src[j]) || src[j] == '.' || src[j] == 'e' || src[j] == 'E') {
				j++
			}
			tokens = append(tokens, src[i:j])
			i = j
		case c == '.' && i+1 < len(src) && src[i+1] == '.':
			tokens = append(tokens, "..")
			i += 2
		case c == '.' && i+1 < len(src) && tmplIsDigit(src[i+1]):
			j := i
			for j < len(src) && (tmplIsDigit(src[j]) || src[j] == '.') {
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

var tmplBareKeyRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func tmplNormalizeJQ(src string) string {
	toks := tmplTokenizeJQ(src)
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
		if ok && tmplBareKeyRE.MatchString(content) {
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

// normalizeTemplate normalizes a Dockerfile template for semantic comparison.
// It splits the source on {{ … }} block boundaries and applies tmplNormalizeJQ
// to each jq block's content, leaving literal text verbatim (the formatter
// preserves it unchanged).  Trim markers (-{{ and -}}) are preserved.
//
// The formatter may reflow a multi-line jq block to a single line or vice
// versa; after normalizing the jq content with tmplNormalizeJQ (which strips
// whitespace), both layouts map to the same token sequence.
func normalizeTemplate(src string) string {
	var sb strings.Builder
	i := 0
	for i < len(src) {
		openIdx := strings.Index(src[i:], "{{")
		if openIdx < 0 {
			sb.WriteString(src[i:])
			break
		}
		// literal text up to the block opener
		sb.WriteString(src[i : i+openIdx])
		i += openIdx + 2

		// leading trim marker
		trimOpen := i < len(src) && src[i] == '-'
		if trimOpen {
			i++
			sb.WriteString("{{-")
		} else {
			sb.WriteString("{{")
		}

		// Find the block close: prefer -}} over }}, take whichever comes first.
		rest := src[i:]
		ci1 := strings.Index(rest, "-}}")
		ci2 := strings.Index(rest, "}}")

		var blockContent string
		var trimClose bool
		var closeLen int
		switch {
		case ci1 >= 0 && (ci2 < 0 || ci1 < ci2):
			blockContent = rest[:ci1]
			trimClose = true
			closeLen = ci1 + 3
		case ci2 >= 0:
			blockContent = rest[:ci2]
			trimClose = false
			closeLen = ci2 + 2
		default:
			// unterminated — treat rest as block content
			blockContent = rest
			closeLen = len(rest)
		}

		sb.WriteString(tmplNormalizeJQ(blockContent))
		if trimClose {
			sb.WriteString("-}}")
		} else {
			sb.WriteString("}}")
		}
		i += closeLen
	}
	return sb.String()
}

// TestFormatPreservesTokens verifies that template.Format does not silently
// alter the program beyond the known mechanical transformations (jq whitespace
// normalization and block layout collapsing).  Expected value is derived from
// raw input text; no golden file is used.
func TestFormatPreservesTokens(t *testing.T) {
	dirs, err := os.ReadDir("testdata/format")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		t.Run(d.Name(), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/format", d.Name(), "input.template"))
			if err != nil {
				t.Skip("no input.template")
				return
			}
			formatted := template.Format(string(src), realJQFmt)
			normIn := normalizeTemplate(string(src))
			normOut := normalizeTemplate(formatted)
			if normIn != normOut {
				t.Errorf("normalizeTemplate(format(input)) != normalizeTemplate(input)\n\nnorm(input):\n%s\n\nnorm(format):\n%s",
					normIn, normOut)
			}
		})
	}
}

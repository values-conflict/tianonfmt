package template_test

import (
	"flag"
	"os"
	"path/filepath"
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
	trimmed := strings.TrimSpace(expr)
	node, err := jq.ParseExpr(trimmed)
	if err != nil {
		f, ferr := jq.ParseFile(trimmed)
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

func TestParse_NestedBraces(t *testing.T) {
	// A block whose jq expression contains literal {{ and }} (e.g. in a string
	// literal).  The parser must handle nested depth correctly.
	src := "{{ if .x then \"{{\" else \"}}\" end }}\n"
	segs := template.Parse(src)
	// Should produce exactly one JQSeg (the expression is the whole block).
	var jqSegs []template.JQSeg
	for _, seg := range segs {
		if j, ok := seg.(template.JQSeg); ok {
			jqSegs = append(jqSegs, j)
		}
	}
	if len(jqSegs) != 1 {
		t.Fatalf("expected 1 JQSeg, got %d: %v", len(jqSegs), segs)
	}
	if !strings.Contains(jqSegs[0].Expr, "if .x") {
		t.Errorf("unexpected expr: %q", jqSegs[0].Expr)
	}
}

func TestParse_NestedEatClose(t *testing.T) {
	// A block containing a nested {{ }} pair where the inner close is -}}.
	// Exercises the "depth > 0 with -}}" code path (pos += len(closeEat)).
	src := "{{ {{ inner -}} outer }}\n"
	segs := template.Parse(src)
	var jqSegs []template.JQSeg
	for _, seg := range segs {
		if j, ok := seg.(template.JQSeg); ok {
			jqSegs = append(jqSegs, j)
		}
	}
	if len(jqSegs) != 1 {
		t.Fatalf("expected 1 JQSeg, got %d: %v", len(jqSegs), segs)
	}
	// The inner -}} should be treated as literal content, not a closer.
	if !strings.Contains(jqSegs[0].Expr, "inner") {
		t.Errorf("inner content missing from expr: %q", jqSegs[0].Expr)
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

func TestFormat_EmptySrc(t *testing.T) {
	// Format("") must return "" (no segments → early return).
	if out := template.Format("", nil); out != "" {
		t.Errorf("Format(\"\") = %q, want \"\"", out)
	}
}

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

// ── additional Format edge cases ─────────────────────────────────────────────

func TestFormat_DefWithSemicolon(t *testing.T) {
	// A def block that already carries a ";" should be formatted without adding
	// another one.  jqFmt succeeds on the as-is expr (ParseFile handles it).
	src := "{{ def foo: .bar; -}}\nFROM debian\n"
	out := template.Format(src, realJQFmt)
	if !strings.Contains(out, "def foo") {
		t.Errorf("def not preserved: %q", out)
	}
	// Idempotency: second pass should produce the same result.
	if out2 := template.Format(out, realJQFmt); out2 != out {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", out, out2)
	}
}

func TestFormat_MultiLineCommentIndented(t *testing.T) {
	// A multi-line comment block whose {{ opener is at a tabbed indent level.
	// The preceding newline ensures leadingTabs sees the tab prefix correctly.
	src := "FROM debian\n\t{{\n\t# comment1\n\t# comment2\n\t-}}\n"
	out := template.Format(src, nil)
	if !strings.Contains(out, "# comment1") || !strings.Contains(out, "# comment2") {
		t.Errorf("multi-line comment not preserved: %q", out)
	}
}

func TestFormat_InlineForceBlockFails(t *testing.T) {
	// An inline block that formats to something with '#' but then fails
	// to format as non-inline falls back to verbatim single-line.
	callCount := 0
	jqFmt := func(expr string, inline bool) string {
		callCount++
		if inline {
			return "# comment " + expr // has '#' → forces block
		}
		return "" // fails on non-inline attempt
	}
	src := "FROM {{ .base }}\n"
	out := template.Format(src, jqFmt)
	// Should preserve the original expression (verbatim fallback)
	if !strings.Contains(out, ".base") {
		t.Errorf("expression not preserved after force-block failure: %q", out)
	}
}

func TestBracketDelta_StringEscape(t *testing.T) {
	// bracketDelta must skip brackets inside string literals and comments.
	// Exercise both the string-escape path (\\) and the comment path (#).
	// This is tested by triggering assembly on a block whose expression
	// contains these constructs — the assembly uses bracketDelta internally.
	src := "{{ if env.v then ( -}}\n{{ .x -}}\n{{ ) else \"\" end -}}\n"
	// Simpler: test that a block with "({)" in a string has delta=0 via Format
	src2 := "{{ .[\"key\"] }}\n"
	out := template.Format(src, realJQFmt)
	out2 := template.Format(src2, realJQFmt)
	if !strings.Contains(out, "if env.v then (") {
		t.Errorf("unexpected: %q", out)
	}
	if !strings.Contains(out2, ".[\"key\"]") {
		t.Errorf("unexpected: %q", out2)
	}
}

func TestTrimPiece_BlankLines(t *testing.T) {
	// trimPiece must strip leading and trailing blank lines.
	// Trigger it with a template that produces assembly pieces with blank lines.
	// Two nested sub-expressions separated by a concat sentinel, where the
	// formatted output happens to have blank lines around the sentinel.
	src := "{{ if env.v == \"a\" then ( -}}\n" +
		"{{ ) else ( -}}\n" +
		"{{ if .x then ( -}}\nX\n{{ ) else \"\" end -}}\n" +
		"{{ if .y then ( -}}\nY\n{{ ) else \"\" end -}}\n" +
		"{{ ) end -}}\n"
	out := template.Format(src, realJQFmt)
	if !strings.Contains(out, "if .x then (") {
		t.Errorf("unexpected: %q", out)
	}
}

func TestParse_NestedClose(t *testing.T) {
	// Block containing "}" in a jq string — bracketDelta must skip string contents
	// (exercises the inString + escape code path in bracketDelta).
	src := `{{ "{\"key\": \"val\"}" }}`
	segs := template.Parse(src)
	var jqSegs []template.JQSeg
	for _, s := range segs {
		if j, ok := s.(template.JQSeg); ok {
			jqSegs = append(jqSegs, j)
		}
	}
	if len(jqSegs) != 1 {
		t.Fatalf("expected 1 JQSeg, got %d", len(jqSegs))
	}
}

func TestParse_CommentInBlock(t *testing.T) {
	// Block containing a jq comment — bracketDelta must skip comment to end-of-line.
	src := "{{\n\t.foo # comment with ) and } chars\n}}\n"
	segs := template.Parse(src)
	var jqSegs []template.JQSeg
	for _, s := range segs {
		if j, ok := s.(template.JQSeg); ok {
			jqSegs = append(jqSegs, j)
		}
	}
	if len(jqSegs) != 1 || !strings.Contains(jqSegs[0].Expr, ".foo") {
		t.Fatalf("unexpected segs: %v", segs)
	}
}

func TestFormat_AdjacentSubexpressions(t *testing.T) {
	// Two independent jq if-then-else blocks inside the same outer else-branch.
	// The awk connects them with + (string concat); our assembler must do the same.
	src := "{{ if env.v == \"a\" then ( -}}\nA\n{{ ) else ( -}}\n" +
		"{{ if .x then ( -}}\nX\n{{ ) else \"\" end -}}\n" +
		"{{ if .y then ( -}}\nY\n{{ ) else \"\" end -}}\n" +
		"{{ ) end -}}\n"
	out := template.Format(src, realJQFmt)
	if !strings.Contains(out, "if .x then (") || !strings.Contains(out, "if .y then (") {
		t.Errorf("sub-expressions not preserved: %q", out)
	}
	out2 := template.Format(out, realJQFmt)
	if out != out2 {
		t.Errorf("not idempotent:\npass1: %q\npass2: %q", out, out2)
	}
}

func TestFormat_UnterminatedBlock(t *testing.T) {
	// An unterminated {{ block (no matching }}) — the remainder after {{ is
	// emitted as a TextSeg.
	src := "FROM {{ .base\n"
	segs := template.Parse(src)
	// Expect two TextSegs: "FROM " and " .base\n" (the unterminated remainder).
	var textSegs []string
	for _, seg := range segs {
		if ts, ok := seg.(template.TextSeg); ok {
			textSegs = append(textSegs, ts.Text)
		}
	}
	if len(textSegs) != 2 {
		t.Fatalf("expected 2 text segs, got %d: %v", len(textSegs), segs)
	}
	if !strings.Contains(textSegs[1], ".base") {
		t.Errorf("unterminated remainder not in text segs: %v", textSegs)
	}
}

// ── token-level format preservation ──────────────────────────────────────────

// normalizeTemplate normalizes a Dockerfile template for semantic comparison.
// It splits the source on {{ … }} block boundaries and applies
// testutil.NormalizeJQ to each jq block's content, leaving literal text
// verbatim (the formatter preserves it unchanged).  Trim markers are kept.
//
// The formatter may reflow a multi-line jq block to a single line or vice
// versa; after normalizing the jq content (which strips whitespace), both
// layouts map to the same token sequence.
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

		sb.WriteString(testutil.NormalizeJQ(blockContent))
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

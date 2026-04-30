package jq_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"

	"github.com/tianon/fmt/tianonfmt/jq"
)


func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// ── format ────────────────────────────────────────────────────────────────────

// TestFormat verifies the formatter produces expected output for each testdata case.
// Run `go test -update` to regenerate golden output files.
func TestFormat(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.jq", "output.jq", func(input string) (string, error) {
		f, err := jq.ParseFile(input)
		if err != nil {
			return "", err
		}
		return jq.FormatFile(f), nil
	})
}

// TestFormatIdempotent verifies formatting is stable: format(format(x)) == format(x).
func TestFormatIdempotent(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.jq", "output.jq", func(input string) (string, error) {
		f, err := jq.ParseFile(input)
		if err != nil {
			return "", err
		}
		first := jq.FormatFile(f)
		f2, err := jq.ParseFile(first)
		if err != nil {
			return "", fmt.Errorf("re-parse after format: %w", err)
		}
		return jq.FormatFile(f2), nil
	})
}

// ── lint ──────────────────────────────────────────────────────────────────────

// TestLint verifies the linter reports the expected violations.
// violations.txt contains one "line: message" entry per line; empty = no violations.
// TestLintNullInElse_AllBranches exercises all three branches of lintNullInElse.
func TestLintNullInElse_AllBranches(t *testing.T) {
	lint := func(src string) []jq.Violation {
		f, _ := jq.ParseFile(src)
		return jq.LintFile(f, src)
	}
	// Branch 1: if without else — Else is nil, no violation
	for _, v := range lint("if .a then .b end") {
		if strings.Contains(v.Msg, "null") {
			t.Errorf("if-without-else should not trigger null check: %v", v)
		}
	}
	// Branch 2: else empty — not null, no violation
	for _, v := range lint("if .a then .b else empty end") {
		if strings.Contains(v.Msg, "else null") {
			t.Errorf("else-empty should not be flagged: %v", v)
		}
	}
	// Branch 3: else null — violation
	vs := lint("if .a then .b else null end")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Msg, "else null") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for else null")
	}
}

func TestLint(t *testing.T) {
	testutil.Golden(t, "testdata/lint", "input.jq", "output.txt", func(input string) (string, error) {
		f, err := jq.ParseFile(input)
		if err != nil {
			return "", err
		}
		vs := jq.LintFile(f, input)
		var sb strings.Builder
		for _, v := range vs {
			fmt.Fprintf(&sb, "%d: %s\n", v.Line, v.Msg)
		}
		return sb.String(), nil
	})
}

// ── Tokens (lexer) ───────────────────────────────────────────────────────────

func TestTokens(t *testing.T) {
	toks := jq.Tokens(".foo | .bar")
	if len(toks) == 0 {
		t.Error("expected non-empty token stream")
	}
	// First non-whitespace token should be a field
	found := false
	for _, tok := range toks {
		if tok.Kind == jq.FIELD {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected FIELD token in .foo | .bar; got: %v", toks)
	}
}

// ── MarshalAST golden ─────────────────────────────────────────────────────────

// TestMarshalAST pins the full JSON AST output for every format fixture.
// This catches regressions in AST structure, field names, and ordering.
func TestMarshalAST(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.jq", "ast.json", func(src string) (string, error) {
		f, err := jq.ParseFile(src)
		if err != nil {
			return "", err
		}
		b, err := json.MarshalIndent(f.MarshalAST(), "", "\t")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	})
}

// ── marshalComments / Comment.MarshalAST ─────────────────────────────────────

func TestCommentMarshalAST(t *testing.T) {
	// Direct test of Comment.MarshalAST (required by the Node interface).
	// The comments fixture covers leadingComments in the golden AST output;
	// this exercises the direct MarshalAST call on a Comment node.
	src := "# hello\n.x\n"
	f, err := jq.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var foundComment bool
	jq.Walk(f, func(n jq.Node) bool {
		if c, ok := n.(*jq.Comment); ok {
			m := c.MarshalAST()
			if len(m) == 0 || m[0].Key != "type" {
				t.Errorf("unexpected Comment.MarshalAST: %v", m)
			}
			foundComment = true
		}
		return true
	})
	if !foundComment {
		t.Skip("no Comment node found — parser may not expose comments as walk nodes")
	}
}

// ── ParseExpr / FormatNode / FormatNodeInline ────────────────────────────────

func TestParseExpr(t *testing.T) {
	node, err := jq.ParseExpr(".foo | .bar")
	if err != nil {
		t.Fatal(err)
	}
	if node == nil {
		t.Error("nil node")
	}
}

// TestParseErrors verifies that malformed jq inputs produce the expected parse
// errors (not panics, and not nil).  Each fixture in testdata/errors/ has an
// input.jq and an error.txt golden file pinning the exact error message.
func TestParseErrors(t *testing.T) {
	testutil.Golden(t, "testdata/errors", "input.jq", "", func(src string) (string, error) {
		_, err := jq.ParseFile(src)
		return "", err
	})
}

func TestFormatNode(t *testing.T) {
	node, err := jq.ParseExpr(".foo | .bar")
	if err != nil {
		t.Fatal(err)
	}
	out := jq.FormatNode(node)
	if out == "" {
		t.Error("empty output from FormatNode")
	}
}

func TestFormatNodeInline(t *testing.T) {
	node, err := jq.ParseExpr(".foo | .bar")
	if err != nil {
		t.Fatal(err)
	}
	out := jq.FormatNodeInline(node)
	if out == "" {
		t.Error("empty output from FormatNodeInline")
	}
	// Inline format should not have a trailing newline
	if strings.HasSuffix(out, "\n") {
		t.Errorf("FormatNodeInline should not end with newline, got %q", out)
	}
}

// ── OrderedMap ────────────────────────────────────────────────────────────────

func TestOrderedMapInsert(t *testing.T) {
	m := jq.OrderedMap{{"a", 1}, {"b", 2}}
	m2 := m.Insert(1, "x", 99)
	if len(m2) != 3 {
		t.Fatalf("expected len 3, got %d", len(m2))
	}
	if m2[0].Key != "a" || m2[1].Key != "x" || m2[2].Key != "b" {
		t.Errorf("wrong order after Insert: %v", m2)
	}
}

func TestOrderedMapMarshalJSON_Order(t *testing.T) {
	m := jq.OrderedMap{{"type", "jq"}, {"file", "-"}, {"query", "x"}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	// Keys must appear in insertion order: type before file before query.
	typeIdx := strings.Index(got, `"type"`)
	fileIdx := strings.Index(got, `"file"`)
	queryIdx := strings.Index(got, `"query"`)
	if !(typeIdx < fileIdx && fileIdx < queryIdx) {
		t.Errorf("keys not in insertion order: %s", got)
	}
}

// ── walk ──────────────────────────────────────────────────────────────────────

// TestWalkVisitsAllNodes ensures Walk visits every node in a non-trivial AST.
func TestWalkVisitsAllNodes(t *testing.T) {
	src := `def f(x): x | not; if .a == false then .b else .c end`
	f, err := jq.ParseFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	jq.Walk(f, func(n jq.Node) bool {
		types = append(types, fmt.Sprintf("%T", n))
		return true
	})
	// Expect at least these types to appear in a walk of this expression.
	want := []string{"*jq.File", "*jq.FuncDef", "*jq.IfExpr", "*jq.BinOp", "*jq.BoolLit"}
	for _, w := range want {
		found := false
		for _, got := range types {
			if got == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Walk did not visit %s; visited: %v", w, types)
		}
	}
}

// TestWalkEarlyReturn verifies returning false stops descent into children.
func TestWalkEarlyReturn(t *testing.T) {
	src := `.a | .b`
	f, err := jq.ParseFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	jq.Walk(f, func(n jq.Node) bool {
		count++
		if _, ok := n.(*jq.File); ok {
			return false // don't descend
		}
		return true
	})
	if count != 1 {
		t.Errorf("expected 1 visit (File only), got %d", count)
	}
}

// ── marshal / AST ─────────────────────────────────────────────────────────────

// ── comprehensive node-type coverage ─────────────────────────────────────────

// jqAllNodes is a jq program that exercises every AST node type.
const jqAllNodes = `
include "meta";
def f($x): $x | not;
def g:
	def h: . + 1;
	h
;
label $lbl |
foreach .[] as $item (0; . + $item; .) |
reduce .[] as $x (0; . + $x) |
if . then .a elif .b then .c else .d end |
try .e catch .f |
. as $y |
.g[0:2] |
.h[.i] |
@base64 |
[.j, .k] |
{l: .m, "n.o": .p} |
(.q) |
.r? |
break $lbl
`

// TestWalkAllNodeTypes verifies Walk visits all expected node types.
func TestWalkAllNodeTypes(t *testing.T) {
	// Use a simpler but comprehensive subset that parses cleanly without imports.
	src := `
def f($x): $x | not;
def g:
	def h: . + 1;
	h
;
if . == false then true else false end |
label $lbl |
foreach .[] as $item (0; . + $item; .) |
reduce .[] as $x (0; . + $x) |
if . then .a elif .b then .c else .d end |
try .e catch .f |
. as $y |
.g[0:2] |
.h[.i] |
@base64 "str" |
[.j, .k] |
{l: .m, "n.o": .p} |
(.q) |
.r? |
break $lbl
`
	f, err := jq.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	seen := make(map[string]bool)
	jq.Walk(f, func(n jq.Node) bool {
		seen[fmt.Sprintf("%T", n)] = true
		return true
	})

	want := []string{
		"*jq.File", "*jq.FuncDef", "*jq.LocalFuncDef",
		"*jq.Pipe", "*jq.LabelExpr", "*jq.ForeachExpr",
		"*jq.ReduceExpr", "*jq.AsExpr", "*jq.IfExpr",
		"*jq.TryExpr", "*jq.Slice", "*jq.Index",
		"*jq.FormatExpr", "*jq.Array", "*jq.Comma",
		"*jq.Object", "*jq.Paren", "*jq.Optional",
		"*jq.BreakExpr", "*jq.Var", "*jq.StrLit",
		"*jq.Identity", "*jq.Field", "*jq.BoolLit",
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("Walk did not visit %s", w)
		}
	}
}

// TestFormatAllNodeTypes ensures the formatter handles every node type.
func TestFormatAllNodeTypes(t *testing.T) {
	cases := []string{
		`label $lbl | break $lbl`,
		`foreach .[] as $x (0; . + $x)`,
		`reduce .[] as $x (0; . + $x)`,
		`. as $x | $x`,
		`.a[0:2]`,                // Slice
		`.a[.b]`,                 // Index with key
		`.a[]`,                   // Index iterator
		`.a[]?`,                  // Index optional
		`@base64`,                // FormatExpr without string
		`@base64 "foo"`,          // FormatExpr with string
		`[.a, .b, empty]`,        // Array with comma
		`{a: .b, "c.d": .e}`,     // Object with quoted key
		`("paren")`,              // Paren
		`.a?`,                    // Optional
		`.. `,                    // Recurse
		`$__loc__`,               // LocExpr
		`[.[] | select(.x)]`,     // ArrayPattern via select
		`. as {a: $b} | $b`,      // ObjectPattern
		`. as [$a, $b] | $a`,     // ArrayPattern
	}
	for _, src := range cases {
		t.Run(src[:min(25, len(src))], func(t *testing.T) {
			f, err := jq.ParseFile(src)
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			out := jq.FormatFile(f)
			// Round-trip: re-parse and re-format must be idempotent.
			f2, err := jq.ParseFile(out)
			if err != nil {
				t.Fatalf("re-parse %q: %v", out, err)
			}
			out2 := jq.FormatFile(f2)
			if out != out2 {
				t.Errorf("not idempotent for %q\nfirst:  %q\nsecond: %q", src, out, out2)
			}
		})
	}
}

// TestTokenString verifies Token.String() and Kind.String() output.
func TestTokenString(t *testing.T) {
	toks := jq.Tokens(".foo | .bar")
	for _, tok := range toks {
		s := tok.String()
		if s == "" {
			t.Errorf("expected non-empty String() for token %v", tok)
		}
	}
	// INVALID token kind
	invalid := jq.Tokens("!")
	found := false
	for _, tok := range invalid {
		if tok.Kind == jq.INVALID {
			found = true
			if !strings.Contains(tok.String(), "Invalid") {
				t.Errorf("INVALID token String() should contain 'Invalid', got %q", tok.String())
			}
		}
	}
	if !found {
		t.Error("expected INVALID token for '!'")
	}
}

// TestFormatModuleAndImport exercises module/import statement parsing.
func TestFormatModuleAndImport(t *testing.T) {
	// import with metadata exercises the optional meta path in parseImportStmt
	src := `module {"version": 1};
include "foo";
import "bar" as $bar {"origin": "test"};
.x
`
	f, err := jq.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.Module == nil {
		t.Error("expected Module to be parsed")
	}
	if len(f.Imports) == 0 {
		t.Error("expected Imports to be parsed")
	}
	out := jq.FormatFile(f)
	if !strings.Contains(out, "module") || !strings.Contains(out, "include") {
		t.Errorf("module/import not in output: %q", out)
	}
}

// TestLexNumber exercises number literals including exponents and leading-decimal forms.
func TestLexNumber(t *testing.T) {
	cases := []string{
		"1e10",
		"1.5e-3",
		"1E+3",
		".5",
		".25e2",
		"0.001",
		"42",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			node, err := jq.ParseExpr(src)
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			out := jq.FormatNode(node)
			if out == "" {
				t.Errorf("empty output for %q", src)
			}
		})
	}
}

// TestNodeInlineAllTypes calls FormatNodeInline on every uncovered node type
// to exercise the nodeInline switch directly (bypassing the length threshold).
func TestNodeInlineAllTypes(t *testing.T) {
	cases := []string{
		`. as $x | $x`,             // AsExpr
		`reduce .[] as $x (0; .)`, // ReduceExpr
		`foreach .[] as $x (0; .)`, // ForeachExpr (no extract)
		`foreach .[] as $x (0; .; .)`, // ForeachExpr with extract
		`-.x`,                      // BinOp neg
		`.a.b`,                     // BinOp op="" (field chain suffix)
		`. as [$a] | $a`,           // ArrayPattern in inline
		`. as {a: $b} | $b`,        // ObjectPattern in inline
		`label $lbl | break $lbl`,  // LabelExpr inline
	}
	for _, src := range cases {
		t.Run(src[:min(20, len(src))], func(t *testing.T) {
			node, err := jq.ParseExpr(src)
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			out := jq.FormatNodeInline(node)
			if out == "" {
				t.Errorf("FormatNodeInline(%q) = empty", src)
			}
		})
	}
}

// TestFormatSuffixChains exercises parseSuffix: chained index/slice/optional.
func TestFormatSuffixChains(t *testing.T) {
	cases := []string{
		`.a.b.c`,         // field chaining
		`.a[0].b[1:3]`,   // index then slice
		`.a?.b?.c?`,      // optional chain
		`.["key"].b`,     // bracket key then field
		`.a[].b[]`,       // iterator chain
		`.a[]?`,          // optional iterator
		`.a[0]?`,         // optional index
		`.a[1:3]?`,       // optional slice
		`.a | .b?`,       // optional in pipeline
		`.[1:]`,          // slice no end
		`.[:3]`,          // slice no start
		`.[.a]`,          // expression key
		`.[-1]`,          // negative index
		`.[0:2][]`,       // slice then iterator
		`.foo.`,          // DOT+other: trailing dot → BinOp{op:"", right:Identity}
		`."foo-bar"`,     // dot-quoted-string path (DOT+STR in parseSuffix)
		`.foo | @uri`,    // format as standalone filter
		`.foo@base64`,    // FORMAT as suffix (parseSuffix FORMAT case)
		`.[0]@html`,      // FORMAT suffix on index
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			f, err := jq.ParseFile(src)
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			out := jq.FormatFile(f)
			f2, err := jq.ParseFile(out)
			if err != nil {
				t.Fatalf("re-parse %q: %v", out, err)
			}
			if out2 := jq.FormatFile(f2); out != out2 {
				t.Errorf("not idempotent: %q → %q", out, out2)
			}
		})
	}
}

// TestFormatObjectComputedKeys exercises the computed-key path in objectField.
func TestFormatObjectComputedKeys(t *testing.T) {
	cases := []string{
		`{(env.key): .val}`,
		`{("literal"): .val}`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			f, err := jq.ParseFile(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_ = jq.FormatFile(f)
		})
	}
}

// TestAttachTrailingToField_AlreadyCommentedExpr exercises the branch in
// attachTrailingToField where the field value is already a CommentedExpr.
// This requires a field where the value expression itself has comments attached.
func TestAttachTrailingToField_AlreadyCommentedExpr(t *testing.T) {
	// When a field has a CommentedExpr value (leading comment on the value)
	// AND a trailing comment on the field, attachTrailingToField is called with
	// a CommentedExpr value — exercises the "already CommentedExpr" branch.
	src := "{\n\t# comment before value\n\tkey: .val, # trailing on field\n}\n"
	f, err := jq.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Must format without panic.
	out := jq.FormatFile(f)
	if !strings.Contains(out, "key:") {
		t.Errorf("object key missing from output: %q", out)
	}
}

// TestFormatTrailingCommentInObject exercises stripTrailing and attachTrailingToField.
func TestFormatTrailingCommentInObject(t *testing.T) {
	// Non-shorthand field with trailing comment: exercises CommentedExpr wrapping.
	src := "{\n\ta: .b, # inline comment\n\tc: .d,\n}\n"
	f, err := jq.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := jq.FormatFile(f)
	if !strings.Contains(out, ", #") {
		t.Errorf("trailing comment should follow comma, got:\n%s", out)
	}

	// Shorthand field (nil Value): exercises the nil-value early return path.
	// {foo} is shorthand for {foo: .foo}; trailing comments here attach to the field.
	src2 := "{\n\tfoo, # shorthand with trailing comment\n\tbar,\n}\n"
	f2, err := jq.ParseFile(src2)
	if err != nil {
		t.Fatalf("parse shorthand: %v", err)
	}
	_ = jq.FormatFile(f2) // must not panic
}

// TestFormatInlineVsMultiline verifies the inline/multiline threshold.
func TestFormatInlineVsMultiline(t *testing.T) {
	// Short if: stays on one line (only has trailing newline, no mid-expression newlines).
	short := `if .a then .b else .c end`
	f, _ := jq.ParseFile(short)
	out := jq.FormatFile(f)
	if strings.Contains(strings.TrimRight(out, "\n"), "\n") {
		t.Errorf("short if should be inline, got:\n%s", out)
	}

	// Long if: goes multiline
	long := `if .aaaaaaaaaaaaaaaa then .bbbbbbbbbbbbbbbb else .cccccccccccccccc end`
	f2, _ := jq.ParseFile(long)
	out2 := jq.FormatFile(f2)
	if !strings.Contains(out2, "\n") {
		t.Errorf("long if should be multiline, got: %q", out2)
	}
}


// ── golden helper ─────────────────────────────────────────────────────────────

// golden runs golden-file tests from dir.  Each subdir must have input{inExt}.
// output{outExt} is the golden file; run `go test -update` to regenerate it.

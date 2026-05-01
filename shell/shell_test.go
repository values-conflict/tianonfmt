package shell_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"

	"github.com/tianon/fmt/tianonfmt/jq"
	"github.com/tianon/fmt/tianonfmt/shell"
	"mvdan.cc/sh/v3/syntax"
)

// realJQFmt mirrors jqFmtFunc from cmd/tianonfmt: parse and re-format jq
// expressions found embedded in shell scripts.
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


func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// ── format / tidy / AST / errors ─────────────────────────────────────────────

func TestFormat(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.sh", []testutil.Case{
		{Out: "output.sh", Fn: func(src string) (string, error) {
			return shell.Format(src, shell.DetectLang(src), nil)
		}, Idem: true},
		{Out: "output.tidy.sh", Fn: func(src string) (string, error) {
			return shell.FormatWithTidy(src, shell.DetectLang(src), nil)
		}, Idem: true},
		{Out: "output.format-jq.sh", Fn: func(src string) (string, error) {
			return shell.Format(src, shell.DetectLang(src), realJQFmt)
		}, Idem: true},
		{Out: "ast.json", Fn: func(src string) (string, error) {
			f, err := shell.ParseFile(src, shell.DetectLang(src))
			if err != nil {
				return "", err
			}
			b, err := json.MarshalIndent(shell.MarshalFile(f, "input.sh"), "", "\t")
			if err != nil {
				return "", err
			}
			return string(b) + "\n", nil
		}},
	})
}

func TestParseErrors(t *testing.T) {
	testutil.Golden(t, "testdata/errors", "input.sh", []testutil.Case{
		{Fn: func(src string) (string, error) {
			_, err := shell.ParseFile(src, shell.DetectLang(src))
			return "", err
		}},
	})
}

// ── DetectLang ────────────────────────────────────────────────────────────────

func TestDetectLang(t *testing.T) {
	tests := []struct {
		src  string
		want syntax.LangVariant
	}{
		{"#!/bin/sh\necho hi\n", syntax.LangPOSIX},
		{"#!/usr/bin/env bash\n", syntax.LangBash},
		{"#!/bin/bash\n", syntax.LangBash},
		{"#!/usr/bin/env mksh\n", syntax.LangMirBSDKorn},
		{"no shebang\n", syntax.LangBash},
	}
	for _, tt := range tests {
		t.Run(tt.src[:min(len(tt.src), 20)], func(t *testing.T) {
			if got := shell.DetectLang(tt.src); got != tt.want {
				t.Errorf("DetectLang(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

// ── TidyShebang ───────────────────────────────────────────────────────────────

func TestTidyShebang(t *testing.T) {
	tests := []struct{ in, want string }{
		{"#!/bin/bash\necho hi\n", "#!/usr/bin/env bash\necho hi\n"},
		{"#!/bin/sh\necho hi\n", "#!/usr/bin/env bash\necho hi\n"},
		{"#!/usr/bin/env bash\necho hi\n", "#!/usr/bin/env bash\necho hi\n"},
		{"#!/bin/bash", "#!/usr/bin/env bash"},
		{"no shebang\n", "no shebang\n"},
	}
	for _, tt := range tests {
		t.Run(tt.in[:min(len(tt.in), 20)], func(t *testing.T) {
			if got := shell.TidyShebang(tt.in); got != tt.want {
				t.Errorf("TidyShebang(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ── FlattenAndChain ───────────────────────────────────────────────────────────

func TestFlattenAndChain(t *testing.T) {
	tests := []struct {
		src   string
		count int // expected len; -1 = nil
	}{
		{"cmd1 && cmd2 && cmd3", 3},
		{"cmd1 && cmd2", 2},
		{"cmd1", 1},          // single command: returns [stmt]
		{"cmd1 || cmd2", -1}, // || at root: nil
		{"cmd1 | cmd2", -1},  // pipeline is a BinaryCmd with pipe op, not &&: nil
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			f, err := shell.ParseFile(tt.src, syntax.LangBash)
			if err != nil || len(f.Stmts) != 1 {
				t.Skip("parse error or multiple stmts")
			}
			got := shell.FlattenAndChain(f.Stmts[0])
			if tt.count < 0 {
				if got != nil {
					t.Errorf("expected nil, got %d stmts", len(got))
				}
			} else if len(got) != tt.count {
				t.Errorf("expected %d stmts, got %d", tt.count, len(got))
			}
		})
	}
}

// ── ApplyTidy ─────────────────────────────────────────────────────────────────

func TestApplyTidy_Backtick(t *testing.T) {
	f, _ := shell.ParseFile("result=`date`\n", syntax.LangBash)
	shell.ApplyTidy(f)
	out, _ := shell.FormatStmtOneLine(f.Stmts[0])
	if !strings.Contains(out, "$(") || strings.Contains(out, "`") {
		t.Errorf("backtick not converted to $(): %q", out)
	}
}

func TestApplyTidy_FunctionKeyword(t *testing.T) {
	f, _ := shell.ParseFile("function greet { echo hi; }\n", syntax.LangBash)
	shell.ApplyTidy(f)
	var buf strings.Builder
	_ = buf
	out, _ := shell.FormatStmtOneLine(f.Stmts[0])
	if strings.Contains(out, "function") {
		t.Errorf("function keyword not removed: %q", out)
	}
}

func TestApplyTidy_BracketDoubleEquals(t *testing.T) {
	f, _ := shell.ParseFile(`if [ "$a" == "$b" ]; then echo same; fi`+"\n", syntax.LangBash)
	shell.ApplyTidy(f)
	out, _ := shell.FormatStmtOneLine(f.Stmts[0])
	if strings.Contains(out, "==") {
		t.Errorf("[ == ] not converted to [ = ]: %q", out)
	}
	if !strings.Contains(out, `"$a" = "$b"`) {
		t.Errorf("expected [ = ] in: %q", out)
	}
}

func TestApplyTidy_OrTrue(t *testing.T) {
	f, _ := shell.ParseFile("cmd || true\n", syntax.LangBash)
	shell.ApplyTidy(f)
	out, _ := shell.FormatStmtOneLine(f.Stmts[0])
	if out != "cmd || :" {
		t.Errorf("got %q, want %q", out, "cmd || :")
	}
}

func TestApplyTidy_Which(t *testing.T) {
	f, _ := shell.ParseFile("which docker\n", syntax.LangBash)
	shell.ApplyTidy(f)
	out, _ := shell.FormatStmtOneLine(f.Stmts[0])
	if out != "command -v docker" {
		t.Errorf("got %q, want %q", out, "command -v docker")
	}
}

func TestApplyTidy_WhichWithFlags_Unchanged(t *testing.T) {
	// which -a has no command -v equivalent; must not be modified
	f, _ := shell.ParseFile("which -a docker\n", syntax.LangBash)
	shell.ApplyTidy(f)
	out, _ := shell.FormatStmtOneLine(f.Stmts[0])
	if out != "which -a docker" {
		t.Errorf("got %q, want unchanged %q", out, "which -a docker")
	}
}

// ── FormatRUN ─────────────────────────────────────────────────────────────────

func TestFormatRUN_BasicIndent(t *testing.T) {
	// Continuation lines at depth 0 get 1 tab.
	lines := []string{"\tapt-get update; \\", "\tapt-get install -y curl"}
	got := shell.FormatRUN(lines, nil)
	for _, l := range got {
		trimmed := strings.TrimRight(l, " \\")
		if !strings.HasPrefix(trimmed, "\t") {
			t.Errorf("expected 1-tab indent, got %q", l)
		}
	}
}

func TestFormatRUN_IfBlock(t *testing.T) {
	// Commands inside if/then get 2 tabs; fi gets 1 tab.
	lines := []string{
		"\tif [ \"$x\" = \"foo\" ]; then \\",
		"\t\techo ok; \\",
		"\tfi",
	}
	got := shell.FormatRUN(lines, nil)
	if len(got) < 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}
	// if line: 1 tab
	if !strings.HasPrefix(got[0], "\tif ") {
		t.Errorf("if line wrong indent: %q", got[0])
	}
	// echo line inside then: 2 tabs
	if !strings.HasPrefix(strings.TrimRight(got[1], " \\"), "\t\techo") {
		t.Errorf("echo line wrong indent: %q", got[1])
	}
	// fi: 1 tab
	if !strings.HasPrefix(got[2], "\tfi") {
		t.Errorf("fi line wrong indent: %q", got[2])
	}
}

func TestFormatRUN_CommentAtColumnZero(t *testing.T) {
	lines := []string{"\tapt-get update; \\", "# a comment", "\tapt-get install -y curl"}
	got := shell.FormatRUN(lines, nil)
	for _, l := range got {
		if strings.HasPrefix(l, "#") {
			// Comment lines must be at column 0 (no tab prefix).
			return
		}
	}
	t.Error("no comment line found at column 0")
}

func TestFormatRUN_Empty(t *testing.T) {
	got := shell.FormatRUN(nil, nil)
	if got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestFormatRUN_Idempotent(t *testing.T) {
	lines := []string{"\tapt-get update; \\", "# comment", "\tapt-get install -y curl"}
	first := shell.FormatRUN(lines, nil)
	second := shell.FormatRUN(first, nil)
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Errorf("FormatRUN not idempotent\nfirst:  %v\nsecond: %v", first, second)
	}
}

// ── NormalizeSetFlags / FormatWithPedantic ────────────────────────────────────

func TestNormalizeSetFlags_Bash(t *testing.T) {
	cases := []struct{ in, want string }{
		{"set -e", "set -Eeuo pipefail"},
		{"set -eu", "set -Eeuo pipefail"},
		{"set -eux", "set -Eeuo pipefail"},    // top-level: -x stripped
		{"set -ex", "set -Eeuo pipefail"},    // top-level: -x stripped (global set -x is Wrong)
		{"\tset -ex", "\tset -Eeuxo pipefail"}, // indented (inside block): -x preserved
		{"set -Eeuo pipefail", "set -Eeuo pipefail"}, // already canonical
		{"echo hi", "echo hi"},             // not a set command
		{"set --", "set --"},               // positional args reset, leave alone
	}
	for _, tt := range cases {
		t.Run(tt.in, func(t *testing.T) {
			got := shell.NormalizeSetFlags(tt.in, syntax.LangBash)
			if got != tt.want {
				t.Errorf("NormalizeSetFlags(%q, bash) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeSetFlags_POSIX(t *testing.T) {
	cases := []struct{ in, want string }{
		{"set -e", "set -eu"},
		{"set -eu", "set -eu"},             // already canonical
		{"set -ex", "set -eu"},             // top-level: -x stripped (global set -x is Wrong)
		{"set -Eeuo pipefail", "set -eu"},  // strips bash-only flags
		{"\tset -ex", "\tset -eux"},        // indented (inside block): -x preserved
	}
	for _, tt := range cases {
		t.Run(tt.in, func(t *testing.T) {
			got := shell.NormalizeSetFlags(tt.in, syntax.LangPOSIX)
			if got != tt.want {
				t.Errorf("NormalizeSetFlags(%q, posix) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatWithPedantic_SetNormalization(t *testing.T) {
	src := "#!/usr/bin/env bash\nset -e\necho hi\n"
	out, err := shell.FormatWithPedantic(src, syntax.LangBash, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set -Eeuo pipefail") {
		t.Errorf("expected set -Eeuo pipefail in output, got: %q", out)
	}
}

func TestFormatWithPedantic_AlreadyCanonical(t *testing.T) {
	src := "#!/usr/bin/env bash\nset -Eeuo pipefail\necho hi\n"
	out, err := shell.FormatWithPedantic(src, syntax.LangBash, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set -Eeuo pipefail") {
		t.Errorf("canonical form should be preserved: %q", out)
	}
}

// ── FormatRUN with jq callback ────────────────────────────────────────────────

// jqFmtPassthrough returns the expression unchanged — used when we only care
// that detection fired (or didn't fire), not the rewrite itself.
func jqFmtPassthrough(expr string, _ bool) string { return expr }

// jqFmtUpperDot replaces `.foo` with `.FOO` so we can verify the callback ran.
func jqFmtUpperDot(expr string, _ bool) string {
	return strings.ReplaceAll(expr, ".foo", ".FOO")
}

func TestFormatRUN_JQ_StartOfLine(t *testing.T) {
	// Plain `jq '...'` at the start of a continuation line.
	lines := []string{"\tjq '.foo' /data.json"}
	got := shell.FormatRUN(lines, jqFmtUpperDot)
	if len(got) != 1 || !strings.Contains(got[0], ".FOO") {
		t.Errorf("jq at start of line not reformatted: %v", got)
	}
}

func TestFormatRUN_JQ_DollarParenPattern(t *testing.T) {
	// `$(jq '...')` — tests the hasJQ detection fix for subshell pattern.
	lines := []string{"\tresult=$(jq '.foo' /data.json)"}
	got := shell.FormatRUN(lines, jqFmtUpperDot)
	if len(got) != 1 || !strings.Contains(got[0], ".FOO") {
		t.Errorf("$(jq '...') not reformatted: %v", got)
	}
}

func TestFormatRUN_JQ_PreservesArgsAfterQuote(t *testing.T) {
	// Filename arg after the closing quote must survive reformatting.
	lines := []string{"\tjq '.foo' /input.json > /output.json"}
	got := shell.FormatRUN(lines, jqFmtPassthrough)
	if len(got) != 1 || !strings.Contains(got[0], "/output.json") {
		t.Errorf("args after closing quote dropped: %v", got)
	}
}

func TestFormatRUN_JQ_PreservesHereString(t *testing.T) {
	// `<<<\"$var\"` after the closing quote must survive reformatting.
	lines := []string{`	jq '.foo' <<<"$var"`}
	got := shell.FormatRUN(lines, jqFmtPassthrough)
	if len(got) != 1 || !strings.Contains(got[0], `<<<"$var"`) {
		t.Errorf("here-string after closing quote dropped: %v", got)
	}
}

func TestFormatRUN_JQ_InPipeline(t *testing.T) {
	// `... | jq '...'` — jq appears mid-line after a pipe.
	lines := []string{"\tcat /data.json | jq '.foo'"}
	got := shell.FormatRUN(lines, jqFmtUpperDot)
	if len(got) != 1 || !strings.Contains(got[0], ".FOO") {
		t.Errorf("jq in pipeline not reformatted: %v", got)
	}
}

func TestFormatRUN_JQ_NonJQSingleQuote_NotTouched(t *testing.T) {
	// A single-quoted string in a non-jq command must not be touched.
	lines := []string{"\techo 'hello world'"}
	got := shell.FormatRUN(lines, jqFmtUpperDot)
	if len(got) != 1 || got[0] != "\techo 'hello world'" {
		t.Errorf("non-jq single-quoted string was modified: %v", got)
	}
}

// ── Format with jq callback ───────────────────────────────────────────────────

func TestFormatWithJQCallback_NoChange(t *testing.T) {
	// When jqFmt returns the same expression (already canonical), reformatSglQuoted
	// must leave the value unchanged — exercises the "formatted == expr" no-op path.
	src := "#!/usr/bin/env bash\nresult=$(jq '.foo' <<<\"$input\")\n"
	jqFmt := func(expr string, inline bool) string {
		return expr // passthrough — always returns the same value
	}
	out, err := shell.Format(src, syntax.LangBash, jqFmt)
	if err != nil {
		t.Fatal(err)
	}
	// The jq expression should be unchanged in the output.
	if !strings.Contains(out, "'.foo'") {
		t.Errorf("expression changed unexpectedly: %q", out)
	}
}

func TestFormatWithJQCallback_Empty(t *testing.T) {
	// Empty single-quoted jq expression: jq '' — triggers the empty-string early return
	src := "#!/usr/bin/env bash\nresult=$(jq '' <<<\"$input\")\n"
	jqFmt := func(expr string, inline bool) string { return expr }
	out, err := shell.Format(src, syntax.LangBash, jqFmt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "jq") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestFormatWithJQCallback_Multiline(t *testing.T) {
	// A multi-line single-quoted jq expression triggers reformatSglQuoted's
	// multi-line path (the one that strips and re-adds indentation).
	src := "#!/usr/bin/env bash\nresult=$(jq '\n\t.foo\n\t| .bar\n' <<<\"$input\")\n"
	jqFmt := func(expr string, inline bool) string {
		if inline {
			return strings.TrimSpace(expr)
		}
		return expr // passthrough for multi-line
	}
	out, err := shell.Format(src, syntax.LangBash, jqFmt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, ".foo") {
		t.Errorf("jq expr not in output: %q", out)
	}
}

func TestFormatWithJQCallback(t *testing.T) {
	src := "#!/usr/bin/env bash\nresult=$(jq -r '.foo' <<<\"$input\")\n"
	jqFmt := func(expr string, inline bool) string {
		if expr == ".foo" {
			return ".foo" // passthrough
		}
		return ""
	}
	out, err := shell.Format(src, syntax.LangBash, jqFmt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, ".foo") {
		t.Errorf("jq expr not preserved: %q", out)
	}
}


// ── token-level format preservation ──────────────────────────────────────────

// scanShSingleQuote scans a '...' shell string from src[start].
func scanShSingleQuote(src string, start int) int {
	i := start + 1
	for i < len(src) && src[i] != '\'' {
		i++
	}
	if i < len(src) {
		i++
	}
	return i
}

// scanShAnsiC scans a $'...' ANSI-C quoted string starting at the ' in src[start].
func scanShAnsiC(src string, start int) int {
	i := start + 1 // skip opening '
	for i < len(src) {
		if src[i] == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		if src[i] == '\'' {
			return i + 1
		}
		i++
	}
	return i
}

// scanShDoubleQuote scans a "..." shell string, handling \" escapes and
// $(…) / `…` command substitutions so nested quotes don't prematurely close it.
func scanShDoubleQuote(src string, start int) int {
	i := start + 1
	for i < len(src) {
		switch src[i] {
		case '\\':
			if i+1 < len(src) {
				i += 2
			} else {
				i++
			}
		case '"':
			return i + 1
		case '$':
			if i+1 < len(src) && src[i+1] == '(' {
				i = scanShParenDepth(src, i+2)
			} else if i+1 < len(src) && src[i+1] == '\'' {
				i = scanShAnsiC(src, i+1)
			} else {
				i++
			}
		case '`':
			i = scanShBacktick(src, i+1)
		default:
			i++
		}
	}
	return i
}

// scanShParenDepth scans a balanced (…) region starting at src[start]
// (just after the opening '('), handling nested quotes.
func scanShParenDepth(src string, start int) int {
	i := start
	depth := 1
	for i < len(src) && depth > 0 {
		switch src[i] {
		case '(':
			depth++
			i++
		case ')':
			depth--
			i++
		case '\'':
			i = scanShSingleQuote(src, i)
		case '"':
			i = scanShDoubleQuote(src, i)
		case '`':
			i = scanShBacktick(src, i+1)
		case '$':
			if i+1 < len(src) && src[i+1] == '\'' {
				i = scanShAnsiC(src, i+1)
			} else {
				i++
			}
		case '\\':
			if i+1 < len(src) {
				i += 2
			} else {
				i++
			}
		default:
			i++
		}
	}
	return i
}

// scanShBacktick scans a `…` region starting at src[start] (just after the
// opening backtick).
func scanShBacktick(src string, start int) int {
	i := start
	for i < len(src) && src[i] != '`' {
		if src[i] == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		i++
	}
	if i < len(src) {
		i++ // consume closing backtick
	}
	return i
}

func shIsWordBreak(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r',
		'\'', '"', '`',
		';', '|', '&',
		'(', ')', '{', '}',
		'<', '>',
		'#':
		return true
	}
	return false
}

// tokenizeShell splits shell source into non-whitespace tokens, discarding
// comments.  Standalone ';' (statement separator) is also discarded — the
// shell formatter replaces it with a newline, making both forms token-equivalent.
// ';;' (case terminator) and multi-char operators (&&, ||, >>, etc.) are kept.
func tokenizeShell(src string) []string {
	var tokens []string
	i := 0
	for i < len(src) {
		c := src[i]
		// whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		// comment
		if c == '#' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// single-quoted string
		if c == '\'' {
			j := scanShSingleQuote(src, i)
			tokens = append(tokens, src[i:j])
			i = j
			continue
		}
		// double-quoted string
		if c == '"' {
			j := scanShDoubleQuote(src, i)
			tokens = append(tokens, src[i:j])
			i = j
			continue
		}
		// $'...' ANSI-C string or $(...) substitution
		if c == '$' && i+1 < len(src) {
			if src[i+1] == '\'' {
				j := scanShAnsiC(src, i+1)
				tokens = append(tokens, src[i:j])
				i = j
				continue
			}
			if src[i+1] == '(' {
				// $(…) — treat as start of a word; fall through to word scanner
			}
		}
		// backtick substitution (inline)
		if c == '`' {
			j := scanShBacktick(src, i+1)
			tokens = append(tokens, src[i:j])
			i = j
			continue
		}
		// ;; and ;& and ;;& (case terminators — keep)
		if c == ';' {
			if i+2 < len(src) && src[i+1] == ';' && src[i+2] == '&' {
				tokens = append(tokens, ";;&")
				i += 3
				continue
			}
			if i+1 < len(src) && src[i+1] == ';' {
				tokens = append(tokens, ";;")
				i += 2
				continue
			}
			if i+1 < len(src) && src[i+1] == '&' {
				tokens = append(tokens, ";&")
				i += 2
				continue
			}
			// standalone ';' — discard (formatter replaces with newline)
			i++
			continue
		}
		// multi-char operators
		if c == '&' && i+1 < len(src) && src[i+1] == '&' {
			tokens = append(tokens, "&&")
			i += 2
			continue
		}
		if c == '|' && i+1 < len(src) && src[i+1] == '|' {
			tokens = append(tokens, "||")
			i += 2
			continue
		}
		if c == '|' && i+1 < len(src) && src[i+1] == '&' {
			tokens = append(tokens, "|&")
			i += 2
			continue
		}
		if c == '>' && i+1 < len(src) && src[i+1] == '>' {
			tokens = append(tokens, ">>")
			i += 2
			continue
		}
		if c == '<' && i+1 < len(src) && src[i+1] == '<' {
			if i+2 < len(src) && src[i+2] == '<' {
				tokens = append(tokens, "<<<")
				i += 3
				continue
			}
			tokens = append(tokens, "<<")
			i += 2
			continue
		}
		if c == '>' && i+1 < len(src) && src[i+1] == '&' {
			tokens = append(tokens, ">&")
			i += 2
			continue
		}
		if c == '<' && i+1 < len(src) && src[i+1] == '&' {
			tokens = append(tokens, "<&")
			i += 2
			continue
		}
		// single-char operators
		if c == '|' || c == '&' || c == '(' || c == ')' || c == '{' || c == '}' || c == '<' || c == '>' {
			tokens = append(tokens, string(c))
			i++
			continue
		}
		// word — everything up to the next break char, embedding $(…) and `…`
		j := i
		for j < len(src) && !shIsWordBreak(src[j]) {
			if src[j] == '$' && j+1 < len(src) && src[j+1] == '(' {
				j = scanShParenDepth(src, j+2)
			} else if src[j] == '$' && j+1 < len(src) && src[j+1] == '{' {
				// ${…} variable expansion — scan to matching }
				depth := 1
				j += 2
				for j < len(src) && depth > 0 {
					if src[j] == '{' {
						depth++
					} else if src[j] == '}' {
						depth--
					}
					j++
				}
			} else {
				j++
			}
		}
		if j > i {
			tokens = append(tokens, src[i:j])
			i = j
		} else {
			// unrecognised single char — emit and advance
			tokens = append(tokens, string(src[i]))
			i++
		}
	}
	return tokens
}

// shNormalizeArith strips spaces inside $((…)) arithmetic expansions embedded
// in a word token.  The formatter adds spaces around operators inside $((...))
// while the source may have none; stripping them makes both canonical.
func shNormalizeArith(tok string) string {
	if !strings.Contains(tok, "$((") {
		return tok
	}
	var out []byte
	i := 0
	for i < len(tok) {
		if i+3 <= len(tok) && tok[i] == '$' && tok[i+1] == '(' && tok[i+2] == '(' {
			out = append(out, '$', '(', '(')
			i += 3
			depth := 2
			for i < len(tok) && depth > 0 {
				c := tok[i]
				if c == '(' {
					depth++
					out = append(out, c)
				} else if c == ')' {
					depth--
					out = append(out, c)
				} else if c == ' ' || c == '\t' {
					// strip spaces inside arithmetic expressions
				} else {
					out = append(out, c)
				}
				i++
			}
		} else {
			out = append(out, tok[i])
			i++
		}
	}
	return string(out)
}

// shExpandSubshells recursively tokenizes $(…) subshell contents inside any
// token — whether a "…" double-quoted string or a bare word.  This is necessary
// because the formatter adjusts whitespace inside $(…): it removes leading/
// trailing spaces (e.g. $( set ) → $(set)) and reformats multi-line subshells
// (pipe placement, indentation).  Tokenizing the inner shell code the same way
// as the outer code makes both forms produce the same token sequence.
//
// For "…" strings the outer quotes are stripped before scanning; the opening
// and closing quote characters are not emitted as separate tokens.
//
// Standalone '\' tokens (backslash line-continuation artifacts added by the
// formatter) are filtered out from inner token sequences.
func shExpandSubshells(tok string) []string {
	if !strings.Contains(tok, "$(") {
		return []string{tok}
	}
	i := 0
	end := len(tok)
	if strings.HasPrefix(tok, `"`) && end >= 2 && tok[end-1] == '"' {
		i = 1   // skip opening "
		end--   // stop before closing "
	}
	var parts []string
	litStart := i
	for i < end {
		c := tok[i]
		if c == '\\' && i+1 < end {
			i += 2
			continue
		}
		if c == '$' && i+1 < end && tok[i+1] == '(' {
			if i > litStart {
				parts = append(parts, tok[litStart:i])
			}
			i += 2 // skip $(
			depth := 1
			contentStart := i
			for i < end && depth > 0 {
				switch tok[i] {
				case '(':
					depth++
				case ')':
					depth--
				case '\'':
					i++
					for i < end && tok[i] != '\'' {
						i++
					}
				case '\\':
					if i+1 < end {
						i++
					}
				}
				i++
			}
			shellCode := tok[contentStart : i-1]
			for _, t := range tokenizeShell(shellCode) {
				if t != `\` {
					parts = append(parts, t)
				}
			}
			litStart = i
			continue
		}
		i++
	}
	if end > litStart {
		parts = append(parts, tok[litStart:end])
	}
	if len(parts) == 0 {
		return []string{tok}
	}
	return parts
}

// normalizeShell returns a canonical token sequence for semantic comparison.
// Known mechanical rewrites applied beyond pure whitespace:
//  1. Standalone ';' discarded (formatter converts "{ cmd; cmd; }" to multi-line).
//  2. Spaces inside $((…)) arithmetic expansions stripped (formatter adds them).
//  3. $(…) subshell contents — inside both "…" strings and bare words — are
//     recursively tokenized so that whitespace changes (leading/trailing spaces,
//     indentation, pipe-placement) inside subshells are normalized away.
//  4. Standalone '\' tokens (backslash line-continuation artifacts) filtered out.
func normalizeShell(src string) string {
	toks := tokenizeShell(src)
	var result []string
	for _, tok := range toks {
		if tok == `\` {
			continue // backslash line-continuation artifact
		}
		tok = shNormalizeArith(tok)
		result = append(result, shExpandSubshells(tok)...)
	}
	return strings.Join(result, " ")
}

// TestFormatPreservesTokens verifies that shell.Format does not silently alter
// the program beyond the known mechanical transformations (semicolon removal,
// arithmetic spacing, and subshell whitespace normalization inside strings).
// Expected value is derived from raw input text; no golden file is used.
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
			src, err := os.ReadFile(filepath.Join("testdata/format", d.Name(), "input.sh"))
			if err != nil {
				t.Skip("no input.sh")
				return
			}
			formatted, err := shell.Format(string(src), shell.DetectLang(string(src)), nil)
			if err != nil {
				t.Skipf("format error: %v", err)
				return
			}
			normIn := normalizeShell(string(src))
			normOut := normalizeShell(formatted)
			if normIn != normOut {
				t.Errorf("normalizeShell(format(input)) != normalizeShell(input)\n\nnorm(input):\n%s\n\nnorm(format):\n%s",
					normIn, normOut)
			}
		})
	}
}

// ── golden helper ─────────────────────────────────────────────────────────────


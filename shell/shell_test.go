package shell_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
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


// ── golden helper ─────────────────────────────────────────────────────────────


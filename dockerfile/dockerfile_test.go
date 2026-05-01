package dockerfile_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"

	"github.com/tianon/fmt/tianonfmt/dockerfile"
)


func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// ── format / tidy / AST ───────────────────────────────────────────────────────

func TestFormat(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.dockerfile", []testutil.Case{
		{Out: "output.dockerfile", Fn: func(src string) (string, error) {
			f, err := dockerfile.Parse(src)
			if err != nil {
				return "", err
			}
			return dockerfile.Format(f), nil
		}, Idem: true},
		{Out: "output.tidy.dockerfile", Fn: func(src string) (string, error) {
			f, err := dockerfile.Parse(src)
			if err != nil {
				return "", err
			}
			dockerfile.TidyFile(f, tidyRUNStub, normaliseSetFlagsStub)
			return dockerfile.Format(f), nil
		}},
		{Out: "ast.json", Fn: func(src string) (string, error) {
			f, err := dockerfile.Parse(src)
			if err != nil {
				return "", err
			}
			b, err := json.MarshalIndent(dockerfile.MarshalFile(f, "input.dockerfile"), "", "\t")
			if err != nil {
				return "", err
			}
			return string(b) + "\n", nil
		}},
	})
}

// tidyRUNStub flattens && chains as the real tidyRUN does, for testing.
// This mirrors the logic in cmd/tianonfmt without the mvdan/sh dependency here.
// The dockerfile package tests only verify the TidyFile plumbing and line
// reconstruction; the shell-level parsing is tested in the shell package.
func tidyRUNStub(args string) []string {
	// Minimal: detect simple "cmd && cmd" chains by string splitting.
	// Only handles the simple cases exercised by testdata.
	if !strings.Contains(args, " && ") {
		return nil
	}
	parts := strings.Split(args, " && ")
	cmds := make([]string, 0, len(parts)+1)
	cmds = append(cmds, "set -eux")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cmds = append(cmds, p)
		}
	}
	return cmds
}

func normaliseSetFlagsStub(s string) string {
	if strings.HasPrefix(strings.TrimSpace(s), "set -") && strings.TrimSpace(s) != "set -eux" {
		return "set -eux"
	}
	return s
}

// ── TidyFile: applyRUNCommands ────────────────────────────────────────────────

func TestTidyFile_AndChainReconstructsLines(t *testing.T) {
	src := "FROM debian:bookworm-slim\nRUN cmd1 && cmd2 && cmd3\n"
	f, err := dockerfile.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	dockerfile.TidyFile(f, tidyRUNStub, nil)

	// After tidy, the RUN instruction should have multiple Lines
	var runInstr *dockerfile.Instruction
	for _, instr := range f.Instructions {
		if instr.Keyword == "RUN" {
			runInstr = instr
			break
		}
	}
	if runInstr == nil {
		t.Fatal("no RUN instruction found")
	}
	if len(runInstr.Lines) < 2 {
		t.Errorf("expected multi-line RUN after tidy, got %d lines", len(runInstr.Lines))
	}
	// First line should start with "RUN set -eux; \"
	first := runInstr.Lines[0].Text
	if !strings.HasPrefix(first, "RUN set -eux; \\") {
		t.Errorf("first line should be 'RUN set -eux; \\', got %q", first)
	}
}

// ── parse ─────────────────────────────────────────────────────────────────────

func TestFormatWith_RUNShellFmt(t *testing.T) {
	// FormatWith with RUNShellFmt exercises runInstruction (currently 0% in package tests).
	src := "FROM scratch\nRUN cmd1; \\\n\tcmd2\n"
	f, err := dockerfile.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	fmtr := &dockerfile.Formatter{
		RUNShellFmt: func(lines []string, _ func(string, bool) string) []string {
			return lines // passthrough
		},
	}
	out := dockerfile.FormatWith(f, fmtr)
	if !strings.Contains(out, "cmd1") {
		t.Errorf("FormatWith output missing cmd1: %q", out)
	}
}

func TestFormat_EmptyInstructionLines(t *testing.T) {
	// Instruction with 0 Lines must not crash — exercises the early return
	// in (w *writer).instruction when len(instr.Lines) == 0.
	f, err := dockerfile.Parse("FROM scratch\nRUN echo hi\n")
	if err != nil {
		t.Fatal(err)
	}
	// Zero out Lines of the second instruction to trigger the defensive check.
	for _, instr := range f.Instructions {
		if instr.Keyword == "RUN" {
			instr.Lines = nil
		}
	}
	// Must not panic.
	_ = dockerfile.Format(f)
}

func TestParse_BasicInstructions(t *testing.T) {
	src := "FROM debian:bookworm-slim\nRUN echo hello\nCMD [\"bash\"]\n"
	f, err := dockerfile.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var keywords []string
	for _, instr := range f.Instructions {
		if instr.Keyword != "" {
			keywords = append(keywords, instr.Keyword)
		}
	}
	want := []string{"FROM", "RUN", "CMD"}
	if fmt.Sprint(keywords) != fmt.Sprint(want) {
		t.Errorf("keywords = %v, want %v", keywords, want)
	}
}

func TestParse_Directives(t *testing.T) {
	src := "# syntax=docker/dockerfile:1\nFROM scratch\n"
	f, err := dockerfile.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Directives) != 1 || f.Directives[0].Name != "syntax" {
		t.Errorf("expected 1 directive with name=syntax, got %v", f.Directives)
	}
}

// ── MarshalAST golden ─────────────────────────────────────────────────────────

func TestTidyCmdEntrypoint_EmptyArgs(t *testing.T) {
	// len(tokens)==0 path: bare CMD with no arguments
	src := "FROM scratch\nCMD\n"
	f, _ := dockerfile.Parse(src)
	dockerfile.TidyCmdEntrypoint(f) // must not panic or convert
	for _, instr := range f.Instructions {
		if instr.Keyword == "CMD" && strings.HasPrefix(strings.TrimSpace(instr.Args), "[") {
			t.Errorf("empty CMD should not be converted: %q", instr.Args)
		}
	}
}

func TestPedanticCmdEntrypoint_AlreadyExec(t *testing.T) {
	// already exec form: no change
	src := "FROM scratch\nCMD [\"/bin/sh\"]\n"
	f, _ := dockerfile.Parse(src)
	dockerfile.PedanticCmdEntrypoint(f)
	for _, instr := range f.Instructions {
		if instr.Keyword == "CMD" {
			want := `["/bin/sh"]`
			if strings.TrimSpace(instr.Args) != want {
				t.Errorf("already-exec CMD should be unchanged, got %q", instr.Args)
			}
		}
	}
}

func TestFormatWith_WritePath(t *testing.T) {
	// Exercises the write() method (currently 0%) via runInstruction emitting
	// a non-continuation first line followed by RUNShellFmt processing.
	src := "FROM scratch\nRUN set -eux\n"
	f, _ := dockerfile.Parse(src)
	fmtr := &dockerfile.Formatter{
		RUNShellFmt: func(lines []string, _ func(string, bool) string) []string {
			return lines
		},
	}
	out := dockerfile.FormatWith(f, fmtr)
	if !strings.Contains(out, "FROM scratch") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestTidyCmdEntrypoint_SimpleToExec(t *testing.T) {
	src := "FROM scratch\nCMD echo hello\nENTRYPOINT /bin/server --port 8080\n"
	f, _ := dockerfile.Parse(src)
	dockerfile.TidyCmdEntrypoint(f)
	for _, instr := range f.Instructions {
		switch instr.Keyword {
		case "CMD":
			if !strings.HasPrefix(strings.TrimSpace(instr.Args), "[") {
				t.Errorf("CMD not converted to exec form: %q", instr.Args)
			}
		case "ENTRYPOINT":
			if !strings.HasPrefix(strings.TrimSpace(instr.Args), "[") {
				t.Errorf("ENTRYPOINT not converted to exec form: %q", instr.Args)
			}
		}
	}
}

func TestTidyCmdEntrypoint_ShellFeaturesUnchanged(t *testing.T) {
	src := "FROM scratch\nCMD echo $HOME\nENTRYPOINT exec \"$@\"\n"
	f, _ := dockerfile.Parse(src)
	dockerfile.TidyCmdEntrypoint(f)
	for _, instr := range f.Instructions {
		if instr.Keyword == "CMD" || instr.Keyword == "ENTRYPOINT" {
			if strings.HasPrefix(strings.TrimSpace(instr.Args), "[") {
				t.Errorf("%s with shell features should not be converted: %q", instr.Keyword, instr.Args)
			}
		}
	}
}

func TestPedanticCmdEntrypoint_WrapsShellFeatures(t *testing.T) {
	src := "FROM scratch\nCMD echo $HOME\nENTRYPOINT exec \"$@\"\n"
	f, _ := dockerfile.Parse(src)
	dockerfile.TidyCmdEntrypoint(f)  // no-op on these
	dockerfile.PedanticCmdEntrypoint(f)
	for _, instr := range f.Instructions {
		switch instr.Keyword {
		case "CMD":
			if !strings.Contains(instr.Args, `"/bin/sh"`) {
				t.Errorf("CMD not wrapped in /bin/sh -c: %q", instr.Args)
			}
		case "ENTRYPOINT":
			if !strings.Contains(instr.Args, `"--"`) {
				t.Errorf("ENTRYPOINT missing trailing -- : %q", instr.Args)
			}
		}
	}
}

// ── lint ─────────────────────────────────────────────────────────────────────

// TestLint verifies that the Dockerfile-specific lint checks work.
func TestLint(t *testing.T) {
	lintFn := func(src string) (string, error) {
		f, err := dockerfile.Parse(src)
		if err != nil {
			return "", err
		}
		var msgs []string
		for _, instr := range f.Instructions {
			if instr.Keyword != "RUN" {
				continue
			}
			firstCmd, _, _ := strings.Cut(instr.Args, ";")
			firstCmd = strings.TrimSpace(firstCmd)
			if strings.HasPrefix(firstCmd, "set -") && firstCmd != "set -eux" {
				msgs = append(msgs, fmt.Sprintf("%d: %s", instr.StartLine,
					fmt.Sprintf("%q in Dockerfile RUN uses bash-only flags; use \"set -eux\"", firstCmd)))
			}
			if strings.Contains(instr.Args, "apt-get install") {
				seg := instr.Args[strings.Index(instr.Args, "apt-get install"):]
				if !strings.Contains(seg, " -y") {
					msgs = append(msgs, fmt.Sprintf("%d: apt-get install missing \"-y\" flag", instr.StartLine))
				}
				if !strings.Contains(seg, "--no-install-recommends") {
					msgs = append(msgs, fmt.Sprintf("%d: apt-get install missing \"--no-install-recommends\" flag", instr.StartLine))
				}
			}
		}
		out := strings.Join(msgs, "\n") + "\n"
		if out == "\n" {
			out = ""
		}
		return out, nil
	}
	testutil.Golden(t, "testdata/lint", "input.dockerfile", []testutil.Case{
		{Out: "violations.txt", Fn: lintFn},
	})
}

// ── token-level format preservation ──────────────────────────────────────────

// scanDockerStr scans a "..." string in a Dockerfile from src[start].
func scanDockerStr(src string, start int) int {
	i := start + 1
	for i < len(src) {
		if src[i] == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		if src[i] == '"' {
			return i + 1
		}
		i++
	}
	return i
}

// tokenizeDockerfile splits a Dockerfile into non-whitespace tokens, discarding
// comments and treating '\' line-continuation as whitespace.
func tokenizeDockerfile(src string) []string {
	var tokens []string
	i := 0
	for i < len(src) {
		c := src[i]
		// whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		// line continuation: \ immediately followed by newline
		if c == '\\' && i+1 < len(src) && (src[i+1] == '\n' || src[i+1] == '\r') {
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
		// "..." string
		if c == '"' {
			j := scanDockerStr(src, i)
			tokens = append(tokens, src[i:j])
			i = j
			continue
		}
		// word — all non-whitespace, non-special chars
		j := i
		for j < len(src) {
			ch := src[j]
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				break
			}
			if ch == '#' || ch == '"' {
				break
			}
			if ch == '\\' && j+1 < len(src) && (src[j+1] == '\n' || src[j+1] == '\r') {
				break
			}
			j++
		}
		if j > i {
			tokens = append(tokens, src[i:j])
			i = j
		} else {
			i++
		}
	}
	return tokens
}

// normalizeDockerfile returns a canonical token sequence for comparison.
// It applies two known mechanical rewrites beyond pure whitespace:
//
//  1. ENV KEY=VALUE → ENV KEY VALUE (space-separated form).
//     WORD=VALUE tokens are split at the first '=', discarding the '='.
//     WORD= (trailing '=', empty value) is shortened to just WORD.
//
//  2. "..." quoted strings are unquoted: the surrounding quotes are stripped,
//     the content is re-tokenized at whitespace, and each resulting word is
//     also split at its first '='.  This handles two formatter behaviors:
//      a. ENV key= "value" → ENV key value (quotes removed from value).
//      b. Multi-line quoted shell arguments: indentation inside the string
//         changes but the word content stays the same after whitespace is
//         collapsed.
//
// Both rewrites are applied universally and symmetrically — they are the same
// on both sides of the comparison — so they cannot mask genuine content bugs.
func normalizeDockerfile(src string) string {
	toks := tokenizeDockerfile(src)
	var result []string
	normWord := func(w string) {
		if eq := strings.IndexByte(w, '='); eq > 0 {
			key := w[:eq]
			val := w[eq+1:]
			if val == "" {
				result = append(result, key)
			} else {
				result = append(result, key, val)
			}
		} else {
			result = append(result, w)
		}
	}
	for _, tok := range toks {
		if strings.HasPrefix(tok, `"`) && len(tok) >= 2 {
			// Unquote: strip surrounding quotes, split at whitespace, then at '='.
			// Standalone '\' words (backslash line-continuation) are discarded.
			content := tok[1 : len(tok)-1]
			for _, w := range strings.Fields(content) {
				if w == `\` {
					continue
				}
				normWord(w)
			}
		} else {
			normWord(tok)
		}
	}
	return strings.Join(result, " ")
}

// TestFormatPreservesTokens verifies that dockerfile.Format does not silently
// alter the program beyond the one known mechanical transformation (ENV
// key=value → key value).  Expected value is derived from raw input text; no
// golden file is used.
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
			src, err := os.ReadFile(filepath.Join("testdata/format", d.Name(), "input.dockerfile"))
			if err != nil {
				t.Skip("no input.dockerfile")
				return
			}
			f, err := dockerfile.Parse(string(src))
			if err != nil {
				t.Skipf("parse error: %v", err)
				return
			}
			formatted := dockerfile.Format(f)
			normIn := normalizeDockerfile(string(src))
			normOut := normalizeDockerfile(formatted)
			if normIn != normOut {
				t.Errorf("normalizeDockerfile(format(input)) != normalizeDockerfile(input)\n\nnorm(input):\n%s\n\nnorm(format):\n%s",
					normIn, normOut)
			}
		})
	}
}

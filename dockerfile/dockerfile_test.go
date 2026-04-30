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

// ── format ────────────────────────────────────────────────────────────────────

func TestFormat(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.dockerfile", "output.dockerfile", func(input string) (string, error) {
		f, err := dockerfile.Parse(input)
		if err != nil {
			return "", err
		}
		return dockerfile.Format(f), nil
	})
}

func TestFormatIdempotent(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.dockerfile", "output.dockerfile", func(input string) (string, error) {
		f, err := dockerfile.Parse(input)
		if err != nil {
			return "", err
		}
		first := dockerfile.Format(f)
		g, err := dockerfile.Parse(first)
		if err != nil {
			return "", fmt.Errorf("re-parse after format: %w", err)
		}
		return dockerfile.Format(g), nil
	})
}

// ── tidy ──────────────────────────────────────────────────────────────────────

func TestTidy(t *testing.T) {
	testutil.Golden(t, "testdata/tidy", "input.dockerfile", "output.dockerfile", func(input string) (string, error) {
		f, err := dockerfile.Parse(input)
		if err != nil {
			return "", err
		}
		dockerfile.TidyFile(f, tidyRUNStub, normaliseSetFlagsStub)
		return dockerfile.Format(f), nil
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

// TestMarshalAST pins the full JSON AST output for every format fixture.
func TestMarshalAST(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.dockerfile", "ast.json", func(src string) (string, error) {
		f, err := dockerfile.Parse(src)
		if err != nil {
			return "", err
		}
		b, err := json.MarshalIndent(dockerfile.MarshalFile(f, "input.dockerfile"), "", "\t")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	})
}

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
// These are tested via golden files in testdata/lint/.
// violations.txt is one "line: message" per line; empty means clean.
func TestLint(t *testing.T) {
	type lintFn func(src string) []string
	run := func(src string) []string {
		f, err := dockerfile.Parse(src)
		if err != nil {
			return nil
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
		return msgs
	}

	goldenLint(t, "testdata/lint", ".dockerfile", func(input string) string {
		msgs := run(input)
		return strings.Join(msgs, "\n") + "\n"
	})
}

// ── golden helpers ────────────────────────────────────────────────────────────


func goldenLint(t *testing.T, dir, inExt string, fn func(string) string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			t.Helper()
			inPath := filepath.Join(dir, e.Name(), "input"+inExt)
			outPath := filepath.Join(dir, e.Name(), "violations.txt")
			in, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			got := fn(string(in))
			if got == "\n" { // normalise "empty" to truly empty
				got = ""
			}
			if *testutil.Update {
				if err := os.WriteFile(outPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("golden %s missing — run `go test -update` to create it: %v", outPath, err)
			}
			if got != string(want) {
				t.Errorf("lint mismatch for %s\n--- got ---\n%q--- want ---\n%q", e.Name(), got, string(want))
			}
		})
	}
}

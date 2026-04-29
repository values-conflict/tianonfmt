// tianonfmt formats jq, Dockerfile, Dockerfile templates, and shell scripts
// according to Tianon's personal style conventions.
//
// Usage:
//
//	tianonfmt [flags] [file ...]
//
// With no file arguments, reads from stdin and writes to stdout.
// With file arguments and no flags, prints formatted output to stdout.
//
// Flags:
//
//	-w, --write     Write result back to each source file; print filenames of changed files.
//	                Mutually exclusive with --diff.  Errors if used with stdin.
//	-d, --diff      Print a unified diff for each file that would change; exit non-zero if
//	                any file differs.  Mutually exclusive with --write.
//	-t, --tidy      Apply idiomatic rewrites beyond formatting:
//	                  shell: #!/bin/bash → #!/usr/bin/env bash; || true → || :
//	                  Dockerfile RUN: && chains → set -eux; semicolons;
//	                                 set -Eeuo pipefail → set -eux
//	-p, --pedantic  Implies --tidy, plus stricter normalisations not applied by --tidy alone:
//	                  shell: set -e → set -eu (sh) or set -Eeuo pipefail (bash)
//	                Additionally fails (exit 1) if any constructs remain Wrong.
//	                Combine with --diff to see what needs changing.
//	    --ast[=mode]  Dump parsed AST as JSON to stdout.
//	                  --ast or --ast=input: pre-format AST
//	                  --ast=format: post-format AST
//	                  Combined with --diff: show diff between the two AST representations.
//
// File type detection (by path):
//   - .jq extension                    → jq formatter
//   - Dockerfile, Dockerfile.*         → Dockerfile formatter
//   - Dockerfile.template (with {{ }}) → jq-template formatter
//   - .sh extension or bash/sh shebang → shell formatter
//   - stdin / unknown: shebang or first keyword detection
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tianon/fmt/tianonfmt/dockerfile"
	"github.com/tianon/fmt/tianonfmt/internal/flags"
	"github.com/tianon/fmt/tianonfmt/jq"
	"github.com/tianon/fmt/tianonfmt/markdown"
	"github.com/tianon/fmt/tianonfmt/shell"
	"github.com/tianon/fmt/tianonfmt/template"
	"mvdan.cc/sh/v3/syntax"
)

// main is the thin shell around run; keeping it separate makes the tool testable.
func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the testable entry point for the formatter.  It returns the exit code.
// stdin/stdout/stderr replace the corresponding os.* variables so tests can
// capture and inject I/O without spawning a subprocess.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) (exitCode int) {
	// die is fatalf for use inside run — it panics with a sentinel so the
	// deferred recover can return 1 instead of calling os.Exit.
	type dieSignal struct{ msg string }
	defer func() {
		if r := recover(); r != nil {
			if ds, ok := r.(dieSignal); ok {
				fmt.Fprintln(stderr, ds.msg)
				exitCode = 1
			} else {
				panic(r) // re-panic for genuine panics
			}
		}
	}()
	die := func(format string, a ...interface{}) {
		panic(dieSignal{"tianonfmt: " + fmt.Sprintf(format, a...)})
	}

	fs := flags.New("tianonfmt")
	writeFlag := fs.Bool("write", 'w', "write result to source file (print filenames of changed files)")
	diffFlag := fs.Bool("diff", 'd', "display diffs; exit non-zero if any file differs")
	astFlag := fs.OptString("ast", 0, "input", "dump parsed AST as JSON to stdout; --ast=format dumps post-format AST; combine with --diff to show AST diff")
	tidyFlag := fs.Bool("tidy", 't', "apply idiomatic rewrites: Dockerfile RUN && chains → set -eux; semicolons, shell || true → || :")
	pedanticFlag := fs.Bool("pedantic", 'p', "fail if any constructs remain that Tianon considers Wrong; lists offending files (use with --diff to show what needs changing)")

	fileArgs, err := fs.Parse(args)
	if err != nil {
		die("%v", err)
	}

	if *writeFlag && *diffFlag {
		die("--write and --diff are mutually exclusive")
	}
	if *writeFlag && *astFlag != "" {
		die("--write and --ast are mutually exclusive")
	}

	fmtr := &formatter{
		tidy:     *tidyFlag || *pedanticFlag,
		pedantic: *pedanticFlag,
	}

	if len(fileArgs) == 0 {
		if *writeFlag {
			die("-w cannot be used with stdin")
		}
		src, err := io.ReadAll(stdin)
		if err != nil {
			die("reading stdin: %v", err)
		}

		if *astFlag != "" {
			pre, post, err := astByContent("-", string(src))
			if err != nil {
				die("%v", err)
			}
			return printAST("<stdin>", pre, post, *astFlag, *diffFlag, stdout, stderr)
		}

		out, err := fmtr.byContent("-", string(src))
		if err != nil {
			die("%v", err)
		}
		if *pedanticFlag {
			if code := pedanticCheck("-", out, false, *diffFlag, stdout, stderr); code != 0 {
				return code
			}
		}
		if *diffFlag {
			diff, err := computeDiff("<stdin>", string(src), out)
			if err != nil {
				die("diff: %v", err)
			}
			if len(diff) > 0 {
				stdout.Write(diff)
				return 1
			}
			return 0
		}
		fmt.Fprint(stdout, out)
		return 0
	}

	for _, path := range fileArgs {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "tianonfmt: %s: %v\n", path, err)
			exitCode = 1
			continue
		}

		if *astFlag != "" {
			pre, post, err := astByPath(path, string(src))
			if err != nil {
				fmt.Fprintf(stderr, "tianonfmt: %s: %v\n", path, err)
				exitCode = 1
				continue
			}
			if code := printAST(path, pre, post, *astFlag, *diffFlag, stdout, stderr); code != 0 {
				exitCode = code
			}
			continue
		}

		out, err := fmtr.byPath(path, string(src))
		if err != nil {
			fmt.Fprintf(stderr, "tianonfmt: %s: %v\n", path, err)
			exitCode = 1
			continue
		}

		if *pedanticFlag {
			if code := pedanticCheck(path, out, true, *diffFlag, stdout, stderr); code != 0 {
				exitCode = code
				continue
			}
		}

		if *diffFlag {
			diff, err := computeDiff(path, string(src), out)
			if err != nil {
				fmt.Fprintf(stderr, "tianonfmt: %s: diff: %v\n", path, err)
				exitCode = 1
				continue
			}
			if len(diff) > 0 {
				stdout.Write(diff)
				exitCode = 1
			}
			continue
		}

		if *writeFlag {
			if out != string(src) {
				if err := os.WriteFile(path, []byte(out), 0o666); err != nil {
					fmt.Fprintf(stderr, "tianonfmt: %s: %v\n", path, err)
					exitCode = 1
					continue
				}
				fmt.Fprintln(stdout, path)
			}
			continue
		}

		fmt.Fprint(stdout, out)
	}
	return exitCode
}

// ── formatter ─────────────────────────────────────────────────────────────────

// formatter holds formatting options and dispatches to per-language formatters.
type formatter struct {
	tidy     bool
	pedantic bool // pedantic implies tidy and adds stricter normalisations
}

func (f *formatter) byPath(path, src string) (string, error) {
	return f.byKind(kindByPath(path, src), src)
}

func (f *formatter) byContent(name, src string) (string, error) {
	return f.byKind(kindByContent(src), src)
}

func (f *formatter) byKind(k fileKind, src string) (string, error) {
	switch k {
	case kindJQ:
		return f.jq(src)
	case kindMarkdown:
		return f.md(src)
	case kindTemplate:
		return f.template(src)
	case kindDockerfile:
		return f.dockerfile(src)
	default: // kindShell
		return f.shell(src)
	}
}

func (f *formatter) jq(src string) (string, error) {
	// TODO: apply jq-specific tidy rewrites (e.g. == false → | not) when
	// f.tidy is true, once jq AST transformation infrastructure exists.
	parsed, err := jq.ParseFile(src)
	if err != nil {
		return "", fmt.Errorf("jq parse: %w", err)
	}
	return jq.FormatFile(parsed), nil
}

func (f *formatter) dockerfile(src string) (string, error) {
	parsed, err := dockerfile.Parse(src)
	if err != nil {
		return "", fmt.Errorf("dockerfile parse: %w", err)
	}
	if f.tidy {
		dockerfile.TidyFile(parsed, tidyRUN, normalizeSetFlags)
		dockerfile.TidyCmdEntrypoint(parsed)
	}
	if f.pedantic {
		dockerfile.PedanticCmdEntrypoint(parsed)
	}
	dfFmt := &dockerfile.Formatter{
		JQFmt:       jqFmtFunc,
		RUNShellFmt: shell.FormatRUN,
	}
	return dockerfile.FormatWith(parsed, dfFmt), nil
}

func (f *formatter) md(src string) (string, error) {
	return markdown.Format(src), nil
}

func (f *formatter) template(src string) (string, error) {
	return template.Format(src, jqFmtFunc), nil
}

func (f *formatter) shell(src string) (string, error) {
	lang := shell.DetectLang(src)
	var out string
	var err error
	switch {
	case f.pedantic:
		out, err = shell.FormatWithPedantic(src, lang, jqFmtFunc)
	case f.tidy:
		out, err = shell.FormatWithTidy(src, lang, jqFmtFunc)
	default:
		out, err = shell.Format(src, lang, jqFmtFunc)
	}
	if err != nil {
		return "", fmt.Errorf("shell format: %w", err)
	}
	return out, nil
}

// ── embedded jq callback ──────────────────────────────────────────────────────

// jqFmtFunc reformats a jq expression for embedding in another language.
// If inline is true, a single-line compact format is returned.
// Returns "" on any parse failure so that callers preserve the original.
func jqFmtFunc(expr string, inline bool) string {
	node, err := jq.ParseExpr(strings.TrimSpace(expr))
	if err != nil {
		f, ferr := jq.ParseFile(strings.TrimSpace(expr))
		if ferr != nil {
			return "" // parse failure — caller preserves original
		}
		return jq.FormatFile(f)
	}
	if inline {
		return jq.FormatNodeInline(node)
	}
	return jq.FormatNode(node)
}

// ── file-type detection ───────────────────────────────────────────────────────

type fileKind int

const (
	kindJQ fileKind = iota
	kindMarkdown
	kindDockerfile
	kindTemplate // Dockerfile.template with {{ }} jq syntax
	kindShell
)

// kindByPath detects the file type from the path extension/name.
func kindByPath(path, src string) fileKind {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	switch {
	case ext == ".jq":
		return kindJQ
	case ext == ".md":
		return kindMarkdown
	case ext == ".sh":
		return kindShell
	case isDockerfileName(base):
		if template.IsTemplate(src) {
			return kindTemplate
		}
		return kindDockerfile
	default:
		return kindByContent(src)
	}
}

// kindByContent detects the file type from content when the path is unknown.
func kindByContent(src string) fileKind {
	first, _, _ := strings.Cut(src, "\n")
	first = strings.TrimSpace(first)
	switch {
	case strings.HasPrefix(first, "#!/"):
		return kindShell
	case template.IsTemplate(src):
		return kindTemplate
	case isDockerfileContent(src):
		return kindDockerfile
	default:
		return kindJQ
	}
}

func isDockerfileName(base string) bool {
	return base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.")
}

var dockerfileKeywords = map[string]bool{
	"FROM": true, "RUN": true, "COPY": true, "ADD": true, "ENV": true,
	"ARG": true, "WORKDIR": true, "EXPOSE": true, "CMD": true,
	"ENTRYPOINT": true, "LABEL": true, "USER": true, "VOLUME": true,
	"STOPSIGNAL": true, "HEALTHCHECK": true, "SHELL": true, "ONBUILD": true,
}

func isDockerfileContent(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		kw, _, _ := strings.Cut(trimmed, " ")
		return dockerfileKeywords[strings.ToUpper(kw)]
	}
	return false
}

// ── tidy callback ─────────────────────────────────────────────────────────────

// tidyRUN parses args as shell and returns the list of commands to emit in
// set -eux; form, or nil if no transformation applies.
// It is the tidyRUN callback for dockerfile.TidyFile.
func tidyRUN(args string) []string {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	f, err := parser.Parse(strings.NewReader(strings.TrimSpace(args)), "")
	if err != nil || len(f.Stmts) != 1 {
		return nil
	}
	shell.ApplyTidy(f)
	stmts := shell.FlattenAndChain(f.Stmts[0])
	if len(stmts) < 2 {
		return nil
	}
	cmdStrs := make([]string, 0, len(stmts))
	for _, stmt := range stmts {
		s, err := shell.FormatStmtOneLine(stmt)
		if err != nil || s == "" {
			return nil
		}
		cmdStrs = append(cmdStrs, s)
	}
	if !strings.HasPrefix(strings.TrimSpace(cmdStrs[0]), "set -") {
		cmdStrs = append([]string{"set -eux"}, cmdStrs...)
	} else {
		// Normalise any "set" variant to "set -eux": bash-only flags like
		// -E and -o pipefail are Wrong in Dockerfile RUN (which uses /bin/sh).
		cmdStrs[0] = normalizeSetFlags(cmdStrs[0])
	}
	return cmdStrs
}

// normalizeSetFlags rewrites a "set ..." command to "set -eux", preserving
// only the flags Tianon uses in Dockerfile RUN blocks (which run under /bin/sh,
// not bash, so -E and -o pipefail are unavailable and wrong).
func normalizeSetFlags(s string) string {
	if strings.TrimSpace(s) == "set -eux" {
		return s // already correct
	}
	if strings.HasPrefix(strings.TrimSpace(s), "set -") {
		return "set -eux"
	}
	return s
}

// ── pedantic check ───────────────────────────────────────────────────────────

// pedanticCheck checks whether out (already tidy-applied) has any Wrong
// constructs that --tidy could not auto-fix.  byPath controls dispatch;
// showDiff controls whether second-tidy diffs are printed to stdout.
//
// Returns 0 (clean) or 1 (Wrong constructs remain).
func pedanticCheck(name, out string, byPath, showDiff bool, stdout, stderr io.Writer) int {
	code := 0

	tidyFmtr := &formatter{tidy: true}
	var further string
	var err error
	if byPath {
		further, err = tidyFmtr.byPath(name, out)
	} else {
		further, err = tidyFmtr.byContent(name, out)
	}
	if err == nil && further != out {
		code = 1
		if showDiff {
			diff, _ := computeDiff(name, out, further)
			stdout.Write(diff)
		} else {
			fmt.Fprintf(stderr, "tianonfmt: %s: Wrong constructs remain after --tidy\n", name)
		}
	}

	for _, v := range lintViolations(name, out, byPath) {
		fmt.Fprintf(stderr, "tianonfmt: %s:%d: %s\n", name, v.Line, v.Msg)
		code = 1
	}

	return code
}

// lintViolations returns pedantic lint violations for src, dispatching by
// file type using the same detection logic as the formatter.
func lintViolations(name, src string, byPathMode bool) []jq.Violation {
	var k fileKind
	if byPathMode {
		k = kindByPath(name, src)
	} else {
		k = kindByContent(src)
	}
	switch k {
	case kindMarkdown, kindTemplate:
		return lintMarkdown(src)
	case kindDockerfile:
		return lintDockerfile(src)
	case kindShell:
		return lintShell(src)
	default: // kindJQ
		return lintJQ(src)
	}
}

// lintMarkdown checks markdown src for pedantic violations.
func lintMarkdown(src string) []jq.Violation {
	vs := markdown.Lint(src)
	out := make([]jq.Violation, len(vs))
	for i, v := range vs {
		out[i] = jq.Violation{Line: v.Line, Msg: v.Msg}
	}
	return out
}

// lintJQ parses src as jq and returns any pedantic violations.
func lintJQ(src string) []jq.Violation {
	f, err := jq.ParseFile(src)
	if err != nil {
		return nil // parse error already caught by format step
	}
	return jq.LintFile(f, src)
}

// lintShell checks shell src for pedantic violations:
//   - echo -e / echo -n: use printf instead
//   - shebang: #!/bin/bash or #!/bin/sh should be #!/usr/bin/env bash
func lintShell(src string) []jq.Violation {
	var out []jq.Violation
	// Shebang check (line 1 only — TidyShebang would fix this).
	firstLine, _, _ := strings.Cut(src, "\n")
	switch strings.TrimSpace(firstLine) {
	case "#!/bin/bash", "#!/bin/sh":
		out = append(out, jq.Violation{Line: 1, Msg: fmt.Sprintf("%q: use \"#!/usr/bin/env bash\" instead", strings.TrimSpace(firstLine))})
	}
	// Line-by-line checks.
	setEuxFound := false
	for i, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if echoFlag := echoFlagViolation(trimmed); echoFlag != "" {
			out = append(out, jq.Violation{Line: i + 1, Msg: echoFlag})
		}
		if strings.HasPrefix(trimmed, "which ") || trimmed == "which" {
			out = append(out, jq.Violation{Line: i + 1, Msg: `"which": use "command -v" instead (which is non-standard; use --tidy to auto-fix flag-free calls)`})
		}
		// set -x at the global level is Wrong; only use locally around specific blocks.
		// Check after normalization: any global set with -x in the flags is Wrong.
		if strings.HasPrefix(trimmed, "set -") && strings.Contains(trimmed, "x") &&
			!strings.HasPrefix(line, "\t") {
			// Flag if this looks like a top-level set (no leading tab = depth 0).
			// Exclude "set --" and "set -o" forms.
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && strings.HasPrefix(parts[1], "-") && parts[1] != "--" {
				out = append(out, jq.Violation{
					Line: i + 1,
					Msg:  `"set -x" at the global level is Wrong; use "set +x" / "set -x" pairs around specific blocks only`,
				})
			}
		}
		if strings.HasPrefix(trimmed, "let ") || trimmed == "let" {
			out = append(out, jq.Violation{Line: i + 1, Msg: `"let" is Wrong: use $((...)) or var=$((expr)) instead`})
		}
		if strings.Contains(trimmed, "declare -i") {
			out = append(out, jq.Violation{Line: i + 1, Msg: `"declare -i" is Wrong: use untyped variables with arithmetic instead`})
		}
		// <<- (tab-stripping heredoc) is always preferred over << (bash.md §Heredocs).
		// Must not match <<< (here-strings, which are fine) or <<- (already correct).
		if idx := strings.Index(trimmed, "<<"); idx >= 0 {
			after := ""
			if idx+2 < len(trimmed) {
				after = trimmed[idx+2:]
			}
			isBareHeredoc := after == "" || (after[0] != '-' && after[0] != '<')
			if isBareHeredoc && !strings.HasPrefix(trimmed, "#") {
				out = append(out, jq.Violation{Line: i + 1, Msg: `"<<" heredoc: use "<<-" (tab-stripping) instead (bash.md §Heredocs)`})
			}
		}
		// Standalone scripts should use set -Eeuo pipefail (bash.md §Setup).
		// Simpler forms (set -eu, set -eux, set -ex) appear in Tianon's corpus
		// for entrypoints and utility scripts and are not flagged.
		// Only flag "set -e" alone — the absolute minimum with no -u or pipefail.
		if strings.HasPrefix(trimmed, "set -") {
			if strings.Contains(trimmed, "E") && strings.Contains(trimmed, "pipefail") {
				setEuxFound = true // canonical form
			} else if trimmed == "set -e" && !setEuxFound {
				out = append(out, jq.Violation{
					Line: i + 1,
					Msg:  `"set -e": standalone scripts should use at least "set -eu" or ideally "set -Eeuo pipefail" (bash.md §Setup)`,
				})
			}
		}
	}
	return out
}

// echoFlagViolation returns a violation message if the line uses echo -e or
// echo -n; returns "" if no violation.
func echoFlagViolation(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	// Allow "echo" to appear anywhere in a pipeline; just check the token itself.
	for i, f := range fields {
		if f != "echo" {
			continue
		}
		if i+1 >= len(fields) {
			break
		}
		next := fields[i+1]
		switch {
		case next == "-e" || strings.HasPrefix(next, "-e"):
			return `"echo -e": use printf for escape sequences`
		case next == "-n" || next == "-ne" || next == "-en":
			return `"echo -n": use printf for literal output without newline`
		}
	}
	return ""
}

// lintDockerfile checks Dockerfile src for pedantic violations.
func lintDockerfile(src string) []jq.Violation {
	f, err := dockerfile.Parse(src)
	if err != nil {
		return nil
	}
	var out []jq.Violation
	for _, instr := range f.Instructions {
		switch instr.Keyword {
		case "FROM":
			// FROM image:latest is Wrong — pinned versions should be used.
			// Exception: FROM scratch (always fine) and multi-stage refs (stage names, not tags).
			ref, _, _ := strings.Cut(strings.TrimSpace(instr.Args), " ") // strip AS alias
			if _, afterColon, hasTag := strings.Cut(ref, ":"); hasTag && afterColon == "latest" {
				out = append(out, jq.Violation{
					Line: instr.StartLine,
					Msg:  fmt.Sprintf(`FROM %s: using ":latest" tag is Wrong; pin to a specific version`, ref),
				})
			}

		case "MAINTAINER":
			out = append(out, jq.Violation{
				Line: instr.StartLine,
				Msg:  `MAINTAINER is deprecated and Wrong: remove it (there is no replacement in Tianon's style)`,
			})

		case "HEALTHCHECK":
			// HEALTHCHECK NONE (disabling an inherited check) is acceptable.
			// HEALTHCHECK CMD ... is Wrong: Tianon never adds health checks.
			if !strings.HasPrefix(strings.TrimSpace(instr.Args), "NONE") {
				out = append(out, jq.Violation{
					Line: instr.StartLine,
					Msg:  `HEALTHCHECK CMD is Wrong: use HEALTHCHECK NONE to disable inherited checks; never add health checks in Tianon's Dockerfiles`,
				})
			}

		case "ONBUILD":
			out = append(out, jq.Violation{
				Line: instr.StartLine,
				Msg:  `ONBUILD is Wrong: never used in Tianon's Dockerfiles`,
			})

		case "LABEL":
			// Exactly two LABEL keys appear in Tianon's corpus, both in
			// dockerfiles/buildkit/ where BuildKit itself requires them:
			//   moby.buildkit.frontend.caps
			//   moby.buildkit.frontend.network.none
			// All other LABEL usage is Wrong.
			key, _, _ := strings.Cut(strings.TrimSpace(instr.Args), "=")
			key = strings.TrimSpace(key)
			if key != "moby.buildkit.frontend.caps" && key != "moby.buildkit.frontend.network.none" {
				out = append(out, jq.Violation{
					Line: instr.StartLine,
					Msg:  `LABEL is Wrong: not used in Tianon's Dockerfiles (exception: moby.buildkit.frontend.caps and moby.buildkit.frontend.network.none in BuildKit images)`,
				})
			}

		case "RUN":
			// Flag set commands using bash-only flags (-E, -o pipefail) in /bin/sh RUN.
			// Simpler forms like set -ex and set -eux are acceptable (docs §RUN shell style).
			firstCmd, _, _ := strings.Cut(instr.Args, ";")
			firstCmd = strings.TrimSpace(firstCmd)
			if strings.HasPrefix(firstCmd, "set -") &&
				(strings.Contains(firstCmd, "E") || strings.Contains(firstCmd, "pipefail")) {
				out = append(out, jq.Violation{
					Line: instr.StartLine,
					Msg:  fmt.Sprintf("%q in Dockerfile RUN uses bash-only flags; use \"set -eux\" (RUN runs under /bin/sh)", firstCmd),
				})
			}
			// Flag apt-get install without -y or --no-install-recommends.
			if v := aptGetInstallViolation(instr.Args, instr.StartLine); v != nil {
				out = append(out, *v)
			}
		}
	}
	return out
}

// aptGetInstallViolation returns a violation if the RUN args contain an
// apt-get install call missing -y or --no-install-recommends.
// Skips local file installs (e.g. ./pkg.deb, /path/to/pkg.deb) which may
// intentionally omit --no-install-recommends.
func aptGetInstallViolation(args string, line int) *jq.Violation {
	idx := strings.Index(args, "apt-get install")
	if idx < 0 {
		return nil
	}
	segment := args[idx:]
	// Skip local file installs: the first non-flag package-like token starts with . or /
	// e.g. "apt-get install -y ./pkg.deb" — recommends may be intentional.
	afterInstall := strings.TrimPrefix(segment, "apt-get install")
	for _, field := range strings.Fields(afterInstall) {
		if strings.HasPrefix(field, "-") {
			continue
		}
		// First non-flag argument: if it looks like a path, skip the check.
		if strings.HasPrefix(field, "./") || strings.HasPrefix(field, "/") || strings.HasSuffix(field, ".deb") {
			return nil
		}
		break
	}
	if !strings.Contains(segment, " -y") && !strings.Contains(segment, "\t-y") {
		return &jq.Violation{Line: line, Msg: `apt-get install missing "-y" flag`}
	}
	if !strings.Contains(segment, "--no-install-recommends") {
		return &jq.Violation{Line: line, Msg: `apt-get install missing "--no-install-recommends" flag`}
	}
	return nil
}

// ── AST dump ─────────────────────────────────────────────────────────────────

// astByPath computes the pre- and post-format AST JSON for a named file.
func astByPath(path, src string) (pre, post string, err error) {
	return astByKind(path, src, kindByPath(path, src))
}

// astByContent is like astByPath but uses content-based detection.
// name should be "-" for stdin.
func astByContent(name, src string) (pre, post string, err error) {
	return astByKind(name, src, kindByContent(src))
}

func astByKind(name, src string, k fileKind) (pre, post string, err error) {
	switch k {
	case kindJQ, kindTemplate:
		return jqASTPair(name, src)
	case kindMarkdown:
		return markdownASTPair(name, src)
	case kindDockerfile:
		return dockerfileASTPair(name, src)
	default: // kindShell
		return shellASTPair(name, src)
	}
}

// markdownASTPair returns the pre- and post-format AST for a markdown file.
// Markdown formatting is a sequence of normalizations (no re-parse like jq/shell).
func markdownASTPair(name, src string) (pre, post string, err error) {
	pre, err = marshalASTJSON(markdown.MarshalFile(src, name))
	if err != nil {
		return "", "", err
	}
	formatted := markdown.Format(src)
	post, err = marshalASTJSON(markdown.MarshalFile(formatted, name))
	return pre, post, err
}

// jqASTPair parses src as jq and returns both the pre-format and post-format
// AST as JSON strings (tab-indented, with trailing newline).
// name is embedded as "file" in each AST object; use "-" for stdin.
func jqASTPair(name, src string) (pre, post string, err error) {
	f, err := jq.ParseFile(src)
	if err != nil {
		return "", "", fmt.Errorf("jq parse: %w", err)
	}
	pre, err = marshalASTJSON(f.MarshalAST().Insert(1, "file", name))
	if err != nil {
		return "", "", err
	}
	formatted := jq.FormatFile(f)
	g, err := jq.ParseFile(formatted)
	if err != nil {
		return "", "", fmt.Errorf("jq re-parse after format: %w", err)
	}
	post, err = marshalASTJSON(g.MarshalAST().Insert(1, "file", name))
	if err != nil {
		return "", "", err
	}
	return pre, post, nil
}

// shellASTPair parses src as a shell script and returns both the pre- and
// post-format AST as JSON strings.
func shellASTPair(name, src string) (pre, post string, err error) {
	lang := shell.DetectLang(src)
	f, err := shell.ParseFile(src, lang)
	if err != nil {
		return "", "", fmt.Errorf("shell parse: %w", err)
	}
	pre, err = marshalASTJSON(shell.MarshalFile(f, name))
	if err != nil {
		return "", "", err
	}
	formatted, err := shell.Format(src, lang, jqFmtFunc)
	if err != nil {
		return "", "", fmt.Errorf("shell format: %w", err)
	}
	g, err := shell.ParseFile(formatted, lang)
	if err != nil {
		return "", "", fmt.Errorf("shell re-parse after format: %w", err)
	}
	post, err = marshalASTJSON(shell.MarshalFile(g, name))
	return pre, post, err
}

// dockerfileASTPair parses src as a Dockerfile and returns both the pre- and
// post-format AST as JSON strings.  name is embedded as "file"; use "-" for stdin.
func dockerfileASTPair(name, src string) (pre, post string, err error) {
	f, err := dockerfile.Parse(src)
	if err != nil {
		return "", "", fmt.Errorf("dockerfile parse: %w", err)
	}
	pre, err = marshalASTJSON(dockerfile.MarshalFile(f, name))
	if err != nil {
		return "", "", err
	}
	fmtr := &dockerfile.Formatter{JQFmt: jqFmtFunc, RUNShellFmt: shell.FormatRUN}
	formatted := dockerfile.FormatWith(f, fmtr)
	g, err := dockerfile.Parse(formatted)
	if err != nil {
		return "", "", fmt.Errorf("dockerfile re-parse after format: %w", err)
	}
	post, err = marshalASTJSON(dockerfile.MarshalFile(g, name))
	return pre, post, err
}

// marshalASTJSON encodes v as tab-indented JSON with a trailing newline.
// HTML escaping is disabled so characters like & appear literally.
func marshalASTJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "\t")
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("marshal AST: %w", err)
	}
	return buf.String(), nil
}

// printAST selects and prints the right AST output based on mode and diffMode.
// Returns an exit code: 0 for no difference, 1 if --diff found differences.
func printAST(name, pre, post, mode string, diffMode bool, stdout, stderr io.Writer) int {
	if diffMode {
		diff, err := computeDiff(name+".ast", pre, post)
		if err != nil {
			fmt.Fprintf(stderr, "tianonfmt: %s: ast diff: %v\n", name, err)
			return 1
		}
		if len(diff) > 0 {
			stdout.Write(diff)
			return 1
		}
		return 0
	}
	if mode == "format" {
		fmt.Fprint(stdout, post)
	} else {
		fmt.Fprint(stdout, pre)
	}
	return 0
}

// ── diff ─────────────────────────────────────────────────────────────────────

// computeDiff returns a unified diff of before vs after for the named file.
// Returns nil if they are identical.
func computeDiff(name, before, after string) ([]byte, error) {
	if before == after {
		return nil, nil
	}

	f1, err := os.CreateTemp("", "tianonfmt-*.orig")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f1.Name())

	f2, err := os.CreateTemp("", "tianonfmt-*.new")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f2.Name())

	if _, err := f1.WriteString(before); err != nil {
		return nil, err
	}
	f1.Close()

	if _, err := f2.WriteString(after); err != nil {
		return nil, err
	}
	f2.Close()

	cmd := exec.Command("diff",
		"--unified",
		"--label=a/"+name,
		"--label=b/"+name,
		f1.Name(), f2.Name())

	var buf bytes.Buffer
	cmd.Stdout = &buf
	// diff exits 1 when there are differences, 2 on error — only treat 2 as error.
	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				err = nil // differences found — not an error
			}
		}
	}
	return buf.Bytes(), err
}




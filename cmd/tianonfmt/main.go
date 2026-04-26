// tianonfmt formats jq, Dockerfile, Dockerfile templates, and shell scripts.
//
// Usage:
//
//	tianonfmt [-w | -d] [file ...]
//
// With no file arguments, reads from stdin and writes to stdout.
// With file arguments and no flags, prints formatted output to stdout.
//
// Flags:
//
//	-w   Write result back to each source file; print filenames of changed files.
//	     Mutually exclusive with -d.  Errors if used with stdin.
//	-d   Print a unified diff for each file that would change; exit non-zero if
//	     any file differs.  Mutually exclusive with -w.
//
// File type detection (by path):
//   - .jq extension                → jq formatter
//   - Dockerfile, Dockerfile.*     → Dockerfile formatter
//   - Dockerfile.template, etc. containing {{ }} → jq-template formatter
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
	"github.com/tianon/fmt/tianonfmt/shell"
	"github.com/tianon/fmt/tianonfmt/template"
	"mvdan.cc/sh/v3/syntax"
)

func main() {
	// TODO: add --tidy / -t: apply idiomatic rewrites (RUN && chains → set -eux; semicolons,
	// shell || true → || :, jq == false → | not, etc.).  See FUTURE.md for the full design.
	//
	// TODO: add --pedantic / -p: reject (exit 1, no file modification) constructs Tianon
	// considers Wrong — capital W intentional, see prose.md §Intentional mid-sentence
	// capitalisation.  Help text should read: "fail if any constructs remain that Tianon
	// considers Wrong".  Acts as a linter; composes with --diff to show what would need to
	// change.  --pedantic without --tidy checks current state; --pedantic with --tidy checks
	// after applying tidy rewrites.
	fs := flags.New("tianonfmt")
	writeFlag := fs.Bool("write", 'w', "write result to source file (print filenames of changed files)")
	diffFlag := fs.Bool("diff", 'd', "display diffs; exit non-zero if any file differs")
	// --ast or --ast=input → pre-format AST JSON; --ast=format → post-format AST JSON.
	// Combined with --diff, shows the diff between the two AST representations.
	// Value must use = syntax: --ast=format (per GNU optional-argument convention).
	astFlag := fs.OptString("ast", 0, "input", "dump parsed AST as JSON to stdout; --ast=format dumps post-format AST; combine with --diff to show AST diff")

	args, err := fs.Parse(os.Args[1:])
	if err != nil {
		fatalf("%v", err)
	}

	if *writeFlag && *diffFlag {
		fatalf("--write and --diff are mutually exclusive")
	}
	if *writeFlag && *astFlag != "" {
		fatalf("--write and --ast are mutually exclusive")
	}

	// args already populated by fs.Parse above

	if len(args) == 0 {
		if *writeFlag {
			fatalf("-w cannot be used with stdin")
		}
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("reading stdin: %v", err)
		}

		if *astFlag != "" {
			pre, post, err := astByContent("-", string(src))
			if err != nil {
				fatalf("%v", err)
			}
			os.Exit(printAST("<stdin>", pre, post, *astFlag, *diffFlag))
		}

		out, err := formatByContent("<stdin>", string(src))
		if err != nil {
			fatalf("%v", err)
		}
		if *diffFlag {
			diff, err := computeDiff("<stdin>", string(src), out)
			if err != nil {
				fatalf("diff: %v", err)
			}
			if len(diff) > 0 {
				os.Stdout.Write(diff)
				os.Exit(1)
			}
			return
		}
		fmt.Print(out)
		return
	}

	exitCode := 0
	for _, path := range args {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tianonfmt: %s: %v\n", path, err)
			exitCode = 1
			continue
		}

		if *astFlag != "" {
			pre, post, err := astByPath(path, string(src))
			if err != nil {
				fmt.Fprintf(os.Stderr, "tianonfmt: %s: %v\n", path, err)
				exitCode = 1
				continue
			}
			if code := printAST(path, pre, post, *astFlag, *diffFlag); code != 0 {
				exitCode = code
			}
			continue
		}

		out, err := formatByPath(path, string(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "tianonfmt: %s: %v\n", path, err)
			exitCode = 1
			continue
		}

		if *diffFlag {
			diff, err := computeDiff(path, string(src), out)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tianonfmt: %s: diff: %v\n", path, err)
				exitCode = 1
				continue
			}
			if len(diff) > 0 {
				os.Stdout.Write(diff)
				exitCode = 1
			}
			continue
		}

		if *writeFlag {
			if out != string(src) {
				if err := os.WriteFile(path, []byte(out), 0o666); err != nil {
					fmt.Fprintf(os.Stderr, "tianonfmt: %s: %v\n", path, err)
					exitCode = 1
					continue
				}
				fmt.Println(path)
			}
			continue
		}

		// Default: print to stdout.
		fmt.Print(out)
	}
	os.Exit(exitCode)
}

// ── type detection ────────────────────────────────────────────────────────────

func formatByPath(path, src string) (string, error) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)

	switch {
	case ext == ".jq":
		return formatJQ(src)

	case isDockerfileName(base):
		// Check for template syntax before choosing formatter.
		if template.IsTemplate(src) {
			return formatTemplate(src)
		}
		return formatDockerfile(src)

	case ext == ".sh":
		return formatShell(src)

	default:
		return formatByContent(path, src)
	}
}

func isDockerfileName(base string) bool {
	return base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.")
}

func formatByContent(name, src string) (string, error) {
	first, _, _ := strings.Cut(src, "\n")
	first = strings.TrimSpace(first)
	switch {
	case strings.HasPrefix(first, "#!/") && (strings.Contains(first, "bash") || strings.Contains(first, "/sh")):
		return formatShell(src)
	case isDockerfileContent(src):
		if template.IsTemplate(src) {
			return formatTemplate(src)
		}
		return formatDockerfile(src)
	default:
		return formatJQ(src)
	}
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

// ── per-language formatters ──────────────────────────────────────────────────

func formatJQ(src string) (string, error) {
	f, err := jq.ParseFile(src)
	if err != nil {
		return "", fmt.Errorf("jq parse: %w", err)
	}
	return jq.FormatFile(f), nil
}

// jqFmtFunc returns a formatter callback suitable for passing to embedded-
// language formatters.  If inline is true, a single-line compact format is
// returned; otherwise the standard multi-line format is used.
func jqFmtFunc(expr string, inline bool) string {
	node, err := jq.ParseExpr(strings.TrimSpace(expr))
	if err != nil {
		// Also try as a full file (for expressions containing top-level defs).
		f, ferr := jq.ParseFile(strings.TrimSpace(expr))
		if ferr != nil {
			return "" // signal parse failure — caller preserves original
		}
		return jq.FormatFile(f)
	}
	if inline {
		return jq.FormatNodeInline(node)
	}
	return jq.FormatNode(node)
}

func formatDockerfile(src string) (string, error) {
	f, err := dockerfile.Parse(src)
	if err != nil {
		return "", fmt.Errorf("dockerfile parse: %w", err)
	}
	fmtr := &dockerfile.Formatter{
		JQFmt:       jqFmtFunc,
		RUNShellFmt: shell.FormatRUN,
	}
	return dockerfile.FormatWith(f, fmtr), nil
}

func formatTemplate(src string) (string, error) {
	return template.Format(src, jqFmtFunc), nil
}

func formatShell(src string) (string, error) {
	lang := shell.DetectLang(src)
	out, err := shell.Format(src, lang, jqFmtFunc)
	if err != nil {
		return "", fmt.Errorf("shell format: %w", err)
	}
	return out, nil
}

// ── AST dump ─────────────────────────────────────────────────────────────────

// astByPath computes the pre- and post-format AST JSON for a named file.
func astByPath(path, src string) (pre, post string, err error) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	switch {
	case ext == ".jq":
		return jqASTPair(path, src)
	case isDockerfileName(base), ext == ".sh":
		return "", "", fmt.Errorf("--ast not yet supported for this file type")
	default:
		return jqASTPair(path, src)
	}
}

// astByContent is like astByPath but uses content-based detection.
// name should be "-" for stdin.
func astByContent(name, src string) (pre, post string, err error) {
	first, _, _ := strings.Cut(src, "\n")
	first = strings.TrimSpace(first)
	switch {
	case strings.HasPrefix(first, "#!/") && (strings.Contains(first, "bash") || strings.Contains(first, "/sh")):
		return "", "", fmt.Errorf("--ast not yet supported for shell files")
	case isDockerfileContent(src):
		return "", "", fmt.Errorf("--ast not yet supported for Dockerfile files")
	default:
		return jqASTPair(name, src)
	}
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

// marshalASTJSON encodes v as tab-indented JSON with a trailing newline.
func marshalASTJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return "", fmt.Errorf("marshal AST: %w", err)
	}
	return string(b) + "\n", nil
}

// printAST selects and prints the right AST output based on mode and diffMode.
// Returns an exit code: 0 for no difference, 1 if --diff found differences.
func printAST(name, pre, post, mode string, diffMode bool) int {
	if diffMode {
		diff, err := computeDiff(name+".ast", pre, post)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tianonfmt: %s: ast diff: %v\n", name, err)
			return 1
		}
		if len(diff) > 0 {
			os.Stdout.Write(diff)
			return 1
		}
		return 0
	}
	if mode == "format" {
		fmt.Print(post)
	} else {
		fmt.Print(pre)
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

// fatalf prints to stderr and exits 1.
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "tianonfmt: "+format+"\n", args...)
	os.Exit(1)
}

// suppress unused import warning; syntax is used via shell package
var _ = syntax.LangBash

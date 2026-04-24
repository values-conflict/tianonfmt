// tianonfmt formats jq, Dockerfile, and shell script files.
//
// Usage:
//
//	tianonfmt [file ...]
//
// With no arguments, reads from stdin and writes to stdout (type detected from
// content).  With file arguments, formats each file in place (original is
// overwritten only when the output differs).
//
// File type detection:
//   - .jq extension → jq formatter
//   - Dockerfile, Dockerfile.* → Dockerfile formatter
//   - .sh extension or bash/sh shebang on line 1 → shell formatter
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tianon/fmt/tianonfmt/dockerfile"
	"github.com/tianon/fmt/tianonfmt/jq"
	"github.com/tianon/fmt/tianonfmt/shell"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		// Stdin mode: read from stdin, detect type from content, write to stdout.
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("reading stdin: %v", err)
		}
		out, err := formatByContent(string(src))
		if err != nil {
			fatalf("%v", err)
		}
		fmt.Print(out)
		return
	}

	exitCode := 0
	for _, path := range args {
		if err := formatFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "tianonfmt: %s: %v\n", path, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func formatFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	out, err := formatByPath(path, string(src))
	if err != nil {
		return err
	}

	if out == string(src) {
		return nil
	}

	return os.WriteFile(path, []byte(out), 0o666)
}

// formatByPath detects file type from path and formats src.
func formatByPath(path, src string) (string, error) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)

	switch {
	case ext == ".jq":
		return formatJQ(src)

	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
		return formatDockerfile(src)

	case ext == ".sh":
		return formatShell(src)

	default:
		// Try shebang detection.
		return formatByContent(src)
	}
}

// formatByContent detects file type from content (shebang or first keyword) and formats.
func formatByContent(src string) (string, error) {
	first, _, _ := strings.Cut(src, "\n")
	first = strings.TrimSpace(first)
	switch {
	case strings.HasPrefix(first, "#!/") && (strings.Contains(first, "bash") || strings.Contains(first, "/sh")):
		return formatShell(src)
	case isDockerfileContent(src):
		return formatDockerfile(src)
	default:
		return formatJQ(src)
	}
}

// dockerfileKeywords is the set of first-word tokens that unambiguously
// identify a file as a Dockerfile.
var dockerfileKeywords = map[string]bool{
	"FROM": true, "RUN": true, "COPY": true, "ADD": true, "ENV": true,
	"ARG": true, "WORKDIR": true, "EXPOSE": true, "CMD": true,
	"ENTRYPOINT": true, "LABEL": true, "USER": true, "VOLUME": true,
	"STOPSIGNAL": true, "HEALTHCHECK": true, "SHELL": true, "ONBUILD": true,
}

// isDockerfileContent returns true if src looks like a Dockerfile by checking
// whether the first non-comment, non-blank line starts with a Dockerfile keyword.
func isDockerfileContent(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		kw, _, _ := strings.Cut(trimmed, " ")
		if !dockerfileKeywords[strings.ToUpper(kw)] {
			return false
		}
		return true
	}
	return false
}

func formatJQ(src string) (string, error) {
	f, err := jq.ParseFile(src)
	if err != nil {
		return "", fmt.Errorf("jq parse: %w", err)
	}
	return jq.FormatFile(f), nil
}

func formatDockerfile(src string) (string, error) {
	f, err := dockerfile.Parse(src)
	if err != nil {
		return "", fmt.Errorf("dockerfile parse: %w", err)
	}
	return dockerfile.Format(f), nil
}

func formatShell(src string) (string, error) {
	lang := shell.DetectLang(src)
	out, err := shell.Format(src, lang)
	if err != nil {
		return "", fmt.Errorf("shell format: %w", err)
	}
	return out, nil
}

// fatalf prints to stderr and exits 1.
func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "tianonfmt: "+format+"\n", args...)
	os.Exit(1)
}

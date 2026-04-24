// Package shell formats shell scripts using mvdan.cc/sh/v3.
//
// Style rules (backed by corpus):
//   - Hard tabs for indentation (corpus: all .sh files use tabs, e.g.
//     corpus/debuerreotype/examples/debian.sh, corpus/docker-bin/*.sh)
//   - Binary operators on the same line (not the start of the next)
//   - Redirects attached to their command
//   - Switch cases kept compact
package shell

import (
	"bytes"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Format formats a shell script source string.
// lang should be syntax.LangBash, syntax.LangPOSIX, etc.
func Format(src string, lang syntax.LangVariant) (string, error) {
	parser := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(lang))
	f, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return "", fmt.Errorf("shell parse: %w", err)
	}

	var buf bytes.Buffer
	printer := syntax.NewPrinter(
		// Indent(0) means tabs — matches corpus tab indentation
		// (corpus/debuerreotype/examples/debian.sh, corpus/docker-bin/*.sh)
		syntax.Indent(0),
		syntax.BinaryNextLine(false),
		syntax.SwitchCaseIndent(true),
		syntax.KeepPadding(false),
	)
	if err := printer.Print(&buf, f); err != nil {
		return "", fmt.Errorf("shell format: %w", err)
	}
	return buf.String(), nil
}

// DetectLang guesses the shell language variant from a shebang line.
// Returns syntax.LangBash as the default.
func DetectLang(src string) syntax.LangVariant {
	line, _, _ := strings.Cut(src, "\n")
	line = strings.TrimSpace(line)
	switch {
	case strings.Contains(line, "/sh") && !strings.Contains(line, "bash"):
		return syntax.LangPOSIX
	case strings.Contains(line, "mksh"):
		return syntax.LangMirBSDKorn
	default:
		return syntax.LangBash
	}
}

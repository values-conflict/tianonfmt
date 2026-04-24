// Package shell formats shell scripts using mvdan.cc/sh/v3.
//
// It also handles two embedded-language scenarios:
//
//  1. jq-in-shell: `jq '...'` invocations where the single-quoted argument is
//     a jq expression; the expression is reformatted in-place before printing.
//
//  2. RUN-line context: shell commands embedded in Dockerfile RUN instructions
//     use ` \` line continuation instead of newlines; the formatter normalises
//     tab indentation while respecting that convention.
package shell

import (
	"bytes"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Context controls how the formatter interprets its input.
type Context int

const (
	// ContextStandalone is a normal shell script file.
	ContextStandalone Context = iota
	// ContextDockerfileRUN is the shell content of a Dockerfile RUN instruction,
	// where commands are separated by "; \" and each non-blank continuation line
	// is indented with tabs relative to its shell nesting depth.
	ContextDockerfileRUN
)

// Format formats a shell script.  jq expressions inside `jq '...'` invocations
// are reformatted using the provided jqFmt function (if non-nil).
func Format(src string, lang syntax.LangVariant, jqFmt func(expr string, inline bool) string) (string, error) {
	parser := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(lang))
	f, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return "", fmt.Errorf("shell parse: %w", err)
	}

	if jqFmt != nil {
		formatJQInAST(f, jqFmt)
	}

	var buf bytes.Buffer
	printer := syntax.NewPrinter(
		syntax.Indent(0),             // 0 = tabs (corpus: all .sh files use tabs)
		syntax.BinaryNextLine(false), // binary ops stay on current line
		syntax.SwitchCaseIndent(true),
		syntax.KeepPadding(false),
	)
	if err := printer.Print(&buf, f); err != nil {
		return "", fmt.Errorf("shell format: %w", err)
	}
	return buf.String(), nil
}

// FormatRUN normalises the shell content of a Dockerfile RUN instruction.
//
// The input is a slice of physical continuation lines (everything AFTER the
// "RUN" keyword on the first line, through the last continuation line).  Each
// line may end with " \" (continuation) or end the instruction.
//
// Normalisation rules (backed by corpus):
//   - Commands and control-flow keywords at depth 0: 1 tab
//   - Commands inside if/then/case/for/while bodies: +1 tab per level
//   - Argument-list continuation lines (no "; \" suffix): same indent as
//     the command they belong to + 1 extra tab
//   - Standalone comment lines ("#..."): 0 tabs (column 0)
//     (corpus/tianon-dockerfiles/steam/Dockerfile.template:7)
//     (corpus/tianon-dockerfiles/tailscale/Dockerfile:23)
//   - Blank continuation lines (lone "\"): preserved as-is
//
// jqFmt, if non-nil, is called to reformat jq expressions found inside the
// shell content.
func FormatRUN(lines []string, jqFmt func(expr string, inline bool) string) []string {
	if len(lines) == 0 {
		return lines
	}

	type outLine struct {
		text    string
		hasCont bool // true if original line had continuation \
	}

	depth := 0
	var result []string

	for _, raw := range lines {
		// Strip the trailing " \" or "\" continuation marker.
		hasCont := strings.HasSuffix(raw, "\\")
		line := raw
		if hasCont {
			line = strings.TrimRight(raw[:len(raw)-1], " \t")
		}
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			// Blank visual separator: preserve as lone backslash.
			if hasCont {
				result = append(result, "\\")
			}
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			// Comment lines: always at column 0.
			result = append(result, appendCont(trimmed, hasCont))
			continue
		}

		// Determine depth adjustments for closing keywords.
		// These keywords go at depth-1 relative to their body.
		keyword := firstWord(trimmed)
		closingKeyword := keyword == "fi" || keyword == "done" || keyword == "esac"
		halfKeyword := keyword == "elif" || keyword == "else"

		if closingKeyword && depth > 0 {
			depth--
		}
		if halfKeyword && depth > 0 {
			depth-- // temporarily decrease for this line (body at same depth as opener)
		}

		// Optionally reformat jq arguments embedded in this line.
		if jqFmt != nil {
			trimmed = reformatJQInLine(trimmed, jqFmt)
		}

		result = append(result, appendCont(strings.Repeat("\t", depth)+trimmed, hasCont))

		// Depth adjustments for opening keywords.
		if closingKeyword {
			// stays at current (already decremented)
		} else if halfKeyword {
			// Re-increment: body of elif/else follows at depth.
			depth++
		} else if endsBlock(trimmed) {
			depth++
		}
	}

	return result
}

// appendCont appends " \" to line if hasCont is true.
func appendCont(line string, hasCont bool) string {
	if hasCont {
		return line + " \\"
	}
	return line
}

// firstWord returns the first whitespace-separated token of s.
func firstWord(s string) string {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s
	}
	return s[:i]
}

// endsBlock returns true if a continuation line opens a new indentation block.
// These are lines ending with the opening keywords/markers.
func endsBlock(line string) bool {
	// Must have continuation (" \") stripped already.
	// Check for common shell block-opening patterns:
	//   if ...; then
	//   elif ...; then
	//   else
	//   case ... in
	//   for ...; do
	//   while ...; do
	kw := firstWord(line)
	switch kw {
	case "else":
		return true
	}
	// Endings that open a block: "then", "do", "in" (for case) at end of line
	trimEnd := strings.TrimRight(line, " \t")
	for _, end := range []string{"; then", " then", "; do", " do", " in"} {
		if strings.HasSuffix(trimEnd, end) {
			return true
		}
	}
	return false
}

// ── jq-in-shell AST rewriting ────────────────────────────────────────────────

// formatJQInAST walks the shell AST and reformats jq expression arguments
// inside `jq '...'` invocations.
//
// Detection heuristic (matches Tianon's corpus patterns):
//   - The command is `jq` (literal word)
//   - The last non-flag, non-value-flag argument is a SglQuoted string
//   - Value flags (--arg, --argjson, etc.) consume their NAME and VALUE args
//
// Corpus refs:
//   - corpus/tianon-dockerfiles/buildkit/versions.sh:62 single-line
//   - corpus/tianon-dockerfiles/buildkit/versions.sh:67-70 multi-line
func formatJQInAST(f *syntax.File, jqFmt func(expr string, inline bool) string) {
	syntax.Walk(f, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		if len(ce.Args) == 0 {
			return true
		}
		if wordLit(ce.Args[0]) != "jq" {
			return true
		}
		sgl := findJQExprArg(ce.Args[1:])
		if sgl == nil {
			return true
		}
		reformatSglQuoted(sgl, jqFmt)
		return true
	})
}

// wordLit returns the literal string value of a word if it is a single literal,
// or "" otherwise.
func wordLit(w *syntax.Word) string {
	if len(w.Parts) != 1 {
		return ""
	}
	lit, ok := w.Parts[0].(*syntax.Lit)
	if !ok {
		return ""
	}
	return lit.Value
}

// valueFlags is the set of jq flags that consume additional arguments (NAME and/or VALUE).
// Each maps to the number of extra args it consumes.
var valueFlags = map[string]int{
	"--arg":        2, // NAME VALUE
	"--argjson":    2,
	"--slurpfile":  2,
	"--rawfile":    2,
	"--json-args":  0, // terminates flag processing; rest are jq positional args
	"--args":       0,
	"-f":           1, // FILENAME
	"--from-file":  1,
	"--indent":     1, // N
	"--tab":        0,
	"--run-tests":  0,
}

// findJQExprArg finds the SglQuoted argument that is the jq filter expression.
// It skips flags and their arguments, and returns the last remaining SglQuoted.
func findJQExprArg(args []*syntax.Word) *syntax.SglQuoted {
	skip := 0
	var last *syntax.SglQuoted

	for _, w := range args {
		if skip > 0 {
			skip--
			continue
		}

		lit := wordLit(w)
		if strings.HasPrefix(lit, "-") {
			if n, ok := valueFlags[lit]; ok {
				skip = n
			}
			// Also handle short combined flags like -rc, -rn, etc. — these
			// consume no additional args unless they include -f.
			continue
		}

		// Non-flag argument: check if it's a SglQuoted
		if len(w.Parts) == 1 {
			if sgl, ok := w.Parts[0].(*syntax.SglQuoted); ok {
				last = sgl
			}
		}
	}
	return last
}

// reformatSglQuoted reformats sgl.Value as a jq expression using jqFmt.
// Preserves the single-line vs multi-line nature of the original.
func reformatSglQuoted(sgl *syntax.SglQuoted, jqFmt func(expr string, inline bool) string) {
	val := sgl.Value
	if val == "" {
		return
	}

	isMultiLine := strings.Contains(val, "\n")

	if !isMultiLine {
		// Single-line: format compactly.
		formatted := jqFmt(strings.TrimSpace(val), true)
		if formatted != "" && !strings.Contains(formatted, "\n") {
			sgl.Value = formatted
		}
		return
	}

	// Multi-line: extract the content lines, strip their common indent,
	// format as jq, re-add the indent.
	//
	// The structure (from corpus/tianon-dockerfiles/buildkit/versions.sh:67-70):
	//   '
	//   \t\tEXPR_LINE_1
	//   \t\t| EXPR_LINE_2
	//   \t'
	//
	// The content is between the leading \n and the trailing \n+indent.
	// We determine the common indent from the first non-empty content line,
	// then strip it for formatting and re-add it after.

	// Trim leading/trailing newline wrappers.
	inner := strings.Trim(val, "\n")
	innerLines := strings.Split(inner, "\n")

	// Find common indent (leading tabs) of the first non-empty content line.
	var contentIndent string
	for _, line := range innerLines {
		if strings.TrimSpace(line) != "" {
			contentIndent = leadingTabs(line)
			break
		}
	}

	// Strip the common indent from each content line.
	var stripped []string
	for _, line := range innerLines {
		stripped = append(stripped, strings.TrimPrefix(line, contentIndent))
	}
	expr := strings.Join(stripped, "\n")

	// Format the expression.
	formatted := jqFmt(strings.TrimSpace(expr), false)
	if formatted == "" || formatted == expr {
		return // parse error or no change
	}

	// Re-add the content indent to each formatted line.
	formattedLines := strings.Split(strings.TrimRight(formatted, "\n"), "\n")
	var reindented []string
	for _, line := range formattedLines {
		if strings.TrimSpace(line) == "" {
			reindented = append(reindented, "")
		} else {
			reindented = append(reindented, contentIndent+line)
		}
	}

	// The closing ' should be at one less tab than the content (shell context level).
	closeIndent := ""
	if len(contentIndent) > 0 {
		closeIndent = contentIndent[:len(contentIndent)-1]
	}

	sgl.Value = "\n" + strings.Join(reindented, "\n") + "\n" + closeIndent
}

// reformatJQInLine reformats any `jq '...'` expression on a single shell line.
// Used by FormatRUN for jq inside Dockerfile RUN blocks.
func reformatJQInLine(line string, jqFmt func(expr string, inline bool) string) string {
	// Find 'jq' followed by optional flags followed by a single-quoted expression.
	// Simple heuristic: look for a single-quoted string at end of line.
	sq := strings.LastIndex(line, "'")
	if sq < 1 {
		return line
	}
	firstSQ := strings.Index(line, "'")
	if firstSQ == sq {
		return line // only one quote — malformed or empty
	}
	expr := line[firstSQ+1 : sq]
	if strings.Contains(expr, "'") {
		return line // nested quotes — too complex
	}

	formatted := jqFmt(strings.TrimSpace(expr), true)
	if formatted == "" || strings.Contains(formatted, "\n") {
		return line
	}
	return line[:firstSQ+1] + formatted + "'"
}

// leadingTabs returns the leading tab characters of s.
func leadingTabs(s string) string {
	i := 0
	for i < len(s) && s[i] == '\t' {
		i++
	}
	return s[:i]
}

// DetectLang guesses the shell language variant from a shebang line.
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

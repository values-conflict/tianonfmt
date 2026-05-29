package shell

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/values-conflict/tianonfmt/jq"
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
// are reformatted using the jq formatter.
func Format(src string, lang syntax.LangVariant) (string, error) {
	f, err := parseShell(src, lang)
	if err != nil {
		return "", err
	}
	applyFormatRewrites(f)
	normalizeAllShortFlags(f, false)
	if err := formatJQInAST(f); err != nil {
		return "", err
	}
	return printShell(f)
}

// FormatWithTidy formats a shell script with idiomatic rewrites applied first.
// Rewrites: shebang normalisation; set flag normalisation (set -e → set -eu / set -Eeuo pipefail);
// "|| true" → "|| :"; "which" → "command -v".
func FormatWithTidy(src string, lang syntax.LangVariant) (string, error) {
	src = TidyShebang(src)
	// Re-detect language after shebang normalisation: TidyShebang may have
	// changed #!/bin/sh → #!/usr/bin/env bash, so NormalizeSetFlags must use
	// the post-tidy language to remain idempotent.
	lang = DetectLang(src)
	src = NormalizeSetFlags(src, lang)
	f, err := parseShell(src, lang)
	if err != nil {
		return "", err
	}
	applyFormatRewrites(f)
	ApplyTidy(f)
	applyTidyHeredocs(f)
	if err := formatJQInAST(f); err != nil {
		return "", err
	}
	return printShell(f)
}

// formatJQExpr parses and reformats a jq expression for embedding in shell.
func formatJQExpr(expr string, inline bool) (string, error) {
	return jq.FormatStr(expr, inline)
}

// ParseFile parses src as a shell script of the given language variant.
func ParseFile(src string, lang syntax.LangVariant) (*syntax.File, error) {
	return parseShell(src, lang)
}

func parseShell(src string, lang syntax.LangVariant) (*syntax.File, error) {
	parser := syntax.NewParser(syntax.KeepComments(true), syntax.Variant(lang))
	f, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return nil, fmt.Errorf("shell parse: %w", err)
	}
	return f, nil
}

func printShell(f *syntax.File) (string, error) {
	var buf bytes.Buffer
	if err := newPrinter().Print(&buf, f); err != nil {
		return "", fmt.Errorf("shell format: %w", err)
	}
	s := fixMultiLineClauses(buf.String())
	s = fixArraySpacing(s)
	s = fixHereStringSpacing(s)
	s = fixFdRedirectSpacing(s)
	s = fixArithSpacing(s)
	s = fixEvalQuoting(s)
	s = joinShiftPairs(s)
	return s, nil
}

// reHeredocSpace matches the space SpaceRedirects inserts after <<, <<-, or <<<
// when followed by a delimiter character (letter, underscore, or quote).
// The digit-prefix case (e.g. arithmetic << 2) is excluded by requiring the
// character after the space to be non-digit.
var reHeredocSpace = regexp.MustCompile(`(<<-?) (['"A-Za-z_])`)

// reFdRedirectSpace matches the space SpaceRedirects inserts after a
// file-descriptor redirect like "2> /dev/null" or "2>> file".
// Corpus style is cuddled: "2>/dev/null".
//
// The pattern anchors on whitespace before the fd digits so that "echo 2 > x"
// (where 2 is a plain argument, not an fd) is never matched — in that form
// ">" is a standalone token separated by spaces on both sides, not
// immediately adjacent to the digit.
var reFdRedirectSpace = regexp.MustCompile(`([ \t])([0-9]+)(>>?) `)

// fixEvalQuoting wraps bare $(...) arguments to eval in double quotes:
//
//	eval $(cmd)   →  eval "$(cmd)"
//
// eval $(cmd) is almost never correct: word-splitting on the output of $(cmd)
// changes semantics unexpectedly.  Implemented as text post-processing rather
// than an AST rewrite to avoid zero-position node issues that cause comment
// misplacement in the printer.  Already-quoted calls are left unchanged.
func fixEvalQuoting(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	changed := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		indent := leadingTabs(line)
		body := line[len(indent):]

		// Only act on "eval $(" lines (not already "eval "$(").
		if !strings.HasPrefix(body, "eval $(") {
			out = append(out, line)
			continue
		}

		// Find the $( position within the line.
		parenAt := len(indent) + len("eval ")
		// Walk the remaining text (possibly spanning lines) to find matching ).
		text := strings.Join(lines[i:], "\n")
		closeRel := evalCmdSubstClose(text, parenAt+1) // just after $
		if closeRel < 0 {
			out = append(out, line)
			continue
		}
		closeAbs := closeRel // relative to start of text (lines[i:])

		// Reconstruct the quoted form.
		full := text[:closeAbs+1] // everything up to and including )
		tail := text[closeAbs+1:] // everything after )
		// Take the newline as the boundary for the tail of the current segment.
		quoted := full[:parenAt] + `"` + full[parenAt:] + `"` + tail
		// Re-split on the newlines we consumed.
		consumed := strings.Count(full, "\n")
		newLines := strings.Split(quoted, "\n")
		out = append(out, newLines[:consumed+1]...)
		i += consumed
		changed = true
	}

	if !changed {
		return src
	}
	return strings.Join(out, "\n")
}

// evalCmdSubstClose finds the index of the ) that closes the $( starting at
// src[start] (src[start] == '(' after the '$').  Returns -1 if not found.
func evalCmdSubstClose(src string, start int) int {
	depth := 1
	i := start + 1 // skip the opening (
	for i < len(src) && depth > 0 {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		case '\'':
			i++
			for i < len(src) && src[i] != '\'' {
				i++
			}
		case '"':
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				i++
			}
		case '`':
			i++
			for i < len(src) && src[i] != '`' {
				i++
			}
		}
		i++
	}
	return -1
}

// fixArithSpacing adds spaces inside non-empty arithmetic expressions:
//
//	((x++))        →  (( x++ ))
//	$((a + b))     →  $(( a + b ))
//	(())           →  unchanged (empty)
//	(( x++ ))      →  unchanged (already spaced)
//
// The printer removes outer spaces; Tianon always writes them.
func fixArithSpacing(src string) string {
	var result strings.Builder
	changed := false
	i := 0
	for i < len(src) {
		dollar := i+3 <= len(src) && src[i] == '$' && src[i+1] == '(' && src[i+2] == '('
		plain := !dollar && i+2 <= len(src) && src[i] == '(' && src[i+1] == '('

		if !dollar && !plain {
			result.WriteByte(src[i])
			i++
			continue
		}

		prefix := "(("
		skip := 2
		if dollar {
			prefix = "$(("
			skip = 3
		}

		close := arithCloseParen(src, i+skip)
		if close < 0 {
			result.WriteByte(src[i])
			i++
			continue
		}

		content := src[i+skip : close]
		if content == "" {
			result.WriteString(prefix + "))")
		} else {
			l, r := "", ""
			if content[0] != ' ' {
				l = " "
				changed = true
			}
			if content[len(content)-1] != ' ' {
				r = " "
				changed = true
			}
			result.WriteString(prefix + l + content + r + "))")
		}
		i = close + 2
	}
	if !changed {
		return src
	}
	return result.String()
}

// arithCloseParen returns the index of the first ')' of the closing '))'
// for an arithmetic expression opening at src[start] (just after '((' or '$((').
// Tracks inner paren depth and skips quoted strings.  Returns -1 if not found.
func arithCloseParen(src string, start int) int {
	depth := 0
	i := start
	for i < len(src) {
		switch src[i] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				if i+1 < len(src) && src[i+1] == ')' {
					return i
				}
				return -1
			}
			depth--
		case '\'':
			i++
			for i < len(src) && src[i] != '\'' {
				i++
			}
		case '"':
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				i++
			}
		}
		i++
	}
	return -1
}

// fixFdRedirectSpacing removes the space that SpaceRedirects inserts between a
// file-descriptor number and its redirect file, restoring corpus style:
//
//	2> /dev/null  →  2>/dev/null
//	2>> log       →  2>>log
//
// Bare redirects (">" and ">>") keep their space; only fd-prefixed forms are
// affected.  "echo 2 > x" (plain argument, not an fd) is not matched because
// ">" is a standalone token with whitespace on both sides, not directly
// adjacent to the digit.
func fixFdRedirectSpacing(src string) string {
	return reFdRedirectSpace.ReplaceAllString(src, "$1$2$3")
}

// fixHereStringSpacing removes the space that SpaceRedirects inserts after
// heredoc and here-string operators so that delimiters are always cuddled:
//
//	<<- 'EOH'  →  <<-'EOH'
//	<<  EOF    →  <<EOF
//	<<<  "$x"  →  <<<"$x"
//
// SpaceRedirects correctly spaces output redirects (> file) but heredoc
// operators are always written cuddled with their delimiter.
func fixHereStringSpacing(src string) string {
	src = strings.ReplaceAll(src, "<<< ", "<<<")
	return reHeredocSpace.ReplaceAllString(src, "$1$2")
}

// joinShiftPairs collapses consecutive assignment + shift lines onto one line
// to emphasise the semantic relationship between consuming a positional
// parameter and advancing the argument list:
//
//	repo="$1"          →   repo="$1"; shift
//	shift
//
//	local a="$1"       →   local a="$1"; shift
//	shift
//
// Both lines must be at the same indentation level.  The next line may be a
// bare "shift" or "shift N" (with a count).  Already-joined lines are skipped.
func joinShiftPairs(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	changed := false

	i := 0
	for i < len(lines) {
		line := lines[i]
		if i+1 < len(lines) {
			next := lines[i+1]
			indent := leadingTabs(line)
			body := line[len(indent):]
			nextIndent := leadingTabs(next)
			nextBody := next[len(nextIndent):]

			if indent == nextIndent &&
				isShiftableAssignment(body) &&
				isShiftStatement(nextBody) {
				out = append(out, line+"; "+nextBody)
				i += 2
				changed = true
				continue
			}
		}
		out = append(out, line)
		i++
	}

	if !changed {
		return src
	}
	return strings.Join(out, "\n")
}

// isShiftableAssignment reports whether s is a plain variable assignment line
// (with optional local/declare prefix) that pairs naturally with a following
// shift.  Lines already containing ';' are excluded (already joined).
func isShiftableAssignment(s string) bool {
	if strings.Contains(s, ";") {
		return false
	}
	// Strip optional prefixes: local, local -r, declare, declare -r, etc.
	stripped := s
	for _, pfx := range []string{
		"local -r ", "local -x ", "local -n ", "local ",
		"declare -r ", "declare -x ", "declare ",
		"readonly ", "export ",
	} {
		if strings.HasPrefix(s, pfx) {
			stripped = s[len(pfx):]
			break
		}
	}
	// Remaining must start with IDENTIFIER=
	j := 0
	for j < len(stripped) && isShellIdentChar(stripped[j]) {
		j++
	}
	return j > 0 && j < len(stripped) && stripped[j] == '='
}

// isShiftStatement reports whether s (trimmed body) is a shift statement.
func isShiftStatement(s string) bool {
	return s == "shift" || strings.HasPrefix(s, "shift ")
}

func isShellIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

// fixArraySpacing adds spaces inside non-empty inline bash array literals:
//
//	args=(--tab -L "$dir")   →  args=( --tab -L "$dir" )
//	files+=("$x")            →  files+=( "$x" )
//	toDelete=()              →  unchanged (empty)
//	declare -A m=(           →  unchanged (multi-line: no ')' on same line)
//
// Multi-line arrays are untouched because the matching ')' is on a later line.
func fixArraySpacing(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		fixed := fixArraySpacingLine(line)
		if fixed != line {
			changed = true
		}
		out = append(out, fixed)
	}
	if !changed {
		return src
	}
	return strings.Join(out, "\n")
}

func fixArraySpacingLine(line string) string {
	if !strings.Contains(line, "=(") {
		return line
	}
	var result strings.Builder
	i := 0
	for i < len(line) {
		c := line[i]
		// Skip single-quoted strings: no array assignments inside.
		if c == '\'' {
			result.WriteByte(c)
			i++
			for i < len(line) && line[i] != '\'' {
				result.WriteByte(line[i])
				i++
			}
			if i < len(line) {
				result.WriteByte(line[i]) // closing '
				i++
			}
			continue
		}
		// Skip double-quoted strings.
		if c == '"' {
			result.WriteByte(c)
			i++
			for i < len(line) && line[i] != '"' {
				if line[i] == '\\' && i+1 < len(line) {
					result.WriteByte(line[i])
					i++
				}
				result.WriteByte(line[i])
				i++
			}
			if i < len(line) {
				result.WriteByte(line[i]) // closing "
				i++
			}
			continue
		}
		// Skip shell comments.
		if c == '#' {
			result.WriteString(line[i:])
			break
		}
		// Match =( — the array opening.
		if c == '=' && i+1 < len(line) && line[i+1] == '(' {
			close := arrayCloseParen(line, i+1)
			if close > 0 {
				content := line[i+2 : close]
				if content == "" {
					result.WriteString("=()")
				} else {
					l, r := " ", " "
					if content[0] == ' ' {
						l = ""
					}
					if content[len(content)-1] == ' ' {
						r = ""
					}
					result.WriteString("=(")
					result.WriteString(l)
					result.WriteString(content)
					result.WriteString(r)
					result.WriteString(")")
				}
				i = close + 1
				continue
			}
		}
		result.WriteByte(c)
		i++
	}
	return result.String()
}

// arrayCloseParen returns the index of the ')' that closes the '(' at openPos,
// or -1 if the closing paren is not found on the same line (multi-line array).
// Tracks paren depth and skips quoted strings and command substitutions.
func arrayCloseParen(line string, openPos int) int {
	depth := 1
	i := openPos + 1
	for i < len(line) && depth > 0 {
		switch line[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		case '\'': // single-quoted: scan to closing '
			i++
			for i < len(line) && line[i] != '\'' {
				i++
			}
		case '"': // double-quoted: scan to closing " (respect \")
			i++
			for i < len(line) && line[i] != '"' {
				if line[i] == '\\' && i+1 < len(line) {
					i++
				}
				i++
			}
		case '`': // backtick substitution
			i++
			for i < len(line) && line[i] != '`' {
				i++
			}
		}
		i++
	}
	return -1 // no matching ) on this line
}

// fixMultiLineClauses normalises multi-line for/while/if constructs so that
// "; do" and "; then" always sit on their own line at the keyword's indentation
// level, with a trailing "\" on every preceding item line.
//
// The printer produces two broken forms:
//
//  1. Last item joined with "; do" on the same line:
//     \tbookworm; do  →  \tbookworm \
//     ; do
//
//  2. "; do" indented one level too deep (same as the items):
//     \t; do  →  ; do
//
// Both corrections are only applied when the preceding line ends with "\"
// (the trailing-backslash signal for a multi-line construct).
func fixMultiLineClauses(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines)+4)
	changed := false

	for i, line := range lines {
		prevHasBackslash := i > 0 && len(lines[i-1]) > 0 && lines[i-1][len(lines[i-1])-1] == '\\'
		if prevHasBackslash {
			// Fix 1: bare "; do"/"; then" sitting one tab level too deep — dedent it.
			if tabs, kw, ok := matchSemiClauseAlone(line); ok && len(tabs) > 0 {
				out = append(out, tabs[1:]+"; "+kw)
				changed = true
				continue
			}
			// Fix 2: "item; do" on one line — add "\" to item, put "; do" on own line.
			if tabs, item, kw, ok := matchSemiClauseJoined(line); ok {
				prevTabs := ""
				if len(tabs) > 0 {
					prevTabs = tabs[1:]
				}
				out = append(out, tabs+item+" \\")
				out = append(out, prevTabs+"; "+kw)
				changed = true
				continue
			}
		}
		out = append(out, line)
	}

	if !changed {
		return src
	}
	return strings.Join(out, "\n")
}

// matchSemiClauseAlone matches a line of the form "\t+; do" or "\t+; then".
func matchSemiClauseAlone(line string) (tabs, keyword string, ok bool) {
	i := 0
	for i < len(line) && line[i] == '\t' {
		i++
	}
	if i == 0 {
		return "", "", false
	}
	rest := line[i:]
	switch rest {
	case "; do", "; then":
		return line[:i], rest[2:], true
	}
	return "", "", false
}

// matchSemiClauseJoined matches a line of the form "\t+<item>; do" or
// "\t+<item>; then" where <item> does not end with "\".
func matchSemiClauseJoined(line string) (tabs, item, keyword string, ok bool) {
	i := 0
	for i < len(line) && line[i] == '\t' {
		i++
	}
	if i == 0 {
		return "", "", "", false
	}
	tabs = line[:i]
	rest := line[i:]
	for _, kw := range []string{"; do", "; then"} {
		if strings.HasSuffix(rest, kw) {
			body := rest[:len(rest)-len(kw)]
			if len(body) > 0 && body[len(body)-1] != '\\' {
				return tabs, body, kw[2:], true
			}
		}
	}
	return "", "", "", false
}

func newPrinter() *syntax.Printer {
	return syntax.NewPrinter(
		syntax.Indent(0),            // 0 = tabs (corpus: all .sh files use tabs)
		syntax.BinaryNextLine(true), // && / || at start of continuation line (bash.md §Notable omissions)
		syntax.SwitchCaseIndent(true),
		syntax.KeepPadding(false),
		syntax.SpaceRedirects(true), // ">file" → "> file"; fd-prefixed forms (e.g. "2>/dev/null") are corrected back by fixFdRedirectSpacing (bash.md §Redirections)
	)
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
//     (https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile.template#L7)
//     (https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/tailscale/Dockerfile#L23)
//   - Blank continuation lines (lone "\"): preserved as-is
//
// FormatRUN normalises the continuation lines of a Dockerfile RUN instruction.
func FormatRUN(lines []string) ([]string, error) {
	if len(lines) == 0 {
		return lines, nil
	}

	depth := 0
	var result []string

	for _, raw := range lines {
		// Strip the trailing " \" or "\" continuation marker.
		// Trim trailing whitespace first: some sources emit "\ " (backslash +
		// trailing space) which is still a continuation marker.
		rawTrimmed := strings.TrimRight(raw, " \t")
		hasCont := strings.HasSuffix(rawTrimmed, "\\")
		line := raw
		if hasCont {
			line = strings.TrimRight(rawTrimmed[:len(rawTrimmed)-1], " \t")
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

		var err error
		trimmed, err = reformatJQInLine(trimmed)
		if err != nil {
			return nil, err
		}

		result = append(result, appendCont(strings.Repeat("\t", depth+1)+trimmed, hasCont))

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

	return result, nil
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

// ── tidy-level heredoc rewrites ──────────────────────────────────────────────

// applyTidyHeredocs applies two heredoc normalisations in --tidy mode:
//
//  1. << → <<- (tab-stripping form): any heredoc whose body has no leading
//     tab characters on any content line is upgraded.  Lines with leading tabs
//     are the single exception — those tabs are intentional output content.
//
//  2. <<WORD → <<'WORD' (single-quoted form): when a heredoc uses an unquoted
//     delimiter but the body contains no bash expansions ($, `, \), the body
//     is literal text and the delimiter is upgraded to single-quoted to make
//     that explicit.
func applyTidyHeredocs(f *syntax.File) {
	syntax.Walk(f, func(n syntax.Node) bool {
		stmt, ok := n.(*syntax.Stmt)
		if !ok {
			return true
		}
		for _, redir := range stmt.Redirs {
			if redir.Hdoc == nil {
				continue
			}

			// Upgrade << → <<- when body has no leading tabs.
			if redir.Op == syntax.Hdoc && !hdocBodyHasLeadingTabs(redir.Hdoc) {
				redir.Op = syntax.DashHdoc
			}

			// Upgrade unquoted delimiter to single-quoted when body is literal.
			if redir.Word != nil && isUnquotedHeredocWord(redir.Word) && !hdocBodyHasExpansions(redir.Hdoc) {
				lit := wordLit(redir.Word)
				redir.Word = &syntax.Word{
					Parts: []syntax.WordPart{
						&syntax.SglQuoted{Value: lit},
					},
				}
			}
		}
		return true
	})
}

// hdocBodyHasLeadingTabs reports whether any non-blank line of the heredoc
// body word starts with a tab.  Such tabs are intentional output content and
// must be preserved — upgrading to <<- would strip them.
func hdocBodyHasLeadingTabs(hdoc *syntax.Word) bool {
	for _, part := range hdoc.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			continue
		}
		for line := range strings.SplitSeq(lit.Value, "\n") {
			if len(line) > 0 && line[0] == '\t' {
				return true
			}
		}
	}
	return false
}

// hdocBodyHasExpansions reports whether the heredoc body word contains any
// non-literal parts (variable expansions, command substitutions, etc.).
// If the body is entirely literal, the delimiter can be safely single-quoted.
func hdocBodyHasExpansions(hdoc *syntax.Word) bool {
	for _, part := range hdoc.Parts {
		if _, ok := part.(*syntax.Lit); !ok {
			return true
		}
	}
	return false
}

// isUnquotedHeredocWord reports whether a heredoc delimiter word is a plain
// unquoted literal (not single-quoted, double-quoted, or otherwise complex).
func isUnquotedHeredocWord(w *syntax.Word) bool {
	if len(w.Parts) != 1 {
		return false
	}
	_, isLit := w.Parts[0].(*syntax.Lit)
	return isLit
}

// ── format-level AST rewrites ────────────────────────────────────────────────

// applyFormatRewrites applies format-level AST rewrites that are always correct
// regardless of tidy mode.  Called from both Format and FormatWithTidy.
// Note: eval $(...) → eval "$(...)​" quoting is done as a text-level
// post-process (fixEvalQuoting) to avoid AST position issues with
// zero-position nodes that cause comment misplacement.
func applyFormatRewrites(_ *syntax.File) {
}

// reLongFlagInComment matches a long-form CLI flag name inside a comment:
// "--" followed by at least two lowercase letters.  This distinguishes actual
// flag references like "--canonicalize" from "--" used as an em-dash in prose.
var reLongFlagInComment = regexp.MustCompile(`--[a-z]{2,}`)

// isPlatformComment reports whether text (the body of a shell comment, without
// the leading '#') signals a platform-specific or portability-related flag
// constraint that the formatter should respect rather than normalise.
//
// Two signals are recognised:
//   - Platform/tooling keywords (case-insensitive): macos, darwin, bsd,
//     coreutils, gnu, busybox, alpine, musl, posix, portable, portability
//   - A long-form flag reference: "--word" (at least two lowercase letters)
//     signals an explicit short-vs-long comparison (e.g. "# -f not --canonicalize").
//     A bare "--" is treated as an em-dash and does not match.
func isPlatformComment(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range []string{
		"macos", "darwin", "bsd",
		"coreutils", "gnu", "busybox", "alpine", "musl",
		"posix", "portable", "portability",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return reLongFlagInComment.MatchString(text)
}

// stmtHasPlatformComment reports whether s has a trailing inline comment (on
// the same line as s.End()) that signals a platform-specific or
// portability-related flag constraint.  Comments that merely describe the
// command's purpose (e.g. "# check mode") are not matched.
func stmtHasPlatformComment(s *syntax.Stmt) bool {
	endLine := s.End().Line()
	for _, c := range s.Comments {
		if c.Pos().Line() == endLine {
			return isPlatformComment(c.Text)
		}
	}
	return false
}

// collectProtectedLines builds a set of source line numbers on which flag
// normalization should be suppressed.  Every line within the span of a
// statement whose trailing comment signals a platform constraint is marked
// protected — this covers not only the direct command but also any command
// nested inside a $(…) subshell on the same source line
// (e.g. gitDir="$(readlink -f …)" # -f not --canonicalize for macOS).
func collectProtectedLines(f *syntax.File) map[uint]bool {
	var protected map[uint]bool
	syntax.Walk(f, func(n syntax.Node) bool {
		s, ok := n.(*syntax.Stmt)
		if !ok || !stmtHasPlatformComment(s) {
			return true
		}
		if protected == nil {
			protected = make(map[uint]bool)
		}
		for line := s.Pos().Line(); line <= s.End().Line(); line++ {
			protected[line] = true
		}
		return true
	})
	return protected
}

// normalizeAllShortFlags walks every CallExpr in the file and applies
// normalizeShortFlags, skipping commands on source lines that are protected
// by a trailing comment.  tidy=false for the format pass, tidy=true for tidy.
// Called unconditionally (unlike formatJQInAST which is jqFmt-gated).
func normalizeAllShortFlags(f *syntax.File, tidy bool) {
	protected := collectProtectedLines(f)
	syntax.Walk(f, func(n syntax.Node) bool {
		if ce, ok := n.(*syntax.CallExpr); ok {
			if !protected[ce.Pos().Line()] {
				normalizeShortFlags(ce, false, tidy)
			}
		}
		return true
	})
}

// ── jq-in-shell AST rewriting ────────────────────────────────────────────────

// formatJQInAST rewrites jq expression arguments inside `jq '...'` invocations,
// proactively indenting multi-line expressions to match the shell nesting depth
// of the jq call.  A jq call at top level (depth 0) gets 1-tab content; inside
// a for/if/while body (depth 1) it gets 2-tab content; and so on.
//
// Detection heuristic: the command is `jq`, the last non-flag argument is a
// SglQuoted string (value flags like --arg NAME VALUE are skipped).
//
// Corpus refs:
//   - https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L62 single-line
//   - https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L67-L70 multi-line
func formatJQInAST(f *syntax.File) error {
	return jqWalkStmts(f.Stmts, 0)
}

// jqWalkStmts visits each statement in stmts at the given shell nesting depth.
func jqWalkStmts(stmts []*syntax.Stmt, depth int) error {
	for _, s := range stmts {
		if err := jqWalkStmt(s, depth); err != nil {
			return err
		}
	}
	return nil
}

// jqWalkStmt visits a single statement (its command and any redirects).
func jqWalkStmt(stmt *syntax.Stmt, depth int) error {
	if stmt == nil {
		return nil
	}
	if err := jqWalkCmd(stmt.Cmd, depth); err != nil {
		return err
	}
	for _, r := range stmt.Redirs {
		if r.Word != nil {
			if err := jqWalkWord(r.Word, depth); err != nil {
				return err
			}
		}
	}
	return nil
}

// jqWalkCmd visits a shell command, increasing depth for compound-statement bodies.
func jqWalkCmd(cmd syntax.Command, depth int) error {
	if cmd == nil {
		return nil
	}
	switch v := cmd.(type) {
	case *syntax.CallExpr:
		if len(v.Args) > 0 && wordLit(v.Args[0]) == "jq" {
			if sgl := findJQExprArg(v.Args[1:]); sgl != nil {
				if err := reformatSglQuoted(sgl, depth); err != nil {
					return err
				}
			}
		}
		for _, assign := range v.Assigns {
			if assign.Value != nil {
				if err := jqWalkWord(assign.Value, depth); err != nil {
					return err
				}
			}
		}
		for _, arg := range v.Args {
			if err := jqWalkWord(arg, depth); err != nil {
				return err
			}
		}
	case *syntax.IfClause:
		// IfClause is recursive: .Else is another *IfClause for elif/else.
		for ic := v; ic != nil; ic = ic.Else {
			// When the first condition statement is on a different line than the
			// "if"/"elif" token, the printer indents the condition body by one
			// extra level — mirror that in the depth counter.
			condDepth := depth
			if len(ic.Cond) > 0 && ic.Position.Line() != ic.Cond[0].Pos().Line() {
				condDepth = depth + 1
			}
			if err := jqWalkStmts(ic.Cond, condDepth); err != nil {
				return err
			}
			if err := jqWalkStmts(ic.Then, depth+1); err != nil {
				return err
			}
		}
	case *syntax.WhileClause:
		if err := jqWalkStmts(v.Cond, depth); err != nil {
			return err
		}
		if err := jqWalkStmts(v.Do, depth+1); err != nil {
			return err
		}
	case *syntax.ForClause:
		if loop, ok := v.Loop.(*syntax.WordIter); ok {
			for _, item := range loop.Items {
				if err := jqWalkWord(item, depth); err != nil {
					return err
				}
			}
		}
		if err := jqWalkStmts(v.Do, depth+1); err != nil {
			return err
		}
	case *syntax.FuncDecl:
		if err := jqWalkStmt(v.Body, depth+1); err != nil {
			return err
		}
	case *syntax.Block:
		if err := jqWalkStmts(v.Stmts, depth+1); err != nil {
			return err
		}
	case *syntax.Subshell:
		// ( ... ) does not add a printer indent level
		if err := jqWalkStmts(v.Stmts, depth); err != nil {
			return err
		}
	case *syntax.BinaryCmd:
		if err := jqWalkStmt(v.X, depth); err != nil {
			return err
		}
		// When Y starts on a different line than X, BinaryNextLine mode puts
		// the operator on a new indented line — reflect that in the depth.
		yDepth := depth
		if v.X.Pos().Line() != v.Y.Pos().Line() {
			yDepth = depth + 1
		}
		if err := jqWalkStmt(v.Y, yDepth); err != nil {
			return err
		}
	case *syntax.TimeClause:
		if err := jqWalkStmt(v.Stmt, depth); err != nil {
			return err
		}
	case *syntax.CoprocClause:
		if err := jqWalkStmt(v.Stmt, depth); err != nil {
			return err
		}
	case *syntax.CaseClause:
		if err := jqWalkWord(v.Word, depth); err != nil {
			return err
		}
		for _, item := range v.Items {
			// SwitchCaseIndent(true) adds two indent levels for case-body
			// statements (one for the pattern line, one for the body), so depth
			// must advance by two to match the actual printed indentation.
			if err := jqWalkStmts(item.Stmts, depth+2); err != nil {
				return err
			}
		}
	}
	return nil
}

// jqWalkWord descends into word parts that may contain command substitutions.
func jqWalkWord(word *syntax.Word, depth int) error {
	if word == nil {
		return nil
	}
	for _, part := range word.Parts {
		if err := jqWalkWordPart(part, depth); err != nil {
			return err
		}
	}
	return nil
}

// jqWalkWordPart visits a single word part.
func jqWalkWordPart(part syntax.WordPart, depth int) error {
	switch v := part.(type) {
	case *syntax.CmdSubst:
		// When the body starts on the same line as $( (inline form), the printer
		// does not add an indent level.  When the body starts on a new line, the
		// printer indents by one level — reflect that in the depth.
		cmdDepth := depth
		if len(v.Stmts) > 0 && v.Left.Line() != v.Stmts[0].Pos().Line() {
			cmdDepth = depth + 1
		}
		if err := jqWalkStmts(v.Stmts, cmdDepth); err != nil {
			return err
		}
	case *syntax.DblQuoted:
		for _, p := range v.Parts {
			if err := jqWalkWordPart(p, depth); err != nil {
				return err
			}
		}
	case *syntax.ParamExp:
		if v.Index != nil {
			// index expression — no jq calls inside arithmetic
		}
	case *syntax.ProcSubst:
		// <(...) always puts its body on a new indented line.
		if err := jqWalkStmts(v.Stmts, depth+1); err != nil {
			return err
		}
	}
	return nil
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
	"--arg":       2, // NAME VALUE
	"--argjson":   2,
	"--slurpfile": 2,
	"--rawfile":   2,
	"--json-args": 0, // terminates flag processing; rest are jq positional args
	"--args":      0,
	"-f":          1, // FILENAME
	"--from-file": 1,
	"--indent":    1, // N
	"--tab":       0,
	"--run-tests": 0,
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

// reformatSglQuoted reformats sgl.Value as a jq expression.
// depth is the shell nesting depth of the containing jq call (0 = top level).
// Multi-line expressions are always indented to depth+1 tabs regardless of
// the original indentation, normalising column-0, space-based, or otherwise
// wrong indentation.  Single-line expressions are compacted in place.
func reformatSglQuoted(sgl *syntax.SglQuoted, depth int) error {
	val := sgl.Value
	if val == "" {
		return nil
	}

	line := sgl.Pos().Line()
	wrapErr := func(err error) error {
		return fmt.Errorf("jq expression at line %d: %w", line, err)
	}

	if !strings.Contains(val, "\n") {
		// Single-line: format compactly.
		formatted, err := formatJQExpr(strings.TrimSpace(val), true)
		if err != nil {
			return wrapErr(err)
		}
		if !strings.Contains(formatted, "\n") {
			sgl.Value = formatted
		}
		return nil
	}

	// Multi-line: strip whatever indentation the content has, reformat with jq,
	// then re-indent to the canonical depth-based level.
	contentIndent := strings.Repeat("\t", depth+1)
	closeIndent := strings.Repeat("\t", depth)

	// Determine and strip the existing common indent from the content.
	inner := strings.Trim(val, "\n")
	innerLines := strings.Split(inner, "\n")

	var existingIndent string
	for _, line := range innerLines {
		if strings.TrimSpace(line) != "" {
			existingIndent = leadingWhitespace(line)
			break
		}
	}
	var stripped []string
	for _, line := range innerLines {
		stripped = append(stripped, strings.TrimPrefix(line, existingIndent))
	}
	expr := strings.Join(stripped, "\n")

	// Format the jq expression.
	formatted, err := formatJQExpr(strings.TrimSpace(expr), false)
	if err != nil {
		return wrapErr(err)
	}

	// Re-indent the formatted lines at the canonical depth.
	formattedLines := strings.Split(strings.TrimRight(formatted, "\n"), "\n")
	var reindented []string
	for _, line := range formattedLines {
		if strings.TrimSpace(line) == "" {
			reindented = append(reindented, "")
		} else {
			reindented = append(reindented, contentIndent+line)
		}
	}

	newVal := "\n" + strings.Join(reindented, "\n") + "\n" + closeIndent
	if newVal != val {
		sgl.Value = newVal
	}
	return nil
}

// reformatJQInLine reformats any `jq '...'` expression on a single shell line.
// Used by FormatRUN for jq inside Dockerfile RUN blocks.
func reformatJQInLine(line string) (string, error) {
	// Only attempt jq reformatting if the line actually contains a jq invocation.
	// Without this guard, PowerShell single-quoted strings (and other non-jq
	// constructs) would be incorrectly passed to the jq formatter.
	trimmed := strings.TrimLeft(line, "\t ")
	hasJQ := strings.HasPrefix(trimmed, "jq ") || strings.HasPrefix(trimmed, "jq'") ||
		strings.Contains(line, " jq '") || strings.Contains(line, "\tjq '") ||
		strings.Contains(line, "$(jq '") || strings.Contains(line, "$(jq -")
	if !hasJQ {
		return line, nil
	}

	sq := strings.LastIndex(line, "'")
	if sq < 1 {
		return line, nil
	}
	firstSQ := strings.Index(line, "'")
	if firstSQ == sq {
		return line, nil // only one quote — malformed or empty
	}
	expr := line[firstSQ+1 : sq]
	if strings.Contains(expr, "'") {
		return line, nil // nested quotes — too complex
	}

	formatted, err := formatJQExpr(strings.TrimSpace(expr), true)
	if err != nil {
		return "", err
	}
	if strings.Contains(formatted, "\n") {
		return line, nil
	}
	// Preserve everything after the closing quote (e.g. filename args, redirects).
	return line[:firstSQ+1] + formatted + "'" + line[sq+1:], nil
}

// leadingWhitespace returns the leading whitespace (tabs and spaces) of s.
func leadingWhitespace(s string) string {
	i := 0
	for i < len(s) && (s[i] == '\t' || s[i] == ' ') {
		i++
	}
	return s[:i]
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

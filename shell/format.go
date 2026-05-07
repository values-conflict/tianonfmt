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
	f, err := parseShell(src, lang)
	if err != nil {
		return "", err
	}
	applyFormatRewrites(f)
	if jqFmt != nil {
		formatJQInAST(f, jqFmt)
	}
	return printShell(f)
}

// FormatWithTidy formats a shell script with idiomatic rewrites applied first.
// Rewrites: shebang normalisation; set flag normalisation (set -e → set -eu / set -Eeuo pipefail);
// "|| true" → "|| :"; "which" → "command -v".
func FormatWithTidy(src string, lang syntax.LangVariant, jqFmt func(expr string, inline bool) string) (string, error) {
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
	if jqFmt != nil {
		formatJQInAST(f, jqFmt)
	}
	return printShell(f)
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
	return buf.String(), nil
}

func newPrinter() *syntax.Printer {
	return syntax.NewPrinter(
		syntax.Indent(0),            // 0 = tabs (corpus: all .sh files use tabs)
		syntax.BinaryNextLine(true), // && / || at start of continuation line (bash.md §Notable omissions)
		syntax.SwitchCaseIndent(true),
		syntax.KeepPadding(false),
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

// ── format-level AST rewrites ────────────────────────────────────────────────

// applyFormatRewrites applies format-level AST rewrites that are always correct
// regardless of tidy mode.  Called from both Format and FormatWithTidy.
func applyFormatRewrites(f *syntax.File) {
	quoteEvalArgs(f)
}

// quoteEvalArgs wraps bare $(...) arguments to eval in double quotes.
// eval $(cmd) is almost never correct: word-splitting on the output of $(cmd)
// changes semantics unexpectedly.  eval "$(cmd)" is the idiomatic safe form.
// Only bare CmdSubst args are affected; already-quoted args are left alone.
func quoteEvalArgs(f *syntax.File) {
	syntax.Walk(f, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || len(ce.Args) == 0 || wordLit(ce.Args[0]) != "eval" {
			return true
		}
		for _, arg := range ce.Args[1:] {
			if len(arg.Parts) == 1 {
				if cs, ok := arg.Parts[0].(*syntax.CmdSubst); ok {
					arg.Parts[0] = &syntax.DblQuoted{
						Parts: []syntax.WordPart{cs},
					}
				}
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
func formatJQInAST(f *syntax.File, jqFmt func(expr string, inline bool) string) {
	jqWalkStmts(f.Stmts, 0, jqFmt)
}

// jqWalkStmts visits each statement in stmts at the given shell nesting depth.
func jqWalkStmts(stmts []*syntax.Stmt, depth int, jqFmt func(expr string, inline bool) string) {
	for _, s := range stmts {
		jqWalkStmt(s, depth, jqFmt)
	}
}

// jqWalkStmt visits a single statement (its command and any redirects).
func jqWalkStmt(stmt *syntax.Stmt, depth int, jqFmt func(expr string, inline bool) string) {
	if stmt == nil {
		return
	}
	jqWalkCmd(stmt.Cmd, depth, jqFmt)
	for _, r := range stmt.Redirs {
		if r.Word != nil {
			jqWalkWord(r.Word, depth, jqFmt)
		}
	}
}

// jqWalkCmd visits a shell command, increasing depth for compound-statement bodies.
func jqWalkCmd(cmd syntax.Command, depth int, jqFmt func(expr string, inline bool) string) {
	if cmd == nil {
		return
	}
	switch v := cmd.(type) {
	case *syntax.CallExpr:
		if len(v.Args) > 0 && wordLit(v.Args[0]) == "jq" {
			if sgl := findJQExprArg(v.Args[1:]); sgl != nil {
				reformatSglQuoted(sgl, depth, jqFmt)
			}
		}
		for _, assign := range v.Assigns {
			if assign.Value != nil {
				jqWalkWord(assign.Value, depth, jqFmt)
			}
		}
		for _, arg := range v.Args {
			jqWalkWord(arg, depth, jqFmt)
		}
	case *syntax.IfClause:
		// IfClause is recursive: .Else is another *IfClause for elif/else.
		for ic := v; ic != nil; ic = ic.Else {
			jqWalkStmts(ic.Cond, depth, jqFmt)
			jqWalkStmts(ic.Then, depth+1, jqFmt)
		}
	case *syntax.WhileClause:
		jqWalkStmts(v.Cond, depth, jqFmt)
		jqWalkStmts(v.Do, depth+1, jqFmt)
	case *syntax.ForClause:
		if loop, ok := v.Loop.(*syntax.WordIter); ok {
			for _, item := range loop.Items {
				jqWalkWord(item, depth, jqFmt)
			}
		}
		jqWalkStmts(v.Do, depth+1, jqFmt)
	case *syntax.FuncDecl:
		jqWalkStmt(v.Body, depth+1, jqFmt)
	case *syntax.Block:
		jqWalkStmts(v.Stmts, depth+1, jqFmt)
	case *syntax.Subshell:
		// ( ... ) does not add a printer indent level
		jqWalkStmts(v.Stmts, depth, jqFmt)
	case *syntax.BinaryCmd:
		jqWalkStmt(v.X, depth, jqFmt)
		// When Y starts on a different line than X, BinaryNextLine mode puts
		// the operator on a new indented line — reflect that in the depth.
		yDepth := depth
		if v.X.Pos().Line() != v.Y.Pos().Line() {
			yDepth = depth + 1
		}
		jqWalkStmt(v.Y, yDepth, jqFmt)
	case *syntax.TimeClause:
		jqWalkStmt(v.Stmt, depth, jqFmt)
	case *syntax.CoprocClause:
		jqWalkStmt(v.Stmt, depth, jqFmt)
	case *syntax.CaseClause:
		jqWalkWord(v.Word, depth, jqFmt)
		for _, item := range v.Items {
			jqWalkStmts(item.Stmts, depth+1, jqFmt)
		}
	}
}

// jqWalkWord descends into word parts that may contain command substitutions.
func jqWalkWord(word *syntax.Word, depth int, jqFmt func(expr string, inline bool) string) {
	if word == nil {
		return
	}
	for _, part := range word.Parts {
		jqWalkWordPart(part, depth, jqFmt)
	}
}

// jqWalkWordPart visits a single word part.
func jqWalkWordPart(part syntax.WordPart, depth int, jqFmt func(expr string, inline bool) string) {
	switch v := part.(type) {
	case *syntax.CmdSubst:
		// When the body starts on the same line as $( (inline form), the printer
		// does not add an indent level.  When the body starts on a new line, the
		// printer indents by one level — reflect that in the depth.
		cmdDepth := depth
		if len(v.Stmts) > 0 && v.Left.Line() != v.Stmts[0].Pos().Line() {
			cmdDepth = depth + 1
		}
		jqWalkStmts(v.Stmts, cmdDepth, jqFmt)
	case *syntax.DblQuoted:
		for _, p := range v.Parts {
			jqWalkWordPart(p, depth, jqFmt)
		}
	case *syntax.ParamExp:
		if v.Index != nil {
			// index expression — no jq calls inside arithmetic
		}
	case *syntax.ProcSubst:
		// <(...) always puts its body on a new indented line.
		jqWalkStmts(v.Stmts, depth+1, jqFmt)
	}
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
// depth is the shell nesting depth of the containing jq call (0 = top level).
// Multi-line expressions are always indented to depth+1 tabs regardless of
// the original indentation, normalising column-0, space-based, or otherwise
// wrong indentation.  Single-line expressions are compacted in place.
func reformatSglQuoted(sgl *syntax.SglQuoted, depth int, jqFmt func(expr string, inline bool) string) {
	val := sgl.Value
	if val == "" {
		return
	}

	if !strings.Contains(val, "\n") {
		// Single-line: format compactly.
		formatted := jqFmt(strings.TrimSpace(val), true)
		if formatted != "" && !strings.Contains(formatted, "\n") {
			sgl.Value = formatted
		}
		return
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
	formatted := jqFmt(strings.TrimSpace(expr), false)
	if formatted == "" {
		return // parse failure — preserve original
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
}

// reformatJQInLine reformats any `jq '...'` expression on a single shell line.
// Used by FormatRUN for jq inside Dockerfile RUN blocks.
func reformatJQInLine(line string, jqFmt func(expr string, inline bool) string) string {
	// Only attempt jq reformatting if the line actually contains a jq invocation.
	// Without this guard, PowerShell single-quoted strings (and other non-jq
	// constructs) would be incorrectly passed to the jq formatter.
	trimmed := strings.TrimLeft(line, "\t ")
	hasJQ := strings.HasPrefix(trimmed, "jq ") || strings.HasPrefix(trimmed, "jq'") ||
		strings.Contains(line, " jq '") || strings.Contains(line, "\tjq '") ||
		strings.Contains(line, "$(jq '") || strings.Contains(line, "$(jq -")
	if !hasJQ {
		return line
	}

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
	// Preserve everything after the closing quote (e.g. filename args, redirects).
	return line[:firstSQ+1] + formatted + "'" + line[sq+1:]
}

// isAllSpaces reports whether s consists entirely of space characters (no tabs).
func isAllSpaces(s string) bool {
	return len(s) > 0 && strings.TrimLeft(s, " ") == ""
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

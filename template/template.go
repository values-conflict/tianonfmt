// Package template handles Tianon's jq-template Dockerfile format.
//
// The format is defined by https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk:
//
//   - Plain text lines are passed through as Dockerfile content.
//   - {{ expr }}  — a jq expression whose output is inserted inline.
//   - {{ expr -}} — same, but strips the trailing newline from the text output.
//   - {{ def f: ...; }} / {{ include "…"; }} / {{ import "…" as $x; }}
//     — jq definitions: hoisted to the top of the generated program.
//   - {{ # comment -}} — ignored (pure comment, produces no output).
//   - Multi-line blocks are supported: {{ and }} must be balanced across lines.
//
// The formatter's job is simpler than the awk evaluator's: it only needs to
// locate each {{ }} block and reformat the jq expression inside it.
// Text outside the blocks is passed through verbatim (it's Dockerfile content).
package template

import (
	"regexp"
	"strconv"
	"strings"
)

// Segment is one piece of a template file.
type Segment interface{ templateSeg() }

// TextSeg is verbatim Dockerfile content between {{ }} blocks.
type TextSeg struct{ Text string }

// JQSeg is a {{ jq_expression }} block.
type JQSeg struct {
	Expr   string // raw jq expression text (trimmed)
	EatEOL bool   // true when the closing marker is -}} (strips trailing newline)
}

func (TextSeg) templateSeg() {}
func (JQSeg) templateSeg()   {}

// Parse splits a template source into segments.
func Parse(src string) []Segment {
	const open = "{{"
	const close = "}}"
	const closeEat = "-}}"

	var segs []Segment
	remaining := src

	for {
		start := strings.Index(remaining, open)
		if start < 0 {
			// No more blocks — rest is text.
			if remaining != "" {
				segs = append(segs, TextSeg{Text: remaining})
			}
			break
		}

		// Text before the block.
		if start > 0 {
			segs = append(segs, TextSeg{Text: remaining[:start]})
		}
		remaining = remaining[start+len(open):]

		// Collect until the matching }}, handling nested {{ }} (balanced counting).
		// The awk implementation counts occurrences to handle multi-line blocks.
		depth := 1
		pos := 0
		for pos < len(remaining) && depth > 0 {
			nextOpen := strings.Index(remaining[pos:], open)
			nextEatClose := strings.Index(remaining[pos:], closeEat)
			nextClose := strings.Index(remaining[pos:], close)

			// Find which comes first.
			minIdx := -1
			which := 0 // 1=open, 2=eatClose, 3=close

			if nextOpen >= 0 && (minIdx < 0 || nextOpen < minIdx) {
				minIdx = nextOpen
				which = 1
			}
			// -}} must be checked before }} (it's a longer match).
			if nextEatClose >= 0 && (minIdx < 0 || nextEatClose < minIdx) {
				minIdx = nextEatClose
				which = 2
			} else if nextClose >= 0 && (minIdx < 0 || nextClose < minIdx) {
				minIdx = nextClose
				which = 3
			}

			if minIdx < 0 {
				pos = len(remaining) // unterminated block
				break
			}

			pos += minIdx
			switch which {
			case 1:
				depth++
				pos += len(open)
			case 2:
				depth--
				if depth == 0 {
					expr := stripMinIndent(remaining[:pos])
					segs = append(segs, JQSeg{Expr: expr, EatEOL: true})
					remaining = remaining[pos+len(closeEat):]
				} else {
					pos += len(closeEat)
				}
			case 3:
				depth--
				if depth == 0 {
					expr := stripMinIndent(remaining[:pos])
					segs = append(segs, JQSeg{Expr: expr, EatEOL: false})
					remaining = remaining[pos+len(close):]
				} else {
					pos += len(close)
				}
			}
			if depth == 0 {
				break
			}
		}
		if depth > 0 {
			// Unterminated block — emit remainder as text.
			segs = append(segs, TextSeg{Text: remaining})
			break
		}
	}
	return segs
}

// Format reformats a template file.
//
// jqFmt is called for each jq expression found inside {{ }} blocks.
// It receives the raw expression text and a bool indicating whether the
// block is embedded inline (same line as non-whitespace text).  It returns
// the formatted version, or "" on parse failure so the expression is kept
// as-is.
//
// Non-def, non-comment block expressions are assembled into groups by
// balanced bracket depth and formatted together so that fragments like
// "{{ ) else ( -}}" receive proper context-aware indentation.
func Format(src string, jqFmt func(expr string, inline bool) string) string {
	segs := Parse(src)
	if len(segs) == 0 {
		return src
	}

	// ── Pass 1: classify each JQSeg ─────────────────────────────────────────
	type segClass struct {
		inline    bool
		isDef     bool
		isComment bool
	}
	classes := make([]segClass, len(segs))
	{
		var sim strings.Builder
		for i, seg := range segs {
			switch v := seg.(type) {
			case TextSeg:
				sim.WriteString(v.Text)
			case JQSeg:
				classes[i] = segClass{
					inline:    isInlineContext(sim.String()),
					isDef:     isDefLike(v.Expr),
					isComment: isPureComment(v.Expr),
				}
				// Advance simulated output; use a non-whitespace placeholder
				// so inline-context detection works for subsequent segs.
				sim.WriteString("{{X}}")
			}
		}
	}

	// ── Pass 2: format block-expr segs via whole-program assembly ──────────
	//
	// Non-def, non-comment, non-inline block expressions are grouped by balanced
	// bracket depth (when the running depth returns to 0, a group ends).  Each
	// group is assembled into one jq expression with sentinel markers between
	// blocks, formatted as a whole, then split back into per-block pieces.
	//
	// Sentinel choice per boundary:
	//   - If block[i] ends with "(" AND block[i+1] starts with ")": string
	//     sentinel "SENTINEL_N" fills the empty paren (jq disallows bare ()).
	//   - Otherwise: comment sentinel # SENTINEL_N (safe between any expressions).
	//
	// After formatting, the jq formatter (with comment-placement fix) places:
	//   - Comment sentinels on their own indented lines.
	//   - String sentinels on their own indented lines (cuddled paren format
	//     ensures the "then ()" bodies are rendered multi-line).
	//
	// formattedFor[i] holds the formatted expression for segs[i].
	// An empty string means "use verbatim fallback".
	formattedFor := make([]string, len(segs))

	if jqFmt != nil {
		type blockInfo struct {
			idx  int
			expr string
		}
		var curGroup []blockInfo
		groupDepth := 0

		flushGroup := func() {
			if len(curGroup) == 0 {
				return
			}
			// Single-block groups: format directly and we're done.
			if len(curGroup) == 1 {
				b := curGroup[0]
				if result := jqFmt(b.expr, false); result != "" {
					formattedFor[b.idx] = result
				}
				curGroup = curGroup[:0]
				groupDepth = 0
				return
			}
			// Multi-block groups: always assemble.  Blocks that can be
			// formatted standalone (e.g. ".key" embedded in a deeper
			// context) are still part of the same jq expression and
			// must be formatted in context to get the right indentation.
			// The split gives back a single-line piece for simple
			// expressions; TrimSpace in writeBlockExpr strips depth tabs.
			exprs := make([]string, len(curGroup))
			for j, b := range curGroup {
				exprs[j] = strings.TrimSpace(b.expr)
			}
			var parts []string
			for j, e := range exprs {
				parts = append(parts, e)
				if j < len(exprs)-1 {
					parts = append(parts, assemblySentinel(j, exprs[j], exprs[j+1]))
				}
			}
			assembled := strings.Join(parts, "\n")
			result := jqFmt(assembled, false)
			if result != "" {
				pieces := assemblySentinelRE.Split(result, -1)
				if len(pieces) == len(curGroup) {
					for j, b := range curGroup {
						if p := trimPiece(pieces[j]); p != "" {
							formattedFor[b.idx] = p
						}
					}
				}
				// Sentinel count mismatch → verbatim fallback for all in group.
			}
			curGroup = curGroup[:0]
			groupDepth = 0
		}

		for i, seg := range segs {
			jqSeg, ok := seg.(JQSeg)
			if !ok {
				continue
			}
			cl := classes[i]
			if cl.isComment || cl.isDef || cl.inline {
				continue
			}
			curGroup = append(curGroup, blockInfo{idx: i, expr: jqSeg.Expr})
			groupDepth += bracketDelta(jqSeg.Expr)
			if groupDepth <= 0 {
				flushGroup()
			}
		}
		flushGroup()
	}

	// ── Pass 3: emit ─────────────────────────────────────────────────────────
	var b strings.Builder
	for i, seg := range segs {
		switch v := seg.(type) {
		case TextSeg:
			b.WriteString(v.Text)
		case JQSeg:
			cl := classes[i]
			if cl.isComment {
				writeComment(&b, v)
			} else if cl.isDef {
				writeDefBlock(&b, v, jqFmt)
			} else if cl.inline {
				writeInlineBlock(&b, v, jqFmt)
			} else {
				writeBlockExpr(&b, v, formattedFor[i])
			}
		}
	}
	return b.String()
}

// IsTemplate returns true if src looks like a jq-template file (contains {{ }}).
func IsTemplate(src string) bool {
	return strings.Contains(src, "{{") && strings.Contains(src, "}}")
}

// ── emit helpers ─────────────────────────────────────────────────────────────

// writeComment emits a pure-comment JQSeg.
func writeComment(b *strings.Builder, v JQSeg) {
	var parts []string
	for _, line := range strings.Split(v.Expr, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			parts = append(parts, t)
		}
	}
	closer := " }}"
	if v.EatEOL {
		closer = " -}}"
	}
	if len(parts) == 1 {
		b.WriteString("{{ ")
		b.WriteString(parts[0])
		b.WriteString(closer)
		return
	}
	// Multi-line: indent relative to the current {{ opener position.
	acc := b.String()
	lastNL := strings.LastIndex(acc, "\n")
	var openerIndent string
	if lastNL >= 0 {
		openerIndent = leadingTabs(acc[lastNL+1:])
	}
	b.WriteString("{{\n")
	for _, part := range parts {
		b.WriteString(openerIndent)
		b.WriteString("\t")
		b.WriteString(part)
		b.WriteByte('\n')
	}
	b.WriteString(openerIndent)
	b.WriteString(strings.TrimSpace(closer))
}

// writeDefBlock formats and emits a def/include/import JQSeg.
// The template engine (jq-template.awk) implies the trailing ";" that jq
// requires; the formatted output should not include it.
func writeDefBlock(b *strings.Builder, v JQSeg, jqFmt func(string, bool) string) {
	formatted := ""
	if jqFmt != nil {
		expr := strings.TrimSpace(v.Expr)
		// Try as-is first (expr may already carry a ";").
		if result := jqFmt(expr, false); result != "" {
			formatted = result
		} else if result := jqFmt(expr+"\n;", false); result != "" {
			// Strip the template-implied trailing ";\n".
			stripped := stripImpliedSemicolon(result)
			// Stability check: some jq expressions with comments are not
			// idempotent under repeated formatting (comments migrate between
			// positions).  Only accept the result if re-formatting converges.
			if r2 := jqFmt(strings.TrimSpace(stripped)+"\n;", false); r2 != "" {
				if stripImpliedSemicolon(r2) == stripped {
					formatted = stripped
				}
				// else: unstable → fall through to verbatim
			}
		}
	}
	writeBlockExpr(b, v, formatted)
}

// writeInlineBlock formats and emits an inline JQSeg (embedded in a text line).
func writeInlineBlock(b *strings.Builder, v JQSeg, jqFmt func(string, bool) string) {
	closer := " }}"
	if v.EatEOL {
		closer = " -}}"
	}

	fmtOK := false
	formatted := v.Expr
	if jqFmt != nil && v.Expr != "" {
		if result := jqFmt(v.Expr, true); result != "" {
			formatted = result
			fmtOK = true
		}
	}

	// jq '#' comments are newline-terminated; an inline block containing one
	// would swallow all text up to end-of-string.  Force multi-line layout.
	forceBlock := fmtOK && strings.Contains(formatted, "#")
	if forceBlock {
		if result := jqFmt(v.Expr, false); result != "" {
			formatted = result
		} else {
			fmtOK = false
			formatted = v.Expr
			forceBlock = false
		}
	}

	if !forceBlock && (!fmtOK || !strings.Contains(formatted, "\n")) {
		b.WriteString("{{ ")
		b.WriteString(strings.TrimSpace(formatted))
		b.WriteString(closer)
		return
	}

	// Multi-line formatted result (fmtOK guaranteed true at this point).
	writeBlockExpr(b, v, formatted)
}

// writeBlockExpr emits a non-inline, non-def, non-comment JQSeg.
// formatted is the jq-formatted expression (may be ""), empty means verbatim.
func writeBlockExpr(b *strings.Builder, v JQSeg, formatted string) {
	closer := " }}"
	if v.EatEOL {
		closer = " -}}"
	}

	// Use formatted content if available, otherwise fall back to the raw expr.
	content := formatted
	if content == "" {
		content = v.Expr
	}

	// Single-line (formatted or verbatim): no embedded newlines after trimming.
	// TrimSpace (not just TrimRight) removes depth-context leading tabs that
	// arrive from whole-program assembly pieces — they carry no meaning in the
	// emitted {{ expr }} form.
	contentTrimmed := strings.TrimSpace(content)
	if !strings.Contains(contentTrimmed, "\n") {
		b.WriteString("{{ ")
		b.WriteString(contentTrimmed)
		b.WriteString(closer)
		return
	}

	if formatted != "" {
		// Multi-line formatted content.
		// When the formatted piece came from whole-program assembly (depth > 0),
		// it already carries the correct leading indentation — emit as-is.
		// When the piece is at depth 0 (single standalone expression or def),
		// the jq formatter produces no leading tabs — add one level.
		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		needsIndent := false
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				needsIndent = len(line) == 0 || line[0] != '\t'
				break
			}
		}
		b.WriteString("{{\n")
		for _, line := range lines {
			trimmed := strings.TrimRight(line, " \t")
			if trimmed == "" {
				b.WriteByte('\n')
			} else if needsIndent {
				b.WriteByte('\t')
				b.WriteString(trimmed)
				b.WriteByte('\n')
			} else {
				b.WriteString(trimmed)
				b.WriteByte('\n')
			}
		}
		if v.EatEOL {
			b.WriteString("-}}")
		} else {
			b.WriteString("}}")
		}
		return
	}

	// Multi-line verbatim fallback: preserve content indented under {{.
	acc := b.String()
	lastNL := strings.LastIndex(acc, "\n")
	var openerIndent string
	if lastNL >= 0 {
		openerIndent = leadingTabs(acc[lastNL+1:])
	}
	contentIndent := openerIndent + "\t"
	b.WriteString("{{\n")
	for _, line := range strings.Split(strings.TrimRight(v.Expr, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteByte('\n')
		} else {
			// Strip leading spaces (not tabs) before prepending contentIndent.
			// The template {{ syntax can leave a spurious space before the
			// first token (e.g. "{{ def …" yields " def …" as the expression).
			// TrimLeft " " removes only spaces, preserving tab-based relative
			// indentation within the expression.
			b.WriteString(contentIndent)
			b.WriteString(strings.TrimRight(strings.TrimLeft(line, " "), " \t"))
			b.WriteByte('\n')
		}
	}
	b.WriteString(openerIndent)
	if v.EatEOL {
		b.WriteString("-}}")
	} else {
		b.WriteString("}}")
	}
}


// ── def-block helpers ─────────────────────────────────────────────────────────

// isDefLike reports whether expr is a def/include/import block.
// The jq-template.awk engine hoists these to the top of the generated program
// and appends ";" when assembling; they must not carry the ";" in template source.
// Leading whitespace is ignored; stripMinIndent only removes tabs, so non-tab
// leading whitespace (e.g. a space from "{{ def …") may survive.
func isDefLike(expr string) bool {
	t := strings.TrimSpace(expr)
	return strings.HasPrefix(t, "def ") || strings.HasPrefix(t, "def\t") ||
		strings.HasPrefix(t, "include ") || strings.HasPrefix(t, "import ")
}

// stripImpliedSemicolon removes the trailing ";\n" that FormatFile emits for
// def/include/import programs.  The template engine implies the semicolon; it
// must not appear in the template source.
func stripImpliedSemicolon(s string) string {
	s = strings.TrimRight(s, " \t\n")
	if strings.HasSuffix(s, ";") {
		s = strings.TrimRight(s[:len(s)-1], " \t")
	}
	return s + "\n"
}

// ── assembly helpers ──────────────────────────────────────────────────────────

// assemblySentinel returns the sentinel string to insert between expressions i
// and i+1 during whole-program assembly.
//
// When expression i ends with "(" and expression i+1 starts with ")", jq
// forbids an empty-paren body (jq does not allow "()" as an expression).  In
// that case a string-literal sentinel "SENTINEL_N" is used to fill the paren.
// In all other positions a comment sentinel # SENTINEL_N is used.
func assemblySentinel(n int, exprBefore, exprAfter string) string {
	lastBefore := strings.TrimRight(strings.TrimSpace(exprBefore), " \t")
	firstAfter := strings.TrimLeft(strings.TrimSpace(exprAfter), " \t\n")
	if strings.HasSuffix(lastBefore, "(") && strings.HasPrefix(firstAfter, ")") {
		return "\"__TIANONFMT_" + strconv.Itoa(n) + "__\""
	}
	return "# __TIANONFMT_" + strconv.Itoa(n) + "__"
}

// assemblySentinelRE matches any line consisting solely of a sentinel marker
// (comment form "# __TIANONFMT_N__" or string form "\"__TIANONFMT_N__\"").
var assemblySentinelRE = regexp.MustCompile(`(?m)^\s*(?:# __TIANONFMT_\d+__|"__TIANONFMT_\d+__")\s*$`)

// trimPiece strips leading and trailing blank lines while preserving internal
// whitespace and indentation.
func trimPiece(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// bracketDelta returns the net change in bracket depth for expr, counting
// unquoted, non-commented (, [, { as +1 and ), ], } as -1.
func bracketDelta(expr string) int {
	depth := 0
	inString := false
	inComment := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if inComment {
			if c == '\n' {
				inComment = false
			}
			continue
		}
		if inString {
			if c == '\\' {
				i++
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '#':
			inComment = true
		case '"':
			inString = true
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
	}
	return depth
}

// ── utility ───────────────────────────────────────────────────────────────────

// isInlineContext returns true if the last character before the current
// position in accumulated output is non-whitespace (meaning we're mid-line).
func isInlineContext(acc string) bool {
	lastNL := strings.LastIndex(acc, "\n")
	var linesSoFar string
	if lastNL < 0 {
		linesSoFar = acc
	} else {
		linesSoFar = acc[lastNL+1:]
	}
	return strings.TrimSpace(linesSoFar) != ""
}

// isPureComment reports whether expr consists entirely of jq comment lines
// (every non-empty line begins with #).
func isPureComment(expr string) bool {
	hasComment := false
	for _, line := range strings.Split(expr, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			if !strings.HasPrefix(t, "#") {
				return false
			}
			hasComment = true
		}
	}
	return hasComment
}

// stripMinIndent normalises the indentation of a multi-line jq expression
// extracted from a {{ }} block.  For single-line expressions it behaves like
// strings.TrimSpace.  For multi-line expressions it:
//
//  1. Strips surrounding blank lines (the outer \n wrappers).
//  2. Finds the minimum leading-tab count across all non-blank lines.
//  3. Strips exactly that many tabs from the start of every non-blank line.
//
// This preserves relative indentation between lines while establishing a
// consistent base (minimum indent = 0 tabs).
func stripMinIndent(s string) string {
	s = strings.Trim(s, "\n")
	if !strings.Contains(s, "\n") {
		return strings.TrimSpace(s)
	}
	lines := strings.Split(s, "\n")
	minTabs := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		tabs := 0
		for _, c := range line {
			if c == '\t' {
				tabs++
			} else {
				break
			}
		}
		if minTabs < 0 || tabs < minTabs {
			minTabs = tabs
		}
	}
	if minTabs <= 0 {
		return s
	}
	prefix := strings.Repeat("\t", minTabs)
	result := make([]string, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			result[i] = ""
		} else {
			result[i] = strings.TrimPrefix(line, prefix)
		}
	}
	return strings.Join(result, "\n")
}

// leadingTabs returns the leading tab characters of s.
func leadingTabs(s string) string {
	i := 0
	for i < len(s) && s[i] == '\t' {
		i++
	}
	return s[:i]
}

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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/values-conflict/tianonfmt/jq"
)

// Segment is one piece of a template file.
type Segment interface{ templateSeg() }

// TextSeg is verbatim Dockerfile content between {{ }} blocks.
type TextSeg struct{ Text string }

// JQSeg is a {{ jq_expression }} block.
type JQSeg struct {
	Expr   string // raw jq expression text (trimmed)
	EatEOL bool   // true when the closing marker is -}} (strips trailing newline)
	Line   int    // 1-based source line of the opening {{ in the template file
}

func (TextSeg) templateSeg() {}
func (JQSeg) templateSeg()   {}

// Parse splits a template source into segments.
// Returns an error if the source contains an unterminated {{ block.
func Parse(src string) ([]Segment, error) {
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
			return segs, nil
		}

		// Position of this {{ in the full source (for error reporting).
		openPos := len(src) - len(remaining) + start
		openLine := strings.Count(src[:openPos], "\n") + 1

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
					segs = append(segs, JQSeg{Expr: expr, EatEOL: true, Line: openLine})
					remaining = remaining[pos+len(closeEat):]
				} else {
					pos += len(closeEat)
				}
			case 3:
				depth--
				if depth == 0 {
					expr := stripMinIndent(remaining[:pos])
					segs = append(segs, JQSeg{Expr: expr, EatEOL: false, Line: openLine})
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
			return nil, fmt.Errorf("template: unterminated {{ block at line %d", openLine)
		}
	}
}

// Format reformats a jq-template file.  jq expressions inside {{ }} blocks
// are formatted using the jq formatter.  Non-def, non-comment block
// expressions are assembled into groups by balanced bracket depth and
// formatted together so that fragments like "{{ ) else ( -}}" receive
// proper context-aware indentation.
func Format(src string) (string, error) {
	segs, err := Parse(src)
	if err != nil {
		return "", err
	}
	if len(segs) == 0 {
		return src, nil
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

	type blockInfo struct {
		idx        int
		line       int // source line of the {{ opener
		expr       string
		isFragment bool // false = standalone (already formatted), true = needs assembly
	}
	var curGroup []blockInfo
	groupDepth := 0

	flushGroup := func() error {
		defer func() { curGroup = curGroup[:0]; groupDepth = 0 }()

		// Collect the fragments (standalone blocks were formatted on entry).
		var frags []blockInfo
		for _, b := range curGroup {
			if b.isFragment {
				frags = append(frags, b)
			}
		}
		if len(frags) < 2 {
			// 0 or 1 fragments: nothing to assemble (single fragment stays
			// verbatim; standalone blocks were already handled above).
			return nil
		}

		// Assemble only the fragment blocks — standalone blocks inside the
		// group are semantically independent and their presence between
		// fragments would produce invalid jq if included.
		exprs := make([]string, len(frags))
		for j, b := range frags {
			exprs[j] = strings.TrimSpace(b.expr)
		}
		// Compute running bracket depth after each fragment.
		depths := make([]int, len(frags))
		d := 0
		for j := range frags {
			d += bracketDelta(exprs[j])
			depths[j] = d
		}
		var parts []string
		sents := make([]string, len(exprs)-1)
		for j, e := range exprs {
			parts = append(parts, e)
			if j < len(exprs)-1 {
				prevPrev := 0
				if j > 0 {
					prevPrev = depths[j-1]
				}
				sents[j] = assemblySentinelDepth(j, exprs[j], exprs[j+1], depths[j], prevPrev)
				// Concatenation-based sentinels ("+ SENTINEL") need a trailing "+"
				// so the following expression is also concatenation-connected.
				if strings.HasPrefix(sents[j], "+ ") {
					parts = append(parts, sents[j]+" +")
				} else {
					parts = append(parts, sents[j])
				}
			}
		}
		assembled := strings.Join(parts, "\n")
		result, err := jq.FormatStr(assembled, false)
		if err != nil {
			return fmt.Errorf("assembled block starting at line %d: %w", frags[0].line, err)
		}
		pieces := splitAssembled(result, sents)
		if len(pieces) != len(frags) {
			panic("template: sentinel mismatch after assembly formatting — formatter bug (assembled: " + assembled + ")")
		}
		for j, b := range frags {
			p := trimPiece(pieces[j])
			if p == "" {
				continue
			}
			// Prefer re-formatting the piece individually: assembly pieces
			// can have sentinel-induced comments that force pipelines
			// multi-line even when the block is short and should be inline.
			stripped := strings.TrimSpace(p)
			ind, indErr := jq.FormatStr(stripped, false)
			if indErr == nil {
				formattedFor[b.idx] = ind
			} else {
				// Fragment piece: cannot be formatted standalone; use assembly result.
				formattedFor[b.idx] = p
			}
		}
		return nil
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
		// Try standalone formatting first.  Valid jq expressions always
		// have balanced brackets (delta == 0), so a standalone block never
		// changes the open-bracket depth of the surrounding group.
		result, standErr := jq.FormatStr(jqSeg.Expr, false)
		if standErr == nil {
			formattedFor[i] = result
			curGroup = append(curGroup, blockInfo{idx: i, line: jqSeg.Line, expr: jqSeg.Expr, isFragment: false})
			// depth unchanged (standalone blocks have bracketDelta == 0)
		} else {
			curGroup = append(curGroup, blockInfo{idx: i, line: jqSeg.Line, expr: jqSeg.Expr, isFragment: true})
			groupDepth += bracketDelta(jqSeg.Expr)
			if groupDepth <= 0 {
				if err := flushGroup(); err != nil {
					return "", err
				}
			}
		}
	}
	if err := flushGroup(); err != nil {
		return "", err
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
				if err := writeDefBlock(&b, v); err != nil {
					return "", err
				}
			} else if cl.inline {
				if err := writeInlineBlock(&b, v); err != nil {
					return "", err
				}
			} else {
				writeBlockExpr(&b, v, formattedFor[i])
			}
		}
	}
	return b.String(), nil
}

// IsTemplate returns true if src looks like a jq-template file (contains {{ }}).
func IsTemplate(src string) bool {
	return strings.Contains(src, "{{") && strings.Contains(src, "}}")
}

// ── emit helpers ─────────────────────────────────────────────────────────────

// writeComment emits a pure-comment JQSeg.
func writeComment(b *strings.Builder, v JQSeg) {
	var parts []string
	for line := range strings.SplitSeq(v.Expr, "\n") {
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
func writeDefBlock(b *strings.Builder, v JQSeg) error {
	expr := strings.TrimSpace(v.Expr)
	wrapErr := func(err error) error {
		return fmt.Errorf("def block at line %d: %w", v.Line, err)
	}
	// Try as-is first (expr may already carry a ";").
	result, err := jq.FormatStr(expr, false)
	if err != nil {
		// Try with template-implied semicolon.
		result2, err2 := jq.FormatStr(expr+"\n;", false)
		if err2 != nil {
			return wrapErr(err2)
		}
		stripped := stripImpliedSemicolon(result2)
		// Stability check: only accept the result if re-formatting converges.
		r2, err3 := jq.FormatStr(strings.TrimSpace(stripped)+"\n;", false)
		if err3 != nil {
			return wrapErr(err3)
		}
		if stripImpliedSemicolon(r2) != stripped {
			return fmt.Errorf("template: def block at line %d: formatter not idempotent for %q", v.Line, expr)
		}
		result = stripped
	}
	writeBlockExpr(b, v, result)
	return nil
}

// writeInlineBlock formats and emits an inline JQSeg (embedded in a text line).
func writeInlineBlock(b *strings.Builder, v JQSeg) error {
	closer := " }}"
	if v.EatEOL {
		closer = " -}}"
	}
	wrapErr := func(err error) error {
		return fmt.Errorf("inline block at line %d: %w", v.Line, err)
	}

	formatted := v.Expr
	fmtOK := false
	if v.Expr != "" {
		result, err := jq.FormatStr(v.Expr, true)
		if err != nil {
			return wrapErr(err)
		}
		formatted = result
		fmtOK = true
	}

	// jq '#' comments are newline-terminated; an inline block containing one
	// would swallow all text up to end-of-string.  Force multi-line layout.
	if fmtOK && strings.Contains(formatted, "#") {
		result, err := jq.FormatStr(v.Expr, false)
		if err != nil {
			return wrapErr(err)
		}
		writeBlockExpr(b, v, result)
		return nil
	}

	if !fmtOK || !strings.Contains(formatted, "\n") {
		b.WriteString("{{ ")
		b.WriteString(strings.TrimSpace(formatted))
		b.WriteString(closer)
		return nil
	}

	// Multi-line formatted result.
	writeBlockExpr(b, v, formatted)
	return nil
}

// writeBlockExpr emits a non-inline, non-def, non-comment JQSeg.
// formatted must be non-empty; passing "" panics to surface bugs immediately.
func writeBlockExpr(b *strings.Builder, v JQSeg, formatted string) {
	closer := " }}"
	if v.EatEOL {
		closer = " -}}"
	}

	if formatted == "" {
		// This should never happen for valid templates: all fragment blocks
		// are assembled and formatted, and standalone blocks are formatted
		// individually.  Panic loudly so regressions are caught immediately
		// rather than silently passing through malformed output.
		panic("template: writeBlockExpr called with empty formatted — this is a bug (expr: " + v.Expr + ")")
	}

	// Single-line: no embedded newlines after trimming depth-context tabs.
	// TrimSpace removes leading tabs that arrive from whole-program assembly
	// pieces — they carry no meaning in the emitted {{ expr }} form.
	contentTrimmed := strings.TrimSpace(formatted)
	if !strings.Contains(contentTrimmed, "\n") {
		b.WriteString("{{ ")
		b.WriteString(contentTrimmed)
		b.WriteString(closer)
		return
	}

	// Multi-line formatted content.
	// When the piece came from whole-program assembly (depth > 0) it already
	// carries correct leading indentation — emit as-is.
	// When the piece is at depth 0 (standalone expression or def) the jq
	// formatter produces no leading tabs — add one level.
	lines := strings.Split(strings.TrimRight(formatted, "\n"), "\n")
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

// assemblySentinelDepth returns the sentinel string to insert between
// expression i and i+1 during whole-program assembly.
// depthAfterBefore is the running bracket depth after exprBefore.
// depthAfterPrev is the running depth after the fragment before exprBefore
// (-1 or 0 if exprBefore is the first fragment).
//
// When fragment i completed a sub-expression (depthAfterBefore < depthAfterPrev)
// AND fragment i+1 starts a new sub-expression at the same level, two jq
// expressions are adjacent inside the same bracket context.  The jq template
// engine connects them with a pipe operator; we use "| SENTINEL" so the
// assembled program remains valid jq.  The caller appends " |" to connect the
// sentinel to the following fragment.
func assemblySentinelDepth(n int, exprBefore, exprAfter string, depthAfterBefore, depthAfterPrev int) string {
	lastBefore := strings.TrimRight(strings.TrimSpace(exprBefore), " \t")
	firstAfter := strings.TrimLeft(strings.TrimSpace(exprAfter), " \t\n")

	if strings.HasSuffix(lastBefore, "(") && strings.HasPrefix(firstAfter, ")") {
		// Empty paren: string literal fills the required expression.
		return "\"__TIANONFMT_" + strconv.Itoa(n) + "__\""
	}

	if depthAfterPrev >= 0 &&
		depthAfterBefore < depthAfterPrev && // exprBefore decreased depth (completed sub-expr)
		depthAfterBefore > 0 && // still inside an outer bracket (not group close)
		!strings.HasPrefix(firstAfter, ")") { // exprAfter starts a new sub-expression
		// Two adjacent sub-expressions at the same bracket depth need a
		// string-concatenation connector, matching jq-template.awk's append()
		// which uses "\n+ " between such expressions.  The assembled jq reads:
		//   sub_expr_a + "SENTINEL" + sub_expr_b
		return "+ \"__TIANONFMT_" + strconv.Itoa(n) + "__\""
	}

	return "# __TIANONFMT_" + strconv.Itoa(n) + "__"
}

// assemblySentinelRE matches any line consisting solely of a sentinel marker:
//
//	comment form:           # __TIANONFMT_N__
//	string form:            "__TIANONFMT_N__"
//	concat-string form:     + "__TIANONFMT_N__"
//
// The concat form appears when two adjacent sub-expressions are connected by
// the string-concatenation operator (+), mirroring jq-template.awk's append().
var assemblySentinelRE = regexp.MustCompile(`(?m)^\s*(?:# __TIANONFMT_\d+__|"__TIANONFMT_\d+__"|\+\s*"__TIANONFMT_\d+__")\s*$`)

// splitAssembled splits a formatted jq program back into per-block pieces using
// the sentinel markers that were inserted between blocks.
//
// Comment sentinels ("# __TIANONFMT_N__") always appear on their own line.
// String sentinels ("__TIANONFMT_N__") may appear inline when the jq formatter
// applies the cuddled short-else form (e.g. `) else ("SENTINEL") end`).
// For inline string sentinels the "(" before the sentinel ends the current
// piece and the ")" after it starts the following piece.
func splitAssembled(formatted string, sents []string) []string {
	pieces := make([]string, len(sents)+1)
	remaining := formatted

	for i, sent := range sents {
		isConcat := strings.HasPrefix(sent, "+ ")
		if strings.HasPrefix(sent, "#") || isConcat {
			// Comment sentinel or concat-string sentinel: split at the sentinel line.
			// For concat sentinels, the stored sentinel is "+ SENTINEL" but the
			// assembly appended " +" making it "+ SENTINEL +"; the formatted output
			// puts it as "+ SENTINEL" on its own line.
			marker := strings.TrimSpace(sent)
			if isConcat {
				// Strip trailing " +" if present (the caller appended it).
				marker = strings.TrimRight(marker, " +")
				marker = strings.TrimSpace(marker)
			}
			lines := strings.Split(remaining, "\n")
			found := -1
			for j, line := range lines {
				lTrimmed := strings.TrimSpace(line)
				if lTrimmed == marker ||
					(isConcat && strings.HasPrefix(lTrimmed, "+ ") && strings.TrimSpace(lTrimmed[2:]) == strings.TrimPrefix(marker, "+ ")) {
					found = j
					break
				}
			}
			if found < 0 {
				return nil
			}
			pieces[i] = trimPiece(strings.Join(lines[:found], "\n"))
			next := lines[found+1:]
			if isConcat {
				// After a concat-based sentinel the jq formatter puts "+ EXPR" on
				// the next content line.  Strip the leading "+ " so the piece
				// contains just the expression, not the concatenation operator.
				for k, line := range next {
					if strings.TrimSpace(line) != "" {
						idx := strings.Index(line, "+")
						if idx >= 0 && strings.TrimSpace(line[:idx]) == "" {
							// Leading whitespace then "+": strip the "+ "
							after := line[idx+1:]
							if strings.HasPrefix(after, " ") {
								after = after[1:]
							}
							next[k] = line[:idx] + after
						}
						break
					}
				}
			}
			remaining = strings.Join(next, "\n")
		} else {
			// String sentinel: positional search handles both own-line and inline.
			idx := strings.Index(remaining, sent)
			if idx < 0 {
				return nil
			}
			before := remaining[:idx]
			lastParen := strings.LastIndex(before, "(")
			if lastParen < 0 {
				return nil
			}
			pieces[i] = trimPiece(before[:lastParen+1])

			after := remaining[idx+len(sent):]
			closeParen := strings.Index(after, ")")
			if closeParen < 0 {
				return nil
			}
			lineStart := strings.LastIndex(after[:closeParen], "\n")
			if lineStart >= 0 {
				remaining = after[lineStart+1:]
			} else {
				remaining = after[closeParen:]
			}
		}
	}
	pieces[len(sents)] = trimPiece(remaining)
	return pieces
}

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
	for line := range strings.SplitSeq(expr, "\n") {
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

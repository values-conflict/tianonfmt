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
					expr := strings.TrimSpace(remaining[:pos])
					segs = append(segs, JQSeg{Expr: expr, EatEOL: true})
					remaining = remaining[pos+len(closeEat):]
				} else {
					pos += len(closeEat)
				}
			case 3:
				depth--
				if depth == 0 {
					expr := strings.TrimSpace(remaining[:pos])
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
// It receives the raw expression text and returns the formatted version.
// If jqFmt returns an empty string (parse error), the expression is left as-is.
//
// isInline is true when the {{ }} block is embedded within a larger text line
// (i.e., the surrounding text on the same line is non-empty).  The caller
// should format inline blocks compactly (no newlines).
//
// Auto-detection of inline vs block: a block is "inline" when the text
// immediately preceding the {{ on the same line contains non-whitespace.
func Format(src string, jqFmt func(expr string, inline bool) string) string {
	segs := Parse(src)
	if len(segs) == 0 {
		return src
	}

	var b strings.Builder

	for _, seg := range segs {
		switch v := seg.(type) {
		case TextSeg:
			b.WriteString(v.Text)

		case JQSeg:
			// Determine inline vs block context.
			// A block is inline if, looking back at the accumulated output,
			// the last character before {{ on the current line is non-whitespace.
			inline := isInlineContext(b.String())

			// Pure comment blocks (every non-empty line is a # comment).
			// Single comment line → collapsed inline: {{ # foo -}}
			// Multiple comment lines → multi-line with one tab per line:
			//   {{
			//   \t# foo
			//   \t# bar
			//   -}}
			if isPureComment(v.Expr) {
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
				} else {
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
				break
			}

			// Format the expression.
			fmtOK := false
			formatted := v.Expr
			if jqFmt != nil && v.Expr != "" {
				if result := jqFmt(v.Expr, inline); result != "" {
					formatted = result
					fmtOK = true
				}
			}

			// jq '#' comments are newline-terminated: embedding one in an inline
			// {{ expr }} block causes it to swallow subsequent tokens up to end of
			// string, breaking the expression.  Force multi-line layout instead.
			if fmtOK && inline && strings.Contains(formatted, "#") {
				if result := jqFmt(v.Expr, false); result != "" {
					formatted = result
					inline = false
				} else {
					// Can't reformat multi-line; emit verbatim to preserve structure.
					fmtOK = false
					formatted = v.Expr
				}
			}

			// Emit the block.
			if inline || !strings.Contains(formatted, "\n") {
				// Inline or single-line: {{ expr }} on same line.
				b.WriteString("{{ ")
				b.WriteString(strings.TrimSpace(formatted))
				if v.EatEOL {
					b.WriteString(" -}}")
				} else {
					b.WriteString(" }}")
				}
			} else if !fmtOK {
				// jqFmt couldn't parse the expression — emit verbatim so the
				// block is preserved exactly and re-formatting is idempotent.
				b.WriteString("{{")
				b.WriteString(v.Expr)
				if v.EatEOL {
					b.WriteString("-}}")
				} else {
					b.WriteString("}}")
				}
			} else {
				// Multi-line block with a successfully formatted expression.
				// The content is indented one level deeper than the {{ opener;
				// the closing }} sits at the same level as the opener.
				acc := b.String()
				lastNL := strings.LastIndex(acc, "\n")
				var openerIndent string
				if lastNL >= 0 {
					openerIndent = leadingTabs(acc[lastNL+1:])
				}
				contentIndent := openerIndent + "\t"
				b.WriteString("{{\n")
				for _, line := range strings.Split(strings.TrimRight(formatted, "\n"), "\n") {
					if strings.TrimSpace(line) == "" {
						b.WriteByte('\n')
					} else {
						b.WriteString(contentIndent)
						b.WriteString(line)
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
		}
	}

	return b.String()
}

// IsTemplate returns true if src looks like a jq-template file (contains {{ }}).
func IsTemplate(src string) bool {
	return strings.Contains(src, "{{") && strings.Contains(src, "}}")
}

// isInlineContext returns true if the last character before the current
// position in accumulated output is non-whitespace (meaning we're mid-line).
func isInlineContext(acc string) bool {
	// Find the last newline in acc.
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
// (every non-empty line begins with #).  A block that merely starts with a
// comment but also contains expression content returns false.
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

// leadingTabs returns the leading tab characters of s.
func leadingTabs(s string) string {
	i := 0
	for i < len(s) && s[i] == '\t' {
		i++
	}
	return s[:i]
}

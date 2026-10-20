// reformat-md-2sp rewrites markdown files in-place to use the double-space-
// after-sentence convention and to ensure consistent blank-line structure
// around block elements.  It replaces the reformat-md-2sp.pl Perl script with
// an AST-aware implementation that avoids the oscillation issues of the regex-
// based approach.
//
// Rules applied:
//   - Wrapped prose paragraphs are joined into a single long line.
//   - Sentences are separated by exactly two spaces after . ! ?
//   - A paragraph that starts with **Bold:** (or **`Code`:** etc.) is treated
//     as a new topic and separated from the preceding paragraph with a blank.
//   - Headings, blockquotes, list blocks, code fences, and --- each get a
//     blank line before and after them (except at start/end of file, and
//     adjacent same-type blocks are not further separated).
//   - Runs of 2+ blank lines are collapsed to one.
//
// Usage: reformat-md-2sp FILE [FILE ...]
package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// abbrevs lists lowercase tokens that end with a period but are not sentence
// terminators.
var abbrevs = map[string]bool{
	"e.g.": true, "i.e.": true, "etc.": true, "cf.": true, "vs.": true,
	"viz.": true, "mr.": true, "mrs.": true, "ms.": true, "dr.": true,
	"st.": true, "jr.": true, "sr.": true, "no.": true, "vol.": true,
	"ch.": true, "fig.": true, "eq.": true, "ref.": true, "sec.": true,
	"approx.": true, "inc.": true, "co.": true, "ltd.": true, "corp.": true,
	"a.m.": true, "p.m.": true, "rfc.": true,
}

// sentenceSpacing normalizes the whitespace between sentences in a single-line
// paragraph to exactly two spaces after each sentence terminator.
func sentenceSpacing(s string) string {
	runes := []rune(s)
	n := len(runes)
	var buf strings.Builder
	buf.Grow(len(s) + 16)

	inCode := false
	codeLen := 0
	inLink := false
	bracketDepth := 0
	parenDepth := 0

	i := 0
	for i < n {
		ch := runes[i]

		// Track backtick code spans.
		if ch == '`' && !inLink {
			if !inCode {
				j := i
				for j < n && runes[j] == '`' {
					j++
				}
				codeLen = j - i
				inCode = true
				for k := i; k < j; k++ {
					buf.WriteRune(runes[k])
				}
				i = j
				continue
			}
			// Closing: count matching backticks.
			j := i
			for j < n && runes[j] == '`' {
				j++
			}
			if j-i == codeLen {
				inCode = false
				codeLen = 0
			}
			for k := i; k < j; k++ {
				buf.WriteRune(runes[k])
			}
			i = j
			continue
		}
		if inCode {
			buf.WriteRune(ch)
			i++
			continue
		}

		// Track link text/URL spans to avoid splitting inside URLs.
		if ch == '[' {
			bracketDepth++
			buf.WriteRune(ch)
			i++
			continue
		}
		if ch == ']' && bracketDepth > 0 {
			bracketDepth--
			buf.WriteRune(ch)
			i++
			if bracketDepth == 0 && i < n && runes[i] == '(' {
				inLink = true
				parenDepth = 1
				buf.WriteRune(runes[i])
				i++
			}
			continue
		}
		if inLink {
			if ch == '(' {
				parenDepth++
			} else if ch == ')' {
				parenDepth--
				if parenDepth == 0 {
					inLink = false
				}
			}
			buf.WriteRune(ch)
			i++
			continue
		}

		// Sentence terminator detection.
		isTerm := ch == '.' || ch == '!' || ch == '?'
		if isTerm {
			// Collect any trailing quotes/brackets.
			j := i + 1
			for j < n && (runes[j] == '"' || runes[j] == '\'' || runes[j] == ')' || runes[j] == ']') {
				j++
			}
			atEnd := j >= n
			nextIsSpace := j < n && runes[j] == ' '

			if !atEnd && !nextIsSpace {
				// Not a sentence boundary.
				buf.WriteRune(ch)
				i++
				continue
			}

			if ch == '.' {
				// Check for abbreviation: extract the word ending in this period.
				k := i
				for k > 0 && runes[k-1] != ' ' {
					k--
				}
				word := strings.ToLower(string(runes[k : i+1]))
				// Single-letter initial (e.g. "U.") -- not a sentence end if
				// preceded by another single-letter + period (initialism).
				isSingleInitial := len(word) == 2 && unicode.IsLetter(runes[k])
				if abbrevs[word] || isSingleInitial {
					buf.WriteRune(ch)
					i++
					continue
				}
				// Multi-period token (e.g. "v1.2.3") -- check for prior period
				// in the word, but strip inline code spans first (a period inside
				// `go.mod` or `v1.2.3` should not suppress double-spacing of the
				// sentence-ending period AFTER the code span).
				wordBody := string(runes[k:i])
				plain := wordBody
				// Strip `code` spans.
				for {
					start := strings.IndexByte(plain, '`')
					if start < 0 {
						break
					}
					end := strings.IndexByte(plain[start+1:], '`')
					if end < 0 {
						break
					}
					plain = plain[:start] + plain[start+1+end+1:]
				}
				// Strip [link text] portions.
				for {
					start := strings.IndexByte(plain, '[')
					if start < 0 {
						break
					}
					end := strings.IndexByte(plain[start:], ']')
					if end < 0 {
						break
					}
					plain = plain[:start] + plain[start+end+1:]
				}
				if strings.ContainsRune(plain, '.') {
					buf.WriteRune(ch)
					i++
					continue
				}
			}

			// Emit the terminator and any trailing quotes.
			for k := i; k < j; k++ {
				buf.WriteRune(runes[k])
			}
			i = j

			// Consume existing whitespace, then emit exactly two spaces.
			for i < n && runes[i] == ' ' {
				i++
			}
			if i < n {
				buf.WriteString("  ")
			}
			continue
		}

		buf.WriteRune(ch)
		i++
	}
	return buf.String()
}

// isBoldLabelParagraph reports whether a paragraph (as a single joined line)
// starts with a bold topic label: **Label:** or **`code`:** etc.
func isBoldLabelParagraph(s string) bool {
	if !strings.HasPrefix(s, "**") {
		return false
	}
	// Find the closing **
	rest := s[2:]
	end := strings.Index(rest, "**")
	if end < 2 {
		return false
	}
	after := rest[end+2:]
	return strings.HasPrefix(after, ":") || strings.HasPrefix(after, " ")
}

// splitBoldTopics splits a long paragraph at bold-topic boundaries of the
// form `sentence-end.  **Capital` and returns the resulting sub-paragraphs.
// Only splits when the line is longer than splitThresh characters.
const splitThresh = 400

func splitBoldTopics(s string) []string {
	if len(s) < splitThresh {
		return []string{s}
	}
	runes := []rune(s)
	n := len(runes)
	var result []string
	start := 0
	for i := 0; i < n-3; i++ {
		ch := runes[i]
		// Look for: [.!?)\]] followed by 1-3 spaces then ** then Capital/backtick/[
		isTerm := ch == '.' || ch == '!' || ch == '?' || ch == ')' || ch == ']'
		if !isTerm {
			continue
		}
		j := i + 1
		for j < n && (runes[j] == '"' || runes[j] == '\'' || runes[j] == ')' || runes[j] == ']') {
			j++
		}
		// 1-3 spaces
		sp := j
		for sp < n && runes[sp] == ' ' && sp-j < 3 {
			sp++
		}
		if sp == j || sp >= n-1 {
			continue
		}
		if runes[sp] != '*' || runes[sp+1] != '*' {
			continue
		}
		if sp+2 >= n {
			continue
		}
		first := runes[sp+2]
		if !unicode.IsUpper(first) && first != '`' && first != '[' && first != '-' && first != '_' && first != '(' {
			continue
		}
		// Split here.
		chunk := strings.TrimSpace(string(runes[start:j]))
		if chunk != "" {
			result = append(result, chunk)
		}
		start = sp
		i = sp
	}
	tail := strings.TrimSpace(string(runes[start:]))
	if tail != "" {
		result = append(result, tail)
	}
	if len(result) == 0 {
		return []string{s}
	}
	return result
}

// blockKind classifies a non-blank output line for blank-line insertion rules.
func blockKind(line string) string {
	if strings.HasPrefix(line, "#") {
		return "heading"
	}
	if strings.HasPrefix(line, ">") {
		return "blockquote"
	}
	if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "***") || strings.HasPrefix(line, "___") {
		if strings.TrimRight(line, "-*_ \t") == "" {
			return "hr"
		}
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return "list"
	}
	if len(line) > 1 && (line[0] >= '0' && line[0] <= '9') {
		if idx := strings.IndexByte(line, '.'); idx > 0 && idx < 4 && idx+1 < len(line) && line[idx+1] == ' ' {
			return "list"
		}
	}
	if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
		return "fence"
	}
	if isBoldLabelParagraph(line) {
		return "bold-label"
	}
	return "prose"
}

// Formatter holds the state for a single reformat pass.
type Formatter struct {
	src    []byte
	output []string // output lines (before blank-line insertion)
}

// extractLines returns the raw source lines for a node that has line segments.
func (f *Formatter) extractLines(node ast.Node) []string {
	var lines []string
	segs := node.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		line := strings.TrimRight(string(f.src[seg.Start:seg.Stop]), "\n\r")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// emitParagraph joins raw lines, normalizes spacing, handles bold-label splits.
// Note: letter-ordinal list items (a. text, b. text, etc.) are not supported
// by CommonMark or GitHub Flavored Markdown -- goldmark parses them as plain
// paragraph text.  They will be joined here, making them visible in diffs and
// prompting manual conversion to numbered lists (1. 2. 3.).
func (f *Formatter) emitParagraph(node ast.Node) {
	rawLines := f.extractLines(node)
	if len(rawLines) == 0 {
		return
	}
	joined := strings.Join(rawLines, " ")
	// Collapse internal whitespace runs to single spaces before spacing normalization.
	var buf strings.Builder
	prevSpace := false
	for _, r := range joined {
		if unicode.IsSpace(r) {
			if !prevSpace {
				buf.WriteRune(' ')
			}
			prevSpace = true
		} else {
			buf.WriteRune(r)
			prevSpace = false
		}
	}
	joined = strings.TrimSpace(buf.String())
	normalized := sentenceSpacing(joined)
	parts := splitBoldTopics(normalized)
	f.output = append(f.output, parts...)
}

// emitVerbatim appends all raw source lines for a node without modification.
func (f *Formatter) emitVerbatim(node ast.Node) {
	segs := node.Lines()
	var lines []string
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		line := strings.TrimRight(string(f.src[seg.Start:seg.Stop]), "\n\r")
		lines = append(lines, line)
	}
	f.output = append(f.output, strings.Join(lines, "\n"))
}

// emitNode dispatches on node kind.
func (f *Formatter) emitNode(node ast.Node) {
	switch node.Kind() {
	case ast.KindDocument:
		// Emit a blank entry between every pair of top-level block nodes so
		// that all blocks are separated in the output.  Consecutive list items
		// within a single List node are handled by emitList without extra blanks
		// (tight lists), but between separate top-level nodes we always want one.
		first := true
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if !first {
				f.output = append(f.output, "")
			}
			f.emitNode(child)
			first = false
		}
	case ast.KindParagraph:
		f.emitParagraph(node)
	case ast.KindHeading:
		h := node.(*ast.Heading)
		prefix := strings.Repeat("#", h.Level) + " "
		// Headings have no Lines(); extract text from children.
		var text strings.Builder
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			f.extractInlineText(child, &text)
		}
		f.output = append(f.output, prefix+strings.TrimSpace(text.String()))
	case ast.KindFencedCodeBlock:
		f.emitFencedCode(node.(*ast.FencedCodeBlock))
	case ast.KindCodeBlock:
		// Indented code block: preserve with 4-space indent.
		segs := node.Lines()
		var lines []string
		for i := 0; i < segs.Len(); i++ {
			seg := segs.At(i)
			line := strings.TrimRight(string(f.src[seg.Start:seg.Stop]), "\n\r")
			lines = append(lines, "    "+line)
		}
		f.output = append(f.output, strings.Join(lines, "\n"))
	case ast.KindBlockquote:
		// CommonMark "lazy continuation": a paragraph following "> " lines with
		// no ">" prefix lands inside the BlockQuote node in the same Paragraph.
		// goldmark strips "> " from stored segments, so we detect real blockquote
		// lines by scanning backwards from seg.Start in the raw source to the
		// beginning of that line and checking for ">".
		// Strategy: walk every line segment of every child; group consecutive
		// same-type (bq vs lazy) lines; emit bq groups prefixed with "> " and
		// lazy groups as plain paragraphs.
		lineIsBQ := func(segStart int) bool {
			pos := segStart
			for pos > 0 && f.src[pos-1] != '\n' {
				pos--
			}
			return pos < len(f.src) && f.src[pos] == '>'
		}

		type lineGroup struct {
			isBQ  bool
			lines []string
		}
		var groups []lineGroup
		var cur lineGroup
		cur.isBQ = true // will be set on first line
		first := true

		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			segs := child.Lines()
			if segs == nil {
				// Non-line child (e.g. nested list) -- emit via inner formatter.
				inner := &Formatter{src: f.src}
				inner.emitNode(child)
				rendered := strings.TrimRight(inner.render(), "\n")
				for _, line := range strings.Split(rendered, "\n") {
					isBQ := true // nested blocks stay inside blockquote
					if first || isBQ != cur.isBQ {
						if !first {
							groups = append(groups, cur)
						}
						cur = lineGroup{isBQ: isBQ}
						first = false
					}
					cur.lines = append(cur.lines, line)
				}
				continue
			}
			for i := 0; i < segs.Len(); i++ {
				seg := segs.At(i)
				line := strings.TrimRight(string(f.src[seg.Start:seg.Stop]), "\n\r")
				isBQ := lineIsBQ(seg.Start)
				if first || isBQ != cur.isBQ {
					if !first {
						groups = append(groups, cur)
					}
					cur = lineGroup{isBQ: isBQ}
					first = false
				}
				cur.lines = append(cur.lines, line)
			}
		}
		if !first {
			groups = append(groups, cur)
		}

		for gi, g := range groups {
			if gi > 0 {
				f.output = append(f.output, "")
			}
			joined := strings.Join(g.lines, " ")
			normalized := sentenceSpacing(strings.TrimSpace(joined))
			parts := splitBoldTopics(normalized)
			if g.isBQ {
				for _, p := range parts {
					f.output = append(f.output, "> "+p)
				}
			} else {
				f.output = append(f.output, parts...)
			}
		}
	case ast.KindList:
		f.emitList(node.(*ast.List))
	case ast.KindThematicBreak:
		f.output = append(f.output, "---")
	case ast.KindHTMLBlock:
		f.emitVerbatim(node)
	case extast.KindTable:
		f.emitTable(node)
	default:
		// Unknown block node: emit verbatim if it has lines.
		if node.Lines() != nil && node.Lines().Len() > 0 {
			f.emitVerbatim(node)
		} else {
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				f.emitNode(child)
			}
		}
	}
}

// cellText reconstructs the inline markdown of a table cell, preserving
// code spans, emphasis, links, and plain text.
func (f *Formatter) cellText(cell ast.Node) string {
	var buf strings.Builder
	ast.Walk(cell, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch v := n.(type) {
		case *ast.Text:
			if entering {
				buf.Write(v.Segment.Value(f.src))
				if v.SoftLineBreak() || v.HardLineBreak() {
					buf.WriteByte(' ')
				}
			}
		case *ast.CodeSpan:
			if entering {
				buf.WriteByte('`')
				for c := n.FirstChild(); c != nil; c = c.NextSibling() {
					if t, ok := c.(*ast.Text); ok {
						buf.Write(t.Segment.Value(f.src))
					}
				}
				buf.WriteByte('`')
				return ast.WalkSkipChildren, nil
			}
		case *ast.Emphasis:
			buf.WriteString(strings.Repeat("*", v.Level))
		case *ast.Link:
			if entering {
				buf.WriteByte('[')
			} else {
				buf.WriteString("](")
				buf.Write(v.Destination)
				buf.WriteByte(')')
			}
		case *ast.String:
			if entering {
				buf.Write(v.Value)
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}

// emitTable reconstructs the markdown table from goldmark's parsed AST.
func (f *Formatter) emitTable(node ast.Node) {
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, f.cellText(cell))
		}
		f.output = append(f.output, "| "+strings.Join(cells, " | ")+" |")
		if row.Kind() == extast.KindTableHeader {
			sep := make([]string, len(cells))
			for i := range sep {
				sep[i] = "---"
			}
			f.output = append(f.output, "|"+strings.Join(sep, "|")+"|")
		}
	}
}

// emitFencedCode emits a fenced code block preserving the info string and content.
func (f *Formatter) emitFencedCode(node *ast.FencedCodeBlock) {
	fence := "```"
	if node.Info != nil {
		fence += string(node.Info.Segment.Value(f.src))
	}
	var lines []string
	lines = append(lines, fence)
	segs := node.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		line := strings.TrimRight(string(f.src[seg.Start:seg.Stop]), "\n\r")
		lines = append(lines, line)
	}
	lines = append(lines, "```")
	f.output = append(f.output, strings.Join(lines, "\n"))
}

// emitList emits a list, handling tight vs. loose and nested lists.
func (f *Formatter) emitList(list *ast.List) {
	loose := !list.IsTight
	idx := list.Start
	firstItem := true
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		// Loose lists had blank lines between items in the source; preserve them.
		if loose && !firstItem {
			f.output = append(f.output, "")
		}
		firstItem = false
		li := item.(*ast.ListItem)
		var marker string
		if list.IsOrdered() {
			marker = fmt.Sprintf("%d. ", idx)
			idx++
		} else {
			marker = string(list.Marker) + " "
		}
		indent := strings.Repeat(" ", len(marker))

		// Collect item content.  The first paragraph uses marker; subsequent
		// paragraphs (loose list items) use indent with a blank line before them.
		var parts []string
		firstPara := true
		for child := li.FirstChild(); child != nil; child = child.NextSibling() {
			if child.Kind() == ast.KindTextBlock || child.Kind() == ast.KindParagraph {
				rawLines := f.extractLines(child)
				if len(rawLines) == 0 {
					continue
				}
				joined := strings.Join(rawLines, " ")
				normalized := sentenceSpacing(strings.TrimSpace(joined))
				// Bold-topic splits within list items stay indented.
				topics := splitBoldTopics(normalized)
				for i, t := range topics {
					if i == 0 && firstPara {
						parts = append(parts, marker+t)
					} else {
						parts = append(parts, "")
						parts = append(parts, indent+t)
					}
				}
				firstPara = false
			} else {
				// Nested list or other block -- emit recursively, indented.
				inner := &Formatter{src: f.src}
				inner.emitNode(child)
				rendered := strings.TrimRight(inner.render(), "\n")
				if !firstPara {
					parts = append(parts, "")
				}
				for _, line := range strings.Split(rendered, "\n") {
					if line == "" {
						parts = append(parts, "")
					} else {
						parts = append(parts, indent+line)
					}
				}
				firstPara = false
			}
		}
		f.output = append(f.output, parts...)
	}
}

// extractInlineText recursively extracts plain text from inline nodes, preserving
// the raw markdown syntax (bold, italic, code, links).
func (f *Formatter) extractInlineText(node ast.Node, buf *strings.Builder) {
	switch n := node.(type) {
	case *ast.Text:
		buf.Write(n.Segment.Value(f.src))
		if n.SoftLineBreak() || n.HardLineBreak() {
			buf.WriteByte(' ')
		}
	case *ast.String:
		buf.Write(n.Value)
	case *ast.CodeSpan:
		buf.WriteByte('`')
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				buf.Write(t.Segment.Value(f.src))
			}
		}
		buf.WriteByte('`')
	case *ast.Emphasis:
		marker := strings.Repeat("*", n.Level)
		buf.WriteString(marker)
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			f.extractInlineText(child, buf)
		}
		buf.WriteString(marker)
	case *extast.Strikethrough:
		buf.WriteString("~~")
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			f.extractInlineText(child, buf)
		}
		buf.WriteString("~~")
	case *ast.Link:
		buf.WriteByte('[')
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			f.extractInlineText(child, buf)
		}
		buf.WriteString("](")
		buf.Write(n.Destination)
		buf.WriteByte(')')
	default:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			f.extractInlineText(child, buf)
		}
	}
}

// render applies blank-line insertion rules and returns the final formatted text.
func (f *Formatter) render() string {
	// Expand multi-line items (fenced code, blockquotes) into individual lines.
	var lines []string
	for _, item := range f.output {
		lines = append(lines, strings.Split(item, "\n")...)
	}

	// Insert blank lines around block elements and between bold-label paragraphs.
	// Lines inside fenced code blocks are passed through verbatim -- no blank-line
	// rules apply to them.
	var result []string
	inFence := false
	fenceChar := byte(0)
	fenceLen := 0

	for i, line := range lines {
		// Fence state tracking: detect opening and closing fences.
		// The fence may be indented (e.g. inside a list item continuation).
		trimmed := strings.TrimLeft(line, " \t")
		if !inFence {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = true
				fenceChar = trimmed[0]
				j := 0
				for j < len(trimmed) && trimmed[j] == fenceChar {
					j++
				}
				fenceLen = j
				// Blank before opening fence if prev is non-blank.
				prev := ""
				if len(result) > 0 {
					prev = result[len(result)-1]
				}
				if prev != "" {
					result = append(result, "")
				}
				result = append(result, line)
				continue
			}
		} else {
			result = append(result, line)
			// Check for closing fence: same char, length >= opening, no other content.
			if len(trimmed) >= fenceLen && trimmed[0] == fenceChar {
				allFence := true
				for k := 0; k < len(trimmed); k++ {
					if trimmed[k] != fenceChar {
						allFence = false
						break
					}
				}
				if allFence {
					inFence = false
					// Blank after closing fence if next is non-blank.
					j := i + 1
					for j < len(lines) && lines[j] == "" {
						j++
					}
					if j < len(lines) {
						result = append(result, "")
					}
				}
			}
			continue
		}

		if line == "" {
			result = append(result, "")
			continue
		}
		cur := blockKind(line)
		var prevNonBlank string
		for j := len(result) - 1; j >= 0; j-- {
			if result[j] != "" {
				prevNonBlank = blockKind(result[j])
				break
			}
		}
		prevIsBlank := len(result) > 0 && result[len(result)-1] == ""

		needBlankBefore := false
		if prevNonBlank != "" && !prevIsBlank {
			// Bold-label paragraphs always get a blank before them when preceded
			// by any non-blank content -- including another bold-label.  This is
			// the common case from splitBoldTopics emitting consecutive topics.
			if cur == "bold-label" {
				needBlankBefore = true
			} else if cur != prevNonBlank {
				// Other block-level transitions.
				isBlock := cur == "heading" || cur == "blockquote" || cur == "hr"
				prevIsBlock := prevNonBlank == "heading" || prevNonBlank == "blockquote" || prevNonBlank == "hr"
				if isBlock || prevIsBlock {
					needBlankBefore = true
				}
			}
		}
		if needBlankBefore {
			result = append(result, "")
		}

		result = append(result, line)

		// Blank AFTER a non-fence block element if the next non-blank line is different.
		if cur == "heading" || cur == "blockquote" || cur == "hr" {
			j := i + 1
			for j < len(lines) && lines[j] == "" {
				j++
			}
			if j < len(lines) {
				nextKind := blockKind(lines[j])
				if nextKind != cur {
					result = append(result, "")
				}
			}
		}
	}

	// Collapse 2+ consecutive blank lines to 1.
	var collapsed []string
	blanks := 0
	for _, line := range result {
		if line == "" {
			blanks++
			if blanks == 1 {
				collapsed = append(collapsed, "")
			}
		} else {
			blanks = 0
			collapsed = append(collapsed, line)
		}
	}

	// Single trailing newline.
	text := strings.Join(collapsed, "\n")
	text = strings.TrimRight(text, "\n") + "\n"
	return text
}

// reformat parses src as markdown and returns the reformatted text.
func reformat(src []byte) []byte {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	reader := text.NewReader(src)
	doc := md.Parser().Parse(reader)

	f := &Formatter{src: src}
	f.emitNode(doc)

	prev := src
	for {
		result := []byte(f.render())
		if bytes.Equal(result, prev) {
			return result
		}
		// Re-parse and re-format until stable (bold-splits may create new
		// paragraphs that themselves need splitting).
		prev = result
		reader2 := text.NewReader(result)
		doc2 := md.Parser().Parse(reader2)
		f = &Formatter{src: result}
		f.emitNode(doc2)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: reformat-md-2sp FILE [FILE ...]")
		os.Exit(2)
	}
	any := false
	for _, path := range os.Args[1:] {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reformat-md-2sp: %v\n", err)
			os.Exit(2)
		}
		result := reformat(data)
		if !bytes.Equal(result, data) {
			if err := os.WriteFile(path, result, 0666); err != nil {
				fmt.Fprintf(os.Stderr, "reformat-md-2sp: %v\n", err)
				os.Exit(2)
			}
			fmt.Printf("Updated: %s\n", path)
			any = true
		} else {
			fmt.Printf("No change: %s\n", path)
		}
	}
	_ = any
}

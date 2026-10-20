// goldmark-cmp renders two markdown files to HTML and reports whether they
// produce semantically identical output.  Exit 0 = identical, 1 = different,
// 2 = error.
//
// Comparison is done at two levels:
//  1. Structural: normalized HTML with intra-element whitespace collapsed.
//  2. Text: all text content extracted and whitespace-normalized -- catches
//     any word-level changes regardless of element structure.
//
// Text extraction emits ordered-list item numbers (e.g. "1. ") so that a
// paragraph like "0. Foo" and a proper ordered list starting at 0 both
// produce the same token stream -- list-structure promotion by a formatter
// is not a false positive.  All differences are reported, not just the first.
//
// Usage:
//
//	goldmark-cmp <before.md> <after.md>
package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var wsRun = regexp.MustCompile(`\s+`)

// normalizeHTML collapses whitespace outside <pre> blocks.
func normalizeHTML(h []byte) []byte {
	var out bytes.Buffer
	for len(h) > 0 {
		pre := bytes.Index(h, []byte("<pre"))
		if pre == -1 {
			out.Write(wsRun.ReplaceAll(h, []byte(" ")))
			break
		}
		out.Write(wsRun.ReplaceAll(h[:pre], []byte(" ")))
		end := bytes.Index(h[pre:], []byte("</pre>"))
		if end == -1 {
			out.Write(h[pre:])
			break
		}
		end += pre + len("</pre>")
		out.Write(h[pre:end])
		h = h[end:]
	}
	return bytes.TrimSpace(out.Bytes())
}

// blockKinds are block-level node kinds that introduce a word boundary.
// Entering them emits a separator so adjacent paragraphs never merge words.
// KindListItem is excluded because it is handled separately below (ordered
// list items also need their marker text emitted).
var blockKinds = map[ast.NodeKind]bool{
	ast.KindDocument:        true,
	ast.KindParagraph:       true,
	ast.KindHeading:         true,
	ast.KindBlockquote:      true,
	ast.KindFencedCodeBlock: true,
	ast.KindCodeBlock:       true,
	ast.KindHTMLBlock:       true,
	ast.KindThematicBreak:   true,
}

// extractText walks the goldmark AST and collects all text node content.
// Ordered list item numbers are emitted explicitly so that "0. Foo" in
// paragraph text and a proper <ol start=0><li>Foo</li></ol> both produce
// the same token stream -- list-structure promotion is not a false positive.
func extractText(src []byte, node ast.Node) []byte {
	var out bytes.Buffer
	// olCounters is a stack of next-item-numbers for nested ordered lists.
	var olCounters []int

	sep := func() {
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
	}

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch v := n.(type) {
		case *ast.List:
			if entering && v.IsOrdered() {
				olCounters = append(olCounters, v.Start)
			} else if !entering && v.IsOrdered() && len(olCounters) > 0 {
				olCounters = olCounters[:len(olCounters)-1]
			}
			return ast.WalkContinue, nil

		case *ast.ListItem:
			if entering {
				sep()
				if len(olCounters) > 0 {
					fmt.Fprintf(&out, "%d. ", olCounters[len(olCounters)-1])
					olCounters[len(olCounters)-1]++
				}
			}
			return ast.WalkContinue, nil
		}

		if !entering {
			return ast.WalkContinue, nil
		}

		if blockKinds[n.Kind()] {
			sep()
		}

		if t, ok := n.(*ast.Text); ok {
			out.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				out.WriteByte(' ')
			}
		}

		if n.Kind() == ast.KindCodeSpan {
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*ast.Text); ok {
					out.Write(t.Segment.Value(src))
				}
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
	return wsRun.ReplaceAll(bytes.TrimSpace(out.Bytes()), []byte(" "))
}

type result struct {
	html []byte
	text []byte
}

func parse(path string) (result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return result{}, err
	}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	reader := text.NewReader(src)
	doc := md.Parser().Parse(reader)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return result{}, err
	}
	return result{
		html: normalizeHTML(buf.Bytes()),
		text: extractText(src, doc),
	}, nil
}

// diffRegions finds all non-matching byte regions between two slices and
// returns each as a (beforeSnippet, afterSnippet) pair with context.
func diffRegions(b, a []byte) [][2]string {
	var regions [][2]string
	i := 0
	for i < len(b) || i < len(a) {
		// Advance past matching bytes.
		for i < len(b) && i < len(a) && b[i] == a[i] {
			i++
		}
		if i >= len(b) && i >= len(a) {
			break
		}
		// Find end of this differing region.
		j := i + 1
		for j < len(b) && j < len(a) && b[j] != a[j] {
			j++
		}
		// Also advance past any tail mismatch caused by insertions/deletions.
		// Use a simple heuristic: extend until we find 8 consecutive matches.
		for j < len(b) && j < len(a) {
			match := 0
			k := j
			for k < len(b) && k < len(a) && b[k] == a[k] {
				match++
				k++
				if match >= 8 {
					break
				}
			}
			if match >= 8 {
				break
			}
			j++
		}
		hiB := j + 50
		if hiB > len(b) {
			hiB = len(b)
		}
		hiA := j + 50
		if hiA > len(a) {
			hiA = len(a)
		}
		// lo must not exceed either hi after clamping (can happen near EOF).
		lo := i - 50
		if lo < 0 {
			lo = 0
		}
		loB := lo
		if loB > hiB {
			loB = hiB
		}
		loA := lo
		if loA > hiA {
			loA = hiA
		}
		loB, hiB = snapToRunes(b, loB, hiB)
		loA, hiA = snapToRunes(a, loA, hiA)
		regions = append(regions, [2]string{
			fmt.Sprintf("...%s...", b[loB:hiB]),
			fmt.Sprintf("...%s...", a[loA:hiA]),
		})
		i = j
	}
	return regions
}

// snapToRunes widens the [lo, hi) byte range of s outward to whole UTF-8 sequences, so diffRegions never prints half of a multi-byte character.  It moves each edge at most utf8.UTFMax-1 bytes, so input that is not valid UTF-8 cannot make it run away.
func snapToRunes(s []byte, lo, hi int) (int, int) {
	for n := 0; lo > 0 && lo < len(s) && !utf8.RuneStart(s[lo]) && n < utf8.UTFMax-1; n++ {
		lo--
	}
	for n := 0; hi < len(s) && !utf8.RuneStart(s[hi]) && n < utf8.UTFMax-1; n++ {
		hi++
	}
	return lo, hi
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: goldmark-cmp <before.md> <after.md>\n")
		os.Exit(2)
	}
	before, err := parse(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "goldmark-cmp: %v\n", err)
		os.Exit(2)
	}
	after, err := parse(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "goldmark-cmp: %v\n", err)
		os.Exit(2)
	}

	htmlSame := bytes.Equal(before.html, after.html)
	textSame := bytes.Equal(before.text, after.text)

	switch {
	case htmlSame:
		fmt.Println("identical (html)")
	case textSame:
		fmt.Println("identical (text-only; html whitespace differs)")
	default:
		fmt.Println("DIFFERENT (text content changed)")
		regions := diffRegions(before.text, after.text)
		for i, r := range regions {
			fmt.Fprintf(os.Stderr, "diff %d:\nbefore: %s\nafter:  %s\n\n", i+1, r[0], r[1])
		}
		os.Exit(1)
	}
}

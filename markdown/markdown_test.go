package markdown_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"

	"github.com/tianon/fmt/tianonfmt/markdown"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// ── format / AST ─────────────────────────────────────────────────────────────

func TestFormat(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.md", []testutil.Case{
		{Out: "output.md", Fn: func(src string) (string, error) {
			return markdown.Format(src), nil
		}, Idem: true},
		{Out: "ast.json", Fn: func(src string) (string, error) {
			b, err := json.MarshalIndent(markdown.MarshalFile(src, "input.md"), "", "\t")
			if err != nil {
				return "", err
			}
			return string(b) + "\n", nil
		}},
	})
}

// ── lint ──────────────────────────────────────────────────────────────────────

func TestLint(t *testing.T) {
	testutil.Golden(t, "testdata/lint", "input.md", []testutil.Case{
		{Out: "output.txt", Fn: func(src string) (string, error) {
			vs := markdown.Lint(src)
			var sb strings.Builder
			for _, v := range vs {
				fmt.Fprintf(&sb, "%d: %s\n", v.Line, v.Msg)
			}
			return sb.String(), nil
		}},
	})
}

// ── token-level format preservation ──────────────────────────────────────────

// mdBulletRE matches a list-item bullet (* or +) with its trailing space.
var mdBulletRE = regexp.MustCompile(`^([ \t]*)([*+])([ \t])`)

// normalizeMarkdown applies the four known mechanical rewrites that the
// markdown formatter makes beyond pure whitespace, so that
// normalizeMarkdown(format(input)) == normalizeMarkdown(input):
//
//  1. Strip trailing whitespace (preserving exactly 2 trailing spaces, the
//     CommonMark soft-break convention).
//  2. Normalize list-item bullets: * and + → -.
//  3. Convert setext headings (text / === or ---) to ATX (# text).
//  4. Collapse runs of 2+ consecutive blank lines to a single blank line.
func normalizeMarkdown(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Setext heading detection: non-blank line followed by a line of only
		// '=' (level 1) or '-' (level 2, at least 2 chars to distinguish from
		// a one-char thematic element, though markdown formatters don't emit those).
		if i+1 < len(lines) {
			under := strings.TrimRight(lines[i+1], " \t")
			text := strings.TrimRight(line, " \t")
			if len(under) >= 1 && text != "" && strings.Count(under, "=") == len(under) {
				out = append(out, "# "+text)
				i++ // consume underline
				continue
			}
			if len(under) >= 2 && text != "" && strings.Count(under, "-") == len(under) {
				out = append(out, "## "+text)
				i++
				continue
			}
		}

		// Strip trailing whitespace, but preserve exactly two trailing spaces
		// (CommonMark soft break).
		stripped := strings.TrimRight(line, " \t")
		if strings.HasSuffix(line, "  ") && !strings.HasSuffix(line, "   ") {
			stripped += "  "
		}

		// Normalize bullet markers: * and + → -.
		stripped = mdBulletRE.ReplaceAllString(stripped, "$1-$3")

		out = append(out, stripped)
	}

	// Collapse consecutive blank lines to at most one.
	result := make([]string, 0, len(out))
	lastBlank := false
	for _, line := range out {
		isBlank := strings.TrimSpace(line) == ""
		if isBlank && lastBlank {
			continue
		}
		result = append(result, line)
		lastBlank = isBlank
	}

	return strings.Join(result, "\n")
}

// TestFormatPreservesTokens verifies that markdown.Format does not silently
// alter content beyond the four known mechanical transformations (bullet
// normalization, setext→ATX, blank-line collapsing, trailing-whitespace
// removal).  Expected value is derived from the raw input text; no golden file
// is used.
func TestFormatPreservesTokens(t *testing.T) {
	dirs, err := os.ReadDir("testdata/format")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		t.Run(d.Name(), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata/format", d.Name(), "input.md"))
			if err != nil {
				t.Skip("no input.md")
				return
			}
			formatted := markdown.Format(string(src))
			normIn := normalizeMarkdown(string(src))
			normOut := normalizeMarkdown(formatted)
			if normIn != normOut {
				t.Errorf("normalizeMarkdown(format(input)) != normalizeMarkdown(input)\n\nnorm(input):\n%s\n\nnorm(format):\n%s",
					normIn, normOut)
			}
		})
	}
}

// ── golden helper ─────────────────────────────────────────────────────────────

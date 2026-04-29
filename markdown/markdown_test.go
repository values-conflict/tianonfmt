package markdown_test

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"

	"github.com/tianon/fmt/tianonfmt/markdown"
)


func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// ── Format ────────────────────────────────────────────────────────────────────

func TestFormat(t *testing.T) {
	testutil.Golden(t, "testdata/format", ".md", ".md", func(src string) (string, error) {
		return markdown.Format(src), nil
	})
}

func TestFormatIdempotent(t *testing.T) {
	testutil.Golden(t, "testdata/format", ".md", ".md", func(src string) (string, error) {
		p1 := markdown.Format(src)
		return markdown.Format(p1), nil
	})
}

// ── Format unit tests ─────────────────────────────────────────────────────────

func TestFormat_SetextToATX(t *testing.T) {
	src := "Title\n=====\n\nSub\n---\n\nParagraph.\n"
	got := markdown.Format(src)
	if !strings.HasPrefix(got, "# Title") {
		t.Errorf("setext h1 not converted: %q", got)
	}
	if !strings.Contains(got, "## Sub") {
		t.Errorf("setext h2 not converted: %q", got)
	}
}

func TestFormat_BulletNormalize(t *testing.T) {
	src := "* a\n+ b\n- c\n"
	got := markdown.Format(src)
	if strings.Contains(got, "*") || strings.Contains(got, "+") {
		t.Errorf("bullets not normalized to -: %q", got)
	}
	if strings.Count(got, "- ") != 3 {
		t.Errorf("expected 3 - bullets: %q", got)
	}
}

func TestFormat_TrailingWhitespace(t *testing.T) {
	src := "# Title   \nParagraph   \nSoft break.  \nNext.\n"
	got := markdown.Format(src)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// Header: trailing spaces removed
	if strings.HasSuffix(lines[0], " ") {
		t.Errorf("header should have no trailing space: %q", lines[0])
	}
	// Regular line: trailing spaces removed
	if strings.HasSuffix(lines[1], " ") {
		t.Errorf("paragraph should have no trailing space: %q", lines[1])
	}
	// Soft break: exactly two trailing spaces preserved
	if !strings.HasSuffix(lines[2], "  ") {
		t.Errorf("soft break should preserve two trailing spaces: %q", lines[2])
	}
}

func TestFormat_MaxOneBlankLine(t *testing.T) {
	src := "# A\n\n\n\n# B\n"
	got := markdown.Format(src)
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("more than one consecutive blank line in output: %q", got)
	}
}

func TestFormat_FencedPassthrough(t *testing.T) {
	// Content inside fenced code blocks must not be modified.
	src := "# Title\n\n```bash\n* star not converted\n+ plus not converted\n```\n"
	got := markdown.Format(src)
	if !strings.Contains(got, "* star not converted") {
		t.Errorf("content inside fence should be unchanged: %q", got)
	}
}

// ── Lint ──────────────────────────────────────────────────────────────────────

// ── MarshalFile ───────────────────────────────────────────────────────────────

func TestMarshalFile(t *testing.T) {
	src := "# Title\n\nParagraph.\n\n```bash\ncode\n```\n\n- item\n"
	v := markdown.MarshalFile(src, "README.md")
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatal("expected map")
	}
	if m["type"] != "markdown" {
		t.Errorf("expected type=markdown, got %v", m["type"])
	}
	if m["file"] != "README.md" {
		t.Errorf("expected file=README.md, got %v", m["file"])
	}
	blocks := m["blocks"].([]any)
	found := map[string]bool{}
	for _, b := range blocks {
		if bm, ok := b.(map[string]any); ok {
			found[bm["type"].(string)] = true
		}
	}
	for _, want := range []string{"heading", "paragraph", "codeBlock", "listItem", "blank"} {
		if !found[want] {
			t.Errorf("expected block type %q in AST", want)
		}
	}
}

func TestMarshalFile_SetextHeader(t *testing.T) {
	src := "Title\n=====\n\nSub\n---\n"
	v := markdown.MarshalFile(src, "-")
	m := v.(map[string]any)
	blocks := m["blocks"].([]any)
	if len(blocks) < 2 {
		t.Fatal("expected at least 2 blocks")
	}
	h1 := blocks[0].(map[string]any)
	if h1["level"] != 1 || h1["text"] != "Title" {
		t.Errorf("expected h1 Title, got %v", h1)
	}
}

// ── Lint ──────────────────────────────────────────────────────────────────────

func TestLint_ShLanguage(t *testing.T) {
	src := "# Readme\n\n```sh\necho hi\n```\n"
	vs := markdown.Lint(src)
	if len(vs) == 0 {
		t.Error("expected violation for 'sh' language identifier")
	}
	if !strings.Contains(vs[0].Msg, "sh") {
		t.Errorf("unexpected message: %q", vs[0].Msg)
	}
}

func TestLint_ShellLanguage(t *testing.T) {
	src := "# Readme\n\n```shell\necho hi\n```\n"
	vs := markdown.Lint(src)
	if len(vs) == 0 {
		t.Error("expected violation for 'shell' language identifier")
	}
}

func TestLint_BashOK(t *testing.T) {
	src := "# Readme\n\n```bash\nset -Eeuo pipefail\n```\n"
	vs := markdown.Lint(src)
	if len(vs) != 0 {
		t.Errorf("expected no violations for 'bash' identifier, got %v", vs)
	}
}

func TestLint_ConsoleOK(t *testing.T) {
	src := "# Readme\n\n```console\n$ echo hi\n```\n"
	vs := markdown.Lint(src)
	if len(vs) != 0 {
		t.Errorf("expected no violations for 'console' identifier, got %v", vs)
	}
}

// ── golden helper ─────────────────────────────────────────────────────────────



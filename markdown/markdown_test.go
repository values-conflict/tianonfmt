package markdown_test

import (
	"encoding/json"
	"flag"
	"fmt"
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
	testutil.Golden(t, "testdata/format", "input.md", "output.md", func(src string) (string, error) {
		return markdown.Format(src), nil
	})
}

func TestFormatIdempotent(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.md", "output.md", func(src string) (string, error) {
		p1 := markdown.Format(src)
		return markdown.Format(p1), nil
	})
}

func TestFormatRoundTrip(t *testing.T) {
	testutil.Golden(t, "testdata/format", "output.md", "output.md", func(src string) (string, error) {
		return markdown.Format(src), nil
	})
}

// ── Lint ──────────────────────────────────────────────────────────────────────

func TestLint(t *testing.T) {
	testutil.Golden(t, "testdata/lint", "input.md", "output.txt", func(src string) (string, error) {
		vs := markdown.Lint(src)
		var sb strings.Builder
		for _, v := range vs {
			fmt.Fprintf(&sb, "%d: %s\n", v.Line, v.Msg)
		}
		return sb.String(), nil
	})
}

// ── MarshalAST golden ─────────────────────────────────────────────────────────

// TestMarshalAST pins the full JSON AST output for every format fixture.
func TestMarshalAST(t *testing.T) {
	testutil.Golden(t, "testdata/format", "input.md", "ast.json", func(src string) (string, error) {
		b, err := json.MarshalIndent(markdown.MarshalFile(src, "input.md"), "", "\t")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	})
}

// ── golden helper ─────────────────────────────────────────────────────────────

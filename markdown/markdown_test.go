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

// ── golden helper ─────────────────────────────────────────────────────────────

package shell_test

import (
	"github.com/values-conflict/tianonfmt/shell"
	"mvdan.cc/sh/v3/syntax"
	"strings"
	"testing"
)

func TestFormatFlagNormalization(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"curl -S https://example.com\n", "curl -sS https://example.com\n"},
		// -f -s stay separate in format mode: merging is a tidy-mode operation.
		{"curl -f -s https://example.com\n", "curl -f -s https://example.com\n"},
		{"curl -sS https://example.com\n", "curl -sS https://example.com\n"},
	}
	for _, c := range cases {
		out, err := shell.Format(c.in, syntax.LangBash)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !strings.Contains(out, strings.TrimSuffix(c.want, "\n")) {
			t.Errorf("Format(%q) = %q, want contains %q", c.in, out, c.want)
		}
	}
}

// Package testutil provides shared test helpers for tianonfmt packages.
package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Update is the -update flag.  When set, golden output files are regenerated
// rather than compared.  Each test binary registers this flag independently.
var Update = flag.Bool("update", false, "update golden test files")

// Case describes one output variant for a fixture suite.
type Case struct {
	// Out is the output filename within each fixture directory (e.g. "output.jq",
	// "output.tidy.jq", "ast.json").  Pass "" for error-only cases: Fn must
	// return a non-nil error, and the message is compared to error.txt.
	Out string

	// Fn transforms the input source and returns the expected output.
	// For error-only cases (Out==""), Fn must return a non-nil error.
	Fn func(string) (string, error)

	// Idem verifies idempotency: Fn(Fn(input)) == Fn(input).
	// Only meaningful when Out != "".
	Idem bool
}

// Golden runs golden-file tests from dir.  Each subdirectory must contain a
// file named in.  For each Case, the corresponding output (or error.txt) is
// compared or written on -update.
//
// Typical call — one per suite directory per package:
//
//	testutil.Golden(t, "testdata/format", "input.jq", []testutil.Case{
//	    {Out: "output.jq",       Fn: formatFn, Idem: true},
//	    {Out: "output.tidy.jq",  Fn: tidyFn,   Idem: true},
//	    {Out: "ast.json",        Fn: astFn},
//	})
func Golden(t *testing.T, dir, in string, cases []Case) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			t.Helper()
			inPath := filepath.Join(dir, e.Name(), in)
			src, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			for _, c := range cases {
				name := c.Out
				if name == "" {
					name = "error"
				}
				t.Run(name, func(t *testing.T) {
					t.Helper()
					runCase(t, filepath.Join(dir, e.Name()), string(src), c)
				})
			}
		})
	}
}

func runCase(t *testing.T, fixtureDir, src string, c Case) {
	t.Helper()
	errPath := filepath.Join(fixtureDir, "error.txt")

	got, fnErr := c.Fn(src)

	// Error-only case (Out == "").
	if c.Out == "" {
		if *Update {
			if fnErr == nil {
				t.Fatalf("out is empty (error-only case) but fn succeeded with output: %s", got)
			}
			if err := os.WriteFile(errPath, []byte(fnErr.Error()+"\n"), 0o644); err != nil {
				t.Fatalf("write error.txt: %v", err)
			}
			return
		}
		errData, readErr := os.ReadFile(errPath)
		if readErr != nil {
			t.Fatalf("error.txt missing — run `go test -update` to create it: %v", readErr)
		}
		if fnErr == nil {
			t.Errorf("expected error but fn succeeded;\noutput: %s", got)
			return
		}
		want := strings.TrimRight(string(errData), "\n")
		if fnErr.Error() != want {
			t.Errorf("error mismatch\ngot:  %q\nwant: %q", fnErr.Error(), want)
		}
		return
	}

	// Success case.
	outPath := filepath.Join(fixtureDir, c.Out)

	if fnErr != nil {
		t.Fatalf("run: %v", fnErr)
	}

	if c.Idem {
		got2, err2 := c.Fn(got)
		if err2 != nil {
			t.Fatalf("idempotency re-run: %v", err2)
		}
		if got2 != got {
			t.Errorf("not idempotent:\nfirst:  %s\nsecond: %s", got, got2)
		}
	}

	if *Update {
		if err := os.WriteFile(outPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("golden %s missing — run `go test -update` to create it: %v", outPath, err)
	}
	if got != string(want) {
		t.Errorf("output mismatch\n--- got ---\n%s--- want ---\n%s", got, string(want))
	}
}

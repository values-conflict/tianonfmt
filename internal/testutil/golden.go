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

// Golden runs golden-file tests from dir.  Each subdirectory must contain a
// file named in.  The expected outcome is declared by whichever golden file
// exists in that subdirectory:
//
//   - out       — fn must succeed; output is compared to this file.
//     Pass "" for out to declare an error-only suite; Golden then fails the
//     test if fn ever succeeds (output would have nowhere to go).
//   - error.txt — fn must return a non-nil error; the error message
//     (without trailing newline) is compared to this file.
//
// Typical calls:
//
//	Golden(t, "testdata/format", "input.jq", "output.jq", fn)
//	Golden(t, "testdata/format", "input.jq", "ast.json",  fn)  // AST variant
//	Golden(t, "testdata/errors", "input.jq", "",          fn)  // error-only suite
//
// Run `go test -update` to regenerate golden files.  On -update, whichever
// file type matches the actual outcome is written; the other is removed.
func Golden(t *testing.T, dir, in, out string, fn func(string) (string, error)) {
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
			errPath := filepath.Join(dir, e.Name(), "error.txt")

			src, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			got, fnErr := fn(string(src))

			// Error-only suite: fn must never succeed.
			if out == "" && fnErr == nil {
				t.Fatalf("%s: out is empty (error-only suite) but fn succeeded with output: %s", e.Name(), got)
				return
			}

			outPath := filepath.Join(dir, e.Name(), out)

			if *Update {
				if fnErr != nil {
					if err := os.WriteFile(errPath, []byte(fnErr.Error()+"\n"), 0o644); err != nil {
						t.Fatalf("write error.txt: %v", err)
					}
					if out != "" {
						os.Remove(outPath)
					}
				} else {
					if err := os.WriteFile(outPath, []byte(got), 0o644); err != nil {
						t.Fatalf("write golden: %v", err)
					}
					os.Remove(errPath)
				}
				return
			}

			errData, errFileErr := os.ReadFile(errPath)
			outData, outFileErr := func() ([]byte, error) {
				if out == "" {
					return nil, os.ErrNotExist
				}
				return os.ReadFile(outPath)
			}()

			switch {
			case errFileErr == nil:
				// This fixture expects an error.
				if fnErr == nil {
					t.Errorf("%s: expected error but fn succeeded;\noutput: %s", e.Name(), got)
					return
				}
				want := strings.TrimRight(string(errData), "\n")
				if fnErr.Error() != want {
					t.Errorf("%s: error mismatch\ngot:  %q\nwant: %q", e.Name(), fnErr.Error(), want)
				}
			case outFileErr == nil:
				// This fixture expects success.
				if fnErr != nil {
					t.Fatalf("%s: run: %v", e.Name(), fnErr)
				}
				if got != string(outData) {
					t.Errorf("output mismatch for %s\n--- got ---\n%s--- want ---\n%s", e.Name(), got, string(outData))
				}
			default:
				if out == "" {
					t.Fatalf("%s: no error.txt found — run `go test -update` to create one", e.Name())
				} else {
					t.Fatalf("%s: no golden file found (neither %s nor error.txt) — run `go test -update` to create one", e.Name(), out)
				}
			}
		})
	}
}

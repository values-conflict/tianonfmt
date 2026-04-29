// Package testutil provides shared test helpers for tianonfmt packages.
package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// Update is the -update flag.  When set, golden output files are regenerated
// rather than compared.  Each test binary registers this flag independently.
var Update = flag.Bool("update", false, "update golden test files")

// Golden runs golden-file tests from dir.  Each subdirectory must contain
// input{inExt}; output{outExt} is the golden file.
// Run `go test -update` to regenerate golden files.
func Golden(t *testing.T, dir, inExt, outExt string, fn func(string) (string, error)) {
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
			inPath := filepath.Join(dir, e.Name(), "input"+inExt)
			outPath := filepath.Join(dir, e.Name(), "output"+outExt)
			in, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			got, err := fn(string(in))
			if err != nil {
				t.Fatalf("run: %v", err)
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
				t.Errorf("output mismatch for %s\n--- got ---\n%s--- want ---\n%s", e.Name(), got, string(want))
			}
		})
	}
}

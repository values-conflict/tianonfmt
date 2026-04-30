# tianonfmt — development guidelines

## Code coverage

**Target: 100%.**  Before and after every non-trivial change, run the full test suite with coverage and confirm no regression:

```sh
go test -coverprofile=/tmp/cov.out ./...
go tool cover -func=/tmp/cov.out | tail -1
```

If coverage drops, add tests before moving on.  Rechecking after a refactor is not optional — it has caught real regressions in this codebase.

## Test hierarchy

Prefer in this order:

1. **Real corpus fixtures** — inputs taken verbatim from `../corpus/` and committed to `testdata/`.  Most convincing; proves the formatter round-trips actual code.
2. **Contrived golden fixtures** — hand-written input/output pairs in `testdata/`.  Use when the corpus doesn't cover an edge case.
3. **Go table/unit tests** — only when a golden file would be awkward (e.g., testing a pure function with many small inputs, or testing error paths).

Never write a plain unit test when a golden file could serve the same purpose (and achieve the same coverage).

## Golden fixture pattern

All file-in / file-out formatters use `testutil.Golden()` from `internal/testutil`:

```go
testutil.Golden(t, "testdata/format", ".sh", ".sh", func(src string) (string, error) {
    return shell.Format(src, shell.DetectLang(src), nil)
})
```

- Input files: `testdata/<suite>/<name>/input<inExt>`
- Golden output files: `testdata/<suite>/<name>/output<outExt>` (regenerate with `-update`)
- Always add both an idempotency test (apply twice, compare) alongside the primary golden test
- Organize testdata by suite (`format/`, `tidy/`, `pedantic/`, `errors/`, `lint/`) so purpose is obvious from the path

### Golden error fixtures

If a fixture directory contains `error.txt` instead of `output<outExt>`, `testutil.Golden` expects the function to return a non-nil error and compares `err.Error()` to the file content.  Use this to pin exact parse-error messages.  Run `go test -update` to generate or regenerate `error.txt` files.

### AST golden fixtures

Every package with an AST marshaler has a `TestMarshalAST` golden test that reuses `testdata/format/` inputs and writes `output.json`, pinning the complete `--ast` JSON structure:

```go
testutil.Golden(t, "testdata/format", ".jq", ".json", func(src string) (string, error) {
    f, err := jq.ParseFile(src)
    if err != nil { return "", err }
    b, err := json.MarshalIndent(f.MarshalAST(), "", "\t")
    if err != nil { return "", err }
    return string(b) + "\n", nil
})
```

Any regression in field names, nesting, or ordering produces a readable diff.  Packages: `jq`, `shell`, `dockerfile`, `markdown`.

## Architecture

- **One package per language**: `jq/`, `shell/`, `dockerfile/`, `markdown/`, `template/`
- **Shared utilities in `internal/`** — never copy helpers across packages; extract to `internal/` instead
- **Testable entry point**: the `cmd/` binary exposes `run(args []string, stdin, stdout, stderr) int` so the CLI can be integration-tested without subprocess overhead
- **Single dispatch enum** (`fileKind`) — when the same set of file types is switched on in multiple places, consolidate into one enum and one set of helper functions; parallel switches are a maintenance hazard

## Code quality

- **DRY**: proactively look for duplication and dead code; eliminate before adding new code
- **Builtins over helpers**: Go 1.21+ provides `min`/`max` builtins — do not write local equivalents; remove any that exist
- **Exhaustive switches**: for AST node types and `fileKind`, use compile-time-checked exhaustive switches so new variants cause build failures, not silent no-ops
- **American English spelling**: `normalize` not `normalise`; `color` not `colour`; etc.
- **Compiler enforcement of interface implementation**: Every type that claims to implement an interface gets a compile-time assertion adjacent to the type definition
- **Avoid premature interfaces**: Go interfaces with a single canonical implementation are painful to traverse in a codebase — every call requires an extra indirection. Use interfaces only when: (1) there are multiple concrete implementations, or (2) the interface defines a contract for external implementers.

## CLI flags

- `--tidy` applies idiomatic rewrites (shebang, `|| true → || :`, `which → command -v`, simple shell-form → JSON form)
- `--pedantic` implies `--tidy` and applies stricter rewrites (set-flag normalization, all shell forms → JSON form with explicit shell injection)
- `-w` (write): prints names of changed files, silent for unchanged; errors on stdin; mutually exclusive with `-d`
- `-d` (diff): prints unified diffs; mutually exclusive with `-w`

## Dockerfile instruction form terminology

Use **"json form"** (bracket syntax) and **"plain form"** (bare string syntax) when referring to `COPY`, `ADD`, `VOLUME`.  Never "exec form" or "shell form" — those terms are misleading for instructions that have no shell evaluation semantics.

## Embedded languages

When a language is embedded inside another (jq in shell, shell in Dockerfile RUN), the formatter must:
1. Detect the embedded fragment
2. Extract it, format it with the appropriate sub-formatter, and re-insert it in-place
3. Preserve surrounding context (arguments, redirects, whitespace) exactly

Known limitation (as of 2026-04): multi-line jq expressions spread across multiple Dockerfile RUN continuation lines are not reformatted — the expression must appear on a single continuation line. This should be fixed.

## Documentation

Style documentation lives in `docs/`.  Rules:
- Each language gets its own file; embedded-language variants get their own errata file (e.g., `jq-sh.md` alongside `jq.md`)
- Cross-reference related docs wherever relevant
- Document what differs from enforced ecosystem norms; do not document what `gofmt` already enforces automatically
- Document intentional omissions explicitly under a "Notable omissions" section
- `TODO` is always ALL-CAPS followed by a concrete, specific description — no vague TODOs

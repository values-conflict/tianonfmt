# tianonfmt — development guidelines

## Code coverage

**Target: 100%.**  Before and after every non-trivial change, run the full test suite with coverage and confirm no regression:

```sh
go test -coverprofile=/tmp/cov.out -coverpkg=./... ./...
go tool cover -func=/tmp/cov.out | tail -1
```

`-coverpkg=./...` is required to correctly track coverage for shared helper packages such as `internal/testutil/jqnorm.go`, which are imported by test binaries in other packages and would otherwise show 0% without this flag.

**Known uncoverable lines** (not a bug, not a gap):
- `jqNode()` / `nodePos()` one-liner interface-marker methods in `jq/ast.go` — Go's coverage tool shows these as 0% because they are empty `{}` bodies or single-return stubs whose only purpose is interface satisfaction; there is no executable statement to instrument.
- `templateSeg()` marker methods in `template/template.go` — same reason.
- `main()` in `cmd/tianonfmt/main.go` — the test harness calls `run()` directly; `main()` is intentionally excluded.

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

### Minimise the number of distinct input files

**If we can parse it, we can format it, tidy it, and pedantic it.**  Every input that exists should be tested against every applicable transformer.

- **Do not create separate input files per suite.**  `testdata/format/` is the primary home for inputs.  `TestFormat`, `TestTidy`, `TestFormatRoundTrip`, and `TestMarshalAST` all read from `testdata/format/` and write differently-named outputs into the same fixture directory (`output.sh`, `output.tidy.sh`, `ast.json`).
- **`testdata/tidy/` (and other suite subdirectories) exist only for inputs whose edge case is impossible to express in the format suite.**  If an input could live in `testdata/format/`, it must — a duplicate in `testdata/tidy/` is dead weight.
- Before adding any new fixture, verify no existing fixture already exercises the same AST paths.  If one does, extend it rather than creating a parallel one.

### Fixture attribution (`meta.txt`)

Every fixture directory whose input file was copied verbatim from an external source must contain a `meta.txt`:

```
Source: https://github.com/foo/bar/blob/<full-40-char-SHA>/path/to/file
License: <Debian well-known short name>  (Expat, Apache-2.0, GPL-2, GPL-3, AGPL-3, …)
```

Use the full 40-character commit SHA — never a branch ref.  Add a `Note:` line for anything needing clarification (e.g. the file is a snapshot of an older version, or it is shared verbatim between multiple upstream projects).

For fixtures sourced from `corpus/` or `anticorpus/` (Tianon's own code or Docker official image repos), still include `Source:`.  If the source repo has no license file, write `License: **WARNING:** UNKNOWN` instead of omitting the line.

This convention is enforced by review, not tooling — always add `meta.txt` when copying fixture content from any repo.

### AST design: parser and formatter are separate concerns

**Parsers** and **formatters** must never be conflated.  The parser's job is to capture everything — every syntax form, every comment, every structurally-meaningful choice — into the AST.  The formatter's job is to transform that AST into canonical text, applying style rules.

Concretely:

- An AST node must distinguish syntactically different forms that are semantically equivalent.  For example, `jq.Index.DotAccess` records whether the original source used `."key"` (dot-quoted) vs `.["key"]` (bracket) — both mean the same thing but the AST must remember which was written.
- **Whitespace** is the one exception: whitespace between tokens is not preserved in the AST.  The formatter applies canonical whitespace.
- **Comments** must always be preserved in the AST and reproduced faithfully by the formatter.
- If `format(parse(any_valid_input))` produces output that differs from `format(parse(format(parse(any_valid_input))))`, the AST is incomplete — it dropped information on the first parse.

If you find a valid input where `format(parse(x)) != format(parse(format(parse(x))))`, the AST node for the relevant construct is missing a field.  Add the field to the AST, set it in the parser, and use it in the formatter.

### AST round-trip test

`TestFormatIdempotent` asserts `format(format(input)) == format(input)`.  `TestFormat` asserts `format(input) == golden`.  Together these imply `format(golden) == golden` — the round-trip property — so no separate `TestFormatRoundTrip` is needed.

If the formatter changes something on a second pass, `TestFormatIdempotent` catches it; if it produces wrong output, `TestFormat` catches it.

### Token-level semantic preservation test

The golden-file and idempotency tests share a blind spot: if a formatter bug is *consistent* — it makes the same wrong change on every pass — `TestFormatIdempotent` passes and `TestFormat` passes (once the golden file is regenerated to match the bug).  The golden file is only as trustworthy as the formatter that produced it.

To close this gap, every language package has a **`TestFormatPreservesTokens`** that verifies `normalize(format(input)) == normalize(input)` using a **pure text normalizer** — no AST, no parser, no golden file for the expected value.  The expected result is derived directly from the raw input source.

**How it works:**

1. **Tokenize** the source into a flat sequence of non-whitespace, non-comment tokens, discarding all layout.  The tokenizer must correctly handle the language's string quoting and any string-interpolation syntax — a naïve scanner that stops at the first closing delimiter will produce different wrong token boundaries for input vs. formatted output when the formatter reflows code near the incorrectly-identified boundary.

2. **Normalize** the token sequence by applying only the known mechanical rewrites the formatter makes beyond pure whitespace.  Known normalizations per language:

   | Language | Normalizations |
   |---|---|
   | **jq** | Unquote `"identifier":` → `identifier:` (object key); remove `,` before `}` (trailing comma) |
   | **shell** | Discard standalone `;` (→ newline); strip spaces inside `$((…))`; recursively tokenize `$(…)` subshell contents inside all tokens (words and `"…"` strings) — the formatter reflows indentation and pipe placement inside subshells; discard standalone `\` (line-continuation artifact) |
   | **dockerfile** | Split `WORD=VALUE` at first `=`; unquote `"…"` strings by stripping quotes, splitting at whitespace, and splitting each word at `=` — handles ENV quote removal and indentation changes inside multi-line shell args |
   | **markdown** | Normalize bullets `*`/`+` → `-`; convert setext headings to ATX; strip trailing whitespace (preserving exactly 2-space soft break); collapse 2+ blank lines to one |
   | **template** | Apply jq normalization (`testutil.NormalizeJQ`) to each `{{ … }}` block's content; leave literal text verbatim |

3. **Compare** the two normalized sequences.  If they differ, the formatter changed a token — dropped something, reordered, or rewrote content — without authorization.

**What it catches that the other tests do not:**

- A comment silently dropped by the formatter (the golden file was regenerated without noticing)
- An expression reordered or a token deleted because of a self-consistent parser/formatter bug
- Any regression where `format∘format = format` holds but `format(x) ≠ x` in a semantically meaningful way

**How to identify the normalizations for a new language:**

Compare token sequences of input and formatted output across all fixtures: anything that differs after discarding whitespace is a candidate normalization.  If the list is long, the formatter is making unauthorized content changes and those should be fixed rather than normalized away.

**Implementing the tokenizer:**

The tokenizer must use no imports from the language package so the test does not transitively trust the parser.  For languages where the formatter reformats code inside string-embedded sub-languages (shell `$(…)`, jq `\(…)`, etc.) the scanner must **recursively tokenize those embedded regions** — not just correctly identify their boundaries.  This is a stronger requirement than just getting string boundaries right: even with a perfect boundary scanner, the formatter may reflow code inside the subshell, producing different whitespace in the same opaque string token.  Recursive tokenization of the embedded code discards that whitespace and makes both sides compare equal.

For `$(…)` / `\(…)` nesting, use mutual recursion: `scanString` delegates to `scanInterp` on the opener, `scanInterp` recurses back into `scanString` when it encounters a nested string.

**Shared jq tokenizer:**

`testutil.TokenizeJQ` and `testutil.NormalizeJQ` in `internal/testutil/jqnorm.go` are the canonical jq tokenizer/normalizer, shared by both `jq/jq_test.go` and `template/template_test.go`.

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

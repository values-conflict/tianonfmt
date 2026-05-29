# tianonfmt — development guidelines

## Code coverage

**Target: 100%.**  Before and after every non-trivial change, run the full test suite with coverage and confirm no regression:

```sh
go test -coverprofile=/tmp/cov.out -coverpkg=./... ./...
go tool cover -func=/tmp/cov.out | tail -1
```

`-coverpkg=./...` is required to correctly track coverage for shared helper packages such as `internal/testutil/jqnorm.go`, which are imported by test binaries in other packages and would otherwise show 0% without this flag.

**Known uncoverable lines** (not a bug, not a gap):
- `jqNode()` / `nodePos()` one-liner interface-marker methods in `jq/ast.go` — Go's coverage tool shows these as 0% because they are empty `{}` bodies or single-return stubs whose only purpose is interface satisfaction; there is no executable statement to instrument.  This applies to all AST node types including `InterpStr`.
- `normalizeInterpInStr` `end < 0 || end > len(tok)` fallback in `internal/testutil/jqnorm.go` — fires only for string tokens with unterminated `\(...)` interpolations; `TokenizeJQ` uses `ScanJQStr` which correctly balances `\(...)` depth, so tokens it produces will never trigger this branch.
- `templateSeg()` marker methods in `template/template.go` — same reason.
- `main()` in `cmd/tianonfmt/main.go` — the test harness calls `run()` directly; `main()` is intentionally excluded.
- `sortFlagsByOrder` `pri < 0` branch in `shell/flags.go` — fires only if a canonical combined-flag group contains a char absent from the order string, which is a configuration error impossible via well-formed input.
- `preMergeFlags` `fs.hasArg` guard in `shell/flags.go` — fires if a hasArg flag is listed in a command's merge group; current configuration never puts hasArg flags in merge groups.
- `movePriorityFlags` stable-second-pass exit path in `shell/flags.go` — the outer `for changed` loop's second iteration exits immediately when no further moves are needed; the `changed = false` reset is instrumented but the "nothing to do" steady state is indistinguishable from a fully-covered run.
- `reorderCombinedGroups` `sorted != flags` branch in `shell/flags.go` — covered: format mode with `grep -Eq` triggers it (see `TestNormalizeShortFlags_GrepReorderFormat`).
- `panic(...)` assertion in `template/template.go`'s `writeBlockExpr` — this is the structural guard that fires if a valid template block somehow arrives with an empty formatted string; it is intentionally unreachable for well-formed templates (if it ever fires, that's a bug to fix, not to test around).
- `trimPiece` blank-line stripping loops in `template/template.go` — these strip leading/trailing blank lines from assembly pieces; the jq formatter's output for current corpus templates never produces sentinel-adjacent blank lines, so these paths go untriggered.
- `bracketDelta` comment/escape inner paths in `template/template.go` — covered by the `bracket-delta-paths` contrived fixture, which uses a multi-line template block with a jq comment and an escaped string.
- `applyFormatRewrites` in `shell/format.go` — empty `{}` body; no statements to instrument.  The function is a placeholder for future format-pass AST rewrites.
- `arithCloseParen` `return -1` paths in `shell/format.go` — fire when the closing `))` of an arithmetic expression is missing entirely (end-of-string) or when a `)` at depth 0 is not doubled.  Both are unreachable for valid shell (the printer never produces malformed arithmetic); the `\\` escape-skip inside the double-quote loop similarly requires a backslash-escaped quote character inside an arithmetic subscript, which does not appear in the corpus.
- `evalCmdSubstClose` backtick path and `return -1` path in `shell/format.go` — the backtick path fires only when a backtick substitution appears inside `eval $(...)`, which doesn't appear in the corpus (backticks are converted to `$()` by the printer); the `return -1` path fires only for unclosed command substitutions.
- `jqWalkStmt` and `jqWalkWord` nil guards in `shell/format.go` — defensive checks; all call sites either guard before calling or pass struct fields that are non-nil for valid ASTs.
- `jq/token.go:Kind.String()` `fmt.Sprintf` fallback — fires only for a `Kind` value not in `kindNames`, impossible for any well-formed `Kind` constant.
- `optStringFlag.setShort()` in `internal/flags/flags.go` — covered by `TestParse_OptStringShort` in the flags unit tests, which registers an OptString with a non-zero short byte and invokes it via `-o`.
- `jqWalkWord` nil guard in `shell/format.go` — every call site either checks `!= nil` before calling or passes a struct field that is always non-nil for valid shell ASTs.
- `Kind.String()` `fmt.Sprintf` fallback in `jq/token.go` — fires only for a `Kind` value not in `kindNames`, which cannot occur for any well-formed `Kind` constant.
- `marshalASTJSON` error path in `cmd/tianonfmt/main.go` — `json.Encoder.Encode` fails only for non-serialisable types (channels, functions); all AST marshal methods return plain maps/slices.
- Re-parse-after-format error paths in `jqASTPair`, `shellASTPair`, `dockerfileASTPair` — the formatter is guaranteed to produce valid output for valid input; if it doesn't, that is a bug to fix, not to test around.
- `computeDiff` error path in `printAST` — `os.CreateTemp` failure requires OS-level conditions (full disk, bad permissions) that cannot be injected by unit tests.

If coverage drops, add tests before moving on.  Rechecking after a refactor is not optional — it has caught real regressions in this codebase.

**Reading coverage numbers correctly:**  Always use the canonical command above and trust `tail -1` as ground truth.  When coverage drops, identify regressions with `go tool cover -func=/tmp/cov.out | grep -v 100.0%` and diagnose from the actual numbers — do not invent explanations.  Never compare across incompatible baselines: stashed vs. unstashed code, cached vs. fresh runs, and runs where tests were failing all produce different totals and must not be treated as equivalent.

## Test hierarchy

Prefer in this order:

1. **Real corpus fixtures** — inputs taken verbatim from `../corpus/` or `../anticorpus/` and committed to `testdata/`.  Most convincing; proves the formatter round-trips actual code.
2. **Contrived golden fixtures** — hand-written input/output pairs in `testdata/`.  Use when the corpus doesn't cover an edge case.
3. **Go table/unit tests** — only when a golden file would be awkward (e.g., testing a pure function with many small inputs, or testing error paths).

**HARD RULE: never write a `func TestXxx` for new formatter behaviour when a golden fixture would cover the same case.**  A test function that calls `shell.FormatWithTidy(src, ...)` and asserts on the output is always worse than a golden fixture — it's less readable, doesn't document intent, and isn't backed by real code.  The only legitimate uses of new `func TestXxx` beyond the standard `TestFormat`/`TestTidy`/etc. suite runners are:
- error paths (parse failures, invalid input)
- pure-function unit tests for small helpers (e.g. `sortFlagsByOrder`)
- edge cases that literally cannot be expressed as a file-in/file-out golden fixture

**When adding corpus/anticorpus fixtures:** always search `../corpus` and `../anticorpus` first before writing contrived fixtures.  The corpus search should target the specific pattern being tested (flag usage, if-condition style, etc.).  A contrived fixture is only justified when a corpus search turns up nothing useful.

**Corpus fixture sourcing — three hard requirements:**
1. **Remote commit only.** The file content must come from a commit reachable on `origin/main` or `origin/master` — not a local-only commit, not a local working-tree file that may have been modified by the formatter or by hand.  Verify with `git -C ../corpus/REPO ls-remote origin main master` before use.
2. **Verbatim from the commit, not the filesystem.** Always extract the fixture content with `git -C ../corpus/REPO show <SHA>:path/to/file`, never by copying the on-disk file.  The on-disk copy may have local modifications (including prior formatter runs) that silently corrupt the fixture.
3. **Full 40-character SHA in `meta.txt`.** The `Source:` URL must use the exact commit SHA obtained from `ls-remote` (not a branch name or short hash), producing a stable permalink.

## Golden fixture pattern

All file-in / file-out formatters use `testutil.Golden()` from `internal/testutil`:

```go
testutil.Golden(t, "testdata/format", "input.sh", []testutil.Case{
    {Out: "output.sh", Fn: func(src string) (string, error) {
        return shell.Format(src, shell.DetectLang(src))
    }, Idem: true},
})
```

- Input files: `testdata/<suite>/<name>/input.sh` (or `input.jq`, etc. — filename matches the `inFile` arg)
- Golden output files: `testdata/<suite>/<name>/<Case.Out>` (regenerate with `-update`)
- Set `Idem: true` on any case where the output should be idempotent (format∘format = format)
- Multiple cases per input (format, tidy, AST) are expressed as a `[]testutil.Case` slice
- Organize testdata by suite (`format/`, `errors/`) so purpose is obvious from the path

### Minimise the number of distinct input files

**If we can parse it, we can format it and tidy it.**  Every input that exists should be tested against every applicable transformer.

- **Do not create separate input files per suite.**  `testdata/format/` is the primary home for inputs.  A single `testutil.Golden` call with a `[]testutil.Case` slice covers format, tidy, AST, and any other transformer — all writing differently-named outputs into the same fixture directory (`output.sh`, `output.tidy.sh`, `ast.json`).
- **`testdata/errors/`, `testdata/lint/`, and any other suite subdirectory exist only for inputs whose edge case is impossible to express in the format suite.**  If an input could live in `testdata/format/`, it must — a duplicate elsewhere is dead weight.
- Before adding any new fixture, verify no existing fixture already exercises the same AST paths.  If one does, extend it rather than creating a parallel one.

### Fixture attribution (`meta.txt`)

Every fixture directory whose input file was copied verbatim from an external source must contain a `meta.txt`:

```
Source: https://github.com/foo/bar/blob/<full-40-char-SHA>/path/to/file
License: <Debian well-known short name>  (Expat, Apache-2.0, GPL-2, GPL-3, AGPL-3, …)
```

Use the full 40-character commit SHA from `origin/main` or `origin/master` — never a branch ref, never a local-only commit.  Add a `Note:` line for anything needing clarification (e.g. the file is a snapshot of an older version, or it is shared verbatim between multiple upstream projects).

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
testutil.Golden(t, "testdata/format", "input.jq", []testutil.Case{
    {Out: "output.json", Fn: func(src string) (string, error) {
        f, err := jq.ParseFile(src)
        if err != nil { return "", err }
        b, err := json.MarshalIndent(f.MarshalAST(), "", "\t")
        if err != nil { return "", err }
        return string(b) + "\n", nil
    }},
})
```

Any regression in field names, nesting, or ordering produces a readable diff.  Packages: `jq`, `shell`, `dockerfile`, `markdown`.

## Architecture

- **One package per language**: `jq/`, `shell/`, `dockerfile/`, `markdown/`, `template/`
- **Shared utilities in `internal/`** — never copy helpers across packages; extract to `internal/` instead
- **Testable entry point**: the `cmd/` binary exposes `run(args []string, stdin, stdout, stderr) int` so the CLI can be integration-tested without subprocess overhead; `stdout`/`stderr` are `io.Writer` parameters so tests can capture output without subprocesses
- **`string` in, `(string, error)` out for formatters**: all language formatters accept source text as `string` and return `(string, error)`.  The `error` return must be present even if the formatter is currently infallible — dropping it would require a breaking API change the moment any failure mode is added.  Formatters require the full source in memory (you cannot stream-format a jq expression one token at a time), and source files are small — `io.Reader` would add complexity with no benefit
- **Consistent language-package API** — every language package must expose these entry points with consistent names:
  - `Format(src string) (string, error)` — combined parse+format.  Packages that need extra parameters (e.g. `shell` needs `lang syntax.LangVariant`) add them; the name and the `error` return are non-negotiable.
  - `MarshalFile` — marshal a file with the filename embedded, returning `(any, error)`.  Packages with a distinct `*File` AST type: `MarshalFile(f *File, filename string) (any, error)`.  Packages without one (e.g. `markdown`): `MarshalFile(src, filename string) (any, error)`.  The error is currently always nil for valid input, but the slot must exist for the same reason as `Format` — future failure modes must not require a breaking API change.
  - Any outlier — wrong name, missing entry point, missing `error` return — is a code smell.  The `cmd/` dispatch functions must not contain per-package special cases that exist solely because of API divergence.  All known API gaps have been closed; if new packages or entry points are added, they must conform immediately.
- **Single dispatch enum** (`fileKind`) — when the same set of file types is switched on in multiple places, consolidate into one enum and one set of helper functions; parallel switches are a maintenance hazard
- **No callback parameters to defer imports** — if a function takes a `func(...)` parameter whose only purpose is to let the caller inject a dependency from another package, that is deferred-import avoidance, not real abstraction.  Before accepting such a parameter, verify that a direct import would create a circular dependency.  If no cycle exists, delete the callback and import directly.  Every such callback that has existed in this codebase (`JQFmt`, `RUNShellFmt`, the `jqFmt func(expr string, inline bool) string` threading through `shell.Format`/`FormatRUN`) turned out to be unnecessary after checking for cycles.

## Formatter integrity — no silent pass-through

**A formatter must never silently pass through content it failed to format.**

If `format(x) == x` for some input, it must be because `x` is already in canonical form — not because the formatter gave up.  A verbatim fallback (returning the raw input when formatting fails) is indistinguishable from correct output: the diff is empty, the idempotency check passes, and the bug hides forever.

**Rule: every formatter code path that produces output must have formatted the content.**  Any path where a formatting attempt failed and the raw input is returned verbatim is a bug.

Implementation consequences:
- Functions that receive a formatted string as input must assert it is non-empty.  Use `panic(...)` as a structural guard rather than silently falling through.  A panic on malformed-formatter output is the correct behavior: it surfaces the bug immediately in tests rather than hiding it in production.
- Formatters are for **valid** input.  If the formatter cannot handle some input, the fix is either (a) fix the formatter's handling of that valid input, or (b) confirm the input is genuinely invalid and document why the formatter is not responsible for it.  Returning the input unchanged is never the right answer.
- When adding a new code path, ask: "can this path return the raw input unchanged?"  If yes, replace it with a formatter fix or a panic.

This rule was hard-won from the jq-template formatter, where a verbatim fallback for "unparseable fragment blocks" masked the real bug (the fragments were never assembled together the way `jq-template.awk` does).  Once the fallback was replaced with a `panic`, the real bugs became immediately visible and fixable.

## Parse error format

**Parse errors must carry position information.**

Every error returned by a `Parse` or `Format` function must include at minimum a 1-based line number; column is included when the parser tracks byte offsets.  The canonical format is `line:col: message` (consistent with mvdan/sh and Go's `go/token` convention).  A bare error message with no position is not acceptable — it forces callers to scan the source themselves.

Concrete requirements per package:
- **jq**: `jq parse error at LINE:COL: message` — `errorfAt(t Token, ...)` always called with the token at fault; `columnOf` derives the 1-based column from the token's byte offset and the source string.
- **shell**: position provided natively by mvdan/sh — `shell parse: LINE:COL: message`.
- **dockerfile**: `dockerfile parse error at line LINE: message` — line number from the parser's `p.pos` counter (1-based).
- **template**: `template: unterminated {{ block at line LINE` — line counted by `strings.Count(src[:openPos], "\n") + 1`.
- **markdown**: no invalid syntax exists by spec — `Format` and `MarshalFile` always return `nil` error.

## Code quality

- **DRY**: proactively look for duplication and dead code; eliminate before adding new code
- **Builtins over helpers**: Go 1.21+ provides `min`/`max` builtins — do not write local equivalents; remove any that exist
- **Exhaustive switches**: for AST node types and `fileKind`, use compile-time-checked exhaustive switches so new variants cause build failures, not silent no-ops
- **American English spelling**: `normalize` not `normalise`; `color` not `colour`; etc.
- **Compiler enforcement of interface implementation**: Every type that claims to implement an interface gets a compile-time assertion adjacent to the type definition
- **Avoid premature interfaces**: Go interfaces with a single canonical implementation are painful to traverse in a codebase — every call requires an extra indirection. Use interfaces only when: (1) there are multiple concrete implementations, or (2) the interface defines a contract for external implementers.

## CLI flags

- `--tidy` behavior: see `README.md` and the implementation — do not duplicate the rewrite list here
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

## Definition of done

A formatter change is not done until **all** of the following are true:

1. **Tests pass** — `go test -coverprofile=... -coverpkg=./... ./...` green, no regression in coverage.
2. **Corpus/anticorpus fixtures** — the new behaviour is demonstrated by at least one golden fixture sourced from `../corpus/` or `../anticorpus/`.  Contrived fixtures are acceptable only when the corpus contains no relevant example.  No new `func TestXxx` was added when a golden fixture could serve the same purpose.
3. **Docs updated** — the relevant file in `docs/` documents the new rule/behaviour.  For shell formatter changes: `docs/bash.md`.  For jq formatter changes: `docs/jq.md` and/or `docs/jq-sh.md`.  For Dockerfile changes: `docs/dockerfile.md`.  For markdown changes: `docs/markdown.md`.
4. **Idempotency** — running the formatter twice on any affected input produces identical output (`Idem: true` in the golden test, or verified manually).
5. **No verbatim fallback introduced** — the change does not add any code path where the formatter returns raw input unchanged when formatting fails.  See *Formatter integrity — no silent pass-through* above.  If a new code path cannot always produce formatted output, it needs a `panic` assertion, not a silent fallback.

If any of these is missing, the task is not done.  Finish all five before reporting completion.

## LLM guidance

- **No logging in library code.** Library packages (`jq/`, `shell/`, `dockerfile/`, `markdown/`, `template/`) must never write to stderr or stdout directly.  Return errors; let the caller (`cmd/`) decide how to surface them.  If a library function genuinely needs to produce output, accept an `io.Writer` parameter — never write to `os.Stdout` or `os.Stderr` directly.
- **No `context.Background()` inside library code.** If a function signature takes a `context.Context`, require it from the caller.  Do not paper over a missing context with `context.Background()` or `context.TODO()` inside library functions.
- **No out-of-scope TODOs.** Do not add `TODO` comments for things the current task does not require.  The existing rule still applies: any `TODO` that does exist must be ALL-CAPS with a concrete, specific description.
- **Do not change `go.mod` or `go.sum`** unless the task explicitly requires a new dependency.  Prefer stdlib alternatives; check whether the standard library already covers the use case before reaching for an import.
- **Match the existing error style in the file you are editing.** If the file uses `errors.New`, do not introduce `fmt.Errorf`, and vice versa.
- **Return the concrete type, not an interface**, unless the function is part of an existing API that already returns an interface.
- **Do not strip error information.** Use `%w` not `%v` when wrapping errors; `%v` loses the original error type and breaks `errors.Is`/`errors.As` for callers.
- **Prefer `t.Fatal` over `t.Error` + `return`** in tests when continuing after a failure would cause a nil dereference or meaningless subsequent failures.

## Build system

Use the standard Go toolchain directly — `go build`, `go test`, `go vet`, `go mod tidy`.  Do not introduce a Makefile: it is a parallel, inferior build system on top of one that already works, and it hides what is actually running.  For multi-step release or maintenance tasks, write a small Go program under `cmd/` instead.

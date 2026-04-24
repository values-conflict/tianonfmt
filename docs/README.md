# tianonfmt style documentation

This directory documents the formatting conventions Tianon uses for each file type found in the corpus.  These files are the authoritative reference for what `tianonfmt` should produce.

The documentation is deliberately exhaustive — it captures not only what Tianon *does* do, but also notable things he *doesn't* do, since both kinds of patterns are valuable when building or auditing a formatter.

## File index

| File | Covers |
|------|--------|
| [bash.md](bash.md) | Standalone Bash/shell scripts (`.sh` files and shebang-identified scripts) |
| [jq.md](jq.md) | Standalone jq source files (`.jq`) |
| [jq-sh.md](jq-sh.md) | jq expressions embedded inside shell scripts (`jq '...'` invocations) |
| [jq-template.md](jq-template.md) | `Dockerfile.template` files — Tianon's jq-template format |
| [dockerfile.md](dockerfile.md) | Plain `Dockerfile` and `Dockerfile.*` files |
| [yaml.md](yaml.md) | YAML files, primarily GitHub Actions workflows |
| [json.md](json.md) | JSON data files (`versions.json`, etc.) |
| [go.md](go.md) | Go source code (beyond what `gofmt` enforces) |
| [perl.md](perl.md) | Perl scripts and modules |
| [awk.md](awk.md) | AWK scripts |

## Cross-cutting principles

A few things are true across every file type:

- **Hard tabs for indentation in all non-web languages.**  Shell, jq, Dockerfile continuation lines, Makefile, Go, AWK, and Perl all use hard tabs.  The sole exceptions are YAML (2 spaces, required by spec) and JSON (2 spaces, conventional).

- **No trailing whitespace** on any line, in any file type.

- **Files end with a single newline** — no blank line at the end, no missing terminator.

- **Comments are always meaningful.**  Tianon writes comments that explain *why* something is done, not *what* the code does.  TODO comments are common and concrete.

## Embedding relationships

Several of these formats appear inside one another.  When that happens:

- **Shell inside Dockerfile `RUN`** → see [dockerfile.md §RUN shell style](dockerfile.md#run-shell-style) and [bash.md](bash.md) for the shared conventions; [dockerfile.md](dockerfile.md) documents what changes.
- **Shell inside YAML `run:`** → see [yaml.md §Shell in run steps](yaml.md#shell-in-run-steps) and [bash.md](bash.md); format is identical to standalone shell except for the YAML indentation context.
- **jq inside shell** → see [jq-sh.md](jq-sh.md) for the specific embedding conventions; [jq.md](jq.md) documents standalone jq style.
- **jq inside Dockerfile.template** → see [jq-template.md](jq-template.md); the jq style matches [jq.md](jq.md) when multi-line, and is compact when inline.

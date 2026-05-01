# tianonfmt

An opinionated formatter for jq, shell scripts, Dockerfiles (including `Dockerfile.template` files), and Markdown — enforcing my personal style conventions across my Docker Official Images work, personal tools, and infrastructure scripts.

## Supported types

File type is detected from the path extension or name:

| Extension / filename | Format |
|---|---|
| `.jq` | jq |
| `Dockerfile`, `Dockerfile.*` | Dockerfile |
| `Dockerfile.template` (with `{{ }}`) | jq-template |
| `.sh` or bash/sh shebang | shell |
| `.md` | Markdown |

For stdin (or files with unknown extensions), detection falls back to content heuristics: shebang → shell, `FROM` → Dockerfile, `{{` → jq-template, everything else → jq.

## Usage

With no file arguments, reads from stdin and writes to stdout:

```console
$ tianonfmt < script.sh
$ tianonfmt < filter.jq
```

With file arguments and no flags, prints formatted output to stdout without modifying the files:

```console
$ tianonfmt Dockerfile script.sh filter.jq
```

`-w` writes formatted output back to source files and prints the names of files that changed:

```console
$ tianonfmt -w Dockerfile script.sh *.jq
```

`-d` prints a unified diff for each file that would change and exits non-zero if any diffs are found:

```console
$ tianonfmt -d script.sh
```

`-t` (`--tidy`) applies idiomatic rewrites beyond basic formatting:

- shell: `#!/bin/bash` → `#!/usr/bin/env bash`; `|| true` → `|| :`; `which` → `command -v`
- Dockerfile `RUN`: `&&`-chained commands → `set -eux; semicolon; separated` form

```console
$ tianonfmt -t -w script.sh Dockerfile
```

`-p` (`--pedantic`) implies `--tidy` and fails (exit 1) if any constructs remain that I consider Wrong.  Combine with `-d` to see exactly what needs changing:

```console
$ tianonfmt -p -d script.sh
```

## Why?

Formatters are great — `gofmt` is the obvious example — but they only exist where someone bothered to write one, and they only enforce the conventions of whoever wrote them.  I have strong opinions about how my code should look, and I got tired of those opinions living only in my head (or surfacing only in code review comments).

`tianonfmt` makes "run the formatter" the answer to style questions.

Style documentation lives in [`docs/`](docs/).  None of it is exhaustive, and some of it is probably wrong — but it's interesting.

## Attribution

The research, corpus analysis, style documentation, and initial implementation were produced by "claude my eyes right out".  Tianon was harmed in the creation of this tool.

# YAML style

Covers `.yml` and `.yaml` files in the corpus.  The `.yml` extension is always used -- `.yaml` does not appear.

The corpus contains two distinct YAML contexts: **GitHub Actions workflows** (the majority) and **Docker Compose / Swarm stack files** (a small number).  Most of this document covers GitHub Actions.  Docker Compose files follow the same generic YAML conventions but may use additional constructs (like flow sequences) that do not appear in GHA workflows.

## Generic YAML conventions

These apply regardless of the YAML file's purpose.

### Indentation

**2 spaces per level.**  Tabs are forbidden by the YAML specification -- this is the one format where hard tabs cannot be used.  2 spaces is the standard choice, not a preference.  See [universal.md](universal.md).

### Quoting

- **Unquoted** for simple alphanumeric values, version strings, and most GitHub Actions expressions
- **Single-quoted** for strings containing YAML special characters or that should be treated as literals: `'bash -Eeuo pipefail -x {0}'`
- **Double-quoted** only when the string contains a single quote or requires escape sequences
- Booleans (`true`, `false`) and numbers are always **unquoted**

### Comments

Comments appear on their own lines, indented to match the surrounding context.  Inline comments are used sparingly.  `TODO` comments follow the same concrete style as code comments -- see [prose.md](prose.md).

## File structure and top-level key order

The standard top-level key order for GitHub Actions workflows:

```yaml
name: GitHub CI

on:
  ...

defaults:
  run:
    shell: 'bash -Eeuo pipefail -x {0}'

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  ...
```

1. `name:` -- always present, describes the workflow
2. `on:` -- triggers
3. `defaults:` -- default run shell (always present in action workflows)
4. `concurrency:` -- cancellation policy
5. `permissions:` -- minimal permissions declaration
6. `jobs:` -- job definitions

Not all files have all sections, but when they appear, this is the order.

Corpus refs: [`actions/.github/workflows/checkout.yml`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/.github/workflows/checkout.yml), [`tianon-dockerfiles/.github/workflows/ci.yml`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/.github/workflows/ci.yml).

## `defaults.run.shell`

The shell for all `run:` steps is declared globally and is always:

```yaml
defaults:
  run:
    shell: 'bash -Eeuo pipefail -x {0}'
```

The value is single-quoted.  The `-x` flag (trace execution) is included -- this means every `run:` step echoes its commands to the log.  This is deliberate: it makes CI logs self-documenting.

The `{0}` placeholder is GitHub Actions syntax for the script file path.

Corpus refs: [`actions/.github/workflows/checkout.yml#L10-L12`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/.github/workflows/checkout.yml#L10-L12), [`tianon-dockerfiles/.github/workflows/ci.yml#L14-L16`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/.github/workflows/ci.yml#L14-L16).

## `on:` triggers

Each trigger is listed on its own line under `on:`:

```yaml
on:
  pull_request:
  push:
  schedule:
    - cron: 0 0 * * 0
  workflow_dispatch:
```

- `pull_request:` and `push:` without sub-keys are fine as-is (empty mapping)
- `schedule:` uses a single-item list with `cron:` (no quotes around the cron expression); the minute and hour values are deliberately off-round-numbers (e.g. `13 15` rather than `0 0`) to avoid the thundering-herd effect of every repository's scheduled job firing simultaneously, and sometimes chosen to land during the workday when the job result calls for human attention
- `workflow_dispatch:` allows manual triggering -- almost always included

When `push:` has a `branches-ignore:` filter, the branches are listed in block sequence:

```yaml
on:
  push:
    branches-ignore:
      - update-versions
      - update/*
```

Corpus refs: [`actions/.github/workflows/checkout.yml#L3-L8`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/.github/workflows/checkout.yml#L3-L8), [`tianon-dockerfiles/.github/workflows/ci.yml#L3-L10`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/.github/workflows/ci.yml#L3-L10).

## `concurrency`

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

Always uses this exact structure: `group` with `github.workflow` and `github.ref`, `cancel-in-progress: true` (boolean, not quoted).

For workflows with a fixed group name (update workflows where parallel runs must be serialised):

```yaml
concurrency:
  group: update
  cancel-in-progress: true
```

## `permissions`

Minimal permissions are declared explicitly:

```yaml
permissions:
  contents: read
```

Additional permissions are added only as needed (`contents: write`, `packages: write`, etc.).

## Jobs

Job names are written as short kebab-case identifiers.  A display `name:` is added when the job ID would be unclear to a human reader:

```yaml
jobs:

  generate-jobs:
    name: Generate Jobs
    runs-on: ubuntu-latest
```

Note the **blank line after `jobs:`**: a visual separator before the first job definition.  This blank line appears consistently in multi-job workflow files.

### `runs-on`

Common values:
- `ubuntu-latest` (most common)
- `ubuntu-24.04`, `ubuntu-22.04` (when a specific version is needed)
- `${{ matrix.runner }}` (when runner is a matrix variable)

### `outputs`

Job outputs use the `${{ steps.id.outputs.key }}` reference pattern:

```yaml
outputs:
  strategy: ${{ steps.generate-jobs.outputs.strategy }}
```

### `steps`

Steps use `uses:` for actions and `run:` for shell commands.  The `name:` field is included when the step's purpose isn't immediately obvious from the `uses:` or `run:` content.

Steps without a `name:` are common for short, self-explanatory commands:

```yaml
- run: ls -laFh
- run: git log --oneline
```

**When to omit `name:`:** a step's `run:` stands as its own name when it is the kind of command a developer would type directly or see in a project `README` -- a single, standard tool invocation with familiar flags.  When `run:` involves unusual flags, cross-module plumbing (`-C`, `../...`), or phrasing that would require a reader to pause and parse, add a `name:` that states what the step is actually doing.

When using `name:`, the name is the exact thing being done -- the command, the flag set, or the output artifact -- not an abstract category:

```yaml
# Good: exact command or flag set
- name: staticcheck
  run: go tool -C .go-tools staticcheck ../...

- name: go test
  run: go test -coverpkg=./... -race -count=1 ./... -args -test.gocoverdir="$GOCOVERDIR"

- name: go test -tags posix_trap
  run: go test -tags posix_trap -coverpkg=./... -race -count=1 ./... -args -test.gocoverdir="$GOCOVERDIR"

# Good: step that produces a file whose name is obvious for its purpose → use the filename as the name
- name: coverage.out
  run: go tool covdata textfmt -i "$GOCOVERDIR" -o coverage.out

- name: coverage.html
  run: go tool cover -html=coverage.out -o coverage.html

- name: upload coverage.out
  uses: actions/upload-artifact@v7
  with:
    path: coverage.out

# Avoid: category labels that hide what actually runs
- name: lint
  run: go tool -C .go-tools staticcheck ../...

- name: coverage report
  run: go tool cover -html=coverage.out -o coverage.html
```

**Separate mutation from assertion.**  When one step modifies state (formats files, tidies modules) and a following step asserts the result (`git diff --exit-code`), keep them as separate unnamed steps rather than joining with `&&`.  Each step does one thing; failure is attributed to the right step; the diff output is visible in its own step log with shell tracing (`-x`):

```yaml
- run: gofmt -s -w .
- run: git diff --exit-code

- name: go mod tidy
  run: |
    go mod tidy
    go -C .go-tools mod tidy
    go -C .go-tools work sync
- run: git diff --exit-code
```

### `uses:` -- action references

Action references always pin to a specific version tag (e.g., `@v6`), never `@main` or `@HEAD`:

```yaml
- uses: actions/checkout@v6
- uses: tianon/bashbrew@tianon
```

Pinned versions are kept current: bump to a newer tag when a new version is available and the bump does not cause workflow breakage or loss of fidelity in the action's output.

Local actions (within the same repository) use relative paths:

```yaml
- uses: ./checkout
- uses: ./.github/tmp/checkout
```

## Shell in run steps

### Never use `${{ }}` inside `run:` blocks

GitHub Actions expressions (`${{ }}`) inside `run:` blocks are **almost never correct** and should be avoided.  They create two compounding problems:

**Security**: the expression value is interpolated directly into the shell command string before the shell sees it.  An attacker-controlled value (e.g. a PR branch name, a commit message, a label) containing shell metacharacters becomes a command injection vulnerability.  GitHub's own security hardening guide explicitly warns against this pattern.

**Shell quoting**: the expression value arrives as a raw string fragment embedded in YAML, then in shell.  Quoting it correctly for both layers simultaneously is error-prone and context-dependent.  There is no safe general solution.

The correct pattern is to pass any expression value through `env:` and reference it as a plain shell variable:

```yaml
# Wrong -- expression injected into shell:
- run: echo '${{ steps.build.outputs.digest }}'

# Correct -- value passed safely via environment:
- env:
    DIGEST: ${{ steps.build.outputs.digest }}
  run: echo "$DIGEST"
```

The runner assigns `env:` values before the shell starts, treating them as data rather than code.  Shell variables are never interpreted as commands.

**When `${{ }}` IS appropriate**: outside of shell code, expressions are fine and expected: `runs-on:`, `if:`, `with:`, `env:` values at step/job level, `concurrency.group`, `strategy:` fields, `outputs:` at job level.  The rule applies specifically to the content of `run:` blocks and any other context where values are interpolated into shell source.

**The same principle applies to any action input that accepts code**, not just `run:`.  The most common case beyond `run:` is `actions/github-script`'s `script:` input, which accepts JavaScript.  The correct pattern is identical -- pass via `env:`, read as `process.env.VAR`:

```yaml
# Wrong -- expression injected into JavaScript source:
- uses: actions/github-script@v8
  with:
    script: |
      const n = ${{ github.event.pull_request.number }};

# Correct -- value passed safely via environment:
- uses: actions/github-script@v8
  env:
    PR_NUMBER: ${{ github.event.pull_request.number }}
  with:
    script: |
      const n = Number(process.env.PR_NUMBER);
```

Corpus ref (correct pattern): [`doi/official-images/.github/workflows/munge-pr.yml`](https://github.com/docker-library/official-images/blob/master/.github/workflows/munge-pr.yml) -- `IMAGES: ${{ needs.gather.outputs.images }}` passed via `env:`, consumed as `process.env.IMAGES` inside the `script:` block.

### Single-line commands

Single commands are written directly without `|`:

```yaml
- run: ls -l checkout/checkout.sh .git
- run: ${{ matrix.runs.prepare }}
```

### Multi-line commands

Multi-line commands use the YAML literal block scalar `|`:

```yaml
- run: |
    mkdir -p .github/tmp
    wget -O .github/tmp/actions.tgz "https://github.com/$GITHUB_REPOSITORY/archive/$GITHUB_SHA.tar.gz"
    tar --extract --verbose --file .github/tmp/actions.tgz ...
```

The shell code inside `|` blocks follows the same conventions as standalone [bash scripts](bash.md):
- 2-space YAML indentation is separate from the shell's tab indentation -- the shell code inside a `run: |` block uses tabs for its own indentation, but the entire block is indented 4 spaces (2 for step list, 2 for the `run:` value)
- `if`, `for`, `case` structures follow [bash.md](bash.md) conventions exactly

The folded block scalar `>` is **never used** for shell code.

### jq inside `run:` steps

jq invocations inside workflow shell steps follow [jq-sh.md](jq-sh.md) conventions exactly.  The only addition is the `$GITHUB_OUTPUT` pattern for passing values between steps:

```yaml
- run: |
    strategy="$(
      find -name versions.json -exec dirname --zero '{}' + \
        | jq -rcsR '
          split("\u0000")
          | map(ltrimstr("./"))
          - ["", empty]
          | sort
          | { matrix: { dir: . }, "fail-fast": false }
        '
    )"
    EOF="EOF-$RANDOM-$RANDOM-$RANDOM"
    echo "strategy<<$EOF" >> "$GITHUB_OUTPUT"
    jq <<<"$strategy" . | tee -a "$GITHUB_OUTPUT"
    echo "$EOF" >> "$GITHUB_OUTPUT"
```

The `EOF-$RANDOM-$RANDOM-$RANDOM` delimiter pattern is used for multi-line output values to avoid delimiter collision.

Corpus ref: [`tianon-dockerfiles/.github/workflows/update.yml#L29-L48`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/.github/workflows/update.yml#L29-L48).

## `with:` -- action inputs

```yaml
- uses: actions/checkout@v6
  with:
    fetch-depth: 3
    clean: false
```

Boolean values are unquoted (`true`, `false`).  Numbers are unquoted.  Strings are unquoted when they contain no special YAML characters.

## `strategy:` and `matrix:`

```yaml
strategy:
  matrix:
    runner:
      - ubuntu-24.04
      - ubuntu-22.04
  fail-fast: false
```

`fail-fast: false` appears consistently -- Tianon prefers to see all jobs complete rather than cancelling on the first failure.

`fail-fast: false` always comes **at the end** of the `strategy:` block, after `matrix:`. The matrix definition is the primary content; `fail-fast` is a modifier and reads naturally as a postscript.

Dynamic matrix from a previous job:

```yaml
strategy: ${{ fromJson(needs.generate-jobs.outputs.strategy) }}
```

The `include:` pattern for heterogeneous matrix items:

```yaml
strategy:
  matrix:
    include:
      - name: 'Build amd64'
        runs:
          build: './build.sh amd64'
```

## `env:` -- environment variables

At job or step level:

```yaml
env:
  dir: ${{ matrix.dir }}
  branch: update/${{ matrix.dir }}
```

Values that are GitHub Actions expressions are unquoted.  Values that need to be strings are single-quoted when they contain special characters.

## String quoting in YAML

- **Unquoted** for simple alphanumeric values, version strings, and most GitHub Actions expressions
- **Single-quoted** for strings that contain YAML special characters or that should be treated as literal strings: `'bash -Eeuo pipefail -x {0}'`
- **Double-quoted** only when the string contains a single quote or requires escape sequences
- Booleans (`true`, `false`) and numbers are always **unquoted**

The key insight: if a string looks like it might be interpreted as something other than a string (contains `{`, `}`, `:`, `#`, `@`, etc.), it gets quoted.

## Comments

Comments appear on their own lines, indented to match the surrounding context:

```yaml
      # ideally, this would just be "uses: ./checkout"...
      - name: checkout-checkout
```

```yaml
    # https://docs.github.com/en/actions/writing-workflows/...
    - ubuntu-24.04
```

Inline comments (on the same line as a value) are used sparingly:

```yaml
      - cron: 13 15 * * *
        timezone: America/Los_Angeles   # ~3:13pm PT
```

`TODO` comments in YAML follow the same concrete pattern as elsewhere in the corpus:

```yaml
          # TODO - windows-2025
          # TODO - windows-2022
```

## `if:` conditionals on steps

Step conditionals use GitHub Actions expression syntax. **`if:` is always the last field in a step**, after `name:`, `id:`, `uses:`/`run:`, `with:`, and `env:`. This ordering puts the action first (what the step does) and the condition last (whether it runs), matching the natural reading order.

```yaml
# Good: if: last
- name: coverage.out
  run: go tool covdata textfmt -i "$GOCOVERDIR" -o coverage.out
  if: matrix.go == 'stable'

# Bad: if: before run:
- name: coverage.out
  if: matrix.go == 'stable'
  run: go tool covdata textfmt -i "$GOCOVERDIR" -o coverage.out
```

Examples:

```yaml
- name: Single Branch
  if: always()
```

```yaml
- name: Push
  if: github.event_name == 'push' && github.ref == 'refs/heads/main'
```

## `id:` on steps

Step IDs are assigned to steps whose outputs are referenced:

```yaml
- id: generate-jobs
  name: Generate Jobs
  run: |
    ...
    echo "strategy=$strategy" >> "$GITHUB_OUTPUT"
```

IDs are kebab-case, matching the step's logical purpose.

## Notable omissions

- `.yaml` extension -- always `.yml`
- `true` / `false` for booleans as strings (never `'true'` or `"true"`)
- `null` values -- absent keys are omitted rather than set to null
- Tab indentation -- YAML requires spaces; 2 spaces specifically
- `on: push` triggering `main` branch specifically -- all branches trigger unless filtered with `branches-ignore`

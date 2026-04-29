# Bash / shell script style

Covers standalone `.sh` files and executable scripts identified by a bash or sh shebang.  For jq expressions that appear inside these scripts, see [jq-sh.md](jq-sh.md).  For shell code embedded in Dockerfile `RUN` instructions, see [dockerfile.md §RUN shell style](dockerfile.md#run-shell-style) — that context has different conventions.

See [universal.md](universal.md) for the general indentation and inline-vs-multiline principles.

---

## File preamble

### Shebang

Always `#!/usr/bin/env bash`.  Never `#!/bin/bash` even when `/bin/bash` is guaranteed to exist.

```bash
#!/usr/bin/env bash
```

Corpus refs: [`actions/checkout/checkout.sh#L1`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/checkout/checkout.sh#L1), [`tianon-dockerfiles/buildkit/versions.sh#L1`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L1), [`debian-bin/repo/buildd.sh#L1`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L1).

### `set` options

The very next line after the shebang is always:

```bash
set -Eeuo pipefail
```

The flags appear in this exact order, combined into a single argument.  Breaking them out (`set -E -e -u -o pipefail`) is never done.

- `-E` — ERR trap is inherited by shell functions and subshells
- `-e` — exit immediately on non-zero exit status
- `-u` — treat unset variables as an error
- `-o pipefail` — a pipeline fails if any command in it fails

A `-x` flag is **not** added globally.  When tracing is needed for a specific block, `set -x` / `set +x` pairs are used inline around that block only.

Corpus ref: [`actions/checkout/checkout.sh#L1-L2`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/checkout/checkout.sh#L1-L2), [`debian-bin/repo/buildd.sh#L1-L2`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L1-L2).

### `shopt` options

When needed, `shopt` calls appear near the top of the script, immediately after `set`:

- `shopt -s dotglob` — include dotfiles in glob patterns
- `shopt -s nullglob` — return empty list (not the literal pattern) when a glob matches nothing
- `shopt -s globstar` — enable `**` for recursive glob matching across directory levels

These appear near the top because they change the semantics of glob expansion throughout the entire script.  If a glob option is truly necessary, it must apply consistently — having different glob behavior in different places is already a footgun for unexpected behavior.  Placing them at the top makes the script's behavior uniform and makes it clear to readers that globs behave non-standardly.

`globstar` specifically enables the `**/` pattern for recursive directory traversal without `find`, which avoids `find`'s notoriously awkward output-feeding behaviour.  Combined with an array, this is the idiomatic pattern:

```bash
shopt -s globstar
files=( **/**.go go.mod go.sum )
```

Inside a loop, `for f; do` (without `in`) iterates the script's own positional arguments — equivalent to `for f in "$@"; do` but idiomatic shorthand:

```bash
shopt -s globstar
for go in **/**.go go.mod go.sum; do
    for f; do   # iterates $@ (the script's own positional args)
        if [ "$go" -nt "$f" ]; then exit 0; fi
    done
done
```

Corpus ref: [`meta-scripts-cosine/.any-go-nt.sh`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/.any-go-nt.sh).

### Vim modelines for ambiguous file types

See [universal.md §Vim modelines](universal.md#vim-modelines) for the general rule.  In bash scripts specifically, the modeline goes on the second line, immediately after the shebang:

```bash
#!/usr/bin/env hocker
# vim:set ft=sh:
```

Corpus refs: [`home/git-config.d/common`](https://github.com/tianon/home/blob/720c476e79a50ab0dd133f7187bd046b32cd5b73/git-config.d/common), the `# vim:set ft=sh:` pattern in executable scripts without `.sh` extension.

---

## POSIX compatibility as a design goal

Tianon's Bash scripts are consciously written so that they remain **mostly reasonable to read and write as POSIX shell** without requiring a style change.  This is an explicit goal — the control structures, quoting conventions, and general idioms are chosen to be familiar to anyone reading plain `sh`.

When "POSIX shell" is the target, that means **BusyBox `sh`** in practice (Alpine, Docker scratch-adjacent images), and occasionally Debian's `dash`.  `local` is not technically POSIX but is supported by both BusyBox and dash, which is why it is used freely even in nominally-POSIX contexts.

The POSIX specification itself supports tabs as the canonical shell indentation character.  The `<<-` heredoc operator strips *leading tab characters* specifically — from [POSIX.1-2024 §2.7.4](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/V3_chap02.html#tag_19_07_04):

> If the redirection operator is `<<-`, all leading `<tab>` characters shall be stripped from input lines… and from the line containing the trailing delimiter.

This operator exists precisely so that heredoc content can be indented with the surrounding code.  It would serve no purpose if shell scripts were indented with spaces.  The spec is effectively endorsing tabs as the canonical indentation character.  See also [universal.md](universal.md) for the general principle.

The Bash-specific features that *are* used deliberately:

- **Arrays** (`variants=( ... )`, `${array[@]}`) — considered worth the Bash dependency
- **`$BASH_SOURCE`** — required for correct self-location when a script may be `source`d
- **`set -Eeuo pipefail`** — `-E` and `-o pipefail` are Bash extensions; `-E` in particular makes trap inheritance reliable
- **Here-strings** (`<<<`) — cleaner than a pipe from `echo` and avoids a subshell
- **`shopt`** — for `dotglob`, `nullglob`, etc. when needed
- **`local`** — used freely (supported by BusyBox and dash; see above)

Features *avoided* even in Bash contexts, to preserve POSIX readability:

- `[[ ]]` — appears occasionally but `[ ]` is strongly preferred; `case` is used as the POSIX alternative for pattern matching (see below)
- `$'...'` quoting — used only for embedding literal escape sequences (`$'\n'`, `$'\t'`), not as a general quoting form

When a script genuinely requires Bash features, Bash is explicitly installed even in minimal-shell environments like Alpine — but this is a conscious tradeoff, not a default.

---

## Naming and quoting

### Variable naming conventions

**`SCREAMING_SNAKE_CASE`** for variables that are conventional Unix environment variables or carry a namespace prefix:

```bash
export PATH='/usr/sbin:/usr/bin:/sbin:/bin'
export TZ='UTC' LC_ALL='C'
export BASHBREW_NAMESPACE='tianon'
export TIANON_PYTHON_FROM_TEMPLATE='python:%%PYTHON%%-alpine3.23'
```

**`camelCase`** for multi-word script-specific variables, whether local or exported:

```bash
dpkgArch="$(dpkg --print-architecture)"
exitTrap="$(printf 'rm -rf %q' "$dir")"
buildArgs=()
export dpkgArch    # exported to subshells/jq, but still camelCase
```

**`lowercase`** for short single-concept variables:

```bash
version="1.2.3"
json='{}'
suite="trixie"
dir="$(dirname "$BASH_SOURCE")"
```

The distinction is not simply "exported vs local" — `dpkgArch` is exported but stays camelCase because it is a script-specific variable, not a conventional environment variable name.

### Variable quoting

**Double-quote every variable reference** unless there is a specific reason not to.  Bare `$var` references are essentially never seen outside of arithmetic contexts.

```bash
echo "$message"
cp "$src" "$dst"
[ "$#" -eq 0 ]
```

Parameter expansion syntax uses the full `${var}` form only when necessary for disambiguation (`${var}suffix`) — simple references use `$var` or `"$var"`.

### Command substitution

**Always `$()`**, never backticks.  Backtick substitution does not appear anywhere in the corpus.

```bash
dir="$(dirname "$BASH_SOURCE")"
uid="$(id -u)"
```

Nesting is done naturally without escaping:

```bash
cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"
```

### Script self-location

To find the directory a script lives in, `$BASH_SOURCE` is used — never `$0`:

```bash
dir="$(readlink -ve "$BASH_SOURCE")"
dir="$(dirname "$dir")"
```

`readlink -ve` is preferred over `realpath` because it is more portable and gives a clearer error if the path does not exist.

`$0` is acceptable in pure POSIX shell scripts and in usage/help text at the top level of a Bash script — it more closely reflects what the user actually typed.  `$BASH_SOURCE` is preferred everywhere else in Bash because it remains correct when the script is `source`d.

Corpus refs: [`tianon-dockerfiles/buildkit/versions.sh#L6-L7`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L6-L7), [`debian-bin/repo/buildd.sh#L90`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L90).

---

## Control structures

### `if` / `then` / `elif` / `else` / `fi`

`then` is **always on the same line** as the condition when both fit on one line.  It is never alone on the next line in the simple case:

```bash
if [ "$#" -eq 0 ]; then
    fatal_usage "expected at least one argument"
fi

if [ "$distro" = 'debian' ]; then
    ...
elif [ "$distro" = 'ubuntu' ]; then
    ...
else
    ...
fi
```

When the condition itself is too long and uses `\` continuation, `; then` moves to its own line at the same indentation as the conditions:

```bash
if ! docker-sbuild \
        --mount "type=bind,src=$dscDir,dst=/dsc,ro" \
        --workdir /dsc \
        "$targetDirectory" \
    ; then
    # ...
fi
```

`fi` is always on its own line at the same indentation as `if`.

POSIX `[ ]` test brackets are used; `[[ ]]` bash-specific tests appear but are not preferred — `case` is usually the POSIX-compatible alternative for pattern matching.

Corpus refs: [`debian-bin/repo/buildd.sh#L35-L55`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L35-L55), [`debian-bin/repo/buildd.sh#L115-L126`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L115-L126).

### `case` / `esac`

```bash
case "$suite" in
    debian-* | ubuntu-*)
        distro="${suite%%-*}"
        distroSuite="${suite#$distro-}"
        ;;

    *)
        echo >&2 "error: ..."
        exit 1
        ;;
esac
```

- Each pattern is indented one tab from `case`
- Pattern body is indented one tab from the pattern
- `;;` terminator is on its own line at the body's indentation level
- A blank line separates distinct pattern blocks when the bodies are more than one line
- Alternatives in a pattern use ` | ` with spaces: `debian-* | ubuntu-*`
- `esac` is on its own line at the same indentation as `case`
- One-line patterns use `pattern) body ;;` on a single line: `*) echo >&2 "error: unknown variant: '$variant'"; exit 1 ;;`

When the alternative list is too long to fit on one line, each alternative goes on its own line with ` | \` at the end, except the final one which closes with ` )`:

```bash
--cap-add | \
--device | \
--entrypoint | \
--env | \
--tmpfs | \
--workdir )
    dockerArgs+=( "$1" "$2" )
    ;;
```

Corpus refs: [`tianon-dockerfiles/buildkit/versions.sh#L27-L59`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L27-L59), [`docker-bin/homes-docker#L44-L65`](https://github.com/tianon/docker-bin/blob/docker-bin/homes-docker#L44-L65).

### `for` / `while` / `done`

`do` is **always on the same line** as the loop keyword when the list fits on one line:

```bash
for variant in "${variants[@]}"; do
    ...
done
```

When the list is long and uses `\` continuation, `; do` moves to its own line:

```bash
for suite in \
    "$debian" \
    bookworm \
; do
    ...
done
```

`done` is always on its own line at the loop's indentation level.

### Functions

The `function` keyword is **never used**.  Functions are defined with the bare `name()` form:

```bash
usage() {
    local self="$0"; self="$(basename "$self")"
    echo "usage: $self ..."
}
```

- Opening brace `{` always on the same line as `name()`
- Closing brace `}` on its own line at the function's indentation level
- Local variables declared with `local` in the function body

Corpus refs: [`debian-bin/repo/buildd.sh#L4-L8`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L4-L8), [`debian-bin/repo/apt-ftparchive-generate-conf.sh#L4-L7`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/apt-ftparchive-generate-conf.sh#L4-L7).

---

## Data

### Arrays

Array declaration with the parenthesised form:

```bash
variants=(
    ''
    'rc'
    '0.16'
    '0.13'
)
```

When the array is short enough to fit clearly on one line: `sbuildArgs+=( --arch "$dpkgArch" --no-arch-all )`.

Array expansion is always quoted: `"${variants[@]}"`.

Associative arrays use `declare -A`:

```bash
declare -A files=(
    ["$HOME/.bashrc"]="source '$thisDir/bashrc'"
)
```

### Here-documents

`<<-` (with tab-stripping) is **always** used in preference to `<<`; `--pedantic` flags bare `<<` heredocs.  The `-` variant strips leading tab characters from the body and from the delimiter line, which lets the heredoc content be indented naturally with the surrounding code.  Beyond the immediate indentation benefit, using `<<-` consistently means the heredoc can be freely moved inside a deeper control structure later without having to change the operator.

```bash
cat <<-EOH
    #
    # NOTE: THIS DOCKERFILE IS GENERATED VIA "apply-templates.sh"
    #
EOH
```

The delimiter is:
- **Single-quoted** (`<<'EOF'`) when the body should not undergo parameter expansion
- **Unquoted** when variable interpolation is wanted

Corpus refs: [`docker-qemu/apply-templates.sh#L20-L28`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/apply-templates.sh#L20-L28), [`debian-bin/repo/apt-ftparchive-generate-conf.sh`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/apt-ftparchive-generate-conf.sh).

### String manipulation

Parameter expansion is strongly preferred over spawning `cut`, `sed`, or `awk` for simple string transformations.  Reasons: avoids the non-trivial overhead of shelling out to an external process, avoids data-passing awkwardness (`<<<` / `echo | ...`), and works cleanly for long chains of successive prefix/suffix stripping:

```bash
distro="${suite%%-*}"           # remove longest suffix matching -*
distroSuite="${suite#$distro-}" # remove shortest prefix
```

Case conversion: `${var,,}` for lowercase, `${var^^}` for uppercase.

---

## I/O and error handling

### Pipe placement

When a multi-line pipeline spans continuation lines, `|` goes at the **start** of each continuation line.  Continuation lines are indented one extra tab from the first command.

```bash
urls="$(
    wget -qO- 'https://www.qemu.org/download/' \
        | grep -oE 'https://download[.]qemu[.]org/qemu-([^"]+)[.]tar[.]xz' \
        | sort -ruV
)"
```

Multi-line pipelines most commonly appear inside a `$(…)` capture, though they do appear outside one when the output flows directly to stdout or a redirect.

One reason to prefer a capture over an inline pipeline is `set -e` behaviour: when `foo | bar` runs and `foo` fails, `bar` still executes and its secondary error (`bar: unexpected EOF`, etc.) can obscure the real failure.  A capture makes each step's failure independently visible.  This motivates a broader pattern: capture output into a variable first, then feed subsequent commands via here-string:

```bash
json="$(curl -fsSL "$url")"
count="$(jq <<<"$json" '.results | length')"
```

Note: when the capture itself will produce a lot of output, consider that `bash -x` is the primary debugging tool for shell scripts and prints captured content in full.  Large captures are best kept at the top of a script or at the beginning/end of a loop so that `bash -x` trace output remains readable.

See [jq-sh.md](jq-sh.md) for how this capture pattern applies specifically to `jq` invocations.

Corpus refs: [`docker-qemu/versions.sh#L19-L25`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/versions.sh#L19-L25), [`debuerreotype/scripts/debuerreotype-recalculate-epoch#L32-L38`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/scripts/debuerreotype-recalculate-epoch#L32-L38).

### `&&` and `||` placement

When `&&` or `||` must break across lines, the operator goes at the **start** of the continuation line:

```bash
{ wget -qO- "$packages.xz" | xz -d 2>/dev/null; } \
|| { wget -qO- "$packages.bz2" | bunzip2 2>/dev/null; } \
|| { wget -qO- "$packages.gz" | gunzip 2>/dev/null; } \
|| wget -qO- "$packages"
```

```bash
if \
    ! _check "$secmirror" "$suite-security" \
    && ! _check "$secmirror" "$suite/updates" \
; then
```

Short chains naturally stay on one line: `git remote remove origin || :`.

The corpus contains zero examples of `&&` or `||` at the end of a line before a continuation.

The null-command `|| :` is the idiomatic form for suppressing errors; `|| true` appears in contexts where other contributors may not be familiar with `:` being equivalent to `true` in shell.  Both are correct; the choice reflects audience.

Corpus refs: [`tianon-dockerfiles/.libs/deb-repo.sh#L22-L26`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/.libs/deb-repo.sh#L22-L26), [`debuerreotype/examples/debian-all.sh#L131-L135`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/examples/debian-all.sh#L131-L135).

### Error handling

**Cleanup via trap** — the standard pattern for temporary directories:

```bash
dir="$(mktemp -d -t "$prog.XXXXXX")"
exitTrap="$(printf 'rm -rf %q' "$dir")"
trap "$exitTrap" EXIT
```

`printf '%q'` safely shell-quotes the path in the trap command string.

Corpus ref: [`debian-bin/repo/buildd.sh#L86-L88`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L86-L88).

**Subshell error isolation** — `( ... ) || ...` isolates a block so errors inside don't propagate:

```bash
if ! (
    set -Eeuo pipefail
    cd "$someDir"
    eval "$shell"
); then
    echo >&2 "warning: ..."
    continue
fi
```

Corpus ref: [`debian-bin/repo/incoming.sh#L44-L53`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/incoming.sh#L44-L53).

**Error messages** always go to stderr: `echo >&2 "error: ..."` or `echo >&2 "warning: ..."`.  The prefix `error:` or `warning:` is literal text, lowercased, always followed by a colon, always active voice and specific.

**Suppressing `-x` trace around sensitive values** — when `bash -x` trace output would expose credential material, `set +x` / `set -x` pairs isolate the sensitive commands:

```bash
set +x # TODO
: "${INPUT_TOKEN:=$ACTIONS_RUNTIME_TOKEN}"
b64token="$(tr -d '\n' <<<"x-access-token:$INPUT_TOKEN" | base64 -w0)"
git config --local "http.$host/.extraheader" "Authorization: Basic $b64token"
set -x # TODO
```

The `# TODO` comments are an acknowledgement that the overall design should eventually not require this workaround — not a directive to fix the surrounding code.  See also [groovy.md §Suppressing trace for sensitive operations](groovy.md#suppressing-trace-for-sensitive-operations) for the same pattern in Jenkins pipeline shell blocks.

Corpus ref: [`actions/checkout/checkout.sh#L47-L51`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/checkout/checkout.sh#L47-L51).

### `eval` with jq output

A recurring pattern for constructing arrays or variables from JSON data:

```bash
shell="$(jq -rs '[ .[] | values[] ] | map(@json | @sh) | join(" ")' data.json)"
eval "items=( $shell )"
```

jq's `@sh` format produces shell-safely-quoted strings.  `eval` is only used with controlled jq output, never with user input.

Corpus refs: [`debian-bin/repo/buildd.sh#L91-L92`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L91-L92), [`docker-qemu/apply-templates.sh#L14-L16`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/apply-templates.sh#L14-L16).

---

## `(( ))` arithmetic — use with caution

Arithmetic compound commands interact poorly with `set -e` because their exit status reflects the *arithmetic result* (zero = failure, non-zero = success), not whether the operation succeeded:

```bash
var=0
(( var++ ))   # exits 1 — post-increment returns the OLD value (0); set -e aborts
(( var++ ))   # exits 0 — returns old value (1); fine
(( var = 0 )) # exits 1 — assigning zero is "false"; set -e aborts
```

The classic footgun is incrementing a counter that starts at zero: `(( failures++ ))` aborts the script the first time it runs.  The corpus handles this explicitly with `|| :`:

```bash
(( failures++ )) || :
```

Safe alternatives that avoid the issue entirely:

- `(( ++var ))` — pre-increment returns the *new* value (truthy when starting from 0)
- `(( var += 1 ))` — addition also returns the new value

Corpus ref: [`debian-bin/repo/buildd.sh#L124`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L124).

---

## Tool integrations

### jq

See [jq-sh.md](jq-sh.md) for the full conventions.  The short summary: `jq <<<"$var" flags 'expression'` is the standard form; prefer long-form flags (`--raw-output`, `--compact-output`) over short forms.

---

## Notable omissions

Things Tianon **never** does in standalone shell scripts:

- `#!/bin/bash` or `#!/bin/sh` shebang (always `#!/usr/bin/env bash`) — `--tidy` fixes this automatically
- `set -e` alone (bare minimum with no `-u`) — `--pedantic` flags bare `set -e` and also auto-normalises it: bash scripts get `set -Eeuo pipefail`; POSIX/sh scripts get `set -eu`.  If `-x` is already present, it is preserved (`-eux` → `-Eeuxo pipefail`).  Simpler forms already containing `-u` (`set -eu`, `set -eux`) appear in Tianon's corpus for entrypoints and are not changed by `--pedantic`; only bare `set -e` is normalised.
- `which` to locate commands (`command -v` is the POSIX-correct alternative; Tianon even aliases `which='command -v'` in his own bashrc) — `--tidy` fixes flag-free `which cmd` calls automatically
- `echo -e` for escape sequences — `printf` handles escapes portably; `echo -e` is not POSIX; `--pedantic` flags this
- `echo -n` for output without a trailing newline — use `printf '%s'` instead; `--pedantic` flags this
- Backtick command substitution — always `$(...)` style; `--tidy` converts `` `cmd` `` → `$(cmd)` automatically
- The `function` keyword — always `name() { ... }` style; `--tidy` removes `function` automatically
- `set -x` at the global level — only `set -x` / `set +x` pairs locally; `--tidy` strips `-x` from top-level `set` normalisations; `--pedantic` flags any global `set` containing `-x`
- `[ ]` tests with `==` (uses `=` for POSIX string comparison) — `--tidy` converts `[ ... == ... ]` → `[ ... = ... ]` automatically
- Arithmetic with `let` or `$((var = expr))` assignment — use `$((...))` or `var=$((expr))`; `--pedantic` flags `let`
- `declare -i` for integer-type variables — use untyped variables; `--pedantic` flags this
- `&&` or `||` at the *end* of a continuation line (always at the start of the next line) — the formatter enforces this automatically via `BinaryNextLine` mode

**Things that appear less often than you might expect:**

- `printf` for simple string output — `echo` is used for straightforward messages; `printf` appears when the format string needs escape sequences (`\n`, `\t`, `%q`) or when generating content to pipe to another command
- `$'...'` quoting — used specifically for embedding literal escape sequences (`IFS=$'\n'`, `cut -d$'\t' -f1`, multi-line GUI dialog strings); not as a general quoting form
- `|| true` as a no-op — `|| :` is idiomatic; `|| true` appears in scripts shared with audiences less familiar with `:` being equivalent to `true`
- `[[ ]]` test brackets — `[ ]` strongly preferred; `[[ ]]` appears occasionally for pattern matching (`[[ "$x" == */ ]]`) where `[ ]` cannot be used, but `case` is usually the POSIX-compatible alternative:
  ```bash
  case "$suite" in
      */security | */updates) ... ;;
  esac
  ```

# Bash / shell script style

Covers standalone `.sh` files and executable scripts identified by a bash or sh shebang.  For jq expressions that appear inside these scripts, see [jq-sh.md](jq-sh.md).  For shell code embedded in Dockerfile `RUN` instructions, see [dockerfile.md §RUN shell style](dockerfile.md#run-shell-style) — that context has different conventions.

## Shebang

Always `#!/usr/bin/env bash`.  Never `#!/bin/bash` even when `/bin/bash` is guaranteed to exist.

```bash
#!/usr/bin/env bash
```

Corpus refs: [`actions/checkout/checkout.sh#L1`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/checkout/checkout.sh#L1), [`tianon-dockerfiles/buildkit/versions.sh#L1`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L1), [`debian-bin/repo/buildd.sh#L1`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L1).

## set options

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

## Indentation

Hard tabs, one per nesting level.  No spaces for indentation anywhere in shell scripts.

## Function definitions

The `function` keyword is **never used**.  Functions are defined with the bare `name()` form:

```bash
usage() {
	local self="$0"; self="$(basename "$self")"
	echo "usage: $self ..."
}
```

- Opening brace `{` always on the same line as `name()`
- Closing brace `}` on its own line at the function's indentation level
- Local variables declared immediately with `local` at the top of the function body

Corpus refs: [`debian-bin/repo/buildd.sh#L4-L8`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L4-L8), [`debian-bin/repo/apt-ftparchive-generate-conf.sh#L4-L7`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/apt-ftparchive-generate-conf.sh#L4-L7).

## Control structures

### `if` / `then` / `elif` / `else` / `fi`

`then` is **always on the same line** as the condition.  It is never alone on the next line.

```bash
if [ "$#" -eq 0 ]; then
	fatal_usage "expected at least one argument"
fi
```

```bash
if [ "$distro" = 'debian' ]; then
	...
elif [ "$distro" = 'ubuntu' ]; then
	...
else
	...
fi
```

`fi` is always on its own line at the same indentation as `if`.

POSIX `[ ]` test brackets are used; `[[ ]]` bash-specific tests appear but are not preferred.

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
- One-line patterns (body fits on one line) use `pattern) body ;;` on a single line: `*) echo >&2 "error: unknown variant: '$variant'"; exit 1 ;;`

Corpus refs: [`debian-bin/repo/buildd.sh#L35-L55`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L35-L55), [`tianon-dockerfiles/buildkit/versions.sh#L27-L59`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L27-L59).

### `for` / `while` / `done`

`do` is **always on the same line** as the loop keyword:

```bash
for variant in "${variants[@]}"; do
	...
done
```

```bash
while true; do
	...
done
```

`done` is always on its own line at the loop's indentation level.

## Variable naming conventions

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

## Variable quoting

**Double-quote every variable reference** unless there is a specific reason not to.  Bare `$var` references are essentially never seen outside of arithmetic contexts.

```bash
# always:
echo "$message"
cp "$src" "$dst"
[ "$#" -eq 0 ]
```

Parameter expansion syntax uses the full `${var}` form only when necessary for disambiguation (`${var}suffix`) — simple references use `$var` or `"$var"`.

## Command substitution

**Always `$()`**, never backticks.  Backtick substitution does not appear anywhere in the corpus.

```bash
dir="$(dirname "$BASH_SOURCE")"
uid="$(id -u)"
```

Nesting is done naturally without escaping:

```bash
cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"
```

## Script self-location

To find the directory a script lives in, `$BASH_SOURCE` is used — never `$0`:

```bash
dir="$(readlink -ve "$BASH_SOURCE")"
dir="$(dirname "$dir")"
```

`readlink -ve` is preferred over `realpath` because it is more portable and gives a clearer error if the path does not exist.

`$0` is acceptable in pure POSIX shell scripts and in usage/help text at the top level of a Bash script — it more closely reflects what the user actually typed.  `$BASH_SOURCE` is preferred everywhere else in Bash because it remains correct when the script is `source`d.

Corpus refs: [`tianon-dockerfiles/buildkit/versions.sh#L6-L7`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L6-L7), [`debian-bin/repo/buildd.sh#L90`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L90).

## Pipe placement

When a multi-line pipeline spans continuation lines, `|` goes at the **start** of each continuation line — matching the convention in [jq.md](jq.md).  The rule follows the language being written, not the outer shell context.

```bash
urls="$(
	wget -qO- 'https://www.qemu.org/download/' \
		| grep -oE 'https://download[.]qemu[.]org/qemu-([^"]+)[.]tar[.]xz' \
		| sort -ruV
)"
```

Continuation lines are indented one extra tab from the first command.

Multi-line pipelines most commonly appear inside a `$(…)` capture, though they do appear outside one when the output flows directly to stdout or a redirect.  One reason to prefer a capture over an inline pipeline is `set -e` behaviour: when `foo | bar` runs and `foo` fails, `bar` still executes, and its secondary error (`bar: unexpected EOF`, etc.) can obscure the real failure — making the pipeline harder to debug than a captured intermediate value would be.

This motivates a broader pattern: capture output into a variable first, then feed the next command via here-string rather than a pipe:

```bash
json="$(curl -fsSL "$url")"
count="$(jq <<<"$json" '.results | length')"
```

Each step's exit status is checked independently by `set -e`, and any error message points clearly at the failing command.  See also [jq-sh.md](jq-sh.md) for how this applies specifically to `jq` invocations.

Corpus refs: [`docker-qemu/versions.sh#L19-L25`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/versions.sh#L19-L25), [`debuerreotype/scripts/debuerreotype-recalculate-epoch#L32-L38`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/scripts/debuerreotype-recalculate-epoch#L32-L38).

## `&&` and `||` placement

When `&&` or `||` must break across lines, the operator goes at the **start** of the continuation line — consistent with how `|` is placed:

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

## Arrays

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

## Here-documents

`<<-` (with tab-stripping) is preferred when the body is indented:

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

`<<` without `-` is only used when the body is not indented (rare).

Corpus refs: [`docker-qemu/apply-templates.sh#L20-L28`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/apply-templates.sh#L20-L28), [`debian-bin/repo/apt-ftparchive-generate-conf.sh`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/apt-ftparchive-generate-conf.sh).

## Error handling patterns

### Cleanup via trap

The standard pattern for temporary directories:

```bash
dir="$(mktemp -d -t "$prog.XXXXXX")"
exitTrap="$(printf 'rm -rf %q' "$dir")"
trap "$exitTrap" EXIT
```

`printf '%q'` is used to safely shell-quote the path in the trap command string.

Corpus ref: [`debian-bin/repo/buildd.sh#L86-L88`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L86-L88).

### Subshell error isolation

`( ... ) || ...` isolates a block so that errors inside don't propagate, allowing custom handling:

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

### Error messages

Error messages always go to stderr with `echo >&2 "error: ..."` or `echo >&2 "warning: ..."`.  The prefix `error:` or `warning:` is literal text, lowercased.

## `shopt` options

When needed, `shopt` calls appear near the top of the script, after `set`:

- `shopt -s dotglob` — include dotfiles in glob patterns
- `shopt -s nullglob` — return empty list (not the literal pattern) when a glob matches nothing

## String manipulation

Parameter expansion is strongly preferred over spawning `cut`, `sed`, or `awk` for simple transformations:

```bash
distro="${suite%%-*}"        # remove longest suffix matching -*
distroSuite="${suite#$distro-}"  # remove shortest prefix
```

Case conversion: `${var,,}` for lowercase, `${var^^}` for uppercase.

## `eval` with jq output

A recurring pattern for constructing arrays or variables from JSON data:

```bash
shell="$(jq -rs '[ .[] | values[] ] | map(@json | @sh) | join(" ")' data.json)"
eval "items=( $shell )"
```

jq's `@sh` format is used to produce shell-safely-quoted strings.  `eval` is only used with controlled jq output, never with user input.

Corpus refs: [`debian-bin/repo/buildd.sh#L91-L92`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L91-L92), [`docker-qemu/apply-templates.sh#L14-L16`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/apply-templates.sh#L14-L16).

## jq invocations

See [jq-sh.md](jq-sh.md) for the full conventions.  The short summary: `jq <<<"$var" flags 'expression'` is the standard form.

## Vim modelines for ambiguous file types

Executable scripts without a `.sh` extension, and non-standard config files that vim cannot auto-detect, carry a vim modeline comment to set the filetype explicitly:

```bash
#!/usr/bin/env hocker
# vim:set ft=sh:
```

```gitconfig
# vim:set ft=gitconfig:
```

The modeline goes on the second line for scripts (after the shebang), or the first line for non-executable files.

Corpus refs: [`home/git-config.d/common`](https://github.com/tianon/home/blob/home/git-config.d/common), [`hocker/README.md` example](https://github.com/infosiftr/hocker/blob/ff4d4df2370391ca582abc51a64022501d903577/README.md#L29).

## POSIX compatibility as a design goal

Tianon's Bash scripts are consciously written so that they remain **mostly reasonable to read and write as POSIX shell** without requiring a style change.  This is an explicit goal, not an accident — the control structures, quoting conventions, and general idioms are chosen to be familiar to anyone reading plain `sh`.

When "POSIX shell" is the target, that means **BusyBox `sh`** in practice (Alpine, Docker scratch-adjacent images), and occasionally Debian's `dash`.  This is relevant for feature availability: `local` is not technically POSIX but is supported by both BusyBox and dash, which is why it is used freely even in nominally-POSIX contexts.

The Bash-specific features that *are* used deliberately:

- **Arrays** (`variants=( ... )`, `${array[@]}`) — considered worth the Bash dependency
- **`$BASH_SOURCE`** — required for correct self-location when a script may be `source`d
- **`set -Eeuo pipefail`** — `-E` and `-o pipefail` are Bash extensions; `-E` in particular makes trap inheritance reliable
- **Here-strings** (`<<<`) — cleaner than a pipe from `echo` and avoids a subshell
- **`shopt`** — for `dotglob`, `nullglob`, etc. when needed
- **`local`** — used freely (see above)

Features that are *avoided* even in Bash contexts, to preserve POSIX readability:

- `[[ ]]` — appears occasionally but `[ ]` is strongly preferred; `case` is used as the POSIX alternative for pattern matching
- `$'...'` quoting — used only for embedding literal escape sequences (`$'\n'`, `$'\t'`), not as a general quoting form

When a script genuinely requires Bash features, Bash is explicitly installed even in minimal-shell environments like Alpine — but this is a conscious tradeoff, not a default.  When POSIX sh is sufficient for the task, a POSIX shebang and style are used instead.

## `(( ))` arithmetic — use with caution

Arithmetic compound commands interact poorly with `set -e` because their exit status reflects the *arithmetic result* (zero = failure, non-zero = success), not whether the operation succeeded:

```bash
var=0
(( var++ ))   # exits 1 — post-increment returns the OLD value (0), set -e aborts
(( var++ ))   # exits 0 — returns old value (1), fine
```

```bash
(( var = 0 )) # exits 1 — assigning zero is "false", set -e aborts
(( var = 1 )) # exits 0 — fine
```

The classic footgun is incrementing a counter that starts at zero: `(( failures++ ))` aborts the script the first time it runs because the post-increment returns 0.  The corpus handles this explicitly:

```bash
(( failures++ )) || :
```

The `|| :` suppresses the false exit status.  Safe alternatives that avoid the issue entirely:

- `(( ++var ))` — pre-increment returns the *new* value, so starting from 0 returns 1 (truthy)
- `(( var += 1 ))` — addition also returns the new value

Corpus ref: [`debian-bin/repo/buildd.sh#L124`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh#L124).

## Notable omissions

Things Tianon **never** does in standalone shell scripts:

- `#!/bin/bash` shebang (always `#!/usr/bin/env bash`)
- Backtick command substitution
- The `function` keyword
- `set -x` at the global level (only used locally around specific blocks)
- `[ ]` tests with `==` (uses `=` for string comparison, per POSIX)
- Arithmetic with `let` or `$((var = expr))` assignment
- `declare -i` for integer-type variables

**Things that appear less often than you might expect:**

- `printf` for simple string output — `echo` is used for straightforward messages; `printf` appears when the format string contains escape sequences (`\n`, `\t`, `%q`) or when generating content piped to another command
- `$'...'` quoting for general string literals — appears specifically for embedding literal escape sequences: `IFS=$'\n'`, `cut -d$'\t' -f1`, or multi-line strings for GUI dialogs; not used as a general alternative to `"`
- `|| true` as a no-op — `|| :` is preferred; `|| true` appears in scripts shared with audiences less familiar with `: ` being equivalent to `true`
- `[[ ]]` test brackets — `[ ]` strongly preferred; `[[ ]]` appears occasionally for pattern matching (`[[ "$x" == */ ]]`) where `[ ]` cannot be used, but `case` is usually the fully POSIX-compatible alternative and is often chosen instead:
  ```bash
  case "$suite" in
      */security | */updates) ... ;;
  esac
  ```

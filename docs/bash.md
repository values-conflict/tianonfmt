# Bash / shell script style

Covers standalone `.sh` files and executable scripts identified by a bash or sh shebang.  For jq expressions that appear inside these scripts, see [jq-sh.md](jq-sh.md).  For shell code embedded in Dockerfile `RUN` instructions, see [dockerfile.md §RUN shell style](dockerfile.md#run-shell-style) — that context has different conventions.

## Shebang

Always `#!/usr/bin/env bash`.  Never `#!/bin/bash` even when `/bin/bash` is guaranteed to exist.

```bash
#!/usr/bin/env bash
```

Corpus refs: `corpus/actions/checkout/checkout.sh:1`, `corpus/tianon-dockerfiles/buildkit/versions.sh:1`, `corpus/debian-bin/repo/buildd.sh:1`.

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

Corpus ref: `corpus/actions/checkout/checkout.sh:1-2`, `corpus/debian-bin/repo/buildd.sh:1-2`.

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

Corpus refs: `corpus/debian-bin/repo/buildd.sh:4-8`, `corpus/debian-bin/repo/apt-ftparchive-generate-conf.sh:4-7`.

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

Corpus refs: `corpus/debian-bin/repo/buildd.sh:35-55`, `corpus/tianon-dockerfiles/buildkit/versions.sh:27-59`.

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

Corpus refs: `corpus/tianon-dockerfiles/buildkit/versions.sh:6-7`, `corpus/debian-bin/repo/buildd.sh:90`.

## Pipe placement

In shell scripts, pipes are almost always written on a **single line**.  Multi-step pipelines that must break across lines have `|` at the **end** of the preceding line — this is the opposite of the convention in standalone `.jq` files where `|` leads the next line (see [jq.md](jq.md)).

```bash
shell="$("$thisDir/needs-build.sh" "$dpkgArch" "$repo" "$suite" "$comp" \
	| jq -rs '[ .[] | values[] ] | map(@json | @sh) | join(" ")')"
```

In practice this situation is rare: when a pipeline is complex enough to need multiple lines, it is usually moved inside a command substitution `$(...)` where line breaks are natural.

Corpus ref: `corpus/debian-bin/repo/buildd.sh:91`.

## `&&` and `||` placement

In multi-line boolean chains, operators appear at the **end** of the line:

```bash
git -C "$path" clean -ffdx &&
git -C "$path" reset --hard HEAD
```

Short chains on a single line are fine: `git remote remove origin || :`.

The null-command idiom `|| :` is used instead of `|| true` for suppressing errors on non-critical operations.

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

Corpus refs: `corpus/docker-qemu/apply-templates.sh:20-28`, `corpus/debian-bin/repo/apt-ftparchive-generate-conf.sh`.

## Error handling patterns

### Cleanup via trap

The standard pattern for temporary directories:

```bash
dir="$(mktemp -d -t "$prog.XXXXXX")"
exitTrap="$(printf 'rm -rf %q' "$dir")"
trap "$exitTrap" EXIT
```

`printf '%q'` is used to safely shell-quote the path in the trap command string.

Corpus ref: `corpus/debian-bin/repo/buildd.sh:86-88`.

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

Corpus ref: `corpus/debian-bin/repo/incoming.sh:44-53`.

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

Corpus refs: `corpus/debian-bin/repo/buildd.sh:91-92`, `corpus/docker-qemu/apply-templates.sh:14-16`.

## jq invocations

See [jq-sh.md](jq-sh.md) for the full conventions.  The short summary: `jq <<<"$var" flags 'expression'` is the standard form.

## Notable omissions

Things Tianon **never** does in standalone shell scripts:

- `#!/bin/bash` shebang (always `#!/usr/bin/env bash`)
- Backtick command substitution
- The `function` keyword
- `$0` for the script path (always `$BASH_SOURCE`)
- `printf` for simple string output (uses `echo`; `printf` is reserved for format strings like `printf '%q'`)
- `set -x` at the global level (only used locally around specific blocks)
- `[ ]` tests with `==` (uses `=` for string comparison, per POSIX)
- Arithmetic with `let` or `$((var = expr))` assignment (uses `(( var++ ))` for increments when needed)
- `declare -i` for integer-type variables
- Pipes at the **start** of continuation lines (in shell; jq does this — see [jq.md](jq.md))
- `||` or `&&` at the start of continuation lines (always at end)
- `true` in place of `: ` or `|| :` for no-ops

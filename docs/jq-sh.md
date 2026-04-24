# jq embedded in shell scripts

This file documents how jq expressions are written when they appear inside Bash scripts as arguments to the `jq` command.  The conventions differ meaningfully from standalone `.jq` files (see [jq.md](jq.md)).

## Input method: always here-string

The input to jq is provided via **here-string** (`<<<`), not a pipe from `echo` or `cat`:

```bash
# correct:
jq <<<"$json" '.foo'

# never:
echo "$json" | jq '.foo'
```

The here-string goes **before the flags and expression**, immediately after `jq`:

```bash
jq <<<"$json" -r '.commit // .version'
jq <<<"$json" -S .
```

Corpus refs: `corpus/tianon-dockerfiles/buildkit/versions.sh:62`, `corpus/tianon-dockerfiles/steam/versions.sh:18`, `corpus/docker-qemu/versions.sh:52`.

## Flag ordering

Flags appear **between the here-string and the expression**:

```
jq <<<"$input" [flags...] 'expression'
```

Common flags and their order:
- `-r` / `--raw-output` — raw string output (no quotes)
- `-c` / `--compact-output` — compact JSON output
- `-s` / `--slurp` — slurp all inputs into an array
- `-n` / `--null-input` — no input (use `null` as input)
- `-S` / `--sort-keys` — sort object keys
- `-e` / `--exit-status` — exit non-zero if last value is false or null
- `--arg name value` — bind shell variable as jq string
- `--argjson name value` — bind shell variable as jq value (parsed JSON)
- `-f filename` / `--from-file filename` — load jq program from a file

Multiple flags are written separately (never combined like `-rc`):

```bash
jq <<<"$json" -r -S '.tags[]'
jq <<<"$bk" -r '.commit // .version'
```

Corpus refs: `corpus/debian-bin/repo/buildd.sh:62`, `corpus/docker-qemu/versions.sh:52`.

## Quoting the expression

jq expressions passed as shell arguments use **single quotes**:

```bash
jq <<<"$json" '.version |= split(":")[1]'
```

Single quotes are used because the jq expression often contains `$`, `"`, or `\` characters that would be interpreted by the shell inside double quotes.  The only exception is when the expression itself needs to reference a shell variable directly (rare — the preferred approach is always `--arg` or `--argjson`).

## Passing shell variables to jq

Shell variables are **never interpolated directly** into the jq expression string.  Instead:

- `--arg name "$shellvar"` binds a string value as `$name` inside jq
- `--argjson name "$shelljson"` binds a JSON value as `$name` inside jq
- `env.VARNAME` reads a shell environment variable directly inside jq (for `export`ed variables)

```bash
# correct — using --arg:
jq <<<"$json" --arg go "$go" '.versions[$go]'

# correct — using env (for exported variables):
export variant
jq -n 'if env.variant == "" then . else .[env.variant] end'

# never — direct interpolation:
jq <<<"$json" ".versions[\"$go\"]"   # wrong
```

Corpus refs: `corpus/tianon-dockerfiles/buildkit/versions.sh:67`, `corpus/debian-bin/repo/buildd.sh:59-71`.

## Single-line expressions

Short expressions stay on one line with the expression in single quotes:

```bash
commit="$(jq <<<"$bk" -r '.commit // .version')"
jq <<<"$json" '.version |= split(":")[1]' > versions.json
```

No special formatting rules apply to single-line jq expressions — they are written compactly.

## Multi-line expressions

When the jq expression is too complex to read on one line, it moves to a **multi-line single-quoted string**:

```bash
json="$(jq <<<"$json" --argjson bk "$bk" --arg go "$go" '
	if env.variant == "" then . else .[env.variant] end += $bk + { go: { version: $go } }
	| .variants += [ env.variant ]
')"
```

Layout rules for multi-line jq in shell:

1. The opening `'` ends the current line immediately after the flags
2. The jq content starts on the **next line**, indented by **one tab relative to the surrounding shell indentation** (i.e., shell indent + 1 tab)
3. The closing `'` is on its **own line at the shell's indentation level** (one tab less than the content)
4. Pipe-chain style follows [jq.md](jq.md): `|` at the **start** of continuation lines

The indentation inside the single-quoted string is **relative to the shell context**, not absolute.  If the `jq` invocation is inside a `for` loop (already at 1 tab), the jq content gets 2 tabs:

```bash
for variant in "${variants[@]}"; do          # 0 tabs (shell)
	json="$(jq <<<"$json" --arg v "$variant" '   # 1 tab
		if env.variant == "" then . else .[env.variant] end   # 2 tabs
		| .variants += [ env.variant ]                        # 2 tabs
	')"                                          # 1 tab
done
```

Corpus ref: `corpus/tianon-dockerfiles/buildkit/versions.sh:67-70`.

## `-f` flag for file-based programs

When the jq program is in a `.jq` file, `jq -f file` or `jq --from-file file` is used:

```bash
jq -c -f .github/workflows/version-components.jq "$dir/versions.json"
```

Or for library inclusion, the `include` directive with `-L`:

```bash
exec jq -sR -L"$dir/../jq" 'include "deb822"; deb822_parse' "$@"
```

Corpus refs: `corpus/tianon-dockerfiles/.github/workflows/update.yml:87`, `corpus/debian-bin/generic/deb822-json:7`.

## Output redirection

When writing jq output to a file, the redirection uses `>` (overwrite), not `>>`:

```bash
jq <<<"$json" -S . > versions.json
jq <<<"$json" '.' > versions.json
```

For a plain "identity" format pass (pretty-print only), the expression is `.`:

```bash
jq <<<"$json" '.' > versions.json
```

Corpus refs: `corpus/docker-qemu/versions.sh:52`, `corpus/tianon-dockerfiles/buildkit/versions.sh:73`.

## `eval` integration

A common pattern for constructing shell arrays from JSON:

```bash
shell="$(jq <<<"$json" -rs '
	[ .[] | values[] ]
	| map(@json | @sh)
	| join(" ")
')"
eval "items=( $shell )"
```

`@sh` in jq produces shell-quoted strings safe to `eval`.  `@json` converts values to their JSON string representation before shell-quoting.

Corpus ref: `corpus/debian-bin/repo/buildd.sh:91-92`.

## jq inside YAML `run:` steps

Inside GitHub Actions `run:` blocks, jq invocations follow the same conventions as in standalone shell scripts.  The indentation of the shell code is determined by YAML's `|` block scalar, not by the jq invocation itself.

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
```

The jq multi-line expression follows the same shell embedding rules: content indented one level inside the `'`, closing `'` at the outer shell level.

Corpus ref: `corpus/tianon-dockerfiles/.github/workflows/update.yml:31-47`.

## Style differences from standalone `.jq`

| Aspect | Standalone `.jq` | Embedded in shell |
|--------|-----------------|-------------------|
| Indentation | Hard tabs | Tabs, relative to shell context |
| Expression length | Multi-line freely | Single-line preferred; multi-line for complex cases |
| Leading `\|` | At start of continuation line | At start of continuation line (same) |
| Shell variables | n/a | Via `--arg`, `--argjson`, `env.VAR` |
| `include` | At top of file | Via `-L dir 'include "module"; ...'` |
| Input | Via `inputs`, `input`, filters | Via here-string `<<<` or `-f file` |

## Notable omissions

Things Tianon **never** does when embedding jq in shell:

- `echo "$var" | jq` — always uses `<<<` here-string
- Double-quoted jq expressions — always single-quoted (or `-f file`)
- Directly interpolating shell variables into the expression string — always uses `--arg`/`--argjson`
- Flags after the expression: the expression is always last
- `-e` flag except when actually checking exit status
- `jq .` on its own without at least either input or flags (i.e., always provides the expression)

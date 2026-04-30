# jq style (standalone `.jq` files)

Covers `.jq` source files used as libraries or standalone programs.  For jq expressions embedded inside shell scripts, see [jq-sh.md](jq-sh.md).  For jq expressions inside Dockerfile templates, see [jq-template.md](jq-template.md).

## Indentation

**Hard tabs, one per nesting level.**  No spaces are ever used for indentation inside jq files.

Corpus refs: [`debian-bin/jq/deb822.jq`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq), [`debian-bin/jq/dpkg-version.jq`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq), [`tianon-dockerfiles/scratch/multiarch.jq`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq).

## Pipe `|` placement

In standalone jq files, `|` goes at the **start** of the continuation line, not at the end of the previous one.  This matches the convention in [shell scripts](bash.md) — both use leading operators on continuation lines.

```jq
del(.out)
| ($line | _trimstart) as $ltrim
| ($ltrim | _trimend) as $trim
| if $ltrim != $line then
    ...
```

**Each pipe step always gets its own line.**  The formatter never joins multiple pipe steps onto a single line, regardless of length.  Even two short consecutive steps like `ltrimstr("docker.io/") | ltrimstr("library/")` must be on separate lines:

```jq
# correct:
ltrimstr("docker.io/")
| ltrimstr("library/")

# wrong:
ltrimstr("docker.io/") | ltrimstr("library/")
```

The **one exception** is inside an inline subexpression that is itself already on one line (e.g. a single-line `if` condition, or an argument to a call): `startswith("#") | not` is acceptable when the enclosing expression is itself inline.

**`] | join(...)` always splits.**  When a `]` closes an array and a `| join(...)` follows, the pipe goes on the next line:

```jq
# correct:
[
    ...,
    empty
]
| join("\n")

# wrong:
] | join("\n")
```

Corpus refs: [`debian-bin/jq/deb822.jq#L23-L34`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L23-L34), [`debian-bin/jq/dpkg-version.jq#L32-L52`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L32-L52).

## Comma `,` placement

In multi-line comma (generator) sequences, `,` goes at the **end** of the line:

```jq
foreach (
	lines,
	""
	| select(...)
) as $line (
```

Short comma sequences that fit on one line stay inline: `"linux", "windows", "freebsd"`.

The trailing-comma preference (see [universal.md](universal.md)) means that when a comma sequence is multi-line, all elements including the last should ideally carry a trailing comma.  Since jq's comma operator is semantic (not decorative), the `empty` element is used as a no-op last entry to allow this — `empty` generates nothing but lets every real element end with `,`:

```jq
(
	"linux",
	"windows",
	"freebsd",
	empty # trailing comma
)
```

**Comma continuation indentation — pipe context.**  When a multi-line comma expression appears as the body of a `| ` pipe step or `as $x |` binding, the continuation elements (second and later) are indented **one extra level** relative to the first element, to distinguish them visually from the next pipe step:

```jq
# correct:
| @sh "docker pull \($ref)",
    @sh "docker tag \($ref) \(.key)"

# wrong — continuation at same level as pipe:
| @sh "docker pull \($ref)",
@sh "docker tag \($ref) \(.key)"
```

This extra indent applies only to comma expressions that are direct children of a `|` or `as $x |` step.  Inside `[]` or `()`, comma elements stay at the enclosing scope's depth.

Corpus ref: [`meta-scripts/meta.jq`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/meta.jq).

Corpus ref: [`tianon-dockerfiles/scratch/multiarch.jq#L6-L20`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L6-L20).

## `def` — function definitions

Top-level function definitions always use multi-line layout.  The body is indented one tab from the `def` line, and the closing `;` is at the same indentation as `def`:

```jq
def dpkg_version_string:
	if .epoch then
		"\(.epoch):"
	else "" end
	+ .upstream
	+ if .revision then
		"-\(.revision)"
	else "" end
;
```

The parameter list (if any) goes inline on the `def` line:

```jq
def deb822_stream(lines):
	...
;
```

Multiple parameters are separated by `; ` (semicolon-space):

```jq
def validate($value; $check; $msg):
	...
;
```

**Local (inline) `def`** inside an expression body uses single-line form when the body is short enough (up to ~100 chars):

```jq
def _trimstart: until(startswith(" ") or startswith("\t") | not; .[1:]);
def _trimend: until(endswith(" ") or endswith("\t") | not; .[:-1]);
```

When the body is long or complex, local defs also use the multi-line form.

Corpus refs: [`debian-bin/jq/deb822.jq#L7-L39`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L7-L39), [`debian-bin/jq/dpkg-version.jq#L21-L29`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L21-L29), [`debian-bin/jq/deb822.jq#L21-L22`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L21-L22).

## `if` / `then` / `elif` / `else` / `end`

**Inline form** when the entire expression (condition + both branches) is short and simple enough to read comfortably on one line.  There is no hard rule — it is vibes-based ("the numbers got too big") — but corpus analysis shows the threshold is approximately 60 characters.  Note that this is not *only* about length: a short expression that is hard to parse at a glance still gets multi-line treatment.  This "short and clear enough to inline" heuristic applies consistently across jq, shell, Dockerfile `RUN` blocks, and everywhere else.

```jq
if index(":") then . else "0:" + . end
if . == "~" then -2 elif . == "-" then -1 else explode[0] end
```

**Multi-line form** when any part is complex.  The `then` body is indented one tab from `if`; `elif` and `else` are at the same level as `if`:

```jq
if $line == "" then
	{ out: .accum, accum: {} }
elif $ltrim != $line then
	.accum[.cur] += "\n" + $trim
else
	...
end
```

**When `then` is multi-line, `else` must also be multi-line.**  If the `then` body spans multiple lines (e.g. because it has a leading comment before its value, producing a comment line + value line), the `else` branch must also be fully multi-line — not the short `else VALUE end` form.  The reason is visual symmetry: once the `then` body fills more than one line, collapsing the `else` onto a single line looks inconsistent:

```jq
# correct — then body has comment+value spanning two lines, else is multi-line too:
if $arch | startswith("windows-") then
	# https://github.com/...
	"classic"
else
	"buildkit"
end

# wrong — then spans two lines but else is crammed onto one:
if $arch | startswith("windows-") then
	# https://github.com/...
	"classic"
else "buildkit" end
```

Short `else` body: when the `then` body is exactly one visual line (a single statement with no leading comment) and the `else` body is short, keep `else` and its value on the same line:

```jq
if .epoch then
	"\(.epoch):"
else "" end
```

Corpus refs: [`debian-bin/jq/dpkg-version.jq#L22-L29`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L22-L29), [`debian-bin/jq/deb822.jq#L18-L35`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L18-L35).

## `reduce` and `foreach`

Both are always multi-line.  The body is indented inside the `(init; update)` or `(init; update; extract)` parentheses:

```jq
reduce .[] as $arr (
	{};
	. - (. - $arr)
)
```

```jq
foreach (
	lines,
	""
	| select(startswith("#") | not)
) as $line ({ accum: {} };
	if $line == "" then
		{ out: .accum, accum: {} }
	else
		...
	end;
	if .out and (.out | length) > 0 then .out else empty end
)
```

The **short extract** pattern (when the extract expression is ≤50 chars and contains no newlines) puts `; extract)` on the closing line:

```jq
	; if .out then .out else empty end)
```

Corpus refs: [`debian-bin/jq/deb822.jq#L8-L38`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L8-L38), [`tianon-dockerfiles/scratch/multiarch.jq#L67-L68`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L67-L68).

## `try` / `catch`

Short form stays on one line:

```jq
try tonumber // (...)
```

With a catch handler:

```jq
try tonumber catch error("failed: \(.)")
```

Corpus ref: [`debian-bin/jq/dpkg-version.jq#L38`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L38).

## Function calls

A function call whose single argument is a simple expression (a variable, a field access, or another call — not a pipeline or compound expression) stays on one line:

```jq
# correct:
build_annotations(git_build_url)

# wrong — single simple arg does not get a line break:
build_annotations(git_build_url
)
```

When an argument is a complex, multi-line expression, the closing `)` goes on its own line at the same indentation as the call.

## Object literals

**Inline** when all key-value pairs fit within ~60 characters, with spaces inside braces:

```jq
{ out: .accum, accum: {} }
{ $parse, $string }
```

**Multi-line** with one field per line and a trailing comma on each field:

```jq
{
	os: (
		"linux",
		"windows",
		empty
	),
	architecture: (
		"amd64", "386",
		empty
	),
}
```

Object key syntax:
- **Unquoted identifiers** when the key is a valid jq identifier (`[a-zA-Z_][a-zA-Z0-9_]*`): `os`, `architecture`
- **Quoted strings** when the key contains special characters: `"os.version"`, `"armel | armhf"`
- **Computed keys** with `(expr)`: `{ (env.key): value }`
- **Variable shorthand** `{ $foo }` for `{ foo: $foo }` — seen in test data

The formatter automatically converts `{"foo": .}` → `{foo: .}` when the key is a valid unquoted identifier.  Keys with dots, hyphens, or other special characters are always kept quoted.

Corpus refs: [`tianon-dockerfiles/scratch/multiarch.jq#L5-L16`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L5-L16), [`debian-bin/jq/deb822.jq#L19`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L19).

## Array literals

**Inline** when all elements fit within ~60 characters: `["amd64", "arm64"]`.

**Multi-line** with one element per line:

```jq
[
	"v9.6", "v9.5", "v9.4",
	empty
]
```

The **trailing-comma hack** using `empty` as a "null" last element appears often to allow all real elements to have trailing commas:

```jq
[
	"amd64",
	"arm64",
	empty # trailing comma
]
```

**Never compact a multi-line array.**  If an array was written with elements on separate lines, the formatter must not collapse it to a single line, even if the result would fit within the threshold.  The author's choice to use multi-line format is intentional — it signals that each element is meaningful and should be visually distinct.  In particular, any array ending with `empty` (the trailing-comma idiom) must always be multi-line since `empty` is a sentinel value that makes no sense on a line by itself:

```jq
# always wrong — do not compact:
[@sh "crane push temp \(.img)", "rm -rf temp", empty] | join("\n")

# correct:
[
	@sh "crane push temp \(.img)",
	"rm -rf temp",
	empty
]
| join("\n")
```

**Blank lines inside array literals are preserved.**  A blank line inside `[...]` separates logical groups and must not be removed.

Corpus ref: [`meta-scripts/oci.jq`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/oci.jq).

## Object literals — blank lines preserved

**Blank lines inside object literals are preserved.**  Authors use blank lines to separate logical groups of fields and these must not be removed:

```jq
{
	"org.opencontainers.image.source": $buildUrl,
	"org.opencontainers.image.revision": ...,

	# TODO: consider adding vendor field
	"com.docker.official-images.bashbrew.arch": .build.arch,
}
```

## Arithmetic chains (multi-line `+`, etc.)

When an arithmetic operator chain is too long to fit on one line, each subsequent operand goes on its own line with the operator **leading** that line:

```jq
if .epoch then
	"\(.epoch):"
else "" end
+ .upstream
+ if .revision then
	"-\(.revision)"
else "" end
```

Corpus ref: [`debian-bin/jq/dpkg-version.jq#L22-L28`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L22-L28).

## Alternative operator `//`

When breaking a `//` alternative across lines, the `//` leads the continuation:

```jq
.value.manifests[0].annotations["org.opencontainers.image.ref.name"]
// .value.annotations["org.opencontainers.image.ref.name"]
// error("parent \(.key) missing ref")
```

Corpus ref: [`meta-scripts/meta.jq#L39-L41`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/meta.jq#L39-L41).

## `def` separation

One blank line separates top-level `def` blocks from each other and from surrounding `include`/`import` statements and the main expression.  No blank line is added inside the body of a `def`.

## File endings

Every formatted jq file ends with **exactly one newline** — the final `;` of the last `def` (or the last line of the main expression) is followed by `\n` and nothing else.  Two trailing newlines is always a bug.

## Comments

Comments explain *why*, not *what*.  Trailing comments after expressions are common:

```jq
empty # trailing comma hack
empty # tailing comma (sic)
```

`TODO` comments appear frequently and are concrete:

```jq
# TODO consider splitting this into a separate function?
```

Comment blocks at the top of `.jq` files describe the module's purpose, expected input, and output format.

**Comments are never dropped.**  This is a hard invariant: every comment in the source must appear in the formatted output.  A formatter that silently discards a comment is broken.

**Trailing comments stay on their original line.**  A `# comment` that appears at the end of a line of code must remain on that same line in the output:

```jq
# correct:
"org.opencontainers.image.version": ( # value of the first image tag
	first(...)
)

# wrong — comment moved inside brackets:
"org.opencontainers.image.version": (
	first(.source.arches[.build.arch # value of the first image tag
	].tags[])
)
```

**Leading comments stay before their expression.**  A `# comment` that appears on its own line immediately before an expression must remain before that expression in the output.  The formatter must not absorb it into the interior of the following expression (e.g. as a leading comment on the first argument of a function call):

```jq
# correct:
# trim out comment lines and unnecessary indentation
gsub("(?m)^(...)"; "\(.extra // "")")

# wrong — comment moved inside the argument list:
gsub(
	# trim out comment lines and unnecessary indentation
	"(?m)^(...)";
	"\(.extra // "")"
)
```

**Comments before a closing `)` are preserved as closing-delimiter comments.**  A comment on its own line immediately before `)` is kept there in the output, not moved inside the body:

```jq
# correct:
( # value of the first image tag
    first(...)
    | sub("^.*:"; "")
    # TODO maybe we should prefer the longest non-latest tag?
),

# wrong — comment discarded or moved:
(
    first(...)
    | sub("^.*:"; "")
),
```

**Blank lines between a leading comment and an object field key have no trailing whitespace.**  When an object field has a leading comment and a blank line separates the comment from the key, the blank line is truly empty (no tabs):

```
# TODO: see notes above
          ← blank line with NO tabs (correct)
"com.docker.official-images.bashbrew.arch": .build.arch,
```

Corpus ref: [`debian-bin/jq/deb822.jq#L1-L6`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L1-L6).

## `include` and `import`

`include` statements appear at the very top of the file, before any `def` or main expression:

```jq
include "meta";

def needs_build:
	...
```

Corpus ref: [`meta-scripts/meta.jq#L1-L3`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/meta.jq#L1-L3).

## String interpolation

String interpolation `\(.expr)` is used freely.  The interpolated expression is kept compact (inline) within the string:

```jq
"\(.epoch):"
"error: unknown variant: '\($variant)'"
```

Long interpolated expressions are never broken across lines within a string literal.

## Path expressions

`.["key"]` is used for keys that are not valid identifiers.  `."key"` (dot followed by a quoted string) is also valid and occasionally seen:

```jq
.["os.version"]
."os.version"   # equivalent
```

## `@format` strings

`@sh` is used for shell-escaping (see [jq-sh.md](jq-sh.md)).  `@json` is used to produce compact JSON from a value.  `@csv` and `@tsv` appear occasionally for tabular output.

## Notable omissions

Things Tianon **never** does in standalone `.jq` files:

- Spaces for indentation (always tabs)
- `|` at the **end** of a line before a continuation (always at the start of the next)
- `function` keyword (not valid jq, but worth noting for clarity)
- Semi-colons at the end of `def` bodies on the same line as the body when the body is multi-line
- `if` without `end` (the `end` is never omitted, even when following `else`)
- `reduce`/`foreach` written on a single line when the expression is non-trivial — short expressions that fit comfortably on one line do appear: `reduce .[] as $a ([]; if IN(.[]; $a) then . else . += [$a] end)`
- Empty `{}` or `[]` with spaces inside: `{ }`, `[ ]` — uses `{}`, `[]`
- `null` in place of `empty` for "nothing" in generators — `empty` is preferred
- `not` written as `== false` (always `| not`) — note: `| not` also matches `null`, so the two are not always equivalent; `--pedantic` flags `== false`
- Positive boolean checks written as `== true` — use the expression directly; `--pedantic` flags `== true`
- `!= false` (truthy check) — use the expression directly; `--pedantic` flags it
- `!= true` (negation) — use `| not`; `--pedantic` flags it
- Quoted object keys for plain identifiers: `{"foo": .}` should be `{foo: .}` — the formatter enforces this automatically

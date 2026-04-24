# jq style (standalone `.jq` files)

Covers `.jq` source files used as libraries or standalone programs.  For jq expressions embedded inside shell scripts, see [jq-sh.md](jq-sh.md).  For jq expressions inside Dockerfile templates, see [jq-template.md](jq-template.md).

## Indentation

**Hard tabs, one per nesting level.**  No spaces are ever used for indentation inside jq files.

Corpus refs: `corpus/debian-bin/jq/deb822.jq`, `corpus/debian-bin/jq/dpkg-version.jq`, `corpus/tianon-dockerfiles/scratch/multiarch.jq`.

## Pipe `|` placement

In standalone jq files, `|` goes at the **start** of the continuation line, not at the end of the previous one.  This is the **opposite** of the convention in [shell scripts](bash.md).

```jq
del(.out)
| ($line | _trimstart) as $ltrim
| ($ltrim | _trimend) as $trim
| if $ltrim != $line then
    ...
```

Short pipe chains that fit on a single line stay inline: `startswith("#") | not`.

Corpus refs: `corpus/debian-bin/jq/deb822.jq:23-34`, `corpus/debian-bin/jq/dpkg-version.jq:32-52`.

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

Corpus ref: `corpus/tianon-dockerfiles/scratch/multiarch.jq:6-20`.

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

Corpus refs: `corpus/debian-bin/jq/deb822.jq:7-39`, `corpus/debian-bin/jq/dpkg-version.jq:21-29`, `corpus/debian-bin/jq/deb822.jq:21-22`.

## `if` / `then` / `elif` / `else` / `end`

**Inline form** when the entire expression (condition + both branches) fits within ~60 characters:

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

Short `else` body: when the `then` body is multi-line but the `else` body is short, keep `else` and its value on the same line:

```jq
if .epoch then
	"\(.epoch):"
else "" end
```

Corpus refs: `corpus/debian-bin/jq/dpkg-version.jq:22-29`, `corpus/debian-bin/jq/deb822.jq:18-35`.

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

Corpus refs: `corpus/debian-bin/jq/deb822.jq:8-38`, `corpus/tianon-dockerfiles/scratch/multiarch.jq:67-68`.

## `try` / `catch`

Short form stays on one line:

```jq
try tonumber // (...)
```

With a catch handler:

```jq
try tonumber catch error("failed: \(.)")
```

Corpus ref: `corpus/debian-bin/jq/dpkg-version.jq:38`.

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
- **Unquoted identifiers** when the key is a valid jq identifier: `os`, `architecture`
- **Quoted strings** when the key contains special characters: `"os.version"`, `"armel | armhf"`
- **Computed keys** with `(expr)`: `{ (env.key): value }`
- **Variable shorthand** `{ $foo }` for `{ foo: $foo }` — seen in test data

Corpus refs: `corpus/tianon-dockerfiles/scratch/multiarch.jq:5-16`, `corpus/debian-bin/jq/deb822.jq:19`.

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

Corpus ref: `corpus/meta-scripts/oci.jq`.

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

Corpus ref: `corpus/debian-bin/jq/dpkg-version.jq:22-28`.

## Alternative operator `//`

When breaking a `//` alternative across lines, the `//` leads the continuation:

```jq
.value.manifests[0].annotations["org.opencontainers.image.ref.name"]
// .value.annotations["org.opencontainers.image.ref.name"]
// error("parent \(.key) missing ref")
```

Corpus ref: `corpus/meta-scripts/meta.jq:39-41`.

## Comments

Comments explain *why*, not *what*.  Trailing comments after expressions are common:

```jq
empty # trailing comma hack
empty # tailing comma (sic)
```

TODO comments appear frequently and are concrete:

```jq
# TODO consider splitting this into a separate function?
```

Comment blocks at the top of `.jq` files describe the module's purpose, expected input, and output format.

Corpus ref: `corpus/debian-bin/jq/deb822.jq:1-6`.

## `include` and `import`

`include` statements appear at the very top of the file, before any `def` or main expression:

```jq
include "meta";

def needs_build:
	...
```

Corpus ref: `corpus/meta-scripts/meta.jq:1-3`.

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
- `reduce`/`foreach` written on a single line (always multi-line)
- Empty `{}` or `[]` with spaces inside: `{ }`, `[ ]` — uses `{}`, `[]`
- `null` in place of `empty` for "nothing" in generators — `empty` is preferred
- `not` written as `== false` (always `| not`)

# AWK style

Covers `.awk` files in the corpus.  The only significant AWK file is [`doi/bashbrew/scripts/jq-template.awk`](https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk), which is the canonical processor for Tianon's jq-template Dockerfile format (see [jq-template.md](jq-template.md)).

When Tianon writes complex AWK scripts, they often end up requiring gawk features — the `jq-template.awk` file itself says `# this script assumes gawk!`.  However, simpler scripts are written to work in `mawk` as well (mawk and BusyBox `awk` appear to be similar enough that targeting one usually means the other works too).  POSIX awk compatibility is less of a concern than BusyBox/mawk compatibility.

## Header comment

AWK files open with a comment block explaining the script's purpose, requirements, and usage:

```awk
# this script assumes gawk! (busybox "awk" is not quite sufficient)

# see https://github.com/docker-library/php or ... for examples of usage ("apply-templates.sh")
```

Corpus ref: [`doi/bashbrew/scripts/jq-template.awk#L1-L3`](https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk#L1-L3).

## Indentation

**Hard tabs, one per nesting level.**  Consistent with all other non-YAML/JSON file types.

## `BEGIN` block

The `BEGIN` block appears after function definitions, at the point where global variables are initialised:

```awk
BEGIN {
	jq_expr_defs = ""
	jq_expr = ""
	agg_jq = ""
	agg_text = ""

	OPEN = "{{"
	CLOSE = "}}"
	CLOSE_EAT_EOL = "-" CLOSE ORS
}
```

Blank lines separate logically distinct groups of variable initialisations.

Corpus ref: [`doi/bashbrew/scripts/jq-template.awk#L27-L36`](https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk#L27-L36).

## Function definitions

Functions are declared before `BEGIN` and pattern-action blocks:

```awk
function jq_escape(str) {
	gsub(/\\/, "\\\\", str)
	gsub(/\n/, "\\n", str)
	...
	return "\"" str "\""
}
```

```awk
function num(haystack, needle, # parameters
             ret, i          ) # locals
{
	...
}
```

- Opening brace `{` always on the same line as `function name(params)`
- The local variable declaration trick (extra parameters not passed by callers) is documented with comments: `# parameters` and `# locals`
- Blank line before the opening `{` is used when the parameter list spans multiple lines

Corpus ref: [`doi/bashbrew/scripts/jq-template.awk#L6-L25`](https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk#L6-L25).

## Pattern-action blocks

The main processing block:

```awk
{
	line = $0 ORS
	...
}
```

`END` block for final processing:

```awk
END {
	...
	prog = "jq --join-output --from-file /dev/stdin versions.json"
	printf "%s", jq_expr | prog

	e = close(prog)
	if (e != 0) {
		exit(e)
	}
}
```

Corpus ref: [`doi/bashbrew/scripts/jq-template.awk#L71-L135`](https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk#L71-L135).

## String handling

- **Concatenation** via adjacency (no operator): `"prefix" str "suffix"`
- **`gsub`** for global substitution: `gsub(/pattern/, replacement, target)`
- **`index`** for substring search: `i = index(haystack, needle)`
- **`substr`** for substrings: `substr(str, start, len)`
- Regular expressions in `/pattern/` syntax

## Control flow

Standard AWK `if`/`else`, `while`, `next`:

```awk
if (i) {
	agg_text = agg_text substr(line, 1, i - 1)
	line = substr(line, i)
}
```

```awk
while (i = index(agg_jq, OPEN)) {
	...
}
```

`next` to skip to the next input record.

## `printf` for output

`printf "%s", value` is used instead of `print value` when precise control over output is needed (especially to avoid the trailing newline that `print` adds).

```awk
printf "%s", jq_expr | prog
```

## Piping to external commands

AWK's ability to pipe directly to a shell command:

```awk
prog = "jq --join-output --from-file /dev/stdin versions.json"
printf "%s", jq_expr | prog
e = close(prog)
if (e != 0) {
    exit(e)
}
```

The command string is stored in a variable for clarity.  `close(prog)` captures the exit code.

## `ENVIRON` for environment variables

```awk
if (ENVIRON["DEBUG"]) {
    print jq_expr > "/dev/stderr"
}
```

Corpus ref: [`doi/bashbrew/scripts/jq-template.awk#L124-L126`](https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk#L124-L126).

## Notable omissions

- `print` for output when precision is needed — always `printf` with explicit format
- Associative array iteration with `for (key in arr)` — not needed in this script
- Field separator (`FS`) modification — not used; the script operates on whole lines via `$0`
- Multiple output files — only stdout and stderr
- `getline` — not used; input flows through the normal record processing

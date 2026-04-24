# jq template format (`Dockerfile.template`)

Files named `Dockerfile.template` (or occasionally `Dockerfile.bookworm`, `Dockerfile.rc`, etc. that contain `{{ }}` markers) use Tianon's jq-template format.  The canonical processor is `corpus/doi/bashbrew/scripts/jq-template.awk`.

These files are **not** pure Dockerfiles — they are generators that produce Dockerfiles when evaluated.  The formatter must handle them differently from plain Dockerfiles.  See [dockerfile.md](dockerfile.md) for the Dockerfile rules that apply to the *output* (and to the non-template portions of the input).

## The `{{ }}` delimiter

Template expressions are wrapped in `{{` and `}}`.  The content is a jq expression evaluated against a versions JSON object.

```
{{ .version }}
{{ .url | @sh }}
{{ if env.variant == "native" then ( -}}
```

The `-}}` variant strips the trailing newline from the template output, allowing tight control over whitespace.

## Expression types and their layout

### Inline expressions

When the `{{ }}` block is embedded within a line of Dockerfile content (i.e., the surrounding text on the same line is non-empty), the expression is written compactly on a single line:

```dockerfile
ENV QEMU_VERSION {{ .version }}
ENV QEMU_URL {{ .url }}
```

```dockerfile
	wget -O steam.deb {{ .url | @sh }}; \
```

The jq expression is kept short and does not contain newlines.

Corpus refs: `corpus/docker-qemu/Dockerfile.template:76-77`, `corpus/tianon-dockerfiles/steam/Dockerfile.template:37`.

### Block expressions

When a `{{ }}` block occupies its own line (or lines), the jq content is formatted according to [jq.md](jq.md) rules — tabs, pipe at start of continuation lines, etc.:

```dockerfile
{{ def firmware_packages: {
	amd64: "ovmf",
	arm64: "qemu-efi-aarch64",
	"armel | armhf": "qemu-efi-arm",
	i386: "ovmf-ia32",
	riscv64: "opensbi u-boot-qemu",
# TODO add u-boot-qemu to more arches?
} -}}
```

```dockerfile
{{
	[
		firmware_packages
		| to_entries[]
		| (
-}}
```

Block expressions use `-}}` at the end to suppress the newline they would otherwise emit into the output.

Corpus ref: `corpus/docker-qemu/Dockerfile.template:21-28`, `corpus/docker-qemu/Dockerfile.template:35-45`.

### Conditional blocks

`if/then/else/end` across template blocks:

```dockerfile
{{ if env.variant == "native" then ( -}}
	arch="$(dpkg --print-architecture)"; \
	case "$arch" in \
{{ ) else ( -}}
	apt-get install -y --no-install-recommends \
{{ ) end -}}
```

Each conditional branch delimiter (`then (`, `) else (`, `) end`) is on its own `{{ ... -}}` line.  The `-}}` suppresses the newline so the Dockerfile content immediately follows.

Corpus ref: `corpus/docker-qemu/Dockerfile.template:32-63`.

### `def` hoisting

`def` declarations inside `{{ }}` blocks are hoisted to the top of the generated jq program (this is how `jq-template.awk` works).  They appear at the beginning of the template, before any conditional or output expressions:

```dockerfile
{{ def firmware_packages: {
	amd64: "ovmf",
	...
} -}}
```

A `def` block always uses `-}}` and appears before the first non-def content.

### `include` and `import`

`include "module"` and `import "module" as $m` also get hoisted to the top, same as `def`.

## Environment variables

`env.VARNAME` is the standard way to access environment variables inside template expressions:

```dockerfile
{{ if env.version | IN("7.2", "8.0") then "" else ( -}}
```

```dockerfile
{{ if env.variant == "native" then ( -}}
```

The variable is always read through `env.VARNAME`, never passed via `--arg` (since the template processor uses environment variables).

Corpus refs: `corpus/docker-qemu/Dockerfile.template:32`, `corpus/docker-qemu/Dockerfile.template:124`.

## Comments inside `{{ }}`

Pure-comment blocks are supported: `{{ # this is a comment -}}`.  They produce no output and are ignored by the processor.

## Dockerfile content between blocks

The Dockerfile content that appears outside `{{ }}` blocks is formatted according to [dockerfile.md](dockerfile.md):

- UPPERCASE instruction keywords
- Tab indentation for continuation lines
- Double-tab for sub-items within a continuation
- Inline comments (within RUN continuation blocks) at column 0

```dockerfile
FROM debian:trixie-slim

RUN set -eux; \
{{ # ...template block... -}}
	apt-get install -y --no-install-recommends ca-certificates; \
```

## Generated file header

Templates generate files that include a standard comment warning at the top:

```dockerfile
#
# NOTE: THIS DOCKERFILE IS GENERATED VIA "apply-templates.sh"
#
# PLEASE DO NOT EDIT IT DIRECTLY.
#
```

This header is emitted by the `apply-templates.sh` script (using a `generated_warning()` bash function), not by the template itself.

## Detection heuristic

A file is a jq template if it contains both `{{` and `}}`.  The file extension `Dockerfile.template` is the clearest signal, but the `{{ }}` content is the definitive marker — plain Dockerfiles never contain `{{`.

## Style summary: template jq vs standalone jq

The jq inside `{{ }}` blocks follows [jq.md](jq.md) style exactly when multi-line.  The key difference is context:

| Aspect | Standalone `.jq` | Template `{{ }}` block |
|--------|-----------------|----------------------|
| Indentation | Tabs (absolute) | Tabs (matches surrounding Dockerfile indent) |
| Pipe `\|` | Start of continuation line | Start of continuation line |
| `def` | Top of file | Hoisted by processor; `-}}` to suppress newline |
| `env.VAR` | Used when vars are exported | Primary way to access variant/version |
| Inline vs block | Multi-line freely | Inline when on same line as Dockerfile text |

## Notable omissions

- `{{ expr }}` blocks are **never** used with newlines inside for inline expressions — if a block needs to be multi-line, it is a block expression, not an inline one
- The closing marker is always `-}}` for block expressions (to suppress whitespace) — `}}` without `-` is only used for inline expressions that are genuinely producing inline output
- The template format does not support `//=` assignment or other jq update operators at the top level (since the generated program is an expression, not a filter pipeline with side effects against the input)

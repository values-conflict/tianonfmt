# Things to explore further

Topics that came up during documentation but were deferred because the corpus
is too sparse, the pattern is too specialised, or the area needs Tianon's
direct input to document correctly.

## The bashbrew library maintenance trio

Three related script patterns form Tianon's workflow for maintaining Docker
Official Images (and his personal images):

### `versions.sh`

A highly consistent script pattern that appears across virtually every image
directory in [`tianon/dockerfiles`](https://github.com/tianon/dockerfiles):

```bash
#!/usr/bin/env bash
set -Eeuo pipefail

[ -e versions.json ]

dir="$(readlink -ve "$BASH_SOURCE")"
dir="$(dirname "$dir")"
source "$dir/../.libs/git.sh"   # or deb-repo.sh, pypi.sh, etc.

# ... fetch data, build $json ...

jq --tab <<<"$json" '.' > versions.json
```

The structure is invariant:
- **Line 1**: `#!/usr/bin/env bash`
- **Line 2**: `set -Eeuo pipefail`
- **Line 3**: blank
- **Line 4**: `[ -e versions.json ]` — a rough "are we in the right directory?" check: if `versions.json` doesn't exist, the script is almost certainly being run from the wrong working directory; exits non-zero and stops immediately if the file is absent
- **Line 5**: blank
- **Lines 6+**: optional `source` of a shared `.libs/` helper, then fetch + transform logic
- **Last line**: always `jq ... > versions.json`, writing the result back

The final `jq` write varies by complexity — `jq --tab <<<"$json" '.'` for a straight pass-through, `jq --null-input --sort-keys '{ version: env.version }'` when building from exported variables, or a more complex expression when transforming intermediate data.

Corpus ref: [`tianon-dockerfiles/buildkit/versions.sh`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh), and nearly every `*/versions.sh` in that repo.

### `apply-templates.sh`

Runs `jq-template.awk` on `Dockerfile.template` to generate concrete
`Dockerfile` files.  Documented in [jq-template.md](jq-template.md) for the
template format itself; the script that drives it deserves its own section here.

### `gsl.sh` (Generate Stackbrew Library)

Generates the `library/NAME` entry for `docker-library/official-images`,
using exported `arches`, `commit`, and `dir` variables to produce the bashbrew
format via jq.  Corpus ref: [`tianon-dockerfiles/buildkit/gsl.sh`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/gsl.sh).

All three ultimately feed the `library/` files consumed by bashbrew, used both
for DOI and for Tianon's personal Docker images.  They deserve a unified
document explaining how the pipeline fits together.

## Docker Compose / stack YAML

The corpus has Docker Compose and Swarm stack files (`docker-moosefs/example-swarm/stack.yml`)
that use conventions different from GitHub Actions workflows — notably flow
sequences `{ key: { aliases: [ 'value' ] } }` — but there are too few committed
Compose files to identify a stable pattern.  Worth revisiting if more examples
are added to the corpus.

## `home/` git configuration

[`home/git-config.d/common`](https://github.com/tianon/home/blob/720c476e79a50ab0dd133f7187bd046b32cd5b73/git-config.d/common) contains Tianon's global git config.  The settings are mostly self-explanatory personal preferences; a few are genuinely non-obvious to developers who haven't gone looking (`diff.colorMoved = dimmed_zebra`, `rebase.missingCommitsCheck = warn`, `core.pager = less --quit-if-one-screen --no-init`, `tag.sort = creatordate`).

These are tool-configuration preferences rather than file-format or code-style choices, so they are out of scope for the current docs.  Worth revisiting if the documentation scope expands to cover Tianon's general development workflow.

## Vimscript

The [`home/vimrc.d/`](https://github.com/tianon/home/tree/720c476e79a50ab0dd133f7187bd046b32cd5b73/vimrc.d) and [`home/vim-tianon/`](https://github.com/tianon/home/tree/720c476e79a50ab0dd133f7187bd046b32cd5b73/vim-tianon) directories contain
vimscript configuration files.  The style is minimal and follows standard
vimscript conventions with Tianon's usual voice (descriptive comments, issue
links).  Not currently enough distinctive content to warrant a `vim.md`, but
worth revisiting if more vimscript enters the corpus.

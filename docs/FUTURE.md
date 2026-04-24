# Things to explore further

Topics that came up during documentation but were deferred because the corpus
is too sparse, the pattern is too specialised, or the area needs Tianon's
direct input to document correctly.

## The bashbrew library maintenance trio

Three related script patterns that together form Tianon's workflow for
maintaining Docker Official Images (and his personal images):

- **`versions.sh`** — fetches upstream version data, writes `versions.json`
  (documented in [jq-sh.md](jq-sh.md))
- **`apply-templates.sh`** — runs `jq-template.awk` on `Dockerfile.template`
  to generate concrete `Dockerfile` files (documented in [jq-template.md](jq-template.md))
- **`gsl.sh`** (Generate Stackbrew Library) — generates the `library/NAME`
  entry for `docker-library/official-images`, using `arches`, `commit`, and
  `dir` to produce the bashbrew format via jq

All three ultimately feed the `library/` files consumed by bashbrew, used both
for DOI and for Tianon's personal Docker images.  They deserve a unified
document explaining how the pipeline fits together.

Corpus ref: [`tianon-dockerfiles/buildkit/gsl.sh`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/gsl.sh).

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

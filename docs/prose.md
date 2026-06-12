# Prose style: code comments and written documentation

This document covers two closely related things:

1. **Code comment conventions**: the mechanical rules for how comments are formatted across all languages
2. **Writing voice**: the tone and style of the prose itself, whether in comments or documentation

These are kept together because the same voice shows up in both: the person who writes `# TODO something less ... hanky?` in a shell script is the same person who writes "I've eventually grown to dislike every 'Docker DNS' project for one reason or another" in a README.

## Reliability and attribution

Several repos in the corpus are collaborative.  For code comment style, the mechanical patterns (capitalisation, punctuation, `TODO` format) are consistent enough across the corpus that they appear to reflect a house style Tianon established even in collaborative contexts.  For prose *voice*, the most reliable sources are repos where Tianon is clearly the primary or sole author:

**High confidence (Tianon-authored):**
- [`tianon-dockerfiles`](https://github.com/tianon/dockerfiles/tree/2118a1979eff7545e06570d1eefc6434d691e68d) -- personal Dockerfiles
- [`debuerreotype`](https://github.com/debuerreotype/debuerreotype/tree/3c3272fa743e0257ae64081987c500c2923ea963) -- his project
- [`gosu`](https://github.com/tianon/gosu/tree/3d395d499a92ffa47d70c79d24a738b85075f477) -- his project
- [`rawdns`](https://github.com/tianon/rawdns/tree/ea662544c8b03ef7133cc6fc75f63e107265b3f2) -- his project
- [`abstract-sockets`](https://github.com/tianon/abstract-sockets/tree/d08ca7040801fdde8c7ca0b1c844dbf28c2d1a1e) -- his project
- [`hocker`](https://github.com/infosiftr/hocker/tree/ff4d4df2370391ca582abc51a64022501d903577) -- his project
- [`fake-git`](https://github.com/tianon/fake-git/tree/4639d58ce5f6488e448a019acc2b5ffc55d0925f) -- his project
- [`debian-bin`](https://github.com/tianon/debian-bin/tree/d508ea34f15e88b8ac63d71ffb1938fccbc21206) -- his personal scripts
- [`actions/checkout/`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/checkout/) -- his action
- [`blog`](https://github.com/tianon/tianon.github.io/tree/70f318822f383612a86900e95689f72793b9b083) -- personal blog; the longest-form personal prose in the corpus

**Mixed / collaborative** (commit counts as of 2026-04-24, at the corpus HEAD SHAs linked above):
- [`doi/official-images`](https://github.com/docker-library/official-images) -- heavily multi-contributor; Tianon is second-ranked author (~8,786 commits vs yosifkit's ~11,492); DOI image READMEs are generated stubs (see [markdown.md](markdown.md))
- [`doi/bashbrew`](https://github.com/docker-library/bashbrew/tree/d662ff01570964b5f648df009c9269f388285692) -- primarily Tianon (482 commits) with minor contributions from yosifkit, Joe Ferguson, and others
- [`doi/perl-bashbrew`](https://github.com/docker-library/perl-bashbrew/tree/2ab6f478d8cf809b67ebd21930e84c51ad61dc7b) -- primarily Tianon (43 commits) with 3 external commits
- [`meta-scripts`](https://github.com/docker-library/meta-scripts/tree/205031aee2fdfbbd449038afd58f0f0a6915c217) -- primarily Tianon (224 commits) with meaningful contributions from yosifkit, Laurent Goderre, Joseph Ferguson

[`docker-moosefs`](https://github.com/tianon/docker-moosefs) (73 commits, solely Tianon-authored) can be treated as high-confidence.

---

## Code comment conventions

Two distinct modes appear in code files.  The rules in this section (lowercase, no period, fragments) apply to **inline code comments** -- comments that explain *how* or *why* code is written a certain way, adjacent to the code they describe.  When a comment instead documents *what a thing is* -- a package doc comment, a large explanatory header in a file, something that could move to a README with no loss of meaning -- it is **documentation prose** and follows the formal style of the Writing voice section instead: complete sentences, sentence-case capitals, terminal periods.  The test: if the comment would make sense unchanged in a README, it is documentation prose, not an inline comment.

Note that README and documentation prose are also distinct from each other: README is allowed to be playful and tongue-in-cheek because it partly serves a marketing function; documentation prose inside a code file is purely documentation and stays formal.

### Capitalisation: first word is lowercase

Comment sentences begin with a **lowercase letter**, even when they form a complete sentence.  This is consistent across bash, jq, Go, Perl, and AWK.

```bash
# get the directory of this script
dir="$(dirname "$BASH_SOURCE")"
```

```jq
# inject a synthetic blank line at the end of the input stream to make sure we output everything
```

```go
// see `LookupType*` consts for possible values for this type
```

```perl
# optional "os" prefix ("windows-", etc)
```

```awk
# this script assumes gawk! (busybox "awk" is not quite sufficient)
```

**The one exception**: Go exported doc comments must start with the symbol name (Go language convention), which is capitalised by definition:

```go
// Lookup is a wrapper around [ociregistry.Interface.GetManifest]...
func Lookup(ctx context.Context, ref Reference, opts *LookupOptions) ...
```

Corpus refs: [`doi/bashbrew/scripts/jq-template.awk#L1`](https://github.com/docker-library/bashbrew/blob/d662ff01570964b5f648df009c9269f388285692/scripts/jq-template.awk#L1), [`meta-scripts/registry/lookup.go#L12`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/lookup.go#L12), [`debian-bin/jq/deb822.jq#L3`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L3), [`doi/perl-bashbrew/lib/Bashbrew.pm#L16`](https://github.com/docker-library/perl-bashbrew/blob/2ab6f478d8cf809b67ebd21930e84c51ad61dc7b/lib/Bashbrew.pm#L16).

**List items follow the same lowercase rule.** Fragments are strongly preferred -- a list item is almost always a short label, phrase, or clause, not a sentence.  The first word is lowercase.  If a list item is genuinely so large it requires one or more full, complete sentences to express, those sentences may begin with a capital, but that situation is the rare exception.

```go
// good:
//   - no rlimit pre-raise capture: guests see the host value
//   - no seccomp-unotify fast path (Phase 2+): every syscall pays the full round-trip cost

// bad:
//   - No rlimit pre-raise capture: guests see the host value
//   - No seccomp-unotify fast path (Phase 2+): every syscall pays the full round-trip cost
```

### Punctuation: no terminal period

Comments do **not** end with a period, even when the comment is a complete sentence.  This is consistent across all languages.

```bash
# get the directory of this script      ← no period
# TODO --pull flag                       ← no period
```

```go
// intentional subset of https://... to minimize parsing   ← no period
```

```jq
# ignore malformed lines that miss a colon                 ← no period
```

Exceptions: `!` appears for genuine emphasis (`# this script assumes gawk!`), and `?` appears in question-form `TODO`s and consideration comments.  A comment that is a verbatim quote or a genuinely complete and formal thought may start with a capital, but this is rare -- most of Tianon's comments are fragments, not sentences.

The near-absence of sentences in inline code comments is not accidental.  Sentences require capitals, periods, and complete grammatical structure -- all of which add visual noise and pressure to wrap.  The zero occurrences of double-space-after-period in corpus Go inline comments is not a gap in the data; it is the data: sentences do not appear there, so the question never arises.

**Double space after sentence terminators** (`.`, `!`, `?`) applies to documentation prose and exported godoc, where sentences are unavoidable.  It does not apply to inline comments because inline comments should not contain sentences.

### `TODO` format

`TODO` is always **all-caps** and is followed by a **concrete, specific description**.  Vague `TODO`s do not appear.

In code comments (bash, jq, Go, etc.), `TODO` is unquoted -- the comment syntax already marks the context.  In markdown prose, `TODO` is backticked: it is a programming convention appearing in natural language, so it follows the same rule that backticks any technical term (`--flag`, `$VARIABLE`, command names).  This is not consistently visible in the current corpus but is a stated preference.

```bash
# TODO --pull flag
# TODO --image flag
# TODO add this back when I figure out a clean way to do something more akin to a "weekly snapshot" or something so it doesn't have an update every single day
```

```jq
# TODO consider splitting this into a separate function, like "filter_inline_pgp_noise" ?
# TODO should we throw an error if a line contains a line contains a newline? (that's bad input)
```

```go
// TODO allow providing a Descriptor here for more validation and/or for automatic usage of any usable/valid Data field?
// TODO (also, if the provided Reference includes a Digest, we should probably validate it? are there cases where we don't want to / shouldn't?)
```

```perl
# TODO create dedicated Bashbrew::Arch package?
# TODO make this promise-based and non-blocking? (and/or make a dedicated Package for it?)
```

Patterns within `TODO` comments:
- Question-form `TODO`s often end with `?`: `# TODO should we throw an error...?`
- A concrete alternative or example may be included: `like "filter_inline_pgp_noise"`
- Linked considerations are parenthesised: `(and/or make a dedicated Package for it?)`
- Some `TODO`s in jq use a space before the `?`: `# TODO consider ... ?` -- this is an idiosyncrasy of jq comment style

Corpus refs: [`tianon-dockerfiles/buildkit/versions.sh#L17`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L17), [`debian-bin/jq/deb822.jq#L15`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L15), [`meta-scripts/registry/lookup.go#L27-L28`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/lookup.go#L27-L28), [`doi/perl-bashbrew/lib/Bashbrew.pm#L12`](https://github.com/docker-library/perl-bashbrew/blob/2ab6f478d8cf809b67ebd21930e84c51ad61dc7b/lib/Bashbrew.pm#L12).

### Consistent mode within a block

A comment block should stay in one mode throughout -- either all fragments (no terminal periods, lowercase first words) or all complete sentences (periods, sentence-case first words).  Flopping between modes in the same block is confusing:

```go
// bad -- mixes a sentence-like opening with fragment continuations:
// shimFlag is the internal argv[1] that activates shim mode.
// this check must precede cobra init; cobra exits 2 on unknown flags

// good -- one line when it fits, or explicit bullets if longer:
// shimFlag is the argv[1] for shim mode -- must precede cobra init

// or, when there are genuinely separate thoughts:
// shimFlag is the argv[1] for shim mode
// - the Matrix (1999) reference: none of the kernel interfaces are real
// - must precede cobra init; cobra exits 2 on unknown flags
```

The practical rule: if the first line ends with a period, every line in the block should end with a period.  If the first line does not, none should.

### Never wrap a single thought; use explicit bullets for multiple thoughts

Wrapping a single comment thought across multiple lines is always wrong -- this applies to inline comments, exported godoc, and documentation prose alike.  Wrapping makes diffs noisier (adding one word can reflow an entire block), and inconsistent wrapping signals nothing meaningful about structure -- it's just accidental noise from whenever the line got too long.  Go's doc tools (`go doc`, `godoc`, every IDE) render continuation lines as joined text, so a one-long-line comment and its wrapped equivalent produce identical output.

A secondary benefit: single-line comments are grep-friendly.  `grep TODO` returns the complete thought in one output line; a wrapped TODO requires reading surrounding lines to reconstruct the intent.

The two correct forms:

- **Short fragment or long one-liner:** either keep it short, or write one long line.  There is no length limit.  A 120-character comment line is fine; a reflowed 80-character block is not.
- **Explicit `// - item` bullets when there are multiple distinct thoughts.**  Each bullet is its own line.  Bullets make clear that items are separate concepts, not just a wrapped sentence:

```go
// good -- explicit bullets, each a separate fragment:
// - captures rlimits before any raise
// - sets umask(0)
// - exits if _SIMULACRA_FUNC is set

// avoid -- wrapping makes it look like one thought:
// captures rlimits before any raise, sets umask(0), and exits if
// _SIMULACRA_FUNC is set
```

### URL citations

A bare URL on its own comment line cites a source without additional text:

```bash
# https://github.com/jberger/Mojolicious-Plugin-TailLog/...
# https://metacpan.org/pod/Capture::Tiny
```

```jq
# https://manpages.debian.org/testing/dpkg-dev/deb822.5.en.html
# https://www.debian.org/doc/debian-policy/ch-controlfields.html#version
```

```go
// intentional subset of https://github.com/opencontainers/image-spec/blob/v1.1.0/specs-go/v1/index.go#L21 to minimize parsing
```

When a URL is cited alongside other text, the URL comes at the end:

```perl
# TODO make this promise-based and non-blocking? (and/or make a dedicated Package for it?)
# https://github.com/jberger/Mojolicious-Plugin-TailLog/...
```

URLs in comments are always **full URLs** (never shorthand like `#123` unless in markdown linking context).

Corpus refs: [`debian-bin/jq/deb822.jq#L1`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L1), [`doi/perl-bashbrew/lib/Bashbrew.pm#L43-L46`](https://github.com/docker-library/perl-bashbrew/blob/2ab6f478d8cf809b67ebd21930e84c51ad61dc7b/lib/Bashbrew.pm#L43-L46).

### Quoted examples within comments

Values, names, and short examples appear in `"double quotes"` within comment text:

```bash
# if suite isn't "{debian,ubuntu}-xxx", then we should try to auto-detect
echo >&2 "error: unknown variant: '$variant'"
```

```perl
# optional "os" prefix ("windows-", etc)
# "architecture" bit ("arm64", "s390x", etc)
# optional "variant" suffix ("v7", "v6", etc)
# "riscv64" is not "risc, v64" 😂
```

```go
// unspecified implies [LookupTypeManifest]
```

### `etc` without a period

`etc` appears consistently without a trailing period in parenthetical lists:

```perl
# ("arm64", "s390x", etc)
# ("v7", "v6", etc)
```

```jq
# ("windows-", etc)
```

### Parenthetical asides

Parenthetical qualifications appear freely throughout comments:

```bash
# (especially targeted at Docker images, with comments explicitly describing Docker use cases)
```

```go
// (in the case of a HEAD request, it will be a zero-length reader with just a valid descriptor)
```

```perl
# TODO make this promise-based and non-blocking? (and/or make a dedicated Package for it?)
```

These asides add context or caveats without restructuring the sentence.

### `...` as an informal gesture

Three dots appear for an informal "gesture at a vague concept" rather than as a formal ellipsis:

```bash
# TODO something less ... hanky?
```

```go
// *technically* this should be two separate structs chosen based on mediaType (...), but that makes the code a lot more annoying
```

Corpus ref: [`debian-bin/repo/buildd.sh`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh), [`meta-scripts/registry/manifest-children.go#L10`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/manifest-children.go#L10).

### Intentional mid-sentence capitalisation

Occasionally a common word is capitalised mid-sentence to elevate it from adjective or noun to concept -- treating it as a proper noun signals "this is A Thing, not just a description".  The archetypal example is **Wrong** (capital W): when something is not merely incorrect but violates a principle, it is Wrong.

```
fail if any constructs remain that Tianon considers Wrong
```

This device is used sparingly (overuse dilutes it) and only where the conceptual elevation genuinely adds meaning.  It is a **stated preference, not yet consistently observed in the corpus**: the corpus shows it primarily as proper nouns (Cloudflare, Gatekeeper, etc.) rather than as rhetorical elevation.

### Asterisks for emphasis within comments

Single asterisks around a word for emphasis, matching markdown italic style:

```go
// *technically* this should be two separate structs...
```

This appears in Go comments specifically, where the asterisks would not render as markdown but still read as emphasis in the raw text.

### `(sic)` for intentional irregularities

`(sic)` marks intentional "mistakes" or unusual choices to signal they are deliberate:

```jq
empty # tailing comma (sic)
```

(Note: "tailing" rather than "trailing" -- the `(sic)` acknowledges the typo is intentional.)

Corpus ref: [`meta-scripts/oci.jq`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/oci.jq).

### Emoji in comments

Emoji appear sparingly, reserved for genuine humor, irony, or self-aware moments:

- 😂 -- "I find this absurd": `# "riscv64" is not "risc, v64" 😂`
- 😅 -- "I acknowledge this is awkward/embarrassing": appears in YAML comments, shell error messages
- 🙃 -- upside-down smile, meaning "this is cursed": appears in YAML step names
- 🙈 -- "hear no evil", meaning "I'm choosing not to look at this problem": `"" # buildkit has to pull during build 🙈`
- ❗ -- strong emphasis for important warnings

Emoji never appear in formal API documentation (Go exported doc comments).

Corpus refs: [`doi/perl-bashbrew/lib/Bashbrew.pm#L29`](https://github.com/docker-library/perl-bashbrew/blob/2ab6f478d8cf809b67ebd21930e84c51ad61dc7b/lib/Bashbrew.pm#L29), [`meta-scripts/meta.jq#L50`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/meta.jq#L50), [`tianon-dockerfiles/.github/workflows/ci.yml`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/.github/workflows/ci.yml).

### Error and warning messages

Shell error messages follow a consistent format:

```bash
echo >&2 "error: unknown variant: '$variant'"
echo >&2 "warning: '$changes' appears to be invalid or incomplete! (skipping)"
```

Rules:
- `error:` or `warning:` prefix, **always lowercase**
- A literal colon immediately after the prefix, no space before it
- The message itself is lowercase (or sentence-case for the variable part)
- Specific variable values appear in single quotes: `'$variant'`
- The `>&2` redirect always appears immediately after `echo`, before the message
- Exclamation marks are used when appropriate for genuine warnings: `"warning: ... (skipping)"`

This format is the shell-specific manifestation of the same voice seen in prose: lowercase, concrete, specific.

Corpus refs: [`tianon-dockerfiles/buildkit/versions.sh#L58`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/buildkit/versions.sh#L58), [`debian-bin/repo/incoming.sh`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/incoming.sh).

### `/` as shorthand for "or"

A slash between alternatives is used both in comments and in prose:

```go
// whether or not to do a HEAD instead of a GET (will still return ..., but with an empty body / zero bytes)
```

```bash
# TODO alternatives and aptitude resolver should be optional somehow (and more sbuild flags should be possible to add)
```

This appears in Go comments especially.  It reads as "and/or" or "or", depending on context.

### `and/or` phrasing

`and/or` appears explicitly when both possibilities should be considered:

```perl
# TODO make this promise-based and non-blocking? (and/or make a dedicated Package for it?)
```

---

## Git commit messages

Commit messages follow the standard imperative, sentence-case form -- the same fragment style as code comments:

- No trailing period
- Sentence case (first word capitalised)
- Imperative mood: "Update", "Fix", "Add", "Remove", "Rename", "Handle", "Prepare"
- Version bumps: **"Update NAME to VERSION"**: consistent across all automated and manual version updates
- Parenthetical context appended when needed: `"Handle the case where distro-info-data isn't already installed (bookworm/mips64le)"`
- Emoji used sparingly to convey tone -- frustration, irony, or wry commentary on external constraints: `"Update to actions/checkout@v4 🙃"` (the 🙃 here is disparagement of GitHub forcing version-pinning churn on users, not self-deprecation)
- No GitHub issue, PR, or discussion references -- neither full URLs nor `#NNN` shorthand; both forms generate notification spam on the referenced thread

Examples from the corpus:

```
Update tinygo to 0.41.1
Fix typo
Rename deb822 function to deb822_parse (matching dpkg_version_parse)
Add functions for explicitly parsing dpkg versions
Remove pgp-happy-eyeballs
Install distro-info-data conditionally (if newer)
Adjust auto update to run a little earlier
```

Corpus refs: [`tianon-dockerfiles` log](https://github.com/tianon/dockerfiles/commits/2118a1979eff7545e06570d1eefc6434d691e68d), [`debian-bin` log](https://github.com/tianon/debian-bin/commits/d508ea34f15e88b8ac63d71ffb1938fccbc21206).

## Date and time formatting

Dates always use **ISO 8601** format.

- Date only: `YYYY-MM-DD` -- e.g. `2017-05-16`
- Datetime with UTC timezone: `YYYY-MM-DDTHH:MM:SSZ` -- e.g. `2017-01-01T00:00:00Z`

This applies in code, comments, documentation, and filenames.  Other formats (`MM/DD/YYYY`, `DD Mon YYYY`, "January 1st") do not appear.

The `Z` suffix is always included for datetimes -- bare local-time datetimes are not used.

Corpus refs: [`debuerreotype/README.md#L48`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/README.md#L48) (`2017-01-01T00:00:00Z`), [`fake-git/README.md#L18`](https://github.com/tianon/fake-git/blob/4639d58ce5f6488e448a019acc2b5ffc55d0925f/README.md#L18) (`1970-01-01T00:00:00Z`).

## Writing voice (prose documentation)

### Register: informal technical

The writing is simultaneously technically precise and conversationally informal.  It does not write up or down to the reader -- it assumes technical sophistication while using casual vocabulary:

> "This is a simple tool grown out of the simple fact that `su` and `sudo` have very strange and often annoying TTY and signal-forwarding behavior."

> "Since DNS is a protocol (which is a type of API), and Docker has an API, it makes a lot more sense to have DNS be a raw interface to Docker..."

Corpus refs: [`gosu/README.md#L3`](https://github.com/tianon/gosu/blob/3d395d499a92ffa47d70c79d24a738b85075f477/README.md#L3), [`rawdns/README.md#L41`](https://github.com/tianon/rawdns/blob/ea662544c8b03ef7133cc6fc75f63e107265b3f2/README.md#L41).

### First person: both singular and plural

`I` appears when expressing a personal opinion or experience:

> "I've eventually grown to dislike every 'Docker DNS' project for one reason or another..."
> "my favorite thing to say about databases: if you have the same data in two places..."

`we` appears when describing what the project does:

> "The goal is to create an auditable, reproducible process for creating rootfs tarballs..."
> "...so we get to adapt instead."

Corpus refs: [`rawdns/README.md#L45-L46`](https://github.com/tianon/rawdns/blob/ea662544c8b03ef7133cc6fc75f63e107265b3f2/README.md#L45-L46), [`debuerreotype/README.md#L18`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/README.md#L18).

### Wry observations

A recurring pattern: a statement of fact followed by a wry implication or observation:

> "Once the user/group is processed, we switch to that user, then we `exec` the specified process and `gosu` itself is no longer resident or involved in the process lifecycle at all.  This avoids all the issues of signal passing and TTY, and punts them to the process invoking `gosu` and the process being invoked by `gosu`, where they belong."

The phrase "where they belong" is the wry part -- technically accurate but delivered with a shrug.

> "The most important caveat here is that this is all *really* hacky..."
> "don't expect it to work as-is!"

Corpus refs: [`gosu/README.md#L17-L18`](https://github.com/tianon/gosu/blob/3d395d499a92ffa47d70c79d24a738b85075f477/README.md#L17-L18), [`fake-git/README.md#L28`](https://github.com/tianon/fake-git/blob/4639d58ce5f6488e448a019acc2b5ffc55d0925f/README.md#L28), [`docker-postgres-upgrade/README.md#L3`](https://github.com/tianon/docker-postgres-upgrade/blob/ffec3042572b093ab2c61310945f51d6d00fb09c/README.md#L3).

### Self-deprecating honesty

Problems, hacks, and limitations are stated directly without minimisation:

> "This is all *really* hacky, and prone to breakage at any/every turn."
> "I've eventually grown to dislike every 'Docker DNS' project..."
> "the core of how `gosu` works is stolen directly from how Docker/libcontainer itself starts an application"

Corpus refs: [`fake-git/README.md#L28-L29`](https://github.com/tianon/fake-git/blob/4639d58ce5f6488e448a019acc2b5ffc55d0925f/README.md#L28-L29), [`gosu/README.md#L5`](https://github.com/tianon/gosu/blob/3d395d499a92ffa47d70c79d24a738b85075f477/README.md#L5).

### Colloquial vocabulary

Internet-culture vocabulary and informal terms appear without apology:

- `wat` as a section header (rhetorical surprise)
- `hanky` (as in "less ... hanky")  
- `hacky` (used positively to describe pragmatic approaches)
- `yolo` (appears in `Dockerfile.yolo` in the corpus)

Corpus refs: [`rawdns/README.md#L39`](https://github.com/tianon/rawdns/blob/ea662544c8b03ef7133cc6fc75f63e107265b3f2/README.md#L39), [`tianon-dockerfiles/true/`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/true/), [`debian-bin/repo/buildd.sh`](https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/repo/buildd.sh).

### Parenthetical qualifications

Parenthetical asides are extremely common in prose, adding context, nuance, or caveats:

> "Reproducible, [snapshot](http://snapshot.debian.org)-based Debian rootfs builds (especially for Docker)."
> "The core use case for `gosu` is to step _down_ from `root` to a non-privileged user during container startup (specifically in the `ENTRYPOINT`, usually)."

Corpus ref: [`debuerreotype/README.md#L5`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/README.md#L5), [`gosu/README.md#L21`](https://github.com/tianon/gosu/blob/3d395d499a92ffa47d70c79d24a738b85075f477/README.md#L21).

### Dashes

Unicode dashes never appear.  Both the em-dash (`—`) and en-dash (`–`) are replaced by ASCII equivalents:

- **Em-dash → ` -- `** (space, double hyphen, space): used very liberally for clause breaks, asides, and pivots in running prose, and as a label-description separator in list items:

  > "it's *technically* defensible but practically sloppy -- and the sloppiness matters"

  ```markdown
  - `cgroup_enable=memory` -- enable "memory accounting" for containers
  - `swapaccount=1` -- enable "swap accounting" for containers
  ```

- **En-dash → `-`** (single hyphen): used where a typographic en-dash would appear (eg. in ranges).

Corpus refs: [`blog/_posts/2026-05-20-container-security.md#L6`](https://github.com/tianon/tianon.github.io/blob/70f318822f383612a86900e95689f72793b9b083/_posts/2026-05-20-container-security.md#L6), [`blog/_posts/2016-12-07-docker-setup.md#L106-L108`](https://github.com/tianon/tianon.github.io/blob/70f318822f383612a86900e95689f72793b9b083/_posts/2016-12-07-docker-setup.md#L106-L108).

### Concrete specificity

Abstract descriptions are almost immediately followed by concrete examples:

> "...locking (for singleton-style processes), but without files"
> "step _down_ from `root` to a non-privileged user during container startup (specifically in the `ENTRYPOINT`, usually)"

Vague motivation statements are avoided.  "I've eventually grown to dislike" is followed immediately by *why*: "treating DNS like a database".

### Acknowledgements and credits

Sources and inspirations are cited explicitly and graciously:

> "The core of how `gosu` works is stolen directly from how Docker/libcontainer itself starts an application..."
> "This is based on [lamby](https://github.com/lamby)'s work for reproducible `debootstrap`"

Corpus refs: [`gosu/README.md#L5`](https://github.com/tianon/gosu/blob/3d395d499a92ffa47d70c79d24a738b85075f477/README.md#L5), [`debuerreotype/README.md#L7`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/README.md#L7).

### Naming explanations

Non-obvious project names get an explanation:

> ### "Debuerreotype"?
> The name is an attempt at riffing off the photography basis of the word "snapshot".

Corpus ref: [`debuerreotype/README.md#L12-L14`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/README.md#L12-L14).

### `## Why?` section

Nearly every personal project README has a `## Why?` or `## why` section explaining the motivation for the project's existence.  This is not boilerplate -- each one articulates a genuine frustration or gap being filled.

Corpus refs: [`gosu/README.md#L37`](https://github.com/tianon/gosu/blob/3d395d499a92ffa47d70c79d24a738b85075f477/README.md#L37), [`rawdns/README.md#L44`](https://github.com/tianon/rawdns/blob/ea662544c8b03ef7133cc6fc75f63e107265b3f2/README.md#L44), [`hocker/README.md#L5`](https://github.com/infosiftr/hocker/blob/ff4d4df2370391ca582abc51a64022501d903577/README.md#L5).

---

## Notable omissions

Things that do not appear in Tianon's comments or documentation prose:

- Capital letter to start a comment sentence (except Go exported doc comments)
- Period at the end of a comment line
- Vague `TODO`s (`# TODO fix this`, `# TODO improve`)
- `e.g.` or `i.e.` with periods -- `ie,` is Tianon's shorthand for `i.e.,` ("that is": a restatement, not an example); punctuationally informal but semantically correct in prose; in CLI usage blocks (`eg: ./gosu tianon bash`), `eg:` is correct and `ie:` would be wrong -- those lines are genuinely illustrative examples, not restatements; `eg,` does not appear in corpus prose (yet)
- `etc.` with a period -- always `etc` without
- Passive voice in error messages (`"an error occurred"`) -- always active and specific
- Over-hedging in error messages (`"there may have been a problem"`) -- states the problem directly
- Marketing language or superlatives in documentation
- Formal register / corporate tone
- `s/he`, `they` (singular) or other gender-neutral contortions -- documentation addresses "you" directly or uses "we" for the project
- Punctuation inside quotation marks -- terminators (`.`, `?`, `!`) always go *outside* the closing quote, contrary to American typographic convention: `"foo bar".` not `"foo bar."` -- this is the logical-quotation style; when a sentence ends with a quoted fragment and also ends with `?` or `!`, the terminator is typically spaced from the closing quote: `this question ends with a "quoted bit" ?` (space before the `?`)
- Curly/typographic quotation marks -- always straight ASCII `"..."` and `'...'` in all files, not just markdown
- Unicode em-dash `—` -- always ` -- ` (spaced double hyphen) instead; see *Dashes* above
- Unicode en-dash `–` -- always `-` (single hyphen) instead; see *Dashes* above

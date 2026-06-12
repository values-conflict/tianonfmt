# Markdown style

Covers `.md` files -- READMEs, installation guides, and other prose documentation.

## Reliability note: DOI vs personal projects

The corpus contains two distinct sources of markdown:

**Personal projects** (most reliable for style inference):
[`gosu`](https://github.com/tianon/gosu/tree/3d395d499a92ffa47d70c79d24a738b85075f477), [`rawdns`](https://github.com/tianon/rawdns/tree/ea662544c8b03ef7133cc6fc75f63e107265b3f2), [`abstract-sockets`](https://github.com/tianon/abstract-sockets/tree/d08ca7040801fdde8c7ca0b1c844dbf28c2d1a1e), [`debuerreotype`](https://github.com/debuerreotype/debuerreotype/tree/3c3272fa743e0257ae64081987c500c2923ea963), [`hocker`](https://github.com/infosiftr/hocker/tree/ff4d4df2370391ca582abc51a64022501d903577), [`fake-git`](https://github.com/tianon/fake-git/tree/4639d58ce5f6488e448a019acc2b5ffc55d0925f), [`docker-postgres-upgrade`](https://github.com/tianon/docker-postgres-upgrade/tree/ffec3042572b093ab2c61310945f51d6d00fb09c), [`blog`](https://github.com/tianon/tianon.github.io/tree/70f318822f383612a86900e95689f72793b9b083)

These READMEs reflect Tianon's preferred markdown style without external constraints.

**DOI (Docker Official Images)** repos ([docker-library/official-images](https://github.com/docker-library/official-images) image source repos):
The image READMEs in these directories are generated stubs that redirect to `docker-library/docs`.  The actual image descriptions go through [`tianon/markdownfmt`](https://github.com/tianon/markdownfmt) -- a fork of [`shurcooL/markdownfmt`](https://github.com/shurcooL/markdownfmt) -- and have historically been shaped by Docker Hub's markdown rendering quirks.

The two most significant things `markdownfmt` does that Tianon finds annoying but lives with:
- **Paragraph collapsing**: wrapped paragraphs are joined onto a single line (no line-length limit enforced in output)
- **Whitespace normalisation**: all inter-word whitespace is collapsed to a single space, eliminating any two-spaces-after-period habit

Tianon's additions to the fork are all bug fixes (escaped periods after links, empty-file handling, code-block newline trimming) -- none change the paragraph or whitespace behaviour.  The personal-project READMEs also use single spaces throughout, so these are constraints Tianon lives with across the board rather than preferences enforced elsewhere.

**These DOI-processed files should not be taken as representative of Tianon's preferred markdown style.**

## Headers

**ATX style always**: `#` prefix, never setext underline (`===` or `---`).

```markdown
# Project Title

## Why?

### Sub-topic
```

`##` is the most common section level.  `###` appears occasionally for nested sections but is not common in personal project READMEs.

**Blog posts differ**: the post title is supplied by Jekyll frontmatter (`title: ...`), so the body's first-level sections use `#` -- not `##`.  `##` would then be a subsection of a `#` section.  This is not a style inconsistency; it is a structural consequence of where the title lives.

Section names follow no strict capitalisation rule but tend to be:
- Title-case for formal sections: `## Usage`, `## Installation`, `## Warning`, `## Caveat`
- Capitalised question-form: `## Why?`, `## What?`
- Occasionally lowercase and/or colloquial for personality: `## why`, `## wat`, `## how`
- ALL CAPS for strong emphasis: `## SHOW ME`

The `## Why?` section (explaining project motivation) appears in nearly every personal project README.

Corpus refs: [`rawdns/README.md#L39-L52`](https://github.com/tianon/rawdns/blob/ea662544c8b03ef7133cc6fc75f63e107265b3f2/README.md#L39-L52), [`hocker/README.md#L5-L7`](https://github.com/infosiftr/hocker/blob/ff4d4df2370391ca582abc51a64022501d903577/README.md#L5-L7), [`debuerreotype/README.md#L12`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/README.md#L12).

## Code blocks

**Fenced with triple backticks**, never indented.  A language identifier is always provided.  When a code block itself contains fenced code blocks (e.g. a markdown example showing a fenced block), the outer fence uses four or more backticks -- a fence closes at the next fence of equal or greater length, so triple-backtick inner fences would incorrectly close the outer block.

**`--tidy` reformats fenced block contents** using the appropriate sub-formatter for the language tag.  Tags supported: `dockerfile`, `jq`, `bash`, `sh`, `shell`.  Untagged fences are auto-detected (a block opening with `FROM` or a `#!` shebang is reformatted; otherwise the block is left unchanged).  If the contents cannot be parsed, they are left unchanged and a warning is emitted.  Because `--tidy` is applied to the whole file, fenced block contents are also tidied -- there is no way to format-only without tidy inside a tidy markdown pass.  `console` and other unrecognised tags are always left unchanged.

The critical distinction between language identifiers:

- **`console`**: interactive terminal session, showing the `$` prompt and command output.  Used for showing what the user would actually type and see.
- **`bash`**: a shell script being shown as an example (a source listing, not a session).
- **`json`**, **`dockerfile`**, **`yaml`**, etc. -- for their respective file types.

```markdown
```console
$ gosu nobody bash
$ 
```
```

```markdown
```bash
#!/usr/bin/env bash
set -Eeuo pipefail
```
```

`shell` is not used as a language identifier.  `sh` does appear, but only for shell config snippets that are neither a complete script nor an interactive session -- the archetypal use is a `GRUB_CMDLINE_LINUX_DEFAULT` excerpt shown in isolation.

Corpus refs: [`rawdns/README.md#L28-L33`](https://github.com/tianon/rawdns/blob/ea662544c8b03ef7133cc6fc75f63e107265b3f2/README.md#L28-L33), [`hocker/README.md#L15-L24`](https://github.com/infosiftr/hocker/blob/ff4d4df2370391ca582abc51a64022501d903577/README.md#L15-L24), [`fake-git/README.md#L33-L53`](https://github.com/tianon/fake-git/blob/4639d58ce5f6488e448a019acc2b5ffc55d0925f/README.md#L33-L53), [`blog/_posts/2016-12-07-docker-setup.md#L113`](https://github.com/tianon/tianon.github.io/blob/70f318822f383612a86900e95689f72793b9b083/_posts/2016-12-07-docker-setup.md#L113).

## Inline code

Backticks are used liberally for:
- Command names: `` `gosu` ``, `` `git` ``, `` `docker` ``
- File and path names: `` `Dockerfile` ``, `` `/etc/rawdns/config.json` ``
- Environment variable names: `` `FAKEGIT_GO_SEMVER` ``
- Flags and arguments: `` `--dns` ``, `` `-buildvcs=true` ``
- Package/module paths: `` `github.com/miekg/dns` ``
- Dockerfile keywords when referenced as concepts: `` `ENTRYPOINT` ``
- Version strings and other literal values

Corpus refs: [`gosu/README.md`](https://github.com/tianon/gosu/blob/3d395d499a92ffa47d70c79d24a738b85075f477/README.md), [`fake-git/README.md#L9-L23`](https://github.com/tianon/fake-git/blob/4639d58ce5f6488e448a019acc2b5ffc55d0925f/README.md#L9-L23).

## Unordered lists

Bullet character is always `-`.  `*` and `+` are never used.

```markdown
- automatically cleaned up when the last process using them exits
- locking (for singleton-style processes), but without files
- not tied to specific files on-disk
```

**Tight lists** (no blank lines between items) are preferred for short bullet points.  A blank line between items is only added when each item itself spans multiple lines.

Nested lists use 2-space indentation per level:

```markdown
- **X11 / Xorg**
  - security impact: keylogging, screen capture
  - typical sockets: `@/tmp/.X11-unix/X0`
  - mitigation / further reading:
    - https://tstarling.com/blog/...
```

Corpus refs: [`abstract-sockets/README.md#L4-L7`](https://github.com/tianon/abstract-sockets/blob/d08ca7040801fdde8c7ca0b1c844dbf28c2d1a1e/README.md#L4-L7), [`abstract-sockets/README.md#L24-L30`](https://github.com/tianon/abstract-sockets/blob/d08ca7040801fdde8c7ca0b1c844dbf28c2d1a1e/README.md#L24-L30).

## Ordered lists

Used for step-by-step instructions where order matters:

```markdown
1. download `gosu-$(dpkg --print-architecture)` as `gosu`
2. download `gosu-$(dpkg --print-architecture).asc` as `gosu.asc`
3. fetch my public key: `gpg --batch ...`
4. `gpg --batch --verify gosu.asc gosu`
5. `chmod +x gosu`
```

## Line breaks within list items

A "soft line break" (two trailing spaces) is used to wrap long content within a single list item when the content is a continuation of the same point rather than a new point.  This is the one case where trailing whitespace is intentional and meaningful -- it renders as `<br>` and keeps continuation text visually attached to its bullet:

```markdown
- `FAKEGIT_GO_SEMVER`  
  the actual semantic version number you want embedded in the build metadata; must match [Go's `vMAJOR[.MINOR[.PATCH]]`](https://pkg.go.dev/golang.org/x/mod/semver)  
  additionally, Go tends to be even pickier about this for VCS metadata...
```

Corpus ref: [`fake-git/README.md#L9-L17`](https://github.com/tianon/fake-git/blob/4639d58ce5f6488e448a019acc2b5ffc55d0925f/README.md#L9-L17).

## Emphasis

**Bold** uses `**double asterisks**` -- for genuinely important warnings or for heading-like text within a list item:

```markdown
**even in those cases, BEFORE suggesting an addition to this list**, we expect you...
```

```markdown
- **X11 / Xorg**
```

**Italic** uses `_underscores_` for mild emphasis within prose:

```markdown
The core use case for `gosu` is to step _down_ from `root` to a non-privileged user...
```

When asterisks appear around a word in prose (`*really*`), it is for the same italic effect.

There is no strict rule separating `_` from `*` for italics -- both appear -- but `_` is slightly more common in personal READMEs.

## Links

Inline link format `[text](url)` for contextual links:

```markdown
This is based on [lamby](https://github.com/lamby)'s work for reproducible `debootstrap`.
```

Bare URLs appear in reference-link sections and in lists of resources where the URL is the primary content:

```markdown
- https://gitlab.gnome.org/GNOME/glib/-/merge_requests/911#note_529866
- https://gitlab.gnome.org/GNOME/at-spi2-core/-/issues/28#note_992076
```

GitHub issue/PR shorthand `#N` is used within the context of the same repo.  External references always use full URLs.

## Tables

GitHub-flavored pipe tables for structured reference information (especially script-to-purpose mappings):

```markdown
| *script* | *purpose* |
| --- | --- |
| `debuerreotype-init` | create the initial "rootfs", given a suite and a timestamp |
| `debuerreotype-chroot` | run a command in the given "rootfs" |
```

- Header cells may use `*italics*` for column name styling
- Alignment markers (`---`) without explicit left/right/center alignment (plain `---`)
- No padding to align the column widths (no extra spaces to make columns line up)

Corpus ref: [`debuerreotype/README.md#L28-L39`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/README.md#L28-L39).

## Inline HTML

`<small>...</small>` is the one HTML tag that appears in markdown prose.  It serves three roles:

- **Standalone footnote-style paragraph** at the end of a post -- caveats, credits, or tangential context that shouldn't interrupt the main body:

  ```markdown
  <small>(this post was written with the assistance of "claude my eyes right out" but all thoughts and understanding are Tianon's)</small>
  ```

- **Inline mid-sentence parenthetical**: when an aside is too long or digressive for normal parentheses but should stay anchored to its host sentence:

  ```markdown
  In a YubiKey <small>(or any other PIV-signing-supporting smart card? do they actually *have* competitors in this specific niche? 🤔)</small>, a given "slot" can hold one single private key.
  ```

- **Table cell decoration**: `<small>0x</small>` to visually downplay a prefix while keeping the adjacent inline code prominent.

All other HTML tags are avoided; markdown syntax covers everything else.

Corpus refs: [`blog/_posts/2026-05-20-container-security.md#L29`](https://github.com/tianon/tianon.github.io/blob/70f318822f383612a86900e95689f72793b9b083/_posts/2026-05-20-container-security.md#L29), [`blog/_posts/2025-07-10-yubi-whati.md#L12`](https://github.com/tianon/tianon.github.io/blob/70f318822f383612a86900e95689f72793b9b083/_posts/2025-07-10-yubi-whati.md#L12), [`blog/_posts/2025-07-10-yubi-whati.md#L18`](https://github.com/tianon/tianon.github.io/blob/70f318822f383612a86900e95689f72793b9b083/_posts/2025-07-10-yubi-whati.md#L18).

## Paragraphs and blank lines

- **One blank line** between paragraphs
- **One blank line** after each heading before the first paragraph
- **No blank line** between a heading and a tight list that immediately follows
- **No trailing blank line** at the end of the file (or at most one)

## Quotation marks in prose

Straight ASCII double quotes `"..."` for quoting terms and names, not typographic curly quotes.  This applies in all files Tianon writes, not just markdown -- see [prose.md](prose.md).

```markdown
"Hocker" is a Docker wrapper for "hacky shell script deployment".
```

## Abbreviations and contractions

Contractions appear freely: `it's`, `they're`, `we're`, `you've`, `can't`.  This is an informal register -- technical precision and conversational tone coexist.

Abbreviations -- these apply universally across all prose, not just markdown (see [prose.md](prose.md)):
- `ie,` (not `i.e.,` -- no periods) for "that is"; grammatically dubious but consistent corpus habit
- `e.g.` with periods for "for example" in documentation prose; in code comments, restructure to avoid or just write the concrete example directly
- `etc` (not `etc.`) -- no trailing period

Corpus refs: [`rawdns/README.md#L35`](https://github.com/tianon/rawdns/blob/ea662544c8b03ef7133cc6fc75f63e107265b3f2/README.md#L35), [`fake-git/README.md#L8`](https://github.com/tianon/fake-git/blob/4639d58ce5f6488e448a019acc2b5ffc55d0925f/README.md#L8).

## Badges / CI status

CI status badges appear near the top of the README, after the title and any initial one-line description, often in a brief links section:

```markdown
# rawdns

- [Docker Hub](https://index.docker.io/u/tianon/rawdns/)
- [GitHub](https://github.com/tianon/rawdns)
- [![Smoke Test](https://github.com/.../badge.svg)](https://github.com/.../actions)
```

Or inline after the title + one-line summary:

```markdown
# Debuerreotype

[![GitHub CI](https://github.com/debuerreotype/debuerreotype/workflows/...)](...)

Reproducible, [snapshot](http://snapshot.debian.org)-based Debian rootfs builds...
```

## Notable omissions

- Setext headers (`===` or `---` underlines) -- never used
- Indented code blocks (4-space indent) -- always fenced
- `*` or `+` for unordered list bullets -- always `-`
- `i.e.`, `e.g.`, `etc.` with periods -- always written without
- HTML block elements -- avoided in general; `<small>` is the one exception (see *Inline HTML* above)
- Reference-style links (`[text][label]` with a `[label]: url` reference block) -- inline links only
- Horizontal rules (`---` or `***`) as section separators
- Explicit `<br>` for line breaks (uses two trailing spaces instead)

# Dockerfile style

Covers `Dockerfile` and `Dockerfile.*` files (excluding `Dockerfile.template` files — those are a separate format, see [jq-template.md](jq-template.md)).

## Instruction keywords

**All instruction keywords are UPPERCASE.**  `FROM`, `RUN`, `COPY`, `ADD`, `ENV`, `ARG`, `WORKDIR`, `EXPOSE`, `CMD`, `ENTRYPOINT`, `LABEL`, `USER`, `VOLUME`, `STOPSIGNAL`, `HEALTHCHECK`, `SHELL`, `ONBUILD`.

Lowercase or mixed-case keywords never appear.

Corpus refs: All Dockerfiles in the corpus.

## Blank lines between instructions

**A single blank line** separates instruction groups.  Two blank lines never appear between instructions.  No blank line appears between logically related instructions (e.g., a `RUN` followed immediately by its associated `ENV`).

```dockerfile
FROM debian:trixie-slim

RUN set -eux; \
	apt-get update; \
	...

ENV WGETRC /.wgetrc
RUN echo 'hsts=0' >> "$WGETRC"
```

Corpus refs: [`debuerreotype/Dockerfile`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/Dockerfile), [`tianon-dockerfiles/steam/Dockerfile`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile).

## Generated file header

Dockerfiles generated from templates always start with this exact comment block:

```dockerfile
#
# NOTE: THIS DOCKERFILE IS GENERATED VIA "apply-templates.sh"
#
# PLEASE DO NOT EDIT IT DIRECTLY.
#
```

Hand-authored Dockerfiles may start with a comment block explaining usage or bootstrapping instructions, but the generated-warning format above is reserved for generated files.

Corpus ref: [`docker-qemu/10.1/Dockerfile#L1-L5`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/10.1/Dockerfile#L1-L5).

## `FROM`

A single `FROM` per stage.  Multi-stage builds use multiple `FROM` instructions.  `AS name` naming follows the pattern of the build stage purpose:

```dockerfile
FROM tianon/nolibc AS build

FROM scratch
```

A blank line follows `FROM`.

## `ENV`

Single-value `ENV` goes on one line, using the key-value form:

```dockerfile
ENV LANG en_US.utf8
ENV HOME /home/steam
```

The old `ENV key value` form (with a space separator, no `=`) is used.  The `key=value` form with `=` is not used for single-value ENV.

Multi-value `ENV` uses the continuation form:

```dockerfile
ENV QEMU_KEYS \
# Michael Roth
		CEACC9E15534EBABB82D3FA03353C9CEF108B584 \
# https://wiki.qemu.org/...
		ABCDEF...
```

Corpus refs: [`tianon-dockerfiles/steam/Dockerfile#L19-L20`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile#L19-L20), [`docker-qemu/10.1/Dockerfile#L69-L72`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/10.1/Dockerfile#L69-L72).

## `RUN` — general structure

Every non-trivial `RUN` instruction begins with `set -eux; \` as its first command:

```dockerfile
RUN set -eux; \
	apt-get update; \
	apt-get install -y ...; \
	rm -rf /var/lib/apt/lists/*
```

Some simpler scripts use `set -ex` (without `-u`).  In Dockerfiles specifically — unlike standalone shell scripts — `set -eux` is the norm, not `set -Eeuo pipefail`, because Docker's default shell is `/bin/sh` (not bash), and `-E`, `-o pipefail` are bash-specific.

Corpus refs: [`debuerreotype/Dockerfile#L14`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/Dockerfile#L14), [`tianon-dockerfiles/steam/Dockerfile#L3`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile#L3).

## RUN — continuation line indentation

Continuation lines within a `RUN` instruction are indented with **one tab**:

```dockerfile
RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		pkg1 \
		pkg2 \
	; \
	rm -rf /var/lib/apt/lists/*
```

Argument lists for commands like `apt-get install` are indented with **two tabs** (one for the continuation, one for being an argument to the preceding command):

```dockerfile
	apt-get install -y --no-install-recommends \
		ca-certificates \
		curl \
		wget \
	; \
```

The semicolon-backslash `;  \` that closes an argument list is at **one tab** (back to the continuation level), with the `;` immediately after `\` (no space between).

Corpus refs: [`tianon-dockerfiles/steam/Dockerfile#L3-L16`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile#L3-L16), [`debuerreotype/Dockerfile#L14-L29`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/Dockerfile#L14-L29).

## RUN shell style

The shell code inside `RUN` instructions is **POSIX sh**, not Bash — Docker's default `/bin/sh` is BusyBox `sh` on Alpine and `dash` on Debian-based images.  Bash-specific features (`[[ ]]`, arrays, `$'...'`, process substitution, here-strings `<<<`, etc.) do not appear inside Dockerfiles unless Bash has been explicitly installed and invoked with `SHELL ["/bin/bash", "-c"]`.  See [bash.md §POSIX compatibility as a design goal](bash.md#posix-compatibility-as-a-design-goal) for the broader context of how this shapes the shell style.

The POSIX sh style conventions that apply:
- `if condition; then ... fi` — `then` on same line
- `case ... in ... esac` — same structure as in [bash.md](bash.md)
- `for x in ...; do ... done` — `do` on same line
- `$()` for command substitution (not backticks)
- `"$var"` — variables always quoted

These are the same conventions as [bash.md](bash.md), with the added constraint of POSIX compatibility.

## RUN — inline comments

Within a `RUN` continuation block, comments appear at **column 0** — no leading whitespace, regardless of the surrounding indentation:

```dockerfile
RUN set -eux; \
	apt-get install -y --no-install-recommends \
		ca-certificates \
# zenity is used during early startup for dialogs and progress bars
		zenity \
# wget is used for uploading crash dumps
		wget \
	; \
	rm -rf /var/lib/apt/lists/*
```

This is a deliberate style choice: inline comments are not part of the shell code, so they don't carry indentation.

Corpus refs: [`tianon-dockerfiles/steam/Dockerfile#L6-L14`](https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile#L6-L14), [`debuerreotype/Dockerfile#L23-L24`](https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/Dockerfile#L23-L24).

## RUN — blank continuation lines

A lone `\` (backslash on a line by itself) is used as a visual separator within a long `RUN` block:

```dockerfile
RUN set -eux; \
	apt-mark auto '.*' > /dev/null; \
	\
	find /usr/local -type f ...; \
	\
	apt-get purge -y --auto-remove ...; \
```

These blank separators group logically related commands.

Corpus ref: [`docker-qemu/10.1/Dockerfile#L228-L247`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/10.1/Dockerfile#L228-L247).

## `COPY`

Single-file or single-directory `COPY` on one line:

```dockerfile
COPY . $DEBUERREOTYPE_DIRECTORY
COPY *.patch /qemu-patches/
COPY start-qemu /usr/local/bin/
```

Multi-source `COPY` uses the space-separated form, not the JSON array form.

## `CMD` and `ENTRYPOINT`

JSON array (exec) form is used:

```dockerfile
CMD ["start-qemu"]
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
```

Shell form is used only when the command requires shell features.

## `STOPSIGNAL`, `EXPOSE`, `WORKDIR`

Each on its own line, no trailing characters:

```dockerfile
STOPSIGNAL SIGHUP

EXPOSE 22
EXPOSE 5900

WORKDIR /tmp
```

Multiple `EXPOSE` instructions are used (one port per line) rather than grouping them.

Corpus ref: [`docker-qemu/10.1/Dockerfile#L251-L255`](https://github.com/tianon/docker-qemu/blob/3ce36843e253ddb7f63a39a6d0a27a7a46762e8b/10.1/Dockerfile#L251-L255).

## Parser directives

Parser directives (`# syntax=`, `# escape=`) appear at the very top before any other content, including comments.  They are rare in the corpus.

## Generated vs hand-authored Dockerfiles

Generated Dockerfiles (produced from `Dockerfile.template` sources) carry the generated-file header and are structurally identical to hand-authored ones — the template system produces the same style.

## Notable omissions

- `set -Eeuo pipefail` inside `RUN` — the simpler `set -eux` is used (POSIX sh, no `-E` or pipefail)
- Heredoc syntax inside `RUN` — not used (only `\` continuation style)
- `ARG` before `FROM` for multi-stage builds — not seen in corpus
- `LABEL` instructions — not common in corpus Dockerfiles
- `HEALTHCHECK` with complex checks — only `HEALTHCHECK NONE` or simple forms
- `USER` before `RUN` — user is set in the image build process, not as a Dockerfile instruction, in most corpus examples
- JSON array form for `ENV` — always uses the space-separated `key value` form
- Multi-value `ENV` with `=` signs — not used (`ENV KEY VALUE` not `ENV KEY=VALUE`)

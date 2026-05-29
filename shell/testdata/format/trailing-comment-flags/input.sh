#!/usr/bin/env bash
set -Eeuo pipefail

sha256sum -c - <<<"${sha256} *${file}"
sha256sum -c - <<<"${sha256} *${file}" # these flags have to be silly for macOS's sake (it has no GNU coreutils)
sha256sum -c - <<<"${sha256} *${file}" # short form, not --check, for tool compat

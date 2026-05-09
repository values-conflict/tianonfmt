#!/usr/bin/env bash
set -Eeuo pipefail

# eval $( with single-quoted format string — exercises evalCmdSubstClose single-quote skip
eval "$(printf '%s=%s' "$key" "$val")"

# eval $( with double-quoted argument — exercises double-quote skip with backslash
eval "$(printf "%q=%q" "$key" "$val")"

# eval $( with nested parens — exercises depth tracking
eval "$(printf '%s' "$(basename "$path")")"

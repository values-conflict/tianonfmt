#!/usr/bin/env bash
set -Eeuo pipefail

# Simple expression: unchanged by jq formatter — original 4-space indent preserved.
var="$(jq -r '
    .foo
    | .bar
' <<<"$input")"

# Complex expression: reformatted by jq formatter — space indent normalised to tabs.
result="$(jq -r '
    {foo:.foo,bar:.bar,baz:[.items[]|select(.active)|.name]}
' <<<"$json")"

#!/usr/bin/env bash
set -Eeuo pipefail

# Simple expression: unchanged by jq formatter — original 4-space indent preserved.
var="$(jq --raw-output '
    .foo
    | .bar
' <<< "$input")"

# Complex expression: reformatted by jq formatter — space indent normalised to tabs.
result="$(jq --raw-output '
    {foo:.foo,bar:.bar,baz:[.items[]|select(.active)|.name]}
' <<< "$json")"

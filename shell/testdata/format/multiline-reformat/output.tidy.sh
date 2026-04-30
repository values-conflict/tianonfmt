#!/usr/bin/env bash
set -Eeuo pipefail

json="$(jq -n --arg foo "bar" --argjson x "1" '
	{"foo": $foo, "x": $x}
	| .total = (.x + 1)
')"
echo "$json"

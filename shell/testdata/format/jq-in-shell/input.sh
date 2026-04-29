#!/usr/bin/env bash
set -Eeuo pipefail

json="$(jq -n --arg foo "bar" '{"foo": $foo}')"
result="$(jq -r '.foo' <<<"$json")"
echo "$result"

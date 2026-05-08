#!/usr/bin/env bash
set -Eeuo pipefail

json="$(jq --null-input --arg foo "bar" '{"foo": $foo}')"
result="$(jq --raw-output '.foo' <<< "$json")"
echo "$result"

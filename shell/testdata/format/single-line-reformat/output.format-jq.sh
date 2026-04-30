#!/usr/bin/env bash
set -Eeuo pipefail

result="$(jq -n --arg foo "bar" '{ foo: $foo }')"
echo "$result"

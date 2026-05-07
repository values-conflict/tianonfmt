#!/usr/bin/env bash
set -Eeuo pipefail

result="$(jq --null-input --arg foo "bar" '{"foo": $foo}')"
echo "$result"

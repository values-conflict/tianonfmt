#!/usr/bin/env bash
set -Eeuo pipefail

result="$(jq -r '.foo | .bar' <<<"$json")"
echo "$result"

#!/usr/bin/env bash
set -Eeuo pipefail

result="$(jq --raw-output '.foo | .bar' <<< "$json")"
echo "$result"

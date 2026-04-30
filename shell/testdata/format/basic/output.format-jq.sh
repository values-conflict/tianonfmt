#!/usr/bin/env bash
set -Eeuo pipefail

dir="$(mktemp -d)"
exitTrap="$(printf 'rm -rf %q' "$dir")"
trap "$exitTrap" EXIT

for arch in amd64 arm64; do
	file="$dir/$arch.tar"
	curl -fsSL "https://example.com/$arch.tar" -o "$file"
	tar -xzf "$file" -C "$dir"
done

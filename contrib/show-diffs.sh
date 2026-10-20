#!/usr/bin/env bash
# show-diffs -- display git diffs between input and golden output across all
# testdata/format fixtures, skipping pairs where input and output are identical.
#
# Usage:
#   ./contrib/show-diffs.sh [--tidy] [--corpus | --anticorpus] [--ignore-all-space]
#
#   --tidy              compare input vs output.tidy.* (default: input vs output.*)
#   --corpus            only show corpus-* fixtures
#   --anticorpus        only show anticorpus-* fixtures
#   --ignore-all-space  pass -w to git diff (ignore whitespace differences)
set -Eeuo pipefail

tidy=false
filter=''
diffOpts=()

while [ "$#" -gt 0 ]; do
	case "$1" in
		--tidy | -t) tidy=true ;;
		--corpus) filter=corpus ;;
		--anticorpus) filter=anticorpus ;;
		--ignore-all-space | -w) diffOpts+=( --ignore-all-space ) ;;
		*)
			echo >&2 "usage: $0 [--tidy] [--corpus|--anticorpus] [--ignore-all-space]"
			exit 1
			;;
	esac
	shift
done

# the fixtures live relative to the repo root, one directory up from contrib/
cd "$(dirname "$(readlink -f "$BASH_SOURCE")")/.."

found=0

for input in */testdata/format/*/input.*; do
	[ -f "$input" ] || continue

	dir=$(dirname "$input")
	name=$(basename "$dir")
	ext="${input##*/input}"

	case "$filter,$name" in
		corpus,corpus-*) ;;
		corpus,*) continue ;;
		anticorpus,anticorpus-*) ;;
		anticorpus,*) continue ;;
	esac

	if $tidy; then
		output="$dir/output.tidy$ext"
	else
		output="$dir/output$ext"
	fi

	[ -f "$output" ] || continue
	cmp -s "$input" "$output" && continue

	git --no-pager diff --no-index "${diffOpts[@]}" "$input" "$output" || true
	found=$((found + 1))
done

[ "$found" -gt 0 ] || echo "No diffs found."

#!/usr/bin/env bash
set -Eeuo pipefail

# jq expressions inside case branches must be indented one level deeper
# than the assignment, with the closing quote at the assignment's level.

for item in "${items[@]}"; do
	case "$item" in
		stable)
			version="$(jq <<<"$dist" --raw-output '
				first(
					.[]
					| select(.stable == true)
					| .version
					| ltrimstr("go")
				)
				// error("no stable version found")
			')"
			;;
		*.*.*)
			version="$(jq <<<"$dist" --raw-output --arg spec "$item" '
				first(
					.[]
					| select(.version == ("go" + $spec))
					| .version
					| ltrimstr("go")
				)
				// error("version go\($spec) not found")
			')"
			;;
		*)
			if
				! version="$(jq <<<"$dist" --raw-output --arg spec "$item" '
					first(
						.[]
						| select(.stable == true)
						| select(.version | ltrimstr("go") | startswith($spec + "."))
						| .version
						| ltrimstr("go")
					)
					// error("no version matching \($spec) found")
				')" \
					|| [ -z "$version" ] \
					;
			then
				echo >&2 "warning: no match for $item"
				continue
			fi
			;;
	esac
done

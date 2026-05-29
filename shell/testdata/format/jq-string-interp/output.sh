#!/usr/bin/env bash
set -Eeuo pipefail

# jq \(...) interpolations inside shell 'jq' blocks are formatted recursively.
# Inline interpolations are compacted; multi-line object interpolations are
# re-indented to match the jq nesting depth.

# inline: simple variable and field-access interpolations — stay on one line
result=$(jq --raw-output '
	.name as $n
	| @sh "hello \($n), you are \(.age) years old"
')

# multi-line: large object literal inside \(...) gets re-indented
cmd=$(jq --raw-output '
	.tag as $t
	| @sh "my-tool --payload \(
		{
			name: .name,
			version: .version,
			description: .description,
			author: .author,
		}
		| tojson
	)"
')

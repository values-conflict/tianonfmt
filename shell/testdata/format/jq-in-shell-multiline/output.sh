#!/usr/bin/env bash
set -Eeuo pipefail

result=$(jq '
	.foo
	| .bar
' input.json)

jq --arg name "world" '
	{
		greeting: ("Hello, " + $name),
		value: .data,
	}
' input.json

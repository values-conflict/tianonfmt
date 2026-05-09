#!/usr/bin/env bash
set -Eeuo pipefail

# Associative array subscripts with quoted keys inside arithmetic
# exercises arithCloseParen's single-quote and double-quote skip paths
declare -A counts=( ["apples"]=3 ['oranges']=7 )

total=$(( counts["apples"] + counts['oranges'] ))
echo "total: $total"

# Double-quoted key in arithmetic context — also exercises the backslash
# escape path inside the string scanner
label="apples"
echo "count: $(( counts["$label"] ))"

# Nested parens inside arithmetic — exercises depth tracking (case '(': depth++)
result=$(( (counts["apples"] + 1) * 2 ))
echo "$result"

# Array assignments with quoted elements — exercises arrayCloseParen quote-skip paths
arr=( 'hello world' other 'key=val' )
arr2=( "key=value" "other=item" )

# Heredoc with single-quoted word — exercises isUnquotedHeredocWord false path
cat <<-'EOF_INNER'
	raw content: $not_expanded
EOF_INNER

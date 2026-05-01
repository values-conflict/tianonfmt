// Package shell formats shell scripts using mvdan.cc/sh/v3.
//
// It also handles two embedded-language scenarios:
//
//  1. jq-in-shell: `jq '...'` invocations where the single-quoted argument is
//     a jq expression; the expression is reformatted in-place before printing.
//
//  2. RUN-line context: shell commands embedded in Dockerfile RUN instructions
//     use ` \` line continuation instead of newlines; the formatter normalizes
//     tab indentation while respecting that convention.
package shell

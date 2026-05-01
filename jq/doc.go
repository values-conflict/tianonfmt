// Package jq parses and formats jq source files.
//
// The main entry points are [ParseFile] and [ParseExpr] for parsing, and
// [FormatFile] / [FormatFileTidy] / [FormatNode] / [FormatNodeInline] for
// formatting.  [LintFile] reports pedantic style violations that the formatter
// cannot automatically correct.
//
// The AST is rooted at [File] and composed of types that implement [Node].
// All AST types are defined in ast.go; token types are in token.go.
package jq

// Package dockerfile parses and formats Dockerfile source files.
//
// Parsing: [Parse] reads a Dockerfile from source text and returns a [File] AST.
// The grammar follows https://docs.docker.com/reference/dockerfile/ and the moby
// project's daemon/builder/dockerfile package; edge-case behaviour was cross-checked
// against the jq-based reference parser at fmt/dockerfile-parser/dockerfile.jq.
//
// Formatting: [Format] and [FormatWith] apply canonical style rules backed by
// corpus samples:
//
//   - Instruction keywords are uppercased
//   - Continuation lines preserve original leading-tab depth
//   - Inline comments within continuation blocks sit at column 0
//   - RUN shell content is normalized for tab depth
//   - A single blank line separates instruction groups
//   - No trailing whitespace; file ends with a single newline
package dockerfile

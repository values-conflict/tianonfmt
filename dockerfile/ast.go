package dockerfile

// File is the top-level AST node for a parsed Dockerfile.
type File struct {
	Directives   []*Directive  // # syntax= / # escape= lines before first instruction
	Instructions []*Instruction
}

// Directive is a parser directive comment at the top of a Dockerfile.
// e.g. "# syntax=docker/dockerfile:1" or "# escape=`"
type Directive struct {
	Name  string // "syntax" or "escape" (lowercased)
	Value string
	Raw   string // the original comment line
}

// Instruction represents a single logical Dockerfile instruction
// (possibly spanning multiple physical lines via continuation).
type Instruction struct {
	// Keyword is the uppercased instruction keyword, e.g. "FROM", "RUN".
	// It is "COMMENT" for comment-only lines and "" for blank lines.
	Keyword string

	// Args is the raw argument text after the keyword (logical line, with
	// continuation escapes removed and lines joined with space).
	Args string

	// Lines are the original physical source lines that make up this instruction,
	// in order.  They include line-ending comment lines interspersed in RUN blocks.
	Lines []Line

	// StartLine and EndLine are 1-based line numbers in the source file.
	StartLine int
	EndLine   int
}

// Line is a single physical source line with its classification.
type Line struct {
	// Text is the raw line text, without the trailing newline.
	Text string
	// Kind classifies the line.
	Kind LineKind
}

// LineKind classifies a physical source line within an instruction.
type LineKind int

const (
	LineKindInstruction LineKind = iota // first line of an instruction
	LineKindContinuation                // continuation of the previous line (after \)
	LineKindComment                     // a # comment line inside a continuation block
	LineKindBlank                       // a blank line between instructions
	LineKindDirective                   // a parser directive line
)

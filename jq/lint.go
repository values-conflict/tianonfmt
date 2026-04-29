package jq

// Violation describes a pedantic style issue that cannot be auto-fixed.
type Violation struct {
	Line int
	Msg  string
}

// LintFile checks the parsed File for pedantic style violations that --tidy
// cannot automatically rewrite.  src is the original source text, used to
// compute 1-based line numbers from byte offsets.
func LintFile(f *File, src string) []Violation {
	var out []Violation
	lintNullInElse(f, src, &out)

	Walk(f, func(n Node) bool {
		b, ok := n.(*BinOp)
		if !ok {
			return true
		}
		switch b.Op {
		case "==":
			bl, ok := b.Right.(*BoolLit)
			if !ok {
				return true
			}
			// expr == false / expr == true: Tianon prefers "expr | not" or
			// the bare expression, but auto-rewriting changes semantics
			// (null | not is true; null == false is false).
			if !bl.Val {
				out = append(out, Violation{
					Line: posToLine(src, b.At),
					Msg:  `"== false": use "| not" instead (note: "| not" also matches null — verify semantics before changing)`,
				})
			} else {
				out = append(out, Violation{
					Line: posToLine(src, b.At),
					Msg:  `"== true": use the expression directly or "| . == true" for strict boolean check`,
				})
			}
		case "!=":
			bl, ok := b.Right.(*BoolLit)
			if !ok {
				return true
			}
			if !bl.Val {
				// expr != false: semantically "not exactly false" — truthy or null pass.
				// Use "expr" (truthy) or "expr | . != false" for strict check.
				out = append(out, Violation{
					Line: posToLine(src, b.At),
					Msg:  `"!= false": use the expression directly for a truthy check (note: this also passes null)`,
				})
			} else {
				// expr != true: semantically "not exactly true".
				// Use "expr | not" but note null | not is also true.
				out = append(out, Violation{
					Line: posToLine(src, b.At),
					Msg:  `"!= true": use "| not" instead (note: "| not" also matches null — verify semantics before changing)`,
				})
			}
		}
		return true
	})
	return out
}

// lintNullInGenerator checks if a NullLit appears in a position where empty
// would be semantically more appropriate — specifically, as the else branch of
// an if inside an array or comma expression.  This is heuristic: it only fires
// when a NullLit appears directly as the Else branch of an IfExpr.
func lintNullInElse(f *File, src string, out *[]Violation) {
	Walk(f, func(n Node) bool {
		ie, ok := n.(*IfExpr)
		if !ok || ie.Else == nil {
			return true
		}
		if _, isNull := ie.Else.(*NullLit); isNull {
			*out = append(*out, Violation{
				Line: posToLine(src, pos(ie.Else)),
				Msg:  `"else null": use "else empty" in generators/arrays to produce no output (semantically equivalent only when filtering nulls is intended)`,
			})
		}
		return true
	})
}

// posToLine converts a byte offset to a 1-based line number in src.
func posToLine(src string, p Pos) int {
	line := 1
	for i := 0; i < int(p) && i < len(src); i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}

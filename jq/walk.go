package jq

import "fmt"

// Walk traverses n depth-first, calling fn before descending into each node.
// If fn returns false for a node, its children are not visited.
func Walk(n Node, fn func(Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	switch v := n.(type) {
	case *Comment:
		// leaf

	case *File:
		if v.Module != nil {
			Walk(v.Module, fn)
		}
		for _, imp := range v.Imports {
			Walk(imp, fn)
		}
		for _, fd := range v.FuncDefs {
			Walk(fd, fn)
		}
		if v.Query != nil {
			Walk(v.Query, fn)
		}

	case *ModuleStmt:
		Walk(v.Meta, fn)

	case *ImportStmt:
		if v.Meta != nil {
			Walk(v.Meta, fn)
		}

	case *FuncDef:
		for _, c := range v.LeadingComments {
			Walk(c, fn)
		}
		Walk(v.Body, fn)

	case *LocalFuncDef:
		for _, c := range v.LeadingComments {
			Walk(c, fn)
		}
		Walk(v.Body, fn)
		Walk(v.Rest, fn)

	case *CommentedExpr:
		for _, c := range v.LeadingComments {
			Walk(c, fn)
		}
		Walk(v.Expr, fn)
		if v.TrailingComment != nil {
			Walk(v.TrailingComment, fn)
		}

	case *Pipe:
		Walk(v.Left, fn)
		Walk(v.Right, fn)

	case *Comma:
		Walk(v.Left, fn)
		Walk(v.Right, fn)

	case *AsExpr:
		Walk(v.Expr, fn)
		Walk(v.Pattern, fn)
		Walk(v.Body, fn)

	case *LabelExpr:
		Walk(v.Body, fn)

	case *BinOp:
		Walk(v.Left, fn)
		Walk(v.Right, fn)

	case *IfExpr:
		Walk(v.Cond, fn)
		Walk(v.Then, fn)
		for _, ei := range v.ElseIfs {
			Walk(ei.Cond, fn)
			Walk(ei.Then, fn)
		}
		if v.Else != nil {
			Walk(v.Else, fn)
		}

	case *ReduceExpr:
		Walk(v.Expr, fn)
		Walk(v.Pattern, fn)
		Walk(v.Init, fn)
		Walk(v.Update, fn)

	case *ForeachExpr:
		Walk(v.Expr, fn)
		Walk(v.Pattern, fn)
		Walk(v.Init, fn)
		Walk(v.Update, fn)
		if v.Extract != nil {
			Walk(v.Extract, fn)
		}

	case *TryExpr:
		Walk(v.Body, fn)
		if v.Handler != nil {
			Walk(v.Handler, fn)
		}

	case *Index:
		if v.Expr != nil {
			Walk(v.Expr, fn)
		}
		if v.Key != nil {
			Walk(v.Key, fn)
		}

	case *Slice:
		if v.Expr != nil {
			Walk(v.Expr, fn)
		}
		if v.Start != nil {
			Walk(v.Start, fn)
		}
		if v.End != nil {
			Walk(v.End, fn)
		}

	case *FormatExpr:
		if v.Str != nil {
			Walk(v.Str, fn)
		}

	case *Call:
		for _, arg := range v.Args {
			Walk(arg, fn)
		}

	case *Array:
		if v.Elem != nil {
			Walk(v.Elem, fn)
		}

	case *Object:
		for _, f := range v.Fields {
			for _, c := range f.LeadingComments {
				Walk(c, fn)
			}
			Walk(f.Key, fn)
			if f.Value != nil {
				Walk(f.Value, fn)
			}
		}

	case *Paren:
		Walk(v.Expr, fn)

	case *Optional:
		Walk(v.Expr, fn)

	case *ArrayPattern:
		for _, elem := range v.Elems {
			Walk(elem, fn)
		}

	case *ObjectPattern:
		// leaf — ObjPatField has no Node children

	// ── leaf types (no children) ──────────────────────────────────────────────
	//
	// Every concrete Node type in ast.go must appear either in the composite
	// cases above or in this leaf case.  The compiler enforces that new types
	// implement Node (via the unexported jqNode() method on the sealed
	// interface), but does NOT enforce that Walk handles them — the default
	// panic below catches any missed additions at test time.
	case *Identity, *Recurse, *Field, *Ident, *Var,
		*NumberLit, *StrLit, *NullLit, *BoolLit,
		*BreakExpr, *LocExpr:
		// leaves — Comment is already handled as a leaf at the top of the switch
		// leaf — fn was already called above; nothing to descend into

	default:
		// A new concrete Node type was added to ast.go without a Walk case.
		// Add it to either the composite cases above or the leaf case here.
		panic(fmt.Sprintf("jq/walk: unhandled Node type %T — add a case to Walk", n))
	}
}


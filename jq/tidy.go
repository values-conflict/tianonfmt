package jq

import "strings"

// TidyFile applies idiomatic in-place rewrites to the parsed AST.
//
// Object field shorthand:
//
//	{foo: .foo}               → {foo}
//	{bar: $bar}               → {$bar}
//	{"foo-bar": .["foo-bar"]} → {"foo-bar"}
//	{"foo-bar": ."foo-bar"}   → {"foo-bar"}
//
// Fields with attached trailing comments are skipped to avoid losing them.
func TidyFile(f *File) {
	Walk(f, func(n Node) bool {
		obj, ok := n.(*Object)
		if !ok {
			return true
		}
		for _, field := range obj.Fields {
			if field.Value == nil {
				continue
			}
			if _, isCommented := field.Value.(*CommentedExpr); isCommented {
				continue
			}
			switch key := field.Key.(type) {
			case *Ident:
				switch v := field.Value.(type) {
				case *Field:
					// {foo: .foo} → {foo}
					if v.Name == "."+key.Name {
						field.Value = nil
					}
				case *Var:
					// {foo: $foo} → {$foo}
					if v.Name == "$"+key.Name {
						field.Key = v
						field.Value = nil
					}
				}
			case *StrLit:
				// {"foo-bar": .["foo-bar"]} → {"foo-bar"}
				// Matches Index with either nil or explicit Identity Expr.
				if v, ok := field.Value.(*Index); ok && !v.Optional {
					if strKey, ok := v.Key.(*StrLit); ok && strKey.Raw == key.Raw {
						_, isIdentity := v.Expr.(*Identity)
						if v.Expr == nil || isIdentity {
							field.Value = nil
						}
					}
				}
			}
		}
		return true
	})
}

// isIdentifier reports whether s is a valid bare jq field name (usable as .foo
// without bracket notation or quoting).
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		switch {
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// unquoteStrLit removes surrounding double-quotes from a raw StrLit value and
// returns the unescaped content, or "" if the value is not a simple quoted
// string (e.g. has backslash escapes or interpolation).
func unquoteStrLit(raw string) string {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return ""
	}
	inner := raw[1 : len(raw)-1]
	if strings.ContainsAny(inner, `\`) {
		return ""
	}
	return inner
}

package jq

// MarshalAST implementations for every Node type.
//
// JSON key naming: single-word keys are lowercase; multi-word keys are
// camelCase; no underscores.  See docs/json.md and docs/universal.md.
//
// When adding a new Node type to ast.go, the compiler enforces that you add
// a MarshalAST() implementation here — a missing method is a build error.

// ── helpers ───────────────────────────────────────────────────────────────────

func marshalNode(n Node) any {
	if n == nil {
		return nil
	}
	return n.MarshalAST()
}

func marshalNodes[T Node](ns []T) []any {
	out := make([]any, len(ns))
	for i, n := range ns {
		out[i] = n.MarshalAST()
	}
	return out
}

func marshalComments(cs []*Comment) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Text
	}
	return out
}

// ── node implementations ──────────────────────────────────────────────────────

func (c *Comment) MarshalAST() map[string]any {
	return map[string]any{"type": "comment", "text": c.Text}
}

func (f *File) MarshalAST() map[string]any {
	m := map[string]any{"type": "jq"}
	if f.Module != nil {
		m["module"] = marshalNode(f.Module)
	}
	if len(f.Imports) > 0 {
		m["imports"] = marshalNodes(f.Imports)
	}
	if len(f.FuncDefs) > 0 {
		m["funcDefs"] = marshalNodes(f.FuncDefs)
	}
	if f.Query != nil {
		m["query"] = marshalNode(f.Query)
	}
	return m
}

func (m *ModuleStmt) MarshalAST() map[string]any {
	return map[string]any{"type": "module", "meta": marshalNode(m.Meta)}
}

func (i *ImportStmt) MarshalAST() map[string]any {
	m := map[string]any{
		"type":    "import",
		"include": i.Include,
		"path":    i.Path,
	}
	if i.Binding != "" {
		m["binding"] = i.Binding
	}
	if i.Meta != nil {
		m["meta"] = marshalNode(i.Meta)
	}
	return m
}

func (f *FuncDef) MarshalAST() map[string]any {
	m := map[string]any{
		"type": "funcDef",
		"name": f.Name,
		"body": marshalNode(f.Body),
	}
	if len(f.Params) > 0 {
		m["params"] = f.Params
	}
	if len(f.LeadingComments) > 0 {
		m["leadingComments"] = marshalComments(f.LeadingComments)
	}
	return m
}

func (f *LocalFuncDef) MarshalAST() map[string]any {
	m := map[string]any{
		"type": "localFuncDef",
		"name": f.Name,
		"body": marshalNode(f.Body),
		"rest": marshalNode(f.Rest),
	}
	if len(f.Params) > 0 {
		m["params"] = f.Params
	}
	return m
}

func (c *CommentedExpr) MarshalAST() map[string]any {
	m := map[string]any{
		"type": "commented",
		"expr": marshalNode(c.Expr),
	}
	if len(c.LeadingComments) > 0 {
		m["leadingComments"] = marshalComments(c.LeadingComments)
	}
	if c.TrailingComment != nil {
		m["trailingComment"] = c.TrailingComment.Text
	}
	return m
}

func (p *Pipe) MarshalAST() map[string]any {
	return map[string]any{"type": "pipe", "left": marshalNode(p.Left), "right": marshalNode(p.Right)}
}

func (c *Comma) MarshalAST() map[string]any {
	return map[string]any{"type": "comma", "left": marshalNode(c.Left), "right": marshalNode(c.Right)}
}

func (a *AsExpr) MarshalAST() map[string]any {
	return map[string]any{
		"type":    "as",
		"expr":    marshalNode(a.Expr),
		"pattern": marshalNode(a.Pattern),
		"body":    marshalNode(a.Body),
	}
}

func (l *LabelExpr) MarshalAST() map[string]any {
	return map[string]any{"type": "label", "binding": l.Binding, "body": marshalNode(l.Body)}
}

func (b *BinOp) MarshalAST() map[string]any {
	return map[string]any{"type": "binOp", "op": b.Op, "left": marshalNode(b.Left), "right": marshalNode(b.Right)}
}

func (i *IfExpr) MarshalAST() map[string]any {
	m := map[string]any{
		"type": "if",
		"cond": marshalNode(i.Cond),
		"then": marshalNode(i.Then),
	}
	if len(i.ElseIfs) > 0 {
		eifs := make([]any, len(i.ElseIfs))
		for j, ei := range i.ElseIfs {
			eifs[j] = map[string]any{"cond": marshalNode(ei.Cond), "then": marshalNode(ei.Then)}
		}
		m["elseIfs"] = eifs
	}
	if i.Else != nil {
		m["else"] = marshalNode(i.Else)
	}
	return m
}

func (r *ReduceExpr) MarshalAST() map[string]any {
	return map[string]any{
		"type":    "reduce",
		"expr":    marshalNode(r.Expr),
		"pattern": marshalNode(r.Pattern),
		"init":    marshalNode(r.Init),
		"update":  marshalNode(r.Update),
	}
}

func (f *ForeachExpr) MarshalAST() map[string]any {
	m := map[string]any{
		"type":    "foreach",
		"expr":    marshalNode(f.Expr),
		"pattern": marshalNode(f.Pattern),
		"init":    marshalNode(f.Init),
		"update":  marshalNode(f.Update),
	}
	if f.Extract != nil {
		m["extract"] = marshalNode(f.Extract)
	}
	return m
}

func (t *TryExpr) MarshalAST() map[string]any {
	m := map[string]any{"type": "try", "body": marshalNode(t.Body)}
	if t.Handler != nil {
		m["handler"] = marshalNode(t.Handler)
	}
	return m
}

func (i *Identity) MarshalAST() map[string]any {
	return map[string]any{"type": "identity"}
}

func (r *Recurse) MarshalAST() map[string]any {
	return map[string]any{"type": "recurse"}
}

func (f *Field) MarshalAST() map[string]any {
	return map[string]any{"type": "field", "name": f.Name}
}

func (i *Index) MarshalAST() map[string]any {
	m := map[string]any{"type": "index"}
	if i.Expr != nil {
		m["expr"] = marshalNode(i.Expr)
	}
	if i.Key != nil {
		m["key"] = marshalNode(i.Key)
	}
	if i.Optional {
		m["optional"] = true
	}
	return m
}

func (s *Slice) MarshalAST() map[string]any {
	m := map[string]any{"type": "slice"}
	if s.Expr != nil {
		m["expr"] = marshalNode(s.Expr)
	}
	if s.Start != nil {
		m["start"] = marshalNode(s.Start)
	}
	if s.End != nil {
		m["end"] = marshalNode(s.End)
	}
	if s.Optional {
		m["optional"] = true
	}
	return m
}

func (i *Ident) MarshalAST() map[string]any {
	return map[string]any{"type": "ident", "name": i.Name}
}

func (v *Var) MarshalAST() map[string]any {
	return map[string]any{"type": "var", "name": v.Name}
}

func (f *FormatExpr) MarshalAST() map[string]any {
	m := map[string]any{"type": "format", "name": f.Name}
	if f.Str != nil {
		m["str"] = marshalNode(f.Str)
	}
	return m
}

func (c *Call) MarshalAST() map[string]any {
	m := map[string]any{"type": "call", "name": c.Name}
	if len(c.Args) > 0 {
		m["args"] = marshalNodes(c.Args)
	}
	return m
}

func (n *NumberLit) MarshalAST() map[string]any {
	return map[string]any{"type": "number", "raw": n.Raw}
}

func (s *StrLit) MarshalAST() map[string]any {
	return map[string]any{"type": "string", "raw": s.Raw}
}

func (n *NullLit) MarshalAST() map[string]any {
	return map[string]any{"type": "null"}
}

func (b *BoolLit) MarshalAST() map[string]any {
	return map[string]any{"type": "bool", "value": b.Val}
}

func (a *Array) MarshalAST() map[string]any {
	m := map[string]any{"type": "array"}
	if a.Elem != nil {
		m["elem"] = marshalNode(a.Elem)
	}
	return m
}

func (o *Object) MarshalAST() map[string]any {
	fields := make([]any, len(o.Fields))
	for i, f := range o.Fields {
		fm := map[string]any{"key": marshalNode(f.Key)}
		if f.Value != nil {
			fm["value"] = marshalNode(f.Value)
		}
		if f.KeyOptional {
			fm["keyOptional"] = true
		}
		if len(f.LeadingComments) > 0 {
			fm["leadingComments"] = marshalComments(f.LeadingComments)
		}
		fields[i] = fm
	}
	return map[string]any{"type": "object", "fields": fields}
}

func (p *Paren) MarshalAST() map[string]any {
	return map[string]any{"type": "paren", "expr": marshalNode(p.Expr)}
}

func (o *Optional) MarshalAST() map[string]any {
	return map[string]any{"type": "optional", "expr": marshalNode(o.Expr)}
}

func (b *BreakExpr) MarshalAST() map[string]any {
	return map[string]any{"type": "break", "binding": b.Binding}
}

func (l *LocExpr) MarshalAST() map[string]any {
	return map[string]any{"type": "loc"}
}

func (a *ArrayPattern) MarshalAST() map[string]any {
	return map[string]any{"type": "arrayPattern", "elems": marshalNodes(a.Elems)}
}

func (o *ObjectPattern) MarshalAST() map[string]any {
	fields := make([]any, len(o.Fields))
	for i, f := range o.Fields {
		fields[i] = map[string]any{"key": f.Key, "binding": f.Binding}
	}
	return map[string]any{"type": "objectPattern", "fields": fields}
}

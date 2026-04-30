package jq

import (
	"bytes"
	"encoding/json"
)

// OrderedMap is a JSON object whose keys are emitted in insertion order.
// It implements json.Marshaler so that encoding/json preserves that order
// when the value appears anywhere in a JSON tree.
//
// TODO: reconsider whether to share/replace with the om.OrderedMap[T] in
// github.com/docker-library/meta-scripts once that package is importable
// standalone.  Key differences: that version is generic, has UnmarshalJSON,
// and uses map+keys (O(1) lookup); ours is append-only and simpler for a
// write-once-serialize use case.  Note: its MarshalJSON uses json.NewEncoder
// which appends \n after every token; the raw []byte has embedded newlines,
// though json.MarshalIndent normalizes them so the final output is unaffected.
type OrderedMap []MapEntry

// MapEntry is a single key-value pair in an OrderedMap.
type MapEntry struct {
	Key string
	Val any
}

// Insert returns a new OrderedMap with key:val inserted at position pos.
func (m OrderedMap) Insert(pos int, key string, val any) OrderedMap {
	out := make(OrderedMap, len(m)+1)
	copy(out, m[:pos])
	out[pos] = MapEntry{key, val}
	copy(out[pos+1:], m[pos:])
	return out
}

// MarshalJSON emits a JSON object with keys in slice order.
func (m OrderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, e := range m {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(e.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		v, err := json.Marshal(e.Val)
		if err != nil {
			return nil, err
		}
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// MarshalAST implementations for every Node type.
//
// JSON key naming: single-word keys are lowercase; multi-word keys are
// camelCase; no underscores.  See docs/json.md and docs/universal.md.
// Keys are emitted in semantic order: "type" first, then fields in the
// order a reader would naturally expect them.
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

func (c *Comment) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "comment"}, {"text", c.Text}}
}

func (f *File) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "jq"}}
	if f.Module != nil {
		m = append(m, MapEntry{"module", marshalNode(f.Module)})
	}
	if len(f.Imports) > 0 {
		m = append(m, MapEntry{"imports", marshalNodes(f.Imports)})
	}
	if len(f.FuncDefs) > 0 {
		m = append(m, MapEntry{"funcDefs", marshalNodes(f.FuncDefs)})
	}
	if f.Query != nil {
		m = append(m, MapEntry{"query", marshalNode(f.Query)})
	}
	return m
}

func (ms *ModuleStmt) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "module"}, {"meta", marshalNode(ms.Meta)}}
}

func (i *ImportStmt) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "import"}, {"include", i.Include}, {"path", i.Path}}
	if i.Binding != "" {
		m = append(m, MapEntry{"binding", i.Binding})
	}
	if i.Meta != nil {
		m = append(m, MapEntry{"meta", marshalNode(i.Meta)})
	}
	if len(i.LeadingComments) > 0 {
		m = append(m, MapEntry{"leadingComments", marshalComments(i.LeadingComments)})
	}
	return m
}

func (f *FuncDef) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "funcDef"}, {"name", f.Name}}
	if len(f.Params) > 0 {
		m = append(m, MapEntry{"params", f.Params})
	}
	if len(f.LeadingComments) > 0 {
		m = append(m, MapEntry{"leadingComments", marshalComments(f.LeadingComments)})
	}
	m = append(m, MapEntry{"body", marshalNode(f.Body)})
	return m
}

func (f *LocalFuncDef) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "localFuncDef"}, {"name", f.Name}}
	if len(f.Params) > 0 {
		m = append(m, MapEntry{"params", f.Params})
	}
	m = append(m, MapEntry{"body", marshalNode(f.Body)})
	m = append(m, MapEntry{"rest", marshalNode(f.Rest)})
	return m
}

func (c *CommentedExpr) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "commented"}}
	if len(c.LeadingComments) > 0 {
		m = append(m, MapEntry{"leadingComments", marshalComments(c.LeadingComments)})
	}
	m = append(m, MapEntry{"expr", marshalNode(c.Expr)})
	if c.TrailingComment != nil {
		m = append(m, MapEntry{"trailingComment", c.TrailingComment.Text})
	}
	return m
}

func (p *Pipe) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "pipe"}, {"left", marshalNode(p.Left)}, {"right", marshalNode(p.Right)}}
}

func (c *Comma) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "comma"}, {"left", marshalNode(c.Left)}, {"right", marshalNode(c.Right)}}
}

func (a *AsExpr) MarshalAST() OrderedMap {
	return OrderedMap{
		{"type", "as"},
		{"expr", marshalNode(a.Expr)},
		{"pattern", marshalNode(a.Pattern)},
		{"body", marshalNode(a.Body)},
	}
}

func (l *LabelExpr) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "label"}, {"binding", l.Binding}, {"body", marshalNode(l.Body)}}
}

func (b *BinOp) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "binOp"}, {"op", b.Op}, {"left", marshalNode(b.Left)}, {"right", marshalNode(b.Right)}}
}

func (i *IfExpr) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "if"}, {"cond", marshalNode(i.Cond)}, {"then", marshalNode(i.Then)}}
	if len(i.ElseIfs) > 0 {
		eifs := make([]any, len(i.ElseIfs))
		for j, ei := range i.ElseIfs {
			eifs[j] = OrderedMap{{"cond", marshalNode(ei.Cond)}, {"then", marshalNode(ei.Then)}}
		}
		m = append(m, MapEntry{"elseIfs", eifs})
	}
	if i.Else != nil {
		m = append(m, MapEntry{"else", marshalNode(i.Else)})
	}
	return m
}

func (r *ReduceExpr) MarshalAST() OrderedMap {
	return OrderedMap{
		{"type", "reduce"},
		{"expr", marshalNode(r.Expr)},
		{"pattern", marshalNode(r.Pattern)},
		{"init", marshalNode(r.Init)},
		{"update", marshalNode(r.Update)},
	}
}

func (f *ForeachExpr) MarshalAST() OrderedMap {
	m := OrderedMap{
		{"type", "foreach"},
		{"expr", marshalNode(f.Expr)},
		{"pattern", marshalNode(f.Pattern)},
		{"init", marshalNode(f.Init)},
		{"update", marshalNode(f.Update)},
	}
	if f.Extract != nil {
		m = append(m, MapEntry{"extract", marshalNode(f.Extract)})
	}
	return m
}

func (t *TryExpr) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "try"}, {"body", marshalNode(t.Body)}}
	if t.Handler != nil {
		m = append(m, MapEntry{"handler", marshalNode(t.Handler)})
	}
	return m
}

func (i *Identity) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "identity"}}
}

func (r *Recurse) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "recurse"}}
}

func (f *Field) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "field"}, {"name", f.Name}}
}

func (i *Index) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "index"}}
	if i.Expr != nil {
		m = append(m, MapEntry{"expr", marshalNode(i.Expr)})
	}
	if i.Key != nil {
		m = append(m, MapEntry{"key", marshalNode(i.Key)})
	}
	if i.DotAccess {
		m = append(m, MapEntry{"dotAccess", true})
	}
	if i.Optional {
		m = append(m, MapEntry{"optional", true})
	}
	return m
}

func (s *Slice) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "slice"}}
	if s.Expr != nil {
		m = append(m, MapEntry{"expr", marshalNode(s.Expr)})
	}
	if s.Start != nil {
		m = append(m, MapEntry{"start", marshalNode(s.Start)})
	}
	if s.End != nil {
		m = append(m, MapEntry{"end", marshalNode(s.End)})
	}
	if s.Optional {
		m = append(m, MapEntry{"optional", true})
	}
	return m
}

func (i *Ident) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "ident"}, {"name", i.Name}}
}

func (v *Var) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "var"}, {"name", v.Name}}
}

func (f *FormatExpr) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "format"}, {"name", f.Name}}
	if f.Str != nil {
		m = append(m, MapEntry{"str", marshalNode(f.Str)})
	}
	return m
}

func (c *Call) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "call"}, {"name", c.Name}}
	if len(c.Args) > 0 {
		m = append(m, MapEntry{"args", marshalNodes(c.Args)})
	}
	return m
}

func (n *NumberLit) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "number"}, {"raw", n.Raw}}
}

func (s *StrLit) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "string"}, {"raw", s.Raw}}
}

func (n *NullLit) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "null"}}
}

func (b *BoolLit) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "bool"}, {"value", b.Val}}
}

func (a *Array) MarshalAST() OrderedMap {
	m := OrderedMap{{"type", "array"}}
	if a.Elem != nil {
		m = append(m, MapEntry{"elem", marshalNode(a.Elem)})
	}
	return m
}

func (o *Object) MarshalAST() OrderedMap {
	fields := make([]any, len(o.Fields))
	for i, f := range o.Fields {
		fm := OrderedMap{{"key", marshalNode(f.Key)}}
		if f.Value != nil {
			fm = append(fm, MapEntry{"value", marshalNode(f.Value)})
		}
		if f.KeyOptional {
			fm = append(fm, MapEntry{"keyOptional", true})
		}
		if len(f.LeadingComments) > 0 {
			fm = append(fm, MapEntry{"leadingComments", marshalComments(f.LeadingComments)})
		}
		fields[i] = fm
	}
	return OrderedMap{{"type", "object"}, {"fields", fields}}
}

func (p *Paren) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "paren"}, {"expr", marshalNode(p.Expr)}}
}

func (o *Optional) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "optional"}, {"expr", marshalNode(o.Expr)}}
}

func (b *BreakExpr) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "break"}, {"binding", b.Binding}}
}

func (l *LocExpr) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "loc"}}
}

func (a *ArrayPattern) MarshalAST() OrderedMap {
	return OrderedMap{{"type", "arrayPattern"}, {"elems", marshalNodes(a.Elems)}}
}

func (o *ObjectPattern) MarshalAST() OrderedMap {
	fields := make([]any, len(o.Fields))
	for i, f := range o.Fields {
		fields[i] = OrderedMap{{"key", f.Key}, {"binding", f.Binding}}
	}
	return OrderedMap{{"type", "objectPattern"}, {"fields", fields}}
}

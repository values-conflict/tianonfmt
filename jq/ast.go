package jq

// Node is implemented by every AST node.
type Node interface {
	jqNode()
	nodePos() Pos
}

func pos(n Node) Pos { return n.nodePos() }

// Comment is a # comment line.  The Text includes the # but not the trailing
// newline.  Comments are attached to the next non-comment node as LeadingComments,
// or to the enclosing node as TrailingComments when they appear after the last
// token on the same line.
type Comment struct {
	At   Pos
	Text string
}

func (c *Comment) jqNode()      {}
func (c *Comment) nodePos() Pos { return c.At }

// File is the top-level node produced by Parse.
type File struct {
	At       Pos
	Module   *ModuleStmt   // optional
	Imports  []*ImportStmt // may be empty
	FuncDefs []*FuncDef    // top-level defs before the main query
	Query    Node          // the main query expression (may be nil for def-only files)
}

func (f *File) jqNode()      {}
func (f *File) nodePos() Pos { return f.At }

// ModuleStmt: module {...};
type ModuleStmt struct {
	At   Pos
	Meta Node
}

func (m *ModuleStmt) jqNode()      {}
func (m *ModuleStmt) nodePos() Pos { return m.At }

// ImportStmt: import "path" as $name; OR include "path";
type ImportStmt struct {
	At      Pos
	Include bool   // true for "include", false for "import"
	Path    string // the string literal text (raw, with quotes)
	Binding string // "$name" for import; empty for include
	Meta    Node   // optional trailing object metadata
}

func (i *ImportStmt) jqNode()      {}
func (i *ImportStmt) nodePos() Pos { return i.At }

// FuncDef: def name(params): body;
type FuncDef struct {
	At              Pos
	Name            string
	Params          []string // "$var" or "funcname"
	Body            Node
	LeadingComments []*Comment
}

func (f *FuncDef) jqNode()      {}
func (f *FuncDef) nodePos() Pos { return f.At }

// Pipe: left | right
type Pipe struct {
	At    Pos
	Left  Node
	Right Node
}

func (p *Pipe) jqNode()      {}
func (p *Pipe) nodePos() Pos { return p.At }

// Comma: left , right (generator)
type Comma struct {
	At    Pos
	Left  Node
	Right Node
}

func (c *Comma) jqNode()      {}
func (c *Comma) nodePos() Pos { return c.At }

// AsExpr: expr as $pattern | body
type AsExpr struct {
	At      Pos
	Expr    Node
	Pattern Node // Var or DestructurePattern
	Body    Node
}

func (a *AsExpr) jqNode()      {}
func (a *AsExpr) nodePos() Pos { return a.At }

// LabelExpr: label $x | body
type LabelExpr struct {
	At      Pos
	Binding string // "$x"
	Body    Node
}

func (l *LabelExpr) jqNode()      {}
func (l *LabelExpr) nodePos() Pos { return l.At }

// BinOp: arithmetic, comparison, logical, and update operators.
type BinOp struct {
	At    Pos
	Op    string
	Left  Node
	Right Node
}

func (b *BinOp) jqNode()      {}
func (b *BinOp) nodePos() Pos { return b.At }

// IfExpr: if cond then body (elif cond then body)* (else body)? end
type IfExpr struct {
	At      Pos
	Cond    Node
	Then    Node
	ElseIfs []ElseIfClause
	Else    Node // nil if no else
}

// ElseIfClause is one elif branch.
type ElseIfClause struct {
	Cond Node
	Then Node
}

func (i *IfExpr) jqNode()      {}
func (i *IfExpr) nodePos() Pos { return i.At }

// ReduceExpr: reduce expr as $pattern (init; update)
type ReduceExpr struct {
	At      Pos
	Expr    Node
	Pattern Node
	Init    Node
	Update  Node
}

func (r *ReduceExpr) jqNode()      {}
func (r *ReduceExpr) nodePos() Pos { return r.At }

// ForeachExpr: foreach expr as $pattern (init; update) or (init; update; extract)
type ForeachExpr struct {
	At      Pos
	Expr    Node
	Pattern Node
	Init    Node
	Update  Node
	Extract Node // nil if absent
}

func (f *ForeachExpr) jqNode()      {}
func (f *ForeachExpr) nodePos() Pos { return f.At }

// TryExpr: try body (catch handler)?
type TryExpr struct {
	At      Pos
	Body    Node
	Handler Node // nil if no catch
}

func (t *TryExpr) jqNode()      {}
func (t *TryExpr) nodePos() Pos { return t.At }

// Identity: .
type Identity struct{ At Pos }

func (i *Identity) jqNode()      {}
func (i *Identity) nodePos() Pos { return i.At }

// Recurse: ..
type Recurse struct{ At Pos }

func (r *Recurse) jqNode()      {}
func (r *Recurse) nodePos() Pos { return r.At }

// Field: .name (a field access, produced by the Field token)
type Field struct {
	At   Pos
	Name string // the full text including leading dot, e.g. ".foo"
}

func (f *Field) jqNode()      {}
func (f *Field) nodePos() Pos { return f.At }

// Index: expr[key] or .[key] or expr[] or .[]
type Index struct {
	At       Pos
	Expr     Node // nil when standalone e.g. .[0]
	Key      Node // nil for iterator .[]
	Optional bool // trailing ?
}

func (i *Index) jqNode()      {}
func (i *Index) nodePos() Pos { return i.At }

// Slice: expr[start:end] — supports .[start:end], .[start:], .[:end]
type Slice struct {
	At       Pos
	Expr     Node // nil when standalone
	Start    Node // nil for [:end]
	End      Node // nil for [start:]
	Optional bool
}

func (s *Slice) jqNode()      {}
func (s *Slice) nodePos() Pos { return s.At }

// Ident: a bare name used as identity or in certain positions (e.g. object key)
type Ident struct {
	At   Pos
	Name string
}

func (i *Ident) jqNode()      {}
func (i *Ident) nodePos() Pos { return i.At }

// Var: $name
type Var struct {
	At   Pos
	Name string // includes the $ prefix
}

func (v *Var) jqNode()      {}
func (v *Var) nodePos() Pos { return v.At }

// FormatExpr: @base64 or @base64 "string"
type FormatExpr struct {
	At   Pos
	Name string // without @
	Str  Node   // nil when used as a filter; set when @fmt "string"
}

func (f *FormatExpr) jqNode()      {}
func (f *FormatExpr) nodePos() Pos { return f.At }

// Call: name or name(arg; arg; …)
type Call struct {
	At   Pos
	Name string
	Args []Node
}

func (c *Call) jqNode()      {}
func (c *Call) nodePos() Pos { return c.At }

// NumberLit: 42, 3.14
type NumberLit struct {
	At  Pos
	Raw string
}

func (n *NumberLit) jqNode()      {}
func (n *NumberLit) nodePos() Pos { return n.At }

// StrLit: a complete string literal, raw text preserved.
type StrLit struct {
	At  Pos
	Raw string // includes surrounding quotes
}

func (s *StrLit) jqNode()      {}
func (s *StrLit) nodePos() Pos { return s.At }

// NullLit: null
type NullLit struct{ At Pos }

func (n *NullLit) jqNode()      {}
func (n *NullLit) nodePos() Pos { return n.At }

// BoolLit: true | false
type BoolLit struct {
	At  Pos
	Val bool
}

func (b *BoolLit) jqNode()      {}
func (b *BoolLit) nodePos() Pos { return b.At }

// Array: [ expr ]
type Array struct {
	At   Pos
	Elem Node // nil for empty array
}

func (a *Array) jqNode()      {}
func (a *Array) nodePos() Pos { return a.At }

// Object: { field, … }
type Object struct {
	At     Pos
	Fields []*ObjectField
}

func (o *Object) jqNode()      {}
func (o *Object) nodePos() Pos { return o.At }

// ObjectField is a single key:value pair in an object literal.
// If Value is nil, this is a shorthand field ({foo} == {foo: .foo}).
type ObjectField struct {
	At          Pos
	Key         Node   // StrLit, Ident, or Paren (for computed keys)
	KeyOptional bool   // key followed by ? inside the object
	Value       Node   // nil for shorthand
}

// LocalFuncDef: def name(params): body; REST — a function definition scoped to
// the rest of an expression.  The semicolon terminates the def and REST follows
// on the next line WITHOUT a pipe.
// This is separate from FuncDef (which is a top-level definition) so the
// formatter can apply single-line layout for short bodies.
type LocalFuncDef struct {
	At              Pos
	Name            string
	Params          []string
	Body            Node
	Rest            Node // the expression that follows the def
	LeadingComments []*Comment
}

func (l *LocalFuncDef) jqNode()      {}
func (l *LocalFuncDef) nodePos() Pos { return l.At }

// Paren: (expr) — preserves explicit parentheses
type Paren struct {
	At   Pos
	Expr Node
}

func (p *Paren) jqNode()      {}
func (p *Paren) nodePos() Pos { return p.At }

// Optional: expr?
type Optional struct {
	At   Pos
	Expr Node
}

func (o *Optional) jqNode()      {}
func (o *Optional) nodePos() Pos { return o.At }

// BreakExpr: break $x
type BreakExpr struct {
	At      Pos
	Binding string // "$x"
}

func (b *BreakExpr) jqNode()      {}
func (b *BreakExpr) nodePos() Pos { return b.At }

// LocExpr: $__loc__
type LocExpr struct{ At Pos }

func (l *LocExpr) jqNode()      {}
func (l *LocExpr) nodePos() Pos { return l.At }

// ArrayPattern: [a, b] — used in destructuring "as" patterns
type ArrayPattern struct {
	At   Pos
	Elems []Node
}

func (a *ArrayPattern) jqNode()      {}
func (a *ArrayPattern) nodePos() Pos { return a.At }

// ObjectPattern: {a: $b, c} — used in destructuring "as" patterns
type ObjectPattern struct {
	At     Pos
	Fields []*ObjPatField
}

func (o *ObjectPattern) jqNode()      {}
func (o *ObjectPattern) nodePos() Pos { return o.At }

// ObjPatField: key: $binding or shorthand key (which implies $key)
type ObjPatField struct {
	Key     string
	Binding string // "$name"; empty means shorthand (binding == "$"+key)
}

package jq

// Format formats a parsed jq File back to source.
//
// Style rules (all backed by corpus samples):
//
//   - Indentation: 1 hard tab per level
//   - Pipe |: emitted at the START of the next line, not the end of the current
//     (corpus ref: corpus/debian-bin/jq/deb822.jq:23-34, dpkg-version.jq:32-52)
//   - Comma ,: emitted at the END of the line (commas in generators)
//     (corpus ref: corpus/tianon-dockerfiles/scratch/multiarch.jq:6-20)
//   - Arithmetic ops (+, -, etc.): lead the continuation line when broken
//     (corpus ref: corpus/debian-bin/jq/dpkg-version.jq:22-28)
//   - def body: indented 1 tab; closing ; at the def's own indentation
//     (corpus ref: corpus/debian-bin/jq/deb822.jq:7-39)
//   - if/then/else/end: inline when the whole expression fits <= shortThreshold;
//     multi-line otherwise.  Short else body: "else BODY end" on same line.
//     (corpus ref: dpkg-version.jq:22-29 multi, :35 inline)
//   - foreach / reduce: always multi-line
//     (corpus ref: deb822.jq:8-38)
//   - Object literals: multi-line with one field+trailing-comma per line
//     (corpus ref: multiarch.jq:5-16)
//   - Comments: re-emitted before the node they precede

import (
	"strings"
)

const shortThreshold = 60 // max chars for inline if/else forms

// FormatFile formats a complete jq file.
func FormatFile(f *File) string {
	p := &printer{}
	p.file(f)
	return p.out.String()
}

// FormatNode formats a single node.
func FormatNode(n Node) string {
	p := &printer{}
	p.node(n)
	return p.out.String()
}

// printer accumulates formatted output.
type printer struct {
	out   strings.Builder
	depth int
}

func (p *printer) tab() string { return strings.Repeat("\t", p.depth) }

func (p *printer) write(s string)   { p.out.WriteString(s) }
func (p *printer) writeln(s string) { p.out.WriteString(s); p.out.WriteByte('\n') }
func (p *printer) indent()          { p.depth++ }
func (p *printer) dedent()          { p.depth-- }

// newline writes a newline followed by the current indentation.
func (p *printer) newline() { p.out.WriteByte('\n'); p.write(p.tab()) }

// nl writes just a newline (no indentation follows).
func (p *printer) nl() { p.out.WriteByte('\n') }

// ── top-level ────────────────────────────────────────────────────────────────

func (p *printer) file(f *File) {
	if f.Module != nil {
		p.moduleStmt(f.Module)
		p.nl()
		p.nl()
	}
	for _, imp := range f.Imports {
		p.importStmt(imp)
		p.nl()
	}
	if len(f.Imports) > 0 && (len(f.FuncDefs) > 0 || f.Query != nil) {
		p.nl()
	}
	for _, fd := range f.FuncDefs {
		p.comments(fd.LeadingComments)
		p.funcDef(fd)
		p.nl()
		p.nl()
	}
	if f.Query != nil {
		p.node(f.Query)
		p.nl()
	}
}

func (p *printer) moduleStmt(m *ModuleStmt) {
	p.write("module ")
	p.node(m.Meta)
	p.write(";")
}

func (p *printer) importStmt(i *ImportStmt) {
	if i.Include {
		p.write("include ")
	} else {
		p.write("import ")
	}
	p.write(i.Path)
	if i.Binding != "" {
		p.write(" as ")
		p.write(i.Binding)
	}
	if i.Meta != nil {
		p.write(" ")
		p.node(i.Meta)
	}
	p.write(";")
}

func (p *printer) comments(cs []*Comment) {
	for _, c := range cs {
		p.write(p.tab())
		p.writeln(c.Text)
	}
}

// localFuncDef formats a local function definition that scopes to REST.
// Short bodies: "def name: body;" on one line followed by REST on the next.
// Long bodies: multi-line form.
// Style ref: corpus/debian-bin/jq/deb822.jq:21-22
//   def _trimstart: until(startswith(" ") or startswith("\t") | not; .[1:]);
//   def _trimend:   until(endswith(" ") or endswith("\t") | not; .[:-1]);
func (p *printer) localFuncDef(v *LocalFuncDef) {
	// Format the signature prefix.
	var sig strings.Builder
	sig.WriteString("def ")
	sig.WriteString(v.Name)
	if len(v.Params) > 0 {
		sig.WriteString("(")
		for i, param := range v.Params {
			if i > 0 {
				sig.WriteString("; ")
			}
			sig.WriteString(param)
		}
		sig.WriteString(")")
	}
	sig.WriteString(": ")

	bodyInline := shortInline(v.Body)
	// Local defs use a higher threshold (100) since the corpus shows them on
	// single lines even when relatively long.
	if len(sig.String())+len(bodyInline) <= 100 && !strings.Contains(bodyInline, "\n") {
		// Single-line local def.
		p.write(sig.String())
		p.write(bodyInline)
		p.write(";")
	} else {
		// Multi-line: body indented, ; on own line at same level.
		p.write(strings.TrimRight(sig.String(), " "))
		p.nl()
		p.indent()
		p.write(p.tab())
		p.node(v.Body)
		p.nl()
		p.dedent()
		p.write(p.tab())
		p.write(";")
	}
	p.nl()
	p.write(p.tab())
	p.node(v.Rest)
}

// funcDef formats: def name(params):\n\tbody\n;
// Style ref: corpus/debian-bin/jq/deb822.jq lines 7-39
func (p *printer) funcDef(fd *FuncDef) {
	p.write("def ")
	p.write(fd.Name)
	if len(fd.Params) > 0 {
		p.write("(")
		for i, param := range fd.Params {
			if i > 0 {
				p.write("; ")
			}
			p.write(param)
		}
		p.write(")")
	}
	p.write(":")
	p.nl()
	p.indent()
	p.write(p.tab())
	p.node(fd.Body)
	p.nl()
	p.dedent()
	p.write(p.tab())
	p.write(";")
}

// ── node dispatch ────────────────────────────────────────────────────────────

func (p *printer) node(n Node) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *FuncDef:
		p.funcDef(v)
	case *LocalFuncDef:
		p.localFuncDef(v)
	case *Pipe:
		p.pipeChain(v)
	case *Comma:
		p.commaExpr(v)
	case *AsExpr:
		p.asExpr(v)
	case *LabelExpr:
		p.write("label ")
		p.write(v.Binding)
		p.write(" |")
		p.newline()
		p.node(v.Body)
	case *BinOp:
		p.binOp(v)
	case *IfExpr:
		p.ifExpr(v)
	case *ReduceExpr:
		p.reduceExpr(v)
	case *ForeachExpr:
		p.foreachExpr(v)
	case *TryExpr:
		p.tryExpr(v)
	case *Identity:
		p.write(".")
	case *Recurse:
		p.write("..")
	case *Field:
		p.write(v.Name)
	case *Index:
		p.indexExpr(v)
	case *Slice:
		p.sliceExpr(v)
	case *Var:
		p.write(v.Name)
	case *LocExpr:
		p.write("$__loc__")
	case *FormatExpr:
		p.write("@")
		p.write(v.Name)
		if v.Str != nil {
			p.write(" ")
			p.node(v.Str)
		}
	case *Call:
		p.callExpr(v)
	case *NumberLit:
		p.write(v.Raw)
	case *StrLit:
		p.write(v.Raw)
	case *NullLit:
		p.write("null")
	case *BoolLit:
		if v.Val {
			p.write("true")
		} else {
			p.write("false")
		}
	case *Array:
		p.arrayExpr(v)
	case *Object:
		p.objectExpr(v)
	case *Paren:
		inner := shortInline(v.Expr)
		if len(inner) <= shortThreshold && !strings.Contains(inner, "\n") {
			p.write("(")
			p.write(inner)
			p.write(")")
		} else {
			p.write("(")
			p.indent()
			p.newline()
			p.node(v.Expr)
			p.dedent()
			p.newline()
			p.write(")")
		}
	case *Optional:
		p.node(v.Expr)
		p.write("?")
	case *BreakExpr:
		p.write("break ")
		p.write(v.Binding)
	case *Ident:
		p.write(v.Name)
	case *ArrayPattern:
		p.write("[")
		for i, e := range v.Elems {
			if i > 0 {
				p.write(", ")
			}
			p.node(e)
		}
		p.write("]")
	case *ObjectPattern:
		p.write("{")
		for i, f := range v.Fields {
			if i > 0 {
				p.write(", ")
			}
			p.write(f.Key)
			if f.Binding != "" {
				p.write(": ")
				p.write(f.Binding)
			}
		}
		p.write("}")
	}
}

// ── expression formatters ────────────────────────────────────────────────────

// pipeChain formats a | chain.
// Style ref: corpus/debian-bin/jq/deb822.jq:23-34 — | at the START of each
// continuation line, at the same indentation as the expressions.
//
//	EXPR1
//	| EXPR2
//	| EXPR3
//
// Short pipes (e.g. "$line | _trimstart") stay inline.
func (p *printer) pipeChain(v *Pipe) {
	inline := shortInline(v)
	if len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}
	parts := flattenPipe(v)
	for i, part := range parts {
		if i > 0 {
			p.newline()
			p.write("| ")
		}
		p.node(part)
	}
}

func flattenPipe(v *Pipe) []Node {
	var parts []Node
	var walk func(n Node)
	walk = func(n Node) {
		if pp, ok := n.(*Pipe); ok {
			walk(pp.Left)
			parts = append(parts, pp.Right)
		} else {
			parts = append(parts, n)
		}
	}
	walk(v.Left)
	parts = append(parts, v.Right)
	return parts
}

// commaExpr formats a , generator chain.
// Style ref: corpus/tianon-dockerfiles/scratch/multiarch.jq:6-20 — comma at
// END of line, each element on its own line when multi-line.
func (p *printer) commaExpr(v *Comma) {
	inline := shortInline(v)
	if len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}
	parts := flattenComma(v)
	for i, part := range parts {
		if i > 0 {
			p.write(",")
			p.newline()
		}
		p.node(part)
	}
}

func flattenComma(v *Comma) []Node {
	var parts []Node
	var walk func(n Node)
	walk = func(n Node) {
		if c, ok := n.(*Comma); ok {
			walk(c.Left)
			parts = append(parts, c.Right)
		} else {
			parts = append(parts, n)
		}
	}
	walk(v.Left)
	parts = append(parts, v.Right)
	return parts
}

// asExpr formats: expr as $pat\n| body
// The | that separates the pattern binding from its scope belongs at the START
// of the next line, matching the pipe-at-start-of-line convention.
// Style ref: corpus/debian-bin/jq/deb822.jq:24
//   ($line | _trimstart) as $ltrim
//   | ($ltrim | _trimend) as $trim
func (p *printer) asExpr(v *AsExpr) {
	p.node(v.Expr)
	p.write(" as ")
	p.node(v.Pattern)
	p.newline()
	p.write("| ")
	p.node(v.Body)
}

// binOp formats a binary expression.
// For chained arithmetic/logical ops that exceed shortThreshold, each operand
// starts on a new line with the operator leading.
// Style ref: dpkg-version.jq:22-28 "+ .upstream", "+ if .revision..."
func (p *printer) binOp(v *BinOp) {
	if v.Op == "neg" {
		p.write("-")
		p.node(v.Left)
		return
	}
	// Field chaining: left.field or left.field (Right is a Field node)
	if v.Op == "" {
		p.node(v.Left)
		p.node(v.Right) // Field already has its leading dot
		return
	}
	// Pipe embedded in suffix (e.g. @fmt pipe) stays inline
	if v.Op == "|" {
		p.node(v.Left)
		p.write(" | ")
		p.node(v.Right)
		return
	}

	// For all other binary operators: try inline first; break if too long.
	inline := shortInline(v)
	if len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}

	// Multi-line: each operand on its own line, operator leads subsequent lines.
	// Flatten the left-spine so chains like a + b + c all break uniformly.
	type opPart struct {
		op   string
		node Node
	}
	var parts []opPart
	var flatten func(n Node)
	flatten = func(n Node) {
		if b, ok := n.(*BinOp); ok && b.Op == v.Op {
			flatten(b.Left)
			parts = append(parts, opPart{op: b.Op, node: b.Right})
		} else {
			parts = append(parts, opPart{node: n})
		}
	}
	flatten(v)

	for i, part := range parts {
		if i > 0 {
			p.newline()
			p.write(part.op)
			p.write(" ")
		}
		p.node(part.node)
	}
}

// ifExpr formats if/then/else/end.
// Style refs:
//   - Inline:     dpkg-version.jq:35 "if index(":") then . else "0:" + . end"
//   - Multi-line: dpkg-version.jq:22-29
//   - Short else: "else "" end" on same line (dpkg-version.jq:24,28)
func (p *printer) ifExpr(v *IfExpr) {
	inline := shortInline(v)
	if len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}

	p.write("if ")
	p.node(v.Cond)
	p.write(" then")
	p.indent()
	p.newline()
	p.node(v.Then)
	p.dedent()

	for _, ei := range v.ElseIfs {
		p.newline()
		p.write("elif ")
		p.node(ei.Cond)
		p.write(" then")
		p.indent()
		p.newline()
		p.node(ei.Then)
		p.dedent()
	}

	if v.Else != nil {
		p.newline()
		elseInline := shortInline(v.Else)
		if len(elseInline) <= 30 && !strings.Contains(elseInline, "\n") {
			p.write("else ")
			p.write(elseInline)
			p.write(" end")
		} else {
			p.write("else")
			p.indent()
			p.newline()
			p.node(v.Else)
			p.dedent()
			p.newline()
			p.write("end")
		}
	} else {
		p.newline()
		p.write("end")
	}
}

// reduceExpr formats: reduce expr as $pat (\n\tinit;\n\tupdate\n)
// Style ref: corpus/tianon-dockerfiles/scratch/multiarch.jq:67-68
func (p *printer) reduceExpr(v *ReduceExpr) {
	p.write("reduce ")
	p.node(v.Expr)
	p.write(" as ")
	p.node(v.Pattern)
	p.write(" (")
	p.indent()
	p.newline()
	p.node(v.Init)
	p.write(";")
	p.newline()
	p.node(v.Update)
	p.dedent()
	p.newline()
	p.write(")")
}

// foreachExpr formats: foreach expr as $pat (\n\tinit;\n\tupdate\n)
// or with extract: foreach ... (\n\tinit;\n\tupdate;\n\textract\n)
// Style ref: corpus/debian-bin/jq/deb822.jq:8-38
func (p *printer) foreachExpr(v *ForeachExpr) {
	p.write("foreach ")
	p.node(v.Expr)
	p.write(" as ")
	p.node(v.Pattern)
	p.write(" (")
	p.indent()
	p.newline()
	p.node(v.Init)
	p.write(";")
	p.newline()
	p.node(v.Update)
	if v.Extract != nil {
		extractInline := shortInline(v.Extract)
		if len(extractInline) <= 50 && !strings.Contains(extractInline, "\n") {
			// Short extract: "; extract)" on the close line
			// Style ref: deb822.jq:60 "; if .out then .out else empty end)"
			p.dedent()
			p.newline()
			p.write("; ")
			p.write(extractInline)
		} else {
			p.write(";")
			p.newline()
			p.node(v.Extract)
			p.dedent()
			p.newline()
		}
	} else {
		p.dedent()
		p.newline()
	}
	p.write(")")
}

// tryExpr formats: try body (catch handler)?
// Style ref: dpkg-version.jq:38 "try tonumber // (...)"
func (p *printer) tryExpr(v *TryExpr) {
	p.write("try ")
	p.node(v.Body)
	if v.Handler != nil {
		p.write(" catch ")
		p.node(v.Handler)
	}
}

func (p *printer) indexExpr(v *Index) {
	if v.Expr != nil {
		p.node(v.Expr)
	}
	if v.Key == nil {
		p.write("[]")
	} else {
		p.write("[")
		p.node(v.Key)
		p.write("]")
	}
	if v.Optional {
		p.write("?")
	}
}

func (p *printer) sliceExpr(v *Slice) {
	if v.Expr != nil {
		p.node(v.Expr)
	}
	p.write("[")
	if v.Start != nil {
		p.node(v.Start)
	}
	p.write(":")
	if v.End != nil {
		p.node(v.End)
	}
	p.write("]")
	if v.Optional {
		p.write("?")
	}
}

func (p *printer) callExpr(v *Call) {
	p.write(v.Name)
	if len(v.Args) > 0 {
		p.write("(")
		for i, arg := range v.Args {
			if i > 0 {
				p.write("; ")
			}
			p.node(arg)
		}
		p.write(")")
	}
}

// arrayExpr formats an array literal.
// Style ref: corpus/tianon-dockerfiles/scratch/multiarch.jq — multi-line when
// the element expression is a pipe chain or comma generator.
func (p *printer) arrayExpr(v *Array) {
	if v.Elem == nil {
		p.write("[]")
		return
	}
	inline := "[" + shortInline(v.Elem) + "]"
	if len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}
	p.write("[")
	p.indent()
	p.newline()
	p.node(v.Elem)
	p.dedent()
	p.newline()
	p.write("]")
}

// objectExpr formats an object literal.
// Style ref: multiarch.jq:5-16 — each field on its own line, trailing comma.
func (p *printer) objectExpr(v *Object) {
	if len(v.Fields) == 0 {
		p.write("{}")
		return
	}
	inline := shortInlineObject(v)
	if len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}
	p.write("{")
	p.indent()
	for _, f := range v.Fields {
		p.newline()
		p.objectField(f)
		p.write(",")
	}
	p.dedent()
	p.newline()
	p.write("}")
}

func (p *printer) objectField(f *ObjectField) {
	// Variable shorthand: {$foo} — key is Var, no value
	if v, ok := f.Key.(*Var); ok && f.Value == nil {
		p.write(v.Name)
		return
	}
	p.node(f.Key)
	if f.KeyOptional {
		p.write("?")
	}
	if f.Value != nil {
		p.write(": ")
		p.node(f.Value)
	}
}

// ── "short inline" helpers ───────────────────────────────────────────────────
//
// shortInline renders a node as a single-line string for length-check purposes.
// The actual output is generated by node(); this is only for the threshold test.

func shortInline(n Node) string {
	p := &printer{}
	p.nodeInline(n)
	return p.out.String()
}

// shortInlineObject returns the single-line form of an object literal.
// Non-empty inline objects use spaces inside the braces:
//   { key: val, other: val }
// per corpus/debian-bin/jq/deb822.jq:17 "{ accum: {} }" and :19 "{ out: .accum, accum: {} }"
func shortInlineObject(v *Object) string {
	if len(v.Fields) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteString("{ ")
	for i, f := range v.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		ip := &printer{}
		ip.objectField(f)
		b.WriteString(ip.out.String())
	}
	b.WriteString(" }")
	return b.String()
}

// nodeInline renders n without multi-line expansion.
func (p *printer) nodeInline(n Node) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *Pipe:
		parts := flattenPipe(v)
		for i, part := range parts {
			if i > 0 {
				p.write(" | ")
			}
			p.nodeInline(part)
		}
	case *Comma:
		parts := flattenComma(v)
		for i, part := range parts {
			if i > 0 {
				p.write(", ")
			}
			p.nodeInline(part)
		}
	case *AsExpr:
		p.nodeInline(v.Expr)
		p.write(" as ")
		p.nodeInline(v.Pattern)
		p.write(" | ")
		p.nodeInline(v.Body)
	case *BinOp:
		if v.Op == "neg" {
			p.write("-")
			p.nodeInline(v.Left)
			return
		}
		if v.Op == "" {
			p.nodeInline(v.Left)
			p.nodeInline(v.Right)
			return
		}
		p.nodeInline(v.Left)
		p.write(" ")
		p.write(v.Op)
		p.write(" ")
		p.nodeInline(v.Right)
	case *IfExpr:
		p.write("if ")
		p.nodeInline(v.Cond)
		p.write(" then ")
		p.nodeInline(v.Then)
		for _, ei := range v.ElseIfs {
			p.write(" elif ")
			p.nodeInline(ei.Cond)
			p.write(" then ")
			p.nodeInline(ei.Then)
		}
		if v.Else != nil {
			p.write(" else ")
			p.nodeInline(v.Else)
		}
		p.write(" end")
	case *ReduceExpr:
		p.write("reduce ")
		p.nodeInline(v.Expr)
		p.write(" as ")
		p.nodeInline(v.Pattern)
		p.write(" (")
		p.nodeInline(v.Init)
		p.write("; ")
		p.nodeInline(v.Update)
		p.write(")")
	case *ForeachExpr:
		p.write("foreach ")
		p.nodeInline(v.Expr)
		p.write(" as ")
		p.nodeInline(v.Pattern)
		p.write(" (")
		p.nodeInline(v.Init)
		p.write("; ")
		p.nodeInline(v.Update)
		if v.Extract != nil {
			p.write("; ")
			p.nodeInline(v.Extract)
		}
		p.write(")")
	case *TryExpr:
		p.write("try ")
		p.nodeInline(v.Body)
		if v.Handler != nil {
			p.write(" catch ")
			p.nodeInline(v.Handler)
		}
	case *Array:
		if v.Elem == nil {
			p.write("[]")
			return
		}
		p.write("[")
		p.nodeInline(v.Elem)
		p.write("]")
	case *Object:
		inl := shortInlineObject(v)
		p.write(inl)
	case *Paren:
		p.write("(")
		p.nodeInline(v.Expr)
		p.write(")")
	case *Optional:
		p.nodeInline(v.Expr)
		p.write("?")
	case *LocalFuncDef:
		bodyInline := shortInline(v.Body)
		p.write("def ")
		p.write(v.Name)
		if len(v.Params) > 0 {
			p.write("(")
			for i, param := range v.Params {
				if i > 0 {
					p.write("; ")
				}
				p.write(param)
			}
			p.write(")")
		}
		p.write(": ")
		p.write(bodyInline)
		p.write("; ")
		p.nodeInline(v.Rest)
	default:
		p.node(n)
	}
}

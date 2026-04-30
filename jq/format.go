package jq

// Format formats a parsed jq File back to source.
//
// Style rules (all backed by corpus samples):
//
//   - Indentation: 1 hard tab per level
//   - Pipe |: emitted at the START of the next line
//     (https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L23-L34)
//   - Comma ,: emitted at the END of the line
//     (https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L6-L20)
//   - Arithmetic ops (+, -, etc.): lead the continuation line when broken
//     (https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L22-L28)
//   - def body: indented 1 tab; closing ; at the def's own indentation
//     (https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L7-L39)
//   - if/then/else/end: inline when everything fits <= shortThreshold
//     (https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L22-L29 multi, :35 inline)
//   - foreach/reduce: always multi-line
//   - Object literals: multi-line with { key: val, } per line
//     (https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L5-L16)
//   - COMMENT RULE: trailing comments force multi-line on the enclosing
//     comma/pipe sequence; leading comments are emitted before their node.
//     (gofmt rule; corpus ref: deb822.jq "# inject a synthetic blank line…")

import (
	"regexp"
	"strings"
)

const shortThreshold = 60

// FormatFile formats a complete jq file.
func FormatFile(f *File) string {
	p := &printer{}
	p.file(f)
	return p.out.String()
}

// FormatFileTidy formats f with tidy-level index-notation normalisations:
//
//	.["foo"]  → .foo   (identifier-safe bracket key → dot notation)
//	."foo"    → .foo   (dot-quoted identifier-safe key → dot notation)
//	.["foo-bar"] → ."foo-bar"  (non-identifier bracket → dot-quoted)
func FormatFileTidy(f *File) string {
	p := &printer{tidy: true}
	p.file(f)
	return p.out.String()
}

// FormatNode formats a single AST node.
func FormatNode(n Node) string {
	p := &printer{}
	p.node(n)
	return p.out.String()
}

// FormatNodeInline formats a single AST node as a single-line compact string.
// Used for jq-in-shell single-line expressions.
func FormatNodeInline(n Node) string {
	p := &printer{}
	p.nodeInline(n)
	return p.out.String()
}

// printer accumulates formatted output.
type printer struct {
	out             strings.Builder
	depth           int
	lastWasTrailing bool // true immediately after emitting a trailing comment
	tidy            bool // apply tidy-level normalisations (index notation, etc.)
}

// clearTrailing resets lastWasTrailing and returns its old value.
func (p *printer) clearTrailing() bool {
	v := p.lastWasTrailing
	p.lastWasTrailing = false
	return v
}

// closeDelimiter writes a closing bracket/paren.
//
// In "trailing comment" state (set by commentedExpr or writeAfterPunct), adds
// a newline first so the delimiter appears on its own line and not inside the
// comment.  In multi-line-block mode (atBlockEnd == true), always adds a
// newline before the delimiter regardless (same as the old explicit p.newline()
// call in array/paren/object formatters).
func (p *printer) closeDelimiter(s string) {
	if p.lastWasTrailing {
		p.newline()
		p.lastWasTrailing = false
	}
	p.write(s)
}


func (p *printer) tab() string { return strings.Repeat("\t", p.depth) }
func (p *printer) write(s string) {
	p.out.WriteString(s)
}
func (p *printer) writeln(s string) { p.out.WriteString(s); p.out.WriteByte('\n') }
func (p *printer) indent()          { p.depth++ }
func (p *printer) dedent()          { p.depth-- }

// newline writes a newline followed by the current indentation, and resets
// lastWasTrailing (a newline always means we're on a fresh line).
func (p *printer) newline() {
	p.lastWasTrailing = false
	p.out.WriteByte('\n')
	p.write(p.tab())
}

// nl writes just a bare newline.
func (p *printer) nl() { p.out.WriteByte('\n') }

// atLineStart reports whether the printer is logically at the start of a line:
// either the buffer is empty, or everything after the last newline is whitespace.
// This is true after p.newline() (which writes "\n" + tabs) as well as after p.nl().
func (p *printer) atLineStart() bool {
	s := p.out.String()
	if s == "" {
		return true
	}
	lastNL := strings.LastIndex(s, "\n")
	if lastNL < 0 {
		return strings.TrimLeft(s, " \t") == ""
	}
	return strings.TrimLeft(s[lastNL+1:], " \t") == ""
}

// ── helpers for comment-forced multi-line ────────────────────────────────────

// hasTrailingComment reports whether n is a CommentedExpr with a trailing comment.
func hasTrailingComment(n Node) bool {
	ce, ok := n.(*CommentedExpr)
	return ok && ce.TrailingComment != nil
}

// hasAnyComment reports whether n is a CommentedExpr (has any kind of comment).
func hasAnyComment(n Node) bool {
	_, ok := n.(*CommentedExpr)
	return ok
}

// anyPartHasTrailingComment reports whether any element has a trailing comment.
func anyPartHasTrailingComment(parts []Node) bool {
	for _, part := range parts {
		if hasTrailingComment(part) {
			return true
		}
	}
	return false
}

// anyPartHasComment reports whether any element has any kind of comment.
func anyPartHasComment(parts []Node) bool {
	for _, part := range parts {
		if hasAnyComment(part) {
			return true
		}
	}
	return false
}

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
	p.comments(i.LeadingComments)
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
		p.writeln(c.Text)
		p.write(p.tab())
	}
}

// localFuncDef formats a local function definition scoped to REST.
// Short bodies stay on one line; long bodies use multi-line form.
// Style ref: https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L21-L22
func (p *printer) localFuncDef(v *LocalFuncDef) {
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

	bodyInline := p.shortInline(v.Body)
	if len(sig.String())+len(bodyInline) <= 100 && !strings.Contains(bodyInline, "\n") {
		p.write(sig.String())
		p.write(bodyInline)
		p.write(";")
	} else {
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

// funcDef formats a top-level def: always multi-line body.
// Style ref: https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L7-L39
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
	case *CommentedExpr:
		p.commentedExpr(v)
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
		inner := p.inlineSafe(v.Expr)
		if inner != "" && len(inner) <= shortThreshold && !strings.Contains(inner, "\n") && !hasAnyComment(v.Expr) {
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

// ── CommentedExpr ────────────────────────────────────────────────────────────

// commentedExpr emits leading comments, then the expression, then the trailing
// comment (if any) on the same line.
//
// If the printer is not at the start of a line when this is called, a newline
// is injected first — this ensures comments always appear on their own line,
// never appended to the end of the previous token.
//
// After emitting a trailing comment, sets lastWasTrailing so closing delimiters
// (], ), }) know to go on the next line instead of inside the comment.
func (p *printer) commentedExpr(v *CommentedExpr) {
	p.lastWasTrailing = false
	for _, c := range v.LeadingComments {
		if !p.atLineStart() {
			p.nl()
			p.write(p.tab())
		}
		p.writeln(c.Text)
		p.write(p.tab())
	}
	p.node(v.Expr)
	if v.TrailingComment != nil {
		p.write(" ")
		p.write(v.TrailingComment.Text)
		p.lastWasTrailing = true
	}
}

// ── expression formatters ────────────────────────────────────────────────────

// pipeChain formats a | chain.
// | at the START of continuation lines; forced multi-line if any part has a
// trailing comment (gofmt rule).
// Style ref: https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L23-L34
func (p *printer) pipeChain(v *Pipe) {
	parts := flattenPipe(v)

	// Force multi-line if any part has any comment (trailing or leading).
	// Trailing comments can only live at end-of-line; leading comments need
	// their own line — both require multi-line layout.
	if anyPartHasComment(parts) {
		p.pipeChainMultiLine(parts)
		return
	}

	inline := p.inlineSafe(v)
	if inline != "" && len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}
	p.pipeChainMultiLine(parts)
}

func (p *printer) pipeChainMultiLine(parts []Node) {
	for i, part := range parts {
		if i > 0 {
			// If this part has leading comments, emit them BEFORE the |.
			// This matches the corpus style where comments between pipe
			// elements appear on their own line before the | :
			//   prev_expr
			//   # comment
			//   | next_expr
			// (corpus: version-components.jq lines 13-14)
			if ce, ok := part.(*CommentedExpr); ok && len(ce.LeadingComments) > 0 {
				for _, c := range ce.LeadingComments {
					p.newline()
					p.write(c.Text)
				}
				p.newline()
				p.write("| ")
				// Emit the rest of the CommentedExpr (expr + trailing comment)
				// without repeating the leading comments.
				p.node(ce.Expr)
				if ce.TrailingComment != nil {
					p.write(" ")
					p.write(ce.TrailingComment.Text)
				}
				continue
			}
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
// , at END of line; forced multi-line if any part has a trailing comment.
// Style ref: https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L6-L20
func (p *printer) commaExpr(v *Comma) {
	parts := flattenComma(v)

	if anyPartHasComment(parts) {
		p.commaExprMultiLine(parts)
		return
	}

	inline := p.inlineSafe(v)
	if inline != "" && len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}
	p.commaExprMultiLine(parts)
}

func (p *printer) commaExprMultiLine(parts []Node) {
	for i, part := range parts {
		if i > 0 {
			// The comma belongs after the previous part.  Leading comments of
			// the CURRENT part belong before it (handled by commentedExpr).
			p.newline()
		}
		if i < len(parts)-1 {
			// For all but the last part: strip trailing comment, emit node,
			// then write "," BEFORE the trailing comment so it isn't eaten.
			inner, tc := stripTrailing(part)
			p.node(inner)
			p.writeAfterPunct(",", tc)
		} else {
			p.node(part)
		}
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
// Style ref: https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L24
func (p *printer) asExpr(v *AsExpr) {
	p.node(v.Expr)
	p.write(" as ")
	p.node(v.Pattern)
	p.newline()
	p.write("| ")
	p.node(v.Body)
}

// binOp formats a binary expression.
// For chained arithmetic/logical ops that exceed threshold, each operand starts
// on a new line with the operator leading.
// Style ref: https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/dpkg-version.jq#L22-L28
func (p *printer) binOp(v *BinOp) {
	if v.Op == "neg" {
		p.write("-")
		p.node(v.Left)
		return
	}
	if v.Op == "" {
		p.node(v.Left)
		p.node(v.Right)
		return
	}
	if v.Op == "|" {
		// Inline pipe from suffix (format expressions)
		p.node(v.Left)
		p.write(" | ")
		p.node(v.Right)
		return
	}

	inline := p.inlineSafe(v)
	if inline != "" && len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}

	// Multi-line: flatten same-op chain so all parts break uniformly.
	type part struct {
		op   string
		node Node
	}
	var parts []part
	var flatten func(n Node)
	flatten = func(n Node) {
		if b, ok := n.(*BinOp); ok && b.Op == v.Op {
			flatten(b.Left)
			parts = append(parts, part{op: b.Op, node: b.Right})
		} else {
			parts = append(parts, part{node: n})
		}
	}
	flatten(v)

	for i, pt := range parts {
		if i > 0 {
			p.newline()
			p.write(pt.op)
			p.write(" ")
		}
		p.node(pt.node)
	}
}

// ifExpr formats if/then/else/end.
// Inline when everything fits; multi-line otherwise.
// Style refs: dpkg-version.jq:22-29 (multi), :35 (inline); deb822.jq:18-35
func (p *printer) ifExpr(v *IfExpr) {
	// Any comment forces multi-line.
	forcedMulti := hasAnyComment(v.Then) || hasAnyComment(v.Else)
	for _, ei := range v.ElseIfs {
		if hasAnyComment(ei.Then) {
			forcedMulti = true
		}
	}

	// If the condition has leading comments, they belong BEFORE the `if` keyword
	// (they appeared before `if` in the source).  Stripping them from the
	// condition and emitting them first prevents non-idempotency: without this,
	// the comment ends up between `if ` and the condition on the first format
	// pass, and the second pass re-parses it as a comment inside the condition's
	// first argument — making the output diverge.
	var condLeadingComments []*Comment
	cond := v.Cond
	if ce, ok := v.Cond.(*CommentedExpr); ok && len(ce.LeadingComments) > 0 {
		condLeadingComments = ce.LeadingComments
		forcedMulti = true
		if ce.TrailingComment != nil {
			cond = &CommentedExpr{At: ce.At, Expr: ce.Expr, TrailingComment: ce.TrailingComment}
		} else {
			cond = ce.Expr
		}
	}

	if !forcedMulti {
		inline := p.inlineSafe(v)
		if inline != "" && len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
			p.write(inline)
			return
		}
	}

	for _, c := range condLeadingComments {
		if !p.atLineStart() {
			p.nl()
			p.write(p.tab())
		}
		p.writeln(c.Text)
		p.write(p.tab())
	}
	p.write("if ")
	p.node(cond)
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
		elseInline := p.shortInline(v.Else)
		if len(elseInline) <= 30 && !strings.Contains(elseInline, "\n") && !hasAnyComment(v.Else) {
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

// reduceExpr: always multi-line.
// Style ref: https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L67-L68
func (p *printer) reduceExpr(v *ReduceExpr) {
	p.write("reduce ")
	p.node(v.Expr)
	p.write(" as ")
	p.node(v.Pattern)
	p.write(" (")
	p.indent()
	p.newline()
	initInner, initTC := stripTrailing(v.Init)
	p.node(initInner)
	p.writeAfterPunct(";", initTC)
	p.newline()
	p.node(v.Update)
	p.dedent()
	p.newline()
	p.write(")")
}

// foreachExpr: always multi-line.
// Style ref: https://github.com/tianon/debian-bin/blob/d508ea34f15e88b8ac63d71ffb1938fccbc21206/jq/deb822.jq#L8-L38
func (p *printer) foreachExpr(v *ForeachExpr) {
	p.write("foreach ")
	p.node(v.Expr)
	p.write(" as ")
	p.node(v.Pattern)
	p.write(" (")
	p.indent()
	p.newline()
	initInner, initTC := stripTrailing(v.Init)
	p.node(initInner)
	p.writeAfterPunct(";", initTC)
	p.newline()
	updateInner, updateTC := stripTrailing(v.Update)
	p.node(updateInner)
	if v.Extract != nil {
		extractInline := p.shortInline(v.Extract)
		if len(extractInline) <= 50 && !strings.Contains(extractInline, "\n") && !hasAnyComment(v.Extract) {
			// Short extract on the same close line, with the `;` separator.
			// Style ref: deb822.jq:60 "; if .out then .out else empty end)"
			// The updateTC comment (if any) goes after the node but before the ";".
			// We OMIT the trailing comment here because if we wrote it, we'd need
			// to start a new line — defeating the "short extract on close line" goal.
			// In practice, trailing comments on the update of a foreach are rare.
			if updateTC != nil {
				p.write(" ")
				p.write(updateTC.Text)
			}
			p.dedent()
			p.newline()
			p.write("; ")
			p.write(extractInline)
		} else {
			// Multi-line extract: `;` separates update from extract.
			// updateTC goes after update and before the `;`.
			p.writeAfterPunct(";", updateTC)
			p.newline()
			p.node(v.Extract)
			p.dedent()
			p.newline()
		}
	} else {
		// No extract: trailing comment (if any) goes inline after update.
		if updateTC != nil {
			p.write(" ")
			p.write(updateTC.Text)
		}
		p.dedent()
		p.newline()
	}
	p.closeDelimiter(")")
}

// tryExpr: try body (catch handler)?
func (p *printer) tryExpr(v *TryExpr) {
	p.write("try ")
	p.node(v.Body)
	if v.Handler != nil {
		p.write(" catch ")
		p.node(v.Handler)
	}
}

func (p *printer) indexExpr(v *Index) {
	// Tidy: normalise string-key index notation.
	//   .["foo"]   → .foo    (identifier-safe bracket → dot notation)
	//   ."foo"     → .foo    (dot-quoted identifier-safe → dot notation)
	//   .["foo-bar"] → ."foo-bar"  (non-identifier bracket → dot-quoted)
	if p.tidy {
		if strKey, ok := v.Key.(*StrLit); ok {
			unquoted := unquoteStrLit(strKey.Raw)
			if unquoted != "" {
				_, exprIsIdentity := v.Expr.(*Identity)
				exprIsNilOrIdentity := v.Expr == nil || exprIsIdentity
				if !exprIsNilOrIdentity {
					// On an arbitrary expression: emit the expression first.
					p.node(v.Expr)
				}
				if isIdentifier(unquoted) {
					p.write(".")
					p.write(unquoted)
				} else {
					// Dot-quoted form: ."foo-bar"
					p.write(".")
					p.write(strKey.Raw)
				}
				if v.Optional {
					p.write("?")
				}
				return
			}
		}
	}

	// Non-tidy: preserve the original dot-access form (."key") vs bracket form (.["key"]).
	if v.DotAccess {
		if v.Expr != nil {
			p.node(v.Expr)
		}
		p.write(".")
		p.node(v.Key)
		if v.Optional {
			p.write("?")
		}
		return
	}

	if v.Expr != nil {
		p.node(v.Expr)
	} else {
		// nil Expr with bracket notation: .[key] on identity — emit leading dot.
		p.write(".")
	}
	if v.Key == nil {
		p.write("[]")
	} else {
		p.write("[")
		p.node(v.Key)
		p.closeDelimiter("]")
	}
	if v.Optional {
		p.write("?")
	}
}

func (p *printer) sliceExpr(v *Slice) {
	if v.Expr != nil {
		p.node(v.Expr)
	} else {
		p.write(".")
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
	if len(v.Args) == 0 {
		return
	}

	// If any argument has comments, use multi-line call format so each argument
	// starts on its own line (ensuring leading comments are at line start).
	anyCommented := false
	for _, arg := range v.Args {
		if hasAnyComment(arg) {
			anyCommented = true
			break
		}
	}

	if anyCommented {
		p.write("(")
		p.indent()
		for i, arg := range v.Args {
			if i > 0 {
				p.write(";")
			}
			p.newline()
			p.node(arg)
		}
		p.dedent()
		p.newline()
		p.write(")")
		return
	}

	p.write("(")
	for i, arg := range v.Args {
		if i > 0 {
			p.write("; ")
		}
		p.node(arg)
	}
	p.closeDelimiter(")")
}

// arrayExpr: multi-line when element is complex.
func (p *printer) arrayExpr(v *Array) {
	if v.Elem == nil {
		p.write("[]")
		return
	}
	if hasAnyComment(v.Elem) {
		p.write("[")
		p.indent()
		p.newline()
		p.node(v.Elem)
		p.dedent()
		p.newline()
		p.write("]")
		return
	}
	elemInline := p.inlineSafe(v.Elem)
	if elemInline != "" && len("["+elemInline+"]") <= shortThreshold && !strings.Contains(elemInline, "\n") {
		p.write("[" + elemInline + "]")
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

// objectExpr: multi-line with trailing commas.
// Style ref: https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/scratch/multiarch.jq#L5-L16
func (p *printer) objectExpr(v *Object) {
	if len(v.Fields) == 0 {
		p.write("{}")
		return
	}
	inline := p.shortInlineObject(v)
	if inline != "" && len(inline) <= shortThreshold && !strings.Contains(inline, "\n") {
		p.write(inline)
		return
	}
	p.write("{")
	p.indent()
	for _, f := range v.Fields {
		p.newline()
		tc := p.objectField(f)
		// Comma must come BEFORE the trailing comment so it isn't eaten:
		//   key: value, # comment  ← correct
		//   key: value # comment,  ← wrong (comma inside comment)
		p.writeAfterPunct(",", tc)
	}
	p.dedent()
	p.newline()
	p.write("}")
}

// objectField emits the field's leading comments (if any), the key, and the
// value.  If the value has a trailing comment, it is NOT emitted — it is
// returned to the caller so that structural punctuation (the field comma) can
// be written before the comment rather than after it.
func (p *printer) objectField(f *ObjectField) *Comment {
	// Emit field-level leading comments (e.g. comment before the key).
	for _, c := range f.LeadingComments {
		if !p.atLineStart() {
			p.nl()
			p.write(p.tab())
		}
		p.writeln(c.Text)
		p.write(p.tab())
	}

	if v, ok := f.Key.(*Var); ok && f.Value == nil {
		// {$foo} shorthand
		p.write(v.Name)
		return nil
	}
	// Unquote string keys that are valid bare identifiers: {"foo": .} → {foo: .}
	if sl, ok := f.Key.(*StrLit); ok {
		if bare := bareKey(sl.Raw); bare != "" {
			p.write(bare)
			if f.KeyOptional {
				p.write("?")
			}
			if f.Value != nil {
				p.write(": ")
				val, tc := stripTrailing(f.Value)
				p.node(val)
				return tc
			}
			return nil
		}
	}
	p.node(f.Key)
	if f.KeyOptional {
		p.write("?")
	}
	if f.Value != nil {
		p.write(": ")
		val, tc := stripTrailing(f.Value)
		p.node(val)
		return tc
	}
	return nil
}

// ── structural-punctuation-before-trailing-comment helpers ───────────────────

// stripTrailing returns the node with its trailing comment removed (if any),
// and the trailing comment separately.  Use this when structural punctuation
// (comma, semicolon) must appear AFTER the value but BEFORE the trailing comment:
//
//	key: value, # comment   ← correct
//	key: value # comment,   ← wrong (comma is eaten by the comment)
func stripTrailing(n Node) (Node, *Comment) {
	if ce, ok := n.(*CommentedExpr); ok && ce.TrailingComment != nil {
		tc := ce.TrailingComment
		var inner Node
		if len(ce.LeadingComments) > 0 {
			inner = &CommentedExpr{At: ce.At, LeadingComments: ce.LeadingComments, Expr: ce.Expr}
		} else {
			inner = ce.Expr
		}
		return inner, tc
	}
	return n, nil
}

// writeAfterPunct emits `punct` then (if tc != nil) ` tc.Text` on the same line.
// Sets lastWasTrailing when a trailing comment is emitted, so that any
// subsequent closeDelimiter call knows to add a newline first.
func (p *printer) writeAfterPunct(punct string, tc *Comment) {
	p.write(punct)
	if tc != nil {
		p.write(" ")
		p.write(tc.Text)
		p.lastWasTrailing = true
	}
}

// ── "short inline" helpers ───────────────────────────────────────────────────

func (p *printer) shortInline(n Node) string {
	ip := &printer{tidy: p.tidy}
	ip.nodeInline(n)
	return ip.out.String()
}

// inlineSafe returns the inline representation of n if it is safe to follow
// immediately with a closing delimiter (], ), }).  If n or any subnode carries
// a trailing comment, the delimiter would land inside the comment — in that
// case, returns "" to signal "cannot inline".
//
// This uses an AST walk rather than a text scan to avoid false positives from
// string literals that happen to contain " #" in their text content.
func (p *printer) inlineSafe(n Node) string {
	if anyNodeHasTrailingComment(n) {
		return ""
	}
	return p.shortInline(n)
}

// anyNodeHasTrailingComment recursively checks whether n or any of its
// directly-rendered sub-nodes carries a trailing comment.
func anyNodeHasTrailingComment(n Node) bool {
	if n == nil {
		return false
	}
	switch v := n.(type) {
	case *CommentedExpr:
		// Leading comments are line comments: they must appear on their own line,
		// so any CommentedExpr with leading comments cannot be safely inlined.
		return v.TrailingComment != nil || len(v.LeadingComments) > 0 || anyNodeHasTrailingComment(v.Expr)
	case *Pipe:
		return anyNodeHasTrailingComment(v.Left) || anyNodeHasTrailingComment(v.Right)
	case *Comma:
		return anyNodeHasTrailingComment(v.Left) || anyNodeHasTrailingComment(v.Right)
	case *BinOp:
		return anyNodeHasTrailingComment(v.Left) || anyNodeHasTrailingComment(v.Right)
	case *AsExpr:
		return anyNodeHasTrailingComment(v.Expr) || anyNodeHasTrailingComment(v.Body)
	case *IfExpr:
		if anyNodeHasTrailingComment(v.Then) || anyNodeHasTrailingComment(v.Else) {
			return true
		}
		for _, ei := range v.ElseIfs {
			if anyNodeHasTrailingComment(ei.Then) {
				return true
			}
		}
	case *Array:
		return anyNodeHasTrailingComment(v.Elem)
	case *Object:
		for _, f := range v.Fields {
			_, tc := stripTrailing(f.Value)
			if tc != nil || anyNodeHasTrailingComment(f.Value) {
				return true
			}
		}
	case *Paren:
		return anyNodeHasTrailingComment(v.Expr)
	case *Optional:
		return anyNodeHasTrailingComment(v.Expr)
	case *Index:
		return anyNodeHasTrailingComment(v.Key)
	case *Slice:
		return anyNodeHasTrailingComment(v.Start) || anyNodeHasTrailingComment(v.End)
	}
	return false
}

// shortInlineObject returns "" if unsafe to inline (any trailing comment on a field value).
func (p *printer) shortInlineObject(v *Object) string {
	if len(v.Fields) == 0 {
		return "{}"
	}
	// Check: any trailing comment on a field value makes inline unsafe.
	for _, f := range v.Fields {
		if _, tc := stripTrailing(f.Value); tc != nil {
			return ""
		}
	}
	var b strings.Builder
	b.WriteString("{ ")
	for i, f := range v.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		ip := &printer{tidy: p.tidy}
		ip.objectField(f)
		b.WriteString(ip.out.String())
	}
	b.WriteString(" }")
	return b.String()
}

// nodeInline renders n as a single-line string (no newlines ever).
// Used for threshold checks and for jq-in-shell inline mode.
func (p *printer) nodeInline(n Node) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *CommentedExpr:
		// Leading comments are line-terminated: inlining them would make subsequent
		// tokens unreachable.  Fall back to the multi-line printer.
		if len(v.LeadingComments) > 0 {
			p.node(n)
			return
		}
		p.nodeInline(v.Expr)
		if v.TrailingComment != nil {
			p.write(" ")
			p.write(v.TrailingComment.Text)
		}
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
	case *LocalFuncDef:
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
		p.nodeInline(v.Body)
		p.write("; ")
		p.nodeInline(v.Rest)
	case *Array:
		if v.Elem == nil {
			p.write("[]")
			return
		}
		p.write("[")
		p.nodeInline(v.Elem)
		p.write("]")
	case *Object:
		inl := p.shortInlineObject(v)
		if inl != "" {
			p.write(inl)
		} else {
			p.node(n) // fall back to multi-line
		}
	case *Paren:
		p.write("(")
		p.nodeInline(v.Expr)
		p.write(")")
	case *Optional:
		p.nodeInline(v.Expr)
		p.write("?")
	default:
		p.node(n)
	}
}

// ── key helpers ───────────────────────────────────────────────────────────────

var jqIdentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// bareKey returns the unquoted form of a double-quoted string literal key if
// it is a valid jq identifier with no escape sequences (e.g. "foo" → foo).
// Returns "" if the key must stay quoted.
func bareKey(raw string) string {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return ""
	}
	inner := raw[1 : len(raw)-1]
	if strings.ContainsRune(inner, '\\') {
		return "" // escape sequences — keep quoted
	}
	if !jqIdentRe.MatchString(inner) {
		return "" // not a bare identifier (contains dots, hyphens, etc.)
	}
	return inner
}

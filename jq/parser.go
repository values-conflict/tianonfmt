package jq

import (
	"fmt"
	"strings"
)

// ParseFile parses a complete .jq file (module, imports, function defs, query).
func ParseFile(src string) (*File, error) {
	p := &parser{lex: NewLexer(src), src: src}
	return p.parseFile()
}

// ParseExpr parses a single jq expression (no top-level def/import/module).
func ParseExpr(src string) (Node, error) {
	p := &parser{lex: NewLexer(src), src: src}
	node, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.Kind != EOF {
		return nil, p.errorf("unexpected token %s after expression", tok.Kind)
	}
	return node, nil
}

type parser struct {
	lex             *Lexer
	src             string
	pending         []*Comment // leading comments (own-line)
	pendingTrailing *Comment   // trailing comment (same line as last non-comment token)
	lastLine        int        // line of the last non-comment token consumed
}

// ── comment harvesting ───────────────────────────────────────────────────────
//
// The key distinction:
//   - A comment on the SAME LINE as the preceding non-comment token is a
//     "trailing" comment for that token's expression.
//   - A comment on its OWN LINE (different from preceding) is a "leading"
//     comment for the next non-comment token's expression.
//
// trailing comments go into p.pendingTrailing (only one can exist at a time).
// leading comments go into p.pending (a slice, since there can be many).

func (p *parser) next() Token {
	for {
		t := p.lex.Next()
		if t.Kind == COMMENT {
			p.absorbComment(t)
			continue
		}
		p.lastLine = t.Line
		return t
	}
}

func (p *parser) peek() Token {
	for {
		t := p.lex.Peek()
		if t.Kind == COMMENT {
			p.lex.Next()
			p.absorbComment(t)
			continue
		}
		return t
	}
}

func (p *parser) absorbComment(t Token) {
	c := &Comment{At: t.At, Line: t.Line, Text: t.Text}
	if p.lastLine > 0 && t.Line == p.lastLine {
		// Trailing: on the same line as the last non-comment token.
		p.pendingTrailing = c
	} else {
		// Leading: on its own line before the next real token.
		p.pending = append(p.pending, c)
	}
}

// takePendingTrailing returns and clears the pending trailing comment.
func (p *parser) takePendingTrailing() *Comment {
	tc := p.pendingTrailing
	p.pendingTrailing = nil
	return tc
}

// drainComments returns and clears all pending leading comments.
func (p *parser) drainComments() []*Comment {
	c := p.pending
	p.pending = nil
	return c
}

// wrapComments wraps n with any pending leading comments and the trailing
// comment (if any) that was set during or after parsing n.
// Returns n unchanged if there are no comments.
func (p *parser) wrapComments(n Node) Node {
	leading := p.drainComments()
	tc := p.takePendingTrailing()
	if len(leading) == 0 && tc == nil {
		return n
	}
	return &CommentedExpr{
		At:              n.nodePos(),
		LeadingComments: leading,
		Expr:            n,
		TrailingComment: tc,
	}
}

// ── error helpers ────────────────────────────────────────────────────────────

func (p *parser) errorf(format string, args ...interface{}) error {
	return fmt.Errorf("jq parse error: "+format, args...)
}

func (p *parser) expect(k Kind) (Token, error) {
	t := p.next()
	if t.Kind != k {
		return t, p.errorf("expected %s, got %s (%q)", k, t.Kind, t.Text)
	}
	return t, nil
}

// ── top-level ────────────────────────────────────────────────────────────────

func (p *parser) parseFile() (*File, error) {
	f := &File{At: 0}

	if p.peek().Kind == KWMODULE {
		m, err := p.parseModuleStmt()
		if err != nil {
			return nil, err
		}
		f.Module = m
	}

	for p.peek().Kind == KWIMPORT || p.peek().Kind == KWINCLUDE {
		leading := p.drainComments()
		imp, err := p.parseImportStmt()
		if err != nil {
			return nil, err
		}
		imp.LeadingComments = append(leading, imp.LeadingComments...)
		f.Imports = append(f.Imports, imp)
	}

	for p.peek().Kind != EOF {
		leading := p.drainComments()
		if p.peek().Kind == KWDEF {
			fd, err := p.parseFuncDef()
			if err != nil {
				return nil, err
			}
			fd.LeadingComments = append(leading, fd.LeadingComments...)
			f.FuncDefs = append(f.FuncDefs, fd)
		} else {
			q, err := p.parseQuery(0)
			if err != nil {
				return nil, err
			}
			if len(leading) > 0 {
				q = &CommentedExpr{At: q.nodePos(), LeadingComments: leading, Expr: q}
			}
			f.Query = q
			break
		}
	}

	return f, nil
}

func (p *parser) parseModuleStmt() (*ModuleStmt, error) {
	tok, _ := p.expect(KWMODULE)
	meta, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(SEMI); err != nil {
		return nil, err
	}
	return &ModuleStmt{At: tok.At, Meta: meta}, nil
}

func (p *parser) parseImportStmt() (*ImportStmt, error) {
	tok := p.next()
	isInclude := tok.Kind == KWINCLUDE

	pathTok, err := p.expect(STR)
	if err != nil {
		return nil, err
	}

	stmt := &ImportStmt{
		At:      tok.At,
		Include: isInclude,
		Path:    pathTok.Text,
	}

	if !isInclude {
		if _, err := p.expect(KWAS); err != nil {
			return nil, err
		}
		varTok, err := p.expect(VAR)
		if err != nil {
			return nil, err
		}
		stmt.Binding = varTok.Text
	}

	if p.peek().Kind == LBRACE {
		meta, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		stmt.Meta = meta
	}

	if _, err := p.expect(SEMI); err != nil {
		return nil, err
	}
	return stmt, nil
}

func (p *parser) parseFuncDef() (*FuncDef, error) {
	tok, _ := p.expect(KWDEF)
	nameTok := p.next()
	if nameTok.Kind != IDENT && nameTok.Kind != FIELD {
		return nil, p.errorf("expected function name after def, got %s", nameTok.Kind)
	}
	name := nameTok.Text

	var params []string
	if p.peek().Kind == LPAREN {
		p.next()
		for {
			pt := p.next()
			if pt.Kind != VAR && pt.Kind != IDENT {
				return nil, p.errorf("expected parameter name, got %s", pt.Kind)
			}
			params = append(params, pt.Text)
			sep := p.next()
			if sep.Kind == RPAREN {
				break
			}
			if sep.Kind != SEMI {
				return nil, p.errorf("expected ; or ) in parameter list, got %s", sep.Kind)
			}
		}
	}

	if _, err := p.expect(COLON); err != nil {
		return nil, err
	}

	body, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(SEMI); err != nil {
		return nil, err
	}

	return &FuncDef{At: tok.At, Name: name, Params: params, Body: body}, nil
}

// ── query / expression ───────────────────────────────────────────────────────

const (
	precLowest = 0
	precPipe   = 1
	precComma  = 2
	precAlt    = 3
	precAssign = 4
	precOr     = 5
	precAnd    = 6
	precCmp    = 7
	precAdd    = 8
	precMul    = 9
)

func (p *parser) parseQuery(minPrec int) (Node, error) {
	left, err := p.parseExpr(minPrec)
	if err != nil {
		return nil, err
	}

	// "as $pattern | body" — right-associative at pipe level
	if p.peek().Kind == KWAS && minPrec <= precPipe {
		p.next()
		pat, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(PIPE); err != nil {
			return nil, err
		}
		body, err := p.parseQuery(precPipe)
		if err != nil {
			return nil, err
		}
		return &AsExpr{At: pos(left), Expr: left, Pattern: pat, Body: body}, nil
	}

	return left, nil
}

// parseExpr is the Pratt parser.  After each operand is parsed, we check for
// a trailing comment (same line as the operand's last token) and any leading
// comments that were drained before the operand.  These are attached to the node.
//
// Pre-first-token comments (absorbed by parsePrimary's peek()) are handled in
// parseTerm itself and arrive here already wrapped on the left node.
func (p *parser) parseExpr(minPrec int) (Node, error) {
	// Leading comments drained before parseTerm starts.
	preLeading := p.drainComments()

	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}

	// parseTerm may have returned a CommentedExpr (pre-first-token comments).
	// Merge preLeading and any trailing comment into it to avoid double-wrapping.
	tc := p.takePendingTrailing()

	if len(preLeading) > 0 {
		if ce, ok := left.(*CommentedExpr); ok {
			ce.LeadingComments = append(preLeading, ce.LeadingComments...)
		} else {
			left = &CommentedExpr{At: left.nodePos(), LeadingComments: preLeading, Expr: left}
		}
	}
	if tc != nil {
		if ce, ok := left.(*CommentedExpr); ok {
			ce.TrailingComment = tc
		} else {
			left = &CommentedExpr{At: left.nodePos(), Expr: left, TrailingComment: tc}
		}
	}

	for {
		opText, prec, rightPrec, ok := p.infixOp()
		if !ok || prec < minPrec {
			break
		}
		prevLine := p.lastLine
		opTok := p.next()

		// For comma: a comment on the SAME LINE as the comma is a trailing
		// comment for the LEFT operand, not a leading comment for the right.
		// Peek once to absorb it and attach it to left before parsing right.
		// (This mirrors the same fix in parseObject for the field-comma case.)
		if opText == "," {
			p.peek() // absorbs any same-line comment into pendingTrailing
			if tc := p.takePendingTrailing(); tc != nil {
				if ce, ok := left.(*CommentedExpr); ok {
					if ce.TrailingComment == nil {
						ce.TrailingComment = tc
					}
				} else {
					left = &CommentedExpr{
						At:              left.nodePos(),
						Expr:            left,
						TrailingComment: tc,
					}
				}
			}
		}

		// Drain leading comments between the operator and the right operand.
		rightLeading := p.drainComments()

		// Capture the line of the right side's first token BEFORE parsing it,
		// so we can detect multi-line / blank-line separators accurately.
		rightFirstLine := p.peek().Line

		right, err := p.parseExpr(rightPrec)
		if err != nil {
			return nil, err
		}

		// Trailing comment immediately after the right operand.
		rightTC := p.takePendingTrailing()
		// Leave any remaining p.pending entries in place: they are leading
		// comments for the next operand or closing comments for the enclosing
		// delimiter.  They will be picked up by the next rightLeading drain or
		// by the caller's drainComments() call if no next iteration.

		if len(rightLeading) > 0 || rightTC != nil {
			right = &CommentedExpr{
				At:              right.nodePos(),
				LeadingComments: rightLeading,
				Expr:            right,
				TrailingComment: rightTC,
			}
		}

		switch opText {
		case "|":
			// MultiLine: the | token appeared on a different line than the
			// last token of the left operand — source was written multi-line.
			left = &Pipe{At: opTok.At, Left: left, Right: right, MultiLine: opTok.Line > prevLine}
		case ",":
			// MultiLine: the right operand's first real token is on a
			// different line than the , token — source was multi-line.
			// BlankLineAfter: there was a blank line after the comma.
			isML := rightFirstLine > opTok.Line
			hasBlank := rightFirstLine > opTok.Line+1
			left = &Comma{At: opTok.At, Left: left, Right: right, MultiLine: isML, BlankLineAfter: hasBlank}
		default:
			// MultiLine: operator appeared on a different line than the left operand.
			left = &BinOp{At: opTok.At, Op: opText, Left: left, Right: right, MultiLine: opTok.Line > prevLine}
		}
	}
	return left, nil
}

func (p *parser) infixOp() (string, int, int, bool) {
	t := p.peek()
	switch t.Kind {
	case PIPE:
		return "|", precPipe, precPipe, true
	case COMMA:
		return ",", precComma, precComma + 1, true
	case ALT:
		return "//", precAlt, precAlt, true
	case ALTALT:
		return "?//", precAlt, precAlt, true
	case ASSIGN:
		return "=", precAssign, precAssign + 1, true
	case PIPEEQ:
		return "|=", precAssign, precAssign + 1, true
	case PLUSEQ:
		return "+=", precAssign, precAssign + 1, true
	case MINUSEQ:
		return "-=", precAssign, precAssign + 1, true
	case STAREQ:
		return "*=", precAssign, precAssign + 1, true
	case SLASHEQ:
		return "/=", precAssign, precAssign + 1, true
	case PERCENTEQ:
		return "%=", precAssign, precAssign + 1, true
	case ALTEQ:
		return "//=", precAssign, precAssign + 1, true
	case KWOR:
		return "or", precOr, precOr + 1, true
	case KWAND:
		return "and", precAnd, precAnd + 1, true
	case EQ:
		return "==", precCmp, precCmp + 1, true
	case NEQ:
		return "!=", precCmp, precCmp + 1, true
	case LT:
		return "<", precCmp, precCmp + 1, true
	case GT:
		return ">", precCmp, precCmp + 1, true
	case LTEQ:
		return "<=", precCmp, precCmp + 1, true
	case GTEQ:
		return ">=", precCmp, precCmp + 1, true
	case PLUS:
		return "+", precAdd, precAdd + 1, true
	case MINUS:
		return "-", precAdd, precAdd + 1, true
	case STAR:
		return "*", precMul, precMul + 1, true
	case SLASH:
		return "/", precMul, precMul + 1, true
	case PERCENT:
		return "%", precMul, precMul + 1, true
	}
	return "", 0, 0, false
}

// ── term + postfix ───────────────────────────────────────────────────────────

// parseTerm parses a primary expression + postfix operators.
//
// Comment-attachment rule for terms:
//   - Comments absorbed by parsePrimary's peek() calls appear BEFORE the first
//     token of the primary (e.g. the comment between "(" and "add" in a reduce
//     init).  These are PRE-FIRST-TOKEN and become leading comments for this term.
//   - Comments absorbed by parseSuffix's peek() calls appear AFTER the primary
//     (e.g. the comment between "startswith()" and "| not").  These are
//     POST-TERM and must remain in p.pending for the parent expression to assign
//     as a leading comment to the next right operand.
func (p *parser) parseTerm() (Node, error) {
	// Track p.pending length before parsePrimary to isolate pre-first-token comments.
	beforePrimary := len(p.pending)

	primary, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	// Any new entries in p.pending that were added during parsePrimary are
	// pre-first-token comments (absorbed by the first peek() before any
	// non-comment token of the primary was consumed).  Attach them as leading
	// comments to the primary and remove them from p.pending so they don't
	// accidentally get picked up as leading comments for the right operand.
	if afterPrimary := len(p.pending); afterPrimary > beforePrimary {
		prePrimary := make([]*Comment, afterPrimary-beforePrimary)
		copy(prePrimary, p.pending[beforePrimary:])
		p.pending = p.pending[:beforePrimary] // restore

		primary = &CommentedExpr{
			At:              primary.nodePos(),
			LeadingComments: prePrimary,
			Expr:            primary,
		}
	}

	return p.parseSuffix(primary)
}

func (p *parser) parseSuffix(left Node) (Node, error) {
	for {
		switch p.peek().Kind {
		case FIELD:
			tok := p.next()
			right := &Field{At: tok.At, Name: tok.Text}
			left = &BinOp{At: tok.At, Op: "", Left: left, Right: right}

		case DOT:
			tok := p.next()
			switch p.peek().Kind {
			case IDENT:
				nameTok := p.next()
				right := &Field{At: nameTok.At, Name: "." + nameTok.Text}
				left = &BinOp{At: tok.At, Op: "", Left: left, Right: right}
			case STR:
				strTok := p.next()
				key := &StrLit{At: strTok.At, Raw: strTok.Text}
				left = &Index{At: tok.At, Expr: left, Key: key, DotAccess: true}
			default:
				left = &BinOp{At: tok.At, Op: "", Left: left, Right: &Identity{At: tok.At}}
			}

		case LBRACKET:
			node, err := p.parseIndex(left)
			if err != nil {
				return nil, err
			}
			left = node

		case QUEST:
			tok := p.next()
			left = &Optional{At: tok.At, Expr: left}

		case FORMAT:
			tok := p.next()
			var strNode Node
			if p.peek().Kind == STR {
				strTok := p.next()
				strNode = &StrLit{At: strTok.At, Raw: strTok.Text}
			}
			right := &FormatExpr{At: tok.At, Name: tok.Text[1:], Str: strNode}
			left = &BinOp{At: tok.At, Op: "|", Left: left, Right: right}

		default:
			return left, nil
		}
	}
}

func (p *parser) parseIndex(expr Node) (Node, error) {
	at := p.next().At // consume [

	if p.peek().Kind == RBRACKET {
		p.next()
		opt := p.peek().Kind == QUEST
		if opt {
			p.next()
		}
		return &Index{At: at, Expr: expr, Optional: opt}, nil
	}

	if p.peek().Kind == COLON {
		p.next()
		end, err := p.parseQuery(precLowest)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(RBRACKET); err != nil {
			return nil, err
		}
		opt := p.peek().Kind == QUEST
		if opt {
			p.next()
		}
		return &Slice{At: at, Expr: expr, End: end, Optional: opt}, nil
	}

	first, err := p.parseQuery(precLowest)
	if err != nil {
		return nil, err
	}

	if p.peek().Kind == COLON {
		p.next()
		var end Node
		if p.peek().Kind != RBRACKET {
			end, err = p.parseQuery(precLowest)
			if err != nil {
				return nil, err
			}
		}
		if _, err := p.expect(RBRACKET); err != nil {
			return nil, err
		}
		opt := p.peek().Kind == QUEST
		if opt {
			p.next()
		}
		return &Slice{At: at, Expr: expr, Start: first, End: end, Optional: opt}, nil
	}

	if _, err := p.expect(RBRACKET); err != nil {
		return nil, err
	}
	opt := p.peek().Kind == QUEST
	if opt {
		p.next()
	}
	return &Index{At: at, Expr: expr, Key: first, Optional: opt}, nil
}

// ── primary expressions ──────────────────────────────────────────────────────

func (p *parser) parsePrimary() (Node, error) {
	t := p.peek()
	switch t.Kind {
	case KWIF:
		return p.parseIf()
	case KWREDUCE:
		return p.parseReduce()
	case KWFOREACH:
		return p.parseForeach()
	case KWTRY:
		return p.parseTry()
	case KWLABEL:
		return p.parseLabel()
	case KWBREAK:
		return p.parseBreak()
	case KWDEF:
		tok := p.peek()
		fd, err := p.parseFuncDef()
		if err != nil {
			return nil, err
		}
		rest, err := p.parseQuery(0)
		if err != nil {
			return nil, err
		}
		return &LocalFuncDef{
			At: tok.At, Name: fd.Name, Params: fd.Params,
			Body: fd.Body, Rest: rest,
		}, nil

	case LPAREN:
		return p.parseParen()
	case LBRACKET:
		return p.parseArray()
	case LBRACE:
		return p.parseObject()

	case DOTDOT:
		tok := p.next()
		return &Recurse{At: tok.At}, nil

	case DOT:
		tok := p.next()
		switch p.peek().Kind {
		case IDENT:
			nameTok := p.next()
			return &Field{At: tok.At, Name: "." + nameTok.Text}, nil
		case STR:
			strTok := p.next()
			key := &StrLit{At: strTok.At, Raw: strTok.Text}
			return &Index{At: tok.At, Key: key, DotAccess: true}, nil
		}
		return &Identity{At: tok.At}, nil

	case FIELD:
		tok := p.next()
		return &Field{At: tok.At, Name: tok.Text}, nil

	case VAR:
		tok := p.next()
		return &Var{At: tok.At, Name: tok.Text}, nil

	case KWLOC:
		tok := p.next()
		return &LocExpr{At: tok.At}, nil

	case FORMAT:
		tok := p.next()
		var strNode Node
		if p.peek().Kind == STR {
			strTok := p.next()
			strNode = &StrLit{At: strTok.At, Raw: strTok.Text}
		}
		return &FormatExpr{At: tok.At, Name: tok.Text[1:], Str: strNode}, nil

	case IDENT:
		return p.parseIdentOrCall()

	case NUMBER:
		tok := p.next()
		return &NumberLit{At: tok.At, Raw: tok.Text}, nil

	case STR:
		tok := p.next()
		return &StrLit{At: tok.At, Raw: tok.Text}, nil

	case KWAND, KWOR:
		tok := p.next()
		return &Ident{At: tok.At, Name: tok.Text}, nil

	case MINUS:
		tok := p.next()
		operand, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		return &BinOp{At: tok.At, Op: "neg", Left: operand}, nil
	}

	return nil, p.errorf("unexpected token %s (%q) in expression", t.Kind, t.Text)
}

func (p *parser) parseIdentOrCall() (Node, error) {
	tok := p.next()
	name := tok.Text

	switch name {
	case "null":
		return &NullLit{At: tok.At}, nil
	case "true":
		return &BoolLit{At: tok.At, Val: true}, nil
	case "false":
		return &BoolLit{At: tok.At, Val: false}, nil
	}

	if p.peek().Kind == LPAREN {
		p.next()
		var args []Node
		for p.peek().Kind != RPAREN {
			arg, err := p.parseQuery(0)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.peek().Kind == SEMI {
				p.next()
			}
		}
		p.next()
		return &Call{At: tok.At, Name: name, Args: args}, nil
	}

	return &Call{At: tok.At, Name: name}, nil
}

// ── control structures ───────────────────────────────────────────────────────

func (p *parser) parseIf() (*IfExpr, error) {
	tok, _ := p.expect(KWIF)
	cond, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(KWTHEN); err != nil {
		return nil, err
	}
	then, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	// Trailing comment after then body
	then = p.wrapComments(then)

	expr := &IfExpr{At: tok.At, Cond: cond, Then: then}

	for p.peek().Kind == KWELIF {
		p.next()
		ec, err := p.parseQuery(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(KWTHEN); err != nil {
			return nil, err
		}
		et, err := p.parseQuery(0)
		if err != nil {
			return nil, err
		}
		et = p.wrapComments(et)
		expr.ElseIfs = append(expr.ElseIfs, ElseIfClause{Cond: ec, Then: et})
	}

	if p.peek().Kind == KWELSE {
		p.next()
		el, err := p.parseQuery(0)
		if err != nil {
			return nil, err
		}
		el = p.wrapComments(el)
		expr.Else = el
	}

	if _, err := p.expect(KWEND); err != nil {
		return nil, err
	}
	return expr, nil
}

func (p *parser) parseReduce() (*ReduceExpr, error) {
	tok, _ := p.expect(KWREDUCE)
	expr, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(KWAS); err != nil {
		return nil, err
	}
	pat, err := p.parsePattern()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(LPAREN); err != nil {
		return nil, err
	}
	init, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	init = p.wrapComments(init)
	if _, err := p.expect(SEMI); err != nil {
		return nil, err
	}
	update, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	update = p.wrapComments(update)
	if _, err := p.expect(RPAREN); err != nil {
		return nil, err
	}
	return &ReduceExpr{At: tok.At, Expr: expr, Pattern: pat, Init: init, Update: update}, nil
}

func (p *parser) parseForeach() (*ForeachExpr, error) {
	tok, _ := p.expect(KWFOREACH)
	expr, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(KWAS); err != nil {
		return nil, err
	}
	pat, err := p.parsePattern()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(LPAREN); err != nil {
		return nil, err
	}
	init, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	init = p.wrapComments(init)
	if _, err := p.expect(SEMI); err != nil {
		return nil, err
	}
	update, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	update = p.wrapComments(update)
	var extract Node
	if p.peek().Kind == SEMI {
		p.next()
		extract, err = p.parseQuery(0)
		if err != nil {
			return nil, err
		}
		extract = p.wrapComments(extract)
	}
	if _, err := p.expect(RPAREN); err != nil {
		return nil, err
	}
	return &ForeachExpr{At: tok.At, Expr: expr, Pattern: pat, Init: init, Update: update, Extract: extract}, nil
}

func (p *parser) parseTry() (*TryExpr, error) {
	tok, _ := p.expect(KWTRY)
	body, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	var handler Node
	if p.peek().Kind == KWCATCH {
		p.next()
		handler, err = p.parseTerm()
		if err != nil {
			return nil, err
		}
	}
	return &TryExpr{At: tok.At, Body: body, Handler: handler}, nil
}

func (p *parser) parseLabel() (*LabelExpr, error) {
	tok, _ := p.expect(KWLABEL)
	varTok, err := p.expect(VAR)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(PIPE); err != nil {
		return nil, err
	}
	body, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	return &LabelExpr{At: tok.At, Binding: varTok.Text, Body: body}, nil
}

func (p *parser) parseBreak() (*BreakExpr, error) {
	tok, _ := p.expect(KWBREAK)
	varTok, err := p.expect(VAR)
	if err != nil {
		return nil, err
	}
	return &BreakExpr{At: tok.At, Binding: varTok.Text}, nil
}

func (p *parser) parseParen() (*Paren, error) {
	tok, _ := p.expect(LPAREN)
	// Peek once so that a trailing comment on the ( line is absorbed into
	// pendingTrailing before we take it.  Without this, the comment isn't
	// absorbed until the first peek() inside parseQuery, which is too late.
	p.peek()
	openComment := p.takePendingTrailing()
	inner, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	// Drain any comments that appeared before the closing ) — they were
	// absorbed by the last peek() inside parseQuery but never attached.
	closingComments := p.drainComments()
	if _, err := p.expect(RPAREN); err != nil {
		return nil, err
	}
	return &Paren{At: tok.At, Expr: inner, OpenComment: openComment, ClosingComments: closingComments}, nil
}

func (p *parser) parseArray() (*Array, error) {
	tok, _ := p.expect(LBRACKET)
	if p.peek().Kind == RBRACKET {
		p.next()
		return &Array{At: tok.At}, nil
	}
	elem, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	elem = p.wrapComments(elem)
	if _, err := p.expect(RBRACKET); err != nil {
		return nil, err
	}
	return &Array{At: tok.At, Elem: elem}, nil
}

func (p *parser) parseObject() (*Object, error) {
	tok, _ := p.expect(LBRACE)
	obj := &Object{At: tok.At}
	lastFieldLine := tok.Line // line of the { token
	isFirst := true
	for {
		// Check the raw next token (before comment absorption) to detect blank
		// lines: if the first upcoming token (comment or key) is 2+ lines after
		// the last consumed token, there was an intentional blank line.
		rawNext := p.lex.Peek()
		blankBefore := rawNext.Line > lastFieldLine+1 && rawNext.Kind != EOF
		// Now absorb comments and check for closing brace.
		if p.peek().Kind == RBRACE {
			break
		}
		field, err := p.parseObjectField()
		if err != nil {
			return nil, err
		}
		field.BlankLineBefore = blankBefore
		// Set Object.MultiLine if the first field is on a different line than {.
		if isFirst {
			if rawNext.Line > tok.Line {
				obj.MultiLine = true
			}
			isFirst = false
		}
		obj.Fields = append(obj.Fields, field)
		if p.peek().Kind == COMMA {
			commaTok := p.next() // consume ","
			lastFieldLine = commaTok.Line
			// Peek once so that any trailing comment on the SAME LINE as the
			// comma is absorbed into pendingTrailing.  That comment belongs to
			// the PREVIOUS field's value — not the next field.
			p.peek()
			if tc := p.takePendingTrailing(); tc != nil {
				p.attachTrailingToField(field, tc)
			}
			lastFieldLine = p.lastLine
		} else {
			// No comma: peek so any trailing comment on the last field's line
			// is absorbed.  It belongs to the last field's value.
			p.peek()
			if tc := p.takePendingTrailing(); tc != nil {
				p.attachTrailingToField(field, tc)
			}
			lastFieldLine = p.lastLine
			break
		}
	}
	if _, err := p.expect(RBRACE); err != nil {
		return nil, err
	}
	return obj, nil
}

// attachTrailingToField sets tc as the trailing comment on field.Value,
// wrapping it in a CommentedExpr if necessary.
func (p *parser) attachTrailingToField(field *ObjectField, tc *Comment) {
	if field.Value == nil {
		return
	}
	if ce, ok := field.Value.(*CommentedExpr); ok {
		if ce.TrailingComment == nil {
			ce.TrailingComment = tc
		}
	} else {
		field.Value = &CommentedExpr{
			At:              field.Value.nodePos(),
			Expr:            field.Value,
			TrailingComment: tc,
		}
	}
}

func (p *parser) parseObjectField() (*ObjectField, error) {
	// Drain any leading comments that appeared before the field key.
	// Without this, they would end up absorbed by parseTerm during value
	// parsing and get misplaced on the value instead of the field.
	leading := p.drainComments()

	// BlankAfterComments: blank line between last leading comment and the key.
	// p.peek() here returns the key token (no comments left to absorb since
	// parseObject's p.peek() already drained them into leading above).
	blankAfterComments := false
	if len(leading) > 0 {
		lastCmt := leading[len(leading)-1]
		blankAfterComments = p.peek().Line > lastCmt.Line+1
	}

	at := p.peek().At
	var key Node

	switch p.peek().Kind {
	case STR:
		tok := p.next()
		key = &StrLit{At: tok.At, Raw: tok.Text}
	case LPAREN:
		paren, err := p.parseParen()
		if err != nil {
			return nil, err
		}
		key = paren
	case IDENT, KWIF, KWTHEN, KWELSE, KWEND, KWAS, KWDEF, KWREDUCE,
		KWFOREACH, KWTRY, KWCATCH, KWLABEL, KWBREAK, KWAND, KWOR:
		tok := p.next()
		key = &Ident{At: tok.At, Name: tok.Text}
	case VAR:
		// {$foo} shorthand for {foo: $foo}
		tok := p.next()
		key = &Var{At: tok.At, Name: tok.Text}
	default:
		return nil, p.errorf("expected object key, got %s (%q)", p.peek().Kind, p.peek().Text)
	}

	keyOpt := p.peek().Kind == QUEST
	if keyOpt {
		p.next()
	}

	if p.peek().Kind != COLON {
		return &ObjectField{At: at, LeadingComments: leading, BlankAfterComments: blankAfterComments, Key: key, KeyOptional: keyOpt}, nil
	}
	p.next()

	val, err := p.parseQuery(precAlt)
	if err != nil {
		return nil, err
	}
	val = p.wrapComments(val)
	return &ObjectField{At: at, LeadingComments: leading, BlankAfterComments: blankAfterComments, Key: key, KeyOptional: keyOpt, Value: val}, nil
}

// ── patterns ─────────────────────────────────────────────────────────────────

func (p *parser) parsePattern() (Node, error) {
	switch p.peek().Kind {
	case VAR:
		tok := p.next()
		return &Var{At: tok.At, Name: tok.Text}, nil
	case LBRACKET:
		return p.parseArrayPattern()
	case LBRACE:
		return p.parseObjectPattern()
	default:
		return nil, p.errorf("expected pattern, got %s", p.peek().Kind)
	}
}

func (p *parser) parseArrayPattern() (*ArrayPattern, error) {
	tok, _ := p.expect(LBRACKET)
	ap := &ArrayPattern{At: tok.At}
	for p.peek().Kind != RBRACKET {
		elem, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		ap.Elems = append(ap.Elems, elem)
		if p.peek().Kind == COMMA {
			p.next()
		} else {
			break
		}
	}
	if _, err := p.expect(RBRACKET); err != nil {
		return nil, err
	}
	return ap, nil
}

func (p *parser) parseObjectPattern() (*ObjectPattern, error) {
	tok, _ := p.expect(LBRACE)
	op := &ObjectPattern{At: tok.At}
	for p.peek().Kind != RBRACE {
		keyTok := p.next()
		if keyTok.Kind != IDENT {
			return nil, p.errorf("expected identifier in object pattern, got %s", keyTok.Kind)
		}
		field := &ObjPatField{Key: keyTok.Text}
		if p.peek().Kind == COLON {
			p.next()
			varTok, err := p.expect(VAR)
			if err != nil {
				return nil, err
			}
			field.Binding = varTok.Text
		}
		op.Fields = append(op.Fields, field)
		if p.peek().Kind == COMMA {
			p.next()
		} else {
			break
		}
	}
	if _, err := p.expect(RBRACE); err != nil {
		return nil, err
	}
	return op, nil
}

// isIdent is used by the formatter to check if a name needs quoting.
func isIdent(s string) bool {
	if len(s) == 0 {
		return false
	}
	if !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentContinue(s[i]) {
			return false
		}
	}
	return true
}

var _ = strings.TrimPrefix // used indirectly

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
	lex     *Lexer
	src     string
	pending []*Comment
}

// next returns the next non-comment token, collecting comments into p.pending.
func (p *parser) next() Token {
	for {
		t := p.lex.Next()
		if t.Kind == COMMENT {
			p.pending = append(p.pending, &Comment{At: t.At, Text: t.Text})
			continue
		}
		return t
	}
}

// peek returns the next non-comment token without consuming it.
func (p *parser) peek() Token {
	for {
		t := p.lex.Peek()
		if t.Kind == COMMENT {
			p.lex.Next()
			p.pending = append(p.pending, &Comment{At: t.At, Text: t.Text})
			continue
		}
		return t
	}
}

func (p *parser) drainComments() []*Comment {
	c := p.pending
	p.pending = nil
	return c
}

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
		imp, err := p.parseImportStmt()
		if err != nil {
			return nil, err
		}
		f.Imports = append(f.Imports, imp)
	}

	for p.peek().Kind != EOF {
		leading := p.drainComments()
		if p.peek().Kind == KWDEF {
			fd, err := p.parseFuncDef()
			if err != nil {
				return nil, err
			}
			fd.LeadingComments = leading
			f.FuncDefs = append(f.FuncDefs, fd)
		} else {
			q, err := p.parseQuery(0)
			if err != nil {
				return nil, err
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

func (p *parser) parseExpr(minPrec int) (Node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}

	for {
		opText, prec, rightPrec, ok := p.infixOp()
		if !ok || prec < minPrec {
			break
		}
		opTok := p.next()
		right, err := p.parseExpr(rightPrec)
		if err != nil {
			return nil, err
		}
		// Use dedicated Pipe / Comma nodes so the formatter can handle them
		// differently from arithmetic/logical binary operators.
		switch opText {
		case "|":
			left = &Pipe{At: opTok.At, Left: left, Right: right}
		case ",":
			left = &Comma{At: opTok.At, Left: left, Right: right}
		default:
			left = &BinOp{At: opTok.At, Op: opText, Left: left, Right: right}
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

func (p *parser) parseTerm() (Node, error) {
	primary, err := p.parsePrimary()
	if err != nil {
		return nil, err
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
				// expr."string" is syntactic sugar for expr["string"]
				strTok := p.next()
				key := &StrLit{At: strTok.At, Raw: strTok.Text}
				left = &Index{At: tok.At, Expr: left, Key: key}
			default:
				// bare . means identity
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
		// Local function definition: def name: body; REST
		// We parse this as a LocalFuncDef node so the formatter can render it
		// without inserting a pipe between the def and its scope.
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
			// ."string" is syntactic sugar for .["string"]
			strTok := p.next()
			key := &StrLit{At: strTok.At, Raw: strTok.Text}
			return &Index{At: tok.At, Key: key}, nil
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
		expr.ElseIfs = append(expr.ElseIfs, ElseIfClause{Cond: ec, Then: et})
	}

	if p.peek().Kind == KWELSE {
		p.next()
		el, err := p.parseQuery(0)
		if err != nil {
			return nil, err
		}
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
	if _, err := p.expect(SEMI); err != nil {
		return nil, err
	}
	update, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
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
	if _, err := p.expect(SEMI); err != nil {
		return nil, err
	}
	update, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	var extract Node
	if p.peek().Kind == SEMI {
		p.next()
		extract, err = p.parseQuery(0)
		if err != nil {
			return nil, err
		}
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
	inner, err := p.parseQuery(0)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(RPAREN); err != nil {
		return nil, err
	}
	return &Paren{At: tok.At, Expr: inner}, nil
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
	if _, err := p.expect(RBRACKET); err != nil {
		return nil, err
	}
	return &Array{At: tok.At, Elem: elem}, nil
}

func (p *parser) parseObject() (*Object, error) {
	tok, _ := p.expect(LBRACE)
	obj := &Object{At: tok.At}
	for p.peek().Kind != RBRACE {
		field, err := p.parseObjectField()
		if err != nil {
			return nil, err
		}
		obj.Fields = append(obj.Fields, field)
		if p.peek().Kind == COMMA {
			p.next()
		} else {
			break
		}
	}
	if _, err := p.expect(RBRACE); err != nil {
		return nil, err
	}
	return obj, nil
}

func (p *parser) parseObjectField() (*ObjectField, error) {
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
		// {$foo} is shorthand for {foo: $foo}
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
		return &ObjectField{At: at, Key: key, KeyOptional: keyOpt}, nil
	}
	p.next()

	val, err := p.parseQuery(precAlt)
	if err != nil {
		return nil, err
	}
	return &ObjectField{At: at, Key: key, KeyOptional: keyOpt, Value: val}, nil
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

// isIdent is used by the formatter to determine if an object key needs quoting.
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

// suppress unused import
var _ = strings.TrimPrefix

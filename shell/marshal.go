package shell

import (
	"bytes"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// MarshalFile converts a parsed shell File to a JSON-serialisable value.
// filename is embedded as "file" in the top-level object; use "-" for stdin.
func MarshalFile(f *syntax.File, filename string) any {
	return fileAST{
		Type:  "shell",
		File:  filename,
		Stmts: marshalStmts(f.Stmts),
	}
}

// ── JSON shapes ───────────────────────────────────────────────────────────────
//
// Struct-based for structural nodes (tree ordering); leaf words are rendered
// as text using the printer — this avoids re-implementing all of mvdan/sh's
// word-part types while still showing useful tree structure.

type fileAST struct {
	Type  string `json:"type"`
	File  string `json:"file"`
	Stmts []any  `json:"stmts"`
}

type stmtAST struct {
	Type       string `json:"type"`
	Cmd        any    `json:"cmd"`
	Negated    bool   `json:"negated,omitempty"`
	Background bool   `json:"background,omitempty"`
	Redirs     []any  `json:"redirs,omitempty"`
}

type callAST struct {
	Type    string   `json:"type"`
	Assigns []string `json:"assigns,omitempty"`
	Args    []string `json:"args"`
}

type binaryAST struct {
	Type string `json:"type"`
	Op   string `json:"op"`
	X    any    `json:"x"`
	Y    any    `json:"y"`
}

type ifAST struct {
	Type  string `json:"type"`
	Cond  []any  `json:"cond"`
	Then  []any  `json:"then"`
	Elifs []any  `json:"elifs,omitempty"`
	Else  []any  `json:"else,omitempty"`
}

type elifAST struct {
	Cond []any `json:"cond"`
	Then []any `json:"then"`
}

type whileAST struct {
	Type  string `json:"type"` // "while" or "until"
	Cond  []any  `json:"cond"`
	Do    []any  `json:"do"`
}

type forAST struct {
	Type string `json:"type"`
	Loop any    `json:"loop"`
	Do   []any  `json:"do"`
}

type wordIterAST struct {
	Type  string   `json:"type"`
	Name  string   `json:"name"`
	Items []string `json:"items,omitempty"`
}

type cStyleLoopAST struct {
	Type string `json:"type"`
	Init string `json:"init,omitempty"`
	Cond string `json:"cond,omitempty"`
	Post string `json:"post,omitempty"`
}

type caseAST struct {
	Type  string       `json:"type"`
	Word  string       `json:"word"`
	Items []caseItemAST `json:"items"`
}

type caseItemAST struct {
	Patterns []string `json:"patterns"`
	Stmts    []any    `json:"stmts"`
}

type funcDeclAST struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Body any    `json:"body"`
}

type subshellAST struct {
	Type  string `json:"type"`
	Stmts []any  `json:"stmts"`
}

type blockAST struct {
	Type  string `json:"type"`
	Stmts []any  `json:"stmts"`
}

type redirAST struct {
	Op   string `json:"op"`
	Word string `json:"word"`
	N    string `json:"n,omitempty"` // fd number if explicit
}

// ── helpers ───────────────────────────────────────────────────────────────────

func marshalStmts(stmts []*syntax.Stmt) []any {
	out := make([]any, len(stmts))
	for i, s := range stmts {
		out[i] = marshalStmt(s)
	}
	return out
}

func marshalStmt(s *syntax.Stmt) any {
	redirs := marshalRedirs(s.Redirs)
	cmd := marshalCommand(s.Cmd)
	// For simple non-negated non-background statements with no redirects,
	// omit the wrapping stmtAST for cleaner output.
	if !s.Negated && !s.Background && len(redirs) == 0 {
		return cmd
	}
	return stmtAST{
		Type:       "stmt",
		Cmd:        cmd,
		Negated:    s.Negated,
		Background: s.Background,
		Redirs:     redirs,
	}
}

func marshalCommand(cmd syntax.Command) any {
	if cmd == nil {
		return nil
	}
	switch v := cmd.(type) {
	case *syntax.CallExpr:
		assigns := make([]string, len(v.Assigns))
		for i, a := range v.Assigns {
			assigns[i] = nodeText(a)
		}
		args := make([]string, len(v.Args))
		for i, w := range v.Args {
			args[i] = nodeText(w)
		}
		return callAST{Type: "call", Assigns: assigns, Args: args}

	case *syntax.BinaryCmd:
		return binaryAST{
			Type: "binary",
			Op:   v.Op.String(),
			X:    marshalStmt(v.X),
			Y:    marshalStmt(v.Y),
		}

	case *syntax.IfClause:
		m := ifAST{
			Type: "if",
			Cond: marshalStmts(v.Cond),
			Then: marshalStmts(v.Then),
		}
		// Else is a chained *IfClause: elif branches have Cond set,
		// the else branch has empty Cond.
		for e := v.Else; e != nil; e = e.Else {
			if len(e.Cond) > 0 {
				m.Elifs = append(m.Elifs, elifAST{
					Cond: marshalStmts(e.Cond),
					Then: marshalStmts(e.Then),
				})
			} else {
				m.Else = marshalStmts(e.Then)
				break
			}
		}
		return m

	case *syntax.WhileClause:
		t := "while"
		if v.Until {
			t = "until"
		}
		return whileAST{Type: t, Cond: marshalStmts(v.Cond), Do: marshalStmts(v.Do)}

	case *syntax.ForClause:
		var loop any
		switch l := v.Loop.(type) {
		case *syntax.WordIter:
			items := make([]string, len(l.Items))
			for i, it := range l.Items {
				items[i] = nodeText(it)
			}
			loop = wordIterAST{Type: "wordIter", Name: l.Name.Value, Items: items}
		case *syntax.CStyleLoop:
			loop = cStyleLoopAST{
				Type: "cStyleLoop",
				Init: nodeText(l.Init),
				Cond: nodeText(l.Cond),
				Post: nodeText(l.Post),
			}
		}
		return forAST{Type: "for", Loop: loop, Do: marshalStmts(v.Do)}

	case *syntax.CaseClause:
		items := make([]caseItemAST, len(v.Items))
		for i, item := range v.Items {
			patterns := make([]string, len(item.Patterns))
			for j, p := range item.Patterns {
				patterns[j] = nodeText(p)
			}
			items[i] = caseItemAST{Patterns: patterns, Stmts: marshalStmts(item.Stmts)}
		}
		return caseAST{Type: "case", Word: nodeText(v.Word), Items: items}

	case *syntax.DeclClause:
		// local, declare, export, readonly, typeset, nameref
		assigns := make([]string, len(v.Args))
		for i, a := range v.Args {
			assigns[i] = nodeText(a)
		}
		return map[string]any{"type": v.Variant.Value, "args": assigns}

	case *syntax.FuncDecl:
		return funcDeclAST{Type: "funcDecl", Name: v.Name.Value, Body: marshalStmt(v.Body)}

	case *syntax.Subshell:
		return subshellAST{Type: "subshell", Stmts: marshalStmts(v.Stmts)}

	case *syntax.Block:
		return blockAST{Type: "block", Stmts: marshalStmts(v.Stmts)}

	// ── bash-specific command forms ───────────────────────────────────────────
	// These are rendered as text rather than deep AST to keep the marshal
	// surface small; the structure is still correct for tooling purposes.

	case *syntax.ArithmCmd:
		// (( expr )) — arithmetic command
		return map[string]any{"type": "arithmCmd", "expr": nodeText(v.X)}

	case *syntax.TestClause:
		// [[ expr ]] — bash conditional expression
		return map[string]any{"type": "testClause", "x": nodeText(v.X)}

	case *syntax.LetClause:
		// let expr [expr ...] — arithmetic let command
		exprs := make([]string, len(v.Exprs))
		for i, e := range v.Exprs {
			exprs[i] = nodeText(e)
		}
		return map[string]any{"type": "let", "exprs": exprs}

	case *syntax.TimeClause:
		// time [pipeline] — time a command
		m := map[string]any{"type": "time"}
		if v.Stmt != nil {
			m["stmt"] = marshalStmt(v.Stmt)
		}
		return m

	case *syntax.CoprocClause:
		// coproc [name] cmd — run command as coprocess
		m := map[string]any{"type": "coproc"}
		if v.Name != nil {
			m["name"] = nodeText(v.Name)
		}
		m["stmt"] = marshalStmt(v.Stmt)
		return m

	case *syntax.TestDecl:
		// bash test declaration (rare/internal)
		return map[string]any{"type": "testDecl"}
	}

	// No case matched — this means mvdan/sh added a new Command type that we
	// haven't handled.  Panic immediately so tests catch it; update this switch
	// when upgrading mvdan/sh.
	//
	// Known types (mvdan.cc/sh/v3 v3.13.1):
	//   CallExpr, IfClause, WhileClause, ForClause, CaseClause, Block,
	//   Subshell, BinaryCmd, FuncDecl, DeclClause,
	//   ArithmCmd, TestClause, LetClause, TimeClause, CoprocClause, TestDecl
	panic(fmt.Sprintf("shell/marshal: unhandled syntax.Command type %T — update marshalCommand when upgrading mvdan/sh", cmd))
}

func marshalRedirs(redirs []*syntax.Redirect) []any {
	if len(redirs) == 0 {
		return nil
	}
	out := make([]any, len(redirs))
	for i, r := range redirs {
		rd := redirAST{Op: r.Op.String(), Word: nodeText(r.Word)}
		if r.N != nil {
			rd.N = r.N.Value
		}
		out[i] = rd
	}
	return out
}

// nodeText renders a syntax.Node as compact text using the mvdan/sh printer.
func nodeText(n syntax.Node) string {
	if n == nil {
		return ""
	}
	var buf bytes.Buffer
	newPrinter().Print(&buf, n)
	return strings.TrimRight(buf.String(), "\n")
}

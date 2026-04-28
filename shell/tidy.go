package shell

import (
	"bytes"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ApplyTidy applies idiomatic shell rewrites to f in place:
//   - "|| true" → "|| :" (the POSIX null command is preferred over /bin/true)
func ApplyTidy(f *syntax.File) {
	syntax.Walk(f, func(n syntax.Node) bool {
		bc, ok := n.(*syntax.BinaryCmd)
		if !ok || bc.Op != syntax.OrStmt {
			return true
		}
		if isCallTo(bc.Y, "true") {
			setCallName(bc.Y, ":")
		}
		return true
	})
}

// isCallTo reports whether stmt is a simple no-assign call to name with no other args.
func isCallTo(stmt *syntax.Stmt, name string) bool {
	ce, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(ce.Args) != 1 || len(ce.Assigns) != 0 || len(stmt.Redirs) != 0 {
		return false
	}
	return wordLit(ce.Args[0]) == name
}

// setCallName replaces the command name literal in a single-command Stmt.
// Only safe to call after isCallTo confirms the shape.
func setCallName(stmt *syntax.Stmt, name string) {
	ce := stmt.Cmd.(*syntax.CallExpr)
	ce.Args[0].Parts[0].(*syntax.Lit).Value = name
}

// FlattenAndChain returns the ordered Stmts of a pure && tree.
// Returns nil if any non-&& binary operator appears at the root level,
// or if the tree contains redirects on the BinaryCmd itself.
func FlattenAndChain(stmt *syntax.Stmt) []*syntax.Stmt {
	if len(stmt.Redirs) != 0 {
		return nil
	}
	bc, ok := stmt.Cmd.(*syntax.BinaryCmd)
	if !ok {
		return []*syntax.Stmt{stmt}
	}
	if bc.Op != syntax.AndStmt {
		return nil
	}
	left := FlattenAndChain(bc.X)
	if left == nil {
		return nil
	}
	right := FlattenAndChain(bc.Y)
	if right == nil {
		return nil
	}
	return append(left, right...)
}

// FormatStmtOneLine formats stmt as a compact single-line string (no trailing newline).
func FormatStmtOneLine(stmt *syntax.Stmt) (string, error) {
	f := &syntax.File{Stmts: []*syntax.Stmt{stmt}}
	var buf bytes.Buffer
	if err := newPrinter().Print(&buf, f); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

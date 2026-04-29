package shell

import (
	"bytes"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// TidyShebang normalises the shebang line of src if present:
//   - "#!/bin/bash" → "#!/usr/bin/env bash"
//   - "#!/bin/sh"   → "#!/usr/bin/env bash"  (Tianon always targets bash)
//
// Returns src unchanged if no shebang is present or it already uses env.
func TidyShebang(src string) string {
	line, rest, hasRest := strings.Cut(src, "\n")
	switch strings.TrimSpace(line) {
	case "#!/bin/bash", "#!/bin/sh":
		if hasRest {
			return "#!/usr/bin/env bash\n" + rest
		}
		return "#!/usr/bin/env bash"
	}
	return src
}

// NormalizeSetFlags applies text-level normalization of set flag combinations.
// It operates on source text (before parsing) so the parser assigns correct
// positions — the same approach used by TidyShebang.
//
// For each line that looks like a set command:
//   - POSIX/sh:  set -eu  (or set -eux if -x was already present)
//   - bash/mksh: set -Eeuo pipefail  (or set -Eeuxo pipefail with -x)
//
// Only single-argument flag forms are rewritten; "set --", "set -o",
// and already-canonical forms are left unchanged.
func NormalizeSetFlags(src string, lang syntax.LangVariant) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		lines[i] = normalizeSetLine(line, lang)
	}
	return strings.Join(lines, "\n")
}

func normalizeSetLine(line string, lang syntax.LangVariant) string {
	trimmed := strings.TrimLeft(line, "\t ")
	if !strings.HasPrefix(trimmed, "set ") {
		return line
	}
	parts := strings.Fields(trimmed)
	if len(parts) < 2 || !strings.HasPrefix(parts[1], "-") || parts[1] == "--" || parts[1] == "-o" {
		return line // not simple flags
	}
	flags := parts[1]
	indent := line[:len(line)-len(strings.TrimLeft(line, "\t "))]

	// Determine if -x should be preserved: only at depth > 0 (inside a function/block).
	// At top level (no leading indent), -x is globally Wrong, so strip it.
	isTopLevel := indent == ""
	hasX := strings.Contains(flags, "x") && !isTopLevel

	switch lang {
	case syntax.LangPOSIX, syntax.LangMirBSDKorn:
		canonical := "-eu"
		if hasX {
			canonical = "-eux"
		}
		if flags == canonical {
			return line
		}
		return indent + "set " + canonical
	default: // bash
		canonical := "-Eeuo"
		if hasX {
			canonical = "-Eeuxo"
		}
		if flags == canonical && len(parts) >= 3 && parts[2] == "pipefail" {
			return line
		}
		return indent + "set " + canonical + " pipefail"
	}
}

// ApplyTidy applies idiomatic shell rewrites to f in place:
//   - "|| true" → "|| :"
//   - "which cmd" → "command -v cmd"
//   - backtick `cmd` → $(cmd)
//   - "function name" → "name()" (removes the non-POSIX function keyword)
//   - `[ ... == ... ]` → `[ ... = ... ]` (POSIX string comparison in test brackets)
func ApplyTidy(f *syntax.File) {
	syntax.Walk(f, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.CmdSubst:
			v.Backquotes = false // backtick → $()

		case *syntax.FuncDecl:
			v.RsrvWord = false // "function name" → "name()"

		case *syntax.BinaryCmd:
			if v.Op == syntax.OrStmt && isCallTo(v.Y, "true") {
				setCallName(v.Y, ":")
			}

		case *syntax.CallExpr:
			if len(v.Args) == 0 {
				return true
			}
			cmd := wordLit(v.Args[0])

			// `[ ... == ... ]` → `[ ... = ... ]`
			// Must be `[` (POSIX test), not `[[` (bash test — handled as TestClause).
			if cmd == "[" {
				for _, arg := range v.Args {
					if wordLit(arg) == "==" {
						arg.Parts[0].(*syntax.Lit).Value = "="
					}
				}
			}

			// which cmd → command -v cmd (flag-free only)
			if cmd == "which" {
				hasFlags := false
				for _, arg := range v.Args[1:] {
					if strings.HasPrefix(wordLit(arg), "-") {
						hasFlags = true
						break
					}
				}
				if !hasFlags && len(v.Args) >= 2 {
					v.Args[0].Parts[0].(*syntax.Lit).Value = "command"
					flag := &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: "-v"}}}
					v.Args = append(v.Args[:1], append([]*syntax.Word{flag}, v.Args[1:]...)...)
				}
			}
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

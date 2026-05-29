package shell

import (
	"bytes"
	"sort"
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
	if len(parts) < 2 || !strings.HasPrefix(parts[1], "-") || parts[1] == "--" {
		return line
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, "\t "))]

	// Only rewrite set commands that touch the managed error-handling flags.
	// A bare "set -x" / "set +x" is an xtrace toggle — leave it alone.
	if !hasManagedFlag(parts) {
		return line
	}

	isTopLevel := indent == ""

	switch lang {
	case syntax.LangPOSIX, syntax.LangMirBSDKorn:
		// POSIX canonical: "set -eu" or "set -eux". Only e, u, x are recognised;
		// bash-specific flags (E, o pipefail, …) are stripped.
		hasX := false
		scanSetArgs(parts,
			func(c byte) { hasX = hasX || c == 'x' },
			func(opt string) { hasX = hasX || opt == "xtrace" },
		)
		canonical := "set -eu"
		if hasX {
			canonical = "set -eux"
		}
		if trimmed == canonical {
			return line
		}
		return indent + canonical
	default: // bash
		extras := collectSetExtras(parts)
		canonical := buildBashSet(extras, isTopLevel)
		if trimmed == canonical {
			return line
		}
		return indent + canonical
	}
}

// setExtras holds the non-core flags extracted from a bash "set ..." command.
type setExtras struct {
	chars []byte   // extra single-char flags, sorted alphabetically, combined as -<abc>
	opts  []string // extra -o <option> names with no idiomatic short form, sorted alphabetically
}

// optToChar maps -o option names to their single-char equivalents.
// Core options (errtrace/errexit/nounset) are included so they collapse into
// the canonical -Eeuo cluster rather than being emitted as -o <name>.
// "pipefail" has no single-char form and is handled separately (it is the -o
// argument that completes the canonical core; it is never an extra).
// Unusual options with obscure char forms (e.g. "noglob" → "f") are omitted
// intentionally — they stay as -o <name> for readability.
var optToChar = map[string]byte{
	"allexport": 'a',
	"errtrace":  'E',
	"errexit":   'e',
	"nounset":   'u',
	"xtrace":    'x',
}

// scanSetArgs walks all arguments of a "set" command (parts[1:]), calling
// onChar for each single-char flag and onOpt for each -o <option> value.
// Handles standalone "-o <name>", embedded "-o<name>" in a cluster, and
// regular "-<chars>" clusters uniformly.
func scanSetArgs(parts []string, onChar func(byte), onOpt func(string)) {
	i := 1
	for i < len(parts) {
		p := parts[i]
		i++
		if !strings.HasPrefix(p, "-") || p == "--" {
			continue
		}
		for j := 1; j < len(p); j++ {
			c := p[j]
			if c != 'o' {
				onChar(c)
				continue
			}
			// -o takes the next token as its option name: either the rest of
			// the cluster ("-onoglob") or the next separate arg ("-o noglob").
			rest := p[j+1:]
			if rest != "" {
				onOpt(rest)
				j = len(p) // rest consumed; exit inner loop
			} else if i < len(parts) {
				onOpt(parts[i])
				i++
			}
		}
	}
}

// hasManagedFlag reports whether a "set" command's args include any managed
// error-handling flag (e, u, E, or their -o equivalents, or pipefail).
// Used to distinguish "set -x" xtrace toggles from error-handling set commands.
func hasManagedFlag(parts []string) bool {
	managed := false
	scanSetArgs(parts,
		func(c byte) { managed = managed || strings.ContainsRune("euE", rune(c)) },
		func(opt string) {
			if opt == "pipefail" {
				managed = true
				return
			}
			if c, ok := optToChar[opt]; ok {
				managed = managed || strings.ContainsRune("euE", rune(c))
			}
		},
	)
	return managed
}

// collectSetExtras extracts non-core flags from a bash "set ..." command via
// scanSetArgs. Core is {E, e, u, o, pipefail}. Well-known -o options are
// collapsed to their char form; unusual ones stay as -o <name>.
func collectSetExtras(parts []string) setExtras {
	const core = "Eeuo"
	charSeen := map[byte]bool{}
	optSeen := map[string]bool{}
	var chars []byte
	var opts []string

	scanSetArgs(parts,
		func(c byte) {
			if !strings.ContainsRune(core, rune(c)) && !charSeen[c] {
				charSeen[c] = true
				chars = append(chars, c)
			}
		},
		func(opt string) {
			if opt == "pipefail" {
				return // core; always emitted by buildBashSet
			}
			if c, ok := optToChar[opt]; ok {
				if !charSeen[c] {
					charSeen[c] = true
					// Core chars are always emitted by buildBashSet; don't repeat them.
					if !strings.ContainsRune(core, rune(c)) {
						chars = append(chars, c)
					}
				}
			} else if !optSeen[opt] {
				optSeen[opt] = true
				opts = append(opts, opt)
			}
		},
	)

	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	sort.Strings(opts)
	return setExtras{chars: chars, opts: opts}
}

// buildBashSet constructs the canonical "set ..." string for bash.
// At top level: -Eeuo pipefail first, then char extras as -<abc>, then -o <name> extras.
// Non-top-level: char extras embedded in the cluster as -Eeu<abc>o pipefail, then -o <name> extras.
func buildBashSet(extras setExtras, isTopLevel bool) string {
	var b strings.Builder
	b.WriteString("set ")
	if isTopLevel {
		b.WriteString("-Eeuo pipefail")
		if len(extras.chars) > 0 {
			b.WriteString(" -")
			b.Write(extras.chars)
		}
	} else {
		b.WriteString("-Eeu")
		b.Write(extras.chars)
		b.WriteString("o pipefail")
	}
	for _, opt := range extras.opts {
		b.WriteString(" -o ")
		b.WriteString(opt)
	}
	return b.String()
}

// ApplyTidy applies idiomatic shell rewrites to f in place:
//   - "|| true" → "|| :"
//   - "which cmd" → "command -v cmd"
//   - backtick `cmd` → $(cmd)
//   - "function name" → "name()" (removes the non-POSIX function keyword)
//   - `[ ... == ... ]` → `[ ... = ... ]` (POSIX string comparison in test brackets)
//   - "${FOO}" → "$FOO" and bare ${FOO} → $FOO when the variable is the sole content
func ApplyTidy(f *syntax.File) {
	protected := collectProtectedLines(f)
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

		case *syntax.DblQuoted:
			// "${FOO}" → "$FOO": strip braces when the variable is the sole
			// content of the double-quoted string and has no special operators.
			if len(v.Parts) == 1 {
				if pe, ok := v.Parts[0].(*syntax.ParamExp); ok && isSimpleParamExp(pe) {
					pe.Short = true
				}
			}

		case *syntax.Word:
			// ${FOO} (unquoted, whole word) → $FOO: same rule as DblQuoted.
			if len(v.Parts) == 1 {
				if pe, ok := v.Parts[0].(*syntax.ParamExp); ok && isSimpleParamExp(pe) {
					pe.Short = true
				}
			}

		case *syntax.CallExpr:
			if !protected[v.Pos().Line()] {
				normalizeShortFlags(v, false, true)
			}
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

// isSimpleParamExp reports whether pe is a plain variable reference with no
// special operators — the only kind where braces are purely cosmetic.
// Equivalent to: every field except Dollar, Rbrace, and Param is its zero
// value.  We cannot use struct equality directly because NestedParam and Index
// are interfaces (comparing interface values with == is undefined for
// non-comparable concrete types), so we enumerate the fields explicitly.
func isSimpleParamExp(pe *syntax.ParamExp) bool {
	return !pe.Short &&
		!pe.Excl &&
		!pe.Length &&
		!pe.Width &&
		!pe.IsSet &&
		pe.Flags == nil &&
		pe.NestedParam == nil &&
		pe.Index == nil &&
		pe.Modifiers == nil &&
		pe.Slice == nil &&
		pe.Repl == nil &&
		pe.Names == 0 &&
		pe.Exp == nil
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

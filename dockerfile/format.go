// Package dockerfile provides formatting for Dockerfile source files.
//
// Style rules (backed by corpus samples):
//
//   - Instruction keywords are uppercased (all Dockerfiles in corpus)
//   - Continuation lines preserve original leading-tab depth
//     (https://github.com/debuerreotype/debuerreotype/blob/3c3272fa743e0257ae64081987c500c2923ea963/Dockerfile#L17 — 2 tabs for apt-get arguments)
//   - Inline comments within continuation blocks sit at column 0
//     (https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile.template#L7)
//   - RUN shell content is normalised for tab depth using the shell.FormatRUN
//     function (depth-based tab normalisation without restructuring)
//   - A single blank line separates instruction groups
//   - No trailing whitespace; file ends with a single newline
package dockerfile

import (
	"strings"
)

// Formatter holds optional callbacks for embedded-language formatting.
type Formatter struct {
	// JQFmt, if set, is called to reformat jq expressions found in
	// jq '...' invocations within RUN blocks.
	JQFmt func(expr string, inline bool) string

	// RUNShellFmt, if set, is called with the continuation lines of each RUN
	// instruction to normalise their shell formatting.  It receives a slice of
	// raw lines (with ` \` continuation markers) and returns a replacement slice.
	RUNShellFmt func(lines []string, jqFmt func(expr string, inline bool) string) []string
}

// Format formats a parsed Dockerfile back to canonical source using the
// default formatter (no embedded-language rewriting).
func Format(f *File) string {
	return (&Formatter{}).FormatFile(f)
}

// FormatWith formats with the provided embedded-language formatters.
func FormatWith(f *File, fmt *Formatter) string {
	return fmt.FormatFile(f)
}

// FormatFile is the method form of Format.
func (fmtr *Formatter) FormatFile(f *File) string {
	w := &writer{fmtr: fmtr}
	w.file(f)
	return w.out.String()
}

type writer struct {
	out  strings.Builder
	fmtr *Formatter
}

func (w *writer) writeln(s string) { w.out.WriteString(s); w.out.WriteByte('\n') }
func (w *writer) newline()         { w.out.WriteByte('\n') }

func (w *writer) file(f *File) {
	for _, d := range f.Directives {
		w.writeln(d.Raw)
	}
	if len(f.Directives) > 0 && len(f.Instructions) > 0 {
		w.newline()
	}

	prevWasBlank := true
	for _, instr := range f.Instructions {
		switch instr.Keyword {
		case "":
			if !prevWasBlank {
				w.newline()
				prevWasBlank = true
			}
		case "COMMENT":
			w.writeln(strings.TrimRight(instr.Args, " \t"))
			prevWasBlank = false
		default:
			w.instruction(instr)
			prevWasBlank = false
		}
	}
}

func (w *writer) instruction(instr *Instruction) {
	if len(instr.Lines) == 0 {
		return
	}

	// For RUN instructions, optionally normalise the continuation-line shell.
	if instr.Keyword == "RUN" && w.fmtr != nil && w.fmtr.RUNShellFmt != nil {
		w.runInstruction(instr)
		return
	}

	w.plainInstruction(instr)
}

// plainInstruction emits a Dockerfile instruction preserving original
// continuation-line indentation.
func (w *writer) plainInstruction(instr *Instruction) {
	for _, line := range instr.Lines {
		switch line.Kind {
		case LineKindInstruction:
			first := formatFirstLine(line.Text, instr.Keyword)
			// Normalise single-line ENV "KEY=VALUE" → "KEY VALUE" (Dockerfile-native form).
			if instr.Keyword == "ENV" && len(instr.Lines) == 1 {
				first = "ENV " + normalizeEnvArgs(instr.Args)
			}
			w.writeln(first)

		case LineKindContinuation:
			stripped := strings.TrimRight(line.Text, " \t")
			if stripped == "" {
				w.writeln("\\")
			} else {
				tabs := countLeadingTabs(line.Text)
				rest := strings.TrimLeft(line.Text, " \t")
				w.writeln(strings.Repeat("\t", tabs) + rest)
			}

		case LineKindComment:
			// Inline comments within continuation blocks: column 0.
			// Style ref: https://github.com/tianon/dockerfiles/blob/2118a1979eff7545e06570d1eefc6434d691e68d/steam/Dockerfile.template#L7
			w.writeln(strings.TrimLeft(line.Text, " \t"))
		}
	}
}

// runInstruction emits a RUN instruction, applying shell normalisation to the
// continuation lines.
func (w *writer) runInstruction(instr *Instruction) {
	if len(instr.Lines) == 0 {
		return
	}

	// Emit the first line (RUN ...) unchanged.
	firstLine := instr.Lines[0]
	w.writeln(formatFirstLine(firstLine.Text, instr.Keyword))

	if len(instr.Lines) == 1 {
		return // single-line RUN
	}

	// Collect continuation + comment lines and pass to the shell formatter.
	var contLines []string
	for _, line := range instr.Lines[1:] {
		contLines = append(contLines, line.Text)
	}

	normalised := w.fmtr.RUNShellFmt(contLines, w.fmtr.JQFmt)
	for _, line := range normalised {
		w.writeln(line)
	}
}

// formatFirstLine uppercases the keyword while preserving the rest of the line.
func formatFirstLine(raw, keyword string) string {
	trimmed := strings.TrimSpace(raw)
	idx := strings.IndexAny(trimmed, " \t\\")
	if idx < 0 {
		return keyword
	}
	return keyword + trimmed[idx:]
}

// normalizeEnvArgs converts shell-assignment form ENV args ("KEY=VALUE") to
// Dockerfile-native form ("KEY VALUE") for single-value ENV instructions.
// Multi-pair args (containing spaces before a second "=") are left unchanged,
// as are empty values.
// Per dockerfile.md: the Dockerfile-native space-separated form is preferred.
func normalizeEnvArgs(args string) string {
	args = strings.TrimSpace(args)
	eqIdx := strings.IndexByte(args, '=')
	if eqIdx <= 0 {
		return args // already "KEY VALUE" form or no =
	}
	key := args[:eqIdx]
	// Key must look like a valid env var name with no whitespace.
	if strings.ContainsAny(key, " \t") {
		return args
	}
	value := args[eqIdx+1:]
	if value == "" {
		return args // don't normalise empty-value form
	}
	// Strip a single layer of surrounding quotes from the value.
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key + " " + value
}

// countLeadingTabs counts leading tab characters in s.
func countLeadingTabs(s string) int {
	n := 0
	for _, ch := range s {
		if ch == '\t' {
			n++
		} else {
			break
		}
	}
	return n
}

// Package dockerfile provides formatting for Dockerfile source files.
//
// Style rules (backed by corpus samples):
//
//   - Instruction keywords are uppercased (corpus: all Dockerfiles use UPPERCASE)
//   - Continuation lines preserve their original leading-tab indentation
//     (corpus/debuerreotype/Dockerfile:17 "apt-get install" args have 2 tabs;
//     corpus/tianon-dockerfiles/steam/Dockerfile.template:6 "ca-certificates" 2 tabs)
//   - Inline comments within continuation blocks have NO indentation — they sit
//     at column 0 (corpus/tianon-dockerfiles/steam/Dockerfile.template:7
//     "# zenity is used during early startup…"; debuerreotype/Dockerfile:23)
//   - A single blank line separates instruction groups
//   - No trailing whitespace on any line
//   - File ends with a single newline
package dockerfile

import (
	"strings"
)

// Format formats a parsed Dockerfile back to canonical source.
func Format(f *File) string {
	w := &writer{}
	w.file(f)
	return w.out.String()
}

type writer struct {
	out strings.Builder
}

func (w *writer) write(s string)   { w.out.WriteString(s) }
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
	for _, line := range instr.Lines {
		switch line.Kind {
		case LineKindInstruction:
			w.writeln(formatFirstLine(line.Text, instr.Keyword))

		case LineKindContinuation:
			// Preserve the original leading-tab indentation exactly.
			// Only strip trailing whitespace.
			// A blank continuation line (lone \ used as a visual separator) is
			// preserved as a single \ with no extra indentation.
			stripped := strings.TrimRight(line.Text, " \t")
			if stripped == "" {
				// The original line was blank after stripping the escape; emit \
				// to preserve the visual separator.
				w.writeln("\\")
			} else {
				// Re-emit with original leading tabs, normalised from any mix of
				// spaces: count leading tabs in the original and preserve them.
				tabs := countLeadingTabs(line.Text)
				rest := strings.TrimLeft(line.Text, " \t")
				w.writeln(strings.Repeat("\t", tabs) + rest)
			}

		case LineKindComment:
			// Inline comments within a continuation block sit at column 0 — no
			// leading indentation.
			// Style ref: corpus/tianon-dockerfiles/steam/Dockerfile.template:7
			// "# zenity is used during early startup for dialogs and progress bars"
			// corpus/debuerreotype/Dockerfile:23
			// "# add "gpgv" explicitly (for now) since it's transitively-essential…"
			w.writeln(strings.TrimLeft(line.Text, " \t"))
		}
	}
}

// formatFirstLine uppercases the keyword of the first instruction line while
// preserving the rest of the line verbatim.
func formatFirstLine(raw, keyword string) string {
	trimmed := strings.TrimSpace(raw)
	idx := strings.IndexAny(trimmed, " \t\\")
	if idx < 0 {
		return keyword
	}
	return keyword + trimmed[idx:]
}

// countLeadingTabs counts leading tab characters (not spaces) in s.
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

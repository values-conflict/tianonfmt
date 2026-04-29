package dockerfile

import (
	"encoding/json"
	"strings"
)

// TidyFile applies tidy rewrites to a parsed Dockerfile in place.
//
// tidyRUN, if non-nil, is called with the logical shell content of each RUN
// instruction (its Args field).  It returns the ordered list of commands to
// emit as a semicolon-separated set-eux block, or nil if no transformation
// applies (e.g. already in semicolon form, or not a pure && chain).
//
// normSetFlags, if non-nil, is called with a "set ..." command string and
// returns the normalised form (e.g. "set -Eeuo pipefail" → "set -eux").
// It is applied to the first command of every RUN instruction, even those
// already in semicolon form that tidyRUN did not restructure.
//
// Additional per-instruction transforms can be added here as the need arises,
// keeping tidy rewrites separate from the Formatter's presentation concerns.
func TidyFile(f *File, tidyRUN func(args string) []string, normSetFlags func(string) string) {
	for _, instr := range f.Instructions {
		if instr.Keyword != "RUN" {
			continue
		}
		if tidyRUN != nil {
			if cmds := tidyRUN(instr.Args); cmds != nil {
				applyRUNCommands(instr, cmds)
				continue
			}
		}
		// Even when tidyRUN didn't restructure, normalise the set flags on
		// the first physical line (handles already-semicolon-form blocks).
		if normSetFlags != nil {
			normRUNFirstLine(instr, normSetFlags)
		}
	}
}

// normRUNFirstLine applies normSetFlags to the first command of a RUN
// instruction by rewriting both Lines[0].Text and Args in place.
func normRUNFirstLine(instr *Instruction, normSetFlags func(string) string) {
	if len(instr.Lines) == 0 {
		return
	}
	// Extract the first command from Args (before the first ";").
	firstCmd, rest, hasSemi := strings.Cut(instr.Args, ";")
	firstCmd = strings.TrimSpace(firstCmd)
	if !strings.HasPrefix(firstCmd, "set -") {
		return
	}
	normed := normSetFlags(firstCmd)
	if normed == firstCmd {
		return
	}
	// Rewrite Args.
	if hasSemi {
		instr.Args = normed + ";" + rest
	} else {
		instr.Args = normed
	}
	// Rewrite Lines[0].Text: replace the first word sequence up to the first
	// ";" or end-of-text after "RUN ".
	line := instr.Lines[0].Text
	after, hasCont := strings.CutSuffix(line, " \\")
	before, lineRest, lineHasSemi := strings.Cut(after, ";")
	// before should be "RUN set -Eeuo pipefail" (or similar)
	kw, oldCmd, _ := strings.Cut(strings.TrimSpace(before), " ")
	_ = oldCmd
	var newLine string
	if lineHasSemi {
		newLine = kw + " " + normed + ";" + lineRest
	} else {
		newLine = kw + " " + normed
	}
	if hasCont {
		newLine += " \\"
	}
	instr.Lines[0].Text = newLine
}

// shellSpecialChars is the set of characters that indicate shell features.
// Any command containing one of these cannot be safely converted to exec form
// by simple whitespace splitting — leave it for pedantic's explicit wrapping.
const shellSpecialChars = "$&|;<>(){}[]`*?'\"\\"

// hasShellFeatures reports whether cmd uses shell features that prevent safe
// auto-conversion to exec form (variable expansion, pipes, globs, quoting, …).
func hasShellFeatures(cmd string) bool {
	return strings.ContainsAny(cmd, shellSpecialChars)
}

// buildExecLine constructs a `CMD ["a","b"]` style instruction text.
func buildExecLine(kw string, args []string) (text, jsonArgs string) {
	b, _ := json.Marshal(args)
	jsonArgs = string(b)
	return kw + " " + jsonArgs, jsonArgs
}

// TidyCmdEntrypoint converts shell-form CMD/ENTRYPOINT to exec form when the
// command contains no shell features (no $, |, ;, &, etc.).  Safe to auto-fix
// because simple whitespace-split produces a semantically equivalent exec form.
func TidyCmdEntrypoint(f *File) {
	for _, instr := range f.Instructions {
		if instr.Keyword != "CMD" && instr.Keyword != "ENTRYPOINT" {
			continue
		}
		args := strings.TrimSpace(instr.Args)
		if strings.HasPrefix(args, "[") {
			continue // already exec form
		}
		if hasShellFeatures(args) {
			continue // has shell features — leave for pedantic
		}
		tokens := strings.Fields(args)
		if len(tokens) == 0 {
			continue
		}
		text, _ := buildExecLine(instr.Keyword, tokens)
		instr.Args = string(mustMarshal(tokens))
		if len(instr.Lines) > 0 {
			instr.Lines[0].Text = text
		}
	}
}

// PedanticCmdEntrypoint wraps any remaining shell-form CMD/ENTRYPOINT in an
// explicit /bin/sh -c invocation — the --pedantic fallback for commands with
// shell features that cannot be safely split.
//
// CMD:        CMD ["/bin/sh", "-c", "shell command"]
// ENTRYPOINT: ENTRYPOINT ["/bin/sh", "-c", "shell command", "--"]
//
// The trailing "--" on ENTRYPOINT lets CMD arguments pass through as positional
// parameters to the shell command rather than as flags to /bin/sh.
func PedanticCmdEntrypoint(f *File) {
	for _, instr := range f.Instructions {
		if instr.Keyword != "CMD" && instr.Keyword != "ENTRYPOINT" {
			continue
		}
		args := strings.TrimSpace(instr.Args)
		if strings.HasPrefix(args, "[") {
			continue // already exec form
		}
		var wrapped []string
		if instr.Keyword == "ENTRYPOINT" {
			wrapped = []string{"/bin/sh", "-c", args, "--"}
		} else {
			wrapped = []string{"/bin/sh", "-c", args}
		}
		text, _ := buildExecLine(instr.Keyword, wrapped)
		instr.Args = string(mustMarshal(wrapped))
		if len(instr.Lines) > 0 {
			instr.Lines[0].Text = text
		}
	}
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// applyRUNCommands rewrites instr to emit cmds as a semicolon-separated block.
// The first command goes on the RUN line; subsequent commands become
// tab-indented continuation lines.
func applyRUNCommands(instr *Instruction, cmds []string) {
	instr.Args = strings.Join(cmds, "; ")
	lines := make([]Line, 0, len(cmds))
	lines = append(lines, Line{
		Kind: LineKindInstruction,
		Text: "RUN " + cmds[0] + "; \\",
	})
	for i, cmd := range cmds[1:] {
		text := "\t" + cmd
		if i < len(cmds)-2 {
			text += "; \\"
		}
		lines = append(lines, Line{Kind: LineKindContinuation, Text: text})
	}
	instr.Lines = lines
}

package dockerfile

import "strings"

// TidyFile applies tidy rewrites to a parsed Dockerfile in place.
//
// tidyRUN, if non-nil, is called with the logical shell content of each RUN
// instruction (its Args field).  It returns the ordered list of commands to
// emit as a semicolon-separated set-eux block, or nil if no transformation
// applies (e.g. already in semicolon form, or not a pure && chain).
//
// Additional per-instruction transforms can be added here as the need arises,
// keeping tidy rewrites separate from the Formatter's presentation concerns.
func TidyFile(f *File, tidyRUN func(args string) []string) {
	for _, instr := range f.Instructions {
		if instr.Keyword == "RUN" && tidyRUN != nil {
			if cmds := tidyRUN(instr.Args); cmds != nil {
				applyRUNCommands(instr, cmds)
			}
		}
	}
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

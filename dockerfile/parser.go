package dockerfile

import (
	"fmt"
	"strings"
)

// Parse parses a Dockerfile from src and returns the AST.
func Parse(src string) (*File, error) {
	lines := splitLines(src)
	p := &parser{lines: lines}
	return p.parse()
}

// splitLines splits src into individual lines without trailing newlines.
func splitLines(src string) []string {
	// strings.Split would produce a spurious empty trailing element for files
	// ending in \n; we handle that manually.
	raw := strings.Split(src, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	// Strip \r for Windows line endings.
	for i, l := range raw {
		raw[i] = strings.TrimRight(l, "\r")
	}
	return raw
}

type parser struct {
	lines  []string
	pos    int  // index into lines (0-based)
	escape byte // continuation escape character, default '\'
}

func (p *parser) parse() (*File, error) {
	f := &File{}

	// Phase 1: collect parser directives from the top of the file.
	// Per spec §2.1, directives are only recognised before the first
	// non-directive content.
	directivesDone := false
	for p.pos < len(p.lines) {
		raw := p.lines[p.pos]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			// Blank line ends the directive zone.
			directivesDone = true
			break
		}
		if !strings.HasPrefix(trimmed, "#") {
			// First non-comment line ends the directive zone.
			directivesDone = true
			break
		}
		// Try to parse as a directive.
		if d := parseDirective(trimmed); d != nil {
			d.Raw = raw
			f.Directives = append(f.Directives, d)
			if d.Name == "escape" && len(d.Value) == 1 {
				p.escape = d.Value[0]
			}
			p.pos++
		} else {
			// It's a plain comment, not a directive — end directive zone.
			directivesDone = true
			break
		}
	}
	_ = directivesDone

	// Set default escape if not overridden.
	if p.escape == 0 {
		p.escape = '\\'
	}

	// Phase 2: parse instructions.
	for p.pos < len(p.lines) {
		instr, err := p.parseInstruction()
		if err != nil {
			return nil, err
		}
		if instr != nil {
			f.Instructions = append(f.Instructions, instr)
		}
	}

	return f, nil
}

// parseDirective attempts to parse a line as a parser directive.
// Returns nil if the line is not a directive.
func parseDirective(trimmed string) *Directive {
	// Must match: # key = value (case-insensitive key)
	rest := strings.TrimPrefix(trimmed, "#")
	rest = strings.TrimSpace(rest)
	before, after, ok := strings.Cut(rest, "=")
	if !ok {
		return nil
	}
	key := strings.TrimSpace(before)
	val := strings.TrimSpace(after)
	keyLower := strings.ToLower(key)
	if keyLower != "syntax" && keyLower != "escape" && keyLower != "check" {
		return nil
	}
	return &Directive{Name: keyLower, Value: val}
}

// parseInstruction parses one logical instruction (which may span multiple
// physical lines via the escape continuation character).
func (p *parser) parseInstruction() (*Instruction, error) {
	if p.pos >= len(p.lines) {
		return nil, nil
	}

	startPos := p.pos
	raw := p.lines[p.pos]
	trimmed := strings.TrimSpace(raw)

	// Blank line: emit as a blank instruction marker.
	if trimmed == "" {
		p.pos++
		return &Instruction{
			Keyword:   "",
			Lines:     []Line{{Text: raw, Kind: LineKindBlank}},
			StartLine: startPos + 1,
			EndLine:   startPos + 1,
		}, nil
	}

	// Comment line: emit as a COMMENT instruction.
	if strings.HasPrefix(trimmed, "#") {
		p.pos++
		return &Instruction{
			Keyword:   "COMMENT",
			Args:      raw, // preserve verbatim
			Lines:     []Line{{Text: raw, Kind: LineKindComment}},
			StartLine: startPos + 1,
			EndLine:   startPos + 1,
		}, nil
	}

	// Real instruction: accumulate logical line by following continuation chars.
	instr := &Instruction{
		StartLine: startPos + 1,
	}

	var logicalParts []string
	escape := string(p.escape)
	unterminated := false

	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		stripped := strings.TrimRight(line, "\r")

		if p.pos == startPos {
			// First line of instruction.
			instr.Lines = append(instr.Lines, Line{Text: stripped, Kind: LineKindInstruction})
		} else {
			// Continuation line: could be a comment within a continuation block
			// or a real continuation.
			lineKind := LineKindContinuation
			trimLine := strings.TrimSpace(stripped)
			if strings.HasPrefix(trimLine, "#") {
				// Inline comment within a continuation block.
				// Per Dockerfile spec, comments within a continuation block are
				// stripped (they don't break the continuation).
				instr.Lines = append(instr.Lines, Line{Text: stripped, Kind: LineKindComment})
				p.pos++
				continue
			}
			if trimLine == "" {
				// Blank continuation line (a lone backslash + blank line is
				// used as a visual separator in RUN blocks).
				instr.Lines = append(instr.Lines, Line{Text: stripped, Kind: LineKindContinuation})
				p.pos++
				continue
			}
			instr.Lines = append(instr.Lines, Line{Text: stripped, Kind: lineKind})
		}

		// Trim trailing whitespace before checking for the escape char so that
		// "\ " (backslash + trailing space) is treated as a continuation, matching
		// the behaviour of the Docker daemon itself.
		strippedTrimmed := strings.TrimRight(stripped, " \t")
		continues := strings.HasSuffix(strippedTrimmed, escape)
		if continues {
			// Strip the trailing escape char and accumulate without it.
			logicalParts = append(logicalParts, strings.TrimRight(strippedTrimmed[:len(strippedTrimmed)-len(escape)], " \t"))
			unterminated = true
			p.pos++
		} else {
			logicalParts = append(logicalParts, stripped)
			unterminated = false
			p.pos++
			break
		}
	}
	if unterminated {
		return nil, fmt.Errorf("dockerfile parse error at line %d: unterminated continuation", p.pos)
	}

	instr.EndLine = p.pos // 1-based: p.pos now points past the last line

	// Build the logical line by joining parts with a single space.
	logical := strings.Join(logicalParts, " ")
	logical = strings.TrimSpace(logical)

	// Extract keyword and args from the logical line.
	kw, args := splitKeyword(logical)
	instr.Keyword = strings.ToUpper(kw)
	instr.Args = args

	if err := validateInstruction(instr); err != nil {
		return nil, err
	}
	return instr, nil
}

// validateInstruction checks structural well-formedness for instructions whose
// args are required or constrained by the Dockerfile spec.  It is called at
// parse time so that callers never receive an AST containing instructions that
// cannot be formatted back into valid Dockerfiles.
//
// TODO: validate ENV — detect the ambiguous mixed form "ENV KEY VALUE=something",
// where the old space-separated syntax is used but the value contains "=", making
// it look like a new-style "KEY=VALUE" pair to readers.  Surface as a lint violation
// (not a parse error) since the form is syntactically valid; it just sets KEY to the
// literal string "VALUE=something".  The fix the user wants is "ENV KEY=VALUE=something".
//
// TODO: validate COPY/ADD — require at least two path arguments (src + dest).
//
// TODO: validate ARG — verify whether bare "ARG" with no name is invalid per
// spec before adding a parse error; the grammar says ARG requires a name but
// this has not been confirmed against actual Docker behaviour.
//
// TODO: surface unknown instructions as lint violations rather than parse
// errors, since BuildKit allows custom syntax via # syntax= directives and
// future Docker releases add new instructions.  A lint check can warn without
// blocking formatting of otherwise-valid files.
func validateInstruction(instr *Instruction) error {
	line := instr.StartLine
	switch instr.Keyword {
	case "FROM":
		args := strings.TrimSpace(instr.Args)
		// Strip --platform=P so we can inspect the image ref that follows.
		if rest, ok := strings.CutPrefix(args, "--platform="); ok {
			_, args, _ = strings.Cut(rest, " ")
			args = strings.TrimSpace(args)
		}
		if args == "" {
			return fmt.Errorf("dockerfile parse error at line %d: FROM requires an image reference", line)
		}
		// AS clause must be a single token — "FROM img AS a b" is malformed.
		// Note: strings.Index looks for " AS " (spaces both sides), so "FROM img AS"
		// with no trailing word produces no match and is handled by Docker as a bare ref.
		if idx := strings.Index(strings.ToUpper(args), " AS "); idx >= 0 {
			alias := strings.TrimSpace(args[idx+4:])
			if strings.ContainsAny(alias, " \t") {
				return fmt.Errorf("dockerfile parse error at line %d: FROM alias must be a single token, got %q", line, alias)
			}
		}
	}
	return nil
}

// splitKeyword splits a logical line into its keyword and the rest.
func splitKeyword(s string) (keyword, rest string) {
	s = strings.TrimSpace(s)
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimSpace(s[idx:])
}

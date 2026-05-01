package dockerfile

import (
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
	pos    int // index into lines (0-based)
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
	eqIdx := strings.IndexByte(rest, '=')
	if eqIdx < 0 {
		return nil
	}
	key := strings.TrimSpace(rest[:eqIdx])
	val := strings.TrimSpace(rest[eqIdx+1:])
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

		continues := strings.HasSuffix(stripped, escape)
		if continues {
			// Strip the trailing escape char and accumulate without it.
			logicalParts = append(logicalParts, strings.TrimRight(stripped[:len(stripped)-len(escape)], " \t"))
			p.pos++
		} else {
			logicalParts = append(logicalParts, stripped)
			p.pos++
			break
		}
	}

	instr.EndLine = p.pos // 1-based: p.pos now points past the last line

	// Build the logical line by joining parts with a single space.
	logical := strings.Join(logicalParts, " ")
	logical = strings.TrimSpace(logical)

	// Extract keyword and args from the logical line.
	kw, args := splitKeyword(logical)
	instr.Keyword = strings.ToUpper(kw)
	instr.Args = args

	return instr, nil
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

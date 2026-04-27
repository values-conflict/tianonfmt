package dockerfile

import (
	"encoding/json"
	"strings"
)

// MarshalFile converts a parsed Dockerfile File to a JSON-serialisable value.
// filename is embedded as "file" in the top-level object; use "-" for stdin.
func MarshalFile(f *File, filename string) any {
	return fileAST{
		Type:         "dockerfile",
		File:         filename,
		Directives:   marshalDirectives(f.Directives),
		Instructions: marshalInstructions(f.Instructions),
	}
}

// ── JSON shapes ───────────────────────────────────────────────────────────────
//
// Structs give us semantic key ordering for free (encoding/json uses field
// declaration order), without needing an OrderedMap.
//
// Instructions is []any because different keywords produce different structs.

type fileAST struct {
	Type         string         `json:"type"`
	File         string         `json:"file"`
	Directives   []directiveAST `json:"directives,omitempty"`
	Instructions []any          `json:"instructions"`
}

type directiveAST struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// genericInstrAST covers keywords we don't parse further.
type genericInstrAST struct {
	Type      string    `json:"type"`
	Args      string    `json:"args,omitempty"`
	Lines     []lineAST `json:"lines"`
	StartLine int       `json:"startLine"`
	EndLine   int       `json:"endLine"`
}

// fromAST covers FROM [--platform=P] ref [AS alias].
// ref is an image reference or an earlier stage name.
type fromAST struct {
	Type      string    `json:"type"`
	Platform  string    `json:"platform,omitempty"`
	Ref       string    `json:"ref"`
	Alias     string    `json:"alias,omitempty"`
	Lines     []lineAST `json:"lines"`
	StartLine int       `json:"startLine"`
	EndLine   int       `json:"endLine"`
}

// execFormAST covers CMD/ENTRYPOINT/SHELL/RUN where exec vs shell form matters.
type execFormAST struct {
	Type      string    `json:"type"`
	Form      string    `json:"form"` // "exec" | "shell"
	Args      any       `json:"args"` // []string for exec, string for shell
	Lines     []lineAST `json:"lines"`
	StartLine int       `json:"startLine"`
	EndLine   int       `json:"endLine"`
}

// copyAST covers COPY and ADD with flags extracted and exec/shell form detected.
type copyAST struct {
	Type      string    `json:"type"`
	From      string    `json:"from,omitempty"`
	Form      string    `json:"form"`  // "exec" | "shell"
	Paths     any       `json:"paths"` // []string for exec, string for shell
	Lines     []lineAST `json:"lines"`
	StartLine int       `json:"startLine"`
	EndLine   int       `json:"endLine"`
}

type lineAST struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

// ── helpers ───────────────────────────────────────────────────────────────────

func marshalDirectives(ds []*Directive) []directiveAST {
	out := make([]directiveAST, len(ds))
	for i, d := range ds {
		out[i] = directiveAST{Name: d.Name, Value: d.Value}
	}
	return out
}

func marshalInstructions(instrs []*Instruction) []any {
	out := make([]any, len(instrs))
	for i, instr := range instrs {
		out[i] = marshalInstruction(instr)
	}
	return out
}

func marshalInstruction(instr *Instruction) any {
	lines := marshalLines(instr.Lines)
	kw := instr.Keyword
	if kw == "" {
		kw = "blank"
	}

	switch kw {
	case "FROM":
		platform, ref, alias := parseFROMArgs(instr.Args)
		return fromAST{
			Type:      "FROM",
			Platform:  platform,
			Ref:       ref,
			Alias:     alias,
			Lines:     lines,
			StartLine: instr.StartLine,
			EndLine:   instr.EndLine,
		}

	case "CMD", "ENTRYPOINT", "SHELL", "RUN", "VOLUME":
		form, execArgs, shellArgs := parseExecForm(instr.Args)
		var args any
		if form == "exec" {
			args = execArgs
		} else {
			args = shellArgs
		}
		return execFormAST{
			Type:      kw,
			Form:      form,
			Args:      args,
			Lines:     lines,
			StartLine: instr.StartLine,
			EndLine:   instr.EndLine,
		}

	case "COPY", "ADD":
		from, form, paths := parseCOPYArgs(instr.Args)
		return copyAST{
			Type:      kw,
			From:      from,
			Form:      form,
			Paths:     paths,
			Lines:     lines,
			StartLine: instr.StartLine,
			EndLine:   instr.EndLine,
		}

	default:
		return genericInstrAST{
			Type:      kw,
			Args:      instr.Args,
			Lines:     lines,
			StartLine: instr.StartLine,
			EndLine:   instr.EndLine,
		}
	}
}

var lineKindNames = [...]string{
	LineKindInstruction:  "instruction",
	LineKindContinuation: "continuation",
	LineKindComment:      "comment",
	LineKindBlank:        "blank",
	LineKindDirective:    "directive",
}

func marshalLines(lines []Line) []lineAST {
	out := make([]lineAST, len(lines))
	for i, l := range lines {
		out[i] = lineAST{Kind: lineKindNames[l.Kind], Text: l.Text}
	}
	return out
}

// ── instruction-specific parsers ──────────────────────────────────────────────

// parseFROMArgs splits "FROM [--platform=P] ref [AS alias]" args into parts.
func parseFROMArgs(args string) (platform, ref, alias string) {
	args = strings.TrimSpace(args)
	if rest, ok := strings.CutPrefix(args, "--platform="); ok {
		platform, args, _ = strings.Cut(rest, " ")
		args = strings.TrimSpace(args)
	}
	// "AS" is case-insensitive per BuildKit spec
	upper := strings.ToUpper(args)
	if idx := strings.Index(upper, " AS "); idx >= 0 {
		ref = strings.TrimSpace(args[:idx])
		alias = strings.TrimSpace(args[idx+4:])
	} else {
		ref = args
	}
	return
}

// parseExecForm detects JSON exec form ("[...]") vs shell form.
func parseExecForm(args string) (form string, execArgs []string, shellArgs string) {
	trimmed := strings.TrimSpace(args)
	if strings.HasPrefix(trimmed, "[") {
		var exec []string
		if err := json.Unmarshal([]byte(trimmed), &exec); err == nil {
			return "exec", exec, ""
		}
	}
	return "shell", nil, args
}

// parseCOPYArgs strips COPY/ADD flags and detects exec vs shell form.
// All COPY/ADD flags use = syntax (--from=, --chown=, --chmod=, --link, etc.),
// so we can split on spaces to advance past flags without ambiguity.
// The path spec is then the remaining string; if it starts with "[" and is
// valid JSON it is exec form, otherwise shell form.
func parseCOPYArgs(args string) (from, form string, paths any) {
	s := strings.TrimSpace(args)
	for strings.HasPrefix(s, "--") {
		i := strings.IndexByte(s, ' ')
		if i < 0 {
			// Only flags, no path spec.
			if val, ok := strings.CutPrefix(s, "--from="); ok {
				from = val
			}
			return from, "shell", ""
		}
		flag := s[:i]
		if val, ok := strings.CutPrefix(flag, "--from="); ok {
			from = val
		}
		s = strings.TrimSpace(s[i:])
	}
	// s is now the path spec; may contain spaces (exec form paths like "C:\Program Files").
	if strings.HasPrefix(s, "[") {
		var exec []string
		if err := json.Unmarshal([]byte(s), &exec); err == nil {
			return from, "exec", exec
		}
	}
	return from, "shell", s
}

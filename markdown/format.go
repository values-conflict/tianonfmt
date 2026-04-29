// Package markdown formats Markdown files according to Tianon's style.
//
// Rules enforced (markdown.md):
//   - ATX headers (#) instead of setext underlines (=== / ---)
//   - Bullet character is always "-", never "*" or "+"
//   - No trailing whitespace (except intentional soft breaks: two trailing spaces)
//   - At most one blank line between blocks
//   - Code fences always have a language identifier; "sh"/"shell" are Wrong
//
// Lines inside fenced code blocks are passed through unchanged.
package markdown

import (
	"strings"
)

// Format formats a Markdown source string.
func Format(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))

	inFence := false
	var fenceMarker string // the opening fence (e.g. "```", "````")
	consecutiveBlanks := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Detect fenced code blocks.
		if !inFence {
			if marker := fenceStart(line); marker != "" {
				inFence = true
				fenceMarker = marker
				out = append(out, line)
				consecutiveBlanks = 0
				continue
			}
		} else {
			if fenceEnd(line, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			out = append(out, line)
			consecutiveBlanks = 0
			continue
		}

		// Outside code blocks: apply normalizations.

		// Convert setext-style headers to ATX.
		// A setext underline is a line of === or --- immediately after a non-blank line.
		if i+1 < len(lines) && !inFence {
			next := lines[i+1]
			if isSetextUnderline(next, '=') {
				out = append(out, "# "+strings.TrimSpace(line))
				i++ // skip the underline
				consecutiveBlanks = 0
				continue
			}
			if isSetextUnderline(next, '-') && !isBullet(line) {
				out = append(out, "## "+strings.TrimSpace(line))
				i++ // skip the underline
				consecutiveBlanks = 0
				continue
			}
		}

		// Normalise bullet characters: * and + → -
		if bullet, rest := parseBullet(line); bullet != "" && bullet != "-" {
			indent := leadingSpaces(line)
			line = indent + "- " + rest
		}

		// Trailing whitespace: remove unless it's exactly two spaces (soft break).
		trimmed := strings.TrimRight(line, " \t")
		trailingSpaces := len(line) - len(strings.TrimRight(line, " "))
		if trailingSpaces == 2 {
			line = trimmed + "  " // preserve intentional soft break
		} else {
			line = trimmed
		}

		// Limit consecutive blank lines to 1.
		if line == "" {
			consecutiveBlanks++
			if consecutiveBlanks > 1 {
				continue // drop this blank
			}
		} else {
			consecutiveBlanks = 0
		}

		out = append(out, line)
	}

	// Ensure single trailing newline.
	result := strings.Join(out, "\n")
	result = strings.TrimRight(result, "\n") + "\n"
	return result
}

// MarshalFile converts a parsed markdown file to a JSON-serialisable value.
// filename is embedded as "file" in the top-level object; use "-" for stdin.
func MarshalFile(src, filename string) any {
	type block = map[string]any
	var blocks []any
	lines := strings.Split(src, "\n")
	inFence := false
	var fenceMarker, fenceLang string
	var fenceLines []string

	flush := func(b any) { blocks = append(blocks, b) }

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if !inFence {
			if marker := fenceStart(line); marker != "" {
				inFence = true
				fenceMarker = marker
				fenceLang = strings.TrimSpace(line[len(marker):])
				fenceLines = nil
				continue
			}
		} else {
			if fenceEnd(line, fenceMarker) {
				flush(block{"type": "codeBlock", "lang": fenceLang, "content": strings.Join(fenceLines, "\n")})
				inFence = false
				fenceMarker, fenceLang = "", ""
				continue
			}
			fenceLines = append(fenceLines, line)
			continue
		}

		// ATX heading
		if strings.HasPrefix(line, "#") {
			level := 0
			for level < len(line) && line[level] == '#' {
				level++
			}
			if level <= 6 && (level == len(line) || line[level] == ' ') {
				text := strings.TrimSpace(line[level:])
				flush(block{"type": "heading", "level": level, "text": text})
				continue
			}
		}

		// Setext heading (peek ahead)
		if i+1 < len(lines) && line != "" {
			next := lines[i+1]
			if isSetextUnderline(next, '=') {
				flush(block{"type": "heading", "level": 1, "text": strings.TrimSpace(line)})
				i++
				continue
			}
			if isSetextUnderline(next, '-') && !isBullet(line) {
				flush(block{"type": "heading", "level": 2, "text": strings.TrimSpace(line)})
				i++
				continue
			}
		}

		// Blank line
		if strings.TrimSpace(line) == "" {
			flush(block{"type": "blank"})
			continue
		}

		// List item
		if b, rest := parseBullet(line); b != "" {
			flush(block{"type": "listItem", "bullet": b, "text": rest})
			continue
		}

		// Paragraph / other
		flush(block{"type": "paragraph", "text": line})
	}

	return map[string]any{
		"type":   "markdown",
		"file":   filename,
		"blocks": blocks,
	}
}

// Lint returns pedantic violations in a markdown file.
// Currently detects: "sh" or "shell" language identifiers in fenced blocks.
func Lint(src string) []Violation {
	var out []Violation
	inFence := false
	var fenceMarker string
	for i, line := range strings.Split(src, "\n") {
		if !inFence {
			if marker := fenceStart(line); marker != "" {
				inFence = true
				fenceMarker = marker
				// Check language identifier on the fence opening line.
				lang := strings.TrimSpace(line[len(marker):])
				if lang == "sh" || lang == "shell" {
					out = append(out, Violation{
						Line: i + 1,
						Msg:  `code fence uses "` + lang + `" language identifier; use "bash" for scripts or "console" for interactive sessions`,
					})
				}
			}
		} else {
			if fenceEnd(line, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
		}
	}
	return out
}

// Violation is a pedantic style issue in a markdown file.
type Violation struct {
	Line int
	Msg  string
}

// ── helpers ───────────────────────────────────────────────────────────────────

func fenceStart(line string) string {
	s := strings.TrimLeft(line, " \t")
	for _, m := range []string{"````", "```", "~~~"} {
		if strings.HasPrefix(s, m) {
			return m
		}
	}
	return ""
}

func fenceEnd(line, marker string) bool {
	s := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(s, marker) && strings.TrimRight(s[len(marker):], " \t") == ""
}

func isSetextUnderline(line string, ch byte) bool {
	s := strings.TrimSpace(line)
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != ch {
			return false
		}
	}
	return true
}

// parseBullet returns the bullet character and the rest of the line
// if the line starts with a list bullet (* / + / -).
func parseBullet(line string) (bullet, rest string) {
	indent := leadingSpaces(line)
	s := line[len(indent):]
	for _, b := range []string{"* ", "+ ", "- "} {
		if strings.HasPrefix(s, b) {
			return string(b[0]), s[len(b):]
		}
	}
	return "", ""
}

func isBullet(line string) bool {
	b, _ := parseBullet(line)
	return b != ""
}

func leadingSpaces(line string) string {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[:i]
}

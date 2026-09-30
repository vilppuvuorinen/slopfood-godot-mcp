// Package gdcover is a source-to-source instrumenter for GDScript.
//
// Class-level declarations and inner-class bodies are deliberately left alone:
// the injected helper is a member of the script itself, and a nested class
// cannot see it.
package gdcover

import (
	"bytes"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// HitFunc is the name of the helper instrumented scripts call to record a hit.
const HitFunc = "__gdcover_hit"

// FileEnv is the default environment variable that holds the hit output path.
const FileEnv = "GDCOVER_FILE"

var funcHeader = regexp.MustCompile(`^(?:static\s+)?func\s+[A-Za-z_]\w*\s*\(`)

// Result is an instrumented script and the lines measured for coverage.
type Result struct {
	Source []byte
	Lines  []int
}

func Instrument(src []byte) (Result, error) {
	return InstrumentEnv(src, FileEnv)
}

// InstrumentEnv is Instrument with a caller-chosen hit-output environment
// variable, and returns an error if src is already instrumented.
func InstrumentEnv(src []byte, fileEnv string) (Result, error) {
	if bytes.Contains(src, []byte(HitFunc)) {
		return Result{}, errors.New("gdcover: script is already instrumented")
	}

	lines := splitLines(string(src))
	indent := detectIndent(lines)
	instrumentable := map[int]struct{}{}
	record := func(line int, indentation string) string {
		instrumentable[line] = struct{}{}
		return hit(indentation, line)
	}

	before := map[int][]string{}
	after := map[int][]string{}
	inFunc := false
	depth := 0
	inMultiline := 0

	for i := 0; i < len(lines); {
		start := i
		var code strings.Builder
		headIndent := leadingWhitespace(lines[start])

		for {
			lineCode, delta, continued, multiline := scanLine(lines[i], inMultiline)
			code.WriteString(lineCode)
			depth += delta
			inMultiline = multiline
			i++
			if depth <= 0 && !continued && inMultiline == 0 {
				break
			}
			if i >= len(lines) {
				break
			}
		}
		end := i - 1

		trimmed := strings.TrimSpace(code.String())
		if trimmed == "" {
			continue
		}

		isFunc := headIndent == "" && funcHeader.MatchString(trimmed)
		if headIndent == "" {
			inFunc = isFunc
		}

		switch {
		case strings.HasSuffix(trimmed, ":"):
			// A match has no single body: its arms are the branches, and each
			// arm is instrumented on its own. Injecting before the first arm
			// would be read as a pattern, so count the match line itself.
			if firstWord(trimmed) == "match" {
				if inFunc {
					before[start] = append(before[start], record(start+1, headIndent))
				}
				continue
			}
			// A block header: count the header when its body runs. Bodies are
			// entered via the first more-indented line after the header.
			if !isFunc && !inFunc {
				continue
			}
			if next := nextNonBlank(lines, end+1); next >= 0 {
				nextIndent := leadingWhitespace(lines[next])
				if isDeeper(nextIndent, headIndent) {
					before[next] = append(before[next], record(start+1, nextIndent))
				}
			}
		case !inFunc:
			// A class-level declaration has no enclosing function to report to.
			continue
		case isTerminator(trimmed):
			// A return/break/continue/pass can only be followed by unreachable
			// code, so count it before it runs instead of after.
			before[start] = append(before[start], record(start+1, headIndent))
		case strings.HasPrefix(strings.TrimSpace(lines[start]), "@"):
			// An annotation belongs to the declaration that follows it; leave
			// the pair adjacent.
			continue
		default:
			after[end] = append(after[end], record(start+1, headIndent))
		}
	}

	header := headerEnd(lines)
	out := make([]string, 0, len(lines)+len(prologue(indent, fileEnv))+1)
	if header < 0 {
		out = append(out, prologue(indent, fileEnv)...)
	}
	for i, line := range lines {
		out = append(out, before[i]...)
		out = append(out, strings.TrimRight(line, "\r"))
		if i == header {
			out = append(out, prologue(indent, fileEnv)...)
		}
		out = append(out, after[i]...)
	}

	covered := make([]int, 0, len(instrumentable))
	for line := range instrumentable {
		covered = append(covered, line)
	}
	sort.Ints(covered)

	return Result{
		Source: []byte(strings.Join(out, "\n") + "\n"),
		Lines:  covered,
	}, nil
}

func hit(indent string, line int) string {
	return indent + HitFunc + "(" + strconv.Itoa(line) + ")"
}

// prologue is the helper injected after the script's header declarations. It
// appends each hit immediately so hits survive a quit() partway through.
func prologue(unit, fileEnv string) []string {
	return []string{
		`var __gdcover_path: String = OS.get_environment("` + fileEnv + `")`,
		`var __gdcover_out: FileAccess`,
		``,
		`func ` + HitFunc + `(__gdcover_line: int) -> void:`,
		unit + `if __gdcover_path.is_empty():`,
		unit + unit + `return`,
		unit + `if __gdcover_out == null:`,
		unit + unit + `if FileAccess.file_exists(__gdcover_path):`,
		unit + unit + unit + `__gdcover_out = FileAccess.open(__gdcover_path, FileAccess.READ_WRITE)`,
		unit + unit + unit + `if __gdcover_out != null:`,
		unit + unit + unit + unit + `__gdcover_out.seek_end()`,
		unit + unit + `else:`,
		unit + unit + unit + `__gdcover_out = FileAccess.open(__gdcover_path, FileAccess.WRITE)`,
		unit + unit + `if __gdcover_out == null:`,
		unit + unit + unit + `return`,
		unit + `__gdcover_out.store_line(str(__gdcover_line))`,
		unit + `__gdcover_out.flush()`,
	}
}

// scanLine strips a line's comment and string contents and counts net bracket
// nesting outside strings. Multiline state: 0 none, 1 triple-double, 2 triple-single.
func scanLine(line string, inMultiline int) (code string, delta int, continued bool, multiline int) {
	var b strings.Builder
	state := inMultiline
	i := 0
	for i < len(line) {
		c := line[i]
		if state != 0 {
			delim := `"""`
			if state == 2 {
				delim = "'''"
			}
			if strings.HasPrefix(line[i:], delim) {
				b.WriteString("   ")
				i += len(delim)
				state = 0
				continue
			}
			b.WriteByte(' ')
			i++
			continue
		}
		switch c {
		case '#':
			i = len(line)
		case '"', '\'':
			if i+2 < len(line) && line[i+1] == c && line[i+2] == c {
				state = 2
				if c == '"' {
					state = 1
				}
				b.WriteString("   ")
				i += 3
				continue
			}
			b.WriteByte(' ')
			i++
			for i < len(line) {
				if line[i] == '\\' {
					b.WriteString("  ")
					i += 2
					continue
				}
				if line[i] == c {
					i++
					break
				}
				b.WriteByte(' ')
				i++
			}
		case '(', '[', '{':
			b.WriteByte(c)
			delta++
			i++
		case ')', ']', '}':
			b.WriteByte(c)
			delta--
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	code = b.String()
	continued = state == 0 && strings.HasSuffix(strings.TrimRight(code, " \t"), `\`)
	return code, delta, continued, state
}

// headerEnd returns the last index of the leading header block, or -1 if there
// is none, in which case the helper goes at the very top.
func headerEnd(lines []string) int {
	last := -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case t == "",
			strings.HasPrefix(t, "#"),
			strings.HasPrefix(t, "@tool"),
			strings.HasPrefix(t, "@icon"),
			strings.HasPrefix(t, "@static_unload"),
			strings.HasPrefix(t, "class_name"),
			strings.HasPrefix(t, "extends"):
			last = i
		default:
			return last
		}
	}
	return last
}

func nextNonBlank(lines []string, from int) int {
	for i := from; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

func isTerminator(code string) bool {
	switch firstWord(code) {
	case "return", "break", "continue", "pass":
		return true
	default:
		return false
	}
}

func firstWord(code string) string {
	code = strings.TrimSpace(code)
	i := strings.IndexFunc(code, func(r rune) bool {
		return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if i < 0 {
		return code
	}
	return code[:i]
}

func isDeeper(child, parent string) bool {
	return len(child) > len(parent) && strings.HasPrefix(child, parent)
}

func leadingWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// detectIndent returns the indentation unit the injected prologue should use.
func detectIndent(lines []string) string {
	for _, line := range lines {
		ws := leadingWhitespace(line)
		if strings.Contains(ws, "\t") {
			return "\t"
		}
		if strings.Contains(ws, " ") {
			return "    "
		}
	}
	return "    "
}

// splitLines splits text into lines, dropping the empty line from a final newline.
func splitLines(text string) []string {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

package godot

import (
	"regexp"
	"strings"
)

// Matches the engine's project.godot parse-failure diagnostic; Godot still
// exits 0, so callers that need to reject a broken project inspect stderr.
var projectParseErrorPattern = regexp.MustCompile(`Error parsing '.+project\.godot'|Couldn't load file '.+project\.godot'`)

// ProjectParseErrors returns the engine's project.godot parse-failure
// diagnostics found in stderr, one per matching line.
func ProjectParseErrors(stderr string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if projectParseErrorPattern.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}

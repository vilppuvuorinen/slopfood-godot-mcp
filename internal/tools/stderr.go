package tools

import (
	"regexp"
	"strings"
)

// Godot writes some failures to stderr while still exiting 0, so a clean exit
// code is not proof of success; tools declare known-benign output via allowed.
func unexpectedStderr(stderr string, allowed ...*regexp.Regexp) string {
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matched := false
		for _, re := range allowed {
			if re.MatchString(line) {
				matched = true
				break
			}
		}
		if !matched {
			return line
		}
	}
	return ""
}

func scriptFailed(exitCode int, stderr string, allowed ...*regexp.Regexp) bool {
	return exitCode != 0 || unexpectedStderr(stderr, allowed...) != ""
}

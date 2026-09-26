package tools

import (
	"regexp"
	"strings"
)

// unexpectedStderr returns the first stderr line that is not matched by any of
// the allowed patterns, or "" when every non-empty line is expected.
//
// Godot writes some failures to stderr while still exiting 0 (an invalid
// project.godot, a script that does not compile), so a clean exit code is not
// proof of success. Treating any unrecognized stderr as a failure makes those
// cases loud; tools with known benign output declare it via allowed.
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

// scriptFailed reports whether a Godot invocation failed, combining the exit
// code with stderr: a non-zero exit always fails, and on exit 0 any stderr line
// not listed in allowed is unexpected and also fails. The operations script
// calls quit(1) on every error branch, but engine-level failures log to stderr
// and still exit 0, so both signals are needed.
//
// The allowed patterns are the per-tool allowlist for known-benign engine
// output: a tool declares them for engine noise it cannot prevent and does not
// treat as a failure. Add a pattern only for output outside the operation's
// control that is provably benign, and keep it as narrow as the line it matches.
func scriptFailed(exitCode int, stderr string, allowed ...*regexp.Regexp) bool {
	return exitCode != 0 || unexpectedStderr(stderr, allowed...) != ""
}

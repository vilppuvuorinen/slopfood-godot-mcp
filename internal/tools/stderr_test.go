package tools

import (
	"regexp"
	"testing"
)

func TestUnexpectedStderr(t *testing.T) {
	known := regexp.MustCompile(`^WARNING: known`)
	tests := []struct {
		name    string
		stderr  string
		allowed []*regexp.Regexp
		want    string
	}{
		{name: "empty", stderr: "", want: ""},
		{name: "blank lines", stderr: "\n  \n", want: ""},
		{name: "allowed line", stderr: "WARNING: known noise\n", allowed: []*regexp.Regexp{known}, want: ""},
		{name: "unexpected line", stderr: "ERROR: boom\n", allowed: []*regexp.Regexp{known}, want: "ERROR: boom"},
		{name: "mixed", stderr: "WARNING: known noise\nERROR: boom\n", allowed: []*regexp.Regexp{known}, want: "ERROR: boom"},
		{name: "unexpected without allowlist", stderr: "ERROR: boom\n", want: "ERROR: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unexpectedStderr(tt.stderr, tt.allowed...); got != tt.want {
				t.Errorf("unexpectedStderr(%q) = %q, want %q", tt.stderr, got, tt.want)
			}
		})
	}
}

func TestScriptFailedAllowedStderr(t *testing.T) {
	known := regexp.MustCompile(`^WARNING: known`)
	if scriptFailed(0, "WARNING: known noise\n", known) {
		t.Error("an allowed stderr line on exit 0 must not fail")
	}
	if !scriptFailed(0, "WARNING: known noise\nERROR: boom\n", known) {
		t.Error("an unexpected stderr line must fail")
	}
}

package tools

import (
	"strings"
	"testing"
)

// TestScanValueTrailingBackslash guards against a value whose quoted string
// ends with a lone backslash at EOF; the scanner must not return an offset past
// the end of the content.
func TestScanValueTrailingBackslash(t *testing.T) {
	const content = "key=\"abc\\"
	if got := scanValue(content, 4); got > len(content) {
		t.Fatalf("scanValue = %d, want <= %d", got, len(content))
	}
	// A malformed value must not panic downstream.
	_ = parseProjectSections(content)
}

func TestParseProjectSectionsMultilineValue(t *testing.T) {
	content := "config_version=5\n\n[input]\n\njump={\n\"deadzone\": 0.5,\n\"events\": []\n}\n\n[application]\n\nconfig/name=\"Demo\"\n"
	sections := parseProjectSections(content)
	if got, want := sections["input"]["jump"], "{\n\"deadzone\": 0.5,\n\"events\": []\n}"; got != want {
		t.Errorf("jump = %q, want %q", got, want)
	}
	if got := sections["application"]["config/name"]; got != `"Demo"` {
		t.Errorf("config/name = %q", got)
	}
}

func TestRemoveSectionKeyMultiline(t *testing.T) {
	content := "config_version=5\n\n[input]\n\njump={\n\"deadzone\": 0.5,\n\"events\": []\n}\nmove={\"deadzone\": 0.5}\n"
	got := removeSectionKey(content, "input", "jump")
	if strings.Contains(got, "jump") || strings.Contains(got, `"events"`) {
		t.Errorf("jump not fully removed:\n%s", got)
	}
	if !strings.Contains(got, `move={"deadzone": 0.5}`) {
		t.Errorf("other key lost:\n%s", got)
	}
}

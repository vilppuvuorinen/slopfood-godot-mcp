package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectSchema(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    configSchema
		wantErr string
	}{
		{name: "godot 4", content: "config_version=5\n\n[application]\n", want: schemaGodot4},
		{name: "godot 3", content: "config_version=4\n", wantErr: "unsupported project.godot config_version 4"},
		{name: "future", content: "config_version=6\n", wantErr: "unsupported project.godot config_version 6"},
		{name: "missing", content: "[application]\n\nconfig/name=\"x\"\n", wantErr: "no config_version"},
		{name: "not an integer", content: "config_version=\"5\"\n", wantErr: "is not an integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projectSchema(tt.content)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("schema = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestWriteProjectGodotRejectsUnsupportedSchema checks the write path refuses
// content whose schema this server does not implement and leaves no file behind.
func TestWriteProjectGodotRejectsUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	if err := writeProjectGodot(dir, "config_version=4\n"); err == nil {
		t.Fatal("expected an error for Godot 3 content")
	}
	if _, err := os.Stat(filepath.Join(dir, "project.godot")); !os.IsNotExist(err) {
		t.Fatalf("project.godot written despite unsupported schema: %v", err)
	}
}

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

package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotconfig"
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

// TestWriteProjectGodotRejectsUnparseableContent checks the write path refuses
// content the line-based schema check accepts but the strict ConfigFile parser
// rejects, leaving no file behind. A section name ending in a backslash escapes
// the closing bracket for Godot's parser, so the tag never terminates.
func TestWriteProjectGodotRejectsUnparseableContent(t *testing.T) {
	dir := t.TempDir()
	if err := writeProjectGodot(dir, "config_version=5\n\n[trail\\]\n\nkey=1\n"); err == nil {
		t.Fatal("expected an error for a section tag that never terminates")
	}
	if _, err := os.Stat(filepath.Join(dir, "project.godot")); !os.IsNotExist(err) {
		t.Fatalf("project.godot written despite invalid content: %v", err)
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
	eachAssignment(content, func(_, _ string, _, _, _ int) bool { return true })
}

// TestSettingsStoreMultilineValue checks the typed store treats a multi-line
// dictionary value as a single value, so it never splices the file.
func TestSettingsStoreMultilineValue(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"),
		"config_version=5\n\n[input]\n\njump={\n\"deadzone\": 0.5,\n\"events\": []\n}\n\n[application]\n\nconfig/name=\"Demo\"\n")

	store, failure := openSettings(root)
	if failure != nil {
		t.Fatalf("openSettings: %+v", failure.Content)
	}
	value, ok := store.Get("input", "jump")
	if !ok {
		t.Fatal("jump is missing")
	}
	if got, want := godotconfig.FormatValue(value), "{\n\"deadzone\": 0.5,\n\"events\": []\n}"; got != want {
		t.Errorf("jump = %q, want %q", got, want)
	}
}

// TestSettingsStoreDeleteMultiline checks Delete removes a whole multi-line
// value, leaves an unrelated key behind, and survives a reparse.
func TestSettingsStoreDeleteMultiline(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"),
		"config_version=5\n\n[input]\n\njump={\n\"deadzone\": 0.5,\n\"events\": []\n}\nmove={\"deadzone\": 0.5}\n")

	store, failure := openSettings(root)
	if failure != nil {
		t.Fatalf("openSettings: %+v", failure.Content)
	}
	if !store.Delete("input", "jump") {
		t.Fatal("Delete reported jump absent")
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened, failure := openSettings(root)
	if failure != nil {
		t.Fatalf("reopen: %+v", failure.Content)
	}
	if _, ok := reopened.Get("input", "jump"); ok {
		t.Error("jump still present")
	}
	if _, ok := reopened.Get("input", "move"); !ok {
		t.Error("move was removed")
	}
}

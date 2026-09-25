package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

const fakeUIDVersion = `if [ "$1" = "--version" ]; then echo "4.7.2.stable.official.fake"; exit 0; fi`

func TestGetUIDSchema(t *testing.T) {
	tool := getUID()
	if tool.Name != "get_uid" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Get the UID for a specific file in a Godot project (for Godot 4.4+)" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "filePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 1 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"filePath"} {
		property, ok := props[key].(map[string]any)
		if !ok || property["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
}

func TestGetUIDSuccess(t *testing.T) {
	fake := fakeGodotOps(t, fakeUIDVersion+"\nprintf '<<<GO_DOT_MCP_JSON_START>>>\n{\"file\":\"res://player.gd\",\"uid\":\"uid://abc\",\"exists\":true}\n<<<GO_DOT_MCP_JSON_END>>>\n'")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "script")

	result := callToolArgs(t, getUID(), root, `{"filePath":"player.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, `"uid": "uid://abc"`) {
		t.Errorf("text = %q, want it to contain pretty uid field", text)
	}
	if !strings.Contains(text, `"exists": true`) {
		t.Errorf("text = %q, want it to contain pretty exists field", text)
	}
	if strings.Contains(text, jsonStartMarker) || strings.Contains(text, jsonEndMarker) {
		t.Errorf("text still contains markers: %q", text)
	}
}

func TestGetUIDNoMarkersStructured(t *testing.T) {
	fake := fakeGodotOps(t, fakeUIDVersion+"\necho 'Getting UID for file: res://player.gd'")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "script")

	result := callToolArgs(t, getUID(), root, `{"filePath":"player.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["file"] != "player.gd" {
		t.Errorf("file = %v, want player.gd", structured["file"])
	}
	if structured["absolutePath"] != filepath.Join(root, "player.gd") {
		t.Errorf("absolutePath = %v", structured["absolutePath"])
	}
	if structured["exists"] != false {
		t.Errorf("exists = %v, want false", structured["exists"])
	}
	if _, ok := structured["uid"]; ok {
		t.Errorf("uid = %v, want it absent when no structured output was emitted", structured["uid"])
	}
}

func TestGetUIDVersionGate(t *testing.T) {
	fake := fakeGodotOps(t, `if [ "$1" = "--version" ]; then echo "4.3.1.stable"; exit 0; fi`+"\nexit 1")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "script")

	result := callToolArgs(t, getUID(), root, `{"filePath":"player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable" {
		t.Errorf("message = %q", got)
	}
}

func TestGetUIDValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "script")
	notAProject := t.TempDir()

	tests := []struct {
		name    string
		project string
		args    string
		want    string
	}{
		{name: "missing params", args: `{}`, want: "Missing required parameters"},
		{name: "invalid path", project: "../escape", args: `{"filePath":"player.gd"}`, want: "Invalid path"},
		{name: "not a project", project: notAProject, args: `{"filePath":"player.gd"}`, want: "Not a valid Godot project"},
		{name: "missing file", project: root, args: `{"filePath":"missing.gd"}`, want: "File does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, getUID(), tt.project, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestGetUIDScriptFailure(t *testing.T) {
	fake := fakeGodotOps(t, fakeUIDVersion+"\necho 'Failed to get UID: boom' >&2; exit 1")
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "script")

	result := callToolArgs(t, getUID(), root, `{"filePath":"player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to get UID: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestGetUIDSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "script")

	result := callToolArgs(t, getUID(), root, `{"filePath":"player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to get UID: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

func TestGetUIDVersionFailure(t *testing.T) {
	fake := fakeGodotOps(t, `echo "boom" >&2; exit 1`)
	t.Setenv(godot.PathEnv, fake)
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "player.gd"), "script")

	result := callToolArgs(t, getUID(), root, `{"filePath":"player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to get UID: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

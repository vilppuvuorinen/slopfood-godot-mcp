package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestAttachScriptSchema(t *testing.T) {
	tool := attachScript()
	if tool.Name != "attach_script" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Attach a GDScript to a scene node (headless)" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scenePath,nodePath,scriptPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "scenePath", "nodePath", "scriptPath"} {
		prop, exists := props[key]
		if !exists {
			t.Fatalf("missing property %q", key)
		}
		if prop.(map[string]any)["type"] != "string" {
			t.Errorf("property %q = %v", key, prop)
		}
	}
}

func TestAttachScriptSuccess(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, attachScript(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Player","scriptPath":"scripts/player.gd"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "attach_script succeeded.") {
		t.Errorf("text = %q", text)
	}
	for _, want := range []string{`"scene_path":"scenes/main.tscn"`, `"node_path":"root/Player"`, `"script_path":"scripts/player.gd"`} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestAttachScriptValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath, scenePath, nodePath, and scriptPath are required."},
		{name: "missing scriptPath", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"main.tscn","nodePath":"root/Player"}`, want: "projectPath, scenePath, nodePath, and scriptPath are required."},
		{name: "invalid path", args: `{"projectPath":"../escape","scenePath":"main.tscn","nodePath":"root/Player","scriptPath":"scripts/player.gd"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"main.tscn","nodePath":"root/Player","scriptPath":"scripts/player.gd"}`, want: "Not a valid Godot project: "},
		{name: "missing scene", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"missing.tscn","nodePath":"root/Player","scriptPath":"scripts/player.gd"}`, want: "Scene file does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, attachScript(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestAttachScriptScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to load script: res://scripts/player.gd' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, attachScript(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root","scriptPath":"scripts/player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "attach_script failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestAttachScriptSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, attachScript(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","nodePath":"root/Player","scriptPath":"scripts/player.gd"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "attach_script failed: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

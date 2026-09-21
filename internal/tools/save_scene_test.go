package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
)

func TestSaveSceneSchema(t *testing.T) {
	tool := saveScene()
	if tool.Name != "save_scene" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Save changes to a scene file" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,scenePath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 3 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"projectPath", "scenePath", "newPath"} {
		property, ok := props[key].(map[string]any)
		if !ok || property["type"] != "string" {
			t.Errorf("%s property = %v", key, props[key])
		}
	}
}

func TestSaveSceneInPlace(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, saveScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Scene saved successfully to: scenes/main.tscn") {
		t.Errorf("text = %q", text)
	}
	if !strings.Contains(text, `"scene_path":"scenes/main.tscn"`) {
		t.Errorf("text = %q, want it to contain the scene_path", text)
	}
	if strings.Contains(text, "new_path") {
		t.Errorf("text = %q, want it not to contain new_path", text)
	}
}

func TestSaveSceneVariant(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, saveScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn","newPath":"variants/main_variant.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	text := result.Content[0].Text
	if !strings.HasPrefix(text, "Scene saved successfully to: variants/main_variant.tscn") {
		t.Errorf("text = %q", text)
	}
	if !strings.Contains(text, `"new_path":"variants/main_variant.tscn"`) {
		t.Errorf("text = %q, want it to contain the new_path", text)
	}
}

func TestSaveSceneSnakeCaseAliases(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo "$@"`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, saveScene(), `{"project_path":`+jsonString(root)+`,"scene_path":"scenes/main.tscn","new_path":"variants/main_variant.tscn"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Scene saved successfully to: variants/main_variant.tscn") {
		t.Errorf("text = %q", result.Content[0].Text)
	}
}

func TestSaveSceneValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "Missing required parameters"},
		{name: "invalid path", args: `{"projectPath":"../escape","scenePath":"scenes/main.tscn"}`, want: "Invalid path"},
		{name: "invalid new path", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"scenes/main.tscn","newPath":"../escape"}`, want: "Invalid new path"},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"scenePath":"scenes/main.tscn"}`, want: "Not a valid Godot project"},
		{name: "missing scene", args: `{"projectPath":` + jsonString(root) + `,"scenePath":"missing.tscn"}`, want: "Scene file does not exist: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, saveScene(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}

func TestSaveSceneScriptFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, fakeGodotOps(t, `echo 'Failed to save scene: boom' >&2; exit 1`))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, saveScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to save scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestSaveSceneSpawnFailure(t *testing.T) {
	t.Setenv(godot.PathEnv, filepath.Join(t.TempDir(), "missing-godot"))
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "scenes", "main.tscn"), "scene")

	result := callToolArgs(t, saveScene(), `{"projectPath":`+jsonString(root)+`,"scenePath":"scenes/main.tscn"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if !strings.HasPrefix(result.Content[0].Text, "Failed to save scene: ") {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}
